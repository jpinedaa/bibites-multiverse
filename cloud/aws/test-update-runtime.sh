#!/usr/bin/env bash
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
test_root="$(mktemp -d)"
trap 'find "$test_root" -depth -delete' EXIT

fixture_repo="$test_root/repo"
fixture_cloud="$fixture_repo/cloud/aws"
fixture_dist="$fixture_cloud/dist"
fixture_bin="$test_root/bin"
install -d "$fixture_cloud/lib" "$fixture_dist" "$fixture_bin"
cp "$repo/cloud/aws/update-runtime.sh" "$fixture_cloud/update-runtime.sh"
cp "$repo/cloud/aws/lib/validation.sh" "$fixture_cloud/lib/validation.sh"
chmod 0755 "$fixture_cloud/update-runtime.sh"

runtime_sha=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
game_sha=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
bepinex_sha=cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc
manifest_sha=dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd
pointer_prefix=/bibites/cloud/runtime-pointer/fixture
raw_secret_canary='RAW-POINTER-SECRET-MUST-NOT-ENTER-SSM'

cat >"$fixture_dist/artifacts.env" <<EOF
RUNTIME_FILE=bibites-cloud-runtime.tar.gz
RUNTIME_SHA256=$runtime_sha
GAME_FILE=game.zip
GAME_SHA256=$game_sha
BEPINEX_FILE=bepinex.zip
BEPINEX_SHA256=$bepinex_sha
EOF

write_staged_receipt() {
  local scope="$1"
  cat >"$fixture_dist/staged.env" <<EOF
AWS_PROFILE=fixture-profile
AWS_REGION=us-east-1
ARTIFACT_BUCKET=fixture-artifacts
ARTIFACT_PREFIX=cloud/v1
RUNTIME_OBJECT=runtime/$runtime_sha.tar.gz
RUNTIME_SHA256=$runtime_sha
GAME_OBJECT=runtime-inputs/game/$game_sha.zip
GAME_SHA256=$game_sha
BEPINEX_OBJECT=runtime-inputs/bepinex/$bepinex_sha.zip
BEPINEX_SHA256=$bepinex_sha
MANIFEST_OBJECT=worlds.$manifest_sha.json
MANIFEST_SHA256=$manifest_sha
STAGING_SCOPE=$scope
EOF
}
write_staged_receipt runtime-only

aws_log="$test_root/aws.log"
command_log="$test_root/command.log"
cat >"$fixture_bin/aws" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
args=("$@")
joined=" $* "
printf '%s\n' "$*" >>"$MOCK_AWS_LOG"
case "$joined" in
  *' sts get-caller-identity '*)
    printf '123456789012\n'
    ;;
  *' cloudformation describe-stacks '*)
    if [ "$MOCK_SCENARIO" = legacy_domain ]; then
      relay_parameters='[
        {"ParameterKey":"ArtifactBucket","ParameterValue":"fixture-artifacts"},
        {"ParameterKey":"ArtifactPrefix","ParameterValue":"cloud/v1"},
        {"ParameterKey":"RelayPrivateIp","ParameterValue":"10.1.2.3"},
        {"ParameterKey":"CredentialParameterPrefix","ParameterValue":"/bibites/cloud"}]'
    elif [ "$MOCK_SCENARIO" = legacy_prefix ]; then
      relay_parameters='[
        {"ParameterKey":"ArtifactBucket","ParameterValue":"fixture-artifacts"},
        {"ParameterKey":"ArtifactPrefix","ParameterValue":"cloud/v1"},
        {"ParameterKey":"RelayPrivateIp","ParameterValue":"10.1.2.3"},
        {"ParameterKey":"RelayDomain","ParameterValue":"relay.example.test"}]'
    elif [ "$MOCK_SCENARIO" = artifact_target_mismatch ]; then
      relay_parameters='[
        {"ParameterKey":"ArtifactBucket","ParameterValue":"other-artifacts"},
        {"ParameterKey":"ArtifactPrefix","ParameterValue":"other/v1"},
        {"ParameterKey":"RelayPrivateIp","ParameterValue":"10.1.2.3"},
        {"ParameterKey":"RelayDomain","ParameterValue":"relay.example.test"},
        {"ParameterKey":"CredentialParameterPrefix","ParameterValue":"/bibites/cloud"}]'
    else
      relay_parameters='[
        {"ParameterKey":"ArtifactBucket","ParameterValue":"fixture-artifacts"},
        {"ParameterKey":"ArtifactPrefix","ParameterValue":"cloud/v1"},
        {"ParameterKey":"RelayPrivateIp","ParameterValue":"10.1.2.3"},
        {"ParameterKey":"RelayDomain","ParameterValue":"relay.example.test"},
        {"ParameterKey":"CredentialParameterPrefix","ParameterValue":"/bibites/cloud"}]'
    fi
    jq -nc --argjson parameters "$relay_parameters" '{Stacks:[{
      StackStatus:"UPDATE_COMPLETE",
      Outputs:[
        {OutputKey:"InstanceId",OutputValue:"i-0123456789abcdef0"},
        {OutputKey:"DataVolumeId",OutputValue:"vol-0123456789abcdef0"}],
      Parameters:$parameters}]}'
    ;;
  *' ec2 describe-volumes '*)
    printf '%s\n' '{"Volumes":[{
      "VolumeId":"vol-0123456789abcdef0",
      "Attachments":[{"InstanceId":"i-0123456789abcdef0","State":"attached"}]}]}'
    ;;
  *' ssm describe-instance-information '*)
    printf '%s\n' '{"InstanceInformationList":[{
      "InstanceId":"i-0123456789abcdef0","PingStatus":"Online",
      "PlatformType":"Linux"}]}'
    ;;
  *' ssm send-command '*)
    if [ "$MOCK_SCENARIO" = send_failure ]; then
      echo 'transport response lost' >&2
      exit 254
    fi
    parameters=
    for ((index=0; index < ${#args[@]}; index++)); do
      if [ "${args[$index]}" = --parameters ]; then
        parameters="${args[$((index + 1))]}"
        break
      fi
    done
    [ -n "$parameters" ] || exit 64
    jq -er '.commands | if length == 1 then .[0] else error("commands") end' \
      <<<"$parameters" \
      >"$MOCK_COMMAND_LOG"
    if grep -Fq "$MOCK_RAW_SECRET_CANARY" "$MOCK_COMMAND_LOG"; then
      echo 'raw pointer credential entered command metadata' >&2
      exit 64
    fi
    printf '11111111-1111-1111-1111-111111111111\n'
    ;;
  *' ssm get-command-invocation '*)
    case "$MOCK_SCENARIO" in
      success|legacy_domain|legacy_prefix)
        printf '%s\n' '{"Status":"Success","ResponseCode":0,
          "StandardOutputContent":"transaction complete","StandardErrorContent":""}'
        ;;
      success_bad_response)
        printf '%s\n' '{"Status":"Success","ResponseCode":7,
          "StandardOutputContent":"unexpected","StandardErrorContent":""}'
        ;;
      rollback_complete)
        printf '%s\n' '{"Status":"Failed","ResponseCode":22,
          "StandardOutputContent":"","StandardErrorContent":"prior runtime restored"}'
        ;;
      preflight_rejected)
        printf '%s\n' '{"Status":"Failed","ResponseCode":2,
          "StandardOutputContent":"","StandardErrorContent":"transaction preflight rejected"}'
        ;;
      failed_zero)
        printf '%s\n' '{"Status":"Failed","ResponseCode":0,
          "StandardOutputContent":"","StandardErrorContent":"invalid fixture"}'
        ;;
      failed_signal)
        printf '%s\n' '{"Status":"Failed","ResponseCode":143,
          "StandardOutputContent":"","StandardErrorContent":"terminated"}'
        ;;
      failed_aws)
        printf '%s\n' '{"Status":"Failed","ResponseCode":254,
          "StandardOutputContent":"","StandardErrorContent":"AWS CLI failure"}'
        ;;
      unknown_status)
        echo 'An error occurred (AccessDeniedException)' >&2
        exit 254
        ;;
      timed_out)
        printf '%s\n' '{"Status":"TimedOut","ResponseCode":-1,
          "StandardOutputContent":"","StandardErrorContent":""}'
        ;;
      *) exit 65 ;;
    esac
    ;;
  *)
    echo "unexpected AWS fixture call: $*" >&2
    exit 64
    ;;
esac
MOCK
chmod 0755 "$fixture_bin/aws"

run_case() {
  local scenario="$1"
  local selected_pointer_prefix="${BIBITES_TEST_POINTER_PREFIX:-$pointer_prefix}"
  PATH="$fixture_bin:$PATH" \
    BIBITES_AWS_ACCOUNT_ID=123456789012 \
    BIBITES_POINTER_CREDENTIAL_PARAMETER_PREFIX="$selected_pointer_prefix" \
    MOCK_SCENARIO="$scenario" MOCK_AWS_LOG="$aws_log" \
    MOCK_COMMAND_LOG="$command_log" \
    MOCK_RAW_SECRET_CANARY="$raw_secret_canary" \
    BIBITES_RELAY_DOMAIN="${BIBITES_TEST_RELAY_DOMAIN:-}" \
    BIBITES_CREDENTIAL_PARAMETER_PREFIX="${BIBITES_TEST_CREDENTIAL_PREFIX:-}" \
    RUNTIME_SHA256="${BIBITES_TEST_AMBIENT_RUNTIME_SHA256:-}" \
    "$fixture_cloud/update-runtime.sh"
}

: >"$aws_log"
: >"$command_log"
success_output="$(run_case success 2>&1)"
grep -Fq 'Systems Manager command ID: 11111111-1111-1111-1111-111111111111' \
  <<<"$success_output"
grep -Fq 'transaction complete' <<<"$success_output"
for expected in \
  "runtime/$runtime_sha.tar.gz" \
  "runtime-inputs/game/$game_sha.zip" \
  "runtime-inputs/bepinex/$bepinex_sha.zip" \
  "worlds.$manifest_sha.json" \
  "$pointer_prefix"; do
  grep -Fq "$expected" "$command_log" || {
    echo "SSM command omitted pinned transaction argument $expected" >&2
    exit 1
  }
done
if grep -Fq "$raw_secret_canary" "$command_log" ||
   grep -Fq "$raw_secret_canary" <<<"$success_output"; then
  echo 'raw pointer credential escaped into command metadata or output' >&2
  exit 1
fi

: >"$aws_log"
set +e
rollback_output="$(run_case rollback_complete 2>&1)"
rollback_status=$?
set -e
[ "$rollback_status" -eq 22 ] || {
  echo "terminal transaction status 22 returned $rollback_status" >&2
  exit 1
}
grep -Fq 'prior runtime restored' <<<"$rollback_output"

: >"$aws_log"
set +e
preflight_output="$(run_case preflight_rejected 2>&1)"
preflight_status=$?
set -e
[ "$preflight_status" -eq 2 ] || {
  echo "terminal transaction status 2 returned $preflight_status" >&2
  exit 1
}
grep -Fq 'transaction preflight rejected' <<<"$preflight_output"

for unknown_scenario in unknown_status timed_out send_failure success_bad_response \
  failed_zero failed_signal failed_aws; do
  : >"$aws_log"
  set +e
  unknown_output="$(run_case "$unknown_scenario" 2>&1)"
  unknown_status=$?
  set -e
  [ "$unknown_status" -eq 25 ] || {
    echo "$unknown_scenario returned $unknown_status instead of 25" >&2
    exit 1
  }
  if [ "$unknown_scenario" = send_failure ]; then
    grep -Fq 'command ID is unavailable' <<<"$unknown_output"
  else
    grep -Fq '11111111-1111-1111-1111-111111111111' <<<"$unknown_output"
  fi
  grep -Fq 'Do not retry it blindly' <<<"$unknown_output"
done

for invalid_scope in complete missing; do
  if [ "$invalid_scope" = missing ]; then
    write_staged_receipt runtime-only
    sed -i '/^STAGING_SCOPE=/d' "$fixture_dist/staged.env"
  else
    write_staged_receipt complete
  fi
  : >"$aws_log"
  set +e
  scope_output="$(run_case success 2>&1)"
  scope_status=$?
  set -e
  [ "$scope_status" -ne 0 ]
  [ ! -s "$aws_log" ] || {
    echo "$invalid_scope staging scope reached AWS" >&2
    exit 1
  }
  grep -Fq 'requires STAGING_SCOPE=runtime-only' <<<"$scope_output"
done

write_staged_receipt runtime-only

# A legacy stack has no RelayDomain parameter. It requires the explicit,
# validated operator input, while a stack-owned domain rejects disagreement.
: >"$aws_log"
legacy_output="$(BIBITES_TEST_RELAY_DOMAIN=relay.example.test \
  run_case legacy_domain 2>&1)"
grep -Fq 'transaction complete' <<<"$legacy_output"

: >"$aws_log"
set +e
legacy_missing_output="$(run_case legacy_domain 2>&1)"
legacy_missing_status=$?
set -e
[ "$legacy_missing_status" -ne 0 ]
grep -Fq 'stack has no RelayDomain parameter; set BIBITES_RELAY_DOMAIN' \
  <<<"$legacy_missing_output"
if grep -Fq 'ssm send-command' "$aws_log"; then
  echo 'legacy stack without an explicit relay domain reached host mutation' >&2
  exit 1
fi

: >"$aws_log"
set +e
domain_mismatch_output="$(BIBITES_TEST_RELAY_DOMAIN=other.example.test \
  run_case success 2>&1)"
domain_mismatch_status=$?
set -e
[ "$domain_mismatch_status" -ne 0 ]
grep -Fq 'differs from the stack RelayDomain parameter' \
  <<<"$domain_mismatch_output"
if grep -Fq 'ssm send-command' "$aws_log"; then
  echo 'stack relay-domain disagreement reached host mutation' >&2
  exit 1
fi

: >"$aws_log"
set +e
artifact_mismatch_output="$(run_case artifact_target_mismatch 2>&1)"
artifact_mismatch_status=$?
set -e
[ "$artifact_mismatch_status" -ne 0 ]
grep -Fq 'staging target differs from the selected stack artifacts' \
  <<<"$artifact_mismatch_output"
if grep -Fq 'ssm send-command' "$aws_log"; then
  echo 'stack artifact-target disagreement reached host mutation' >&2
  exit 1
fi

# Older stacks can also lack CredentialParameterPrefix. Require the explicit
# host-readable scope, and reject disagreement when a newer stack owns it.
: >"$aws_log"
legacy_prefix_output="$(BIBITES_TEST_CREDENTIAL_PREFIX=/bibites/cloud \
  run_case legacy_prefix 2>&1)"
grep -Fq 'transaction complete' <<<"$legacy_prefix_output"

: >"$aws_log"
set +e
legacy_prefix_missing_output="$(run_case legacy_prefix 2>&1)"
legacy_prefix_missing_status=$?
set -e
[ "$legacy_prefix_missing_status" -ne 0 ]
grep -Fq 'stack has no CredentialParameterPrefix parameter; set BIBITES_CREDENTIAL_PARAMETER_PREFIX' \
  <<<"$legacy_prefix_missing_output"
if grep -Fq 'ssm send-command' "$aws_log"; then
  echo 'legacy stack without an explicit credential prefix reached host mutation' >&2
  exit 1
fi

: >"$aws_log"
set +e
credential_mismatch_output="$(BIBITES_TEST_CREDENTIAL_PREFIX=/other/prefix \
  run_case success 2>&1)"
credential_mismatch_status=$?
set -e
[ "$credential_mismatch_status" -ne 0 ]
grep -Fq 'differs from the stack CredentialParameterPrefix parameter' \
  <<<"$credential_mismatch_output"
if grep -Fq 'ssm send-command' "$aws_log"; then
  echo 'stack credential-prefix disagreement reached host mutation' >&2
  exit 1
fi

# Missing receipt fields cannot be supplied by artifacts.env or the ambient
# environment. Receipt validation must fail before the first AWS call.
write_staged_receipt runtime-only
sed -i '/^RUNTIME_SHA256=/d' "$fixture_dist/staged.env"
: >"$aws_log"
set +e
ambient_output="$(BIBITES_TEST_AMBIENT_RUNTIME_SHA256="$runtime_sha" \
  run_case success 2>&1)"
ambient_status=$?
set -e
[ "$ambient_status" -ne 0 ]
[ ! -s "$aws_log" ] || {
  echo 'ambient value supplied a missing staged receipt field' >&2
  exit 1
}
grep -Fq 'Set RUNTIME_SHA256 before the runtime update' <<<"$ambient_output"

write_staged_receipt runtime-only
: >"$aws_log"
set +e
prefix_output="$(BIBITES_TEST_POINTER_PREFIX=/different/prefix \
  run_case success 2>&1)"
prefix_status=$?
set -e
[ "$prefix_status" -ne 0 ]
if grep -Fq 'ssm send-command' "$aws_log"; then
  echo 'out-of-scope pointer credentials reached host mutation' >&2
  exit 1
fi

printf 'runtime update wrapper fixtures passed\n'
