package tokenbucket_test

import (
	"context"
	"errors"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	ratelimit "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting"
	tb "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/tokenbucket"
)

func TestBurstFractionalRefillAndClockRollback(t *testing.T) {
	now := time.Unix(100, 0)
	l, err := tb.New(tb.Config{Limit: 2, Window: time.Second, Capacity: 3, MaxKeys: 2}, tb.WithClock(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if d := l.Allow("u"); !d.Allowed || d.Remaining != 2-i {
			t.Fatalf("burst %d: %+v", i, d)
		}
	}
	if d := l.Allow("u"); d.RetryAfter != 500*time.Millisecond || d.Reason != ratelimit.ReasonRateLimited {
		t.Fatalf("empty %+v", d)
	}
	now = now.Add(250 * time.Millisecond)
	if d := l.Allow("u"); d.RetryAfter != 250*time.Millisecond {
		t.Fatalf("fractional %+v", d)
	}
	now = now.Add(-time.Second)
	if d := l.Allow("u"); d.RetryAfter != 250*time.Millisecond {
		t.Fatalf("rollback %+v", d)
	}
	now = time.Unix(100, 0).Add(500 * time.Millisecond)
	if d := l.Allow("u"); !d.Allowed || d.Remaining != 0 {
		t.Fatalf("refill %+v", d)
	}
	now = now.Add(time.Hour)
	if d := l.Allow("u"); !d.Allowed || d.Remaining != 2 {
		t.Fatalf("capacity cap %+v", d)
	}
}
func TestCleanupPreservesPartiallyEmptyBuckets(t *testing.T) {
	now := time.Unix(100, 0)
	l, _ := tb.New(tb.Config{Limit: 1, Window: time.Second, Capacity: 3, MaxKeys: 1}, tb.WithClock(func() time.Time { return now }))
	for range 3 {
		l.Allow("u")
	}
	now = now.Add(time.Second)
	if n := l.CleanupExpired(); n != 0 {
		t.Fatalf("partial bucket deleted: %d", n)
	}
	if d := l.Allow("other"); d.Reason != ratelimit.ReasonCapacityExceeded {
		t.Fatalf("capacity %+v", d)
	}
	if u := l.Usage(); u != 1 {
		t.Fatalf("usage %v", u)
	}
	now = now.Add(2 * time.Second)
	if d := l.Allow("other"); !d.Allowed {
		t.Fatalf("idle capacity not reclaimed %+v", d)
	}
	now = now.Add(time.Second)
	if n := l.CleanupExpired(); n != 1 {
		t.Fatalf("full bucket retained: %d", n)
	}
	if l.Usage() != 0 {
		t.Fatal("idle usage")
	}
}
func TestConcurrentConsumption(t *testing.T) {
	now := time.Now()
	l, _ := tb.New(tb.Config{Limit: 1, Window: time.Hour, Capacity: 25, MaxKeys: 2}, tb.WithClock(func() time.Time { return now }))
	var wg sync.WaitGroup
	var allowed atomic.Int32
	for range 200 {
		wg.Go(func() {
			if l.Allow("shared").Allowed {
				allowed.Add(1)
			}
		})
	}
	wg.Wait()
	if allowed.Load() != 25 {
		t.Fatalf("overspent burst: %d", allowed.Load())
	}
	if !l.Allow("independent").Allowed {
		t.Fatal("identities share quota")
	}
}
func TestValidationAndCancellation(t *testing.T) {
	good := tb.Config{Limit: 1, Window: time.Second, Capacity: 1, MaxKeys: 1}
	for _, mutate := range []func(*tb.Config){func(c *tb.Config) { c.Limit = 0 }, func(c *tb.Config) { c.Window = 0 }, func(c *tb.Config) { c.Capacity = 0 }, func(c *tb.Config) { c.MaxKeys = 0 }, func(c *tb.Config) { c.Window = time.Duration(math.MaxInt64); c.Capacity = 2 }} {
		bad := good
		mutate(&bad)
		if _, err := tb.New(bad); err == nil {
			t.Fatalf("accepted %+v", bad)
		}
	}
	for _, o := range []tb.Option{nil, tb.WithClock(nil)} {
		if _, err := tb.New(good, o); err == nil {
			t.Fatal("invalid option accepted")
		}
	}
	l, _ := tb.New(good)
	for _, key := range []string{"", " \t", strings.Repeat("x", 257)} {
		if d := l.Allow(key); d.Reason != ratelimit.ReasonInvalidKey {
			t.Fatalf("invalid key %+v", d)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := l.Evaluate(ctx, "u"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel %v", err)
	}
	if !l.Allow("u").Allowed {
		t.Fatal("invalid checks consumed quota")
	}
}
func TestRetryRoundsUp(t *testing.T) {
	now := time.Unix(100, 0)
	l, _ := tb.New(tb.Config{Limit: 3, Window: time.Second, Capacity: 1, MaxKeys: 1}, tb.WithClock(func() time.Time { return now }))
	l.Allow("u")
	d := l.Allow("u")
	if d.RetryAfter != 333333334*time.Nanosecond {
		t.Fatalf("rounding %+v", d)
	}
	now = now.Add(d.RetryAfter)
	if !l.Allow("u").Allowed {
		t.Fatal("retry hint too short")
	}
}

func TestRepeatedRejectionsDoNotDelayTokenBoundary(t *testing.T) {
	now := time.Unix(100, 0)
	l, _ := tb.New(tb.Config{Limit: 1, Window: time.Second, Capacity: 1, MaxKeys: 1}, tb.WithClock(func() time.Time { return now }))
	l.Allow("u")
	for i := 1; i <= 10; i++ {
		now = now.Add(100 * time.Millisecond)
		d := l.Allow("u")
		if d.Allowed != (i == 10) {
			t.Fatalf("step %d: %+v", i, d)
		}
	}
}
