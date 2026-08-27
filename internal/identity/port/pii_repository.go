package identityport

import "context"

// PIIRepository owns the per-identity key lifecycle behind crypto-shredding
// (migrations/0022). Shred is idempotent: erasing an already-erased identity
// reports false, not an error — "already unrecoverable" is the goal state.
type PIIRepository interface {
	CreatePIIKey(ctx context.Context, tenantID string, sealed []byte, kmsRef string) (string, error)
	PIIKey(ctx context.Context, tenantID, id string) (sealed []byte, err error)
	SetIdentityPIIKey(ctx context.Context, tenantID, identityID, keyID string) error
	ShredPIIKey(ctx context.Context, keyID, reason string) (bool, error)

	// IdentityAttributes returns the sealed envelope, the id of the key that
	// opens it, and that key still sealed under the master key. A key id with
	// no key material means retention shredded it: the envelope is noise
	// forever, which is a successful erasure rather than a fault.
	IdentityAttributes(ctx context.Context, tenantID, identityID string) (envelope, sealedKey []byte, keyID string, err error)
	SetIdentityAttributes(ctx context.Context, tenantID, identityID string, envelope []byte) error
}
