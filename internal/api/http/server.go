package apihttp

import (
	"expvar"
	"log/slog"
	"net/http"
	"net/http/pprof"
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

// HTTPServer builds the http.Server with native unencrypted HTTP/2 enabled
// so gRPC works over cleartext in dev (Go 1.24+ Protocols knob; no x/net).
func (s *Server) HTTPServer(addr string) *http.Server {
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetHTTP2(true)
	protocols.SetUnencryptedHTTP2(true)

	return &http.Server{
		Addr:              addr,
		Handler:           s.mux,
		Protocols:         protocols,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
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
