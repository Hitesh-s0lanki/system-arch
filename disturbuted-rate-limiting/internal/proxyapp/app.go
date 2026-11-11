// Package proxyapp wires configurable routes to a rate-limited reverse proxy.
package proxyapp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/netip"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

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
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/auth"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/identity"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/internal/httpserver"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/internal/telemetry"
	httpmiddleware "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/middleware/http"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/service"
	redisclient "github.com/redis/go-redis/v9"
)

type App struct {
	draining        atomic.Bool
	handler         http.Handler
	ram             []interface{ CleanupExpired() int }
	redis           redisclient.UniversalClient
	readyChecks     []func(context.Context) error
	metrics         *telemetry.Metrics
	usages          map[string][]func(context.Context) (float64, error)
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
	if config.MaxBodyBytes == 0 {
		config.MaxBodyBytes = 1 << 20
	}
	if config.MaxConcurrent == 0 {
		config.MaxConcurrent = 128
	}
	if config.MaxBodyBytes < 1 || config.MaxBodyBytes > 64<<20 || config.MaxConcurrent < 1 || config.MaxConcurrent > 10000 {
		return nil, fmt.Errorf("proxy config: body limit must be 1..64MiB and concurrency 1..10000")
	}
	if config.MaxBodyBytes*int64(config.MaxConcurrent) > 256<<20 {
		return nil, fmt.Errorf("proxy config: buffered body budget exceeds 256MiB; lower concurrency or body limit")
	}
	if len(config.Routes) > 256 {
		return nil, fmt.Errorf("proxy config: at most 256 routes")
	}
	tenants := map[string]bool{}
	if len(config.Tenants) > 0 {
		for _, tenant := range config.Tenants {
			tenants[tenant] = true
		}
	} else {
		tenants[config.AnonymousTenant] = true
		for _, credential := range credentials {
			tenants[credential.TenantID] = true
		}
	}
	if config.OIDC != nil && len(config.Tenants) == 0 {
		return nil, fmt.Errorf("proxy config: OIDC requires explicit provisioned tenants")
	}
	if len(tenants) > 256 {
		return nil, fmt.Errorf("proxy config: at most 256 provisioned tenants")
	}
	tenantIDs := make([]string, 0, len(tenants))
	for tenant := range tenants {
		if _, err := identity.Key("validate", tenant, identity.Subject{Kind: identity.User, Value: "validate"}, ""); err != nil {
			return nil, err
		}
		tenantIDs = append(tenantIDs, tenant)
	}
	sort.Strings(tenantIDs)
	var proxies []netip.Prefix
	for _, value := range config.TrustedProxies {
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			return nil, fmt.Errorf("proxy config: invalid trusted proxy CIDR")
		}
		proxies = append(proxies, prefix)
	}
	var authenticator httpmiddleware.Authenticator
	if config.OIDC != nil {
		if len(credentials) != 0 {
			return nil, fmt.Errorf("proxy config: choose static credentials or OIDC")
		}
		authenticator, err = auth.NewOIDC(ctx, *config.OIDC)
	} else {
		authenticator, err = auth.NewBearer(credentials)
	}
	if err != nil {
		return nil, err
	}
	a := &App{cleanupInterval: cleanup, upstreamTimeout: timeout, redisTimeout: redisTimeout, metrics: telemetry.New(), usages: make(map[string][]func(context.Context) (float64, error))}
	defer func() {
		if err != nil {
			a.Close()
		}
	}()
	if config.Storage.Mode == "redis" {
		options, parseErr := redisOptions(config.Storage, redisTimeout)
		if parseErr != nil {
			return nil, parseErr
		}
		a.redis = redisclient.NewUniversalClient(options)
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
		if route.Algorithm == "" {
			route.Algorithm = "fixed_window"
		}
		if route.Algorithm != "fixed_window" && route.Algorithm != "sliding_window_log" && route.Algorithm != "sliding_window_counter" && route.Algorithm != "token_bucket" && route.Algorithm != "leaky_bucket" {
			return nil, fmt.Errorf("proxy config: algorithm must be fixed_window, sliding_window_log, sliding_window_counter, token_bucket, or leaky_bucket")
		}

		if route.Revision == "" {
			route.Revision = "v1"
		}
		parts := strings.Fields(route.Pattern)
		if len(parts) < 1 || len(parts) > 2 || !strings.HasPrefix(parts[len(parts)-1], "/") || len(route.Pattern) > 256 {
			return nil, fmt.Errorf("proxy config: invalid route pattern")
		}
		path := parts[len(parts)-1]
		if path == "/healthz" || path == "/readyz" || path == "/metrics" {
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
			if previous.Capacity != route.Capacity || previous.Algorithm != route.Algorithm || previous.Revision != route.Revision || previous.Policy != route.Policy || previous.Limit != route.Limit || previous.Window != route.Window || previous.MaxKeys != route.MaxKeys {
				return nil, fmt.Errorf("proxy config: policy %q has inconsistent settings", route.Policy.ID)
			}
			continue
		}
		if route.Revision == "" {
			route.Revision = "v1"
		}
		if strings.TrimSpace(route.Revision) == "" || len(route.Revision) > 64 {
			return nil, fmt.Errorf("proxy config: invalid policy revision")
		}
		scoped := &tenantLimiter{tenants: make(map[string]service.Limiter)}
		for _, tenant := range tenantIDs {
			var limiter service.Limiter
			if a.redis != nil {
				var remote interface {
					service.Limiter
					Ready(context.Context) error
					Usage(context.Context) (float64, error)
				}
				remoteConfig := rediswindow.Config{PolicyID: route.Policy.ID, Namespace: config.Storage.Namespace, Limit: route.Limit, Window: window, MaxKeys: route.MaxKeys, Timeout: redisTimeout, Revision: route.Revision, TenantID: tenant, Semantics: fmt.Sprintf("%s:%t", route.Policy.Identity, route.Policy.PerRoute)}
				if route.Algorithm == "leaky_bucket" {
					remote, err = redisleaky.New(a.redis, redisleaky.Config{Config: remoteConfig, Capacity: route.Capacity})
				} else if route.Algorithm == "sliding_window_counter" {
					remote, err = rediscounter.New(a.redis, remoteConfig)
				} else if route.Algorithm == "sliding_window_log" {
					remote, err = redissliding.New(a.redis, remoteConfig)
				} else if route.Algorithm == "token_bucket" {
					remote, err = redistoken.New(a.redis, redistoken.Config{Config: remoteConfig, Capacity: route.Capacity})
				} else {
					remote, err = rediswindow.New(a.redis, remoteConfig)
				}
				if err == nil {
					limiter = remote
					if err = remote.Ready(ctx); err == nil {
						a.readyChecks = append(a.readyChecks, remote.Ready)
						a.usages[route.Policy.ID] = append(a.usages[route.Policy.ID], remote.Usage)
					}
				}
			} else {
				var local interface {
					service.Limiter
					CleanupExpired() int
					Usage() float64
				}
				localConfig := fixedwindow.Config{Limit: route.Limit, Window: window, MaxKeys: route.MaxKeys}
				if route.Algorithm == "leaky_bucket" {
					local, err = leakybucket.New(leakybucket.Config{Limit: route.Limit, Window: window, MaxKeys: route.MaxKeys, Capacity: route.Capacity})
				} else if route.Algorithm == "sliding_window_counter" {
					local, err = slidingcounter.New(localConfig)
				} else if route.Algorithm == "sliding_window_log" {
					local, err = slidingwindow.New(localConfig)
				} else if route.Algorithm == "token_bucket" {
					local, err = tokenbucket.New(tokenbucket.Config{Limit: route.Limit, Window: window, Capacity: route.Capacity, MaxKeys: route.MaxKeys})
				} else {
					local, err = fixedwindow.New(localConfig)
				}
				if err == nil {
					limiter = local
					a.ram = append(a.ram, local)
					a.usages[route.Policy.ID] = append(a.usages[route.Policy.ID], func(context.Context) (float64, error) { return local.Usage(), nil })
				}
			}
			if err != nil {
				return nil, fmt.Errorf("proxy: policy %q initialization failed: %w", route.Policy.ID, err)
			}
			scoped.tenants[tenant] = limiter
		}
		var limiter service.Limiter = scoped
		registered[route.Policy.ID] = route
		bindings = append(bindings, service.Binding{Policy: route.Policy, Limiter: limiter})
	}
	s, err := service.New(bindings)
	if err != nil {
		return nil, err
	}
	a.transport = http.DefaultTransport.(*http.Transport).Clone()
	a.transport.Proxy = nil
	a.transport.MaxConnsPerHost = config.MaxConcurrent
	a.transport.MaxIdleConnsPerHost = config.MaxConcurrent
	a.transport.MaxIdleConns = config.MaxConcurrent
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
			slog.Warn("upstream request failed", "error_type", fmt.Sprintf("%T", err))
			var networkError net.Error
			if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &networkError) && networkError.Timeout()) {
				a.metrics.UpstreamErrors.WithLabelValues("timeout").Inc()
				writeError(w, 504, "upstream_timeout")
			} else {
				a.metrics.UpstreamErrors.WithLabelValues("unavailable").Inc()
				writeError(w, 502, "upstream_unavailable")
			}
		},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { health(w, r, true) })
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), a.redisTimeout)
		defer cancel()
		ready := a.Ready(ctx) == nil
		health(w, r, ready)
	})
	metricsToken := os.Getenv("RATE_LIMIT_METRICS_TOKEN")
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		expected := sha256.Sum256([]byte("Bearer " + metricsToken))
		supplied := sha256.Sum256([]byte(r.Header.Get("Authorization")))
		if metricsToken == "" || subtle.ConstantTimeCompare(expected[:], supplied[:]) != 1 {
			http.NotFound(w, r)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), a.redisTimeout)
		defer cancel()
		for policy, usages := range a.usages {
			maximum := 0.0
			valid := true
			for _, usage := range usages {
				value, err := usage(ctx)
				if err != nil {
					valid = false
					break
				}
				if value > maximum {
					maximum = value
				}
			}
			if valid {
				a.metrics.Capacity.WithLabelValues(policy).Set(maximum)
			} else {
				a.metrics.Capacity.DeleteLabelValues(policy)
			}
		}
		a.metrics.Handler().ServeHTTP(w, r)
	})
	slots := make(chan struct{}, config.MaxConcurrent)
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
		limited, err := httpmiddleware.Wrap(telemetry.Checker{Service: s, Metrics: a.metrics}, httpmiddleware.Config{Policy: route.Policy, Route: route.Pattern, AnonymousTenant: config.AnonymousTenant, TrustedProxies: proxies, Authenticator: authenticator}, forward)
		if err != nil {
			return nil, err
		}
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			recorder := &statusWriter{ResponseWriter: w, status: 200}
			w = recorder
			defer func() {
				a.metrics.HTTP.WithLabelValues(route.Pattern, strconv.Itoa(recorder.status)).Inc()
				slog.Info("request completed", "route", route.Pattern, "status", recorder.status, "duration_ms", time.Since(start).Milliseconds())
			}()
			select {
			case slots <- struct{}{}:
				defer func() { <-slots; a.metrics.Inflight.Dec() }()
				a.metrics.Inflight.Inc()
			default:
				writeError(w, 503, "gateway_overloaded")
				return
			}
			if r.Header.Get("Upgrade") != "" {
				writeError(w, 501, "protocol_upgrade_not_supported")
				return
			}
			if r.ContentLength > config.MaxBodyBytes {
				writeError(w, 413, "request_body_too_large")
				return
			}
			if r.Body != nil {
				body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, config.MaxBodyBytes))
				_ = r.Body.Close()
				if err != nil {
					var maxErr *http.MaxBytesError
					if errors.As(err, &maxErr) {
						writeError(w, 413, "request_body_too_large")
					} else {
						writeError(w, 400, "invalid_request_body")
					}
					return
				}
				r.Body = io.NopCloser(bytes.NewReader(body))
				r.ContentLength = int64(len(body))
			}
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
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			a.draining.Store(true)
		case <-done:
		}
	}()
	return httpserver.Serve(ctx, listener, a.handler, a.cleanupInterval, a.upstreamTimeout+5*time.Second, func() {
		for _, limiter := range a.ram {
			limiter.CleanupExpired()
		}
	})
}

// Ready probes all bound policies without consuming user allowances.
func (a *App) Ready(ctx context.Context) error {
	if a.draining.Load() {
		return fmt.Errorf("proxy: draining")
	}
	for _, check := range a.readyChecks {
		if err := check(ctx); err != nil {
			return err
		}
	}
	return nil
}

type statusWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (w *statusWriter) WriteHeader(code int) {
	if code >= 100 && code < 200 {
		w.ResponseWriter.WriteHeader(code)
		return
	}
	if !w.wrote {
		w.status = code
		w.wrote = true
		w.ResponseWriter.WriteHeader(code)
	}
}
func (w *statusWriter) Write(p []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(200)
	}
	return w.ResponseWriter.Write(p)
}
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
