package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"

	gen "github.com/gsoultan/anubis/internal/adapter/postgres/gen"
	"github.com/gsoultan/anubis/internal/domain"
	"github.com/gsoultan/anubis/internal/snapshot"
)

// LoadSnapshot freezes one tenant's catalog for the gate. Every query runs
// inside ONE REPEATABLE READ read-only transaction — loading tables from
// different MVCC snapshots yields a torn read: a grant referencing a scope
// node absent from the node map, silently wrong roughly weekly (ADR-0005 §10).
func (s *Store) LoadSnapshot(ctx context.Context, tenantID, tenantSlug string, revokedWindow time.Duration) (*snapshot.Data, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return nil, domain.ErrInternal.Wrap(err)
	}
	defer tx.Rollback(context.WithoutCancel(ctx)) //nolint:errcheck

	// Assert the isolation we asked for — a silently-downgraded level would
	// reintroduce torn reads without any failing test.
	var iso string
	if err := tx.QueryRow(ctx, "SHOW transaction_isolation").Scan(&iso); err != nil {
		return nil, domain.ErrInternal.Wrap(err)
	}
	if iso != "repeatable read" {
		return nil, domain.ErrInternal.Wrap(errIsolation(iso))
	}

	q := gen.New(tx)
	d := &snapshot.Data{
		TenantID:         tenantID,
		TenantSlug:       tenantSlug,
		LoadedAt:         time.Now(),
		StrictAxes:       map[string]bool{},
		Up:               map[string]map[string]int16{},
		GrantsByIdentity: map[string][]snapshot.Grant{},
		RolePermissions:  map[string]map[string]bool{},
		Permissions:      map[string]snapshot.Permission{},
		Identities:       map[string]snapshot.Identity{},
		RevokedSessions:  map[string]bool{},
	}

	if v, err := q.GetCatalogVersion(ctx, tenantID); err == nil {
		d.Version = v.Version
	}

	axes, err := q.SnapshotAxes(ctx)
	if err != nil {
		return nil, mapErr(err)
	}
	for _, a := range axes {
		if a.DefaultEffect == "deny" {
			d.StrictAxes[a.Code] = true
		}
	}

	closure, err := q.SnapshotClosure(ctx, tenantID)
	if err != nil {
		return nil, mapErr(err)
	}
	for _, c := range closure {
		up, ok := d.Up[c.DescendantID]
		if !ok {
			up = map[string]int16{}
			d.Up[c.DescendantID] = up
		}
		up[c.AncestorID] = c.Depth
	}

	grants, err := q.SnapshotGrants(ctx, tenantID)
	if err != nil {
		return nil, mapErr(err)
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
		return nil, mapErr(err)
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

	rps, err := q.SnapshotRolePermissions(ctx, tenantID)
	if err != nil {
		return nil, mapErr(err)
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
		return nil, mapErr(err)
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
		return nil, mapErr(err)
	}
	for _, i := range idents {
		d.Identities[i.ID] = snapshot.Identity{
			TokenEpoch:     int(i.TokenEpoch),
			Blocked:        derefBool(i.Blocked) || i.Status != "active",
			AssuranceLevel: int(i.AssuranceLevel),
		}
	}

	revoked, err := q.RecentlyRevokedSessions(ctx, gen.RecentlyRevokedSessionsParams{
		TenantID: tenantID, RevokedWindow: revokedWindow.String(),
	})
	if err != nil {
		return nil, mapErr(err)
	}
	for _, sid := range revoked {
		d.RevokedSessions[sid] = true
	}

	routes, err := q.SnapshotRoutes(ctx, tenantID)
	if err != nil {
		return nil, mapErr(err)
	}
	for _, r := range routes {
		var bindings map[string]string
		_ = json.Unmarshal(r.ScopeBindings, &bindings)
		d.Routes = append(d.Routes, snapshot.Route{
			AppSlug: r.ApplicationSlug, Priority: int(r.Priority), Effect: r.Effect,
			PathPattern: r.PathPattern, HostPattern: deref(r.HostPattern),
			Methods: r.Methods, PermissionKey: deref(r.PermissionKey),
			ScopeBindings: bindings,
		})
	}
	return d, nil
}

type errIsolation string

func (e errIsolation) Error() string {
	return "snapshot loaded under isolation " + string(e) + ", need repeatable read"
}

// WatchCatalog LISTENs on anubis_catalog and invokes onBump per notification
// (payload = tenant id). NOTIFY is the push path; the Manager's poll is the
// correctness backstop — notifications are not delivered across drops.
func (s *Store) WatchCatalog(ctx context.Context, onBump func(tenantID string)) error {
	conn, err := s.pool.Acquire(ctx)
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

func derefBool(b *bool) bool { return b != nil && *b }
