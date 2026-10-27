package identitymail

import (
	"context"
	"log/slog"

	"github.com/gsoultan/anubis/internal/identity/domain/outbound"
)

// LogSender writes the message to the log instead of sending it. It is for
// a development machine with no relay, and the composition root refuses to
// build it in production: the body of a reset mail IS the reset link, and a
// log line is not a private channel.
type LogSender struct {
	logger *slog.Logger
}

func NewLogSender(logger *slog.Logger) *LogSender { return &LogSender{logger: logger} }

func (s *LogSender) Send(_ context.Context, m outbound.Mail) error {
	s.logger.Info("mail (not sent: no relay configured)",
		"to", m.To, "subject", m.Subject, "text", m.Text)
	return nil
}
