package authhttp

import "net/http"

// Cookie policy.
//
// Production uses the `__Host-` prefix: it forces Secure, forbids Domain, and
// pins Path=/, which together mean a subdomain cannot plant or overwrite the
// session cookie. That prefix REQUIRES HTTPS — a browser silently refuses the
// cookie otherwise — so over plain HTTP the browser flows simply do not work.
//
// That is correct for production and useless for development, where the API
// runs on http://localhost. So outside prod, and only for a non-TLS request,
// the same cookies are issued without the prefix and without Secure. The
// guard is deliberately narrow: prod always gets the hardened form regardless
// of what the request looks like, because a proxy-terminated TLS deployment
// still reaches this handler over plain HTTP.
type cookiePolicy struct {
	prod bool
}

func (p cookiePolicy) hardened(r *http.Request) bool {
	return p.prod || r.TLS != nil
}

// name returns the cookie name for this request. base is the un-prefixed name.
func (p cookiePolicy) name(r *http.Request, base string) string {
	if p.hardened(r) {
		return "__Host-" + base
	}
	return base
}

// set writes a cookie with the strongest attributes the transport allows.
func (p cookiePolicy) set(w http.ResponseWriter, r *http.Request, base, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     p.name(r, base),
		Value:    value,
		Path:     "/",
		Secure:   p.hardened(r),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
	})
}

// clear expires a cookie under whichever name this request would have used.
func (p cookiePolicy) clear(w http.ResponseWriter, r *http.Request, base string) {
	p.set(w, r, base, "", -1)
}

// get reads a cookie, tolerating both names: a session started before a TLS
// change should not strand the user with a cookie nothing looks for.
func (p cookiePolicy) get(r *http.Request, base string) string {
	if c, err := r.Cookie(p.name(r, base)); err == nil && c.Value != "" {
		return c.Value
	}
	if c, err := r.Cookie(base); err == nil && c.Value != "" {
		return c.Value
	}
	if c, err := r.Cookie("__Host-" + base); err == nil {
		return c.Value
	}
	return ""
}

// Cookie base names; the prefix is decided per request by the policy.
const (
	ssoCookieBase  = "anubis_sso"
	logoutCSRFBase = "anubis_logout"
)
