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

printf '%s\n' '{"traffic":{"available":true,"complete":true,"windowSeconds":60,"requests":120,"status1xx":0,"status2xx":100,"status3xx":0,"status4xx":14,"status5xx":6,"p50Ms":12.5,"p95Ms":48.25,"clientAddress":"198.51.100.4","routes":[{"id":"pages","requests":120,"status1xx":0,"privatePath":"/person/42"},{"id":"/private-route","requests":1}]},"privateClient":"198.51.100.4"}' >"$tmp/viewers.json"
printf '%s\n' '{"ledgerRecords":1120,"ledgerRawBytes":2048,"ledgerSegments":3,"ledgerSegmentsAwaitingColdCopy":1,"ledgerRetiredTotal":2,"rollupCoveredRecords":1100,"genomeGaps":4,"duplicatesRefused":5,"privatePath":"/secret"}' >"$tmp/status.json"

sample="$(MV_PROC_ROOT="$proc" MV_DATA_DIR="$tmp/data" \
  MV_TRAFFIC_DOCUMENT="$tmp/viewers.json" MV_STATUS_SOURCE="$tmp/status.json" \
  MV_HOST_SAMPLE_UNITS='' MV_HOST_SAMPLE_PROC_UNITS='' \
  "$DIR/service-host-sample" --stdout)"

jq -e '
  .pressure.cpu.someUsec == 7000000 and .pressure.memory.fullUsec == 1500000 and
  .pressure.io.fullUsec == 3000000 and .vm.swapInPages == 13 and
  .vm.swapOutPages == 24 and .vm.oomKills == 2 and
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
  MV_HOST_SAMPLE_UNITS='' MV_HOST_SAMPLE_PROC_UNITS='' \
  "$DIR/service-host-sample" --stdout)"
jq -e '.traffic == {} and .archive == {}' <<<"$missing" >/dev/null

printf 'service-host-sample tests: PASS\n'
