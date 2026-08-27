-- ============================================================================
-- 0036 — enrol-or-deny needs a deadline, not a flag.
--
-- A realm can list `totp` in required_factors today and still admit a
-- password-only login from somebody who never enrolled one. That gap was
-- deliberate (docs/enrolment-rollout.md): closing it with a boolean means
-- every unenrolled user loses access the instant it flips, and the enrolment
-- endpoint needs a session they can no longer get. The people best placed to
-- notice are the ones locked out.
--
-- So the switch is a date. Before it, an unenrolled user signs in and is told
-- when they must comply. After it, the refusal carries an enrolment token, so
-- "you must enrol" comes with the means to do it — enrol-or-deny rather than
-- deny.
--
-- NULL means the policy is not in force, which is every realm today. Turning
-- it on is setting a date, and rolling back is setting it to NULL again:
-- enrolments survive, so a second attempt starts ahead of the first.
-- ============================================================================

ALTER TABLE realms
    ADD COLUMN factor_enrolment_deadline timestamptz;

COMMENT ON COLUMN realms.factor_enrolment_deadline IS
    'When required_factors starts being enforced against members who have not '
    'enrolled. NULL = not in force. Future = grace period, sign-in works and '
    'warns. Past = enrol-or-deny. See docs/enrolment-rollout.md.';
