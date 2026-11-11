package redis_test

import (
	"bytes"
	"context"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	remote "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/tokenbucket/redis"
	redis "github.com/redis/go-redis/v9"
)

type lostReplyConn struct {
	net.Conn
	armed  *atomic.Bool
	writes *atomic.Int32
	drop   bool
}

func (c *lostReplyConn) Write(p []byte) (int, error) {
	if c.armed.Load() && bytes.Contains(bytes.ToLower(p), []byte("evalsha")) {
		c.writes.Add(1)
		c.drop = true
	}
	return c.Conn.Write(p)
}
func (c *lostReplyConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 && c.drop {
		c.drop = false
		c.Conn.Close()
		return 0, io.EOF
	}
	return n, err
}
func TestLostReplyDoesNotReplayTokenConsumption(t *testing.T) {
	client, cfg := backend(t)
	cfg.Capacity = 10
	cfg.Limit = 1
	cfg.Window = time.Hour
	var armed atomic.Bool
	var writes atomic.Int32
	opts := *client.Options()
	opts.MaxRetries = -1
	opts.PoolSize = 1
	opts.MaxActiveConns = 1
	opts.Dialer = func(ctx context.Context, network, address string) (net.Conn, error) {
		c, err := (&net.Dialer{Timeout: time.Second}).DialContext(ctx, network, address)
		if err != nil {
			return nil, err
		}
		return &lostReplyConn{Conn: c, armed: &armed, writes: &writes}, nil
	}
	wrapped := redis.NewClient(&opts)
	defer wrapped.Close()
	l, err := remote.New(wrapped, cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if d, err := l.Evaluate(ctx, "u"); err != nil || !d.Allowed {
		t.Fatalf("warmup %+v %v", d, err)
	}
	armed.Store(true)
	if d, err := l.Evaluate(ctx, "u"); err == nil || d.Allowed {
		t.Fatalf("lost reply %+v %v", d, err)
	}
	armed.Store(false)
	if writes.Load() != 1 {
		t.Fatalf("command replayed %d times", writes.Load())
	}
	if d, err := l.Evaluate(ctx, "u"); err != nil || !d.Allowed || d.Remaining != 7 {
		t.Fatalf("quota after lost reply %+v %v", d, err)
	}
}
func TestConfigurationSignatureIncludesBurstAndSemantics(t *testing.T) {
	client, cfg := backend(t)
	l, err := remote.New(client, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = l.Ready(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*remote.Config){func(c *remote.Config) { c.Capacity++ }, func(c *remote.Config) { c.MaxKeys++ }, func(c *remote.Config) { c.Window += time.Millisecond }, func(c *remote.Config) { c.Semantics = "different" }} {
		other := cfg
		mutate(&other)
		l, err := remote.New(client, other)
		if err != nil {
			t.Fatal(err)
		}
		if l.Ready(context.Background()) == nil {
			t.Fatalf("accepted incompatible %+v", other)
		}
		if _, err = l.Evaluate(context.Background(), "u"); err == nil {
			t.Fatal("incompatible decision succeeded")
		}
	}
	cfg.Revision = "fresh"
	fresh, err := remote.New(client, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = fresh.Ready(context.Background()); err != nil {
		t.Fatal(err)
	}
}
