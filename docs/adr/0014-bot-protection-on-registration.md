# ADR-0014 — Bot protection on public registration

## Status

Accepted. Applies while self-registration is enabled on a public realm.

## Context

A realm with `self_registration = true` accepts account creation from the
internet. Today the only defence is the rate limiter: bounded per IP, with
email verification required before the account is worth anything.

That bounds the *rate* of the damage; it does not stop a determined script.
With a modest pool of addresses, an attacker can create accounts steadily and
cheaply. What they get is worth knowing precisely, because it decides how
much defence is warranted:

- An unverified identity in one public realm, at `min_assurance` (IAL1).
- **No grants.** Registration creates a person, never authority — the whole
  point of separating identity from authorization.
- No tenant data. A public-realm identity with no grant is denied by
  `authorize()` on every axis a permission constrains.

So the loss is not access. It is **noise**: rows in the directory, mail sent
to addresses that did not ask for it (a reputational cost paid by the
tenant's sending domain), and a retention obligation for records nobody
wanted.

The obvious defences each carry a real cost:

- **CAPTCHA** — a third-party script on the sign-in page, which ADR-0006's
  CSP (`default-src 'none'`) exists to forbid, and a dependency on a vendor
  who then sees every registration attempt.
- **Proof of work** — no third-party dependency, but it taxes the honest
  user's phone to inconvenience an attacker's server.
- **Email-first** — send the verification mail before creating any row, so a
  bogus registration leaves nothing behind. Cheap, but moves the abuse to
  the mail path, which is the expensive one.
- **Invite-only** — the complete answer, and it stops being self-registration.

## Decision

**Ship without bot protection. Do not add a CAPTCHA.**

Instead:

1. **Self-registration is off by default.** It is a per-realm flag a tenant
   turns on knowingly. A tenant who does not need it is not exposed to any
   of this, which is most of them.
2. **Rate limits stay per IP** and the audit log records every attempt, so a
   campaign is visible in
   `anubis_endpoint_requests_total{endpoint="auth.register"}` and in the
   `rate_limited` rate that [alerting.md](../alerting.md) already watches.
3. **Retention does the cleanup.** Unverified identities age out under the
   realm's `default_retention` — the sweeper anonymises and shreds without
   anyone deciding which rows were bots.
4. **The escape hatch is the flag.** A tenant under attack turns
   self-registration off, and their existing users are unaffected. That is a
   30-second mitigation with no vendor, no deploy, and no new dependency.

## Consequences

- A tenant that enables self-registration on a public realm **accepts
  registration spam as a possibility**, and the documentation must say so
  rather than implying the rate limiter prevents it.
- Anubis's sign-in pages keep their strict CSP. No third-party script runs
  on a page that handles credentials — which is worth more than the abuse
  this would prevent.
- The mitigation path is operational (turn it off, let retention clean up),
  not architectural. That is deliberate: the mechanism that would prevent
  the abuse costs more than the abuse.

## Revisit when

- A production tenant actually suffers a registration campaign. Then the
  measurement exists, and a proof-of-work challenge — self-hosted, no vendor,
  no third-party script — becomes arguable on evidence rather than fear.
- Registration ever confers anything beyond an unverified identity. The
  moment a self-registered account receives a grant automatically, the loss
  stops being noise and this ADR is void.
