#!/usr/bin/env bash
# Build the GitHub release's artifacts into release/dist/, from this checkout.
#
#   release/dist/bibites-multiverse-<release>-windows-x64.zip   the Windows download
#   release/dist/bibites-multiverse-<release>-linux-x64.zip     the Linux download
#   release/dist/*-complete.zip                                 optional complete
#                                                               game-bundled editions
#   release/dist/*-setup.exe                                    single-file Windows setup
#   release/dist/bibites-multiverse-windows-x64-setup.exe       stable-named copies,
#   release/dist/bibites-multiverse-linux-x64-complete.zip        which the homepage links
#   release/dist/SHA256SUMS                                     all checksums
#   release/dist/RELEASE-PAGE.md                                the page text,
#                                                               with the checksums in it
#
# TWO PLATFORMS, OPTIONAL COMPLETE EDITIONS, ONE MOD. The plugin is the SAME FILE, byte for byte:
# it is platform-independent IL, a Harmony patch against managed types that the
# game's Mono build carries identically on both platforms. What differs between
# the archives is the sidecar (a native binary, cross-compiled twice), the mod
# framework (BepInEx win_x64 against linux_x64), and the kit — PowerShell against
# bash. This script builds one plugin and copies it into every staged edition.
#
# IT PUBLISHES NOTHING. There is no gh, no git tag and no network call except the
# two cached BepInEx downloads. Publishing is a separate, deliberate act — see
# release/README.md for the four steps the owner performs by hand.
#
# WHAT IT CHECKS BEFORE IT PACKAGES ANYTHING. THE RELEASE REFERENCE IS
# docs/support-matrix.md's "testedBuild" record: the sha256 of the plugin that
# was tested, and the digest of the cmd/sidecar input manifest of the source
# that was tested. This script requires the freshly built plugin to hash to the
# first and this tree's cmd/sidecar inputs to digest to the second. Unrelated Go
# commands do not affect that comparison. The launcher is one of those unrelated
# commands: it is built and VCS-stamped here, and its inputs are deliberately
# outside the manifest. What the two gates prove is that the mod being packaged
# is the mod somebody tested, and that the sidecar being packaged is built from
# the source somebody tested. They do not prove that either artifact ran on
# another computer: the record's own `evidence` sentence is where that is
# claimed, by the person who ran it.
#
# It also requires the game build to be the one docs/support-matrix.md names.
# The matrix is the single source of that pin: the JSON block inside that
# document is copied into the archive unchanged, so the installer's refusal and
# the published table cannot drift apart.
#
# PREREQUISITES, on a machine that has the game:
#   * bibites-mod/libs/, from bibites-mod/sync-game-refs.sh (the reference
#     assemblies are the game's own and are never committed)
#   * the .NET SDK — DOTNET_ROOT, else /usr/local/dotnet, else ~/.dotnet — and Go
#     in $GOROOT or on PATH. A PLAYER needs neither; this is the build side
#   * git, python3, tar, zip, unzip, curl
#   * NSIS 3.09 or newer when you build the Windows complete edition
#
# Everything heavy runs under nice -n 19 to limit interference with local tests.
set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RELDIR="$REPO/release"
DIST="$RELDIR/dist"
BUILD="$DIST/build"
MATRIX_DOC="$REPO/docs/support-matrix.md"
# farend/ keeps the private-map far-end tooling; this is only its BepInEx
# download cache, which the release build and that tooling share.
CACHE="$REPO/farend/dist/cache"
PROJECT_LICENSE="$REPO/LICENSE"
THIRD_PARTY_NOTICES="$REPO/THIRD_PARTY_NOTICES.md"
# A linked worktree can make Go omit VCS metadata when its .git file points to
# another filesystem. Use a clean checkout of this exact commit in that case.
SIDECAR_BUILD_REPO="${RELEASE_SIDECAR_BUILD_REPO:-$REPO}"

RELEASE=0.2.8
TAG="v$RELEASE"
ZIP_NAME="bibites-multiverse-${RELEASE}-windows-x64.zip"
LINUX_ZIP_NAME="bibites-multiverse-${RELEASE}-linux-x64.zip"
COMPLETE_ZIP_NAME="bibites-multiverse-${RELEASE}-windows-x64-complete.zip"
LINUX_COMPLETE_ZIP_NAME="bibites-multiverse-${RELEASE}-linux-x64-complete.zip"
WINDOWS_SETUP_NAME="bibites-multiverse-${RELEASE}-windows-x64-setup.exe"
STAGE_NAME="bibites-multiverse-${RELEASE}"
# THE TWO NAMES THAT NEVER CHANGE. GitHub answers
# /releases/latest/download/<name> out of whichever release is newest, and it
# can only do that for an asset whose NAME is the same in every release. These
# are byte-for-byte copies of the Windows setup and the Linux complete package,
# published beside them, and they are what the public homepage's two download
# buttons address (go/internal/archive/landing.go). Rename either one and the
# homepage silently starts serving a 404, so these strings are a contract with
# that file and with .github/workflows/release.yml.
STABLE_SETUP_NAME="bibites-multiverse-windows-x64-setup.exe"
STABLE_LINUX_COMPLETE_NAME="bibites-multiverse-linux-x64-complete.zip"
# The installed application's entry point. The shortcuts open this file, so the
# name is what a player sees in Task Manager and in a SmartScreen prompt.
LAUNCHER_NAME="BibitesMultiverseLauncher.exe"
MAKENSIS="${MAKENSIS:-$(command -v makensis || true)}"

# Optional, and only ever a check: an unpacked copy of the LINUX game, so the
# Linux matrix row's hash is verified against a real file at build time rather
# than trusted. Absent, the build says so and goes on - a machine that builds the
# release is not required to hold two copies of the game.
LINUX_GAME_DIR="${LINUX_GAME_DIR:-/mnt/wsl/data/scratch/m5-linux-rehearsal/game}"

# THE TOOLCHAIN, WITHOUT CLOBBERING ONE THAT IS ALREADY RIGHT. A release runner
# supplies both compilers through its own environment; this box has kept Go in
# $HOME/go and the .NET SDK in either the machine-wide /usr/local/dotnet or the
# per-user $HOME/.dotnet. Take a local convention only when it is real, and never
# overwrite a value somebody else set. A GOROOT that points at a directory which
# no longer exists does not merely fail to help: the toolchain obeys it, so it
# turns a perfectly good `go` on PATH into "cannot find GOROOT directory". The
# same reasoning applies to DOTNET_ROOT, which a dedicated runner user sets to a
# path outside its own home — so both are dropped when they name a directory
# that holds no compiler, and both are then looked for in a fixed order.
if [ -n "${GOROOT:-}" ] && [ ! -x "$GOROOT/bin/go" ]; then
  unset GOROOT
fi
if [ -z "${GOROOT:-}" ] && [ -x "$HOME/go/bin/go" ]; then
  export GOROOT="$HOME/go"
fi
[ -z "${GOROOT:-}" ] || export PATH="$GOROOT/bin:$PATH"
if [ -n "${DOTNET_ROOT:-}" ] && [ ! -x "$DOTNET_ROOT/dotnet" ]; then
  unset DOTNET_ROOT
fi
if [ -z "${DOTNET_ROOT:-}" ]; then
  for dotnet_candidate in /usr/local/dotnet "$HOME/.dotnet"; do
    if [ -x "$dotnet_candidate/dotnet" ]; then
      export DOTNET_ROOT="$dotnet_candidate"
      break
    fi
  done
fi
# Still nothing found: name the per-user path, so the preflight below fails with
# a message that says where it looked rather than with an empty variable.
export DOTNET_ROOT="${DOTNET_ROOT:-$HOME/.dotnet}"
export PATH="$DOTNET_ROOT:$DOTNET_ROOT/tools:$PATH"
export TZ=UTC

ALLOW_DIRTY=0
WINDOWS_GAME_PAYLOAD=''
LINUX_GAME_PAYLOAD=''
GAME_REDISTRIBUTION_NOTICE=''
while [ $# -gt 0 ]; do
  case "$1" in
    --allow-dirty) ALLOW_DIRTY=1; shift ;;
    --windows-game-dir) [ $# -gt 1 ] || { printf '%s needs a value\n' "$1" >&2; exit 2; }; WINDOWS_GAME_PAYLOAD="$2"; shift 2 ;;
    --linux-game-dir)   [ $# -gt 1 ] || { printf '%s needs a value\n' "$1" >&2; exit 2; }; LINUX_GAME_PAYLOAD="$2"; shift 2 ;;
    --game-redistribution-notice) [ $# -gt 1 ] || { printf '%s needs a value\n' "$1" >&2; exit 2; }; GAME_REDISTRIBUTION_NOTICE="$2"; shift 2 ;;
    *) printf 'usage: %s [--allow-dirty] [--windows-game-dir <dir>] [--linux-game-dir <dir>] [--game-redistribution-notice <file>]\n' "$0" >&2; exit 2 ;;
  esac
done

if { [ -n "$WINDOWS_GAME_PAYLOAD" ] || [ -n "$LINUX_GAME_PAYLOAD" ]; } && [ -z "$GAME_REDISTRIBUTION_NOTICE" ]; then
  printf 'complete packages require --game-redistribution-notice <permission notice file>\n' >&2
  exit 2
fi
if [ -n "$GAME_REDISTRIBUTION_NOTICE" ] && [ -z "$WINDOWS_GAME_PAYLOAD" ] && [ -z "$LINUX_GAME_PAYLOAD" ]; then
  printf '%s has no effect without --windows-game-dir or --linux-game-dir\n' '--game-redistribution-notice' >&2
  exit 2
fi

note() { printf '    %s\n' "$*"; }
step() { printf '\n=== %s\n' "$*"; }
die()  { printf '\n!! %s\n' "$*" >&2; exit 1; }

sha() { sha256sum "$1" | cut -d' ' -f1; }
SHA_UPPER() { sha "$1" | tr 'a-f' 'A-F'; }

# BOTH COMPILERS, PROVED NOW RATHER THAN DISCOVERED LATER. Go is needed within
# seconds; dotnet not until gate 3, three minutes and two cross-builds in. A
# runner user whose DOTNET_ROOT is wrong should learn that here, not there.
command -v go >/dev/null 2>&1 \
  || die "no go on PATH. Set GOROOT to a Go installation, or put go on PATH."
go version >/dev/null 2>&1 \
  || die "go is on PATH but will not run — GOROOT is ${GOROOT:-unset}: $(go version 2>&1 | head -n 1)"
command -v dotnet >/dev/null 2>&1 \
  || die "no dotnet on PATH. The plugin gate needs the .NET SDK; DOTNET_ROOT is $DOTNET_ROOT."

# ONE PARSE OF THE PLUGIN'S VERSION, shared with release/check-drift.sh so the
# gate and the check cannot disagree about what the declaration says.
MOD_VERSION_LIB="$RELDIR/lib/mod-version.sh"
[ -f "$MOD_VERSION_LIB" ] || die "missing $MOD_VERSION_LIB"
# shellcheck source=lib/mod-version.sh
. "$MOD_VERSION_LIB"

# ONE PARSE OF THE TESTED-BUILD RECORD, shared with release/check-drift.sh for
# the same reason: gates 2, 3 and E and the pull-request check all read the same
# object, and a second reader is a second opinion about what it says.
TESTED_BUILD_LIB="$RELDIR/lib/tested-build.sh"
[ -f "$TESTED_BUILD_LIB" ] || die "missing $TESTED_BUILD_LIB"
# shellcheck source=lib/tested-build.sh
. "$TESTED_BUILD_LIB"

# ------------------------------------------------------------------ the tree

step "the tree this release is built from"
DIRT="$(git -C "$REPO" status --porcelain)"
if [ -n "$DIRT" ]; then
  printf '%s\n' "$DIRT" | sed 's/^/    /'
  if [ "$ALLOW_DIRTY" -eq 0 ]; then
    die "the working tree is not clean. A Go binary records whether its tree was dirty, so a
   release built from here would ship an artifact that matches no published commit. Commit
   first, or pass --allow-dirty for a rehearsal that will not be published."
  fi
  note "--allow-dirty: this is a rehearsal and MUST NOT be published."
else
  note "clean at $(git -C "$REPO" rev-parse --short HEAD)"
fi

SOURCE_REV="$(git -C "$REPO" rev-parse HEAD)"
[ -d "$SIDECAR_BUILD_REPO/go" ] \
  || die "missing $SIDECAR_BUILD_REPO/go"
SIDECAR_BUILD_REV="$(git -C "$SIDECAR_BUILD_REPO" rev-parse HEAD)"
[ "$SIDECAR_BUILD_REV" = "$SOURCE_REV" ] \
  || die "the sidecar build checkout is $SIDECAR_BUILD_REV, want $SOURCE_REV"
if [ "$SIDECAR_BUILD_REPO" != "$REPO" ]; then
  [ -z "$(git -C "$SIDECAR_BUILD_REPO" status --porcelain)" ] \
    || die "the sidecar build checkout is dirty: $SIDECAR_BUILD_REPO"
  note "the clean sidecar build checkout matches this package revision"
fi

# THE VERSION SURFACE, before any of it is baked into an artifact. RELEASE above
# is the source of truth for a release string that also appears in the matrix,
# the launcher, the homepage default, both kits and the deploy defaults.
# release/bump-version.sh --check asserts they all still agree and lists the ones
# that do not. A release built from a surface that disagrees with itself ships a
# player an installer that names one version and downloads another, and no gate
# further down would notice. THE GATE IS NOT OPTIONAL and it is not skipped when
# the script looks unusable: a missing file or a lost mode bit is a broken
# checkout, and a gate that vanishes without a word is worse than no gate.
step "the version surface"
BUMP_VERSION="$RELDIR/bump-version.sh"
[ -f "$BUMP_VERSION" ] \
  || die "missing $BUMP_VERSION. It is where the release string is checked against every
   place that repeats it, and a release must not be built without that check."
[ -x "$BUMP_VERSION" ] \
  || die "$BUMP_VERSION is not executable, so the version-surface gate could only be
   skipped in silence. Restore the mode bit: chmod +x $BUMP_VERSION"
if BUMP_CHECK="$("$BUMP_VERSION" --check 2>&1)"; then
  printf '%s\n' "$BUMP_CHECK" | sed 's/^/    /'
else
  printf '%s\n' "$BUMP_CHECK" | sed 's/^/    /' >&2
  die "the release version is not consistent across the tree. Fix the places listed above —
   release/bump-version.sh $RELEASE rewrites them — and build again."
fi

# ------------------------------------------------------------------ the matrix

step "the support matrix"
[ -f "$MATRIX_DOC" ] || die "missing $MATRIX_DOC"
mkdir -p "$BUILD"
MATRIX_JSON="$BUILD/support-matrix.json"
# The extraction lives in release/lib/tested-build.sh, so the bytes this build
# copies into every archive and the bytes the pull-request check reads are
# produced by one function.
matrix_json_block "$MATRIX_DOC" "$MATRIX_JSON" \
  || die "the support matrix could not be read; see the line above"

# EVERY ENTRY CARRIES THE SAME KEYS. That is the matrix's own published rule and
# it is checked here rather than trusted, because both installers walk the whole
# list to print what the release supports: PowerShell under Set-StrictMode throws
# on a property one row happens not to have, and the shell reader would print an
# empty field. A key that exists on one row and not another is caught at build
# time, where it costs nothing.
python3 - "$MATRIX_JSON" <<'PY' || die "the matrix block's entries are not uniform"
import json, sys
required = {"gameVersion", "platform", "store", "storeBuild", "assemblySha256", "mod",
            "sidecar", "bepInEx", "bepInExFlavour", "contractA", "contractB", "tested"}
doc = json.load(open(sys.argv[1]))
seen = set()
for i, e in enumerate(doc["entries"]):
    missing, extra = required - set(e), set(e) - required
    if missing or extra:
        print("entry %d: missing %s, unexpected %s" % (i, sorted(missing), sorted(extra)))
        raise SystemExit(1)
    key = (e["gameVersion"], e["platform"])
    if key in seen:
        print("two rows for %s — a row is keyed on (gameVersion, platform)" % (key,))
        raise SystemExit(1)
    seen.add(key)
PY

matrix_field() { python3 -c "
import json,sys
print(json.load(open(sys.argv[1]))[sys.argv[2]])
" "$MATRIX_JSON" "$1"; }

matrix_row() { python3 -c "
import json,sys
rows=[e for e in json.load(open(sys.argv[1]))['entries'] if e['platform']==sys.argv[2]]
if len(rows)!=1: raise SystemExit('want exactly one %s row, found %d' % (sys.argv[2], len(rows)))
print(rows[0][sys.argv[3]])
" "$MATRIX_JSON" "$1" "$2"; }

MATRIX_RELEASE="$(matrix_field release)"
[ "$MATRIX_RELEASE" = "$RELEASE" ] \
  || die "$MATRIX_DOC says release \"$MATRIX_RELEASE\"; this script builds \"$RELEASE\""
GAME_VERSION="$(matrix_row Windows gameVersion)"
GAME_SHA="$(matrix_row Windows assemblySha256)"
LINUX_GAME_SHA="$(matrix_row Linux assemblySha256)"
MOD_VERSION="$(matrix_row Windows mod)"
SIDECAR_VERSION="$(matrix_row Windows sidecar)"
BEPINEX_VERSION="$(matrix_row Windows bepInEx)"
WIN_FLAVOUR="$(matrix_row Windows bepInExFlavour)"
LINUX_FLAVOUR="$(matrix_row Linux bepInExFlavour)"
[ "$(matrix_row Linux gameVersion)" = "$GAME_VERSION" ] \
  || die "the two matrix rows name different game versions; this release ships one"
[ "$(matrix_row Linux mod)" = "$MOD_VERSION" ] \
  || die "the two matrix rows name different mod versions; the plugin is one file for both"
[ "$(matrix_row Linux bepInEx)" = "$BEPINEX_VERSION" ] \
  || die "the two matrix rows name different BepInEx versions"
note "release $RELEASE — game $GAME_VERSION, mod $MOD_VERSION, sidecar $SIDECAR_VERSION, BepInEx $BEPINEX_VERSION"
note "two platforms: Windows ($WIN_FLAVOUR) and Linux ($LINUX_FLAVOUR)"

# ------------------------------------------------------------------ the tested build

step "the tested build this release is measured against"
# THE RELEASE REFERENCE, read once and used by three gates below. It is a
# record rather than a binary: whoever tested a build wrote down what they
# tested, and gates 2, 3 and E refuse anything else. The library validates the
# whole object on every read, so a malformed record fails here rather than
# turning a gate into a comparison against nothing.
TB_MOD="$(tested_build_field "$MATRIX_JSON" mod)" \
  || die "the tested-build record in $MATRIX_DOC cannot be read; see above.
   It is the reference this build compares against, so there is nothing to compare to."
TB_PLUGIN_SHA="$(tested_build_field "$MATRIX_JSON" pluginSha256)"
TB_SIDECAR_COMMIT="$(tested_build_field "$MATRIX_JSON" sidecarSourceCommit)"
TB_SIDECAR_INPUTS="$(tested_build_field "$MATRIX_JSON" sidecarInputsSha256)"
TB_TESTED_ON="$(tested_build_field "$MATRIX_JSON" testedOn)"
note "mod $TB_MOD, tested on $TB_TESTED_ON"
note "plugin  $TB_PLUGIN_SHA"
note "sidecar source $TB_SIDECAR_COMMIT, inputs $TB_SIDECAR_INPUTS"

# ------------------------------------------------------------------ the mod version

step "the mod version this release claims"
# THE PLUGIN'S OWN VERSION AGAINST THE MATRIX AND THE TESTED-BUILD RECORD, on
# plain text, before anything is compiled. The matrix's `mod` field is what both
# installers print and what the published table promises; MultiversePlugin.cs is
# what BepInEx registers at runtime; testedBuild.mod is what somebody says they
# tested. Nothing else in this build compares the three, so a release could ship
# them disagreeing and say nothing — the plugin identity gate further down
# compares a HASH and is blind to what those bytes call themselves. This gate
# sits here, seconds into the run, because it needs no game, no .NET and no Go.
PLUGIN_SOURCE="$REPO/bibites-mod/src/MultiversePlugin.cs"
PLUGIN_VERSION="$(mod_version "$PLUGIN_SOURCE")" \
  || die "the plugin's version cannot be read; see above. release/lib/mod-version.sh holds
   the one parse this gate and release/check-drift.sh both use."
# The two rows were proved to name the same mod above, so $MOD_VERSION is both.
if [ "$PLUGIN_VERSION" != "$MOD_VERSION" ] || [ "$PLUGIN_VERSION" != "$TB_MOD" ]; then
  echo "!! bibites-mod/src/MultiversePlugin.cs says $PLUGIN_VERSION" >&2
  echo "!! docs/support-matrix.md \"mod\" says $MOD_VERSION on both rows" >&2
  echo "!! docs/support-matrix.md testedBuild.mod says $TB_MOD" >&2
  die "the mod version this tree declares, the version the matrix publishes and the version
   the tested-build record names do not all agree, so this release would print one number
   and load another. Decide which is right, then make the whole set agree: the Version
   constant, the \"mod\" field on BOTH matrix rows, testedBuild.mod, and the Plugin row of
   dev_environment.md. If the plugin itself changed, the record is not a text edit: test the
   new plugin, refresh this machine's game plugin (bibites-mod/deploy.sh), and write the
   whole testedBuild block from release/record-tested-build.sh."
fi
note "the plugin source, both matrix rows and the tested-build record say mod $PLUGIN_VERSION"

# ------------------------------------------------------------------ the game pin

step "the game build this release is compiled against"
GAME_DLL="$REPO/bibites-mod/libs/BibitesAssembly.dll"
[ -f "$GAME_DLL" ] || die "missing $GAME_DLL — run bibites-mod/sync-game-refs.sh"
GOT_GAME_SHA="$(SHA_UPPER "$GAME_DLL")"
if [ "$GOT_GAME_SHA" != "$GAME_SHA" ]; then
  echo "!! docs/support-matrix.md pins $GAME_SHA" >&2
  echo "!! bibites-mod/libs/BibitesAssembly.dll is $GOT_GAME_SHA" >&2
  die "the game moved. Re-sync the references, rebuild, TEST, and then add a matrix row —
   a matrix entry is a claim that a build was run against that game version."
fi
note "libs/BibitesAssembly.dll matches the matrix: game $GAME_VERSION on Windows"

# The Linux row's hash, against a real file when there is one to check. The mod
# is compiled against the WINDOWS reference assembly and runs on both, which is
# the finding the Linux row rests on — so the one thing worth proving here is
# that the hash in the row is the hash of a file somebody actually has.
# ONLY THE LAST PATH COMPONENT IS PRINTED on the success path. This build's log
# is uploaded as an artifact of a PUBLIC repository, and where the owner keeps
# an unpacked copy of the game is nobody else's business. The die still names
# the file: a failing build is being read by the owner, and the whole point of
# that message is to say which file did not match.
LINUX_ASSEMBLY="$LINUX_GAME_DIR/The Bibites_Data/Managed/BibitesAssembly.dll"
if [ -f "$LINUX_ASSEMBLY" ]; then
  GOT_LINUX_SHA="$(SHA_UPPER "$LINUX_ASSEMBLY")"
  [ "$GOT_LINUX_SHA" = "$LINUX_GAME_SHA" ] \
    || die "the Linux row pins $LINUX_GAME_SHA but $LINUX_ASSEMBLY is $GOT_LINUX_SHA"
  note "the Linux game at $(basename "$LINUX_GAME_DIR") matches the matrix's Linux row"
else
  note "no unpacked Linux game at $(basename "$LINUX_GAME_DIR"); the Linux row's hash stands on its record"
fi

# ------------------------------------------------------------------ the sidecar

step "the Windows sidecar (cross-compiled)"
# GATE 2 — cmd/sidecar's INPUTS AGAINST THE TESTED BUILD. A Go binary carries a
# VCS stamp, so two builds from identical inputs at two commits differ as files
# and the hash of a binary cannot be the reference. The manifest is what can be
# compared: every main-module source file `go list -deps` reaches from
# cmd/sidecar, with its sha256, plus the identity of every external module that
# graph selects, so added, removed and build-tagged files are all covered.
# Standard-library packages use the local toolchain and are not in it. The
# digest of that manifest at the tested source is
# testedBuild.sidecarInputsSha256, and this gate recomputes it here.
#
# THE MANIFEST IS COMPUTED FROM THE WORKING TREE AND NOTHING ELSE — no `git
# archive` of an older commit, so no history. That is what lets CI run this
# identical gate on a shallow clone. testedBuild.sidecarSourceCommit is kept for
# the failure message, where it names the diff a person should read.
#
# THE MANIFEST FUNCTION LIVES IN release/lib/sidecar-manifest.sh. It is sourced
# rather than written twice, because release/check-drift.sh runs this same gate
# without any of the game bytes the rest of this script needs. One copy, so the
# check that runs on every pull request cannot drift away from the one that
# runs here.
SIDECAR_MANIFEST_LIB="$RELDIR/lib/sidecar-manifest.sh"
[ -f "$SIDECAR_MANIFEST_LIB" ] || die "missing $SIDECAR_MANIFEST_LIB"
# shellcheck source=lib/sidecar-manifest.sh
. "$SIDECAR_MANIFEST_LIB"

CURRENT_MANIFEST="$BUILD/sidecar-inputs-current.txt"
sidecar_manifest "$SIDECAR_BUILD_REPO" "$CURRENT_MANIFEST" "$BUILD/sidecar-packages-current.json"
GOT_SIDECAR_INPUTS="$(sha "$CURRENT_MANIFEST")"
if [ "$GOT_SIDECAR_INPUTS" != "$TB_SIDECAR_INPUTS" ]; then
  echo "!! this tree's cmd/sidecar inputs digest to $GOT_SIDECAR_INPUTS" >&2
  echo "!! docs/support-matrix.md testedBuild.sidecarInputsSha256 is $TB_SIDECAR_INPUTS" >&2
  echo "!! the manifest just computed is $CURRENT_MANIFEST" >&2
  die "the files or modules that build cmd/sidecar are not the ones that were tested. The
   tested source is commit $TB_SIDECAR_COMMIT; read what moved with
     git diff $TB_SIDECAR_COMMIT HEAD -- go/
   Then either test this sidecar and record the build it makes
   (release/record-tested-build.sh), or build the release from the tested source."
fi
note "cmd/sidecar inputs match the recorded tested build: $GOT_SIDECAR_INPUTS"
note "that manifest holds $(grep -c '^file' "$CURRENT_MANIFEST") file entries and $(grep -c '^module' "$CURRENT_MANIFEST") module entries"

( cd "$SIDECAR_BUILD_REPO/go" && nice -n 19 env GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
    go build -buildvcs=true -o "$BUILD/multiverse-sidecar.exe" ./cmd/sidecar )
BUILT_REV="$(go version -m "$BUILD/multiverse-sidecar.exe" \
  | sed -n 's/^[[:space:]]*build[[:space:]]*vcs\.revision=//p')"
[ "$BUILT_REV" = "$SOURCE_REV" ] \
  || die "the Windows sidecar VCS stamp is '${BUILT_REV:-missing}', want $SOURCE_REV"
BUILT_SIDECAR_SHA="$(sha "$BUILD/multiverse-sidecar.exe")"
# NOT A GATE, AND IT NEVER WAS ONE. The tested sidecar's BYTES are not the
# reference — its inputs are, above — so what belongs in the log is what this
# build produced and which commit it records.
note "$BUILT_SIDECAR_SHA"
note "stamped $BUILT_REV; the tested source is $TB_SIDECAR_COMMIT"

# ------------------------------------------------------------------ the Linux sidecar

step "the Linux sidecar (cross-compiled)"
# ONE MANIFEST COVERS BOTH BUILDS. The gate above lists the package graph for
# GOOS=windows, and the Linux binary is built from the same Go package in the
# same tree, so nothing further is compared here. Neither manifest proves that
# either binary ran.
( cd "$SIDECAR_BUILD_REPO/go" && nice -n 19 env GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
    go build -buildvcs=true -o "$BUILD/multiverse-sidecar" ./cmd/sidecar )
LINUX_BUILT_REV="$(go version -m "$BUILD/multiverse-sidecar" \
  | sed -n 's/^[[:space:]]*build[[:space:]]*vcs\.revision=//p')"
[ "$LINUX_BUILT_REV" = "$SOURCE_REV" ] \
  || die "the Linux sidecar VCS stamp is '${LINUX_BUILT_REV:-missing}', want $SOURCE_REV"
note "$(sha "$BUILD/multiverse-sidecar")"
note "same inputs as the recorded tested build ($TB_SIDECAR_INPUTS), stamped $LINUX_BUILT_REV, built for linux/amd64"
note "static: CGO is off, so it needs no libc of a particular vintage"

# ------------------------------------------------------------------ the launcher

step "the launcher (cross-compiled, both platforms)"
# THIS BINARY IS NOT IN THE SIDECAR MANIFEST GATE ABOVE. That gate digests the
# package graph of cmd/sidecar, and the launcher is a separate command that
# shares none of it. What stands in for it
# here is the same VCS-stamp rule the sidecar gets: the shipped file must record
# the commit this package is built from, so a downloaded exe names a public
# revision. The launcher uses the standard library only, so there is no module
# set to compare.
# VET BOTH TARGETS. A vet at the host GOOS never opens proc_windows.go, which
# is the highest-risk file in the launcher and the one no test here can run.
# The GOOS=windows build below catches compile errors in it but not a vet
# diagnostic, so the second line is not a duplicate of the first.
( cd "$SIDECAR_BUILD_REPO/go" && nice -n 19 go vet ./cmd/multiverse-launcher ./internal/launcher )
( cd "$SIDECAR_BUILD_REPO/go" && nice -n 19 env GOOS=windows GOARCH=amd64 \
    go vet ./cmd/multiverse-launcher ./internal/launcher )
( cd "$SIDECAR_BUILD_REPO/go" && nice -n 19 env GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
    go build -buildvcs=true -o "$BUILD/$LAUNCHER_NAME" ./cmd/multiverse-launcher )
LAUNCHER_REV="$(go version -m "$BUILD/$LAUNCHER_NAME" \
  | sed -n 's/^[[:space:]]*build[[:space:]]*vcs\.revision=//p')"
[ "$LAUNCHER_REV" = "$SOURCE_REV" ] \
  || die "the launcher VCS stamp is '${LAUNCHER_REV:-missing}', want $SOURCE_REV"
file "$BUILD/$LAUNCHER_NAME" | grep -q 'PE32+' \
  || die "$LAUNCHER_NAME is not a Windows executable"
# Linux is a compile gate only; the Linux kit does not ship the launcher yet.
( cd "$SIDECAR_BUILD_REPO/go" && nice -n 19 env GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
    go build -buildvcs=true -o "$BUILD/bibites-multiverse-launcher" ./cmd/multiverse-launcher )
note "$(sha "$BUILD/$LAUNCHER_NAME")"
note "linux/amd64 build succeeds; the Linux kit keeps its shell scripts in this release"

# ------------------------------------------------------------------ the plugin

step "the plugin (fresh Release build)"
nice -n 19 dotnet build "$REPO/bibites-mod/BibitesMultiverse.csproj" -c Release \
  --nologo -v quiet -clp:ErrorsOnly
PLUGIN="$REPO/bibites-mod/bin/Release/BibitesMultiverse.dll"
[ -f "$PLUGIN" ] || die "the build produced no $PLUGIN"
# GATE 3 — THE PLUGIN AGAINST THE TESTED BUILD. The freshly compiled DLL must be
# the DLL somebody tested, and the record names it by hash. This also assumes a
# deterministic .NET build on this machine: a toolchain update makes the fresh
# build differ from any recorded hash, and the answer then is to test the new
# build and record it rather than to relax the gate.
BUILT_PLUGIN_SHA="$(sha "$PLUGIN")"
if [ "$BUILT_PLUGIN_SHA" != "$TB_PLUGIN_SHA" ]; then
  echo "!! this tree builds                          $BUILT_PLUGIN_SHA" >&2
  echo "!! docs/support-matrix.md testedBuild.pluginSha256 is $TB_PLUGIN_SHA" >&2
  die "the mod in this tree is not the mod that was tested. Test the plugin this tree
   builds, refresh this machine's game plugin (bibites-mod/deploy.sh), then record what
   ran: release/record-tested-build.sh prints the testedBuild block to paste into
   docs/support-matrix.md. Only then build the release."
fi
note "byte-identical to the recorded tested plugin"

DEPLOYED="/mnt/c/Program Files (x86)/Steam/steamapps/common/The Bibites/BepInEx/plugins/BibitesMultiverse.dll"
if [ -f "$DEPLOYED" ]; then
  if [ "$(sha256sum <"$DEPLOYED" | cut -d' ' -f1)" != "$BUILT_PLUGIN_SHA" ]; then
    die "the plugin in this machine's game is a different build again.
   Three copies must agree before a release: this tree, the tested-build record, and the
   plugin this machine's game actually loads (bibites-mod/deploy.sh)."
  fi
  note "byte-identical to the plugin in this machine's game"
else
  note "this machine's game directory is not readable from here; the recorded hash stands alone"
fi

# ------------------------------------------------------------------ BepInEx

step "BepInEx $BEPINEX_VERSION, both flavours"
BEPINEX_ZIP="BepInEx_${WIN_FLAVOUR}_${BEPINEX_VERSION}.zip"
BEPINEX_SHA=41a089e5b1b1f0713b331346baf6677b1184c69eabebf51101097954e854c749
BEPINEX_LINUX_ZIP="BepInEx_${LINUX_FLAVOUR}_${BEPINEX_VERSION}.zip"
BEPINEX_LINUX_SHA=b9ef28b37676f18277cfd8dded01f675e934f207c30f65f2a4bb93c7e41abdbb
mkdir -p "$CACHE"

fetch_bepinex() { # $1 file name, $2 expected sha256
  local url="https://github.com/BepInEx/BepInEx/releases/download/v${BEPINEX_VERSION}/$1" got
  if [ ! -f "$CACHE/$1" ]; then
    note "downloading $url"
    curl -fsSL -o "$CACHE/$1.tmp" "$url"
    mv -f "$CACHE/$1.tmp" "$CACHE/$1"
  fi
  got="$(sha "$CACHE/$1")"
  [ "$got" = "$2" ] || die "$1 is $got, want $2"
  note "$1 sha256 verified: $got"
}
fetch_bepinex "$BEPINEX_ZIP"       "$BEPINEX_SHA"
fetch_bepinex "$BEPINEX_LINUX_ZIP" "$BEPINEX_LINUX_SHA"

# The Linux archive carries BepInEx's own launcher, and the mode bit on it is
# what makes it a launcher rather than a text file. zip stores the mode; the
# staging below preserves it; the installer sets it again by name anyway, after
# the checksum. Three places, because a lost executable bit is silent.
unzip -Z1 "$CACHE/$BEPINEX_LINUX_ZIP" | grep -qx 'run_bepinex.sh' \
  || die "$BEPINEX_LINUX_ZIP holds no run_bepinex.sh — the Linux kit's launcher is that file"

# ------------------------------------------------------------------ staging

step "staging the add-on archives and any complete editions"
[ -f "$PROJECT_LICENSE" ] || die "missing $PROJECT_LICENSE"
[ -f "$THIRD_PARTY_NOTICES" ] || die "missing $THIRD_PARTY_NOTICES"
STAGE="$DIST/stage/windows/$STAGE_NAME"
LINUX_STAGE="$DIST/stage/linux/$STAGE_NAME"
COMPLETE_STAGE="$DIST/stage/windows-complete/$STAGE_NAME"
LINUX_COMPLETE_STAGE="$DIST/stage/linux-complete/$STAGE_NAME"
rm -rf "$DIST/stage" "$DIST/$ZIP_NAME" "$DIST/$LINUX_ZIP_NAME" \
       "$DIST/$COMPLETE_ZIP_NAME" "$DIST/$LINUX_COMPLETE_ZIP_NAME" \
       "$DIST/$WINDOWS_SETUP_NAME" \
       "$DIST/$STABLE_SETUP_NAME" "$DIST/$STABLE_LINUX_COMPLETE_NAME"
mkdir -p "$STAGE" "$LINUX_STAGE"

cp "$RELDIR/kit/Install-BibitesMultiverse.ps1"   "$STAGE/"
cp "$RELDIR/kit/Install-BibitesMultiverse-Gui.ps1" "$STAGE/"
cp "$RELDIR/kit/Install-BibitesMultiverse.cmd"   "$STAGE/"
cp "$RELDIR/kit/Find-BibitesGame.ps1"            "$STAGE/"
cp "$RELDIR/kit/public-map.json"                  "$STAGE/"
cp "$RELDIR/kit/Uninstall-BibitesMultiverse.ps1" "$STAGE/"
cp "$RELDIR/kit/README.md"                       "$STAGE/"
cp "$RELDIR/kit/bibites-multiverse.ico"          "$STAGE/"
cp "$MATRIX_JSON"                                "$STAGE/support-matrix.json"
cp "$PLUGIN"                                     "$STAGE/BibitesMultiverse.dll"
cp "$BUILD/multiverse-sidecar.exe"               "$STAGE/"
cp "$BUILD/$LAUNCHER_NAME"                       "$STAGE/"
cp "$CACHE/$BEPINEX_ZIP"                         "$STAGE/"
cp "$PROJECT_LICENSE"                            "$STAGE/LICENSE"
cp "$THIRD_PARTY_NOTICES"                        "$STAGE/THIRD_PARTY_NOTICES.md"

cp "$RELDIR/kit/install-bibites-multiverse.sh"   "$LINUX_STAGE/"
cp "$RELDIR/kit/uninstall-bibites-multiverse.sh" "$LINUX_STAGE/"
cp "$RELDIR/kit/README-linux.md"                 "$LINUX_STAGE/README.md"
cp "$RELDIR/kit/public-map.json"                  "$LINUX_STAGE/"
cp "$MATRIX_JSON"                                "$LINUX_STAGE/support-matrix.json"
# THE SAME FILE, not a second build of it. The plugin is platform-independent IL.
cp "$PLUGIN"                                     "$LINUX_STAGE/BibitesMultiverse.dll"
cp "$BUILD/multiverse-sidecar"                   "$LINUX_STAGE/"
cp "$CACHE/$BEPINEX_LINUX_ZIP"                   "$LINUX_STAGE/"
cp "$PROJECT_LICENSE"                            "$LINUX_STAGE/LICENSE"
cp "$THIRD_PARTY_NOTICES"                        "$LINUX_STAGE/THIRD_PARTY_NOTICES.md"
[ "$(sha "$STAGE/BibitesMultiverse.dll")" = "$(sha "$LINUX_STAGE/BibitesMultiverse.dll")" ] \
  || die "the two archives hold different plugins, which this release does not do"
chmod +x "$LINUX_STAGE/install-bibites-multiverse.sh" \
         "$LINUX_STAGE/uninstall-bibites-multiverse.sh" \
         "$LINUX_STAGE/multiverse-sidecar"

# The Linux scripts must survive the trip as they were written: LF, and no
# byte-order mark. bash reads a CR at the end of a line as part of the last word
# and fails in ways that read like a corrupted download.
for f in install-bibites-multiverse.sh uninstall-bibites-multiverse.sh README.md LICENSE THIRD_PARTY_NOTICES.md; do
  ! grep -q $'\r' "$LINUX_STAGE/$f" || die "$f carries CRLF; the Linux kit ships LF"
done
for f in install-bibites-multiverse.sh uninstall-bibites-multiverse.sh; do
  nice -n 19 bash -n "$LINUX_STAGE/$f" || die "$f does not parse under bash -n"
done
note "the Linux scripts parse, are LF, and carry their executable bit"

# PowerShell reads LF, but a player who opens a script in Notepad should not see
# one long line. Done here rather than with unix2dos, which is not on every
# machine: whether a tool happens to be installed must not change the archive's
# checksum. THE LINUX KIT IS DELIBERATELY NOT IN THIS LIST.
#
# THIS LIST IS TEXT FILES ONLY, and it is written out by name for that reason. A
# binary — the sidecar, the launcher, the plugin, the icon, a BepInEx zip — would
# be rewritten byte for byte by the replacement below and would no longer run.
# Never add one.
for f in Install-BibitesMultiverse.cmd Install-BibitesMultiverse.ps1 Install-BibitesMultiverse-Gui.ps1 Find-BibitesGame.ps1 Uninstall-BibitesMultiverse.ps1 README.md LICENSE THIRD_PARTY_NOTICES.md; do
  python3 -c "
import sys
p = sys.argv[1]
data = open(p, 'rb').read().replace(b'\r\n', b'\n').replace(b'\n', b'\r\n')
open(p, 'wb').write(data)
" "$STAGE/$f"
done

# A complete archive is the add-on archive plus a permitted, unmodified game
# payload. Proprietary game bytes are never read from the repository and never
# committed: the game directory is an explicit build input. The
# same installer detects game-payload.json and selects the managed-runtime path.
validate_game_payload() { # $1 source, $2 platform, $3 executable, $4 assembly sha
  local source="$1" platform="$2" executable="$3" expected_sha="$4" assembly got bad
  [ -d "$source" ] || die "$platform game payload directory does not exist: $source"
  [ -f "$source/$executable" ] || die "$platform game payload has no $executable"
  assembly="$source/The Bibites_Data/Managed/BibitesAssembly.dll"
  [ -f "$assembly" ] || die "$platform game payload has no BibitesAssembly.dll"
  got="$(SHA_UPPER "$assembly")"
  [ "$got" = "$expected_sha" ] || die "$platform game payload assembly is $got, want matrix hash $expected_sha"
  bad="$(find "$source" -type l -print -quit)"
  [ -z "$bad" ] || die "$platform game payload contains a symbolic link: $bad"
  for bad in BepInEx winhttp.dll version.dll doorstop_config.ini run_bepinex.sh libdoorstop.so .doorstop_version; do
    [ ! -e "$source/$bad" ] || die "$platform game payload is already modded ($bad exists). Use a clean source game directory."
  done
  if [ "$platform" = Linux ]; then
    [ -x "$source/$executable" ] || die "$source/$executable is not executable"
  fi
  note "$platform complete payload: game $GAME_VERSION, assembly $got"
}

stage_complete() { # $1 add-on stage, $2 complete stage, $3 game source, $4 platform, $5 executable, $6 assembly sha
  local addon="$1" complete="$2" source="$3" platform="$4" executable="$5" expected_sha="$6"
  validate_game_payload "$source" "$platform" "$executable" "$expected_sha"
  [ -f "$GAME_REDISTRIBUTION_NOTICE" ] && [ -s "$GAME_REDISTRIBUTION_NOTICE" ] && [ ! -L "$GAME_REDISTRIBUTION_NOTICE" ] \
    || die "game redistribution notice must be a non-empty regular file, not a symbolic link: $GAME_REDISTRIBUTION_NOTICE"
  mkdir -p "$complete"
  cp -a "$addon/." "$complete/"
  mkdir -p "$complete/game"
  cp -a "$source/." "$complete/game/"
  cp "$GAME_REDISTRIBUTION_NOTICE" "$complete/GAME-REDISTRIBUTION-NOTICE.txt"
  python3 - "$complete/game-payload.json" "$platform" "$GAME_VERSION" "$expected_sha" <<'PY'
import json, sys
path, platform, version, assembly_sha = sys.argv[1:]
doc = {
    "format": "bibites-multiverse/game-payload/1",
    "platform": platform,
    "gameVersion": version,
    "assemblySha256": assembly_sha,
    "redistributionNoticeFile": "GAME-REDISTRIBUTION-NOTICE.txt",
}
with open(path, "w", encoding="utf-8", newline="\n") as f:
    json.dump(doc, f, indent=2)
    f.write("\n")
PY
}

if [ -n "$WINDOWS_GAME_PAYLOAD" ]; then
  stage_complete "$STAGE" "$COMPLETE_STAGE" "$WINDOWS_GAME_PAYLOAD" Windows 'The Bibites.exe' "$GAME_SHA"
fi
if [ -n "$LINUX_GAME_PAYLOAD" ]; then
  stage_complete "$LINUX_STAGE" "$LINUX_COMPLETE_STAGE" "$LINUX_GAME_PAYLOAD" Linux 'The Bibites.x86_64' "$LINUX_GAME_SHA"
fi

# The manifest each installer checks itself against, before it touches anything.
# It cannot cover itself, which is what the archive's own published checksum is
# for: the page's checksum covers the archive, the archive's manifest covers the
# files, and the installer verifies the second before it uses any of them.
MANIFEST_STAGES=("$STAGE" "$LINUX_STAGE")
[ -z "$WINDOWS_GAME_PAYLOAD" ] || MANIFEST_STAGES+=("$COMPLETE_STAGE")
[ -z "$LINUX_GAME_PAYLOAD" ] || MANIFEST_STAGES+=("$LINUX_COMPLETE_STAGE")
for s in "${MANIFEST_STAGES[@]}"; do
  ( cd "$s" && find . -type f ! -name MANIFEST.sha256 -printf '%P\n' | LC_ALL=C sort \
      | while read -r f; do printf '%s  %s\n' "$(sha256sum "$f" | cut -d' ' -f1)" "$f"; done \
      > MANIFEST.sha256 )
done
note "MANIFEST.sha256: $(grep -c . "$STAGE/MANIFEST.sha256") files (Windows), $(grep -c . "$LINUX_STAGE/MANIFEST.sha256") files (Linux)"

# ------------------------------------------------------------------ the archives

step "the archives"
# Deterministic: one fixed timestamp, sorted entry order, no extra attributes —
# so the owner can rebuild these from the tag and get the same checksums. The
# Linux archive keeps its mode bits, which is what -X does NOT strip: -X drops
# the extra fields (uid, gid, timestamps), and the Unix permission bits travel in
# the entry's external attributes rather than in one of those.
SOURCE_DATE="$(git -C "$REPO" log -1 --format=%cI)"
export SOURCE_DATE_EPOCH="$(git -C "$REPO" log -1 --format=%ct)"
find "$DIST/stage" -exec touch -d "$SOURCE_DATE" {} +
( cd "$DIST/stage/windows" && find . -type f -printf '%P\n' | LC_ALL=C sort \
    | zip -q -X -@ "$DIST/$ZIP_NAME" )
( cd "$DIST/stage/linux" && find . -type f -printf '%P\n' | LC_ALL=C sort \
    | zip -q -X -@ "$DIST/$LINUX_ZIP_NAME" )
if [ -n "$WINDOWS_GAME_PAYLOAD" ]; then
  ( cd "$DIST/stage/windows-complete" && find . -type f -printf '%P\n' | LC_ALL=C sort \
      | zip -q -X -@ "$DIST/$COMPLETE_ZIP_NAME" )
fi
if [ -n "$LINUX_GAME_PAYLOAD" ]; then
  ( cd "$DIST/stage/linux-complete" && find . -type f -printf '%P\n' | LC_ALL=C sort \
      | zip -q -X -@ "$DIST/$LINUX_COMPLETE_ZIP_NAME" )
fi

WINDOWS_SETUP_SHA=''; WINDOWS_SETUP_SIZE=''
if [ -n "$WINDOWS_GAME_PAYLOAD" ]; then
  [ -n "$MAKENSIS" ] && [ -x "$MAKENSIS" ] \
    || die "the Windows complete edition needs makensis 3.09 or newer"
  step "the single-file Windows setup"
  nice -n 19 "$MAKENSIS" -V2 \
    -DPRODUCT_VERSION="$RELEASE" \
    -DPACKAGE_DIR="$COMPLETE_STAGE" \
    -DOUTPUT_FILE="$DIST/$WINDOWS_SETUP_NAME" \
    -DPRODUCT_ICON="$RELDIR/kit/bibites-multiverse.ico" \
    "$RELDIR/windows-installer.nsi"
  file "$DIST/$WINDOWS_SETUP_NAME" | grep -q 'Nullsoft Installer self-extracting archive' \
    || die "$WINDOWS_SETUP_NAME is not an NSIS executable"
  WINDOWS_SETUP_SHA="$(sha "$DIST/$WINDOWS_SETUP_NAME")"
  WINDOWS_SETUP_SIZE="$(numfmt --to=iec --suffix=B "$(stat -c %s "$DIST/$WINDOWS_SETUP_NAME")")"
  note "$DIST/$WINDOWS_SETUP_NAME  ($WINDOWS_SETUP_SIZE)"
  note "sha256 $WINDOWS_SETUP_SHA"
fi

ZIP_SHA="$(sha "$DIST/$ZIP_NAME")"
ZIP_BYTES="$(stat -c %s "$DIST/$ZIP_NAME")"
ZIP_SIZE="$(numfmt --to=iec --suffix=B "$ZIP_BYTES")"
LINUX_ZIP_SHA="$(sha "$DIST/$LINUX_ZIP_NAME")"
LINUX_ZIP_BYTES="$(stat -c %s "$DIST/$LINUX_ZIP_NAME")"
LINUX_ZIP_SIZE="$(numfmt --to=iec --suffix=B "$LINUX_ZIP_BYTES")"
ARCHIVE_NAMES=("$ZIP_NAME" "$LINUX_ZIP_NAME")
COMPLETE_ZIP_SHA=''; COMPLETE_ZIP_SIZE=''
LINUX_COMPLETE_ZIP_SHA=''; LINUX_COMPLETE_ZIP_SIZE=''
if [ -n "$WINDOWS_GAME_PAYLOAD" ]; then
  COMPLETE_ZIP_SHA="$(sha "$DIST/$COMPLETE_ZIP_NAME")"
  COMPLETE_ZIP_SIZE="$(numfmt --to=iec --suffix=B "$(stat -c %s "$DIST/$COMPLETE_ZIP_NAME")")"
  ARCHIVE_NAMES+=("$COMPLETE_ZIP_NAME")
fi
if [ -n "$LINUX_GAME_PAYLOAD" ]; then
  LINUX_COMPLETE_ZIP_SHA="$(sha "$DIST/$LINUX_COMPLETE_ZIP_NAME")"
  LINUX_COMPLETE_ZIP_SIZE="$(numfmt --to=iec --suffix=B "$(stat -c %s "$DIST/$LINUX_COMPLETE_ZIP_NAME")")"
  ARCHIVE_NAMES+=("$LINUX_COMPLETE_ZIP_NAME")
fi
CHECKSUM_NAMES=("${ARCHIVE_NAMES[@]}")
[ -z "$WINDOWS_SETUP_SHA" ] || CHECKSUM_NAMES+=("$WINDOWS_SETUP_NAME")

# The stable-named copies: the same bytes under a name that carries no release,
# so https://github.com/<repo>/releases/latest/download/<name> resolves to this
# release the moment it becomes the newest one. That is the whole mechanism the
# homepage relies on, and it is why no archive rebuild or deployment is part of
# a release any more. Both names go into SHA256SUMS as extra lines, so
# `sha256sum -c` still verifies the file for somebody who downloaded the stable
# name rather than the versioned one.
STABLE_NAMES=()
if [ -n "$WINDOWS_SETUP_SHA" ]; then
  cp "$DIST/$WINDOWS_SETUP_NAME" "$DIST/$STABLE_SETUP_NAME"
  STABLE_NAMES+=("$STABLE_SETUP_NAME")
fi
if [ -n "$LINUX_GAME_PAYLOAD" ]; then
  cp "$DIST/$LINUX_COMPLETE_ZIP_NAME" "$DIST/$STABLE_LINUX_COMPLETE_NAME"
  STABLE_NAMES+=("$STABLE_LINUX_COMPLETE_NAME")
fi
if [ "${#STABLE_NAMES[@]}" -gt 0 ]; then
  CHECKSUM_NAMES+=("${STABLE_NAMES[@]}")
  note "stable-named copies for the homepage: ${STABLE_NAMES[*]}"
fi
( cd "$DIST" && sha256sum "${CHECKSUM_NAMES[@]}" > SHA256SUMS )

# The copies are copies. A truncated one would still be listed in SHA256SUMS
# under its own digest, so the check that matters is that its digest equals the
# versioned file's.
if [ -n "$WINDOWS_SETUP_SHA" ]; then
  [ "$(sha "$DIST/$STABLE_SETUP_NAME")" = "$WINDOWS_SETUP_SHA" ] \
    || die "$STABLE_SETUP_NAME is not a copy of $WINDOWS_SETUP_NAME"
fi
if [ -n "$LINUX_GAME_PAYLOAD" ]; then
  [ "$(sha "$DIST/$STABLE_LINUX_COMPLETE_NAME")" = "$LINUX_COMPLETE_ZIP_SHA" ] \
    || die "$STABLE_LINUX_COMPLETE_NAME is not a copy of $LINUX_COMPLETE_ZIP_NAME"
fi

# The executable bit, read back out of the archive that will actually be
# downloaded. A player who unzips a kit whose scripts lost their mode has a
# refusal to debug that no page explains.
unzip -Z "$DIST/$LINUX_ZIP_NAME" "$STAGE_NAME/install-bibites-multiverse.sh" \
  | grep -q '^-rwx' || die "the Linux archive lost the executable bit on the installer"
if [ -n "$LINUX_GAME_PAYLOAD" ]; then
  unzip -Z "$DIST/$LINUX_COMPLETE_ZIP_NAME" "$STAGE_NAME/install-bibites-multiverse.sh" \
    | grep -q '^-rwx' || die "the complete Linux archive lost the executable bit on the installer"
  unzip -Z "$DIST/$LINUX_COMPLETE_ZIP_NAME" "$STAGE_NAME/game/The Bibites.x86_64" \
    | grep -q '^-rwx' || die "the complete Linux archive lost the executable bit on the game"
fi

for archive in "${ARCHIVE_NAMES[@]}"; do
  [ "$(unzip -Z1 "$DIST/$archive" "$STAGE_NAME/LICENSE")" = "$STAGE_NAME/LICENSE" ] \
    || die "$archive does not contain the Apache license"
  [ "$(unzip -Z1 "$DIST/$archive" "$STAGE_NAME/THIRD_PARTY_NOTICES.md")" = "$STAGE_NAME/THIRD_PARTY_NOTICES.md" ] \
    || die "$archive does not contain the third-party notices"
  [ "$(unzip -Z1 "$DIST/$archive" "$STAGE_NAME/public-map.json")" = "$STAGE_NAME/public-map.json" ] \
    || die "$archive does not contain the public join configuration"
  [ "$(unzip -p "$DIST/$archive" "$STAGE_NAME/public-map.json" | sha256sum | cut -d' ' -f1)" = \
    "$(sha "$RELDIR/kit/public-map.json")" ] \
    || die "$archive contains a changed public join configuration"
done
note "every archive contains LICENSE, THIRD_PARTY_NOTICES.md, and the public join configuration"

# The launcher is a Windows payload in this release, so it is checked by name
# against the Windows archives rather than inside the loop above. The complete
# stage inherits it through `cp -a "$addon/." "$complete/"` — and THAT COPY IS
# EXACTLY WHAT THIS CHECKS, because the setup built from it hard-codes the
# shortcut target, so a complete stage without the launcher would ship shortcuts
# pointing at a file that does not exist and nothing else would notice.
[ "$(unzip -Z1 "$DIST/$ZIP_NAME" "$STAGE_NAME/$LAUNCHER_NAME")" = "$STAGE_NAME/$LAUNCHER_NAME" ] \
  || die "the Windows add-on archive does not contain the launcher"
note "the Windows add-on archive contains $LAUNCHER_NAME"
if [ -n "$WINDOWS_GAME_PAYLOAD" ]; then
  [ "$(unzip -Z1 "$DIST/$COMPLETE_ZIP_NAME" "$STAGE_NAME/$LAUNCHER_NAME")" = "$STAGE_NAME/$LAUNCHER_NAME" ] \
    || die "the Windows complete archive does not contain the launcher, so the setup's shortcuts would point at nothing"
  [ -f "$COMPLETE_STAGE/$LAUNCHER_NAME" ] \
    || die "the NSIS payload directory does not contain the launcher"
  note "the Windows complete archive and the setup payload contain $LAUNCHER_NAME"
fi

note "$DIST/$ZIP_NAME  ($ZIP_SIZE)"
note "sha256 $ZIP_SHA"
note "$DIST/$LINUX_ZIP_NAME  ($LINUX_ZIP_SIZE)"
note "sha256 $LINUX_ZIP_SHA"
if [ -n "$WINDOWS_GAME_PAYLOAD" ]; then
  note "$DIST/$COMPLETE_ZIP_NAME  ($COMPLETE_ZIP_SIZE)"
  note "sha256 $COMPLETE_ZIP_SHA"
  note "$DIST/$WINDOWS_SETUP_NAME  ($WINDOWS_SETUP_SIZE)"
  note "sha256 $WINDOWS_SETUP_SHA"
fi
if [ -n "$LINUX_GAME_PAYLOAD" ]; then
  note "$DIST/$LINUX_COMPLETE_ZIP_NAME  ($LINUX_COMPLETE_ZIP_SIZE)"
  note "sha256 $LINUX_COMPLETE_ZIP_SHA"
fi

# ------------------------------------------------------------------ the page

step "the release page text"
COMMIT="$(git -C "$REPO" rev-parse --short HEAD)"
if [ -n "$(git -C "$REPO" status --porcelain)" ]; then COMMIT="$COMMIT (working tree not clean)"; fi
BUILT_UTC="$(date -u +'%Y-%m-%dT%H:%M:%SZ')"
ORIGIN="$(git -C "$REPO" config --get remote.origin.url || true)"
REPO_SLUG="$(printf '%s' "$ORIGIN" | sed -E 's#\.git$##; s#^.*[:/]([^/]+/[^/]+)$#\1#')"
[ -n "$REPO_SLUG" ] || REPO_SLUG="OWNER/REPO"

inner_table() { # $1 a staging directory, $2 the file to write
  {
    printf '| File in the archive | SHA-256 |\n|---|---|\n'
    while read -r h f; do printf '| `%s` | `%s` |\n' "$f" "$h"; done < "$1/MANIFEST.sha256"
  } > "$2"
}
INNER_TABLE="$BUILD/inner.md"
LINUX_INNER_TABLE="$BUILD/inner-linux.md"
inner_table "$STAGE"       "$INNER_TABLE"
inner_table "$LINUX_STAGE" "$LINUX_INNER_TABLE"
COMPLETE_ROWS="$BUILD/complete-rows.md"
: > "$COMPLETE_ROWS"
if [ -n "$WINDOWS_GAME_PAYLOAD" ]; then
  printf '| **Windows setup (recommended)** | [`%s`](https://github.com/%s/releases/download/%s/%s) | `%s` |\n' \
    "$WINDOWS_SETUP_NAME" "$REPO_SLUG" "$TAG" "$WINDOWS_SETUP_NAME" "$WINDOWS_SETUP_SHA" >> "$COMPLETE_ROWS"
  printf '| Windows complete ZIP (advanced) | [`%s`](https://github.com/%s/releases/download/%s/%s) | `%s` |\n' \
    "$COMPLETE_ZIP_NAME" "$REPO_SLUG" "$TAG" "$COMPLETE_ZIP_NAME" "$COMPLETE_ZIP_SHA" >> "$COMPLETE_ROWS"
fi
if [ -n "$LINUX_GAME_PAYLOAD" ]; then
  printf '| Linux complete | [`%s`](https://github.com/%s/releases/download/%s/%s) | `%s` |\n' \
    "$LINUX_COMPLETE_ZIP_NAME" "$REPO_SLUG" "$TAG" "$LINUX_COMPLETE_ZIP_NAME" "$LINUX_COMPLETE_ZIP_SHA" >> "$COMPLETE_ROWS"
fi

# One sentence about the stable names, and only for the files that exist in this
# build. It carries no digest, so the workflow's release-page audit — which
# re-hashes every download row that has one — does not read it as a row.
STABLE_NOTE="$BUILD/stable-note.md"
: > "$STABLE_NOTE"
if [ "${#STABLE_NAMES[@]}" -gt 0 ]; then
  {
    printf 'The same packages are also published under names that carry no version. These\n'
    printf 'addresses always give the newest release:\n\n'
    for name in "${STABLE_NAMES[@]}"; do
      printf -- '- [`%s`](https://github.com/%s/releases/latest/download/%s)\n' \
        "$name" "$REPO_SLUG" "$name"
    done
    printf '\n`SHA256SUMS` lists both the versioned and the stable name of each file, so the\n'
    printf 'checksum check works for whichever name you downloaded.\n\n'
  } > "$STABLE_NOTE"
fi

if [ -n "$WINDOWS_GAME_PAYLOAD" ] || [ -n "$LINUX_GAME_PAYLOAD" ]; then
  EDITION_NOTE='The Windows setup and complete ZIP use their included game by default. The Windows GUI can instead select an existing game. The Linux complete package uses its included game. Add-on packages use an existing game.'
else
  EDITION_NOTE='This release contains add-on packages only. The installer finds your existing game automatically. There is no edition choice during installation.'
fi

PAGE="$DIST/RELEASE-PAGE.md"
python3 - "$RELDIR/RELEASE-PAGE.md" "$PAGE" "$INNER_TABLE" "$LINUX_INNER_TABLE" "$COMPLETE_ROWS" "$STABLE_NOTE" <<PY
import sys
src, dst, inner, linux_inner, complete_rows, stable_note = sys.argv[1:]
text = open(src, encoding='utf-8').read()
fields = {
    '@@RELEASE@@':    '$RELEASE',
    '@@TAG@@':        '$TAG',
    '@@REPO@@':       '$REPO_SLUG',
    '@@ZIP_NAME@@':   '$ZIP_NAME',
    '@@ZIP_SHA256@@': '$ZIP_SHA',
    '@@ZIP_SIZE@@':   '$ZIP_SIZE',
    '@@LINUX_ZIP_NAME@@':   '$LINUX_ZIP_NAME',
    '@@LINUX_ZIP_SHA256@@': '$LINUX_ZIP_SHA',
    '@@LINUX_ZIP_SIZE@@':   '$LINUX_ZIP_SIZE',
    '@@BUILT_UTC@@':  '$BUILT_UTC',
    '@@COMMIT@@':     '$COMMIT',
    '@@EDITION_NOTE@@': '$EDITION_NOTE',
    '@@INNER_TABLE@@': open(inner, encoding='utf-8').read().rstrip('\n'),
    '@@LINUX_INNER_TABLE@@': open(linux_inner, encoding='utf-8').read().rstrip('\n'),
    # Rows already end in a newline. Empty means the SHA256SUMS row follows the
    # two add-on rows directly, without a blank line that would end the table.
    '@@COMPLETE_DOWNLOAD_ROWS@@': open(complete_rows, encoding='utf-8').read(),
    # Already ends in a blank line when it is there at all. Empty leaves the
    # download table followed directly by the next heading.
    '@@STABLE_NAME_NOTE@@': open(stable_note, encoding='utf-8').read(),
}
for k, v in fields.items():
    text = text.replace(k, v)
open(dst, 'w', encoding='utf-8').write(text)
PY
note "$PAGE"

LEFT="$(grep -o '@@[A-Z0-9:_]*@@' "$PAGE" | sort -u || true)"
if [ -n "$LEFT" ]; then
  printf '\n=== unresolved release-page template fields\n'
  printf '%s\n' "$LEFT" | while read -r m; do
    printf '    %s\n' "$m"
    grep -n -- "$m" "$PAGE" | head -3 | sed 's/^/        /'
  done
  die "the generated release page still contains template fields"
fi

printf '\n=== ready, and NOTHING IS PUBLISHED\n\n'
printf '  archives  %s/%s\n' "$DIST" "$ZIP_NAME"
printf '            %s/%s\n' "$DIST" "$LINUX_ZIP_NAME"
[ -z "$WINDOWS_GAME_PAYLOAD" ] || printf '            %s/%s\n' "$DIST" "$COMPLETE_ZIP_NAME"
[ -z "$WINDOWS_GAME_PAYLOAD" ] || printf '            %s/%s\n' "$DIST" "$WINDOWS_SETUP_NAME"
[ -z "$LINUX_GAME_PAYLOAD" ] || printf '            %s/%s\n' "$DIST" "$LINUX_COMPLETE_ZIP_NAME"
for name in ${STABLE_NAMES[@]+"${STABLE_NAMES[@]}"}; do
  printf '  stable    %s/%s  (the homepage links this name)\n' "$DIST" "$name"
done
printf '  checksum  %s/SHA256SUMS  (covers every artifact above)\n' "$DIST"
printf '  page      %s\n\n' "$PAGE"
printf 'Publishing is four hand steps, in release/README.md. Nothing above touched a\n'
printf 'network except the two cached BepInEx downloads, and nothing above tagged\n'
printf 'anything.\n'
