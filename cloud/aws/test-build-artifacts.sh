#!/usr/bin/env bash
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
test_root="$(mktemp -d)"
trap 'find "$test_root" -depth -delete' EXIT
fixture="$test_root/repo"
mock_bin="$test_root/bin"
install -d "$fixture/cloud/aws/runtime" "$fixture/go" "$fixture/bibites-mod" \
  "$mock_bin" "$test_root/home"
cp "$repo/cloud/aws/build-artifacts.sh" "$fixture/cloud/aws/build-artifacts.sh"
printf '%s\n' '#!/usr/bin/env bash' 'exit 0' >"$fixture/cloud/aws/runtime/bibites-placeholder"
printf '%s\n' '#!/usr/bin/env bash' 'exit 0' >"$fixture/cloud/aws/runtime/install-host"
printf '%s\n' '#!/usr/bin/env bash' 'exit 0' >"$fixture/cloud/aws/runtime/install-broadcast-host"
printf 'game\n' >"$test_root/game.zip"
printf 'bepinex\n' >"$test_root/bepinex.zip"

printf '%s\n' \
  '#!/usr/bin/env bash' \
  'set -euo pipefail' \
  '[ -z "${GOROOT:-}" ] || { echo "build invented GOROOT=$GOROOT" >&2; exit 65; }' \
  'out=""' \
  'while [ "$#" -gt 0 ]; do' \
  '  if [ "$1" = -o ]; then shift; out="$1"; fi' \
  '  shift' \
  'done' \
  '[ -n "$out" ]' \
  'install -d "$(dirname "$out")"' \
  'printf "sidecar\n" >"$out"' >"$mock_bin/go"

printf '%s\n' \
  '#!/usr/bin/env bash' \
  'set -euo pipefail' \
  'out=""' \
  'while [ "$#" -gt 0 ]; do' \
  '  if [ "$1" = -o ]; then shift; out="$1"; fi' \
  '  shift' \
  'done' \
  '[ -n "$out" ]' \
  'install -d "$out"' \
  'printf "plugin\n" >"$out/BibitesMultiverse.dll"' >"$mock_bin/dotnet"

printf '%s\n' \
  '#!/usr/bin/env bash' \
  'printf "%s\n" "ELF 64-bit LSB executable, x86-64"' >"$mock_bin/file"

printf '%s\n' \
  '#!/usr/bin/env bash' \
  'set -euo pipefail' \
  'archive=""' \
  'while [ "$#" -gt 0 ]; do' \
  '  if [ "$1" = -czf ]; then shift; archive="$1"; fi' \
  '  shift' \
  'done' \
  '[ -n "$archive" ]' \
  'printf "archive\n" >"$archive"' >"$mock_bin/tar"

printf '%s\n' \
  '#!/usr/bin/env bash' \
  'set -euo pipefail' \
  'digest=E15695D7944B4DED9E6D29A21518D00E9689C1A2B8CEB7288D51651ADCE57F4E' \
  'for path in "$@"; do printf "%s  %s\n" "$digest" "$path"; done' >"$mock_bin/sha256sum"

printf '%s\n' '#!/usr/bin/env bash' 'exit 0' >"$mock_bin/unzip"
chmod 0755 "$mock_bin"/* "$fixture/cloud/aws/build-artifacts.sh" \
  "$fixture/cloud/aws/runtime"/*

env -u GOROOT \
  HOME="$test_root/home" \
  DOTNET_ROOT="$mock_bin" \
  PATH="$mock_bin:/usr/bin:/bin" \
  BIBITES_GAME_ZIP="$test_root/game.zip" \
  BIBITES_BEPINEX_ZIP="$test_root/bepinex.zip" \
  "$fixture/cloud/aws/build-artifacts.sh" >/dev/null

[ -f "$fixture/cloud/aws/dist/bibites-cloud-runtime.tar.gz" ] || {
  echo 'artifact build did not produce the runtime archive' >&2
  exit 1
}

printf 'artifact build environment fixture passed\n'
