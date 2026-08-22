#!/usr/bin/env bash
# Restart the archive and keep the permanent record complete.
#
# RUN AS ROOT ON THE SERVICE HOST. Costs a participant outage for the LENGTH OF
# THE LEDGER REPLAY, which grows with the ledger and is minutes, not seconds.
# The website is down for that window too. Announce it first.
#
# THIS IS NOT TONIGHT'S SCRIPT. It is the executable form of
# RESTART-POLICY.md's "Restart with a complete record", for the next planned
# archive change and for CI to exercise. If you only need the relay back, use
# restart-relay.sh: it costs 30 to 60 seconds and never replays anything.
#
# ---------------------------------------------------------------------------
# THE SEQUENCE, AND THE ONE THING THIS ADDS TO IT
#
# The policy sequence is: stop the relay, restart the archive, wait for the
# replay to finish, start the relay promptly, and prove from the relay log that
# the archive's subscription preceded the first placement claim. Stopping the
# relay is what makes the record complete — an archive that is not subscribed
# cannot record a crossing, so the map must not run while it replays.
#
# What this adds is the peer gate. The policy's own text ends on a race it
# cannot remove: "The archive normally resubscribes before the relay places the
# first peer. That is a race and not a guarantee." Every sidecar has spent the
# whole replay failing, so each is near its 30 s backoff ceiling while the
# archive's ladder is fresh — which is why the archive normally wins, and why
# "normally" is not good enough for a permanent record. Raising the gate before
# the relay stops means that when the relay returns, the archive is the only
# Contract B client that can reach it. The race becomes a wait.
#
# ---------------------------------------------------------------------------
# THE TWO GUARDS
#
#   The archive-deploy hold. Another operator is managing the archive. See
#   rl_check_hold in restart-lib.sh.
#
#   The replay-headroom gate. RESTART-POLICY.md's pre-restart checks say: "If
#   projected archive memory is critical, do not restart the archive." An
#   archive that cannot fit its own ledger back into RAM does not come back, and
#   the outage stops being a planned window. This script reads that verdict from
#   monitor.sh rather than recomputing it, because a second copy of the
#   arithmetic is a second definition of the threshold and the two would drift.
#
# ---------------------------------------------------------------------------
# USAGE
#
#   restart-archive.sh [--dry-run] [--reason "<text>"]
#                      [--rebuild-rollup]
#                      [--ignore-archive-hold]
#                      [--i-proved-the-replay-fits --proof "<text>"]
#
#   --dry-run                    Print every step and act on nothing.
#   --reason "<text>"            Copied into the receipt. Use it.
#   --rebuild-rollup             Preserve rollup.jsonl, then rebuild it from
#                                the complete raw record. The option refuses
#                                when a raw segment is absent from the host.
#   --ignore-archive-hold        Proceed despite an archive-deploy hold.
#   --i-proved-the-replay-fits   Proceed despite a critical replay verdict.
#                                REQUIRES --proof. The flag is deliberately
#                                long: the verdict it overrides is the one that
#                                stands between a planned window and an archive
#                                that will not start.
#   --proof "<text>"             What you measured, and where it is written
#                                down. Echoed verbatim into the receipt so the
#                                deployment record carries the reasoning and not
#                                just the fact that somebody typed a flag.

set -uo pipefail

DRY=0
REBUILD_ROLLUP=0
IGNORE_HOLD=0
OVERRIDE_REPLAY=0
PROOF=""
REASON=""
RC=0

while [ $# -gt 0 ]; do
  case "$1" in
    --dry-run) DRY=1 ;;
    --rebuild-rollup) REBUILD_ROLLUP=1 ;;
    --ignore-archive-hold) IGNORE_HOLD=1 ;;
    --i-proved-the-replay-fits) OVERRIDE_REPLAY=1 ;;
    --proof) PROOF="${2:?--proof needs text}"; shift ;;
    --reason) REASON="${2:?--reason needs text}"; shift ;;
    -h|--help) sed -n '2,60p' "$0"; exit 0 ;;
    *) echo "restart-archive: unknown argument $1" >&2; exit 2 ;;
  esac
  shift
done

if [ "$OVERRIDE_REPLAY" = 1 ] && [ -z "$PROOF" ]; then
  echo "restart-archive: --i-proved-the-replay-fits requires --proof \"<text>\"." >&2
  echo "  The override is only worth having if the receipt says what was measured." >&2
  exit 2
fi

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=restart-lib.sh
. "$HERE/restart-lib.sh"
rl_init

LOCK_FILE="${MV_ARCHIVE_RESTART_LOCK:-/run/lock/bibites-archive-restart.lock}"
MONITOR="${MV_MONITOR_SH:-$HERE/monitor.sh}"
# A replay is minutes and grows with the ledger, so this is generous on purpose.
# It is a "something is wrong" timeout and not an estimate: estimate the replay
# from RESTART-POLICY.md's formula before you start.
ARCHIVE_HEALTHZ_TIMEOUT="${MV_ARCHIVE_HEALTHZ_TIMEOUT:-900}"

T_START=""; T_RELAY_STOP=""; T_ARCHIVE=""; T_HEALTHZ=""
T_RELAY_START=""; T_SUBSCRIBED=""; T_PEERS=""
REPLAY_SEV="not read"; REPLAY_MSG=""
REPLAY_SECONDS="not measured"
ROLLUP_FILE="${MV_STATE:-/var/lib/multiverse}/archive/rollup.jsonl"
SEGMENTS_DIR="${MV_STATE:-/var/lib/multiverse}/archive/segments"
ROLLUP_BACKUP="not requested"
ROLLUP_BACKUP_SHA="not measured"
REBUILD_CANCELLED=0

# Scratch holds a COPY OF deploy.env (see check_replay_headroom), so it comes
# from the library rather than from a bare mktemp -d: rl_mktemp_d registers the
# directory with the same EXIT trap that takes the gate down, and every path out
# of this script — a refused preflight, a failed restart, an interrupt, a
# --dry-run — removes it.
TMPD=""
rl_mktemp_d archive TMPD ||
  die "cannot create a private work directory under ${TMPDIR:-/tmp}"

# ---------------------------------------------------------------- preflight

preflight_reads() {
  step "preflight: what this host looks like now"
  say "relay backend   $MV_RELAY_BACKEND"
  say "archive http    $MV_ARCHIVE_HTTP"
  say "relay log       $RELAY_LOG"
  say "gate directory  $GATE_DIR"
  rl_gate_precheck
  if [ "$DRY" = 0 ] && [ ! -r "$RELAY_LOG" ]; then
    die "cannot read $RELAY_LOG. The proof comes from that log and there is no other
     proof; a restart whose result cannot be read must not start."
  fi
  local units
  units="$(systemctl is-active multiverse-relay multiverse-archive 2>/dev/null | tr '\n' ' ')"
  say "units (relay archive): ${units:-unknown}"
}

# RESTART-POLICY.md's CAUTION, made executable. An older archive unit named the
# relay in Wants= as well as After=. Wants= is a start-time pull, so
# `systemctl restart multiverse-archive` STARTED THE STOPPED RELAY AGAIN: the
# map ran live for the whole replay and nothing recorded it. That is the exact
# outage this whole sequence exists to prevent, produced by the sequence itself.
check_unit_dependency() {
  step "preflight: the archive unit must not pull the relay"
  if [ "$DRY" = 1 ] && ! command -v systemctl >/dev/null 2>&1; then
    say "[dry-run] would check: systemctl show -p Wants --value multiverse-archive.service"
    return 0
  fi
  local wants
  wants="$(systemctl show -p Wants --value multiverse-archive.service 2>/dev/null)"
  say "archive Wants= ${wants:-<empty>}"
  if printf '%s' "$wants" | grep -q 'multiverse-relay'; then
    die "the archive unit still pulls the relay in Wants=. Restarting the archive would
     START THE RELAY this script just stopped, the map would run live for the whole
     replay, and nothing would record it. Install the current units first:
       /opt/multiverse/deploy/provision.sh --only systemd"
  fi
  say "the archive unit does not pull the relay; the sequence is executable"
}

# The replay-headroom gate, read from monitor.sh's own arithmetic.
#
# monitor.sh's --only replay computes the projected peak against PHYSICAL RAM
# and reports CRIT at or above MV_REPLAY_HEADROOM. Swap is deliberately not in
# that denominator: the check once divided by MemTotal + SwapTotal, which meant
# a swap file could clear a critical verdict without changing one byte of
# retained state. Do not reintroduce that here by reading `free` instead.
#
# --only replay reads its record count from a status document and computes
# nothing without one, so this fetches /api/status first. NO DOCUMENT IS A
# REFUSAL, not a pass: an archive whose console cannot be read is an archive
# whose ledger size is unknown, and this gate exists precisely to not restart
# into an unknown.
check_replay_headroom() {
  step "preflight: does the replay still fit in RAM?"
  if [ ! -x "$MONITOR" ]; then
    REPLAY_SEV="unreadable"
    if [ "$OVERRIDE_REPLAY" = 1 ]; then
      say "no executable monitor at $MONITOR; overridden"
      return 0
    fi
    die "no executable monitor at $MONITOR, so the replay-headroom verdict cannot be
     read. Set MV_MONITOR_SH, or re-run with --i-proved-the-replay-fits --proof."
  fi

  mkdir -p "$TMPD/monitor"
  if ! curl -fs --max-time 10 "http://$MV_ARCHIVE_HTTP/api/status" -o "$TMPD/status.json" 2>/dev/null \
     || [ ! -s "$TMPD/status.json" ]; then
    REPLAY_SEV="unreadable"
    # A dry run is meant to be runnable anywhere, including CI and a laptop,
    # where no archive answers. Say what the gate WOULD do rather than refuse a
    # run that was never going to change anything.
    if [ "$DRY" = 1 ]; then
      say "[dry-run] no archive console at $MV_ARCHIVE_HTTP, so there is no verdict to read here."
      say "[dry-run] on the host this runs '$MONITOR --only replay' and REFUSES at CRIT,"
      say "[dry-run] and refuses on a console it cannot read at all."
      return 0
    fi
    if [ "$OVERRIDE_REPLAY" = 1 ]; then
      say "the archive console did not answer on $MV_ARCHIVE_HTTP; overridden"
      return 0
    fi
    die "the archive console did not answer on http://$MV_ARCHIVE_HTTP/api/status, so
     the ledger size and therefore the replay projection are unknown. This gate does
     not pass on missing evidence. Fix the archive, or re-run with
     --i-proved-the-replay-fits --proof \"<what you measured>\"."
  fi

  # monitor.sh sources the env file with `set -a` AFTER reading MV_STATE from
  # the environment, so an exported MV_STATE would be overwritten by the file's
  # own value and the monitor would write its severity into the LIVE state
  # directory. That would suppress the timer's next genuine transition alert.
  # Append the override to a copy instead: last assignment wins.
  { cat "$ENV_FILE" 2>/dev/null; printf 'MV_STATE=%s\n' "$TMPD"; } >"$TMPD/deploy.env"

  local out
  out="$(MV_ENV_FILE="$TMPD/deploy.env" MV_STATUS_JSON="$TMPD/status.json" \
         "$MONITOR" --only replay --quiet --verbose 2>&1)"
  REPLAY_SEV="$(printf '%s\n' "$out" | awk '$1 == "replay" { print $2; exit }')"
  REPLAY_MSG="$(printf '%s\n' "$out" | awk '$1 == "replay" { $1=""; $2=""; sub(/^ +/, ""); print; exit }')"
  : "${REPLAY_SEV:=no verdict}"
  say "monitor replay verdict: $REPLAY_SEV"
  [ -n "$REPLAY_MSG" ] && say "$REPLAY_MSG"

  # PRE-RESTART CHECK 4, "estimate replay time with the target-host rate", read
  # from the same pass. The gate above is a MEMORY verdict and it is what this
  # script refuses on; this is what the restart will COST the map in seconds,
  # and it is what the announcement has to state. It is printed and NOT gated
  # on: a restart that is expensive is a restart that has to be announced, and
  # refusing it here would leave an operator with no supported way to do the
  # thing the policy tells them to schedule.
  local cost_sev cost_msg
  cost_sev="$(printf '%s\n' "$out" | awk '$1 == "replay-cost" { print $2; exit }')"
  cost_msg="$(printf '%s\n' "$out" | awk '$1 == "replay-cost" { $1=""; $2=""; sub(/^ +/, ""); print; exit }')"
  if [ -n "$cost_sev" ]; then
    say "projected replay cost: $cost_sev"
    [ -n "$cost_msg" ] && say "$cost_msg"
    case "$cost_sev" in
      WARN|CRIT)
        say "THAT PROJECTION IS THE PARTICIPANT OUTAGE. This sequence holds the relay"
        say "down for the whole replay on purpose, so announce it before you proceed"
        say "(RESTART-POLICY.md, pre-restart check 7)." ;;
    esac
  fi

  case "$REPLAY_SEV" in
    OK|WARN)
      say "below the ban threshold; the restart may proceed"
      return 0 ;;
  esac

  if [ "$OVERRIDE_REPLAY" = 1 ]; then
    say "verdict is $REPLAY_SEV, OVERRIDDEN with --i-proved-the-replay-fits"
    say "proof: $PROOF"
    return 0
  fi
  die "the replay-headroom verdict is $REPLAY_SEV and RESTART-POLICY.md's pre-restart
     checks forbid an archive restart at or above the threshold: an archive that
     cannot fit its ledger back into RAM does not come back, and this stops being a
     planned window. Only three things move this number — fewer records, more RAM, or
     per-record constants (MV_REPLAY_RESIDENT_B, MV_REPLAY_PEAK_B) that a measurement
     on this host's own ledger has corrected. A swap file does not, and MV_ARCHIVE_GOMEMLIMIT
     does not. If you have measured that it fits, re-run with
       --i-proved-the-replay-fits --proof \"<what you measured, and where it is written>\""
}

# Count receipts whose raw segment is absent. This is the durable answer across
# process restarts. ledgerRetiredTotal is not: it counts only the segments that
# the current process removed.
count_retired_segments() {
  local receipt gz plain count=0
  [ -d "$SEGMENTS_DIR" ] || { printf '0\n'; return; }
  while IFS= read -r -d '' receipt; do
    gz="${receipt%.receipt}"
    plain="${gz%.gz}"
    if [ ! -f "$gz" ] && [ ! -f "$plain" ]; then
      count=$((count + 1))
    fi
  done < <(find "$SEGMENTS_DIR" -maxdepth 1 -type f -name '*.jsonl.gz.receipt' -print0)
  printf '%s\n' "$count"
}

# A capacity correction cannot recover entries that the old bounded aggregate
# never wrote to rollup.jsonl. This option pays one full replay while every raw
# line is still on the host. A receipt without either raw segment form proves
# that the on-host record is incomplete. Restore those cold copies first.
check_rollup_rebuild() {
  [ "$REBUILD_ROLLUP" = 1 ] || return 0

  step "preflight: the full raw record can rebuild the roll-up"
  local retired records="unknown"
  if [ -d "$SEGMENTS_DIR" ] && [ ! -r "$SEGMENTS_DIR" ]; then
    die "$SEGMENTS_DIR is not readable. The raw record cannot be proved complete."
  fi
  if [ -s "$TMPD/status.json" ] && command -v jq >/dev/null 2>&1; then
    records="$(jq -r '.ledgerRecords // 0' "$TMPD/status.json" 2>/dev/null)"
  fi
  retired="$(count_retired_segments)"
  case "$retired" in
    ''|*[!0-9]*) die "the retired-segment count is not a whole number: $retired" ;;
  esac
  say "ledger records       $records"
  say "absent raw segments  $retired"
  if [ "$retired" != 0 ]; then
    die "$retired raw segment(s) are absent from this host. Restore every required
     confirmed cold copy before a full rebuild. Rebuilding only the on-host window
     would discard older aggregate answers."
  fi

  if [ ! -e "$ROLLUP_FILE" ]; then
    if [ "$DRY" = 1 ] && [ ! -s "$TMPD/status.json" ]; then
      say "[dry-run] the production host must contain a regular file at $ROLLUP_FILE"
      return 0
    fi
    die "$ROLLUP_FILE is absent. Nothing can be preserved."
  fi
  [ -f "$ROLLUP_FILE" ] && [ ! -L "$ROLLUP_FILE" ] \
    || die "$ROLLUP_FILE is absent or is not a regular file. Nothing can be preserved."
  say "the complete raw record is present; the rebuild can proceed"
}

announce_reminder() {
  step "preflight: the checks this script cannot make for you"
  say "1. identity backup created and checked"
  say "2. approved off-host backup or snapshot taken"
  say "3. the window announced on the announcements page (ANNOUNCEMENT.md)"
  say "4. replay time estimated from THIS host's measured rate, not an old elapsed time"
  say "RESTART-POLICY.md, 'Pre-restart checks', owns all four. This script checks none"
  say "of them. It checks the two that can be read from the machine."
}

# ---------------------------------------------------------------- the receipt

receipt() {
  cat <<RECEIPT

======================================================================
RECEIPT — archive restart, complete record$([ "$DRY" = 1 ] && printf ' (DRY RUN — nothing changed)')
======================================================================
  host                 $(hostname 2>/dev/null || echo unknown)
  reason               ${REASON:-not given}
  archive hold         $([ -e "$HOLD_README" ] && printf 'present%s' "$([ "$IGNORE_HOLD" = 1 ] && printf ', OVERRIDDEN with --ignore-archive-hold')" || printf 'none')
  replay verdict       $REPLAY_SEV$([ "$OVERRIDE_REPLAY" = 1 ] && printf ', OVERRIDDEN with --i-proved-the-replay-fits')
  replay proof         ${PROOF:-not given}
  roll-up rebuild      $([ "$REBUILD_ROLLUP" = 1 ] && printf requested || printf 'not requested')
  prior roll-up        $ROLLUP_BACKUP
  prior roll-up sha256 $ROLLUP_BACKUP_SHA
  started              ${T_START:--}
  gate raised          ${T_GATE_UP:--}
  relay stopped        ${T_RELAY_STOP:--}
  archive restarted    ${T_ARCHIVE:--}
  archive /healthz ok  ${T_HEALTHZ:--}
  relay started        ${T_RELAY_START:--}
  archive subscribed   ${T_SUBSCRIBED:--}
  gate removed         ${T_GATE_DOWN:--}
  peers read           ${T_PEERS:--}
  finished             $(now)

  ledger replay        $REPLAY_SECONDS  (archive restarted -> /healthz answered)
  participant outage   $(rl_outage_seconds)  (gate raised -> gate removed)
  peers after ${PEER_SETTLE}s      $PEER_SUMMARY
  verdict              $VERDICT

  archive line         ${ARCHIVE_LINE:-none}
  first claim after    ${CLAIM_LINE:-none}
======================================================================
RECEIPT
}

# ---------------------------------------------------------------- sequence

T_START="$(now)"
step "archive restart, complete record$([ "$DRY" = 1 ] && printf ' — DRY RUN')"
say "started $T_START"

rl_check_root
rl_check_hold "$IGNORE_HOLD" --ignore-archive-hold
rl_take_lock "$LOCK_FILE"
preflight_reads
check_unit_dependency
check_replay_headroom
check_rollup_rebuild
announce_reminder

# THE GATE GOES UP BEFORE THE RELAY STOPS. Raising it afterwards would leave the
# window between the relay starting and the gate being written, which is the
# race this removes.
rl_gate_up || die "could not raise the gate. Nothing was restarted."

step "stop the relay — the participant outage and the complete record start here"
rl_mark_log
T_RELAY_STOP="$(now)"
if ! run systemctl stop multiverse-relay; then
  crit "could not stop the relay. Nothing else has been done."
  RC=1
fi

if [ "$RC" = 0 ]; then
  if [ "$REBUILD_ROLLUP" = 1 ]; then
    step "stop the archive before preserving its roll-up"
    if ! run systemctl stop multiverse-archive; then
      crit "the archive stop command failed. THE RELAY IS STOPPED."
      RC=1
    fi

    if [ "$RC" = 0 ]; then
      step "preserve the old roll-up and force one full replay"
      # Stop the archive before this second read. A segment can retire between
      # the preflight and the stop. If that occurs, keep the sidecar and perform
      # an ordinary, complete-record restart instead of rebuilding from a gap.
      retired_now="$(count_retired_segments)"
      if [ "$retired_now" != 0 ]; then
        crit "$retired_now raw segment(s) became absent before the archive stopped.
        The roll-up rebuild is cancelled. The old sidecar stays in place."
        REBUILD_CANCELLED=1
        ROLLUP_BACKUP="rebuild cancelled; old sidecar kept in place"
      else
        ROLLUP_BACKUP="$ROLLUP_FILE.pre-rebuild-$(date -u +%Y%m%dT%H%M%SZ)"
      fi
      if [ "$REBUILD_CANCELLED" = 1 ]; then
        :
      elif [ "$DRY" = 0 ] && [ -e "$ROLLUP_BACKUP" ]; then
        crit "the backup target already exists: $ROLLUP_BACKUP"
        RC=1
      elif ! run mv -- "$ROLLUP_FILE" "$ROLLUP_BACKUP"; then
        crit "could not preserve $ROLLUP_FILE as $ROLLUP_BACKUP. THE ARCHIVE IS STOPPED."
        RC=1
      elif [ "$DRY" = 0 ]; then
        if ! ROLLUP_BACKUP_SHA="$(sha256sum "$ROLLUP_BACKUP" | awk '{print $1}')" \
           || [ -z "$ROLLUP_BACKUP_SHA" ]; then
          crit "could not calculate the checksum of $ROLLUP_BACKUP. THE ARCHIVE IS STOPPED."
          RC=1
        else
          say "preserved $ROLLUP_BACKUP"
          say "sha256 $ROLLUP_BACKUP_SHA"
        fi
      fi
    fi

    if [ "$RC" = 0 ]; then
      step "start the archive and rebuild the roll-up"
      T_ARCHIVE="$(now)"
      if ! run systemctl start multiverse-archive; then
        crit "the archive start command failed. THE RELAY IS STOPPED."
        RC=1
      fi
    fi
  else
    step "restart the archive"
    T_ARCHIVE="$(now)"
    if ! run systemctl restart multiverse-archive; then
      crit "the archive restart command failed. THE RELAY IS STOPPED."
      RC=1
    fi
  fi
fi

if [ "$RC" = 0 ]; then
  step "wait for the ledger replay"
  say "the archive binds its HTTP port only after the replay finishes, so this"
  say "answers exactly once the replay is done"
  if [ "$DRY" = 1 ]; then
    say "[dry-run] would poll http://$MV_ARCHIVE_HTTP/healthz for up to ${ARCHIVE_HEALTHZ_TIMEOUT}s"
  else
    deadline=$(( $(epoch) + ARCHIVE_HEALTHZ_TIMEOUT ))
    while [ "$(epoch)" -lt "$deadline" ]; do
      if curl -fs --max-time 5 "http://$MV_ARCHIVE_HTTP/healthz" >/dev/null 2>&1; then
        T_HEALTHZ="$(now)"
        REPLAY_SECONDS="$(( $(date -u -d "$T_HEALTHZ" +%s) - $(date -u -d "$T_ARCHIVE" +%s) ))s"
        say "archive answers /healthz at $T_HEALTHZ after $REPLAY_SECONDS"
        break
      fi
      sleep 5
    done
    if [ -z "$T_HEALTHZ" ]; then
      crit "the archive did not answer /healthz within ${ARCHIVE_HEALTHZ_TIMEOUT}s.
      THE RELAY IS STILL STOPPED and the map is down. Do not wait for this script:
        systemctl status multiverse-archive
        journalctl -u multiverse-archive -n 200
      If the archive cannot replay, START THE RELAY BY HAND to end the participant
      outage — the record will have a gap, and that gap belongs in the record:
        systemctl start multiverse-relay"
      RC=1
    fi
  fi
fi

if [ "$RC" = 0 ]; then
  # "Promptly" is the policy's word and it is load-bearing: every extra second
  # here is a second of participant outage for no gain, because the archive is
  # already up and the gate — not the timing — is what orders the subscription.
  step "start the relay, promptly"
  T_RELAY_START="$(now)"
  if ! run systemctl start multiverse-relay; then
    crit "could not start the relay. The map is DOWN. Start it by hand."
    RC=1
  fi
fi

if [ "$RC" = 0 ]; then
  if rl_wait_relay_healthz; then :; else RC=1; fi
fi
if [ "$RC" = 0 ]; then
  if rl_wait_archive_subscribed; then T_SUBSCRIBED="$(now)"; else RC=1; fi
fi

rl_gate_down

if [ "$RC" = 0 ]; then
  rl_prove_order || RC=1
  rl_count_peers || RC=1
  T_PEERS="$(now)"
fi

if [ "$REBUILD_CANCELLED" = 1 ] && [ "$RC" = 0 ]; then
  RC=1
fi

receipt

if [ "$RC" != 0 ]; then
  if [ "$REBUILD_CANCELLED" = 1 ]; then
    printf '\nCRIT: the restart completed, but the requested roll-up rebuild was cancelled.\n' >&2
    printf '      Restore the absent raw segments before another rebuild attempt.\n' >&2
  else
    printf '\nCRIT: the archive restart did not complete cleanly. The gate is down.\n' >&2
    printf '      Read the receipt above and write it into the deployment record,\n' >&2
    printf '      including any record gap.\n' >&2
  fi
fi
exit "$RC"
