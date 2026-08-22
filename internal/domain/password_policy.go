package domain

import "encoding/json"

// PasswordPolicy is stored per realm as jsonb. Only length rules: composition
// rules (mandatory symbols etc.) are NIST-discouraged and push users toward
// predictable patterns.
type PasswordPolicy struct {
	MinLength int `json:"min_length"`
}

func ParsePasswordPolicy(raw []byte) PasswordPolicy {
	p := PasswordPolicy{}
	_ = json.Unmarshal(raw, &p) // absent/garbled policy falls back to defaults
	if p.MinLength <= 0 {
		p.MinLength = 12
	}
	return p
}

func (p PasswordPolicy) Check(password string) error {
	if len(password) < p.MinLength {
		return ErrPasswordPolicy.With("min_length", itoa(p.MinLength))
	}
	if len(password) > 512 { // absurd input is a KDF DoS vector
		return ErrPasswordPolicy.With("max_length", "512")
	}
	return nil
}
