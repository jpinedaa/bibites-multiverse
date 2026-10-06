#!/bin/sh
# Refuse to open the relay until the archive has finished its startup replay and
# bound its health endpoint. systemd orders the units, but Type=simple becomes
# active at exec; this probe is the readiness boundary that ordering alone lacks.
set -eu

archive_http=${MULTIVERSE_ARCHIVE_HTTP:-127.0.0.1:8796}
timeout=${MULTIVERSE_ARCHIVE_READY_TIMEOUT:-900}
interval=${MULTIVERSE_ARCHIVE_READY_INTERVAL:-2}

case "$timeout:$interval" in
  *[!0-9:]*|0:*|*:0) echo "wait-for-archive: timeout and interval must be positive whole seconds" >&2; exit 2 ;;
esac

elapsed=0
started=$(date +%s)
deadline=$((started + timeout))
while :; do
  now=$(date +%s)
  remaining=$((deadline - now))
  [ "$remaining" -gt 0 ] || break
  probe_timeout=$interval
  [ "$probe_timeout" -le "$remaining" ] || probe_timeout=$remaining
  if curl -fsS --max-time "$probe_timeout" "http://$archive_http/healthz" >/dev/null 2>&1; then
    elapsed=$(($(date +%s) - started))
    echo "wait-for-archive: archive ready at $archive_http after ${elapsed}s"
    exit 0
  fi
  now=$(date +%s)
  remaining=$((deadline - now))
  [ "$remaining" -gt 0 ] || break
  nap=$interval
  [ "$nap" -le "$remaining" ] || nap=$remaining
  sleep "$nap"
done

echo "wait-for-archive: archive did not become ready at $archive_http within ${timeout}s; relay remains stopped" >&2
exit 1
