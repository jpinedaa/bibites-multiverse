#!/usr/bin/env bash
# Build the GitHub release's artifacts into release/dist/, from this checkout.
#
#   release/dist/bibites-multiverse-<release>-windows-x64.zip   the Windows download
#   release/dist/bibites-multiverse-<release>-linux-x64.zip     the Linux download
#   release/dist/*-complete.zip                                 optional authorized
#                                                               game-bundled editions
#   release/dist/SHA256SUMS                                     all checksums
#   release/dist/RELEASE-PAGE.md                                the page text,
#                                                               with the checksums in it
#
# TWO PLATFORMS, OPTIONAL COMPLETE EDITIONS, ONE MOD. The plugin is the SAME FILE, byte for byte:
# it is platform-independent IL, a Harmony patch against managed types that the
# game's Mono build carries identically on both platforms. What differs between
# the archives is the sidecar (a native binary, cross-compiled twice), the mod
# framework (BepInEx win_x64 against linux_x64), and the kit — PowerShell against
# bash. This script builds one plugin and copies it into every staged edition,
# rather than building it twice and hoping.
#
# IT PUBLISHES NOTHING. There is no gh, no git tag and no network call except the
# two cached BepInEx downloads. Publishing is a separate, deliberate act — see
# release/README.md for the four steps the owner performs by hand.
#
# WHAT IT PROVES BEFORE IT PACKAGES ANYTHING. A release that ships a mod or a
# sidecar the project's own deployment does not run is a release nobody has
# tested. So this script builds both and then requires them to be BYTE-IDENTICAL
# to the copies in farend/dist/farend-bundle.zip — the tracked artifact set the
# living fleet runs, present in any clean clone. A mismatch stops the build and
# says which side moved.
#
# It also requires the game build to be the one docs/support-matrix.md names.
# The matrix is the single source of that pin: the JSON block inside that
# document is copied into the archive unchanged, so the installer's refusal and
# the published table cannot drift apart.
#
# PREREQUISITES, on a machine that has the game:
#   * bibites-mod/libs/, from bibites-mod/sync-game-refs.sh (the reference
#     assemblies are the game's own and are never committed)
#   * the .NET SDK in ~/.dotnet and Go in $GOROOT — the same two the rest of this
#     repository uses. A PLAYER needs neither; this is the build side
#   * zip, unzip, curl
#
# Everything heavy runs under nice -n 19: this host runs a live six-world
# deployment and unniced load on it has twice reproduced a sidecar session storm.
set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RELDIR="$REPO/release"
DIST="$RELDIR/dist"
BUILD="$DIST/build"
MATRIX_DOC="$REPO/docs/support-matrix.md"
BUNDLE="$REPO/farend/dist/farend-bundle.zip"
CACHE="$REPO/farend/dist/cache"
PROJECT_LICENSE="$REPO/LICENSE"
THIRD_PARTY_NOTICES="$REPO/THIRD_PARTY_NOTICES.md"

RELEASE=m5.0
TAG="$RELEASE"
ZIP_NAME="bibites-multiverse-${RELEASE}-windows-x64.zip"
LINUX_ZIP_NAME="bibites-multiverse-${RELEASE}-linux-x64.zip"
COMPLETE_ZIP_NAME="bibites-multiverse-${RELEASE}-windows-x64-complete.zip"
LINUX_COMPLETE_ZIP_NAME="bibites-multiverse-${RELEASE}-linux-x64-complete.zip"
STAGE_NAME="bibites-multiverse-${RELEASE}"

# Optional, and only ever a check: an unpacked copy of the LINUX game, so the
# Linux matrix row's hash is verified against a real file at build time rather
# than trusted. Absent, the build says so and goes on - a machine that builds the
# release is not required to hold two copies of the game.
LINUX_GAME_DIR="${LINUX_GAME_DIR:-/mnt/wsl/data/scratch/m5-linux-rehearsal/game}"

export GOROOT="${GOROOT:-$HOME/go}"
export PATH="$GOROOT/bin:$PATH"
export DOTNET_ROOT="$HOME/.dotnet"
export PATH="$HOME/.dotnet:$HOME/.dotnet/tools:$PATH"
export TZ=UTC

ALLOW_DIRTY=0
WINDOWS_GAME_PAYLOAD=''
LINUX_GAME_PAYLOAD=''
GAME_LICENSE=''
while [ $# -gt 0 ]; do
  case "$1" in
    --allow-dirty) ALLOW_DIRTY=1; shift ;;
    --windows-game-dir) [ $# -gt 1 ] || { printf '%s needs a value\n' "$1" >&2; exit 2; }; WINDOWS_GAME_PAYLOAD="$2"; shift 2 ;;
    --linux-game-dir)   [ $# -gt 1 ] || { printf '%s needs a value\n' "$1" >&2; exit 2; }; LINUX_GAME_PAYLOAD="$2"; shift 2 ;;
    --game-license)     [ $# -gt 1 ] || { printf '%s needs a value\n' "$1" >&2; exit 2; }; GAME_LICENSE="$2"; shift 2 ;;
    *) printf 'usage: %s [--allow-dirty] [--windows-game-dir <dir>] [--linux-game-dir <dir>] [--game-license <file>]\n' "$0" >&2; exit 2 ;;
  esac
done

if { [ -n "$WINDOWS_GAME_PAYLOAD" ] || [ -n "$LINUX_GAME_PAYLOAD" ]; } && [ -z "$GAME_LICENSE" ]; then
  printf 'complete packages require --game-license <publisher-provided license file>\n' >&2
  exit 2
fi
if [ -n "$GAME_LICENSE" ] && [ -z "$WINDOWS_GAME_PAYLOAD" ] && [ -z "$LINUX_GAME_PAYLOAD" ]; then
  printf '%s has no effect without --windows-game-dir or --linux-game-dir\n' '--game-license' >&2
  exit 2
fi

note() { printf '    %s\n' "$*"; }
step() { printf '\n=== %s\n' "$*"; }
die()  { printf '\n!! %s\n' "$*" >&2; exit 1; }

sha() { sha256sum "$1" | cut -d' ' -f1; }
SHA_UPPER() { sha "$1" | tr 'a-f' 'A-F'; }

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
  note "--allow-dirty: this is a rehearsal. The sidecar will be stamped vcs.modified=true"
  note "and MUST NOT be published."
else
  note "clean at $(git -C "$REPO" rev-parse --short HEAD)"
fi

# ------------------------------------------------------------------ the matrix

step "the support matrix"
[ -f "$MATRIX_DOC" ] || die "missing $MATRIX_DOC"
mkdir -p "$BUILD"
MATRIX_JSON="$BUILD/support-matrix.json"
awk '/SUPPORT-MATRIX-JSON-BEGIN/{f=1;next} /SUPPORT-MATRIX-JSON-END/{f=0} f' "$MATRIX_DOC" \
  | sed '/^```/d' > "$MATRIX_JSON"
[ -s "$MATRIX_JSON" ] || die "no JSON block between the markers in $MATRIX_DOC"
python3 -c "import json,sys; json.load(open(sys.argv[1]))" "$MATRIX_JSON" \
  || die "the matrix block in $MATRIX_DOC is not valid JSON"

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
LINUX_ASSEMBLY="$LINUX_GAME_DIR/The Bibites_Data/Managed/BibitesAssembly.dll"
if [ -f "$LINUX_ASSEMBLY" ]; then
  GOT_LINUX_SHA="$(SHA_UPPER "$LINUX_ASSEMBLY")"
  [ "$GOT_LINUX_SHA" = "$LINUX_GAME_SHA" ] \
    || die "the Linux row pins $LINUX_GAME_SHA but $LINUX_ASSEMBLY is $GOT_LINUX_SHA"
  note "the Linux game at $LINUX_GAME_DIR matches the matrix's Linux row"
else
  note "no unpacked Linux game at $LINUX_GAME_DIR; the Linux row's hash stands on its record"
fi

# ------------------------------------------------------------------ the fleet's own artifacts

step "what the living deployment runs (the byte-identity reference)"
[ -f "$BUNDLE" ] || die "missing $BUNDLE — it is tracked; this is not a clean clone"
REF="$BUILD/ref"
rm -rf "$REF"; mkdir -p "$REF"
unzip -q -o "$BUNDLE" -d "$REF"
REF_PLUGIN="$REF/farend-bundle/BibitesMultiverse.dll"
REF_SIDECAR="$REF/farend-bundle/multiverse-sidecar.exe"
[ -f "$REF_PLUGIN" ] && [ -f "$REF_SIDECAR" ] || die "$BUNDLE does not hold both artifacts"
note "plugin  $(sha "$REF_PLUGIN")"
note "sidecar $(sha "$REF_SIDECAR")"

# ------------------------------------------------------------------ the sidecar

step "the Windows sidecar (cross-compiled)"
# A Go binary carries a VCS stamp — the commit, its time, and whether the tree
# was dirty — so two builds of IDENTICAL SOURCE at two commits are different
# files. Byte-comparing this one against the fleet's copy would therefore fail
# for a documentation commit, which is not the thing worth failing on. What is
# worth failing on is a SOURCE difference, so that is what is compared: the
# revision the fleet's binary records against this tree, over go/.
REF_REV="$(go version -m "$REF_SIDECAR" | sed -n 's/^[[:space:]]*build[[:space:]]*vcs\.revision=//p')"
[ -n "$REF_REV" ] || die "the fleet's sidecar carries no VCS stamp; this check needs one"
note "the fleet's sidecar was built from $REF_REV"
git -C "$REPO" cat-file -e "$REF_REV^{commit}" 2>/dev/null \
  || die "commit $REF_REV is not in this clone, so the fleet's build cannot be compared to it"
if ! git -C "$REPO" diff --quiet "$REF_REV" HEAD -- go/; then
  git -C "$REPO" diff --stat "$REF_REV" HEAD -- go/ >&2
  die "go/ has moved since the sidecar the deployment runs was built. Roll the change onto
   the deployment and rebuild farend/dist/farend-bundle.zip, or release from the commit the
   fleet is on. A release nobody has run is not a release."
fi
if [ -n "$(git -C "$REPO" status --porcelain -- go/)" ]; then
  die "go/ has uncommitted changes, so the binary this would ship is not any published commit"
fi
note "go/ is identical to the tree the fleet's sidecar was built from"

( cd "$REPO/go" && nice -n 19 env GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
    go build -o "$BUILD/multiverse-sidecar.exe" ./cmd/sidecar )
BUILT_SIDECAR_SHA="$(sha "$BUILD/multiverse-sidecar.exe")"
if [ "$BUILT_SIDECAR_SHA" = "$(sha "$REF_SIDECAR")" ]; then
  note "byte-identical to the fleet's sidecar"
else
  note "same source, different VCS stamp:"
  note "  the fleet's copy records $REF_REV"
  note "  this build records        $(git -C "$REPO" rev-parse HEAD)"
  note "  $BUILT_SIDECAR_SHA"
fi

# ------------------------------------------------------------------ the Linux sidecar

step "the Linux sidecar (cross-compiled)"
# THERE IS NO BYTE-IDENTITY REFERENCE FOR THIS ONE, and pretending otherwise
# would be the dishonest move. The fleet is Windows: farend/dist/farend-bundle.zip
# holds no linux binary to compare against. What the check above already
# established is the thing that matters and it covers this build too — go/ is
# identical, commit for commit, to the tree the deployment's sidecar was built
# from. Same source, second target.
( cd "$REPO/go" && nice -n 19 env GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
    go build -o "$BUILD/multiverse-sidecar" ./cmd/sidecar )
note "$(sha "$BUILD/multiverse-sidecar")"
note "same source as the fleet's sidecar ($REF_REV), built for linux/amd64"
note "static: CGO is off, so it needs no libc of a particular vintage"

# ------------------------------------------------------------------ the plugin

step "the plugin (fresh Release build)"
nice -n 19 dotnet build "$REPO/bibites-mod/BibitesMultiverse.csproj" -c Release \
  --nologo -v quiet -clp:ErrorsOnly
PLUGIN="$REPO/bibites-mod/bin/Release/BibitesMultiverse.dll"
[ -f "$PLUGIN" ] || die "the build produced no $PLUGIN"
if [ "$(sha "$PLUGIN")" != "$(sha "$REF_PLUGIN")" ]; then
  echo "!! this tree builds  $(sha "$PLUGIN")" >&2
  echo "!! the fleet runs    $(sha "$REF_PLUGIN")" >&2
  die "the mod in this tree is not the mod the deployment runs. Deploy it, rebuild
   farend/dist/farend-bundle.zip, and release from there."
fi
note "byte-identical to the fleet's plugin"

DEPLOYED="/mnt/c/Program Files (x86)/Steam/steamapps/common/The Bibites/BepInEx/plugins/BibitesMultiverse.dll"
if [ -f "$DEPLOYED" ]; then
  if [ "$(sha256sum <"$DEPLOYED" | cut -d' ' -f1)" != "$(sha "$PLUGIN")" ]; then
    die "the plugin deployed into this machine's own game is a different build again.
   Three copies must agree before a release: this tree, the far-end bundle, and the games
   the deployment is running."
  fi
  note "byte-identical to the plugin deployed in this machine's game"
else
  note "this machine's game directory is not readable from here; the bundle check stands alone"
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
       "$DIST/$COMPLETE_ZIP_NAME" "$DIST/$LINUX_COMPLETE_ZIP_NAME"
mkdir -p "$STAGE" "$LINUX_STAGE"

cp "$RELDIR/kit/Install-BibitesMultiverse.ps1"   "$STAGE/"
cp "$RELDIR/kit/Uninstall-BibitesMultiverse.ps1" "$STAGE/"
cp "$RELDIR/kit/README.md"                       "$STAGE/"
cp "$MATRIX_JSON"                                "$STAGE/support-matrix.json"
cp "$PLUGIN"                                     "$STAGE/BibitesMultiverse.dll"
cp "$BUILD/multiverse-sidecar.exe"               "$STAGE/"
cp "$CACHE/$BEPINEX_ZIP"                         "$STAGE/"
cp "$PROJECT_LICENSE"                            "$STAGE/LICENSE"
cp "$THIRD_PARTY_NOTICES"                        "$STAGE/THIRD_PARTY_NOTICES.md"

cp "$RELDIR/kit/install-bibites-multiverse.sh"   "$LINUX_STAGE/"
cp "$RELDIR/kit/uninstall-bibites-multiverse.sh" "$LINUX_STAGE/"
cp "$RELDIR/kit/README-linux.md"                 "$LINUX_STAGE/README.md"
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
for f in Install-BibitesMultiverse.ps1 Uninstall-BibitesMultiverse.ps1 README.md LICENSE THIRD_PARTY_NOTICES.md; do
  python3 -c "
import sys
p = sys.argv[1]
data = open(p, 'rb').read().replace(b'\r\n', b'\n').replace(b'\n', b'\r\n')
open(p, 'wb').write(data)
" "$STAGE/$f"
done

# A complete archive is the add-on archive plus an authorized, unmodified game
# payload. Proprietary game bytes are never read from the repository and never
# committed: the publisher-provided directory is an explicit build input. The
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
  for bad in BepInEx winhttp.dll doorstop_config.ini run_bepinex.sh libdoorstop.so .doorstop_version; do
    [ ! -e "$source/$bad" ] || die "$platform game payload is already modded ($bad exists). Use a clean publisher-provided game directory."
  done
  if [ "$platform" = Linux ]; then
    [ -x "$source/$executable" ] || die "$source/$executable is not executable"
  fi
  note "$platform complete payload: game $GAME_VERSION, assembly $got"
}

stage_complete() { # $1 add-on stage, $2 complete stage, $3 game source, $4 platform, $5 executable, $6 assembly sha
  local addon="$1" complete="$2" source="$3" platform="$4" executable="$5" expected_sha="$6"
  validate_game_payload "$source" "$platform" "$executable" "$expected_sha"
  [ -f "$GAME_LICENSE" ] && [ -s "$GAME_LICENSE" ] && [ ! -L "$GAME_LICENSE" ] \
    || die "game license must be a non-empty regular file, not a symbolic link: $GAME_LICENSE"
  mkdir -p "$complete"
  cp -a "$addon/." "$complete/"
  mkdir -p "$complete/game"
  cp -a "$source/." "$complete/game/"
  cp "$GAME_LICENSE" "$complete/GAME-LICENSE.txt"
  python3 - "$complete/game-payload.json" "$platform" "$GAME_VERSION" "$expected_sha" <<'PY'
import json, sys
path, platform, version, assembly_sha = sys.argv[1:]
doc = {
    "format": "bibites-multiverse/game-payload/1",
    "platform": platform,
    "gameVersion": version,
    "assemblySha256": assembly_sha,
    "licenseFile": "GAME-LICENSE.txt",
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
( cd "$DIST" && sha256sum "${ARCHIVE_NAMES[@]}" > SHA256SUMS )

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
  unzip -Z1 "$DIST/$archive" | grep -qx "$STAGE_NAME/LICENSE" \
    || die "$archive does not contain the Apache license"
  unzip -Z1 "$DIST/$archive" | grep -qx "$STAGE_NAME/THIRD_PARTY_NOTICES.md" \
    || die "$archive does not contain the third-party notices"
done
note "every archive contains LICENSE and THIRD_PARTY_NOTICES.md"

note "$DIST/$ZIP_NAME  ($ZIP_SIZE)"
note "sha256 $ZIP_SHA"
note "$DIST/$LINUX_ZIP_NAME  ($LINUX_ZIP_SIZE)"
note "sha256 $LINUX_ZIP_SHA"
if [ -n "$WINDOWS_GAME_PAYLOAD" ]; then
  note "$DIST/$COMPLETE_ZIP_NAME  ($COMPLETE_ZIP_SIZE)"
  note "sha256 $COMPLETE_ZIP_SHA"
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
  printf '| [`%s`](https://github.com/%s/releases/download/%s/%s) | Windows complete: authorized game + Multiverse | %s | `%s` |\n' \
    "$COMPLETE_ZIP_NAME" "$REPO_SLUG" "$TAG" "$COMPLETE_ZIP_NAME" "$COMPLETE_ZIP_SIZE" "$COMPLETE_ZIP_SHA" >> "$COMPLETE_ROWS"
fi
if [ -n "$LINUX_GAME_PAYLOAD" ]; then
  printf '| [`%s`](https://github.com/%s/releases/download/%s/%s) | Linux complete: authorized game + Multiverse | %s | `%s` |\n' \
    "$LINUX_COMPLETE_ZIP_NAME" "$REPO_SLUG" "$TAG" "$LINUX_COMPLETE_ZIP_NAME" "$LINUX_COMPLETE_ZIP_SIZE" "$LINUX_COMPLETE_ZIP_SHA" >> "$COMPLETE_ROWS"
fi

PAGE="$DIST/RELEASE-PAGE.md"
python3 - "$RELDIR/RELEASE-PAGE.md" "$PAGE" "$INNER_TABLE" "$LINUX_INNER_TABLE" "$COMPLETE_ROWS" <<PY
import sys
src, dst, inner, linux_inner, complete_rows = sys.argv[1:]
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
    '@@INNER_TABLE@@': open(inner, encoding='utf-8').read().rstrip('\n'),
    '@@LINUX_INNER_TABLE@@': open(linux_inner, encoding='utf-8').read().rstrip('\n'),
    # Rows already end in a newline. Empty means the SHA256SUMS row follows the
    # two add-on rows directly, without a blank line that would end the table.
    '@@COMPLETE_DOWNLOAD_ROWS@@': open(complete_rows, encoding='utf-8').read(),
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
[ -z "$LINUX_GAME_PAYLOAD" ] || printf '            %s/%s\n' "$DIST" "$LINUX_COMPLETE_ZIP_NAME"
printf '  checksum  %s/SHA256SUMS  (covers every archive above)\n' "$DIST"
printf '  page      %s\n\n' "$PAGE"
printf 'Publishing is four hand steps, in release/README.md. Nothing above touched a\n'
printf 'network except the two cached BepInEx downloads, and nothing above tagged\n'
printf 'anything.\n'
