#!/usr/bin/env bash
# Prove that release-page links do not depend on the clone's origin.
set -uo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BUILDER="$REPO/release/make-release.sh"
WORKFLOW="$REPO/.github/workflows/release.yml"
[ -f "$BUILDER" ] || { printf 'not here: %s\n' "$BUILDER" >&2; exit 2; }
[ -f "$WORKFLOW" ] || { printf 'not here: %s\n' "$WORKFLOW" >&2; exit 2; }

# Exercise the builder's actual resolver without running the package build.
resolver="$(awk '
  /^CANONICAL_RELEASE_REPO_SLUG=/ { print }
  /^release_repo_slug\(\) \{$/,/^\}$/ { print }
' "$BUILDER")"
[ "$(printf '%s\n' "$resolver" | grep -c '^CANONICAL_RELEASE_REPO_SLUG=')" -eq 1 ] \
  || { printf 'cannot find one canonical release repository in %s\n' "$BUILDER" >&2; exit 2; }
[ "$(printf '%s\n' "$resolver" | grep -c '^release_repo_slug() {$')" -eq 1 ] \
  || { printf 'cannot find one release_repo_slug function in %s\n' "$BUILDER" >&2; exit 2; }
eval "$resolver"

CHECKS=0
FAILURES=0
pass() { CHECKS=$((CHECKS + 1)); printf '  PASS  %s\n' "$1"; }
fail() { CHECKS=$((CHECKS + 1)); FAILURES=$((FAILURES + 1)); printf '  FAIL  %s\n' "$1"; }
expect_slug() { # $1 expected, $2 label; RELEASE_REPO_SLUG comes from the caller
  local got
  if got="$(release_repo_slug)" && [ "$got" = "$1" ]; then pass "$2"; else fail "$2"; fi
}
refuse_slug() { # $1 value, $2 label
  if RELEASE_REPO_SLUG="$1" release_repo_slug >/dev/null 2>&1; then fail "$2"; else pass "$2"; fi
}

FIXTURE="$(mktemp -d)"
trap 'rm -rf "$FIXTURE"' EXIT
git -C "$FIXTURE" init -q
git -C "$FIXTURE" remote add origin /srv/old-clones/j_jor/bibites-multiverse

PROJECT_REPO="$REPO"
REPO="$FIXTURE"
unset RELEASE_REPO_SLUG
cd "$FIXTURE"
expect_slug 'jpinedaa/bibites-multiverse' 'a stale local origin cannot change the canonical default'
REPO="$PROJECT_REPO"
cd "$PROJECT_REPO"

RELEASE_REPO_SLUG='example-owner/example_repo'
expect_slug 'example-owner/example_repo' 'a valid explicit repository overrides the default'

refuse_slug 'https://github.com/jpinedaa/bibites-multiverse' 'a URL is not a repository slug'
refuse_slug 'j_jor/bibites-multiverse' 'an invalid GitHub owner is refused'
refuse_slug 'owner/repo/extra' 'more than two path segments are refused'

if grep -q 'remote\.origin\.url' "$BUILDER"; then
  fail 'the builder does not infer release-page links from origin'
else
  pass 'the builder does not infer release-page links from origin'
fi
if grep -Fq "'@@REPO@@':       '\$RELEASE_REPO_SLUG'" "$BUILDER"; then
  pass 'the release-page repository field uses the resolved slug'
else
  fail 'the release-page repository field uses the resolved slug'
fi
if grep -Eq '^      RELEASE_REPO_SLUG: jpinedaa/bibites-multiverse$' "$WORKFLOW"; then
  pass 'the release workflow supplies the canonical slug'
else
  fail 'the release workflow supplies the canonical slug'
fi

printf '\n'
if [ "$FAILURES" -gt 0 ]; then
  printf '!! %d of %d checks failed\n' "$FAILURES" "$CHECKS"
  exit 1
fi
printf '    %d checks pass\n' "$CHECKS"
