#!/usr/bin/env bash
# Exercise running-binary capture and explicit generation restore without a host.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SUT="$HERE/binary-generation.sh"
[ -x "$SUT" ] || { echo "test-binary-generation: $SUT is not executable" >&2; exit 1; }

TMP="$(mktemp -d "${TMPDIR:-/tmp}/mv-generation.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT
PASS=0
FAIL=0
ok() { PASS=$((PASS + 1)); printf '  ok    %s\n' "$1"; }
bad() { FAIL=$((FAIL + 1)); printf '  FAIL  %s\n' "$1"; }

assert_true() {
  local label="$1"
  shift
  if "$@"; then ok "$label"; else bad "$label"; fi
}

assert_eq() {
  local label="$1" expected="$2" actual="$3"
  if [ "$expected" = "$actual" ]; then
    ok "$label"
  else
    bad "$label (expected '$expected', got '$actual')"
  fi
}

FAKEBIN="$TMP/fakebin"
mkdir -p "$FAKEBIN"
REAL_DD="$(command -v dd)"

cat >"$FAKEBIN/systemctl" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"${FAKE_SYSTEMCTL_LOG:?}"
[ "${1:-}" = show ] || exit 90
unit="${!#}"
count=0
if [ -f "${FAKE_SYSTEMCTL_STATE:?}" ]; then
  read -r count <"$FAKE_SYSTEMCTL_STATE"
fi
count=$((count + 1))
printf '%s\n' "$count" >"$FAKE_SYSTEMCTL_STATE"
case "$unit" in
  multiverse-relay.service)
    if [ "${FAKE_SYSTEMCTL_MODE:-normal}" = race ] && [ "$count" -ge 3 ]; then
      printf '%s\n' "${FAKE_RACE_RELAY_PID:-303}"
    else
      printf '%s\n' "${FAKE_RELAY_PID:-101}"
    fi
    ;;
  multiverse-archive.service) printf '%s\n' "${FAKE_ARCHIVE_PID:-202}" ;;
  *) exit 91 ;;
esac
SH

cat >"$FAKEBIN/dd" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
count=0
if [ -f "${FAKE_DD_STATE:?}" ]; then
  read -r count <"$FAKE_DD_STATE"
fi
count=$((count + 1))
printf '%s\n' "$count" >"$FAKE_DD_STATE"
if [ "${FAKE_DD_FAIL_ON:-0}" = "$count" ]; then
  exit 92
fi
if [ "${FAKE_DD_MUTATE_ON:-0}" = "$count" ]; then
  printf 'changed during capture\n' >>"${FAKE_DD_MUTATE_PATH:?}"
fi
exec "${REAL_DD:?}" "$@"
SH
chmod +x "$FAKEBIN/systemctl" "$FAKEBIN/dd"

new_fixture() {
  local name="$1" relation="${2:-split}"
  FIXTURE="$TMP/$name"
  BIN="$FIXTURE/bin"
  PROC="$FIXTURE/proc"
  STORE="$FIXTURE/store"
  SYSTEMCTL_LOG="$FIXTURE/systemctl.log"
  SYSTEMCTL_STATE="$FIXTURE/systemctl.state"
  DD_STATE="$FIXTURE/dd.state"
  mkdir -p "$BIN" "$PROC/101" "$PROC/202"
  printf 'running relay for %s\n' "$name" >"$FIXTURE/running-relay"
  printf 'running archive for %s\n' "$name" >"$FIXTURE/running-archive"
  ln -s "$FIXTURE/running-relay" "$PROC/101/exe"
  ln -s "$FIXTURE/running-archive" "$PROC/202/exe"
  if [ "$relation" = equal ]; then
    cp "$FIXTURE/running-relay" "$BIN/multiverse-relay"
    cp "$FIXTURE/running-archive" "$BIN/multiverse-archive"
  else
    printf 'installed relay for %s\n' "$name" >"$BIN/multiverse-relay"
    printf 'installed archive for %s\n' "$name" >"$BIN/multiverse-archive"
  fi
  printf 'installed ringstat for %s\n' "$name" >"$BIN/ringstat"
}

run_generation() {
  PATH="$FAKEBIN:$PATH" \
  REAL_DD="$REAL_DD" \
  FAKE_DD_STATE="$DD_STATE" \
  FAKE_DD_FAIL_ON="${DD_FAIL_ON:-0}" \
  FAKE_DD_MUTATE_ON="${DD_MUTATE_ON:-0}" \
  FAKE_DD_MUTATE_PATH="${DD_MUTATE_PATH:-unused}" \
  FAKE_SYSTEMCTL_LOG="$SYSTEMCTL_LOG" \
  FAKE_SYSTEMCTL_STATE="$SYSTEMCTL_STATE" \
  FAKE_SYSTEMCTL_MODE="${SYSTEMCTL_MODE:-normal}" \
  FAKE_RELAY_PID="${RELAY_PID:-101}" \
  FAKE_ARCHIVE_PID="${ARCHIVE_PID:-202}" \
  "$SUT" "$@"
}

generation_from() {
  sed -n 's/^rollback_generation=//p' | tail -1
}

evidence_from() {
  grep -E '^(rollback_generation|generation_status)='
}

echo
echo "== active and installed generations can differ"
new_fixture split
out="$(run_generation capture --bin-dir "$BIN" --store "$STORE" --proc-root "$PROC")"
generation="$(printf '%s\n' "$out" | generation_from)"
generation_dir="$STORE/$generation"
assert_true "capture reports a content-addressed generation" \
  bash -c '[[ "$1" =~ ^sha256-[0-9a-f]{64}$ ]]' _ "$generation"
assert_true "relay comes from the stable running handle" \
  cmp -s "$FIXTURE/running-relay" "$generation_dir/multiverse-relay"
assert_true "archive comes from the stable running handle" \
  cmp -s "$FIXTURE/running-archive" "$generation_dir/multiverse-archive"
assert_true "the installed relay split is not mistaken for the running relay" \
  bash -c '! cmp -s "$1" "$2"' _ "$BIN/multiverse-relay" "$generation_dir/multiverse-relay"
installed_relay_sha="$(sha256sum "$BIN/multiverse-relay" | awk '{print $1}')"
assert_true "manifest records the installed relay hash" \
  grep -Fxq $'installed_relay_sha256\t'"$installed_relay_sha" "$generation_dir/manifest.tsv"
assert_eq "generation directory is owner-only" 700 "$(stat -c '%a' "$generation_dir")"
assert_eq "rollback artifact is owner-only" 600 "$(stat -c '%a' "$generation_dir/multiverse-relay")"
assert_eq "a new capture emits one complete evidence pair" \
  "rollback_generation=$generation"$'\n''generation_status=captured' \
  "$(printf '%s\n' "$out" | evidence_from)"

out="$(run_generation capture --bin-dir "$BIN" --store "$STORE" --proc-root "$PROC")"
assert_eq "an existing capture emits one complete evidence pair" \
  "rollback_generation=$generation"$'\n''generation_status=already-captured' \
  "$(printf '%s\n' "$out" | evidence_from)"

echo
echo "== the normal installed and running generation"
new_fixture equal equal
out="$(run_generation capture --bin-dir "$BIN" --store "$STORE" --proc-root "$PROC")"
generation="$(printf '%s\n' "$out" | generation_from)"
running_relay_sha="$(sha256sum "$FIXTURE/running-relay" | awk '{print $1}')"
assert_true "equal generation still produces a verified manifest" \
  grep -Fxq $'relay_sha256\t'"$running_relay_sha" "$STORE/$generation/manifest.tsv"
assert_true "equal installed hash is recorded independently" \
  grep -Fxq $'installed_relay_sha256\t'"$running_relay_sha" "$STORE/$generation/manifest.tsv"

echo
echo "== a PID race rejects the capture"
new_fixture pid-race
SYSTEMCTL_MODE=race
if run_generation capture --bin-dir "$BIN" --store "$STORE" --proc-root "$PROC" \
    >"$FIXTURE/out" 2>&1; then
  bad "PID race is rejected"
else
  ok "PID race is rejected"
fi
SYSTEMCTL_MODE=normal
artifact_count=0
[ ! -d "$STORE" ] || artifact_count="$(find "$STORE" -mindepth 1 -maxdepth 1 | wc -l)"
assert_eq "PID race publishes no generation or partial capture" 0 "$artifact_count"

echo
echo "== a partial copy publishes nothing"
new_fixture partial
DD_FAIL_ON=2
if run_generation capture --bin-dir "$BIN" --store "$STORE" --proc-root "$PROC" \
    >"$FIXTURE/out" 2>&1; then
  bad "partial copy is rejected"
else
  ok "partial copy is rejected"
fi
DD_FAIL_ON=0
artifact_count=0
[ ! -d "$STORE" ] || artifact_count="$(find "$STORE" -mindepth 1 -maxdepth 1 | wc -l)"
assert_eq "partial copy leaves no generation or temporary directory" 0 "$artifact_count"

echo
echo "== a running hash race rejects the capture"
new_fixture hash-race
DD_MUTATE_ON=2
DD_MUTATE_PATH="$FIXTURE/running-relay"
if run_generation capture --bin-dir "$BIN" --store "$STORE" --proc-root "$PROC" \
    >"$FIXTURE/out" 2>&1; then
  bad "running hash race is rejected"
else
  ok "running hash race is rejected"
fi
DD_MUTATE_ON=0
DD_MUTATE_PATH=""
artifact_count=0
[ ! -d "$STORE" ] || artifact_count="$(find "$STORE" -mindepth 1 -maxdepth 1 | wc -l)"
assert_eq "running hash race publishes no generation" 0 "$artifact_count"

echo
echo "== dry-run proves the preimage and writes nothing"
new_fixture dry-run
relay_sha="$(sha256sum "$FIXTURE/running-relay" | awk '{print $1}')"
out="$(run_generation capture --dry-run --bin-dir "$BIN" --store "$STORE" --proc-root "$PROC")"
generation="$(printf '%s\n' "$out" | generation_from)"
if printf '%s\n' "$out" | grep -Fq "running_relay_pid=101 sha256=$relay_sha"; then
  ok "dry-run output proves the running relay hash"
else
  bad "dry-run output proves the running relay hash"
fi
assert_eq "dry-run completes both rechecks and emits one evidence pair" \
  "rollback_generation=$generation"$'\n''generation_status=dry-run-verified' \
  "$(printf '%s\n' "$out" | evidence_from)"
assert_true "dry-run does not create the rollback store" test ! -e "$STORE"

echo
echo "== restore requires one exact verified generation"
new_fixture restore
out="$(run_generation capture --bin-dir "$BIN" --store "$STORE" --proc-root "$PROC")"
generation="$(printf '%s\n' "$out" | generation_from)"
printf 'new relay\n' >"$BIN/multiverse-relay"
printf 'new archive\n' >"$BIN/multiverse-archive"
printf 'new ringstat\n' >"$BIN/ringstat"
new_relay_sha="$(sha256sum "$BIN/multiverse-relay" | awk '{print $1}')"
: >"$SYSTEMCTL_LOG"
if run_generation restore --bin-dir "$BIN" --store "$STORE" >"$FIXTURE/out" 2>&1; then
  bad "restore without --generation is rejected"
else
  ok "restore without --generation is rejected"
fi
assert_eq "missing generation changes no installed byte" "$new_relay_sha" \
  "$(sha256sum "$BIN/multiverse-relay" | awk '{print $1}')"
run_generation restore --dry-run --generation "$generation" --bin-dir "$BIN" --store "$STORE" \
  >"$FIXTURE/dry-restore.out"
assert_eq "restore dry-run changes no installed byte" "$new_relay_sha" \
  "$(sha256sum "$BIN/multiverse-relay" | awk '{print $1}')"
run_generation restore --generation "$generation" --bin-dir "$BIN" --store "$STORE" \
  >"$FIXTURE/restore.out"
assert_true "named restore installs the captured relay" \
  cmp -s "$FIXTURE/running-relay" "$BIN/multiverse-relay"
assert_true "named restore installs the captured archive" \
  cmp -s "$FIXTURE/running-archive" "$BIN/multiverse-archive"
assert_true "named restore installs the captured ringstat" \
  cmp -s "$STORE/$generation/ringstat" "$BIN/ringstat"
assert_true "restore does not call systemctl or restart a unit" test ! -s "$SYSTEMCTL_LOG"

echo
echo "== a generation without ringstat restores exact absence"
new_fixture no-ringstat
unlink "$BIN/ringstat"
out="$(run_generation capture --bin-dir "$BIN" --store "$STORE" --proc-root "$PROC")"
generation="$(printf '%s\n' "$out" | generation_from)"
printf 'new relay\n' >"$BIN/multiverse-relay"
printf 'later ringstat\n' >"$BIN/ringstat"
run_generation restore --dry-run --generation "$generation" --bin-dir "$BIN" --store "$STORE" \
  >"$FIXTURE/dry-restore.out"
assert_true "absence dry-run does not remove a later ringstat" test -f "$BIN/ringstat"
run_generation restore --generation "$generation" --bin-dir "$BIN" --store "$STORE" \
  >"$FIXTURE/restore.out"
assert_true "generation manifest records that ringstat was absent" \
  grep -Fxq $'ringstat_sha256\t-' "$STORE/$generation/manifest.tsv"
assert_true "restore removes a later ringstat to reproduce exact absence" \
  test ! -e "$BIN/ringstat"

echo
echo "== unsafe ringstat absence fails before replacement"
new_fixture unsafe-ringstat
unlink "$BIN/ringstat"
out="$(run_generation capture --bin-dir "$BIN" --store "$STORE" --proc-root "$PROC")"
generation="$(printf '%s\n' "$out" | generation_from)"
printf 'candidate relay must stay\n' >"$BIN/multiverse-relay"
candidate_relay_sha="$(sha256sum "$BIN/multiverse-relay" | awk '{print $1}')"
ln -s "$FIXTURE/running-relay" "$BIN/ringstat"
if run_generation restore --generation "$generation" --bin-dir "$BIN" --store "$STORE" \
    >"$FIXTURE/out" 2>&1; then
  bad "unsafe ringstat path is rejected"
else
  ok "unsafe ringstat path is rejected"
fi
assert_eq "unsafe ringstat rejection precedes service replacement" "$candidate_relay_sha" \
  "$(sha256sum "$BIN/multiverse-relay" | awk '{print $1}')"
assert_true "unsafe ringstat path is not followed or removed" test -L "$BIN/ringstat"

echo
echo "== tampering fails before replacement"
new_fixture tamper
out="$(run_generation capture --bin-dir "$BIN" --store "$STORE" --proc-root "$PROC")"
generation="$(printf '%s\n' "$out" | generation_from)"
printf 'tampered\n' >>"$STORE/$generation/multiverse-relay"
printf 'must stay installed\n' >"$BIN/multiverse-relay"
before_tamper_restore="$(sha256sum "$BIN/multiverse-relay" | awk '{print $1}')"
if run_generation restore --generation "$generation" --bin-dir "$BIN" --store "$STORE" \
    >"$FIXTURE/out" 2>&1; then
  bad "tampered generation is rejected"
else
  ok "tampered generation is rejected"
fi
assert_eq "tamper rejection occurs before replacement" "$before_tamper_restore" \
  "$(sha256sum "$BIN/multiverse-relay" | awk '{print $1}')"

echo
echo "== initial install has no false rollback generation"
FIXTURE="$TMP/initial"
BIN="$FIXTURE/bin"
PROC="$FIXTURE/proc"
STORE="$FIXTURE/store"
SYSTEMCTL_LOG="$FIXTURE/systemctl.log"
SYSTEMCTL_STATE="$FIXTURE/systemctl.state"
DD_STATE="$FIXTURE/dd.state"
mkdir -p "$BIN" "$PROC"
RELAY_PID=0
ARCHIVE_PID=0
out="$(run_generation capture --bin-dir "$BIN" --store "$STORE" --proc-root "$PROC")"
initial_evidence="$(printf '%s\n' "$out" | evidence_from)"
assert_eq "empty first install emits one complete terminal evidence pair" \
  $'rollback_generation=initial-install\ngeneration_status=initial-install' \
  "$initial_evidence"
assert_eq "initial-install status is the terminal output" \
  generation_status=initial-install "$(printf '%s\n' "$out" | tail -1)"
assert_true "initial install does not create a rollback store" test ! -e "$STORE"

echo
printf '%d pass, %d fail\n' "$PASS" "$FAIL"
[ "$FAIL" = 0 ] || exit 1
echo "binary-generation.sh OK"
