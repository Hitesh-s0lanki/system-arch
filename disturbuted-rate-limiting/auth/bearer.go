// Package auth supplies an authentication hook and a static bearer-token adapter.
// Static credentials are intended for local wiring or small controlled setups;
// applications can implement the same method using their own identity provider.
package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"

	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/identity"
)

var ErrUnauthenticated = errors.New("auth: invalid credentials")

// Principal contains verified identifiers, never a raw credential secret.
type Principal struct {
	TenantID string `json:"tenant_id"`
	UserID   string `json:"user_id"`
	APIKeyID string `json:"api_key_id"`
}

// Credential is startup configuration. Never log Token or commit real tokens.
type Credential struct {
	Token string `json:"token"`
	Principal
}

type entry struct {
	digest    [32]byte
	principal Principal
}

// Bearer holds an immutable set of hashed tokens and verified identities.
type Bearer struct{ entries []entry }

func NewBearer(credentials []Credential) (*Bearer, error) {
	b := &Bearer{}
	seen := make(map[[32]byte]bool)
	for _, credential := range credentials {
		if len(credential.Token) < 32 || len(credential.Token) > 4096 || strings.IndexFunc(credential.Token, func(r rune) bool { return r <= 0x20 || r >= 0x7f }) >= 0 {
			return nil, fmt.Errorf("auth: bearer tokens must be 32–4096 printable ASCII bytes without whitespace")
		}
		p := credential.Principal
		if p.UserID == "" && p.APIKeyID == "" {
			return nil, fmt.Errorf("auth: a user or credential ID is required")
		}
		for kind, value := range map[identity.Kind]string{identity.User: p.UserID, identity.APIKey: p.APIKeyID} {
			if value == "" {
				continue
			}
			if _, err := identity.Key("auth", p.TenantID, identity.Subject{Kind: kind, Value: value}, ""); err != nil {
				return nil, err
			}
		}
		digest := sha256.Sum256([]byte(credential.Token))
		if seen[digest] {
			return nil, fmt.Errorf("auth: duplicate bearer token")
		}
		seen[digest] = true
		b.entries = append(b.entries, entry{digest, p})
	}
	return b, nil
}

// Authenticate verifies a token without retaining it in limiter state.
func (b *Bearer) Authenticate(ctx context.Context, token string) (Principal, error) {
	if err := ctx.Err(); err != nil {
		return Principal{}, err
	}
	if len(token) < 32 || len(token) > 4096 {
		return Principal{}, ErrUnauthenticated
	}
	digest := sha256.Sum256([]byte(token))
	var principal Principal
	found := 0
	for _, entry := range b.entries {
		match := subtle.ConstantTimeCompare(digest[:], entry.digest[:])
		if match == 1 {
			principal = entry.principal
		}
		found |= match
	}
	if found == 0 {
		return Principal{}, ErrUnauthenticated
	}
	return principal, nil
}
