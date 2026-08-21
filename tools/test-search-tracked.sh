#!/usr/bin/env bash

set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
search="$root/tools/search-tracked.sh"
scratch=$(mktemp -d)
trap 'rm -rf "$scratch"' EXIT

repo="$scratch/repository"
git init -q "$repo"
mkdir -p "$repo/docs" "$repo/source" "$repo/cloud/aws/private"

printf '/cloud/aws/private/\n' >"$repo/.gitignore"
printf 'SEARCH_SCOPE_CANARY tracked root\n' >"$repo/tracked.txt"
printf 'SEARCH_SCOPE_CANARY tracked docs\n' >"$repo/docs/tracked.txt"
printf 'SEARCH_SCOPE_CANARY tracked source\n' >"$repo/source/tracked.txt"
printf 'SEARCH_SCOPE_CANARY ignored custody\n' >"$repo/cloud/aws/private/credential.join"
printf 'SEARCH_SCOPE_CANARY ordinary untracked\n' >"$repo/untracked.txt"
ln -s cloud/aws/private/credential.join "$repo/tracked-link"
git -C "$repo" add .gitignore tracked.txt docs/tracked.txt source/tracked.txt tracked-link

output=$(cd "$repo/source" && "$search" SEARCH_SCOPE_CANARY)
expected=$(printf '%s\n' \
  'docs/tracked.txt:1:SEARCH_SCOPE_CANARY tracked docs' \
  'source/tracked.txt:1:SEARCH_SCOPE_CANARY tracked source' \
  'tracked.txt:1:SEARCH_SCOPE_CANARY tracked root')
[ "$output" = "$expected" ] || {
  echo "!! tracked search returned the wrong files" >&2
  diff -u <(printf '%s\n' "$expected") <(printf '%s\n' "$output") >&2 || true
  exit 1
}

output=$(cd "$repo" && "$search" SEARCH_SCOPE_CANARY -- docs)
[ "$output" = 'docs/tracked.txt:1:SEARCH_SCOPE_CANARY tracked docs' ] || {
  echo "!! path-limited tracked search escaped its pathspec" >&2
  exit 1
}

printf '%s\n' '-SEARCH_SCOPE_OPTION_CANARY tracked' >"$repo/leading-option.txt"
git -C "$repo" add leading-option.txt
output=$(cd "$repo" && "$search" -SEARCH_SCOPE_OPTION_CANARY)
[ "$output" = 'leading-option.txt:1:-SEARCH_SCOPE_OPTION_CANARY tracked' ] || {
  echo "!! a pattern beginning with a dash was parsed as an option" >&2
  exit 1
}

if (cd "$repo" && "$search" SEARCH_SCOPE_NO_MATCH >/dev/null); then
  echo "!! a search with no matches returned success" >&2
  exit 1
fi

echo "tracked-only repository search: PASS"
