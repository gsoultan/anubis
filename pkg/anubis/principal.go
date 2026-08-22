package anubis

import "context"

type ctxKey struct{}

// Principal is what a verified request carries.
type Principal struct {
	Claims *Claims
	Token  string
}

// FromContext returns the verified principal, if any.
func FromContext(ctx context.Context) (*Principal, bool) {
	p, ok := ctx.Value(ctxKey{}).(*Principal)
	return p, ok
}

// WithPrincipal is exported for tests and custom transports.
func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, ctxKey{}, p)
}
