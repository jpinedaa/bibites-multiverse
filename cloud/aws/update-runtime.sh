#!/usr/bin/env bash
# Install the staged, content-addressed runtime without replacing the EC2 host.
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
# shellcheck source=/dev/null
. "$dist/staged.env"
: "${RUNTIME_OBJECT:?run stage-artifacts.sh again to create an immutable runtime object}"
: "${BIBITES_AWS_ACCOUNT_ID:?set the approved 12-digit AWS account identifier}"

bibites_require_account_id "$BIBITES_AWS_ACCOUNT_ID" BIBITES_AWS_ACCOUNT_ID
bibites_require_region "$AWS_REGION" AWS_REGION
bibites_require_s3_bucket "$ARTIFACT_BUCKET" ARTIFACT_BUCKET
bibites_require_s3_prefix "$ARTIFACT_PREFIX" ARTIFACT_PREFIX
bibites_require_s3_key "$RUNTIME_OBJECT" RUNTIME_OBJECT
bibites_require_s3_filename "$RUNTIME_FILE" RUNTIME_FILE
bibites_require_sha256 "$RUNTIME_SHA256" RUNTIME_SHA256
bibites_require_s3_filename "$GAME_FILE" GAME_FILE
bibites_require_sha256 "$GAME_SHA256" GAME_SHA256
bibites_require_s3_filename "$BEPINEX_FILE" BEPINEX_FILE
bibites_require_sha256 "$BEPINEX_SHA256" BEPINEX_SHA256
[ "$RUNTIME_OBJECT" = "runtime/$RUNTIME_SHA256.tar.gz" ] || {
  echo 'RUNTIME_OBJECT does not match the staged runtime digest' >&2
  exit 1
}
runtime_key="$ARTIFACT_PREFIX/$RUNTIME_OBJECT"
game_key="$ARTIFACT_PREFIX/$GAME_FILE"
bepinex_key="$ARTIFACT_PREFIX/$BEPINEX_FILE"
manifest_key="$ARTIFACT_PREFIX/worlds.json"
for key in "$runtime_key" "$game_key" "$bepinex_key" "$manifest_key"; do
  bibites_require_s3_key "$key" "runtime object key $key"
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

stack_parameter_optional() {
  local key="$1"
  jq -er --arg key "$key" '
    [.Stacks[0].Parameters[] | select(.ParameterKey == $key) | .ParameterValue] |
    if length == 0 then ""
    elif length == 1 and (.[0] | type == "string") and .[0] != "" then .[0]
    else error("empty or duplicate stack parameter " + $key)
    end
  ' <<<"$description"
}

instance="$(stack_output InstanceId)"
volume="$(stack_output DataVolumeId)"
relay_private_ip="$(stack_parameter_optional RelayPrivateIp)"
stack_relay_domain="$(stack_parameter_optional RelayDomain)"

bibites_require_resource_id "$instance" 'InstanceId stack output' i
bibites_require_resource_id "$volume" 'DataVolumeId stack output' vol
bibites_require_rfc1918_ipv4 "$relay_private_ip" 'RelayPrivateIp stack parameter'

if [ -n "$stack_relay_domain" ]; then
  relay_domain="$stack_relay_domain"
  if [ -n "${BIBITES_RELAY_DOMAIN:-}" ] && [ "$BIBITES_RELAY_DOMAIN" != "$relay_domain" ]; then
    echo 'BIBITES_RELAY_DOMAIN differs from the stack RelayDomain parameter' >&2
    exit 1
  fi
else
  : "${BIBITES_RELAY_DOMAIN:?legacy stack has no RelayDomain; set an explicit validated BIBITES_RELAY_DOMAIN}"
  relay_domain="$BIBITES_RELAY_DOMAIN"
fi
bibites_require_hostname "$relay_domain" BIBITES_RELAY_DOMAIN

volume_description="$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ec2 describe-volumes \
  --volume-ids "$volume" --output json)"
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
  describe-instance-information --filters "Key=InstanceIds,Values=$instance" --output json)"
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
[ "$#" -eq 13 ] || { echo 'runtime update received the wrong argument count' >&2; exit 1; }

aws_region="$1"
data_volume_id="$2"
artifact_bucket="$3"
runtime_key="$4"
runtime_sha256="$5"
game_key="$6"
game_sha256="$7"
bepinex_key="$8"
bepinex_sha256="$9"
manifest_key="${10}"
relay_private_ip="${11}"
relay_domain="${12}"
runtime_root="${13}"

[[ "$aws_region" =~ ^[a-z]{2}(-gov)?-[a-z]+-[0-9]+$ ]] || exit 2
[[ "$data_volume_id" =~ ^vol-[0-9a-f]{8,17}$ ]] || exit 2
[[ "$artifact_bucket" =~ ^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$ ]] || exit 2
[[ "$runtime_sha256" =~ ^[0-9a-f]{64}$ ]] || exit 2
[[ "$game_sha256" =~ ^[0-9a-f]{64}$ ]] || exit 2
[[ "$bepinex_sha256" =~ ^[0-9a-f]{64}$ ]] || exit 2
[[ "$relay_private_ip" =~ ^[0-9]{1,3}(\.[0-9]{1,3}){3}$ ]] || exit 2
[[ "$relay_domain" =~ ^([A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?\.)+[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?$ ]] || exit 2
[ "$runtime_root" = /opt/bibites-runtime ] || exit 2

archive="$(mktemp /tmp/bibites-runtime.XXXXXX.tar.gz)"
new_runtime="$(mktemp -d /opt/bibites-runtime.new.XXXXXX)"
cleanup() {
  rm -f "$archive"
  [ -z "${new_runtime:-}" ] || rm -rf "$new_runtime"
}
trap cleanup EXIT

aws --region "$aws_region" s3 cp "s3://$artifact_bucket/$runtime_key" "$archive" \
  --only-show-errors
printf '%s  %s\n' "$runtime_sha256" "$archive" | sha256sum -c -
tar -xzf "$archive" -C "$new_runtime"
test -x "$new_runtime/install-host"
test -x "$new_runtime/bibites-stop-worlds"
test -x "$new_runtime/bibites-activate-runtime"
test -r "$new_runtime/validate-world-manifest.jq"
jq -n -f "$new_runtime/validate-world-manifest.jq" >/dev/null
while IFS= read -r script; do
  bash -n "$script"
done < <(find "$new_runtime" -maxdepth 1 -type f -name 'bibites-*' -print)

set +e
"$new_runtime/bibites-activate-runtime" \
  "$new_runtime" "$archive" "$aws_region" "$data_volume_id" \
  "$artifact_bucket" "$runtime_key" "$runtime_sha256" \
  "$game_key" "$game_sha256" "$bepinex_key" "$bepinex_sha256" \
  "$manifest_key" "$relay_private_ip" "$relay_domain" "$runtime_root"
activate_status=$?
set -e
exit "$activate_status"
REMOTE

remote_arguments=(
  "$AWS_REGION"
  "$volume"
  "$ARTIFACT_BUCKET"
  "$runtime_key"
  "$RUNTIME_SHA256"
  "$game_key"
  "$GAME_SHA256"
  "$bepinex_key"
  "$BEPINEX_SHA256"
  "$manifest_key"
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
command_id="$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ssm send-command \
  --instance-ids "$instance" --document-name AWS-RunShellScript --parameters "$parameters" \
  --comment 'Install staged Bibites cloud runtime' --timeout-seconds 120 \
  --query Command.CommandId --output text)"
[[ "$command_id" =~ ^[0-9a-f-]{36}$ ]] || {
  echo "Systems Manager returned an invalid command identifier: $command_id" >&2
  exit 1
}
set +e
invocation="$(bibites_wait_ssm_invocation "$AWS_PROFILE" "$AWS_REGION" \
  "$command_id" "$instance" 3720 3)"
wait_status=$?
set -e

if [ -n "$invocation" ]; then
  jq '{status:.Status,responseCode:.ResponseCode,
    stdout:.StandardOutputContent,stderr:.StandardErrorContent}' <<<"$invocation"
fi
if [ "$wait_status" -eq 0 ]; then
  "$repo/cloud/aws/promote-runtime.sh" "$RUNTIME_OBJECT" "$RUNTIME_SHA256"
  exit 0
fi

response_code="$(jq -r '.ResponseCode // empty' <<<"${invocation:-{}}")"
case "$response_code" in
  20)
    echo 'runtime update failed; the host completed rollback' >&2
    exit 20
    ;;
  21)
    echo 'runtime update failed; host rollback also failed' >&2
    exit 21
    ;;
esac
exit "$wait_status"
