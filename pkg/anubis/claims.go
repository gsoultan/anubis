// Package anubis is the client SDK for services that consume Anubis tokens.
//
// It verifies access tokens OFFLINE — signature, expiry, audience — against
// published public keys, so Anubis is never in your request hot path. The
// package deliberately has zero dependencies outside the Go standard library:
// what every service embeds must not drag a dependency tree behind it.
//
// Typical use:
//
//	v, _ := anubis.NewVerifier(anubis.Config{
//	    Issuer:   "https://anubis.internal",
//	    Audience: "billing-api",
//	    KeysURL:  "https://anubis.internal/.well-known/anubis-keys.json",
//	})
//	mux.Handle("/api/", v.Middleware(apiHandler))
//
// The aud check is not optional. Without it a token minted for the HR
// application is accepted by the payments application — the classic confused
// deputy. This SDK refuses to run without an audience configured.
package anubis

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"
)

// Claims is the access-token claim set (docs/architecture.md). `scopes` is a
// map, never fixed fields: adding a scope axis must not change the token
// format.
type Claims struct {
	Issuer    string            `json:"iss"`
	Subject   string            `json:"sub"`
	Audience  []string          `json:"aud"`
	Expires   int64             `json:"exp"`
	IssuedAt  int64             `json:"iat"`
	NotBefore int64             `json:"nbf"`
	TokenID   string            `json:"jti"`
	Session   string            `json:"sid"`
	Tenant    string            `json:"tid"`
	Roles     []string          `json:"roles,omitempty"`
	Scope     string            `json:"scp,omitempty"`
	Scopes    map[string]string `json:"scopes,omitempty"`
	Realm     string            `json:"realm,omitempty"`
	IAL       int               `json:"ial,omitempty"`
	AMR       []string          `json:"amr,omitempty"`
	AuthTime  int64             `json:"auth_time,omitempty"`
	Epoch     int               `json:"epoch"`
	Version   int               `json:"ver"`
}

// Validate applies the time and identity checks that make a
// cryptographically valid token an *acceptable* one.
func (c *Claims) Validate(now time.Time, issuer, audience string, leeway time.Duration) error {
	n := now.Unix()
	l := int64(leeway / time.Second)
	if c.Expires != 0 && n > c.Expires+l {
		return ErrExpired
	}
	if c.NotBefore != 0 && n < c.NotBefore-l {
		return ErrNotYetValid
	}
	if issuer != "" && c.Issuer != issuer {
		return ErrIssuer
	}
	if audience == "" {
		return ErrNoAudience
	}
	for _, a := range c.Audience {
		if a == audience {
			return nil
		}
	}
	return ErrAudience
}

func parseClaims(message []byte) (*Claims, error) {
	var c Claims
	dec := json.NewDecoder(bytes.NewReader(message))
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("anubis: claims decode: %w", err)
	}
	return &c, nil
}
