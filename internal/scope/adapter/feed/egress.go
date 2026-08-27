package feed

import (
	"net"
	"os"
	"strings"
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
const externalTimeout = 60 * time.Second

// alwaysDenied is not configurable. 169.254.0.0/16 carries the metadata
// service on every major cloud, and fe80::/10 is its IPv6 equivalent;
// reaching either from a feed is not a use case anyone has.
var alwaysDenied = []string{"169.254.0.0/16", "fe80::/10"}

func allowExternalHost(host string) error {
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
