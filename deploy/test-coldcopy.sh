#!/usr/bin/env bash
# Exercise deploy/coldcopy.sh with NO AWS account, NO bucket and NO host.
#
# THE RULE THIS FILE EXISTS FOR: a receipt is only written after the object store
# has been asked what it holds and has agreed. The receipt is the gate on the one
# irreversible action in this system — the archive removing a segment from the
# host — so every way the store can disagree has to leave NO RECEIPT, and every
# one of them is a case below.
#
# The seam is MV_COLDCOPY_AWS: a fake `aws` that records what it was asked and
# answers what the case wants it to. No network call is made and no credential
# is read.
#
#   deploy/test-coldcopy.sh        run every case
#   deploy/test-coldcopy.sh -v     also print each case as it passes
set -uo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CC="$HERE/coldcopy.sh"
[ -x "$CC" ] || { echo "test-coldcopy: $CC is not executable" >&2; exit 2; }

SHOW=0
[ "${1:-}" = "-v" ] && SHOW=1

TMP="$(mktemp -d "${TMPDIR:-/tmp}/mvcctest.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT

PASS=0; FAIL=0
fail() { printf 'FAIL  %s\n' "$1" >&2; [ $# -gt 1 ] && printf '      %s\n' "${@:2}" >&2; FAIL=$((FAIL+1)); }
pass() { PASS=$((PASS+1)); [ "$SHOW" = 1 ] && printf 'ok    %s\n' "$1"; return 0; }
eq()   { if [ "$2" = "$3" ]; then pass "$1"; else fail "$1" "expected: $3" "actual:   $2"; fi; }
has()  { if printf '%s' "$2" | grep -qF -- "$3"; then pass "$1"; else fail "$1" "expected to contain: $3" "actual: $2"; fi; }

# ---------------------------------------------------------------- the fixture

# newstate builds a data directory with closed segments in it and an env file
# pointing at it. MV_CC_MODE selects how the fake store behaves.
newstate() {
  local root="$TMP/$1"; shift
  rm -rf "$root"
  mkdir -p "$root/archive/segments"
  local d
  for d in "$@"; do
    printf '{"type":"MIGRATION","recordedAt":1,"migrationId":"%s"}\n' "$d" \
      | gzip -1 -c > "$root/archive/segments/$d.jsonl.gz"
  done
  cat > "$root/deploy.env" <<EOF
MV_STATE=$root
MV_COLDCOPY=s3
MV_COLDCOPY_URI=s3://example-cold/ledger
MV_COLDCOPY_AWS=$TMP/fake-aws
MV_COLDCOPY_STORAGE_CLASS=STANDARD_IA
EOF
  printf '%s' "$root"
}

# The fake AWS CLI. It writes every invocation to $MV_CC_CALLS and answers
# head-object according to $MV_CC_MODE.
cat > "$TMP/fake-aws" <<'FAKE'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "${MV_CC_CALLS:-/dev/null}"
op=""
for a in "$@"; do case "$a" in put-object|head-object|get-object) op="$a"; break;; esac; done
# The body/key this call named, so the fake can answer about the right file.
body=""; key=""
prev=""
for a in "$@"; do
  case "$prev" in --body) body="$a";; --key) key="$a";; esac
  prev="$a"
done
case "$op" in
  put-object)
    case "${MV_CC_MODE:-ok}" in
      put-fails) echo "An error occurred (AccessDenied)" >&2; exit 1 ;;
      no-checksum-flag)
        for a in "$@"; do [ "$a" = "--checksum-algorithm" ] && {
          echo "Unknown options: --checksum-algorithm" >&2; exit 255; }; done ;;
    esac
    echo '{"ETag": "\"deadbeef\""}'
    ;;
  head-object)
    src="${MV_CC_SRC:-}/$(basename "$key")"
    bytes="$(stat -c %s "$src" 2>/dev/null || echo 0)"
    sha="$(sha256sum "$src" 2>/dev/null | cut -d' ' -f1)"
    md5="$(md5sum "$src" 2>/dev/null | cut -d' ' -f1)"
    b64="$(printf '%s' "$sha" | xxd -r -p | base64)"
    case "${MV_CC_MODE:-ok}" in
      head-fails) echo "An error occurred (404) when calling the HeadObject operation" >&2; exit 1 ;;
      short) bytes=$((bytes - 1)) ;;
      wrong-sum) b64="AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=" ;;
      no-checksum-flag|etag-only) printf '{"ContentLength": %s, "ETag": "\\"%s\\""}\n' "$bytes" "$md5"; exit 0 ;;
      etag-wrong) printf '{"ContentLength": %s, "ETag": "\\"%s\\""}\n' "$bytes" "00000000000000000000000000000000"; exit 0 ;;
      nothing) printf '{"ContentLength": %s}\n' "$bytes"; exit 0 ;;
    esac
    printf '{"ContentLength": %s, "ChecksumSHA256": "%s", "ETag": "\\"%s\\""}\n' "$bytes" "$b64" "$md5"
    ;;
  get-object)
    src="${MV_CC_SRC:-}/$(basename "$key")"
    dest=""
    for a in "$@"; do dest="$a"; done
    case "${MV_CC_MODE:-ok}" in
      corrupt-restore) printf 'not a gzip stream at all' > "$dest" ;;
      *) cp "$src" "$dest" ;;
    esac
    echo '{"ContentLength": 1}'
    ;;
  *) echo '{}' ;;
esac
FAKE
chmod +x "$TMP/fake-aws"

run() { # run <root> [args...]
  local root="$1"; shift
  MV_ENV_FILE="$root/deploy.env" \
  MV_CC_CALLS="$root/calls.txt" \
  MV_CC_SRC="$root/archive/segments" \
  MV_CC_MODE="${MODE:-ok}" \
  "$CC" "$@" 2>&1
}

receipts() { find "$1/archive/segments" -name '*.receipt' 2>/dev/null | wc -l | tr -d ' '; }

# ---------------------------------------------------------------- cases

# 1. The happy path: every segment copied, a receipt for each, in the shape the
#    archive's gate reads.
R="$(newstate happy 2026-08-14-0000 2026-08-15-0000)"
MODE=ok out="$(run "$R")"
eq  "happy: both segments copied" "$(receipts "$R")" "2"
has "happy: says what it verified with" "$out" "verified by sha256"
rec="$R/archive/segments/2026-08-14-0000.jsonl.gz.receipt"
for field in '"segment":"2026-08-14-0000.jsonl.gz"' '"sha256":' '"destination":"s3://example-cold/ledger/' \
             '"remoteChecksumKind":"sha256"' '"verifiedAtMs":' '"verifiedBy":"coldcopy.sh'; do
  has "happy: the receipt carries $field" "$(cat "$rec")" "$field"
done
# The receipt's sha256 is the segment's real one — the gate recomputes it and a
# receipt that lied would keep the segment forever.
want_sha="$(sha256sum "$R/archive/segments/2026-08-14-0000.jsonl.gz" | cut -d' ' -f1)"
has "happy: the receipt's sha256 is the file's" "$(cat "$rec")" "\"sha256\":\"$want_sha\""
want_bytes="$(stat -c %s "$R/archive/segments/2026-08-14-0000.jsonl.gz")"
has "happy: the receipt's size is the file's" "$(cat "$rec")" "\"bytes\":$want_bytes"
# And the history log has a line per copy.
eq "happy: the coldcopy log has a line per segment" \
   "$(wc -l < "$R/archive/segments/coldcopy.jsonl" | tr -d ' ')" "2"

# 2. A second run copies nothing: a receipt means done.
MODE=ok out="$(run "$R")"
has "idempotent: nothing waiting on the second run" "$out" "nothing waiting"

# 3. THE VERIFICATION IS REAL. Every way the store can disagree leaves NO
#    RECEIPT, which is the only thing standing between a defect and a deleted
#    record.
for mode in put-fails head-fails short wrong-sum etag-wrong nothing; do
  R2="$(newstate "bad-$mode" 2026-08-14-0000)"
  MODE="$mode" out="$(run "$R2")"
  eq "$mode: no receipt is written" "$(receipts "$R2")" "0"
done

# 4. A store with no ChecksumSHA256 support falls back to the ETag, and the ETag
#    of a single PUT is the MD5. That is the Lightsail-bucket path.
R3="$(newstate etag 2026-08-14-0000)"
MODE=etag-only out="$(run "$R3")"
eq  "etag fallback: a receipt is written" "$(receipts "$R3")" "1"
has "etag fallback: the receipt says which check was used" \
    "$(cat "$R3/archive/segments/2026-08-14-0000.jsonl.gz.receipt")" '"remoteChecksumKind":"etag"'

# 5. A store that rejects --checksum-algorithm is retried without it.
R4="$(newstate noflag 2026-08-14-0000)"
MODE=no-checksum-flag out="$(run "$R4")"
eq  "no checksum flag: a receipt is still written" "$(receipts "$R4")" "1"
has "no checksum flag: it says it retried" "$out" "retrying without it"

# 6. IT USES put-object, NOT `s3 cp`. `s3 cp` goes multipart above 8 MB and a
#    multipart ETag is not an MD5, so the fallback check would be comparing a
#    number to a different number.
has "single PUT: put-object is what is called" "$(cat "$R3/calls.txt")" "s3api put-object"
if grep -qE '(^| )s3 cp' "$R3/calls.txt"; then
  fail "single PUT: the script used 's3 cp', whose ETag is not an MD5 above 8 MB"
else
  pass "single PUT: 's3 cp' is not used"
fi

# 7. --dry-run makes no call at all.
R5="$(newstate dry 2026-08-14-0000)"
MODE=ok out="$(run "$R5" --dry-run)"
eq  "dry run: no receipt" "$(receipts "$R5")" "0"
eq  "dry run: no AWS call" "$(cat "$R5/calls.txt" 2>/dev/null | wc -l | tr -d ' ')" "0"
has "dry run: says what it would do" "$out" "would PUT"

# 8. OFF IS THE DEFAULT and it is not a failure of configuration, it is the
#    contract-safe state: no receipt, so the archive removes nothing.
R6="$(newstate off 2026-08-14-0000)"
sed -i 's/^MV_COLDCOPY=s3$/MV_COLDCOPY=off/' "$R6/deploy.env"
out="$(run "$R6")"; rc=$?
eq  "off: exits 3" "$rc" "3"
has "off: says the window cannot take effect" "$out" "window can never take effect"
eq  "off: no receipt" "$(receipts "$R6")" "0"

# 9. --list is readable and names what is waiting.
R7="$(newstate list 2026-08-14-0000 2026-08-15-0000)"
MODE=ok run "$R7" >/dev/null
rm -f "$R7/archive/segments/2026-08-15-0000.jsonl.gz.receipt"
out="$(run "$R7" --list)"
has "list: names a copied segment"  "$out" "2026-08-14-0000.jsonl.gz"
has "list: marks the uncopied one"  "$out" "WAITING"
# A receipt with no segment is a RETIRED one and must be shown as such: it is
# the only record on this host of where those bytes went.
rm -f "$R7/archive/segments/2026-08-14-0000.jsonl.gz"
out="$(run "$R7" --list)"
has "list: names a retired segment" "$out" "RETIRED"

# 10. --restore fetches and VERIFIES: the digest against the receipt, and the
#     gzip stream itself, because a gz that hashes right and will not decompress
#     is still no record.
R8="$(newstate restore 2026-08-14-0000)"
MODE=ok run "$R8" >/dev/null
mv "$R8/archive/segments/2026-08-14-0000.jsonl.gz" "$R8/2026-08-14-0000.jsonl.gz"
MV_CC_SRC="$R8" MODE=ok out="$(MV_ENV_FILE="$R8/deploy.env" MV_CC_CALLS="$R8/calls.txt" \
  MV_CC_SRC="$R8" MV_CC_MODE=ok "$CC" --restore 2026-08-14-0000.jsonl.gz 2>&1)"
has "restore: says it verified" "$out" "sha256 verified"
if [ -e "$R8/archive/segments/2026-08-14-0000.jsonl.gz" ]; then
  pass "restore: the segment is back in place"
else
  fail "restore: the segment was not put back" "$out"
fi

R9="$(newstate restore-bad 2026-08-14-0000)"
MODE=ok run "$R9" >/dev/null
mv "$R9/archive/segments/2026-08-14-0000.jsonl.gz" "$R9/2026-08-14-0000.jsonl.gz"
out="$(MV_ENV_FILE="$R9/deploy.env" MV_CC_CALLS="$R9/calls.txt" MV_CC_SRC="$R9" \
  MV_CC_MODE=corrupt-restore "$CC" --restore 2026-08-14-0000.jsonl.gz 2>&1)"; rc=$?
eq "restore: a corrupt fetch fails" "$rc" "1"
if [ -e "$R9/archive/segments/2026-08-14-0000.jsonl.gz" ]; then
  fail "restore: a corrupt object was put into place as a segment"
else
  pass "restore: a corrupt object is NOT put into place"
fi

# 11. It never touches the live file or a plain segment: only *.jsonl.gz leaves
#     this host, because only a verified gz can carry a receipt.
R10="$(newstate scope 2026-08-14-0000)"
printf '{"type":"MIGRATION"}\n' > "$R10/archive/migrations.jsonl"
printf '{"type":"MIGRATION"}\n' > "$R10/archive/segments/2026-08-15-0000.jsonl"
MODE=ok run "$R10" >/dev/null
if grep -q 'migrations.jsonl' "$R10/calls.txt" || grep -qE '2026-08-15-0000\.jsonl( |$)' "$R10/calls.txt"; then
  fail "scope: the live file or a plain segment was uploaded" "$(cat "$R10/calls.txt")"
else
  pass "scope: only compressed closed segments are uploaded"
fi
eq "scope: exactly one receipt" "$(receipts "$R10")" "1"

# 12. Ordering: the legacy segment holds the OLDEST records and must be copied
#     first, even though "legacy-" sorts after a digit.
R11="$(newstate order legacy-2026-07-01-to-2026-08-13-0000 2026-08-14-0000)"
MODE=ok run "$R11" >/dev/null
first="$(head -1 "$R11/calls.txt")"
has "order: the legacy segment is copied first" "$first" "legacy-2026-07-01"

printf '\n%d passed, %d failed\n' "$PASS" "$FAIL"
[ "$FAIL" = 0 ]
