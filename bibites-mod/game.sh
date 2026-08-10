#!/usr/bin/env bash
# Control The Bibites on the Windows side from WSL, one to five instances at a time.
#
#   game.sh start   [instance]        launch and record the Windows PID
#   game.sh stop    [instance|all]    stop one instance by PID; no argument stops every instance
#   game.sh status                    every known instance, its PID and whether it is alive
#   game.sh log     [instance] [n]    tail that instance's BepInEx log
#   game.sh logfile [instance]        print the log file that belongs to that instance
#   game.sh pid     [instance]        print the recorded Windows PID
#   game.sh wait    [instance] [s]    block until the process exits, or until the timeout
#
# The old single-instance forms still work: `game.sh start`, `game.sh stop`,
# `game.sh status`, `game.sh log 60`.
#
# Two facts from dev_environment.md shape this script:
#
#   * Every instance shares one plugins/ DLL and one config/ directory, so a per-instance
#     setting can only arrive through the environment — and WSLENV must name every
#     variable, or WSL forwards none of them.
#   * BepInEx gives the FIRST process to start `LogOutput.log` and each later one the
#     next free `LogOutput.log.N` (N = 1..4, then it gives up), because the earlier
#     process holds a lock. Which instance owns which file is therefore a race, not a
#     rule. This script never assumes; it globs every candidate and greps each for the
#     instance's own marker, which the mod prints once at startup on its
#     `[M2] config:` line. The M3 ring runs three instances, so `.log.2` is routine.
set -euo pipefail

GAME_WIN='C:\Program Files (x86)\Steam\steamapps\common\The Bibites\The Bibites.exe'
GAME_DIR="/mnt/c/Program Files (x86)/Steam/steamapps/common/The Bibites"
LOG_DIR="$GAME_DIR/BepInEx"
PS=/mnt/c/Windows/System32/WindowsPowerShell/v1.0/powershell.exe
TASKLIST=/mnt/c/Windows/System32/tasklist.exe

# Where the per-instance PID and log marker live. Outside the repo on purpose: it is
# runtime state, not source.
RUN_DIR="${BIBITES_RUN_DIR:-$HOME/.cache/bibites-multiverse/instances}"
mkdir -p "$RUN_DIR"

DEFAULT_INSTANCE=main

# ---------------------------------------------------------------- helpers

# Both interop calls read stdin, so both need it closed: `stop all` drives them from
# inside a `while read` loop, and a child that swallows the loop's input stream silently
# stops the loop after its first iteration — which stops one instance and leaves the other.
ps_run() { cd /mnt/c && "$PS" -NoProfile -NonInteractive -Command "$1" </dev/null; }

instances() {
  local f name
  for f in "$RUN_DIR"/*.pid; do
    [ -e "$f" ] || continue
    name="$(basename "$f" .pid)"
    printf '%s\n' "$name"
  done
}

pid_of() {
  local instance="$1"
  [ -f "$RUN_DIR/$instance.pid" ] && cat "$RUN_DIR/$instance.pid" || true
}

alive() {
  local pid="$1"
  [ -n "$pid" ] || return 1
  "$TASKLIST" /FI "PID eq $pid" /NH 2>/dev/null </dev/null | grep -qi 'Bibites'
}

# The marker that identifies this instance in a BepInEx log. Recorded at start from the
# environment the instance was launched with.
marker_for_env() {
  if [ -n "${MULTIVERSE_RING_SLOT:-}" ]; then
    printf '\\[M2\\] config: .*ringSlot=%s ' "$MULTIVERSE_RING_SLOT"
  elif [ -n "${MULTIVERSE_WORLD:-}" ]; then
    printf 'MULTIVERSE_WORLD=%s' "$MULTIVERSE_WORLD"
  else
    printf ''
  fi
}

# Resolve instance -> log file by content, never by start order.
logfile_for() {
  local instance="$1"
  local marker=""
  [ -f "$RUN_DIR/$instance.marker" ] && marker="$(cat "$RUN_DIR/$instance.marker")"

  # BepInEx falls back through LogOutput.log.1 .. .4, so glob rather than list: with three
  # game instances the third owns LogOutput.log.2 and a two-entry list never finds it.
  local candidates=() f
  for f in "$LOG_DIR/LogOutput.log" "$LOG_DIR"/LogOutput.log.[0-9]; do
    if [ -f "$f" ]; then candidates+=("$f"); fi
  done

  if [ "${#candidates[@]}" -eq 0 ]; then
    echo "no BepInEx log found under $LOG_DIR" >&2
    return 1
  fi

  if [ -n "$marker" ]; then
    for f in "${candidates[@]}"; do
      if grep -qE "$marker" "$f" 2>/dev/null; then
        printf '%s\n' "$f"
        return 0
      fi
    done
    echo "no BepInEx log matches instance '$instance' (marker: $marker)" >&2
    return 1
  fi

  printf '%s\n' "${candidates[0]}"
  return 0
}

# ---------------------------------------------------------------- commands

cmd_start() {
  local instance="${1:-$DEFAULT_INSTANCE}"

  local existing
  existing="$(pid_of "$instance")"
  if [ -n "$existing" ] && alive "$existing"; then
    echo "instance '$instance' is already running (pid $existing)" >&2
    return 1
  fi

  # BIBITES_EXTRA_ARGS (e.g. `-batchmode -nographics`) is this script's own knob and is
  # read here, in WSL — so it is the one per-instance setting that must NOT be named in
  # WSLENV. That channel exists to reach the game process; this one never leaves the
  # shell, it only shapes the command line below. A token is a bare flag, with no space
  # and no quote in it, which is what makes one pair of single quotes each enough.
  local extra="" tok tokens=()
  read -r -a tokens <<<"${BIBITES_EXTRA_ARGS:-}"
  for tok in "${tokens[@]}"; do extra+="${extra:+,}'$tok'"; done
  [ -z "$extra" ] || extra=" -ArgumentList $extra"

  # Start-Process detaches the game from this shell's process tree, so the game
  # survives when the WSL command exits. -PassThru is what gives us the PID, which is
  # the only way to stop one instance without stopping the other.
  local pid
  pid="$(ps_run "(Start-Process -FilePath '$GAME_WIN'$extra -PassThru).Id" | tr -d '\r' | tail -n 1)"

  if ! [[ "$pid" =~ ^[0-9]+$ ]]; then
    echo "Start-Process returned no usable PID: '$pid'" >&2
    return 1
  fi

  printf '%s\n' "$pid" > "$RUN_DIR/$instance.pid"
  marker_for_env > "$RUN_DIR/$instance.marker"
  echo "started instance='$instance' pid=$pid${extra:+ args='$BIBITES_EXTRA_ARGS'}"
}

cmd_stop() {
  local target="${1:-all}"

  if [ "$target" = all ]; then
    local any=0 name
    while read -r name; do
      [ -n "$name" ] || continue
      any=1
      cmd_stop "$name"
    done < <(instances)

    # Anything left that no instance file claims — an orphan from an earlier run.
    ps_run "Stop-Process -Name 'The Bibites' -ErrorAction SilentlyContinue" >/dev/null || true
    [ "$any" = 1 ] || echo "stopped (by name; no recorded instances)"
    return 0
  fi

  local pid
  pid="$(pid_of "$target")"
  if [ -z "$pid" ]; then
    echo "no recorded pid for instance '$target'" >&2
    return 1
  fi

  ps_run "Stop-Process -Id $pid -Force -ErrorAction SilentlyContinue" >/dev/null || true
  rm -f "$RUN_DIR/$target.pid"
  echo "stopped instance='$target' pid=$pid"
}

cmd_status() {
  local found=0 name pid state
  while read -r name; do
    [ -n "$name" ] || continue
    found=1
    pid="$(pid_of "$name")"
    if alive "$pid"; then state=running; else state=gone; fi
    printf 'instance=%s pid=%s state=%s log=%s\n' \
      "$name" "$pid" "$state" "$(logfile_for "$name" 2>/dev/null || echo '<unresolved>')"
  done < <(instances)

  [ "$found" = 1 ] || echo "no recorded instances"

  echo "--- tasklist ---"
  cd /mnt/c && ("$TASKLIST" | grep -i "The Bibites" || echo "not running")
}

cmd_log() {
  local instance="$DEFAULT_INSTANCE" lines=40

  # `game.sh log 60` keeps working: a bare number is a line count.
  if [[ "${1:-}" =~ ^[0-9]+$ ]]; then
    lines="$1"
  else
    [ -n "${1:-}" ] && instance="$1"
    [ -n "${2:-}" ] && lines="$2"
  fi

  local file
  file="$(logfile_for "$instance")" || return 1
  tail -n "$lines" "$file"
}

cmd_wait() {
  local instance="${1:-$DEFAULT_INSTANCE}"
  local timeout="${2:-60}"
  local pid deadline
  pid="$(pid_of "$instance")"
  [ -n "$pid" ] || { echo "no recorded pid for instance '$instance'" >&2; return 1; }

  deadline=$(( $(date +%s) + timeout ))
  while alive "$pid"; do
    if [ "$(date +%s)" -ge "$deadline" ]; then
      echo "instance='$instance' pid=$pid still running after ${timeout}s" >&2
      return 1
    fi
    sleep 1
  done

  rm -f "$RUN_DIR/$instance.pid"
  echo "instance='$instance' pid=$pid exited"
}

case "${1:-status}" in
  start)   shift; cmd_start "$@" ;;
  stop)    shift; cmd_stop "$@" ;;
  status)  cmd_status ;;
  log)     shift; cmd_log "$@" ;;
  logfile) shift; logfile_for "${1:-$DEFAULT_INSTANCE}" ;;
  pid)     shift; pid_of "${1:-$DEFAULT_INSTANCE}" ;;
  wait)    shift; cmd_wait "$@" ;;
  *)
    echo "usage: game.sh start|stop|status|log|logfile|pid|wait [instance] [n]" >&2
    exit 1
    ;;
esac
