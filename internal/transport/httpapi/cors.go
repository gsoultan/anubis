package httpapi

import "net/http"

// corsMiddleware exists for DEV ONLY (console on :7447 talking to :7448
// directly). Production is same-origin behind the gateway; an empty origin
// disables this entirely — no wildcard, no reflection.
func corsMiddleware(origin string, next http.Handler) http.Handler {
	if origin == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") == origin {
			h := w.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Vary", "Origin")
			h.Set("Access-Control-Allow-Credentials", "true")
			if r.Method == http.MethodOptions {
				h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
				h.Set("Access-Control-Allow-Headers",
					"Authorization, Content-Type, Connect-Protocol-Version, Connect-Timeout-Ms, X-Request-Id")
				h.Set("Access-Control-Max-Age", "600")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
