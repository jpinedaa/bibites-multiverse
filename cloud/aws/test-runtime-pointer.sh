#!/usr/bin/env bash
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=lib/validation.sh
. "$repo/cloud/aws/lib/validation.sh"

sha256=0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
runtime_file="runtime/$sha256.tar.gz"
document="$(bibites_runtime_pointer_document "$runtime_file" "$sha256")"
bibites_require_runtime_pointer_document "$document"

reject() {
  if bibites_require_runtime_pointer_document "$1" >/dev/null 2>&1; then
    echo "invalid runtime pointer passed: $2" >&2
    exit 1
  fi
}

reject "$(jq '.schema = 2' <<<"$document")" 'unknown schema'
reject "$(jq '.runtimeFile = "runtime/other.tar.gz"' <<<"$document")" \
  'file outside its content address'
reject "$(jq '.extra = true' <<<"$document")" 'unknown field'
reject "$(jq '.runtimeSha256 = (.runtimeSha256 | ascii_upcase)' <<<"$document")" \
  'uppercase digest'

grep -Fq 'RuntimeManifestFile:' "$repo/cloud/aws/template.yaml"
if grep -Eq '^  Runtime(File|Sha256):' "$repo/cloud/aws/template.yaml"; then
  echo 'template still exposes mutable runtime bootstrap parameters' >&2
  exit 1
fi
grep -Fq '"$repo/cloud/aws/promote-runtime.sh" "$RUNTIME_OBJECT" "$RUNTIME_SHA256"' \
  "$repo/cloud/aws/update-runtime.sh"

printf 'runtime-pointer fixtures passed\n'
