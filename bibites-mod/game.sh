#!/usr/bin/env bash
# Control The Bibites on the Windows side from WSL: start | stop | status | log [n]
set -euo pipefail

GAME_WIN='C:\Program Files (x86)\Steam\steamapps\common\The Bibites\The Bibites.exe'
GAME_DIR="/mnt/c/Program Files (x86)/Steam/steamapps/common/The Bibites"
LOG="$GAME_DIR/BepInEx/LogOutput.log"
PS=/mnt/c/Windows/System32/WindowsPowerShell/v1.0/powershell.exe

case "${1:-status}" in
  start)
    # Start-Process detaches the game from this shell's process tree, so the
    # game survives when the WSL command exits.
    cd /mnt/c && "$PS" -NoProfile -Command "Start-Process -FilePath '$GAME_WIN'" >/dev/null
    echo "started"
    ;;
  stop)
    cd /mnt/c && "$PS" -NoProfile -Command "Stop-Process -Name 'The Bibites' -ErrorAction SilentlyContinue" >/dev/null
    echo "stopped"
    ;;
  status)
    cd /mnt/c && (/mnt/c/Windows/System32/tasklist.exe | grep -i "The Bibites" || echo "not running")
    ;;
  log)
    tail -n "${2:-40}" "$LOG"
    ;;
  *)
    echo "usage: game.sh start|stop|status|log [n]" >&2
    exit 1
    ;;
esac
