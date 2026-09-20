// Package egress is the one policy for "Anubis is about to connect to a host
// somebody configured".
//
// Two features need it and neither may have its own copy: a scope feed reads
// an organisation's structure from an ERP, and a catalog source reads an
// application's permissions and roles from wherever that team publishes
// them. A guard that exists twice is a guard that will differ, and the half
// that is weaker is the one an attacker finds.
package egress

import (
	"net"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/gsoultan/anubis/internal/shared/apperr"
)

// A sync source names a host Anubis will then connect to, and "any host" is
// the point of the feature: the truth about an organisation's structure
// lives in an ERP, a CRM, somebody's warehouse, and none of those are here.
//
// It is also a request from a configured string to an outbound connection,
// which is the shape of every SSRF. The authority to configure one is high
// (anubis:sync:admin, operators only), so this is not the last line of
// defence — but the cloud metadata endpoint hands out credentials to anyone
// who can make a plain GET, and no structure feed has ever lived at
// 169.254.169.254.
//
// Default: link-local and this machine's loopback are refused, everything
// else is allowed. ANUBIS_SYNC_DENY_HOSTS extends it with CIDRs for an
// installation that wants its own internal ranges off limits;
// ANUBIS_SYNC_ALLOW_LOOPBACK re-opens loopback for a development machine
// where the "external" database is a container on the same host.
// Timeout bounds a single conversation with an external source.
const Timeout = 60 * time.Second

// alwaysDenied is not configurable. Reaching a metadata service from a
// structure feed is not a use case anyone has.
//
// fd00:ec2::/32 is the correction. This list used to be 169.254.0.0/16 and
// fe80::/10, described as "its IPv6 equivalent" — which is wrong: AWS serves
// IMDS over IPv6 at fd00:ec2::254, a UNIQUE-LOCAL address, and nothing in
// fe80::/10 covers it. A redirect to that address went straight through.
//
// The rest of fd00::/8 stays allowed, deliberately. Unique-local is the IPv6
// counterpart of RFC1918, and a feed living at https://erp.internal on a
// private network is the case this whole feature exists for — denying the
// range wholesale would close the hole by removing the feature. Private IPv4
// is allowed for the same reason.
var alwaysDenied = []string{"169.254.0.0/16", "fe80::/10", "fd00:ec2::/32"}

// AllowHost refuses a hostname whose addresses are off limits. It resolves
// the name, because the policy is about where the packet goes and a name is
// not where the packet goes.
func AllowHost(host string) error {
	if host == "" {
		return apperr.ErrInvalidArgument.With("dsn", "no host")
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		// Name resolution failing is the feed being unreachable, not the
		// operator getting the policy wrong.
		return apperr.ErrUnavailableFeed.With("host", host).Wrap(err)
	}
	for _, ip := range ips {
		if err := allowIP(ip, host); err != nil {
			return err
		}
	}
	return nil
}

func allowIP(ip net.IP, host string) error {
	for _, cidr := range alwaysDenied {
		if _, n, err := net.ParseCIDR(cidr); err == nil && n.Contains(ip) {
			return apperr.ErrInvalidArgument.
				With("host", host).
				With("reason", "link-local addresses carry cloud metadata services, never structure feeds")
		}
	}
	if ip.IsLoopback() && os.Getenv("ANUBIS_SYNC_ALLOW_LOOPBACK") != "1" {
		return apperr.ErrInvalidArgument.
			With("host", host).
			With("reason", "loopback is this server; set ANUBIS_SYNC_ALLOW_LOOPBACK=1 for a local development source")
	}
	for _, cidr := range strings.Split(os.Getenv("ANUBIS_SYNC_DENY_HOSTS"), ",") {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}
		_, n, err := net.ParseCIDR(cidr)
		if err != nil {
			continue // a malformed entry must not silently deny everything
		}
		if n.Contains(ip) {
			return apperr.ErrInvalidArgument.
				With("host", host).
				With("reason", "denied by ANUBIS_SYNC_DENY_HOSTS ("+cidr+")")
		}
	}
	return nil
}

// DialControl validates the address a connection is ACTUALLY being made to.
// It is net.Dialer.Control's signature, so a transport wired with it cannot
// reach a denied address by any route.
//
// AllowHost checks a NAME before a request goes out. This checks the packet,
// and the gap between the two is where the holes were:
//
//   - A redirect goes somewhere nobody checked. The feed fetcher validated
//     the URL an operator configured and then followed 302s wherever they
//     led, which on a cloud host means the metadata service is one hostile
//     response away.
//   - A name that resolved to a permitted address when it was checked can
//     resolve to a denied one when the dialer looks it up again moments
//     later. Checking the resolved address removes the window rather than
//     narrowing it.
//
// Both are the same mistake: judging the request instead of the connection.
func DialControl(network, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return apperr.ErrInvalidArgument.With("address", address).
			With("reason", "not an address this policy can judge")
	}
	ip := net.ParseIP(host)
	if ip == nil {
		// Control is handed a resolved address. A name here means the
		// resolution did not happen, and a policy that cannot see the
		// address must not assume it is fine.
		return apperr.ErrInvalidArgument.With("address", address).
			With("reason", "unresolved address")
	}
	return allowIP(ip, host)
}
