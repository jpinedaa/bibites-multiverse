#!/usr/bin/env bash
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
wait_sidecar="$repo/cloud/aws/runtime/bibites-wait-sidecar"
set_timescale="$repo/cloud/aws/runtime/bibites-set-timescale"
test_root="$(mktemp -d)"
responder_pid=''
cleanup() {
  [ -z "$responder_pid" ] || kill "$responder_pid" 2>/dev/null || true
  find "$test_root" -depth -delete
}
trap cleanup EXIT

world_root="$test_root/worlds"
world_dir="$world_root/slot-1"
sidecar_data="$world_dir/sidecar"
token_file="$sidecar_data/contract-a.token"
sidecar_log="$world_dir/sidecar.log"
command_file="$world_dir/command.txt"
command_log="$command_file.log"
mock_sidecar="$test_root/multiverse-sidecar"
install -d "$world_dir" "$sidecar_data"

printf '%s\n' \
  '#!/usr/bin/env bash' \
  'set -euo pipefail' \
  'printf "%s\n" "$MOCK_SLOT_JSON"' >"$mock_sidecar"
chmod 0755 "$mock_sidecar"

{
  printf 'SIDECAR_DATA=%q\n' "$sidecar_data"
  printf 'SIDECAR_LOG=%q\n' "$sidecar_log"
  printf 'CONTRACT_A_TOKEN_FILE=%q\n' "$token_file"
  printf 'COMMAND_FILE=%q\n' "$command_file"
  printf 'TARGET_TIME_SCALE=%q\n' 37
} >"$world_dir/world.env"
printf 'local-token\n' >"$token_file"

ready_json='{
  "schema":"multiverse-own-slot/1",
  "credentialConfigured":true,
  "relay":{"connected":true},
  "slot":{"slot":4,"grantSeen":true,"granted":true},
  "mod":{"connected":true}
}'

# A rotated log has no current grant line. It can also retain an old failure.
# Neither is current state, so both must be ignored when /my-slot is ready.
printf '%s\n' \
  'old session: HTTP 401' \
  'the current grant rotated out of this file' >"$sidecar_log"
MOCK_SLOT_JSON="$ready_json" \
  BIBITES_WORLD_ROOT="$world_root" \
  BIBITES_SIDECAR_BIN="$mock_sidecar" \
  "$wait_sidecar" slot-1

refused_json='{
  "schema":"multiverse-own-slot/1",
  "credentialConfigured":true,
  "relay":{"connected":true},
  "slot":{"slot":0,"grantSeen":true,"granted":false,"grantReason":"role has no slot"},
  "mod":{"connected":false}
}'
set +e
refusal_output="$(MOCK_SLOT_JSON="$refused_json" \
  BIBITES_WORLD_ROOT="$world_root" \
  BIBITES_SIDECAR_BIN="$mock_sidecar" \
  "$wait_sidecar" slot-1 2>&1)"
status=$?
set -e
[ "$status" -eq 1 ] || {
  echo "slot refusal returned $status instead of 1" >&2
  exit 1
}
grep -Fq 'sidecar reported a permanent relay or slot refusal' <<<"$refusal_output" || {
  echo 'slot refusal did not use the live sidecar state' >&2
  exit 1
}

# Drive the time-scale command to completion. The fake game acknowledges each
# new token, which proves both sends survive the restart path's measured retry.
seen_commands="$test_root/seen-commands"
: >"$seen_commands"
(
  last_token=''
  count=0
  while [ "$count" -lt 2 ]; do
    if [ -s "$command_file" ]; then
      command="$(<"$command_file")"
      token="${command%% *}"
      if [ "$token" != "$last_token" ]; then
        printf '%s OK targetTimeScale=37\n' "$token" >"$command_log"
        printf '%s\n' "$command" >>"$seen_commands"
        last_token="$token"
        count=$((count + 1))
      fi
    fi
    sleep 0.01
  done
) &
responder_pid=$!
MOCK_SLOT_JSON="$ready_json" \
  BIBITES_WORLD_ROOT="$world_root" \
  BIBITES_SIDECAR_BIN="$mock_sidecar" \
  BIBITES_SCALE_RETRY_DELAY_SECONDS=0 \
  "$set_timescale" slot-1
wait "$responder_pid"
responder_pid=''
[ "$(wc -l <"$seen_commands")" -eq 2 ] || {
  echo 'time-scale restoration did not send the command twice' >&2
  exit 1
}
[ "$(awk '{print $2, $3}' "$seen_commands" | sort -u)" = 'timescale 37' ] || {
  echo 'time-scale restoration sent the wrong target' >&2
  exit 1
}
[ "$(awk '{print $1}' "$seen_commands" | sort -u | wc -l)" -eq 2 ] || {
  echo 'time-scale restoration reused an acknowledgement token' >&2
  exit 1
}

game_unit="$repo/cloud/aws/runtime/bibites-game@.service"
timescale_unit="$repo/cloud/aws/runtime/bibites-timescale@.service"
sync_worlds="$repo/cloud/aws/runtime/bibites-sync-worlds"
grep -Fq 'Wants=bibites-timescale@%i.service' "$game_unit" || {
  echo 'a restarted game no longer queues its time-scale unit' >&2
  exit 1
}
grep -Fq 'PartOf=bibites-game@%i.service' "$timescale_unit" || {
  echo 'an explicit game restart no longer propagates to its time-scale unit' >&2
  exit 1
}
grep -Fq "printf 'MULTIVERSE_STARTUP_TIME_SCALE=%q" "$sync_worlds" || {
  echo 'world synchronization no longer gives the mod its startup time scale' >&2
  exit 1
}

printf 'runtime readiness and time-scale fixtures passed\n'
