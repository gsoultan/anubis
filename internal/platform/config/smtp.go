package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// SMTPConfig is the relay Anubis sends through (ANUBIS_SMTP_*). Host empty
// means no relay: in development mail goes to the log, in production the
// features that need it refuse.
//
//	ANUBIS_SMTP_HOST      relay hostname; unset = no mail
//	ANUBIS_SMTP_PORT      default 587
//	ANUBIS_SMTP_USERNAME  with ANUBIS_SMTP_PASSWORD or ANUBIS_SMTP_PASSWORD_FILE
//	ANUBIS_SMTP_FROM      the sender address, required when a host is set
//	ANUBIS_SMTP_SECURITY  starttls (default) | implicit | none
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	Security string
}

// Configured reports whether a relay was named at all.
func (c SMTPConfig) Configured() bool { return c.Host != "" }

func loadSMTP() (SMTPConfig, error) {
	c := SMTPConfig{
		Host:     os.Getenv("ANUBIS_SMTP_HOST"),
		Port:     envInt("ANUBIS_SMTP_PORT", 587),
		Username: os.Getenv("ANUBIS_SMTP_USERNAME"),
		Password: os.Getenv("ANUBIS_SMTP_PASSWORD"),
		From:     os.Getenv("ANUBIS_SMTP_FROM"),
		Security: strings.ToLower(envOr("ANUBIS_SMTP_SECURITY", "starttls")),
	}
	// The same rule as the master key: a file is the better home for a
	// secret, and a file that is named but unreadable is a hard failure.
	if path := os.Getenv("ANUBIS_SMTP_PASSWORD_FILE"); path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			return c, fmt.Errorf("config: ANUBIS_SMTP_PASSWORD_FILE: %w", err)
		}
		c.Password = strings.TrimSpace(string(raw))
	}
	if !c.Configured() {
		return c, nil
	}
	if c.From == "" {
		return c, errors.New("config: ANUBIS_SMTP_FROM is required when ANUBIS_SMTP_HOST is set")
	}
	if c.Port <= 0 || c.Port > 65535 {
		return c, errors.New("config: ANUBIS_SMTP_PORT is out of range: " + strconv.Itoa(c.Port))
	}
	switch c.Security {
	case "starttls", "implicit", "none":
	default:
		return c, errors.New("config: ANUBIS_SMTP_SECURITY must be starttls, implicit or none")
	}
	return c, nil
}
