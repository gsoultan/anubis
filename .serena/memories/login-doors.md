# Login has three doors, and one shared primitive

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

## What the browser can and cannot do

Both doors refuse an overdue member and both offer the same way out, but they
spend the grant differently. The API returns it and the caller drives
`BeginTOTP`/`ConfirmTOTP`; the hosted page drives those itself and keeps the
grant server-side behind an opaque single-use handle
(`one_time_tokens.kind = 'browser_enrol'`, migration 0050). The grant never
reaches the browser.

Deliberate, not rough edges:

- **A wrong code discards the key.** `ConfirmTOTP` spends the enrolment token
  *before* verifying the code, so a stolen grant buys one guess rather than
  thousands. The page issues a fresh key and says so.
- **No QR.** A QR encoder is a feature of its own under ADR-0002; the
  `otpauth://` link opens an authenticator on a phone, and the key is typed
  anywhere else.
- **`PageView.EnrolURI` is `template.URL`.** `html/template` rewrites
  `otpauth://` to `#ZgotmplZ` — the scheme is not on its allowlist — and the
  link renders dead with no error anywhere. See [[page-rendering]].
- **Enrolling is not signing in.** Recovery codes render above the ordinary
  form and the next attempt is challenged for the factor, which is also how
  the test proves the enrolment took.

**The grace period warns now too.** The page writes the session and cookie
FIRST and then renders the warning, so "set it up now" and "continue without
it" both lead somewhere and neither blocks — `openBrowserSession` exists
apart from `issueCode` for exactly this. Continue re-enters `/v1/authorize`,
which finds the cookie and issues the code, so PKCE and redirect_uri checks
are not copied anywhere. Enrolling from the warning does NOT re-ask for the
password: that member already holds a session, and charging extra for
complying early is how a rollout stalls.

**Still missing: a QR code.** Only that.

## The hosted second-factor step had never worked

Separate from the above and found the same day. The MFA branch of the form
carries `mfa_token` and a code and NO credentials, while `LoginForm` required
a valid password on every submit — so a browser typed a correct code and got
"Invalid username or password".

It passed CI for weeks because the test posted a username and password the
form does not contain. **Parse the rendered form and submit that**; see
[[tests-must-submit-what-the-ui-renders]]. `mfa_token` now stands for the
password as the template always claimed, the identity comes from the token's
payload rather than the form, and `CanAuthenticate` is re-checked at the
submit that issues the session because an identity can be blocked between the
two requests.

## The THIRD door: the control plane

`internal/control/app/platform_auth_interactor.go` is a separate
implementation again, and deliberately so — ADR-0011 keeps platform users in
their own table with nothing joining them to identities. It must NOT be
merged into `PasswordAuthenticator`. What had to be shared is the primitive:

- **Uniform timing is now inside `kdf.Verify`.** An absent or unparseable
  hash burns a full dummy derivation. It used to be each caller's job and the
  control plane did not do it: `PlatformUserByUsername` returns `""` for an
  unknown operator, so an operator who exists took **51.9 ms and one who does
  not took 0.35 ms**. Any future credential surface gets the property free.
- **`scripts/check/kdf-rehash.sh`** fails the build if a `kdf.Verify` discards
  `needsRehash`, allowing only a deliberate burn against `kdf.Dummy()`.

**Operator passwords cannot be rotated.** There is no self-service change and
no admin reset anywhere in the API — create, assign, set-status, and that is
all. A login rehash is therefore the only way an operator's KDF parameters
ever move, which is why discarding the flag mattered more here than on the
tenant plane. Remedy for a compromise is disable + recreate, losing the
account's assignments. Open product decision, recorded in `security.md`.

## Boot: fatal only for a key that must sign

`cmd/anubisd/keyload` (carved out of `cmd/anubisd` when the folder hit 11
files). An unsealable **active or pending** key is fatal — this instance can
issue nothing. A **retiring** one is logged at ERROR and loaded public-only:
it exists to keep verifying, `NewRing` never selects it to sign, and treating
it as fatal turned a botched `keys reseal` into a total outage. Anything still
sealed under it cannot be opened; for local keys that is MFA challenges and
enrolment grants, all minutes long.

## Adjacent bug, same day

`NewOIDCHandler` took a `RealmAdminRepository` and the struct literal never
assigned it. Its only caller sits behind `cfg.Features.ShowRealmPicker`, off
by default — so a page with the picker on never returned (no response in
20s; 6 ms after the fix). An optional feature is where an unassigned
dependency hides. `TestSignInPageRendersTheRealmPicker` holds it.

See [[core]], [[page-resolution]], [[tenant-scoped-reads]].
