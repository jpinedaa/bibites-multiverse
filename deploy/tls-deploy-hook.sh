#!/usr/bin/env bash
# certbot deploy hook: install the renewed pair for nginx and reload nginx.
#
# Installed at /etc/letsencrypt/renewal-hooks/deploy/multiverse-tls.sh, which
# certbot runs — every executable in that directory — after ANY successful
# renewal. It is the directory rather than a per-certificate --deploy-hook so
# that a re-issue, an added name or a certbot upgrade cannot leave the hook
# behind.
#
# WHY A COPY. nginx can read the Let's Encrypt files as root. The separate copy
# gives the monitor one stable certificate path without exposing certbot's tree.
#
#   1. ONE FILE, ONE OWNER, ONE MODE. A group grant has to hold across
#      /etc/letsencrypt/live, /etc/letsencrypt/archive, the dated subdirectory
#      and the key itself — four objects certbot creates and re-creates on its
#      own schedule, and whose modes it has reset across versions. An ACL that
#      survives one renewal is not evidence it survives the next.
#   2. NO SERVICE TRAVERSES certbot's TREE. live/ is a symlink farm into
#      archive/. Here nginx opens two regular files in one directory.
#   3. ATOMIC. Each file is written to a temporary and renamed. nginx reloads
#      only after both files are installed.
#   4. AUDITABLE. `ls -l /etc/multiverse/tls` is the whole permissions story, and
#      the failure mode a copy introduces (a hook that silently stops running) is
#      exactly what monitor.sh watches. It compares the served certificate with
#      the file on disk.
#
# The cost is 8 KB duplicated and one hook that must not be forgotten. That is
# what the renewal-hooks directory is for.
#
# WHAT IT DOES NOT DO: restart a service. nginx gets a reload. Existing website
# requests and WebSocket connections continue while nginx opens the new pair.
set -euo pipefail

ENV_FILE="${MV_ENV_FILE:-/etc/multiverse/deploy.env}"
[ -r "$ENV_FILE" ] || { echo "multiverse-tls: no $ENV_FILE" >&2; exit 1; }
# shellcheck source=/dev/null
set -a; . "$ENV_FILE"; set +a

: "${MV_GROUP:=multiverse}"
: "${MV_TLSDIR:=/etc/multiverse/tls}"

# certbot sets RENEWED_LINEAGE for a renewal; provision.sh sets it by hand for
# the first issuance.
LINEAGE="${RENEWED_LINEAGE:-/etc/letsencrypt/live/$MV_DOMAIN}"

log() { logger -t multiverse-tls -- "$*" 2>/dev/null || true; printf 'multiverse-tls: %s\n' "$*"; }

if [ ! -f "$LINEAGE/fullchain.pem" ] || [ ! -f "$LINEAGE/privkey.pem" ]; then
  log "no pair at $LINEAGE — nothing to install"
  exit 0
fi

# Only act on OUR lineage. certbot runs every deploy hook for every renewed
# certificate, and a box that ever holds a second one must not have its relay's
# key replaced by the other certificate's.
case "$LINEAGE" in
  */"$MV_DOMAIN") ;;
  *) log "lineage $LINEAGE is not $MV_DOMAIN — ignoring"; exit 0 ;;
esac

install -d -m 0750 -o root -g "$MV_GROUP" "$MV_TLSDIR"

changed=0
install_one() {
  local src="$1" dst="$2" mode="$3" owner="$4"
  if [ -f "$dst" ] && cmp -s "$src" "$dst"; then
    chown "$owner" "$dst"
    chmod "$mode" "$dst"
    return 0
  fi
  # install(1) writes to the destination directly, so go through a temporary and
  # rename: the relay may read either file at any moment.
  local tmp
  tmp="$(mktemp "${dst}.XXXXXX")"
  cat "$src" >"$tmp"
  chown "$owner" "$tmp"
  chmod "$mode" "$tmp"
  mv -f "$tmp" "$dst"
  changed=1
}

# The monitor reads the certificate. Only nginx's root master reads the key.
install_one "$LINEAGE/fullchain.pem" "$MV_TLSDIR/fullchain.pem" 0640 "root:$MV_GROUP"
install_one "$LINEAGE/privkey.pem"   "$MV_TLSDIR/privkey.pem"   0600 root:root

if [ "$changed" = 0 ]; then
  log "pair unchanged; nothing installed"
  exit 0
fi

log "installed a renewed pair into $MV_TLSDIR (expires $(openssl x509 -in "$MV_TLSDIR/fullchain.pem" -noout -enddate 2>/dev/null | cut -d= -f2))"

# nginx gets a reload, not a restart. Existing connections continue.
if systemctl is-active --quiet nginx 2>/dev/null; then
  systemctl reload nginx && log "nginx reloaded for the HTTPS front door"
fi
