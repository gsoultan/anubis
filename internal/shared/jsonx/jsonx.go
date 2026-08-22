// Package jsonx holds the tiny JSON helpers the application layer shares.
package jsonx

import "encoding/json"

// Must marshals v, returning an empty object rather than failing a business
// operation because an audit detail could not be encoded.
func Must(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
}
