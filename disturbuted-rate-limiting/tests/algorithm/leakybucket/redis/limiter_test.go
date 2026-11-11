package redis_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	ratelimit "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting"
	fixedredis "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/fixedwindow/redis"
	sw "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/leakybucket"
	remote "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/leakybucket/redis"
	redis "github.com/redis/go-redis/v9"
	"io"
	"math/rand"
	"net"
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
		t.Skip("set REDIS_TEST_URL for real Redis leaky-bucket integration")
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
	cfg := remote.Config{Capacity: 3, Config: fixedredis.Config{PolicyID: "users", Namespace: fmt.Sprintf("lbtest:%s:%d", t.Name(), time.Now().UnixNano()), Limit: 3, Window: 10 * time.Second, MaxKeys: 2, Timeout: time.Second}}
	t.Cleanup(func() {
		keys, _ := client.Keys(context.Background(), cfg.Namespace+":lb:*").Result()
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
	keys, err := c.Keys(context.Background(), cfg.Namespace+":lb:*:config").Result()
	if err != nil || len(keys) != 1 {
		t.Fatalf("keys %v %v", keys, err)
	}
	return strings.TrimSuffix(keys[0], ":config")
}
func TestRAMRedisDrainParity(t *testing.T) {
	client, cfg := backend(t)
	cfg.Limit = 2
	cfg.Capacity = 2
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
	cfg.Limit = 2
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
	cfg.Capacity = 1
	cfg.Window = 100 * time.Millisecond
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
	cfg := remote.Config{Capacity: 3, Config: fixedredis.Config{PolicyID: "p", Namespace: "test", Limit: 1, Window: time.Second, MaxKeys: 1, Timeout: time.Second}}
	if _, err := remote.New(nil, cfg); err == nil {
		t.Fatal("nil client accepted")
	}
	for _, mutate := range []func(*remote.Config){func(c *remote.Config) { c.Limit = 0 }, func(c *remote.Config) { c.MaxKeys = 0 }, func(c *remote.Config) { c.Window = time.Nanosecond }, func(c *remote.Config) { c.Namespace = "bad{}" }, func(c *remote.Config) { c.Timeout = 0 }} {
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

func TestRandomizedRAMRedisParity(t *testing.T) {
	for _, rate := range []struct {
		limit, capacity int
		window          time.Duration
	}{{3, 7, time.Second}, {7, 3, 13 * time.Millisecond}, {100, 2, time.Millisecond}} {
		t.Run(fmt.Sprintf("%d-%s", rate.limit, rate.window), func(t *testing.T) {
			client, cfg := backend(t)
			cfg.Limit, cfg.Capacity, cfg.Window = rate.limit, rate.capacity, rate.window
			cfg.MaxKeys = 5
			remoteLimiter, err := remote.New(client, cfg)
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			if err := remoteLimiter.Ready(ctx); err != nil {
				t.Fatal(err)
			}
			key := stateKey(t, client, cfg)
			serverNow, err := client.Time(ctx).Result()
			if err != nil {
				t.Fatal(err)
			}
			now := serverNow.Truncate(time.Millisecond).Add(time.Hour)
			high := now
			ram, err := sw.New(sw.Config{Limit: cfg.Limit, Capacity: cfg.Capacity, Window: cfg.Window, MaxKeys: cfg.MaxKeys}, sw.WithClock(func() time.Time { return now }))
			if err != nil {
				t.Fatal(err)
			}
			rng := rand.New(rand.NewSource(99))
			for i := range 1000 {
				now = now.Add(time.Duration(rng.Intn(21)-5) * time.Millisecond)
				if i%113 == 0 {
					now = now.Add(cfg.Window * time.Duration(cfg.Capacity+1))
				}
				if now.After(high) {
					high = now
				}
				if err := client.HSet(ctx, key, "_last", high.UnixMilli()).Err(); err != nil {
					t.Fatal(err)
				}
				identity := fmt.Sprintf("u%d", rng.Intn(8))
				got, err := remoteLimiter.Evaluate(ctx, identity)
				if err != nil {
					t.Fatal(err)
				}
				want := ram.Allow(identity)
				want.RetryAfter = (want.RetryAfter + time.Millisecond - 1) / time.Millisecond * time.Millisecond
				if got != want {
					t.Fatalf("step %d: Redis %+v RAM rounded to ms %+v", i, got, want)
				}
				if i%19 == 0 {
					got, err := remoteLimiter.Usage(ctx)
					if err != nil || got != ram.Usage() {
						t.Fatalf("usage at %d: %v %v RAM %v", i, got, err, ram.Usage())
					}
				}
			}
		})
	}
}

func TestPartialDrainExpiryAndPolicyScope(t *testing.T) {
	client, cfg := backend(t)
	cfg.Limit, cfg.Capacity, cfg.Window, cfg.MaxKeys = 1, 3, time.Second, 3
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
	pin := func(offset time.Duration) {
		t.Helper()
		if err := client.HSet(ctx, key, "_last", base.Add(offset).UnixMilli()).Err(); err != nil {
			t.Fatal(err)
		}
	}
	pin(0)
	for range 3 {
		if d, err := l.Evaluate(ctx, "long"); err != nil || !d.Allowed {
			t.Fatalf("burst %+v %v", d, err)
		}
	}
	pin(time.Second)
	if d, err := l.Evaluate(ctx, "short"); err != nil || !d.Allowed {
		t.Fatalf("other %+v %v", d, err)
	}
	// The short bucket drains at +2s; the long bucket must survive until +3s.
	ttl, err := client.PExpireTime(ctx, key).Result()
	if err != nil {
		t.Fatal(err)
	}
	if ttl != time.Duration(base.Add(3*time.Second).UnixMilli())*time.Millisecond {
		t.Fatalf("state expiry %v", ttl)
	}
	pin(2 * time.Second)
	if u, err := l.Usage(ctx); err != nil || u != 1.0/3 {
		t.Fatalf("partial expiry usage %v %v", u, err)
	}
	if d, err := l.Evaluate(ctx, "long"); err != nil || !d.Allowed || d.Remaining != 1 {
		t.Fatalf("partial bucket reset %+v %v", d, err)
	}
	// Capacity changes in a shared scope must fail; explicit revision and tenant
	// scopes intentionally create separate quota.
	incompatibleCfg := cfg
	incompatibleCfg.Capacity++
	incompatible, err := remote.New(client, incompatibleCfg)
	if err != nil {
		t.Fatal(err)
	}
	if incompatible.Ready(ctx) == nil {
		t.Fatal("capacity mismatch accepted")
	}
	for _, alter := range []func(*remote.Config){func(c *remote.Config) { c.Revision = "v2" }, func(c *remote.Config) { c.TenantID = "other" }, func(c *remote.Config) { c.PolicyID = "other" }} {
		isolated := cfg
		alter(&isolated)
		other, err := remote.New(client, isolated)
		if err != nil {
			t.Fatal(err)
		}
		if d, err := other.Evaluate(ctx, "long"); err != nil || !d.Allowed || d.Remaining != 2 {
			t.Fatalf("scope isolation %+v %v", d, err)
		}
	}
	// Warm scripts are deliberately flushed to exercise safe NOSCRIPT fallback.
	if err := client.ScriptFlush(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	if d, err := l.Evaluate(ctx, "long"); err != nil || !d.Allowed || d.Remaining != 0 {
		t.Fatalf("NOSCRIPT fallback %+v %v", d, err)
	}
}

func TestBackendDeadlineFailsClosed(t *testing.T) {
	backendClient, cfg := backend(t)
	cfg.Timeout = 30 * time.Millisecond
	var stalled atomic.Bool
	opts := *backendClient.Options()
	opts.MaxRetries = -1
	opts.Dialer = func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := (&net.Dialer{Timeout: time.Second}).DialContext(ctx, network, address)
		if err != nil {
			return nil, err
		}
		return &timeoutReplyConn{Conn: conn, stalled: &stalled}, nil
	}
	client := redis.NewClient(&opts)
	defer client.Close()
	l, err := remote.New(client, cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := l.Ready(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Evaluate(ctx, "warm"); err != nil {
		t.Fatal(err)
	}
	// Withhold this client's next real Redis reply. The socket must obey the
	// limiter deadline without disturbing other tests sharing the Redis server.
	stalled.Store(true)
	started := time.Now()
	d, err := l.Evaluate(ctx, "stalled")
	if err == nil || d.Allowed {
		t.Fatalf("deadline did not fail closed %+v %v", d, err)
	}
	var timeout net.Error
	if !errors.Is(err, context.DeadlineExceeded) && !(errors.As(err, &timeout) && timeout.Timeout()) {
		t.Fatalf("expected timeout, got %v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("timeout took %s", elapsed)
	}
	if err := l.Ready(ctx); err != nil {
		t.Fatalf("readiness did not recover: %v", err)
	}
}

type timeoutReplyConn struct {
	net.Conn
	stalled *atomic.Bool
}

func (c *timeoutReplyConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 && c.stalled.Swap(false) {
		return c.Conn.Read(p)
	}
	return n, err
}

func TestLostReplyDoesNotReplayAdmission(t *testing.T) {
	backendClient, cfg := backend(t)
	cfg.Capacity = 3
	cfg.Limit = 1
	cfg.Window = time.Hour
	var armed atomic.Bool
	var writes atomic.Int32
	opts := *backendClient.Options()
	opts.MaxRetries = -1
	opts.Dialer = func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := (&net.Dialer{Timeout: time.Second}).DialContext(ctx, network, address)
		if err != nil {
			return nil, err
		}
		return &lostReplyConn{Conn: conn, armed: &armed, writes: &writes}, nil
	}
	client := redis.NewClient(&opts)
	defer client.Close()
	l, err := remote.New(client, cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if d, err := l.Evaluate(ctx, "u"); err != nil || !d.Allowed || d.Remaining != 2 {
		t.Fatalf("warmup %+v %v", d, err)
	}
	armed.Store(true)
	if d, err := l.Evaluate(ctx, "u"); err == nil || d.Allowed {
		t.Fatalf("lost reply reported success %+v %v", d, err)
	}
	armed.Store(false)
	if writes.Load() != 1 {
		t.Fatalf("ambiguous write sent %d times", writes.Load())
	}
	if d, err := l.Evaluate(ctx, "u"); err != nil || !d.Allowed || d.Remaining != 0 {
		t.Fatalf("lost or replayed quota %+v %v", d, err)
	}
	if d, err := l.Evaluate(ctx, "u"); err != nil || d.Reason != ratelimit.ReasonRateLimited {
		t.Fatalf("quota after lost reply %+v %v", d, err)
	}
}

type lostReplyConn struct {
	net.Conn
	armed  *atomic.Bool
	writes *atomic.Int32
	drop   atomic.Bool
}

func (c *lostReplyConn) Write(p []byte) (int, error) {
	if c.armed.Load() && bytes.Contains(bytes.ToLower(p), []byte("evalsha")) {
		c.writes.Add(1)
		c.drop.Store(true)
	}
	return c.Conn.Write(p)
}
func (c *lostReplyConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 && c.drop.Swap(false) {
		_ = c.Conn.Close()
		return 0, io.EOF
	}
	return n, err
}
