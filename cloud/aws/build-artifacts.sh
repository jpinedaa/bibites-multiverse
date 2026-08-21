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
export PATH="$DOTNET_ROOT:$PATH"
# An unset GOROOT tells the selected go executable to use the toolchain it was
# installed with. Only override that discovery when the operator names a root.
if [ -n "${GOROOT:-}" ]; then
  export GOROOT
  export PATH="$GOROOT/bin:$PATH"
fi

for command in file go dotnet tar gzip sha256sum unzip; do
  command -v "$command" >/dev/null || { echo "missing $command" >&2; exit 1; }
done
runtime_epoch=946684800
tar_version="$(tar --version 2>/dev/null)" || {
  echo 'GNU tar is required to build a deterministic runtime archive' >&2
  exit 1
}
case "$tar_version" in
  *'GNU tar'*) ;;
  *)
    echo 'GNU tar is required to build a deterministic runtime archive' >&2
    exit 1
    ;;
esac
tar --format=gnu --sort=name --mtime="@$runtime_epoch" \
  --owner=0 --group=0 --numeric-owner \
  --mode='a-s,a-t,u+rwX,go+rX,go-w' \
  -cf /dev/null --files-from /dev/null >/dev/null 2>&1 || {
    echo 'GNU tar lacks the options required for a deterministic runtime archive' >&2
    exit 1
  }
printf '' | gzip -n >/dev/null 2>&1 || {
  echo 'gzip with -n support is required to build a deterministic runtime archive' >&2
  exit 1
}
[ -f "$game_zip" ] || { echo "missing game archive: $game_zip" >&2; exit 1; }
[ -f "$bepinex_zip" ] || { echo "missing BepInEx archive: $bepinex_zip" >&2; exit 1; }

game_sha="$(sha256sum "$game_zip" | awk '{print $1}')"
[ "${game_sha^^}" = E15695D7944B4DED9E6D29A21518D00E9689C1A2B8CEB7288D51651ADCE57F4E ] || {
  echo "game archive hash $game_sha is outside docs/support-matrix.md" >&2; exit 1; }
bepinex_sha="$(sha256sum "$bepinex_zip" | awk '{print $1}')"

rm -rf "$dist/build" "$dist/runtime"
install -d "$dist/build" "$dist/runtime"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 nice -n 19 \
  go -C "$repo/go" build -buildvcs=false -trimpath \
    -o "$dist/build/multiverse-sidecar" ./cmd/sidecar
LC_ALL=C file -b "$dist/build/multiverse-sidecar" | grep -Eq '^ELF 64-bit.*x86-64' || {
  echo 'the sidecar build is not a linux/amd64 ELF executable' >&2
  exit 1
}
nice -n 19 dotnet build "$repo/bibites-mod/BibitesMultiverse.csproj" -c Release \
  -o "$dist/build/plugin" --nologo >/dev/null

cp "$dist/build/multiverse-sidecar" "$dist/runtime/"
cp "$dist/build/plugin/BibitesMultiverse.dll" "$dist/runtime/"
cp "$runtime_src"/* "$dist/runtime/"
chmod 0755 "$dist/runtime"/bibites-* "$dist/runtime/install-host" \
  "$dist/runtime/install-broadcast-host" "$dist/runtime/multiverse-sidecar"

# Fix every archive metadata field that can vary between equivalent builds.
# 2000-01-01T00:00:00Z is an archive-format epoch, not a source timestamp.
runtime_archive="$dist/bibites-cloud-runtime.tar.gz"
runtime_archive_tmp="$dist/.bibites-cloud-runtime.tar.gz.tmp"
rm -f "$runtime_archive_tmp"
trap 'rm -f "$runtime_archive_tmp"' EXIT
LC_ALL=C tar --format=gnu --sort=name --mtime="@$runtime_epoch" \
  --owner=0 --group=0 --numeric-owner \
  --mode='a-s,a-t,u+rwX,go+rX,go-w' \
  -C "$dist/runtime" -cf - . | gzip -n > "$runtime_archive_tmp"
mv "$runtime_archive_tmp" "$runtime_archive"
trap - EXIT
cp "$game_zip" "$dist/TheBibites-0.6.3.1-Linux.zip"
cp "$bepinex_zip" "$dist/BepInEx_linux_x64_5.4.23.3.zip"
runtime_sha="$(sha256sum "$runtime_archive" | awk '{print $1}')"

{
  printf 'GAME_FILE=%q\n' 'TheBibites-0.6.3.1-Linux.zip'
  printf 'GAME_SHA256=%q\n' "$game_sha"
  printf 'BEPINEX_FILE=%q\n' 'BepInEx_linux_x64_5.4.23.3.zip'
  printf 'BEPINEX_SHA256=%q\n' "$bepinex_sha"
  printf 'RUNTIME_FILE=%q\n' 'bibites-cloud-runtime.tar.gz'
  printf 'RUNTIME_SHA256=%q\n' "$runtime_sha"
} > "$dist/artifacts.env"
sha256sum "$dist"/*.zip "$dist"/*.tar.gz > "$dist/SHA256SUMS"
printf 'built %s\n' "$dist/bibites-cloud-runtime.tar.gz"
cat "$dist/SHA256SUMS"
