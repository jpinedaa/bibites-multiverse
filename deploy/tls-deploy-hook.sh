#!/usr/bin/env bash
# certbot deploy hook: put the renewed pair where the relay is already watching.
#
# Installed at /etc/letsencrypt/renewal-hooks/deploy/multiverse-tls.sh, which
# certbot runs — every executable in that directory — after ANY successful
# renewal. It is the directory rather than a per-certificate --deploy-hook so
# that a re-issue, an added name or a certbot upgrade cannot leave the hook
# behind.
#
# WHY A COPY AND NOT A GROUP GRANT ON /etc/letsencrypt. Both make the key
# readable by a relay that is not root. The copy wins on four counts:
#
#   1. ONE FILE, ONE OWNER, ONE MODE. A group grant has to hold across
#      /etc/letsencrypt/live, /etc/letsencrypt/archive, the dated subdirectory
#      and the key itself — four objects certbot creates and re-creates on its
#      own schedule, and whose modes it has reset across versions. An ACL that
#      survives one renewal is not evidence it survives the next.
#   2. THE RELAY NEVER TRAVERSES certbot's TREE. live/ is a symlink farm into
#      archive/; a reader needs both paths. Here it opens two regular files in
#      one directory that nothing but this hook writes.
#   3. ATOMIC. The pair is written to temporaries and renamed, so the relay never
#      sees a half-written renewal. The CertReloader tolerates that case anyway —
#      it keeps serving the certificate it already has — but not producing the
#      case is better than surviving it.
#   4. AUDITABLE. `ls -l /etc/multiverse/tls` is the whole permissions story, and
#      the failure mode a copy introduces (a hook that silently stops running) is
#      exactly what monitor.sh's certificate check watches for, by comparing the
#      certificate the LISTENER SERVES against the file on disk.
#
# The cost is 8 KB duplicated and one hook that must not be forgotten. That is
# what the renewal-hooks directory is for.
#
# WHAT IT DOES NOT DO: restart the relay. B23's rotation row is a rule about what
# a rotation costs a connected peer and the answer is NOTHING — GetCertificate is
# called once per handshake and stats the pair, so a renewed pair is picked up by
# the next handshake with no signal, no restart and no reload command. nginx is
# the one process here that does need telling, and it gets a reload, not a
# restart.
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
  local src="$1" dst="$2" mode="$3"
  if [ -f "$dst" ] && cmp -s "$src" "$dst"; then
    return 0
  fi
  # install(1) writes to the destination directly, so go through a temporary and
  # rename: the relay may read either file at any moment.
  local tmp
  tmp="$(mktemp "${dst}.XXXXXX")"
  cat "$src" >"$tmp"
  chown "root:$MV_GROUP" "$tmp"
  chmod "$mode" "$tmp"
  mv -f "$tmp" "$dst"
  changed=1
}

# 0640 root:<group>. The relay runs as a member of that group and never as root;
# nothing else on the box is in it.
install_one "$LINEAGE/fullchain.pem" "$MV_TLSDIR/fullchain.pem" 0640
install_one "$LINEAGE/privkey.pem"   "$MV_TLSDIR/privkey.pem"   0640

if [ "$changed" = 0 ]; then
  log "pair unchanged; nothing installed"
  exit 0
fi

log "installed a renewed pair into $MV_TLSDIR (expires $(openssl x509 -in "$MV_TLSDIR/fullchain.pem" -noout -enddate 2>/dev/null | cut -d= -f2))"
log "the relay needs NO restart: its CertReloader picks the pair up on the next TLS handshake"

# nginx holds the certificate open and does need telling. A reload, not a
# restart: an open status-page connection is not worth dropping.
if systemctl is-active --quiet nginx 2>/dev/null; then
  systemctl reload nginx && log "nginx reloaded for the status page's listener"
fi
