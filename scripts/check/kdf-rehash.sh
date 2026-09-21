#!/usr/bin/env bash
# kdf.Verify returns needsRehash, and discarding it is how a password stays at
# an iteration count the installation has already moved past.
#
# Two surfaces got this wrong before anyone noticed. The hosted sign-in page
# wrote `ok, _, kerr :=` and browser-only accounts never migrated; the control
# plane wrote `ok, _, _ :=` and operators never migrated either — and nothing
# in the API changes a platform password, so for them a login is the ONLY
# migration path there will ever be.
#
# Verifying the SAME property in two places is what this whole class of bug is
# made of, so it is checked once, here, for every call site at once.
#
# Two discards are legitimate, and both have to say so on the line:
#
#   kdf.Verify(pw, kdf.Dummy())  — a deliberate burn, called for its time
#                                  rather than its answer, with no hash to
#                                  upgrade.
#   // kdf-rehash-exempt: <why>  — the hash is about to be overwritten
#                                  anyway, as in a password change.
#
# The marker carries a reason because an exemption nobody has to justify is
# how this check stops meaning anything.
set -euo pipefail
cd "$(dirname "$0")/../.."

hits=$(grep -rn --include='*.go' -E '[_a-zA-Z0-9]+, *_, *[_a-zA-Z0-9]+ *:?= *kdf\.Verify\(' \
  cmd internal pkg 2>/dev/null \
  | grep -v 'kdf\.Dummy()' \
  | grep -vE 'kdf-rehash-exempt: *[^ ]' || true)

if [ -n "$hits" ]; then
  echo "FAIL: kdf.Verify's needsRehash discarded — the password stays at its old cost:" >&2
  echo "$hits" >&2
  echo "" >&2
  echo "Use the flag and write the upgraded hash back; or verify against" >&2
  echo "kdf.Dummy() for a timing burn; or mark the line" >&2
  echo "  // kdf-rehash-exempt: <why>" >&2
  echo "if the hash is overwritten regardless." >&2
  exit 1
fi
echo "ok: every kdf.Verify uses needsRehash"
