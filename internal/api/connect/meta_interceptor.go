package apiconnect

import (
	"context"
	"net"
	"strings"

	"connectrpc.com/connect"
	"github.com/gsoultan/anubis/internal/platform/crypto/secret"
	"github.com/gsoultan/anubis/internal/shared/authctx"
)

// MetaInterceptor stamps request id, client IP and user agent into context.
// The IP comes from the PEER address, never a client header — X-Forwarded-For
// is only believable behind a proxy you control, which is a deployment
// decision, not a default.
type MetaInterceptor struct{}

func NewMetaInterceptor() *MetaInterceptor { return &MetaInterceptor{} }

func (i *MetaInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		rid := req.Header().Get("X-Request-Id")
		if rid == "" || len(rid) > 64 {
			if id, err := secret.New(12); err == nil {
				rid = "req_" + id
			}
		}
		ctx = authctx.WithRequestID(ctx, rid)
		if host, _, err := net.SplitHostPort(req.Peer().Addr); err == nil {
			ctx = authctx.WithClientIP(ctx, host)
		}
		if ua := req.Header().Get("User-Agent"); ua != "" {
			ctx = authctx.WithUserAgent(ctx, truncate(ua, 256))
		}
		resp, err := next(ctx, req)
		if err == nil && resp != nil {
			resp.Header().Set("X-Request-Id", rid)
		}
		return resp, err
	}
}

func (i *MetaInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (i *MetaInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

var _ = strings.TrimSpace // reserved
