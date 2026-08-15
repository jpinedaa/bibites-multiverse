#!/usr/bin/env bash
# Install the staged, content-addressed runtime without replacing the EC2 host.
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
dist="$repo/cloud/aws/dist"
[ -r "$dist/artifacts.env" ] || { echo 'run build-artifacts.sh first' >&2; exit 1; }
[ -r "$dist/staged.env" ] || { echo 'run stage-artifacts.sh first' >&2; exit 1; }
# shellcheck source=/dev/null
. "$dist/artifacts.env"
# shellcheck source=/dev/null
. "$dist/staged.env"
: "${RUNTIME_OBJECT:?run stage-artifacts.sh again to create an immutable runtime object}"
: "${BIBITES_AWS_ACCOUNT_ID:?set the approved 12-digit AWS account identifier}"

account="$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" sts get-caller-identity \
  --query Account --output text)"
[ "$account" = "$BIBITES_AWS_ACCOUNT_ID" ] || {
  echo "refusing AWS account $account; expected $BIBITES_AWS_ACCOUNT_ID" >&2
  exit 1
}

stack="${BIBITES_STACK_NAME:-bibites-cloud-worlds}"
description="$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" cloudformation \
  describe-stacks --stack-name "$stack" --output json)"
instance="$(jq -r '.Stacks[0].Outputs[] | select(.OutputKey == "InstanceId") | .OutputValue' \
  <<<"$description")"
volume="$(jq -r '.Stacks[0].Outputs[] | select(.OutputKey == "DataVolumeId") | .OutputValue' \
  <<<"$description")"
relay_private_ip="$(jq -r '.Stacks[0].Parameters[] | select(.ParameterKey == "RelayPrivateIp") | .ParameterValue' \
  <<<"$description")"
relay_domain="$(jq -r '.Stacks[0].Parameters[] | select(.ParameterKey == "RelayDomain") | .ParameterValue' \
  <<<"$description")"

remote="set -euo pipefail; rm -rf /opt/bibites-runtime.new; install -d -m 0755 /opt/bibites-runtime.new; aws --region $AWS_REGION s3 cp s3://$ARTIFACT_BUCKET/$ARTIFACT_PREFIX/$RUNTIME_OBJECT /tmp/bibites-runtime.tar.gz --only-show-errors; printf '$RUNTIME_SHA256  /tmp/bibites-runtime.tar.gz\\n' | sha256sum -c -; tar -xzf /tmp/bibites-runtime.tar.gz -C /opt/bibites-runtime.new; /opt/bibites-runtime.new/bibites-stop-worlds; rm -rf /opt/bibites-runtime; mv /opt/bibites-runtime.new /opt/bibites-runtime; env AWS_REGION=$AWS_REGION DATA_VOLUME_ID=$volume ARTIFACT_BUCKET=$ARTIFACT_BUCKET GAME_KEY=$ARTIFACT_PREFIX/$GAME_FILE GAME_SHA256=$GAME_SHA256 BEPINEX_KEY=$ARTIFACT_PREFIX/$BEPINEX_FILE BEPINEX_SHA256=$BEPINEX_SHA256 MANIFEST_KEY=$ARTIFACT_PREFIX/worlds.json RELAY_PRIVATE_IP=$relay_private_ip RELAY_DOMAIN=$relay_domain /opt/bibites-runtime/install-host"
encoded="$(printf %s "$remote" | base64 -w0)"
parameters="$(jq -nc --arg command "printf %s $encoded | base64 -d | bash" '{commands:[$command]}')"
command_id="$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ssm send-command \
  --instance-ids "$instance" --document-name AWS-RunShellScript --parameters "$parameters" \
  --comment 'Install staged Bibites cloud runtime' --query Command.CommandId --output text)"
aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ssm wait command-executed \
  --command-id "$command_id" --instance-id "$instance" || true
aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ssm get-command-invocation \
  --command-id "$command_id" --instance-id "$instance" \
  --query '{status:Status,stdout:StandardOutputContent,stderr:StandardErrorContent}' --output json
status="$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ssm get-command-invocation \
  --command-id "$command_id" --instance-id "$instance" --query Status --output text)"
[ "$status" = Success ]
