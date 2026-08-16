#!/usr/bin/env bash
# Print the "testedBuild" block for THIS tree, ready to paste into
# docs/support-matrix.md.
#
# WHAT IT IS FOR. The release reference is a record, not a binary: after a real
# test of a plugin and a sidecar, the person who ran the test writes down what
# they ran. This script computes the four values nobody should type by hand —
# the plugin's sha256, the bibites-mod/ tree hash, HEAD, and the digest of
# cmd/sidecar's input manifest — and prints the whole block with them in it.
# release/make-release.sh and release/check-drift.sh then refuse anything else.
#
# IT PROVES NOTHING ABOUT A TEST, and it cannot. It reads the plugin the last
# `dotnet build -c Release` produced and the source in this checkout. Running
# it is the LAST step of a test, not a substitute for one, and the `evidence`
# sentence is the part only a person can write.
#
# ORDER, on the machine that has the game:
#
#   dotnet build bibites-mod/BibitesMultiverse.csproj -c Release
#   bibites-mod/deploy.sh          # the game loads what you are about to record
#   … run it, and watch it …
#   release/record-tested-build.sh --tested-on 2026-08-20 --evidence '…'
#
# …then paste the block into docs/support-matrix.md, set the "mod" field on BOTH
# rows and the Plugin row of dev_environment.md to the version it names, and
# update the human table in that document's "The tested build" section.
#
# Usage:
#   release/record-tested-build.sh [--tested-on YYYY-MM-DD] [--evidence <sentence>]
#
# --tested-on defaults to today in UTC. --evidence defaults to a placeholder that
# says, in the record itself, that nobody wrote one.
#
# It needs go, git, python3 and sha256sum. It writes nothing.
set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RELDIR="$REPO/release"
PLUGIN="$REPO/bibites-mod/bin/Release/BibitesMultiverse.dll"
PLUGIN_SOURCE="$REPO/bibites-mod/src/MultiversePlugin.cs"
DEPLOYED="/mnt/c/Program Files (x86)/Steam/steamapps/common/The Bibites/BepInEx/plugins/BibitesMultiverse.dll"
PLACEHOLDER='REPLACE THIS: which deployment, how long it ran, what was observed'

# The same GOROOT treatment the other two release scripts give it: obey a real
# one, never let a stale one shadow a working go on PATH.
if [ -n "${GOROOT:-}" ] && [ ! -x "$GOROOT/bin/go" ]; then
  unset GOROOT
fi
if [ -z "${GOROOT:-}" ] && [ -x "$HOME/go/bin/go" ]; then
  export GOROOT="$HOME/go"
fi
[ -z "${GOROOT:-}" ] || export PATH="$GOROOT/bin:$PATH"
export TZ=UTC

note() { printf '    %s\n' "$*" >&2; }
die()  { printf '\n!! %s\n' "$*" >&2; exit 1; }

TESTED_ON="$(date -u +%Y-%m-%d)"
EVIDENCE="$PLACEHOLDER"
while [ $# -gt 0 ]; do
  case "$1" in
    --tested-on) [ $# -gt 1 ] || die "--tested-on needs a value"; TESTED_ON="$2"; shift 2 ;;
    --evidence)  [ $# -gt 1 ] || die "--evidence needs a value";  EVIDENCE="$2";  shift 2 ;;
    -h|--help) sed -n '2,34p' "$0" | sed 's/^# \{0,1\}//' >&2; exit 0 ;;
    *) die "unknown option: $1" ;;
  esac
done
printf '%s' "$TESTED_ON" | grep -Eq '^[0-9]{4}-[0-9]{2}-[0-9]{2}$' \
  || die "--tested-on wants YYYY-MM-DD, got '$TESTED_ON'"

for tool in git go python3 sha256sum; do
  command -v "$tool" >/dev/null 2>&1 || die "$tool is not on PATH; this script needs it"
done

# shellcheck source=lib/mod-version.sh
. "$RELDIR/lib/mod-version.sh"
# shellcheck source=lib/sidecar-manifest.sh
. "$RELDIR/lib/sidecar-manifest.sh"

[ -f "$PLUGIN" ] || die "no $PLUGIN.
   The record names the plugin's hash, so there has to be a plugin. Build it first:
     dotnet build bibites-mod/BibitesMultiverse.csproj -c Release"

# A DIRTY TREE MAKES THE RECORD A LIE, quietly: sidecarSourceCommit would name a
# commit that does not contain the files this manifest was computed from. Say so
# loudly and keep printing, because seeing the numbers is often why somebody runs
# this before committing.
if [ -n "$(git -C "$REPO" status --porcelain)" ]; then
  note "WARNING: this tree is dirty. The block below names HEAD, but the files"
  note "hashed into it are the working tree's. Commit first, then run this again."
fi

MOD_VERSION="$(mod_version "$PLUGIN_SOURCE")"
PLUGIN_SHA="$(sha256sum "$PLUGIN" | cut -d' ' -f1)"
MOD_TREE="$(git -C "$REPO" rev-parse "HEAD:bibites-mod")"
COMMIT="$(git -C "$REPO" rev-parse HEAD)"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
sidecar_manifest "$REPO" "$WORK/manifest.txt" "$WORK/packages.json"
SIDECAR_INPUTS="$(sha256sum "$WORK/manifest.txt" | cut -d' ' -f1)"

# GATE 3b's INPUT, CHECKED HERE WHERE IT CAN STILL BE FIXED. The release build
# requires the plugin in this machine's game to be the plugin it compiled. If
# they differ now, the game did not run what is about to be recorded.
if [ -f "$DEPLOYED" ]; then
  if [ "$(sha256sum <"$DEPLOYED" | cut -d' ' -f1)" = "$PLUGIN_SHA" ]; then
    note "this machine's game holds the same plugin"
  else
    note "WARNING: this machine's game holds a DIFFERENT plugin. Whatever you"
    note "tested, it was not this build. Run bibites-mod/deploy.sh, test again,"
    note "then run this again."
  fi
else
  note "this machine's game directory is not readable from here; not checking what it loads"
fi
if [ "$EVIDENCE" = "$PLACEHOLDER" ]; then
  note "no --evidence given: the block below records that nobody wrote one"
fi

# json.dumps for the two free-text values, so a quote or a backslash in the
# evidence sentence cannot produce a block that is not JSON.
python3 - "$MOD_VERSION" "$PLUGIN_SHA" "$MOD_TREE" "$COMMIT" "$SIDECAR_INPUTS" \
         "$TESTED_ON" "$EVIDENCE" <<'PY'
import json
import sys

mod, plugin, tree, commit, inputs, tested_on, evidence = sys.argv[1:8]
print('  "testedBuild": {')
print('    "mod": %s,' % json.dumps(mod))
print('    "pluginSha256": %s,' % json.dumps(plugin))
print('    "bibitesModTree": %s,' % json.dumps(tree))
print('    "sidecarSourceCommit": %s,' % json.dumps(commit))
print('    "sidecarInputsSha256": %s,' % json.dumps(inputs))
print('    "testedOn": %s,' % json.dumps(tested_on))
print('    "evidence": %s' % json.dumps(evidence))
print('  },')
PY

cat >&2 <<EOF

Paste that over the "testedBuild" block in docs/support-matrix.md, and in the
same edit:

  * the human table in that document's "The tested build" section
  * "mod": "$MOD_VERSION" on BOTH rows of the matrix, and the Mod column of both
    table rows above the JSON
  * the Plugin row of dev_environment.md

Then release/check-drift.sh must be green before anything is tagged.
EOF
