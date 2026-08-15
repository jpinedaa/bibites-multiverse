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
#
# WHAT IT WATCHES, and why each one is on the list rather than a longer one:
#
#   units                 the supervisor gave up. Restart=always has a limit, and
#                         the limit existing is what makes this check necessary.
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
#   free disk             WP3's done-when, and Risk 5. The 2026-08-08 ENOSPC
#                         outage is what running out looks like.
#   error lines           WP3's done-when. A rate, from the rotated logs.
#   certificate           days left on the certificate THE LISTENER SERVES, not
#                         the one on disk — which is also how a deploy hook that
#                         silently stopped running is caught.
#   replay headroom       the tripwire the hosting options document asked for:
#                         0.18 KB of peak RAM per ledger record while replaying
#                         and 0.30 KB held resident afterwards, so an archive
#                         eventually outgrows its box. It is better to learn that
#                         from an alert than from a restart.
#   genome gaps           Risk 7. The archive's arrears on genome capture, and
#                         the part a departed stranger takes with them is
#                         permanent.
#   backup freshness      a backup timer that stopped is invisible until the day
#                         it is needed.
#   reboot required       a pending kernel. Unattended upgrades do NOT reboot
#                         here, deliberately, so somebody has to know.
#
# THE ALERT CHANNEL, chosen for a one-person operator on a $12 VPS.
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
# sends anything, and silence is indistinguishable from health. Two answers, both
# cheap: the daily heartbeat below, whose ABSENCE is the signal; and a Lightsail
# metric alarm in the AWS console (instance status check -> email), which is the
# only watcher that survives the box being dead. The console alarm is the owner's
# one manual step; see README.md.
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
: "${MV_ARCHIVE_HTTP:=127.0.0.1:8796}"
: "${MV_EXPECTED_PEERS:=0}"
: "${MV_STATUS_AGE_WARN_MS:=120000}"
: "${MV_BYPASS_RUNS:=3}"
: "${MV_DISK_WARN_PCT:=25}"
: "${MV_DISK_CRIT_PCT:=12}"
: "${MV_ERROR_ALERT:=5}"
: "${MV_GAPS_DELTA_ALERT:=20000}"
: "${MV_CERT_MIN_DAYS:=10}"
: "${MV_REPLAY_HEADROOM:=0.85}"
: "${MV_ALERT_KIND:=ntfy}"
: "${MV_ALERT_URL:=}"
: "${MV_ALERT_COMMAND:=}"
: "${MV_ALERT_REPEAT_HOURS:=12}"
: "${MV_HEARTBEAT_HOUR:=9}"

STATE="$MV_STATE/monitor"
ARCHIVE_DATA="$MV_STATE/archive"
VERBOSE=0
QUIET=0
NOW="$(date -u +%s)"
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
    --test)
      if notify OK self-test "This is a test from $(hostname) at $(date -u +%FT%TZ). If you are reading it, the alert channel works."; then
        echo "sent one $MV_ALERT_KIND alert"
        exit 0
      fi
      echo "the alert channel is NOT working: kind=$MV_ALERT_KIND url set=$([ -n "$MV_ALERT_URL" ] && echo yes || echo no)" >&2
      exit 1 ;;
    -h|--help) sed -n '2,70p' "$0"; exit 0 ;;
    *) echo "monitor: unknown argument $1" >&2; exit 2 ;;
  esac
  shift
done

# ---------------------------------------------------------------- the checks

check_units() {
  local u state bad=""
  for u in multiverse-relay multiverse-archive nginx; do
    state="$(systemctl is-active "$u" 2>/dev/null || true)"
    [ "$state" = active ] || bad="$bad $u=$state"
  done
  if [ -n "$bad" ]; then
    report units CRIT "systemd is not running:$bad. If it says 'failed', the restart limit was reached — 'systemctl status' then RESTART-POLICY.md."
  else
    report units OK "relay, archive and nginx active"
  fi
}

check_relay_healthz() {
  local out
  out="$(curl -fsS --max-time 10 "https://${MV_DOMAIN}:${MV_RELAY_PORT}/healthz" 2>&1)"
  if [ "$out" = ok ]; then
    report relay-healthz OK "answers on ${MV_RELAY_PORT}"
  else
    report relay-healthz CRIT "no healthy answer on https://${MV_DOMAIN}:${MV_RELAY_PORT}/healthz — $out"
  fi
}

STATUS_JSON="$TMP/status.json"
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
    report disk CRIT "${free}% free (${availg} GB)$tail. The 2026-08-08 ENOSPC outage stopped every genome write and left durability damage. Grow the volume — Lightsail can do it online — or apply the retention rule."
  elif [ "$free" -le "$MV_DISK_WARN_PCT" ]; then
    report disk WARN "${free}% free (${availg} GB)$tail. SIZING.md has the arithmetic and the options."
  else
    report disk OK "${free}% free (${availg} GB)$tail"
  fi
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
    report cert WARN "could not read the certificate the relay is serving"
    return
  fi
  end="$(printf '%s' "$served" | openssl x509 -noout -enddate 2>/dev/null | cut -d= -f2)"
  days=$(( ( $(date -u -d "$end" +%s 2>/dev/null || echo "$NOW") - NOW ) / 86400 ))

  fp_served="$(printf '%s' "$served" | openssl x509 -noout -fingerprint -sha256 2>/dev/null | cut -d= -f2)"
  fp_file="$(openssl x509 -in "$MV_TLSDIR/fullchain.pem" -noout -fingerprint -sha256 2>/dev/null | cut -d= -f2)"

  if [ -n "$fp_file" ] && [ "$fp_served" != "$fp_file" ]; then
    # Not an error by itself: the reloader picks a new pair up on the NEXT
    # handshake, so a fresh copy can legitimately be one connection ahead.
    report cert WARN "the certificate on disk is not the one being served. If this persists past the next handshake the CertReloader is not picking the pair up. ($days days left on the served one.)"
  elif [ "$days" -lt "$MV_CERT_MIN_DAYS" ]; then
    report cert CRIT "$days day(s) of certificate left. certbot renews at 30, so renewal has failed: 'certbot renew --dry-run' and check port 80."
  else
    report cert OK "$days days left, disk copy matches the served one"
  fi
}

check_replay_headroom() {
  [ -s "$STATUS_JSON" ] || return 0
  local records peak_mb resident_mb worst_mb worst_what mem_kb swap_kb avail_mb ratio
  records="$(jq -r '.ledgerRecords // 0' "$STATUS_JSON" 2>/dev/null)"
  case "$records" in ''|*[!0-9]*) return ;; esac
  # TWO models, because the box has to satisfy both and they trade places.
  #
  #   184 B/record  peak while replaying. This is the STREAMING archive's, the
  #                 one this kit ships (measured 2026-08-12 on a copy of the
  #                 deployment's own 8.16 M-record ledger). An archive built
  #                 BEFORE that date materialised the whole ledger and peaked at
  #                 ~1330 B/record — seven times this — so a box running an old
  #                 binary is not covered by this check and the fix is the
  #                 upgrade.
  #   300 B/record  held resident for the rest of the day, from the living
  #                 deployment, twice at twelvefold different scale.
  #
  # Since the replay was fixed the RESIDENT term is the larger one, so this
  # alerts on the larger of the two rather than on the replay alone: an archive
  # that restarts fine and then cannot hold what it replayed is the same outage.
  # SIZING.md §4.
  peak_mb=$(( records * 184 / 1048576 ))
  resident_mb=$(( records * 300 / 1048576 ))
  worst_mb=$peak_mb; worst_what="replay peak"
  if [ "$resident_mb" -gt "$peak_mb" ]; then
    worst_mb=$resident_mb; worst_what="resident set"
  fi
  mem_kb="$(awk '/^MemTotal:/  {print $2}' /proc/meminfo)"
  swap_kb="$(awk '/^SwapTotal:/ {print $2}' /proc/meminfo)"
  avail_mb=$(( (mem_kb + swap_kb) / 1024 ))
  [ "$avail_mb" -gt 0 ] || return 0
  ratio="$(awk -v p="$worst_mb" -v a="$avail_mb" 'BEGIN{printf "%.2f", p/a}')"

  if awk -v r="$ratio" -v h="$MV_REPLAY_HEADROOM" 'BEGIN{exit !(r >= h)}'; then
    report replay CRIT "the archive can no longer be sure of fitting on this box: $records ledger records project a ~${worst_mb} MB ${worst_what} (replay peak ~${peak_mb} MB, resident ~${resident_mb} MB) against ${avail_mb} MB of RAM+swap (ratio $ratio). THE MAP IS FINE — the relay is unaffected — but the archive must not be restarted until this is fixed. If the binding term is the resident set, GOMEMLIMIT and swap do not fix it: the fixes are a bigger instance and the retention rule. SIZING.md."
  elif awk -v r="$ratio" -v h="$MV_REPLAY_HEADROOM" 'BEGIN{exit !(r >= h*0.75)}'; then
    report replay WARN "the archive's ${worst_what} projects to ~${worst_mb} MB against ${avail_mb} MB RAM+swap (ratio $ratio). Decide the retention rule or size up before it crosses $MV_REPLAY_HEADROOM."
  else
    report replay OK "~${worst_mb} MB projected ${worst_what}, ${avail_mb} MB available (ratio $ratio)"
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

check_units
check_relay_healthz
if check_archive_healthz; then
  check_map
  check_bypass
  check_replay_headroom
  check_gaps
fi
check_disk
check_errors
check_cert
check_backup
check_reboot

# The dead man's half. A daily line that says the watcher itself is alive, so
# that its ABSENCE is a signal rather than a comfort.
if [ "$MV_HEARTBEAT_HOUR" -ge 0 ] 2>/dev/null; then
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
