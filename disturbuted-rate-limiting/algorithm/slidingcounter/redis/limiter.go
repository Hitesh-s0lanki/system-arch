// Package redis implements an atomic sliding-window counter against a Redis primary.
package redis

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"fmt"
	ratelimit "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting"
	fixedredis "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/fixedwindow/redis"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/slidingcounter"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/internal/redisutil"
	redisclient "github.com/redis/go-redis/v9"
	"strings"
	"time"
)

// Config follows the same scope, timeout and millisecond precision rules as fixed window.
type Config = fixedredis.Config

//go:embed allow.lua
var source string
var script = redisclient.NewScript(source)
var readyScript = redisclient.NewScript(readyScriptSourceText)

// Limiter stores bounded two-window counters shared by replicas. The caller owns the client.
type Limiter struct {
	client    redisclient.UniversalClient
	config    Config
	keys      []string
	signature string
}

// New makes no network calls. The caller owns the client and must disable retries,
// enable ContextTimeoutEnabled, and configure bounded connection timeouts.
func New(client redisclient.UniversalClient, config Config) (*Limiter, error) {
	if err := slidingcounter.Validate(slidingcounter.Config{Limit: config.Limit, Window: config.Window, MaxKeys: config.MaxKeys}); err != nil {
		return nil, err
	}
	// Reuse shared backend configuration and retry-safety validation.
	if _, err := fixedredis.New(client, config); err != nil {
		return nil, fmt.Errorf("slidingcounter: %w", err)
	}
	scope := fmt.Sprintf("%d:%s%d:%s%d:%s", len(config.PolicyID), config.PolicyID, len(config.Revision), config.Revision, len(config.TenantID), config.TenantID)
	sum := sha256.Sum256([]byte(scope))
	base := fmt.Sprintf("%s:sc:{%x}", config.Namespace, sum)
	return &Limiter{client: client, config: config, keys: []string{base, base + ":expiry", base + ":config"}, signature: fmt.Sprintf("sliding_window_counter:%d:%d:%d:%s", config.Limit, config.Window.Milliseconds(), config.MaxKeys, config.Semantics)}, nil
}
func (l *Limiter) run(ctx context.Context, operation, key string) ([]int64, error) {
	ctx, cancel := context.WithTimeout(ctx, l.config.Timeout)
	defer cancel()
	values, err := redisutil.RunOnce(ctx, l.client, script, source, l.keys, operation, key, l.config.Window.Milliseconds(), l.config.Limit, l.signature, l.config.MaxKeys).Int64Slice()
	if err != nil {
		return nil, fmt.Errorf("slidingcounter redis: %w", err)
	}
	if len(values) != 3 {
		return nil, fmt.Errorf("slidingcounter redis: invalid script result")
	}
	return values, nil
}

// Evaluate atomically rotates counters, checks capacity and allowance, and records admission.
func (l *Limiter) Evaluate(ctx context.Context, key string) (ratelimit.Decision, error) {
	if err := ctx.Err(); err != nil {
		return ratelimit.Decision{}, err
	}
	if len(key) > slidingcounter.MaxKeyBytes || strings.TrimSpace(key) == "" {
		return ratelimit.Decision{Reason: ratelimit.ReasonInvalidKey}, nil
	}
	values, err := l.run(ctx, "allow", key)
	if err != nil {
		return ratelimit.Decision{}, err
	}
	switch values[0] {
	case 1:
		return ratelimit.Decision{Allowed: true, Remaining: int(values[1])}, nil
	case 0:
		return ratelimit.Decision{Reason: ratelimit.ReasonRateLimited, RetryAfter: time.Duration(values[2]) * time.Millisecond}, nil
	case -1:
		return ratelimit.Decision{Reason: ratelimit.ReasonCapacityExceeded}, nil
	default:
		return ratelimit.Decision{}, fmt.Errorf("slidingcounter redis: invalid decision")
	}
}

// Ready verifies policy compatibility and read/write access without charging quota.
func (l *Limiter) Ready(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, l.config.Timeout)
	defer cancel()
	keys := append(append([]string{}, l.keys...), l.keys[0]+":probe")
	return redisutil.RunOnce(ctx, l.client, readyScript, readyScriptSourceText, keys, l.signature).Err()
}

// The script source is passed explicitly for NOSCRIPT fallback.
const readyScriptSourceText = `
redis.call('SET', KEYS[3], ARGV[1], 'NX')
if redis.call('GET', KEYS[3]) ~= ARGV[1] then return redis.error_reply('policy configuration mismatch') end
redis.call('TIME')
redis.call('HLEN', KEYS[1])
redis.call('ZCARD', KEYS[2])
redis.call('SET', KEYS[4], '1', 'PX', 10000)
return 1`

// Usage removes expired identities and reports the fraction of active capacity.
func (l *Limiter) Usage(ctx context.Context) (float64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	values, err := l.run(ctx, "usage", "")
	if err != nil {
		return 0, err
	}
	return float64(values[0]) / float64(l.config.MaxKeys), nil
}
