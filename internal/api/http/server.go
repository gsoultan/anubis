package apihttp

import (
	"context"
	"expvar"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"strings"
	"time"
)

// Server assembles both surfaces onto one listener: the connect mux
// (mounted under its rpc paths) and the protocol-shaped stdlib routes.
type Server struct {
	mux    *http.ServeMux
	logger *slog.Logger
}

// NewServer mounts the transport-level routes only: health probes and the
// connect mux. Everything protocol-shaped (OIDC, key discovery, the gate)
// belongs to a bounded context and registers itself through Handle —
// otherwise this package would have to import every context and the
// dependency arrows would point the wrong way.
func NewServer(logger *slog.Logger, uiOrigin string, connectMux http.Handler,
	health *HealthHandler) *Server {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", health.Healthz)
	mux.HandleFunc("GET /readyz", health.Readyz)

	// Everything else falls through to connect (its handlers 404 unknown
	// paths themselves).
	mux.Handle("/", corsMiddleware(uiOrigin, connectMux))

	return &Server{mux: mux, logger: logger}
}

// Handle adds protocol-shaped routes (OIDC, gate) from other packages.
func (s *Server) Handle(pattern string, h http.Handler) {
	s.mux.Handle(pattern, h)
}

func (s *Server) HandleFunc(pattern string, h http.HandlerFunc) {
	s.mux.HandleFunc(pattern, h)
}

// Limits are the transport-level budgets. Every one of them exists because
// its absence is a denial-of-service primitive: a slow header write, an
// endless body, a connection that never closes.
type Limits struct {
	RequestTimeout  time.Duration
	MaxRequestBytes int64
}

// HTTPServer builds the http.Server with native unencrypted HTTP/2 enabled
// so gRPC works over cleartext in dev (Go 1.24+ Protocols knob; no x/net).
//
// WriteTimeout is deliberately NOT set: it would cut long-lived streams
// (revocation events, gRPC), so per-request deadlines carry that duty
// instead — see withLimits.
func (s *Server) HTTPServer(addr string, lim Limits) *http.Server {
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetHTTP2(true)
	protocols.SetUnencryptedHTTP2(true)

	return &http.Server{
		Addr:              addr,
		Handler:           withLimits(s.mux, lim),
		Protocols:         protocols,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    1 << 16, // 64 KiB; anything larger is an attack
		ErrorLog:          slog.NewLogLogger(s.logger.Handler(), slog.LevelWarn),
	}
}

// withLimits bounds body size and request duration, and turns a panic in any
// handler into a 500 instead of a dropped connection.
func withLimits(next http.Handler, lim Limits) http.Handler {
	if lim.RequestTimeout <= 0 {
		lim.RequestTimeout = 30 * time.Second
	}
	if lim.MaxRequestBytes <= 0 {
		lim.MaxRequestBytes = 1 << 20
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic in http handler", "path", r.URL.Path, "panic", rec)
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
		}()
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, lim.MaxRequestBytes)
		}
		// Streaming RPCs manage their own lifetime; a deadline here would
		// truncate them.
		if isStreaming(r) {
			next.ServeHTTP(w, r)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), lim.RequestTimeout)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func isStreaming(r *http.Request) bool {
	ct := r.Header.Get("Content-Type")
	return strings.HasPrefix(ct, "application/grpc") ||
		strings.HasPrefix(ct, "application/connect+")
}

// DebugServer serves pprof and expvar on a loopback-only listener.
func DebugServer(addr string) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/debug/vars", expvar.Handler())
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	return &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
}
