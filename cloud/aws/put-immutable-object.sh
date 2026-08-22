#!/usr/bin/env bash
# Create one content-addressed S3 object without overwriting an existing value.
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
validation="$repo/cloud/aws/lib/validation.sh"

# shellcheck source=lib/validation.sh
. "$validation"

[ "$#" -eq 4 ] || {
  echo 'usage: put-immutable-object.sh LOCAL_FILE EXPECTED_SHA256 BUCKET OBJECT_KEY' >&2
  exit 2
}

local_file="$1"
expected_sha256="$2"
bucket="$3"
object_key="$4"
profile="${AWS_PROFILE:-bibites-multiverse}"
region="${AWS_REGION:-us-east-1}"

: "${BIBITES_AWS_ACCOUNT_ID:?set the approved 12-digit AWS account identifier}"
[ -r "$local_file" ] || {
  echo "cannot read immutable object source: $local_file" >&2
  exit 1
}
bibites_require_account_id "$BIBITES_AWS_ACCOUNT_ID" BIBITES_AWS_ACCOUNT_ID
bibites_require_sha256 "$expected_sha256" EXPECTED_SHA256
bibites_require_s3_bucket "$bucket" BUCKET
bibites_require_s3_key "$object_key" OBJECT_KEY
bibites_require_region "$region" AWS_REGION

source_copy="$(mktemp)"
put_error="$(mktemp)"
verified_copy="$(mktemp)"
trap 'rm -f "$source_copy" "$put_error" "$verified_copy"' EXIT
chmod 0600 "$source_copy"
if ! cp -- "$local_file" "$source_copy"; then
  echo "cannot snapshot immutable object source: $local_file" >&2
  exit 1
fi
if ! printf '%s  %s\n' "$expected_sha256" "$source_copy" | \
  sha256sum -c - >/dev/null 2>&1; then
  echo "immutable object source digest does not match: $local_file" >&2
  exit 1
fi

account="$(aws --profile "$profile" --region "$region" sts get-caller-identity \
  --query Account --output text)"
bibites_require_account_id "$account" 'authenticated AWS account'
[ "$account" = "$BIBITES_AWS_ACCOUNT_ID" ] || {
  echo "refusing AWS account $account; expected $BIBITES_AWS_ACCOUNT_ID" >&2
  exit 1
}

publication_result=created
publication_error=
if aws --profile "$profile" --region "$region" s3api put-object \
  --bucket "$bucket" --key "$object_key" --body "$source_copy" \
  --server-side-encryption AES256 --if-none-match '*' \
  >/dev/null 2>"$put_error"; then
  :
else
  publication_error="$(<"$put_error")"
  if grep -Eq '(PreconditionFailed|412)' <<<"$publication_error"; then
    publication_result=existing
  else
    publication_result=reconciled
  fi
fi

# Verify after every PUT result. This also resolves a lost success response
# without blindly retrying the write.
if ! aws --profile "$profile" --region "$region" s3 cp \
  "s3://$bucket/$object_key" "$verified_copy" --only-show-errors; then
  if [ -n "$publication_error" ]; then
    printf '%s\n' "$publication_error" | \
      sed 's/^/Immutable object publication failed: /' >&2
  fi
  echo "cannot read back immutable object: s3://$bucket/$object_key" >&2
  exit 1
fi
if ! printf '%s  %s\n' "$expected_sha256" "$verified_copy" | \
  sha256sum -c - >/dev/null 2>&1; then
  if [ -n "$publication_error" ]; then
    printf '%s\n' "$publication_error" | \
      sed 's/^/Immutable object publication failed: /' >&2
  fi
  echo "immutable object has unexpected content: s3://$bucket/$object_key" >&2
  exit 1
fi

case "$publication_result" in
  created)
    printf 'created and verified immutable object s3://%s/%s\n' \
      "$bucket" "$object_key"
    ;;
  existing)
    printf 'verified existing immutable object s3://%s/%s\n' \
      "$bucket" "$object_key"
    ;;
  reconciled)
    printf 'reconciled and verified immutable object s3://%s/%s\n' \
      "$bucket" "$object_key"
    ;;
esac
