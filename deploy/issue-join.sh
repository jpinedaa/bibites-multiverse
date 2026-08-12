#!/usr/bin/env bash
# Issue a join string to a new participant.
#
#   issue-join.sh peer-alice                  one peer, with the confirmation
#   issue-join.sh peer-alice peer-bob -y      a batch, no prompt
#   issue-join.sh --check                     who holds a credential today
#   issue-join.sh --grant subscribe obs-1     a subscriber, deliberately
#
# WHY THIS IS A SCRIPT AND NOT A COMMAND. Three facts collide, and each one is
# cheap to get wrong by hand:
#
# 1. MINTING IS A STARTUP COMMAND. --mint-credential opens the data directory,
#    acts and exits. A SERVING relay has already read peers.json into memory and
#    writes it back whole on its own next mint (a handover), so a mint against a
#    live relay's data directory is a second writer: the new peer gets HTTP 401
#    until a restart, and a later handover silently erases the credential. So
#    ISSUANCE COSTS A RELAY RESTART, and this script takes it deliberately —
#    stop, mint every requested identity, start — instead of leaving a
#    credential that appears to exist and does not work.
#
# 2. THE RELAY KEEPS A VERIFIER, NOT THE SECRET. The join string is printed ONCE
#    and cannot be printed again. The only recovery from a lost one is
#    --handover-slot, which mints for a NEW peerId and drops the old — a message
#    to a participant who then has to re-apply. So the secret is captured to a
#    0600 file as well as printed, and the file is deleted once the participant
#    has it.
#
# 3. A RELAY RESTART IS NOT FREE, though it is cheap: it is seconds, every peer
#    reconnects on its backoff ladder and re-claims its own slot with
#    reason=reclaimed, and — until the forward receipt ships — it drops every
#    outstanding forwarded record, which falls back to the sender's bounded
#    24-hour hold instead of re-routing in seconds. BATCH THE ISSUANCE. One
#    restart for five participants, not five restarts.
#    RESTART-POLICY.md, "the relay-only restart", is the whole of it.
#
# WHAT IT NEVER DOES: mint over an existing credential, send a secret anywhere,
# touch the archive, or leave the relay stopped.
set -uo pipefail

ENV_FILE="${MV_ENV_FILE:-/etc/multiverse/deploy.env}"
[ -r "$ENV_FILE" ] || { echo "issue-join: no $ENV_FILE" >&2; exit 2; }
# shellcheck source=/dev/null
set -a; . "$ENV_FILE"; set +a

: "${MV_USER:=multiverse}"
: "${MV_GROUP:=multiverse}"
: "${MV_PREFIX:=/opt/multiverse}"
: "${MV_STATE:=/var/lib/multiverse}"
: "${MV_RELAY_PORT:=443}"

RELAY_DATA="$MV_STATE/relay"
RELAY_BIN="$MV_PREFIX/bin/multiverse-relay"
JOINS="$MV_STATE/joins"
CONTRACT_B_PATH=/contract-b/v4
ADVERTISE_URL="wss://${MV_DOMAIN}${CONTRACT_B_PATH}"
[ "$MV_RELAY_PORT" = 443 ] || ADVERTISE_URL="wss://${MV_DOMAIN}:${MV_RELAY_PORT}${CONTRACT_B_PATH}"

GRANT=peer
YES=0
CHECK=0
PEERS=()

say()  { printf '     %s\n' "$*"; }
step() { printf '\n==== %s\n' "$*"; }
warn() { printf '  !! %s\n' "$*" >&2; }
die()  { printf '\nSTOP: %s\n' "$*" >&2; exit 1; }

while [ $# -gt 0 ]; do
  case "$1" in
    --grant) GRANT="${2:?--grant needs peer, subscribe or admin}"; shift ;;
    -y|--yes) YES=1 ;;
    --check) CHECK=1 ;;
    -h|--help) sed -n '2,40p' "$0"; exit 0 ;;
    -*) die "unknown argument: $1" ;;
    *) PEERS+=("$1") ;;
  esac
  shift
done

[ "$(id -u)" = 0 ] || die "run this with sudo: it stops and starts the relay and writes as $MV_USER."
[ -x "$RELAY_BIN" ] || die "no relay binary at $RELAY_BIN"

case "$GRANT" in
  peer|subscribe|admin) ;;
  *) die "--grant must be peer, subscribe or admin. The three are DISJOINT: a
     subscribe credential cannot claim a slot and a peer credential cannot
     subscribe." ;;
esac

store="$RELAY_DATA/peers.json"
holds() {
  [ -s "$store" ] || return 1
  jq -e --arg id "$1" '.peers[]? | select(.peerId == $id)' "$store" >/dev/null 2>&1
}

# ---------------------------------------------------------------- --check

if [ "$CHECK" = 1 ]; then
  step "credentials this relay holds"
  if [ -s "$store" ]; then
    jq -r '.peers[]? | "     \(.peerId)  (\(.grant))"' "$store" 2>/dev/null || say "unreadable"
  else
    say "none — $store does not exist yet"
  fi
  step "the map"
  if [ -s "$RELAY_DATA/ring.json" ]; then
    jq -r '"     \(.width)x\(.height)"' "$RELAY_DATA/ring.json" 2>/dev/null
    jq -r '.slots[]? | "     slot \(.slot)  \(.peerId)  @\(.col),\(.row)"' "$RELAY_DATA/ring.json" 2>/dev/null
  else
    say "no ring.json yet — no slot has ever been claimed"
  fi
  step "join strings still on this box"
  find "$JOINS" -maxdepth 1 -type f -name '*.secret' 2>/dev/null | sed 's/^/     /' \
    || say "none"
  say ""
  say "Any file listed above is a participant's identity sitting on the operator's"
  say "machine. Delete each one once its owner has applied it."
  exit 0
fi

[ "${#PEERS[@]}" -gt 0 ] || die "name at least one peerId. A peerId is the participant's own identity on
     the map and it is PERMANENT: slot numbers are never reused, so a peerId
     that claims a slot owns that address forever."

# ---------------------------------------------------------------- refuse early

step "what is being asked"
todo=()
for id in "${PEERS[@]}"; do
  case "$id" in
    *[!A-Za-z0-9._-]*) die "peerId '$id' has a character outside [A-Za-z0-9._-]. It goes on the wire,
     into a file name and onto a public page." ;;
  esac
  if holds "$id"; then
    warn "$id ALREADY HOLDS A CREDENTIAL. Skipping."
    warn "    The relay cannot reprint a join string. If it was lost, the recovery is"
    warn "    a handover, which mints for a NEW peerId and drops this one:"
    warn "      $RELAY_BIN --data-dir $RELAY_DATA --handover-slot <n>=<newPeerId>"
  else
    todo+=("$id")
    say "mint  $id  (grant $GRANT)"
  fi
done
[ "${#todo[@]}" -gt 0 ] || { step "nothing to do"; exit 0; }

if [ "$GRANT" = subscribe ]; then
  warn "A SUBSCRIBE GRANT IS A DECISION, NOT A DEFAULT. Its holder is copied on every"
  warn "    payload, every ACK and NACK and every PEER_STATUS the relay routes. It sees"
  warn "    nothing a peer does not already see — there is no subscriber-only field —"
  warn "    but it sees ALL of it, for the whole map, continuously."
fi
if [ "$GRANT" = admin ]; then
  warn "AN ADMIN GRANT CAN RELEASE A SLOT, HAND ONE OVER AND EVICT A PEER. It is only"
  warn "    usable when --admin-listen is bound, which this deployment deliberately"
  warn "    leaves unset. Binding it off loopback also requires TLS."
fi

step "what it costs"
say "This restarts the relay. Every connected peer sees an ordinary disconnect,"
say "reconnects on its backoff ladder and re-claims its own slot with"
say "reason=reclaimed. Slot numbers, coordinates and reservations all survive."
say ""
say "The one real cost: every record the relay had forwarded and not yet seen"
say "acknowledged is dropped, and falls back to the SENDER'S bounded 24-hour hold"
say "rather than re-routing in seconds. Nothing is lost; some crossings are slow."
say ""
say "The archive is NOT restarted and does not replay. It re-subscribes on its own."
say ""
say "Batch: ${#todo[@]} identity(ies) in ONE restart."

if [ "$YES" != 1 ]; then
  printf '\n     Proceed? type yes: '
  read -r answer
  [ "$answer" = yes ] || die "nothing was minted"
fi

# ---------------------------------------------------------------- act

was_active=0
systemctl is-active --quiet multiverse-relay 2>/dev/null && was_active=1

if [ "$was_active" = 1 ]; then
  step "stopping the relay"
  systemctl stop multiverse-relay || die "the relay would not stop; nothing was minted"
  say "stopped at $(date -u +%FT%TZ)"
fi

# Whatever happens below, the relay goes back up.
restart_relay() {
  if [ "$was_active" = 1 ]; then
    step "starting the relay"
    systemctl start multiverse-relay \
      && say "started at $(date -u +%FT%TZ)" \
      || warn "THE RELAY DID NOT START. 'systemctl status multiverse-relay' now."
  fi
}
trap restart_relay EXIT

install -d -m 0700 -o root -g root "$JOINS"

step "minting"
minted=()
for id in "${todo[@]}"; do
  out="$(runuser -u "$MV_USER" -- "$RELAY_BIN" \
        --data-dir "$RELAY_DATA" --advertise-url "$ADVERTISE_URL" \
        --mint-credential "$id" --grant "$GRANT" 2>/dev/null)" || {
    warn "$id: the mint command failed"; continue; }
  secret="$(printf '%s\n' "$out" | awk '$1=="secret" && NF==2 {print $2; exit}')"
  if [ -z "$secret" ]; then
    warn "$id: no join string was printed; nothing was written"
    continue
  fi
  f="$JOINS/$id.secret"
  ( umask 077; printf 'multiverse-join/1 %s %s.%s\n' "$ADVERTISE_URL" "$id" "$secret" > "$f" )
  chmod 0600 "$f"
  minted+=("$id")
  say "$id -> $f (mode 0600)"
done

# The relay comes back before the secrets are printed, so the map is down for the
# mints and not for the reading.
restart_relay
trap - EXIT

[ "${#minted[@]}" -gt 0 ] || die "nothing was minted"

step "the join strings — printed ONCE"
cat <<EOF
================================================================================
Hand each line to its owner OUT OF BAND. Whoever holds one of these lines IS
that world on this map. There is no account, no email and no password reset:
the relay keeps a verifier and cannot print this again.

What the participant does with it is in docs/participant/join.md.
================================================================================
EOF
for id in "${minted[@]}"; do
  printf '\n  %s\n    %s\n' "$id" "$(cat "$JOINS/$id.secret")"
done
cat <<EOF

================================================================================
DELETE EACH FILE ONCE ITS OWNER HAS APPLIED IT — this machine has no further use
for another person's identity:

    sudo shred -u $JOINS/<peerId>.secret

AND BACK UP THE VERIFIER STORE. $store is the third durable file, beside
ring.json and the archive's set. Losing it costs a handover PER PEER:

    sudo systemctl start multiverse-backup.service
================================================================================
EOF
