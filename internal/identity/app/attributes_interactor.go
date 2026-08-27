package identityapp

import (
	"context"
	"errors"
	"time"

	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	auditport "github.com/gsoultan/anubis/internal/audit/port"
	"github.com/gsoultan/anubis/internal/authz/guard"
	"github.com/gsoultan/anubis/internal/identity/domain/pii"
	identityport "github.com/gsoultan/anubis/internal/identity/port"
	"github.com/gsoultan/anubis/internal/platform/crypto/keyring"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/jsonx"
	"github.com/gsoultan/anubis/internal/shared/txm"
)

// maxAttributes bounds what one identity may carry. The column is sealed as a
// single blob, so an unbounded map is an unbounded decryption on every read —
// and a place to hide a payload that nothing else in the system inspects.
const (
	maxAttributes    = 64
	maxAttributeKey  = 128
	maxAttributeSize = 8 << 10
)

// attributesInteractor implements IdentityAttributesUsecase.
//
// Every attribute an identity carries is sealed under a key belonging to that
// identity alone. That is what makes erasure real: retention destroys the key
// and the ciphertext becomes noise, without deleting the identity row that
// grants, sessions and audit entries all reference.
type attributesInteractor struct {
	guard  *guard.Guard
	pii    identityport.PIIRepository
	tx     txm.TxManager
	audit  auditport.Auditor
	master []byte
}

func NewAttributesInteractor(
	ops guard.OperatorAuthority,
	clockNow func() time.Time,
	piiRepo identityport.PIIRepository,
	tx txm.TxManager,
	audit auditport.Auditor,
	master []byte,
) IdentityAttributesUsecase {
	return &attributesInteractor{
		guard: guard.New().WithOperators(ops, clockNow),
		pii:   piiRepo, tx: tx, audit: audit, master: master,
	}
}

// IdentityAttributes returns an identity's attributes in the clear.
//
// It reports erased=true when the key has been shredded. That is not an error:
// the caller asked what this identity carries and the honest answer is "it was
// destroyed, and here is that fact" rather than a 500 that reads like a bug.
func (u *attributesInteractor) IdentityAttributes(ctx context.Context, id string) (map[string]string, bool, error) {
	p, err := u.guard.Require(ctx, "anubis:identity:read")
	if err != nil {
		return nil, false, err
	}
	if id == "" {
		return nil, false, apperr.ErrInvalidArgument
	}
	envelope, sealedKey, keyID, err := u.pii.IdentityAttributes(ctx, p.TenantID, id)
	if err != nil {
		return nil, false, err
	}
	if pii.IsEmpty(envelope) {
		return map[string]string{}, false, nil
	}
	if isErased(envelope, keyID, sealedKey) {
		return nil, true, nil
	}
	key, err := keyring.OpenSecret(u.master, keyKid(id), sealedKey)
	if err != nil {
		return nil, false, err
	}
	attrs, err := pii.OpenAttributes(key, id, envelope)
	if errors.Is(err, pii.ErrShredded) {
		return nil, true, nil
	}
	if err != nil {
		return nil, false, err
	}
	return attrs, false, nil
}

// SetIdentityAttributes replaces the whole attribute map. Replace rather than
// merge: a partial write has no way to express "remove this field", and a
// forgotten field that silently survives an erasure request is the failure
// this column exists to prevent.
func (u *attributesInteractor) SetIdentityAttributes(ctx context.Context, id string, attrs map[string]string) error {
	p, err := u.guard.Require(ctx, "anubis:identity:write")
	if err != nil {
		return err
	}
	if id == "" {
		return apperr.ErrInvalidArgument
	}
	if err := validateAttributes(attrs); err != nil {
		return err
	}
	err = u.tx.WithinTx(ctx, func(ctx context.Context) error {
		envelope, sealedKey, keyID, err := u.pii.IdentityAttributes(ctx, p.TenantID, id)
		if err != nil {
			return err
		}
		// Writing to an erased identity would mint a fresh key and make it
		// readable again, quietly undoing an erasure someone is legally
		// entitled to. Refuse instead.
		//
		// Erasure looks like this and not like a missing key: pii_shred
		// deletes the key row and the foreign key nulls pii_key_id, so an
		// erased identity is indistinguishable from a new one EXCEPT that a
		// ciphertext is still sitting in the column. That leftover envelope
		// is the evidence, which is why the same condition is what the read
		// path calls erased.
		if isErased(envelope, keyID, sealedKey) {
			return apperr.ErrConflict
		}
		if len(attrs) == 0 {
			if pii.IsEmpty(envelope) {
				return nil
			}
			return u.pii.SetIdentityAttributes(ctx, p.TenantID, id, pii.Empty())
		}

		key, err := u.ensureKey(ctx, p.TenantID, id, keyID, sealedKey)
		if err != nil {
			return err
		}
		sealed, err := pii.SealAttributes(key, id, attrs)
		if err != nil {
			return err
		}
		return u.pii.SetIdentityAttributes(ctx, p.TenantID, id, sealed)
	})
	if err != nil {
		return err
	}
	// The names are logged and the values are not. An audit trail that
	// recorded the values would be a second, unsealed copy of the PII this
	// column exists to protect.
	u.audit.Emit(ctx, auditdomain.AuditEvent{
		TenantID: p.TenantID, ActorID: p.IdentityID, ActorKind: "identity", TargetID: id,
		Action: "identity.attributes_set", Result: "allow",
		Detail: jsonx.Must(map[string]any{"fields": attributeNames(attrs)}),
	})
	return nil
}

// ensureKey returns the identity's own PII key, minting one on first write.
// The key is generated here and stored sealed under the master key, so the
// database never holds it in the clear.
func (u *attributesInteractor) ensureKey(ctx context.Context, tenantID, id, keyID string, sealedKey []byte) ([]byte, error) {
	if keyID != "" && len(sealedKey) > 0 {
		return keyring.OpenSecret(u.master, keyKid(id), sealedKey)
	}
	key, err := pii.NewKey()
	if err != nil {
		return nil, err
	}
	sealed, err := keyring.SealSecret(u.master, keyKid(id), key)
	if err != nil {
		return nil, err
	}
	newID, err := u.pii.CreatePIIKey(ctx, tenantID, sealed, "")
	if err != nil {
		return nil, err
	}
	if err := u.pii.SetIdentityPIIKey(ctx, tenantID, id, newID); err != nil {
		return nil, err
	}
	return key, nil
}

// keyKid binds a PII key to the identity it belongs to. Sealing under the
// identity id rather than the key row's id means the key can be minted in one
// statement, and a key row moved onto a different identity stops opening.
func keyKid(identityID string) string { return "pii:" + identityID }

// isErased reports that a ciphertext survives with no key to open it — the
// state pii_shred leaves behind. Callers must have established that the
// envelope is non-empty; an empty column with no key is simply an identity
// that never had attributes.
func isErased(envelope []byte, keyID string, sealedKey []byte) bool {
	return !pii.IsEmpty(envelope) && (keyID == "" || len(sealedKey) == 0)
}

func validateAttributes(attrs map[string]string) error {
	if len(attrs) > maxAttributes {
		return apperr.ErrInvalidArgument
	}
	total := 0
	for k, v := range attrs {
		if k == "" || len(k) > maxAttributeKey {
			return apperr.ErrInvalidArgument
		}
		total += len(k) + len(v)
	}
	if total > maxAttributeSize {
		return apperr.ErrInvalidArgument
	}
	return nil
}

func attributeNames(attrs map[string]string) []string {
	names := make([]string, 0, len(attrs))
	for k := range attrs {
		names = append(names, k)
	}
	return names
}
