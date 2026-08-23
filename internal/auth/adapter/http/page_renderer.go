package authhttp

import (
	"html/template"
	"net/http"

	"github.com/gsoultan/anubis/internal/tenancy/domain/pagecfg"
)

// PageRenderer turns a validated config into HTML.
//
// Two rules hold the whole design together. First, html/template escapes
// every value, and no field is ever marked template.HTML — a tenant admin is
// not a trusted author of markup that Anubis serves on its own origin, on the
// screen where users type passwords. Second, everything that lands in CSS is
// a TOKEN the config chose from a closed set (colours validated as #rgb, radius
// and font mapped to fixed strings), so a config cannot break out of a
// declaration.
//
// The layout choice picks a stylesheet, never a template supplied by the
// tenant: adding a layout is a code change, deliberately.
type PageRenderer struct {
	tmpl *template.Template
}

func NewPageRenderer() *PageRenderer {
	return &PageRenderer{tmpl: template.Must(template.New("page").Parse(pageTemplate))}
}

// PageView is everything a page needs to render. The flow fields are hidden
// inputs carried through the login POST; they are opaque here.
type PageView struct {
	Cfg  *pagecfg.Config
	Kind string

	// Sign-in flow state.
	Tenant      string
	Realm       string
	ClientID    string
	RedirectURI string
	State       string
	Challenge   string
	Method      string
	Nonce       string
	Error       string
	// Realms offered by the picker, when the page enables it.
	Realms []RealmChoice
	// RegistrationURL is empty unless the realm actually allows
	// self-registration: a page must not advertise a door that will not open.
	RegistrationURL string

	// Sign-out state.
	Confirm             bool
	ReturnURL           string
	AutoRedirectSeconds int
	SignedOut           bool
	LogoutCSRF          string
}

// RealmChoice is one option in the realm picker.
type RealmChoice struct {
	Code string
	Name string
}

func (r *PageRenderer) Render(w http.ResponseWriter, status int, v PageView) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// The password screen must never be cached, and must never be framed:
	// clickjacking a login form is how consent gets stolen.
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	// The stylesheet is inline and generated from tokens, so 'unsafe-inline'
	// for styles is required; scripts are forbidden outright because these
	// pages have none.
	w.Header().Set("Content-Security-Policy",
		"default-src 'none'; img-src https: data:; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'")
	w.WriteHeader(status)
	_ = r.tmpl.Execute(w, v)
}
