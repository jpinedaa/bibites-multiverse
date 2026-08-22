#!/usr/bin/env bash
# Install one staged, content-addressed runtime without replacing the EC2 host.
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
dist="$repo/cloud/aws/dist"
validation="$repo/cloud/aws/lib/validation.sh"
[ -r "$dist/artifacts.env" ] || { echo 'run build-artifacts.sh first' >&2; exit 1; }
[ -r "$dist/staged.env" ] || { echo 'run stage-artifacts.sh first' >&2; exit 1; }
# shellcheck source=lib/validation.sh
. "$validation"
# shellcheck source=/dev/null
. "$dist/artifacts.env"
# These values belong exclusively to the immutable staging receipt. Do not let
# a missing receipt field fall back to the caller environment or artifacts.env.
unset AWS_PROFILE AWS_REGION ARTIFACT_BUCKET ARTIFACT_PREFIX STAGING_SCOPE \
  RUNTIME_OBJECT RUNTIME_SHA256 GAME_OBJECT GAME_SHA256 \
  BEPINEX_OBJECT BEPINEX_SHA256 MANIFEST_OBJECT MANIFEST_SHA256
# shellcheck source=/dev/null
. "$dist/staged.env"

[ "${STAGING_SCOPE:-}" = runtime-only ] || {
  echo 'update-runtime.sh requires STAGING_SCOPE=runtime-only from stage-artifacts.sh --runtime-only' >&2
  exit 1
}
for setting in AWS_PROFILE AWS_REGION ARTIFACT_BUCKET ARTIFACT_PREFIX \
  RUNTIME_OBJECT RUNTIME_SHA256 GAME_OBJECT GAME_SHA256 \
  BEPINEX_OBJECT BEPINEX_SHA256 MANIFEST_OBJECT MANIFEST_SHA256 \
  BIBITES_AWS_ACCOUNT_ID BIBITES_POINTER_CREDENTIAL_PARAMETER_PREFIX; do
  [ -n "${!setting:-}" ] || {
    printf 'Set %s before the runtime update.\n' "$setting" >&2
    exit 1
  }
done

bibites_require_account_id "$BIBITES_AWS_ACCOUNT_ID" BIBITES_AWS_ACCOUNT_ID
bibites_require_region "$AWS_REGION" AWS_REGION
bibites_require_s3_bucket "$ARTIFACT_BUCKET" ARTIFACT_BUCKET
bibites_require_s3_prefix "$ARTIFACT_PREFIX" ARTIFACT_PREFIX
for digest_name in RUNTIME_SHA256 GAME_SHA256 BEPINEX_SHA256 MANIFEST_SHA256; do
  bibites_require_sha256 "${!digest_name}" "$digest_name"
done
[ "$RUNTIME_OBJECT" = "runtime/$RUNTIME_SHA256.tar.gz" ] || {
  echo 'RUNTIME_OBJECT does not match the staged runtime digest' >&2
  exit 1
}
[ "$GAME_OBJECT" = "runtime-inputs/game/$GAME_SHA256.zip" ] || {
  echo 'GAME_OBJECT does not match the pinned game digest' >&2
  exit 1
}
[ "$BEPINEX_OBJECT" = "runtime-inputs/bepinex/$BEPINEX_SHA256.zip" ] || {
  echo 'BEPINEX_OBJECT does not match the pinned BepInEx digest' >&2
  exit 1
}
[ "$MANIFEST_OBJECT" = "worlds.$MANIFEST_SHA256.json" ] || {
  echo 'MANIFEST_OBJECT does not match the pinned manifest digest' >&2
  exit 1
}
for object_name in RUNTIME_OBJECT GAME_OBJECT BEPINEX_OBJECT MANIFEST_OBJECT; do
  bibites_require_s3_key "${!object_name}" "$object_name"
  bibites_require_s3_key "$ARTIFACT_PREFIX/${!object_name}" \
    "$object_name with ARTIFACT_PREFIX"
done
bibites_require_ssm_parameter "$BIBITES_POINTER_CREDENTIAL_PARAMETER_PREFIX" \
  BIBITES_POINTER_CREDENTIAL_PARAMETER_PREFIX
for suffix in access-key-id secret-access-key session-token; do
  bibites_require_ssm_parameter \
    "$BIBITES_POINTER_CREDENTIAL_PARAMETER_PREFIX/$suffix" \
    "pointer credential parameter $suffix"
done

account="$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" sts get-caller-identity \
  --query Account --output text)"
bibites_require_account_id "$account" 'authenticated AWS account'
[ "$account" = "$BIBITES_AWS_ACCOUNT_ID" ] || {
  echo "refusing AWS account $account; expected $BIBITES_AWS_ACCOUNT_ID" >&2
  exit 1
}

stack="${BIBITES_STACK_NAME:-bibites-cloud-worlds}"
bibites_require_stack_name "$stack" BIBITES_STACK_NAME
description="$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" cloudformation \
  describe-stacks --stack-name "$stack" --output json)"
jq -e '
  (.Stacks | length) == 1 and
  (.Stacks[0].StackStatus == "CREATE_COMPLETE" or
   .Stacks[0].StackStatus == "UPDATE_COMPLETE" or
   .Stacks[0].StackStatus == "UPDATE_ROLLBACK_COMPLETE" or
   .Stacks[0].StackStatus == "IMPORT_COMPLETE") and
  (.Stacks[0].Outputs | type == "array") and
  (.Stacks[0].Parameters | type == "array")
' <<<"$description" >/dev/null || {
  echo "stack $stack returned an incomplete description" >&2
  exit 1
}

stack_output() {
  local key="$1"
  jq -er --arg key "$key" '
    [.Stacks[0].Outputs[] | select(.OutputKey == $key) | .OutputValue] |
    if length == 1 and (.[0] | type == "string") and .[0] != ""
    then .[0]
    else error("missing or duplicate stack output " + $key)
    end
  ' <<<"$description"
}

stack_parameter() {
  local key="$1"
  jq -er --arg key "$key" '
    [.Stacks[0].Parameters[] | select(.ParameterKey == $key) | .ParameterValue] |
    if length == 1 and (.[0] | type == "string") and .[0] != ""
    then .[0]
    else error("missing, empty, or duplicate stack parameter " + $key)
    end
  ' <<<"$description"
}

instance="$(stack_output InstanceId)"
volume="$(stack_output DataVolumeId)"
stack_artifact_bucket="$(stack_parameter ArtifactBucket)"
stack_artifact_prefix="$(stack_parameter ArtifactPrefix)"
relay_private_ip="$(stack_parameter RelayPrivateIp)"

bibites_require_s3_bucket "$stack_artifact_bucket" \
  'ArtifactBucket stack parameter'
bibites_require_s3_prefix "$stack_artifact_prefix" \
  'ArtifactPrefix stack parameter'
[ "$ARTIFACT_BUCKET" = "$stack_artifact_bucket" ] &&
  [ "$ARTIFACT_PREFIX" = "$stack_artifact_prefix" ] || {
  echo 'the runtime-only staging target differs from the selected stack artifacts' >&2
  exit 1
}

relay_domain_parameters="$(jq -c '
  [.Stacks[0].Parameters[] |
    select(.ParameterKey == "RelayDomain") | .ParameterValue]
' <<<"$description")"
relay_domain_count="$(jq -r 'length' <<<"$relay_domain_parameters")"
case "$relay_domain_count" in
  0)
    relay_domain="${BIBITES_RELAY_DOMAIN:-}"
    [ -n "$relay_domain" ] || {
      echo 'stack has no RelayDomain parameter; set BIBITES_RELAY_DOMAIN' >&2
      exit 1
    }
    ;;
  1)
    relay_domain="$(jq -er '
      .[0] | select(type == "string" and length > 0)
    ' <<<"$relay_domain_parameters")" || {
      echo 'stack RelayDomain parameter is empty or invalid' >&2
      exit 1
    }
    if [ -n "${BIBITES_RELAY_DOMAIN:-}" ] &&
       [ "$BIBITES_RELAY_DOMAIN" != "$relay_domain" ]; then
      echo 'BIBITES_RELAY_DOMAIN differs from the stack RelayDomain parameter' >&2
      exit 1
    fi
    ;;
  *)
    echo 'stack has duplicate RelayDomain parameters' >&2
    exit 1
    ;;
esac

credential_prefix_parameters="$(jq -c '
  [.Stacks[0].Parameters[] |
    select(.ParameterKey == "CredentialParameterPrefix") | .ParameterValue]
' <<<"$description")"
credential_prefix_count="$(jq -r 'length' <<<"$credential_prefix_parameters")"
case "$credential_prefix_count" in
  0)
    credential_parameter_prefix="${BIBITES_CREDENTIAL_PARAMETER_PREFIX:-}"
    [ -n "$credential_parameter_prefix" ] || {
      echo 'stack has no CredentialParameterPrefix parameter; set BIBITES_CREDENTIAL_PARAMETER_PREFIX' >&2
      exit 1
    }
    ;;
  1)
    credential_parameter_prefix="$(jq -er '
      .[0] | select(type == "string" and length > 0)
    ' <<<"$credential_prefix_parameters")" || {
      echo 'stack CredentialParameterPrefix parameter is empty or invalid' >&2
      exit 1
    }
    if [ -n "${BIBITES_CREDENTIAL_PARAMETER_PREFIX:-}" ] &&
       [ "$BIBITES_CREDENTIAL_PARAMETER_PREFIX" != "$credential_parameter_prefix" ]; then
      echo 'BIBITES_CREDENTIAL_PARAMETER_PREFIX differs from the stack CredentialParameterPrefix parameter' >&2
      exit 1
    fi
    ;;
  *)
    echo 'stack has duplicate CredentialParameterPrefix parameters' >&2
    exit 1
    ;;
esac

bibites_require_resource_id "$instance" 'InstanceId stack output' i
bibites_require_resource_id "$volume" 'DataVolumeId stack output' vol
bibites_require_rfc1918_ipv4 "$relay_private_ip" 'RelayPrivateIp stack parameter'
bibites_require_hostname "$relay_domain" 'RelayDomain stack parameter'
bibites_require_ssm_parameter "$credential_parameter_prefix" \
  'CredentialParameterPrefix stack parameter'
case "$BIBITES_POINTER_CREDENTIAL_PARAMETER_PREFIX" in
  "$credential_parameter_prefix"/*) ;;
  *)
    echo 'pointer credential parameters are outside the host-readable prefix' >&2
    exit 1
    ;;
esac
volume_description="$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ec2 \
  describe-volumes --volume-ids "$volume" --output json)"
jq -e --arg volume "$volume" --arg instance "$instance" '
  (.Volumes | length) == 1 and
  .Volumes[0].VolumeId == $volume and
  any(.Volumes[0].Attachments[];
    .InstanceId == $instance and .State == "attached")
' <<<"$volume_description" >/dev/null || {
  echo "$volume is not attached to $instance" >&2
  exit 1
}

managed_description="$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ssm \
  describe-instance-information --filters "Key=InstanceIds,Values=$instance" \
  --output json)"
jq -e --arg instance "$instance" '
  (.InstanceInformationList | length) == 1 and
  .InstanceInformationList[0].InstanceId == $instance and
  .InstanceInformationList[0].PingStatus == "Online" and
  .InstanceInformationList[0].PlatformType == "Linux"
' <<<"$managed_description" >/dev/null || {
  echo "$instance is not an online Linux Systems Manager target" >&2
  exit 1
}

read -r -d '' remote_script <<'REMOTE' || true
set -euo pipefail
[ "$#" -eq 17 ] || { echo 'runtime update received the wrong argument count' >&2; exit 2; }

aws_region="$1"
expected_account="$2"
data_volume_id="$3"
artifact_bucket="$4"
artifact_prefix="$5"
runtime_object="$6"
runtime_sha256="$7"
game_object="$8"
game_sha256="$9"
bepinex_object="${10}"
bepinex_sha256="${11}"
manifest_object="${12}"
manifest_sha256="${13}"
pointer_credential_prefix="${14}"
relay_private_ip="${15}"
relay_domain="${16}"
runtime_root="${17}"
runtime_key="$artifact_prefix/$runtime_object"

archive="$(mktemp /tmp/bibites-runtime.XXXXXX.tar.gz)"
new_runtime="$(mktemp -d /opt/bibites-runtime.new.XXXXXX)"
cleanup() {
  rm -f "$archive"
  [ -z "${new_runtime:-}" ] || rm -rf "$new_runtime"
}
trap cleanup EXIT

aws --region "$aws_region" s3 cp \
  "s3://$artifact_bucket/$runtime_key" "$archive" --only-show-errors
printf '%s  %s\n' "$runtime_sha256" "$archive" | sha256sum -c -
tar -xzf "$archive" -C "$new_runtime"
test -x "$new_runtime/bibites-update-runtime-transaction"
test -x "$new_runtime/bibites-activate-runtime"
test -x "$new_runtime/install-host"
test -x "$new_runtime/bibites-stop-worlds"
test -r "$new_runtime/validate-world-manifest.jq"
bash -n "$new_runtime/bibites-update-runtime-transaction"
bash -n "$new_runtime/bibites-activate-runtime"

set +e
"$new_runtime/bibites-update-runtime-transaction" \
  "$new_runtime" "$archive" "$aws_region" "$expected_account" "$data_volume_id" \
  "$artifact_bucket" "$artifact_prefix" "$runtime_object" "$runtime_sha256" \
  "$game_object" "$game_sha256" "$bepinex_object" "$bepinex_sha256" \
  "$manifest_object" "$manifest_sha256" "$pointer_credential_prefix" \
  "$relay_private_ip" "$relay_domain" "$runtime_root"
transaction_status=$?
set -e
exit "$transaction_status"
REMOTE

remote_arguments=(
  "$AWS_REGION"
  "$BIBITES_AWS_ACCOUNT_ID"
  "$volume"
  "$ARTIFACT_BUCKET"
  "$ARTIFACT_PREFIX"
  "$RUNTIME_OBJECT"
  "$RUNTIME_SHA256"
  "$GAME_OBJECT"
  "$GAME_SHA256"
  "$BEPINEX_OBJECT"
  "$BEPINEX_SHA256"
  "$MANIFEST_OBJECT"
  "$MANIFEST_SHA256"
  "$BIBITES_POINTER_CREDENTIAL_PARAMETER_PREFIX"
  "$relay_private_ip"
  "$relay_domain"
  /opt/bibites-runtime
)
encoded="$(printf '%s' "$remote_script" | base64 -w0)"
printf -v quoted_arguments ' %q' "${remote_arguments[@]}"
printf -v remote_command 'printf %%s %q | base64 -d | bash -s --%s' \
  "$encoded" "$quoted_arguments"
parameters="$(jq -nc --arg command "$remote_command" \
  '{commands:[$command],executionTimeout:["3600"]}')"

command_id=
unknown_partial() {
  if [[ "$command_id" =~ ^[0-9a-f-]{36}$ ]]; then
    echo "runtime transaction $command_id has unknown partial state" >&2
  else
    echo 'runtime transaction has unknown partial state; command ID is unavailable' >&2
  fi
  echo 'Do not retry it blindly; reconcile the SSM command, host runtime, and pointer first.' >&2
  exit 25
}

if ! command_id="$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ssm \
  send-command --instance-ids "$instance" --document-name AWS-RunShellScript \
  --parameters "$parameters" --comment 'Run guarded Bibites runtime transaction' \
  --timeout-seconds 120 --query Command.CommandId --output text)"; then
  echo 'Systems Manager send-command did not return a confirmed result.' >&2
  unknown_partial
fi
[[ "$command_id" =~ ^[0-9a-f-]{36}$ ]] || {
  echo 'Systems Manager accepted a transaction but returned an invalid command ID' >&2
  unknown_partial
}
printf 'Systems Manager command ID: %s\n' "$command_id"

if invocation="$(bibites_wait_ssm_invocation "$AWS_PROFILE" "$AWS_REGION" \
  "$command_id" "$instance" 3720 3)"; then
  wait_status=0
else
  wait_status=$?
fi
if [ -n "$invocation" ]; then
  jq '{status:.Status,responseCode:.ResponseCode,
    stdout:.StandardOutputContent,stderr:.StandardErrorContent}' <<<"$invocation"
fi

if [ "$wait_status" -eq 0 ]; then
  success_response_code="$(jq -r '.ResponseCode // empty' <<<"$invocation")"
  [ "$success_response_code" = 0 ] || unknown_partial
  exit 0
fi
case "$wait_status" in
  2|124) unknown_partial ;;
esac
invocation_status="$(jq -r '.Status // empty' <<<"${invocation:-null}")"
case "$invocation_status" in
  TimedOut|Cancelled|Cancelling) unknown_partial ;;
  Failed)
    response_code="$(jq -r '.ResponseCode // empty' <<<"$invocation")"
    case "$response_code" in
      2|20|21|22|23|24|73) exit "$response_code" ;;
      *) unknown_partial ;;
    esac
    ;;
  *) unknown_partial ;;
esac
