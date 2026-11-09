package fixedwindow_test

import (
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/fixedwindow"
)

func TestInvalidKeysDoNotConsumeCapacity(t *testing.T) {
	l, err := fixedwindow.New(fixedwindow.Config{Limit: 1, Window: time.Second, MaxKeys: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"", " \t\n", strings.Repeat("x", 257), strings.Repeat("é", 129)} {
		if got := l.Allow(key); got != (fixedwindow.Decision{Reason: fixedwindow.ReasonInvalidKey}) {
			t.Fatalf("invalid key: %+v", got)
		}
	}
	if !l.Allow(strings.Repeat("é", 128)).Allowed {
		t.Fatal("256-byte key should be accepted after invalid requests")
	}
}

func TestShortAndUnusualWindows(t *testing.T) {
	for _, window := range []time.Duration{time.Nanosecond, 1500 * time.Millisecond, 7 * time.Second} {
		t.Run(window.String(), func(t *testing.T) {
			start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Truncate(window)
			now := start
			l, err := fixedwindow.New(fixedwindow.Config{Limit: 1, Window: window, MaxKeys: 2}, fixedwindow.WithClock(func() time.Time { return now }))
			if err != nil {
				t.Fatal(err)
			}
			if !l.Allow("u").Allowed {
				t.Fatal("first request rejected")
			}
			now = start.Add(window - time.Nanosecond)
			want := fixedwindow.Decision{Reason: fixedwindow.ReasonRateLimited, RetryAfter: time.Nanosecond}
			if got := l.Allow("u"); got != want {
				t.Fatalf("before boundary: got %+v, want %+v", got, want)
			}
			now = start.Add(window)
			if !l.Allow("u").Allowed {
				t.Fatal("boundary did not reset allowance")
			}
			now = start.Add(6 * window)
			if !l.Allow("u").Allowed {
				t.Fatal("skipped windows did not reset allowance")
			}
		})
	}
}

func TestConcurrentExpirationAndCleanup(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC)
	l, err := fixedwindow.New(fixedwindow.Config{Limit: 8, Window: 10 * time.Second, MaxKeys: 64}, fixedwindow.WithClock(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 16; i++ {
		l.Allow("old-" + strconv.Itoa(i))
	}
	// Advance before launching workers; the captured time is read-only thereafter.
	now = now.Truncate(10 * time.Second).Add(10 * time.Second)
	start := make(chan struct{})
	results := make(chan bool, 128)
	removed := make(chan int, 16)
	var wg sync.WaitGroup
	for i := 0; i < 128; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); <-start; results <- l.Allow("current").Allowed }()
	}
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); <-start; removed <- l.CleanupExpired() }()
	}
	close(start)
	wg.Wait()
	close(results)
	close(removed)
	allowed, cleaned := 0, 0
	for result := range results {
		if result {
			allowed++
		}
	}
	for count := range removed {
		cleaned += count
	}
	if allowed != 8 || (cleaned != 0 && cleaned != 16) || l.Usage() != 1.0/64 {
		t.Fatalf("allowed=%d cleaned=%d usage=%v: expired state must be retired once and the current counter preserved", allowed, cleaned, l.Usage())
	}
}
