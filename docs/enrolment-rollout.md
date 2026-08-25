# Rolling out a required second factor

The rule today: **an enrolled factor is always demanded.** Enrol TOTP and
every subsequent sign-in asks for it, on both planes — tenant identities and
platform operators.

The rule that is deliberately NOT implemented: *required but not enrolled*.
A realm listing `totp` in `required_factors` still admits a password-only
login from somebody who has not enrolled one.

## Why it is not a config flag

Because flipping it is not a configuration change, it is a lockout event.

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

## What "in force" must actually do

When the flag exists, the refusal has to be **enrol-or-deny, not deny**:
a login that fails the policy returns a challenge that carries an enrolment
path, not a bare `mfa_required` the user cannot act on. A policy that locks
people out without telling them how to comply is a support incident with
extra steps.

## Rolling back

Removing the factor from `required_factors` restores the previous behaviour
immediately — no token or session is invalidated by the policy itself, so
rollback is a config change and nothing else. **Enrolments survive**, which
means the second attempt starts ahead of the first.
