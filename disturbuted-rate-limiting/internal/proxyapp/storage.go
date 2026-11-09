package proxyapp

import (
	"context"
	"fmt"
	"os"
	"time"

	ratelimit "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/service"
	redisclient "github.com/redis/go-redis/v9"
)

// tenantLimiter never allocates state for unknown tenants. Each configured tenant
// gets a distinct capacity budget and Redis slot; tenants cannot evict each other.
type tenantLimiter struct{ tenants map[string]service.Limiter }

func (l *tenantLimiter) Evaluate(ctx context.Context, key string) (ratelimit.Decision, error) {
	return ratelimit.Decision{}, fmt.Errorf("proxy: tenant scope required")
}
func (l *tenantLimiter) EvaluateTenant(ctx context.Context, tenant, key string) (ratelimit.Decision, error) {
	backend, ok := l.tenants[tenant]
	if !ok {
		return ratelimit.Decision{}, service.ErrUnauthorizedTenant
	}
	return backend.Evaluate(ctx, key)
}

func redisOptions(config StorageConfig, timeout time.Duration) (*redisclient.UniversalOptions, error) {
	parsed, err := redisclient.ParseURL(config.RedisURL)
	if err != nil {
		return nil, fmt.Errorf("proxy config: invalid Redis URL")
	}
	topology := config.Topology
	if topology == "" {
		topology = "standalone"
	}
	addresses := config.Addresses
	if len(addresses) == 0 {
		addresses = []string{parsed.Addr}
	}
	if config.PoolSize == 0 {
		config.PoolSize = 32
	}
	if config.PoolSize < 1 || config.PoolSize > 1024 {
		return nil, fmt.Errorf("proxy config: Redis pool size must be 1..1024")
	}
	if topology != "standalone" && topology != "sentinel" && topology != "cluster" {
		return nil, fmt.Errorf("proxy config: invalid Redis topology")
	}
	if topology == "standalone" && len(addresses) != 1 {
		return nil, fmt.Errorf("proxy config: standalone needs one address")
	}
	if topology == "sentinel" && config.MasterName == "" {
		return nil, fmt.Errorf("proxy config: Sentinel master name required")
	}
	if topology == "cluster" && parsed.DB != 0 {
		return nil, fmt.Errorf("proxy config: Cluster requires database zero")
	}
	options := &redisclient.UniversalOptions{
		Addrs: addresses, DB: parsed.DB, Username: parsed.Username, Password: parsed.Password, TLSConfig: parsed.TLSConfig,
		MaxRetries: -1, DialTimeout: timeout, ReadTimeout: timeout, WriteTimeout: timeout, PoolTimeout: timeout,
		ContextTimeoutEnabled: true, PoolSize: config.PoolSize, MaxActiveConns: config.PoolSize,
		IsClusterMode: topology == "cluster", SentinelUsername: os.Getenv("REDIS_SENTINEL_USERNAME"), SentinelPassword: os.Getenv("REDIS_SENTINEL_PASSWORD"),
	}
	if topology == "sentinel" {
		options.MasterName = config.MasterName
	}
	return options, nil
}
