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
#   monitor.sh --quiet      no alerts; the verdict is printed, never sent
#   monitor.sh --only NAME  run one group and nothing else: 'transfer',
#                           'hosts-pin', 'replay', 'archive', 'swap' or
#                           'billing'. 'replay' is the two restart checks —
#                           deploy/restart-archive.sh reads its verdict — and
#                           'archive' is the three record-layer checks.
#                           deploy/test-monitor.sh drives the transfer, billing
#                           and replay-headroom arithmetic through this against
#                           fake /proc files, a fake status document, a fake
#                           reconciliation file and a fake clock, on a
#                           workstation, without root and without touching the
#                           network.
#
# THE EXIT STATUS SAYS WHETHER THE MONITOR RAN. It does not say whether the
# service is well.
#
#   0   the pass completed. OK, WARN and CRIT all exit 0.
#   1   only from --test, and only when the alert channel itself failed.
#   2   the pass could not start: no readable environment file, or an argument
#       this does not accept.
#
# A WARN or a CRIT travels in the alert, in the `worst:` line on stdout and in
# the sev.* state under MV_STATE. It used to travel in the exit code as well,
# and that was wrong twice over. multiverse-monitor.service is Type=oneshot, so
# every tick of the five-minute timer left the unit `failed` while the monitor
# was doing exactly its job: `systemctl is-failed` stopped meaning anything,
# three operations records in one night named it, and a deploy pipeline running
# under `set -o pipefail` broke on it. A non-zero status here now means the
# WATCHER could not run — the one fault no check of its own can report.
#
# To read the verdict from a script, parse the `worst:` line, which prints on
# every --verbose run and on every run that is not OK:
#
#     monitor.sh --quiet --verbose | awk '/^worst:/ { print $2 }'
#
# or read the per-check lines --verbose prints, which is what
# deploy/restart-archive.sh does for its replay gate.
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
#   replay cost           WHAT THE NEXT RESTART WILL COST IN SECONDS, which is
#                         a different question from whether it fits. A
#                         record-preserving archive restart holds the relay
#                         down for the length of the replay, so this is the
#                         participant outage an announcement has to state.
#                         Projected from two numbers the archive MEASURES at
#                         its own last start — replayRawSeconds and
#                         replayRawRecords — and from the duplicate window,
#                         which is what bounds the next one. It is not a
#                         memory of the last restart: the record rate this box
#                         samples for itself is what turns a measured cost per
#                         record into a projection of the next scan.
#   cold copy             ledgerSegmentsAwaitingColdCopy, THE NUMBER THAT
#                         SHOULD BE ZERO. A closed segment is never removed
#                         without a receipt confirming an off-host copy, so a
#                         cold archive that stopped working surfaces here and
#                         in disk usage, and never as a record that is gone.
#                         Nothing else watches the object store at all.
#   roll-up state         rollupSavedAtMs. The state sidecar is what makes a
#                         restart cost a window instead of a whole ledger. A
#                         sidecar that stopped saving breaks nothing, moves no
#                         other number, and turns the next restart back into a
#                         full replay — a silent regression of the one promise
#                         the roll-up exists to keep.
#   duplicates            duplicatesRefused. All-time, and zero is the answer
#                         that matters: it is the evidence that the duplicate
#                         window — which is also what a restart costs — may
#                         safely come down. A refused duplicate leaves no other
#                         trace anywhere, so without this counter the guard's
#                         whole working is invisible.
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
# THE RESTART'S COST IN SECONDS. A record-preserving archive restart holds the
# relay down for the length of the replay (RESTART-POLICY.md), so these are
# thresholds on a participant outage and not on a resource. 180 s is an
# announcement; 600 s is a scheduled window.
: "${MV_REPLAY_SECONDS_WARN:=180}"
: "${MV_REPLAY_SECONDS_CRIT:=600}"
# THE DUPLICATE WINDOW IS WHAT A RESTART COSTS, since the roll-up: the replay
# rebuilds its aggregates from the state sidecar and parses only
# min(the raw window on this host, this value) of raw records. It is read from
# deploy.env because that is where provision.sh reads it to write
# MULTIVERSE_ARCHIVE_DEDUP_WINDOW into archive.env — one value, two readers.
# Empty takes the contract's own default, which is also 48h.
: "${MV_ARCHIVE_DEDUP_WINDOW:=}"
# How long a record-rate sample must be open before it is used. An hour of a
# real map is ~130,000 records, which is signal enough; the alternative is
# waiting a day before this check says anything at all.
: "${MV_REPLAY_RATE_MIN_S:=3600}"
# The roll-up state sidecar saves every 30 s. Warn when its last save is older
# than this. It also covers the sidecar that has never saved.
: "${MV_ROLLUP_STALE_S:=900}"
# Closed segments waiting for an off-host copy. One is normal for the hour after
# a rotation; a day of them means the copy has stopped. The factor is the
# multiple of the ledger window at which the backlog stops being late and starts
# being the thing that fills the disk.
: "${MV_COLDCOPY_WAIT_HOURS:=24}"
: "${MV_COLDCOPY_WINDOW_FACTOR:=2}"
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

# go_duration_s <value> <default-seconds> — the subset of Go's duration syntax a
# deployment actually writes: a run of <number><unit> with unit h, m or s.
#
# AN UNPARSEABLE VALUE RETURNS THE DEFAULT AND NEVER ZERO. Zero here would read
# as "no duplicate window", which is a setting no deployment should have and one
# this script must never invent on the operator's behalf: it would make every
# restart projection say a restart is free.
go_duration_s() {
  local v="${1:-}" def="$2" total=0 num unit rest ok=1
  [ -n "$v" ] || { printf '%s' "$def"; return; }
  rest="$v"
  while [ -n "$rest" ]; do
    num="${rest%%[hms]*}"
    case "$num" in ''|*[!0-9]*) ok=0; break ;; esac
    rest="${rest#"$num"}"
    unit="${rest:0:1}"
    rest="${rest:1}"
    case "$unit" in
      h) total=$(( total + num * 3600 )) ;;
      m) total=$(( total + num * 60 )) ;;
      s) total=$(( total + num )) ;;
      *) ok=0; break ;;
    esac
  done
  if [ "$ok" = 1 ] && [ "$total" -gt 0 ]; then printf '%s' "$total"; else printf '%s' "$def"; fi
}

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
    --only) ONLY="${2:?--only needs a check group: transfer, hosts-pin, replay, archive, swap or billing}"; shift ;;
    --test)
      if notify OK self-test "This is a test from $(hostname) at $(date -u +%FT%TZ). If you are reading it, the alert channel works."; then
        echo "sent one $MV_ALERT_KIND alert"
        exit 0
      fi
      echo "the alert channel is NOT working: kind=$MV_ALERT_KIND url set=$([ -n "$MV_ALERT_URL" ] && echo yes || echo no)" >&2
      exit 1 ;;
    # The header block, whatever length it has grown to: everything from the
    # line after the shebang down to the `set` that ends it, minus that line.
    # A hard-coded last line is a comment that silently truncates its own help.
    -h|--help) sed -n '2,/^set -uo pipefail$/p' "$0" | sed '$d'; exit 0 ;;
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

  # THE PROJECTION ABOVE IS A STRAIGHT LINE AND THE RAW LEDGER IS NOT ONE, once
  # a window is in force and the off-host copy is working: the raw lines reach a
  # flat ceiling and everything after that is the genome store and the logs. A
  # straight line through a term that stops growing reads a healthy host as
  # weeks from full. Say which shape this host is in — and say it only when the
  # gate that makes it true is actually closed, because a window with segments
  # waiting for a copy retires nothing and the straight line is then correct.
  if [ -s "$STATUS_JSON" ]; then
    local winms awaiting rawb
    winms="$(jq -r '.ledgerWindowMs // 0' "$STATUS_JSON" 2>/dev/null)"
    awaiting="$(jq -r '.ledgerSegmentsAwaitingColdCopy // 0' "$STATUS_JSON" 2>/dev/null)"
    rawb="$(jq -r '.ledgerRawBytes // 0' "$STATUS_JSON" 2>/dev/null)"
    case "${winms:-x}" in ''|*[!0-9]*) winms=0 ;; esac
    case "${awaiting:-x}" in ''|*[!0-9]*) awaiting=0 ;; esac
    case "${rawb:-x}" in ''|*[!0-9]*) rawb=0 ;; esac
    if [ "$winms" -gt 0 ] && [ "$awaiting" = 0 ]; then
      tail="$tail. The raw ledger is bounded: a $(( winms / 86400000 ))-day window with every older segment copied off-host, holding $(( rawb / 1073741824 )) GB as stored today, so that term stops growing and this projection reads high"
    elif [ "$winms" -gt 0 ]; then
      tail="$tail. A $(( winms / 86400000 ))-day raw window is set but $awaiting segment(s) have no confirmed off-host copy, so NOTHING is being retired and the raw ledger is growing as though there were no window ('cold-copy' check)"
    fi
  fi

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
      report transfer CRIT "$mtd_gb GB of transfer used this month against a $MV_TRANSFER_ALLOWANCE_GB GB allowance counted in BOTH directions, and $drove (${pct}% of the allowance). Overage is billed at \$0.09/GB and is NOT throttled, so this is an open-ended invoice rather than an outage. Levers are in deploy/SIZING.md \"Network transfer\": peer traffic is migration envelopes — about 21 GB/month for each crossing per second on the compressed wire, both directions, and about 88 GB/month uncompressed — and it does NOT fall with time scale, because population grows into the freed CPU; removing a peer removes its crossings, slowing one removes nothing. A continuous HLS viewer costs about 475 GB/month at 1.5 Mbit/s (about 790 GB/month at 2.5, and about 1,150 GB/month for the low-latency variant this origin no longer serves), and the RTMP ingest about 470 GB/month while the publisher runs, which on demand is only while somebody watches. Read from /proc/net/dev on $iface, which matches the provider's NetworkIn+NetworkOut to within 1%." ;;
    WARN)
      report transfer WARN "$mtd_gb GB of transfer used this month against a $MV_TRANSFER_ALLOWANCE_GB GB allowance counted in BOTH directions, and $drove (${pct}% of the allowance). Overage is billed at \$0.09/GB and is not throttled, so it is an invoice rather than an outage. The levers are in deploy/SIZING.md \"Network transfer\": peer traffic (about 21 GB/month for each crossing per second on the compressed wire, both directions, and it does not fall with time scale), a continuous HLS viewer (about 475 GB/month at 1.5 Mbit/s), and the RTMP ingest (about 470 GB/month while the publisher runs). Read from /proc/net/dev on $iface." ;;
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

  # SAY WHEN THE MODEL IS KNOWN TO OVER-ESTIMATE, rather than quietly letting it.
  # Since the record roll-up the archive's retained state is the bounded
  # aggregates plus the duplicate window's keys — not one term for every ledger
  # record there has ever been — so a record-count projection grows against a
  # process that no longer does. The arithmetic is deliberately NOT changed
  # here: the shipped constants err toward calling a host full, which is the
  # safe error, and correcting them needs a measurement on this host rather than
  # a guess from this script. What changes is that the verdict says so.
  local rollup_note="" covered
  covered="$(jq -r '.rollupCoveredRecords // 0' "$STATUS_JSON" 2>/dev/null)"
  case "$covered" in
    ''|*[!0-9]*) covered=0 ;;
  esac
  if [ "$covered" -gt 0 ]; then
    rollup_note=" This ratio is a RECORD-COUNT model and this archive has a roll-up: its retained state is the bounded aggregates plus the duplicate window's keys, so the projection above grows while the process does not, and it errs toward calling this host full. Correct it only with a measurement on this host (SIZING.md, 'Archive memory'). What the next restart COSTS in seconds is the separate 'replay-cost' check."
  fi

  if awk -v r="$ratio" -v h="$MV_REPLAY_HEADROOM" 'BEGIN{exit !(r >= h)}'; then
    report replay CRIT "the archive can no longer be sure of fitting on this box: $records ledger records project a ~${worst_mb} MB ${worst_what} (replay peak ~${peak_mb} MB at ${MV_REPLAY_PEAK_B} B/record, resident ~${resident_mb} MB at ${MV_REPLAY_RESIDENT_B} B/record) against ${ram_mb} MB of PHYSICAL RAM (ratio $ratio). Swap is not in that denominator on purpose; the separate 'swap' check reports it. THE MAP IS FINE — the relay is unaffected — but the archive must not be restarted until this is fixed. This ratio is a MODEL OF RECORD COUNT, so only three things move it: fewer records, more RAM, or per-record constants that a measurement has corrected (MV_REPLAY_RESIDENT_B, MV_REPLAY_PEAK_B). GOMEMLIMIT is a separate and useful lever — it lowers real resident set by removing the collector's headroom, and being a soft limit it cannot fail a replay — but it does not reduce retained state and it does not move this number. See SIZING.md, 'Archive memory'.$rollup_note"
  elif awk -v r="$ratio" -v h="$MV_REPLAY_HEADROOM" 'BEGIN{exit !(r >= h*0.75)}'; then
    report replay WARN "the archive's ${worst_what} projects to ~${worst_mb} MB against ${ram_mb} MB of physical RAM (ratio $ratio; swap is excluded on purpose and has its own check). Size up, reduce the approved retained state, or correct the per-record model with a measurement before it crosses $MV_REPLAY_HEADROOM.$rollup_note"
  else
    report replay OK "~${worst_mb} MB projected ${worst_what}, ${ram_mb} MB of physical RAM (ratio $ratio)$rollup_note"
  fi
}

# check_restart_cost — what the NEXT archive restart will cost, in seconds.
#
# THIS IS THE PROMISE THE ROLL-UP MADE AND IT IS THE ONE NOBODY WAS WATCHING.
# The record-preserving sequence holds the relay down for the whole replay, so
# the replay time IS the participant outage; before the roll-up it grew with
# every record there had ever been, and after it, it is bounded by one number an
# operator sets. A bound nobody measures is a claim nobody can check.
#
# The model, and every term in it is either measured or sampled on this box:
#
#   cost per raw record = replayRawSeconds / replayRawRecords, both MEASURED by
#                         the archive at its own last start and published.
#   records in the window = the record rate this check samples for itself,
#                         times the duplicate window, capped at the whole ledger
#                         because a scan cannot read raw that is not there.
#   projected seconds   = the two multiplied.
#
# WHY NOT JUST REPORT replayRawSeconds. Because it is a memory of the last
# start, and the last start is the one that is never representative: the first
# start after this deployment reads the WHOLE ledger once, and every start after
# it reads a window. A check that reported the measurement alone would raise a
# critical alarm on the healthiest archive this project has ever had, on the day
# it became healthy.
#
# THE SECOND TERM THE MODEL DOES NOT CARRY. Getting TO the window's start is
# free on a plain segment or the live file and is a decompress-and-discard on a
# compressed one, because a gzip stream cannot seek: 5.9 s to skip 1.8 GB in the
# measurement this was built from. In ordinary running the window starts inside
# a recent day and the skip is small; the one-off legacy segment the migration
# produces is the worst case. The verdict says so when segments are present
# rather than pretending the projection is the whole cost.
check_restart_cost() {
  [ -s "$STATUS_JSON" ] || return 0
  local rr rs ledger segs
  rr="$(jq -r '.replayRawRecords // empty' "$STATUS_JSON" 2>/dev/null)"
  rs="$(jq -r '.replayRawSeconds // empty' "$STATUS_JSON" 2>/dev/null)"
  ledger="$(jq -r '.ledgerRecords // 0' "$STATUS_JSON" 2>/dev/null)"
  segs="$(jq -r '.ledgerSegments // 0' "$STATUS_JSON" 2>/dev/null)"
  case "${ledger:-x}" in ''|*[!0-9]*) ledger=0 ;; esac
  case "${segs:-x}" in ''|*[!0-9]*) segs=0 ;; esac

  # THE FALLBACK, and it is a real state rather than an error: an archive built
  # before the roll-up publishes neither field, and the record-count model in
  # the 'replay' check is then the only estimate there is. Say which model is in
  # force, because the two answer different questions.
  case "${rr:-x}" in
    ''|*[!0-9]*)
      report replay-cost OK "this archive does not publish replayRawRecords, so the next restart's cost is not measured on this host. The 'replay' check above is the fallback and it is a model of MEMORY, not of time: for the outage, use RESTART-POLICY.md's ledger_records / measured_records_per_second. An archive built before the record roll-up is the expected reason."
      return ;;
  esac
  if [ "$rr" -le 0 ] 2>/dev/null || ! awk -v v="${rs:-x}" 'BEGIN{exit !(v+0 > 0)}' 2>/dev/null; then
    report replay-cost OK "the archive published replayRawRecords=$rr and replayRawSeconds=${rs:-unset}, which is a start that parsed no raw at all — a first start on an empty ledger. There is nothing to project from yet."
    return
  fi

  local win_s
  win_s="$(go_duration_s "$MV_ARCHIVE_DEDUP_WINDOW" 172800)"

  # THE RECORD RATE, sampled by this box from its own status document. It is
  # kept as records per 1000 s so the arithmetic below stays in integers.
  local rate_k rec_then t_then
  rate_k="$(sget replay.rate "")"
  rec_then="$(sget replay.rec "")"
  t_then="$(sget replay.rec.at 0)"
  case "$t_then" in ''|*[!0-9]*) t_then=0 ;; esac
  if [ -z "$rec_then" ] || [ "$t_then" -le 0 ]; then
    sset replay.rec "$ledger"; sset replay.rec.at "$NOW"
    t_then="$NOW"
  elif [ $(( NOW - t_then )) -ge "$MV_REPLAY_RATE_MIN_S" ]; then
    local grew=$(( ledger - rec_then )) span=$(( NOW - t_then ))
    # A ledger counter that went backwards is a restored or replaced archive,
    # not a negative rate. Drop the sample and start a new one.
    if [ "$grew" -ge 0 ] && [ "$span" -gt 0 ]; then
      rate_k=$(( grew * 1000 / span ))
      sset replay.rate "$rate_k"
    fi
    sset replay.rec "$ledger"; sset replay.rec.at "$NOW"
  fi

  local measured
  measured="$(awk -v s="$rs" -v n="$rr" 'BEGIN{printf "%.1f s to parse %d raw record(s), %.0f/s", s, n, n/s}')"

  local gz_note=""
  [ "$segs" -gt 0 ] 2>/dev/null && gz_note=" Reaching the window's start is not in this figure: it is free on a plain segment and a decompress-and-discard on a compressed one, and this host holds $segs closed segment(s)."

  case "${rate_k:-x}" in
    ''|*[!0-9]*)
      report replay-cost OK "the last start measured $measured. A projection of the NEXT restart needs this box's own record rate, and the sample opened $(( (NOW - t_then) / 60 )) min ago of the $(( MV_REPLAY_RATE_MIN_S / 60 )) min it needs. The duplicate window is what will bound it: $(( win_s / 3600 ))h."
      return ;;
  esac

  local proj_rec proj_s
  proj_rec=$(( rate_k * win_s / 1000 ))
  [ "$proj_rec" -gt "$ledger" ] && proj_rec="$ledger"
  proj_s="$(awk -v n="$proj_rec" -v s="$rs" -v r="$rr" 'BEGIN{printf "%.0f", n * (s/r)}')"

  local common="the next archive restart projects to ~${proj_s} s of held-down relay: ${proj_rec} raw record(s) inside the ${win_s}s duplicate window at the cost the last start measured ($measured). THE REPLAY TIME IS THE PARTICIPANT OUTAGE — the record-preserving sequence holds the relay down for all of it (RESTART-POLICY.md).${gz_note}"

  if [ "$proj_s" -ge "$MV_REPLAY_SECONDS_CRIT" ] 2>/dev/null; then
    report replay-cost CRIT "$common This is past $MV_REPLAY_SECONDS_CRIT s, so a restart is a scheduled and announced window and not a maintenance action. Lower MV_ARCHIVE_DEDUP_WINDOW if duplicatesRefused says it is safe — that is the one knob that moves this number — and do not restart on the strength of an old estimate."
  elif [ "$proj_s" -ge "$MV_REPLAY_SECONDS_WARN" ] 2>/dev/null; then
    report replay-cost WARN "$common This is past $MV_REPLAY_SECONDS_WARN s, so it has to be announced before it is done."
  else
    report replay-cost OK "~${proj_s} s projected (${proj_rec} raw records in a ${win_s}s window; last start $measured)"
  fi
}

# check_cold_copy — ledgerSegmentsAwaitingColdCopy, the number that should be
# zero, and the only thing on this host that watches the object store at all.
#
# WHY IT IS ITS OWN CHECK. A closed segment is never removed until a receipt
# confirms an off-host copy that matches the bytes on this disk. That gate is in
# the code and not in a runbook, so a cold archive that has stopped working
# cannot cost a record — it costs DISK, weeks later, in a different check, with
# nothing in between saying why. This is the "in between".
#
# THE FIRST-PASS BLIND SPOT, and it is easy to fire on: the segment fields are
# refreshed by the archive's maintenance pass, and the pass at Start compresses
# before it refreshes — 21 s on a real legacy segment. A status document
# captured inside that window reads 0 segments and no window start on a
# perfectly healthy archive. So an archive that has not saved its roll-up state
# yet is reported as "not yet", never as "nothing there".
check_cold_copy() {
  [ -s "$STATUS_JSON" ] || return 0
  local awaiting segs winms fromms saved
  awaiting="$(jq -r '.ledgerSegmentsAwaitingColdCopy // empty' "$STATUS_JSON" 2>/dev/null)"
  case "${awaiting:-x}" in ''|*[!0-9]*) return ;; esac
  segs="$(jq -r '.ledgerSegments // 0' "$STATUS_JSON" 2>/dev/null)"
  winms="$(jq -r '.ledgerWindowMs // 0' "$STATUS_JSON" 2>/dev/null)"
  fromms="$(jq -r '.ledgerRawWindowFromMs // 0' "$STATUS_JSON" 2>/dev/null)"
  saved="$(jq -r '.rollupSavedAtMs // 0' "$STATUS_JSON" 2>/dev/null)"
  case "${segs:-x}"   in ''|*[!0-9]*) segs=0   ;; esac
  case "${winms:-x}"  in ''|*[!0-9]*) winms=0  ;; esac
  case "${fromms:-x}" in ''|*[!0-9]*) fromms=0 ;; esac
  case "${saved:-x}"  in ''|*[!0-9]*) saved=0  ;; esac

  if [ "$saved" = 0 ] && [ "$segs" = 0 ] && [ "$awaiting" = 0 ]; then
    report cold-copy OK "the segment layer has not reported yet. The archive refreshes these fields in its maintenance pass, and the pass at start compresses before it refreshes, so a reading taken in the first minute after a restart is 'not yet' rather than 'nothing there'."
    return
  fi

  if [ "$awaiting" = 0 ]; then
    sset coldcopy.awaiting.since 0
    if [ "$winms" -gt 0 ]; then
      report cold-copy OK "every closed segment past the $(( winms / 86400000 ))-day window has a confirmed off-host copy ($segs segment(s) held)"
    else
      report cold-copy OK "no segment is waiting for an off-host copy ($segs segment(s) held; no ledger window is in force, so none would be removed anyway)"
    fi
    return
  fi

  local since age
  since="$(sget coldcopy.awaiting.since 0)"
  case "$since" in ''|*[!0-9]*|0) since="$NOW"; sset coldcopy.awaiting.since "$NOW" ;; esac
  age=$(( NOW - since ))

  # THE CRITICAL CASE IS NOT "LATE", IT IS "THE WINDOW IS NO LONGER A WINDOW".
  # Every segment counted here is already past the window by construction, so
  # lateness alone is the warning. What makes it critical is the raw on this
  # host reaching back further than the window was ever meant to allow, which is
  # the point at which the disk, and not the record, is what is at risk.
  local held_days="" want_days=""
  if [ "$winms" -gt 0 ] && [ "$fromms" -gt 0 ]; then
    held_days=$(( (NOW * 1000 - fromms) / 86400000 ))
    want_days=$(( winms / 86400000 ))
    if [ $(( NOW * 1000 - fromms )) -gt $(( winms * MV_COLDCOPY_WINDOW_FACTOR )) ]; then
      report cold-copy CRIT "$awaiting closed segment(s) have been waiting for an off-host copy for $(( age / 3600 ))h, and this host now holds ${held_days} days of raw lines against a ${want_days}-day window. NO SEGMENT IS EVER REMOVED WITHOUT A CONFIRMED COPY, so the record is safe and the DISK is what pays: the window is doing nothing and the raw ledger is growing as though there were none. Run 'deploy/coldcopy.sh --check' and then 'deploy/coldcopy.sh --list'; the commonest causes are an absent AWS CLI, an unset MV_COLDCOPY_URI, and a storage class the bucket's policy denies."
      return
    fi
  fi

  if [ "$age" -ge $(( MV_COLDCOPY_WAIT_HOURS * 3600 )) ]; then
    report cold-copy WARN "$awaiting closed segment(s) have had no confirmed off-host copy for $(( age / 3600 ))h. The hourly timer should clear one within the hour, so this is the copy failing rather than lagging. Nothing is removed without a receipt, so this costs disk and not record: 'systemctl status multiverse-coldcopy.timer', then 'deploy/coldcopy.sh --check'."
  else
    report cold-copy OK "$awaiting segment(s) waiting for an off-host copy, first seen $(( age / 60 )) min ago (the timer runs hourly)"
  fi
}

# check_rollup — the state sidecar's last save.
#
# IT IS A SILENT RESTART-COST REGRESSION AND NOTHING ELSE WOULD FIND IT. The
# sidecar is what lets a restart rebuild its aggregates from a file instead of
# from the whole ledger. If it stops saving, every published number stays right,
# every other check stays green, the archive serves perfectly — and the next
# restart replays from wherever the file stopped, which on a long enough gap is
# the full replay the roll-up exists to remove. The cost is only ever paid once,
# at the worst possible moment, by a participant.
check_rollup() {
  [ -s "$STATUS_JSON" ] || return 0
  local saved covered age first
  saved="$(jq -r '.rollupSavedAtMs // empty' "$STATUS_JSON" 2>/dev/null)"
  case "${saved:-x}" in ''|*[!0-9]*) return ;; esac
  covered="$(jq -r '.rollupCoveredRecords // 0' "$STATUS_JSON" 2>/dev/null)"
  case "${covered:-x}" in ''|*[!0-9]*) covered=0 ;; esac

  if [ "$saved" -gt 0 ]; then
    sset rollup.zero.since 0
    age=$(( NOW - saved / 1000 ))
    [ "$age" -lt 0 ] && age=0
    if [ "$age" -gt "$MV_ROLLUP_STALE_S" ]; then
      report rollup WARN "the roll-up state sidecar last saved $(( age / 60 )) min ago and it saves every 30 s. Nothing has failed and no published number is wrong — which is the problem: the next restart replays from where this file stopped, so a sidecar that quietly stopped turns a window-bounded restart back into a full one. Check the archive log for a save error and for free space and inodes on the archive volume."
    else
      report rollup OK "state sidecar saved ${age}s ago, covering $covered record(s)"
    fi
    return
  fi

  # Never saved. On a fresh archive that is correct for the first half-minute.
  first="$(sget rollup.zero.since 0)"
  case "$first" in ''|*[!0-9]*|0) first="$NOW"; sset rollup.zero.since "$NOW" ;; esac
  if [ $(( NOW - first )) -gt "$MV_ROLLUP_STALE_S" ]; then
    report rollup WARN "the roll-up state sidecar has never saved, and this host has been reporting that for $(( (NOW - first) / 60 )) min. Every restart from here is a full replay of the raw ledger. Check the archive log at start: an unreadable sidecar is kept aside as .unreadable and rebuilt, and that is reported rather than silent."
  else
    report rollup OK "the state sidecar has not saved yet ($(( NOW - first ))s); it saves every 30 s"
  fi
}

# check_duplicates — duplicatesRefused, all-time.
#
# ZERO IS THE ANSWER THAT MATTERS, which is why the archive does not omit the
# field when it is zero and why this check reports on every pass. The duplicate
# window is sized against how far behind the oldest sidecar on the map can be,
# and it is ALSO what a restart costs; lowering it is the one change that makes
# a restart cheap, and this counter is the only evidence that it is safe. A
# refused duplicate leaves no other trace anywhere: nothing is appended, nothing
# is logged for each one, and the record is by construction the record without
# it.
check_duplicates() {
  [ -s "$STATUS_JSON" ] || return 0
  local dup then t_then delta
  dup="$(jq -r '.duplicatesRefused // empty' "$STATUS_JSON" 2>/dev/null)"
  case "${dup:-x}" in ''|*[!0-9]*) return ;; esac

  if [ "$dup" = 0 ]; then
    report duplicates OK "0 refused since this archive began. That is the evidence MV_ARCHIVE_DEDUP_WINDOW may come down once the participant release has been out for a cycle, and the window is also what a restart costs."
    return
  fi
  then="$(sget dup.24 "")"
  t_then="$(sget dup.24.at 0)"
  case "$t_then" in ''|*[!0-9]*) t_then=0 ;; esac
  if [ -z "$then" ] || [ "$t_then" -le 0 ]; then
    sset dup.24 "$dup"; sset dup.24.at "$NOW"
    report duplicates OK "$dup duplicate record(s) refused all-time (first sample). Expected while sidecars older than contract-b/4.1 §25 B37 are still on the map: each one is a retry the guard caught, and the record is correct because it was caught."
    return
  fi
  if [ $(( NOW - t_then )) -lt 82800 ]; then
    report duplicates OK "$dup duplicate record(s) refused all-time"
    return
  fi
  delta=$(( dup - then ))
  sset dup.24 "$dup"; sset dup.24.at "$NOW"
  if [ "$delta" -gt 0 ]; then
    report duplicates WARN "$delta duplicate record(s) were refused in the last 24 h, $dup all-time. The guard is doing its job and the record is correct — and DO NOT LOWER MV_ARCHIVE_DEDUP_WINDOW while this is moving: something on the map is still re-sending, and a shorter window would let the second copy into the record instead of refusing it."
  else
    report duplicates OK "$dup refused all-time and none in the last 24 h. A full cycle at zero is what says the window may come down."
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
      check_restart_cost
      check_rollup
      check_cold_copy
      check_duplicates
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
  # BOTH RESTART CHECKS, because they answer the two halves of one question and
  # deploy/restart-archive.sh asks it before every planned restart: does the
  # replay still fit in RAM, and what will it cost the map in seconds. That
  # script reads the 'replay' line for its gate and prints the 'replay-cost'
  # line as RESTART-POLICY.md's pre-restart replay estimate.
  replay) check_replay_headroom; check_restart_cost ;;
  # The record layer: the state sidecar that makes a restart cheap, the off-host
  # copy that lets a segment be removed at all, and the duplicate counter that
  # says whether the window may come down.
  archive) check_rollup; check_cold_copy; check_duplicates ;;
  swap) check_swap ;;
  billing) check_billing ;;
  *) echo "monitor: --only accepts 'transfer', 'hosts-pin', 'replay', 'archive', 'swap' or 'billing', not '$ONLY'" >&2; exit 2 ;;
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

# A COMPLETED PASS EXITS 0, WHATEVER IT FOUND. See "THE EXIT STATUS" in the
# header: severity is the alert's job and the `worst:` line's job, and this
# unit is a Type=oneshot whose failure state has to keep meaning "the watcher
# did not run".
exit 0
