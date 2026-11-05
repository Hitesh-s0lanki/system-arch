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
	mu                sync.Mutex
	config            Config
	now               func() time.Time
	windows           map[string]windowState
	lastNow           time.Time
	clockSet          bool
	capacityFull      bool
	capacityFullUntil time.Time
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
	if len(key) > MaxKeyBytes || strings.TrimSpace(key) == "" {
		return Decision{Reason: ReasonInvalidKey}
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.nowLocked()
	windowStart := now.Truncate(l.config.Window)
	state, exists := l.windows[key]
	if !exists && len(l.windows) >= l.config.MaxKeys {
		// A full scan already established that all entries expire no earlier
		// than this boundary. Avoid scanning again for every unknown key.
		if l.capacityFull && now.Before(l.capacityFullUntil) {
			return Decision{Reason: ReasonCapacityExceeded}
		}
		l.cleanupExpiredLocked(now)
		if len(l.windows) >= l.config.MaxKeys {
			l.capacityFull = true
			l.capacityFullUntil = windowStart.Add(l.config.Window)
			// No active key is evicted and no state is stored for this key.
			return Decision{Reason: ReasonCapacityExceeded}
		}
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
		}
	}

	state.count++
	l.windows[key] = state

	return Decision{
		Allowed:   true,
		Remaining: l.config.Limit - state.count,
	}
}

// Evaluate adapts the in-memory limiter to context-aware services. Cancellation
// is checked before Allow; acquiring Allow's mutex is not interruptible.
func (l *Limiter) Evaluate(ctx context.Context, key string) (Decision, error) {
	if err := ctx.Err(); err != nil {
		return Decision{}, err
	}
	return l.Allow(key), nil
}

// CleanupExpired removes keys whose windows have ended and returns the number
// removed. It is safe to call concurrently with Allow. The caller must invoke
// it periodically; the limiter does not start a background cleanup goroutine.
// Cleanup scans all tracked keys while holding the limiter's mutex.
func (l *Limiter) CleanupExpired() int {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.cleanupExpiredLocked(l.nowLocked())
}

// cleanupExpiredLocked scans tracked keys. The caller must hold mu.
func (l *Limiter) cleanupExpiredLocked(now time.Time) int {
	removed := 0
	for key, state := range l.windows {
		if !now.Before(state.start.Add(l.config.Window)) {
			delete(l.windows, key)
			removed++
		}
	}
	return removed
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
