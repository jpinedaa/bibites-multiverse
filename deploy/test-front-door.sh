#!/usr/bin/env bash
# Check the rendered nginx front door without changing the host.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/multiverse-front-door.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT

mkdir -p "$TMP/acme" "$TMP/logs" "$TMP/tls" "$TMP/www/announcements" "$TMP/gates"

render() {
  sed -e 's|@@MV_DOMAIN@@|multiverse.example|g' \
      -e 's|@@MV_RELAY_PORT@@|443|g' \
      -e 's|@@MV_RELAY_BACKEND@@|127.0.0.1:8795|g' \
      -e 's|@@MV_ARCHIVE_HTTP@@|127.0.0.1:8796|g' \
      -e 's|@@MV_STREAM_HLS_BACKEND@@|127.0.0.1:8888|g' \
      -e "s|@@MV_TLSDIR@@|$TMP/tls|g" \
      -e "s|@@ACME_ROOT@@|$TMP/acme|g" \
      -e "s|@@WWW_ROOT@@|$TMP/www|g" \
      -e "s|@@MV_GATEDIR@@|$TMP/gates|g" \
      -e "s|/var/log/nginx|$TMP/logs|g" \
      "$1" >"$2"
}

render "$HERE/nginx/multiverse-10-acme.conf" "$TMP/acme.conf"
render "$HERE/nginx/multiverse-20-status.conf" "$TMP/front-door.conf"

front="$TMP/front-door.conf"
grep -Fq 'listen 443 ssl http2;' "$front"
grep -Fq 'location ^~ /contract-b/' "$front"
grep -Fq 'proxy_pass http://127.0.0.1:8795;' "$front"
grep -Fq 'proxy_set_header Upgrade' "$front"
grep -Fq 'proxy_set_header Connection' "$front"
grep -Fq 'proxy_set_header X-Forwarded-For   $remote_addr;' "$front"
grep -Fq 'limit_req_zone  $binary_remote_addr zone=mvenroll:1m rate=2r/m;' "$front"

# The peer gate. Three properties matter and each is checked separately: the
# map exists, loopback is pinned open by an EXACT key (a map consults exact keys
# before any regex, which is the whole reason the archive is never gated), and
# the gate directory is included as a GLOB so that the empty normal state is a
# valid configuration rather than a file nginx cannot open.
grep -Fq 'map $remote_addr $mv_peer_gate {' "$front"
awk '/map \$remote_addr \$mv_peer_gate \{/,/^\}/' "$front" >"$TMP/gate-map.conf"
grep -Eq '^\s+default\s+0;' "$TMP/gate-map.conf"
grep -Eq '^\s+127\.0\.0\.1\s+0;' "$TMP/gate-map.conf"
grep -Eq '^\s+::1\s+0;' "$TMP/gate-map.conf"
grep -Fq "include $TMP/gates/*.map;" "$TMP/gate-map.conf"
# The relay route must refuse BEFORE it proxies. A gate evaluated after
# proxy_pass would be no gate at all.
awk '/location \^~ \/contract-b\/ \{/,/^    \}/' "$front" >"$TMP/contract-b.conf"
grep -Fq 'if ($mv_peer_gate) {' "$TMP/contract-b.conf"
grep -Fq 'add_header Retry-After 15 always;' "$TMP/contract-b.conf"
grep -Fq 'return 503;' "$TMP/contract-b.conf"
if [ "$(grep -n 'if (\$mv_peer_gate)' "$TMP/contract-b.conf" | cut -d: -f1)" -ge \
     "$(grep -n 'proxy_pass' "$TMP/contract-b.conf" | head -1 | cut -d: -f1)" ]; then
  echo 'the peer gate is evaluated after proxy_pass; it must refuse before it' >&2
  exit 1
fi
# No other route may carry the gate. Gating the website would take the map, the
# announcements page and /healthz down with the relay, and the announcements
# page exists precisely to survive the outage it announces.
if [ "$(grep -c 'if (\$mv_peer_gate)' "$front")" != 1 ]; then
  echo 'the peer gate is applied outside the relay route' >&2
  exit 1
fi
grep -Fq 'location = /api/enroll {' "$front"
grep -Fq 'limit_except POST { deny all; }' "$front"
grep -Fq 'limit_req  zone=mvenroll burst=3 nodelay;' "$front"
grep -Fq 'location ^~ /stream/' "$front"
grep -Fq 'proxy_pass http://127.0.0.1:8888/;' "$front"
grep -Fq 'proxy_redirect ~^/(.*)$ /stream/$1;' "$front"
grep -Fq 'limit_conn mvconn 24;' "$front"
if grep -Fq 'zone=mvstream' "$front"; then
  echo 'the HLS route contains an incompatible request-rate limit' >&2
  exit 1
fi
grep -Fq 'location = /announcements {' "$front"
grep -Fq 'return 301 /announcements/;' "$front"
grep -Fq 'location ^~ /announcements/ {' "$front"
grep -Fq 'ssi on;' "$front"
# The announcements page must not be a proxy: an archive route would take a
# service notice offline for the outage the notice is about.
awk '/location \^~ \/announcements\/ \{/,/^    \}/' "$front" | grep -Fq 'proxy_pass' && {
  echo 'the announcements route proxies to a backend; it must serve from disk' >&2
  exit 1
}
awk '/location \^~ \/announcements\/ \{/,/^    \}/' "$front" | grep -Fq 'limit_except GET HEAD'
grep -Fq 'location / {' "$front"
grep -Fq 'proxy_pass http://127.0.0.1:8796;' "$front"
grep -Fq 'limit_except GET HEAD' "$front"
grep -Fq 'Strict-Transport-Security "max-age=31536000" always;' "$front"
grep -Fq 'Permissions-Policy "camera=(), geolocation=(), microphone=(), payment=(), usb=()" always;' "$front"
grep -Fq 'Cross-Origin-Opener-Policy same-origin always;' "$front"
grep -Fq "media-src 'self' blob:;" "$front"
grep -Fq "Content-Security-Policy \"default-src 'self';" "$front"
render "$HERE/www/announcements/index.html" "$TMP/announcements.html"
grep -Fq '<!--# include file="notices.html" -->' "$TMP/announcements.html"
# The page is chrome only. A notice that lived in this file would need a
# provision run to publish, which is the coupling the split exists to remove.
if grep -Fq 'class="notice"' "$TMP/announcements.html"; then
  echo 'the announcements chrome contains a notice; notices belong in notices.html' >&2
  exit 1
fi
grep -Fq 'class="notice"' "$HERE/www/announcements/notices.html"
if grep -Eq '^[[:space:]]*</?(html|head|body)\b' "$HERE/www/announcements/notices.html"; then
  echo 'notices.html must be an HTML fragment, not a whole document' >&2
  exit 1
fi

if grep -Eq '@@|MV_STATUS_PORT|8443' "$TMP/acme.conf" "$front" "$TMP/announcements.html"; then
  echo 'front-door render contains an obsolete value or unresolved placeholder' >&2
  exit 1
fi

if command -v nginx >/dev/null && command -v openssl >/dev/null; then
  openssl req -x509 -newkey rsa:2048 -nodes -days 1 \
    -subj '/CN=multiverse.example' \
    -keyout "$TMP/tls/privkey.pem" -out "$TMP/tls/fullchain.pem" \
    >/dev/null 2>&1
  sed -e 's/listen 80 /listen 18080 /' \
      -e 's/\[::\]:80 /[::]:18080 /' \
      "$TMP/acme.conf" >"$TMP/acme-syntax.conf"
  sed -e 's/listen 443 /listen 18443 /' \
      -e 's/\[::\]:443 /[::]:18443 /' \
      "$TMP/front-door.conf" >"$TMP/front-door-syntax.conf"
  cat >"$TMP/nginx.conf" <<EOF
pid $TMP/nginx.pid;
error_log $TMP/logs/error.log;
events {}
http {
    access_log $TMP/logs/access.log;
    include $TMP/acme-syntax.conf;
    include $TMP/front-door-syntax.conf;
}
EOF
  # BOTH gate states must be valid nginx, and they fail in opposite directions.
  #
  # Gate ABSENT is the state this host is in at every moment but one, and it is
  # the one a literal `include` would break: nginx refuses to start on a named
  # file that does not exist. $TMP/gates is empty here on purpose.
  nginx -t -q -p "$TMP/" -c "$TMP/nginx.conf"
  # Gate PRESENT is the state a restart puts the host in, and getting it wrong
  # means discovering it with the relay already stopped. The fixture is the
  # exact content restart-relay.sh writes.
  printf '~. 1;\n' >"$TMP/gates/peer-gate.map"
  nginx -t -q -p "$TMP/" -c "$TMP/nginx.conf"
  rm -f "$TMP/gates/peer-gate.map"
  # And absent again, because the interesting failure is a gate that cannot be
  # taken back down.
  nginx -t -q -p "$TMP/" -c "$TMP/nginx.conf"
  echo 'front-door render and nginx syntax, gate down and up: PASS'
else
  echo 'front-door render: PASS (nginx syntax skipped because nginx or openssl is unavailable)'
fi
