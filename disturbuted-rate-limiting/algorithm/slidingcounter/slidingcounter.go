// Package slidingcounter estimates rolling usage with two aligned window counters.
package slidingcounter

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	ratelimit "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/fixedwindow"
)

type Config = fixedwindow.Config

const MaxKeyBytes = fixedwindow.MaxKeyBytes

// Validate bounds integer products to values represented exactly by Redis Lua.
// Both backends use epoch-aligned windows and millisecond precision.
func Validate(c Config) error {
	if c.Limit <= 0 || int64(c.Limit) > 2147483647 || c.MaxKeys <= 0 || int64(c.MaxKeys) > 2147483647 || c.Window > time.Duration(1<<63-1)/2 || c.Window < time.Millisecond || c.Window%time.Millisecond != 0 || int64(c.Limit) > (1<<52)/c.Window.Milliseconds() {
		return fmt.Errorf("slidingcounter: positive bounded limits and millisecond-aligned window required; limit * window milliseconds must be <= 2^52")
	}
	return nil
}

type counter struct {
	key                      string
	start, previous, current int64
}
type Option func(*Limiter)

func WithClock(now func() time.Time) Option { return func(l *Limiter) { l.now = now } }

// Limiter is safe for concurrent use, starts no workers, and must not be copied.
type Limiter struct {
	mu       sync.Mutex
	config   Config
	now      func() time.Time
	last     int64
	clockSet bool
	counters map[string]counter
}

func New(config Config, options ...Option) (*Limiter, error) {
	if err := Validate(config); err != nil {
		return nil, err
	}
	l := &Limiter{config: config, now: time.Now, counters: make(map[string]counter)}
	for _, o := range options {
		if o == nil {
			return nil, fmt.Errorf("slidingcounter: nil option")
		}
		o(l)
	}
	if l.now == nil {
		return nil, fmt.Errorf("slidingcounter: nil clock")
	}
	return l, nil
}
func (l *Limiter) nowLocked() int64 {
	now := l.now().UnixMilli()
	if l.clockSet && now < l.last {
		return l.last
	}
	l.last, l.clockSet = now, true
	return now
}
func startOf(now, window int64) int64 {
	remainder := now % window
	if remainder < 0 {
		remainder += window
	}
	return now - remainder
}
func (l *Limiter) Allow(key string) ratelimit.Decision {
	d, _ := l.Evaluate(context.Background(), key)
	return d
}

// Evaluate admits only when the weighted estimate plus one is at most Limit.
// Rejections consume no quota. Remaining is rounded down; RetryAfter rounds up.
func (l *Limiter) Evaluate(ctx context.Context, key string) (ratelimit.Decision, error) {
	if err := ctx.Err(); err != nil {
		return ratelimit.Decision{}, err
	}
	if len(key) > MaxKeyBytes || strings.TrimSpace(key) == "" {
		return ratelimit.Decision{Reason: ratelimit.ReasonInvalidKey}, nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return ratelimit.Decision{}, err
	}
	now := l.nowLocked()
	window := l.config.Window.Milliseconds()
	start := startOf(now, window)
	b, exists := l.counters[key]
	if !exists {
		if len(l.counters) >= l.config.MaxKeys {
			l.cleanupLocked(now)
			if len(l.counters) >= l.config.MaxKeys {
				return ratelimit.Decision{Reason: ratelimit.ReasonCapacityExceeded}, nil
			}
		}
		b = counter{key: strings.Clone(key), start: start}
	}
	if start > b.start {
		b.previous = 0
		if start-b.start == window {
			b.previous = b.current
		}
		b.current, b.start = 0, start
	}
	elapsed := now - start
	limit := int64(l.config.Limit)
	used := b.current*window + b.previous*(window-elapsed)
	if used+window > limit*window {
		var retry int64
		if b.current >= limit {
			retry = window - elapsed + (window+b.current-1)/b.current
		} else {
			deficit := used - (limit-1)*window
			retry = (deficit + b.previous - 1) / b.previous
		}
		return ratelimit.Decision{Reason: ratelimit.ReasonRateLimited, RetryAfter: time.Duration(retry) * time.Millisecond}, nil
	}
	b.current++
	l.counters[b.key] = b
	return ratelimit.Decision{Allowed: true, Remaining: int((limit*window - used - window) / window)}, nil
}
func (l *Limiter) cleanupLocked(now int64) int {
	removed := 0
	window := l.config.Window.Milliseconds()
	for key, b := range l.counters {
		if now >= b.start+2*window {
			delete(l.counters, key)
			removed++
		}
	}
	return removed
}

// CleanupExpired reclaims identities after both counters cease contributing.
func (l *Limiter) CleanupExpired() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.cleanupLocked(l.nowLocked())
}
func (l *Limiter) Usage() float64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cleanupLocked(l.nowLocked())
	return float64(len(l.counters)) / float64(l.config.MaxKeys)
}
