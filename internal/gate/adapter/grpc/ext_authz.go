// Package gategrpc serves Envoy's external authorization contract.
//
// Envoy's ext_authz filter can call either an HTTP endpoint or a gRPC one.
// The HTTP form has always worked through /v1/gate/check; this is the gRPC
// form, which is what most Envoy deployments configure because it carries the
// request attributes as a typed message instead of as invented headers.
//
// The DECISION is gateapp.Decider, shared with the HTTP transport. Two copies
// of an authorization decision is two places for it to drift, and a gate that
// allows over one protocol what it denies over another is the worst kind of
// bug to find.
package gategrpc

import (
	"context"
	"encoding/json"
	"strings"

	"connectrpc.com/connect"

	authv3 "github.com/gsoultan/anubis/gen/go/envoy/service/auth/v3"
	gateapp "github.com/gsoultan/anubis/internal/gate/app"
)

// google.rpc.Code values Envoy reads. Only these three are ever sent: Envoy
// treats OK as allow and anything else as deny, and the distinction between
// the two denials is for the operator reading logs.
const (
	codeOK               = 0
	codePermissionDenied = 7
	codeUnauthenticated  = 16
)

// defaultTenant matches the HTTP transport's fallback. A proxy that does not
// set the tenant gets the bootstrap one rather than a failure, because the
// single-tenant deployment is the common one and making it configure a
// header it does not need is friction for nothing.
const defaultTenant = "impack"

// ExtAuthz implements envoy.service.auth.v3.Authorization.
type ExtAuthz struct {
	decider  *gateapp.Decider
	loginURL string
}

func New(decider *gateapp.Decider, loginURL string) *ExtAuthz {
	return &ExtAuthz{decider: decider, loginURL: loginURL}
}

// Check answers one ext_authz query.
//
// It never returns a transport error. A gRPC error makes Envoy apply
// failure_mode_allow, which on a misconfigured mesh means an internal fault
// here opens the door; an explicit PERMISSION_DENIED cannot be read that way.
// The only thing this method can do wrong is fail open, so it does not have a
// path that fails at all.
func (e *ExtAuthz) Check(
	ctx context.Context,
	req *connect.Request[authv3.CheckRequest],
) (*connect.Response[authv3.CheckResponse], error) {
	http := req.Msg.GetAttributes().GetRequest().GetHttp()

	// Headers arrive lower-cased from Envoy, but the map is attacker-
	// influenced and a proxy in front may not normalise, so read it case-
	// insensitively rather than trusting the convention.
	headers := http.GetHeaders()
	get := func(name string) string {
		if v, ok := headers[name]; ok {
			return v
		}
		for k, v := range headers {
			if strings.EqualFold(k, name) {
				return v
			}
		}
		return ""
	}

	tenant := get("x-anubis-tenant")
	if tenant == "" {
		tenant = defaultTenant
	}
	var token string
	if v := get("authorization"); len(v) > 7 && strings.EqualFold(v[:7], "bearer ") {
		token = v[7:]
	}
	host := http.GetHost()
	if host == "" {
		host = get("host")
	}

	d := e.decider.Decide(gateapp.Request{
		Tenant: tenant,
		Method: http.GetMethod(),
		Path:   http.GetPath(),
		Host:   host,
		Token:  token,
	})

	return connect.NewResponse(e.response(d)), nil
}

func (e *ExtAuthz) response(d gateapp.Decision) *authv3.CheckResponse {
	if d.Outcome == gateapp.Allow {
		headers := []*authv3.HeaderValueOption{
			{Header: &authv3.HeaderValue{Key: "X-Anubis-Subject", Value: d.Subject}},
			{Header: &authv3.HeaderValue{Key: "X-Anubis-Session", Value: d.Session}},
		}
		if len(d.Scopes) > 0 {
			raw, _ := json.Marshal(d.Scopes)
			headers = append(headers, &authv3.HeaderValueOption{
				Header: &authv3.HeaderValue{Key: "X-Anubis-Scope", Value: string(raw)},
			})
		}
		return &authv3.CheckResponse{
			Status: &authv3.Status{Code: codeOK},
			HttpResponse: &authv3.CheckResponse_OkResponse{
				OkResponse: &authv3.OkHttpResponse{
					Headers: headers,
					// Strip anything the CLIENT sent under these names. The
					// upstream service trusts them as Anubis's word; letting
					// a caller supply their own would be identity spoofing
					// through a header the gate is supposed to own.
					HeadersToRemove: []string{
						"x-anubis-subject", "x-anubis-session", "x-anubis-scope",
					},
				},
			},
		}
	}

	status, code := int32(403), int32(codePermissionDenied)
	var extra []*authv3.HeaderValueOption
	if d.Outcome == gateapp.Unauthenticated {
		status, code = 401, codeUnauthenticated
		extra = append(extra, &authv3.HeaderValueOption{
			Header: &authv3.HeaderValue{Key: "Location", Value: e.loginURL},
		})
	}
	if d.Outcome == gateapp.BadRequest {
		status = 400
	}
	return &authv3.CheckResponse{
		Status: &authv3.Status{Code: code, Message: d.Reason},
		HttpResponse: &authv3.CheckResponse_DeniedResponse{
			DeniedResponse: &authv3.DeniedHttpResponse{
				Status:  &authv3.HttpStatus{Code: status},
				Headers: extra,
				Body:    d.Reason,
			},
		},
	}
}
