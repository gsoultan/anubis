package authhttp

import (
	"encoding/base64"
	"net/http"

	apihttp "github.com/gsoultan/anubis/internal/api/http"
	"github.com/gsoultan/anubis/internal/platform/crypto/keyring"
	"github.com/gsoultan/anubis/pkg/anubis/keys"
)

// WellKnownHandler publishes key discovery and the OIDC-shaped configuration
// document. Public keys are served with Cache-Control so consumer caches
// warm before a rotation flips the active key.
type WellKnownHandler struct {
	issuer string
	ring   *keyring.Manager
}

func NewWellKnownHandler(issuer string, ring *keyring.Manager) *WellKnownHandler {
	return &WellKnownHandler{issuer: issuer, ring: ring}
}

func (h *WellKnownHandler) Keys(w http.ResponseWriter, _ *http.Request) {
	doc := keys.Document{Issuer: h.issuer}
	for _, k := range h.ring.Ring().All() {
		if k.Purpose != keyring.PurposeAccess || len(k.Public) == 0 {
			continue
		}
		doc.Keys = append(doc.Keys, keys.Entry{
			Kid:       k.Kid,
			Alg:       "Ed25519",
			PublicKey: base64.RawURLEncoding.EncodeToString(k.Public),
			NotBefore: k.NotBefore.Unix(),
			NotAfter:  k.NotAfter.Unix(),
		})
	}
	w.Header().Set("Cache-Control", "public, max-age=300")
	apihttp.WriteJSON(w, http.StatusOK, doc)
}

func (h *WellKnownHandler) OpenIDConfiguration(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "public, max-age=3600")
	apihttp.WriteJSON(w, http.StatusOK, map[string]any{
		"issuer":                                h.issuer,
		"authorization_endpoint":                h.issuer + "/v1/authorize",
		"token_endpoint":                        h.issuer + "/v1/token",
		"jwks_uri":                              h.issuer + "/.well-known/anubis-keys.json",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"none", "client_secret_post"},
		"subject_types_supported":               []string{"public"},
	})
}
