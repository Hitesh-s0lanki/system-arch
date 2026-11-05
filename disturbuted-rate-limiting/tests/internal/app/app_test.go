package app_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/auth"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/internal/app"
)

func config() app.Config {
	return app.Config{Limit: 1, Window: 10 * time.Second, MaxKeys: 100, CleanupInterval: time.Millisecond}
}

func TestHTTPEndToEnd(t *testing.T) {
	cfg := config()
	var now atomic.Int64
	now.Store(time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC).UnixNano())
	cfg.Clock = func() time.Time { return time.Unix(0, now.Load()) }
	tokenA, tokenB, tokenC := strings.Repeat("a", 32), strings.Repeat("b", 32), strings.Repeat("c", 32)
	cfg.Credentials = []auth.Credential{
		{Token: tokenA, Principal: auth.Principal{TenantID: "a", UserID: "42", APIKeyID: "key-a"}},
		{Token: tokenB, Principal: auth.Principal{TenantID: "a", UserID: "43", APIKeyID: "key-b"}},
		{Token: tokenC, Principal: auth.Principal{TenantID: "b", UserID: "42", APIKeyID: "key-c"}},
	}
	a, err := app.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(a.Handler())
	defer server.Close()
	client := server.Client()
	client.Timeout = 2 * time.Second
	check := func(path, token string, want int) {
		t.Helper()
		r, err := http.NewRequest("GET", server.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		r.Header.Set("X-Forwarded-For", "198.51.100.99")
		r.Header.Set("X-User-ID", "forged")
		r.Header.Set("X-Tenant-ID", "forged")
		response, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, _ := io.ReadAll(response.Body)
		if response.StatusCode != want {
			t.Fatalf("%s: got %d %s, want %d", path, response.StatusCode, body, want)
		}
		if want == 429 && response.Header.Get("Retry-After") != "9" {
			t.Fatalf("unexpected retry header: %v", response.Header)
		}
		if want == 200 && path != "/healthz" && response.Header.Get("X-RateLimit-Remaining") != "0" {
			t.Fatal("missing remaining header")
		}
	}
	check("/healthz", "", 200)
	check("/v1/ip", "", 200)
	check("/v1/ip", "", 429)
	check("/v1/user", "", 401)
	check("/v1/user", strings.Repeat("x", 32), 401)
	check("/v1/user", tokenA, 200)
	check("/v1/user", tokenA, 429)
	check("/v1/user", tokenB, 200)
	check("/v1/user", tokenC, 200)
	check("/v1/api-key", tokenA, 200)
	check("/v1/api-key", tokenA, 429)
	check("/v1/items/1", tokenA, 200)
	check("/v1/items/2", tokenA, 429)
	check("/v1/orders", tokenA, 200)
	check("/not-found", "", 404)
	now.Add(int64(10 * time.Second))
	check("/v1/user", tokenA, 200)
}

func TestHTTPOverloadIsNotRateExhaustion(t *testing.T) {
	cfg := config()
	cfg.MaxKeys = 1
	cfg.Credentials = []auth.Credential{
		{Token: strings.Repeat("a", 32), Principal: auth.Principal{TenantID: "t", UserID: "a"}},
		{Token: strings.Repeat("b", 32), Principal: auth.Principal{TenantID: "t", UserID: "b"}},
	}
	a, err := app.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for i, token := range []string{strings.Repeat("a", 32), strings.Repeat("b", 32)} {
		r := httptest.NewRequest("GET", "/v1/user", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, r)
		want := 200
		if i == 1 {
			want = 503
		}
		if w.Code != want {
			t.Fatalf("got %d want %d", w.Code, want)
		}
		if i == 1 && w.Header().Get("Retry-After") != "" {
			t.Fatal("capacity rejection promised retry time")
		}
	}
}

func TestServeShutdown(t *testing.T) {
	a, err := app.New(config())
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- a.Serve(ctx, listener) }()
	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Get("http://" + listener.Addr().String() + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	response.Body.Close()
	client.CloseIdleConnections()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("HTTP/cleanup shutdown did not finish")
	}
	if connection, err := net.DialTimeout("tcp", listener.Addr().String(), 100*time.Millisecond); err == nil {
		connection.Close()
		t.Fatal("listener still open")
	}
}

func TestServeListenerFailureStopsCleanup(t *testing.T) {
	a, err := app.New(config())
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	listener.Close()
	done := make(chan error, 1)
	go func() { done <- a.Serve(context.Background(), listener) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected listener error")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("listener failure left cleanup running")
	}
}

func TestAppConfigValidation(t *testing.T) {
	for _, mutate := range []func(*app.Config){
		func(c *app.Config) { c.Limit = 0 },
		func(c *app.Config) { c.Window = 0 },
		func(c *app.Config) { c.MaxKeys = 0 },
		func(c *app.Config) { c.CleanupInterval = 0 },
		func(c *app.Config) { c.Credentials = []auth.Credential{{Token: "short"}} },
	} {
		cfg := config()
		mutate(&cfg)
		if _, err := app.New(cfg); err == nil {
			t.Fatal("invalid config accepted")
		}
	}
}
