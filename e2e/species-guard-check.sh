#!/usr/bin/env bash
# The regression check for the species-history save failure (SpeciesHistoryGuard).
#
#   species-guard-check.sh          run both halves
#   species-guard-check.sh off      only the unguarded half (the failure must reproduce)
#   species-guard-check.sh on       only the guarded half (the save must succeed)
#
# It runs ONE game instance at a time on a throwaway copy of a seeded world, so it needs
# the rig to be down (BepInEx hands out only five log files, and the mod does not run in a
# sixth instance — see run-m4-lan.sh's header). It never writes a rig world: the instance
# loads M5-GuardTest and saves to M5-GuardTestOut, with the periodic save and the save on
# quit both off.
#
# What it proves, in one run:
#
#   MULTIVERSE_SPECIES_GUARD=0   speciespoison -> save  ==>  IndexOutOfRangeException in
#                                Utility.LogLikeSpeciesDataArray.SavePointToArrays, i.e.
#                                the incident's own stack, on demand.
#   (guard armed, the default)   speciespoison -> save  ==>  OK, and the world on disk is a
#                                readable archive that still carries its species.
set -uo pipefail

E2E="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(cd "$E2E/.." && pwd)"
GAME_SH="$REPO/bibites-mod/game.sh"
BEPINEX="/mnt/c/Program Files (x86)/Steam/steamapps/common/The Bibites/BepInEx"
SAVES="/mnt/c/Users/j_jor/AppData/LocalLow/The Bibites/The Bibites/Savefiles"
WIN_TEMP='C:\Users\j_jor\AppData\Local\Temp'
WIN_CMD_DIR="$WIN_TEMP\\bibites-guard"
WSL_CMD_DIR="$(wslpath -u "$WIN_TEMP")/bibites-guard"
CMD_WSL="$WSL_CMD_DIR/cmd-guard.txt"
CMD_WIN="$WIN_CMD_DIR\\cmd-guard.txt"

SOURCE_WORLD="${SOURCE_WORLD:-M4-Slot1}"
TEST_WORLD=M5-GuardTest
OUT_WORLD=M5-GuardTestOut
INSTANCE=guardcheck
EVIDENCE="$E2E/logs-m5-guard"

mkdir -p "$WSL_CMD_DIR" "$EVIDENCE"

step()    { printf '\n== %s\n' "$*"; }
note()    { printf '   %s\n' "$*"; }
fail()    { printf '   FAIL: %s\n' "$*" >&2; }
now()     { date +%s; }

logfile() { "$GAME_SH" logfile "$INSTANCE" 2>/dev/null; }

send() {
  local verb="$1"; shift
  local args="$*" token hit deadline
  token="t$(date +%s%N)"
  printf '%s %s %s\n' "$token" "$verb" "$args" > "$CMD_WSL.tmp"
  mv -f "$CMD_WSL.tmp" "$CMD_WSL"
  deadline=$(( $(now) + ${CMD_TIMEOUT:-180} ))
  while :; do
    if [ -f "$CMD_WSL.log" ]; then
      hit="$(grep -a "^$token " "$CMD_WSL.log" 2>/dev/null | tail -n 1)"
      if [ -n "$hit" ]; then
        printf '%s\n' "$hit"
        case "$hit" in *" OK "*) return 0 ;; *) return 1 ;; esac
      fi
    fi
    [ "$(now)" -ge "$deadline" ] && { fail "timeout waiting for '$verb $args'"; return 2; }
    sleep 0.2
  done
}

wait_ready() {
  local deadline=$(( $(now) + ${READY_TIMEOUT:-600} )) f
  while [ "$(now)" -lt "$deadline" ]; do
    f="$(logfile)"
    if [ -n "$f" ] && [ -f "$f" ] \
       && grep -aqE "\[M2-WORLD\] world '$TEST_WORLD' (loaded|seeded and live)" "$f"; then
      note "world '$TEST_WORLD' is live in $f"
      return 0
    fi
    sleep 2
  done
  fail "world '$TEST_WORLD' never came up"
  return 1
}

start_instance() {
  local guard="$1"
  rm -f "$CMD_WSL" "$CMD_WSL.log" "$CMD_WSL.tmp"
  # The two halves load the same world under the same instance name, so the previous
  # half's log satisfies the readiness marker on sight. Clear the BepInEx logs first and
  # readiness can only be the run that is starting now.
  rm -f "$BEPINEX"/LogOutput.log "$BEPINEX"/LogOutput.log.[0-9] 2>/dev/null || true
  MULTIVERSE_WORLD="$TEST_WORLD" \
  MULTIVERSE_CMD_FILE="$CMD_WIN" \
  MULTIVERSE_SAVE_MINUTES=0 \
  MULTIVERSE_SAVE_ON_QUIT=0 \
  MULTIVERSE_SPECIES_GUARD="$guard" \
  WSLENV='MULTIVERSE_WORLD:MULTIVERSE_CMD_FILE:MULTIVERSE_SAVE_MINUTES:MULTIVERSE_SAVE_ON_QUIT:MULTIVERSE_SPECIES_GUARD' \
    "$GAME_SH" start "$INSTANCE" >/dev/null || return 1
}

stop_instance() {
  send quit >/dev/null 2>&1 || true
  sleep 4
  "$GAME_SH" stop "$INSTANCE" >/dev/null 2>&1 || true
  sleep 2
}

prepare_world() {
  if [ ! -f "$SAVES/$SOURCE_WORLD.zip" ]; then
    fail "no source world at $SAVES/$SOURCE_WORLD.zip"
    return 1
  fi
  cp -f "$SAVES/$SOURCE_WORLD.zip" "$SAVES/$TEST_WORLD.zip"
  rm -f "$SAVES/$OUT_WORLD.zip"
  note "throwaway world $TEST_WORLD.zip copied from $SOURCE_WORLD.zip ($(stat -c%s "$SAVES/$TEST_WORLD.zip") bytes)"
}

# ---- half 1: the failure, on demand, with the guard disabled ---------------------------
half_off() {
  step "HALF 1 — MULTIVERSE_SPECIES_GUARD=0: the poisoned array must break the world save"
  prepare_world || return 1
  start_instance 0 || return 1
  wait_ready || { stop_instance; return 1; }

  note "audit before: $(send speciesaudit || true)"
  local poison; poison="$(send speciespoison)" || { fail "speciespoison failed: $poison"; stop_instance; return 1; }
  note "poison: $poison"
  note "audit after: $(send speciesaudit || true)"

  local out rc
  out="$(send save "$OUT_WORLD")"; rc=$?
  note "save: $out"

  # The log is copied only after the process is gone: BepInEx buffers, and the tail that
  # carries the stack is not on disk while the game still holds the file.
  local f; f="$(logfile)"
  stop_instance
  cp -f "$f" "$EVIDENCE/guard-off.log" 2>/dev/null || true

  if [ "$rc" = 0 ]; then
    fail "the save SUCCEEDED with the guard off — the failure did not reproduce"
    return 1
  fi
  if ! grep -aq 'IndexOutOfRangeException' <<<"$out"; then
    fail "the save failed for some other reason: $out"
    return 1
  fi
  # 'at ...' and not the bare type name, so the guard's own DISABLED warning — which names
  # the same method — cannot satisfy the assertion.
  if ! grep -aq 'at Utility.LogLikeSpeciesDataArray.SavePointToArrays' "$EVIDENCE/guard-off.log"; then
    fail "the incident's stack frame is not in the log"
    return 1
  fi
  note "REPRODUCED — the stack from $EVIDENCE/guard-off.log:"
  grep -a -A 7 'world save exception' "$EVIDENCE/guard-off.log" | head -n 8 | sed 's/^/     /'
  return 0
}

# ---- half 2: the same poison, guarded --------------------------------------------------
half_on() {
  step "HALF 2 — guard at its default: the same poison must save cleanly"
  prepare_world || return 1
  start_instance 1 || return 1
  wait_ready || { stop_instance; return 1; }

  note "audit before: $(send speciesaudit || true)"
  local poison; poison="$(send speciespoison)" || { fail "speciespoison failed: $poison"; stop_instance; return 1; }
  note "poison: $poison"
  note "audit after (wouldOverrun must be 1): $(send speciesaudit || true)"

  local out rc
  out="$(send save "$OUT_WORLD")"; rc=$?
  note "save: $out"
  note "audit after the save (wouldOverrun must be 0): $(send speciesaudit || true)"

  local f; f="$(logfile)"
  stop_instance
  cp -f "$f" "$EVIDENCE/guard-on.log" 2>/dev/null || true

  if grep -aq 'at Utility.LogLikeSpeciesDataArray.SavePointToArrays' "$EVIDENCE/guard-on.log"; then
    fail "the guarded run still threw inside SavePointToArrays"
    return 1
  fi

  if [ "$rc" != 0 ]; then
    fail "the save FAILED with the guard armed: $out"
    return 1
  fi

  step "the world on disk"
  local zip="$SAVES/$OUT_WORLD.zip"
  if [ ! -f "$zip" ]; then fail "no $zip was written"; return 1; fi
  note "$zip ($(stat -c%s "$zip") bytes)"
  python3 - "$zip" <<'PY' || return 1
import json, sys, zipfile
z = zipfile.ZipFile(sys.argv[1])
names = z.namelist()
required = ["scene.bb8scene", "speciesData.json", "data.bin"]
missing = [n for n in required if n not in names]
if missing:
    print("   FAIL: the archive is missing", missing); sys.exit(1)
bad = z.testzip()
if bad is not None:
    print("   FAIL: corrupt entry", bad); sys.exit(1)

scene = json.loads(z.read("scene.bb8scene").decode("utf-8-sig"))
print(f"   scene.bb8scene: nBibites={scene.get('nBibites')} nPellets={scene.get('nPellets')} "
      f"simulatedTime={scene.get('simulatedTime')}")

# speciesData.json is GlobalLineageManager.SaveState(); data.bin carries the binary
# histories that GlobalLineageManager.SaveStateBin writes — the block that was throwing.
lineage = json.loads(z.read("speciesData.json").decode("utf-8-sig"))
recorded = lineage.get("recordedSpecies")
if not isinstance(recorded, list) or not recorded:
    print("   FAIL: speciesData.json carries no recordedSpecies"); sys.exit(1)
labels = [f"{s.get('genericName','?')} {s.get('specificName','?')}" for s in recorded if isinstance(s, dict)]
print(f"   speciesData.json: recordedSpecies={len(recorded)} activeSpeciesList={len(lineage.get('activeSpeciesList') or [])}"
      f" nextSpeciesID={lineage.get('nextSpeciesID')}")
print(f"   e.g. {labels[:3]}")
print(f"   the poisoned test species is {'PRESENT' if 'Poisoned testcase' in labels else 'ABSENT'}")
print(f"   data.bin: {z.getinfo('data.bin').file_size} bytes of binary history")
if z.getinfo("data.bin").file_size < 1024:
    print("   FAIL: data.bin is implausibly small"); sys.exit(1)
print("   archive OK")
PY

  # The decisive check: the game must be able to read back what it wrote.
  step "loading the saved world back"
  local saved_world="$TEST_WORLD"
  TEST_WORLD="$OUT_WORLD"
  start_instance 1 || { TEST_WORLD="$saved_world"; return 1; }
  if ! wait_ready; then TEST_WORLD="$saved_world"; stop_instance; return 1; fi
  local back; back="$(send speciesaudit)"
  note "reloaded: $back"
  f="$(logfile)"
  stop_instance
  cp -f "$f" "$EVIDENCE/guard-reload.log" 2>/dev/null || true
  TEST_WORLD="$saved_world"
  case "$back" in
    *" OK "*) ;;
    *) fail "the saved world did not reload cleanly"; return 1 ;;
  esac
  case "$back" in
    *wouldOverrun=0*) ;;
    *) fail "the reloaded world came back with an array that would overrun"; return 1 ;;
  esac

  note "PASS — the guarded save produced a complete, readable world that the game loads back"
  return 0
}

rc=0
case "${1:-both}" in
  off)  half_off || rc=1 ;;
  on)   half_on  || rc=1 ;;
  both) half_off || rc=1; half_on || rc=1 ;;
  *)    echo "usage: species-guard-check.sh [off|on|both]" >&2; exit 2 ;;
esac

step "RESULT"
if [ "$rc" = 0 ]; then note "species-guard regression check PASSED"; else fail "species-guard regression check FAILED"; fi
exit "$rc"
