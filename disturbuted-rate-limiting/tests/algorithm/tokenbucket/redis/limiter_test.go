package redis_test

import (
	"context"
	"errors"
	"fmt"
	ratelimit "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting"
	fixedredis "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/fixedwindow/redis"
	sw "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/tokenbucket"
	remote "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/tokenbucket/redis"
	redis "github.com/redis/go-redis/v9"
	"math/rand"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func backend(t *testing.T) (*redis.Client, remote.Config) {
	t.Helper()
	url := os.Getenv("REDIS_TEST_URL")
	if url == "" {
		t.Skip("set REDIS_TEST_URL for real Redis token-bucket integration")
	}
	options, err := redis.ParseURL(url)
	if err != nil {
		t.Fatal(err)
	}
	options.MaxRetries = -1
	options.ContextTimeoutEnabled = true
	options.PoolSize = 8
	options.MaxActiveConns = 8
	client := redis.NewClient(options)
	cfg := remote.Config{Config: fixedredis.Config{PolicyID: "users", Namespace: fmt.Sprintf("tbtest:%s:%d", t.Name(), time.Now().UnixNano()), Limit: 3, Window: 10 * time.Second, MaxKeys: 2, Timeout: time.Second}, Capacity: 3}
	t.Cleanup(func() {
		keys, _ := client.Keys(context.Background(), cfg.Namespace+":tb:*").Result()
		if len(keys) > 0 {
			client.Del(context.Background(), keys...)
		}
		client.Close()
	})
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	return client, cfg
}
func stateKey(t *testing.T, c *redis.Client, cfg remote.Config) string {
	t.Helper()
	keys, err := c.Keys(context.Background(), cfg.Namespace+":tb:*:config").Result()
	if err != nil || len(keys) != 1 {
		t.Fatalf("keys %v %v", keys, err)
	}
	return strings.TrimSuffix(keys[0], ":config")
}
func TestRAMRedisRefillParity(t *testing.T) {
	client, cfg := backend(t)
	cfg.Limit = 2
	l, err := remote.New(client, cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := l.Ready(ctx); err != nil {
		t.Fatal(err)
	}
	key := stateKey(t, client, cfg)
	serverNow, err := client.Time(ctx).Result()
	if err != nil {
		t.Fatal(err)
	}
	base := serverNow.Truncate(time.Millisecond).Add(time.Hour)
	now := base
	ram, _ := sw.New(sw.Config{Capacity: cfg.Capacity, Limit: cfg.Limit, Window: cfg.Window, MaxKeys: cfg.MaxKeys}, sw.WithClock(func() time.Time { return now }))
	steps := []struct {
		seconds int
		key     string
	}{{1, "u"}, {9, "u"}, {10, "u"}, {11, "u"}, {5, "u"}, {12, "v"}, {13, "w"}, {19, "u"}, {22, "w"}, {23, "u"}, {40, "u"}}
	for _, step := range steps {
		now = base.Add(time.Duration(step.seconds) * time.Second)
		// Seed the persisted high-water mark ahead of real server time to exercise
		// exact timestamps deterministically; never expose a test clock in production.
		last, err := client.HGet(ctx, key, "_last").Int64()
		if err != nil && err != redis.Nil {
			t.Fatal(err)
		}
		if now.UnixMilli() > last {
			if err := client.HSet(ctx, key, "_last", now.UnixMilli()).Err(); err != nil {
				t.Fatal(err)
			}
		}
		got, err := l.Evaluate(ctx, step.key)
		want := ram.Allow(step.key)
		if err != nil || got != want {
			t.Fatalf("at %d key %s: Redis %+v err %v RAM %+v", step.seconds, step.key, got, err, want)
		}
	}
	if usage, err := l.Usage(ctx); err != nil || usage != ram.Usage() {
		t.Fatalf("usage %v %v RAM %v", usage, err, ram.Usage())
	}
}
func TestReplicasConcurrencyCapacityAndCompatibility(t *testing.T) {
	client, cfg := backend(t)
	cfg.Limit = 1
	cfg.Capacity = 25
	cfg.Window = time.Hour
	first, err := remote.New(client, cfg)
	if err != nil {
		t.Fatal(err)
	}
	other := redis.NewClient(&redis.Options{Addr: client.Options().Addr, Username: client.Options().Username, Password: client.Options().Password, DB: client.Options().DB, PoolSize: 8, MaxActiveConns: 8, MaxRetries: -1, ContextTimeoutEnabled: true})
	defer other.Close()
	second, err := remote.New(other, cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	var wg sync.WaitGroup
	var allowed atomic.Int32
	for i := range 200 {
		wg.Go(func() {
			l := first
			if i%2 == 0 {
				l = second
			}
			d, err := l.Evaluate(ctx, "shared")
			if err != nil {
				t.Error(err)
			} else if d.Allowed {
				allowed.Add(1)
			} else if d.Reason != ratelimit.ReasonRateLimited {
				t.Errorf("unexpected %+v", d)
			}
		})
	}
	wg.Wait()
	if allowed.Load() != 25 {
		t.Fatalf("allowed %d", allowed.Load())
	}
	if d, err := second.Evaluate(ctx, "other"); err != nil || !d.Allowed {
		t.Fatalf("isolation %+v %v", d, err)
	}
	if d, err := first.Evaluate(ctx, "third"); err != nil || d.Reason != ratelimit.ReasonCapacityExceeded {
		t.Fatalf("capacity %+v %v", d, err)
	}
	cfg.Limit++
	incompatible, _ := remote.New(client, cfg)
	if incompatible.Ready(ctx) == nil {
		t.Fatal("readiness accepted incompatible replica")
	}
	if _, err := incompatible.Evaluate(ctx, "shared"); err == nil {
		t.Fatal("accepted incompatible quota")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := first.Evaluate(canceled, "shared"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation %v", err)
	}
	if err := other.Close(); err != nil {
		t.Fatal(err)
	}
	if d, err := second.Evaluate(ctx, "shared"); err == nil || d.Allowed {
		t.Fatalf("failure did not fail closed %+v %v", d, err)
	}
}
func TestExpiryInvalidKeysAndReadiness(t *testing.T) {
	client, cfg := backend(t)
	cfg.Limit = 1
	cfg.Window = 100 * time.Millisecond
	cfg.Capacity = 1
	cfg.MaxKeys = 1
	l, err := remote.New(client, cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := l.Ready(ctx); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"", " \t", strings.Repeat("x", 257)} {
		d, err := l.Evaluate(ctx, key)
		if err != nil || d.Reason != ratelimit.ReasonInvalidKey {
			t.Fatalf("invalid key %+v %v", d, err)
		}
	}
	if d, err := l.Evaluate(ctx, "_last"); err != nil || !d.Allowed {
		t.Fatalf("readiness or validation consumed quota %+v %v", d, err)
	}
	key := stateKey(t, client, cfg)
	deadline := time.Now().Add(2 * time.Second)
	for {
		n, err := client.Exists(ctx, key, key+":expiry").Result()
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("idle logs did not expire")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if u, err := l.Usage(ctx); err != nil || u != 0 {
		t.Fatalf("idle usage %v %v", u, err)
	}
	if d, err := l.Evaluate(ctx, "new"); err != nil || !d.Allowed {
		t.Fatalf("capacity not released %+v %v", d, err)
	}
}
func TestConstructorValidation(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1})
	defer client.Close()
	cfg := remote.Config{Config: fixedredis.Config{PolicyID: "p", Namespace: "test", Limit: 1, Window: time.Second, MaxKeys: 1, Timeout: time.Second}, Capacity: 3}
	if _, err := remote.New(nil, cfg); err == nil {
		t.Fatal("nil client accepted")
	}
	for _, mutate := range []func(*remote.Config){func(c *remote.Config) { c.Capacity = 0 }, func(c *remote.Config) { c.Limit = 0 }, func(c *remote.Config) { c.MaxKeys = 0 }, func(c *remote.Config) { c.Window = time.Nanosecond }, func(c *remote.Config) { c.Namespace = "bad{}" }, func(c *remote.Config) { c.Timeout = 0 }} {
		bad := cfg
		mutate(&bad)
		if _, err := remote.New(client, bad); err == nil {
			t.Fatal("invalid configuration accepted")
		}
	}
	retrying := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	defer retrying.Close()
	if _, err := remote.New(retrying, cfg); err == nil {
		t.Fatal("automatic retries accepted")
	}
}

func TestExpiryWaitsForFullRefillAcrossIdentities(t *testing.T) {
	client, cfg := backend(t)
	cfg.Limit = 1
	cfg.Capacity = 3
	cfg.Window = 10 * time.Second
	l, err := remote.New(client, cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err = l.Ready(ctx); err != nil {
		t.Fatal(err)
	}
	key := stateKey(t, client, cfg)
	serverNow, err := client.Time(ctx).Result()
	if err != nil {
		t.Fatal(err)
	}
	base := serverNow.Truncate(time.Millisecond).Add(time.Hour)
	setNow := func(now time.Time) {
		t.Helper()
		if err := client.HSet(ctx, key, "_last", now.UnixMilli()).Err(); err != nil {
			t.Fatal(err)
		}
	}
	setNow(base)
	for range 3 {
		if d, err := l.Evaluate(ctx, "a"); err != nil || !d.Allowed {
			t.Fatalf("burst %+v %v", d, err)
		}
	}
	setNow(base.Add(time.Second))
	if d, err := l.Evaluate(ctx, "b"); err != nil || !d.Allowed {
		t.Fatalf("second identity %+v %v", d, err)
	}
	// B becomes full at +11s; A must remain tracked until +30s.
	deadline, err := client.PExpireTime(ctx, key).Result()
	if err != nil || deadline != time.Duration(base.Add(30*time.Second).UnixMilli())*time.Millisecond {
		t.Fatalf("premature state expiry: %v %v", deadline, err)
	}
	setNow(base.Add(11 * time.Second))
	if u, err := l.Usage(ctx); err != nil || u != 0.5 {
		t.Fatalf("partial bucket lost: %v %v", u, err)
	}
	setNow(base.Add(30 * time.Second))
	if u, err := l.Usage(ctx); err != nil || u != 0 {
		t.Fatalf("full bucket retained: %v %v", u, err)
	}
}

func TestRepeatedFractionalChecksAndRandomizedParity(t *testing.T) {
	client, cfg := backend(t)
	cfg.Limit = 1
	cfg.Window = time.Second
	cfg.Capacity = 1
	l, err := remote.New(client, cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err = l.Ready(ctx); err != nil {
		t.Fatal(err)
	}
	key := stateKey(t, client, cfg)
	realNow, err := client.Time(ctx).Result()
	if err != nil {
		t.Fatal(err)
	}
	now := realNow.Truncate(time.Millisecond).Add(time.Hour)
	base := now
	ram, err := sw.New(sw.Config{Limit: cfg.Limit, Window: cfg.Window, Capacity: cfg.Capacity, MaxKeys: cfg.MaxKeys}, sw.WithClock(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	rng := rand.New(rand.NewSource(42))
	for step := 0; step < 2500; step++ {
		if step <= 10 {
			now = base.Add(time.Duration(step) * 100 * time.Millisecond)
		} else {
			now = now.Add(time.Duration(rng.Intn(101)) * time.Millisecond)
		}
		if err := client.HSet(ctx, key, "_last", now.UnixMilli()).Err(); err != nil {
			t.Fatal(err)
		}
		got, err := l.Evaluate(ctx, "u")
		want := ram.Allow("u")
		want.RetryAfter = (want.RetryAfter + time.Millisecond - 1) / time.Millisecond * time.Millisecond
		if err != nil || got != want {
			t.Fatalf("step %d: Redis %+v %v RAM %+v", step, got, err, want)
		}
	}
}
