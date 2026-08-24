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

remote_runtime_tree="$test_root/remote-runtime"
remote_runtime_archive="$test_root/remote-runtime.tar.gz"
remote_runtime_root="$test_root/remote-root"
transaction_log="$test_root/transaction.log"
remote_output="$test_root/remote-command.out"
install -d "$remote_runtime_tree" "$remote_runtime_root"
cat >"$remote_runtime_tree/bibites-update-runtime-transaction" <<'REMOTE_TRANSACTION'
#!/usr/bin/env bash
set -euo pipefail
{
  printf '%s\n' "$#"
  printf '%s\n' "$@"
} >"$MOCK_TRANSACTION_LOG"
REMOTE_TRANSACTION
for remote_executable in bibites-activate-runtime install-host bibites-stop-worlds; do
  cat >"$remote_runtime_tree/$remote_executable" <<'REMOTE_EXECUTABLE'
#!/usr/bin/env bash
set -euo pipefail
exit 0
REMOTE_EXECUTABLE
done
printf 'true\n' >"$remote_runtime_tree/validate-world-manifest.jq"
chmod 0755 "$remote_runtime_tree/bibites-update-runtime-transaction" \
  "$remote_runtime_tree/bibites-activate-runtime" \
  "$remote_runtime_tree/install-host" \
  "$remote_runtime_tree/bibites-stop-worlds"
tar -czf "$remote_runtime_archive" -C "$remote_runtime_tree" .
runtime_sha="$(sha256sum "$remote_runtime_archive" | awk '{print $1}')"
game_sha=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
bepinex_sha=cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc
manifest_sha=dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd
manifest_preimage_etag='"22222222222222222222222222222222"'
prior_sha=eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee
prior_etag='"11111111111111111111111111111111"'
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
MANIFEST_PREIMAGE_ETAG=$(printf '%q' "$manifest_preimage_etag")
STAGING_SCOPE=$scope
EOF
}
write_staged_receipt runtime-only

aws_log="$test_root/aws.log"
command_log="$test_root/command.log"
cat >"$fixture_bin/mktemp" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
if [ "${1:-}" = -d ] && [ "${2:-}" = /opt/bibites-runtime.new.XXXXXX ]; then
  exec /usr/bin/mktemp -d "$MOCK_REMOTE_ROOT/bibites-runtime.new.XXXXXX"
fi
exec /usr/bin/mktemp "$@"
MOCK
chmod 0755 "$fixture_bin/mktemp"

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
  *' s3 cp '*)
    [ "$MOCK_SCENARIO" = execute_remote ] || exit 64
    source=''
    destination=''
    for ((index=0; index < ${#args[@]}; index++)); do
      if [ "${args[$index]}" = cp ]; then
        source="${args[$((index + 1))]}"
        destination="${args[$((index + 2))]}"
        break
      fi
    done
    [ "$source" = "s3://fixture-artifacts/cloud/v1/runtime/$MOCK_RUNTIME_SHA.tar.gz" ] ||
      exit 64
    cp "$MOCK_REMOTE_RUNTIME_ARCHIVE" "$destination"
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
    parameters_command="$(jq -er \
      '.commands | if length == 1 then .[0] else error("commands") end' \
      <<<"$parameters")"
    printf '%s\n' "$parameters_command" >"$MOCK_COMMAND_LOG"
    if grep -Fq "$MOCK_RAW_SECRET_CANARY" "$MOCK_COMMAND_LOG"; then
      echo 'raw pointer credential entered command metadata' >&2
      exit 64
    fi
    if [ "$MOCK_SCENARIO" = execute_remote ]; then
      if ! bash -c "$parameters_command" >"$MOCK_REMOTE_OUTPUT" 2>&1; then
        echo 'generated remote command failed execution' >&2
        exit 64
      fi
    fi
    printf '11111111-1111-1111-1111-111111111111\n'
    ;;
  *' ssm get-command-invocation '*)
    case "$MOCK_SCENARIO" in
      success|legacy_domain|legacy_prefix|execute_remote)
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
      prior_mismatch)
        printf '%s\n' '{"Status":"Failed","ResponseCode":26,
          "StandardOutputContent":"","StandardErrorContent":"expected prior pointer mismatch"}'
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
  local selected_prior_sha="${BIBITES_TEST_EXPECTED_PRIOR_SHA256:-$prior_sha}"
  local selected_prior_object="${BIBITES_TEST_EXPECTED_PRIOR_OBJECT:-runtime/$selected_prior_sha.tar.gz}"
  local selected_prior_etag="${BIBITES_TEST_EXPECTED_PRIOR_ETAG:-$prior_etag}"
  PATH="$fixture_bin:$PATH" \
    BIBITES_AWS_ACCOUNT_ID=123456789012 \
    BIBITES_POINTER_CREDENTIAL_PARAMETER_PREFIX="$selected_pointer_prefix" \
    MOCK_SCENARIO="$scenario" MOCK_AWS_LOG="$aws_log" \
    MOCK_COMMAND_LOG="$command_log" \
    MOCK_RAW_SECRET_CANARY="$raw_secret_canary" \
    MOCK_REMOTE_RUNTIME_ARCHIVE="$remote_runtime_archive" \
    MOCK_RUNTIME_SHA="$runtime_sha" MOCK_REMOTE_ROOT="$remote_runtime_root" \
    MOCK_TRANSACTION_LOG="$transaction_log" MOCK_REMOTE_OUTPUT="$remote_output" \
    BIBITES_RELAY_DOMAIN="${BIBITES_TEST_RELAY_DOMAIN:-}" \
    BIBITES_CREDENTIAL_PARAMETER_PREFIX="${BIBITES_TEST_CREDENTIAL_PREFIX:-}" \
    RUNTIME_SHA256="${BIBITES_TEST_AMBIENT_RUNTIME_SHA256:-}" \
    MANIFEST_PREIMAGE_ETAG="${BIBITES_TEST_AMBIENT_MANIFEST_PREIMAGE_ETAG:-}" \
    "$fixture_cloud/update-runtime.sh" \
      --expected-prior-runtime-object "$selected_prior_object" \
      --expected-prior-runtime-sha256 "$selected_prior_sha" \
      --expected-prior-pointer-etag "$selected_prior_etag"
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
  "${manifest_preimage_etag//\"/}" \
  "runtime/$prior_sha.tar.gz" \
  "$prior_sha" \
  "${prior_etag//\"/}" \
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

# Decode and run the generated host command. This proves every internal
# argument position through the extracted transaction entry point.
: >"$transaction_log"
: >"$remote_output"
execute_output="$(run_case execute_remote 2>&1)"
grep -Fq 'transaction complete' <<<"$execute_output"
mapfile -t dispatched_transaction <"$transaction_log"
[ "${#dispatched_transaction[@]}" -eq 24 ]
[ "${dispatched_transaction[0]}" -eq 23 ]
[[ "${dispatched_transaction[1]}" == "$remote_runtime_root"/bibites-runtime.new.* ]]
[[ "${dispatched_transaction[2]}" == /tmp/bibites-runtime.*.tar.gz ]]
[ "${dispatched_transaction[3]}" = us-east-1 ]
[ "${dispatched_transaction[4]}" = 123456789012 ]
[ "${dispatched_transaction[5]}" = vol-0123456789abcdef0 ]
[ "${dispatched_transaction[6]}" = fixture-artifacts ]
[ "${dispatched_transaction[7]}" = cloud/v1 ]
[ "${dispatched_transaction[8]}" = "runtime/$runtime_sha.tar.gz" ]
[ "${dispatched_transaction[9]}" = "$runtime_sha" ]
[ "${dispatched_transaction[10]}" = "runtime-inputs/game/$game_sha.zip" ]
[ "${dispatched_transaction[11]}" = "$game_sha" ]
[ "${dispatched_transaction[12]}" = "runtime-inputs/bepinex/$bepinex_sha.zip" ]
[ "${dispatched_transaction[13]}" = "$bepinex_sha" ]
[ "${dispatched_transaction[14]}" = "worlds.$manifest_sha.json" ]
[ "${dispatched_transaction[15]}" = "$manifest_sha" ]
[ "${dispatched_transaction[16]}" = "$manifest_preimage_etag" ]
[ "${dispatched_transaction[17]}" = "$pointer_prefix" ]
[ "${dispatched_transaction[18]}" = 10.1.2.3 ]
[ "${dispatched_transaction[19]}" = relay.example.test ]
[ "${dispatched_transaction[20]}" = /opt/bibites-runtime ]
[ "${dispatched_transaction[21]}" = "runtime/$prior_sha.tar.gz" ]
[ "${dispatched_transaction[22]}" = "$prior_sha" ]
[ "${dispatched_transaction[23]}" = "$prior_etag" ]
grep -Eq '^/tmp/bibites-runtime\..*\.tar\.gz: OK$' "$remote_output"

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

: >"$aws_log"
set +e
prior_mismatch_output="$(run_case prior_mismatch 2>&1)"
prior_mismatch_status=$?
set -e
[ "$prior_mismatch_status" -eq 26 ] || {
  echo "terminal transaction status 26 returned $prior_mismatch_status" >&2
  exit 1
}
grep -Fq 'expected prior pointer mismatch' <<<"$prior_mismatch_output"

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

# All expected-prior inputs are required and validated before the first AWS call.
: >"$aws_log"
set +e
missing_prior_output="$(PATH="$fixture_bin:$PATH" \
  "$fixture_cloud/update-runtime.sh" 2>&1)"
missing_prior_status=$?
set -e
[ "$missing_prior_status" -eq 2 ]
[ ! -s "$aws_log" ]
grep -Fq 'Set all three expected-prior inputs' <<<"$missing_prior_output"

for malformed_prior in object sha256 etag relationship; do
  unset BIBITES_TEST_EXPECTED_PRIOR_OBJECT \
    BIBITES_TEST_EXPECTED_PRIOR_SHA256 BIBITES_TEST_EXPECTED_PRIOR_ETAG
  case "$malformed_prior" in
    object) BIBITES_TEST_EXPECTED_PRIOR_OBJECT='../unsafe-runtime.tar.gz' ;;
    sha256) BIBITES_TEST_EXPECTED_PRIOR_SHA256=abcd ;;
    etag) BIBITES_TEST_EXPECTED_PRIOR_ETAG=11111111111111111111111111111111 ;;
    relationship)
      BIBITES_TEST_EXPECTED_PRIOR_OBJECT=runtime/ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff.tar.gz
      ;;
  esac
  : >"$aws_log"
  set +e
  malformed_prior_output="$(run_case success 2>&1)"
  malformed_prior_status=$?
  set -e
  [ "$malformed_prior_status" -ne 0 ] || {
    echo "$malformed_prior expected-prior input passed" >&2
    exit 1
  }
  [ ! -s "$aws_log" ] || {
    echo "$malformed_prior expected-prior input reached AWS" >&2
    exit 1
  }
done
unset BIBITES_TEST_EXPECTED_PRIOR_OBJECT \
  BIBITES_TEST_EXPECTED_PRIOR_SHA256 BIBITES_TEST_EXPECTED_PRIOR_ETAG

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
for ambient_field in RUNTIME_SHA256 MANIFEST_PREIMAGE_ETAG; do
  write_staged_receipt runtime-only
  sed -i "/^$ambient_field=/d" "$fixture_dist/staged.env"
  : >"$aws_log"
  set +e
  case "$ambient_field" in
    RUNTIME_SHA256)
      ambient_output="$(BIBITES_TEST_AMBIENT_RUNTIME_SHA256="$runtime_sha" \
        run_case success 2>&1)"
      ;;
    MANIFEST_PREIMAGE_ETAG)
      ambient_output="$(BIBITES_TEST_AMBIENT_MANIFEST_PREIMAGE_ETAG="$manifest_preimage_etag" \
        run_case success 2>&1)"
      ;;
  esac
  ambient_status=$?
  set -e
  [ "$ambient_status" -ne 0 ]
  [ ! -s "$aws_log" ] || {
    echo "ambient value supplied missing $ambient_field receipt data" >&2
    exit 1
  }
  grep -Fq "Set $ambient_field before the runtime update" <<<"$ambient_output"
done

# The mutable-manifest preimage ETag must keep the exact quoted S3 form.
for malformed_manifest_etag in missing-quotes invalid-digest; do
  write_staged_receipt runtime-only
  case "$malformed_manifest_etag" in
    missing-quotes)
      sed -i 's/^MANIFEST_PREIMAGE_ETAG=.*/MANIFEST_PREIMAGE_ETAG=22222222222222222222222222222222/' \
        "$fixture_dist/staged.env"
      ;;
    invalid-digest)
      sed -i 's/^MANIFEST_PREIMAGE_ETAG=.*/MANIFEST_PREIMAGE_ETAG=\\"short\\"/' \
        "$fixture_dist/staged.env"
      ;;
  esac
  : >"$aws_log"
  set +e
  malformed_manifest_output="$(run_case success 2>&1)"
  malformed_manifest_status=$?
  set -e
  [ "$malformed_manifest_status" -ne 0 ]
  [ ! -s "$aws_log" ] || {
    echo "$malformed_manifest_etag manifest ETag reached AWS" >&2
    exit 1
  }
  grep -Fq 'MANIFEST_PREIMAGE_ETAG must be one quoted S3 ETag' \
    <<<"$malformed_manifest_output"
done

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
