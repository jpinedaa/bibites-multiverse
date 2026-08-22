#!/usr/bin/env bash
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=lib/validation.sh
. "$repo/cloud/aws/lib/validation.sh"

state="$(mktemp)"
next_state="$(mktemp)"
trap 'rm -f "$state" "$next_state"' EXIT
mock_epoch=100

date() {
  [ "$#" -eq 1 ] && [ "$1" = +%s ] || return 64
  printf '%s\n' "$mock_epoch"
}

sleep() {
  [ "$#" -eq 1 ] && [[ "$1" =~ ^[0-9]+$ ]] || return 64
  mock_epoch=$((mock_epoch + 10#$1))
}

set_responses() {
  printf '0\n' >"$state"
  printf '%s\n' "$@" >>"$state"
}

aws() {
  local index line_number response
  index="$(sed -n '1p' "$state")"
  line_number=$((index + 2))
  response="$(sed -n "${line_number}p" "$state")"
  [ -n "$response" ] || response="$(tail -n 1 "$state")"
  {
    printf '%s\n' "$((index + 1))"
    tail -n +2 "$state"
  } >"$next_state"
  mv "$next_state" "$state"

  case "$response" in
    InvocationDoesNotExist)
      echo 'An error occurred (InvocationDoesNotExist)' >&2
      return 254
      ;;
    AWSFailure)
      echo 'An error occurred (AccessDeniedException)' >&2
      return 254
      ;;
    *) jq -cn --arg status "$response" '{Status:$status}' ;;
  esac
}

command_id=12345678-1234-1234-1234-123456789abc
instance_id=i-0123456789abcdef0

set_responses InvocationDoesNotExist Pending Delayed InProgress Success
result="$(bibites_wait_ssm_invocation test-profile us-east-1 \
  "$command_id" "$instance_id" 2 0)"
[ "$(jq -r .Status <<<"$result")" = Success ] || {
  echo 'eventual-consistency fixture did not reach Success' >&2
  exit 1
}
[ "$(sed -n '1p' "$state")" = 5 ] || {
  echo 'eventual-consistency fixture used an unexpected poll count' >&2
  exit 1
}

set_responses Failed
set +e
result="$(bibites_wait_ssm_invocation test-profile us-east-1 \
  "$command_id" "$instance_id" 2 0)"
status=$?
set -e
[ "$status" -eq 1 ] && [ "$(jq -r .Status <<<"$result")" = Failed ] || {
  echo 'terminal-failure fixture returned the wrong result' >&2
  exit 1
}

set_responses AWSFailure
set +e
bibites_wait_ssm_invocation test-profile us-east-1 \
  "$command_id" "$instance_id" 2 0 >/dev/null 2>&1
status=$?
set -e
[ "$status" -eq 2 ] || {
  echo 'AWS status-error fixture returned the wrong result' >&2
  exit 1
}

set_responses Pending
mock_epoch=100
set +e
bibites_wait_ssm_invocation test-profile us-east-1 \
  "$command_id" "$instance_id" 1 1 >/dev/null 2>&1
status=$?
set -e
[ "$status" -eq 124 ] && [ "$(sed -n '1p' "$state")" = 1 ] || {
  echo 'bounded-timeout fixture returned the wrong result' >&2
  exit 1
}

printf 'Systems Manager status fixtures passed\n'
