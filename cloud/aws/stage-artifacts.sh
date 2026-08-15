#!/usr/bin/env bash
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
dist="$repo/cloud/aws/dist"
imports="$repo/cloud/aws/imports"
profile="${AWS_PROFILE:-bibites-multiverse}"
region="${AWS_REGION:-us-east-1}"
prefix="${BIBITES_ARTIFACT_PREFIX:-cloud/v1}"
account="$(aws --profile "$profile" sts get-caller-identity --query Account --output text)"
bucket="${BIBITES_ARTIFACT_BUCKET:-bibites-multiverse-cloud-$account-$region}"

[ -r "$dist/artifacts.env" ] || { echo 'run build-artifacts.sh first' >&2; exit 1; }
[ -r "$dist/worlds.json" ] || { echo 'run make-manifest.sh first' >&2; exit 1; }
# shellcheck source=/dev/null
. "$dist/artifacts.env"

if ! aws --profile "$profile" --region "$region" s3api head-bucket --bucket "$bucket" 2>/dev/null; then
  aws --profile "$profile" --region "$region" s3api create-bucket --bucket "$bucket" >/dev/null
fi
aws --profile "$profile" --region "$region" s3api put-public-access-block --bucket "$bucket" \
  --public-access-block-configuration \
  BlockPublicAcls=true,IgnorePublicAcls=true,BlockPublicPolicy=true,RestrictPublicBuckets=true
aws --profile "$profile" --region "$region" s3api put-bucket-encryption --bucket "$bucket" \
  --server-side-encryption-configuration \
  'Rules=[{ApplyServerSideEncryptionByDefault={SSEAlgorithm=AES256},BucketKeyEnabled=true}]'

runtime_object="runtime/$RUNTIME_SHA256.tar.gz"
aws --profile "$profile" --region "$region" s3 cp "$dist/$RUNTIME_FILE" \
  "s3://$bucket/$prefix/$runtime_object" --only-show-errors
for file in "$GAME_FILE" "$BEPINEX_FILE" worlds.json; do
  aws --profile "$profile" --region "$region" s3 cp "$dist/$file" "s3://$bucket/$prefix/$file" \
    --only-show-errors
done
while IFS= read -r save; do
  aws --profile "$profile" --region "$region" s3 cp "$save" \
    "s3://$bucket/$prefix/imports/$(basename "$save")" --only-show-errors
done < <(find "$imports" -maxdepth 1 -type f -name 'M4-Slot*.zip' | sort)

cat > "$dist/staged.env" <<EOF
AWS_PROFILE='$profile'
AWS_REGION='$region'
ARTIFACT_BUCKET='$bucket'
ARTIFACT_PREFIX='$prefix'
RUNTIME_OBJECT='$runtime_object'
EOF
printf 'staged private artifacts at s3://%s/%s/\n' "$bucket" "$prefix"
