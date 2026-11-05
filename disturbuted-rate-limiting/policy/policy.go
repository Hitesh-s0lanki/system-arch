// Package policy defines how identities are grouped for rate limiting.
package policy

import (
	"fmt"
	"strings"

	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/identity"
)

// Policy selects the identity and whether each route has its own allowance.
// The bound limiter owns algorithm-specific settings such as rate and capacity.
type Policy struct {
	ID       string        `json:"id"`
	Identity identity.Kind `json:"identity"`
	PerRoute bool          `json:"per_route"`
}

// Validate checks configuration before a service starts handling requests.
func (p Policy) Validate() error {
	if strings.TrimSpace(p.ID) == "" || len(p.ID) > 64 {
		return fmt.Errorf("policy: ID must be nonblank and at most 64 bytes")
	}
	if !p.Identity.Valid() {
		return fmt.Errorf("policy: unsupported identity kind %q", p.Identity)
	}
	return nil
}
