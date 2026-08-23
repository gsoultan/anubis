package identityapp

import (
	"context"

	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	auditport "github.com/gsoultan/anubis/internal/audit/port"
	identityport "github.com/gsoultan/anubis/internal/identity/port"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/jsonx"
	"github.com/gsoultan/anubis/internal/shared/txm"
)

// retentionInteractor implements RetentionUsecase.
type retentionInteractor struct {
	retention identityport.RetentionRepository
	pii       identityport.PIIRepository
	tx        txm.TxManager
	audit     auditport.Auditor
}

func NewRetentionInteractor(
	retention identityport.RetentionRepository,
	pii identityport.PIIRepository,
	tx txm.TxManager,
	audit auditport.Auditor,
) RetentionUsecase {
	return &retentionInteractor{retention: retention, pii: pii, tx: tx, audit: audit}
}

func (u *retentionInteractor) Sweep(ctx context.Context) (SweepReport, error) {
	var rep SweepReport
	err := u.tx.WithinTx(ctx, func(ctx context.Context) error {
		stamped, err := u.retention.ApplyRealmRetention(ctx)
		if err != nil {
			return err
		}
		rep.Stamped = stamped

		ids, tenants, keys, err := u.retention.ExpireRetained(ctx)
		if err != nil {
			return err
		}
		rep.Anonymized = len(ids)

		// Shred inside the same transaction: an identity marked anonymised
		// whose key survived would be a lie told to a regulator.
		for _, k := range keys {
			shredded, serr := u.pii.ShredPIIKey(ctx, k, "retention")
			if serr != nil {
				return serr
			}
			if shredded {
				rep.Shredded++
			}
		}
		for i, id := range ids {
			u.audit.Emit(ctx, auditdomain.AuditEvent{
				TenantID: tenants[i], ActorKind: "system", TargetID: id,
				Action: "identity.retention_anonymized", Result: "allow",
				Detail: jsonx.Must(map[string]string{"reason": "retention"}),
			})
		}
		return nil
	})
	return rep, err
}

func (u *retentionInteractor) Erase(ctx context.Context, tenantID, identityID, reason string) error {
	if tenantID == "" || identityID == "" {
		return apperr.ErrInvalidArgument
	}
	if reason == "" {
		reason = "erasure_request"
	}
	return u.tx.WithinTx(ctx, func(ctx context.Context) error {
		keyID, err := u.retention.Anonymize(ctx, tenantID, identityID)
		if err != nil {
			return err
		}
		if keyID != "" {
			if _, err := u.pii.ShredPIIKey(ctx, keyID, reason); err != nil {
				return err
			}
		}
		u.audit.Emit(ctx, auditdomain.AuditEvent{
			TenantID: tenantID, ActorKind: "system", TargetID: identityID,
			Action: "identity.erased", Result: "allow",
			Detail: jsonx.Must(map[string]string{"reason": reason}),
		})
		return nil
	})
}
