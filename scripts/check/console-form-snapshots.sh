#!/usr/bin/env bash
# Console components must not read tanstack-form state as a snapshot.
#
# `form.state` is the form's state at the moment it is read. Typing into a
# field re-renders that field and nothing else, so whatever a component
# computes from `form.state.values` during render is frozen at the values it
# had the last time something ELSE re-rendered it.
#
# It shipped twice, found on 2026-09-25:
#   - CreatePermission built its key preview from form.state.values, and the
#     submit button required the preview — so however the form was filled,
#     the preview stayed empty and the button stayed disabled.
#   - CreateIdentity looked up the chosen population the same way, so the
#     population's facts never appeared and its categories were never
#     fetched: nobody added from the console could be given a category.
# Three more reads sat inside <form.Subscribe> and worked only because a
# validator happened to flip canSubmit after the last keystroke.
#
# Subscribe to what render reads: useStore(form.store, selector), or put it in
# the <form.Subscribe> selector. In an event handler, where a snapshot is
# exactly right, use form.getFieldValue(name).
set -euo pipefail
cd "$(dirname "$0")/../.."

hits="$(grep -rn 'form\.state\.' ui/src --include='*.tsx' --include='*.ts' || true)"
if [ -n "$hits" ]; then
  echo "FAIL: console code reads tanstack-form state as a snapshot:" >&2
  printf '%s\n' "$hits" | sed 's/^/  /' >&2
  echo "" >&2
  echo "Render never sees a later keystroke through form.state. Subscribe with" >&2
  echo "useStore(form.store, (s) => ...) or a <form.Subscribe> selector; in an" >&2
  echo "event handler, read form.getFieldValue(name)." >&2
  exit 1
fi

echo "ok: no console component reads form state as a snapshot"
