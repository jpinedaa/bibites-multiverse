#!/usr/bin/env bash
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
imports="$repo/cloud/aws/imports"
dist="$repo/cloud/aws/dist"
install -d "$dist"

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
printf '{"schema":1,"worlds":[' > "$tmp"
first=1
disabled=" ${BIBITES_DISABLED_WORLD_IDS:-} "
for n in 1 2 3 4 5 6; do
  save="$imports/M4-Slot$n.zip"
  [ -f "$save" ] || continue
  unzip -tqq "$save"
  case "$n" in
    1) position='0,0' ;; 2) position='1,0' ;; 3) position='2,0' ;;
    4) position='0,1' ;; 5) position='1,1' ;; 6) position='2,1' ;;
  esac
  [ "$first" -eq 1 ] || printf ',' >> "$tmp"
  first=0
  enabled=true
  case "$disabled" in
    *" slot-$n "*) enabled=false ;;
  esac
  jq -cn \
    --arg id "slot-$n" --arg peer "slot-$n" --arg world "M4-Slot$n" \
    --argjson port "$((8786 + n))" --arg save "imports/M4-Slot$n.zip" \
    --arg parameter "/bibites-multiverse/cloud/slot-$n/peer-secret" \
    --arg position "$position" --argjson slot "$n" --argjson enabled "$enabled" \
    '{id:$id,peerId:$peer,worldName:$world,sidecarPort:$port,saveKey:$save,
      credentialParameter:$parameter,position:$position,preferredSlot:$slot,
      targetTimeScale:100,saveMinutes:10,saveKeep:6,enabled:$enabled}' >> "$tmp"
done
printf ']}\n' >> "$tmp"
jq -e '.worlds | length > 0' "$tmp" >/dev/null
jq . "$tmp" > "$dist/worlds.json"
printf 'manifest has %s world(s): %s\n' \
  "$(jq '.worlds | length' "$dist/worlds.json")" \
  "$(jq -r '[.worlds[].id] | join(", ")' "$dist/worlds.json")"
