#!/usr/bin/env bash
# Read one test rig's state from /api/status and report whether it meets the bar.
# READ-ONLY: one HTTP GET and nothing else.
#
# The historical protocol-crossing runbook uses this mechanical check.
# It avoids a manual comparison of the status values.
#
#   e2e/crossing/rig-check.sh              # the pre-window bar: 6/6, 24/24
#   e2e/crossing/rig-check.sh --expect 5   # during the window: 5 live is expected
#   e2e/crossing/rig-check.sh --wire       # also require contract-a/2.4 and 0.6.4
#
# EXIT 0 means the bar is met; 1 means it is not; 2 means the page did not answer,
# which during a bring-up usually means the archive is still inside its ledger
# replay and is not a verdict about the map.
set -uo pipefail

URL="${ARCHIVE_URL:-http://127.0.0.1:8796}"
EXPECT_SLOTS="${EXPECT_SLOTS:-6}"
want_wire=0

while [ $# -gt 0 ]; do
  case "$1" in
    --expect) EXPECT_SLOTS="$2"; shift 2 ;;
    --wire)   want_wire=1; shift ;;
    --url)    URL="$2"; shift 2 ;;
    *) echo "usage: rig-check.sh [--expect N] [--wire] [--url http://host:port]" >&2; exit 2 ;;
  esac
done

body="$(curl -s -m 30 "$URL/api/status")" || body=""
if [ -z "$body" ]; then
  echo "the status page at $URL did not answer."
  echo "If the archive was just started it is inside its ledger replay and serves"
  echo "nothing until that finishes — size the wait from the ledger, not from a guess."
  exit 2
fi

# The page's JSON travels in the environment, not on stdin: the here-document
# below IS stdin, so a pipe here would be swallowed by the interpreter.
STATUS_JSON="$body" EXPECT_SLOTS="$EXPECT_SLOTS" WANT_WIRE="$want_wire" python3 - <<'PY'
import json, os, sys

d = json.loads(os.environ["STATUS_JSON"])
expect = int(os.environ["EXPECT_SLOTS"])
want_wire = os.environ["WANT_WIRE"] == "1"
bad = []

slots = d.get("slots", [])
t = d.get("totals", {})
lanes = d.get("lanes", [])

print("relayUrl        %s" % d.get("relayUrl"))
print("relayConnected  %s" % d.get("relayConnected"))
print("ledgerRecords   %s   genomeGaps %s   skippedLines %s"
      % (d.get("ledgerRecords"), d.get("genomeGaps"), d.get("ledgerSkippedLines")))
print("totals          live %s  dark %s  holes %s  unknown %s  pop %s"
      % (t.get("liveSlots"), t.get("darkSlots"), t.get("holes"),
         t.get("unknownSlots"), t.get("population")))
print("queues          custody %s  paced %s  held %s  timeoutBounces %s"
      % (t.get("custodyDepth"), t.get("pacedDepth"), t.get("heldDepth"),
         t.get("timeoutBounces")))
print("migrations      %s at %s/min" % (t.get("migrations"), t.get("perMinute")))
print("")
print("slot peerId    live mod  pop  ts    achieved sm/sk edges mod     contract-a")
for s in sorted(slots, key=lambda s: s.get("slot", 0)):
    ach = s.get("achievedTimeScale")
    print("%-4s %-9s %-4s %-4s %-4s %-5s %-8s %s/%-3s %-5s %-7s %s" % (
        s.get("slot"), s.get("peerId"), s.get("live"), s.get("modConnected"),
        s.get("population"), s.get("timeScale"),
        ("%.1f" % ach) if isinstance(ach, (int, float)) else "-",
        s.get("saveMinutes"), s.get("saveKeep"),
        len(s.get("exportEdges") or []), s.get("modVersion"), s.get("contractAVersion")))

live = [s for s in slots if s.get("live")]
if len(live) != expect:
    bad.append("%d slots live, want %d" % (len(live), expect))
notmod = [s.get("slot") for s in live if not s.get("modConnected")]
if notmod:
    bad.append("live but modConnected false: %s "
               "(config race writes 'configuration failed'; log starvation writes nothing "
               "at all — dev_environment.md, Gotchas)" % notmod)

# 24/24 counts sum(len(exportEdges)) over the LIVE slots, which is the same
# arithmetic the living-deployment readings use.
want_lanes = 4 * expect
declared = sum(len(s.get("exportEdges") or []) for s in live)
open_lanes = [l for l in lanes if l.get("open")]
closed = [(l.get("fromSlot"), l.get("edge"), l.get("reason")) for l in lanes if not l.get("open")]
bypass = [(l.get("fromSlot"), l.get("edge"), l.get("toSlot")) for l in lanes if l.get("skipped")]
print("")
print("lanes           %d/%d open, %d closed, %d bypass"
      % (len(open_lanes), declared, len(closed), len(bypass)))
if closed:
    print("  closed        %s" % closed)
if bypass:
    print("  bypass        %s" % bypass)
if declared != want_lanes:
    bad.append("%d lanes declared over %d live slots, want %d" % (declared, len(live), want_lanes))
if len(open_lanes) != declared:
    # A HOLE IN THE MAP CLOSES LANES LEGITIMATELY. With slot 6 dark, column 2 is
    # {3, 6} and section 2.1 closes slot 3's north and south with `no_peer` until
    # the second computer joins; slot 5's east re-pairs around the hole and stays
    # open as a bypass. That closure IS the evidence of a hole, not a fault — so
    # while fewer than six slots are expected it is reported and not failed.
    no_peer = [c for c in closed if c[2] == "no_peer"]
    if expect < 6 and len(no_peer) == len(closed):
        print("note            %d lane(s) closed with no_peer, which is what a hole in the map "
              "looks like while %d slots are expected" % (len(closed), expect))
    else:
        bad.append("%d of %d lanes open" % (len(open_lanes), declared))

# heldDepth and timeoutBounces are the HARD zeros: a held entry is an organism
# waiting on a dark destination, and a timeout bounce is one that gave up.
for k in ("heldDepth", "timeoutBounces"):
    if t.get(k):
        bad.append("%s is %s, want 0" % (k, t.get(k)))
# custodyDepth and pacedDepth are NOT. Both flicker on a busy map — the live
# deployment reads custody 24-29 and paced 2-8 at x100 while perfectly healthy —
# and the reading that matters is a depth that never FALLS, which one sample
# cannot show. An early M5 snapshot required 0 on pacedDepth. The measured rig
# proved that a nonzero sample can be healthy, so this check reports it.
for k in ("custodyDepth", "pacedDepth"):
    if t.get(k):
        print("note            %s is %s — normal on a busy map; the signal is a depth that "
              "never falls, not a non-zero sample" % (k, t.get(k)))

# The historical crossing rig used saveMinutes 10, saveKeep 6, and timeScale 100
# for its five local worlds. This check does not assert slot 6's configuration.
for s in live:
    if s.get("slot") == 6:
        continue
    if (s.get("saveMinutes"), s.get("saveKeep")) != (10, 6):
        bad.append("slot %s reports saveMinutes/saveKeep %s/%s, want 10/6 — it came up from a "
                   "stale environment and needs its GAME restarted, not the rig"
                   % (s.get("slot"), s.get("saveMinutes"), s.get("saveKeep")))
    if round(float(s.get("timeScale") or 0)) != 100:
        bad.append("slot %s reports timeScale %s, want 100 — re-send it: "
                   "e2e/run-m4-lan.sh send %s timescale 100"
                   % (s.get("slot"), s.get("timeScale"), s.get("slot")))

if want_wire:
    url = d.get("relayUrl") or ""
    if not url.startswith("wss://") or not url.endswith("/contract-b/v4"):
        bad.append("the archive dials %s, want wss://.../contract-b/v4" % url)
    for s in live:
        if s.get("contractAVersion") != "contract-a/2.4":
            bad.append("slot %s speaks %s, want contract-a/2.4%s"
                       % (s.get("slot"), s.get("contractAVersion"),
                          " (slot 6 stays behind until its operator applies the bundle)"
                          if s.get("slot") == 6 else ""))
        if s.get("slot") != 6 and s.get("modVersion") != "0.6.4":
            bad.append("slot %s runs mod %s, want 0.6.4" % (s.get("slot"), s.get("modVersion")))

print("")
if bad:
    print("NOT AT THE BAR:")
    for b in bad:
        print("  - %s" % b)
    sys.exit(1)
print("AT THE BAR: %d/%d live and modConnected, %d/%d lanes peer_live, queues clear."
      % (len(live), expect, len(open_lanes), declared))
PY
