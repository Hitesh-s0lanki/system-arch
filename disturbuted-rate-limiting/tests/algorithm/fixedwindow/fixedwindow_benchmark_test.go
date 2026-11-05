package fixedwindow_test

import (
	"strconv"
	"testing"
	"time"

	fixedwindow "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/fixedwindow"
)

// A fixed clock keeps these benchmarks inside a single window.
func benchmarkLimiter(b *testing.B, limit int, clock func() time.Time) *fixedwindow.Limiter {
	b.Helper()
	limiter, err := fixedwindow.New(
		fixedwindow.Config{MaxKeys: 10000, Limit: limit, Window: 10 * time.Second},
		fixedwindow.WithClock(clock),
	)
	if err != nil {
		b.Fatal(err)
	}
	return limiter
}

func BenchmarkAllowAllowed(b *testing.B) {
	now := time.Date(2026, time.January, 1, 0, 0, 1, 0, time.UTC)
	limiter := benchmarkLimiter(b, int(^uint(0)>>1), func() time.Time { return now })
	limiter.Allow("user") // Exclude first-key allocation from the hot path.
	b.ReportAllocs()

	for b.Loop() {
		if !limiter.Allow("user").Allowed {
			b.Fatal("unexpected rejection")
		}
	}
}

func BenchmarkAllowRejected(b *testing.B) {
	now := time.Date(2026, time.January, 1, 0, 0, 1, 0, time.UTC)
	limiter := benchmarkLimiter(b, 1, func() time.Time { return now })
	limiter.Allow("user")
	b.ReportAllocs()

	for b.Loop() {
		if limiter.Allow("user").Allowed {
			b.Fatal("unexpected allowance")
		}
	}
}

func BenchmarkAllowParallel(b *testing.B) {
	now := time.Date(2026, time.January, 1, 0, 0, 1, 0, time.UTC)
	limiter := benchmarkLimiter(b, int(^uint(0)>>1), func() time.Time { return now })
	limiter.Allow("user")
	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if !limiter.Allow("user").Allowed {
				b.Error("unexpected rejection")
				return
			}
		}
	})
}

func BenchmarkAllowManyKeys(b *testing.B) {
	now := time.Date(2026, time.January, 1, 0, 0, 1, 0, time.UTC)
	limiter := benchmarkLimiter(b, int(^uint(0)>>1), func() time.Time { return now })
	keys := make([]string, 1024)
	for i := range keys {
		keys[i] = "user-" + strconv.Itoa(i)
		limiter.Allow(keys[i])
	}
	b.ReportAllocs()
	index := 0

	// Measure existing-key lookups; key creation and map growth are excluded.
	for b.Loop() {
		if !limiter.Allow(keys[index]).Allowed {
			b.Fatal("unexpected rejection")
		}
		index = (index + 1) % len(keys)
	}
}

func BenchmarkCleanupExpired(b *testing.B) {
	for _, size := range []int{100, 1000, 10000} {
		b.Run(strconv.Itoa(size)+"Keys", func(b *testing.B) {
			b.StopTimer()
			keys := make([]string, size)
			for i := range keys {
				keys[i] = "user-" + strconv.Itoa(i)
			}
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				// Rebuild outside the timer so every measured scan removes
				// expired keys, rather than scanning an already-empty map.
				now := time.Date(2026, time.January, 1, 0, 0, 1, 0, time.UTC)
				limiter := benchmarkLimiter(b, 1, func() time.Time { return now })
				for _, key := range keys {
					limiter.Allow(key)
				}
				now = now.Add(10 * time.Second)

				b.StartTimer()
				removed := limiter.CleanupExpired()
				b.StopTimer()

				if removed != size {
					b.Fatalf("removed %d keys, want %d", removed, size)
				}
			}
		})
	}
}

func BenchmarkAllowNewKeys(b *testing.B) {
	now := time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC)
	keys := make([]string, 1024)
	for i := range keys {
		keys[i] = "new-" + strconv.Itoa(i)
	}
	var limiter *fixedwindow.Limiter
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%len(keys) == 0 {
			b.StopTimer()
			limiter = benchmarkLimiter(b, 1, func() time.Time { return now })
			b.StartTimer()
		}
		if !limiter.Allow(keys[i%len(keys)]).Allowed {
			b.Fatal("new key rejected")
		}
	}
}

func BenchmarkAllowCapacityRejected(b *testing.B) {
	for _, size := range []int{100, 1000, 10000} {
		b.Run(strconv.Itoa(size)+"Keys", func(b *testing.B) {
			now := time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC)
			limiter, err := fixedwindow.New(fixedwindow.Config{Limit: 1, Window: 10 * time.Second, MaxKeys: size}, fixedwindow.WithClock(func() time.Time { return now }))
			if err != nil {
				b.Fatal(err)
			}
			for i := 0; i < size; i++ {
				limiter.Allow("user-" + strconv.Itoa(i))
			}
			// Exclude the first full scan; measure repeated full-capacity traffic.
			limiter.Allow("unknown")
			b.ReportAllocs()
			for b.Loop() {
				if got := limiter.Allow("unknown"); got.Reason != fixedwindow.ReasonCapacityExceeded {
					b.Fatalf("unexpected decision: %+v", got)
				}
			}
		})
	}
}
