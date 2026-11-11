package leakybucket_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	ratelimit "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting"
	lb "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/leakybucket"
)

func TestDrainRetryAndClockRollback(t *testing.T) {
	base := time.Unix(100, 0)
	now := base
	l, err := lb.New(lb.Config{Limit: 2, Window: time.Second, Capacity: 3, MaxKeys: 1}, lb.WithClock(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if d := l.Allow("u"); !d.Allowed || d.Remaining != 2-i {
			t.Fatalf("burst %d: %+v", i, d)
		}
	}
	if d := l.Allow("u"); d.Reason != ratelimit.ReasonRateLimited || d.RetryAfter != 500*time.Millisecond {
		t.Fatalf("full: %+v", d)
	}
	now = base.Add(250 * time.Millisecond)
	if d := l.Allow("u"); d.Allowed || d.RetryAfter != 250*time.Millisecond {
		t.Fatalf("partial drain: %+v", d)
	}
	now = base.Add(100 * time.Millisecond)
	if d := l.Allow("u"); d.Allowed || d.RetryAfter != 250*time.Millisecond {
		t.Fatalf("rollback: %+v", d)
	}
	now = base.Add(500 * time.Millisecond)
	if d := l.Allow("u"); !d.Allowed || d.Remaining != 0 {
		t.Fatalf("retry boundary: %+v", d)
	}
	// Rejections must not add load; the bucket drains fully 1.5s after admission.
	now = base.Add(2 * time.Second)
	if n := l.CleanupExpired(); n != 1 {
		t.Fatalf("cleanup %d", n)
	}
	if d := l.Allow("new"); !d.Allowed || d.Remaining != 2 {
		t.Fatalf("recreated: %+v", d)
	}
}

func TestCapacityValidationAndCancellation(t *testing.T) {
	now := time.Unix(100, 0)
	cfg := lb.Config{Limit: 1, Window: time.Second, Capacity: 2, MaxKeys: 1}
	l, err := lb.New(cfg, lb.WithClock(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"", " \t", strings.Repeat("x", 257)} {
		if d := l.Allow(key); d.Reason != ratelimit.ReasonInvalidKey {
			t.Fatalf("invalid: %+v", d)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := l.Evaluate(ctx, "u"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if l.Usage() != 0 {
		t.Fatal("invalid checks retained state")
	}
	if !l.Allow("u").Allowed || !l.Allow("u").Allowed {
		t.Fatal("burst rejected")
	}
	now = now.Add(time.Second)
	if d := l.Allow("v"); d.Reason != ratelimit.ReasonCapacityExceeded {
		t.Fatalf("active bucket evicted: %+v", d)
	}
	if l.CleanupExpired() != 0 || l.Usage() != 1 {
		t.Fatal("partially drained bucket removed")
	}
	now = now.Add(time.Second)
	if d := l.Allow("v"); !d.Allowed {
		t.Fatalf("capacity not reclaimed: %+v", d)
	}
	for _, mutate := range []func(*lb.Config){func(c *lb.Config) { c.Limit = 0 }, func(c *lb.Config) { c.Window = 0 }, func(c *lb.Config) { c.Capacity = 0 }, func(c *lb.Config) { c.MaxKeys = 0 }, func(c *lb.Config) { c.Capacity = math.MaxInt32; c.Window = time.Duration(math.MaxInt64) }} {
		bad := cfg
		mutate(&bad)
		if _, err := lb.New(bad); err == nil {
			t.Fatal("accepted invalid config")
		}
	}
	if _, err := lb.New(cfg, nil); err == nil {
		t.Fatal("nil option accepted")
	}
	if _, err := lb.New(cfg, lb.WithClock(nil)); err == nil {
		t.Fatal("nil clock accepted")
	}
}

func TestConcurrentAdmission(t *testing.T) {
	l, err := lb.New(lb.Config{Limit: 1, Window: time.Hour, Capacity: 17, MaxKeys: 1}, lb.WithClock(func() time.Time { return time.Unix(100, 0) }))
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var allowed atomic.Int32
	for range 200 {
		wg.Go(func() {
			if l.Allow("u").Allowed {
				allowed.Add(1)
			}
		})
	}
	wg.Wait()
	if allowed.Load() != 17 {
		t.Fatalf("allowed %d", allowed.Load())
	}
}

func TestFractionalRateRetry(t *testing.T) {
	now := time.Unix(100, 0)
	l, err := lb.New(lb.Config{Limit: 3, Window: time.Second, Capacity: 1, MaxKeys: 1}, lb.WithClock(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	l.Allow("u")
	d := l.Allow("u")
	if d.RetryAfter != 333333334*time.Nanosecond {
		t.Fatalf("retry %+v", d)
	}
	now = now.Add(d.RetryAfter)
	if !l.Allow("u").Allowed {
		t.Fatal("rounded retry did not permit admission")
	}
}

// The reference uses integer debt in nanoseconds, independent of the limiter's
// floating-point state. Small configurations keep all products within int64.
func TestRandomizedIntegerReference(t *testing.T) {
	for _, cfg := range []lb.Config{
		{Limit: 3, Window: time.Second, Capacity: 7, MaxKeys: 5},
		{Limit: 7, Window: 13 * time.Millisecond, Capacity: 3, MaxKeys: 5},
		{Limit: 10, Window: time.Nanosecond, Capacity: 1, MaxKeys: 5},
		{Limit: 1, Window: 37 * time.Nanosecond, Capacity: 9, MaxKeys: 5},
	} {
		t.Run(fmt.Sprintf("%d-%s", cfg.Limit, cfg.Window), func(t *testing.T) {
			now := time.Unix(100, 0)
			l, err := lb.New(cfg, lb.WithClock(func() time.Time { return now }))
			if err != nil {
				t.Fatal(err)
			}
			type state struct {
				debt    int64
				updated time.Time
			}
			states := map[string]state{}
			high := now
			rng := rand.New(rand.NewSource(42))
			for i := 0; i < 5000; i++ {
				// Include same-time arrivals, backwards clocks, long idle periods, and
				// attempts to insert more identities than the configured bound.
				delta := time.Duration(rng.Intn(9)-2) * cfg.Window / 10
				if i%101 == 0 {
					delta = cfg.Window * time.Duration(cfg.Capacity+1)
				}
				now = now.Add(delta)
				if now.After(high) {
					high = now
				}
				key := fmt.Sprintf("u%d", rng.Intn(8))
				prune := func() {
					for k, s := range states {
						if int64(high.Sub(s.updated))*int64(cfg.Limit) >= s.debt {
							delete(states, k)
						}
					}
				}
				if i%17 == 0 {
					before := len(states)
					prune()
					if got := l.CleanupExpired(); got != before-len(states) {
						t.Fatalf("cleanup step %d: %d", i, got)
					}
				}
				s, exists := states[key]
				want := ratelimit.Decision{}
				if !exists && len(states) >= cfg.MaxKeys {
					prune()
				}
				if !exists && len(states) >= cfg.MaxKeys {
					want.Reason = ratelimit.ReasonCapacityExceeded
				} else {
					debt := int64(0)
					if exists {
						debt = max(int64(0), s.debt-int64(high.Sub(s.updated))*int64(cfg.Limit))
					}
					if debt+int64(cfg.Window) <= int64(cfg.Capacity)*int64(cfg.Window) {
						debt += int64(cfg.Window)
						want.Allowed = true
						want.Remaining = int((int64(cfg.Capacity)*int64(cfg.Window) - debt) / int64(cfg.Window))
					} else {
						want.Reason = ratelimit.ReasonRateLimited
						excess := debt + int64(cfg.Window) - int64(cfg.Capacity)*int64(cfg.Window)
						want.RetryAfter = time.Duration((excess + int64(cfg.Limit) - 1) / int64(cfg.Limit))
					}
					states[key] = state{debt, high}
				}
				if got := l.Allow(key); got != want {
					t.Fatalf("step %d key %s got %+v want %+v", i, key, got, want)
				}
				if i%29 == 0 {
					prune()
					if got, want := l.Usage(), float64(len(states))/float64(cfg.MaxKeys); got != want {
						t.Fatalf("usage %v want %v", got, want)
					}
				}
			}
		})
	}
}

func TestConcurrentCleanupAndUsage(t *testing.T) {
	var tick atomic.Int64
	base := time.Unix(100, 0)
	l, err := lb.New(lb.Config{Limit: 3, Window: time.Millisecond, Capacity: 5, MaxKeys: 32}, lb.WithClock(func() time.Time { return base.Add(time.Duration(tick.Add(1000))) }))
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for worker := range 12 {
		wg.Go(func() {
			for i := range 1000 {
				if i%7 == 0 {
					l.CleanupExpired()
				} else if i%11 == 0 {
					if u := l.Usage(); u < 0 || u > 1 {
						t.Errorf("usage %v", u)
					}
				} else {
					d := l.Allow(fmt.Sprintf("u%d", (worker+i)%64))
					if d.Remaining < 0 || d.Remaining > 4 || (!d.Allowed && d.Reason == "") {
						t.Errorf("invalid %+v", d)
					}
				}
			}
		})
	}
	wg.Wait()
	tick.Add(int64(time.Second))
	if u := l.Usage(); u != 0 {
		t.Fatalf("idle usage %v", u)
	}
}
