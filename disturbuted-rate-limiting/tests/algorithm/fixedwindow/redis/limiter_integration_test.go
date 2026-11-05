package redis_test

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	ratelimit "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting"
	rediswindow "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/fixedwindow/redis"
	redisclient "github.com/redis/go-redis/v9"
)

func clients(t *testing.T) (*redisclient.Client, *redisclient.Client) {
	t.Helper()
	address := os.Getenv("REDIS_TEST_URL")
	if address == "" {
		t.Skip("set REDIS_TEST_URL for real Redis integration")
	}
	create := func() *redisclient.Client {
		options, err := redisclient.ParseURL(address)
		if err != nil {
			t.Fatal("invalid REDIS_TEST_URL")
		}
		options.MaxRetries = -1
		options.ContextTimeoutEnabled = true
		client := redisclient.NewClient(options)
		t.Cleanup(func() { client.Close() })
		if err := client.Ping(context.Background()).Err(); err != nil {
			t.Fatal(err)
		}
		return client
	}
	return create(), create()
}

func config(t *testing.T) rediswindow.Config {
	return rediswindow.Config{PolicyID: "policy", Namespace: fmt.Sprintf("test:%s:%d", t.Name(), time.Now().UnixNano()), Limit: 10, Window: 876000 * time.Hour, MaxKeys: 32, Timeout: time.Second}
}

func TestSharedAtomicAllowance(t *testing.T) {
	first, second := clients(t)
	cfg := config(t)
	cleanupNamespace(t, first, cfg.Namespace)
	a, err := rediswindow.New(first, cfg)
	if err != nil {
		t.Fatal(err)
	}
	b, err := rediswindow.New(second, cfg)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan ratelimit.Decision, 200)
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			l := a
			if i%2 == 0 {
				l = b
			}
			decision, err := l.Evaluate(context.Background(), "shared")
			if err != nil {
				t.Error(err)
			}
			results <- decision
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)
	allowed := 0
	seen := map[int]bool{}
	for decision := range results {
		if decision.Allowed {
			allowed++
			if seen[decision.Remaining] {
				t.Error("duplicate remaining allowance")
			}
			seen[decision.Remaining] = true
		} else if decision.Reason != ratelimit.ReasonRateLimited || decision.RetryAfter <= 0 {
			t.Errorf("invalid rejection: %+v", decision)
		}
	}
	if allowed != cfg.Limit {
		t.Fatalf("allowed=%d want=%d", allowed, cfg.Limit)
	}
	// No command should run after cancellation, nor be retried on ambiguity.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := a.Evaluate(ctx, "new"); err == nil {
		t.Fatal("cancellation ignored")
	}
}

func TestSharedCapacityAndMetadataIsolation(t *testing.T) {
	first, second := clients(t)
	cfg := config(t)
	cleanupNamespace(t, first, cfg.Namespace)
	cfg.Limit = 1
	a, err := rediswindow.New(first, cfg)
	if err != nil {
		t.Fatal(err)
	}
	b, err := rediswindow.New(second, cfg)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan bool, 100)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			l := a
			if i%2 == 0 {
				l = b
			}
			decision, err := l.Evaluate(context.Background(), strconv.Itoa(i))
			if err != nil {
				t.Error(err)
			}
			results <- decision.Allowed
		}(i)
	}
	wg.Wait()
	close(results)
	allowed := 0
	for value := range results {
		if value {
			allowed++
		}
	}
	if allowed != cfg.MaxKeys {
		t.Fatalf("tracked %d keys, want %d", allowed, cfg.MaxKeys)
	}
	other := cfg
	other.PolicyID = "metadata"
	l, err := rediswindow.New(first, other)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"_config", "_window", "_last"} {
		decision, err := l.Evaluate(context.Background(), key)
		if err != nil || !decision.Allowed {
			t.Fatalf("metadata collision: %+v %v", decision, err)
		}
	}
}

func TestExpiryAndConfigurationMismatch(t *testing.T) {
	client, _ := clients(t)
	cfg := config(t)
	cleanupNamespace(t, client, cfg.Namespace)
	a, err := rediswindow.New(client, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Evaluate(context.Background(), "u"); err != nil {
		t.Fatal(err)
	}
	changed := cfg
	changed.Limit++
	b, err := rediswindow.New(client, changed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Evaluate(context.Background(), "u"); err == nil {
		t.Fatal("inconsistent replica configuration accepted")
	}
	short := cfg
	short.PolicyID = "expiring"
	short.Window = 500 * time.Millisecond
	short.Limit = 1
	l, err := rediswindow.New(client, short)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.Evaluate(context.Background(), "u"); err != nil {
		t.Fatal(err)
	}
	keys, err := client.Keys(context.Background(), cfg.Namespace+"*").Result()
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		remaining := 0
		for _, key := range keys {
			ttl, err := client.PTTL(context.Background(), key).Result()
			if err != nil {
				t.Fatal(err)
			}
			if ttl > 0 && ttl <= 500*time.Millisecond {
				remaining++
			}
		}
		if remaining == 0 {
			decision, err := l.Evaluate(context.Background(), "u")
			if err != nil || !decision.Allowed {
				t.Fatalf("expired quota did not reset: %+v %v", decision, err)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("short policy did not expire")
}

func cleanupNamespace(t *testing.T, client *redisclient.Client, namespace string) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		keys, err := client.Keys(ctx, namespace+":fw:*").Result()
		if err == nil && len(keys) > 0 {
			if err := client.Del(ctx, keys...).Err(); err != nil {
				t.Error(err)
			}
		}
	})
}
