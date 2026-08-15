#!/usr/bin/env bash
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
validator="$repo/cloud/aws/runtime/validate-world-manifest.jq"

valid='{
  "schema": 1,
  "worlds": [{
    "id": "world-a",
    "peerId": "peer-a",
    "worldName": "Example World",
    "sidecarPort": 8787,
    "saveKey": "imports/Example-World.zip",
    "credentialParameter": "/bibites-multiverse/cloud/world-a/peer-secret",
    "position": "0,0",
    "preferredSlot": 1,
    "targetTimeScale": 100,
    "saveMinutes": 10,
    "saveKeep": 6,
    "enabled": true
  }]
}'

accept() {
  local label="$1" document="$2"
  printf '%s\n' "$document" | jq -e -f "$validator" >/dev/null || {
    printf 'valid fixture failed: %s\n' "$label" >&2
    exit 1
  }
}

reject() {
  local label="$1" document="$2"
  if printf '%s\n' "$document" | jq -e -f "$validator" >/dev/null 2>&1; then
    printf 'invalid fixture passed: %s\n' "$label" >&2
    exit 1
  fi
}

changed() {
  jq -c "$1" <<<"$valid"
}

accept 'complete manifest' "$valid"
accept 'optional fields omitted' "$(changed 'del(.worlds[0].position, .worlds[0].preferredSlot, .worlds[0].targetTimeScale, .worlds[0].saveMinutes, .worlds[0].saveKeep, .worlds[0].enabled)')"

reject 'unsafe world id' "$(changed '.worlds[0].id = "../world"')"
reject 'peer id type' "$(changed '.worlds[0].peerId = 7')"
reject 'world name traversal' "$(changed '.worlds[0].worldName = "../World"')"
reject 'world name separator' "$(changed '.worlds[0].worldName = "World/One"')"
reject 'port type' "$(changed '.worlds[0].sidecarPort = "8787"')"
reject 'port range' "$(changed '.worlds[0].sidecarPort = 70000')"
reject 'save key traversal' "$(changed '.worlds[0].saveKey = "imports/../World.zip"')"
reject 'save extension' "$(changed '.worlds[0].saveKey = "imports/World.bb8"')"
reject 'parameter traversal' "$(changed '.worlds[0].credentialParameter = "/cloud/../secret"')"
reject 'position type' "$(changed '.worlds[0].position = [0, 0]')"
reject 'position range' "$(changed '.worlds[0].position = "1000001,0"')"
reject 'slot range' "$(changed '.worlds[0].preferredSlot = 0')"
reject 'time-scale range' "$(changed '.worlds[0].targetTimeScale = 1001')"
reject 'save interval range' "$(changed '.worlds[0].saveMinutes = -1')"
reject 'save count type' "$(changed '.worlds[0].saveKeep = 1.5')"
reject 'enabled type' "$(changed '.worlds[0].enabled = "false"')"
reject 'unknown field' "$(changed '.worlds[0].enable = true')"
reject 'duplicate identity' "$(changed '.worlds += [.worlds[0]]')"

printf 'manifest schema fixtures passed\n'
