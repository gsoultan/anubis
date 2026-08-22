package usecase

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"time"

	"github.com/gsoultan/anubis/internal/authctx"
	"github.com/gsoultan/anubis/internal/domain"
	"github.com/gsoultan/anubis/internal/repository"
)

// allowAuditSampleRate: denies are always audited; allows are sampled so the
// hot path is not write-bound while strict-dry-run still gets replay data.
const allowAuditSampleRate = 16

// authorizeInteractor implements AuthorizeUsecase.
type authorizeInteractor struct {
	authz   repository.AuthzRepository
	clock   repository.Clock
	audit   repository.Auditor
	counter atomic.Uint64
}

func NewAuthorizeInteractor(authz repository.AuthzRepository, clock repository.Clock, audit repository.Auditor) AuthorizeUsecase {
	return &authorizeInteractor{authz: authz, clock: clock, audit: audit}
}

func (u *authorizeInteractor) Execute(ctx context.Context, in AuthorizeInput) (*Decision, error) {
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

	allow, err := u.authz.Authorize(ctx, in.Subject, p.TenantID, in.Permission, targets)
	if err != nil {
		return nil, domain.ErrInternal.Wrap(err)
	}

	d := &Decision{Allow: allow}
	if allow {
		// Step-up gates AFTER the grant check: holding the permission with
		// weak/stale authentication is "yes, but not like this".
		if step := u.stepUp(ctx, p.TenantID, in); step != nil {
			d = step
		}
	} else {
		d = u.explainDenial(ctx, p.TenantID, in, targets)
	}

	u.auditDecision(ctx, p, in, d)
	return d, nil
}

func (u *authorizeInteractor) stepUp(ctx context.Context, tenantID string, in AuthorizeInput) *Decision {
	perm, err := u.authz.PermissionByKey(ctx, tenantID, in.Permission)
	if err != nil || perm == nil {
		return nil
	}
	req := domain.PermissionRequirements{RequiresAMR: perm.RequiresAMR, MaxAuthAge: perm.MaxAuthAge}
	var authTime time.Time
	if in.AuthTime > 0 {
		authTime = time.Unix(in.AuthTime, 0)
	}
	step := domain.EvaluateStepUp(req, in.AMR, authTime, u.clock.Now())
	if step == nil {
		return nil
	}
	return &Decision{
		Allow:       false,
		Reason:      "step_up_required",
		Message:     "Permission requires stronger or fresher authentication",
		RequiredAMR: step.RequiredAMR,
		MaxAuthAge:  step.MaxAuthAge.String(),
		CurrentAMR:  step.CurrentAMR,
		AuthAge:     step.AuthAge.String(),
	}
}

// explainDenial runs the decomposition ONLY on the deny path — allows stay a
// single engine call.
func (u *authorizeInteractor) explainDenial(ctx context.Context, tenantID string, in AuthorizeInput, targets []byte) *Decision {
	d := &Decision{Allow: false, Reason: "permission_denied", Message: "Denied"}
	detail, err := u.authz.AuthorizeExplain(ctx, in.Subject, tenantID, in.Permission, targets)
	if err != nil {
		return d
	}
	var parsed struct {
		Reason      *string `json:"reason"`
		FailingAxis *string `json:"failing_axis"`
	}
	if json.Unmarshal([]byte(detail), &parsed) == nil {
		if parsed.Reason != nil {
			d.Reason = *parsed.Reason
		}
		if parsed.FailingAxis != nil {
			d.FailingAxis = *parsed.FailingAxis
			d.Message = "no grant at or above the requested " + *parsed.FailingAxis + " node"
		}
	}
	return d
}

func (u *authorizeInteractor) auditDecision(ctx context.Context, p *authctx.Principal, in AuthorizeInput, d *Decision) {
	result := "deny"
	if d.Allow {
		result = "allow"
		if u.counter.Add(1)%allowAuditSampleRate != 0 {
			return
		}
	}
	u.audit.Emit(ctx, repository.AuditEvent{
		TenantID:  p.TenantID,
		ActorID:   p.IdentityID,
		ActorKind: "service",
		TargetID:  in.Subject,
		Action:    "authorize",
		Result:    result,
		IP:        authctx.ClientIP(ctx),
		Detail: mustJSON(map[string]any{
			"subject":    in.Subject,
			"permission": in.Permission,
			"targets":    in.Scopes,
			"reason":     d.Reason,
		}),
	})
}
