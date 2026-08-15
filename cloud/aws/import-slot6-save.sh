#!/usr/bin/env bash
set -euo pipefail

[ $# -eq 1 ] || { echo "usage: $0 /path/to/M4-Slot6.zip" >&2; exit 2; }
src="$1"
[ -f "$src" ] || { echo "not a file: $src" >&2; exit 1; }
repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
out="$repo/cloud/aws/imports"
install -d -m 0700 "$out"
contents="$(mktemp)"
trap 'rm -f "$contents"' EXIT
cp -p "$src" "$out/M4-Slot6.zip.new"
unzip -tqq "$out/M4-Slot6.zip.new"
unzip -Z1 "$out/M4-Slot6.zip.new" > "$contents"
grep -Fxq 'scene.bb8scene' "$contents"
mv "$out/M4-Slot6.zip.new" "$out/M4-Slot6.zip"
chmod 0600 "$out/M4-Slot6.zip"
printf 'captured M4-Slot6  %s bytes  %s\n' \
  "$(stat -c %s "$out/M4-Slot6.zip")" \
  "$(sha256sum "$out/M4-Slot6.zip" | awk '{print $1}')"
