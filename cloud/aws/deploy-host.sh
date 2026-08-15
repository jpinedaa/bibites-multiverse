#!/usr/bin/env bash
# Deploy the generic headless-world stack from staged private artifacts.
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
dist="$repo/cloud/aws/dist"
template="$repo/cloud/aws/template.yaml"

[ -r "$dist/artifacts.env" ] || { echo 'run build-artifacts.sh first' >&2; exit 1; }
[ -r "$dist/staged.env" ] || { echo 'run stage-artifacts.sh first' >&2; exit 1; }
[ -r "$dist/worlds.json" ] || { echo 'stage-artifacts.sh did not stage a manifest' >&2; exit 1; }

# shellcheck source=/dev/null
. "$dist/artifacts.env"
# shellcheck source=/dev/null
. "$dist/staged.env"

: "${RUNTIME_OBJECT:?run stage-artifacts.sh again to create an immutable runtime object}"
: "${BIBITES_AWS_ACCOUNT_ID:?set the approved 12-digit AWS account identifier}"
: "${BIBITES_SUBNET_ID:?set the target subnet identifier}"
: "${BIBITES_VPC_ID:?set the target VPC identifier}"
: "${BIBITES_AVAILABILITY_ZONE:?set the target availability zone}"
: "${BIBITES_RELAY_PRIVATE_IP:?set the private relay address}"
: "${BIBITES_RELAY_DOMAIN:?set the relay certificate name}"
: "${BIBITES_CREDENTIAL_PARAMETER_PREFIX:?set the SSM credential parameter prefix}"
: "${BIBITES_INSTANCE_TYPE:?set the approved EC2 instance type}"

stack="${BIBITES_STACK_NAME:-bibites-cloud-worlds}"
instance_type="$BIBITES_INSTANCE_TYPE"
data_volume_gib="${BIBITES_DATA_VOLUME_GIB:-40}"
enable_peering="${BIBITES_ENABLE_LIGHTSAIL_PEERING:-0}"

account="$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" sts get-caller-identity \
  --query Account --output text)"
[ "$account" = "$BIBITES_AWS_ACCOUNT_ID" ] || {
  echo "refusing AWS account $account; expected $BIBITES_AWS_ACCOUNT_ID" >&2
  exit 1
}

missing=0
while IFS= read -r parameter; do
  case "$parameter" in
    "$BIBITES_CREDENTIAL_PARAMETER_PREFIX"/*) ;;
    *)
      echo "credential parameter is outside the approved prefix: $parameter" >&2
      missing=1
      continue
      ;;
  esac
  if ! aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ssm get-parameter \
    --name "$parameter" --query Parameter.Name --output text >/dev/null 2>&1; then
    echo "missing credential parameter: $parameter" >&2
    missing=1
  fi
done < <(jq -r '.worlds[].credentialParameter' "$dist/worlds.json")
[ "$missing" -eq 0 ] || exit 1

case "$enable_peering" in
  0) ;;
  1)
    if [ "$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" lightsail is-vpc-peered \
      --query isPeered --output text)" != True ]; then
      aws --profile "$AWS_PROFILE" --region "$AWS_REGION" lightsail peer-vpc >/dev/null
      echo 'enabled Lightsail-to-default-VPC peering'
    fi
    ;;
  *) echo 'BIBITES_ENABLE_LIGHTSAIL_PEERING must be 0 or 1' >&2; exit 1 ;;
esac

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
    RuntimeFile="$RUNTIME_OBJECT" \
    RuntimeSha256="$RUNTIME_SHA256" \
    GameFile="$GAME_FILE" \
    GameSha256="$GAME_SHA256" \
    BepInExFile="$BEPINEX_FILE" \
    BepInExSha256="$BEPINEX_SHA256" \
    ManifestFile=worlds.json \
    DataVolumeGiB="$data_volume_gib" \
    RelayPrivateIp="$BIBITES_RELAY_PRIVATE_IP" \
    RelayDomain="$BIBITES_RELAY_DOMAIN" \
    CredentialParameterPrefix="$BIBITES_CREDENTIAL_PARAMETER_PREFIX"

instance="$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" cloudformation describe-stacks \
  --stack-name "$stack" \
  --query 'Stacks[0].Outputs[?OutputKey==`InstanceId`].OutputValue' --output text)"
printf 'stack ready; instance=%s\n' "$instance"
printf 'session: aws --profile %s --region %s ssm start-session --target %s\n' \
  "$AWS_PROFILE" "$AWS_REGION" "$instance"
