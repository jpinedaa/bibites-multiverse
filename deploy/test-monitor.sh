#!/usr/bin/env bash
# Exercise monitor.sh's money and memory arithmetic on a workstation.
#
# THE ARITHMETIC IS THE RISK, not the plumbing. A transfer check that alerts
# every five minutes gets muted, and a transfer check that never alerts is
# decoration; both failures are silent and both are arithmetic. The archive's
# memory gate has the same shape and one worse failure of its own: it once
# divided by RAM plus swap, so a swap file could clear a critical verdict
# without changing one byte of retained state. So this drives the real
# monitor.sh through the read seams it exposes — MV_PROC_NET_DEV,
# MV_PROC_NET_ROUTE, MV_HOSTS_FILE, MV_PROC_MEMINFO, MV_STATUS_JSON, MV_NOW,
# MV_STATE, MV_ENV_FILE — against fake counters, a fake /proc/meminfo, a fake
# status document and a fake clock.
#
# It needs no root, no network, no systemd and no host: --only transfer,
# --only hosts-pin, --only replay and --only swap skip every check that would
# dial something. Nothing here touches /var, /etc or a real interface.
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
has 'the warning carries the same measured levers' "$out" '88 GB/month for each crossing per second'

seed_warm "$GIB" "$(( BASE - 86400 ))" 4000 4000
out="$(run "$BASE" transfer $FLAT)"
eq  'the trailing rate is critical at a hundred percent' "$(sev_of "$out" transfer)" CRIT
has 'the critical line says both directions are counted' "$out" 'counted in BOTH directions'
has 'the critical line points at the levers'             "$out" 'deploy/SIZING.md'
# THE LEVERS ARE THE POINT OF THE ALERT. An operator who reads this line at 03:00
# acts on the number in it, so the numbers are pinned to SIZING.md's measured
# model here: peer traffic is a crossing-rate term and NOT a time-scale term, and
# the retired "50 GB per unit of S" rule must never come back.
has 'the critical line gives the measured peer lever'    "$out" '88 GB/month for each crossing per second'
has 'the critical line says time scale is not a lever'   "$out" 'does NOT fall with time scale'
has 'the critical line gives the measured viewer lever'  "$out" '1,150 GB/month'
has 'the critical line gives the ingest lever'           "$out" '780 GB/month'
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

# ---------------------------------------------------------------- result

printf '\n'
if [ "$FAIL" -gt 0 ]; then
  printf 'monitor arithmetic checks: FAIL (%d passed, %d failed)\n' "$PASS" "$FAIL" >&2
  exit 1
fi
printf 'monitor arithmetic checks: PASS (%d assertions)\n' "$PASS"
