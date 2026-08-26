#!/usr/bin/env bash
# The off-host half of the record — tier 3, which backup.sh says is "not this
# script's". This is that script.
#
# WHAT IT IS FOR. The archive rotates its ledger into daily segments and
# compresses each one once it is closed. A closed segment is IMMUTABLE: it is
# renamed into place and never rewritten, so it can be copied once and never
# again. This copies each one to an object store and writes a RECEIPT beside it.
#
# THE RECEIPT IS A GATE, NOT A NOTE. The archive will not remove a segment from
# the host until a receipt exists that names the segment, carries its exact size
# and sha256, names a destination, and carries a checksum THE STORE ITSELF
# returned after the upload. No receipt, no removal — forever if need be. So a
# cold archive that has stopped working shows up as
# `ledgerSegmentsAwaitingColdCopy` climbing on /api/status and as a disk filling
# up, and never as a record that is gone.
#
#   THE RECEIPT IS ONLY WRITTEN AFTER THE OBJECT IS READ BACK. An upload that
#   returned 200 and stored nothing durable is exactly the failure a retention
#   rule must not act on, so this script HEADs the object it just wrote and
#   compares what the store says it holds against what is on this disk.
#
# WHAT IT WILL NOT DO. It never touches the live file, never touches a plain
# (uncompressed) segment, never deletes anything on the host, and never writes a
# receipt for a segment whose verification failed. Removal is the archive's,
# behind the gate this script opens.
#
#   coldcopy.sh                  copy every closed segment that has no receipt
#   coldcopy.sh --check          can this host copy at all? Make no upload
#   coldcopy.sh --list           what is copied, what is waiting, and how much
#   coldcopy.sh --verify         re-read every receipt's object from the store
#   coldcopy.sh --restore NAME [--restore-tier ledger|genomes|metrics]
#                                fetch one file back and verify it. Name the tier
#                                when the day-seq name could exist in more than one
#   coldcopy.sh --dry-run        say what would be copied; make no call
#
# RUN `--check` BEFORE THE FIRST REAL COPY, and after any change to the
# destination. It is the one mode that answers "would this work" without writing
# an object: the CLI is on PATH, the URI names a bucket, and a LIST of the
# prefix returns. Every way this can be misconfigured has the same symptom
# otherwise — no receipt, so nothing retires, so the disk this protects fills up
# while every number on the status page looks reasonable.
set -uo pipefail

ENV_FILE="${MV_ENV_FILE:-/etc/multiverse/deploy.env}"
if [ ! -e "$ENV_FILE" ]; then
  echo "coldcopy: no $ENV_FILE" >&2
  exit 2
elif [ ! -r "$ENV_FILE" ]; then
  echo "coldcopy: $ENV_FILE exists but is NOT READABLE by $(id -un). It must be" >&2
  echo "          0640 root:${MV_GROUP:-multiverse}." >&2
  exit 2
fi
# shellcheck source=/dev/null
set -a; . "$ENV_FILE"; set +a

: "${MV_STATE:=/var/lib/multiverse}"
# off | s3 — OFF IS THE DEFAULT and it is the contract-safe one: with no cold
# archive configured, no receipt is written, and the archive removes nothing.
: "${MV_COLDCOPY:=off}"
# s3://bucket/prefix. The object key is <prefix>/<segment file name>.
: "${MV_COLDCOPY_URI:=}"
# Empty for AWS S3 itself. A Lightsail bucket or any other S3-compatible store
# puts its endpoint here; nothing else in this script changes.
: "${MV_COLDCOPY_ENDPOINT:=}"
# Empty uses the instance's own role or the host's default credentials. A named
# profile is the alternative. THE VALUE OF A KEY NEVER APPEARS IN THIS FILE OR
# IN THE PARAMETER FILE — see "which product" below.
: "${MV_COLDCOPY_PROFILE:=}"
# THE STORAGE CLASS, AND WHY THE DEFAULT IS THE PLAIN ONE.
#
# STANDARD_IA looks right on paper for an object written once and read only in a
# recovery: about half the storage price, a 30-day minimum billable life and a
# per-GB retrieval charge. It was this script's default, and against a Lightsail
# bucket EVERY UPLOAD FAILED with an explicit deny in the bucket's own policy —
# measured, one class per PUT: STANDARD and GLACIER_IR allowed, STANDARD_IA,
# ONEZONE_IA and INTELLIGENT_TIERING each a 403. A bucket that bills a flat
# bundle price charges the same for every class, so the cheaper class bought
# nothing there and cost every receipt.
#
# So the default is the class every S3-compatible store accepts. A deployment
# whose store prices classes separately, and allows the cheaper one, sets it in
# deploy.env; there is no code change in it either way. put_and_verify below
# refuses to retry a denial silently, and names this variable when it fails.
: "${MV_COLDCOPY_STORAGE_CLASS:=STANDARD}"
# The AWS CLI, overridable so deploy/test-coldcopy.sh can drive this script
# without an account.
: "${MV_COLDCOPY_AWS:=aws}"
# How many segments one run will upload. A backlog is drained over several runs
# rather than in one three-hour burst that competes with a live map.
: "${MV_COLDCOPY_MAX_PER_RUN:=4}"

ARCHIVE_DATA="$MV_STATE/archive"
SEGMENTS="$ARCHIVE_DATA/segments"
# THE THREE TIERS this script copies off-host, each an immutable .jsonl.gz with a
# receipt beside it that the archive re-checks before it removes anything:
#
#   segments          the ledger's closed daily segments (the original tier)
#   genome-bundles    the genome cold tier's dated bundles (genomecold.go)
#   metrics-segments  the metrics window's closed daily segments (metricsseg.go)
#
# They differ only in their directory and in the S3 sub-prefix their objects land
# under, so a bundle and a ledger segment of the same day-seq name cannot collide
# in the bucket. The ledger tier keeps the ORIGINAL key layout (no sub-prefix) so
# receipts already written on the host still name the object that is there.
GENOME_BUNDLES="$ARCHIVE_DATA/genome-bundles"
METRICS_SEGMENTS="$ARCHIVE_DATA/metrics-segments"
LOG="$SEGMENTS/coldcopy.jsonl"
VERSION="coldcopy.sh 2"

# tier_rows prints "dir<TAB>subprefix" for every tier whose directory exists. The
# ledger's sub-prefix is empty, which reproduces the pre-tier key exactly.
tier_rows() {
  [ -d "$SEGMENTS" ]        && printf '%s\t%s\n' "$SEGMENTS" ""
  [ -d "$GENOME_BUNDLES" ]  && printf '%s\t%s\n' "$GENOME_BUNDLES" "genome-bundles"
  [ -d "$METRICS_SEGMENTS" ] && printf '%s\t%s\n' "$METRICS_SEGMENTS" "metrics"
}
MODE=copy
RESTORE_NAME=""
RESTORE_TIER=""
DRY=0

say()  { printf '     %s\n' "$*"; }
step() { printf '\n==== %s\n' "$*"; }
warn() { printf '  !! %s\n' "$*" >&2; }
human(){ numfmt --to=iec --suffix=B "${1:-0}" 2>/dev/null || echo "${1:-0}B"; }
now_ms(){ date -u +%s%3N; }

while [ $# -gt 0 ]; do
  case "$1" in
    --list) MODE=list ;;
    --check) MODE=check ;;
    --verify) MODE=verify ;;
    --restore) MODE=restore; RESTORE_NAME="${2:-}"; shift ;;
    # WHICH TIER a restore names, because all three tiers share the day-seq name
    # grammar starting at 0000: 2026-08-25-0000.jsonl.gz can exist in every one of
    # them. Without this, a restore resolves to whichever tier comes first in
    # tier_rows (the ledger) and fetches the wrong object back. Values: ledger,
    # genomes, metrics.
    --restore-tier) RESTORE_TIER="${2:-}"; shift ;;
    --dry-run) DRY=1 ;;
    -h|--help) sed -n '2,/^set -uo pipefail$/p' "$0" | sed '$d'; exit 0 ;;
    *) echo "coldcopy: unknown argument $1" >&2; exit 2 ;;
  esac
  shift
done

# ---------------------------------------------------------------- destination

# split_uri fills BUCKET and PREFIX from s3://bucket/prefix.
BUCKET=""; PREFIX=""
split_uri() {
  local u="${1#s3://}"
  BUCKET="${u%%/*}"
  PREFIX="${u#"$BUCKET"}"
  PREFIX="${PREFIX#/}"
  PREFIX="${PREFIX%/}"
  [ -n "$BUCKET" ] || { warn "MV_COLDCOPY_URI is '$1', which names no bucket"; return 1; }
}

# awscli runs the CLI with whatever endpoint and profile were configured. Every
# call in this script goes through it, so a Lightsail bucket and an S3 bucket
# differ by one environment variable and nothing else.
awscli() {
  local args=()
  [ -n "$MV_COLDCOPY_PROFILE" ] && args+=(--profile "$MV_COLDCOPY_PROFILE")
  [ -n "$MV_COLDCOPY_ENDPOINT" ] && args+=(--endpoint-url "$MV_COLDCOPY_ENDPOINT")
  AWS_PAGER="" "$MV_COLDCOPY_AWS" "${args[@]}" "$@"
}

# key_for builds the object key: <prefix>/<subprefix>/<name>, with either part
# elided when empty. The ledger tier passes an empty sub-prefix, so its key is
# unchanged from before the cold tier existed.
key_for() {
  local name="${1:-}" sub="${2:-}" key
  key="$name"
  [ -n "$sub" ] && key="$sub/$name"
  if [ -n "$PREFIX" ]; then printf '%s/%s' "$PREFIX" "$key"; else printf '%s' "$key"; fi
}

# ---------------------------------------------------------------- the files

# closed_files lists every compressed closed file in a tier directory, oldest
# first. Sorting is on the DAY inside the name, so the ledger's legacy segment —
# whose name begins "legacy-" — sorts first rather than last, exactly as the
# archive's own ordering does. Bundles and metrics segments have no legacy form,
# so the same sort key works for all three tiers.
closed_files() {
  local dir="$1" f base sortkey
  [ -d "$dir" ] || return 0
  for f in "$dir"/*.jsonl.gz; do
    [ -e "$f" ] || continue
    base="$(basename "$f")"
    case "$base" in
      legacy-*) sortkey="${base#legacy-}"; sortkey="${sortkey%%-to-*}" ;;
      *) sortkey="${base%%-[0-9][0-9][0-9][0-9].jsonl.gz}" ;;
    esac
    printf '%s\t%s\n' "$sortkey" "$base"
  done | sort | cut -f2
}

receipt_of() { printf '%s/%s.receipt' "${2:-$SEGMENTS}" "$1"; }

# ---------------------------------------------------------------- the actions

# is_denial — does this CLI output say the STORE refused the request, rather
# than the CLI refusing a flag? Matched on the store's own vocabulary, because
# the exit status of `aws` is 255 or 1 for both.
is_denial() {
  case "$1" in
    *AccessDenied*|*"Access Denied"*|*"explicit deny"*|*"not authorized to perform"*|\
    *"(403)"*|*"status code: 403"*|*InvalidStorageClass*|*"storage class"*) return 0 ;;
  esac
  return 1
}

# deny_report — the loud failure. It names the storage class FIRST because that
# is the setting that produced this in practice, and because it is the one an
# operator can change without a release.
deny_report() {
  warn "the store REFUSED the upload of $1. This is a denial, not a retryable fault,"
  warn "    and nothing else was tried."
  warn ""
  warn "    storage class : MV_COLDCOPY_STORAGE_CLASS=$MV_COLDCOPY_STORAGE_CLASS"
  warn "    destination   : s3://$BUCKET/$(key_for "$1")"
  warn "    profile       : ${MV_COLDCOPY_PROFILE:-<none: the credentials this host already has>}"
  warn "    endpoint      : ${MV_COLDCOPY_ENDPOINT:-<AWS S3>}"
  warn ""
  warn "    A bucket policy can deny a storage class outright: a Lightsail bucket"
  warn "    allows STANDARD and GLACIER_IR and denies STANDARD_IA, ONEZONE_IA and"
  warn "    INTELLIGENT_TIERING, and a bucket billed at a flat bundle price charges"
  warn "    the same for all of them. If this deployment set a class, try STANDARD."
  warn ""
  warn "    NO RECEIPT WAS WRITTEN, so the segment stays on this host and nothing"
  warn "    retires. Run 'coldcopy.sh --check' after changing anything."
  warn "    the store said: $2"
}

# put_and_verify uploads one segment and, only if the store agrees about what it
# now holds, writes the receipt.
#
# `s3api put-object`, not `s3 cp`: `s3 cp` switches to a multipart upload above
# 8 MB and a multipart object's ETag is NOT the MD5 of its content, so the
# fallback verification would be checking a number against a different number. A
# single PUT is good to 5 GB and a segment is a few hundred megabytes.
put_and_verify() {
  local name="$1" dir="${2:-$SEGMENTS}" sub="${3:-}"
  local path="$dir/$name" key bytes sha md5 out remote kind up_ms ver_ms
  key="$(key_for "$name" "$sub")"
  bytes="$(stat -c %s "$path")"
  sha="$(sha256sum "$path" | cut -d' ' -f1)"
  md5="$(md5sum "$path" | cut -d' ' -f1)"

  if [ "$DRY" = 1 ]; then
    say "would PUT $name ($(human "$bytes")) -> s3://$BUCKET/$key"
    return 0
  fi

  up_ms="$(now_ms)"
  # --checksum-algorithm SHA256 asks the store to record and return the digest
  # it computed. A store that does not support it fails the flag rather than
  # silently ignoring it, so the second form is the fallback.
  if ! out="$(awscli s3api put-object --bucket "$BUCKET" --key "$key" --body "$path" \
        --storage-class "$MV_COLDCOPY_STORAGE_CLASS" --checksum-algorithm SHA256 2>&1)"; then
    # A DENIAL IS NOT A FLAG PROBLEM AND MUST NOT BE RETRIED QUIETLY. The retry
    # below drops --checksum-algorithm and changes nothing else, so a store that
    # refused the request itself refuses it again and the operator reads
    # "retrying without it" followed by a failure that names neither cause. The
    # measured case is a bucket policy that denies a STORAGE CLASS: every upload
    # fails, no receipt is ever written, nothing retires, and the disk fills.
    if is_denial "$out"; then
      deny_report "$name" "$out"
      return 1
    fi
    say "the store refused --checksum-algorithm; retrying without it (ETag will be the check)"
    if ! out="$(awscli s3api put-object --bucket "$BUCKET" --key "$key" --body "$path" \
          --storage-class "$MV_COLDCOPY_STORAGE_CLASS" 2>&1)"; then
      if is_denial "$out"; then
        deny_report "$name" "$out"
        return 1
      fi
      warn "upload of $name FAILED: $out"
      warn "    nothing was written on this host: no receipt, so the segment stays."
      return 1
    fi
  fi

  # THE VERIFICATION. Ask the store what it holds; do not trust the PUT.
  local head
  if ! head="$(awscli s3api head-object --bucket "$BUCKET" --key "$key" 2>&1)"; then
    warn "$name uploaded but HEAD failed: $head"
    warn "    no receipt is written, so the segment stays on the host."
    return 1
  fi
  local rbytes rsha retag
  rbytes="$(printf '%s' "$head" | sed -n 's/.*"ContentLength": *\([0-9]*\).*/\1/p' | head -1)"
  rsha="$(printf '%s' "$head" | sed -n 's/.*"ChecksumSHA256": *"\([^"]*\)".*/\1/p' | head -1)"
  retag="$(printf '%s' "$head" | sed -n 's/.*"ETag": *"\\*"\{0,1\}\([0-9a-fA-F]*\)[^"]*".*/\1/p' | head -1)"

  if [ "${rbytes:-0}" != "$bytes" ]; then
    warn "$name: the store holds ${rbytes:-unknown} bytes and this host has $bytes. No receipt."
    return 1
  fi
  if [ -n "$rsha" ]; then
    # ChecksumSHA256 is base64 of the RAW digest, so compare in that form.
    local want_b64
    want_b64="$(printf '%s' "$sha" | xxd -r -p | base64)"
    if [ "$rsha" != "$want_b64" ]; then
      warn "$name: the store's sha256 does not match this host's. No receipt."
      return 1
    fi
    remote="$rsha"; kind="sha256"
  elif [ -n "$retag" ]; then
    if [ "$retag" != "$md5" ]; then
      warn "$name: the store's ETag ($retag) is not this file's MD5 ($md5)."
      warn "    An ETag that is not an MD5 means the object was stored multipart;"
      warn "    this script uses a single PUT, so treat it as a failed verification."
      return 1
    fi
    remote="$retag"; kind="etag"
  else
    warn "$name: the store returned neither a checksum nor a usable ETag. No receipt."
    return 1
  fi

  ver_ms="$(now_ms)"
  # THE RECEIPT, written atomically. Its shape is the archive's ColdCopyReceipt
  # (go/internal/archive/coldcopy.go) and the archive re-checks every field of it
  # against the bytes on this disk before it removes anything.
  local rp; rp="$(receipt_of "$name" "$dir")"
  cat > "$rp.tmp" <<JSON
{"segment":"$name","bytes":$bytes,"sha256":"$sha","destination":"s3://$BUCKET/$key","endpoint":"$MV_COLDCOPY_ENDPOINT","remoteChecksum":"$remote","remoteChecksumKind":"$kind","uploadedAtMs":$up_ms,"verifiedAtMs":$ver_ms,"verifiedBy":"$VERSION"}
JSON
  mv -f "$rp.tmp" "$rp" || { warn "could not write $rp"; return 1; }
  # The append-only history. It outlives the receipts and it is what an operator
  # reads to answer "where did the first month go".
  printf '%s\n' "$(cat "$rp")" >> "$LOG"
  say "$name ($(human "$bytes")) -> s3://$BUCKET/$key  verified by $kind"
  return 0
}

do_copy() {
  step "closed files with no cold-copy receipt (ledger, genome bundles, metrics)"
  split_uri "$MV_COLDCOPY_URI" || exit 2
  local n=0 ok=0 fail=0 name dir sub capped=0
  while IFS="$(printf '\t')" read -r dir sub; do
    [ -n "$dir" ] || continue
    while IFS= read -r name; do
      [ -n "$name" ] || continue
      [ -e "$(receipt_of "$name" "$dir")" ] && continue
      n=$((n + 1))
      if [ "$n" -gt "$MV_COLDCOPY_MAX_PER_RUN" ]; then
        say "$MV_COLDCOPY_MAX_PER_RUN uploaded or attempted this run; the rest wait for the next."
        capped=1
        break
      fi
      if put_and_verify "$name" "$dir" "$sub"; then ok=$((ok + 1)); else fail=$((fail + 1)); fi
    done < <(closed_files "$dir")
    [ "$capped" = 1 ] && break
  done < <(tier_rows)
  if [ "$n" = 0 ]; then
    say "nothing waiting: every closed file already has a verified receipt."
  fi
  say "copied $ok, failed $fail"
  [ "$fail" = 0 ]
}

do_verify() {
  step "re-reading every receipt's object from the store (all tiers)"
  split_uri "$MV_COLDCOPY_URI" || exit 2
  local rp name key bad=0 seen=0 dir sub
  while IFS="$(printf '\t')" read -r dir sub; do
    [ -n "$dir" ] || continue
    for rp in "$dir"/*.jsonl.gz.receipt; do
      [ -e "$rp" ] || continue
      seen=$((seen + 1))
      name="$(basename "$rp" .receipt)"
      key="$(key_for "$name" "$sub")"
      local want_bytes; want_bytes="$(sed -n 's/.*"bytes":\([0-9]*\).*/\1/p' "$rp")"
      local head
      if ! head="$(awscli s3api head-object --bucket "$BUCKET" --key "$key" 2>&1)"; then
        warn "$name: the object is NOT in the store ($head)"
        warn "    THE HOST COPY MAY ALREADY BE GONE. Do not delete anything; investigate."
        bad=$((bad + 1)); continue
      fi
      local rbytes; rbytes="$(printf '%s' "$head" | sed -n 's/.*"ContentLength": *\([0-9]*\).*/\1/p' | head -1)"
      if [ "${rbytes:-0}" != "${want_bytes:-x}" ]; then
        warn "$name: the store holds ${rbytes:-unknown} bytes, the receipt says ${want_bytes:-unknown}"
        bad=$((bad + 1)); continue
      fi
      say "$name  ok  ($(human "${rbytes:-0}"))"
    done
  done < <(tier_rows)
  say "$seen receipt(s), $bad problem(s)"
  [ "$bad" = 0 ]
}

do_restore() {
  [ -n "$RESTORE_NAME" ] || { warn "--restore needs a file name, e.g. 2026-08-16-0000.jsonl.gz"; exit 2; }
  split_uri "$MV_COLDCOPY_URI" || exit 2
  step "restoring $RESTORE_NAME"
  # WHICH TIER owns this name? An EXPLICIT --restore-tier is authoritative, because
  # all three tiers share the day-seq grammar and a name can exist in more than one
  # — the archive's on-demand restore always names the tier for exactly this
  # reason. Only when none is given do we fall back to searching by receipt (a
  # manual ledger restore), and that search takes the ledger first.
  local dir="" sub="" d s
  if [ -n "$RESTORE_TIER" ]; then
    case "$RESTORE_TIER" in
      ledger)  dir="$SEGMENTS"; sub="" ;;
      genomes|genome-bundles) dir="$GENOME_BUNDLES"; sub="genome-bundles" ;;
      metrics|metrics-segments) dir="$METRICS_SEGMENTS"; sub="metrics" ;;
      *) warn "--restore-tier '$RESTORE_TIER' is not one of: ledger, genomes, metrics"; exit 2 ;;
    esac
    say "tier: $dir  (sub-prefix ${sub:-<none>}, from --restore-tier $RESTORE_TIER)"
  else
    while IFS="$(printf '\t')" read -r d s; do
      [ -n "$d" ] || continue
      if [ -e "$d/$RESTORE_NAME.receipt" ] || [ -e "$d/$RESTORE_NAME" ]; then
        dir="$d"; sub="$s"; break
      fi
    done < <(tier_rows)
    if [ -z "$dir" ]; then
      dir="$SEGMENTS"; sub=""
      say "no receipt for $RESTORE_NAME on this host and no --restore-tier; assuming the ledger tier."
    else
      say "tier: $dir  (sub-prefix ${sub:-<none>}, resolved by receipt)"
    fi
  fi
  local rp key dest want_sha
  rp="$(receipt_of "$RESTORE_NAME" "$dir")"
  key="$(key_for "$RESTORE_NAME" "$sub")"
  dest="$dir/$RESTORE_NAME"
  if [ -e "$dest" ]; then
    warn "$dest already exists. Move it aside first; this script does not overwrite a file."
    exit 1
  fi
  if [ -r "$rp" ]; then
    want_sha="$(sed -n 's/.*"sha256":"\([0-9a-f]*\)".*/\1/p' "$rp")"
    say "the receipt says sha256 ${want_sha:0:12}…"
  else
    warn "there is no receipt for $RESTORE_NAME on this host."
    warn "    The fetch will still run; the digest cannot be checked against anything local."
  fi
  mkdir -p "$dir" || exit 1
  if ! awscli s3api get-object --bucket "$BUCKET" --key "$key" "$dest.part" >/dev/null; then
    warn "fetching s3://$BUCKET/$key failed"
    rm -f "$dest.part"; exit 1
  fi
  local got_sha; got_sha="$(sha256sum "$dest.part" | cut -d' ' -f1)"
  if [ -n "${want_sha:-}" ] && [ "$got_sha" != "$want_sha" ]; then
    warn "the fetched object hashes to ${got_sha:0:12}… and the receipt says ${want_sha:0:12}…"
    warn "    it is left at $dest.part and NOT put into place."
    exit 1
  fi
  # It must also decompress, which is the check that matters for a ledger
  # segment: a gz that hashes right and will not decompress is still no record.
  if ! gzip -t "$dest.part" 2>/dev/null; then
    warn "the fetched object is not a valid gzip stream. Left at $dest.part."
    exit 1
  fi
  local lines; lines="$(gzip -dc "$dest.part" | wc -l)"
  mv -f "$dest.part" "$dest" || exit 1
  chown "${MV_USER:-multiverse}:${MV_GROUP:-multiverse}" "$dest" 2>/dev/null || true
  say "restored $dest  ($lines record line(s), sha256 verified)"
  say ""
  say "The archive picks it up on its next start: segments are read in order by"
  say "the day inside the name, and a restored one simply reappears in the run."
  say "The window will retire it again once it is past the window AND its receipt"
  say "is present, so keep the receipt if you want it kept."
}

# do_check — "would a copy work?", answered without writing an object.
#
# EVERY WAY THIS CAN BE MISCONFIGURED HAS THE SAME SYMPTOM: no receipt, so the
# archive retires nothing, so the disk fills while /api/status still reads
# sensibly and ledgerSegmentsAwaitingColdCopy climbs. Two of those ways were
# found by trying, not by reading: the AWS CLI was not installed on the service
# host at all, and the shipped storage class was denied by the bucket's own
# policy. So this exists to be run BEFORE the first real copy and after any
# change to the destination.
#
# WHAT IT CANNOT PROVE: that a PUT will be accepted. A bucket can allow a LIST
# and deny a write, and only a write finds that out. It says so rather than
# implying otherwise, and the first real run is still the proof.
do_check() {
  step "can this host copy a segment off-box?"
  local bad=0

  if [ "$MV_COLDCOPY" = off ]; then
    warn "MV_COLDCOPY=off. Nothing is copied and therefore NOTHING IS EVER RETIRED,"
    warn "    whatever MV_LEDGER_WINDOW says. That is the safe default, not a fault."
    warn "    Set MV_COLDCOPY=s3 and MV_COLDCOPY_URI to turn the off-host copy on."
    return 3
  fi
  say "mode                 $MV_COLDCOPY"

  if command -v "$MV_COLDCOPY_AWS" >/dev/null 2>&1; then
    say "aws cli              $(command -v "$MV_COLDCOPY_AWS")  ($("$MV_COLDCOPY_AWS" --version 2>&1 | head -1))"
  else
    warn "aws cli              NOT ON PATH as '$MV_COLDCOPY_AWS'."
    warn "    The service host does not get one by default. deploy/provision.sh installs"
    warn "    it in the packages phase: sudo deploy/provision.sh --only packages"
    bad=$((bad + 1))
  fi

  if [ -z "$MV_COLDCOPY_URI" ]; then
    warn "MV_COLDCOPY_URI     UNSET. It is a live resource id and lives in this host's"
    warn "    deploy.env and in the private operations record, never in the repository."
    bad=$((bad + 1))
  elif split_uri "$MV_COLDCOPY_URI"; then
    say "bucket               $BUCKET"
    say "prefix               ${PREFIX:-<none>}"
  else
    bad=$((bad + 1))
  fi

  say "storage class        $MV_COLDCOPY_STORAGE_CLASS  (a PUT is the only thing that proves it)"
  say "endpoint             ${MV_COLDCOPY_ENDPOINT:-<AWS S3>}"
  say "profile              ${MV_COLDCOPY_PROFILE:-<none: the credentials this host already has>}"

  if [ "$bad" = 0 ]; then
    # THE READ. head-bucket first: it is the cheapest proof that the bucket
    # exists, that this host has a credential for it, and that the region and
    # endpoint are right. A store that does not grant it is not a failure — the
    # LIST below is the one that matters, because it reads the PREFIX this
    # script writes into.
    local out
    if out="$(awscli s3api head-bucket --bucket "$BUCKET" 2>&1)"; then
      say "head-bucket          ok"
    else
      say "head-bucket          refused (not fatal): $(printf '%s' "$out" | head -1)"
    fi
    if out="$(awscli s3api list-objects-v2 --bucket "$BUCKET" --prefix "$(key_for '')" --max-keys 1 2>&1)"; then
      local n
      n="$(printf '%s' "$out" | grep -c '"Key"')"
      say "list of the prefix   ok ($n object(s) shown of however many are there)"
    else
      warn "list of the prefix   FAILED: $out"
      warn "    This is the credential, the bucket name, the region or the endpoint."
      if is_denial "$out"; then
        warn "    The store denied the request rather than failing to find it."
      fi
      bad=$((bad + 1))
    fi
  fi

  # The receipts land beside the files, and a directory this process cannot write
  # is a copy that succeeds off-host and is never recorded on it. Every tier the
  # archive uses must be writable — the systemd unit names all three in
  # ReadWritePaths.
  local d
  for d in "$SEGMENTS" "$GENOME_BUNDLES" "$METRICS_SEGMENTS"; do
    if [ -d "$d" ]; then
      if [ -w "$d" ]; then
        say "tier directory       $d (writable)"
      else
        warn "tier directory       $d is NOT WRITABLE by $(id -un)."
        warn "    An upload would succeed and its receipt would not be written, so the"
        warn "    file would be uploaded again on every run and never retire."
        bad=$((bad + 1))
      fi
    else
      say "tier directory       $d does not exist yet: nothing closed in it."
    fi
  done

  say ""
  if [ "$bad" = 0 ]; then
    say "READY. Nothing was uploaded. The first real run is still the proof that a"
    say "PUT is allowed; read its output, then 'coldcopy.sh --list'."
    return 0
  fi
  warn "$bad problem(s). No segment can leave this host until they are fixed, and"
  warn "    the archive retires nothing meanwhile — which is the designed safety."
  return 1
}

do_list() {
  step "the off-host copy of the record"
  say "destination          ${MV_COLDCOPY_URI:-<unset>}  (mode $MV_COLDCOPY)"
  say "endpoint             ${MV_COLDCOPY_ENDPOINT:-<AWS S3>}"
  local gtotal=0 gwaiting=0 gcopied=0 gretired=0 name dir sub rp bytes label
  while IFS="$(printf '\t')" read -r dir sub; do
    [ -n "$dir" ] || continue
    label="${sub:-ledger}"
    say ""
    say "tier $label   ($dir)"
    local total=0 waiting=0 copied=0 retired=0
    while IFS= read -r name; do
      [ -n "$name" ] || continue
      total=$((total + 1)); gtotal=$((gtotal + 1))
      bytes="$(stat -c %s "$dir/$name" 2>/dev/null || echo 0)"
      if [ -e "$(receipt_of "$name" "$dir")" ]; then
        copied=$((copied + 1)); gcopied=$((gcopied + 1))
        printf '     %-46s %10s  copied\n' "$name" "$(human "$bytes")"
      else
        waiting=$((waiting + 1)); gwaiting=$((gwaiting + 1))
        printf '     %-46s %10s  WAITING\n' "$name" "$(human "$bytes")"
      fi
    done < <(closed_files "$dir")
    # A receipt with no file is a RETIRED one: the bytes are off-host and this is
    # the only record of where they went.
    for rp in "$dir"/*.jsonl.gz.receipt; do
      [ -e "$rp" ] || continue
      name="$(basename "$rp" .receipt)"
      [ -e "$dir/$name" ] && continue
      retired=$((retired + 1)); gretired=$((gretired + 1))
      printf '     %-46s %10s  RETIRED (off-host only)\n' "$name" "-"
    done
    say "     $total present ($copied copied, $waiting waiting), $retired retired"
  done < <(tier_rows)
  say ""
  say "totals: $gtotal present on this host — $gcopied copied, $gwaiting waiting; $gretired retired"
  say ""
  say "The archive publishes the same picture per tier on /api/status:"
  say "  ledger  ledgerSegments / ledgerSegmentsAwaitingColdCopy / ledgerRetiredTotal"
  say "  genomes genomesCold / genomeBundlesAwaitingColdCopy / genomesRetired"
  say "  metrics metricsSegmentsAwaitingColdCopy"
}

# ---------------------------------------------------------------- run

# --check runs before every gate below, because the gates are what it reports on:
# a missing CLI and an unset URI are two of the things it exists to find, and a
# host with no closed segment yet is exactly when an operator wants to run it.
if [ "$MODE" = check ]; then
  do_check; exit $?
fi

if [ ! -d "$SEGMENTS" ] && [ ! -d "$GENOME_BUNDLES" ] && [ ! -d "$METRICS_SEGMENTS" ]; then
  say "no tier directory yet under $ARCHIVE_DATA: this archive has closed nothing."
  exit 0
fi

case "$MODE" in
  list) do_list; exit 0 ;;
esac

if [ "$MV_COLDCOPY" = off ]; then
  cat >&2 <<'EOF'
coldcopy: MV_COLDCOPY=off, so nothing is copied off this host.

  That is the safe default and it is not a failure. With no cold copy the
  archive writes no receipt, so it REMOVES NOTHING: every closed segment is
  compressed and kept, which is design option D and changes no promise.

  It also means the raw ledger window can never take effect. Set MV_COLDCOPY=s3
  and MV_COLDCOPY_URI before turning the window on, or the disk this protects
  fills up instead.
EOF
  exit 3
fi

command -v "$MV_COLDCOPY_AWS" >/dev/null 2>&1 || {
  warn "$MV_COLDCOPY_AWS is not on PATH. Install the AWS CLI or set MV_COLDCOPY_AWS."
  exit 2
}
[ -n "$MV_COLDCOPY_URI" ] || { warn "MV_COLDCOPY_URI is unset"; exit 2; }

case "$MODE" in
  copy)    do_copy ;;
  verify)  do_verify ;;
  restore) do_restore ;;
esac
