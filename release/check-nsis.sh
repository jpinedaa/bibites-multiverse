#!/usr/bin/env bash
# Compile release/windows-installer.nsi against a stub payload.
#
# The installer script is only ever exercised as the last step of a full
# "complete edition" build, which needs the proprietary game bytes. This
# compiles the same file against a one-file stub package, so a syntax error, a
# missing !include, a malformed VIProductVersion, or a renamed
# File/CreateShortCut path is caught on every pull request in about two seconds
# and without a single game file.
#
# Usage:
#   release/check-nsis.sh
#
# Environment:
#   MAKENSIS         path to makensis; default: the one on PATH
#   NSISDIR          the NSIS data directory (Include/, Stubs/, Plugins/).
#                    Optional: an installed makensis finds its own. Required
#                    for a makensis unpacked into an arbitrary prefix.
#   PRODUCT_VERSION  the version string to compile with. Default: whatever
#                    release/bump-version.sh --print reports, so this file
#                    never holds a version literal of its own.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(cd "$HERE/.." && pwd)"

note() { printf '    %s\n' "$*"; }
die()  { printf '\n!! check-nsis: %s\n' "$*" >&2; exit 1; }

NSI="$REPO/release/windows-installer.nsi"
ICON="$REPO/release/kit/bibites-multiverse.ico"
[ -f "$NSI" ]  || die "missing $NSI"
[ -f "$ICON" ] || die "missing $ICON"

MAKENSIS="${MAKENSIS:-$(command -v makensis || true)}"
[ -n "$MAKENSIS" ] && [ -x "$MAKENSIS" ] \
  || die "no makensis. Install NSIS 3.09 or newer, or set MAKENSIS to its path."

# One machine-readable source for the release string: release/bump-version.sh
# reads release/make-release.sh's RELEASE= line, so this file never carries a
# version literal of its own. Whether that value is RIGHT is the consistency
# job's question, not this one's; what matters here is that the installer
# compiles with the real string, because VIProductVersion rejects a malformed
# one.
VERSION="${PRODUCT_VERSION:-}"
if [ -z "$VERSION" ]; then
  [ -x "$REPO/release/bump-version.sh" ] \
    || die "missing $REPO/release/bump-version.sh; set PRODUCT_VERSION to compile anyway"
  VERSION="$("$REPO/release/bump-version.sh" --print)"
fi
case "$VERSION" in
  *[!0-9.]*|''|.*|*.) die "the release string $VERSION is not dotted digits" ;;
esac

WORK="$(mktemp -d "${TMPDIR:-/tmp}/check-nsis.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT
STUB="$WORK/package"
OUT="$WORK/setup-check.exe"
mkdir -p "$STUB"
printf 'stub\n' > "$STUB/stub.txt"

# NSISDIR is exported rather than passed, because makensis reads it from the
# environment to find Include/, Stubs/ and Plugins/.
if [ -n "${NSISDIR:-}" ]; then
  [ -d "$NSISDIR" ] || die "NSISDIR=$NSISDIR is not a directory"
  export NSISDIR
fi

# Absolute paths throughout: makensis resolves a relative -D value against its
# own working directory, not against the .nsi.
"$MAKENSIS" -V2 \
  -DPRODUCT_VERSION="$VERSION" \
  -DPACKAGE_DIR="$STUB" \
  -DOUTPUT_FILE="$OUT" \
  -DPRODUCT_ICON="$ICON" \
  "$NSI" \
  || die "makensis refused $NSI"

[ -f "$OUT" ] || die "makensis reported success but wrote no $OUT"

DESC="$(file -b "$OUT")"
printf '%s\n' "$DESC" | grep -q 'Nullsoft Installer self-extracting archive' \
  || die "the compiled output is not an NSIS installer: $DESC"

note "release/windows-installer.nsi compiles at $VERSION"
note "$DESC"
