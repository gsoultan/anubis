package gatehttp

import (
	"encoding/json"
	"net/http"
	"strings"

	gateapp "github.com/gsoultan/anubis/internal/gate/app"
	"github.com/gsoultan/anubis/internal/platform/crypto/keyring"
	"github.com/gsoultan/anubis/pkg/anubis"
)

// GateHandler is /v1/gate/check — forward auth for nginx auth_request,
// Traefik forwardAuth, Envoy ext_authz. Served ENTIRELY from the snapshot:
// p99 < 1 ms, no database on the path. Fail-static while the snapshot is
// within max age; fail-closed beyond it.
//
// The decision itself is gateapp.Decider: Envoy's ext_authz speaks gRPC, and
// two copies of an authorization decision is two places for it to drift.

// Snapshots is all a decision needs from the snapshot manager. It is defined
// in gateapp now, because the gRPC transport needs the same thing and the
// decision itself moved there; this alias keeps the name available where it
// has always been.
type Snapshots = gateapp.Snapshots

type GateHandler struct {
	decider  *gateapp.Decider
	loginURL string
}

func NewGateHandler(issuer string, ring *keyring.Manager, snaps gateapp.Snapshots) *GateHandler {
	return &GateHandler{
		decider:  gateapp.NewDecider(issuer, ring, snaps),
		loginURL: issuer + "/v1/authorize",
	}
}

func (h *GateHandler) Check(w http.ResponseWriter, r *http.Request) {
	tenant := r.Header.Get("X-Anubis-Tenant")
	if tenant == "" {
		tenant = "impack"
	}
	token, ok := anubis.BearerToken(r)
	if !ok {
		// nginx passes the original Authorization header through by default.
		if v := r.Header.Get("X-Original-Authorization"); strings.HasPrefix(v, "Bearer ") {
			token = v[len("Bearer "):]
		}
	}

	d := h.decider.Decide(gateapp.Request{
		Tenant: tenant,
		Method: r.Header.Get("X-Original-Method"),
		Path:   r.Header.Get("X-Original-URI"),
		Host:   r.Header.Get("X-Original-Host"),
		Token:  token,
	})

	switch d.Outcome {
	case gateapp.Allow:
		w.Header().Set("X-Anubis-Subject", d.Subject)
		w.Header().Set("X-Anubis-Session", d.Session)
		if len(d.Scopes) > 0 {
			raw, _ := json.Marshal(d.Scopes)
			w.Header().Set("X-Anubis-Scope", string(raw))
		}
		w.WriteHeader(http.StatusNoContent)
	case gateapp.Unauthenticated:
		w.Header().Set("Location", h.loginURL)
		http.Error(w, d.Reason, http.StatusUnauthorized)
	case gateapp.BadRequest:
		http.Error(w, "missing X-Original-URI/X-Original-Method", http.StatusBadRequest)
	default:
		// Denied and Unavailable are both 403: a caller must not be able to
		// tell "policy says no" from "this instance cannot decide", or the
		// difference becomes a probe for when the gate is degraded.
		http.Error(w, d.Reason, http.StatusForbidden)
	}
}
