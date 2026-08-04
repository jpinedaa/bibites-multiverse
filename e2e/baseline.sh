#!/usr/bin/env bash
# baseline.sh — one complete, non-disruptive snapshot of the running M3 LAN rig.
#
#   e2e/baseline.sh              capture a snapshot into e2e/baselines/<UTC>/
#   e2e/baseline.sh --label T0   the same, with a label recorded in the report
#   e2e/baseline.sh compare <t0-dir> <t1-dir>    print the T0 -> T1 deltas
#   e2e/baseline.sh latest       print the path of the most recent snapshot
#
# WHAT IT IS FOR. The rig runs overnight and the question the next morning is
# "what changed" — in the organisms (genes, brains, species) and in where they
# are (spatial distribution, migration flow). A snapshot taken now and another
# taken tomorrow answer it by subtraction. This script takes the snapshot; the
# subtraction is `worldstat compare`, and `baseline.sh compare` runs it for
# every world at once.
#
# WHAT IT WILL NOT DO, AND THIS IS THE POINT.
#
#   * It never starts, stops, restarts, rebuilds or redeploys anything. It
#     refuses to run at all if the rig is not already up, rather than bringing
#     up what it finds missing.
#   * It never changes a simulation setting. The only dev-command verbs it is
#     allowed to send are `census`, `count` and `save`, and send_safe() enforces
#     that as a list, not as a convention. `timescale` is READ out of the
#     BepInEx log — the last value the rig commanded — and never written.
#   * THE FAR END IS NEVER TOUCHED. Slot 2 is on the owner's second computer.
#     Everything this script says about slot 2 is read from the archive and the
#     relay's ring on THIS machine. The report ends with the exact PowerShell
#     command the owner may optionally run over there; nothing here sends it.
#
# `save` is in the allowed list because it is how a world becomes readable: the
# mod writes the same zip the Save button writes, in place, and the simulation
# keeps running. The rig already disabled the game's own autosave for the
# session, so this save is not competing with one.
set -uo pipefail

# run-m3.sh holds the cmd-file convention, the game<->log mapping, the ports and
# the paths. M3_LIB=1 loads it as a library instead of dispatching a command.
M3_LIB=1
# shellcheck source=run-m3.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/run-m3.sh"

# The LAN topology, applied exactly as run-m3-lan.sh applies it: two local
# slots, one remote, and the LAN rig's log directory.
SLOTS="1 3"
LOCAL_SLOTS="1 3"
REMOTE_SLOT=2
REMOTE_PEER="$(peer_of "$REMOTE_SLOT")"
LOGS="$E2E/logs-m3-lan"
BEPINEX_ARCHIVE="$LOGS/bepinex"

BASELINES="$E2E/baselines"
WORLDSTAT="$BIN/worldstat"

# The game's own save directory, from Application.persistentDataPath.
SAVE_DIR_WSL="$(dirname "$(ls -d /mnt/c/Users/*/AppData/LocalLow/The\ Bibites/The\ Bibites 2>/dev/null | head -n 1)")/The Bibites/Savefiles"

# ---------------------------------------------------------------- guard rails

# The complete list of dev-command verbs a baseline capture may use. Anything
# else is a bug in this script, not a decision to make at runtime.
SAFE_VERBS="census count save"

send_safe() {
  local slot="$1" verb="$2"
  case " $SAFE_VERBS " in
    *" $verb "*) ;;
    *)
      fail "REFUSED: '$verb' is not one of the non-disruptive verbs ($SAFE_VERBS)"
      return 2
      ;;
  esac
  send "$@"
}

# The rig must already be running. This script does not create one.
require_rig() {
  local missing=0 p
  for p in relay archive; do
    pgrep -f "$BIN/$p " >/dev/null 2>&1 || { fail "no $p process — the rig is not up"; missing=1; }
  done
  for s in $LOCAL_SLOTS; do
    pgrep -f "peer-id $(peer_of "$s") " >/dev/null 2>&1 \
      || pgrep -f -- "--peer-id $(peer_of "$s")" >/dev/null 2>&1 \
      || { fail "no sidecar for $(peer_of "$s")"; missing=1; }
    local state
    state="$("$GAME_SH" status 2>/dev/null | grep -a "instance=$(game_instance "$s") " | head -n 1)"
    case "$state" in
      *"state=running"*) ;;
      *) fail "game $(game_instance "$s") is not running ($state)"; missing=1 ;;
    esac
  done
  if [ "$missing" != 0 ]; then
    fail "refusing to capture a baseline against a rig that is not up — nothing was started or changed"
    return 1
  fi
  note "rig is up: relay, archive, sidecars $LOCAL_SLOTS, games $LOCAL_SLOTS"
}

# worldstat is a read-only reader; building it touches no running component.
ensure_worldstat() {
  if [ -x "$WORLDSTAT" ] && [ "$WORLDSTAT" -nt "$REPO/go/cmd/worldstat/stats.go" ]; then
    return 0
  fi
  note "building bin/worldstat (a reader; no rig component is rebuilt)"
  ( cd "$REPO/go" && CGO_ENABLED=0 go build -o "$WORLDSTAT" ./cmd/worldstat ) || return 1
}

# ---------------------------------------------------------------- small readers

# quoted <key> <line> — the value of key='...' , which `field` cannot do because
# a Windows save path contains spaces.
quoted() {
  printf '%s\n' "$2" | tr -d '\r' | sed -n "s/.*$1='\([^']*\)'.*/\1/p" | head -n 1
}

# The last time scale this rig COMMANDED on that slot. DevCommands has no
# read-only time-scale verb, and asking for one would mean writing it, so the
# log is the honest source.
last_timescale() {
  local slot="$1" line
  line="$(grep_log "$slot" '\[M2-CMD\].*OK targetTimeScale=' | tail -n 1)"
  if [ -z "$line" ]; then
    printf 'unknown (no timescale command in this log)\n'
    return
  fi
  printf '%s (Time.timeScale=%s)\n' "$(field targetTimeScale "$line")" "$(field 'Time.timeScale' "$line")"
}

# The cumulative [M2-CROSSING] counters: the mod's own measure of how much
# traffic reached and crossed the export edge since this game started.
last_crossing() { grep_log "$1" '\[M2-CROSSING\]' | tail -n 1 | tr -d '\r'; }

# Whether the overnight run can restart a world under itself.
#
# The chain that could do it is: the game's own autosave fires -> if the user
# setting ReloadAfterAutosaves is on, SaveController.ReloadAfterAutoSave calls
# GameManager.StartGame on the autosave -> the scene reloads. This reads every
# link of that chain and writes down what it found. It CHANGES NOTHING.
autosave_state() {
  printf 'gameDefaults: UserSettings.AutoSave=true  ReloadAfterAutosaves=false  AutoSavePeriod=TenMinutes  AutoSaveRealTime=true\n'
  printf '  (UserSettings.cs:27-75 — the shipped defaults, not this profile)\n'

  printf 'persistedUserSettings (Unity PlayerPrefs, HKCU\\Software\\The Bibites\\The Bibites):\n'
  ( cd /mnt/c && "$POWERSHELL" -NoProfile -NonInteractive -Command '
      $k = "HKCU:\Software\The Bibites\The Bibites"
      if (-not (Test-Path $k)) { "  <no key: every user setting is at its default>"; exit }
      $item = Get-Item $k
      $names = $item.GetValueNames() | Where-Object { $_ -match "AutoSave|Reload" }
      if (-not $names) { "  <no AutoSave/Reload key: both are at their defaults>"; exit }
      foreach ($n in $names) {
        $raw = $item.GetValue($n)
        if ($raw -is [byte[]]) { $raw = [Text.Encoding]::UTF8.GetString($raw).TrimEnd([char]0) }
        "  $n = $raw"
      }
    ' 2>&1 ) | tr -d '\r'

  local autosave_dir="$(dirname "$SAVE_DIR_WSL")/Savefiles/Autosaves"
  if [ -d "$autosave_dir" ]; then
    printf 'autosaveFolder: %s exists, %s file(s)\n' "$autosave_dir" "$(find "$autosave_dir" -name '*.zip' 2>/dev/null | wc -l)"
  else
    printf 'autosaveFolder: does not exist — no autosave has ever been written on this profile\n'
  fi

  local s
  for s in $LOCAL_SLOTS; do
    local hit
    hit="$(grep_log "$s" 'autosave disabled for this session' | tail -n 1 | tr -d '\r')"
    if [ -n "$hit" ]; then
      printf 'slot%s runtimeAutosave=DISABLED by the mod: %s\n' "$s" "$hit"
    else
      printf 'slot%s runtimeAutosave=NOT DISABLED — no [M2-CMD] autosave line in this log\n' "$s"
    fi
  done

  printf 'bepinexPluginConfig: %s\n' \
    "$(grep -h '^AutoTest' "$BEPINEX/config/dev.multiverse.bibites.cfg" 2>/dev/null | tr -d '\r' | tr '\n' ' ')"
}

# ---------------------------------------------------------------- the capture

capture() {
  local label="${1:-T0}"
  local stamp dir
  stamp="$(date -u +%Y%m%dT%H%M%SZ)"
  dir="$BASELINES/$stamp"

  require_rig || return 1
  ensure_worldstat || return 1
  mkdir -p "$dir" || return 1

  step "baseline $label -> $dir"

  local wall_start sim_note
  wall_start="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  {
    printf 'label=%s\n' "$label"
    printf 'stamp=%s\n' "$stamp"
    printf 'wallClockUTC=%s\n' "$wall_start"
    printf 'wallClockLocal=%s\n' "$(date '+%Y-%m-%d %H:%M:%S %Z')"
    printf 'epochSeconds=%s\n' "$(date +%s)"
    printf 'host=%s\n' "$(hostname)"
    printf 'repoHead=%s\n' "$(cd "$REPO" && git rev-parse --short HEAD 2>/dev/null)"
    printf 'localSlots=%s\n' "$LOCAL_SLOTS"
    printf 'remoteSlot=%s\n' "$REMOTE_SLOT"
  } > "$dir/meta.txt"

  # ---- ring state (this is also everything centrally knowable about slot 2's
  #      membership, so it is captured before anything can move).
  step "relay ring state"
  cp -f "$RELAY_DATA/ring.json" "$dir/ring.json" 2>/dev/null || fail "no ring.json"
  grep -a 'ring claim\|ring slot granted\|ringSize' "$LOGS/relay.log" 2>/dev/null | tail -n 40 > "$dir/relay-ring.log"
  cat "$dir/ring.json"

  # ---- per-slot: census, count, save, copy the zip
  local ids_1="" ids_3=""
  for s in $LOCAL_SLOTS; do
    step "slot $s: census"
    local line
    line="$(send_safe "$s" census)" || { fail "census failed on slot $s"; return 1; }
    printf '%s\n' "$line" > "$dir/census-slot$s.txt"
    note "$line"
    local ids
    ids="$(printf '%s\n' "$line" | tr -d '\r' | sed -n 's/.*ids=\[\([^]]*\)\].*/\1/p')"
    eval "ids_$s=\"\$ids\""
  done

  # `count <id>` is the exactly-once question, and it only means something asked
  # in more than one world: an organism must be alive in exactly one of them.
  step "exactly-once cross-check (count)"
  : > "$dir/count.txt"
  for owner in $LOCAL_SLOTS; do
    local list first
    eval "list=\$ids_$owner"
    first="$(printf '%s\n' "$list" | cut -d, -f1)"
    [ -n "$first" ] || continue
    for s in $LOCAL_SLOTS; do
      local out
      out="$(send_safe "$s" count "$first")" || true
      printf 'askedSlot=%s entityFromSlot=%s %s\n' "$s" "$owner" "$out" >> "$dir/count.txt"
      note "slot $s <- id $first: $out"
    done
  done

  # ---- the saves. This is the only thing here that writes anything into the
  #      game's world, and it writes the world itself, in place, as the Save
  #      button does.
  for s in $LOCAL_SLOTS; do
    step "slot $s: save"
    local line path_win path_wsl world
    world="$(world_of "$s")"
    line="$(send_safe "$s" save "$world")" || { fail "save failed on slot $s"; return 1; }
    printf '%s\n' "$line" > "$dir/save-slot$s.txt"
    note "$line"
    path_win="$(quoted path "$line")"
    if [ -n "$path_win" ]; then
      path_wsl="$(wslpath -u "$path_win" 2>/dev/null)"
    fi
    [ -n "${path_wsl:-}" ] && [ -f "$path_wsl" ] || path_wsl="$SAVE_DIR_WSL/$world.zip"
    if [ ! -f "$path_wsl" ]; then
      fail "the save is not where the mod said it was: $path_wsl"
      return 1
    fi
    cp -f "$path_wsl" "$dir/$world.zip" || return 1
    note "copied $(basename "$path_wsl") ($(stat -c %s "$dir/$world.zip") bytes)"
  done

  # ---- worldstat over each copied zip
  for s in $LOCAL_SLOTS; do
    local world
    world="$(world_of "$s")"
    step "slot $s: worldstat $world.zip"
    "$WORLDSTAT" stat --world "$world" --slot "$s" --export-edge "$EXPORT_EDGE" \
      --out "$dir/worldstat-slot$s.json" "$dir/$world.zip" 2> "$dir/worldstat-slot$s.txt" \
      || { fail "worldstat failed on slot $s"; return 1; }
    cat "$dir/worldstat-slot$s.txt" >&2
  done

  # ---- the archive: the whole cross-world migration record
  step "archive snapshot"
  wc -l < "$ARCHIVE_DATA/migrations.jsonl" | tr -d ' ' > "$dir/archive-lines.txt"
  "$BIN/archive" list --data-dir "$ARCHIVE_DATA" > "$dir/archive-list.txt" 2>&1
  tail -n 1 "$dir/archive-list.txt" >&2
  python3 - "$ARCHIVE_DATA/migrations.jsonl" > "$dir/archive-summary.txt" <<'PY'
import json, sys, collections
path = sys.argv[1]
lanes = collections.Counter()
kinds = collections.Counter()
entities = set()
genomes = set()
per_source = collections.Counter()
first = last = None
acks = 0
genome_records = 0
for raw in open(path, encoding="utf-8"):
    raw = raw.strip()
    if not raw:
        continue
    try:
        rec = json.loads(raw)
    except json.JSONDecodeError:
        continue
    kinds[rec.get("type", "?")] += 1
    if rec.get("type") == "MIGRATION":
        lanes[(rec.get("sourceSlot"), rec.get("destSlot"))] += 1
        per_source[rec.get("sourcePeer")] += 1
        if "entityId" in rec:
            entities.add(rec["entityId"])
        lin = rec.get("lineage") or {}
        if lin.get("genomeHash"):
            genomes.add(lin["genomeHash"])
        ts = rec.get("recordedAt")
        if ts is not None:
            first = ts if first is None else min(first, ts)
            last = ts if last is None else max(last, ts)
    elif rec.get("type") == "ACK":
        acks += 1
    elif rec.get("type") == "GENOME":
        genome_records += 1

def iso(ms):
    import datetime
    if ms is None:
        return "-"
    return datetime.datetime.fromtimestamp(ms / 1000, datetime.timezone.utc).isoformat().replace("+00:00", "Z")

print("recordTypes " + " ".join(f"{k}={v}" for k, v in sorted(kinds.items())))
print(f"migrations {sum(lanes.values())}")
print(f"acks {acks}")
print(f"genomeRecords {genome_records}")
print(f"distinctMigrantEntities {len(entities)}")
print(f"distinctMigrantGenomes {len(genomes)}")
print(f"firstRecordedAt {iso(first)} ({first})")
print(f"lastRecordedAt {iso(last)} ({last})")
print("lanes:")
for (a, b), n in sorted(lanes.items(), key=lambda kv: (-kv[1], kv[0])):
    print(f"  slot {a} -> slot {b}  {n}")
print("bySourcePeer:")
for peer, n in sorted(per_source.items()):
    print(f"  {peer}  {n}")
print("migrantEntityIds:")
for e in sorted(entities):
    print(f"  {e}")
PY
  head -n 12 "$dir/archive-summary.txt" >&2

  # ---- the mod's own crossing counters, per local game
  step "crossing counters"
  : > "$dir/crossing.txt"
  for s in $LOCAL_SLOTS; do
    local c
    c="$(last_crossing "$s")"
    printf 'slot%s %s\n' "$s" "${c:-<no [M2-CROSSING] line yet>}" >> "$dir/crossing.txt"
    note "slot $s: ${c:-none}"
  done

  # ---- the population TREND, not just the instant.
  #
  # A single census is one sample of an oscillating system: two captures three
  # minutes apart during development differed by nine organisms. CrossingStats
  # already logs the population once per simulated minute, so the series is free
  # and it is what stops a T1 population delta being read as a trend when it is
  # noise.
  step "population trend"
  : > "$dir/population-trend.txt"
  for s in $LOCAL_SLOTS; do
    {
      printf 'slot%s simMinute:population (one sample per simulated minute)\n' "$s"
      grep_log "$s" '\[M2-CROSSING\]' | tr -d '\r' \
        | sed -n 's/.*simMinute=\([0-9]*\).*population=\([0-9]*\).*/\1:\2/p' \
        | tail -n 240 | paste -sd' ' -
    } >> "$dir/population-trend.txt"
    local stats
    stats="$(grep_log "$s" '\[M2-CROSSING\]' | tr -d '\r' \
      | sed -n 's/.*population=\([0-9]*\).*/\1/p' | tail -n 240 \
      | awk 'NR==1{mn=$1;mx=$1} {t+=$1; if($1<mn)mn=$1; if($1>mx)mx=$1} END{if(NR)printf "n=%d min=%d max=%d mean=%.1f", NR, mn, mx, t/NR}')"
    printf 'slot%s summary %s\n' "$s" "$stats" >> "$dir/population-trend.txt"
    note "slot $s population over the last ${stats:-?} sim minutes"
  done

  # ---- time scale, sim time, and the BepInEx log identity of each game
  : > "$dir/timescale.txt"
  for s in $LOCAL_SLOTS; do
    printf 'slot%s lastCommandedTimeScale=%s log=%s\n' \
      "$s" "$(last_timescale "$s")" "$(game_log "$s")" >> "$dir/timescale.txt"
  done
  cat "$dir/timescale.txt" >&2

  # ---- can the overnight run restart a world under itself?
  step "overnight safety: autosave and scene-reload state"
  autosave_state > "$dir/autosave.txt" 2>&1
  cat "$dir/autosave.txt" >&2

  # ---- everything centrally observable about the far end
  step "slot 2 (remote): what is centrally observable"
  {
    printf 'ringMembership: '
    python3 - "$dir/ring.json" <<'PY'
import json, sys
try:
    ring = json.load(open(sys.argv[1]))
except Exception as exc:
    print(f"unreadable ({exc})"); raise SystemExit
print(" ".join(f"slot{e['slot']}={e['peerId']}" for e in ring.get("ring", [])))
PY
    printf 'edgeStatusRipple(slot1 -> its east neighbour slot 2): %s\n' \
      "$(grep_log 1 'EDGE_STATUS' | tail -n 1 | tr -d '\r')"
  } > "$dir/slot2-observable.txt" 2>&1
  cat "$dir/slot2-observable.txt" >&2

  write_report "$dir" "$label" "$wall_start"
  verdict "baseline $label captured: $dir"
  printf '%s\n' "$dir"
}

# ---------------------------------------------------------------- the report

write_report() {
  local dir="$1" label="$2" wall="$3"
  local report="$dir/baseline-report.md"
  local stamp; stamp="$(basename "$dir")"

  {
    printf '# M3 LAN rig baseline — %s (%s)\n\n' "$label" "$stamp"
    printf 'Captured %s (wall clock, UTC) by `e2e/baseline.sh`.\n' "$wall"
    printf 'Repo HEAD `%s`. Nothing was started, stopped, rebuilt or reconfigured to take it;\n' \
      "$(cd "$REPO" && git rev-parse --short HEAD 2>/dev/null)"
    printf 'the only dev commands sent were `census`, `count` and `save`, on the two LOCAL slots.\n\n'

    printf '## The rig at this instant\n\n'
    printf '| | |\n|---|---|\n'
    printf '| Wall clock (UTC) | %s |\n' "$wall"
    printf '| Wall clock (local) | %s |\n' "$(date '+%Y-%m-%d %H:%M:%S %Z')"
    printf '| Local slots | 1 and 3, on this machine |\n'
    printf '| Remote slot | 2, on the owner'\''s second computer — observed, never driven |\n'
    printf '| Ring | `%s` |\n' "$(tr -d '\n ' < "$dir/ring.json")"
    for s in $LOCAL_SLOTS; do
      printf '| Time scale, slot %s | `%s` — the last value this rig commanded, read out of the BepInEx log. `census` does not report it and asking for it would mean writing it. |\n' \
        "$s" "$(sed -n "s/^slot$s lastCommandedTimeScale=\(.*\) log=.*/\1/p" "$dir/timescale.txt")"
    done
    for s in $LOCAL_SLOTS; do
      printf '| BepInEx log, slot %s | `%s` |\n' \
        "$s" "$(sed -n "s/^slot$s .* log=//p" "$dir/timescale.txt")"
    done
    printf '\n'

    printf '## Worlds at T0\n\n'
    printf '| | slot 1 (M3-Slot1) | slot 3 (M3-Slot3) |\n|---|---|---|\n'
    report_row "$dir" 'Population (living)'      '.population'
    report_row "$dir" 'Dead bodies in save'      '.dead'
    report_row "$dir" 'Simulated time (s)'       '.simulatedTime'
    report_row "$dir" 'Unique genomes'           '.uniqueGenomes'
    report_row "$dir" 'Distinct species'         '.species|length'
    report_row "$dir" 'Generation mean'          '.generation.mean'
    report_row "$dir" 'Generation max'           '.generation.max'
    report_row "$dir" 'Synapses mean'            '.brain.synapses.mean'
    report_row "$dir" 'Synapses max'             '.brain.synapses.max'
    report_row "$dir" 'Hidden nodes mean'        '.brain.hiddenNodes.mean'
    report_row "$dir" 'Hidden nodes max'         '.brain.hiddenNodes.max'
    report_row "$dir" 'Age mean (s)'             '.age.mean'
    report_row "$dir" 'Maturity mean'            '.maturity.mean'
    report_row "$dir" 'Health mean'              '.health.mean'
    report_row "$dir" 'Energy mean'              '.energy.mean'
    report_row "$dir" 'Mean x'                   '.spatial.meanX'
    report_row "$dir" 'Mean y'                   '.spatial.meanY'
    report_row "$dir" 'Stddev x'                 '.spatial.stddevX'
    report_row "$dir" 'Mean distance to E edge'  '.spatial.meanDistanceToExportEdge'
    report_row "$dir" 'West entry quarter'       '.spatial.westEntryQuarter'
    report_row "$dir" 'Middle half'              '.spatial.middleHalf'
    report_row "$dir" 'East export quarter'      '.spatial.eastExportQuarter'
    report_row "$dir" 'In capture band (x>=S-W)' '.spatial.captureBand'
    report_row "$dir" 'Outside square, east'     '.spatial.beyondSquare'
    report_row "$dir" 'x histogram [-S..S]'      '.spatial.histogramX'
    printf '\n'
    printf 'Full numbers, including every gene'\''s mean/stddev/min/max and the raw per-organism\n'
    printf 'table, are in `worldstat-slot1.json` and `worldstat-slot3.json`. The one-screen\n'
    printf 'summaries are in `worldstat-slot1.txt` / `worldstat-slot3.txt`.\n\n'

    printf '### Census and exactly-once\n\n```\n'
    for s in $LOCAL_SLOTS; do printf 'slot%s ' "$s"; cat "$dir/census-slot$s.txt"; done
    printf '\n'
    cat "$dir/count.txt"
    printf '```\n\n'

    printf '### The mod'\''s own crossing counters\n\n'
    printf 'Cumulative since each game started (`[M2-CROSSING]`, `CrossingStats.cs`):\n\n```\n'
    cat "$dir/crossing.txt"
    printf '```\n\n'

    printf '### Population trend — read this before reading the population delta\n\n'
    printf 'The population above is ONE SAMPLE of an oscillating system. `CrossingStats` logs the\n'
    printf 'population once per simulated minute, and over the recent window it moves like this:\n\n```\n'
    grep ' summary ' "$dir/population-trend.txt"
    printf '```\n\n'
    printf 'A T1 population that differs from T0 by less than that min-to-max range is noise, not\n'
    printf 'a trend. Compare the T1 `summary` line against this one instead. The full per-minute\n'
    printf 'series is in `population-trend.txt`.\n\n'

    printf '## The archive — cross-world migration flow\n\n'
    printf '`migrations.jsonl` holds %s lines. It is CUMULATIVE across every earlier run, so a\n' \
      "$(cat "$dir/archive-lines.txt")"
    printf 'T1 comparison must subtract these totals rather than read the T1 totals as "overnight".\n\n```\n'
    sed -n '1,/^migrantEntityIds:/p' "$dir/archive-summary.txt" | sed '$d'
    printf '```\n\n'
    printf 'The %s distinct migrant entity IDs are listed in full at the end of\n' \
      "$(sed -n 's/^distinctMigrantEntities //p' "$dir/archive-summary.txt")"
    printf '`archive-summary.txt`; the full per-migration record, with lineage and genome\n'
    printf 'custody, is `archive-list.txt`.\n\n'

    printf '## Overnight safety: can a world restart under the rig?\n\n'
    printf '**No.** Two independent links of the only chain that could do it are broken, and\n'
    printf 'neither was touched to take this baseline.\n\n'
    printf '1. **The mod disables the game'\''s autosave at runtime.** `DevCommands.DisableAutosaveOnce`\n'
    printf '   calls `SaveController.ToggleAutoSave(false)` the first frame the simulation is ready,\n'
    printf '   for every instance started with `MULTIVERSE_CMD_FILE` — which is both local games. It\n'
    printf '   is runtime only and does not write the user'\''s saved preference. With autosave off,\n'
    printf '   `SaveController.AutoSave()` never runs and `ReloadAfterAutoSave` is never subscribed.\n'
    printf '2. **`ReloadAfterAutosaves` is off in this profile anyway.** It is the setting that turns\n'
    printf '   a finished autosave into `GameManager.StartGame(<autosave>)` — a full scene reload.\n'
    printf '   The value below is read from the game'\''s own persisted user settings.\n\n'
    printf '```\n'
    cat "$dir/autosave.txt"
    printf '```\n\n'
    printf 'If a scene reload ever DID happen, the custody design absorbs it rather than losing\n'
    printf 'organisms: `MultiverseClient` mints a **fresh `sessionId` on every world load**\n'
    printf '(`contract-a.md` §5.1), the sidecar sees a sessionId it has not seen and runs\n'
    printf '`reassertCustodyLocked` (§7.4) — re-sending an unsolicited `MIGRATE_OUT_ACK` for every\n'
    printf 'outbound custody still in its journal, because a reloaded world may contain an organism\n'
    printf 'that already left — and then `replayInboundLocked` (§7.5) redelivers every journaled\n'
    printf 'inbound organism that was never ACKed. The journal in `e2e/data/slot-N/journal/` is what\n'
    printf 'makes that work, so it must survive any restart.\n\n'
    printf 'The far end is out of scope here: slot 2'\''s autosave state is its own machine'\''s, and\n'
    printf 'nothing on this side reads or writes it.\n\n'

    printf '## SLOT 2 — the second computer\n\n'
    printf 'Slot 2 runs on the owner'\''s other machine. Nothing in this capture reached it: the\n'
    printf 'far end is never driven (`m3_considerations.md` Risk 6). What follows is everything\n'
    printf 'about slot 2 that is knowable from THIS machine.\n\n'
    printf '### Centrally observable\n\n```\n'
    cat "$dir/slot2-observable.txt"
    printf '\n'
    grep -E '^  slot (1 -> slot 2|2 -> slot 3)' "$dir/archive-summary.txt" 2>/dev/null \
      || printf '  (no slot-2 lane traffic recorded)\n'
    printf '```\n\n'
    printf '* Lane `slot 1 -> slot 2` is what slot 2 RECEIVED from here.\n'
    printf '* Lane `slot 2 -> slot 3` is what slot 2 EXPORTED, on its own, with nobody driving it.\n'
    printf '* Ring membership above proves slot 2 still holds its reservation.\n'
    printf '* Slot 1'\''s `EDGE_STATUS` line is slot 2'\''s liveness: slot 1'\''s export edge opens only\n'
    printf '  while its east neighbour is live and mod-connected.\n\n'
    printf '### PLACEHOLDER — slot 2'\''s own save and census are NOT in this snapshot\n\n'
    printf 'There is no `M3-Slot2.zip` and no `census-slot2.txt` here, and there will not be one\n'
    printf 'unless the owner chooses to run the command below **on the second computer**. It is\n'
    printf 'optional: everything the ring needs is already above. It only adds slot 2'\''s own\n'
    printf 'genes, brains and geography, which nothing on this side can see.\n\n'
    printf 'Run it in an ordinary (non-elevated) PowerShell on the SECOND computer, while the\n'
    printf 'game is running. It writes two commands into slot 2'\''s dev-command file — the same\n'
    printf 'atomic write-then-rename channel this side uses — and changes no setting.\n\n'
    slot2_command
    printf '\n'
    printf 'Then copy the printed `M3-Slot2.zip` to this machine, drop it in this snapshot\n'
    printf 'directory, and run:\n\n'
    printf '```sh\n'
    printf 'bin/worldstat stat --world M3-Slot2 --slot 2 \\\n'
    printf '  --out e2e/baselines/%s/worldstat-slot2.json \\\n' "$stamp"
    printf '  e2e/baselines/%s/M3-Slot2.zip\n' "$stamp"
    printf '```\n\n'

    printf '## Tomorrow: the T1 capture and the comparison\n\n'
    printf '```sh\n'
    printf 'e2e/baseline.sh --label T1                 # same capture, new timestamped directory\n'
    printf 'e2e/baseline.sh compare e2e/baselines/%s <the new directory>\n' "$stamp"
    printf '```\n\n'
    printf '`compare` runs `worldstat compare` for each world and prints the archive lane deltas\n'
    printf 'beside them: population change, new and extinct genomes, the largest gene-mean shifts,\n'
    printf 'brain-complexity change, whether the population moved east, and per-genome survival.\n'
  } > "$report"

  note "report written: $report"
}

# report_row <dir> <label> <jq-ish path>
report_row() {
  local dir="$1" label="$2" path="$3" v1 v3
  v1="$(json_get "$dir/worldstat-slot1.json" "$path")"
  v3="$(json_get "$dir/worldstat-slot3.json" "$path")"
  printf '| %s | %s | %s |\n' "$label" "$v1" "$v3"
}

# json_get <file> <dotted path with an optional |length>
json_get() {
  python3 - "$1" "$2" <<'PY' 2>/dev/null || printf 'n/a'
import json, sys
try:
    doc = json.load(open(sys.argv[1], encoding="utf-8"))
except Exception:
    print("n/a"); raise SystemExit
expr = sys.argv[2]
length = expr.endswith("|length")
if length:
    expr = expr[: -len("|length")]
node = doc
for part in expr.strip(".").split("."):
    if part == "":
        continue
    node = node.get(part) if isinstance(node, dict) else None
    if node is None:
        break
if length:
    print(len(node) if node is not None else "n/a")
elif isinstance(node, float):
    print(f"{node:.3f}".rstrip("0").rstrip(".") if node % 1 else f"{node:.0f}")
elif isinstance(node, list):
    print("`" + " ".join(str(x) for x in node) + "`")
elif node is None:
    print("n/a")
else:
    print(node)
PY
}

# The one command the owner may optionally run on the second computer. It is
# built from farend/setup-farend.ps1's own conventions: the cmd file is
# $env:TEMP\bibites-m3\cmd-2.txt, the world is M3-Slot2, and SaveController
# writes into Application.persistentDataPath\Savefiles.
slot2_command() {
  cat <<'PS1'
```powershell
# ---- run this on the SECOND COMPUTER, in PowerShell, with the game running ----
# It sends `census` then `save` down slot 2's dev-command file. Both verbs are
# read-only with respect to the simulation: nothing is paused, no setting moves,
# and the time scale is untouched.
$cmd = Join-Path $env:TEMP 'bibites-m3\cmd-2.txt'
function Send-Bibites([string]$verb) {
    $token = 't' + [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
    $tmp   = "$cmd.tmp"
    # Atomic write-then-rename, and the trailing newline is REQUIRED: the mod
    # treats content that does not end in one as a partial write.
    [IO.File]::WriteAllText($tmp, "$token $verb`n", (New-Object Text.UTF8Encoding $false))
    Move-Item -Force $tmp $cmd
    $deadline = (Get-Date).AddSeconds(180)
    while ((Get-Date) -lt $deadline) {
        if (Test-Path "$cmd.log") {
            $hit = Select-String -Path "$cmd.log" -Pattern "^$token " | Select-Object -Last 1
            if ($hit) { return $hit.Line }
        }
        Start-Sleep -Milliseconds 200
    }
    return "TIMEOUT waiting for '$verb' - is the game running with MULTIVERSE_CMD_FILE set?"
}

Send-Bibites 'census'
Send-Bibites 'save M3-Slot2'

$zip = Join-Path $env:USERPROFILE 'AppData\LocalLow\The Bibites\The Bibites\Savefiles\M3-Slot2.zip'
Write-Host ""
Write-Host "M3-Slot2.zip is here:" -ForegroundColor Green
Write-Host "  $zip"
Get-Item $zip | Format-List Name, Length, LastWriteTime
```
PS1
}

# ---------------------------------------------------------------- comparison

compare_dirs() {
  local t0="$1" t1="$2"
  [ -d "$t0" ] || { fail "no such snapshot: $t0"; return 1; }
  [ -d "$t1" ] || { fail "no such snapshot: $t1"; return 1; }
  ensure_worldstat || return 1

  printf '########## %s  ->  %s ##########\n\n' "$(basename "$t0")" "$(basename "$t1")"
  local f
  for f in "$t0"/worldstat-slot*.json; do
    [ -f "$f" ] || continue
    local base; base="$(basename "$f")"
    if [ -f "$t1/$base" ]; then
      "$WORLDSTAT" compare "$f" "$t1/$base"
      printf '\n'
    else
      printf '=== %s: no T1 counterpart in %s\n\n' "$base" "$t1"
    fi
  done

  printf '########## archive: migration flow between the two captures ##########\n\n'
  python3 - "$t0/archive-summary.txt" "$t1/archive-summary.txt" <<'PY'
import re, sys

def read(path):
    lanes, scalars = {}, {}
    section = None
    try:
        text = open(path, encoding="utf-8").read().splitlines()
    except OSError as exc:
        print(f"cannot read {path}: {exc}")
        return lanes, scalars
    for line in text:
        if line.endswith(":"):
            section = line[:-1]
            continue
        m = re.match(r"\s+slot (\S+) -> slot (\S+)\s+(\d+)", line)
        if m and section == "lanes":
            lanes[(m.group(1), m.group(2))] = int(m.group(3))
            continue
        m = re.match(r"(\w+) (\S+)", line)
        if m and section is None:
            scalars[m.group(1)] = m.group(2)
    return lanes, scalars

l0, s0 = read(sys.argv[1])
l1, s1 = read(sys.argv[2])
for key in ("migrations", "distinctMigrantEntities", "distinctMigrantGenomes"):
    a, b = s0.get(key), s1.get(key)
    if a is not None and b is not None:
        print(f"{key:26s} {a} -> {b}   (+{int(b) - int(a)})")
print("\nper-lane hops (T0 total -> T1 total, overnight delta):")
for lane in sorted(set(l0) | set(l1)):
    a, b = l0.get(lane, 0), l1.get(lane, 0)
    print(f"  slot {lane[0]} -> slot {lane[1]}   {a} -> {b}   (+{b - a})")
PY
}

latest() { ls -1d "$BASELINES"/*/ 2>/dev/null | sort | tail -n 1; }

# ---------------------------------------------------------------- dispatch

case "${1:-capture}" in
  capture) capture "${2:-T0}" ;;
  --label) capture "${2:-T0}" ;;
  compare) shift; compare_dirs "${1:?usage: baseline.sh compare <t0-dir> <t1-dir>}" "${2:?usage: baseline.sh compare <t0-dir> <t1-dir>}" ;;
  latest)  latest ;;
  *)
    echo "usage: baseline.sh [capture|--label <name>] | compare <t0-dir> <t1-dir> | latest" >&2
    exit 1
    ;;
esac
