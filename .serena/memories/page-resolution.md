# Hosted page resolution — which door a person sees (2026-09-07..08)

A tenant publishes many sign-in and sign-out pages. Resolution is
**slug → application → population → default**, each step requiring
`status='active'`. Enforced by four indexes on `auth_pages`:

    auth_pages_one_default    (tenant_id, kind) WHERE is_default
    auth_pages_one_per_realm  (realm_id, kind)  WHERE realm_id IS NOT NULL
    auth_pages_one_per_app    (application_id, kind) WHERE application_id IS NOT NULL
    UNIQUE                    (tenant_id, kind, slug)

So: unlimited pages per kind, at most one per population and one per
application, exactly one default. `auth_pages_one_binding` forbids app AND
realm on the same row.

## Sign-out used to resolve differently from sign-in

`logout_handler.go` passed neither binding:

    cfg := h.resolvePage(r, tenant.ID, "signout", q.Get("page"), "")

collapsing sign-out to "explicit `page=` or the tenant default", while sign-in
used the full chain. A binding the console wrote and the schema indexed, that
the server ignored for half the pages it applied to — a partner got the
partner door on the way in and a generic one on the way out, with nothing
anywhere saying why. Fixed in PR #16; everything needed was already at hand,
since `performLogout` read the SSO cookie anyway and `SessionView` carries
`ApplicationID` and `RealmCode`.

The session is now resolved **once** and drives both the page and the
revocation, so the two halves of a sign-out cannot disagree about which
session they mean.

**Two asymmetries that look like bugs and are not:**

- `signoutBindings` **is** tenant-scoped — another tenant's cookie must not
  select this tenant's page.
- `sessionFromSSOCookie` deliberately **is not** — revocation must still end
  whatever session the cookie names even under a mismatched `?tenant=`.
  Filtering there would render "you are signed out" without signing anybody
  out.

## A sign-in page is a launcher, not a form

`ServePage` hands the flow to `/v1/authorize`, which needs an application to
sign in *to*. A sign-in page bound only to a population, or serving as the
tenant default, renders "This page is not linked to an application yet."
Sign-out always renders.

Sign-out has **two states with different copy**: the landing state uses
`confirm_heading`/`confirm_body`, the post-sign-out state uses
`copy.heading`/`copy.body`. A direct GET always shows the landing state
regardless of `behavior.confirm`, because a GET must never end a session —
`confirm` controls CSRF enforcement on the POST, not whether the page asks.

## The address a page is served at

`/p/{tenant}/{kind}/{slug}`, computed server-side as `AuthPage.url` because
only the server knows the issuer. It must be built from the tenant the
**caller is administering** (`X-Anubis-Tenant` → `Principal.TenantSlug`), not
`cfg.DefaultTenant` — `ServePage` resolves the tenant from the path, so a URL
naming the configured default 404s while the page it names works. Wrong in the
worst direction: it looks correct. The configured default remains the fallback
for a caller with no tenant on its principal.

See [[core]].
