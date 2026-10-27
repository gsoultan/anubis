package identityport

import (
	"context"

	"github.com/gsoultan/anubis/internal/identity/domain/outbound"
)

// MailSender delivers a message to a person. Nil in the composition root
// means the installation cannot send mail, and the usecases that need it
// refuse rather than pretend.
type MailSender interface {
	Send(ctx context.Context, m outbound.Mail) error
}
