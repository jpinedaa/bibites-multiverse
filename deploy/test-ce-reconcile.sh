#!/usr/bin/env bash
# Exercise deploy/ce-reconcile.sh's arithmetic with no AWS account and no host.
#
# THE PARSER IS THE RISK, and it cannot be exercised against the live API: each
# Cost Explorer call costs $0.01 and the answer changes every day, so a test
# that called it would be both expensive and non-repeatable. So this drives the
# real script through the two seams it exposes:
#
#   --from-file            a saved Cost Explorer response under deploy/testdata.
#                          NO Cost Explorer call is made.
#   MV_BILLING_METRIC_CMD  a fake instance-metric provider. NO Lightsail call is
#                          made, and it returns its buckets OUT OF ORDER and in
#                          two timestamp spellings, because the real API returns
#                          them unsorted and the CLI renders them in local time.
#   MV_BILLING_NOW         a fixed clock, so the month window is the fixture's.
#
# The fixture is three settled days at 60 GB in and 40 GB out, followed by two
# days the fourteen-hour lag has not reached. That shape is the point: a reader
# that divides month-to-date by the days of the MONTH rather than the days COST
# EXPLORER HAS reports a service using two thirds of what it uses.
#
#   deploy/test-ce-reconcile.sh          run every case
#   deploy/test-ce-reconcile.sh -v       also print each case as it passes
set -uo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CE="$HERE/ce-reconcile.sh"
DATA="$HERE/testdata"
[ -x "$CE" ] || { echo "test-ce-reconcile: $CE is not executable" >&2; exit 2; }

SHOW=0
[ "${1:-}" = "-v" ] && SHOW=1

TMP="$(mktemp -d "${TMPDIR:-/tmp}/mvcetest.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT

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

# ---------------------------------------------------------------- the fakes

# A metric provider that answers the way the real one does and no better: the
# buckets are DELIBERATELY out of order, one carries a Z and the rest carry an
# explicit offset, and one bucket falls outside the days Cost Explorer has so
# that the month-to-date sum and the ratio window are provably different sums.
#
# 1.01 is the measured, systematic residual between the instance metric and the
# invoice. The whole reason the ratio is published rather than corrected is that
# nobody can explain it, so the day it moves has to be visible.
cat >"$TMP/metric" <<'FAKE'
#!/usr/bin/env bash
gib() { python3 -c "print('%.1f' % ($1 * 1073741824))"; }
case "$1" in
  NetworkIn)  settled=60.6; late=20.0 ;;
  NetworkOut) settled=40.4; late=10.0 ;;
  *) echo "fake metric: unknown metric $1" >&2; exit 1 ;;
esac
cat <<JSON
[
  {"sum": $(gib "$late"),    "timestamp": "2026-08-04T05:00:00+00:00", "unit": "Bytes"},
  {"sum": $(gib "$settled"), "timestamp": "2026-08-02T00:00:00+00:00", "unit": "Bytes"},
  {"sum": $(gib "$settled"), "timestamp": "2026-08-01T00:00:00Z",      "unit": "Bytes"},
  {"sum": $(gib "$settled"), "timestamp": "2026-08-03T00:00:00+00:00", "unit": "Bytes"}
]
JSON
FAKE
chmod +x "$TMP/metric"

NOW="$(date -u -d '2026-08-05T06:30:00Z' +%s)"

run() { # run <ce-response> [KEY=VALUE ...]
  local response="$1"
  shift
  env MV_BILLING_NOW="$NOW" \
      MV_BILLING_METRIC_CMD="$TMP/metric" \
      MV_BILLING_HOST= \
      MV_TRANSFER_ALLOWANCE_GB=3072 \
      MV_LIGHTSAIL_INSTANCE= \
      "$@" \
      "$CE" --from-file "$response" --no-ship 2>"$TMP/err"
}

field() { # field <json> <key>
  printf '%s' "$1" | python3 -c '
import json, sys
record = json.load(sys.stdin)
value = record[sys.argv[1]]
if isinstance(value, bool):
    print("true" if value else "false")
elif value is None:
    print("null")
else:
    print(value)
' "$2"
}

# ---------------------------------------------- 1. the ordinary daily reading

out="$(run "$DATA/ce-response.json")"
rc=$?
eq 'a saved response reconciles without touching AWS' "$rc" 0

eq 'the month comes from the clock, not the file'   "$(field "$out" month)" 2026-08
eq 'the through-date is the last day WITH DATA, not the last day requested' \
   "$(field "$out" ceThroughDate)" 2026-08-03
eq 'and the two days the lag has not reached are not settled days' \
   "$(field "$out" ceSettledDays)" 3
eq 'inbound quantity is summed across the settled days'  "$(field "$out" ceInGiB)"  180.0
eq 'outbound quantity is summed across the settled days' "$(field "$out" ceOutGiB)" 120.0
eq 'the bundle-hours usage type is not transfer and is ignored' \
   "$(field "$out" ceOverageGiB)" 0.0
eq 'no overage means no overage cost'                    "$(field "$out" ceOverageUsd)" 0.0
eq 'the current month is flagged estimated'              "$(field "$out" ceEstimated)" true

# The metric sums are MONTH TO DATE and therefore include the bucket that falls
# after the last settled day. The ratio sums do not. Proving these are different
# numbers is the point of the fixture.
eq 'the metric sum is month to date, so it includes the unsettled bucket' \
   "$(field "$out" metricInGiB)"  201.8
eq 'and the same for outbound'                       "$(field "$out" metricOutGiB)" 131.2
eq 'the inbound ratio compares the SAME DAYS, so it is 1.01 and not 1.12' \
   "$(field "$out" ratioIn)"  1.01
eq 'and the outbound ratio likewise'                 "$(field "$out" ratioOut)" 1.01

# 300 GB over three settled days is 100 GB/day, and August has 31 of them.
eq 'the projection runs the settled rate out over the whole month' \
   "$(field "$out" projectedMonthGiB)" 3100.0
eq 'and projects outbound separately, because only outbound is billed' \
   "$(field "$out" projectedOutGiB)" 1240.0
eq 'the allowance is carried in the file so its reader needs nothing else' \
   "$(field "$out" allowanceGiB)" 3072.0
# min(1240, max(0, 3100 - 3072)) = 28 GB at $0.09.
eq 'the billed projection applies the vendor rule, not the combined excess' \
   "$(field "$out" projectedOverageUsd)" 2.52
eq 'a run from a file spends nothing'                "$(field "$out" ceCallsThisRun)" 0
has 'the file records the principal and the acceptance' \
    "$(field "$out" principal)" 'root (read-only; owner-accepted 2026-08-16)'
has 'and the timestamp is UTC with a Z'              "$(field "$out" asOf)" 'Z'

# ---------------------------------------------------------------- 2. overage

out="$(run "$DATA/ce-response-overage.json")"
eq 'a billed overage line is read as a quantity' "$(field "$out" ceOverageGiB)" 12.5
eq 'and its unblended cost is read beside it'    "$(field "$out" ceOverageUsd)" 1.125
eq 'the transfer totals are unaffected by it'    "$(field "$out" ceInGiB)" 180.0

# ------------------------------------------------------------- 3. --print

printed="$(env MV_BILLING_NOW="$NOW" MV_BILLING_METRIC_CMD="$TMP/metric" \
  MV_BILLING_HOST= MV_LIGHTSAIL_INSTANCE= \
  "$CE" --from-file "$DATA/ce-response.json" --no-ship --print 2>&1)"
has 'the human reading names the through-date'   "$printed" 'Cost Explorer through 2026-08-03'
has 'and states the rule it applied'             "$printed" 'billed = min(out, max(0, (in + out) - allowance))'
has 'and says the residual is uncorrected'       "$printed" 'systematic and uncorrected residual'
has 'and states what the run cost'               "$printed" 'Cost Explorer calls this run: 0'

# ---------------------------------------------------------------- 4. refusals

# A SECOND PAGE IS A SECOND $0.01. The script refuses rather than paying it.
python3 - "$DATA/ce-response.json" "$TMP/paged.json" <<'PY'
import json, sys
record = json.load(open(sys.argv[1], encoding="utf-8"))
record["NextPageToken"] = "AQAB-not-a-real-token"
json.dump(record, open(sys.argv[2], "w", encoding="utf-8"), indent=2)
PY
run "$TMP/paged.json" >/dev/null
eq  'a paginated answer is a failure, not a partial reading' "$?" 1
has 'and it says why it will not fetch the second page' \
    "$(cat "$TMP/err")" 'will not fetch a'

# THE UNIT IS THE FOUNDATION. 2^30 against 10^9 is 7.4% — smaller than nothing
# else here would notice and larger than the margin the allowance leaves.
python3 - "$DATA/ce-response.json" "$TMP/bytes.json" <<'PY'
import json, sys
record = json.load(open(sys.argv[1], encoding="utf-8"))
for period in record["ResultsByTime"]:
    for group in period["Groups"]:
        if group["Keys"][0].endswith("Bytes"):
            group["Metrics"]["UsageQuantity"]["Unit"] = "Bytes"
json.dump(record, open(sys.argv[2], "w", encoding="utf-8"), indent=2)
PY
run "$TMP/bytes.json" >/dev/null
eq  'a quantity quoted in another unit is refused' "$?" 1
has 'and it names the base every figure assumes' \
    "$(cat "$TMP/err")" "2^30"

# A response with no transfer at all is a reading of "nothing yet", not zero.
python3 - "$TMP/empty.json" <<'PY'
import json, sys
json.dump({
    "GroupDefinitions": [{"Type": "DIMENSION", "Key": "USAGE_TYPE"}],
    "ResultsByTime": [{
        "TimePeriod": {"Start": "2026-08-01", "End": "2026-08-02"},
        "Total": {}, "Groups": [], "Estimated": True,
    }],
    "DimensionValueAttributes": [],
}, open(sys.argv[1], "w", encoding="utf-8"), indent=2)
PY
out="$(run "$TMP/empty.json")"
eq 'no transfer data yet gives a null through-date, never a zero' \
   "$(field "$out" ceThroughDate)" null
eq 'and an unknown ratio, which is not 1.0'    "$(field "$out" ratioIn)" null
has 'and it says the lag is why'               "$(cat "$TMP/err")" 'lags about fourteen hours'

# Without an instance name there is nothing to compare the invoice against, and
# guessing one would query somebody else's resource.
env MV_BILLING_NOW="$NOW" MV_BILLING_HOST= MV_LIGHTSAIL_INSTANCE= \
  "$CE" --from-file "$DATA/ce-response.json" --no-ship >/dev/null 2>"$TMP/err"
eq  'no instance name is a failure rather than a silent half-reading' "$?" 1
has 'and it says which knob names it' "$(cat "$TMP/err")" 'MV_LIGHTSAIL_INSTANCE'

# ---------------------------------------------------------------- result

printf '\n'
if [ "$FAIL" -gt 0 ]; then
  printf 'Cost Explorer reconciliation checks: FAIL (%d passed, %d failed)\n' "$PASS" "$FAIL" >&2
  exit 1
fi
printf 'Cost Explorer reconciliation checks: PASS (%d assertions)\n' "$PASS"
