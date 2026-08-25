package authctx

import "testing"

func trust(t *testing.T, list string) *ProxyTrust {
	t.Helper()
	pt, err := NewProxyTrust(list)
	if err != nil {
		t.Fatalf("parse %q: %v", list, err)
	}
	return pt
}

// The header is worthless from an arbitrary caller: believing it would let
// anyone spend another address's rate-limit budget and write a false address
// into the audit log.
func TestForwardedForIgnoredFromAnUntrustedPeer(t *testing.T) {
	got := ClientIPFrom("203.0.113.9:5555", "1.2.3.4", trust(t, ""))
	if got != "203.0.113.9" {
		t.Fatalf("trusted a forged header: got %q", got)
	}
	// Even with SOME proxies configured, a peer outside the list is not one.
	got = ClientIPFrom("203.0.113.9:5555", "1.2.3.4", trust(t, "10.0.0.0/8"))
	if got != "203.0.113.9" {
		t.Fatalf("believed a header from a peer that is not a proxy: got %q", got)
	}
}

// Behind a named proxy the client's own address is what matters — otherwise
// every request shares the proxy's rate-limit bucket and one attacker locks
// out the whole installation.
func TestForwardedForUsedFromATrustedProxy(t *testing.T) {
	got := ClientIPFrom("10.0.0.5:443", "198.51.100.7", trust(t, "10.0.0.0/8"))
	if got != "198.51.100.7" {
		t.Fatalf("did not take the client address: got %q", got)
	}
}

// A client that prepends hops must not be able to choose its own identity.
// Walking from the right skips only addresses added by trusted proxies.
func TestSpoofedHopsOnTheLeftAreSkipped(t *testing.T) {
	// The client claims to be 1.2.3.4; the proxy appended the truth.
	got := ClientIPFrom("10.0.0.5:443", "1.2.3.4, 198.51.100.7", trust(t, "10.0.0.0/8"))
	if got == "1.2.3.4" {
		t.Fatal("a client chose its own address by prepending a hop")
	}
	if got != "198.51.100.7" {
		t.Fatalf("want the rightmost untrusted hop, got %q", got)
	}
}

// Chained proxies: skip the ones we know, stop at the first we do not.
func TestChainedProxiesResolveToTheRealClient(t *testing.T) {
	got := ClientIPFrom("10.0.0.5:443", "198.51.100.7, 10.0.0.9, 10.0.0.5",
		trust(t, "10.0.0.0/8"))
	if got != "198.51.100.7" {
		t.Fatalf("want the client behind the chain, got %q", got)
	}
}

// Garbage must fail closed to the peer, not be passed along as an address.
func TestMalformedHopFallsBackToThePeer(t *testing.T) {
	got := ClientIPFrom("10.0.0.5:443", "not-an-address", trust(t, "10.0.0.0/8"))
	if got != "10.0.0.5" {
		t.Fatalf("want the peer, got %q", got)
	}
}

func TestSingleHostAndIPv6Trust(t *testing.T) {
	if got := ClientIPFrom("127.0.0.1:8080", "198.51.100.7", trust(t, "127.0.0.1")); got != "198.51.100.7" {
		t.Fatalf("bare address should parse as a single host: got %q", got)
	}
	if got := ClientIPFrom("[::1]:8080", "198.51.100.7", trust(t, "::1/128")); got != "198.51.100.7" {
		t.Fatalf("ipv6 loopback proxy not trusted: got %q", got)
	}
}
