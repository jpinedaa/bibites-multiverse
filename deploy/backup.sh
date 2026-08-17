#!/usr/bin/env bash
# Backup of the irreplaceable things — Design Question 2, third obligation.
#
#   "The relay's ring.json is the slot-to-identity binding for every participant;
#    losing it does not lose organisms but does lose everyone's address. The
#    archive's three durable files are the record itself, and D11 makes them the
#    seed of M7. Neither is reproducible from anywhere else."
#
# The things here are not the same kind of thing, so they are not backed up the
# same way. Pretending otherwise is how a backup plan becomes a 26 GB copy that
# will not fit on the volume it is copied from.
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
#   TIER 2 — WHAT IS STILL BEING WRITTEN, daily, one generation.
#     Everything in this tier is a file the archive is APPENDING to. That is what
#     makes the cheap forms below correct — a copy taken while an append-only
#     file grows is a valid PREFIX — and it is also what decides what belongs
#     here: a file that is finished belongs in tier 3, copied once, not copied
#     again every night.
#
#     <archive-data>/rollup.jsonl       THE ROLL-UP STATE SIDECAR, and the thing
#                                       this tier gained. It is the archive's own
#                                       fold — species, lanes, ancestry, coverage
#                                       buckets, the genome-gap queue and the
#                                       cursor into the raw record — about 6.5 MB
#                                       on a real map. Losing it loses NO answer:
#                                       every one of them is re-derivable from the
#                                       raw lines. What it loses is TIME, and a
#                                       lot of it: the next start replays the
#                                       whole raw record instead of one duplicate
#                                       window, and that replay is a held-down
#                                       relay and a participant outage. It is
#                                       small, so it is copied.
#     <archive-data>/brains.jsonl       the brain-history sidecar, same shape and
#                                       the same reasoning.
#     <archive-data>/migrations.jsonl   the LIVE ledger segment — at most one UTC
#                                       day of records. 338 B/record measured.
#                                       BEFORE THE ROLL-UP THIS WAS THE WHOLE
#                                       LEDGER and it was the largest thing here;
#                                       it is now one day, and this tier shrank by
#                                       an order of magnitude with it.
#     <archive-data>/metrics.jsonl      1.6 MB/day/slot, speed-independent.
#     <archive-data>/genomes/           content-addressed, ~14.2 KB per object.
#     gzip for the logs, a HARDLINK farm for the store.
#
#   TIER 3 — OFF-HOST, and half of it is now in this kit.
#     A local copy survives a mistake. It does not survive the instance.
#
#     <archive-data>/segments/  THE CLOSED LEDGER SEGMENTS. This script does not
#                               copy them and must not: each one is immutable,
#                               already compressed by the archive, and a second
#                               local copy would double the largest thing on the
#                               disk for no protection this tier does not already
#                               give. deploy/coldcopy.sh copies each one ONCE to
#                               an object store, verifies it by reading the
#                               object's checksum back out of the store, and
#                               writes a receipt beside it.
#                               THAT RECEIPT IS A GATE AND NOT A NOTE: the
#                               archive removes no segment from this host until a
#                               receipt confirms a copy that matches these exact
#                               bytes. No receipt, no removal, forever if need
#                               be — so a cold archive that has stopped working
#                               shows up as ledgerSegmentsAwaitingColdCopy
#                               climbing and as a disk filling up, and never as a
#                               record that is gone.
#     EVERYTHING ELSE           ring.json, peers.json, rollup.jsonl,
#                               brains.jsonl, metrics.jsonl and the genome store
#                               are NOT covered by coldcopy.sh. Their only
#                               off-host copy is a checked provider snapshot or
#                               another approved off-host backup, and that is
#                               still the operator's to configure.
#
#   WHAT THIS SCRIPT DOES AND DOES NOT DO, in one place, because the three tiers
#   above are easy to read as three copies of everything:
#     it DOES     copy the identity pair hourly, with a week of history;
#     it DOES     copy the appending archive files once a day, one generation;
#     it DOES NOT copy a closed ledger segment, ever — that is coldcopy.sh's;
#     it DOES NOT put anything off this host, ever;
#     it DOES NOT delete anything the archive owns.
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
   THE LEDGER IS A RUN OF FILES, not one file. The archive reads
   <archive-data>/segments/ in day order and then <archive-data>/migrations.jsonl,
   so a restore has to think about both halves:

     the LIVE file      at most one UTC day of records. This is what the copy
                        below holds, and it is the half that can be lost.
     the SEGMENTS       closed, compressed and immutable. They are not in this
                        backup. A missing one is fetched back with
                        `deploy/coldcopy.sh --restore <name>.jsonl.gz`, which
                        verifies the digest against the receipt before it puts
                        the file into place.

   Restoring the live file:

     systemctl stop multiverse-archive
     gunzip -c /var/lib/multiverse/backup/record/<stamp>/migrations.jsonl.gz \
       > /var/lib/multiverse/archive/migrations.jsonl
     chown multiverse:multiverse /var/lib/multiverse/archive/migrations.jsonl
     systemctl start multiverse-archive

   A copy taken BEFORE a rotation holds records that are now in a segment, so
   restoring it over a live file that has already rotated would replay those
   records twice. Check what the copy's last line says and what
   `ls /var/lib/multiverse/archive/segments/` holds before you type this.

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
   Same as the ledger, and the least costly: it is a sampled series, nothing
   depends on it for correctness, and a gap in it is a gap in a graph.

6. rollup.jsonl and brains.jsonl — the state sidecars
   THESE ARE THE ONE CASE WHERE DOING NOTHING IS A VALID RECOVERY. Every value
   in them is re-derivable from the raw crossing lines, so a lost or damaged
   sidecar costs REPLAY TIME and no answer at all. The archive already handles
   the damaged case by itself: an unreadable file is kept aside as
   <name>.unreadable, reported, and rebuilt by a full replay.

     systemctl stop multiverse-archive
     gunzip -c /var/lib/multiverse/backup/record/<stamp>/rollup.jsonl.gz \
       > /var/lib/multiverse/archive/rollup.jsonl
     chown multiverse:multiverse /var/lib/multiverse/archive/rollup.jsonl
     systemctl start multiverse-archive

   RESTORE ONE ONLY TO SAVE THE REPLAY, and only from a copy of THIS archive's
   own data directory. An older sidecar is not wrong — the archive folds the raw
   records after its cursor on the next start and arrives at the same answers —
   but a sidecar whose cursor names a raw segment that has since been retired
   off this host leaves a hole it will say out loud (replayFromRetired on
   /api/status) and cannot close by itself. If in doubt, restore neither: delete
   them and let the archive rebuild, and pay one full replay for certainty.

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
    -h|--help) sed -n '2,/^set -uo pipefail$/p' "$0" | sed '$d'; exit 0 ;;
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
  step "tier 2 — the archive files that are still being appended to"
  local pct; pct="$(free_pct)"
  local out="$DEST/record/$STAMP"

  case "$MV_BACKUP_LEDGER" in
    never)
      say "MV_BACKUP_LEDGER=never — the ledger's only protection is the offsite half."
      ;;
    auto)
      if [ "${pct:-0}" -lt "$MV_BACKUP_LEDGER_MIN_FREE_PCT" ] 2>/dev/null; then
        say "MV_BACKUP_LEDGER=auto and free space is ${pct}% (< ${MV_BACKUP_LEDGER_MIN_FREE_PCT}%)."
        say "SKIPPING the live-file and metrics copy: a backup must not fill the volume"
        say "it protects. The two state sidecars are copied anyway — they are about ten"
        say "megabytes and losing them costs a full replay. The approved off-host backup"
        say "must cover the rest."
        MV_BACKUP_LEDGER=skipped
      fi
      ;;
  esac

  # THE TWO SIDECARS ARE COPIED WHATEVER THE SPACE RULE SAYS, and that is a
  # deliberate exception to the skip above.
  #
  # The rule exists because a backup must not fill the volume it protects, and
  # it was written when this tier's largest item was the WHOLE ledger — 0.22 GB
  # then and 16 to 21 GB by the end of the announced run. The roll-up moved that
  # item out of this tier: the live file is now one UTC day. What arrived in its
  # place is about 6.5 MB of state sidecar plus the brain sidecar, and skipping
  # ten megabytes to save a volume from a copy that was never going to fill it is
  # the wrong trade in both directions — it protects nothing and it costs the one
  # file whose loss turns the next restart into a full replay.
  #
  # So: the sidecars are copied on every tier-2 run, and the space rule governs
  # the two append-only logs, which are the only items here that can be large.
  mkdir -p "$out"
  local sc
  for sc in rollup.jsonl brains.jsonl; do
    [ -f "$ARCHIVE_DATA/$sc" ] || continue
    local sc_bytes; sc_bytes="$(stat -c %s "$ARCHIVE_DATA/$sc" 2>/dev/null || echo 0)"
    if gzip -1 -c "$ARCHIVE_DATA/$sc" > "$out/$sc.gz.part" 2>/dev/null; then
      mv -f "$out/$sc.gz.part" "$out/$sc.gz"
      say "$sc ($(human "$sc_bytes")) -> $out/$sc.gz — no ANSWER depends on it; a REPLAY does"
    else
      rm -f "$out/$sc.gz.part"
      warn "compressing $sc failed. Nothing was left behind. Losing it costs replay"
      warn "    time on the next start and no published number."
    fi
  done

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
  fi
  # Exactly one generation, and the prune is OUTSIDE the space rule above on
  # purpose: the sidecars are copied on every run, so a skipped ledger copy still
  # creates this run's directory, and a prune that only ran on the copying path
  # would accumulate a generation a day on the volume the rule exists to protect.
  find "$DEST/record" -maxdepth 1 -mindepth 1 -type d ! -name "$STAMP" -exec rm -rf {} + 2>/dev/null

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
    printf 'migrations.jsonl %s bytes (LIVE segment only)\n' "$(stat -c %s "$ARCHIVE_DATA/migrations.jsonl" 2>/dev/null || echo absent)"
    printf 'rollup.jsonl     %s bytes (the fold; losing it costs replay time, not answers)\n' \
      "$(stat -c %s "$ARCHIVE_DATA/rollup.jsonl" 2>/dev/null || echo absent)"
    printf 'brains.jsonl     %s bytes\n' "$(stat -c %s "$ARCHIVE_DATA/brains.jsonl" 2>/dev/null || echo absent)"
    printf 'closed segments  %s file(s), %s bytes\n' \
      "$(find "$ARCHIVE_DATA/segments" -maxdepth 1 -name '*.jsonl' -o -maxdepth 1 -name '*.jsonl.gz' 2>/dev/null | wc -l)" \
      "$(find "$ARCHIVE_DATA/segments" -maxdepth 1 \( -name '*.jsonl' -o -name '*.jsonl.gz' \) -printf '%s\n' 2>/dev/null | awk '{t+=$1} END {print t+0}')"
    printf 'cold-copy receipts %s\n' "$(find "$ARCHIVE_DATA/segments" -maxdepth 1 -name '*.receipt' 2>/dev/null | wc -l)"
    printf 'ledger window    %s\n' "${MV_LEDGER_WINDOW:-<the genome horizon>}"
    printf 'off-host copy    %s %s\n' "${MV_COLDCOPY:-off}" "${MV_COLDCOPY_URI:-}"
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

  deploy/coldcopy.sh covers ONE part of this tier: the closed ledger segments.
  It is also the gate on the archive's raw-ledger window — no receipt, no
  removal, forever if need be. It does NOT cover ring.json, peers.json,
  rollup.jsonl, brains.jsonl, metrics.jsonl or the genome store.

  Run 'deploy/coldcopy.sh --check' before trusting that half, and read
  ledgerSegmentsAwaitingColdCopy on /api/status afterwards. That number is the
  one that should be zero: while it is not, no segment is being removed — the
  record is safe and the DISK is what pays.

  TAKE ONE BY HAND BEFORE ANY OF THESE: a binary upgrade, a retention-rule
  change, a restore, and the wind-down. WIND-DOWN.md requires a final checked
  backup before resource retirement.
EOF
