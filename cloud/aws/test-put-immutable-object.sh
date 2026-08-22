#!/usr/bin/env bash
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
test_root="$(mktemp -d)"
trap 'find "$test_root" -depth -delete' EXIT

fixture_repo="$test_root/repo"
fixture_cloud="$fixture_repo/cloud/aws"
fixture_bin="$test_root/bin"
install -d "$fixture_cloud/lib" "$fixture_bin"
cp "$repo/cloud/aws/put-immutable-object.sh" "$fixture_cloud/put-immutable-object.sh"
cp "$repo/cloud/aws/lib/validation.sh" "$fixture_cloud/lib/validation.sh"
chmod 0755 "$fixture_cloud/put-immutable-object.sh"

source_file="$test_root/runtime.tar.gz"
printf 'fixture runtime\n' >"$source_file"
source_sha256="$(sha256sum "$source_file" | awk '{print $1}')"
remote_file="$test_root/remote-object"

cat >"$fixture_bin/aws" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"$MOCK_AWS_LOG"
args=" $* "
case "$args" in
  *' sts get-caller-identity '*)
    if [ "${MOCK_MUTATE_SOURCE:-0}" = 1 ]; then
      printf 'changed after validation\n' >"$MOCK_SOURCE_FILE"
    fi
    printf '%s\n' "${MOCK_ACCOUNT:-123456789012}"
    ;;
  *' s3api put-object '*)
    body=''
    while [ "$#" -gt 0 ]; do
      if [ "$1" = --body ]; then body="$2"; break; fi
      shift
    done
    [ -n "$body" ] || exit 65
    case "${MOCK_PUT_RESULT:-success}" in
      success) cp "$body" "$MOCK_REMOTE_FILE" ;;
      precondition)
        echo 'An error occurred (PreconditionFailed) when calling PutObject: 412' >&2
        exit 255
        ;;
      lost-response)
        cp "$body" "$MOCK_REMOTE_FILE"
        echo 'RequestTimeout: response was lost after the object was stored' >&2
        exit 255
        ;;
      error)
        echo 'An error occurred (AccessDenied) when calling PutObject: denied' >&2
        exit 255
        ;;
      *) exit 65 ;;
    esac
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
    [ "$source" = 's3://fixture-artifacts/cloud/v1/runtime/object.tar.gz' ] || exit 65
    cp "$MOCK_REMOTE_FILE" "$destination"
    ;;
  *)
    echo "unexpected AWS command: $*" >&2
    exit 65
    ;;
esac
MOCK
chmod 0755 "$fixture_bin/aws"

run_fixture() {
  env PATH="$fixture_bin:$PATH" AWS_PROFILE=fixture AWS_REGION=us-east-1 \
    BIBITES_AWS_ACCOUNT_ID=123456789012 \
    MOCK_AWS_LOG="$test_root/aws.log" MOCK_REMOTE_FILE="$remote_file" \
    MOCK_SOURCE_FILE="$source_file" "$@" \
    "$fixture_cloud/put-immutable-object.sh" "$source_file" "$source_sha256" \
    fixture-artifacts cloud/v1/runtime/object.tar.gz
}

# A new object is conditionally created, encrypted, read back, and verified.
rm -f "$remote_file"
: >"$test_root/aws.log"
create_output="$(run_fixture MOCK_PUT_RESULT=success)"
grep -Fq 'created and verified immutable object' <<<"$create_output"
cmp -s "$source_file" "$remote_file"
[ "$(wc -l <"$test_root/aws.log")" -eq 3 ]
grep -Fq 'sts get-caller-identity' "$test_root/aws.log"
grep -Fq -- '--if-none-match *' "$test_root/aws.log"
grep -Fq -- '--server-side-encryption AES256' "$test_root/aws.log"
grep -Fq 's3 cp s3://fixture-artifacts/cloud/v1/runtime/object.tar.gz' \
  "$test_root/aws.log"

# The upload uses one verified snapshot even when the caller's source changes
# after local validation and before the conditional PUT.
cp "$source_file" "$test_root/source-before-mutation"
rm -f "$remote_file"
: >"$test_root/aws.log"
mutation_output="$(run_fixture MOCK_MUTATE_SOURCE=1)"
grep -Fq 'created and verified immutable object' <<<"$mutation_output"
cmp -s "$test_root/source-before-mutation" "$remote_file"
if cmp -s "$source_file" "$remote_file"; then
  echo 'immutable upload read the changing caller source instead of its snapshot' >&2
  exit 1
fi
mv "$test_root/source-before-mutation" "$source_file"

# A 412 accepts the concurrent winner only when its bytes match the address.
cp "$source_file" "$remote_file"
: >"$test_root/aws.log"
existing_output="$(run_fixture MOCK_PUT_RESULT=precondition)"
grep -Fq 'verified existing immutable object' <<<"$existing_output"
[ "$(wc -l <"$test_root/aws.log")" -eq 3 ]

# A lost successful PUT response is reconciled from an exact readback.
rm -f "$remote_file"
: >"$test_root/aws.log"
reconciled_output="$(run_fixture MOCK_PUT_RESULT=lost-response)"
grep -Fq 'reconciled and verified immutable object' <<<"$reconciled_output"
cmp -s "$source_file" "$remote_file"
[ "$(wc -l <"$test_root/aws.log")" -eq 3 ]

# A corrupt concurrent winner is preserved but rejected.
printf 'different bytes\n' >"$remote_file"
cp "$remote_file" "$test_root/corrupt-before"
: >"$test_root/aws.log"
set +e
corrupt_output="$(run_fixture MOCK_PUT_RESULT=precondition 2>&1)"
corrupt_status=$?
set -e
[ "$corrupt_status" -eq 1 ]
grep -Fq 'immutable object has unexpected content' <<<"$corrupt_output"
cmp -s "$test_root/corrupt-before" "$remote_file"
[ "$(wc -l <"$test_root/aws.log")" -eq 3 ]

# A non-precondition error also reads back, but cannot accept different bytes.
printf 'different bytes\n' >"$remote_file"
: >"$test_root/aws.log"
set +e
error_output="$(run_fixture MOCK_PUT_RESULT=error 2>&1)"
error_status=$?
set -e
[ "$error_status" -eq 1 ]
grep -Fq 'Immutable object publication failed:' <<<"$error_output"
grep -Fq 'AccessDenied' <<<"$error_output"
grep -Fq 'immutable object has unexpected content' <<<"$error_output"
[ "$(wc -l <"$test_root/aws.log")" -eq 3 ]

# The account guard stops before the immutable PUT.
: >"$test_root/aws.log"
set +e
account_output="$(run_fixture MOCK_ACCOUNT=210987654321 2>&1)"
account_status=$?
set -e
[ "$account_status" -eq 1 ]
grep -Fq 'refusing AWS account 210987654321' <<<"$account_output"
[ "$(wc -l <"$test_root/aws.log")" -eq 1 ]

printf 'immutable-object fixtures passed\n'
