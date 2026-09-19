package anubis

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gsoultan/anubis/pkg/anubis/jws"
	"github.com/gsoultan/anubis/pkg/anubis/keys"
	"github.com/gsoultan/anubis/pkg/anubis/paseto"
)

// Verifier verifies v4.public access tokens offline. Zero I/O on the verify
// path except a bounded, rate-limited key refetch on unknown kid.
type Verifier struct {
	cfg   Config
	cache *keys.Cache
}

func NewVerifier(cfg Config) (*Verifier, error) {
	if cfg.Audience == "" {
		return nil, ErrNoAudience
	}
	if cfg.KeysURL == "" && cfg.StaticKeys == nil {
		return nil, errors.New("anubis: either KeysURL or StaticKeys is required")
	}
	if cfg.Leeway == 0 {
		cfg.Leeway = 60 * time.Second
	}
	if cfg.now == nil {
		cfg.now = time.Now
	}
	v := &Verifier{cfg: cfg}
	if cfg.KeysURL != "" {
		v.cache = keys.NewCache(cfg.KeysURL)
	}
	return v, nil
}

// Verify checks signature, expiry, nbf, issuer and audience, and returns the
// claims. It does NOT check epoch or session revocation — those need state
// only Anubis holds; use introspection when instant revocation matters.
// pasetoPrefix is the format marker a v4.public token must start with.
// Anything else is offered to the JWS codec, which pins EdDSA.
const pasetoPrefix = "v4.public."

func (v *Verifier) Verify(ctx context.Context, token string) (*Claims, error) {
	// Which CODEC, chosen by the format marker the token is required to
	// carry — never by an `alg` field inside it. Both codecs are Ed25519 and
	// nothing else, so this choice cannot downgrade anything; it is a
	// question of encoding, which is the only reason it is safe to read
	// before verifying.
	var (
		msg []byte
		kid string
	)
	if strings.HasPrefix(token, pasetoPrefix) {
		// The kid rides in the footer, which is authenticated by the
		// signature — but we must read it BEFORE verification to select the
		// key. That pre-verification read may only ever index the bounded
		// key map.
		_, _, footer, err := paseto.Parse(token)
		if err != nil {
			return nil, err
		}
		if len(footer) > 0 {
			var tf tokenFooter
			if err := json.Unmarshal(footer, &tf); err != nil {
				return nil, fmt.Errorf("anubis: token footer: %w", err)
			}
			kid = tf.Kid
		}
		pk, err := v.key(ctx, kid)
		if err != nil {
			return nil, err
		}
		if msg, _, err = paseto.Verify(pk, token, nil); err != nil {
			return nil, err
		}
	} else {
		// JWS: the kid is in the PROTECTED header, so reading it early is
		// the same bargain — untrusted until the signature checks out, and
		// only ever used to index the same bounded map.
		h, _, _, err := jws.Parse(token)
		if err != nil {
			return nil, err
		}
		kid = h.Kid
		pk, err := v.key(ctx, kid)
		if err != nil {
			return nil, err
		}
		if msg, _, err = jws.Verify(pk, token); err != nil {
			return nil, err
		}
	}
	claims, err := parseClaims(msg)
	if err != nil {
		return nil, err
	}
	if claims.Version != 0 && claims.Version != 1 {
		return nil, ErrTokenVersion
	}
	if err := claims.Validate(v.cfg.now(), v.cfg.Issuer, v.cfg.Audience, v.cfg.Leeway); err != nil {
		return nil, err
	}
	return claims, nil
}

func (v *Verifier) key(ctx context.Context, kid string) (ed25519.PublicKey, error) {
	if v.cfg.StaticKeys != nil {
		if pk, ok := v.cfg.StaticKeys.Get(kid); ok {
			return pk, nil
		}
		if v.cache == nil {
			return nil, ErrUnknownKid
		}
	}
	return v.cache.Get(ctx, kid)
}
