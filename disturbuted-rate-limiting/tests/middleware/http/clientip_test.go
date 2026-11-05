package httpmiddleware_test

import (
	"net/http/httptest"
	"net/netip"
	"testing"

	httpmiddleware "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/middleware/http"
)

func TestClientIPTrustBoundary(t *testing.T) {
	proxies := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	for _, tt := range []struct {
		name, peer, forwarded, want string
		invalid                     bool
	}{
		{"direct spoof", "192.0.2.1:1234", "198.51.100.1", "192.0.2.1", false},
		{"direct malformed ignored", "192.0.2.1:1234", "garbage", "192.0.2.1", false},
		{"trusted proxy", "10.0.0.1:1234", "192.0.2.1", "192.0.2.1", false},
		{"trusted chain", "10.0.0.1:1234", "192.0.2.1, 10.0.0.2", "192.0.2.1", false},
		{"forged leftmost", "10.0.0.1:1234", "198.51.100.9, 192.0.2.1", "192.0.2.1", false},
		{"no header", "10.0.0.1:1234", "", "10.0.0.1", false},
		{"IPv6", "[2001:db8::1]:1234", "", "2001:db8::1", false},
		{"mapped IPv4", "[::ffff:192.0.2.1]:1234", "", "192.0.2.1", false},
		{"invalid trusted chain", "10.0.0.1:1234", "garbage", "", true},
		{"invalid peer", "garbage", "", "", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = tt.peer
			r.Header.Set("X-Forwarded-For", tt.forwarded)
			got, err := httpmiddleware.ClientIP(r, proxies)
			if (err != nil) != tt.invalid || got != tt.want {
				t.Fatalf("got %q, %v; want %q invalid=%v", got, err, tt.want, tt.invalid)
			}
		})
	}
}
