#!/usr/bin/env bash
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
transaction="$repo/cloud/aws/runtime/bibites-update-runtime-transaction"
activator="$repo/cloud/aws/runtime/bibites-activate-runtime"
validator="$repo/cloud/aws/runtime/validate-world-manifest.jq"
test_root="$(mktemp -d)"
trap 'find "$test_root" -depth -delete' EXIT

mock_bin="$test_root/bin"
object_store="$test_root/objects"
runtime_root="$test_root/bibites-runtime"
lock_file="$test_root/runtime.lock"
cloud_conf="$test_root/bibites-cloud.conf"
pointer_state="$test_root/current.json"
etag_state="$test_root/current.etag"
aws_log="$test_root/aws.log"
action_log="$test_root/actions.log"
activation_entered="$test_root/activation-entered"
activation_release="$test_root/activation-release"
candidate_active="$test_root/candidate-active"
prior_install_failure_marker="$test_root/prior-install-failed-once"
install -d "$mock_bin" "$object_store/cloud/v1/runtime" \
  "$object_store/cloud/v1/runtime-inputs/game" \
  "$object_store/cloud/v1/runtime-inputs/bepinex"

access_key_id=ASIAABCDEFGHIJKLMNOP
secret_access_key=ssssssssssssssssssssssssssssssssssssssss
session_token=tttttttttttttttttttttttttttttttt
credential_prefix=/bibites/cloud/runtime-pointer/fixture
initial_pointer_etag='"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"'
third_pointer_sha=eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee
game_snapshot="$test_root/game.zip"
bepinex_snapshot="$test_root/bepinex.zip"
printf 'pinned game fixture\n' >"$game_snapshot"
printf 'pinned BepInEx fixture\n' >"$bepinex_snapshot"
game_sha="$(sha256sum "$game_snapshot" | awk '{print $1}')"
bepinex_sha="$(sha256sum "$bepinex_snapshot" | awk '{print $1}')"
game_object="runtime-inputs/game/$game_sha.zip"
bepinex_object="runtime-inputs/bepinex/$bepinex_sha.zip"
cp "$game_snapshot" "$object_store/cloud/v1/$game_object"
cp "$bepinex_snapshot" "$object_store/cloud/v1/$bepinex_object"

manifest="$test_root/worlds.json"
cat >"$manifest" <<'JSON'
{"schema":1,"worlds":[{"id":"slot-1","peerId":"slot-1-fixture",
"worldName":"Fixture-World","sidecarPort":8787,
"saveKey":"imports/Fixture-World.zip",
"credentialParameter":"/bibites/cloud/slot-1/peer-secret",
"position":"0,0","preferredSlot":1,"targetTimeScale":1,
"saveMinutes":10,"saveKeep":6,"enabled":true}]}
JSON
manifest_sha="$(sha256sum "$manifest" | awk '{print $1}')"
manifest_object="worlds.$manifest_sha.json"
cp "$manifest" "$object_store/cloud/v1/$manifest_object"

cat >"$mock_bin/systemctl" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
if [ "${1:-}" = list-units ]; then exit 0; fi
exit 0
MOCK
chmod 0755 "$mock_bin/systemctl"

cat >"$mock_bin/mv" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
destination="${!#}"
if { [ "$MOCK_SCENARIO" = restore_once ] ||
     [ "$MOCK_SCENARIO" = candidate_21_recovered ]; } &&
   [ "$destination" = "$MOCK_CLOUD_CONF" ] &&
   [ ! -e "$MOCK_RESTORE_FAILURE_MARKER" ]; then
  : >"$MOCK_RESTORE_FAILURE_MARKER"
  exit 1
fi
exec /usr/bin/mv "$@"
MOCK
chmod 0755 "$mock_bin/mv"

cat >"$mock_bin/env" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
case " $* " in
  *"$MOCK_SECRET_ACCESS_KEY"*|*"$MOCK_SESSION_TOKEN"*)
    echo 'pointer credential entered an external env command argument' >&2
    exit 66
    ;;
esac
exec /usr/bin/env "$@"
MOCK
chmod 0755 "$mock_bin/env"

make_runtime_tree() {
  local tree="$1" label="$2"
  install -d "$tree"
  printf '%s\n' "$label" >"$tree/label"
  cp "$activator" "$tree/bibites-activate-runtime"
  cp "$validator" "$tree/validate-world-manifest.jq"
  cp "$transaction" "$tree/bibites-update-runtime-transaction"
  cat >"$tree/install-host" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
label="$(<"$here/label")"
printf '%s-install\n' "$label" >>"$MOCK_ACTION_LOG"
if [ "$label" = A ] && [ ! -e "$MOCK_ACTIVATION_RELEASE" ]; then
  : >"$MOCK_ACTIVATION_ENTERED"
  while [ ! -e "$MOCK_ACTIVATION_RELEASE" ]; do sleep 0.02; done
fi
if [ "$label" != P ]; then : >"$MOCK_CANDIDATE_ACTIVE"; fi
{
  printf 'AWS_REGION=%q\n' "$AWS_REGION"
  printf 'ARTIFACT_BUCKET=%q\n' "$ARTIFACT_BUCKET"
  printf 'MANIFEST_KEY=%q\n' "$MANIFEST_KEY"
  printf 'RELAY_URL=%q\n' "wss://$RELAY_DOMAIN/contract-b/v4"
} >"$BIBITES_CLOUD_CONF"
if [ "$MOCK_SCENARIO" = candidate_21_recovered ]; then
  if [ "$label" = H ]; then exit 7; fi
  if [ "$label" = P ] &&
     [ ! -e "$MOCK_PRIOR_INSTALL_FAILURE_MARKER" ]; then
    : >"$MOCK_PRIOR_INSTALL_FAILURE_MARKER"
    exit 9
  fi
fi
if [ "$MOCK_SCENARIO" = rollback_failure ] &&
   [ "$label" = P ] && [ -e "$MOCK_CANDIDATE_ACTIVE" ]; then
  exit 9
fi
MOCK
  cat >"$tree/bibites-stop-worlds" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
printf '%s-stop\n' "$(<"$here/label")" >>"$MOCK_ACTION_LOG"
MOCK
  chmod 0755 "$tree/install-host" "$tree/bibites-stop-worlds" \
    "$tree/bibites-activate-runtime" \
    "$tree/bibites-update-runtime-transaction"
}

make_runtime_archive() {
  local label="$1" tree="$2" archive="$3"
  make_runtime_tree "$tree" "$label"
  tar -czf "$archive" -C "$tree" .
  archive_sha="$(sha256sum "$archive" | awk '{print $1}')"
  archive_object="runtime/$archive_sha.tar.gz"
  cp "$archive" "$object_store/cloud/v1/$archive_object"
}

cat >"$mock_bin/aws" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
args=("$@")
joined=" $* "
printf '%s\t%s\n' "$TX_LABEL" "$*" >>"$MOCK_AWS_LOG"
case "$joined" in
  *' sts get-caller-identity '*)
    if [ -n "${AWS_ACCESS_KEY_ID:-}" ]; then
      [ "$AWS_ACCESS_KEY_ID" = "$MOCK_ACCESS_KEY_ID" ] || exit 64
      [ "$MOCK_SCENARIO" != wrong_pointer_account ] || {
        printf '999999999999\n'
        exit 0
      }
    elif [ "$MOCK_SCENARIO" = wrong_instance_account ]; then
      printf '999999999999\n'
      exit 0
    fi
    printf '123456789012\n'
    ;;
  *' ssm get-parameter '*)
    name=
    for ((index=0; index < ${#args[@]}; index++)); do
      if [ "${args[$index]}" = --name ]; then name="${args[$((index + 1))]}"; break; fi
    done
    case "$name" in
      "$MOCK_CREDENTIAL_PREFIX/access-key-id") value="$MOCK_ACCESS_KEY_ID" ;;
      "$MOCK_CREDENTIAL_PREFIX/secret-access-key")
        [ "$MOCK_SCENARIO" != missing_credentials ] || exit 254
        value="$MOCK_SECRET_ACCESS_KEY"
        ;;
      "$MOCK_CREDENTIAL_PREFIX/session-token") value="$MOCK_SESSION_TOKEN" ;;
      *) exit 64 ;;
    esac
    jq -nc --arg name "$name" --arg value "$value" \
      '{Parameter:{Name:$name,Type:"SecureString",Value:$value}}'
    ;;
  *' s3api put-object --generate-cli-skeleton input '*)
    [ "${AWS_ACCESS_KEY_ID:-}" = "$MOCK_ACCESS_KEY_ID" ] || exit 64
    if [ "$MOCK_SCENARIO" = missing_if_match_model ]; then
      printf '%s\n' '{"ServerSideEncryption":""}'
    else
      printf '%s\n' '{"IfMatch":"","ServerSideEncryption":""}'
    fi
    ;;
  *' s3api get-object '*)
    if [ -n "${AWS_ACCESS_KEY_ID:-}" ]; then
      [ "$AWS_ACCESS_KEY_ID" = "$MOCK_ACCESS_KEY_ID" ] || exit 64
      if [ "$MOCK_SCENARIO" = expired_after_activation ] &&
         [ -e "$MOCK_CANDIDATE_ACTIVE" ]; then
        echo 'ExpiredToken' >&2
        exit 254
      fi
    else
      [ "$MOCK_SCENARIO" = expired_after_activation ] &&
        [ -e "$MOCK_CANDIDATE_ACTIVE" ] || exit 64
    fi
    destination=
    key=
    for ((index=0; index < ${#args[@]}; index++)); do
      case "${args[$index]}" in
        --key) key="${args[$((index + 1))]}" ;;
        get-object)
          for ((scan=index + 1; scan < ${#args[@]}; scan++)); do
            case "${args[$scan]}" in
              --bucket|--key|--output) scan=$((scan + 1)) ;;
              --*) ;;
              *) destination="${args[$scan]}" ;;
            esac
          done
          ;;
      esac
    done
    [ "$key" = cloud/v1/runtime/current.json ] && [ -n "$destination" ] || exit 64
    cp "$MOCK_POINTER_STATE" "$destination"
    jq -nc --arg etag "$(<"$MOCK_ETAG_STATE")" '{ETag:$etag}'
    ;;
  *' s3api put-object '*)
    [ "${AWS_ACCESS_KEY_ID:-}" = "$MOCK_ACCESS_KEY_ID" ] || exit 64
    body=
    match=
    for ((index=0; index < ${#args[@]}; index++)); do
      case "${args[$index]}" in
        --body) body="${args[$((index + 1))]}" ;;
        --if-match) match="${args[$((index + 1))]}" ;;
      esac
    done
    [[ "$joined" == *' --server-side-encryption AES256 '* ]] || exit 64
    if [ "$MOCK_SCENARIO" = rollback ] ||
       [ "$MOCK_SCENARIO" = rollback_failure ] ||
       [ "$MOCK_SCENARIO" = expired_after_activation ]; then
      echo 'PreconditionFailed: 412' >&2
      exit 254
    fi
    [ "$match" = "$(<"$MOCK_ETAG_STATE")" ] || {
      echo 'PreconditionFailed: 412' >&2
      exit 254
    }
    if [ "$MOCK_SCENARIO" = put_response_lost ]; then
      cp "$body" "$MOCK_POINTER_STATE"
      printf '%s\n' '"ffffffffffffffffffffffffffffffff"' >"$MOCK_ETAG_STATE"
      echo 'connection closed after pointer publication' >&2
      exit 254
    fi
    if [ "$MOCK_SCENARIO" = third_pointer ]; then
      jq -cn --arg sha "$MOCK_THIRD_POINTER_SHA" \
        '{schema:1,runtimeFile:("runtime/" + $sha + ".tar.gz"),runtimeSha256:$sha}' \
        >"$MOCK_POINTER_STATE"
      printf '%s\n' '"eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"' >"$MOCK_ETAG_STATE"
      echo 'PreconditionFailed: 412' >&2
      exit 254
    fi
    cp "$body" "$MOCK_POINTER_STATE"
    printf '%s\n' '"ffffffffffffffffffffffffffffffff"' >"$MOCK_ETAG_STATE"
    printf '%s\n' '{}'
    ;;
  *' s3 cp '*)
    [ -z "${AWS_ACCESS_KEY_ID:-}" ] || exit 64
    source=
    destination=
    for ((index=0; index < ${#args[@]}; index++)); do
      if [ "${args[$index]}" = cp ]; then
        source="${args[$((index + 1))]}"
        destination="${args[$((index + 2))]}"
        break
      fi
    done
    key="${source#s3://fixture-artifacts/}"
    [ "$key" != "$source" ] || exit 64
    if { [ "$MOCK_SCENARIO" = rollback ] ||
         [ "$MOCK_SCENARIO" = expired_after_activation ]; } &&
       [ -e "$MOCK_CANDIDATE_ACTIVE" ] &&
       [[ "$key" == cloud/v1/runtime/* ]]; then
      echo 'retained prior object is no longer readable' >&2
      exit 254
    fi
    cp "$MOCK_OBJECT_STORE/$key" "$destination"
    if [ "$MOCK_SCENARIO" = corrupt_game ] &&
       [ "$key" = "$MOCK_GAME_KEY" ]; then
      printf 'corrupt game fixture\n' >"$destination"
    fi
    ;;
  *)
    echo "unexpected AWS fixture call: $*" >&2
    exit 64
    ;;
esac
MOCK
chmod 0755 "$mock_bin/aws"

write_pointer() {
  local sha="$1"
  jq -cn --arg sha "$sha" \
    '{schema:1,runtimeFile:("runtime/" + $sha + ".tar.gz"),runtimeSha256:$sha}' \
    >"$pointer_state"
  printf '%s\n' "$initial_pointer_etag" >"$etag_state"
}

write_cloud_conf() {
  cat >"$cloud_conf" <<'EOF'
AWS_REGION=us-east-1
ARTIFACT_BUCKET=fixture-artifacts
MANIFEST_KEY=cloud/v1/worlds.json
RELAY_URL=wss://relay.example.test/contract-b/v4
EOF
}

run_transaction() {
  local label="$1" tree="$2" archive="$3" sha="$4" scenario="$5"
  local expected_prior_object="${6:-runtime/$prior_sha.tar.gz}"
  local expected_prior_sha="${7:-$prior_sha}"
  local expected_prior_etag="${8:-$initial_pointer_etag}"
  env PATH="$mock_bin:$PATH" TX_LABEL="$label" MOCK_SCENARIO="$scenario" \
    MOCK_AWS_LOG="$aws_log" MOCK_OBJECT_STORE="$object_store" \
    MOCK_POINTER_STATE="$pointer_state" MOCK_ETAG_STATE="$etag_state" \
    MOCK_CREDENTIAL_PREFIX="$credential_prefix" \
    MOCK_ACCESS_KEY_ID="$access_key_id" \
    MOCK_SECRET_ACCESS_KEY="$secret_access_key" \
    MOCK_SESSION_TOKEN="$session_token" \
    MOCK_THIRD_POINTER_SHA="$third_pointer_sha" \
    MOCK_ACTION_LOG="$action_log" \
    MOCK_ACTIVATION_ENTERED="$activation_entered" \
    MOCK_ACTIVATION_RELEASE="$activation_release" \
    MOCK_CANDIDATE_ACTIVE="$candidate_active" \
    MOCK_GAME_KEY="cloud/v1/$game_object" \
    MOCK_CLOUD_CONF="$cloud_conf" \
    MOCK_RESTORE_FAILURE_MARKER="$test_root/restore-failed-once" \
    MOCK_PRIOR_INSTALL_FAILURE_MARKER="$prior_install_failure_marker" \
    BIBITES_RUNTIME_TEST_ROOT="$runtime_root" \
    BIBITES_RUNTIME_LOCK_FILE="$lock_file" \
    BIBITES_RUNTIME_LOCK_WAIT_SECONDS="${TEST_LOCK_WAIT_SECONDS:-10}" \
    BIBITES_CLOUD_CONF="$cloud_conf" \
    "$transaction" "$tree" "$archive" us-east-1 123456789012 \
      vol-0123456789abcdef0 fixture-artifacts cloud/v1 \
      "runtime/$sha.tar.gz" "$sha" "$game_object" "$game_sha" \
      "$bepinex_object" "$bepinex_sha" "$manifest_object" "$manifest_sha" \
      "$credential_prefix" 10.0.0.5 relay.example.test "$runtime_root" \
      "$expected_prior_object" "$expected_prior_sha" "$expected_prior_etag"
}

wait_for_file() {
  local path="$1"
  for _ in $(seq 1 200); do
    [ -e "$path" ] && return 0
    sleep 0.02
  done
  echo "timed out waiting for $path" >&2
  return 1
}

# Build the common prior runtime and two independent candidates.
prior_tree="$test_root/prior-source"
prior_archive="$test_root/prior.tar.gz"
make_runtime_archive P "$prior_tree" "$prior_archive"
prior_sha="$archive_sha"

candidate_a_tree="$test_root/candidate-a"
candidate_a_archive="$test_root/candidate-a.tar.gz"
make_runtime_archive A "$candidate_a_tree" "$candidate_a_archive"
candidate_a_sha="$archive_sha"

candidate_b_tree="$test_root/candidate-b"
candidate_b_archive="$test_root/candidate-b.tar.gz"
make_runtime_archive B "$candidate_b_tree" "$candidate_b_archive"
candidate_b_sha="$archive_sha"

cp -a "$prior_tree" "$runtime_root"
write_pointer "$prior_sha"
write_cloud_conf
: >"$aws_log"
: >"$action_log"
: >"$activation_release.tmp"
rm -f "$activation_release.tmp" "$activation_entered" "$activation_release" \
  "$candidate_active"

run_transaction A "$candidate_a_tree" "$candidate_a_archive" \
  "$candidate_a_sha" normal >"$test_root/a.out" 2>&1 &
pid_a=$!
if ! wait_for_file "$activation_entered"; then
  cat "$test_root/a.out" >&2
  wait "$pid_a" || true
  exit 1
fi
run_transaction B "$candidate_b_tree" "$candidate_b_archive" \
  "$candidate_b_sha" normal >"$test_root/b.out" 2>&1 &
pid_b=$!

# B can validate local inputs, but it must not snapshot the pointer or touch the
# host while A holds the transaction lock.
sleep 0.2
if grep -q '^B[[:space:]]' "$aws_log"; then
  echo 'second transaction passed the host lock before the first completed' >&2
  exit 1
fi
if grep -Fq B-install "$action_log"; then
  echo 'second transaction activated before the first completed' >&2
  exit 1
fi

: >"$activation_release"
wait "$pid_a"
set +e
wait "$pid_b"
stale_b_status=$?
set -e
[ "$stale_b_status" -eq 26 ] || {
  echo "stale serialized transaction returned $stale_b_status instead of 26" >&2
  cat "$test_root/b.out" >&2
  exit 1
}
[ "$(<"$runtime_root/label")" = A ] || {
  echo 'a stale serialized transaction changed the active runtime' >&2
  exit 1
}
[ "$(jq -r .runtimeSha256 "$pointer_state")" = "$candidate_a_sha" ] || {
  echo 'a stale serialized transaction changed the runtime pointer' >&2
  exit 1
}
grep -q '^B[[:space:]].*s3api get-object' "$aws_log"
if grep -Eq '^B-(stop|install)$' "$action_log"; then
  echo 'a stale serialized transaction reached service mutation' >&2
  exit 1
fi
if grep -q '^B[[:space:]].*s3api put-object --bucket' "$aws_log"; then
  echo 'a stale serialized transaction attempted pointer publication' >&2
  exit 1
fi
grep -Fq 'locked pointer does not match the expected prior runtime object' \
  "$test_root/b.out"
[ "$(grep -c '^MANIFEST_KEY=' "$cloud_conf")" -eq 1 ]
grep -Fxq 'MANIFEST_KEY=cloud/v1/worlds.json' "$cloud_conf"

# A locked preimage mismatch is status 26. It cannot stop a service, install a
# runtime, rewrite the host configuration, or publish the pointer.
for prior_mismatch in object_sha etag; do
  rm -rf "$runtime_root"
  cp -a "$prior_tree" "$runtime_root"
  write_pointer "$prior_sha"
  write_cloud_conf
  cp "$pointer_state" "$test_root/$prior_mismatch-pointer-before"
  cp "$etag_state" "$test_root/$prior_mismatch-etag-before"
  cp "$cloud_conf" "$test_root/$prior_mismatch-cloud-conf-before"
  : >"$aws_log"
  : >"$action_log"
  rm -f "$candidate_active"
  case "$prior_mismatch" in
    object_sha)
      mismatch_object="runtime/$candidate_a_sha.tar.gz"
      mismatch_sha="$candidate_a_sha"
      mismatch_etag="$initial_pointer_etag"
      ;;
    etag)
      mismatch_object="runtime/$prior_sha.tar.gz"
      mismatch_sha="$prior_sha"
      mismatch_etag='"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"'
      ;;
  esac
  set +e
  mismatch_output="$(run_transaction B "$candidate_b_tree" \
    "$candidate_b_archive" "$candidate_b_sha" normal \
    "$mismatch_object" "$mismatch_sha" "$mismatch_etag" 2>&1)"
  mismatch_status=$?
  set -e
  [ "$mismatch_status" -eq 26 ] || {
    echo "$prior_mismatch mismatch returned $mismatch_status instead of 26" >&2
    echo "$mismatch_output" >&2
    exit 1
  }
  [ "$(<"$runtime_root/label")" = P ]
  cmp -s "$test_root/$prior_mismatch-pointer-before" "$pointer_state"
  cmp -s "$test_root/$prior_mismatch-etag-before" "$etag_state"
  cmp -s "$test_root/$prior_mismatch-cloud-conf-before" "$cloud_conf"
  [ ! -s "$action_log" ] || {
    echo "$prior_mismatch mismatch reached service mutation" >&2
    exit 1
  }
  if grep -Fq 's3api put-object --bucket' "$aws_log"; then
    echo "$prior_mismatch mismatch attempted pointer publication" >&2
    exit 1
  fi
  case "$prior_mismatch" in
    object_sha)
      grep -Fq 'expected prior runtime object' <<<"$mismatch_output"
      grep -Fq 'expected prior runtime SHA-256' <<<"$mismatch_output"
      ;;
    etag)
      grep -Fq 'expected prior ETag' <<<"$mismatch_output"
      ;;
  esac
done

# The remote transaction rejects malformed expected-prior inputs before AWS or
# service access. The local wrapper applies the same gate before SSM dispatch.
for malformed_prior in object sha256 etag relationship; do
  case "$malformed_prior" in
    object)
      malformed_object='../unsafe-runtime.tar.gz'
      malformed_sha="$prior_sha"
      malformed_etag="$initial_pointer_etag"
      ;;
    sha256)
      malformed_object=runtime/abcd.tar.gz
      malformed_sha=abcd
      malformed_etag="$initial_pointer_etag"
      ;;
    etag)
      malformed_object="runtime/$prior_sha.tar.gz"
      malformed_sha="$prior_sha"
      malformed_etag=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
      ;;
    relationship)
      malformed_object="runtime/$candidate_a_sha.tar.gz"
      malformed_sha="$prior_sha"
      malformed_etag="$initial_pointer_etag"
      ;;
  esac
  : >"$aws_log"
  : >"$action_log"
  set +e
  malformed_output="$(run_transaction B "$candidate_b_tree" \
    "$candidate_b_archive" "$candidate_b_sha" normal \
    "$malformed_object" "$malformed_sha" "$malformed_etag" 2>&1)"
  malformed_status=$?
  set -e
  [ "$malformed_status" -eq 2 ] || {
    echo "$malformed_prior input returned $malformed_status instead of 2" >&2
    echo "$malformed_output" >&2
    exit 1
  }
  [ ! -s "$aws_log" ] || {
    echo "$malformed_prior input reached AWS" >&2
    exit 1
  }
  [ ! -s "$action_log" ] || {
    echo "$malformed_prior input reached service mutation" >&2
    exit 1
  }
done

# A failed CAS after candidate activation must use the already-retained prior
# archive. The mock rejects any attempt to fetch that runtime again.
rm -rf "$runtime_root"
cp -a "$prior_tree" "$runtime_root"
rollback_tree="$test_root/rollback-candidate"
rollback_archive="$test_root/rollback-candidate.tar.gz"
make_runtime_archive C "$rollback_tree" "$rollback_archive"
rollback_sha="$archive_sha"
write_pointer "$prior_sha"
write_cloud_conf
: >"$aws_log"
: >"$action_log"
rm -f "$candidate_active"
set +e
rollback_output="$(run_transaction C "$rollback_tree" "$rollback_archive" \
  "$rollback_sha" rollback 2>&1)"
rollback_status=$?
set -e
[ "$rollback_status" -eq 22 ] || {
  echo "retained-prior rollback returned $rollback_status instead of 22" >&2
  echo "$rollback_output" >&2
  exit 1
}
[ "$(<"$runtime_root/label")" = P ]
[ "$(jq -r .runtimeSha256 "$pointer_state")" = "$prior_sha" ]
[ "$(grep -Fc "s3://fixture-artifacts/cloud/v1/runtime/$prior_sha.tar.gz" \
  "$aws_log")" -eq 1 ] || {
  echo 'rollback fetched the prior archive again after activation' >&2
  exit 1
}
grep -Fq 'retained prior runtime is active again' <<<"$rollback_output"
grep -Fxq 'MANIFEST_KEY=cloud/v1/worlds.json' "$cloud_conf"

# If the pointer write completed but its response was lost, the locked reread
# identifies the active candidate and completes the transaction successfully.
rm -rf "$runtime_root"
cp -a "$prior_tree" "$runtime_root"
lost_tree="$test_root/lost-response-candidate"
lost_archive="$test_root/lost-response-candidate.tar.gz"
make_runtime_archive J "$lost_tree" "$lost_archive"
lost_sha="$archive_sha"
write_pointer "$prior_sha"
write_cloud_conf
: >"$aws_log"
: >"$action_log"
rm -f "$candidate_active"
lost_output="$(run_transaction J "$lost_tree" "$lost_archive" \
  "$lost_sha" put_response_lost 2>&1)"
[ "$(<"$runtime_root/label")" = J ]
[ "$(jq -r .runtimeSha256 "$pointer_state")" = "$lost_sha" ]
grep -Fq 'pointer identifies the active candidate despite the publication error' \
  <<<"$lost_output"
grep -Fxq 'MANIFEST_KEY=cloud/v1/worlds.json' "$cloud_conf"

# A third valid pointer is concurrent state. Keep it and the active candidate
# for explicit reconciliation; do not overwrite it or roll back across it.
rm -rf "$runtime_root"
cp -a "$prior_tree" "$runtime_root"
third_tree="$test_root/third-pointer-candidate"
third_archive="$test_root/third-pointer-candidate.tar.gz"
make_runtime_archive K "$third_tree" "$third_archive"
third_sha="$archive_sha"
write_pointer "$prior_sha"
write_cloud_conf
: >"$aws_log"
: >"$action_log"
rm -f "$candidate_active"
set +e
third_output="$(run_transaction K "$third_tree" "$third_archive" \
  "$third_sha" third_pointer 2>&1)"
third_status=$?
set -e
[ "$third_status" -eq 23 ] || {
  echo "third-pointer conflict returned $third_status instead of 23" >&2
  echo "$third_output" >&2
  exit 1
}
[ "$(<"$runtime_root/label")" = K ]
[ "$(jq -r .runtimeSha256 "$pointer_state")" = "$third_pointer_sha" ]
if grep -Fq P-install "$action_log"; then
  echo 'third-pointer conflict rolled back over concurrent state' >&2
  exit 1
fi
grep -Fq 'pointer changed concurrently' <<<"$third_output"
grep -Fxq 'MANIFEST_KEY=cloud/v1/worlds.json' "$cloud_conf"

# If the prior reactivation itself fails after a CAS failure, report the
# distinct rollback-failed status and leave the pointer untouched.
rm -rf "$runtime_root"
cp -a "$prior_tree" "$runtime_root"
rollback_failure_tree="$test_root/rollback-failure-candidate"
rollback_failure_archive="$test_root/rollback-failure-candidate.tar.gz"
make_runtime_archive L "$rollback_failure_tree" "$rollback_failure_archive"
rollback_failure_sha="$archive_sha"
write_pointer "$prior_sha"
write_cloud_conf
: >"$aws_log"
: >"$action_log"
rm -f "$candidate_active"
set +e
rollback_failure_output="$(run_transaction L "$rollback_failure_tree" \
  "$rollback_failure_archive" "$rollback_failure_sha" rollback_failure 2>&1)"
rollback_failure_status=$?
set -e
[ "$rollback_failure_status" -eq 24 ] || {
  echo "rollback failure returned $rollback_failure_status instead of 24" >&2
  echo "$rollback_failure_output" >&2
  exit 1
}
[ "$(<"$runtime_root/label")" = L ]
[ "$(jq -r .runtimeSha256 "$pointer_state")" = "$prior_sha" ]
grep -Fq 'prior runtime reactivation also failed' <<<"$rollback_failure_output"
grep -Fxq 'MANIFEST_KEY=cloud/v1/worlds.json' "$cloud_conf"

# If the one-hour pointer session expires after activation, the read-only
# instance role still classifies the pointer and permits a retained rollback.
rm -rf "$runtime_root"
cp -a "$prior_tree" "$runtime_root"
expiry_tree="$test_root/expiry-candidate"
expiry_archive="$test_root/expiry-candidate.tar.gz"
make_runtime_archive G "$expiry_tree" "$expiry_archive"
expiry_sha="$archive_sha"
write_pointer "$prior_sha"
write_cloud_conf
: >"$aws_log"
: >"$action_log"
rm -f "$candidate_active"
set +e
expiry_output="$(run_transaction G "$expiry_tree" "$expiry_archive" \
  "$expiry_sha" expired_after_activation 2>&1)"
expiry_status=$?
set -e
[ "$expiry_status" -eq 22 ] || {
  echo "expired-session recovery returned $expiry_status instead of 22" >&2
  echo "$expiry_output" >&2
  exit 1
}
[ "$(<"$runtime_root/label")" = P ]
[ "$(jq -r .runtimeSha256 "$pointer_state")" = "$prior_sha" ]
[ "$(grep -Fc 's3api get-object' "$aws_log")" -eq 3 ] || {
  echo 'expired pointer session did not use one instance-role classification read' >&2
  exit 1
}

# The ordinary manifest must be restored before pointer publication. If that
# first restore fails, the transaction restores the retained prior instead of
# publishing a candidate with a pinned future-sync configuration.
rm -rf "$runtime_root"
cp -a "$prior_tree" "$runtime_root"
restore_tree="$test_root/restore-candidate"
restore_archive="$test_root/restore-candidate.tar.gz"
make_runtime_archive E "$restore_tree" "$restore_archive"
restore_sha="$archive_sha"
write_pointer "$prior_sha"
write_cloud_conf
: >"$aws_log"
: >"$action_log"
rm -f "$test_root/restore-failed-once" "$candidate_active"
set +e
restore_output="$(run_transaction E "$restore_tree" "$restore_archive" \
  "$restore_sha" restore_once 2>&1)"
restore_status=$?
set -e
[ "$restore_status" -eq 22 ] || {
  echo "manifest-restore recovery returned $restore_status instead of 22" >&2
  echo "$restore_output" >&2
  exit 1
}
[ "$(<"$runtime_root/label")" = P ]
[ "$(jq -r .runtimeSha256 "$pointer_state")" = "$prior_sha" ]
if grep -Fq 's3api put-object --bucket' "$aws_log"; then
  echo 'candidate pointer published before ordinary-manifest restoration' >&2
  exit 1
fi
grep -Fq 'stopped before pointer publication' <<<"$restore_output"
grep -Fxq 'MANIFEST_KEY=cloud/v1/worlds.json' "$cloud_conf"

# If the candidate activator reports that its internal rollback failed, a
# successful retained-prior recovery still means activation failed with a
# complete rollback. No pointer publication was attempted.
rm -rf "$runtime_root"
cp -a "$prior_tree" "$runtime_root"
recovered_tree="$test_root/recovered-candidate"
recovered_archive="$test_root/recovered-candidate.tar.gz"
make_runtime_archive H "$recovered_tree" "$recovered_archive"
recovered_sha="$archive_sha"
write_pointer "$prior_sha"
write_cloud_conf
: >"$aws_log"
: >"$action_log"
rm -f "$test_root/restore-failed-once" "$prior_install_failure_marker" \
  "$candidate_active"
set +e
recovered_output="$(run_transaction H "$recovered_tree" \
  "$recovered_archive" "$recovered_sha" candidate_21_recovered 2>&1)"
recovered_status=$?
set -e
[ "$recovered_status" -eq 20 ] || {
  echo "recovered activator rollback returned $recovered_status instead of 20" >&2
  echo "$recovered_output" >&2
  exit 1
}
[ "$(<"$runtime_root/label")" = P ]
[ "$(jq -r .runtimeSha256 "$pointer_state")" = "$prior_sha" ]
if grep -Fq 's3api put-object --bucket' "$aws_log"; then
  echo 'recovered activation failure attempted pointer publication' >&2
  exit 1
fi
grep -Fq 'candidate activation failed; the prior runtime and ordinary manifest are restored' \
  <<<"$recovered_output"
grep -Fxq 'MANIFEST_KEY=cloud/v1/worlds.json' "$cloud_conf"

# An activator preflight failure before service stop must leave the runtime,
# action log, and already-ordinary cloud configuration untouched. A leftover
# previous-runtime directory is an activator preflight gate.
rm -rf "$runtime_root" "$runtime_root.previous"
cp -a "$prior_tree" "$runtime_root"
cp -a "$prior_tree" "$runtime_root.previous"
prestop_tree="$test_root/prestop-candidate"
prestop_archive="$test_root/prestop-candidate.tar.gz"
make_runtime_archive I "$prestop_tree" "$prestop_archive"
prestop_sha="$archive_sha"
write_pointer "$prior_sha"
write_cloud_conf
cp -a "$runtime_root" "$test_root/prestop-runtime-before"
cp "$cloud_conf" "$test_root/prestop-cloud-conf-before"
cloud_conf_inode="$(stat -c %i "$cloud_conf")"
: >"$aws_log"
: >"$action_log"
set +e
prestop_output="$(run_transaction I "$prestop_tree" "$prestop_archive" \
  "$prestop_sha" normal 2>&1)"
prestop_status=$?
set -e
[ "$prestop_status" -ne 0 ]
grep -Fq 'a previous runtime transaction is unresolved' <<<"$prestop_output"
diff -r "$test_root/prestop-runtime-before" "$runtime_root"
cmp -s "$test_root/prestop-cloud-conf-before" "$cloud_conf"
[ "$(stat -c %i "$cloud_conf")" = "$cloud_conf_inode" ] || {
  echo 'activator preflight failure rewrote the ordinary cloud configuration' >&2
  exit 1
}
[ ! -s "$action_log" ] || {
  echo 'activator preflight failure reached service stop or installation' >&2
  exit 1
}
if grep -Fq 's3api put-object --bucket' "$aws_log"; then
  echo 'activator preflight failure attempted pointer publication' >&2
  exit 1
fi
rm -rf "$runtime_root.previous"

# The transaction accepts only the ordinary manifest pointer as its initial
# configuration. A pinned or otherwise malformed starting state is unresolved
# host state, not something this update may silently rewrite.
rm -rf "$runtime_root"
cp -a "$prior_tree" "$runtime_root"
write_pointer "$prior_sha"
write_cloud_conf
sed -i "s#MANIFEST_KEY=cloud/v1/worlds.json#MANIFEST_KEY=cloud/v1/$manifest_object#" \
  "$cloud_conf"
cp "$cloud_conf" "$test_root/invalid-cloud-conf-before"
invalid_cloud_conf_inode="$(stat -c %i "$cloud_conf")"
: >"$aws_log"
: >"$action_log"
set +e
invalid_conf_output="$(run_transaction I "$prestop_tree" "$prestop_archive" \
  "$prestop_sha" normal 2>&1)"
invalid_conf_status=$?
set -e
[ "$invalid_conf_status" -ne 0 ]
grep -Fq 'does not match the transaction target and ordinary manifest pointer' \
  <<<"$invalid_conf_output"
[ "$(<"$runtime_root/label")" = P ]
cmp -s "$test_root/invalid-cloud-conf-before" "$cloud_conf"
[ "$(stat -c %i "$cloud_conf")" = "$invalid_cloud_conf_inode" ]
[ ! -s "$action_log" ]
if grep -Fq 's3api put-object --bucket' "$aws_log"; then
  echo 'invalid initial manifest state attempted pointer publication' >&2
  exit 1
fi

# A valid but different region, artifact bucket, or relay target is also
# unresolved host state. Reject it before service stop or pointer publication.
for target_mismatch in region bucket relay; do
  rm -rf "$runtime_root"
  cp -a "$prior_tree" "$runtime_root"
  write_pointer "$prior_sha"
  write_cloud_conf
  case "$target_mismatch" in
    region) sed -i 's/^AWS_REGION=.*/AWS_REGION=us-west-2/' "$cloud_conf" ;;
    bucket) sed -i 's/^ARTIFACT_BUCKET=.*/ARTIFACT_BUCKET=other-artifacts/' "$cloud_conf" ;;
    relay)
      sed -i \
        's#^RELAY_URL=.*#RELAY_URL=wss://other.example.test/contract-b/v4#' \
        "$cloud_conf"
      ;;
  esac
  cp "$cloud_conf" "$test_root/$target_mismatch-cloud-conf-before"
  : >"$aws_log"
  : >"$action_log"
  set +e
  mismatch_output="$(run_transaction I "$prestop_tree" "$prestop_archive" \
    "$prestop_sha" normal 2>&1)"
  mismatch_status=$?
  set -e
  [ "$mismatch_status" -ne 0 ]
  grep -Fq 'does not match the transaction target and ordinary manifest pointer' \
    <<<"$mismatch_output"
  [ "$(<"$runtime_root/label")" = P ]
  cmp -s "$test_root/$target_mismatch-cloud-conf-before" "$cloud_conf"
  [ ! -s "$action_log" ] || {
    echo "$target_mismatch configuration mismatch allowed host mutation" >&2
    exit 1
  }
  if grep -Fq 's3api put-object --bucket' "$aws_log"; then
    echo "$target_mismatch configuration mismatch attempted pointer publication" >&2
    exit 1
  fi
done

# Account binding, CLI-model support, and every pinned digest are preflight
# gates. None can fail after a host mutation has started.
preflight_tree="$test_root/preflight-candidate"
preflight_archive="$test_root/preflight-candidate.tar.gz"
make_runtime_archive F "$preflight_tree" "$preflight_archive"
preflight_sha="$archive_sha"
for preflight_scenario in wrong_instance_account wrong_pointer_account \
  missing_if_match_model corrupt_game; do
  rm -rf "$runtime_root"
  cp -a "$prior_tree" "$runtime_root"
  write_pointer "$prior_sha"
  write_cloud_conf
  : >"$aws_log"
  : >"$action_log"
  rm -f "$candidate_active"
  set +e
  preflight_output="$(run_transaction F "$preflight_tree" "$preflight_archive" \
    "$preflight_sha" "$preflight_scenario" 2>&1)"
  preflight_status=$?
  set -e
  [ "$preflight_status" -ne 0 ]
  [ "$(<"$runtime_root/label")" = P ]
  [ ! -s "$action_log" ] || {
    echo "$preflight_scenario allowed host mutation" >&2
    exit 1
  }
done

# A one-hour SSM execution and pointer session leave a fixed 55-minute
# transaction reserve; the lock wait cannot consume that reserve.
: >"$aws_log"
: >"$action_log"
set +e
lock_wait_output="$(TEST_LOCK_WAIT_SECONDS=301 \
  run_transaction F "$preflight_tree" "$preflight_archive" \
    "$preflight_sha" normal 2>&1)"
lock_wait_status=$?
set -e
[ "$lock_wait_status" -ne 0 ]
[ ! -s "$aws_log" ]
[ ! -s "$action_log" ]
grep -Fq 'invalid runtime lock wait' <<<"$lock_wait_output"

# Missing temporary pointer credentials fail after taking the lock but before
# the authoritative pointer snapshot or any runtime activation.
rm -rf "$runtime_root"
cp -a "$prior_tree" "$runtime_root"
missing_tree="$test_root/missing-candidate"
missing_archive="$test_root/missing-candidate.tar.gz"
make_runtime_archive D "$missing_tree" "$missing_archive"
missing_sha="$archive_sha"
write_pointer "$prior_sha"
write_cloud_conf
: >"$aws_log"
: >"$action_log"
set +e
missing_output="$(run_transaction D "$missing_tree" "$missing_archive" \
  "$missing_sha" missing_credentials 2>&1)"
missing_status=$?
set -e
[ "$missing_status" -ne 0 ]
[ "$(<"$runtime_root/label")" = P ]
[ ! -s "$action_log" ] || {
  echo 'missing pointer credentials allowed host mutation' >&2
  exit 1
}
if grep -Fq 's3api get-object' "$aws_log"; then
  echo 'missing pointer credentials reached pointer access' >&2
  exit 1
fi
grep -Fq 'pointer secret-key parameter is missing or invalid' <<<"$missing_output"

for secret in "$access_key_id" "$secret_access_key" "$session_token"; do
  if grep -Fq "$secret" "$aws_log" || grep -Fq "$secret" <<<"$missing_output"; then
    echo 'temporary pointer credential escaped into arguments or output' >&2
    exit 1
  fi
done

printf 'runtime transaction interleaving fixtures passed\n'
