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
#   coldcopy.sh --list           what is copied, what is waiting, and how much
#   coldcopy.sh --verify         re-read every receipt's object from the store
#   coldcopy.sh --restore NAME   fetch one segment back and verify it
#   coldcopy.sh --dry-run        say what would be copied; make no call
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
# STANDARD_IA halves the storage price for an object nobody reads. It has a
# 30-day minimum billable life and a per-GB retrieval charge, which is the right
# shape for this: a segment is written once and read only in a recovery.
: "${MV_COLDCOPY_STORAGE_CLASS:=STANDARD_IA}"
# The AWS CLI, overridable so deploy/test-coldcopy.sh can drive this script
# without an account.
: "${MV_COLDCOPY_AWS:=aws}"
# How many segments one run will upload. A backlog is drained over several runs
# rather than in one three-hour burst that competes with a live map.
: "${MV_COLDCOPY_MAX_PER_RUN:=4}"

ARCHIVE_DATA="$MV_STATE/archive"
SEGMENTS="$ARCHIVE_DATA/segments"
LOG="$SEGMENTS/coldcopy.jsonl"
VERSION="coldcopy.sh 1"
MODE=copy
RESTORE_NAME=""
DRY=0

say()  { printf '     %s\n' "$*"; }
step() { printf '\n==== %s\n' "$*"; }
warn() { printf '  !! %s\n' "$*" >&2; }
human(){ numfmt --to=iec --suffix=B "${1:-0}" 2>/dev/null || echo "${1:-0}B"; }
now_ms(){ date -u +%s%3N; }

while [ $# -gt 0 ]; do
  case "$1" in
    --list) MODE=list ;;
    --verify) MODE=verify ;;
    --restore) MODE=restore; RESTORE_NAME="${2:-}"; shift ;;
    --dry-run) DRY=1 ;;
    -h|--help) sed -n '2,33p' "$0"; exit 0 ;;
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

key_for() {
  if [ -n "$PREFIX" ]; then printf '%s/%s' "$PREFIX" "$1"; else printf '%s' "$1"; fi
}

# ---------------------------------------------------------------- the segments

# closed_segments lists every compressed closed segment, oldest first. Sorting
# is on the DAY inside the name, so the legacy segment — whose name begins
# "legacy-" and whose records are the oldest in the system — sorts first rather
# than last, exactly as the archive's own ordering does.
closed_segments() {
  [ -d "$SEGMENTS" ] || return 0
  local f base sortkey
  for f in "$SEGMENTS"/*.jsonl.gz; do
    [ -e "$f" ] || continue
    base="$(basename "$f")"
    case "$base" in
      legacy-*) sortkey="${base#legacy-}"; sortkey="${sortkey%%-to-*}" ;;
      *) sortkey="${base%%-[0-9][0-9][0-9][0-9].jsonl.gz}" ;;
    esac
    printf '%s\t%s\n' "$sortkey" "$base"
  done | sort | cut -f2
}

receipt_of() { printf '%s/%s.receipt' "$SEGMENTS" "$1"; }

# ---------------------------------------------------------------- the actions

# put_and_verify uploads one segment and, only if the store agrees about what it
# now holds, writes the receipt.
#
# `s3api put-object`, not `s3 cp`: `s3 cp` switches to a multipart upload above
# 8 MB and a multipart object's ETag is NOT the MD5 of its content, so the
# fallback verification would be checking a number against a different number. A
# single PUT is good to 5 GB and a segment is a few hundred megabytes.
put_and_verify() {
  local name="$1" path="$SEGMENTS/$1" key bytes sha md5 out remote kind up_ms ver_ms
  key="$(key_for "$name")"
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
    say "the store refused --checksum-algorithm; retrying without it (ETag will be the check)"
    if ! out="$(awscli s3api put-object --bucket "$BUCKET" --key "$key" --body "$path" \
          --storage-class "$MV_COLDCOPY_STORAGE_CLASS" 2>&1)"; then
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
  local rp; rp="$(receipt_of "$name")"
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
  step "closed segments with no cold-copy receipt"
  split_uri "$MV_COLDCOPY_URI" || exit 2
  local n=0 ok=0 fail=0 name
  while IFS= read -r name; do
    [ -n "$name" ] || continue
    [ -e "$(receipt_of "$name")" ] && continue
    n=$((n + 1))
    if [ "$n" -gt "$MV_COLDCOPY_MAX_PER_RUN" ]; then
      say "$MV_COLDCOPY_MAX_PER_RUN uploaded or attempted this run; the rest wait for the next."
      break
    fi
    if put_and_verify "$name"; then ok=$((ok + 1)); else fail=$((fail + 1)); fi
  done < <(closed_segments)
  if [ "$n" = 0 ]; then
    say "nothing waiting: every closed segment already has a verified receipt."
  fi
  say "copied $ok, failed $fail"
  [ "$fail" = 0 ]
}

do_verify() {
  step "re-reading every receipt's object from the store"
  split_uri "$MV_COLDCOPY_URI" || exit 2
  local rp name key bad=0 seen=0
  for rp in "$SEGMENTS"/*.jsonl.gz.receipt; do
    [ -e "$rp" ] || continue
    seen=$((seen + 1))
    name="$(basename "$rp" .receipt)"
    key="$(key_for "$name")"
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
  say "$seen receipt(s), $bad problem(s)"
  [ "$bad" = 0 ]
}

do_restore() {
  [ -n "$RESTORE_NAME" ] || { warn "--restore needs a segment file name, e.g. 2026-08-16-0000.jsonl.gz"; exit 2; }
  split_uri "$MV_COLDCOPY_URI" || exit 2
  step "restoring $RESTORE_NAME"
  local rp key dest want_sha
  rp="$(receipt_of "$RESTORE_NAME")"
  key="$(key_for "$RESTORE_NAME")"
  dest="$SEGMENTS/$RESTORE_NAME"
  if [ -e "$dest" ]; then
    warn "$dest already exists. Move it aside first; this script does not overwrite a segment."
    exit 1
  fi
  if [ -r "$rp" ]; then
    want_sha="$(sed -n 's/.*"sha256":"\([0-9a-f]*\)".*/\1/p' "$rp")"
    say "the receipt says sha256 ${want_sha:0:12}…"
  else
    warn "there is no receipt for $RESTORE_NAME on this host."
    warn "    The fetch will still run; the digest cannot be checked against anything local."
  fi
  mkdir -p "$SEGMENTS" || exit 1
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

do_list() {
  step "the off-host copy of the record"
  say "segments directory   $SEGMENTS"
  say "destination          ${MV_COLDCOPY_URI:-<unset>}  (mode $MV_COLDCOPY)"
  say "endpoint             ${MV_COLDCOPY_ENDPOINT:-<AWS S3>}"
  say ""
  local name total=0 waiting=0 copied=0 bytes
  while IFS= read -r name; do
    [ -n "$name" ] || continue
    total=$((total + 1))
    bytes="$(stat -c %s "$SEGMENTS/$name" 2>/dev/null || echo 0)"
    if [ -e "$(receipt_of "$name")" ]; then
      copied=$((copied + 1))
      printf '     %-46s %10s  copied\n' "$name" "$(human "$bytes")"
    else
      waiting=$((waiting + 1))
      printf '     %-46s %10s  WAITING\n' "$name" "$(human "$bytes")"
    fi
  done < <(closed_segments)
  # A receipt with no segment is a RETIRED segment: the bytes are off-host and
  # this is the only record of where they went.
  local rp retired=0
  for rp in "$SEGMENTS"/*.jsonl.gz.receipt; do
    [ -e "$rp" ] || continue
    name="$(basename "$rp" .receipt)"
    [ -e "$SEGMENTS/$name" ] && continue
    retired=$((retired + 1))
    printf '     %-46s %10s  RETIRED (off-host only)\n' "$name" "-"
  done
  say ""
  say "$total closed segment(s) on this host: $copied copied, $waiting waiting"
  say "$retired retired segment(s): the off-host copy is the only one"
  say ""
  say "The archive publishes the same picture as ledgerSegments,"
  say "ledgerSegmentsAwaitingColdCopy and ledgerRetiredTotal on /api/status."
}

# ---------------------------------------------------------------- run

if [ ! -d "$SEGMENTS" ]; then
  say "no $SEGMENTS yet: this archive has not closed a segment."
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
