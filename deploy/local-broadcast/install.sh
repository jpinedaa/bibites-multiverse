#!/usr/bin/env bash
# Install one temporary Windows broadcast world as WSL user services.
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source_game="${BIBITES_LOCAL_GAME_DIR:-/mnt/c/Program Files (x86)/Steam/steamapps/common/The Bibites}"
source_obs="${BIBITES_WINDOWS_OBS_DIR:-/mnt/c/Program Files/obs-studio}"
profile="${AWS_PROFILE:-bibites-multiverse}"
region="${AWS_REGION:-us-east-1}"
expected_account="${BIBITES_AWS_ACCOUNT_ID:-}"
origin_ssh="${BIBITES_STREAM_ORIGIN_SSH:-}"
origin_private_ip="${BIBITES_STREAM_ORIGIN_PRIVATE_IP:-}"
cloud_stack="${BIBITES_CLOUD_STACK_NAME:-bibites-cloud-worlds}"
world_name="${BIBITES_LOCAL_WORLD_NAME:-Broadcast-Live}"
public_map="${BIBITES_PUBLIC_MAP:-$repo/release/kit/public-map.json}"
sidecar_port="${BIBITES_BROADCAST_SIDECAR_PORT:-8787}"
export_edges="${BIBITES_BROADCAST_EXPORT_EDGES:-E,N,W,S}"
exclude_species="${BIBITES_BROADCAST_EXCLUDE_SPECIES:-Basic bibite}"
release_id="${BIBITES_BROADCAST_RELEASE:-local-broadcast}"
start=1

config_root="${XDG_CONFIG_HOME:-$HOME/.config}/bibites-local-broadcast"
runtime_root="$HOME/.local/lib/bibites-local-broadcast"
unit_root="$HOME/.config/systemd/user"

export DOTNET_ROOT="${DOTNET_ROOT:-$HOME/.dotnet}"
export PATH="$DOTNET_ROOT:$PATH"

say() { printf '[local-broadcast] %s\n' "$*"; }
die() { printf '[local-broadcast] ERROR: %s\n' "$*" >&2; exit 1; }

acquire_install_lock() {
  install -d -m 0700 "$config_root"
  exec {install_lock_fd}>"$config_root/install.lock"
  flock -n "$install_lock_fd" || \
    die 'another local broadcast installation is active'
}

protect_windows_identity() {
  local windows_identity_root
  windows_identity_root="$(wslpath -w "$multiverse_root")"
  powershell.exe -NoProfile -Command '& {
    param([string]$Root)
    $ErrorActionPreference = "Stop"
    $sid = [System.Security.Principal.WindowsIdentity]::GetCurrent().User

    function Set-PrivateAcl([System.IO.FileSystemInfo]$Item) {
      if ($Item.PSIsContainer) {
        $acl = New-Object System.Security.AccessControl.DirectorySecurity
        $rule = New-Object System.Security.AccessControl.FileSystemAccessRule(
          $sid, "FullControl", "ContainerInherit,ObjectInherit", "None", "Allow"
        )
      } else {
        $acl = New-Object System.Security.AccessControl.FileSecurity
        $rule = New-Object System.Security.AccessControl.FileSystemAccessRule(
          $sid, "FullControl", "Allow"
        )
      }
      $acl.SetOwner($sid)
      $acl.SetAccessRuleProtection($true, $false)
      $acl.AddAccessRule($rule)
      $Item.SetAccessControl($acl)

      $actual = $Item.GetAccessControl([System.Security.AccessControl.AccessControlSections]::Access)
      if (-not $actual.AreAccessRulesProtected) {
        throw "ACL inheritance remains enabled on $($Item.FullName)"
      }
      $rules = @($actual.GetAccessRules(
        $true, $true, [System.Security.Principal.SecurityIdentifier]
      ))
      $other = @($rules | Where-Object {
        $_.AccessControlType -ne "Allow" -or $_.IdentityReference -ne $sid
      })
      if ($rules.Count -ne 1 -or $other.Count -ne 0) {
        throw "ACL includes an account other than the current Windows user on $($Item.FullName)"
      }
    }

    $rootItem = Get-Item -LiteralPath $Root -Force
    Set-PrivateAcl $rootItem
    Get-ChildItem -LiteralPath $Root -Force -Recurse | ForEach-Object {
      Set-PrivateAcl $_
    }
  }' "$windows_identity_root" || \
    die 'Windows could not enforce the current-user-only ACL on the broadcast identity'
}

pending_identity() {
  jq -e '.format == "bibites-multiverse/enrollment-pending/1" and
         (.installId | type == "string") and (.secret | type == "string")' \
    "$pending_file" >/dev/null 2>&1 || \
    die "$pending_file is not an enrollment record this installer can use"
  install_id="$(jq -r '.installId' "$pending_file")"
  secret="$(jq -r '.secret' "$pending_file")"
  [[ "$install_id" =~ ^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$ ]] || \
    die "$pending_file holds an invalid installation id"
  [[ "$secret" =~ ^[0-9a-f]{64}$ ]] || \
    die "$pending_file holds an invalid credential"
  install_id="$(printf '%s' "$install_id" | tr 'A-F' 'a-f')"
  expected_peer="public-${install_id//-/}"
}

install_id_for_peer() {
  local peer_hex="${1#public-}"
  printf '%s-%s-%s-%s-%s\n' \
    "${peer_hex:0:8}" "${peer_hex:8:4}" "${peer_hex:12:4}" \
    "${peer_hex:16:4}" "${peer_hex:20:12}"
}

request_public_identity() {
  local requested_install_id="$1"
  local requested_secret="$2"
  local requested_peer="$3"
  local enrollment_response
  say "checking the broadcast identity with $enrollment_url"
  enrollment_response="$(
    printf '%s\n' "$requested_secret" |
      jq -Rn --arg id "$requested_install_id" --arg release "$release_id" \
        'input as $secret |
         {format:"bibites-multiverse/enrollment-request/1",installId:$id,secret:$secret,release:$release}' |
      curl -fsS --max-time 30 -X POST -H 'Content-Type: application/json' \
        --data-binary @- "$enrollment_url"
  )" || die 'the public map did not accept the broadcast identity; no live service was stopped'
  jq -e --arg peer "$requested_peer" --arg relay "$relay_url" \
    '.format == "bibites-multiverse/enrollment-response/1" and
     .peerId == $peer and .relayUrl == $relay' \
    <<<"$enrollment_response" >/dev/null || \
    die 'the public map answered for a different identity or relay'
}

write_completed_identity() {
  local entry_umask
  entry_umask="$(umask)"
  umask 077
  printf '%s\n' "$expected_peer" >"$durable_peer_file"
  printf '%s\n' "$secret" >"$credential_file"
  jq -nc --arg peer "$expected_peer" --arg relay "$relay_url" --arg world "$world_name" \
    '{format:"bibites-multiverse/broadcast-identity/1",peerId:$peer,relayUrl:$relay,world:$world}' \
    >"$record_file"
  umask "$entry_umask"
  protect_windows_identity
}

preflight_public_identity() {
  [ -f "$public_map" ] || die "missing public join configuration: $public_map"
  jq -e '.format == "bibites-multiverse/public-map/1"' "$public_map" >/dev/null || \
    die "$public_map has an unsupported format"
  relay_url="$(jq -r '.relayUrl // ""' "$public_map")"
  enrollment_url="$(jq -r '.enrollmentUrl // ""' "$public_map")"
  [[ "$relay_url" =~ ^wss://[^[:space:]]+$ ]] || \
    die "$public_map has no secure wss:// relay address"
  [[ "$enrollment_url" =~ ^https://[^[:space:]]+$ ]] || \
    die "$public_map has no secure https:// enrollment address"

  local record_held=0 credential_held=0 durable_peer_held=0 pending_held=0
  [ -f "$record_file" ] && record_held=1
  [ -f "$credential_file" ] && credential_held=1
  [ -f "$durable_peer_file" ] && durable_peer_held=1
  [ -f "$pending_file" ] && pending_held=1
  if [ -e "$multiverse_data" ] && [ ! -d "$multiverse_data" ]; then
    die "$multiverse_data is not a directory; no identity was changed"
  fi
  if [ "$durable_peer_held" -eq 0 ] && [ -d "$multiverse_data" ]; then
    local durable_entry
    durable_entry="$(find "$multiverse_data" -mindepth 1 -print -quit)" || \
      die "$multiverse_data cannot be inspected; no identity was changed"
    [ -z "$durable_entry" ] || \
      die "$multiverse_data holds custody state without a durable peer identity; no identity was changed"
  fi

  install_id=''
  secret=''
  expected_peer=''
  if [ "$pending_held" -eq 1 ]; then
    pending_identity
  fi

  if [ "$record_held" -eq 1 ] && [ "$credential_held" -eq 1 ] && [ "$durable_peer_held" -eq 1 ]; then
    local recorded_peer recorded_relay recorded_world recorded_secret durable_peer
    jq -e '.format == "bibites-multiverse/broadcast-identity/1"' "$record_file" >/dev/null 2>&1 || \
      die "$record_file is not a completed broadcast identity"
    recorded_peer="$(jq -r '.peerId // ""' "$record_file")"
    recorded_relay="$(jq -r '.relayUrl // ""' "$record_file")"
    recorded_world="$(jq -r '.world // ""' "$record_file")"
    recorded_secret="$(tr -d '\r' <"$credential_file")"
    [[ "$recorded_peer" =~ ^public-[0-9a-f]{32}$ ]] || \
      die "$record_file holds an invalid peer identity"
    [ "$recorded_relay" = "$relay_url" ] || \
      die "$record_file names a different relay"
    [ "$recorded_world" = "$world_name" ] || \
      die "$record_file belongs to world '$recorded_world', not '$world_name'"
    [[ "$recorded_secret" =~ ^[0-9a-f]{64}$ ]] || \
      die "$credential_file holds an invalid credential"
    durable_peer="$(tr -d '\r' <"$durable_peer_file")"
    [ "$durable_peer" = "$recorded_peer" ] || \
      die "$durable_peer_file disagrees with the completed identity"
    if [ "$pending_held" -eq 1 ]; then
      [ "$expected_peer" = "$recorded_peer" ] && [ "$secret" = "$recorded_secret" ] || \
        die 'the pending and completed broadcast identities disagree; neither identity was changed'
    fi

    expected_peer="$recorded_peer"
    secret="$recorded_secret"
    install_id="$(install_id_for_peer "$recorded_peer")"
    protect_windows_identity
    request_public_identity "$install_id" "$secret" "$expected_peer"
    if [ "$pending_held" -eq 1 ]; then
      rm -f "$pending_file"
    fi
    peer_id="$recorded_peer"
    say "reusing the broadcast world's map identity $peer_id"
    return
  fi

  if [ "$pending_held" -eq 1 ] && \
     { [ "$record_held" -eq 1 ] || [ "$credential_held" -eq 1 ] || [ "$durable_peer_held" -eq 1 ]; }; then
    local partial_value
    if [ "$record_held" -eq 1 ]; then
      jq -e --arg peer "$expected_peer" --arg relay "$relay_url" --arg world "$world_name" \
        '.format == "bibites-multiverse/broadcast-identity/1" and
         .peerId == $peer and .relayUrl == $relay and .world == $world' \
        "$record_file" >/dev/null 2>&1 || \
        die 'the pending identity disagrees with the partial enrollment record'
    fi
    if [ "$credential_held" -eq 1 ]; then
      partial_value="$(tr -d '\r' <"$credential_file")"
      [ "$partial_value" = "$secret" ] || \
        die 'the pending identity disagrees with the partial credential'
    fi
    if [ "$durable_peer_held" -eq 1 ]; then
      partial_value="$(tr -d '\r' <"$durable_peer_file")"
      [ "$partial_value" = "$expected_peer" ] || \
        die 'the pending identity disagrees with the partial durable peer ID'
    fi
    protect_windows_identity
    request_public_identity "$install_id" "$secret" "$expected_peer"
    write_completed_identity
    rm -f "$pending_file"
    peer_id="$expected_peer"
    say "completed the interrupted broadcast identity $peer_id"
    return
  fi

  if [ "$record_held" -eq 1 ] || [ "$credential_held" -eq 1 ] || [ "$durable_peer_held" -eq 1 ]; then
    die "$multiverse_root holds an incomplete map identity; no identity was changed"
  fi

  install -d -m 0700 "$multiverse_root" "$multiverse_data"
  if [ "$pending_held" -eq 0 ]; then
    # Protect the empty directory first. The pending secret then inherits a
    # current-user-only ACL from its first write.
    protect_windows_identity
    install_id="$(tr 'A-F' 'a-f' </proc/sys/kernel/random/uuid | tr -d '\r\n')"
    secret="$(head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n')"
    expected_peer="public-${install_id//-/}"
    local entry_umask
    entry_umask="$(umask)"
    umask 077
    printf '%s\n' "$secret" |
      jq -Rn --arg id "$install_id" \
      'input as $secret |
       {format:"bibites-multiverse/enrollment-pending/1",installId:$id,secret:$secret}' \
      >"$pending_file"
    umask "$entry_umask"
  else
    say 'retrying the pending public-map enrollment'
  fi
  protect_windows_identity
  request_public_identity "$install_id" "$secret" "$expected_peer"
  write_completed_identity
  rm -f "$pending_file"
  peer_id="$expected_peer"
  say "the public map created the broadcast identity $peer_id"
}

status_has_ready_peer() {
  local expected_peer_id="$1"
  local expected_edges_json="$2"
  jq -e --arg peer "$expected_peer_id" --argjson edges "$expected_edges_json" \
    'any(.slots[]?;
      .peerId == $peer and .live == true and .modConnected == true and
      ((.exportEdges // []) | sort) == ($edges | sort))' >/dev/null
}

hls_relative_reference() {
  local reference="$1"
  local kind="$2"

  [ -n "$reference" ] || return 1
  [[ "$reference" =~ ^[A-Za-z0-9._~-]+(/[A-Za-z0-9._~-]+)*$ ]] || return 1
  if [[ "$reference" =~ (^|/)\.\.?(/|$) ]]; then
    return 1
  fi

  case "$kind:$reference" in
    playlist:*.m3u8) return 0 ;;
    segment:*.mp4|segment:*.m4s|segment:*.ts) return 0 ;;
    *) return 1 ;;
  esac
}

hls_probe_cleanup() {
  local probe_dir="$1"
  [ -d "$probe_dir" ] || return 0
  find "$probe_dir" -depth -delete >/dev/null 2>&1
}

hls_probe_fail() {
  local probe_dir="$1"
  local reason="$2"
  hls_probe_error="$reason"
  if ! hls_probe_cleanup "$probe_dir"; then
    hls_probe_error='the HLS readiness check could not remove its temporary files'
  fi
  return 1
}

hls_stream_ready() {
  local manifest_url="$1"
  local probe_parent="${TMPDIR:-/tmp}"
  local probe_dir cookie_jar master_file child_file
  local manifest_dir child_reference child_url child_dir segment_reference segment_url
  local first_line

  hls_probe_error=''
  if [[ "$manifest_url" != https://*/index.m3u8 ]]; then
    hls_probe_error='the HLS master URL is not a secure index.m3u8 URL'
    return 1
  fi

  probe_dir="$(mktemp -d "$probe_parent/bibites-hls.XXXXXX")" || {
    hls_probe_error='the HLS readiness check could not create a temporary directory'
    return 1
  }
  cookie_jar="$probe_dir/cookies"
  master_file="$probe_dir/master.m3u8"
  child_file="$probe_dir/child.m3u8"

  if ! curl --fail --silent --show-error --location --max-redirs 3 \
      --connect-timeout 5 --max-time 20 --proto '=https' --proto-redir '=https' \
      --cookie '' --cookie-jar "$cookie_jar" --output "$master_file" "$manifest_url"; then
    hls_probe_fail "$probe_dir" 'the public HLS master request failed'
    return 1
  fi
  if ! awk -F '\t' \
      '$6 == "hlsSession" && length($7) > 0 { found=1 } END { exit !found }' \
      "$cookie_jar"; then
    hls_probe_fail "$probe_dir" 'the public HLS master did not create a session cookie'
    return 1
  fi
  first_line="$(head -n 1 "$master_file" | tr -d '\r')"
  if [ "$first_line" != '#EXTM3U' ]; then
    hls_probe_fail "$probe_dir" 'the public HLS master is not an M3U8 playlist'
    return 1
  fi

  child_reference="$(
    awk '{ sub(/\r$/, "") }
         NF && substr($0, 1, 1) != "#" && $0 ~ /\.m3u8$/ { print; exit }' \
      "$master_file"
  )"
  if ! hls_relative_reference "$child_reference" playlist; then
    hls_probe_fail "$probe_dir" 'the public HLS master has no safe relative child playlist'
    return 1
  fi
  manifest_dir="${manifest_url%/*}"
  child_url="$manifest_dir/$child_reference"
  if ! curl --fail --silent --show-error --connect-timeout 5 --max-time 20 \
      --proto '=https' --cookie "$cookie_jar" --output "$child_file" "$child_url"; then
    hls_probe_fail "$probe_dir" 'the public HLS child playlist request failed'
    return 1
  fi
  first_line="$(head -n 1 "$child_file" | tr -d '\r')"
  if [ "$first_line" != '#EXTM3U' ]; then
    hls_probe_fail "$probe_dir" 'the public HLS child is not an M3U8 playlist'
    return 1
  fi

  segment_reference="$(
    awk '{ sub(/\r$/, "") }
         NF && substr($0, 1, 1) != "#" { latest=$0 }
         END { print latest }' "$child_file"
  )"
  if ! hls_relative_reference "$segment_reference" segment; then
    hls_probe_fail "$probe_dir" 'the public HLS child has no safe completed segment'
    return 1
  fi
  child_dir="${child_url%/*}"
  segment_url="$child_dir/$segment_reference"
  if ! curl --fail --silent --show-error --connect-timeout 5 --max-time 20 \
      --proto '=https' --cookie "$cookie_jar" --output /dev/null "$segment_url"; then
    hls_probe_fail "$probe_dir" 'the latest completed HLS segment request failed'
    return 1
  fi

  if ! hls_probe_cleanup "$probe_dir"; then
    hls_probe_error='the HLS readiness check could not remove its temporary files'
    return 1
  fi
  return 0
}

if [[ "${BASH_SOURCE[0]}" != "$0" ]]; then
  return 0
fi

while [ $# -gt 0 ]; do
  case "$1" in
    --game-dir) source_game="${2:?--game-dir needs a path}"; shift ;;
    --world) world_name="${2:?--world needs a name}"; shift ;;
    --no-start) start=0 ;;
    -h|--help) sed -n '2,52p' "$0"; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
  shift
done

case "$world_name" in *[!A-Za-z0-9._-]*|'') die 'the world name contains an unsupported character' ;; esac
[[ "$expected_account" =~ ^[0-9]{12}$ ]] || die 'set BIBITES_AWS_ACCOUNT_ID to the 12-digit AWS account ID'
[ -n "$origin_ssh" ] || die 'set BIBITES_STREAM_ORIGIN_SSH to the stream-origin SSH destination'
[ -n "$origin_private_ip" ] || die 'set BIBITES_STREAM_ORIGIN_PRIVATE_IP to the stream-origin private IP address'
[[ "$sidecar_port" =~ ^[0-9]{2,5}$ ]] || die 'BIBITES_BROADCAST_SIDECAR_PORT is not a port number'
[ -n "$exclude_species" ] || die 'BIBITES_BROADCAST_EXCLUDE_SPECIES is empty, which turns the exclusion policy off'
[ -n "$export_edges" ] || die 'BIBITES_BROADCAST_EXPORT_EDGES names no edge; the broadcast world must stay on the map'
[ -n "$release_id" ] && [ "${#release_id}" -le 32 ] || \
  die 'BIBITES_BROADCAST_RELEASE must contain 1 to 32 characters'
for edge in ${export_edges//,/ }; do
  case "$edge" in E|N|W|S) ;; *) die "BIBITES_BROADCAST_EXPORT_EDGES holds '$edge'; use E, N, W or S" ;; esac
done
for command in aws awk curl dotnet find flock go head install jq mktemp nvidia-smi powershell.exe session-manager-plugin sha256sum ssh tmux tr wslpath; do
  command -v "$command" >/dev/null || die "missing command: $command"
done
acquire_install_lock
expected_edges_json="$(jq -nc --arg edges "$export_edges" \
  '$edges | gsub("[[:space:]]"; "") | split(",")')"
jq -e 'length > 0 and length == (unique | length) and
       all(.[]; . == "E" or . == "N" or . == "W" or . == "S")' \
  <<<"$expected_edges_json" >/dev/null || \
  die 'BIBITES_BROADCAST_EXPORT_EDGES must name each selected edge once'
export_edges="$(jq -r 'join(",")' <<<"$expected_edges_json")"
[ -f "$source_game/The Bibites.exe" ] || die "missing Windows game in $source_game"
[ -f "$source_game/winhttp.dll" ] || die "BepInEx is not installed in $source_game"
[ -f "$source_obs/bin/64bit/obs64.exe" ] || die "missing Windows OBS: $source_obs"

assembly="$source_game/The Bibites_Data/Managed/BibitesAssembly.dll"
[ -f "$assembly" ] || die "missing game assembly: $assembly"
assembly_sha="$(sha256sum "$assembly" | awk '{print toupper($1)}')"
[ "$assembly_sha" = 12455E485199CDBCAEA5978B8B0095EEDCBDD09D1FB87EFD65CCACB15D96E7EE ] || \
  die "the game assembly hash is not the supported Windows 0.6.3.1 build: $assembly_sha"

account="$(aws --profile "$profile" sts get-caller-identity --query Account --output text)"
[ "$account" = "$expected_account" ] || die "wrong AWS account: $account"
powershell_bin="$(command -v powershell.exe)"
nvidia-smi >/dev/null || die 'the local NVIDIA GPU is not ready'

publish_password="$(ssh -o BatchMode=yes "$origin_ssh" \
  "sudo sed -n 's/^MV_STREAM_PUBLISH_PASSWORD=//p' /etc/multiverse/stream-publish.env")"
[[ "$publish_password" =~ ^[0-9a-f]{64}$ ]] || die 'the origin publish password is malformed'

windows_local_appdata="$(powershell.exe -NoProfile -Command \
  '[Environment]::GetFolderPath("LocalApplicationData")' | tr -d '\r')"
[ -n "$windows_local_appdata" ] || die 'Windows did not return the local application-data path'
windows_root="$windows_local_appdata\\BibitesMultiverse\\broadcast"
windows_root_wsl="$(wslpath -u "$windows_root")"
game_dir="$windows_root_wsl/game"
obs_dir="$windows_root_wsl/obs"

multiverse_root="$windows_root_wsl/multiverse"
multiverse_data="$multiverse_root/data"
credential_file="$multiverse_root/peer-secret.txt"
record_file="$multiverse_root/enrollment.json"
pending_file="$multiverse_root/enrollment-pending.json"
durable_peer_file="$multiverse_data/peer-id"
peer_id=''

# Complete every public-map and identity check before this installer stops a
# live process or replaces a runtime file.
preflight_public_identity

private_assembly="$game_dir/The Bibites_Data/Managed/BibitesAssembly.dll"
if [ -f "$game_dir/The Bibites.exe" ]; then
  [ -f "$private_assembly" ] || die "the private game copy is incomplete: $private_assembly"
  private_assembly_sha="$(sha256sum "$private_assembly" | awk '{print toupper($1)}')"
  [ "$private_assembly_sha" = "$assembly_sha" ] || \
    die 'the private game copy does not match the installed game; remove it and reinstall'
fi

tmux -L bibites-broadcast kill-session -t bibites-local-broadcast >/dev/null 2>&1 || true
systemctl --user disable --now bibites-local-broadcast-windows.service >/dev/null 2>&1 || true
systemctl --user stop bibites-local-broadcast-tunnel.service \
  bibites-local-broadcast.target >/dev/null 2>&1 || true
powershell.exe -NoProfile -Command \
  'Unregister-ScheduledTask -TaskName "BibitesLocalBroadcast" -Confirm:$false -ErrorAction SilentlyContinue' || true
existing_stopper="$windows_root_wsl/stop-windows.ps1"
if [ -f "$existing_stopper" ]; then
  existing_stopper_windows="$(wslpath -w "$existing_stopper")"
  powershell.exe -NoProfile -ExecutionPolicy RemoteSigned -File \
    "$existing_stopper_windows" >/dev/null || true
fi
private_game_exe_windows="$(wslpath -w "$game_dir/The Bibites.exe")"
for _ in $(seq 1 60); do
  private_games="$(powershell.exe -NoProfile -Command \
    '& { param($path) @(Get-Process -Name "The Bibites" -ErrorAction SilentlyContinue | Where-Object { $_.Path -eq $path }).Count }' \
    "$private_game_exe_windows" | tr -d '\r')"
  [ "$private_games" = 0 ] && break
  sleep 1
done
[ "$private_games" = 0 ] || die 'the private broadcast game did not stop'
private_obs_exe_windows="$(wslpath -w "$obs_dir/bin/64bit/obs64.exe")"
for _ in $(seq 1 30); do
  private_obs="$(powershell.exe -NoProfile -Command \
    '& { param($path) @(Get-Process -Name "obs64" -ErrorAction SilentlyContinue | Where-Object { $_.Path -eq $path }).Count }' \
    "$private_obs_exe_windows" | tr -d '\r')"
  [ "$private_obs" = 0 ] && break
  sleep 1
done
[ "$private_obs" = 0 ] || die 'the private OBS process did not stop'
# A running sidecar holds this world's place on the map and locks its own
# executable, which the build below replaces.
private_sidecar_exe_windows="$(wslpath -w "$windows_root_wsl/multiverse-sidecar.exe")"
for _ in $(seq 1 30); do
  private_sidecars="$(powershell.exe -NoProfile -Command \
    '& { param($path) @(Get-Process -Name "multiverse-sidecar" -ErrorAction SilentlyContinue | Where-Object { $_.Path -eq $path }).Count }' \
    "$private_sidecar_exe_windows" | tr -d '\r')"
  [ "$private_sidecars" = 0 ] && break
  sleep 1
done
[ "$private_sidecars" = 0 ] || die 'the private broadcast sidecar did not stop'
install -d -m 0700 "$config_root"
install -d -m 0755 "$runtime_root/bin" "$unit_root" "$windows_root_wsl" \
  "$windows_root_wsl/logs" "$windows_root_wsl/state"
if [ ! -f "$game_dir/The Bibites.exe" ]; then
  say 'copying the Windows game into the private broadcast directory'
  cp -a "$source_game/." "$game_dir/"
fi
if [ ! -f "$obs_dir/bin/64bit/obs64.exe" ]; then
  say 'copying OBS into the private broadcast directory'
  cp -a "$source_obs/." "$obs_dir/"
fi
[ -f "$private_assembly" ] || die "the private game copy is incomplete: $private_assembly"
private_assembly_sha="$(sha256sum "$private_assembly" | awk '{print toupper($1)}')"
[ "$private_assembly_sha" = "$assembly_sha" ] || \
  die 'the private game copy does not match the installed game; remove it and reinstall'
# Current Windows builds can prefer protected system DLLs over the BepInEx
# proxies in the game directory. The executable-local marker enables DLL
# redirection. Doorstop also supports version.dll as an alternate proxy name.
install -m 0644 "$repo/deploy/local-broadcast/doorstop.local" \
  "$game_dir/The Bibites.exe.local"
install -m 0644 "$game_dir/winhttp.dll" "$game_dir/version.dll"

dotnet build "$repo/bibites-mod/BibitesMultiverse.csproj" -c Release --nologo >/dev/null
plugin="$repo/bibites-mod/bin/Release/BibitesMultiverse.dll"
[ -f "$plugin" ] || die "the plugin build did not create $plugin"
install -m 0644 "$plugin" "$game_dir/BepInEx/plugins/BibitesMultiverse.dll"

# The broadcast world is a participant of the public map, so it needs the same
# two halves every participant needs: a sidecar that speaks Contract B, and one
# identity of its own. It never copies another world's credential.
sidecar_exe="$windows_root_wsl/multiverse-sidecar.exe"
say 'building the Windows sidecar for the broadcast world'
( cd "$repo/go" && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 nice -n 19 \
    go build -trimpath -o "$sidecar_exe" ./cmd/sidecar )
[ -f "$sidecar_exe" ] || die "the sidecar build did not create $sidecar_exe"

install -m 0644 "$repo/deploy/local-broadcast/run-windows.ps1" "$windows_root_wsl/run-windows.ps1"
install -m 0644 "$repo/deploy/local-broadcast/stop-windows.ps1" "$windows_root_wsl/stop-windows.ps1"
for script in run-loop run-tunnel run-windows start stop stop-windows; do
  install -m 0755 "$repo/deploy/local-broadcast/$script" "$runtime_root/bin/$script"
done
for unit in bibites-local-broadcast.target bibites-local-broadcast-tunnel.service; do
  install -m 0644 "$repo/deploy/local-broadcast/$unit" "$unit_root/$unit"
done

runner_windows="$(wslpath -w "$windows_root_wsl/run-windows.ps1")"
stopper_windows="$(wslpath -w "$windows_root_wsl/stop-windows.ps1")"
game_windows="$(wslpath -w "$game_dir")"
obs_windows="$(wslpath -w "$obs_dir/bin/64bit/obs64.exe")"
windows_config="$windows_root_wsl/config.env"
obs_config="$obs_dir/config/obs-studio"
obs_profile="$obs_config/basic/profiles/BibitesBroadcast"
obs_scenes="$obs_config/basic/scenes"
umask 077
install -d -m 0700 "$obs_profile" "$obs_scenes"
install -m 0600 "$repo/deploy/local-broadcast/obs-basic.ini" "$obs_profile/basic.ini"
install -m 0600 "$repo/deploy/local-broadcast/obs-scene.json" \
  "$obs_scenes/BibitesBroadcast.json"
printf '{"settings":{"bwtest":false,"key":"bibites?user=broadcaster&pass=%s","server":"rtmp://127.0.0.1:1935","use_auth":false},"type":"rtmp_custom"}\n' \
  "$publish_password" >"$obs_profile/service.json"
chmod 0600 "$obs_profile/service.json"
{
  printf 'GameDir=%s\n' "$game_windows"
  printf 'Obs=%s\n' "$obs_windows"
  printf 'WorldName=%s\n' "$world_name"
  printf 'PublishPort=1935\n'
  printf 'SidecarExe=%s\n' "$(wslpath -w "$sidecar_exe")"
  printf 'DataRoot=%s\n' "$(wslpath -w "$multiverse_root")"
  printf 'RelayUrl=%s\n' "$relay_url"
  printf 'PeerId=%s\n' "$peer_id"
  printf 'SidecarPort=%s\n' "$sidecar_port"
  printf 'ExportEdges=%s\n' "$export_edges"
  printf 'ExcludeSpecies=%s\n' "$exclude_species"
} >"$windows_config"

config_file="$config_root/broadcast.env"
{
  printf 'AWS_PROFILE=%q\n' "$profile"
  printf 'AWS_REGION=%q\n' "$region"
  printf 'BIBITES_AWS_ACCOUNT_ID=%q\n' "$expected_account"
  printf 'CLOUD_STACK=%q\n' "$cloud_stack"
  printf 'STREAM_ORIGIN_PRIVATE_IP=%q\n' "$origin_private_ip"
  printf 'STREAM_LOCAL_PORT=%q\n' 1935
  printf 'POWERSHELL_BIN=%q\n' "$powershell_bin"
  printf 'WINDOWS_RUNNER=%q\n' "$runner_windows"
  printf 'WINDOWS_STOPPER=%q\n' "$stopper_windows"
} >"$config_file"
chmod 0600 "$config_file"

powershell.exe -NoProfile -ExecutionPolicy RemoteSigned -File "$stopper_windows" >/dev/null
rm -f "$windows_root_wsl/state/director.json" "$windows_root_wsl/state/command.txt"
rm -f "$unit_root/bibites-local-broadcast-windows.service"
systemctl --user daemon-reload
systemctl --user disable bibites-local-broadcast.target >/dev/null 2>&1 || true
if [ "$start" -eq 1 ]; then
  # Keep the install lock in this process through verification. Do not pass the
  # descriptor to the long-lived broadcaster processes that start creates.
  "$runtime_root/bin/start" {install_lock_fd}>&-
  status_url="${relay_url/#wss:\/\//https://}"
  status_url="${status_url%/contract-b/v4}/api/status"
  for _ in $(seq 1 120); do
    if curl -fsS --max-time 10 "$status_url" 2>/dev/null |
        status_has_ready_peer "$peer_id" "$expected_edges_json"; then
      say "the broadcast world is ready on the map as $peer_id"
      break
    fi
    sleep 2
  done
  curl -fsS --max-time 10 "$status_url" |
    status_has_ready_peer "$peer_id" "$expected_edges_json" || \
    die "the map does not show $peer_id with its game and expected export edges"
  public_manifest='https://bibitesmultiverse.com/stream/bibites/index.m3u8'
  hls_ready=0
  for _ in $(seq 1 90); do
    if hls_stream_ready "$public_manifest" >/dev/null 2>&1; then
      hls_ready=1
      say 'the public HLS master, child playlist, and latest completed segment are live'
      break
    fi
    sleep 2
  done
  [ "$hls_ready" -eq 1 ] || \
    die "the local services started, but ${hls_probe_error:-the public HLS session is not ready}"
fi

say "installed Windows world=$world_name peer=$peer_id data=$windows_root"
say "the world holds its own map identity; it never copies another world's credential"
say 'use tmux -L bibites-broadcast has-session -t bibites-local-broadcast to read its state'
