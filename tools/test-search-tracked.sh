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
printf 'ALTERNATION_LEFT\nALTERNATION_RIGHT\n' >"$repo/alternation.txt"
printf '%s\n' '-h' >"$repo/short-help-pattern.txt"
printf '%s\n' '--help' >"$repo/long-help-pattern.txt"
ln -s cloud/aws/private/credential.join "$repo/tracked-link"
git -C "$repo" add \
  .gitignore alternation.txt docs/tracked.txt long-help-pattern.txt \
  short-help-pattern.txt source/tracked.txt tracked-link tracked.txt

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

output=$(cd "$repo" && "$search" 'ALTERNATION_(LEFT|RIGHT)' -- alternation.txt)
expected=$(printf '%s\n' \
  'alternation.txt:1:ALTERNATION_LEFT' \
  'alternation.txt:2:ALTERNATION_RIGHT')
[ "$output" = "$expected" ] || {
  echo "!! the advertised extended-regexp alternation did not match" >&2
  exit 1
}

output=$(cd "$repo" && "$search" -- -h -- short-help-pattern.txt)
[ "$output" = 'short-help-pattern.txt:1:-h' ] || {
  echo "!! the literal -h pattern opened help or returned the wrong match" >&2
  exit 1
}

output=$(cd "$repo" && "$search" -- --help -- long-help-pattern.txt)
[ "$output" = 'long-help-pattern.txt:1:--help' ] || {
  echo "!! the literal --help pattern opened help or returned the wrong match" >&2
  exit 1
}

printf '%s\n' '-SEARCH_SCOPE_OPTION_CANARY tracked' >"$repo/leading-option.txt"
git -C "$repo" add leading-option.txt
output=$(cd "$repo" && "$search" -SEARCH_SCOPE_OPTION_CANARY)
[ "$output" = 'leading-option.txt:1:-SEARCH_SCOPE_OPTION_CANARY tracked' ] || {
  echo "!! a pattern beginning with a dash was parsed as an option" >&2
  exit 1
}

printf 'CONFIG_LEFT\nCONFIG_RIGHT\n' >"$repo/config-output.txt"
git -C "$repo" add config-output.txt
git -C "$repo" config grep.column true
git -C "$repo" config grep.lineNumber false
git -C "$repo" config grep.patternType fixed
git -C "$repo" config color.grep always
output=$(cd "$repo" && "$search" 'CONFIG_(LEFT|RIGHT)' -- config-output.txt)
expected=$(printf '%s\n' \
  'config-output.txt:1:CONFIG_LEFT' \
  'config-output.txt:2:CONFIG_RIGHT')
[ "$output" = "$expected" ] || {
  echo "!! user Git configuration changed the search expression or output" >&2
  exit 1
}

if (cd "$repo" && "$search" SEARCH_SCOPE_NO_MATCH >/dev/null); then
  echo "!! a search with no matches returned success" >&2
  exit 1
fi

echo "tracked-only repository search: PASS"
