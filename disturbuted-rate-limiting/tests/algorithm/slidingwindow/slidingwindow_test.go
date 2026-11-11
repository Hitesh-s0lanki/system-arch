package slidingwindow_test

import (
	"context"
	"errors"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	ratelimit "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting"
	sw "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/slidingwindow"
)

func TestRollingWindowAndRollback(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := base
	l, err := sw.New(sw.Config{Limit: 2, Window: 10 * time.Second, MaxKeys: 2}, sw.WithClock(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	steps := []struct {
		offset    time.Duration
		allowed   bool
		remaining int
		retry     time.Duration
	}{
		{time.Second, true, 1, 0}, {9 * time.Second, true, 0, 0},
		{10 * time.Second, false, 0, time.Second}, // An aligned fixed-window boundary must not reset quota.
		{11*time.Second - time.Nanosecond, false, 0, time.Nanosecond},
		{11 * time.Second, true, 0, 0},               // Only the first timestamp expires.
		{5 * time.Second, false, 0, 8 * time.Second}, // Rollback must not create allowance.
		{19 * time.Second, true, 0, 0},
	}
	for _, s := range steps {
		now = base.Add(s.offset)
		want := ratelimit.Decision{Allowed: s.allowed, Remaining: s.remaining, RetryAfter: s.retry}
		if !s.allowed {
			want.Reason = ratelimit.ReasonRateLimited
		}
		if got := l.Allow("user"); got != want {
			t.Fatalf("at %v: got %+v want %+v", s.offset, got, want)
		}
	}
}
func TestConcurrentQuotaAndCapacity(t *testing.T) {
	now := time.Now()
	l := newLimiter(t, sw.Config{Limit: 7, Window: time.Hour, MaxKeys: 2}, sw.WithClock(func() time.Time { return now }))
	var wg sync.WaitGroup
	var allowed atomic.Int32
	var seen [7]atomic.Int32
	for range 200 {
		wg.Go(func() {
			d := l.Allow("same")
			if d.Allowed {
				allowed.Add(1)
				if d.Remaining < 0 || d.Remaining >= 7 || d.Reason != "" || d.RetryAfter != 0 {
					t.Errorf("invalid admission %+v", d)
				} else if seen[d.Remaining].Add(1) != 1 {
					t.Errorf("duplicate remaining %d", d.Remaining)
				}
			} else if want := (ratelimit.Decision{Reason: ratelimit.ReasonRateLimited, RetryAfter: time.Hour}); d != want {
				t.Errorf("rejection %+v want %+v", d, want)
			}
		})
	}
	wg.Wait()
	if allowed.Load() != 7 {
		t.Fatalf("allowed %d", allowed.Load())
	}
	if !l.Allow("other").Allowed || l.Allow("third").Reason != ratelimit.ReasonCapacityExceeded {
		t.Fatal("capacity or identity isolation")
	}
	now = now.Add(time.Hour)
	if !l.Allow("third").Allowed {
		t.Fatal("expired identities were not reclaimed at capacity")
	}
	now = now.Add(time.Hour)
	if n := l.CleanupExpired(); n != 1 || l.Usage() != 0 {
		t.Fatalf("cleanup %d usage %v", n, l.Usage())
	}
}
func TestValidationAndCancellation(t *testing.T) {
	for _, cfg := range []sw.Config{{}, {Limit: 1, Window: time.Second}, {Limit: 1, MaxKeys: 1}, {Window: time.Second, MaxKeys: 1}, {Limit: -1, Window: time.Second, MaxKeys: 1}, {Limit: 1, Window: -1, MaxKeys: 1}, {Limit: 1, Window: time.Second, MaxKeys: -1}} {
		if _, err := sw.New(cfg); err == nil {
			t.Fatal("accepted invalid config")
		}
	}
	cfg := sw.Config{Limit: 1, Window: time.Second, MaxKeys: 1}
	for _, o := range []sw.Option{nil, sw.WithClock(nil)} {
		if _, err := sw.New(cfg, o); err == nil {
			t.Fatal("accepted invalid option")
		}
	}
	l := newLimiter(t, cfg)
	for _, key := range []string{"", " \t", strings.Repeat("é", 129)} {
		if l.Allow(key).Reason != ratelimit.ReasonInvalidKey {
			t.Fatal("accepted invalid key")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := l.Evaluate(ctx, "valid"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if !l.Allow(strings.Repeat("é", 128)).Allowed {
		t.Fatal("invalid requests consumed capacity")
	}
}

// Compare against a simple independent reference across many rolling boundaries,
// capacity rejections, rollback readings, and explicit cleanup calls.
func TestRandomizedReference(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := base
	effective := base
	l := newLimiter(t, sw.Config{Limit: 3, Window: 10 * time.Second, MaxKeys: 3}, sw.WithClock(func() time.Time { return now }))
	reference := map[string][]time.Time{}
	rng := rand.New(rand.NewSource(42))
	for i := 0; i < 3000; i++ {
		now = now.Add(time.Duration(rng.Intn(7)-2) * time.Second)
		if now.After(effective) {
			effective = now
		}
		for key, log := range reference {
			var kept []time.Time
			for _, stamp := range log {
				if stamp.After(effective.Add(-10 * time.Second)) {
					kept = append(kept, stamp)
				}
			}
			if len(kept) == 0 {
				delete(reference, key)
			} else {
				reference[key] = kept
			}
		}
		if i%13 == 0 {
			l.CleanupExpired()
		}
		key := string(rune('a' + rng.Intn(5)))
		log := reference[key]
		want := ratelimit.Decision{}
		switch {
		case len(log) == 0 && len(reference) >= 3:
			want.Reason = ratelimit.ReasonCapacityExceeded
		case len(log) >= 3:
			want.Reason = ratelimit.ReasonRateLimited
			want.RetryAfter = log[0].Add(10 * time.Second).Sub(effective)
		default:
			reference[key] = append(log, effective)
			want.Allowed = true
			want.Remaining = 3 - len(log) - 1
		}
		if got := l.Allow(key); got != want {
			t.Fatalf("step %d got %+v want %+v", i, got, want)
		}
	}
}

func TestConcurrentCleanupPreservesActiveLog(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l := newLimiter(t, sw.Config{Limit: 7, Window: time.Second, MaxKeys: 2}, sw.WithClock(func() time.Time { return now }))
	l.Allow("old")
	now = now.Add(time.Second)
	var wg sync.WaitGroup
	var admitted atomic.Int32
	var retired atomic.Int32
	for range 100 {
		wg.Go(func() {
			if l.Allow("active").Allowed {
				admitted.Add(1)
			}
		})
	}
	for range 20 {
		wg.Go(func() { retired.Add(int32(l.CleanupExpired())); l.Usage() })
	}
	wg.Wait()
	if admitted.Load() != 7 || retired.Load() > 1 || l.Usage() != 0.5 {
		t.Fatalf("admitted %d retired %d usage %v", admitted.Load(), retired.Load(), l.Usage())
	}
}

func newLimiter(t *testing.T, cfg sw.Config, options ...sw.Option) *sw.Limiter {
	t.Helper()
	l, err := sw.New(cfg, options...)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestRejectedAttemptsDoNotExtendLogOrCapacity(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := base
	l := newLimiter(t, sw.Config{Limit: 1, Window: 10 * time.Second, MaxKeys: 1}, sw.WithClock(func() time.Time { return now }))
	if got := l.Allow("original"); got != (ratelimit.Decision{Allowed: true}) {
		t.Fatal(got)
	}
	for second := 1; second < 10; second++ {
		now = base.Add(time.Duration(second) * time.Second)
		if got := l.Allow("original"); got != (ratelimit.Decision{Reason: ratelimit.ReasonRateLimited, RetryAfter: time.Duration(10-second) * time.Second}) {
			t.Fatalf("at %d: %+v", second, got)
		}
		if got := l.Allow("untracked"); got != (ratelimit.Decision{Reason: ratelimit.ReasonCapacityExceeded}) {
			t.Fatal(got)
		}
	}
	now = base.Add(10 * time.Second)
	if n := l.CleanupExpired(); n != 1 {
		t.Fatalf("rejections changed expiry or stored keys: removed %d", n)
	}
	if got := l.Allow("untracked"); got != (ratelimit.Decision{Allowed: true}) {
		t.Fatal(got)
	}
}

func TestHeapRescheduleAndPartialExpiry(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := base
	l := newLimiter(t, sw.Config{Limit: 2, Window: 10 * time.Second, MaxKeys: 3}, sw.WithClock(func() time.Time { return now }))
	for _, key := range []string{"a", "b", "c"} {
		if !l.Allow(key).Allowed {
			t.Fatal("initial admission", key)
		}
	}
	now = base.Add(9 * time.Second)
	if got := l.Allow("a"); got != (ratelimit.Decision{Allowed: true}) {
		t.Fatal(got)
	}
	now = base.Add(10 * time.Second)
	if removed := l.CleanupExpired(); removed != 2 {
		t.Fatalf("removed %d want 2; updated heap entry must survive", removed)
	}
	if l.Usage() != 1.0/3 {
		t.Fatalf("usage %v", l.Usage())
	}
	if got := l.Allow("a"); got != (ratelimit.Decision{Allowed: true}) {
		t.Fatalf("oldest timestamp did not expire independently %+v", got)
	}
	now = base.Add(19 * time.Second)
	if got := l.Allow("a"); got != (ratelimit.Decision{Allowed: true}) {
		t.Fatal(got)
	}
	now = base.Add(29 * time.Second)
	if l.CleanupExpired() != 1 || l.CleanupExpired() != 0 {
		t.Fatal("identity must be retired exactly once")
	}
}

func TestCancellationWhileWaitingForMutexDoesNotCharge(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	l := newLimiter(t, sw.Config{Limit: 2, Window: time.Hour, MaxKeys: 1}, sw.WithClock(func() time.Time {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
		return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	}))
	done := make(chan ratelimit.Decision, 1)
	go func() { done <- l.Allow("u") }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("first check did not acquire lock")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// The first check still owns the mutex, so this cancellation cannot charge quota.
	checked := make(chan struct{})
	observed := &observedContext{Context: ctx, checked: checked}
	waiting := make(chan error, 1)
	go func() { _, err := l.Evaluate(observed, "u"); waiting <- err }()
	select {
	case <-checked:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("waiting request never checked context")
	}
	cancel()
	close(release)
	select {
	case d := <-done:
		if d != (ratelimit.Decision{Allowed: true, Remaining: 1}) {
			t.Fatal(d)
		}
	case <-time.After(time.Second):
		t.Fatal("first check blocked")
	}
	select {
	case err := <-waiting:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled check blocked")
	}
	if got := l.Allow("u"); got != (ratelimit.Decision{Allowed: true}) {
		t.Fatalf("cancellation charged allowance %+v", got)
	}
}

func TestNanosecondWindowAndExactKeySemantics(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l := newLimiter(t, sw.Config{Limit: 1, Window: time.Nanosecond, MaxKeys: 3}, sw.WithClock(func() time.Time { return now }))
	for _, key := range []string{"u", " u ", strings.Repeat("é", 128)} {
		if got := l.Allow(key); got != (ratelimit.Decision{Allowed: true}) {
			t.Fatalf("key %q: %+v", key, got)
		}
	}
	if got := l.Allow("u"); got != (ratelimit.Decision{Reason: ratelimit.ReasonRateLimited, RetryAfter: time.Nanosecond}) {
		t.Fatal(got)
	}
	now = now.Add(time.Nanosecond)
	if got := l.Allow("u"); got != (ratelimit.Decision{Allowed: true}) {
		t.Fatal(got)
	}
}

// Signal only after capturing the initial, uncanceled Err result. This guarantees
// cancellation is tested after the pre-lock check without timing-based sleeps.
type observedContext struct {
	context.Context
	checked chan struct{}
	calls   atomic.Int32
}

func (c *observedContext) Err() error {
	err := c.Context.Err()
	if c.calls.Add(1) == 1 {
		close(c.checked)
	}
	return err
}
