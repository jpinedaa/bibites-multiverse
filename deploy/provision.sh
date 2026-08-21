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
#   sudo deploy/provision.sh --only binaries \
#        --stage-dir /path/to/stage \
#        --expect-sha256 archive=<sha256> --expect-sha256 relay=<sha256>
#                                             # refuse unless the STAGED artifact
#                                             # is the build you meant to ship
#
# WHAT IT NEEDS BEFORE IT RUNS. All five, and it refuses rather than half-builds:
#   1. /etc/multiverse/deploy.env, filled in (deploy.env.example is the template).
#   2. MV_DOMAIN's A record already pointing at this instance's STATIC IP.
#   3. Ports 80, 443 and 22 open in the LIGHTSAIL CONSOLE's own
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
# A deployment can name one immutable staging directory for this run. Keep the
# command-line value separate until after deploy.env is sourced, because that
# file also carries the host's default MV_STAGE_DIR and shell sourcing normally
# overwrites an exported value.
STAGE_DIR_OVERRIDE=""
# --expect-sha256 <name>=<sha256>, repeatable. What THIS run is supposed to
# install, stated by the operator and checked against the staged file before
# anything is compared or copied. See phase_binaries.
EXPECT_SHA=()

PHASES="preflight needrestart packages account directories swap binaries envfiles firewall
        nginxacme tls nginxfront bootstrap systemd streamorigin upgrades verify"

# `needrestart` IS THE FIRST PHASE AFTER PREFLIGHT, AND `packages` STAYS AHEAD
# OF `binaries`. Both are safety rules rather than preferences.
#
# needrestart runs after every apt transaction and restarts services whose
# binaries have been replaced on disk. On 2026-08-17 an ad-hoc `apt install
# unzip`, run three minutes after `--only binaries` had put new binaries in
# place and before the deliberate restart sequence had begun, was turned by
# needrestart into an undirected restart of the relay and the archive together.
# It cost 134.063 s of the permanent crossing record, 96.44 s of it with the
# relay live and no archive subscribed.
#
# The phase below installs the override that stops that happening again — for
# an apt command this script never runs as much as for its own. The ordering is
# the second half: no apt transaction of any kind belongs between staging
# binaries and the restart. RESTART-POLICY.md, "Package installs and
# needrestart", carries the rule.

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
    --stage-dir) STAGE_DIR_OVERRIDE="${2:?--stage-dir needs a path}"; shift ;;
    --expect-sha256)
      case "${2:?--expect-sha256 needs <relay|archive|ringstat>=<sha256>}" in
        relay=*|archive=*|ringstat=*) EXPECT_SHA+=("$2") ;;
        *) die "--expect-sha256 takes <relay|archive|ringstat>=<sha256>, not '$2'" ;;
      esac
      shift ;;
    -h|--help) sed -n '2,29p' "$0"; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
  shift
done

[ "$(id -u)" = 0 ] || die "run this with sudo. It creates a system user, writes /etc and drives systemd."
[ -r "$ENV_FILE" ] || die "no $ENV_FILE. Copy deploy/deploy.env.example there and fill it in."

# shellcheck source=/dev/null
set -a; . "$ENV_FILE"; set +a

# The invocation is more specific than the host default. deploy.sh has already
# checked the payload at this path and must not validate one stage, then install
# from the stale canonical stage named in deploy.env.
[ -n "$STAGE_DIR_OVERRIDE" ] && MV_STAGE_DIR="$STAGE_DIR_OVERRIDE"

KIT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

: "${MV_USER:=multiverse}"
: "${MV_GROUP:=multiverse}"
: "${MV_PREFIX:=/opt/multiverse}"
: "${MV_STATE:=/var/lib/multiverse}"
: "${MV_LOGDIR:=/var/log/multiverse}"
: "${MV_NGINX_LOGDIR:=/var/log/multiverse/nginx}"
: "${MV_GATEDIR:=/etc/multiverse/nginx-gates}"
: "${MV_TLSDIR:=/etc/multiverse/tls}"
: "${MV_STAGE_DIR:=/home/ubuntu/multiverse-stage}"
: "${MV_RELAY_PORT:=443}"
: "${MV_RELAY_BACKEND:=127.0.0.1:8795}"
: "${MV_ARCHIVE_HTTP:=127.0.0.1:8796}"
: "${MV_STREAM_ORIGIN_ENABLED:=0}"
: "${MV_STREAM_RTMP_ADDRESS:=}"
: "${MV_STREAM_PUBLISH_CIDR:=}"
: "${MV_STREAM_HLS_BACKEND:=127.0.0.1:8888}"
: "${MV_ARCHIVE_PEER_ID:=archive-main}"
: "${MV_HOMEPAGE_REPO:=jpinedaa/bibites-multiverse}"
: "${MV_HOMEPAGE_GAME_VERSION:=0.6.3.1}"
: "${MV_ACME_MODE:=webroot}"
: "${MV_SWAP_GB:=0}"
: "${MV_LOG_ROTATE_MB:=100}"
: "${MV_LOG_KEEP:=5}"
: "${MV_LOG_LEVEL:=info}"
: "${MV_PUBLIC_ENROLLMENT:=1}"
: "${MV_PUBLIC_ENROLLMENT_MAX:=256}"
: "${MV_PUBLIC_ENROLLMENT_PER_ADDRESS:=8}"
: "${MV_PUBLIC_ENROLLMENT_WINDOW_HOURS:=24}"

RELAY_DATA="$MV_STATE/relay"
ARCHIVE_DATA="$MV_STATE/archive"
BIN="$MV_PREFIX/bin"
ACME_ROOT=/var/www/acme
WWW_ROOT=/var/www
ANNOUNCE_ROOT="$WWW_ROOT/announcements"
# The viewer-presence document nginx publishes at /api/viewers. It is static
# content written by a timer, not a backend, so that reading it costs nothing and
# a stopped archive does not hide the audience from the publisher.
VIEWERS_ROOT="$WWW_ROOT/multiverse-status"
ARCHIVE_SECRET=/etc/multiverse/archive.secret
# Where restart-relay.sh drops the peer gate. The front-door template globs this
# directory, so an empty one is the normal state and a valid configuration.
#
# It is MV_GATEDIR because restart-lib.sh already reads that knob, and the two
# have to name the same directory: this script renders it into the front door
# and creates it, and restart-lib.sh writes the gate file into it. A host that
# moved the directory in deploy.env used to get a relay gate written where
# nginx never looks — the gate would report itself raised and every peer would
# still be let straight through.
GATE_DIR="$MV_GATEDIR"
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
  [ "$MV_RELAY_PORT" = 443 ] || die "MV_RELAY_PORT must be 443. nginx publishes the website and relay on standard HTTPS."
  case "$MV_RELAY_BACKEND" in
    127.0.0.1:*) ;;
    *) die "MV_RELAY_BACKEND must use 127.0.0.1. The relay backend must not be public." ;;
  esac
  case "${MV_ACME_EMAIL:-}" in
    ""|CHOOSE*) die "MV_ACME_EMAIL is unset. Let's Encrypt's expiry warning is the last line
     of defence behind monitor.sh's certificate check." ;;
  esac
  case "${MV_RETENTION:-}" in
    keep-everything|bounded-ledger|prune-genomes|graduate|rollup-window)
      say "retention rule: $MV_RETENTION" ;;
    *)
      warn "MV_RETENTION is '${MV_RETENTION:-unset}'. Decision 3 requires the rule to EXIST"
      warn "    before D24's announced ending, even if it is 'keep everything'. Provisioning"
      warn "    continues — the rule changes no command here — but SIZING.md and"
      warn "    WIND-DOWN.md both have a hole until it is set." ;;
  esac
  # D24's announced period. Nothing else in the kit reads these two, which is
  # exactly why they are checked here: a full provision succeeds with the literal
  # YYYY-MM-DD still in place, and those dates are what a participant is told
  # BEFORE joining and what may be extended by announcement but never silently
  # shortened. ANNOUNCEMENT.md and WIND-DOWN.md both consume them. Provisioning
  # continues — no command below reads them — but the release gate does.
  local pstart pend
  pstart="${MV_PERIOD_START:-}"
  pend="${MV_PERIOD_END:-}"
  if [[ ! "$pstart" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]] || [[ ! "$pend" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]]; then
    warn "MV_PERIOD_START/MV_PERIOD_END are '${pstart:-unset}' / '${pend:-unset}', which is not"
    warn "    a pair of YYYY-MM-DD dates. D24's period must be STATED before anybody"
    warn "    joins; ANNOUNCEMENT.md and WIND-DOWN.md are built from these two values,"
    warn "    so a placeholder here becomes a promise no participant can read."
    warn "    Fill them in before publication. See README.md, 'Public service commitments'."
  elif ! date -d "$pstart" +%s >/dev/null 2>&1 || ! date -d "$pend" +%s >/dev/null 2>&1; then
    warn "MV_PERIOD_START/MV_PERIOD_END look like dates and are not: '$pstart' / '$pend'."
  elif [ "$(date -d "$pend" +%s)" -le "$(date -d "$pstart" +%s)" ]; then
    warn "MV_PERIOD_END ($pend) is not after MV_PERIOD_START ($pstart). The announced period"
    warn "    would run backwards, and D24's bound is never silently shortened."
  else
    say "announced period: $pstart -> $pend ($(( ( $(date -d "$pend" +%s) - $(date -d "$pstart" +%s) ) / 86400 )) days)"
  fi

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
  #
  # THIS CHECK IS ONLY A DNS CHECK ON THE FIRST RUN. The envfiles phase pins
  # MV_DOMAIN to 127.0.0.1 in /etc/hosts on purpose — it is what keeps the
  # archive's dial to the relay on loopback — and getent reads /etc/hosts before
  # it reads DNS. So every later run reads this script's own pin back. That is
  # reported as what it is below, because a reader who meets it as a failure
  # either panics or learns to ignore the line, and the second is worse.
  local want have
  want="$(curl -fsS --max-time 5 http://169.254.169.254/latest/meta-data/public-ipv4 2>/dev/null || true)"
  have="$(getent ahostsv4 "$MV_DOMAIN" 2>/dev/null | awk 'NR==1{print $1}' || true)"
  if [ -z "$have" ]; then
    warn "$MV_DOMAIN does not resolve yet. ACME issuance will fail until it does."
  elif [ "${have#127.}" != "$have" ]; then
    say "$MV_DOMAIN -> $have — THIS INSTANCE'S OWN /etc/hosts PIN, NOT DNS."
    say "      Expected on every run after the first: the envfiles phase adds that"
    say "      entry deliberately, and getent answers from it. This check can"
    say "      therefore say nothing about the real A record. Verify it from OFF"
    say "      the box, where no pin is in the way:"
    say "          dig +short $MV_DOMAIN @1.1.1.1"
    if [ -n "$want" ]; then
      say "      It must print $want, this instance's public IPv4."
    fi
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
  say "website     https://$MV_DOMAIN/"
  say "relay       $ADVERTISE_URL"
  say "relay back  $MV_RELAY_BACKEND  (loopback)"
  say "archive     $MV_ARCHIVE_HTTP  (loopback, deliberately — see README)"
}

# ---------------------------------------------------------------- needrestart

# Where the override lands. 90- so it sorts after anything a distribution or an
# operator dropped in first, because needrestart parses conf.d in sort order and
# the last assignment wins.
#
# MV_NEEDRESTART_CONF_D is a read seam, the same idea as monitor.sh's: it lets
# this phase be exercised against a directory in a temporary tree instead of the
# host's /etc. Nothing on a real host sets it.
NEEDRESTART_CONF_D="${MV_NEEDRESTART_CONF_D:-/etc/needrestart/conf.d}"
NEEDRESTART_CONF="${NEEDRESTART_CONF_D%/conf.d}/needrestart.conf"
NEEDRESTART_OVERRIDE="$NEEDRESTART_CONF_D/90-multiverse.conf"

# needrestart_mode_is_pinned — does anything OTHER than our own override already
# choose a restart mode on this host? An operator who has pinned one has made a
# decision, and this script states what it found rather than overwriting it.
needrestart_mode_is_pinned() {
  local f
  for f in "$NEEDRESTART_CONF" "$NEEDRESTART_CONF_D"/*.conf; do
    [ -f "$f" ] || continue
    [ "$f" = "$NEEDRESTART_OVERRIDE" ] && continue
    # An uncommented assignment to $nrconf{restart}. The shipped
    # needrestart.conf carries the line commented out, which is not a pin.
    if grep -Eq '^[[:space:]]*\$nrconf\{restart\}[[:space:]]*=' "$f"; then
      return 0
    fi
  done
  return 1
}

# needrestart_override_text prints the file this phase wants on disk. It takes
# one argument: 1 to include the host-wide list-only mode, 0 to leave the mode
# alone because something else already pins it.
needrestart_override_text() {
  local with_mode="$1"
  cat <<'EOF'
# needrestart: the multiverse units are never restarted by a package transaction.
#
# INSTALLED BY deploy/provision.sh, phase `needrestart`. Edit it there.
#
# 2026-08-17T11:20:33.788Z: installing `unzip` for the AWS CLI ran an apt
# transaction. needrestart runs after every apt transaction, saw that
# multiverse-archive and multiverse-relay had had their binaries replaced on
# disk three minutes earlier, and restarted BOTH -- outside the deliberate
# record-preserving sequence in deploy/RESTART-POLICY.md, with no peer gate and
# no relay hold-down. It cost 134.063 s of the permanent crossing record,
# 96.44 s of it with the relay live and nothing recording it.
#
# The deployment lock coordinates OPERATORS. needrestart is not an operator.
# When these services restart is decided by RESTART-POLICY.md and by a person,
# never by a package manager.
$nrconf{override_rc}{qr(^multiverse-)} = 0;
EOF
  [ "$with_mode" = 1 ] || return 0
  cat <<'EOF'

# And nothing else on this host is auto-restarted by apt either: (l)ist only.
# The Ubuntu apt hook calls `needrestart -m u`, whose restart mode defaults to
# (a)utomatic, so an unattended upgrade of a shared library is enough to restart
# a service nobody asked to restart. Listing keeps the report and drops the act.
#
# This line is written ONLY when nothing else on the host pins a mode. An
# operator's own choice is left alone and provision.sh says so on the run that
# finds it.
$nrconf{restart} = q(l);
EOF
}

phase_needrestart() {
  step "needrestart override"

  local with_mode=1
  if needrestart_mode_is_pinned; then
    with_mode=0
    say "a restart mode is already pinned outside this override — leaving it alone."
    say "      Only the multiverse unit rule is written. Read the pin with:"
    say "          grep -rn '\$nrconf{restart}' /etc/needrestart/"
  fi

  local want
  want="$(needrestart_override_text "$with_mode")"

  if [ -f "$NEEDRESTART_OVERRIDE" ] && [ "$(cat "$NEEDRESTART_OVERRIDE")" = "$want" ]; then
    say "already: $NEEDRESTART_OVERRIDE"
  else
    # Only when it is absent. On a host that has needrestart the directory is
    # the package's, and re-imposing a mode and an owner on somebody else's
    # directory is not this script's business.
    if [ ! -d "$NEEDRESTART_CONF_D" ]; then
      run install -d -m 0755 -o root -g root "$NEEDRESTART_CONF_D"
    fi
    printf '%s\n' "$want" | write_file "$NEEDRESTART_OVERRIDE" 0644 root:root
    say "wrote $NEEDRESTART_OVERRIDE"
    if [ "$with_mode" = 1 ]; then
      say "      multiverse units: never auto-restarted. Host mode: list only."
    fi
  fi

  # Prove it parses AND that it decides the two units the way needrestart itself
  # would, by running needrestart's own override loop (needrestart 3.6, the
  # `foreach my $re (keys %{$nrconf{override_rc}})` in `sub _do_check`). A
  # syntax error in conf.d makes needrestart die on every apt transaction
  # afterwards, which is a broken host found by the next person to install
  # anything; a rule that parses but matches nothing is worse, because it looks
  # like protection.
  if [ "$DRY" = 0 ] && command -v perl >/dev/null 2>&1; then
    if perl -e '
        my $f = shift @ARGV;
        our %nrconf; $nrconf{override_rc} = {};
        eval do { local(@ARGV, $/) = $f; <> };
        die $@ if $@;
        for my $rc (qw(multiverse-archive.service multiverse-relay.service)) {
            my $ok = 0;
            for my $re (keys %{$nrconf{override_rc}}) {
                next unless($rc =~ /$re/);
                $ok = ($nrconf{override_rc}->{$re} == 0);
                last;
            }
            exit 3 unless($ok);
        }
        exit 0;' "$NEEDRESTART_OVERRIDE" 2>/dev/null; then
      say "parses, and both multiverse units resolve to never-restart"
    else
      die "$NEEDRESTART_OVERRIDE does not parse as needrestart configuration, or does
     not carry the multiverse rule. needrestart dies on every apt transaction
     while that is true. Remove the file and re-run this phase."
    fi
  fi

  if [ "$DRY" = 0 ] && ! command -v needrestart >/dev/null 2>&1; then
    say "note: needrestart is not installed on this host. The override is in place"
    say "      for the day it is, which is the day apt would otherwise start"
    say "      restarting these services on its own."
  fi
}

# ---------------------------------------------------------------- packages

phase_packages() {
  step "packages"
  local want=(nginx certbot ufw jq curl ca-certificates unattended-upgrades bc)
  # THE AWS CLI IS NOT ON THIS HOST BY DEFAULT. `deploy/coldcopy.sh` needs it,
  # and coldcopy.sh is the GATE on the raw-ledger window: with no CLI there is
  # no upload, with no upload there is no receipt, and with no receipt the
  # archive retires nothing and the disk fills. The failure is silent in every
  # place an operator would look — the timer runs, the archive is healthy, the
  # status page reads sensibly — which is why the package is installed here and
  # checked in `verify` rather than left to a runbook step.
  #
  #
  # THE AWS CLI IS NOT AN UBUNTU 24.04 PACKAGE, and this phase must not pretend
  # otherwise. On 2026-08-17 `apt-cache policy awscli` on this release returned
  # an empty version table with universe, restricted and multiverse all enabled
  # and the mirror reachable: the package is gone, and AWS ships v2 through its
  # own signed installer only. So `dpkg -s awscli` can NEVER succeed here, and
  # the version of this phase that tested it printed the "no ledger segment can
  # leave this host" warning below on every single run while the CLI sat there
  # working. A warning that is always wrong is a warning that stops being read,
  # which costs more than the check was ever worth.
  #
  # The CLI is therefore detected by `command -v aws` — wherever it came from —
  # and it is never put on the apt list. `unzip` is, because the vendor
  # installer needs it. `verify` fails on a missing CLI, and deploy/README.md
  # carries the install command with its signature check.
  local need_aws=0
  if [ -n "${MV_COLDCOPY_URI:-}" ] || [ "${MV_COLDCOPY:-off}" != off ]; then
    need_aws=1
    want+=(unzip)
  fi
  local missing=()
  local p
  for p in "${want[@]}"; do
    dpkg -s "$p" >/dev/null 2>&1 || missing+=("$p")
  done
  if [ "${#missing[@]}" = 0 ]; then
    say "already: ${want[*]}"
    packages_report_aws "$need_aws"
    return 0
  fi
  say "installing: ${missing[*]}"
  # NEEDRESTART_MODE=l: THIS APT TRANSACTION RESTARTS NOTHING.
  #
  # needrestart's apt hook runs `needrestart -m u`, whose restart mode defaults
  # to (a)utomatic, and it restarts any service whose binary or libraries have
  # been replaced on disk. On 2026-08-17 that turned an apt transaction into an
  # undirected restart of the relay and the archive and cost 134.063 s of the
  # permanent crossing record. `l` is list-only: the report is still printed and
  # nothing is acted on.
  #
  # This is the second lock on that door, not the first. The `needrestart` phase
  # writes /etc/needrestart/conf.d/90-multiverse.conf, which holds for apt
  # commands this script never runs — an operator's `apt install`, an
  # unattended upgrade, anything. This variable only covers the two lines below,
  # and it covers them even on a host whose override was removed by hand.
  local apt_env=(DEBIAN_FRONTEND=noninteractive NEEDRESTART_MODE=l)
  run env "${apt_env[@]}" apt-get update -qq
  # IDEMPOTENT AND OFFLINE-SAFE. This phase is re-run on every provision, and a
  # host with no archive mirror reachable must not lose the fifteen other things
  # this script does afterwards. So the install is allowed to fail: what it
  # leaves behind is a printed follow-up and a `verify` check that names the one
  # thing that is missing.
  if ! run env "${apt_env[@]}" apt-get install -y -qq "${missing[@]}"; then
    warn "apt-get install failed for: ${missing[*]}"
    warn "    Provisioning CONTINUES. Re-run '--only packages' when the archive"
    warn "    mirror is reachable, and read the 'verify' phase for what is still"
    warn "    missing."
    packages_report_aws "$need_aws"
    return 0
  fi
  packages_report_aws "$need_aws"
}

# Say what the AWS CLI actually is on this host, and say it only when this
# deployment has a cold archive to copy to. Record what landed: the deployment
# record has to name the version that wrote the first receipt.
packages_report_aws() {
  local need="$1"
  [ "$need" = 1 ] || return 0
  if command -v aws >/dev/null 2>&1; then
    say "aws cli: $(aws --version 2>&1 | head -1)  at $(command -v aws)"
    return 0
  fi
  warn "the AWS CLI is NOT on this host, and MV_COLDCOPY is on."
  warn "    No ledger segment can leave this host, so the archive retires"
  warn "    nothing — which is safe, and is not what a deployment record that"
  warn "    claims a working cold archive says happened."
  warn "    It is NOT an apt package on Ubuntu 24.04. Install it from AWS's own"
  warn "    signed installer; deploy/README.md carries the command and the"
  warn "    signature check. Then re-run '--only verify'."
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
  # The gate directory is created EMPTY and stays empty. nginx reads it, so it
  # is world-readable; only root writes into it, and only restart-relay.sh
  # should. It exists here rather than being created on demand so that an
  # operator can see the mechanism on a healthy host, and so that the one
  # command that raises the gate is a file write and not a mkdir as well.
  run install -d -m 0755 -o root -g root "$GATE_DIR"
  run install -d -m 0755 -o root -g root "$MV_PREFIX" "$BIN"
  run install -d -m 0750 -o "$MV_USER" -g "$MV_GROUP" "$MV_STATE" "$RELAY_DATA" "$ARCHIVE_DATA"
  # The closed ledger segments and their cold-copy receipts. The archive creates
  # this itself on its first start after the roll-up, and coldcopy.sh's unit
  # names it in ReadWritePaths — systemd refuses to start a unit whose
  # ReadWritePaths does not exist, so the timer would fail at every tick on a
  # host that had not yet rotated a segment.
  #
  # CREATING IT EMPTY USED TO SUPPRESS THE ARCHIVE'S ONE-TIME LEGACY MIGRATION,
  # which read the directory's mere existence as "already segmented". It did
  # exactly that on 2026-08-17. The archive now reads a marker FILE inside the
  # directory instead (go/internal/archive: migratedMarkerName), so an empty
  # directory created here means what it looks like: nothing has happened yet.
  run install -d -m 0750 -o "$MV_USER" -g "$MV_GROUP" "$ARCHIVE_DATA/segments"
  run install -d -m 0750 -o "$MV_USER" -g "$MV_GROUP" "$MV_STATE/backup" "$MV_STATE/monitor"
  # 0751, not 0750: nginx runs as www-data, which is not in the multiverse
  # group, and it must be able to REACH its own log directory below. The
  # execute bit grants traversal to a known name and nothing else — the
  # directory still cannot be listed, so relay.log and archive.log are no
  # more discoverable than they were.
  run install -d -m 0751 -o "$MV_USER" -g "$MV_GROUP" "$MV_LOGDIR"
  # The front door writes here rather than to /var/log/nginx, so that the
  # kit owns its rotation policy. See deploy/nginx/logrotate-multiverse.
  run install -d -m 0750 -o www-data -g adm "$MV_NGINX_LOGDIR"
  run install -d -m 0755 -o root -g root "$ACME_ROOT" "$ACME_ROOT/.well-known"
  # root writes viewers.json from the timer; nginx reads it as www-data.
  run install -d -m 0755 -o root -g www-data "$VIEWERS_ROOT"
  # The kit itself lives on the box, because RESTART-POLICY.md is a document an
  # operator reads at 03:00 on the machine that is misbehaving — and because the
  # on-box copy must be able to re-run this script, which needs the templates.
  run install -d -m 0755 -o root -g root "$MV_PREFIX/deploy" \
    "$MV_PREFIX/deploy/nginx" "$MV_PREFIX/deploy/systemd" \
    "$MV_PREFIX/deploy/www" "$MV_PREFIX/deploy/www/announcements"
  local f
  for f in "$KIT_DIR"/*.sh; do
    [ -e "$f" ] || continue
    run install -m 0755 -o root -g root "$f" "$MV_PREFIX/deploy/$(basename "$f")"
  done
  # Executable kit files that carry no .sh suffix. The glob above is by
  # extension, so a script named without one silently never reaches the box and
  # its unit lands with nothing to run. Name them.
  for f in service-host-sample; do
    [ -e "$KIT_DIR/$f" ] || continue
    run install -m 0755 -o root -g root "$KIT_DIR/$f" "$MV_PREFIX/deploy/$f"
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
  for f in "$KIT_DIR"/www/announcements/*; do
    [ -e "$f" ] || continue
    run install -m 0644 -o root -g root "$f" \
      "$MV_PREFIX/deploy/www/announcements/$(basename "$f")"
  done
  # The front door's rotation policy. Installed here rather than in the
  # nginxfront phase because it is kit content and must land on a host whose
  # nginx is not configured yet; `missingok` covers the gap.
  if [ -d /etc/logrotate.d ]; then
    render_template "$KIT_DIR/nginx/logrotate-multiverse" /etc/logrotate.d/multiverse-nginx
  else
    say "no /etc/logrotate.d — the front-door rotation policy was NOT installed"
  fi
  say "state $MV_STATE  logs $MV_LOGDIR  front-door logs $MV_NGINX_LOGDIR  tls $MV_TLSDIR  kit $MV_PREFIX/deploy"
}

# ---------------------------------------------------------------- swap

phase_swap() {
  step "swap (the memory verdict's second half)"
  if [ "${MV_SWAP_GB}" = 0 ] || [ -z "${MV_SWAP_GB}" ]; then
    say "MV_SWAP_GB=0 — none configured."
    say "Swap is a crash barrier, never replay capacity, and it is a per-host"
    say "decision that has to be recorded before it is made. It cannot clear an"
    say "archive-memory verdict: monitor.sh divides by physical RAM alone and"
    say "reports swap in its own check. The lever that does move resident set is"
    say "MV_ARCHIVE_GOMEMLIMIT. See 'Archive memory' and 'Swap' in SIZING.md."
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

# sha_of prints a file's sha256, or "-" when there is no file. Used for the
# before/after report below, so it never fails a phase.
sha_of() {
  [ -f "$1" ] || { printf '%s\n' -; return 0; }
  sha256sum "$1" 2>/dev/null | awk '{print $1}'
}

# expected_sha <relay|archive|ringstat> prints what --expect-sha256 said this
# run must install for that artifact, or nothing when the operator said nothing.
expected_sha() {
  local want="$1" e
  [ "${#EXPECT_SHA[@]}" -gt 0 ] || return 0
  for e in "${EXPECT_SHA[@]}"; do
    case "$e" in "$want="*) printf '%s\n' "${e#*=}"; return 0 ;; esac
  done
  return 0
}

phase_binaries() {
  step "binaries"
  local arch
  case "$(uname -m)" in
    x86_64)  arch=amd64 ;;
    aarch64) arch=arm64 ;;
    *) die "unsupported architecture $(uname -m)" ;;
  esac
  say "this instance is $arch"

  # THE STAGE DIRECTORY IS CHECKED AGAINST ITS OWN MANIFEST BEFORE ANYTHING IS
  # COMPARED, AND THE PHASE REPORTS CHECKSUMS RATHER THAN VERDICTS.
  #
  # 2026-08-17: this phase reported `already current` for all three binaries and
  # installed none of them. It was telling the truth about the wrong files — the
  # PREVIOUS deployment's artifacts were still sitting in the stage directory
  # under exactly the names it reads, and the intended binaries had been staged
  # alongside them under different names. Nothing was overwritten, nothing was
  # lost, and the deployment would have gone on to restart services onto the old
  # build believing it had upgraded them. It was caught only because the
  # operator checked the INSTALLED checksum against the intended one instead of
  # trusting this phase's own report.
  #
  # ship.sh writes SHA256SUMS beside the artifacts it staged and verifies it
  # over ssh, so the manifest is the stage directory's own statement about
  # itself: if a file has been replaced by hand since, `sha256sum -c` says so.
  # A stale-but-self-consistent stage directory it cannot catch, which is what
  # --expect-sha256 is for.
  if [ -f "$MV_STAGE_DIR/SHA256SUMS" ]; then
    if ( cd "$MV_STAGE_DIR" && sha256sum -c --quiet SHA256SUMS ) >/dev/null 2>&1; then
      say "stage manifest: SHA256SUMS verifies ($(date -u -r "$MV_STAGE_DIR/SHA256SUMS" '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null || echo 'unknown time'))"
    else
      ( cd "$MV_STAGE_DIR" && sha256sum -c SHA256SUMS 2>&1 | sed 's/^/     /' ) || true
      die "$MV_STAGE_DIR/SHA256SUMS does not match the files beside it. The stage
     directory has been edited since ship.sh wrote it, so this phase cannot say
     which build it is about to install. Re-run deploy/ship.sh, or remove the
     stage directory and stage again. NOTHING WAS INSTALLED."
    fi
  else
    warn "no $MV_STAGE_DIR/SHA256SUMS. This phase cannot check the provenance of what"
    warn "    it is about to install, and a stale artifact under a canonical name is"
    warn "    indistinguishable from a fresh one. deploy/ship.sh writes that manifest;"
    warn "    prefer it over staging by hand. Use --expect-sha256 <name>=<sum> to pin"
    warn "    what this run is supposed to install."
  fi

  local installed=0 skipped=0 name src dst before after want
  for name in relay archive ringstat; do
    src="$MV_STAGE_DIR/${name}-linux-${arch}"
    [ -f "$src" ] || src="$MV_STAGE_DIR/$name"
    dst="$BIN/multiverse-$name"
    [ "$name" = ringstat ] && dst="$BIN/ringstat"
    if [ ! -f "$src" ]; then
      [ "$name" = ringstat ] && { say "no ringstat staged — skipping (it is the operator's terminal tool, not the service)"; continue; }
      die "no $name binary at $MV_STAGE_DIR (looked for ${name}-linux-${arch} and $name)"
    fi

    # The operator's own statement of what this run is supposed to install,
    # checked against the STAGED file before the comparison that decides
    # anything. It is the only check that catches a stale stage directory whose
    # manifest agrees with itself.
    want="$(expected_sha "$name")"
    before="$(sha_of "$src")"
    if [ -n "$want" ] && [ "$before" != "$want" ]; then
      die "the staged $name is not the one this run was told to install.
     staged   $src
              $before
     expected $want
     Nothing has been installed. This is the stale-stage-directory case: the
     file under the canonical name is from an earlier ship.sh run."
    fi

    after="$(sha_of "$dst")"
    say "$name: staged    $before"
    say "$name: installed $after"
    if [ -f "$dst" ] && cmp -s "$src" "$dst"; then
      say "already current: $dst"
      skipped=$((skipped + 1))
      continue
    fi
    # A running binary cannot be written in place (ETXTBSY) but CAN be replaced
    # by rename. `install` does that, and the old inode stays alive under the
    # running process until it restarts — which is what makes an upgrade a
    # deliberate restart rather than an accident.
    run install -m 0755 -o root -g root "$src" "$dst"
    say "installed $dst  -> $(sha_of "$dst")"
    installed=1
  done

  # NOTHING CHANGED. Say so in the loudest form this script has that is not a
  # failure, because "already current" for every artifact is both the ordinary
  # state of a re-run and the exact signature of the 2026-08-17 defect. An
  # operator who expected an upgrade must not read past this.
  if [ "$installed" = 0 ] && [ "$skipped" -gt 0 ]; then
    if [ "${#EXPECT_SHA[@]}" -gt 0 ]; then
      say "nothing to install: the staged artifacts are already installed, and each"
      say "      one matched the --expect-sha256 given. This IS the upgrade."
    else
      warn "THIS PHASE INSTALLED NOTHING. Every staged artifact was already installed."
      warn "    That is correct for a re-run and it is also what a STALE STAGE"
      warn "    DIRECTORY looks like: the checksums above are the whole evidence."
      warn "    If you expected new binaries, compare the 'staged' checksums above"
      warn "    against the build you meant to ship, and re-run with"
      warn "    --expect-sha256 <name>=<sum> so this phase refuses instead of"
      warn "    reassuring. Do NOT restart on the strength of this line alone."
    fi
  fi

  if [ "$DRY" = 0 ] && [ -x "$BIN/multiverse-relay" ]; then
    local relay_help
    relay_help="$("$BIN/multiverse-relay" --help 2>&1 || true)"
    printf '%s\n' "$relay_help" | grep -q -- '-mint-credential' \
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

  # THE PARAMETER FILE ITSELF, FIRST. deploy.env.example's header states the
  # installed mode as 0640 root:$MV_GROUP, and a file copied by hand under sudo
  # is root:root however carefully the mode was typed. That ownership is not
  # cosmetic: multiverse-monitor.service and multiverse-backup.service run as
  # $MV_USER, both scripts exit 2 on an env file they cannot read, and BOTH
  # TIMERS THEN FAIL ON EVERY TICK WITH NOTHING SAYING SO — no alerts, no
  # heartbeat, no identity snapshots, discovered on the day one of them was
  # needed. So make it true here instead of assuming it, and let `verify` prove
  # it afterwards by reading the file AS $MV_USER.
  if ! getent group "$MV_GROUP" >/dev/null; then
    if [ "$DRY" = 1 ] && [ -z "$ONLY" ]; then
      say "[dry-run] the preceding account phase would create group $MV_GROUP"
    else
      die "group $MV_GROUP does not exist yet, so this phase cannot give
     $ENV_FILE the group that lets the monitor and backup timers read it. Run
     the account phase first (--only account), or run every phase in order."
    fi
  fi
  run chown "root:$MV_GROUP" "$ENV_FILE"
  run chmod 0640 "$ENV_FILE"
  say "$ENV_FILE is root:$MV_GROUP mode 0640 (monitor.sh and backup.sh read it as $MV_USER)"

  # Every knob these two processes have exists as BOTH a flag and an environment
  # variable, deliberately: "the rig sets one way and a service unit sets the
  # other" (go/internal/relay/main.go). So ExecStart carries no arguments at all
  # and everything an operator retunes is one file and one restart.
  write_file /etc/multiverse/relay.env 0640 "root:$MV_GROUP" <<EOF
# Generated by deploy/provision.sh from $ENV_FILE. Edit deploy.env and re-run,
# or edit here and 'systemctl restart multiverse-relay' — this file is
# regenerated on the next provision run either way.
MULTIVERSE_RELAY_LISTEN=$MV_RELAY_BACKEND
MULTIVERSE_RELAY_DATA_DIR=$RELAY_DATA
MULTIVERSE_RELAY_ADVERTISE_URL=$ADVERTISE_URL
MULTIVERSE_MIN_CONTRACT_VERSION=${MV_MIN_CONTRACT_VERSION:-}
MULTIVERSE_PUBLIC_ENROLLMENT=$MV_PUBLIC_ENROLLMENT
MULTIVERSE_PUBLIC_ENROLLMENT_MAX=$MV_PUBLIC_ENROLLMENT_MAX
MULTIVERSE_PUBLIC_ENROLLMENT_PER_ADDRESS=$MV_PUBLIC_ENROLLMENT_PER_ADDRESS
MULTIVERSE_PUBLIC_ENROLLMENT_WINDOW_HOURS=$MV_PUBLIC_ENROLLMENT_WINDOW_HOURS
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
# Read-only dashboard inputs. The archive publishes a fixed safe projection of
# these files; it never serves a path, raw monitor message, billing value or PID.
MULTIVERSE_MONITOR_STATE_DIR=$MV_STATE/monitor
MULTIVERSE_HOST_METRICS_FILE=$MV_STATE/metrics/service-host.jsonl
# The world the shared camera at /watch is showing, which no frame on either wire
# announces. Empty names no world, and both pages then say so. Display only: it
# changes no placement, no routing and no record. See deploy.env.example.
MULTIVERSE_BROADCAST_PEER=${MV_BROADCAST_PEER_ID:-}
# Homepage links on the landing page. The release is NOT one of these values and
# must not become one again: the page's download buttons address GitHub's
# /releases/latest, so the newest published release is what a visitor gets, and
# no deployment is part of a release. The page does NAME that release beside the
# buttons, and that number is not a value either — the archive resolves it from
# api.github.com/repos/<REPO>/releases/latest once an hour in the background, so
# it follows a release on its own and simply is not rendered when the lookup
# fails. Only the repository and the game label are set here; the repository is
# also what that lookup follows.
MULTIVERSE_HOMEPAGE_REPO=${MV_HOMEPAGE_REPO:-}
MULTIVERSE_HOMEPAGE_GAME_VERSION=${MV_HOMEPAGE_GAME_VERSION:-}
MULTIVERSE_LOG_FILE=$MV_LOGDIR/archive.log
MULTIVERSE_LOG_ROTATE_MB=$MV_LOG_ROTATE_MB
MULTIVERSE_LOG_KEEP=$MV_LOG_KEEP
MULTIVERSE_LOG_LEVEL=$MV_LOG_LEVEL
# Decision 3's retention horizon (§23, B33). Empty or 0 evicts nothing, which is
# the contract's default; 720h is the announced 30 days. It prunes genome BLOBS
# and never the ledger. See deploy.env.example.
MULTIVERSE_GENOME_HORIZON=${MV_ARCHIVE_GENOME_HORIZON:-}
# The RAW LEDGER's window. Empty takes MULTIVERSE_GENOME_HORIZON, which is the
# intended setting and is why there is no default here: one horizon, three
# mechanisms (§23 B34, extended to the raw ledger). A negative value keeps every
# closed segment forever whatever the horizon is. IT IS THE RAW LINES ONLY —
# every answer the archive publishes is kept forever at every setting — and a
# segment past the window is still not removed unless deploy/coldcopy.sh has
# written a receipt confirming an off-host copy. See deploy.env.example.
MULTIVERSE_LEDGER_WINDOW=${MV_LEDGER_WINDOW:-}
# The duplicate guard's window (§25, B38). Empty takes the contract's default of
# 48h. IT IS ALSO WHAT A RESTART COSTS: since the roll-up the replay parses only
# min(the raw window on this host, this value) of raw records, so this is the
# number monitor.sh projects the next restart's outage from. It is written here
# rather than left implicit BECAUSE monitor.sh reads MV_ARCHIVE_DEDUP_WINDOW out
# of deploy.env to make that projection — the two must be one value, and they
# are only one value if this line exists. See deploy.env.example.
MULTIVERSE_ARCHIVE_DEDUP_WINDOW=${MV_ARCHIVE_DEDUP_WINDOW:-}
# The memory verdict, as Go sees it. Empty means unset. It is a SOFT limit: it
# removes collector headroom from the resident set and cannot fail a replay, and
# it is selected from the host's archive ceiling. See SIZING.md, "Archive memory".
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
# record goes on holding what happened and nothing here removes anything from
# it, so removal from the record is not promised and must not be promised to
# anybody. The ledger's own window ages LINES BY DATE and is not a takedown
# either: it cannot name a peer, a species or an organism.
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
  run ufw allow "$MV_RELAY_PORT/tcp" comment 'multiverse HTTPS front door'
  if [ "$MV_STREAM_ORIGIN_ENABLED" = 1 ]; then
    [ -n "$MV_STREAM_RTMP_ADDRESS" ] || die "MV_STREAM_RTMP_ADDRESS is required when the stream origin is enabled"
    [ -n "$MV_STREAM_PUBLISH_CIDR" ] || die "MV_STREAM_PUBLISH_CIDR is required when the stream origin is enabled"
    run ufw allow from "$MV_STREAM_PUBLISH_CIDR" to "${MV_STREAM_RTMP_ADDRESS%:*}" \
      port "${MV_STREAM_RTMP_ADDRESS##*:}" proto tcp comment 'private broadcast ingest'
  fi
  if ufw status 2>/dev/null | grep -q '^8443/tcp'; then
    run ufw --force delete allow 8443/tcp
    say "removed the obsolete public status port 8443"
  fi
  if [ "$MV_ACME_MODE" = webroot ]; then
    run ufw allow 80/tcp comment 'ACME HTTP-01 only'
  fi
  run ufw --force enable
  say "$(ufw status 2>/dev/null | tr '\n' ' ' || echo 'ufw enabled')"
  say "REMINDER: the Lightsail console has its own firewall and it is the one that"
  say "faces the internet first. Open the same three ports there."
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
  render_template "$KIT_DIR/nginx/multiverse-10-acme.conf" /etc/nginx/conf.d/multiverse-10-acme.conf
  run nginx -t
  run systemctl enable --now nginx
  run systemctl reload nginx
  say "port 80 serves $ACME_ROOT/.well-known/acme-challenge/ and nothing else"
}

render_template() {
  local src="$1" dst="$2"
  [ -f "$src" ] || die "missing kit file $src"
  sed -e "s|@@MV_DOMAIN@@|$MV_DOMAIN|g" \
      -e "s|@@MV_RELAY_PORT@@|$MV_RELAY_PORT|g" \
      -e "s|@@MV_RELAY_BACKEND@@|$MV_RELAY_BACKEND|g" \
      -e "s|@@MV_ARCHIVE_HTTP@@|$MV_ARCHIVE_HTTP|g" \
      -e "s|@@MV_STREAM_HLS_BACKEND@@|$MV_STREAM_HLS_BACKEND|g" \
      -e "s|@@MV_TLSDIR@@|$MV_TLSDIR|g" \
      -e "s|@@ACME_ROOT@@|$ACME_ROOT|g" \
      -e "s|@@WWW_ROOT@@|$WWW_ROOT|g" \
      -e "s|@@MV_NGINX_LOGDIR@@|$MV_NGINX_LOGDIR|g" \
      -e "s|@@MV_GATEDIR@@|$GATE_DIR|g" \
      "$src" | write_file "$dst" 0644 root:root
  say "rendered $dst"
}

# ---------------------------------------------------------------- TLS

phase_tls() {
  step "TLS: Let's Encrypt, and the hook that puts it where nginx looks"

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
  elif [ "$DRY" = 1 ] && [ -z "$ONLY" ]; then
    say "[dry-run] the packages phase would install certbot.timer; the full run will enable and verify it"
  else
    warn "no certbot.timer on this host. Renewal is NOT automatic. Add one:"
    warn "    systemd-run --on-calendar='*-*-* 03,15:00:00' --unit=certbot-renew certbot renew -q"
  fi
  say "ROTATION COSTS NO RESTART: the hook reloads nginx after it installs the renewed pair."
}

# ---------------------------------------------------------------- nginx, HTTPS front door

phase_nginxfront() {
  step "nginx: website and relay front door (port $MV_RELAY_PORT, TLS)"
  if [ ! -f "$MV_TLSDIR/fullchain.pem" ]; then
    if [ "$DRY" = 1 ] && [ -z "$ONLY" ]; then
      say "[dry-run] the preceding tls phase would create $MV_TLSDIR/fullchain.pem"
    else
      die "no certificate at $MV_TLSDIR/fullchain.pem — run the tls phase first"
    fi
  fi
  # The announcements page ships with the front door because nginx, not the
  # archive, is what serves it — see the /announcements block in the template.
  run install -d -m 0755 -o root -g root "$ANNOUNCE_ROOT"
  render_template "$KIT_DIR/www/announcements/index.html" "$ANNOUNCE_ROOT/index.html"
  # THE NOTICES FILE IS OPERATOR CONTENT AND THIS SCRIPT MUST NOT OWN IT. Seed it
  # once on a host that has none; never again. An unconditional install here
  # would delete every posted notice on the next provision run, silently, and the
  # operator would find out from a participant.
  if [ -e "$ANNOUNCE_ROOT/notices.html" ]; then
    say "kept the existing $ANNOUNCE_ROOT/notices.html — posted notices are not kit content"
  else
    run install -m 0644 -o root -g root "$KIT_DIR/www/announcements/notices.html" \
      "$ANNOUNCE_ROOT/notices.html"
    say "seeded $ANNOUNCE_ROOT/notices.html from the kit"
  fi
  # The front door globs $GATE_DIR, so the directory must exist before the
  # render is tested. A glob that matches nothing is valid nginx and a missing
  # directory globs to nothing too, but an operator reading a healthy host
  # should be able to see the mechanism, so create it here as well as in the
  # directories phase — `--only nginxfront` is a real way to reach this file.
  run install -d -m 0755 -o root -g root "$GATE_DIR"
  render_template "$KIT_DIR/nginx/multiverse-20-status.conf" /etc/nginx/conf.d/multiverse-20-status.conf
  run nginx -t
  run systemctl reload nginx
  say "https://$MV_DOMAIN/ -> $MV_ARCHIVE_HTTP, GET and HEAD only, rate limited"
  say "https://$MV_DOMAIN/announcements/ -> $ANNOUNCE_ROOT, static, no archive needed"
  say "$ADVERTISE_URL -> $MV_RELAY_BACKEND, WebSocket proxy"
  say "peer gate: $GATE_DIR is empty, so /contract-b/ is open. restart-relay.sh raises it."
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
           multiverse-backup.service multiverse-backup.timer \
           multiverse-host-sample.service multiverse-host-sample.timer \
           multiverse-coldcopy.service multiverse-coldcopy.timer \
           multiverse-viewers.service multiverse-viewers.timer; do
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

  # The reboot hold-down (RESTART-POLICY.md, "Host reboot"). A reboot must be
  # able to keep the relay down while the archive replays, or the map runs live
  # with nothing recording it. `systemctl mask` CANNOT do that here and this
  # phase is the reason: these units are real files in /etc/systemd/system, with
  # no vendor copy under /lib, and a mask is a symlink to /dev/null at that same
  # path — "Failed to mask unit: File /etc/systemd/system/multiverse-relay.service
  # already exists", at the last step before the reboot. A condition on a file is
  # the hold-down that survives `enable`, `start`, and a re-run of this script.
  run install -d -m 0755 /etc/systemd/system/multiverse-relay.service.d
  write_file /etc/systemd/system/multiverse-relay.service.d/hold.conf 0644 root:root <<'EOF'
# Generated by provision.sh. RESTART-POLICY.md, "Host reboot", is the procedure.
# While this file exists the relay does not start — not at boot, not from
# `systemctl start`, which reports success and starts nothing. `systemctl status`
# names the condition. Remove the file to release it.
[Unit]
ConditionPathExists=!/etc/multiverse/RELAY-HOLD
EOF
  say "reboot hold-down: touch /etc/multiverse/RELAY-HOLD holds the relay down; rm releases it"

  run systemctl daemon-reload
  # The sampler unit declares ReadWritePaths for this directory, and systemd
  # refuses to start a unit whose ReadWritePaths does not exist.
  run install -d -m 0755 -o "$MV_USER" -g "$MV_GROUP" /var/lib/multiverse/metrics
  # Same rule for the viewer-presence unit, and the directories phase is not
  # guaranteed to have run: `--only systemd` from a fresh kit is a supported way
  # to install one new unit on a host that already has everything else.
  run install -d -m 0755 -o root -g www-data "$VIEWERS_ROOT"
  run systemctl enable multiverse-relay.service multiverse-archive.service
  run systemctl enable --now multiverse-monitor.timer multiverse-backup.timer \
    multiverse-host-sample.timer multiverse-viewers.timer
  # The off-host segment copy is enabled ONLY when a destination is configured.
  # Its unit treats MV_COLDCOPY=off as a success, so enabling it unconditionally
  # would be harmless but dishonest: a timer running hourly to print "off" is a
  # timer an operator stops reading. The archive is safe either way — with no
  # receipt it removes nothing.
  if [ "${MV_COLDCOPY:-off}" != off ]; then
    run install -d -m 0755 -o "$MV_USER" -g "$MV_GROUP" /var/lib/multiverse/archive/segments
    run systemctl enable --now multiverse-coldcopy.timer
    say "off-host segment copy: ON, to ${MV_COLDCOPY_URI:-UNSET}"
  else
    run systemctl disable --now multiverse-coldcopy.timer 2>/dev/null || true
    say "off-host segment copy: OFF (MV_COLDCOPY unset or off). The archive keeps every"
    say "    closed segment: with no confirmed off-host copy it retires nothing, whatever"
    say "    the ledger window says."
  fi
  # start, not restart: re-running provision must not cost the map an outage.
  # An upgrade is a deliberate act and RESTART-POLICY.md is where it is written.
  run systemctl start multiverse-relay.service
  run systemctl start multiverse-archive.service
  say "enabled at boot and started. RESTART-POLICY.md is what restarts them by hand."
  # This phase is an ENABLE and a START, so it defeats a `systemctl disable`
  # hold-down and it starts nothing while the hold file is there. Say both out
  # loud: a silent no-op here is how a relay stays down after an operator
  # believes the reboot is finished.
  if [ -e /etc/multiverse/RELAY-HOLD ]; then
    warn "/etc/multiverse/RELAY-HOLD exists, so the relay is HELD DOWN and did not start.
     Release it when the archive answers /healthz:  rm /etc/multiverse/RELAY-HOLD
     then  systemctl start multiverse-relay   (RESTART-POLICY.md, 'Host reboot')"
  fi
}

# ---------------------------------------------------------------- stream origin

phase_streamorigin() {
  step "live stream origin"
  local args=(--env-file "$ENV_FILE")
  [ "$DRY" = 0 ] || args+=(--dry-run)
  "$KIT_DIR/install-stream-origin.sh" "${args[@]}"
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
  say "security updates automatic; reboots are NOT (RESTART-POLICY.md, 'Host reboot')"
  say "monitor.sh reports /var/run/reboot-required so a pending kernel is visible."
}

# ---------------------------------------------------------------- verify

listener_is_loopback_only() {
  local address="$1" port="${1##*:}"
  ! ss -ltnH 2>/dev/null | awk '{print $4}' | grep -E ":${port}\$" | grep -qv '^127\.0\.0\.1:'
}

# ci_key_is_forced_command — is the CI deployment key still pinned to the gate?
#
# This is checked because the failure is SILENT AND TOTAL. The login account has
# passwordless sudo, so the difference between a key with `command="…ci-gate.sh"`
# and the same key without it is the difference between eight allowed verbs and
# root. Nothing about the running service looks different either way, and the
# deployments keep working, so only a check like this one can notice.
#
# The four restrictions are checked too, and they are not decoration: without
# no-pty a client can still ask for a terminal, and port or agent forwarding
# turns the deployment key into a route into the private network behind the box.
ci_key_is_forced_command() {
  local f="${MV_CI_AUTHORIZED_KEYS:-/home/ubuntu/.ssh/authorized_keys}" line opt
  [ -r "$f" ] || return 1
  line="$(grep -F -- "$MV_CI_DEPLOY_KEY_PUB" "$f" 2>/dev/null | head -1)"
  [ -n "$line" ] || return 1
  case "$line" in
    "command=\"$MV_PREFIX/deploy/ci-gate.sh\","*) : ;;
    *) return 1 ;;
  esac
  for opt in no-port-forwarding no-agent-forwarding no-pty no-X11-forwarding; do
    printf '%s' "$line" | grep -Fq "$opt" || return 1
  done
  return 0
}

phase_verify() {
  step "verify"
  local ok=0 bad=0
  chk() {
    if [ "$DRY" = 1 ]; then
      printf '     [dry-run] would verify  %s\n' "$1"
      ok=$((ok + 1))
    elif eval "$2" >/dev/null 2>&1; then printf '     PASS  %s\n' "$1"; ok=$((ok + 1))
    else printf '     FAIL  %s\n' "$1"; bad=$((bad + 1)); fi
  }
  chk "relay unit active"        "systemctl is-active --quiet multiverse-relay"
  chk "archive unit active"      "systemctl is-active --quiet multiverse-archive"
  if [ "$MV_STREAM_ORIGIN_ENABLED" = 1 ]; then
    chk "stream origin unit active" "systemctl is-active --quiet multiverse-stream"
    chk "stream HLS is loopback only" "listener_is_loopback_only '$MV_STREAM_HLS_BACKEND'"
  fi
  chk "nginx active"             "systemctl is-active --quiet nginx"
  chk "monitor timer active"     "systemctl is-active --quiet multiverse-monitor.timer"
  chk "backup timer active"      "systemctl is-active --quiet multiverse-backup.timer"
  chk "host sample timer active" "systemctl is-active --quiet multiverse-host-sample.timer"
  chk "viewers timer active"     "systemctl is-active --quiet multiverse-viewers.timer"
  chk "host sampler is installed" "test -x $MV_PREFIX/deploy/service-host-sample"
  chk "relay backend healthz"    "curl -fsS --max-time 10 http://$MV_RELAY_BACKEND/healthz"
  chk "archive healthz local"    "curl -fsS --max-time 10 http://$MV_ARCHIVE_HTTP/healthz"
  chk "website over TLS"         "curl -fsS --max-time 15 https://$MV_DOMAIN/"
  chk "status API over TLS"      "curl -fsS --max-time 15 https://$MV_DOMAIN/api/status"
  chk "health dashboard over TLS" "curl -fsS --max-time 15 https://$MV_DOMAIN/health | grep -q 'Live production evidence'"
  chk "health API over TLS"       "curl -fsS --max-time 15 https://$MV_DOMAIN/api/health | grep -q '\"serviceHost\"'"
  # The publisher stops when this says nobody is watching, so an endpoint that
  # 404s reads to a watcher as a broken service rather than as an empty room.
  chk "viewer presence over TLS" \
      "curl -fsS --max-time 15 https://$MV_DOMAIN/api/viewers | grep -q '\"watching\":'"
  chk "relay path rejects no credential" \
      "test \"\$(curl -sS -o /dev/null -w '%{http_code}' --max-time 10 https://$MV_DOMAIN$CONTRACT_B_PATH)\" = 401"
  chk "public enrollment rejects GET" \
      "test \"\$(curl -sS -o /dev/null -w '%{http_code}' --max-time 10 https://$MV_DOMAIN/api/enroll)\" = 403"
  chk "public enrollment is enabled" \
      "grep -qx 'MULTIVERSE_PUBLIC_ENROLLMENT=1' /etc/multiverse/relay.env"
  chk "relay backend is loopback only" "listener_is_loopback_only '$MV_RELAY_BACKEND'"
  chk "archive listener is loopback only" "listener_is_loopback_only '$MV_ARCHIVE_HTTP'"
  # Tested by READING IT AS $MV_USER, not by inspecting the mode: the mode can be
  # right while the group is wrong, and it is the read that the two timers do.
  chk "monitor and backup read $ENV_FILE without root" \
      "runuser -u $MV_USER -- test -r '$ENV_FILE'"
  chk "admin path is NOT bound" \
      "! grep -q '^MULTIVERSE_RELAY_ADMIN_LISTEN=..*' /etc/multiverse/relay.env"

  # THE needrestart OVERRIDE. Without it, any apt transaction on this host —
  # this script's own packages phase, an unattended upgrade, an operator
  # installing an unrelated tool — restarts whichever multiverse unit has had
  # its binary replaced on disk, outside every protection RESTART-POLICY.md
  # specifies. It cost 134.063 s of the permanent crossing record on 2026-08-17.
  # Checked as a FILE and as a RULE: a file that parses but matches neither unit
  # looks like protection and is not.
  chk "needrestart override is installed" "test -f $NEEDRESTART_OVERRIDE"
  chk "needrestart never restarts a multiverse unit" \
      "grep -Eq 'override_rc.*multiverse-' $NEEDRESTART_OVERRIDE"

  # THE RAW-LEDGER WINDOW'S THREE PARTS, checked because all three fail
  # silently. The archive segments its ledger whatever this deployment does, so
  # the directory below exists on every host; what varies is whether anything
  # can leave it. With no off-host copy the archive retires nothing and the disk
  # is what pays — visibly, but weeks later and in a different check.
  chk "archive segments directory exists" \
      "test -d $ARCHIVE_DATA/segments"
  if [ -n "${MV_COLDCOPY_URI:-}" ] || [ "${MV_COLDCOPY:-off}" != off ]; then
    # The two findings that would each, alone, have stopped the first segment
    # ever leaving this host.
    chk "aws cli is installed"        "command -v aws"
    chk "coldcopy timer active"       "systemctl is-active --quiet multiverse-coldcopy.timer"
    chk "coldcopy destination is set" "test -n '${MV_COLDCOPY_URI:-}'"
    # --check makes no upload. It reads the destination, so it proves the
    # credential, the bucket, the region and the endpoint in one call.
    chk "coldcopy can reach its destination" \
        "runuser -u $MV_USER -- $MV_PREFIX/deploy/coldcopy.sh --check"
  else
    say "off-host copy is not configured (MV_COLDCOPY unset or off), so the aws cli,"
    say "    the coldcopy timer and the destination are not checked. The archive"
    say "    retires NOTHING in this state, whatever MV_LEDGER_WINDOW says."
  fi

  # Only when this deployment has a CI deployment key. The key's PUBLIC half
  # goes in deploy.env as MV_CI_DEPLOY_KEY_PUB; the private half lives in the
  # CI system's secret store and never on this host or in any repository.
  # A deployment with no CI key sets nothing and is checked for nothing.
  if [ -n "${MV_CI_DEPLOY_KEY_PUB:-}" ]; then
    chk "ci-gate.sh is installed"   "test -x $MV_PREFIX/deploy/ci-gate.sh"
    chk "ci-gate.sh is root-owned"  "test \"\$(stat -c %U:%G $MV_PREFIX/deploy/ci-gate.sh)\" = root:root"
    chk "CI deploy key is pinned to the forced command" "ci_key_is_forced_command"
  fi

  # That one check is worth its own remedy, because its failure is the only one
  # here that is otherwise silent: the units stay `active` (they are timers), the
  # oneshots exit 2, and no alert can be raised by the thing that cannot start.
  if [ "$DRY" = 0 ] && ! runuser -u "$MV_USER" -- test -r "$ENV_FILE" 2>/dev/null; then
    warn "$ENV_FILE is not readable by $MV_USER. Until it is, the monitor and"
    warn "    backup timers exit 2 on every tick and NOTHING announces it: no"
    warn "    alerts, no daily heartbeat, no identity snapshots. Remedy:"
    warn "        sudo chown root:$MV_GROUP $ENV_FILE && sudo chmod 0640 $ENV_FILE"
  fi

  printf '\n     %d pass, %d fail\n' "$ok" "$bad"
  if [ "$DRY" = 1 ]; then
    say "dry-run complete: the service is unchanged; the checks above run for real after provisioning"
    return 0
  fi
  if [ "$bad" = 0 ]; then
    cat <<EOF

The service is up.

  map          $ADVERTISE_URL
  website      https://$MV_DOMAIN/
  issue a join string   sudo $MV_PREFIX/deploy/issue-join.sh <peerId>
  read the policy       $MV_PREFIX/deploy/RESTART-POLICY.md
  test the alerts       sudo -u $MV_USER $MV_PREFIX/deploy/monitor.sh --test

Two things are NOT done by this script and are the owner's:
  1. the Lightsail console firewall (three ports: 22, 80, $MV_RELAY_PORT)
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
