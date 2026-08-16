#!/usr/bin/env bash
# Capture ONE numeric health reading of the live map, as a record rather than a
# threshold.
#
#   deploy/health-snapshot.sh                      read the public front door
#   deploy/health-snapshot.sh --url http://127.0.0.1:8796   read the archive directly
#   deploy/health-snapshot.sh --label before-archive-1.2    name the reading
#   deploy/health-snapshot.sh --markdown           emit a receipt table row
#   deploy/health-snapshot.sh --watch 15m          sample until the window closes
#
# WHY THIS EXISTS, AND WHY IT IS NOT monitor.sh.
#
# monitor.sh answers "is anything wrong right now" and then THROWS THE NUMBER
# AWAY: every check compares a reading to a threshold and keeps only a severity.
# That is the right shape for an alert and the wrong shape for a deployment,
# because it cannot answer the question a deployment actually raises — "is this
# worse than it was fifteen minutes ago". A deployment record that says every
# check passed is compatible with the service being measurably degraded, and the
# existing records say exactly that and nothing more.
#
# So this tool keeps numbers and no thresholds. It never decides anything. It
# has no severity, no exit code that means "bad", and nothing to configure but
# where to read from. What it produces is evidence: a line per sample, and on
# request a markdown row that drops into a deployment receipt's health window.
#
# THE UNKNOWN RULE APPLIES HERE TOO (§10.1, and status.go's header). A reading
# the map does not publish is emitted as null and never as zero. A world that
# reported nothing is unknown, not empty. `population: 0` and `population: null`
# mean different things and this tool preserves the difference, because a
# deployment comparison that silently reads a gap as a zero manufactures a
# regression that did not happen — or hides one that did.
#
# NOTHING HERE WRITES TO THE SERVICE. It is GET-only against a read-only mux, so
# it is safe to run against production during an incident, and safe to run from
# a laptop. Read-only collection needs no production change record.
set -u

URL="${MV_HEALTH_URL:-https://bibitesmultiverse.com}"
LABEL=""
MARKDOWN=0
WATCH=""
EVERY="${MV_HEALTH_EVERY:-30}"

say()  { printf '%s\n' "$*" >&2; }
die()  { printf 'STOP: %s\n' "$*" >&2; exit 2; }

while [ $# -gt 0 ]; do
  case "$1" in
    --url)      URL="${2:?--url needs a value}"; shift ;;
    --label)    LABEL="${2:?--label needs a value}"; shift ;;
    --markdown) MARKDOWN=1 ;;
    --watch)    WATCH="${2:?--watch needs a duration, e.g. 15m}"; shift ;;
    --every)    EVERY="${2:?--every needs seconds}"; shift ;;
    -h|--help)  sed -n '2,40p' "$0"; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
  shift
done

command -v curl >/dev/null || die "no curl on PATH"
command -v jq   >/dev/null || die "no jq on PATH; this tool emits JSON and will not hand-roll it"

URL="${URL%/}"

# Seconds from a duration like 15m / 90s / 2h. A deployment window is stated in
# minutes by every runbook that mentions one, so accept that spelling.
to_seconds() {
  case "$1" in
    *h) printf '%s' $(( ${1%h} * 3600 )) ;;
    *m) printf '%s' $(( ${1%m} * 60 )) ;;
    *s) printf '%s' "${1%s}" ;;
    *)  printf '%s' "$1" ;;
  esac
}

# One sample. Emits a single JSON object on stdout, or nothing on failure —
# and a failure is itself a reading, so it is emitted as a record with ok:false
# rather than swallowed. An unreachable front door during a deployment window is
# the most interesting sample the window can contain.
sample() {
  local now body code rtt json
  now="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  body="$(curl -s --max-time 20 -w '\n%{http_code}\n%{time_total}' "$URL/api/status" 2>/dev/null)"
  code="$(printf '%s' "$body" | tail -n 2 | head -n 1)"
  rtt="$(printf '%s' "$body" | tail -n 1)"
  json="$(printf '%s' "$body" | head -n -2)"

  if [ "$code" != "200" ] || ! printf '%s' "$json" | jq -e . >/dev/null 2>&1; then
    jq -nc --arg t "$now" --arg l "$LABEL" --arg c "$code" --arg r "${rtt:-0}" \
      '{t:$t, label:$l, ok:false, httpCode:$c, rttSeconds:($r|tonumber)}'
    return 0
  fi

  # Everything below is read straight from the published status. Nothing is
  # derived that the map does not already derive, except the two counts, which
  # are lengths and not measurements.
  #
  # `structuralHoles` is published beside `bypassedLanes` on purpose. A map whose
  # grid is larger than its peer count has permanently empty cells, and the lanes
  # around them are bypassed forever. Reporting the bypass count alone makes a
  # static map shape read as a worsening incident, which is a mistake this
  # project already made once.
  #
  # It is a COUNT OF CELLS and not the lane floor: one empty cell bypasses
  # several lanes, and how many depends on where in the grid it sits and whether
  # the map wraps. So the two are published side by side and neither is
  # subtracted from the other. Naming this number "the floor" would be the same
  # confident-but-wrong reading it exists to prevent.
  printf '%s' "$json" | jq -c \
    --arg t "$now" --arg l "$LABEL" --arg r "${rtt:-0}" '
    {
      t: $t,
      label: $l,
      ok: true,
      rttSeconds: ($r|tonumber),

      relayConnected: .relayConnected,
      statusAgeMs:    .statusAgeMs,
      epoch:          .epoch,
      observers:      .observers,
      mapWidth:       .map.width,
      mapHeight:      .map.height,
      slotCount:      .slotCount,

      liveSlots:  ([.slots[] | select(.live)]        | length),
      darkSlots:  ([.slots[] | select(.live | not)]  | length),
      knownSlots: ([.slots[] | select(.statsKnown)]  | length),

      structuralHoles: ((.map.width * .map.height) - (.slots | length)),
      bypassedLanes:   ([.lanes[]? | select(.open | not)] | length),

      # Unknown stays unknown. A slot with no census contributes null, and a sum
      # over nulls is null rather than a confident zero.
      population: ( [.slots[] | .population] as $p
                    | if ($p | map(select(. != null)) | length) == 0
                      then null else ($p | map(. // 0) | add) end ),

      # SIZING.md s workload variable: the sum of achieved time scales. This is
      # the number every capacity formula in that document is written against,
      # so a deployment window that changes it has changed the cost model.
      achievedSum: ( [.slots[] | .achievedTimeScale] as $a
                     | if ($a | map(select(. != null)) | length) == 0
                       then null else (($a | map(. // 0) | add) * 100 | floor) / 100 end ),

      maxStatsAgeMs:   ([.slots[] | .statsAgeMs   // 0] | max),
      maxDarkForMs:    ([.slots[] | .darkForMs    // 0] | max),
      oldestSaveAgeMs: ([.slots[] | .lastSaveAgeMs // null] | map(select(. != null))
                        | if length == 0 then null else max end),

      refusals: [.slots[] | select(.lastRefusal != null)
                 | {slot: .slot, refusal: .lastRefusal}],

      ledgerRecords:      .ledgerRecords,
      ledgerSkippedLines: .ledgerSkippedLines,
      genomeGaps:         .genomeGaps
    }'
}

# A receipt row. Deliberately narrow: the columns an operator compares across a
# deployment, not everything the sample carries. The JSON keeps the rest.
markdown_row() {
  jq -r '
    def n: if . == null then "unknown" else tostring end;
    "| \(.label // "-") | \(.t) | \(if .ok then "yes" else "NO (HTTP \(.httpCode))" end) "
    + "| \(.liveSlots // "-")/\(.slotCount // "-") | \(.darkSlots // "-") "
    + "| \(.population | n) | \(.achievedSum | n) | \(.statusAgeMs | n) "
    + "| \(.maxStatsAgeMs | n) | \(.bypassedLanes | n) with \(.structuralHoles | n) empty cell(s) "
    + "| \(.ledgerRecords | n) |"'
}

emit() {
  if [ "$MARKDOWN" = 1 ]; then markdown_row; else cat; fi
}

if [ "$MARKDOWN" = 1 ]; then
  printf '| label | at (UTC) | reachable | live/slots | dark | population | achieved sum | status age ms | worst stats age ms | bypassed lanes | ledger records |\n'
  printf '|---|---|---|---|---:|---:|---:|---:|---:|---|---:|\n'
fi

if [ -z "$WATCH" ]; then
  sample | emit
  exit 0
fi

# A window, not a moment. One reading before and after a deployment is two
# points and cannot show a slow regression or a flap that resolves between them;
# the runbooks' single post-deploy curl is exactly that shape and it is why a
# four-minute outage inside a deployment window has never once been recorded.
deadline=$(( $(date -u +%s) + $(to_seconds "$WATCH") ))
say "sampling every ${EVERY}s until $(date -u -d "@$deadline" +%H:%M:%SZ 2>/dev/null || echo "the window closes")"
while [ "$(date -u +%s)" -lt "$deadline" ]; do
  sample | emit
  sleep "$EVERY"
done
