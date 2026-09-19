package authrpc

import (
	"context"
	"time"

	"connectrpc.com/connect"

	anubisv1 "github.com/gsoultan/anubis/gen/go/anubis/v1"
	"github.com/gsoultan/anubis/gen/go/anubis/v1/anubisv1connect"
	apiconnect "github.com/gsoultan/anubis/internal/api/connect"
	tokenapp "github.com/gsoultan/anubis/internal/auth/app/token"
	authep "github.com/gsoultan/anubis/internal/auth/endpoint"
	"github.com/gsoultan/anubis/internal/platform/revocation"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
)

// TokenHandler implements anubisv1connect.TokenServiceHandler.
type TokenHandler struct {
	eps authep.TokenEndpoints
	// revocations is optional: nil means the stream is not offered, which is
	// what an instance without a gate manager serves.
	revocations *revocation.Broker
}

func NewTokenHandler(eps authep.TokenEndpoints, revocations *revocation.Broker) *TokenHandler {
	return &TokenHandler{eps: eps, revocations: revocations}
}

var _ anubisv1connect.TokenServiceHandler = (*TokenHandler)(nil)

// Introspect is service-auth only (api.md): most applications should verify
// offline; whoever calls this is trusted with session-state answers.
func (h *TokenHandler) Introspect(ctx context.Context, req *connect.Request[anubisv1.IntrospectRequest]) (*connect.Response[anubisv1.IntrospectResponse], error) {
	if p, ok := authctx.From(ctx); !ok || !p.Service {
		return nil, apiconnect.Err(ctx, apperr.ErrUnauthenticated)
	}
	out, err := h.eps.Introspect(ctx, req.Msg.Token)
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	r := out.(*tokenapp.IntrospectResult)
	return connect.NewResponse(&anubisv1.IntrospectResponse{
		Active: r.Active, Sub: r.Subject, Sid: r.Session, Tid: r.Tenant,
		Realm: r.Realm, Roles: r.Roles, Scopes: r.Scopes, Amr: r.AMR,
		Aud: r.Audience, Exp: r.Expires, AuthTime: r.AuthTime,
		Ial: int32(r.IAL), Epoch: int32(r.Epoch),
	}), nil
}

func (h *TokenHandler) Revoke(ctx context.Context, req *connect.Request[anubisv1.RevokeRequest]) (*connect.Response[anubisv1.RevokeResponse], error) {
	if _, err := h.eps.Revoke(ctx, authep.RevokeRequest{
		Token: req.Msg.Token, Hint: req.Msg.TokenTypeHint,
	}); err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.RevokeResponse{}), nil
}

// StreamRevocations pushes revocations to a service-authenticated caller.
//
// Service-auth only, like Introspect, and for the same reason: this is
// session state, and an identity's own token does not entitle it to watch
// everybody else's sessions end.
//
// The stream is a CACHE INVALIDATION and the contract says so. A consumer
// that disconnects misses whatever happened while it was away — the broker
// drops rather than queueing, because a slow reader must never hold up the
// gate's refresh path — so correctness has to come from short token
// lifetimes and from re-checking, never from having seen every event. The
// KIND_SYNCED message marks the point where the consumer is current, so
// "nothing has happened" is distinguishable from "not connected yet".
func (h *TokenHandler) StreamRevocations(
	ctx context.Context,
	req *connect.Request[anubisv1.StreamRevocationsRequest],
	stream *connect.ServerStream[anubisv1.StreamRevocationsResponse],
) error {
	p, ok := authctx.From(ctx)
	if !ok || !p.Service {
		return apiconnect.Err(ctx, apperr.ErrUnauthenticated)
	}
	if h.revocations == nil {
		return apiconnect.Err(ctx, apperr.ErrStreamUnavailable)
	}

	// A tenant-scoped credential watches its own tenant and nothing else.
	// Honouring the request field for one would let a service key read
	// another tenant's revocation traffic, which is a cross-tenant leak
	// dressed as a subscription.
	tenant := req.Msg.GetTenant()
	if p.TenantSlug != "" {
		tenant = p.TenantSlug
	}
	if tenant == "" {
		return apiconnect.Err(ctx, apperr.ErrInvalidArgument.With("tenant", "required"))
	}

	events, cancel := h.revocations.Subscribe(tenant)
	defer cancel()

	// Sent BEFORE any event and after the subscription exists, so there is
	// no window in which a revocation happens, the consumer is told it is
	// synced, and the event was never delivered.
	if err := stream.Send(&anubisv1.StreamRevocationsResponse{
		Kind:       anubisv1.StreamRevocationsResponse_KIND_SYNCED,
		Tenant:     tenant,
		ObservedAt: time.Now().Unix(),
	}); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, open := <-events:
			if !open {
				return nil
			}
			if err := stream.Send(revocationMessage(ev)); err != nil {
				// The consumer went away. Not an error worth logging as one.
				return nil
			}
		}
	}
}

func revocationMessage(ev revocation.Event) *anubisv1.StreamRevocationsResponse {
	m := &anubisv1.StreamRevocationsResponse{
		Tenant:     ev.TenantSlug,
		Sid:        ev.SessionID,
		Sub:        ev.IdentityID,
		Epoch:      int32(ev.Epoch),
		ObservedAt: ev.ObservedAt.Unix(),
	}
	switch ev.Kind {
	case revocation.KindSession:
		m.Kind = anubisv1.StreamRevocationsResponse_KIND_SESSION_REVOKED
	case revocation.KindEpoch:
		m.Kind = anubisv1.StreamRevocationsResponse_KIND_EPOCH_BUMPED
	default:
		m.Kind = anubisv1.StreamRevocationsResponse_KIND_UNSPECIFIED
	}
	return m
}
