#!/usr/bin/env bash
# Preview or execute a fail-closed headless-host stack change.
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
dist="$repo/cloud/aws/dist"
template="$repo/cloud/aws/template.yaml"
validation="$repo/cloud/aws/lib/validation.sh"
host_change="$repo/cloud/aws/lib/host-change.sh"
manifest_validator="$repo/cloud/aws/runtime/validate-world-manifest.jq"

execute=0
change_set_name="${BIBITES_CHANGE_SET_NAME:-}"
while [ "$#" -gt 0 ]; do
  case "$1" in
    --change-set-name)
      [ "$#" -ge 2 ] || { echo 'missing change-set name' >&2; exit 2; }
      change_set_name="$2"
      shift 2
      ;;
    --execute)
      execute=1
      shift
      ;;
    *)
      echo 'usage: deploy-host.sh --change-set-name NAME [--execute]' >&2
      exit 2
      ;;
  esac
done
[ -n "$change_set_name" ] || {
  echo 'set --change-set-name or BIBITES_CHANGE_SET_NAME for every preview and execution' >&2
  exit 2
}

[ -r "$dist/artifacts.env" ] || { echo 'run build-artifacts.sh first' >&2; exit 1; }
[ -r "$dist/staged.env" ] || { echo 'run stage-artifacts.sh first' >&2; exit 1; }
[ -r "$dist/worlds.json" ] || { echo 'stage-artifacts.sh did not stage a manifest' >&2; exit 1; }

# shellcheck source=lib/validation.sh
. "$validation"
# shellcheck source=lib/host-change.sh
. "$host_change"
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
  echo 'deploy-host.sh requires STAGING_SCOPE=complete from stage-artifacts.sh' >&2
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
bibites_require_change_set_name "$change_set_name" change-set
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
  "$ARTIFACT_PREFIX/$MANIFEST_OBJECT"; do
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
pointer_key="$ARTIFACT_PREFIX/$pointer_file"
bibites_require_s3_key "$pointer_key" 'runtime pointer key'
lookup_error="$(mktemp)"
pointer_archive="$(mktemp)"
pointer_path="$(mktemp)"
current_pointer_path="$(mktemp)"
trap 'rm -f "$lookup_error" "$pointer_archive" "$pointer_path" "$current_pointer_path"' EXIT
if stack_description="$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" cloudformation \
  describe-stacks --stack-name "$stack" --output json 2>"$lookup_error")"; then
  stack_status="$(jq -er '.Stacks[0].StackStatus' <<<"$stack_description")"
  if [ "$stack_status" = REVIEW_IN_PROGRESS ]; then
    stack_exists=0
  else
    stack_exists=1
    case "$stack_status" in
      CREATE_COMPLETE|UPDATE_COMPLETE|UPDATE_ROLLBACK_COMPLETE) ;;
      *) echo "stack $stack is not in a deployable state: $stack_status" >&2; exit 1 ;;
    esac
  fi
elif grep -Fq 'does not exist' "$lookup_error"; then
  stack_exists=0
  stack_description=''
  stack_status=''
else
  sed 's/^/CloudFormation stack lookup failed: /' "$lookup_error" >&2
  exit 1
fi

fetch_pointer_snapshot() {
  local destination="$1" metadata
  : >"$lookup_error"
  : >"$destination"
  metadata="$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" s3api get-object \
    --bucket "$ARTIFACT_BUCKET" --key "$pointer_key" "$destination" \
    --output json 2>"$lookup_error")" || return 1
  pointer_snapshot_etag="$(jq -er '
    .ETag |
    select(type == "string" and
      test("^\\\"[0-9A-Fa-f]{32}(-[0-9]+)?\\\"$"))
  ' <<<"$metadata")" || {
    echo 'runtime pointer lookup returned an invalid ETag' >&2
    return 2
  }
  pointer_snapshot_document="$(<"$destination")"
  bibites_require_runtime_pointer_document "$pointer_snapshot_document" || return 2
  pointer_snapshot_canonical="$(jq -cS . <<<"$pointer_snapshot_document")" || return 2
}

if fetch_pointer_snapshot "$pointer_path"; then
  pointer_document="$pointer_snapshot_document"
  prior_pointer_etag="$pointer_snapshot_etag"
  prior_pointer_canonical="$pointer_snapshot_canonical"
  pointer_missing=0
else
  pointer_status=$?
  if [ "$pointer_status" -eq 1 ] &&
     grep -Eq '(404|NoSuchKey|Not Found)' "$lookup_error"; then
    pointer_missing=1
  elif [ "$pointer_status" -eq 1 ]; then
    sed 's/^/Runtime pointer lookup failed: /' "$lookup_error" >&2
    exit 1
  else
    exit 1
  fi
fi

if [ "$pointer_missing" -eq 0 ]; then
  runtime_object="$(jq -r .runtimeFile <<<"$pointer_document")"
  runtime_sha256="$(jq -r .runtimeSha256 <<<"$pointer_document")"
elif [ "$stack_exists" -eq 1 ]; then
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
  runtime_object="$(jq -er '
    [.Stacks[0].Parameters[] |
      select(.ParameterKey == "RuntimeObject" or .ParameterKey == "RuntimeFile") |
      .ParameterValue] |
    if length == 1 then .[0]
    else error("missing or ambiguous checked runtime object") end
  ' <<<"$stack_description")"
  runtime_sha256="$(jq -er '
    [.Stacks[0].Parameters[] | select(.ParameterKey == "RuntimeSha256") | .ParameterValue] |
    if length == 1 then .[0] else error("missing legacy RuntimeSha256") end
  ' <<<"$stack_description")"
else
  runtime_object="$RUNTIME_OBJECT"
  runtime_sha256="$RUNTIME_SHA256"
fi
bibites_require_s3_key "$runtime_object" 'bootstrap runtime object'
bibites_require_sha256 "$runtime_sha256" 'bootstrap runtime digest'
if [ "$pointer_missing" -eq 1 ] && [ "$stack_exists" -eq 1 ] &&
   [ "$runtime_object" != "runtime/$runtime_sha256.tar.gz" ]; then
  echo 'the legacy stack runtime is not a matching content-addressed object' >&2
  echo 'migrate the legacy runtime explicitly before creating the runtime pointer' >&2
  exit 1
fi

use_legacy_attachment=false
host_launch_template_version=1
if [ "$stack_exists" -eq 1 ]; then
  stack_resources="$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" cloudformation \
    list-stack-resources --stack-name "$stack" --output json)"
  use_legacy_attachment="$(bibites_legacy_attachment_mode "$stack_resources")"
  host_id="$(jq -er '[.StackResourceSummaries[] | select(
    .LogicalResourceId == "Host" and .ResourceType == "AWS::EC2::Instance") |
    .PhysicalResourceId] | if length == 1 then .[0] else error("Host") end' \
    <<<"$stack_resources")"
  bibites_require_resource_id "$host_id" 'stack Host resource' i
  live_host="$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ec2 describe-instances \
    --instance-ids "$host_id" --output json)"
  IFS=$'\t' read -r bound_host_id launch_template_id host_launch_template_version \
    <<<"$(bibites_live_host_launch_template_binding "$stack_resources" "$live_host")"
  [ "$bound_host_id" = "$host_id" ]
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

type_description="$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ec2 \
  describe-instance-types --instance-types "$instance_type" --output json)"
bibites_require_x86_64_instance_description "$type_description" "$instance_type"

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

aws --profile "$AWS_PROFILE" --region "$AWS_REGION" s3 cp \
  "s3://$ARTIFACT_BUCKET/$ARTIFACT_PREFIX/$runtime_object" \
  "$pointer_archive" --only-show-errors
printf '%s  %s\n' "$runtime_sha256" "$pointer_archive" | sha256sum -c - >/dev/null

parameters=(
  "ParameterKey=InstanceType,ParameterValue=$instance_type"
  "ParameterKey=AvailabilityZone,ParameterValue=$BIBITES_AVAILABILITY_ZONE"
  "ParameterKey=SubnetId,ParameterValue=$BIBITES_SUBNET_ID"
  "ParameterKey=VpcId,ParameterValue=$BIBITES_VPC_ID"
  "ParameterKey=ArtifactBucket,ParameterValue=$ARTIFACT_BUCKET"
  "ParameterKey=ArtifactPrefix,ParameterValue=$ARTIFACT_PREFIX"
  "ParameterKey=RuntimeObject,ParameterValue=$runtime_object"
  "ParameterKey=RuntimeSha256,ParameterValue=$runtime_sha256"
  "ParameterKey=GameFile,ParameterValue=$GAME_FILE"
  "ParameterKey=GameSha256,ParameterValue=$GAME_SHA256"
  "ParameterKey=BepInExFile,ParameterValue=$BEPINEX_FILE"
  "ParameterKey=BepInExSha256,ParameterValue=$BEPINEX_SHA256"
  "ParameterKey=ManifestFile,ParameterValue=$MANIFEST_OBJECT"
  "ParameterKey=ManifestSha256,ParameterValue=$MANIFEST_SHA256"
  "ParameterKey=DataVolumeGiB,ParameterValue=$data_volume_gib"
  "ParameterKey=RelayPrivateIp,ParameterValue=$BIBITES_RELAY_PRIVATE_IP"
  "ParameterKey=RelayDomain,ParameterValue=$BIBITES_RELAY_DOMAIN"
  "ParameterKey=CredentialParameterPrefix,ParameterValue=$BIBITES_CREDENTIAL_PARAMETER_PREFIX"
  "ParameterKey=HostLaunchTemplateVersion,ParameterValue=$host_launch_template_version"
  "ParameterKey=UseLegacyDataAttachment,ParameterValue=$use_legacy_attachment"
)
if [ "$stack_exists" -eq 1 ]; then
  parameters+=("ParameterKey=UbuntuAmi,UsePreviousValue=true")
fi
change_set_type="$(bibites_change_set_type_for_stack_status "$stack_status")"

if [ "$execute" -eq 0 ]; then
  aws --profile "$AWS_PROFILE" --region "$AWS_REGION" cloudformation create-change-set \
    --stack-name "$stack" --change-set-name "$change_set_name" \
    --change-set-type "$change_set_type" --template-body "file://$template" \
    --description 'Preview fail-closed Bibites host-stack change' \
    --capabilities CAPABILITY_NAMED_IAM --parameters "${parameters[@]}" >/dev/null
  set +e
  change_set_description="$(bibites_wait_change_set "$AWS_PROFILE" "$AWS_REGION" \
    "$stack" "$change_set_name")"
  change_set_status=$?
  set -e
  [ -z "$change_set_description" ] || bibites_change_set_summary "$change_set_description"
  if [ "$change_set_status" -ne 0 ]; then
    reason="$(jq -r '.StatusReason // "unknown change-set failure"' \
      <<<"${change_set_description:-{}}")"
    echo "change-set preview failed: $reason" >&2
    exit "$change_set_status"
  fi
else
  change_set_description="$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" \
    cloudformation describe-change-set --stack-name "$stack" \
    --change-set-name "$change_set_name" --include-property-values --output json)"
  jq -e --arg name "$change_set_name" --arg type "$change_set_type" '
    .ChangeSetName == $name and .ChangeSetType == $type and
    .Status == "CREATE_COMPLETE" and .ExecutionStatus == "AVAILABLE"
  ' <<<"$change_set_description" >/dev/null || {
    echo 'named change set is not the reviewed executable change set for this operation' >&2
    exit 1
  }
  bibites_change_set_summary "$change_set_description"
fi

require_change_parameter() {
  bibites_require_change_set_parameter "$change_set_description" "$1" "$2"
}
require_change_parameter InstanceType "$instance_type"
require_change_parameter AvailabilityZone "$BIBITES_AVAILABILITY_ZONE"
require_change_parameter SubnetId "$BIBITES_SUBNET_ID"
require_change_parameter VpcId "$BIBITES_VPC_ID"
require_change_parameter ArtifactBucket "$ARTIFACT_BUCKET"
require_change_parameter ArtifactPrefix "$ARTIFACT_PREFIX"
require_change_parameter RuntimeObject "$runtime_object"
require_change_parameter RuntimeSha256 "$runtime_sha256"
require_change_parameter GameFile "$GAME_FILE"
require_change_parameter GameSha256 "$GAME_SHA256"
require_change_parameter BepInExFile "$BEPINEX_FILE"
require_change_parameter BepInExSha256 "$BEPINEX_SHA256"
require_change_parameter ManifestFile "$MANIFEST_OBJECT"
require_change_parameter ManifestSha256 "$MANIFEST_SHA256"
require_change_parameter DataVolumeGiB "$data_volume_gib"
require_change_parameter RelayPrivateIp "$BIBITES_RELAY_PRIVATE_IP"
require_change_parameter RelayDomain "$BIBITES_RELAY_DOMAIN"
require_change_parameter CredentialParameterPrefix "$BIBITES_CREDENTIAL_PARAMETER_PREFIX"
require_change_parameter HostLaunchTemplateVersion "$host_launch_template_version"
require_change_parameter UseLegacyDataAttachment "$use_legacy_attachment"
bibites_require_safe_host_change_set "$change_set_description"

if [ "$execute" -eq 0 ]; then
  echo "safe change set $change_set_name is ready; no stack resource was changed"
  echo "after separate authorization, rerun with the same inputs and --change-set-name $change_set_name --execute"
  exit 0
fi

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

aws --profile "$AWS_PROFILE" --region "$AWS_REGION" cloudformation execute-change-set \
  --stack-name "$stack" --change-set-name "$change_set_name"
previous_stack_marker=''
if [ "$stack_exists" -eq 1 ]; then
  previous_stack_marker="$(jq -er '
    .Stacks[0].LastUpdatedTime // .Stacks[0].CreationTime // ""
  ' <<<"$stack_description")"
fi
set +e
terminal_description="$(bibites_wait_stack_terminal "$AWS_PROFILE" "$AWS_REGION" \
  "$stack" 3600 15 "$previous_stack_marker")"
terminal_status=$?
set -e
[ "$terminal_status" -eq 0 ] || {
  echo "stack $stack did not complete; the runtime pointer was not published" >&2
  exit "$terminal_status"
}

if [ "$pointer_missing" -eq 0 ]; then
  if fetch_pointer_snapshot "$current_pointer_path"; then
    current_pointer_etag="$pointer_snapshot_etag"
    if [ "$pointer_snapshot_canonical" != "$prior_pointer_canonical" ]; then
      echo 'The stack completed, but runtime/current.json changed during the stack operation.' >&2
      printf 'Runtime pointer ETags: before=%s after=%s\n' \
        "$prior_pointer_etag" "$current_pointer_etag" >&2
      echo 'Deployment status: partial. Reconcile runtime/current.json before another deployment.' >&2
      exit 1
    fi
  else
    pointer_status=$?
    echo 'The stack completed, but the runtime pointer could not be verified.' >&2
    if [ "$pointer_status" -eq 1 ] && [ -s "$lookup_error" ]; then
      sed 's/^/Runtime pointer lookup failed: /' "$lookup_error" >&2
    fi
    echo 'Deployment status: partial. Reconcile runtime/current.json before another deployment.' >&2
    exit 1
  fi
fi

instance="$(jq -er '
  [.Stacks[0].Outputs[] | select(.OutputKey == "InstanceId") | .OutputValue] |
  if length == 1 then .[0] else error("missing deployed InstanceId output") end
' <<<"$terminal_description")"
bibites_require_resource_id "$instance" 'deployed InstanceId output' i
if [ "$pointer_missing" -eq 1 ]; then
  AWS_PROFILE="$AWS_PROFILE" AWS_REGION="$AWS_REGION" \
    BIBITES_AWS_ACCOUNT_ID="$BIBITES_AWS_ACCOUNT_ID" \
    ARTIFACT_BUCKET="$ARTIFACT_BUCKET" ARTIFACT_PREFIX="$ARTIFACT_PREFIX" \
    "$repo/cloud/aws/promote-runtime.sh" --if-absent \
    "$runtime_object" "$runtime_sha256"
  pointer_document="$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" s3 cp \
    "s3://$ARTIFACT_BUCKET/$ARTIFACT_PREFIX/$pointer_file" - --only-show-errors)"
  bibites_require_runtime_pointer_document "$pointer_document"
  [ "$(jq -r .runtimeSha256 <<<"$pointer_document")" = "$runtime_sha256" ] || {
    echo 'published runtime pointer differs from the successful stack runtime' >&2
    exit 1
  }
fi
printf 'stack ready; instance=%s\n' "$instance"
printf 'session: aws --profile %s --region %s ssm start-session --target %s\n' \
  "$AWS_PROFILE" "$AWS_REGION" "$instance"
