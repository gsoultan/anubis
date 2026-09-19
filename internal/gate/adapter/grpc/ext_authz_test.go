package gategrpc

import (
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"

	authv3 "github.com/gsoultan/anubis/gen/go/envoy/service/auth/v3"
	gateapp "github.com/gsoultan/anubis/internal/gate/app"
)

// THE test this vendored proto needs.
//
// proto/envoy/service/auth/v3/external_auth.proto reproduces a foreign
// contract by FIELD NUMBER. A wrong number does not fail loudly: it reads a
// different string, or drops a header, and the gate authorizes the wrong
// request or the upstream never learns who the caller is. So the numbers are
// asserted here against the values taken from envoyproxy/envoy, rather than
// left to a comment nobody re-checks.
//
// Upstream sources, all on main at the time of writing:
//
//	api/envoy/service/auth/v3/external_auth.proto
//	api/envoy/service/auth/v3/attribute_context.proto
//	api/envoy/config/core/v3/base.proto
//	api/envoy/type/v3/http_status.proto
func TestWireNumbersMatchEnvoy(t *testing.T) {
	num := func(m protoreflect.ProtoMessage, field string) int32 {
		t.Helper()
		fd := m.ProtoReflect().Descriptor().Fields().ByName(protoreflect.Name(field))
		if fd == nil {
			t.Fatalf("%s has no field %q", m.ProtoReflect().Descriptor().FullName(), field)
		}
		return int32(fd.Number())
	}

	for _, c := range []struct {
		msg   protoreflect.ProtoMessage
		field string
		want  int32
	}{
		{&authv3.CheckRequest{}, "attributes", 1},

		{&authv3.AttributeContext{}, "source", 1},
		{&authv3.AttributeContext{}, "destination", 2},
		{&authv3.AttributeContext{}, "request", 4}, // NOT 3
		{&authv3.AttributeContext{}, "context_extensions", 10},

		{&authv3.AttributeContext_Request{}, "http", 2}, // 1 is `time`

		{&authv3.AttributeContext_HttpRequest{}, "id", 1},
		{&authv3.AttributeContext_HttpRequest{}, "method", 2},
		{&authv3.AttributeContext_HttpRequest{}, "headers", 3},
		{&authv3.AttributeContext_HttpRequest{}, "path", 4},
		{&authv3.AttributeContext_HttpRequest{}, "host", 5},
		{&authv3.AttributeContext_HttpRequest{}, "scheme", 6},
		{&authv3.AttributeContext_HttpRequest{}, "query", 7},
		{&authv3.AttributeContext_HttpRequest{}, "fragment", 8},
		{&authv3.AttributeContext_HttpRequest{}, "size", 9},
		{&authv3.AttributeContext_HttpRequest{}, "protocol", 10},
		{&authv3.AttributeContext_HttpRequest{}, "body", 11},
		{&authv3.AttributeContext_HttpRequest{}, "raw_body", 12},

		{&authv3.CheckResponse{}, "status", 1},
		{&authv3.CheckResponse{}, "denied_response", 2},
		{&authv3.CheckResponse{}, "ok_response", 3},

		{&authv3.DeniedHttpResponse{}, "status", 1},
		{&authv3.DeniedHttpResponse{}, "headers", 2},
		{&authv3.DeniedHttpResponse{}, "body", 3},

		// The one most easily got wrong: OkHttpResponse.headers is 2, not 1.
		// At 1 every header the gate adds is silently dropped.
		{&authv3.OkHttpResponse{}, "headers", 2},
		{&authv3.OkHttpResponse{}, "headers_to_remove", 5},

		{&authv3.HeaderValueOption{}, "header", 1},
		{&authv3.HeaderValue{}, "key", 1},
		{&authv3.HeaderValue{}, "value", 2},
		{&authv3.HttpStatus{}, "code", 1},
		{&authv3.Status{}, "code", 1},
		{&authv3.Status{}, "message", 2},
	} {
		if got := num(c.msg, c.field); got != c.want {
			t.Errorf("%s.%s = %d, want %d",
				c.msg.ProtoReflect().Descriptor().FullName(), c.field, got, c.want)
		}
	}
}

// Envoy dials a literal path. Rename the service or the method and every
// request 404s, which presents as the mesh being misconfigured.
func TestServicePathIsWhatEnvoyDials(t *testing.T) {
	const want = "envoy.service.auth.v3.Authorization"
	got := string((&authv3.CheckRequest{}).ProtoReflect().Descriptor().ParentFile().
		Services().Get(0).FullName())
	if got != want {
		t.Fatalf("service is %q, want %q", got, want)
	}
	if m := (&authv3.CheckRequest{}).ProtoReflect().Descriptor().ParentFile().
		Services().Get(0).Methods().Get(0).Name(); m != "Check" {
		t.Fatalf("method is %q, want Check", m)
	}
}

// An allow must strip client-supplied identity headers. Without it a caller
// sets X-Anubis-Subject itself and the upstream trusts it as the gate's word.
func TestAllowStripsClientSuppliedIdentityHeaders(t *testing.T) {
	e := New(nil, "https://issuer/v1/authorize")
	resp := e.response(gateapp.Decision{
		Outcome: gateapp.Allow, Subject: "alice", Session: "s1",
		Scopes: map[string]string{"org": "n1"},
	})
	ok := resp.GetOkResponse()
	if ok == nil {
		t.Fatal("allow did not produce an ok_response")
	}
	if resp.GetStatus().GetCode() != codeOK {
		t.Fatalf("status code %d, want 0", resp.GetStatus().GetCode())
	}
	want := map[string]bool{
		"x-anubis-subject": true, "x-anubis-session": true, "x-anubis-scope": true,
	}
	for _, h := range ok.GetHeadersToRemove() {
		delete(want, h)
	}
	if len(want) != 0 {
		t.Fatalf("these were not stripped from the client: %v", want)
	}
	var subject string
	for _, h := range ok.GetHeaders() {
		if h.GetHeader().GetKey() == "X-Anubis-Subject" {
			subject = h.GetHeader().GetValue()
		}
	}
	if subject != "alice" {
		t.Fatalf("subject header is %q", subject)
	}
}

// Every non-allow must be an explicit deny with an OK-free status code.
// Returning a transport error instead would make Envoy apply
// failure_mode_allow, turning an internal fault into an open door.
func TestEveryDenialIsExplicit(t *testing.T) {
	e := New(nil, "https://issuer/v1/authorize")
	for _, c := range []struct {
		outcome    gateapp.Outcome
		wantStatus int32
		wantCode   int32
	}{
		{gateapp.Denied, 403, codePermissionDenied},
		{gateapp.Unauthenticated, 401, codeUnauthenticated},
		{gateapp.Unavailable, 403, codePermissionDenied},
		{gateapp.BadRequest, 400, codePermissionDenied},
	} {
		resp := e.response(gateapp.Decision{Outcome: c.outcome, Reason: "because"})
		if resp.GetOkResponse() != nil {
			t.Fatalf("outcome %v produced an ok_response", c.outcome)
		}
		d := resp.GetDeniedResponse()
		if d == nil {
			t.Fatalf("outcome %v produced no denied_response", c.outcome)
		}
		if d.GetStatus().GetCode() != c.wantStatus {
			t.Errorf("outcome %v: http status %d, want %d", c.outcome, d.GetStatus().GetCode(), c.wantStatus)
		}
		if resp.GetStatus().GetCode() != c.wantCode {
			t.Errorf("outcome %v: rpc code %d, want %d", c.outcome, resp.GetStatus().GetCode(), c.wantCode)
		}
	}
}
