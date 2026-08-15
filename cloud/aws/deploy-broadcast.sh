#!/usr/bin/env bash
# Deploy one GPU broadcaster from a snapshot after the source world stops.
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
dist="$repo/cloud/aws/dist"
template="$repo/cloud/aws/broadcast-template.yaml"

[ -r "$dist/artifacts.env" ] || { echo 'run build-artifacts.sh first' >&2; exit 1; }
[ -r "$dist/staged.env" ] || { echo 'run stage-artifacts.sh first' >&2; exit 1; }

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

stack="${BIBITES_BROADCAST_STACK_NAME:-bibites-live-broadcast}"
snapshot="$BIBITES_BROADCAST_SNAPSHOT_ID"
world="$BIBITES_BROADCAST_WORLD_ID"

case "$world" in
  ''|*[!A-Za-z0-9._-]*) echo "invalid world identifier: $world" >&2; exit 1 ;;
esac

account="$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" sts get-caller-identity \
  --query Account --output text)"
[ "$account" = "$BIBITES_AWS_ACCOUNT_ID" ] || {
  echo "refusing AWS account $account; expected $BIBITES_AWS_ACCOUNT_ID" >&2
  exit 1
}

quota="$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" service-quotas get-service-quota \
  --service-code ec2 --quota-code L-3819A6DF --query Quota.Value --output text)"
awk -v value="$quota" 'BEGIN { exit !(value >= 4) }' || {
  echo "Spot G-instance quota is $quota vCPUs; at least 4 vCPUs are required" >&2
  exit 1
}

state="$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ec2 describe-snapshots \
  --snapshot-ids "$snapshot" --query 'Snapshots[0].State' --output text)"
[ "$state" = completed ] || { echo "snapshot $snapshot is $state, not completed" >&2; exit 1; }

aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ssm get-parameter \
  --name "$BIBITES_PUBLISH_PASSWORD_PARAMETER" \
  --query Parameter.Name --output text >/dev/null

aws --profile "$AWS_PROFILE" --region "$AWS_REGION" s3 cp \
  "s3://$ARTIFACT_BUCKET/$ARTIFACT_PREFIX/worlds.json" - --only-show-errors \
  | jq -e --arg world "$world" \
      '.worlds[] | select(.id == $world and .enabled == false)' >/dev/null || {
    echo "the staged manifest must disable $world before GPU deployment" >&2
    exit 1
  }

source_instance="$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" \
  cloudformation describe-stacks --stack-name "$BIBITES_SOURCE_STACK_NAME" \
  --query 'Stacks[0].Outputs[?OutputKey==`InstanceId`].OutputValue' --output text)"

printf -v source_check \
  '! systemctl is-active --quiet bibites-game@%q.service; ! systemctl is-active --quiet bibites-sidecar@%q.service; test "$(systemctl is-enabled bibites-game@%q.service 2>/dev/null || true)" = disabled; test "$(systemctl is-enabled bibites-sidecar@%q.service 2>/dev/null || true)" = disabled' \
  "$world" "$world" "$world" "$world"
parameters="$(jq -nc --arg command "$source_check" '{commands:[$command]}')"

command_id="$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ssm send-command \
  --instance-ids "$source_instance" --document-name AWS-RunShellScript \
  --parameters "$parameters" --query Command.CommandId --output text)"
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
    InstanceType="$BIBITES_BROADCAST_INSTANCE_TYPE" \
    AvailabilityZone="$BIBITES_AVAILABILITY_ZONE" \
    SubnetId="$BIBITES_SUBNET_ID" \
    VpcId="$BIBITES_VPC_ID" \
    ArtifactBucket="$ARTIFACT_BUCKET" \
    RuntimeKey="$ARTIFACT_PREFIX/$RUNTIME_OBJECT" \
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
printf 'broadcast stack ready; instance=%s\n' "$instance"
