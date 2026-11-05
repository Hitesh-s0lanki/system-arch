package policy_test

import (
	"strings"
	"testing"

	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/identity"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/policy"
)

func TestValidate(t *testing.T) {
	for _, kind := range []identity.Kind{identity.IP, identity.User, identity.APIKey} {
		if err := (policy.Policy{ID: "p", Identity: kind}).Validate(); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []policy.Policy{
		{Identity: identity.User},
		{ID: strings.Repeat("p", 65), Identity: identity.User},
		{ID: "p", Identity: "unknown"},
	} {
		if err := p.Validate(); err == nil {
			t.Fatalf("expected invalid policy: %+v", p)
		}
	}
}
