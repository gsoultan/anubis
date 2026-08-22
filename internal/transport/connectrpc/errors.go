package connectrpc

import (
	"context"

	"connectrpc.com/connect"

	anubisv1 "github.com/gsoultan/anubis/gen/go/anubis/v1"
	"github.com/gsoultan/anubis/internal/authctx"
	"github.com/gsoultan/anubis/internal/domain"
)

// toConnectErr renders a domain error as a connect error carrying the SAME
// stable machine code the HTTP envelope uses, plus the request id, as an
// anubis.v1.ErrorInfo detail. One vocabulary, two transports.
func toConnectErr(ctx context.Context, err error) error {
	de := domain.AsError(err)
	cerr := connect.NewError(codeFor(de.Kind), de)
	info := &anubisv1.ErrorInfo{
		Code:      de.Code,
		RequestId: authctx.RequestID(ctx),
		Details:   de.Details,
	}
	if detail, derr := connect.NewErrorDetail(info); derr == nil {
		cerr.AddDetail(detail)
	}
	return cerr
}

func codeFor(k domain.Kind) connect.Code {
	switch k {
	case domain.KindInvalidArgument:
		return connect.CodeInvalidArgument
	case domain.KindUnauthenticated:
		return connect.CodeUnauthenticated
	case domain.KindPermissionDenied:
		return connect.CodePermissionDenied
	case domain.KindNotFound:
		return connect.CodeNotFound
	case domain.KindConflict:
		return connect.CodeAlreadyExists
	case domain.KindFailedPrecondition:
		return connect.CodeFailedPrecondition
	case domain.KindRateLimited:
		return connect.CodeResourceExhausted
	case domain.KindUnavailable:
		return connect.CodeUnavailable
	default:
		return connect.CodeInternal
	}
}
