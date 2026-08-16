#!/usr/bin/env bash
# Assert that a release/make-release.sh log shows every gate actually running.
#
# WHY THIS FILE EXISTS. Several of the build's strongest gates DOWNGRADE
# silently when an input is unreadable instead of failing:
#
#   * the Linux matrix row check "stands on its record" when there is no
#     unpacked Linux game;
#   * the three-way plugin check says "the bundle check stands alone" when this
#     machine's game directory cannot be read;
#   * the bundle provenance file is skipped with a note when it is absent.
#
# All three are reasonable for a hand build on the owner's machine. Under a
# dedicated CI runner user, a permission mistake would turn any of them into a
# permanent, invisible no-op — the release would keep passing while the check
# that gives it its value had stopped running months earlier. So the release
# workflow requires the full success set to appear in the log, by literal text.
#
# Every pattern below was checked against the actual note() text in
# release/make-release.sh — the line number is given beside each one — and
# against the logs of the last two real builds. Line numbers move; the TEXT is
# the contract. A missing line means a gate did not run, never that the log is
# formatted differently.
#
# Usage:
#   release/verify-build-log.sh <log> [<release>]
#
# With a release argument it additionally requires the log to name the five
# artifacts for that exact version, which catches a log left over from an
# earlier build. (The release string is interpolated into a regex, so its dots
# match any character. Harmless: it only ever makes the match more permissive.)
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(cd "$HERE/.." && pwd)"

LOG="${1:-}"
RELEASE="${2:-}"
[ -n "$LOG" ] || { printf '!! verify-build-log: usage: %s <log> [<release>]\n' "$0" >&2; exit 2; }
[ -f "$LOG" ] || { printf '!! verify-build-log: no such log: %s\n' "$LOG" >&2; exit 2; }

# Each entry is "<extended regex>|<what it proves>", split on the LAST '|', so
# a pattern may contain an alternation. A description must therefore never
# contain '|'.
REQUIRED=(
  '^ *clean at [0-9a-f]{7,40}$|the build ran on a clean tree (make-release.sh:160)'
  'allow-listed locations, no stray version literal|the version surface agrees with itself (make-release.sh:193)'
  'the plugin source and both matrix rows say mod |gate E, the plugin version against the matrix (make-release.sh:293)'
  'libs/BibitesAssembly\.dll matches the matrix: game .+ on Windows|the game pin held (make-release.sh:307)'
  "the Linux game at .+ matches the matrix's Linux row|the Linux row was checked against a real game, not trusted (make-release.sh:318)"
  'the bundled sidecar was built from [0-9a-f]{40}|the bundle stamp was read back (make-release.sh:393)'
  'cmd/sidecar uses the same repository inputs and module versions as the bundled sidecar|gate 2, the sidecar input manifest (make-release.sh:425)'
  'same source as the bundled sidecar \([0-9a-f]{40}\)|the Linux sidecar came from the bundled source (make-release.sh:456)'
  'byte-identical to the bundled plugin|gate 3, the plugin matches the tracked bundle (make-release.sh:503)'
  "byte-identical to the plugin in this machine's game|gate 3b, the three-way leg (make-release.sh:511)"
  'BepInEx_win_x64_.+\.zip sha256 verified: [0-9a-f]{64}|the Windows BepInEx pin (make-release.sh:534)'
  'BepInEx_linux_x64_.+\.zip sha256 verified: [0-9a-f]{64}|the Linux BepInEx pin (make-release.sh:534)'
  'the Linux scripts parse, are LF, and carry their executable bit|the Linux kit is usable as shipped (make-release.sh:602)'
  'Windows complete payload: game .+, assembly [0-9A-F]{64}|the Windows game payload was staged and hashed (make-release.sh:642)'
  'Linux complete payload: game .+, assembly [0-9A-F]{64}|the Linux game payload was staged and hashed (make-release.sh:642)'
  'MANIFEST\.sha256: [0-9]+ files \(Windows\), [0-9]+ files \(Linux\)|both per-archive manifests were written (make-release.sh:690)'
  'every archive contains LICENSE, THIRD_PARTY_NOTICES\.md, and the public join configuration|the licence and join config are in every archive (make-release.sh:781)'
  'the Windows add-on archive contains BibitesMultiverseLauncher\.exe|the add-on archive ships the launcher (make-release.sh:791)'
  'the Windows complete archive and the setup payload contain BibitesMultiverseLauncher\.exe|the setup shortcuts point at a file that exists (make-release.sh:797)'
  '^=== ready, and NOTHING IS PUBLISHED$|the build reached its end (make-release.sh:894)'
)

# Text that must NOT appear. Each of these is a downgrade the build prints
# instead of failing, and each one silently deletes a gate.
FORBIDDEN=(
  '--allow-dirty: this is a rehearsal and MUST NOT be published\.|the build was a rehearsal; a rehearsal must never be published (make-release.sh:158)'
  'no unpacked Linux game at|the Linux row check downgraded to its record: MV_RELEASE_LINUX_GAME is wrong or unreadable (make-release.sh:320)'
  "this machine's game directory is not readable from here|gate 3b did not run: the runner user cannot read the Steam plugin (make-release.sh:513)"
)

# THE BUNDLE PROVENANCE FILE. farend/dist/BUNDLE-SOURCE.txt is the second of
# the two independent witnesses that the tracked far-end bundle is the bundle
# this tree builds. make-release.sh asserts it against the zip it just
# unpacked, and prints a note instead when the file is absent — which is right
# for a bundle built before the record existed, and is exactly the silent
# no-op this script exists to catch once the record IS tracked.
#
# So the requirement follows the tree, with no edit needed on the day it lands:
# while the file is absent, one of the two notes must appear (silence fails);
# once it is present, only the assertion will do and the "predates it" note
# becomes forbidden.
if [ -f "$REPO/farend/dist/BUNDLE-SOURCE.txt" ]; then
  REQUIRED+=(
    'BUNDLE-SOURCE\.txt agrees with the zip; the bundle was built at [0-9a-f]{40}|the bundle provenance file was asserted against the zip (make-release.sh:377)'
  )
  FORBIDDEN+=(
    'no farend/dist/BUNDLE-SOURCE\.txt|the provenance record is tracked but the build did not see it, so the second witness stopped existing (make-release.sh:379)'
  )
else
  REQUIRED+=(
    'BUNDLE-SOURCE\.txt agrees with the zip; the bundle was built at [0-9a-f]{40}|no farend/dist/BUNDLE-SOURCE\.txt|the build either asserted the bundle provenance record or said in so many words that there is none (make-release.sh:377 or :379)'
  )
fi

fails=0
for entry in "${REQUIRED[@]}"; do
  pattern="${entry%|*}"
  why="${entry##*|}"
  if ! grep -Eq -- "$pattern" "$LOG"; then
    printf '!! missing from %s: %s\n' "$LOG" "$why" >&2
    printf '   expected to match: %s\n' "$pattern" >&2
    fails=$((fails + 1))
  fi
done

for entry in "${FORBIDDEN[@]}"; do
  pattern="${entry%|*}"
  why="${entry##*|}"
  if grep -Eq -- "$pattern" "$LOG"; then
    printf '!! %s reports a downgraded gate: %s\n' "$LOG" "$why" >&2
    printf '   matched: %s\n' "$(grep -Em1 -- "$pattern" "$LOG")" >&2
    fails=$((fails + 1))
  fi
done

if [ -n "$RELEASE" ]; then
  for name in \
    "bibites-multiverse-$RELEASE-windows-x64-setup\.exe" \
    "bibites-multiverse-$RELEASE-windows-x64-complete\.zip" \
    "bibites-multiverse-$RELEASE-windows-x64\.zip" \
    "bibites-multiverse-$RELEASE-linux-x64-complete\.zip" \
    "bibites-multiverse-$RELEASE-linux-x64\.zip"
  do
    if ! grep -Eq -- "$name" "$LOG"; then
      printf '!! %s never names %s — is this the log of this build?\n' "$LOG" "$name" >&2
      fails=$((fails + 1))
    fi
  done
fi

if [ "$fails" -ne 0 ]; then
  printf '\n!! verify-build-log: %d problem(s) in %s.\n' "$fails" "$LOG" >&2
  printf '   A missing success line means a gate did not run, not that the log is\n' >&2
  printf '   formatted differently. Do not publish this build.\n' >&2
  exit 1
fi

printf '    %s shows all %d required gates and none of the %d silent downgrades\n' \
  "$LOG" "${#REQUIRED[@]}" "${#FORBIDDEN[@]}"
