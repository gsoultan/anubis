// Package identityrquery is the ONE place where this context's SQL lives in Go
// — the successor to db/queries/identity/*.sql under ADR-0009 §5: SQL is
// reviewable in exactly one package per context, and every statement here is
// PREPAREd against the live schema at generate time, so a query that drifts
// from the database fails the build naming the column, not the request.
//
// identities and realms are entirely here, with no model at all. Every read of
// them LEFT JOINs the realm and the category for display, or renders an
// interval as text AND as seconds; every write is a guarded transition that
// bumps token_epoch in the DATABASE — computed in Go that is a
// read-modify-write, and two concurrent disables would lose an increment and
// leave one person's tokens live.
//
// Files mirror the old db/queries/identity layout (identity, credential,
// consent, pii, realm) so review diffs read side by side.
package identityrquery

import "github.com/gsoultan/storm"

// Queries is what cmd/stormgen validates and emits scanners for. Every
// declaration in the package MUST be listed: an omitted one does not merely
// lose its generate-time schema check, it REFUSES TO RUN, and the build is
// clean and the tests pass while a scheduled job is where you find out.
func Queries() []storm.RawDecl {
	return []storm.RawDecl{
		// identity.go
		GetIdentityForLogin, GetIdentity, ListIdentities, CreateIdentity,
		DisableIdentity, EnableIdentity, BumpTokenEpoch, TouchLastLogin,
		RequestErasure, LinkIdentities, GetIdentityAuthState,
		ExpireRetainedIdentities, SetRetentionFromRealm, AnonymizeIdentity,
		CountIdentitiesByRealm, CountRetentionBacklog, CountIdentitiesByCategory,
		// credential.go
		GetPasswordCredential, CreateCredential, RevokeCredential,
		RevokeCredentialsOfKind, TouchCredentialUsed, UpdateCredentialSecret,
		UpdateCredentialParams, ListActiveCredentialKinds,
		GetActiveCredentialOfKind, ConsumeRecoveryCode,
		// consent.go
		WithdrawConsent,
		// pii.go
		ShredPIIKey, SetIdentityPIIKey, GetIdentityAttributes, SetIdentityAttributes,
		ListResealablePIIKeys, ResealPIIKey,
		// realm.go
		GetRealmByCode, GetRealm, ListRealms, CreateRealm, UpdateRealm,
		CorrectEmptyRealmIdentity,
	}
}
