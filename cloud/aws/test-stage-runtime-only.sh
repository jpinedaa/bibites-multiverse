#!/usr/bin/env bash
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
test_root="$(mktemp -d)"
trap 'find "$test_root" -depth -delete' EXIT

fixture_repo="$test_root/repo"
fixture_cloud="$fixture_repo/cloud/aws"
fixture_dist="$fixture_cloud/dist"
fixture_bin="$test_root/bin"
object_dir="$test_root/objects"
save_dir="$test_root/saves"
install -d "$fixture_cloud/lib" "$fixture_cloud/runtime" "$fixture_dist" \
  "$fixture_bin" "$object_dir" "$save_dir"
cp "$repo/cloud/aws/stage-artifacts.sh" "$fixture_cloud/stage-artifacts.sh"
cp "$repo/cloud/aws/put-immutable-object.sh" "$fixture_cloud/put-immutable-object.sh"
cp "$repo/cloud/aws/lib/validation.sh" "$fixture_cloud/lib/validation.sh"
cp "$repo/cloud/aws/runtime/validate-world-manifest.jq" \
  "$fixture_cloud/runtime/validate-world-manifest.jq"
chmod 0755 "$fixture_cloud/stage-artifacts.sh" \
  "$fixture_cloud/put-immutable-object.sh"

runtime_file=runtime.tar.gz
game_file=game.zip
bepinex_file=bepinex.zip
printf 'fixture runtime\n' >"$fixture_dist/$runtime_file"
printf 'fixture game\n' >"$fixture_dist/$game_file"
printf 'fixture BepInEx\n' >"$fixture_dist/$bepinex_file"
runtime_sha256="$(sha256sum "$fixture_dist/$runtime_file" | awk '{print $1}')"
game_sha256="$(sha256sum "$fixture_dist/$game_file" | awk '{print $1}')"
bepinex_sha256="$(sha256sum "$fixture_dist/$bepinex_file" | awk '{print $1}')"
cat >"$fixture_dist/artifacts.env" <<EOF
RUNTIME_FILE=$runtime_file
RUNTIME_SHA256=$runtime_sha256
GAME_FILE=$game_file
GAME_SHA256=$game_sha256
BEPINEX_FILE=$bepinex_file
BEPINEX_SHA256=$bepinex_sha256
EOF

remote_manifest="$test_root/remote-worlds.json"
cat >"$remote_manifest" <<'EOF'
{"schema":1,"worlds":[{"id":"fixture","peerId":"fixture-peer",
"worldName":"Fixture World","sidecarPort":8787,
"saveKey":"imports/Fixture.zip",
"credentialParameter":"/bibites/cloud/fixture/peer-secret",
"position":"0,0","preferredSlot":1,"targetTimeScale":100,
"saveMinutes":10,"saveKeep":6,"enabled":true}]}
EOF
manifest_sha256="$(sha256sum "$remote_manifest" | awk '{print $1}')"
invalid_manifest="$test_root/invalid-worlds.json"
printf '{"schema":2,"worlds":[]}\n' >"$invalid_manifest"

# A local worlds.json from another operation must not be read or replaced by
# runtime-only staging.
printf 'existing local manifest sentinel\n' >"$fixture_dist/worlds.json"
cp "$fixture_dist/worlds.json" "$test_root/worlds-before"

save_contents="$test_root/save-contents"
install -d "$save_contents"
printf 'scene\n' >"$save_contents/scene.bb8scene"
(cd "$save_contents" && zip -q "$save_dir/Fixture.zip" scene.bb8scene)

cat >"$fixture_bin/aws" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"$MOCK_AWS_LOG"
args=" $* "
case "$args" in
  *' sts get-caller-identity '*) printf '123456789012\n' ;;
  *' s3api head-bucket '*)
    if [ "$MOCK_SCENARIO" = missing_bucket ]; then
      echo '404 Not Found' >&2
      exit 255
    fi
    ;;
  *' s3api put-public-access-block '*|*' s3api put-bucket-encryption '*) ;;
  *' s3api put-object '*)
    body=''
    key=''
    encryption=''
    if_none_match=''
    while [ "$#" -gt 0 ]; do
      case "$1" in
        --body) body="$2"; shift ;;
        --key) key="$2"; shift ;;
        --server-side-encryption) encryption="$2"; shift ;;
        --if-none-match) if_none_match="$2"; shift ;;
      esac
      shift
    done
    [ -n "$body" ] && [ -n "$key" ] || exit 65
    [ "$encryption" = AES256 ] && [ "$if_none_match" = '*' ] || exit 65
    remote="$MOCK_OBJECT_DIR/$key"
    if [ -e "$remote" ]; then
      echo 'An error occurred (PreconditionFailed) when calling PutObject: 412' >&2
      exit 255
    fi
    mkdir -p "$(dirname "$remote")"
    cp "$body" "$remote"
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
    if [[ "$source" == s3://* ]]; then
      [ "$(stat -c %a "$destination")" = 600 ] || {
        echo 'S3 destination was not owner-only before download' >&2
        exit 65
      }
      key="${source#s3://fixture-artifacts/}"
      if [ "$key" = cloud/v1/worlds.json ]; then
        case "$MOCK_SCENARIO" in
          missing_manifest)
            echo '404 NoSuchKey' >&2
            exit 255
            ;;
          invalid_manifest) cp "$MOCK_INVALID_MANIFEST" "$destination" ;;
          *) cp "$MOCK_REMOTE_MANIFEST" "$destination" ;;
        esac
      else
        cp "$MOCK_OBJECT_DIR/$key" "$destination"
      fi
    elif [[ "$destination" == s3://fixture-artifacts/* ]]; then
      key="${destination#s3://fixture-artifacts/}"
      remote="$MOCK_OBJECT_DIR/$key"
      mkdir -p "$(dirname "$remote")"
      cp "$source" "$remote"
    else
      echo "unexpected S3 copy: $source $destination" >&2
      exit 65
    fi
    ;;
  *)
    echo "unexpected AWS command: $*" >&2
    exit 65
    ;;
esac
MOCK
chmod 0755 "$fixture_bin/aws"

run_runtime_only() {
  local scenario="$1"
  env PATH="$fixture_bin:$PATH" AWS_PROFILE=fixture AWS_REGION=us-east-1 \
    BIBITES_AWS_ACCOUNT_ID=123456789012 \
    BIBITES_ARTIFACT_BUCKET=fixture-artifacts \
    MOCK_SCENARIO="$scenario" MOCK_AWS_LOG="$test_root/aws.log" \
    MOCK_OBJECT_DIR="$object_dir" MOCK_REMOTE_MANIFEST="$remote_manifest" \
    MOCK_INVALID_MANIFEST="$invalid_manifest" \
    "$fixture_cloud/stage-artifacts.sh" --runtime-only
}

run_complete() {
  env PATH="$fixture_bin:$PATH" AWS_PROFILE=fixture AWS_REGION=us-east-1 \
    BIBITES_AWS_ACCOUNT_ID=123456789012 \
    BIBITES_ARTIFACT_BUCKET=fixture-artifacts \
    BIBITES_MANIFEST_FILE="$remote_manifest" BIBITES_SAVE_DIR="$save_dir" \
    MOCK_SCENARIO=complete MOCK_AWS_LOG="$test_root/aws.log" \
    MOCK_OBJECT_DIR="$object_dir" MOCK_REMOTE_MANIFEST="$remote_manifest" \
    MOCK_INVALID_MANIFEST="$invalid_manifest" \
    "$fixture_cloud/stage-artifacts.sh"
}

runtime_object="runtime/$runtime_sha256.tar.gz"
game_object="runtime-inputs/game/$game_sha256.zip"
bepinex_object="runtime-inputs/bepinex/$bepinex_sha256.zip"
manifest_object="worlds.$manifest_sha256.json"

# Runtime-only staging snapshots and validates the remote manifest, then
# publishes exactly four digest-derived objects.
: >"$test_root/aws.log"
runtime_output="$(run_runtime_only success)"
grep -Fq 'staged immutable runtime inputs' <<<"$runtime_output"
cmp -s "$test_root/worlds-before" "$fixture_dist/worlds.json"
cmp -s "$fixture_dist/$runtime_file" "$object_dir/cloud/v1/$runtime_object"
cmp -s "$fixture_dist/$game_file" "$object_dir/cloud/v1/$game_object"
cmp -s "$fixture_dist/$bepinex_file" "$object_dir/cloud/v1/$bepinex_object"
cmp -s "$remote_manifest" "$object_dir/cloud/v1/$manifest_object"
[ ! -e "$object_dir/cloud/v1/$game_file" ]
[ ! -e "$object_dir/cloud/v1/$bepinex_file" ]
[ ! -e "$object_dir/cloud/v1/worlds.json" ]
[ ! -e "$object_dir/cloud/v1/imports/Fixture.zip" ]
[ "$(wc -l <"$test_root/aws.log")" -eq 15 ]
[ "$(grep -Fc 'sts get-caller-identity' "$test_root/aws.log")" -eq 5 ]
[ "$(grep -Fc 's3api put-object' "$test_root/aws.log")" -eq 4 ]
[ "$(grep -Fc -- '--server-side-encryption AES256 --if-none-match *' \
  "$test_root/aws.log")" -eq 4 ]
grep -Fq 's3api head-bucket --bucket fixture-artifacts' "$test_root/aws.log"
grep -Fq "s3 cp s3://fixture-artifacts/cloud/v1/worlds.json" \
  "$test_root/aws.log"
if grep -Eq 's3 cp [^ ]+ s3://' "$test_root/aws.log"; then
  echo 'runtime-only staging used a mutable S3 copy upload' >&2
  exit 1
fi
if grep -Eq '(imports/Fixture|put-public-access-block|put-bucket-encryption)' \
  "$test_root/aws.log"; then
  echo 'runtime-only staging touched saves or bucket settings' >&2
  exit 1
fi
grep 's3api put-object' "$test_root/aws.log" |
  sed 's/.* --key \([^ ]*\) .*/\1/' >"$test_root/put-keys"
cat >"$test_root/expected-put-keys" <<EOF
cloud/v1/$runtime_object
cloud/v1/$game_object
cloud/v1/$bepinex_object
cloud/v1/$manifest_object
EOF
cmp -s "$test_root/expected-put-keys" "$test_root/put-keys"

# The atomic receipt names the exact bytes consumed by update-runtime.
cat >"$test_root/expected-staged.env" <<EOF
AWS_PROFILE=fixture
AWS_REGION=us-east-1
ARTIFACT_BUCKET=fixture-artifacts
ARTIFACT_PREFIX=cloud/v1
RUNTIME_OBJECT=$runtime_object
RUNTIME_SHA256=$runtime_sha256
GAME_OBJECT=$game_object
GAME_SHA256=$game_sha256
BEPINEX_OBJECT=$bepinex_object
BEPINEX_SHA256=$bepinex_sha256
MANIFEST_OBJECT=$manifest_object
MANIFEST_SHA256=$manifest_sha256
STAGING_SCOPE=runtime-only
EOF
cmp -s "$test_root/expected-staged.env" "$fixture_dist/staged.env"
[ "$(stat -c %a "$fixture_dist/staged.env")" = 600 ]

assert_stale_receipt_removed() {
  [ ! -e "$fixture_dist/staged.env" ] || {
    echo 'failed staging left an older runtime receipt available' >&2
    exit 1
  }
}

# A corrupt or missing local input fails before the first AWS request and
# invalidates any earlier receipt.
cp "$fixture_dist/$game_file" "$test_root/game-before"
printf 'corrupt local game\n' >"$fixture_dist/$game_file"
cp "$test_root/expected-staged.env" "$fixture_dist/staged.env"
: >"$test_root/aws.log"
set +e
corrupt_output="$(run_runtime_only success 2>&1)"
corrupt_status=$?
set -e
[ "$corrupt_status" -eq 1 ]
grep -Fq 'staged artifact digest does not match' <<<"$corrupt_output"
[ ! -s "$test_root/aws.log" ]
assert_stale_receipt_removed
mv "$test_root/game-before" "$fixture_dist/$game_file"

mv "$fixture_dist/$bepinex_file" "$test_root/bepinex-before"
cp "$test_root/expected-staged.env" "$fixture_dist/staged.env"
: >"$test_root/aws.log"
set +e
missing_input_output="$(run_runtime_only success 2>&1)"
missing_input_status=$?
set -e
[ "$missing_input_status" -eq 1 ]
grep -Fq 'missing staged artifact' <<<"$missing_input_output"
[ ! -s "$test_root/aws.log" ]
assert_stale_receipt_removed
mv "$test_root/bepinex-before" "$fixture_dist/$bepinex_file"

# A caller variable cannot fill a missing artifacts.env field.
cp "$fixture_dist/artifacts.env" "$test_root/complete-artifacts.env"
sed -i '/^RUNTIME_SHA256=/d' "$fixture_dist/artifacts.env"
cp "$test_root/expected-staged.env" "$fixture_dist/staged.env"
export RUNTIME_SHA256="$runtime_sha256"
: >"$test_root/aws.log"
set +e
missing_artifact_output="$(run_runtime_only success 2>&1)"
missing_artifact_status=$?
set -e
unset RUNTIME_SHA256
[ "$missing_artifact_status" -eq 1 ]
grep -Fq 'artifacts.env is missing RUNTIME_SHA256' <<<"$missing_artifact_output"
[ ! -s "$test_root/aws.log" ]
assert_stale_receipt_removed
cp "$test_root/complete-artifacts.env" "$fixture_dist/artifacts.env"

# A missing or invalid remote manifest fails before any immutable PUT and also
# invalidates a prior receipt.
for manifest_scenario in missing_manifest invalid_manifest; do
  cp "$test_root/expected-staged.env" "$fixture_dist/staged.env"
  : >"$test_root/aws.log"
  set +e
  manifest_output="$(run_runtime_only "$manifest_scenario" 2>&1)"
  manifest_status=$?
  set -e
  [ "$manifest_status" -eq 1 ]
  case "$manifest_scenario" in
    missing_manifest)
      grep -Fq 'cannot read the current staged world manifest' <<<"$manifest_output"
      ;;
    invalid_manifest)
      grep -Fq 'the current staged world manifest is invalid' <<<"$manifest_output"
      ;;
  esac
  if grep -Fq 's3api put-object' "$test_root/aws.log"; then
    echo 'manifest failure published an immutable input' >&2
    exit 1
  fi
  assert_stale_receipt_removed
done

# Runtime-only staging never creates or reconfigures an absent bucket.
cp "$test_root/expected-staged.env" "$fixture_dist/staged.env"
: >"$test_root/aws.log"
set +e
missing_bucket_output="$(run_runtime_only missing_bucket 2>&1)"
missing_bucket_status=$?
set -e
[ "$missing_bucket_status" -ne 0 ]
grep -Fq '404 Not Found' <<<"$missing_bucket_output"
if grep -Eq '(put-object|create-bucket|put-public-access-block|put-bucket-encryption)' \
  "$test_root/aws.log"; then
  echo 'runtime-only staging mutated an absent bucket' >&2
  exit 1
fi
assert_stale_receipt_removed

# Default staging retains the complete manifest/save transaction and marks its
# receipt so update-runtime cannot mistake it for an immutable input snapshot.
rm -f "$object_dir/cloud/v1/$manifest_object"
: >"$test_root/aws.log"
complete_output="$(run_complete)"
grep -Fq 'staged private artifacts' <<<"$complete_output"
grep -Fq 'put-public-access-block' "$test_root/aws.log"
grep -Fq 'put-bucket-encryption' "$test_root/aws.log"
cmp -s "$fixture_dist/$game_file" "$object_dir/cloud/v1/$game_file"
cmp -s "$fixture_dist/$bepinex_file" "$object_dir/cloud/v1/$bepinex_file"
cmp -s "$remote_manifest" "$object_dir/cloud/v1/worlds.json"
cmp -s "$remote_manifest" "$object_dir/cloud/v1/$manifest_object"
cmp -s "$save_dir/Fixture.zip" "$object_dir/cloud/v1/imports/Fixture.zip"
cat >"$test_root/expected-complete-staged.env" <<EOF
AWS_PROFILE=fixture
AWS_REGION=us-east-1
ARTIFACT_BUCKET=fixture-artifacts
ARTIFACT_PREFIX=cloud/v1
RUNTIME_OBJECT=$runtime_object
STAGED_RUNTIME_SHA256=$runtime_sha256
STAGED_GAME_SHA256=$game_sha256
STAGED_BEPINEX_SHA256=$bepinex_sha256
MANIFEST_OBJECT=$manifest_object
MANIFEST_SHA256=$manifest_sha256
STAGING_SCOPE=complete
EOF
cmp -s "$test_root/expected-complete-staged.env" "$fixture_dist/staged.env"
[ "$(stat -c %a "$fixture_dist/staged.env")" = 600 ]

printf 'runtime-only staging fixtures passed\n'
