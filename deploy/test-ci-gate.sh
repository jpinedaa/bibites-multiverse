#!/usr/bin/env bash
# Drive deploy/ci-gate.sh through its read seams on a workstation: no host, no
# root, no ssh, no systemd.
#
# WHAT THIS FILE IS FOR. ci-gate.sh is the only thing standing between a key that
# lives in GitHub and an account with `(ALL) NOPASSWD: ALL` on the production
# host. Its refusals are therefore not error handling — they are the product.
# A test suite that only checked the happy path would prove the least important
# half. Most of what follows is an attempt to get past it.
#
#   deploy/test-ci-gate.sh
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GATE="$HERE/ci-gate.sh"
[ -x "$GATE" ] || { echo "test-ci-gate: $GATE is not executable" >&2; exit 1; }

TMP="$(mktemp -d "${TMPDIR:-/tmp}/mvgate.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT

PASS=0
FAIL=0

# ---------------------------------------------------------------- the fixture
#
# A fake kit root with stub scripts, a recorder standing in for sudo, and an
# inbox holding one well-formed kit. Everything the gate can reach is inside
# $TMP, so a test that gets past the gate damages nothing.

KIT_ROOT="$TMP/opt/deploy"
INBOX="$TMP/ci-kits"
STAGE="$TMP/stage"
GATELOG="$TMP/ci-gate.log"
HOLD="$TMP/HOLD-README"
# MUST be seamed. The default is the REAL /run/lock/bibites-archive-deploy.lock,
# and a test suite that reaches for the production deploy lock is a test suite
# that can interfere with a live deployment.
LOCKFILE="$TMP/deploy.lock"
RECORD="$TMP/record"

mkdir -p "$KIT_ROOT" "$INBOX" "$STAGE"

# The recorder stands in for sudo: it writes the command it was given and then
# runs it, so a test can assert both "what would have run as root" and the real
# effect of the stub.
cat >"$TMP/record-sudo" <<'EOF'
#!/usr/bin/env bash
# The gate writes its own audit log through sudo too. That write is bookkeeping,
# not a deploy action, so it is excluded here — otherwise every refusal would
# look like "something ran" and the assertion below would prove nothing.
case "$*" in
  *ci-gate.log*) exec "$@" ;;
esac
printf '%s\n' "$*" >> "$RECORD"
exec "$@"
EOF
chmod +x "$TMP/record-sudo"

for s in provision.sh deploy.sh restart-relay.sh restart-archive.sh; do
  cat >"$KIT_ROOT/$s" <<EOF
#!/usr/bin/env bash
printf 'STUB %s %s\n' "$s" "\$*" >> "$RECORD"
exit 0
EOF
  chmod +x "$KIT_ROOT/$s"
done

# One well-formed kit in the inbox: a deploy/ tree with its own provision.sh
# (that is what kit-install runs) and a stage/ tree with matching checksums.
GOODKIT="$INBOX/20260816T000000Z-abc1234"
mkdir -p "$GOODKIT/deploy" "$GOODKIT/stage"
cp "$KIT_ROOT/provision.sh" "$GOODKIT/deploy/provision.sh"
cp "$KIT_ROOT/deploy.sh"    "$GOODKIT/deploy/deploy.sh"
printf 'relay\n' > "$GOODKIT/stage/relay-linux-amd64"
( cd "$GOODKIT/stage" && sha256sum ./relay-linux-amd64 > SHA256SUMS )

# A kit whose staged binaries do NOT match their own checksum file.
BADKIT="$INBOX/20260816T000000Z-bad0000"
mkdir -p "$BADKIT/deploy" "$BADKIT/stage"
cp "$KIT_ROOT/provision.sh" "$BADKIT/deploy/provision.sh"
cp "$KIT_ROOT/deploy.sh"    "$BADKIT/deploy/deploy.sh"
printf 'relay\n' > "$BADKIT/stage/relay-linux-amd64"
( cd "$BADKIT/stage" && sha256sum ./relay-linux-amd64 > SHA256SUMS )
printf 'tampered\n' > "$BADKIT/stage/relay-linux-amd64"

run_gate() {
  RECORD="$RECORD" \
  SSH_ORIGINAL_COMMAND="$1" \
  MV_CI_KIT_ROOT="$KIT_ROOT" \
  MV_CI_KIT_INBOX="$INBOX" \
  MV_CI_STAGE_DIR="$STAGE" \
  MV_CI_GATE_LOG="$GATELOG" \
  MV_CI_HOLD="$HOLD" \
  MV_CI_LOCK="$LOCKFILE" \
  MV_CI_SUDO="$TMP/record-sudo" \
  MV_CI_NOW="2026-08-16T12:00:00Z" \
  "$GATE" >"$TMP/out" 2>"$TMP/err" </dev/null || return $?
}

ok()  { PASS=$((PASS + 1)); printf '  ok    %s\n' "$1"; }
bad() { FAIL=$((FAIL + 1)); printf '  FAIL  %s\n' "$1"; [ -s "$TMP/err" ] && sed 's/^/          /' "$TMP/err"; }

# allow <label> <command> <substring the recorder must contain>
allow() {
  local label="$1" cmd="$2" want="$3" rc=0
  : > "$RECORD"
  run_gate "$cmd" || rc=$?
  if [ "$rc" = 3 ]; then bad "$label — refused, should have been allowed"; return; fi
  if ! grep -qF -- "$want" "$RECORD" 2>/dev/null; then
    bad "$label — recorder does not contain: $want"
    sed 's/^/          recorded: /' "$RECORD" 2>/dev/null
    return
  fi
  ok "$label"
}

# refuse <label> <command> [substring the refusal message must contain]
#
# A refusal is three things at once and all three are asserted: exit 3, a message
# naming the reason, and NO STUB HAVING RUN. The last is the one that matters — a
# gate that refuses after it has already run the command refuses nothing.
#
# The test is for a `STUB` line, which only a stub script writes when it actually
# executes, rather than for any recorder line at all. Those are different: when
# the deploy lock is already held, `flock` is invoked and refuses, so the
# recorder shows the attempt while provision.sh never ran. Asserting on the
# attempt would call that a failure, and it is precisely the behavior wanted.
refuse() {
  local label="$1" cmd="$2" want="${3:-}" rc=0
  : > "$RECORD"
  run_gate "$cmd" || rc=$?
  if [ "$rc" != 3 ]; then bad "$label — exit $rc, expected 3 (refusal)"; return; fi
  if grep -q '^STUB ' "$RECORD" 2>/dev/null; then
    bad "$label — refused but SOMETHING RAN"
    sed 's/^/          recorded: /' "$RECORD"
    return
  fi
  if [ -n "$want" ] && ! grep -qF -- "$want" "$TMP/err"; then
    bad "$label — refusal message does not mention: $want"
    sed 's/^/          said: /' "$TMP/err"
    return
  fi
  ok "$label"
}

echo
echo "== allowed verbs reach the stub"

allow "verify"                  "verify"                                  "provision.sh --only verify"
allow "verify --dry-run"        "verify --dry-run"                        "provision.sh --only verify --dry-run"
allow "phase directories"       "phase directories"                       "provision.sh --only directories"
allow "phase binaries"          "phase binaries"                          "provision.sh --only binaries"
allow "phase nginxfront"        "phase nginxfront"                        "provision.sh --only nginxfront"
allow "phase streamorigin"      "phase streamorigin"                      "provision.sh --only streamorigin"
allow "phase … --dry-run"       "phase directories --dry-run"             "provision.sh --only directories --dry-run"
allow "kit-install"             "kit-install $(basename "$GOODKIT")"      "$GOODKIT/deploy/deploy.sh --kit $GOODKIT"
allow "binaries-install"        "binaries-install $(basename "$GOODKIT")" "deploy.sh --kit $GOODKIT --stage $GOODKIT/stage --binaries"
allow "kit-install, digest"     "kit-install $(basename "$GOODKIT") deadbeef" "--expect-kit-digest deadbeef"
allow "restart-relay"           "restart-relay"                           "restart-relay.sh"
allow "restart-relay --dry-run" "restart-relay --dry-run"                 "restart-relay.sh --dry-run"
allow "restart-archive dry-run" "restart-archive --dry-run"               "restart-archive.sh --dry-run"

echo
echo "== the archive's real restart is not offered at all"

refuse "restart-archive (real)" "restart-archive" "not offered through CI"

echo
echo "== phases that are not on the list"

refuse "phase tls"        "phase tls"        "not offered through CI"
refuse "phase bootstrap"  "phase bootstrap"  "not offered through CI"
refuse "phase systemd"    "phase systemd"    "not offered through CI"
refuse "phase firewall"   "phase firewall"   "not offered through CI"
refuse "phase packages"   "phase packages"   "not offered through CI"
refuse "phase swap"       "phase swap"       "not offered through CI"
refuse "phase envfiles"   "phase envfiles"   "not offered through CI"
refuse "phase nonesuch"   "phase nonesuch"   "not offered through CI"

echo
echo "== everything that is not a verb"

refuse "empty command"    ""                 "no command"
refuse "a shell"          "bash"             "not an allowed verb: bash"
refuse "rm -rf /"         "rm -rf /"         "not an allowed verb: rm"
refuse "sudo -i"          "sudo -i"          "not an allowed verb: sudo"
refuse "scp"              "scp -t /tmp"      "not an allowed verb: scp"
refuse "rsync"            "rsync --server ." "not an allowed verb: rsync"
refuse "systemctl"        "systemctl restart multiverse-archive" "not an allowed verb: systemctl"

echo
echo "== the command is split into words, never interpreted as shell syntax"
#
# Each of these is a real attempt at the boundary. If ci-gate.sh ever grows an
# eval, a `sh -c`, or an unquoted expansion into a command position, one of them
# starts passing and this file starts failing.

refuse "semicolon"        "verify; rm -rf /"     "not an allowed verb: verify;"
refuse "&& chain"         "verify && id"         "verify takes no argument"
refuse "pipe"             "verify | sh"          "verify takes no argument"
refuse 'command sub'      'verify $(id)'         "verify takes no argument"
refuse 'backticks'        'verify `id`'          "verify takes no argument"
# A newline must be REFUSED, not silently truncated. `read -ra` stops at the
# first one, so without an explicit check the gate would run `verify` and drop
# the rest without saying so — and a gate whose log does not match what it did
# is not auditable.
refuse "newline"          "$(printf 'verify\nid')" "newline or a control character"
refuse "carriage return"  "$(printf 'verify\rid')" "newline or a control character"
refuse "escape sequence"  "$(printf 'verify\033[2J')" "newline or a control character"
refuse "unknown flag"     "verify --force"       "unknown flag"
refuse "extra argument"   "receipt now"          "receipt takes no argument"
refuse "phase, two names" "phase verify binaries" "phase takes exactly one"

echo
echo "== the kit inbox cannot be escaped"

refuse "traversal"        "kit-install ../../etc"      "not a plain path"
refuse "absolute path"    "kit-install /etc"           "outside"
refuse "absent kit"       "kit-install 20260101T000000Z-0000000" "no such kit directory"
refuse "glob"             "kit-install *"              "not a plain path"
refuse "kit-receive junk" "kit-receive not-a-sha"      "git revision in hex"
refuse "kit-receive dots" "kit-receive ../../etc"      "plain revision token"

echo
echo "== a kit that does not match its own checksums is refused"

# The staged-checksum check now lives in deploy.sh, which the stub replaces, so
# what is asserted here is that the gate ROUTES to it with the right arguments
# rather than doing the check itself.
allow "binaries route to deploy.sh" "binaries-install $(basename "$BADKIT")" "--binaries"
refuse "digest not a token" "kit-install $(basename "$GOODKIT") a;b" "not a plain token"

echo
echo "== an archive deploy hold stops every mutating verb"

: > "$HOLD"
refuse "hold: kit-install"      "kit-install $(basename "$GOODKIT")"      "hold is in place"
refuse "hold: binaries-install" "binaries-install $(basename "$GOODKIT")" "hold is in place"
refuse "hold: phase binaries"   "phase binaries"                          "hold is in place"
refuse "hold: restart-relay"    "restart-relay"                           "hold is in place"
refuse "hold: kit-receive"      "kit-receive abc1234"                     "hold is in place"
# Read-only verbs stay available on purpose: a daily verify that goes red because
# a person is holding a lock is noise, and noise is how a check stops being read.
allow  "hold: verify still runs"      "verify"                    "provision.sh --only verify"
allow  "hold: dry runs still allowed" "phase binaries --dry-run"  "provision.sh --only binaries --dry-run"
rm -f "$HOLD"

echo
echo "== mutations take the SAME deploy lock the hand-run scripts take"
#
# This is the consistency claim the whole workflow rests on. /home/ubuntu/deploy-*.sh
# run under `sudo flock -n /run/lock/bibites-archive-deploy.lock`. If CI took a
# different lock, or none, the two paths would serialize against nothing — and
# the first day CI is used is the first day a second actor exists.

allow "kit-install locks"      "kit-install $(basename "$GOODKIT")"      "flock -n -E 75 $LOCKFILE"
allow "binaries-install locks" "binaries-install $(basename "$GOODKIT")" "flock -n -E 75 $LOCKFILE"
allow "phase binaries locks"   "phase binaries"                          "flock -n -E 75 $LOCKFILE"

# Read-only and rehearsal paths must NOT take it: a dry run that blocks on a
# real deployment's lock cannot be used to check on that deployment.
: > "$RECORD"
run_gate "verify" >/dev/null 2>&1 || true
if grep -qF -- "flock" "$RECORD"; then
  bad "verify must not take the deploy lock"
else
  ok "verify does not take the deploy lock"
fi

: > "$RECORD"
run_gate "phase binaries --dry-run" >/dev/null 2>&1 || true
if grep -qF -- "flock" "$RECORD"; then
  bad "a dry run must not take the deploy lock"
else
  ok "a dry run does not take the deploy lock"
fi

# A lock already held by somebody else stops the deployment, and says so with its
# own reason rather than looking like a failed provision run.
exec 8>"$LOCKFILE"
flock -n 8 || { echo "test-ci-gate: could not take the fixture lock" >&2; exit 1; }
refuse "lock already held" "phase binaries" "another deployment holds"
exec 8>&-

echo
echo "== restart-relay is handed a reason, assembled from constants plus one token"

allow "reason, no tag"  "restart-relay --dry-run"          "--reason CI"
allow "reason, tagged"  "restart-relay --dry-run 12345"    "--reason CI run 12345"
refuse "reason injected" "restart-relay --dry-run a;b"     "not a plain word"
refuse "two tags"        "restart-relay --dry-run 1 2"     "at most one run tag"
# restart-relay.sh manages its own lock and reads the hold itself, so CI must not
# wrap it in the archive deploy lock as well.
: > "$RECORD"
run_gate "restart-relay --dry-run" >/dev/null 2>&1 || true
if grep -qF -- "flock" "$RECORD"; then
  bad "restart-relay must not be wrapped in the archive deploy lock"
else
  ok "restart-relay is not wrapped in the archive deploy lock"
fi

echo
echo "== every invocation is logged, allowed and refused alike"

: > "$GATELOG"
run_gate "verify" >/dev/null 2>&1 || true
run_gate "rm -rf /" >/dev/null 2>&1 || true

# The format is a contract: UTC timestamp, verdict, then the command. An operator
# asking "what did CI do to this box, and when" reads this file and nothing else.
if grep -Eq '^2026-08-16T12:00:00Z ALLOW verify$' "$GATELOG"; then
  ok "ALLOW line format"
else
  bad "ALLOW line format"; sed 's/^/          /' "$GATELOG"
fi

if grep -Eq '^2026-08-16T12:00:00Z REFUSE not an allowed verb: rm$' "$GATELOG"; then
  ok "REFUSE line format"
else
  bad "REFUSE line format"; sed 's/^/          /' "$GATELOG"
fi

if [ "$(wc -l < "$GATELOG")" = 2 ]; then
  ok "one line per invocation"
else
  bad "one line per invocation — got $(wc -l < "$GATELOG")"; sed 's/^/          /' "$GATELOG"
fi

# A refused command must not be able to forge a log line or smuggle an escape
# sequence into the file an operator reads. Two layers have to hold: the
# control-character refusal above, and log()'s own `tr -d` for anything that
# reaches it by another route.
: > "$GATELOG"
run_gate "$(printf 'evil\033[2Jthing')" >/dev/null 2>&1 || true
run_gate "$(printf 'evil\nALLOW forged')" >/dev/null 2>&1 || true
if [ "$(wc -l < "$GATELOG")" = 2 ] \
   && ! grep -q "$(printf '\033')" "$GATELOG" \
   && ! grep -q '^ALLOW forged' "$GATELOG"; then
  ok "no control character or forged line reaches the log"
else
  bad "no control character or forged line reaches the log"; sed 's/^/          /' "$GATELOG"
fi

# The log is capped per line. A caller must not be able to push an operator's
# terminal around with a kilobyte of text.
: > "$GATELOG"
run_gate "$(printf 'x%.0s' $(seq 1 500))" >/dev/null 2>&1 || true
if [ "$(wc -L < "$GATELOG")" -le 240 ]; then
  ok "log lines are bounded"
else
  bad "log lines are bounded — longest is $(wc -L < "$GATELOG")"
fi

echo
printf '%d pass, %d fail\n' "$PASS" "$FAIL"
[ "$FAIL" = 0 ] || exit 1
echo "ci-gate OK"
