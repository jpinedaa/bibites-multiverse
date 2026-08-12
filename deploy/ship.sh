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
# BOTH ARCHITECTURES, every time. Lightsail sells x86 and ARM bundles and the
# bundle is not chosen yet; two 10 MB binaries cost nothing to carry and
# provision.sh picks by `uname -m` on the far side. If the bundle is settled,
# MV_ARCHS=amd64 halves the build.
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

step "build"
say "go $(go version | awk '{print $3}')  from $ROOT/go"
mkdir -p "$STAGE"
# nice -n 19, because this machine is also the living deployment. Unniced load on
# this host reproduces the sidecar session storm — on record twice, once as an
# accident and once as a deliberate reproduction that dropped seven sidecar
# sessions and three mods. A cross-compile is not a test suite, but the rule is
# about the host and not about the workload.
#
# The output goes to deploy/stage/ and NEVER to bin/: five running sidecars hold
# bin/sidecar open, and an in-place build there fails with ETXTBSY.
for arch in $ARCHS; do
  for cmd in relay archive ringstat; do
    out="$STAGE/${cmd}-linux-${arch}"
    ( cd "$ROOT/go" && CGO_ENABLED=0 GOOS=linux GOARCH="$arch" \
        nice -n 19 go build -trimpath -o "$out" "./cmd/$cmd" )
    say "$(basename "$out")  $(stat -c %s "$out" 2>/dev/null || echo ?) bytes"
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
