package proxyapp_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/auth"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/identity"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/internal/proxyapp"
)

func TestBodyLimitsBeforeChargingOrForwarding(t *testing.T) {
	var calls atomic.Int64
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		if string(body) != "ok" {
			t.Errorf("unexpected body %q", body)
		}
		w.WriteHeader(204)
	}))
	defer backend.Close()
	cfg := configuration(backend.URL)
	cfg.MaxBodyBytes = 2
	cfg.Routes[0].Limit = 1
	a, err := proxyapp.New(context.Background(), cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	for _, length := range []int64{3, -1} {
		r := httptest.NewRequest("POST", "/", strings.NewReader("big"))
		r.ContentLength = length
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, r)
		if w.Code != 413 {
			t.Fatalf("length %d got %d", length, w.Code)
		}
	}
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, httptest.NewRequest("POST", "/", strings.NewReader("ok")))
	if w.Code != 204 || calls.Load() != 1 {
		t.Fatalf("oversize request consumed quota or reached backend: %d calls=%d", w.Code, calls.Load())
	}
}

func TestOverloadDoesNotQueueOrConsumeQuota(t *testing.T) {
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case entered <- struct{}{}:
		default:
		}
		<-release
		w.WriteHeader(204)
	}))
	defer backend.Close()
	cfg := configuration(backend.URL)
	cfg.MaxConcurrent = 1
	a, err := proxyapp.New(context.Background(), cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	done := make(chan int, 1)
	go func() {
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
		done <- w.Code
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("upstream not entered")
	}
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	close(release)
	if w.Code != 503 || !strings.Contains(w.Body.String(), "gateway_overloaded") {
		t.Fatalf("overload: %d %s", w.Code, w.Body.String())
	}
	if code := <-done; code != 204 {
		t.Fatal(code)
	}
	w = httptest.NewRecorder()
	a.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 204 {
		t.Fatalf("overload consumed quota: %d", w.Code)
	}
}

func TestTenantCapacityIsolationAndProvisioning(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer backend.Close()
	cfg := configuration(backend.URL)
	cfg.Tenants = []string{"a", "b"}
	cfg.Routes[0].Policy.Identity = identity.User
	cfg.Routes[0].MaxKeys = 1
	credentials := []auth.Credential{}
	for i, p := range []auth.Principal{{TenantID: "a", UserID: "first"}, {TenantID: "a", UserID: "second"}, {TenantID: "b", UserID: "first"}, {TenantID: "unprovisioned", UserID: "first"}} {
		credentials = append(credentials, auth.Credential{Token: strings.Repeat(string(rune('a'+i)), 32), Principal: p})
	}
	a, err := proxyapp.New(context.Background(), cfg, credentials)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	for i, want := range []int{204, 503, 204, 403} {
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("Authorization", "Bearer "+credentials[i].Token)
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("tenant request %d got %d want %d", i, w.Code, want)
		}
	}
}

func TestMetricsAuthorizationAndBoundedLabels(t *testing.T) {
	t.Setenv("RATE_LIMIT_METRICS_TOKEN", "metrics-test-secret")
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer backend.Close()
	cfg := configuration(backend.URL)
	a, err := proxyapp.New(context.Background(), cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/private-user-id?secret=secret-query", nil))
	}
	for _, authorized := range []bool{false, true} {
		r := httptest.NewRequest("GET", "/metrics", nil)
		if authorized {
			r.Header.Set("Authorization", "Bearer metrics-test-secret")
		}
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, r)
		if !authorized {
			if w.Code != 404 {
				t.Fatal(w.Code)
			}
			continue
		}
		text := w.Body.String()
		for _, want := range []string{`ratelimit_decisions_total{outcome="allowed",policy="ip"} 2`, `ratelimit_decisions_total{outcome="rate_limited",policy="ip"} 1`, `ratelimit_capacity_utilization_ratio{policy="ip"} 0.01`, `ratelimit_inflight_requests 0`} {
			if !strings.Contains(text, want) {
				t.Fatalf("missing %s", want)
			}
		}
		for _, secret := range []string{"private-user-id", "secret-query", "metrics-test-secret", "192.0.2.1"} {
			if strings.Contains(text, secret) {
				t.Fatalf("sensitive metric label: %s", secret)
			}
		}
	}
}
