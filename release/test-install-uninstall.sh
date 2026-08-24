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
# Nine scenarios:
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
#      Then the whole cycle that used to end in INS-RUNTIME: a world runs and
#      BepInEx writes its log, its config and its cache; a second install over
#      the same managed runtime still records that framework as its own; the
#      uninstall reclaims the managed game copy WHOLE, residue included; and a
#      third install starts again from the payload. A husk built here by hand -
#      every recorded game file gone, the framework left - is rebuilt by an
#      install rather than refused, and a game file somebody changed is still
#      kept and still keeps the directory around it.
#   L  a secret nothing ever used: an install that stopped before the world ever
#      ran leaves a credential belonging to no world, so it is renamed aside and
#      a new identity is enrolled - while a journal, or any sidecar log line with
#      a peer in it, still means a world exists and the refusal stands.
#   K  the sidecar's own log as an identity source: one world in it is adopted
#      with the relay from the same line, two worlds are refused and listed, the
#      printed remedy works, a folder whose logs name two worlds is not enrolled
#      over without the switch, and a kit unpacked beside the data root counts.
#   J  what may adopt a world and what may never overwrite one: a credential
#      behind a blank first line, a file that is not a credential at all, a name
#      whose secret is gone, a join string backed only by a hand-written claim,
#      the one handover that does replace a secret, a private world adopted whole
#      after an uninstall, --relay-url pointed elsewhere, and a loose mode.
#   I  public-map enrollment on Linux. A failed response must keep one private
#      pending identity. A retry must reuse it. A repair must not enroll again.
#      An uninstall must KEEP the world's credential, so installing again over
#      the same data root is the same world on the same slot; --remove-world-data
#      is what ends it. A secret with nothing left to name its world is refused
#      rather than overwritten, and naming it in data/peer-id is the way back.
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
contains() { # [--] $1 needle, $2 text
  [ "$1" != -- ] || shift
  case "$2" in *"$1"*) return 0 ;; esac; return 1
}
b() { if "$@"; then echo 0; else echo 1; fi; }

check "the text helper matches an option-like literal after --" \
  "$(b contains -- '--credential-file' 'sidecar --credential-file path')"
check "the text helper does not reduce an option-like literal to --" \
  "$(if contains -- '--credential-file' 'sidecar --different-option path'; then echo 1; else echo 0; fi)"

SANDBOX="$(mktemp -d "${TMPDIR:-/tmp}/bibites-multiverse-test-XXXXXXXX")"
trap '[ "$KEEP_SANDBOX" -eq 1 ] || rm -rf "$SANDBOX"' EXIT

# ---------------------------------------------------------------- the kit

if [ -z "$KIT_DIR" ]; then
  KIT_DIR="$SANDBOX/kit"
  mkdir -p "$KIT_DIR"
  cp "$KIT_SRC/install-bibites-multiverse.sh"   "$KIT_DIR/"
  cp "$KIT_SRC/uninstall-bibites-multiverse.sh" "$KIT_DIR/"
  [ -f "$KIT_SRC/README-linux.md" ] && cp "$KIT_SRC/README-linux.md" "$KIT_DIR/README.md"
  cp "$KIT_SRC/public-map.json" "$KIT_DIR/"
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
[ -f "$KIT_DIR/public-map.json" ] || {
  printf 'the Linux kit has no public-map.json: %s\n' "$KIT_DIR" >&2
  exit 2
}

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
check "the private join file overrides the packaged public map" \
  "$(b contains "your world's identity on this map: test-world" "$OUT")"
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
    "MULTIVERSE_SAVE_ON_QUIT='true'" \
    "MULTIVERSE_STARTUP_TIME_SCALE='10'"
  do
    check "the start script sets ${setting%%=*} explicitly" \
      "$(b contains "export $setting" "$START_TEXT")"
  done
  # The mod's command channel. Nothing on Linux needs it - stop-multiverse.sh
  # sends SIGTERM, which a headless game handles - but the world is told about it
  # anyway, so one world answers one request whichever front door reaches it.
  check "the start script names this world's mod command file" \
    "$(b contains 'export MULTIVERSE_CMD_FILE="$DATA_ROOT/cmd.txt"' "$START_TEXT")"
  check "the start script clears a command an interrupted stop left behind" \
    "$(b contains 'rm -f "$MULTIVERSE_CMD_FILE" "$MULTIVERSE_CMD_FILE.log"' "$START_TEXT")"
  check "the start script carries no secret" "$(b bash -c '! grep -qF "$2" <<<"$1"' _ "$START_TEXT" "$A_SECRET")"
  check "the start script passes the credential as a file, never as a value" \
    "$(b contains -- '--credential-file' "$START_TEXT")"
  check "the start script launches BepInEx's own run_bepinex.sh with the game binary" \
    "$(b contains './run_bepinex.sh "./$GAME_EXE"' "$START_TEXT")"
  check "the start script sets no WSLENV, because there is none to set here" \
    "$(b bash -c '! grep -qE "WSLENV=|export WSLENV" "$1"' _ "$START")"
  check "the start script sets the contract-A token path as a plain path" \
    "$(b contains 'MULTIVERSE_CONTRACT_A_TOKEN_FILE="$DATA_DIR/contract-a.token"' "$START_TEXT")"
  # THE SILENT-FAILURE GATE. A start script that does not check whether the mod
  # reached the sidecar is a start script that reports success for a world sitting
  # at the main menu (LOCAL-CONFIGRACE).
  for probe in \
    '/my-slot' \
    'THE GAME STARTED BUT ITS MOD HAS NOT REACHED THE SIDECAR' \
    'LOCAL-CONFIGRACE' \
    'LOCAL-STARVATION' \
    'the game joined the map: mod connected'
  do
    check "the start script checks that the mod connected ($probe)" \
      "$(b contains -- "$probe" "$START_TEXT")"
  done
  check "the start script's mod check is a warning, not a failure" \
    "$(b contains 'this is a warning, not a failure' "$START_TEXT")"

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
check "the credential is kept, because the world it names is still on the map" \
  "$(b test -f "$CREDENTIAL")"
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
# Keep this record larger than 64 KiB. An early field must not cause the pipe
# writer to fail while the installer or uninstaller reads the remaining fields.
mkdir -p "$H_KIT/game/The Bibites_Data/StreamingAssets/pipe-status-regression"
H_PIPE_ENTRY=0
while [ "$H_PIPE_ENTRY" -lt 128 ]; do
  H_PIPE_ENTRY=$((H_PIPE_ENTRY + 1))
  printf 'pipe status regression fixture\n' \
    > "$H_KIT/game/The Bibites_Data/StreamingAssets/pipe-status-regression/entry-$H_PIPE_ENTRY.txt"
done
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
check "the install record is larger than 64 KiB" \
  "$(b test "$(stat -c %s "$H_DATA/install-record.json")" -gt 65536)"

# THE CYCLE THAT COULD NOT RUN, end to end. Install, run a world, install again
# over the same managed runtime, uninstall, install again. The release before
# this fix stopped dead at the last step - "The managed runtime at ... is
# incomplete (changelog.txt is missing). It was not overwritten." - with no way
# past it but deleting a directory by hand.

# What the game and BepInEx write into the game directory once a world has run.
mkdir -p "$H_RUNTIME/BepInEx/cache" "$H_RUNTIME/BepInEx/config"
printf 'a world ran here\n' > "$H_RUNTIME/BepInEx/LogOutput.log"
printf '[Logging]\n'        > "$H_RUNTIME/BepInEx/config/BepInEx.cfg"
printf 'cache\n'            > "$H_RUNTIME/BepInEx/cache/chainloader_typeloader.dat"

run_script bash "$H_KIT/install-bibites-multiverse.sh" --data-root "$H_DATA" \
  --join-string-file "$H_JOIN"
check "installing again over the managed runtime succeeded" "$(b test "$RC" -eq 0)" "$OUT"
check "it reused the game copy it had already verified" \
  "$(b contains 'reusing the verified managed game runtime' "$OUT")"
# THE BUG THAT MADE THE HUSK. This install finds BepInEx already in the managed
# runtime - because the install before it put it there - and used to record it as
# somebody else's, with no files, so the uninstall left the whole framework
# behind while removing the game payload around it.
check "it says the framework in that directory is its own" \
  "$(b contains "already in this package's own managed game copy" "$OUT")"
check "the second install still owns the BepInEx in its own managed runtime" \
  "$(b grep -q '"installedByThisInstaller": true' "$H_DATA/install-record.json")"

printf 'left in the game copy\n' > "$H_RUNTIME/user-note.txt"
run_script bash "$H_KIT/uninstall-bibites-multiverse.sh" --data-root "$H_DATA"
check "the complete uninstall succeeded" "$(b test "$RC" -eq 0)" "$OUT"
check "the unchanged game executable was removed" "$(b test ! -e "$H_RUNTIME/The Bibites.x86_64")"
# Nothing this install recorded is left in that directory, so it is not a game
# any more - and this package's own copy of the game, with the log, the config
# and the cache the game wrote inside it, goes whole rather than staying behind
# as something the next install cannot repair.
check "the managed game copy went whole, residue and all" "$(b test ! -d "$H_RUNTIME")"
check "the uninstall said how much it removed beyond its own record" \
  "$(b contains 'file(s) created by the game and BepInEx after the install' "$OUT")"
check "the runtimes directory went with the last runtime in it" \
  "$(b test ! -d "$H_DATA/runtimes")"

# THE INSTALL THAT USED TO DIE. Nothing about this run is special: the same
# package, the same data root, the same world.
run_script bash "$H_KIT/install-bibites-multiverse.sh" --data-root "$H_DATA" \
  --join-string-file "$H_JOIN"
check "installing again after the uninstall succeeded" "$(b test "$RC" -eq 0)" "$OUT"
check "nothing about the managed runtime was refused" \
  "$(if contains 'INS-RUNTIME' "$OUT"; then echo 1; else echo 0; fi)" "$OUT"
check "it staged the game payload again" \
  "$(b contains 'installed the verified game payload into a managed runtime' "$OUT")"
check "the game is back in the managed runtime" "$(b test -x "$H_RUNTIME/The Bibites.x86_64")"

# THE HUSK THE UNINSTALL BEFORE THIS FIX LEFT BEHIND, made here exactly as that
# release made it: every recorded game file gone, the framework still there. A
# machine that already carries one is healed by an install rather than refused.
# Every absolute path the record names inside the managed runtime: the game
# payload this install staged there, which is exactly what its uninstall took.
while IFS= read -r H_PAYLOAD_FILE; do
  [ -n "$H_PAYLOAD_FILE" ] || continue
  rm -f "$H_PAYLOAD_FILE"
done < <(grep -o '"path": "[^"]*"' "$H_DATA/install-record.json" | cut -d'"' -f4 | grep -F "$H_RUNTIME/")
check "the husk has no game left in it" "$(b test ! -e "$H_RUNTIME/The Bibites.x86_64")"
check "and still holds the mod framework" "$(b test -d "$H_RUNTIME/BepInEx")"
run_script bash "$H_KIT/install-bibites-multiverse.sh" --data-root "$H_DATA" \
  --join-string-file "$H_JOIN"
check "an install over that husk succeeded" "$(b test "$RC" -eq 0)" "$OUT"
check "it said what it found there" "$(b contains 'is not the game any more' "$OUT")"
check "it removed the incomplete copy whole" \
  "$(b contains 'removed the incomplete managed runtime whole' "$OUT")"
check "and put the game back" "$(b test -x "$H_RUNTIME/The Bibites.x86_64")"

# A GAME FILE SOMEBODY CHANGED IS STILL KEPT, and keeps the directory around it.
# The sweep is for rubble; a copy something in it is still vouching for is not
# rubble, and neither script decides that for you.
printf 'changed by hand\n' >> "$H_RUNTIME/The Bibites.x86_64"
run_script bash "$H_KIT/uninstall-bibites-multiverse.sh" --data-root "$H_DATA"
check "the uninstall after a hand-changed game file succeeded" "$(b test "$RC" -eq 0)" "$OUT"
check "it kept that file and said why" "$(b contains 'CHANGED since the install' "$OUT")"
check "the changed file is still there" "$(b test -f "$H_RUNTIME/The Bibites.x86_64")"
check "and the directory around it stayed" "$(b test -d "$H_RUNTIME")"

# ---------------------------------------------------------------- I

scenario "I - automatic Linux enrollment, safe retry, and identity reuse"

I_ROOT="$SANDBOX/I"; I_GAME="$I_ROOT/game"; I_DATA="$I_ROOT/data"
I_FAKE_BIN="$I_ROOT/fake-bin"; I_REQUESTS="$I_ROOT/requests.jsonl"; I_ARGS="$I_ROOT/curl-args.txt"
I_STATE="$I_ROOT/fail-first.state"
new_sandbox_game "$I_GAME"
mkdir -p "$I_FAKE_BIN"
cat > "$I_FAKE_BIN/curl" <<'FAKE_CURL'
#!/usr/bin/env bash
set -eu
body="$(cat)"
printf '%s\n' "$body" >> "$FAKE_ENROLL_REQUESTS"
printf '%s\n' "$*" >> "$FAKE_ENROLL_ARGS"
if [ "${FAKE_ENROLL_FAIL_FIRST:-0}" = 1 ] && [ ! -e "$FAKE_ENROLL_STATE" ]; then
  : > "$FAKE_ENROLL_STATE"
  printf '%s\n' 'simulated lost enrollment response' >&2
  exit 22
fi
install_id="$(printf '%s' "$body" | sed -n 's/.*"installId":"\([^"]*\)".*/\1/p')"
[ -n "$install_id" ] || exit 65
peer_id="public-$(printf '%s' "$install_id" | tr -d '-' | tr 'A-F' 'a-f')"
printf '{"format":"bibites-multiverse/enrollment-response/1","relayUrl":"%s","peerId":"%s","created":true}\n' \
  "$FAKE_ENROLL_RELAY_URL" "$peer_id"
FAKE_CURL
chmod 755 "$I_FAKE_BIN/curl"

run_script env PATH="$I_FAKE_BIN:$PATH" \
  FAKE_ENROLL_REQUESTS="$I_REQUESTS" FAKE_ENROLL_STATE="$I_STATE" \
  FAKE_ENROLL_ARGS="$I_ARGS" \
  FAKE_ENROLL_FAIL_FIRST=1 \
  FAKE_ENROLL_RELAY_URL='wss://bibitesmultiverse.com/contract-b/v4' \
  bash "$INSTALLER" --game-dir "$I_GAME" --data-root "$I_DATA"
check "the lost response stops with INS-ENROLL" \
  "$(b bash -c 'test "$1" -eq 1 && case "$2" in *INS-ENROLL*) exit 0 ;; *) exit 1 ;; esac' _ "$RC" "$OUT")" "$OUT"
check "the failed request keeps a pending identity" \
  "$(b test -f "$I_DATA/enrollment-pending.json")"
check "the pending identity is mode 0600" \
  "$(b test "$(stat -c '%a' "$I_DATA/enrollment-pending.json" 2>/dev/null || true)" = 600)"
check "the failed request writes no final credential" \
  "$(b test ! -e "$I_DATA/peer-secret.txt")"

run_script env PATH="$I_FAKE_BIN:$PATH" \
  FAKE_ENROLL_REQUESTS="$I_REQUESTS" FAKE_ENROLL_STATE="$I_STATE" \
  FAKE_ENROLL_ARGS="$I_ARGS" \
  FAKE_ENROLL_FAIL_FIRST=1 \
  FAKE_ENROLL_RELAY_URL='wss://bibitesmultiverse.com/contract-b/v4' \
  bash "$INSTALLER" --game-dir "$I_GAME" --data-root "$I_DATA"
check "the safe enrollment retry succeeds" "$(b test "$RC" -eq 0)" "$OUT"
check "the retry reports that it reused the pending identity" \
  "$(b contains 'retrying the pending public-map enrollment' "$OUT")"
check "the two enrollment requests are byte-identical" \
  "$(b bash -c 'test "$(sed -n "1p" "$1")" = "$(sed -n "2p" "$1")"' _ "$I_REQUESTS")"
I_REQUEST_SECRET="$(sed -n '1s/.*"secret":"\([^"]*\)".*/\1/p' "$I_REQUESTS")"
check "curl receives the secret through standard input, not its command line" \
  "$(b bash -c '! grep -qF "$2" "$1"' _ "$I_ARGS" "$I_REQUEST_SECRET")"
check "the request identifies release 0.3.7" \
  "$(b grep -qF '"release":"0.3.7"' "$I_REQUESTS")"
check "the completed install removes the pending identity" \
  "$(b test ! -e "$I_DATA/enrollment-pending.json")"
check "the completed credential is mode 0600" \
  "$(b test "$(stat -c '%a' "$I_DATA/peer-secret.txt" 2>/dev/null || true)" = 600)"
check "the install record names a public identity" \
  "$(b grep -Eq '"peerId": "public-[0-9a-f]{32}"' "$I_DATA/install-record.json")"
I_SECRET="$(cat "$I_DATA/peer-secret.txt")"
I_REQUEST_COUNT="$(wc -l < "$I_REQUESTS" | tr -d ' ')"

run_script env PATH="$I_FAKE_BIN:$PATH" \
  FAKE_ENROLL_REQUESTS="$I_REQUESTS" FAKE_ENROLL_STATE="$I_STATE" \
  FAKE_ENROLL_ARGS="$I_ARGS" \
  FAKE_ENROLL_FAIL_FIRST=1 \
  FAKE_ENROLL_RELAY_URL='wss://bibitesmultiverse.com/contract-b/v4' \
  bash "$INSTALLER" --game-dir "$I_GAME" --data-root "$I_DATA"
check "a repair reuses the completed public identity" "$(b test "$RC" -eq 0)" "$OUT"
check "the repair reports identity reuse" \
  "$(b contains "reusing the map identity already in $I_DATA" "$OUT")"
check "the repair sends no enrollment request" \
  "$(b test "$(wc -l < "$I_REQUESTS" | tr -d ' ')" = "$I_REQUEST_COUNT")"
check "the repair keeps the same secret" \
  "$(b test "$(cat "$I_DATA/peer-secret.txt")" = "$I_SECRET")"

I_PEER_ID="$(sed -n 's/.*"peerId": "\(public-[0-9a-f]*\)".*/\1/p' "$I_DATA/install-record.json" | head -n1)"
check "the record names the world this data root now owns" \
  "$(b test -n "$I_PEER_ID")"
check "the installer wrote the identity beside the journal" \
  "$(b bash -c 'test "$(cat "$1")" = "$2"' _ "$I_DATA/data/peer-id" "$I_PEER_ID")"

run_script bash "$UNINSTALLER" --data-root "$I_DATA"
check "the public-map install uninstalls" "$(b test "$RC" -eq 0)" "$OUT"
check "the uninstall KEEPS the world's credential" \
  "$(b test -f "$I_DATA/peer-secret.txt")"
check "the uninstall says the identity stays" \
  "$(b contains 'keeps its place on the map' "$OUT")"
check "the uninstall keeps the identity beside the journal" \
  "$(b test -f "$I_DATA/data/peer-id")"
check "the uninstall removes its own record" \
  "$(b test ! -e "$I_DATA/install-record.json")"

# The whole point of keeping it: installing again over an uninstalled world is
# the same world, on the same slot, with no second identity spent on the map.
run_script env PATH="$I_FAKE_BIN:$PATH" \
  FAKE_ENROLL_REQUESTS="$I_REQUESTS" FAKE_ENROLL_STATE="$I_STATE" \
  FAKE_ENROLL_ARGS="$I_ARGS" \
  FAKE_ENROLL_FAIL_FIRST=1 \
  FAKE_ENROLL_RELAY_URL='wss://bibitesmultiverse.com/contract-b/v4' \
  bash "$INSTALLER" --game-dir "$I_GAME" --data-root "$I_DATA"
check "installing again after an uninstall succeeds" "$(b test "$RC" -eq 0)" "$OUT"
check "it reports that it reused the world already in the data root" \
  "$(b contains "reusing the map identity already in $I_DATA" "$OUT")"
check "it sends no enrollment request" \
  "$(b test "$(wc -l < "$I_REQUESTS" | tr -d ' ')" = "$I_REQUEST_COUNT")"
check "it keeps the same secret, byte for byte" \
  "$(b test "$(cat "$I_DATA/peer-secret.txt")" = "$I_SECRET")"
check "the new record names the same world" \
  "$(b bash -c 'grep -qF "\"peerId\": \"$2\"" "$1"' _ "$I_DATA/install-record.json" "$I_PEER_ID")"
check "the new start script names the same world" \
  "$(b bash -c 'grep -qF "PEER_ID='"'"'$2'"'"'" "$1"' _ "$KIT_DIR/start-multiverse.sh" "$I_PEER_ID")"

run_script bash "$UNINSTALLER" --data-root "$I_DATA" --remove-world-data
check "the uninstall with --remove-world-data succeeds" "$(b test "$RC" -eq 0)" "$OUT"
check "--remove-world-data removes the credential" \
  "$(b test ! -e "$I_DATA/peer-secret.txt")"
check "--remove-world-data says the world ends on the map" \
  "$(b contains 'end of this world on the map' "$OUT")"

# A secret with nothing left to name it, in a folder a world HAS run in, is the
# state the installer refuses: overwriting it would destroy the only recoverable
# half of an identity that still holds a place on the map. The journal is what
# says a world ran here - scenario L covers the folder where none ever did.
I_ORPHAN="$I_ROOT/orphan"
mkdir -p "$I_ORPHAN/data/journal"
printf '%s\n' '0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef' > "$I_ORPHAN/peer-secret.txt"
chmod 600 "$I_ORPHAN/peer-secret.txt"
printf '{"seq":1}\n' > "$I_ORPHAN/data/journal/journal.log"
run_script env PATH="$I_FAKE_BIN:$PATH" \
  FAKE_ENROLL_REQUESTS="$I_REQUESTS" FAKE_ENROLL_STATE="$I_STATE" \
  FAKE_ENROLL_ARGS="$I_ARGS" \
  FAKE_ENROLL_RELAY_URL='wss://bibitesmultiverse.com/contract-b/v4' \
  bash "$INSTALLER" --game-dir "$I_GAME" --data-root "$I_ORPHAN"
check "a secret no file can name, where a world has run, stops with INS-ENROLL" \
  "$(b bash -c 'test "$1" -eq 1 && case "$2" in *INS-ENROLL*) exit 0 ;; *) exit 1 ;; esac' _ "$RC" "$OUT")" "$OUT"
check "the refusal names the file it would not overwrite" \
  "$(b contains "$I_ORPHAN/peer-secret.txt" "$OUT")"
check "the refusal names the file that would name the world" \
  "$(b contains "$I_ORPHAN/data/peer-id" "$OUT")"
check "the refusal leaves that secret untouched" \
  "$(b bash -c 'test "$(cat "$1")" = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"' _ "$I_ORPHAN/peer-secret.txt")"
check "the refusal sends no enrollment request" \
  "$(b test "$(wc -l < "$I_REQUESTS" | tr -d ' ')" = "$I_REQUEST_COUNT")"

# Naming the world is the remedy the refusal prints, and it has to work.
printf '%s\n' 'public-ffffffffffffffffffffffffffffffff' > "$I_ORPHAN/data/peer-id"
run_script env PATH="$I_FAKE_BIN:$PATH" \
  FAKE_ENROLL_REQUESTS="$I_REQUESTS" FAKE_ENROLL_STATE="$I_STATE" \
  FAKE_ENROLL_ARGS="$I_ARGS" \
  FAKE_ENROLL_RELAY_URL='wss://bibitesmultiverse.com/contract-b/v4' \
  bash "$INSTALLER" --game-dir "$I_GAME" --data-root "$I_ORPHAN"
check "naming the world in data/peer-id installs it" "$(b test "$RC" -eq 0)" "$OUT"
check "the named world is the one the record carries" \
  "$(b grep -qF '"peerId": "public-ffffffffffffffffffffffffffffffff"' "$I_ORPHAN/install-record.json")"
check "naming the world sends no enrollment request" \
  "$(b test "$(wc -l < "$I_REQUESTS" | tr -d ' ')" = "$I_REQUEST_COUNT")"

# ---------------------------------------------------------------- J

scenario "J - what may adopt a world, and what may never overwrite one"

J_ROOT="$SANDBOX/J"; J_GAME="$J_ROOT/game"; J_REQUESTS="$J_ROOT/requests.jsonl"
J_ARGS="$J_ROOT/curl-args.txt"; J_STATE="$J_ROOT/unused.state"
new_sandbox_game "$J_GAME"
mkdir -p "$J_ROOT"
: > "$J_REQUESTS"
J_PUBLIC_RELAY='wss://bibitesmultiverse.com/contract-b/v4'
j_install() { # $@ -> installer arguments, with the fake enrollment endpoint on PATH
  run_script env PATH="$I_FAKE_BIN:$PATH" \
    FAKE_ENROLL_REQUESTS="$J_REQUESTS" FAKE_ENROLL_STATE="$J_STATE" \
    FAKE_ENROLL_ARGS="$J_ARGS" FAKE_ENROLL_FAIL_FIRST=0 \
    FAKE_ENROLL_RELAY_URL="$J_PUBLIC_RELAY" \
    bash "$INSTALLER" --game-dir "$J_GAME" "$@"
}
j_requests() { wc -l < "$J_REQUESTS" | tr -d ' '; }

# J1 - a credential whose FIRST LINE is blank is still a credential. Reading one
# line rather than the file would call it absent, delete it, and spend a second
# identity.
J1="$J_ROOT/blank-first-line"; mkdir -p "$J1/data"
J1_SECRET="$(printf 'e%.0s' $(seq 1 64))"
printf '\n%s\n' "$J1_SECRET" > "$J1/peer-secret.txt"
chmod 600 "$J1/peer-secret.txt"
J1_BEFORE="$(sha256sum "$J1/peer-secret.txt" | cut -d' ' -f1)"
printf 'public-33333333333333333333333333333333\n' > "$J1/data/peer-id"
J1_COUNT="$(j_requests)"
j_install --data-root "$J1"
check "a credential behind a blank first line is adopted, not replaced" "$(b test "$RC" -eq 0)" "$OUT"
check "it says so" "$(b contains "reusing the map identity already in $J1" "$OUT")"
check "it spends no identity on it" "$(b test "$(j_requests)" = "$J1_COUNT")"
check "the credential is byte-identical afterwards" \
  "$(b bash -c 'test "$(sha256sum "$1" | cut -d" " -f1)" = "$2"' _ "$J1/peer-secret.txt" "$J1_BEFORE")"
check "the record names the world that file belongs to" \
  "$(b grep -qF '"peerId": "public-33333333333333333333333333333333"' "$J1/install-record.json")"

# J2 - something that is not a credential is a refusal, never an overwrite.
J2="$J_ROOT/not-a-credential"; mkdir -p "$J2"
printf 'hello\n' > "$J2/peer-secret.txt"
J2_COUNT="$(j_requests)"
j_install --data-root "$J2"
check "a peer-secret.txt that is not a credential stops with INS-ENROLL" \
  "$(b bash -c 'test "$1" -eq 1 && case "$2" in *INS-ENROLL*) exit 0 ;; *) exit 1 ;; esac' _ "$RC" "$OUT")" "$OUT"
check "the refusal says what a credential is" "$(b contains 'not a map credential' "$OUT")"
check "it changes that file not at all" \
  "$(b bash -c 'test "$(cat "$1")" = "hello"' _ "$J2/peer-secret.txt")"
check "it asks the map for nothing" "$(b test "$(j_requests)" = "$J2_COUNT")"

# J3 - a name with no secret is a SECOND place on the map, and that is a decision.
J3="$J_ROOT/lost-secret"; mkdir -p "$J3/data"
printf 'public-44444444444444444444444444444444\n' > "$J3/data/peer-id"
J3_COUNT="$(j_requests)"
j_install --data-root "$J3"
check "a lost secret stops rather than taking a new identity" \
  "$(b bash -c 'test "$1" -eq 1 && case "$2" in *INS-ENROLL*) exit 0 ;; *) exit 1 ;; esac' _ "$RC" "$OUT")" "$OUT"
check "the refusal names the world that goes dark" \
  "$(b contains 'public-44444444444444444444444444444444' "$OUT")"
check "the refusal names the switch that accepts the cost" \
  "$(b contains '--replace-world-identity' "$OUT")"
check "the refusal names the handover that keeps the place instead" \
  "$(b contains 'handover' "$OUT")"
check "nothing was enrolled" "$(b test "$(j_requests)" = "$J3_COUNT")"
check "no credential was written" "$(b test ! -e "$J3/peer-secret.txt")"

j_install --data-root "$J3" --replace-world-identity
check "the switch takes the new identity" "$(b test "$RC" -eq 0)" "$OUT"
check "and it enrolled exactly once" "$(b test "$(j_requests)" = "$((J3_COUNT + 1))")"
check "the world that went dark is kept where the operator can be told" \
  "$(b bash -c 'test "$(cat "$1")" = "public-44444444444444444444444444444444"' _ "$J3/data/peer-id.previous")"
check "the summary names it too" "$(b contains 'NOW DARK ON THE MAP' "$OUT")"
check "the new identity is the one in the record" \
  "$(b grep -Eq '"peerId": "public-[0-9a-f]{32}"' "$J3/install-record.json")"
check "and it is not the old one" \
  "$(b bash -c '! grep -qF "public-44444444444444444444444444444444" "$1"' _ "$J3/install-record.json")"

# J4 - a join string may not destroy a secret on the word of a file anybody can
# write. data/peer-id is exactly that file: the refusal above tells people to
# write it.
J4="$J_ROOT/unproven"; mkdir -p "$J4/data"
J4_SECRET="$(printf 'y%.0s' $(seq 1 64))"
printf '%s\n' "$J4_SECRET" > "$J4/peer-secret.txt"
chmod 600 "$J4/peer-secret.txt"
printf 'public-55555555555555555555555555555555\n' > "$J4/data/peer-id"
printf 'multiverse-join/1 %s public-55555555555555555555555555555555.%s\n' \
  "$J_PUBLIC_RELAY" "$(printf 'x%.0s' $(seq 1 64))" > "$J_ROOT/j4-join.txt"
J4_COUNT="$(j_requests)"
j_install --data-root "$J4" --join-string-file "$J_ROOT/j4-join.txt"
check "a join string over an UNPROVEN claim stops with INS-ENROLL" \
  "$(b bash -c 'test "$1" -eq 1 && case "$2" in *INS-ENROLL*) exit 0 ;; *) exit 1 ;; esac' _ "$RC" "$OUT")" "$OUT"
check "the refusal names the file that made the claim" "$(b contains "$J4/data/peer-id" "$OUT")"
check "the refusal says what such a file may and may not do" \
  "$(b contains 'ordinary text file' "$OUT")"
check "the secret it was about to destroy is still there" \
  "$(b bash -c 'test "$(cat "$1")" = "$2"' _ "$J4/peer-secret.txt" "$J4_SECRET")"
check "and nothing was enrolled" "$(b test "$(j_requests)" = "$J4_COUNT")"

# J5 - the secret of a world the install record itself names may be replaced;
# a join string for a DIFFERENT identity is a slot handover or a mistake, and
# nothing here can tell which, so it is gated either way.
J5="$J_ROOT/handover"
J5_RELAY='wss://private.example.test/contract-b/v4'
J5_SECRET_ONE="$(head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n')"
J5_SECRET_TWO="$(head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n')"
J5_SECRET_THREE="$(head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n')"
printf 'multiverse-join/1 %s priv-j5.%s\n' "$J5_RELAY" "$J5_SECRET_ONE" > "$J_ROOT/j5-join.txt"
j_install --data-root "$J5" --join-string-file "$J_ROOT/j5-join.txt"
check "the private-map install succeeded" "$(b test "$RC" -eq 0)" "$OUT"
check "it wrote the world's own map beside its journal" \
  "$(b bash -c 'test "$(cat "$1")" = "$2"' _ "$J5/data/relay-url" "$J5_RELAY")"

printf 'multiverse-join/1 %s priv-j5.%s\n' "$J5_RELAY" "$J5_SECRET_TWO" > "$J_ROOT/j5-rekey.txt"
j_install --data-root "$J5" --join-string-file "$J_ROOT/j5-rekey.txt"
check "a new secret for the world the record names is applied" "$(b test "$RC" -eq 0)" "$OUT"
check "it says it is the same world" "$(b contains 'the same world, priv-j5' "$OUT")"
check "the new secret is in place" \
  "$(b bash -c 'test "$(cat "$1")" = "$2"' _ "$J5/peer-secret.txt" "$J5_SECRET_TWO")"
check "the replaced secret is kept beside it rather than destroyed" \
  "$(b bash -c 'ls "$1"/peer-secret.txt.*.old >/dev/null 2>&1' _ "$J5")"
check "and the kept copy is the secret it replaced" \
  "$(b bash -c 'grep -qxF "$2" "$1"/peer-secret.txt.*.old' _ "$J5" "$J5_SECRET_ONE")"
check "the kept copy is mode 0600" \
  "$(b bash -c 'test "$(stat -c %a "$1"/peer-secret.txt.*.old)" = 600' _ "$J5")"
rm -f "$J5"/peer-secret.txt.*.old

# A handover names a NEW identity (contract-b-m4.md 7.5), so this is both the
# recovery path and the shape of a mistake. Refused until it is asked for.
printf 'multiverse-join/1 %s priv-j5b.%s\n' "$J5_RELAY" "$J5_SECRET_THREE" > "$J_ROOT/j5-handover.txt"
j_install --data-root "$J5" --join-string-file "$J_ROOT/j5-handover.txt"
check "a join string for another identity stops with INS-ENROLL" \
  "$(b bash -c 'test "$1" -eq 1 && case "$2" in *INS-ENROLL*) exit 0 ;; *) exit 1 ;; esac' _ "$RC" "$OUT")" "$OUT"
check "the refusal names both worlds" \
  "$(b bash -c 'case "$1" in *priv-j5b*) case "$1" in *"is the world priv-j5 "*) exit 0 ;; esac ;; esac; exit 1' _ "$OUT")" "$OUT"
check "the refusal names the switch a handover needs" \
  "$(b contains '--replace-world-identity' "$OUT")"
check "the world it would have replaced still has its own secret" \
  "$(b bash -c 'test "$(cat "$1")" = "$2"' _ "$J5/peer-secret.txt" "$J5_SECRET_TWO")"

j_install --data-root "$J5" --join-string-file "$J_ROOT/j5-handover.txt" --replace-world-identity
check "the switch applies the handover" "$(b test "$RC" -eq 0)" "$OUT"
check "the new identity is the world's" \
  "$(b grep -qF '"peerId": "priv-j5b"' "$J5/install-record.json")"
check "the name it used to answer to is kept for its operator" \
  "$(b bash -c 'test "$(cat "$1")" = "priv-j5"' _ "$J5/data/peer-id.previous")"
check "the secret it replaced is kept too" \
  "$(b bash -c 'grep -qxF "$2" "$1"/peer-secret.txt.*.old' _ "$J5" "$J5_SECRET_TWO")"
check "and the change of identity is stated at the end" \
  "$(b contains "CHANGED IDENTITY" "$OUT")"
rm -f "$J5"/peer-secret.txt.*.old

# J6 - a PRIVATE world survives an uninstall whole: the uninstall keeps the name
# and the map beside the journal, so the next install adopts both.
run_script bash "$UNINSTALLER" --data-root "$J5"
check "the private-map uninstall succeeded" "$(b test "$RC" -eq 0)" "$OUT"
check "it kept the world's map" "$(b test -f "$J5/data/relay-url")"
J6_COUNT="$(j_requests)"
j_install --data-root "$J5"
check "installing again with no join string adopts the private world" "$(b test "$RC" -eq 0)" "$OUT"
check "it names the map that world is on" "$(b contains "$J5_RELAY" "$OUT")"
check "it adopts the identity the handover gave it" \
  "$(b grep -qF '"peerId": "priv-j5b"' "$J5/install-record.json")"
check "it did not fall through to the public map" \
  "$(b bash -c 'grep -qF "\"relayUrl\": \"$2\"" "$1"' _ "$J5/install-record.json" "$J5_RELAY")"
check "and it asked the public map for nothing" "$(b test "$(j_requests)" = "$J6_COUNT")"
check "the certificate step follows the map this world dials" \
  "$(b bash -c 'case "$1" in *"which is not the map this package ships"*) exit 0 ;; *) exit 1 ;; esac' _ "$OUT")" "$OUT"

# J7 - --relay-url names the map a world is on; it does not move one.
j_install --data-root "$J5" --relay-url 'wss://two.example.test/contract-b/v4'
check "--relay-url at another map stops with INS-ENROLL" \
  "$(b bash -c 'test "$1" -eq 1 && case "$2" in *INS-ENROLL*) exit 0 ;; *) exit 1 ;; esac' _ "$RC" "$OUT")" "$OUT"
check "the refusal names both maps" \
  "$(b bash -c 'case "$1" in *"$2"*) case "$1" in *"$3"*) exit 0 ;; esac ;; esac; exit 1' _ "$OUT" "$J5_RELAY" 'wss://two.example.test/contract-b/v4')" "$OUT"
check "the world still dials its own map" \
  "$(b bash -c 'grep -qF "\"relayUrl\": \"$2\"" "$1"' _ "$J5/install-record.json" "$J5_RELAY")"

# J8 - adopting re-applies the credential's mode in place. Up to that release every
# install rewrote the file and re-tightened it; adopting must not lose that.
chmod 644 "$J5/peer-secret.txt"
J8_SECRET="$(cat "$J5/peer-secret.txt")"
j_install --data-root "$J5"
check "the repair over a loose credential succeeded" "$(b test "$RC" -eq 0)" "$OUT"
check "adopting tightened its mode again" \
  "$(b test "$(stat -c '%a' "$J5/peer-secret.txt")" = 600)"
check "without touching what is in it" \
  "$(b bash -c 'test "$(cat "$1")" = "$2"' _ "$J5/peer-secret.txt" "$J8_SECRET")"

# ---------------------------------------------------------------- K

scenario "K - the sidecar's own log as the last witness a data root keeps"

K_ROOT="$SANDBOX/K"; K_GAME="$K_ROOT/game"; K_REQUESTS="$K_ROOT/requests.jsonl"
K_ARGS="$K_ROOT/curl-args.txt"; K_STATE="$K_ROOT/unused.state"
new_sandbox_game "$K_GAME"
mkdir -p "$K_ROOT"
: > "$K_REQUESTS"
K_PUBLIC_RELAY='wss://bibitesmultiverse.com/contract-b/v4'
k_install() { # $@ -> installer arguments, with the fake enrollment endpoint on PATH
  run_script env PATH="$I_FAKE_BIN:$PATH" \
    FAKE_ENROLL_REQUESTS="$K_REQUESTS" FAKE_ENROLL_STATE="$K_STATE" \
    FAKE_ENROLL_ARGS="$K_ARGS" FAKE_ENROLL_FAIL_FIRST=0 \
    FAKE_ENROLL_RELAY_URL="$K_PUBLIC_RELAY" \
    bash "$INSTALLER" --game-dir "$K_GAME" "$@"
}
k_requests() { wc -l < "$K_REQUESTS" | tr -d ' '; }
# The lines the sidecar really writes: slog's text format, one key=value per
# attribute, with peer= on every line because the identity is an attribute of
# the logger itself (go/internal/sidecar/sidecar.go).
k_write_log() { # $1 file, $2 peer, $3 relay
  mkdir -p "$(dirname "$1")"
  {
    printf 'time=2026-08-16T12:00:00.000Z level=WARN msg="sidecar: no peer credential configured"\n'
    printf 'time=2026-08-16T12:00:00.100Z level=INFO msg="sidecar: listening" peer=%s addr=127.0.0.1:8787 path=/contract-a/v2 relay=%s dataDir=%s preferredSlot=0 relayCredential=configured\n' \
      "$2" "$3" "$(dirname "$1")"
    printf 'time=2026-08-16T12:00:02.000Z level=INFO msg="contract B: slot granted" peer=%s slot=3 position=1,0 reason=granted map=m slotCount=5 lanes="E->4 N->2"\n' "$2"
  } >> "$1"
}

# K1 - one world in the log, and nothing else left in the folder at all. This is
# the state a pre-profiles kit unpacked somewhere else leaves behind.
K1="$K_ROOT/one-world"
K1_PEER='public-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'
K1_SECRET="$(printf 'a%.0s' $(seq 1 64))"
mkdir -p "$K1"
printf '%s\n' "$K1_SECRET" > "$K1/peer-secret.txt"
chmod 600 "$K1/peer-secret.txt"
K1_BEFORE="$(sha256sum "$K1/peer-secret.txt" | cut -d' ' -f1)"
k_write_log "$K1/logs/sidecar.log" "$K1_PEER" "$K_PUBLIC_RELAY"
K1_COUNT="$(k_requests)"
k_install --data-root "$K1"
check "one identity in the sidecar log is adopted" "$(b test "$RC" -eq 0)" "$OUT"
check "it names the log line it read" \
  "$(b contains 'sidecar.log ("sidecar: listening")' "$OUT")"
check "it adopts that world" "$(b contains "peer $K1_PEER" "$OUT")"
check "it spends no identity on it" "$(b test "$(k_requests)" = "$K1_COUNT")"
check "the credential is byte-identical afterwards" \
  "$(b bash -c 'test "$(sha256sum "$1" | cut -d" " -f1)" = "$2"' _ "$K1/peer-secret.txt" "$K1_BEFORE")"
check "the record names the world the log named" \
  "$(b grep -qF "\"peerId\": \"$K1_PEER\"" "$K1/install-record.json")"
check "and the map that log line named" \
  "$(b grep -qF "\"relayUrl\": \"$K_PUBLIC_RELAY\"" "$K1/install-record.json")"
check "the name is written beside the journal, where a later install finds it first" \
  "$(b bash -c 'test "$(cat "$1")" = "$2"' _ "$K1/data/peer-id" "$K1_PEER")"

# K2 - a PRIVATE world's relay comes out of the same line.
K2="$K_ROOT/private-world"
K2_PEER='priv-logged'
K2_RELAY='wss://private.example.test/contract-b/v4'
mkdir -p "$K2"
printf '%s\n' "$(printf 'b%.0s' $(seq 1 64))" > "$K2/peer-secret.txt"
chmod 600 "$K2/peer-secret.txt"
k_write_log "$K2/logs/sidecar.log" "$K2_PEER" "$K2_RELAY"
K2_COUNT="$(k_requests)"
k_install --data-root "$K2"
check "a private world in the log is adopted with its own map" "$(b test "$RC" -eq 0)" "$OUT"
check "it says which map that is" "$(b contains "$K2_RELAY" "$OUT")"
check "the record carries that map" \
  "$(b grep -qF "\"relayUrl\": \"$K2_RELAY\"" "$K2/install-record.json")"
check "it asked the public map for nothing" "$(b test "$(k_requests)" = "$K2_COUNT")"

# K3 - two worlds in the logs: the installer will not choose, and says which two.
K3="$K_ROOT/two-worlds"
K3_PEER_A='public-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb'
K3_PEER_B='public-cccccccccccccccccccccccccccccccc'
K3_SECRET="$(printf 'c%.0s' $(seq 1 64))"
mkdir -p "$K3"
printf '%s\n' "$K3_SECRET" > "$K3/peer-secret.txt"
chmod 600 "$K3/peer-secret.txt"
k_write_log "$K3/logs/sidecar.log.1" "$K3_PEER_A" "$K_PUBLIC_RELAY"
k_write_log "$K3/logs/sidecar.log" "$K3_PEER_B" "$K_PUBLIC_RELAY"
K3_COUNT="$(k_requests)"
k_install --data-root "$K3"
check "two identities in the logs stop with INS-ENROLL" \
  "$(b bash -c 'test "$1" -eq 1 && case "$2" in *INS-ENROLL*) exit 0 ;; *) exit 1 ;; esac' _ "$RC" "$OUT")" "$OUT"
check "the refusal says the installer read the log itself" \
  "$(b contains 'READ THE SIDECAR LOG' "$OUT")"
check "it lists the first world" "$(b contains "$K3_PEER_A" "$OUT")"
check "it lists the second" "$(b contains "$K3_PEER_B" "$OUT")"
check "it says where to put the one that is right" \
  "$(b contains "$K3/data/peer-id" "$OUT")"
check "it changed nothing" \
  "$(b bash -c 'test "$(cat "$1")" = "$2"' _ "$K3/peer-secret.txt" "$K3_SECRET")"
check "and asked the map for nothing" "$(b test "$(k_requests)" = "$K3_COUNT")"

# The remedy the refusal prints has to work, and it wins over the log.
printf '%s\n' "$K3_PEER_B" > "$K3/data/peer-id"
k_install --data-root "$K3"
check "naming one of them installs it" "$(b test "$RC" -eq 0)" "$OUT"
check "the named world is the one in the record" \
  "$(b grep -qF "\"peerId\": \"$K3_PEER_B\"" "$K3/install-record.json")"
check "naming it sends no enrollment request" "$(b test "$(k_requests)" = "$K3_COUNT")"

# K4 - two worlds in the logs and no secret at all: nothing here can be kept, and
# taking a new identity over their journal is still a decision.
K4="$K_ROOT/two-worlds-no-secret"
mkdir -p "$K4"
k_write_log "$K4/logs/sidecar.log.1" "$K3_PEER_A" "$K_PUBLIC_RELAY"
k_write_log "$K4/logs/sidecar.log" "$K3_PEER_B" "$K_PUBLIC_RELAY"
K4_COUNT="$(k_requests)"
k_install --data-root "$K4"
check "a folder whose logs name two worlds is not enrolled over" \
  "$(b bash -c 'test "$1" -eq 1 && case "$2" in *INS-ENROLL*) exit 0 ;; *) exit 1 ;; esac' _ "$RC" "$OUT")" "$OUT"
check "the refusal names both of them" \
  "$(b bash -c 'case "$1" in *"$2"*) case "$1" in *"$3"*) exit 0 ;; esac ;; esac; exit 1' _ "$OUT" "$K3_PEER_A" "$K3_PEER_B")" "$OUT"
check "the refusal names the switch that accepts the cost" \
  "$(b contains '--replace-world-identity' "$OUT")"
check "it enrolled nothing" "$(b test "$(k_requests)" = "$K4_COUNT")"
k_install --data-root "$K4" --replace-world-identity
check "the switch takes a new identity there" "$(b test "$RC" -eq 0)" "$OUT"
check "and it enrolled exactly once" "$(b test "$(k_requests)" = "$((K4_COUNT + 1))")"

# K5 - a kit unpacked BESIDE the data root, which is where an advanced ZIP goes.
# Its start script names this data root, and that is a stronger claim than a log.
K5="$K_ROOT/beside-the-kit/data"
mkdir -p "$K5"
K5_PEER='priv-beside'
K5_RELAY='wss://beside.example.test/contract-b/v4'
printf '%s\n' "$(printf 'd%.0s' $(seq 1 64))" > "$K5/peer-secret.txt"
chmod 600 "$K5/peer-secret.txt"
cat > "$K_ROOT/beside-the-kit/start-multiverse.sh" <<EOF
#!/usr/bin/env bash
GAME_DIR='$K_GAME'
DATA_ROOT='$K5'
RELAY_URL='$K5_RELAY'
PEER_ID='$K5_PEER'
EOF
K5_COUNT="$(k_requests)"
k_install --data-root "$K5"
check "a kit unpacked beside the data root is found" "$(b test "$RC" -eq 0)" "$OUT"
check "it adopts the world that start script names" "$(b contains "peer $K5_PEER" "$OUT")"
check "on the map that start script names" \
  "$(b grep -qF "\"relayUrl\": \"$K5_RELAY\"" "$K5/install-record.json")"
check "and asked the map for nothing" "$(b test "$(k_requests)" = "$K5_COUNT")"

# ---------------------------------------------------------------- L

scenario "L - a secret no world ever used is an orphan, not a question"

L_ROOT="$SANDBOX/L"; L_GAME="$L_ROOT/game"; L_REQUESTS="$L_ROOT/requests.jsonl"
L_ARGS="$L_ROOT/curl-args.txt"; L_STATE="$L_ROOT/unused.state"
new_sandbox_game "$L_GAME"
mkdir -p "$L_ROOT"
: > "$L_REQUESTS"
L_PUBLIC_RELAY='wss://bibitesmultiverse.com/contract-b/v4'
l_install() {
  run_script env PATH="$I_FAKE_BIN:$PATH" \
    FAKE_ENROLL_REQUESTS="$L_REQUESTS" FAKE_ENROLL_STATE="$L_STATE" \
    FAKE_ENROLL_ARGS="$L_ARGS" FAKE_ENROLL_FAIL_FIRST=0 \
    FAKE_ENROLL_RELAY_URL="$L_PUBLIC_RELAY" \
    bash "$INSTALLER" --game-dir "$L_GAME" "$@"
}
l_requests() { wc -l < "$L_REQUESTS" | tr -d ' '; }
l_orphans() { ls "$1"/peer-secret.txt.*.orphan 2>/dev/null | wc -l | tr -d ' '; }

# L1 - the reported state: an earlier install got a secret and stopped before the
# world ever ran. No sidecar has written here, so that identity holds no place on
# the map and there is nothing for a person to decide.
L1="$L_ROOT/never-ran"
L1_SECRET="$(printf 'e%.0s' $(seq 1 64))"
mkdir -p "$L1"
printf '%s\n' "$L1_SECRET" > "$L1/peer-secret.txt"
chmod 600 "$L1/peer-secret.txt"
L1_COUNT="$(l_requests)"
l_install --data-root "$L1"
check "a secret no world ever used installs rather than refusing" "$(b test "$RC" -eq 0)" "$OUT"
check "it says the secret was an orphan" "$(b contains 'was an orphan' "$OUT")"
check "it says why - the world never ran" "$(b contains 'stopped before' "$OUT")"
check "it enrolled exactly one new identity" "$(b test "$(l_requests)" = "$((L1_COUNT + 1))")"
check "the orphaned secret is KEPT, not deleted" "$(b test "$(l_orphans "$L1")" = 1)"
check "and it is the secret that was there" \
  "$(b bash -c 'grep -qxF "$2" "$1"/peer-secret.txt.*.orphan' _ "$L1" "$L1_SECRET")"
check "the kept copy is mode 0600" \
  "$(b bash -c 'test "$(stat -c %a "$1"/peer-secret.txt.*.orphan)" = 600' _ "$L1")"
check "the new credential is a different secret" \
  "$(b bash -c 'test "$(cat "$1/peer-secret.txt")" != "$2"' _ "$L1" "$L1_SECRET")"
check "the record names a new public identity" \
  "$(b grep -Eq '"peerId": "public-[0-9a-f]{32}"' "$L1/install-record.json")"

# L2 - an install log in logs/ is not a world having run: the graphical setup
# writes one there, and an empty data/ is what this installer itself creates.
L2="$L_ROOT/install-log-only"
mkdir -p "$L2/data" "$L2/logs"
printf '%s\n' "$(printf 'f%.0s' $(seq 1 64))" > "$L2/peer-secret.txt"
chmod 600 "$L2/peer-secret.txt"
printf 'setup said hello\n' > "$L2/logs/install-20260101T000000Z.log"
L2_COUNT="$(l_requests)"
l_install --data-root "$L2"
check "an empty data folder and an install log still read as an orphan" "$(b test "$RC" -eq 0)" "$OUT"
check "it enrolled exactly one new identity" "$(b test "$(l_requests)" = "$((L2_COUNT + 1))")"
check "the orphaned secret is kept" "$(b test "$(l_orphans "$L2")" = 1)"

# L3 - a journal means a sidecar ran here, so a world exists somewhere and the
# refusal stands, whatever the logs were rotated away.
L3="$L_ROOT/has-journal"
L3_SECRET="$(printf 'g%.0s' $(seq 1 64))"
mkdir -p "$L3/data/journal"
printf '%s\n' "$L3_SECRET" > "$L3/peer-secret.txt"
chmod 600 "$L3/peer-secret.txt"
printf '{"seq":1}\n' > "$L3/data/journal/journal.log"
L3_COUNT="$(l_requests)"
l_install --data-root "$L3"
check "a secret beside a journal is still refused" \
  "$(b bash -c 'test "$1" -eq 1 && case "$2" in *INS-ENROLL*) exit 0 ;; *) exit 1 ;; esac' _ "$RC" "$OUT")" "$OUT"
check "the refusal is the one about naming the world" \
  "$(b contains 'says which' "$OUT")"
check "nothing was renamed" "$(b test "$(l_orphans "$L3")" = 0)"
check "the secret is exactly as it was" \
  "$(b bash -c 'test "$(cat "$1/peer-secret.txt")" = "$2"' _ "$L3" "$L3_SECRET")"
check "and nothing was enrolled" "$(b test "$(l_requests)" = "$L3_COUNT")"

# L4 - a sidecar log that names a peer this installer cannot use is still proof
# that a sidecar ran here.
L4="$L_ROOT/unusable-peer-line"
L4_SECRET="$(printf 'h%.0s' $(seq 1 64))"
mkdir -p "$L4/logs"
printf '%s\n' "$L4_SECRET" > "$L4/peer-secret.txt"
chmod 600 "$L4/peer-secret.txt"
printf 'time=2026-01-01T00:00:00Z level=INFO msg="sidecar: listening" peer="two words" relay=%s\n' \
  "$L_PUBLIC_RELAY" > "$L4/logs/sidecar.log"
L4_COUNT="$(l_requests)"
l_install --data-root "$L4"
check "a peer line the installer cannot use still counts as a world having run" \
  "$(b bash -c 'test "$1" -eq 1 && case "$2" in *INS-ENROLL*) exit 0 ;; *) exit 1 ;; esac' _ "$RC" "$OUT")" "$OUT"
check "nothing was renamed there either" "$(b test "$(l_orphans "$L4")" = 0)"
check "and nothing was enrolled" "$(b test "$(l_requests)" = "$L4_COUNT")"

# L5 - the same rule frees the join-string path: an orphan is not a world, so a
# private map's join string is not blocked by one.
L5="$L_ROOT/orphan-then-join"
L5_SECRET="$(printf 'i%.0s' $(seq 1 64))"
L5_NEW_SECRET="$(head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n')"
mkdir -p "$L5"
printf '%s\n' "$L5_SECRET" > "$L5/peer-secret.txt"
chmod 600 "$L5/peer-secret.txt"
printf 'multiverse-join/1 wss://private.example.test/contract-b/v4 priv-l5.%s\n' "$L5_NEW_SECRET" \
  > "$L_ROOT/l5-join.txt"
L5_COUNT="$(l_requests)"
l_install --data-root "$L5" --join-string-file "$L_ROOT/l5-join.txt"
check "a join string over an orphan installs" "$(b test "$RC" -eq 0)" "$OUT"
check "the world is the one the join string names" \
  "$(b grep -qF '"peerId": "priv-l5"' "$L5/install-record.json")"
check "the join string's secret is in place" \
  "$(b bash -c 'test "$(cat "$1/peer-secret.txt")" = "$2"' _ "$L5" "$L5_NEW_SECRET")"
check "the orphan is kept beside it" "$(b test "$(l_orphans "$L5")" = 1)"
check "and no enrollment request was sent" "$(b test "$(l_requests)" = "$L5_COUNT")"

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
