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
start=1

config_root="${XDG_CONFIG_HOME:-$HOME/.config}/bibites-local-broadcast"
runtime_root="$HOME/.local/lib/bibites-local-broadcast"
unit_root="$HOME/.config/systemd/user"

export DOTNET_ROOT="${DOTNET_ROOT:-$HOME/.dotnet}"
export PATH="$DOTNET_ROOT:$PATH"

say() { printf '[local-broadcast] %s\n' "$*"; }
die() { printf '[local-broadcast] ERROR: %s\n' "$*" >&2; exit 1; }

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
for command in aws curl dotnet install nvidia-smi powershell.exe session-manager-plugin sha256sum ssh tmux wslpath; do
  command -v "$command" >/dev/null || die "missing command: $command"
done
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
private_assembly="$game_dir/The Bibites_Data/Managed/BibitesAssembly.dll"
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
  "$runtime_root/bin/start"
  public_manifest='https://bibitesmultiverse.com/stream/bibites/index.m3u8?cookieCheck=1'
  for _ in $(seq 1 90); do
    if curl -fsS -H 'Cookie: cookieCheck=1' "$public_manifest" >/dev/null 2>&1; then
      say 'the public HLS manifest is live'
      break
    fi
    sleep 2
  done
  curl -fsS -H 'Cookie: cookieCheck=1' "$public_manifest" >/dev/null || \
    die 'the local services started, but the public HLS manifest is not live'
fi

say "installed Windows world=$world_name data=$windows_root"
say 'the world is offline from the Multiverse map and cannot duplicate a live credential'
say 'use tmux -L bibites-broadcast has-session -t bibites-local-broadcast to read its state'
