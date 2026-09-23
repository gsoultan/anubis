#!/usr/bin/env bash
# A migration that has shipped is a historical record, not documentation.
#
# The runner pins each applied migration by sha256 of the WHOLE FILE, so
# editing one that is already out there — even only its comments — makes every
# existing installation log this on every boot, forever:
#
#   ERROR migration modified after being applied (checksum drift)
#
# It is not fatal and it is not clearable: `anubisd baseline` inserts missing
# rows with ON CONFLICT DO NOTHING, so it will not update a checksum that is
# already recorded. The only remedy is editing schema_migrations by hand,
# which nobody should be told to do. The cost is worse than the noise — an
# ERROR that everybody learns to ignore is a broken alarm, and the next one
# will be real.
#
# It happened: 0005_routes_audit.sql had a comment corrected in the v0.4.0
# cycle. The correction was right — the hash chain does not stop a wholesale
# rewrite — but the place for it is the migration that FIXED it (0049), which
# every installation applies fresh. This check exists because that is not
# obvious until you have watched a dev server print the error for a week.
#
# Compared against the last release tag rather than the previous commit: a
# migration added and then amended before it ships has never been applied
# anywhere, and reworking it is normal.
set -euo pipefail
cd "$(dirname "$0")/../.."

tag="$(git tag --sort=-creatordate | head -1)"
if [ -z "$tag" ]; then
  echo "ok: no release tag yet, nothing has shipped"
  exit 0
fi

# Only files that EXISTED at the tag can drift; new ones cannot.
changed=""
while IFS= read -r f; do
  [ -z "$f" ] && continue
  if git cat-file -e "$tag:$f" 2>/dev/null; then
    changed="$changed$f"$'\n'
  fi
done < <(git diff --name-only "$tag"..HEAD -- 'migrations/*.sql' 2>/dev/null || true)

if [ -n "$changed" ]; then
  echo "FAIL: migrations already shipped in $tag were modified:" >&2
  printf '%s' "$changed" | sed 's/^/  /' >&2
  echo "" >&2
  echo "Every installation that applied these will log a checksum-drift ERROR" >&2
  echo "on every boot, and baseline cannot clear it. Restore them with" >&2
  echo "" >&2
  echo "  git checkout $tag -- <file>" >&2
  echo "" >&2
  echo "and put whatever you were correcting in the migration that changed the" >&2
  echo "behaviour, or in docs/. A shipped migration records what it did then." >&2
  exit 1
fi

echo "ok: no migration shipped in $tag has been modified"
