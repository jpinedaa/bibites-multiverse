#!/usr/bin/env bash
# Build the two binaries on the DEVELOPMENT machine and put them on the instance.
#
# This is the only script in the kit that runs anywhere but the instance. It
# exists because the instance has no Go toolchain and must not get one: the
# relay is a single static binary with one non-stdlib dependency and no cgo, so
# it cross-compiles without ceremony, and a build toolchain on a public host is
# an attack surface bought for nothing.
#
#   deploy/ship.sh                 build amd64 + arm64, scp to the instance
#   deploy/ship.sh --build-only    build into deploy/stage/, send nothing
#   deploy/ship.sh --host 1.2.3.4  override the destination
#
# THE BUILD RULE IS THE REPOSITORY'S, not a new one: `CGO_ENABLED=0 go build`
# from go/, exactly as e2e/run-m3.sh does it. -trimpath is added so the shipped
# binary carries no path from this machine.
#
# Build both supported architectures by default. The provisioning script selects
# the correct artifact with `uname -m`. Set MV_ARCHS to limit the build.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/.." && pwd)"
STAGE="$HERE/stage"
ARCHS="${MV_ARCHS:-amd64 arm64}"
BUILD_ONLY=0

# The instance. SSH_HOST is what `scp` and `ssh` are given, so a ~/.ssh/config
# alias works here and is the tidier answer than an IP in a shell variable.
SSH_HOST="${MV_SSH_HOST:-}"
SSH_USER="${MV_SSH_USER:-ubuntu}"
REMOTE_STAGE="${MV_STAGE_DIR:-/home/ubuntu/multiverse-stage}"

say()  { printf '     %s\n' "$*"; }
step() { printf '\n==== %s\n' "$*"; }
die()  { printf '\nSTOP: %s\n' "$*" >&2; exit 1; }

while [ $# -gt 0 ]; do
  case "$1" in
    --build-only) BUILD_ONLY=1 ;;
    --host) SSH_HOST="${2:?--host needs a value}"; shift ;;
    --user) SSH_USER="${2:?--user needs a value}"; shift ;;
    -h|--help) sed -n '2,26p' "$0"; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
  shift
done

command -v go >/dev/null || die "no go on PATH"

# WHAT THIS TREE IS ON. `go build` stamps vcs.revision into the binary from the
# repository it builds in, and that stamp is the only thing on the far side that
# says WHICH SOURCE a running binary came from. Every deployment record is
# written against it. So it is asserted here, at the point of creation, rather
# than discovered later.
#
# On 2026-08-17 a deployment installed nothing and reported success: stale
# artifacts from an earlier build sat under the canonical staged names, and the
# install phase compared stale-against-installed, found them equal, and said so
# truthfully about the wrong files. That class of defect — a tool reporting
# success about the wrong file — is caught cheaply here and expensively anywhere
# else.
EXPECT_REV=""
if command -v git >/dev/null && ( cd "$ROOT" && git rev-parse --git-dir >/dev/null 2>&1 ); then
  EXPECT_REV="$(cd "$ROOT" && git rev-parse HEAD)"
fi

# Read the stamp with `go version -m`, never with `strings`. `strings` finds the
# revision of ANY commit whose hash happens to be embedded in the binary, which
# on a Go build includes module versions and is not the build's own answer.
stamp_of() {
  go version -m "$1" 2>/dev/null | sed -n "s/^[[:space:]]*build[[:space:]]\+$2=//p" | head -1
}

assert_revision() {
  local out="$1" rev modified
  rev="$(stamp_of "$out" vcs.revision)"
  modified="$(stamp_of "$out" vcs.modified)"
  if [ -z "$rev" ]; then
    say "  WARNING: $(basename "$out") carries NO vcs.revision stamp."
    say "           A build inside a git worktree, or one with -buildvcs=false,"
    say "           produces an artifact nothing downstream can identify. Do not"
    say "           ship it without recording where it came from by hand."
    return 0
  fi
  if [ -n "$EXPECT_REV" ] && [ "$rev" != "$EXPECT_REV" ]; then
    die "$(basename "$out") is stamped $rev but this tree is on $EXPECT_REV.
     The artifact is NOT this source. Remove $STAGE and build again."
  fi
  if [ "$modified" = true ]; then
    say "  WARNING: $(basename "$out") is stamped vcs.modified=true."
    say "           It was built from a dirty tree, so its revision names a"
    say "           commit whose content is not what was compiled. A deployment"
    say "           record that cites that revision would be wrong."
  fi
  return 0
}

step "build"
say "go $(go version | awk '{print $3}')  from $ROOT/go"
if [ -n "$EXPECT_REV" ]; then say "tree at $EXPECT_REV"; fi
mkdir -p "$STAGE"
# Use low scheduling priority because a development host can also run test worlds.
# Write artifacts to deploy/stage/. A running process can keep a binary in bin/
# open, which makes an in-place replacement fail with ETXTBSY.
for arch in $ARCHS; do
  for cmd in relay archive ringstat; do
    out="$STAGE/${cmd}-linux-${arch}"
    ( cd "$ROOT/go" && CGO_ENABLED=0 GOOS=linux GOARCH="$arch" \
        nice -n 19 go build -trimpath -o "$out" "./cmd/$cmd" )
    say "$(basename "$out")  $(stat -c %s "$out" 2>/dev/null || echo ?) bytes"
    assert_revision "$out"
  done
done

( cd "$STAGE" && sha256sum ./*-linux-* > SHA256SUMS )
step "checksums"
sed 's/^/     /' "$STAGE/SHA256SUMS"

# A static binary is the claim; this is the check. A cgo build would say
# "dynamically linked" here and would then fail on a different libc.
if command -v file >/dev/null; then
  step "linkage"
  file "$STAGE"/relay-linux-* | sed 's/^/     /'
  file "$STAGE"/relay-linux-* | grep -q 'statically linked' \
    || die "the relay did not build statically. CGO_ENABLED=0 is the whole requirement."
fi

if [ "$BUILD_ONLY" = 1 ]; then
  step "done — nothing was sent"
  say "artifacts in $STAGE"
  exit 0
fi

[ -n "$SSH_HOST" ] || die "no destination. Pass --host <ip-or-alias>, or set MV_SSH_HOST.
     The kit never guesses a host: there is exactly one instance and sending
     binaries to the wrong one is not a recoverable mistake."

step "ship to $SSH_USER@$SSH_HOST:$REMOTE_STAGE"
ssh "$SSH_USER@$SSH_HOST" "mkdir -p '$REMOTE_STAGE'"
scp "$STAGE"/*-linux-* "$STAGE/SHA256SUMS" "$SSH_USER@$SSH_HOST:$REMOTE_STAGE/"
say "verifying on the far side"
ssh "$SSH_USER@$SSH_HOST" "cd '$REMOTE_STAGE' && sha256sum -c SHA256SUMS" | sed 's/^/     /'

cat <<EOF

Shipped. On the instance:

    sudo /opt/multiverse/deploy/provision.sh --only binaries

  installs whichever architecture that box is, and RECORDS THE CHECKSUMS at
  /etc/multiverse/BINARIES.sha256. It does NOT restart anything: a new binary on
  disk is not a running new binary, and the restart is a deliberate act with a
  policy behind it. Read RESTART-POLICY.md, then:

    sudo systemctl restart multiverse-relay     # seconds
    sudo systemctl restart multiverse-archive   # a REPLAY. Read the policy first.
EOF
