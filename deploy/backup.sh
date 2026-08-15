#!/usr/bin/env bash
# Backup of the irreplaceable things — Design Question 2, third obligation.
#
#   "The relay's ring.json is the slot-to-identity binding for every participant;
#    losing it does not lose organisms but does lose everyone's address. The
#    archive's three durable files are the record itself, and D11 makes them the
#    seed of M7. Neither is reproducible from anywhere else."
#
# There are FIVE files and they are not the same kind of thing, so they are not
# backed up the same way. Pretending otherwise is how a backup plan becomes a
# 26 GB copy that will not fit on the volume it is copied from.
#
#   TIER 1 — IDENTITY, hourly, kilobytes, many generations.
#     <relay-data>/ring.json    the slot-to-identity register: which peerId holds
#                               which slot number at which coordinate.
#     <relay-data>/peers.json   the credential VERIFIERS and their grants. Never
#                               a secret — salted hashes — and losing it costs a
#                               slot handover PER PEER, which is a message to
#                               every participant and a re-mint each.
#     Together a few kilobytes. There is no excuse for these ever being lost, so
#     they get a full timestamped copy every hour and a week of history.
#
#   TIER 2 — THE RECORD, daily, tens of gigabytes, one generation.
#     <archive-data>/migrations.jsonl   the ledger. 337 B/record measured; tens of
#                                       millions of records over the announced run.
#     <archive-data>/metrics.jsonl      1.6 MB/day/slot, speed-independent.
#     <archive-data>/genomes/           content-addressed, ~14.2 KB per object.
#     Append-only and immutable-once-written, which is what makes the cheap forms
#     below correct: gzip for the two logs, a HARDLINK farm for the store.
#
#   TIER 3 — OFF-HOST, and it is not this script's. See "the off-host half".
#     A local copy survives a mistake. It does not survive the instance. Use a
#     checked provider snapshot or another approved off-host copy.
#
#   monitor.sh watches whether this ran. A backup timer that quietly stopped is
#   invisible until the day it is needed.
#
#   backup.sh                run the tier that is due
#   backup.sh --full         run tier 2 now, whatever the hour
#   backup.sh --identity     tier 1 only
#   backup.sh --list         what is held, and how big
#   backup.sh --restore-help the procedure, printed where the operator is
set -uo pipefail

ENV_FILE="${MV_ENV_FILE:-/etc/multiverse/deploy.env}"
# MISSING AND UNREADABLE ARE DIFFERENT FAULTS and this process runs unprivileged,
# so say which one it is: a file installed root:root 0640 is present and
# unreadable, this exits 2 on every tick of the timer, and the only symptom is
# that there are no identity snapshots on the day one is needed.
if [ ! -e "$ENV_FILE" ]; then
  echo "backup: no $ENV_FILE" >&2
  exit 2
elif [ ! -r "$ENV_FILE" ]; then
  echo "backup: $ENV_FILE exists but is NOT READABLE by $(id -un). It must be" >&2
  echo "        0640 root:${MV_GROUP:-multiverse}:" >&2
  echo "        sudo chown root:${MV_GROUP:-multiverse} $ENV_FILE && sudo chmod 0640 $ENV_FILE" >&2
  exit 2
fi
# shellcheck source=/dev/null
set -a; . "$ENV_FILE"; set +a

: "${MV_STATE:=/var/lib/multiverse}"
: "${MV_BACKUP_KEEP:=168}"
: "${MV_BACKUP_LEDGER:=auto}"
: "${MV_BACKUP_LEDGER_MIN_FREE_PCT:=30}"
: "${MV_BACKUP_GENOMES:=1}"
: "${MV_BACKUP_DAILY_HOUR:=4}"

RELAY_DATA="$MV_STATE/relay"
ARCHIVE_DATA="$MV_STATE/archive"
DEST="$MV_STATE/backup"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
MODE=due

say()  { printf '     %s\n' "$*"; }
step() { printf '\n==== %s\n' "$*"; }
warn() { printf '  !! %s\n' "$*" >&2; }

free_pct() { df -P "$DEST" 2>/dev/null | awk 'NR==2 {print 100 - ($5+0)}'; }
human()    { numfmt --to=iec --suffix=B "${1:-0}" 2>/dev/null || echo "${1:-0}B"; }

restore_help() {
cat <<'EOF'

RESTORING — read the whole of the relevant section before typing anything.

  Nothing here is a one-command restore, on purpose. Each of the five files
  fails differently and three of them have consequences a participant sees.

1. ring.json — the addresses
   Stop the relay, put the file back, start it.

     systemctl stop multiverse-relay
     cp -a /var/lib/multiverse/backup/identity/<stamp>/ring.json \
           /var/lib/multiverse/relay/ring.json
     chown multiverse:multiverse /var/lib/multiverse/relay/ring.json
     systemctl start multiverse-relay

   WHAT IT COSTS: every slot claimed AFTER that snapshot is gone from the map.
   Those peers re-claim on their next connection and D8/D12 never reuse a slot
   number, so they come back on NEW addresses while their sidecars still hold
   the old ones. Expect re-claims with reason=reclaimed for everyone in the
   snapshot and a fresh placement for everyone after it. Restore the NEWEST
   snapshot, always.

2. peers.json — the credentials
   Same procedure, same directory. Restore it WITH the matching ring.json from
   the SAME snapshot directory, never one from each.

     WHAT IT COSTS: every credential minted after the snapshot stops verifying.
     Those peers get HTTP 401 at the door. The relay keeps verifiers and cannot
     reprint a join string, so the only recovery for each is a handover:

       multiverse-relay --handover-slot <n>=<newPeerId> --data-dir <relay-data>

     which mints a fresh credential for a NEW peerId and drops the old one —
     and the participant has to re-apply the new join string by hand. This is
     why tier 1 runs hourly and keeps a week.

   If BOTH files are lost with no backup at all, the map is gone as an address
   space: every participant needs a new join string and a new slot, and their
   journals hold entries addressed to slots that no longer mean the same thing.

3. migrations.jsonl — the ledger
   Stop the archive, decompress in place, start it.

     systemctl stop multiverse-archive
     gunzip -c /var/lib/multiverse/backup/record/<stamp>/migrations.jsonl.gz \
       > /var/lib/multiverse/archive/migrations.jsonl
     chown multiverse:multiverse /var/lib/multiverse/archive/migrations.jsonl
     systemctl start multiverse-archive

   WHAT IT COSTS: every crossing between the snapshot and now is gone and NOT
   recoverable — no peer and no relay holds a copy of the archive's record. The
   archive re-subscribes and starts recording again from that moment; the hole
   stays a hole. Expect a replay: see RESTART-POLICY.md for how long.

   A ledger that is DAMAGED rather than lost is a different case and usually a
   better one: the reader skips a line that does not parse and reports how many
   it skipped, and an unfinished tail record was never durable and is ignored.
   Run `multiverse-archive list --data-dir <archive-data> >/dev/null` and read
   what it says on stderr BEFORE deciding to restore over it.

4. genomes/ — the store
   The hardlink snapshot is a directory of the same inodes, so a restore is a
   copy back:

     systemctl stop multiverse-archive
     cp -al /var/lib/multiverse/backup/genomes/<stamp>/. \
            /var/lib/multiverse/archive/genomes/
     systemctl start multiverse-archive

   It is content-addressed, so merging an older snapshot into a newer store is
   safe: a hash either matches its content or it does not exist. Missing objects
   show up in `genomeGaps` and the archive re-requests what a live peer can
   still serve — but a peer that has LEFT takes its unfetched genomes with it
   permanently (Risk 7).

5. metrics.jsonl — the history
   Same as the ledger, and the least costly of the five: it is a sampled series,
   nothing depends on it for correctness, and a gap in it is a gap in a graph.

RESTORING THE WHOLE INSTANCE, which the local copies do not cover:
   create a replacement instance from the checked off-host backup, attach the
   same stable address, and run provision.sh again. The participant service name
   must not change during this recovery.
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --full) MODE=full ;;
    --identity) MODE=identity ;;
    --list) MODE=list ;;
    --restore-help) restore_help; exit 0 ;;
    -h|--help) sed -n '2,50p' "$0"; exit 0 ;;
    *) echo "backup: unknown argument $1" >&2; exit 2 ;;
  esac
  shift
done

if [ "$MODE" = list ]; then
  step "what is held in $DEST"
  du -sh "$DEST"/* 2>/dev/null | sed 's/^/     /'
  say ""
  say "identity snapshots: $(find "$DEST/identity" -maxdepth 1 -mindepth 1 -type d 2>/dev/null | wc -l) (keeping $MV_BACKUP_KEEP)"
  say "newest record copy: $(find "$DEST/record" -maxdepth 1 -mindepth 1 -type d 2>/dev/null | sort | tail -1 || echo none)"
  say "free on this filesystem: $(free_pct)%"
  exit 0
fi

mkdir -p "$DEST/identity" "$DEST/record" "$DEST/genomes" 2>/dev/null

# ---------------------------------------------------------------- tier 1

backup_identity() {
  step "tier 1 — the identity files"
  local out="$DEST/identity/$STAMP" f n=0
  mkdir -p "$out" || { warn "cannot create $out"; return 1; }
  for f in ring.json peers.json; do
    if [ -f "$RELAY_DATA/$f" ]; then
      # cp -p then rename: the relay writes these by atomic rename itself, so a
      # plain copy can never catch a half-written one, but the destination
      # should still appear whole.
      cp -p "$RELAY_DATA/$f" "$out/$f.part" && mv -f "$out/$f.part" "$out/$f" && n=$((n + 1))
    else
      say "absent: $RELAY_DATA/$f"
    fi
  done
  ( cd "$out" && sha256sum ./*.json 2>/dev/null > SHA256SUMS ) || true
  say "$n file(s) -> $out"
  [ -s "$out/SHA256SUMS" ] && sed 's/^/       /' "$out/SHA256SUMS"

  # Prune. Sorting by name is sorting by time: the stamp is ISO-8601 UTC.
  local keep="$MV_BACKUP_KEEP" old
  old="$(find "$DEST/identity" -maxdepth 1 -mindepth 1 -type d | sort | head -n -"$keep")"
  if [ -n "$old" ]; then
    printf '%s\n' "$old" | while IFS= read -r d; do rm -rf "$d"; done
    say "pruned $(printf '%s\n' "$old" | wc -l) old snapshot(s), keeping $keep"
  fi
}

# ---------------------------------------------------------------- tier 2

backup_record() {
  step "tier 2 — the record"
  local pct; pct="$(free_pct)"
  local out="$DEST/record/$STAMP"

  case "$MV_BACKUP_LEDGER" in
    never)
      say "MV_BACKUP_LEDGER=never — the ledger's only protection is the offsite half."
      ;;
    auto)
      if [ "${pct:-0}" -lt "$MV_BACKUP_LEDGER_MIN_FREE_PCT" ] 2>/dev/null; then
        say "MV_BACKUP_LEDGER=auto and free space is ${pct}% (< ${MV_BACKUP_LEDGER_MIN_FREE_PCT}%)."
        say "SKIPPING the ledger copy: a backup must not fill the volume it protects."
        say "The approved off-host backup must cover this case."
        MV_BACKUP_LEDGER=skipped
      fi
      ;;
  esac

  if [ "$MV_BACKUP_LEDGER" = auto ] || [ "$MV_BACKUP_LEDGER" = always ]; then
    mkdir -p "$out"
    local f
    for f in migrations.jsonl metrics.jsonl; do
      [ -f "$ARCHIVE_DATA/$f" ] || continue
      local src_bytes; src_bytes="$(stat -c %s "$ARCHIVE_DATA/$f" 2>/dev/null || echo 0)"
      say "compressing $f ($(human "$src_bytes")) — append-only, so a copy taken while it grows is a valid PREFIX"
      # gzip -1: the ledger is JSON lines and compresses well at any level; CPU
      # is the scarce thing on a two-vCPU box shared with a live map.
      if gzip -1 -c "$ARCHIVE_DATA/$f" > "$out/$f.gz.part" 2>/dev/null; then
        mv -f "$out/$f.gz.part" "$out/$f.gz"
        say "  -> $out/$f.gz ($(human "$(stat -c %s "$out/$f.gz" 2>/dev/null || echo 0)"))"
      else
        rm -f "$out/$f.gz.part"
        warn "compressing $f failed — most likely out of space. Nothing was left behind."
      fi
    done
    # Exactly one generation. Two would double the largest thing on the disk.
    find "$DEST/record" -maxdepth 1 -mindepth 1 -type d ! -name "$STAMP" -exec rm -rf {} + 2>/dev/null
  fi

  if [ "$MV_BACKUP_GENOMES" = 1 ] && [ -d "$ARCHIVE_DATA/genomes" ]; then
    local gout="$DEST/genomes/$STAMP"
    say "hardlink snapshot of the genome store — the objects are content-addressed and"
    say "immutable once written, so this costs inodes and almost no bytes"
    if cp -al "$ARCHIVE_DATA/genomes" "$gout" 2>/dev/null; then
      say "  -> $gout"
      find "$DEST/genomes" -maxdepth 1 -mindepth 1 -type d ! -name "$STAMP" -exec rm -rf {} + 2>/dev/null
    else
      rm -rf "$gout"
      warn "the hardlink snapshot failed (out of inodes, or a cross-device destination)."
      warn "    It protects against deletion, not against losing the disk. Set"
      warn "    MV_BACKUP_GENOMES=0 if this keeps failing; the offsite half is what matters."
    fi
  fi

  # The manifest is what a restore is planned from, and it is cheap enough to
  # write even when every copy above was skipped.
  {
    printf 'stamp            %s\n' "$STAMP"
    printf 'retention rule   %s\n' "${MV_RETENTION:-UNSET}"
    printf 'free percent     %s\n' "$(free_pct)"
    printf 'ring.json        %s bytes\n' "$(stat -c %s "$RELAY_DATA/ring.json" 2>/dev/null || echo absent)"
    printf 'peers.json       %s bytes\n' "$(stat -c %s "$RELAY_DATA/peers.json" 2>/dev/null || echo absent)"
    printf 'migrations.jsonl %s bytes\n' "$(stat -c %s "$ARCHIVE_DATA/migrations.jsonl" 2>/dev/null || echo absent)"
    printf 'metrics.jsonl    %s bytes\n' "$(stat -c %s "$ARCHIVE_DATA/metrics.jsonl" 2>/dev/null || echo absent)"
    printf 'genome objects   %s\n' "$(find "$ARCHIVE_DATA/genomes" -type f 2>/dev/null | wc -l)"
    printf 'ledger copy      %s\n' "$MV_BACKUP_LEDGER"
    printf 'genome snapshot  %s\n' "$MV_BACKUP_GENOMES"
  } > "$DEST/MANIFEST.txt.part" && mv -f "$DEST/MANIFEST.txt.part" "$DEST/MANIFEST.txt"
  say "manifest -> $DEST/MANIFEST.txt"
}

# ---------------------------------------------------------------- run

backup_identity

case "$MODE" in
  full) backup_record ;;
  identity) : ;;
  due)
    hour="$(date -u +%H)"; hour="${hour#0}"; : "${hour:=0}"
    today="$(date -u +%F)"
    marker="$DEST/.record-day"
    if [ "$hour" = "$MV_BACKUP_DAILY_HOUR" ] && [ "$(cat "$marker" 2>/dev/null)" != "$today" ]; then
      printf '%s' "$today" > "$marker"
      backup_record
    else
      say "tier 2 is due at ${MV_BACKUP_DAILY_HOUR}:00 UTC; last run $(cat "$marker" 2>/dev/null || echo never)"
    fi ;;
esac

cat <<EOF

THE OFF-HOST HALF, which this script cannot do and which is the one that matters.

  Everything above is on the same disk as the thing it protects. It survives a
  mistake — a bad restore, a wrong rm, a corrupted write — and it does not
  survive the instance. The ledger and the genome store are only genuinely
  protected by a checked provider snapshot or another approved off-host copy.

  Configure its schedule and retention in the provider or backup service.
  Record the policy, owner, and latest recovery check in private operations storage.

  Check current storage and transfer prices before you select the policy.
  This repository does not contain a current quote or deployment forecast.

  TAKE ONE BY HAND BEFORE ANY OF THESE: a binary upgrade, a retention-rule
  change, a restore, and the wind-down. WIND-DOWN.md requires a final checked
  backup before resource retirement.
EOF
