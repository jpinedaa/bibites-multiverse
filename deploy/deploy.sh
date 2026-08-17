#!/usr/bin/env bash
# THE deployment sequence, on the host. One implementation, run the same way by
# a person and by CI.
#
#   sudo deploy/deploy.sh --kit <staged-kit-dir> [options]
#
#   --kit <dir>            A staged copy of the repository's deploy/ tree. Its
#                          OWN provision.sh installs the kit, because a script
#                          installs the kit it belongs to.
#   --binaries             Also install the staged binaries from <dir>/../stage
#                          or --stage.
#   --stage <dir>          Where the built binaries are. Default: the kit's
#                          sibling stage/, else $MV_STAGE_DIR.
#   --expect-kit-digest <sha256>
#                          Refuse unless the kit that lands has exactly this
#                          listing digest.
#   --allow-envfiles       Permit /etc/multiverse/*.env to change. Without it,
#                          a phase that rewrites one is restored and the run
#                          fails before anything else happens.
#   --dry-run              Rehearse. provision.sh routes every mutation through
#                          one helper, so the rehearsal is honest.
#   --print-kit-digest <dir>
#                          Print the listing digest of <dir> and exit. This is
#                          the ONLY implementation of that computation.
#
# WHY THIS FILE EXISTS. Three hand-written deploy scripts lived on the host,
# each copied from the last with the constants edited. They agreed on the order
# and disagreed on everything else: two used `set -euo pipefail` and one used
# `set -uo pipefail` with explicit `|| die`; one ran `--only envfiles` and
# another asserted the env files had not changed; only the newest asserted the
# installed kit was the shipped kit. The valuable parts of the newest one are
# below, once, so that the next deployment cannot lose them by being copied from
# an older script.
#
# WHAT IT DOES NOT DO: restart anything. A new binary on disk is not a running
# new binary, and the restart is a deliberate act with a policy behind it —
# RESTART-POLICY.md, and deploy/restart-relay.sh which implements it.
set -uo pipefail

KIT=""
STAGE=""
EXPECT_DIGEST=""
WITH_BINARIES=0
ALLOW_ENVFILES=0
DRY=0
ETC=/etc/multiverse

say()  { printf '     %s\n' "$*"; }
step() { printf '\n==== %s  %s\n' "$(date -u +%Y-%m-%dT%H:%M:%S.%3NZ)" "$*"; }
die()  { printf '\nSTOP: %s\n' "$*" >&2; exit 2; }

# kit_listing_digest <dir> — THE kit listing digest, defined in exactly one place.
#
# sha256 of a text listing of "<sha256>  <path>" per file, one line per file, in
# LC_ALL sort order, with paths relative and carrying no leading "./".
#
# The "./" detail is not pedantry: the same listing built with find's default
# prefix produces a different value, and the operations source lock records this
# number so that an installed kit can be checked against the commit it claims to
# be. `-printf '%P\n'` is what drops the prefix. An earlier hand-written script
# reached the same value a different way, with `find . -type f | sort | xargs
# sha256sum | sed 's|  \./|  |'`; two implementations of one number is how the
# two silently stop agreeing, so there is now one.
kit_listing_digest() {
  local dir="$1"
  [ -d "$dir" ] || return 1
  ( cd "$dir" && find . -type f -printf '%P\n' | LC_ALL=C sort | xargs -r sha256sum |
      sha256sum | cut -d' ' -f1 )
}

while [ $# -gt 0 ]; do
  case "$1" in
    --kit)               KIT="${2:?--kit needs a directory}"; shift ;;
    --stage)             STAGE="${2:?--stage needs a directory}"; shift ;;
    --expect-kit-digest) EXPECT_DIGEST="${2:?--expect-kit-digest needs a sha256}"; shift ;;
    --binaries)          WITH_BINARIES=1 ;;
    --allow-envfiles)    ALLOW_ENVFILES=1 ;;
    --dry-run)           DRY=1 ;;
    --print-kit-digest)
      kit_listing_digest "${2:?--print-kit-digest needs a directory}" || die "cannot read ${2}"
      exit 0 ;;
    -h|--help)           sed -n '2,38p' "$0"; exit 0 ;;
    *)                   die "unknown argument: $1" ;;
  esac
  shift
done

[ -n "$KIT" ] || die "no --kit. There is nothing to install without one."
[ -d "$KIT" ] || die "no such kit directory: $KIT"
PROVISION="$KIT/deploy/provision.sh"
[ -x "$PROVISION" ] || PROVISION="$KIT/provision.sh"
[ -x "$PROVISION" ] || die "no executable provision.sh under $KIT"
[ "$(id -u)" = 0 ] || die "run this with sudo. It writes /etc and /opt."

INSTALLED_KIT="${MV_PREFIX:-/opt/multiverse}/deploy"
if [ -z "$STAGE" ]; then
  if [ -d "$KIT/../stage" ]; then STAGE="$(cd "$KIT/../stage" && pwd)"
  else STAGE="${MV_STAGE_DIR:-/home/ubuntu/multiverse-stage}"; fi
fi

DRYFLAG=()
[ "$DRY" = 1 ] && DRYFLAG=(--dry-run)

step "0  what this run will do"
say "kit          $KIT"
say "provision    $PROVISION"
say "installed to $INSTALLED_KIT"
[ "$WITH_BINARIES" = 1 ] && say "binaries     $STAGE"
[ "$DRY" = 1 ] && say "DRY RUN — nothing is changed"

# ---------------------------------------------------------------- 1  env snapshot
#
# Taken BEFORE anything runs, because the assertion in step 4 is only worth
# something if the "before" was recorded before the change that might break it.
step "1  snapshot the environment files"
BACKUP="$(mktemp -d /root/deploy-envbackup.XXXXXX)"
chmod 0700 "$BACKUP"
ENVFILES=""
for f in "$ETC"/*.env; do
  [ -e "$f" ] || continue
  cp -p "$f" "$BACKUP/$(basename "$f")" || die "could not back up $f"
  ENVFILES="$ENVFILES $(basename "$f")"
  say "$(sha256sum "$f")"
done
[ -n "$ENVFILES" ] || say "no environment files present yet"

restore_envfiles() {
  local n
  for n in $ENVFILES; do cp -p "$BACKUP/$n" "$ETC/$n"; done
}

# ---------------------------------------------------------------- 2  the kit
step "2  install the kit  (provision.sh --only directories, from the staged copy)"
"$PROVISION" --only directories "${DRYFLAG[@]}" || die "the directories phase failed"

# BEFORE ANY BINARY IS STAGED INTO PLACE. From here on the running services and
# their files on disk disagree, and that disagreement is exactly what
# needrestart acts on after an apt transaction — restarting the relay and the
# archive together, outside the peer gate and the relay hold-down. It cost
# 134.063 s of permanent record on 2026-08-17. RESTART-POLICY.md, "Package
# installs and needrestart".
"$PROVISION" --only needrestart "${DRYFLAG[@]}" || die "the needrestart phase failed"

# ---------------------------------------------------------------- 3  prove the kit
#
# The exit code of the phase above says a script ran. It does not say the right
# files landed. This does.
step "3  prove the installed kit is the staged kit"
if [ "$DRY" = 1 ]; then
  say "[dry-run] would compare every installed file with its staged original"
else
  # FILE BY FILE, not digest against digest.
  #
  # The staged kit and the installed kit are deliberately different SETS: the
  # staged tree is all of deploy/, and provision.sh installs a subset of it
  # (scripts, documents, nginx/, systemd/, the announcements seed) chosen by
  # rules that live in provision.sh. Comparing a digest of one against a digest
  # of the other would mean restating those rules here, and the restatement
  # would be wrong the first time the rules changed — the failure mode being a
  # deployment that refuses for no reason, or worse, one that passes for none.
  #
  # Asking instead "is every file that landed byte-identical to the file it came
  # from" needs no knowledge of the rules at all, and proves more: it catches a
  # partial copy, a truncated write, and a file that was modified on the host
  # between staging and now.
  KITSRC="$KIT/deploy"; [ -d "$KITSRC" ] || KITSRC="$KIT"
  missing=0; differs=0; checked=0
  while IFS= read -r rel; do
    checked=$((checked + 1))
    if [ ! -e "$KITSRC/$rel" ]; then
      say "NOT IN THE STAGED KIT: $rel"; missing=$((missing + 1))
    elif ! cmp -s "$INSTALLED_KIT/$rel" "$KITSRC/$rel"; then
      say "DIFFERS: $rel"; differs=$((differs + 1))
    fi
  done < <(cd "$INSTALLED_KIT" && find . -type f -printf '%P\n' | LC_ALL=C sort)
  say "$checked installed files checked, $differs differ, $missing not in the staged kit"
  if [ "$differs" != 0 ]; then
    die "the installed kit is not the kit that was staged. No binary has been
     installed and nothing has been restarted. A kit that is not the one that was
     built is a kit nobody reviewed."
  fi
  # A leftover from an older kit is a finding, not a failure: provision.sh
  # replaces files and does not delete ones it no longer ships, so a file the
  # staged kit does not contain is stale rather than wrong. Say so loudly.
  [ "$missing" = 0 ] || say "NOTE: $missing installed file(s) are not in this kit. They are left over
     from an earlier one. provision.sh replaces files and never deletes them."

  got="$(kit_listing_digest "$INSTALLED_KIT")" || die "cannot read $INSTALLED_KIT"
  say "installed listing digest  $got"
  if [ -n "$EXPECT_DIGEST" ]; then
    say "expected                  $EXPECT_DIGEST"
    [ "$got" = "$EXPECT_DIGEST" ] \
      || die "the installed listing digest is not the expected one. Nothing further has run."
    say "MATCH"
  fi
fi

# ---------------------------------------------------------------- 4  prove the env files
#
# The directories phase must not write an environment file. When it does, the
# deployment has silently changed the service's identity — its domain, its
# retention rule, its enrollment limits — and the operator finds out from the
# behaviour rather than from the record.
step "4  the environment files must be untouched"
if [ "$DRY" = 1 ]; then
  say "[dry-run] would compare each $ETC/*.env against its snapshot"
elif [ "$ALLOW_ENVFILES" = 1 ]; then
  say "--allow-envfiles given; changes are permitted and are shown below"
  for n in $ENVFILES; do
    cmp -s "$BACKUP/$n" "$ETC/$n" || { say "$n CHANGED:"; diff -u "$BACKUP/$n" "$ETC/$n" | sed 's/^/       /'; }
  done
else
  changed=""
  for n in $ENVFILES; do
    cmp -s "$BACKUP/$n" "$ETC/$n" || changed="$changed $n"
  done
  if [ -n "$changed" ]; then
    say "CHANGED:$changed"
    for n in $changed; do diff -u "$BACKUP/$n" "$ETC/$n" | sed 's/^/       /'; done
    restore_envfiles
    die "a phase rewrote$changed. All environment files have been RESTORED from the
     snapshot and nothing further has run. If this change was intended, re-run
     with --allow-envfiles and record the diff above in the deployment record."
  fi
  say "unchanged:$ENVFILES"
fi

# ---------------------------------------------------------------- 5  binaries
if [ "$WITH_BINARIES" = 1 ]; then
  step "5  verify the staged binaries"
  [ -d "$STAGE" ] || die "no staged binaries at $STAGE"
  if [ -f "$STAGE/SHA256SUMS" ]; then
    ( cd "$STAGE" && sha256sum -c SHA256SUMS ) >/dev/null \
      || die "the staged binaries do not match their own SHA256SUMS"
    say "staged checksums verified"
  else
    say "no SHA256SUMS beside the staged binaries — cannot verify them"
  fi

  step "6  install the binaries  (provision.sh --only binaries)"
  # The INSTALLED provision.sh, not the staged one: by now they are the same
  # file, and using the installed copy is what proves step 2 took effect.
  "$INSTALLED_KIT/provision.sh" --only binaries "${DRYFLAG[@]}" \
    || die "the binaries phase failed"

  step "7  prove the installed binaries are the staged binaries"
  if [ "$DRY" = 1 ]; then
    say "[dry-run] would compare each installed binary with its staged artifact"
  else
    arch=amd64
    case "$(uname -m)" in aarch64) arch=arm64 ;; esac
    bad=0
    for pair in "multiverse-relay:relay" "multiverse-archive:archive" "ringstat:ringstat"; do
      inst="${MV_PREFIX:-/opt/multiverse}/bin/${pair%%:*}"
      src="$STAGE/${pair##*:}-linux-$arch"
      [ -f "$inst" ] && [ -f "$src" ] || continue
      if cmp -s "$inst" "$src"; then say "${pair%%:*}  matches the staged artifact"
      else say "${pair%%:*}  DOES NOT MATCH $src"; bad=1; fi
    done
    [ "$bad" = 0 ] || die "an installed binary is not the artifact that was staged."
    say "NOTE: new binaries do not take effect until the units restart — RESTART-POLICY.md"
  fi
fi

# ---------------------------------------------------------------- receipt
step "done"
if [ "$DRY" = 0 ]; then
  printf 'kit_listing_digest=%s\n' "$(kit_listing_digest "$INSTALLED_KIT")"
  [ -r /etc/multiverse/BINARIES.sha256 ] && sed 's/^/binary /' /etc/multiverse/BINARIES.sha256
fi
rm -rf "$BACKUP"
exit 0
