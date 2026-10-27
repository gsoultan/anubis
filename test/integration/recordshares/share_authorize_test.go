package recordshares

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

/*
Record shares (0058), held to authorize() — the function the gate and every
decision run — rather than to the row.

Every test builds its own world inside a transaction that is rolled back: a
permission, a sharer, a recipient. The sharer holds nothing; the recipient
holds nothing; only the share says yes, and only where it should.
*/

type world struct {
	tenant, perm, sharer, recipient string
	appID                           string
}

func buildWorld(t *testing.T, ctx context.Context, tx pgx.Tx) world {
	t.Helper()
	w := world{tenant: firstTenant(ctx, t)}
	sfx := strconv.FormatInt(time.Now().UnixNano()%1_000_000_000, 36)
	var realm string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM realms WHERE tenant_id = $1 ORDER BY created_at LIMIT 1`, w.tenant).Scan(&realm); err != nil {
		t.Fatalf("no realm in the first tenant: %v", err)
	}
	must := func(what string, err error) {
		if err != nil {
			t.Fatalf("%s: %v", what, err)
		}
	}
	must("application", tx.QueryRow(ctx, `INSERT INTO applications (tenant_id, kind, slug, name)
		VALUES ($1, 'spa', $2, $2) RETURNING id::text`, w.tenant, "rs_"+sfx).Scan(&w.appID))
	must("permission", tx.QueryRow(ctx, `INSERT INTO permissions (tenant_id, application_id, app_slug, resource, action)
		VALUES ($1, $2, $3, 'requisition', 'comment') RETURNING key`, w.tenant, w.appID, "rs_"+sfx).Scan(&w.perm))
	must("sharer", tx.QueryRow(ctx, `INSERT INTO identities (tenant_id, realm_id, username)
		VALUES ($1, $2, $3) RETURNING id::text`, w.tenant, realm, "sharer_"+sfx).Scan(&w.sharer))
	must("recipient", tx.QueryRow(ctx, `INSERT INTO identities (tenant_id, realm_id, username)
		VALUES ($1, $2, $3) RETURNING id::text`, w.tenant, realm, "recipient_"+sfx).Scan(&w.recipient))
	return w
}

func (w world) allows(t *testing.T, ctx context.Context, tx pgx.Tx, identity, permission, targets string) bool {
	t.Helper()
	var ok bool
	if err := tx.QueryRow(ctx, `SELECT authorize($1, $2, $3, $4::jsonb)`, identity, w.tenant, permission, targets).Scan(&ok); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	return ok
}

func (w world) share(t *testing.T, ctx context.Context, tx pgx.Tx, record string, perms []string, until string) string {
	t.Helper()
	var id string
	if err := tx.QueryRow(ctx, `INSERT INTO record_shares (tenant_id, from_identity_id, to_identity_id, record, permission_keys, valid_until)
		VALUES ($1, $2, $3, $4, $5, now() + $6::interval) RETURNING id::text`,
		w.tenant, w.sharer, w.recipient, record, perms, until).Scan(&id); err != nil {
		t.Fatalf("share: %v", err)
	}
	return id
}

func inRolledBackTx(t *testing.T, fn func(ctx context.Context, tx pgx.Tx)) {
	t.Helper()
	skipWithoutDB(t)
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	fn(ctx, tx)
}

// The whole contract in one table: a share says yes on its record, for its
// permissions, to its recipient, while it lasts — and nowhere else.
func TestAShareAllowsExactlyWhatItNames(t *testing.T) {
	inRolledBackTx(t, func(ctx context.Context, tx pgx.Tx) {
		w := buildWorld(t, ctx, tx)
		rec := `{"_record":"job_requisition/JR-0142"}`

		if w.allows(t, ctx, tx, w.recipient, w.perm, rec) {
			t.Fatal("allowed before any share existed")
		}
		w.share(t, ctx, tx, "job_requisition/JR-0142", []string{w.perm}, "7 days")

		cases := map[string]struct {
			identity, permission, targets string
			want                          bool
		}{
			"the shared record":          {w.recipient, w.perm, rec, true},
			"another record":             {w.recipient, w.perm, `{"_record":"job_requisition/JR-0001"}`, false},
			"no record named":            {w.recipient, w.perm, `{}`, false},
			"a place named, no record":   {w.recipient, w.perm, `{"org":"` + w.tenant + `"}`, false},
			"a permission not shared":    {w.recipient, "rs_x:requisition:approve", rec, false},
			"the sharer, who holds none": {w.sharer, w.perm, rec, false},
		}
		for name, c := range cases {
			if got := w.allows(t, ctx, tx, c.identity, c.permission, c.targets); got != c.want {
				t.Errorf("%s: authorize=%v want %v", name, got, c.want)
			}
		}
	})
}

func TestAShareEndsWhenRevokedOrLapsedOrTheRecipientIsDisabled(t *testing.T) {
	inRolledBackTx(t, func(ctx context.Context, tx pgx.Tx) {
		w := buildWorld(t, ctx, tx)
		rec := `{"_record":"job_requisition/JR-0142"}`
		id := w.share(t, ctx, tx, "job_requisition/JR-0142", []string{w.perm}, "7 days")
		if !w.allows(t, ctx, tx, w.recipient, w.perm, rec) {
			t.Fatal("a live share was refused")
		}

		if _, err := tx.Exec(ctx, `UPDATE record_shares SET revoked_at = now() WHERE id = $1`, id); err != nil {
			t.Fatal(err)
		}
		if w.allows(t, ctx, tx, w.recipient, w.perm, rec) {
			t.Fatal("a revoked share still allowed")
		}

		// A share that is live by the row but whose recipient was disabled:
		// the identity gate outranks the share, as it outranks grants.
		w.share(t, ctx, tx, "job_requisition/JR-0142", []string{w.perm}, "7 days")
		if _, err := tx.Exec(ctx, `UPDATE identities SET disabled_at = now() WHERE id = $1`, w.recipient); err != nil {
			t.Fatal(err)
		}
		if w.allows(t, ctx, tx, w.recipient, w.perm, rec) {
			t.Fatal("a disabled recipient was let through by a share")
		}
	})
}

// A share that is made or ended must reach the snapshot: the catalog version
// bumps, which is what the gate polls.
func TestAShareBumpsTheCatalogVersion(t *testing.T) {
	inRolledBackTx(t, func(ctx context.Context, tx pgx.Tx) {
		w := buildWorld(t, ctx, tx)
		version := func() int64 {
			var v int64
			if err := tx.QueryRow(ctx, `SELECT COALESCE((SELECT version FROM catalog_version WHERE tenant_id = $1), 0)`, w.tenant).Scan(&v); err != nil {
				t.Fatalf("catalog version: %v", err)
			}
			return v
		}
		before := version()
		id := w.share(t, ctx, tx, "job_requisition/JR-0142", []string{w.perm}, "1 day")
		if version() <= before {
			t.Fatal("making a share did not bump the catalog version; the gate would not learn of it")
		}
		mid := version()
		if _, err := tx.Exec(ctx, `UPDATE record_shares SET revoked_at = now() WHERE id = $1`, id); err != nil {
			t.Fatal(err)
		}
		if version() <= mid {
			t.Fatal("ending a share did not bump the catalog version; the gate would keep allowing it")
		}
	})
}

// authorize_explain names the share that let the call through.
func TestExplainNamesTheShare(t *testing.T) {
	inRolledBackTx(t, func(ctx context.Context, tx pgx.Tx) {
		w := buildWorld(t, ctx, tx)
		id := w.share(t, ctx, tx, "job_requisition/JR-0142", []string{w.perm}, "7 days")
		var allow bool
		var shareID string
		if err := tx.QueryRow(ctx, `
			SELECT (e->>'allow')::bool, e->'shares'->0->>'share_id'
			  FROM authorize_explain($1, $2, $3, '{"_record":"job_requisition/JR-0142"}'::jsonb) e`,
			w.recipient, w.tenant, w.perm).Scan(&allow, &shareID); err != nil {
			t.Fatalf("explain: %v", err)
		}
		if !allow || shareID != id {
			t.Fatalf("explain allow=%v share=%q want %q", allow, shareID, id)
		}
	})
}
