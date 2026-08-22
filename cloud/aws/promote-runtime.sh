#!/usr/bin/env bash
# Promote one verified runtime archive as the replacement-host bootstrap source.
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
dist="$repo/cloud/aws/dist"
validation="$repo/cloud/aws/lib/validation.sh"

# shellcheck source=lib/validation.sh
. "$validation"

[ "${1:-}" = --if-absent ] || {
  echo 'Runtime bootstrap promotion requires --if-absent.' >&2
  echo 'Use update-runtime.sh to replace an existing runtime pointer.' >&2
  echo 'usage: promote-runtime.sh --if-absent [RUNTIME_OBJECT [RUNTIME_SHA256]]' >&2
  exit 2
}
shift
[ "$#" -le 2 ] || {
  echo 'usage: promote-runtime.sh --if-absent [RUNTIME_OBJECT [RUNTIME_SHA256]]' >&2
  exit 2
}

require_setting() {
  local name="$1"
  [ -n "${!name:-}" ] || {
    printf 'Set %s for runtime promotion.\n' "$name" >&2
    exit 1
  }
}

require_complete_staging_receipt() {
  local receipt_name local_manifest_sha256
  [ "${STAGING_SCOPE:-}" = complete ] || {
    echo 'Implicit runtime promotion requires STAGING_SCOPE=complete from stage-artifacts.sh.' >&2
    exit 1
  }
  for receipt_name in STAGED_RUNTIME_SHA256 STAGED_GAME_SHA256 STAGED_BEPINEX_SHA256; do
    require_setting "$receipt_name"
    bibites_require_sha256 "${!receipt_name}" "$receipt_name"
  done
  [ "$STAGED_RUNTIME_SHA256" = "${RUNTIME_SHA256:-}" ] &&
    [ "$STAGED_GAME_SHA256" = "${GAME_SHA256:-}" ] &&
    [ "$STAGED_BEPINEX_SHA256" = "${BEPINEX_SHA256:-}" ] || {
    echo 'The complete staging receipt does not match artifacts.env. Run stage-artifacts.sh again.' >&2
    exit 1
  }
  [ "${RUNTIME_OBJECT:-}" = "runtime/$STAGED_RUNTIME_SHA256.tar.gz" ] || {
    echo 'The complete staging receipt runtime object does not match its digest.' >&2
    exit 1
  }
  for receipt_name in MANIFEST_OBJECT MANIFEST_SHA256; do
    require_setting "$receipt_name"
  done
  bibites_require_sha256 "$MANIFEST_SHA256" MANIFEST_SHA256
  bibites_require_s3_filename "$MANIFEST_OBJECT" MANIFEST_OBJECT
  [ "$MANIFEST_OBJECT" = "worlds.$MANIFEST_SHA256.json" ] || {
    echo 'The complete staging receipt manifest object does not match its digest.' >&2
    exit 1
  }
  [ -r "$dist/worlds.json" ] || {
    echo 'The complete staging receipt requires a local staged manifest.' >&2
    exit 1
  }
  local_manifest_sha256="$(sha256sum "$dist/worlds.json" | awk '{print $1}')"
  [ "$local_manifest_sha256" = "$MANIFEST_SHA256" ] || {
    echo 'The staged manifest does not match the complete staging receipt.' >&2
    echo 'Run stage-artifacts.sh again.' >&2
    exit 1
  }
}

load_staged_settings() {
  local artifact_runtime_file artifact_runtime_sha256 artifact_game_file
  local artifact_game_sha256 artifact_bepinex_file artifact_bepinex_sha256
  [ -r "$dist/artifacts.env" ] || {
    echo 'Run build-artifacts.sh first.' >&2
    exit 1
  }
  [ -r "$dist/staged.env" ] || {
    echo 'Run stage-artifacts.sh first.' >&2
    exit 1
  }
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
  # Implicit mode uses only settings recorded by this staging receipt.
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
  require_complete_staging_receipt
}

case "$#" in
  0)
    load_staged_settings
    require_setting RUNTIME_OBJECT
    require_setting RUNTIME_SHA256
    source_runtime_file="$RUNTIME_OBJECT"
    source_runtime_sha256="$RUNTIME_SHA256"
    ;;
  1)
    load_staged_settings
    require_setting RUNTIME_SHA256
    source_runtime_file="$1"
    source_runtime_sha256="$RUNTIME_SHA256"
    ;;
  2)
    # An explicit bootstrap promotion supplies the runtime identity and all
    # AWS settings. Do not let stale build or stage files replace those values.
    source_runtime_file="$1"
    source_runtime_sha256="$2"
    ;;
esac

for setting in AWS_PROFILE AWS_REGION BIBITES_AWS_ACCOUNT_ID \
  ARTIFACT_BUCKET ARTIFACT_PREFIX; do
  require_setting "$setting"
done
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
[ "$source_runtime_file" = "$promoted_runtime_file" ] || {
  echo 'The source runtime object must already be its content-addressed runtime object.' >&2
  exit 1
}
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
pointer_path="$(mktemp)"
pointer_error="$(mktemp)"
trap 'rm -f "$archive" "$pointer_path" "$pointer_error"' EXIT
aws --profile "$AWS_PROFILE" --region "$AWS_REGION" s3 cp \
  "s3://$ARTIFACT_BUCKET/$ARTIFACT_PREFIX/$source_runtime_file" "$archive" \
  --only-show-errors
printf '%s  %s\n' "$source_runtime_sha256" "$archive" | sha256sum -c - >/dev/null

pointer_already_identifies_candidate() {
  local current_pointer_document current_runtime_file current_runtime_sha256
  : >"$pointer_error"
  if ! current_pointer_document="$(aws --profile "$AWS_PROFILE" \
    --region "$AWS_REGION" s3 cp \
    "s3://$ARTIFACT_BUCKET/$ARTIFACT_PREFIX/$pointer_file" - \
    --only-show-errors 2>"$pointer_error")"; then
    sed 's/^/Concurrent runtime pointer lookup failed: /' "$pointer_error" >&2
    return 2
  fi
  bibites_require_runtime_pointer_document "$current_pointer_document" || return 2
  current_runtime_file="$(jq -r .runtimeFile <<<"$current_pointer_document")"
  current_runtime_sha256="$(jq -r .runtimeSha256 <<<"$current_pointer_document")"
  [ "$current_runtime_file" = "$promoted_runtime_file" ] &&
    [ "$current_runtime_sha256" = "$source_runtime_sha256" ]
}

# One S3 PUT creates the descriptor only after its content-addressed archive is verified.
# Existing-pointer replacement belongs to the host-wide update transaction.
printf '%s\n' "$pointer_document" >"$pointer_path"
if aws --profile "$AWS_PROFILE" --region "$AWS_REGION" s3api put-object \
  --bucket "$ARTIFACT_BUCKET" --key "$ARTIFACT_PREFIX/$pointer_file" \
  --body "$pointer_path" --content-type application/json \
  --server-side-encryption AES256 --if-none-match '*' \
  >/dev/null 2>"$pointer_error"; then
  :
else
  publication_error="$(<"$pointer_error")"
  if pointer_already_identifies_candidate; then
    echo 'the runtime pointer already identifies this verified archive'
  else
    pointer_status=$?
    if grep -Eq '(PreconditionFailed|412)' <<<"$publication_error" &&
       [ "$pointer_status" -eq 1 ]; then
      echo 'refusing to overwrite a different runtime pointer published concurrently' >&2
    else
      printf '%s\n' "$publication_error" | \
        sed 's/^/Conditional runtime pointer publication failed: /' >&2
      if [ "$pointer_status" -eq 1 ]; then
        echo 'the runtime pointer does not identify the candidate after the failed publication' >&2
      fi
    fi
    exit 1
  fi
fi
printf 'promoted bootstrap runtime %s\n' "$source_runtime_sha256"
