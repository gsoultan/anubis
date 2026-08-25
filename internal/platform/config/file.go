package config

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gsoultan/anubis/internal/platform/yamlmin"
)

// The installation config file. Its EXISTENCE is what says an installation
// has been set up: with no file, anubisd serves the installer and nothing
// else; with one, the installer is gone. That makes this file the single
// source of truth for "is this thing configured", which is why it is written
// last, only after the database it names has been migrated and provisioned.
const (
	// DefaultConfigFile is looked for beside the binary's working directory.
	DefaultConfigFile = "config.yaml"
	// DefaultKeyFile holds a generated master key when one is not supplied
	// through the environment.
	DefaultKeyFile = "anubis.key"
)

// FileConfig is the on-disk installation configuration.
//
// Deliberately flat. The file nests for the operator's benefit; a struct that
// mirrored the nesting would buy nothing but three more types to keep in step
// with the reader and the writer.
type FileConfig struct {
	DBHost     string
	DBPort     int
	DBName     string
	DBUser     string
	DBPassword string
	DBSSLMode  string

	Listen   string
	Issuer   string
	UIOrigin string
}

// ConfigPath is where the installation config lives: ANUBIS_CONFIG when set,
// else config.yaml in the working directory.
func ConfigPath() string {
	if p := os.Getenv("ANUBIS_CONFIG"); p != "" {
		return p
	}
	return DefaultConfigFile
}

// KeyPath is where a generated master key lives, beside the config.
func KeyPath() string {
	if p := os.Getenv("ANUBIS_KEY_FILE"); p != "" {
		return p
	}
	return filepath.Join(filepath.Dir(ConfigPath()), DefaultKeyFile)
}

// Configured reports whether this installation has somewhere to run: either
// the config file the installer writes, or a database URL in the environment.
//
// The second is not a loophole. A container handed ANUBIS_DB_URL by its
// orchestrator IS configured, and showing it an installer would be both wrong
// and dangerous — it would offer to re-point a running deployment at a
// different database.
//
// It asks the filesystem every time rather than caching: a stale "yes" keeps
// the installer shut on an install that has none, and a stale "no" reopens it
// on one that is already running.
func Configured() bool {
	if os.Getenv("ANUBIS_DB_URL") != "" {
		return true
	}
	_, err := os.Stat(ConfigPath())
	return err == nil
}

// LoadFile reads the installation config and opens its sealed password.
func LoadFile(path string, master []byte) (*FileConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	doc, err := yamlmin.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	c := &FileConfig{
		DBHost:    doc.Str("database", "host"),
		DBPort:    doc.Int(5432, "database", "port"),
		DBName:    doc.Str("database", "name"),
		DBUser:    doc.Str("database", "user"),
		DBSSLMode: doc.Str("database", "sslmode"),
		Listen:    doc.Str("server", "listen"),
		Issuer:    doc.Str("server", "issuer"),
		UIOrigin:  doc.Str("server", "ui_origin"),
	}
	if c.DBSSLMode == "" {
		c.DBSSLMode = "require"
	}
	pw := doc.Str("database", "password")
	if pw == "" {
		return nil, fmt.Errorf("%s: database.password is missing", path)
	}
	opened, err := Open(master, pw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	c.DBPassword = opened
	return c, nil
}

// Save writes the config, sealing the password under the master key.
//
// Written 0600 and via a temporary file in the same directory, so a crash
// mid-write cannot leave a half-written config that the installer would then
// treat as a configured installation.
func (c *FileConfig) Save(path string, master []byte) error {
	sealed, err := Seal(master, c.DBPassword)
	if err != nil {
		return err
	}
	body := c.render(sealed)

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".config-*.yaml")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.WriteString(body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// render emits the config file. Hand-formatted rather than produced by a
// general YAML encoder: this file is read and edited by operators, and the
// comments are most of its value.
func (c *FileConfig) render(sealedPassword string) string {
	var b strings.Builder
	b.WriteString("# Anubis installation configuration.\n")
	b.WriteString("#\n")
	b.WriteString("# Written by the installer. This file existing is what tells anubisd the\n")
	b.WriteString("# installation is set up — delete it and the installer opens again, which\n")
	b.WriteString("# means anyone who can reach the console could re-point this instance at a\n")
	b.WriteString("# database of their choosing. Treat it as a secret and keep it out of git.\n")
	b.WriteString("\ndatabase:\n")
	fmt.Fprintf(&b, "  host: %s\n", yamlScalar(c.DBHost))
	fmt.Fprintf(&b, "  port: %d\n", c.DBPort)
	fmt.Fprintf(&b, "  name: %s\n", yamlScalar(c.DBName))
	fmt.Fprintf(&b, "  user: %s\n", yamlScalar(c.DBUser))
	b.WriteString("  # Sealed with the master key (AES-256-GCM). Not readable without it.\n")
	fmt.Fprintf(&b, "  password: %s\n", yamlScalar(sealedPassword))
	fmt.Fprintf(&b, "  sslmode: %s\n", yamlScalar(c.DBSSLMode))
	b.WriteString("\nserver:\n")
	fmt.Fprintf(&b, "  listen: %s\n", yamlScalar(c.Listen))
	fmt.Fprintf(&b, "  issuer: %s\n", yamlScalar(c.Issuer))
	fmt.Fprintf(&b, "  ui_origin: %s\n", yamlScalar(c.UIOrigin))
	return b.String()
}

// yamlScalar quotes whenever a bare value could be misread — which for
// values like ":7448" and "enc:v1:..." is most of the time.
func yamlScalar(s string) string {
	if s == "" {
		return `""`
	}
	safe := true
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' ||
			r == '-' || r == '_' || r == '.' {
			continue
		}
		safe = false
		break
	}
	if safe {
		return s
	}
	esc := strings.ReplaceAll(s, `\`, `\\`)
	esc = strings.ReplaceAll(esc, `"`, `\"`)
	return `"` + esc + `"`
}

// DatabaseURL builds the DSN. The password is escaped rather than
// interpolated: one '@' or '/' in a generated password would otherwise
// silently produce a DSN pointing somewhere else entirely.
func (c *FileConfig) DatabaseURL() string {
	host := c.DBHost
	if c.DBPort > 0 {
		host = host + ":" + strconv.Itoa(c.DBPort)
	}
	u := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(c.DBUser, c.DBPassword),
		Host:   host,
		Path:   "/" + c.DBName,
	}
	q := url.Values{}
	if c.DBSSLMode != "" {
		q.Set("sslmode", c.DBSSLMode)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// MasterKey resolves the key that opens the config, preferring the
// environment so production never has key material on disk beside the data
// it protects. Falling back to a file is what lets a plain install work with
// no prior setup at all.
// MasterKeyConfigured reports whether a key SOURCE was named — the
// environment variable, or a readable key file. It distinguishes "no key was
// configured" (dev may proceed with its deterministic key) from "a key was
// configured and is wrong" (nobody may proceed).
func MasterKeyConfigured() bool {
	if os.Getenv("ANUBIS_MASTER_KEY") != "" {
		return true
	}
	_, err := os.Stat(KeyPath())
	return err == nil
}

func MasterKey() ([]byte, error) {
	if raw := os.Getenv("ANUBIS_MASTER_KEY"); raw != "" {
		key, err := base64.RawURLEncoding.DecodeString(raw)
		if err != nil || len(key) != 32 {
			return nil, errors.New("config: ANUBIS_MASTER_KEY must be 32 bytes, base64url")
		}
		return key, nil
	}
	raw, err := os.ReadFile(KeyPath())
	if err != nil {
		return nil, err
	}
	key, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("config: %s does not hold a 32-byte base64url key", KeyPath())
	}
	return key, nil
}

// EnsureMasterKey returns the master key, generating and writing one when
// neither the environment nor a key file supplies it. Reports whether the
// key was newly written, so the installer can tell the operator that a file
// they must now back up has appeared.
func EnsureMasterKey() (key []byte, generated bool, err error) {
	if key, err = MasterKey(); err == nil {
		return key, false, nil
	}
	key = make([]byte, 32)
	if _, err = rand.Read(key); err != nil {
		return nil, false, err
	}
	encoded := base64.RawURLEncoding.EncodeToString(key)
	if err = os.WriteFile(KeyPath(), []byte(encoded+"\n"), 0o600); err != nil {
		return nil, false, err
	}
	return key, true, nil
}
