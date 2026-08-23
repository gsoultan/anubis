// Package pagecfg is the sign-in and sign-out page builder's data model.
//
// The whole design is one decision: a tenant admin configures a CONSTRAINED
// TOKEN SET, never markup. Anubis serves these pages from its own origin, on
// the one screen where users type their password — so "let admins paste HTML"
// would hand every tenant admin a stored-XSS primitive against every user of
// that tenant, and a broken template would take down sign-in entirely.
//
// Everything here is therefore an enum, a bounded string, a validated colour
// or an https URL. Parse never fails on unknown fields (a newer console may
// send more than this build understands) but it does fail on invalid values:
// silently dropping a bad colour would show the admin a preview that does not
// match what users see.
package pagecfg

import (
	"encoding/json"

	"github.com/gsoultan/anubis/internal/shared/apperr"
)

// Kind distinguishes the two page types; each ignores the other's section.
type Kind string

const (
	KindSignin  Kind = "signin"
	KindSignout Kind = "signout"
)

// Parse decodes and validates a stored or submitted config, filling defaults
// so an empty object renders a complete, usable page.
func Parse(kind Kind, raw []byte) (*Config, error) {
	c := &Config{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, c); err != nil {
			return nil, apperr.ErrInvalidArgument.With("config", "invalid JSON")
		}
	}
	c.applyDefaults(kind)
	if err := c.Validate(kind); err != nil {
		return nil, err
	}
	return c, nil
}

// Marshal renders a validated config back to storage form.
func (c *Config) Marshal() ([]byte, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}
	return b, nil
}
