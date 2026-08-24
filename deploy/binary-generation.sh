#!/usr/bin/env bash
# Capture and restore exact service-binary generations.
#
# Capture reads the relay and archive through stable /proc/<pid>/exe handles.
# It writes an owner-only, content-addressed generation only after it proves
# that neither service restarted and no installed binary changed during the
# capture. Restore accepts one exact generation identifier, verifies every
# byte before replacement, and does not restart a service.
set -euo pipefail

DRY=0
ACTION=""
GENERATION=""
BIN_DIR="${MV_PREFIX:-/opt/multiverse}/bin"
STORE="${MV_BINARY_GENERATION_DIR:-${MV_STATE:-/var/lib/multiverse}/rollback/binaries}"
PROC_ROOT="/proc"
SELF_PROC="/proc"
CURRENT_UID="$(id -u)"
TMP_CAPTURE=""
TMP_RESTORE=""

say() { printf '%s\n' "$*"; }
die() { printf 'STOP: %s\n' "$*" >&2; exit 1; }

usage() {
  sed -n '2,12p' "$0"
  cat <<'EOF'

Usage:
  binary-generation.sh capture [--dry-run] [--bin-dir DIR] [--store DIR]
                               [--proc-root DIR]
  binary-generation.sh restore --generation sha256-<digest> [--dry-run]
                               [--bin-dir DIR] [--store DIR]

Capture must run before a binary replacement. Restore changes only installed
paths. It never restarts a service and never selects a generation implicitly.
EOF
}

cleanup() {
  [ -z "$TMP_CAPTURE" ] || rm -rf -- "$TMP_CAPTURE"
  [ -z "$TMP_RESTORE" ] || rm -rf -- "$TMP_RESTORE"
}
trap cleanup EXIT

while [ "$#" -gt 0 ]; do
  case "$1" in
    capture|restore)
      [ -z "$ACTION" ] || die "choose only one action"
      ACTION="$1"
      ;;
    --generation)
      GENERATION="${2:?--generation needs sha256-<digest>}"
      shift
      ;;
    --bin-dir)
      BIN_DIR="${2:?--bin-dir needs a directory}"
      shift
      ;;
    --store)
      STORE="${2:?--store needs a directory}"
      shift
      ;;
    --proc-root)
      PROC_ROOT="${2:?--proc-root needs a directory}"
      shift
      ;;
    --dry-run) DRY=1 ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
  shift
done

[ -n "$ACTION" ] || { usage >&2; exit 1; }
case "$ACTION" in
  capture)
    [ -z "$GENERATION" ] || die "capture does not take --generation"
    ;;
  restore)
    [ -n "$GENERATION" ] || die "restore requires an exact --generation"
    [[ "$GENERATION" =~ ^sha256-[0-9a-f]{64}$ ]] \
      || die "invalid generation identifier: $GENERATION"
    ;;
esac

sha_path() {
  local path="$1"
  if [ ! -e "$path" ] && [ ! -L "$path" ]; then
    printf '%s\n' -
    return 0
  fi
  [ -f "$path" ] || die "$path is not a regular file"
  [ ! -L "$path" ] || die "$path is a symbolic link"
  sha256sum "$path" | awk '{print $1}'
}

sha_fd() {
  sha256sum "$SELF_PROC/self/fd/$1" | awk '{print $1}'
}

size_fd() {
  stat -Lc '%s' "$SELF_PROC/self/fd/$1"
}

main_pid() {
  local unit="$1" pid
  pid="$(systemctl show --property=MainPID --value "$unit" 2>/dev/null || true)"
  case "$pid" in
    ''|0|*[!0-9]*) printf '%s\n' - ;;
    *) printf '%s\n' "$pid" ;;
  esac
}

generation_id() {
  local relay_sha="$1" archive_sha="$2" ringstat_sha="$3"
  printf 'relay_sha256=%s\narchive_sha256=%s\nringstat_sha256=%s\n' \
    "$relay_sha" "$archive_sha" "$ringstat_sha" |
    sha256sum | awk '{print "sha256-" $1}'
}

mode_of() { stat -c '%a' "$1"; }
owner_of() { stat -c '%u' "$1"; }

require_owner_only_dir() {
  local path="$1"
  [ -d "$path" ] || die "$path is not a directory"
  [ ! -L "$path" ] || die "$path is a symbolic link"
  [ "$(owner_of "$path")" = "$CURRENT_UID" ] \
    || die "$path is not owned by uid $CURRENT_UID"
  [ "$(mode_of "$path")" = 700 ] || die "$path must have mode 0700"
}

require_owner_only_file() {
  local path="$1"
  [ -f "$path" ] || die "$path is not a regular file"
  [ ! -L "$path" ] || die "$path is a symbolic link"
  [ "$(owner_of "$path")" = "$CURRENT_UID" ] \
    || die "$path is not owned by uid $CURRENT_UID"
  [ "$(mode_of "$path")" = 600 ] || die "$path must have mode 0600"
}

prepare_store() {
  if [ -e "$STORE" ]; then
    require_owner_only_dir "$STORE"
    return 0
  fi
  umask 077
  mkdir -p -- "$STORE"
  chmod 0700 "$STORE"
  require_owner_only_dir "$STORE"
}

# validate_generation sets the VALID_* fields after it verifies the named
# generation. It accepts no extra manifest keys or files.
validate_generation() {
  local expected_generation="$1" dir="$STORE/$1" key value extra count=0
  local relay_actual archive_actual ringstat_actual actual_generation name
  local -A manifest=()
  local -a required=(
    format generation captured_at_utc relay_pid archive_pid
    relay_sha256 relay_size archive_sha256 archive_size
    ringstat_sha256 ringstat_size installed_relay_sha256
    installed_archive_sha256 installed_ringstat_sha256
  )

  require_owner_only_dir "$STORE"
  require_owner_only_dir "$dir"
  require_owner_only_file "$dir/manifest.tsv"
  require_owner_only_file "$dir/SHA256SUMS"

  while IFS=$'\t' read -r key value extra || [ -n "${key:-}" ]; do
    [ -n "${key:-}" ] && [ -n "${value:-}" ] && [ -z "${extra:-}" ] \
      || die "$dir/manifest.tsv has an invalid row"
    [ -z "${manifest[$key]+set}" ] || die "$dir/manifest.tsv repeats $key"
    manifest[$key]="$value"
    count=$((count + 1))
  done <"$dir/manifest.tsv"

  [ "$count" = "${#required[@]}" ] || die "$dir/manifest.tsv has an unexpected key count"
  for key in "${required[@]}"; do
    [ -n "${manifest[$key]+set}" ] || die "$dir/manifest.tsv is missing $key"
  done

  [ "${manifest[format]}" = multiverse-binary-generation-v1 ] \
    || die "$dir/manifest.tsv has an unsupported format"
  [ "${manifest[generation]}" = "$expected_generation" ] \
    || die "$dir/manifest.tsv names a different generation"
  [[ "${manifest[captured_at_utc]}" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$ ]] \
    || die "$dir/manifest.tsv has an invalid capture time"
  [[ "${manifest[relay_pid]}" =~ ^[1-9][0-9]*$ ]] \
    && [[ "${manifest[archive_pid]}" =~ ^[1-9][0-9]*$ ]] \
    || die "$dir/manifest.tsv has an invalid service PID"

  for key in relay_sha256 archive_sha256; do
    [[ "${manifest[$key]}" =~ ^[0-9a-f]{64}$ ]] \
      || die "$dir/manifest.tsv has an invalid $key"
  done
  for key in installed_relay_sha256 installed_archive_sha256 installed_ringstat_sha256; do
    [ "${manifest[$key]}" = - ] && continue
    [[ "${manifest[$key]}" =~ ^[0-9a-f]{64}$ ]] \
      || die "$dir/manifest.tsv has an invalid $key"
  done
  for key in relay_size archive_size; do
    [[ "${manifest[$key]}" =~ ^[0-9]+$ ]] || die "$dir/manifest.tsv has an invalid $key"
  done

  require_owner_only_file "$dir/multiverse-relay"
  require_owner_only_file "$dir/multiverse-archive"
  relay_actual="$(sha256sum "$dir/multiverse-relay" | awk '{print $1}')"
  archive_actual="$(sha256sum "$dir/multiverse-archive" | awk '{print $1}')"
  [ "$relay_actual" = "${manifest[relay_sha256]}" ] || die "$dir/multiverse-relay is corrupt"
  [ "$archive_actual" = "${manifest[archive_sha256]}" ] || die "$dir/multiverse-archive is corrupt"
  [ "$(stat -c '%s' "$dir/multiverse-relay")" = "${manifest[relay_size]}" ] \
    || die "$dir/multiverse-relay has the wrong size"
  [ "$(stat -c '%s' "$dir/multiverse-archive")" = "${manifest[archive_size]}" ] \
    || die "$dir/multiverse-archive has the wrong size"

  if [ "${manifest[ringstat_sha256]}" = - ]; then
    [ "${manifest[ringstat_size]}" = - ] || die "$dir/manifest.tsv has an invalid ringstat size"
    [ ! -e "$dir/ringstat" ] || die "$dir has an unexpected ringstat"
    ringstat_actual=-
  else
    [[ "${manifest[ringstat_sha256]}" =~ ^[0-9a-f]{64}$ ]] \
      || die "$dir/manifest.tsv has an invalid ringstat hash"
    [[ "${manifest[ringstat_size]}" =~ ^[0-9]+$ ]] \
      || die "$dir/manifest.tsv has an invalid ringstat size"
    require_owner_only_file "$dir/ringstat"
    ringstat_actual="$(sha256sum "$dir/ringstat" | awk '{print $1}')"
    [ "$ringstat_actual" = "${manifest[ringstat_sha256]}" ] || die "$dir/ringstat is corrupt"
    [ "$(stat -c '%s' "$dir/ringstat")" = "${manifest[ringstat_size]}" ] \
      || die "$dir/ringstat has the wrong size"
  fi

  while IFS= read -r name; do
    case "$name" in
      manifest.tsv|SHA256SUMS|multiverse-relay|multiverse-archive) ;;
      ringstat) [ "$ringstat_actual" != - ] || die "$dir has an unexpected ringstat" ;;
      *) die "$dir has an unexpected file: $name" ;;
    esac
  done < <(find "$dir" -mindepth 1 -maxdepth 1 -printf '%f\n' | LC_ALL=C sort)

  if ! ( cd "$dir" && sha256sum -c --strict SHA256SUMS ) >/dev/null 2>&1; then
    die "$dir/SHA256SUMS does not verify"
  fi
  if [ "$ringstat_actual" = - ]; then
    [ "$(wc -l <"$dir/SHA256SUMS")" = 2 ] || die "$dir/SHA256SUMS has an unexpected entry"
    ! grep -q 'ringstat' "$dir/SHA256SUMS" || die "$dir/SHA256SUMS has an unexpected ringstat entry"
  else
    [ "$(wc -l <"$dir/SHA256SUMS")" = 3 ] || die "$dir/SHA256SUMS has an unexpected entry"
  fi
  grep -Fxq "$relay_actual  multiverse-relay" "$dir/SHA256SUMS" \
    || die "$dir/SHA256SUMS has an invalid relay entry"
  grep -Fxq "$archive_actual  multiverse-archive" "$dir/SHA256SUMS" \
    || die "$dir/SHA256SUMS has an invalid archive entry"
  [ "$ringstat_actual" = - ] || grep -Fxq "$ringstat_actual  ringstat" "$dir/SHA256SUMS" \
    || die "$dir/SHA256SUMS has an invalid ringstat entry"

  actual_generation="$(generation_id "$relay_actual" "$archive_actual" "$ringstat_actual")"
  [ "$actual_generation" = "$expected_generation" ] \
    || die "$dir does not match its generation identifier"

  VALID_RELAY_SHA="$relay_actual"
  VALID_ARCHIVE_SHA="$archive_actual"
  VALID_RINGSTAT_SHA="$ringstat_actual"
}

copy_fd() {
  local fd="$1" dst="$2" expected="$3"
  dd if="$SELF_PROC/self/fd/$fd" of="$dst" status=none
  chmod 0600 "$dst"
  [ "$(sha256sum "$dst" | awk '{print $1}')" = "$expected" ] \
    || die "the copied artifact $dst failed its hash check"
}

capture() {
  local relay_path="$BIN_DIR/multiverse-relay"
  local archive_path="$BIN_DIR/multiverse-archive"
  local ringstat_path="$BIN_DIR/ringstat"
  local relay_pid archive_pid relay_pid_after archive_pid_after
  local relay_fd archive_fd ringstat_fd=""
  local relay_sha archive_sha ringstat_sha=- relay_size archive_size ringstat_size=-
  local installed_relay installed_archive installed_ringstat
  local installed_relay_after installed_archive_after installed_ringstat_after
  local generated final captured_at already_captured=0

  relay_pid="$(main_pid multiverse-relay.service)"
  archive_pid="$(main_pid multiverse-archive.service)"

  if [ "$relay_pid" = - ] && [ "$archive_pid" = - ]; then
    if [ ! -e "$relay_path" ] && [ ! -L "$relay_path" ] \
       && [ ! -e "$archive_path" ] && [ ! -L "$archive_path" ] \
       && [ ! -e "$ringstat_path" ] && [ ! -L "$ringstat_path" ]; then
      say "capture_mode=initial-install"
      say "rollback_generation=initial-install"
      say "No installed binary or running service exists. There is no preimage to capture."
      say "generation_status=initial-install"
      return 0
    fi
    die "the relay and archive are not both running, but an installed binary exists. Nothing was captured or installed"
  fi
  [ "$relay_pid" != - ] && [ "$archive_pid" != - ] \
    || die "the relay and archive do not have one active generation. Nothing was captured or installed"

  exec {relay_fd}<"$PROC_ROOT/$relay_pid/exe" \
    || die "cannot open the running relay executable for pid $relay_pid"
  exec {archive_fd}<"$PROC_ROOT/$archive_pid/exe" \
    || die "cannot open the running archive executable for pid $archive_pid"
  relay_sha="$(sha_fd "$relay_fd")"
  archive_sha="$(sha_fd "$archive_fd")"
  relay_size="$(size_fd "$relay_fd")"
  archive_size="$(size_fd "$archive_fd")"

  installed_relay="$(sha_path "$relay_path")"
  installed_archive="$(sha_path "$archive_path")"
  installed_ringstat="$(sha_path "$ringstat_path")"
  if [ "$installed_ringstat" != - ]; then
    exec {ringstat_fd}<"$ringstat_path" || die "cannot open $ringstat_path"
    ringstat_sha="$(sha_fd "$ringstat_fd")"
    ringstat_size="$(size_fd "$ringstat_fd")"
  fi

  generated="$(generation_id "$relay_sha" "$archive_sha" "$ringstat_sha")"
  final="$STORE/$generated"
  say "running_relay_pid=$relay_pid sha256=$relay_sha size=$relay_size"
  say "running_archive_pid=$archive_pid sha256=$archive_sha size=$archive_size"
  say "installed_relay_sha256=$installed_relay"
  say "installed_archive_sha256=$installed_archive"
  say "installed_ringstat_sha256=$installed_ringstat"
  [ "$ringstat_sha" = - ] || say "captured_ringstat_sha256=$ringstat_sha size=$ringstat_size"
  say "rollback_generation=$generated"
  say "rollback_path=$final"

  if [ "$DRY" = 0 ]; then
    prepare_store
    if [ -e "$final" ]; then
      validate_generation "$generated"
      already_captured=1
    else
      umask 077
      TMP_CAPTURE="$(mktemp -d "$STORE/.capture.XXXXXX")"
      chmod 0700 "$TMP_CAPTURE"
      copy_fd "$relay_fd" "$TMP_CAPTURE/multiverse-relay" "$relay_sha"
      copy_fd "$archive_fd" "$TMP_CAPTURE/multiverse-archive" "$archive_sha"
      if [ "$ringstat_sha" != - ]; then
        copy_fd "$ringstat_fd" "$TMP_CAPTURE/ringstat" "$ringstat_sha"
      fi
    fi
  fi

  relay_pid_after="$(main_pid multiverse-relay.service)"
  archive_pid_after="$(main_pid multiverse-archive.service)"
  [ "$relay_pid_after" = "$relay_pid" ] && [ "$archive_pid_after" = "$archive_pid" ] \
    || die "a service restarted during capture. The generation was rejected"
  [ "$(sha256sum "$PROC_ROOT/$relay_pid_after/exe" | awk '{print $1}')" = "$relay_sha" ] \
    || die "the running relay changed during capture. The generation was rejected"
  [ "$(sha256sum "$PROC_ROOT/$archive_pid_after/exe" | awk '{print $1}')" = "$archive_sha" ] \
    || die "the running archive changed during capture. The generation was rejected"

  installed_relay_after="$(sha_path "$relay_path")"
  installed_archive_after="$(sha_path "$archive_path")"
  installed_ringstat_after="$(sha_path "$ringstat_path")"
  [ "$installed_relay_after" = "$installed_relay" ] \
    && [ "$installed_archive_after" = "$installed_archive" ] \
    && [ "$installed_ringstat_after" = "$installed_ringstat" ] \
    || die "an installed binary changed during capture. The generation was rejected"
  if [ "$ringstat_sha" != - ]; then
    [ "$(sha_fd "$ringstat_fd")" = "$ringstat_sha" ] \
      || die "the ringstat handle changed during capture. The generation was rejected"
  fi

  if [ "$DRY" = 1 ]; then
    say "generation_status=dry-run-verified"
    say "Dry run: no rollback directory or artifact was written."
  elif [ "$already_captured" = 1 ]; then
    say "generation_status=already-captured"
  elif [ -n "$TMP_CAPTURE" ]; then
    captured_at="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
    {
      printf '%s  multiverse-relay\n' "$relay_sha"
      printf '%s  multiverse-archive\n' "$archive_sha"
      [ "$ringstat_sha" = - ] || printf '%s  ringstat\n' "$ringstat_sha"
    } >"$TMP_CAPTURE/SHA256SUMS"
    {
      printf 'format\tmultiverse-binary-generation-v1\n'
      printf 'generation\t%s\n' "$generated"
      printf 'captured_at_utc\t%s\n' "$captured_at"
      printf 'relay_pid\t%s\n' "$relay_pid"
      printf 'archive_pid\t%s\n' "$archive_pid"
      printf 'relay_sha256\t%s\n' "$relay_sha"
      printf 'relay_size\t%s\n' "$relay_size"
      printf 'archive_sha256\t%s\n' "$archive_sha"
      printf 'archive_size\t%s\n' "$archive_size"
      printf 'ringstat_sha256\t%s\n' "$ringstat_sha"
      printf 'ringstat_size\t%s\n' "$ringstat_size"
      printf 'installed_relay_sha256\t%s\n' "$installed_relay"
      printf 'installed_archive_sha256\t%s\n' "$installed_archive"
      printf 'installed_ringstat_sha256\t%s\n' "$installed_ringstat"
    } >"$TMP_CAPTURE/manifest.tsv"
    chmod 0600 "$TMP_CAPTURE/SHA256SUMS" "$TMP_CAPTURE/manifest.tsv"
    if ! mv -T -- "$TMP_CAPTURE" "$final"; then
      if [ -d "$final" ]; then
        validate_generation "$generated"
        rm -rf -- "$TMP_CAPTURE"
      else
        die "cannot publish the captured generation $generated"
      fi
    fi
    TMP_CAPTURE=""
    validate_generation "$generated"
    say "generation_status=captured"
  fi

  exec {relay_fd}<&-
  exec {archive_fd}<&-
  [ -z "$ringstat_fd" ] || exec {ringstat_fd}<&-
}

restore() {
  local dir="$STORE/$GENERATION" dst staged name expected bin_mode
  local ringstat_path="$BIN_DIR/ringstat" remove_ringstat=0
  [ -d "$BIN_DIR" ] || die "$BIN_DIR is not a directory"
  [ ! -L "$BIN_DIR" ] || die "$BIN_DIR is a symbolic link"
  [ "$(owner_of "$BIN_DIR")" = "$CURRENT_UID" ] \
    || die "$BIN_DIR is not owned by uid $CURRENT_UID"
  bin_mode="$(mode_of "$BIN_DIR")"
  case "${bin_mode: -2}" in
    *[2367]*) die "$BIN_DIR is writable by its group or by other users" ;;
  esac
  validate_generation "$GENERATION"

  # A generation that has no ringstat records absence, not "leave whatever is
  # there." Validate a later installed path before the first replacement. An
  # unlink removes only that directory entry, but symbolic links and other file
  # types are still refused so the operator sees unexpected state.
  if [ "$VALID_RINGSTAT_SHA" = - ] && { [ -e "$ringstat_path" ] || [ -L "$ringstat_path" ]; }; then
    [ -f "$ringstat_path" ] || die "$ringstat_path is not a regular file"
    [ ! -L "$ringstat_path" ] || die "$ringstat_path is a symbolic link"
    [ "$(owner_of "$ringstat_path")" = "$CURRENT_UID" ] \
      || die "$ringstat_path is not owned by uid $CURRENT_UID"
    remove_ringstat=1
  fi

  say "restore_generation=$GENERATION"
  say "restore_relay_sha256=$VALID_RELAY_SHA"
  say "restore_archive_sha256=$VALID_ARCHIVE_SHA"
  say "restore_ringstat_sha256=$VALID_RINGSTAT_SHA"
  [ "$remove_ringstat" = 0 ] || say "restore_ringstat_action=remove $ringstat_path"
  if [ "$DRY" = 1 ]; then
    say "Dry run: the named generation verifies. No installed path was changed."
    say "No service was restarted."
    return 0
  fi

  umask 077
  TMP_RESTORE="$(mktemp -d "$BIN_DIR/.restore.XXXXXX")"
  chmod 0700 "$TMP_RESTORE"
  for name in multiverse-relay multiverse-archive; do
    install -m 0755 "$dir/$name" "$TMP_RESTORE/$name"
  done
  if [ "$VALID_RINGSTAT_SHA" != - ]; then
    install -m 0755 "$dir/ringstat" "$TMP_RESTORE/ringstat"
  fi
  [ "$(sha256sum "$TMP_RESTORE/multiverse-relay" | awk '{print $1}')" = "$VALID_RELAY_SHA" ] \
    || die "the staged relay restore failed verification"
  [ "$(sha256sum "$TMP_RESTORE/multiverse-archive" | awk '{print $1}')" = "$VALID_ARCHIVE_SHA" ] \
    || die "the staged archive restore failed verification"
  if [ "$VALID_RINGSTAT_SHA" != - ]; then
    [ "$(sha256sum "$TMP_RESTORE/ringstat" | awk '{print $1}')" = "$VALID_RINGSTAT_SHA" ] \
      || die "the staged ringstat restore failed verification"
  fi

  for name in multiverse-relay multiverse-archive; do
    dst="$BIN_DIR/$name"
    expected="$VALID_RELAY_SHA"
    [ "$name" = multiverse-archive ] && expected="$VALID_ARCHIVE_SHA"
    staged="$TMP_RESTORE/$name"
    mv -fT -- "$staged" "$dst"
    [ "$(sha_path "$dst")" = "$expected" ] || die "the restored $dst failed verification"
  done
  if [ "$VALID_RINGSTAT_SHA" != - ]; then
    mv -fT -- "$TMP_RESTORE/ringstat" "$BIN_DIR/ringstat"
    [ "$(sha_path "$BIN_DIR/ringstat")" = "$VALID_RINGSTAT_SHA" ] \
      || die "the restored $BIN_DIR/ringstat failed verification"
  elif [ "$remove_ringstat" = 1 ]; then
    [ -f "$ringstat_path" ] && [ ! -L "$ringstat_path" ] \
      && [ "$(owner_of "$ringstat_path")" = "$CURRENT_UID" ] \
      || die "$ringstat_path became unsafe during restore"
    unlink -- "$ringstat_path"
    [ ! -e "$ringstat_path" ] && [ ! -L "$ringstat_path" ] \
      || die "$ringstat_path still exists after restore"
  fi
  rm -rf -- "$TMP_RESTORE"
  TMP_RESTORE=""
  say "restored_generation=$GENERATION"
  if [ "$VALID_RINGSTAT_SHA" = - ]; then
    say "The generation has no ringstat. The installed path is now absent."
  fi
  say "No service was restarted. Running processes still use their open executable inodes."
}

case "$ACTION" in
  capture) capture ;;
  restore) restore ;;
esac
