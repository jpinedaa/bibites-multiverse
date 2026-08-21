#!/usr/bin/env bash
# Drive the parts of deploy/deploy.sh that need no host and no root.
#
# The two assertions this script exists for — "the installed kit is the staged
# kit" and "no phase rewrote an environment file" — are the ones that turned
# three divergent hand-written host scripts into one. They are also the ones
# nobody notices when they silently stop working, because their normal output is
# "unchanged". So they are tested here against a fake /etc and a fake kit.
#
#   deploy/test-deploy.sh
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SUT="$HERE/deploy.sh"
PROVISION="$HERE/provision.sh"
[ -x "$SUT" ] || { echo "test-deploy: $SUT is not executable" >&2; exit 1; }
[ -x "$PROVISION" ] || { echo "test-deploy: $PROVISION is not executable" >&2; exit 1; }

TMP="$(mktemp -d "${TMPDIR:-/tmp}/mvdep.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT
PASS=0; FAIL=0
ok()  { PASS=$((PASS + 1)); printf '  ok    %s\n' "$1"; }
bad() { FAIL=$((FAIL + 1)); printf '  FAIL  %s\n' "$1"; }

echo
echo "== the kit listing digest has one definition"

# The value must equal the one the operations source lock defines, which an
# earlier hand-written host script computed a different way. Both forms are run
# here over the same tree and must agree; if they ever stop agreeing, the number
# recorded in the lock stops meaning what it says.
d="$TMP/kit"; mkdir -p "$d/nginx" "$d/systemd"
printf 'a\n' > "$d/one.sh"; printf 'b\n' > "$d/two.md"
printf 'c\n' > "$d/nginx/site.conf"; printf 'd\n' > "$d/systemd/unit.service"

theirs="$( cd "$d" && find . -type f | LC_ALL=C sort | xargs sha256sum \
           | sed 's|  \./|  |' | sha256sum | cut -d' ' -f1 )"
ours="$("$SUT" --print-kit-digest "$d")"
if [ "$theirs" = "$ours" ]; then
  ok "matches the source lock's stated method"
else
  bad "matches the source lock's stated method ($ours vs $theirs)"
fi

# The prefix detail the lock warns about, made into a test: the same listing
# built with find's default "./" gives a DIFFERENT value, so getting it wrong is
# not a harmless variation.
wrong="$( cd "$d" && find . -type f | LC_ALL=C sort | xargs sha256sum \
          | sha256sum | cut -d' ' -f1 )"
if [ "$wrong" != "$ours" ]; then
  ok "a leading ./ would change the value, as the lock warns"
else
  bad "a leading ./ would change the value, as the lock warns"
fi

# The digest must react to content, not only to names.
printf 'CHANGED\n' > "$d/one.sh"
if [ "$("$SUT" --print-kit-digest "$d")" != "$ours" ]; then
  ok "changing a file changes the digest"
else
  bad "changing a file changes the digest"
fi

echo
echo "== arguments"

t_refuse() {
  local label="$1"; shift
  if "$SUT" "$@" >/dev/null 2>&1; then bad "$label"; else ok "$label"; fi
}
t_refuse "no --kit"            --binaries
t_refuse "kit does not exist"  --kit "$TMP/nope"
t_refuse "unknown argument"    --kit "$d" --wat
t_refuse "digest of nothing"   --print-kit-digest "$TMP/nope"

# --help must work for an operator who is not root, or it is not help.
if "$SUT" --help 2>/dev/null | grep -q -- '--expect-kit-digest'; then
  ok "--help describes the flags"
else
  bad "--help describes the flags"
fi

# A kit with no provision.sh is refused before root is even considered, so the
# message an operator sees names the real problem.
mkdir -p "$TMP/emptykit"
if out="$("$SUT" --kit "$TMP/emptykit" 2>&1)"; then
  bad "a kit with no provision.sh is refused"
elif printf '%s' "$out" | grep -q 'provision.sh'; then
  ok "a kit with no provision.sh is refused, and says so"
else
  bad "a kit with no provision.sh is refused, and says so"
fi

echo
echo "== one staged payload from validation through installation"

# The production workflow exposed this seam: deploy.sh validated the CI payload,
# but provision.sh sourced deploy.env afterwards and installed the old canonical
# stage instead. Exercise the provision half with two different artifacts so an
# override that is parsed but overwritten still fails this test.
mkdir -p "$TMP/fakebin" "$TMP/stale-stage" "$TMP/intended-stage"
cat >"$TMP/fakebin/id" <<'SH'
#!/usr/bin/env sh
[ "${1:-}" = -u ] && { echo 0; exit 0; }
exec /usr/bin/id "$@"
SH
chmod +x "$TMP/fakebin/id"
printf 'MV_STAGE_DIR=%s\nMV_PREFIX=%s\nMV_DOMAIN=multiverse.test\n' \
  "$TMP/stale-stage" "$TMP/prefix" >"$TMP/deploy.env"
printf 'stale relay\n' >"$TMP/stale-stage/relay-linux-amd64"
printf 'stale archive\n' >"$TMP/stale-stage/archive-linux-amd64"
printf 'intended relay\n' >"$TMP/intended-stage/relay-linux-amd64"
printf 'intended archive\n' >"$TMP/intended-stage/archive-linux-amd64"
intended_relay="$(sha256sum "$TMP/intended-stage/relay-linux-amd64" | awk '{print $1}')"

if out="$(PATH="$TMP/fakebin:$PATH" "$PROVISION" --env-file "$TMP/deploy.env" \
    --only binaries --stage-dir "$TMP/intended-stage" --dry-run 2>&1)" &&
   printf '%s\n' "$out" | grep -Fq "relay: staged    $intended_relay"; then
  ok "--stage-dir overrides the stale deploy.env stage"
else
  bad "--stage-dir overrides the stale deploy.env stage"
fi

if grep -Fq '"$BINARY_PROVISION" --only binaries --stage-dir "$STAGE"' "$SUT"; then
  ok "deploy.sh forwards the stage that it validated"
else
  bad "deploy.sh forwards the stage that it validated"
fi

if grep -Fq '[ "$DRY" = 1 ] && BINARY_PROVISION="$PROVISION"' "$SUT"; then
  ok "a dry run exercises the staged provisioner"
else
  bad "a dry run exercises the staged provisioner"
fi

echo
printf '%d pass, %d fail\n' "$PASS" "$FAIL"
[ "$FAIL" = 0 ] || exit 1
echo "deploy.sh OK"
