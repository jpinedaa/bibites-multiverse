#!/usr/bin/env bash
# Provision a fresh AWS Lightsail Ubuntu instance to run the Bibites Multiverse
# relay and archive as a SERVICE — m5_considerations.md, Design Question 2.
#
# It is idempotent. Every phase checks the state it is about to create and says
# "already" instead of doing it again, so re-running after a failed step, after a
# binary upgrade or after a change to /etc/multiverse/deploy.env is the normal
# way to use it rather than the recovery from a mistake.
#
#   sudo deploy/provision.sh                  # every phase, in order
#   sudo deploy/provision.sh --only tls       # one phase
#   sudo deploy/provision.sh --dry-run        # print, change nothing
#   sudo deploy/provision.sh --list           # the phases, in order
#
# WHAT IT NEEDS BEFORE IT RUNS. All five, and it refuses rather than half-builds:
#   1. /etc/multiverse/deploy.env, filled in (deploy.env.example is the template).
#   2. MV_DOMAIN's A record already pointing at this instance's STATIC IP.
#   3. Ports 80, 443, MV_STATUS_PORT and 22 open in the LIGHTSAIL CONSOLE's own
#      firewall. ufw below is the second of two firewalls, not the only one.
#   4. The two binaries staged at $MV_STAGE_DIR (ship.sh puts them there).
#   5. Ubuntu with systemd, apt and root.
#
# WHAT IT DOES NOT DO: buy anything, create an AWS account, touch DNS, publish a
# release, or reach any machine but this one.
set -euo pipefail

ENV_FILE="${MV_ENV_FILE:-/etc/multiverse/deploy.env}"
DRY=0
ONLY=""

PHASES="preflight packages account directories swap binaries envfiles firewall
        nginxacme tls nginxstatus bootstrap systemd upgrades verify"

say()  { printf '     %s\n' "$*"; }
step() { printf '\n==== %s\n' "$*"; }
warn() { printf '  !! %s\n' "$*" >&2; }
die()  { printf '\nSTOP: %s\n' "$*" >&2; exit 1; }

# run executes a command, or prints it under --dry-run. Every mutation in this
# script goes through it, which is what makes --dry-run an honest rehearsal
# rather than a partial one.
run() {
  if [ "$DRY" = 1 ]; then
    printf '     [dry-run] %s\n' "$*"
    return 0
  fi
  "$@"
}

# write_file <path> <mode> <owner:group> — content on stdin, atomic, dry-run aware.
write_file() {
  local path="$1" mode="$2" owner="$3" tmp
  if [ "$DRY" = 1 ]; then
    printf '     [dry-run] write %s (mode %s, %s)\n' "$path" "$mode" "$owner"
    cat >/dev/null
    return 0
  fi
  tmp="$(mktemp "${path}.XXXXXX")"
  cat >"$tmp"
  chmod "$mode" "$tmp"
  chown "$owner" "$tmp"
  mv -f "$tmp" "$path"
}

while [ $# -gt 0 ]; do
  case "$1" in
    --dry-run) DRY=1 ;;
    --only) ONLY="${2:?--only needs a phase name}"; shift ;;
    --list) printf '%s\n' $PHASES; exit 0 ;;
    --env-file) ENV_FILE="${2:?--env-file needs a path}"; shift ;;
    -h|--help) sed -n '2,30p' "$0"; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
  shift
done

[ "$(id -u)" = 0 ] || die "run this with sudo. It creates a system user, writes /etc and drives systemd."
[ -r "$ENV_FILE" ] || die "no $ENV_FILE. Copy deploy/deploy.env.example there and fill it in."

# shellcheck source=/dev/null
set -a; . "$ENV_FILE"; set +a

KIT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

: "${MV_USER:=multiverse}"
: "${MV_GROUP:=multiverse}"
: "${MV_PREFIX:=/opt/multiverse}"
: "${MV_STATE:=/var/lib/multiverse}"
: "${MV_LOGDIR:=/var/log/multiverse}"
: "${MV_TLSDIR:=/etc/multiverse/tls}"
: "${MV_STAGE_DIR:=/home/ubuntu/multiverse-stage}"
: "${MV_RELAY_PORT:=443}"
: "${MV_STATUS_PORT:=8443}"
: "${MV_ARCHIVE_HTTP:=127.0.0.1:8796}"
: "${MV_ARCHIVE_PEER_ID:=archive-main}"
: "${MV_ACME_MODE:=webroot}"
: "${MV_SWAP_GB:=0}"
: "${MV_LOG_ROTATE_MB:=100}"
: "${MV_LOG_KEEP:=5}"
: "${MV_LOG_LEVEL:=info}"

RELAY_DATA="$MV_STATE/relay"
ARCHIVE_DATA="$MV_STATE/archive"
BIN="$MV_PREFIX/bin"
ACME_ROOT=/var/www/acme
ARCHIVE_SECRET=/etc/multiverse/archive.secret
CONTRACT_B_PATH=/contract-b/v4
ADVERTISE_URL="wss://${MV_DOMAIN}${CONTRACT_B_PATH}"
[ "$MV_RELAY_PORT" = 443 ] || ADVERTISE_URL="wss://${MV_DOMAIN}:${MV_RELAY_PORT}${CONTRACT_B_PATH}"

# ---------------------------------------------------------------- preflight

phase_preflight() {
  step "preflight"

  case "$MV_DOMAIN" in
    ""|CHOOSE*|*example.org|*example.com)
      die "MV_DOMAIN is still the placeholder. The name is PERMANENT for the run —
     it is baked into every join string this relay ever mints — so this script
     will not proceed on a guess." ;;
  esac
  case "${MV_ACME_EMAIL:-}" in
    ""|CHOOSE*) die "MV_ACME_EMAIL is unset. Let's Encrypt's expiry warning is the last line
     of defence behind monitor.sh's certificate check." ;;
  esac
  case "${MV_RETENTION:-}" in
    keep-everything|bounded-ledger|prune-genomes|graduate)
      say "retention rule: $MV_RETENTION" ;;
    *)
      warn "MV_RETENTION is '${MV_RETENTION:-unset}'. Decision 3 requires the rule to EXIST"
      warn "    before D24's announced ending, even if it is 'keep everything'. Provisioning"
      warn "    continues — the rule changes no command here — but SIZING.md and"
      warn "    WIND-DOWN.md both have a hole until it is set." ;;
  esac
  case "$MV_ACME_MODE" in
    webroot) ;;
    dns) die "MV_ACME_MODE=dns needs a certbot plugin for the registrar the owner picks,
     and that choice is not made. Install certbot-dns-<provider>, put the
     credentials at /etc/multiverse/dns-credentials.ini (mode 0600), and re-run
     with the tls phase adapted. This script refuses to guess a provider." ;;
    *) die "MV_ACME_MODE must be webroot or dns" ;;
  esac

  command -v systemctl >/dev/null || die "no systemd on this host"
  command -v apt-get   >/dev/null || die "this script is written for Ubuntu (apt)"

  # The A record. A wrong one is the single most common way an ACME issuance
  # fails, and it fails AFTER the rest of the box is built, which is the
  # expensive order to find out.
  local want have
  want="$(curl -fsS --max-time 5 http://169.254.169.254/latest/meta-data/public-ipv4 2>/dev/null || true)"
  have="$(getent ahostsv4 "$MV_DOMAIN" 2>/dev/null | awk 'NR==1{print $1}' || true)"
  if [ -z "$have" ]; then
    warn "$MV_DOMAIN does not resolve yet. ACME issuance will fail until it does."
  elif [ -n "$want" ] && [ "$have" != "$want" ]; then
    warn "$MV_DOMAIN resolves to $have; this instance's public IPv4 is $want."
    warn "    If that is a stale record, wait for the TTL. If it is deliberate, the"
    warn "    HTTP-01 challenge will be answered by the OTHER host and issuance fails."
  else
    say "$MV_DOMAIN -> ${have:-?} (this instance)"
  fi
  # An AAAA record that points somewhere else breaks HTTP-01 silently, because
  # the validator prefers IPv6. This is advice rather than a check: a resolver
  # can answer ahostsv6 with a v4-mapped address, so "there is an AAAA" is not
  # something a getent probe can establish honestly.
  say "note: IF $MV_DOMAIN has an AAAA record it MUST point at this instance too."
  say "      The ACME validator prefers IPv6, so a stale AAAA fails issuance while"
  say "      every IPv4 test on this box passes."

  [ -d "$MV_STAGE_DIR" ] || die "no staged binaries at $MV_STAGE_DIR. Run deploy/ship.sh on the
     development machine first; it builds and scps them here."
  say "staged: $(ls -1 "$MV_STAGE_DIR" 2>/dev/null | tr '\n' ' ')"

  say "domain      $MV_DOMAIN"
  say "relay       wss://$MV_DOMAIN:$MV_RELAY_PORT$CONTRACT_B_PATH"
  say "status page https://$MV_DOMAIN:$MV_STATUS_PORT/"
  say "archive     $MV_ARCHIVE_HTTP  (loopback, deliberately — see README)"
}

# ---------------------------------------------------------------- packages

phase_packages() {
  step "packages"
  local want=(nginx certbot ufw jq curl ca-certificates unattended-upgrades bc)
  local missing=()
  local p
  for p in "${want[@]}"; do
    dpkg -s "$p" >/dev/null 2>&1 || missing+=("$p")
  done
  if [ "${#missing[@]}" = 0 ]; then
    say "already: ${want[*]}"
    return 0
  fi
  say "installing: ${missing[*]}"
  run env DEBIAN_FRONTEND=noninteractive apt-get update -qq
  run env DEBIAN_FRONTEND=noninteractive apt-get install -y -qq "${missing[@]}"
}

# ---------------------------------------------------------------- account

phase_account() {
  step "the service account"
  if getent group "$MV_GROUP" >/dev/null; then
    say "already: group $MV_GROUP"
  else
    run groupadd --system "$MV_GROUP"
    say "created group $MV_GROUP"
  fi
  if id -u "$MV_USER" >/dev/null 2>&1; then
    say "already: user $MV_USER"
  else
    run useradd --system --gid "$MV_GROUP" --home-dir "$MV_STATE" \
      --no-create-home --shell /usr/sbin/nologin \
      --comment "Bibites Multiverse relay and archive" "$MV_USER"
    say "created user $MV_USER (no login shell, no home)"
  fi
}

# ---------------------------------------------------------------- directories

phase_directories() {
  step "directories"
  # /etc/multiverse holds the env files and the TLS copy: root writes, the
  # service only reads, and the group is what carries the read.
  run install -d -m 0750 -o root -g "$MV_GROUP" /etc/multiverse "$MV_TLSDIR"
  run install -d -m 0755 -o root -g root "$MV_PREFIX" "$BIN"
  run install -d -m 0750 -o "$MV_USER" -g "$MV_GROUP" "$MV_STATE" "$RELAY_DATA" "$ARCHIVE_DATA"
  run install -d -m 0750 -o "$MV_USER" -g "$MV_GROUP" "$MV_STATE/backup" "$MV_STATE/monitor"
  run install -d -m 0750 -o "$MV_USER" -g "$MV_GROUP" "$MV_LOGDIR"
  run install -d -m 0755 -o root -g root "$ACME_ROOT" "$ACME_ROOT/.well-known"
  # The kit itself lives on the box, because RESTART-POLICY.md is a document an
  # operator reads at 03:00 on the machine that is misbehaving — and because the
  # on-box copy must be able to re-run this script, which needs the templates.
  run install -d -m 0755 -o root -g root "$MV_PREFIX/deploy" \
    "$MV_PREFIX/deploy/nginx" "$MV_PREFIX/deploy/systemd"
  local f
  for f in "$KIT_DIR"/*.sh; do
    [ -e "$f" ] || continue
    run install -m 0755 -o root -g root "$f" "$MV_PREFIX/deploy/$(basename "$f")"
  done
  for f in "$KIT_DIR"/*.md "$KIT_DIR"/deploy.env.example; do
    [ -e "$f" ] || continue
    run install -m 0644 -o root -g root "$f" "$MV_PREFIX/deploy/$(basename "$f")"
  done
  for f in "$KIT_DIR"/nginx/* "$KIT_DIR"/systemd/*; do
    [ -e "$f" ] || continue
    run install -m 0644 -o root -g root "$f" \
      "$MV_PREFIX/deploy/$(basename "$(dirname "$f")")/$(basename "$f")"
  done
  say "state $MV_STATE  logs $MV_LOGDIR  tls $MV_TLSDIR  kit $MV_PREFIX/deploy"
}

# ---------------------------------------------------------------- swap

phase_swap() {
  step "swap (the memory verdict's second half)"
  if [ "${MV_SWAP_GB}" = 0 ] || [ -z "${MV_SWAP_GB}" ]; then
    say "MV_SWAP_GB=0 — none configured."
    say "This is the verdict, not a default: the streamed replay wants ~0.18 KB per"
    say "ledger record and the archive then HOLDS ~0.30 KB, so on this build swap"
    say "buys time against a peak that no longer binds. See SIZING.md; monitor.sh's"
    say "replay-headroom check is what tells you when this stops being true."
    return 0
  fi
  if swapon --show=NAME --noheadings 2>/dev/null | grep -q .; then
    say "already: $(swapon --show=NAME,SIZE --noheadings | tr '\n' ' ')"
    return 0
  fi
  say "creating a ${MV_SWAP_GB}G swap file"
  run fallocate -l "${MV_SWAP_GB}G" /swapfile || run dd if=/dev/zero of=/swapfile bs=1M count=$((MV_SWAP_GB * 1024))
  run chmod 600 /swapfile
  run mkswap /swapfile
  run swapon /swapfile
  if ! grep -q '^/swapfile ' /etc/fstab 2>/dev/null; then
    run sh -c 'printf "/swapfile none swap sw 0 0\n" >> /etc/fstab'
  fi
  # Swap here is a replay backstop, not a working set. A low swappiness keeps the
  # steady-state processes resident and still lets a replay peak spill.
  run sh -c 'printf "vm.swappiness=10\n" > /etc/sysctl.d/60-multiverse.conf'
  run sysctl -q -p /etc/sysctl.d/60-multiverse.conf
}

# ---------------------------------------------------------------- binaries

phase_binaries() {
  step "binaries"
  local arch
  case "$(uname -m)" in
    x86_64)  arch=amd64 ;;
    aarch64) arch=arm64 ;;
    *) die "unsupported architecture $(uname -m)" ;;
  esac
  say "this instance is $arch"

  local installed=0 name src dst
  for name in relay archive ringstat; do
    src="$MV_STAGE_DIR/${name}-linux-${arch}"
    [ -f "$src" ] || src="$MV_STAGE_DIR/$name"
    dst="$BIN/multiverse-$name"
    [ "$name" = ringstat ] && dst="$BIN/ringstat"
    if [ ! -f "$src" ]; then
      [ "$name" = ringstat ] && { say "no ringstat staged — skipping (it is the operator's terminal tool, not the service)"; continue; }
      die "no $name binary at $MV_STAGE_DIR (looked for ${name}-linux-${arch} and $name)"
    fi
    if [ -f "$dst" ] && cmp -s "$src" "$dst"; then
      say "already current: $dst"
      continue
    fi
    # A running binary cannot be written in place (ETXTBSY) but CAN be replaced
    # by rename. `install` does that, and the old inode stays alive under the
    # running process until it restarts — which is what makes an upgrade a
    # deliberate restart rather than an accident.
    run install -m 0755 -o root -g root "$src" "$dst"
    say "installed $dst"
    installed=1
  done

  if [ "$DRY" = 0 ] && [ -x "$BIN/multiverse-relay" ]; then
    "$BIN/multiverse-relay" --help >/dev/null 2>&1 || true
    "$BIN/multiverse-relay" --help 2>&1 | grep -q -- '-mint-credential' \
      || die "$BIN/multiverse-relay does not understand --mint-credential, so it is a
     pre-contract-b/4 relay. Rebuild and re-ship."
    ( cd "$BIN" && sha256sum multiverse-relay multiverse-archive 2>/dev/null ) \
      | write_file /etc/multiverse/BINARIES.sha256 0644 root:root
    say "recorded /etc/multiverse/BINARIES.sha256"
  fi
  [ "$installed" = 1 ] && say "NOTE: new binaries do not take effect until the units restart — RESTART-POLICY.md"
  return 0
}

# ---------------------------------------------------------------- env files

phase_envfiles() {
  step "environment files"
  # Every knob these two processes have exists as BOTH a flag and an environment
  # variable, deliberately: "the rig sets one way and a service unit sets the
  # other" (go/internal/relay/main.go). So ExecStart carries no arguments at all
  # and everything an operator retunes is one file and one restart.
  write_file /etc/multiverse/relay.env 0640 "root:$MV_GROUP" <<EOF
# Generated by deploy/provision.sh from $ENV_FILE. Edit deploy.env and re-run,
# or edit here and 'systemctl restart multiverse-relay' — this file is
# regenerated on the next provision run either way.
MULTIVERSE_RELAY_LISTEN=0.0.0.0:$MV_RELAY_PORT
MULTIVERSE_RELAY_DATA_DIR=$RELAY_DATA
MULTIVERSE_RELAY_ADVERTISE_URL=$ADVERTISE_URL
MULTIVERSE_RELAY_TLS_CERT=$MV_TLSDIR/fullchain.pem
MULTIVERSE_RELAY_TLS_KEY=$MV_TLSDIR/privkey.pem
MULTIVERSE_RELAY_TLS_MIN_VERSION=1.2
MULTIVERSE_MIN_CONTRACT_VERSION=${MV_MIN_CONTRACT_VERSION:-}
MULTIVERSE_LOG_FILE=$MV_LOGDIR/relay.log
MULTIVERSE_LOG_ROTATE_MB=$MV_LOG_ROTATE_MB
MULTIVERSE_LOG_KEEP=$MV_LOG_KEEP
MULTIVERSE_LOG_LEVEL=$MV_LOG_LEVEL
# MULTIVERSE_RELAY_ADMIN_LISTEN IS DELIBERATELY ABSENT. B28's admin listener is
# compiled in and bound to nothing until an owner-level act turns it on, and off
# loopback it also demands TLS. Turning it on is a decision with an audit trail,
# not a default. To use it: bind 127.0.0.1 and reach it through an SSH tunnel.
EOF
  say "wrote /etc/multiverse/relay.env"

  write_file /etc/multiverse/archive.env 0640 "root:$MV_GROUP" <<EOF
# Generated by deploy/provision.sh from $ENV_FILE.
#
# The archive dials the relay BY NAME, not by address: a public certificate names
# the domain and nothing else, and the sidecar-side TLS client sets no
# configuration and may not ("NO TLS CONFIGURATION IS SET HERE AND NONE MAY BE").
# The /etc/hosts entry below is what keeps that dial on loopback — on Lightsail
# the public address is NAT'd and is not reachable from the instance itself.
MULTIVERSE_RELAY=$ADVERTISE_URL
MULTIVERSE_ARCHIVE_PEER_ID=$MV_ARCHIVE_PEER_ID
MULTIVERSE_ARCHIVE_DATA_DIR=$ARCHIVE_DATA
MULTIVERSE_ARCHIVE_HTTP=$MV_ARCHIVE_HTTP
MULTIVERSE_CREDENTIAL_FILE=$ARCHIVE_SECRET
MULTIVERSE_ARCHIVE_DENY_LIST=/etc/multiverse/deny-list
MULTIVERSE_LOG_FILE=$MV_LOGDIR/archive.log
MULTIVERSE_LOG_ROTATE_MB=$MV_LOG_ROTATE_MB
MULTIVERSE_LOG_KEEP=$MV_LOG_KEEP
MULTIVERSE_LOG_LEVEL=$MV_LOG_LEVEL
# Decision 3's retention horizon (§23, B33). Empty or 0 evicts nothing, which is
# the contract's default; 720h is the announced 30 days. It prunes genome BLOBS
# and never the ledger. See deploy.env.example.
MULTIVERSE_GENOME_HORIZON=${MV_ARCHIVE_GENOME_HORIZON:-}
# The memory verdict, as Go sees it. Empty means unset; the kit ships 5GiB as a
# ceiling against regression rather than as a fix. See SIZING.md §4.
GOMEMLIMIT=${MV_ARCHIVE_GOMEMLIMIT:-}
EOF
  say "wrote /etc/multiverse/archive.env"

  # The loopback pin the comment above depends on. It also means every check this
  # box runs against its own name takes the inside path: EXTERNAL reachability is
  # proven by a participant connecting, never by a curl from here.
  if ! grep -q "127.0.0.1[[:space:]].*$MV_DOMAIN" /etc/hosts 2>/dev/null; then
    run sh -c "printf '127.0.0.1 %s\n' '$MV_DOMAIN' >> /etc/hosts"
    say "added a loopback entry for $MV_DOMAIN to /etc/hosts"
  else
    say "already: /etc/hosts pins $MV_DOMAIN to loopback"
  fi

  # B30's deny list is re-read in place, so an empty file now costs nothing and
  # means moderating later is an edit rather than a restart.
  if [ ! -e /etc/multiverse/deny-list ]; then
    write_file /etc/multiverse/deny-list 0644 "root:$MV_GROUP" <<'EOF'
# B30's operator-side render deny list. One species name or peer:<peerId> per
# line; # starts a comment. It suppresses THE VIEW AND NOT THE RECORD — the
# ledger goes on holding what happened and nothing here evicts from it, so
# removal from the record is not promised and must not be promised to anybody.
# The archive re-reads this file in place: moderating costs an edit, never a
# restart.
EOF
    say "wrote an empty /etc/multiverse/deny-list"
  fi
}

# ---------------------------------------------------------------- firewall

phase_firewall() {
  step "firewall (ufw — the second of two)"
  # Order matters: allow SSH BEFORE enabling, or this session is the last one.
  # `ufw allow` is idempotent (it skips a rule it already holds), so there is no
  # reset here — a reset would drop the firewall for the width of a re-run.
  run ufw default deny incoming
  run ufw default allow outgoing
  run ufw allow 22/tcp comment 'SSH'
  run ufw allow "$MV_RELAY_PORT/tcp" comment 'multiverse relay wss'
  run ufw allow "$MV_STATUS_PORT/tcp" comment 'multiverse status page'
  if [ "$MV_ACME_MODE" = webroot ]; then
    run ufw allow 80/tcp comment 'ACME HTTP-01 only'
  fi
  run ufw --force enable
  say "$(ufw status 2>/dev/null | tr '\n' ' ' || echo 'ufw enabled')"
  say "REMINDER: the Lightsail console has its own firewall and it is the one that"
  say "faces the internet first. Open the same four ports there."
}

# ---------------------------------------------------------------- nginx, ACME half

phase_nginxacme() {
  step "nginx: the ACME half (port 80)"
  # Ubuntu's packaged default site also listens on 80 and would answer the
  # challenge from the wrong root.
  if [ -e /etc/nginx/sites-enabled/default ]; then
    run rm -f /etc/nginx/sites-enabled/default
    say "removed the packaged default site"
  fi
  render_nginx "$KIT_DIR/nginx/multiverse-10-acme.conf" /etc/nginx/conf.d/multiverse-10-acme.conf
  run nginx -t
  run systemctl enable --now nginx
  run systemctl reload nginx
  say "port 80 serves $ACME_ROOT/.well-known/acme-challenge/ and nothing else"
}

render_nginx() {
  local src="$1" dst="$2"
  [ -f "$src" ] || die "missing kit file $src"
  sed -e "s|@@MV_DOMAIN@@|$MV_DOMAIN|g" \
      -e "s|@@MV_STATUS_PORT@@|$MV_STATUS_PORT|g" \
      -e "s|@@MV_ARCHIVE_HTTP@@|$MV_ARCHIVE_HTTP|g" \
      -e "s|@@MV_TLSDIR@@|$MV_TLSDIR|g" \
      -e "s|@@ACME_ROOT@@|$ACME_ROOT|g" \
      "$src" | write_file "$dst" 0644 root:root
  say "rendered $dst"
}

# ---------------------------------------------------------------- TLS

phase_tls() {
  step "TLS: Let's Encrypt, and the hook that puts it where the relay looks"

  # The deploy hook goes in the renewal-hooks/deploy DIRECTORY rather than into
  # one certificate's renewal configuration: certbot runs every executable there
  # after any successful renewal, so a re-issue, a second name or a certbot
  # upgrade cannot leave the hook behind.
  run install -d -m 0755 /etc/letsencrypt/renewal-hooks/deploy
  run install -m 0755 "$KIT_DIR/tls-deploy-hook.sh" /etc/letsencrypt/renewal-hooks/deploy/multiverse-tls.sh
  say "installed /etc/letsencrypt/renewal-hooks/deploy/multiverse-tls.sh"

  local live="/etc/letsencrypt/live/$MV_DOMAIN/fullchain.pem"
  if [ -f "$live" ]; then
    say "already: a certificate exists for $MV_DOMAIN"
  else
    local d=()
    local n
    for n in "$MV_DOMAIN" ${MV_CERT_EXTRA_NAMES:-}; do d+=(-d "$n"); done
    say "requesting a certificate for: $MV_DOMAIN ${MV_CERT_EXTRA_NAMES:-}"
    run certbot certonly --webroot -w "$ACME_ROOT" "${d[@]}" \
      --non-interactive --agree-tos -m "$MV_ACME_EMAIL" \
      --keep-until-expiring --no-eff-email
  fi

  # Run the hook once by hand: issuance does not fire the deploy hooks directory
  # the way a renewal does.
  run env RENEWED_LINEAGE="/etc/letsencrypt/live/$MV_DOMAIN" \
    /etc/letsencrypt/renewal-hooks/deploy/multiverse-tls.sh

  # certbot's own timer is what renews. Ubuntu's package ships it enabled; say so
  # rather than assume it.
  if systemctl list-unit-files 2>/dev/null | grep -q '^certbot\.timer'; then
    run systemctl enable --now certbot.timer
    say "certbot.timer: $(systemctl is-active certbot.timer 2>/dev/null || echo unknown)"
  else
    warn "no certbot.timer on this host. Renewal is NOT automatic. Add one:"
    warn "    systemd-run --on-calendar='*-*-* 03,15:00:00' --unit=certbot-renew certbot renew -q"
  fi
  say "ROTATION COSTS NO RESTART: the relay re-reads the pair on the next TLS"
  say "handshake (CertReloader, verified in WP2). nginx does need a reload, and"
  say "the hook does it."
}

# ---------------------------------------------------------------- nginx, status half

phase_nginxstatus() {
  step "nginx: the status page (port $MV_STATUS_PORT, TLS)"
  [ -f "$MV_TLSDIR/fullchain.pem" ] || die "no certificate at $MV_TLSDIR/fullchain.pem — run the tls phase first"
  render_nginx "$KIT_DIR/nginx/multiverse-20-status.conf" /etc/nginx/conf.d/multiverse-20-status.conf
  run nginx -t
  run systemctl reload nginx
  say "https://$MV_DOMAIN:$MV_STATUS_PORT/ -> $MV_ARCHIVE_HTTP, GET and HEAD only, rate limited"
}

# ---------------------------------------------------------------- bootstrap

phase_bootstrap() {
  step "bootstrap: the archive's SUBSCRIBE credential"
  # B27: the archive is an AUTHORISED SUBSCRIBER, not a peer. Its grant is
  # disjoint from `peer` — it cannot claim a slot, and no peer credential can
  # subscribe. Minting is a startup command: it opens the data dir, acts and
  # exits, so it must not race a serving relay (see issue-join.sh).
  #
  # THIS PHASE RUNS BEFORE `systemd` AND THE ORDER IS LOAD-BEARING, not tidiness.
  # A relay whose credential store is empty REFUSES TO SERVE — "the credential
  # store holds no credentials, so this relay can admit nobody" — and exits 1.
  # On a fresh instance this mint is what makes the store non-empty, so a
  # provision run that started the units first would leave a unit in `failed`
  # and an operator reading systemd instead of the relay's own log. Verified
  # against the real binary, 2026-08-12.
  if [ -s "$ARCHIVE_SECRET" ]; then
    say "already: $ARCHIVE_SECRET"
    return 0
  fi
  if [ "$DRY" = 1 ]; then
    say "[dry-run] would mint the $MV_ARCHIVE_PEER_ID subscribe credential"
    return 0
  fi
  if systemctl is-active --quiet multiverse-relay 2>/dev/null; then
    die "the relay is serving. A mint against a serving relay's data dir is a second
     WRITER of peers.json and the running process would overwrite it from memory.
     Stop it first:  systemctl stop multiverse-relay"
  fi
  local out secret
  out="$(runuser -u "$MV_USER" -- "$BIN/multiverse-relay" \
        --data-dir "$RELAY_DATA" --advertise-url "$ADVERTISE_URL" \
        --mint-credential "$MV_ARCHIVE_PEER_ID" --grant subscribe 2>/dev/null)" \
    || die "the mint command failed"
  secret="$(printf '%s\n' "$out" | awk '$1=="secret" && NF==2 {print $2; exit}')"
  [ -n "$secret" ] || die "no join string was printed. $MV_ARCHIVE_PEER_ID may already hold a
     credential with no secret file beside it — the only recovery is a handover."
  ( umask 077; printf '%s\n' "$secret" > "$ARCHIVE_SECRET" )
  chown "root:$MV_GROUP" "$ARCHIVE_SECRET"
  chmod 0640 "$ARCHIVE_SECRET"
  say "minted $MV_ARCHIVE_PEER_ID (subscribe) -> $ARCHIVE_SECRET, mode 0640"
  say "The relay keeps a VERIFIER and cannot print this again."
  say "BACK UP $RELAY_DATA/peers.json — a lost verifier store costs a handover per peer."
}

# ---------------------------------------------------------------- systemd

phase_systemd() {
  step "systemd units"
  local u
  for u in multiverse-relay.service multiverse-archive.service \
           multiverse-monitor.service multiverse-monitor.timer \
           multiverse-backup.service multiverse-backup.timer; do
    run install -m 0644 -o root -g root "$KIT_DIR/systemd/$u" "/etc/systemd/system/$u"
  done

  # The one templated value: the archive's soft memory ceiling. A drop-in keeps
  # the unit file itself byte-identical to the one in the repository.
  run install -d -m 0755 /etc/systemd/system/multiverse-archive.service.d
  if [ -n "${MV_ARCHIVE_MEMORY_HIGH:-}" ]; then
    write_file /etc/systemd/system/multiverse-archive.service.d/10-memory.conf 0644 root:root <<EOF
# Generated by provision.sh from MV_ARCHIVE_MEMORY_HIGH. MemoryHigh THROTTLES and
# reclaims; it does not kill. There is deliberately no MemoryMax: a hard cap kills
# the replay, and the replay is the one thing that must not fail.
[Service]
MemoryHigh=$MV_ARCHIVE_MEMORY_HIGH
EOF
    say "archive MemoryHigh=$MV_ARCHIVE_MEMORY_HIGH"
  else
    run rm -f /etc/systemd/system/multiverse-archive.service.d/10-memory.conf
    say "archive MemoryHigh unset (no cgroup ceiling)"
  fi

  run systemctl daemon-reload
  run systemctl enable multiverse-relay.service multiverse-archive.service
  run systemctl enable --now multiverse-monitor.timer multiverse-backup.timer
  # start, not restart: re-running provision must not cost the map an outage.
  # An upgrade is a deliberate act and RESTART-POLICY.md is where it is written.
  run systemctl start multiverse-relay.service
  run systemctl start multiverse-archive.service
  say "enabled at boot and started. RESTART-POLICY.md is what restarts them by hand."
}

# ---------------------------------------------------------------- upgrades

phase_upgrades() {
  step "unattended security upgrades"
  write_file /etc/apt/apt.conf.d/51multiverse-unattended 0644 root:root <<'EOF'
// Security updates apply themselves. A REBOOT DOES NOT.
//
// An automatic 03:00 reboot would restart the archive unannounced, and an
// archive restart on a live map costs a ledger gap equal to its replay time —
// which grows with the ledger (RESTART-POLICY.md). Kernel reboots are therefore
// the operator's, scheduled and announced.
Unattended-Upgrade::Automatic-Reboot "false";
Unattended-Upgrade::Remove-Unused-Kernel-Packages "true";
Unattended-Upgrade::Remove-Unused-Dependencies "true";
EOF
  run systemctl enable --now unattended-upgrades
  say "security updates automatic; reboots are NOT (RESTART-POLICY.md, 'the reboot')"
  say "monitor.sh reports /var/run/reboot-required so a pending kernel is visible."
}

# ---------------------------------------------------------------- verify

archive_is_loopback_only() {
  local port="${MV_ARCHIVE_HTTP##*:}"
  ! ss -ltnH 2>/dev/null | awk '{print $4}' | grep -E ":${port}\$" | grep -qv '^127\.0\.0\.1:'
}

phase_verify() {
  step "verify"
  local ok=0 bad=0
  chk() {
    if eval "$2" >/dev/null 2>&1; then printf '     PASS  %s\n' "$1"; ok=$((ok + 1))
    else printf '     FAIL  %s\n' "$1"; bad=$((bad + 1)); fi
  }
  chk "relay unit active"        "systemctl is-active --quiet multiverse-relay"
  chk "archive unit active"      "systemctl is-active --quiet multiverse-archive"
  chk "nginx active"             "systemctl is-active --quiet nginx"
  chk "monitor timer active"     "systemctl is-active --quiet multiverse-monitor.timer"
  chk "backup timer active"      "systemctl is-active --quiet multiverse-backup.timer"
  chk "relay healthz over TLS"   "curl -fsS --max-time 10 https://$MV_DOMAIN:$MV_RELAY_PORT/healthz"
  chk "archive healthz local"    "curl -fsS --max-time 10 http://$MV_ARCHIVE_HTTP/healthz"
  chk "status page over TLS"     "curl -fsS --max-time 15 https://$MV_DOMAIN:$MV_STATUS_PORT/api/status"
  chk "archive listener is loopback only" "archive_is_loopback_only"
  chk "relay reads its key without root" \
      "runuser -u $MV_USER -- test -r $MV_TLSDIR/privkey.pem"
  chk "admin path is NOT bound" \
      "! grep -q '^MULTIVERSE_RELAY_ADMIN_LISTEN=..*' /etc/multiverse/relay.env"

  printf '\n     %d pass, %d fail\n' "$ok" "$bad"
  if [ "$bad" = 0 ]; then
    cat <<EOF

The service is up.

  map          wss://$MV_DOMAIN:$MV_RELAY_PORT$CONTRACT_B_PATH
  status page  https://$MV_DOMAIN:$MV_STATUS_PORT/
  issue a join string   sudo $MV_PREFIX/deploy/issue-join.sh <peerId>
  read the policy       $MV_PREFIX/deploy/RESTART-POLICY.md
  test the alerts       sudo -u $MV_USER $MV_PREFIX/deploy/monitor.sh --test

Two things are NOT done by this script and are the owner's:
  1. the Lightsail console firewall (four ports: 22, 80, $MV_RELAY_PORT, $MV_STATUS_PORT)
  2. a Lightsail automatic-snapshot schedule — the offsite half of backup.sh
EOF
  else
    warn "read the failures above before announcing anything to a participant."
    return 1
  fi
}

# ---------------------------------------------------------------- main

main() {
  local p
  if [ -n "$ONLY" ]; then
    printf '%s\n' $PHASES | grep -qx "$ONLY" || die "no such phase: $ONLY (--list)"
    "phase_$ONLY"
    return
  fi
  for p in $PHASES; do "phase_$p"; done
}

main
