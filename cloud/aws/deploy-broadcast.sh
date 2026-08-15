#!/usr/bin/env bash
# Deploy one GPU broadcaster from a snapshot after the source world stops.
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
dist="$repo/cloud/aws/dist"
template="$repo/cloud/aws/broadcast-template.yaml"
validation="$repo/cloud/aws/lib/validation.sh"
manifest_validator="$repo/cloud/aws/runtime/validate-world-manifest.jq"

[ -r "$dist/artifacts.env" ] || { echo 'run build-artifacts.sh first' >&2; exit 1; }
[ -r "$dist/staged.env" ] || { echo 'run stage-artifacts.sh first' >&2; exit 1; }

# shellcheck source=lib/validation.sh
. "$validation"
# shellcheck source=/dev/null
. "$dist/artifacts.env"
# shellcheck source=/dev/null
. "$dist/staged.env"

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
bibites_require_sha256 "$RUNTIME_SHA256" RUNTIME_SHA256
[[ "$world" =~ ^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$ ]] || {
  echo "invalid world identifier: $world" >&2
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

bibites_require_x86_64_instance_type "$AWS_PROFILE" "$AWS_REGION" "$instance_type"

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
awk -v value="$quota" 'BEGIN { exit !(value >= 4) }' || {
  echo "Spot G-instance quota is $quota vCPUs; at least 4 vCPUs are required" >&2
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

parameter_name="$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ssm get-parameter \
  --name "$BIBITES_PUBLISH_PASSWORD_PARAMETER" \
  --query Parameter.Name --output text)"
[ "$parameter_name" = "$BIBITES_PUBLISH_PASSWORD_PARAMETER" ] || {
  echo 'Parameter Store returned an unexpected publish-password parameter' >&2
  exit 1
}

manifest="$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" s3 cp \
  "s3://$ARTIFACT_BUCKET/$ARTIFACT_PREFIX/worlds.json" - --only-show-errors \
  )"
jq -e -f "$manifest_validator" <<<"$manifest" >/dev/null
jq -e --arg world "$world" \
  'any(.worlds[]; .id == $world and .enabled == false)' <<<"$manifest" >/dev/null || {
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

printf -v source_check \
  '! systemctl is-active --quiet bibites-game@%q.service; ! systemctl is-active --quiet bibites-sidecar@%q.service; test "$(systemctl is-enabled bibites-game@%q.service 2>/dev/null || true)" = disabled; test "$(systemctl is-enabled bibites-sidecar@%q.service 2>/dev/null || true)" = disabled' \
  "$world" "$world" "$world" "$world"
parameters="$(jq -nc --arg command "$source_check" '{commands:[$command]}')"

command_id="$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ssm send-command \
  --instance-ids "$source_instance" --document-name AWS-RunShellScript \
  --parameters "$parameters" --query Command.CommandId --output text)"
[[ "$command_id" =~ ^[0-9a-f-]{36}$ ]] || {
  echo "Systems Manager returned an invalid command identifier: $command_id" >&2
  exit 1
}
aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ssm wait command-executed \
  --command-id "$command_id" --instance-id "$source_instance" || true

source_status="$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ssm get-command-invocation \
  --command-id "$command_id" --instance-id "$source_instance" --query Status --output text)"
[ "$source_status" = Success ] || {
  echo "$world is active or enabled on the source host; refusing a duplicate deployment" >&2
  exit 1
}

aws --profile "$AWS_PROFILE" --region "$AWS_REGION" cloudformation deploy \
  --stack-name "$stack" --template-file "$template" \
  --capabilities CAPABILITY_NAMED_IAM \
  --parameter-overrides \
    InstanceType="$instance_type" \
    AvailabilityZone="$BIBITES_AVAILABILITY_ZONE" \
    SubnetId="$BIBITES_SUBNET_ID" \
    VpcId="$BIBITES_VPC_ID" \
    ArtifactBucket="$ARTIFACT_BUCKET" \
    RuntimeKey="$runtime_key" \
    RuntimeSha256="$RUNTIME_SHA256" \
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
