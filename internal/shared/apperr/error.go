package apperr

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Error is a stable machine-readable failure. Code values are the public
// vocabulary from docs/api.md — they appear verbatim in the HTTP envelope and
// in the connect ErrorInfo detail, so changing one is an API break.
type Error struct {
	Code    string
	Message string
	Kind    Kind
	// Details carries machine-readable context (e.g. failing_axis). Never put
	// secrets or internal identifiers a caller should not learn here.
	Details map[string]string
	wrapped error
}

// Error includes the details map. The wire already carries it (ErrorInfo on
// connect, the envelope on HTTP) — omitting it from the Go string meant logs
// said "invalid argument" while the caller was told exactly which field was
// wrong, and the operator reading the log was the one who needed it most.
// Keys are sorted so the same failure always reads the same way.
func (e *Error) Error() string {
	var b strings.Builder
	b.WriteString(e.Code)
	b.WriteString(": ")
	b.WriteString(e.Message)
	if len(e.Details) > 0 {
		keys := make([]string, 0, len(e.Details))
		for k := range e.Details {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteString(" (")
		for i, k := range keys {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "%s=%s", k, e.Details[k])
		}
		b.WriteString(")")
	}
	if e.wrapped != nil {
		fmt.Fprintf(&b, ": %v", e.wrapped)
	}
	return b.String()
}

func (e *Error) Unwrap() error { return e.wrapped }

// E builds a domain error.
func E(kind Kind, code, message string) *Error {
	return &Error{Code: code, Message: message, Kind: kind}
}

func (e *Error) Wrap(err error) *Error {
	c := *e
	c.wrapped = err
	return &c
}

func (e *Error) With(k, v string) *Error {
	c := *e
	c.Details = make(map[string]string, len(e.Details)+1)
	for dk, dv := range e.Details {
		c.Details[dk] = dv
	}
	c.Details[k] = v
	return &c
}

// AsError extracts a *Error, wrapping unknown errors as internal so no raw
// database or driver error text ever reaches a client.
func AsError(err error) *Error {
	var de *Error
	if errors.As(err, &de) {
		return de
	}
	return ErrInternal.Wrap(err)
}
