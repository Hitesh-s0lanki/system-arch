package proxyapp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	redisclient "github.com/redis/go-redis/v9"

	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/auth"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/identity"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/internal/proxyapp"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/policy"
)

func configuration(upstream string) proxyapp.Config {
	return proxyapp.Config{Version: 1, Listen: ":8080", Upstream: upstream, Storage: proxyapp.StorageConfig{Mode: "ram", Namespace: "test", Timeout: "100ms"}, CleanupInterval: "10ms", UpstreamTimeout: "1s", AnonymousTenant: "public",
		Routes: []proxyapp.Route{{Pattern: "/", Policy: policy.Policy{ID: "ip", Identity: identity.IP}, Limit: 2, Window: "876000h", MaxKeys: 100}}}
}

func TestReverseProxyPreservesRequestAndStopsRejectedTraffic(t *testing.T) {
	var calls atomic.Int64
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		if r.Method != "POST" || r.URL.Path != "/api/orders" || r.URL.RawQuery != "x=1&x=2" || string(body) != "payload" {
			t.Errorf("request changed: %s %s body=%q", r.Method, r.URL, body)
		}
		if r.Header.Get("X-User-ID") != "" || r.Header.Get("X-RateLimit-User-ID") != "" || r.Header.Get("Authorization") != "" {
			t.Error("untrusted identity or bearer token forwarded")
		}
		if r.Header.Get("X-Forwarded-For") != "192.0.2.1" {
			t.Errorf("spoofed client address: %q", r.Header.Get("X-Forwarded-For"))
		}
		w.Header().Set("X-Backend", "preserved")
		w.WriteHeader(201)
		_, _ = w.Write([]byte("backend response"))
	}))
	defer backend.Close()
	cfg := configuration(backend.URL + "/api")
	a, err := proxyapp.New(context.Background(), cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	for i := 0; i < 3; i++ {
		r := httptest.NewRequest("POST", "/orders?x=1&x=2", strings.NewReader("payload"))
		r.RemoteAddr = "192.0.2.1:1234"
		for _, header := range []string{"X-User-ID", "X-RateLimit-User-ID", "Authorization", "X-Forwarded-For"} {
			r.Header.Set(header, "forged")
		}
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, r)
		if i < 2 {
			if w.Code != 201 || w.Body.String() != "backend response" || w.Header().Get("X-Backend") != "preserved" {
				t.Fatalf("response not preserved: %+v", w)
			}
		} else if w.Code != 429 || w.Header().Get("Retry-After") == "" {
			t.Fatalf("expected rejection: %d", w.Code)
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("rejected request reached backend: calls=%d", calls.Load())
	}
}

func TestVerifiedIdentityForwarding(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-RateLimit-User-ID") != "verified" || r.Header.Get("X-RateLimit-Tenant-ID") != "tenant" || r.Header.Get("X-User-ID") != "" {
			t.Error("identity boundary failed")
		}
		w.WriteHeader(204)
	}))
	defer backend.Close()
	cfg := configuration(backend.URL)
	cfg.Routes[0].Policy.Identity = identity.User
	token := strings.Repeat("a", 32)
	a, err := proxyapp.New(context.Background(), cfg, []auth.Credential{{Token: token, Principal: auth.Principal{TenantID: "tenant", UserID: "verified"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	for _, authenticated := range []bool{false, true} {
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("X-User-ID", "forged")
		r.Header.Set("X-RateLimit-User-ID", "forged")
		if authenticated {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, r)
		want := 401
		if authenticated {
			want = 204
		}
		if w.Code != want {
			t.Fatalf("got %d want %d", w.Code, want)
		}
	}
}

func TestUpstreamFailureAndTimeout(t *testing.T) {
	for _, timeout := range []bool{false, true} {
		t.Run(fmt.Sprint(timeout), func(t *testing.T) {
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
			defer backend.Close()
			cfg := configuration(backend.URL)
			cfg.UpstreamTimeout = "30ms"
			if !timeout {
				backend.Close()
			}
			a, err := proxyapp.New(context.Background(), cfg, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			w := httptest.NewRecorder()
			a.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
			want := 502
			if timeout {
				want = 504
			}
			if w.Code != want {
				t.Fatalf("got %d want %d", w.Code, want)
			}
		})
	}
}

func TestConfigValidation(t *testing.T) {
	for _, mutate := range []func(*proxyapp.Config){
		func(c *proxyapp.Config) { c.Version = 2 },
		func(c *proxyapp.Config) { c.MaxBodyBytes = -1 },
		func(c *proxyapp.Config) { c.MaxConcurrent = -1 },
		func(c *proxyapp.Config) { c.MaxBodyBytes = 64 << 20; c.MaxConcurrent = 128 },
		func(c *proxyapp.Config) { c.Tenants = []string{""} },
		func(c *proxyapp.Config) { c.Routes[0].Revision = " " },
		func(c *proxyapp.Config) { c.Routes[0].Pattern = "/metrics" }, func(c *proxyapp.Config) { c.Upstream = "file:///tmp" },
		func(c *proxyapp.Config) { c.Upstream = "http://user:secret@localhost" }, func(c *proxyapp.Config) { c.Storage.Mode = "unknown" },
		func(c *proxyapp.Config) { c.CleanupInterval = "0s" }, func(c *proxyapp.Config) { c.UpstreamTimeout = "0s" },
		func(c *proxyapp.Config) { c.Routes = nil }, func(c *proxyapp.Config) { c.Routes[0].Limit = 0 },
		func(c *proxyapp.Config) { c.Routes[0].Pattern = "/healthz" }, func(c *proxyapp.Config) { c.Routes[0].Pattern = "/{" },
		func(c *proxyapp.Config) { c.TrustedProxies = []string{"garbage"} },
		func(c *proxyapp.Config) { c.Routes = append(c.Routes, c.Routes[0]) },
		func(c *proxyapp.Config) {
			r := c.Routes[0]
			r.Pattern = "/other"
			r.Limit++
			c.Routes = append(c.Routes, r)
		},
	} {
		cfg := configuration("http://localhost:1234")
		mutate(&cfg)
		if a, err := proxyapp.New(context.Background(), cfg, nil); err == nil {
			a.Close()
			t.Fatal("invalid config accepted")
		}
	}
}

func TestLoadStrictConfigAndEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "proxy.json")
	encoded, _ := json.Marshal(configuration("http://backend:8080"))
	if err := os.WriteFile(path, encoded, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RATE_LIMIT_STORAGE", "redis")
	t.Setenv("REDIS_URL", "redis://example:6379/0")
	cfg, err := proxyapp.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Storage.Mode != "redis" || cfg.Storage.RedisURL != "redis://example:6379/0" {
		t.Fatal("environment overrides missing")
	}
	for _, text := range []string{`{"version":1,"typo":true}`, `{} {}`, `{"version":`} {
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := proxyapp.Load(path); err == nil {
			t.Fatal("invalid JSON accepted")
		}
	}
	if err := os.WriteFile(path, []byte(strings.Repeat(" ", 1<<20+1)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := proxyapp.Load(path); err == nil {
		t.Fatal("oversized config accepted")
	}
}

func TestRedisReplicasShareQuotaAndFailClosed(t *testing.T) {
	redisURL := os.Getenv("REDIS_TEST_URL")
	if redisURL == "" {
		t.Skip("set REDIS_TEST_URL for real Redis integration")
	}
	var calls atomic.Int64
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(204) }))
	defer backend.Close()
	cfg := configuration(backend.URL)
	cfg.Storage.Mode = "redis"
	cfg.Storage.RedisURL = redisURL
	cfg.Storage.Namespace = fmt.Sprintf("proxy-test:%d", time.Now().UnixNano())
	options, err := redisclient.ParseURL(redisURL)
	if err != nil {
		t.Fatal("invalid REDIS_TEST_URL")
	}
	options.MaxRetries = -1
	cleanupClient := redisclient.NewClient(options)
	t.Cleanup(func() {
		defer cleanupClient.Close()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		keys, err := cleanupClient.Keys(ctx, cfg.Storage.Namespace+":fw:*").Result()
		if err == nil && len(keys) > 0 {
			if err := cleanupClient.Del(ctx, keys...).Err(); err != nil {
				t.Error(err)
			}
		}
	})
	first, err := proxyapp.New(context.Background(), cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := proxyapp.New(context.Background(), cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	for i, a := range []*proxyapp.App{first, second, first} {
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
		want := 204
		if i == 2 {
			want = 429
		}
		if w.Code != want {
			t.Fatalf("replica request %d: got %d want %d", i, w.Code, want)
		}
	}
	first.Close()
	w := httptest.NewRecorder()
	first.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 503 || calls.Load() != 2 {
		t.Fatalf("backend failure did not fail closed: status=%d calls=%d", w.Code, calls.Load())
	}
	w = httptest.NewRecorder()
	first.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/readyz", nil))
	if w.Code != 503 {
		t.Fatal("readiness ignored Redis failure")
	}
	w = httptest.NewRecorder()
	first.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/healthz", nil))
	if w.Code != 200 {
		t.Fatal("liveness should be independent of Redis")
	}
}
