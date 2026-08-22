package keyring

import "fmt"

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

func (r *Ring) ActiveAccess() (*Key, error) {
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
