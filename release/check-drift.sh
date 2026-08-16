#!/usr/bin/env bash
# Is this tree still consistent with the far-end bundle it ships, and with itself?
#
# THIS IS THE HALF OF release/make-release.sh THAT NEEDS NO GAME. The release
# build's strongest gates compare BYTES: the freshly compiled plugin against the
# copy inside farend/dist/farend-bundle.zip, and that copy against the plugin
# installed in this machine's game. Both need the proprietary reference
# assemblies in bibites-mod/libs/ and the .NET SDK, so neither can run on a
# hosted runner. This script reconstructs the same findings from SOURCE:
#
#   A  the bundle is not stale       — the bundle records the commit it was
#                                      built from; bibites-mod/ and
#                                      farend/setup-farend.ps1 have not moved
#                                      since
#   B  the mod version agrees        — bibites-mod/src/MultiversePlugin.cs, both
#                                      rows of docs/support-matrix.md, and the
#                                      Plugin row of dev_environment.md name one
#                                      number
#   C  the sidecar inputs agree      — release/make-release.sh's gate 2, run from
#                                      the same library, so this is the gate
#                                      itself and not a lookalike
#   D  the bundle's provenance file  — farend/dist/BUNDLE-SOURCE.txt, when the
#                                      bundle has been rebuilt with the script
#                                      that writes it, cross-checked against the
#                                      tree hashes it names
#
# It reads only tracked files and the tracked bundle. No game, no .NET, no
# network, nothing written inside the repository.
#
# IT NEEDS FULL GIT HISTORY. Checks A, C and D resolve the bundle's source
# commit and read the tree at it, so a shallow clone cannot answer them. In
# GitHub Actions that means actions/checkout with `fetch-depth: 0`.
#
# Also needs: go (to read the bundled sidecar's VCS stamp and to list its package
# graph), python3, git, unzip, tar.
#
#   release/check-drift.sh        every check, a PASS/FAIL line each
#
# Exit 0 when every active check passes, 1 when any fails, 2 when the
# environment cannot answer the question at all.
set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RELDIR="$REPO/release"
BUNDLE="$REPO/farend/dist/farend-bundle.zip"
BUNDLE_SOURCE="$REPO/farend/dist/BUNDLE-SOURCE.txt"
BUNDLE_SOURCE_FORMAT="bibites-multiverse/farend-bundle-source/1"
MATRIX_DOC="$REPO/docs/support-matrix.md"
PLUGIN_SOURCE="$REPO/bibites-mod/src/MultiversePlugin.cs"
DEV_ENV_DOC="$REPO/dev_environment.md"
SIDECAR_MANIFEST_LIB="$RELDIR/lib/sidecar-manifest.sh"
MOD_VERSION_LIB="$RELDIR/lib/mod-version.sh"

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

for tool in git go python3 unzip tar; do
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
  || die "missing $MOD_VERSION_LIB — checks B and D read the plugin's version through it"
# shellcheck source=lib/mod-version.sh
. "$MOD_VERSION_LIB"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

HEAD_REV="$(git -C "$REPO" rev-parse HEAD)" \
  || die "$REPO has no HEAD commit"
printf 'checking %s at %s\n' "$REPO" "$(git -C "$REPO" rev-parse --short HEAD)"
if [ -n "$(git -C "$REPO" status --porcelain)" ]; then
  note "the working tree is dirty; these checks describe the tree as it is now"
fi

# ---------------------------------------------------------------- shared reads

# The bundle's source commit, recovered from the one artifact that records it.
# farend/make-farend-bundle.sh writes no manifest of its own (BUNDLE-SOURCE.txt
# is check D's fix for that), but it requires a clean tree and stamps the sidecar
# it packs with the commit — so the stamp IS the bundle's provenance for every
# bundle built so far. release/make-release.sh reads it back the same way.
REF_REV=''
BUNDLE_READ_ERROR=''
read_bundle_revision() {
  [ -f "$BUNDLE" ] || { BUNDLE_READ_ERROR="missing $BUNDLE — it is tracked; this is not a clean clone"; return 1; }
  unzip -q -o -j "$BUNDLE" 'farend-bundle/multiverse-sidecar.exe' -d "$WORK" 2>/dev/null \
    || { BUNDLE_READ_ERROR="$BUNDLE holds no farend-bundle/multiverse-sidecar.exe"; return 1; }
  local stamp
  stamp="$(go version -m "$WORK/multiverse-sidecar.exe" 2>&1)" \
    || { BUNDLE_READ_ERROR="go could not read the bundled sidecar's build stamp: $(printf '%s' "$stamp" | head -n 1)"; return 1; }
  REF_REV="$(printf '%s\n' "$stamp" \
    | sed -n 's/^[[:space:]]*build[[:space:]]*vcs\.revision=//p')"
  [ -n "$REF_REV" ] || { BUNDLE_READ_ERROR="the bundled sidecar carries no VCS stamp; checks A and C need one"; return 1; }
  git -C "$REPO" cat-file -e "$REF_REV^{commit}" 2>/dev/null || {
    BUNDLE_READ_ERROR="commit $REF_REV is not in this clone, so the bundled build cannot be compared to it.
       A shallow checkout does this: fetch the full history (actions/checkout fetch-depth: 0)."
    REF_REV=''
    return 1
  }
  return 0
}
read_bundle_revision || true

# The mod version as three documents state it. Each reader returns empty when the
# line it depends on is not there, and the check says which one.
plugin_version() { # empty, never an error: each check says what a missing value means
  mod_version "$PLUGIN_SOURCE" 2>/dev/null || true
}
matrix_mod_versions() { # one line per entry, in matrix order
  [ -f "$MATRIX_DOC" ] || return 0
  awk '/SUPPORT-MATRIX-JSON-BEGIN/{f=1;next} /SUPPORT-MATRIX-JSON-END/{f=0} f' "$MATRIX_DOC" \
    | sed '/^```/d' > "$WORK/support-matrix.json"
  [ -s "$WORK/support-matrix.json" ] || return 0
  python3 - "$WORK/support-matrix.json" <<'PY'
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

check_a() { # the bundle is not stale
  if [ -z "$REF_REV" ]; then
    bad "$BUNDLE_READ_ERROR"
    return 1
  fi
  note "the bundle's sidecar was built from $REF_REV"
  # ERREXIT IS OFF INSIDE THIS FUNCTION — the caller runs it as `check_a || …`,
  # which puts the whole body in a || context. So every command whose failure
  # would otherwise be read as a passing answer is tested here by hand. A git
  # that cannot diff must not look like "nothing changed".
  local changed
  changed="$(git -C "$REPO" diff --name-only "$REF_REV" "$HEAD_REV" \
    -- bibites-mod/ farend/setup-farend.ps1)" \
    || { bad "git could not diff $REF_REV against HEAD; this check cannot answer"; return 1; }
  if [ -z "$changed" ]; then
    note "bibites-mod/ and farend/setup-farend.ps1 are unchanged since that commit"
    return 0
  fi
  bad "farend/dist/farend-bundle.zip was built from $(git -C "$REPO" rev-parse --short "$REF_REV"); the mod sources have changed since."
  git -C "$REPO" diff --stat "$REF_REV" "$HEAD_REV" -- bibites-mod/ farend/setup-farend.ps1 \
    | sed 's/^/       /' >&2
  cat >&2 <<'REMEDY'
       The bundle is the packaging reference the release build compares the freshly
       built plugin against, byte for byte, so it is stale and the build will refuse.
       All three legs, on the machine that has the game:
         1. run farend/make-farend-bundle.sh and commit the new zip IN ITS OWN COMMIT
         2. refresh this machine's Steam plugin (bibites-mod/deploy.sh)
         3. update the "mod" field on BOTH rows of docs/support-matrix.md
REMEDY
  return 1
}

# ---------------------------------------------------------------- check B

check_b() { # one mod version, in all three places
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
  if [ -z "$dev" ]; then
    bad "no '| Plugin | \`<version>\` |' row in $DEV_ENV_DOC"
    ok=1
  else
    note "$(printf '%-38s %s' 'dev_environment.md, Plugin row' "$dev")"
    [ -z "$plugin" ] || [ "$dev" = "$plugin" ] || ok=1
  fi
  if [ "$ok" -ne 0 ]; then
    bad "the mod version does not agree across the three places that state it."
    cat >&2 <<'REMEDY'
       docs/support-matrix.md's "mod" is what both installers print and what the
       published table promises; the constant is what BepInEx registers; the
       dev_environment.md row is what a contributor builds against. Decide which
       number is right and set all three — and if the plugin itself changed, the
       far-end bundle and this machine's game plugin have to be rebuilt too
       (check A above).
REMEDY
    return 1
  fi
  note "all three say mod $plugin"
  return 0
}

# ---------------------------------------------------------------- check C

check_c() { # release/make-release.sh gate 2, from the same library
  if [ -z "$REF_REV" ]; then
    bad "${BUNDLE_READ_ERROR:-the source commit of the bundle is unknown}"
    return 1
  fi
  [ -d "$REPO/go" ] || { bad "missing $REPO/go"; return 1; }
  local ref_source="$WORK/sidecar-ref-source" ref_archive="$WORK/sidecar-ref-source.tar"
  rm -rf "$ref_source" "$ref_archive"
  mkdir -p "$ref_source"
  git -C "$REPO" archive --format=tar -o "$ref_archive" "$REF_REV" -- go \
    || { bad "cannot read the go/ tree at $REF_REV"; return 1; }
  tar -xf "$ref_archive" -C "$ref_source" || { bad "cannot unpack the go/ tree at $REF_REV"; return 1; }
  local ref_manifest="$WORK/sidecar-inputs-ref.txt" head_manifest="$WORK/sidecar-inputs-head.txt"
  sidecar_manifest "$ref_source" "$ref_manifest" "$WORK/sidecar-packages-ref.json" \
    || { bad "go list failed for cmd/sidecar at $REF_REV"; return 1; }
  sidecar_manifest "$REPO" "$head_manifest" "$WORK/sidecar-packages-head.json" \
    || { bad "go list failed for cmd/sidecar in this tree"; return 1; }
  if cmp -s "$ref_manifest" "$head_manifest"; then
    note "cmd/sidecar uses the same repository inputs and module versions as the bundled sidecar"
    note "compared $(grep -c '^file' "$ref_manifest") source files and $(grep -c '^module' "$ref_manifest") external module identities"
    return 0
  fi
  diff -u "$ref_manifest" "$head_manifest" | sed 's/^/       /' >&2 || true
  bad "the files or modules that build cmd/sidecar differ from the bundled sidecar source."
  cat >&2 <<'REMEDY'
       Rebuild farend/dist/farend-bundle.zip, repeat the relevant tests, and then
       build the release. This is release/make-release.sh's gate 2, run from the
       same release/lib/sidecar-manifest.sh, so a pass here is a pass there.
REMEDY
  return 1
}

# ---------------------------------------------------------------- check D

bundle_source_field() { # $1 field name
  sed -n "s/^$1[[:space:]]\{1,\}//p" "$BUNDLE_SOURCE" | head -n 1
}

check_d() { # farend/dist/BUNDLE-SOURCE.txt against the tree it names
  local ok=0 format commit mod_tree farend_blob mod_version
  local want_mod_tree want_farend_blob plugin rows platform value
  format="$(bundle_source_field format)"
  if [ "$format" != "$BUNDLE_SOURCE_FORMAT" ]; then
    bad "$BUNDLE_SOURCE declares format '${format:-none}', want $BUNDLE_SOURCE_FORMAT"
    return 1
  fi
  commit="$(bundle_source_field commit)"
  mod_tree="$(bundle_source_field bibites-mod-tree)"
  farend_blob="$(bundle_source_field setup-farend-blob)"
  mod_version="$(bundle_source_field mod-version)"

  # Two independent witnesses to the same build: the file's own `commit` and the
  # VCS stamp inside the sidecar it packed. They must name one commit.
  if [ -n "$REF_REV" ] && [ -n "$commit" ] && [ "$commit" != "$REF_REV" ]; then
    bad "the file records commit $commit but the bundled sidecar is stamped $REF_REV"
    ok=1
  fi

  # Errexit is off in here too (see check_a): test the two rev-parses rather
  # than let an empty value turn a broken checkout into "the bundle is stale".
  want_mod_tree="$(git -C "$REPO" rev-parse "HEAD:bibites-mod" 2>/dev/null)" \
    || { bad "cannot resolve HEAD:bibites-mod; this checkout has no bibites-mod tree"; return 1; }
  if [ -z "$mod_tree" ]; then
    bad "no bibites-mod-tree field"
    ok=1
  elif [ "$mod_tree" != "$want_mod_tree" ]; then
    bad "bibites-mod-tree is $mod_tree; HEAD:bibites-mod is $want_mod_tree — the bundle is stale"
    ok=1
  else
    note "bibites-mod-tree  $mod_tree  == HEAD:bibites-mod"
  fi

  want_farend_blob="$(git -C "$REPO" rev-parse "HEAD:farend/setup-farend.ps1" 2>/dev/null)" \
    || { bad "cannot resolve HEAD:farend/setup-farend.ps1 in this checkout"; return 1; }
  if [ -z "$farend_blob" ]; then
    bad "no setup-farend-blob field"
    ok=1
  elif [ "$farend_blob" != "$want_farend_blob" ]; then
    bad "setup-farend-blob is $farend_blob; HEAD:farend/setup-farend.ps1 is $want_farend_blob"
    ok=1
  else
    note "setup-farend-blob $farend_blob  == HEAD:farend/setup-farend.ps1"
  fi

  plugin="$(plugin_version)"
  if [ -z "$mod_version" ]; then
    bad "no mod-version field"
    ok=1
  else
    note "mod-version       $mod_version"
    if [ -n "$plugin" ] && [ "$mod_version" != "$plugin" ]; then
      bad "mod-version is $mod_version but MultiversePlugin.cs says $plugin"
      ok=1
    fi
    rows="$(matrix_mod_versions)" \
      || { bad "cannot read the matrix JSON block in $MATRIX_DOC"; return 1; }
    while IFS="$(printf '\t')" read -r platform value; do
      [ -n "$platform" ] || continue
      if [ "$value" != "$mod_version" ]; then
        bad "mod-version is $mod_version but the matrix $platform row says ${value:-<none>}"
        ok=1
      fi
    done <<EOF
$rows
EOF
  fi

  if [ "$ok" -ne 0 ]; then
    cat >&2 <<'REMEDY'
       farend/dist/BUNDLE-SOURCE.txt is written beside the zip by
       farend/make-farend-bundle.sh. Re-run it on the machine that has the game and
       commit the zip and the file together, then set the "mod" field on both rows of
       docs/support-matrix.md to the version the file names.
REMEDY
    return 1
  fi
  note "the bundle's provenance file agrees with this tree"
  return 0
}

# ---------------------------------------------------------------- run them all

A_STATUS=PASS; B_STATUS=PASS; C_STATUS=PASS; D_STATUS=PASS

step "check A — the far-end bundle against this tree"
check_a || A_STATUS=FAIL

step "check B — the mod version, in the three places that state it"
check_b || B_STATUS=FAIL

step "check C — the cmd/sidecar input manifest (release gate 2)"
check_c || C_STATUS=FAIL

step "check D — the bundle's provenance file"
if [ -f "$BUNDLE_SOURCE" ]; then
  check_d || D_STATUS=FAIL
else
  D_STATUS=INACTIVE
  note "no farend/dist/BUNDLE-SOURCE.txt in this tree."
  note "Check D stays inactive until the bundle is rebuilt with the version of"
  note "farend/make-farend-bundle.sh that writes the file. This is a note, not a"
  note "failure: checks A and C already cover the same drift from the bundle itself."
fi

step "summary"
printf '    A  far-end bundle freshness ......... %s\n' "$A_STATUS"
printf '    B  mod version agreement ............ %s\n' "$B_STATUS"
printf '    C  cmd/sidecar input manifest ....... %s\n' "$C_STATUS"
printf '    D  bundle provenance file ........... %s\n' "$D_STATUS"

case "$A_STATUS$B_STATUS$C_STATUS$D_STATUS" in
  *FAIL*)
    printf '\n!! this tree is not releasable as it stands; see the failing checks above\n' >&2
    exit 1 ;;
esac
printf '\n   no drift found\n'
