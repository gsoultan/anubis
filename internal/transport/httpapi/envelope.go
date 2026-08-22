package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/gsoultan/anubis/internal/authctx"
	"github.com/gsoultan/anubis/internal/domain"
)

// envelope is the api.md error shape — the same stable codes the connect
// transport carries as ErrorInfo.
type envelope struct {
	Error     string            `json:"error"`
	Message   string            `json:"message"`
	RequestID string            `json:"request_id"`
	Details   map[string]string `json:"details,omitempty"`
}

func writeErr(w http.ResponseWriter, r *http.Request, err error) {
	de := domain.AsError(err)
	status := statusFor(de.Kind)
	w.Header().Set("Content-Type", "application/json")
	if de.Kind == domain.KindRateLimited {
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

func statusFor(k domain.Kind) int {
	switch k {
	case domain.KindInvalidArgument:
		return http.StatusBadRequest
	case domain.KindUnauthenticated:
		return http.StatusUnauthorized
	case domain.KindPermissionDenied:
		return http.StatusForbidden
	case domain.KindNotFound:
		return http.StatusNotFound
	case domain.KindConflict:
		return http.StatusConflict
	case domain.KindRateLimited:
		return http.StatusTooManyRequests
	case domain.KindUnavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
