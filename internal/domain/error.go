package domain

import (
	"errors"
	"fmt"
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

func (e *Error) Error() string {
	if e.wrapped != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.wrapped)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
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
