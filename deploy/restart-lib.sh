# Shared parts of the restart scripts. Sourced, never executed.
#
#   restart-relay.sh    a relay-only restart, roughly 30 to 60 seconds
#   restart-archive.sh  the complete-record sequence, the length of a replay
#
# Both procedures raise the same peer gate, restart something, wait for the
# archive to resubscribe, take the gate down, and prove the order from the relay
# log. Only what they restart and what they must check first differ. That common
# half lives here so the two cannot drift: a fix to the teardown or to the proof
# that landed in one script and not the other would be discovered during an
# outage, by the operator running the script that did not get it.
#
# A caller must set DRY (0 or 1) before sourcing, and must call rl_init once
# after sourcing to load parameters and install the teardown trap.

# ---------------------------------------------------------------- output

now()   { date -u +%Y-%m-%dT%H:%M:%SZ; }
epoch() { date -u +%s; }
step()  { printf '\n==== %s  %s\n' "$(now)" "$*"; }
say()   { printf '     %s\n' "$*"; }
die()   { printf '\nSTOP: %s\n' "$*" >&2; exit 2; }
crit()  { printf '\nCRIT: %s\n' "$*" >&2; }

# Everything that changes the host goes through run(), so --dry-run is a
# property of one function rather than a promise made in twenty places.
run() {
  if [ "${DRY:-0}" = 1 ]; then printf '     [dry-run] %s\n' "$*"; return 0; fi
  "$@"
}

# ---------------------------------------------------------------- parameters

# Environment seams, all optional, all defaulted from /etc/multiverse/deploy.env
# or from the same constants provision.sh uses. They exist so these scripts can
# be exercised off the host; do not set them on the host.
rl_init() {
  ENV_FILE="${MV_ENV_FILE:-/etc/multiverse/deploy.env}"
  if [ -r "$ENV_FILE" ]; then
    # shellcheck source=/dev/null
    set -a; . "$ENV_FILE"; set +a
  elif [ "${DRY:-0}" = 0 ]; then
    die "no readable $ENV_FILE. This script needs the deployed parameters."
  else
    say "[dry-run] no readable $ENV_FILE; using built-in defaults"
  fi

  : "${MV_RELAY_BACKEND:=127.0.0.1:8795}"
  : "${MV_ARCHIVE_HTTP:=127.0.0.1:8796}"
  : "${MV_LOGDIR:=/var/log/multiverse}"

  GATE_DIR="${MV_GATEDIR:-/etc/multiverse/nginx-gates}"
  GATE_FILE="$GATE_DIR/peer-gate.map"
  RELAY_LOG="${MV_RELAY_LOG:-$MV_LOGDIR/relay.log}"
  HOLD_README="${MV_ARCHIVE_HOLD:-/run/lock/bibites-archive-deploy.HOLD-README}"
  HEALTHZ_TIMEOUT="${MV_HEALTHZ_TIMEOUT:-60}"
  SUBSCRIBE_TIMEOUT="${MV_SUBSCRIBE_TIMEOUT:-90}"
  PEER_SETTLE="${MV_PEER_SETTLE_SECONDS:-60}"

  # The catch-all the operator's file contains. The regex matches every address;
  # the map's exact 127.0.0.1 and ::1 keys are consulted first and win, which is
  # why this cannot gate the archive however broadly it is written.
  GATE_CONTENT='~. 1;'

  GATE_IS_UP=0
  LOG_OFFSET=0
  T_GATE_UP=""; T_GATE_DOWN=""
  ARCHIVE_LINE=""; CLAIM_LINE=""; PEER_SUMMARY="not read"
  VERDICT="INCOMPLETE"
  : "${RC:=0}"

  trap rl_on_exit EXIT
  trap 'crit "interrupted"; exit 130' INT TERM
}

# ---------------------------------------------------------------- root, hold, lock

rl_check_root() {
  [ "${DRY:-0}" = 1 ] && return 0
  [ "$(id -u)" = 0 ] || die "run this as root. It reloads nginx and restarts units."
}

# THE ARCHIVE-DEPLOY HOLD, and why both scripts refuse on it.
#
# A hold file is a person saying "the archive is in a state I am managing".
# restart-archive.sh plainly collides with that. restart-relay.sh does not
# restart the archive at all — but it stops the relay, which DROPS THE ARCHIVE'S
# SUBSCRIPTION and then waits for it to return. Doing that inside somebody
# else's archive window gives two operators one relay log and one set of
# timestamps, and afterwards neither receipt can be read.
#
# The flag exists because for a relay-only restart the refusal is CONSERVATIVE
# RATHER THAN NECESSARY, and an operator who has read the hold and knows it does
# not apply should not have to edit this script or move a file another session
# owns. Refusing is the default because the cost of being wrong is asymmetric:
# waiting costs minutes, and colliding with an archive window costs a record gap
# that cannot be reconstructed.
#
# Read the hold, find who wrote it, ask them, and only then pass the flag.
rl_check_hold() { # rl_check_hold <ignore:0|1> <flag-name>
  local ignore="$1" flag="$2"
  step "preflight: the archive-deploy hold"
  if [ ! -e "$HOLD_README" ]; then
    say "no hold at $HOLD_README"
    return 0
  fi
  say "a hold EXISTS at $HOLD_README:"
  sed 's/^/       | /' "$HOLD_README" 2>/dev/null | head -20
  if [ "$ignore" = 1 ]; then
    say "$flag given; proceeding. This is recorded in the receipt."
    return 0
  fi
  die "an archive-deploy hold is in place. Read it above, find the operator who wrote
     it, agree the window, and re-run with $flag if it genuinely does not apply."
}

rl_take_lock() { # rl_take_lock <lock-file>
  local lock="$1"
  step "preflight: the exclusive lock"
  # TEST THE FILE FIRST, and only then exec. `exec 9>"$f" 2>/dev/null` looks
  # like a guarded open and is not: exec with no command applies EVERY
  # redirection on the line to the shell permanently, so the 2>/dev/null would
  # silence this script's own stderr for the rest of the run — every CRIT, every
  # die, gone — and a failed restart would report success-shaped silence.
  if ! : >>"$lock" 2>/dev/null; then
    if [ "${DRY:-0}" = 1 ]; then
      say "[dry-run] cannot create $lock here; on the host this would take an exclusive flock"
      return 0
    fi
    die "cannot open $lock for writing"
  fi
  exec 9>"$lock"
  if ! flock -n 9; then
    die "another restart holds $lock. Do not run two."
  fi
  say "hold $lock exclusively (flock -n)"
}

# ---------------------------------------------------------------- the gate

rl_reload_nginx() {
  if [ "${DRY:-0}" = 1 ]; then
    say "[dry-run] nginx -t && systemctl reload nginx"
    return 0
  fi
  nginx -t || return 1
  systemctl reload nginx
}

rl_gate_precheck() {
  if [ -e "$GATE_FILE" ]; then
    die "$GATE_FILE ALREADY EXISTS. Either a restart is running, or an earlier one
     left the gate up and peers are being refused right now. Read it, and if it is
     stale remove it and reload nginx before running this."
  fi
}

rl_gate_up() {
  step "raise the peer gate"
  say "writing $GATE_FILE  ($GATE_CONTENT)"
  if [ "${DRY:-0}" = 1 ]; then
    say "[dry-run] would write the gate file and reload nginx"
    T_GATE_UP="$(now)"
    # A dry run must walk every step, and the teardown is the step most worth
    # seeing. Mark the gate up so rl_gate_down prints itself instead of
    # returning early on a run where nothing was ever raised.
    GATE_IS_UP=1
    return 0
  fi
  install -d -m 0755 -o root -g root "$GATE_DIR" || return 1
  printf '%s\n' "$GATE_CONTENT" >"$GATE_FILE" || return 1
  chmod 0644 "$GATE_FILE"
  if ! rl_reload_nginx; then
    # A configuration that will not load must not leave a file behind that the
    # NEXT reload — anyone's, for any reason — would pick up.
    rm -f "$GATE_FILE"
    return 1
  fi
  GATE_IS_UP=1
  T_GATE_UP="$(now)"
  say "gate up at $T_GATE_UP — every non-loopback address now gets 503 on /contract-b/"
}

# THE ONE RULE NEITHER SCRIPT WILL BREAK: THE GATE COMES DOWN. On success, on
# timeout, on an unexpected error, on a signal. A gate that outlives the
# operator who raised it is a self-inflicted outage of exactly the kind these
# scripts exist to shorten, and it is silent: the relay is healthy, the website
# is up, the monitor is green, and no peer can connect. That is why this is
# reachable from an EXIT trap and not only from the success path.
rl_gate_down() {
  [ "$GATE_IS_UP" = 1 ] || return 0
  step "remove the peer gate"
  if [ "${DRY:-0}" = 1 ]; then
    say "[dry-run] would remove $GATE_FILE and reload nginx"
    T_GATE_DOWN="$(now)"
    GATE_IS_UP=0
    return 0
  fi
  rm -f "$GATE_FILE"
  if ! rl_reload_nginx; then
    # The file is already gone, so the gate falls the moment anything reloads
    # nginx. Say so loudly rather than pretend the restart succeeded.
    crit "the gate FILE is removed but nginx did not reload. Peers are still being
      refused by the running configuration. Run:  nginx -t && systemctl reload nginx"
    RC=1
    return 1
  fi
  GATE_IS_UP=0
  T_GATE_DOWN="$(now)"
  say "gate down at $T_GATE_DOWN — peers can reach the relay again"
}

rl_on_exit() {
  local rc=$?
  if [ "${GATE_IS_UP:-0}" = 1 ]; then
    crit "exiting with the gate still up; taking it down before anything else"
    rl_gate_down
  fi
  exit "$rc"
}

# ---------------------------------------------------------------- the relay log

rl_mark_log() {
  LOG_OFFSET="$(stat -c %s "$RELAY_LOG" 2>/dev/null || echo 0)"
  say "relay log offset before the restart: $LOG_OFFSET bytes"
}

# Everything the relay wrote since rl_mark_log. The relay rotates its own log,
# so a size that went BACKWARDS means the window straddled a rotation and the
# offset is meaningless; fall back to the whole current file rather than to
# nothing.
rl_log_since_mark() {
  local size
  size="$(stat -c %s "$RELAY_LOG" 2>/dev/null || echo 0)"
  if [ "$size" -lt "$LOG_OFFSET" ]; then
    cat "$RELAY_LOG" 2>/dev/null
  else
    tail -c +$(( LOG_OFFSET + 1 )) "$RELAY_LOG" 2>/dev/null
  fi
}

# ---------------------------------------------------------------- waits

rl_wait_relay_healthz() {
  step "wait for the relay to answer /healthz"
  if [ "${DRY:-0}" = 1 ]; then
    say "[dry-run] would poll http://$MV_RELAY_BACKEND/healthz for up to ${HEALTHZ_TIMEOUT}s"
    return 0
  fi
  local deadline
  deadline=$(( $(epoch) + HEALTHZ_TIMEOUT ))
  while [ "$(epoch)" -lt "$deadline" ]; do
    if curl -fs --max-time 5 "http://$MV_RELAY_BACKEND/healthz" >/dev/null 2>&1; then
      say "relay answers /healthz at $(now)"
      return 0
    fi
    sleep 2
  done
  crit "the relay did not answer http://$MV_RELAY_BACKEND/healthz within ${HEALTHZ_TIMEOUT}s"
  return 1
}

# The whole point of the gate is that this is a WAIT AND NOT A RACE: with peers
# refused at the front door, the archive is the only Contract B client that can
# reach the relay, so the only thing that can happen next is the thing being
# waited for.
rl_wait_archive_subscribed() {
  step "wait for the archive to resubscribe (gate still up)"
  if [ "${DRY:-0}" = 1 ]; then
    say "[dry-run] would watch $RELAY_LOG for 'client connected .. role=archive' for up to ${SUBSCRIBE_TIMEOUT}s"
    ARCHIVE_LINE='[dry-run] time=... msg="relay: client connected" peer=archive-main role=archive ...'
    return 0
  fi
  local deadline
  deadline=$(( $(epoch) + SUBSCRIBE_TIMEOUT ))
  while [ "$(epoch)" -lt "$deadline" ]; do
    ARCHIVE_LINE="$(rl_log_since_mark | grep -E 'client connected.*role=archive' | head -1)"
    if [ -n "$ARCHIVE_LINE" ]; then
      say "archive subscribed at $(now)"
      return 0
    fi
    sleep 2
  done
  crit "the archive did NOT resubscribe within ${SUBSCRIBE_TIMEOUT}s.
      Taking the gate down anyway — peers must not stay locked out for a fault that
      is not theirs. THE RECORD IS AT RISK: the relay is serving and the archive is
      not subscribed, so any crossing from here until it reconnects is unrecorded.
      Check:  systemctl status multiverse-archive
              curl -s http://$MV_ARCHIVE_HTTP/api/status | grep relayConnected
              tail -50 $RELAY_LOG"
  return 1
}

# ---------------------------------------------------------------- the proof

rl_prove_order() {
  step "proof: the archive line must precede the first placement claim"
  if [ "${DRY:-0}" = 1 ]; then
    say "[dry-run] would read both lines from $RELAY_LOG and assert their order"
    VERDICT="DRY-RUN (nothing was restarted)"
    return 0
  fi
  local window first_archive first_claim
  window="$(rl_log_since_mark | grep -nE 'client connected.*role=archive|placement claim')"
  first_archive="$(printf '%s\n' "$window" | grep -E 'client connected.*role=archive' | head -1)"
  first_claim="$(printf '%s\n' "$window" | grep -E 'placement claim' | head -1)"
  ARCHIVE_LINE="${first_archive#*:}"
  CLAIM_LINE="${first_claim#*:}"

  if [ -z "$first_archive" ]; then
    VERDICT="GAP — no archive subscription in this window"
    crit "$VERDICT"
    return 1
  fi
  say "archive:  $ARCHIVE_LINE"
  if [ -z "$first_claim" ]; then
    say "claim:    none yet in this window"
    VERDICT="COMPLETE RECORD — archive subscribed; no placement claim yet"
    return 0
  fi
  say "claim:    $CLAIM_LINE"
  # Both come from one `grep -n` over one stream, so comparing the line numbers
  # compares position in the log. Wall-clock timestamps in the lines would be
  # the same reading with more ways to parse it wrong.
  if [ "${first_archive%%:*}" -lt "${first_claim%%:*}" ]; then
    VERDICT="COMPLETE RECORD — the archive line precedes the first placement claim"
    say "$VERDICT"
    return 0
  fi
  VERDICT="GAP — a placement claim precedes the archive line"
  crit "$VERDICT. The permanent record has a hole from that claim to the archive
      line. This should be impossible with the gate up: it means something reached
      the relay from loopback, or the gate did not load. Put the gap in the
      deployment record and investigate the front door."
  return 1
}

rl_count_peers() {
  step "proof: peers back after ${PEER_SETTLE}s"
  if [ "${DRY:-0}" = 1 ]; then
    say "[dry-run] would sleep ${PEER_SETTLE}s and read http://$MV_ARCHIVE_HTTP/api/status"
    PEER_SUMMARY="[dry-run] not read"
    return 0
  fi
  say "sleeping ${PEER_SETTLE}s — each sidecar's backoff ladder is 1 to 30 s, so a"
  say "reading taken sooner counts the ladder rather than the outcome"
  sleep "$PEER_SETTLE"
  local body
  body="$(curl -fs --max-time 10 "http://$MV_ARCHIVE_HTTP/api/status" 2>/dev/null)"
  if [ -z "$body" ]; then
    PEER_SUMMARY="the console API did not answer on $MV_ARCHIVE_HTTP"
    crit "$PEER_SUMMARY"
    return 1
  fi
  if command -v jq >/dev/null 2>&1; then
    PEER_SUMMARY="$(printf '%s' "$body" | jq -r '"\(.totals.liveSlots // 0) live, \(.totals.darkSlots // 0) dark of \(.slotCount // 0) slots; relayConnected \(.relayConnected)"')"
  else
    PEER_SUMMARY="$(printf '%s' "$body" | tr -d ' \n' | grep -oE '"liveSlots":[0-9]+|"darkSlots":[0-9]+|"slotCount":[0-9]+|"relayConnected":(true|false)' | tr '\n' ' ')"
  fi
  say "$PEER_SUMMARY"
}

rl_outage_seconds() {
  if [ -n "$T_GATE_UP" ] && [ -n "$T_GATE_DOWN" ] && [ "${DRY:-0}" = 0 ]; then
    printf '%ss' "$(( $(date -u -d "$T_GATE_DOWN" +%s) - $(date -u -d "$T_GATE_UP" +%s) ))"
  else
    printf 'not measured'
  fi
}
