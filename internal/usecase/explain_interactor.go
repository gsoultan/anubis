package usecase

import (
	"context"
	"encoding/json"

	"github.com/gsoultan/anubis/internal/authctx"
	"github.com/gsoultan/anubis/internal/domain"
	"github.com/gsoultan/anubis/internal/repository"
)

// explainInteractor implements ExplainUsecase.
type explainInteractor struct {
	authz repository.AuthzRepository
}

func NewExplainInteractor(authz repository.AuthzRepository) ExplainUsecase {
	return &explainInteractor{authz: authz}
}

func (u *explainInteractor) Execute(ctx context.Context, in AuthorizeInput) (*Explanation, error) {
	p, ok := authctx.From(ctx)
	if !ok {
		return nil, domain.ErrUnauthenticated
	}
	if in.Subject == "" || in.Permission == "" {
		return nil, domain.ErrInvalidArgument
	}
	targets, err := json.Marshal(in.Scopes)
	if err != nil {
		return nil, domain.ErrInvalidArgument.Wrap(err)
	}
	detail, err := u.authz.AuthorizeExplain(ctx, in.Subject, p.TenantID, in.Permission, targets)
	if err != nil {
		return nil, domain.ErrInternal.Wrap(err)
	}
	out := &Explanation{DetailJSON: detail}
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
