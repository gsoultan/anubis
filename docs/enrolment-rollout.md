# Rolling out a required second factor

Two rules are in force.

**An enrolled factor is always demanded.** Enrol TOTP and every subsequent
sign-in asks for it, on both planes — tenant identities and platform
operators. That holds whatever the realm requires.

**Required but not enrolled is governed by a date**, not a flag:
`realms.factor_enrolment_deadline`. NULL — the state of every realm until you
set one — means the policy is not in force and a realm listing `totp` still
admits a password-only login. Set a date and:

- **before it**, sign-in works and the response carries `enrolment_due`: the
  factors owed and the deadline. A client that ignores it costs its user
  access on the day with no warning.
- **on and after it**, sign-in returns `enrolment_required` instead of a
  session — carrying a `grant_token` that is accepted by
  `BeginTotpEnrollment` and `ConfirmTotpEnrollment` in place of one.

That token is the whole design. The policy withholds the session, and
enrolment needs a session, so without it the policy would be unsatisfiable by
exactly the people it applies to.

## Why it is a date and not a config flag

Because flipping a flag is not a configuration change, it is a lockout event.

At the moment the flag turns on, every user who has not enrolled loses
access — and the enrolment endpoint itself requires a session, so they
cannot fix it themselves. The people best placed to notice are the ones
locked out. On the platform plane it is worse: an owner who has not enrolled
locks themselves out of the console that grants authority, and the only way
back is the database. That is why `db.sh devadmin` clears the dev operator's
factor rather than leaving it — an environment that lies about who can sign
in is worse than one with no script at all.

So the missing piece is not the check. It is the **grace period**: a window
where the policy is in force, unenrolled users are told, and they can still
get in to comply.

## The rollout, in order

Do not skip step 2. It is the whole point.

### 1. Measure who would be locked out

```sql
-- Tenant identities in realms that would demand TOTP, without one enrolled.
-- Runs as-is; add the tenant filter to narrow it.
SELECT t.slug AS tenant, r.code AS realm, count(*) AS would_be_locked_out
FROM identities i
JOIN realms r ON r.id = i.realm_id AND r.tenant_id = i.tenant_id
JOIN tenants t ON t.id = i.tenant_id
WHERE i.status = 'active'
  AND 'totp' = ANY(r.required_factors)
  AND NOT EXISTS (
      SELECT 1 FROM credentials c
      WHERE c.identity_id = i.id AND c.kind = 'totp' AND c.revoked_at IS NULL
  )
GROUP BY t.slug, r.code
ORDER BY would_be_locked_out DESC;

-- Operators, who matter more: each one is authority over the installation.
SELECT username, totp_enrolled_at IS NOT NULL AS enrolled
FROM platform_users WHERE status = 'active';
```

If the first query returns a number you are not willing to have call the
service desk on Monday morning, you are not ready for step 4.

### 2. Announce, with a deadline and a link

Tenants own this communication; Anubis cannot mail their people. Give them
the enrolment URL and the date. The Platform users screen shows which
operators have no second factor — chase those by name, especially owners.

### 3. Enrol the operators first

An installation whose owners are all enrolled can recover from anything that
follows. An installation whose owners are not is one flag away from needing
a DBA. Confirm with the query above that every `owner` row reads enrolled
before going near step 4.

### 4. Turn the policy on for ONE realm

Not the whole tenant, and not the internal realm first — pick the smallest
population that still exercises a real sign-in flow. Watch, for 24 hours:

- `anubis_endpoint_requests_total{endpoint="auth.login",code="mfa_required"}`
  rising is the policy working.
- `code="invalid_credentials"` rising instead means people are failing
  before the factor step; that is not this change, look elsewhere.
- The support queue is the real instrument. If it moves, stop.

### 5. Widen, one realm at a time

Public and partner realms last. Those users have the least context, the
worst recovery options, and no service desk of their own.

## What "in force" actually does

The refusal is **enrol-or-deny, not deny**. A login that fails the policy
returns a challenge carrying an enrolment path, not a bare `mfa_required` the
user cannot act on. A policy that locks people out without telling them how to
comply is a support incident with extra steps.

Two safeguards are worth knowing about:

- A grant is minted **only for a member with none of the required factors
  enrolled**. Redeeming one against an identity that already has a factor is
  refused with a conflict — otherwise a leaked grant would *replace* somebody's
  authenticator rather than add their first, which is an account takeover
  wearing a compliance hat.
- A grant lives 15 minutes and buys one thing: enrolling a factor for that
  identity. It is not a session, carries no scopes, and is issued only after
  the correct password was presented.

## Both doors

The policy is evaluated by one `signin.PasswordAuthenticator`, called by
`AuthService.Login` and by the hosted sign-in page alike. That is deliberate
and recent: until 2026-09-20 the page decided for itself and never asked about
the deadline, so a realm in force refused API clients and admitted the same
member through a browser.

Both doors now refuse the same member AND offer the same way out, though they
spend the grant differently:

| | API (`AuthService.Login`) | Hosted page (`POST /v1/login`) |
| :--- | :--- | :--- |
| Past the deadline, nothing enrolled | Refused, with a 15-minute grant token the caller drives `BeginTOTP`/`ConfirmTOTP` with | Refused, and the page drives the same two calls itself — setup key, `otpauth://` link, code field, recovery codes shown once |
| Inside the grace period | Signed in, plus the deadline and the missing factors | Signed in, then shown the date and the missing factor, with "set it up now" and "continue without it" |

The browser never receives the grant. It holds an opaque single-use handle
(`one_time_tokens.kind = 'browser_enrol'`) and the server keeps the grant,
the pending enrolment token and the identity behind it.

**A wrong code costs the whole key, on both doors.** `ConfirmTOTP` spends the
enrolment token *before* it verifies the code, so a stolen grant buys one
guess rather than unlimited ones. The page says so and issues a fresh key
rather than pretending a retry is possible against a secret that no longer
exists. Tell people to transfer the key carefully; it is not a typo-tolerant
step.

**Three ways in**, because which one works depends on the device in front of
the person: scan the QR from another phone, tap the `otpauth://` link to open
an authenticator on this one, or type the key.

The encoder is ours (`internal/platform/qr`) because ADR-0002 forbids the
library that would otherwise draw it. Byte mode, level M, versions 1 to 10 —
an `otpauth://` URI lands around version 6. It renders as inline SVG, so the
page still fetches nothing and its `default-src 'none'` CSP is unchanged.

**If a symbol ever fails to scan, the key beside it is the same secret.** The
page logs a warning and carries on when the encoder returns an error, because
a symbol that will not build is not a reason to refuse somebody an
enrolment.

### What the warning does, and why it is skippable

Inside the grace period the page writes the session and the cookie FIRST and
then renders the warning, so both buttons lead somewhere and neither blocks.
"Continue without it" re-enters `/v1/authorize`, which finds the cookie and
issues the code without prompting — the warning cannot strand anybody.

It appears on every sign-in during the grace period. That is deliberate: the
deadline is a date the operator chose, and the runway only works if the people
on it know about it. If that proves too noisy for a long grace period, the
place to change it is here, not by making the warning conditional on something
the page cannot see.

Somebody who enrols from the warning is **not asked for their password
again** — they already hold a session, and charging extra for complying early
is how a rollout stalls. They get the recovery codes and a "continue" link.
Somebody who was refused has no session, so they enrol and then sign in.

## Rolling back

Setting `factor_enrolment_deadline` back to NULL — or removing the factor
from `required_factors` — restores the previous behaviour immediately — no token or session is invalidated by the policy itself, so
rollback is a config change and nothing else. **Enrolments survive**, which
means the second attempt starts ahead of the first.
