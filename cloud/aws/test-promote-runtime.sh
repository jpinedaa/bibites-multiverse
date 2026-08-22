#!/usr/bin/env bash
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
test_root="$(mktemp -d)"
trap 'rm -rf "$test_root"' EXIT

fixture_repo="$test_root/repo"
fixture_cloud="$fixture_repo/cloud/aws"
fixture_dist="$fixture_cloud/dist"
fixture_bin="$test_root/bin"
install -d "$fixture_cloud/lib" "$fixture_dist" "$fixture_bin"
cp "$repo/cloud/aws/promote-runtime.sh" "$fixture_cloud/promote-runtime.sh"
cp "$repo/cloud/aws/lib/validation.sh" "$fixture_cloud/lib/validation.sh"
chmod 0755 "$fixture_cloud/promote-runtime.sh"

archive="$test_root/runtime.tar.gz"
printf 'fixture runtime\n' >"$archive"
runtime_sha="$(sha256sum "$archive" | awk '{print $1}')"
runtime_object="runtime/$runtime_sha.tar.gz"
printf '{"schema":1,"worlds":[]}\n' >"$fixture_dist/worlds.json"
manifest_sha="$(sha256sum "$fixture_dist/worlds.json" | awk '{print $1}')"
manifest_object="worlds.$manifest_sha.json"

cat >"$fixture_bin/aws" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"$MOCK_AWS_LOG"
args=" $* "
case "$args" in
  *' sts get-caller-identity '*)
    printf '123456789012\n'
    ;;
  *' s3 cp '*)
    source=''
    destination=''
    while [ "$#" -gt 0 ]; do
      if [ "$1" = cp ]; then
        source="$2"
        destination="$3"
        break
      fi
      shift
    done
    if [ "$source" = - ]; then
      cat >"$MOCK_POINTER_STATE"
    elif [[ "$source" == */runtime/current.json ]] && [ "$destination" = - ]; then
      [ -r "$MOCK_POINTER_STATE" ] || {
        echo '404 Not Found' >&2
        exit 1
      }
      cat "$MOCK_POINTER_STATE"
    elif [[ "$source" == s3://* ]] && [ "$destination" != - ]; then
      cp "$MOCK_ARCHIVE" "$destination"
    else
      echo "unexpected S3 copy: $source $destination" >&2
      exit 65
    fi
    ;;
  *' s3api put-object '*)
    body=''
    while [ "$#" -gt 0 ]; do
      case "$1" in
        --body) body="$2"; shift ;;
      esac
      shift
    done
    [ -n "$body" ] || exit 65
    if [ "${MOCK_POINTER_PUT_RESULT:-success}" = precondition ]; then
      echo 'An error occurred (PreconditionFailed) when calling PutObject: 412' >&2
      exit 255
    elif [ "${MOCK_POINTER_PUT_RESULT:-success}" = lost-response ]; then
      cp "$body" "$MOCK_POINTER_STATE"
      echo 'RequestTimeout: response was lost after the object was stored' >&2
      exit 255
    elif [ "${MOCK_POINTER_PUT_RESULT:-success}" = error ]; then
      echo 'An error occurred (AccessDenied) when calling PutObject: denied' >&2
      exit 255
    fi
    cp "$body" "$MOCK_POINTER_STATE"
    ;;
  *)
    echo "unexpected AWS command: $*" >&2
    exit 65
    ;;
esac
MOCK
chmod 0755 "$fixture_bin/aws"

run_explicit() {
  env PATH="$fixture_bin:$PATH" AWS_PROFILE=explicit-profile AWS_REGION=us-east-1 \
    BIBITES_AWS_ACCOUNT_ID=123456789012 ARTIFACT_BUCKET=explicit-artifacts \
    ARTIFACT_PREFIX=explicit/v1 MOCK_ARCHIVE="$archive" \
    MOCK_AWS_LOG="$test_root/aws.log" MOCK_POINTER_STATE="$test_root/pointer.json" \
    "$fixture_cloud/promote-runtime.sh" "$@"
}

# Explicit settings must win without reading stale build or stage files.
cat >"$fixture_dist/artifacts.env" <<'EOF'
printf 'artifacts.env was read\n' >>"$MOCK_ENV_READ_LOG"
exit 91
EOF
cat >"$fixture_dist/staged.env" <<'EOF'
printf 'staged.env was read\n' >>"$MOCK_ENV_READ_LOG"
exit 92
EOF
export MOCK_ENV_READ_LOG="$test_root/env-read.log"
: >"$test_root/aws.log"
run_explicit --if-absent "$runtime_object" "$runtime_sha" >/dev/null
[ ! -e "$test_root/env-read.log" ]
[ "$(jq -r .runtimeSha256 "$test_root/pointer.json")" = "$runtime_sha" ]
grep -Fq -- '--profile explicit-profile --region us-east-1' "$test_root/aws.log"
grep -Fq "s3://explicit-artifacts/explicit/v1/$runtime_object" "$test_root/aws.log"
[ "$(grep -Fc "s3://explicit-artifacts/explicit/v1/$runtime_object" \
  "$test_root/aws.log")" -eq 1 ]

# A missing-pointer condition publishes only through PutObject with If-None-Match.
rm -f "$test_root/pointer.json"
: >"$test_root/aws.log"
run_explicit --if-absent "$runtime_object" "$runtime_sha" >/dev/null
[ "$(jq -r .runtimeSha256 "$test_root/pointer.json")" = "$runtime_sha" ]
grep -Fq -- '--if-none-match *' "$test_root/aws.log"
grep -Fq -- '--server-side-encryption AES256' "$test_root/aws.log"

# A lost successful PUT response is reconciled by reading the pointer.
rm -f "$test_root/pointer.json"
: >"$test_root/aws.log"
lost_output="$(MOCK_POINTER_PUT_RESULT=lost-response run_explicit \
  --if-absent "$runtime_object" "$runtime_sha")"
grep -Fq 'the runtime pointer already identifies this verified archive' \
  <<<"$lost_output"
[ "$(jq -r .runtimeSha256 "$test_root/pointer.json")" = "$runtime_sha" ]
grep -Fq 's3://explicit-artifacts/explicit/v1/runtime/current.json -' \
  "$test_root/aws.log"

# A lost ETag race is idempotent when the winner published the same candidate.
jq -cn --arg file "$runtime_object" --arg sha "$runtime_sha" \
  '{schema:1,runtimeFile:$file,runtimeSha256:$sha}' >"$test_root/pointer.json"
: >"$test_root/aws.log"
identical_output="$(MOCK_POINTER_PUT_RESULT=precondition run_explicit \
  --if-absent "$runtime_object" "$runtime_sha")"
grep -Fq 'the runtime pointer already identifies this verified archive' \
  <<<"$identical_output"
[ "$(jq -r .runtimeSha256 "$test_root/pointer.json")" = "$runtime_sha" ]
grep -Fq -- '--if-none-match *' "$test_root/aws.log"
grep -Fq 's3://explicit-artifacts/explicit/v1/runtime/current.json -' \
  "$test_root/aws.log"

# A different valid pointer leaves it unchanged and fails.
other_sha=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
jq -cn --arg file "runtime/$other_sha.tar.gz" --arg sha "$other_sha" \
  '{schema:1,runtimeFile:$file,runtimeSha256:$sha}' >"$test_root/pointer.json"
cp "$test_root/pointer.json" "$test_root/pointer-before.json"
: >"$test_root/aws.log"
set +e
match_output="$(MOCK_POINTER_PUT_RESULT=precondition run_explicit \
  --if-absent "$runtime_object" "$runtime_sha" 2>&1)"
match_status=$?
set -e
[ "$match_status" -eq 1 ]
grep -Fq 'refusing to overwrite a different runtime pointer published concurrently' \
  <<<"$match_output"
cmp -s "$test_root/pointer-before.json" "$test_root/pointer.json"
grep -Fq -- '--if-none-match *' "$test_root/aws.log"

# Unexpected conditional publication errors are not mistaken for a race.
: >"$test_root/aws.log"
set +e
put_error_output="$(MOCK_POINTER_PUT_RESULT=error run_explicit \
  --if-absent "$runtime_object" "$runtime_sha" 2>&1)"
put_error_status=$?
set -e
[ "$put_error_status" -eq 1 ]
grep -Fq 'Conditional runtime pointer publication failed:' <<<"$put_error_output"
grep -Fq 'AccessDenied' <<<"$put_error_output"

# Existing-pointer replacement and missing conditions are rejected before AWS.
: >"$test_root/aws.log"
set +e
replacement_output="$(run_explicit --if-match '"fixture-etag"' \
  "$runtime_object" "$runtime_sha" 2>&1)"
replacement_status=$?
missing_condition_output="$(run_explicit "$runtime_object" "$runtime_sha" 2>&1)"
missing_condition_status=$?
set -e
[ "$replacement_status" -eq 2 ]
grep -Fq 'Use update-runtime.sh to replace an existing runtime pointer.' \
  <<<"$replacement_output"
[ "$missing_condition_status" -eq 2 ]
grep -Fq 'Runtime bootstrap promotion requires --if-absent.' \
  <<<"$missing_condition_output"
[ ! -s "$test_root/aws.log" ]

# Promotion never copies an arbitrary source into a content-addressed key.
: >"$test_root/aws.log"
set +e
source_output="$(run_explicit --if-absent incoming/runtime.tar.gz "$runtime_sha" 2>&1)"
source_status=$?
set -e
[ "$source_status" -eq 1 ]
grep -Fq 'must already be its content-addressed runtime object' <<<"$source_output"
[ ! -s "$test_root/aws.log" ]

# The no-argument mode keeps the build-and-stage defaults.
cat >"$fixture_dist/artifacts.env" <<EOF
RUNTIME_SHA256=$runtime_sha
GAME_SHA256=1111111111111111111111111111111111111111111111111111111111111111
BEPINEX_SHA256=2222222222222222222222222222222222222222222222222222222222222222
EOF
cat >"$fixture_dist/staged.env" <<EOF
AWS_PROFILE=staged-profile
AWS_REGION=us-east-1
ARTIFACT_BUCKET=staged-artifacts
ARTIFACT_PREFIX=staged/v1
RUNTIME_OBJECT=$runtime_object
STAGED_RUNTIME_SHA256=$runtime_sha
STAGED_GAME_SHA256=1111111111111111111111111111111111111111111111111111111111111111
STAGED_BEPINEX_SHA256=2222222222222222222222222222222222222222222222222222222222222222
MANIFEST_OBJECT=$manifest_object
MANIFEST_SHA256=$manifest_sha
STAGING_SCOPE=complete
EOF
rm -f "$test_root/pointer.json"
: >"$test_root/aws.log"
env -u AWS_PROFILE -u AWS_REGION -u ARTIFACT_BUCKET -u ARTIFACT_PREFIX \
  PATH="$fixture_bin:$PATH" BIBITES_AWS_ACCOUNT_ID=123456789012 \
  MOCK_ARCHIVE="$archive" MOCK_AWS_LOG="$test_root/aws.log" \
  MOCK_POINTER_STATE="$test_root/pointer.json" \
  "$fixture_cloud/promote-runtime.sh" --if-absent >/dev/null
[ "$(jq -r .runtimeSha256 "$test_root/pointer.json")" = "$runtime_sha" ]
grep -Fq -- '--profile staged-profile --region us-east-1' "$test_root/aws.log"
grep -Fq "s3://staged-artifacts/staged/v1/$runtime_object" "$test_root/aws.log"

# The one-argument mode keeps its digest default from artifacts.env.
rm -f "$test_root/pointer.json"
: >"$test_root/aws.log"
env -u AWS_PROFILE -u AWS_REGION -u ARTIFACT_BUCKET -u ARTIFACT_PREFIX \
  PATH="$fixture_bin:$PATH" BIBITES_AWS_ACCOUNT_ID=123456789012 \
  MOCK_ARCHIVE="$archive" MOCK_AWS_LOG="$test_root/aws.log" \
  MOCK_POINTER_STATE="$test_root/pointer.json" \
  "$fixture_cloud/promote-runtime.sh" --if-absent "$runtime_object" >/dev/null
[ "$(jq -r .runtimeSha256 "$test_root/pointer.json")" = "$runtime_sha" ]
grep -Fq -- '--profile staged-profile --region us-east-1' "$test_root/aws.log"

# Implicit modes reject a missing, runtime-only, or stale complete receipt
# before the first AWS command. Ambient scope and digest values cannot fill a
# missing receipt field.
cp "$fixture_dist/staged.env" "$test_root/complete-staged.env"
cp "$fixture_dist/artifacts.env" "$test_root/complete-artifacts.env"
cp "$fixture_dist/worlds.json" "$test_root/complete-worlds.json"
for invalid_receipt in missing-scope runtime-only stale-runtime stale-game \
  stale-bepinex missing-digest missing-artifact missing-manifest-object \
  missing-manifest-sha stale-manifest changed-manifest; do
  cp "$test_root/complete-artifacts.env" "$fixture_dist/artifacts.env"
  cp "$test_root/complete-staged.env" "$fixture_dist/staged.env"
  cp "$test_root/complete-worlds.json" "$fixture_dist/worlds.json"
  case "$invalid_receipt" in
    missing-scope)
      sed -i '/^STAGING_SCOPE=/d' "$fixture_dist/staged.env"
      export STAGING_SCOPE=complete
      ;;
    runtime-only)
      sed -i 's/^STAGING_SCOPE=.*/STAGING_SCOPE=runtime-only/' \
        "$fixture_dist/staged.env"
      ;;
    stale-runtime)
      sed -i 's/^STAGED_RUNTIME_SHA256=.*/STAGED_RUNTIME_SHA256=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb/' \
        "$fixture_dist/staged.env"
      ;;
    stale-game)
      sed -i 's/^STAGED_GAME_SHA256=.*/STAGED_GAME_SHA256=cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc/' \
        "$fixture_dist/staged.env"
      ;;
    stale-bepinex)
      sed -i 's/^STAGED_BEPINEX_SHA256=.*/STAGED_BEPINEX_SHA256=dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd/' \
        "$fixture_dist/staged.env"
      ;;
    missing-digest)
      sed -i '/^STAGED_RUNTIME_SHA256=/d' "$fixture_dist/staged.env"
      export STAGED_RUNTIME_SHA256="$runtime_sha"
      ;;
    missing-artifact)
      sed -i '/^RUNTIME_SHA256=/d' "$fixture_dist/artifacts.env"
      export RUNTIME_SHA256="$runtime_sha"
      ;;
    missing-manifest-object)
      sed -i '/^MANIFEST_OBJECT=/d' "$fixture_dist/staged.env"
      export MANIFEST_OBJECT="$manifest_object"
      ;;
    missing-manifest-sha)
      sed -i '/^MANIFEST_SHA256=/d' "$fixture_dist/staged.env"
      export MANIFEST_SHA256="$manifest_sha"
      ;;
    stale-manifest)
      sed -i \
        's/^MANIFEST_SHA256=.*/MANIFEST_SHA256=dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd/' \
        "$fixture_dist/staged.env"
      ;;
    changed-manifest) printf '\n' >>"$fixture_dist/worlds.json" ;;
  esac
  : >"$test_root/aws.log"
  set +e
  receipt_output="$(env -u AWS_PROFILE -u AWS_REGION -u ARTIFACT_BUCKET \
    -u ARTIFACT_PREFIX PATH="$fixture_bin:$PATH" \
    BIBITES_AWS_ACCOUNT_ID=123456789012 MOCK_ARCHIVE="$archive" \
    MOCK_AWS_LOG="$test_root/aws.log" \
    MOCK_POINTER_STATE="$test_root/pointer.json" \
    "$fixture_cloud/promote-runtime.sh" --if-absent 2>&1)"
  receipt_status=$?
  set -e
  unset STAGING_SCOPE STAGED_RUNTIME_SHA256 RUNTIME_SHA256 \
    MANIFEST_OBJECT MANIFEST_SHA256
  [ "$receipt_status" -ne 0 ]
  [ ! -s "$test_root/aws.log" ] || {
    echo "$invalid_receipt implicit receipt reached AWS" >&2
    exit 1
  }
  case "$invalid_receipt" in
    missing-scope|runtime-only)
      grep -Fq 'requires STAGING_SCOPE=complete' <<<"$receipt_output"
      ;;
    stale-runtime|stale-game|stale-bepinex|missing-artifact)
      grep -Fq 'does not match artifacts.env' <<<"$receipt_output"
      ;;
    missing-digest)
      grep -Fq 'Set STAGED_RUNTIME_SHA256' <<<"$receipt_output"
      ;;
    missing-manifest-object)
      grep -Fq 'Set MANIFEST_OBJECT' <<<"$receipt_output"
      ;;
    missing-manifest-sha)
      grep -Fq 'Set MANIFEST_SHA256' <<<"$receipt_output"
      ;;
    stale-manifest)
      grep -Fq 'manifest object does not match its digest' <<<"$receipt_output"
      ;;
    changed-manifest)
      grep -Fq 'staged manifest does not match the complete staging receipt' \
        <<<"$receipt_output"
      ;;
  esac
done
cp "$test_root/complete-artifacts.env" "$fixture_dist/artifacts.env"
cp "$test_root/complete-staged.env" "$fixture_dist/staged.env"
cp "$test_root/complete-worlds.json" "$fixture_dist/worlds.json"

# Missing explicit settings fail before the first AWS command.
cat >"$fixture_dist/artifacts.env" <<'EOF'
printf 'artifacts.env was read\n' >>"$MOCK_ENV_READ_LOG"
exit 91
EOF
cat >"$fixture_dist/staged.env" <<'EOF'
printf 'staged.env was read\n' >>"$MOCK_ENV_READ_LOG"
exit 92
EOF
rm -f "$test_root/env-read.log"
: >"$test_root/aws.log"
set +e
missing_output="$(env -u ARTIFACT_PREFIX PATH="$fixture_bin:$PATH" \
  AWS_PROFILE=explicit-profile AWS_REGION=us-east-1 \
  BIBITES_AWS_ACCOUNT_ID=123456789012 ARTIFACT_BUCKET=explicit-artifacts \
  MOCK_ARCHIVE="$archive" MOCK_AWS_LOG="$test_root/aws.log" \
  MOCK_POINTER_STATE="$test_root/pointer.json" \
  "$fixture_cloud/promote-runtime.sh" --if-absent \
    "$runtime_object" "$runtime_sha" 2>&1)"
missing_status=$?
set -e
[ "$missing_status" -eq 1 ]
grep -Fq 'Set ARTIFACT_PREFIX for runtime promotion.' <<<"$missing_output"
[ ! -e "$test_root/env-read.log" ]
[ ! -s "$test_root/aws.log" ]

printf 'runtime promotion fixtures passed\n'
