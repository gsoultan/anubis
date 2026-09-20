# Login has two doors and one authenticator

Anubis authenticates a password at two places, and they are not
interchangeable surfaces on one handler — they were, until 2026-09-20, two
independent implementations:

- `AuthService.Login` (Connect RPC) → `signin.loginInteractor`
- `POST /v1/login`, the hosted sign-in page → `authhttp.OIDCHandler.LoginForm`

Both now call **one `signin.PasswordAuthenticator`**, constructed once in
`cmd/anubisd/application.go` as `a.passwordAuth` and handed to both. It owns
everything that decides WHETHER a password gets in; each door owns only what
it then issues.

| Decided by the authenticator | Decided by the door |
| :--- | :--- |
| input shape + KDF burn (uniform timing) | rate limits (per-door budgets) |
| tenant → realm → identity → credential | CSRF (browser only) |
| `kdf.Verify`, rehash on stale params | token pair vs. cookie + auth code |
| `identity.CanAuthenticate()` | the MFA *challenge* mechanism |
| enrolled factors ∩ realm `allowed_factors` | rendering / error copy |
| `Realm.EnrolmentStanceFor` (the deadline) | the success audit event |
| every **deny** audit event | |

`Authenticate` returns a `Decision{Step, Tenant, Realm, Identity, Methods,
Missing, Deadline, Due, Err}` with `Step ∈ {StepDeny, StepFactor, StepEnrol,
StepAllow}`.

## Why it is built this way

Three security defects, all one shape — a property implemented twice and
enforced once:

1. The page had **no factor check at all** (fixed 5e046801). A stolen
   password skipped an enrolled authenticator by using the browser.
2. The page ignored **`factor_enrolment_deadline`**. `EnrolmentStanceFor`
   had exactly one caller in the tree. A realm in force refused API clients
   and admitted the same member through SSO.
3. A **failed browser sign-in was never audited** — the failure branch
   rendered and returned. Wrong passwords on the page humans use left no
   trace at all.

Fixing 1 and 2 individually would have left the next property to diverge.

**Non-obvious trap when merging two implementations: take the STRICTER of
the two, not the shorter one.** On a credential-lookup error the page
answered "a factor exists" (fail closed) and the interactor returned nil and
carried on to issue a session (fail open). The interactor's version was
shorter and was the wrong one to keep. `enrolledFactorKinds` now returns an
error and a failed lookup is `StepDeny`, pinned by
`TestAFactorLookupFailureRefusesRatherThanAdmits` (mutation-checked).

## What the browser still cannot do

`auth_pages.kind` permits `signin` and `signout` only — there is **no hosted
enrolment page**. So:

- Past the deadline: the API returns a 15-minute grant token to enrol with;
  the browser can only refuse and name the missing factor.
- Inside the grace period: the API returns the deadline and missing factors;
  the browser shows **nothing**, because that response redirects straight
  back to the application.

A browser-only member therefore meets an enrol-or-deny policy for the first
time on the day it refuses them. Drive those populations through an operator
until a `kind = 'enrol'` page exists. See `docs/enrolment-rollout.md`.

## Adjacent bug, same day

`NewOIDCHandler` took a `RealmAdminRepository` and the struct literal never
assigned it. Its only caller sits behind `cfg.Features.ShowRealmPicker`, off
by default — so a page with the picker on never returned (no response in
20s; 6 ms after the fix). An optional feature is where an unassigned
dependency hides. `TestSignInPageRendersTheRealmPicker` holds it.

See [[core]], [[page-resolution]], [[tenant-scoped-reads]].
