package authzsvc

import authzshare "github.com/gsoultan/anubis/internal/authz/app/share"

type shareService struct {
	authzshare.ShareUsecase
}

func NewShareService(u authzshare.ShareUsecase) ShareService { return &shareService{ShareUsecase: u} }
