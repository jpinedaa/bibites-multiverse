#!/usr/bin/env bash
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=lib/validation.sh
. "$repo/cloud/aws/lib/validation.sh"

stage="$(mktemp -d)"
trap 'find "$stage" -depth -delete' EXIT
install -d "$stage/valid" "$stage/missing-scene"
printf 'scene\n' >"$stage/valid/scene.bb8scene"
printf 'metadata\n' >"$stage/valid/metadata.txt"
printf 'metadata\n' >"$stage/missing-scene/metadata.txt"
(cd "$stage/valid" && zip -q "$stage/valid.zip" scene.bb8scene metadata.txt)
(cd "$stage/missing-scene" && zip -q "$stage/missing-scene.zip" metadata.txt)
printf 'not a ZIP\n' >"$stage/invalid.zip"

bibites_require_save_archive "$stage/valid.zip" 'valid save'
if bibites_require_save_archive "$stage/missing-scene.zip" \
  'missing-scene save' >/dev/null 2>&1; then
  echo 'save without scene.bb8scene passed validation' >&2
  exit 1
fi
if bibites_require_save_archive "$stage/invalid.zip" \
  'invalid save' >/dev/null 2>&1; then
  echo 'invalid ZIP passed save validation' >&2
  exit 1
fi
if bibites_require_unique_save_basenames imports/World.zip \
  archive/World.zip >/dev/null 2>&1; then
  echo 'flat save-name collision passed validation' >&2
  exit 1
fi

save_upload_line="$(grep -n 's3 cp "$save"' "$repo/cloud/aws/stage-artifacts.sh" |
  tail -n 1 | cut -d: -f1)"
manifest_upload_line="$(grep -n 's3 cp "$dist/worlds.json"' \
  "$repo/cloud/aws/stage-artifacts.sh" | tail -n 1 | cut -d: -f1)"
[ "$manifest_upload_line" -gt "$save_upload_line" ] || {
  echo 'stage script does not publish worlds.json after every save' >&2
  exit 1
}

printf 'stage-input fixtures passed\n'
