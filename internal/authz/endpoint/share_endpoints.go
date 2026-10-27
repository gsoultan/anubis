package authzep

import (
	"context"
	"log/slog"

	"github.com/go-kit/kit/endpoint"
	authzshare "github.com/gsoultan/anubis/internal/authz/app/share"
	authzsvc "github.com/gsoultan/anubis/internal/authz/service"
	"github.com/gsoultan/anubis/internal/platform/mw"
)

// ShareEndpoints is the wired endpoint set for ShareService.
type ShareEndpoints struct {
	Share          endpoint.Endpoint
	Unshare        endpoint.Endpoint
	SharesOnRecord endpoint.Endpoint
	SharedWithMe   endpoint.Endpoint
	CreateLink     endpoint.Endpoint
}

// sharesOnRecordReq carries the three arguments of one list call.
type sharesOnRecordReq struct {
	Kind, Ref string
	Scopes    map[string]string
}

func WrapSharesOnRecord(kind, ref string, scopes map[string]string) any {
	return sharesOnRecordReq{Kind: kind, Ref: ref, Scopes: scopes}
}

func NewShareEndpoints(svc authzsvc.ShareService, logger *slog.Logger) ShareEndpoints {
	return ShareEndpoints{
		Share: mw.Chain("share.create", logger)(func(ctx context.Context, req any) (any, error) {
			return svc.Share(ctx, req.(authzshare.ShareInput))
		}),
		Unshare: mw.Chain("share.revoke", logger)(func(ctx context.Context, req any) (any, error) {
			r := req.([2]string)
			return nil, svc.Unshare(ctx, r[0], r[1])
		}),
		SharesOnRecord: mw.Chain("share.list_record", logger)(func(ctx context.Context, req any) (any, error) {
			r := req.(sharesOnRecordReq)
			return svc.SharesOnRecord(ctx, r.Kind, r.Ref, r.Scopes)
		}),
		SharedWithMe: mw.Chain("share.list_mine", logger)(func(ctx context.Context, _ any) (any, error) {
			return svc.SharedWithMe(ctx)
		}),
		CreateLink: mw.Chain("share.link_create", logger)(func(ctx context.Context, req any) (any, error) {
			return svc.CreateAccessLink(ctx, req.(string))
		}),
	}
}
