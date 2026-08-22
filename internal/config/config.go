// Package config loads process configuration from ANUBIS_* environment
// variables (stdlib only; the shell scripts in scripts/ are the other half
// of this contract).
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
)

// Config is everything anubisd needs to boot.
type Config struct {
	// DatabaseURL: postgres://user:pass@host:port/db (ANUBIS_DB_URL).
	DatabaseURL string
	// Listen is the public listener, default ":7448" (ANUBIS_LISTEN).
	Listen string
	// DebugListen serves pprof/expvar on loopback; empty disables
	// (ANUBIS_DEBUG_LISTEN, e.g. "127.0.0.1:7450").
	DebugListen string
	// Issuer is the iss claim and key-discovery base (ANUBIS_ISSUER).
	Issuer string
	// Env: "dev" or "prod" (ANUBIS_ENV, default dev).
	Env string
	// MasterKey seals private keys at rest (ANUBIS_MASTER_KEY, base64url 32B).
	// Required in prod; dev derives an INSECURE constant and says so loudly.
	MasterKey []byte
	// AutoKeys provisions signing keys at startup when none exist (dev).
	AutoKeys bool
	// UIOrigin enables CORS for the console origin in dev
	// (ANUBIS_UI_ORIGIN, e.g. "http://localhost:7447"). Empty = same-origin only.
	UIOrigin string
}

func Load() (*Config, error) {
	c := &Config{
		DatabaseURL: os.Getenv("ANUBIS_DB_URL"),
		Listen:      envOr("ANUBIS_LISTEN", ":7448"),
		DebugListen: os.Getenv("ANUBIS_DEBUG_LISTEN"),
		Issuer:      envOr("ANUBIS_ISSUER", "http://localhost:7448"),
		Env:         envOr("ANUBIS_ENV", "dev"),
		UIOrigin:    os.Getenv("ANUBIS_UI_ORIGIN"),
	}
	if c.DatabaseURL == "" {
		return nil, errors.New("config: ANUBIS_DB_URL is required")
	}
	c.AutoKeys = c.Env != "prod" || os.Getenv("ANUBIS_AUTOKEYS") == "1"

	if raw := os.Getenv("ANUBIS_MASTER_KEY"); raw != "" {
		key, err := base64.RawURLEncoding.DecodeString(raw)
		if err != nil || len(key) != 32 {
			return nil, fmt.Errorf("config: ANUBIS_MASTER_KEY must be base64url of exactly 32 bytes")
		}
		c.MasterKey = key
	} else {
		if c.Env == "prod" {
			return nil, errors.New("config: ANUBIS_MASTER_KEY is required in prod")
		}
		// Dev-only deterministic key so restarts can unseal what they wrote.
		// 32 bytes, obviously not secret — never let this reach prod.
		c.MasterKey = []byte("anubis-dev-master-key-32-bytes!!")
	}
	return c, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
