#!/usr/bin/env bash
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
dist="$repo/cloud/aws/dist"
runtime_src="$repo/cloud/aws/runtime"
: "${BIBITES_GAME_ZIP:?set BIBITES_GAME_ZIP to an authorized Linux game archive}"
: "${BIBITES_BEPINEX_ZIP:?set BIBITES_BEPINEX_ZIP to a BepInEx Linux archive}"
game_zip="$BIBITES_GAME_ZIP"
bepinex_zip="$BIBITES_BEPINEX_ZIP"

export DOTNET_ROOT="${DOTNET_ROOT:-$HOME/.dotnet}"
export GOROOT="${GOROOT:-$HOME/go}"
export PATH="$DOTNET_ROOT:$GOROOT/bin:$PATH"

for command in go dotnet tar gzip sha256sum unzip; do
  command -v "$command" >/dev/null || { echo "missing $command" >&2; exit 1; }
done
[ -f "$game_zip" ] || { echo "missing game archive: $game_zip" >&2; exit 1; }
[ -f "$bepinex_zip" ] || { echo "missing BepInEx archive: $bepinex_zip" >&2; exit 1; }

game_sha="$(sha256sum "$game_zip" | awk '{print $1}')"
[ "${game_sha^^}" = E15695D7944B4DED9E6D29A21518D00E9689C1A2B8CEB7288D51651ADCE57F4E ] || {
  echo "game archive hash $game_sha is outside docs/support-matrix.md" >&2; exit 1; }
bepinex_sha="$(sha256sum "$bepinex_zip" | awk '{print $1}')"

rm -rf "$dist/build" "$dist/runtime"
install -d "$dist/build" "$dist/runtime"
nice -n 19 go -C "$repo/go" build -trimpath -o "$dist/build/multiverse-sidecar" ./cmd/sidecar
nice -n 19 dotnet build "$repo/bibites-mod/BibitesMultiverse.csproj" -c Release \
  -o "$dist/build/plugin" --nologo >/dev/null

cp "$dist/build/multiverse-sidecar" "$dist/runtime/"
cp "$dist/build/plugin/BibitesMultiverse.dll" "$dist/runtime/"
cp "$runtime_src"/* "$dist/runtime/"
chmod 0755 "$dist/runtime"/bibites-* "$dist/runtime/install-host" \
  "$dist/runtime/install-broadcast-host" "$dist/runtime/multiverse-sidecar"

tar -C "$dist/runtime" -czf "$dist/bibites-cloud-runtime.tar.gz" .
cp "$game_zip" "$dist/TheBibites-0.6.3.1-Linux.zip"
cp "$bepinex_zip" "$dist/BepInEx_linux_x64_5.4.23.3.zip"
runtime_sha="$(sha256sum "$dist/bibites-cloud-runtime.tar.gz" | awk '{print $1}')"

cat > "$dist/artifacts.env" <<EOF
GAME_FILE='TheBibites-0.6.3.1-Linux.zip'
GAME_SHA256='$game_sha'
BEPINEX_FILE='BepInEx_linux_x64_5.4.23.3.zip'
BEPINEX_SHA256='$bepinex_sha'
RUNTIME_FILE='bibites-cloud-runtime.tar.gz'
RUNTIME_SHA256='$runtime_sha'
EOF
sha256sum "$dist"/*.zip "$dist"/*.tar.gz > "$dist/SHA256SUMS"
printf 'built %s\n' "$dist/bibites-cloud-runtime.tar.gz"
cat "$dist/SHA256SUMS"
