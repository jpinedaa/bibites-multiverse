#!/usr/bin/env bash
# Reconcile the host's own NIC counter against the invoice, once a day.
#
# THE MONITOR ON THE HOST WATCHES A PROXY. deploy/monitor.sh reads
# /proc/net/dev, which needs no credential and cannot be the reason an access
# key sits on a public-facing box. That is the right instrument for alerting and
# it is not the bill. This script is the other half: it asks the provider what
# it is actually metering, compares the two, and leaves the answer where the
# monitor can find it.
#
# IT RUNS ANYWHERE EXCEPT THE HOST, on purpose. It needs a cloud credential, so
# it runs on an operator machine — a workstation, a laptop, a scheduler — and
# ships one small JSON file to the host over ssh. The host still holds no
# credential. If this script stops running the monitor says so within
# MV_BILLING_STALE_HOURS, which is the whole reason the file carries a
# timestamp.
#
#   ce-reconcile.sh                 one pass: read, compute, ship
#   ce-reconcile.sh --print         also print the reading for a person
#   ce-reconcile.sh --dry-run       read and compute, print the file and the
#                                   exact ssh command, ship NOTHING
#   ce-reconcile.sh --no-ship       read and compute, do not ship and do not
#                                   pretend to. This is the scheduled shape on
#                                   a machine that cannot reach the host.
#   ce-reconcile.sh --out PATH      also write the JSON to a local file
#   ce-reconcile.sh --from-file F   parse a saved Cost Explorer response instead
#                                   of calling the API. Makes NO CE call.
#   ce-reconcile.sh --save-ce PATH  keep the raw Cost Explorer response. One
#                                   saved response is how a fixture gets made,
#                                   and it costs the same $0.01 as the call.
#
# COST, STATED PLAINLY BECAUSE IT IS THE REASON FOR THE CADENCE:
#
#   Cost Explorer          $0.01 per paginated request. EXACTLY ONE call per
#                          run, --no-paginate. If the answer comes back with a
#                          NextPageToken this script logs it and STOPS rather
#                          than paying for a second page: a Lightsail-only
#                          usage-type query for one month does not page, so a
#                          token means the question changed and a person should
#                          look. Daily is $0.31/month.
#   Lightsail metrics      free. Two calls, NetworkIn and NetworkOut.
#
# WHY THE TWO NUMBERS DO NOT MATCH, AND WHY THAT IS NOT A BUG. The instance
# metric sums about 1.01x the Cost Explorer quantity in the same window. It is
# systematic and it is NOT corrected here: correcting a residual you cannot
# explain hides the day it changes. The ratio is published instead, and the
# monitor warns when it leaves a band. GB IS 2^30 BYTES throughout, which is the
# base the allowance is quoted in and the base that makes this ratio 1.01
# instead of 1.08.
#
# THE BILLING RULE, from https://aws.amazon.com/lightsail/pricing/ (verified
# 2026-08-16): both directions consume the allowance, and only OUTBOUND beyond
# the allowance is billed.
#
#     billed_overage_GB = min( out , max( 0 , (in + out) - allowance ) )
#
# Inbound is never billed and inbound is not free: while any overage is billed,
# a GB of allowance is worth the overage rate whichever direction spends it.
#
# COST EXPLORER LAGS about fourteen hours, so the current month is always
# partly missing and is flagged Estimated. Every ratio below is therefore
# computed over THE DAYS COST EXPLORER HAS, never month-to-date against
# month-to-date — comparing a complete counter with an incomplete invoice
# manufactures a discrepancy that is only the lag.
#
# THE PRINCIPAL. The configured profile authenticates as the account root
# principal. Every call here is read-only. The owner accepted that on
# 2026-08-16 for metrics and billing reads and declined a scoped IAM user; the
# written file records the acceptance so the reader of the file knows.
set -uo pipefail

# ---------------------------------------------------------------- parameters

: "${MV_AWS_PROFILE:=bibites-multiverse}"
: "${MV_AWS_REGION:=us-east-1}"
# No default: naming a live resource is not the public kit's business.
: "${MV_LIGHTSAIL_INSTANCE:=}"
# The provider's GB, 2^30 bytes, counted in BOTH directions.
: "${MV_TRANSFER_ALLOWANCE_GB:=3072}"
# US East (N. Virginia) overage rate per GB beyond the allowance, outbound only.
: "${MV_TRANSFER_OVERAGE_USD_PER_GB:=0.09}"
# The ssh target that receives the file. Empty means "compute but do not ship".
: "${MV_BILLING_HOST:=}"
: "${MV_BILLING_REMOTE_PATH:=/var/lib/multiverse/monitor/billing.json}"
# The service account the monitor runs as, and which must be able to read it.
: "${MV_BILLING_FILE_OWNER:=multiverse}"
: "${MV_BILLING_FILE_GROUP:=multiverse}"

# READ SEAMS, so deploy/test-ce-reconcile.sh can drive the whole computation
# with no AWS account and no network. Each one defaults to the real thing.
#
#   MV_BILLING_NOW         one clock reading, taken once. Everything below
#                          derives its dates from it and nothing asks twice.
#   MV_BILLING_METRIC_CMD  a program called as
#                              CMD <NetworkIn|NetworkOut> <startZ> <endZ>
#                          that prints the same JSON array the Lightsail call
#                          prints. Set, it replaces both free API calls.
: "${MV_BILLING_NOW:=}"
: "${MV_BILLING_METRIC_CMD:=}"

FROM_FILE=""
SAVE_CE=""
OUT_FILE=""
PRINT=0
DRY_RUN=0
NO_SHIP=0

die() { printf 'ce-reconcile: %s\n' "$*" >&2; exit 1; }

while [ $# -gt 0 ]; do
  case "$1" in
    --print) PRINT=1 ;;
    --dry-run) DRY_RUN=1 ;;
    --no-ship) NO_SHIP=1 ;;
    --from-file) FROM_FILE="${2:?--from-file needs a saved Cost Explorer response}"; shift ;;
    --save-ce) SAVE_CE="${2:?--save-ce needs a path}"; shift ;;
    --out) OUT_FILE="${2:?--out needs a path}"; shift ;;
    -h|--help) sed -n '2,69p' "$0"; exit 0 ;;
    *) die "unknown argument $1" ;;
  esac
  shift
done

for c in python3 date; do
  command -v "$c" >/dev/null 2>&1 || die "$c is not installed"
done

NOW="${MV_BILLING_NOW:-$(date -u +%s)}"
case "$NOW" in ''|*[!0-9]*) die "MV_BILLING_NOW must be whole seconds since the epoch, not '$NOW'" ;; esac

MONTH="$(date -u -d "@$NOW" +%Y-%m)"
MONTH_START_DATE="$(date -u -d "@$NOW" +%Y-%m-01)"
MONTH_START_EPOCH="$(date -u -d "$MONTH_START_DATE" +%s)"
NEXT_MONTH_DATE="$(date -u -d "$MONTH_START_DATE +1 month" +%Y-%m-%d)"
DAYS_IN_MONTH=$(( ( $(date -u -d "$NEXT_MONTH_DATE" +%s) - MONTH_START_EPOCH ) / 86400 ))
TODAY="$(date -u -d "@$NOW" +%Y-%m-%d)"
# Cost Explorer's End is EXCLUSIVE, so this asks for whole days only. On the
# first of a month that would be an empty range, which the API rejects, so ask
# for the one partial day instead — it comes back Estimated and the settled-day
# arithmetic below simply has one day in it.
CE_END="$TODAY"
[ "$CE_END" != "$MONTH_START_DATE" ] || CE_END="$(date -u -d "$MONTH_START_DATE +1 day" +%Y-%m-%d)"
NOW_Z="$(date -u -d "@$NOW" +%Y-%m-%dT%H:%M:%SZ)"
MONTH_START_Z="${MONTH_START_DATE}T00:00:00Z"

WORK="$(mktemp -d "${TMPDIR:-/tmp}/mvce.XXXXXX")" || die "cannot create a work directory"
trap 'rm -rf "$WORK"' EXIT

# ---------------------------------------------------------------- Cost Explorer
#
# ONE CALL. Grouped by USAGE_TYPE because the transfer quantity is what is
# wanted and the cost of it is usually zero — the billed quantity is readable
# long before any overage is charged, which is what makes this a leading
# indicator rather than a bill.

CE_JSON="$WORK/ce.json"
CE_CALLS=0
if [ -n "$FROM_FILE" ]; then
  [ -r "$FROM_FILE" ] || die "cannot read $FROM_FILE"
  cp -- "$FROM_FILE" "$CE_JSON" || die "cannot copy $FROM_FILE"
else
  command -v aws >/dev/null 2>&1 || die "the AWS CLI is not installed"
  if ! aws --profile "$MV_AWS_PROFILE" --region "$MV_AWS_REGION" \
        ce get-cost-and-usage \
        --time-period "Start=$MONTH_START_DATE,End=$CE_END" \
        --granularity DAILY \
        --metrics UsageQuantity UnblendedCost \
        --group-by Type=DIMENSION,Key=USAGE_TYPE \
        --filter '{"Dimensions":{"Key":"SERVICE","Values":["Amazon Lightsail"],"MatchOptions":["EQUALS"]}}' \
        --no-paginate \
        --output json >"$CE_JSON" 2>"$WORK/ce.err"; then
    printf 'ce-reconcile: the Cost Explorer call failed:\n' >&2
    sed 's/^/  /' "$WORK/ce.err" >&2
    exit 1
  fi
  CE_CALLS=1
  [ -z "$SAVE_CE" ] || cp -- "$CE_JSON" "$SAVE_CE" || die "cannot write $SAVE_CE"
fi

# ---------------------------------------------------------------- the metric
#
# FREE, and the reason this is a reconciliation rather than a report. Two calls,
# hourly buckets, month to date.
#
# THE API RETURNS THE BUCKETS UNSORTED and the CLI renders each timestamp in the
# LOCAL zone. Both are traps. Sorting is never assumed below, TZ=UTC is forced
# so the rendered offset is +00:00, and the offset is parsed anyway rather than
# trusted — a sum that silently drops or double-counts an hour is exactly the
# kind of quiet arithmetic error this whole file exists to catch.

metric() { # metric <NetworkIn|NetworkOut> <outfile>
  if [ -n "$MV_BILLING_METRIC_CMD" ]; then
    # shellcheck disable=SC2086
    $MV_BILLING_METRIC_CMD "$1" "$MONTH_START_Z" "$NOW_Z" >"$2" 2>"$WORK/metric.err"
  else
    TZ=UTC aws --profile "$MV_AWS_PROFILE" --region "$MV_AWS_REGION" \
      lightsail get-instance-metric-data \
      --instance-name "$MV_LIGHTSAIL_INSTANCE" \
      --metric-name "$1" \
      --period 3600 \
      --unit Bytes \
      --statistics Sum \
      --start-time "$MONTH_START_Z" \
      --end-time "$NOW_Z" \
      --query 'metricData[]' \
      --output json >"$2" 2>"$WORK/metric.err"
  fi
}

if [ -z "$MV_BILLING_METRIC_CMD" ] && [ -z "$MV_LIGHTSAIL_INSTANCE" ]; then
  die "MV_LIGHTSAIL_INSTANCE names the instance whose metric is compared against the invoice, and it is empty"
fi
for m in NetworkIn NetworkOut; do
  if ! metric "$m" "$WORK/$m.json"; then
    printf 'ce-reconcile: the Lightsail %s call failed:\n' "$m" >&2
    sed 's/^/  /' "$WORK/metric.err" >&2
    exit 1
  fi
done

# ---------------------------------------------------------------- arithmetic

RESULT="$WORK/billing.json"
python3 - \
  "$CE_JSON" "$WORK/NetworkIn.json" "$WORK/NetworkOut.json" "$RESULT" \
  "$MONTH" "$NOW_Z" "$MV_TRANSFER_ALLOWANCE_GB" "$MV_TRANSFER_OVERAGE_USD_PER_GB" \
  "$DAYS_IN_MONTH" "$MONTH_START_Z" "$CE_CALLS" <<'PY'
import datetime as dt
import json
import sys

(ce_path, in_path, out_path, result_path, month, as_of, allowance_s,
 rate_s, days_in_month_s, month_start_s, ce_calls_s) = sys.argv[1:12]

GIB = float(1 << 30)
allowance = float(allowance_s)
rate = float(rate_s)
days_in_month = int(days_in_month_s)


def fail(message):
    sys.stderr.write("ce-reconcile: %s\n" % message)
    raise SystemExit(1)


def load(path):
    try:
        with open(path, "r", encoding="utf-8") as handle:
            return json.load(handle)
    except Exception as error:                      # noqa: BLE001
        fail("cannot parse %s: %s" % (path, error))


ce = load(ce_path)

# PAGING IS A STOP, NOT A LOOP. A second page is a second $0.01, and a query
# this narrow does not page — so a token means the question is no longer the
# one this script was priced for, and a person should read it.
token = ce.get("NextPageToken")
if token:
    fail("the Cost Explorer answer carries a NextPageToken, so it is incomplete. "
         "This script pays for exactly one page per run and will not fetch a "
         "second. Narrow the query or accept the extra $0.01 deliberately.")

# The usage types the provider meters this instance's transfer under. Matched by
# suffix so that another Region's prefix (USE1-, USW2-, ...) needs no change.
KIND = (
    ("overage", "DataXfer-Out-Overage-Bytes"),
    ("in", "TotalDataXfer-In-Bytes"),
    ("out", "TotalDataXfer-Out-Bytes"),
)


def classify(usage_type):
    for name, suffix in KIND:
        if usage_type.endswith(suffix):
            return name
    return None


totals = {"in": 0.0, "out": 0.0, "overage": 0.0}
overage_usd = 0.0
estimated = False
days_with_data = []
seen_units = set()

results = ce.get("ResultsByTime")
if not isinstance(results, list):
    fail("the Cost Explorer answer has no ResultsByTime array")

for period in results:
    day = period.get("TimePeriod", {}).get("Start")
    if period.get("Estimated"):
        estimated = True
    day_total = 0.0
    for group in period.get("Groups", []) or []:
        keys = group.get("Keys") or []
        if not keys:
            continue
        kind = classify(keys[0])
        if kind is None:
            continue
        metrics = group.get("Metrics", {})
        quantity = metrics.get("UsageQuantity", {})
        amount = float(quantity.get("Amount", 0.0) or 0.0)
        seen_units.add(quantity.get("Unit"))
        totals[kind] += amount
        day_total += amount
        if kind == "overage":
            overage_usd += float(
                metrics.get("UnblendedCost", {}).get("Amount", 0.0) or 0.0)
    if day_total > 0.0:
        days_with_data.append(day)

# THE UNIT IS THE WHOLE FOUNDATION. Every figure here is the provider's GB of
# 2^30 bytes and the 1.01 ratio is the evidence for that reading. A response
# that started quoting something else would flow through this file silently and
# be wrong by 7.4 percent, which is smaller than the band the monitor watches
# and larger than the margin the allowance leaves.
unexpected = sorted(u for u in seen_units if u not in (None, "GB"))
if unexpected:
    fail("Cost Explorer quoted the transfer quantity in %s, not GB. Every figure "
         "in this file assumes the provider's GB of 2^30 bytes; re-derive the "
         "arithmetic before trusting it." % ", ".join(unexpected))

days_with_data.sort()
ce_through = days_with_data[-1] if days_with_data else None
settled_days = len(days_with_data)

# ---------------------------------------------------------------- the metric
#
# Sum by VALUE, inside an explicit UTC window. Nothing here depends on the
# order the API returned the buckets in.

def parse_stamp(text):
    if text.endswith("Z"):
        text = text[:-1] + "+00:00"
    try:
        moment = dt.datetime.fromisoformat(text)
    except ValueError:
        fail("cannot read a metric timestamp: %r" % text)
    if moment.tzinfo is None:
        moment = moment.replace(tzinfo=dt.timezone.utc)
    return moment.astimezone(dt.timezone.utc)


month_start = parse_stamp(month_start_s)
if ce_through:
    settled_end = (dt.datetime.strptime(ce_through, "%Y-%m-%d")
                   .replace(tzinfo=dt.timezone.utc) + dt.timedelta(days=1))
    settled_start = (dt.datetime.strptime(days_with_data[0], "%Y-%m-%d")
                     .replace(tzinfo=dt.timezone.utc))
else:
    settled_end = month_start
    settled_start = month_start


def sum_metric(path):
    """Return (month-to-date bytes, bytes inside the days Cost Explorer has)."""
    data = load(path)
    if not isinstance(data, list):
        fail("the metric answer in %s is not an array" % path)
    mtd = 0.0
    settled = 0.0
    for point in data:
        value = point.get("sum")
        if value is None:
            continue
        value = float(value)
        stamp = parse_stamp(point["timestamp"])
        if stamp >= month_start:
            mtd += value
        if settled_start <= stamp < settled_end:
            settled += value
    return mtd, settled


metric_in_mtd, metric_in_settled = sum_metric(in_path)
metric_out_mtd, metric_out_settled = sum_metric(out_path)


def ratio(metric_bytes, ce_gb):
    """metric over invoice, on the same days. Unknown is not 1.0 and not 0."""
    if ce_gb <= 0.0:
        return None
    return round((metric_bytes / GIB) / ce_gb, 4)


ce_in = totals["in"]
ce_out = totals["out"]

# THE PROJECTION. The settled days give a rate; a whole month at that rate is
# what the allowance is compared against. Days the instance did not exist are
# not in the denominator, so a mid-month launch projects its running rate rather
# than a diluted one.
if settled_days > 0:
    projected_in = ce_in / settled_days * days_in_month
    projected_out = ce_out / settled_days * days_in_month
else:
    projected_in = projected_out = 0.0
projected_month = projected_in + projected_out

# billed_overage = min( out , max( 0 , (in + out) - allowance ) )
projected_overage = min(projected_out, max(0.0, projected_month - allowance))
projected_overage_usd = round(projected_overage * rate, 2)

record = {
    "asOf": as_of,
    "month": month,
    "ceThroughDate": ce_through,
    "ceInGiB": round(ce_in, 3),
    "ceOutGiB": round(ce_out, 3),
    "ceOverageGiB": round(totals["overage"], 3),
    "ceOverageUsd": round(overage_usd, 4),
    "metricInGiB": round(metric_in_mtd / GIB, 3),
    "metricOutGiB": round(metric_out_mtd / GIB, 3),
    "ratioIn": ratio(metric_in_settled, ce_in),
    "ratioOut": ratio(metric_out_settled, ce_out),
    "projectedMonthGiB": round(projected_month, 1),
    "projectedOverageUsd": projected_overage_usd,
    "allowanceGiB": round(allowance, 1),
    "principal": "root (read-only; owner-accepted 2026-08-16)",
    "ceCallsThisRun": int(ce_calls_s),
    # Beyond the contract above, and each one is here because reading the file
    # without it invites a wrong conclusion.
    "ceEstimated": estimated,
    "ceSettledDays": settled_days,
    "daysInMonth": days_in_month,
    "projectedOutGiB": round(projected_out, 1),
    "overageUsdPerGiB": rate,
}

with open(result_path, "w", encoding="utf-8") as handle:
    json.dump(record, handle, indent=2, sort_keys=False)
    handle.write("\n")

if ce_through is None:
    sys.stderr.write(
        "ce-reconcile: Cost Explorer returned no transfer usage for %s yet. "
        "It lags about fourteen hours, so this is normal early in a month and "
        "is a fault later in one.\n" % month)
PY
[ -s "$RESULT" ] || die "the reconciliation produced no file"

# ---------------------------------------------------------------- output

if [ "$PRINT" = 1 ] || [ "$DRY_RUN" = 1 ]; then
  python3 - "$RESULT" "$CE_CALLS" <<'PY'
import json
import sys

record = json.load(open(sys.argv[1], encoding="utf-8"))


def gb(value):
    return "unknown" if value is None else "%.1f GB" % value


def ratio(value):
    return "unknown" if value is None else "%.2fx" % value


through = record["ceThroughDate"] or "nothing yet"
pct = 0.0
if record["allowanceGiB"]:
    pct = 100.0 * record["projectedMonthGiB"] / record["allowanceGiB"]

print("Cost Explorer through %s (%d settled day(s) of %s, estimated=%s)"
      % (through, record["ceSettledDays"], record["month"],
         "yes" if record["ceEstimated"] else "no"))
print("  invoice        in %s, out %s, overage %s ($%.2f)"
      % (gb(record["ceInGiB"]), gb(record["ceOutGiB"]),
         gb(record["ceOverageGiB"]), record["ceOverageUsd"]))
print("  NIC metric     in %s, out %s (month to date)"
      % (gb(record["metricInGiB"]), gb(record["metricOutGiB"])))
print("  metric/invoice in %s, out %s  (same days; ~1.01x is the known, "
      "systematic and uncorrected residual)"
      % (ratio(record["ratioIn"]), ratio(record["ratioOut"])))
print("  projection     %s of a %s allowance (%.0f%%), billed overage $%.2f"
      % (gb(record["projectedMonthGiB"]), gb(record["allowanceGiB"]), pct,
         record["projectedOverageUsd"]))
print("  rule           billed = min(out, max(0, (in + out) - allowance)) "
      "at $%.2f/GB" % record["overageUsdPerGiB"])
print("  principal      %s" % record["principal"])
print("  Cost Explorer calls this run: %s ($%.2f)"
      % (sys.argv[2], 0.01 * int(sys.argv[2])))
PY
fi

if [ -n "$OUT_FILE" ]; then
  cp -- "$RESULT" "$OUT_FILE" || die "cannot write $OUT_FILE"
fi

# ---------------------------------------------------------------- ship
#
# ONE COMMAND, and the rename is what makes it atomic: the monitor reads this
# file every five minutes and must never see half of it. `install` sets the
# owner, group and mode in the same step that creates the temporary file, so
# there is no window in which the file exists and the monitor cannot read it.

SHIP_CMD="sudo install -o $MV_BILLING_FILE_OWNER -g $MV_BILLING_FILE_GROUP -m 0644 /dev/stdin '$MV_BILLING_REMOTE_PATH.new' && sudo mv -f '$MV_BILLING_REMOTE_PATH.new' '$MV_BILLING_REMOTE_PATH'"

if [ "$NO_SHIP" = 1 ] || [ -z "$MV_BILLING_HOST" ]; then
  [ "$PRINT" = 1 ] || [ "$DRY_RUN" = 1 ] || cat "$RESULT"
  if [ -z "$MV_BILLING_HOST" ] && [ "$NO_SHIP" = 0 ]; then
    printf 'ce-reconcile: MV_BILLING_HOST is empty, so nothing was shipped. The monitor on the host will report this reading as missing until it is.\n' >&2
  fi
  exit 0
fi

if [ "$DRY_RUN" = 1 ]; then
  printf '\nce-reconcile: DRY RUN. Nothing was written to %s. The command it would run:\n' "$MV_BILLING_HOST" >&2
  printf '  ssh %s "%s"\n' "$MV_BILLING_HOST" "$SHIP_CMD" >&2
  exit 0
fi

command -v ssh >/dev/null 2>&1 || die "ssh is not installed and MV_BILLING_HOST is set"
if ! ssh -o BatchMode=yes "$MV_BILLING_HOST" "$SHIP_CMD" <"$RESULT" 2>"$WORK/ssh.err"; then
  printf 'ce-reconcile: could not write %s on %s:\n' "$MV_BILLING_REMOTE_PATH" "$MV_BILLING_HOST" >&2
  sed 's/^/  /' "$WORK/ssh.err" >&2
  printf '  A scheduled run has no ssh agent unless it is given one. Check that\n' >&2
  printf '  the key is reachable without a passphrase prompt and that %s can\n' "${MV_BILLING_HOST%%@*}" >&2
  printf '  sudo without one either.\n' >&2
  exit 1
fi

[ "$PRINT" = 1 ] || printf 'ce-reconcile: wrote %s on %s (%s Cost Explorer call, $%s)\n' \
  "$MV_BILLING_REMOTE_PATH" "$MV_BILLING_HOST" "$CE_CALLS" \
  "$(python3 -c "print('%.2f' % (0.01 * $CE_CALLS))")"
