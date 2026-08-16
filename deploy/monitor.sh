#!/usr/bin/env bash
# Monitoring that reaches a person — Design Question 2, second obligation.
#
#   "The status page is excellent and it is a pull surface. A map that went to
#    zero peers at 03:00 is currently discovered by someone opening a browser."
#
# Run every five minutes by multiverse-monitor.timer, as the multiverse user, and
# never as root: every reading below is available without privilege on purpose.
#
#   monitor.sh              one pass; alert on anything that CHANGED
#   monitor.sh --verbose    one pass, print every check
#   monitor.sh --test       send one alert and exit — prove the channel works
#   monitor.sh --quiet      no alerts, exit code only (1 if anything is not OK)
#   monitor.sh --only NAME  run one group and nothing else: 'transfer',
#                           'hosts-pin', 'replay', 'swap' or 'billing'.
#                           deploy/test-monitor.sh drives the transfer, billing
#                           and replay-headroom arithmetic through this against
#                           fake /proc files, a fake status document, a fake
#                           reconciliation file and a fake clock, on a
#                           workstation, without root and without touching the
#                           network.
#
# WHAT IT WATCHES, and why each one is on the list rather than a longer one:
#
#   units                 the supervisor gave up. Restart=always has a limit, and
#                         the limit existing is what makes this check necessary.
#   stream origin         the private ingest and loopback HLS listener answer.
#   relay healthz         the map answers.
#   archive healthz       the record is being written.
#   relay connected       the archive is SUBSCRIBED. A connected-looking archive
#                         that is not subscribed writes nothing and says nothing.
#   status age            the map is quiet. Not the same as down.
#   peers                 WP3's done-when: "peers lost". Against an expected
#                         floor, because zero peers on a map with none joined yet
#                         is not an incident. The floor SHIPS AT 0 — meaning no
#                         floor, dark slots only — and the check keeps saying so
#                         on every run until an operator raises it.
#   lane bypass           WP3's done-when: "bypasses that persist". Risk 4 —
#                         route-around hides a dead peer, and now nobody is
#                         watching. Persistence is the whole signal: one skip is
#                         a reconnect, three runs of skips is a departure.
#   free disk             the archive stops writing correctly when its volume is
#                         full. Alert before the service reaches that state.
#   transfer              the bundle's monthly data-transfer allowance, counted
#                         in BOTH directions, read from this box's own NIC
#                         counters. Nothing else can watch it: the provider
#                         publishes no CloudWatch metric, overage is BILLED and
#                         not throttled, and the failure mode is an open-ended
#                         invoice that no status page will ever show.
#   transfer rate         a driver that just appeared. Three consecutive hours
#                         above the hourly limit is visible hours before the
#                         month-to-date line has moved far enough to see it.
#   hosts pin             the loopback pin for the service name in /etc/hosts.
#                         The archive dials the relay BY NAME, so losing that
#                         one line puts a ~54 GB/day subscription onto the
#                         billed interface and roughly doubles the bill. It is
#                         one line, nothing else watches it, and the service
#                         stays perfectly healthy while it costs money.
#   billing               the INVOICE, against the counter above. Every other
#                         cost check on this list reads a proxy; this one reads
#                         what the provider is actually metering, and the two
#                         have never disagreed by more than 1%. A daily Cost
#                         Explorer call from an OPERATOR machine leaves
#                         billing.json in the state directory — this host holds
#                         no cloud credential and must not start. It is the one
#                         check whose input arrives from off-box, so its
#                         ABSENCE is the first thing it reports: a
#                         reconciliation that quietly stopped leaves the proxy
#                         unaudited, and nothing else here would notice.
#   error lines           WP3's done-when. A rate, from the rotated logs.
#   certificate           days left on the certificate THE LISTENER SERVES, not
#                         the one on disk — which is also how a deploy hook that
#                         silently stopped running is caught.
#   replay headroom       the tripwire the hosting options document asked for:
#                         a modelled peak while replaying and a modelled
#                         resident set held afterwards, so an archive eventually
#                         outgrows its box. It is better to learn that from an
#                         alert than from a restart. THE DENOMINATOR IS PHYSICAL
#                         RAM AND NOTHING ELSE — see the swap line below. The
#                         two per-record constants are MV_REPLAY_PEAK_B and
#                         MV_REPLAY_RESIDENT_B, so a new measurement can retune
#                         the gate without a code change.
#   swap                  whether swap exists and whether it is being USED, as
#                         a separate and clearly-labelled reading. It is not
#                         headroom and it is not in the ratio above: swap is a
#                         crash barrier, and counting it as replay capacity
#                         would turn the tripwire green without changing one
#                         byte of retained state. SIZING.md reads sustained swap
#                         activity as a sizing signal, so sustained use warns.
#   genome gaps           Risk 7. The archive's arrears on genome capture, and
#                         the part a departed stranger takes with them is
#                         permanent.
#   backup freshness      a backup timer that stopped is invisible until the day
#                         it is needed.
#   reboot required       a pending kernel. Unattended upgrades do NOT reboot
#                         here, deliberately, so somebody has to know.
#
# GB HERE IS THE PROVIDER'S GB: 2^30 bytes, the same base the free-disk check
# uses and the same base the transfer allowance is quoted in. Reconciling the NIC
# counter against the bill gives a ratio of 1.01 on 2^30 and 1.08 on 10^9, so the
# decimal base is the wrong one and choosing it buys a month of false comfort.
#
# THE ALERT CHANNEL. Select one for each deployment and check that a person
# receives its self-test.
#
#   ntfy (the default)  https://ntfy.sh/<a topic nobody can guess>. No account,
#                       no signup, no key, no cost, and a phone app or a browser
#                       tab is the whole receiving end. The URL is a capability:
#                       whoever has it can read and post, which is why it lives
#                       in a 0640 env file and in no document.
#   webhook             any URL that takes a JSON POST — Discord, Slack, a
#                       personal endpoint. One variable.
#   command             a program that gets the text on stdin. This is where an
#                       SMTP relay goes (msmtp is three lines of config), or the
#                       AWS CLI publishing to an SNS topic if the owner would
#                       rather have email from a service he already pays for.
#   none                log only.
#
# WHAT NO CHANNEL HERE CAN DO. If the instance is off, nothing on the instance
# sends anything, and silence is indistinguishable from health. Use the daily
# heartbeat below and a provider-side failed-instance alarm.
set -uo pipefail

ENV_FILE="${MV_ENV_FILE:-/etc/multiverse/deploy.env}"
# MISSING AND UNREADABLE ARE DIFFERENT FAULTS and this process runs unprivileged,
# so say which one it is: a file installed root:root 0640 is present and
# unreadable, this exits 2 on every tick of the timer, and the only symptom is
# that the alerts an operator is trusting never arrive. Naming the cause here is
# what stops the next person debugging the symptom.
if [ ! -e "$ENV_FILE" ]; then
  echo "monitor: no $ENV_FILE" >&2
  exit 2
elif [ ! -r "$ENV_FILE" ]; then
  echo "monitor: $ENV_FILE exists but is NOT READABLE by $(id -un). It must be" >&2
  echo "         0640 root:${MV_GROUP:-multiverse}:" >&2
  echo "         sudo chown root:${MV_GROUP:-multiverse} $ENV_FILE && sudo chmod 0640 $ENV_FILE" >&2
  exit 2
fi
# shellcheck source=/dev/null
set -a; . "$ENV_FILE"; set +a

: "${MV_STATE:=/var/lib/multiverse}"
: "${MV_LOGDIR:=/var/log/multiverse}"
: "${MV_TLSDIR:=/etc/multiverse/tls}"
: "${MV_RELAY_PORT:=443}"
: "${MV_RELAY_BACKEND:=127.0.0.1:8795}"
: "${MV_ARCHIVE_HTTP:=127.0.0.1:8796}"
: "${MV_STREAM_ORIGIN_ENABLED:=0}"
: "${MV_STREAM_HLS_BACKEND:=127.0.0.1:8888}"
: "${MV_EXPECTED_PEERS:=0}"
: "${MV_STATUS_AGE_WARN_MS:=120000}"
: "${MV_BYPASS_RUNS:=3}"
: "${MV_DISK_WARN_PCT:=25}"
: "${MV_DISK_CRIT_PCT:=12}"
: "${MV_ERROR_ALERT:=5}"
: "${MV_GAPS_DELTA_ALERT:=20000}"
: "${MV_CERT_MIN_DAYS:=10}"
: "${MV_REPLAY_HEADROOM:=0.85}"
# THE PER-RECORD MEMORY MODEL, in whole bytes per ledger record. These are the
# only two numbers the replay-headroom check has, and they are a MODEL rather
# than a measurement of this process: SIZING.md's "Archive memory" derives them
# and owns their values. They live here as parameters so that the next
# measurement can retune the gate on a running host without a code change, and
# so that a host whose archive is measured to be cheaper or dearer than the
# reference workload can say so. Changing them changes when the gate trips and
# nothing else.
: "${MV_REPLAY_RESIDENT_B:=300}"
: "${MV_REPLAY_PEAK_B:=184}"
# SWAP IS WATCHED, NOT COUNTED. Percentage of the swap device in use that this
# calls sustained, and how many consecutive passes it must hold to be reported.
# Three passes of the five-minute timer is about a quarter of an hour, which is
# the difference between one replay spilling and a box that is short of RAM.
: "${MV_SWAP_USED_WARN_PCT:=25}"
: "${MV_SWAP_USED_RUNS:=3}"
# The bundle's monthly transfer allowance in the PROVIDER's GB (2^30 bytes),
# counted in BOTH directions. Overage is billed per GB and is not throttled.
: "${MV_TRANSFER_ALLOWANCE_GB:=3072}"
: "${MV_TRANSFER_WARN_PCT:=80}"
: "${MV_TRANSFER_HYST_PCT:=3}"
# Empty means "the default-route interface from /proc/net/route". The monitor
# unit sets RestrictAddressFamilies without AF_NETLINK, so `ip` cannot run here
# and must never be introduced: it would fail under systemd and work by hand.
: "${MV_TRANSFER_IFACE:=}"
: "${MV_TRANSFER_HOURLY_GB:=9}"
: "${MV_TRANSFER_HOURLY_RUNS:=3}"
# THE RECONCILIATION, written into the state directory by an operator machine
# running deploy/ce-reconcile.sh once a day. Hours before a reading is treated
# as stale, and how far the metric-over-invoice ratio may sit from 1 before the
# two instruments are called into question. 36 hours is a daily job plus a
# missed run plus the provider's own ~14-hour billing lag, so a single failed
# run is not an alert and two are. The ratio band is 10 percent around a
# measured, systematic 1.01: wide enough that the known residual never trips it,
# narrow enough that a counter reading the wrong interface would.
: "${MV_BILLING_STALE_HOURS:=36}"
: "${MV_BILLING_RATIO_TOL:=0.10}"
: "${MV_ALERT_KIND:=ntfy}"
: "${MV_ALERT_URL:=}"
: "${MV_ALERT_COMMAND:=}"
: "${MV_ALERT_REPEAT_HOURS:=12}"
: "${MV_HEARTBEAT_HOUR:=9}"

# READ SEAMS, so the arithmetic can be exercised off the host. Each one defaults
# to the real file and is overridden only by deploy/test-monitor.sh.
: "${MV_PROC_NET_DEV:=/proc/net/dev}"
: "${MV_PROC_NET_ROUTE:=/proc/net/route}"
: "${MV_HOSTS_FILE:=/etc/hosts}"
: "${MV_PROC_MEMINFO:=/proc/meminfo}"

STATE="$MV_STATE/monitor"
ARCHIVE_DATA="$MV_STATE/archive"
VERBOSE=0
QUIET=0
ONLY=""
# MV_NOW is the same kind of seam: one clock reading, taken once, that every
# check below derives its dates from. Nothing here calls date for "now" twice.
NOW="${MV_NOW:-$(date -u +%s)}"
WORST=OK
SUMMARY=""

mkdir -p "$STATE" 2>/dev/null || true
TMP="$(mktemp -d "${TMPDIR:-/tmp}/mvmon.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT

# ---------------------------------------------------------------- state

sget() { cat "$STATE/$1" 2>/dev/null || printf '%s' "${2:-}"; }
sset() { printf '%s' "$2" >"$STATE/$1.new" 2>/dev/null && mv -f "$STATE/$1.new" "$STATE/$1"; }

# ---------------------------------------------------------------- the channel

notify() {
  local sev="$1" check="$2" text="$3" title
  title="[$sev] multiverse ${MV_DOMAIN:-} — $check"
  case "$MV_ALERT_KIND" in
    ntfy)
      [ -n "$MV_ALERT_URL" ] || { logger -t multiverse-monitor -- "no MV_ALERT_URL; alert dropped: $title"; return 1; }
      local prio=default tags=warning
      [ "$sev" = CRIT ] && { prio=urgent; tags=rotating_light; }
      [ "$sev" = OK ]   && { prio=low;    tags=white_check_mark; }
      curl -fsS --max-time 15 \
        -H "Title: $title" -H "Priority: $prio" -H "Tags: $tags" \
        -d "$text" "$MV_ALERT_URL" >/dev/null ;;
    webhook)
      [ -n "$MV_ALERT_URL" ] || return 1
      local payload
      payload="$(printf '%s\n%s' "$title" "$text" | jq -Rs '{text: .}')"
      curl -fsS --max-time 15 -H 'Content-Type: application/json' \
        -d "$payload" "$MV_ALERT_URL" >/dev/null ;;
    command)
      [ -n "$MV_ALERT_COMMAND" ] || return 1
      # Unquoted on purpose: the operator's value may carry its own arguments,
      # e.g. MV_ALERT_COMMAND='msmtp owner@example.org'. It comes from a 0640
      # file that only root writes.
      # shellcheck disable=SC2086
      printf '%s\n%s\n' "$title" "$text" | $MV_ALERT_COMMAND "$sev" "$check" ;;
    none) return 0 ;;
    *) logger -t multiverse-monitor -- "unknown MV_ALERT_KIND=$MV_ALERT_KIND"; return 1 ;;
  esac
}

# report <check> <severity> <message>
#
# Alerts on a CHANGE of severity, plus a repeat every MV_ALERT_REPEAT_HOURS while
# a check stays bad, plus a recovery line when it comes back. A monitor that
# alerts on every tick is a monitor whose alerts get muted.
report() {
  local check="$1" sev="$2" msg="$3"
  local prev last_ts age
  prev="$(sget "sev.$check" OK)"
  last_ts="$(sget "at.$check" 0)"
  age=$(( NOW - last_ts ))

  [ "$VERBOSE" = 1 ] && printf '  %-18s %-4s %s\n' "$check" "$sev" "$msg"
  case "$sev" in
    CRIT) WORST=CRIT ;;
    WARN) [ "$WORST" = CRIT ] || WORST=WARN ;;
  esac
  [ "$sev" != OK ] && SUMMARY="$SUMMARY
  $sev $check: $msg"

  if [ "$QUIET" = 1 ]; then
    sset "sev.$check" "$sev"
    return 0
  fi

  if [ "$sev" != "$prev" ]; then
    if [ "$sev" = OK ]; then
      notify OK "$check" "recovered: $msg" && logger -t multiverse-monitor -- "recovered $check"
    else
      notify "$sev" "$check" "$msg" && logger -t multiverse-monitor -- "$sev $check: $msg"
    fi
    sset "sev.$check" "$sev"
    sset "at.$check" "$NOW"
  elif [ "$sev" != OK ] && [ "$age" -ge $(( MV_ALERT_REPEAT_HOURS * 3600 )) ]; then
    notify "$sev" "$check" "still: $msg"
    sset "at.$check" "$NOW"
  fi
}

# ---------------------------------------------------------------- arguments

while [ $# -gt 0 ]; do
  case "$1" in
    -v|--verbose) VERBOSE=1 ;;
    -q|--quiet) QUIET=1 ;;
    --only) ONLY="${2:?--only needs a check group: transfer, hosts-pin, replay, swap or billing}"; shift ;;
    --test)
      if notify OK self-test "This is a test from $(hostname) at $(date -u +%FT%TZ). If you are reading it, the alert channel works."; then
        echo "sent one $MV_ALERT_KIND alert"
        exit 0
      fi
      echo "the alert channel is NOT working: kind=$MV_ALERT_KIND url set=$([ -n "$MV_ALERT_URL" ] && echo yes || echo no)" >&2
      exit 1 ;;
    -h|--help) sed -n '2,121p' "$0"; exit 0 ;;
    *) echo "monitor: unknown argument $1" >&2; exit 2 ;;
  esac
  shift
done

# ---------------------------------------------------------------- the checks

check_units() {
  local u state bad=""
  local units=(multiverse-relay multiverse-archive nginx)
  [ "$MV_STREAM_ORIGIN_ENABLED" = 1 ] && units+=(multiverse-stream)
  for u in "${units[@]}"; do
    state="$(systemctl is-active "$u" 2>/dev/null || true)"
    [ "$state" = active ] || bad="$bad $u=$state"
  done
  if [ -n "$bad" ]; then
    report units CRIT "systemd is not running:$bad. If it says 'failed', the restart limit was reached — 'systemctl status' then RESTART-POLICY.md."
  else
    report units OK "${units[*]} active"
  fi
}

check_stream_origin() {
  [ "$MV_STREAM_ORIGIN_ENABLED" = 1 ] || return 0
  local code
  code="$(curl -sS --max-time 10 -o /dev/null -w '%{http_code}' \
    "http://${MV_STREAM_HLS_BACKEND}/bibites/index.m3u8" 2>/dev/null || true)"
  case "$code" in
    200|302|404) report stream-origin OK "HLS answers on ${MV_STREAM_HLS_BACKEND}" ;;
    *) report stream-origin CRIT "HLS does not answer on ${MV_STREAM_HLS_BACKEND} (HTTP ${code:-none})" ;;
  esac
}

check_relay_healthz() {
  local out
  out="$(curl -fsS --max-time 10 "http://${MV_RELAY_BACKEND}/healthz" 2>&1)"
  if [ "$out" = ok ]; then
    report relay-healthz OK "answers on ${MV_RELAY_BACKEND}"
  else
    report relay-healthz CRIT "no healthy answer on http://${MV_RELAY_BACKEND}/healthz — $out"
  fi
}

# The fourth read seam: the live document normally comes from the archive below,
# and deploy/test-monitor.sh points this at a fixture so that the checks reading
# it can be exercised without an archive.
STATUS_JSON="${MV_STATUS_JSON:-$TMP/status.json}"
check_archive_healthz() {
  if ! curl -fsS --max-time 10 "http://${MV_ARCHIVE_HTTP}/healthz" >/dev/null 2>&1; then
    report archive-healthz CRIT "the archive's own listener does not answer on ${MV_ARCHIVE_HTTP}"
    return 1
  fi
  if ! curl -fsS --max-time 25 "http://${MV_ARCHIVE_HTTP}/api/status" -o "$STATUS_JSON" 2>/dev/null; then
    report archive-healthz WARN "healthz answers but /api/status does not — the archive may still be replaying the ledger (RESTART-POLICY.md)"
    return 1
  fi
  report archive-healthz OK "serving /api/status"
  return 0
}

check_map() {
  [ -s "$STATUS_JSON" ] || return 0
  local connected have age live dark
  connected="$(jq -r '.relayConnected' "$STATUS_JSON" 2>/dev/null)"
  have="$(jq -r '.haveStatus' "$STATUS_JSON" 2>/dev/null)"
  age="$(jq -r '.statusAgeMs // 0' "$STATUS_JSON" 2>/dev/null)"
  live="$(jq -r '.totals.liveSlots // 0' "$STATUS_JSON" 2>/dev/null)"
  dark="$(jq -r '.totals.darkSlots // 0' "$STATUS_JSON" 2>/dev/null)"

  if [ "$connected" != true ]; then
    report subscribed CRIT "the archive is running but NOT subscribed to the relay. Nothing is being recorded."
  else
    report subscribed OK "subscribed"
  fi

  if [ "$have" != true ]; then
    report status-age WARN "the archive has never seen a PEER_STATUS since it started"
  elif [ "$age" -gt "$MV_STATUS_AGE_WARN_MS" ] 2>/dev/null; then
    report status-age WARN "the last PEER_STATUS is $(( age / 1000 ))s old (limit $(( MV_STATUS_AGE_WARN_MS / 1000 ))s). The map is quiet, which is not the same as down."
  else
    report status-age OK "last broadcast $(( age / 1000 ))s ago"
  fi

  # MV_EXPECTED_PEERS=0 MEANS "NO FLOOR": the live count cannot be too low, and
  # this check watches dark slots only. That is the shipped default for one
  # reason — a floor of 1 makes the FIRST tick against a freshly provisioned,
  # empty map a CRIT, and an operator whose first-ever alert is a false one
  # learns to mute the channel while he is still deciding whether to trust it.
  # It is the wrong value once anybody has joined, because "peers lost" is one of
  # WP3's own done-when clauses, so the OK line below keeps saying so until
  # somebody raises it rather than letting an unwatched floor go quiet.
  if [ "${MV_EXPECTED_PEERS:-0}" -le 0 ] 2>/dev/null && [ "${dark:-0}" = 0 ]; then
    report peers OK "$live live, 0 dark — NO FLOOR SET (MV_EXPECTED_PEERS=0), so this check is watching dark slots only. Raise it to the map's peer count once they have joined, or 'peers lost' is unwatched."
  elif [ "${MV_EXPECTED_PEERS:-0}" -gt 0 ] 2>/dev/null && [ "${live:-0}" -lt "$MV_EXPECTED_PEERS" ] 2>/dev/null; then
    report peers CRIT "$live live slot(s), expected at least $MV_EXPECTED_PEERS; $dark dark. A peer that went dark holds its slot number forever — the address never returns to the pool."
  elif [ "${dark:-0}" -gt 0 ] 2>/dev/null; then
    report peers WARN "$live live, $dark DARK. A dark slot is routed around, so its neighbours' traffic looks healthy while it is gone."
  else
    report peers OK "$live live, 0 dark"
  fi
}

check_bypass() {
  [ -s "$STATUS_JSON" ] || return 0
  local skips runs
  # LaneView is flat: one derived effective lane per entry, and `skipped` is the
  # list of slots §8 routed AROUND to find a deliverable target.
  skips="$(jq -r '[.lanes[]? | (.skipped // []) | length] | add // 0' "$STATUS_JSON" 2>/dev/null)"
  case "$skips" in ''|*[!0-9]*) skips=0 ;; esac
  runs="$(sget bypass.runs 0)"
  case "$runs" in ''|*[!0-9]*) runs=0 ;; esac

  if [ "$skips" -gt 0 ]; then
    runs=$(( runs + 1 ))
  else
    runs=0
  fi
  sset bypass.runs "$runs"

  if [ "$runs" -ge "$MV_BYPASS_RUNS" ]; then
    report lane-bypass WARN "$skips lane bypass(es) have persisted for $runs consecutive checks (~$(( runs * 5 )) min). Route-around is working, which is exactly why nobody notices the peer it is routing around. Read the status page's lanes."
  else
    report lane-bypass OK "$skips bypass(es) this pass, $runs consecutive"
  fi
}

check_disk() {
  local pct avail line
  line="$(df -P "$ARCHIVE_DATA" 2>/dev/null | awk 'NR==2 {print $5" "$4}')"
  pct="${line%% *}"; pct="${pct%\%}"
  avail="${line##* }"
  case "$pct" in ''|*[!0-9]*) report disk WARN "cannot read df for $ARCHIVE_DATA"; return ;; esac
  local free=$(( 100 - pct ))
  local availg=$(( avail / 1024 / 1024 ))

  # Days to full, from this box's own last 24 h rather than from a model. The
  # model is in SIZING.md and is what sizes the volume; this is what watches it.
  #
  # The sample is USED SPACE ON THE FILESYSTEM, not `du` over the archive: the
  # genome store is half a million small files and walking it every five minutes
  # would be this monitor's largest cost on a two-vCPU box. df already knows.
  local used_now used_then t_then days=""
  used_now="$(df -P "$ARCHIVE_DATA" 2>/dev/null | awk 'NR==2 {print $3}')"
  used_then="$(sget disk.used24 "")"
  t_then="$(sget disk.used24.at 0)"
  if [ -z "$used_then" ] || [ "${t_then:-0}" -le 0 ] 2>/dev/null; then
    sset disk.used24 "$used_now"; sset disk.used24.at "$NOW"
  elif [ $(( NOW - t_then )) -ge 82800 ]; then
    local grew=$(( used_now - used_then ))
    [ "$grew" -gt 0 ] && days=$(( avail / grew ))
    sset disk.used24 "$used_now"; sset disk.used24.at "$NOW"
  fi
  local tail=""
  [ -n "$days" ] && tail=" — about $days day(s) left at the last 24 h's growth"

  if [ "$free" -le "$MV_DISK_CRIT_PCT" ]; then
    report disk CRIT "${free}% free (${availg} GB)$tail. Grow the volume or apply the announced retention rule before writes fail."
  elif [ "$free" -le "$MV_DISK_WARN_PCT" ]; then
    report disk WARN "${free}% free (${availg} GB)$tail. SIZING.md has the arithmetic and the options."
  else
    report disk OK "${free}% free (${availg} GB)$tail"
  fi
}

# ------------------------------------------------------------ data transfer
#
# THE ONLY THING WATCHING THE BILL. The bundle includes a fixed monthly transfer
# allowance counted in BOTH directions. Overage is billed per GB and is NOT
# throttled, so going over is an invoice rather than an outage: the map stays up,
# every other check here stays green, and the only symptom arrives next month.
#
# It reads /proc/net/dev, and that is the whole design. The provider publishes no
# CloudWatch metric, so there is no alarm to write; its own API needs a
# credential, and this box deliberately holds none. The NIC counter tracks the
# provider's NetworkIn+NetworkOut to 0.002% outbound and 0.9% inbound, needs no
# credential, no API call, no privilege and no network, and it cannot be the
# reason an access key exists on a public-facing host.
#
# GB is 2^30 bytes throughout — the base the allowance is quoted in.

MONTH_START=0
MONTH_SECS=0
# month_window — the UTC month containing NOW, as a start second and a length.
# Month length is asked of date rather than assumed, because a burn-down line
# drawn on 30 days in a 31-day month is 3% wrong in the alarming direction.
month_window() {
  local first
  first="$(date -u -d "@$NOW" +%Y-%m-01)"
  MONTH_START="$(date -u -d "$first" +%s)"
  MONTH_SECS=$(( $(date -u -d "$first +1 month" +%s) - MONTH_START ))
  [ "$MONTH_SECS" -gt 0 ] 2>/dev/null || MONTH_SECS=2592000
}

# transfer_iface — the interface the bill is written against.
#
# THE UNIT FORBIDS AF_NETLINK (RestrictAddressFamilies in
# systemd/multiverse-monitor.service), so `ip route` cannot run here and must
# never be introduced: it would fail under systemd and work perfectly by hand,
# which is the worst pair of behaviours a check can have. /proc/net/route is a
# plain file. Its default route is the row whose destination and mask are both
# all-zero.
transfer_iface() {
  if [ -n "$MV_TRANSFER_IFACE" ]; then
    printf '%s' "$MV_TRANSFER_IFACE"
    return
  fi
  awk 'NR>1 && $2=="00000000" && $8=="00000000" {print $1; exit}' \
    "$MV_PROC_NET_ROUTE" 2>/dev/null
}

check_transfer() {
  month_window
  local iface pair rx tx
  iface="$(transfer_iface)"
  if [ -z "$iface" ]; then
    report transfer WARN "no default-route interface in $MV_PROC_NET_ROUTE, so the transfer allowance is not being watched at all. Name the billed interface in MV_TRANSFER_IFACE."
    return
  fi
  # A long interface name is glued to its colon in /proc/net/dev, so split on the
  # colon instead of trusting the space. Field 2 is receive bytes, field 10 is
  # transmit bytes.
  pair="$(awk -v i="$iface" '{ sub(/:/, " ") } $1 == i { print $2, $10; exit }' \
    "$MV_PROC_NET_DEV" 2>/dev/null)"
  rx="${pair%% *}"
  tx="${pair##* }"
  case "${rx:-x}${tx:-x}" in
    *[!0-9]*)
      report transfer WARN "no usable byte counters for $iface in $MV_PROC_NET_DEV, so the transfer allowance is not being watched."
      return ;;
  esac

  # THE ACCUMULATOR. The counters are cumulative since boot, so the first reading
  # is a seed and never a delta: booking it would charge the whole uptime to this
  # month in one tick. A counter that went BACKWARDS is a reboot, and the new
  # value is then the whole delta.
  local lrx ltx drx dtx delta
  lrx="$(sget net.rx "")"; case "$lrx" in ''|*[!0-9]*) lrx="" ;; esac
  ltx="$(sget net.tx "")"; case "$ltx" in ''|*[!0-9]*) ltx="" ;; esac
  if [ -z "$lrx" ] || [ -z "$ltx" ]; then
    drx=0; dtx=0
  else
    if [ "$rx" -lt "$lrx" ]; then drx="$rx"; else drx=$(( rx - lrx )); fi
    if [ "$tx" -lt "$ltx" ]; then dtx="$tx"; else dtx=$(( tx - ltx )); fi
  fi
  sset net.rx "$rx"
  sset net.tx "$tx"
  delta=$(( drx + dtx ))

  local month prev_month mtd
  month="$(date -u -d "@$NOW" +%Y-%m)"
  prev_month="$(sget net.month "")"
  if [ -z "$prev_month" ]; then
    # Nothing recorded yet: ADOPT the month rather than roll over into it, so a
    # hand-seeded net.mtd and net.since survive the first tick after seeding.
    sset net.month "$month"
    [ -n "$(sget net.since "")" ] || sset net.since "$NOW"
  elif [ "$prev_month" != "$month" ]; then
    # The delta is added AFTER the reset, so the sliver of traffic that straddles
    # the first of the month — at most one tick, five minutes — is booked to the
    # new month. Against a 3 TB allowance that is a rounding error, and the
    # alternative is a reset that silently discards it.
    sset net.mtd 0
    sset net.since "$NOW"
    sset net.month "$month"
  fi
  mtd="$(sget net.mtd 0)"; case "$mtd" in ''|*[!0-9]*) mtd=0 ;; esac
  mtd=$(( mtd + delta ))
  sset net.mtd "$mtd"

  # THE HOURLY BUCKETS. A whole five-minute delta is booked to the hour that
  # contains NOW, so one bucket can be wrong by one tick — at most 8% of an hour.
  # That is fine for a 9 GB/h trip against a 4-5 GB/h baseline, and it is not
  # fine for billing, which is what net.mtd above is for.
  local hourkey prevkey hourbytes hours hot
  hourkey="$(date -u -d "@$NOW" +%Y-%m-%dT%H)"
  prevkey="$(sget net.hourkey "")"
  hourbytes="$(sget net.hourbytes 0)"; case "$hourbytes" in ''|*[!0-9]*) hourbytes=0 ;; esac
  hours="$(sget net.hours "")"
  hot="$(sget net.hot 0)"; case "$hot" in ''|*[!0-9]*) hot=0 ;; esac
  if [ -z "$prevkey" ]; then
    sset net.hourkey "$hourkey"
    hourbytes=0
  elif [ "$prevkey" != "$hourkey" ]; then
    hours="${hours:+$hours }$hourbytes"
    # Keep the last 24 CLOSED hours. Trimmed with text tools rather than awk on
    # purpose: awk would print a ten-digit byte count back as 5e+09.
    hours="$(printf '%s' "$hours" | tr ' ' '\n' | sed '/^$/d' | tail -n 24 | tr '\n' ' ')"
    hours="${hours% }"
    if awk -v b="$hourbytes" -v g="$MV_TRANSFER_HOURLY_GB" 'BEGIN{exit !(b >= g * 1073741824)}'; then
      hot=$(( hot + 1 ))
    else
      hot=0
    fi
    sset net.hours "$hours"
    sset net.hot "$hot"
    sset net.hourkey "$hourkey"
    hourbytes=0
  fi
  hourbytes=$(( hourbytes + delta ))
  sset net.hourbytes "$hourbytes"

  local since age mtd_gb
  since="$(sget net.since "$NOW")"; case "$since" in ''|*[!0-9]*) since="$NOW" ;; esac
  age=$(( NOW - since ))
  [ "$age" -ge 0 ] || age=0
  mtd_gb="$(awk -v b="$mtd" 'BEGIN{printf "%.1f", b / 1073741824}')"

  # WARM-UP. Six hours of observation before any projection is allowed to raise
  # anything. A trailing rate computed from one tick of a freshly seeded counter
  # is noise, and the first alert an operator ever receives decides whether the
  # channel gets trusted or muted.
  if [ "$age" -lt 21600 ]; then
    report transfer OK "warming up: $mtd_gb GB observed over $(( age / 3600 ))h on $iface; the projection needs six hours"
    return
  fi

  # PROJECTION A — the month-to-date burn-down. Where this month's usage sits
  # against a straight line from zero to the allowance. It is the one that
  # cannot be fooled by a quiet day, and the one that is meaningless early in a
  # month whose first days were not observed.
  local elapsed pct_a
  elapsed=$(( NOW - MONTH_START ))
  [ "$elapsed" -gt 0 ] || elapsed=1
  pct_a="$(awk -v m="$mtd" -v a="$MV_TRANSFER_ALLOWANCE_GB" -v e="$elapsed" -v s="$MONTH_SECS" 'BEGIN{
      line = a * 1073741824 * (e / s)
      if (line <= 0) { print "0.0"; exit }
      printf "%.1f", 100 * m / line
    }')"

  # PROJECTION B — the trailing rate. What the last 24 closed hours would cost
  # over a whole month. It is the one that sees a change of behaviour today
  # rather than at the end of the month. With no closed hour it is UNKNOWN, and
  # unknown is not zero: A is then used alone.
  local nhours proj_b_gb pct_b
  nhours="$(awk -v h="$hours" 'BEGIN{print split(h, a, " ")}')"
  proj_b_gb=""
  pct_b=""
  if [ "${nhours:-0}" -gt 0 ] 2>/dev/null; then
    proj_b_gb="$(awk -v h="$hours" -v s="$MONTH_SECS" 'BEGIN{
        n = split(h, a, " "); t = 0
        for (i = 1; i <= n; i++) t += a[i]
        printf "%.0f", (t / n) * (s / 3600) / 1073741824
      }')"
    pct_b="$(awk -v h="$hours" -v s="$MONTH_SECS" -v a="$MV_TRANSFER_ALLOWANCE_GB" 'BEGIN{
        n = split(h, x, " "); t = 0
        for (i = 1; i <= n; i++) t += x[i]
        printf "%.1f", 100 * ((t / n) * (s / 3600)) / (a * 1073741824)
      }')"
  fi

  local pct drove
  pct="$pct_a"
  drove="the month-to-date line is at ${pct_a}% of its burn-down"
  if [ -n "$pct_b" ] && awk -v b="$pct_b" -v a="$pct_a" 'BEGIN{exit !(b > a)}'; then
    pct="$pct_b"
    drove="the last 24 h project a full month of ${proj_b_gb} GB"
  fi

  # HYSTERESIS, because the steady state sits within a point or two of the line.
  # 3,072 GB over 31 days is 99.1 GB/day and the box draws about 98.5, so a
  # trailing projection lands on 99% and stays there. Without hysteresis that
  # would alert, recover, alert and recover every five minutes, and the channel
  # would be muted before lunch. Stepping UP is immediate; stepping DOWN needs
  # MV_TRANSFER_HYST_PCT points of clearance below whichever threshold raised it.
  local prev sev
  prev="$(sget sev.transfer OK)"
  sev=OK
  if awk -v p="$pct" 'BEGIN{exit !(p >= 100)}'; then
    sev=CRIT
  elif awk -v p="$pct" -v w="$MV_TRANSFER_WARN_PCT" 'BEGIN{exit !(p >= w)}'; then
    sev=WARN
  fi
  if [ "$prev" = CRIT ] && [ "$sev" != CRIT ] &&
     ! awk -v p="$pct" -v h="$MV_TRANSFER_HYST_PCT" 'BEGIN{exit !(p <= 100 - h)}'; then
    sev=CRIT
  fi
  if [ "$prev" != OK ] && [ "$sev" = OK ] &&
     ! awk -v p="$pct" -v w="$MV_TRANSFER_WARN_PCT" -v h="$MV_TRANSFER_HYST_PCT" 'BEGIN{exit !(p <= w - h)}'; then
    sev=WARN
  fi

  case "$sev" in
    CRIT)
      report transfer CRIT "$mtd_gb GB of transfer used this month against a $MV_TRANSFER_ALLOWANCE_GB GB allowance counted in BOTH directions, and $drove (${pct}% of the allowance). Overage is billed at \$0.09/GB and is NOT throttled, so this is an open-ended invoice rather than an outage. Levers are in deploy/SIZING.md \"Network transfer\": peer traffic is uncompressed migration envelopes — about 88 GB/month for each crossing per second, both directions — and it does NOT fall with time scale, because population grows into the freed CPU; removing a peer removes its crossings, slowing one removes nothing. A low-latency HLS viewer costs about 1,150 GB/month (about 790 GB/month non-low-latency) and the RTMP ingest about 780 GB/month at 2.5 Mbit/s. Read from /proc/net/dev on $iface, which matches the provider's NetworkIn+NetworkOut to within 1%." ;;
    WARN)
      report transfer WARN "$mtd_gb GB of transfer used this month against a $MV_TRANSFER_ALLOWANCE_GB GB allowance counted in BOTH directions, and $drove (${pct}% of the allowance). Overage is billed at \$0.09/GB and is not throttled, so it is an invoice rather than an outage. The levers are in deploy/SIZING.md \"Network transfer\": peer traffic (about 88 GB/month for each crossing per second, both directions, and it does not fall with time scale), a low-latency HLS viewer (about 1,150 GB/month), and the RTMP ingest (about 780 GB/month). Read from /proc/net/dev on $iface." ;;
    *)
      if [ -n "$proj_b_gb" ]; then
        report transfer OK "$mtd_gb GB used this month, projecting $proj_b_gb GB (${pct_b}% of $MV_TRANSFER_ALLOWANCE_GB GB) at the last 24 h's rate"
      else
        report transfer OK "$mtd_gb GB used this month, ${pct_a}% of the burn-down line to $MV_TRANSFER_ALLOWANCE_GB GB — no closed hour yet, so there is no trailing projection"
      fi ;;
  esac
}

# check_transfer_rate — the leading indicator, reported on EVERY run from state
# that check_transfer wrote. A month-to-date line moves slowly by construction;
# a driver that appears shows up in one hour. The pre-fix day ran 8-10 GB/h with
# three consecutive hours over 9, sixteen hours before a person noticed.
check_transfer_rate() {
  month_window
  local hours hot n last last3 sustain
  hours="$(sget net.hours "")"
  hot="$(sget net.hot 0)"; case "$hot" in ''|*[!0-9]*) hot=0 ;; esac
  n="$(awk -v h="$hours" 'BEGIN{print split(h, a, " ")}')"
  if [ "${n:-0}" -eq 0 ] 2>/dev/null; then
    report transfer-rate OK "no closed hour yet, $hot consecutive above $MV_TRANSFER_HOURLY_GB GB"
    return
  fi
  last="$(awk -v h="$hours" 'BEGIN{n = split(h, a, " "); printf "%.1f", a[n] / 1073741824}')"
  if [ "$hot" -ge "$MV_TRANSFER_HOURLY_RUNS" ] 2>/dev/null; then
    last3="$(awk -v h="$hours" 'BEGIN{
        n = split(h, a, " "); s = (n > 3) ? n - 2 : 1; out = ""
        for (i = s; i <= n; i++) out = out (out == "" ? "" : ", ") sprintf("%.1f", a[i] / 1073741824)
        print out
      }')"
    sustain="$(awk -v a="$MV_TRANSFER_ALLOWANCE_GB" -v s="$MONTH_SECS" 'BEGIN{printf "%.1f", a / (s / 3600)}')"
    report transfer-rate WARN "in+out has exceeded $MV_TRANSFER_HOURLY_GB GB/hour for $hot consecutive hours (last three: $last3 GB). That is more than twice the $sustain GB/hour the allowance sustains. Something started: check open /watch tabs, the broadcast publisher, and the map's crossing rate before the month-to-date line moves."
  else
    report transfer-rate OK "last closed hour $last GB, $hot consecutive above $MV_TRANSFER_HOURLY_GB GB"
  fi
}

# check_hosts_pin — one line of /etc/hosts, and the cheapest check in the file.
#
# The archive subscribes to the relay BY NAME. The name resolves publicly to
# this box's own public address, so without the loopback pin that subscription
# leaves the NIC and comes back, and the provider counts it in both directions.
# It is about 54 GB/day. Nothing about the service looks wrong while it happens.
check_hosts_pin() {
  local domain="${MV_DOMAIN:-}"
  if [ -z "$domain" ]; then
    report hosts-pin WARN "MV_DOMAIN is not set, so the loopback pin cannot be checked"
    return
  fi
  if grep -qE "^[[:space:]]*127\.0\.0\.1[[:space:]]+${domain}([[:space:]]|\$)" "$MV_HOSTS_FILE" 2>/dev/null; then
    report hosts-pin OK "$MV_HOSTS_FILE pins $domain to loopback"
  else
    report hosts-pin CRIT "$MV_HOSTS_FILE NO LONGER pins $domain to 127.0.0.1. The archive dials the relay BY NAME, so without this line its subscription — about 54 GB/day — leaves the box and comes back over the billed interface, and billed transfer roughly doubles. Restore it: echo '127.0.0.1 $domain' | sudo tee -a $MV_HOSTS_FILE (provision.sh --only envfiles also re-adds it). No restart is needed; Go re-resolves."
  fi
}

# ------------------------------------------------------------------ the bill
#
# THE OTHER HALF OF check_transfer. That check reads this box's own NIC counter,
# which needs no credential and is therefore the only thing that can watch the
# allowance from here. It is a PROXY. It tracks the provider's own metric to
# about 1% and it has never been audited by anything on this host, because
# auditing it needs a cloud credential and a public-facing box is the wrong
# place for one.
#
# So the audit happens somewhere else and posts its answer here.
# deploy/ce-reconcile.sh runs once a day on an OPERATOR machine, makes exactly
# one Cost Explorer call, compares the invoice against the same instance metric
# the provider bills from, and writes billing.json into this state directory
# over ssh. Nothing on this host initiates it and nothing on this host holds a
# key.
#
# WHICH MAKES ABSENCE THE FIRST THING TO REPORT. Every other check here fails by
# reading something bad; this one fails by reading nothing, because the machine
# that writes it is somebody's laptop and laptops close. A missing or stale
# reconciliation is not an outage — the NIC counter is still watching the
# allowance and check_transfer will still alert — so it is a WARN that names
# what stopped and where it runs.
#
# WHAT IS A CRITICAL HERE IS AN OVERAGE THAT HAS ALREADY BEEN BILLED. Not a
# projection: check_transfer already projects, and a projection from one settled
# day is noise. A non-zero USE1-DataXfer-Out-Overage-Bytes quantity is the
# provider stating that this month has passed the allowance and the meter is
# running, which no reading on this box can see and which no status page will
# ever show.
check_billing() {
  local f="$STATE/billing.json"
  local tail_where="It runs from an operator machine (deploy/ce-reconcile.sh, one Cost Explorer call a day at \$0.01). The NIC counter is still watching the allowance, so this is an unaudited proxy rather than a blind one."
  if [ ! -s "$f" ]; then
    report billing WARN "the daily Cost Explorer reconciliation has never reported: there is no $f. $tail_where"
    return
  fi

  local asof month through cein ceout ovgib ovusd rin rout proj allow est days
  asof="$(jq -r '.asOf // empty' "$f" 2>/dev/null)"
  month="$(jq -r '.month // "unknown"' "$f" 2>/dev/null)"
  through="$(jq -r '.ceThroughDate // "none"' "$f" 2>/dev/null)"
  cein="$(jq -r '.ceInGiB // 0' "$f" 2>/dev/null)"
  ceout="$(jq -r '.ceOutGiB // 0' "$f" 2>/dev/null)"
  ovgib="$(jq -r '.ceOverageGiB // 0' "$f" 2>/dev/null)"
  ovusd="$(jq -r '.ceOverageUsd // 0' "$f" 2>/dev/null)"
  rin="$(jq -r '.ratioIn // "unknown"' "$f" 2>/dev/null)"
  rout="$(jq -r '.ratioOut // "unknown"' "$f" 2>/dev/null)"
  proj="$(jq -r '.projectedMonthGiB // 0' "$f" 2>/dev/null)"
  allow="$(jq -r '.allowanceGiB // 0' "$f" 2>/dev/null)"
  est="$(jq -r '.ceEstimated // false' "$f" 2>/dev/null)"
  days="$(jq -r '.ceSettledDays // 0' "$f" 2>/dev/null)"

  if [ -z "$asof" ]; then
    report billing WARN "$f exists but carries no readable asOf timestamp, so its age cannot be judged and it must not be trusted. Re-run deploy/ce-reconcile.sh from the operator machine. $tail_where"
    return
  fi
  local at age_h
  at="$(date -u -d "$asof" +%s 2>/dev/null)"
  case "${at:-x}" in
    ''|*[!0-9]*)
      report billing WARN "$f carries an asOf of '$asof', which is not a time this can read. $tail_where"
      return ;;
  esac
  age_h=$(( ( NOW - at ) / 3600 ))
  [ "$age_h" -ge 0 ] || age_h=0

  # THE PROVIDER SAYS SO. An overage quantity or an overage cost is the one
  # reading here that is not an inference, and it outranks staleness: a bill
  # that was billed stays billed while the reconciliation is late.
  # The allowance is a whole number of GB in every bundle the provider sells, and
  # a CRIT that quotes the rule as "- 3072.0" reads like a rounding artefact at
  # 03:00. Print it as it is written on the invoice.
  local allow_txt
  allow_txt="$(awk -v a="$allow" 'BEGIN{ if (a == int(a)) printf "%d", a; else printf "%.1f", a }')"

  local billed=0
  awk -v g="$ovgib" -v u="$ovusd" 'BEGIN{exit !(g > 0 || u > 0)}' && billed=1
  if [ "$billed" = 1 ]; then
    local stale_note=""
    [ "$age_h" -lt "$MV_BILLING_STALE_HOURS" ] || stale_note=" This reading is ALSO $age_h h old, so the real figure is higher."
    report billing CRIT "the provider has BILLED $ovgib GB of transfer overage this month, \$$ovusd so far, and the meter is still running. The rule is billed = min(out, max(0, (in + out) - $allow_txt)) at \$0.09/GB: both directions spend the allowance and only outbound beyond it is charged, so an inbound driver is free by the letter of the rule and costs the same at the margin. Cost Explorer through $through: in $cein GB, out $ceout GB. Nothing on this host can see this — the NIC counter cannot tell an allowed byte from a billed one — and no status page will ever show it. deploy/SIZING.md \"Network transfer\" has the levers.$stale_note"
    return
  fi

  if [ "$age_h" -ge "$MV_BILLING_STALE_HOURS" ] 2>/dev/null; then
    report billing WARN "the daily Cost Explorer reconciliation has not reported for $age_h h (limit $MV_BILLING_STALE_HOURS h; last reading $asof). $tail_where"
    return
  fi

  # UNKNOWN IS A VALUE. A reconciliation that ran and found no transfer usage at
  # all is normal for the first hours of a month — the provider's own data lags
  # about fourteen hours — and is a fault after that. The staleness window is
  # reused as the boundary rather than inventing a second knob.
  month_window
  if [ "$through" = none ]; then
    if [ $(( NOW - MONTH_START )) -ge $(( MV_BILLING_STALE_HOURS * 3600 )) ]; then
      report billing WARN "the reconciliation is fresh ($age_h h old) but Cost Explorer has returned NO Lightsail transfer usage for $month, and the month is now more than $MV_BILLING_STALE_HOURS h old. The invoice side of this comparison is unknown, which is not zero. Read deploy/ce-reconcile.sh --print from the operator machine."
    else
      report billing OK "reconciled $age_h h ago; Cost Explorer has no transfer usage for $month yet, which is its ~14 h lag and not a fault this early in a month"
    fi
    return
  fi

  # THE TWO INSTRUMENTS, ON THE SAME DAYS. The ratio is metric over invoice over
  # the days Cost Explorer has settled, never month-to-date against
  # month-to-date: the lag alone would put that comparison 30% out and an
  # operator would spend a day on it. A ratio outside the band means one of the
  # two is measuring something else — the wrong interface, the wrong instance,
  # or a decimal GB somewhere.
  local drift dir
  drift=""
  dir=""
  if [ "$rin" != unknown ] && awk -v r="$rin" -v t="$MV_BILLING_RATIO_TOL" 'BEGIN{d=r-1; if (d<0) d=-d; exit !(d > t)}'; then
    drift="$(awk -v r="$rin" 'BEGIN{d=(r-1)*100; if (d<0) d=-d; printf "%.1f", d}')"
    dir=inbound
  elif [ "$rout" != unknown ] && awk -v r="$rout" -v t="$MV_BILLING_RATIO_TOL" 'BEGIN{d=r-1; if (d<0) d=-d; exit !(d > t)}'; then
    drift="$(awk -v r="$rout" 'BEGIN{d=(r-1)*100; if (d<0) d=-d; printf "%.1f", d}')"
    dir=outbound
  fi
  if [ -n "$drift" ]; then
    report billing WARN "the NIC counter and the invoice disagree by ${drift}% on $dir over the $days settled day(s) through $through (metric/CE: in ${rin}x, out ${rout}x; the band is ${MV_BILLING_RATIO_TOL} either side of 1). One of them is wrong, and until it is known which, the transfer check above is watching a number nobody has confirmed. Read deploy/ce-reconcile.sh --print. The usual causes are the wrong billed interface (MV_TRANSFER_IFACE), the wrong instance name, or a figure that has become decimal GB somewhere."
    return
  fi

  local pct
  pct="$(awk -v p="$proj" -v a="$allow" 'BEGIN{ if (a <= 0) { print "0"; exit } printf "%.0f", 100 * p / a }')"
  report billing OK "CE through $through: in $cein GB, out $ceout GB, projected $proj GB (${pct}%), overage \$$ovusd; metric/CE in ${rin}x out ${rout}x over $days settled day(s), reconciled $age_h h ago${est:+ (estimated: $est)}"
}

# count_new_errors <logfile> <state-key> — error lines since the last pass,
# surviving the numbered rotation the logger performs (<path>, <path>.1, ...).
count_new_errors() {
  local f="$1" key="$2" off size n
  [ -f "$f" ] || { echo 0; return; }
  size="$(stat -c %s "$f" 2>/dev/null || echo 0)"
  off="$(sget "off.$key" 0)"
  case "$off" in ''|*[!0-9]*) off=0 ;; esac
  # A file that shrank was rotated: the tail we had not read went to <path>.1, so
  # count that generation from where we were and start the live file at zero.
  if [ "$size" -lt "$off" ]; then
    n=$(( $(tail -c "+$((off + 1))" "$f.1" 2>/dev/null | grep -c 'level=ERROR' || true) ))
    n=$(( n + $(grep -c 'level=ERROR' "$f" 2>/dev/null || true) ))
  else
    n=$(( $(tail -c "+$((off + 1))" "$f" 2>/dev/null | grep -c 'level=ERROR' || true) ))
  fi
  sset "off.$key" "$size"
  echo "$n"
}

check_errors() {
  local r a total
  r="$(count_new_errors "$MV_LOGDIR/relay.log" relay)"
  a="$(count_new_errors "$MV_LOGDIR/archive.log" archive)"
  total=$(( r + a ))
  if [ "$total" -gt "$MV_ERROR_ALERT" ]; then
    report errors WARN "$total new ERROR lines since the last check (relay $r, archive $a). Read them: journalctl is not where they are — $MV_LOGDIR/{relay,archive}.log"
  else
    report errors OK "$total new ERROR line(s)"
  fi
}

check_cert() {
  # The certificate THE LISTENER SERVES. This is the check that catches both a
  # renewal that stopped and a deploy hook that stopped, because only one of
  # those is visible in /etc/letsencrypt.
  local served end days fp_served fp_file
  served="$(echo | timeout 15 openssl s_client -connect "${MV_DOMAIN}:${MV_RELAY_PORT}" \
            -servername "$MV_DOMAIN" 2>/dev/null | openssl x509 2>/dev/null)"
  if [ -z "$served" ]; then
    report cert WARN "could not read the certificate nginx is serving"
    return
  fi
  end="$(printf '%s' "$served" | openssl x509 -noout -enddate 2>/dev/null | cut -d= -f2)"
  days=$(( ( $(date -u -d "$end" +%s 2>/dev/null || echo "$NOW") - NOW ) / 86400 ))

  fp_served="$(printf '%s' "$served" | openssl x509 -noout -fingerprint -sha256 2>/dev/null | cut -d= -f2)"
  fp_file="$(openssl x509 -in "$MV_TLSDIR/fullchain.pem" -noout -fingerprint -sha256 2>/dev/null | cut -d= -f2)"

  if [ -n "$fp_file" ] && [ "$fp_served" != "$fp_file" ]; then
    report cert WARN "the certificate on disk is not the one nginx serves. Reload nginx. ($days days left on the served one.)"
  elif [ "$days" -lt "$MV_CERT_MIN_DAYS" ]; then
    report cert CRIT "$days day(s) of certificate left. certbot renews at 30, so renewal has failed: 'certbot renew --dry-run' and check port 80."
  else
    report cert OK "$days days left, disk copy matches the served one"
  fi
}

check_replay_headroom() {
  [ -s "$STATUS_JSON" ] || return 0
  local records peak_mb resident_mb worst_mb worst_what mem_kb ram_mb ratio
  records="$(jq -r '.ledgerRecords // 0' "$STATUS_JSON" 2>/dev/null)"
  case "$records" in ''|*[!0-9]*) return ;; esac
  case "${MV_REPLAY_PEAK_B:-x}${MV_REPLAY_RESIDENT_B:-x}" in
    *[!0-9]*)
      report replay WARN "MV_REPLAY_PEAK_B and MV_REPLAY_RESIDENT_B must be whole bytes per ledger record; they read '$MV_REPLAY_PEAK_B' and '$MV_REPLAY_RESIDENT_B', so nothing is watching the archive's memory."
      return ;;
  esac
  # TWO models, because the box has to satisfy both and they trade places.
  #
  #   MV_REPLAY_PEAK_B      peak while replaying with the streaming archive. The
  #                         older materializing design measured approximately
  #                         1330 B/record.
  #   MV_REPLAY_RESIDENT_B  held resident after replay in the reference workload.
  #
  # In the shipped defaults the RESIDENT term is the larger one, so this alerts
  # on the larger of the two rather than on the replay alone: an archive that
  # restarts fine and then cannot hold what it replayed is the same outage.
  # SIZING.md, "Archive memory", owns both values; they are parameters here so
  # that a measurement on a real ledger can correct them in place.
  #
  # A REAL-LEDGER MEASUREMENT SINCE SAID THEY ARE ONE NUMBER: peak and settled
  # were 0.06 percent apart in every run with no binding GOMEMLIMIT, so
  # deploy.env.example now sets both to the same value and this max() survives
  # only against a future model change. The defaults below stay at the older,
  # more conservative reference figures on purpose — a host with no tuned
  # deploy.env should err toward calling itself full, never toward free.
  peak_mb=$(( records * MV_REPLAY_PEAK_B / 1048576 ))
  resident_mb=$(( records * MV_REPLAY_RESIDENT_B / 1048576 ))
  worst_mb=$peak_mb; worst_what="replay peak"
  if [ "$resident_mb" -gt "$peak_mb" ]; then
    worst_mb=$resident_mb; worst_what="resident set"
  fi

  # PHYSICAL RAM IS THE WHOLE DENOMINATOR, AND SWAP IS DELIBERATELY ABSENT.
  #
  # This check once divided by MemTotal + SwapTotal, which made it possible to
  # clear a critical archive-memory verdict by adding a swap file: the number
  # this gate reports would have halved while the retained state, the record
  # count and the replay were all exactly as they were. A tripwire that a
  # one-line change can turn green without touching what it watches is worse
  # than no tripwire, because somebody will honestly believe it.
  #
  # Swap on this host is a crash barrier for a peak, not capacity for a heap —
  # SIZING.md: "Do not use swap as the normal capacity for a live replay heap."
  # check_swap below reports it as its own reading, which is where a swap file
  # is allowed to be good news.
  mem_kb="$(awk '/^MemTotal:/ {print $2}' "$MV_PROC_MEMINFO" 2>/dev/null)"
  case "${mem_kb:-x}" in
    *[!0-9]*)
      report replay WARN "no MemTotal in $MV_PROC_MEMINFO, so the archive's memory headroom is not being watched at all."
      return ;;
  esac
  ram_mb=$(( mem_kb / 1024 ))
  [ "$ram_mb" -gt 0 ] || return 0
  ratio="$(awk -v p="$worst_mb" -v a="$ram_mb" 'BEGIN{printf "%.2f", p/a}')"

  if awk -v r="$ratio" -v h="$MV_REPLAY_HEADROOM" 'BEGIN{exit !(r >= h)}'; then
    report replay CRIT "the archive can no longer be sure of fitting on this box: $records ledger records project a ~${worst_mb} MB ${worst_what} (replay peak ~${peak_mb} MB at ${MV_REPLAY_PEAK_B} B/record, resident ~${resident_mb} MB at ${MV_REPLAY_RESIDENT_B} B/record) against ${ram_mb} MB of PHYSICAL RAM (ratio $ratio). Swap is not in that denominator on purpose; the separate 'swap' check reports it. THE MAP IS FINE — the relay is unaffected — but the archive must not be restarted until this is fixed. This ratio is a MODEL OF RECORD COUNT, so only three things move it: fewer records, more RAM, or per-record constants that a measurement has corrected (MV_REPLAY_RESIDENT_B, MV_REPLAY_PEAK_B). GOMEMLIMIT is a separate and useful lever — it lowers real resident set by removing the collector's headroom, and being a soft limit it cannot fail a replay — but it does not reduce retained state and it does not move this number. See SIZING.md, 'Archive memory'."
  elif awk -v r="$ratio" -v h="$MV_REPLAY_HEADROOM" 'BEGIN{exit !(r >= h*0.75)}'; then
    report replay WARN "the archive's ${worst_what} projects to ~${worst_mb} MB against ${ram_mb} MB of physical RAM (ratio $ratio; swap is excluded on purpose and has its own check). Size up, reduce the approved retained state, or correct the per-record model with a measurement before it crosses $MV_REPLAY_HEADROOM."
  else
    report replay OK "~${worst_mb} MB projected ${worst_what}, ${ram_mb} MB of physical RAM (ratio $ratio)"
  fi
}

# check_swap — swap, reported honestly and never as headroom.
#
# It is here because check_replay_headroom above deliberately refuses to count
# it, and a reading that is excluded from one place has to appear in another or
# it stops being watched at all. Two different facts, in one line:
#
#   IS THERE ANY?  A swap file is a supported, recorded choice (MV_SWAP_GB in
#                  deploy.env.example, created by provision.sh). Saying so on
#                  every pass is what stops a later reader assuming this box has
#                  none, and what stops the replay ratio being read as though
#                  swap were behind it.
#   IS IT IN USE?  SIZING.md: "Sustained swap activity means that the instance
#                  needs more memory or less retained state." So sustained use
#                  is a SIZING signal and is reported as one — not as an outage,
#                  because the service is still up while it happens, which is
#                  exactly why nobody notices.
#
# Sustained means consecutive passes, not one reading: a single replay peak that
# spills and is reclaimed is the barrier doing its job.
check_swap() {
  local total_kb free_kb used_kb total_mb used_mb pct runs
  total_kb="$(awk '/^SwapTotal:/ {print $2}' "$MV_PROC_MEMINFO" 2>/dev/null)"
  free_kb="$(awk '/^SwapFree:/ {print $2}' "$MV_PROC_MEMINFO" 2>/dev/null)"
  case "${total_kb:-x}${free_kb:-x}" in
    *[!0-9]*)
      report swap WARN "no SwapTotal/SwapFree in $MV_PROC_MEMINFO, so swap use is not being watched."
      return ;;
  esac
  if [ "$total_kb" -eq 0 ]; then
    sset swap.runs 0
    report swap OK "no swap on this host. The replay-headroom ratio is measured against physical RAM either way."
    return
  fi
  used_kb=$(( total_kb - free_kb ))
  [ "$used_kb" -ge 0 ] || used_kb=0
  total_mb=$(( total_kb / 1024 ))
  used_mb=$(( used_kb / 1024 ))
  pct=$(( used_kb * 100 / total_kb ))

  runs="$(sget swap.runs 0)"
  case "$runs" in ''|*[!0-9]*) runs=0 ;; esac
  if [ "$pct" -ge "$MV_SWAP_USED_WARN_PCT" ] 2>/dev/null; then
    runs=$(( runs + 1 ))
  else
    runs=0
  fi
  sset swap.runs "$runs"

  if [ "$runs" -ge "$MV_SWAP_USED_RUNS" ] 2>/dev/null; then
    report swap WARN "${used_mb} MB of ${total_mb} MB of swap has been in use (${pct}%) for $runs consecutive checks (~$(( runs * 5 )) min). SIZING.md reads sustained swap activity as a sizing signal: the instance needs more memory or less retained state. Swap is a crash barrier here, so it is NOT counted as replay headroom — the replay check divides by physical RAM alone, and clearing that check by adding swap would have changed nothing real."
  else
    report swap OK "swap present, ${used_mb} MB of ${total_mb} MB used (${pct}%), $runs consecutive pass(es) at or above ${MV_SWAP_USED_WARN_PCT}%. Not counted as replay headroom."
  fi
}

check_gaps() {
  [ -s "$STATUS_JSON" ] || return 0
  local gaps then t_then delta
  gaps="$(jq -r '.genomeGaps // 0' "$STATUS_JSON" 2>/dev/null)"
  case "$gaps" in ''|*[!0-9]*) return ;; esac
  then="$(sget gaps.24 "")"
  t_then="$(sget gaps.24.at 0)"
  if [ -z "$then" ] || [ "$t_then" -le 0 ] 2>/dev/null; then
    sset gaps.24 "$gaps"; sset gaps.24.at "$NOW"
    report gaps OK "$gaps unresolved genome hashes (first sample)"
    return
  fi
  if [ $(( NOW - t_then )) -lt 82800 ]; then
    report gaps OK "$gaps unresolved genome hashes"
    return
  fi
  delta=$(( gaps - then ))
  sset gaps.24 "$gaps"; sset gaps.24.at "$NOW"
  if [ "$delta" -gt "$MV_GAPS_DELTA_ALERT" ]; then
    report gaps WARN "genomeGaps grew by $delta in 24 h to $gaps. The archive is in arrears on genome capture, and the part a departing peer takes with them is permanent (Risk 7)."
  else
    report gaps OK "$gaps unresolved genome hashes, ${delta:+$delta }in 24 h"
  fi
}

check_backup() {
  local newest age
  newest="$(find "$MV_STATE/backup/identity" -maxdepth 1 -mindepth 1 -type d -printf '%T@\n' 2>/dev/null | sort -n | tail -1)"
  if [ -z "$newest" ]; then
    report backup WARN "no identity snapshot has ever been taken. ring.json and peers.json have no second copy."
    return
  fi
  age=$(( NOW - ${newest%.*} ))
  if [ "$age" -gt 10800 ]; then
    report backup WARN "the newest backup of ring.json/peers.json is $(( age / 3600 ))h old; the timer runs hourly. 'systemctl status multiverse-backup.service'"
  else
    report backup OK "newest snapshot $(( age / 60 )) min old"
  fi
}

check_reboot() {
  if [ -f /var/run/reboot-required ] || [ -f /run/reboot-required ]; then
    report reboot WARN "a kernel update is waiting for a reboot. Unattended upgrades do NOT reboot this box on purpose — a reboot replays the archive and costs a ledger gap. Schedule and announce one: RESTART-POLICY.md."
  else
    report reboot OK "no reboot pending"
  fi
}

# ---------------------------------------------------------------- run

# --only runs one group and nothing else. It exists so the transfer, billing and
# replay-headroom arithmetic can be driven from deploy/test-monitor.sh without a
# network, without root and without the certificate check dialling the live
# front door; by hand it is also the fastest way to re-read one check.
case "${ONLY:-}" in
  '')
    check_units
    check_stream_origin
    check_relay_healthz
    if check_archive_healthz; then
      check_map
      check_bypass
      check_replay_headroom
      check_gaps
    fi
    # Outside that block on purpose: swap is read from /proc and says something
    # about this box whether or not the archive is answering — and an archive
    # that is not answering is one of the cases where it matters most.
    check_swap
    check_disk
    check_transfer
    check_transfer_rate
    check_hosts_pin
    check_billing
    check_errors
    check_cert
    check_backup
    check_reboot
    ;;
  transfer)
    # check_transfer_rate reads only what check_transfer wrote, so it reports on
    # every run even when the counter read above failed.
    check_transfer
    check_transfer_rate
    ;;
  hosts-pin) check_hosts_pin ;;
  replay) check_replay_headroom ;;
  swap) check_swap ;;
  billing) check_billing ;;
  *) echo "monitor: --only accepts 'transfer', 'hosts-pin', 'replay', 'swap' or 'billing', not '$ONLY'" >&2; exit 2 ;;
esac

# The dead man's half. A daily line that says the watcher itself is alive, so
# that its ABSENCE is a signal rather than a comfort.
if [ -z "$ONLY" ] && [ "$MV_HEARTBEAT_HOUR" -ge 0 ] 2>/dev/null; then
  hour="$(date -u +%H)"; hour="${hour#0}"; : "${hour:=0}"
  today="$(date -u +%F)"
  if [ "$hour" = "$MV_HEARTBEAT_HOUR" ] && [ "$(sget heartbeat.day)" != "$today" ]; then
    sset heartbeat.day "$today"
    if [ "$QUIET" = 0 ]; then
      body="$(printf 'state: %s\nup: %s\n%s' "$WORST" "$(uptime -p 2>/dev/null || echo unknown)" \
        "$(jq -r '"peers live \(.totals.liveSlots // 0), dark \(.totals.darkSlots // 0); ledger \(.ledgerRecords // 0) records; gaps \(.genomeGaps // 0)"' "$STATUS_JSON" 2>/dev/null || echo 'no status')")"
      notify OK heartbeat "$body${SUMMARY}"
    fi
  fi
fi

if [ "$VERBOSE" = 1 ] || [ "$WORST" != OK ]; then
  printf '\nworst: %s%s\n' "$WORST" "$SUMMARY"
fi
[ "$WORST" = OK ]
