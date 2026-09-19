package controlrquery

import (
	"time"

	"github.com/gsoultan/storm"
	"github.com/gsoultan/storm/runtime"
)

// AssignmentRow is one control-plane assignment. TenantID and TenantSlug are
// empty for a GLOBAL assignment — an owner of the installation rather than of
// one tenant — which is a different fact from a tenant that has no slug.
type AssignmentRow struct {
	ID         string
	OperatorID string
	TenantID   string
	TenantSlug string
	Role       string
	GrantedBy  string
	Reason     string
	ValidUntil runtime.Null[time.Time]
	RevokedAt  runtime.Null[time.Time]
	CreatedAt  time.Time
}

// ListAssignments is the user-management page's read: every live assignment,
// with the tenant slug resolved for display.
//
// LEFT JOIN, not JOIN: a global assignment has no tenant, and an inner join
// would silently drop exactly the rows that carry the most authority.
var ListAssignments = storm.SQL[AssignmentRow](`
SELECT a.id::text AS id, a.operator_id::text AS operator_id,
       COALESCE(a.tenant_id::text, '') AS tenant_id,
       COALESCE(t.slug, '') AS tenant_slug,
       a.role, COALESCE(a.granted_by::text, '') AS granted_by,
       a.reason, a.valid_until, a.revoked_at, a.created_at
  FROM platform_assignments a
  LEFT JOIN tenants t ON t.id = a.tenant_id
 WHERE a.revoked_at IS NULL
 ORDER BY a.operator_id, a.tenant_id NULLS FIRST`)

// ListAssignmentsForOperator is the guard's lookup, run on EVERY admin call an
// operator makes.
//
// It joins platform_users and requires the account to be active, so disabling
// somebody takes their live tokens down with them: without the join, a
// disabled operator kept working until their token happened to expire, and
// "disabled" that does not disable is worse than no button at all.
var ListAssignmentsForOperator = storm.SQL[AssignmentRow](`
SELECT a.id::text AS id, a.operator_id::text AS operator_id,
       COALESCE(a.tenant_id::text, '') AS tenant_id,
       '' AS tenant_slug,
       a.role, COALESCE(a.granted_by::text, '') AS granted_by,
       a.reason, a.valid_until, a.revoked_at, a.created_at
  FROM platform_assignments a
  JOIN platform_users u ON u.id = a.operator_id
 WHERE a.operator_id = $1
   AND a.revoked_at IS NULL
   AND u.status = 'active'
 ORDER BY a.tenant_id NULLS FIRST`)

// RevokeAssignment revokes a live assignment. Server clock, and guarded so the
// row count distinguishes "revoked" from "already was".
var RevokeAssignment = storm.SQLExec(`
UPDATE platform_assignments
   SET revoked_at = now(), updated_at = now()
 WHERE id = $1 AND revoked_at IS NULL`)

// PresentRow answers an EXISTS.
type PresentRow struct {
	Present bool
}

// HasAnyPlatformOwner reports whether the installation has a global owner.
//
// EXISTS rather than count: the question is whether there is at least one, and
// the planner can stop at the first row.
var HasAnyPlatformOwner = storm.SQL[PresentRow](`
SELECT EXISTS (
    SELECT 1 FROM platform_assignments
     WHERE tenant_id IS NULL AND role = 'owner' AND revoked_at IS NULL
) AS present`)
