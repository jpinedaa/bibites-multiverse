#!/usr/bin/env bash
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
runner="$here/run-windows.ps1"
stopper="$here/stop-windows.ps1"
installer="$here/install.sh"

expect_text() {
  local file="$1"
  local text="$2"
  grep -Fq "$text" "$file" || {
    printf 'missing expected setting in %s: %s\n' "$file" "$text" >&2
    exit 1
  }
}

forbid_text() {
  local file="$1"
  local text="$2"
  if grep -Fq "$text" "$file"; then
    printf 'refused setting is still in %s: %s\n' "$file" "$text" >&2
    exit 1
  fi
}

expect_text "$runner" "MULTIVERSE_BROADCAST_ZOOM = '50'"
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
grep -n "Start-Process -FilePath \$config.SidecarExe" "$runner" >/dev/null || {
  printf 'the runner does not start the sidecar\n' >&2
  exit 1
}
sidecar_line="$(grep -n 'Start-Process -FilePath \$config.SidecarExe' "$runner" | head -1 | cut -d: -f1)"
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
bash -n "$installer"

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
fi

printf 'broadcast presentation and map configuration: PASS\n'
