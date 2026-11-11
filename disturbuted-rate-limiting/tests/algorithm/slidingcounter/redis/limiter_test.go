package redis_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	ratelimit "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting"
	sc "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/slidingcounter"
	remote "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/slidingcounter/redis"
	redis "github.com/redis/go-redis/v9"
)

func TestRedisMatchesRAMAcrossBoundaries(t *testing.T) {
	url := os.Getenv("REDIS_TEST_URL")
	if url == "" {
		t.Skip("set REDIS_TEST_URL for real Redis integration")
	}
	opts, err := redis.ParseURL(url)
	if err != nil {
		t.Fatal(err)
	}
	opts.MaxRetries = -1
	opts.ContextTimeoutEnabled = true
	client := redis.NewClient(opts)
	defer client.Close()
	ctx := context.Background()
	config := remote.Config{PolicyID: "counter", Namespace: fmt.Sprintf("sc-test:%d", time.Now().UnixNano()), Limit: 4, Window: time.Second, MaxKeys: 1, Timeout: time.Second}
	t.Cleanup(func() {
		keys, _ := client.Keys(ctx, config.Namespace+":*").Result()
		if len(keys) > 0 {
			client.Del(ctx, keys...)
		}
	})
	a, err := remote.New(client, config)
	if err != nil {
		t.Fatal(err)
	}
	b, err := remote.New(client, config)
	if err != nil {
		t.Fatal(err)
	}
	if err = a.Ready(ctx); err != nil {
		t.Fatal(err)
	}
	if err = b.Ready(ctx); err != nil {
		t.Fatal(err)
	}
	if usage, err := a.Usage(ctx); err != nil || usage != 0 {
		t.Fatal(usage, err)
	}
	keys, err := client.Keys(ctx, config.Namespace+":sc:*").Result()
	if err != nil {
		t.Fatal(err)
	}
	var hash string
	for _, k := range keys {
		if strings.HasSuffix(k, "}") {
			hash = k
		}
	}
	if hash == "" {
		t.Fatal("state hash absent")
	}
	// Clamp the production Redis clock into the future for exact deterministic
	// parity tests without replacing TIME or changing production script code.
	serverNow, err := client.Time(ctx).Result()
	if err != nil {
		t.Fatal(err)
	}
	base := (serverNow.UnixMilli()/1000 + 1000) * 1000
	now := time.UnixMilli(base)
	ram, _ := sc.New(sc.Config{Limit: 4, Window: time.Second, MaxKeys: 1}, sc.WithClock(func() time.Time { return now }))
	for i, offset := range []int64{0, 0, 0, 0, 0, 1000, 1249, 1250, 1250, 1500, 2000, 2250, 4000} {
		if err := client.HSet(ctx, hash, "_last", base+offset).Err(); err != nil {
			t.Fatal(err)
		}
		now = time.UnixMilli(base + offset)
		want := ram.Allow("u")
		limiter := a
		if i%2 == 1 {
			limiter = b
		}
		got, err := limiter.Evaluate(ctx, "u")
		if err != nil || got != want {
			t.Fatalf("step %d: got %+v %v want %+v", i, got, err, want)
		}
	}
	if d, err := a.Evaluate(ctx, "other"); err != nil || d.Reason != ratelimit.ReasonCapacityExceeded {
		t.Fatal(d, err)
	}
	if err := client.HSet(ctx, hash, "_last", base+6000).Err(); err != nil {
		t.Fatal(err)
	}
	if usage, err := a.Usage(ctx); err != nil || usage != 0 {
		t.Fatal(usage, err)
	}
	// Two replicas must atomically admit at most Limit concurrent requests.
	var wg sync.WaitGroup
	var admitted atomic.Int32
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			l := a
			if i%2 == 1 {
				l = b
			}
			d, err := l.Evaluate(ctx, "fresh")
			if err != nil {
				t.Error(err)
			}
			if d.Allowed {
				admitted.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if admitted.Load() != 4 {
		t.Fatal(admitted.Load())
	}
	config.Limit = 5
	mismatch, err := remote.New(client, config)
	if err != nil {
		t.Fatal(err)
	}
	if err = mismatch.Ready(ctx); err == nil {
		t.Fatal("policy mismatch accepted")
	}
	if _, err = mismatch.Evaluate(ctx, "fresh"); err == nil {
		t.Fatal("evaluation ignored policy mismatch")
	}
	if d, err := a.Evaluate(ctx, " "); err != nil || d.Reason != ratelimit.ReasonInvalidKey {
		t.Fatal(d, err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = a.Evaluate(canceled, "u"); err != context.Canceled {
		t.Fatal(err)
	}
	// Idle counter data expires independently of further requests; the policy
	// guard remains to prevent incompatible replicas from reusing the scope.
	ttlConfig := config
	ttlConfig.Namespace += ":ttl"
	ttlConfig.Window = 30 * time.Millisecond
	ttlLimiter, err := remote.New(client, ttlConfig)
	if err != nil {
		t.Fatal(err)
	}
	if d, err := ttlLimiter.Evaluate(ctx, "idle"); err != nil || !d.Allowed {
		t.Fatal(d, err)
	}
	time.Sleep(100 * time.Millisecond)
	ttlKeys, err := client.Keys(ctx, ttlConfig.Namespace+":*").Result()
	if err != nil {
		t.Fatal(err)
	}
	if len(ttlKeys) != 1 || !strings.HasSuffix(ttlKeys[0], ":config") {
		t.Fatal("idle state did not expire", ttlKeys)
	}
	client.Close()
	if _, err = a.Evaluate(ctx, "u"); err == nil {
		t.Fatal("backend failure hidden")
	}
}
