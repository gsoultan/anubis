package main

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"connectrpc.com/connect"

	"github.com/gsoultan/anubis/gen/go/anubis/v1/anubisv1connect"
	"github.com/gsoultan/anubis/internal/crypto/keyring"
	ep "github.com/gsoultan/anubis/internal/endpoint"
	"github.com/gsoultan/anubis/internal/repository"
	"github.com/gsoultan/anubis/internal/repository/postgres"
	"github.com/gsoultan/anubis/internal/service"
	"github.com/gsoultan/anubis/internal/transport/connectrpc"
	"github.com/gsoultan/anubis/internal/usecase"
)

// storeKeyRotator implements usecase.KeyRotator over the key repository with
// the process master key.
type storeKeyRotator struct {
	keys   repository.KeyRepository
	master []byte
}

func (r *storeKeyRotator) PrepareKey(ctx context.Context, purpose string) (*repository.KeyRecord, error) {
	now := time.Now()
	var k *keyring.Key
	var err error
	if purpose == keyring.PurposeLocal {
		k, err = keyring.GenerateLocalKey(now, keyLifetime)
	} else {
		purpose = keyring.PurposeAccess
		k, err = keyring.GenerateAccessKey(now, keyLifetime)
	}
	if err != nil {
		return nil, err
	}
	material := k.Secret
	if k.Purpose == keyring.PurposeAccess {
		material = k.Private.Seed()
	}
	sealed, err := keyring.SealSecret(r.master, k.Kid, material)
	if err != nil {
		return nil, err
	}
	rec := repository.KeyRecord{
		Kid: k.Kid, Alg: k.Alg, Status: keyring.StatusPending, Purpose: purpose,
		PublicKey: orEmptyBytes(k.Public), PrivateKeyEnc: sealed,
		NotBefore: k.NotBefore, NotAfter: k.NotAfter,
	}
	if err := r.keys.CreateKey(ctx, rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

// adminWiring bundles the values serve.go hands over.
type adminWiring struct {
	store   *postgres.Store
	auditor *postgres.ChainedAuditor
	master  []byte
	logger  *slog.Logger
}

func (w adminWiring) register(mux *http.ServeMux, opts connect.HandlerOption) {
	s := w.store
	f := ep.NewFactory(w.logger)

	identityUC := usecase.NewIdentityAdminInteractor(
		s, s, s, s, s, s, s, s, s, s, s, systemClock{}, w.auditor)
	scopeUC := usecase.NewScopeAdminInteractor(s, s, s, s, s, w.auditor)
	authzUC := usecase.NewAuthzAdminInteractor(s, s, s, s, s, s, s, s, w.auditor)
	tenantUC := usecase.NewTenantAdminInteractor(
		s, s, s, s, s, s, s, s, w.auditor, &storeKeyRotator{keys: s, master: w.master}, w.auditor)

	mux.Handle(anubisv1connect.NewIdentityAdminServiceHandler(
		connectrpc.NewIdentityAdminHandler(service.NewIdentityAdminService(identityUC), f), opts))
	mux.Handle(anubisv1connect.NewScopeAdminServiceHandler(
		connectrpc.NewScopeAdminHandler(service.NewScopeAdminService(scopeUC), f), opts))
	mux.Handle(anubisv1connect.NewAuthzAdminServiceHandler(
		connectrpc.NewAuthzAdminHandler(service.NewAuthzAdminService(authzUC), f), opts))
	mux.Handle(anubisv1connect.NewTenantAdminServiceHandler(
		connectrpc.NewTenantAdminHandler(service.NewTenantAdminService(tenantUC), f), opts))
}
