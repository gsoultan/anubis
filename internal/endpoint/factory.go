package endpoint

import (
	"context"
	"log/slog"

	"github.com/go-kit/kit/endpoint"
)

// Factory lets transports run one-off admin operations through the SAME
// middleware chain as the wired endpoint sets, without declaring sixty
// endpoint fields. The chain per admin call is constructed once per
// invocation — admin traffic, not the hot path.
type Factory struct {
	Logger *slog.Logger
}

func NewFactory(logger *slog.Logger) Factory { return Factory{Logger: logger} }

// Do executes fn as a named endpoint under the standard chain.
func (f Factory) Do(ctx context.Context, name string, fn func(ctx context.Context) (any, error)) (any, error) {
	ep := Chain(name, f.Logger)(func(ctx context.Context, _ any) (any, error) {
		return fn(ctx)
	})
	return ep(ctx, nil)
}

var _ endpoint.Endpoint = (endpoint.Endpoint)(nil)
