#!/usr/bin/env bash
# Apply a newly staged manifest. Existing saves stay unchanged. New worlds start.
set -euo pipefail

profile="${AWS_PROFILE:-bibites-multiverse}"
region="${AWS_REGION:-us-east-1}"
stack="${BIBITES_STACK_NAME:-bibites-cloud-worlds}"
: "${BIBITES_AWS_ACCOUNT_ID:?set the approved 12-digit AWS account identifier}"
account="$(aws --profile "$profile" --region "$region" sts get-caller-identity \
  --query Account --output text)"
[ "$account" = "$BIBITES_AWS_ACCOUNT_ID" ] || {
  echo "refusing AWS account $account; expected $BIBITES_AWS_ACCOUNT_ID" >&2
  exit 1
}
instance="$(aws --profile "$profile" --region "$region" cloudformation describe-stacks \
  --stack-name "$stack" --query 'Stacks[0].Outputs[?OutputKey==`InstanceId`].OutputValue' --output text)"
command_id="$(aws --profile "$profile" --region "$region" ssm send-command \
  --instance-ids "$instance" --document-name AWS-RunShellScript \
  --parameters 'commands=["sudo /usr/local/libexec/bibites-sync-worlds"]' \
  --query Command.CommandId --output text)"
aws --profile "$profile" --region "$region" ssm wait command-executed \
  --command-id "$command_id" --instance-id "$instance" || true
aws --profile "$profile" --region "$region" ssm get-command-invocation \
  --command-id "$command_id" --instance-id "$instance" \
  --query '{status:Status,stdout:StandardOutputContent,stderr:StandardErrorContent}' --output json
status="$(aws --profile "$profile" --region "$region" ssm get-command-invocation \
  --command-id "$command_id" --instance-id "$instance" --query Status --output text)"
[ "$status" = Success ]
