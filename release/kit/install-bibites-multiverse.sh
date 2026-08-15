#!/usr/bin/env bash
#
# install-bibites-multiverse.sh
#
# Install the Bibites Multiverse mod and its sidecar on the game's NATIVE LINUX
# build. The public package enrolls this installation automatically. A private
# map uses the join string that its operator gives you.
#
# It needs no compiler, no SDK, no runtime and nothing from a developer's
# toolchain, and it needs no root. It never starts the game. It writes nothing
# outside your game folder, its own data directory and the folder it was
# unpacked into, and it touches no system trust store, no systemd unit, no
# desktop entry and no package manager.
#
# What it does, in order:
#
#   1. verifies every file in this folder against MANIFEST.sha256, then makes
#      executable the files it verified - and nothing else;
#   2. selects the game: an existing itch.io copy in the add-on edition, or the
#      verified managed payload in the complete edition;
#   3. checks your game build against support-matrix.json and STOPS if there is
#      no Linux entry for it, in the matrix's own words;
#   4. installs BepInEx linux_x64, if it is not already there;
#   5. copies the plugin into BepInEx/plugins;
#   6. creates a unique public-map identity, or splits a private-map join
#      string, and stores the secret in a file only you can read;
#   7. arranges trust for a private map's certificate authority ONLY if you
#      gave it one with --ca-file, which a public map does not need;
#   8. states the settings this install ships with, including the export
#      default;
#   9. writes start-multiverse.sh, stop-multiverse.sh and the record the
#      uninstall reads.
#
# IT WILL NEVER ASK YOU TO TURN A SECURITY CONTROL OFF. No part of this package
# prints an --insecure- flag, a curl | sh, a sudo, or an instruction to skip
# certificate verification. If any tool or page in this project asks you for one
# of those, that is the defect and reporting it is the right response.
#
# WHAT A BARE INSTALL DOES. Your world exports on ALL FOUR EDGES. Nothing
# configured means the whole perimeter, not silence. Step 8 states it again on
# your screen, with what each shipped setting costs you.
#
# WHERE THIS DIFFERS FROM THE WINDOWS INSTALLER, and why:
#
#   * There is no mark of the web on Linux. The ordering that step exists to
#     protect is kept all the same: CHECKSUM FIRST, THEN THE EXECUTABLE BIT, on
#     exactly the files the checksum covered and by name.
#   * The launcher is BepInEx's own run_bepinex.sh rather than a native loader,
#     so the start script runs ./run_bepinex.sh "./The Bibites.x86_64" and the
#     game inherits its environment from there.
#   * A private map's authority is trusted through SSL_CERT_FILE in the start
#     script - the platform's own mechanism, for that one process. NOTHING is
#     written to /etc/ssl, /usr/local/share/ca-certificates or any other store,
#     and update-ca-certificates is never run.
#   * Your saves are under $XDG_CONFIG_HOME (or ~/.config), not AppData.
#   * There is no WSLENV and no path translation anywhere in this package.
#
set -euo pipefail

RELEASE='0.2.1'
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MANIFEST_NAME='MANIFEST.sha256'
MATRIX_NAME='support-matrix.json'
PUBLIC_MAP_NAME='public-map.json'
PAYLOAD_DESCRIPTOR_NAME='game-payload.json'
PLUGIN_NAME='BibitesMultiverse.dll'
SIDECAR_NAME='multiverse-sidecar'
PLUGIN_GUID='dev.multiverse.bibites'
START_NAME='start-multiverse.sh'
STOP_NAME='stop-multiverse.sh'
RECORD_NAME='install-record.json'
UNINSTALL_NAME='uninstall-bibites-multiverse.sh'
GAME_EXE='The Bibites.x86_64'
PLATFORM='Linux'

JOIN_STRING_FILE=''
RELAY_URL=''
CA_FILE=''
GAME_DIR=''
DATA_ROOT=''
WORLD='Multiverse'
EXPORT_EDGES='E,N,W,S'
EXCLUDE_SPECIES='Basic bibite'
NO_MIGRATION_EXCLUSION=0
SIDECAR_PORT=8787
SAVE_MINUTES=10
SAVE_KEEP=6
SAVE_ON_QUIT='on'

say()  { printf '     %s\n' "$*"; }
step() { printf '\n==== %s\n' "$*"; }

stop_setup() {
  # $1 the message, $2 the taxonomy id if there is one
  printf '\n'
  if [ "${2:-}" != '' ]; then printf 'STOP [%s]: %s\n' "$2" "$1"
  else                        printf 'STOP: %s\n' "$1"; fi
  printf '\n'
  printf '     Every refusal this project can hand you is listed, with its remedy and who\n'
  printf '     has to apply it, in docs/error-taxonomy.md on the release page.\n'
  exit 1
}

sha256_of() { sha256sum "$1" | cut -d' ' -f1 | tr 'a-f' 'A-F'; }

usage() {
  cat <<USAGE
usage: ./$(basename "$0") [options]

  --join-string-file <path>  Use a private map instead of the packaged public
                             map. The first non-empty line must be the one-line
                             join string your map's operator handed you:
                               multiverse-join/1 wss://<relay-host>/contract-b/v4 <peerId>.<secret>
                             Without this option, the packaged public map enrolls
                             this installation automatically. A kit without
                             public-map.json asks at the keyboard, with hidden
                             typing.
                             THERE IS DELIBERATELY NO --join-string OPTION: a
                             secret on a command line is in every process listing
                             on this machine and in your shell history. The wire
                             itself has the same rule.
  --relay-url <wss://...>    Only if your operator gave you the two halves
                             separately. It must be wss://; this wire is always
                             encrypted and there is no plain fallback.
  --ca-file <path>           ONLY for a private or LAN map whose relay uses its
                             own certificate authority. Nothing is written to any
                             system trust store, ever - see step 7.
  --game-dir <path>          Add-on edition only: the game folder, if this script
                             does not find it. A complete edition uses its own
                             managed runtime and refuses this option.
  --data-root <path>         Where this install keeps its journal, credential,
                             logs and record. A complete edition also keeps its
                             versioned game runtime here. Default:
                             \${XDG_DATA_HOME:-\$HOME/.local/share}/bibites-multiverse
  --world <name>             The world this install runs. Default: Multiverse.
  --export-edges <list>      The edges your world exports through. THE DEFAULT IS
                             ALL FOUR and it is written out explicitly.
  --exclude-species <name>   Species that never leave your world.
                             Default: 'Basic bibite'.
  --no-migration-exclusion   Turn the exclusion policy off entirely. It takes its
                             own flag on purpose.
  --sidecar-port <n>         Default 8787.
  --save-minutes <n>         Default 10.
  --save-keep <n>            Default 6.
  --save-on-quit on|off      Default on.
  -h, --help                 This text.
USAGE
}

need_value() { [ "$2" -gt 1 ] || stop_setup "$1 needs a value."; }

while [ $# -gt 0 ]; do
  case "$1" in
    --join-string-file) need_value "$1" $#; JOIN_STRING_FILE="$2"; shift 2 ;;
    --relay-url)        need_value "$1" $#; RELAY_URL="$2";        shift 2 ;;
    --ca-file)          need_value "$1" $#; CA_FILE="$2";          shift 2 ;;
    --game-dir)         need_value "$1" $#; GAME_DIR="$2";         shift 2 ;;
    --data-root)        need_value "$1" $#; DATA_ROOT="$2";        shift 2 ;;
    --world)            need_value "$1" $#; WORLD="$2";            shift 2 ;;
    --export-edges)     need_value "$1" $#; EXPORT_EDGES="$2";     shift 2 ;;
    --exclude-species)  need_value "$1" $#; EXCLUDE_SPECIES="$2";  shift 2 ;;
    --sidecar-port)     need_value "$1" $#; SIDECAR_PORT="$2";     shift 2 ;;
    --save-minutes)     need_value "$1" $#; SAVE_MINUTES="$2";     shift 2 ;;
    --save-keep)        need_value "$1" $#; SAVE_KEEP="$2";        shift 2 ;;
    --save-on-quit)     need_value "$1" $#; SAVE_ON_QUIT="$2";     shift 2 ;;
    --no-migration-exclusion) NO_MIGRATION_EXCLUSION=1; shift ;;
    -h|--help) usage; exit 0 ;;
    --join-string|--join-string=*)
      stop_setup "There is no --join-string option, deliberately: a secret typed on a command line is in every process listing on this machine and in your shell history. Run this with no options and paste it at the hidden prompt, or use --join-string-file." ;;
    *) usage >&2; stop_setup "unknown option: $1" ;;
  esac
done

# json_flatten: the whole of this package's JSON reading, in awk, so that a bare
# machine needs no jq and no python. It prints one "path<TAB>value" line per
# scalar - entries.0.platform, plugin.sha256, bepInEx.files.7.path - which is
# the shape both the support matrix and the install record are read in.
json_flatten() {
  awk '
    function unesc(s,   out, i, c, d) {
      out = ""
      for (i = 1; i <= length(s); i++) {
        c = substr(s, i, 1)
        if (c == "\\") {
          i++; d = substr(s, i, 1)
          if      (d == "n") out = out "\n"
          else if (d == "t") out = out "\t"
          else if (d == "r") out = out "\r"
          else if (d == "u") { out = out "?"; i += 4 }
          else               out = out d
        } else out = out c
      }
      return out
    }
    function ws() { while (P <= L && index(" \t\r\n", substr(B, P, 1)) > 0) P++ }
    function readstr(   s, c) {
      P++; s = ""
      while (P <= L) {
        c = substr(B, P, 1)
        if (c == "\\") { s = s substr(B, P, 2); P += 2; continue }
        if (c == "\"") { P++; break }
        s = s c; P++
      }
      return s
    }
    function parse(prefix,   c, k, n, s) {
      ws()
      if (P > L) return
      c = substr(B, P, 1)
      if (c == "{") {
        P++; ws()
        if (substr(B, P, 1) == "}") { P++; return }
        while (P <= L) {
          ws(); k = readstr(); ws(); P++          # the colon
          parse(prefix == "" ? k : prefix "." k)
          ws(); c = substr(B, P, 1); P++
          if (c == "}" || c == "") return
        }
        return
      }
      if (c == "[") {
        P++; n = 0; ws()
        if (substr(B, P, 1) == "]") { P++; return }
        while (P <= L) {
          parse(prefix "." n); n++
          ws(); c = substr(B, P, 1); P++
          if (c == "]" || c == "") return
        }
        return
      }
      if (c == "\"") { printf "%s\t%s\n", prefix, unesc(readstr()); return }
      s = ""
      while (P <= L && index(",}] \t\r\n", substr(B, P, 1)) == 0) { s = s substr(B, P, 1); P++ }
      printf "%s\t%s\n", prefix, s
    }
    { B = B $0 "\n" }
    END { L = length(B); P = 1; parse("") }
  ' "$1"
}

flat_get() {
  # $1 the flattened text, $2 the path. Empty output means absent.
  printf '%s\n' "$1" | awk -v k="$2" 'index($0, k "\t") == 1 { print substr($0, length(k) + 2); exit }'
}

# ---------------------------------------------------------------- 0. the tools

step "0 of 9 - the programs this needs"

# Named rather than assumed, because "it needs no toolchain" is a promise this
# package makes and a promise is worth checking. Three of these are on any Linux
# that can unpack a zip; the fourth is BepInEx's, not this installer's.
MISSING=''
for tool in sha256sum awk unzip file curl; do
  if command -v "$tool" >/dev/null 2>&1; then
    say "$tool: $(command -v "$tool")"
  else
    MISSING="$MISSING $tool"
  fi
done
if [ -n "$MISSING" ]; then
  printf '\n'
  say "missing:$MISSING"
  say "sha256sum is coreutils and awk is mawk; both are on every Debian or Ubuntu"
  say "already. unzip unpacks BepInEx. 'file' is used by BepInEx's OWN launcher,"
  say "run_bepinex.sh, to check the game binary's architecture - without it the game"
  say "will not start even though this install would look complete, so it is checked"
  say "here, where nothing has been installed yet. curl sends one HTTPS enrollment"
  say "request when this package connects to the public map."
  say "On Debian or Ubuntu:   sudo apt install unzip file curl"
  stop_setup "this machine is missing a program this package needs. NOTHING was installed." 'INS-LINUXDEPS'
fi

# ---------------------------------------------------------------- 1. the kit

step "1 of 9 - check this package against its own manifest"

MANIFEST="$HERE/$MANIFEST_NAME"
[ -f "$MANIFEST" ] || stop_setup \
  "$MANIFEST_NAME is missing, so nothing here can be checked. Unpack the release archive again, whole." \
  'INS-CHECKSUM'

VERIFIED=''
VERIFIED_COUNT=0
while IFS= read -r line; do
  line="$(printf '%s' "$line" | tr -d '\r')"
  case "$line" in ''|'#'*) continue ;; esac
  want="$(printf '%s' "$line" | awk '{print toupper($1)}')"
  rel="$(printf '%s' "$line" | sed -e 's/^[0-9A-Fa-f]\{64\}[[:space:]]*\*\{0,1\}//')"
  case "$want" in *[!0-9A-F]*)
    stop_setup "$MANIFEST_NAME has a line this installer cannot read: $line" 'INS-CHECKSUM' ;;
  esac
  [ "${#want}" -eq 64 ] || stop_setup "$MANIFEST_NAME has a line this installer cannot read: $line" 'INS-CHECKSUM'
  [ -n "$rel" ] || stop_setup "$MANIFEST_NAME has a line this installer cannot read: $line" 'INS-CHECKSUM'
  file="$HERE/$rel"
  [ -f "$file" ] || stop_setup \
    "$rel is named in $MANIFEST_NAME and is not in this folder. The package is incomplete; unpack the release archive again, whole." \
    'INS-CHECKSUM'
  got="$(sha256_of "$file")"
  if [ "$got" != "$want" ]; then
    printf '\n%s is not the published file.\n' "$rel"
    say "expected SHA-256: $want"
    say "this copy       : $got"
    stop_setup "Delete the whole folder and the archive, download the archive again from the release page, and check the archive's own checksum against the page BEFORE unpacking it. If it fails twice, report it and do not run it." \
      'INS-CHECKSUM'
  fi
  VERIFIED="$VERIFIED$rel
"
  VERIFIED_COUNT=$((VERIFIED_COUNT + 1))
done < "$MANIFEST"
say "$VERIFIED_COUNT files match $MANIFEST_NAME."

# THE ORDER IS THE POINT, and it is the same point the Windows package makes
# about the mark of the web. Linux has no mark of the web - nothing here arrives
# quarantined and no control has to be cleared. What it has instead is a
# permission bit that a zip may or may not have carried, and the honest moment to
# set it is AFTER the checksum has passed, on exactly the files the checksum
# covered, by name, and on nothing else in this folder or anywhere on this
# machine.
verified_has() { case "
$VERIFIED" in *"
$1
"*) return 0 ;; esac; return 1; }

MADE_EXECUTABLE=0
for rel in "$UNINSTALL_NAME" "$(basename "${BASH_SOURCE[0]}")" "$SIDECAR_NAME"; do
  verified_has "$rel" || continue
  if [ ! -x "$HERE/$rel" ]; then
    chmod +x "$HERE/$rel"
    MADE_EXECUTABLE=$((MADE_EXECUTABLE + 1))
  fi
done
if [ "$MADE_EXECUTABLE" -gt 0 ]; then
  say "made $MADE_EXECUTABLE of them executable, by name, after checking each one."
else
  say "all of them already carry the executable bit they need; nothing to change."
fi
say "Nothing else in this folder was touched, and nothing outside it."

# ---------------------------------------------------------------- 2. the game

step "2 of 9 - select The Bibites runtime"

# The installer has one code path and two package editions. A complete package
# contains game-payload.json and a manifest-covered game/ tree; an add-on
# package contains neither and binds to an existing copy. The descriptor is not
# a switch a player passes, so the archive they checked is the whole choice.
RUNTIME_MODE='external'
RUNTIME_ROOT=''
RUNTIME_FILES=''
PAYLOAD_DESCRIPTOR="$HERE/$PAYLOAD_DESCRIPTOR_NAME"

if [ -f "$PAYLOAD_DESCRIPTOR" ]; then
  [ -z "$GAME_DIR" ] || stop_setup \
    "This is a complete package and installs its own managed game runtime. Do not pass --game-dir; use the add-on package to bind an existing copy." 'INS-RUNTIME'

  PAYLOAD_FLAT="$(json_flatten "$PAYLOAD_DESCRIPTOR")"
  [ "$(flat_get "$PAYLOAD_FLAT" format)" = 'bibites-multiverse/game-payload/1' ] || \
    stop_setup "$PAYLOAD_DESCRIPTOR_NAME has an unsupported format." 'INS-CHECKSUM'
  [ "$(flat_get "$PAYLOAD_FLAT" platform)" = "$PLATFORM" ] || \
    stop_setup "This complete package carries a game payload for $(flat_get "$PAYLOAD_FLAT" platform), not $PLATFORM." 'INS-GAMEBUILD'

  PAYLOAD_SHA="$(flat_get "$PAYLOAD_FLAT" assemblySha256 | tr 'a-f' 'A-F')"
  case "$PAYLOAD_SHA" in ''|*[!0-9A-F]*) stop_setup "$PAYLOAD_DESCRIPTOR_NAME has an invalid assemblySha256." 'INS-CHECKSUM' ;; esac
  [ "${#PAYLOAD_SHA}" -eq 64 ] || stop_setup "$PAYLOAD_DESCRIPTOR_NAME has an invalid assemblySha256." 'INS-CHECKSUM'
  PAYLOAD_GAME_DIR="$HERE/game"
  PAYLOAD_NOTICE="$(flat_get "$PAYLOAD_FLAT" redistributionNoticeFile)"
  [ -d "$PAYLOAD_GAME_DIR" ] || stop_setup "The complete package is missing its game/ directory." 'INS-CHECKSUM'
  [ -n "$PAYLOAD_NOTICE" ] && [ -f "$HERE/$PAYLOAD_NOTICE" ] || \
    stop_setup "The complete package is missing the redistribution notice named by $PAYLOAD_DESCRIPTOR_NAME." 'INS-CHECKSUM'
  [ -f "$PAYLOAD_GAME_DIR/$GAME_EXE" ] || stop_setup "The complete package has no '$GAME_EXE' in game/." 'INS-CHECKSUM'
  [ -x "$PAYLOAD_GAME_DIR/$GAME_EXE" ] || stop_setup "The complete package's '$GAME_EXE' is not executable. Unpack the archive again with permissions preserved." 'INS-CHECKSUM'
  PAYLOAD_ASSEMBLY="$PAYLOAD_GAME_DIR/The Bibites_Data/Managed/BibitesAssembly.dll"
  [ -f "$PAYLOAD_ASSEMBLY" ] || stop_setup "The complete package is missing BibitesAssembly.dll." 'INS-CHECKSUM'
  [ "$(sha256_of "$PAYLOAD_ASSEMBLY")" = "$PAYLOAD_SHA" ] || \
    stop_setup "$PAYLOAD_DESCRIPTOR_NAME does not describe the game assembly in this package." 'INS-CHECKSUM'
  if find "$PAYLOAD_GAME_DIR" -type l -print -quit | grep -q .; then
    stop_setup "The complete package contains a symbolic link. Game payloads must contain regular files and directories only." 'INS-CHECKSUM'
  fi

  # Refuse an unsupported payload before copying anything. Step 3 repeats the
  # normal matrix check against the installed runtime, keeping both editions on
  # the same gate after selection.
  PAYLOAD_MATRIX_FLAT="$(json_flatten "$HERE/$MATRIX_NAME")"
  PAYLOAD_SUPPORTED=0
  payload_i=0
  while :; do
    payload_version="$(flat_get "$PAYLOAD_MATRIX_FLAT" "entries.$payload_i.gameVersion")"
    [ -n "$payload_version" ] || break
    if [ "$(flat_get "$PAYLOAD_MATRIX_FLAT" "entries.$payload_i.platform")" = "$PLATFORM" ] &&
       [ "$(flat_get "$PAYLOAD_MATRIX_FLAT" "entries.$payload_i.assemblySha256" | tr 'a-f' 'A-F')" = "$PAYLOAD_SHA" ]; then
      PAYLOAD_SUPPORTED=1
      break
    fi
    payload_i=$((payload_i + 1))
  done
  [ "$PAYLOAD_SUPPORTED" -eq 1 ] || stop_setup \
    "The game payload in this complete package is not in this release's support matrix. Nothing was installed." 'INS-GAMEBUILD'

  if [ -z "$DATA_ROOT" ]; then DATA_ROOT="${XDG_DATA_HOME:-$HOME/.local/share}/bibites-multiverse"; fi
  mkdir -p "$DATA_ROOT"
  DATA_ROOT="$(cd "$DATA_ROOT" && pwd)"
  case "$DATA_ROOT" in
    *"'"*) stop_setup "The data directory's path contains a single quote, which the generated start script cannot carry safely. Pass --data-root with a path without one." ;;
  esac

  RUNTIME_MODE='bundled'
  RUNTIME_ROOT="$DATA_ROOT/runtimes/$PAYLOAD_SHA"
  RUNTIME_FILES="$(cd "$PAYLOAD_GAME_DIR" && find . -type f -printf '%P\n' | LC_ALL=C sort)"
  [ -n "$RUNTIME_FILES" ] || stop_setup "The complete package's game/ directory contains no files." 'INS-CHECKSUM'

  if [ -d "$RUNTIME_ROOT" ]; then
    while IFS= read -r rel; do
      [ -n "$rel" ] || continue
      [ -f "$RUNTIME_ROOT/$rel" ] || stop_setup \
        "The managed runtime at $RUNTIME_ROOT is incomplete ($rel is missing). It was not overwritten." 'INS-RUNTIME'
      [ "$(sha256_of "$RUNTIME_ROOT/$rel")" = "$(sha256_of "$PAYLOAD_GAME_DIR/$rel")" ] || stop_setup \
        "The managed runtime at $RUNTIME_ROOT was changed ($rel differs). It was not overwritten." 'INS-RUNTIME'
    done <<< "$RUNTIME_FILES"
    say "complete edition: reusing the verified managed game runtime"
  else
    mkdir -p "$(dirname "$RUNTIME_ROOT")"
    RUNTIME_TEMP="$(mktemp -d "$(dirname "$RUNTIME_ROOT")/.installing.XXXXXXXX")"
    trap 'rm -rf "$RUNTIME_TEMP"' EXIT
    cp -a "$PAYLOAD_GAME_DIR/." "$RUNTIME_TEMP/"
    [ "$(sha256_of "$RUNTIME_TEMP/The Bibites_Data/Managed/BibitesAssembly.dll")" = "$PAYLOAD_SHA" ] || \
      stop_setup "The managed runtime copy failed verification." 'INS-RUNTIME'
    mv "$RUNTIME_TEMP" "$RUNTIME_ROOT"
    trap - EXIT
    say "complete edition: installed the verified game payload into a managed runtime"
  fi
  say "game redistribution notice: $HERE/$PAYLOAD_NOTICE"
  GAME_DIR="$RUNTIME_ROOT"
else
  say "add-on edition: binding to an existing game installation"
fi

game_processes_in() {
  # The check is on the game folder this install writes to rather than on a
  # process name: one machine may hold more than one copy of the game, and only
  # the copy being written to has to be closed.
  #
  # Linux does NOT hold the plugin file open the way Windows does, so this is not
  # a refusal the platform forces. It is one this installer keeps anyway:
  # replacing a file the running game has already mapped into memory is a way to
  # crash a world in the middle of a save.
  #
  # Processes this account cannot inspect belong to another user, and a game
  # another user is running out of your game folder is not a case this script can
  # see. It says so rather than pretending otherwise.
  local dir="$1" pid exe
  for pid in /proc/[0-9]*; do
    exe="$(readlink "$pid/exe" 2>/dev/null || true)"
    [ -n "$exe" ] || continue
    case "$exe" in "$dir"/*) printf '%s (pid %s)\n' "$exe" "${pid##*/}" ;; esac
  done
  return 0
}

find_game_dir() {
  local root candidate
  for candidate in \
    "${XDG_CONFIG_HOME:-$HOME/.config}/itch/apps/the-bibites" \
    "${XDG_CONFIG_HOME:-$HOME/.config}/itch/apps/The Bibites" \
    "${XDG_DATA_HOME:-$HOME/.local/share}/itch/apps/the-bibites" \
    "$HOME/Games/the-bibites" "$HOME/Games/The Bibites" \
    "$HOME/games/the-bibites" "$HOME/games/The Bibites" \
    "$HOME/The Bibites" "$HOME/TheBibites" \
    "$HOME/Downloads/The Bibites" "$HOME/Downloads/TheBibites"
  do
    [ -f "$candidate/$GAME_EXE" ] && { printf '%s' "$candidate"; return 0; }
  done
  # The itch app lets you add install locations, so its own roots are searched one
  # level down as well. This is bounded on purpose: it is two directory reads, not
  # a walk of your home directory.
  for root in "${XDG_CONFIG_HOME:-$HOME/.config}/itch/apps" "$HOME/Games" "$HOME/games"; do
    [ -d "$root" ] || continue
    for candidate in "$root"/*; do
      [ -f "$candidate/$GAME_EXE" ] && { printf '%s' "$candidate"; return 0; }
    done
  done
  return 1
}

if [ -z "$GAME_DIR" ]; then
  GAME_DIR="$(find_game_dir || true)"
fi
if [ -z "$GAME_DIR" ] && [ -t 0 ]; then
  printf '\n'
  say "This script could not find The Bibites, and on Linux there is no registry and"
  say "no library index to ask - the game is wherever you unpacked the itch.io"
  say "archive. Type the folder that holds '$GAME_EXE', or press Enter to stop."
  printf '\n'
  printf '     game folder: '
  IFS= read -r GAME_DIR || GAME_DIR=''
  GAME_DIR="${GAME_DIR%/}"
fi
[ -n "$GAME_DIR" ] || stop_setup \
  "No copy of The Bibites was found, and none was given. Unpack the itch.io Linux archive somewhere, or run this script again with --game-dir '/path/to/The Bibites'."
[ -d "$GAME_DIR" ] || stop_setup "There is no folder at $GAME_DIR."
[ -f "$GAME_DIR/$GAME_EXE" ] || stop_setup "There is no '$GAME_EXE' in $GAME_DIR."
GAME_DIR="$(cd "$GAME_DIR" && pwd)"
case "$GAME_DIR" in
  *"'"*) stop_setup "The game folder's path contains a single quote, which the generated start script cannot carry safely. Move the game somewhere without one." ;;
esac
say "game directory: $GAME_DIR"

RUNNING="$(game_processes_in "$GAME_DIR")"
if [ -n "$RUNNING" ]; then
  printf '%s\n' "$RUNNING" | while IFS= read -r line; do say "running: $line"; done
  stop_setup "The Bibites is running from that folder. Close it, then run this script again - overwriting a file the running game has mapped into memory can crash a world mid-save. Nothing was installed."
fi

# ---------------------------------------------------------------- 3. the matrix

step "3 of 9 - check this game build against the support matrix"

MATRIX="$HERE/$MATRIX_NAME"
[ -f "$MATRIX" ] || stop_setup "The package is incomplete: $MATRIX_NAME is missing." 'INS-CHECKSUM'
MATRIX_FLAT="$(json_flatten "$MATRIX")"
[ -n "$MATRIX_FLAT" ] || stop_setup "$MATRIX_NAME could not be read." 'INS-CHECKSUM'
REFUSAL="$(flat_get "$MATRIX_FLAT" refusal)"

ASSEMBLY="$GAME_DIR/The Bibites_Data/Managed/BibitesAssembly.dll"
[ -f "$ASSEMBLY" ] || stop_setup "The game assembly is missing: $ASSEMBLY"
ASSEMBLY_SHA="$(sha256_of "$ASSEMBLY")"

# A matrix row is keyed on (game version, PLATFORM): one game version has one
# file per platform and they do not share a hash. This is the Linux installer, so
# it looks up the Linux rows and nothing else - a Windows hash here is
# INS-GAMEBUILD rather than a confusing match.
ENTRY=''
i=0
while :; do
  gv="$(flat_get "$MATRIX_FLAT" "entries.$i.gameVersion")"
  [ -n "$gv" ] || break
  if [ "$(flat_get "$MATRIX_FLAT" "entries.$i.platform")" = "$PLATFORM" ] &&
     [ "$(flat_get "$MATRIX_FLAT" "entries.$i.assemblySha256" | tr 'a-f' 'A-F')" = "$ASSEMBLY_SHA" ]; then
    ENTRY="$i"; break
  fi
  i=$((i + 1))
done

if [ -z "$ENTRY" ]; then
  printf '\nThe game on this machine is not a build this release supports.\n\n'
  # The matrix's own words, quoted from support-matrix.json rather than written
  # here, so the page a reader looks the refusal up on and the tool that refuses
  # cannot drift apart.
  printf '%s' "$REFUSAL" | sed -e 's/\. /.\n/g' | while IFS= read -r chunk; do say "$chunk"; done
  printf '\n'
  say "this machine's platform           : $PLATFORM"
  say "this machine's BibitesAssembly.dll: $ASSEMBLY_SHA"
  say "the builds this release supports:"
  i=0
  while :; do
    gv="$(flat_get "$MATRIX_FLAT" "entries.$i.gameVersion")"
    [ -n "$gv" ] || break
    say "  game $gv  $(flat_get "$MATRIX_FLAT" "entries.$i.platform")/$(flat_get "$MATRIX_FLAT" "entries.$i.store") ($(flat_get "$MATRIX_FLAT" "entries.$i.storeBuild"))  mod $(flat_get "$MATRIX_FLAT" "entries.$i.mod"), sidecar $(flat_get "$MATRIX_FLAT" "entries.$i.sidecar")"
    say "      SHA-256 $(flat_get "$MATRIX_FLAT" "entries.$i.assemblySha256")"
    i=$((i + 1))
  done
  printf '\n'
  say "A row is a game build AND a platform. If one of the rows above matches your hash on"
  say "another platform, that is the release archive to download rather than this one."
  say "The full matrix, and what a map with two game builds on it does, is in"
  say "docs/support-matrix.md on the release page."
  stop_setup "the game build is not in the support matrix; NOTHING was installed." 'INS-GAMEBUILD'
fi

E_GAMEVERSION="$(flat_get "$MATRIX_FLAT" "entries.$ENTRY.gameVersion")"
E_STORE="$(flat_get "$MATRIX_FLAT" "entries.$ENTRY.store")"
E_STOREBUILD="$(flat_get "$MATRIX_FLAT" "entries.$ENTRY.storeBuild")"
E_MOD="$(flat_get "$MATRIX_FLAT" "entries.$ENTRY.mod")"
E_SIDECAR="$(flat_get "$MATRIX_FLAT" "entries.$ENTRY.sidecar")"
E_BEPINEX="$(flat_get "$MATRIX_FLAT" "entries.$ENTRY.bepInEx")"
E_FLAVOUR="$(flat_get "$MATRIX_FLAT" "entries.$ENTRY.bepInExFlavour")"
E_CONTRACT_A="$(flat_get "$MATRIX_FLAT" "entries.$ENTRY.contractA")"
E_CONTRACT_B="$(flat_get "$MATRIX_FLAT" "entries.$ENTRY.contractB")"
E_TESTED="$(flat_get "$MATRIX_FLAT" "entries.$ENTRY.tested")"

say "game version $E_GAMEVERSION on $PLATFORM/$E_STORE ($E_STOREBUILD) - supported by this release"
say "this release: mod $E_MOD, sidecar $E_SIDECAR, wire $E_CONTRACT_B and $E_CONTRACT_A"
say "tested against: $E_TESTED"

# ---------------------------------------------------------------- 4. BepInEx

step "4 of 9 - BepInEx $E_BEPINEX $E_FLAVOUR"

BEPINEX_ZIP_NAME="BepInEx_${E_FLAVOUR}_${E_BEPINEX}.zip"
BEPINEX_CORE="$GAME_DIR/BepInEx/core/BepInEx.dll"
BEPINEX_OURS='false'
BEPINEX_PATHS=''
BEPINEX_COUNT=0

if [ -f "$BEPINEX_CORE" ]; then
  say "BepInEx is already installed here; left exactly as it is."
  say "The uninstall will not touch it either - it removes only what it put there."
else
  ZIP="$HERE/$BEPINEX_ZIP_NAME"
  [ -f "$ZIP" ] || stop_setup "The package is incomplete: $BEPINEX_ZIP_NAME is missing." 'INS-CHECKSUM'

  # Every path the archive holds, recorded BEFORE it is unpacked, so the uninstall
  # removes those files and nothing that was already here. On Linux the archive
  # lays four files in the game's own root - run_bepinex.sh, libdoorstop.so,
  # .doorstop_version and BepInEx's changelog.txt - which is where the Windows
  # build puts winhttp.dll instead.
  while IFS= read -r rel; do
    case "$rel" in ''|*/) continue ;; esac
    if [ -e "$GAME_DIR/$rel" ]; then
      say "left alone (already here): $rel"
      continue
    fi
    BEPINEX_PATHS="$BEPINEX_PATHS$rel
"
    BEPINEX_COUNT=$((BEPINEX_COUNT + 1))
  done < <(unzip -Z1 "$ZIP")

  # -n, NEVER OVERWRITE, and it is what makes the line above true rather than
  # decorative. BepInEx's archive lays a changelog.txt in the game's own root; a
  # game build that ever ships one of its own would be silently replaced by an
  # extraction that overwrites, and the uninstall - which removes only what the
  # record names - would then leave BepInEx's copy behind for good. A file that
  # was already here is reported, skipped, and left out of the record, all three.
  unzip -q -n "$ZIP" -d "$GAME_DIR"
  [ -f "$BEPINEX_CORE" ] || stop_setup "BepInEx did not unpack into $GAME_DIR."
  BEPINEX_OURS='true'

  # The one bit a zip cannot be relied on to carry, on the one file that is
  # useless without it. Same ordering rule as step 1: this file has just been
  # written out of an archive whose own checksum was verified in step 1.
  for needed in run_bepinex.sh libdoorstop.so .doorstop_version; do
    [ -e "$GAME_DIR/$needed" ] || stop_setup "BepInEx unpacked without $needed, which the launcher needs."
  done
  chmod +x "$GAME_DIR/run_bepinex.sh"
  say "BepInEx $E_BEPINEX installed into the game directory ($BEPINEX_COUNT files)."
  say "run_bepinex.sh is BepInEx's own launcher, unmodified, and is now executable."
  say "The first game start after this is slower: BepInEx writes its configuration then."
fi

# ---------------------------------------------------------------- 5. the plugin

step "5 of 9 - the multiverse plugin"

PLUGIN_SRC="$HERE/$PLUGIN_NAME"
[ -f "$PLUGIN_SRC" ] || stop_setup "The package is incomplete: $PLUGIN_NAME is missing." 'INS-CHECKSUM'
SIDECAR_SRC="$HERE/$SIDECAR_NAME"
[ -f "$SIDECAR_SRC" ] || stop_setup "The package is incomplete: $SIDECAR_NAME is missing." 'INS-CHECKSUM'

PLUGIN_DIR="$GAME_DIR/BepInEx/plugins"
PLUGIN_DST="$PLUGIN_DIR/$PLUGIN_NAME"
PLUGIN_HELD='false'
[ -f "$PLUGIN_DST" ] && PLUGIN_HELD='true'
mkdir -p "$PLUGIN_DIR"
cp -f "$PLUGIN_SRC" "$PLUGIN_DST"
chmod 644 "$PLUGIN_DST"
PLUGIN_SHA="$(sha256_of "$PLUGIN_DST")"
if [ "$PLUGIN_HELD" = 'true' ]; then say "$PLUGIN_NAME replaced in $PLUGIN_DIR (an earlier install was here)"
else                                 say "$PLUGIN_NAME -> $PLUGIN_DIR"; fi
say "mod version $E_MOD, SHA-256 $(printf '%s' "$PLUGIN_SHA" | cut -c1-16)"
say "The plugin is the same file the Windows release ships: it is platform-independent IL."

if [ -z "$DATA_ROOT" ]; then DATA_ROOT="${XDG_DATA_HOME:-$HOME/.local/share}/bibites-multiverse"; fi
mkdir -p "$DATA_ROOT"
DATA_ROOT="$(cd "$DATA_ROOT" && pwd)"
case "$DATA_ROOT" in
  *"'"*) stop_setup "The data directory's path contains a single quote, which the generated start script cannot carry safely. Pass --data-root with a path without one." ;;
esac
DATA_DIR="$DATA_ROOT/data"
LOG_DIR="$DATA_ROOT/logs"
mkdir -p "$DATA_DIR" "$LOG_DIR"
say "this install's own files live in $DATA_ROOT"
say "your WORLDS are not in there and never will be: the game keeps them under"
say "\${XDG_CONFIG_HOME:-\$HOME/.config}/unity3d/The Bibites/The Bibites/Savefiles"

# ---------------------------------------------------------------- 6. the map identity

step "6 of 9 - connect this installation to a map"

PUBLIC_MAP_PATH="$HERE/$PUBLIC_MAP_NAME"
USE_PUBLIC_MAP=0
PENDING_ENROLLMENT_PATH=''
CREDENTIAL_PATH="$DATA_ROOT/peer-secret.txt"
CREDENTIAL=''

if [ -z "$JOIN_STRING_FILE" ] && [ -z "$RELAY_URL" ] && [ -f "$PUBLIC_MAP_PATH" ]; then
  USE_PUBLIC_MAP=1
  PENDING_ENROLLMENT_PATH="$DATA_ROOT/enrollment-pending.json"
  PUBLIC_MAP_FLAT="$(json_flatten "$PUBLIC_MAP_PATH")"
  [ "$(flat_get "$PUBLIC_MAP_FLAT" format)" = 'bibites-multiverse/public-map/1' ] || \
    stop_setup "$PUBLIC_MAP_NAME has an unsupported format." 'INS-ENROLL'
  ENROLLMENT_URL="$(flat_get "$PUBLIC_MAP_FLAT" enrollmentUrl)"
  PUBLIC_RELAY_URL="$(flat_get "$PUBLIC_MAP_FLAT" relayUrl)"
  case "$ENROLLMENT_URL" in
    https://?*) case "$ENROLLMENT_URL" in *[[:space:]]*) stop_setup "$PUBLIC_MAP_NAME does not contain a secure HTTPS enrollment address." 'INS-ENROLL' ;; esac ;;
    *) stop_setup "$PUBLIC_MAP_NAME does not contain a secure HTTPS enrollment address." 'INS-ENROLL' ;;
  esac
  case "$PUBLIC_RELAY_URL" in
    wss://?*) case "$PUBLIC_RELAY_URL" in *[[:space:]]*) stop_setup "$PUBLIC_MAP_NAME does not contain a secure WSS relay address." 'INS-ENROLL' ;; esac ;;
    *) stop_setup "$PUBLIC_MAP_NAME does not contain a secure WSS relay address." 'INS-ENROLL' ;;
  esac

  EXISTING_RECORD_PATH="$DATA_ROOT/$RECORD_NAME"
  EXISTING_RECORD_HELD=0
  CREDENTIAL_HELD=0
  [ -f "$EXISTING_RECORD_PATH" ] && EXISTING_RECORD_HELD=1
  [ -f "$CREDENTIAL_PATH" ] && CREDENTIAL_HELD=1
  REUSED_IDENTITY=0
  if [ "$EXISTING_RECORD_HELD" -eq 1 ] && [ "$CREDENTIAL_HELD" -eq 1 ]; then
    EXISTING_RECORD_FLAT="$(json_flatten "$EXISTING_RECORD_PATH")"
    EXISTING_PEER_ID="$(flat_get "$EXISTING_RECORD_FLAT" peerId)"
    EXISTING_RELAY_URL="$(flat_get "$EXISTING_RECORD_FLAT" relayUrl)"
    EXISTING_SECRET="$(head -n1 "$CREDENTIAL_PATH" | tr -d '\r\n')"
    if printf '%s' "$EXISTING_PEER_ID" | grep -Eq '^public-[0-9a-f]{32}$' &&
       printf '%s' "$EXISTING_SECRET" | grep -Eq '^[0-9a-f]{64}$' &&
       [ "$EXISTING_RELAY_URL" = "$PUBLIC_RELAY_URL" ]; then
      if [ -f "$PENDING_ENROLLMENT_PATH" ]; then
        LEFTOVER_FLAT="$(json_flatten "$PENDING_ENROLLMENT_PATH")"
        LEFTOVER_INSTALL_ID="$(flat_get "$LEFTOVER_FLAT" installId)"
        LEFTOVER_SECRET="$(flat_get "$LEFTOVER_FLAT" secret)"
        LEFTOVER_PEER_ID="public-$(printf '%s' "$LEFTOVER_INSTALL_ID" | tr -d '-' | tr 'A-F' 'a-f')"
        if [ "$(flat_get "$LEFTOVER_FLAT" format)" != 'bibites-multiverse/enrollment-pending/1' ] ||
           [ "$LEFTOVER_PEER_ID" != "$EXISTING_PEER_ID" ] ||
           [ "$LEFTOVER_SECRET" != "$EXISTING_SECRET" ]; then
          stop_setup "The completed identity and $PENDING_ENROLLMENT_PATH disagree. Neither file was changed. Ask the operator before replacing this world identity." 'INS-ENROLL'
        fi
      fi
      RELAY_URL="$PUBLIC_RELAY_URL"
      CREDENTIAL="$EXISTING_PEER_ID.$EXISTING_SECRET"
      REUSED_IDENTITY=1
      say "reusing this installation's existing public-map identity"
    fi
  fi

  if [ "$REUSED_IDENTITY" -eq 0 ]; then
    if { [ "$EXISTING_RECORD_HELD" -eq 1 ] || [ "$CREDENTIAL_HELD" -eq 1 ]; } &&
       [ ! -f "$PENDING_ENROLLMENT_PATH" ]; then
      stop_setup "This data root contains part of a completed map identity, but the installer cannot safely reuse it. Use a different --data-root for a new world, or ask the operator for a slot handover." 'INS-ENROLL'
    fi

    INSTALL_ID=''
    ENROLLMENT_SECRET=''
    if [ -f "$PENDING_ENROLLMENT_PATH" ]; then
      PENDING_FLAT="$(json_flatten "$PENDING_ENROLLMENT_PATH")"
      [ "$(flat_get "$PENDING_FLAT" format)" = 'bibites-multiverse/enrollment-pending/1' ] || \
        stop_setup "$PENDING_ENROLLMENT_PATH is not an enrollment record this installer can use. Remove it only if you want a different map identity." 'INS-ENROLL'
      INSTALL_ID="$(flat_get "$PENDING_FLAT" installId)"
      ENROLLMENT_SECRET="$(flat_get "$PENDING_FLAT" secret)"
      say 'retrying the pending public-map enrollment'
    else
      [ -r /proc/sys/kernel/random/uuid ] || \
        stop_setup 'This Linux system has no kernel UUID source. The installer did not contact the map.' 'INS-ENROLL'
      INSTALL_ID="$(tr 'A-F' 'a-f' < /proc/sys/kernel/random/uuid | tr -d '\r\n')"
      ENROLLMENT_SECRET="$(head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n')"
      ( umask 077
        printf '{\n  "format": "bibites-multiverse/enrollment-pending/1",\n  "installId": "%s",\n  "secret": "%s"\n}\n' \
          "$INSTALL_ID" "$ENROLLMENT_SECRET" > "$PENDING_ENROLLMENT_PATH"
      )
    fi

    if ! printf '%s' "$INSTALL_ID" | grep -Eq '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$' ||
       ! printf '%s' "$ENROLLMENT_SECRET" | grep -Eq '^[0-9a-f]{64}$'; then
      stop_setup "$PENDING_ENROLLMENT_PATH contains an invalid map identity. Remove it only if you want a different identity." 'INS-ENROLL'
    fi
    chmod 600 "$PENDING_ENROLLMENT_PATH"
    PENDING_MODE="$(stat -c '%a' "$PENDING_ENROLLMENT_PATH" 2>/dev/null || echo '?')"
    [ "$PENDING_MODE" = 600 ] || \
      stop_setup "The installer could not protect $PENDING_ENROLLMENT_PATH. It did not contact the map. Use a --data-root on a filesystem with Unix permissions." 'INS-ENROLL'

    EXPECTED_PEER_ID="public-$(printf '%s' "$INSTALL_ID" | tr -d '-')"
    REQUEST_BODY="$(printf '{"format":"bibites-multiverse/enrollment-request/1","installId":"%s","secret":"%s","release":"%s"}' \
      "$INSTALL_ID" "$ENROLLMENT_SECRET" "$RELEASE")"
    say "requesting a unique identity from $ENROLLMENT_URL"
    set +e
    ENROLLMENT_RESPONSE="$(printf '%s' "$REQUEST_BODY" | curl --fail --silent --show-error \
      --max-time 30 --header 'Content-Type: application/json' --data-binary @- "$ENROLLMENT_URL")"
    CURL_RC=$?
    set -e
    REQUEST_BODY=''
    if [ "$CURL_RC" -ne 0 ]; then
      stop_setup "The public map did not create this installation's identity. The pending identity was kept, so running the installer again is a safe retry." 'INS-ENROLL'
    fi
    RESPONSE_FLAT="$(printf '%s' "$ENROLLMENT_RESPONSE" | json_flatten /dev/stdin)"
    if [ "$(flat_get "$RESPONSE_FLAT" format)" != 'bibites-multiverse/enrollment-response/1' ] ||
       [ "$(flat_get "$RESPONSE_FLAT" relayUrl)" != "$PUBLIC_RELAY_URL" ] ||
       [ "$(flat_get "$RESPONSE_FLAT" peerId)" != "$EXPECTED_PEER_ID" ]; then
      stop_setup 'The public map returned an enrollment response for a different identity or relay.' 'INS-ENROLL'
    fi
    RELAY_URL="$PUBLIC_RELAY_URL"
    CREDENTIAL="$EXPECTED_PEER_ID.$ENROLLMENT_SECRET"
  fi
else
  JOIN_STRING=''
  if [ -n "$JOIN_STRING_FILE" ]; then
    [ -f "$JOIN_STRING_FILE" ] || stop_setup "No join string file at $JOIN_STRING_FILE."
    while IFS= read -r line || [ -n "$line" ]; do
      candidate="$(printf '%s' "$line" | tr -d '\r' | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
      case "$candidate" in ''|'#'*) continue ;; esac
      JOIN_STRING="$candidate"; break
    done < "$JOIN_STRING_FILE"
    [ -n "$JOIN_STRING" ] || stop_setup \
      "$JOIN_STRING_FILE holds no join string. Its first non-empty line must be the one your operator sent."
  else
    [ -t 0 ] || stop_setup \
      "There is no join string and no keyboard to ask at. Pass --join-string-file with the one line your operator sent. There is no --join-string option, deliberately."
    printf '\n'
    say "Paste the join string your map's operator handed you. It looks like this:"
    say "    multiverse-join/1 wss://<relay-host>/contract-b/v4 <your-world>.<secret>"
    say "The typing is hidden, because the second half of it is the whole of your"
    say "world's identity on that map. Nothing echoes it and no log ever prints it."
    printf '\n'
    printf '     join string: '
    IFS= read -r -s JOIN_STRING || JOIN_STRING=''
    printf '\n'
    JOIN_STRING="$(printf '%s' "$JOIN_STRING" | tr -d '\r' | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
  fi
  [ -n "$JOIN_STRING" ] || stop_setup "No join string was given, so this world has no identity on any map."

  # shellcheck disable=SC2086
  set -- $JOIN_STRING
  if [ $# -eq 3 ] && [ "$1" = 'multiverse-join/1' ]; then
    RELAY_URL="$2"
    CREDENTIAL="$3"
  elif [ $# -eq 1 ]; then
    CREDENTIAL="$1"
    [ -n "$RELAY_URL" ] || stop_setup \
      "That is the identity half on its own, with no relay address. Either paste the whole one-line join string - it starts with 'multiverse-join/1' - or run this script again with --relay-url wss://<relay-host>/contract-b/v4." \
      'INS-JOINSTRING'
  else
    stop_setup "That is not a join string this installer can read. The one-line form is three parts: 'multiverse-join/1', the wss:// relay address, and your identity and secret joined by a dot. Ask your operator to send the 'one line' from the block their relay printed." \
      'INS-JOINSTRING'
  fi
  set --
fi

case "$RELAY_URL" in
  ws://*) stop_setup "The relay address is ws://, which is not encrypted. This wire is always wss:// and there is no fallback anywhere in this software: a relay that answers ws:// off loopback refuses the connection rather than serving it. Ask your operator for the wss:// address." 'INS-JOINSTRING' ;;
  wss://?*) case "$RELAY_URL" in *[[:space:]]*) stop_setup "The relay address '$RELAY_URL' is not a wss:// URL." 'INS-JOINSTRING' ;; esac ;;
  *) stop_setup "The relay address '$RELAY_URL' is not a wss:// URL." 'INS-JOINSTRING' ;;
esac

# The two halves split on the LAST dot: an identity may legally contain one and a
# secret may not. This is exactly the wire's own rule (contract-b-m4.md 3.1).
PEER_ID="${CREDENTIAL%.*}"
SECRET="${CREDENTIAL##*.}"
if [ "$PEER_ID" = "$CREDENTIAL" ] || [ -z "$PEER_ID" ] || [ -z "$SECRET" ]; then
  stop_setup "The identity half and the secret half are joined by a dot, and this value has no usable one. Paste the whole 'one line' your operator sent." 'INS-JOINSTRING'
fi
SECRET_LEN="${#SECRET}"
if [ "$SECRET_LEN" -lt 32 ] || [ "$SECRET_LEN" -gt 256 ]; then
  stop_setup "The secret half is $SECRET_LEN characters. It must be 32 to 256. Nothing was written; ask your operator to re-send the join string exactly as their relay printed it." 'INS-JOINSTRING'
fi
case "$SECRET"  in *[![:print:]]*|*' '*) stop_setup "The secret half must be printable ASCII with no spaces. Nothing was written." 'INS-JOINSTRING' ;; esac
case "$PEER_ID" in *[![:print:]]*|*' '*) stop_setup "The identity half must be printable ASCII with no spaces. Nothing was written." 'INS-JOINSTRING' ;; esac

# Created empty and private BEFORE a byte of the secret is in it: a file that is
# world-readable for the instant between the write and the chmod is a file that
# was world-readable.
rm -f "$CREDENTIAL_PATH"
( umask 077; : > "$CREDENTIAL_PATH" )
chmod 600 "$CREDENTIAL_PATH"
printf '%s\n' "$SECRET" > "$CREDENTIAL_PATH"
CREDENTIAL_MODE="$(stat -c '%a' "$CREDENTIAL_PATH" 2>/dev/null || echo '?')"
if [ "$CREDENTIAL_MODE" = '600' ]; then
  say "the secret half is in $CREDENTIAL_PATH, mode 0600 - readable by you only"
else
  say "the secret half is in $CREDENTIAL_PATH"
  say "WARNING: its mode reads $CREDENTIAL_MODE rather than 600. If that directory is on a"
  say "filesystem with no Unix permissions, move --data-root somewhere that has them."
fi
SECRET=''
CREDENTIAL=''
ENROLLMENT_SECRET=''
EXISTING_SECRET=''
say "your world's identity on this map: $PEER_ID"
say "the relay it dials: $RELAY_URL"
printf '\n'
say "IF YOU LOSE THAT SECRET there is no software recovery: the relay keeps a verifier"
say "and cannot print it again. The only way back is to ask the operator for a slot"
say "handover, which mints a fresh one. Your slot, your position and everything"
say "addressed to you survive that - see docs/participant/leave.md."
if [ "$USE_PUBLIC_MAP" -eq 0 ] && [ -n "$JOIN_STRING_FILE" ]; then
  printf '\n'
  say "Delete $JOIN_STRING_FILE now. It still holds the secret in clear text, and this"
  say "installer will not delete a file you gave it."
fi
JOIN_STRING=''

# ---------------------------------------------------------------- 7. the certificate

step "7 of 9 - the relay's certificate"

CA_STORED=''
CA_SUBJECT=''
CA_NOTAFTER=''
CA_FILE_SHA=''

if [ -z "$CA_FILE" ]; then
  say "NOTHING WAS WRITTEN TO ANY TRUST STORE, and nothing needs to be."
  say "A public map's relay presents a certificate signed by an authority your"
  say "machine already trusts through /etc/ssl/certs, so your sidecar checks it the"
  say "same way curl checks a bank's. Your sidecar verifies that certificate on every"
  say "connection, and there is no switch anywhere in this software that skips the"
  say "check."
  say "--ca-file exists for a private or LAN map only. If your operator sent you a"
  say "ca.crt, run this installer again with --ca-file ./ca.crt."
else
  [ -f "$CA_FILE" ] || stop_setup "No certificate file at $CA_FILE."
  grep -q -- '-----BEGIN CERTIFICATE-----' "$CA_FILE" || stop_setup \
    "$CA_FILE holds no PEM certificate. A certificate authority for this comes as a PEM file, whose first line reads -----BEGIN CERTIFICATE-----. Ask your operator for that file rather than for a DER or a bundle in another form."
  CA_FILE_SHA="$(sha256_of "$CA_FILE")"
  if command -v openssl >/dev/null 2>&1; then
    CA_SUBJECT="$(openssl x509 -in "$CA_FILE" -noout -subject 2>/dev/null | sed -e 's/^subject= *//' || true)"
    CA_NOTAFTER="$(openssl x509 -in "$CA_FILE" -noout -enddate 2>/dev/null | sed -e 's/^notAfter=//' || true)"
    [ -n "$CA_SUBJECT" ] || stop_setup "$CA_FILE is not a certificate this machine can read."
    say "subject    : $CA_SUBJECT"
    say "valid until: $CA_NOTAFTER"
    if ! openssl x509 -in "$CA_FILE" -noout -checkend 0 >/dev/null 2>&1; then
      stop_setup "That authority expired on $CA_NOTAFTER. Ask your operator for a current one."
    fi
  else
    say "openssl is not on this machine, so the subject and the expiry date could not be"
    say "read. The file is a PEM certificate and it is used as it is; if it has expired,"
    say "your sidecar will say so on its first connection rather than here."
  fi
  say "file SHA-256: $CA_FILE_SHA"

  CA_STORED="$DATA_ROOT/relay-ca.crt"
  cp -f "$CA_FILE" "$CA_STORED"
  chmod 644 "$CA_STORED"

  printf '\n'
  say "WHAT YOU ARE AGREEING TO, because this is the one step that reaches outside"
  say "the game - and on Linux it reaches LESS far than you may expect. This"
  say "authority is NOT added to /etc/ssl/certs. Nothing is copied into"
  say "/usr/local/share/ca-certificates, update-ca-certificates is never run, and no"
  say "part of this needs root. What the start script does instead is set"
  say "SSL_CERT_FILE for the SIDECAR PROCESS ONLY, pointing at the copy above -"
  say "the platform's own mechanism, arranged before the process starts, which is"
  say "the only moment it can be arranged."
  say "The consequence, stated plainly: for that one process this authority is the"
  say "ONLY one trusted - not an addition to the system's list but a replacement of"
  say "it. That is narrower than trusting it everywhere, and it is why nothing else"
  say "on this machine changes. Every other program keeps the trust it had."
  say "Take it out whenever you like:"
  say "    ./$UNINSTALL_NAME"
  say "or by hand: delete $CA_STORED and the SSL_CERT_FILE line in $START_NAME."
  printf '\n'
fi

# ---------------------------------------------------------------- 8. the settings

step "8 of 9 - the settings this install ships with"

for n in SIDECAR_PORT SAVE_KEEP; do
  eval "v=\$$n"
  case "$v" in ''|*[!0-9]*) stop_setup "--$(printf '%s' "$n" | tr 'A-Z_' 'a-z-') takes a whole number, not '$v'." ;; esac
done
case "$SAVE_MINUTES" in ''|*[!0-9.]*) stop_setup "--save-minutes takes a number, not '$SAVE_MINUTES'." ;; esac

EDGE_LIST=''
for e in $(printf '%s' "$EXPORT_EDGES" | tr ',;' '  '); do
  up="$(printf '%s' "$e" | tr '[:lower:]' '[:upper:]')"
  case "$up" in
    E|N|W|S) ;;
    *) stop_setup "--export-edges holds '$e'. Use E, N, W or S." ;;
  esac
  case " $EDGE_LIST " in *" $up "*) stop_setup "--export-edges repeats an edge: '$EXPORT_EDGES'." ;; esac
  EDGE_LIST="$EDGE_LIST $up"
done
EDGE_LIST="$(printf '%s' "$EDGE_LIST" | sed -e 's/^ //')"
[ -n "$EDGE_LIST" ] || stop_setup \
  "--export-edges names no edge. Use E, N, W or S, comma separated - normally 'E,N,W,S'. If you want this world off the map, do not start it."
EXPORT_EDGES="$(printf '%s' "$EDGE_LIST" | tr ' ' ',')"

if [ "$NO_MIGRATION_EXCLUSION" -eq 1 ]; then
  EXCLUDE_SPECIES=''
elif [ -z "$EXCLUDE_SPECIES" ]; then
  stop_setup "--exclude-species is empty, which would turn the exclusion policy off. That is a real choice and it takes its own flag: pass --no-migration-exclusion if you mean it."
fi

case "$SAVE_ON_QUIT" in on|off) ;; *) stop_setup "--save-on-quit takes on or off, not '$SAVE_ON_QUIT'." ;; esac

printf '\n'
printf '  WHAT A BARE INSTALL DOES, STATED ONCE:\n'
printf '  YOUR WORLD EXPORTS ON ALL FOUR EDGES. Nothing configured means the whole\n'
printf '  perimeter, not silence. Every edge is a door that works both ways: organisms\n'
printf '  leave your world on every side and arrive from every side.\n'
printf '\n'
say "This install was set up with export edges: $EXPORT_EDGES"
if [ "$EXPORT_EDGES" = 'E,N,W,S' ]; then
  say "which is the shipped default. It is written out explicitly all the same, so that"
  say "a future change to the mod's own default cannot silently move what your world does."
else
  say "which is a subset you asked for. The edges you left out run no capture band; your"
  say "world still RECEIVES on all four, because an arrival was never gated by this list."
fi
printf '\n'
say "The settings this install runs with, and what each one spends:"
printf '\n'
say "  export edges       $EXPORT_EDGES"
if [ "$EXPORT_EDGES" = 'E,N,W,S' ]; then
  say "                     your world is a full member of the map in every direction"
else
  say "                     your world exports on those edges only, and receives on all four"
fi
if [ -n "$EXCLUDE_SPECIES" ]; then
  say "  species that       '$EXCLUDE_SPECIES'"
  say "  never leave        keeps founder stock off a shared map's lanes"
else
  printf '     species that       NONE - the exclusion policy is OFF\n'
  printf '     never leave        you asked for --no-migration-exclusion. Your world will export\n'
  printf '                        starter stock onto a shared map, and the map'\''s census shows\n'
  printf '                        that as entirely normal while it happens.\n'
fi
say "  saves              every $SAVE_MINUTES minutes, keeping $SAVE_KEEP"
say "                     $SAVE_KEEP copies of your world on your disk. The interval is also how"
say "                     often your world pauses to write itself out"
say "  save on quit       $SAVE_ON_QUIT"
say "                     your world is written out when the game closes, so stopping is not losing"
printf '\n'
say "All four are set explicitly in $START_NAME, and you can edit them there. The"
say "names in that file are the mod's own: MULTIVERSE_EXPORT_EDGES,"
say "MULTIVERSE_MIGRATION_EXCLUDE, MULTIVERSE_SAVE_MINUTES, MULTIVERSE_SAVE_KEEP"
say "and MULTIVERSE_SAVE_ON_QUIT."

# ---------------------------------------------------------------- 9. the scripts

step "9 of 9 - write $START_NAME, $STOP_NAME and the uninstall's record"

SIDECAR_EXE="$HERE/$SIDECAR_NAME"
if [ "$SAVE_ON_QUIT" = 'on' ]; then SAVE_ON_QUIT_VALUE='true'; else SAVE_ON_QUIT_VALUE='false'; fi

IFS= read -r -d '' START_BODY <<'START_TEMPLATE' || true
#!/usr/bin/env bash
# Written by install-bibites-multiverse.sh. Start this world on the map: the
# sidecar first, then the game.
#
#   ./@@STARTNAME@@              the sidecar, then the game
#   ./@@STARTNAME@@ --game-only  the game only, against a sidecar already running
#   ./@@STARTNAME@@ --headless   the world runs with nothing drawn. The simulation
#                                is unchanged; only the picture is gone
#
# THE ORDER MATTERS. The sidecar mints the local token the game's mod presents,
# into its own data directory, the first time it starts. So the sidecar goes
# first and the game follows.
set -uo pipefail

GAME_DIR='@@GAMEDIR@@'
DATA_ROOT='@@DATAROOT@@'
RELAY_URL='@@RELAYURL@@'
SIDECAR='@@SIDECAREXE@@'
PEER_ID='@@PEERID@@'
WORLD='@@WORLD@@'
SIDECAR_PORT='@@SIDECARPORT@@'
CA_FILE='@@CAFILE@@'
GAME_EXE='@@GAMEEXE@@'

GAME_ONLY=0
HEADLESS=0
for arg in "$@"; do
  case "$arg" in
    --game-only) GAME_ONLY=1 ;;
    --headless)  HEADLESS=1 ;;
    *) printf 'usage: %s [--game-only] [--headless]\n' "$0" >&2; exit 2 ;;
  esac
done

DATA_DIR="$DATA_ROOT/data"
LOG_DIR="$DATA_ROOT/logs"
CREDENTIAL_FILE="$DATA_ROOT/peer-secret.txt"
LOG="$LOG_DIR/sidecar.log"
GAME_LOG="$LOG_DIR/game.out"
SIDECAR_PID_FILE="$DATA_ROOT/sidecar.pid"
GAME_PID_FILE="$DATA_ROOT/game.pid"
mkdir -p "$DATA_DIR" "$LOG_DIR"

# The mod reads its whole configuration from the environment of the game
# process, which it inherits from this script through BepInEx's launcher.
#
# EVERY ONE OF THESE IS SET EXPLICITLY, INCLUDING THE ONES THAT MATCH THE MOD'S
# OWN DEFAULT. An unset MULTIVERSE_EXPORT_EDGES already means all four edges;
# writing it out anyway is what keeps a future change to that default from
# silently moving what this world does. Edit the values here.
export MULTIVERSE_EXPORT_EDGES='@@EXPORTEDGES@@'
export MULTIVERSE_MIGRATION_EXCLUDE='@@EXCLUDESPECIES@@'
export MULTIVERSE_SAVE_MINUTES='@@SAVEMINUTES@@'
export MULTIVERSE_SAVE_KEEP='@@SAVEKEEP@@'
export MULTIVERSE_SAVE_ON_QUIT='@@SAVEONQUIT@@'
export MULTIVERSE_SIDECAR_PORT="$SIDECAR_PORT"
export MULTIVERSE_WORLD="$WORLD"
export MULTIVERSE_PORTAL='true'
export MULTIVERSE_PORTAL_FLOURISHES='true'
# The link between the game and the sidecar runs on this machine's loopback and
# is authenticated: the sidecar mints this file at its first start, readable by
# you only, and the game presents its contents on every connection. It is NOT
# the map credential - different secret, different file, different wire - and
# the mod never writes its value to any log.
#
# ON LINUX THIS IS A PLAIN PATH AND NOTHING TRANSLATES IT. There is no WSLENV
# here, no drive letter and no second spelling of the same file: what this
# variable says is what the mod opens.
export MULTIVERSE_CONTRACT_A_TOKEN_FILE="$DATA_DIR/contract-a.token"

running() { # $1 pid file -> 0 when that process is alive
  local pid
  [ -f "$1" ] || return 1
  pid="$(head -n1 "$1" 2>/dev/null || true)"
  [ -n "$pid" ] || return 1
  kill -0 "$pid" 2>/dev/null
}

start_game() {
  # BepInEx's own launcher, unmodified, from the game's own directory, with the
  # game binary as its first argument. It execs the game, so the process id
  # recorded below is the game's own and stop-multiverse.sh can signal it.
  #
  # -batchmode -nographics are Unity's own flags. The simulation is unchanged by
  # them; the mod's min-FPS governor detects a process with no graphics device
  # and disarms itself.
  if [ "$HEADLESS" -eq 1 ]; then
    ( cd "$GAME_DIR" && exec ./run_bepinex.sh "./$GAME_EXE" -batchmode -nographics ) >>"$GAME_LOG" 2>&1 &
  else
    ( cd "$GAME_DIR" && exec ./run_bepinex.sh "./$GAME_EXE" ) >>"$GAME_LOG" 2>&1 &
  fi
  printf '%s\n' "$!" > "$GAME_PID_FILE"
  printf '%s' "$!"
}

if [ "$GAME_ONLY" -eq 1 ]; then
  if ! running "$SIDECAR_PID_FILE"; then
    printf -- '--game-only needs the sidecar already running. It is not.\n' >&2
    printf 'Run ./%s with no arguments.\n' "$(basename "$0")" >&2
    exit 1
  fi
  if running "$GAME_PID_FILE"; then
    printf 'The game is already running.\n'
    exit 1
  fi
  pid="$(start_game)"
  printf "game started (pid %s) against the running sidecar; it loads '%s' by itself.\n" "$pid" "$WORLD"
  printf 'The sidecar delivers everything it took custody of while the world was away, paced.\n'
  exit 0
fi

if running "$SIDECAR_PID_FILE"; then
  printf 'A sidecar is already running. Run ./%s first,\n' '@@STOPNAME@@'
  printf 'or ./%s --game-only to start only the game.\n' '@@STARTNAME@@'
  exit 1
fi
if [ ! -f "$CREDENTIAL_FILE" ]; then
  printf 'There is no credential at %s.\n' "$CREDENTIAL_FILE" >&2
  printf 'Run ./install-bibites-multiverse.sh again. For a private map, pass --join-string-file.\n' >&2
  exit 1
fi

# A private map's authority, the platform's own way. This replaces the trust
# store for THIS PROCESS ONLY - nothing on this machine was changed to make it
# work, and no other program sees it. A public map needs none of this and the
# line below is absent from the script an installer without --ca-file writes.
@@SSLCERTLINE@@

rm -f "$LOG" "$LOG.out"
# --credential-file carries the SECRET HALF only. The identity half is
# --peer-id, and the relay refuses any connection whose credential does not
# authenticate the identity it claims.
nohup "$SIDECAR" \
  --listen "127.0.0.1:$SIDECAR_PORT" \
  --relay "$RELAY_URL" \
  --peer-id "$PEER_ID" \
  --data-dir "$DATA_DIR" \
  --credential-file "$CREDENTIAL_FILE" \
  >"$LOG.out" 2>"$LOG" &
SIDECAR_PID=$!
printf '%s\n' "$SIDECAR_PID" > "$SIDECAR_PID_FILE"
printf 'sidecar started (pid %s) -> %s\n' "$SIDECAR_PID" "$RELAY_URL"
printf 'waiting for the map to grant this world a place ...\n'

GRANTED=''
REFUSED=''
DEADLINE=$(( $(date +%s) + 60 ))
while [ "$(date +%s)" -lt "$DEADLINE" ]; do
  if [ -f "$LOG" ]; then
    GRANTED="$(grep -F 'contract B: slot granted' "$LOG" 2>/dev/null | tail -n1 || true)"
    [ -n "$GRANTED" ] && break
    REFUSED="$(grep -E 'placement claim refused|HTTP 401|certificate did not verify|below this relay' "$LOG" 2>/dev/null | tail -n1 || true)"
    [ -n "$REFUSED" ] && break
  fi
  kill -0 "$SIDECAR_PID" 2>/dev/null || break
  sleep 0.5
done

if [ -n "$GRANTED" ]; then
  printf '\nYOU ARE ON THE MAP:\n'
  printf '  %s\n' "$GRANTED"
else
  printf '\nThe map did not grant a place.\n' >&2
  [ -n "$REFUSED" ] && printf '  %s\n' "$REFUSED" >&2
  [ -f "$LOG" ] && tail -n 20 "$LOG" | sed -e 's/^/  /' >&2
  cat >&2 <<'CAUSES'

  The five usual causes, in order:
   1. The relay is not reachable from here - a name that does not resolve, or a
      network that does not carry the connection. Neither is on the map's side.
   2. 'the relay's TLS certificate did not verify'. On a public map that is a
      certificate problem the operator has to fix; on a private map it means the
      authority is not trusted here yet - re-run the installer with --ca-file.
   3. HTTP 401: this credential is not the one the relay holds for this world.
      Ask the operator for a slot handover; a join string cannot be reprinted.
   4. Your wire version is below the minimum this map admits. Install the newest
      release; nobody on the relay's side can push it to you.
   5. Your game build is not the one this map is on. Only the operator can see
      which build that is.

  Each of those is an entry in docs/error-taxonomy.md, with who has to act.
CAUSES
  printf '  The game was NOT started. Run ./%s, then try again.\n' '@@STOPNAME@@' >&2
  exit 1
fi

pid="$(start_game)"
printf '\n'
printf "game started (pid %s); it loads the world '%s' by itself,\n" "$pid" "$WORLD"
printf 'and seeds it on the first start. It saves itself every @@SAVEMINUTES@@ minutes.\n'
printf 'logs: %s\n' "$LOG"
printf '      %s\n' "$GAME_DIR/BepInEx/LogOutput.log"
printf '      %s   (what the game itself printed)\n' "$GAME_LOG"
printf 'ONE INSTANCE PER GAME FOLDER. Two of them share that BepInEx log, both keep\n'
printf 'writing to it, and it ends as mostly NUL bytes while everything else works.\n'
printf 'See LOCAL-LOGSHRED in docs/error-taxonomy.md.\n'
printf 'Leave both running. Run ./%s when you are done.\n' '@@STOPNAME@@'
START_TEMPLATE

IFS= read -r -d '' STOP_BODY <<'STOP_TEMPLATE' || true
#!/usr/bin/env bash
# Written by install-bibites-multiverse.sh. Stop this world: the game first,
# then the sidecar.
#
#   ./@@STOPNAME@@              the game and the sidecar
#   ./@@STOPNAME@@ --game-only  the game only. The sidecar stays up, keeps this
#                               world's place on the map and keeps taking custody
#                               of everything that arrives while the world is away
#
# Stopping costs nothing and needs nobody. Your place on the map is keyed on
# your world's identity and never expires; your neighbours route around you
# while you are away. See docs/participant/leave.md.
set -uo pipefail

DATA_ROOT='@@DATAROOT@@'

GAME_ONLY=0
for arg in "$@"; do
  case "$arg" in
    --game-only) GAME_ONLY=1 ;;
    *) printf 'usage: %s [--game-only]\n' "$0" >&2; exit 2 ;;
  esac
done

# SIGTERM, then WAIT, and only then SIGKILL. The wait is the point: this world's
# save-on-quit runs while the game is shutting down, and a world that is killed
# outright loses everything since its last save. Twenty seconds is generous
# against the 2 s a clean quit with its save has measured at.
stop_recorded() { # $1 pid file, $2 what it is
  local file="$1" what="$2" pid i
  [ -f "$file" ] || return 0
  pid="$(head -n1 "$file" 2>/dev/null || true)"
  if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
    kill -TERM "$pid" 2>/dev/null || true
    for i in $(seq 1 40); do
      kill -0 "$pid" 2>/dev/null || break
      sleep 0.5
    done
    if kill -0 "$pid" 2>/dev/null; then
      printf '%s did not stop when asked; killing it. A world killed this way loses\n' "$what"
      printf 'everything since its last save.\n'
      kill -KILL "$pid" 2>/dev/null || true
      sleep 1
    else
      printf 'stopped %s (pid %s)\n' "$what" "$pid"
    fi
  fi
  rm -f "$file"
}

stop_recorded "$DATA_ROOT/game.pid" 'the game'

if [ "$GAME_ONLY" -eq 1 ]; then
  printf 'the world is down; the sidecar keeps its place and its journal.\n'
  printf 'Arrivals accumulate there and are delivered, paced, when the world comes back.\n'
  exit 0
fi

stop_recorded "$DATA_ROOT/sidecar.pid" 'the sidecar'

LEFT=0
for f in "$DATA_ROOT/game.pid" "$DATA_ROOT/sidecar.pid"; do
  [ -f "$f" ] && LEFT=$((LEFT + 1))
done
printf 'processes still recorded as running: %s (want 0)\n' "$LEFT"
printf 'The journal in %s/data is kept. Do not delete it: it is this machine'\''s\n' "$DATA_ROOT"
printf 'record of every organism it is holding for somebody.\n'
STOP_TEMPLATE

if [ -n "$CA_STORED" ]; then
  SSL_CERT_LINE="export SSL_CERT_FILE='$CA_STORED'"
else
  SSL_CERT_LINE="# (no --ca-file was given: this map's relay uses an authority this machine already trusts)"
fi

expand_template() {
  local body="$1"
  body="${body//@@GAMEDIR@@/$GAME_DIR}"
  body="${body//@@DATAROOT@@/$DATA_ROOT}"
  body="${body//@@RELAYURL@@/$RELAY_URL}"
  body="${body//@@SIDECAREXE@@/$SIDECAR_EXE}"
  body="${body//@@PEERID@@/$PEER_ID}"
  body="${body//@@WORLD@@/$WORLD}"
  body="${body//@@EXPORTEDGES@@/$EXPORT_EDGES}"
  body="${body//@@EXCLUDESPECIES@@/$EXCLUDE_SPECIES}"
  body="${body//@@SAVEMINUTES@@/$SAVE_MINUTES}"
  body="${body//@@SAVEKEEP@@/$SAVE_KEEP}"
  body="${body//@@SAVEONQUIT@@/$SAVE_ON_QUIT_VALUE}"
  body="${body//@@SIDECARPORT@@/$SIDECAR_PORT}"
  body="${body//@@CAFILE@@/$CA_STORED}"
  body="${body//@@GAMEEXE@@/$GAME_EXE}"
  body="${body//@@SSLCERTLINE@@/$SSL_CERT_LINE}"
  body="${body//@@STARTNAME@@/$START_NAME}"
  body="${body//@@STOPNAME@@/$STOP_NAME}"
  printf '%s\n' "$body"
}

START_PATH="$HERE/$START_NAME"
STOP_PATH="$HERE/$STOP_NAME"
expand_template "$START_BODY" > "$START_PATH"
expand_template "$STOP_BODY"  > "$STOP_PATH"
chmod +x "$START_PATH" "$STOP_PATH"
say "wrote $START_PATH"
say "wrote $STOP_PATH"

# The record the uninstall reads. It names every path this installer created or
# replaced, with the hash it left behind, so the uninstall can remove exactly
# what was added and leave anything a later hand changed.
json_escape() { printf '%s' "$1" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g'; }

RECORD_PATH="$DATA_ROOT/$RECORD_NAME"
{
  printf '{\n'
  printf '  "record": "bibites-multiverse/install-record/2",\n'
  printf '  "release": "%s",\n' "$(json_escape "$RELEASE")"
  printf '  "platform": "%s",\n' "$PLATFORM"
  printf '  "installedUtc": "%s",\n' "$(date -u +'%Y-%m-%dT%H:%M:%SZ')"
  printf '  "kitDir": "%s",\n'   "$(json_escape "$HERE")"
  printf '  "gameDir": "%s",\n'  "$(json_escape "$GAME_DIR")"
  printf '  "dataRoot": "%s",\n' "$(json_escape "$DATA_ROOT")"
  printf '  "runtime": {\n'
  printf '    "mode": "%s",\n' "$RUNTIME_MODE"
  printf '    "managedByThisInstaller": %s,\n' "$( [ "$RUNTIME_MODE" = 'bundled' ] && printf true || printf false )"
  printf '    "root": "%s",\n' "$(json_escape "$RUNTIME_ROOT")"
  printf '    "files": ['
  runtime_first=1
  if [ "$RUNTIME_MODE" = 'bundled' ]; then
    printf '%s\n' "$RUNTIME_FILES" | while IFS= read -r rel; do
      [ -n "$rel" ] || continue
      [ "$runtime_first" -eq 1 ] && printf '\n' || printf ',\n'
      runtime_first=0
      printf '      { "path": "%s", "sha256": "%s" }' \
        "$(json_escape "$RUNTIME_ROOT/$rel")" "$(sha256_of "$PAYLOAD_GAME_DIR/$rel")"
    done
  fi
  printf '\n    ]\n'
  printf '  },\n'
  printf '  "peerId": "%s",\n'   "$(json_escape "$PEER_ID")"
  printf '  "relayUrl": "%s",\n' "$(json_escape "$RELAY_URL")"
  printf '  "plugin": {\n'
  printf '    "path": "%s",\n'   "$(json_escape "$PLUGIN_DST")"
  printf '    "sha256": "%s",\n' "$PLUGIN_SHA"
  printf '    "replacedExisting": %s,\n' "$PLUGIN_HELD"
  printf '    "configFile": "%s"\n' "$(json_escape "$GAME_DIR/BepInEx/config/$PLUGIN_GUID.cfg")"
  printf '  },\n'
  printf '  "bepInEx": {\n'
  printf '    "installedByThisInstaller": %s,\n' "$BEPINEX_OURS"
  printf '    "version": "%s",\n' "$(json_escape "$E_BEPINEX")"
  printf '    "flavour": "%s",\n' "$(json_escape "$E_FLAVOUR")"
  printf '    "files": ['
  first=1
  printf '%s' "$BEPINEX_PATHS" | while IFS= read -r rel; do
    [ -n "$rel" ] || continue
    [ -f "$GAME_DIR/$rel" ] || continue
    [ "$first" -eq 1 ] && printf '\n' || printf ',\n'
    first=0
    printf '      { "path": "%s", "sha256": "%s" }' \
      "$(json_escape "$rel")" "$(sha256_of "$GAME_DIR/$rel")"
  done
  printf '\n    ]\n'
  printf '  },\n'
  printf '  "certificate": {\n'
  printf '    "systemTrustStoreTouched": false,\n'
  printf '    "trustMode": "%s",\n' \
    "$( [ -n "$CA_STORED" ] && printf 'SSL_CERT_FILE in %s, for that process only' "$START_NAME" || printf 'none needed: the platform already trusts this relay' )"
  printf '    "fileSha256": "%s",\n' "$CA_FILE_SHA"
  printf '    "storedCopy": "%s"\n' "$(json_escape "$CA_STORED")"
  printf '  },\n'
  printf '  "generated": ["%s", "%s"],\n' "$(json_escape "$START_PATH")" "$(json_escape "$STOP_PATH")"
  printf '  "credential": "%s",\n' "$(json_escape "$CREDENTIAL_PATH")"
  printf '  "dataDir": "%s",\n' "$(json_escape "$DATA_DIR")"
  printf '  "logDir": "%s"\n' "$(json_escape "$LOG_DIR")"
  printf '}\n'
} > "$RECORD_PATH"
say "wrote $RECORD_PATH - the uninstall reads it, and removes only what is named in it"
if [ "$USE_PUBLIC_MAP" -eq 1 ] && [ -n "$PENDING_ENROLLMENT_PATH" ] &&
   [ -f "$PENDING_ENROLLMENT_PATH" ]; then
  rm -f "$PENDING_ENROLLMENT_PATH"
  say 'removed the pending enrollment record after completing the install record'
fi

# ---------------------------------------------------------------- done

printf '\nSetup is complete.\n'
if [ "$RUNTIME_MODE" = 'bundled' ]; then say "edition      : complete (managed game runtime)"
else                                      say "edition      : add-on (existing game)"; fi
say "map          : $RELAY_URL"
say "your world   : $PEER_ID   world file: $WORLD"
say "game         : $GAME_DIR"
say "export edges : $EXPORT_EDGES   (all four is the shipped default)"
if [ -n "$EXCLUDE_SPECIES" ]; then say "never leaves : $EXCLUDE_SPECIES"
else                               say "never leaves : nothing - the exclusion policy is OFF"; fi
say "saves        : every $SAVE_MINUTES minutes, keeping $SAVE_KEEP, save on quit $SAVE_ON_QUIT"
say "your files   : $DATA_ROOT"
say "your worlds  : \${XDG_CONFIG_HOME:-\$HOME/.config}/unity3d/The Bibites/The Bibites"
if [ -n "$CA_STORED" ]; then
  say "certificate  : trusted through SSL_CERT_FILE in $START_NAME, for that process only"
  say "               NOTHING was written to any system trust store"
else
  say "certificate  : nothing imported, and nothing needed to be"
fi
printf '\n'
say "Next: run  ./$START_NAME"
say "Later: ./$STOP_NAME to stop, and ./$UNINSTALL_NAME to remove all of it."
printf '\n'
say "The four pages written for you, on the release page: install, join, diagnose, leave."
