// Package install is the first-run installer: the only part of anubisd that
// runs before there is a database. It lives beside the composition root
// because building a database from a form submission is a wiring decision,
// and no bounded context should be able to make it.
package install

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	apihttp "github.com/gsoultan/anubis/internal/api/http"
)

// runInstaller serves the first-run installer and returns once an
// installation has been written.
//
// It runs BEFORE any database exists, so there is no pool, no keyring, no
// snapshot and no jobs — only the console and the two setup endpoints. When
// setup succeeds this returns nil and runServe carries on to boot the real
// API on the same port. Stopping one server and starting another takes
// milliseconds on a machine nobody is using yet, and it is far easier to
// reason about than swapping a live handler.
func Run(ctx context.Context, logger *slog.Logger) error {
	token, err := setupToken()
	if err != nil {
		return err
	}

	listen := envOrDefault("ANUBIS_LISTEN", ":7448")
	issuer := envOrDefault("ANUBIS_ISSUER", "http://localhost:7448")
	uiOrigin := envOrDefault("ANUBIS_UI_ORIGIN", "http://localhost:7447")

	done := make(chan struct{}, 1)
	inst := &installer{listen: listen, issuer: issuer, uiOrigin: uiOrigin, logger: logger}
	setup := apihttp.NewSetupHandler(token, inst, done, logger)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/setup/test-connection", setup.TestConnection)
	mux.HandleFunc("POST /v1/setup", setup.Install)
	// The console reads this to decide whether to show the installer. During
	// setup there is no database to ask, so the answer is fixed.
	mux.HandleFunc("GET /v1/console-config", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		apihttp.WriteJSON(w, http.StatusOK, map[string]any{
			"tenant": "", "issuer": issuer, "setup_required": true,
		})
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		apihttp.WriteJSON(w, http.StatusOK, map[string]any{"status": "setup"})
	})

	srv := &http.Server{
		Addr:              listen,
		Handler:           apihttp.CORS(uiOrigin, mux),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		MaxHeaderBytes:    1 << 16,
	}

	announce(logger, uiOrigin, token)

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	select {
	case <-ctx.Done():
		_ = srv.Close()
		return ctx.Err()
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-done:
		// Give the success response its moment on the wire before the
		// listener goes away under it.
		time.Sleep(250 * time.Millisecond)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		logger.Info("setup complete — starting the API")
		return nil
	}
}

// setupToken mints the one-time value that guards the installer. Held only in
// memory: there is no database yet, and setup happens within one process
// lifetime. A restart prints a new one, which is the correct behaviour — a
// token that outlived the process it was printed for would be a standing
// invitation.
func setupToken() (string, error) {
	// An operator may supply one, which is how an unattended install passes
	// it in without reading logs.
	if t := os.Getenv("ANUBIS_SETUP_TOKEN"); t != "" {
		return t, nil
	}
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// announce prints the token where an operator can find it. Deliberately loud
// and deliberately on stdout as well as the log: half the point of the token
// is that reaching it requires access to this process's output.
func announce(logger *slog.Logger, uiOrigin, token string) {
	fmt.Fprintf(os.Stdout, `
  Anubis is not set up yet.

  Open       %s/setup
  Setup key  %s

  The key is required to complete setup and is not stored anywhere.
  Restarting prints a new one.

`, uiOrigin, token)
	logger.Info("installer listening — setup required")
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
