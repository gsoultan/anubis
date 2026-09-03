// Package config loads process configuration from ANUBIS_* environment
// variables (stdlib only; the shell scripts in scripts/ are the other half
// of this contract).
package config

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
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

	// TrustedProxies is a comma-separated list of CIDRs (or bare addresses)
	// whose X-Forwarded-For header is believed. Empty trusts nothing.
	TrustedProxies string
	// AutoKeys provisions signing keys at startup when none exist (dev).
	AutoKeys bool
	// InsecureMasterKey is true when no key was configured and the dev
	// constant below was substituted. Callers with a logger MUST say so —
	// see runServe. Nothing sealed under that key is confidential.
	InsecureMasterKey bool
	// UIOrigin enables CORS for the console origin in dev
	// (ANUBIS_UI_ORIGIN, e.g. "http://localhost:7447"). Empty = same-origin only.
	UIOrigin string

	// --- runtime limits -----------------------------------------------------

	// MaxConns caps the pgx pool (ANUBIS_DB_MAX_CONNS, default 4x GOMAXPROCS).
	// Sized to the DATABASE's capacity, not the app's ambition: past the
	// server's core count more connections make everything slower.
	MaxConns int32
	// StatementTimeout bounds every statement server-side
	// (ANUBIS_DB_STATEMENT_TIMEOUT, default 15s). A query that outlives it is
	// a bug, and an auth service must not hold a connection hostage.
	StatementTimeout time.Duration
	// RequestTimeout bounds every inbound request (ANUBIS_REQUEST_TIMEOUT,
	// default 30s) — except the gate, which has its own tighter budget.
	RequestTimeout time.Duration
	// MaxRequestBytes caps request bodies (ANUBIS_MAX_REQUEST_BYTES, 1 MiB).
	MaxRequestBytes int64
	// ShutdownGrace is how long in-flight requests may finish
	// (ANUBIS_SHUTDOWN_GRACE, default 20s).
	ShutdownGrace time.Duration
	// StrayTrustProxy is true when the retired ANUBIS_TRUST_PROXY is set.
	// It never did anything outside this struct — TrustedProxies is and was
	// the mechanism — but an operator who found it in the source and set it
	// believes proxy trust is configured when it is not, and then every
	// caller shares one rate-limit bucket. Warned about at startup rather
	// than ignored.
	StrayTrustProxy bool
	// SnapshotMaxAge is how stale the gate's snapshot may be before it fails
	// closed and readiness reports unhealthy (ANUBIS_SNAPSHOT_MAX_AGE, 5m).
	SnapshotMaxAge time.Duration

	// DefaultTenant is the tenant assumed when a request carries no tenant of
	// its own — single-tenant installs, and the example page URLs the console
	// displays (ANUBIS_DEFAULT_TENANT).
	DefaultTenant string
}

func Load() (*Config, error) {
	c := &Config{
		DatabaseURL: os.Getenv("ANUBIS_DB_URL"),
		Listen:      envOr("ANUBIS_LISTEN", ":7448"),
		DebugListen: os.Getenv("ANUBIS_DEBUG_LISTEN"),
		Issuer:      envOr("ANUBIS_ISSUER", "http://localhost:7448"),
		Env:         envOr("ANUBIS_ENV", "dev"),
		UIOrigin:    os.Getenv("ANUBIS_UI_ORIGIN"),

		MaxConns:         int32(envInt("ANUBIS_DB_MAX_CONNS", runtime.GOMAXPROCS(0)*4)),
		StatementTimeout: envDuration("ANUBIS_DB_STATEMENT_TIMEOUT", 15*time.Second),
		RequestTimeout:   envDuration("ANUBIS_REQUEST_TIMEOUT", 30*time.Second),
		MaxRequestBytes:  int64(envInt("ANUBIS_MAX_REQUEST_BYTES", 1<<20)),
		ShutdownGrace:    envDuration("ANUBIS_SHUTDOWN_GRACE", 20*time.Second),
		StrayTrustProxy:  os.Getenv("ANUBIS_TRUST_PROXY") != "",
		DefaultTenant:    envOr("ANUBIS_DEFAULT_TENANT", "impack"),
		SnapshotMaxAge:   envDuration("ANUBIS_SNAPSHOT_MAX_AGE", 5*time.Minute),
	}
	if c.MaxConns < 2 {
		c.MaxConns = 2
	}
	// Empty means trust nothing, which is right for a directly exposed
	// server and wrong the moment TLS is terminated in front of it.
	c.TrustedProxies = os.Getenv("ANUBIS_TRUSTED_PROXIES")
	// The installer writes config.yaml; the environment still wins, so a
	// container that sets ANUBIS_DB_URL keeps behaving exactly as before and
	// nothing here changes for deployments that never run the installer.
	if c.DatabaseURL == "" {
		if key, kerr := MasterKey(); kerr == nil {
			if fc, ferr := LoadFile(ConfigPath(), key); ferr == nil {
				c.DatabaseURL = fc.DatabaseURL()
				if os.Getenv("ANUBIS_LISTEN") == "" && fc.Listen != "" {
					c.Listen = fc.Listen
				}
				if os.Getenv("ANUBIS_ISSUER") == "" && fc.Issuer != "" {
					c.Issuer = fc.Issuer
				}
				if os.Getenv("ANUBIS_UI_ORIGIN") == "" && fc.UIOrigin != "" {
					c.UIOrigin = fc.UIOrigin
				}
			} else {
				return nil, ferr
			}
		}
	}

	if c.DatabaseURL == "" {
		return nil, errors.New("config: no database configured — set ANUBIS_DB_URL, or run the installer to write " + ConfigPath())
	}
	c.AutoKeys = c.Env != "prod" || os.Getenv("ANUBIS_AUTOKEYS") == "1"

	// The key may arrive in the environment OR in a file, and the file is the
	// better answer: an environment variable is readable from /proc by
	// anything running as the same user and survives in core dumps. Under
	// systemd the file is a CREDENTIAL (LoadCredential), mounted 0400 into a
	// private tmpfs that only the unit can see, at a path that changes every
	// boot — which is why ANUBIS_KEY_FILE has to be honoured here and not
	// just a fixed location.
	//
	// A source that is CONFIGURED but unreadable is a hard failure. Falling
	// back to the dev key because somebody fat-fingered a path would seal
	// production data under a key that is printed in this file.
	switch {
	case MasterKeyConfigured():
		key, err := MasterKey()
		if err != nil {
			return nil, fmt.Errorf("config: master key: %w", err)
		}
		c.MasterKey = key
	case c.Env == "prod":
		return nil, errors.New("config: no master key in prod — set ANUBIS_MASTER_KEY, " +
			"or ANUBIS_KEY_FILE pointing at one (systemd: LoadCredential)")
	default:
		// Dev-only deterministic key so restarts can unseal what they wrote.
		// 32 bytes, obviously not secret — never let this reach prod.
		c.MasterKey = []byte("anubis-dev-master-key-32-bytes!!")
		c.InsecureMasterKey = true
	}
	return c, nil

}

// PoolURL appends the server-side guards every connection must carry. Doing
// it here means no caller can forget them.
func (c *Config) PoolURL() string {
	sep := "?"
	if strings.Contains(c.DatabaseURL, "?") {
		sep = "&"
	}
	return fmt.Sprintf("%s%sapplication_name=anubisd&statement_timeout=%d",
		c.DatabaseURL, sep, c.StatementTimeout.Milliseconds())
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return def
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
