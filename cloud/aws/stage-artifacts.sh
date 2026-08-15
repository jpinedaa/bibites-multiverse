#!/usr/bin/env bash
# Upload private inputs to a blocked, encrypted S3 artifact bucket.
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
dist="$repo/cloud/aws/dist"
profile="${AWS_PROFILE:-bibites-multiverse}"
region="${AWS_REGION:-us-east-1}"
prefix="${BIBITES_ARTIFACT_PREFIX:-cloud/v1}"

: "${BIBITES_AWS_ACCOUNT_ID:?set the approved 12-digit AWS account identifier}"
: "${BIBITES_MANIFEST_FILE:?set the protected world manifest path}"
: "${BIBITES_SAVE_DIR:?set the directory that contains private save archives}"

[ -r "$dist/artifacts.env" ] || { echo 'run build-artifacts.sh first' >&2; exit 1; }
[ -r "$BIBITES_MANIFEST_FILE" ] || { echo "cannot read $BIBITES_MANIFEST_FILE" >&2; exit 1; }
[ -d "$BIBITES_SAVE_DIR" ] || { echo "not a directory: $BIBITES_SAVE_DIR" >&2; exit 1; }

# shellcheck source=/dev/null
. "$dist/artifacts.env"

account="$(aws --profile "$profile" --region "$region" sts get-caller-identity \
  --query Account --output text)"
[ "$account" = "$BIBITES_AWS_ACCOUNT_ID" ] || {
  echo "refusing AWS account $account; expected $BIBITES_AWS_ACCOUNT_ID" >&2
  exit 1
}

bucket="${BIBITES_ARTIFACT_BUCKET:-bibites-multiverse-cloud-$account-$region}"

jq -e '
  .schema == 1 and
  (.worlds | type == "array") and
  (.worlds | length > 0) and
  (all(.worlds[];
    (.id | type == "string" and length > 0) and
    (.peerId | type == "string" and length > 0) and
    (.saveKey | type == "string" and length > 0) and
    (.credentialParameter | type == "string" and startswith("/"))))
' "$BIBITES_MANIFEST_FILE" >/dev/null

install -m 0600 "$BIBITES_MANIFEST_FILE" "$dist/worlds.json"

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

runtime_object="runtime/$RUNTIME_SHA256.tar.gz"
aws --profile "$profile" --region "$region" s3 cp "$dist/$RUNTIME_FILE" \
  "s3://$bucket/$prefix/$runtime_object" --only-show-errors

for file in "$GAME_FILE" "$BEPINEX_FILE"; do
  aws --profile "$profile" --region "$region" s3 cp "$dist/$file" \
    "s3://$bucket/$prefix/$file" --only-show-errors
done

aws --profile "$profile" --region "$region" s3 cp "$dist/worlds.json" \
  "s3://$bucket/$prefix/worlds.json" --only-show-errors

while IFS= read -r save_key; do
  case "$save_key" in
    /*|*../*|../*|*'/..') echo "unsafe manifest saveKey: $save_key" >&2; exit 1 ;;
  esac
  save="$BIBITES_SAVE_DIR/$(basename "$save_key")"
  [ -r "$save" ] || { echo "missing save for $save_key: $save" >&2; exit 1; }
  unzip -tqq "$save"
  aws --profile "$profile" --region "$region" s3 cp "$save" \
    "s3://$bucket/$prefix/$save_key" --only-show-errors
done < <(jq -r '[.worlds[].saveKey] | unique[]' "$dist/worlds.json")

cat > "$dist/staged.env" <<EOF
AWS_PROFILE='$profile'
AWS_REGION='$region'
ARTIFACT_BUCKET='$bucket'
ARTIFACT_PREFIX='$prefix'
RUNTIME_OBJECT='$runtime_object'
EOF

chmod 0600 "$dist/staged.env"
printf 'staged private artifacts at s3://%s/%s/\n' "$bucket" "$prefix"
