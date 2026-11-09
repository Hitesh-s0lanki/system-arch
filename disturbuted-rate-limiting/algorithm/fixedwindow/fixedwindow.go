package fixedwindow

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	ratelimit "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting"
)

// Config defines a fixed-window limit.
type Config struct {
	Limit  int
	Window time.Duration
	// MaxKeys bounds the number of tracked keys, including expired entries.
	MaxKeys int
}

// Reason explains why a request was rejected. Its zero value means no rejection.
type Reason = ratelimit.Reason

const (
	// ReasonRateLimited means the key exhausted its current window allowance.
	ReasonRateLimited = ratelimit.ReasonRateLimited
	// ReasonCapacityExceeded means a new key could not be tracked.
	ReasonCapacityExceeded = ratelimit.ReasonCapacityExceeded
	ReasonInvalidKey       = ratelimit.ReasonInvalidKey
)

// MaxKeyBytes bounds stored key length, measured in bytes rather than runes.
const MaxKeyBytes = 256

// Decision describes the result of one request.
type Decision = ratelimit.Decision

// windowState tracks one key's active window.
type windowState struct {
	// key owns a bounded copy, reused on updates so caller substrings cannot
	// replace the stored map key with a reference to a large backing string.
	key   string
	start time.Time
	count int
}

// Limiter tracks requests within fixed time windows.
// A Limiter must not be copied after first use.
type Limiter struct {
	mu           sync.Mutex
	config       Config
	now          func() time.Time
	windows      map[string]windowState
	lastNow      time.Time
	clockSet     bool
	activeWindow time.Time
	windowSet    bool
}

// Option configures a limiter during construction.
type Option func(*Limiter)

// WithClock supplies the limiter's time source.
// The function must be non-nil and safe for concurrent use.
func WithClock(now func() time.Time) Option {
	return func(l *Limiter) {
		l.now = now
	}
}

// New creates a limiter with valid configuration.
func New(config Config, options ...Option) (*Limiter, error) {
	if config.Limit <= 0 {
		return nil, fmt.Errorf("fixedwindow: limit must be positive")
	}

	if config.Window <= 0 {
		return nil, fmt.Errorf("fixedwindow: window must be positive")
	}
	if config.MaxKeys <= 0 {
		return nil, fmt.Errorf("fixedwindow: max keys must be positive")
	}

	limiter := &Limiter{
		config:  config,
		now:     time.Now,
		windows: make(map[string]windowState),
	}

	for _, option := range options {
		if option == nil {
			return nil, fmt.Errorf("fixedwindow: option must not be nil")
		}
		option(limiter)
	}

	if limiter.now == nil {
		return nil, fmt.Errorf("fixedwindow: clock must not be nil")
	}

	return limiter, nil
}

// Allow evaluates one request for the given key.
func (l *Limiter) Allow(key string) Decision {
	d, _ := l.allow(context.Background(), key)
	return d
}

func (l *Limiter) allow(ctx context.Context, key string) (Decision, error) {
	if len(key) > MaxKeyBytes || strings.TrimSpace(key) == "" {
		return Decision{Reason: ReasonInvalidKey}, nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Decision{}, err
	}
	now := l.nowLocked()
	windowStart := now.Truncate(l.config.Window)
	// All counters share a boundary. Retire the entire map instead of scanning
	// every identity while holding the request mutex.
	if !l.windowSet || !l.activeWindow.Equal(windowStart) {
		l.windows = make(map[string]windowState)
		l.activeWindow = windowStart
		l.windowSet = true
	}
	state, exists := l.windows[key]
	if !exists && len(l.windows) >= l.config.MaxKeys {
		return Decision{Reason: ReasonCapacityExceeded}, nil
	}

	if !exists {
		key = strings.Clone(key)
	} else {
		key = state.key
	}
	if !exists || !state.start.Equal(windowStart) {
		state = windowState{key: key, start: windowStart}
	}

	if state.count >= l.config.Limit {
		return Decision{
			Allowed:    false,
			Remaining:  0,
			RetryAfter: windowStart.Add(l.config.Window).Sub(now),
			Reason:     ReasonRateLimited,
		}, nil
	}

	state.count++
	l.windows[key] = state

	return Decision{
		Allowed:   true,
		Remaining: l.config.Limit - state.count,
	}, nil
}

// Evaluate adapts the in-memory limiter to context-aware services. Cancellation
// is checked before and after locking; acquiring the mutex is not interruptible.
func (l *Limiter) Evaluate(ctx context.Context, key string) (Decision, error) {
	if err := ctx.Err(); err != nil {
		return Decision{}, err
	}
	return l.allow(ctx, key)
}

// CleanupExpired removes keys whose windows have ended and returns the number
// removed. It is safe to call concurrently with Allow. The caller must invoke
// it periodically; the limiter does not start a background cleanup goroutine.
// Cleanup retires the expired map in constant time under the mutex.
func (l *Limiter) CleanupExpired() int {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.cleanupExpiredLocked(l.nowLocked())
}

// cleanupExpiredLocked retires expired state. The caller must hold mu.
func (l *Limiter) cleanupExpiredLocked(now time.Time) int {
	if l.windowSet && !now.Before(l.activeWindow.Add(l.config.Window)) {
		removed := len(l.windows)
		l.windows = make(map[string]windowState)
		return removed
	}
	return 0
}

// nowLocked returns nondecreasing wall-clock time. The caller must hold mu.
func (l *Limiter) nowLocked() time.Time {
	// Window alignment uses wall time, so compare wall time here too, even when
	// the supplied clock includes a monotonic reading (as time.Now does).
	now := l.now().Round(0)
	if l.clockSet && now.Before(l.lastNow) {
		return l.lastNow
	}
	l.lastNow = now
	l.clockSet = true
	return now
}

// Usage reports the fraction of this tenant's identity budget currently used.
func (l *Limiter) Usage() float64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.windowSet && !l.nowLocked().Before(l.activeWindow.Add(l.config.Window)) {
		return 0
	}
	return float64(len(l.windows)) / float64(l.config.MaxKeys)
}
