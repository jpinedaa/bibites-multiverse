#!/usr/bin/env bash
# The M2 two-instance rig and exit test.
#
#   run-m2.sh build        build the Go binaries into bin/
#   run-m2.sh seed         create (or evolve) M2-SectorA and M2-SectorB, then quit both games
#   run-m2.sh up           relay -> sidecar A, B -> game A, B; wait for two open edges
#   run-m2.sh phase1       happy path: force one organism A->B and back
#   run-m2.sh phase2       family re-link: parent then child, then save BOTH worlds
#   run-m2.sh phase3a      kill -9 the relay mid-migration
#   run-m2.sh phase3b      kill -9 sidecar B mid-migration (MULTIVERSE_FAULT=post-journal)
#   run-m2.sh phase3c      kill game B after a migration, before it saves (resurrection)
#   run-m2.sh phase4 [min] crossing-rate measurement, default 30 real minutes
#   run-m2.sh down         stop everything and leave no process behind
#   run-m2.sh status       what is running right now
#   run-m2.sh all          build, seed, up, phase1..3, down
#
# Each phase is separately invokable on a healthy rig, so a failed phase can be re-run
# without redoing the seed or the bring-up.
#
# Ports 8787, 8788 and 8790 are fixed defaults shared by every rig in this checkout, so
# only one rig may run at a time (dev_environment.md). `up` refuses to start on top of a
# rig that is already there.
set -uo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
E2E="$REPO/e2e"
MOD_DIR="$REPO/bibites-mod"
GAME_SH="$MOD_DIR/game.sh"
BIN="$REPO/bin"

DATA="$E2E/data"
LOGS="$E2E/logs"
ARCHIVE="$LOGS/archive"
RUN="$E2E/run"

RELAY_PORT="${RELAY_PORT:-8790}"
PORT_A="${PORT_A:-8787}"
PORT_B="${PORT_B:-8788}"
RELAY_URL="ws://127.0.0.1:$RELAY_PORT/contract-b/v1"

WORLD_A="${WORLD_A:-M2-SectorA}"
WORLD_B="${WORLD_B:-M2-SectorB}"
EDGE_A=E
EDGE_B=W

# The mod polls a command file and WSL writes it, so the file has to exist on the Windows
# side of the boundary: a path under the WSL filesystem is not reliably reachable from a
# Windows process. Ask Windows for its own temp directory and keep both spellings of it.
POWERSHELL=/mnt/c/Windows/System32/WindowsPowerShell/v1.0/powershell.exe
WIN_TEMP="$(cd /mnt/c && "$POWERSHELL" -NoProfile -NonInteractive -Command '$env:TEMP' 2>/dev/null | tr -d '\r')"
if [ -z "$WIN_TEMP" ]; then
  echo "could not read the Windows TEMP directory — is PowerShell reachable?" >&2
  exit 1
fi
WIN_CMD_DIR="$WIN_TEMP\\bibites-m2"
WSL_CMD_DIR="$(wslpath -u "$WIN_TEMP")/bibites-m2"

BEPINEX="/mnt/c/Program Files (x86)/Steam/steamapps/common/The Bibites/BepInEx"

FAMILY_REPORT="${FAMILY_REPORT:-30}"
TIME_SCALE="${TIME_SCALE:-3}"
SEED_TIME_SCALE="${SEED_TIME_SCALE:-20}"

export GOROOT="${GOROOT:-$HOME/go}"
export PATH="$GOROOT/bin:$PATH"

mkdir -p "$DATA" "$LOGS" "$ARCHIVE" "$RUN" "$WSL_CMD_DIR"

# ---------------------------------------------------------------- output

# Narration goes to stderr, values go to stdout. A helper that mixed the two would let a
# note leak into the variable a caller is capturing — which is how a phase ends up
# counting the wrong organism.
step()  { printf '\n=== %s\n' "$*" >&2; }
note()  { printf '    %s\n' "$*" >&2; }
fail()  { printf '!!! %s\n' "$*" >&2; }
verdict() { printf '\n### %s\n' "$*" >&2; }

now() { date +%s; }

# ---------------------------------------------------------------- go side

build() {
  step "building the Go binaries"
  mkdir -p "$BIN"
  ( cd "$REPO/go" && CGO_ENABLED=0 go build -o "$BIN/" ./cmd/... ) || return 1
  ls -l "$BIN"
}

pidfile() { printf '%s/%s.pid\n' "$RUN" "$1"; }

record_pid() { printf '%s\n' "$2" > "$(pidfile "$1")"; }

read_pid() { [ -f "$(pidfile "$1")" ] && cat "$(pidfile "$1")" || true; }

proc_alive() { local p="$1"; [ -n "$p" ] && kill -0 "$p" 2>/dev/null; }

start_relay() {
  local pid
  nohup "$BIN/relay" --listen "127.0.0.1:$RELAY_PORT" >>"$LOGS/relay.log" 2>&1 &
  pid=$!
  record_pid relay "$pid"
  note "relay pid=$pid port=$RELAY_PORT"
}

# start_sidecar <A|B> [fault]
start_sidecar() {
  local which="$1" fault="${2:-}"
  local port peer dir
  case "$which" in
    A) port="$PORT_A"; peer=sector-a; dir="$DATA/sector-a" ;;
    B) port="$PORT_B"; peer=sector-b; dir="$DATA/sector-b" ;;
    *) fail "unknown sidecar '$which'"; return 1 ;;
  esac
  mkdir -p "$dir"
  rm -f "$dir/fault.hit"

  local pid
  if [ -n "$fault" ]; then
    MULTIVERSE_FAULT="$fault" nohup "$BIN/sidecar" --listen "127.0.0.1:$port" --relay "$RELAY_URL" \
      --peer-id "$peer" --data-dir "$dir" --sector "$which" >>"$LOGS/sidecar-$which.log" 2>&1 &
  else
    nohup "$BIN/sidecar" --listen "127.0.0.1:$port" --relay "$RELAY_URL" \
      --peer-id "$peer" --data-dir "$dir" --sector "$which" >>"$LOGS/sidecar-$which.log" 2>&1 &
  fi
  pid=$!
  record_pid "sidecar-$which" "$pid"
  note "sidecar $which pid=$pid port=$port dir=$dir fault=${fault:-<none>}"
}

wait_healthy() {
  local url="$1" deadline=$(( $(now) + 30 ))
  while :; do
    curl -sf "$url" >/dev/null 2>&1 && return 0
    [ "$(now)" -ge "$deadline" ] && { fail "no /healthz from $url"; return 1; }
    sleep 0.3
  done
}

kill_pid() {
  local name="$1" sig="${2:--9}" pid
  pid="$(read_pid "$name")"
  [ -n "$pid" ] || return 0
  kill "$sig" "$pid" 2>/dev/null
  rm -f "$(pidfile "$name")"
  note "killed $name pid=$pid ($sig)"
}

# ---------------------------------------------------------------- game side

cmd_file_wsl()  { printf '%s/cmd-%s.txt\n' "$WSL_CMD_DIR" "$1"; }
cmd_file_win()  { printf '%s\\cmd-%s.txt\n' "$WIN_CMD_DIR" "$1"; }

game_instance() { printf 'game-%s\n' "$1"; }

start_game() {
  local which="$1"
  local world edge port
  case "$which" in
    A) world="$WORLD_A"; edge="$EDGE_A"; port="$PORT_A" ;;
    B) world="$WORLD_B"; edge="$EDGE_B"; port="$PORT_B" ;;
    *) fail "unknown game '$which'"; return 1 ;;
  esac

  # A stale result log would let an old token satisfy a new wait.
  rm -f "$(cmd_file_wsl "$which")" "$(cmd_file_wsl "$which").log" "$(cmd_file_wsl "$which").tmp"

  # WSLENV has to name EVERY variable or WSL forwards none of them.
  MULTIVERSE_SECTOR="$which" \
  MULTIVERSE_OPEN_EDGE="$edge" \
  MULTIVERSE_SIDECAR_PORT="$port" \
  MULTIVERSE_WORLD="$world" \
  MULTIVERSE_CMD_FILE="$(cmd_file_win "$which")" \
  MULTIVERSE_FAMILY_REPORT="$FAMILY_REPORT" \
  WSLENV='MULTIVERSE_SECTOR:MULTIVERSE_OPEN_EDGE:MULTIVERSE_SIDECAR_PORT:MULTIVERSE_WORLD:MULTIVERSE_CMD_FILE:MULTIVERSE_FAMILY_REPORT' \
    "$GAME_SH" start "$(game_instance "$which")"
}

# Seeding runs with no border edge, so the Contract A client stays off and the world can
# be grown and written without a sidecar in the picture.
start_game_seed() {
  local which="$1" world="$2"
  rm -f "$(cmd_file_wsl "$which")" "$(cmd_file_wsl "$which").log" "$(cmd_file_wsl "$which").tmp"

  MULTIVERSE_SECTOR="$which" \
  MULTIVERSE_WORLD="$world" \
  MULTIVERSE_CMD_FILE="$(cmd_file_win "$which")" \
  MULTIVERSE_FAMILY_REPORT="15" \
  WSLENV='MULTIVERSE_SECTOR:MULTIVERSE_WORLD:MULTIVERSE_CMD_FILE:MULTIVERSE_FAMILY_REPORT' \
    "$GAME_SH" start "$(game_instance "$which")"
}

game_log() { "$GAME_SH" logfile "$(game_instance "$1")" 2>/dev/null; }

# How many lines that instance's log holds right now. Every wait starts from a mark, so a
# line left over from an earlier hop can never satisfy the next one.
log_mark() {
  local file; file="$(game_log "$1")"
  if [ -n "$file" ] && [ -f "$file" ]; then wc -l < "$file" | tr -d ' '; else echo 0; fi
}

# wait_log <A|B> <extended regex> [timeout s] [from line] — prints the last match after
# the mark.
wait_log() {
  local which="$1" pattern="$2" timeout="${3:-120}" from="${4:-0}"
  local deadline=$(( $(now) + timeout )) file hit
  while :; do
    file="$(game_log "$which")"
    if [ -n "$file" ] && [ -f "$file" ]; then
      hit="$(tail -n "+$((from + 1))" "$file" 2>/dev/null | grep -aE "$pattern" | tail -n 1)"
      if [ -n "$hit" ]; then printf '%s\n' "$hit"; return 0; fi
    fi
    [ "$(now)" -ge "$deadline" ] && { fail "timeout(${timeout}s) waiting on game $which for: $pattern"; return 1; }
    sleep 0.5
  done
}

# send <A|B> <verb> [args...] — one command, atomically, and wait for its result line.
send() {
  local which="$1"; shift
  local verb="$1"; shift
  local args="$*"
  local file token deadline hit
  file="$(cmd_file_wsl "$which")"
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
    [ "$(now)" -ge "$deadline" ] && { fail "timeout waiting for '$verb $args' on game $which"; return 1; }
    sleep 0.2
  done
}

archive_bepinex_logs() {
  local tag="${1:-run}" stamp
  stamp="$(date +%Y%m%d-%H%M%S)"
  local f
  for f in "$BEPINEX/LogOutput.log" "$BEPINEX/LogOutput.log.1"; do
    [ -f "$f" ] || continue
    cp -f "$f" "$ARCHIVE/$(basename "$f").$tag.$stamp" 2>/dev/null || true
  done
}

# BepInEx truncates LogOutput.log on launch but leaves a stale LogOutput.log.1 behind when
# only one instance runs, and a stale file can satisfy the instance->log match. Archive
# both, then remove them, with no game running.
reset_bepinex_logs() {
  archive_bepinex_logs "${1:-reset}"
  rm -f "$BEPINEX/LogOutput.log.1" 2>/dev/null || true
  rm -f "$BEPINEX/LogOutput.log" 2>/dev/null || true
}

# ---------------------------------------------------------------- seeding

seed_one() {
  local which="$1" world="$2" want_family="$3"

  step "seeding '$world' (instance $which, family=$want_family)"
  start_game_seed "$which" "$world" || return 1

  wait_log "$which" "\[M2-WORLD\] world '$world' (loaded|seeded and live)" 900 || return 1
  note "world ready"

  send "$which" timescale "$SEED_TIME_SCALE" || return 1

  if [ "$want_family" = yes ]; then
    note "growing until at least one organism has a living parent or child"
    local deadline=$(( $(now) + ${FAMILY_TIMEOUT:-2400} )) line
    while :; do
      line="$(send "$which" family)" || true
      note "$line"
      case "$line" in *"linked=YES"*) break ;; esac
      [ "$(now)" -ge "$deadline" ] && { fail "no family link appeared in '$world' within the timeout"; return 1; }
      sleep 20
    done
  fi

  send "$which" save "$world" || return 1
  send "$which" quit || true
  "$GAME_SH" wait "$(game_instance "$which")" 90 || "$GAME_SH" stop "$(game_instance "$which")"
  archive_bepinex_logs "seed-$which"
  note "'$world' seeded"
}

seed() {
  "$GAME_SH" stop all >/dev/null 2>&1 || true
  sleep 2
  reset_bepinex_logs seed
  seed_one A "$WORLD_A" yes || return 1
  reset_bepinex_logs seed
  seed_one B "$WORLD_B" no || return 1
  return 0
}

# ---------------------------------------------------------------- bring-up

ports_busy() { ss -ltn 2>/dev/null | grep -qE "127\.0\.0\.1:($PORT_A|$PORT_B|$RELAY_PORT) "; }

up() {
  if ports_busy; then
    fail "ports $PORT_A/$PORT_B/$RELAY_PORT are already bound — another rig is running. Run 'down' first."
    return 1
  fi

  "$GAME_SH" stop all >/dev/null 2>&1 || true
  sleep 2
  reset_bepinex_logs up

  step "starting the Go side"
  start_relay
  wait_healthy "http://127.0.0.1:$RELAY_PORT/healthz" || return 1
  start_sidecar A
  start_sidecar B
  wait_healthy "http://127.0.0.1:$PORT_A/healthz" || return 1
  wait_healthy "http://127.0.0.1:$PORT_B/healthz" || return 1
  note "relay and both sidecars are healthy"

  step "starting game A ($WORLD_A, edge $EDGE_A, sidecar $PORT_A)"
  start_game A || return 1
  wait_log A "\[M2-WORLD\] world '$WORLD_A' (loaded|seeded and live)" 900 || return 1

  step "starting game B ($WORLD_B, edge $EDGE_B, sidecar $PORT_B)"
  start_game B || return 1
  wait_log B "\[M2-WORLD\] world '$WORLD_B' (loaded|seeded and live)" 900 || return 1

  step "waiting for both mods to connect and both edges to open"
  wait_log A '\[M2\] CONFIG_UPDATE reason=connect' 180 || return 1
  wait_log B '\[M2\] CONFIG_UPDATE reason=connect' 180 || return 1
  wait_log A "\[M2\] EDGE_STATUS .* edge=$EDGE_A open=True" 180 || return 1
  wait_log B "\[M2\] EDGE_STATUS .* edge=$EDGE_B open=True" 180 || return 1
  note "both edges are open"

  send A timescale "$TIME_SCALE" || return 1
  send B timescale "$TIME_SCALE" || return 1
  note "rig up"
}

down() {
  step "tearing the rig down"
  "$GAME_SH" stop all || true
  kill_pid sidecar-A -TERM
  kill_pid sidecar-B -TERM
  kill_pid relay -TERM
  sleep 1
  local p
  for p in relay sidecar-A sidecar-B; do
    local pid; pid="$(read_pid "$p")"
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
  for p in relay sidecar-A sidecar-B; do
    pid="$(read_pid "$p")"
    if proc_alive "$pid"; then note "$p pid=$pid RUNNING"; else note "$p not running"; fi
  done
  ss -ltn 2>/dev/null | grep -E "127\.0\.0\.1:($PORT_A|$PORT_B|$RELAY_PORT) " || note "no rig port is bound"
  "$GAME_SH" status
}

# wait_edge <A|B> <mark> [timeout] — a NEW EDGE_STATUS with open=True after the mark.
# Without the mark an old line satisfies the wait, the next export fires into an edge that
# is still closed, and the pipeline correctly refuses to migrate.
wait_edge() {
  local which="$1" mark="$2" timeout="${3:-180}" edge
  case "$which" in A) edge="$EDGE_A" ;; B) edge="$EDGE_B" ;; esac
  wait_log "$which" 'CONFIG_UPDATE reason=connect' "$timeout" "$mark" >/dev/null || return 1
  wait_log "$which" "EDGE_STATUS .* edge=$edge open=True" "$timeout" "$mark" >/dev/null || return 1
  note "game $which is reconnected with edge $edge open"
}

# ---------------------------------------------------------------- assertions

# observe_hop <from> <to> <entityId> <mark from> <mark to> [timeout]
# Assert the whole custody chain for one organism that is already on its way. Prints
# "migrationId=<id> entityId=<id> sha=<hash>" on success.
observe_hop() {
  local from="$1" to="$2" eid="$3" mark_from="$4" mark_to="$5" timeout="${6:-90}"
  local line mid sha_out sha_in

  line="$(wait_log "$from" "entityId=$eid phase=MIGRATE_OUT_SENT" "$timeout" "$mark_from")" || return 1
  note "$from: $line"
  mid="$(field migrationId "$line")"
  sha_out="$(field payloadSha256 "$line")"
  [ -n "$mid" ] || { fail "could not read a migrationId out of the MIGRATE_OUT line"; return 1; }

  wait_log "$from" "migrationId=$mid .*phase=DESTROYED" 60 "$mark_from" >/dev/null || return 1
  note "$from: organism destroyed on MIGRATE_OUT_ACK"

  line="$(wait_log "$to" "migrationId=$mid .*phase=MIGRATE_IN_RECEIVED" 120 "$mark_to")" || return 1
  sha_in="$(field payloadSha256 "$line")"
  note "$to: MIGRATE_IN received"

  line="$(wait_log "$to" "migrationId=$mid .*phase=SPAWNED" 120 "$mark_to")" || return 1
  note "$to: $line"

  line="$(wait_log "$to" "migrationId=$mid .*phase=MIGRATE_IN_ACK" 60 "$mark_to")" || return 1
  note "$to: $line"

  if [ -n "$sha_out" ] && [ "$sha_out" = "$sha_in" ]; then
    note "payload sha256 is byte-equal across the hop: $sha_out"
  else
    fail "PAYLOAD MISMATCH out=$sha_out in=$sha_in"
    return 1
  fi

  printf 'migrationId=%s entityId=%s sha=%s\n' "$mid" "$eid" "$sha_out"
}

# hop <from> <to> <selector> — force one organism across and assert the whole chain.
hop() {
  local from="$1" to="$2" selector="$3"
  local line eid mark_from mark_to

  mark_from="$(log_mark "$from")"
  mark_to="$(log_mark "$to")"

  line="$(send "$from" export "$selector")" || { fail "export failed: $line"; return 1; }
  note "export: $line"
  eid="$(field entityId "$line")"
  [ -n "$eid" ] || { fail "the export result named no entityId: $line"; return 1; }

  observe_hop "$from" "$to" "$eid" "$mark_from" "$mark_to" 60
}

journal_a() { printf '%s/sector-a/journal/journal.log\n' "$DATA"; }
journal_b() { printf '%s/sector-b/journal/journal.log\n' "$DATA"; }

field() { sed -n "s/.*$1=\(-\?[0-9a-fA-F-]*\).*/\1/p" <<<"$2" | head -n 1; }

# count_everywhere <entityId> — the organism-count table for the kill test.
count_everywhere() {
  local eid; eid="$(tr -cd '0-9-' <<<"$1")"
  local a b custody total
  a="$(send A count "$eid" || true)"
  b="$(send B count "$eid" || true)"
  local in_a in_b
  in_a="$(field alive "$a")"; in_a="${in_a:-0}"
  in_b="$(field alive "$b")"; in_b="${in_b:-0}"

  custody="$(python3 "$E2E/journal.py" custody "$eid" "$(journal_a)" "$(journal_b)" 2>/dev/null)"
  local custody_count
  custody_count="$(sed -n 's/^custodyCount=\([0-9]*\)$/\1/p' <<<"$custody")"; custody_count="${custody_count:-0}"

  total=$(( in_a + in_b + custody_count ))
  printf '\n    ---- organism %s ----\n' "$eid"
  printf '    world A          : %s\n' "$in_a"
  printf '    world B          : %s\n' "$in_b"
  printf '    live journal rows: %s\n' "$custody_count"
  sed 's/^/      /' <<<"$custody"
  printf '    TOTAL            : %s  -> %s\n' "$total" \
    "$( [ "$total" = 1 ] && echo PASS || { [ "$total" = 0 ] && echo 'PASS (acceptable loss)' || echo FAIL; } )"
  return 0
}

# Ecology does not stop for a kill test: at 3x an organism can wander across the border,
# get eaten, or breed while a process is being restarted, and any of those muddies the
# count. Every phase that has to identify one organism afterwards slows both sims first.
slow_sims() {
  local scale="${1:-0.5}"
  send A timescale "$scale" >/dev/null || true
  send B timescale "$scale" >/dev/null || true
  note "both sims slowed to ${scale}x for the duration of this phase"
}

restore_sims() {
  send A timescale "$TIME_SCALE" >/dev/null || true
  send B timescale "$TIME_SCALE" >/dev/null || true
}

settle() {
  local seconds="${1:-20}"
  note "letting the rig settle for ${seconds}s"
  sleep "$seconds"
}

# ---------------------------------------------------------------- phases

phase1() {
  step "PHASE 1 — happy path, A -> B -> A"
  slow_sims 1

  local out eid mark_a mark_b
  out="$(hop A B any)" || { restore_sims; verdict "PHASE 1: FAIL (A->B)"; return 1; }
  note "A->B $out"
  eid="$(field entityId "$out")"

  # Mark both logs now, so a return crossing that happens while we wait is still visible
  # to the assertion below.
  mark_b="$(log_mark B)"
  mark_a="$(log_mark A)"

  # Entry immunity is 5 simulated seconds. An organism that arrives at the far edge of an
  # empty map often turns round and leaves again on its own, which is a real crossing
  # through the same pipeline — so push it home only if it is still there.
  sleep 12
  local present
  present="$(send B count "$eid" || true)"
  note "B before the return hop: $present"

  if [ "$(field alive "$present")" = 1 ]; then
    out="$(hop B A "$eid")" || { restore_sims; verdict "PHASE 1: FAIL (B->A, forced)"; return 1; }
    note "B->A (forced) $out"
  else
    note "the organism already left B on its own — asserting that return crossing instead"
    out="$(observe_hop B A "$eid" "$mark_b" "$mark_a" 120)" || {
      restore_sims; verdict "PHASE 1: FAIL (B->A, natural)"; return 1; }
    note "B->A (natural) $out"
  fi

  count_everywhere "$eid"
  note "journals:"
  python3 "$E2E/journal.py" summary "$(journal_a)" "$(journal_b)" | sed 's/^/      /' >&2

  restore_sims
  verdict "PHASE 1: see the counts above (1 = PASS)"
}

phase2() {
  step "PHASE 2 — family re-link, then save BOTH worlds"

  local family
  family="$(send A family)" || return 1
  note "A family: $family"
  case "$family" in *"linked=YES"*) ;; *) fail "world A holds no family link — re-run 'seed'"; return 1 ;; esac

  local parent child
  child="$(field sampleChild "$family")"
  parent="$(field sampleParent "$family")"
  note "candidate child=$child parent=$parent"

  # The child's living parent stays behind in A, so A's save is where the stale
  # BibiteID risk lives (m2_considerations.md Risk 5).
  local out mark_b
  mark_b="$(log_mark B)"
  out="$(hop A B "$child")" || { verdict "PHASE 2: FAIL (child hop)"; return 1; }
  note "child hop: $out"

  local ack
  ack="$(wait_log B 'phase=MIGRATE_IN_ACK' 30 "$mark_b")" || true
  note "B: $ack"

  step "saving BOTH worlds — a throw during a save is a FAIL"
  local save_a save_b rc=0
  save_a="$(send A save "$WORLD_A")" || rc=1
  note "A save: $save_a"
  save_b="$(send B save "$WORLD_B")" || rc=1
  note "B save: $save_b"

  if [ "$rc" != 0 ]; then
    verdict "PHASE 2: FAIL — a world save reported an error"
    return 1
  fi

  verdict "PHASE 2: both worlds saved with no exception"
}

# The destination-side counters can only be non-zero when a relative is already in the
# destination world, and the order matters more than it looks.
#
# It has to be CHILD first, then PARENT. BibiteGenes.SaveState (BibiteGenes.cs:552-559)
# writes the parent id only while the parent GameObject is still alive, and the mod's
# Detach nulls that reference when the parent leaves — so a child serialized after its
# parent migrated carries no parentage at all, and the parent-then-child order can only
# ever report zero. Child first keeps the child's parent id in the destination world;
# when the parent lands there afterwards, CaptureForArrival finds the child through
# genes.parent1ID and RestoreDependants repairs it, which is what relinkedParents counts.
phase2b() {
  step "PHASE 2b — non-zero re-link counters: migrate a child, then its parent"
  # 0.2x, not 1x. The natural crossing rate at an open edge is roughly 20 to 25 border
  # approaches per simulated hour (phase 4), and an organism that has just arrived is
  # already at the far edge — so at 1x the child can leave again before the parent lands,
  # and the re-link then has nothing to find.
  slow_sims 0.2

  local family child parent pair
  family="$(send A family)" || { restore_sims; return 1; }
  note "A family: $family"
  child="$(field sampleChild "$family")"
  case "$child" in ''|0) fail "no child with a living parent in A"; restore_sims; return 1 ;; esac

  # The family report prints the pair as sampleChild=<id>(parent=<id>).
  pair="$(wait_log A "\\[M2-FAMILY\\] .*sampleChild=$child\\(parent=" 60 "$(( $(log_mark A) - 40 ))")" || true
  parent="$(sed -n "s/.*sampleChild=$child(parent=\\(-\\?[0-9]*\\)).*/\\1/p" <<<"$pair")"
  case "$parent" in ''|0) fail "could not read the parent id; family line was: $pair"; restore_sims; return 1 ;; esac
  note "child=$child parent=$parent"

  local mark_b present
  hop A B "$child" || { restore_sims; verdict "PHASE 2b: FAIL (child hop)"; return 1; }

  present="$(send B count "$child" || true)"
  note "child in B before the parent hop: $present"
  if [ "$(field alive "$present")" != 1 ]; then
    fail "the child left B again before the parent could follow — re-run the phase"
    restore_sims
    verdict "PHASE 2b: FAIL (the child did not stay in B)"
    return 1
  fi

  mark_b="$(log_mark B)"
  hop A B "$parent" || { restore_sims; verdict "PHASE 2b: FAIL (parent hop)"; return 1; }

  local ack
  ack="$(wait_log B 'phase=MIGRATE_IN_ACK .*relinked(Parents|Children)=[1-9]' 30 "$mark_b")" || {
    fail "the parent's MIGRATE_IN_ACK reported zero in both re-link counters"
    wait_log B 'phase=MIGRATE_IN_ACK' 5 "$mark_b" || true
    restore_sims
    verdict "PHASE 2b: FAIL — the re-link counters stayed at zero"
    return 1
  }
  note "B: $ack"

  local rc=0
  send A save "$WORLD_A" || rc=1
  send B save "$WORLD_B" || rc=1
  restore_sims
  [ "$rc" = 0 ] || { verdict "PHASE 2b: FAIL — a world save reported an error"; return 1; }

  verdict "PHASE 2b: re-link counters non-zero and both worlds saved"
}

phase3a() {
  step "PHASE 3a — kill -9 the relay mid-migration"
  slow_sims 0.5

  # Kill the relay the instant sidecar A hands the payload over.
  local before
  before="$(wc -l < "$LOGS/sidecar-A.log" 2>/dev/null || echo 0)"
  (
    local deadline=$(( $(now) + 120 ))
    while [ "$(now)" -lt "$deadline" ]; do
      if tail -n +"$((before + 1))" "$LOGS/sidecar-A.log" 2>/dev/null | grep -q 'forwarded MIGRATION_PAYLOAD'; then
        kill -9 "$(read_pid relay)" 2>/dev/null
        echo "relay killed at $(date +%T.%N)" >> "$LOGS/kills.log"
        exit 0
      fi
      sleep 0.02
    done
  ) &
  local watcher=$!

  local line eid mark_a
  mark_a="$(log_mark A)"
  line="$(send A export any)" || { fail "export failed: $line"; kill "$watcher" 2>/dev/null; return 1; }
  note "export: $line"
  eid="$(field entityId "$line")"
  line="$(wait_log A "entityId=$eid phase=MIGRATE_OUT_SENT" 60 "$mark_a")" || { kill "$watcher" 2>/dev/null; return 1; }
  note "A: $line"

  wait "$watcher" 2>/dev/null
  note "relay killed"
  rm -f "$(pidfile relay)"

  settle 15
  step "restarting the relay"
  start_relay
  wait_healthy "http://127.0.0.1:$RELAY_PORT/healthz" || return 1
  settle 45

  restore_sims
  count_everywhere "$eid"
  verdict "PHASE 3a: see the count above (1 = PASS, 0 = acceptable loss, 2 = FAIL)"
}

phase3b() {
  step "PHASE 3b — kill -9 sidecar B mid-migration, at MULTIVERSE_FAULT=post-journal"
  slow_sims 0.5
  note "B is the origin for this one, because post-journal is the origin-side fault point:"
  note "the sidecar has durable custody and the game still holds the organism."

  local mark_reconnect
  mark_reconnect="$(log_mark B)"
  kill_pid sidecar-B -TERM
  sleep 2
  start_sidecar B post-journal
  wait_healthy "http://127.0.0.1:$PORT_B/healthz" || return 1
  wait_edge B "$mark_reconnect" 180 || return 1

  local line eid mark_b
  mark_b="$(log_mark B)"
  line="$(send B export any)" || { fail "export failed: $line"; return 1; }
  note "export: $line"
  eid="$(field entityId "$line")"
  line="$(wait_log B "entityId=$eid phase=MIGRATE_OUT_SENT" 60 "$mark_b")" || return 1
  note "B: $line"

  local deadline=$(( $(now) + 60 ))
  while [ ! -f "$DATA/sector-b/fault.hit" ]; do
    [ "$(now)" -ge "$deadline" ] && { fail "the post-journal fault point was never reached"; return 1; }
    sleep 0.2
  done
  note "fault.hit: $(cat "$DATA/sector-b/fault.hit")"

  kill_pid sidecar-B -9
  echo "sidecar B killed at $(date +%T.%N)" >> "$LOGS/kills.log"
  settle 10

  step "restarting sidecar B with no fault"
  local mark_after
  mark_after="$(log_mark B)"
  start_sidecar B
  wait_healthy "http://127.0.0.1:$PORT_B/healthz" || return 1
  wait_edge B "$mark_after" 180 || note "the edge did not reopen in time — counting anyway"
  settle 45

  restore_sims
  count_everywhere "$eid"
  verdict "PHASE 3b: see the count above (1 = PASS, 0 = acceptable loss, 2 = FAIL)"
}

phase3c() {
  step "PHASE 3c — kill game B after a migration, before it saves (autosave rollback)"
  slow_sims 0.5
  note "B first takes an organism and SAVES, so the world on disk holds it. It then exports"
  note "it to A and is killed before the next save, so reloading resurrects a copy the"
  note "sidecar already gave away. The custody assertion of contract-a.md §7.4 must destroy it."

  local out eid
  out="$(hop A B any)" || { verdict "PHASE 3c: FAIL (setup hop A->B)"; return 1; }
  eid="$(field entityId "$out")"
  note "organism $eid is now in B"

  send B save "$WORLD_B" || { fail "could not save B"; return 1; }
  note "B saved WITH the organism on disk — this is the rollback point"

  sleep 15
  out="$(hop B A "$eid")" || { verdict "PHASE 3c: FAIL (hop B->A)"; return 1; }
  note "organism $eid is back in A and gone from B's memory"

  step "killing game B before it saves again"
  "$GAME_SH" stop "$(game_instance B)"
  echo "game B killed at $(date +%T.%N)" >> "$LOGS/kills.log"
  archive_bepinex_logs "phase3c-prekill"
  settle 10

  step "restarting game B — it reloads the world that still contains the organism"
  start_game B || return 1
  wait_log B "\[M2-WORLD\] world '$WORLD_B' (loaded|seeded and live)" 900 || return 1
  wait_log B '\[M2\] CONFIG_UPDATE reason=connect' 180 || return 1
  send B timescale "$TIME_SCALE" || true

  wait_log B 'phase=(CUSTODY_ASSERTION|DESTROYED)' 120 || \
    note "no custody-assertion line yet — the count below is what decides"

  settle 30
  restore_sims
  count_everywhere "$eid"
  verdict "PHASE 3c: see the count above (1 = PASS, 0 = acceptable loss, 2 = FAIL)"
}

phase4() {
  local minutes="${1:-30}"
  step "PHASE 4 — crossing-rate measurement for ${minutes} real minutes"

  send A timescale "${CROSSING_TIME_SCALE:-20}" || return 1
  send B timescale "${CROSSING_TIME_SCALE:-20}" || return 1

  # Measure only what happens inside the window. The counters are cumulative since the
  # world loaded, and every earlier phase forced organisms into the strip by hand, so the
  # cumulative figure is not a natural rate. Take the delta between the first and the last
  # line after this mark instead.
  local mark_a mark_b
  mark_a="$(log_mark A)"; mark_b="$(log_mark B)"
  note "measuring from log line $mark_a (A) / $mark_b (B)"

  local deadline=$(( $(now) + minutes * 60 ))
  while [ "$(now)" -lt "$deadline" ]; do
    sleep 60
    note "$(( (deadline - $(now)) / 60 )) min left — A: $(tail -n "+$((mark_a + 1))" "$(game_log A)" 2>/dev/null | grep -ac 'M2-CROSSING'), B: $(tail -n "+$((mark_b + 1))" "$(game_log B)" 2>/dev/null | grep -ac 'M2-CROSSING')"
  done

  tail -n "+$((mark_a + 1))" "$(game_log A)" | grep -a 'M2-CROSSING' > "$LOGS/crossing-A.log"
  tail -n "+$((mark_b + 1))" "$(game_log B)" | grep -a 'M2-CROSSING' > "$LOGS/crossing-B.log"

  local which
  for which in A B; do
    printf '\n    ---- sector %s (last 3 of %s lines) ----\n' \
      "$which" "$(wc -l < "$LOGS/crossing-$which.log")" >&2
    tail -n 3 "$LOGS/crossing-$which.log" | sed 's/^/      /' >&2
  done

  python3 - "$LOGS/crossing-A.log" "$LOGS/crossing-B.log" <<'PY'
import re, sys
for path in sys.argv[1:]:
    try:
        lines = open(path).read().splitlines()
    except OSError:
        continue
    rows = [dict(re.findall(r'(\w+)=([-\w.]+)', l)) for l in lines if 'simMinute=' in l]
    if len(rows) < 2:
        print("%s: not enough [M2-CROSSING] lines to measure a rate (%d)" % (path, len(rows)))
        continue
    first, last = rows[0], rows[-1]
    sim_minutes = int(last['simMinute']) - int(first['simMinute'])
    entries = int(last['totalStripEntries']) - int(first['totalStripEntries'])
    crossings = int(last['totalCrossings']) - int(first['totalCrossings'])
    print("%s: edge=%s windowSimMinutes=%d stripEntries=%d crossings=%d "
          "stripEntriesPerSimMin=%.3f crossingsPerSimMin=%.3f "
          "stripEntriesPerSimHour=%.1f crossingsPerSimHour=%.1f population=%s"
          % (path, last.get('edge', '?'), sim_minutes, entries, crossings,
             entries / sim_minutes if sim_minutes else 0.0,
             crossings / sim_minutes if sim_minutes else 0.0,
             60.0 * entries / sim_minutes if sim_minutes else 0.0,
             60.0 * crossings / sim_minutes if sim_minutes else 0.0,
             last.get('population', '?')))
PY

  send A timescale "$TIME_SCALE" || true
  send B timescale "$TIME_SCALE" || true
  verdict "PHASE 4: numbers above. The lure decision is the owner's."
}

# The one known-benign error is the chainloader's "Unable to start Unity log writer"
# (dev_environment.md). Everything else has to be explained.
errors() {
  step "error sweep — [Error lines in both BepInEx logs"
  local which file unexplained
  for which in A B; do
    file="$(game_log "$which")"
    [ -n "$file" ] && [ -f "$file" ] || continue
    printf '\n    ---- %s (%s) ----\n' "$which" "$file"
    unexplained="$(grep -a '\[Error' "$file" | grep -av 'Unable to start Unity log writer' || true)"
    if [ -n "$unexplained" ]; then
      sed 's/^/      /' <<<"$unexplained"
    fi
    printf '    total [Error lines: %s, of which unexplained: %s\n' \
      "$(grep -ac '\[Error' "$file" || true)" \
      "$( [ -n "$unexplained" ] && wc -l <<<"$unexplained" || echo 0)"
  done
}

all() {
  build   || return 1
  seed    || return 1
  up      || return 1
  phase1  || true
  phase2  || true
  phase2b || true
  phase3a || true
  phase3b || true
  phase3c || true
  errors
  down
}

case "${1:-status}" in
  build)   build ;;
  seed)    seed ;;
  up)      up ;;
  down)    down ;;
  status)  status ;;
  phase1)  phase1 ;;
  phase2)  phase2 ;;
  phase2b) phase2b ;;
  phase3a) phase3a ;;
  phase3b) phase3b ;;
  phase3c) phase3c ;;
  phase4)  shift; phase4 "$@" ;;
  errors)  errors ;;
  send)    shift; send "$@" ;;
  waitlog) shift; wait_log "$@" ;;
  count)   shift; count_everywhere "$@" ;;
  journal) python3 "$E2E/journal.py" summary "$(journal_a)" "$(journal_b)" ;;
  all)     all ;;
  *) echo "usage: run-m2.sh build|seed|up|phase1|phase2|phase2b|phase3a|phase3b|phase3c|phase4|errors|journal|status|down|all" >&2; exit 1 ;;
esac
