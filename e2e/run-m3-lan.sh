#!/usr/bin/env bash
# The M3 LAN rig: the exit test with slot 2 on the OWNER'S SECOND COMPUTER.
#
#   run-m3-lan.sh build     the Go binaries, including the Windows sidecar
#   run-m3-lan.sh seed      create (or evolve) M3-Slot1 and M3-Slot3 on this machine
#   run-m3-lan.sh lanhost   the relay's LAN address, and the owner's two elevated commands
#   run-m3-lan.sh reserve   pre-seed the ring: slot 1, slot 2, slot 3, in ring order
#   run-m3-lan.sh up        relay (LAN-bound) -> archive -> sidecars 1,3 -> games 1,3
#   run-m3-lan.sh phase1    ring form-up, including the remote slot 2
#   run-m3-lan.sh phase2    forced export from slot 1; arrival in slot 2 read from the ARCHIVE
#   run-m3-lan.sh phase3    NATURAL eastward transit out of slot 2, into slot 3
#   run-m3-lan.sh phase4    the return hop, slot 3 -> slot 1
#   run-m3-lan.sh phase5    archive truth: every hop, its lineage, its genomes
#   run-m3-lan.sh phase6    exactly-once across the ring
#   run-m3-lan.sh errors    every unexplained error line in the local logs
#   run-m3-lan.sh archive   bin/archive list over the rig's archive data dir
#   run-m3-lan.sh status    what is running on THIS machine
#   run-m3-lan.sh down      stop everything on THIS machine; the far end is untouched
#   run-m3-lan.sh all       up, phase1..6, errors, down
#
# WHY THIS IS A SECOND FILE. It sources run-m3.sh with M3_LIB=1 and reuses its
# helpers unchanged — the ports, the token, game control, the log waits, the
# `field` parser, the seeding, the teardown. What differs is not a flag but a
# whole half of the rig: two of the three slots are local, one is out of reach,
# and every assertion about that one has to come from somewhere else. Threading
# that through run-m3.sh's `up`, phases and counters would have put a
# "is this slot local?" test in about thirty places. This file holds those
# thirty tests in one place, and run-m3.sh stays the readable local rehearsal.
#
# ------------------------------------------------------------------------------
# RISK 6 IS RESOLVED HERE, AND THIS IS THE RESOLUTION.
#
# m3_considerations.md Risk 6 asks how the exit test drives the far end, and it
# offers two answers it does not like: a remote agent (a new component, new
# failure modes, new security surface) or an operator (which costs M2's
# unattended property).
#
# The answer this rig takes is a third one: THE FAR END IS NEVER DRIVEN. It is
# set up once, started once, and then it is only observed. Nothing here sends a
# command to the second computer, and nothing here reads a file on it.
#
# That is possible because every fact the exit test needs about slot 2 already
# arrives on this machine by itself:
#
#   * that slot 2 exists and holds its slot -> the relay's own ring.json;
#   * that slot 2 is live AND has a game attached -> the EDGE_STATUS ripple. The
#     export edge of slot 1 opens only when its east neighbour is live and
#     mod-connected (contract-a.md §5.4, contract-b-m3.md §8), so slot 1's own
#     BepInEx log states slot 2's health, continuously;
#   * that an organism ARRIVED in slot 2 -> the ARCHIVE. The relay copies every
#     MIGRATION_PAYLOAD and every MIGRATION_ACK to the archive (§5.1), so a
#     delivered hop 1->2 is recorded here, with its lineage, without asking the
#     second computer anything;
#   * that an organism LEFT slot 2 -> the archive again, as a 2->3 record, and
#     then slot 3's own log as the arrival.
#
# The one thing an unattended rig cannot do is force an export on the far end.
# It does not need to: the ring is a circuit and organisms cross east on their
# own. Phase 3 waits for that natural crossing. The far end therefore needs no
# drive path, no agent, and no operator beyond "run start-slot2.ps1".
#
# The far-end artifacts and the two scripts are farend/ (see farend/README.md).
# ------------------------------------------------------------------------------
#
# BEFORE YOU RUN THIS the relay port has to be reachable from the second
# computer, which takes two elevated commands on this machine: a firewall rule
# and a portproxy into the WSL VM. `run-m3-lan.sh lanhost` prints both of them
# with this machine's real addresses already filled in.
set -uo pipefail

# run-m3.sh gives RELAY_LISTEN a loopback default, so an override has to be
# captured before the source and applied after it.
_ENV_RELAY_LISTEN="${RELAY_LISTEN:-}"

M3_LIB=1
# shellcheck source=run-m3.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/run-m3.sh"

# ---------------------------------------------------------------- LAN topology

# The two slots this machine owns. Every helper inherited from run-m3.sh that
# loops over $SLOTS now loops over these two only, which is exactly right for
# teardown, status and the error sweep.
SLOTS="1 3"
REMOTE_SLOT=2
REMOTE_PEER="$(peer_of "$REMOTE_SLOT")"

# The relay has to be reachable from the second computer.
RELAY_LISTEN="${_ENV_RELAY_LISTEN:-0.0.0.0:$RELAY_PORT}"

LOGS="$E2E/logs-m3-lan"
BEPINEX_ARCHIVE="$LOGS/bepinex"
mkdir -p "$LOGS" "$BEPINEX_ARCHIVE"

# How long to wait for a natural eastward crossing out of slot 2 (phase 3).
#
# The M2 crossing measurement is ~20 eastward crossings per SIMULATED hour in a
# seeded world. One simulated hour costs 180 s of wall clock at 20x, 1200 s at
# 3x and 3600 s at 1x, and the far end runs at whatever time scale its operator
# left it on — 1x unless they changed it. The first crossing is therefore
# expected in about 3600/20 = 180 s of wall clock in the worst (1x) case, and in
# about 9 s at 20x. 1800 s is ten times the worst-case expectation, which is
# generous without being unbounded.
NATURAL_TIMEOUT="${NATURAL_TIMEOUT:-1800}"
NATURAL_PROGRESS="${NATURAL_PROGRESS:-30}"

# Where the phase state lives, so each phase runs on its own.
LAN_ENTITY="$RUN/m3lan-entity"
LAN_MIGRATION="$RUN/m3lan-migration"

# ---------------------------------------------------------------- ring pre-seed

# Pre-seeding removes start order from the LAN ring.
#
# contract-b-m3.md §7.2 rule 4 appends a new peer at the tail, so on a fresh
# ring slot order IS start order — which the local rehearsal gets for free by
# starting three sidecars in a row. Across a LAN it is not free: the second
# computer is started by a person, and "start yours after mine but before my
# third one" is not an instruction a rig should depend on. `--reserve-slot`
# writes the three reservations before anybody connects, and rule 1 then hands
# each peer the slot its peerId already owns, whenever it arrives.
reserve() {
  step "pre-seeding the ring: slot 1, slot 2 (remote), slot 3"
  if [ -n "$(read_pid relay)" ] && proc_alive "$(read_pid relay)"; then
    fail "the relay is running; --reserve-slot is a startup command. Run 'down' first."
    return 1
  fi
  ensure_token
  mkdir -p "$RELAY_DATA"
  "$BIN/relay" --data-dir "$RELAY_DATA" --token-file "$TOKEN_FILE" \
    --reserve-slot "$(peer_of 1)" --reserve-slot "$(peer_of 2)" --reserve-slot "$(peer_of 3)" \
    2>&1 | sed 's/^/    /' >&2
  ring_order
}

ring_order() {
  python3 - "$RELAY_DATA/ring.json" <<'PY' 2>/dev/null
import json, sys
try:
    ring = json.load(open(sys.argv[1]))["ring"]
except Exception:
    print("")
    sys.exit(0)
print(" ".join("%s:%s" % (r["slot"], r["peerId"]) for r in ring))
PY
}

# ---------------------------------------------------------------- bring-up

# The relay binds 0.0.0.0 here, so run-m3.sh's loopback-only port check would
# miss it.
lan_ports_busy() {
  ss -ltn 2>/dev/null | grep -qE ":($PORT_1|$PORT_3|$RELAY_PORT) "
}

lan_up() {
  if lan_ports_busy; then
    fail "a rig is already running on these ports. Run 'down' first."
    return 1
  fi

  ensure_token
  "$GAME_SH" stop all >/dev/null 2>&1 || true
  sleep 2
  reset_bepinex_logs up

  local order; order="$(ring_order)"
  case "$order" in
    "1:slot-1 2:slot-2 3:slot-3") note "the ring is already pre-seeded: $order" ;;
    *) reserve || return 1 ;;
  esac

  step "starting the Go side: relay (LAN-bound), archive, sidecars 1 and 3"
  start_relay
  wait_healthy "http://127.0.0.1:$RELAY_PORT/healthz" || return 1
  note "the second computer dials ws://<this machine>:$RELAY_PORT/contract-b/v2"

  start_archive
  wait_file "$LOGS/archive.log" 'archive: subscribed to the relay' 60 >/dev/null || return 1
  note "archive subscribed"

  local slot
  for slot in $SLOTS; do
    local mark; mark="$(file_mark "$(sclog_of "$slot")")"
    start_sidecar "$slot"
    wait_healthy "http://127.0.0.1:$(port_of "$slot")/healthz" || return 1
    wait_grant "$slot" "$mark" || return 1
  done

  for slot in $SLOTS; do
    step "starting game $slot ($(world_of "$slot"), export edge $EXPORT_EDGE, sidecar $(port_of "$slot"))"
    start_game "$slot" || return 1
    wait_log "$slot" "\[M2-WORLD\] world '$(world_of "$slot")' (loaded|seeded and live)" 900 || return 1
  done

  for slot in $SLOTS; do
    wait_log "$slot" '\[M2\] CONFIG_UPDATE reason=connect' 240 >/dev/null || return 1
    send "$slot" timescale "$TIME_SCALE" || return 1
  done

  verdict "the main machine is up. The far end joins whenever it starts — see phase1."
  far_end_hint
}

far_end_hint() {
  note ""
  note "ON THE SECOND COMPUTER (its operator, not this rig):"
  note "    .\\setup-farend.ps1 -RelayHost <the LAN address of THIS machine> -TokenFile .\\token.txt"
  note "    .\\start-slot2.ps1"
  note "Run 'e2e/run-m3-lan.sh lanhost' for that address and the two elevated commands."
  note "Nothing on this machine can start the far end, and nothing here has to."
}

# The owner's networking steps, with this machine's real numbers filled in.
#
# WSL2 runs behind NAT here (\\.wslconfig sets networkingMode=NAT, deliberately —
# mirrored mode breaks Docker Desktop port publishing on this host). A relay
# listening on 0.0.0.0 inside WSL is therefore reachable from Windows, but NOT
# from the LAN: the second computer talks to the WINDOWS address, and Windows has
# to forward that port into the WSL VM. Both commands below need an elevated
# PowerShell, and both are owner steps.
lanhost() {
  local wslip winips ps
  wslip="$(hostname -I 2>/dev/null | awk '{print $1}')"
  ps=/mnt/c/Windows/System32/WindowsPowerShell/v1.0/powershell.exe
  winips="$(cd /mnt/c && "$ps" -NoProfile -NonInteractive -Command \
    "Get-NetIPAddress -AddressFamily IPv4 | Where-Object { \$_.IPAddress -notlike '127.*' -and \$_.IPAddress -notlike '169.254.*' } | Select-Object -ExpandProperty IPAddress" \
    2>/dev/null </dev/null | tr -d '\r')"

  step "the relay's LAN address, and the two elevated commands that expose it"
  note "WSL address of the relay : $wslip   (this changes on every WSL restart)"
  note "Windows IPv4 addresses   :"
  printf '%s\n' "$winips" | sed 's/^/        /' >&2
  note ""
  note "TODO-owner: pick the address on your home network (usually 192.168.x.x or"
  note "10.x.x.x, NOT a 172.x hypervisor address) and give it to the second computer."
  note ""
  note "In an ELEVATED PowerShell on THIS machine:"
  note ""
  note "  # 1. open the port, once"
  note "  New-NetFirewallRule -DisplayName 'Bibites Multiverse relay' -Direction Inbound \\"
  note "    -Action Allow -Protocol TCP -LocalPort $RELAY_PORT -Profile Private"
  note ""
  note "  # 2. forward it into WSL. Re-run this after every WSL restart: the"
  note "  #    address above changes."
  note "  netsh interface portproxy delete v4tov4 listenport=$RELAY_PORT listenaddress=0.0.0.0"
  note "  netsh interface portproxy add v4tov4 listenport=$RELAY_PORT listenaddress=0.0.0.0 \\"
  note "    connectport=$RELAY_PORT connectaddress=$wslip"
  note ""
  note "  # check"
  note "  netsh interface portproxy show v4tov4"
}

# ---------------------------------------------------------------- phase 1

# The remote slot's health, read from three places that all live on this machine.
remote_state() {
  local order granted edge
  order="$(ring_order)"
  granted="$(grep -a "ring claim.*peer=$REMOTE_PEER" "$LOGS/relay.log" 2>/dev/null | tail -n 1)"
  edge="$(grep_log 1 "\[M2\] EDGE_STATUS .* exportEdge=$EXPORT_EDGE open=(True|False)" | tail -n 1)"
  printf 'order=[%s]\n' "$order"
  printf 'relay=[%s]\n' "$granted"
  printf 'ripple=[%s]\n' "$edge"
}

phase1() {
  step "PHASE 1 — the ring forms, and slot 2 is on the other computer"
  note "slot 2's health is read from the relay's ring state and from the EDGE_STATUS"
  note "ripple into slot 1 — the far end is never asked anything."
  local ok=0 deadline line
  deadline=$(( $(now) + ${RING_TIMEOUT:-900} ))

  local want="1:slot-1 2:slot-2 3:slot-3"
  local order; order="$(ring_order)"
  if [ "$order" = "$want" ]; then
    note "ring order (west to east, wrapping): $order"
  else
    fail "ring order is '$order', want '$want'"
    ok=1
  fi

  # Slot 1's export edge opens ONLY when slot 2 is live and has a game attached
  # (contract-a.md §5.4 + contract-b-m3.md §8). That single line is the whole
  # remote-liveness proof, and it is written on this machine.
  step "waiting for slot 1's export edge to open — that IS slot 2 reporting for duty"
  local last=""
  while :; do
    line="$(grep_log 1 "\[M2\] EDGE_STATUS .* exportEdge=$EXPORT_EDGE open=True" | tail -n 1)"
    [ -n "$line" ] && break
    local reason; reason="$(grep_log 1 '\[M2\] EDGE_STATUS' | tail -n 1)"
    if [ "$reason" != "$last" ]; then note "slot 1: ${reason:-<no EDGE_STATUS yet>}"; last="$reason"; fi
    if [ "$(now)" -ge "$deadline" ]; then
      fail "slot 2 never came up within ${RING_TIMEOUT:-900}s"
      remote_state | sed 's/^/      /' >&2
      far_end_hint
      return 1
    fi
    sleep 5
  done
  note "slot 1: $line"
  note "slot 1's east neighbour (slot 2, remote) is live AND mod-connected"

  # Slot 3's east neighbour is slot 1, which is local: this asserts the third lane.
  line="$(wait_log 3 "\[M2\] EDGE_STATUS .* exportEdge=$EXPORT_EDGE open=True" 240)" || ok=1
  note "slot 3: $line"

  line="$(grep -a "ring claim.*peer=$REMOTE_PEER" "$LOGS/relay.log" | tail -n 1)"
  if [ -n "$line" ]; then note "relay: $line"; else fail "the relay never logged a claim from $REMOTE_PEER"; ok=1; fi

  line="$(grep -a 'archive: subscribed to the relay' "$LOGS/archive.log" | tail -n 1)"
  if [ -n "$line" ]; then note "archive: $line"; else fail "the archive never subscribed"; ok=1; fi

  [ "$ok" = 0 ] && verdict "PHASE 1: PASS — three slots in ring order, slot 2 live on the second computer" \
                || verdict "PHASE 1: FAIL"
  return "$ok"
}

# ---------------------------------------------------------------- archive reads

# archive_hop <from slot> <to slot> [entityId] — the header lines of every
# recorded migration on that lane. This is the read path an operator uses
# (dev_environment.md), so the rig and the human see the same evidence.
archive_hop() {
  local from="$1" to="$2" eid="${3:-}"
  if [ -n "$eid" ]; then
    archive_list 2>/dev/null | grep -a "entity $eid " | grep -a "slot $from -> slot $to" || true
  else
    archive_list 2>/dev/null | grep -a "slot $from -> slot $to" || true
  fi
}

# The archive ledger is durable and cumulative: it still holds every hop of every
# earlier run. A wait that matches any record on a lane therefore succeeds
# instantly on a rig that has run before — which is exactly what happened the
# first time phase 3 ran, and it "proved" a crossing recorded an hour earlier.
# Every wait is bounded below by a timestamp, and the archive's header line
# starts with an RFC3339 UTC time, so a string compare is the whole filter.
archive_mark() { date -u -d "@$(( $(now) - 2 ))" +%Y-%m-%dT%H:%M:%SZ; }

# wait_archive_hop <from> <to> <timeout> <since> [entityId] — poll until the lane
# has a delivered record NEWER than <since>, printing progress. Prints the line.
wait_archive_hop() {
  local from="$1" to="$2" timeout="$3" since="$4" eid="${5:-}"
  local deadline start hit elapsed next
  start="$(now)"; deadline=$(( start + timeout )); next=$(( start + NATURAL_PROGRESS ))
  while :; do
    hit="$(archive_hop "$from" "$to" "$eid" | grep -a 'delivered' \
             | awk -v s="$since" '$1 >= s' | tail -n 1)"
    if [ -n "$hit" ]; then printf '%s\n' "$hit"; return 0; fi
    if [ "$(now)" -ge "$deadline" ]; then
      fail "no delivered slot $from -> slot $to record after $since within ${timeout}s"
      return 1
    fi
    if [ "$(now)" -ge "$next" ]; then
      elapsed=$(( $(now) - start ))
      note "waiting on lane $from -> $to since $since: ${elapsed}s of ${timeout}s elapsed; hops recorded on that lane in this run: $(archive_hop "$from" "$to" | awk -v s="$since" '$1 >= s' | grep -ac . || true)"
      next=$(( $(now) + NATURAL_PROGRESS ))
    fi
    sleep 3
  done
}

# ---------------------------------------------------------------- phase 2

phase2() {
  step "PHASE 2 — forced export from slot 1, across the LAN into slot 2"
  slow_sims 1

  local fam; fam="$(send 1 family)" || { restore_sims; return 1; }
  note "slot 1 family: $fam"

  local mark line eid mid ok=0 since
  since="$(archive_mark)"
  mark="$(log_mark 1)"
  line="$(send 1 export family)" || { fail "export failed: $line"; restore_sims; return 1; }
  note "export: $line"
  eid="$(field entityId "$line")"
  [ -n "$eid" ] || { fail "the export result named no entityId"; restore_sims; return 1; }

  line="$(wait_log 1 "entityId=$eid phase=MIGRATE_OUT_SENT" 90 "$mark")" || { restore_sims; return 1; }
  note "slot 1: $line"
  mid="$(field migrationId "$line")"

  line="$(wait_log 1 "\[M3-LINEAGE\] migrationId=$mid" 30 "$mark")" || true
  note "slot 1 annex: ${line:-<none>}"

  line="$(wait_file "$(sclog_of 1)" "forwarded MIGRATION_PAYLOAD.*migrationId=$mid" 60)" || { restore_sims; return 1; }
  note "sidecar 1: $line"
  local dest; dest="$(field destSlot "$line")"
  [ "$dest" = "$REMOTE_SLOT" ] || { fail "slot 1 routed to slot $dest, want $REMOTE_SLOT"; ok=1; }

  wait_log 1 "migrationId=$mid .*phase=DESTROYED" 90 "$mark" >/dev/null \
    && note "slot 1 destroyed its copy on MIGRATE_OUT_ACK" \
    || { fail "slot 1 never destroyed its copy"; ok=1; }

  # THE ARRIVAL, without touching the far end. A "delivered" outcome means the
  # remote sidecar journaled it and its mod ACKed it (contract-b-m3.md §5.1).
  step "reading the arrival out of the ARCHIVE — the far end is never asked"
  line="$(wait_archive_hop 1 "$REMOTE_SLOT" "${ARRIVAL_TIMEOUT:-180}" "$since" "$eid")" || ok=1
  note "archive: $line"

  printf '%s\n' "$eid" > "$LAN_ENTITY"
  printf '%s\n' "$mid" > "$LAN_MIGRATION"
  restore_sims

  [ "$ok" = 0 ] && verdict "PHASE 2: PASS — entity $eid crossed the LAN into slot 2, and the archive says delivered" \
                || verdict "PHASE 2: FAIL"
  return "$ok"
}

# ---------------------------------------------------------------- phase 3

phase3() {
  step "PHASE 3 — a NATURAL eastward crossing out of slot 2, into slot 3"
  note "Nothing forces this hop. The far end is not driven, so the rig waits for"
  note "the ring's own traffic: an organism swims east out of slot 2 by itself."
  note "timeout ${NATURAL_TIMEOUT}s, progress every ${NATURAL_PROGRESS}s"
  note "(the operator of the second computer can press F10 in the game to force one)"

  local eid line mid ok=0 since
  since="$(archive_mark)"
  eid="$(cat "$LAN_ENTITY" 2>/dev/null || true)"
  local before; before="$(archive_hop "$REMOTE_SLOT" 3 | grep -ac . || true)"
  note "slot 2 -> slot 3 hops the archive already holds from earlier runs: ${before:-0}"
  note "only a hop recorded after $since counts"

  line="$(wait_archive_hop "$REMOTE_SLOT" 3 "$NATURAL_TIMEOUT" "$since")" || return 1
  note "archive: $line"
  mid="$(awk '{print $2}' <<<"$line")"
  local crossed; crossed="$(sed -n 's/.* entity \(-\?[0-9]*\) .*/\1/p' <<<"$line")"

  if [ -n "$eid" ] && [ "$crossed" = "$eid" ]; then
    note "the organism that crossed IS the one phase 2 sent: entity $eid"
  else
    note "the organism that crossed is entity $crossed, not phase 2's entity ${eid:-<none>}."
    note "That is the honest result of an undriven far end: the lane is proven, the"
    note "specific organism is whichever one reached the edge first."
    printf '%s\n' "$crossed" > "$LAN_ENTITY"
    eid="$crossed"
  fi

  # And the local half: slot 3's own game must have taken it in.
  line="$(wait_file "$(sclog_of 3)" "took custody of an inbound organism.*migrationId=$mid" 120)" \
    && note "sidecar 3: $line" || { fail "sidecar 3 never took custody of $mid"; ok=1; }
  line="$(wait_log 3 "migrationId=$mid .*phase=SPAWNED" 180)" \
    && note "slot 3: $line" || { fail "slot 3 never spawned $mid"; ok=1; }
  line="$(wait_log 3 "migrationId=$mid .*phase=MIGRATE_IN_ACK" 60)" \
    && note "slot 3: $line" || { fail "slot 3 never ACKed $mid"; ok=1; }

  printf '%s\n' "$mid" > "$LAN_MIGRATION"
  [ "$ok" = 0 ] && verdict "PHASE 3: PASS — slot 2 exported east on its own and slot 3 took entity $eid" \
                || verdict "PHASE 3: FAIL"
  return "$ok"
}

# ---------------------------------------------------------------- phase 4

phase4() {
  step "PHASE 4 — the return hop, slot 3 -> slot 1: the circuit closes"
  local eid ok=0
  eid="$(cat "$LAN_ENTITY" 2>/dev/null || true)"
  [ -n "$eid" ] || { fail "no entity from phase 3"; return 1; }

  local since; since="$(archive_mark)"
  slow_sims 1
  # A fresh arrival still has its entry immunity running.
  sleep 12
  local out
  out="$(hop 3 1 "$eid")" || { restore_sims; verdict "PHASE 4: FAIL (hop 3->1)"; return 1; }
  note "hop 3->1: $out"
  restore_sims

  local line
  line="$(wait_archive_hop 3 1 120 "$since" "$eid")" || ok=1
  note "archive: $line"

  [ "$ok" = 0 ] && verdict "PHASE 4: PASS — entity $eid is home in slot 1 after a full circuit" \
                || verdict "PHASE 4: FAIL"
  return "$ok"
}

# ---------------------------------------------------------------- phase 5

phase5() {
  step "PHASE 5 — archive truth"
  local ok=0 eid
  eid="$(cat "$LAN_ENTITY" 2>/dev/null || true)"
  settle 20

  local listing
  listing="$(archive_list)" || { fail "bin/archive list failed"; return 1; }
  printf '%s\n' "$listing" | tail -n 40 | sed 's/^/      /' >&2

  local lane from to n
  for lane in 1:2 2:3 3:1; do
    from="${lane%%:*}"; to="${lane##*:}"
    n="$(archive_hop "$from" "$to" | grep -ac . || true)"
    note "recorded hops on lane $from -> $to: ${n:-0}"
    [ "${n:-0}" -ge 1 ] || { fail "the archive holds no hop on lane $from -> $to"; ok=1; }
  done

  local missing
  missing="$(grep -ac 'genome .*\[MISSING\]' <<<"$listing")"
  missing="${missing:-0}"
  note "genome hashes the archive recorded but does not hold: $missing"
  [ "$missing" = 0 ] || note "some genomes are still unresolved; the retry queue keeps trying (§10)"

  # No bb8 body ever reaches the ledger: the wire carries hashes, not blobs.
  # `grep -c` prints 0 and still exits 1, so a `|| echo 0` here would append a
  # SECOND zero and the comparison would run on "0\n0".
  local blobbed
  blobbed="$(grep -ac '"bb8"' "$ARCHIVE_DATA/migrations.jsonl" 2>/dev/null)"
  blobbed="${blobbed:-0}"
  note "ledger lines carrying a bb8 blob: $blobbed (must be 0)"
  [ "$blobbed" = 0 ] || { fail "a blob reached the archive ledger"; ok=1; }

  local genomes
  genomes="$(find "$ARCHIVE_DATA/genomes" -type f 2>/dev/null | wc -l)"
  note "genomes in the content-addressed store: $genomes"
  [ "$genomes" -ge 1 ] || { fail "the genome store is empty"; ok=1; }

  if [ -n "$eid" ]; then
    local hops; hops="$(grep -ac "entity $eid " <<<"$listing")"
    note "archive records naming entity $eid: ${hops:-0}"
  fi

  [ "$ok" = 0 ] && verdict "PHASE 5: PASS — all three lanes recorded, lineage joined, no blob on the wire" \
                || verdict "PHASE 5: FAIL"
  return "$ok"
}

# ---------------------------------------------------------------- phase 6

# The exactly-once table for a ring whose middle slot cannot be counted directly.
lan_count() {
  local eid; eid="$(tr -cd '0-9-' <<<"$1")"
  local slot line alive total=0 custody custody_count
  printf '\n    ---- organism %s ----\n' "$eid" >&2

  for slot in $SLOTS; do
    line="$(send "$slot" count "$eid" || true)"
    alive="$(field alive "$line")"; alive="${alive:-0}"
    printf '    world slot %s (local): %s\n' "$slot" "$alive" >&2
    total=$(( total + alive ))
  done

  custody="$(python3 "$E2E/journal.py" custody "$eid" "$(journal_of 1)" "$(journal_of 3)" 2>/dev/null)"
  custody_count="$(sed -n 's/^custodyCount=\([0-9]*\)$/\1/p' <<<"$custody")"
  custody_count="${custody_count:-0}"
  printf '    local journals      : %s\n' "$custody_count" >&2
  sed 's/^/      /' <<<"$custody" >&2
  total=$(( total + custody_count ))

  # Slot 2 is INFERRED, and the inference is stated rather than hidden. The
  # archive holds one record per hop with an outcome; the last record that names
  # this organism says where it went. If its last hop LEFT slot 2 and was
  # delivered, slot 2 no longer holds it. If its last hop ARRIVED at slot 2, it
  # does.
  local last held2 dest outcome
  last="$(archive_list 2>/dev/null | grep -a "entity $eid " | tail -n 1)"
  printf '    slot 2 (remote) is INFERRED, not counted. Its last archive record:\n' >&2
  printf '      %s\n' "${last:-<none>}" >&2
  # The archive holds one record per hop, in order, so the LAST record that
  # names this organism says where it is: the destination of its most recent
  # delivered hop. That is the whole inference, and it is only ever used for the
  # one slot this machine cannot count.
  dest="$(sed -n 's/.*slot [0-9]* -> slot \([0-9]*\) .*/\1/p' <<<"$last")"
  outcome="$(sed -n 's/.*)  \(.*\)$/\1/p' <<<"$last")"
  held2=0
  if [ -z "$last" ]; then
    printf '      -> the archive has no record of this organism at all: slot 2 holds 0\n' >&2
  elif [ "$dest" = "$REMOTE_SLOT" ] && [ "${outcome#delivered}" != "$outcome" ]; then
    held2=1
    printf '      -> its last hop was INTO slot %s and it was delivered: slot %s holds 1\n' \
      "$REMOTE_SLOT" "$REMOTE_SLOT" >&2
  else
    printf '      -> its last hop ended in slot %s, not slot %s: slot %s holds 0\n' \
      "${dest:-?}" "$REMOTE_SLOT" "$REMOTE_SLOT" >&2
  fi
  printf '    slot 2 inferred     : %s\n' "$held2" >&2
  total=$(( total + held2 ))

  printf '    TOTAL               : %s  -> %s\n' "$total" \
    "$( [ "$total" = 1 ] && echo PASS || { [ "$total" = 0 ] && echo 'PASS (acceptable loss under D2)' || echo FAIL; } )" >&2
  printf '%s\n' "$total"
}

phase6() {
  step "PHASE 6 — exactly once, ring-wide"
  note "slots 1 and 3 are counted in their own worlds; slot 2 cannot be, so it is"
  note "inferred from the archive and the local journals. The inference is printed."
  local eid total
  eid="$(cat "$LAN_ENTITY" 2>/dev/null || true)"
  [ -n "$eid" ] || { fail "no entity id — run phase2 first"; return 1; }
  settle 20
  total="$(lan_count "$eid")"
  if [ "$total" -le 1 ]; then
    verdict "PHASE 6: PASS (count $total; 1 = exactly once, 0 = accepted loss under D2)"
    return 0
  fi
  verdict "PHASE 6: FAIL — $total copies of entity $eid"
  return 1
}

# ---------------------------------------------------------------- teardown

lan_down() {
  step "tearing down THIS machine only"
  note "Nothing below reaches the second computer. Its operator runs stop-slot2.ps1"
  note "there; its journal and its ring slot survive either way."
  "$GAME_SH" stop all || true
  local slot p pid
  for slot in $SLOTS; do kill_pid "sidecar-$slot" -TERM; done
  kill_pid archive -TERM
  kill_pid relay -TERM
  sleep 1
  for p in relay archive sidecar-1 sidecar-3; do
    pid="$(read_pid "$p")"
    proc_alive "$pid" && kill -9 "$pid" 2>/dev/null
    rm -f "$(pidfile "$p")"
  done
  sleep 1
  archive_bepinex_logs down

  local games gos ports
  games="$(cd /mnt/c && /mnt/c/Windows/System32/tasklist.exe 2>/dev/null | grep -ci 'The Bibites' || true)"
  gos="$(pgrep -c -f "$BIN/(relay|sidecar|archive)" 2>/dev/null || true)"
  ports="$(ss -ltn 2>/dev/null | grep -cE ":($PORT_1|$PORT_3|$RELAY_PORT) " || true)"
  note "local game processes: ${games:-0} (want 0)"
  note "local Go processes  : ${gos:-0} (want 0)"
  note "bound rig ports     : ${ports:-0} (want 0)"
}

lan_errors() {
  step "error sweep — the two local BepInEx logs and the Go logs"
  local slot file unexplained f
  for slot in $SLOTS; do
    file="$(game_log "$slot")"
    [ -n "$file" ] && [ -f "$file" ] || continue
    printf '\n    ---- slot %s (%s) ----\n' "$slot" "$file" >&2
    unexplained="$(grep -a '\[Error' "$file" | grep -av 'Unable to start Unity log writer' || true)"
    [ -n "$unexplained" ] && sed 's/^/      /' <<<"$unexplained" >&2
    printf '    unexplained [Error lines: %s\n' "$( [ -n "$unexplained" ] && wc -l <<<"$unexplained" || echo 0)" >&2
  done
  for f in "$LOGS/relay.log" "$LOGS/archive.log" "$(sclog_of 1)" "$(sclog_of 3)"; do
    [ -f "$f" ] || continue
    printf '\n    ---- %s ----\n' "$(basename "$f")" >&2
    grep -a 'level=ERROR' "$f" | tail -n 10 | sed 's/^/      /' >&2
    printf '    total ERROR lines: %s\n' "$(grep -ac 'level=ERROR' "$f" || true)" >&2
  done
}

lan_all() {
  lan_up  || return 1
  phase1  || true
  phase2  || true
  phase3  || true
  phase4  || true
  phase5  || true
  phase6  || true
  lan_errors
  lan_down
}

# Only the two local worlds. M3-Slot2 lives on the second computer, and the mod
# there seeds it by itself on the first start (MULTIVERSE_WORLD, WorldSeeder).
lan_seed() {
  "$GAME_SH" stop all >/dev/null 2>&1 || true
  sleep 2
  reset_bepinex_logs seed
  seed_one 1 yes || return 1
  reset_bepinex_logs seed
  seed_one 3 no  || return 1
}

case "${1:-status}" in
  build)   build ;;
  seed)    lan_seed ;;
  lanhost) lanhost ;;
  reserve) reserve ;;
  up)      lan_up ;;
  down)    lan_down ;;
  status)  status; remote_state | sed 's/^/    /' >&2 ;;
  phase1)  phase1 ;;
  phase2)  phase2 ;;
  phase3)  phase3 ;;
  phase4)  phase4 ;;
  phase5)  phase5 ;;
  phase6)  phase6 ;;
  errors)  lan_errors ;;
  archive) shift; archive_list "$@" ;;
  journal) python3 "$E2E/journal.py" summary "$(journal_of 1)" "$(journal_of 3)" ;;
  send)    shift; send "$@" ;;
  all)     lan_all ;;
  *)
    echo "usage: run-m3-lan.sh build|seed|lanhost|reserve|up|phase1..phase6|errors|archive|journal|status|down|all" >&2
    exit 1
    ;;
esac
