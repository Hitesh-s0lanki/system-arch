package fixedwindow_test

import (
	"sync"
	"testing"
	"time"

	fixedwindow "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/fixedwindow"
)

func TestAllowConcurrent(t *testing.T) {
	const (
		limit    = 100
		requests = 1000
	)

	// Keep every request in the same window, regardless of test execution time.
	now := time.Date(2026, time.January, 1, 0, 0, 1, 0, time.UTC)
	limiter, err := fixedwindow.New(
		fixedwindow.Config{MaxKeys: 10000, Limit: limit, Window: 10 * time.Second},
		fixedwindow.WithClock(func() time.Time { return now }),
	)
	if err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	results := make(chan fixedwindow.Decision, requests)
	var workers sync.WaitGroup
	workers.Add(requests)

	for i := 0; i < requests; i++ {
		go func() {
			defer workers.Done()
			<-start
			limiter.CleanupExpired()
			results <- limiter.Allow("shared-user")
		}()
	}

	// Release the callers together to exercise contention on the same key.
	close(start)
	workers.Wait()
	close(results)

	allowed := 0
	seenRemaining := make(map[int]bool)
	for decision := range results {
		if !decision.Allowed {
			want := fixedwindow.Decision{Reason: fixedwindow.ReasonRateLimited, RetryAfter: 9 * time.Second}
			if decision != want {
				t.Errorf("rejected decision: got %+v, want %+v", decision, want)
			}
			continue
		}

		allowed++
		if decision.Remaining < 0 || decision.Remaining >= limit {
			t.Errorf("remaining allowance out of range: %d", decision.Remaining)
		}
		if seenRemaining[decision.Remaining] {
			t.Errorf("duplicate remaining allowance: %d", decision.Remaining)
		}
		seenRemaining[decision.Remaining] = true
		if decision.RetryAfter != 0 {
			t.Errorf("allowed request retry delay: got %v, want 0", decision.RetryAfter)
		}
	}

	if allowed != limit {
		t.Fatalf("allowed %d of %d requests, want %d", allowed, requests, limit)
	}

	want := fixedwindow.Decision{Reason: fixedwindow.ReasonRateLimited, RetryAfter: 9 * time.Second}
	if got := limiter.Allow("shared-user"); got != want {
		t.Fatalf("after concurrent requests: got %+v, want %+v", got, want)
	}
}

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		config  fixedwindow.Config
		wantErr bool
	}{
		{
			name:   "valid configuration",
			config: fixedwindow.Config{MaxKeys: 10000, Limit: 3, Window: 10 * time.Second},
		},
		{
			name:    "zero limit",
			config:  fixedwindow.Config{MaxKeys: 10000, Limit: 0, Window: time.Second},
			wantErr: true,
		},
		{
			name:    "negative limit",
			config:  fixedwindow.Config{MaxKeys: 10000, Limit: -1, Window: time.Second},
			wantErr: true,
		},
		{
			name:    "zero window",
			config:  fixedwindow.Config{MaxKeys: 10000, Limit: 3, Window: 0},
			wantErr: true,
		},
		{
			name:    "negative window",
			config:  fixedwindow.Config{MaxKeys: 10000, Limit: 3, Window: -time.Second},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limiter, err := fixedwindow.New(tt.config)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				if limiter != nil {
					t.Fatal("expected a nil limiter")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if limiter == nil {
				t.Fatal("expected a limiter")
			}
		})
	}
}

func TestNewRejectsInvalidOptions(t *testing.T) {
	tests := []struct {
		name   string
		option fixedwindow.Option
	}{
		{name: "nil option", option: nil},
		{name: "nil clock", option: fixedwindow.WithClock(nil)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limiter, err := fixedwindow.New(
				fixedwindow.Config{MaxKeys: 10000, Limit: 3, Window: 10 * time.Second},
				tt.option,
			)
			if err == nil {
				t.Fatal("expected an error")
			}
			if limiter != nil {
				t.Fatal("expected a nil limiter")
			}
		})
	}
}

func TestAllow(t *testing.T) {
	now := time.Date(2026, time.January, 1, 0, 0, 1, 0, time.UTC)
	limiter, err := fixedwindow.New(
		fixedwindow.Config{MaxKeys: 10000, Limit: 3, Window: 10 * time.Second},
		fixedwindow.WithClock(func() time.Time { return now }),
	)
	if err != nil {
		t.Fatal(err)
	}

	for request := 1; request <= 3; request++ {
		got := limiter.Allow("user-a")
		want := fixedwindow.Decision{Allowed: true, Remaining: 3 - request}
		if got != want {
			t.Fatalf("request %d: got %+v, want %+v", request, got, want)
		}
	}

	wantRejected := fixedwindow.Decision{Reason: fixedwindow.ReasonRateLimited, RetryAfter: 9 * time.Second}
	for request := 4; request <= 5; request++ {
		if got := limiter.Allow("user-a"); got != wantRejected {
			t.Fatalf("request %d: got %+v, want %+v", request, got, wantRejected)
		}
	}

	wantAllowed := fixedwindow.Decision{Allowed: true, Remaining: 2}
	if got := limiter.Allow("user-b"); got != wantAllowed {
		t.Fatalf("independent key: got %+v, want %+v", got, wantAllowed)
	}

	now = time.Date(2026, time.January, 1, 0, 0, 9, 999999999, time.UTC)
	wantBeforeBoundary := fixedwindow.Decision{Reason: fixedwindow.ReasonRateLimited, RetryAfter: time.Nanosecond}
	if got := limiter.Allow("user-a"); got != wantBeforeBoundary {
		t.Fatalf("before boundary: got %+v, want %+v", got, wantBeforeBoundary)
	}

	now = now.Add(time.Nanosecond)
	if got := limiter.Allow("user-a"); got != wantAllowed {
		t.Fatalf("at boundary: got %+v, want %+v", got, wantAllowed)
	}
}

func TestAllowClockRollback(t *testing.T) {
	base := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	now := base
	limiter, err := fixedwindow.New(
		fixedwindow.Config{MaxKeys: 10000, Limit: 1, Window: 10 * time.Second},
		fixedwindow.WithClock(func() time.Time { return now }),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Run in order: each step uses the state left by the previous request.
	steps := []struct {
		name   string
		second int
		want   fixedwindow.Decision
	}{
		{"initial allowance", 15, fixedwindow.Decision{Allowed: true}},
		{"rollback within window", 12, fixedwindow.Decision{Reason: fixedwindow.ReasonRateLimited, RetryAfter: 5 * time.Second}},
		{"rollback across window", 5, fixedwindow.Decision{Reason: fixedwindow.ReasonRateLimited, RetryAfter: 5 * time.Second}},
		{"clock catches up", 19, fixedwindow.Decision{Reason: fixedwindow.ReasonRateLimited, RetryAfter: time.Second}},
		{"next boundary", 20, fixedwindow.Decision{Allowed: true}},
		{"rollback after reset", 15, fixedwindow.Decision{Reason: fixedwindow.ReasonRateLimited, RetryAfter: 10 * time.Second}},
		{"return to same boundary", 20, fixedwindow.Decision{Reason: fixedwindow.ReasonRateLimited, RetryAfter: 10 * time.Second}},
		{"following window", 30, fixedwindow.Decision{Allowed: true}},
	}
	for _, step := range steps {
		now = base.Add(time.Duration(step.second) * time.Second)
		if got := limiter.Allow("user"); got != step.want {
			t.Fatalf("%s: got %+v, want %+v", step.name, got, step.want)
		}
	}
}

func TestCleanupExpiredClockRollback(t *testing.T) {
	base := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	now := base.Add(time.Second)
	limiter, err := fixedwindow.New(
		fixedwindow.Config{MaxKeys: 10000, Limit: 1, Window: 10 * time.Second},
		fixedwindow.WithClock(func() time.Time { return now }),
	)
	if err != nil {
		t.Fatal(err)
	}
	limiter.Allow("expired")
	now = base.Add(15 * time.Second)
	if got := limiter.CleanupExpired(); got != 1 {
		t.Fatalf("forward cleanup: removed %d keys, want 1", got)
	}

	// Cleanup's clock observation also protects requests for new keys.
	now = base.Add(5 * time.Second)
	if got := limiter.Allow("new"); !got.Allowed {
		t.Fatal("expected a first allowance for the new key")
	}
	now = base.Add(15 * time.Second)
	want := fixedwindow.Decision{Reason: fixedwindow.ReasonRateLimited, RetryAfter: 5 * time.Second}
	if got := limiter.Allow("new"); got != want {
		t.Fatalf("after clock catches up: got %+v, want %+v", got, want)
	}
	now = base.Add(5 * time.Second)
	if got := limiter.CleanupExpired(); got != 0 {
		t.Fatalf("rollback cleanup: removed %d keys, want 0", got)
	}
	if got := limiter.Allow("new"); got != want {
		t.Fatalf("after rollback cleanup: got %+v, want %+v", got, want)
	}
	now = base.Add(20 * time.Second)
	if got := limiter.CleanupExpired(); got != 1 {
		t.Fatalf("boundary cleanup: removed %d keys, want 1", got)
	}
	if got := limiter.Allow("new"); !got.Allowed {
		t.Fatal("expected a fresh allowance after expiration")
	}
}

func TestCleanupExpired(t *testing.T) {
	now := time.Date(2026, time.January, 1, 0, 0, 1, 0, time.UTC)
	limiter, err := fixedwindow.New(
		fixedwindow.Config{MaxKeys: 10000, Limit: 2, Window: 10 * time.Second},
		fixedwindow.WithClock(func() time.Time { return now }),
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := limiter.CleanupExpired(); got != 0 {
		t.Fatalf("empty limiter: removed %d keys, want 0", got)
	}

	limiter.Allow("inactive")
	limiter.Allow("returning")
	limiter.Allow("returning")
	now = time.Date(2026, time.January, 1, 0, 0, 9, 999999999, time.UTC)
	if got := limiter.CleanupExpired(); got != 0 {
		t.Fatalf("before expiration: removed %d keys, want 0", got)
	}
	if got := limiter.Allow("returning"); got.Allowed {
		t.Fatal("cleanup reset an unexpired exhausted counter")
	}

	now = now.Add(time.Nanosecond)
	// A request at the new boundary retires expired counters proactively.
	limiter.Allow("active")
	if got := limiter.CleanupExpired(); got != 0 {
		t.Fatalf("after proactive expiration: removed %d keys, want 0", got)
	}
	if got := limiter.CleanupExpired(); got != 0 {
		t.Fatalf("repeated cleanup: removed %d keys, want 0", got)
	}

	want := fixedwindow.Decision{Allowed: true, Remaining: 0}
	if got := limiter.Allow("active"); got != want {
		t.Fatalf("preserved active counter: got %+v, want %+v", got, want)
	}
	want = fixedwindow.Decision{Allowed: true, Remaining: 1}
	if got := limiter.Allow("returning"); got != want {
		t.Fatalf("returning key: got %+v, want %+v", got, want)
	}

	now = now.Add(time.Minute)
	if got := limiter.CleanupExpired(); got != 2 {
		t.Fatalf("after idle period: removed %d keys, want 2", got)
	}
	if got := limiter.CleanupExpired(); got != 0 {
		t.Fatalf("fully cleaned limiter: removed %d keys, want 0", got)
	}
}
