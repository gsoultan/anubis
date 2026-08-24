package apihttp

import (
	"context"
	"log/slog"
	"net/http"
)

// SetupProbe answers the one question the console has before anybody signs
// in: has this installation been set up? Implemented by the control context;
// declared with plain types so this transport package stays free of any
// context's own.
type SetupProbe interface {
	// AnyPlatformUser reports whether an operator exists. Until one does,
	// there is nobody who could sign in and the installer is what to show.
	AnyPlatformUser(ctx context.Context) (bool, error)
}

// ConsoleHandler serves the console's discovery document.
//
// The console administers the PLATFORM, and platform usernames are globally
// unique because an operator belongs to no tenant — so sign-in needs a
// username and a password and nothing else. The only thing worth asking
// before that form renders is whether the installation exists yet.
//
// It reports a boolean and never a list: an unauthenticated endpoint that
// enumerated anything would be a roster for whoever curls it.
type ConsoleHandler struct {
	setup  SetupProbe
	issuer string
	logger *slog.Logger
}

func NewConsoleHandler(setup SetupProbe, issuer string, logger *slog.Logger) *ConsoleHandler {
	return &ConsoleHandler{setup: setup, issuer: issuer, logger: logger}
}

// consoleConfig is the discovery document. SetupRequired says no operator
// exists yet, which is the console's cue to show the installer rather than a
// sign-in form nobody could complete.
type consoleConfig struct {
	Issuer        string `json:"issuer"`
	SetupRequired bool   `json:"setup_required"`
}

// Config answers GET /v1/console-config, unauthenticated by necessity: it is
// read to decide what to render before any credential exists.
func (h *ConsoleHandler) Config(w http.ResponseWriter, r *http.Request) {
	ready, err := h.setup.AnyPlatformUser(r.Context())
	if err != nil {
		// A database that cannot answer is not the browser's problem to
		// solve, and it must not read as "not set up, please install me".
		h.logger.ErrorContext(r.Context(), "console config", "error", err)
		WriteError(w, r, err)
		return
	}
	// Not cached: setup flips this from one answer to the other exactly once,
	// and a console holding the stale value would offer to install over a
	// working installation.
	w.Header().Set("Cache-Control", "no-store")
	WriteJSON(w, http.StatusOK, consoleConfig{
		Issuer:        h.issuer,
		SetupRequired: !ready,
	})
}
