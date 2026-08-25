package apihttp

import (
	"io/fs"
	"net/http"
	"path"
	"strings"

	consoleassets "github.com/gsoultan/anubis/ui"
)

// apiPrefixes never belong to the SPA: RPC, the protocol surface, hosted
// pages and key discovery. An unknown path under these must 404 as an API
// would, not render HTML — a JSON client probing /v1/nope should never
// parse an index page.
var apiPrefixes = [...]string{"/anubis.v1.", "/v1/", "/p/", "/.well-known/"}

// consoleHandler serves the embedded admin console from the API's own
// origin and hands everything API-shaped to next. Unknown GET paths fall
// back to index.html so the SPA router owns its routes on refresh.
func consoleHandler(next http.Handler) http.Handler {
	dist, err := fs.Sub(consoleassets.Dist, "dist")
	if err != nil {
		// The embed directive guarantees dist exists; failing quietly here
		// would serve RPC 404s for the whole console.
		panic("console assets: " + err.Error())
	}
	// The shell is served directly: http.FileServer 301s "/index.html" back
	// to "/", which turns the SPA fallback into a redirect loop.
	shell, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		panic("console assets: dist/index.html missing: " + err.Error())
	}
	files := http.FileServerFS(dist)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			next.ServeHTTP(w, r)
			return
		}
		for _, p := range apiPrefixes {
			if strings.HasPrefix(r.URL.Path, p) {
				next.ServeHTTP(w, r)
				return
			}
		}

		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" || name == "." {
			name = "index.html"
		}
		if _, err := fs.Stat(dist, name); err != nil {
			name = "index.html" // SPA fallback: /signin, /tenants, … on refresh
		}

		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		if name == "index.html" {
			// The shell must revalidate every load or a deploy strands users
			// on chunk names that no longer exist.
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Content-Security-Policy",
				"default-src 'self'; style-src 'self' 'unsafe-inline'; "+
					"img-src 'self' data:; connect-src 'self'; "+
					"frame-ancestors 'none'; base-uri 'none'")
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if r.Method == http.MethodHead {
				return
			}
			w.Write(shell)
			return
		}

		// Bundle artefacts are content-hashed; cache them forever.
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/" + name
		files.ServeHTTP(w, r2)
	})
}
