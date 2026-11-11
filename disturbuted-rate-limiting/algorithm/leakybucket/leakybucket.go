// Package leakybucket implements a continuously draining bucket with immediate admission decisions.
package leakybucket

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	ratelimit "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting"
)

// MaxKeyBytes bounds identity keys in bytes.
const MaxKeyBytes = 256

// Config drains Limit requests per Window from a bucket of Capacity requests.
// Each accepted request adds one unit of load. MaxKeys bounds retained identities, including idle buckets.
type Config struct {
	Limit    int
	Window   time.Duration
	Capacity int
	MaxKeys  int
}

// Validate checks the rate, burst, and storage bounds. Full drainage must fit in a duration.
func (c Config) Validate() error {
	if c.Limit <= 0 || c.Capacity <= 0 || c.MaxKeys <= 0 || c.Window <= 0 ||
		int64(c.Limit) > 2147483647 || int64(c.Capacity) > 2147483647 ||
		float64(c.Capacity)*float64(c.Window)/float64(c.Limit) >= float64(math.MaxInt64) {
		return fmt.Errorf("leakybucket: positive bounded rate, capacity, window, and max keys required; full drainage must fit in time.Duration")
	}
	return nil
}

type bucket struct {
	key string
	// level is measured in Window units to avoid fractional-request rounding
	// during drainage at common rates. One request adds float64(Window).
	level   float64
	updated time.Time
}

// Option configures a limiter during construction.
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

// New validates configuration and creates a limiter without background workers.
func New(config Config, options ...Option) (*Limiter, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	l := &Limiter{config: config, now: time.Now, buckets: make(map[string]bucket)}
	for _, option := range options {
		if option == nil {
			return nil, fmt.Errorf("leakybucket: nil option")
		}
		option(l)
	}
	if l.now == nil {
		return nil, fmt.Errorf("leakybucket: nil clock")
	}
	return l, nil
}
func (l *Limiter) nowLocked() time.Time {
	now := l.now().Round(0)
	if l.clockSet && now.Before(l.last) {
		return l.last
	}
	l.last, l.clockSet = now, true
	return now
}
func (l *Limiter) drain(b bucket, now time.Time) float64 {
	return math.Max(0, b.level-float64(now.Sub(b.updated))*float64(l.config.Limit))
}

// Allow checks a key using a background context.
func (l *Limiter) Allow(key string) ratelimit.Decision {
	d, _ := l.Evaluate(context.Background(), key)
	return d
}

// Evaluate atomically drains the bucket and adds one request, rejecting without waiting.
// It does not queue requests or pace accepted requests to the upstream.
// Remaining is the number of whole requests that still fit; RetryAfter rounds up to a nanosecond.
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
		b = bucket{key: strings.Clone(key), level: 0, updated: now}
	}
	b.level = l.drain(b, now)
	b.updated = now
	d := ratelimit.Decision{}
	if b.level+float64(l.config.Window) <= float64(l.config.Capacity)*float64(l.config.Window) {
		b.level += float64(l.config.Window)
		d.Allowed = true
		d.Remaining = int(math.Floor(float64(l.config.Capacity) - b.level/float64(l.config.Window)))
	} else {
		d.Reason = ratelimit.ReasonRateLimited
		d.RetryAfter = time.Duration(math.Ceil((b.level + float64(l.config.Window) - float64(l.config.Capacity)*float64(l.config.Window)) / float64(l.config.Limit)))
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
		if l.drain(b, now) <= 0 {
			delete(l.buckets, key)
			removed++
		}
	}
	return removed
}

// CleanupExpired removes only fully drained buckets, preserving quota on recreation.
// It scans tracked identities; schedule it periodically to reclaim idle storage.
func (l *Limiter) CleanupExpired() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.cleanupLocked(l.nowLocked())
}

// Usage removes drained buckets and reports the fraction of identity capacity used.
func (l *Limiter) Usage() float64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cleanupLocked(l.nowLocked())
	return float64(len(l.buckets)) / float64(l.config.MaxKeys)
}
