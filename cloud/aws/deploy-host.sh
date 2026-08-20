#!/usr/bin/env bash
# Deploy the generic headless-world stack from staged private artifacts.
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
dist="$repo/cloud/aws/dist"
template="$repo/cloud/aws/template.yaml"
validation="$repo/cloud/aws/lib/validation.sh"
manifest_validator="$repo/cloud/aws/runtime/validate-world-manifest.jq"

[ -r "$dist/artifacts.env" ] || { echo 'run build-artifacts.sh first' >&2; exit 1; }
[ -r "$dist/staged.env" ] || { echo 'run stage-artifacts.sh first' >&2; exit 1; }
[ -r "$dist/worlds.json" ] || { echo 'stage-artifacts.sh did not stage a manifest' >&2; exit 1; }

# shellcheck source=lib/validation.sh
. "$validation"
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

bibites_require_account_id "$BIBITES_AWS_ACCOUNT_ID" BIBITES_AWS_ACCOUNT_ID
bibites_require_region "$AWS_REGION" AWS_REGION
bibites_require_stack_name "$stack" BIBITES_STACK_NAME
bibites_require_instance_type "$instance_type" BIBITES_INSTANCE_TYPE
bibites_require_positive_integer "$data_volume_gib" BIBITES_DATA_VOLUME_GIB 40 16384
bibites_require_resource_id "$BIBITES_SUBNET_ID" BIBITES_SUBNET_ID subnet
bibites_require_resource_id "$BIBITES_VPC_ID" BIBITES_VPC_ID vpc
bibites_require_availability_zone "$BIBITES_AVAILABILITY_ZONE" BIBITES_AVAILABILITY_ZONE
bibites_require_rfc1918_ipv4 "$BIBITES_RELAY_PRIVATE_IP" BIBITES_RELAY_PRIVATE_IP
bibites_require_hostname "$BIBITES_RELAY_DOMAIN" BIBITES_RELAY_DOMAIN
bibites_require_ssm_parameter "$BIBITES_CREDENTIAL_PARAMETER_PREFIX" \
  BIBITES_CREDENTIAL_PARAMETER_PREFIX
bibites_require_s3_bucket "$ARTIFACT_BUCKET" ARTIFACT_BUCKET
bibites_require_s3_prefix "$ARTIFACT_PREFIX" ARTIFACT_PREFIX
bibites_require_s3_key "$RUNTIME_OBJECT" RUNTIME_OBJECT
bibites_require_s3_filename "$GAME_FILE" GAME_FILE
bibites_require_s3_filename "$BEPINEX_FILE" BEPINEX_FILE
bibites_require_sha256 "$RUNTIME_SHA256" RUNTIME_SHA256
bibites_require_sha256 "$GAME_SHA256" GAME_SHA256
bibites_require_sha256 "$BEPINEX_SHA256" BEPINEX_SHA256
for key in \
  "$ARTIFACT_PREFIX/$RUNTIME_OBJECT" \
  "$ARTIFACT_PREFIX/$GAME_FILE" \
  "$ARTIFACT_PREFIX/$BEPINEX_FILE" \
  "$ARTIFACT_PREFIX/worlds.json"; do
  bibites_require_s3_key "$key" "deployment object key $key"
done
jq -e -f "$manifest_validator" "$dist/worlds.json" >/dev/null

case "$enable_peering" in
  0|1) ;;
  *) echo 'BIBITES_ENABLE_LIGHTSAIL_PEERING must be 0 or 1' >&2; exit 1 ;;
esac

account="$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" sts get-caller-identity \
  --query Account --output text)"
bibites_require_account_id "$account" 'authenticated AWS account'
[ "$account" = "$BIBITES_AWS_ACCOUNT_ID" ] || {
  echo "refusing AWS account $account; expected $BIBITES_AWS_ACCOUNT_ID" >&2
  exit 1
}

pointer_file=runtime/current.json
bibites_require_s3_key "$pointer_file" 'runtime pointer object'
stack_error="$(mktemp)"
pointer_archive="$(mktemp)"
trap 'rm -f "$stack_error" "$pointer_archive"' EXIT
if stack_description="$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" cloudformation \
  describe-stacks --stack-name "$stack" --output json 2>"$stack_error")"; then
  stack_exists=1
elif grep -Fq 'does not exist' "$stack_error"; then
  stack_exists=0
  stack_description=''
else
  sed 's/^/CloudFormation stack lookup failed: /' "$stack_error" >&2
  exit 1
fi

if pointer_document="$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" s3 cp \
  "s3://$ARTIFACT_BUCKET/$ARTIFACT_PREFIX/$pointer_file" - --only-show-errors \
  2>"$stack_error")"; then
  bibites_require_runtime_pointer_document "$pointer_document"
  pointer_missing=0
elif grep -Eq '(404|NoSuchKey|Not Found)' "$stack_error"; then
  pointer_missing=1
else
  sed 's/^/Runtime pointer lookup failed: /' "$stack_error" >&2
  exit 1
fi

if [ "$pointer_missing" -eq 1 ]; then
  if [ "$stack_exists" -eq 1 ]; then
    previous_artifact_bucket="$(jq -er '
      [.Stacks[0].Parameters[] | select(.ParameterKey == "ArtifactBucket") | .ParameterValue] |
      if length == 1 then .[0] else error("missing legacy ArtifactBucket") end
    ' <<<"$stack_description")"
    previous_artifact_prefix="$(jq -er '
      [.Stacks[0].Parameters[] | select(.ParameterKey == "ArtifactPrefix") | .ParameterValue] |
      if length == 1 then .[0] else error("missing legacy ArtifactPrefix") end
    ' <<<"$stack_description")"
    [ "$previous_artifact_bucket" = "$ARTIFACT_BUCKET" ] &&
      [ "$previous_artifact_prefix" = "$ARTIFACT_PREFIX" ] || {
      echo 'legacy stack artifacts differ from the staged target; migrate them explicitly' >&2
      exit 1
    }
    previous_runtime_file="$(jq -er '
      [.Stacks[0].Parameters[] | select(.ParameterKey == "RuntimeFile") | .ParameterValue] |
      if length == 1 then .[0] else error("missing legacy RuntimeFile") end
    ' <<<"$stack_description")"
    previous_runtime_sha256="$(jq -er '
      [.Stacks[0].Parameters[] | select(.ParameterKey == "RuntimeSha256") | .ParameterValue] |
      if length == 1 then .[0] else error("missing legacy RuntimeSha256") end
    ' <<<"$stack_description")"
  else
    previous_runtime_file="$RUNTIME_OBJECT"
    previous_runtime_sha256="$RUNTIME_SHA256"
  fi
fi

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
  if ! bibites_require_default_secure_parameter \
    "$AWS_PROFILE" "$AWS_REGION" "$parameter"; then
    echo "invalid credential parameter: $parameter" >&2
    missing=1
  fi
done < <(jq -r '.worlds[].credentialParameter' "$dist/worlds.json")
[ "$missing" -eq 0 ] || exit 1

if [ "$pointer_missing" -eq 1 ]; then
  "$repo/cloud/aws/promote-runtime.sh" \
    "$previous_runtime_file" "$previous_runtime_sha256"
  pointer_document="$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" s3 cp \
    "s3://$ARTIFACT_BUCKET/$ARTIFACT_PREFIX/$pointer_file" - --only-show-errors)"
  bibites_require_runtime_pointer_document "$pointer_document"
fi

pointer_runtime_file="$(jq -r .runtimeFile <<<"$pointer_document")"
pointer_runtime_sha256="$(jq -r .runtimeSha256 <<<"$pointer_document")"
aws --profile "$AWS_PROFILE" --region "$AWS_REGION" s3 cp \
  "s3://$ARTIFACT_BUCKET/$ARTIFACT_PREFIX/$pointer_runtime_file" \
  "$pointer_archive" --only-show-errors
printf '%s  %s\n' "$pointer_runtime_sha256" "$pointer_archive" | \
  sha256sum -c - >/dev/null

case "$enable_peering" in
  0) ;;
  1)
    peered="$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" lightsail is-vpc-peered \
      --query isPeered --output text)"
    case "$peered" in
      True) ;;
      False)
        aws --profile "$AWS_PROFILE" --region "$AWS_REGION" lightsail peer-vpc >/dev/null
        echo 'enabled Lightsail-to-default-VPC peering'
        ;;
      *) echo "Lightsail returned an invalid peering state: $peered" >&2; exit 1 ;;
    esac
    ;;
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
    RuntimeManifestFile="$pointer_file" \
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
bibites_require_resource_id "$instance" 'deployed InstanceId output' i
printf 'stack ready; instance=%s\n' "$instance"
printf 'session: aws --profile %s --region %s ssm start-session --target %s\n' \
  "$AWS_PROFILE" "$AWS_REGION" "$instance"
