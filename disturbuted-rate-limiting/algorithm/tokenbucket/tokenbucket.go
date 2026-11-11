// Package tokenbucket allows bursts while enforcing a continuous refill rate.
package tokenbucket

import (
	"context"
	"fmt"
	"math"
	"math/bits"
	"strings"
	"sync"
	"time"

	ratelimit "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting"
)

const MaxKeyBytes = 256

// Config refills Limit tokens per Window, up to Capacity tokens. Each request
// costs one token. MaxKeys bounds retained identities, including idle buckets.
type Config struct {
	Limit    int
	Window   time.Duration
	Capacity int
	MaxKeys  int
}

// Validate checks the rate, burst, and storage bounds. Full refill must fit in a duration.
func (c Config) Validate() error {
	if c.Limit <= 0 || c.Capacity <= 0 || c.MaxKeys <= 0 || c.Window <= 0 ||
		int64(c.Limit) > 2147483647 || int64(c.Capacity) > 2147483647 {
		return fmt.Errorf("tokenbucket: positive bounded rate, capacity, window, and max keys required; full refill must fit in time.Duration")
	}
	hi, lo := bits.Mul64(uint64(c.Capacity), uint64(c.Window))
	if hi >= uint64(c.Limit) {
		return fmt.Errorf("tokenbucket: full refill exceeds time.Duration")
	}
	q, r := bits.Div64(hi, lo, uint64(c.Limit))
	if q > math.MaxInt64 || (q == math.MaxInt64 && r > 0) {
		return fmt.Errorf("tokenbucket: full refill exceeds time.Duration")
	}
	return nil
}

type bucket struct {
	key      string
	tokens   uint64
	fraction uint64
	updated  time.Time
}
type Option func(*Limiter)

// WithClock supplies a non-nil clock called under the limiter lock.
func WithClock(now func() time.Time) Option { return func(l *Limiter) { l.now = now } }

// Limiter is safe for concurrent use. Do not copy after use. No workers are started.
type Limiter struct {
	mu       sync.Mutex
	config   Config
	now      func() time.Time
	last     time.Time
	clockSet bool
	buckets  map[string]bucket
}

func New(config Config, options ...Option) (*Limiter, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	l := &Limiter{config: config, now: time.Now, buckets: make(map[string]bucket)}
	for _, option := range options {
		if option == nil {
			return nil, fmt.Errorf("tokenbucket: nil option")
		}
		option(l)
	}
	if l.now == nil {
		return nil, fmt.Errorf("tokenbucket: nil clock")
	}
	return l, nil
}
func (l *Limiter) nowLocked() time.Time {
	now := l.now()
	if l.clockSet && now.Before(l.last) {
		return l.last
	}
	l.last, l.clockSet = now, true
	return now
}

// refill keeps the exact fractional numerator in units of 1/Window tokens.
// A 128-bit intermediate avoids overflow for large rates and elapsed durations.
func (l *Limiter) refill(b bucket, now time.Time) bucket {
	hi, lo := bits.Mul64(uint64(now.Sub(b.updated)), uint64(l.config.Limit))
	lo, carry := bits.Add64(lo, b.fraction, 0)
	hi += carry
	window := uint64(l.config.Window)
	capacity := uint64(l.config.Capacity)
	if hi >= window {
		b.tokens = capacity
		b.fraction = 0
		return b
	}
	whole, fraction := bits.Div64(hi, lo, window)
	if whole >= capacity-b.tokens {
		b.tokens = capacity
		b.fraction = 0
	} else {
		b.tokens += whole
		b.fraction = fraction
	}
	return b
}
func (l *Limiter) Allow(key string) ratelimit.Decision {
	d, _ := l.Evaluate(context.Background(), key)
	return d
}

// Evaluate atomically refills and consumes one token, rejecting without waiting.
// Remaining is the number of whole tokens left; RetryAfter rounds up to a nanosecond.
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
	b, exists := l.buckets[key]
	if !exists {
		if len(l.buckets) >= l.config.MaxKeys {
			l.cleanupLocked(now)
			if len(l.buckets) >= l.config.MaxKeys {
				return ratelimit.Decision{Reason: ratelimit.ReasonCapacityExceeded}, nil
			}
		}
		b = bucket{key: strings.Clone(key), tokens: uint64(l.config.Capacity), updated: now}
	}
	b = l.refill(b, now)
	b.updated = now
	d := ratelimit.Decision{}
	if b.tokens >= 1 {
		b.tokens--
		d.Allowed = true
		d.Remaining = int(b.tokens)
	} else {
		d.Reason = ratelimit.ReasonRateLimited
		numerator := uint64(l.config.Window) - b.fraction
		rate := uint64(l.config.Limit)
		d.RetryAfter = time.Duration(numerator / rate)
		if numerator%rate != 0 {
			d.RetryAfter++
		}
		if d.RetryAfter < time.Nanosecond {
			d.RetryAfter = time.Nanosecond
		}
	}
	l.buckets[b.key] = b
	return d, nil
}
func (l *Limiter) cleanupLocked(now time.Time) int {
	removed := 0
	for key, b := range l.buckets {
		if l.refill(b, now).tokens >= uint64(l.config.Capacity) {
			delete(l.buckets, key)
			removed++
		}
	}
	return removed
}

// CleanupExpired removes only fully refilled buckets, preserving quota on recreation.
// It scans tracked identities; schedule it periodically to reclaim idle storage.
func (l *Limiter) CleanupExpired() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.cleanupLocked(l.nowLocked())
}
func (l *Limiter) Usage() float64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cleanupLocked(l.nowLocked())
	return float64(len(l.buckets)) / float64(l.config.MaxKeys)
}
