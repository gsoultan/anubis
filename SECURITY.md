# Reporting a vulnerability

Do not open a public issue. Use GitHub's private reporting —
**Security → Report a vulnerability** — or email
**gembit.soultan@gmail.com**.

Please include what you need to make the finding reproducible: the version
(`anubisd version`), what you sent, what came back, and what you expected
instead. A proof of concept is welcome and never required.

You will get an acknowledgement within **72 hours** and an assessment within
**7 days**. If a fix is warranted, it ships in a patch release with the
finding credited unless you would rather not be.

## What is in scope

The service and everything shipped with it: the authorization engine, both
authentication planes, the gate, the console served from the binary, the
`pkg/anubis` verifier SDK, the packaging, and the release artefacts.

Findings that matter especially here, because the whole design rests on them:

- **Cross-tenant reach of any kind.** Every scope and grant foreign key is
  composite on `tenant_id` precisely so this is unstorable rather than merely
  refused. A way around that is the most serious report this project can
  receive.
- **Authorization that answers differently from `authorize()`** — the
  in-memory gate snapshot and the SQL engine must agree. A disagreement is a
  bypass even when neither side looks wrong alone.
- **Path normalisation.** The gate and any in-app router must normalise
  identically; the gap between two normalisers is the bypass. Fuzzing found
  two real ones (percent-encoded dot-segments, and a decoded `#` that reads
  as data here and a delimiter to a re-parser).
- **Token handling**: refresh reuse that does not revoke the family,
  signature or audience confusion, a `kid` that drives I/O, anything that
  survives `token_epoch`.
- **Timing that distinguishes a real account from an unknown one.** Login
  deliberately pays the full KDF either way and is asserted by test.

## Known and deliberate

Please do not report these as vulnerabilities; each is a documented decision
with the reasoning written down.

- **Rate limits are per instance.** Counters live in the process, so N
  replicas enforce N times the published allowance
  ([ADR-0012](docs/adr/0012-rate-limits-across-replicas.md)).
- **No bot protection on self-registration.** A self-registered account is an
  unverified identity with no grants
  ([ADR-0014](docs/adr/0014-bot-protection-on-registration.md)).
- **A realm requiring TOTP still admits a password-only login from someone
  not enrolled.** Closing it without a grace period locks out existing users;
  see [the rollout playbook](docs/enrolment-rollout.md).
- **`identities.attributes` is not encrypted yet.** It is also empty by
  construction — a CHECK constraint makes plaintext there unstorable until
  the sealing path lands ([ADR-0013](docs/adr/0013-pii-encryption-scope.md)).

## Verifying what you run

Releases ship an SBOM per archive and a signed `checksums.txt`. The signature
is keyless: the signing identity is the release workflow itself, recorded in
a public transparency log, so there is no private key to compromise.
Verification is in the [README](README.md#installing-a-release).
