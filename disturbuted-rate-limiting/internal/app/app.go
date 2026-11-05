// Package app wires the local HTTP service and owns its shutdown lifecycle.
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"time"

	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/fixedwindow"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/auth"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/identity"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/internal/httpserver"
	httpmiddleware "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/middleware/http"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/policy"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/service"
)

type Config struct {
	Limit           int
	Window          time.Duration
	MaxKeys         int
	CleanupInterval time.Duration
	TrustedProxies  []netip.Prefix
	Credentials     []auth.Credential
	// Clock optionally supplies deterministic time for embedded use and tests.
	Clock func() time.Time
}

type App struct {
	handler         http.Handler
	limiters        []*fixedwindow.Limiter
	cleanupInterval time.Duration
}

func New(config Config) (*App, error) {
	if config.CleanupInterval <= 0 {
		return nil, fmt.Errorf("app: cleanup interval must be positive")
	}
	authenticator, err := auth.NewBearer(config.Credentials)
	if err != nil {
		return nil, err
	}
	a := &App{cleanupInterval: config.CleanupInterval}
	policies := []policy.Policy{
		{ID: "per-ip", Identity: identity.IP},
		{ID: "per-user", Identity: identity.User},
		{ID: "per-credential", Identity: identity.APIKey},
		{ID: "per-user-route", Identity: identity.User, PerRoute: true},
	}
	bindings := make([]service.Binding, 0, len(policies))
	for _, p := range policies {
		var options []fixedwindow.Option
		if config.Clock != nil {
			options = append(options, fixedwindow.WithClock(config.Clock))
		}
		limiter, err := fixedwindow.New(fixedwindow.Config{Limit: config.Limit, Window: config.Window, MaxKeys: config.MaxKeys}, options...)
		if err != nil {
			return nil, err
		}
		a.limiters = append(a.limiters, limiter)
		bindings = append(bindings, service.Binding{Policy: p, Limiter: limiter})
	}
	s, err := service.New(bindings)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	for _, route := range []struct {
		pattern string
		index   int
	}{
		{"GET /v1/ip", 0}, {"GET /v1/user", 1}, {"GET /v1/api-key", 2},
		{"GET /v1/items/{id}", 3}, {"GET /v1/orders", 3},
	} {
		pattern := route.pattern
		handler, err := httpmiddleware.Wrap(s, httpmiddleware.Config{
			Policy: policies[route.index], Route: pattern, AnonymousTenant: "public",
			TrustedProxies: config.TrustedProxies, Authenticator: authenticator,
		}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "allowed", "route": pattern})
		}))
		if err != nil {
			return nil, err
		}
		mux.Handle(pattern, handler)
	}
	a.handler = mux
	return a, nil
}

func (a *App) Handler() http.Handler { return a.handler }

// Serve runs HTTP and periodic cleanup, then joins both during shutdown.
// Each policy owns its own MaxKeys capacity; total entries can be four times it.
func (a *App) Serve(ctx context.Context, listener net.Listener) error {
	return httpserver.Serve(ctx, listener, a.handler, a.cleanupInterval, 10*time.Second, func() {
		for _, limiter := range a.limiters {
			limiter.CleanupExpired()
		}
	})
}
