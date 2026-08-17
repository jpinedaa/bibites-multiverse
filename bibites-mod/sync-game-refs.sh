#!/usr/bin/env bash
# Detect a game update and re-sync the toolchain inputs: the reference DLLs in
# libs/ and the decompiled source in decompiled/BibitesAssembly.
# Usage: sync-game-refs.sh [--force]
set -euo pipefail

MOD_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(cd "$MOD_DIR/.." && pwd)"

# shellcheck source=lib/dotnet-env.sh
. "$MOD_DIR/lib/dotnet-env.sh"
dotnet_env || exit 1

GAME="/mnt/c/Program Files (x86)/Steam/steamapps/common/The Bibites"
MANAGED="$GAME/The Bibites_Data/Managed"
CORE="$GAME/BepInEx/core"
MANIFEST="/mnt/c/Program Files (x86)/Steam/steamapps/appmanifest_2736860.acf"
LIBS="$MOD_DIR/libs"
STAMP="$LIBS/.sync-stamp"
DECOMPILED="$REPO/decompiled/BibitesAssembly"

# The reference set. Everything but 0Harmony/BepInEx comes from the game's
# Managed folder; those two come from the BepInEx install in the game folder.
MANAGED_DLLS=(
  BibitesAssembly.dll
  Newtonsoft.Json.dll
  Newtonsoft.Json.UnityConverters.dll
  ShapesRuntime.dll
  UnityEngine.dll
  UnityEngine.CoreModule.dll
  UnityEngine.InputLegacyModule.dll
  UnityEngine.JSONSerializeModule.dll
  UnityEngine.Physics2DModule.dll
  UnityEngine.PhysicsModule.dll
  UnityEngine.UI.dll
)
CORE_DLLS=(
  0Harmony.dll
  BepInEx.dll
)

FORCE=0
case "${1:-}" in
  --force) FORCE=1 ;;
  "")      ;;
  *)       echo "usage: sync-game-refs.sh [--force]" >&2; exit 1 ;;
esac

if [ ! -f "$MANAGED/BibitesAssembly.dll" ]; then
  echo "game assembly not found: $MANAGED/BibitesAssembly.dll" >&2
  exit 1
fi

hash_of() { sha256sum <"$1" | cut -d' ' -f1; }

read_buildid() {
  [ -f "$MANIFEST" ] || return 0
  sed -n 's/^[[:space:]]*"buildid"[[:space:]]*"\([0-9]*\)".*/\1/p' "$MANIFEST" | head -n1
}

NEW_BUILDID="$(read_buildid)"
[ -n "$NEW_BUILDID" ] || NEW_BUILDID="unknown"

OLD_BUILDID=""
if [ -f "$STAMP" ]; then
  OLD_BUILDID="$(sed -n 's/^buildid=//p' "$STAMP" | head -n1)"
fi
[ -n "$OLD_BUILDID" ] || OLD_BUILDID="unknown"

GAME_HASH="$(hash_of "$MANAGED/BibitesAssembly.dll")"
LIB_HASH="none"
if [ -f "$LIBS/BibitesAssembly.dll" ]; then
  LIB_HASH="$(hash_of "$LIBS/BibitesAssembly.dll")"
fi

if [ "$GAME_HASH" = "$LIB_HASH" ] && [ "$FORCE" -eq 0 ]; then
  echo "up to date (buildid $NEW_BUILDID)"
  exit 0
fi

if [ "$FORCE" -eq 1 ]; then
  echo "forced re-sync (buildid $NEW_BUILDID)"
else
  echo "game update detected"
fi
echo "  buildid:   $OLD_BUILDID -> $NEW_BUILDID"
echo "  assembly:  ${LIB_HASH:0:12} -> ${GAME_HASH:0:12}"

echo "Copying reference DLLs -> $LIBS"
mkdir -p "$LIBS"
for name in "${MANAGED_DLLS[@]}"; do
  cp "$MANAGED/$name" "$LIBS/$name"
done
for name in "${CORE_DLLS[@]}"; do
  cp "$CORE/$name" "$LIBS/$name"
done

echo "Regenerating $DECOMPILED"
rm -rf "$DECOMPILED"
mkdir -p "$DECOMPILED"
ilspycmd -p -o "$DECOMPILED" --referencepath "$LIBS" "$LIBS/BibitesAssembly.dll"

printf 'buildid=%s\nsha256=%s\n' "$NEW_BUILDID" "$GAME_HASH" >"$STAMP"

echo
echo "Re-synced to buildid $NEW_BUILDID (was $OLD_BUILDID)."
echo "The game APIs may have changed. Re-verify the mod against the new source:"
echo "  - re-check the hooked signatures and field paths in $DECOMPILED"
echo "  - rebuild and redeploy: $MOD_DIR/deploy.sh"
echo "  - re-run the M1 exit test before you trust any earlier finding"
