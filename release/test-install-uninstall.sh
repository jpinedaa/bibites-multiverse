#!/usr/bin/env bash
#
# test-install-uninstall.sh
#
# Prove that the LINUX installer puts back exactly what it took, against sandbox
# copies of the real Linux game. The Windows half of this proof is
# test-install-uninstall.ps1; the two cover the same scenarios and this one adds
# the two that only exist on this platform.
#
# It runs the REAL install-bibites-multiverse.sh and
# uninstall-bibites-multiverse.sh against throwaway game copies under a temporary
# directory. It compares each tree hash-for-hash AND mode-for-mode before and
# after. It reads the source game but never changes it. It never touches a trust
# store, a running process, or the network.
#
# Eight scenarios:
#
#   A  a machine with no BepInEx. The installer adds it; the game then writes
#      BepInEx's config, log and cache; the uninstall must leave the tree
#      byte-identical to before the install, permissions included.
#   B  a machine that already has BepInEx and another mod. The installer must
#      touch neither, and the uninstall must remove only the multiverse plugin
#      and its own settings file.
#   C  a plugin somebody changed after the install. The uninstall must KEEP it
#      and say so, rather than deleting a file it did not leave there.
#   D  a game build that is not in the support matrix. The installer must refuse
#      with INS-GAMEBUILD and create nothing at all.
#   E  THE OTHER PLATFORM'S BUILD OF THE SAME GAME VERSION. A row is keyed on
#      (game version, platform), so the Windows assembly must refuse here exactly
#      as a stranger's build does - this is the scenario the schema change exists
#      for, and it cannot be run on Windows.
#   F  a private map, given --ca-file. Nothing may reach a trust store: the proof
#      is SSL_CERT_FILE in the start script and an empty system store.
#   G  a kit file that does not match MANIFEST.sha256. The installer must stop
#      with INS-CHECKSUM before it touches the game, and must not have made
#      anything executable.
#   H  a complete package. The same installer must create a versioned managed
#      runtime without --game-dir, and uninstall only unchanged payload files.
#
# Usage:
#   release/test-install-uninstall.sh --real-game-dir <path to the LINUX game>
#                                     [--windows-assembly <path>] [--bepinex-zip <path>]
#                                     [--kit-dir <a staged archive>] [--keep-sandbox]
#
# With no --kit-dir it stages a kit itself out of this checkout - the two real
# kit scripts, the support matrix extracted from docs/support-matrix.md, the real
# BepInEx archive, and stand-in files for the plugin and the sidecar, which this
# test never executes. That is what makes it runnable without a release build.
#
set -uo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
KIT_SRC="$REPO/release/kit"
MATRIX_DOC="$REPO/docs/support-matrix.md"

REAL_GAME_DIR=''
GAME_ASSEMBLY=''
WINDOWS_ASSEMBLY="$REPO/bibites-mod/libs/BibitesAssembly.dll"
BEPINEX_ZIP=''
KIT_DIR=''
KEEP_SANDBOX=0

while [ $# -gt 0 ]; do
  case "$1" in
    --real-game-dir)    REAL_GAME_DIR="$2";    shift 2 ;;
    --windows-assembly) WINDOWS_ASSEMBLY="$2"; shift 2 ;;
    --bepinex-zip)      BEPINEX_ZIP="$2";      shift 2 ;;
    --kit-dir)          KIT_DIR="$2";          shift 2 ;;
    --keep-sandbox)     KEEP_SANDBOX=1;        shift ;;
    *) printf 'unknown option: %s\n' "$1" >&2; exit 2 ;;
  esac
done

[ -n "$REAL_GAME_DIR" ] || { printf '%s needs --real-game-dir\n' "$0" >&2; exit 2; }
[ -d "$REAL_GAME_DIR" ] || { printf 'no such directory: %s\n' "$REAL_GAME_DIR" >&2; exit 2; }
REAL_GAME_DIR="$(cd "$REAL_GAME_DIR" && pwd)"
[ -x "$REAL_GAME_DIR/The Bibites.x86_64" ] || { printf 'not a real Linux game: %s\n' "$REAL_GAME_DIR" >&2; exit 2; }
GAME_ASSEMBLY="$REAL_GAME_DIR/The Bibites_Data/Managed/BibitesAssembly.dll"
[ -f "$GAME_ASSEMBLY" ] || { printf 'no such file: %s\n' "$GAME_ASSEMBLY" >&2; exit 2; }

CHECKS=0
FAILURES=0

check() { # $1 what, $2 ok (0/1), $3 detail
  CHECKS=$((CHECKS + 1))
  if [ "$2" -eq 0 ]; then
    printf '  PASS  %s\n' "$1"
  else
    FAILURES=$((FAILURES + 1))
    printf '  FAIL  %s\n' "$1"
    [ -n "${3:-}" ] && printf '        %s\n' "$(printf '%s' "$3" | head -c 2000)"
  fi
}
ok()   { check "$1" 0; }
scenario() { printf '\n==== %s\n' "$*"; }
contains() { case "$2" in *"$1"*) return 0 ;; esac; return 1; }
b() { if "$@"; then echo 0; else echo 1; fi; }

SANDBOX="$(mktemp -d "${TMPDIR:-/tmp}/bibites-multiverse-test-XXXXXXXX")"
trap '[ "$KEEP_SANDBOX" -eq 1 ] || rm -rf "$SANDBOX"' EXIT

# ---------------------------------------------------------------- the kit

if [ -z "$KIT_DIR" ]; then
  KIT_DIR="$SANDBOX/kit"
  mkdir -p "$KIT_DIR"
  cp "$KIT_SRC/install-bibites-multiverse.sh"   "$KIT_DIR/"
  cp "$KIT_SRC/uninstall-bibites-multiverse.sh" "$KIT_DIR/"
  [ -f "$KIT_SRC/README-linux.md" ] && cp "$KIT_SRC/README-linux.md" "$KIT_DIR/README.md"
  awk '/SUPPORT-MATRIX-JSON-BEGIN/{f=1;next} /SUPPORT-MATRIX-JSON-END/{f=0} f' "$MATRIX_DOC" \
    | sed '/^```/d' > "$KIT_DIR/support-matrix.json"
  [ -s "$KIT_DIR/support-matrix.json" ] || { printf 'no JSON block in %s\n' "$MATRIX_DOC" >&2; exit 2; }

  BEPINEX_VERSION="$(awk -F'"' '/"bepInEx":/ { print $4; exit }' "$KIT_DIR/support-matrix.json")"
  if [ -z "$BEPINEX_ZIP" ]; then
    for candidate in \
      "/mnt/wsl/data/scratch/m5-linux-rehearsal/bepinex/BepInEx_linux_x64_${BEPINEX_VERSION}.zip" \
      "$REPO/farend/dist/cache/BepInEx_linux_x64_${BEPINEX_VERSION}.zip" \
      "$REPO/release/dist/build/BepInEx_linux_x64_${BEPINEX_VERSION}.zip"
    do
      [ -f "$candidate" ] && { BEPINEX_ZIP="$candidate"; break; }
    done
  fi
  [ -n "$BEPINEX_ZIP" ] && [ -f "$BEPINEX_ZIP" ] || {
    printf 'no BepInEx_linux_x64_%s.zip found; pass --bepinex-zip\n' "$BEPINEX_VERSION" >&2; exit 2; }
  cp "$BEPINEX_ZIP" "$KIT_DIR/BepInEx_linux_x64_${BEPINEX_VERSION}.zip"

  # Stand-ins. The installer copies the plugin and records the sidecar's path; it
  # never runs either, and the uninstall works off hashes, so a stand-in proves
  # the same property a real artifact would.
  printf 'this stands in for the mod, which is platform-independent IL\n' > "$KIT_DIR/BibitesMultiverse.dll"
  printf '#!/bin/sh\necho "this stands in for the sidecar"\n' > "$KIT_DIR/multiverse-sidecar"

  ( cd "$KIT_DIR" && find . -type f ! -name MANIFEST.sha256 -printf '%P\n' | LC_ALL=C sort \
      | while read -r f; do printf '%s  %s\n' "$(sha256sum "$f" | cut -d' ' -f1)" "$f"; done \
      > MANIFEST.sha256 )
  # Exactly what a zip that lost its mode bits looks like, which is the state the
  # installer's step 1 exists to correct - in the right order.
  chmod 644 "$KIT_DIR"/*.sh "$KIT_DIR/multiverse-sidecar"
fi
KIT_DIR="$(cd "$KIT_DIR" && pwd)"
INSTALLER="$KIT_DIR/install-bibites-multiverse.sh"
UNINSTALLER="$KIT_DIR/uninstall-bibites-multiverse.sh"

printf 'sandbox: %s\n' "$SANDBOX"
printf 'kit    : %s\n' "$KIT_DIR"

# ---------------------------------------------------------------- helpers

snapshot() { # $1 root -> "<mode> <sha256> <relative path>" per file, sorted
  local root="$1"
  [ -d "$root" ] || return 0
  ( cd "$root" && find . \( -type f -o -type l \) -printf '%P\n' | LC_ALL=C sort \
      | while IFS= read -r f; do
          printf '%s %s %s\n' "$(stat -c '%a' "$f")" "$(sha256sum "$f" | cut -d' ' -f1)" "$f"
        done )
}

new_sandbox_game() { # $1 path
  local source base
  mkdir -p "$1"
  while IFS= read -r -d '' source; do
    base="$(basename "$source")"
    case "$base" in
      BepInEx|run_bepinex.sh|libdoorstop.so|.doorstop_version) continue ;;
    esac
    cp -a "$source" "$1/"
  done < <(find "$REAL_GAME_DIR" -mindepth 1 -maxdepth 1 -print0)
  [ -x "$1/The Bibites.x86_64" ] || { printf 'real game copy lost its executable: %s\n' "$1" >&2; exit 2; }
}

new_join_file() { # $1 path -> prints the secret
  local secret
  secret="$(head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n')"
  printf 'multiverse-join/1 wss://relay.example.test/contract-b/v4 test-world.%s\n' "$secret" > "$1"
  printf '%s' "$secret"
}

run_script() { # $1.. the command -> sets OUT and RC
  set +e
  OUT="$("$@" 2>&1)"
  RC=$?
  set -e
  set +u
  set -u
}

# ---------------------------------------------------------------- A

scenario "A - a machine with no BepInEx"

A_ROOT="$SANDBOX/A"; A_GAME="$A_ROOT/game"; A_DATA="$A_ROOT/data"
new_sandbox_game "$A_GAME"
A_JOIN="$A_ROOT/join.txt"
A_SECRET="$(new_join_file "$A_JOIN")"
BEFORE_A="$(snapshot "$A_GAME")"

run_script bash "$INSTALLER" --game-dir "$A_GAME" --data-root "$A_DATA" \
  --join-string-file "$A_JOIN" --world TestWorld
check "the installer succeeded" "$(b test "$RC" -eq 0)" "$OUT"
check "it states the all-four-edges default in its own output" \
  "$(b contains 'EXPORTS ON ALL FOUR EDGES' "$OUT")"
check "it states that nothing was written to any trust store" \
  "$(b contains 'NOTHING WAS WRITTEN TO ANY TRUST STORE' "$OUT")"
check "it says the mod is the same platform-independent IL the Windows kit ships" \
  "$(b contains 'platform-independent IL' "$OUT")"
check "it never tells anybody to pass an insecure flag" \
  "$(b bash -c 'case "$1" in *--insecure-*) exit 1 ;; esac; exit 0' _ "$OUT")"
check "it never asks for root" \
  "$(b bash -c 'case "$1" in *"sudo ./"*|*"sudo bash"*|*"curl "*"| sh"*) exit 1 ;; esac; exit 0' _ "$OUT")"
check "the plugin is in BepInEx/plugins" "$(b test -f "$A_GAME/BepInEx/plugins/BibitesMultiverse.dll")"
check "BepInEx was installed" "$(b test -f "$A_GAME/BepInEx/core/BepInEx.dll")"
check "BepInEx's own launcher is there" "$(b test -f "$A_GAME/run_bepinex.sh")"
check "and it is executable, which is the checksum-then-chmod step's whole point" \
  "$(b test -x "$A_GAME/run_bepinex.sh")"
check "libdoorstop.so is there" "$(b test -f "$A_GAME/libdoorstop.so")"
check ".doorstop_version is there" "$(b test -f "$A_GAME/.doorstop_version")"
check "the game's own changelog.txt was left unchanged" \
  "$(b cmp -s "$REAL_GAME_DIR/changelog.txt" "$A_GAME/changelog.txt")"
check "the start script was written" "$(b test -f "$KIT_DIR/start-multiverse.sh")"
check "the stop script was written" "$(b test -f "$KIT_DIR/stop-multiverse.sh")"
check "both are executable" \
  "$(b bash -c 'test -x "$1" && test -x "$2"' _ "$KIT_DIR/start-multiverse.sh" "$KIT_DIR/stop-multiverse.sh")"
check "the uninstaller was made executable, by name, after its checksum passed" \
  "$(b test -x "$UNINSTALLER")"

RECORD="$A_DATA/install-record.json"
check "the install record exists" "$(b test -f "$RECORD")"
if [ -f "$RECORD" ]; then
  REC="$(cat "$RECORD")"
  check "the record says BepInEx was installed by the installer" \
    "$(b contains '"installedByThisInstaller": true' "$REC")"
  check "the record says no system trust store was touched" \
    "$(b contains '"systemTrustStoreTouched": false' "$REC")"
  check "the record names the platform" "$(b contains '"platform": "Linux"' "$REC")"
  check "the record is valid JSON" \
    "$(b bash -c 'command -v python3 >/dev/null || exit 0; python3 -c "import json,sys; json.load(open(sys.argv[1]))" "$1"' _ "$RECORD")"
fi

CREDENTIAL="$A_DATA/peer-secret.txt"
check "the credential file exists" "$(b test -f "$CREDENTIAL")"
if [ -f "$CREDENTIAL" ]; then
  check "the credential file is mode 0600" "$(b test "$(stat -c '%a' "$CREDENTIAL")" = 600)"
  check "the credential file holds the secret half only" \
    "$(b test "$(cat "$CREDENTIAL")" = "$A_SECRET")"
  check "the credential file does not hold the identity half" \
    "$(b bash -c '! grep -q test-world "$1"' _ "$CREDENTIAL")"
fi

# The game runs: BepInEx writes its own configuration, log and cache. None of
# these three is in the BepInEx archive - they appear on the first start - so the
# uninstall has to account for files no manifest ever listed.
mkdir -p "$A_GAME/BepInEx/cache" "$A_GAME/BepInEx/config"
printf '[Logging]\n' > "$A_GAME/BepInEx/config/BepInEx.cfg"
printf '[M4]\n'      > "$A_GAME/BepInEx/config/dev.multiverse.bibites.cfg"
printf 'a log\n'     > "$A_GAME/BepInEx/LogOutput.log"
printf 'cache\n'     > "$A_GAME/BepInEx/cache/chainloader.dat"

START="$KIT_DIR/start-multiverse.sh"
if [ -f "$START" ]; then
  run_script bash -n "$START"
  check "the generated start script parses" "$(b test "$RC" -eq 0)" "$OUT"
  START_TEXT="$(cat "$START")"
  for setting in \
    "MULTIVERSE_EXPORT_EDGES='E,N,W,S'" \
    "MULTIVERSE_MIGRATION_EXCLUDE='Basic bibite'" \
    "MULTIVERSE_SAVE_MINUTES='10'" \
    "MULTIVERSE_SAVE_KEEP='6'" \
    "MULTIVERSE_SAVE_ON_QUIT='true'"
  do
    check "the start script sets ${setting%%=*} explicitly" \
      "$(b contains "export $setting" "$START_TEXT")"
  done
  check "the start script carries no secret" "$(b bash -c '! grep -qF "$2" <<<"$1"' _ "$START_TEXT" "$A_SECRET")"
  check "the start script passes the credential as a file, never as a value" \
    "$(b contains -- '--credential-file' "$START_TEXT")"
  check "the start script launches BepInEx's own run_bepinex.sh with the game binary" \
    "$(b contains './run_bepinex.sh "./$GAME_EXE"' "$START_TEXT")"
  check "the start script sets no WSLENV, because there is none to set here" \
    "$(b bash -c '! grep -qE "WSLENV=|export WSLENV" "$1"' _ "$START")"
  check "the start script sets the contract-A token path as a plain path" \
    "$(b contains 'MULTIVERSE_CONTRACT_A_TOKEN_FILE="$DATA_DIR/contract-a.token"' "$START_TEXT")"
  check "the start script warns about one instance per game folder" \
    "$(b contains 'ONE INSTANCE PER GAME FOLDER' "$START_TEXT")"
  check "the start script sets no SSL_CERT_FILE on a public map" \
    "$(b bash -c '! grep -q "^export SSL_CERT_FILE" <<<"$1"' _ "$START_TEXT")"
fi
if [ -f "$KIT_DIR/stop-multiverse.sh" ]; then
  run_script bash -n "$KIT_DIR/stop-multiverse.sh"
  check "the generated stop script parses" "$(b test "$RC" -eq 0)" "$OUT"
  check "the stop script asks before it kills, so save-on-quit can run" \
    "$(b bash -c 'grep -q -- "-TERM" "$1"' _ "$KIT_DIR/stop-multiverse.sh")"
fi

run_script bash "$UNINSTALLER" --data-root "$A_DATA" --dry-run
check "the dry run succeeded" "$(b test "$RC" -eq 0)" "$OUT"
check "the dry run changed nothing" "$(b test -f "$A_GAME/BepInEx/plugins/BibitesMultiverse.dll")"

run_script bash "$UNINSTALLER" --data-root "$A_DATA"
check "the uninstall succeeded" "$(b test "$RC" -eq 0)" "$OUT"

AFTER_A="$(snapshot "$A_GAME")"
DIFF_A="$(diff <(printf '%s\n' "$BEFORE_A") <(printf '%s\n' "$AFTER_A") || true)"
check "the game tree is hash-for-hash and mode-for-mode what it was before the install" \
  "$(b test -z "$DIFF_A")" "$DIFF_A"
check "no BepInEx directory is left behind" "$(b test ! -d "$A_GAME/BepInEx")"
check "no run_bepinex.sh is left behind" "$(b test ! -e "$A_GAME/run_bepinex.sh")"
check "no libdoorstop.so is left behind" "$(b test ! -e "$A_GAME/libdoorstop.so")"
check "no .doorstop_version is left behind" "$(b test ! -e "$A_GAME/.doorstop_version")"
check "the credential is gone" "$(b test ! -e "$CREDENTIAL")"
check "the install record is gone" "$(b test ! -e "$RECORD")"
check "the journal is kept, because nobody asked for it to go" "$(b test -d "$A_DATA/data")"
check "the start script is gone" "$(b test ! -e "$KIT_DIR/start-multiverse.sh")"
check "the stop script is gone" "$(b test ! -e "$KIT_DIR/stop-multiverse.sh")"

# ---------------------------------------------------------------- B

scenario "B - a machine that already has BepInEx and another mod"

B_ROOT="$SANDBOX/B"; B_GAME="$B_ROOT/game"; B_DATA="$B_ROOT/data"
new_sandbox_game "$B_GAME"
mkdir -p "$B_GAME/BepInEx/core" "$B_GAME/BepInEx/plugins" "$B_GAME/BepInEx/config"
printf 'somebody elses BepInEx\n' > "$B_GAME/BepInEx/core/BepInEx.dll"
printf 'another mod\n'            > "$B_GAME/BepInEx/plugins/SomeOtherMod.dll"
printf '[Logging]\n'              > "$B_GAME/BepInEx/config/BepInEx.cfg"
printf 'their own launcher\n'     > "$B_GAME/run_bepinex.sh"
chmod 755 "$B_GAME/run_bepinex.sh"
B_JOIN="$B_ROOT/join.txt"; new_join_file "$B_JOIN" >/dev/null
BEFORE_B="$(snapshot "$B_GAME")"

run_script bash "$INSTALLER" --game-dir "$B_GAME" --data-root "$B_DATA" --join-string-file "$B_JOIN"
check "the installer succeeded" "$(b test "$RC" -eq 0)" "$OUT"
check "it left the existing BepInEx alone" \
  "$(b contains 'already installed here; left exactly as it is' "$OUT")"
check "the record says BepInEx was not the installer's" \
  "$(b bash -c 'grep -q "\"installedByThisInstaller\": false" "$1"' _ "$B_DATA/install-record.json")"
check "the existing run_bepinex.sh was not overwritten" \
  "$(b bash -c 'grep -q "their own launcher" "$1"' _ "$B_GAME/run_bepinex.sh")"

printf '[M4]\n' > "$B_GAME/BepInEx/config/dev.multiverse.bibites.cfg"

run_script bash "$UNINSTALLER" --data-root "$B_DATA"
check "the uninstall succeeded" "$(b test "$RC" -eq 0)" "$OUT"
AFTER_B="$(snapshot "$B_GAME")"
DIFF_B="$(diff <(printf '%s\n' "$BEFORE_B") <(printf '%s\n' "$AFTER_B") || true)"
check "the game tree is hash-for-hash and mode-for-mode what it was before the install" \
  "$(b test -z "$DIFF_B")" "$DIFF_B"
check "the other mod's plugin is untouched" "$(b test -f "$B_GAME/BepInEx/plugins/SomeOtherMod.dll")"
check "the existing BepInEx is untouched" "$(b test -f "$B_GAME/BepInEx/core/BepInEx.dll")"

# ---------------------------------------------------------------- C

scenario "C - a plugin somebody changed after the install"

C_ROOT="$SANDBOX/C"; C_GAME="$C_ROOT/game"; C_DATA="$C_ROOT/data"
new_sandbox_game "$C_GAME"
C_JOIN="$C_ROOT/join.txt"; new_join_file "$C_JOIN" >/dev/null

run_script bash "$INSTALLER" --game-dir "$C_GAME" --data-root "$C_DATA" --join-string-file "$C_JOIN"
check "the installer succeeded" "$(b test "$RC" -eq 0)" "$OUT"

C_PLUGIN="$C_GAME/BepInEx/plugins/BibitesMultiverse.dll"
printf 'somebody replaced this by hand\n' > "$C_PLUGIN"

run_script bash "$UNINSTALLER" --data-root "$C_DATA"
check "the uninstall succeeded" "$(b test "$RC" -eq 0)" "$OUT"
check "the changed plugin was KEPT rather than deleted" "$(b test -f "$C_PLUGIN")"
check "the uninstall said so" "$(b contains 'CHANGED since the install' "$OUT")"

# ---------------------------------------------------------------- D

scenario "D - a game build that is not in the support matrix"

D_ROOT="$SANDBOX/D"; D_GAME="$D_ROOT/game"; D_DATA="$D_ROOT/data"
new_sandbox_game "$D_GAME"
printf 'a different game build\n' > "$D_GAME/The Bibites_Data/Managed/BibitesAssembly.dll"
D_JOIN="$D_ROOT/join.txt"; new_join_file "$D_JOIN" >/dev/null

run_script bash "$INSTALLER" --game-dir "$D_GAME" --data-root "$D_DATA" --join-string-file "$D_JOIN"
check "the installer refused" "$(b test "$RC" -eq 1)"
check "it refused with INS-GAMEBUILD" "$(b contains 'INS-GAMEBUILD' "$OUT")"
check "it quoted the matrix's own refusal" "$(b contains 'This release supports one game build' "$OUT")"
check "it said nothing was installed" "$(b contains 'NOTHING was installed' "$OUT")"
check "it named this machine's platform" "$(b contains "this machine's platform           : Linux" "$OUT")"
check "no BepInEx was installed" "$(b test ! -d "$D_GAME/BepInEx")"
check "no credential was written" "$(b test ! -e "$D_DATA/peer-secret.txt")"
check "no start script was written" "$(b test ! -e "$KIT_DIR/start-multiverse.sh")"

# ---------------------------------------------------------------- E

scenario "E - the OTHER platform's build of the same game version"

if [ -f "$WINDOWS_ASSEMBLY" ]; then
  E_ROOT="$SANDBOX/E"; E_GAME="$E_ROOT/game"; E_DATA="$E_ROOT/data"
  new_sandbox_game "$E_GAME"
  cp "$WINDOWS_ASSEMBLY" "$E_GAME/The Bibites_Data/Managed/BibitesAssembly.dll"
  E_JOIN="$E_ROOT/join.txt"; new_join_file "$E_JOIN" >/dev/null

  run_script bash "$INSTALLER" --game-dir "$E_GAME" --data-root "$E_DATA" --join-string-file "$E_JOIN"
  check "the installer refused a build that IS in the matrix, on the other platform" \
    "$(b test "$RC" -eq 1)" "$OUT"
  check "it refused with INS-GAMEBUILD" "$(b contains 'INS-GAMEBUILD' "$OUT")"
  check "it printed the row that matches, so the reader can see it is the wrong archive" \
    "$(b contains 'Windows/Steam' "$OUT")"
  check "it says a row is a game build AND a platform" \
    "$(b contains 'A row is a game build AND a platform' "$OUT")"
  check "nothing was installed" "$(b test ! -d "$E_GAME/BepInEx")"
else
  printf '  SKIP  no Windows BibitesAssembly.dll at %s\n' "$WINDOWS_ASSEMBLY"
fi

# ---------------------------------------------------------------- F

scenario "F - a private map, whose relay signs its own certificate"

F_ROOT="$SANDBOX/F"; F_GAME="$F_ROOT/game"; F_DATA="$F_ROOT/data"
new_sandbox_game "$F_GAME"
F_JOIN="$F_ROOT/join.txt"; new_join_file "$F_JOIN" >/dev/null
F_CA="$F_ROOT/ca.crt"
mkdir -p "$F_ROOT"
if command -v openssl >/dev/null 2>&1; then
  openssl req -x509 -newkey rsa:2048 -keyout "$F_ROOT/ca.key" -out "$F_CA" -days 30 -nodes \
    -subj '/CN=a test map operator' >/dev/null 2>&1
  F_CA_KIND='a real self-signed certificate'
else
  printf -- '-----BEGIN CERTIFICATE-----\nnot a real certificate\n-----END CERTIFICATE-----\n' > "$F_CA"
  F_CA_KIND='a PEM stand-in (openssl is not on this machine)'
fi
printf '     using %s\n' "$F_CA_KIND"

run_script bash "$INSTALLER" --game-dir "$F_GAME" --data-root "$F_DATA" \
  --join-string-file "$F_JOIN" --ca-file "$F_CA"
check "the installer succeeded" "$(b test "$RC" -eq 0)" "$OUT"
check "it printed what trusting an authority means" "$(b contains 'WHAT YOU ARE AGREEING TO' "$OUT")"
check "it says the authority is NOT added to /etc/ssl/certs" \
  "$(b contains 'NOT added to /etc/ssl/certs' "$OUT")"
check "it never tells anybody to run update-ca-certificates" \
  "$(b bash -c 'case "$1" in *"run update-ca-certificates"*|*"sudo update-ca-certificates"*) exit 1 ;; esac; exit 0' _ "$OUT")"
check "it kept a copy of the authority beside the data" "$(b test -f "$F_DATA/relay-ca.crt")"
check "the record says no system trust store was touched" \
  "$(b bash -c 'grep -q "\"systemTrustStoreTouched\": false" "$1"' _ "$F_DATA/install-record.json")"
check "the record names SSL_CERT_FILE as the trust mode" \
  "$(b bash -c 'grep -q "SSL_CERT_FILE" "$1"' _ "$F_DATA/install-record.json")"
check "the start script exports SSL_CERT_FILE at the copy" \
  "$(b bash -c 'grep -q "^export SSL_CERT_FILE=.\{0,\}relay-ca.crt" "$1"' _ "$KIT_DIR/start-multiverse.sh")"
run_script bash -n "$KIT_DIR/start-multiverse.sh"
check "the start script still parses with the certificate line in it" "$(b test "$RC" -eq 0)" "$OUT"

run_script bash "$UNINSTALLER" --data-root "$F_DATA"
check "the uninstall succeeded" "$(b test "$RC" -eq 0)" "$OUT"
check "it says it never wrote to a trust store" \
  "$(b contains 'wrote nothing to any trust store' "$OUT")"
check "the copy of the authority is gone" "$(b test ! -e "$F_DATA/relay-ca.crt")"

# ---------------------------------------------------------------- G

scenario "G - a kit file that does not match MANIFEST.sha256"

G_ROOT="$SANDBOX/G"; G_GAME="$G_ROOT/game"; G_DATA="$G_ROOT/data"; G_KIT="$G_ROOT/kit"
new_sandbox_game "$G_GAME"
cp -r "$KIT_DIR" "$G_KIT"
rm -f "$G_KIT/start-multiverse.sh" "$G_KIT/stop-multiverse.sh"
printf 'somebody changed this after it was published\n' > "$G_KIT/BibitesMultiverse.dll"
chmod 644 "$G_KIT/uninstall-bibites-multiverse.sh"
G_JOIN="$G_ROOT/join.txt"; new_join_file "$G_JOIN" >/dev/null

run_script bash "$G_KIT/install-bibites-multiverse.sh" --game-dir "$G_GAME" --data-root "$G_DATA" \
  --join-string-file "$G_JOIN"
check "the installer refused" "$(b test "$RC" -eq 1)"
check "it refused with INS-CHECKSUM" "$(b contains 'INS-CHECKSUM' "$OUT")"
check "it printed both hashes" "$(b contains 'this copy       :' "$OUT")"
check "it made nothing executable, because the checksum comes first" \
  "$(b test ! -x "$G_KIT/uninstall-bibites-multiverse.sh")"
check "no BepInEx was installed" "$(b test ! -d "$G_GAME/BepInEx")"
check "no credential was written" "$(b test ! -e "$G_DATA/peer-secret.txt")"

# ---------------------------------------------------------------- H

scenario "H - a complete package with a bundled game payload"

H_ROOT="$SANDBOX/H"; H_KIT="$H_ROOT/kit"; H_DATA="$H_ROOT/data"
mkdir -p "$H_KIT"
cp -a "$KIT_DIR/." "$H_KIT/"
rm -f "$H_KIT/start-multiverse.sh" "$H_KIT/stop-multiverse.sh"
mkdir -p "$H_KIT/game"
new_sandbox_game "$H_KIT/game"
H_SHA="$(sha256sum "$GAME_ASSEMBLY" | cut -d' ' -f1 | tr 'a-f' 'A-F')"
printf 'test redistribution permission\n' > "$H_KIT/GAME-REDISTRIBUTION-NOTICE.txt"
printf '{\n  "format": "bibites-multiverse/game-payload/1",\n  "platform": "Linux",\n  "gameVersion": "test",\n  "assemblySha256": "%s",\n  "redistributionNoticeFile": "GAME-REDISTRIBUTION-NOTICE.txt"\n}\n' \
  "$H_SHA" > "$H_KIT/game-payload.json"
( cd "$H_KIT" && find . -type f ! -name MANIFEST.sha256 -printf '%P\n' | LC_ALL=C sort \
    | while read -r f; do printf '%s  %s\n' "$(sha256sum "$f" | cut -d' ' -f1)" "$f"; done \
    > MANIFEST.sha256 )
chmod 644 "$H_KIT"/*.sh "$H_KIT/multiverse-sidecar"
H_JOIN="$H_ROOT/join.txt"; new_join_file "$H_JOIN" >/dev/null

run_script bash "$H_KIT/install-bibites-multiverse.sh" --game-dir "$H_KIT/game" \
  --data-root "$H_DATA" --join-string-file "$H_JOIN"
check "the complete edition refuses an external game path" "$(b test "$RC" -eq 1)" "$OUT"
check "that refusal has the INS-RUNTIME taxonomy id" "$(b contains 'INS-RUNTIME' "$OUT")"
check "the refused selection copied no managed runtime" "$(b test ! -d "$H_DATA/runtimes")"

run_script bash "$H_KIT/install-bibites-multiverse.sh" --data-root "$H_DATA" \
  --join-string-file "$H_JOIN"
check "the complete installer succeeded without --game-dir" "$(b test "$RC" -eq 0)" "$OUT"
check "it selected the complete edition" "$(b contains 'complete edition: installed' "$OUT")"
H_RUNTIME="$H_DATA/runtimes/$H_SHA"
check "the game was copied into the versioned managed runtime" \
  "$(b test -x "$H_RUNTIME/The Bibites.x86_64")"
check "the record identifies a bundled managed runtime" \
  "$(b bash -c 'grep -q '"'"'"mode": "bundled"'"'"' "$1" && grep -q '"'"'"managedByThisInstaller": true'"'"' "$1"' _ "$H_DATA/install-record.json")"
check "the generated start script points at the managed runtime" \
  "$(b bash -c 'grep -qF "$2" "$1"' _ "$H_KIT/start-multiverse.sh" "$H_RUNTIME")"

# A file not in the payload ledger belongs to the participant and keeps the
# runtime directory alive; all unchanged publisher files still go.
printf 'keep me\n' > "$H_RUNTIME/user-note.txt"
run_script bash "$H_KIT/uninstall-bibites-multiverse.sh" --data-root "$H_DATA"
check "the complete uninstall succeeded" "$(b test "$RC" -eq 0)" "$OUT"
check "the unchanged game executable was removed" "$(b test ! -e "$H_RUNTIME/The Bibites.x86_64")"
check "a user-added runtime file was kept" "$(b test -f "$H_RUNTIME/user-note.txt")"
check "the uninstall explains why the non-empty runtime stays" \
  "$(b contains 'not empty, so it stays' "$OUT")"

# ---------------------------------------------------------------- the verdict

printf '\n'
if [ "$FAILURES" -eq 0 ]; then
  printf 'ALL %s CHECKS PASSED\n' "$CHECKS"
else
  printf '%s of %s CHECKS FAILED\n' "$FAILURES" "$CHECKS"
fi

if [ "$KEEP_SANDBOX" -eq 1 ]; then
  printf 'sandbox kept at %s\n' "$SANDBOX"
else
  printf 'sandbox removed\n'
fi

[ "$FAILURES" -eq 0 ] || exit 1
exit 0
