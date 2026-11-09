package sdk_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	ratelimit "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/fixedwindow"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/identity"
	httpmiddleware "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/middleware/http"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/policy"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/sdk"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/service"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/tests/testredis"
	redis "github.com/redis/go-redis/v9"
)

func policies() []sdk.Policy {
	return []sdk.Policy{{
		Policy:      policy.Policy{ID: "users", Identity: identity.User},
		FixedWindow: fixedwindow.Config{Limit: 3, Window: time.Hour, MaxKeys: 2},
	}}
}

func TestMemoryConcurrencyCapacityAndRollover(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	configured := policies()
	c, err := sdk.NewMemory(configured, fixedwindow.WithClock(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	// Changing caller-owned configuration cannot change a running client's policy.
	configured[0].Policy.Identity = identity.IP
	configured[0].FixedWindow.Limit = 100
	request := service.Request{TenantID: "tenant", UserID: "user"}
	var allowed atomic.Int32
	var workers sync.WaitGroup
	for range 50 {
		workers.Go(func() {
			d, err := c.Check(context.Background(), "users", request)
			if err != nil {
				t.Error(err)
			} else if d.Allowed {
				allowed.Add(1)
			} else if d.Reason != ratelimit.ReasonRateLimited || d.RetryAfter != time.Hour {
				t.Errorf("unexpected rejection: %+v", d)
			}
		})
	}
	workers.Wait()
	if allowed.Load() != 3 {
		t.Fatalf("allowed %d, want 3", allowed.Load())
	}
	request.TenantID = "other-tenant"
	if d, err := c.Check(context.Background(), "users", request); err != nil || !d.Allowed {
		t.Fatalf("tenant isolation: %+v %v", d, err)
	}
	request.UserID = "other-user"
	if d, err := c.Check(context.Background(), "users", request); err != nil || d.Reason != ratelimit.ReasonCapacityExceeded {
		t.Fatalf("capacity: %+v %v", d, err)
	}
	now = now.Add(time.Hour)
	if removed := c.CleanupExpired(); removed != 2 {
		t.Fatalf("removed %d, want 2", removed)
	}
	if d, err := c.Check(context.Background(), "users", request); err != nil || !d.Allowed {
		t.Fatalf("rollover: %+v %v", d, err)
	}
}

func TestValidationAndCancellationDoNotConsumeQuota(t *testing.T) {
	c, err := sdk.NewMemory(policies())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request := service.Request{TenantID: "tenant", UserID: "user"}
	if _, err := c.Check(ctx, "users", request); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	if err := c.Ready(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("readiness cancellation: %v", err)
	}
	if _, err := c.Check(context.Background(), "missing", request); !errors.Is(err, service.ErrUnknownPolicy) {
		t.Fatalf("unknown policy: %v", err)
	}
	if _, err := c.Check(context.Background(), "users", service.Request{TenantID: "tenant"}); !errors.Is(err, service.ErrInvalidRequest) {
		t.Fatalf("invalid identity: %v", err)
	}
	if d, err := c.Check(context.Background(), "users", request); err != nil || !d.Allowed || d.Remaining != 2 {
		t.Fatalf("validation consumed allowance: %+v %v", d, err)
	}
}

func TestConstructorsRejectInvalidConfiguration(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1})
	defer client.Close()
	for _, invalid := range [][]sdk.Policy{
		nil,
		{policies()[0], policies()[0]},
		{{Policy: policy.Policy{ID: "users", Identity: identity.User}}},
		{{Policy: policy.Policy{ID: "users", Identity: "unsupported"}, FixedWindow: policies()[0].FixedWindow}},
	} {
		if _, err := sdk.NewMemory(invalid); err == nil {
			t.Fatal("memory accepted invalid configuration")
		}
		if _, err := sdk.NewRedis(client, sdk.RedisConfig{Policies: invalid, Namespace: "sdk-test", Timeout: time.Second}); err == nil {
			t.Fatal("Redis accepted invalid configuration")
		}
	}
	if _, err := sdk.NewRedis(nil, sdk.RedisConfig{Policies: policies(), Namespace: "sdk-test", Timeout: time.Second}); err == nil {
		t.Fatal("nil Redis accepted")
	}
}

func TestHTTPMiddlewareIntegration(t *testing.T) {
	p := policies()[0]
	p.Policy.Identity = identity.IP
	p.FixedWindow.Limit = 1
	c, err := sdk.NewMemory([]sdk.Policy{p})
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	handler, err := httpmiddleware.Wrap(c, httpmiddleware.Config{Policy: p.Policy, AnonymousTenant: "public"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusNoContent)
	}))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []int{http.StatusNoContent, http.StatusTooManyRequests} {
		r := httptest.NewRequest(http.MethodGet, "/orders", nil)
		r.RemoteAddr = "192.0.2.1:1234"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("status %d, want %d", w.Code, want)
		}
		if want == http.StatusTooManyRequests && w.Header().Get("Retry-After") == "" {
			t.Fatal("missing Retry-After")
		}
	}
	if calls != 1 {
		t.Fatalf("handler called %d times", calls)
	}
}

func TestRedisSharedQuotaReadinessAndCompatibility(t *testing.T) {
	backend := testredis.Start(t, testredis.Address(t), "", false)
	config := sdk.RedisConfig{Policies: policies(), Namespace: "sdk-test", Revision: "v1", Timeout: time.Second}
	first, err := sdk.NewRedis(backend, config)
	if err != nil {
		t.Fatal(err)
	}
	// A separate pool represents an independently embedded application replica.
	otherBackend := redis.NewClient(&redis.Options{Addr: backend.Options().Addr, MaxRetries: -1, ContextTimeoutEnabled: true})
	defer otherBackend.Close()
	second, err := sdk.NewRedis(otherBackend, config)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []*sdk.Client{first, second} {
		if err := c.Ready(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	request := service.Request{TenantID: "tenant", UserID: "user"}
	for i, c := range []*sdk.Client{first, second, first, second} {
		d, err := c.Check(context.Background(), "users", request)
		if err != nil || d.Allowed != (i < 3) || (i < 3 && d.Remaining != 2-i) || (i == 3 && d.Reason != ratelimit.ReasonRateLimited) {
			t.Fatalf("request %d: %+v %v", i, d, err)
		}
	}
	config.Policies[0].Policy.PerRoute = true
	incompatible, err := sdk.NewRedis(backend, config)
	if err != nil {
		t.Fatal(err)
	}
	if err := incompatible.Ready(context.Background()); err == nil {
		t.Fatal("incompatible grouping accepted")
	}
	if _, err := incompatible.Check(context.Background(), "users", service.Request{TenantID: "tenant", UserID: "user", Route: "GET /orders"}); err == nil {
		t.Fatal("incompatible grouping consumed quota")
	}
	if err := otherBackend.Close(); err != nil {
		t.Fatal(err)
	}
	if d, err := second.Check(context.Background(), "users", request); err == nil || d.Allowed {
		t.Fatalf("backend failure did not propagate: %+v %v", d, err)
	}
}
