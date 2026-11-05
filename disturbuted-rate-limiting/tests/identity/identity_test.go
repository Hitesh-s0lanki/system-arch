package identity_test

import (
	"strings"
	"testing"

	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/identity"
)

func TestKeyIPNormalization(t *testing.T) {
	var first string
	for _, ip := range []string{"192.0.2.1", "::ffff:192.0.2.1"} {
		key, err := identity.Key("policy", "tenant", identity.Subject{Kind: identity.IP, Value: ip}, "")
		if err != nil {
			t.Fatal(err)
		}
		if first != "" && key != first {
			t.Fatal("equivalent IPv4 representations received different keys")
		}
		first = key
		if len(key) != 70 {
			t.Fatalf("key length: got %d, want 70", len(key))
		}
	}
}

func TestKeyRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name, policy, tenant, value, route string
		kind                               identity.Kind
	}{
		{"blank policy", "", "t", "u", "", identity.User},
		{"long policy", strings.Repeat("p", 65), "t", "u", "", identity.User},
		{"blank tenant", "p", " ", "u", "", identity.User},
		{"long tenant", "p", strings.Repeat("t", 129), "u", "", identity.User},
		{"blank subject", "p", "t", "", "", identity.User},
		{"long subject", "p", "t", strings.Repeat("u", 257), "", identity.User},
		{"invalid IP", "p", "t", "not-an-ip", "", identity.IP},
		{"IP with port", "p", "t", "192.0.2.1:80", "", identity.IP},
		{"IP with zone", "p", "t", "fe80::1%en0", "", identity.IP},
		{"unknown kind", "p", "t", "u", "", identity.Kind("other")},
		{"blank route", "p", "t", "u", " ", identity.User},
		{"long route", "p", "t", "u", strings.Repeat("r", 257), identity.User},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := identity.Key(tt.policy, tt.tenant, identity.Subject{Kind: tt.kind, Value: tt.value}, tt.route); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestKeyComponentBoundaries(t *testing.T) {
	// Delimiter-containing components must not produce the same key.
	a, err := identity.Key("a:b", "c", identity.Subject{Kind: identity.User, Value: "u"}, "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := identity.Key("a", "b:c", identity.Subject{Kind: identity.User, Value: "u"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("different component boundaries collided")
	}
}
