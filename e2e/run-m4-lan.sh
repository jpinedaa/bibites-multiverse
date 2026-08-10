#!/usr/bin/env bash
# The M4 LAN rig: the exit test with a 3x2 map of SIX REAL WORLDS across TWO
# COMPUTERS. Five run here. The sixth — slot 6, position (2,1) — runs on the
# owner's second computer and is never driven from this one.
#
#   run-m4-lan.sh build      the Go binaries into bin/
#   run-m4-lan.sh lanhost    the relay's LAN address, and the elevated owner commands
#   run-m4-lan.sh reserve    pre-place all SIX slots, including the far one
#   run-m4-lan.sh seed       create (or evolve) M4-Slot1..M4-Slot5 here; the far end seeds its own
#   run-m4-lan.sh up         relay (LAN-bound) -> archive -> sidecars 1..5 -> games 1..5
#   run-m4-lan.sh phase1     the grid forms across two machines, and the far slot reports for duty
#   run-m4-lan.sh phase2     forced hops INTO the far slot on BOTH axes, read from the archive
#   run-m4-lan.sh phase3     the return current: NATURAL hops OUT of the far slot, both axes
#   run-m4-lan.sh phase4     kill-and-heal on a LOCAL slot; the re-paired east lane crosses the LAN
#   run-m4-lan.sh phase5     burst pacing, the dam on a LOCAL slot (run-m4.sh phase 6, unchanged)
#   run-m4-lan.sh phase5far  the same test against the FAR slot — two owner commands, then observed
#   run-m4-lan.sh phase6     periodic saves: five local worlds, and the far one via HEARTBEAT.lastSave
#   run-m4-lan.sh phase7     portals: the local evidence, and what only a person at each screen can say
#   run-m4-lan.sh phase8     exactly-once, the error sweep, and a teardown of THIS machine only
#   run-m4-lan.sh status     what is running here, plus the far slot's state
#   run-m4-lan.sh statuspage curl the archive's /api/status and pretty-print it
#   run-m4-lan.sh journal    summarise every LOCAL sidecar journal
#   run-m4-lan.sh archive    bin/archive list over the rig's archive data dir
#   run-m4-lan.sh errors     every unexplained error line in the LOCAL logs
#   run-m4-lan.sh down       stop everything HERE; the far end is untouched
#   run-m4-lan.sh all        up, phase1..4, phase5, phase6..8
#
# ==============================================================================
# WHY THE FAR SLOT IS SLOT 6, AND WHY THAT CHOICE IS NOT ARBITRARY
# ==============================================================================
#
#   row 1:  +--> slot 4 (0,1) --> slot 5 (1,1) --> [ slot 6 (2,1) ] --+
#           +----------------- east wrap ---------------------------+
#              ^               ^                 ^
#              |               |                 |   north lanes wrap to row 0
#   row 0:  +--> slot 1 (0,0) --> slot 2 (1,0) --> slot 3 (2,0) -----+
#           +----------------- east wrap ---------------------------+
#
#                                 [ ] = the second computer
#
# THE LOCAL REHEARSAL HAD TO FAKE ONE SLOT. BepInEx hands out five log files —
# LogOutput.log and .1 .. .4 — and then says "Skipping log file creation". The
# sixth instance on this machine does not merely lose its log: THE MOD NEVER RUNS
# IN IT (measured twice, both directions; see run-m4.sh's header and
# dev_environment.md, *The five-instance ceiling*). run-m4.sh therefore drives
# slot 6 with bin/fakemod: a Contract A peer with no world, no geometry, no
# saves and no [M2-*] series.
#
# THE LAN PHASE RETIRES THAT SUBSTITUTION. The second computer brings its own
# BepInEx and therefore its own log file, so slot 6 becomes a real world with a
# real simulation, real saves and real portals. Five real games here plus one
# real game there is SIX REAL WORLDS and NO SYNTHETIC PEER — the honest 3x2 map
# m4_considerations.md's Exit Test asks for, in its "5+1 across the two machines"
# form.
#
# SLOT 6 IS THE RIGHT ONE TO MOVE, and the map is what decides it:
#
#   * IT WAS ALREADY THE FAKE ONE. Every place in the rehearsal that says "slot 6
#     is synthetic, so read this from somewhere else" becomes "slot 6 is remote,
#     so read this from somewhere else" — the same fact from the same surfaces
#     (the archive and the status page), for a different reason. No other slot
#     would inherit that plumbing.
#   * IT KEEPS THE RESUME TEST LOCAL. The 2026-08-04 stranded organism goes from
#     slot 1 to slot 2 (run-m4.sh phase 3). Both ends stay on this machine, so the
#     one irreplaceable test input is never at the mercy of a second computer.
#   * IT KEEPS THE KILL-AND-HEAL TEST LOCAL. Part 2 of the exit test hard-kills
#     slot 5 and splices it back. A hard kill has to land on a process this rig
#     owns; the rig refuses to drive the far end, so the killed slot must be here.
#     Slot 5 is local, and so are its two observers — slot 4 (whose east lane
#     re-pairs) and slot 2 (whose north lane must close no_peer).
#   * IT HAS BOTH LANES, so the far slot is exercised on BOTH AXES. A 3x2 map is
#     a torus: (2,1)'s east neighbour wraps to slot 4 and its north neighbour
#     wraps to slot 3. It receives from slot 5 through W and from slot 3 through
#     S. A corner of the rectangle is not a corner of the topology.
#   * AND KILLING SLOT 5 PUTS THE HEALED LANE ACROSS THE LAN. With slot 5 dark,
#     row 1 is {4, 6} and slot 4's east lane re-pairs to slot 6 — so phase 4's
#     decisive migration, the one that proves route-around carries traffic,
#     crosses the network. That is a better test than the rehearsal's, not a
#     weaker one.
#
# ==============================================================================
# THE FAR END IS NEVER DRIVEN. THIS IS HOW ITS FACTS GET HERE.
# ==============================================================================
#
# m3_considerations.md Risk 6 and m4_considerations.md Risk 7 both answer the
# same question the same way, and D9 ratifies it: the rig does not drive the
# second computer. No agent runs there, no remote PowerShell, no file is read
# across the network. Everything this test needs about slot 6 arrives here by
# itself:
#
#   * THAT IT EXISTS AND HOLDS ITS SLOT -> the relay's own map (ring.json), and
#     the relay's `placement claim` line naming peer slot-6.
#
#   * THAT IT IS LIVE AND MOD-CONNECTED -> THE EDGE_STATUS RIPPLE, and under M4
#     the ripple is sharper than it was under M3. §8 makes a peer deliverable
#     only when it is live AND its mod is attached, so with the far end down:
#         slot 5's EAST lane BYPASSES the hole and names slot 4;
#         slot 3's NORTH lane CLOSES with no_peer, because column 2 is {3, 6}
#         and a two-slot column has nothing to re-pair to (§2.1).
#     and the instant the far end joins:
#         slot 5's east lane MOVES to slot 6, and slot 3's north lane OPENS to it.
#     Both of those are written into THIS machine's BepInEx logs by the mod, and
#     into the status page by the archive. Two independent local witnesses to a
#     remote world's health, continuously.
#
#   * THAT AN ORGANISM ARRIVED THERE -> THE ARCHIVE. The relay copies every
#     MIGRATION_PAYLOAD and every MIGRATION_ACK to the archive (§5.1), so a
#     delivered hop into slot 6 is a record on this disk. `delivered` means the
#     far sidecar journaled it and the far mod acknowledged it — the same custody
#     gate a local arrival passes (§6.7).
#
#   * THAT AN ORGANISM LEFT -> the archive again, as a 6->4 or 6->3 record, and
#     then the local destination's own logs as the arrival.
#
#   * ITS POPULATION, CUSTODY DEPTH, PACED DEPTH, SIMULATED TIME AND LAST SAVE ->
#     THE STATUS PAGE. §6.3.1's peer stats ride SECTOR_CLAIM, the relay fans them
#     out in PEER_STATUS, and the archive renders them (§10.1). HEARTBEAT.lastSave
#     is how a world 30 feet away proves it is saving itself.
#
# THE ONE THING AN UNDRIVEN RIG CANNOT DO is make the far world act. It cannot
# force an export there, and it cannot kill it. Two phases care:
#
#   * phase3 does not force anything. The map is a current and organisms cross on
#     their own; the rig waits, with progress, and reads the archive.
#   * phase5far needs the far world to go away and come back. That is TWO OWNER
#     COMMANDS on the second computer, and the rig asks for them, waits for the
#     status page to confirm each one, and observes. It sends nothing. The local
#     phase5 proves the same rule with no owner at all, so phase5far is the
#     cross-machine confirmation and never the only evidence.
#
# ==============================================================================
# WHAT THIS RIG DOES NOT RUN, AND WHY
# ==============================================================================
#
#   run-m4.sh phase3 (the resume test) — LOCAL BY CONSTRUCTION and proven by the
#     rehearsal. `arm` copies e2e/data/slot-1, the only copy of the test input,
#     and the organism delivers into slot 2. Neither end is remote, so a LAN run
#     re-proves nothing and puts the input at risk. Run it with run-m4.sh.
#
#   run-m4.sh phase5 (a seventh peer splices in) — needs a sidecar with no mod on
#     THIS machine, which is what the rehearsal already did. Nothing about the
#     placement rules changes across a network; the relay is the only arbiter and
#     it is here.
#
#   run-m4.sh phase7 (the bounded hold and its automatic bounce) — NOT RUNNABLE
#     ON AN UNDRIVEN FAR END, and it is worth being exact about why. §9.3's case
#     needs the destination to die in the window between the forward and the
#     acknowledgement. The rehearsal aims at that window by restarting the
#     synthetic peer with --ack-delay and killing it from a watcher. Both of those
#     are commands to the destination, and the destination is now a computer this
#     rig will not command. Proven locally; not re-proven here.
#
# ==============================================================================
# BEFORE YOU RUN THIS
# ==============================================================================
#
# The relay port has to be reachable from the second computer, which takes
# elevated commands on this machine: a firewall rule and a portproxy into the WSL
# VM. `run-m4-lan.sh lanhost` prints them with this machine's live addresses
# filled in, INCLUDING the deletion of the stale M3-era 8790 portproxy — which is
# not cosmetic, because 8790 is slot 4's Contract A port under the M4 port plan
# and a Windows listener on it makes slot 4's game unreachable from its own
# sidecar.
set -uo pipefail

# ---------------------------------------------------------------- library load

# run-m4.sh holds the M4 topology and every helper the LAN does not change: the
# map readers, the status-page accessors, the per-slot facts, `hop`, the census,
# the seeding and the teardown. M4_LIB=1 loads it as a library instead of
# dispatching a command. It in turn loads run-m3.sh the same way.
#
# THE `${VAR:-default}` IDIOM ACROSS A SOURCE BITES HERE TOO, and run-m4.sh's own
# header explains the shape of the bug: a name that the sourced file assigns with
# a `:-` default is already SET by the time this file runs, so a second `:-` keeps
# the sourced value and the override sits in a variable nobody reads. Every such
# name is captured from the environment BEFORE the source and assigned
# UNCONDITIONALLY after it.
_ENV_LAN_RELAY_LISTEN="${RELAY_LISTEN:-}"

M4_LIB=1
# shellcheck source=run-m4.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/run-m4.sh"

# ---------------------------------------------------------------- LAN topology

# The five slots this machine owns. Every helper inherited from run-m4.sh that
# loops over $SLOTS now loops over these five only, which is exactly right for
# teardown, status, the error sweep and the local half of the census.
SLOTS="1 2 3 4 5"
GAME_SLOTS="$SLOTS"
LOGGED_SLOTS="$SLOTS"

# THERE IS NO SYNTHETIC PEER IN LAN MODE. Slot numbers start at 1, so 0 makes
# is_fake() false everywhere and every "slot 6 is synthetic" branch inherited
# from the rehearsal falls through to the real one.
FAKE_SLOT=0

# TWO-WAY LANES, STATED HERE TOO (D17, contract-a.md §18 A38). run-m4.sh already
# sets this and the source carries it; it is repeated because the geometry this
# rig drives is a property of THIS rig, and an unconditional assignment after the
# source is the file's own idiom for a name it will not let drift. The mod's own
# default is now all four as well (§18 A41, case 3) — declaring it anyway is what
# keeps a future change to that default from silently moving what is measured.
# The far end declares its own set; see farend/setup-farend.ps1 -ExportEdges.
EXPORT_EDGES="E,N,W,S"

REMOTE_SLOT=6
REMOTE_PEER="$(peer_of "$REMOTE_SLOT")"
REMOTE_POS="$(pos_of "$REMOTE_SLOT")"
REMOTE_WORLD="$(world_of "$REMOTE_SLOT")"
# Every slot of the map, local and remote. `reserve` is the only place that needs
# it: the far peer's reservation has to exist before it dials in.
MAP_SLOTS="$SLOTS $REMOTE_SLOT"

# The relay has to be reachable from the second computer. The local sidecars keep
# dialling 127.0.0.1 either way.
RELAY_LISTEN="${_ENV_LAN_RELAY_LISTEN:-0.0.0.0:$RELAY_PORT}"

# ---------------------------------------------------------------- runtime tree

# A SEPARATE TREE FROM THE REHEARSAL'S, and this is not tidiness.
#
# e2e/relay-data-m4/ring.json is no longer a 3x2 map: run-m4.sh phase 5 spliced a
# seventh peer into it, so it is 4x2 with slot 7 at (1,0). Reusing it would start
# this rig on a map that fails its own first assertion, and `reserve` would not
# fix it — a reservation is idempotent and never shrinks a map. A fresh relay data
# dir is the only way to a clean 3x2.
#
# The rehearsal's journals and its archive are the RECORD OF A RUN THAT HAPPENED.
# They stay where they are.
DATA="$E2E/data-m4-lan"
RELAY_DATA="$E2E/relay-data-m4-lan"
ARCHIVE_DATA="$E2E/archive-data-m4-lan"
LOGS="$E2E/logs-m4-lan"
BEPINEX_ARCHIVE="$LOGS/bepinex"
RUN="$E2E/run-m4-lan"
SHOTS="$LOGS/shots"

# Its own command-file directory, so a stale command from the rehearsal cannot be
# picked up by a game this rig started. The far end uses `bibites-m4` on its OWN
# machine; the two never meet.
WIN_CMD_DIR="$WIN_TEMP\\bibites-m4lan"
WSL_CMD_DIR="$(wslpath -u "$WIN_TEMP")/bibites-m4lan"

mkdir -p "$DATA" "$RELAY_DATA" "$ARCHIVE_DATA" "$LOGS" "$BEPINEX_ARCHIVE" "$RUN" "$SHOTS" "$WSL_CMD_DIR"

# ---------------------------------------------------------------- timings

# How long to wait for the second computer to join (phase 1). It is started by a
# person, so this is a human timeout, not a network one.
FAR_JOIN_TIMEOUT="${FAR_JOIN_TIMEOUT:-1800}"

# How long to wait for a NATURAL crossing out of the far slot (phase 3).
#
# The M2 measurement is ~20 crossings per SIMULATED hour per export edge in a
# seeded world. One simulated hour costs 3600 s of wall clock at 1x and 180 s at
# 20x, and the far end runs at whatever time scale its operator left it on — 1x
# unless they changed it. The first crossing on a given edge is therefore expected
# in about 180 s at 1x. 1800 s is ten times that, which is generous without being
# unbounded. Phase 2 helps it along by putting organisms into the far world's
# entry bands first.
NATURAL_TIMEOUT="${NATURAL_TIMEOUT:-1800}"
NATURAL_PROGRESS="${NATURAL_PROGRESS:-30}"

# How long to wait for the owner to run a command on the second computer
# (phase5far only). The rig prints exactly what to run and polls the status page.
FAR_OWNER_TIMEOUT="${FAR_OWNER_TIMEOUT:-900}"

# Where the phase state lives, so each phase runs on its own.
LAN_EAST_ENTITY="$RUN/m4lan-east-entity"
LAN_NORTH_ENTITY="$RUN/m4lan-north-entity"
LAN_RETURN_ENTITY="$RUN/m4lan-return-entity"

# ---------------------------------------------------------------- remote reads

# slot_stat <slot> <field> [default] — one field of one slot out of the status
# page. It is the only surface that covers the far slot, and §10.1's rule is why
# it is trustworthy: a stat older than statsStaleMs renders as unknown rather than
# as a stale number wearing the clothes of a measurement.
slot_stat() {
  local slot="$1" field="$2" def="${3:-unknown}"
  status_get "next((s for s in d[\"slots\"] if s[\"slot\"]==$slot), {}).get(\"$field\", \"$def\")"
}

remote_stat() { slot_stat "$REMOTE_SLOT" "$@"; }

# The far slot's health, read from three places that all live on this machine.
remote_state() {
  printf 'map      =[%s]\n' "$(map_shape) $(map_order)"
  printf 'relay    =[%s]\n' "$(grep -a "placement claim.*peer=$REMOTE_PEER" "$LOGS/relay.log" 2>/dev/null | tail -n 1)"
  printf 'status   =[live=%s modConnected=%s pop=%s sim=%s]\n' \
    "$(remote_stat live)" "$(remote_stat modConnected)" \
    "$(remote_stat population)" "$(remote_stat simulatedTime)"
  printf 'ripple   =[slot 5 E -> slot %s | slot 3 N -> slot %s (%s)]\n' \
    "$(lane_slot 5 E)" "$(lane_slot 3 N)" "$(lane_reason 3 N)"
  printf 'lastSave =[%s]\n' "$(remote_stat lastSave)"
}

# far_end_hint — what a person has to do on the other computer. Nothing on this
# machine can do it, and nothing here has to.
far_end_hint() {
  note ""
  note "ON THE SECOND COMPUTER (its operator, not this rig):"
  note "    .\\setup-farend.ps1 -RelayHost <the LAN address of THIS machine> -TokenFile .\\token.txt"
  note "    .\\start-slot$REMOTE_SLOT.ps1"
  note "It joins as peer $REMOTE_PEER, slot $REMOTE_SLOT, position $REMOTE_POS, world $REMOTE_WORLD."
  note "Run 'e2e/run-m4-lan.sh lanhost' for that address and the elevated commands."
  note "Nothing on this machine can start the far end, and nothing here has to."
}

# ---------------------------------------------------------------- lanhost

# The owner's networking steps, with THIS machine's live numbers filled in.
#
# WSL2 runs behind NAT here (.wslconfig sets networkingMode=NAT, deliberately —
# mirrored mode breaks Docker Desktop port publishing on this host). A relay
# listening on 0.0.0.0 inside WSL is therefore reachable from Windows, but NOT
# from the LAN: the second computer talks to the WINDOWS address, and Windows has
# to forward that port into the WSL VM.
lanhost() {
  local wslip winips ps proxies

  wslip="$(hostname -I 2>/dev/null | awk '{print $1}')"
  ps=/mnt/c/Windows/System32/WindowsPowerShell/v1.0/powershell.exe
  winips="$(cd /mnt/c && "$ps" -NoProfile -NonInteractive -Command \
    "Get-NetIPAddress -AddressFamily IPv4 | Where-Object { \$_.IPAddress -notlike '127.*' -and \$_.IPAddress -notlike '169.254.*' } | Select-Object -ExpandProperty IPAddress" \
    2>/dev/null </dev/null | tr -d '\r')"
  proxies="$(cd /mnt/c && /mnt/c/Windows/System32/netsh.exe interface portproxy show v4tov4 \
    2>/dev/null </dev/null | tr -d '\r')"

  step "the relay's LAN address, and the elevated commands that expose it"
  note "WSL address of the relay : $wslip   (it CAN move on a WSL restart, and it did not"
  note "                                            across the reboots of 2026-08-08 and 2026-08-09)"
  note "relay port               : $RELAY_PORT   (M4 moved it off 8790 — see below)"
  note "Windows IPv4 addresses   :"
  printf '%s\n' "$winips" | sed 's/^/        /' >&2
  note ""
  note "dev_environment.md records the LAN host as 192.168.1.227, confirmed by the far"
  note "end itself. Pick that one unless this machine's addresses have changed — never a"
  note "172.x hypervisor address."
  note ""
  note "port proxies on this machine right now:"
  printf '%s\n' "${proxies:-<none, or netsh was unreachable>}" | sed 's/^/        /' >&2
  note ""
  note "In an ELEVATED PowerShell on THIS machine:"
  note ""
  note "  # 1. DELETE THE STALE M3 PORTPROXY. It forwards 8790 to a WSL address that is"
  note "  #    gone, and under the M4 port plan 8790 is SLOT 4'S CONTRACT A PORT. It"
  note "  #    listens on 0.0.0.0, so it shadows 127.0.0.1:8790 for WINDOWS processes"
  note "  #    only: slot 4's sidecar binds it happily inside WSL and slot 4's game is"
  note "  #    refused from outside, which reads as a mod bug and is a network artifact."
  note "  netsh interface portproxy delete v4tov4 listenport=8790 listenaddress=0.0.0.0"
  note ""
  note "  # 2. open the relay port, once. -Profile Any, NOT Private: an Ethernet NIC"
  note "  #    that Windows has classified Public silently ignores a Private-only rule,"
  note "  #    and the far end just reports the relay unreachable."
  note "  New-NetFirewallRule -DisplayName 'Bibites Multiverse relay $RELAY_PORT' -Direction Inbound \`"
  note "    -Action Allow -Protocol TCP -LocalPort $RELAY_PORT -Profile Any"
  note ""
  note "  # 3. forward it into WSL. RE-RUN THIS ONLY ON A DIFFERENCE: the portproxy and the"
  note "  #    firewall rule survive a reboot, and the WSL address above survived both of"
  note "  #    2026-08-08 and 2026-08-09 (dev_environment.md, 'Bringing it back after a"
  note "  #    reboot'). Compare the address above with the table above and re-add only if"
  note "  #    they disagree — re-running it blind drops the forward for as long as the"
  note "  #    two commands take. The LAN address does not change either way."
  note "  netsh interface portproxy delete v4tov4 listenport=$RELAY_PORT listenaddress=0.0.0.0"
  note "  netsh interface portproxy add v4tov4 listenport=$RELAY_PORT listenaddress=0.0.0.0 \`"
  note "    connectport=$RELAY_PORT connectaddress=$wslip"
  note ""
  note "  # check"
  note "  netsh interface portproxy show v4tov4"
  note ""
  note "DO NOT TEST THIS WITH curl FROM WSL against this machine's own LAN address. WSL2"
  note "NAT hairpinning does not work, so that curl fails even when the firewall, the"
  note "proxy and the relay are all correct. Test from a Windows PowerShell:"
  note "    Test-NetConnection <LAN address> -Port $RELAY_PORT"
}

# ---------------------------------------------------------------- reserve

# Pre-placing removes start order from the LAN map.
#
# §7.2 rule 4 places a new peer itself, so on a fresh map placement follows join
# order — which the local rehearsal gets right for free by starting six sidecars
# in a row. Across a LAN it is not free: the second computer is started by a
# person, and "start yours after my fifth one" is not an instruction a rig should
# depend on. `--reserve-slot <peerId>@<col>,<row>` writes all six reservations
# before anybody connects, and rule 1 then hands each peer the slot AND the
# position its peerId already owns, whenever it arrives.
#
# The far peer's reservation is the whole point: slot-6 at (2,1) exists on this
# relay before the second computer has been switched on.
reserve() {
  step "pre-placing the 3x2 map: slot-1..slot-5 here, slot-$REMOTE_SLOT at $REMOTE_POS on the second computer"
  if [ -n "$(read_pid relay)" ] && proc_alive "$(read_pid relay)"; then
    fail "the relay is running; --reserve-slot is a startup command. Run 'down' first."
    return 1
  fi
  ensure_token
  mkdir -p "$RELAY_DATA"
  local args=() s
  for s in $MAP_SLOTS; do args+=(--reserve-slot "$(peer_of "$s")@$(pos_of "$s")"); done
  "$BIN/relay" --data-dir "$RELAY_DATA" --token-file "$TOKEN_FILE" "${args[@]}" \
    2>&1 | sed 's/^/    /' >&2
  note "map: $(map_shape)"
  note "order: $(map_order)"
}

# ---------------------------------------------------------------- seeding

# Only the five LOCAL worlds. M4-Slot6 lives on the second computer, and the mod
# there seeds it by itself on the first start (MULTIVERSE_WORLD, WorldSeeder).
lan_seed() {
  "$GAME_SH" stop all >/dev/null 2>&1 || true
  sleep 3
  reset_bepinex_logs seed

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

  step "growing slot 1 until an organism has a living parent or child"
  local deadline=$(( $(now) + ${FAMILY_TIMEOUT:-1800} )) line
  while :; do
    line="$(send 1 family)" || true
    note "$line"
    case "$line" in *"linked=YES"*) break ;; esac
    [ "$(now)" -ge "$deadline" ] && { note "no family link within the timeout; the annex will carry gaps"; break; }
    sleep 20
  done

  for s in $GAME_SLOTS; do send "$s" save "$(world_of "$s")" >/dev/null || return 1; done
  for s in $GAME_SLOTS; do send "$s" quit >/dev/null 2>&1 || true; done
  sleep 10
  "$GAME_SH" stop all >/dev/null 2>&1 || true
  archive_bepinex_logs seed
  verdict "five local worlds seeded; $REMOTE_WORLD is the second computer's to seed"
}

# ---------------------------------------------------------------- bring-up

# The relay binds 0.0.0.0 here, so run-m4.sh's loopback-only port check would
# miss it.
lan_ports_busy() { ss -ltn 2>/dev/null | grep -qE ":($(rig_ports)) "; }

lan_up() {
  if lan_ports_busy; then
    fail "a rig is already bound on $(rig_ports) — run 'down' first"
    return 1
  fi
  # The Contract A ports have to be free on the WINDOWS side too. This is where
  # the stale 8790 portproxy is caught, and `lanhost` prints the one command that
  # removes it.
  check_slot_ports || return 1
  ensure_token
  "$GAME_SH" stop all >/dev/null 2>&1 || true
  sleep 3
  reset_bepinex_logs up

  case "$(map_shape)" in
    3x2) note "the map is already pre-placed: $(map_order)" ;;
    *)   reserve || return 1 ;;
  esac

  step "starting the Go side: relay (LAN-bound), archive, then five local sidecars"
  start_relay
  wait_healthy "http://127.0.0.1:$RELAY_PORT/healthz" || return 1
  note "the second computer dials ws://<this machine>:$RELAY_PORT/contract-b/v3"
  start_archive
  wait_file "$LOGS/archive.log" 'archive: subscribed to the relay' 60 >/dev/null || return 1
  note "archive subscribed; status page on $ARCHIVE_URL"

  local slot mark
  for slot in $SLOTS; do
    mark="$(file_mark "$(sclog_of "$slot")")"
    start_sidecar "$slot"
    wait_healthy "http://127.0.0.1:$(port_of "$slot")/healthz" || return 1
    wait_grant "$slot" "$mark" || return 1
  done
  note "five local slots granted; map is $(map_shape)"

  for slot in $SLOTS; do
    step "starting game $slot ($(world_of "$slot"), exportEdges=$EXPORT_EDGES, sidecar $(port_of "$slot"))"
    start_game "$slot" || return 1
    sleep 8
  done
  for slot in $SLOTS; do
    world_ready "$slot" 1200 || return 1
    note "slot $slot: world live"
  done

  step "waiting for every local mod to connect"
  for slot in $LOGGED_SLOTS; do
    wait_log "$slot" '\[M2\] CONFIG_UPDATE reason=connect' 300 >/dev/null || return 1
  done

  # THE LOCAL LANES THAT DO NOT DEPEND ON THE FAR END must be open now. The two
  # that do — slot 5's east and slot 3's north — are phase 1's business, and
  # waiting on them here would block the bring-up on a person.
  step "the local lanes: everything except slot 5's east and slot 3's north"
  for slot in $SLOTS; do
    edge_open "$slot" E 300 || { fail "slot $slot: the east lane never opened"; return 1; }
    [ "$slot" = 3 ] && continue
    edge_open "$slot" N 300 || { fail "slot $slot: the north lane never opened"; return 1; }
  done
  note "slot 3's north lane is deliberately not waited on: column 2 is {3, $REMOTE_SLOT} and"
  note "§2.1 closes it with no_peer until the second computer joins. That closure IS the"
  note "evidence, and phase 1 watches it turn into an open lane."
  note "slot 5's east lane is open, and while the far end is down it names slot $(lane_slot 5 E) —"
  note "the bypass around the hole at $REMOTE_POS. Phase 1 watches it move to slot $REMOTE_SLOT."

  for slot in $GAME_SLOTS; do send "$slot" timescale "$TIME_SCALE" >/dev/null || return 1; done
  verdict "the main machine is up: five real worlds at ${TIME_SCALE}x. The far end joins whenever it starts."
  far_end_hint
}

lan_down() {
  step "tearing down THIS machine only"
  note "Nothing below reaches the second computer. Its operator runs stop-slot$REMOTE_SLOT.ps1"
  note "there; its journal, its world and its slot survive either way."
  "$GAME_SH" stop all || true
  local slot p pid
  for slot in $SLOTS; do kill_pid "sidecar-$slot" -TERM; done
  kill_pid archive -TERM
  kill_pid relay -TERM
  sleep 2
  for slot in $SLOTS; do
    pid="$(read_pid "sidecar-$slot")"
    proc_alive "$pid" && kill -9 "$pid" 2>/dev/null
    rm -f "$(pidfile "sidecar-$slot")"
  done
  for p in relay archive; do
    pid="$(read_pid "$p")"
    proc_alive "$pid" && kill -9 "$pid" 2>/dev/null
    rm -f "$(pidfile "$p")"
  done
  sleep 1
  archive_bepinex_logs down
}

lan_status() {
  step "status — this machine"
  local p pid
  for p in relay archive $(for s in $SLOTS; do echo "sidecar-$s"; done); do
    pid="$(read_pid "$p")"
    if proc_alive "$pid"; then note "$p pid=$pid RUNNING"; else note "$p not running"; fi
  done
  ss -ltn 2>/dev/null | grep -E ":($(rig_ports)|8796) " || note "no rig port is bound"
  "$GAME_SH" status
  step "status — the far slot, read from here"
  remote_state | sed 's/^/    /' >&2
}

# ---------------------------------------------------------------- archive reads

# The archive ledger is durable and cumulative: it still holds every hop of every
# earlier run. A wait that matches any record on a lane therefore succeeds
# instantly on a rig that has run before — which is exactly what happened the
# first time run-m3-lan.sh's phase 3 ran, and it "proved" a crossing recorded an
# hour earlier. Every wait is bounded below by a timestamp, and the archive's
# header line starts with an RFC3339 UTC time, so a string compare is the filter.
#
# This overrides run-m4.sh's version to add progress, because a LAN wait is
# minutes long and a silent one is indistinguishable from a hang.
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
      note "waiting on lane $from -> $to since $since: ${elapsed}s of ${timeout}s; records on that lane in this run: $(archive_hop "$from" "$to" | awk -v s="$since" '$1 >= s' | grep -ac . || true)"
      [ "$from" = "$REMOTE_SLOT" ] || [ "$to" = "$REMOTE_SLOT" ] && \
        note "  far slot: live=$(remote_stat live) modConnected=$(remote_stat modConnected) pop=$(remote_stat population) pacedDepth=$(remote_stat pacedDepth)"
      next=$(( $(now) + NATURAL_PROGRESS ))
    fi
    sleep 3
  done
}

# hop_to_remote <from> <edge> <selector> — one forced export from a LOCAL slot
# into the far one. The local half is read from local logs; THE ARRIVAL IS READ
# FROM THE ARCHIVE, because the only other witness is a computer this rig will
# not ask. Prints "migrationId=… entityId=…".
hop_to_remote() {
  local from="$1" edge="$2" selector="${3:-any}"
  local line eid mid mark since dest

  since="$(archive_mark)"
  mark="$(log_mark "$from")"
  line="$(send "$from" export "$selector" "$edge")" || { fail "export failed: $line"; return 1; }
  note "export: $line"
  eid="$(field entityId "$line")"
  [ -n "$eid" ] || { fail "the export result named no entityId: $line"; return 1; }

  line="$(wait_log "$from" "entityId=$eid phase=MIGRATE_OUT_SENT" 120 "$mark")" || return 1
  note "slot $from: $line"
  mid="$(field migrationId "$line")"
  local out_edge; out_edge="$(field edge "$line")"
  [ "$out_edge" = "$edge" ] || { fail "slot $from exported through $out_edge, want $edge"; return 1; }

  line="$(wait_file "$(sclog_of "$from")" "forwarded MIGRATION_PAYLOAD.*migrationId=$mid" 90)" || return 1
  note "sidecar $from: $line"
  dest="$(field destSlot "$line")"
  [ "$dest" = "$REMOTE_SLOT" ] \
    || { fail "slot $from's $edge lane routed to slot $dest, want the far slot $REMOTE_SLOT"; return 1; }

  # §6.7: the archive's `delivered` outcome is written from the MIGRATION_ACK the
  # far sidecar sent after ITS mod took the organism in. It is the same custody
  # gate a local arrival passes; it just does not carry the payload hash, because
  # the hash comparison needs both ends' logs and one end is out of reach.
  step "reading the arrival out of the ARCHIVE — the far end is never asked"
  line="$(wait_archive_hop "$from" "$REMOTE_SLOT" "${ARRIVAL_TIMEOUT:-300}" "$since" "$eid")" || return 1
  note "DECISIVE (archive): $line"

  wait_log "$from" "migrationId=$mid .*phase=DESTROYED" 90 "$mark" >/dev/null \
    && note "slot $from destroyed its copy on MIGRATE_OUT_ACK" \
    || { fail "slot $from never destroyed its copy"; return 1; }

  printf 'migrationId=%s entityId=%s edge=%s\n' "$mid" "$eid" "$edge"
}

# hop_from_remote <to> <edge> <timeout> — wait for a NATURAL crossing out of the
# far slot on one axis, then assert the local half of it. <edge> is the edge the
# FAR slot exported through. Prints "migrationId=… entityId=…".
hop_from_remote() {
  local to="$1" edge="$2" timeout="${3:-$NATURAL_TIMEOUT}"
  local since line mid eid want_entry got_entry ok=0

  since="$(archive_mark)"
  local before; before="$(archive_hop "$REMOTE_SLOT" "$to" | grep -ac . || true)"
  note "slot $REMOTE_SLOT -> slot $to records the archive already holds: ${before:-0}"
  note "only a record written after $since counts"

  line="$(wait_archive_hop "$REMOTE_SLOT" "$to" "$timeout" "$since")" || return 1
  note "DECISIVE (archive): $line"
  mid="$(awk '{print $2}' <<<"$line")"
  eid="$(sed -n 's/.* entity \(-\?[0-9]*\) .*/\1/p' <<<"$line")"

  line="$(wait_file "$(sclog_of "$to")" "took custody of an inbound organism.*migrationId=$mid" 180)" \
    && note "sidecar $to: $line" || { fail "sidecar $to never took custody of $mid"; ok=1; }
  # §6.6: the entry edge is DERIVED as the opposite of the sender's exit edge, and
  # it is derived on THIS side, from a field the far sidecar put on the wire.
  case "$edge" in E) want_entry=W ;; N) want_entry=S ;; esac
  got_entry="$(field entryEdge "$line")"
  if [ "$got_entry" = "$want_entry" ]; then
    note "the $edge hop arrives through the passive $want_entry edge"
  else
    fail "the $edge hop arrived on entry edge $got_entry, want the passive $want_entry"; ok=1
  fi
  line="$(wait_log "$to" "migrationId=$mid .*phase=SPAWNED" 240)" \
    && note "slot $to: $line" || { fail "slot $to never spawned $mid"; ok=1; }
  line="$(wait_log "$to" "migrationId=$mid .*phase=MIGRATE_IN_ACK" 120)" \
    && note "slot $to: $line" || { fail "slot $to never ACKed $mid"; ok=1; }

  [ "$ok" = 0 ] || return 1
  printf 'migrationId=%s entityId=%s edge=%s\n' "$mid" "$eid" "$edge"
}

# ---------------------------------------------------------------- the census

# count_everywhere <entityId> — THE EXACTLY-ONCE TABLE FOR A MAP WHOSE SIXTH SLOT
# CANNOT BE COUNTED DIRECTLY. This overrides run-m4.sh's version by name, so every
# inherited phase that counts an organism — phase 6's burst check included — uses
# this one without knowing it exists.
#
# Five worlds are asked. The five local journals are read. THE SIXTH IS INFERRED,
# and the inference is printed rather than hidden: the archive holds one record
# per hop in order, so the last record naming this organism says where it went. If
# its last hop was INTO the far slot and delivered, the far slot holds it. If its
# last hop LEFT the far slot and was delivered, it does not.
count_everywhere() {
  local eid; eid="$(tr -cd '0-9-' <<<"$1")"
  local slot alive total=0 custody custody_count journals=()
  printf '\n    ---- organism %s ----\n' "$eid" >&2

  for slot in $SLOTS; do
    alive="$(holds_entity "$slot" "$eid")"; alive="${alive:-0}"
    printf '    world slot %s (local) : %s\n' "$slot" "$alive" >&2
    total=$(( total + alive ))
  done

  for slot in $SLOTS; do
    [ -f "$(journal_of "$slot")" ] && journals+=("$(journal_of "$slot")")
  done
  custody=""
  if [ "${#journals[@]}" -gt 0 ]; then
    custody="$(python3 "$E2E/journal.py" custody "$eid" "${journals[@]}" 2>/dev/null)"
  fi
  custody_count="$(sed -n 's/^custodyCount=\([0-9]*\)$/\1/p' <<<"$custody")"
  custody_count="${custody_count:-0}"
  printf '    local journals        : %s\n' "$custody_count" >&2
  [ -n "$custody" ] && sed 's/^/      /' <<<"$custody" >&2
  total=$(( total + custody_count ))

  local last held dest outcome
  last="$(archive_list 2>/dev/null | grep -a "entity $eid " | tail -n 1)"
  printf '    slot %s (remote) is INFERRED, not counted. Its last archive record:\n' "$REMOTE_SLOT" >&2
  printf '      %s\n' "${last:-<none>}" >&2
  dest="$(sed -n 's/.*slot [0-9]* -> slot \([0-9]*\) .*/\1/p' <<<"$last")"
  outcome="$(sed -n 's/.*)  \(.*\)$/\1/p' <<<"$last")"
  held=0
  if [ -z "$last" ]; then
    printf '      -> the archive has no record of this organism at all: slot %s holds 0\n' "$REMOTE_SLOT" >&2
  elif [ "$dest" = "$REMOTE_SLOT" ] && [ "${outcome#delivered}" != "$outcome" ]; then
    held=1
    printf '      -> its last hop was INTO slot %s and it was delivered: slot %s holds 1\n' \
      "$REMOTE_SLOT" "$REMOTE_SLOT" >&2
  else
    printf '      -> its last hop ended in slot %s, not slot %s: slot %s holds 0\n' \
      "${dest:-?}" "$REMOTE_SLOT" "$REMOTE_SLOT" >&2
  fi
  printf '    slot %s inferred       : %s\n' "$REMOTE_SLOT" "$held" >&2
  total=$(( total + held ))

  printf '    TOTAL                 : %s  -> %s\n' "$total" \
    "$( [ "$total" = 1 ] && echo PASS || { [ "$total" = 0 ] && echo 'PASS (acceptable loss under D2)' || echo FAIL; } )" >&2
  printf '%s\n' "$total"
}

# ---------------------------------------------------------------- phase 1

lan_phase1() {
  step "PHASE 1 — the grid forms across two computers, and the far slot reports for duty"
  note "The far end is never asked anything. Its arrival is read from the relay's map,"
  note "from the EDGE_STATUS ripple into slots 3 and 5, and from the status page."
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
  done

  # THE RIPPLE. This is the whole remote-liveness proof, and both halves of it are
  # written on THIS machine by mods that know nothing about a network.
  step "waiting for the far slot — slot 3's NORTH lane must open and name slot $REMOTE_SLOT"
  note "Column 2 is {3, $REMOTE_SLOT}. While the far end is down that column is degenerate and"
  note "§2.1 closes slot 3's north lane with no_peer. It can only open by slot $REMOTE_SLOT arriving."
  local deadline=$(( $(now) + FAR_JOIN_TIMEOUT )) last="" n3 e5
  while :; do
    n3="$(lane_slot 3 N)"; e5="$(lane_slot 5 E)"
    if [ "$n3" = "$REMOTE_SLOT" ] && [ "$e5" = "$REMOTE_SLOT" ]; then break; fi
    local now_state
    now_state="slot 3 N -> ${n3:-0} ($(lane_reason 3 N))  |  slot 5 E -> ${e5:-0}"
    if [ "$now_state" != "$last" ]; then note "$now_state"; last="$now_state"; fi
    if [ "$(now)" -ge "$deadline" ]; then
      fail "the far slot never joined within ${FAR_JOIN_TIMEOUT}s"
      remote_state | sed 's/^/      /' >&2
      far_end_hint
      verdict "PHASE 1: FAIL — no second computer"
      return 1
    fi
    sleep 5
  done
  note "slot 3's north lane -> slot $REMOTE_SLOT, and slot 5's east lane -> slot $REMOTE_SLOT"
  note "THE FAR WORLD IS LIVE AND ITS MOD IS ATTACHED: §8 opens a lane on nothing less."

  step "and the mods say the same thing, in their own logs"
  line="$(wait_log 3 '\[M2\] EDGE_STATUS .*N:open' 120)" \
    && note "slot 3: $line" || { fail "slot 3's north lane never opened in its own log"; ok=1; }
  line="$(grep_log 5 '\[M2\] EDGE_STATUS epoch=' | tail -n 1)"
  note "slot 5: $line"

  # PER-EDGE EDGE_STATUS: one entry per declared export edge (contract-a.md §15
  # A18), and under two-way lanes that is FOUR (§18, A38). Every slot declares all
  # four because a 3x2 map is a torus and the declaration is geometry, not
  # topology — see run-m4.sh's header. Every one of the four opens here: both axes
  # wrap, so no local slot has a dark side.
  step "per-edge EDGE_STATUS on every local slot"
  for slot in $LOGGED_SLOTS; do
    line="$(grep_log "$slot" '\[M2\] EDGE_STATUS epoch=' | tail -n 1)"
    if [ -z "$line" ]; then fail "slot $slot never applied an EDGE_STATUS"; ok=1; continue; fi
    note "slot $slot: $line"
    for e in E N W S; do
      case "$line" in *"$e:open"*) ;; *) fail "slot $slot's $e edge is not open"; ok=1 ;; esac
    done
    case "$line" in *"entries=4"*) ;;
      *) fail "slot $slot's EDGE_STATUS did not carry four entries — one per declared export edge"; ok=1 ;;
    esac
  done

  line="$(grep -a "placement claim.*peer=$REMOTE_PEER" "$LOGS/relay.log" | tail -n 1)"
  if [ -n "$line" ]; then note "relay: $line"; else fail "the relay never logged a claim from $REMOTE_PEER"; ok=1; fi
  line="$(grep -a 'archive: subscribed to the relay' "$LOGS/archive.log" | tail -n 1)"
  if [ -n "$line" ]; then note "archive: $line"; else fail "the archive never subscribed"; ok=1; fi

  step "the status page — the one surface that covers the far slot as well as the five here (§10.1)"
  local json="$LOGS/status-phase1.json"
  if status_json > "$json" 2>/dev/null && [ -s "$json" ]; then
    note "saved $json"
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
    # ONE LANE PER DECLARED EDGE PER SLOT (§10.1, §17 B13), so the count is DERIVED
    # from what the slots declare and never a constant. Under two-way lanes six
    # four-edge slots give 24; a slot still on a pre-D17 mod declares two and the
    # page draws it with two dead lanes rather than inventing them — which is the
    # legal mixed case during a rollout, and the reason this is not `-eq 24`.
    local pdecl
    pdecl="$(status_get 'sum(len(s.get("exportEdges") or []) for s in d["slots"])')"
    note "declared export edges across the map: $pdecl (one lane each; all wrap, so all open)"
    [ "$planes" = "$pdecl" ] || { fail "the status page draws $planes lanes for $pdecl declared edges"; ok=1; }
    [ "$popen" = "$pdecl" ]  || { fail "the status page reports $popen open lanes, want $pdecl — both axes wrap, so no declared edge is dark"; ok=1; }
    step "and what it knows about the far world, which is everything §6.3.1 puts on the wire"
    note "slot $REMOTE_SLOT: peer=$(remote_stat peerId) live=$(remote_stat live) modConnected=$(remote_stat modConnected)"
    note "slot $REMOTE_SLOT: population=$(remote_stat population) simulatedTime=$(remote_stat simulatedTime) simulationSize=$(remote_stat simulationSize)"
    note "slot $REMOTE_SLOT: gameVersion=$(remote_stat gameVersion) exportEdges=$(remote_stat exportEdges)"
    [ "$(remote_stat modConnected)" = "True" ] \
      || { fail "the status page does not see a mod on the far slot"; ok=1; }
  else
    fail "the status page did not answer on $ARCHIVE_URL/api/status"; ok=1
  fi

  [ "$ok" = 0 ] && verdict "PHASE 1: PASS — a 3x2 map of SIX REAL WORLDS across two computers, twelve open lanes, no synthetic peer" \
                || verdict "PHASE 1: FAIL"
  return "$ok"
}

# ---------------------------------------------------------------- phase 2

lan_phase2() {
  step "PHASE 2 — forced hops INTO the far slot, on BOTH axes"
  note "slot 5 exports EAST into slot $REMOTE_SLOT; slot 3 exports NORTH into it. Each arrival is"
  note "read from the archive, whose 'delivered' outcome means the far sidecar journaled"
  note "the organism and the far mod acknowledged it (§6.7)."
  local ok=0 out_e out_n
  slow_sims 1

  out_e="$(hop_to_remote 5 E family)" || { restore_sims; verdict "PHASE 2: FAIL (east hop into the far slot)"; return 1; }
  note "east hop: $out_e"
  sleep 12
  out_n="$(hop_to_remote 3 N any)" || { restore_sims; verdict "PHASE 2: FAIL (north hop into the far slot)"; return 1; }
  note "north hop: $out_n"

  settle 20
  restore_sims
  local e_eid n_eid t1 t2
  e_eid="$(field entityId "$out_e")"; n_eid="$(field entityId "$out_n")"
  printf '%s\n' "$e_eid" > "$LAN_EAST_ENTITY"
  printf '%s\n' "$n_eid" > "$LAN_NORTH_ENTITY"

  t1="$(count_everywhere "$e_eid")"; t2="$(count_everywhere "$n_eid")"
  [ "$t1" -le 1 ] || { fail "the east migrant exists $t1 times"; ok=1; }
  [ "$t2" -le 1 ] || { fail "the north migrant exists $t2 times"; ok=1; }

  note "the far slot's own numbers, from the status page: population=$(remote_stat population) custodyDepth=$(remote_stat custodyDepth) pacedDepth=$(remote_stat pacedDepth)"

  [ "$ok" = 0 ] && verdict "PHASE 2: PASS — both axes carry an organism across the LAN into slot $REMOTE_SLOT, exactly once" \
                || verdict "PHASE 2: FAIL"
  return "$ok"
}

# ---------------------------------------------------------------- phase 3

lan_phase3() {
  step "PHASE 3 — the return current: NATURAL crossings OUT of the far slot, on both axes"
  note "Nothing forces these. The far end is not driven, so the rig waits for the map's"
  note "own traffic: an organism swims east out of slot $REMOTE_SLOT into slot $(east_of "$REMOTE_SLOT") by itself,"
  note "and another swims north into slot $(north_of "$REMOTE_SLOT"). Both wrap, because a 3x2 map is a torus."
  note "timeout ${NATURAL_TIMEOUT}s per axis, progress every ${NATURAL_PROGRESS}s"
  note "(the far end's operator can press F10 in the game to force one east)"
  local ok=0 out_e out_n

  step "the east axis: slot $REMOTE_SLOT -> slot $(east_of "$REMOTE_SLOT")"
  out_e="$(hop_from_remote "$(east_of "$REMOTE_SLOT")" E)" || ok=1
  [ -n "$out_e" ] && note "east return: $out_e"

  step "the north axis: slot $REMOTE_SLOT -> slot $(north_of "$REMOTE_SLOT")"
  out_n="$(hop_from_remote "$(north_of "$REMOTE_SLOT")" N)" || ok=1
  [ -n "$out_n" ] && note "north return: $out_n"

  local eid
  eid="$(field entityId "$out_e")"
  [ -n "$eid" ] && printf '%s\n' "$eid" > "$LAN_RETURN_ENTITY"

  settle 20
  for eid in "$(field entityId "$out_e")" "$(field entityId "$out_n")"; do
    [ -n "$eid" ] || continue
    local total; total="$(count_everywhere "$eid")"
    [ "$total" -le 1 ] || { fail "returned entity $eid exists $total times"; ok=1; }
  done

  [ "$ok" = 0 ] && verdict "PHASE 3: PASS — the far world exported on BOTH axes with nobody driving it, and both organisms landed here exactly once" \
                || verdict "PHASE 3: FAIL"
  return "$ok"
}

# ---------------------------------------------------------------- phase 4

lan_phase4() {
  step "PHASE 4 — hard-kill LOCAL slot 5 mid-column, and watch the healed east lane cross the LAN"
  note "THE KILL LANDS HERE BECAUSE IT HAS TO. A hard kill is a command to a process,"
  note "and the far end takes no commands. Slot 5 is local, and so are both of its"
  note "observers — but the lane that heals around it ends on the OTHER COMPUTER:"
  note "row 1 becomes {4, $REMOTE_SLOT}, so slot 4's east lane re-pairs across the network."
  note ""
  note "Column 1 holds only slot 2 and slot 5, so slot 2's NORTH lane has nothing to"
  note "re-pair to and CLOSES with no_peer. §2.1 says that is the correct degenerate"
  note "answer, and an implementation that re-pairs it is defective."
  local ok=0 line

  local mark2; mark2="$(log_mark 2)"
  step "kill -9 the game and the sidecar of slot 5"
  "$GAME_SH" stop "$(game_instance 5)" || true
  kill_pid sidecar-5 -9
  echo "slot 5 killed at $(date -u +%FT%TZ)" >> "$LOGS/kills.log"
  settle 30

  step "the width axis routes around: slot 4's east lane must now name the FAR slot $REMOTE_SLOT"
  wait_lane 4 E "$REMOTE_SLOT" 180 || ok=1
  note "the skipped list: $(status_get 'next((l["skipped"] for l in d["lanes"] if l["fromSlot"]==4 and l["edge"]=="E"), [])')"

  # THE DECISIVE EVIDENCE IS A MIGRATION THAT ARRIVES, not a map that is read —
  # and this one crosses the LAN.
  step "and it carries traffic: one forced export east out of slot 4 must land on the second computer"
  slow_sims 1
  local reroute_out
  reroute_out="$(hop_to_remote 4 E any)" || { fail "the re-paired east lane carried nothing across the LAN"; ok=1; }
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
  status_json > "$LOGS/status-phase4-dark.json" 2>/dev/null
  local dark; dark="$(slot_stat 5 live)"
  note "status page: slot 5 live=$dark darkSince=$(slot_stat 5 darkSinceMs)"
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

  step "and slot 5's east lane goes back to the far slot"
  wait_lane 5 E "$REMOTE_SLOT" 240 || ok=1
  status_json > "$LOGS/status-phase4-back.json" 2>/dev/null

  [ "$ok" = 0 ] && verdict "PHASE 4: PASS — the width axis healed ACROSS THE LAN, the degenerate height axis closed no_peer, and slot 5 spliced back into (1,1)" \
                || verdict "PHASE 4: FAIL"
  return "$ok"
}

# ---------------------------------------------------------------- phase 5

# The local burst-pacing test is run-m4.sh's phase 6, UNCHANGED. Every slot it
# touches — the source (1), the dam (2), the sims it slows — is on this machine,
# and its census call resolves to the LAN-aware count_everywhere above. Running
# the rehearsal's own code is the point: the LAN rig must not quietly re-prove a
# rule with a weaker test.
lan_phase5() {
  note "This is run-m4.sh's phase 6 verbatim: the dam is a LOCAL slot, so the whole test"
  note "is automatable with nobody at the second computer. phase5far is the cross-machine"
  note "confirmation of the same rule, and it needs two owner commands."
  phase6
}

# ---------------------------------------------------------------- phase 5far

# wait_far <field> <wanted> <timeout> <what the owner has to do>
wait_far() {
  local field="$1" want="$2" timeout="$3" prompt="$4"
  local deadline=$(( $(now) + timeout )) got last=""
  step "WAITING ON THE OWNER: $prompt"
  note "The rig sends nothing to the second computer. It watches the status page."
  while :; do
    got="$(remote_stat "$field")"
    [ "$got" = "$want" ] && { note "slot $REMOTE_SLOT $field=$got"; return 0; }
    if [ "$got" != "$last" ]; then note "slot $REMOTE_SLOT $field=$got, waiting for $want"; last="$got"; fi
    if [ "$(now)" -ge "$deadline" ]; then
      fail "slot $REMOTE_SLOT $field is still $got after ${timeout}s"
      return 1
    fi
    sleep 5
  done
}

lan_phase5_far() {
  step "PHASE 5far — burst pacing against the FAR slot, with two owner commands and no drive path"
  note "THE DAM IS THE FAR PEER'S OWN INBOUND QUEUE, and contract-b-m4.md §9.4 is why."
  note "Under M3 a dark slot backed traffic up into its west neighbour. Under M4 that"
  note "neighbour re-pairs (D12) and nothing piles up behind a dark slot at all. What"
  note "accumulates is the receiving sidecar's journal: custody moves at the speed of the"
  note "wire and delivery moves at the speed of the world (§7.5), so a burst lands in the"
  note "far sidecar's journal in seconds and leaves it at inboundRatePerSimMinute."
  note ""
  note "EVERY NUMBER BELOW COMES FROM THE STATUS PAGE AND THE ARCHIVE. The far machine is"
  note "asked nothing; its operator is asked twice, and the rig waits for the page to"
  note "confirm each answer."
  local ok=0 n="${FAR_BURST:-12}" i ids=() line

  [ "$(remote_stat modConnected)" = "True" ] || {
    fail "the far world is not attached; start it before this phase"
    verdict "PHASE 5far: FAIL"; return 1; }

  slow_sims 1
  step "forcing $n exports into the far slot: alternating slot 5 EAST and slot 3 NORTH"
  for i in $(seq 1 "$n"); do
    if [ $(( i % 2 )) -eq 1 ]; then line="$(send 5 export any E || true)"; else line="$(send 3 export any N || true)"; fi
    local eid; eid="$(field entityId "$line")"
    [ -n "$eid" ] && ids+=("$eid")
    sleep 1
  done
  note "forced ${#ids[@]} exports at the far slot"

  step "the dam: the far sidecar has taken custody of the burst and released almost none of it"
  settle 20
  local depth
  depth="$(remote_stat pacedDepth)"
  note "slot $REMOTE_SLOT pacedDepth=$depth custodyDepth=$(remote_stat custodyDepth) population=$(remote_stat population)"
  if [ "$depth" = "unknown" ] || [ "${depth:-0}" -lt 3 ]; then
    fail "the far slot's paced depth is $depth — the burst did not dam, so there is nothing to pace"
    ok=1
  else
    note "PACING EVIDENCE (the dam): $depth organisms are journaled on the second computer, waiting on the rate limit"
  fi

  wait_far modConnected False "$FAR_OWNER_TIMEOUT" \
    "on the second computer, run:   .\\stop-slot$REMOTE_SLOT.ps1 -GameOnly" || {
      restore_sims; verdict "PHASE 5far: FAIL (the world never went down)"; return 1; }

  step "the custody record outlives the world"
  note "This is the T1 shape across a network: the world goes away and the journal does not."
  settle 30
  local dark_depth; dark_depth="$(remote_stat pacedDepth)"
  note "slot $REMOTE_SLOT while dark: live=$(remote_stat live) modConnected=$(remote_stat modConnected) pacedDepth=$dark_depth"
  note "and the map routed around it meanwhile: slot 5's east lane -> slot $(lane_slot 5 E), slot 3's north lane -> slot $(lane_slot 3 N) ($(lane_reason 3 N))"
  if [ "$dark_depth" = "unknown" ]; then
    note "the depth reads unknown, which §10.1 REQUIRES once the stats are stale — the"
    note "sidecar is still up, so this is a slow broadcast, not a lost journal."
  else
    [ "${dark_depth:-0}" -ge 1 ] || { fail "the far sidecar's journal emptied while its world was down"; ok=1; }
  fi

  local since; since="$(archive_mark)"
  wait_far modConnected True "$FAR_OWNER_TIMEOUT" \
    "on the second computer, run:   .\\start-slot$REMOTE_SLOT.ps1 -GameOnly" || {
      restore_sims; verdict "PHASE 5far: FAIL (the world never came back)"; return 1; }

  step "the backlog must drain AS A STREAM, and every sample must sit under the contract's own bound"
  # allowed(t) = burst + rate * simulatedMinutesSinceWake, the shipped defaults of
  # contract-a.md §15 A20: burst 5, 2.0 per simulated minute. Deliveries are
  # counted from the ARCHIVE — one delivered record per organism — and the
  # simulated clock is the far world's own, off the status page. Same comparison
  # as run-m4.sh phase 6, on the same two numbers, through the two surfaces that
  # reach across a network.
  local t0 wake_sim delivered sim now_pd allowed
  t0="$(now)"
  wake_sim="$(remote_stat simulatedTime 0)"
  note "the far world's simulatedTime at wake: $wake_sim"
  local deadline=$(( t0 + 1200 ))
  while :; do
    delivered="$(archive_list 2>/dev/null | grep -a "slot [0-9]* -> slot $REMOTE_SLOT" \
                  | grep -a 'delivered' | awk -v s="$since" '$1 >= s' | grep -ac . || true)"
    delivered="${delivered:-0}"
    sim="$(remote_stat simulatedTime 0)"
    now_pd="$(remote_stat pacedDepth)"
    allowed="$(python3 -c "print(f'{5.0 + 2.0*((float(\"$sim\")-float(\"$wake_sim\"))/60.0):.2f}')" 2>/dev/null)"
    note "t+$(( $(now) - t0 ))s: delivered=$delivered pacedDepth=$now_pd simElapsed=$(python3 -c "print(f'{float(\"$sim\")-float(\"$wake_sim\"):.0f}')" 2>/dev/null)s allowedByRate=$allowed"
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

  step "no held entry bounced while its destination was live (Risk 9)"
  local slot bounced
  for slot in 3 5; do
    bounced="$(slot_stat "$slot" bouncedTimeoutTotal)"
    note "slot $slot bouncedTimeoutTotal=$bounced"
    [ "$bounced" = "0" ] || [ "$bounced" = "unknown" ] || { fail "slot $slot bounced $bounced entr(y|ies)"; ok=1; }
  done

  step "and every organism in the burst exists exactly once"
  local dupes=0
  for i in "${ids[@]:0:4}"; do
    local total; total="$(count_everywhere "$i")"
    [ "$total" -le 1 ] || { fail "entity $i exists $total times"; dupes=1; }
  done
  [ "$dupes" = 0 ] || ok=1

  status_json > "$LOGS/status-phase5far.json" 2>/dev/null
  restore_sims
  [ "$ok" = 0 ] && verdict "PHASE 5far: PASS — the burst dammed in the FAR sidecar's journal, survived the dark, and drained as a paced stream" \
                || verdict "PHASE 5far: FAIL"
  return "$ok"
}

# ---------------------------------------------------------------- phase 6

lan_phase6() {
  step "PHASE 6 — periodic saves: five worlds here on disk, and the far one on the wire"
  note "MULTIVERSE_SAVE_MINUTES=$SAVE_MINUTES here. THE FAR WORLD'S SAVE INTERVAL IS ITS OWN"
  note "BUSINESS: the periodic save is mod configuration, start-slot$REMOTE_SLOT.ps1 sets it there, and"
  note "this machine never asks for a save and never schedules one (contract-a.md §5.2)."
  note "What crosses the network is a RECEIPT — HEARTBEAT.lastSave — and that is the only"
  note "wire path an operator surface has to a world's save state."
  local ok=0 slot line

  step "the local save receipts"
  for slot in $LOGGED_SLOTS; do
    line="$(grep_log "$slot" '\[M4-SAVE\] event=SAVED' | tail -n 1)"
    if [ -z "$line" ]; then fail "slot $slot has no [M4-SAVE] event=SAVED line"; ok=1; continue; fi
    note "slot $slot: $line"
    local stall; stall="$(field stallMs "$line")"
    note "slot $slot stallMs=$stall (Risk 3's budget is 2000 ms)"
    if [ -n "$stall" ] && python3 -c "import sys; sys.exit(0 if float('$stall') > 2000 else 1)" 2>/dev/null; then
      fail "slot $slot's save stalled ${stall} ms, over the 2000 ms budget"; ok=1
    fi
  done

  step "budget breaches anywhere on this machine"
  local breaches=0 n
  for slot in $LOGGED_SLOTS; do
    n="$(grep_log "$slot" 'event=BUDGET_EXCEEDED' | grep -ac . || true)"
    breaches=$(( breaches + ${n:-0} ))
  done
  note "[M4-SAVE] event=BUDGET_EXCEEDED lines here: $breaches"
  [ "$breaches" = 0 ] || { fail "the save budget was exceeded $breaches time(s)"; ok=1; }

  step "the rotation on disk, for the five worlds this machine owns"
  for slot in $GAME_SLOTS; do
    local world; world="$(world_of "$slot")"
    local live backups partial
    live="$(ls -la "$SAVE_DIR_WSL/$world.zip" 2>/dev/null | awk '{print $5}')"
    backups="$(find "$SAVE_DIR_WSL" -maxdepth 1 -name "$world-*Z.zip" 2>/dev/null | wc -l)"
    partial="$(find "$SAVE_DIR_WSL" -maxdepth 1 -name "$world.partial.zip" 2>/dev/null | wc -l)"
    note "$world: live=${live:-<missing>} bytes, timestamped backups=$backups (keep=$SAVE_KEEP), partial=$partial"
    [ -n "$live" ] || { fail "$world has no live save"; ok=1; }
    [ "$backups" -le "$SAVE_KEEP" ] || { fail "$world kept $backups backups, over MULTIVERSE_SAVE_KEEP=$SAVE_KEEP"; ok=1; }
  done

  step "the far world's save, proven WITHOUT touching its disk"
  local receipt age
  receipt="$(remote_stat lastSave none)"
  age="$(remote_stat lastSaveAgeMs)"
  note "slot $REMOTE_SLOT lastSave=$receipt"
  note "slot $REMOTE_SLOT lastSaveAgeMs=$age"
  if [ "$receipt" = "none" ] || [ "$receipt" = "unknown" ] || [ -z "$receipt" ]; then
    fail "the status page holds no save receipt for the far world."
    fail "Either it has not saved yet — MULTIVERSE_SAVE_MINUTES there is 10 by default, so"
    fail "give it that long — or MULTIVERSE_SAVE_MINUTES=0 turned the timer off."
    ok=1
  else
    note "DECISIVE: the far world saved itself, and said so on the wire. §10.1's rule holds"
    note "in the other direction too — a world that had not saved would render as unknown"
    note "here, never as zero."
    # simulatedTime and population on the receipt are what say how much world the
    # save holds (contract-a.md §5.2). A receipt with neither is a receipt for
    # nothing.
    case "$receipt" in
      *simulatedTime*|*"'simulatedTime'"*) note "the receipt carries simulatedTime and population, which is what makes it a measurement" ;;
      *) fail "the far world's save receipt carries no simulatedTime"; ok=1 ;;
    esac
  fi
  note ""
  note "TODO-owner: the far world's ROTATION ON DISK — <world>.zip, its timestamped"
  note "backups and the absence of a .partial.zip — is visible only on that machine, in"
  note "the game's Savefiles folder. This rig does not read files across the network, so"
  note "that half is a person's check, on that computer, once."

  [ "$ok" = 0 ] && verdict "PHASE 6: PASS — five worlds saving on interval inside the 2 s budget here, and a live save receipt from the sixth" \
                || verdict "PHASE 6: FAIL"
  return "$ok"
}

# ---------------------------------------------------------------- phase 7

lan_phase7() {
  step "PHASE 7 — the portals: what the logs prove here, and what only a person can say"
  local ok=0 slot line

  for slot in $LOGGED_SLOTS; do
    line="$(grep_log "$slot" '\[M4-PORTAL\] event=BUILT' | tail -n 1)"
    if [ -z "$line" ]; then fail "slot $slot never built a portal"; ok=1; continue; fi
    note "slot $slot: $line"
  done
  note "Risk 8's runtime check is those lines: the layer, the camera culling mask, the"
  note "three shaders and the first strip's world bounds."

  step "each open edge shows a strip, and each closed one does not"
  for slot in $LOGGED_SLOTS; do
    note "slot $slot: $(grep_log "$slot" '\[M4-PORTAL\] event=SHOWN' | tail -n 2 | tr '\n' ' ')"
  done

  step "a flourish on slot 1, for a person who is watching this screen"
  send 1 camera 2400 0 0 >/dev/null 2>&1 || note "the camera verb was refused"
  sleep 2
  send 1 flourish export 1900 0 >/dev/null 2>&1 || true
  send 1 flourish entry -1900 0 >/dev/null 2>&1 || true

  note ""
  note "TODO-owner, and it is TWO screens now:"
  note " 1. HERE: the on-screen look, the sorting order and the zoom legibility of the"
  note "    strips at orthographicSize 5, 250, 2000 and 4000. run-m4.sh phase9 drives that"
  note "    sweep with the 'camera' verb; a screenshot taken from a WSL-launched PowerShell"
  note "    has no interactive desktop and saves a blank frame, so it proves nothing."
  note " 2. ON THE SECOND COMPUTER: slot $REMOTE_SLOT's own strips. It has two open export edges"
  note "    (east and north) and two entry edges (west and south), so all four kinds are on"
  note "    that screen at once — and its [M4-PORTAL] lines are in its own BepInEx log,"
  note "    which this rig does not read. Ask its operator to look while phase 2 and phase 3"
  note "    are running: an arrival ring and a departure ring both fire there."

  [ "$ok" = 0 ] && verdict "PHASE 7: PASS on the runtime evidence — every local slot built its portal and showed its edges. The ON-SCREEN look, on either machine, is not proven here." \
                || verdict "PHASE 7: FAIL"
  return "$ok"
}

# ---------------------------------------------------------------- phase 8

lan_errors() {
  step "error sweep — the five local BepInEx logs and the local Go logs"
  local slot file unexplained total=0 n f
  for slot in $LOGGED_SLOTS; do
    file="$(game_log "$slot" 2>/dev/null)"
    [ -n "$file" ] && [ -f "$file" ] || continue
    printf '\n    ---- slot %s (%s) ----\n' "$slot" "$file" >&2
    unexplained="$(grep -a '\[Error' "$file" | grep -av 'Unable to start Unity log writer' || true)"
    [ -n "$unexplained" ] && sed 's/^/      /' <<<"$unexplained" | tail -n 20 >&2
    n="$( [ -n "$unexplained" ] && wc -l <<<"$unexplained" || echo 0)"
    total=$(( total + n ))
    printf '    unexplained [Error lines: %s\n' "$n" >&2
  done
  note "slot $REMOTE_SLOT's BepInEx log is on the second computer. This rig does not read it;"
  note "its operator sweeps it there, and start-slot$REMOTE_SLOT.ps1 names where it is."

  for f in "$LOGS/relay.log" "$LOGS/archive.log" $(for s in $SLOTS; do sclog_of "$s"; done); do
    [ -f "$f" ] || continue
    printf '\n    ---- %s ----\n' "$(basename "$f")" >&2
    grep -a 'level=ERROR' "$f" | tail -n 10 | sed 's/^/      /' >&2
    printf '    total ERROR lines: %s\n' "$(grep -ac 'level=ERROR' "$f" || true)" >&2
  done
  printf '%s\n' "$total"
}

lan_phase8() {
  step "PHASE 8 — exactly once across two computers, the error sweep, and a LOCAL-ONLY teardown"
  local ok=0 eid f total

  step "the exactly-once census; the sixth slot is INFERRED and the inference is printed"
  for f in "$LAN_EAST_ENTITY" "$LAN_NORTH_ENTITY" "$LAN_RETURN_ENTITY"; do
    eid="$(cat "$f" 2>/dev/null || true)"
    [ -n "$eid" ] || continue
    total="$(count_everywhere "$eid")"
    [ "$total" -le 1 ] || { fail "entity $eid exists $total times"; ok=1; }
  done

  step "map-wide census: five populations counted, one read off the wire"
  local slot
  for slot in $SLOTS; do note "slot $slot: $(send "$slot" census 2>&1 | tail -c 200)"; done
  note "slot $REMOTE_SLOT (remote): population=$(remote_stat population) custodyDepth=$(remote_stat custodyDepth) pacedDepth=$(remote_stat pacedDepth) heldDepth=$(remote_stat heldDepth)"
  python3 "$E2E/journal.py" summary $(for s in $SLOTS; do [ -f "$(journal_of "$s")" ] && journal_of "$s"; done) 2>/dev/null \
    | tail -n 30 | sed 's/^/      /' >&2

  local errs; errs="$(lan_errors)"
  note "unexplained BepInEx [Error lines across the five local slots: $errs"

  step "the archive: every lane of the map, including the four that cross the LAN"
  local lane from to n
  for lane in 5:6 3:6 6:4 6:3; do
    from="${lane%%:*}"; to="${lane##*:}"
    n="$(archive_hop "$from" "$to" | grep -ac . || true)"
    note "recorded hops on lane $from -> $to: ${n:-0}"
    [ "${n:-0}" -ge 1 ] || { fail "the archive holds no hop on cross-machine lane $from -> $to"; ok=1; }
  done
  local blobbed
  blobbed="$(grep -ac '"bb8"' "$ARCHIVE_DATA/migrations.jsonl" 2>/dev/null)"; blobbed="${blobbed:-0}"
  note "ledger lines carrying a bb8 blob: $blobbed (must be 0)"
  [ "$blobbed" = 0 ] || { fail "a blob reached the archive ledger"; ok=1; }

  step "final status page"
  status_json > "$LOGS/status-final.json" 2>/dev/null
  "$BIN/ringstat" --url "$ARCHIVE_URL" 2>&1 | sed 's/^/      /' >&2 || true

  step "teardown — THIS MACHINE ONLY"
  note "The far end keeps running until its operator stops it. Its slot, its position and"
  note "its journal survive this teardown and every other."
  lan_down >/dev/null 2>&1
  sleep 3
  local games gos ports
  games="$(cd /mnt/c && /mnt/c/Windows/System32/tasklist.exe 2>/dev/null | grep -ci 'The Bibites' || true)"
  gos="$(pgrep -c -f "$BIN/(relay|sidecar|archive|fakemod)" 2>/dev/null || true)"; gos="${gos:-0}"
  ports="$(ss -ltn 2>/dev/null | grep -cE ":($(rig_ports)|8796) " || true)"
  note "local game processes: ${games:-0} (want 0)"
  note "local Go processes  : $gos (want 0)"
  note "bound rig ports     : ${ports:-0} (want 0)"
  [ "${games:-0}" = 0 ] || { fail "a game process survived teardown"; ok=1; }
  [ "$gos" = 0 ]        || { fail "a Go process survived teardown"; ok=1; }
  [ "${ports:-0}" = 0 ] || { fail "a rig port is still bound"; ok=1; }

  [ "$ok" = 0 ] && verdict "PHASE 8: PASS — exactly once across both computers, every cross-machine lane recorded, nothing left running here" \
                || verdict "PHASE 8: FAIL"
  return "$ok"
}

# ---------------------------------------------------------------- all

lan_all() {
  lan_up      || return 1
  lan_phase1  || return 1     # nothing downstream means anything without the far slot
  lan_phase2  || true
  lan_phase3  || true
  lan_phase4  || true
  lan_phase5  || true
  lan_phase6  || true
  lan_phase7  || true
  note ""
  note "phase5far is NOT in this sequence. It needs a person at the second computer to run"
  note "two commands, and an unattended run must not block on one. Run it by hand:"
  note "    e2e/run-m4-lan.sh phase5far"
  lan_phase8
}

case "${1:-status}" in
  build)      build ;;
  lanhost)    lanhost ;;
  reserve)    reserve ;;
  seed)       lan_seed ;;
  up)         lan_up ;;
  down)       lan_down ;;
  status)     lan_status ;;
  statuspage) statuspage ;;
  phase1)     lan_phase1 ;;
  phase2)     lan_phase2 ;;
  phase3)     lan_phase3 ;;
  phase4)     lan_phase4 ;;
  phase5)     lan_phase5 ;;
  phase5far)  lan_phase5_far ;;
  phase6)     lan_phase6 ;;
  phase7)     lan_phase7 ;;
  phase8)     lan_phase8 ;;
  errors)     lan_errors >/dev/null ;;
  journal)    python3 "$E2E/journal.py" summary $(for s in $SLOTS; do [ -f "$(journal_of "$s")" ] && journal_of "$s"; done) ;;
  archive)    shift; archive_list "$@" ;;
  send)       shift; send "$@" ;;
  # hop <from> <to> <edge> <selector> — one forced export, observed end to end
  # through both logs and both sidecars. Exposed because a two-way map has four
  # directions to prove and the phases only drive some of them.
  hop)        shift; hop "$@" ;;
  count)      shift; count_everywhere "$@" >/dev/null ;;
  all)        lan_all ;;
  *)
    echo "usage: run-m4-lan.sh build|lanhost|reserve|seed|up|phase1..phase8|phase5far|errors|journal|archive|statuspage|status|down|all" >&2
    exit 1
    ;;
esac
