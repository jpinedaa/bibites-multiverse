#!/usr/bin/env bash
# Exercise the roll-up rebuild gate without a service host.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SUT="$HERE/restart-archive.sh"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/mvrestarttest.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT

PASS=0
FAIL=0
ok() { printf '  ok    %s\n' "$1"; PASS=$((PASS + 1)); }
bad() { printf '  FAIL  %s\n' "$1" >&2; FAIL=$((FAIL + 1)); }

mkdir -p "$TMP/fakebin" "$TMP/state/archive/segments"
printf 'old roll-up\n' >"$TMP/state/archive/rollup.jsonl"
printf 'MV_STATE=%s\nMV_REPLAY_PEAK_B=1\nMV_REPLAY_RESIDENT_B=1\n' "$TMP/state" >"$TMP/deploy.env"

cat >"$TMP/fakebin/curl" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
out=""
while [ $# -gt 0 ]; do
  if [ "$1" = -o ]; then out="${2:?curl stub: -o needs a path}"; shift; fi
  shift
done
[ -n "$out" ] || { echo "curl stub: no -o path" >&2; exit 2; }
cp "$MV_FAKE_STATUS" "$out"
SH

cat >"$TMP/fakebin/systemctl" <<'SH'
#!/usr/bin/env sh
case "${1:-}" in
  is-active) printf 'active\nactive\n' ;;
  show) : ;;
esac
SH
chmod +x "$TMP/fakebin/curl" "$TMP/fakebin/systemctl"

run_sut() {
  PATH="$TMP/fakebin:$PATH" MV_ENV_FILE="$TMP/deploy.env" MV_FAKE_STATUS="$1" \
    "$SUT" --dry-run --rebuild-rollup --i-proved-the-replay-fits \
      --proof 'test fixture' --reason 'test the roll-up rebuild gate'
}

printf '{"ledgerRecords":100,"ledgerRetiredTotal":0}\n' >"$TMP/complete.json"
if out="$(run_sut "$TMP/complete.json" 2>&1)" &&
   printf '%s\n' "$out" | grep -Fq 'absent raw segments  0' &&
   printf '%s\n' "$out" | grep -Fq "mv -- $TMP/state/archive/rollup.jsonl"; then
  ok "a complete raw record reaches the preserved-sidecar step"
else
  bad "a complete raw record reaches the preserved-sidecar step"
fi

printf 'receipt\n' >"$TMP/state/archive/segments/2026-01-01-0000.jsonl.gz.receipt"
printf '{"ledgerRecords":100,"ledgerRetiredTotal":0}\n' >"$TMP/retired.json"
if out="$(run_sut "$TMP/retired.json" 2>&1)"; then
  bad "an absent raw segment refuses the rebuild"
elif printf '%s\n' "$out" | grep -Fq '1 raw segment(s) are absent'; then
  ok "an absent raw segment refuses the rebuild"
else
  bad "an absent raw segment refuses the rebuild"
fi

printf 'restored raw segment\n' >"$TMP/state/archive/segments/2026-01-01-0000.jsonl"
if run_sut "$TMP/retired.json" >/dev/null 2>&1; then
  ok "a restored plain segment satisfies its receipt"
else
  bad "a restored plain segment satisfies its receipt"
fi

printf '\n%d pass, %d fail\n' "$PASS" "$FAIL"
[ "$FAIL" = 0 ]
