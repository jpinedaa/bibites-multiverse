#!/usr/bin/env bash
# Restart the relay, and only the relay, without losing a ledger record.
#
# RUN AS ROOT ON THE SERVICE HOST. Costs a participant outage of roughly 30 to
# 60 seconds. Does not touch the archive, does not replay the ledger, and does
# not take the website down.
#
# ---------------------------------------------------------------------------
# WHAT THIS SOLVES
#
# RESTART-POLICY.md, "Restart with a complete record", ends in a race that it
# names honestly and cannot remove: when the relay comes back, the archive and
# every sidecar are dialling it on the same 1-30 s backoff, and the permanent
# record is whole only if the archive's SUBSCRIBE lands before the first peer's
# placement claim. It normally does — the archive's ladder is short and each
# sidecar's has climbed toward its ceiling during the outage — but a lost race
# is a permanent hole in the ledger, and the operator finds out afterwards by
# reading the log, when nothing can be done about it.
#
# The peer gate replaces that race with an order. While the gate is up, nginx
# answers every NON-loopback connection to /contract-b/ with 503 and never
# reaches the relay; the archive is on 127.0.0.1 and is pinned open by an exact
# key in the map, so it is the only thing that can subscribe. The peers come
# back after the archive is provably subscribed, not in a footrace with it.
#
# The gate is an operational shutter and not a security control. Peers hold
# credentials and the relay checks them. This decides only whether a connection
# reaches the relay to be checked at all.
#
# The teardown rule, the gate mechanism and the proof live in restart-lib.sh,
# which restart-archive.sh sources too.
#
# ---------------------------------------------------------------------------
# USAGE
#
#   restart-relay.sh [--dry-run] [--ignore-archive-hold] [--reason "<text>"]
#
#   --dry-run               Print every step and act on nothing. Reads state,
#                           writes none. Safe on a live host and in CI.
#   --ignore-archive-hold   Proceed even though an archive-deploy hold is in
#                           place. Read rl_check_hold in restart-lib.sh first.
#   --reason "<text>"       Copied into the receipt. Use it.
#
# The receipt on stdout is written to be pasted into a deployment record.

set -uo pipefail

DRY=0
IGNORE_HOLD=0
REASON=""
RC=0

while [ $# -gt 0 ]; do
  case "$1" in
    --dry-run) DRY=1 ;;
    --ignore-archive-hold) IGNORE_HOLD=1 ;;
    --reason) REASON="${2:?--reason needs text}"; shift ;;
    -h|--help) sed -n '2,45p' "$0"; exit 0 ;;
    *) echo "restart-relay: unknown argument $1" >&2; exit 2 ;;
  esac
  shift
done

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=restart-lib.sh
. "$HERE/restart-lib.sh"
rl_init

LOCK_FILE="${MV_RELAY_RESTART_LOCK:-/run/lock/bibites-relay-restart.lock}"
T_START=""; T_RESTART=""; T_HEALTHZ=""; T_SUBSCRIBED=""; T_PEERS=""

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
  if [ "$DRY" = 0 ] && ! systemctl is-active --quiet multiverse-archive; then
    die "the archive is not active. This sequence waits for the archive to resubscribe
     and it never will. Fix the archive first, or use restart-archive.sh."
  fi
}

# ---------------------------------------------------------------- the receipt

receipt() {
  cat <<RECEIPT

======================================================================
RECEIPT — relay-only restart$([ "$DRY" = 1 ] && printf ' (DRY RUN — nothing changed)')
======================================================================
  host                 $(hostname 2>/dev/null || echo unknown)
  reason               ${REASON:-not given}
  archive hold         $([ -e "$HOLD_README" ] && printf 'present%s' "$([ "$IGNORE_HOLD" = 1 ] && printf ', OVERRIDDEN with --ignore-archive-hold')" || printf 'none')
  started              ${T_START:--}
  gate raised          ${T_GATE_UP:--}
  relay restarted      ${T_RESTART:--}
  relay /healthz ok    ${T_HEALTHZ:--}
  archive subscribed   ${T_SUBSCRIBED:--}
  gate removed         ${T_GATE_DOWN:--}
  peers read           ${T_PEERS:--}
  finished             $(now)

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
step "relay-only restart$([ "$DRY" = 1 ] && printf ' — DRY RUN')"
say "started $T_START"

rl_check_root
rl_check_hold "$IGNORE_HOLD" --ignore-archive-hold
rl_take_lock "$LOCK_FILE"
preflight_reads

rl_gate_up || die "could not raise the gate. Nothing was restarted."

step "restart the relay"
rl_mark_log
T_RESTART="$(now)"
if ! run systemctl restart multiverse-relay; then
  crit "the relay restart command failed"
  RC=1
else
  say "restart issued at $T_RESTART"
fi

if [ "$RC" = 0 ]; then
  if rl_wait_relay_healthz; then T_HEALTHZ="$(now)"; else RC=1; fi
fi
if [ "$RC" = 0 ]; then
  if rl_wait_archive_subscribed; then T_SUBSCRIBED="$(now)"; else RC=1; fi
fi

# The gate comes down here on the success path, and the EXIT trap takes it down
# on every other one. Either way it is down before the proof is printed, because
# the proof is a report and the gate is an outage.
rl_gate_down

if [ "$RC" = 0 ]; then
  rl_prove_order || RC=1
  rl_count_peers || RC=1
  T_PEERS="$(now)"
fi

receipt

if [ "$RC" != 0 ]; then
  printf '\nCRIT: the relay-only restart did not complete cleanly. The gate is down.\n' >&2
  printf '      Read the receipt above and write it into the deployment record.\n' >&2
fi
exit "$RC"
