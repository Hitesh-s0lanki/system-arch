// Package identity validates rate-limit identities and constructs bounded keys.
package identity

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
)

// Kind identifies the source of a rate-limit identity.
type Kind string

const (
	IP     Kind = "ip"
	User   Kind = "user"
	APIKey Kind = "api_key"
)

// Valid reports whether the kind is supported.
func (k Kind) Valid() bool {
	return k == IP || k == User || k == APIKey
}

// Subject is an already-resolved identity. APIKey values must be credential IDs,
// never raw secrets. User and credential IDs must come from verified authentication.
type Subject struct {
	Kind  Kind
	Value string
}

// Key identifies a policy, tenant, subject, and optional canonical route.
// Components are length-prefixed before hashing to prevent delimiter collisions.
// The result has fixed length; hashing does not authenticate or anonymize inputs.
func Key(policyID, tenantID string, subject Subject, route string) (string, error) {
	if !subject.Kind.Valid() {
		return "", fmt.Errorf("identity: unsupported kind %q", subject.Kind)
	}
	for _, component := range []struct {
		name  string
		value string
		max   int
	}{
		{"policy ID", policyID, 64},
		{"tenant ID", tenantID, 128},
		{"subject", subject.Value, 256},
	} {
		if strings.TrimSpace(component.value) == "" || len(component.value) > component.max {
			return "", fmt.Errorf("identity: %s must be nonblank and at most %d bytes", component.name, component.max)
		}
	}
	if len(route) > 256 || (route != "" && strings.TrimSpace(route) == "") {
		return "", fmt.Errorf("identity: route must be nonblank when supplied and at most 256 bytes")
	}
	if subject.Kind == IP {
		address, err := netip.ParseAddr(subject.Value)
		if err != nil || address.Zone() != "" {
			return "", fmt.Errorf("identity: IP must be an address without a port or zone")
		}
		subject.Value = address.Unmap().String()
	}

	var encoded strings.Builder
	for _, part := range []string{policyID, tenantID, string(subject.Kind), subject.Value, route} {
		encoded.WriteString(strconv.Itoa(len(part)))
		encoded.WriteByte(':')
		encoded.WriteString(part)
	}
	sum := sha256.Sum256([]byte(encoded.String()))
	return "rl:v1:" + hex.EncodeToString(sum[:]), nil
}
