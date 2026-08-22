package usecase

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/gsoultan/anubis/internal/authctx"
	"github.com/gsoultan/anubis/internal/domain"
	"github.com/gsoultan/anubis/internal/repository"
)

// scopeAdminInteractor implements ScopeAdminUsecase.
type scopeAdminInteractor struct {
	guard *adminGuard
	axes  repository.ScopeAxisRepository
	nodes repository.ScopeNodeRepository
	sync  repository.ScopeSyncRepository
	authz repository.AuthzRepository
	tx    repository.TxManager
	audit repository.Auditor
}

func NewScopeAdminInteractor(
	authz repository.AuthzRepository,
	axes repository.ScopeAxisRepository,
	nodes repository.ScopeNodeRepository,
	sync repository.ScopeSyncRepository,
	tx repository.TxManager,
	audit repository.Auditor,
) ScopeAdminUsecase {
	return &scopeAdminInteractor{
		guard: newAdminGuard(authz), axes: axes, nodes: nodes, sync: sync,
		authz: authz, tx: tx, audit: audit,
	}
}

func (u *scopeAdminInteractor) emit(ctx context.Context, p *authctx.Principal, action, target string, detail map[string]string) {
	u.audit.Emit(ctx, repository.AuditEvent{
		TenantID: p.TenantID, ActorID: p.IdentityID, ActorKind: "identity",
		SessionID: p.SessionID, TargetID: target, Action: action, Result: "allow",
		IP: authctx.ClientIP(ctx), Detail: mustJSON(detail),
	})
}

func (u *scopeAdminInteractor) ListScopeAxes(ctx context.Context) ([]repository.ScopeAxisRecord, error) {
	if _, err := u.guard.require(ctx, "anubis:identity:read"); err != nil {
		return nil, err
	}
	return u.axes.ListScopeAxes(ctx)
}

func (u *scopeAdminInteractor) CreateScopeAxis(ctx context.Context, a repository.ScopeAxisRecord) (*repository.ScopeAxisRecord, error) {
	p, err := u.guard.require(ctx, "anubis:scope:admin")
	if err != nil {
		return nil, err
	}
	if !domain.ValidCode(a.Code) {
		return nil, domain.ErrInvalidArgument.With("code", a.Code)
	}
	if err := u.axes.CreateScopeAxis(ctx, a); err != nil {
		return nil, err
	}
	u.emit(ctx, p, "scope.axis_create", a.Code, map[string]string{"effect": a.DefaultEffect})
	return u.axes.ScopeAxis(ctx, a.Code)
}

func (u *scopeAdminInteractor) UpdateScopeAxis(ctx context.Context, a repository.ScopeAxisRecord) (*repository.ScopeAxisRecord, error) {
	p, err := u.guard.require(ctx, "anubis:scope:admin")
	if err != nil {
		return nil, err
	}
	if err := u.axes.UpdateScopeAxis(ctx, a); err != nil {
		return nil, err
	}
	u.emit(ctx, p, "scope.axis_update", a.Code, map[string]string{"effect": a.DefaultEffect, "status": a.Status})
	return u.axes.ScopeAxis(ctx, a.Code)
}

// StrictDryRun: replay recent allow decisions against a hypothetically-strict
// axis. On the seed data the flip took allows 800 -> 0 — better learned from
// this report than an outage (docs/api.md).
func (u *scopeAdminInteractor) StrictDryRun(ctx context.Context, axis string, sampleSize int) (int, int, string, error) {
	p, err := u.guard.require(ctx, "anubis:scope:admin")
	if err != nil {
		return 0, 0, "", err
	}
	if sampleSize <= 0 || sampleSize > 10000 {
		sampleSize = 2000
	}
	samples, err := u.authz.SampleAuthorizeDecisions(ctx, p.TenantID, sampleSize)
	if err != nil {
		return 0, 0, "", err
	}
	type sample struct {
		Subject    string            `json:"subject"`
		Permission string            `json:"permission"`
		Targets    map[string]string `json:"targets"`
	}
	wouldDeny := 0
	sampled := 0
	examples := make([]sample, 0, 20)
	for _, raw := range samples {
		var s sample
		if json.Unmarshal(raw, &s) != nil || s.Subject == "" || s.Permission == "" {
			continue
		}
		sampled++
		targets, _ := json.Marshal(s.Targets)
		allow, serr := u.authz.AuthorizeStrictSim(ctx, s.Subject, p.TenantID, s.Permission, targets, axis)
		if serr != nil {
			return 0, 0, "", serr
		}
		if !allow {
			wouldDeny++
			if len(examples) < 20 {
				examples = append(examples, s)
			}
		}
	}
	raw, _ := json.Marshal(examples)
	return sampled, wouldDeny, string(raw), nil
}

func (u *scopeAdminInteractor) ListScopeNodeTypes(ctx context.Context, axis string) ([]repository.ScopeNodeTypeRecord, error) {
	if _, err := u.guard.require(ctx, "anubis:identity:read"); err != nil {
		return nil, err
	}
	return u.nodes.ListScopeNodeTypes(ctx, axis)
}

func (u *scopeAdminInteractor) CreateScopeNodeType(ctx context.Context, t repository.ScopeNodeTypeRecord) error {
	p, err := u.guard.require(ctx, "anubis:scope:admin")
	if err != nil {
		return err
	}
	if err := u.nodes.CreateScopeNodeType(ctx, t); err != nil {
		return err
	}
	u.emit(ctx, p, "scope.node_type_create", t.Code, map[string]string{"axis": t.Axis})
	return nil
}

func (u *scopeAdminInteractor) ListScopeNodes(ctx context.Context, axis, parentID, query string, includeArchived bool) ([]repository.ScopeNodeRecord, error) {
	p, err := u.guard.require(ctx, "anubis:identity:read")
	if err != nil {
		return nil, err
	}
	return u.nodes.ListScopeNodes(ctx, p.TenantID, axis, parentID, query, includeArchived)
}

func (u *scopeAdminInteractor) CreateScopeNode(ctx context.Context, axis, nodeType, parentID, slug, name, externalRef string) (*repository.ScopeNodeRecord, error) {
	p, err := u.guard.require(ctx, "anubis:scope:admin")
	if err != nil {
		return nil, err
	}
	if parentID == "" {
		root, rerr := u.nodes.EnsureAxisRoot(ctx, p.TenantID, axis)
		if rerr != nil {
			return nil, rerr
		}
		parentID = root
	}
	if slug == "" {
		slug = slugify(name)
	}
	id, err := u.nodes.AddScopeNode(ctx, p.TenantID, axis, nodeType, parentID, slug, name, externalRef)
	if err != nil {
		return nil, err
	}
	u.emit(ctx, p, "scope.node_create", id, map[string]string{"axis": axis, "name": name})
	return u.nodes.ScopeNode(ctx, p.TenantID, id)
}

func (u *scopeAdminInteractor) EnsureAxisRoot(ctx context.Context, axis string) (string, error) {
	p, err := u.guard.require(ctx, "anubis:scope:admin")
	if err != nil {
		return "", err
	}
	return u.nodes.EnsureAxisRoot(ctx, p.TenantID, axis)
}

func (u *scopeAdminInteractor) MoveScopeNode(ctx context.Context, nodeID, newParentID string) error {
	p, err := u.guard.require(ctx, "anubis:scope:admin")
	if err != nil {
		return err
	}
	// Tenancy: both ends must live in the caller's tenant before the move.
	if _, err := u.nodes.ScopeNode(ctx, p.TenantID, nodeID); err != nil {
		return domain.ErrNotFound
	}
	if _, err := u.nodes.ScopeNode(ctx, p.TenantID, newParentID); err != nil {
		return domain.ErrNotFound
	}
	if err := u.nodes.MoveScopeNode(ctx, nodeID, newParentID); err != nil {
		return err
	}
	u.emit(ctx, p, "scope.node_move", nodeID, map[string]string{"new_parent": newParentID})
	return nil
}

func (u *scopeAdminInteractor) ArchiveScopeNode(ctx context.Context, nodeID string) error {
	p, err := u.guard.require(ctx, "anubis:scope:admin")
	if err != nil {
		return err
	}
	if err := u.nodes.ArchiveScopeNode(ctx, p.TenantID, nodeID); err != nil {
		return err
	}
	u.emit(ctx, p, "scope.node_archive", nodeID, nil)
	return nil
}

// UpsertScopeNodes: bulk reconcile keyed on external_ref. Adds, renames and
// moves run through the schema functions so the 0014 level guard still bites;
// nodes missing from the feed archive only when they carry a ref (manual
// nodes are never sync's to remove).
func (u *scopeAdminInteractor) UpsertScopeNodes(ctx context.Context, axis, defaultNodeType string, rows []SyncRowInput, dry bool) (string, error) {
	p, err := u.guard.require(ctx, "anubis:scope:admin")
	if err != nil {
		return "", err
	}
	if len(rows) > 50000 {
		return "", domain.ErrInvalidArgument.With("rows", "max 50000 per call")
	}
	report := map[string]any{"dry": dry}
	added, renamed, moved, unchanged := 0, 0, 0, 0
	archived := 0
	errs := []map[string]string{}

	run := func(ctx context.Context) error {
		root, rerr := u.nodes.EnsureAxisRoot(ctx, p.TenantID, axis)
		if rerr != nil {
			return rerr
		}
		seen := make(map[string]bool, len(rows))
		for _, row := range rows {
			if row.Ref == "" || row.Name == "" {
				errs = append(errs, map[string]string{"ref": row.Ref, "error": "missing ref or name"})
				continue
			}
			seen[row.Ref] = true
			parent := root
			if row.ParentRef != "" {
				pn, perr := u.nodes.ScopeNodeByRef(ctx, p.TenantID, axis, row.ParentRef)
				if perr != nil {
					errs = append(errs, map[string]string{"ref": row.Ref, "error": "parent ref not found (parents must come first)"})
					continue
				}
				parent = pn.ID
			}
			nodeType := row.NodeType
			if nodeType == "" {
				nodeType = defaultNodeType
			}
			existing, gerr := u.nodes.ScopeNodeByRef(ctx, p.TenantID, axis, row.Ref)
			switch {
			case gerr != nil: // new node
				if _, aerr := u.nodes.AddScopeNode(ctx, p.TenantID, axis, nodeType,
					parent, slugify(row.Name)+"-"+suffix(row.Ref), row.Name, row.Ref); aerr != nil {
					errs = append(errs, map[string]string{"ref": row.Ref, "error": domain.AsError(aerr).Message})
					continue
				}
				added++
			case existing.ParentID != parent:
				if merr := u.nodes.MoveScopeNode(ctx, existing.ID, parent); merr != nil {
					errs = append(errs, map[string]string{"ref": row.Ref, "error": domain.AsError(merr).Message})
					continue
				}
				moved++
			case existing.Name != row.Name || existing.Status == "archived":
				if rerr := u.nodes.RenameScopeNode(ctx, p.TenantID, existing.ID, row.Name); rerr != nil {
					errs = append(errs, map[string]string{"ref": row.Ref, "error": domain.AsError(rerr).Message})
					continue
				}
				renamed++
			default:
				unchanged++
			}
		}
		// Archive sync-owned nodes absent from the feed.
		all, lerr := u.nodes.ListScopeNodes(ctx, p.TenantID, axis, "", "", false)
		if lerr != nil {
			return lerr
		}
		for _, n := range all {
			if n.ExternalRef != "" && !seen[n.ExternalRef] && !n.IsAxisRoot {
				if aerr := u.nodes.ArchiveScopeNode(ctx, p.TenantID, n.ID); aerr == nil {
					archived++
				}
			}
		}
		return nil
	}

	if dry {
		sentinel := domain.E(domain.KindInternal, "dry_run_rollback", "dry run")
		if err := u.tx.WithinTx(ctx, func(ctx context.Context) error {
			if rerr := run(ctx); rerr != nil {
				return rerr
			}
			return sentinel
		}); err != nil && domain.AsError(err).Code != "dry_run_rollback" {
			return "", err
		}
	} else {
		if err := u.tx.WithinTx(ctx, run); err != nil {
			return "", err
		}
		u.emit(ctx, p, "scope.bulk_upsert", axis, map[string]string{"rows": itoaLen(rows)})
	}
	report["added"], report["renamed"], report["moved"] = added, renamed, moved
	report["archived"], report["unchanged"], report["errors"] = archived, unchanged, errs
	raw, _ := json.Marshal(report)
	return string(raw), nil
}

func (u *scopeAdminInteractor) ListSyncSources(ctx context.Context) ([]repository.SyncSourceRecord, error) {
	p, err := u.guard.require(ctx, "anubis:sync:admin")
	if err != nil {
		return nil, err
	}
	return u.sync.ListSyncSources(ctx, p.TenantID)
}

func (u *scopeAdminInteractor) CreateSyncSource(ctx context.Context, s repository.SyncSourceRecord) (*repository.SyncSourceRecord, error) {
	p, err := u.guard.require(ctx, "anubis:sync:admin")
	if err != nil {
		return nil, err
	}
	id, err := u.sync.CreateSyncSource(ctx, p.TenantID, s)
	if err != nil {
		return nil, err
	}
	u.emit(ctx, p, "sync.source_create", id, map[string]string{"axis": s.Axis, "kind": s.Kind})
	return u.sync.SyncSource(ctx, p.TenantID, id)
}

// RunSync pushes rows through the database-side reconciler
// (scope_sync_apply) with its per-row error capture and dry-run support.
func (u *scopeAdminInteractor) RunSync(ctx context.Context, sourceID string, rows []SyncRowInput, dry bool) (string, error) {
	p, err := u.guard.require(ctx, "anubis:sync:admin")
	if err != nil {
		return "", err
	}
	if _, err := u.sync.SyncSource(ctx, p.TenantID, sourceID); err != nil {
		return "", domain.ErrNotFound
	}
	raw, err := json.Marshal(rows)
	if err != nil {
		return "", domain.ErrInvalidArgument.Wrap(err)
	}
	report, err := u.sync.ScopeSyncApply(ctx, sourceID, raw, dry)
	if err != nil {
		return "", err
	}
	if !dry {
		u.emit(ctx, p, "sync.run", sourceID, map[string]string{"rows": itoaLen(rows)})
	}
	return report, nil
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(name string) string {
	s := slugRe.ReplaceAllString(strings.ToLower(name), "-")
	s = strings.Trim(s, "-")
	if len(s) > 40 {
		s = s[:40]
	}
	if s == "" {
		s = "node"
	}
	return s
}

func suffix(ref string) string {
	if len(ref) > 6 {
		ref = ref[len(ref)-6:]
	}
	return strings.ToLower(ref)
}

func itoaLen[T any](v []T) string {
	n := len(v)
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
