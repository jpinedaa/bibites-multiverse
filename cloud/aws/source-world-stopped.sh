#!/usr/bin/env bash
# Prove that one source world is stopped and disabled before a snapshot starts.
set -euo pipefail

[ "$#" -eq 1 ] || { echo 'source check requires one world identifier' >&2; exit 1; }
world="$1"
[[ "$world" =~ ^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$ ]] || {
  echo 'source check received an invalid world identifier' >&2
  exit 1
}

require_inactive() {
  local unit="$1" state
  state="$(systemctl is-active "$unit" 2>/dev/null || true)"
  if [ "$state" != inactive ]; then
    echo "$unit is not inactive" >&2
    return 1
  fi
}

require_disabled() {
  local unit="$1" state
  state="$(systemctl is-enabled "$unit" 2>/dev/null || true)"
  [ "$state" = disabled ] || {
    echo "$unit is not disabled" >&2
    return 1
  }
}

game_unit="bibites-game@$world.service"
sidecar_unit="bibites-sidecar@$world.service"
require_inactive "$game_unit"
require_inactive "$sidecar_unit"
require_disabled "$game_unit"
require_disabled "$sidecar_unit"

printf '%s is inactive and disabled on the source host\n' "$world"
