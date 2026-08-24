#!/usr/bin/env bash
# Drive viewers-presence.sh against a fake access log, fake metrics and a fixed
# clock. No root, no network, no nginx, no mediamtx, no host.
#
# WHAT IS BEING PROVED. The publisher stops while this says nobody is watching,
# so a false negative takes the broadcast off the air in front of a person who is
# looking at it, and a false positive spends 780 GB a month on an empty room.
# Each case below is one of the ways this has to be got right:
#
#   1. Silence is silence.                     no requests   -> false, null age
#   2. A FAILED request still proves presence.  404 on /stream -> true
#   3. Our own probes are not an audience.      loopback      -> ignored
#   4. A player that reached the stream counts. hls_sessions  -> true
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT="$HERE/viewers-presence.sh"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/multiverse-viewers.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT

# 2026-08-16T21:00:00Z. Every stamp below is written against this one clock.
NOW=1786914000
mkdir -p "$TMP/out"

fail() { echo "test-viewers-presence: $1" >&2; exit 1; }

# The `combined` format nginx writes, with the timestamp `n` seconds before NOW.
line() { # addr seconds-ago "request" status
  local addr="$1" ago="$2" request="$3" status="$4"
  printf '%s - - [%s] "%s" %s 512 "-" "probe"\n' \
    "$addr" "$(date -u -d "@$(( NOW - ago ))" '+%d/%b/%Y:%H:%M:%S +0000')" \
    "$request" "$status"
}

timed_line() { # addr seconds-ago "request" status request-seconds
  local addr="$1" ago="$2" request="$3" status="$4" request_seconds="$5"
  printf '%s - - [%s] "%s" %s 512 "-" "probe" mv_msec=%s.000 mv_request_time=%s\n' \
    "$addr" "$(date -u -d "@$(( NOW - ago ))" '+%d/%b/%Y:%H:%M:%S +0000')" \
    "$request" "$status" "$(( NOW - ago ))" "$request_seconds"
}

metrics() { # count
  printf '# HLS sessions\nhls_sessions %s\nhls_sessions_outbound_bytes 918273\n' "$1"
  printf 'hls_muxers{name="bibites"} 1\n'
}

run_case() { # name log-file metrics-file
  MV_NOW="$NOW" \
  MV_ACCESS_LOG="$2" \
  MV_METRICS_URL="$3" \
  MV_OUT="$TMP/out/viewers.json" \
    bash "$SCRIPT" || fail "$1: the script exited non-zero"
  cat "$TMP/out/viewers.json"
}

field() { # json key
  printf '%s' "$1" | sed -n "s/.*\"$2\":\\([^,}]*\\).*/\\1/p"
}

expect() { # name key got want
  [ "$3" = "$4" ] || fail "$1: $2 = $3, want $4"
}

# ------------------------------------------------------------------ case 1
# An empty room. Nothing has asked for the broadcast, so nothing is watching and
# there is no age to report — `null`, not `0`, because zero would read as "a
# request arrived this second" and is the exact opposite answer.
: >"$TMP/quiet.log"
metrics 0 >"$TMP/m0.txt"
out="$(run_case 'quiet' "$TMP/quiet.log" "$TMP/m0.txt")"
expect quiet watching                 "$(field "$out" watching)" false
expect quiet hlsSessions              "$(field "$out" hlsSessions)" 0
expect quiet lastViewerRequestAgeSec  "$(field "$out" lastViewerRequestAgeSec)" null
case "$out" in
  *'"asOf":"2026-08-16T21:00:00Z"'*) ;;
  *) fail "quiet: asOf is not the fixed clock in UTC ISO-8601: $out" ;;
esac

# ------------------------------------------------------------------ case 2
# THE CASE THIS SIGNAL EXISTS FOR. The publisher is offline, so mediamtx has zero
# sessions and every playlist request 404s. A person is nonetheless sitting on
# /watch waiting for it to start, and the 404 is the only trace they leave.
{
  line 203.0.113.14 41 'GET /watch HTTP/1.1' 200
  line 203.0.113.14 20 'GET /stream/bibites/index.m3u8 HTTP/1.1' 404
} >"$TMP/waiting.log"
out="$(run_case 'waiting' "$TMP/waiting.log" "$TMP/m0.txt")"
expect waiting watching                "$(field "$out" watching)" true
expect waiting hlsSessions             "$(field "$out" hlsSessions)" 0
expect waiting lastViewerRequestAgeSec "$(field "$out" lastViewerRequestAgeSec)" 20

# The same request, older than the window, is no longer an audience. The age is
# still reported, because "nobody for four minutes" and "nobody ever" are
# different facts to a watcher deciding whether to stop.
{
  line 203.0.113.14 240 'GET /stream/bibites/index.m3u8 HTTP/1.1' 404
} >"$TMP/stale.log"
out="$(run_case 'stale' "$TMP/stale.log" "$TMP/m0.txt")"
expect stale watching                "$(field "$out" watching)" false
expect stale lastViewerRequestAgeSec "$(field "$out" lastViewerRequestAgeSec)" 240

# ------------------------------------------------------------------ case 3
# OUR OWN PROBES ARE NOT AN AUDIENCE. monitor.sh and the verification phase reach
# the front door over loopback. Counting them would hold the publisher up for
# ever with nobody watching, which is the whole cost this mechanism removes.
{
  line 127.0.0.1 5 'GET /watch HTTP/1.1' 200
  line ::1       3 'GET /stream/bibites/index.m3u8 HTTP/1.1' 200
} >"$TMP/loopback.log"
out="$(run_case 'loopback' "$TMP/loopback.log" "$TMP/m0.txt")"
expect loopback watching                "$(field "$out" watching)" false
expect loopback lastViewerRequestAgeSec "$(field "$out" lastViewerRequestAgeSec)" null

# A loopback probe must not mask a real viewer that is older in the file either.
{
  line 203.0.113.14 30 'GET /stream/bibites/index.m3u8 HTTP/1.1' 200
  line 127.0.0.1     1 'GET /watch HTTP/1.1' 200
} >"$TMP/mixed.log"
out="$(run_case 'mixed' "$TMP/mixed.log" "$TMP/m0.txt")"
expect mixed watching                "$(field "$out" watching)" true
expect mixed lastViewerRequestAgeSec "$(field "$out" lastViewerRequestAgeSec)" 30

# Another public path is not the broadcast. A reader of /live or /api/status is
# on the map, not on the camera, and must not hold the publisher up.
{
  line 203.0.113.14 5 'GET /api/status HTTP/1.1' 200
  line 203.0.113.14 4 'GET /live HTTP/1.1' 200
} >"$TMP/console.log"
out="$(run_case 'console' "$TMP/console.log" "$TMP/m0.txt")"
expect console watching                "$(field "$out" watching)" false
expect console lastViewerRequestAgeSec "$(field "$out" lastViewerRequestAgeSec)" null

# ------------------------------------------------------------------ case 4
# A player that reached the stream is watching whatever the log says. This is the
# steady state: the publisher is up, mediamtx counts the session, and the answer
# must not depend on a log line arriving through a ten-second buffer flush.
metrics 1 >"$TMP/m1.txt"
out="$(run_case 'session' "$TMP/quiet.log" "$TMP/m1.txt")"
expect session watching                "$(field "$out" watching)" true
expect session hlsSessions             "$(field "$out" hlsSessions)" 1
expect session lastViewerRequestAgeSec "$(field "$out" lastViewerRequestAgeSec)" null

# `hls_sessions_outbound_bytes` is a different metric with the same prefix, and
# reading it as the session count would report an audience for ever.
printf 'hls_sessions 0\nhls_sessions_outbound_bytes 918273\n' >"$TMP/m-prefix.txt"
out="$(run_case 'prefix' "$TMP/quiet.log" "$TMP/m-prefix.txt")"
expect prefix hlsSessions "$(field "$out" hlsSessions)" 0
expect prefix watching    "$(field "$out" watching)" false

# ------------------------------------------------------------ HTTP traffic
# One timed row before the window proves that the bounded tail covers the full
# minute. Loopback traffic is a monitor probe, not public demand, and stays out
# of every total.
{
  timed_line 203.0.113.14 65 'GET /old HTTP/1.1' 200 0.010
  timed_line 203.0.113.14 50 'GET / HTTP/1.1' 200 0.012
  timed_line 203.0.113.14 40 'GET /api/status?token=top-secret HTTP/1.1' 500 0.100
  timed_line 203.0.113.14 30 'GET /contract-b/ HTTP/1.1' 101 0.250
  timed_line 203.0.113.14 20 'GET /stream/bibites/index.m3u8 HTTP/1.1' 404 0.040
  timed_line 127.0.0.1     10 'GET /healthz HTTP/1.1' 200 0.900
} >"$TMP/traffic.log"
out="$(run_case 'traffic' "$TMP/traffic.log" "$TMP/m0.txt")"
expect traffic available "$(printf '%s' "$out" | jq -r '.traffic.available')" true
expect traffic complete "$(printf '%s' "$out" | jq -r '.traffic.complete')" true
expect traffic requests "$(printf '%s' "$out" | jq -r '.traffic.requests')" 4
expect traffic status1xx "$(printf '%s' "$out" | jq -r '.traffic.status1xx')" 1
expect traffic status5xx "$(printf '%s' "$out" | jq -r '.traffic.status5xx')" 1
expect traffic p50Ms "$(printf '%s' "$out" | jq -r '.traffic.p50Ms')" 40.000
expect traffic p95Ms "$(printf '%s' "$out" | jq -r '.traffic.p95Ms')" 100.000
expect traffic api-p95 "$(printf '%s' "$out" | jq -r '.traffic.routes[] | select(.id == "api") | .p95Ms')" 100.000
expect traffic relay-p95 "$(printf '%s' "$out" | jq -r '.traffic.routes[] | select(.id == "relay") | .p95Ms')" null
expect traffic stream-4xx "$(printf '%s' "$out" | jq -r '.traffic.routes[] | select(.id == "stream") | .status4xx')" 1
if printf '%s' "$out" | grep -Eq '203\.0\.113\.14|top-secret|/api/status'; then
  fail "traffic: the public document disclosed an address or raw request path"
fi

# A timed format that has not covered one full window is available but partial.
# The dashboard must not read its partial counters as a full minute.
timed_line 203.0.113.14 20 'GET / HTTP/1.1' 200 0.010 >"$TMP/traffic-partial.log"
out="$(run_case 'traffic-partial' "$TMP/traffic-partial.log" "$TMP/m0.txt")"
expect traffic-partial available "$(printf '%s' "$out" | jq -r '.traffic.available')" true
expect traffic-partial complete "$(printf '%s' "$out" | jq -r '.traffic.complete')" false

# An old timed row proves the format existed once, not that the current window
# is observable. A stale log must stay unknown instead of reporting zero demand.
timed_line 203.0.113.14 65 'GET /old HTTP/1.1' 200 0.010 >"$TMP/traffic-stale.log"
out="$(run_case 'traffic-stale' "$TMP/traffic-stale.log" "$TMP/m0.txt")"
expect traffic-stale available "$(printf '%s' "$out" | jq -r '.traffic.available')" true
expect traffic-stale complete "$(printf '%s' "$out" | jq -r '.traffic.complete')" false

# ------------------------------------------------------------------ degraded
# An unreachable metrics endpoint reads as zero sessions rather than as an error.
# The access-log half of the signal still answers, and the direction that matters
# is never claiming an audience that is not there.
out="$(run_case 'no-metrics' "$TMP/waiting.log" "$TMP/does-not-exist.txt")"
expect no-metrics hlsSessions "$(field "$out" hlsSessions)" 0
expect no-metrics watching    "$(field "$out" watching)" true

# A missing access log is the same rule: no evidence is not evidence of presence.
out="$(run_case 'no-log' "$TMP/does-not-exist.log" "$TMP/m0.txt")"
expect no-log watching                "$(field "$out" watching)" false
expect no-log lastViewerRequestAgeSec "$(field "$out" lastViewerRequestAgeSec)" null

# ------------------------------------------------------------------ the write
# The document is replaced by one rename, so a reader never sees a half-written
# file, and it is left readable by the nginx worker rather than by root alone.
mode="$(stat -c '%a' "$TMP/out/viewers.json")"
[ "$mode" = 644 ] || fail "viewers.json mode is $mode, want 644"
[ -z "$(find "$TMP/out" -name '.viewers.json.*' -print -quit)" ] ||
  fail "a temporary file was left beside the document"

echo 'viewers presence: PASS'
