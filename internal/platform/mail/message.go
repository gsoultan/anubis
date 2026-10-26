// Package mail speaks SMTP from the standard library. It sends plain-text
// messages and nothing else: the only mail Anubis has reason to send is a
// link somebody asked for, and a link does not need markup.
package mail

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"mime"
	"mime/quotedprintable"
	"strings"
	"time"
)

// Encode builds one RFC 5322 message. Quoted-printable for the body and
// Q-encoding for the subject, so a username with accents or a link with a
// long token survive any relay in between; CRLF throughout, because SMTP
// servers disagree about bare LF and the strict ones drop the mail.
func Encode(from, to, subject, text string, now time.Time, host string) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + to + "\r\n")
	b.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", subject) + "\r\n")
	b.WriteString("Date: " + now.UTC().Format(time.RFC1123Z) + "\r\n")
	id, err := messageID(host)
	if err != nil {
		return nil, err
	}
	b.WriteString("Message-ID: " + id + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("Content-Transfer-Encoding: quoted-printable\r\n")
	// Hints the receiving side that nobody reads replies, and that this is
	// transactional rather than bulk — which is what keeps it out of the
	// promotions tab.
	b.WriteString("Auto-Submitted: auto-generated\r\n")
	b.WriteString("\r\n")
	w := quotedprintable.NewWriter(&b)
	if _, err := w.Write([]byte(strings.ReplaceAll(text, "\r\n", "\n"))); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func messageID(host string) (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	if host == "" {
		host = "anubis"
	}
	return "<" + hex.EncodeToString(raw[:]) + "@" + host + ">", nil
}
