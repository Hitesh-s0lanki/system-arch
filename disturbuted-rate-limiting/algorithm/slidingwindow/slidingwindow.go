// Package slidingwindow implements an exact rolling-window log of accepted requests.
package slidingwindow

import (
	"container/heap"
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	ratelimit "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/fixedwindow"
)

// Config bounds the rolling interval, accepted requests, and tracked identities.
type Config = fixedwindow.Config

const MaxKeyBytes = fixedwindow.MaxKeyBytes

// Option configures a limiter during construction.
type Option func(*Limiter)

// WithClock supplies a non-nil clock, called while the limiter mutex is held.
func WithClock(now func() time.Time) Option { return func(l *Limiter) { l.now = now } }

type logState struct {
	key     string
	stamps  []time.Time
	expires time.Time
	index   int
}

// expiryQueue has one mutable entry per identity, so repeated requests do not
// accumulate stale heap entries. Its root is the next identity that can retire.
type expiryQueue []*logState

func (q expiryQueue) Len() int           { return len(q) }
func (q expiryQueue) Less(i, j int) bool { return q[i].expires.Before(q[j].expires) }
func (q expiryQueue) Swap(i, j int)      { q[i], q[j] = q[j], q[i]; q[i].index = i; q[j].index = j }
func (q *expiryQueue) Push(x any) {
	state := x.(*logState)
	state.index = len(*q)
	*q = append(*q, state)
}
func (q *expiryQueue) Pop() any {
	old := *q
	n := len(old)
	state := old[n-1]
	old[n-1] = nil
	*q = old[:n-1]
	return state
}

// Limiter counts accepted timestamps in (now-Window, now]. Do not copy after use.
// State is bounded by MaxKeys identities and Limit active timestamps per identity.
type Limiter struct {
	mu       sync.Mutex
	config   Config
	now      func() time.Time
	last     time.Time
	clockSet bool
	logs     map[string]*logState
	expiry   expiryQueue
}

// New validates configuration and creates a limiter without background workers.
func New(config Config, options ...Option) (*Limiter, error) {
	if config.Limit <= 0 || config.Window <= 0 || config.MaxKeys <= 0 {
		return nil, fmt.Errorf("slidingwindow: positive limit, window, and max keys required")
	}
	l := &Limiter{config: config, now: time.Now, logs: make(map[string]*logState)}
	for _, o := range options {
		if o == nil {
			return nil, fmt.Errorf("slidingwindow: nil option")
		}
		o(l)
	}
	if l.now == nil {
		return nil, fmt.Errorf("slidingwindow: nil clock")
	}
	return l, nil
}
func (l *Limiter) nowLocked() time.Time {
	now := l.now().Round(0)
	if l.clockSet && now.Before(l.last) {
		return l.last
	}
	l.last = now
	l.clockSet = true
	return now
}
func prune(log []time.Time, cutoff time.Time) []time.Time {
	i := 0
	for i < len(log) && !log[i].After(cutoff) {
		i++
	}
	if i > 0 {
		copy(log, log[i:])
		clear(log[len(log)-i:])
		log = log[:len(log)-i]
	}
	return log
}

// Allow checks one key using a background context. Rejections consume no quota.
func (l *Limiter) Allow(key string) ratelimit.Decision {
	d, _ := l.Evaluate(context.Background(), key)
	return d
}

// Evaluate checks cancellation before and after locking; mutex acquisition is
// not interruptible. Invalid keys consume neither capacity nor allowance.
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
	cutoff := now.Add(-l.config.Window)
	state, exists := l.logs[key]
	var log []time.Time
	if exists {
		log = state.stamps
	}
	if !exists && len(l.logs) >= l.config.MaxKeys {
		l.cleanupLocked(now)
		if len(l.logs) >= l.config.MaxKeys {
			return ratelimit.Decision{Reason: ratelimit.ReasonCapacityExceeded}, nil
		}
	}
	log = prune(log, cutoff)
	if len(log) >= l.config.Limit {
		return ratelimit.Decision{Reason: ratelimit.ReasonRateLimited, RetryAfter: log[0].Add(l.config.Window).Sub(now)}, nil
	}
	if !exists {
		state = &logState{key: strings.Clone(key)}
		l.logs[state.key] = state
	}
	state.stamps = append(log, now)
	state.expires = now.Add(l.config.Window)
	if exists {
		heap.Fix(&l.expiry, state.index)
	} else {
		heap.Push(&l.expiry, state)
	}
	return ratelimit.Decision{Allowed: true, Remaining: l.config.Limit - len(log) - 1}, nil
}
func (l *Limiter) cleanupLocked(now time.Time) int {
	removed := 0
	for len(l.expiry) > 0 && !l.expiry[0].expires.After(now) {
		state := heap.Pop(&l.expiry).(*logState)
		delete(l.logs, state.key)
		removed++
	}
	return removed
}

// CleanupExpired removes fully expired identities and returns their count.
// Partially expired logs are pruned when their identity next makes a request.
func (l *Limiter) CleanupExpired() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.cleanupLocked(l.nowLocked())
}

// Usage reports the active fraction of the identity capacity after pruning.
func (l *Limiter) Usage() float64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cleanupLocked(l.nowLocked())
	return float64(len(l.logs)) / float64(l.config.MaxKeys)
}
