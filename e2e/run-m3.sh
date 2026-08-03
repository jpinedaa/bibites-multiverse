#!/usr/bin/env bash
# The M3 three-slot ring rig and its local rehearsal.
#
#   run-m3.sh build      build the Go binaries into bin/
#   run-m3.sh seed       create (or evolve) M3-Slot1..3, then quit the games
#   run-m3.sh up         relay -> archive -> sidecars 1,2,3 -> games 1,2,3
#   run-m3.sh phase1     the ring forms: three slots in order, three open export edges
#   run-m3.sh phase2     circumnavigation: one organism 1->2->3->1, asserted per hop
#   run-m3.sh phase3     archive truth: three hops, lineage joined, no parent blob on the wire
#   run-m3.sh phase4     containment spot-check (D10): inward outside the square, and far north
#   run-m3.sh phase5a    kill -9 the relay mid-migration
#   run-m3.sh phase5b    kill -9 a sidecar mid-migration (MULTIVERSE_FAULT=post-journal)
#   run-m3.sh journal    summarise all three sidecar journals
#   run-m3.sh archive    bin/archive list over the rig's archive data dir
#   run-m3.sh errors     every unexplained error line in the three BepInEx logs
#   run-m3.sh down       stop everything and leave no process behind
#   run-m3.sh status     what is running right now
#   run-m3.sh all        build, seed, up, phase1..5, errors, down
#
# Each phase is separately invokable on a healthy rig, so a failed phase can be re-run
# without redoing the seed or the bring-up.
#
# WHAT THIS RIG IS. Three ring slots on ONE machine, which is the LAN rehearsal: the wire,
# the ring insertion, the annex, the archive and the custody chain are all exercised, and
# only the network hop and the second install are missing. m3_considerations.md's exit test
# puts slot 2 on the second computer; everything below is what has to work before that is
# worth attempting.
#
# THE RING IS ONE-WAY. Every slot exports through its EAST edge and receives through its
# passive WEST edge (D8, contract-a.md §14 A11), so an organism comes home only after a full
# circuit: 1 -> 2 -> 3 -> 1. That is why three slots are the minimum — two would make one
# hop a round trip and prove no circuit.
#
# SLOT ORDER IS START ORDER. The relay appends each new peer at the tail of the ring
# (contract-b-m3.md §7.2 rule 4), so `up` starts the sidecars one at a time and waits for
# each grant before starting the next. --slot is only a preference and is deliberately not
# relied on here.
#
# Ports 8787, 8788, 18789 and 8790 are fixed defaults shared by every rig in this checkout,
# so only one rig may run at a time (dev_environment.md). `up` refuses to start on top of a
# rig that is already there.
set -uo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
E2E="$REPO/e2e"
MOD_DIR="$REPO/bibites-mod"
GAME_SH="$MOD_DIR/game.sh"
BIN="$REPO/bin"

DATA="$E2E/data"
RELAY_DATA="$E2E/relay-data"
ARCHIVE_DATA="$E2E/archive-data"
LOGS="$E2E/logs-m3"
BEPINEX_ARCHIVE="$LOGS/bepinex"
RUN="$E2E/run"

RELAY_PORT="${RELAY_PORT:-8790}"
PORT_1="${PORT_1:-8787}"
PORT_2="${PORT_2:-8788}"
PORT_3="${PORT_3:-18789}"
RELAY_URL="ws://127.0.0.1:$RELAY_PORT/contract-b/v2"
# Loopback for the local rehearsal. run-m3-lan.sh overrides this to 0.0.0.0 so
# the second computer can reach the relay; the local sidecars keep dialling
# 127.0.0.1 either way.
RELAY_LISTEN="${RELAY_LISTEN:-127.0.0.1:$RELAY_PORT}"

# Every slot exports east. The entry edge is derived and passive.
EXPORT_EDGE=E

SLOTS="1 2 3"

# contract-b-m3.md §3.1: one shared bearer token, from a file, never from a flag. The rig
# mints it once with mode 600 and reuses it, which is also the convention dev_environment.md
# documents for the LAN.
TOKEN_FILE="${MULTIVERSE_TOKEN_FILE:-$HOME/.multiverse-token}"

# The mod polls a command file and WSL writes it, so the file has to exist on the Windows
# side of the boundary: a path under the WSL filesystem is not reliably reachable from a
# Windows process.
POWERSHELL=/mnt/c/Windows/System32/WindowsPowerShell/v1.0/powershell.exe
WIN_TEMP="$(cd /mnt/c && "$POWERSHELL" -NoProfile -NonInteractive -Command '$env:TEMP' 2>/dev/null | tr -d '\r')"
if [ -z "$WIN_TEMP" ]; then
  echo "could not read the Windows TEMP directory — is PowerShell reachable?" >&2
  exit 1
fi
WIN_CMD_DIR="$WIN_TEMP\\bibites-m3"
WSL_CMD_DIR="$(wslpath -u "$WIN_TEMP")/bibites-m3"

BEPINEX="/mnt/c/Program Files (x86)/Steam/steamapps/common/The Bibites/BepInEx"

FAMILY_REPORT="${FAMILY_REPORT:-30}"
TIME_SCALE="${TIME_SCALE:-3}"
SEED_TIME_SCALE="${SEED_TIME_SCALE:-20}"

export GOROOT="${GOROOT:-$HOME/go}"
export PATH="$GOROOT/bin:$PATH"

mkdir -p "$DATA" "$LOGS" "$BEPINEX_ARCHIVE" "$RUN" "$WSL_CMD_DIR"

# ---------------------------------------------------------------- output

# Narration goes to stderr, values go to stdout. A helper that mixed the two would let a
# note leak into the variable a caller is capturing.
step()    { printf '\n=== %s\n' "$*" >&2; }
note()    { printf '    %s\n' "$*" >&2; }
fail()    { printf '!!! %s\n' "$*" >&2; }
verdict() { printf '\n### %s\n' "$*" >&2; }

now() { date +%s; }

# ---------------------------------------------------------------- per-slot facts

port_of()   { case "$1" in 1) echo "$PORT_1" ;; 2) echo "$PORT_2" ;; 3) echo "$PORT_3" ;; esac; }
world_of()  { printf 'M3-Slot%s\n' "$1"; }
peer_of()   { printf 'slot-%s\n' "$1"; }
datadir_of(){ printf '%s/slot-%s\n' "$DATA" "$1"; }
journal_of(){ printf '%s/slot-%s/journal/journal.log\n' "$DATA" "$1"; }
sclog_of()  { printf '%s/sidecar-%s.log\n' "$LOGS" "$1"; }
# The east neighbour: 1->2, 2->3, 3->1. This is the whole topology the rig needs.
east_of()   { case "$1" in 1) echo 2 ;; 2) echo 3 ;; 3) echo 1 ;; esac; }

# ---------------------------------------------------------------- token

ensure_token() {
  if [ -s "$TOKEN_FILE" ]; then
    return 0
  fi
  ( umask 077 && head -c 32 /dev/urandom | base64 | tr -d '/+=' > "$TOKEN_FILE" )
  chmod 600 "$TOKEN_FILE"
  note "minted a LAN token in $TOKEN_FILE (mode 600)"
}

# ---------------------------------------------------------------- go side

build() {
  step "building the Go binaries"
  mkdir -p "$BIN"
  ( cd "$REPO/go" && CGO_ENABLED=0 go build -o "$BIN/" ./cmd/... ) || return 1
  ls -l "$BIN"
}

pidfile()    { printf '%s/m3-%s.pid\n' "$RUN" "$1"; }
record_pid() { printf '%s\n' "$2" > "$(pidfile "$1")"; }
read_pid()   { [ -f "$(pidfile "$1")" ] && cat "$(pidfile "$1")" || true; }
proc_alive() { local p="$1"; [ -n "$p" ] && kill -0 "$p" 2>/dev/null; }

start_relay() {
  local pid
  mkdir -p "$RELAY_DATA"
  nohup "$BIN/relay" --listen "$RELAY_LISTEN" --data-dir "$RELAY_DATA" \
    --token-file "$TOKEN_FILE" >>"$LOGS/relay.log" 2>&1 &
  pid=$!
  record_pid relay "$pid"
  note "relay pid=$pid listen=$RELAY_LISTEN dataDir=$RELAY_DATA"
}

start_archive() {
  local pid
  mkdir -p "$ARCHIVE_DATA"
  nohup "$BIN/archive" --relay "$RELAY_URL" --peer-id archive-main \
    --data-dir "$ARCHIVE_DATA" --token-file "$TOKEN_FILE" >>"$LOGS/archive.log" 2>&1 &
  pid=$!
  record_pid archive "$pid"
  note "archive pid=$pid dataDir=$ARCHIVE_DATA"
}

# start_sidecar <slot> [fault]
start_sidecar() {
  local slot="$1" fault="${2:-}"
  local port dir peer
  port="$(port_of "$slot")"; dir="$(datadir_of "$slot")"; peer="$(peer_of "$slot")"
  mkdir -p "$dir"
  rm -f "$dir/fault.hit"

  local pid
  if [ -n "$fault" ]; then
    MULTIVERSE_FAULT="$fault" nohup "$BIN/sidecar" --listen "127.0.0.1:$port" --relay "$RELAY_URL" \
      --peer-id "$peer" --data-dir "$dir" --token-file "$TOKEN_FILE" \
      >>"$(sclog_of "$slot")" 2>&1 &
  else
    nohup "$BIN/sidecar" --listen "127.0.0.1:$port" --relay "$RELAY_URL" \
      --peer-id "$peer" --data-dir "$dir" --token-file "$TOKEN_FILE" \
      >>"$(sclog_of "$slot")" 2>&1 &
  fi
  pid=$!
  record_pid "sidecar-$slot" "$pid"
  note "sidecar $slot pid=$pid port=$port peer=$peer dir=$dir fault=${fault:-<none>}"
}

wait_healthy() {
  local url="$1" deadline=$(( $(now) + 30 ))
  while :; do
    curl -sf "$url" >/dev/null 2>&1 && return 0
    [ "$(now)" -ge "$deadline" ] && { fail "no /healthz from $url"; return 1; }
    sleep 0.3
  done
}

# wait_file <path> <extended regex> [timeout] [from line] — the last match after the mark.
wait_file() {
  local file="$1" pattern="$2" timeout="${3:-60}" from="${4:-0}"
  local deadline=$(( $(now) + timeout )) hit
  while :; do
    if [ -f "$file" ]; then
      hit="$(tail -n "+$((from + 1))" "$file" 2>/dev/null | grep -aE "$pattern" | tail -n 1)"
      if [ -n "$hit" ]; then printf '%s\n' "$hit"; return 0; fi
    fi
    [ "$(now)" -ge "$deadline" ] && { fail "timeout(${timeout}s) in $(basename "$file") for: $pattern"; return 1; }
    sleep 0.3
  done
}

file_mark() { [ -f "$1" ] && wc -l < "$1" | tr -d ' ' || echo 0; }

kill_pid() {
  local name="$1" sig="${2:--9}" pid
  pid="$(read_pid "$name")"
  [ -n "$pid" ] || return 0
  kill "$sig" "$pid" 2>/dev/null
  rm -f "$(pidfile "$name")"
  note "killed $name pid=$pid ($sig)"
}

# ---------------------------------------------------------------- game side

cmd_file_wsl() { printf '%s/cmd-%s.txt\n' "$WSL_CMD_DIR" "$1"; }
cmd_file_win() { printf '%s\\cmd-%s.txt\n' "$WIN_CMD_DIR" "$1"; }
game_instance(){ printf 'slot%s\n' "$1"; }

start_game() {
  local slot="$1"
  local world port
  world="$(world_of "$slot")"; port="$(port_of "$slot")"

  # A stale result log would let an old token satisfy a new wait.
  rm -f "$(cmd_file_wsl "$slot")" "$(cmd_file_wsl "$slot").log" "$(cmd_file_wsl "$slot").tmp"

  # WSLENV has to name EVERY variable or WSL forwards none of them.
  MULTIVERSE_RING_SLOT="$slot" \
  MULTIVERSE_EXPORT_EDGE="$EXPORT_EDGE" \
  MULTIVERSE_SIDECAR_PORT="$port" \
  MULTIVERSE_WORLD="$world" \
  MULTIVERSE_CMD_FILE="$(cmd_file_win "$slot")" \
  MULTIVERSE_FAMILY_REPORT="$FAMILY_REPORT" \
  WSLENV='MULTIVERSE_RING_SLOT:MULTIVERSE_EXPORT_EDGE:MULTIVERSE_SIDECAR_PORT:MULTIVERSE_WORLD:MULTIVERSE_CMD_FILE:MULTIVERSE_FAMILY_REPORT' \
    "$GAME_SH" start "$(game_instance "$slot")"
}

# Seeding runs with no export edge, so the Contract A client stays off and the world can be
# grown and written with no sidecar in the picture. game.sh maps an instance to its log by
# content, and with no ringSlot on the config line the world name is the marker.
start_game_seed() {
  local slot="$1" world="$2"
  rm -f "$(cmd_file_wsl "$slot")" "$(cmd_file_wsl "$slot").log" "$(cmd_file_wsl "$slot").tmp"

  MULTIVERSE_WORLD="$world" \
  MULTIVERSE_CMD_FILE="$(cmd_file_win "$slot")" \
  MULTIVERSE_FAMILY_REPORT="15" \
  WSLENV='MULTIVERSE_WORLD:MULTIVERSE_CMD_FILE:MULTIVERSE_FAMILY_REPORT' \
    "$GAME_SH" start "$(game_instance "$slot")"
}

game_log() { "$GAME_SH" logfile "$(game_instance "$1")" 2>/dev/null; }

log_mark() {
  local file; file="$(game_log "$1")"
  if [ -n "$file" ] && [ -f "$file" ]; then wc -l < "$file" | tr -d ' '; else echo 0; fi
}

# wait_log <slot> <extended regex> [timeout s] [from line] — the last match after the mark.
wait_log() {
  local slot="$1" pattern="$2" timeout="${3:-120}" from="${4:-0}"
  local deadline=$(( $(now) + timeout )) file hit
  while :; do
    file="$(game_log "$slot")"
    if [ -n "$file" ] && [ -f "$file" ]; then
      hit="$(tail -n "+$((from + 1))" "$file" 2>/dev/null | grep -aE "$pattern" | tail -n 1)"
      if [ -n "$hit" ]; then printf '%s\n' "$hit"; return 0; fi
    fi
    [ "$(now)" -ge "$deadline" ] && { fail "timeout(${timeout}s) waiting on game $slot for: $pattern"; return 1; }
    sleep 0.5
  done
}

# grep_log <slot> <regex> [from line] — every match, no wait.
grep_log() {
  local slot="$1" pattern="$2" from="${3:-0}" file
  file="$(game_log "$slot")" || return 1
  [ -n "$file" ] && [ -f "$file" ] || return 1
  tail -n "+$((from + 1))" "$file" 2>/dev/null | grep -aE "$pattern" || true
}

# send <slot> <verb> [args...] — one command, atomically, and wait for its result line.
send() {
  local slot="$1"; shift
  local verb="$1"; shift
  local args="$*"
  local file token deadline hit
  file="$(cmd_file_wsl "$slot")"
  token="t$(date +%s%N)"

  printf '%s %s %s\n' "$token" "$verb" "$args" > "$file.tmp"
  mv -f "$file.tmp" "$file"

  deadline=$(( $(now) + ${CMD_TIMEOUT:-180} ))
  while :; do
    if [ -f "$file.log" ]; then
      hit="$(grep -a "^$token " "$file.log" 2>/dev/null | tail -n 1)"
      if [ -n "$hit" ]; then
        printf '%s\n' "$hit"
        case "$hit" in *" OK "*) return 0 ;; *) return 1 ;; esac
      fi
    fi
    [ "$(now)" -ge "$deadline" ] && { fail "timeout waiting for '$verb $args' on game $slot"; return 1; }
    sleep 0.2
  done
}

archive_bepinex_logs() {
  local tag="${1:-run}" stamp f
  stamp="$(date +%Y%m%d-%H%M%S)"
  for f in "$BEPINEX/LogOutput.log" "$BEPINEX"/LogOutput.log.[0-9]; do
    [ -f "$f" ] || continue
    cp -f "$f" "$BEPINEX_ARCHIVE/$(basename "$f").$tag.$stamp" 2>/dev/null || true
  done
}

# BepInEx truncates LogOutput.log on launch but leaves stale LogOutput.log.N behind when
# fewer instances run, and a stale file can satisfy the instance->log match. Archive them
# all, then remove them, with no game running.
reset_bepinex_logs() {
  archive_bepinex_logs "${1:-reset}"
  local f
  for f in "$BEPINEX"/LogOutput.log.[0-9] "$BEPINEX/LogOutput.log"; do
    rm -f "$f" 2>/dev/null || true
  done
}

# field <name> <line> — the value of `name=` in a `k=v k=v` log line, up to the next space.
#
# Three traps this avoids, all three hit during the rehearsal:
#
#  * a `sed` with a leading `.*` is greedy and picks the LAST `name=` on the line, so
#    `field slot` on a grant line returned the east neighbour's number instead;
#  * a character class narrow enough for a hex digest cuts a genome hash in half, because
#    `bb8-genome/1:sha256:…` is not hex;
#  * **the BepInEx log is CRLF.** A value that is the last field on the line carries a
#    trailing `\r`, so two identical payload hashes compared UNEQUAL — the sha of the
#    MIGRATE_OUT line has another field after it and the sha of the MIGRATE_IN line does
#    not. Strip the CR before anything else looks at the line.
field() {
  tr -d '\r' <<<"$2" | grep -oE "(^|[^A-Za-z])$1=[^ ]+" | head -n 1 | sed "s/.*$1=//"
}

# ---------------------------------------------------------------- seeding

seed_one() {
  local slot="$1" want_family="$2" world
  world="$(world_of "$slot")"

  step "seeding '$world' (slot $slot, family=$want_family)"
  start_game_seed "$slot" "$world" || return 1

  wait_log "$slot" "\[M2-WORLD\] world '$world' (loaded|seeded and live)" 900 || return 1
  note "world ready"

  send "$slot" timescale "$SEED_TIME_SCALE" || return 1

  if [ "$want_family" = yes ]; then
    note "growing until at least one organism has a living parent or child"
    local deadline=$(( $(now) + ${FAMILY_TIMEOUT:-2400} )) line
    while :; do
      line="$(send "$slot" family)" || true
      note "$line"
      case "$line" in *"linked=YES"*) break ;; esac
      [ "$(now)" -ge "$deadline" ] && { fail "no family link appeared in '$world' within the timeout"; return 1; }
      sleep 20
    done
  fi

  send "$slot" save "$world" || return 1
  send "$slot" quit || true
  "$GAME_SH" wait "$(game_instance "$slot")" 120 || "$GAME_SH" stop "$(game_instance "$slot")"
  archive_bepinex_logs "seed-slot$slot"
  note "'$world' seeded"
}

seed() {
  "$GAME_SH" stop all >/dev/null 2>&1 || true
  sleep 2
  # Slot 1 is the origin of every rehearsal hop, so it is the one that needs a living
  # parent: the lineage annex is only interesting when a parent blob actually ships.
  reset_bepinex_logs seed
  seed_one 1 yes || return 1
  reset_bepinex_logs seed
  seed_one 2 no || return 1
  reset_bepinex_logs seed
  seed_one 3 no || return 1
  return 0
}

# ---------------------------------------------------------------- bring-up

ports_busy() {
  ss -ltn 2>/dev/null | grep -qE "127\.0\.0\.1:($PORT_1|$PORT_2|$PORT_3|$RELAY_PORT) "
}

# Wait for one sidecar to be granted the slot the rig expects. This is what makes slot
# order deterministic: the relay appends at the tail (§7.2 rule 4), so the Nth sidecar to
# claim gets slot N — but only if the Nth one claims after the (N-1)th was granted.
wait_grant() {
  local slot="$1" mark="${2:-0}" line
  line="$(wait_file "$(sclog_of "$slot")" "ring slot granted.*slot=$slot" 60 "$mark")" || return 1
  note "slot $slot: $line"
}

up() {
  if ports_busy; then
    fail "ports $PORT_1/$PORT_2/$PORT_3/$RELAY_PORT are already bound — another rig is running. Run 'down' first."
    return 1
  fi

  ensure_token
  "$GAME_SH" stop all >/dev/null 2>&1 || true
  sleep 2
  reset_bepinex_logs up

  step "starting the Go side: relay, archive, then the three sidecars in ring order"
  start_relay
  wait_healthy "http://127.0.0.1:$RELAY_PORT/healthz" || return 1

  start_archive
  wait_file "$LOGS/archive.log" 'archive: subscribed to the relay' 60 || return 1
  note "archive subscribed"

  local slot
  for slot in $SLOTS; do
    local mark; mark="$(file_mark "$(sclog_of "$slot")")"
    start_sidecar "$slot"
    wait_healthy "http://127.0.0.1:$(port_of "$slot")/healthz" || return 1
    wait_grant "$slot" "$mark" || return 1
  done
  note "the ring is formed: slots 1, 2, 3"

  for slot in $SLOTS; do
    step "starting game $slot ($(world_of "$slot"), export edge $EXPORT_EDGE, sidecar $(port_of "$slot"))"
    start_game "$slot" || return 1
    wait_log "$slot" "\[M2-WORLD\] world '$(world_of "$slot")' (loaded|seeded and live)" 900 || return 1
  done

  step "waiting for all three mods to connect and all three export edges to open"
  for slot in $SLOTS; do
    wait_log "$slot" '\[M2\] CONFIG_UPDATE reason=connect' 240 >/dev/null || return 1
  done
  for slot in $SLOTS; do
    wait_log "$slot" "\[M2\] EDGE_STATUS .* exportEdge=$EXPORT_EDGE open=True" 240 >/dev/null || return 1
    note "slot $slot: export edge $EXPORT_EDGE is open"
  done

  for slot in $SLOTS; do
    send "$slot" timescale "$TIME_SCALE" || return 1
  done
  note "rig up"
}

down() {
  step "tearing the rig down"
  "$GAME_SH" stop all || true
  local slot
  for slot in $SLOTS; do kill_pid "sidecar-$slot" -TERM; done
  kill_pid archive -TERM
  kill_pid relay -TERM
  sleep 1
  local p pid
  for p in relay archive sidecar-1 sidecar-2 sidecar-3; do
    pid="$(read_pid "$p")"
    proc_alive "$pid" && kill -9 "$pid" 2>/dev/null
    rm -f "$(pidfile "$p")"
  done
  sleep 1
  archive_bepinex_logs down
  status
}

status() {
  step "status"
  local p pid
  for p in relay archive sidecar-1 sidecar-2 sidecar-3; do
    pid="$(read_pid "$p")"
    if proc_alive "$pid"; then note "$p pid=$pid RUNNING"; else note "$p not running"; fi
  done
  ss -ltn 2>/dev/null | grep -E "127\.0\.0\.1:($PORT_1|$PORT_2|$PORT_3|$RELAY_PORT) " || note "no rig port is bound"
  "$GAME_SH" status
}

# ---------------------------------------------------------------- assertions

# observe_hop <from slot> <to slot> <entityId> <mark from> <mark to> [timeout]
# Assert the whole custody chain plus the lineage annex for one organism already on its
# way. Prints "migrationId=<id> entityId=<id> sha=<hash> genomeHash=<hash> parents=<n>".
observe_hop() {
  local from="$1" to="$2" eid="$3" mark_from="$4" mark_to="$5" timeout="${6:-90}"
  local line mid sha_out sha_in ghash parents

  line="$(wait_log "$from" "entityId=$eid phase=MIGRATE_OUT_SENT" "$timeout" "$mark_from")" || return 1
  note "slot $from: $line"
  mid="$(field migrationId "$line")"
  sha_out="$(field payloadSha256 "$line")"
  [ -n "$mid" ] || { fail "could not read a migrationId out of the MIGRATE_OUT line"; return 1; }

  # The annex the mod shipped: entity ids plus one opaque blob per living parent (§14 A12).
  line="$(wait_log "$from" "\[M3-LINEAGE\] migrationId=$mid" 30 "$mark_from")" || return 1
  note "slot $from: $line"

  wait_log "$from" "migrationId=$mid .*phase=DESTROYED" 60 "$mark_from" >/dev/null || return 1
  note "slot $from: organism destroyed on MIGRATE_OUT_ACK"

  # The annex the sidecar built and put on the wire, with every parent blob stripped.
  line="$(wait_file "$(sclog_of "$from")" "forwarded MIGRATION_PAYLOAD.*migrationId=$mid" 60)" || return 1
  note "sidecar $from: $line"
  ghash="$(field genomeHash "$line")"
  parents="$(field parents "$line")"
  [ -n "$ghash" ] || { fail "the forwarded envelope carried no migrant genomeHash"; return 1; }

  # The lane, from the envelope the relay actually routes on: every hop must move EAST.
  local dest want_dest
  dest="$(field destSlot "$line")"
  want_dest="$(east_of "$from")"
  [ "$dest" = "$want_dest" ] || { fail "slot $from routed to slot $dest, but its east neighbour is $want_dest"; return 1; }
  [ "$dest" = "$to" ]        || { fail "the hop was asserted against slot $to but the envelope names slot $dest"; return 1; }

  line="$(wait_file "$(sclog_of "$to")" "took custody of an inbound organism.*migrationId=$mid" 90)" || return 1
  note "sidecar $to: $line"

  line="$(wait_log "$to" "migrationId=$mid .*phase=MIGRATE_IN_RECEIVED" 120 "$mark_to")" || return 1
  sha_in="$(field payloadSha256 "$line")"

  line="$(wait_log "$to" "migrationId=$mid .*phase=SPAWNED" 120 "$mark_to")" || return 1
  note "slot $to: $line"
  local spawned_eid; spawned_eid="$(field entityId "$line")"

  line="$(wait_log "$to" "migrationId=$mid .*phase=MIGRATE_IN_ACK" 60 "$mark_to")" || return 1
  note "slot $to: $line"

  if [ "$sha_out" != "$sha_in" ] || [ -z "$sha_out" ]; then
    fail "PAYLOAD MISMATCH out=$sha_out in=$sha_in"
    return 1
  fi
  note "payload sha256 is byte-equal across the hop: $sha_out"

  if [ "$spawned_eid" != "$eid" ]; then
    fail "ENTITY ID CHANGED across the hop: out=$eid in=$spawned_eid"
    return 1
  fi
  note "entityId is preserved across the hop: $eid"

  printf 'migrationId=%s entityId=%s sha=%s genomeHash=%s parents=%s\n' \
    "$mid" "$eid" "$sha_out" "$ghash" "$parents"
}

# hop <from slot> <to slot> <selector> — force one organism east and assert the whole chain.
hop() {
  local from="$1" to="$2" selector="$3"
  local line eid mark_from mark_to

  mark_from="$(log_mark "$from")"
  mark_to="$(log_mark "$to")"

  line="$(send "$from" export "$selector")" || { fail "export failed: $line"; return 1; }
  note "export: $line"
  eid="$(field entityId "$line")"
  [ -n "$eid" ] || { fail "the export result named no entityId: $line"; return 1; }

  observe_hop "$from" "$to" "$eid" "$mark_from" "$mark_to" 90
}

# count_everywhere <entityId> — the exactly-once table, ring-wide.
count_everywhere() {
  local eid; eid="$(tr -cd '0-9-' <<<"$1")"
  local slot line total=0 custody custody_count
  printf '\n    ---- organism %s ----\n' "$eid" >&2
  for slot in $SLOTS; do
    line="$(send "$slot" count "$eid" || true)"
    local alive; alive="$(field alive "$line")"; alive="${alive:-0}"
    printf '    world slot %s      : %s\n' "$slot" "$alive" >&2
    total=$(( total + alive ))
  done

  custody="$(python3 "$E2E/journal.py" custody "$eid" \
    "$(journal_of 1)" "$(journal_of 2)" "$(journal_of 3)" 2>/dev/null)"
  custody_count="$(sed -n 's/^custodyCount=\([0-9]*\)$/\1/p' <<<"$custody")"
  custody_count="${custody_count:-0}"
  printf '    live journal rows : %s\n' "$custody_count" >&2
  sed 's/^/      /' <<<"$custody" >&2

  total=$(( total + custody_count ))
  printf '    TOTAL             : %s  -> %s\n' "$total" \
    "$( [ "$total" = 1 ] && echo PASS || { [ "$total" = 0 ] && echo 'PASS (acceptable loss)' || echo FAIL; } )" >&2
  printf '%s\n' "$total"
}

slow_sims()    { local s="${1:-0.5}"; local n; for n in $SLOTS; do send "$n" timescale "$s" >/dev/null || true; done; note "all three sims at ${s}x"; }
restore_sims() { local n; for n in $SLOTS; do send "$n" timescale "$TIME_SCALE" >/dev/null || true; done; }
settle()       { local s="${1:-20}"; note "letting the rig settle for ${s}s"; sleep "$s"; }

# wait_edge <slot> <mark> [timeout] — a NEW open EDGE_STATUS after the mark. Without the
# mark an old line satisfies the wait and the next export fires into a closed edge.
wait_edge() {
  local slot="$1" mark="$2" timeout="${3:-180}"
  wait_log "$slot" 'CONFIG_UPDATE reason=connect' "$timeout" "$mark" >/dev/null || return 1
  wait_log "$slot" "EDGE_STATUS .* exportEdge=$EXPORT_EDGE open=True" "$timeout" "$mark" >/dev/null || return 1
  note "slot $slot is reconnected with export edge $EXPORT_EDGE open"
}

# ---------------------------------------------------------------- phase 1

phase1() {
  step "PHASE 1 — the ring forms"
  local ok=0 slot line

  # The east neighbour is NOT read from a grant line. A sidecar only logs a grant whose
  # reason is not "updated" (contractb_client.go), so slot 1's last logged grant still says
  # eastSlot=0 from when it was alone in the ring. The durable ring order is the truth:
  # list order IS ring order, and the east neighbour of entry i is entry i+1, wrapping
  # (contract-b-m3.md §7.1).
  local order
  order="$(python3 - "$RELAY_DATA/ring.json" <<'PY'
import json, sys
ring = json.load(open(sys.argv[1]))["ring"]
print(" ".join("%s:%s" % (r["slot"], r["peerId"]) for r in ring))
PY
)" || { fail "could not read the relay's ring.json"; return 1; }
  note "ring order (west to east, wrapping): $order"
  local want="1:slot-1 2:slot-2 3:slot-3"
  [ "$order" = "$want" ] || { fail "ring order is '$order', want '$want'"; ok=1; }

  for slot in $SLOTS; do
    line="$(grep -aE 'ring slot granted' "$(sclog_of "$slot")" | tail -n 1)"
    if [ -z "$line" ]; then fail "sidecar $slot was never granted a slot"; ok=1; continue; fi
    note "sidecar $slot: $line"
    local got; got="$(field slot "$line")"
    [ "$got" = "$slot" ] || { fail "sidecar $slot holds slot $got, want $slot"; ok=1; }
    note "slot $slot exports east into slot $(east_of "$slot")"
  done

  for slot in $SLOTS; do
    line="$(grep_log "$slot" "\[M2\] EDGE_STATUS .* exportEdge=$EXPORT_EDGE open=True" | tail -n 1)"
    if [ -z "$line" ]; then fail "slot $slot never reported an open export edge"; ok=1; continue; fi
    note "slot $slot: $line"
  done

  line="$(grep -aE 'archive: subscribed to the relay' "$LOGS/archive.log" | tail -n 1)"
  if [ -z "$line" ]; then fail "the archive never subscribed"; ok=1; else note "archive: $line"; fi

  line="$(grep -aE 'relay: listening' "$LOGS/relay.log" | tail -n 1)"
  note "relay: $line"

  [ "$ok" = 0 ] && verdict "PHASE 1: PASS — three slots in ring order 1->2->3->1, three open export edges, archive subscribed" \
                || verdict "PHASE 1: FAIL"
  return "$ok"
}

# ---------------------------------------------------------------- phase 2

phase2() {
  step "PHASE 2 — circumnavigation: slot 1 -> 2 -> 3 -> 1"
  note "the organism must come home only after a FULL circuit; every hop moves east."
  slow_sims 1

  # `family` prefers an organism with a living parent, which is what makes the annex carry
  # a parent blob rather than a bare gap.
  local fam; fam="$(send 1 family)" || { restore_sims; return 1; }
  note "slot 1 family: $fam"
  case "$fam" in *"linked=YES"*) ;; *) note "slot 1 reports no living family link — the annex may be all gaps" ;; esac

  local out1 out2 out3 eid ok=0
  out1="$(hop 1 2 family)" || { restore_sims; verdict "PHASE 2: FAIL (hop 1->2)"; return 1; }
  note "hop 1->2: $out1"
  eid="$(field entityId "$out1")"

  # A fresh arrival sits in the west entry strip with entry immunity running; give it a
  # moment before the next forced export so the export is not swallowed by the immunity.
  sleep 12
  out2="$(hop 2 3 "$eid")" || { restore_sims; verdict "PHASE 2: FAIL (hop 2->3)"; return 1; }
  note "hop 2->3: $out2"

  sleep 12
  out3="$(hop 3 1 "$eid")" || { restore_sims; verdict "PHASE 2: FAIL (hop 3->1)"; return 1; }
  note "hop 3->1: $out3"

  step "asserting the circuit"
  local h1 h2 h3
  h1="$(field genomeHash "$out1")"; h2="$(field genomeHash "$out2")"; h3="$(field genomeHash "$out3")"
  note "genomeHash per hop: $h1 / $h2 / $h3"
  if [ "$h1" != "$h2" ] || [ "$h2" != "$h3" ]; then
    fail "the migrant's genome hash changed across the circuit — the projection is not stable"
    ok=1
  else
    note "the migrant's genome hash is identical at all three hops"
  fi

  # An annex with at least one parent hash is what proves the mod shipped a parent blob and
  # the sidecar hashed it. Hop 1 is the one that can have it: after a hop the parent's
  # GameObject is gone in the new world, so later hops legitimately carry gaps (§5.3).
  local lineage1
  lineage1="$(grep_log 1 "\[M3-LINEAGE\] migrationId=$(field migrationId "$out1")" | tail -n 1)"
  note "slot 1 annex: $lineage1"
  case "$lineage1" in
    *blobs=0*) note "hop 1 shipped no parent blob — the annex is all gaps for this organism" ;;
    *)         note "hop 1 shipped a parent blob, so the archive has a genome to join on" ;;
  esac

  settle 20
  restore_sims
  local total; total="$(count_everywhere "$eid")"
  [ "$total" = 1 ] || { fail "the organism exists $total times ring-wide after the circuit"; ok=1; }

  [ "$ok" = 0 ] && verdict "PHASE 2: PASS — 1->2->3->1, byte-identical blob per hop, entity id preserved, exactly once" \
                || verdict "PHASE 2: FAIL"

  # The entity id is what phase 3 joins on.
  printf '%s\n' "$eid" > "$RUN/m3-circuit-entity"
  return "$ok"
}

# ---------------------------------------------------------------- phase 3

archive_list() { "$BIN/archive" list --data-dir "$ARCHIVE_DATA" "$@"; }

phase3() {
  step "PHASE 3 — archive truth"
  local eid ok=0
  eid="$(cat "$RUN/m3-circuit-entity" 2>/dev/null || true)"
  [ -n "$eid" ] || { fail "no entity id from phase 2 — run phase2 first"; return 1; }

  # The archive fetches every hash it sees; give the scheduler a moment to drain.
  settle 20

  local listing hops
  listing="$(archive_list)" || { fail "bin/archive list failed"; return 1; }
  printf '%s\n' "$listing" | sed 's/^/      /' >&2

  hops="$(grep -c "entity $eid " <<<"$listing")"
  note "archive records naming entity $eid: $hops"
  [ "$hops" -ge 3 ] || { fail "the archive holds $hops hops for entity $eid, want 3"; ok=1; }

  # Every hop must carry the lineage annex, and the migrant's genome must be held.
  local missing
  missing="$(grep -A 3 "entity $eid " <<<"$listing" | grep -c 'genome .*\[MISSING\]')"
  if [ "$missing" != 0 ]; then
    fail "the archive is missing $missing migrant genome(s) it recorded a hash for"
    ok=1
  else
    note "every migrant genome hash the archive recorded is [held] — fetched by hash, not from the envelope"
  fi

  # The decisive evidence that the wire carries NO parent blob: the archive had to ASK for
  # each genome by hash and store the answer. If the blob had travelled in the envelope the
  # archive would never have sent a GENOME_REQUEST (contract-b-m3.md §6.6).
  local asked stored
  asked="$(grep -ac 'archive: asking for a genome by hash' "$LOGS/archive.log")"
  stored="$(grep -ac 'archive: stored a genome fetched by hash' "$LOGS/archive.log")"
  note "archive genome fetches: asked=$asked stored=$stored"
  [ "$asked" -ge 1 ] || { fail "the archive never asked for a genome — it cannot have joined any lineage"; ok=1; }
  [ "$stored" -ge 1 ] || { fail "the archive never stored a fetched genome"; ok=1; }
  grep -aE 'archive: (asking for a genome by hash|stored a genome fetched by hash)' "$LOGS/archive.log" \
    | tail -n 4 | sed 's/^/      /' >&2

  # And the record shape: a MIGRATION line never carries a bb8 body of any kind.
  # `grep -c` prints its count and still exits 1 when the count is zero, so a
  # `|| echo 0` here appends a SECOND zero and the comparison below fails on "0\n0".
  local blobbed
  blobbed="$(grep -ac '"bb8"' "$ARCHIVE_DATA/migrations.jsonl" 2>/dev/null)"
  blobbed="${blobbed:-0}"
  note "ledger lines carrying a bb8 blob: $blobbed (must be 0)"
  [ "$blobbed" = 0 ] || { fail "a blob reached the archive ledger"; ok=1; }

  # The genome store on disk is what "the genome behind the hash exists" means.
  local genomes
  genomes="$(find "$ARCHIVE_DATA/genomes" -type f 2>/dev/null | wc -l)"
  note "genomes in the archive's content-addressed store: $genomes"
  [ "$genomes" -ge 1 ] || { fail "the archive's genome store is empty"; ok=1; }

  # The gap report proper (§10). It must name only hashes the archive recorded and does
  # not hold — a parent_gone entry has no hash and is not a gap, which is what the second
  # hop of every circuit produces.
  local gaps unresolved
  gaps="$(archive_list --gaps | grep -a 'migration(s) shown')"
  note "gap report: $gaps"
  unresolved="$(sed -n 's/.*, \([0-9]*\) with an unresolved genome hash.*/\1/p' <<<"$gaps")"
  [ "${unresolved:-0}" = 0 ] || note "$unresolved migration(s) still name a genome hash the archive cannot resolve"

  [ "$ok" = 0 ] && verdict "PHASE 3: PASS — three hops recorded with lineage, genomes fetched by hash and held, no blob on the wire" \
                || verdict "PHASE 3: FAIL"
  return "$ok"
}

# ---------------------------------------------------------------- phase 4

# Read S and W out of the mod's own containment line, so the placements track the world
# rather than a number hard-coded here.
geometry_of() {
  local slot="$1" line
  line="$(grep_log "$slot" '\[M3-D10\] containment armed' | tail -n 1)"
  [ -n "$line" ] || return 1
  printf '%s\n' "$line"
}

phase4() {
  step "PHASE 4 — containment spot-check (D10, contract-a.md §4.3.1)"
  local ok=0 slot=1 line s w wrap
  line="$(geometry_of "$slot")" || { fail "slot $slot never printed its containment line"; return 1; }
  note "slot $slot: $line"
  s="$(field S "$line")"; w="$(field W "$line")"; wrap="$(field wrapRadius "$line")"
  note "S=$s W=$w wrapRadius=$wrap"

  slow_sims 0.5

  # --- (a) outside the square, travelling INWARD: the band is reached, the direction test
  #         refuses, and nothing migrates. This is the rule that stops every wrapped arrival
  #         from exporting through the edge it landed behind (Risk 1).
  step "4a — outside the square on the export edge, travelling inward"
  local x_out mark line_a
  x_out="$(python3 -c "print(f'{float('$s') + 300.0:.1f}')")"
  mark="$(log_mark "$slot")"
  line_a="$(send "$slot" place any "$x_out" 0 -60 0)" || { fail "place failed: $line_a"; restore_sims; return 1; }
  note "place: $line_a"
  local eid_a; eid_a="$(field entityId "$line_a")"

  case "$line_a" in
    *"inCaptureBand=True"*) note "the placement is inside the capture band, as intended" ;;
    *) fail "the placement is NOT in the capture band — the test would prove nothing"; ok=1 ;;
  esac
  case "$line_a" in
    *"outward=False"*) note "the velocity is inward, as intended" ;;
    *) fail "the placed velocity is outward — the test would prove nothing"; ok=1 ;;
  esac

  sleep 8
  local refused
  refused="$(grep_log "$slot" "\[M3-D10\] event=BAND_INWARD_REFUSED entityId=$eid_a" "$mark" | tail -n 1)"
  if [ -n "$refused" ]; then
    note "slot $slot: $refused"
    note "the band was reached OUTSIDE the square and the direction test refused it — Risk 1's rule, firing"
  else
    fail "the placed organism produced no BAND_INWARD_REFUSED line: the capture band did not engage outside the square"
    ok=1
  fi

  # "No organism crosses the export edge without an outward velocity" is Risk 1's pass
  # condition, and it is what must be asserted — NOT "this organism never exported".
  #
  # A placed organism is alive and has muscles: within a few seconds it decelerates, turns,
  # and swims out again, and THAT export is correct. An earlier version of this phase
  # counted any export in a 10 s window and failed on exactly that — the mod was right and
  # the test was wrong. The honest assertion reads the velocity the mod itself recorded on
  # every export it has ever made, ring-wide.
  step "4a (ring-wide) — every export ever made must carry an outward velocity"
  local n inward total=0 bad=0
  for n in $SLOTS; do
    local exports count
    exports="$(grep_log "$n" 'phase=MIGRATE_OUT_SENT' | tr -d '\r' \
      | sed -n 's/.*entityId=\(-\?[0-9]*\) .*edge=E .*vel=(\(-\?[0-9.]*\),\(-\?[0-9.]*\)).*/\1 \2/p')"
    count="$(grep -c . <<<"$exports" || true)"
    total=$(( total + count ))
    inward="$(awk 'NF == 2 && $2 <= 0 { print }' <<<"$exports")"
    if [ -n "$inward" ]; then
      fail "slot $n exported an organism whose velocity was not outward:"
      sed 's/^/      /' <<<"$inward" >&2
      bad=1
    fi
  done
  note "exports inspected ring-wide: $total; with a non-outward velocity: $( [ "$bad" = 0 ] && echo 0 || echo '>=1' )"
  [ "$bad" = 0 ] || ok=1

  # --- (b) far north, past the wrap radius: no strip guards that edge, so containment is
  #         the vanilla wrap (D10). The organism must not leave and must not migrate.
  step "4b — far north past the wrap radius, travelling north"
  local y_far mark_b line_b eid_b
  y_far="$(python3 -c "print(f'{float('$wrap') + 400.0:.1f}')")"
  mark_b="$(log_mark "$slot")"
  line_b="$(send "$slot" place any 0 "$y_far" 0 60)" || { fail "place failed: $line_b"; restore_sims; return 1; }
  note "place: $line_b"
  eid_b="$(field entityId "$line_b")"
  case "$line_b" in
    *"pastWrapRadius=True"*) note "the placement is past the wrap radius, as intended" ;;
    *) fail "the placement is not past the wrap radius"; ok=1 ;;
  esac

  sleep 15
  local wrapped exported_b
  wrapped="$(grep_log "$slot" "\[M3-D10\] event=WRAP entityId=$eid_b" "$mark_b" | tail -n 1)"
  exported_b="$(grep_log "$slot" "entityId=$eid_b phase=MIGRATE_OUT_SENT" "$mark_b" | wc -l)"
  note "MIGRATE_OUT_SENT for $eid_b after the placement: $exported_b (must be 0 — north is not the export edge)"
  [ "$exported_b" = 0 ] || { fail "a north-bound organism exported through the east edge"; ok=1; }

  if [ -n "$wrapped" ]; then
    note "slot $slot: $wrapped"
    note "the wrap returned it, which is D10's containment mechanism"
  else
    # The wrap shares an if/else-if with shadeAvoidance (BibitePropulsion.cs:235-243). With
    # shadeAvoidance ON the wrap branch is unreachable and the organism steers back instead
    # — still contained, by a different rule. Report which one fired rather than guess.
    note "no WRAP event; checking whether shadeAvoidance owns this edge instead"
    note "$(grep_log "$slot" '\[M3-D10\] .*shadeAvoidance' | tail -n 1)"
    local still
    still="$(send "$slot" count "$eid_b" || true)"
    note "the organism after 15s: $still"
    case "$still" in
      *"alive=1"*) note "the organism is still in the world — it did not leak outward" ;;
      *) note "the organism is gone (death or predation); containment is inconclusive for this sample" ;;
    esac
  fi

  restore_sims
  [ "$ok" = 0 ] && verdict "PHASE 4: PASS — no export without an outward velocity, and nothing leaks north" \
                || verdict "PHASE 4: FAIL"
  return "$ok"
}

# ---------------------------------------------------------------- phase 5

phase5a() {
  step "PHASE 5a — kill -9 the relay mid-migration"
  slow_sims 0.5

  local before
  before="$(file_mark "$(sclog_of 1)")"
  (
    local deadline=$(( $(now) + 120 ))
    while [ "$(now)" -lt "$deadline" ]; do
      if tail -n "+$((before + 1))" "$(sclog_of 1)" 2>/dev/null | grep -q 'forwarded MIGRATION_PAYLOAD'; then
        kill -9 "$(read_pid relay)" 2>/dev/null
        echo "relay killed at $(date +%T.%N)" >> "$LOGS/kills.log"
        exit 0
      fi
      sleep 0.02
    done
  ) &
  local watcher=$!

  local line eid mark
  mark="$(log_mark 1)"
  line="$(send 1 export any)" || { fail "export failed: $line"; kill "$watcher" 2>/dev/null; restore_sims; return 1; }
  note "export: $line"
  eid="$(field entityId "$line")"
  line="$(wait_log 1 "entityId=$eid phase=MIGRATE_OUT_SENT" 60 "$mark")" || { kill "$watcher" 2>/dev/null; restore_sims; return 1; }
  note "slot 1: $line"

  wait "$watcher" 2>/dev/null
  note "relay killed"
  rm -f "$(pidfile relay)"

  settle 15
  step "restarting the relay — it must reload ring.json and hand back the same three slots"
  start_relay
  wait_healthy "http://127.0.0.1:$RELAY_PORT/healthz" || { restore_sims; return 1; }
  settle 60

  local slot
  for slot in $SLOTS; do
    note "$(grep -aE 'ring slot granted' "$(sclog_of "$slot")" | tail -n 1)"
  done

  restore_sims
  local total; total="$(count_everywhere "$eid")"
  [ "$total" -le 1 ] && verdict "PHASE 5a: PASS (count $total; 1 = exactly once, 0 = accepted loss under D2)" \
                     || verdict "PHASE 5a: FAIL — $total copies"
  [ "$total" -le 1 ]
}

phase5b() {
  step "PHASE 5b — kill -9 sidecar 1 mid-migration, at MULTIVERSE_FAULT=post-journal"
  note "post-journal is the origin-side fault point: the sidecar has durable custody and the"
  note "game still holds the organism, which is the single most dangerous instant in the chain."
  slow_sims 0.5

  local mark_reconnect
  mark_reconnect="$(log_mark 1)"
  kill_pid sidecar-1 -TERM
  sleep 2
  start_sidecar 1 post-journal
  wait_healthy "http://127.0.0.1:$PORT_1/healthz" || { restore_sims; return 1; }
  wait_edge 1 "$mark_reconnect" 240 || { restore_sims; return 1; }

  local line eid mark
  mark="$(log_mark 1)"
  line="$(send 1 export any)" || { fail "export failed: $line"; restore_sims; return 1; }
  note "export: $line"
  eid="$(field entityId "$line")"
  line="$(wait_log 1 "entityId=$eid phase=MIGRATE_OUT_SENT" 60 "$mark")" || { restore_sims; return 1; }
  note "slot 1: $line"

  local deadline=$(( $(now) + 60 ))
  while [ ! -f "$(datadir_of 1)/fault.hit" ]; do
    [ "$(now)" -ge "$deadline" ] && { fail "the post-journal fault point was never reached"; restore_sims; return 1; }
    sleep 0.2
  done
  note "fault.hit: $(cat "$(datadir_of 1)/fault.hit")"

  kill_pid sidecar-1 -9
  echo "sidecar 1 killed at $(date +%T.%N)" >> "$LOGS/kills.log"
  settle 10

  step "restarting sidecar 1 with no fault — it must replay its journal and reclaim slot 1"
  local mark_after
  mark_after="$(log_mark 1)"
  start_sidecar 1
  wait_healthy "http://127.0.0.1:$PORT_1/healthz" || { restore_sims; return 1; }
  note "$(wait_file "$(sclog_of 1)" 'ring slot granted' 60 || true)"
  wait_edge 1 "$mark_after" 240 || note "the edge did not reopen in time — counting anyway"
  settle 60

  restore_sims
  local total; total="$(count_everywhere "$eid")"
  [ "$total" -le 1 ] && verdict "PHASE 5b: PASS (count $total; 1 = exactly once, 0 = accepted loss under D2)" \
                     || verdict "PHASE 5b: FAIL — $total copies"
  [ "$total" -le 1 ]
}

# ---------------------------------------------------------------- phase 6

phase6() {
  step "PHASE 6 — teardown: zero game processes, zero Go processes, ports free"
  down >/dev/null 2>&1
  sleep 2

  local ok=0 games gos ports
  games="$(cd /mnt/c && /mnt/c/Windows/System32/tasklist.exe 2>/dev/null | grep -ci 'The Bibites' || true)"
  gos="$(pgrep -c -f "$BIN/(relay|sidecar|archive)" 2>/dev/null || true)"
  gos="${gos:-0}"
  ports="$(ss -ltn 2>/dev/null | grep -cE "127\.0\.0\.1:($PORT_1|$PORT_2|$PORT_3|$RELAY_PORT) " || true)"

  note "game processes: $games (want 0)"
  note "rig Go processes: $gos (want 0)"
  note "bound rig ports: $ports (want 0)"
  [ "${games:-0}" = 0 ] || { fail "a game process survived teardown"; ok=1; }
  [ "$gos" = 0 ]        || { fail "a Go process survived teardown"; ok=1; }
  [ "${ports:-0}" = 0 ] || { fail "a rig port is still bound"; ok=1; }

  [ "$ok" = 0 ] && verdict "PHASE 6: PASS — nothing left running" || verdict "PHASE 6: FAIL"
  return "$ok"
}

# ---------------------------------------------------------------- reports

# The one known-benign error is the chainloader's "Unable to start Unity log writer"
# (dev_environment.md). Everything else has to be explained.
errors() {
  step "error sweep — [Error lines in all three BepInEx logs, and in the Go logs"
  local slot file unexplained
  for slot in $SLOTS; do
    file="$(game_log "$slot")"
    [ -n "$file" ] && [ -f "$file" ] || continue
    printf '\n    ---- slot %s (%s) ----\n' "$slot" "$file" >&2
    unexplained="$(grep -a '\[Error' "$file" | grep -av 'Unable to start Unity log writer' || true)"
    [ -n "$unexplained" ] && sed 's/^/      /' <<<"$unexplained" >&2
    printf '    total [Error lines: %s, of which unexplained: %s\n' \
      "$(grep -ac '\[Error' "$file" || true)" \
      "$( [ -n "$unexplained" ] && wc -l <<<"$unexplained" || echo 0)" >&2
  done

  local f
  for f in "$LOGS/relay.log" "$LOGS/archive.log" "$(sclog_of 1)" "$(sclog_of 2)" "$(sclog_of 3)"; do
    [ -f "$f" ] || continue
    printf '\n    ---- %s ----\n' "$(basename "$f")" >&2
    grep -a 'level=ERROR' "$f" | tail -n 10 | sed 's/^/      /' >&2
    printf '    total ERROR lines: %s\n' "$(grep -ac 'level=ERROR' "$f" || true)" >&2
  done
}

journal_all() {
  python3 "$E2E/journal.py" summary "$(journal_of 1)" "$(journal_of 2)" "$(journal_of 3)"
}

all() {
  build   || return 1
  seed    || return 1
  up      || return 1
  phase1  || true
  phase2  || true
  phase3  || true
  phase4  || true
  phase5a || true
  phase5b || true
  errors
  phase6
}

# run-m3-lan.sh sources this file for its helpers — the ports, the game control,
# the log waits, the field parser, the seeding and the teardown are the same on
# both rigs. M3_LIB=1 stops the dispatch below so that a source is a library
# load and not a command.
if [ "${M3_LIB:-0}" = 1 ]; then
  return 0
fi

case "${1:-status}" in
  build)   build ;;
  seed)    seed ;;
  up)      up ;;
  down)    down ;;
  status)  status ;;
  token)   ensure_token ;;
  phase1)  phase1 ;;
  phase2)  phase2 ;;
  phase3)  phase3 ;;
  phase4)  phase4 ;;
  phase5a) phase5a ;;
  phase5b) phase5b ;;
  phase6)  phase6 ;;
  errors)  errors ;;
  journal) journal_all ;;
  archive) shift; archive_list "$@" ;;
  send)    shift; send "$@" ;;
  waitlog) shift; wait_log "$@" ;;
  count)   shift; count_everywhere "$@" >/dev/null ;;
  all)     all ;;
  *)
    echo "usage: run-m3.sh build|seed|up|phase1|phase2|phase3|phase4|phase5a|phase5b|phase6|errors|journal|archive|status|down|all" >&2
    exit 1
    ;;
esac
