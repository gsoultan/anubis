package authctx

import (
	"net"
	"net/netip"
	"strings"
)

// ProxyTrust decides which address is the CLIENT's.
//
// By default the peer address is the answer and X-Forwarded-For is ignored:
// that header is trivially forged, and believing it from an arbitrary caller
// hands an attacker the ability to spend somebody else's rate-limit budget
// or to write a false address into the audit log.
//
// But TLS termination means a proxy, and behind one the peer address is the
// PROXY on every request — so a per-IP limit stops bounding one client and
// starts bounding the whole installation, which is an outage waiting for a
// busy morning rather than a security control. Naming the proxies resolves
// it: the header is believed when, and only when, the peer is one of them.
type ProxyTrust struct {
	nets []netip.Prefix
}

// NewProxyTrust parses a comma-separated list of CIDRs (or bare addresses,
// which are taken as single hosts). An empty list trusts nothing, which is
// the default and the safe answer for a directly exposed server.
func NewProxyTrust(list string) (*ProxyTrust, error) {
	t := &ProxyTrust{}
	for _, raw := range strings.Split(list, ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if p, err := netip.ParsePrefix(raw); err == nil {
			t.nets = append(t.nets, p)
			continue
		}
		addr, err := netip.ParseAddr(raw)
		if err != nil {
			return nil, err
		}
		t.nets = append(t.nets, netip.PrefixFrom(addr, addr.BitLen()))
	}
	return t, nil
}

// Trusted reports whether an address is one of the named proxies.
func (t *ProxyTrust) Trusted(ip string) bool {
	if t == nil || len(t.nets) == 0 {
		return false
	}
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	addr = addr.Unmap()
	for _, n := range t.nets {
		if n.Contains(addr) {
			return true
		}
	}
	return false
}

// ClientIPFrom returns the address to treat as the caller's.
//
// When the peer is a trusted proxy, it walks X-Forwarded-For from the RIGHT
// and returns the first address that is not itself trusted. Right-to-left is
// the only correct direction: everything on the left is whatever the client
// chose to send, so a client that prepends "X-Forwarded-For: 1.2.3.4" cannot
// make itself look like 1.2.3.4 — it can only add hops that are skipped past.
func ClientIPFrom(peerAddr, forwardedFor string, trust *ProxyTrust) string {
	peer := peerAddr
	if host, _, err := net.SplitHostPort(peerAddr); err == nil {
		peer = host
	}
	if !trust.Trusted(peer) || forwardedFor == "" {
		return peer
	}
	parts := strings.Split(forwardedFor, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		hop := strings.TrimSpace(parts[i])
		// A port may be present on IPv6 or from sloppy proxies.
		if host, _, err := net.SplitHostPort(hop); err == nil {
			hop = host
		}
		hop = strings.Trim(hop, "[]")
		if hop == "" {
			continue
		}
		if _, err := netip.ParseAddr(hop); err != nil {
			// Not an address: an obfuscated or malformed hop. Stop rather
			// than skip past it — beyond it nothing is verifiable.
			return peer
		}
		if !trust.Trusted(hop) {
			return hop
		}
	}
	// Every hop was a trusted proxy; the nearest one is the best answer.
	return peer
}
