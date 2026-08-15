#!/usr/bin/env bash
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
check="$repo/cloud/aws/source-world-stopped.sh"
mock_dir="$(mktemp -d)"
trap 'rm -f "$mock_dir/systemctl"; rmdir "$mock_dir"' EXIT

printf '%s\n' '#!/usr/bin/env bash' >"$mock_dir/systemctl"
printf '%s\n' '
set -euo pipefail
action="$1"
unit="${2:-}"
case "$action:$unit" in
  is-active:bibites-game@slot-1.service) state="${GAME_ACTIVE:-inactive}" ;;
  is-active:bibites-sidecar@slot-1.service) state="${SIDECAR_ACTIVE:-inactive}" ;;
  is-enabled:bibites-game@slot-1.service) state="${GAME_ENABLED:-disabled}" ;;
  is-enabled:bibites-sidecar@slot-1.service) state="${SIDECAR_ENABLED:-disabled}" ;;
  *) exit 3 ;;
esac
printf "%s\n" "$state"
case "$action:$state" in
  is-active:active|is-enabled:enabled) exit 0 ;;
  *) exit 1 ;;
esac' >>"$mock_dir/systemctl"
chmod 0755 "$mock_dir/systemctl"

run_check() {
  PATH="$mock_dir:$PATH" \
    GAME_ACTIVE="${GAME_ACTIVE:-inactive}" \
    SIDECAR_ACTIVE="${SIDECAR_ACTIVE:-inactive}" \
    GAME_ENABLED="${GAME_ENABLED:-disabled}" \
    SIDECAR_ENABLED="${SIDECAR_ENABLED:-disabled}" \
    "$check" slot-1
}

run_check >/dev/null
for failure in GAME_ACTIVE SIDECAR_ACTIVE GAME_ENABLED SIDECAR_ENABLED; do
  unset GAME_ACTIVE SIDECAR_ACTIVE GAME_ENABLED SIDECAR_ENABLED
  case "$failure" in
    GAME_ACTIVE) GAME_ACTIVE=active ;;
    SIDECAR_ACTIVE) SIDECAR_ACTIVE=active ;;
    GAME_ENABLED) GAME_ENABLED=enabled ;;
    SIDECAR_ENABLED) SIDECAR_ENABLED=enabled ;;
  esac
  export GAME_ACTIVE SIDECAR_ACTIVE GAME_ENABLED SIDECAR_ENABLED 2>/dev/null || true
  if run_check >/dev/null 2>&1; then
    echo "$failure fixture did not fail" >&2
    exit 1
  fi
done

printf 'source-world state fixtures passed\n'
