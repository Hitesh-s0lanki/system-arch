package tokenbucket_test

import (
	"context"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tb "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/tokenbucket"
)

func TestRandomizedExactCreditReference(t *testing.T) {
	for _, seed := range []int64{1, 42, 9876} {
		rng := rand.New(rand.NewSource(seed))
		now := time.Unix(100, 0)
		cfg := tb.Config{Limit: 7, Window: 17 * time.Millisecond, Capacity: 13, MaxKeys: 8}
		l, err := tb.New(cfg, tb.WithClock(func() time.Time { return now }))
		if err != nil {
			t.Fatal(err)
		}
		type state struct {
			credit int64
			last   time.Time
		}
		states := map[string]state{}
		high := now
		for step := 0; step < 10000; step++ {
			now = now.Add(time.Duration(rng.Intn(2000000) - 100000))
			if now.After(high) {
				high = now
			}
			key := string(rune('a' + rng.Intn(8)))
			s, ok := states[key]
			if !ok {
				s = state{int64(cfg.Capacity) * int64(cfg.Window), high}
			}
			s.credit += high.Sub(s.last).Nanoseconds() * int64(cfg.Limit)
			if max := int64(cfg.Capacity) * int64(cfg.Window); s.credit > max {
				s.credit = max
			}
			s.last = high
			allowed := s.credit >= int64(cfg.Window)
			retry := time.Duration(0)
			if allowed {
				s.credit -= int64(cfg.Window)
			} else {
				n := int64(cfg.Window) - s.credit
				retry = time.Duration((n + int64(cfg.Limit) - 1) / int64(cfg.Limit))
			}
			states[key] = s
			got := l.Allow(key)
			remaining := 0
			if allowed {
				remaining = int(s.credit / int64(cfg.Window))
			}
			if got.Allowed != allowed || got.Remaining != remaining || got.RetryAfter != retry {
				t.Fatalf("seed %d step %d: %+v expected allowed=%v remaining=%d retry=%v", seed, step, got, allowed, remaining, retry)
			}
			if step%31 == 0 {
				l.CleanupExpired()
			}
		}
	}
}
func TestLargeRateArithmeticAndConcurrentMaintenance(t *testing.T) {
	now := time.Unix(100, 0)
	l, err := tb.New(tb.Config{Limit: 2147483647, Window: time.Duration(1 << 62), Capacity: 1, MaxKeys: 1}, tb.WithClock(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	l.Allow("u")
	now = now.Add(time.Duration(1 << 62))
	if d := l.Allow("u"); !d.Allowed || d.Remaining != 0 {
		t.Fatalf("128-bit refill %+v", d)
	}
	var ticks atomic.Int64
	m, _ := tb.New(tb.Config{Limit: 1, Window: time.Second, Capacity: 20, MaxKeys: 20}, tb.WithClock(func() time.Time { return time.Unix(100, ticks.Load()) }))
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 500 {
				m.Evaluate(context.Background(), "u")
				m.CleanupExpired()
				m.Usage()
				ticks.Add(1000)
			}
		})
	}
	wg.Wait()
}
