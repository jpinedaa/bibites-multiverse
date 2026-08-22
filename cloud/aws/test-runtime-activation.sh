#!/usr/bin/env bash
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
activate="$repo/cloud/aws/runtime/bibites-activate-runtime"
test_root="$(mktemp -d)"
trap 'find "$test_root" -depth -delete' EXIT
mock_bin="$test_root/bin"
install -d "$mock_bin"
printf '%s\n' \
  '#!/usr/bin/env bash' \
  'set -euo pipefail' \
  'if [ "${1:-}" = list-units ]; then' \
  '  if [[ " $* " == *" --plain "* ]]; then' \
  '    printf "%s\n" "bibites-timescale@slot-1.service loaded failed failed"' \
  '  else' \
  '    printf "%s\n" "● bibites-timescale@slot-1.service loaded failed failed"' \
  '  fi' \
  '  exit 0' \
  'fi' \
  'if [ "${1:-}" = stop ] && [ "${2:-}" = "●" ]; then exit 1; fi' \
  'exit 0' >"$mock_bin/systemctl"
chmod 0755 "$mock_bin/systemctl"

make_runtime() {
  local tree="$1" label="$2"
  install -d "$tree"
  printf '%s\n' "$label" >"$tree/label"
  printf '%s\n' '#!/usr/bin/env bash' \
    'set -euo pipefail' \
    'here="$(cd "$(dirname "$0")" && pwd)"' \
    'label="$(cat "$here/label")"' \
    '[ "$MANIFEST_SHA256" = 3333333333333333333333333333333333333333333333333333333333333333 ]' \
    'printf "%s-install\n" "$label" >>"$MOCK_LOG"' \
    'if [ "$label" = new ]; then exit "${NEW_INSTALL_STATUS:-0}"; fi' \
    'status="${OLD_INSTALL_STATUS:-0}"' \
    'if [ "$status" -eq 0 ]; then printf "old-worlds-restarted\n" >>"$MOCK_LOG"; fi' \
    'exit "$status"' >"$tree/install-host"
  printf '%s\n' '#!/usr/bin/env bash' \
    'set -euo pipefail' \
    'here="$(cd "$(dirname "$0")" && pwd)"' \
    'printf "%s-stop\n" "$(cat "$here/label")" >>"$MOCK_LOG"' \
    'exit "${STOP_STATUS:-0}"' >"$tree/bibites-stop-worlds"
  chmod 0755 "$tree/install-host" "$tree/bibites-stop-worlds"
}

setup_case() {
  case_root="$(mktemp -d "$test_root/case.XXXXXX")"
  runtime_root="$case_root/bibites-runtime"
  staged_runtime="$case_root/staged-runtime"
  archive="$case_root/runtime.tar.gz"
  log="$case_root/actions.log"
  lock="$case_root/update.lock"
  make_runtime "$runtime_root" old
  make_runtime "$staged_runtime" new
  printf 'archive\n' >"$archive"
  : >"$log"
}

run_activation() {
  PATH="$mock_bin:$PATH" \
    MOCK_LOG="$log" \
    BIBITES_RUNTIME_TEST_ROOT="$runtime_root" \
    BIBITES_RUNTIME_LOCK_FILE="$lock" \
    BIBITES_RUNTIME_LOCK_FD="${INHERITED_LOCK_FD:-}" \
    NEW_INSTALL_STATUS="${NEW_INSTALL_STATUS:-0}" \
    OLD_INSTALL_STATUS="${OLD_INSTALL_STATUS:-0}" \
    STOP_STATUS="${STOP_STATUS:-0}" \
    "$activate" \
      "$staged_runtime" "$archive" us-east-1 vol-0123456789abcdef0 \
      example-artifacts cloud/v1/runtime/runtime.tar.gz \
      0000000000000000000000000000000000000000000000000000000000000000 \
      cloud/v1/game.zip \
      1111111111111111111111111111111111111111111111111111111111111111 \
      cloud/v1/bepinex.zip \
      2222222222222222222222222222222222222222222222222222222222222222 \
      cloud/v1/worlds.json \
      3333333333333333333333333333333333333333333333333333333333333333 \
      10.0.0.5 relay.example.net "$runtime_root"
}

setup_case
run_activation >/dev/null
[ "$(cat "$runtime_root/label")" = new ] || {
  echo 'successful activation did not keep the new runtime' >&2
  exit 1
}
[ ! -e "$runtime_root.previous" ] || {
  echo 'successful activation retained the previous runtime' >&2
  exit 1
}
grep -Fxq new-install "$log" || {
  echo 'successful activation did not run the new installer' >&2
  exit 1
}
[ "$(sed -n '1p' "$log")" = new-stop ] || {
  echo 'successful activation did not stop worlds before installation' >&2
  exit 1
}

setup_case
exec 8>"$lock"
flock -n 8
INHERITED_LOCK_FD=8 run_activation >/dev/null
flock -u 8
exec 8>&-
[ "$(cat "$runtime_root/label")" = new ] || {
  echo 'activation did not reuse the transaction lock descriptor' >&2
  exit 1
}

setup_case
NEW_INSTALL_STATUS=7
set +e
failure_output="$(run_activation 2>&1)"
status=$?
set -e
unset NEW_INSTALL_STATUS
[ "$status" -eq 20 ] || {
  echo "successful rollback returned $status instead of 20" >&2
  exit 1
}
grep -Fq 'rollback completed' <<<"$failure_output" || {
  echo 'successful rollback did not report completion' >&2
  exit 1
}
[ "$(cat "$runtime_root/label")" = old ] || {
  echo 'rollback did not restore the old runtime' >&2
  exit 1
}
grep -Fxq old-install "$log" || {
  echo 'rollback did not reinstall the old host files and services' >&2
  exit 1
}
grep -Fxq old-worlds-restarted "$log" || {
  echo 'rollback did not resynchronize and restart the old worlds' >&2
  exit 1
}
[ "$(grep -Fxc new-stop "$log")" -ge 2 ] || {
  echo 'rollback did not stop the partial new services' >&2
  exit 1
}
mapfile -t failed_paths < <(find "$case_root" -maxdepth 1 -type d \
  -name 'bibites-runtime.failed-*' -print)
[ "${#failed_paths[@]}" -eq 1 ] &&
  [ "$(cat "${failed_paths[0]}/runtime/label")" = new ] &&
  [ -f "${failed_paths[0]}/runtime.tar.gz" ] || {
    echo 'rollback did not retain the failed runtime and archive' >&2
    exit 1
  }

setup_case
NEW_INSTALL_STATUS=7
OLD_INSTALL_STATUS=9
set +e
failure_output="$(run_activation 2>&1)"
status=$?
set -e
unset NEW_INSTALL_STATUS OLD_INSTALL_STATUS
[ "$status" -eq 21 ] || {
  echo "failed rollback returned $status instead of 21" >&2
  exit 1
}
grep -Fq 'rollback also failed' <<<"$failure_output" || {
  echo 'failed rollback did not report a distinct failure' >&2
  exit 1
}
[ "$(cat "$runtime_root/label")" = old ] || {
  echo 'failed rollback did not leave the old runtime tree in place' >&2
  exit 1
}

setup_case
exec 8>"$lock"
flock -n 8
set +e
run_activation >/dev/null 2>&1
status=$?
set -e
flock -u 8
exec 8>&-
[ "$status" -eq 73 ] || {
  echo "lock contention returned $status instead of 73" >&2
  exit 1
}
[ ! -s "$log" ] || {
  echo 'lock contention changed runtime services' >&2
  exit 1
}

update_wrapper="$repo/cloud/aws/update-runtime.sh"
grep -Fq 'executionTimeout:["3600"]' "$update_wrapper" || {
  echo 'runtime update has no explicit Systems Manager execution timeout' >&2
  exit 1
}
if grep -Fq 'ssm wait command-executed' "$update_wrapper"; then
  echo 'runtime update still uses the fixed Systems Manager waiter' >&2
  exit 1
fi
extract_line="$(grep -n 'tar -xzf "$archive"' "$update_wrapper" | cut -d: -f1)"
transaction_line="$(grep -n '"$new_runtime/bibites-update-runtime-transaction"' \
  "$update_wrapper" | tail -n 1 | cut -d: -f1)"
[ "$extract_line" -lt "$transaction_line" ] || {
  echo 'runtime update does not pre-stage before activation' >&2
  exit 1
}

printf 'runtime activation fixtures passed\n'
