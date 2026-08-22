package txm

import "context"

// TxManager runs fn inside one database transaction. Store methods called
// with the ctx fn receives participate in that transaction.
type TxManager interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}
