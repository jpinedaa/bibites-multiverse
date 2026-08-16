#!/usr/bin/env bash
# Is this tree still the build that was tested, and is it consistent with itself?
#
# THIS IS THE HALF OF release/make-release.sh THAT NEEDS NO GAME. The release
# build's strongest gates hash things it has just built: the freshly compiled
# plugin against docs/support-matrix.md's testedBuild.pluginSha256, and that
# same file against the plugin installed in this machine's game. The first needs
# the proprietary reference assemblies in bibites-mod/libs/ and the .NET SDK,
# the second needs the game, so neither can run on a hosted runner. This script
# reconstructs the same findings from SOURCE:
#
#   A  the tested build is current   — testedBuild.bibitesModTree is still
#                                      HEAD:bibites-mod, so the mod this tree
#                                      builds is the mod somebody tested
#   B  the mod version agrees        — bibites-mod/src/MultiversePlugin.cs, both
#                                      rows of docs/support-matrix.md,
#                                      testedBuild.mod, and the Plugin row of
#                                      dev_environment.md name one number
#   C  the sidecar inputs agree      — release/make-release.sh's gate 2, run from
#                                      the same library, so this is the gate
#                                      itself and not a lookalike
#
# It reads only tracked files. No game, no .NET, no network, no binary in the
# repository, nothing written inside it.
#
# IT NEEDS NO GIT HISTORY. Every comparison is between a value recorded in the
# matrix and a value read from HEAD's own tree — one `git rev-parse
# HEAD:bibites-mod`, one manifest of the working tree — so a shallow clone
# answers all three. A failing check names the recorded commit and the `git
# diff` to run, which is read in a local checkout rather than in CI.
#
# Also needs: go (to list cmd/sidecar's package graph), python3, git.
#
#   release/check-drift.sh        every check, a PASS/FAIL line each
#
# Exit 0 when every check passes, 1 when any fails, 2 when the environment
# cannot answer the question at all.
set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RELDIR="$REPO/release"
MATRIX_DOC="$REPO/docs/support-matrix.md"
PLUGIN_SOURCE="$REPO/bibites-mod/src/MultiversePlugin.cs"
DEV_ENV_DOC="$REPO/dev_environment.md"
SIDECAR_MANIFEST_LIB="$RELDIR/lib/sidecar-manifest.sh"
MOD_VERSION_LIB="$RELDIR/lib/mod-version.sh"
TESTED_BUILD_LIB="$RELDIR/lib/tested-build.sh"

# GO COMES FROM TWO PLACES. On the owner's box it lives in $GOROOT, the same
# convention release/make-release.sh uses; on a hosted runner actions/setup-go
# puts it on PATH and owns GOROOT itself. Take the local convention only when it
# is real — a GOROOT left over in a shell profile, pointing at a directory that
# no longer exists, must not shadow a working toolchain on PATH.
if [ -n "${GOROOT:-}" ] && [ ! -x "$GOROOT/bin/go" ]; then
  unset GOROOT
fi
if [ -z "${GOROOT:-}" ] && [ -x "$HOME/go/bin/go" ]; then
  export GOROOT="$HOME/go"
fi
[ -z "${GOROOT:-}" ] || export PATH="$GOROOT/bin:$PATH"
export TZ=UTC

[ $# -eq 0 ] || { printf 'usage: %s\n' "$0" >&2; exit 2; }

note() { printf '    %s\n' "$*"; }
step() { printf '\n=== %s\n' "$*"; }
bad()  { printf '    !! %s\n' "$*" >&2; }
die()  { printf '\n!! %s\n' "$*" >&2; exit 2; }

for tool in git go python3; do
  command -v "$tool" >/dev/null 2>&1 || die "$tool is not on PATH; this script needs it"
done
# GOROOT is taken on trust above; prove the toolchain actually runs, so a broken
# one is one clear line here instead of a puzzling read failure inside a check.
go version >/dev/null 2>&1 \
  || die "go is on PATH but will not run — GOROOT is ${GOROOT:-unset}: $(go version 2>&1 | head -n 1)"
git -C "$REPO" rev-parse --git-dir >/dev/null 2>&1 || die "$REPO is not a git checkout"
[ -f "$SIDECAR_MANIFEST_LIB" ] \
  || die "missing $SIDECAR_MANIFEST_LIB — check C runs the release build's own gate from it"
# shellcheck source=lib/sidecar-manifest.sh
. "$SIDECAR_MANIFEST_LIB"
[ -f "$MOD_VERSION_LIB" ] \
  || die "missing $MOD_VERSION_LIB — check B reads the plugin's version through it"
# shellcheck source=lib/mod-version.sh
. "$MOD_VERSION_LIB"
[ -f "$TESTED_BUILD_LIB" ] \
  || die "missing $TESTED_BUILD_LIB — every check compares against the record it parses"
# shellcheck source=lib/tested-build.sh
. "$TESTED_BUILD_LIB"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

git -C "$REPO" rev-parse HEAD >/dev/null 2>&1 \
  || die "$REPO has no HEAD commit"
printf 'checking %s at %s\n' "$REPO" "$(git -C "$REPO" rev-parse --short HEAD)"
if [ -n "$(git -C "$REPO" status --porcelain)" ]; then
  note "the working tree is dirty; these checks describe the tree as it is now"
fi

# ---------------------------------------------------------------- shared reads

# THE TESTED-BUILD RECORD, read once, through the same library the release build
# uses. Every check compares against a field of it, so a record that cannot be
# read is reported once here and each check says what it could not answer.
MATRIX_JSON="$WORK/support-matrix.json"
TB_MOD='' TB_MOD_TREE='' TB_COMMIT='' TB_INPUTS='' TB_TESTED_ON=''
TESTED_BUILD_ERROR=''
read_tested_build() {
  local err
  err="$(matrix_json_block "$MATRIX_DOC" "$MATRIX_JSON" 2>&1)" \
    || { TESTED_BUILD_ERROR="$err"; return 1; }
  # One validation of the whole record happens inside the first read; the rest
  # cannot fail on their own. Each is still tested, because a value that silently
  # became empty would turn a comparison into a comparison against nothing.
  TB_MOD="$(tested_build_field "$MATRIX_JSON" mod 2>&1)" \
    || { TESTED_BUILD_ERROR="$TB_MOD"; TB_MOD=''; return 1; }
  TB_MOD_TREE="$(tested_build_field "$MATRIX_JSON" bibitesModTree)" || return 1
  TB_COMMIT="$(tested_build_field "$MATRIX_JSON" sidecarSourceCommit)" || return 1
  TB_INPUTS="$(tested_build_field "$MATRIX_JSON" sidecarInputsSha256)" || return 1
  TB_TESTED_ON="$(tested_build_field "$MATRIX_JSON" testedOn)" || return 1
  return 0
}
read_tested_build || true
if [ -n "$TESTED_BUILD_ERROR" ]; then
  note "the tested-build record could not be read; checks A, B and C say so below"
else
  note "the tested build: mod $TB_MOD, recorded at $TB_COMMIT, tested $TB_TESTED_ON"
fi

# The mod version as four places state it. Each reader returns empty when the
# line it depends on is not there, and the check says which one.
plugin_version() { # empty, never an error: each check says what a missing value means
  mod_version "$PLUGIN_SOURCE" 2>/dev/null || true
}
matrix_mod_versions() { # one line per entry, in matrix order
  [ -s "$MATRIX_JSON" ] || return 0
  python3 - "$MATRIX_JSON" <<'PY'
import json, sys
doc = json.load(open(sys.argv[1]))
for entry in doc["entries"]:
    print("%s\t%s" % (entry.get("platform", "?"), entry.get("mod", "")))
PY
}
dev_environment_plugin_version() {
  [ -f "$DEV_ENV_DOC" ] || return 0
  sed -n 's/^|[[:space:]]*Plugin[[:space:]]*|[[:space:]]*`\([^`]*\)`[[:space:]]*|.*/\1/p' \
    "$DEV_ENV_DOC" | head -n 1
}

# ---------------------------------------------------------------- check A

check_a() { # the mod in this tree is the mod that was tested
  if [ -z "$TB_MOD_TREE" ]; then
    bad "${TESTED_BUILD_ERROR:-the tested-build record carries no bibitesModTree}"
    return 1
  fi
  # ERREXIT IS OFF INSIDE THIS FUNCTION — the caller runs it as `check_a || …`,
  # which puts the whole body in a || context. So every command whose failure
  # would otherwise be read as a passing answer is tested here by hand. A
  # rev-parse that cannot answer must not look like "nothing changed".
  local got
  got="$(git -C "$REPO" rev-parse "HEAD:bibites-mod" 2>/dev/null)" \
    || { bad "cannot resolve HEAD:bibites-mod; this checkout has no bibites-mod tree"; return 1; }
  note "$(printf '%-31s %s' 'testedBuild.bibitesModTree' "$TB_MOD_TREE")"
  note "$(printf '%-31s %s' 'HEAD:bibites-mod' "$got")"
  if [ "$TB_MOD_TREE" = "$got" ]; then
    note "the mod in this tree is the mod that was tested, on $TB_TESTED_ON"
    return 0
  fi
  bad "bibites-mod/ has moved since the tested build; this tree's plugin was never tested."
  cat >&2 <<REMEDY
       The release build compares the plugin it compiles against
       testedBuild.pluginSha256, so it will refuse this tree. All three legs, on
       the machine that has the game:
         1. build the plugin this tree makes, and TEST it
         2. refresh this machine's game plugin (bibites-mod/deploy.sh)
         3. run release/record-tested-build.sh and put its block in
            docs/support-matrix.md, with the "mod" field on BOTH rows to match
       What moved, read in a checkout that has the history:
         git diff $TB_COMMIT HEAD -- bibites-mod/
REMEDY
  return 1
}

# ---------------------------------------------------------------- check B

check_b() { # one mod version, in all four places
  local plugin rows dev ok=0 row platform value
  plugin="$(plugin_version)"
  rows="$(matrix_mod_versions)" \
    || { bad "cannot read the matrix JSON block in $MATRIX_DOC"; return 1; }
  dev="$(dev_environment_plugin_version)"
  if [ -z "$plugin" ]; then
    bad "no 'public const string Version' in $PLUGIN_SOURCE"
    ok=1
  else
    note "$(printf '%-38s %s' 'bibites-mod/src/MultiversePlugin.cs' "$plugin")"
  fi
  if [ -z "$rows" ]; then
    bad "no matrix entries with a \"mod\" field in $MATRIX_DOC"
    ok=1
  else
    while IFS="$(printf '\t')" read -r platform value; do
      [ -n "$platform" ] || continue
      note "$(printf '%-38s %s' "docs/support-matrix.md, $platform row" "${value:-<none>}")"
      if [ -z "$value" ]; then
        bad "the $platform row carries no \"mod\" field"
        ok=1
      elif [ -n "$plugin" ] && [ "$value" != "$plugin" ]; then
        ok=1
      fi
    done <<EOF
$rows
EOF
  fi
  if [ -z "$TB_MOD" ]; then
    bad "${TESTED_BUILD_ERROR:-the tested-build record carries no mod}"
    ok=1
  else
    note "$(printf '%-38s %s' 'docs/support-matrix.md, testedBuild' "$TB_MOD")"
    [ -z "$plugin" ] || [ "$TB_MOD" = "$plugin" ] || ok=1
  fi
  if [ -z "$dev" ]; then
    bad "no '| Plugin | \`<version>\` |' row in $DEV_ENV_DOC"
    ok=1
  else
    note "$(printf '%-38s %s' 'dev_environment.md, Plugin row' "$dev")"
    [ -z "$plugin" ] || [ "$dev" = "$plugin" ] || ok=1
  fi
  if [ "$ok" -ne 0 ]; then
    bad "the mod version does not agree across the four places that state it."
    cat >&2 <<'REMEDY'
       docs/support-matrix.md's "mod" is what both installers print and what the
       published table promises; testedBuild.mod is the version somebody tested;
       the constant is what BepInEx registers; the dev_environment.md row is what
       a contributor builds against. Decide which number is right and set all
       four — and if the plugin itself changed, testedBuild is not a text edit:
       test the new plugin and record it (check A above).
REMEDY
    return 1
  fi
  note "all four say mod $plugin"
  return 0
}

# ---------------------------------------------------------------- check C

check_c() { # release/make-release.sh gate 2, from the same library
  if [ -z "$TB_INPUTS" ]; then
    bad "${TESTED_BUILD_ERROR:-the tested-build record carries no sidecarInputsSha256}"
    return 1
  fi
  [ -d "$REPO/go" ] || { bad "missing $REPO/go"; return 1; }
  local head_manifest="$WORK/sidecar-inputs-head.txt" got
  sidecar_manifest "$REPO" "$head_manifest" "$WORK/sidecar-packages-head.json" \
    || { bad "go list failed for cmd/sidecar in this tree"; return 1; }
  got="$(sha256sum "$head_manifest" | cut -d' ' -f1)" \
    || { bad "cannot hash the manifest this tree produced"; return 1; }
  note "$(printf '%-31s %s' 'testedBuild.sidecarInputsSha256' "$TB_INPUTS")"
  note "$(printf '%-31s %s' 'this tree' "$got")"
  if [ "$got" = "$TB_INPUTS" ]; then
    note "cmd/sidecar uses the repository inputs and module versions that were tested"
    note "that manifest holds $(grep -c '^file' "$head_manifest") file entries and $(grep -c '^module' "$head_manifest") module entries"
    return 0
  fi
  bad "the files or modules that build cmd/sidecar are not the ones that were tested."
  cat >&2 <<REMEDY
       The sidecar changes far more often than the mod, so this is the check that
       catches a sidecar nobody has run. This is release/make-release.sh's gate 2,
       computed from the same release/lib/sidecar-manifest.sh, so a pass here is a
       pass there.
       What moved, read in a checkout that has the history:
         git diff $TB_COMMIT HEAD -- go/
       Then test this sidecar and record the build it makes:
         release/record-tested-build.sh
REMEDY
  return 1
}

# ---------------------------------------------------------------- run them all

A_STATUS=PASS; B_STATUS=PASS; C_STATUS=PASS

step "check A — the tested build against this tree's mod"
check_a || A_STATUS=FAIL

step "check B — the mod version, in the four places that state it"
check_b || B_STATUS=FAIL

step "check C — the cmd/sidecar input manifest (release gate 2)"
check_c || C_STATUS=FAIL

step "summary"
printf '    A  tested build is current .......... %s\n' "$A_STATUS"
printf '    B  mod version agreement ............ %s\n' "$B_STATUS"
printf '    C  cmd/sidecar input manifest ....... %s\n' "$C_STATUS"

case "$A_STATUS$B_STATUS$C_STATUS" in
  *FAIL*)
    printf '\n!! this tree is not releasable as it stands; see the failing checks above\n' >&2
    exit 1 ;;
esac
printf '\n   no drift found\n'
