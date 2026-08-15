#!/usr/bin/env bash
# Check the rendered nginx front door without changing the host.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/multiverse-front-door.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT

mkdir -p "$TMP/acme" "$TMP/logs" "$TMP/tls"

render() {
  sed -e 's|@@MV_DOMAIN@@|multiverse.example|g' \
      -e 's|@@MV_RELAY_PORT@@|443|g' \
      -e 's|@@MV_RELAY_BACKEND@@|127.0.0.1:8795|g' \
      -e 's|@@MV_ARCHIVE_HTTP@@|127.0.0.1:8796|g' \
      -e "s|@@MV_TLSDIR@@|$TMP/tls|g" \
      -e "s|@@ACME_ROOT@@|$TMP/acme|g" \
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
grep -Fq 'location / {' "$front"
grep -Fq 'proxy_pass http://127.0.0.1:8796;' "$front"
grep -Fq 'limit_except GET HEAD' "$front"
grep -Fq 'Strict-Transport-Security "max-age=31536000" always;' "$front"
grep -Fq 'Permissions-Policy "camera=(), geolocation=(), microphone=(), payment=(), usb=()" always;' "$front"
grep -Fq 'Cross-Origin-Opener-Policy same-origin always;' "$front"
grep -Fq "Content-Security-Policy \"default-src 'self';" "$front"
if grep -Eq '@@|MV_STATUS_PORT|8443' "$TMP/acme.conf" "$front"; then
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
  nginx -t -q -p "$TMP/" -c "$TMP/nginx.conf"
  echo 'front-door render and nginx syntax: PASS'
else
  echo 'front-door render: PASS (nginx syntax skipped because nginx or openssl is unavailable)'
fi
