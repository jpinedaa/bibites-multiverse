#!/usr/bin/env bash
# Copy the five established Windows worlds into ignored local migration staging.
# The originals are read-only inputs. The script refuses a live game because a
# save copied while Unity is replacing it is not a migration checkpoint.
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
out="$repo/cloud/aws/imports"
ps='/mnt/c/Windows/System32/WindowsPowerShell/v1.0/powershell.exe'

running="$($ps -NoProfile -NonInteractive -Command \
  '(Get-Process -Name "The Bibites" -ErrorAction SilentlyContinue | Measure-Object).Count' \
  </dev/null | tr -d '\r')"
[ "${running:-0}" = 0 ] || {
  echo "$running Bibites process(es) are running. Stop them cleanly so save-on-quit lands, then retry." >&2
  exit 1
}

save_win="$($ps -NoProfile -NonInteractive -Command \
  'Join-Path $env:USERPROFILE "AppData\LocalLow\The Bibites\The Bibites\Savefiles"' \
  </dev/null | tr -d '\r')"
save_dir="$(wslpath -u "$save_win")"
install -d -m 0700 "$out"
contents="$(mktemp)"
trap 'rm -f "$contents"' EXIT

for n in 1 2 3 4 5; do
  src="$save_dir/M4-Slot$n.zip"
  dst="$out/M4-Slot$n.zip"
  [ -f "$src" ] || { echo "missing $src" >&2; exit 1; }
  cp -p "$src" "$dst.new"
  unzip -tqq "$dst.new"
  unzip -Z1 "$dst.new" > "$contents"
  grep -Fxq 'scene.bb8scene' "$contents"
  mv "$dst.new" "$dst"
  chmod 0600 "$dst"
  printf 'captured M4-Slot%s  %s bytes  %s\n' "$n" "$(stat -c %s "$dst")" "$(sha256sum "$dst" | awk '{print $1}')"
done

sha256sum "$out"/M4-Slot{1,2,3,4,5}.zip > "$out/SHA256SUMS"
chmod 0600 "$out/SHA256SUMS"
printf 'PC migration staging: %s\n' "$out"
