// Package authctx carries the authenticated principal through a request. It
// is the ONLY bridge between the authn interceptor (transport) and the
// usecases, so neither imports the other.
package authctx

import "context"

type key struct{}

func With(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, key{}, p)
}

func From(ctx context.Context) (*Principal, bool) {
	p, ok := ctx.Value(key{}).(*Principal)
	return p, ok
}

// RequestID travels separately: it exists for unauthenticated requests too.
type ridKey struct{}

func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ridKey{}, id)
}

func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(ridKey{}).(string)
	return id
}

// ClientIP is set by the transport from the peer address (never from
// spoofable headers unless explicitly configured behind a trusted proxy).
type ipKey struct{}

func WithClientIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, ipKey{}, ip)
}

func ClientIP(ctx context.Context) string {
	ip, _ := ctx.Value(ipKey{}).(string)
	return ip
}

// UserAgent for session records.
type uaKey struct{}

func WithUserAgent(ctx context.Context, ua string) context.Context {
	return context.WithValue(ctx, uaKey{}, ua)
}

func UserAgent(ctx context.Context) string {
	ua, _ := ctx.Value(uaKey{}).(string)
	return ua
}
