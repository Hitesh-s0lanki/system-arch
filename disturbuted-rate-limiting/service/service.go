// Package service connects trusted request identities, policies, and algorithms.
package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	ratelimit "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/identity"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/policy"
)

var (
	ErrUnauthorizedTenant = errors.New("service: tenant is not provisioned")
	ErrUnknownPolicy      = errors.New("service: unknown policy")
	ErrInvalidRequest     = errors.New("service: invalid request identity")
)

// Limiter is the consumer's algorithm contract. Implementations must be safe for
// concurrent use. Errors represent backend failures, not ordinary rejections.
type Limiter interface {
	Evaluate(context.Context, string) (ratelimit.Decision, error)
}

// TenantLimiter provides independently bounded storage for each trusted tenant.
type TenantLimiter interface {
	EvaluateTenant(context.Context, string, string) (ratelimit.Decision, error)
}

// Binding connects a policy to a non-nil limiter. Prefer a dedicated limiter per
// policy when each policy needs independent capacity and algorithm settings.
type Binding struct {
	Policy  policy.Policy
	Limiter Limiter
}

// Request contains identities resolved by trusted application code. IP must be
// the resolved client address; APIKeyID is a credential ID, not the secret.
// Route is a canonical method plus route template, e.g. "POST /orders/{id}".
type Request struct {
	TenantID string
	IP       string
	UserID   string
	APIKeyID string
	Route    string
}

// Service holds an immutable policy registry, safe for concurrent checks.
type Service struct {
	bindings map[string]Binding
}

// New validates and copies the registry. Policy registration is startup-only.
func New(bindings []Binding) (*Service, error) {
	if len(bindings) == 0 {
		return nil, fmt.Errorf("service: at least one policy is required")
	}
	s := &Service{bindings: make(map[string]Binding, len(bindings))}
	for _, binding := range bindings {
		if err := binding.Policy.Validate(); err != nil {
			return nil, err
		}
		if nilLimiter(binding.Limiter) {
			return nil, fmt.Errorf("service: policy %q needs a limiter", binding.Policy.ID)
		}
		if _, exists := s.bindings[binding.Policy.ID]; exists {
			return nil, fmt.Errorf("service: duplicate policy %q", binding.Policy.ID)
		}
		s.bindings[binding.Policy.ID] = binding
	}
	return s, nil
}

// An interface containing a typed nil pointer is not itself nil.
func nilLimiter(limiter Limiter) bool {
	if limiter == nil {
		return true
	}
	value := reflect.ValueOf(limiter)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

// Check evaluates exactly one policy. The application selects policyID; it must
// not trust a client to choose a more permissive policy. Invalid requests consume
// no allowance. There is no implicit fallback between identity types.
func (s *Service) Check(ctx context.Context, policyID string, request Request) (ratelimit.Decision, error) {
	if err := ctx.Err(); err != nil {
		return ratelimit.Decision{}, err
	}
	binding, exists := s.bindings[policyID]
	if !exists {
		return ratelimit.Decision{}, fmt.Errorf("%w: %q", ErrUnknownPolicy, policyID)
	}
	subject := identity.Subject{Kind: binding.Policy.Identity}
	switch subject.Kind {
	case identity.IP:
		subject.Value = request.IP
	case identity.User:
		subject.Value = request.UserID
	case identity.APIKey:
		subject.Value = request.APIKeyID
	}
	route := ""
	if binding.Policy.PerRoute {
		if strings.TrimSpace(request.Route) == "" {
			return ratelimit.Decision{}, fmt.Errorf("%w: canonical route required", ErrInvalidRequest)
		}
		route = request.Route
	}
	key, err := identity.Key(policyID, request.TenantID, subject, route)
	if err != nil {
		return ratelimit.Decision{}, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}
	if scoped, ok := binding.Limiter.(TenantLimiter); ok {
		return scoped.EvaluateTenant(ctx, request.TenantID, key)
	}
	return binding.Limiter.Evaluate(ctx, key)
}
