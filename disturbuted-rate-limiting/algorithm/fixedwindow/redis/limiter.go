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
}

// Limiter uses one bounded hash per policy. Redis owns the clock and expiration.
// The caller owns the client and must disable automatic command retries.
type Limiter struct {
	client    *redisclient.Client
	config    Config
	key       string
	signature string
}

func New(client *redisclient.Client, config Config) (*Limiter, error) {
	if client == nil || strings.TrimSpace(config.PolicyID) == "" || len(config.PolicyID) > 64 || strings.TrimSpace(config.Namespace) == "" || len(config.Namespace) > 128 || strings.ContainsAny(config.Namespace, "{}") {
		return nil, fmt.Errorf("redis limiter: client, policy ID, and bounded namespace required")
	}
	if config.Limit <= 0 || int64(config.Limit) > 2147483647 || config.MaxKeys <= 0 || int64(config.MaxKeys) > 2147483647 || config.Window < time.Millisecond || config.Window%time.Millisecond != 0 || config.Timeout <= 0 {
		return nil, fmt.Errorf("redis limiter: positive bounded limits, millisecond-aligned window, and timeout required")
	}
	if client.Options().MaxRetries != 0 {
		return nil, fmt.Errorf("redis limiter: disable automatic retries with MaxRetries: -1")
	}
	sum := sha256.Sum256([]byte(config.PolicyID))
	return &Limiter{client: client, config: config, key: config.Namespace + ":fw:{" + hex.EncodeToString(sum[:]) + "}",
		signature: fmt.Sprintf("%d:%d:%d", config.Limit, config.Window.Milliseconds(), config.MaxKeys)}, nil
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
	values, err := script.Run(ctx, l.client, []string{l.key}, key, l.config.Limit, l.config.Window.Milliseconds(), l.config.MaxKeys, l.signature).Int64Slice()
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
