// Package sdk provides an embedded rate limiter with policy and
// identity handling. Clients are safe for concurrent use and start no goroutines.
package sdk

import (
	"context"
	"fmt"
	"time"

	ratelimit "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/fixedwindow"
	rediswindow "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/fixedwindow/redis"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/leakybucket"
	redisleaky "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/leakybucket/redis"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/slidingcounter"
	rediscounter "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/slidingcounter/redis"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/slidingwindow"
	redissliding "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/slidingwindow/redis"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/tokenbucket"
	redistoken "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/tokenbucket/redis"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/policy"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/service"
	redisclient "github.com/redis/go-redis/v9"
)

// Policy binds identity grouping to an independent rate and capacity budget.
// MaxKeys is shared by all tenants using this policy; tenant IDs separate quotas,
// but do not reserve storage capacity for individual tenants.
type Policy struct {
	Policy         policy.Policy
	FixedWindow    fixedwindow.Config
	SlidingWindow  slidingwindow.Config
	SlidingCounter slidingcounter.Config
	LeakyBucket    leakybucket.Config
	TokenBucket    tokenbucket.Config
}

// RedisConfig must match across replicas sharing the same namespace and revision.
// A revision change creates fresh quota and requires a coordinated cutover.
type RedisConfig struct {
	Policies  []Policy
	Namespace string
	Revision  string
	Timeout   time.Duration
}

// Client owns immutable policy bindings. Use constructors before serving requests.
// The caller owns Redis connections and any periodic cleanup goroutine.
type Client struct {
	service *service.Service
	memory  []interface{ CleanupExpired() int }
	redis   []interface{ Ready(context.Context) error }
}

// NewMemory creates one in-process fixed-window limiter per policy. Replicas do not share quota.
// Options apply to each limiter (for example, a concurrency-safe test clock).
// Expired windows are reclaimed on the next check; CleanupExpired can release
// retained identities during idle periods.
func NewMemory(policies []Policy, options ...fixedwindow.Option) (*Client, error) {
	c := &Client{}
	bindings := make([]service.Binding, 0, len(policies))
	for _, p := range policies {
		limiter, err := fixedwindow.New(p.FixedWindow, options...)
		if err != nil {
			return nil, fmt.Errorf("sdk: policy %q: %w", p.Policy.ID, err)
		}
		c.memory = append(c.memory, limiter)
		bindings = append(bindings, service.Binding{Policy: p.Policy, Limiter: limiter})
	}
	var err error
	c.service, err = service.New(bindings)
	if err != nil {
		return nil, fmt.Errorf("sdk: %w", err)
	}
	return c, nil
}

// NewRedis creates shared fixed-window limiters without network calls. Call Ready at
// startup to verify Redis write access and policy compatibility. The caller must
// disable automatic retries (MaxRetries: -1), configure connection timeouts and
// ContextTimeoutEnabled, and close the client when its own lifecycle ends.
// Backend failures propagate from Check without a local fallback or retry.
func NewRedis(client redisclient.UniversalClient, config RedisConfig) (*Client, error) {
	c := &Client{}
	bindings := make([]service.Binding, 0, len(config.Policies))
	for _, p := range config.Policies {
		limiter, err := rediswindow.New(client, rediswindow.Config{
			PolicyID: p.Policy.ID, Namespace: config.Namespace, Revision: config.Revision,
			Limit: p.FixedWindow.Limit, Window: p.FixedWindow.Window,
			MaxKeys: p.FixedWindow.MaxKeys, Timeout: config.Timeout,
			Semantics: fmt.Sprintf("sdk:identity=%s;per_route=%t", p.Policy.Identity, p.Policy.PerRoute),
		})
		if err != nil {
			return nil, fmt.Errorf("sdk: policy %q: %w", p.Policy.ID, err)
		}
		c.redis = append(c.redis, limiter)
		bindings = append(bindings, service.Binding{Policy: p.Policy, Limiter: limiter})
	}
	var err error
	c.service, err = service.New(bindings)
	if err != nil {
		return nil, fmt.Errorf("sdk: %w", err)
	}
	return c, nil
}

// Check consumes one allowance for one application-selected policy. Resolve the
// tenant and subject through trusted application code. Invalid requests consume
// no quota. A rejection is a Decision with Allowed=false; errors indicate invalid
// input, cancellation, or backend failure. Multiple checks are not atomic.
// Client implements middleware/http.Checker and can be passed directly to Wrap.
func (c *Client) Check(ctx context.Context, policyID string, request service.Request) (ratelimit.Decision, error) {
	return c.service.Check(ctx, policyID, request)
}

// Ready checks Redis write capability and policy compatibility without consuming
// allowance. It returns the context error for a canceled in-memory client.
func (c *Client) Ready(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, limiter := range c.redis {
		if err := limiter.Ready(ctx); err != nil {
			return fmt.Errorf("sdk: readiness: %w", err)
		}
	}
	return nil
}

// CleanupExpired releases expired in-memory identities and returns their count.
// It is safe alongside Check. Redis expires state itself, so this is a no-op for
// Redis clients. No maintenance worker is required for correct quota decisions.
func (c *Client) CleanupExpired() int {
	removed := 0
	for _, limiter := range c.memory {
		removed += limiter.CleanupExpired()
	}
	return removed
}

// NewSlidingWindowMemory constructs rolling-window logs using Policy.SlidingWindow.
// Options apply to every policy. The client starts no background cleanup workers.
func NewSlidingWindowMemory(policies []Policy, options ...slidingwindow.Option) (*Client, error) {
	c := &Client{}
	bindings := make([]service.Binding, 0, len(policies))
	for _, p := range policies {
		l, err := slidingwindow.New(p.SlidingWindow, options...)
		if err != nil {
			return nil, fmt.Errorf("sdk: policy %q: %w", p.Policy.ID, err)
		}
		c.memory = append(c.memory, l)
		bindings = append(bindings, service.Binding{Policy: p.Policy, Limiter: l})
	}
	var err error
	c.service, err = service.New(bindings)
	if err != nil {
		return nil, fmt.Errorf("sdk: %w", err)
	}
	return c, nil
}

// NewSlidingWindowRedis constructs shared rolling-window logs using Policy.SlidingWindow.
// Connection ownership and retry/timeout requirements match NewRedis.
func NewSlidingWindowRedis(client redisclient.UniversalClient, config RedisConfig) (*Client, error) {
	c := &Client{}
	bindings := make([]service.Binding, 0, len(config.Policies))
	for _, p := range config.Policies {
		l, err := redissliding.New(client, redissliding.Config{
			PolicyID: p.Policy.ID, Namespace: config.Namespace, Revision: config.Revision,
			Limit: p.SlidingWindow.Limit, Window: p.SlidingWindow.Window, MaxKeys: p.SlidingWindow.MaxKeys, Timeout: config.Timeout,
			Semantics: fmt.Sprintf("sdk:identity=%s;per_route=%t", p.Policy.Identity, p.Policy.PerRoute),
		})
		if err != nil {
			return nil, fmt.Errorf("sdk: policy %q: %w", p.Policy.ID, err)
		}
		c.redis = append(c.redis, l)
		bindings = append(bindings, service.Binding{Policy: p.Policy, Limiter: l})
	}
	var err error
	c.service, err = service.New(bindings)
	if err != nil {
		return nil, fmt.Errorf("sdk: %w", err)
	}
	return c, nil
}

// NewTokenBucketMemory constructs token buckets using Policy.TokenBucket.
// Options apply to every policy. The client starts no background cleanup workers.
func NewTokenBucketMemory(policies []Policy, options ...tokenbucket.Option) (*Client, error) {
	c := &Client{}
	bindings := make([]service.Binding, 0, len(policies))
	for _, p := range policies {
		l, err := tokenbucket.New(p.TokenBucket, options...)
		if err != nil {
			return nil, fmt.Errorf("sdk: policy %q: %w", p.Policy.ID, err)
		}
		c.memory = append(c.memory, l)
		bindings = append(bindings, service.Binding{Policy: p.Policy, Limiter: l})
	}
	var err error
	c.service, err = service.New(bindings)
	if err != nil {
		return nil, fmt.Errorf("sdk: %w", err)
	}
	return c, nil
}

// NewTokenBucketRedis constructs shared token buckets using Policy.TokenBucket.
// Connection ownership and retry/timeout requirements match NewRedis.
func NewTokenBucketRedis(client redisclient.UniversalClient, config RedisConfig) (*Client, error) {
	c := &Client{}
	bindings := make([]service.Binding, 0, len(config.Policies))
	for _, p := range config.Policies {
		l, err := redistoken.New(client, redistoken.Config{Config: rediswindow.Config{
			PolicyID: p.Policy.ID, Namespace: config.Namespace, Revision: config.Revision,
			Limit: p.TokenBucket.Limit, Window: p.TokenBucket.Window, MaxKeys: p.TokenBucket.MaxKeys, Timeout: config.Timeout,
			Semantics: fmt.Sprintf("sdk:identity=%s;per_route=%t", p.Policy.Identity, p.Policy.PerRoute),
		}, Capacity: p.TokenBucket.Capacity})
		if err != nil {
			return nil, fmt.Errorf("sdk: policy %q: %w", p.Policy.ID, err)
		}
		c.redis = append(c.redis, l)
		bindings = append(bindings, service.Binding{Policy: p.Policy, Limiter: l})
	}
	var err error
	c.service, err = service.New(bindings)
	if err != nil {
		return nil, fmt.Errorf("sdk: %w", err)
	}
	return c, nil
}

// NewLeakyBucketMemory constructs leaky buckets using Policy.LeakyBucket.
// Options apply to every policy. The client starts no background cleanup workers.
func NewLeakyBucketMemory(policies []Policy, options ...leakybucket.Option) (*Client, error) {
	c := &Client{}
	bindings := make([]service.Binding, 0, len(policies))
	for _, p := range policies {
		l, err := leakybucket.New(p.LeakyBucket, options...)
		if err != nil {
			return nil, fmt.Errorf("sdk: policy %q: %w", p.Policy.ID, err)
		}
		c.memory = append(c.memory, l)
		bindings = append(bindings, service.Binding{Policy: p.Policy, Limiter: l})
	}
	var err error
	c.service, err = service.New(bindings)
	if err != nil {
		return nil, fmt.Errorf("sdk: %w", err)
	}
	return c, nil
}

// NewLeakyBucketRedis constructs shared leaky buckets using Policy.LeakyBucket.
// Connection ownership and retry/timeout requirements match NewRedis.
func NewLeakyBucketRedis(client redisclient.UniversalClient, config RedisConfig) (*Client, error) {
	c := &Client{}
	bindings := make([]service.Binding, 0, len(config.Policies))
	for _, p := range config.Policies {
		l, err := redisleaky.New(client, redisleaky.Config{Config: rediswindow.Config{
			PolicyID: p.Policy.ID, Namespace: config.Namespace, Revision: config.Revision,
			Limit: p.LeakyBucket.Limit, Window: p.LeakyBucket.Window, MaxKeys: p.LeakyBucket.MaxKeys, Timeout: config.Timeout,
			Semantics: fmt.Sprintf("sdk:identity=%s;per_route=%t", p.Policy.Identity, p.Policy.PerRoute),
		}, Capacity: p.LeakyBucket.Capacity,
		})
		if err != nil {
			return nil, fmt.Errorf("sdk: policy %q: %w", p.Policy.ID, err)
		}
		c.redis = append(c.redis, l)
		bindings = append(bindings, service.Binding{Policy: p.Policy, Limiter: l})
	}
	var err error
	c.service, err = service.New(bindings)
	if err != nil {
		return nil, fmt.Errorf("sdk: %w", err)
	}
	return c, nil
}

// NewSlidingCounterMemory constructs weighted sliding-window counters using Policy.SlidingCounter.
// Options apply to every policy. The client starts no background cleanup workers.
func NewSlidingCounterMemory(policies []Policy, options ...slidingcounter.Option) (*Client, error) {
	c := &Client{}
	bindings := make([]service.Binding, 0, len(policies))
	for _, p := range policies {
		l, err := slidingcounter.New(p.SlidingCounter, options...)
		if err != nil {
			return nil, fmt.Errorf("sdk: policy %q: %w", p.Policy.ID, err)
		}
		c.memory = append(c.memory, l)
		bindings = append(bindings, service.Binding{Policy: p.Policy, Limiter: l})
	}
	var err error
	c.service, err = service.New(bindings)
	if err != nil {
		return nil, fmt.Errorf("sdk: %w", err)
	}
	return c, nil
}

// NewSlidingCounterRedis constructs shared weighted sliding-window counters using Policy.SlidingCounter.
// Connection ownership and retry/timeout requirements match NewRedis.
func NewSlidingCounterRedis(client redisclient.UniversalClient, config RedisConfig) (*Client, error) {
	c := &Client{}
	bindings := make([]service.Binding, 0, len(config.Policies))
	for _, p := range config.Policies {
		l, err := rediscounter.New(client, rediscounter.Config{
			PolicyID: p.Policy.ID, Namespace: config.Namespace, Revision: config.Revision,
			Limit: p.SlidingCounter.Limit, Window: p.SlidingCounter.Window, MaxKeys: p.SlidingCounter.MaxKeys, Timeout: config.Timeout,
			Semantics: fmt.Sprintf("sdk:identity=%s;per_route=%t", p.Policy.Identity, p.Policy.PerRoute),
		})
		if err != nil {
			return nil, fmt.Errorf("sdk: policy %q: %w", p.Policy.ID, err)
		}
		c.redis = append(c.redis, l)
		bindings = append(bindings, service.Binding{Policy: p.Policy, Limiter: l})
	}
	var err error
	c.service, err = service.New(bindings)
	if err != nil {
		return nil, fmt.Errorf("sdk: %w", err)
	}
	return c, nil
}
