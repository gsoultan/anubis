package identityapp

import "context"

// IdentityAdminUsecase is the composed identity-administration surface
// (proto IdentityAdminService). Composition, not width.
type IdentityAdminUsecase interface {
	IdentityLifecycleUsecase
	CredentialAdminUsecase
	ConsentAdminUsecase
}

// IdentityAttributesUsecase is the sealed side of the directory: the one
// column ADR-0013 encrypts, behind the one API that knows how to seal it.
//
// It is deliberately separate from IdentityLifecycleUsecase. Everything there
// reads and writes plaintext columns; everything here needs a key, and a
// caller that cannot tell the two apart is a caller that will eventually put
// a home address in `username`.
type IdentityAttributesUsecase interface {
	// IdentityAttributes returns the attributes in the clear. erased is true
	// when the key has been shredded — the data is gone, not broken.
	IdentityAttributes(ctx context.Context, id string) (attrs map[string]string, erased bool, err error)
	// SetIdentityAttributes replaces the whole map; an empty map clears it.
	SetIdentityAttributes(ctx context.Context, id string, attrs map[string]string) error
}
