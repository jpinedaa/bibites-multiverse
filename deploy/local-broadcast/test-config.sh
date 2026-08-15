#!/usr/bin/env bash
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
runner="$here/run-windows.ps1"

expect_text() {
  local file="$1"
  local text="$2"
  grep -Fq "$text" "$file" || {
    printf 'missing expected setting in %s: %s\n' "$file" "$text" >&2
    exit 1
  }
}

expect_text "$runner" "MULTIVERSE_BROADCAST_ZOOM = '45'"
expect_text "$runner" "MULTIVERSE_BROADCAST_HIDE_UI = 'false'"
expect_text "$runner" "MULTIVERSE_BROADCAST_TIME_SCALE = '7.5'"
expect_text "$runner" "MULTIVERSE_BROADCAST_PANELS = 'brain,biology'"
expect_text "$runner" "MULTIVERSE_BROADCAST_PANEL_SECONDS = '15'"
expect_text "$runner" "MULTIVERSE_BROADCAST_SHOW_FOV = 'true'"

if command -v powershell.exe >/dev/null && command -v wslpath >/dev/null; then
  windows_runner="$(wslpath -w "$runner")"
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
  }' "$windows_runner"
fi

printf 'broadcast presentation configuration: PASS\n'
