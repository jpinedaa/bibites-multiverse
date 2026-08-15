#!/usr/bin/env bash
set -euo pipefail

profile="${AWS_PROFILE:-bibites-multiverse}"
region="${AWS_REGION:-us-east-1}"
stack="${BIBITES_STACK_NAME:-bibites-cloud-worlds}"
instance="$(aws --profile "$profile" --region "$region" cloudformation describe-stacks \
  --stack-name "$stack" --query 'Stacks[0].Outputs[?OutputKey==`InstanceId`].OutputValue' --output text)"

command_id="$(aws --profile "$profile" --region "$region" ssm send-command \
  --instance-ids "$instance" --document-name AWS-RunShellScript \
  --parameters 'commands=["cloud-init status --wait >/dev/null 2>&1 || true","sudo test -f /srv/bibites/.installed","sudo bibites-cloud-ready 720","sudo systemctl is-active bibites-spot-watch.service bibites-performance.timer"]' \
  --query Command.CommandId --output text)"
aws --profile "$profile" --region "$region" ssm wait command-executed \
  --command-id "$command_id" --instance-id "$instance" || true
aws --profile "$profile" --region "$region" ssm get-command-invocation \
  --command-id "$command_id" --instance-id "$instance" \
  --query '{status:Status,stdout:StandardOutputContent,stderr:StandardErrorContent}' --output json
status="$(aws --profile "$profile" --region "$region" ssm get-command-invocation \
  --command-id "$command_id" --instance-id "$instance" --query Status --output text)"
[ "$status" = Success ]
