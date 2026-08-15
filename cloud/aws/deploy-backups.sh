#!/usr/bin/env bash
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
profile="${AWS_PROFILE:-bibites-multiverse}"
region="${AWS_REGION:-us-east-1}"
host_stack="${BIBITES_STACK_NAME:-bibites-cloud-worlds}"
backup_stack="${BIBITES_BACKUP_STACK_NAME:-bibites-cloud-worlds-backups}"
: "${BIBITES_AWS_ACCOUNT_ID:?set the approved 12-digit AWS account identifier}"

account="$(aws --profile "$profile" --region "$region" sts get-caller-identity \
  --query Account --output text)"
[ "$account" = "$BIBITES_AWS_ACCOUNT_ID" ] || {
  echo "refusing AWS account $account; expected $BIBITES_AWS_ACCOUNT_ID" >&2
  exit 1
}
volume="$(aws --profile "$profile" --region "$region" cloudformation describe-stacks \
  --stack-name "$host_stack" \
  --query 'Stacks[0].Outputs[?OutputKey==`DataVolumeId`].OutputValue' --output text)"
[ -n "$volume" ] || { echo "no data volume in stack $host_stack" >&2; exit 1; }

aws --profile "$profile" --region "$region" ec2 create-tags --resources "$volume" \
  --tags Key=BibitesBackup,Value=daily
aws --profile "$profile" --region "$region" cloudformation deploy \
  --stack-name "$backup_stack" --template-file "$repo/cloud/aws/backup-template.yaml" \
  --capabilities CAPABILITY_IAM
policy="$(aws --profile "$profile" --region "$region" cloudformation describe-stacks \
  --stack-name "$backup_stack" \
  --query 'Stacks[0].Outputs[?OutputKey==`LifecyclePolicyId`].OutputValue' --output text)"
aws --profile "$profile" --region "$region" dlm get-lifecycle-policy --policy-id "$policy" \
  --query 'Policy.{id:PolicyId,state:State,description:Description}' --output json
