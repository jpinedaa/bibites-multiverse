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
immutable_manifest_line="$(grep -n \
  'put_immutable_input "$dist/worlds.json" "$manifest_sha256" "$manifest_object"' \
  "$repo/cloud/aws/stage-artifacts.sh" | cut -d: -f1)"
[ "$manifest_upload_line" -gt "$save_upload_line" ] || {
  echo 'stage script does not publish worlds.json after every save' >&2
  exit 1
}
[ "$manifest_upload_line" -gt "$immutable_manifest_line" ] || {
  echo 'stage script publishes worlds.json before its immutable manifest' >&2
  exit 1
}
[ "$immutable_manifest_line" -gt "$save_upload_line" ] || {
  echo 'stage script publishes the immutable manifest before every save' >&2
  exit 1
}
grep -Fq '"$repo/cloud/aws/put-immutable-object.sh"' \
  "$repo/cloud/aws/stage-artifacts.sh"
grep -Fq 'put_immutable_input "$dist/$RUNTIME_FILE" "$RUNTIME_SHA256" "$runtime_object"' \
  "$repo/cloud/aws/stage-artifacts.sh"
grep -Fq "printf 'MANIFEST_OBJECT=%q\\n'" "$repo/cloud/aws/stage-artifacts.sh"
grep -Fq "printf 'MANIFEST_SHA256=%q\\n'" "$repo/cloud/aws/stage-artifacts.sh"
grep -Fq 'staging_scope=runtime-only' "$repo/cloud/aws/stage-artifacts.sh"
grep -Fq 'staging_scope=complete' "$repo/cloud/aws/stage-artifacts.sh"
grep -Fq "printf 'STAGING_SCOPE=%q\\n'" "$repo/cloud/aws/stage-artifacts.sh"

manifest_snapshot_line="$(grep -n \
  'install -m 0600 "$BIBITES_MANIFEST_FILE" "$dist/worlds.json"' \
  "$repo/cloud/aws/stage-artifacts.sh" | cut -d: -f1)"
snapshot_validation_line="$(grep -n \
  'jq -e -f "$manifest_validator" "$dist/worlds.json"' \
  "$repo/cloud/aws/stage-artifacts.sh" | cut -d: -f1)"
[ "$manifest_snapshot_line" -lt "$snapshot_validation_line" ] || {
  echo 'complete staging validates a different manifest source than it uploads' >&2
  exit 1
}

printf 'stage-input fixtures passed\n'
