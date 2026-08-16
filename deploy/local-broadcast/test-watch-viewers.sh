#!/usr/bin/env bash
# Drive watch-viewers.ps1 against a written presence document.
#
# The watcher decides whether a production broadcast publishes, so the decisions
# are checked here rather than on the live publisher. Every case runs with
# -WhatIf, which reaches no OBS at all and carries a simulated stream state
# instead, so this test cannot touch a running broadcast.
#
# The presence document is served by a local python3 server on loopback. WSL
# forwards a loopback listener to the Windows side, which is the same path the
# live publisher uses to reach its RTMP tunnel.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
watcher="$here/watch-viewers.ps1"

fail() {
  printf 'test-watch-viewers: %s\n' "$*" >&2
  exit 1
}

for command in python3 powershell.exe wslpath; do
  command -v "$command" >/dev/null || {
    printf 'test-watch-viewers: SKIP, no %s on this host\n' "$command"
    exit 0
  }
done

watcher_windows="$(wslpath -w "$watcher")"
work="$(mktemp -d)"
server_pid=''
cleanup() {
  [ -n "$server_pid" ] && kill "$server_pid" 2>/dev/null || true
  rm -rf "$work"
}
trap cleanup EXIT

# --------------------------------------------------------- the handshake math

# The obs-websocket authentication digest is
# base64(sha256(base64(sha256(password + salt)) + challenge)). A wrong digest
# gives a watcher that connects, is refused, and silently never publishes, so it
# is checked against a second implementation rather than against itself.
expected_digest="$(python3 - <<'PY'
import base64, hashlib
password = 'supersecretpassword'
salt = 'lM1GncleQOaCu9lT1yeUZhFYnqhsLLP1G5lAGo3ixaI='
challenge = '+IxH4CnCiqpX1rM9scsNynZzbOe4KhDeYcTNS3PDaeY='
secret = base64.b64encode(hashlib.sha256((password + salt).encode()).digest()).decode()
print(base64.b64encode(hashlib.sha256((secret + challenge).encode()).digest()).decode())
PY
)"
self_test="$(powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$watcher_windows" -SelfTest |
  tr -d '\r')"
printf '%s\n' "$self_test" | grep -Fqx "authDigest=$expected_digest" || \
  fail "the obs-websocket digest disagrees with the reference implementation: $self_test"
printf '%s\n' "$self_test" | grep -Fqx 'futureAge=0' || \
  fail 'a presence document from the future did not read as age zero'
printf '%s\n' "$self_test" | grep -Fqx 'unreadableAge=' || \
  fail 'an unreadable asOf value did not read as unknown'

# ------------------------------------------------------------ the fake service

document="$work/viewers.json"
printf '{"watching":false,"hlsSessions":0,"lastViewerRequestAgeSec":null,"asOf":"1970-01-01T00:00:00Z"}\n' \
  >"$document"

cat >"$work/serve.py" <<'PY'
import http.server
import sys

DOCUMENT = sys.argv[1]
PORT = int(sys.argv[2])


class Handler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path != '/api/viewers':
            self.send_response(404)
            self.send_header('Content-Length', '0')
            self.end_headers()
            return
        # Read on every request, so a case can rewrite the document between runs.
        with open(DOCUMENT, 'rb') as handle:
            body = handle.read()
        self.send_response(200)
        self.send_header('Content-Type', 'application/json')
        self.send_header('Cache-Control', 'no-store')
        self.send_header('Content-Length', str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *_args):
        return


http.server.HTTPServer(('127.0.0.1', PORT), Handler).serve_forever()
PY

port=0
for candidate in $(seq 18790 18830); do
  if ! (exec 3<>"/dev/tcp/127.0.0.1/$candidate") 2>/dev/null; then
    port="$candidate"
    break
  fi
  exec 3>&- 2>/dev/null || true
done
[ "$port" -ne 0 ] || fail 'no free loopback port for the presence fixture'

python3 "$work/serve.py" "$document" "$port" &
server_pid=$!
presence_url="http://127.0.0.1:$port/api/viewers"
ready=0
for _ in $(seq 1 40); do
  if curl -fsS --max-time 2 "$presence_url" >/dev/null 2>&1; then ready=1; break; fi
  sleep 0.25
done
[ "$ready" -eq 1 ] || fail 'the presence fixture did not start'

now_stamp() { date -u +%Y-%m-%dT%H:%M:%SZ; }

write_document() { printf '%s\n' "$1" >"$document"; }

# Run one cycle and return what the watcher decided. -WhatIf keeps every case
# away from OBS; -Once keeps it away from a sleep.
run_case() {
  local url="$1"
  shift
  powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$watcher_windows" \
    -PresenceUrl "$url" -Once -WhatIf -PollSeconds 1 "$@" 2>&1 | tr -d '\r'
}

expect_line() {
  local output="$1" pattern="$2" description="$3"
  printf '%s\n' "$output" | grep -Fq -- "$pattern" || \
    fail "$description; the watcher said: $output"
}

forbid_line() {
  local output="$1" pattern="$2" description="$3"
  if printf '%s\n' "$output" | grep -Fq -- "$pattern"; then
    fail "$description; the watcher said: $output"
  fi
}

# ---------------------------------------------------------------- the cases

# A viewer arrives at a stopped publisher.
write_document "{\"watching\":true,\"hlsSessions\":0,\"lastViewerRequestAgeSec\":3,\"asOf\":\"$(now_stamp)\"}"
output="$(run_case "$presence_url")"
expect_line "$output" 'WHATIF would start the stream' 'an arriving viewer did not start the stream'
forbid_line "$output" 'obs-websocket' '-WhatIf reached obs-websocket'

# The same reading against a publisher that is already up changes nothing.
output="$(run_case "$presence_url" -AssumeStreaming)"
forbid_line "$output" 'WHATIF would' 'a live stream was changed for a viewer who was already served'

# The room empties. One empty reading is not a stop.
write_document "{\"watching\":false,\"hlsSessions\":0,\"lastViewerRequestAgeSec\":400,\"asOf\":\"$(now_stamp)\"}"
output="$(run_case "$presence_url" -AssumeStreaming)"
expect_line "$output" 'nobody is watching, so the stream stops in 180 seconds' \
  'an empty room did not start the idle timer'
forbid_line "$output" 'WHATIF would stop' 'the stream stopped on the first empty reading'

# The idle period expires.
output="$(run_case "$presence_url" -AssumeStreaming -IdleStopSeconds 0)"
expect_line "$output" 'WHATIF would stop the stream' 'an expired idle period did not stop the stream'

# An empty room with the publisher already stopped is quiet.
output="$(run_case "$presence_url" -IdleStopSeconds 0)"
forbid_line "$output" 'WHATIF would' 'a stopped stream was stopped again'

# A frozen status timer is not an empty room. This is the case that would
# otherwise take a watched broadcast off the air.
write_document '{"watching":false,"hlsSessions":0,"lastViewerRequestAgeSec":null,"asOf":"2000-01-01T00:00:00Z"}'
output="$(run_case "$presence_url" -AssumeStreaming -IdleStopSeconds 0)"
expect_line "$output" 'presence unknown, holding the current state' \
  'a frozen presence document was read as an answer'
expect_line "$output" 'seconds old' 'the frozen document was not reported as stale'
forbid_line "$output" 'WHATIF would' 'a frozen presence document changed the stream'

# A document that is not the contract is unknown, not an empty room.
write_document '{"hlsSessions":0}'
output="$(run_case "$presence_url" -AssumeStreaming -IdleStopSeconds 0)"
expect_line "$output" 'has no boolean "watching" field' 'a document without watching was accepted'
forbid_line "$output" 'WHATIF would' 'a document without watching changed the stream'

write_document 'not json at all'
output="$(run_case "$presence_url" -AssumeStreaming -IdleStopSeconds 0)"
expect_line "$output" 'the presence document is not JSON' 'unparsable content was accepted'
forbid_line "$output" 'WHATIF would' 'unparsable content changed the stream'

# The endpoint is not deployed. A 404 must hold the stream, not stop it.
output="$(run_case "http://127.0.0.1:$port/api/missing" -AssumeStreaming -IdleStopSeconds 0)"
expect_line "$output" 'presence unknown, holding the current state' 'an HTTP 404 was read as an answer'
forbid_line "$output" 'WHATIF would' 'an HTTP 404 changed the stream'

# The service is unreachable.
kill "$server_pid" 2>/dev/null || true
wait "$server_pid" 2>/dev/null || true
server_pid=''
output="$(run_case "$presence_url" -AssumeStreaming -IdleStopSeconds 0 -HttpTimeoutSeconds 3)"
expect_line "$output" 'presence unknown, holding the current state' \
  'an unreachable service was read as an answer'
forbid_line "$output" 'WHATIF would' 'an unreachable service changed the stream'

# Every line carries a UTC stamp, because these logs are read beside the
# service's own records.
printf '%s\n' "$output" | grep -Eq '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z ' || \
  fail 'the watcher log lines carry no UTC stamp'

printf 'viewer watcher decisions, presence failures, and handshake digest: PASS\n'
