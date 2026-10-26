package mail

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"time"
)

// Security is how the connection to the relay is protected.
type Security string

const (
	// SecurityStartTLS opens plain and upgrades before anything else is
	// said; a relay that cannot upgrade is refused. The usual port is 587.
	SecurityStartTLS Security = "starttls"
	// SecurityImplicit is TLS from the first byte. The usual port is 465.
	SecurityImplicit Security = "implicit"
	// SecurityNone sends in the clear. Only for a relay on the same host or
	// a test; credentials are refused on it unless the host is loopback,
	// which net/smtp enforces for us.
	SecurityNone Security = "none"
)

// SMTP is a client for one relay. Zero-value fields are refused by New, so a
// half-configured sender fails at startup rather than on the first reset.
type SMTP struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	Security Security
	// Timeout bounds the whole conversation; a relay that hangs must not
	// hold a goroutine forever.
	Timeout time.Duration
	// Dial is replaced in tests with a listener on loopback.
	Dial func(ctx context.Context, addr string) (net.Conn, error)
}

// New validates the configuration once.
func New(host string, port int, username, password, from string, sec Security) (*SMTP, error) {
	switch {
	case host == "":
		return nil, errors.New("mail: SMTP host is required")
	case port <= 0 || port > 65535:
		return nil, fmt.Errorf("mail: SMTP port %d is out of range", port)
	case from == "":
		return nil, errors.New("mail: a From address is required")
	case (username == "") != (password == ""):
		return nil, errors.New("mail: SMTP username and password go together")
	}
	switch sec {
	case SecurityStartTLS, SecurityImplicit, SecurityNone:
	case "":
		sec = SecurityStartTLS
	default:
		return nil, fmt.Errorf("mail: unknown SMTP security %q (starttls, implicit or none)", sec)
	}
	return &SMTP{
		Host: host, Port: port, Username: username, Password: password,
		From: from, Security: sec, Timeout: 30 * time.Second,
	}, nil
}

// Send delivers one plain-text message and returns when the relay has
// accepted it. Acceptance by the relay is the only thing it can promise.
func (s *SMTP) Send(ctx context.Context, to, subject, text string) error {
	if s.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.Timeout)
		defer cancel()
	}
	addr := net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
	dial := s.Dial
	if dial == nil {
		dial = func(ctx context.Context, addr string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "tcp", addr)
		}
	}
	conn, err := dial(ctx, addr)
	if err != nil {
		return fmt.Errorf("mail: dial %s: %w", addr, err)
	}
	// The context deadline becomes the socket deadline: net/smtp itself
	// takes no context, and a relay that stops answering mid-DATA would
	// otherwise block past the timeout the caller chose.
	if d, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(d)
	}
	tlsCfg := &tls.Config{ServerName: s.Host, MinVersion: tls.VersionTLS12}
	if s.Security == SecurityImplicit {
		conn = tls.Client(conn, tlsCfg)
	}
	c, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("mail: greeting from %s: %w", addr, err)
	}
	defer c.Close()

	if s.Security == SecurityStartTLS {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return fmt.Errorf("mail: %s does not offer STARTTLS; refusing to send in the clear", addr)
		}
		if err := c.StartTLS(tlsCfg); err != nil {
			return fmt.Errorf("mail: STARTTLS with %s: %w", addr, err)
		}
	}
	if s.Username != "" {
		// PlainAuth refuses to send a password over a connection it cannot
		// see is encrypted, except to localhost. That refusal is the one we
		// want; it is not worked around here.
		if err := c.Auth(smtp.PlainAuth("", s.Username, s.Password, s.Host)); err != nil {
			return fmt.Errorf("mail: authenticate to %s: %w", addr, err)
		}
	}
	if err := c.Mail(s.From); err != nil {
		return fmt.Errorf("mail: MAIL FROM: %w", err)
	}
	if err := c.Rcpt(to); err != nil {
		return fmt.Errorf("mail: RCPT TO: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("mail: DATA: %w", err)
	}
	msg, err := Encode(s.From, to, subject, text, time.Now(), s.Host)
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("mail: body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("mail: relay refused the message: %w", err)
	}
	return c.Quit()
}
