package authzsvc

import authzshare "github.com/gsoultan/anubis/internal/authz/app/share"

// ShareService is the record-sharing surface (proto ShareService).
type ShareService interface {
	authzshare.ShareUsecase
}
