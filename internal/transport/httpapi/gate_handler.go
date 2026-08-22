package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gsoultan/anubis/internal/crypto/keyring"
	"github.com/gsoultan/anubis/internal/gate"
	"github.com/gsoultan/anubis/internal/snapshot"
	"github.com/gsoultan/anubis/pkg/anubis"
	"github.com/gsoultan/anubis/pkg/anubis/paseto"
)

// GateHandler is /v1/gate/check — forward auth for nginx auth_request,
// Traefik forwardAuth, Envoy ext_authz. Served ENTIRELY from the snapshot:
// p99 < 1 ms, no database on the path. Fail-static while the snapshot is
// within max age; fail-closed beyond it.
type GateHandler struct {
	issuer   string
	ring     *keyring.Manager
	snaps    *snapshot.Manager
	loginURL string
	clock    func() time.Time
}

func NewGateHandler(issuer string, ring *keyring.Manager, snaps *snapshot.Manager) *GateHandler {
	return &GateHandler{
		issuer: issuer, ring: ring, snaps: snaps,
		loginURL: issuer + "/v1/authorize",
		clock:    time.Now,
	}
}

func (h *GateHandler) Check(w http.ResponseWriter, r *http.Request) {
	uri := r.Header.Get("X-Original-URI")
	method := r.Header.Get("X-Original-Method")
	host := r.Header.Get("X-Original-Host")
	tenant := r.Header.Get("X-Anubis-Tenant")
	if tenant == "" {
		tenant = "impack"
	}
	if uri == "" || method == "" {
		http.Error(w, "missing X-Original-URI/X-Original-Method", http.StatusBadRequest)
		return
	}

	snap, fresh := h.snaps.Get(tenant)
	if snap == nil || !fresh {
		// No snapshot, or stale beyond max age: FAIL CLOSED. A cached answer
		// beats an outage; an unbounded-stale answer does not.
		http.Error(w, "authorization snapshot unavailable", http.StatusForbidden)
		return
	}

	normPath, err := gate.NormalizePath(uri)
	if err != nil {
		// Ambiguous path = deny. The gap between two normalisers is the
		// bypass; anything this one cannot canonicalise nothing may serve.
		http.Error(w, "ambiguous path", http.StatusForbidden)
		return
	}

	route, params := gate.Match(snap.Routes, host, method, normPath)
	if route == nil {
		// No policy for this path: deny. An unlisted path behind the gate is
		// a configuration hole, not an allow.
		http.Error(w, "no route policy", http.StatusForbidden)
		return
	}

	switch route.Effect {
	case "public":
		w.WriteHeader(http.StatusNoContent)
		return
	case "deny":
		http.Error(w, "denied by policy", http.StatusForbidden)
		return
	}

	claims := h.verify(r, snap)
	if claims == nil {
		w.Header().Set("Location", h.loginURL)
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
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
		if !snap.Evaluate(claims.Subject, route.PermissionKey, targets, h.clock()) {
			http.Error(w, "permission denied", http.StatusForbidden)
			return
		}
	}

	w.Header().Set("X-Anubis-Subject", claims.Subject)
	w.Header().Set("X-Anubis-Session", claims.Session)
	if len(claims.Scopes) > 0 {
		raw, _ := json.Marshal(claims.Scopes)
		w.Header().Set("X-Anubis-Scope", string(raw))
	}
	w.WriteHeader(http.StatusNoContent)
}

// verify checks the bearer token offline against the ring and the snapshot's
// revocation/epoch state. Zero I/O.
func (h *GateHandler) verify(r *http.Request, snap *snapshot.Data) *anubis.Claims {
	token, ok := anubis.BearerToken(r)
	if !ok {
		// nginx passes the original Authorization header through by default.
		if v := r.Header.Get("X-Original-Authorization"); strings.HasPrefix(v, "Bearer ") {
			token = v[len("Bearer "):]
		} else {
			return nil
		}
	}
	_, _, footer, err := paseto.Parse(token)
	if err != nil {
		return nil
	}
	var tf struct {
		Kid string `json:"kid"`
	}
	if len(footer) > 0 && json.Unmarshal(footer, &tf) != nil {
		return nil
	}
	key, err := h.ring.Ring().Lookup(tf.Kid)
	if err != nil || key.Purpose != keyring.PurposeAccess {
		return nil
	}
	msg, _, err := paseto.Verify(key.Public, token, nil)
	if err != nil {
		return nil
	}
	var claims anubis.Claims
	if json.Unmarshal(msg, &claims) != nil {
		return nil
	}
	now := h.clock().Unix()
	if claims.Issuer != h.issuer || claims.Tenant != snap.TenantSlug ||
		(claims.Expires != 0 && now > claims.Expires) ||
		(claims.NotBefore != 0 && now < claims.NotBefore-60) {
		return nil
	}
	if !snap.SessionAlive(claims.Session, claims.Epoch, claims.Subject) {
		return nil
	}
	return &claims
}
