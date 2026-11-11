package slidingcounter_test

import (
	"context"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	ratelimit "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting"
	sc "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/slidingcounter"
)

func TestWeightedBoundaryAndRetry(t *testing.T) {
	now := time.UnixMilli(0)
	l, err := sc.New(sc.Config{Limit: 4, Window: time.Second, MaxKeys: 1}, sc.WithClock(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		if d := l.Allow("u"); !d.Allowed || d.Remaining != 3-i {
			t.Fatal(d)
		}
	}
	if d := l.Allow("u"); d.Allowed || d.RetryAfter != 1250*time.Millisecond {
		t.Fatal(d)
	}
	now = time.UnixMilli(1000)
	if d := l.Allow("u"); d.Allowed || d.RetryAfter != 250*time.Millisecond {
		t.Fatal(d)
	}
	now = time.UnixMilli(1249)
	if d := l.Allow("u"); d.Allowed || d.RetryAfter != time.Millisecond {
		t.Fatal(d)
	}
	now = time.UnixMilli(1250)
	if d := l.Allow("u"); !d.Allowed || d.Remaining != 0 {
		t.Fatal(d)
	}
	// Rollback cannot grant quota.
	now = time.UnixMilli(1000)
	if d := l.Allow("u"); d.Allowed || d.RetryAfter != 250*time.Millisecond {
		t.Fatal(d)
	}
	now = time.UnixMilli(2000)
	if n := l.CleanupExpired(); n != 0 {
		t.Fatal("removed contributing current window", n)
	}
	now = time.UnixMilli(3000)
	if n := l.CleanupExpired(); n != 1 || l.Usage() != 0 {
		t.Fatal(n)
	}
	if !l.Allow("new").Allowed {
		t.Fatal("capacity not reclaimed")
	}
}

func TestRandomizedAgainstTimestampBuckets(t *testing.T) {
	now := time.UnixMilli(0)
	l, _ := sc.New(sc.Config{Limit: 7, Window: time.Second, MaxKeys: 1}, sc.WithClock(func() time.Time { return now }))
	rng := rand.New(rand.NewSource(42))
	var accepted []int64
	for i := 0; i < 5000; i++ {
		now = now.Add(time.Duration(rng.Intn(350)) * time.Millisecond)
		ms := now.UnixMilli()
		start := ms - ms%1000
		var current, previous int64
		for _, stamp := range accepted {
			if stamp >= start {
				current++
			} else if stamp >= start-1000 {
				previous++
			}
		}
		used := current*1000 + previous*(1000-(ms-start))
		want := used+1000 <= 7000
		d := l.Allow("u")
		if d.Allowed != want {
			t.Fatalf("step %d at %d: %+v usage %d", i, ms, d, used)
		}
		if want {
			accepted = append(accepted, ms)
			if d.Remaining != int((6000-used)/1000) {
				t.Fatal(d)
			}
		} else if d.RetryAfter <= 0 {
			t.Fatal(d)
		}
	}
}
func TestConcurrentAdmissionCapacityAndValidation(t *testing.T) {
	l, _ := sc.New(sc.Config{Limit: 10, Window: time.Second, MaxKeys: 1}, sc.WithClock(func() time.Time { return time.UnixMilli(0) }))
	var wg sync.WaitGroup
	var admitted atomic.Int32
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if l.Allow("u").Allowed {
				admitted.Add(1)
			}
			l.CleanupExpired()
		}()
	}
	wg.Wait()
	if admitted.Load() != 10 {
		t.Fatal(admitted.Load())
	}
	if d := l.Allow("other"); d.Reason != ratelimit.ReasonCapacityExceeded {
		t.Fatal(d)
	}
	for _, key := range []string{"", "  ", strings.Repeat("x", 257)} {
		if d := l.Allow(key); d.Reason != ratelimit.ReasonInvalidKey {
			t.Fatal(d)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := l.Evaluate(ctx, "u"); err != context.Canceled {
		t.Fatal(err)
	}
	for _, c := range []sc.Config{{}, {Limit: 1, Window: time.Nanosecond, MaxKeys: 1}, {Limit: 2147483647, Window: time.Hour, MaxKeys: 1}} {
		if _, err := sc.New(c); err == nil {
			t.Fatal("invalid config accepted", c)
		}
	}
	valid := sc.Config{Limit: 1, Window: time.Second, MaxKeys: 1}
	if _, err := sc.New(valid, nil); err == nil {
		t.Fatal("nil option accepted")
	}
	if _, err := sc.New(valid, sc.WithClock(nil)); err == nil {
		t.Fatal("nil clock accepted")
	}
}
