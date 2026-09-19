#!/usr/bin/env bash
# The alert rules are a shipped artefact, so something has to parse them.
#
# docs/alerting.md carried these as a markdown table for as long as they have
# existed and nothing ever loaded one. A rule naming a metric that does not
# exist, or selecting on a label the exporter never emits, evaluates to "ok"
# with an empty result — indistinguishable from a rule whose condition is
# simply false. The place you discover the difference is the incident it was
# written for.
#
# promtool catches the syntax. What it cannot catch is a name that parses and
# does not exist, which is why packaging/anubis.rules.yml was checked against a
# live scrape when it was written; see the header there.
set -euo pipefail
cd "$(dirname "$0")/../.."

RULES=packaging/anubis.rules.yml
[ -f "$RULES" ] || { echo "FAIL: $RULES is missing" >&2; exit 1; }

if ! command -v promtool >/dev/null 2>&1; then
  # Say so rather than pretending the check ran — the same reasoning
  # gen-drift.sh applies to storm without a database.
  echo "ok: alert rules present (promtool not installed — syntax unchecked)"
  exit 0
fi

promtool check rules "$RULES"

# Every alert carries the two things an operator needs at 3am: what broke, and
# where the answer is written down. promtool requires neither.
#
# Scoped to each alert's own block. The first version of this check let the
# "found a runbook" flag run past the end of the alert it belonged to, so a
# deleted runbook was satisfied by the NEXT alert's — a check that passed
# whatever it was given, which is the exact failure it exists to catch.
missing=$(awk '
  /^[[:space:]]*- alert:/ {
    if (name != "" && !seen) print name
    name = $3; seen = 0; next
  }
  /^[[:space:]]*runbook:/ { seen = 1 }
  END { if (name != "" && !seen) print name }
' "$RULES")

if [ -n "$missing" ]; then
  echo "FAIL: these alerts name no runbook:" >&2
  echo "$missing" | sed 's/^/  /' >&2
  exit 1
fi

echo "ok: alert rules parse and every alert names a runbook"
