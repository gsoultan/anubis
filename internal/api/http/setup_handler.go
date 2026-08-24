package apihttp

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/gsoultan/anubis/internal/shared/apperr"
)

// SetupRequest is an installation, as the installer's form sends it. Plain
// fields on purpose: this package is transport plumbing and must not know a
// bounded context's types.
type SetupRequest struct {
	Token string `json:"token"`

	DBHost     string `json:"db_host"`
	DBPort     int    `json:"db_port"`
	DBName     string `json:"db_name"`
	DBUser     string `json:"db_user"`
	DBPassword string `json:"db_password"`
	DBSSLMode  string `json:"db_sslmode"`

	OwnerUsername string `json:"owner_username"`
	OwnerEmail    string `json:"owner_email"`
	OwnerPassword string `json:"owner_password"`

	// The first tenant, optional.
	FirstTenantSlug string `json:"first_tenant_slug"`
	FirstTenantName string `json:"first_tenant_name"`
}

// SetupRunner performs an installation. Implemented by the composition root,
// which is the only place that may build a database from a form submission.
type SetupRunner interface {
	// Validate reports field-level problems, keyed by form field.
	Validate(in SetupRequest) map[string]string
	// ValidateDatabase reports only what is wrong with the connection, for
	// the test-connection step that runs before the rest of the form exists.
	ValidateDatabase(in SetupRequest) map[string]string
	// TestConnection reaches the database without changing anything.
	TestConnection(ctx context.Context, in SetupRequest) error
	// Install migrates, provisions and writes the config file — in that
	// order, so a failure never leaves a configured-looking installation.
	Install(ctx context.Context, in SetupRequest) error
}

// SetupHandler serves the first-run installer.
//
// It is the most dangerous endpoint an installation has: whoever completes it
// chooses the database this instance trusts and owns the platform afterwards.
// Two things guard it. It answers at all only while the installation is
// unconfigured, and it demands the one-time token printed to the server's
// console at boot — otherwise a fresh public deployment is a race that the
// operator does not always win.
type SetupHandler struct {
	tokenHash [32]byte
	runner    SetupRunner
	done      chan<- struct{}
	logger    *slog.Logger
}

func NewSetupHandler(token string, runner SetupRunner, done chan<- struct{}, logger *slog.Logger) *SetupHandler {
	return &SetupHandler{
		tokenHash: sha256.Sum256([]byte(token)),
		runner:    runner,
		done:      done,
		logger:    logger,
	}
}

// checkToken compares in constant time. Hashing first keeps the comparison a
// fixed length, so it cannot leak the token's size either.
func (h *SetupHandler) checkToken(given string) bool {
	got := sha256.Sum256([]byte(given))
	return subtle.ConstantTimeCompare(got[:], h.tokenHash[:]) == 1
}

func (h *SetupHandler) decode(w http.ResponseWriter, r *http.Request) (SetupRequest, bool) {
	var in SetupRequest
	// An installer body is small; a large one is not a form.
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		WriteError(w, r, apperr.ErrInvalidArgument.With("body", "could not be read as JSON"))
		return in, false
	}
	if !h.checkToken(in.Token) {
		// Deliberately says nothing about which part was wrong.
		h.logger.WarnContext(r.Context(), "setup attempted with a bad token", "ip", r.RemoteAddr)
		WriteError(w, r, apperr.ErrPermissionDenied.With("token", "not the setup token for this server"))
		return in, false
	}
	return in, true
}

// TestConnection answers POST /v1/setup/test-connection.
func (h *SetupHandler) TestConnection(w http.ResponseWriter, r *http.Request) {
	in, ok := h.decode(w, r)
	if !ok {
		return
	}
	if problems := h.runner.ValidateDatabase(in); len(problems) > 0 {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "fields": problems})
		return
	}
	if err := h.runner.TestConnection(r.Context(), in); err != nil {
		WriteJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// Install answers POST /v1/setup.
func (h *SetupHandler) Install(w http.ResponseWriter, r *http.Request) {
	in, ok := h.decode(w, r)
	if !ok {
		return
	}
	if problems := h.runner.Validate(in); len(problems) > 0 {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "fields": problems})
		return
	}
	if err := h.runner.Install(r.Context(), in); err != nil {
		h.logger.ErrorContext(r.Context(), "setup failed", "error", err)
		WriteJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
	// Told the browser first: the server is about to stop being an installer
	// and start being an API, and the response has to be on its way out
	// before that happens.
	select {
	case h.done <- struct{}{}:
	default:
	}
}
