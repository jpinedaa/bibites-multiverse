#!/usr/bin/env bash
# Drive the sidecar-only updater through local files and deterministic service seams.
set -Eeuo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=update-sidecar.sh
source "$here/update-sidecar.sh"

work="$(mktemp -d)"
cleanup() {
  find "$work" -depth -delete >/dev/null 2>&1 || true
}
trap cleanup EXIT

fail() {
  printf 'test-update-sidecar: %s\n' "$*" >&2
  exit 1
}

old_revision='1111111111111111111111111111111111111111'
new_revision='2222222222222222222222222222222222222222'
peer_id='public-123456781234423482341234567890ab'
slot=7
fixture=''
fixture_root=''
candidate_file=''
old_sha=''
new_sha=''
process_state_file=''
events_file=''
status_counter_file=''
target_acl_file=''
custody_file=''
reparse_file=''
acl_drift_file=''
acl_drift_target=''
case_mode='success'
observation_mode='success'

write_state() {
  printf '%s\n' "$1" >"$process_state_file"
}

read_state() {
  tr -d '\r\n' <"$process_state_file"
}

record_event() {
  printf '%s\n' "$1" >>"$events_file"
}

prepare_fixture() {
  local name="$1" label path
  fixture="$work/$name"
  fixture_root="$fixture/root"
  candidate_file="$fixture/candidate/multiverse-sidecar.exe"
  config_root="$fixture/home/.config/bibites-local-broadcast"
  runtime_root="$fixture/home/.local/lib/bibites-local-broadcast"
  process_state_file="$fixture/process-state"
  events_file="$fixture/events"
  status_counter_file="$fixture/status-counter"
  target_acl_file="$fixture/target-acl"
  custody_file="$fixture_root/multiverse/data/custody-forward.log"
  reparse_file="$fixture/reparse-paths"
  acl_drift_file="$fixture/acl-drift"
  acl_drift_target="$fixture_root/run-windows.ps1"
  case_mode='success'
  observation_mode='success'
  observation_samples=21
  observation_interval_seconds=30
  health_attempts=1
  health_interval_seconds=0
  in_observation=0

  mkdir -p "$fixture_root" "$(dirname "$candidate_file")" "$config_root" "$runtime_root/bin"
  printf 'old sidecar fixture %s\n' "$name" >"$fixture_root/multiverse-sidecar.exe"
  printf 'new sidecar fixture %s\n' "$name" >"$candidate_file"
  old_sha="$(sha256sum "$fixture_root/multiverse-sidecar.exe" | awk '{print $1}')"
  new_sha="$(sha256sum "$candidate_file" | awk '{print $1}')"
  : >"$config_root/install.lock"
  : >"$events_file"
  : >"$reparse_file"
  printf '0\n' >"$status_counter_file"
  printf 'fixture-target-acl\n' >"$target_acl_file"
  write_state up

  root="$fixture_root"
  while IFS=$'\t' read -r label path; do
    mkdir -p "$(dirname "$path")"
    printf 'static fixture %s %s\n' "$name" "$label" >"$path"
  done < <(static_entries)
  printf 'forward custody 0\n' >"$custody_file"
}

# The fixtures replace only operating-system boundaries. File hashes, copies,
# transaction order, manifests, rollback, and observation rules remain real.
require_tools() { :; }

assert_exact_broadcast_root() {
  [ "$root" = "$(realpath -e "$fixture_root")" ] || die 'The fixture root is not exact'
}

assert_no_windows_reparse() {
  if grep -Fxq -- "$1" "$reparse_file"; then
    die "A fixture Windows reparse point is not permitted: $1"
  fi
}

capture_windows_file_metadata() {
  local path="$1" acl='fixture-static-acl'
  if [ -f "$acl_drift_file" ] && [ "$path" = "$acl_drift_target" ]; then
    acl='fixture-drifted-acl'
  fi
  printf '%s\t%s\n' "$(sha256sum "$path" | awk '{print $1}')" "$acl"
}

capture_windows_path_acl() {
  printf 'fixture-directory-acl\n'
}

get_windows_acl() {
  local acl
  acl="$(tr -d '\r\n' <"$target_acl_file")"
  if [[ "$acl" == *'D:AR'* ]]; then
    die 'The ACL seal has an unresolved DACL inheritance request'
    return 1
  fi
  printf '%s\n' "$acl"
}

set_windows_acl() {
  printf '%s\n' "$2" >"$target_acl_file"
  record_event set-acl
}

protect_windows_directory() {
  chmod 0700 "$1"
  find "$1" -type f -exec chmod 0600 {} +
  record_event protect-directory
}

atomic_replace_windows() {
  mv -f -- "$1" "$2"
  printf 'fixture-replacement-acl\n' >"$target_acl_file"
  record_event atomic-replace
  if [ "$case_mode" = replacement-failure ] &&
      [ "$(grep -c '^atomic-replace$' "$events_file")" -eq 1 ]; then
    return 1
  fi
}

validate_installed_configuration() {
  [ -n "${install_lock_fd:-}" ] || die 'Configuration validation ran without the installation lock'
}
assert_windows_executable() { :; }

read_vcs_metadata() {
  local hash modified=false
  hash="$(sha256sum "$1" | awk '{print $1}')"
  if [ "$hash" = "$old_sha" ]; then
    printf '%s\tfalse\n' "$old_revision"
    return 0
  fi
  if [ "$case_mode" = dirty-candidate ]; then modified=true; fi
  if [ "$hash" = "$new_sha" ]; then
    printf '%s\t%s\n' "$new_revision" "$modified"
    return 0
  fi
  printf 'unknown\ttrue\n'
}

get_windows_process_counts() {
  case "$(read_state)" in
    up) printf '%s\n' '{"game":1,"sidecar":1,"obs":1,"runner":1,"watcher":1}' ;;
    down) printf '%s\n' '{"game":0,"sidecar":0,"obs":0,"runner":0,"watcher":0}' ;;
    partial) printf '%s\n' '{"game":1,"sidecar":0,"obs":0,"runner":0,"watcher":0}' ;;
    *) return 1 ;;
  esac
}

installed_stop() {
  local stops
  record_event stop
  stops="$(grep -c '^stop$' "$events_file")"
  if { [ "$case_mode" = stop-failure ] && [ "$stops" -eq 1 ]; } ||
      [ "$case_mode" = persistent-stop-failure ]; then
    write_state partial
    return 1
  fi
  if [ "$case_mode" = static-hash-drift ] && [ "$stops" -eq 1 ]; then
    printf 'unexpected static mutation\n' >>"$fixture_root/game/BepInEx/plugins/BibitesMultiverse.dll"
  fi
  if [ "$case_mode" = static-acl-drift ] && [ "$stops" -eq 1 ]; then
    : >"$acl_drift_file"
  fi
  if [ "$case_mode" = target-hash-drift-after-stop ] && [ "$stops" -eq 1 ]; then
    printf 'concurrent target drift\n' >"$fixture_root/multiverse-sidecar.exe"
  fi
  if [ "$case_mode" = rollback-static-drift ] && [ "$stops" -eq 2 ]; then
    printf 'rollback-time static drift\n' >>"$fixture_root/run-windows.ps1"
  fi
  if [[ "$case_mode" == mutable-custody || "$case_mode" == replacement-failure ||
        "$case_mode" == start-failure || "$case_mode" == health-failure ]]; then
    printf 'forward custody %s\n' "$stops" >>"$custody_file"
  fi
  write_state down
}

installed_start() {
  local starts
  record_event start
  starts="$(grep -c '^start$' "$events_file")"
  if [ "$case_mode" = start-failure ] && [ "$starts" -eq 1 ]; then
    write_state down
    return 1
  fi
  if [ "$case_mode" = target-acl-drift ] && [ "$starts" -eq 1 ]; then
    printf 'fixture-target-acl-drifted\n' >"$target_acl_file"
  fi
  write_state up
}

tmux_session_exists() { [ "$(read_state)" = up ]; }

unit_is_active() { [ "$(read_state)" = up ]; }

sleep_update() {
  if [ "$case_mode" = term-during-observation ] && [ "$in_observation" -eq 1 ]; then
    case_mode=term-delivered
    kill -TERM "$BASHPID"
  fi
}

status_document() {
  local index="$1" connected="$2" total all_zero=false rate=0.2
  case "$observation_mode" in
    success)
      total=100
      [ "$index" -ge 5 ] && total=101
      [ "$index" -ge 15 ] && total=102
      [ "$index" -eq 8 ] && all_zero=true
      ;;
    repeated-zero)
      total=$((100 + index))
      if [ "$index" -eq 3 ] || [ "$index" -eq 4 ]; then all_zero=true; fi
      ;;
    no-first-progress)
      total=100
      [ "$index" -ge 15 ] && total=101
      ;;
    no-second-progress)
      total=100
      [ "$index" -ge 5 ] && total=101
      ;;
    *) return 1 ;;
  esac
  [ "$all_zero" = false ] || rate=0
  jq -nc --arg peer "$peer_id" --argjson slot "$slot" --argjson connected "$connected" \
    --argjson total "$total" --argjson rate "$rate" '
    {
      flowWindowMs: 300000,
      slots: [{
        slot: $slot,
        peerId: $peer,
        live: true,
        modConnected: $connected,
        statsKnown: true,
        population: 42,
        exportEdges: ["E", "N", "W", "S"]
      }],
      lanes: [
        {fromSlot: $slot, edge: "E", open: true, migrations: $total, perMinute: $rate},
        {fromSlot: $slot, edge: "N", open: true, migrations: 0, perMinute: 0},
        {fromSlot: $slot, edge: "S", open: true, migrations: 0, perMinute: 0},
        {fromSlot: $slot, edge: "W", open: false, migrations: 0, perMinute: 0}
      ]
    }'
}

fetch_status() {
  local index=0 connected=true current_hash
  if [ "$in_observation" -eq 1 ]; then
    index="$(tr -d '\r\n' <"$status_counter_file")"
    printf '%s\n' "$((index + 1))" >"$status_counter_file"
    if { [ "$case_mode" = target-acl-midpoint ] && [ "$index" -eq 10 ]; } ||
        { [ "$case_mode" = target-acl-final-boundary ] && [ "$index" -eq 20 ]; }; then
      printf 'fixture-target-acl-drifted\n' >"$target_acl_file"
    fi
  fi
  current_hash="$(sha256sum "$fixture_root/multiverse-sidecar.exe" | awk '{print $1}')"
  if [ "$case_mode" = target-acl-final-health ] && [ "$in_observation" -eq 0 ] &&
      [ "$current_hash" = "$new_sha" ] &&
      [ "$(tr -d '\r\n' <"$status_counter_file")" -eq 21 ]; then
    printf 'fixture-target-acl-drifted\n' >"$target_acl_file"
  fi
  if [[ "$case_mode" == health-failure || "$case_mode" == rollback-static-drift ]] &&
      [ "$current_hash" = "$new_sha" ]; then
    connected=false
  fi
  if [ "$case_mode" = rollback-health-failure ]; then
    if [ "$current_hash" = "$new_sha" ] ||
        [ "$(grep -c '^atomic-replace$' "$events_file")" -ge 2 ]; then
      connected=false
    fi
  fi
  status_document "$index" "$connected"
}

update_arguments() {
  printf '%s\0' \
    --root "$fixture_root" \
    --candidate "$candidate_file" \
    --expected-old-sha256 "$old_sha" \
    --expected-new-sha256 "$new_sha" \
    --expected-old-revision "$old_revision" \
    --expected-new-revision "$new_revision" \
    --expected-peer-id "$peer_id" \
    --expected-slot "$slot" \
    --status-url 'https://fixture.invalid/api/status'
}

run_status=0
run_update() {
  local output="$1" status
  local -a arguments=()
  shift
  while IFS= read -r -d '' argument; do arguments+=("$argument"); done < <(update_arguments)
  arguments+=("$@")
  set +e
  (
    set -Eeuo pipefail
    run_cli "${arguments[@]}"
  ) >"$output" 2>&1
  status=$?
  set -e
  run_status="$status"
  if grep -Fq -- "$peer_id" "$output"; then
    fail 'an updater receipt or error emitted the protected peer identity'
  fi
  return 0
}

assert_target_hash() {
  [ "$(sha256sum "$fixture_root/multiverse-sidecar.exe" | awk '{print $1}')" = "$1" ] ||
    fail "the installed target has the wrong hash in $fixture"
}

assert_executable_only_backup() {
  local count backup
  count="$(find "$fixture_root/.sidecar-update-backups" -type f | wc -l)"
  [ "$count" -eq 1 ] || fail "the backup contains $count files, not one executable"
  backup="$(find "$fixture_root/.sidecar-update-backups" -type f -name multiverse-sidecar.exe)"
  [ -n "$backup" ] || fail 'the protected backup does not contain the old sidecar executable'
  [ "$(sha256sum "$backup" | awk '{print $1}')" = "$old_sha" ] ||
    fail 'the protected backup does not match the old executable'
}

prepare_fixture success
run_update "$fixture/output"
[ "$run_status" -eq 0 ] || fail "the success fixture failed: $(cat "$fixture/output")"
assert_target_hash "$new_sha"
[ "$(get_windows_acl)" = 'fixture-target-acl' ] ||
  fail 'the success fixture did not restore the saved ACL seal after replacement'
assert_executable_only_backup
grep -Fq 'source flow advanced in both windows' "$fixture/output" ||
  fail 'the success fixture did not complete both observation windows'
grep -Fq '30-second waits between completed samples' "$fixture/output" ||
  fail 'the success fixture did not report the observation wait contract'
grep -Fq 'source-flow cumulative migrations start=100 midpoint=101 end=102' "$fixture/output" ||
  fail 'the success fixture did not print receipt-safe window boundary totals'
[ -z "$(find "$fixture_root" -maxdepth 1 -name '.sidecar-update-stage.*' -print -quit)" ] ||
  fail 'the success fixture kept a stage path'
printf 'success: PASS\n'

prepare_fixture dry-run
before="$(find "$fixture_root" "$config_root" "$runtime_root" -type f -print0 |
  sort -z | xargs -0 sha256sum)"
run_update "$fixture/output" --dry-run
[ "$run_status" -eq 0 ] || fail "the dry-run fixture failed: $(cat "$fixture/output")"
after="$(find "$fixture_root" "$config_root" "$runtime_root" -type f -print0 |
  sort -z | xargs -0 sha256sum)"
[ "$before" = "$after" ] || fail 'the dry-run fixture changed installed file content'
[ ! -e "$fixture_root/.sidecar-update-backups" ] || fail 'the dry-run fixture created a backup'
[ ! -s "$events_file" ] || fail 'the dry-run fixture stopped, started, replaced, or changed an ACL'
printf 'dry-run: PASS\n'

prepare_fixture target-acl-auto-inherit-request
crafted_ar_acl='O:S-1-5-18G:S-1-5-18D:AR(A;;FA;;;S-1-5-18)'
printf '%s\n' "$crafted_ar_acl" >"$target_acl_file"
run_update "$fixture/output"
[ "$run_status" -eq 20 ] ||
  fail "a D:AR target ACL returned status $run_status, not 20"
assert_target_hash "$old_sha"
[ "$(tr -d '\r\n' <"$target_acl_file")" = "$crafted_ar_acl" ] ||
  fail 'the D:AR preflight changed the target ACL seal'
[ ! -s "$events_file" ] ||
  fail 'the D:AR preflight stopped, replaced, or changed an ACL'
[ "$(read_state)" = up ] || fail 'the D:AR preflight changed the runtime state'
[ ! -e "$fixture_root/.sidecar-update-backups" ] ||
  fail 'the D:AR preflight created an executable backup'
grep -Fq 'unresolved DACL inheritance request' "$fixture/output" ||
  fail 'the D:AR preflight reported the wrong error'
printf 'DACL auto-inherit request rejection before mutation: PASS\n'

prepare_fixture legacy-missing-lock
find "$config_root" -maxdepth 1 -type f -name install.lock -delete
run_update "$fixture/output" --dry-run
[ "$run_status" -eq 0 ] || fail "the missing-lock dry-run failed: $(cat "$fixture/output")"
[ -f "$config_root/install.lock" ] || fail 'the updater did not create the shared installation lock'
[ ! -s "$config_root/install.lock" ] || fail 'the new installation lock is not empty'
[ ! -s "$events_file" ] || fail 'the missing-lock dry-run reached service or executable work'
printf 'legacy missing installation lock: PASS\n'

prepare_fixture stop-failure
case_mode='stop-failure'
run_update "$fixture/output"
[ "$run_status" -ne 0 ] || fail 'the stop-failure fixture succeeded'
[ "$run_status" -eq 30 ] || fail "a recovered stop failure returned status $run_status, not 30"
assert_target_hash "$old_sha"
grep -Fq stop "$events_file" || fail 'the stop-failure fixture did not call the installed stop script'
if grep -Fq atomic-replace "$events_file"; then fail 'a stop failure reached executable replacement'; fi
[ "$(grep -c '^stop$' "$events_file")" -eq 2 ] || fail 'the stop recovery did not use one bounded retry'
[ "$(read_state)" = up ] || fail 'the bounded stop recovery did not restart the verified old runtime'
printf 'stop failure: PASS\n'

prepare_fixture persistent-stop-failure
case_mode='persistent-stop-failure'
run_update "$fixture/output"
[ "$run_status" -ne 0 ] || fail 'the persistent-stop-failure fixture succeeded'
[ "$run_status" -eq 50 ] || fail "an unrecovered stop failure returned status $run_status, not 50"
assert_target_hash "$old_sha"
[ "$(grep -c '^stop$' "$events_file")" -eq 2 ] || fail 'persistent stop failure used more than one recovery retry'
if grep -q '^start$' "$events_file"; then fail 'persistent private processes were started over'; fi
[ "$(read_state)" = partial ] || fail 'the persistent stop fixture did not retain its evidence state'
printf 'persistent stop failure never starts over private processes: PASS\n'

for failure_mode in replacement-failure start-failure health-failure; do
  prepare_fixture "$failure_mode"
  case_mode="$failure_mode"
  run_update "$fixture/output"
  [ "$run_status" -ne 0 ] || fail "$failure_mode succeeded"
  [ "$run_status" -eq 40 ] || fail "$failure_mode returned status $run_status, not 40"
  assert_target_hash "$old_sha"
  assert_executable_only_backup
  grep -Fq 'the executable-only rollback is healthy' "$fixture/output" ||
    fail "$failure_mode did not complete a verified executable-only rollback: $(cat "$fixture/output")"
  [ "$(grep -c '^atomic-replace$' "$events_file")" -ge 2 ] ||
    fail "$failure_mode did not atomically restore the old executable"
  [ "$(wc -l <"$custody_file")" -gt 1 ] || fail "$failure_mode restored old custody data"
  printf '%s with executable-only rollback: PASS\n' "$failure_mode"
done

prepare_fixture term-during-observation
case_mode='term-during-observation'
run_update "$fixture/output"
[ "$run_status" -eq 40 ] ||
  fail "a terminated observation returned status $run_status, not 40"
assert_target_hash "$old_sha"
grep -Fq 'the executable-only rollback is healthy' "$fixture/output" ||
  fail 'a terminated observation did not complete executable-only rollback'
[ "$(grep -c '^atomic-replace$' "$events_file")" -ge 2 ] ||
  fail 'a terminated observation did not atomically restore the old executable'
printf 'termination during observation with executable-only rollback: PASS\n'

prepare_fixture mutable-custody
case_mode='mutable-custody'
custody_before="$(sha256sum "$custody_file" | awk '{print $1}')"
run_update "$fixture/output"
[ "$run_status" -eq 0 ] || fail "the mutable-custody fixture failed: $(cat "$fixture/output")"
custody_after="$(sha256sum "$custody_file" | awk '{print $1}')"
[ "$custody_before" != "$custody_after" ] || fail 'forward custody data did not advance during the update'
grep -Fq 'forward custody 1' "$custody_file" || fail 'the updater restored an old custody tree'
printf 'mutable custody stays forward: PASS\n'

prepare_fixture static-hash-drift
case_mode='static-hash-drift'
run_update "$fixture/output"
[ "$run_status" -ne 0 ] || fail 'the static-hash-drift fixture succeeded'
[ "$run_status" -eq 50 ] || fail "static hash drift returned status $run_status, not 50"
assert_target_hash "$old_sha"
if grep -Fq atomic-replace "$events_file"; then fail 'static hash drift reached executable replacement'; fi
[ "$(read_state)" = down ] || fail 'static hash drift restarted from an unsealed static file'
if grep -q '^start$' "$events_file"; then fail 'static hash drift restarted the runtime'; fi
printf 'static hash drift: PASS\n'

prepare_fixture static-acl-drift
case_mode='static-acl-drift'
run_update "$fixture/output"
[ "$run_status" -ne 0 ] || fail 'the static-ACL-drift fixture succeeded'
[ "$run_status" -eq 50 ] || fail "static ACL drift returned status $run_status, not 50"
assert_target_hash "$old_sha"
if grep -Fq atomic-replace "$events_file"; then fail 'static ACL drift reached executable replacement'; fi
[ "$(read_state)" = down ] || fail 'static ACL drift restarted from an unsealed static ACL'
if grep -q '^start$' "$events_file"; then fail 'static ACL drift restarted the runtime'; fi
printf 'static ACL drift: PASS\n'

prepare_fixture concurrent-target-drift
case_mode='target-hash-drift-after-stop'
run_update "$fixture/output"
[ "$run_status" -eq 50 ] || fail "concurrent target drift returned status $run_status, not 50"
if grep -Fq atomic-replace "$events_file"; then fail 'concurrent target drift reached replacement'; fi
[ "$(read_state)" = down ] || fail 'concurrent target drift restarted the unsealed executable'
if grep -q '^start$' "$events_file"; then fail 'concurrent target drift restarted the runtime'; fi
printf 'concurrent target drift before replacement: PASS\n'

prepare_fixture static-drift-during-rollback
case_mode='rollback-static-drift'
run_update "$fixture/output"
[ "$run_status" -eq 50 ] || fail "rollback static drift returned status $run_status, not 50"
assert_target_hash "$old_sha"
[ "$(read_state)" = down ] || fail 'rollback static drift restarted from an unsealed script'
[ "$(grep -c '^start$' "$events_file")" -eq 1 ] ||
  fail 'rollback static drift started the runtime after the seal failed'
grep -Fq 'rollback static-file proof failed before restart' "$fixture/output" ||
  fail 'rollback static drift did not report the failed pre-start proof'
printf 'rollback refuses to start from static drift: PASS\n'

prepare_fixture reparse-rejection
printf '%s\n' "$candidate_file" >"$reparse_file"
run_update "$fixture/output"
[ "$run_status" -ne 0 ] || fail 'the Windows reparse fixture succeeded'
[ "$run_status" -eq 20 ] || fail "reparse rejection returned status $run_status, not 20"
assert_target_hash "$old_sha"
[ ! -s "$events_file" ] || fail 'the Windows reparse fixture reached mutation work'
printf 'Windows reparse rejection: PASS\n'

prepare_fixture dirty-vcs
case_mode='dirty-candidate'
run_update "$fixture/output"
[ "$run_status" -ne 0 ] || fail 'the dirty VCS fixture succeeded'
[ "$run_status" -eq 20 ] || fail "dirty VCS rejection returned status $run_status, not 20"
assert_target_hash "$old_sha"
[ ! -s "$events_file" ] || fail 'the dirty VCS fixture reached mutation work'
printf 'clean VCS stamps: PASS\n'

prepare_fixture target-acl-after-start
case_mode='target-acl-drift'
run_update "$fixture/output"
[ "$run_status" -ne 0 ] || fail 'the target ACL drift fixture succeeded'
[ "$run_status" -eq 40 ] || fail "target ACL drift returned status $run_status, not 40"
assert_target_hash "$old_sha"
grep -Fq 'the executable-only rollback is healthy' "$fixture/output" ||
  fail 'target ACL drift did not trigger a verified executable-only rollback'
[ "$(get_windows_acl)" = 'fixture-target-acl' ] || fail 'rollback did not restore the exact target ACL'
printf 'post-start target ACL drift: PASS\n'

prepare_fixture target-acl-at-midpoint
case_mode='target-acl-midpoint'
run_update "$fixture/output"
[ "$run_status" -eq 40 ] || fail "midpoint ACL drift returned status $run_status, not 40"
assert_target_hash "$old_sha"
grep -Fq 'the executable-only rollback is healthy' "$fixture/output" ||
  fail 'midpoint ACL drift did not trigger a verified executable-only rollback'
[ "$(get_windows_acl)" = 'fixture-target-acl' ] ||
  fail 'midpoint rollback did not restore the exact target ACL'
printf 'observation-boundary target ACL drift: PASS\n'

prepare_fixture target-acl-at-final-boundary
case_mode='target-acl-final-boundary'
run_update "$fixture/output"
[ "$run_status" -eq 40 ] || fail "final-boundary ACL drift returned status $run_status, not 40"
assert_target_hash "$old_sha"
[ "$(get_windows_acl)" = 'fixture-target-acl' ] ||
  fail 'final-boundary rollback did not restore the exact target ACL'
printf 'final observation-boundary target ACL drift: PASS\n'

prepare_fixture target-acl-at-final-health
case_mode='target-acl-final-health'
run_update "$fixture/output"
[ "$run_status" -eq 40 ] || fail "final-health ACL drift returned status $run_status, not 40"
assert_target_hash "$old_sha"
[ "$(get_windows_acl)" = 'fixture-target-acl' ] ||
  fail 'final-health rollback did not restore the exact target ACL'
printf 'final-health target ACL drift: PASS\n'

prepare_fixture failed-rollback-recovery
case_mode='rollback-health-failure'
run_update "$fixture/output"
[ "$run_status" -eq 50 ] || fail "a failed rollback returned status $run_status, not 50"
assert_target_hash "$old_sha"
grep -Fq 'rollback did not reach a verified state' "$fixture/output" ||
  fail 'the failed rollback did not report a critical unreconciled state'
printf 'failed rollback recovery status: PASS\n'

for malformed in \
    '--expected-old-sha256 bad' \
    '--expected-new-revision short' \
    '--expected-peer-id private-not-allowed' \
    '--expected-slot 0' \
    '--status-url http://fixture.invalid/api/status'; do
  prepare_fixture malformed-input
  read -r malformed_name malformed_value <<<"$malformed"
  run_update "$fixture/output" "$malformed_name" "$malformed_value"
  [ "$run_status" -eq 20 ] ||
    fail "$malformed_name malformed input returned status $run_status, not 20"
  [ ! -s "$events_file" ] || fail "$malformed_name malformed input reached mutation work"
done
prepare_fixture protected-identity-as-unknown-argument
run_update "$fixture/output" "$peer_id"
[ "$run_status" -eq 20 ] || fail "an unknown argument returned status $run_status, not 20"
[ ! -s "$events_file" ] || fail 'an unknown argument reached mutation work'
prepare_fixture missing-option-value
run_update "$fixture/output" --expected-peer-id
[ "$run_status" -eq 20 ] || fail "a missing option value returned status $run_status, not 20"
[ ! -s "$events_file" ] || fail 'a missing option value reached mutation work'
printf 'malformed transaction inputs: PASS\n'

for mismatch in old-hash new-hash old-revision new-revision; do
  prepare_fixture "$mismatch-mismatch"
  case "$mismatch" in
    old-hash) mismatch_arguments=(--expected-old-sha256 "$(printf '0%.0s' {1..64})") ;;
    new-hash) mismatch_arguments=(--expected-new-sha256 "$(printf 'f%.0s' {1..64})") ;;
    old-revision) mismatch_arguments=(--expected-old-revision "$(printf '3%.0s' {1..40})") ;;
    new-revision) mismatch_arguments=(--expected-new-revision "$(printf '4%.0s' {1..40})") ;;
  esac
  run_update "$fixture/output" "${mismatch_arguments[@]}"
  [ "$run_status" -eq 20 ] || fail "$mismatch returned status $run_status, not 20"
  [ ! -s "$events_file" ] || fail "$mismatch reached mutation work"
done
printf 'sealed hash and VCS-revision mismatches: PASS\n'

prepare_fixture windows-helper-seal-drift
stage_windows_helper || fail 'the Windows helper seal fixture could not stage the helper'
chmod 0700 "$sealed_windows_helper"
printf '# concurrent helper drift\n' >>"$sealed_windows_helper"
function powershell.exe { record_event powershell-invoked; }
set +e
run_windows_helper -Operation AssertNoReparse -Path 'fixture' >"$fixture/output" 2>&1
helper_status=$?
set -e
unset -f powershell.exe
[ "$helper_status" -ne 0 ] || fail 'a changed sealed Windows helper was accepted'
if grep -Fq powershell-invoked "$events_file"; then
  fail 'a changed sealed Windows helper reached PowerShell execution'
fi
cleanup_stage
printf 'Windows helper seal before every operation: PASS\n'

prepare_fixture conditional-guards
target="$fixture_root/multiverse-sidecar.exe"
target_acl="$(get_windows_acl)"
expected_peer_id="$peer_id"
expected_slot="$slot"
status_url='https://fixture.invalid/api/status'
if assert_runtime_health "$new_sha" "$new_revision" >/dev/null 2>&1; then
  fail 'an early target-hash failure was masked by later healthy checks'
fi
write_state partial
if assert_runtime_stopped >/dev/null 2>&1; then
  fail 'an early nonzero process count was masked by later stopped supervisors'
fi
write_state up
first_static="$(static_entries | head -1 | cut -f2)"
mv "$first_static" "$first_static.missing"
if static_probe="$(capture_static_manifest 2>/dev/null)"; then
  fail 'an early static-file failure was masked by later manifest entries'
fi
[ -z "${static_probe:-}" ] || fail 'a failed static manifest emitted a partial success manifest'
printf 'conditional-context validation guards: PASS\n'

prepare_fixture observation-logic
target="$fixture_root/multiverse-sidecar.exe"
target_acl="$(get_windows_acl)"
expected_peer_id="$peer_id"
expected_slot="$slot"
status_url='https://fixture.invalid/api/status'
for observation_mode in success; do
  printf '0\n' >"$status_counter_file"
  observe_source_flow >"$fixture/observation-output" ||
    fail 'the valid observation fixture failed with one naturally closed lane'
done
for observation_mode in repeated-zero no-first-progress no-second-progress; do
  printf '0\n' >"$status_counter_file"
  set +e
  ( set -e; observe_source_flow ) >"$fixture/observation-output" 2>&1
  observation_status=$?
  set -e
  [ "$observation_status" -ne 0 ] || fail "the $observation_mode observation fixture succeeded"
done
printf 'two-window observation logic and naturally closed lanes: PASS\n'

printf 'sidecar-only preflight, replacement, rollback, preservation, and flow observation: PASS\n'
