package anubis

// KeysDocument is the shape served at /.well-known/anubis-keys.json.
type KeysDocument struct {
	Issuer string     `json:"issuer"`
	Keys   []KeyEntry `json:"keys"`
}
