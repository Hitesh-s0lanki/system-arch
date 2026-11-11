package proxyapp_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/auth"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/identity"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/internal/proxyapp"
	redis "github.com/redis/go-redis/v9"
)

func tokenRedisConfig(t *testing.T, cfg *proxyapp.Config) {
	t.Helper()
	cfg.Storage.RedisURL = os.Getenv("REDIS_TEST_URL")
	if cfg.Storage.RedisURL == "" {
		t.Skip("set REDIS_TEST_URL for token-bucket server tests")
	}
	cfg.Storage.Mode = "redis"
	cfg.Storage.Namespace = "tb-server:" + strconv.FormatInt(time.Now().UnixNano(), 10)
	cfg.Storage.Timeout = "1s"
	opts, err := redis.ParseURL(cfg.Storage.RedisURL)
	if err != nil {
		t.Fatal(err)
	}
	client := redis.NewClient(opts)
	t.Cleanup(func() {
		keys, err := client.Keys(context.Background(), cfg.Storage.Namespace+":*").Result()
		if err != nil {
			t.Error(err)
		} else if len(keys) > 0 {
			if err = client.Del(context.Background(), keys...).Err(); err != nil {
				t.Error(err)
			}
		}
		client.Close()
	})
}
func tokenHTTPGet(t *testing.T, client *http.Client, url, token string) (int, http.Header) {
	t.Helper()
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("X-User-ID", "forged")
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, response.Body)
	response.Body.Close()
	return response.StatusCode, response.Header
}
func TestTokenBucketTCPReplicasIsolationAndCapacity(t *testing.T) {
	var calls atomic.Int32
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(204) }))
	defer backend.Close()
	cfg := configuration(backend.URL)
	tokenRedisConfig(t, &cfg)
	route := cfg.Routes[0]
	route.Algorithm = "token_bucket"
	route.Limit = 1
	route.Window = "1h"
	route.Capacity = 7
	route.MaxKeys = 2
	route.Policy.Identity = identity.User
	route.Policy.PerRoute = true
	cfg.Routes = nil
	for _, path := range []string{"/a", "/b", "/c"} {
		r := route
		r.Pattern = path
		cfg.Routes = append(cfg.Routes, r)
	}
	tokenA, tokenB := strings.Repeat("a", 32), strings.Repeat("b", 32)
	credentials := []auth.Credential{{Token: tokenA, Principal: auth.Principal{TenantID: "one", UserID: "same"}}, {Token: tokenB, Principal: auth.Principal{TenantID: "two", UserID: "same"}}}
	var servers []*httptest.Server
	for range 2 {
		a, err := proxyapp.New(context.Background(), cfg, credentials)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(a.Close)
		server := httptest.NewServer(a.Handler())
		t.Cleanup(server.Close)
		servers = append(servers, server)
	}
	client := &http.Client{Timeout: 3 * time.Second}
	defer client.CloseIdleConnections()
	// Probes and unauthenticated requests must not consume allowance.
	for _, path := range []string{"/readyz", "/readyz", "/a"} {
		status, _ := tokenHTTPGet(t, client, servers[0].URL+path, "")
		want := 200
		if path == "/a" {
			want = 401
		}
		if status != want {
			t.Fatalf("%s: %d want %d", path, status, want)
		}
	}
	var accepted, rejected atomic.Int32
	var wg sync.WaitGroup
	for i := range 100 {
		wg.Go(func() {
			status, headers := tokenHTTPGet(t, client, servers[i%2].URL+"/a", tokenA)
			switch status {
			case 204:
				accepted.Add(1)
			case 429:
				rejected.Add(1)
				if headers.Get("Retry-After") == "" || headers.Get("X-RateLimit-Remaining") != "0" {
					t.Error("missing rejection headers")
				}
			default:
				t.Errorf("unexpected status %d", status)
			}
		})
	}
	wg.Wait()
	if accepted.Load() != 7 || rejected.Load() != 93 {
		t.Fatalf("accepted %d rejected %d", accepted.Load(), rejected.Load())
	}
	for _, step := range []struct {
		path, token string
		want        int
	}{{"/b", tokenA, 204}, {"/c", tokenA, 503}, {"/a", tokenB, 204}, {"/a", tokenA, 429}} {
		status, _ := tokenHTTPGet(t, client, servers[1].URL+step.path, step.token)
		if status != step.want {
			t.Fatalf("%s status %d want %d", step.path, status, step.want)
		}
	}
	if calls.Load() != 9 {
		t.Fatalf("rejected traffic reached upstream: %d", calls.Load())
	}
}

// The bridge drops commands after startup, leaving Redis reads to time out.
func TestTokenBucketServerRedisTimeoutAndRecovery(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(204) }))
	defer upstream.Close()
	cfg := configuration(upstream.URL)
	tokenRedisConfig(t, &cfg)
	opts, err := redis.ParseURL(cfg.Storage.RedisURL)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var blocked atomic.Bool
	var connections sync.Map
	var wg sync.WaitGroup
	wg.Go(func() {
		for {
			incoming, err := listener.Accept()
			if err != nil {
				return
			}
			connections.Store(incoming, true)
			wg.Go(func() {
				defer incoming.Close()
				defer connections.Delete(incoming)
				outgoing, err := net.DialTimeout("tcp", opts.Addr, time.Second)
				if err != nil {
					return
				}
				defer outgoing.Close()
				done := make(chan struct{})
				go func() { defer close(done); io.Copy(incoming, outgoing); incoming.Close() }()
				buf := make([]byte, 32*1024)
				for {
					n, err := incoming.Read(buf)
					if n > 0 && !blocked.Load() {
						if _, e := outgoing.Write(buf[:n]); e != nil {
							break
						}
					}
					if err != nil {
						break
					}
				}
				outgoing.Close()
				<-done
			})
		}
	})
	t.Cleanup(func() {
		listener.Close()
		connections.Range(func(key, value any) bool { key.(net.Conn).Close(); return true })
		wg.Wait()
	})
	forwarded, err := url.Parse(cfg.Storage.RedisURL)
	if err != nil {
		t.Fatal(err)
	}
	forwarded.Host = listener.Addr().String()
	cfg.Storage.RedisURL = forwarded.String()
	cfg.Storage.Timeout = "100ms"
	cfg.Routes[0].Algorithm = "token_bucket"
	cfg.Routes[0].Capacity = 1
	cfg.Routes[0].Window = "1h"
	cfg.Routes[0].Limit = 1
	a, err := proxyapp.New(context.Background(), cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	server := httptest.NewServer(a.Handler())
	defer server.Close()
	client := &http.Client{Timeout: 2 * time.Second}
	defer client.CloseIdleConnections()
	if status, _ := tokenHTTPGet(t, client, server.URL+"/", ""); status != 204 {
		t.Fatal(status)
	}
	blocked.Store(true)
	for _, path := range []string{"/", "/readyz"} {
		started := time.Now()
		if status, _ := tokenHTTPGet(t, client, server.URL+path, ""); status != 503 {
			t.Fatalf("timeout %s status %d", path, status)
		}
		if time.Since(started) > time.Second {
			t.Fatal("unbounded Redis failure latency")
		}
	}
	blocked.Store(false)
	if status, _ := tokenHTTPGet(t, client, server.URL+"/readyz", ""); status != 200 {
		t.Fatalf("readiness did not recover %d", status)
	}
	if status, _ := tokenHTTPGet(t, client, server.URL+"/", ""); status != 429 {
		t.Fatalf("outage reset quota: %d", status)
	}
	if calls.Load() != 1 {
		t.Fatalf("outage forwarded requests: %d", calls.Load())
	}
}
