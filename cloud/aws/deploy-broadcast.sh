#!/usr/bin/env bash
# Deploy one GPU broadcaster from a snapshot after the source world stops.
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
dist="$repo/cloud/aws/dist"
template="$repo/cloud/aws/broadcast-template.yaml"
validation="$repo/cloud/aws/lib/validation.sh"
manifest_validator="$repo/cloud/aws/runtime/validate-world-manifest.jq"
source_checker="$repo/cloud/aws/source-world-stopped.sh"

[ -r "$dist/artifacts.env" ] || { echo 'run build-artifacts.sh first' >&2; exit 1; }
[ -r "$dist/staged.env" ] || { echo 'run stage-artifacts.sh first' >&2; exit 1; }
[ -r "$dist/worlds.json" ] || { echo 'stage-artifacts.sh did not stage a manifest' >&2; exit 1; }
[ -r "$source_checker" ] || { echo 'missing source-world safety check' >&2; exit 1; }

# shellcheck source=lib/validation.sh
. "$validation"
# Keep build identity separate from receipt identity. A missing field in either
# file must not inherit a value from the caller or from the other file.
unset RUNTIME_FILE RUNTIME_SHA256 GAME_FILE GAME_SHA256 \
  BEPINEX_FILE BEPINEX_SHA256
# shellcheck source=/dev/null
. "$dist/artifacts.env"
artifact_runtime_file="${RUNTIME_FILE:-}"
artifact_runtime_sha256="${RUNTIME_SHA256:-}"
artifact_game_file="${GAME_FILE:-}"
artifact_game_sha256="${GAME_SHA256:-}"
artifact_bepinex_file="${BEPINEX_FILE:-}"
artifact_bepinex_sha256="${BEPINEX_SHA256:-}"
# The receipt must supply its own scope, target, and staged identity. Do not let
# ambient settings make an incomplete receipt look complete.
unset AWS_PROFILE AWS_REGION ARTIFACT_BUCKET ARTIFACT_PREFIX RUNTIME_OBJECT \
  STAGING_SCOPE STAGED_RUNTIME_SHA256 STAGED_GAME_SHA256 STAGED_BEPINEX_SHA256 \
  MANIFEST_OBJECT MANIFEST_SHA256
# shellcheck source=/dev/null
. "$dist/staged.env"
RUNTIME_FILE="$artifact_runtime_file"
RUNTIME_SHA256="$artifact_runtime_sha256"
GAME_FILE="$artifact_game_file"
GAME_SHA256="$artifact_game_sha256"
BEPINEX_FILE="$artifact_bepinex_file"
BEPINEX_SHA256="$artifact_bepinex_sha256"
[ "${STAGING_SCOPE:-}" = complete ] || {
  echo 'deploy-broadcast.sh requires STAGING_SCOPE=complete from stage-artifacts.sh' >&2
  exit 1
}
for receipt_name in STAGED_RUNTIME_SHA256 STAGED_GAME_SHA256 STAGED_BEPINEX_SHA256; do
  [ -n "${!receipt_name:-}" ] || {
    echo "the complete staging receipt is missing $receipt_name; run stage-artifacts.sh again" >&2
    exit 1
  }
  bibites_require_sha256 "${!receipt_name}" "$receipt_name"
done
[ "$STAGED_RUNTIME_SHA256" = "${RUNTIME_SHA256:-}" ] &&
  [ "$STAGED_GAME_SHA256" = "${GAME_SHA256:-}" ] &&
  [ "$STAGED_BEPINEX_SHA256" = "${BEPINEX_SHA256:-}" ] || {
  echo 'the complete staging receipt does not match artifacts.env; run stage-artifacts.sh again' >&2
  exit 1
}
[ "${RUNTIME_OBJECT:-}" = "runtime/$STAGED_RUNTIME_SHA256.tar.gz" ] || {
  echo 'the complete staging receipt runtime object does not match its digest' >&2
  exit 1
}
for receipt_name in MANIFEST_OBJECT MANIFEST_SHA256; do
  [ -n "${!receipt_name:-}" ] || {
    echo "the complete staging receipt is missing $receipt_name; run stage-artifacts.sh again" >&2
    exit 1
  }
done
bibites_require_sha256 "$MANIFEST_SHA256" MANIFEST_SHA256
bibites_require_s3_filename "$MANIFEST_OBJECT" MANIFEST_OBJECT
[ "$MANIFEST_OBJECT" = "worlds.$MANIFEST_SHA256.json" ] || {
  echo 'the complete staging receipt manifest object does not match its digest' >&2
  exit 1
}
local_manifest_sha256="$(sha256sum "$dist/worlds.json" | awk '{print $1}')"
[ "$local_manifest_sha256" = "$MANIFEST_SHA256" ] || {
  echo 'the staged manifest does not match the complete staging receipt' >&2
  echo 'run stage-artifacts.sh again' >&2
  exit 1
}

: "${BIBITES_AWS_ACCOUNT_ID:?set the approved 12-digit AWS account identifier}"
: "${BIBITES_BROADCAST_SNAPSHOT_ID:?set a completed snapshot made after the source world stopped}"
: "${BIBITES_SOURCE_STACK_NAME:?set the source headless-world stack name}"
: "${BIBITES_BROADCAST_INSTANCE_TYPE:?set the GPU instance type}"
: "${BIBITES_AVAILABILITY_ZONE:?set the target availability zone}"
: "${BIBITES_SUBNET_ID:?set the target subnet identifier}"
: "${BIBITES_VPC_ID:?set the target VPC identifier}"
: "${BIBITES_RELAY_PRIVATE_IP:?set the private relay address}"
: "${BIBITES_RELAY_DOMAIN:?set the relay certificate name}"
: "${BIBITES_STREAM_RTMP_ADDRESS:?set the private RTMP address on port 1935}"
: "${BIBITES_PUBLISH_PASSWORD_PARAMETER:?set the existing SSM publish-password parameter}"
: "${BIBITES_BROADCAST_WORLD_ID:?set the manifest world identifier}"
: "${RUNTIME_OBJECT:?run stage-artifacts.sh again to create an immutable runtime object}"

stack="${BIBITES_BROADCAST_STACK_NAME:-bibites-live-broadcast}"
snapshot="$BIBITES_BROADCAST_SNAPSHOT_ID"
world="$BIBITES_BROADCAST_WORLD_ID"
instance_type="$BIBITES_BROADCAST_INSTANCE_TYPE"
runtime_key="$ARTIFACT_PREFIX/$RUNTIME_OBJECT"

bibites_require_account_id "$BIBITES_AWS_ACCOUNT_ID" BIBITES_AWS_ACCOUNT_ID
bibites_require_region "$AWS_REGION" AWS_REGION
bibites_require_stack_name "$stack" BIBITES_BROADCAST_STACK_NAME
bibites_require_stack_name "$BIBITES_SOURCE_STACK_NAME" BIBITES_SOURCE_STACK_NAME
bibites_require_resource_id "$snapshot" BIBITES_BROADCAST_SNAPSHOT_ID snap
bibites_require_instance_type "$instance_type" BIBITES_BROADCAST_INSTANCE_TYPE
[[ "$instance_type" =~ ^g[0-9][a-z0-9.]*$ ]] || {
  echo 'BIBITES_BROADCAST_INSTANCE_TYPE must be an EC2 G-family instance type' >&2
  exit 1
}
bibites_require_resource_id "$BIBITES_SUBNET_ID" BIBITES_SUBNET_ID subnet
bibites_require_resource_id "$BIBITES_VPC_ID" BIBITES_VPC_ID vpc
bibites_require_availability_zone "$BIBITES_AVAILABILITY_ZONE" BIBITES_AVAILABILITY_ZONE
bibites_require_rfc1918_ipv4 "$BIBITES_RELAY_PRIVATE_IP" BIBITES_RELAY_PRIVATE_IP
bibites_require_hostname "$BIBITES_RELAY_DOMAIN" BIBITES_RELAY_DOMAIN
bibites_require_ssm_parameter "$BIBITES_PUBLISH_PASSWORD_PARAMETER" \
  BIBITES_PUBLISH_PASSWORD_PARAMETER
bibites_require_s3_bucket "$ARTIFACT_BUCKET" ARTIFACT_BUCKET
bibites_require_s3_prefix "$ARTIFACT_PREFIX" ARTIFACT_PREFIX
bibites_require_s3_key "$RUNTIME_OBJECT" RUNTIME_OBJECT
bibites_require_s3_key "$runtime_key" 'broadcast runtime object key'
bibites_require_s3_key "$ARTIFACT_PREFIX/$MANIFEST_OBJECT" \
  'broadcast manifest object key'
bibites_require_sha256 "$RUNTIME_SHA256" RUNTIME_SHA256
[[ "$world" =~ ^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$ ]] || {
  echo "invalid world identifier: $world" >&2
  exit 1
}
[ "$world" = slot-1 ] || {
  echo 'BIBITES_BROADCAST_WORLD_ID must be slot-1' >&2
  exit 1
}

rtmp_ip="${BIBITES_STREAM_RTMP_ADDRESS%:1935}"
[ "$BIBITES_STREAM_RTMP_ADDRESS" = "$rtmp_ip:1935" ] || {
  echo 'BIBITES_STREAM_RTMP_ADDRESS must use an IPv4 address on port 1935' >&2
  exit 1
}
bibites_require_rfc1918_ipv4 "$rtmp_ip" BIBITES_STREAM_RTMP_ADDRESS

account="$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" sts get-caller-identity \
  --query Account --output text)"
bibites_require_account_id "$account" 'authenticated AWS account'
[ "$account" = "$BIBITES_AWS_ACCOUNT_ID" ] || {
  echo "refusing AWS account $account; expected $BIBITES_AWS_ACCOUNT_ID" >&2
  exit 1
}

subnet_description="$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ec2 describe-subnets \
  --subnet-ids "$BIBITES_SUBNET_ID" --output json)"
jq -e --arg subnet "$BIBITES_SUBNET_ID" --arg vpc "$BIBITES_VPC_ID" \
  --arg zone "$BIBITES_AVAILABILITY_ZONE" '
    (.Subnets | length) == 1 and
    .Subnets[0].SubnetId == $subnet and
    .Subnets[0].VpcId == $vpc and
    .Subnets[0].AvailabilityZone == $zone
  ' <<<"$subnet_description" >/dev/null || {
  echo 'the subnet, VPC, and availability zone do not describe one target subnet' >&2
  exit 1
}

instance_description="$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ec2 \
  describe-instance-types --instance-types "$instance_type" --output json)"
bibites_require_x86_64_instance_description "$instance_description" "$instance_type"
bibites_require_nvidia_gpu_instance_description "$instance_description" "$instance_type"
required_vcpus="$(bibites_instance_default_vcpus "$instance_description" "$instance_type")"

route_table="$(bibites_effective_route_table "$AWS_PROFILE" "$AWS_REGION" \
  "$BIBITES_VPC_ID" "$BIBITES_SUBNET_ID")"
relay_route="$(bibites_private_route_for_ip "$route_table" "$BIBITES_RELAY_PRIVATE_IP")" || {
  echo 'the selected subnet has no approved private route to the relay address' >&2
  exit 1
}
rtmp_route="$(bibites_private_route_for_ip "$route_table" "$rtmp_ip")" || {
  echo 'the selected subnet has no approved private route to the RTMP address' >&2
  exit 1
}
printf 'approved relay route: %s\n' "$relay_route"
printf 'approved RTMP route: %s\n' "$rtmp_route"

quota="$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" service-quotas get-service-quota \
  --service-code ec2 --quota-code L-3819A6DF --query Quota.Value --output text)"
[[ "$quota" =~ ^[0-9]+([.][0-9]+)?$ ]] || {
  echo "Spot G-instance quota is malformed: $quota" >&2
  exit 1
}
awk -v value="$quota" -v required="$required_vcpus" \
  'BEGIN { exit !(value >= required) }' || {
  echo "Spot G-instance quota is $quota vCPUs; $instance_type requires $required_vcpus" >&2
  exit 1
}

snapshot_description="$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ec2 describe-snapshots \
  --snapshot-ids "$snapshot" --output json)"
jq -e --arg snapshot "$snapshot" --arg owner "$account" '
  (.Snapshots | length) == 1 and
  .Snapshots[0].SnapshotId == $snapshot and
  .Snapshots[0].OwnerId == $owner and
  .Snapshots[0].Encrypted == true and
  .Snapshots[0].State == "completed"
' <<<"$snapshot_description" >/dev/null || {
  echo "snapshot $snapshot must be completed, encrypted, and owned by the approved account" >&2
  exit 1
}

bibites_require_default_secure_parameter "$AWS_PROFILE" "$AWS_REGION" \
  "$BIBITES_PUBLISH_PASSWORD_PARAMETER"

jq -e -f "$manifest_validator" "$dist/worlds.json" >/dev/null
jq -e --arg world "$world" \
  'any(.worlds[]; .id == $world and .enabled == false)' \
  "$dist/worlds.json" >/dev/null || {
    echo "the staged manifest must disable $world before GPU deployment" >&2
    exit 1
  }

source_description="$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" \
  cloudformation describe-stacks --stack-name "$BIBITES_SOURCE_STACK_NAME" --output json)"
source_instance="$(jq -er '
  [.Stacks[0].Outputs[] | select(.OutputKey == "InstanceId") | .OutputValue] |
  if length == 1 then .[0] else error("missing or duplicate InstanceId") end
' <<<"$source_description")"
bibites_require_resource_id "$source_instance" 'source stack InstanceId output' i

source_managed="$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ssm \
  describe-instance-information --filters "Key=InstanceIds,Values=$source_instance" --output json)"
jq -e --arg instance "$source_instance" '
  (.InstanceInformationList | length) == 1 and
  .InstanceInformationList[0].InstanceId == $instance and
  .InstanceInformationList[0].PingStatus == "Online" and
  .InstanceInformationList[0].PlatformType == "Linux"
' <<<"$source_managed" >/dev/null || {
  echo "$source_instance is not an online Linux Systems Manager target" >&2
  exit 1
}

echo 'broadcast deployment is disabled because FFmpeg exposes the RTMP password in process arguments' >&2
echo 'No remote source check or CloudFormation change was sent.' >&2
exit 1

source_check_encoded="$(base64 -w0 "$source_checker")"
printf -v source_check 'printf %%s %q | base64 -d | bash -s -- %q' \
  "$source_check_encoded" "$world"
parameters="$(jq -nc --arg command "$source_check" \
  '{commands:[$command],executionTimeout:["120"]}')"

command_id="$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ssm send-command \
  --instance-ids "$source_instance" --document-name AWS-RunShellScript \
  --parameters "$parameters" --timeout-seconds 60 \
  --query Command.CommandId --output text)"
[[ "$command_id" =~ ^[0-9a-f-]{36}$ ]] || {
  echo "Systems Manager returned an invalid command identifier: $command_id" >&2
  exit 1
}
set +e
source_result="$(bibites_wait_ssm_invocation "$AWS_PROFILE" "$AWS_REGION" \
  "$command_id" "$source_instance" 180 3)"
source_wait_status=$?
set -e
if [ "$source_wait_status" -ne 0 ]; then
  [ -z "$source_result" ] || jq \
    '{status:.Status,stdout:.StandardOutputContent,stderr:.StandardErrorContent}' \
    <<<"$source_result"
  echo "$world is active or enabled on the source host; refusing a duplicate deployment" >&2
  exit 1
fi

aws --profile "$AWS_PROFILE" --region "$AWS_REGION" cloudformation deploy \
  --stack-name "$stack" --template-file "$template" \
  --capabilities CAPABILITY_NAMED_IAM \
  --parameter-overrides \
    InstanceType="$instance_type" \
    AvailabilityZone="$BIBITES_AVAILABILITY_ZONE" \
    SubnetId="$BIBITES_SUBNET_ID" \
    VpcId="$BIBITES_VPC_ID" \
    ArtifactBucket="$ARTIFACT_BUCKET" \
    ArtifactPrefix="$ARTIFACT_PREFIX" \
    RuntimeKey="$runtime_key" \
    RuntimeSha256="$RUNTIME_SHA256" \
    ManifestFile="$MANIFEST_OBJECT" \
    ManifestSha256="$MANIFEST_SHA256" \
    DataSnapshotId="$snapshot" \
    RelayPrivateIp="$BIBITES_RELAY_PRIVATE_IP" \
    RelayDomain="$BIBITES_RELAY_DOMAIN" \
    StreamRtmpAddress="$BIBITES_STREAM_RTMP_ADDRESS" \
    PublishPasswordParameter="$BIBITES_PUBLISH_PASSWORD_PARAMETER" \
    WorldId="$world"

instance="$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" cloudformation describe-stacks \
  --stack-name "$stack" \
  --query 'Stacks[0].Outputs[?OutputKey==`InstanceId`].OutputValue' --output text)"
bibites_require_resource_id "$instance" 'broadcast InstanceId output' i
printf 'broadcast stack ready; instance=%s\n' "$instance"
