package middleware

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// VPNGate is the first auth layer. It replaces the old Tailscale header check:
// a request is only served if its real client IP sits inside one of the
// WireGuard subnets (e.g. the wg-easy peer range 10.8.0.0/24). Anything else
// gets a bare 404 so the app does not reveal itself.
//
// The server normally sits behind Caddy on loopback, so the client IP is taken
// from the rightmost X-Forwarded-For entry, but only when the direct peer is a
// trusted proxy. Caddy overwrites X-Forwarded-For with the address it saw, so a
// peer cannot spoof its way in by sending the header itself.
type VPNGate struct {
	allowed []netip.Prefix
	trusted []netip.Prefix
}

// NewVPNGate parses the allowed client subnets and trusted proxy subnets.
func NewVPNGate(allowedCIDRs, trustedProxies []string) (*VPNGate, error) {
	allowed, err := parsePrefixes(allowedCIDRs)
	if err != nil {
		return nil, fmt.Errorf("vpn_cidrs: %w", err)
	}
	if len(allowed) == 0 {
		return nil, fmt.Errorf("vpn_cidrs: at least one subnet is required")
	}
	trusted, err := parsePrefixes(trustedProxies)
	if err != nil {
		return nil, fmt.Errorf("trusted_proxies: %w", err)
	}
	return &VPNGate{allowed: allowed, trusted: trusted}, nil
}

func parsePrefixes(list []string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, s := range list {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return nil, err
		}
		out = append(out, p.Masked())
	}
	return out, nil
}

func contains(list []netip.Prefix, ip netip.Addr) bool {
	for _, p := range list {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

// ClientIP returns the real client address for r, or an invalid Addr if it
// cannot be determined.
func (g *VPNGate) ClientIP(r *http.Request) netip.Addr {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}
	}
	peer = peer.Unmap()
	if !contains(g.trusted, peer) {
		return peer
	}
	xff := r.Header.Values("X-Forwarded-For")
	if len(xff) == 0 {
		return peer
	}
	parts := strings.Split(xff[len(xff)-1], ",")
	ip, err := netip.ParseAddr(strings.TrimSpace(parts[len(parts)-1]))
	if err != nil {
		return netip.Addr{}
	}
	return ip.Unmap()
}

// Middleware rejects any request whose client IP is outside the VPN subnets.
// On success, r.RemoteAddr is rewritten to the bare client IP so downstream
// code (rate limiting, logging) keys on the real peer instead of the proxy.
// The health endpoint is also open to loopback for container health checks.
func (g *VPNGate) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := g.ClientIP(r)
		ok := ip.IsValid() && contains(g.allowed, ip)
		if !ok && r.URL.Path == "/api/health" && ip.IsLoopback() {
			ok = true
		}
		if !ok {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
		r.RemoteAddr = ip.String()
		next.ServeHTTP(w, r)
	})
}
