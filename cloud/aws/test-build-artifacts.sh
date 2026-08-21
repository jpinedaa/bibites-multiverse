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
  'digest=E15695D7944B4DED9E6D29A21518D00E9689C1A2B8CEB7288D51651ADCE57F4E' \
  'if [ "$#" -eq 1 ] && [ "$1" = "${TEST_GAME_ZIP:?}" ]; then' \
  '  printf "%s  %s\n" "$digest" "$1"' \
  '  exit 0' \
  'fi' \
  'exec /usr/bin/sha256sum "$@"' >"$mock_bin/sha256sum"

printf '%s\n' '#!/usr/bin/env bash' 'exit 0' >"$mock_bin/unzip"
chmod 0755 "$mock_bin"/* "$fixture/cloud/aws/build-artifacts.sh" \
  "$fixture/cloud/aws/runtime"/*

run_build() {
  env -u GOROOT \
    HOME="$test_root/home" \
    DOTNET_ROOT="$mock_bin" \
    PATH="$mock_bin:/usr/bin:/bin" \
    TEST_GAME_ZIP="$test_root/game.zip" \
    BIBITES_GAME_ZIP="$test_root/game.zip" \
    BIBITES_BEPINEX_ZIP="$test_root/bepinex.zip" \
    "$fixture/cloud/aws/build-artifacts.sh" >/dev/null
}

touch -d '2001-02-03 04:05:06 UTC' "$fixture/cloud/aws/runtime"/*
run_build
cp "$fixture/cloud/aws/dist/bibites-cloud-runtime.tar.gz" "$test_root/runtime-one.tar.gz"

# Both the inputs and the generated build outputs receive different mtimes.
sleep 1
touch -d '2031-12-13 14:15:16 UTC' "$fixture/cloud/aws/runtime"/*
run_build
cp "$fixture/cloud/aws/dist/bibites-cloud-runtime.tar.gz" "$test_root/runtime-two.tar.gz"

[ -f "$fixture/cloud/aws/dist/bibites-cloud-runtime.tar.gz" ] || {
  echo 'artifact build did not produce the runtime archive' >&2
  exit 1
}
cmp -s "$test_root/runtime-one.tar.gz" "$test_root/runtime-two.tar.gz" || {
  echo 'equivalent runtime builds did not produce byte-identical archives' >&2
  exit 1
}

cat >"$test_root/expected-members" <<'EOF'
./
./BibitesMultiverse.dll
./bibites-placeholder
./install-broadcast-host
./install-host
./multiverse-sidecar
EOF
tar -tzf "$test_root/runtime-two.tar.gz" >"$test_root/actual-members"
cmp -s "$test_root/expected-members" "$test_root/actual-members" || {
  echo 'runtime archive membership or order changed' >&2
  diff -u "$test_root/expected-members" "$test_root/actual-members" >&2 || true
  exit 1
}

TZ=UTC tar --numeric-owner --full-time -tvzf "$test_root/runtime-two.tar.gz" \
  >"$test_root/archive-metadata"
awk '
  $2 != "0/0" { exit 1 }
  $4 != "2000-01-01" || $5 != "00:00:00" { exit 1 }
' "$test_root/archive-metadata" || {
  echo 'runtime archive metadata is not normalized' >&2
  exit 1
}
cat >"$test_root/expected-modes" <<'EOF'
drwxr-xr-x ./
-rw-r--r-- ./BibitesMultiverse.dll
-rwxr-xr-x ./bibites-placeholder
-rwxr-xr-x ./install-broadcast-host
-rwxr-xr-x ./install-host
-rwxr-xr-x ./multiverse-sidecar
EOF
awk '{ print $1, $6 }' "$test_root/archive-metadata" >"$test_root/actual-modes"
cmp -s "$test_root/expected-modes" "$test_root/actual-modes" || {
  echo 'runtime archive modes are not normalized' >&2
  diff -u "$test_root/expected-modes" "$test_root/actual-modes" >&2 || true
  exit 1
}

non_gnu_bin="$test_root/non-gnu-bin"
install -d "$non_gnu_bin"
printf '%s\n' '#!/usr/bin/env bash' 'printf "%s\n" "bsdtar 3.7.0"' >"$non_gnu_bin/tar"
chmod 0755 "$non_gnu_bin/tar"
if env -u GOROOT \
  HOME="$test_root/home" \
  DOTNET_ROOT="$mock_bin" \
  PATH="$non_gnu_bin:$mock_bin:/usr/bin:/bin" \
  TEST_GAME_ZIP="$test_root/game.zip" \
  BIBITES_GAME_ZIP="$test_root/game.zip" \
  BIBITES_BEPINEX_ZIP="$test_root/bepinex.zip" \
  "$fixture/cloud/aws/build-artifacts.sh" >"$test_root/non-gnu.out" 2>"$test_root/non-gnu.err"; then
  echo 'artifact build accepted a non-GNU tar implementation' >&2
  exit 1
fi
grep -Fq 'GNU tar is required to build a deterministic runtime archive' \
  "$test_root/non-gnu.err" || {
    echo 'artifact build did not explain its GNU tar requirement' >&2
    exit 1
  }

limited_gnu_bin="$test_root/limited-gnu-bin"
install -d "$limited_gnu_bin"
cat >"$limited_gnu_bin/tar" <<'EOF'
#!/usr/bin/env bash
if [ "${1:-}" = --version ]; then
  printf '%s\n' 'tar (GNU tar) fixture'
  exit 0
fi
exit 2
EOF
chmod 0755 "$limited_gnu_bin/tar"
if env -u GOROOT \
  HOME="$test_root/home" \
  DOTNET_ROOT="$mock_bin" \
  PATH="$limited_gnu_bin:$mock_bin:/usr/bin:/bin" \
  TEST_GAME_ZIP="$test_root/game.zip" \
  BIBITES_GAME_ZIP="$test_root/game.zip" \
  BIBITES_BEPINEX_ZIP="$test_root/bepinex.zip" \
  "$fixture/cloud/aws/build-artifacts.sh" \
    >"$test_root/limited-gnu.out" 2>"$test_root/limited-gnu.err"; then
  echo 'artifact build accepted GNU tar without the required options' >&2
  exit 1
fi
grep -Fq 'GNU tar lacks the options required for a deterministic runtime archive' \
  "$test_root/limited-gnu.err" || {
    echo 'artifact build did not explain its GNU tar option requirements' >&2
    exit 1
  }

printf 'artifact build determinism fixture passed\n'
