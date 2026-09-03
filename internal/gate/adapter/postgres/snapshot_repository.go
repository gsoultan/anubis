package gatepg

import (
	"context"
	"encoding/json"
	"time"

	gen "github.com/gsoultan/anubis/internal/gate/adapter/postgres/gen"
	"github.com/gsoultan/anubis/internal/gate/snapshot"
	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/jackc/pgx/v5"
)

// LoadSnapshot freezes one tenant's catalog for the gate. Every query runs
// inside ONE REPEATABLE READ read-only transaction — loading tables from
// different MVCC snapshots yields a torn read: a grant referencing a scope
// node absent from the node map, silently wrong roughly weekly (ADR-0005 §10).
func (s *Repository) LoadSnapshot(ctx context.Context, tenantID, tenantSlug string, revokedWindow time.Duration) (*snapshot.Data, error) {
	tx, err := s.Pool().BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}
	defer tx.Rollback(context.WithoutCancel(ctx)) //nolint:errcheck

	// Assert the isolation we asked for — a silently-downgraded level would
	// reintroduce torn reads without any failing test.
	var iso string
	if err := tx.QueryRow(ctx, "SHOW transaction_isolation").Scan(&iso); err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}
	if iso != "repeatable read" {
		return nil, apperr.ErrInternal.Wrap(errIsolation(iso))
	}

	q := gen.New(tx)
	d := &snapshot.Data{
		TenantID:         tenantID,
		TenantSlug:       tenantSlug,
		LoadedAt:         time.Now(),
		BuiltAt:          time.Now(),
		StrictAxes:       map[string]bool{},
		GrantsByIdentity: map[string][]snapshot.Grant{},
		RolePermissions:  map[string]map[string]bool{},
		Permissions:      map[string]snapshot.Permission{},
		Identities:       map[string]snapshot.Identity{},
		RevokedSessions:  map[string]bool{},
	}

	if v, err := q.SnapshotCatalogVersion(ctx, tenantID); err == nil {
		d.Version = v.Version
	}

	axes, err := q.SnapshotAxes(ctx)
	if err != nil {
		return nil, database.MapErr(err)
	}
	for _, a := range axes {
		if a.DefaultEffect == "deny" {
			d.StrictAxes[a.Code] = true
		}
	}

	// Parent pointers, not the closure: one row per node instead of one per
	// (node, ancestor) pair. ScopeIndex walks them to answer the same
	// question the closure answered by lookup.
	nodes, err := q.SnapshotNodes(ctx, tenantID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	hierarchy := make([]snapshot.ScopeNode, len(nodes))
	for i, n := range nodes {
		hierarchy[i] = snapshot.ScopeNode{ID: n.ID}
		if n.ParentID != nil {
			hierarchy[i].Parent = *n.ParentID
		}
	}
	d.Scope = snapshot.NewScopeIndex(hierarchy)

	grants, err := q.SnapshotGrants(ctx, tenantID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	byGrant := map[string]*snapshot.Grant{}
	order := map[string]string{} // grant id -> identity
	for _, g := range grants {
		sg := snapshot.Grant{
			ID: g.ID, RoleID: g.RoleID, SelfScoped: g.SelfScoped,
			ValidFrom: g.ValidFrom, Scopes: map[string][]snapshot.ScopeConstraint{},
		}
		if g.ValidUntil != nil {
			sg.ValidUntil = *g.ValidUntil
		}
		byGrant[g.ID] = &sg
		order[g.ID] = g.IdentityID
	}
	gscopes, err := q.SnapshotGrantScopes(ctx, tenantID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	for _, gs := range gscopes {
		if g, ok := byGrant[gs.GrantID]; ok {
			g.Scopes[gs.AxisCode] = append(g.Scopes[gs.AxisCode], snapshot.ScopeConstraint{
				NodeID: gs.ScopeNodeID, Inherit: gs.Inherit,
			})
		}
	}
	for id, g := range byGrant {
		ident := order[id]
		d.GrantsByIdentity[ident] = append(d.GrantsByIdentity[ident], *g)
	}
	// Both the index and the grants exist now, so the constraints can be
	// resolved to dense indices. Skipping this denies everything (by design —
	// see ScopeConstraint.node), so it must not move above either load.
	d.InternGrantScopes()

	rps, err := q.SnapshotRolePermissions(ctx, tenantID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	for _, rp := range rps {
		m, ok := d.RolePermissions[rp.RoleID]
		if !ok {
			m = map[string]bool{}
			d.RolePermissions[rp.RoleID] = m
		}
		if rp.Key != nil {
			m[*rp.Key] = true
		}
	}

	perms, err := q.SnapshotPermissions(ctx, tenantID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	for _, p := range perms {
		if p.Key == nil {
			continue
		}
		sp := snapshot.Permission{
			Key: *p.Key, MinAssurance: int(p.MinAssurance),
			RequiresAMR: p.RequiresAmr, Risk: p.Risk,
		}
		sp.MaxAuthAgeSecs = p.MaxAuthAgeSecs
		d.Permissions[*p.Key] = sp
	}

	idents, err := q.SnapshotIdentities(ctx, tenantID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	for _, i := range idents {
		d.Identities[i.ID] = snapshot.Identity{
			TokenEpoch:     int(i.TokenEpoch),
			Blocked:        database.DerefBool(i.Blocked) || i.Status != "active",
			AssuranceLevel: int(i.AssuranceLevel),
		}
	}

	revoked, err := q.SnapshotRevokedSessions(ctx, gen.SnapshotRevokedSessionsParams{
		TenantID: tenantID, RevokedWindow: revokedWindow.String(),
	})
	if err != nil {
		return nil, database.MapErr(err)
	}
	for _, sid := range revoked {
		d.RevokedSessions[sid] = true
	}

	routes, err := q.SnapshotRoutes(ctx, tenantID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	for _, r := range routes {
		var bindings map[string]string
		_ = json.Unmarshal(r.ScopeBindings, &bindings)
		d.Routes = append(d.Routes, snapshot.Route{
			AppSlug: r.ApplicationSlug, Priority: int(r.Priority), Effect: r.Effect,
			PathPattern: r.PathPattern, HostPattern: database.Deref(r.HostPattern),
			Methods: r.Methods, PermissionKey: database.Deref(r.PermissionKey),
			ScopeBindings: bindings,
		})
	}
	return d, nil
}

type errIsolation string

func (e errIsolation) Error() string {
	return "snapshot loaded under isolation " + string(e) + ", need repeatable read"
}

// CatalogVersion reads just the invalidation counter — one indexed row.
// The Manager asks this before rebuilding, so an unchanged tenant costs a
// single lookup instead of a full snapshot load. Sound only because every
// table carrying authorization state bumps the counter (migrations 0005/0006
// and 0040); TestSnapshotTablesAreClassifiedPushOrPoll pins that.
func (s *Repository) CatalogVersion(ctx context.Context, tenantID string) (int64, error) {
	v, err := gen.New(s.Pool()).SnapshotCatalogVersion(ctx, tenantID)
	if err != nil {
		return 0, database.MapErr(err)
	}
	return v.Version, nil
}

// WatchCatalog LISTENs on anubis_catalog and invokes onBump per notification
// (payload = tenant id). NOTIFY is the push path; the Manager's poll is the
// correctness backstop — notifications are not delivered across drops.
func (s *Repository) WatchCatalog(ctx context.Context, onBump func(tenantID string)) error {
	conn, err := s.Pool().Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	raw := conn.Conn()
	if _, err := raw.Exec(ctx, "LISTEN anubis_catalog"); err != nil {
		return err
	}
	for {
		n, err := raw.WaitForNotification(ctx)
		if err != nil {
			return err
		}
		onBump(n.Payload)
	}
}
