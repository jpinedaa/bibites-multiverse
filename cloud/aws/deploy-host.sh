#!/usr/bin/env bash
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
dist="$repo/cloud/aws/dist"
private="$repo/cloud/aws/private"
template="$repo/cloud/aws/template.yaml"
[ -r "$dist/artifacts.env" ] || { echo 'run build-artifacts.sh first' >&2; exit 1; }
[ -r "$dist/staged.env" ] || { echo 'run stage-artifacts.sh first' >&2; exit 1; }
# shellcheck source=/dev/null
. "$dist/artifacts.env"
# shellcheck source=/dev/null
. "$dist/staged.env"
: "${RUNTIME_OBJECT:?run stage-artifacts.sh again to create an immutable runtime object}"

stack="${BIBITES_STACK_NAME:-bibites-cloud-worlds}"
subnet="${BIBITES_SUBNET_ID:-subnet-08275ddfc41ef5eb9}"
instance_type="${BIBITES_INSTANCE_TYPE:-m6a.2xlarge}"
relay_private_ip="${BIBITES_RELAY_PRIVATE_IP:-172.26.12.110}"

# Store only the secret half. The peer id and relay URL are non-secret manifest fields.
for n in 1 2 3 4 5 6; do
  join="$private/slot-$n.join"
  [ -f "$join" ] || continue
  read -r scheme relay identity < "$join"
  [ "$scheme" = multiverse-join/1 ] || { echo "$join: bad join scheme" >&2; exit 1; }
  peer="${identity%.*}"
  secret="${identity##*.}"
  [ "$peer" = "slot-$n" ] || { echo "$join: peer is $peer, want slot-$n" >&2; exit 1; }
  [[ "$relay" = wss://bibitesmultiverse.com/* ]] || { echo "$join: unexpected relay $relay" >&2; exit 1; }
  [[ "$secret" =~ ^[0-9a-fA-F]{64}$ ]] || { echo "$join: malformed secret" >&2; exit 1; }
  aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ssm put-parameter \
    --name "/bibites-multiverse/cloud/slot-$n/peer-secret" --type SecureString \
    --value "$secret" --overwrite >/dev/null
  printf 'stored slot-%s credential in SSM Parameter Store\n' "$n"
done

missing=0
while IFS= read -r parameter; do
  if ! aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ssm get-parameter \
    --name "$parameter" --query Parameter.Name --output text >/dev/null 2>&1; then
    echo "missing credential parameter: $parameter" >&2
    missing=1
  fi
done < <(jq -r '.worlds[].credentialParameter' "$dist/worlds.json")
[ "$missing" -eq 0 ] || exit 1

if [ "$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" lightsail is-vpc-peered \
  --query isPeered --output text)" != True ]; then
  aws --profile "$AWS_PROFILE" --region "$AWS_REGION" lightsail peer-vpc >/dev/null
  echo 'enabled free Lightsail-to-default-VPC peering'
fi

aws --profile "$AWS_PROFILE" --region "$AWS_REGION" cloudformation deploy \
  --stack-name "$stack" --template-file "$template" \
  --capabilities CAPABILITY_NAMED_IAM \
  --parameter-overrides \
    InstanceType="$instance_type" AvailabilityZone=us-east-1a SubnetId="$subnet" \
    VpcId=vpc-0f500b6ca5088a0cd \
    ArtifactBucket="$ARTIFACT_BUCKET" ArtifactPrefix="$ARTIFACT_PREFIX" \
    RuntimeFile="$RUNTIME_OBJECT" RuntimeSha256="$RUNTIME_SHA256" \
    GameFile="$GAME_FILE" GameSha256="$GAME_SHA256" \
    BepInExFile="$BEPINEX_FILE" BepInExSha256="$BEPINEX_SHA256" \
    ManifestFile=worlds.json DataVolumeGiB=80 RelayPrivateIp="$relay_private_ip"

instance="$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" cloudformation describe-stacks \
  --stack-name "$stack" --query 'Stacks[0].Outputs[?OutputKey==`InstanceId`].OutputValue' --output text)"
printf 'stack ready; instance=%s\n' "$instance"
printf 'session: aws --profile %s --region %s ssm start-session --target %s\n' \
  "$AWS_PROFILE" "$AWS_REGION" "$instance"
