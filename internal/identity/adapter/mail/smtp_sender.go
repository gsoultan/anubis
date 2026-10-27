// Package identitymail delivers the identity context's messages. Two
// senders: a relay for an installation that has one, and the log for a
// development machine that does not.
package identitymail

import (
	"context"

	"github.com/gsoultan/anubis/internal/identity/domain/outbound"
	"github.com/gsoultan/anubis/internal/platform/mail"
)

// SMTPSender hands messages to a relay.
type SMTPSender struct {
	relay *mail.SMTP
}

func NewSMTPSender(relay *mail.SMTP) *SMTPSender { return &SMTPSender{relay: relay} }

func (s *SMTPSender) Send(ctx context.Context, m outbound.Mail) error {
	return s.relay.Send(ctx, m.To, m.Subject, m.Text)
}
