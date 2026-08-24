#!/usr/bin/env bash
# Replace only the executable for one installed local-broadcast sidecar.
set -Eeuo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
windows_helper_source="$here/update-sidecar-windows.ps1"
expected_windows_helper_sha='70197df322e21649b17cc4b297f1c7c586e32a27ec56d383b9bffb94ead3f921'
config_root="${XDG_CONFIG_HOME:-$HOME/.config}/bibites-local-broadcast"
runtime_root="$HOME/.local/lib/bibites-local-broadcast"

observation_samples=21
observation_interval_seconds=30
health_attempts=120
health_interval_seconds=2
in_observation=0

root=''
candidate=''
expected_old_sha=''
expected_new_sha=''
expected_old_revision=''
expected_new_revision=''
expected_peer_id=''
expected_slot=''
status_url=''
dry_run=0
help_requested=0

target=''
target_acl=''
static_baseline=''
stage_dir=''
helper_stage_dir=''
sealed_windows_helper=''
backup_dir=''
backup_file=''
install_lock_fd=''
rollback_needed=0
phase='preflight'

say() { printf '[sidecar-update] %s\n' "$*"; }
die() { printf '[sidecar-update] ERROR: %s\n' "$*" >&2; return 1; }

usage() {
  printf '%s\n' \
    'Usage: update-sidecar.sh --root PATH --candidate FILE' \
    '  --expected-old-sha256 SHA256 --expected-new-sha256 SHA256' \
    '  --expected-old-revision COMMIT --expected-new-revision COMMIT' \
    '  --expected-peer-id PEER --expected-slot SLOT --status-url HTTPS_URL [--dry-run]'
}

require_tools() {
  local command
  for command in awk curl file find flock go head install jq mktemp powershell.exe realpath \
      sed seq sha256sum sleep stat systemctl tmux tr wslpath; do
    if ! command -v "$command" >/dev/null; then
      die "The required command is missing: $command"
      return 1
    fi
  done
}

assert_sha256() {
  if [[ ! "$1" =~ ^[0-9a-f]{64}$ ]]; then
    die "$2 must be one lowercase SHA-256 value"
    return 1
  fi
}

assert_revision() {
  if [[ ! "$1" =~ ^[0-9a-f]{40}$ ]]; then
    die "$2 must be one lowercase 40-character Git revision"
    return 1
  fi
}

assert_no_unix_symlink_path() {
  local path="$1" current='/' part
  if [[ "$path" != /* ]]; then
    die "The path is not absolute: $path"
    return 1
  fi
  IFS='/' read -r -a parts <<<"${path#/}"
  for part in "${parts[@]}"; do
    [ -n "$part" ] || continue
    if [ "$current" = / ]; then
      current="/$part"
    else
      current="$current/$part"
    fi
    if [ -L "$current" ]; then
      die "A symbolic link is not permitted: $current"
      return 1
    fi
  done
}

windows_path() {
  wslpath -w "$1" | tr -d '\r'
}

assert_windows_helper_seal() {
  local actual resolved
  if [ -z "$sealed_windows_helper" ] || [ ! -f "$sealed_windows_helper" ]; then
    die 'The sealed Windows helper is missing'
    return 1
  fi
  assert_no_unix_symlink_path "$sealed_windows_helper" || return 1
  resolved="$(realpath -e -- "$sealed_windows_helper")" || return 1
  if [ "$resolved" != "$sealed_windows_helper" ]; then
    die 'The sealed Windows helper path changed'
    return 1
  fi
  actual="$(sha256sum "$sealed_windows_helper" | awk '{print $1}')" || return 1
  if [ "$actual" != "$expected_windows_helper_sha" ]; then
    die 'The sealed Windows helper hash changed'
    return 1
  fi
}

stage_windows_helper() {
  local staged actual
  assert_no_unix_symlink_path "$windows_helper_source" || return 1
  if [ ! -f "$windows_helper_source" ]; then
    die 'The checked-in Windows helper is missing'
    return 1
  fi
  helper_stage_dir="$(mktemp -d /tmp/bibites-sidecar-helper.XXXXXX)" || return 1
  chmod 0700 "$helper_stage_dir" || return 1
  staged="$helper_stage_dir/update-sidecar-windows.ps1"
  install -m 0500 "$windows_helper_source" "$staged" || return 1
  sealed_windows_helper="$(realpath -e -- "$staged")" || return 1
  actual="$(sha256sum "$sealed_windows_helper" | awk '{print $1}')" || return 1
  if [ "$actual" != "$expected_windows_helper_sha" ]; then
    die 'The checked-in Windows helper does not match the updater seal'
    return 1
  fi
  assert_windows_helper_seal || return 1
  assert_no_windows_reparse "$sealed_windows_helper" || return 1
}

run_windows_helper() {
  local helper_windows
  assert_windows_helper_seal || return 1
  helper_windows="$(windows_path "$sealed_windows_helper")"
  # WSL exposes the staged helper through a UNC path. RemoteSigned rejects
  # an unsigned script at that path. Bypass applies only to this child process;
  # -File still pins the exact helper that this updater selected.
  powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$helper_windows" "$@" |
    tr -d '\r'
}

assert_exact_broadcast_root() {
  run_windows_helper -Operation AssertRoot -Root "$(windows_path "$root")" >/dev/null
}

assert_no_windows_reparse() {
  run_windows_helper -Operation AssertNoReparse -Path "$(windows_path "$1")" >/dev/null
}

capture_windows_file_metadata() {
  run_windows_helper -Operation FileMetadata -Path "$(windows_path "$1")"
}

capture_windows_path_acl() {
  run_windows_helper -Operation PathAcl -Path "$(windows_path "$1")"
}

get_windows_acl() {
  run_windows_helper -Operation GetAcl -Path "$(windows_path "$1")"
}

set_windows_acl() {
  run_windows_helper -Operation SetAcl -Path "$(windows_path "$1")" \
    -AclBase64 "$2" >/dev/null
}

protect_windows_directory() {
  run_windows_helper -Operation ProtectDirectory -Path "$(windows_path "$1")" >/dev/null
}

atomic_replace_windows() {
  run_windows_helper -Operation AtomicReplace -Source "$(windows_path "$1")" \
    -Destination "$(windows_path "$2")" >/dev/null
}

validate_installed_configuration() {
  run_windows_helper -Operation ValidateInstall -Root "$(windows_path "$root")" \
    -ExpectedPeerId "$expected_peer_id" >/dev/null
}

get_windows_process_counts() {
  run_windows_helper -Operation ProcessCounts -Root "$(windows_path "$root")"
}

assert_windows_executable() {
  if ! file "$1" | grep -Fq 'PE32+ executable'; then
    die "The candidate is not a Windows PE32+ executable: $1"
    return 1
  fi
}

read_vcs_metadata() {
  local path="$1" info revision modified
  if ! info="$(go version -m "$path" 2>/dev/null)"; then
    die "Go cannot read the VCS stamp from $path"
    return 1
  fi
  revision="$(printf '%s\n' "$info" |
    sed -n 's/^[[:space:]]*build[[:space:]]*vcs\.revision=//p' | head -1)"
  modified="$(printf '%s\n' "$info" |
    sed -n 's/^[[:space:]]*build[[:space:]]*vcs\.modified=//p' | head -1)"
  printf '%s\t%s\n' "$revision" "$modified"
}

static_entries() {
  printf '%s\t%s\n' \
    identity-record "$root/multiverse/enrollment.json" \
    identity-secret "$root/multiverse/peer-secret.txt" \
    identity-peer "$root/multiverse/data/peer-id" \
    identity-contract-a-token "$root/multiverse/data/contract-a.token" \
    plugin "$root/game/BepInEx/plugins/BibitesMultiverse.dll" \
    windows-config "$root/config.env" \
    windows-runner "$root/run-windows.ps1" \
    windows-stopper "$root/stop-windows.ps1" \
    windows-watcher "$root/watch-viewers.ps1" \
    obs-profile "$root/obs/config/obs-studio/basic/profiles/BibitesBroadcast/basic.ini" \
    obs-encoder "$root/obs/config/obs-studio/basic/profiles/BibitesBroadcast/streamEncoder.json" \
    obs-publish "$root/obs/config/obs-studio/basic/profiles/BibitesBroadcast/service.json" \
    obs-scene "$root/obs/config/obs-studio/basic/scenes/BibitesBroadcast.json" \
    obs-websocket "$root/obs/config/obs-studio/plugin_config/obs-websocket/config.json" \
    runtime-config "$config_root/broadcast.env" \
    runtime-loop "$runtime_root/bin/run-loop" \
    runtime-tunnel "$runtime_root/bin/run-tunnel" \
    runtime-windows "$runtime_root/bin/run-windows" \
    runtime-start "$runtime_root/bin/start" \
    runtime-stop "$runtime_root/bin/stop" \
    runtime-stop-windows "$runtime_root/bin/stop-windows"
}

static_directory_entries() {
  printf '%s\t%s\n' \
    broadcast-root "$root" \
    identity-root "$root/multiverse" \
    identity-data "$root/multiverse/data"
}

capture_static_manifest() {
  local label path metadata acl
  while IFS=$'\t' read -r label path; do
    [ -f "$path" ] || { die "The required static file is missing: $label"; return 1; }
    assert_no_unix_symlink_path "$path" || return 1
    assert_no_windows_reparse "$path" || return 1
    metadata="$(capture_windows_file_metadata "$path")" || return 1
    [[ "$metadata" == *$'\t'* ]] || {
      die "Windows returned invalid metadata for $label"
      return 1
    }
    printf '%s\t%s\n' "$label" "$metadata"
  done < <(static_entries)
  while IFS=$'\t' read -r label path; do
    [ -d "$path" ] || { die "The required static directory is missing: $label"; return 1; }
    assert_no_unix_symlink_path "$path" || return 1
    assert_no_windows_reparse "$path" || return 1
    acl="$(capture_windows_path_acl "$path")" || return 1
    [ -n "$acl" ] || { die "Windows returned an empty directory ACL for $label"; return 1; }
    printf '%s\t%s\n' "$label" "directory-$acl"
  done < <(static_directory_entries)
}

assert_static_unchanged() {
  local current
  current="$(capture_static_manifest)" || return 1
  [ "$current" = "$static_baseline" ] || \
    { die 'A static identity, plugin, script, or OBS configuration hash or ACL changed'; return 1; }
}

acquire_install_lock() {
  local lock="$config_root/install.lock"
  if [ ! -d "$config_root" ]; then
    die "The installed configuration directory is missing: $config_root"
    return 1
  fi
  assert_no_unix_symlink_path "$config_root" || return 1
  assert_no_windows_reparse "$config_root" || return 1
  if [ ! -e "$lock" ]; then
    (umask 077; : >>"$lock") || return 1
  fi
  if [ ! -f "$lock" ]; then
    die "The installation lock is not a regular file: $lock"
    return 1
  fi
  assert_no_unix_symlink_path "$lock" || return 1
  assert_no_windows_reparse "$lock" || return 1
  exec {install_lock_fd}<>"$lock" || return 1
  if [ "$(stat -Lc '%d:%i' "$lock")" != "$(stat -Lc '%d:%i' "/proc/self/fd/$install_lock_fd")" ]; then
    die 'The installation lock path changed while it was opened'
    return 1
  fi
  if ! flock -n "$install_lock_fd"; then
    die 'Another local-broadcast install or update holds the install lock'
    return 1
  fi
}

installed_stop() {
  "$runtime_root/bin/stop"
}

installed_start() {
  "$runtime_root/bin/start" {install_lock_fd}>&-
}

tmux_session_exists() {
  tmux -L bibites-broadcast has-session -t bibites-local-broadcast 2>/dev/null
}

unit_is_active() {
  systemctl --user is-active --quiet "$1"
}

assert_private_process_counts() {
  local wanted="$1" counts
  counts="$(get_windows_process_counts)" || return 1
  if ! jq -e --argjson wanted "$wanted" '
    (.game | type == "number") and (.game == $wanted) and
    (.sidecar | type == "number") and (.sidecar == $wanted) and
    (.obs | type == "number") and (.obs == $wanted) and
    (.runner | type == "number") and (.runner == $wanted) and
    (.watcher | type == "number") and (.watcher == $wanted)
  ' <<<"$counts" >/dev/null; then
    die "The private Windows process counts are not all $wanted"
    return 1
  fi
}

assert_runtime_stopped() {
  assert_private_process_counts 0 || return 1
  if tmux_session_exists; then
    die 'The private tmux supervisor is still active'
    return 1
  fi
  if unit_is_active bibites-local-broadcast-tunnel.service; then
    die 'The private RTMP tunnel is still active'
    return 1
  fi
  if unit_is_active bibites-local-broadcast.target; then
    die 'The local-broadcast systemd target is still active'
    return 1
  fi
}

fetch_status() {
  curl -fsS --max-time 10 --proto '=https' "$status_url"
}

assert_status_health() {
  jq -e --arg peer "$expected_peer_id" --argjson slot "$expected_slot" '
    (.flowWindowMs == 300000) and
    ([.slots[]? | select(.slot == $slot and .peerId == $peer)] | length == 1) and
    any(.slots[]?;
      .slot == $slot and .peerId == $peer and .live == true and
      .modConnected == true and .statsKnown == true and
      (.population | type == "number") and .population > 0 and
      ((.exportEdges // []) | sort) == (["E", "N", "W", "S"] | sort))
  ' >/dev/null
}

assert_target_acl_unchanged() {
  local current_acl
  current_acl="$(get_windows_acl "$target")" || return 1
  if [ "$current_acl" != "$target_acl" ]; then
    die 'The installed sidecar ACL does not match the saved preflight ACL'
    return 1
  fi
}

assert_target_identity() {
  local expected_sha="$1" expected_revision="$2" actual metadata revision modified
  actual="$(sha256sum "$target" | awk '{print $1}')" || {
    die 'The installed sidecar hash is unreadable'
    return 1
  }
  if [ "$actual" != "$expected_sha" ]; then
    die 'The installed sidecar hash does not match the active transaction'
    return 1
  fi
  metadata="$(read_vcs_metadata "$target")" || return 1
  IFS=$'\t' read -r revision modified <<<"$metadata"
  if [ "$revision" != "$expected_revision" ] || [ "$modified" != false ]; then
    die 'The installed sidecar VCS stamp does not match the active transaction'
    return 1
  fi
  assert_target_acl_unchanged || return 1
}

assert_runtime_health() {
  local expected_sha="$1" expected_revision="$2" status
  assert_target_identity "$expected_sha" "$expected_revision" || return 1
  assert_private_process_counts 1 || return 1
  if ! tmux_session_exists; then
    die 'The private tmux supervisor is not active'
    return 1
  fi
  if ! unit_is_active bibites-local-broadcast-tunnel.service; then
    die 'The private RTMP tunnel is not active'
    return 1
  fi
  if ! unit_is_active bibites-local-broadcast.target; then
    die 'The local-broadcast systemd target is not active'
    return 1
  fi
  status="$(fetch_status)" || return 1
  if ! printf '%s\n' "$status" | assert_status_health; then
    die "The map does not show the sealed identity in slot $expected_slot with the required state"
    return 1
  fi
  assert_target_acl_unchanged || return 1
}

sleep_update() {
  sleep "$1"
}

wait_for_runtime_health() {
  local expected_sha="$1" expected_revision="$2" attempt
  for attempt in $(seq 1 "$health_attempts"); do
    if assert_runtime_health "$expected_sha" "$expected_revision" >/dev/null 2>&1; then
      assert_runtime_health "$expected_sha" "$expected_revision" || return 1
      return 0
    fi
    [ "$attempt" -eq "$health_attempts" ] || sleep_update "$health_interval_seconds"
  done
  die 'The local-broadcast runtime did not reach the required health state'
  return 1
}

status_source_sample() {
  jq -er --arg peer "$expected_peer_id" --argjson slot "$expected_slot" '
    if .flowWindowMs != 300000 then error("unexpected flow window") else . end |
    ([.slots[]? | select(.slot == $slot and .peerId == $peer)]) as $slots |
    if ($slots | length) != 1 then error("the source slot is not unique") else . end |
    ($slots[0]) as $source |
    if ($source.live != true or $source.modConnected != true or $source.statsKnown != true or
        (($source.population | type) != "number") or $source.population <= 0 or
        (($source.exportEdges // []) | sort) != (["E", "N", "W", "S"] | sort))
      then error("the source slot is not live, populated, and ready") else . end |
    ([.lanes[]? | select(.fromSlot == $slot and .open == true)]) as $open |
    if ($open | length) == 0 then error("the source has no naturally open lane") else . end |
    if (($open | map(.edge) | unique | length) != ($open | length))
      then error("the source has duplicate open lanes") else . end |
    if any($open[]; ((.edge | type) != "string") or
                     (.edge as $edge | (["E", "N", "W", "S"] | index($edge)) == null) or
                     ((.migrations | type) != "number") or .migrations < 0 or
                     (.migrations | floor) != .migrations or
                     ((.perMinute | type) != "number") or .perMinute < 0)
      then error("an open lane has invalid flow values") else . end |
    [
      ($open | map(.edge) | sort | join(",")),
      ($open | map(.migrations) | add),
      (all($open[]; .perMinute == 0))
    ] | @tsv
  '
}

observe_source_flow() {
  local planned_seconds midpoint index sample edges total all_zero previous_zero=false
  local expected_edges=''
  local -a totals=()

  if [ "$observation_samples" -lt 3 ] || [ $((observation_samples % 2)) -ne 1 ]; then
    die 'The observation sample count must be an odd number of at least three'
    return 1
  fi
  midpoint=$(((observation_samples - 1) / 2))
  planned_seconds=$((midpoint * observation_interval_seconds))
  if [ "$planned_seconds" -lt 300 ]; then
    die 'Each source-flow window must span at least five minutes'
    return 1
  fi

  say 'observing two non-overlapping five-minute source-flow windows'
  in_observation=1
  for index in $(seq 0 $((observation_samples - 1))); do
    if [ "$index" -ne 0 ]; then
      sleep_update "$observation_interval_seconds" || return 1
    fi
    if ! sample="$(fetch_status | status_source_sample)"; then
      die 'The source-flow sample did not satisfy the slot and lane contract'
      return 1
    fi
    IFS=$'\t' read -r edges total all_zero <<<"$sample"
    if [ -z "$expected_edges" ]; then
      expected_edges="$edges"
    else
      if [ "$edges" != "$expected_edges" ]; then
        die 'The naturally open lane set changed during the source-flow observation'
        return 1
      fi
    fi
    if [ "$all_zero" = true ] && [ "$previous_zero" = true ]; then
      die 'Every naturally open outbound lane reported zero in repeated samples'
      return 1
    fi
    previous_zero="$all_zero"
    totals[index]="$total"
    if [ "$index" -eq 0 ] || [ "$index" -eq "$midpoint" ] ||
        [ "$index" -eq $((observation_samples - 1)) ]; then
      assert_target_acl_unchanged || return 1
    fi
  done

  say "source-flow cumulative migrations start=${totals[0]} midpoint=${totals[midpoint]} end=${totals[observation_samples - 1]}"
  if ! jq -en --argjson start "${totals[0]}" --argjson end "${totals[midpoint]}" \
      '$end > $start' >/dev/null; then
    die 'The source made no cumulative migration progress in the first five-minute window'
    return 1
  fi
  if ! jq -en --argjson start "${totals[midpoint]}" \
      --argjson end "${totals[observation_samples - 1]}" '$end > $start' >/dev/null; then
    die 'The source made no cumulative migration progress in the second five-minute window'
    return 1
  fi
  in_observation=0
  say "source flow advanced in both windows across open edges $expected_edges"
}

cleanup_stage() {
  if [ -n "$stage_dir" ] && [ -d "$stage_dir" ]; then
    find "$stage_dir" -depth -delete >/dev/null 2>&1 || true
  fi
  if [ -n "$helper_stage_dir" ] && [ -d "$helper_stage_dir" ]; then
    find "$helper_stage_dir" -depth -delete >/dev/null 2>&1 || true
  fi
}

restore_old_executable() {
  local rollback_stage="$stage_dir/multiverse-sidecar.rollback.exe" metadata revision modified
  say 'restoring only the old sidecar executable'
  if ! assert_static_unchanged; then
    say 'rollback refused to run the installed stopper because a static-file seal changed'
    return 1
  fi
  installed_stop >/dev/null 2>&1 || true
  if ! assert_runtime_stopped; then say 'rollback stop proof failed'; return 1; fi
  if ! assert_static_unchanged; then
    say 'rollback detected a static-file change after stop; it will restore the executable but not restart'
  fi
  if ! install -m 0600 "$backup_file" "$rollback_stage"; then say 'rollback staging failed'; return 1; fi
  if [ "$(sha256sum "$rollback_stage" | awk '{print $1}')" != "$expected_old_sha" ]; then
    say 'rollback staging produced the wrong old executable hash'
    return 1
  fi
  if ! atomic_replace_windows "$rollback_stage" "$target"; then say 'rollback replacement failed'; return 1; fi
  if ! set_windows_acl "$target" "$target_acl"; then say 'rollback ACL restore failed'; return 1; fi
  if [ "$(get_windows_acl "$target")" != "$target_acl" ]; then
    say 'rollback ACL proof failed'
    return 1
  fi
  if ! metadata="$(read_vcs_metadata "$target")"; then say 'rollback VCS-stamp read failed'; return 1; fi
  IFS=$'\t' read -r revision modified <<<"$metadata"
  if [ "$revision" != "$expected_old_revision" ] || [ "$modified" != false ]; then
    say 'rollback VCS-stamp proof failed'
    return 1
  fi
  if [ "$(sha256sum "$target" | awk '{print $1}')" != "$expected_old_sha" ]; then
    say 'rollback installed-hash proof failed'
    return 1
  fi
  if ! assert_static_unchanged; then
    say 'rollback static-file proof failed before restart'
    return 1
  fi
  if ! installed_start; then say 'rollback start failed'; return 1; fi
  if ! wait_for_runtime_health "$expected_old_sha" "$expected_old_revision"; then
    say 'rollback health proof failed'
    return 1
  fi
  if ! assert_static_unchanged; then say 'rollback static-file proof failed'; return 1; fi
  say 'the executable-only rollback is healthy; custody and save data stayed forward'
}

recover_old_runtime_without_replacement() {
  say 'checking the old-runtime seals before bounded recovery'
  if ! assert_target_identity "$expected_old_sha" "$expected_old_revision"; then
    say 'the old runtime was not recovered because its executable seal changed'
    return 1
  fi
  if ! assert_static_unchanged; then
    say 'the old runtime was not recovered because a static-file seal changed'
    return 1
  fi
  if ! assert_runtime_stopped >/dev/null 2>&1; then
    say 'running one bounded stop retry before old-runtime recovery'
    installed_stop >/dev/null 2>&1 || true
  fi
  if ! assert_runtime_stopped; then
    say 'the old runtime was not restarted because private processes are still active'
    return 1
  fi
  if ! assert_target_identity "$expected_old_sha" "$expected_old_revision"; then
    say 'the old runtime was not restarted because its executable seal changed'
    return 1
  fi
  if ! assert_static_unchanged; then
    say 'the old runtime was not restarted because a static-file seal changed'
    return 1
  fi
  installed_start || return 1
  wait_for_runtime_health "$expected_old_sha" "$expected_old_revision" || return 1
  assert_static_unchanged || return 1
  say 'the unchanged old runtime recovered after the failed pre-replacement stop'
}

on_error() {
  local original_status="$1" exit_status
  trap - ERR HUP INT QUIT TERM
  set +e
  exit_status="$original_status"
  if [ "$rollback_needed" -eq 1 ]; then
    if restore_old_executable; then
      exit_status=40
    else
      printf '[sidecar-update] ERROR: The executable-only rollback did not reach a verified state\n' >&2
      exit_status=50
    fi
  else
    case "$phase" in
      preflight) exit_status=20 ;;
      stopping|stopped)
        if recover_old_runtime_without_replacement; then
          exit_status=30
        else
          printf '[sidecar-update] ERROR: The old runtime did not reach a verified recovery state\n' >&2
          exit_status=50
        fi
        ;;
      *) [ "$exit_status" -ne 0 ] || exit_status=1 ;;
    esac
  fi
  cleanup_stage
  exit "$exit_status"
}

parse_arguments() {
  while [ $# -gt 0 ]; do
    case "$1" in
      --root|--candidate|--expected-old-sha256|--expected-new-sha256|\
      --expected-old-revision|--expected-new-revision|--expected-peer-id|\
      --expected-slot|--status-url)
        if [ "$#" -lt 2 ]; then
          die "The option needs a value: $1"
          return 1
        fi
        ;;
    esac
    case "$1" in
      --root) root="$2"; shift ;;
      --candidate) candidate="$2"; shift ;;
      --expected-old-sha256) expected_old_sha="$2"; shift ;;
      --expected-new-sha256) expected_new_sha="$2"; shift ;;
      --expected-old-revision) expected_old_revision="$2"; shift ;;
      --expected-new-revision) expected_new_revision="$2"; shift ;;
      --expected-peer-id) expected_peer_id="$2"; shift ;;
      --expected-slot) expected_slot="$2"; shift ;;
      --status-url) status_url="$2"; shift ;;
      --dry-run) dry_run=1 ;;
      -h|--help) help_requested=1 ;;
      *) die 'The command has an unknown argument'; return 1 ;;
    esac
    shift
  done
}

main() {
  local old_actual new_actual metadata revision modified backup_parent staged_candidate
  parse_arguments "$@" || return 1
  if [ "$help_requested" -eq 1 ]; then usage; return 0; fi
  require_tools || return 1

  if [ -z "$root" ] || [ -z "$candidate" ]; then
    die 'Both --root and --candidate are required'
    return 1
  fi
  assert_sha256 "$expected_old_sha" '--expected-old-sha256' || return 1
  assert_sha256 "$expected_new_sha" '--expected-new-sha256' || return 1
  assert_revision "$expected_old_revision" '--expected-old-revision' || return 1
  assert_revision "$expected_new_revision" '--expected-new-revision' || return 1
  if [ "$expected_old_sha" = "$expected_new_sha" ]; then
    die 'The old and new executable hashes are equal'
    return 1
  fi
  if [ "$expected_old_revision" = "$expected_new_revision" ]; then
    die 'The old and new VCS revisions are equal'
    return 1
  fi
  if [[ ! "$expected_peer_id" =~ ^public-[0-9a-f]{32}$ ]]; then
    die '--expected-peer-id is not a public peer identity'
    return 1
  fi
  if [[ ! "$expected_slot" =~ ^[1-9][0-9]*$ ]]; then
    die '--expected-slot is not a positive slot number'
    return 1
  fi
  if [[ ! "$status_url" =~ ^https://[^[:space:]]+/api/status$ ]]; then
    die '--status-url must be one secure /api/status URL'
    return 1
  fi

  assert_no_unix_symlink_path "$root" || return 1
  assert_no_unix_symlink_path "$candidate" || return 1
  root="$(realpath -e -- "$root")" || return 1
  candidate="$(realpath -e -- "$candidate")" || return 1
  if [ ! -d "$root" ]; then die 'The broadcast root is not a directory'; return 1; fi
  if [ ! -f "$candidate" ]; then die 'The candidate is not a regular file'; return 1; fi
  stage_windows_helper || return 1
  assert_exact_broadcast_root || return 1
  assert_no_windows_reparse "$candidate" || return 1
  acquire_install_lock || return 1
  target="$root/multiverse-sidecar.exe"
  if [ ! -f "$target" ]; then die "The installed sidecar is missing: $target"; return 1; fi
  assert_no_unix_symlink_path "$target" || return 1
  assert_no_windows_reparse "$target" || return 1
  validate_installed_configuration || return 1
  assert_windows_executable "$candidate" || return 1

  old_actual="$(sha256sum "$target" | awk '{print $1}')"
  new_actual="$(sha256sum "$candidate" | awk '{print $1}')"
  if [ "$old_actual" != "$expected_old_sha" ]; then
    die 'The installed sidecar does not match the required old SHA-256'
    return 1
  fi
  if [ "$new_actual" != "$expected_new_sha" ]; then
    die 'The candidate sidecar does not match the required new SHA-256'
    return 1
  fi

  metadata="$(read_vcs_metadata "$target")" || return 1
  IFS=$'\t' read -r revision modified <<<"$metadata"
  if [ "$revision" != "$expected_old_revision" ] || [ "$modified" != false ]; then
    die 'The installed sidecar does not have the required clean old VCS stamp'
    return 1
  fi
  metadata="$(read_vcs_metadata "$candidate")" || return 1
  IFS=$'\t' read -r revision modified <<<"$metadata"
  if [ "$revision" != "$expected_new_revision" ] || [ "$modified" != false ]; then
    die 'The candidate sidecar does not have the required clean new VCS stamp'
    return 1
  fi

  target_acl="$(get_windows_acl "$target")" || return 1
  if [ -z "$target_acl" ]; then die 'Windows returned an empty ACL for the installed sidecar'; return 1; fi
  static_baseline="$(capture_static_manifest)" || return 1
  assert_runtime_health "$expected_old_sha" "$expected_old_revision" || return 1
  say "preflight passed for the sealed identity in slot $expected_slot"
  if [ "$dry_run" -eq 1 ]; then
    say 'dry-run complete; no service, executable, custody, journal, save, or configuration data changed'
    cleanup_stage
    return 0
  fi

  backup_parent="$root/.sidecar-update-backups"
  if [ -e "$backup_parent" ]; then
    if [ ! -d "$backup_parent" ]; then
      die 'The sidecar backup path exists and is not a directory'
      return 1
    fi
    assert_no_unix_symlink_path "$backup_parent" || return 1
    assert_no_windows_reparse "$backup_parent" || return 1
  else
    install -d -m 0700 "$backup_parent" || return 1
    protect_windows_directory "$backup_parent" || return 1
  fi
  backup_dir="$(mktemp -d "$backup_parent/update.XXXXXX")" || return 1
  protect_windows_directory "$backup_dir" || return 1
  backup_file="$backup_dir/multiverse-sidecar.exe"
  install -m 0600 "$target" "$backup_file" || return 1
  if [ "$(sha256sum "$backup_file" | awk '{print $1}')" != "$expected_old_sha" ]; then
    die 'The protected executable backup does not match the old SHA-256'
    return 1
  fi
  protect_windows_directory "$backup_dir" || return 1

  stage_dir="$(mktemp -d "$root/.sidecar-update-stage.XXXXXX")" || return 1
  if [ "$(stat -c %d "$stage_dir")" != "$(stat -c %d "$target")" ]; then
    die 'The candidate stage and installed sidecar are not on the same filesystem'
    return 1
  fi
  staged_candidate="$stage_dir/multiverse-sidecar.exe"
  install -m 0600 "$candidate" "$staged_candidate" || return 1
  if [ "$(sha256sum "$staged_candidate" | awk '{print $1}')" != "$expected_new_sha" ]; then
    die 'The staged candidate does not match the required new SHA-256'
    return 1
  fi
  assert_no_windows_reparse "$staged_candidate" || return 1
  assert_static_unchanged || return 1
  assert_runtime_health "$expected_old_sha" "$expected_old_revision" || return 1

  phase='stopping'
  installed_stop || return 1
  assert_runtime_stopped || return 1
  phase='stopped'
  assert_static_unchanged || return 1
  assert_target_identity "$expected_old_sha" "$expected_old_revision" || return 1

  rollback_needed=1
  phase='replacement-attempted'
  atomic_replace_windows "$staged_candidate" "$target" || return 1
  set_windows_acl "$target" "$target_acl" || return 1
  if [ "$(get_windows_acl "$target")" != "$target_acl" ]; then
    die 'The replacement sidecar does not have the exact saved ACL'
    return 1
  fi
  if [ "$(sha256sum "$target" | awk '{print $1}')" != "$expected_new_sha" ]; then
    die 'The installed replacement does not match the required new SHA-256'
    return 1
  fi
  assert_static_unchanged || return 1

  phase='starting'
  installed_start || return 1
  wait_for_runtime_health "$expected_new_sha" "$expected_new_revision" || return 1
  assert_static_unchanged || return 1
  phase='observing'
  observe_source_flow || return 1
  wait_for_runtime_health "$expected_new_sha" "$expected_new_revision" || return 1
  assert_static_unchanged || return 1

  rollback_needed=0
  phase='complete'
  cleanup_stage
  say "sidecar-only update complete; the protected old executable is $backup_file"
}

run_cli() {
  trap 'on_error $?' ERR
  trap 'on_error 129' HUP
  trap 'on_error 130' INT
  trap 'on_error 131' QUIT
  trap 'on_error 143' TERM
  main "$@"
  trap - ERR HUP INT QUIT TERM
}

if [[ "${BASH_SOURCE[0]}" = "$0" ]]; then
  run_cli "$@"
fi
