package fixedwindow_test

import (
	"strconv"
	"sync"
	"testing"
	"time"

	fixedwindow "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/fixedwindow"
)

func TestNewRejectsInvalidCapacity(t *testing.T) {
	for _, maxKeys := range []int{0, -1} {
		t.Run(strconv.Itoa(maxKeys), func(t *testing.T) {
			limiter, err := fixedwindow.New(fixedwindow.Config{
				Limit: 2, Window: 10 * time.Second, MaxKeys: maxKeys,
			})
			if err == nil || limiter != nil {
				t.Fatalf("got limiter %v, error %v; want nil limiter and error", limiter, err)
			}
		})
	}
}

func TestAllowCapacity(t *testing.T) {
	base := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	now := base.Add(time.Second)
	limiter, err := fixedwindow.New(
		fixedwindow.Config{Limit: 2, Window: 10 * time.Second, MaxKeys: 2},
		fixedwindow.WithClock(func() time.Time { return now }),
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"a", "b"} {
		want := fixedwindow.Decision{Allowed: true, Remaining: 1}
		if got := limiter.Allow(key); got != want {
			t.Fatalf("first request for %q: got %+v, want %+v", key, got, want)
		}
	}
	// Repeated capacity rejections must not store the key or evict active keys.
	for i := 0; i < 3; i++ {
		want := fixedwindow.Decision{Reason: fixedwindow.ReasonCapacityExceeded}
		if got := limiter.Allow("c"); got != want {
			t.Fatalf("capacity rejection: got %+v, want %+v", got, want)
		}
	}
	for _, key := range []string{"a", "b"} {
		want := fixedwindow.Decision{Allowed: true}
		if got := limiter.Allow(key); got != want {
			t.Fatalf("existing key %q: got %+v, want %+v", key, got, want)
		}
		want = fixedwindow.Decision{
			Reason: fixedwindow.ReasonRateLimited, RetryAfter: 9 * time.Second,
		}
		if got := limiter.Allow(key); got != want {
			t.Fatalf("exhausted key %q: got %+v, want %+v", key, got, want)
		}
	}

	// Rollback must not free capacity by expiring active counters.
	now = base.Add(-time.Second)
	if got := limiter.Allow("c"); got.Reason != fixedwindow.ReasonCapacityExceeded {
		t.Fatalf("rollback at capacity: got %+v", got)
	}

	// No explicit cleanup: a new key triggers reclamation at the boundary.
	now = base.Add(10 * time.Second)
	want := fixedwindow.Decision{Allowed: true, Remaining: 1}
	if got := limiter.Allow("c"); got != want {
		t.Fatalf("after expiration: got %+v, want %+v", got, want)
	}
	if got := limiter.Allow("a"); got != want {
		t.Fatalf("returning key: got %+v, want %+v", got, want)
	}

	// Counting removals through the public API also checks that rejected keys
	// never increased the number of stored entries.
	now = base.Add(20 * time.Second)
	if got := limiter.CleanupExpired(); got != 2 {
		t.Fatalf("removed %d stored keys, want 2", got)
	}
}

func TestAllowCapacityConcurrent(t *testing.T) {
	const capacity, requests = 32, 256
	now := time.Date(2026, time.January, 1, 0, 0, 1, 0, time.UTC)
	limiter, err := fixedwindow.New(
		fixedwindow.Config{Limit: 1, Window: 10 * time.Second, MaxKeys: capacity},
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
		go func(key string) {
			defer workers.Done()
			<-start
			results <- limiter.Allow(key)
		}(strconv.Itoa(i))
	}
	close(start)
	workers.Wait()
	close(results)

	allowed := 0
	for got := range results {
		if got.Allowed {
			allowed++
			if got != (fixedwindow.Decision{Allowed: true}) {
				t.Errorf("unexpected allowed decision: %+v", got)
			}
		} else if got != (fixedwindow.Decision{Reason: fixedwindow.ReasonCapacityExceeded}) {
			t.Errorf("unexpected rejection: %+v", got)
		}
	}
	if allowed != capacity {
		t.Fatalf("allowed %d unique keys, want %d", allowed, capacity)
	}

	// All workers have finished before advancing the test clock.
	now = now.Add(10 * time.Second)
	if got := limiter.CleanupExpired(); got != capacity {
		t.Fatalf("stored %d keys, want %d", got, capacity)
	}
}
