package keyring

import (
	"fmt"
	"time"
)

// Ring is an immutable snapshot of the key set. Swap the whole ring to
// rotate; never mutate one in place.
type Ring struct {
	byKid        map[string]*Key
	activeAccess *Key
	activeLocal  *Key
}

func NewRing(keys []*Key) (*Ring, error) {
	if len(keys) > maxKeys {
		return nil, ErrTooManyKeys
	}
	r := &Ring{byKid: make(map[string]*Key, len(keys))}
	for _, k := range keys {
		if k.Status == StatusRetired {
			continue
		}
		if _, dup := r.byKid[k.Kid]; dup {
			return nil, fmt.Errorf("keyring: duplicate kid %q", k.Kid)
		}
		r.byKid[k.Kid] = k
		if k.Status == StatusActive {
			switch k.Purpose {
			case PurposeAccess:
				if r.activeAccess != nil {
					return nil, fmt.Errorf("keyring: two active access keys (%s, %s)", r.activeAccess.Kid, k.Kid)
				}
				r.activeAccess = k
			case PurposeLocal:
				if r.activeLocal != nil {
					return nil, fmt.Errorf("keyring: two active local keys (%s, %s)", r.activeLocal.Kid, k.Kid)
				}
				r.activeLocal = k
			}
		}
	}
	return r, nil
}

// Lookup is the verify-path probe: map read, zero I/O, unknown kid rejects.
func (r *Ring) Lookup(kid string) (*Key, error) {
	k, ok := r.byKid[kid]
	if !ok {
		return nil, ErrUnknownKid
	}
	return k, nil
}

// ActiveAccess returns the key to sign with, as of now.
//
// Signing with a key outside its published validity window mints tokens every
// conforming verifier is obliged to reject — and the SDKs do reject them. That
// failure would surface as "all tokens are suddenly invalid" on whatever day
// the window closed, far from the cause. Failing here instead turns it into a
// loud, immediate error at the one place that can still be fixed by promoting
// a prepared key.
//
// This is deliberately the obvious name: a new signing path that reaches for
// ActiveAccess gets the check. Callers that only want to know whether a key is
// configured want ActiveAccessPresent.
func (r *Ring) ActiveAccess() (*Key, error) {
	return r.ActiveAccessAt(time.Now())
}

// ActiveAccessAt is ActiveAccess against a supplied clock, for callers that
// already hold one.
func (r *Ring) ActiveAccessAt(now time.Time) (*Key, error) {
	k, err := r.ActiveAccessPresent()
	if err != nil {
		return nil, err
	}
	if !k.InWindow(now) {
		return nil, fmt.Errorf("%w: kid %s is valid %s..%s", ErrKeyOutOfWindow,
			k.Kid, k.NotBefore.Format(time.RFC3339), k.NotAfter.Format(time.RFC3339))
	}
	return k, nil
}

// ActiveAccessPresent reports the active access key without judging its
// validity window. For callers asking "is a key configured at all?" — startup
// provisioning and operator-facing reporting, which must keep seeing a key
// precisely when it has expired.
func (r *Ring) ActiveAccessPresent() (*Key, error) {
	if r.activeAccess == nil || r.activeAccess.Private == nil {
		return nil, ErrNoActiveKey
	}
	return r.activeAccess, nil
}

func (r *Ring) ActiveLocal() (*Key, error) {
	if r.activeLocal == nil || len(r.activeLocal.Secret) == 0 {
		return nil, ErrNoActiveKey
	}
	return r.activeLocal, nil
}

// All returns the loaded keys (publication filters by purpose/status).
func (r *Ring) All() []*Key {
	out := make([]*Key, 0, len(r.byKid))
	for _, k := range r.byKid {
		out = append(out, k)
	}
	return out
}
