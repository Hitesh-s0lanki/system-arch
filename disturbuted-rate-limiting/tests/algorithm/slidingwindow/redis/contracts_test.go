package redis_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	ratelimit "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting"
	remote "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/slidingwindow/redis"
	redis "github.com/redis/go-redis/v9"
)

func newRemote(t *testing.T, client redis.UniversalClient, cfg remote.Config) *remote.Limiter {
	t.Helper()
	l, err := remote.New(client, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestScopeIsolationAndPersistentCompatibility(t *testing.T) {
	client, cfg := backend(t)
	cfg.Limit = 1
	cfg.MaxKeys = 1
	ctx := context.Background()
	original := newRemote(t, client, cfg)
	if d, err := original.Evaluate(ctx, "u"); err != nil || d != (ratelimit.Decision{Allowed: true}) {
		t.Fatalf("initial %+v %v", d, err)
	}
	for _, test := range []struct {
		name   string
		change func(*remote.Config)
	}{
		{"policy", func(c *remote.Config) { c.PolicyID = "other" }},
		{"revision", func(c *remote.Config) { c.Revision = "v2" }},
		{"tenant", func(c *remote.Config) { c.TenantID = "other" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			scoped := cfg
			test.change(&scoped)
			l := newRemote(t, client, scoped)
			if err := l.Ready(ctx); err != nil {
				t.Fatal(err)
			}
			if d, err := l.Evaluate(ctx, "u"); err != nil || d != (ratelimit.Decision{Allowed: true}) {
				t.Fatalf("independent scope %+v %v", d, err)
			}
			if d, err := l.Evaluate(ctx, "v"); err != nil || d != (ratelimit.Decision{Reason: ratelimit.ReasonCapacityExceeded}) {
				t.Fatalf("independent capacity %+v %v", d, err)
			}
		})
	}
	// Each signature component must reject a mismatched replica, including when
	// quota state is absent. Delete only quota keys to model idle TTL expiration.
	keys, err := client.Keys(ctx, cfg.Namespace+":sl:*:config").Result()
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range keys {
		base := key[:len(key)-len(":config")]
		if err := client.Del(ctx, base, base+":expiry").Err(); err != nil {
			t.Fatal(err)
		}
	}
	for _, test := range []struct {
		name   string
		change func(*remote.Config)
	}{
		{"limit", func(c *remote.Config) { c.Limit++ }},
		{"window", func(c *remote.Config) { c.Window += time.Millisecond }},
		{"capacity", func(c *remote.Config) { c.MaxKeys++ }},
		{"identity_semantics", func(c *remote.Config) { c.Semantics = "different grouping" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			changed := cfg
			test.change(&changed)
			l := newRemote(t, client, changed)
			if err := l.Ready(ctx); err == nil {
				t.Fatal("readiness accepted incompatible policy after quota expiry")
			}
			if d, err := l.Evaluate(ctx, "u"); err == nil || d.Allowed {
				t.Fatalf("incompatible policy accepted %+v %v", d, err)
			}
			if _, err := l.Usage(ctx); err == nil {
				t.Fatal("usage accepted incompatible policy")
			}
		})
	}
	if d, err := original.Evaluate(ctx, "u"); err != nil || !d.Allowed {
		t.Fatalf("original policy guard was changed %+v %v", d, err)
	}
}

func TestInvalidAndCanceledChecksDoNotTouchRedis(t *testing.T) {
	client, cfg := backend(t)
	l := newRemote(t, client, cfg)
	ctx := context.Background()
	for _, key := range []string{"", " \n", "\t"} {
		if d, err := l.Evaluate(ctx, key); err != nil || d != (ratelimit.Decision{Reason: ratelimit.ReasonInvalidKey}) {
			t.Fatalf("invalid %+v %v", d, err)
		}
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := l.Evaluate(canceled, "u"); !errors.Is(err, context.Canceled) {
		t.Fatalf("evaluate %v", err)
	}
	if err := l.Ready(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("ready %v", err)
	}
	if _, err := l.Usage(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("usage %v", err)
	}
	if keys, err := client.Keys(ctx, cfg.Namespace+":sl:*").Result(); err != nil || len(keys) != 0 {
		t.Fatalf("invalid/canceled calls wrote state %v %v", keys, err)
	}
}

// Drop the reply only after the real Redis response has arrived. The command
// executed, but its caller cannot tell whether it did; retrying would double-charge.
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
		c.Conn.Close()
		return 0, io.EOF
	}
	return n, err
}

func TestLostReplyDoesNotRetryOrDoubleCharge(t *testing.T) {
	client, cfg := backend(t)
	cfg.Limit = 10
	cfg.Window = time.Hour
	options := *client.Options()
	options.MaxRetries = -1
	options.PoolSize = 1
	options.MaxActiveConns = 1
	dial := options.Dialer
	var armed atomic.Bool
	var writes atomic.Int32
	options.Dialer = func(ctx context.Context, network, address string) (net.Conn, error) {
		c, err := dial(ctx, network, address)
		if err != nil {
			return nil, err
		}
		return &lostReplyConn{Conn: c, armed: &armed, writes: &writes}, nil
	}
	intercepted := redis.NewClient(&options)
	defer intercepted.Close()
	l := newRemote(t, intercepted, cfg)
	ctx := context.Background()
	if d, err := l.Evaluate(ctx, "u"); err != nil || d != (ratelimit.Decision{Allowed: true, Remaining: 9}) {
		t.Fatalf("warmup %+v %v", d, err)
	}
	armed.Store(true)
	if d, err := l.Evaluate(ctx, "u"); err == nil || d.Allowed {
		t.Fatalf("lost reply incorrectly reported success %+v %v", d, err)
	}
	if writes.Load() != 1 {
		t.Fatalf("ambiguous mutation executed %d times", writes.Load())
	}
	armed.Store(false)
	if d, err := l.Evaluate(ctx, "u"); err != nil || d != (ratelimit.Decision{Allowed: true, Remaining: 7}) {
		t.Fatalf("lost reply was replayed or refunded %+v %v", d, err)
	}
}

func TestBackendTimeoutIsBounded(t *testing.T) {
	// A private TCP server consumes requests and never replies. This avoids global
	// CLIENT PAUSE commands or other effects on a shared test Redis instance.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		io.Copy(io.Discard, conn)
	}()
	client := redis.NewClient(&redis.Options{Addr: listener.Addr().String(), MaxRetries: -1, ContextTimeoutEnabled: true, DialTimeout: time.Second, ReadTimeout: time.Second, WriteTimeout: time.Second})
	defer func() {
		client.Close()
		listener.Close()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("test backend did not stop")
		}
	}()
	l := newRemote(t, client, remote.Config{PolicyID: "p", Namespace: "timeout", Limit: 1, Window: time.Second, MaxKeys: 1, Timeout: 30 * time.Millisecond})
	started := time.Now()
	d, err := l.Evaluate(context.Background(), "u")
	if err == nil || d.Allowed {
		t.Fatalf("unresponsive backend allowed request %+v %v", d, err)
	}
	var networkError net.Error
	if !errors.Is(err, context.DeadlineExceeded) && !(errors.As(err, &networkError) && networkError.Timeout()) {
		t.Fatalf("expected deadline or I/O timeout, got %v", err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("configured timeout did not bound operation: %v", elapsed)
	}
}

func TestMillisecondBoundaryAndExactKeySemantics(t *testing.T) {
	client, cfg := backend(t)
	cfg.Window = time.Millisecond
	cfg.Limit = 1
	cfg.MaxKeys = 3
	l := newRemote(t, client, cfg)
	ctx := context.Background()
	if err := l.Ready(ctx); err != nil {
		t.Fatal(err)
	}
	serverNow, err := client.Time(ctx).Result()
	if err != nil {
		t.Fatal(err)
	}
	frozen := serverNow.Add(time.Hour).UnixMilli()
	key := stateKey(t, client, cfg)
	if err := client.HSet(ctx, key, "_last", frozen).Err(); err != nil {
		t.Fatal(err)
	}
	for _, subject := range []string{"u", " u ", strings.Repeat("é", 128)} {
		if d, err := l.Evaluate(ctx, subject); err != nil || d != (ratelimit.Decision{Allowed: true}) {
			t.Fatalf("key %q: %+v %v", subject, d, err)
		}
	}
	if d, err := l.Evaluate(ctx, "u"); err != nil || d != (ratelimit.Decision{Reason: ratelimit.ReasonRateLimited, RetryAfter: time.Millisecond}) {
		t.Fatalf("before boundary %+v %v", d, err)
	}
	if err := client.HSet(ctx, key, "_last", frozen+1).Err(); err != nil {
		t.Fatal(err)
	}
	if d, err := l.Evaluate(ctx, "u"); err != nil || d != (ratelimit.Decision{Allowed: true}) {
		t.Fatalf("exact boundary %+v %v", d, err)
	}
}
