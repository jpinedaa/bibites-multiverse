#!/usr/bin/env bash
#
# uninstall-bibites-multiverse.sh
#
# Remove the Bibites Multiverse mod and its sidecar from the game's native Linux
# build, and leave the game exactly as the installer found it.
#
# It needs no root and removes NOTHING it did not put there.
#
# It reads install-record.json - the record the installer wrote - and works from
# that list alone. Every path in the record was created or replaced by the
# installer, with the hash it left behind, so this script can tell a file it
# installed from a file somebody has since changed. A changed file is reported
# and KEPT.
#
# WHAT IT REMOVES
#
#   * BepInEx/plugins/BibitesMultiverse.dll, if it is still the file the
#     installer put there
#   * BepInEx/config/dev.multiverse.bibites.cfg, the plugin's own settings file,
#     which nothing else writes
#   * BepInEx itself and its generated files - ONLY if the installer put BepInEx
#     there. That includes the four files BepInEx's Linux archive lays in the
#     game's own root: run_bepinex.sh, libdoorstop.so, .doorstop_version and its
#     changelog.txt. If BepInEx was already installed on this machine, all of it
#     is left completely alone
#   * start-multiverse.sh and stop-multiverse.sh
#   * the copy of a private map's certificate authority
#   * an unchanged managed game payload, when this was the complete edition -
#     and, once nothing this install recorded is left in
#     <data root>/runtimes/<sha>, that whole directory with it, including what
#     the game and BepInEx wrote inside it while it ran. It is this package's own
#     copy of the game, nothing of yours is in it, and a game directory with no
#     game left in it is one the next install cannot repair. A game file somebody
#     CHANGED is still reported and kept, and keeps the directory around it
#
# WHAT IT NEVER HAD TO DO, on this platform
#
#   * take anything out of a trust store. The Linux installer puts nothing in
#     one: a private map's authority is trusted through SSL_CERT_FILE in the
#     start script, for that process only, so deleting the copy and the script
#     IS the removal. Nothing under /etc/ssl or
#     /usr/local/share/ca-certificates was ever written, and
#     update-ca-certificates was never run
#
# WHAT IT KEEPS, DELIBERATELY
#
#   * YOUR WORLDS AND THEIR BACKUPS. They are the game's own files, under
#     ${XDG_CONFIG_HOME:-$HOME/.config}/unity3d/The Bibites/The Bibites, and
#     nothing in this package has ever written outside its own directory there.
#     This script does not go near them
#   * YOUR JOURNAL - the record of every organism this machine took custody of
#     and has not handed on. Pass --remove-world-data to delete it too, and read
#     the warning that prints before it happens
#   * YOUR MAP CREDENTIAL - peer-secret.txt. It is the world's, not the
#     install's: the map keeps a verifier and can never print that secret again,
#     so removing a build must not end the world that ran on it. Installing again
#     over the same data root reuses it and spends no second identity. It goes
#     with the journal, under --remove-world-data, and never on its own
#   * any file the installer did not create, including another mod's plugin,
#     ANYWHERE OUTSIDE THIS PACKAGE'S OWN MANAGED GAME COPY - a game directory
#     you chose is never swept, only the files this install put in it are taken,
#     and a game you installed yourself is left exactly as it was found
#
set -euo pipefail

RECORD_NAME='install-record.json'
PLUGIN_GUID='dev.multiverse.bibites'

DATA_ROOT=''
RECORD_FILE=''
REMOVE_WORLD_DATA=0
DRY_RUN=0

say()  { printf '     %s\n' "$*"; }
step() { printf '\n==== %s\n' "$*"; }
stop_uninstall() { printf '\nSTOP: %s\n' "$*"; exit 1; }

sha256_of() { sha256sum "$1" | cut -d' ' -f1 | tr 'a-f' 'A-F'; }

is_managed_runtime() {
  # THE ONE DIRECTORY THIS SCRIPT MAY REMOVE WHOLE, and the answer is no for
  # every other path on the machine. It is the same rule, written the same way,
  # as the installer's own is_managed_runtime - the two scripts have to agree
  # about which directory the complete edition owns, and a rule in one place that
  # the other only approximates is how they would come to disagree.
  #
  #   * an absolute path with no `..` anywhere in it
  #   * its parent is EXACTLY <data root>/runtimes
  #   * its own name is a 64-character hex SHA-256
  #
  # The record's own dataRoot is what it is measured against, because the
  # record's runtime.root and its dataRoot were written by one run of the
  # installer and cannot disagree with each other.
  #
  # $1 path, $2 data root
  local path="$1" root="${2:-}" parent leaf
  [ -n "$path" ] && [ -n "$root" ] || return 1
  case "$path" in /*) ;; *) return 1 ;; esac
  case "/$path/" in */../*) return 1 ;; esac
  path="${path%/}"; root="${root%/}"
  [ -n "$path" ] && [ -n "$root" ] || return 1
  parent="$(dirname "$path")"
  leaf="$(basename "$path")"
  [ "$parent" = "$root/runtimes" ] || return 1
  [ "${#leaf}" -eq 64 ] || return 1
  case "$leaf" in *[!0-9A-Fa-f]*) return 1 ;; esac
  return 0
}

usage() {
  cat <<USAGE
usage: ./$(basename "$0") [options]

  --data-root <path>     Where this install kept its files. Default:
                         \${XDG_DATA_HOME:-\$HOME/.local/share}/bibites-multiverse
  --record-file <path>   The install record, if it is not in the usual place.
  --remove-world-data    Also delete the journal, the logs and the data
                         directory. The journal may still hold organisms other
                         worlds handed to this one.
  --dry-run              Print the ledger of what would be removed and what would
                         be kept, and change nothing.
  -h, --help             This text.

There is no --keep-certificate, because there is nothing to keep: this platform's
installer never wrote to a trust store.
USAGE
}

need_value() { [ "$2" -gt 1 ] || stop_uninstall "$1 needs a value."; }

while [ $# -gt 0 ]; do
  case "$1" in
    --data-root)   need_value "$1" $#; DATA_ROOT="$2";   shift 2 ;;
    --record-file) need_value "$1" $#; RECORD_FILE="$2"; shift 2 ;;
    --remove-world-data) REMOVE_WORLD_DATA=1; shift ;;
    --dry-run)           DRY_RUN=1;           shift ;;
    -h|--help) usage; exit 0 ;;
    *) usage >&2; stop_uninstall "unknown option: $1" ;;
  esac
done

# The same JSON reader the installer uses, and for the same reason: a bare
# machine has awk and needs neither jq nor python to take this off again.
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
  printf '%s\n' "$1" | awk -v k="$2" 'index($0, k "\t") == 1 { print substr($0, length(k) + 2); exit }'
}

REMOVED=''
KEPT=''
add_removed() { REMOVED="$REMOVED$1
"; }
add_kept()    { KEPT="$KEPT$1
"; }

remove_recorded() { # $1 path, $2 expected sha256 or '', $3 what it is or ''
  local path="$1" want="${2:-}" what="${3:-}" got
  if [ ! -e "$path" ]; then
    add_kept "gone already : $path"
    return 0
  fi
  if [ -n "$want" ]; then
    got="$(sha256_of "$path")"
    if [ "$got" != "$(printf '%s' "$want" | tr 'a-f' 'A-F')" ]; then
      add_kept "CHANGED since the install, so it is left alone : $path"
      return 0
    fi
  fi
  [ "$DRY_RUN" -eq 1 ] || rm -f "$path"
  if [ -n "$what" ]; then add_removed "$path   ($what)"; else add_removed "$path"; fi
}

remove_empty_directory() { # $1 path
  local path="$1" left
  [ -d "$path" ] || return 0
  left="$(find "$path" -mindepth 1 -maxdepth 1 | wc -l)"
  if [ "$left" -gt 0 ]; then
    add_kept "not empty, so it stays : $path ($left item(s) this installer did not create)"
    return 0
  fi
  [ "$DRY_RUN" -eq 1 ] || rmdir "$path"
  add_removed "$path   (empty directory)"
}

# ---------------------------------------------------------------- the record

step "the install record"

[ -n "$DATA_ROOT" ]   || DATA_ROOT="${XDG_DATA_HOME:-$HOME/.local/share}/bibites-multiverse"
[ -n "$RECORD_FILE" ] || RECORD_FILE="$DATA_ROOT/$RECORD_NAME"
[ -f "$RECORD_FILE" ] || stop_uninstall \
  "No install record at $RECORD_FILE, so this script cannot tell what it put on this machine and refuses to guess. If you moved the data directory, pass --data-root. Nothing was removed."

REC="$(json_flatten "$RECORD_FILE")"
[ -n "$REC" ] || stop_uninstall "$RECORD_FILE could not be read. Nothing was removed."

R_GAMEDIR="$(flat_get "$REC" gameDir)"
R_DATAROOT="$(flat_get "$REC" dataRoot)"
R_DATADIR="$(flat_get "$REC" dataDir)"
R_LOGDIR="$(flat_get "$REC" logDir)"
R_KITDIR="$(flat_get "$REC" kitDir)"
R_CREDENTIAL="$(flat_get "$REC" credential)"
R_PEERID="$(flat_get "$REC" peerId)"

say "record   : $RECORD_FILE"
say "installed: $(flat_get "$REC" installedUtc)   release $(flat_get "$REC" release)"
say "platform : $(flat_get "$REC" platform)"
say "game     : $R_GAMEDIR"
say "world id : $R_PEERID"
if [ "$DRY_RUN" -eq 1 ]; then
  printf '\n  --dry-run: nothing below is actually removed.\n'
fi

# Both checks are on THIS install's own paths rather than on a process name: a
# machine may hold a second copy of the game or a second world, and only the one
# this record describes has to be stopped.
process_under() { # $1 directory prefix -> prints every live process running from it
  local dir="$1" pid exe
  [ -n "$dir" ] || return 0
  for pid in /proc/[0-9]*; do
    exe="$(readlink "$pid/exe" 2>/dev/null || true)"
    [ -n "$exe" ] || continue
    case "$exe" in "$dir"/*) printf '%s (pid %s)\n' "$exe" "${pid##*/}" ;; esac
  done
  return 0
}

GAME_RUNNING="$(process_under "$R_GAMEDIR")"
if [ -n "$GAME_RUNNING" ]; then
  printf '%s\n' "$GAME_RUNNING" | while IFS= read -r l; do say "running: $l"; done
  stop_uninstall "The Bibites is running from $R_GAMEDIR. Close it first: removing a file the running game has mapped into memory can crash a world mid-save. Nothing was removed."
fi
SIDECAR_RUNNING="$(process_under "$R_KITDIR")"
if [ -n "$SIDECAR_RUNNING" ]; then
  printf '%s\n' "$SIDECAR_RUNNING" | while IFS= read -r l; do say "running: $l"; done
  stop_uninstall "This install's sidecar is running. Run ./stop-multiverse.sh first. Nothing was removed."
fi

# ---------------------------------------------------------------- the plugin

step "the plugin"

remove_recorded "$(flat_get "$REC" plugin.path)" "$(flat_get "$REC" plugin.sha256)" 'the mod'
remove_recorded "$(flat_get "$REC" plugin.configFile)" '' "the plugin's own settings, written by BepInEx"

# ---------------------------------------------------------------- BepInEx

step "BepInEx"

if [ "$(flat_get "$REC" bepInEx.installedByThisInstaller)" != 'true' ]; then
  say "BepInEx was already on this machine before the install, so none of it is touched."
  add_kept "BepInEx : already present before the install; left whole"
else
  i=0
  while :; do
    p="$(flat_get "$REC" "bepInEx.files.$i.path")"
    [ -n "$p" ] || break
    remove_recorded "$R_GAMEDIR/$p" "$(flat_get "$REC" "bepInEx.files.$i.sha256")" 'BepInEx'
    i=$((i + 1))
  done

  # What BepInEx itself writes once the game has run: its own configuration, its
  # log and its cache. They exist because this installer put BepInEx here, so
  # they go with it - but only after every foreign file has been counted.
  #
  # LogOutput.log.1 to .4 are a Windows shape and this script removes them all
  # the same, because a game folder carried between machines can hold them and a
  # file that exists is a file to account for. On Linux the rotated copies never
  # appear: every instance shares LogOutput.log itself, which is the whole of
  # LOCAL-LOGSHRED.
  BEPINEX_DIR="$R_GAMEDIR/BepInEx"
  for generated in LogOutput.log LogOutput.log.1 LogOutput.log.2 LogOutput.log.3 \
                   LogOutput.log.4 config/BepInEx.cfg; do
    remove_recorded "$BEPINEX_DIR/$generated" '' 'written by BepInEx after it was installed'
  done
  if [ -d "$BEPINEX_DIR/cache" ]; then
    while IFS= read -r cached; do
      [ -n "$cached" ] || continue
      remove_recorded "$cached" '' "BepInEx's cache"
    done < <(find "$BEPINEX_DIR/cache" -maxdepth 1 -type f)
  fi

  # Directories last, deepest first, and only while they are empty. Anything left
  # inside one is a file this installer did not create - another mod's plugin, a
  # log somebody kept - and it keeps its directory.
  if [ -d "$BEPINEX_DIR" ]; then
    while IFS= read -r dir; do
      [ -n "$dir" ] || continue
      remove_empty_directory "$dir"
    done < <(find "$BEPINEX_DIR" -mindepth 1 -type d | awk '{ print length($0), $0 }' | sort -rn | cut -d' ' -f2-)
  fi
  remove_empty_directory "$BEPINEX_DIR"
fi

# ---------------------------------------------------------------- the managed runtime

step "the managed game runtime"

if [ "$(flat_get "$REC" runtime.managedByThisInstaller)" != 'true' ]; then
  say "This was the add-on edition, so the game installation is not this package's to remove."
  add_kept "game runtime : external installation; left whole"
else
  R_RUNTIME_ROOT="$(flat_get "$REC" runtime.root)"
  RUNTIME_RECORDED=''
  RUNTIME_SURVIVORS=0
  i=0
  while :; do
    p="$(flat_get "$REC" "runtime.files.$i.path")"
    [ -n "$p" ] || break
    want="$(flat_get "$REC" "runtime.files.$i.sha256")"
    remove_recorded "$p" "$want" 'the complete edition game payload'
    RUNTIME_RECORDED="$RUNTIME_RECORDED$p
"
    # DID ANY FILE THIS INSTALL PUT THERE SURVIVE? That is what decides whether
    # the directory is still somebody's game copy or only what is left of one.
    #
    # It is asked of the hashes rather than of the ledger, and --dry-run is why:
    # under a dry run every recorded file is still on disk, so "it is there"
    # means nothing. A recorded file that still MATCHES its recorded hash is one
    # this run removes (or would); one that is there and DIFFERS is one the
    # ledger has just reported CHANGED and kept, and that is a survivor.
    if [ -f "$p" ] && [ "$(sha256_of "$p")" != "$(printf '%s' "$want" | tr 'a-f' 'A-F')" ]; then
      RUNTIME_SURVIVORS=$((RUNTIME_SURVIVORS + 1))
    fi
    i=$((i + 1))
  done

  if [ -n "$R_RUNTIME_ROOT" ] && [ -d "$R_RUNTIME_ROOT" ]; then
    # What is in the directory that this install never recorded: BepInEx's log,
    # its configuration and its cache, whatever else the game wrote while it ran.
    RUNTIME_RESIDUE=0
    while IFS= read -r f; do
      [ -n "$f" ] || continue
      if printf '%s\n' "$RUNTIME_RECORDED" | grep -qxF -- "$f"; then continue; fi
      RUNTIME_RESIDUE=$((RUNTIME_RESIDUE + 1))
    done < <(find "$R_RUNTIME_ROOT" -type f)

    if [ "$RUNTIME_SURVIVORS" -eq 0 ] && [ "$RUNTIME_RESIDUE" -gt 0 ] &&
       is_managed_runtime "$R_RUNTIME_ROOT" "$R_DATAROOT"; then
      # THE MANAGED GAME COPY IS RECLAIMED WHOLE, RESIDUE AND ALL. Not one file
      # this install recorded is left in <data root>/runtimes/<sha>, so what is
      # in it is no longer a game: it is BepInEx's log, its configuration, its
      # cache, and whatever else ran there. Leaving that behind is what broke the
      # next install - it found a directory with no game in it, refused to
      # overwrite it, and named a file nobody could put back. This directory is
      # this package's own and holds nothing of yours; your worlds are the game's
      # own saves elsewhere, and the journal, the logs and the credential are
      # OUTSIDE it in the data root, which this script keeps unless you ask for
      # --remove-world-data.
      #
      # A SURVIVING RECORDED FILE STOPS THIS. A game file somebody changed is
      # reported CHANGED and kept above, and then the directory around it keeps
      # itself too: this sweep is for rubble, never for a copy something in it is
      # still vouching for.
      say "nothing this install recorded is left in $R_RUNTIME_ROOT, and $RUNTIME_RESIDUE file(s) the"
      say "game and BepInEx wrote while it ran are. That directory is this package's own copy of the"
      say "game, so it goes whole - a game directory with no game in it serves nobody and is exactly"
      say "what an install over it cannot repair."
      [ "$DRY_RUN" -eq 1 ] || rm -rf "$R_RUNTIME_ROOT"
      add_removed "$R_RUNTIME_ROOT   ($RUNTIME_RESIDUE file(s) created by the game and BepInEx after the install, and the managed game copy around them)"
    else
      # Remove directories only when empty. A game file somebody changed or any
      # file somebody added keeps itself and every parent it needs.
      while IFS= read -r -d '' dir; do
        remove_empty_directory "$dir"
      done < <(find "$R_RUNTIME_ROOT" -depth -type d -print0)
    fi
  fi
  remove_empty_directory "$(dirname "$R_RUNTIME_ROOT")"
fi

# ---------------------------------------------------------------- the certificate

step "the certificate authority"

R_CA="$(flat_get "$REC" certificate.storedCopy)"
say "This installer wrote nothing to any trust store, so there is nothing to take out."
say "A public map's relay uses an authority this machine already trusts through"
say "/etc/ssl/certs, and that one is not this package's to add or remove. A private"
say "map's authority was trusted through SSL_CERT_FILE in the start script, for that"
say "one process - so removing the copy below and the start script is the whole of"
say "the removal, and no other program on this machine ever saw it."
if [ -n "$R_CA" ]; then
  remove_recorded "$R_CA" '' "the copy of the map's certificate authority"
fi

# ---------------------------------------------------------------- this install's own files

step "this install's own files"

i=0
while :; do
  g="$(flat_get "$REC" "generated.$i")"
  [ -n "$g" ] || break
  remove_recorded "$g" '' 'written by the installer'
  i=$((i + 1))
done
R_PENDING="$R_DATAROOT/enrollment-pending.json"
if [ "$REMOVE_WORLD_DATA" -eq 1 ]; then
  remove_recorded "$R_CREDENTIAL" '' 'your map credential, with the world data'
  if [ -f "$R_PENDING" ]; then
    remove_recorded "$R_PENDING" '' 'a half-finished enrollment, with the world data'
  fi
else
  # THE CREDENTIAL IS THE WORLD'S, NOT THE INSTALL'S. The map keeps a verifier
  # and cannot print the secret again, so deleting it here would end the world on
  # the map for a person who is only replacing a build - and installing again
  # over this data root reuses it rather than spending another identity. It goes
  # with the journal, under --remove-world-data, and never on its own.
  add_kept "identity: $R_CREDENTIAL - the world $R_PEERID keeps its place on the map, and installing again here reuses it"
  if [ -f "$R_PENDING" ]; then
    # It holds a second copy of a secret, so it is never kept silently.
    add_kept "pending : $R_PENDING - a half-finished enrollment, and a second copy of a secret. Installing again finishes it; --remove-world-data removes it"
  fi
fi
for leftover in sidecar.pid game.pid; do
  remove_recorded "$R_DATAROOT/$leftover" ''
done

if [ "$REMOVE_WORLD_DATA" -eq 1 ]; then
  cat <<'WARNING'

  --remove-world-data: the journal and this world's identity go too.
  The journal is this machine's record of every organism it took custody of and
  has not handed on. Nobody else holds a copy, and no operator command can reach
  it. Deleting it drops whatever it still held. Your worlds are NOT in it and are
  not affected.
  The credential goes with it, and that is the end of this world on the map: the
  relay keeps a verifier and cannot print that secret again. Its slot stays
  reserved until you ask the operator to release it or hand it on.

WARNING
  for dir in "$R_DATADIR" "$R_LOGDIR"; do
    if [ -n "$dir" ] && [ -d "$dir" ]; then
      [ "$DRY_RUN" -eq 1 ] || rm -rf "$dir"
      add_removed "$dir   (recursively, by request)"
    fi
  done
else
  add_kept "journal : $R_DATADIR - custody of organisms this world still holds"
  add_kept "logs    : $R_LOGDIR"
fi

[ "$DRY_RUN" -eq 1 ] || rm -f "$RECORD_FILE"
add_removed "$RECORD_FILE   (the install record itself, last)"
if [ "$REMOVE_WORLD_DATA" -eq 0 ]; then
  say "the journal, the logs and this world's identity stay. Installing again over"
  say "$R_DATAROOT reuses that identity. Pass --remove-world-data to remove them as well."
else
  remove_empty_directory "$R_DATAROOT"
fi

# ---------------------------------------------------------------- the ledger

printf '\n==== what was removed\n'
printf '%s' "$REMOVED" | while IFS= read -r l; do [ -n "$l" ] && say "$l"; done
printf '\n==== what was kept, and why\n'
printf '%s' "$KEPT" | while IFS= read -r l; do [ -n "$l" ] && say "$l"; done
printf '\n'
say "Untouched, in every run of this script: your worlds and their backups, under"
say "\${XDG_CONFIG_HOME:-\$HOME/.config}/unity3d/The Bibites/The Bibites. This package"
say "never wrote outside its own directory there, and this script never reads them."
printf '\n'
if [ "$DRY_RUN" -eq 1 ]; then
  printf 'Nothing was changed. Run it again without --dry-run to do it.\n'
else
  if [ "$(flat_get "$REC" runtime.managedByThisInstaller)" = 'true' ]; then
    printf 'Done. Every unchanged managed game file was removed.\n'
    say "A changed or user-added runtime file remains in place, if the ledger above names one,"
    say "and keeps the directory around it. With nothing of this install's left in that directory,"
    say "the managed game copy goes whole - what the game wrote inside it included."
  else
    printf 'Done. The game is as the installer found it.\n'
    say "You can prove that: the itch.io archive's SHA-256 is published in"
    say "docs/support-matrix.md, so unpacking it again beside this folder and diffing"
    say "the two trees is a check nobody has to take on trust."
  fi
fi
printf '\n'
say "Leaving a map is a separate act from uninstalling, and it is one message to the"
say "map's operator. Until they release your place, the map keeps it for you and every"
say "organism addressed to it waits out its hold. See docs/participant/leave.md."
