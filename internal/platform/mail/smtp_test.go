package mail

import (
	"bufio"
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

// fakeRelay speaks just enough SMTP to accept one message and hand back
// what it received, so the test checks the conversation and the envelope
// rather than trusting that net/smtp was called.
type fakeRelay struct {
	ln       net.Listener
	from, to string
	data     string
	saw      []string
}

func newFakeRelay(t *testing.T) *fakeRelay {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	r := &fakeRelay{ln: ln}
	go r.serve()
	t.Cleanup(func() { _ = ln.Close() })
	return r
}

func (r *fakeRelay) serve() {
	conn, err := r.ln.Accept()
	if err != nil {
		return
	}
	defer conn.Close()
	br := bufio.NewReader(conn)
	say := func(s string) { _, _ = conn.Write([]byte(s + "\r\n")) }
	say("220 fake ESMTP")
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		r.saw = append(r.saw, line)
		switch {
		case strings.HasPrefix(line, "EHLO"):
			say("250-fake")
			say("250 8BITMIME")
		case strings.HasPrefix(line, "MAIL FROM:"):
			r.from = line
			say("250 ok")
		case strings.HasPrefix(line, "RCPT TO:"):
			r.to = line
			say("250 ok")
		case line == "DATA":
			say("354 go")
			var b strings.Builder
			for {
				l, err := br.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" {
					break
				}
				b.WriteString(l)
			}
			r.data = b.String()
			say("250 queued")
		case line == "QUIT":
			say("221 bye")
			return
		default:
			say("250 ok")
		}
	}
}

func TestASendIsOneAcceptedMessageWithTheRightEnvelope(t *testing.T) {
	relay := newFakeRelay(t)
	host, port, _ := net.SplitHostPort(relay.ln.Addr().String())
	var p int
	for _, c := range port {
		p = p*10 + int(c-'0')
	}
	s, err := New(host, p, "", "", "anubis@example.test", SecurityNone)
	if err != nil {
		t.Fatal(err)
	}
	s.Timeout = 5 * time.Second

	err = s.Send(context.Background(), "dewi@example.test", "Reset your password — Impack",
		"Open this link:\n\n  https://id.example.test/p/impack/reset/complete?token=abc\n")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if !strings.Contains(relay.from, "anubis@example.test") || !strings.Contains(relay.to, "dewi@example.test") {
		t.Fatalf("envelope wrong: %q / %q", relay.from, relay.to)
	}
	for _, want := range []string{
		"From: anubis@example.test\r\n",
		"To: dewi@example.test\r\n",
		"Subject: =?utf-8?q?Reset_your_password_=E2=80=94_Impack?=\r\n",
		"Content-Type: text/plain; charset=utf-8\r\n",
		"token=3Dabc", // quoted-printable escapes '='
	} {
		if !strings.Contains(relay.data, want) {
			t.Errorf("message lacks %q:\n%s", want, relay.data)
		}
	}
}

// A relay that cannot upgrade must not receive the message in the clear.
func TestStartTLSIsRefusedNotDowngraded(t *testing.T) {
	relay := newFakeRelay(t)
	host, port, _ := net.SplitHostPort(relay.ln.Addr().String())
	var p int
	for _, c := range port {
		p = p*10 + int(c-'0')
	}
	s, err := New(host, p, "", "", "anubis@example.test", SecurityStartTLS)
	if err != nil {
		t.Fatal(err)
	}
	s.Timeout = 5 * time.Second
	if err := s.Send(context.Background(), "x@example.test", "s", "t"); err == nil {
		t.Fatal("sent in the clear to a relay without STARTTLS")
	}
	if relay.data != "" {
		t.Fatal("the message body reached the relay")
	}
}

func TestHalfConfiguredSenderIsRefusedAtConstruction(t *testing.T) {
	cases := []struct {
		name                   string
		host, user, pass, from string
		port                   int
		sec                    Security
	}{
		{"no host", "", "", "", "a@b", 587, SecurityStartTLS},
		{"no from", "h", "", "", "", 587, SecurityStartTLS},
		{"user without password", "h", "u", "", "a@b", 587, SecurityStartTLS},
		{"bad port", "h", "", "", "a@b", 0, SecurityStartTLS},
		{"unknown security", "h", "", "", "a@b", 587, "ssl"},
	}
	for _, c := range cases {
		if _, err := New(c.host, c.port, c.user, c.pass, c.from, c.sec); err == nil {
			t.Errorf("%s: accepted", c.name)
		}
	}
	if s, err := New("h", 587, "", "", "a@b", ""); err != nil || s.Security != SecurityStartTLS {
		t.Fatalf("empty security should default to starttls: %v %v", s, err)
	}
}
