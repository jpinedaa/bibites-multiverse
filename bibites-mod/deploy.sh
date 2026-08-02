#!/usr/bin/env bash
# Build the plugin and copy it into the game's BepInEx plugins folder.
set -euo pipefail

export DOTNET_ROOT="$HOME/.dotnet"
export PATH="$HOME/.dotnet:$HOME/.dotnet/tools:$PATH"

MOD_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GAME="/mnt/c/Program Files (x86)/Steam/steamapps/common/The Bibites"
PLUGINS="$GAME/BepInEx/plugins"

dotnet build "$MOD_DIR/BibitesMultiverse.csproj" -c Release -v quiet
mkdir -p "$PLUGINS"
cp "$MOD_DIR/bin/Release/BibitesMultiverse.dll" "$PLUGINS/"
echo "Deployed BibitesMultiverse.dll -> $PLUGINS"
