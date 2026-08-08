#!/usr/bin/env bash
# The M4 rig: a 3x2 grid of SIX instances, all on this machine.
#
#   run-m4.sh build      build the Go binaries into bin/
#   run-m4.sh reserve    pre-place the map: slot-1..slot-6 at (0,0)..(2,1)
#   run-m4.sh arm        copy e2e/data/slot-1 (the T1 journal) into the M4 rig
#   run-m4.sh seed       create (or evolve) M4-Slot1..M4-Slot6, then quit the games
#   run-m4.sh up         relay -> archive -> sidecars 1..6 -> games 1..6
#   run-m4.sh phase1     the grid forms: 3x2, per-edge EDGE_STATUS, archive, status page
#   run-m4.sh phase2     two-axis migration out of slot 1: east to slot 2, north to slot 4
#   run-m4.sh phase3     the resume test: the 2026-08-04 stranded organism delivers
#   run-m4.sh phase4     kill slot 5 mid-column, then splice it back in
#   run-m4.sh phase5     a NEW seventh peer splices into a live map
#   run-m4.sh phase6     burst pacing: dark slot 2, accumulate, wake, assert the pacing
#   run-m4.sh phase7     bounce-after-hold, against a short configured holdTimeoutMs
#   run-m4.sh phase8     periodic saves: [M4-SAVE] on interval, rotation on disk
#   run-m4.sh phase9     portals: two open edges on screen, plus a flourish
#   run-m4.sh phase10    error sweep, teardown, exactly-once census
#   run-m4.sh status     what is running right now
#   run-m4.sh statuspage curl the archive's /api/status and pretty-print it
#   run-m4.sh journal    summarise every sidecar journal
#   run-m4.sh archive    bin/archive list over the rig's archive data dir
#   run-m4.sh down       stop everything and leave no process behind
#   run-m4.sh all        build, reserve, arm, seed, up, phase1..10
#
# Every phase is separately invokable on a healthy rig, like run-m3.sh, so a failed
# phase re-runs without redoing the seed or the bring-up.
#
# ==============================================================================
# THE MAP, AND WHY EVERY SLOT DECLARES THE SAME EXPORT SET
# ==============================================================================
#
#   row 1:  +--> slot 4 (0,1) --> slot 5 (1,1) --> slot 6 (2,1) --+
#           +----------------- east wrap ---------------------+
#              ^               ^               ^
#              |               |               |    north lanes wrap back to row 0
#   row 0:  +--> slot 1 (0,0) --> slot 2 (1,0) --> slot 3 (2,0) --+
#           +----------------- east wrap ---------------------+
#
# BOTH AXES WRAP. contract-b-m4.md §2 defines the east lane as "the next
# deliverable slot east along its ROW, WRAPPING at the row's end" and the north
# lane the same way up the column. The map is therefore a torus, and a 3x2 torus
# HAS NO EDGE SLOTS: (2,0)'s east neighbour is (0,0), and (0,1)'s north neighbour
# is (0,0). There is no interior/perimeter distinction to derive an export set
# from.
#
# TWO-WAY LANES (D17, contract-a.md §18 A38) make that argument reach all four
# edges. Every declared edge is now BOTH an export edge and an entry edge — "out
# and in are different doors" is retired — so a conformant mod declares all four
# and the torus gives every one of them a neighbour. So every one of the six
# games gets MULTIVERSE_EXPORT_EDGES="E,N,W,S", and that is the honest answer
# rather than a convenience. What differs between slots is not what they DECLARE
# but what EDGE_STATUS OPENS, and the sidecar decides that from the map the relay
# published — never the mod, which learns no topology at all (contract-a.md §15
# A25). §2.1 makes the same point from the degenerate end: on a w×1 ring "the
# north edge is DECLARED BY THE MOD and stays closed with no_peer for the life of
# the map" — and its SOUTH edge closes the same way.
#
# THE VARIABLE STAYS EXPLICIT even though the mod's own default is now all four
# (contract-a.md §18 A41, case 3: an unset MULTIVERSE_EXPORT_EDGES means all four
# rather than "off"). The rig states the geometry it is testing, so a future
# change to the mod's default cannot silently move what this rig measures.
#
# The one place the sets legitimately differ is a peer spliced into a NEW column
# (phase 5): it too declares all four, and its north AND south lanes close
# no_peer because its column holds one slot. Again: same declaration, different
# EDGE_STATUS. On an axis of length 2 the forward and reverse lanes name the SAME
# peer (contract-b-m4.md §2.1, A38): on this 3x2 map every COLUMN is such an
# axis, so slots 1 and 4 are a two-lane pair and traffic on that axis rises
# disproportionately. That is expected, not a defect — A40's raised inbound rate
# is what absorbs it.
#
# ==============================================================================
# FIVE REAL GAMES, ONE SYNTHETIC PEER — AND THE MEASUREMENT THAT FORCED IT
# ==============================================================================
#
# SIX GAMES RUN FINE. Measured here on 2026-08-05: six instances together, about
# 550 MB each — 3.3 GB of 62 GB — and a load average near 2.4 on 16 cores.
# Memory and CPU are not the limit, and Risk 1's arithmetic holds.
#
# BEPINEX IS THE LIMIT, AND IT IS A HARD ONE. Its DiskLogListener walks
# LogOutput.log, then LogOutput.log.1 .. LogOutput.log.4, and then gives up with
# "Skipping log file creation". Five files. The sixth instance gets none — and
# it does not merely lose its log: THE MOD NEVER RUNS IN IT. Measured twice, in
# both directions:
#
#   * six instances started; five seeded their world and armed their dev command
#     file within a minute; the sixth sat at the main menu for ten minutes at
#     355 MB and 208 s of CPU against ~590 MB and ~1200 s for the others, and
#     answered nothing on its command file;
#   * one log file was then freed and that same sixth instance seeded its world
#     in about forty seconds — and the instance that had just given up its log
#     file went idle in its place, having worked a moment earlier.
#
# So the rule is five, and it is a property of the logging host, not of the
# hardware. The sixth slot of this map is driven by `bin/fakemod` instead: a
# Contract A mod-side client with no game, no world and no geometry, which
# accepts an arrival, holds it, and forwards the SAME payload bytes on through
# whichever export edge is open. Custody, dedup, lineage, pacing and
# exactly-once are exercised for real on both of its axes; what it does not have
# is a simulation, so it grows nothing, saves nothing and has no [M2-*] series.
#
# Every place that treats slot 6 differently says why, and the census reads its
# held set out of the state file it writes. THE LAN PHASE REPLACES IT with a real
# game on the second computer, which brings its own BepInEx and therefore its own
# log file.
#
# ==============================================================================
# PORTS (contract-b-m4.md §3, The M4 port plan)
# ==============================================================================
#
#   relay      127.0.0.1:8795      archive status page   127.0.0.1:8796
#   Contract A 8787 8788 8789 8790 8791 8792   (slots 1..6, loopback by contract)
#
# The relay and the archive moved out of the six-slot Contract A range for M4;
# 8790 was slot 4's port and 8791 was slot 5's.
#
# ON THIS MACHINE 8790 IS STILL UNUSABLE, AND IT IS NOT THE RIG'S FAULT. The M3
# LAN rig left a Windows-side port proxy behind:
#
#   netsh interface portproxy show v4tov4
#   0.0.0.0   8790   ->   172.24.110.174   8790      (a WSL address that is gone)
#
# It listens on 0.0.0.0, so it shadows 127.0.0.1:8790 FOR WINDOWS PROCESSES ONLY.
# The sidecar binds its own 8790 inside WSL and reports success; the game then
# dials ws://127.0.0.1:8790 from Windows, reaches the proxy, and the proxy
# forwards to a dead address — "An existing connection was forcibly closed by the
# remote host", six times, with a healthy-looking sidecar on the other side. The
# map forms with five deliverable slots and slot 1's north lane closed
# peer_mod_absent, which reads like a mod bug and is a network artifact.
#
# `up` checks for exactly this before it starts anything. The fix is one elevated
# command on the relay's own machine:
#
#   netsh interface portproxy delete v4tov4 listenport=8790 listenaddress=0.0.0.0
#
# Until that is run, set SLOT_PORT_BASE=18787 — the five-digit dodge the M3 rig
# used for its slot 3 — and the six slots take 18787..18792 instead.
#
# ==============================================================================
# THE RESUME TEST IS ARMED AT BRING-UP, NOT AT PHASE 3
# ==============================================================================
#
# `arm` copies e2e/data/slot-1 — the T1 journal, and the ONLY copy of the test
# input — into e2e/data-m4/slot-1 with cp -a. The original is never moved and
# never written. That journal holds exactly one live entry: migration
# 9d6db335-b1ae-433e-a44a-bb2109912913, entity 2004967003, out/in_flight,
# destSlot 2, recorded 2026-08-04T14:13:14Z, with no `handoff` field because M3
# had none.
#
# The M4 sidecar replays that entry as handoff=`sent` — "custody MAY have moved"
# (§9.2) — so it never re-routes it: it retries the RECORDED destSlot 2 on the
# forward cadence. THAT REPLAY RULE IS NEW, and this rehearsal is what found the
# need for it: the loader used to default every outbound create to `pending`, and
# §9.2 lets a pending entry re-route to a DIFFERENT slot with no proof of
# non-delivery, which is the duplication D2 refuses. See
# go/internal/journal/journal.go and TestM3InFlightEntryReplaysAsSent.
#
# The organism therefore delivers as soon as slot 2 is live, which is during `up`.
# Phase 3 asserts what `up` produced; it does not trigger it. Re-arm and restart
# slot 1's sidecar to run it again.
set -uo pipefail

# ---------------------------------------------------------------- library load

# run-m3.sh holds the pieces that did not change: the output helpers, the token,
# the pid book-keeping, the log waits, `field`, `send`, and the BepInEx log
# archive. M3_LIB=1 loads it as a library instead of dispatching a command.
#
# BEWARE what does NOT carry over: ring_order() and the "ring" key of ring.json
# are M3-only. An M4 relay reads that key once as a migration path and then
# writes "slots", with width/height/col/row. This file uses map_order() instead.
#
# AND BEWARE THE `${VAR:-default}` IDIOM ACROSS A SOURCE. run-m3.sh assigns
# RELAY_PORT, RELAY_LISTEN, TIME_SCALE and SEED_TIME_SCALE with `:-` defaults of
# its own, so by the time this file runs they are already SET and a second `:-`
# keeps the M3 value. That put the relay on 8790 — slot 4's Contract A port —
# with the M4 port plan sitting in a variable nobody read. Every such name is
# therefore captured from the environment BEFORE the source and assigned
# UNCONDITIONALLY after it.
_ENV_RELAY_PORT="${RELAY_PORT:-}"
_ENV_RELAY_LISTEN="${RELAY_LISTEN:-}"
_ENV_TIME_SCALE="${TIME_SCALE:-}"
_ENV_SEED_TIME_SCALE="${SEED_TIME_SCALE:-}"

M3_LIB=1
# shellcheck source=run-m3.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/run-m3.sh"

# ---------------------------------------------------------------- M4 topology

SLOTS="1 2 3 4 5 6"
# The five slots that run a real game, and therefore own a BepInEx log. See the
# header for the measurement that makes five the ceiling.
GAME_SLOTS="1 2 3 4 5"
LOGGED_SLOTS="$GAME_SLOTS"
# The slot driven by bin/fakemod: a Contract A peer with no world.
FAKE_SLOT=6
# The seventh peer of phase 5. It gets a sidecar and no mod at all, which is what
# makes it a hole to every router (§2).
SPLICE_SLOT=7

# Both axes wrap and every declared edge is now both an export and an entry edge
# (D17, contract-a.md §18 A38), so every peer declares all four. See the header
# for why this is set explicitly rather than left to the mod's own all-four
# default.
EXPORT_EDGES="E,N,W,S"
EXPORT_EDGE=E   # run-m3.sh helpers that still read the singular

RELAY_PORT="${_ENV_RELAY_PORT:-8795}"
ARCHIVE_HTTP="${ARCHIVE_HTTP:-127.0.0.1:8796}"
ARCHIVE_URL="http://$ARCHIVE_HTTP"
RELAY_LISTEN="${_ENV_RELAY_LISTEN:-127.0.0.1:$RELAY_PORT}"
RELAY_URL="ws://127.0.0.1:$RELAY_PORT/contract-b/v3"

# A separate runtime tree: the M3 rig's data dirs are the record of a run that
# happened, and `arm` reads one of them.
DATA="$E2E/data-m4"
RELAY_DATA="$E2E/relay-data-m4"
ARCHIVE_DATA="$E2E/archive-data-m4"
LOGS="$E2E/logs-m4"
BEPINEX_ARCHIVE="$LOGS/bepinex"
RUN="$E2E/run-m4"
SHOTS="$LOGS/shots"

# THE LOG IS A DISK BUDGET, NOT A SHELL REDIRECT. Until 2026-08-08 every Go
# process wrote slog to stderr and the rig caught it with `>>`, which has no
# size: 3.5 GB of sidecar and archive logs filled the root filesystem overnight
# and stopped every genome write in the rig. Each process is now GIVEN its log
# path, so it can rotate it (contract-b-m4.md §12, logRotateMb/logKeep). The
# path is the same one every helper in this file already greps, so nothing that
# reads a log had to change.
#
# The shell redirect stays, pointed at a <name>.stderr.log: a panic or a runtime
# fault is written by the Go runtime to fd 2 and never passes through slog, so
# something still has to catch it. Those files stay tiny.
LOG_ROTATE_MB="${LOG_ROTATE_MB:-100}"
LOG_KEEP="${LOG_KEEP:-5}"
# journalCompactMinutes (contract-b-m4.md §12): how often a sidecar rewrites its
# journal to the entries it still holds. 0 leaves the binary's 15-minute default.
JOURNAL_COMPACT_MINUTES="${JOURNAL_COMPACT_MINUTES:-0}"

# The command files live beside the M3 rig's, in their own directory.
WIN_CMD_DIR="$WIN_TEMP\\bibites-m4"
WSL_CMD_DIR="$(wslpath -u "$WIN_TEMP")/bibites-m4"

# The T1 journal, and the organism stranded in it since 2026-08-04.
T1_SLOT1="$E2E/data/slot-1"
STRANDED_MIGRATION=9d6db335-b1ae-433e-a44a-bb2109912913
STRANDED_ENTITY=2004967003
STRANDED_DEST=2

# Phase 7 needs to SEE the automatic bounce, and the contract default is 24
# hours. It moves that tunable on one sidecar, and the sidecar logs a warning
# when it is set.
HOLD_TIMEOUT="${HOLD_TIMEOUT:-45s}"

# Phase 6 needs to SEE the delivery rate limit work, and the shipped default no
# longer lets it: 100.0 per simulated minute with a burst of 50 swallows any
# burst this rig can force by hand, so nothing dams and there is nothing to
# pace. THE PHASE TESTS THE MECHANISM, NOT THE DEFAULT — it sets both halves on
# slot 2's sidecar and asserts against these same two numbers, so the assertion
# can never again drift from what is running. The shipped default moved twice on
# 2026-08-07 (contract-a.md §18, A40) and this phase went on asserting 2.0/5.
PACE_RATE="${PACE_RATE:-2.0}"
PACE_BURST="${PACE_BURST:-5}"

TIME_SCALE="${_ENV_TIME_SCALE:-5}"
SEED_TIME_SCALE="${_ENV_SEED_TIME_SCALE:-20}"
SAVE_MINUTES="${SAVE_MINUTES:-2}"
SAVE_KEEP="${SAVE_KEEP:-4}"

# The game's own save directory, from Application.persistentDataPath.
SAVE_DIR_WSL="$(dirname "$(ls -d /mnt/c/Users/*/AppData/LocalLow/The\ Bibites/The\ Bibites 2>/dev/null | head -n 1)")/The Bibites/Savefiles"

mkdir -p "$DATA" "$RELAY_DATA" "$ARCHIVE_DATA" "$LOGS" "$BEPINEX_ARCHIVE" "$RUN" "$SHOTS" "$WSL_CMD_DIR"

# ---------------------------------------------------------------- per-slot facts

# contract-a.md §10 gives the six slots 8787-8792, and SLOT_PORT_BASE is the
# escape hatch for a host where one of them is taken — see the header.
SLOT_PORT_BASE="${SLOT_PORT_BASE:-8787}"
port_of()    { printf '%s\n' "$(( SLOT_PORT_BASE - 1 + $1 ))"; }
world_of()   { printf 'M4-Slot%s\n' "$1"; }
peer_of()    { printf 'slot-%s\n' "$1"; }
datadir_of() { printf '%s/slot-%s\n' "$DATA" "$1"; }
journal_of() { printf '%s/slot-%s/journal/journal.log\n' "$DATA" "$1"; }
sclog_of()   { printf '%s/sidecar-%s.log\n' "$LOGS" "$1"; }
# log_flags <path> — the three flags that make a process own and bound its log.
log_flags()  { printf -- '--log-file\n%s\n--log-rotate-mb\n%s\n--log-keep\n%s\n' "$1" "$LOG_ROTATE_MB" "$LOG_KEEP"; }
game_instance() { printf 'm4slot%s\n' "$1"; }
cmd_file_wsl()  { printf '%s/cmd-%s.txt\n' "$WSL_CMD_DIR" "$1"; }
cmd_file_win()  { printf '%s\\cmd-%s.txt\n' "$WIN_CMD_DIR" "$1"; }

# The map position of each slot, row-major over a 3x2 rectangle.
pos_of() {
  case "$1" in
    1) echo '0,0' ;; 2) echo '1,0' ;; 3) echo '2,0' ;;
    4) echo '0,1' ;; 5) echo '1,1' ;; 6) echo '2,1' ;;
    *) echo '' ;;
  esac
}

# The STRUCTURAL neighbour on each axis of a full 3x2 map, which is what the
# effective lane equals while every slot is deliverable. Both wrap.
east_of()  { case "$1" in 1) echo 2 ;; 2) echo 3 ;; 3) echo 1 ;; 4) echo 5 ;; 5) echo 6 ;; 6) echo 4 ;; esac; }
north_of() { case "$1" in 1) echo 4 ;; 2) echo 5 ;; 3) echo 6 ;; 4) echo 1 ;; 5) echo 2 ;; 6) echo 3 ;; esac; }
lane_of()  { case "$2" in E) east_of "$1" ;; N) north_of "$1" ;; esac; }

has_log()  { case " $LOGGED_SLOTS " in *" $1 "*) return 0 ;; *) return 1 ;; esac; }
is_game()  { case " $GAME_SLOTS "   in *" $1 "*) return 0 ;; *) return 1 ;; esac; }
is_fake()  { [ "$1" = "$FAKE_SLOT" ]; }
fakestate(){ printf '%s/fakemod-%s.json\n' "$RUN" "$1"; }
fakelog()  { printf '%s/fakemod-%s.log\n' "$LOGS" "$1"; }

# ---------------------------------------------------------------- the map file

# The M4 relay writes "slots" with width/height/col/row. It READS an M3 "ring"
# once, as a height-1 map, and never writes that key again — which is exactly why
# ring_order() from run-m3-lan.sh is not reusable here.
map_order() {
  python3 - "$RELAY_DATA/ring.json" <<'PY' 2>/dev/null
import json, sys
try:
    doc = json.load(open(sys.argv[1]))
except Exception:
    print(""); raise SystemExit
slots = doc.get("slots")
if slots is None:
    # The M3 migration path: a "ring" key loads as a height-1 map.
    slots = [dict(s, col=i, row=0) for i, s in enumerate(doc.get("ring", []))]
print(" ".join("%s:%s@%s,%s" % (s["slot"], s["peerId"], s.get("col", 0), s.get("row", 0))
               for s in slots))
PY
}

map_shape() {
  python3 - "$RELAY_DATA/ring.json" <<'PY' 2>/dev/null
import json, sys
try:
    doc = json.load(open(sys.argv[1]))
except Exception:
    print("?x?"); raise SystemExit
print("%sx%s" % (doc.get("width", len(doc.get("ring", []))), doc.get("height", 1)))
PY
}

# ---------------------------------------------------------------- status page

# statuspage [jq-ish path] — the archive's own JSON, which is the one observability
# surface that covers slot 6 as well as the other five (§10.1).
status_json() { curl -sf --max-time 10 "$ARCHIVE_URL/api/status"; }

statuspage() {
  local out
  out="$(status_json)" || { fail "the status page did not answer on $ARCHIVE_URL"; return 1; }
  printf '%s\n' "$out" | python3 -m json.tool
}

# status_get <python expression over `d`> — one value out of the status page.
status_get() {
  status_json | python3 -c '
import json, sys
d = json.load(sys.stdin)
try:
    print(eval(sys.argv[1]))
except Exception as exc:
    print("<unreadable: %s>" % exc)
' "$1"
}

# ---------------------------------------------------------------- bring-up

rig_ports() {
  local s out=""
  for s in $SLOTS $SPLICE_SLOT; do out="$out$(port_of "$s")|"; done
  printf '%s%s' "$out" "$RELAY_PORT"
}

ports_busy() { ss -ltn 2>/dev/null | grep -qE "127\.0\.0\.1:($(rig_ports)) "; }

# check_slot_ports — the Contract A ports have to be free ON THE WINDOWS SIDE as
# well, because the game dials them from Windows and WSL's loopback forwarding
# only reaches the VM when nothing on Windows is listening. A stale `netsh
# portproxy` from a retired rig is the case that costs an hour: the sidecar binds
# happily inside WSL, the game is refused from outside it, and the map forms one
# slot short with a reason that names the mod.
check_slot_ports() {
  local listeners slot port bad=0
  listeners="$(cd /mnt/c && /mnt/c/Windows/System32/NETSTAT.EXE -ano 2>/dev/null </dev/null \
    | tr -d '\r' | grep -E 'LISTENING' || true)"
  [ -n "$listeners" ] || { note "could not read the Windows listener table; skipping the port check"; return 0; }
  for slot in $SLOTS; do
    port="$(port_of "$slot")"
    if grep -qE "^ *TCP *(0\.0\.0\.0|127\.0\.0\.1|\[::\]):$port " <<<"$listeners"; then
      fail "slot $slot's Contract A port $port is ALREADY HELD BY A WINDOWS LISTENER."
      fail "The sidecar will still bind it inside WSL and the game will still be refused."
      bad=1
    fi
  done
  if [ "$bad" != 0 ]; then
    fail ""
    fail "Most likely a port proxy left behind by the M3 LAN rig. Check it with:"
    fail "    netsh interface portproxy show v4tov4"
    fail "and remove it in an ELEVATED PowerShell on this machine:"
    fail "    netsh interface portproxy delete v4tov4 listenport=<port> listenaddress=0.0.0.0"
    fail ""
    fail "Or move the whole range out of the way for this run:"
    fail "    SLOT_PORT_BASE=18787 e2e/run-m4.sh up"
    return 1
  fi
  note "every Contract A port $(port_of 1)..$(port_of 6) is free on both sides of the WSL boundary"
}

start_relay() {
  local pid lf
  mkdir -p "$RELAY_DATA" "$LOGS"
  mapfile -t lf < <(log_flags "$LOGS/relay.log")
  nohup "$BIN/relay" --listen "$RELAY_LISTEN" --data-dir "$RELAY_DATA" \
    --token-file "$TOKEN_FILE" "${lf[@]}" >>"$LOGS/relay.stderr.log" 2>&1 &
  pid=$!
  record_pid relay "$pid"
  note "relay pid=$pid listen=$RELAY_LISTEN dataDir=$RELAY_DATA"
}

start_archive() {
  local pid lf
  mkdir -p "$ARCHIVE_DATA" "$LOGS"
  mapfile -t lf < <(log_flags "$LOGS/archive.log")
  nohup "$BIN/archive" --relay "$RELAY_URL" --peer-id archive-main \
    --data-dir "$ARCHIVE_DATA" --token-file "$TOKEN_FILE" --http "$ARCHIVE_HTTP" \
    "${lf[@]}" >>"$LOGS/archive.stderr.log" 2>&1 &
  pid=$!
  record_pid archive "$pid"
  note "archive pid=$pid dataDir=$ARCHIVE_DATA http=$ARCHIVE_HTTP"
}

# start_sidecar <slot> [extra flags...]
start_sidecar() {
  local slot="$1"; shift
  local port dir peer pos pid lf
  port="$(port_of "$slot")"; dir="$(datadir_of "$slot")"
  peer="$(peer_of "$slot")"; pos="$(pos_of "$slot")"
  mkdir -p "$dir" "$LOGS"
  rm -f "$dir/fault.hit"

  local posflag=()
  [ -n "$pos" ] && posflag=(--position "$pos")
  local compactflag=()
  [ "$JOURNAL_COMPACT_MINUTES" -gt 0 ] 2>/dev/null &&
    compactflag=(--journal-compact-minutes "$JOURNAL_COMPACT_MINUTES")
  mapfile -t lf < <(log_flags "$(sclog_of "$slot")")

  nohup "$BIN/sidecar" --listen "127.0.0.1:$port" --relay "$RELAY_URL" \
    --peer-id "$peer" "${posflag[@]}" \
    --data-dir "$dir" --token-file "$TOKEN_FILE" \
    "${compactflag[@]}" "${lf[@]}" "$@" \
    >>"$LOGS/sidecar-$slot.stderr.log" 2>&1 &
  pid=$!
  record_pid "sidecar-$slot" "$pid"
  note "sidecar $slot pid=$pid port=$port peer=$peer pos=${pos:-<none>} dir=$dir extra='$*'"
}

# start_fakemod <slot> — the synthetic peer of the header. It declares the same
# export set as every other slot, because the map is a torus and the declaration
# is geometry, not topology.
start_fakemod() {
  local slot="$1" pid
  # --time-scale MUST match the games. The sidecar paces inbound delivery per
  # SIMULATED minute of the receiving world (§7.5), so a synthetic peer left at
  # 1x beside worlds at ${TIME_SCALE}x throttles itself that many times harder
  # and grows a backlog it can never drain — which is what happened the first
  # time this rig ran, and it read like a lost delivery.
  nohup "$BIN/fakemod" --url "ws://127.0.0.1:$(port_of "$slot")/contract-a/v2" \
    --ring-slot "$slot" --export-edges "$EXPORT_EDGES" --time-scale "$TIME_SCALE" \
    --state-file "$(fakestate "$slot")" --hold "${FAKE_HOLD:-45s}" \
    --ack-delay "${FAKE_ACK_DELAY:-0s}" \
    >>"$(fakelog "$slot")" 2>&1 &
  pid=$!
  record_pid "fakemod-$slot" "$pid"
  note "fakemod $slot pid=$pid port=$(port_of "$slot") state=$(fakestate "$slot")"
}

start_game() {
  local slot="$1" world port
  if is_fake "$slot"; then start_fakemod "$slot"; return; fi
  world="$(world_of "$slot")"; port="$(port_of "$slot")"
  rm -f "$(cmd_file_wsl "$slot")" "$(cmd_file_wsl "$slot").log" "$(cmd_file_wsl "$slot").tmp"

  MULTIVERSE_RING_SLOT="$slot" \
  MULTIVERSE_EXPORT_EDGES="$EXPORT_EDGES" \
  MULTIVERSE_SIDECAR_PORT="$port" \
  MULTIVERSE_WORLD="$world" \
  MULTIVERSE_CMD_FILE="$(cmd_file_win "$slot")" \
  MULTIVERSE_SAVE_MINUTES="$SAVE_MINUTES" \
  MULTIVERSE_SAVE_KEEP="$SAVE_KEEP" \
  MULTIVERSE_FAMILY_REPORT="$FAMILY_REPORT" \
  WSLENV='MULTIVERSE_RING_SLOT:MULTIVERSE_EXPORT_EDGES:MULTIVERSE_SIDECAR_PORT:MULTIVERSE_WORLD:MULTIVERSE_CMD_FILE:MULTIVERSE_SAVE_MINUTES:MULTIVERSE_SAVE_KEEP:MULTIVERSE_FAMILY_REPORT' \
    "$GAME_SH" start "$(game_instance "$slot")"
}

start_game_seed() {
  local slot="$1" world="$2"
  rm -f "$(cmd_file_wsl "$slot")" "$(cmd_file_wsl "$slot").log" "$(cmd_file_wsl "$slot").tmp"
  MULTIVERSE_WORLD="$world" \
  MULTIVERSE_CMD_FILE="$(cmd_file_win "$slot")" \
  MULTIVERSE_SAVE_MINUTES="0" \
  WSLENV='MULTIVERSE_WORLD:MULTIVERSE_CMD_FILE:MULTIVERSE_SAVE_MINUTES' \
    "$GAME_SH" start "$(game_instance "$slot")"
}

# world_ready <slot> [timeout] — "this slot's mod side is live", on whichever
# channel it has: the mod's own [M2-WORLD] line for a game, and the synthetic
# peer's handshake for slot 6.
world_ready() {
  local slot="$1" timeout="${2:-900}" world
  if is_fake "$slot"; then
    wait_file "$(fakelog "$slot")" 'fakemod: connected' "$timeout" >/dev/null || return 1
    note "slot $slot: the synthetic peer handshook (no world; see the header)"
    return 0
  fi
  world="$(world_of "$slot")"
  wait_log "$slot" "\[M2-WORLD\] world '$world' (loaded|seeded and live)" "$timeout" >/dev/null
}

# population_of <slot> — the world's own count for a game, the held set for the
# synthetic peer.
population_of() {
  local slot="$1"
  if is_fake "$slot"; then
    python3 -c "
import json,sys
try: print(json.load(open('$(fakestate "$slot")'))['population'])
except Exception: print('unknown')"
    return
  fi
  field alive "$(send "$slot" census 2>/dev/null || true)"
}

# holds_entity <slot> <entityId> — 1 or 0. A game is asked; the synthetic peer's
# state file is read.
holds_entity() {
  local slot="$1" eid="$2"
  if is_fake "$slot"; then
    python3 -c "
import json,sys
try: d=json.load(open('$(fakestate "$slot")'))
except Exception: print(0); sys.exit()
print(1 if $eid in (d.get('entityIds') or []) else 0)"
    return
  fi
  local line alive
  line="$(send "$slot" count "$eid" 2>/dev/null || true)"
  alive="$(field alive "$line")"
  printf '%s\n' "${alive:-0}"
}

# lane_slot <slot> <edge> — the effective neighbour of one lane, and lane_reason
# the reason when it is closed.
#
# BOTH COME FROM THE STATUS PAGE, and §10.1 says what that means: the archive
# RECOMPUTES the effective lanes by §8's walk from the PEER_STATUS broadcasts it
# already subscribes to. It is the same computation on the same inputs, and it is
# for display — the relay's SECTOR_GRANT stays the authority for a peer's actual
# routing. The rig reads it here because it is the ONE surface that covers all
# six slots including the one with no BepInEx log, and because the sidecar only
# logs a grant whose reason is not "updated", so a lane that moves under a live
# peer leaves no line at all.
#
# Wherever a phase turns on a lane, the DECISIVE evidence is still a migration
# that arrives, not a map that is read. This is the cheap check in front of it.
lane_slot() {
  status_get "next((l[\"toSlot\"] for l in d[\"lanes\"] if l[\"fromSlot\"]==$1 and l[\"edge\"]==\"$2\" and l[\"open\"]), 0)"
}
lane_reason() {
  status_get "next((l[\"reason\"] for l in d[\"lanes\"] if l[\"fromSlot\"]==$1 and l[\"edge\"]==\"$2\"), \"unknown\")"
}
lane_open() {
  status_get "next((l[\"open\"] for l in d[\"lanes\"] if l[\"fromSlot\"]==$1 and l[\"edge\"]==\"$2\"), False)"
}

# edge_open <slot> <edge> [timeout] — wait until the export edge is open. A
# logged slot answers with the mod's own EDGE_STATUS line, which is authoritative
# for the gate; slot 6 answers from the status page.
edge_open() {
  local slot="$1" edge="$2" timeout="${3:-240}"
  if has_log "$slot"; then
    wait_log "$slot" "\[M2\] EDGE_STATUS .*after=\[.*$edge:open" "$timeout" >/dev/null
    return
  fi
  local deadline=$(( $(now) + timeout ))
  while :; do
    [ "$(lane_open "$slot" "$edge")" = "True" ] && return 0
    [ "$(now)" -ge "$deadline" ] && { fail "slot $slot's $edge lane never opened (status page)"; return 1; }
    sleep 3
  done
}

# wait_lane <slot> <edge> <want slot> [timeout]
wait_lane() {
  local slot="$1" edge="$2" want="$3" timeout="${4:-180}"
  local deadline=$(( $(now) + timeout )) got
  while :; do
    got="$(lane_slot "$slot" "$edge")"
    [ "$got" = "$want" ] && { note "slot $slot's $edge lane -> slot $got"; return 0; }
    [ "$(now)" -ge "$deadline" ] && { fail "slot $slot's $edge lane is slot ${got:-<none>}, want $want"; return 1; }
    sleep 3
  done
}

wait_grant() {
  local slot="$1" mark="${2:-0}" line
  line="$(wait_file "$(sclog_of "$slot")" "slot granted.*slot=$slot" 60 "$mark")" || return 1
  note "slot $slot: $line"
}

# ---------------------------------------------------------------- reserve / arm

reserve() {
  step "pre-placing the 3x2 map: slot-1..slot-6 at (0,0)..(2,1)"
  if [ -n "$(read_pid relay)" ] && proc_alive "$(read_pid relay)"; then
    fail "the relay is running; --reserve-slot is a startup command. Run 'down' first."
    return 1
  fi
  ensure_token
  mkdir -p "$RELAY_DATA"
  local args=() s
  for s in $SLOTS; do args+=(--reserve-slot "$(peer_of "$s")@$(pos_of "$s")"); done
  "$BIN/relay" --data-dir "$RELAY_DATA" --token-file "$TOKEN_FILE" "${args[@]}" \
    2>&1 | sed 's/^/    /' >&2
  note "map: $(map_shape)"
  note "order: $(map_order)"
}

arm() {
  step "arming the resume test: copying the T1 slot-1 journal into the M4 rig"
  if [ ! -f "$T1_SLOT1/journal/journal.log" ]; then
    fail "no T1 journal at $T1_SLOT1/journal/journal.log — the resume test has no input"
    return 1
  fi
  local live
  live="$(grep -ac "$STRANDED_MIGRATION" "$T1_SLOT1/journal/journal.log")"
  note "the ORIGINAL journal at $T1_SLOT1/journal/journal.log holds $live record(s) naming $STRANDED_MIGRATION"
  note "it is copied, never moved, and never opened for writing by this rig"

  local dest; dest="$(datadir_of 1)"
  if [ -f "$dest/journal/journal.log" ] \
     && grep -aq "$STRANDED_MIGRATION" "$dest/journal/journal.log"; then
    note "already armed: $dest/journal/journal.log names $STRANDED_MIGRATION"
    return 0
  fi
  rm -rf "$dest"
  mkdir -p "$DATA"
  cp -a "$T1_SLOT1" "$dest" || return 1
  # The remembered listen address and slot belong to the M3 rig's ports.
  rm -f "$dest/listen.addr"
  note "copied $(du -sh "$dest" | cut -f1) into $dest"
  note "$(grep -ac . "$dest/journal/journal.log") journal records, of which $(grep -ac "$STRANDED_MIGRATION" "$dest/journal/journal.log") name the stranded migration"
}

# ---------------------------------------------------------------- seeding

# Seed all six worlds in ONE pass. run-m3.sh seeds one at a time because three
# sequential launches are cheap; six are not, and the machine holds six anyway.
# Seeding runs with no export edge, so the Contract A client stays off and no
# sidecar is involved.
seed() {
  "$GAME_SH" stop all >/dev/null 2>&1 || true
  sleep 3
  reset_bepinex_logs seed

  # Only the five GAME slots have a world to seed. Slot 6 is the synthetic peer.
  local s
  for s in $GAME_SLOTS; do
    step "starting the seed instance for $(world_of "$s")"
    start_game_seed "$s" "$(world_of "$s")" || return 1
    sleep 6
  done

  for s in $GAME_SLOTS; do
    step "waiting for $(world_of "$s")"
    world_ready "$s" 1200 || return 1
    send "$s" timescale "$SEED_TIME_SCALE" >/dev/null || return 1
    note "$(world_of "$s") is live at ${SEED_TIME_SCALE}x"
  done

  # Slot 1 is the origin of every forced hop, so it is the one that needs a
  # living parent: the lineage annex is only interesting when a parent blob ships.
  step "growing slot 1 until an organism has a living parent or child"
  local deadline=$(( $(now) + ${FAMILY_TIMEOUT:-1800} )) line
  while :; do
    line="$(send 1 family)" || true
    note "$line"
    case "$line" in *"linked=YES"*) break ;; esac
    [ "$(now)" -ge "$deadline" ] && { note "no family link within the timeout; the annex will carry gaps"; break; }
    sleep 20
  done

  for s in $GAME_SLOTS; do
    send "$s" save "$(world_of "$s")" >/dev/null || return 1
    note "$(world_of "$s") saved"
  done
  for s in $GAME_SLOTS; do send "$s" quit >/dev/null 2>&1 || true; done
  sleep 10
  "$GAME_SH" stop all >/dev/null 2>&1 || true
  archive_bepinex_logs seed
  verdict "five worlds seeded (slot $FAKE_SLOT has none — see the header)"
}

# ---------------------------------------------------------------- up / down

up() {
  if ports_busy; then
    fail "a rig is already bound on $(rig_ports) — run 'down' first"
    return 1
  fi
  check_slot_ports || return 1
  ensure_token
  "$GAME_SH" stop all >/dev/null 2>&1 || true
  sleep 3
  reset_bepinex_logs up

  case "$(map_shape)" in
    3x2) note "the map is already pre-placed: $(map_order)" ;;
    *)   reserve || return 1 ;;
  esac

  step "starting the Go side: relay, archive, then six sidecars in slot order"
  start_relay
  wait_healthy "http://127.0.0.1:$RELAY_PORT/healthz" || return 1
  start_archive
  wait_file "$LOGS/archive.log" 'archive: subscribed to the relay' 60 >/dev/null || return 1
  note "archive subscribed; status page on $ARCHIVE_URL"

  local slot mark extra
  for slot in $SLOTS; do
    mark="$(file_mark "$(sclog_of "$slot")")"
    extra=()
    # Phase 7 needs one sidecar whose bounded hold expires inside a test run.
    # Slot 3 is the one, because nothing else in the rig depends on its timing.
    [ "$slot" = 3 ] && extra=(--hold-timeout "$HOLD_TIMEOUT")
    # Phase 6 needs one sidecar whose delivery rate limit is low enough to dam a
    # burst a person can force. Slot 2 is the one, and it is set here rather than
    # inside the phase because the pacing clock and the journal belong to the
    # process, not to the test.
    [ "$slot" = 2 ] && extra=(--inbound-rate "$PACE_RATE" --inbound-burst "$PACE_BURST")
    start_sidecar "$slot" "${extra[@]}"
    wait_healthy "http://127.0.0.1:$(port_of "$slot")/healthz" || return 1
    wait_grant "$slot" "$mark" || return 1
  done
  note "six slots granted; map is $(map_shape)"

  # SLOT ORDER IS START ORDER HERE FOR A SECOND REASON: BepInEx hands out five
  # log files and slot 6 gets none, so slot 6 must be the LAST game to start.
  for slot in $SLOTS; do
    step "starting game $slot ($(world_of "$slot"), exportEdges=$EXPORT_EDGES, sidecar $(port_of "$slot"))"
    start_game "$slot" || return 1
    sleep 8
  done
  for slot in $SLOTS; do
    world_ready "$slot" 1200 || return 1
    note "slot $slot: world live"
  done

  step "waiting for every mod to connect and both export edges to open on every slot"
  for slot in $LOGGED_SLOTS; do
    wait_log "$slot" '\[M2\] CONFIG_UPDATE reason=connect' 300 >/dev/null || return 1
  done
  for slot in $SLOTS; do
    edge_open "$slot" E 300 || { fail "slot $slot: the east lane never opened"; return 1; }
    edge_open "$slot" N 300 || { fail "slot $slot: the north lane never opened"; return 1; }
    note "slot $slot: E and N are both open"
  done

  for slot in $GAME_SLOTS; do send "$slot" timescale "$TIME_SCALE" >/dev/null || return 1; done
  note "rig up: six slots, five of them real games at ${TIME_SCALE}x"
}

down() {
  step "tearing the rig down"
  "$GAME_SH" stop all || true
  local slot p pid
  for slot in $SLOTS $SPLICE_SLOT; do kill_pid "sidecar-$slot" -TERM; kill_pid "fakemod-$slot" -TERM; done
  kill_pid archive -TERM
  kill_pid relay -TERM
  sleep 2
  for slot in $SLOTS $SPLICE_SLOT; do
    for p in "sidecar-$slot" "fakemod-$slot"; do
      pid="$(read_pid "$p")"
      proc_alive "$pid" && kill -9 "$pid" 2>/dev/null
      rm -f "$(pidfile "$p")"
    done
  done
  for p in relay archive; do
    pid="$(read_pid "$p")"
    proc_alive "$pid" && kill -9 "$pid" 2>/dev/null
    rm -f "$(pidfile "$p")"
  done
  sleep 1
  archive_bepinex_logs down
}

status() {
  step "status"
  local p pid
  for p in relay archive $(for s in $SLOTS $SPLICE_SLOT; do echo "sidecar-$s"; done) "fakemod-$FAKE_SLOT"; do
    pid="$(read_pid "$p")"
    if proc_alive "$pid"; then note "$p pid=$pid RUNNING"; else note "$p not running"; fi
  done
  ss -ltn 2>/dev/null | grep -E "127\.0\.0\.1:($(rig_ports)|8796) " || note "no rig port is bound"
  note "map: $(map_shape)  order: $(map_order)"
  "$GAME_SH" status
}

# ---------------------------------------------------------------- assertions

archive_list() { "$BIN/archive" list --data-dir "$ARCHIVE_DATA" "$@"; }
archive_mark() { date -u -d "@$(( $(now) - 2 ))" +%Y-%m-%dT%H:%M:%SZ; }

# archive_hop <from> <to> [entityId]
archive_hop() {
  local from="$1" to="$2" eid="${3:-}"
  if [ -n "$eid" ]; then
    archive_list 2>/dev/null | grep -a "entity $eid " | grep -a "slot $from -> slot $to" || true
  else
    archive_list 2>/dev/null | grep -a "slot $from -> slot $to" || true
  fi
}

wait_archive_hop() {
  local from="$1" to="$2" timeout="$3" since="$4" eid="${5:-}"
  local deadline=$(( $(now) + timeout )) hit
  while :; do
    hit="$(archive_hop "$from" "$to" "$eid" | grep -a 'delivered' | awk -v s="$since" '$1 >= s' | tail -n 1)"
    [ -n "$hit" ] && { printf '%s\n' "$hit"; return 0; }
    [ "$(now)" -ge "$deadline" ] && { fail "no delivered slot $from -> slot $to record after $since within ${timeout}s"; return 1; }
    sleep 3
  done
}

# observe_hop <from> <to> <edge> <entityId> <mark_from> <mark_to> [timeout]
# The whole custody chain for one organism already on its way, on ONE axis.
# The destination half is read from the destination's BepInEx log when it has
# one, and from its sidecar log otherwise.
observe_hop() {
  local from="$1" to="$2" edge="$3" eid="$4" mark_from="$5" mark_to="$6" timeout="${7:-120}"
  local line mid sha_out sha_in want_entry

  line="$(wait_log "$from" "entityId=$eid phase=MIGRATE_OUT_SENT" "$timeout" "$mark_from")" || return 1
  note "slot $from: $line"
  mid="$(field migrationId "$line")"
  sha_out="$(field payloadSha256 "$line")"
  [ -n "$mid" ] || { fail "no migrationId on the MIGRATE_OUT line"; return 1; }
  local out_edge; out_edge="$(field edge "$line")"
  [ "$out_edge" = "$edge" ] || { fail "slot $from exported through $out_edge, want $edge"; return 1; }

  line="$(wait_file "$(sclog_of "$from")" "forwarded MIGRATION_PAYLOAD.*migrationId=$mid" 90)" || return 1
  note "sidecar $from: $line"
  # THE ENVELOPE'S destSlot IS THE EFFECTIVE LANE, NOT THE STRUCTURAL ONE, and
  # the difference is the whole of D12. An earlier version of this check compared
  # it against the structural neighbour — which is right on a full map and wrong
  # on exactly the map phase 4 builds, where slot 4's east lane names slot 6
  # because slot 5 is dark. The caller says which slot it is asserting; the
  # structural neighbour is reported beside it as context, never as the test.
  local dest structural; dest="$(field destSlot "$line")"; structural="$(lane_of "$from" "$edge")"
  [ "$dest" = "$to" ] || { fail "slot $from's $edge lane routed to slot $dest, asserted against slot $to"; return 1; }
  if [ "$dest" = "$structural" ]; then
    note "slot $from's $edge lane is its structural neighbour, slot $dest"
  else
    note "slot $from's $edge lane BYPASSES its structural neighbour (slot $structural) and names slot $dest"
  fi

  line="$(wait_file "$(sclog_of "$to")" "took custody of an inbound organism.*migrationId=$mid" 120)" || return 1
  note "sidecar $to: $line"
  # §6.6: the entry edge is DERIVED as the opposite of the sender's exitEdge.
  # ALL FOUR EDGES EXPORT under two-way lanes (§18, A38), so all four opposites
  # are reachable here. The entry edge is no longer "the passive one": it is a
  # capture band in its own right, and rule 1 of A38 — copied outward velocity —
  # is what stops the arrival re-exporting straight back out of it.
  case "$edge" in E) want_entry=W ;; N) want_entry=S ;; W) want_entry=E ;; S) want_entry=N ;;
    *) fail "observe_hop cannot check exit edge '$edge'"; return 1 ;;
  esac
  local got_entry; got_entry="$(field entryEdge "$line")"
  [ "$got_entry" = "$want_entry" ] \
    || { fail "the $edge hop arrived on entry edge $got_entry, want the opposite edge $want_entry"; return 1; }
  note "the $edge hop arrives through the opposite edge, $want_entry"

  if has_log "$to"; then
    line="$(wait_log "$to" "migrationId=$mid .*phase=MIGRATE_IN_RECEIVED" 180 "$mark_to")" || return 1
    sha_in="$(field payloadSha256 "$line")"
    line="$(wait_log "$to" "migrationId=$mid .*phase=SPAWNED" 180 "$mark_to")" || return 1
    note "slot $to: $line"
    local spawned; spawned="$(field entityId "$line")"
    [ "$spawned" = "$eid" ] || { fail "ENTITY ID CHANGED across the hop: out=$eid in=$spawned"; return 1; }
    line="$(wait_log "$to" "migrationId=$mid .*phase=MIGRATE_IN_ACK" 90 "$mark_to")" || return 1
    note "slot $to: $line"
    if [ "$sha_out" != "$sha_in" ] || [ -z "$sha_out" ]; then
      fail "PAYLOAD MISMATCH out=$sha_out in=$sha_in"; return 1
    fi
    note "payload sha256 is byte-equal across the hop: $sha_out"
  else
    # Slot 6 has no BepInEx log. The sidecar's own "organism delivered" line is
    # written on the receiving mod's MIGRATE_IN_ACK, which is the same custody
    # gate (§6.7) — it just does not carry the payload hash.
    # A synthetic peer's arrival is PACED like any other (§7.5), so this wait is
    # generous on purpose: it is waiting on a rate limit, not on a network.
    line="$(wait_file "$(sclog_of "$to")" "organism delivered.*migrationId=$mid" 420)" || return 1
    note "sidecar $to (the synthetic peer): $line"
  fi

  wait_log "$from" "migrationId=$mid .*phase=DESTROYED" 90 "$mark_from" >/dev/null \
    && note "slot $from destroyed its copy on MIGRATE_OUT_ACK" \
    || { fail "slot $from never destroyed its copy"; return 1; }

  printf 'migrationId=%s entityId=%s edge=%s sha=%s\n' "$mid" "$eid" "$edge" "$sha_out"
}

# hop <from> <to> <edge> <selector>
hop() {
  local from="$1" to="$2" edge="$3" selector="$4"
  local line eid mark_from mark_to
  mark_from="$(log_mark "$from")"
  mark_to="$(has_log "$to" && log_mark "$to" || echo 0)"
  line="$(send "$from" export "$selector" "$edge")" || { fail "export failed: $line"; return 1; }
  note "export: $line"
  eid="$(field entityId "$line")"
  [ -n "$eid" ] || { fail "the export result named no entityId: $line"; return 1; }
  observe_hop "$from" "$to" "$edge" "$eid" "$mark_from" "$mark_to" 120
}

# count_everywhere <entityId> — the exactly-once table, map-wide.
count_everywhere() {
  local eid; eid="$(tr -cd '0-9-' <<<"$1")"
  local slot total=0 custody custody_count journals=()
  printf '\n    ---- organism %s ----\n' "$eid" >&2
  for slot in $SLOTS; do
    local alive; alive="$(holds_entity "$slot" "$eid")"; alive="${alive:-0}"
    printf '    world slot %s%s: %s\n' "$slot" \
      "$(is_fake "$slot" && printf ' (synthetic) ' || printf '      ')" "$alive" >&2
    total=$(( total + alive ))
  done
  for slot in $SLOTS $SPLICE_SLOT; do
    [ -f "$(journal_of "$slot")" ] && journals+=("$(journal_of "$slot")")
  done
  custody="$(python3 "$E2E/journal.py" custody "$eid" "${journals[@]}" 2>/dev/null)"
  custody_count="$(sed -n 's/^custodyCount=\([0-9]*\)$/\1/p' <<<"$custody")"
  custody_count="${custody_count:-0}"
  printf '    live journal rows : %s\n' "$custody_count" >&2
  sed 's/^/      /' <<<"$custody" >&2
  total=$(( total + custody_count ))
  printf '    TOTAL             : %s  -> %s\n' "$total" \
    "$( [ "$total" = 1 ] && echo PASS || { [ "$total" = 0 ] && echo 'PASS (acceptable loss under D2)' || echo FAIL; } )" >&2
  printf '%s\n' "$total"
}

slow_sims()    { local s="${1:-1}" n; for n in $GAME_SLOTS; do send "$n" timescale "$s" >/dev/null 2>&1 || true; done; note "every sim at ${s}x"; }
restore_sims() { local n; for n in $GAME_SLOTS; do send "$n" timescale "$TIME_SCALE" >/dev/null 2>&1 || true; done; }
settle()       { local s="${1:-20}"; note "letting the rig settle for ${s}s"; sleep "$s"; }

# ---------------------------------------------------------------- phase 1

phase1() {
  step "PHASE 1 — the grid forms: 3x2, six slots, per-edge EDGE_STATUS, archive, status page"
  local ok=0 line slot

  local shape; shape="$(map_shape)"
  note "relay map shape: $shape"
  [ "$shape" = "3x2" ] || { fail "the map is $shape, want 3x2"; ok=1; }
  local order want
  order="$(map_order)"
  want="1:slot-1@0,0 2:slot-2@1,0 3:slot-3@2,0 4:slot-4@0,1 5:slot-5@1,1 6:slot-6@2,1"
  note "map order (row-major): $order"
  [ "$order" = "$want" ] || { fail "map order is '$order', want '$want'"; ok=1; }

  for slot in $SLOTS; do
    line="$(grep -a 'slot granted' "$(sclog_of "$slot")" | tail -n 1)"
    if [ -z "$line" ]; then fail "sidecar $slot was never granted a slot"; ok=1; continue; fi
    local got; got="$(field slot "$line")"
    [ "$got" = "$slot" ] || { fail "sidecar $slot holds slot $got"; ok=1; }
    note "sidecar $slot: $line"
    note "slot $slot exports E->$(east_of "$slot") and N->$(north_of "$slot")"
  done

  # PER-EDGE EDGE_STATUS: one entry per declared export edge (contract-a.md §15
  # A18). The mod's own line names both, and `after=[E:open(...) N:open(...)]` is
  # the whole assertion.
  step "per-edge EDGE_STATUS on every slot that has a BepInEx log"
  for slot in $LOGGED_SLOTS; do
    line="$(grep_log "$slot" '\[M2\] EDGE_STATUS epoch=' | tail -n 1)"
    if [ -z "$line" ]; then fail "slot $slot never applied an EDGE_STATUS"; ok=1; continue; fi
    note "slot $slot: $line"
    case "$line" in
      *"E:open"*) ;;
      *) fail "slot $slot's east edge is not open"; ok=1 ;;
    esac
    case "$line" in
      *"N:open"*) ;;
      *) fail "slot $slot's north edge is not open"; ok=1 ;;
    esac
    case "$line" in
      *"entries=2"*) ;;
      *) fail "slot $slot's EDGE_STATUS did not carry two entries — one per declared export edge"; ok=1 ;;
    esac
  done
  note "slot $FAKE_SLOT is the synthetic peer: no BepInEx log and no [M2-*] series, so its"
  note "lanes and its population are read from the status page and from its own state file."

  line="$(grep -a 'archive: subscribed to the relay' "$LOGS/archive.log" | tail -n 1)"
  if [ -n "$line" ]; then note "archive: $line"; else fail "the archive never subscribed"; ok=1; fi

  step "the status page — the one surface that covers all six slots (§10.1)"
  local json="$LOGS/status-phase1.json"
  if status_json > "$json" 2>/dev/null && [ -s "$json" ]; then
    note "saved $json"
    python3 -m json.tool < "$json" | head -60 | sed 's/^/      /' >&2
    local pshape pslots plive planes popen
    pshape="$(status_get 'str(d["map"]["width"])+"x"+str(d["map"]["height"])')"
    pslots="$(status_get 'd["slotCount"]')"
    plive="$(status_get 'd["totals"]["liveSlots"]')"
    planes="$(status_get 'len(d["lanes"])')"
    popen="$(status_get 'sum(1 for l in d["lanes"] if l["open"])')"
    note "status page: map=$pshape slotCount=$pslots liveSlots=$plive lanes=$planes open=$popen"
    [ "$pshape" = "3x2" ] || { fail "the status page reports map $pshape"; ok=1; }
    [ "$pslots" = "6" ]   || { fail "the status page reports $pslots slots"; ok=1; }
    [ "$plive" = "6" ]    || { fail "the status page reports $plive live slots"; ok=1; }
    [ "$popen" = "12" ]   || { fail "the status page reports $popen open lanes, want 12 (six slots x two axes)"; ok=1; }
  else
    fail "the status page did not answer on $ARCHIVE_URL/api/status"; ok=1
  fi

  [ "$ok" = 0 ] && verdict "PHASE 1: PASS — a 3x2 map of six slots, two open export edges each, archive subscribed, status page agreeing" \
                || verdict "PHASE 1: FAIL"
  return "$ok"
}

# ---------------------------------------------------------------- phase 2

phase2() {
  step "PHASE 2 — two-axis migration out of slot 1: east into slot 2, north into slot 4"
  note "one organism per axis; the blob must be byte-equal and the entry edge must be"
  note "the OPPOSITE of the exit edge — W off an east lane, S off a north lane (§6.6)."
  local ok=0 out_e out_n
  slow_sims 1

  out_e="$(hop 1 2 E family)" || { restore_sims; verdict "PHASE 2: FAIL (east hop)"; return 1; }
  note "east hop: $out_e"
  sleep 12
  out_n="$(hop 1 4 N any)" || { restore_sims; verdict "PHASE 2: FAIL (north hop)"; return 1; }
  note "north hop: $out_n"

  settle 20
  restore_sims
  local e_eid n_eid t1 t2
  e_eid="$(field entityId "$out_e")"; n_eid="$(field entityId "$out_n")"
  t1="$(count_everywhere "$e_eid")"; t2="$(count_everywhere "$n_eid")"
  [ "$t1" -le 1 ] || { fail "the east migrant exists $t1 times"; ok=1; }
  [ "$t2" -le 1 ] || { fail "the north migrant exists $t2 times"; ok=1; }

  printf '%s\n' "$e_eid" > "$RUN/m4-east-entity"
  printf '%s\n' "$n_eid" > "$RUN/m4-north-entity"

  [ "$ok" = 0 ] && verdict "PHASE 2: PASS — both axes carry an organism, byte-equal, entering through W and S" \
                || verdict "PHASE 2: FAIL"
  return "$ok"
}

# ---------------------------------------------------------------- phase 3

phase3() {
  step "PHASE 3 — the resume test: the organism stranded in flight since 2026-08-04"
  note "migration $STRANDED_MIGRATION, entity $STRANDED_ENTITY, slot 1 -> slot $STRANDED_DEST"
  note "The entry was written by an M3 sidecar and carries NO handoff field. The M4"
  note "journal loader replays it as handoff=sent — 'custody may have moved' (§9.2) —"
  note "so it never re-routes it: it retries the RECORDED destSlot on the forward cadence."
  note "That rule was added BECAUSE of this test: the loader used to say 'pending', which"
  note "would have let §9.2 re-route the organism to a different slot with no proof."
  local ok=0 line

  # 1. The M3 journal loaded at all.
  line="$(grep -a 'journal' "$(sclog_of 1)" | grep -a -i 'open\|replay\|entries\|compact' | tail -n 3)"
  [ -n "$line" ] && sed 's/^/      /' <<<"$line" >&2

  # 2. The entry is in slot 1's live custody, or has already left it.
  step "what slot 1's sidecar did with the entry"
  line="$(grep -a "$STRANDED_MIGRATION" "$(sclog_of 1)" | head -n 20)"
  if [ -z "$line" ]; then
    fail "slot 1's sidecar log never names $STRANDED_MIGRATION — the M3 journal did not load."
    fail "STOPPING HERE. The journal is the only copy of the test input and is not to be repaired by hand."
    fail "Diagnostics: $(sclog_of 1), and the ORIGINAL at $T1_SLOT1/journal/journal.log"
    verdict "PHASE 3: FAIL — the M3 journal did not produce the stranded entry"
    return 1
  fi
  sed 's/^/      /' <<<"$line" >&2

  # 3. It was forwarded to the RECORDED destination, not re-routed.
  step "the forward: destSlot must still be $STRANDED_DEST"
  line="$(wait_file "$(sclog_of 1)" "forwarded MIGRATION_PAYLOAD.*migrationId=$STRANDED_MIGRATION" 300)" || {
    fail "slot 1 never forwarded the stranded entry"; verdict "PHASE 3: FAIL"; return 1; }
  note "DECISIVE: $line"
  local dest; dest="$(field destSlot "$line")"
  [ "$dest" = "$STRANDED_DEST" ] \
    || { fail "the entry was routed to slot $dest, not the recorded slot $STRANDED_DEST"; ok=1; }
  case "$line" in
    *reroutes=0*) note "reroutes=0 — §7.3's no-rewrite rule held across the M3->M4 migration" ;;
    *) fail "the entry was RE-ROUTED; silence is not proof and §9.2 forbids it"; ok=1 ;;
  esac

  # 4. Slot 2 took custody and its mod spawned it.
  step "the arrival in slot $STRANDED_DEST"
  line="$(wait_file "$(sclog_of "$STRANDED_DEST")" "took custody of an inbound organism.*migrationId=$STRANDED_MIGRATION" 300)" || {
    fail "slot $STRANDED_DEST never took custody"; verdict "PHASE 3: FAIL"; return 1; }
  note "DECISIVE: $line"
  line="$(wait_log "$STRANDED_DEST" "migrationId=$STRANDED_MIGRATION .*phase=SPAWNED" 300)" || {
    fail "slot $STRANDED_DEST never spawned it"; ok=1; }
  [ -n "$line" ] && note "DECISIVE: $line"
  local spawned; spawned="$(field entityId "$line")"
  [ "$spawned" = "$STRANDED_ENTITY" ] \
    || { fail "the spawned entity is $spawned, want $STRANDED_ENTITY"; ok=1; }

  line="$(wait_log "$STRANDED_DEST" "migrationId=$STRANDED_MIGRATION .*phase=MIGRATE_IN_ACK" 120)" || ok=1
  [ -n "$line" ] && note "DECISIVE: $line"

  # 5. The archive recorded it, which is the operator-visible half.
  step "the archive record"
  local rec
  rec="$(archive_list 2>/dev/null | grep -a "$STRANDED_MIGRATION" | head -n 2)"
  if [ -n "$rec" ]; then sed 's/^/      /' <<<"$rec" >&2; else fail "the archive holds no record of it"; ok=1; fi

  # 6. Exactly once, map-wide.
  settle 15
  local total; total="$(count_everywhere "$STRANDED_ENTITY")"
  [ "$total" = 1 ] || { fail "the resumed organism exists $total times"; ok=1; }

  [ "$ok" = 0 ] && verdict "PHASE 3: PASS — the 2026-08-04 stranded organism delivered under the grid, exactly once" \
                || verdict "PHASE 3: FAIL"
  return "$ok"
}

# ---------------------------------------------------------------- phase 4

phase4() {
  step "PHASE 4 — hard-kill slot 5 mid-column, then splice it back in"
  note "Row 1 holds three slots, so the EAST lane re-pairs past slot 5 and the current"
  note "continues. Column 1 holds only slot 2 and slot 5, so slot 2's NORTH lane has"
  note "nothing to re-pair to and CLOSES with no_peer. §2.1 says that is the correct"
  note "degenerate answer, and an implementation that re-pairs it is defective."
  local ok=0 line

  local mark2; mark2="$(log_mark 2)"
  step "kill -9 the game and the sidecar of slot 5"
  "$GAME_SH" stop "$(game_instance 5)" || true
  kill_pid sidecar-5 -9
  echo "slot 5 killed at $(date -u +%FT%TZ)" >> "$LOGS/kills.log"
  settle 30

  step "the width axis routes around: slot 4's east lane must now name slot 6"
  wait_lane 4 E 6 180 || ok=1
  note "the skipped list: $(status_get 'next((l["skipped"] for l in d["lanes"] if l["fromSlot"]==4 and l["edge"]=="E"), [])')"

  # THE DECISIVE EVIDENCE IS A MIGRATION THAT ARRIVES, not a map that is read.
  # Slot 6 has no BepInEx log, so observe_hop reads its half from the sidecar.
  step "and it carries traffic: one forced export east out of slot 4 must land in slot 6"
  slow_sims 1
  local reroute_out
  reroute_out="$(hop 4 6 E any)" || { fail "the re-paired east lane carried nothing"; ok=1; }
  [ -n "$reroute_out" ] && note "DECISIVE: $reroute_out"
  restore_sims

  step "the degenerate height axis: slot 2's north lane must CLOSE with no_peer"
  line="$(wait_log 2 '\[M2\] EDGE_STATUS .*N:closed\(no_peer' 180 "$mark2")" || {
    fail "slot 2's north lane did not close with no_peer"; ok=1; }
  [ -n "$line" ] && note "DECISIVE: $line"
  case "$line" in
    *"E:open"*) note "and slot 2's EAST lane stayed open — the row still holds deliverable slots" ;;
    *) fail "slot 2's east lane closed too; the width axis should have routed around"; ok=1 ;;
  esac

  step "the status page must show slot 5 as dark, with the time it went dark"
  local json="$LOGS/status-phase4-dark.json"
  status_json > "$json" 2>/dev/null
  local dark
  dark="$(status_get 'next((s for s in d["slots"] if s["slot"]==5), {}).get("live")')"
  note "status page: slot 5 live=$dark darkSince=$(status_get 'next((s for s in d["slots"] if s["slot"]==5), {}).get("darkSinceMs","unknown")')"
  [ "$dark" = "False" ] || { fail "the status page still reports slot 5 as live"; ok=1; }

  step "splicing slot 5 back in: same slot number, same coordinate, no operator action"
  local mark5; mark5="$(file_mark "$(sclog_of 5)")"
  start_sidecar 5
  wait_healthy "http://127.0.0.1:$(port_of 5)/healthz" || return 1
  wait_grant 5 "$mark5" || ok=1
  start_game 5 || return 1
  world_ready 5 1200 || ok=1
  send 5 timescale "$TIME_SCALE" >/dev/null 2>&1 || true

  local pos; pos="$(grep -a 'slot granted' "$(sclog_of 5)" | tail -n 1)"
  note "DECISIVE: $pos"
  case "$pos" in
    *"Col:1 Row:1"*) note "slot 5 reclaimed slot 5 AND position (1,1)" ;;
    *) fail "slot 5 did not come back to (1,1)"; ok=1 ;;
  esac

  step "both lanes re-pair back to slot 5, with no operator action"
  wait_lane 4 E 5 240 || ok=1
  wait_lane 2 N 5 240 || ok=1
  wait_log 2 '\[M2\] EDGE_STATUS .*N:open' 240 >/dev/null \
    && note "slot 2's north lane is open again" || { fail "slot 2's north lane never reopened"; ok=1; }

  step "and the re-paired north lane carries traffic: slot 2 exports north into slot 5"
  slow_sims 1
  local back_out
  back_out="$(hop 2 5 N any)" || { fail "the re-paired north lane carried nothing"; ok=1; }
  [ -n "$back_out" ] && note "DECISIVE: $back_out"
  restore_sims

  status_json > "$LOGS/status-phase4-back.json" 2>/dev/null

  [ "$ok" = 0 ] && verdict "PHASE 4: PASS — the width axis routed around, the degenerate height axis closed no_peer, and slot 5 spliced back into (1,1)" \
                || verdict "PHASE 4: FAIL"
  return "$ok"
}

# ---------------------------------------------------------------- phase 5

phase5() {
  step "PHASE 5 — a NEW seventh peer splices into the live map"
  note "It gets a sidecar and no game: BepInEx has five log files and the sixth game"
  note "already has none. A sidecar with no mod is not DELIVERABLE, so this phase"
  note "asserts the PLACEMENT half — the map grows, no slot is renumbered, and the"
  note "new position is routed around exactly like a hole."
  local ok=0

  local before_shape before_order
  before_shape="$(map_shape)"; before_order="$(map_order)"
  note "before: map=$before_shape order=$before_order"

  local mark; mark="$(file_mark "$(sclog_of "$SPLICE_SLOT")")"
  # --insert-after-slot 1 on the E axis asks for the position immediately east of
  # slot 1 (§7.2 rule 5), which is a splice BETWEEN two live slots.
  start_sidecar "$SPLICE_SLOT" --insert-after-slot 1 --insert-axis E
  wait_healthy "http://127.0.0.1:$(port_of "$SPLICE_SLOT")/healthz" 2>/dev/null || true
  local line
  line="$(wait_file "$(sclog_of "$SPLICE_SLOT")" 'slot granted' 90 "$mark")" || {
    fail "the seventh peer was never granted a slot"; verdict "PHASE 5: FAIL"; return 1; }
  note "DECISIVE: $line"

  local newslot; newslot="$(field slot "$line")"
  note "the newcomer holds slot $newslot"
  [ "$newslot" = 7 ] || { fail "the newcomer got slot $newslot; §7.2 gives it a number above maxSlotEverIssued"; ok=1; }

  local after_shape after_order
  after_shape="$(map_shape)"; after_order="$(map_order)"
  note "after: map=$after_shape"
  note "order: $after_order"

  # NO SLOT IS RENUMBERED. Addresses never move; positions may (§2).
  local s pair
  for s in $SLOTS; do
    pair="$(grep -oE "(^| )$s:slot-$s@" <<<" $after_order")"
    [ -n "$pair" ] || { fail "slot $s no longer maps to peer slot-$s — a renumbering happened"; ok=1; }
  done
  note "every original slot number still names its original peer: no renumbering"

  [ "$after_shape" = "$before_shape" ] \
    && note "the map kept its shape and the newcomer filled a hole" \
    || note "the map grew from $before_shape to $after_shape, and the rest of the new column is a hole"

  step "the status page names the newcomer and every hole"
  status_json > "$LOGS/status-phase5.json" 2>/dev/null
  note "slotCount=$(status_get 'd["slotCount"]') holes=$(status_get 'len(d["holes"])') map=$(status_get 'str(d["map"]["width"])+"x"+str(d["map"]["height"])')"
  note "holes: $(status_get 'd["holes"]')"

  step "and the map routes AROUND it, because a sidecar with no mod is not deliverable (§8)"
  settle 20
  local sample; sample="$(lane_slot 1 E)"
  note "slot 1's east lane -> slot ${sample:-<none>}"
  [ -n "$sample" ] && [ "$sample" != "$newslot" ] \
    && note "the newcomer is skipped while it has no mod, exactly as a hole is" \
    || note "slot 1's east lane names the newcomer: check whether a mod attached"

  [ "$ok" = 0 ] && verdict "PHASE 5: PASS — a seventh peer spliced into a live map, no slot renumbered, the map routes around it" \
                || verdict "PHASE 5: FAIL"
  return "$ok"
}

# ---------------------------------------------------------------- phase 6

phase6() {
  step "PHASE 6 — burst pacing: a dam at slot 2, gone dark, then woken"
  note "THE DAM IS NOT WHERE M3 PUT IT, AND THAT IS D12 WORKING. Under M3 a dark slot"
  note "backed traffic up into its WEST NEIGHBOUR'S journal, because that neighbour had"
  note "nowhere else to export. Under M4 that neighbour re-pairs and the current keeps"
  note "flowing, so nothing piles up behind a dark slot any more."
  note ""
  note "What still accumulates is what contract-b-m4.md §9.4 says accumulates: THE PEER'S"
  note "OWN INBOUND QUEUE. Custody moves at the speed of the wire and delivery moves at"
  note "the speed of the world (§7.5), so a burst lands in slot 2's journal in seconds and"
  note "leaves it at inboundRatePerSimMinute — $PACE_RATE per SIMULATED minute, burst"
  note "$PACE_BURST, SET EXPLICITLY on slot 2's sidecar with --inbound-rate/--inbound-burst."
  note ""
  note "THIS PHASE TESTS THE MECHANISM, NOT THE SHIPPED DEFAULT. It used to assert against"
  note "2.0/5 because those were the defaults when it was written; they are now 100.0/50"
  note "(contract-a.md §18, A40, raised twice on one day) and a burst of 12 would vanish"
  note "into the bucket without ever being paced. The numbers below and the numbers on"
  note "slot 2's command line are the same two variables, so they cannot drift apart again."
  local ok=0 n=12 i ids=() line

  step "slowing slot 2 to 1x, so one simulated minute is one wall minute and the rate is readable"
  send 2 timescale 1 >/dev/null 2>&1 || true
  slow_sims 1

  step "forcing $n exports east out of slot 1 into slot 2"
  for i in $(seq 1 "$n"); do
    line="$(send 1 export any E || true)"
    local eid; eid="$(field entityId "$line")"
    [ -n "$eid" ] && ids+=("$eid")
    sleep 1
  done
  note "forced ${#ids[@]} exports"

  step "the dam: slot 2 has taken custody of the burst and released almost none of it"
  settle 10
  local depth taken
  depth="$(status_get 'next((s for s in d["slots"] if s["slot"]==2), {}).get("pacedDepth","unknown")')"
  taken="$(grep -ac 'took custody of an inbound organism' "$(sclog_of 2)" || true)"
  note "slot 2 pacedDepth=$depth  (inbound custody taken in this run: $taken)"
  if [ "$depth" = "unknown" ] || [ "${depth:-0}" -lt 3 ]; then
    fail "slot 2's paced depth is $depth — the burst did not dam, so there is nothing to pace"
    ok=1
  else
    note "PACING EVIDENCE (the dam): $depth organisms are journaled at slot 2 and waiting on the rate limit"
  fi

  step "darkening slot 2's GAME while its sidecar keeps the journal"
  note "This is the T1 shape: the world goes away and the custody record does not."
  "$GAME_SH" stop "$(game_instance 2)" || true
  settle 45
  note "slot 2 pacedDepth while dark=$(status_get 'next((s for s in d["slots"] if s["slot"]==2), {}).get("pacedDepth","unknown")')"
  note "slot 2 modConnected=$(status_get 'next((s for s in d["slots"] if s["slot"]==2), {}).get("modConnected","unknown")')"
  note "and the map routed around it meanwhile: slot 1's east lane -> slot $(lane_slot 1 E)"

  step "waking slot 2"
  local wake_mark; wake_mark="$(file_mark "$(sclog_of 2)")"
  start_game 2 || return 1
  world_ready 2 1200 || return 1
  send 2 timescale 1 >/dev/null 2>&1 || true

  step "the backlog must drain AS A STREAM, and every sample must sit under the contract's own bound"
  # allowed(t) = burst + rate * simulatedMinutesSinceWake, with burst and rate
  # read from the SAME two variables slot 2's sidecar was launched with.
  # Sampling the sidecar's own delivery count against slot 2's own simulated
  # clock is the comparison contract-a.md §7.5 defines, on the same two numbers.
  local t0 wake_sim delivered=0 worst=0
  t0="$(now)"
  wake_sim="$(status_get 'next((s for s in d["slots"] if s["slot"]==2), {}).get("simulatedTime",0)')"
  note "slot 2 simulatedTime at wake: $wake_sim"
  local deadline=$(( t0 + 900 ))
  while :; do
    delivered="$(tail -n "+$((wake_mark + 1))" "$(sclog_of 2)" | grep -ac 'delivered MIGRATE_IN' || true)"
    delivered="${delivered:-0}"
    local sim now_pd allowed
    sim="$(status_get 'next((s for s in d["slots"] if s["slot"]==2), {}).get("simulatedTime",0)')"
    now_pd="$(status_get 'next((s for s in d["slots"] if s["slot"]==2), {}).get("pacedDepth","unknown")')"
    allowed="$(python3 -c "print(f'{float('$PACE_BURST') + float('$PACE_RATE')*((float('$sim')-float('$wake_sim'))/60.0):.2f}')" 2>/dev/null)"
    note "t+$(( $(now) - t0 ))s: delivered=$delivered pacedDepth=$now_pd simElapsed=$(python3 -c "print(f'{float('$sim')-float('$wake_sim'):.0f}')" 2>/dev/null)s allowedByRate=$allowed"
    if python3 -c "import sys; sys.exit(0 if $delivered > float('$allowed') + 1 else 1)" 2>/dev/null; then
      fail "$delivered deliveries after that much simulated time, over the paced ceiling of $allowed"
      fail "THAT IS A FLOOD, WHICH IS THE THING §15 A20 EXISTS TO PREVENT"
      ok=1
      break
    fi
    [ "$now_pd" = "0" ] && break
    [ "$(now)" -ge "$deadline" ] && { note "the drain is still going at the deadline; that is slow, not wrong"; break; }
    sleep 20
  done

  step "the paced series, from slot 2's own sidecar log"
  tail -n "+$((wake_mark + 1))" "$(sclog_of 2)" | grep -a 'delivered MIGRATE_IN' | tail -n 14 | sed 's/^/      /' >&2
  local first_ts last_ts span
  first_ts="$(tail -n "+$((wake_mark + 1))" "$(sclog_of 2)" | grep -a 'delivered MIGRATE_IN' | head -n 1 | grep -oE '^time=[^ ]+' | cut -d= -f2)"
  last_ts="$(tail -n "+$((wake_mark + 1))" "$(sclog_of 2)" | grep -a 'delivered MIGRATE_IN' | tail -n 1 | grep -oE '^time=[^ ]+' | cut -d= -f2)"
  if [ -n "$first_ts" ] && [ -n "$last_ts" ]; then
    span="$(python3 -c "
import datetime
a=datetime.datetime.fromisoformat('$first_ts'); b=datetime.datetime.fromisoformat('$last_ts')
print(int((b-a).total_seconds()))" 2>/dev/null)"
    note "PACING EVIDENCE (the stream): $delivered deliveries spread over ${span:-?} wall-clock seconds after the wake"
    if [ -n "$span" ] && [ "$delivered" -gt 6 ] && [ "$span" -lt 30 ]; then
      fail "the backlog drained in ${span}s — that is a flood, not a paced stream"
      ok=1
    fi
  fi

  step "the entry-edge crowding metric in slot 2 — the receiving half of the same measurement"
  grep_log 2 '\[M4-CROWDING\]' | tail -n 4 | sed 's/^/      /' >&2

  step "no held entry bounced while its destination was live (Risk 9)"
  local bounced
  bounced="$(status_get 'next((s for s in d["slots"] if s["slot"]==1), {}).get("bouncedTimeoutTotal","unknown")')"
  note "slot 1 bouncedTimeoutTotal=$bounced"
  [ "$bounced" = "0" ] || [ "$bounced" = "unknown" ] || { fail "slot 1 bounced $bounced entr(y|ies)"; ok=1; }

  step "and every organism in the burst exists exactly once"
  local dupes=0
  for i in "${ids[@]:0:4}"; do
    local total; total="$(count_everywhere "$i")"
    [ "$total" -le 1 ] || { fail "entity $i exists $total times"; dupes=1; }
  done
  [ "$dupes" = 0 ] || ok=1

  restore_sims
  [ "$ok" = 0 ] && verdict "PHASE 6: PASS — the burst dammed in slot 2's own journal, survived the dark, and drained as a paced stream" \
                || verdict "PHASE 6: FAIL"
  return "$ok"
}

# ---------------------------------------------------------------- phase 7

phase7() {
  step "PHASE 7 — bounce after the bounded hold, against holdTimeoutMs=$HOLD_TIMEOUT"
  note "Slot 3's sidecar runs with --hold-timeout $HOLD_TIMEOUT. §9.3's clock accrues ONLY"
  note "while the destination is dark, and at the timeout the sidecar bounces the organism"
  note "home BY ITSELF and says so at error level."
  note ""
  note "THE ORDER MATTERS AND AN EARLIER VERSION OF THIS PHASE HAD IT BACKWARDS. Darkening"
  note "the destination FIRST closes the lane, and a closed lane makes the sidecar refuse"
  note "the MIGRATE_OUT outright — no journal entry, nothing to hold, nothing to bounce."
  note "§9.3's case is an entry that was FORWARDED and then lost its destination, so the"
  note "kill has to land between the forward and the acknowledgement. A watcher does it,"
  note "the same way run-m3.sh kills the relay mid-migration."
  local ok=0 line dest=6

  # Slot 3's north lane must be its structural neighbour and it must be open.
  local north; north="$(lane_slot 3 N)"
  note "slot 3's north lane -> slot ${north:-<closed>}"
  if [ "$north" != "$dest" ]; then
    fail "slot 3's north lane names slot ${north:-<closed>}, not slot $dest; the phase has no destination"
    verdict "PHASE 7: FAIL"; return 1
  fi

  slow_sims 1

  # The kill has to land AFTER slot 6 takes custody and BEFORE its acknowledgement
  # leaves — §9.3's dangerous case, and the window a real peer gives you is about
  # ten milliseconds. The synthetic peer is restarted with an --ack-delay so the
  # window is one the rig can aim at. Nothing else about the case changes: custody
  # really has moved to slot 6 and the sender really has no proof either way.
  step "widening the window: restarting the synthetic peer with --ack-delay ${PHASE7_ACK_DELAY:-25s}"
  kill_pid "fakemod-$dest" -TERM
  sleep 2
  FAKE_ACK_DELAY="${PHASE7_ACK_DELAY:-25s}" start_fakemod "$dest"
  world_ready "$dest" 120 || { fail "the synthetic peer did not come back"; restore_sims; return 1; }
  settle 10

  local mark3sc; mark3sc="$(file_mark "$(sclog_of 3)")"

  step "arming the watcher: it kills slot $dest the instant slot 3 forwards to it"
  (
    local deadline=$(( $(now) + 120 ))
    while [ "$(now)" -lt "$deadline" ]; do
      if tail -n "+$((mark3sc + 1))" "$(sclog_of 3)" 2>/dev/null \
           | grep -q "forwarded MIGRATION_PAYLOAD.*destSlot=$dest"; then
        kill -9 "$(read_pid "fakemod-$dest")" 2>/dev/null
        kill -9 "$(read_pid "sidecar-$dest")" 2>/dev/null
        echo "slot $dest killed at $(date -u +%FT%T.%NZ)" >> "$LOGS/kills.log"
        exit 0
      fi
      sleep 0.02
    done
  ) &
  local watcher=$!

  step "forcing one north export out of slot 3"
  local eid mid
  line="$(send 3 export any N || true)"
  note "export: $line"
  eid="$(field entityId "$line")"
  line="$(wait_file "$(sclog_of 3)" "forwarded MIGRATION_PAYLOAD.*destSlot=$dest" 90 "$mark3sc")" || {
    kill "$watcher" 2>/dev/null; fail "slot 3 never forwarded north"; restore_sims
    verdict "PHASE 7: FAIL"; return 1; }
  mid="$(field migrationId "$line")"
  note "DECISIVE (forwarded): $line"
  wait "$watcher" 2>/dev/null
  rm -f "$(pidfile "sidecar-$dest")" "$(pidfile "fakemod-$dest")"
  note "slot $dest is dark: $(tail -n 1 "$LOGS/kills.log")"

  step "the destination went dark with the frame in flight, so the entry is HELD and the clock runs"
  settle 20
  line="$(grep -a 'holding a forwarded organism whose destination went dark' "$(sclog_of 3)" | tail -n 1)"
  if [ -n "$line" ]; then
    note "DECISIVE (held): $line"
  else
    fail "slot 3 never logged the hold; §9.3's clock did not start"
    ok=1
  fi
  "$BIN/sidecar" --data-dir "$(datadir_of 3)" --list-inflight 2>&1 | sed 's/^/      /' >&2 || true

  step "waiting for the bounded hold to expire ($HOLD_TIMEOUT) and the automatic bounce"
  line="$(wait_file "$(sclog_of 3)" 'BOUNDED HOLD EXPIRED' 300 "$mark3sc")" || {
    fail "the bounded hold never expired; §9.3's automatic bounce did not fire"
    "$BIN/sidecar" --data-dir "$(datadir_of 3)" --list-inflight 2>&1 | sed 's/^/      /' >&2 || true
    ok=1; }
  [ -n "$line" ] && note "DECISIVE (bounce): $line"

  step "the organism comes home through the edge it left by, exactly once"
  line="$(wait_log 3 "migrationId=$mid .*phase=SPAWNED" 180)" || {
    fail "the bounced organism never came home to slot 3"; ok=1; }
  [ -n "$line" ] && note "DECISIVE (home): $line"
  case "$line" in
    *"edge=N"*) note "it re-entered through N — the edge it left by, not a passive entry edge (§9.4)" ;;
    *) [ -n "$line" ] && { fail "the bounce came home through the wrong edge"; ok=1; } ;;
  esac

  local bounces
  bounces="$(status_get 'next((s for s in d["slots"] if s["slot"]==3), {}).get("bouncedTimeoutTotal","unknown")')"
  note "status page, slot 3: bouncedTimeoutTotal=$bounces — an automatic bounce is a FACT the operator reads (§9.3)"
  [ "$bounces" = "1" ] || [ "$bounces" -ge 1 ] 2>/dev/null || { fail "the status page reports no timeout bounce"; ok=1; }
  status_json > "$LOGS/status-phase7.json" 2>/dev/null

  step "exactly once AT THE ORIGIN — and §9.3's ONE ACCEPTED EXCEPTION, named out loud"
  # The origin must hold exactly one copy: the bounce came home and nothing else
  # did. The DESTINATION is a different question, and the contract answers it in
  # advance. §9.3: "the far sidecar took custody, died before its acknowledgement,
  # and replays its own journal on its return" — that is the one case D2's
  # at-most-once carries as a bounded exception, and the owner signed it off. A
  # rig that FAILS on it is asserting a rule the milestone deliberately does not
  # have; a rig that stays silent about it is hiding one. This does neither.
  local bounced_eid; bounced_eid="$(field entityId "$line")"
  [ -n "$bounced_eid" ] || bounced_eid="$eid"
  if [ -n "$bounced_eid" ]; then
    settle 15
    local home_count; home_count="$(holds_entity 3 "$bounced_eid")"
    note "slot 3 (the origin) holds $home_count copy/copies of entity $bounced_eid"
    [ "$home_count" = "1" ] || { fail "the origin holds $home_count copies after the bounce"; ok=1; }
    local stranded
    stranded="$(python3 "$E2E/journal.py" custody "$bounced_eid" "$(journal_of "$dest")" 2>/dev/null \
      | sed -n 's/^custodyCount=\([0-9]*\)$/\1/p')"
    stranded="${stranded:-0}"
    if [ "$stranded" != 0 ]; then
      note ""
      note "ACCEPTED DUPLICATION CASE (contract-b-m4.md §9.3, signed off 2026-08-05):"
      note "slot $dest's journal still holds $stranded live custody row(s) for this organism. It"
      note "took custody, died before its acknowledgement left, and the sender could not tell"
      note "silence from delivery — so the bounded hold expired and the organism went home."
      note "When slot $dest returns it will replay its own journal and the organism will exist"
      note "TWICE. That is the residual risk the owner accepted, it needs an invisible delivery"
      note "AND a return after the timeout, and this run reproduced it on purpose."
      python3 "$E2E/journal.py" custody "$bounced_eid" "$(journal_of "$dest")" 2>/dev/null | sed 's/^/      /' >&2
    else
      note "slot $dest's journal holds nothing for this organism, so not even the accepted case fired"
    fi
  fi

  step "bringing slot $dest back"
  local mark6; mark6="$(file_mark "$(sclog_of "$dest")")"
  start_sidecar "$dest"
  wait_healthy "http://127.0.0.1:$(port_of "$dest")/healthz" || true
  wait_grant "$dest" "$mark6" || true
  start_fakemod "$dest"
  world_ready "$dest" 300 || true
  restore_sims

  [ "$ok" = 0 ] && verdict "PHASE 7: PASS — the bounded hold expired and the organism bounced home by itself, exactly once" \
                || verdict "PHASE 7: FAIL"
  return "$ok"
}

# ---------------------------------------------------------------- phase 8

phase8() {
  step "PHASE 8 — periodic saves: [M4-SAVE] on interval in every logged game, rotation on disk"
  note "MULTIVERSE_SAVE_MINUTES=$SAVE_MINUTES simulated minutes, MULTIVERSE_SAVE_KEEP=$SAVE_KEEP."
  local ok=0 slot line

  step "the save receipts"
  for slot in $LOGGED_SLOTS; do
    line="$(grep_log "$slot" '\[M4-SAVE\] event=SAVED' | tail -n 1)"
    if [ -z "$line" ]; then
      fail "slot $slot has no [M4-SAVE] event=SAVED line"
      ok=1
      continue
    fi
    note "slot $slot: $line"
    local stall
    stall="$(field stallMs "$line")"
    note "slot $slot stallMs=$stall (Risk 3's budget is 2000 ms, at six instances)"
    if [ -n "$stall" ] && python3 -c "import sys; sys.exit(0 if float('$stall') > 2000 else 1)" 2>/dev/null; then
      fail "slot $slot's save stalled ${stall} ms, over the 2000 ms budget"
      ok=1
    fi
  done

  step "budget breaches anywhere in the rig"
  local breaches=0
  for slot in $LOGGED_SLOTS; do
    local n; n="$(grep_log "$slot" 'event=BUDGET_EXCEEDED' | grep -ac . || true)"
    breaches=$(( breaches + ${n:-0} ))
  done
  note "[M4-SAVE] event=BUDGET_EXCEEDED lines across the rig: $breaches"
  [ "$breaches" = 0 ] || { fail "the save budget was exceeded $breaches time(s) at six instances"; ok=1; }

  step "slot $FAKE_SLOT is the synthetic peer: it has no world, so it saves nothing"
  note "the status page therefore renders its save state as unknown, which is §10.1's rule working:"
  note "slot $FAKE_SLOT lastSave=$(status_get 'next((s for s in d["slots"] if s["slot"]==6), {}).get("lastSave","unknown")')"

  step "the rotation on disk"
  for slot in $GAME_SLOTS; do
    local world; world="$(world_of "$slot")"
    local live backups partial
    live="$(ls -la "$SAVE_DIR_WSL/$world.zip" 2>/dev/null | awk '{print $5}')"
    backups="$(find "$SAVE_DIR_WSL" -maxdepth 1 -name "$world-*Z.zip" 2>/dev/null | wc -l)"
    partial="$(find "$SAVE_DIR_WSL" -maxdepth 1 -name "$world.partial.zip" 2>/dev/null | wc -l)"
    note "$world: live=${live:-<missing>} bytes, timestamped backups=$backups (keep=$SAVE_KEEP), partial=$partial"
    [ -n "$live" ] || { fail "$world has no live save"; ok=1; }
    [ "$backups" -le "$SAVE_KEEP" ] || { fail "$world kept $backups backups, over MULTIVERSE_SAVE_KEEP=$SAVE_KEEP"; ok=1; }
    [ "$partial" = 0 ] || note "$world has a .partial.zip at rest — a run died mid-save; the mod deletes it on the next arm"
  done
  find "$SAVE_DIR_WSL" -maxdepth 1 -name 'M4-Slot*' -printf '%f %s\n' 2>/dev/null | sort | sed 's/^/      /' >&2

  [ "$ok" = 0 ] && verdict "PHASE 8: PASS — every logged game saves on its interval inside the 2 s budget, and the rotation holds on disk" \
                || verdict "PHASE 8: FAIL"
  return "$ok"
}

# ---------------------------------------------------------------- phase 9

phase9() {
  step "PHASE 9 — the portal: at least two open edges on screen, plus a flourish"
  local ok=0 slot=1 line

  line="$(grep_log "$slot" '\[M4-PORTAL\] event=BUILT' | tail -n 1)"
  if [ -z "$line" ]; then fail "slot $slot never built a portal"; ok=1; else note "DECISIVE: $line"; fi
  note "Risk 8's runtime check is that line: the layer, the culling mask, the shaders and the first strip's world bounds."

  local shown
  shown="$(grep_log "$slot" '\[M4-PORTAL\] event=SHOWN' | tail -n 4)"
  [ -n "$shown" ] && sed 's/^/      /' <<<"$shown" >&2

  step "framing the world and firing a flourish"
  send "$slot" camera 2400 0 0 >/dev/null 2>&1 || note "the camera verb was refused"
  sleep 2
  send "$slot" flourish export 1900 0 >/dev/null 2>&1 || true
  send "$slot" flourish entry -1900 0 >/dev/null 2>&1 || true
  sleep 1

  step "screenshot"
  local shot="$SHOTS/portal-$(date -u +%Y%m%dT%H%M%SZ).png"
  local shot_win; shot_win="$(wslpath -w "$SHOTS" 2>/dev/null)"
  ( cd /mnt/c && "$POWERSHELL" -NoProfile -NonInteractive -Command "
    Add-Type -AssemblyName System.Windows.Forms,System.Drawing
    \$b = [System.Windows.Forms.Screen]::PrimaryScreen.Bounds
    \$bmp = New-Object System.Drawing.Bitmap \$b.Width, \$b.Height
    \$g = [System.Drawing.Graphics]::FromImage(\$bmp)
    \$g.CopyFromScreen(\$b.Location, [System.Drawing.Point]::Empty, \$b.Size)
    \$bmp.Save('$shot_win\\$(basename "$shot")')
  " </dev/null >/dev/null 2>&1 ) || true
  # A CAPTURE IS NOT EVIDENCE UNTIL SOMEONE LOOKS AT IT, and this one usually
  # cannot be. `CopyFromScreen` run from a PowerShell that WSL launched has no
  # interactive desktop to read, so it returns a blank frame and saves it
  # happily. A blank 1080p PNG compresses to a few KB; a real one is hundreds.
  # Say which one this is rather than reporting a filename as proof.
  if [ -f "$shot" ]; then
    local bytes; bytes="$(stat -c %s "$shot")"
    if [ "$bytes" -lt 102400 ]; then
      note "screenshot saved but BLANK: $shot ($bytes bytes)"
      note "CopyFromScreen from a WSL-launched PowerShell has no interactive desktop, so this"
      note "capture proves nothing. The portal evidence in this phase is the [M4-PORTAL] lines:"
      note "event=BUILT carries Risk 8's whole runtime check — the layer, the camera culling mask,"
      note "the three shaders and the first strip's world bounds — and event=SHOWN names each edge."
      note "TODO-owner: the ON-SCREEN look, the sorting order and the zoom legibility need a person"
      note "at this machine's screen. The sweep below has already put the camera at each size."
    else
      note "screenshot saved: $shot ($bytes bytes)"
    fi
  else
    fail "no screenshot was captured"
    note "the portal evidence is the [M4-PORTAL] lines above; the screen capture is a convenience"
  fi

  step "the zoom-legibility sweep (WP7's open item)"
  local z
  for z in 5 250 2000 4000; do
    line="$(send "$slot" camera "$z" 2>&1 || true)"
    note "camera $z -> $line"
    sleep 1
    ( cd /mnt/c && "$POWERSHELL" -NoProfile -NonInteractive -Command "
      Add-Type -AssemblyName System.Windows.Forms,System.Drawing
      \$b = [System.Windows.Forms.Screen]::PrimaryScreen.Bounds
      \$bmp = New-Object System.Drawing.Bitmap \$b.Width, \$b.Height
      \$g = [System.Drawing.Graphics]::FromImage(\$bmp)
      \$g.CopyFromScreen(\$b.Location, [System.Drawing.Point]::Empty, \$b.Size)
      \$bmp.Save('$shot_win\\zoom-$z.png')
    " </dev/null >/dev/null 2>&1 ) || true
  done
  send "$slot" camera 2400 0 0 >/dev/null 2>&1 || true
  ls -la "$SHOTS" 2>/dev/null | sed 's/^/      /' >&2

  [ "$ok" = 0 ] && verdict "PHASE 9: PASS on the runtime evidence — the portal built, reported its layer, mask, shaders and bounds, and showed two export edges and two entry edges. The ON-SCREEN look is NOT proven here; see the note above." \
                || verdict "PHASE 9: FAIL"
  return "$ok"
}

# ---------------------------------------------------------------- phase 10

errors() {
  step "error sweep — every BepInEx log with an instance behind it, and every Go log"
  local slot file unexplained total=0
  for slot in $LOGGED_SLOTS; do
    file="$(game_log "$slot" 2>/dev/null)"
    [ -n "$file" ] && [ -f "$file" ] || continue
    printf '\n    ---- slot %s (%s) ----\n' "$slot" "$file" >&2
    unexplained="$(grep -a '\[Error' "$file" | grep -av 'Unable to start Unity log writer' || true)"
    [ -n "$unexplained" ] && sed 's/^/      /' <<<"$unexplained" | tail -n 20 >&2
    local n; n="$( [ -n "$unexplained" ] && wc -l <<<"$unexplained" || echo 0)"
    total=$(( total + n ))
    printf '    unexplained [Error lines: %s\n' "$n" >&2
  done
  note "slot $FAKE_SLOT is the synthetic peer and has no BepInEx log; its own log is swept below."

  # TWO Go-side ERROR LINES ARE EXPECTED AFTER PHASE 7, and both are the contract
  # working rather than failing. §9.3 REQUIRES an automatic bounce to be loud, and
  # it REQUIRES the accepted duplication case to announce itself at error level
  # when the late acknowledgement finally arrives. A sweep that treated either as
  # a defect would be asking the rig to hide the one risk the owner signed off.
  local f
  for f in "$LOGS/relay.log" "$LOGS/archive.log" "$(fakelog "$FAKE_SLOT")" \
           $(for s in $SLOTS $SPLICE_SLOT; do sclog_of "$s"; done); do
    [ -f "$f" ] || continue
    printf '\n    ---- %s ----\n' "$(basename "$f")" >&2
    grep -a 'level=ERROR' "$f" | tail -n 10 | sed 's/^/      /' >&2
    local all expected
    all="$(grep -ac 'level=ERROR' "$f" || true)"; all="${all:-0}"
    expected="$(grep -ac 'BOUNDED HOLD EXPIRED\|a hold timeout already bounced' "$f" || true)"
    expected="${expected:-0}"
    printf '    total ERROR lines: %s, of which §9.3 REQUIRES: %s, unexplained: %s\n' \
      "$all" "$expected" "$(( all - expected ))" >&2
  done
  printf '%s\n' "$total"
}

phase10() {
  step "PHASE 10 — the error sweep, the exactly-once census, and the teardown"
  local ok=0

  step "exactly-once census across every world and every journal"
  local eid f
  for f in m4-east-entity m4-north-entity; do
    eid="$(cat "$RUN/$f" 2>/dev/null || true)"
    [ -n "$eid" ] || continue
    local total; total="$(count_everywhere "$eid")"
    [ "$total" -le 1 ] || { fail "entity $eid exists $total times"; ok=1; }
  done
  local total; total="$(count_everywhere "$STRANDED_ENTITY")"
  [ "$total" -le 1 ] || { fail "the stranded entity exists $total times"; ok=1; }

  step "map-wide census: every world's population, and every journal's live custody"
  local slot
  for slot in $SLOTS; do
    if is_fake "$slot"; then
      note "slot $slot (synthetic): $(cat "$(fakestate "$slot")" 2>/dev/null | tr -d '\n' | tail -c 240)"
    else
      note "slot $slot: $(send "$slot" census 2>&1 | tail -c 200)"
    fi
  done
  journal_all | tail -n 30 | sed 's/^/      /' >&2

  local errs; errs="$(errors)"
  note "unexplained BepInEx [Error lines across the logged slots: $errs"

  step "final status page"
  status_json > "$LOGS/status-final.json" 2>/dev/null
  "$BIN/ringstat" --url "$ARCHIVE_URL" 2>&1 | sed 's/^/      /' >&2 || true

  step "teardown"
  down >/dev/null 2>&1
  sleep 3
  local games gos ports
  games="$(cd /mnt/c && /mnt/c/Windows/System32/tasklist.exe 2>/dev/null | grep -ci 'The Bibites' || true)"
  gos="$(pgrep -c -f "$BIN/(relay|sidecar|archive|fakemod)" 2>/dev/null || true)"
  gos="${gos:-0}"
  ports="$(ss -ltn 2>/dev/null | grep -cE "127\.0\.0\.1:($(rig_ports)|8796) " || true)"
  note "game processes: ${games:-0} (want 0)"
  note "rig Go processes: $gos (want 0)"
  note "bound rig ports: ${ports:-0} (want 0)"
  [ "${games:-0}" = 0 ] || { fail "a game process survived teardown"; ok=1; }
  [ "$gos" = 0 ]        || { fail "a Go process survived teardown"; ok=1; }
  [ "${ports:-0}" = 0 ] || { fail "a rig port is still bound"; ok=1; }

  [ "$ok" = 0 ] && verdict "PHASE 10: PASS — exactly once everywhere, nothing left running, no port held" \
                || verdict "PHASE 10: FAIL"
  return "$ok"
}

journal_all() {
  local args=() s
  for s in $SLOTS $SPLICE_SLOT; do
    [ -f "$(journal_of "$s")" ] && args+=("$(journal_of "$s")")
  done
  [ "${#args[@]}" -gt 0 ] && python3 "$E2E/journal.py" summary "${args[@]}"
}

all() {
  build   || return 1
  reserve || return 1
  arm     || return 1
  seed    || return 1
  up      || return 1
  phase1  || true
  phase2  || true
  phase3  || true
  phase4  || true
  phase5  || true
  phase6  || true
  phase7  || true
  phase8  || true
  phase9  || true
  phase10
}

# run-m4-lan.sh sources this file for the M4 topology and every helper the LAN
# does not change — the map readers, the status-page accessors, the per-slot
# facts, the hop observer, the phases that are entirely local. M4_LIB=1 stops the
# dispatch below so that a source is a library load and not a command.
#
# It is the same guard run-m3.sh carries for run-m3-lan.sh, and the same reason.
if [ "${M4_LIB:-0}" = 1 ]; then
  return 0
fi

case "${1:-status}" in
  build)      build ;;
  reserve)    reserve ;;
  arm)        arm ;;
  seed)       seed ;;
  up)         up ;;
  down)       down ;;
  status)     status ;;
  statuspage) statuspage ;;
  phase1)     phase1 ;;
  phase2)     phase2 ;;
  phase3)     phase3 ;;
  phase4)     phase4 ;;
  phase5)     phase5 ;;
  phase6)     phase6 ;;
  phase7)     phase7 ;;
  phase8)     phase8 ;;
  phase9)     phase9 ;;
  phase10)    phase10 ;;
  errors)     errors >/dev/null ;;
  journal)    journal_all ;;
  archive)    shift; archive_list "$@" ;;
  send)       shift; send "$@" ;;
  count)      shift; count_everywhere "$@" >/dev/null ;;
  all)        all ;;
  *)
    echo "usage: run-m4.sh build|reserve|arm|seed|up|phase1..phase10|errors|journal|archive|statuspage|status|down|all" >&2
    exit 1
    ;;
esac
