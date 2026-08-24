#!/usr/bin/env bash
# Publish "is anyone watching the broadcast?" as one small JSON document.
#
# WHY THIS EXISTS. The publisher costs approximately 780 GB of inbound transfer
# each month whether or not a single person is watching, which is a quarter of
# the whole allowance spent on an empty room (deploy/SIZING.md, "Network
# transfer"). The fix is to publish only while someone watches. The publisher is
# OBS on a Windows machine that this host cannot reach and holds no credential
# for, so mediamtx's own `runOnDemand` cannot start it: the Windows side polls
# instead, and this file is what it polls.
#
# THE CONTRACT, which a Windows watcher and docs/live-broadcast.md both depend on:
#
#   GET https://<domain>/api/viewers
#   {"watching":false,"hlsSessions":0,"lastViewerRequestAgeSec":null,
#    "asOf":"2026-08-16T21:04:00Z","traffic":{...}}
#
#   watching = hlsSessions > 0 || lastViewerRequestAgeSec <= 60
#
# The audience fields remain the publisher-control contract. `traffic` is a
# fixed public aggregate for the health dashboard. It contains no client
# address, raw request path, or query string.
#
# TWO SOURCES, AND WHY NEITHER IS ENOUGH ALONE.
#
#   hls_sessions from mediamtx counts players that reached the stream. It is
#   exactly zero whenever the publisher is offline, because there is no stream to
#   have a session with — which is the whole state this signal has to detect.
#
#   The nginx access log therefore carries the other half: a browser on /watch
#   polls /stream/bibites/index.m3u8 every five seconds and gets 404 while the
#   publisher is down. THAT 404 IS THE SIGNAL. Status code is deliberately
#   ignored; a request that failed still proves a person is waiting.
#
# LOOPBACK IS NOT AN AUDIENCE. Our own health probes reach the front door over
# loopback, and counting them would hold the publisher up forever with nobody
# watching. They are dropped by source address.
#
# Read seams for deploy/test-viewers-presence.sh: MV_ACCESS_LOG, MV_METRICS_URL
# (an http URL or a file), MV_NOW (epoch seconds), MV_OUT.
set -euo pipefail

# This default is MV_NGINX_LOGDIR/front-door.access.log, which is where
# deploy/nginx/multiverse-20-status.conf writes and NOT /var/log/nginx. Three
# places carry that directory and all three must move together: the nginx
# template, this default, and ReadOnlyPaths= in
# deploy/systemd/multiverse-viewers.service. deploy/test-units.sh reads this
# line and checks the unit against it, so the drift fails a test rather than
# an empty document on a host.
ACCESS_LOG="${MV_ACCESS_LOG:-/var/log/multiverse/nginx/front-door.access.log}"
METRICS="${MV_METRICS_URL:-http://127.0.0.1:9998/metrics}"
OUT="${MV_OUT:-/var/www/multiverse-status/viewers.json}"
# Sixty seconds is six polls of the /watch page's own five-second playlist check,
# so a viewer stays "watching" across a handful of lost requests.
WINDOW="${MV_VIEWER_WINDOW_SEC:-60}"
# The log is scanned from its tail, not from its head, so this costs the same on
# a fresh file and on a day-old one. One viewer produces roughly a hundred lines
# a minute, so three thousand lines covers the sixty-second window with room for
# a crowd. A qualifying request older than this window reads as no request, which
# is the same answer the window itself gives.
TAIL_LINES="${MV_TAIL_LINES:-3000}"
TRAFFIC_WINDOW="${MV_TRAFFIC_WINDOW_SEC:-60}"

now="${MV_NOW:-$(date -u +%s)}"

# ----------------------------------------------------------------- mediamtx

metrics_text=''
case "$METRICS" in
  http://*|https://*)
    metrics_text="$(curl -fsS --max-time 3 "$METRICS" 2>/dev/null || true)"
    ;;
  *)
    [ -r "$METRICS" ] && metrics_text="$(cat -- "$METRICS")" || metrics_text=''
    ;;
esac

# `hls_sessions` and not `hls_sessions_outbound_bytes`, which is why the field is
# matched whole. An unreachable origin reads as zero sessions rather than as an
# error: the access-log half of the signal still answers, and the failure
# direction that matters is never claiming an audience that is not there.
sessions="$(printf '%s\n' "$metrics_text" |
  awk '$1=="hls_sessions" && NF==2 {v=$2} END{ if (v=="") v=0; printf "%d", v+0 }')"

# ----------------------------------------------------------------- access log

# The `combined` format, which is what the front door logs:
#   $remote_addr - $remote_user [$time_local] "$request" $status ...
#        $1                          $4  $5    $6  $7            $9
# so $4 is `[16/Aug/2026:21:03:41`, $5 is `+0000]` and $7 is the path.
#
# The log is buffered with a ten-second flush, so a fresh request can be up to
# ten seconds late here. That is inside the sixty-second window and does not
# change any answer; it does mean this file is never a millisecond-accurate
# clock and must not be read as one.
last_stamp=''
log_tail=''
if [ -r "$ACCESS_LOG" ]; then
  log_tail="$(tail -n "$TAIL_LINES" -- "$ACCESS_LOG" 2>/dev/null || true)"
  last_stamp="$(printf '%s\n' "$log_tail" | awk '
    $1 == "127.0.0.1" || $1 == "::1" || $1 == "localhost" { next }
    NF >= 7 {
      if (index($7, "/watch") == 1 || index($7, "/stream/") == 1) {
        stamp = $4; sub(/^\[/, "", stamp)
        zone  = $5; sub(/\]$/, "", zone)
        last = stamp " " zone
      }
    }
    END { if (last != "") print last }
  ' || true)"
fi

age=null
if [ -n "$last_stamp" ]; then
  # `16/Aug/2026:21:03:41 +0000` is not a format `date` reads. Turning the three
  # date separators into spaces makes it one. One conversion for one line, so no
  # per-line `date` call: the whole log is scanned by awk, which has no portable
  # mktime on this host's mawk.
  day="${last_stamp%%:*}"
  clock="${last_stamp#*:}"
  epoch="$(date -u -d "$(printf '%s' "$day" | tr '/' ' ') $clock" +%s 2>/dev/null || true)"
  case "$epoch" in
    ''|*[!0-9]*) ;;
    *)
      age=$(( now - epoch ))
      # A buffered flush or a small clock step can put a stamp in the future.
      # Zero is the honest reading of that, not a negative age.
      [ "$age" -ge 0 ] || age=0
      ;;
  esac
fi

# ------------------------------------------------------------ HTTP traffic

# New front-door rows keep the combined-log field positions and finish with:
#
#   mv_msec=<Unix seconds> mv_request_time=<seconds>
#
# The numeric timestamp permits a bounded exact window with no per-row `date`
# process. The request duration is measured at nginx. It is not an end-to-end
# browser measurement and excludes DNS, connection setup, and browser rendering.
traffic_rows="$(printf '%s\n' "$log_tail" | awk -v cutoff="$(( now - TRAFFIC_WINDOW ))" '
  function route(path) {
    if (index(path, "/contract-b/") == 1 || path == "/api/enroll") return "relay"
    if (index(path, "/stream/") == 1) return "stream"
    if (index(path, "/api/") == 1 || path == "/healthz") return "api"
    return "pages"
  }
  {
    if (NF < 2) next
    msec = ""; request_time = ""
    for (i = NF - 1; i <= NF; i++) {
      if ($i ~ /^mv_msec=[0-9]+([.][0-9]+)?$/) {
        msec = $i; sub(/^mv_msec=/, "", msec)
      }
      if ($i ~ /^mv_request_time=[0-9]+([.][0-9]+)?$/) {
        request_time = $i; sub(/^mv_request_time=/, "", request_time)
      }
    }
    if (msec == "" || request_time == "") next
    timed++
    if (oldest == "" || msec < oldest) oldest = msec
    if (newest == "" || msec > newest) newest = msec
    if (msec < cutoff) next
    if ($1 == "127.0.0.1" || $1 == "::1" || $1 == "localhost") next
    if ($9 !~ /^[0-9][0-9][0-9]$/) next
    printf "R\t%s\t%s\t%.3f\n", route($7), $9, request_time * 1000
  }
  END { printf "M\t%d\t%s\t%s\n", timed + 0, oldest, newest }
' || true)"

traffic_json="$(printf '%s\n' "$traffic_rows" | jq -Rsc \
  --argjson cutoff "$(( now - TRAFFIC_WINDOW ))" \
  --argjson window "$TRAFFIC_WINDOW" '
  def percentile(values; q):
    (values | sort) as $sorted
    | ($sorted | length) as $count
    | if $count == 0 then null
      else $sorted[([(($count * q) | ceil) - 1, 0] | max)]
      end;
  (split("\n") | map(select(length > 0) | split("\t"))) as $rows
  | ($rows | map(select(.[0] == "M")) | first // ["M", "0", "", ""]) as $meta
  | ($rows | map(select(.[0] == "R") | {
      route: .[1], status: (.[2] | tonumber), durationMs: (.[3] | tonumber)
    })) as $requests
  | def summary($id):
      ($requests | map(select(.route == $id))) as $route
      | {
          id: $id,
          requests: ($route | length),
          status1xx: ($route | map(select(.status >= 100 and .status < 200)) | length),
          status2xx: ($route | map(select(.status >= 200 and .status < 300)) | length),
          status3xx: ($route | map(select(.status >= 300 and .status < 400)) | length),
          status4xx: ($route | map(select(.status >= 400 and .status < 500)) | length),
          status5xx: ($route | map(select(.status >= 500 and .status < 600)) | length),
          p50Ms: percentile(($route | map(select(.status != 101) | .durationMs)); 0.50),
          p95Ms: percentile(($route | map(select(.status != 101) | .durationMs)); 0.95)
        };
  ($meta[1] | tonumber) as $timed
  | ($meta[2] | tonumber? // null) as $oldest
  | ($meta[3] | tonumber? // null) as $newest
  | {
      available: ($timed > 0),
      complete: ($timed > 0 and $oldest != null and $oldest <= $cutoff and
                 $newest != null and $newest >= $cutoff),
      windowSeconds: $window,
      requests: ($requests | length),
      status1xx: ($requests | map(select(.status >= 100 and .status < 200)) | length),
      status2xx: ($requests | map(select(.status >= 200 and .status < 300)) | length),
      status3xx: ($requests | map(select(.status >= 300 and .status < 400)) | length),
      status4xx: ($requests | map(select(.status >= 400 and .status < 500)) | length),
      status5xx: ($requests | map(select(.status >= 500 and .status < 600)) | length),
      p50Ms: percentile(($requests | map(select(.status != 101) | .durationMs)); 0.50),
      p95Ms: percentile(($requests | map(select(.status != 101) | .durationMs)); 0.95),
      routes: [summary("pages"), summary("api"), summary("relay"), summary("stream")]
    }
')" || exit 1

# ----------------------------------------------------------------- the answer

watching=false
if [ "$sessions" -gt 0 ]; then
  watching=true
elif [ "$age" != null ] && [ "$age" -le "$WINDOW" ]; then
  watching=true
fi

as_of="$(date -u -d "@$now" +%Y-%m-%dT%H:%M:%SZ)"

# One rename, so a reader never sees a half-written document. The temporary file
# is in the destination directory because a rename across filesystems is a copy.
out_dir="$(dirname -- "$OUT")"
[ -d "$out_dir" ] || { printf 'STOP: %s does not exist\n' "$out_dir" >&2; exit 1; }
tmp="$(mktemp "$out_dir/.viewers.json.XXXXXX")"
trap 'rm -f "$tmp"' EXIT
printf '{"watching":%s,"hlsSessions":%d,"lastViewerRequestAgeSec":%s,"asOf":"%s","traffic":%s}\n' \
  "$watching" "$sessions" "$age" "$as_of" "$traffic_json" >"$tmp"
# mktemp creates at 0600 and nginx reads this as www-data.
chmod 0644 "$tmp"
mv -f "$tmp" "$OUT"
trap - EXIT
