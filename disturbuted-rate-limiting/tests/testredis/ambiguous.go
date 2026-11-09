package testredis

import (
	"bytes"
	"context"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	rediswindow "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/fixedwindow/redis"
	redis "github.com/redis/go-redis/v9"
)

// CheckAmbiguousClusterReply drops a real script reply after execution. Cluster
// must return the error without replaying the increment on another connection.
func CheckAmbiguousClusterReply(t *testing.T, addresses []string) {
	t.Helper()
	var armed atomic.Bool
	var writes atomic.Int64
	client := redis.NewClusterClient(&redis.ClusterOptions{Addrs: addresses, MaxRetries: -1, ContextTimeoutEnabled: true, Dialer: func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := (&net.Dialer{Timeout: time.Second}).DialContext(ctx, network, address)
		if err != nil {
			return nil, err
		}
		return &lostReplyConn{Conn: conn, armed: &armed, writes: &writes}, nil
	}})
	defer client.Close()
	limiter, err := rediswindow.New(client, rediswindow.Config{PolicyID: "ambiguous", Namespace: "ambiguous-test", Limit: 10, Window: time.Hour, MaxKeys: 10, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if decision, err := limiter.Evaluate(context.Background(), "subject"); err != nil || !decision.Allowed {
		t.Fatalf("warmup: %+v %v", decision, err)
	}
	armed.Store(true)
	if _, err := limiter.Evaluate(context.Background(), "subject"); err == nil {
		t.Fatal("ambiguous write unexpectedly succeeded")
	}
	if writes.Load() != 1 {
		t.Fatalf("ambiguous quota command replayed %d times", writes.Load())
	}
	armed.Store(false)
	// A fresh decision must see exactly two consumed requests: warmup + lost reply.
	decision, err := limiter.Evaluate(context.Background(), "subject")
	if err != nil || !decision.Allowed || decision.Remaining != 7 {
		t.Fatalf("ambiguous command was replayed or lost: %+v %v", decision, err)
	}
}

type lostReplyConn struct {
	net.Conn
	armed  *atomic.Bool
	writes *atomic.Int64
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
