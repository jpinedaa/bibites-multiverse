#!/usr/bin/env bash
# Exercise monitor.sh's money and memory arithmetic on a workstation.
#
# THE ARITHMETIC IS THE RISK, not the plumbing. A transfer check that alerts
# every five minutes gets muted, and a transfer check that never alerts is
# decoration; both failures are silent and both are arithmetic. The archive's
# memory gate has the same shape and one worse failure of its own: it once
# divided by RAM plus swap, so a swap file could clear a critical verdict
# without changing one byte of retained state. The billing check has a third
# shape again: its input is written from OFF-BOX, so its commonest failure is
# that nothing arrives at all, and a check that reads absence as health is
# worse than no check. So this drives the real monitor.sh through the read
# seams it exposes — MV_PROC_NET_DEV, MV_PROC_NET_ROUTE, MV_HOSTS_FILE,
# MV_PROC_MEMINFO, MV_STATUS_JSON, MV_NOW, MV_STATE, MV_ENV_FILE — against fake
# counters, a fake /proc/meminfo, a fake status document, a fake reconciliation
# file and a fake clock.
#
# It needs no root, no network, no systemd and no host: --only transfer,
# --only hosts-pin, --only replay, --only swap and --only billing skip every
# check that would dial something. Nothing here touches /var, /etc or a real
# interface.
#
#   deploy/test-monitor.sh          run every case
#   deploy/test-monitor.sh -v       also print each case as it passes
set -uo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MON="$HERE/monitor.sh"
[ -x "$MON" ] || { echo "test-monitor: $MON is not executable" >&2; exit 2; }

SHOW=0
[ "${1:-}" = "-v" ] && SHOW=1

TMP="$(mktemp -d "${TMPDIR:-/tmp}/mvmontest.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT

STATE="$TMP/state"
DEV="$TMP/net-dev"
ROUTE="$TMP/net-route"
HOSTS="$TMP/hosts"
ENVF="$TMP/deploy.env"
MEMINFO="$TMP/meminfo"
STATUSJ="$TMP/status.json"

# A parameter file with no live value in it. MV_ALERT_KIND=none is a second belt
# beside --quiet: nothing in this file may ever reach a network.
cat >"$ENVF" <<'EOF'
MV_DOMAIN=multiverse.example
MV_ALERT_KIND=none
MV_ALERT_URL=
EOF

GIB=1073741824
PASS=0
FAIL=0

fail() {
  printf 'FAIL  %s\n' "$1" >&2
  [ $# -gt 1 ] && printf '      %s\n' "${@:2}" >&2
  FAIL=$(( FAIL + 1 ))
}
pass() {
  PASS=$(( PASS + 1 ))
  [ "$SHOW" = 1 ] && printf 'ok    %s\n' "$1"
  return 0
}
eq() { # eq <label> <actual> <expected>
  if [ "$2" = "$3" ]; then pass "$1"; else fail "$1" "expected: $3" "actual:   $2"; fi
}
has() { # has <label> <haystack> <needle>
  case "$2" in
    *"$3"*) pass "$1" ;;
    *) fail "$1" "expected to contain: $3" "actual: $2" ;;
  esac
}
hasnt() { # hasnt <label> <haystack> <needle>
  case "$2" in
    *"$3"*) fail "$1" "expected NOT to contain: $3" "actual: $2" ;;
    *) pass "$1" ;;
  esac
}

# ---------------------------------------------------------------- fixtures

reset_state() { rm -rf "$STATE"; mkdir -p "$STATE/monitor"; }
sset() { printf '%s' "$2" >"$STATE/monitor/$1"; }
sget() { cat "$STATE/monitor/$1" 2>/dev/null; }

dev() { # dev <rx-bytes> <tx-bytes> [iface]
  local i="${3:-eth0}"
  {
    printf 'Inter-|   Receive                                                |  Transmit\n'
    printf ' face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed\n'
    printf '    lo: 51 5 0 0 0 0 0 0 51 5 0 0 0 0 0 0\n'
    printf '  %s: %s 7 0 0 0 0 0 0 %s 9 0 0 0 0 0 0\n' "$i" "$1" "$2"
  } >"$DEV"
}
route() { # route [iface]
  local i="${1:-eth0}"
  {
    printf 'Iface\tDestination\tGateway \tFlags\tRefCnt\tUse\tMetric\tMask\t\tMTU\tWindow\tIRTT\n'
    printf '%s\t00001AAC\t00000000\t0001\t0\t0\t100\t00F0FFFF\t0\t0\t0\n' "$i"
    printf '%s\t00000000\t01001AAC\t0003\t0\t0\t100\t00000000\t0\t0\t0\n' "$i"
  } >"$ROUTE"
}
route_without_default() {
  {
    printf 'Iface\tDestination\tGateway \tFlags\tRefCnt\tUse\tMetric\tMask\t\tMTU\tWindow\tIRTT\n'
    printf 'eth0\t00001AAC\t00000000\t0001\t0\t0\t100\t00F0FFFF\t0\t0\t0\n'
  } >"$ROUTE"
}

# meminfo <MemTotal kB> <SwapTotal kB> <SwapFree kB>. The field order is
# deliberately not the kernel's, because the monitor parses by name and must not
# start depending on position.
meminfo() {
  {
    printf 'MemTotal:       %8s kB\n' "$1"
    printf 'MemFree:          117524 kB\n'
    printf 'SwapFree:       %8s kB\n' "$3"
    printf 'MemAvailable:     393816 kB\n'
    printf 'SwapTotal:      %8s kB\n' "$2"
    printf 'AnonPages:       1215924 kB\n'
  } >"$MEMINFO"
}
status() { printf '{"ledgerRecords": %s, "genomeGaps": 0}\n' "$1" >"$STATUSJ"; }

# THE STATUS DOCUMENT AN ARCHIVE WITH THE RECORD ROLL-UP PUBLISHES. The one
# above is the document the build before it published, and it is kept because
# the fallback path is a real state on a real host: a monitor shipped with this
# kit has to keep working against the binary that is running when it lands.
#
# Each field is a variable with a default so that a case sets only the one it is
# about. Every value below is a shape, not a measurement.
ST_LEDGER=5408123
ST_RAWREC=100000        # records the LAST start parsed
ST_RAWSEC=10            # seconds it took: 0.0001 s a record
ST_SEGS=2
ST_AWAIT=0
ST_WINMS=2592000000     # 30 days
ST_FROMMS=0
ST_SAVED=0
ST_DUP=0
ST_RAWBYTES=6100000000
status_rollup() {
  cat >"$STATUSJ" <<EOF
{"ledgerRecords": $ST_LEDGER,
 "genomeGaps": 0,
 "rollupCoveredRecords": $ST_LEDGER,
 "rollupSavedAtMs": $ST_SAVED,
 "replayRawRecords": $ST_RAWREC,
 "replayRawSeconds": $ST_RAWSEC,
 "duplicatesRefused": $ST_DUP,
 "ledgerSegments": $ST_SEGS,
 "ledgerRawBytes": $ST_RAWBYTES,
 "ledgerRawWindowFromMs": $ST_FROMMS,
 "ledgerWindowMs": $ST_WINMS,
 "ledgerSegmentsAwaitingColdCopy": $ST_AWAIT,
 "ledgerRetiredTotal": 0}
EOF
}

# A record rate this box has already sampled, so a case can exercise the
# projection without waiting an hour of fake clock for the sample to close.
# 1000 records per 1000 s is one a second, which makes every figure below
# readable by eye.
seed_rate() { # seed_rate <records-per-1000s> <now>
  sset replay.rate "$1"
  sset replay.rec "$ST_LEDGER"
  sset replay.rec.at "$2"
}

run() { # run <epoch> <group> [KEY=VALUE ...]
  local now="$1" group="$2"
  shift 2
  env MV_ENV_FILE="$ENVF" MV_STATE="$STATE" \
      MV_PROC_NET_DEV="$DEV" MV_PROC_NET_ROUTE="$ROUTE" MV_HOSTS_FILE="$HOSTS" \
      MV_PROC_MEMINFO="$MEMINFO" MV_STATUS_JSON="$STATUSJ" \
      MV_NOW="$now" "$@" "$MON" --only "$group" --quiet --verbose 2>&1
}

sev_of() { # sev_of <output> <check-name>
  printf '%s\n' "$1" | awk -v c="$2" '$1 == c { print $2; exit }'
}
msg_of() { # msg_of <output> <check-name>
  printf '%s\n' "$1" | awk -v c="$2" '$1 == c { $1 = ""; $2 = ""; sub(/^ +/, ""); print; exit }'
}

# 2026-08 has 31 days, so a month is 744 hours. Every threshold case below sets
# MV_TRANSFER_ALLOWANCE_GB=744, which makes the sustainable rate exactly
# 1 GiB/hour and every expected percentage readable by eye.
BASE="$(date -u -d '2026-08-10T00:00:00Z' +%s)"
JULY="$(date -u -d '2026-07-31T23:58:00Z' +%s)"
FLAT="MV_TRANSFER_ALLOWANCE_GB=744"

# Seed a state directory that is already six hours warm, with a known trailing
# hour and a month-to-date of zero, so a case can isolate one projection.
seed_warm() { # seed_warm <closed-hour-bytes> <since-epoch> <rx> <tx>
  reset_state
  sset net.month 2026-08
  sset net.mtd 0
  sset net.since "$2"
  sset net.rx "$3"
  sset net.tx "$4"
  sset net.hours "$1"
  sset net.hot 0
}

# ---------------------------------------------------------------- 1. seeding

route
dev 1000 2000
printf '127.0.0.1 multiverse.example\n' >"$HOSTS"
reset_state

out="$(run "$BASE" transfer)"
eq  'first tick records the raw receive counter'   "$(sget net.rx)"  1000
eq  'first tick records the raw transmit counter'  "$(sget net.tx)"  2000
eq  'first tick books no bytes at all'             "$(sget net.mtd)" 0
eq  'first tick adopts the current UTC month'      "$(sget net.month)" 2026-08
eq  'first tick starts the observation clock'      "$(sget net.since)" "$BASE"
eq  'first tick raises nothing'                    "$(sev_of "$out" transfer)" OK
has 'first tick says it is warming up'             "$out" 'warming up'
eq  'the rate check reports on the first tick too' "$(sev_of "$out" transfer-rate)" OK
has 'the rate check has no closed hour yet'        "$out" 'no closed hour yet'

# ---------------------------------------------------------------- 2. deltas

dev 1500 2500
run "$(( BASE + 300 ))" transfer >/dev/null
eq 'a delta of both directions accumulates' "$(sget net.mtd)" 1000
dev 2000 3000
run "$(( BASE + 600 ))" transfer >/dev/null
eq 'deltas keep accumulating'               "$(sget net.mtd)" 2000

# ---------------------------------------------------------------- 3. reboot

dev 100 200
run "$(( BASE + 900 ))" transfer >/dev/null
eq 'a counter that went backwards is a reboot, not a negative delta' \
   "$(sget net.mtd)" 2300
eq 'the reboot leaves the new raw counter behind' "$(sget net.rx)" 100

# ---------------------------------------------------------------- 4. hour close

reset_state
dev 0 0
run "$BASE" transfer >/dev/null
dev 5000 0
run "$(( BASE + 1800 ))" transfer >/dev/null
eq  'traffic lands in the open hour' "$(sget net.hourbytes)" 5000
eq  'nothing is closed inside the hour' "$(sget net.hours)" ''
run "$(( BASE + 3700 ))" transfer >/dev/null
eq  'crossing the hour closes the bucket' "$(sget net.hours)" 5000
eq  'the new hour starts empty'           "$(sget net.hourbytes)" 0

reset_state
dev 0 0
for i in $(seq 0 29); do
  dev "$(( i * 1000 ))" "$(( i * 1000 ))"
  run "$(( BASE + i * 3600 + 60 ))" transfer MV_TRANSFER_HOURLY_GB=1000 >/dev/null
done
eq 'the closed-hour ring holds twenty-four hours and no more' \
   "$(awk '{print NF}' "$STATE/monitor/net.hours")" 24

# ---------------------------------------------------------------- 5. hot streak

reset_state
dev 0 0
run "$BASE" transfer MV_TRANSFER_HOURLY_GB=1 >/dev/null
for i in 1 2 3 4; do
  dev "$(( i * 2 * GIB ))" 0
  out="$(run "$(( BASE + i * 3600 ))" transfer MV_TRANSFER_HOURLY_GB=1)"
done
eq  'three closed hours over the limit make a streak of three' "$(sget net.hot)" 3
eq  'the streak trips the rate check'   "$(sev_of "$out" transfer-rate)" WARN
has 'the rate warning names the last three hours' "$out" 'last three: 2.0, 2.0, 2.0 GB'
has 'the rate warning states what the allowance sustains' "$out" 'GB/hour the allowance sustains'

# The counters stop moving here, so the hour that closes at +6 h is genuinely
# empty. The hour closing at +5 h still carries the traffic r4 booked into it.
out="$(run "$(( BASE + 5 * 3600 ))" transfer MV_TRANSFER_HOURLY_GB=1)"
eq 'a fourth hot hour keeps counting'    "$(sget net.hot)" 4
out="$(run "$(( BASE + 6 * 3600 ))" transfer MV_TRANSFER_HOURLY_GB=1)"
eq 'a quiet hour resets the streak'      "$(sget net.hot)" 0
eq 'and the rate check recovers'         "$(sev_of "$out" transfer-rate)" OK
has 'reporting the last closed hour'     "$out" 'last closed hour 0.0 GB'

# ---------------------------------------------------------------- 6. warm-up

dev 4000 4000
seed_warm "$GIB" "$(( BASE - 3600 ))" 4000 4000
out="$(run "$BASE" transfer $FLAT)"
eq  'one hour of observation cannot raise a projection' "$(sev_of "$out" transfer)" OK
has 'and it says why'                                   "$out" 'the projection needs six hours'

sset net.since "$(( BASE - 21600 ))"
out="$(run "$(( BASE + 60 ))" transfer $FLAT)"
eq  'six hours of observation lets the same figure through' "$(sev_of "$out" transfer)" CRIT

# ---------------------------------------------------------------- 7. projection B

dev 4000 4000
seed_warm 751619277 "$(( BASE - 86400 ))" 4000 4000
out="$(run "$BASE" transfer $FLAT)"
eq  'seventy percent of the allowance is not an alert' "$(sev_of "$out" transfer)" OK
has 'and the OK line still carries the projection'     "$out" 'projecting 521 GB (70.0% of 744 GB)'

seed_warm 859000000 "$(( BASE - 86400 ))" 4000 4000
out="$(run "$BASE" transfer $FLAT)"
eq  'the trailing rate warns at eighty percent' "$(sev_of "$out" transfer)" WARN
has 'the warning names the trailing projection' "$out" 'the last 24 h project a full month of'
has 'the warning states the billing rate'       "$out" '$0.09/GB'
has 'the warning carries the same measured levers' "$out" '21 GB/month for each crossing per second'

seed_warm "$GIB" "$(( BASE - 86400 ))" 4000 4000
out="$(run "$BASE" transfer $FLAT)"
eq  'the trailing rate is critical at a hundred percent' "$(sev_of "$out" transfer)" CRIT
has 'the critical line says both directions are counted' "$out" 'counted in BOTH directions'
has 'the critical line points at the levers'             "$out" 'deploy/SIZING.md'
# THE LEVERS ARE THE POINT OF THE ALERT. An operator who reads this line at 03:00
# acts on the number in it, so the numbers are pinned to SIZING.md's measured
# model here: peer traffic is a crossing-rate term and NOT a time-scale term, and
# the retired "50 GB per unit of S" rule must never come back. The peer figure is
# the COMPRESSED one since 2026-08-17, because that is the wire the host runs;
# the uncompressed 88 stays beside it so a map without the extension still reads
# a true number.
has 'the critical line gives the measured peer lever'    "$out" '21 GB/month for each crossing per second'
has 'the critical line keeps the uncompressed figure'    "$out" '88 GB/month uncompressed'
has 'the critical line says time scale is not a lever'   "$out" 'does NOT fall with time scale'
has 'the critical line gives the measured viewer lever'  "$out" '475 GB/month at 1.5 Mbit/s'
has 'the critical line gives the ingest lever'           "$out" '470 GB/month'
hasnt 'the critical line no longer sells the retired rule' "$out" '50 GB/month per unit'
has 'the critical line names the interface it read'      "$out" '/proc/net/dev on eth0'

# ---------------------------------------------------------------- 8. projection A

# A month-to-date line with no closed hour at all: B is unknown, and unknown
# must not be read as zero.
reset_state
sset net.month 2026-08
sset net.since "$(( BASE - 86400 ))"
sset net.mtd "$(( 744 * GIB ))"
sset net.rx 4000
sset net.tx 4000
dev 4000 4000
out="$(run "$BASE" transfer $FLAT)"
eq  'the burn-down line alone can raise a critical' "$(sev_of "$out" transfer)" CRIT
has 'and it says the burn-down drove it'           "$out" 'the month-to-date line is at'
eq  'the rate check still reports with no hours'   "$(sev_of "$out" transfer-rate)" OK

# ---------------------------------------------------------------- 9. hysteresis

seed_warm "$GIB" "$(( BASE - 86400 ))" 4000 4000
out="$(run "$BASE" transfer $FLAT)"
eq 'a hundred percent is critical'                 "$(sev_of "$out" transfer)" CRIT
sset net.hours 1052266987
out="$(run "$(( BASE + 300 ))" transfer $FLAT)"
eq 'ninety-eight percent stays critical'           "$(sev_of "$out" transfer)" CRIT
sset net.hours 1036160860
out="$(run "$(( BASE + 600 ))" transfer $FLAT)"
eq 'three points of clearance releases it'         "$(sev_of "$out" transfer)" WARN
sset net.hours 848256840
out="$(run "$(( BASE + 900 ))" transfer $FLAT)"
eq 'seventy-nine percent is not clear of the warning threshold' \
   "$(sev_of "$out" transfer)" WARN
sset net.hours 816043786
out="$(run "$(( BASE + 1200 ))" transfer $FLAT)"
eq 'seventy-six percent is'                        "$(sev_of "$out" transfer)" OK

# ---------------------------------------------------------------- 10. rollover

reset_state
sset net.month 2026-07
sset net.mtd 999999999
sset net.since "$JULY"
sset net.rx 1000
sset net.tx 2000
sset net.hours 123
dev 1500 2500
run "$BASE" transfer >/dev/null
eq 'a new month starts from zero'                  "$(sget net.mtd)" 1000
eq 'and records which month it is now'             "$(sget net.month)" 2026-08
eq 'and restarts the observation clock'            "$(sget net.since)" "$BASE"

# ---------------------------------------------------------------- 11. bad reads

reset_state
dev 1000 2000 ens5
out="$(run "$BASE" transfer)"
eq  'an interface that is not in /proc/net/dev warns' "$(sev_of "$out" transfer)" WARN
has 'and says the allowance is unwatched'            "$out" 'not being watched'

reset_state
dev 1000 2000 ens5
out="$(run "$BASE" transfer MV_TRANSFER_IFACE=ens5)"
eq  'MV_TRANSFER_IFACE overrides the route lookup'   "$(sev_of "$out" transfer)" OK
eq  'and the override reads the right counters'      "$(sget net.rx)" 1000

reset_state
route_without_default
dev 1000 2000
out="$(run "$BASE" transfer)"
eq  'no default route warns rather than guessing'    "$(sev_of "$out" transfer)" WARN
has 'and names the knob that fixes it'               "$out" 'MV_TRANSFER_IFACE'
route

# ---------------------------------------------------------------- 12. hosts pin

reset_state
printf '127.0.0.1 localhost\n127.0.0.1 multiverse.example\n' >"$HOSTS"
out="$(run "$BASE" hosts-pin)"
eq  'the loopback pin is found'  "$(sev_of "$out" hosts-pin)" OK
has 'and reported plainly'       "$out" 'to loopback'

printf '127.0.0.1\tmultiverse.example\t# pinned\n' >"$HOSTS"
out="$(run "$BASE" hosts-pin)"
eq  'a tab-separated pin with a trailing comment is found' \
    "$(sev_of "$out" hosts-pin)" OK

printf '127.0.0.1 localhost\n' >"$HOSTS"
out="$(run "$BASE" hosts-pin)"
eq  'a missing pin is critical'  "$(sev_of "$out" hosts-pin)" CRIT
has 'and says what it costs'     "$out" '54 GB/day'
has 'and says how to restore it' "$out" 'sudo tee -a'

printf '127.0.0.1 multiverse.example.org\n' >"$HOSTS"
out="$(run "$BASE" hosts-pin)"
eq  'a longer name that merely starts the same does not count' \
    "$(sev_of "$out" hosts-pin)" CRIT

# ------------------------------------------------------- 13. replay headroom
#
# The numbers below are the production service host as it stood on 2026-08-16:
# MemTotal 1,953,544 kB (1,907 MB) and 5,213,509 ledger records, which the
# shipped 300 B/record model projects to 1,491 MB and a ratio of 0.78.

reset_state
status 5213509
meminfo 1953544 0 0
out="$(run "$BASE" replay)"
eq  'the production reading is a warning'            "$(sev_of "$out" replay)" WARN
has 'and reports the ratio against physical RAM'     "$out" 'ratio 0.78'
has 'and names the denominator'                      "$out" '1907 MB of physical RAM'

# THE SWAP TRAP, and the reason this section exists. The same box with a 2 GiB
# swap file must report the SAME ratio and the SAME severity. The old
# denominator was MemTotal + SwapTotal, which turned this reading into 0.38 and
# an OK — a critical archive-memory verdict cleared by one swapon, with the
# ledger, the retained state and the replay all exactly as they were.
reset_state
meminfo 1953544 2097152 2097152
out="$(run "$BASE" replay)"
eq  'adding two gigabytes of swap does not move the ratio' \
    "$(sev_of "$out" replay)" WARN
has 'and the ratio is the identical figure'          "$out" 'ratio 0.78'
hasnt 'and swap is nowhere in the denominator'       "$out" 'RAM+swap'

reset_state
status 5700000
out="$(run "$BASE" replay)"
eq  'a larger ledger crosses the critical threshold' "$(sev_of "$out" replay)" CRIT
has 'the critical line says the denominator is physical RAM' \
    "$out" 'PHYSICAL RAM'
has 'and says the ratio is a model of record count'  "$out" 'MODEL OF RECORD COUNT'
has 'and says GOMEMLIMIT lowers resident set'        "$out" 'lowers real resident set'
has 'and says GOMEMLIMIT does not move this number'  "$out" 'does not move this number'
hasnt 'and no longer claims GOMEMLIMIT does not help' \
      "$out" 'GOMEMLIMIT and swap do not fix it'
has 'and points at the constants a measurement would correct' \
    "$out" 'MV_REPLAY_RESIDENT_B'

# The constants are parameters, so a corrected measurement retunes the gate
# without a release.
reset_state
status 5213509
out="$(run "$BASE" replay MV_REPLAY_RESIDENT_B=200)"
eq  'a cheaper measured resident model clears the warning' \
    "$(sev_of "$out" replay)" OK
has 'and reports the smaller projection'             "$out" '994 MB projected resident set'

reset_state
out="$(run "$BASE" replay MV_REPLAY_PEAK_B=400 MV_REPLAY_RESIDENT_B=200)"
eq  'a peak model larger than the resident one binds instead' \
    "$(sev_of "$out" replay)" CRIT
has 'and the message says which term binds'          "$out" 'replay peak'

reset_state
out="$(run "$BASE" replay MV_REPLAY_RESIDENT_B=notanumber)"
eq  'a per-record constant that is not a number warns rather than dividing by it' \
    "$(sev_of "$out" replay)" WARN
has 'and names both constants'                       "$out" 'MV_REPLAY_PEAK_B'

reset_state
printf 'MemFree: 117524 kB\nSwapTotal: 0 kB\n' >"$MEMINFO"
out="$(run "$BASE" replay)"
eq  'no MemTotal warns rather than reporting a ratio of nothing' \
    "$(sev_of "$out" replay)" WARN
has 'and says the archive is unwatched'              "$out" 'not being watched'

# ------------------------------------------- 13b. what the restart will COST
#
# The memory gate above answers "will it fit". This answers "what will it cost
# the map in seconds", which is the question the roll-up exists to change and
# the one nobody was watching: the record-preserving sequence holds the relay
# down for the whole replay, so the projection IS the participant outage.

# THE FALLBACK, and it is a real state: the build running when this kit lands
# publishes neither field, and the record-count model is then the only estimate.
# It must say which model is in force rather than reporting silence.
reset_state
status 5213509
meminfo 1953544 0 0
out="$(run "$BASE" replay)"
eq  'an archive with no replay measurement still gets a memory verdict' \
    "$(sev_of "$out" replay)" WARN
eq  'and the cost check reports rather than staying silent' \
    "$(sev_of "$out" replay-cost)" OK
has 'and it names the field it needs'                "$out" 'replayRawRecords'
has 'and it says the fallback is a memory model'     "$out" 'model of MEMORY'
hasnt 'and the memory verdict carries no roll-up caveat on that build' \
      "$out" 'this archive has a roll-up'

# With the fields present but no record rate yet, it reports the MEASUREMENT and
# says a projection needs a sample. It must not report the last start as the
# next one: the first start after this deployment reads the whole ledger once.
reset_state
status_rollup
out="$(run "$BASE" replay)"
eq  'a warming projection is not an alarm'           "$(sev_of "$out" replay-cost)" OK
has 'and it states what the last start measured'     "$out" '10.0 s to parse 100000 raw record'
has 'and it says a sample is still opening'          "$out" 'record rate'
has 'and it names the window that will bound it'     "$out" '48h'

# With a sample, the projection is the whole model: records in the window at the
# cost per record the archive itself measured.
#   1 record/s * 172,800 s = 172,800 records * 0.0001 s = 17 s
reset_state
seed_rate 1000 "$BASE"
out="$(run "$BASE" replay)"
eq  'a cheap restart is OK'                          "$(sev_of "$out" replay-cost)" OK
has 'and it prints the projected seconds'            "$out" '~17 s projected'
has 'and the record count it projected'              "$out" '172800 raw records'

# The same box, a build that parses a hundred times slower: 1,728 s of held-down
# relay, which is a scheduled window and not a maintenance action.
reset_state
ST_RAWSEC=1000 status_rollup
seed_rate 1000 "$BASE"
out="$(run "$BASE" replay)"
eq  'an unaffordable outage is critical'             "$(sev_of "$out" replay-cost)" CRIT
has 'and it says the replay time is the outage'      "$out" 'THE REPLAY TIME IS THE PARTICIPANT OUTAGE'
has 'and it names the one knob that moves it'        "$out" 'MV_ARCHIVE_DEDUP_WINDOW'
has 'and it names the counter that says lowering is safe' "$out" 'duplicatesRefused'

reset_state
ST_RAWSEC=200 status_rollup
seed_rate 1000 "$BASE"
out="$(run "$BASE" replay)"
eq  'an outage that must be announced is a warning'  "$(sev_of "$out" replay-cost)" WARN
has 'and it says so'                                 "$out" 'announced before it is done'

# THE KNOB. The same binary, the same state, a shorter duplicate window: the
# projection falls with it, because the window is what the restart reads.
reset_state
ST_RAWSEC=1000 status_rollup
seed_rate 1000 "$BASE"
out="$(run "$BASE" replay MV_ARCHIVE_DEDUP_WINDOW=1h)"
eq  'a one-hour window clears the critical projection' "$(sev_of "$out" replay-cost)" OK
has 'and the window it used is the one that was set'   "$out" '3600s'

# A window this cannot parse must never be read as no window at all, which would
# make every restart project as free.
reset_state
seed_rate 1000 "$BASE"
out="$(run "$BASE" replay MV_ARCHIVE_DEDUP_WINDOW=notaduration)"
eq  'an unparseable window falls back and stays critical' "$(sev_of "$out" replay-cost)" CRIT
has 'and it uses the default window'                 "$out" '172800s'

# The compressed-segment term the model does not carry, named rather than
# implied: reaching the window's start is a decompress-and-discard on a gz.
has 'and it says reaching the window is not in the figure' "$out" "Reaching the window's start is not in this figure"

# The memory gate says out loud that it is now the conservative model, so that
# nobody reads a record-count ratio as a measurement of a bounded process.
reset_state
status_rollup
out="$(run "$BASE" replay)"
has 'the memory gate admits the record-count model over-estimates' \
    "$out" 'this archive has a roll-up'
has 'and points at the check that answers the other question' \
    "$out" 'replay-cost'

# ------------------------------------------- 13c. the record layer
#
# Three checks, and each one watches something whose failure is INVISIBLE
# everywhere else: an off-host copy that stopped (nothing else on this host
# looks at the object store), a state sidecar that stopped saving (no published
# number moves), and a duplicate counter that is the only evidence the window
# may come down.

reset_state
status_rollup
out="$(run "$BASE" archive)"
eq  'a healthy record layer is OK on all three'      "$(sev_of "$out" cold-copy)" OK
eq  'the state sidecar too'                          "$(sev_of "$out" rollup)" OK
eq  'and the duplicate counter'                      "$(sev_of "$out" duplicates)" OK

# THE FIRST-PASS BLIND SPOT. The archive refreshes the segment fields in its
# maintenance pass, and the pass at start COMPRESSES before it refreshes — 21 s
# on a real legacy segment. A reading taken inside that window shows a healthy
# archive with no segments at all, and a check that fires on it fires on every
# restart.
reset_state
ST_SEGS=0 ST_AWAIT=0 ST_SAVED=0 status_rollup
out="$(run "$BASE" archive)"
eq  'no segment layer yet is not an alarm'           "$(sev_of "$out" cold-copy)" OK
has 'and it says the pass has not reported yet'      "$out" 'not reported yet'

# A segment waiting for its copy is normal for the hour after a rotation.
reset_state
ST_AWAIT=1 ST_SAVED=$(( BASE * 1000 )) status_rollup
out="$(run "$BASE" archive)"
eq  'one segment waiting is not yet a problem'       "$(sev_of "$out" cold-copy)" OK
has 'and it says the timer runs hourly'              "$out" 'timer runs hourly'

# A day of waiting is the copy failing rather than lagging.
reset_state
sset coldcopy.awaiting.since "$(( BASE - 25 * 3600 ))"
out="$(run "$BASE" archive)"
eq  'a day of waiting is a warning'                  "$(sev_of "$out" cold-copy)" WARN
has 'and it says the record is safe and the disk is not' "$out" 'costs disk and not record'
has 'and it names the mode that diagnoses it'        "$out" 'coldcopy.sh --check'

# The critical case is not lateness. It is the raw on this host reaching back
# further than the window was ever meant to allow, which is the point at which
# the window is doing nothing at all.
reset_state
ST_AWAIT=3 ST_SAVED=$(( BASE * 1000 )) ST_FROMMS=$(( BASE * 1000 - 6000000000 )) status_rollup
sset coldcopy.awaiting.since "$(( BASE - 30 * 3600 ))"
out="$(run "$BASE" archive)"
eq  'a backlog past twice the window is critical'    "$(sev_of "$out" cold-copy)" CRIT
has 'and it contrasts the days held with the window' "$out" '69 days of raw lines against a 30-day window'
has 'and it says no segment is ever removed without a copy' "$out" 'NO SEGMENT IS EVER REMOVED'
has 'and it names the three commonest causes'        "$out" 'storage class'

# A state sidecar that stopped saving breaks nothing and moves no other number.
reset_state
ST_AWAIT=0 ST_FROMMS=0 ST_SAVED=$(( (BASE - 1800) * 1000 )) status_rollup
out="$(run "$BASE" archive)"
eq  'a stale state sidecar is a warning'             "$(sev_of "$out" rollup)" WARN
has 'and it says what it costs: the next restart'    "$out" 'window-bounded restart back into a full one'

# One that has never saved is correct for the first half-minute and not after.
reset_state
ST_SAVED=0 ST_SEGS=2 status_rollup
out="$(run "$BASE" archive)"
eq  'a sidecar that has not saved yet is not an alarm' "$(sev_of "$out" rollup)" OK
sset rollup.zero.since "$(( BASE - 3600 ))"
out="$(run "$BASE" archive)"
eq  'one that has never saved for an hour is a warning' "$(sev_of "$out" rollup)" WARN
has 'and it says every restart from here is a full replay' "$out" 'full replay of the raw ledger'

# duplicatesRefused: zero is the answer that matters and it is reported as such.
reset_state
ST_SAVED=$(( BASE * 1000 )) ST_DUP=0 status_rollup
out="$(run "$BASE" archive)"
eq  'zero refusals is OK'                            "$(sev_of "$out" duplicates)" OK
has 'and it says what zero buys'                     "$out" 'may come down'

# A non-zero counter during the transition is information, not an alarm — and it
# becomes one only when it is still MOVING, because a moving counter is what
# says the window must not come down yet.
reset_state
ST_DUP=7 status_rollup
out="$(run "$BASE" archive)"
eq  'refusals during the transition are not an alarm' "$(sev_of "$out" duplicates)" OK
has 'and it says why they are expected'              "$out" 'B37'

sset dup.24 4
sset dup.24.at "$(( BASE - 25 * 3600 ))"
out="$(run "$BASE" archive)"
eq  'refusals still arriving after a day is a warning' "$(sev_of "$out" duplicates)" WARN
has 'and it forbids lowering the window while it moves' "$out" 'DO NOT LOWER'

sset dup.24 7
sset dup.24.at "$(( BASE - 25 * 3600 ))"
out="$(run "$BASE" archive)"
eq  'a flat day at a non-zero total is OK'           "$(sev_of "$out" duplicates)" OK
has 'and it says a full cycle at zero is the bar'    "$out" 'full cycle at zero'

# ------------------------------------------- 13d. disk, with a bounded ledger
#
# The days-to-full projection is a straight line, and once the window is in
# force and the copy is working the raw ledger is not one. A straight line
# through a term that stops growing reads a healthy host as weeks from full.

reset_state
ST_DUP=0 ST_AWAIT=0 status_rollup
out="$(run "$BASE" replay)"
hasnt 'the disk caveat does not leak into the replay group' "$out" 'raw ledger is bounded'

# ---------------------------------------------------------------- 14. swap

reset_state
meminfo 1953544 0 0
out="$(run "$BASE" swap)"
eq  'no swap is a fact worth reporting, not silence' "$(sev_of "$out" swap)" OK
has 'and it says the ratio is against RAM either way' "$out" 'physical RAM'

reset_state
meminfo 1953544 2097152 2097152
out="$(run "$BASE" swap)"
eq  'an unused swap file is not a problem'           "$(sev_of "$out" swap)" OK
has 'and its size is reported'                       "$out" '0 MB of 2048 MB used'
has 'and it says it is not headroom'                 "$out" 'Not counted as replay headroom'

# Sustained means consecutive passes. One spilled replay peak is the barrier
# working; a quarter of an hour of it is a sizing signal.
reset_state
meminfo 1953544 2097152 1048576
out="$(run "$BASE" swap)"
eq  'half the swap in use once is not yet sustained' "$(sev_of "$out" swap)" OK
out="$(run "$(( BASE + 300 ))" swap)"
eq  'twice is not either'                            "$(sev_of "$out" swap)" OK
out="$(run "$(( BASE + 600 ))" swap)"
eq  'three consecutive passes is'                    "$(sev_of "$out" swap)" WARN
has 'and it is stated as a sizing signal'            "$out" 'sizing signal'
has 'and it repeats that swap is not headroom'       "$out" 'NOT counted as replay headroom'

meminfo 1953544 2097152 2097152
out="$(run "$(( BASE + 900 ))" swap)"
eq  'and it recovers when the swap is given back'    "$(sev_of "$out" swap)" OK
eq  'the streak resets with it'                      "$(sget swap.runs)" 0

# ---------------------------------------------------------------- 15. billing
#
# THE ONE CHECK WHOSE INPUT COMES FROM OFF-BOX. deploy/ce-reconcile.sh runs on
# an operator machine and ships billing.json here, so this check has a failure
# mode none of the others have: nothing arrives, and nothing on this host can
# tell the difference between "the bill is fine" and "the machine that reads the
# bill is closed". Absence has to be a reading, and these cases hold it to that.
#
# The fixture is the shape deploy/test-ce-reconcile.sh produces: three settled
# days at 60 GB in and 40 GB out, a metric-over-invoice ratio of 1.01, and a
# 3,100 GB projection against a 3,072 GB allowance.

billing() { # billing <asOf> [KEY=JSON-VALUE ...] — write a reconciliation file
  local asof="$1"
  shift
  python3 - "$STATE/monitor/billing.json" "$asof" "$@" <<'PY'
import json, sys
path, as_of = sys.argv[1], sys.argv[2]
record = {
    "asOf": as_of, "month": "2026-08", "ceThroughDate": "2026-08-03",
    "ceInGiB": 180.0, "ceOutGiB": 120.0, "ceOverageGiB": 0.0, "ceOverageUsd": 0.0,
    "metricInGiB": 201.8, "metricOutGiB": 131.2,
    "ratioIn": 1.01, "ratioOut": 1.01,
    "projectedMonthGiB": 3100.0, "projectedOverageUsd": 2.52, "allowanceGiB": 3072.0,
    "principal": "root (read-only; owner-accepted 2026-08-16)", "ceCallsThisRun": 1,
    "ceEstimated": True, "ceSettledDays": 3, "daysInMonth": 31,
    "projectedOutGiB": 1240.0, "overageUsdPerGiB": 0.09,
}
for override in sys.argv[3:]:
    key, _, value = override.partition("=")
    record[key] = json.loads(value)
with open(path, "w", encoding="utf-8") as handle:
    json.dump(record, handle, indent=2)
PY
}

iso() { date -u -d "@$1" +%Y-%m-%dT%H:%M:%SZ; }

# NOTHING HAS EVER ARRIVED. Not an outage — the NIC counter above is still
# watching the allowance — so it is a warning that names what stopped and where
# that thing runs, because the operator reading it is not on this host.
reset_state
out="$(run "$BASE" billing)"
eq  'a reconciliation that has never arrived is a warning, not silence' \
    "$(sev_of "$out" billing)" WARN
has 'and it says which script produces it'    "$out" 'deploy/ce-reconcile.sh'
has 'and that it runs off this host'          "$out" 'operator machine'
has 'and that the NIC counter still watches the allowance' \
    "$out" 'still watching the allowance'

# FRESH AND ORDINARY.
reset_state
billing "$(iso "$(( BASE - 3600 ))")"
out="$(run "$BASE" billing)"
eq  'a fresh reading is OK'                   "$(sev_of "$out" billing)" OK
has 'and the OK line carries the through-date' "$out" 'CE through 2026-08-03'
has 'and both invoice quantities'             "$out" 'in 180.0 GB, out 120.0 GB'
has 'and the projection against the allowance' "$out" 'projected 3100.0 GB (101%)'
has 'and the ratio that says the two agree'   "$out" 'metric/CE in 1.01x out 1.01x'

# STALE. A daily job plus one missed run plus the provider's own lag is inside
# the window; two missed runs is not.
reset_state
billing "$(iso "$(( BASE - 35 * 3600 ))")"
out="$(run "$BASE" billing)"
eq  'thirty-five hours is inside the default window' "$(sev_of "$out" billing)" OK

reset_state
billing "$(iso "$(( BASE - 37 * 3600 ))")"
out="$(run "$BASE" billing)"
eq  'thirty-seven hours is stale'             "$(sev_of "$out" billing)" WARN
has 'and it reports the age in hours'         "$out" 'has not reported for 37 h'
has 'and names the limit it crossed'          "$out" 'limit 36 h'

reset_state
billing "$(iso "$(( BASE - 13 * 3600 ))")"
out="$(run "$BASE" billing MV_BILLING_STALE_HOURS=12)"
eq  'the staleness window is a knob'          "$(sev_of "$out" billing)" WARN

# THE PROVIDER HAS BILLED. This is the only reading here that is not an
# inference, and it outranks staleness: a bill that was billed stays billed
# while the reconciliation is late.
reset_state
billing "$(iso "$(( BASE - 3600 ))")" ceOverageGiB=41.5 ceOverageUsd=3.735
out="$(run "$BASE" billing)"
eq  'a billed overage is critical'            "$(sev_of "$out" billing)" CRIT
has 'and it states the money'                 "$out" '$3.735'
has 'and the quantity'                        "$out" 'BILLED 41.5 GB'
has 'and the vendor rule it came from'        "$out" 'min(out, max(0, (in + out) - 3072))'
has 'and the rate'                            "$out" '$0.09/GB'
has 'and that nothing on this host can see it' "$out" 'Nothing on this host can see this'

reset_state
billing "$(iso "$(( BASE - 3600 ))")" ceOverageGiB=0 ceOverageUsd=0.4
out="$(run "$BASE" billing)"
eq  'an overage cost with no quantity is still critical' \
    "$(sev_of "$out" billing)" CRIT

reset_state
billing "$(iso "$(( BASE - 200 * 3600 ))")" ceOverageGiB=41.5 ceOverageUsd=3.735
out="$(run "$BASE" billing)"
eq  'a stale file with a billed overage is still critical' \
    "$(sev_of "$out" billing)" CRIT
has 'and it says the real figure is higher'   "$out" 'the real figure is higher'

# THE TWO INSTRUMENTS DISAGREE. 1.01 is the measured, systematic residual; a
# band of 0.10 either side of 1 lets that through and catches a counter that has
# started reading something else.
reset_state
billing "$(iso "$(( BASE - 3600 ))")" ratioOut=1.34
out="$(run "$BASE" billing)"
eq  'an out-of-band outbound ratio warns'     "$(sev_of "$out" billing)" WARN
has 'and it states the disagreement as a percentage' "$out" 'disagree by 34.0% on outbound'
has 'and says one of them is wrong'           "$out" 'One of them is wrong'
has 'and where to read the detail'            "$out" 'deploy/ce-reconcile.sh --print'
has 'and names the usual cause'               "$out" 'MV_TRANSFER_IFACE'

reset_state
billing "$(iso "$(( BASE - 3600 ))")" ratioIn=0.62
out="$(run "$BASE" billing)"
eq  'an inbound ratio below the band warns too' "$(sev_of "$out" billing)" WARN
has 'and the direction is named'              "$out" 'on inbound'

reset_state
billing "$(iso "$(( BASE - 3600 ))")" ratioOut=1.09
out="$(run "$BASE" billing)"
eq  'nine percent is inside the default band' "$(sev_of "$out" billing)" OK

reset_state
billing "$(iso "$(( BASE - 3600 ))")" ratioOut=1.05
out="$(run "$BASE" billing MV_BILLING_RATIO_TOL=0.02)"
eq  'the band is a knob'                      "$(sev_of "$out" billing)" WARN

# UNKNOWN IS A VALUE, AND IT IS NOT 1.0. A reconciliation that ran before the
# provider's ~14 h lag caught up has no invoice side at all.
reset_state
billing "$(iso "$(( BASE - 3600 ))")" ratioIn=null ratioOut=null
out="$(run "$BASE" billing)"
eq  'an unknown ratio is not read as agreement, and not as drift' \
    "$(sev_of "$out" billing)" OK
has 'and it is printed as unknown rather than as a number' "$out" 'in unknownx'

MONTH_TOP="$(date -u -d '2026-08-01T12:00:00Z' +%s)"
reset_state
billing "$(iso "$(( MONTH_TOP - 1800 ))")" ceThroughDate=null ratioIn=null ratioOut=null
out="$(run "$MONTH_TOP" billing)"
eq  'no invoice data in the first hours of a month is the lag, not a fault' \
    "$(sev_of "$out" billing)" OK
has 'and it says so'                          "$out" '14 h lag'

reset_state
billing "$(iso "$(( BASE - 3600 ))")" ceThroughDate=null ratioIn=null ratioOut=null
out="$(run "$BASE" billing)"
eq  'the same emptiness nine days into the month is a fault' \
    "$(sev_of "$out" billing)" WARN
has 'and it says unknown is not zero'         "$out" 'which is not zero'

# A FILE THAT CANNOT BE DATED CANNOT BE TRUSTED, and reading it as fresh would
# be the worst of the three options.
reset_state
printf '{"month": "2026-08"}\n' >"$STATE/monitor/billing.json"
out="$(run "$BASE" billing)"
eq  'a reconciliation with no timestamp is a warning' "$(sev_of "$out" billing)" WARN
has 'and it says the age cannot be judged'    "$out" 'age cannot be judged'

reset_state
billing 'the day before yesterday'
out="$(run "$BASE" billing)"
eq  'an unreadable timestamp is a warning rather than a guess' \
    "$(sev_of "$out" billing)" WARN

reset_state
printf 'this is not json\n' >"$STATE/monitor/billing.json"
out="$(run "$BASE" billing)"
eq  'a corrupt file is a warning and not a crash' "$(sev_of "$out" billing)" WARN

# ------------------------------------------------------- 16. exit status
#
# THE EXIT CODE SAYS WHETHER THE MONITOR RAN, NOT WHAT IT FOUND. It used to say
# both: a WARN or a CRIT exited 1, so a Type=oneshot unit driven by a
# five-minute timer sat in `failed` almost permanently while the monitor was
# doing its job. `systemctl is-failed multiverse-monitor.service` stopped
# meaning anything, and a deploy pipeline under `set -o pipefail` broke on it.
# Severity travels in the alert and in the `worst:` line; these cases hold the
# exit code to the one thing it still has to say.

reset_state
printf '127.0.0.1 localhost\n127.0.0.1 multiverse.example\n' >"$HOSTS"
out="$(run "$BASE" hosts-pin)"; rc=$?
eq  'an OK pass exits 0'                             "$rc" 0
eq  'and the verdict is still OK'                    "$(sev_of "$out" hosts-pin)" OK

# A WARN. Three consecutive passes with half the swap in use is the sizing
# signal from section 14, reached here for its exit code.
reset_state
meminfo 1953544 2097152 1048576
run "$BASE" swap >/dev/null
run "$(( BASE + 300 ))" swap >/dev/null
out="$(run "$(( BASE + 600 ))" swap)"; rc=$?
eq  'a WARN pass exits 0 as well'                    "$rc" 0
eq  'and the WARN is still reported'                 "$(sev_of "$out" swap)" WARN
has 'and the worst line still carries it'            "$out" 'worst: WARN'

# A CRIT. The missing loopback pin from section 12, which is the most expensive
# verdict this script can reach without a network.
reset_state
printf '127.0.0.1 localhost\n' >"$HOSTS"
out="$(run "$BASE" hosts-pin)"; rc=$?
eq  'a CRIT pass exits 0 too'                        "$rc" 0
eq  'and the CRIT is still reported'                 "$(sev_of "$out" hosts-pin)" CRIT
has 'and the worst line still carries it'            "$out" 'worst: CRIT'

# CANNOT RUN IS STILL NON-ZERO, and it is the only thing left that is. An
# environment file the monitor cannot read means no thresholds, no alert
# channel and no pass at all, and nothing else on the host would notice.
missing="$TMP/there-is-no-deploy-env"
rm -f "$missing"
err="$(env MV_ENV_FILE="$missing" MV_STATE="$STATE" "$MON" --only swap --quiet 2>&1)"; rc=$?
eq  'a missing environment file exits 2'             "$rc" 2
has 'and it names the file it could not find'        "$err" "$missing"

# The same code for an argument it does not accept, because both are "this pass
# never started".
err="$(env MV_ENV_FILE="$ENVF" MV_STATE="$STATE" "$MON" --only nonesuch --quiet 2>&1)"; rc=$?
eq  'an unknown --only group exits 2'                "$rc" 2
err="$(env MV_ENV_FILE="$ENVF" MV_STATE="$STATE" "$MON" --nonesuch 2>&1)"; rc=$?
eq  'an unknown argument exits 2'                    "$rc" 2

# ---------------------------------------------------------------- result

printf '\n'
if [ "$FAIL" -gt 0 ]; then
  printf 'monitor arithmetic checks: FAIL (%d passed, %d failed)\n' "$PASS" "$FAIL" >&2
  exit 1
fi
printf 'monitor arithmetic checks: PASS (%d assertions)\n' "$PASS"
