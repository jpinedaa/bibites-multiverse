#!/usr/bin/env bash
# Build the plugin and copy it into the game's BepInEx plugins folder.
set -euo pipefail

export DOTNET_ROOT="$HOME/.dotnet"
export PATH="$HOME/.dotnet:$HOME/.dotnet/tools:$PATH"

MOD_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GAME="/mnt/c/Program Files (x86)/Steam/steamapps/common/The Bibites"
PLUGINS="$GAME/BepInEx/plugins"
GAME_DLL="$GAME/The Bibites_Data/Managed/BibitesAssembly.dll"
LIB_DLL="$MOD_DIR/libs/BibitesAssembly.dll"

# Steam auto-updates stay on, so the game can move under us. Warn (don't fail)
# when libs/ no longer matches the installed game.
if [ -f "$GAME_DLL" ] && [ -f "$LIB_DLL" ] \
   && [ "$(sha256sum <"$GAME_DLL")" != "$(sha256sum <"$LIB_DLL")" ]; then
  echo "!! WARNING: the game updated — libs/BibitesAssembly.dll is stale." >&2
  echo "!! Run $MOD_DIR/sync-game-refs.sh, then build again." >&2
  echo >&2
fi

dotnet build "$MOD_DIR/BibitesMultiverse.csproj" -c Release -v quiet
mkdir -p "$PLUGINS"
cp "$MOD_DIR/bin/Release/BibitesMultiverse.dll" "$PLUGINS/"
echo "Deployed BibitesMultiverse.dll -> $PLUGINS"
