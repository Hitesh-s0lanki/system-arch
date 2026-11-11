package sdk_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	ratelimit "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/slidingwindow"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/sdk"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/service"
	redis "github.com/redis/go-redis/v9"
)

func TestSlidingWindowSDKBothBackends(t *testing.T) {
	for _, mode := range []string{"ram", "redis"} {
		t.Run(mode, func(t *testing.T) {
			p := policies()[0]
			p.Policy.PerRoute = true
			p.SlidingWindow = slidingwindow.Config{Limit: 2, Window: time.Hour, MaxKeys: 4}
			var c *sdk.Client
			var err error
			if mode == "ram" {
				c, err = sdk.NewSlidingWindowMemory([]sdk.Policy{p})
			} else {
				url := os.Getenv("REDIS_TEST_URL")
				if url == "" {
					t.Skip("set REDIS_TEST_URL for sliding-window SDK integration")
				}
				opts, e := redis.ParseURL(url)
				if e != nil {
					t.Fatal(e)
				}
				opts.MaxRetries = -1
				opts.ContextTimeoutEnabled = true
				backend := redis.NewClient(opts)
				namespace := fmt.Sprintf("sdk-sl:%d", time.Now().UnixNano())
				t.Cleanup(func() {
					keys, _ := backend.Keys(context.Background(), namespace+":*").Result()
					if len(keys) > 0 {
						backend.Del(context.Background(), keys...)
					}
					backend.Close()
				})
				c, err = sdk.NewSlidingWindowRedis(backend, sdk.RedisConfig{Policies: []sdk.Policy{p}, Namespace: namespace, Timeout: time.Second})
			}
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			if err := c.Ready(ctx); err != nil {
				t.Fatal(err)
			}
			request := service.Request{TenantID: "tenant", UserID: "user", Route: "GET /orders"}
			for i := 0; i < 3; i++ {
				d, err := c.Check(ctx, p.Policy.ID, request)
				if err != nil {
					t.Fatal(err)
				}
				if i < 2 {
					want := ratelimit.Decision{Allowed: true, Remaining: 1 - i}
					if d != want {
						t.Fatalf("request %d got %+v want %+v", i, d, want)
					}
				} else if d.Allowed || d.Reason != ratelimit.ReasonRateLimited || d.Remaining != 0 || d.RetryAfter <= 0 || d.RetryAfter > time.Hour {
					t.Fatalf("invalid rejection %+v", d)
				}
			}
			request.Route = "POST /orders"
			if d, err := c.Check(ctx, p.Policy.ID, request); err != nil || d != (ratelimit.Decision{Allowed: true, Remaining: 1}) {
				t.Fatalf("route isolation %+v %v", d, err)
			}
			request.TenantID = "other"
			if d, err := c.Check(ctx, p.Policy.ID, request); err != nil || !d.Allowed {
				t.Fatalf("tenant isolation %+v %v", d, err)
			}
			request.UserID = "other-user"
			if d, err := c.Check(ctx, p.Policy.ID, request); err != nil || d != (ratelimit.Decision{Allowed: true, Remaining: 1}) {
				t.Fatalf("subject isolation %+v %v", d, err)
			}
			request.UserID = "untracked"
			if d, err := c.Check(ctx, p.Policy.ID, request); err != nil || d != (ratelimit.Decision{Reason: ratelimit.ReasonCapacityExceeded}) {
				t.Fatalf("capacity %+v %v", d, err)
			}
			canceled, cancel := context.WithCancel(ctx)
			cancel()
			if _, err := c.Check(canceled, p.Policy.ID, request); !errors.Is(err, context.Canceled) {
				t.Fatalf("canceled SDK check %v", err)
			}
			if err := c.Ready(canceled); !errors.Is(err, context.Canceled) {
				t.Fatalf("canceled SDK ready %v", err)
			}
			if n := c.CleanupExpired(); n != 0 {
				t.Fatalf("active state cleaned: %d", n)
			}
		})
	}
}
func TestSlidingWindowSDKCleanupAndValidation(t *testing.T) {
	p := policies()[0]
	p.SlidingWindow = slidingwindow.Config{Limit: 1, Window: time.Second, MaxKeys: 1}
	now := time.Now()
	c, err := sdk.NewSlidingWindowMemory([]sdk.Policy{p}, slidingwindow.WithClock(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Check(context.Background(), p.Policy.ID, service.Request{TenantID: "t", UserID: "u"}); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Second)
	if c.CleanupExpired() != 1 {
		t.Fatal("SDK did not clean expired logs")
	}
	for _, invalid := range [][]sdk.Policy{nil, {p, p}, {{Policy: p.Policy}}} {
		if _, err := sdk.NewSlidingWindowMemory(invalid); err == nil {
			t.Fatal("accepted invalid policies")
		}
	}
}
