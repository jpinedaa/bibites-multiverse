#!/usr/bin/env bash
# Build farend/dist/farend-bundle.zip: everything the second computer needs, and
# nothing it cannot use.
#
# The bundle holds four artifacts and two scripts:
#
#   setup-farend.ps1        install: find the game, check its version, install
#                           BepInEx and the plugin, store the token, write the
#                           start and stop scripts
#   README.md               the human steps
#   multiverse-sidecar.exe  the Windows build of the sidecar
#   BibitesMultiverse.dll   a FRESH build of the plugin (deploy.sh)
#   BepInEx_win_x64_*.zip   the pinned BepInEx release, downloaded and verified
#
# Nothing binary is committed. dist/ is gitignored, and the BepInEx download is
# cached under dist/cache/.
#
# Stop every game before running this: deploy.sh cannot overwrite a plugin DLL
# that a running game holds open (dev_environment.md).
set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
FAREND="$REPO/farend"
DIST="$FAREND/dist"
CACHE="$DIST/cache"
STAGE="$DIST/farend-bundle"
ZIP="$DIST/farend-bundle.zip"

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
mkdir -p "$REPO/bin"
( cd "$REPO/go" && GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o "$REPO/bin/multiverse-sidecar.exe" ./cmd/sidecar )
note "$(ls -l "$REPO/bin/multiverse-sidecar.exe" | awk '{print $5" bytes  "$NF}')"

step "the plugin (fresh build)"
if cd /mnt/c 2>/dev/null && /mnt/c/Windows/System32/tasklist.exe 2>/dev/null | grep -qi 'The Bibites'; then
  echo "!! a game is running; stop it first (bibites-mod/game.sh stop all)" >&2
  exit 1
fi
"$REPO/bibites-mod/deploy.sh"
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

Copy it to the second computer with the token file (~/.multiverse-token) and
tell that machine's operator the relay host. farend/README.md is inside.
EOF
