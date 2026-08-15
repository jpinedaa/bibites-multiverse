#!/usr/bin/env bash
# Upload private inputs to a blocked, encrypted S3 artifact bucket.
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
dist="$repo/cloud/aws/dist"
validation="$repo/cloud/aws/lib/validation.sh"
manifest_validator="$repo/cloud/aws/runtime/validate-world-manifest.jq"
profile="${AWS_PROFILE:-bibites-multiverse}"
region="${AWS_REGION:-us-east-1}"
prefix="${BIBITES_ARTIFACT_PREFIX:-cloud/v1}"

: "${BIBITES_AWS_ACCOUNT_ID:?set the approved 12-digit AWS account identifier}"
: "${BIBITES_MANIFEST_FILE:?set the protected world manifest path}"
: "${BIBITES_SAVE_DIR:?set the directory that contains private save archives}"

[ -r "$dist/artifacts.env" ] || { echo 'run build-artifacts.sh first' >&2; exit 1; }
[ -r "$BIBITES_MANIFEST_FILE" ] || { echo "cannot read $BIBITES_MANIFEST_FILE" >&2; exit 1; }
[ -d "$BIBITES_SAVE_DIR" ] || { echo "not a directory: $BIBITES_SAVE_DIR" >&2; exit 1; }

# shellcheck source=lib/validation.sh
. "$validation"
# shellcheck source=/dev/null
. "$dist/artifacts.env"

bibites_require_account_id "$BIBITES_AWS_ACCOUNT_ID" BIBITES_AWS_ACCOUNT_ID
bibites_require_region "$region" AWS_REGION
bibites_require_s3_prefix "$prefix" BIBITES_ARTIFACT_PREFIX
bibites_require_s3_filename "$GAME_FILE" GAME_FILE
bibites_require_sha256 "$GAME_SHA256" GAME_SHA256
bibites_require_s3_filename "$BEPINEX_FILE" BEPINEX_FILE
bibites_require_sha256 "$BEPINEX_SHA256" BEPINEX_SHA256
bibites_require_s3_filename "$RUNTIME_FILE" RUNTIME_FILE
bibites_require_sha256 "$RUNTIME_SHA256" RUNTIME_SHA256
jq -e -f "$manifest_validator" "$BIBITES_MANIFEST_FILE" >/dev/null

install -m 0600 "$BIBITES_MANIFEST_FILE" "$dist/worlds.json"

runtime_object="runtime/$RUNTIME_SHA256.tar.gz"
for key in "$runtime_object" "$GAME_FILE" "$BEPINEX_FILE" worlds.json; do
  bibites_require_s3_key "$prefix/$key" "staged object key $key"
done

for artifact in \
  "$RUNTIME_FILE:$RUNTIME_SHA256" \
  "$GAME_FILE:$GAME_SHA256" \
  "$BEPINEX_FILE:$BEPINEX_SHA256"; do
  file="${artifact%%:*}"
  digest="${artifact##*:}"
  [ -r "$dist/$file" ] || { echo "missing staged artifact: $dist/$file" >&2; exit 1; }
  printf '%s  %s\n' "$digest" "$dist/$file" | sha256sum -c - >/dev/null
done

# Check every save before the script creates a bucket or uploads an object.
while IFS= read -r save_key; do
  bibites_require_s3_key "$prefix/$save_key" "manifest saveKey $save_key"
  save="$BIBITES_SAVE_DIR/$(basename "$save_key")"
  [ -r "$save" ] || { echo "missing save for $save_key: $save" >&2; exit 1; }
  unzip -tqq "$save"
done < <(jq -r '[.worlds[].saveKey] | unique[]' "$dist/worlds.json")

account="$(aws --profile "$profile" --region "$region" sts get-caller-identity \
  --query Account --output text)"
bibites_require_account_id "$account" 'authenticated AWS account'
[ "$account" = "$BIBITES_AWS_ACCOUNT_ID" ] || {
  echo "refusing AWS account $account; expected $BIBITES_AWS_ACCOUNT_ID" >&2
  exit 1
}

bucket="${BIBITES_ARTIFACT_BUCKET:-bibites-multiverse-cloud-$account-$region}"
bibites_require_s3_bucket "$bucket" BIBITES_ARTIFACT_BUCKET

if ! aws --profile "$profile" --region "$region" s3api head-bucket \
  --bucket "$bucket" 2>/dev/null; then
  if [ "$region" = us-east-1 ]; then
    aws --profile "$profile" --region "$region" s3api create-bucket \
      --bucket "$bucket" >/dev/null
  else
    aws --profile "$profile" --region "$region" s3api create-bucket \
      --bucket "$bucket" \
      --create-bucket-configuration "LocationConstraint=$region" >/dev/null
  fi
fi

aws --profile "$profile" --region "$region" s3api put-public-access-block \
  --bucket "$bucket" --public-access-block-configuration \
  BlockPublicAcls=true,IgnorePublicAcls=true,BlockPublicPolicy=true,RestrictPublicBuckets=true

aws --profile "$profile" --region "$region" s3api put-bucket-encryption \
  --bucket "$bucket" --server-side-encryption-configuration \
  'Rules=[{ApplyServerSideEncryptionByDefault={SSEAlgorithm=AES256},BucketKeyEnabled=true}]'

aws --profile "$profile" --region "$region" s3 cp "$dist/$RUNTIME_FILE" \
  "s3://$bucket/$prefix/$runtime_object" --only-show-errors

for file in "$GAME_FILE" "$BEPINEX_FILE"; do
  aws --profile "$profile" --region "$region" s3 cp "$dist/$file" \
    "s3://$bucket/$prefix/$file" --only-show-errors
done

aws --profile "$profile" --region "$region" s3 cp "$dist/worlds.json" \
  "s3://$bucket/$prefix/worlds.json" --only-show-errors

while IFS= read -r save_key; do
  save="$BIBITES_SAVE_DIR/$(basename "$save_key")"
  aws --profile "$profile" --region "$region" s3 cp "$save" \
    "s3://$bucket/$prefix/$save_key" --only-show-errors
done < <(jq -r '[.worlds[].saveKey] | unique[]' "$dist/worlds.json")

{
  printf 'AWS_PROFILE=%q\n' "$profile"
  printf 'AWS_REGION=%q\n' "$region"
  printf 'ARTIFACT_BUCKET=%q\n' "$bucket"
  printf 'ARTIFACT_PREFIX=%q\n' "$prefix"
  printf 'RUNTIME_OBJECT=%q\n' "$runtime_object"
} > "$dist/staged.env"

chmod 0600 "$dist/staged.env"
printf 'staged private artifacts at s3://%s/%s/\n' "$bucket" "$prefix"
