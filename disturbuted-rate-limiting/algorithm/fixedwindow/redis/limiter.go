// Package redis implements fixed-window decisions against a Redis primary.
package redis

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	ratelimit "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/fixedwindow"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/internal/redisutil"
	redisclient "github.com/redis/go-redis/v9"
)

//go:embed allow.lua
var source string
var script = redisclient.NewScript(source)

// Config must be identical on replicas sharing a namespace and policy ID.
type Config struct {
	PolicyID  string
	Namespace string
	Limit     int
	Window    time.Duration
	MaxKeys   int
	Timeout   time.Duration
	// Revision intentionally creates fresh quota state. Use coordinated cutovers.
	Revision  string
	TenantID  string
	Semantics string
}

// Limiter uses one bounded hash per policy. Redis owns the clock and expiration.
// The caller owns the client and must disable automatic command retries.
type Limiter struct {
	client    redisclient.UniversalClient
	config    Config
	key       string
	signature string
}

func New(client redisclient.UniversalClient, config Config) (*Limiter, error) {
	if client == nil || strings.TrimSpace(config.PolicyID) == "" || len(config.PolicyID) > 64 || strings.TrimSpace(config.Namespace) == "" || len(config.Namespace) > 128 || strings.ContainsAny(config.Namespace, "{}") {
		return nil, fmt.Errorf("redis limiter: client, policy ID, and bounded namespace required")
	}
	if config.Limit <= 0 || int64(config.Limit) > 2147483647 || config.MaxKeys <= 0 || int64(config.MaxKeys) > 2147483647 || config.Window < time.Millisecond || config.Window%time.Millisecond != 0 || config.Timeout <= 0 {
		return nil, fmt.Errorf("redis limiter: positive bounded limits, millisecond-aligned window, and timeout required")
	}
	retries := -1
	switch c := client.(type) {
	case *redisclient.Client:
		if c == nil {
			return nil, fmt.Errorf("redis limiter: client required")
		}
		retries = c.Options().MaxRetries
	case *redisclient.ClusterClient:
		if c == nil {
			return nil, fmt.Errorf("redis limiter: client required")
		}
		retries = c.Options().MaxRetries
		if retries < 0 {
			retries = 0
		}
	}
	if retries != 0 {
		return nil, fmt.Errorf("redis limiter: disable automatic retries with MaxRetries: -1")
	}
	if len(config.Revision) > 64 || len(config.TenantID) > 128 || len(config.Semantics) > 256 {
		return nil, fmt.Errorf("redis limiter: scope too long")
	}
	scope := config.PolicyID
	if config.Revision != "" || config.TenantID != "" {
		scope = fmt.Sprintf("%d:%s%d:%s%d:%s", len(config.PolicyID), config.PolicyID, len(config.Revision), config.Revision, len(config.TenantID), config.TenantID)
	}
	sum := sha256.Sum256([]byte(scope))
	return &Limiter{client: client, config: config, key: config.Namespace + ":fw:{" + hex.EncodeToString(sum[:]) + "}",
		signature: fmt.Sprintf("%d:%d:%d:%s", config.Limit, config.Window.Milliseconds(), config.MaxKeys, config.Semantics)}, nil
}

func (l *Limiter) Evaluate(ctx context.Context, key string) (ratelimit.Decision, error) {
	if err := ctx.Err(); err != nil {
		return ratelimit.Decision{}, err
	}
	if len(key) > fixedwindow.MaxKeyBytes || strings.TrimSpace(key) == "" {
		return ratelimit.Decision{Reason: ratelimit.ReasonInvalidKey}, nil
	}
	ctx, cancel := context.WithTimeout(ctx, l.config.Timeout)
	defer cancel()
	values, err := redisutil.RunOnce(ctx, l.client, script, source, []string{l.key, l.key + ":config"}, key, l.config.Limit, l.config.Window.Milliseconds(), l.config.MaxKeys, l.signature).Int64Slice()
	if err != nil {
		return ratelimit.Decision{}, fmt.Errorf("redis limiter: decision failed: %w", err)
	}
	if len(values) != 3 {
		return ratelimit.Decision{}, fmt.Errorf("redis limiter: invalid script result")
	}
	switch values[0] {
	case 1:
		return ratelimit.Decision{Allowed: true, Remaining: int(values[1])}, nil
	case 0:
		return ratelimit.Decision{Reason: ratelimit.ReasonRateLimited, RetryAfter: time.Duration(values[2]) * time.Millisecond}, nil
	case -1:
		return ratelimit.Decision{Reason: ratelimit.ReasonCapacityExceeded}, nil
	default:
		return ratelimit.Decision{}, fmt.Errorf("redis limiter: invalid decision")
	}
}

// Ready checks policy compatibility and write capability without charging quota.
// Probe keys share the policy hash slot for Cluster and expire after ten seconds.
var readinessScript = redisclient.NewScript(`
redis.call('SET', KEYS[3], ARGV[1], 'NX')
if redis.call('GET', KEYS[3]) ~= ARGV[1] then return redis.error_reply('policy configuration mismatch') end
local config = redis.call('HGET', KEYS[1], '_config')
if config and config ~= ARGV[1] then return redis.error_reply('policy configuration mismatch') end
redis.call('TIME')
redis.call('HLEN', KEYS[1])
redis.call('HSET', KEYS[2], '_probe', '1')
redis.call('PEXPIRE', KEYS[2], 10000)
return 1`)

func (l *Limiter) Ready(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, l.config.Timeout)
	defer cancel()
	return readinessScript.Run(ctx, l.client, []string{l.key, l.key + ":probe", l.key + ":config"}, l.signature).Err()
}

func (l *Limiter) Usage(ctx context.Context) (float64, error) {
	ctx, cancel := context.WithTimeout(ctx, l.config.Timeout)
	defer cancel()
	count, err := l.client.HLen(ctx, l.key).Result()
	if count < 3 {
		count = 3
	}
	return float64(count-3) / float64(l.config.MaxKeys), err
}
