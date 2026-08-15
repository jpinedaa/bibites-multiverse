#!/usr/bin/env bash
# Build farend/dist/farend-bundle.zip: the part of the far end's handover that is
# the same for every deployment, and nothing it cannot use.
#
# Under M4 the second computer is map SLOT 6, at position (2,1) — a real world
# with a real game, not a spare. The bundle is what makes it one, so a stale zip
# is a stale member of the map: rebuild and commit it whenever the plugin, the
# sidecar binary, setup-farend.ps1 or the $AssemblySha256 pin changes.
#
# The bundle holds five files — one install script, one document, three artifacts:
#
#   setup-farend.ps1        install: find the game, check its version, install
#                           BepInEx and the plugin, trust the relay's certificate
#                           authority, store this world's own credential, write
#                           the start and stop scripts
#   README.md               the human steps
#   multiverse-sidecar.exe  the Windows build of the sidecar
#   BibitesMultiverse.dll   a FRESH build of the plugin (dotnet build)
#   BepInEx_win_x64_*.zip   the pinned BepInEx release, downloaded and verified
#
# THE ZIP IS NOT THE WHOLE HANDOVER. At contract-b/4.0 the shared token is gone,
# so the far end needs two more files that are deliberately NOT in here: the
# relay's certificate authority (e2e/tls-m4-lan/ca.crt — per deployment, not a
# secret) and the secret half of slot 6's join string (printed once by the relay,
# and a secret). setup-farend.ps1 takes them as -CaFile and -PeerSecretFile.
# farend/README.md is the operator-facing version of the same list.
#
# THE ZIP ITSELF IS COMMITTED, and deliberately. farend/.gitignore says `dist/*`
# and then `!dist/farend-bundle.zip`, so everything under dist/ is ignored EXCEPT
# this one tracked binary: the second computer takes it out of a clone rather than
# off a USB stick. Rebuilding it is therefore a repository act — commit the new
# zip, in its own commit. The build scratch (dist/farend-bundle/) and the cached
# BepInEx download (dist/cache/) stay ignored.
#
# THIS RUNS AGAINST A LIVE DEPLOYMENT. It builds the plugin and reads the result
# out of bibites-mod/bin/Release/; it never writes into the game's plugins folder,
# so no game has to be stopped. That is why it does not call deploy.sh: deploy.sh
# is build PLUS the copy into BepInEx/plugins, and only the copy is what a running
# game can block (dev_environment.md). Deploying to THIS machine's games is a
# separate act with its own downtime — see the M4 rigs' teardown.
set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
FAREND="$REPO/farend"
DIST="$FAREND/dist"
CACHE="$DIST/cache"
STAGE="$DIST/farend-bundle"
ZIP="$DIST/farend-bundle.zip"
# Go can omit VCS metadata when a linked worktree and its Git metadata are on
# different filesystems. A release needs that provenance stamp. Set this to a
# clean checkout of the same commit when the current worktree has that shape.
SIDECAR_BUILD_REPO="${FAREND_SIDECAR_BUILD_REPO:-$REPO}"

BEPINEX_VERSION=5.4.23.3
BEPINEX_ZIP="BepInEx_win_x64_${BEPINEX_VERSION}.zip"
BEPINEX_URL="https://github.com/BepInEx/BepInEx/releases/download/v${BEPINEX_VERSION}/${BEPINEX_ZIP}"
BEPINEX_SHA=41a089e5b1b1f0713b331346baf6677b1184c69eabebf51101097954e854c749

export GOROOT="${GOROOT:-$HOME/go}"
export PATH="$GOROOT/bin:$PATH"

note() { printf '    %s\n' "$*"; }
step() { printf '\n=== %s\n' "$*"; }

step "the pinned game version"
GAME_DLL="$REPO/bibites-mod/libs/BibitesAssembly.dll"
[ -f "$GAME_DLL" ] || { echo "missing $GAME_DLL — run bibites-mod/sync-game-refs.sh" >&2; exit 1; }
DLL_SHA="$(sha256sum "$GAME_DLL" | cut -d' ' -f1 | tr 'a-f' 'A-F')"
note "libs/BibitesAssembly.dll = $DLL_SHA"

# The version gate in setup-farend.ps1 is only true while it holds THIS hash.
PINNED="$(grep -oE "AssemblySha256\s*=\s*'[0-9A-F]+'" "$FAREND/setup-farend.ps1" | grep -oE "[0-9A-F]{64}")"
if [ "$PINNED" != "$DLL_SHA" ]; then
  echo "!! setup-farend.ps1 pins $PINNED" >&2
  echo "!! but bibites-mod/libs/BibitesAssembly.dll is $DLL_SHA" >&2
  echo "!! The game moved. Update \$AssemblySha256 in setup-farend.ps1 and rebuild the mod." >&2
  exit 1
fi
note "setup-farend.ps1 pins the same hash"

step "the Windows sidecar"
SOURCE_REV="$(git -C "$REPO" rev-parse HEAD)"
[ -d "$SIDECAR_BUILD_REPO/go" ] \
  || { echo "missing $SIDECAR_BUILD_REPO/go" >&2; exit 1; }
BUILD_REV="$(git -C "$SIDECAR_BUILD_REPO" rev-parse HEAD)"
[ "$BUILD_REV" = "$SOURCE_REV" ] \
  || { echo "sidecar build checkout is $BUILD_REV, want $SOURCE_REV" >&2; exit 1; }
[ -z "$(git -C "$SIDECAR_BUILD_REPO" status --porcelain)" ] \
  || { echo "sidecar build checkout is dirty: $SIDECAR_BUILD_REPO" >&2; exit 1; }
mkdir -p "$REPO/bin"
( cd "$SIDECAR_BUILD_REPO/go" && GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
    go build -buildvcs=true -o "$REPO/bin/multiverse-sidecar.exe" ./cmd/sidecar )
STAMP_REV="$(go version -m "$REPO/bin/multiverse-sidecar.exe" \
  | sed -n 's/^[[:space:]]*build[[:space:]]*vcs\.revision=//p')"
STAMP_DIRTY="$(go version -m "$REPO/bin/multiverse-sidecar.exe" \
  | sed -n 's/^[[:space:]]*build[[:space:]]*vcs\.modified=//p')"
[ "$STAMP_REV" = "$SOURCE_REV" ] \
  || { echo "sidecar VCS stamp is '${STAMP_REV:-missing}', want $SOURCE_REV" >&2; exit 1; }
[ "$STAMP_DIRTY" = false ] \
  || { echo "sidecar VCS stamp says modified=${STAMP_DIRTY:-missing}" >&2; exit 1; }
note "VCS stamp: $STAMP_REV, modified=false"
note "$(ls -l "$REPO/bin/multiverse-sidecar.exe" | awk '{print $5" bytes  "$NF}')"

step "the plugin (fresh build)"
# Steam auto-updates stay on, so the game can move under us. deploy.sh warns about
# a stale libs/ and this needs the same warning, because the bundle no longer goes
# through it. The $AssemblySha256 check above compares libs/ to the far end's
# version gate; this compares libs/ to the game actually installed here.
GAME_MANAGED="/mnt/c/Program Files (x86)/Steam/steamapps/common/The Bibites/The Bibites_Data/Managed/BibitesAssembly.dll"
if [ -f "$GAME_MANAGED" ] \
   && [ "$(sha256sum <"$GAME_MANAGED")" != "$(sha256sum <"$GAME_DLL")" ]; then
  echo "!! WARNING: the game updated — libs/BibitesAssembly.dll is stale." >&2
  echo "!! Run $REPO/bibites-mod/sync-game-refs.sh, then build again." >&2
  echo >&2
fi
export DOTNET_ROOT="$HOME/.dotnet"
export PATH="$HOME/.dotnet:$HOME/.dotnet/tools:$PATH"
dotnet build "$REPO/bibites-mod/BibitesMultiverse.csproj" -c Release -v quiet
PLUGIN="$REPO/bibites-mod/bin/Release/BibitesMultiverse.dll"
[ -f "$PLUGIN" ] || { echo "deploy.sh produced no $PLUGIN" >&2; exit 1; }
note "$(ls -l "$PLUGIN" | awk '{print $5" bytes"}')  $(sha256sum "$PLUGIN" | cut -c1-16)"

step "BepInEx $BEPINEX_VERSION"
mkdir -p "$CACHE"
if [ ! -f "$CACHE/$BEPINEX_ZIP" ]; then
  note "downloading $BEPINEX_URL"
  curl -fsSL -o "$CACHE/$BEPINEX_ZIP.tmp" "$BEPINEX_URL"
  mv -f "$CACHE/$BEPINEX_ZIP.tmp" "$CACHE/$BEPINEX_ZIP"
fi
GOT="$(sha256sum "$CACHE/$BEPINEX_ZIP" | cut -d' ' -f1)"
[ "$GOT" = "$BEPINEX_SHA" ] || { echo "!! $BEPINEX_ZIP is $GOT, want $BEPINEX_SHA" >&2; exit 1; }
note "sha256 verified: $GOT"

step "staging"
rm -rf "$STAGE" "$ZIP"
mkdir -p "$STAGE"
cp "$FAREND/setup-farend.ps1" "$FAREND/README.md" "$STAGE/"
cp "$REPO/bin/multiverse-sidecar.exe" "$STAGE/"
cp "$PLUGIN" "$STAGE/"
cp "$CACHE/$BEPINEX_ZIP" "$STAGE/"
# PowerShell reads LF fine, but a person opening the script in Notepad should not
# see one long line.
command -v unix2dos >/dev/null 2>&1 && unix2dos -q "$STAGE/setup-farend.ps1" || true

step "zipping"
( cd "$DIST" && zip -q -r "$(basename "$ZIP")" "$(basename "$STAGE")" )
rm -rf "$STAGE"
note "$ZIP"
unzip -l "$ZIP"

cat <<EOF

The bundle is ready:

  $ZIP

It is ONE of THREE files the second computer needs. Carry all three by hand:

  1. $ZIP
  2. e2e/tls-m4-lan/ca.crt, as ca.crt — the relay's certificate authority.
     Not a secret.
  3. slot 6's join secret, as peer-secret.txt — THIS ONE IS A SECRET. The relay
     prints it once and cannot reprint it; losing it costs a slot handover.

Tell that machine's operator the relay host as well — it has to be the name the
certificate was issued for. setup-farend.ps1 takes the two extra files as
-CaFile and -PeerSecretFile; farend/README.md, inside the zip, walks the
operator through all of it.
EOF
