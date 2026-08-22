package endpoint

import (
	"context"
	"expvar"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-kit/kit/endpoint"

	"github.com/gsoultan/anubis/internal/authctx"
	"github.com/gsoultan/anubis/internal/crypto/secret"
	"github.com/gsoultan/anubis/internal/domain"
)

// Chain is the standard middleware order: recover outermost (a panic in
// logging must still be caught), then request-id, then logging, then
// metrics. Rate limiting attaches per-endpoint where it applies.
func Chain(name string, logger *slog.Logger) endpoint.Middleware {
	return endpoint.Chain(
		Recover(name, logger),
		RequestID(),
		Logging(name, logger),
		Metrics(name),
	)
}

// Recover converts panics into internal errors instead of dead connections.
func Recover(name string, logger *slog.Logger) endpoint.Middleware {
	return func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, req any) (resp any, err error) {
			defer func() {
				if r := recover(); r != nil {
					logger.Error("panic recovered", "endpoint", name, "panic", fmt.Sprint(r))
					resp, err = nil, domain.ErrInternal
				}
			}()
			return next(ctx, req)
		}
	}
}

// RequestID assigns the id that stitches responses, logs and the audit chain
// together. Transports may already have set one from the wire.
func RequestID() endpoint.Middleware {
	return func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, req any) (any, error) {
			if authctx.RequestID(ctx) == "" {
				if id, err := secret.New(12); err == nil {
					ctx = authctx.WithRequestID(ctx, "req_"+id)
				}
			}
			return next(ctx, req)
		}
	}
}

// Logging emits one structured line per call. Codes, not messages: the
// message already went to the caller.
func Logging(name string, logger *slog.Logger) endpoint.Middleware {
	return func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, req any) (any, error) {
			start := time.Now()
			resp, err := next(ctx, req)
			attrs := []any{
				"endpoint", name,
				"duration_ms", time.Since(start).Milliseconds(),
				"request_id", authctx.RequestID(ctx),
			}
			if err != nil {
				attrs = append(attrs, "error_code", domain.AsError(err).Code)
				logger.Warn("endpoint", attrs...)
			} else {
				logger.Info("endpoint", attrs...)
			}
			return resp, err
		}
	}
}

// Metrics publishes per-endpoint counters via expvar (served on the debug
// listener). Stdlib-only by design.
var (
	mCalls    = expvar.NewMap("anubis_endpoint_calls")
	mErrors   = expvar.NewMap("anubis_endpoint_errors")
	mDuration = expvar.NewMap("anubis_endpoint_duration_ms")
)

func Metrics(name string) endpoint.Middleware {
	return func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, req any) (any, error) {
			start := time.Now()
			resp, err := next(ctx, req)
			mCalls.Add(name, 1)
			mDuration.Add(name, time.Since(start).Milliseconds())
			if err != nil {
				mErrors.Add(name, 1)
			}
			return resp, err
		}
	}
}
