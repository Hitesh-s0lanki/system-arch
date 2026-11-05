package httpmiddleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	ratelimit "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/auth"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/identity"
	httpmiddleware "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/middleware/http"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/policy"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/service"
)

type checkFunc func(context.Context, string, service.Request) (ratelimit.Decision, error)

func (f checkFunc) Check(c context.Context, p string, r service.Request) (ratelimit.Decision, error) {
	return f(c, p, r)
}

type authFunc func(context.Context, string) (auth.Principal, error)

func (f authFunc) Authenticate(c context.Context, s string) (auth.Principal, error) { return f(c, s) }

func TestRejectionResponses(t *testing.T) {
	for _, tt := range []struct {
		name     string
		decision ratelimit.Decision
		err      error
		status   int
		retry    string
	}{
		{"allowed", ratelimit.Decision{Allowed: true, Remaining: 2}, nil, 204, ""},
		{"rate limited", ratelimit.Decision{Reason: ratelimit.ReasonRateLimited, RetryAfter: 1500 * time.Millisecond}, nil, 429, "2"},
		{"capacity", ratelimit.Decision{Reason: ratelimit.ReasonCapacityExceeded}, nil, 503, ""},
		{"invalid backend key", ratelimit.Decision{Reason: ratelimit.ReasonInvalidKey}, nil, 500, ""},
		{"backend error", ratelimit.Decision{}, errors.New("private backend error"), 503, ""},
		{"unknown policy", ratelimit.Decision{}, service.ErrUnknownPolicy, 500, ""},
		{"invalid service identity", ratelimit.Decision{}, service.ErrInvalidRequest, 500, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			h, err := httpmiddleware.Wrap(checkFunc(func(context.Context, string, service.Request) (ratelimit.Decision, error) { return tt.decision, tt.err }),
				httpmiddleware.Config{Policy: policy.Policy{ID: "ip", Identity: identity.IP}, AnonymousTenant: "public"},
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true; w.WriteHeader(204) }))
			if err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest("GET", "/", nil)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tt.status || called != tt.decision.Allowed {
				t.Fatalf("status=%d handler=%v", w.Code, called)
			}
			if got := w.Header().Get("Retry-After"); got != tt.retry {
				t.Fatalf("Retry-After=%q want %q", got, tt.retry)
			}
			if strings.Contains(w.Body.String(), "private backend error") {
				t.Fatal("backend details leaked")
			}
			if tt.status != 204 && w.Header().Get("Content-Type") != "application/json" {
				t.Fatal("error response is not JSON")
			}
		})
	}
}

func TestAuthenticationBoundary(t *testing.T) {
	for _, tt := range []struct {
		name, header string
		principal    auth.Principal
		authErr      error
		status       int
	}{
		{"missing token", "", auth.Principal{}, nil, 401},
		{"wrong scheme", "Basic abc", auth.Principal{}, nil, 401},
		{"invalid token", "Bearer invalid", auth.Principal{}, auth.ErrUnauthenticated, 401},
		{"auth unavailable", "Bearer valid", auth.Principal{}, errors.New("auth backend failure"), 503},
		{"wrong identity", "Bearer valid", auth.Principal{TenantID: "t", APIKeyID: "key"}, nil, 403},
		{"verified user", "Bearer valid", auth.Principal{TenantID: "t", UserID: "verified"}, nil, 204},
	} {
		t.Run(tt.name, func(t *testing.T) {
			checks := 0
			h, err := httpmiddleware.Wrap(checkFunc(func(_ context.Context, _ string, r service.Request) (ratelimit.Decision, error) {
				checks++
				if r.UserID != "verified" || r.TenantID != "t" {
					t.Errorf("untrusted identity used: %+v", r)
				}
				return ratelimit.Decision{Allowed: true}, nil
			}), httpmiddleware.Config{Policy: policy.Policy{ID: "users", Identity: identity.User}, Authenticator: authFunc(func(context.Context, string) (auth.Principal, error) { return tt.principal, tt.authErr })}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				principal, ok := httpmiddleware.PrincipalFromContext(r.Context())
				if !ok || principal != tt.principal {
					t.Errorf("verified principal missing from handler context: %+v", principal)
				}
				w.WriteHeader(204)
			}))
			if err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest("GET", "/", nil)
			r.Header.Set("Authorization", tt.header)
			r.Header.Set("X-User-ID", "forged")
			r.Header.Set("X-Tenant-ID", "forged")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tt.status {
				t.Fatalf("status=%d want %d", w.Code, tt.status)
			}
			if tt.status != 204 && checks != 0 {
				t.Fatal("failed authentication consumed allowance")
			}
			if tt.status == 401 && w.Header().Get("WWW-Authenticate") == "" {
				t.Fatal("missing authentication challenge")
			}
		})
	}
}

func TestMiddlewareConfigValidation(t *testing.T) {
	checker := checkFunc(func(context.Context, string, service.Request) (ratelimit.Decision, error) {
		return ratelimit.Decision{Allowed: true}, nil
	})
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	for _, config := range []httpmiddleware.Config{
		{},
		{Policy: policy.Policy{ID: "u", Identity: identity.User}},
		{Policy: policy.Policy{ID: "ip", Identity: identity.IP}},
		{Policy: policy.Policy{ID: "ip", Identity: identity.IP, PerRoute: true}, AnonymousTenant: "public"},
	} {
		if _, err := httpmiddleware.Wrap(checker, config, next); err == nil {
			t.Fatal("invalid config accepted")
		}
	}
	valid := httpmiddleware.Config{Policy: policy.Policy{ID: "ip", Identity: identity.IP}, AnonymousTenant: "public"}
	var absent *service.Service
	if _, err := httpmiddleware.Wrap(absent, valid, next); err == nil {
		t.Fatal("typed nil checker accepted")
	}
	var absentHandler *http.ServeMux
	if _, err := httpmiddleware.Wrap(checker, valid, absentHandler); err == nil {
		t.Fatal("typed nil handler accepted")
	}
}

func TestDuplicateAuthorizationRejected(t *testing.T) {
	called := false
	h, err := httpmiddleware.Wrap(checkFunc(func(context.Context, string, service.Request) (ratelimit.Decision, error) {
		called = true
		return ratelimit.Decision{Allowed: true}, nil
	}),
		httpmiddleware.Config{Policy: policy.Policy{ID: "users", Identity: identity.User}, Authenticator: authFunc(func(context.Context, string) (auth.Principal, error) { called = true; return auth.Principal{}, nil })},
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Add("Authorization", "Bearer one")
	r.Header.Add("Authorization", "Bearer two")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 401 || called {
		t.Fatalf("status=%d downstream called=%v", w.Code, called)
	}
}
