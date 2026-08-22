package apihttp

import (
	"encoding/json"
	"net/http"

	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
)

// envelope is the api.md error shape — the same stable codes the connect
// transport carries as ErrorInfo.
type envelope struct {
	Error     string            `json:"error"`
	Message   string            `json:"message"`
	RequestID string            `json:"request_id"`
	Details   map[string]string `json:"details,omitempty"`
}

// WriteError renders the api.md error envelope: the same stable machine
// code the connect transport carries as ErrorInfo.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	de := apperr.AsError(err)
	status := statusFor(de.Kind)
	w.Header().Set("Content-Type", "application/json")
	if de.Kind == apperr.KindRateLimited {
		if ra, ok := de.Details["retry_after"]; ok {
			w.Header().Set("Retry-After", ra)
		}
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope{
		Error:     de.Code,
		Message:   de.Message,
		RequestID: authctx.RequestID(r.Context()),
		Details:   de.Details,
	})
}

func statusFor(k apperr.Kind) int {
	switch k {
	case apperr.KindInvalidArgument:
		return http.StatusBadRequest
	case apperr.KindUnauthenticated:
		return http.StatusUnauthorized
	case apperr.KindPermissionDenied:
		return http.StatusForbidden
	case apperr.KindNotFound:
		return http.StatusNotFound
	case apperr.KindConflict:
		return http.StatusConflict
	case apperr.KindRateLimited:
		return http.StatusTooManyRequests
	case apperr.KindUnavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// WriteJSON writes a JSON body with the given status.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
