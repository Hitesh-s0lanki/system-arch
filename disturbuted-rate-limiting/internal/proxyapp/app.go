// Package proxyapp wires configurable routes to a fixed-window reverse proxy.
package proxyapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/fixedwindow"
	rediswindow "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/fixedwindow/redis"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/auth"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/internal/httpserver"
	httpmiddleware "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/middleware/http"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/service"
	redisclient "github.com/redis/go-redis/v9"
)

type App struct {
	handler         http.Handler
	ram             []*fixedwindow.Limiter
	redis           *redisclient.Client
	transport       *http.Transport
	cleanupInterval time.Duration
	upstreamTimeout time.Duration
	redisTimeout    time.Duration
}

type clientIPKey struct{}

func New(ctx context.Context, config Config, credentials []auth.Credential) (result *App, err error) {
	if config.Version != 1 || config.Listen == "" || len(config.Routes) == 0 {
		return nil, fmt.Errorf("proxy config: version 1, listen address, and routes required")
	}
	upstream, err := url.Parse(config.Upstream)
	if err != nil || upstream.Host == "" || (upstream.Scheme != "http" && upstream.Scheme != "https") || upstream.User != nil || upstream.RawQuery != "" || upstream.Fragment != "" {
		return nil, fmt.Errorf("proxy config: upstream must be an HTTP(S) URL without credentials, query, or fragment")
	}
	cleanup, err := positiveDuration(config.CleanupInterval)
	if err != nil {
		return nil, fmt.Errorf("proxy config: invalid cleanup interval")
	}
	timeout, err := positiveDuration(config.UpstreamTimeout)
	if err != nil || timeout > 24*time.Hour {
		return nil, fmt.Errorf("proxy config: upstream timeout must be positive and at most 24h")
	}
	redisTimeout, err := positiveDuration(config.Storage.Timeout)
	if err != nil {
		return nil, fmt.Errorf("proxy config: invalid storage timeout")
	}
	if config.Storage.Mode != "ram" && config.Storage.Mode != "redis" {
		return nil, fmt.Errorf("proxy config: storage mode must be ram or redis")
	}
	var proxies []netip.Prefix
	for _, value := range config.TrustedProxies {
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			return nil, fmt.Errorf("proxy config: invalid trusted proxy CIDR")
		}
		proxies = append(proxies, prefix)
	}
	authenticator, err := auth.NewBearer(credentials)
	if err != nil {
		return nil, err
	}
	a := &App{cleanupInterval: cleanup, upstreamTimeout: timeout, redisTimeout: redisTimeout}
	defer func() {
		if err != nil {
			a.Close()
		}
	}()
	if config.Storage.Mode == "redis" {
		options, parseErr := redisclient.ParseURL(config.Storage.RedisURL)
		if parseErr != nil {
			return nil, fmt.Errorf("proxy config: invalid Redis URL")
		}
		options.MaxRetries = -1
		options.DialTimeout = redisTimeout
		options.ReadTimeout = redisTimeout
		options.WriteTimeout = redisTimeout
		options.ContextTimeoutEnabled = true
		a.redis = redisclient.NewClient(options)
		pingCtx, cancel := context.WithTimeout(ctx, redisTimeout)
		pingErr := a.redis.Ping(pingCtx).Err()
		cancel()
		if pingErr != nil {
			return nil, fmt.Errorf("proxy: Redis unavailable at startup")
		}
	}
	var bindings []service.Binding
	registered := make(map[string]Route)
	for _, route := range config.Routes {
		parts := strings.Fields(route.Pattern)
		if len(parts) < 1 || len(parts) > 2 || !strings.HasPrefix(parts[len(parts)-1], "/") || len(route.Pattern) > 256 {
			return nil, fmt.Errorf("proxy config: invalid route pattern")
		}
		path := parts[len(parts)-1]
		if path == "/healthz" || path == "/readyz" {
			return nil, fmt.Errorf("proxy config: health and readiness paths are reserved")
		}
		if err := route.Policy.Validate(); err != nil {
			return nil, err
		}
		window, err := positiveDuration(route.Window)
		if err != nil {
			return nil, fmt.Errorf("proxy config: invalid window")
		}
		if previous, exists := registered[route.Policy.ID]; exists {
			if previous.Policy != route.Policy || previous.Limit != route.Limit || previous.Window != route.Window || previous.MaxKeys != route.MaxKeys {
				return nil, fmt.Errorf("proxy config: policy %q has inconsistent settings", route.Policy.ID)
			}
			continue
		}
		var limiter service.Limiter
		if a.redis != nil {
			limiter, err = rediswindow.New(a.redis, rediswindow.Config{PolicyID: route.Policy.ID, Namespace: config.Storage.Namespace, Limit: route.Limit, Window: window, MaxKeys: route.MaxKeys, Timeout: redisTimeout})
		} else {
			var local *fixedwindow.Limiter
			local, err = fixedwindow.New(fixedwindow.Config{Limit: route.Limit, Window: window, MaxKeys: route.MaxKeys})
			if err == nil {
				limiter = local
				a.ram = append(a.ram, local)
			}
		}
		if err != nil {
			return nil, err
		}
		registered[route.Policy.ID] = route
		bindings = append(bindings, service.Binding{Policy: route.Policy, Limiter: limiter})
	}
	s, err := service.New(bindings)
	if err != nil {
		return nil, err
	}
	a.transport = http.DefaultTransport.(*http.Transport).Clone()
	a.transport.Proxy = nil
	a.transport.DialContext = (&net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}).DialContext
	a.transport.ResponseHeaderTimeout = timeout
	a.transport.TLSHandshakeTimeout = timeout
	proxy := &httputil.ReverseProxy{
		Transport: a.transport,
		Rewrite: func(request *httputil.ProxyRequest) {
			request.SetURL(upstream)
			request.SetXForwarded()
			request.Out.Header.Set("X-Forwarded-For", request.In.Context().Value(clientIPKey{}).(string))
			for _, header := range []string{"X-User-ID", "X-Tenant-ID", "X-API-Key-ID", "X-RateLimit-User-ID", "X-RateLimit-Tenant-ID", "X-RateLimit-API-Key-ID", "X-RateLimit-Policy", "X-Real-IP"} {
				request.Out.Header.Del(header)
			}
			if !config.ForwardAuthorization {
				request.Out.Header.Del("Authorization")
			}
			if principal, ok := httpmiddleware.PrincipalFromContext(request.In.Context()); ok {
				request.Out.Header.Set("X-RateLimit-Tenant-ID", principal.TenantID)
				request.Out.Header.Set("X-RateLimit-User-ID", principal.UserID)
				request.Out.Header.Set("X-RateLimit-API-Key-ID", principal.APIKeyID)
			}
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			var networkError net.Error
			if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &networkError) && networkError.Timeout()) {
				writeError(w, 504, "upstream_timeout")
			} else {
				writeError(w, 502, "upstream_unavailable")
			}
		},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { health(w, r, true) })
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		ready := true
		if a.redis != nil {
			ctx, cancel := context.WithTimeout(r.Context(), a.redisTimeout)
			defer cancel()
			ready = a.redis.Ping(ctx).Err() == nil
		}
		health(w, r, ready)
	})
	for _, route := range config.Routes {
		forward := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// This first release supports bounded HTTP requests, not upgrades.
			if r.Header.Get("Upgrade") != "" {
				writeError(w, 501, "protocol_upgrade_not_supported")
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()
			proxy.ServeHTTP(w, r.WithContext(ctx))
		})
		limited, err := httpmiddleware.Wrap(s, httpmiddleware.Config{Policy: route.Policy, Route: route.Pattern, AnonymousTenant: config.AnonymousTenant, TrustedProxies: proxies, Authenticator: authenticator}, forward)
		if err != nil {
			return nil, err
		}
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip, err := httpmiddleware.ClientIP(r, proxies)
			if err != nil {
				writeError(w, 400, "invalid_client_ip")
				return
			}
			limited.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), clientIPKey{}, ip)))
		})
		if err := register(mux, route.Pattern, handler); err != nil {
			return nil, err
		}
	}
	a.handler = mux
	return a, nil
}

func positiveDuration(value string) (time.Duration, error) {
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("invalid positive duration")
	}
	return duration, nil
}

func register(mux *http.ServeMux, pattern string, handler http.Handler) (err error) {
	defer func() {
		if recover() != nil {
			err = fmt.Errorf("proxy config: invalid or conflicting route pattern")
		}
	}()
	mux.Handle(pattern, handler)
	return nil
}

func writeError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
}
func health(w http.ResponseWriter, r *http.Request, ready bool) {
	if r.Method != "GET" && r.Method != "HEAD" {
		w.Header().Set("Allow", "GET, HEAD")
		writeError(w, 405, "method_not_allowed")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	if !ready {
		writeError(w, 503, "not_ready")
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
func (a *App) Handler() http.Handler { return a.handler }
func (a *App) Close() {
	if a.transport != nil {
		a.transport.CloseIdleConnections()
	}
	if a.redis != nil {
		_ = a.redis.Close()
	}
}
func (a *App) Serve(ctx context.Context, listener net.Listener) error {
	defer a.Close()
	return httpserver.Serve(ctx, listener, a.handler, a.cleanupInterval, a.upstreamTimeout+5*time.Second, func() {
		for _, limiter := range a.ram {
			limiter.CleanupExpired()
		}
	})
}
