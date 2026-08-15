#!/usr/bin/env bash
# Deploy the GPU broadcast host from a safe snapshot after the source world stops.
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
dist="$repo/cloud/aws/dist"
template="$repo/cloud/aws/broadcast-template.yaml"
profile="${AWS_PROFILE:-bibites-multiverse}"
region="${AWS_REGION:-us-east-1}"
stack="${BIBITES_BROADCAST_STACK_NAME:-bibites-live-broadcast}"
snapshot="${BIBITES_BROADCAST_SNAPSHOT_ID:-}"
origin_ssh="${BIBITES_STREAM_ORIGIN_SSH:-ubuntu@3.229.27.163}"

[ -n "$snapshot" ] || { echo 'set BIBITES_BROADCAST_SNAPSHOT_ID to a snapshot made after the source world stopped' >&2; exit 1; }
[ -r "$dist/artifacts.env" ] || { echo 'run build-artifacts.sh first' >&2; exit 1; }
[ -r "$dist/staged.env" ] || { echo 'run stage-artifacts.sh first' >&2; exit 1; }
# shellcheck source=/dev/null
. "$dist/artifacts.env"
# shellcheck source=/dev/null
. "$dist/staged.env"

account="$(aws --profile "$profile" sts get-caller-identity --query Account --output text)"
[ "$account" = 663615031964 ] || { echo "wrong AWS account: $account" >&2; exit 1; }
quota="$(aws --profile "$profile" --region "$region" service-quotas get-service-quota \
  --service-code ec2 --quota-code L-3819A6DF --query Quota.Value --output text)"
awk -v value="$quota" 'BEGIN { exit !(value >= 4) }' || {
  echo "Spot G-instance quota is $quota vCPUs; 4 vCPUs are required" >&2
  exit 1
}

state="$(aws --profile "$profile" --region "$region" ec2 describe-snapshots \
  --snapshot-ids "$snapshot" --query 'Snapshots[0].State' --output text)"
[ "$state" = completed ] || { echo "snapshot $snapshot is $state, not completed" >&2; exit 1; }

aws --profile "$profile" --region "$region" s3 cp \
  "s3://$ARTIFACT_BUCKET/$ARTIFACT_PREFIX/worlds.json" - --only-show-errors \
  | jq -e '.worlds[] | select(.id == "slot-1" and .enabled == false)' >/dev/null || {
    echo 'the staged manifest must disable slot-1 before GPU deployment' >&2
    exit 1
  }

source_instance="$(aws --profile "$profile" --region "$region" cloudformation describe-stacks \
  --stack-name bibites-cloud-worlds \
  --query 'Stacks[0].Outputs[?OutputKey==`InstanceId`].OutputValue' --output text)"
command_id="$(aws --profile "$profile" --region "$region" ssm send-command \
  --instance-ids "$source_instance" --document-name AWS-RunShellScript \
  --parameters 'commands=["! systemctl is-active --quiet bibites-game@slot-1.service","! systemctl is-active --quiet bibites-sidecar@slot-1.service","test $(systemctl is-enabled bibites-game@slot-1.service 2>/dev/null || true) = disabled","test $(systemctl is-enabled bibites-sidecar@slot-1.service 2>/dev/null || true) = disabled"]' \
  --query Command.CommandId --output text)"
aws --profile "$profile" --region "$region" ssm wait command-executed \
  --command-id "$command_id" --instance-id "$source_instance" || true
source_status="$(aws --profile "$profile" --region "$region" ssm get-command-invocation \
  --command-id "$command_id" --instance-id "$source_instance" --query Status --output text)"
[ "$source_status" = Success ] || {
  echo 'slot-1 is active or enabled on the source host; refusing duplicate deployment' >&2
  exit 1
}

publish_password="$(ssh "$origin_ssh" \
  "sudo sed -n 's/^MV_STREAM_PUBLISH_PASSWORD=//p' /etc/multiverse/stream-publish.env")"
[[ "$publish_password" =~ ^[0-9a-f]{64}$ ]] || { echo 'the origin publish password is malformed' >&2; exit 1; }
aws --profile "$profile" --region "$region" ssm put-parameter \
  --name /bibites-multiverse/broadcast/publish-password --type SecureString \
  --value "$publish_password" --overwrite >/dev/null

aws --profile "$profile" --region "$region" cloudformation deploy \
  --stack-name "$stack" --template-file "$template" \
  --capabilities CAPABILITY_NAMED_IAM \
  --parameter-overrides \
    InstanceType=g4dn.xlarge AvailabilityZone=us-east-1a \
    SubnetId=subnet-08275ddfc41ef5eb9 VpcId=vpc-0f500b6ca5088a0cd \
    ArtifactBucket="$ARTIFACT_BUCKET" \
    RuntimeKey="$ARTIFACT_PREFIX/$RUNTIME_OBJECT" RuntimeSha256="$RUNTIME_SHA256" \
    DataSnapshotId="$snapshot" RelayPrivateIp=172.26.12.110 \
    StreamRtmpAddress=172.26.12.110:1935 WorldId=slot-1

instance="$(aws --profile "$profile" --region "$region" cloudformation describe-stacks \
  --stack-name "$stack" --query 'Stacks[0].Outputs[?OutputKey==`InstanceId`].OutputValue' --output text)"
printf 'broadcast stack ready; instance=%s\n' "$instance"
