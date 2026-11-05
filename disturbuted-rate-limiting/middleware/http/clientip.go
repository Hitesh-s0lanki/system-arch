package httpmiddleware

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

func trusted(address netip.Addr, prefixes []netip.Prefix) bool {
	for _, prefix := range prefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

// ClientIP ignores forwarding headers unless the socket peer is trusted. It
// walks X-Forwarded-For right to left, stopping at the first untrusted hop.
// Trusted proxies must append or overwrite that header with the observed peer.
func ClientIP(r *http.Request, proxies []netip.Prefix) (string, error) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil || peer.Zone() != "" {
		return "", fmt.Errorf("invalid socket peer")
	}
	peer = peer.Unmap()
	if !trusted(peer, proxies) {
		return peer.String(), nil
	}
	forwarded := strings.Join(r.Header.Values("X-Forwarded-For"), ",")
	if forwarded == "" {
		return peer.String(), nil
	}
	if len(forwarded) > 8192 {
		return "", fmt.Errorf("forwarded chain too long")
	}
	parts := strings.Split(forwarded, ",")
	if len(parts) > 32 {
		return "", fmt.Errorf("too many forwarded hops")
	}
	addresses := make([]netip.Addr, len(parts))
	for i, part := range parts {
		address, err := netip.ParseAddr(strings.TrimSpace(part))
		if err != nil || address.Zone() != "" {
			return "", fmt.Errorf("invalid forwarded address")
		}
		addresses[i] = address.Unmap()
	}
	current := peer
	for i := len(addresses) - 1; i >= 0 && trusted(current, proxies); i-- {
		current = addresses[i]
	}
	return current.String(), nil
}
