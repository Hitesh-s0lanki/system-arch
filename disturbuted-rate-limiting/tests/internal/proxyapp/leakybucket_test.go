package proxyapp_test

import (
	"context"
	"fmt"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/internal/proxyapp"
	redis "github.com/redis/go-redis/v9"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func TestLeakyBucketProxyBothBackends(t *testing.T) {
	for _, mode := range []string{"ram", "redis"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(204) }))
			defer backend.Close()
			cfg := configuration(backend.URL)
			cfg.Routes[0].Algorithm = "leaky_bucket"
			cfg.Routes[0].Window = "500ms"
			cfg.Routes[0].Limit = 1
			cfg.Routes[0].Capacity = 1
			cfg.Storage.Mode = mode
			cfg.Storage.Timeout = "1s"
			if mode == "redis" {
				cfg.Storage.RedisURL = os.Getenv("REDIS_TEST_URL")
				if cfg.Storage.RedisURL == "" {
					t.Skip("set REDIS_TEST_URL for leaky-bucket proxy integration")
				}
				cfg.Storage.Namespace = fmt.Sprintf("proxy-lb:%d", time.Now().UnixNano())
				opts, err := redis.ParseURL(cfg.Storage.RedisURL)
				if err != nil {
					t.Fatal(err)
				}
				client := redis.NewClient(opts)
				t.Cleanup(func() {
					keys, _ := client.Keys(context.Background(), cfg.Storage.Namespace+":*").Result()
					if len(keys) > 0 {
						client.Del(context.Background(), keys...)
					}
					client.Close()
				})
			}
			first, err := proxyapp.New(context.Background(), cfg, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer first.Close()
			second := first
			if mode == "redis" {
				second, err = proxyapp.New(context.Background(), cfg, nil)
				if err != nil {
					t.Fatal(err)
				}
				defer second.Close()
			}
			for i, a := range []*proxyapp.App{first, second} {
				w := httptest.NewRecorder()
				a.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
				want := 204
				if i == 1 {
					want = 429
				}
				if w.Code != want {
					t.Fatalf("status %d want %d", w.Code, want)
				}
				if i == 1 && w.Header().Get("Retry-After") == "" {
					t.Fatal("missing retry hint")
				}
			}
			if calls.Load() != 1 {
				t.Fatal("rejected request reached upstream")
			}
			time.Sleep(550 * time.Millisecond)
			w := httptest.NewRecorder()
			second.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
			if w.Code != 204 || calls.Load() != 2 {
				t.Fatalf("rolling expiry status %d calls %d", w.Code, calls.Load())
			}
			if mode == "redis" {
				second.Close()
				w = httptest.NewRecorder()
				second.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
				if w.Code != 503 || calls.Load() != 2 {
					t.Fatalf("failure did not fail closed: %d", w.Code)
				}
			}
		})
	}
}

func TestLeakyBucketProxyCapacityValidation(t *testing.T) {
	for _, capacity := range []int{0, -1, 2} {
		cfg := configuration("http://localhost:1234")
		cfg.Routes[0].Algorithm = "leaky_bucket"
		cfg.Routes[0].Capacity = capacity
		if capacity == 2 {
			other := cfg.Routes[0]
			other.Pattern = "/other"
			other.Capacity = 3
			cfg.Routes = append(cfg.Routes, other)
		}
		if a, err := proxyapp.New(context.Background(), cfg, nil); err == nil {
			a.Close()
			t.Fatal("accepted missing, invalid, or inconsistent capacity")
		}
	}
}
