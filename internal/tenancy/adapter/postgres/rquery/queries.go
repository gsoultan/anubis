// Package tenancyrquery is the ONE place where this context's SQL lives in Go
// — the successor to db/queries/tenancy/*.sql under ADR-0009 §5: SQL is
// reviewable in exactly one package per context, and every statement here is
// PREPAREd against the live schema at generate time, so a query that drifts
// from the database fails the build naming the column, not the request.
//
// Files mirror the old db/queries/tenancy layout (application, pages, signin,
// tenant) so review diffs read side by side.
//
// What is here, and why it is not a builder:
//
//   - The JOINS. An auth page resolves its application's slug and its realm's
//     code for display, and both are LEFT joins because a page may be bound to
//     neither. Reading them separately is two more round trips per page.
//   - The INTERVAL renderings. An application's TTLs are stored as interval
//     and travel to the API as seconds and as text; extract(epoch FROM …) is
//     the conversion, and it belongs where the column is.
//   - The updates setting a SERVER-side expression — `updated_at = now()`,
//     `manifest_version = manifest_version + 1`. The second is the reason:
//     computed in Go it is a read-modify-write, and two concurrent edits would
//     both publish version N+1 while the client cache keyed on it kept one of
//     them forever.
//   - signin_pages, which is a VIEW over auth_pages. storm models tables.
package tenancyrquery

import "github.com/gsoultan/storm"

// Queries is what cmd/stormgen validates and emits scanners for. Every
// declaration in the package MUST be listed: an omitted one does not merely
// lose its generate-time schema check, it REFUSES TO RUN, and the build is
// clean and the tests pass while a scheduled job is where you find out.
func Queries() []storm.RawDecl {
	return []storm.RawDecl{
		// application.go
		GetApplicationBySlug, GetApplication, ListApplications, AllApplications,
		CreateApplication, UpdateApplication, SetClientSecretHash, BumpManifestVersion,
		ListRoutePoliciesByApp, DeleteRoutePoliciesByApp,
		// pages.go
		ListAuthPages, GetAuthPage, GetAuthPageBySlug, GetDefaultAuthPage,
		UpdateAuthPage, DeleteAuthPage, ClearDefaultAuthPage, SetDefaultAuthPage,
		// signin.go
		GetSigninPage, PutSigninPage,
		// tenant.go
		UpdateTenant, SetTenantStatus, BumpCatalogVersion,
		CountTenantIdentities, GetTenantStats,
	}
}
