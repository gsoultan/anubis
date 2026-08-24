package controldomain

import (
	"strings"
	"unicode/utf8"
)

// MinOwnerPassword matches the floor the bootstrap CLI enforces. Setup runs
// before any realm exists, so there is no realm password policy to consult
// yet — this is the one place the rule has to be stated directly.
const MinOwnerPassword = 12

// SetupInput is everything the installer needs to bring an installation into
// existence: which database to use, and who owns the platform.
//
// It carries a database password in memory for exactly as long as it takes to
// connect, migrate and seal it into the config file. Nothing here is ever
// logged, and the transport that accepts it must be the one endpoint the
// server exposes before it has a database at all.
type SetupInput struct {
	// Token is the one-time value printed to the server's console at first
	// boot. Without it, whoever reaches the installer first chooses the
	// database this instance trusts.
	Token string

	DBHost     string
	DBPort     int
	DBName     string
	DBUser     string
	DBPassword string
	DBSSLMode  string

	OwnerUsername string
	OwnerEmail    string
	OwnerPassword string

	// The first tenant, optional. Setting one up here is a convenience; an
	// installation with no tenants yet is a perfectly valid thing to have.
	FirstTenantSlug string
	FirstTenantName string
}

// sslModes are the values Postgres accepts. Anything else is a typo that
// would otherwise surface as an opaque connection failure.
var sslModes = map[string]bool{
	"disable": true, "allow": true, "prefer": true,
	"require": true, "verify-ca": true, "verify-full": true,
}

// DatabaseProblems reports only what is wrong with the connection details.
//
// Separate from Problems because the installer offers a "test connection"
// step BEFORE it asks who the owner will be — validating the whole form there
// would report a missing password for an account nobody has typed yet.
func (in SetupInput) DatabaseProblems() map[string]string {
	bad := map[string]string{}
	if strings.TrimSpace(in.Token) == "" {
		bad["token"] = "required"
	}
	if strings.TrimSpace(in.DBHost) == "" {
		bad["db_host"] = "required"
	}
	if in.DBPort <= 0 || in.DBPort > 65535 {
		bad["db_port"] = "must be a port number"
	}
	if strings.TrimSpace(in.DBName) == "" {
		bad["db_name"] = "required"
	}
	if strings.TrimSpace(in.DBUser) == "" {
		bad["db_user"] = "required"
	}
	if in.DBSSLMode != "" && !sslModes[in.DBSSLMode] {
		bad["db_sslmode"] = "disable, allow, prefer, require, verify-ca or verify-full"
	}
	return bad
}

// Problems returns everything wrong with the input, keyed by field, so the
// installer can mark all of its fields at once rather than one per attempt.
func (in SetupInput) Problems() map[string]string {
	bad := in.DatabaseProblems()

	if strings.TrimSpace(in.OwnerUsername) == "" {
		bad["owner_username"] = "required"
	}
	if utf8.RuneCountInString(in.OwnerPassword) < MinOwnerPassword {
		bad["owner_password"] = "at least 12 characters"
	}

	// The first tenant is optional, but a half-filled pair is a typo, not a
	// choice — saying so beats silently ignoring one of the two fields.
	slug, name := strings.TrimSpace(in.FirstTenantSlug), strings.TrimSpace(in.FirstTenantName)
	switch {
	case slug != "" && !validSlug(slug):
		bad["first_tenant_slug"] = "lower-case letters, digits, dash or underscore"
	case slug == "" && name != "":
		bad["first_tenant_slug"] = "required when a name is given"
	case slug != "" && name == "":
		bad["first_tenant_name"] = "required when a slug is given"
	}
	return bad
}

// validSlug mirrors the tenants.slug CHECK in migration 0001. Rejecting here
// as well turns a constraint violation into a field-level message.
func validSlug(s string) bool {
	if len(s) < 2 || len(s) > 63 {
		return false
	}
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
		case (r == '-' || r == '_') && i > 0:
		default:
			return false
		}
	}
	return true
}
