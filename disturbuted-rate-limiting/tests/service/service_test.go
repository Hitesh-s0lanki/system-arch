package service_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	ratelimit "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/fixedwindow"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/identity"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/policy"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/service"
)

func limiter(t *testing.T) *fixedwindow.Limiter {
	t.Helper()
	now := time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC)
	l, err := fixedwindow.New(fixedwindow.Config{Limit: 1, Window: 10 * time.Second, MaxKeys: 100},
		fixedwindow.WithClock(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestIdentityAndPolicyIsolation(t *testing.T) {
	l := limiter(t)
	bindings := []service.Binding{
		{Policy: policy.Policy{ID: "users", Identity: identity.User}, Limiter: l},
		{Policy: policy.Policy{ID: "other-users", Identity: identity.User}, Limiter: l},
		{Policy: policy.Policy{ID: "ips", Identity: identity.IP}, Limiter: l},
		{Policy: policy.Policy{ID: "credentials", Identity: identity.APIKey}, Limiter: l},
		{Policy: policy.Policy{ID: "routes", Identity: identity.User, PerRoute: true}, Limiter: l},
	}
	s, err := service.New(bindings)
	if err != nil {
		t.Fatal(err)
	}
	// Mutating the caller's registry must not alter the running service.
	bindings[0].Policy.ID = "changed"
	tests := []struct {
		name, policy string
		request      service.Request
		allowed      bool
	}{
		{"first user", "users", service.Request{TenantID: "a", UserID: "42"}, true},
		{"same user", "users", service.Request{TenantID: "a", UserID: "42"}, false},
		{"another user", "users", service.Request{TenantID: "a", UserID: "43"}, true},
		{"another tenant", "users", service.Request{TenantID: "b", UserID: "42"}, true},
		{"another policy", "other-users", service.Request{TenantID: "a", UserID: "42"}, true},
		{"IP", "ips", service.Request{TenantID: "a", IP: "192.0.2.1"}, true},
		{"equivalent IP", "ips", service.Request{TenantID: "a", IP: "::ffff:192.0.2.1"}, false},
		{"credential", "credentials", service.Request{TenantID: "a", APIKeyID: "credential-1"}, true},
		{"same credential", "credentials", service.Request{TenantID: "a", APIKeyID: "credential-1"}, false},
		{"route one", "routes", service.Request{TenantID: "a", UserID: "42", Route: "GET /orders"}, true},
		{"route two", "routes", service.Request{TenantID: "a", UserID: "42", Route: "POST /orders"}, true},
		{"same route", "routes", service.Request{TenantID: "a", UserID: "42", Route: "GET /orders"}, false},
		{"route ignored by global policy", "users", service.Request{TenantID: "a", UserID: "42", Route: "GET /other"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.Check(context.Background(), tt.policy, tt.request)
			if err != nil {
				t.Fatal(err)
			}
			if got.Allowed != tt.allowed {
				t.Fatalf("got %+v, want allowed=%v", got, tt.allowed)
			}
			if !tt.allowed && got.Reason != ratelimit.ReasonRateLimited {
				t.Fatalf("unexpected rejection: %+v", got)
			}
		})
	}
}

type failingLimiter struct {
	calls int
	err   error
}

func (l *failingLimiter) Evaluate(context.Context, string) (ratelimit.Decision, error) {
	l.calls++
	return ratelimit.Decision{}, l.err
}

func TestCheckErrors(t *testing.T) {
	backendError := errors.New("backend unavailable")
	l := &failingLimiter{err: backendError}
	s, err := service.New([]service.Binding{{Policy: policy.Policy{ID: "p", Identity: identity.User, PerRoute: true}, Limiter: l}})
	if err != nil {
		t.Fatal(err)
	}
	valid := service.Request{TenantID: "a", UserID: "42", Route: "GET /orders"}
	if _, err := s.Check(context.Background(), "unknown", valid); !errors.Is(err, service.ErrUnknownPolicy) {
		t.Fatalf("got %v", err)
	}
	for _, request := range []service.Request{
		{UserID: "42", Route: "GET /orders"},
		{TenantID: "a", IP: "192.0.2.1", Route: "GET /orders"}, // No fallback to IP.
		{TenantID: "a", UserID: "42"},
	} {
		if _, err := s.Check(context.Background(), "p", request); !errors.Is(err, service.ErrInvalidRequest) {
			t.Fatalf("got %v", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Check(ctx, "p", valid); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
	if l.calls != 0 {
		t.Fatalf("invalid requests reached backend %d times", l.calls)
	}
	if _, err := s.Check(context.Background(), "p", valid); !errors.Is(err, backendError) {
		t.Fatalf("backend error lost: %v", err)
	}
}

func TestNewRejectsInvalidBindings(t *testing.T) {
	l := limiter(t)
	var nilLimiter *fixedwindow.Limiter
	valid := service.Binding{Policy: policy.Policy{ID: "p", Identity: identity.User}, Limiter: l}
	for _, bindings := range [][]service.Binding{
		nil,
		{{Policy: valid.Policy}},
		{{Policy: valid.Policy, Limiter: nilLimiter}},
		{{Limiter: l}},
		{valid, valid},
	} {
		if _, err := service.New(bindings); err == nil {
			t.Fatal("expected invalid bindings")
		}
	}
}

func TestEvaluateCanceledDoesNotConsumeAllowance(t *testing.T) {
	l := limiter(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := l.Evaluate(ctx, "user"); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
	got, err := l.Evaluate(context.Background(), "user")
	if err != nil || !got.Allowed {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestCheckConcurrent(t *testing.T) {
	s, err := service.New([]service.Binding{{
		Policy: policy.Policy{ID: "users", Identity: identity.User}, Limiter: limiter(t),
	}})
	if err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	results := make(chan bool, 100)
	for i := 0; i < 100; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			decision, err := s.Check(context.Background(), "users", service.Request{TenantID: "a", UserID: "42"})
			if err != nil {
				t.Error(err)
			}
			results <- decision.Allowed
		}()
	}
	workers.Wait()
	close(results)
	allowed := 0
	for result := range results {
		if result {
			allowed++
		}
	}
	if allowed != 1 {
		t.Fatalf("allowed %d requests, want 1", allowed)
	}
}

func TestCheckPreservesCapacityRejection(t *testing.T) {
	l, err := fixedwindow.New(fixedwindow.Config{Limit: 1, Window: time.Hour, MaxKeys: 1})
	if err != nil {
		t.Fatal(err)
	}
	s, err := service.New([]service.Binding{{Policy: policy.Policy{ID: "users", Identity: identity.User}, Limiter: l}})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := s.Check(context.Background(), "users", service.Request{TenantID: "a", UserID: "42"}); err != nil || !got.Allowed {
		t.Fatalf("first user: got %+v, %v", got, err)
	}
	got, err := s.Check(context.Background(), "users", service.Request{TenantID: "a", UserID: "43"})
	if err != nil || got != (ratelimit.Decision{Reason: ratelimit.ReasonCapacityExceeded}) {
		t.Fatalf("capacity: got %+v, %v", got, err)
	}
}
