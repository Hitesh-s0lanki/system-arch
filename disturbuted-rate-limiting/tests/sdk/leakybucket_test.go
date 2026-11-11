package sdk_test

import (
	"context"
	"fmt"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/leakybucket"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/sdk"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/service"
	redis "github.com/redis/go-redis/v9"
	"os"
	"testing"
	"time"
)

func TestLeakyBucketSDKBothBackends(t *testing.T) {
	for _, mode := range []string{"ram", "redis"} {
		t.Run(mode, func(t *testing.T) {
			p := policies()[0]
			p.LeakyBucket = leakybucket.Config{Capacity: 2, Limit: 2, Window: time.Hour, MaxKeys: 2}
			var c *sdk.Client
			var err error
			if mode == "ram" {
				c, err = sdk.NewLeakyBucketMemory([]sdk.Policy{p})
			} else {
				url := os.Getenv("REDIS_TEST_URL")
				if url == "" {
					t.Skip("set REDIS_TEST_URL for leaky-bucket SDK integration")
				}
				opts, e := redis.ParseURL(url)
				if e != nil {
					t.Fatal(e)
				}
				opts.MaxRetries = -1
				opts.ContextTimeoutEnabled = true
				backend := redis.NewClient(opts)
				namespace := fmt.Sprintf("sdk-lb:%d", time.Now().UnixNano())
				t.Cleanup(func() {
					keys, _ := backend.Keys(context.Background(), namespace+":*").Result()
					if len(keys) > 0 {
						backend.Del(context.Background(), keys...)
					}
					backend.Close()
				})
				c, err = sdk.NewLeakyBucketRedis(backend, sdk.RedisConfig{Policies: []sdk.Policy{p}, Namespace: namespace, Timeout: time.Second})
			}
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			if err := c.Ready(ctx); err != nil {
				t.Fatal(err)
			}
			request := service.Request{TenantID: "tenant", UserID: "user"}
			for i := 0; i < 3; i++ {
				d, err := c.Check(ctx, p.Policy.ID, request)
				if err != nil || d.Allowed != (i < 2) {
					t.Fatalf("request %d: %+v %v", i, d, err)
				}
			}
			request.TenantID = "other"
			if d, err := c.Check(ctx, p.Policy.ID, request); err != nil || !d.Allowed {
				t.Fatalf("tenant isolation %+v %v", d, err)
			}
			if n := c.CleanupExpired(); n != 0 {
				t.Fatalf("active state cleaned: %d", n)
			}
		})
	}
}
func TestLeakyBucketSDKCleanupAndValidation(t *testing.T) {
	p := policies()[0]
	p.LeakyBucket = leakybucket.Config{Capacity: 1, Limit: 1, Window: time.Second, MaxKeys: 1}
	now := time.Now()
	c, err := sdk.NewLeakyBucketMemory([]sdk.Policy{p}, leakybucket.WithClock(func() time.Time { return now }))
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
		if _, err := sdk.NewLeakyBucketMemory(invalid); err == nil {
			t.Fatal("accepted invalid policies")
		}
	}
}
