#!/usr/bin/env bash
# Promote one verified runtime archive as the replacement-host bootstrap source.
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
dist="$repo/cloud/aws/dist"
validation="$repo/cloud/aws/lib/validation.sh"

[ -r "$dist/artifacts.env" ] || { echo 'run build-artifacts.sh first' >&2; exit 1; }
[ -r "$dist/staged.env" ] || { echo 'run stage-artifacts.sh first' >&2; exit 1; }
# shellcheck source=lib/validation.sh
. "$validation"
# shellcheck source=/dev/null
. "$dist/artifacts.env"
# shellcheck source=/dev/null
. "$dist/staged.env"

: "${BIBITES_AWS_ACCOUNT_ID:?set the approved 12-digit AWS account identifier}"
: "${RUNTIME_OBJECT:?run stage-artifacts.sh again to create an immutable runtime object}"

publish_if_absent=0
if [ "${1:-}" = --if-absent ]; then
  publish_if_absent=1
  shift
fi
[ "$#" -le 2 ] || {
  echo 'usage: promote-runtime.sh [--if-absent] [RUNTIME_OBJECT [RUNTIME_SHA256]]' >&2
  exit 2
}
source_runtime_file="${1:-$RUNTIME_OBJECT}"
source_runtime_sha256="${2:-$RUNTIME_SHA256}"
pointer_file=runtime/current.json
promoted_runtime_file="runtime/$source_runtime_sha256.tar.gz"

bibites_require_account_id "$BIBITES_AWS_ACCOUNT_ID" BIBITES_AWS_ACCOUNT_ID
bibites_require_region "$AWS_REGION" AWS_REGION
bibites_require_s3_bucket "$ARTIFACT_BUCKET" ARTIFACT_BUCKET
bibites_require_s3_prefix "$ARTIFACT_PREFIX" ARTIFACT_PREFIX
bibites_require_s3_key "$source_runtime_file" 'source runtime object'
bibites_require_sha256 "$source_runtime_sha256" 'source runtime digest'
bibites_require_s3_key "$promoted_runtime_file" 'promoted runtime object'
bibites_require_s3_key "$pointer_file" 'runtime pointer object'
pointer_document="$(bibites_runtime_pointer_document \
  "$promoted_runtime_file" "$source_runtime_sha256")"

account="$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" sts get-caller-identity \
  --query Account --output text)"
bibites_require_account_id "$account" 'authenticated AWS account'
[ "$account" = "$BIBITES_AWS_ACCOUNT_ID" ] || {
  echo "refusing AWS account $account; expected $BIBITES_AWS_ACCOUNT_ID" >&2
  exit 1
}

archive="$(mktemp)"
promoted_archive="$(mktemp)"
object_error="$(mktemp)"
pointer_path="$(mktemp)"
pointer_error="$(mktemp)"
trap 'rm -f "$archive" "$promoted_archive" "$object_error" "$pointer_path" "$pointer_error"' EXIT
aws --profile "$AWS_PROFILE" --region "$AWS_REGION" s3 cp \
  "s3://$ARTIFACT_BUCKET/$ARTIFACT_PREFIX/$source_runtime_file" "$archive" \
  --only-show-errors
printf '%s  %s\n' "$source_runtime_sha256" "$archive" | sha256sum -c - >/dev/null

if aws --profile "$AWS_PROFILE" --region "$AWS_REGION" s3 cp \
  "s3://$ARTIFACT_BUCKET/$ARTIFACT_PREFIX/$promoted_runtime_file" \
  "$promoted_archive" --only-show-errors 2>"$object_error"; then
  printf '%s  %s\n' "$source_runtime_sha256" "$promoted_archive" | \
    sha256sum -c - >/dev/null
elif grep -Eq '(404|NoSuchKey|Not Found)' "$object_error"; then
  aws --profile "$AWS_PROFILE" --region "$AWS_REGION" s3 cp "$archive" \
    "s3://$ARTIFACT_BUCKET/$ARTIFACT_PREFIX/$promoted_runtime_file" \
    --only-show-errors
else
  sed 's/^/Promoted runtime lookup failed: /' "$object_error" >&2
  exit 1
fi

# One S3 PUT publishes the descriptor only after its content-addressed archive is verified.
if [ "$publish_if_absent" -eq 0 ]; then
  printf '%s\n' "$pointer_document" | \
    aws --profile "$AWS_PROFILE" --region "$AWS_REGION" s3 cp - \
      "s3://$ARTIFACT_BUCKET/$ARTIFACT_PREFIX/$pointer_file" \
      --content-type application/json --only-show-errors
else
  printf '%s\n' "$pointer_document" >"$pointer_path"
  if aws --profile "$AWS_PROFILE" --region "$AWS_REGION" s3api put-object \
    --bucket "$ARTIFACT_BUCKET" --key "$ARTIFACT_PREFIX/$pointer_file" \
    --body "$pointer_path" --content-type application/json --if-none-match '*' \
    >/dev/null 2>"$pointer_error"; then
    :
  elif grep -Eq '(PreconditionFailed|412)' "$pointer_error"; then
    if ! current_pointer_document="$(aws --profile "$AWS_PROFILE" \
      --region "$AWS_REGION" s3 cp \
      "s3://$ARTIFACT_BUCKET/$ARTIFACT_PREFIX/$pointer_file" - \
      --only-show-errors 2>"$pointer_error")"; then
      sed 's/^/Concurrent runtime pointer lookup failed: /' "$pointer_error" >&2
      exit 1
    fi
    bibites_require_runtime_pointer_document "$current_pointer_document"
    current_runtime_file="$(jq -r .runtimeFile <<<"$current_pointer_document")"
    current_runtime_sha256="$(jq -r .runtimeSha256 <<<"$current_pointer_document")"
    if [ "$current_runtime_file" != "$promoted_runtime_file" ] ||
       [ "$current_runtime_sha256" != "$source_runtime_sha256" ]; then
      echo 'refusing to overwrite a different runtime pointer published concurrently' >&2
      exit 1
    fi
    echo 'the runtime pointer already identifies this verified archive'
  else
    sed 's/^/Conditional runtime pointer publication failed: /' "$pointer_error" >&2
    exit 1
  fi
fi
printf 'promoted bootstrap runtime %s\n' "$source_runtime_sha256"
