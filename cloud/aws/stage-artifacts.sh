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

runtime_only=false
case "$#" in
  0) ;;
  1)
    [ "$1" = --runtime-only ] || {
      echo 'usage: stage-artifacts.sh [--runtime-only]' >&2
      exit 2
    }
    runtime_only=true
    ;;
  *)
    echo 'usage: stage-artifacts.sh [--runtime-only]' >&2
    exit 2
    ;;
esac
if [ "$runtime_only" = true ]; then
  staging_scope=runtime-only
else
  staging_scope=complete
fi

staged_settings="$dist/staged.env"
staged_settings_temp=
manifest_snapshot=
cleanup() {
  [ -z "$staged_settings_temp" ] || rm -f "$staged_settings_temp"
  [ -z "$manifest_snapshot" ] || rm -f "$manifest_snapshot"
}
trap cleanup EXIT
# A failed staging attempt must not leave an older candidate available to
# update-runtime.sh.
rm -f "$staged_settings"
: "${BIBITES_AWS_ACCOUNT_ID:?set the approved 12-digit AWS account identifier}"
[ -r "$dist/artifacts.env" ] || { echo 'run build-artifacts.sh first' >&2; exit 1; }

# shellcheck source=lib/validation.sh
. "$validation"
# Artifact identity must come from artifacts.env, not the caller environment.
unset RUNTIME_FILE RUNTIME_SHA256 GAME_FILE GAME_SHA256 \
  BEPINEX_FILE BEPINEX_SHA256
# shellcheck source=/dev/null
. "$dist/artifacts.env"
for artifact_setting in RUNTIME_FILE RUNTIME_SHA256 GAME_FILE GAME_SHA256 \
  BEPINEX_FILE BEPINEX_SHA256; do
  [ -n "${!artifact_setting:-}" ] || {
    echo "artifacts.env is missing $artifact_setting; run build-artifacts.sh again" >&2
    exit 1
  }
done

bibites_require_account_id "$BIBITES_AWS_ACCOUNT_ID" BIBITES_AWS_ACCOUNT_ID
bibites_require_region "$region" AWS_REGION
bibites_require_s3_prefix "$prefix" BIBITES_ARTIFACT_PREFIX
bibites_require_s3_filename "$RUNTIME_FILE" RUNTIME_FILE
bibites_require_sha256 "$RUNTIME_SHA256" RUNTIME_SHA256
bibites_require_s3_filename "$GAME_FILE" GAME_FILE
bibites_require_sha256 "$GAME_SHA256" GAME_SHA256
bibites_require_s3_filename "$BEPINEX_FILE" BEPINEX_FILE
bibites_require_sha256 "$BEPINEX_SHA256" BEPINEX_SHA256

runtime_object="runtime/$RUNTIME_SHA256.tar.gz"
game_object="runtime-inputs/game/$GAME_SHA256.zip"
bepinex_object="runtime-inputs/bepinex/$BEPINEX_SHA256.zip"
bibites_require_s3_key "$prefix/$runtime_object" 'staged runtime object key'
for artifact in \
  "$RUNTIME_FILE:$RUNTIME_SHA256" \
  "$GAME_FILE:$GAME_SHA256" \
  "$BEPINEX_FILE:$BEPINEX_SHA256"; do
  file="${artifact%%:*}"
  digest="${artifact##*:}"
  [ -r "$dist/$file" ] || {
    echo "missing staged artifact: $dist/$file" >&2
    exit 1
  }
  if ! printf '%s  %s\n' "$digest" "$dist/$file" | \
    sha256sum -c - >/dev/null 2>&1; then
    echo "staged artifact digest does not match: $dist/$file" >&2
    exit 1
  fi
done

if [ "$runtime_only" = false ]; then
  : "${BIBITES_MANIFEST_FILE:?set the protected world manifest path}"
  : "${BIBITES_SAVE_DIR:?set the directory that contains private save archives}"
  [ -r "$BIBITES_MANIFEST_FILE" ] || {
    echo "cannot read $BIBITES_MANIFEST_FILE" >&2
    exit 1
  }
  [ -d "$BIBITES_SAVE_DIR" ] || {
    echo "not a directory: $BIBITES_SAVE_DIR" >&2
    exit 1
  }
  for key in "$GAME_FILE" "$BEPINEX_FILE" worlds.json; do
    bibites_require_s3_key "$prefix/$key" "staged object key $key"
  done
  # Validate the same owner-only snapshot that this stage hashes and uploads.
  # A concurrent edit of the protected source cannot bypass this check.
  install -m 0600 "$BIBITES_MANIFEST_FILE" "$dist/worlds.json"
  jq -e -f "$manifest_validator" "$dist/worlds.json" >/dev/null
  manifest_sha256="$(sha256sum "$dist/worlds.json" | awk '{print $1}')"
  bibites_require_sha256 "$manifest_sha256" MANIFEST_SHA256
  manifest_object="worlds.$manifest_sha256.json"
  bibites_require_s3_filename "$manifest_object" MANIFEST_OBJECT
fi

# Check every save before the script creates a bucket or uploads an object.
if [ "$runtime_only" = false ]; then
  mapfile -t save_keys < <(jq -r '[.worlds[].saveKey] | unique[]' "$dist/worlds.json")
  bibites_require_unique_save_basenames "${save_keys[@]}"
  for save_key in "${save_keys[@]}"; do
    bibites_require_s3_key "$prefix/$save_key" "manifest saveKey $save_key"
    save_basename="$(basename "$save_key")"
    save="$BIBITES_SAVE_DIR/$save_basename"
    [ -r "$save" ] || { echo "missing save for $save_key: $save" >&2; exit 1; }
    bibites_require_save_archive "$save" "save for $save_key"
  done
fi

account="$(aws --profile "$profile" --region "$region" sts get-caller-identity \
  --query Account --output text)"
bibites_require_account_id "$account" 'authenticated AWS account'
[ "$account" = "$BIBITES_AWS_ACCOUNT_ID" ] || {
  echo "refusing AWS account $account; expected $BIBITES_AWS_ACCOUNT_ID" >&2
  exit 1
}

bucket="${BIBITES_ARTIFACT_BUCKET:-bibites-multiverse-cloud-$account-$region}"
bibites_require_s3_bucket "$bucket" BIBITES_ARTIFACT_BUCKET

write_staged_settings() {
  staged_settings_temp="$(mktemp "$dist/staged.env.XXXXXX")"
  {
    printf 'AWS_PROFILE=%q\n' "$profile"
    printf 'AWS_REGION=%q\n' "$region"
    printf 'ARTIFACT_BUCKET=%q\n' "$bucket"
    printf 'ARTIFACT_PREFIX=%q\n' "$prefix"
    printf 'RUNTIME_OBJECT=%q\n' "$runtime_object"
    if [ "$runtime_only" = true ]; then
      printf 'RUNTIME_SHA256=%q\n' "$RUNTIME_SHA256"
      printf 'GAME_OBJECT=%q\n' "$game_object"
      printf 'GAME_SHA256=%q\n' "$GAME_SHA256"
      printf 'BEPINEX_OBJECT=%q\n' "$bepinex_object"
      printf 'BEPINEX_SHA256=%q\n' "$BEPINEX_SHA256"
      printf 'MANIFEST_OBJECT=%q\n' "$manifest_object"
      printf 'MANIFEST_SHA256=%q\n' "$manifest_sha256"
    else
      # Keep these names separate from artifacts.env. Consumers must compare
      # the completed stage with the current build before they call AWS.
      printf 'STAGED_RUNTIME_SHA256=%q\n' "$RUNTIME_SHA256"
      printf 'STAGED_GAME_SHA256=%q\n' "$GAME_SHA256"
      printf 'STAGED_BEPINEX_SHA256=%q\n' "$BEPINEX_SHA256"
      printf 'MANIFEST_OBJECT=%q\n' "$manifest_object"
      printf 'MANIFEST_SHA256=%q\n' "$manifest_sha256"
    fi
    printf 'STAGING_SCOPE=%q\n' "$staging_scope"
  } >"$staged_settings_temp"
  chmod 0600 "$staged_settings_temp"
  mv -f "$staged_settings_temp" "$staged_settings"
  staged_settings_temp=
}

put_immutable_input() {
  local input_file="$1" input_sha256="$2" input_object="$3"
  AWS_PROFILE="$profile" AWS_REGION="$region" \
    BIBITES_AWS_ACCOUNT_ID="$BIBITES_AWS_ACCOUNT_ID" \
    "$repo/cloud/aws/put-immutable-object.sh" \
    "$input_file" "$input_sha256" "$bucket" "$prefix/$input_object"
}

if [ "$runtime_only" = true ]; then
  aws --profile "$profile" --region "$region" s3api head-bucket \
    --bucket "$bucket" >/dev/null

  # Snapshot the mutable deployment manifest before creating any immutable
  # input. Runtime activation can then use one coherent set of exact bytes.
  manifest_snapshot="$(mktemp)"
  chmod 0600 "$manifest_snapshot"
  if ! aws --profile "$profile" --region "$region" s3 cp \
    "s3://$bucket/$prefix/worlds.json" "$manifest_snapshot" \
    --only-show-errors; then
    echo 'cannot read the current staged world manifest' >&2
    exit 1
  fi
  chmod 0600 "$manifest_snapshot"
  if ! jq -e -f "$manifest_validator" "$manifest_snapshot" >/dev/null; then
    echo 'the current staged world manifest is invalid' >&2
    exit 1
  fi
  manifest_sha256="$(sha256sum "$manifest_snapshot" | awk '{print $1}')"
  bibites_require_sha256 "$manifest_sha256" MANIFEST_SHA256
  # Keep the snapshot beside worlds.json so relative saveKey values continue
  # to resolve under the existing artifact prefix.
  manifest_object="worlds.$manifest_sha256.json"
  for object in "$game_object" "$bepinex_object" "$manifest_object"; do
    bibites_require_s3_key "$prefix/$object" "immutable runtime input $object"
  done

  # These are the only remote writes in runtime-only mode. Each helper call is
  # an AES256, create-if-absent PUT followed by an exact readback check.
  put_immutable_input "$dist/$RUNTIME_FILE" "$RUNTIME_SHA256" "$runtime_object"
  put_immutable_input "$dist/$GAME_FILE" "$GAME_SHA256" "$game_object"
  put_immutable_input "$dist/$BEPINEX_FILE" "$BEPINEX_SHA256" "$bepinex_object"
  put_immutable_input "$manifest_snapshot" "$manifest_sha256" "$manifest_object"
  write_staged_settings
  printf 'staged immutable runtime inputs at s3://%s/%s/\n' "$bucket" "$prefix"
  exit 0
fi

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

put_immutable_input "$dist/$RUNTIME_FILE" "$RUNTIME_SHA256" "$runtime_object"

for file in "$GAME_FILE" "$BEPINEX_FILE"; do
  aws --profile "$profile" --region "$region" s3 cp "$dist/$file" \
    "s3://$bucket/$prefix/$file" --only-show-errors
done

for save_key in "${save_keys[@]}"; do
  save="$BIBITES_SAVE_DIR/$(basename "$save_key")"
  aws --profile "$profile" --region "$region" s3 cp "$save" \
    "s3://$bucket/$prefix/$save_key" --only-show-errors
done

put_immutable_input "$dist/worlds.json" "$manifest_sha256" "$manifest_object"

# Publish the manifest last. A host never sees references to incomplete uploads.
aws --profile "$profile" --region "$region" s3 cp "$dist/worlds.json" \
  "s3://$bucket/$prefix/worlds.json" --only-show-errors

write_staged_settings
printf 'staged private artifacts at s3://%s/%s/\n' "$bucket" "$prefix"
