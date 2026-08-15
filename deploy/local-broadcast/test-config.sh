#!/usr/bin/env bash
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
runner="$here/run-windows.ps1"
stopper="$here/stop-windows.ps1"
installer="$here/install.sh"

expect_text() {
  local file="$1"
  local text="$2"
  grep -Fq -- "$text" "$file" || {
    printf 'missing expected setting in %s: %s\n' "$file" "$text" >&2
    exit 1
  }
}

forbid_text() {
  local file="$1"
  local text="$2"
  if grep -Fq -- "$text" "$file"; then
    printf 'refused setting is still in %s: %s\n' "$file" "$text" >&2
    exit 1
  fi
}

fail() {
  printf '%s\n' "$*" >&2
  exit 1
}

expect_text "$runner" "MULTIVERSE_BROADCAST_ZOOM = '200'"
expect_text "$runner" "MULTIVERSE_BROADCAST_HIDE_UI = 'false'"
expect_text "$runner" "MULTIVERSE_BROADCAST_TIME_SCALE = '7.5'"
expect_text "$runner" "MULTIVERSE_BROADCAST_PANELS = 'brain,biology,biology'"
expect_text "$runner" "MULTIVERSE_BROADCAST_PANEL_SECONDS = '15'"
expect_text "$runner" "MULTIVERSE_BROADCAST_SHOW_FOV = 'true'"
expect_text "$runner" "MULTIVERSE_BROADCAST_DISABLE_SPAWN_TEMPLATES = 'Basic bibite'"

# The broadcast world is a participant of the map, not an offline exhibition.
expect_text "$runner" 'MULTIVERSE_EXPORT_EDGES = $config.ExportEdges'
expect_text "$runner" 'MULTIVERSE_MIGRATION_EXCLUDE = $config.ExcludeSpecies'
expect_text "$runner" 'MULTIVERSE_SIDECAR_PORT = $config.SidecarPort'
expect_text "$runner" 'MULTIVERSE_CONTRACT_A_TOKEN_FILE'
expect_text "$runner" "MULTIVERSE_PORTAL = 'true'"
expect_text "$runner" "MULTIVERSE_PORTAL_FLOURISHES = 'true'"
expect_text "$runner" 'contract B: slot granted'
forbid_text "$runner" "MULTIVERSE_EXPORT_EDGES = 'none'"
forbid_text "$runner" "MULTIVERSE_PORTAL = 'false'"

# The sidecar starts before the game and stops after it.
grep -n "Start-NativeProcess -FilePath \$config.SidecarExe" "$runner" >/dev/null || {
  printf 'the runner does not start the sidecar\n' >&2
  exit 1
}
sidecar_line="$(grep -n 'Start-NativeProcess -FilePath \$config.SidecarExe' "$runner" | head -1 | cut -d: -f1)"
game_line="$(grep -n 'Start-Process -FilePath \$gameExe' "$runner" | head -1 | cut -d: -f1)"
[ "$sidecar_line" -lt "$game_line" ] || {
  printf 'the runner starts the game before the sidecar\n' >&2
  exit 1
}
expect_text "$stopper" "Get-RecordedProcess 'sidecar'"
stop_game_line="$(grep -n "Get-RecordedProcess 'game'" "$stopper" | head -1 | cut -d: -f1)"
stop_sidecar_line="$(grep -n "Get-RecordedProcess 'sidecar'" "$stopper" | head -1 | cut -d: -f1)"
[ "$stop_game_line" -lt "$stop_sidecar_line" ] || {
  printf 'the stopper stops the sidecar before the game\n' >&2
  exit 1
}

# The installer gives this world its own identity and never copies another one.
expect_text "$installer" 'bibites-multiverse/enrollment-request/1'
expect_text "$installer" 'bibites-multiverse/enrollment-response/1'
expect_text "$installer" 'bibites-multiverse/enrollment-pending/1'
expect_text "$installer" 'GOOS=windows GOARCH=amd64'
expect_text "$installer" 'SetAccessRuleProtection($true, $false)'
expect_text "$installer" 'ACL includes an account other than the current Windows user'
expect_text "$installer" 'another local broadcast installation is active'
expect_text "$installer" 'custody state without a durable peer identity'
expect_text "$installer" '"$runtime_root/bin/start" {install_lock_fd}>&-'
expect_text "$installer" '.modConnected == true'
expect_text "$installer" '((.exportEdges // []) | sort) == ($edges | sort)'
expect_text "$installer" 'hls_stream_ready "$public_manifest"'
expect_text "$installer" '--cookie-jar "$cookie_jar"'
expect_text "$installer" '$6 == "hlsSession"'
expect_text "$installer" "--write-out '%{http_code}'"
expect_text "$installer" 'the latest completed HLS segment did not return HTTP 200'
expect_text "$installer" 'the latest completed HLS segment was empty'
expect_text "$installer" 'the latest completed HLS segment request failed'
forbid_text "$installer" '--output /dev/null "$segment_url"'
forbid_text "$installer" 'Cookie: cookieCheck=1'
forbid_text "$installer" 'index.m3u8?cookieCheck=1'
forbid_text "$installer" '--arg secret'
expect_text "$runner" '$durablePeer -ne $config.PeerId'
expect_text "$runner" 'ConvertTo-WindowsArgument'
expect_text "$runner" '$startInfo.Arguments'
forbid_text "$runner" 'Start-Process -FilePath $config.SidecarExe'
bash -n "$installer"

# The lock starts before identity preflight and remains open through startup.
lock_line="$(grep -n '^acquire_install_lock$' "$installer" | cut -d: -f1)"
preflight_line="$(grep -n '^preflight_public_identity$' "$installer" | cut -d: -f1)"
stop_line="$(grep -n '^tmux -L bibites-broadcast kill-session' "$installer" | cut -d: -f1)"
start_line="$(grep -n '^  "$runtime_root/bin/start"' "$installer" | cut -d: -f1)"
[ "$lock_line" -lt "$preflight_line" ] || fail 'the install lock starts after identity preflight'
[ "$preflight_line" -lt "$stop_line" ] || fail 'identity preflight starts after the live-service stop'
[ "$stop_line" -lt "$start_line" ] || fail 'the startup ordering fixture is invalid'

# Load the installer functions without running the installer.
# shellcheck source=install.sh
source "$installer"

fixtures="$(mktemp -d)"
cleanup() {
  if [ -n "${windows_acl_root:-}" ]; then
    powershell.exe -NoProfile -Command \
      'param($path) Remove-Item -LiteralPath $path -Recurse -Force -ErrorAction SilentlyContinue' \
      "$windows_acl_root" >/dev/null 2>&1 || true
  fi
  rm -rf "$fixtures"
}
trap cleanup EXIT

fixture_secret='aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'
fixture_install_id='12345678-1234-4234-8234-1234567890ab'
fixture_peer='public-123456781234423482341234567890ab'
fixture_relay='wss://fixture.invalid/contract-b/v4'

prepare_fixture() {
  local name="$1"
  fixture="$fixtures/$name"
  multiverse_root="$fixture/multiverse"
  multiverse_data="$multiverse_root/data"
  credential_file="$multiverse_root/peer-secret.txt"
  record_file="$multiverse_root/enrollment.json"
  pending_file="$multiverse_root/enrollment-pending.json"
  durable_peer_file="$multiverse_data/peer-id"
  public_map="$fixture/public-map.json"
  events="$fixture/events"
  world_name='Broadcast-Live'
  release_id='fixture'
  peer_id=''
  mkdir -p "$multiverse_data"
  jq -nc --arg relay "$fixture_relay" \
    '{format:"bibites-multiverse/public-map/1",enrollmentUrl:"https://fixture.invalid/api/enroll",relayUrl:$relay}' \
    >"$public_map"
  : >"$events"
}

write_completed_fixture() {
  local peer="${1:-$fixture_peer}"
  local world="${2:-$world_name}"
  printf '%s\n' "$fixture_secret" >"$credential_file"
  printf '%s\n' "$peer" >"$durable_peer_file"
  jq -nc --arg peer "$peer" --arg relay "$fixture_relay" --arg world "$world" \
    '{format:"bibites-multiverse/broadcast-identity/1",peerId:$peer,relayUrl:$relay,world:$world}' \
    >"$record_file"
}

write_pending_fixture() {
  local install_id="$1"
  local pending_secret="$2"
  jq -nc --arg id "$install_id" --arg secret "$pending_secret" \
    '{format:"bibites-multiverse/enrollment-pending/1",installId:$id,secret:$secret}' \
    >"$pending_file"
}

run_mocked_preflight() {
  (
    protect_windows_identity() { printf 'acl\n' >>"$events"; }
    request_public_identity() {
      [ "$1" = "$fixture_install_id" ] || fail 'preflight sent the wrong installation id'
      [ "$2" = "$fixture_secret" ] || fail 'preflight sent the wrong credential'
      [ "$3" = "$fixture_peer" ] || fail 'preflight sent the wrong peer identity'
      printf 'post\n' >>"$events"
    }
    preflight_public_identity
  )
}

# The enrollment request keeps the secret in the request body, not in a child argument.
request_body="$fixtures/enrollment-request.json"
relay_url="$fixture_relay"
enrollment_url='https://fixture.invalid/api/enroll'
release_id='fixture'
curl() {
  cat >"$request_body"
  jq -nc --arg peer "$fixture_peer" --arg relay "$fixture_relay" \
    '{format:"bibites-multiverse/enrollment-response/1",peerId:$peer,relayUrl:$relay,created:false}'
}
request_public_identity "$fixture_install_id" "$fixture_secret" "$fixture_peer" >/dev/null
jq -e --arg id "$fixture_install_id" --arg secret "$fixture_secret" \
  '.installId == $id and .secret == $secret and .format == "bibites-multiverse/enrollment-request/1"' \
  "$request_body" >/dev/null || fail 'the enrollment request body changed the identity'
unset -f curl

prepare_fixture new-identity
(
  protect_windows_identity() { printf 'acl\n' >>"$events"; }
  request_public_identity() { printf 'post\n' >>"$events"; }
  preflight_public_identity
) >/dev/null
[ "$(tr '\n' ' ' <"$events")" = 'acl acl post acl ' ] || \
  fail 'a new pending credential was not protected before enrollment'

prepare_fixture missing-durable
write_completed_fixture
rm -f "$durable_peer_file"
if run_mocked_preflight >/dev/null 2>&1; then fail 'preflight accepted a missing durable peer id'; fi
[ ! -s "$events" ] || fail 'missing durable peer id reached ACL or enrollment work'

prepare_fixture orphaned-custody
printf '%s\n' 'orphaned fixture' >"$multiverse_data/journal"
orphaned_sha="$(sha256sum "$multiverse_data/journal" | awk '{print $1}')"
if run_mocked_preflight >/dev/null 2>&1; then fail 'preflight accepted custody state without an identity'; fi
[ ! -e "$pending_file" ] || fail 'orphaned custody state created a pending identity'
[ ! -s "$events" ] || fail 'orphaned custody state reached ACL or enrollment work'
[ "$(find "$multiverse_data" -mindepth 1 | wc -l)" -eq 1 ] || \
  fail 'orphaned custody preflight wrote another data file'
[ "$(sha256sum "$multiverse_data/journal" | awk '{print $1}')" = "$orphaned_sha" ] || \
  fail 'orphaned custody preflight changed existing data'

prepare_fixture mismatched-durable
write_completed_fixture
printf '%s\n' 'public-ffffffffffffffffffffffffffffffff' >"$durable_peer_file"
if run_mocked_preflight >/dev/null 2>&1; then fail 'preflight accepted a mismatched durable peer id'; fi
[ ! -s "$events" ] || fail 'mismatched durable peer id reached ACL or enrollment work'

prepare_fixture world-mismatch
write_completed_fixture "$fixture_peer" 'Another-World'
if run_mocked_preflight >/dev/null 2>&1; then fail 'preflight accepted an enrollment record for another world'; fi
[ ! -s "$events" ] || fail 'world mismatch reached ACL or enrollment work'

prepare_fixture conflicting-pending
write_completed_fixture
write_pending_fixture 'ffffffff-ffff-4fff-8fff-ffffffffffff' \
  'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb'
pending_sha="$(sha256sum "$pending_file" | awk '{print $1}')"
if run_mocked_preflight >/dev/null 2>&1; then fail 'preflight accepted a conflicting pending identity'; fi
[ -f "$pending_file" ] || fail 'preflight removed a conflicting pending identity'
[ "$(sha256sum "$pending_file" | awk '{print $1}')" = "$pending_sha" ] || \
  fail 'preflight changed a conflicting pending identity'
[ ! -s "$events" ] || fail 'conflicting pending identity reached ACL or enrollment work'

prepare_fixture matching-pending
write_completed_fixture
write_pending_fixture "$fixture_install_id" "$fixture_secret"
run_mocked_preflight >/dev/null
[ ! -e "$pending_file" ] || fail 'preflight kept a matching completed pending identity'
[ "$(tr '\n' ' ' <"$events")" = 'acl post ' ] || \
  fail 'preflight did not protect the identity before its enrollment request'

prepare_fixture interrupted-completion
write_pending_fixture "$fixture_install_id" "$fixture_secret"
printf '%s\n' "$fixture_peer" >"$durable_peer_file"
run_mocked_preflight >/dev/null
[ "$(cat "$credential_file")" = "$fixture_secret" ] || \
  fail 'preflight did not recover the matching partial credential'
[ "$(jq -r '.peerId' "$record_file")" = "$fixture_peer" ] || \
  fail 'preflight did not recover the matching partial record'
[ ! -e "$pending_file" ] || fail 'preflight kept the recovered pending identity'

# Two installer processes cannot hold the same lock.
config_root="$fixtures/install-lock"
lock_ready="$fixtures/lock-ready"
lock_release="$fixtures/lock-release"
(
  acquire_install_lock
  : >"$lock_ready"
  while [ ! -f "$lock_release" ]; do sleep 0.05; done
) &
lock_holder=$!
for _ in $(seq 1 100); do [ -f "$lock_ready" ] && break; sleep 0.05; done
[ -f "$lock_ready" ] || fail 'the lock fixture did not start'
if ( acquire_install_lock ) >/dev/null 2>&1; then fail 'two installers acquired the same lock'; fi
: >"$lock_release"
wait "$lock_holder"

# A long-lived runtime child must not inherit the installer lock descriptor.
config_root="$fixtures/descendant-lock"
descendant_ready="$fixtures/descendant-ready"
descendant_release="$fixtures/descendant-release"
descendant_done="$fixtures/descendant-done"
(
  acquire_install_lock
  bash -c '
    : >"$1"
    while [ ! -f "$2" ]; do sleep 0.05; done
    : >"$3"
  ' _ "$descendant_ready" "$descendant_release" "$descendant_done" \
    {install_lock_fd}>&- </dev/null >/dev/null 2>&1 &
) &
lock_parent=$!
for _ in $(seq 1 100); do [ -f "$descendant_ready" ] && break; sleep 0.05; done
[ -f "$descendant_ready" ] || fail 'the descendant lock fixture did not start'
wait "$lock_parent"
( acquire_install_lock ) || fail 'a long-lived runtime descendant inherited the install lock'
: >"$descendant_release"
for _ in $(seq 1 100); do [ -f "$descendant_done" ] && break; sleep 0.05; done
[ -f "$descendant_done" ] || fail 'the descendant lock fixture did not stop'

# Installer success requires the game connection and the configured export edges.
expected_edges='["E","N","W","S"]'
jq -nc --arg peer "$fixture_peer" \
  '{slots:[{peerId:$peer,live:true,modConnected:false,exportEdges:["E","N","W","S"]}]}' \
  >"$fixtures/no-mod.json"
if status_has_ready_peer "$fixture_peer" "$expected_edges" <"$fixtures/no-mod.json"; then
  fail 'a live sidecar without its mod passed the readiness check'
fi
jq -nc --arg peer "$fixture_peer" \
  '{slots:[{peerId:$peer,live:true,modConnected:true,exportEdges:["E","N"]}]}' \
  >"$fixtures/wrong-edges.json"
if status_has_ready_peer "$fixture_peer" "$expected_edges" <"$fixtures/wrong-edges.json"; then
  fail 'a peer with missing export edges passed the readiness check'
fi
jq -nc --arg peer "$fixture_peer" \
  '{slots:[{peerId:$peer,live:true,modConnected:true,exportEdges:["S","W","N","E"]}]}' \
  >"$fixtures/ready.json"
status_has_ready_peer "$fixture_peer" "$expected_edges" <"$fixtures/ready.json" || \
  fail 'the ready peer fixture failed'

# HLS readiness keeps the server session from the master through one completed segment.
hls_relative_reference 'video1_stream.m3u8' playlist || \
  fail 'the HLS reference rule rejected a relative child playlist'
hls_relative_reference 'rendition/video1_stream.m3u8' playlist || \
  fail 'the HLS reference rule rejected a child playlist directory'
hls_relative_reference 'rendition/video1_seg10.mp4' segment || \
  fail 'the HLS reference rule rejected a relative media segment'
for unsafe_reference in \
  'https://other.invalid/video1_stream.m3u8' \
  '/video1_stream.m3u8' \
  '../video1_stream.m3u8' \
  'rendition/../video1_stream.m3u8' \
  'video1_stream.m3u8?session=fixture' \
  'video1 stream.m3u8'; do
  if hls_relative_reference "$unsafe_reference" playlist; then
    fail "the HLS reference rule accepted an unsafe child playlist: $unsafe_reference"
  fi
done
if hls_relative_reference 'video1_seg10.exe' segment; then
  fail 'the HLS reference rule accepted a non-media segment'
fi

hls_tmp="$fixtures/hls-tmp"
hls_events="$fixtures/hls-events"
hls_reason="$fixtures/hls-reason"
hls_mock_error="$fixtures/hls-mock-error"
hls_manifest='https://fixture.invalid/stream/bibites/index.m3u8'
hls_root='https://fixture.invalid/stream/bibites'
mkdir -p "$hls_tmp"

run_hls_fixture() {
  local mode="$1"
  : >"$hls_events"
  : >"$hls_reason"
  rm -f "$hls_mock_error"
  (
    fixture_hls_mode="$mode"
    fixture_hls_cookie_jar=''

    mock_hls_fail() {
      printf '%s\n' "$*" >"$hls_mock_error"
      return 99
    }

    curl() {
      local cookie_seen=0 cookie_arg='' cookie_jar_arg='' output_arg=''
      local write_out_arg=''
      local location_seen=0 url=''
      while [ "$#" -gt 0 ]; do
        case "$1" in
          --fail|--silent|--show-error) shift ;;
          --location) location_seen=1; shift ;;
          --max-redirs|--connect-timeout|--max-time|--proto|--proto-redir)
            [ "$#" -ge 2 ] || { mock_hls_fail "missing value for $1"; return 99; }
            shift 2
            ;;
          --cookie)
            [ "$#" -ge 2 ] || { mock_hls_fail 'missing cookie value'; return 99; }
            cookie_seen=1
            cookie_arg="$2"
            shift 2
            ;;
          --cookie-jar)
            [ "$#" -ge 2 ] || { mock_hls_fail 'missing cookie-jar value'; return 99; }
            cookie_jar_arg="$2"
            shift 2
            ;;
          --output)
            [ "$#" -ge 2 ] || { mock_hls_fail 'missing output value'; return 99; }
            output_arg="$2"
            shift 2
            ;;
          --write-out)
            [ "$#" -ge 2 ] || { mock_hls_fail 'missing write-out value'; return 99; }
            write_out_arg="$2"
            shift 2
            ;;
          https://*)
            [ -z "$url" ] || { mock_hls_fail 'the HLS request has two URLs'; return 99; }
            url="$1"
            shift
            ;;
          *) mock_hls_fail "unexpected HLS curl argument: $1"; return 99 ;;
        esac
      done

      [ -n "$url" ] || { mock_hls_fail 'the HLS request has no URL'; return 99; }
      if [ "$url" = "$hls_manifest" ]; then
        [ "$location_seen" -eq 1 ] || { mock_hls_fail 'the master request did not follow redirects'; return 99; }
        [ "$cookie_seen" -eq 1 ] && [ -z "$cookie_arg" ] || {
          mock_hls_fail 'the master request did not start an empty cookie engine'
          return 99
        }
        [ -n "$cookie_jar_arg" ] && [ -n "$output_arg" ] || {
          mock_hls_fail 'the master request omitted its cookie jar or output'
          return 99
        }
        fixture_hls_cookie_jar="$cookie_jar_arg"
        printf '# Netscape HTTP Cookie File\n' >"$cookie_jar_arg"
        printf 'fixture.invalid\tFALSE\t/stream/bibites/\tTRUE\t0\tcookieCheck\t1\n' \
          >>"$cookie_jar_arg"
        if [ "$fixture_hls_mode" != missing-session ]; then
          printf '#HttpOnly_fixture.invalid\tTRUE\t/stream/bibites/\tTRUE\t0\thlsSession\tfixture-session\n' \
            >>"$cookie_jar_arg"
        fi
        if [ "$fixture_hls_mode" = invalid-child ]; then
          printf '#EXTM3U\nhttps://other.invalid/video1_stream.m3u8\n' >"$output_arg"
        else
          printf '#EXTM3U\nvideo1_stream.m3u8\n' >"$output_arg"
        fi
        printf 'master\n' >>"$hls_events"
        return 0
      fi

      if [ "$url" = "$hls_root/video1_stream.m3u8" ]; then
        printf 'child\n' >>"$hls_events"
        [ "$cookie_seen" -eq 1 ] && [ "$cookie_arg" = "$fixture_hls_cookie_jar" ] || {
          mock_hls_fail 'the child request did not reuse the cookie jar'
          return 99
        }
        grep -Fq $'\thlsSession\tfixture-session' "$cookie_arg" || {
          mock_hls_fail 'the child request had no HLS session'
          return 99
        }
        [ "$fixture_hls_mode" != child-failure ] || return 22
        if [ "$fixture_hls_mode" = invalid-segment ]; then
          printf '#EXTM3U\n../video1_seg10.mp4\n' >"$output_arg"
        else
          printf '#EXTM3U\nvideo1_seg9.mp4\nvideo1_seg10.mp4\n' >"$output_arg"
        fi
        return 0
      fi

      if [ "$url" = "$hls_root/video1_seg10.mp4" ]; then
        printf 'segment\n' >>"$hls_events"
        [ "$cookie_seen" -eq 1 ] && [ "$cookie_arg" = "$fixture_hls_cookie_jar" ] || {
          mock_hls_fail 'the segment request did not reuse the cookie jar'
          return 99
        }
        [ -n "$output_arg" ] && [ "$output_arg" != /dev/null ] || {
          mock_hls_fail 'the segment request did not store the media body'
          return 99
        }
        [ "$write_out_arg" = '%{http_code}' ] || {
          mock_hls_fail 'the segment request did not read the HTTP status'
          return 99
        }
        [ "$fixture_hls_mode" != segment-failure ] || return 22
        if [ "$fixture_hls_mode" = segment-redirect ]; then
          printf 'redirect body\n' >"$output_arg"
          printf '302'
          return 0
        fi
        if [ "$fixture_hls_mode" = segment-no-content ]; then
          : >"$output_arg"
          printf '204'
          return 0
        fi
        if [ "$fixture_hls_mode" = segment-empty ]; then
          : >"$output_arg"
          printf '200'
          return 0
        fi
        printf 'fixture media\n' >"$output_arg"
        printf '200'
        return 0
      fi

      mock_hls_fail "the HLS request used an unexpected URL: $url"
      return 99
    }

    if TMPDIR="$hls_tmp" hls_stream_ready "$hls_manifest"; then
      result=0
    else
      result=$?
    fi
    printf '%s' "$hls_probe_error" >"$hls_reason"
    exit "$result"
  )
}

assert_hls_fixture_clean() {
  [ ! -s "$hls_mock_error" ] || fail "$(cat "$hls_mock_error")"
  [ -z "$(find "$hls_tmp" -mindepth 1 -print -quit)" ] || \
    fail 'the HLS readiness fixture kept temporary files'
}

run_hls_fixture success || fail 'the complete HLS session fixture failed'
[ "$(tr '\n' ' ' <"$hls_events")" = 'master child segment ' ] || \
  fail 'the complete HLS session did not read the master, child, and segment'
[ ! -s "$hls_reason" ] || fail 'the complete HLS session kept a failure reason'
assert_hls_fixture_clean

if run_hls_fixture missing-session; then fail 'HLS readiness accepted a missing session cookie'; fi
[ "$(tr '\n' ' ' <"$hls_events")" = 'master ' ] || \
  fail 'the missing HLS session reached a child request'
grep -Fq 'did not create a session cookie' "$hls_reason" || \
  fail 'the missing HLS session reported the wrong failure'
assert_hls_fixture_clean

if run_hls_fixture child-failure; then fail 'HLS readiness accepted a failed child request'; fi
[ "$(tr '\n' ' ' <"$hls_events")" = 'master child ' ] || \
  fail 'the failed HLS child reached a segment request'
grep -Fq 'child playlist request failed' "$hls_reason" || \
  fail 'the failed HLS child reported the wrong failure'
assert_hls_fixture_clean

if run_hls_fixture segment-failure; then fail 'HLS readiness accepted a failed segment request'; fi
[ "$(tr '\n' ' ' <"$hls_events")" = 'master child segment ' ] || \
  fail 'the failed HLS segment fixture did not complete its request chain'
grep -Fq 'latest completed HLS segment request failed' "$hls_reason" || \
  fail 'the failed HLS segment reported the wrong failure'
assert_hls_fixture_clean

if run_hls_fixture segment-redirect; then fail 'HLS readiness accepted an HTTP 302 segment'; fi
[ "$(tr '\n' ' ' <"$hls_events")" = 'master child segment ' ] || \
  fail 'the HTTP 302 HLS fixture did not complete its request chain'
grep -Fq 'did not return HTTP 200' "$hls_reason" || \
  fail 'the HTTP 302 HLS segment reported the wrong failure'
assert_hls_fixture_clean

if run_hls_fixture segment-no-content; then fail 'HLS readiness accepted an HTTP 204 segment'; fi
[ "$(tr '\n' ' ' <"$hls_events")" = 'master child segment ' ] || \
  fail 'the HTTP 204 HLS fixture did not complete its request chain'
grep -Fq 'did not return HTTP 200' "$hls_reason" || \
  fail 'the HTTP 204 HLS segment reported the wrong failure'
assert_hls_fixture_clean

if run_hls_fixture segment-empty; then fail 'HLS readiness accepted an empty HTTP 200 segment'; fi
[ "$(tr '\n' ' ' <"$hls_events")" = 'master child segment ' ] || \
  fail 'the empty HTTP 200 HLS fixture did not complete its request chain'
grep -Fq 'latest completed HLS segment was empty' "$hls_reason" || \
  fail 'the empty HTTP 200 HLS segment reported the wrong failure'
assert_hls_fixture_clean

if run_hls_fixture invalid-child; then fail 'HLS readiness accepted an absolute child URL'; fi
[ "$(tr '\n' ' ' <"$hls_events")" = 'master ' ] || \
  fail 'an unsafe HLS child reached another request'
assert_hls_fixture_clean

if run_hls_fixture invalid-segment; then fail 'HLS readiness accepted a parent segment path'; fi
[ "$(tr '\n' ' ' <"$hls_events")" = 'master child ' ] || \
  fail 'an unsafe HLS segment reached a media request'
assert_hls_fixture_clean

if command -v powershell.exe >/dev/null && command -v wslpath >/dev/null; then
  for script in "$runner" "$stopper"; do
    windows_script="$(wslpath -w "$script")"
    powershell.exe -NoProfile -Command '& {
      param($path)
      $tokens = $null
      $errors = $null
      [System.Management.Automation.Language.Parser]::ParseFile(
        $path, [ref]$tokens, [ref]$errors
      ) | Out-Null
      if ($errors.Count -gt 0) {
        $errors | ForEach-Object { [Console]::Error.WriteLine($_.Message) }
        exit 1
      }
    }' "$windows_script"
  done

  # The ACL fixture uses a Windows-local path. It checks the effective rule set.
  windows_acl_root="$(powershell.exe -NoProfile -Command \
    '[IO.Path]::Combine([IO.Path]::GetTempPath(), "bibites-acl-" + [guid]::NewGuid().ToString("N"))' |
    tr -d '\r')"
  windows_acl_wsl="$(wslpath -u "$windows_acl_root")"
  multiverse_root="$windows_acl_wsl"
  mkdir -p "$multiverse_root/data"
  printf '%s\n' "$fixture_peer" >"$multiverse_root/data/peer-id"
  printf '%s\n' "$fixture_secret" >"$multiverse_root/peer-secret.txt"
  protect_windows_identity
  powershell.exe -NoProfile -Command '& {
    param([string]$Root)
    $sid = [System.Security.Principal.WindowsIdentity]::GetCurrent().User
    foreach ($item in @(Get-Item -LiteralPath $Root -Force) + @(Get-ChildItem -LiteralPath $Root -Force -Recurse)) {
      $acl = $item.GetAccessControl([System.Security.AccessControl.AccessControlSections]::Access)
      $rules = @($acl.GetAccessRules($true, $true, [System.Security.Principal.SecurityIdentifier]))
      $other = @($rules | Where-Object { $_.AccessControlType -ne "Allow" -or $_.IdentityReference -ne $sid })
      if (-not $acl.AreAccessRulesProtected -or $rules.Count -ne 1 -or $other.Count -ne 0) { exit 1 }
    }
  }' "$windows_acl_root"

  runner_windows="$(wslpath -w "$runner")"
  powershell.exe -NoProfile -ExecutionPolicy Bypass -File \
    "$runner_windows" -ArgumentQuotingSelfTest
fi

printf 'broadcast presentation, identity, lock, ACL, argv, map, and HLS readiness: PASS\n'
