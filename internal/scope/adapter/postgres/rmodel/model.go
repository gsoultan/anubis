// Package scopermodel declares the scope tables storm generates code for.
//
// It is a PROJECTION of the schema, not its source of truth: anubis's schema
// of record stays migrations/ (forward-only, checksummed), and this model is
// validated against the live schema the same way the raw queries are — by
// preparing generated statements against it. Columns and defaults here must
// match `\d` on the table exactly.
//
// `tenants` belongs to the tenancy context, so a node and a sync source carry
// tenant_id as a plain column rather than a relation — the boundary AGENTS.md
// draws. `scope_closure` is absent: it is maintained entirely by triggers and
// read only through scope_ancestors, so a model would describe a table nothing
// in Go ever writes.
package scopermodel

import (
	"time"

	"github.com/gsoultan/storm"
)

// ScopeAxis is public.scope_axes: one dimension a grant can be scoped along.
type ScopeAxis struct {
	CreatedAt     time.Time
	SortOrder     int32
	Code          string
	DisplayName   string
	DefaultEffect string
	Status        string

	// Resolution says where a request's value for this axis comes from.
	Resolution storm.JSON
	UiSchema   storm.JSON
}

func (m *ScopeAxis) Schema(t *storm.Table) {
	t.Name("scope_axes")
	t.PrimaryKey(&m.Code)
	t.Col(&m.CreatedAt).Default("now()")
	t.Col(&m.SortOrder).Default("100")
	t.Col(&m.DefaultEffect).Default("'unconstrained'::text")
	t.Col(&m.Status).Default("'active'::text")
	t.Col(&m.Resolution).Default(`'{"from": "context"}'::jsonb`)
	t.Col(&m.UiSchema).Default("'{}'::jsonb")
	t.CheckNamed("scope_axes_code_check", "code ~ '^[a-z][a-z0-9_]{1,30}$'::text")
	t.CheckNamed("scope_axes_default_effect_check",
		"default_effect = ANY (ARRAY['unconstrained'::text, 'deny'::text])")
	t.CheckNamed("scope_axes_status_check",
		"status = ANY (ARRAY['active'::text, 'deprecated'::text])")
}

// ScopeNodeType is public.scope_node_types: what kinds of node an axis has,
// and which types may parent which.
type ScopeNodeType struct {
	Code        string
	AxisCode    string
	DisplayName string
	ParentTypes []string
}

func (m *ScopeNodeType) Schema(t *storm.Table) {
	var axis ScopeAxis

	t.Name("scope_node_types")
	t.PrimaryKey(&m.Code)
	t.Col(&m.ParentTypes).Default("'{}'::text[]")
	t.UniqueNamed("scope_node_types_code_axis_code_key", &m.Code, &m.AxisCode)
	t.CheckNamed("scope_node_types_code_check", "code ~ '^[a-z][a-z0-9_]{1,30}$'::text")
	t.ForeignKey(&m.AxisCode).References(&axis, &axis.Code).
		Named("scope_node_types_axis_code_fkey").OnDelete(storm.Restrict).NoIndex()
}

// ScopeNode is public.scope_nodes: one place in one axis's tree.
//
// Every node but the axis root has a parent, and the tree is per (tenant,
// axis). The closure table that answers ancestry is maintained by triggers.
type ScopeNode struct {
	storm.Model

	TenantID    [16]byte
	ParentID    *[16]byte
	IsAxisRoot  bool
	Status      string
	AxisCode    string
	NodeType    string
	Slug        string
	Name        string
	ExternalRef *string
	Attributes  storm.JSON
}

func (m *ScopeNode) Schema(t *storm.Table) {
	var parent ScopeNode
	var nt ScopeNodeType

	t.Name("scope_nodes")
	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.IsAxisRoot).Default("false")
	t.Col(&m.Status).Default("'active'::text")
	t.Col(&m.Attributes).Default("'{}'::jsonb")

	// (id, tenant_id, axis_code) is what the self-reference below points at:
	// a parent must be in the same tenant AND the same axis, which a single
	// key to id alone could not say.
	t.UniqueNamed("scope_nodes_id_tenant_id_axis_code_key", &m.ID, &m.TenantID, &m.AxisCode)

	t.CheckNamed("nonroot_has_parent", "is_axis_root OR (parent_id IS NOT NULL)")
	t.CheckNamed("root_has_no_parent", "(NOT is_axis_root) OR (parent_id IS NULL)")
	t.CheckNamed("scope_nodes_slug_check", "slug <> ''::text")
	t.CheckNamed("scope_nodes_status_check",
		"status = ANY (ARRAY['active'::text, 'archived'::text])")

	t.Index(&m.TenantID, &m.AxisCode).Include(&m.ParentID, &m.NodeType).
		Where("status = 'active'::text").Named("scope_nodes_axis")
	t.Index(&m.TenantID, &m.AxisCode, &m.ExternalRef).Unique().
		Where("external_ref IS NOT NULL").Named("scope_nodes_extref")
	t.Index(&m.TenantID, &m.AxisCode).Unique().
		Where("is_axis_root").Named("scope_nodes_one_root")
	// The keyset paging index (0039): (name, id) is the cursor, and name is
	// not unique, which is why id is in the key as well as in the ORDER BY.
	t.Index(&m.TenantID, &m.AxisCode, &m.Name, &m.ID).Named("scope_nodes_paging")
	t.Index(&m.ParentID, &m.Slug).Unique().
		Where("parent_id IS NOT NULL").Named("scope_nodes_sibling_slug")

	t.ForeignKey(&m.NodeType, &m.AxisCode).References(&nt, &nt.Code, &nt.AxisCode).
		Named("scope_nodes_node_type_axis_code_fkey").OnDelete(storm.Restrict).NoIndex()
	// Self-referential and three columns wide: the parent must be the same
	// tenant and the same axis, or a tree could be grafted across either.
	t.ForeignKey(&m.ParentID, &m.TenantID, &m.AxisCode).
		References(&parent, &parent.ID, &parent.TenantID, &parent.AxisCode).
		Named("scope_nodes_parent_id_tenant_id_axis_code_fkey").OnDelete(storm.Restrict)
}

// ScopeSyncSource is public.scope_sync_sources: a feed that keeps one axis's
// tree in step with somebody else's system.
type ScopeSyncSource struct {
	CreatedAt time.Time
	LastRunAt *time.Time
	ID        [16]byte
	TenantID  [16]byte
	AxisCode  string
	Kind      string
	Status    string

	// Config holds the feed's credentials. It is replaced wholesale on an
	// edit rather than merged — merging secrets is how half-rotated
	// credentials happen — which is why rescheduling has its own statement.
	Config          storm.JSON
	NextRunAt       *time.Time
	IntervalSeconds int32
}

func (m *ScopeSyncSource) Schema(t *storm.Table) {
	var axis ScopeAxis

	t.Name("scope_sync_sources")
	t.PrimaryKey(&m.ID)
	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.CreatedAt).Default("now()")
	t.Col(&m.Status).Default("'active'::text")
	t.Col(&m.IntervalSeconds).Default("0")

	t.UniqueNamed("scope_sync_sources_tenant_id_axis_code_key", &m.TenantID, &m.AxisCode)
	// Zero means manual. The floor exists so a misconfigured feed cannot
	// hammer somebody else's server every second.
	t.CheckNamed("scope_sync_sources_interval_seconds_check",
		"(interval_seconds = 0) OR (interval_seconds >= 300)")
	t.CheckNamed("scope_sync_sources_kind_check",
		"kind = ANY (ARRAY['http'::text, 'db_query'::text, 'db_table'::text])")
	t.CheckNamed("scope_sync_sources_status_check",
		"status = ANY (ARRAY['active'::text, 'disabled'::text])")
	// The scheduler's index: partial, because only a live scheduled source is
	// ever due, and that is a small slice of the table.
	t.Index(&m.NextRunAt).
		Where("(status = 'active'::text) AND (next_run_at IS NOT NULL)").
		Named("scope_sync_sources_due")
	t.ForeignKey(&m.AxisCode).References(&axis, &axis.Code).
		Named("scope_sync_sources_axis_code_fkey").OnDelete(storm.Cascade).NoIndex()
}

// ScopeSyncRun is public.scope_sync_runs: what one sync attempt did.
//
// It carries no tenant_id — a run is reachable only through its source, which
// is what scopes the history to the tenant that owns the feed.
type ScopeSyncRun struct {
	StartedAt  time.Time
	FinishedAt *time.Time
	ID         [16]byte
	SourceID   [16]byte
	Dry        bool
	Status     string
	Report     storm.JSON
}

func (m *ScopeSyncRun) Schema(t *storm.Table) {
	var src ScopeSyncSource

	t.Name("scope_sync_runs")
	t.PrimaryKey(&m.ID)
	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.StartedAt).Default("now()")
	t.Col(&m.Dry).Default("false")
	t.Col(&m.Status).Default("'running'::text")
	t.Col(&m.Report).Default("'{}'::jsonb")
	t.CheckNamed("scope_sync_runs_status_check",
		"status = ANY (ARRAY['running'::text, 'ok'::text, 'failed'::text, 'dry_run'::text])")
	t.Index(&m.SourceID, storm.Desc(&m.StartedAt)).Named("scope_sync_runs_src")
	t.ForeignKey(&m.SourceID).References(&src, &src.ID).
		Named("scope_sync_runs_source_id_fkey").OnDelete(storm.Cascade)
}

func All() []any {
	return []any{&ScopeAxis{}, &ScopeNodeType{}, &ScopeNode{}, &ScopeSyncSource{}, &ScopeSyncRun{}}
}
