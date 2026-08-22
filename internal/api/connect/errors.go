package apiconnect

import (
	"context"

	"connectrpc.com/connect"

	anubisv1 "github.com/gsoultan/anubis/gen/go/anubis/v1"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
)

// Err renders a domain error as a connect error carrying the SAME
// stable machine code the HTTP envelope uses, plus the request id, as an
// anubis.v1.ErrorInfo detail. One vocabulary, two transports.
func Err(ctx context.Context, err error) error {
	de := apperr.AsError(err)
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

func codeFor(k apperr.Kind) connect.Code {
	switch k {
	case apperr.KindInvalidArgument:
		return connect.CodeInvalidArgument
	case apperr.KindUnauthenticated:
		return connect.CodeUnauthenticated
	case apperr.KindPermissionDenied:
		return connect.CodePermissionDenied
	case apperr.KindNotFound:
		return connect.CodeNotFound
	case apperr.KindConflict:
		return connect.CodeAlreadyExists
	case apperr.KindFailedPrecondition:
		return connect.CodeFailedPrecondition
	case apperr.KindRateLimited:
		return connect.CodeResourceExhausted
	case apperr.KindUnavailable:
		return connect.CodeUnavailable
	default:
		return connect.CodeInternal
	}
}
