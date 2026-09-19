package gateapp

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/gsoultan/anubis/internal/gate/routepath"
	"github.com/gsoultan/anubis/internal/gate/snapshot"
	"github.com/gsoultan/anubis/internal/platform/crypto/accesstoken"
	"github.com/gsoultan/anubis/internal/platform/crypto/keyring"
	"github.com/gsoultan/anubis/pkg/anubis"
)

// Snapshots is all a decision needs from the snapshot manager: the tenant's
// data, and whether it is fresh enough to decide from.
type Snapshots interface {
	Get(tenantSlug string) (*snapshot.Data, bool)
}

// Request is one forward-auth question, in whatever shape the proxy asked it.
type Request struct {
	Tenant string
	Method string
	Path   string
	Host   string
	Token  string // bearer token, already stripped of its scheme
}

// Outcome is what the gate decided.
type Outcome int

const (
	// Allow: the request may proceed.
	Allow Outcome = iota
	// Unauthenticated: no usable token. The caller should be sent to log in.
	Unauthenticated
	// Denied: authenticated (or not required) but not permitted.
	Denied
	// Unavailable: no snapshot, or one too stale to decide from. FAIL
	// CLOSED — a cached answer beats an outage, an unbounded-stale one
	// does not.
	Unavailable
	// BadRequest: the proxy did not supply enough to decide.
	BadRequest
)

// Decision is the answer plus what a proxy should forward upstream on allow.
type Decision struct {
	Outcome Outcome
	// Reason is for operators, not for the caller: it names which rule
	// produced the answer, which is the difference between debugging a
	// policy and guessing at one.
	Reason  string
	Subject string
	Session string
	Scopes  map[string]string
}

// Decider answers forward-auth questions from the snapshot alone. Zero I/O,
// which is what lets the gate hold a p99 under a millisecond.
//
// It lives in the app layer, not in a transport, because there are two
// transports — nginx/Traefik over HTTP and Envoy over gRPC ext_authz — and a
// second copy of this logic is a second place for it to drift. A gate that
// allows over one protocol what it denies over another is the worst kind of
// bug to find.
type Decider struct {
	issuer string
	ring   *keyring.Manager
	snaps  Snapshots
	clock  func() time.Time
}

func NewDecider(issuer string, ring *keyring.Manager, snaps Snapshots) *Decider {
	return &Decider{issuer: issuer, ring: ring, snaps: snaps, clock: time.Now}
}

// Decide answers one request.
func (d *Decider) Decide(req Request) Decision {
	if req.Path == "" || req.Method == "" {
		return Decision{Outcome: BadRequest, Reason: "missing method or path"}
	}
	snap, fresh := d.snaps.Get(req.Tenant)
	if snap == nil || !fresh {
		return Decision{Outcome: Unavailable, Reason: "authorization snapshot unavailable"}
	}

	normPath, err := routepath.NormalizePath(req.Path)
	if err != nil {
		// Ambiguous path = deny. The gap between two normalisers is the
		// bypass; anything this one cannot canonicalise nothing may serve.
		return Decision{Outcome: Denied, Reason: "ambiguous path"}
	}

	route, params := routepath.Match(snap.Routes, req.Host, req.Method, normPath)
	if route == nil {
		// An unlisted path behind the gate is a configuration hole, not an
		// allow.
		return Decision{Outcome: Denied, Reason: "no route policy"}
	}

	switch route.Effect {
	case "public":
		return Decision{Outcome: Allow, Reason: "public route"}
	case "deny":
		return Decision{Outcome: Denied, Reason: "denied by policy"}
	}

	claims := d.verify(req.Token, snap)
	if claims == nil {
		return Decision{Outcome: Unauthenticated, Reason: "authentication required"}
	}

	if route.Effect == "require_permission" {
		targets := map[string]string{}
		for axis, from := range route.ScopeBindings {
			switch {
			case from == "token":
				if v, ok := claims.Scopes[axis]; ok {
					targets[axis] = v
				}
			case strings.HasPrefix(from, "path."):
				if v, ok := params[from[len("path."):]]; ok {
					targets[axis] = v
				}
			}
		}
		if !snap.Evaluate(claims.Subject, route.PermissionKey, targets, d.clock()) {
			return Decision{Outcome: Denied, Reason: "permission denied"}
		}
	}

	return Decision{
		Outcome: Allow, Reason: "allowed",
		Subject: claims.Subject, Session: claims.Session, Scopes: claims.Scopes,
	}
}

// verify checks a bearer token offline against the ring and the snapshot's
// revocation/epoch state. Zero I/O.
func (d *Decider) verify(token string, snap *snapshot.Data) *anubis.Claims {
	if token == "" {
		return nil
	}
	kid, err := accesstoken.Kid(token)
	if err != nil {
		return nil
	}
	key, err := d.ring.Ring().Lookup(kid)
	if err != nil || key.Purpose != keyring.PurposeAccess {
		return nil
	}
	msg, err := accesstoken.Verify(key.Public, token)
	if err != nil {
		return nil
	}
	var claims anubis.Claims
	if json.Unmarshal(msg, &claims) != nil {
		return nil
	}
	now := d.clock().Unix()
	if claims.Issuer != d.issuer || claims.Tenant != snap.TenantSlug ||
		(claims.Expires != 0 && now > claims.Expires) ||
		(claims.NotBefore != 0 && now < claims.NotBefore-60) {
		return nil
	}
	if !snap.SessionAlive(claims.Session, claims.Epoch, claims.Subject) {
		return nil
	}
	return &claims
}
