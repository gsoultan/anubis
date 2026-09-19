package authpg

import (
	"context"
	"time"

	"github.com/gsoultan/anubis/internal/auth/adapter/postgres/rgen/onetimetoken"
	authrquery "github.com/gsoultan/anubis/internal/auth/adapter/postgres/rquery"
	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/gsoultan/storm/runtime"
)

func (s *Repository) CreateOneTime(ctx context.Context, tenantID, kind string, hash []byte, payload []byte, expiresAt time.Time) (string, error) {
	tid, err := database.ParseUUID(tenantID)
	if err != nil {
		return "", database.MapErr(err)
	}
	n := onetimetoken.Create()
	n.SetTenantID(tid)
	n.SetKind(kind)
	n.SetTokenHash(hash)
	n.SetPayload(runtime.JSON(database.OrEmptyJSON(payload)))
	n.SetExpiresAt(expiresAt)
	row, err := n.Insert(ctx, s.ex(ctx))
	if err != nil {
		return "", database.MapErr(err)
	}
	return database.UUIDStr(row.ID), nil
}

// ConsumeOneTime is atomic single use: DELETE ... RETURNING has GETDEL
// semantics, so a second presentation finds no row. GET-then-DEL would leave a
// replay window exactly as wide as the round trip between them.
func (s *Repository) ConsumeOneTime(ctx context.Context, kind string, hash []byte) (string, []byte, error) {
	row, ok, err := authrquery.ConsumeOneTimeToken.One(ctx, s.ex(ctx), hash, kind)
	if err != nil {
		return "", nil, database.MapErr(err)
	}
	if !ok {
		return "", nil, database.NotFound()
	}
	return row.TenantID, []byte(row.Payload), nil
}
