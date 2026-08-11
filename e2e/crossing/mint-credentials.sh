#!/usr/bin/env bash
# Mint the M4 LAN rig's per-peer credentials against its EXISTING ring.json, and
# capture each secret into a file only its owner can read (contract-b-m4.md §22,
# B22).
#
# WHY THIS EXISTS AND WHY THE RIG MAY NOT DO IT INLINE. `--reserve-slot` folded
# issuance into reservation: a peer that already holds a slot keeps it, and a peer
# that holds no CREDENTIAL gets one minted and its join string printed ONCE, on
# stdout. The relay keeps a verifier and cannot print it again. A rig that piped
# that stdout through `sed` into a log would put six unrecoverable secrets in a
# terminal scrollback and nowhere else — and the only recovery from a lost one is
# --handover-slot, which changes the peerId a far machine has persisted. So the
# capture belongs in one tool, and this is it.
#
# WHAT IT PRESERVES. Nothing about the map moves. ReserveSlot returns the existing
# reservation untouched when the peerId already holds a slot — the @<col>,<row>
# suffix is not even consulted — so the six live slot claims, their numbers and
# their coordinates survive byte for byte. Rehearsed 2026-08-11 against a COPY of
# e2e/relay-data-m4-lan/ring.json: same sha256 before and after, same mtime, six
# "this peer already holds a slot; left alone" lines and seven credentials minted.
#
# WHEN TO RUN IT. Before the window, while the OLD relay is still serving. The
# running contract-b/3.5 relay never opens peers.json and never re-writes ring.json
# at startup, so this is additive and an abandoned crossing leaves nothing to undo.
#
#   e2e/crossing/mint-credentials.sh            # mint what is missing
#   e2e/crossing/mint-credentials.sh --check    # report, mint nothing
#
# WHAT IT NEVER DOES: print a peer secret for a LOCAL peer (it goes straight to a
# 0600 file), mint a second credential over an existing one, or touch a running
# process.
set -uo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
E2E="$(cd "$HERE/.." && pwd)"
ROOT="$(cd "$E2E/.." && pwd)"

# RELAY_BIN MUST BE A contract-b/4.0 RELAY, AND THE DEFAULT USUALLY IS NOT ONE AT
# THE MOMENT THIS RUNS. At RUNBOOK P0.5 the crossing has not renamed anything yet
# (that is P3), so $ROOT/bin/relay is still the OLD binary, which has no
# --mint-credential, no --grant and no --advertise-url. Point this at the crossing
# build instead:
#
#     RELAY_BIN=/mnt/wsl/data/crossing-build/relay e2e/crossing/mint-credentials.sh
#
# The default is kept as bin/relay because that is correct everywhere else — after
# the crossing, and on any rig that is already at 4.0. The capability check below
# is what makes the wrong one loud instead of silent.
RELAY_BIN="${RELAY_BIN:-$ROOT/bin/relay}"
RELAY_DATA="${RELAY_DATA:-$E2E/relay-data-m4-lan}"
SECRETS_DIR="${SECRETS_DIR:-$HOME/.multiverse}"
TLS_DIR="${TLS_DIR:-$E2E/tls-m4-lan}"
RELAY_PORT="${RELAY_PORT:-8795}"
CONTRACT_B_PATH="${CONTRACT_B_PATH:-/contract-b/v4}"

# The 3x2 map exactly as e2e/relay-data-m4-lan/ring.json already holds it. The
# peerIds are slot-1 .. slot-6 — run-m4.sh's peer_of() is `slot-<n>`, NOT any
# `peer-lan-*` form.
MAP_PEERS="${MAP_PEERS:-slot-1@0,0 slot-2@1,0 slot-3@2,0 slot-4@0,1 slot-5@1,1 slot-6@2,1}"
# The one peer that is not on this machine. Its join string is PRINTED, because a
# person has to carry it to the second computer (D9: nothing here may send it).
#
# `-` and not `:-`: an EXPLICITLY EMPTY FAR_PEER means "every peer is local", which
# is what the single-machine rehearsal (run-m4.sh, where slot 6 is bin/fakemod)
# passes. With `:-` an empty value would have silently reinstated slot-6 as remote
# and left the rehearsal's sixth secret unfiled.
FAR_PEER="${FAR_PEER-slot-6}"
LAN_RELAY_HOST="${LAN_RELAY_HOST:-192.168.1.227}"
FAR_URL="${FAR_URL:-wss://$LAN_RELAY_HOST:$RELAY_PORT$CONTRACT_B_PATH}"
LOCAL_URL="${LOCAL_URL:-wss://127.0.0.1:$RELAY_PORT$CONTRACT_B_PATH}"
# The archive's own identity. Its grant is `subscribe`, which is DISJOINT from
# `peer`: it cannot claim a slot and no peer credential can subscribe (B27).
SUBSCRIBER="${SUBSCRIBER:-archive-main}"

check_only=0
[ "${1:-}" = "--check" ] && check_only=1

say()  { printf '     %s\n' "$*"; }
step() { printf '\n==== %s\n' "$*"; }
warn() { printf '  !! %s\n' "$*" >&2; }
die()  { printf '\nSTOP: %s\n' "$*" >&2; exit 1; }

[ -x "$RELAY_BIN" ] || die "no relay binary at $RELAY_BIN"
[ -d "$RELAY_DATA" ] || die "no relay data dir at $RELAY_DATA"

# A PRE-4.0 RELAY FAILS EVERY MINT SILENTLY, so refuse before writing anything.
# Measured in the 2026-08-11 window: every mint against the old binary died with
# `flag provided but not defined: -advertise-url`, capture() discards stderr, and
# the only symptom was seven "no join string was printed" warnings and an empty
# peers.json. One --help probe turns that into one sentence.
if ! "$RELAY_BIN" --help 2>&1 | grep -q -- '-mint-credential'; then
  die "$RELAY_BIN does not understand --mint-credential, so it is a pre-4.0 relay.
     Every mint would fail with 'flag provided but not defined' and this script
     would report seven missing join strings instead.

     At RUNBOOK P0.5, bin/relay is still the OLD binary — P3 is what renames the
     new one over it. Point RELAY_BIN at the crossing build:

         RELAY_BIN=/mnt/wsl/data/crossing-build/relay $0 ${1:-}"
fi

peer_secret_file() { printf '%s/peer-%s.secret\n' "$SECRETS_DIR" "$1"; }
sub_secret_file()  { printf '%s/%s.secret\n' "$SECRETS_DIR" "$1"; }

umask 077
mkdir -p "$SECRETS_DIR"
chmod 700 "$SECRETS_DIR"

# ---------------------------------------------------------------- the state now

store="$RELAY_DATA/peers.json"
held() {
  # Whether the store already holds a credential for $1. A credential that exists
  # with no secret file beside it is the one unrecoverable state, and it is
  # reported rather than papered over.
  [ -s "$store" ] || return 1
  python3 - "$store" "$1" <<'PY'
import json, sys
try:
    d = json.load(open(sys.argv[1]))
except Exception:
    sys.exit(1)
sys.exit(0 if any(p.get("peerId") == sys.argv[2] for p in d.get("peers", [])) else 1)
PY
}

step "what this relay data dir holds now"
say "ring.json   $(python3 -c "
import json;d=json.load(open('$RELAY_DATA/ring.json'))
print('%dx%d,' % (d['width'], d['height']), ', '.join('%d=%s@%d,%d' % (s['slot'], s['peerId'], s['col'], s['row']) for s in d['slots']))")"
if [ -s "$store" ]; then
  say "peers.json  $(python3 -c "
import json;d=json.load(open('$store'));print(', '.join('%s(%s)' % (p['peerId'], p['grant']) for p in d['peers']))")"
else
  say "peers.json  <absent — no credential has been minted yet>"
fi

ring_before="$(sha256sum "$RELAY_DATA/ring.json" | awk '{print $1}')"

# ---------------------------------------------------------------- the gap report

missing=()
stranded=()
for spec in $MAP_PEERS; do
  id="${spec%@*}"
  f="$(peer_secret_file "$id")"
  if held "$id"; then
    if [ "$id" = "$FAR_PEER" ]; then
      # The far peer's secret does not belong on this machine at all. Its file, if
      # it is still here, is the handoff copy — which the runbook says to delete
      # once the second computer has applied it.
      if [ -s "$SECRETS_DIR/$id-handoff.secret" ]; then
        say "have    $id  credential; the handoff copy is STILL HERE — delete it once"
        say "        the second computer has applied it"
      else
        say "have    $id  credential; its secret is the far end's and is not kept here"
      fi
    elif [ -s "$f" ]; then
      say "have    $id  credential + secret file"
    else
      stranded+=("$id")
    fi
  else
    missing+=("$spec")
  fi
done
if held "$SUBSCRIBER"; then
  [ -s "$(sub_secret_file "$SUBSCRIBER")" ] || stranded+=("$SUBSCRIBER")
  [ -s "$(sub_secret_file "$SUBSCRIBER")" ] && say "have    $SUBSCRIBER  credential + secret file"
fi

if [ "${#stranded[@]}" -gt 0 ]; then
  warn "these identities hold a credential in $store with NO secret file on this"
  warn "machine: ${stranded[*]}"
  warn "The relay keeps a verifier and cannot reprint a join string. The only"
  warn "recovery is  multiverse-relay --handover-slot <n>=<newPeerId>,  which mints"
  warn "a fresh credential for a NEW peerId and drops the old one (§7.5, B22)."
  warn "Decide that deliberately; this script will not do it."
fi

if [ "$check_only" = 1 ]; then
  step "--check: nothing was minted"
  say "${#missing[@]} identity(ies) still need a credential"
  exit 0
fi

# ---------------------------------------------------------------- refuse to race

# --reserve-slot and --mint-credential are STARTUP commands: they open the data
# dir, act and exit. Running one against a data dir a serving relay owns is safe
# for ring.json (the reserve path never re-writes it when every peer already holds
# a slot) but a second WRITER of peers.json is not a race worth taking during a
# window, and a mint while the NEW relay serves would be one.
if ss -ltn 2>/dev/null | grep -qE ":$RELAY_PORT " && [ "${ALLOW_LIVE_RELAY:-0}" != 1 ]; then
  cat >&2 <<EOF

A relay is listening on $RELAY_PORT.

  Minting against the data dir of the CURRENTLY RUNNING contract-b/3.5 relay is
  the intended pre-window act: that relay never opens peers.json and never
  re-writes ring.json, so this is additive and an abandoned crossing leaves
  nothing to undo. Rehearsed 2026-08-11; ring.json came back byte-identical.

  Minting against a SERVING contract-b/4.0 relay is different — that one holds
  the store open and answers upgrades from it.

  If the relay on $RELAY_PORT is the old one, re-run with:

      ALLOW_LIVE_RELAY=1 $0

EOF
  exit 1
fi

# ---------------------------------------------------------------- mint

# capture <peerId> <destination file> <relay args...>
# Runs one relay startup command, writes the printed secret to the destination
# with mode 0600, and never lets the secret reach a log or the terminal.
capture() {
  local id="$1" dest="$2"; shift 2
  local out secret
  out="$("$RELAY_BIN" "$@" 2>/dev/null)" || { warn "$id: the relay command failed"; return 1; }
  secret="$(printf '%s\n' "$out" | awk '$1=="secret" && NF==2 {print $2; exit}')"
  if [ -z "$secret" ]; then
    warn "$id: no join string was printed. It already holds a credential, or the"
    warn "    command did not mint. Nothing was written to $dest."
    return 1
  fi
  ( umask 077; printf '%s\n' "$secret" > "$dest" )
  chmod 600 "$dest"
  say "$id -> $dest (mode 600)"
  return 0
}

# The URLs the join strings name are given explicitly with --advertise-url, so a
# missing certificate cannot make one say ws://. This is only a pre-flight: a
# credential minted for a relay that has no certificate is a credential nobody can
# use, and finding that out here is cheaper than finding it out in the window.
if [ ! -s "$TLS_DIR/relay.crt" ] || [ ! -s "$TLS_DIR/relay.key" ]; then
  warn "no TLS material in $TLS_DIR. The credentials below will be correct, but the"
  warn "    relay cannot serve $LOCAL_URL until e2e/crossing/mint-tls.sh has run."
fi

far_report=""
if [ "${#missing[@]}" -gt 0 ]; then
  step "minting ${#missing[@]} peer credential(s) against the existing ring.json"
fi
for spec in "${missing[@]:-}"; do
  [ -n "$spec" ] || continue
  id="${spec%@*}"
  if [ "$id" = "$FAR_PEER" ]; then
    # The far end's join string has to be READ by a person and carried to the
    # second computer, so it is printed in full — and also written to a file, so a
    # closed terminal is not a lost identity.
    far_report="$("$RELAY_BIN" --data-dir "$RELAY_DATA" \
      --advertise-url "$FAR_URL" --reserve-slot "$spec" 2>/dev/null)"
    secret="$(printf '%s\n' "$far_report" | awk '$1=="secret" && NF==2 {print $2; exit}')"
    if [ -z "$secret" ]; then
      warn "$id: no join string was printed"
      continue
    fi
    ( umask 077; printf '%s\n' "$secret" > "$SECRETS_DIR/$id-handoff.secret" )
    chmod 600 "$SECRETS_DIR/$id-handoff.secret"
    say "$id -> $SECRETS_DIR/$id-handoff.secret (mode 600) AND printed below"
  else
    capture "$id" "$(peer_secret_file "$id")" \
      --data-dir "$RELAY_DATA" --advertise-url "$LOCAL_URL" \
      --reserve-slot "$spec"
  fi
done

if ! held "$SUBSCRIBER"; then
  step "minting the archive's SUBSCRIBE credential"
  say "It is a decision, not a default (B27): its holder sees every payload, every"
  say "ACK/NACK and every PEER_STATUS the relay routes. On this rig the holder is"
  say "this machine's own archive, which is where that traffic already lands."
  capture "$SUBSCRIBER" "$(sub_secret_file "$SUBSCRIBER")" \
    --data-dir "$RELAY_DATA" --advertise-url "$LOCAL_URL" \
    --mint-credential "$SUBSCRIBER" --grant subscribe
fi

# ---------------------------------------------------------------- prove the map

ring_after="$(sha256sum "$RELAY_DATA/ring.json" | awk '{print $1}')"
step "the map after"
if [ "$ring_before" = "$ring_after" ]; then
  say "ring.json is BYTE-IDENTICAL ($ring_after)"
  say "every slot number and every coordinate survived; only peers.json was written"
else
  warn "ring.json CHANGED: $ring_before -> $ring_after"
  warn "That is not what a slot-preserving mint does. Stop and read it before"
  warn "starting the new relay."
fi
python3 -c "
import json;d=json.load(open('$RELAY_DATA/ring.json'))
print('     %dx%d: ' % (d['width'], d['height']) + ', '.join('%d=%s@%d,%d' % (s['slot'], s['peerId'], s['col'], s['row']) for s in d['slots']))"

if [ -n "$far_report" ]; then
  cat <<EOF

================================================================================
THE FAR END'S JOIN STRING — printed ONCE. Carry it to the second computer by
hand; nothing on this machine may send it (D9).
================================================================================
$far_report
It is also in $SECRETS_DIR/$FAR_PEER-handoff.secret (mode 600).
DELETE that file once the second computer has applied it: this machine has no
further use for another machine's identity.
================================================================================
EOF
fi

step "done"
say "peer secrets   $SECRETS_DIR/peer-slot-N.secret   (the sidecars' --credential-file)"
say "archive secret $(sub_secret_file "$SUBSCRIBER")   (the archive's --credential-file)"
say "verifiers      $store   — BACK THIS UP. It is the third durable file, beside"
say "               ring.json and the archive's set (D24, DQ2)."
