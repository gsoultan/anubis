package authzapp

import (
	"context"
	"encoding/json"

	authzdomain "github.com/gsoultan/anubis/internal/authz/domain"
	authzport "github.com/gsoultan/anubis/internal/authz/port"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
)

// explainInteractor implements ExplainUsecase.
type explainInteractor struct {
	authz authzport.AuthzRepository
}

func NewExplainInteractor(authz authzport.AuthzRepository) ExplainUsecase {
	return &explainInteractor{authz: authz}
}

func (u *explainInteractor) Execute(ctx context.Context, in AuthorizeInput) (*authzdomain.Explanation, error) {
	p, ok := authctx.From(ctx)
	if !ok {
		return nil, apperr.ErrUnauthenticated
	}
	if in.Subject == "" || in.Permission == "" {
		return nil, apperr.ErrInvalidArgument
	}
	targets, err := json.Marshal(in.Scopes)
	if err != nil {
		return nil, apperr.ErrInvalidArgument.Wrap(err)
	}
	detail, err := u.authz.AuthorizeExplain(ctx, in.Subject, p.TenantID, in.Permission, targets)
	if err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}
	out := &authzdomain.Explanation{DetailJSON: detail}
	var parsed struct {
		Allow       bool    `json:"allow"`
		Reason      *string `json:"reason"`
		FailingAxis *string `json:"failing_axis"`
	}
	if err := json.Unmarshal([]byte(detail), &parsed); err == nil {
		out.Allow = parsed.Allow
		if parsed.Reason != nil {
			out.Reason = *parsed.Reason
		}
		if parsed.FailingAxis != nil {
			out.FailingAxis = *parsed.FailingAxis
		}
	}
	return out, nil
}
