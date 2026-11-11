package proxyapp

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/auth"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/policy"
)

type StorageConfig struct {
	Mode       string   `json:"mode"`
	RedisURL   string   `json:"redis_url"`
	Namespace  string   `json:"namespace"`
	Timeout    string   `json:"timeout"`
	Topology   string   `json:"topology"`
	Addresses  []string `json:"addresses"`
	MasterName string   `json:"master_name"`
	PoolSize   int      `json:"pool_size"`
}

type Route struct {
	Capacity  int           `json:"capacity,omitempty"`
	Algorithm string        `json:"algorithm,omitempty"`
	Pattern   string        `json:"pattern"`
	Revision  string        `json:"revision"`
	Policy    policy.Policy `json:"policy"`
	Limit     int           `json:"limit"`
	Window    string        `json:"window"`
	MaxKeys   int           `json:"max_keys"`
}

// Config is a versioned proxy configuration. Secrets may be supplied through
// REDIS_URL and RATE_LIMIT_CREDENTIALS instead of storing them in this file.
type Config struct {
	MaxBodyBytes         int64            `json:"max_body_bytes"`
	MaxConcurrent        int              `json:"max_concurrent"`
	Tenants              []string         `json:"tenants"`
	OIDC                 *auth.OIDCConfig `json:"oidc"`
	Version              int              `json:"version"`
	Listen               string           `json:"listen"`
	Upstream             string           `json:"upstream"`
	Storage              StorageConfig    `json:"storage"`
	CleanupInterval      string           `json:"cleanup_interval"`
	UpstreamTimeout      string           `json:"upstream_timeout"`
	TrustedProxies       []string         `json:"trusted_proxies"`
	AnonymousTenant      string           `json:"anonymous_tenant"`
	ForwardAuthorization bool             `json:"forward_authorization"`
	Routes               []Route          `json:"routes"`
}

func Load(path string) (Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("proxy config: cannot open file: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.Size() > 1<<20 {
		return Config{}, fmt.Errorf("proxy config: file must be at most 1 MiB")
	}
	config := Config{Listen: ":8080", Storage: StorageConfig{Mode: "ram", Namespace: "ratelimit", Timeout: "500ms"}, CleanupInterval: "1s", UpstreamTimeout: "30s", AnonymousTenant: "public"}
	decoder := json.NewDecoder(io.LimitReader(file, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("proxy config: invalid JSON or unknown fields")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return Config{}, fmt.Errorf("proxy config: expected one JSON object")
	}
	for name, destination := range map[string]*string{
		"RATE_LIMIT_LISTEN": &config.Listen, "RATE_LIMIT_UPSTREAM": &config.Upstream,
		"RATE_LIMIT_STORAGE": &config.Storage.Mode, "REDIS_URL": &config.Storage.RedisURL,
		"RATE_LIMIT_NAMESPACE": &config.Storage.Namespace,
	} {
		if value, exists := os.LookupEnv(name); exists {
			*destination = value
		}
	}
	return config, nil
}
