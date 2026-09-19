package tenancyrquery

import (
	"time"

	"github.com/gsoultan/storm"
	"github.com/gsoultan/storm/runtime"
)

// SigninPageRow is a tenant's default sign-in page configuration.
type SigninPageRow struct {
	TenantID  string
	Config    runtime.JSON
	UpdatedAt time.Time
}

// GetSigninPage reads signin_pages, which is a VIEW over auth_pages selecting
// the default sign-in page. storm models tables, so this stays SQL — and it
// should: the view is the compatibility surface that kept migrations/0018's
// shape working after 0024 moved pages into their own table.
var GetSigninPage = storm.SQL[SigninPageRow](`
SELECT tenant_id::text AS tenant_id, config, updated_at
FROM signin_pages WHERE tenant_id = $1`)

// PutSigninPage writes through the same surface.
var PutSigninPage = storm.SQLExec(`
INSERT INTO signin_pages (tenant_id, config, updated_at)
VALUES ($1, $2::jsonb, now())
ON CONFLICT (tenant_id) DO UPDATE
    SET config = EXCLUDED.config, updated_at = now()`)
