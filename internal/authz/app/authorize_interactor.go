package authzapp

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"time"

	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	auditport "github.com/gsoultan/anubis/internal/audit/port"
	authzdomain "github.com/gsoultan/anubis/internal/authz/domain"
	authzport "github.com/gsoultan/anubis/internal/authz/port"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
	"github.com/gsoultan/anubis/internal/shared/clock"
	"github.com/gsoultan/anubis/internal/shared/jsonx"
)

// allowAuditSampleRate: denies are always audited; allows are sampled so the
// hot path is not write-bound while strict-dry-run still gets replay data.
const allowAuditSampleRate = 16

// authorizeInteractor implements AuthorizeUsecase.
type authorizeInteractor struct {
	authz   authzport.AuthzRepository
	clock   clock.Clock
	audit   auditport.Auditor
	counter atomic.Uint64
}

func NewAuthorizeInteractor(authz authzport.AuthzRepository, clock clock.Clock, audit auditport.Auditor) AuthorizeUsecase {
	return &authorizeInteractor{authz: authz, clock: clock, audit: audit}
}

func (u *authorizeInteractor) Execute(ctx context.Context, in AuthorizeInput) (*authzdomain.Decision, error) {
	p, ok := authctx.From(ctx)
	if !ok {
		return nil, apperr.ErrUnauthenticated
	}
	if in.Subject == "" || in.Permission == "" {
		return nil, apperr.ErrInvalidArgument
	}
	// A nil map marshals to the JSON literal null, and jsonb_each_text(null)
	// blows the whole decision up as an internal error. A caller that sent no
	// scopes asked about no targets — that is {}, and it must produce a
	// DECISION (deny, most likely), never a 500 a gateway cannot act on.
	if in.Scopes == nil {
		in.Scopes = map[string]string{}
	}
	targets, err := json.Marshal(in.Scopes)
	if err != nil {
		return nil, apperr.ErrInvalidArgument.Wrap(err)
	}

	allow, err := u.authz.Authorize(ctx, in.Subject, p.TenantID, in.Permission, targets)
	if err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}

	d := &authzdomain.Decision{Allow: allow}
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

func (u *authorizeInteractor) stepUp(ctx context.Context, tenantID string, in AuthorizeInput) *authzdomain.Decision {
	perm, err := u.authz.PermissionByKey(ctx, tenantID, in.Permission)
	if err != nil || perm == nil {
		return nil
	}
	req := authzdomain.PermissionRequirements{RequiresAMR: perm.RequiresAMR, MaxAuthAge: perm.MaxAuthAge}
	var authTime time.Time
	if in.AuthTime > 0 {
		authTime = time.Unix(in.AuthTime, 0)
	}
	step := authzdomain.EvaluateStepUp(req, in.AMR, authTime, u.clock.Now())
	if step == nil {
		return nil
	}
	return &authzdomain.Decision{
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
func (u *authorizeInteractor) explainDenial(ctx context.Context, tenantID string, in AuthorizeInput, targets []byte) *authzdomain.Decision {
	d := &authzdomain.Decision{Allow: false, Reason: "permission_denied", Message: "Denied"}
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

func (u *authorizeInteractor) auditDecision(ctx context.Context, p *authctx.Principal, in AuthorizeInput, d *authzdomain.Decision) {
	result := "deny"
	if d.Allow {
		result = "allow"
		if u.counter.Add(1)%allowAuditSampleRate != 0 {
			return
		}
	}
	u.audit.Emit(ctx, auditdomain.AuditEvent{
		TenantID:  p.TenantID,
		ActorID:   p.IdentityID,
		ActorKind: "service",
		TargetID:  in.Subject,
		Action:    "authorize",
		Result:    result,
		IP:        authctx.ClientIP(ctx),
		Detail: jsonx.Must(map[string]any{
			"subject":    in.Subject,
			"permission": in.Permission,
			"targets":    in.Scopes,
			"reason":     d.Reason,
		}),
	})
}
