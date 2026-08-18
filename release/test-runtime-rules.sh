#!/usr/bin/env bash
#
# test-runtime-rules.sh
#
# Prove the rule that decides what the Linux installer and the Linux uninstall
# may delete. The PowerShell half of this proof is release/test-runtime-rules.ps1
# and it drives the same rule in the same order.
#
# release/test-install-uninstall.sh needs a real Linux game and a machine to
# install it on. This one needs neither: is_managed_runtime is a pure function
# over a path, and it is the guard on a `rm -rf`. It is lifted out of the two kit
# scripts themselves rather than copied here, so a rule that changes in one and
# not in the other fails this.
#
#   release/test-runtime-rules.sh        PASS/FAIL per check, exit 0 or 1
#
set -uo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
INSTALLER="$REPO/release/kit/install-bibites-multiverse.sh"
UNINSTALLER="$REPO/release/kit/uninstall-bibites-multiverse.sh"
for f in "$INSTALLER" "$UNINSTALLER"; do
  [ -f "$f" ] || { printf 'not here: %s\n' "$f" >&2; exit 2; }
done

# The function as each script has it, from its opening line to the closing brace
# at column zero. The uninstall's copy is renamed so both can be driven here.
lift() { # $1 file, $2 new name or ''
  local body
  body="$(awk '/^is_managed_runtime\(\) \{$/,/^\}$/' "$1")"
  [ -n "$body" ] || { printf 'no is_managed_runtime in %s\n' "$1" >&2; exit 2; }
  if [ -n "${2:-}" ]; then body="${body/is_managed_runtime()/$2()}"; fi
  printf '%s\n' "$body"
}
eval "$(lift "$INSTALLER")"
eval "$(lift "$UNINSTALLER" is_managed_runtime_uninstall)"

CHECKS=0
FAILURES=0
pass() { CHECKS=$((CHECKS + 1)); printf '  PASS  %s\n' "$1"; }
fail() { CHECKS=$((CHECKS + 1)); FAILURES=$((FAILURES + 1)); printf '  FAIL  %s\n' "$1"; }
accepts() { if "$1" "$3" "$4" "${5:-}"; then pass "$2"; else fail "$2"; fi; }
refuses() { if "$1" "$3" "$4" "${5:-}"; then fail "$2"; else pass "$2"; fi; }
scenario() { printf '\n==== %s\n' "$*"; }

ROOT="/home/someone/.local/share/bibites-multiverse"
SHA="$(printf 'A%.0s' $(seq 64))"
OTHER="$(printf 'B%.0s' $(seq 64))"
RUNTIMES="$ROOT/runtimes"
MANAGED="$RUNTIMES/$SHA"
CHOSEN="/opt/games/The Bibites"

for pair in "is_managed_runtime the installer" "is_managed_runtime_uninstall the uninstall"; do
  fn="${pair%% *}"
  who="${pair#* }"
  scenario "$who's copy of the managed-runtime rule"
  accepts "$fn" "accepts <data root>/runtimes/<sha>"                "$MANAGED" "$ROOT"
  accepts "$fn" "accepts it with a trailing slash"                  "$MANAGED/" "$ROOT"
  accepts "$fn" "accepts a lower-case name"                         "$RUNTIMES/$(printf '%s' "$SHA" | tr 'A-F' 'a-f')" "$ROOT"
  refuses "$fn" "refuses the data root itself"                      "$ROOT" "$ROOT"
  refuses "$fn" "refuses the journal's directory"                   "$ROOT/data" "$ROOT"
  refuses "$fn" "refuses the logs directory"                        "$ROOT/logs" "$ROOT"
  refuses "$fn" "refuses the runtimes directory itself"             "$RUNTIMES" "$ROOT"
  refuses "$fn" "refuses a directory inside the runtime"            "$MANAGED/BepInEx" "$ROOT"
  refuses "$fn" "refuses a name that is not a sha256"               "$RUNTIMES/game" "$ROOT"
  refuses "$fn" "refuses a 63-character name"                       "$RUNTIMES/${SHA%?}" "$ROOT"
  refuses "$fn" "refuses a path with .. in it"                      "$MANAGED/../$SHA" "$ROOT"
  refuses "$fn" "refuses .. that climbs out of the data root"       "$RUNTIMES/../../etc" "$ROOT"
  refuses "$fn" "refuses a sibling that only starts the same way"   "$ROOT/runtimes-old/$SHA" "$ROOT"
  refuses "$fn" "refuses a runtime under another data root"         "$MANAGED" "$ROOT/elsewhere"
  refuses "$fn" "refuses a game directory somebody chose"           "$CHOSEN" "$ROOT"
  refuses "$fn" "refuses a relative path"                           "runtimes/$SHA" "$ROOT"
  refuses "$fn" "refuses the root of the filesystem"                "/" "$ROOT"
  refuses "$fn" "refuses an empty path"                             "" "$ROOT"
  refuses "$fn" "refuses an empty data root"                        "$MANAGED" ""
done

# Only the installer's copy takes a payload hash: it knows which runtime this run
# is installing, and may remove that one and no other. The uninstall removes what
# its own record names, so it has no second hash to compare against.
scenario "the installer's rule, against the payload it is installing"
accepts is_managed_runtime "accepts the runtime whose name is that payload's hash" "$MANAGED" "$ROOT" "$SHA"
accepts is_managed_runtime "accepts it whatever case the name is written in" \
  "$RUNTIMES/$(printf '%s' "$SHA" | tr 'A-F' 'a-f')" "$ROOT" "$SHA"
refuses is_managed_runtime "refuses another runtime, which this run is not installing" \
  "$RUNTIMES/$OTHER" "$ROOT" "$SHA"

printf '\n'
if [ "$FAILURES" -gt 0 ]; then
  printf '!! %d of %d checks failed\n' "$FAILURES" "$CHECKS"
  exit 1
fi
printf '    %d checks pass\n' "$CHECKS"
