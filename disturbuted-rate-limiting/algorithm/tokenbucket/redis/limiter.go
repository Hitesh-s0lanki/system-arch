// Package redis implements an atomic token bucket against a Redis primary.
package redis

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"fmt"
	"strings"
	"time"

	ratelimit "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting"
	fixedredis "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/fixedwindow/redis"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/tokenbucket"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/internal/redisutil"
	redisclient "github.com/redis/go-redis/v9"
)

// Config follows the same scope, timeout and millisecond precision rules as fixed window.
type Config struct {
	fixedredis.Config
	Capacity int
}

//go:embed allow.lua
var source string
var script = redisclient.NewScript(source)
var readyScript = redisclient.NewScript(readyScriptSourceText)

// Limiter stores bounded token balances shared by replicas. The caller owns the client.
type Limiter struct {
	client    redisclient.UniversalClient
	config    Config
	keys      []string
	signature string
}

// New makes no network calls. The caller owns the client and must disable retries,
// enable ContextTimeoutEnabled, and configure bounded connection timeouts.
func New(client redisclient.UniversalClient, config Config) (*Limiter, error) {
	// Reuse shared backend configuration and retry-safety validation.
	if _, err := fixedredis.New(client, config.Config); err != nil {
		return nil, fmt.Errorf("tokenbucket: %w", err)
	}
	if err := (tokenbucket.Config{Limit: config.Limit, Window: config.Window, Capacity: config.Capacity, MaxKeys: config.MaxKeys}).Validate(); err != nil {
		return nil, err
	}
	// Lua numbers are doubles: bound the fractional-credit intermediate to exact integers.
	if config.Window.Milliseconds() > (1<<53)/int64(config.Limit+1) {
		return nil, fmt.Errorf("tokenbucket redis: window * (limit + 1) exceeds exact integer precision")
	}
	scope := fmt.Sprintf("%d:%s%d:%s%d:%s", len(config.PolicyID), config.PolicyID, len(config.Revision), config.Revision, len(config.TenantID), config.TenantID)
	sum := sha256.Sum256([]byte(scope))
	base := fmt.Sprintf("%s:tb:{%x}", config.Namespace, sum)
	return &Limiter{client: client, config: config, keys: []string{base, base + ":expiry", base + ":config"}, signature: fmt.Sprintf("token_bucket:v2:%d:%d:%d:%d:%s", config.Limit, config.Window.Milliseconds(), config.Capacity, config.MaxKeys, config.Semantics)}, nil
}
func (l *Limiter) run(ctx context.Context, operation, key string) ([]int64, error) {
	ctx, cancel := context.WithTimeout(ctx, l.config.Timeout)
	defer cancel()
	values, err := redisutil.RunOnce(ctx, l.client, script, source, l.keys, operation, key, l.config.Window.Milliseconds(), l.config.Limit, l.signature, l.config.MaxKeys, l.config.Capacity).Int64Slice()
	if err != nil {
		return nil, fmt.Errorf("tokenbucket redis: %w", err)
	}
	if len(values) != 3 {
		return nil, fmt.Errorf("tokenbucket redis: invalid script result")
	}
	return values, nil
}

// Evaluate atomically reclaims full buckets, refills, and consumes one token.
func (l *Limiter) Evaluate(ctx context.Context, key string) (ratelimit.Decision, error) {
	if err := ctx.Err(); err != nil {
		return ratelimit.Decision{}, err
	}
	if len(key) > tokenbucket.MaxKeyBytes || strings.TrimSpace(key) == "" {
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
		return ratelimit.Decision{}, fmt.Errorf("tokenbucket redis: invalid decision")
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
