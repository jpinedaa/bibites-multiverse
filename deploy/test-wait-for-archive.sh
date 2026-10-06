#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SUT="$HERE/wait-for-archive.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

cat >"$TMP/curl" <<'EOF'
#!/bin/sh
n=0
[ -f "$WAIT_COUNT" ] && n="$(cat "$WAIT_COUNT")"
n=$((n + 1))
printf '%s\n' "$n" >"$WAIT_COUNT"
[ "$n" -ge "${WAIT_SUCCEED_AT:-999}" ]
EOF
cat >"$TMP/sleep" <<'EOF'
#!/bin/sh
:
EOF
chmod +x "$TMP/curl" "$TMP/sleep"

WAIT_COUNT="$TMP/count" WAIT_SUCCEED_AT=3 PATH="$TMP:$PATH" \
  MULTIVERSE_ARCHIVE_READY_TIMEOUT=5 MULTIVERSE_ARCHIVE_READY_INTERVAL=1 \
  "$SUT" | grep -Fq 'archive ready'
[ "$(cat "$TMP/count")" = 3 ]

rm -f "$TMP/count"
started=$(date +%s)
if WAIT_COUNT="$TMP/count" PATH="$TMP:$PATH" \
   MULTIVERSE_ARCHIVE_READY_TIMEOUT=2 MULTIVERSE_ARCHIVE_READY_INTERVAL=1 \
   "$SUT" >"$TMP/out" 2>&1; then
  echo 'wait-for-archive test: timeout unexpectedly passed' >&2
  exit 1
fi
elapsed=$(($(date +%s) - started))
[ "$elapsed" -ge 2 ] && [ "$elapsed" -le 3 ] || {
  echo "wait-for-archive test: 2s deadline took ${elapsed}s" >&2
  exit 1
}
grep -Fq 'relay remains stopped' "$TMP/out"

if MULTIVERSE_ARCHIVE_READY_TIMEOUT=bad "$SUT" >"$TMP/out" 2>&1; then
  echo 'wait-for-archive test: invalid timeout unexpectedly passed' >&2
  exit 1
fi
grep -Fq 'positive whole seconds' "$TMP/out"

echo 'archive readiness gate OK'
