#!/usr/bin/env bash
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
proc="$tmp/proc"
mkdir -p "$proc/net" "$proc/sys/net/netfilter" "$proc/pressure" "$tmp/data"

printf '%s\n' 'cpu 100 0 50 800 0 0 0 0 0 0' >"$proc/stat"
printf '%s\n' \
  'MemTotal:       1000000 kB' \
  'MemAvailable:    400000 kB' \
  'SwapTotal:       100000 kB' \
  'SwapFree:         90000 kB' >"$proc/meminfo"
printf '%s\n' '0.50 0.40 0.30 1/100 42' >"$proc/loadavg"
printf '%s\n' \
  'Tcp: CurrEstab ActiveOpens PassiveOpens AttemptFails EstabResets RetransSegs OutRsts' \
  'Tcp: 7 10 20 2 3 4 5' >"$proc/net/snmp"
printf '%s\n' \
  'TcpExt: ListenOverflows ListenDrops TCPSynRetrans' \
  'TcpExt: 1 2 3' >"$proc/net/netstat"
printf '%s\n' '9' >"$proc/sys/net/netfilter/nf_conntrack_count"
printf '%s\n' '100' >"$proc/sys/net/netfilter/nf_conntrack_max"
printf '%s\n' 'some avg10=0.00 avg60=0.00 avg300=0.00 total=7000000' 'full avg10=0.00 avg60=0.00 avg300=0.00 total=500000' >"$proc/pressure/cpu"
printf '%s\n' 'some avg10=0.00 avg60=0.00 avg300=0.00 total=3500000' 'full avg10=0.00 avg60=0.00 avg300=0.00 total=1500000' >"$proc/pressure/memory"
printf '%s\n' 'some avg10=0.00 avg60=0.00 avg300=0.00 total=6000000' 'full avg10=0.00 avg60=0.00 avg300=0.00 total=3000000' >"$proc/pressure/io"
printf '%s\n' 'pswpin 13' 'pswpout 24' 'oom_kill 2' >"$proc/vmstat"

sample_stamp='2026-08-24T16:00:00Z'
sample_now="$(date -u -d "$sample_stamp" +%s)"
printf '%s\n' '{"asOf":"2026-08-24T16:00:00Z","traffic":{"available":true,"complete":true,"windowSeconds":60,"requests":120,"status1xx":0,"status2xx":100,"status3xx":0,"status4xx":14,"status5xx":6,"p50Ms":12.5,"p95Ms":48.25,"clientAddress":"198.51.100.4","routes":[{"id":"pages","requests":120,"status1xx":0,"privatePath":"/person/42"},{"id":"/private-route","requests":1}]},"privateClient":"198.51.100.4"}' >"$tmp/viewers.json"
printf '%s\n' '{"ledgerRecords":1120,"ledgerRawBytes":2048,"ledgerSegments":3,"ledgerSegmentsAwaitingColdCopy":1,"ledgerRetiredTotal":2,"rollupCoveredRecords":1100,"genomeGaps":4,"duplicatesRefused":5,"privatePath":"/secret"}' >"$tmp/status.json"

sample="$(MV_PROC_ROOT="$proc" MV_DATA_DIR="$tmp/data" \
  MV_TRAFFIC_DOCUMENT="$tmp/viewers.json" MV_STATUS_SOURCE="$tmp/status.json" \
  MV_HOST_SAMPLE_NOW="$sample_now" \
  MV_HOST_SAMPLE_UNITS='' MV_HOST_SAMPLE_PROC_UNITS='' \
  "$DIR/service-host-sample" --stdout)"

jq -e '
  .pressure.cpu.someUsec == 7000000 and .pressure.memory.fullUsec == 1500000 and
  .pressure.io.fullUsec == 3000000 and .vm.swapInPages == 13 and
  .vm.swapOutPages == 24 and .vm.oomKills == 2 and
  .at == "2026-08-24T16:00:00Z" and .traffic.asOf == "2026-08-24T16:00:00Z" and
  .traffic.complete == true and .traffic.requests == 120 and
  .archive.ledgerRecords == 1120 and .archive.ledgerSegmentsAwaitingColdCopy == 1 and
  .units == []
' <<<"$sample" >/dev/null

if [[ "$sample" == *"198.51.100.4"* || "$sample" == *"/person/42"* || "$sample" == *"/private-route"* || "$sample" == *"/secret"* || "$sample" == *"privateClient"* || "$sample" == *"privatePath"* ]]; then
  printf 'service host sample disclosed a private source field\n' >&2
  exit 1
fi

missing="$(MV_PROC_ROOT="$proc" MV_DATA_DIR="$tmp/data" \
  MV_TRAFFIC_DOCUMENT="$tmp/no-viewers.json" MV_STATUS_SOURCE="$tmp/no-status.json" \
  MV_HOST_SAMPLE_NOW="$sample_now" \
  MV_HOST_SAMPLE_UNITS='' MV_HOST_SAMPLE_PROC_UNITS='' \
  "$DIR/service-host-sample" --stdout)"
jq -e '.traffic == {} and .archive == {}' <<<"$missing" >/dev/null

timestamp_case() {
  printf '%s\n' "$1" >"$tmp/timestamp-viewers.json"
  MV_PROC_ROOT="$tmp/no-proc" MV_DATA_DIR="$tmp/data" \
    MV_TRAFFIC_DOCUMENT="$tmp/timestamp-viewers.json" MV_STATUS_SOURCE="$tmp/status.json" \
    MV_HOST_SAMPLE_NOW="${2:-$sample_now}" \
    MV_HOST_SAMPLE_UNITS='' MV_HOST_SAMPLE_PROC_UNITS='' \
    "$DIR/service-host-sample" --stdout
}

traffic_body='"traffic":{"available":true,"complete":true,"windowSeconds":60,"requests":120,"status2xx":120,"routes":[{"id":"pages","requests":120,"status2xx":120}]}'
past_boundary="$(timestamp_case "{\"asOf\":\"2026-08-24T15:59:30Z\",$traffic_body}")"
future_boundary="$(timestamp_case "{\"asOf\":\"2026-08-24T16:00:30Z\",$traffic_body}")"
frozen="$(timestamp_case "{\"asOf\":\"2026-08-24T15:59:29Z\",$traffic_body}")"
future="$(timestamp_case "{\"asOf\":\"2026-08-24T16:00:31Z\",$traffic_body}")"
missing_stamp="$(timestamp_case "{$traffic_body}")"
malformed="$(timestamp_case "{\"asOf\":\"not-a-time\",$traffic_body}")"
leap_second="$(timestamp_case "{\"asOf\":\"2026-08-24T15:59:60Z\",$traffic_body}")"
short_fields="$(timestamp_case "{\"asOf\":\"2026-8-24T16:00:00Z\",$traffic_body}")"
offset_stamp="$(timestamp_case "{\"asOf\":\"2026-08-24T12:00:00-04:00\",$traffic_body}")"
fraction_stamp="$(timestamp_case "{\"asOf\":\"2026-08-24T16:00:00.000Z\",$traffic_body}")"
feb_30="$(timestamp_case "{\"asOf\":\"2026-02-30T16:00:00Z\",$traffic_body}" \
  "$(date -u -d '2026-03-02T16:00:00Z' +%s)")"
nonleap_feb_29="$(timestamp_case "{\"asOf\":\"2026-02-29T16:00:00Z\",$traffic_body}" \
  "$(date -u -d '2026-03-01T16:00:00Z' +%s)")"
valid_leap_day="$(timestamp_case "{\"asOf\":\"2024-02-29T16:00:00Z\",$traffic_body}" \
  "$(date -u -d '2024-02-29T16:00:00Z' +%s)")"
malformed_numbers="$(timestamp_case '{"asOf":"2026-08-24T16:00:00Z","traffic":{"available":true,"complete":true,"windowSeconds":60.5,"requests":1.5,"status1xx":-1,"status2xx":9007199254740992,"status3xx":9007199254740991,"status4xx":1.5,"status5xx":1e999,"p50Ms":-0.5,"p95Ms":4.5,"routes":[{"id":"pages","requests":9007199254740992,"status1xx":1.5,"status2xx":-1,"status3xx":9007199254740991,"status4xx":1e999,"status5xx":0.0,"p50Ms":1e999,"p95Ms":2.5}]}}')"

jq -e '.traffic.complete == true' <<<"$past_boundary" >/dev/null
jq -e '.traffic.complete == true' <<<"$future_boundary" >/dev/null
jq -e '.traffic.complete == false and .traffic.asOf == "2026-08-24T15:59:29Z"' <<<"$frozen" >/dev/null
jq -e '.traffic.complete == false and .traffic.asOf == "2026-08-24T16:00:31Z"' <<<"$future" >/dev/null
jq -e '.traffic.complete == false and .traffic.asOf == null' <<<"$missing_stamp" >/dev/null
jq -e '.traffic.complete == false and .traffic.asOf == "not-a-time"' <<<"$malformed" >/dev/null
jq -e '.traffic.complete == false and .traffic.asOf == "2026-08-24T15:59:60Z"' <<<"$leap_second" >/dev/null
jq -e '.traffic.complete == false and .traffic.asOf == "2026-8-24T16:00:00Z"' <<<"$short_fields" >/dev/null
jq -e '.traffic.complete == false and .traffic.asOf == "2026-08-24T12:00:00-04:00"' <<<"$offset_stamp" >/dev/null
jq -e '.traffic.complete == false and .traffic.asOf == "2026-08-24T16:00:00.000Z"' <<<"$fraction_stamp" >/dev/null
jq -e '.traffic.complete == false and .traffic.asOf == "2026-02-30T16:00:00Z"' <<<"$feb_30" >/dev/null
jq -e '.traffic.complete == false and .traffic.asOf == "2026-02-29T16:00:00Z"' <<<"$nonleap_feb_29" >/dev/null
jq -e '.traffic.complete == true and .traffic.asOf == "2024-02-29T16:00:00Z"' <<<"$valid_leap_day" >/dev/null
jq -e '
  type == "object" and .traffic.windowSeconds == null and .traffic.requests == null and
  .traffic.status1xx == null and .traffic.status2xx == null and
  .traffic.status3xx == 9007199254740991 and .traffic.status4xx == null and
  .traffic.status5xx == null and .traffic.p50Ms == null and .traffic.p95Ms == 4.5 and
  (.traffic.routes | length) == 1 and .traffic.routes[0].id == "pages" and
  .traffic.routes[0].requests == null and .traffic.routes[0].status1xx == null and
  .traffic.routes[0].status2xx == null and
  .traffic.routes[0].status3xx == 9007199254740991 and
  .traffic.routes[0].status4xx == null and .traffic.routes[0].status5xx == 0 and
  .traffic.routes[0].p50Ms == null and .traffic.routes[0].p95Ms == 2.5
' <<<"$malformed_numbers" >/dev/null

printf 'service-host-sample tests: PASS\n'
