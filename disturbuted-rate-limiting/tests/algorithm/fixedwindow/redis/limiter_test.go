package redis_test

import (
	"context"
	"strings"
	"testing"
	"time"

	ratelimit "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting"
	rediswindow "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/fixedwindow/redis"
	redisclient "github.com/redis/go-redis/v9"
)

func TestValidationWithoutRedis(t *testing.T) {
	client := redisclient.NewClient(&redisclient.Options{Addr: "127.0.0.1:1", MaxRetries: -1})
	defer client.Close()
	valid := rediswindow.Config{PolicyID: "p", Namespace: "test", Limit: 1, MaxKeys: 1, Window: time.Second, Timeout: time.Second}
	for _, mutate := range []func(*rediswindow.Config){
		func(c *rediswindow.Config) { c.Limit = 0 }, func(c *rediswindow.Config) { c.MaxKeys = 0 },
		func(c *rediswindow.Config) { c.Window = time.Nanosecond }, func(c *rediswindow.Config) { c.Window = 1500 * time.Microsecond },
		func(c *rediswindow.Config) { c.Timeout = 0 }, func(c *rediswindow.Config) { c.PolicyID = "" },
		func(c *rediswindow.Config) { c.Namespace = " " },
	} {
		cfg := valid
		mutate(&cfg)
		if _, err := rediswindow.New(client, cfg); err == nil {
			t.Fatal("invalid configuration accepted")
		}
	}
	if _, err := rediswindow.New(nil, valid); err == nil {
		t.Fatal("nil client accepted")
	}
	retrying := redisclient.NewClient(&redisclient.Options{Addr: "127.0.0.1:1"})
	defer retrying.Close()
	if _, err := rediswindow.New(retrying, valid); err == nil {
		t.Fatal("automatic retries accepted")
	}
	l, err := rediswindow.New(client, valid)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"", " \t", strings.Repeat("x", 257)} {
		decision, err := l.Evaluate(context.Background(), key)
		if err != nil || decision.Reason != ratelimit.ReasonInvalidKey {
			t.Fatalf("invalid key reached Redis: %+v %v", decision, err)
		}
	}
}
