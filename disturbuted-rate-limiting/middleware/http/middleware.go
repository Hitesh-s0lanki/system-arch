// Package httpmiddleware connects net/http handlers to the rate-limit service.
package httpmiddleware

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"reflect"
	"strconv"
	"strings"
	"time"

	ratelimit "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/auth"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/identity"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/policy"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/service"
)

type Checker interface {
	Check(context.Context, string, service.Request) (ratelimit.Decision, error)
}

type Authenticator interface {
	Authenticate(context.Context, string) (auth.Principal, error)
}

type principalKey struct{}

// PrincipalFromContext returns the identity verified by authenticated middleware.
// Anonymous IP policies do not populate a principal.
func PrincipalFromContext(ctx context.Context) (auth.Principal, bool) {
	principal, ok := ctx.Value(principalKey{}).(auth.Principal)
	return principal, ok
}

// Config is fixed by application code, never by request headers. AnonymousTenant
// applies only to IP policies. Route must match the router's canonical pattern.
type Config struct {
	Policy          policy.Policy
	Route           string
	AnonymousTenant string
	TrustedProxies  []netip.Prefix
	Authenticator   Authenticator
}

// Wrap builds one route's middleware. Use the same policy in the service registry.
func Wrap(checker Checker, config Config, next http.Handler) (http.Handler, error) {
	if nilValue(checker) || nilValue(next) {
		return nil, fmt.Errorf("httpmiddleware: checker and handler required")
	}
	if err := config.Policy.Validate(); err != nil {
		return nil, err
	}
	if config.Policy.PerRoute && strings.TrimSpace(config.Route) == "" {
		return nil, fmt.Errorf("httpmiddleware: canonical route required")
	}
	if len(config.Route) > 256 {
		return nil, fmt.Errorf("httpmiddleware: route too long")
	}
	if config.Policy.Identity == identity.IP {
		if _, err := identity.Key(config.Policy.ID, config.AnonymousTenant, identity.Subject{Kind: identity.IP, Value: "127.0.0.1"}, ""); err != nil {
			return nil, err
		}
	} else if nilValue(config.Authenticator) {
		return nil, fmt.Errorf("httpmiddleware: authenticator required")
	}
	config.TrustedProxies = append([]netip.Prefix(nil), config.TrustedProxies...)
	for _, prefix := range config.TrustedProxies {
		if !prefix.IsValid() {
			return nil, fmt.Errorf("httpmiddleware: invalid proxy prefix")
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		input := service.Request{Route: config.Route}
		if config.Policy.Identity == identity.IP {
			ip, err := ClientIP(r, config.TrustedProxies)
			if err != nil {
				respond(w, http.StatusBadRequest, "invalid_client_ip")
				return
			}
			input.TenantID, input.IP = config.AnonymousTenant, ip
		} else {
			values := r.Header.Values("Authorization")
			var fields []string
			if len(values) == 1 && len(values[0]) <= 4103 {
				fields = strings.Fields(values[0])
			}
			if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") {
				unauthorized(w)
				return
			}
			principal, err := config.Authenticator.Authenticate(r.Context(), fields[1])
			if errors.Is(err, auth.ErrUnauthenticated) {
				unauthorized(w)
				return
			}
			if err != nil {
				respond(w, http.StatusServiceUnavailable, "authentication_unavailable")
				return
			}
			if (config.Policy.Identity == identity.User && principal.UserID == "") || (config.Policy.Identity == identity.APIKey && principal.APIKeyID == "") {
				respond(w, http.StatusForbidden, "identity_not_authorized")
				return
			}
			input.TenantID, input.UserID, input.APIKeyID = principal.TenantID, principal.UserID, principal.APIKeyID
			r = r.WithContext(context.WithValue(r.Context(), principalKey{}, principal))
		}
		decision, err := checker.Check(r.Context(), config.Policy.ID, input)
		if err != nil {
			switch {
			case errors.Is(err, service.ErrUnauthorizedTenant):
				respond(w, http.StatusForbidden, "tenant_not_authorized")
			case errors.Is(err, service.ErrUnknownPolicy), errors.Is(err, service.ErrInvalidRequest):
				respond(w, http.StatusInternalServerError, "rate_limit_configuration_error")
			default:
				respond(w, http.StatusServiceUnavailable, "rate_limiter_unavailable")
			}
			return
		}
		if decision.Allowed {
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(decision.Remaining))
			next.ServeHTTP(w, r)
			return
		}
		switch decision.Reason {
		case ratelimit.ReasonRateLimited:
			w.Header().Set("X-RateLimit-Remaining", "0")
			if decision.RetryAfter > 0 {
				seconds := (decision.RetryAfter-1)/time.Second + 1
				w.Header().Set("Retry-After", strconv.FormatInt(int64(seconds), 10))
			}
			respond(w, http.StatusTooManyRequests, "rate_limited")
		case ratelimit.ReasonCapacityExceeded:
			respond(w, http.StatusServiceUnavailable, "rate_limit_capacity_exceeded")
		default:
			respond(w, http.StatusInternalServerError, "rate_limit_configuration_error")
		}
	}), nil
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="api"`)
	respond(w, http.StatusUnauthorized, "unauthenticated")
}

func respond(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
}

func nilValue(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}
