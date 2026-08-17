#!/usr/bin/env bash
# The forced command for the CI deployment key — the whole boundary between a
# GitHub Actions job and this host.
#
# It is installed by provision.sh's `directories` phase, like every other kit
# script, and it is named in one `authorized_keys` line:
#
#   command="/opt/multiverse/deploy/ci-gate.sh",no-port-forwarding,
#   no-agent-forwarding,no-pty,no-X11-forwarding ssh-ed25519 AAAA... mv-deploy-ci
#
# WHY THIS FILE IS THE WHOLE BOUNDARY. The `ubuntu` account has
# `(ALL) NOPASSWD: ALL`. A key in its `authorized_keys` without a forced command
# is root on this box, permanently, for whoever holds the private half — and the
# private half of the CI key lives in GitHub, where anyone who can change a
# workflow file on a branch can read it into a job. So the key gets a forced
# command, and this script is it. Everything below follows from that:
#
#   * The dispatcher lives in /opt/multiverse/deploy, which is root:root 0755.
#     `ubuntu` cannot rewrite the thing that constrains `ubuntu`.
#   * $SSH_ORIGINAL_COMMAND is split into words and matched against a fixed verb
#     table. It is never `eval`-ed, never interpolated into a shell, and globbing
#     is off for the whole process, so no argument can become a filename set.
#   * Every argument is checked against a narrow pattern before it is used.
#   * The default is refusal. A verb that is not in the table is a log line and
#     exit 3.
#
#   ssh <host> verify                     provision.sh --only verify
#   ssh <host> phase <name>               provision.sh --only <allowlisted name>
#   ssh <host> kit-receive <sha7>         tar on stdin -> a fresh kit inbox dir
#   ssh <host> kit-install <dir>          that kit's provision.sh --only directories
#   ssh <host> binaries-install <dir>     stage that kit's binaries, then --only binaries
#   ssh <host> restart-relay [--dry-run]  deploy/restart-relay.sh
#   ssh <host> restart-archive --dry-run  deploy/restart-archive.sh --dry-run
#   ssh <host> receipt                    host facts for a deployment receipt
#
# Add --dry-run to any mutating verb and it reaches provision.sh, which routes
# every mutation through one `run` helper — so the rehearsal is honest.
#
# WHAT IS DELIBERATELY NOT OFFERED, and must not be added without reading the
# reasoning first:
#
#   * A REAL archive restart. `restart-archive` is accepted with --dry-run and
#     with nothing else. An archive restart replays the whole ledger, costs the
#     map a full relay outage for the length of the replay, and its memory
#     headroom is still an open question (RESTART-POLICY.md, "Archive restart";
#     SIZING.md, "Archive memory"). A restart that needs an operator to type a
#     proof that the replay fits is not a restart a scheduled job should be able
#     to start. It stays a hand-run act until that work is closed.
#   * monitor.sh. Not an oversight — running it from here would BREAK the
#     alerting it appears to check. monitor.sh sources /etc/multiverse/deploy.env
#     under `set -a`, so MV_ALERT_KIND cannot be suppressed by a caller, and its
#     report() writes the shared sev.* state that the five-minute timer uses for
#     change detection. An out-of-band pass that observes a new CRIT records it
#     as already-alerted, and the timer then never alerts on it. CI reading the
#     monitor must not be able to silence the monitor, so `verify` reports the
#     monitor's FRESHNESS from outside instead and never runs a pass.
#   * Any provision phase that installs packages, edits the firewall, touches
#     certbot, mints a credential, or drives systemd. See ALLOWED_PHASES.
#   * A shell, a subsystem, port forwarding, or an agent. The authorized_keys
#     options above deny those; this script never re-opens them.
set -euo pipefail
# Globbing off for the whole process. An argument that reaches an unquoted
# expansion cannot become a filename list, which removes a whole class of
# mistake from every line below rather than one line at a time.
set -f

# ---------------------------------------------------------------- read seams
#
# Each one defaults to the real path and is overridden only by
# deploy/test-ci-gate.sh, so the verb table can be exercised on a workstation
# with no host, no root, and no ssh.
: "${MV_CI_KIT_ROOT:=/opt/multiverse/deploy}"
: "${MV_CI_KIT_INBOX:=/home/ubuntu/ci-kits}"
: "${MV_CI_STAGE_DIR:=/home/ubuntu/multiverse-stage}"
: "${MV_CI_GATE_LOG:=/var/log/multiverse/ci-gate.log}"
: "${MV_CI_HOLD:=/run/lock/bibites-archive-deploy.HOLD-README}"
: "${MV_CI_LOCK:=/run/lock/bibites-archive-deploy.lock}"
: "${MV_CI_SUDO:=sudo}"
: "${MV_CI_NOW:=}"

# The provision phases CI may run, and nothing else.
#
#   directories   installs the kit, including this file. No service is touched.
#   binaries      replaces the binaries on disk by rename. Explicitly does NOT
#                 restart anything: a new binary on disk is not a running new
#                 binary (ship.sh's closing note, provision.sh phase_binaries).
#   nginxfront    renders the front door and reloads nginx. A reload, not a
#                 restart, and it costs no connection.
#   streamorigin  the optional HLS origin. Independent of the relay and archive.
#   verify        reads. Changes nothing.
#
# Every other phase is excluded on purpose, and the reason is not uniform:
#   preflight/packages/upgrades  apt writes on a live host
#   account                      creates a system user
#   swap                         mkswap on a 1.9 GB box
#   envfiles                     writes /etc/multiverse/*.env, the deployment's
#                                identity — an operator act, not a job's
#   firewall                     ufw. A wrong rule locks CI, and the operator,
#                                out of the only way back in
#   nginxacme/tls                certbot. Let's Encrypt rate limits are per week
#                                and a scheduled job can exhaust them
#   bootstrap                    mints the archive credential AND requires the
#                                relay stopped. Both are hand-run acts
#   systemd                      daemon-reload plus `systemctl start` on the
#                                relay and archive. Starting a unit an operator
#                                stopped is exactly the failure RESTART-POLICY.md
#                                documents under "CAUTION"
ALLOWED_PHASES="directories binaries nginxfront streamorigin verify"

now()  { [ -n "$MV_CI_NOW" ] && { printf '%s' "$MV_CI_NOW"; return; }; date -u +%FT%TZ; }
say()  { printf '     %s\n' "$*"; }
step() { printf '\n==== %s\n' "$*"; }

# log <result> <text> — one line, UTC, appended.
#
# The log is written through sudo because /var/log/multiverse is 0750
# multiverse:multiverse and this script runs as `ubuntu`, which cannot even
# traverse it. It is best-effort on purpose: a refusal must still be a refusal
# when the log cannot be written, or an unwritable log becomes a way in.
log() {
  local result="$1" text="$2" line
  # Strip control characters and truncate. The text comes from the far side of
  # an ssh connection, and a log a person reads must not be able to carry an
  # escape sequence or a forged newline.
  text="$(printf '%s' "$text" | tr -d '\000-\037\177' | cut -c1-200)"
  line="$(printf '%s %s %s' "$(now)" "$result" "$text")"
  if [ -n "$MV_CI_SUDO" ]; then
    [ -e "$MV_CI_GATE_LOG" ] || \
      $MV_CI_SUDO install -m 0640 -o root -g root /dev/null "$MV_CI_GATE_LOG" 2>/dev/null || true
    printf '%s\n' "$line" | $MV_CI_SUDO tee -a "$MV_CI_GATE_LOG" >/dev/null 2>&1 || true
  else
    mkdir -p "$(dirname "$MV_CI_GATE_LOG")" 2>/dev/null || true
    printf '%s\n' "$line" >>"$MV_CI_GATE_LOG" 2>/dev/null || true
  fi
}

# refuse <text> — log it, say it, exit 3. Every rejection lands here so that
# "what did CI try" is one grep.
refuse() {
  log REFUSE "$*"
  printf 'ci-gate: refused: %s\n' "$*" >&2
  exit 3
}

# A held archive deploy is another operator's lock. The README beside it is the
# contract, and the contract is that nothing else deploys while it exists.
# Read-only verbs are still allowed: a daily verify going red because a person
# is working is noise, and noise is how a check stops being read.
#
# Note what is NOT here: an override. restart-relay.sh offers
# --ignore-archive-hold for an operator who has read the hold and knows it does
# not apply. CI never passes it and has no way to ask for it. A person can judge
# that another person's hold is irrelevant; a scheduled job cannot.
refuse_if_held() {
  [ -e "$MV_CI_HOLD" ] || return 0
  refuse "$1: an archive deploy hold is in place ($MV_CI_HOLD). Nothing deploys until it is released."
}

# locked <command...> — run a mutation under the SAME exclusive lock the
# hand-written /home/ubuntu/deploy-*.sh scripts take.
#
# This is the point of the whole exercise. Those scripts run as
#
#   sudo flock -n /run/lock/bibites-archive-deploy.lock bash deploy-<sha>.sh
#
# so two operators cannot deploy at once. If CI took a different lock, or none,
# it would serialize against nothing and the guarantee would be gone the day
# CI started being used — which is exactly when a second actor first exists.
# `-n` rather than a wait: a deployment that queues behind an unknown holder is
# a deployment nobody is watching.
# -E 75 matters: flock's default "could not acquire" code is 1, which is also a
# perfectly ordinary failure code from the wrapped command. Without -E, "somebody
# else is deploying" and "provision.sh failed" are the same observation, and the
# receipt would record the wrong one.
locked() {
  local rc=0
  $MV_CI_SUDO flock -n -E 75 "$MV_CI_LOCK" "$@" || rc=$?
  [ "$rc" = 75 ] && refuse "another deployment holds $MV_CI_LOCK. Do not run two."
  return "$rc"
}

# ok_token <value> — the pattern every free-form argument must match. Deliberately
# narrow: letters, digits, dot, dash, underscore, slash. No spaces, no quotes, no
# $ ` \ ; & | < > ( ) newline, and no leading dash that could become a flag.
ok_token() {
  case "$1" in
    -*|*..*|'') return 1 ;;
  esac
  printf '%s' "$1" | grep -Eq '^[A-Za-z0-9._/-]+$'
}

# resolve_kit_dir <arg> — resolve a caller-supplied kit directory and prove it is
# one of ours. The caller names a directory; it must be under the inbox and it
# must exist. `..` is already rejected by ok_token, and the prefix check is the
# second of the two so that neither alone is load-bearing.
#
# The answer comes back in KIT_DIR rather than on stdout, and that is not a
# style choice: `refuse` ends in `exit`, and an `exit` inside a command
# substitution ends only the subshell. A refusal that reached this function
# through $(...) would print its message, set the variable to empty, and let the
# caller carry on — which is the exact shape of a gate that does not gate.
KIT_DIR=""
resolve_kit_dir() {
  local arg="$1" dir
  ok_token "$arg" || refuse "kit directory is not a plain path: $arg"
  case "$arg" in
    /*) dir="$arg" ;;
     *) dir="$MV_CI_KIT_INBOX/$arg" ;;
  esac
  case "$dir" in
    "$MV_CI_KIT_INBOX"/*) : ;;
    *) refuse "kit directory is outside $MV_CI_KIT_INBOX: $arg" ;;
  esac
  [ -d "$dir" ] || refuse "no such kit directory: $arg"
  KIT_DIR="$dir"
}

# ---------------------------------------------------------------- the verbs

verb_verify() {
  local dry="$1"
  step "verify"
  local args=(--only verify)
  [ "$dry" = 1 ] && args+=(--dry-run)
  local rc=0
  $MV_CI_SUDO "$MV_CI_KIT_ROOT/provision.sh" "${args[@]}" || rc=$?

  # The monitor is READ FROM THE OUTSIDE, never run. See the header: a pass from
  # here would write the sev.* state the timer compares against, and a CRIT this
  # job discovered would be recorded as already-alerted. What CI is allowed to
  # know is whether the watcher is alive and recent, which is visible without
  # touching it.
  step "monitor freshness (read only — the monitor is NOT run from CI)"
  say "timer:   $(systemctl is-active multiverse-monitor.timer 2>/dev/null || echo unknown)"
  # 0 IS THE HEALTHY VALUE EVEN WHILE A CHECK IS CRIT. monitor.sh carries
  # severity in the alert and in its printed `worst:` line, so a completed pass
  # exits 0 whatever it found; a non-zero here means the pass could not run,
  # and 2 is the unreadable /etc/multiverse/deploy.env that silences alerting
  # altogether. It is reported rather than failed on, because ExecMainStatus
  # still holds the previous tick's value for up to five minutes after a kit is
  # installed.
  say "service: $(systemctl show -p ExecMainStatus --value multiverse-monitor.service 2>/dev/null || echo unknown) (last exit status; 0 is healthy even at CRIT, non-zero means the pass could not RUN)"
  # Read through sudo: /var/lib/multiverse is 0750 multiverse:multiverse and this
  # script runs as `ubuntu`, which cannot traverse it.
  local newest
  newest="$($MV_CI_SUDO find /var/lib/multiverse/monitor -maxdepth 1 -type f -newermt '-15 minutes' 2>/dev/null | head -1 || true)"
  if [ -n "$newest" ]; then
    say "state:   written within the last 15 minutes — the monitor is running"
  else
    say "state:   NOTHING written in the last 15 minutes. The five-minute timer is"
    say "         not completing passes, which means no alert can be raised by the"
    say "         thing that cannot start. This is a finding, not a formality."
    rc=1
  fi
  return "$rc"
}

verb_phase() {
  local name="$1" dry="$2"
  ok_token "$name" || refuse "phase name is not a plain word: $name"
  printf '%s\n' $ALLOWED_PHASES | grep -qx "$name" \
    || refuse "phase not offered through CI: $name (allowed: $ALLOWED_PHASES)"
  [ "$name" = verify ] || [ "$dry" = 1 ] || refuse_if_held "phase $name"
  step "phase $name"
  local args=(--only "$name")
  [ "$dry" = 1 ] && args+=(--dry-run)
  if [ "$name" = verify ] || [ "$dry" = 1 ]; then
    $MV_CI_SUDO "$MV_CI_KIT_ROOT/provision.sh" "${args[@]}"
  else
    locked "$MV_CI_KIT_ROOT/provision.sh" "${args[@]}"
  fi
}

# kit-receive <sha7> — the file transfer, and the reason there is no scp here.
#
# A forced command replaces whatever the client asked for, so `scp host:path`
# never runs the remote scp binary and `rsync --rsync-path` would mean
# allowlisting rsync's own option surface, which is large and is a shell in
# places. A tar stream on stdin needs no extra allowlist at all: the verb takes
# one token, the payload arrives on a pipe, and tar is told exactly where to put
# it with no absolute paths and no traversal.
verb_kit_receive() {
  local sha="$1" dir
  ok_token "$sha" || refuse "kit-receive needs a plain revision token"
  printf '%s' "$sha" | grep -Eq '^[0-9a-f]{7,40}$' \
    || refuse "kit-receive needs a git revision in hex, got: $sha"
  refuse_if_held kit-receive
  dir="$MV_CI_KIT_INBOX/$(now | tr -d ':-')-${sha:0:7}"
  step "kit-receive $sha"
  mkdir -p "$dir"
  # -P is NOT passed, --absolute-names is NOT passed: tar therefore strips a
  # leading / and refuses to write outside $dir even if the archive asks.
  tar -xzf - -C "$dir" --no-same-owner
  if [ -f "$dir/SHA256SUMS" ]; then
    ( cd "$dir" && sha256sum -c SHA256SUMS ) >/dev/null \
      || refuse "the received kit does not match its own SHA256SUMS"
    say "checksums verified"
  fi
  say "$dir"
  printf 'KIT_DIR=%s\n' "$dir"
}

# Both install verbs hand off to deploy/deploy.sh in the STAGED kit, and neither
# repeats a step of it here. That script carries the kit-digest assertion, the
# environment-file assertion with its automatic restore, and the staged-binary
# comparison — the parts of the hand-written host scripts worth keeping. A copy
# of any of them in this file would be a second implementation, and the CI path
# and the hand path would then be "consistent" only until one of them changed.
#
# The digest an install is checked against is supplied by the caller with
# --expect-kit-digest, which CI computes from the payload it built.
verb_kit_install() {
  local dry="$2" want="${3:-}" dir
  resolve_kit_dir "$1"; dir="$KIT_DIR"
  [ -x "$dir/deploy/deploy.sh" ] || refuse "no executable deploy/deploy.sh in $dir"
  [ "$dry" = 1 ] || refuse_if_held kit-install
  step "kit-install $dir"
  local args=(--kit "$dir")
  [ -n "$want" ] && args+=(--expect-kit-digest "$want")
  if [ "$dry" = 1 ]; then
    args+=(--dry-run)
    $MV_CI_SUDO "$dir/deploy/deploy.sh" "${args[@]}"
  else
    locked "$dir/deploy/deploy.sh" "${args[@]}"
  fi
}

verb_binaries_install() {
  local dry="$2" want="${3:-}" dir
  resolve_kit_dir "$1"; dir="$KIT_DIR"
  [ -x "$dir/deploy/deploy.sh" ] || refuse "no executable deploy/deploy.sh in $dir"
  [ -d "$dir/stage" ] || refuse "no stage/ directory in $dir"
  [ "$dry" = 1 ] || refuse_if_held binaries-install
  step "binaries-install $dir"
  local args=(--kit "$dir" --stage "$dir/stage" --binaries)
  [ -n "$want" ] && args+=(--expect-kit-digest "$want")
  if [ "$dry" = 1 ]; then
    args+=(--dry-run)
    $MV_CI_SUDO "$dir/deploy/deploy.sh" "${args[@]}"
  else
    locked "$dir/deploy/deploy.sh" "${args[@]}"
  fi
}

# restart-relay.sh takes its own exclusive lock (/run/lock/bibites-relay-restart.lock),
# reads the archive hold itself, and writes a receipt on stdout. It is therefore
# NOT wrapped in locked() here — wrapping it would make CI hold the archive
# deploy lock for the length of a relay restart, which is a different and larger
# claim than the script itself makes.
verb_restart_relay() {
  local dry="$1" tag="${2:-}"
  local script="$MV_CI_KIT_ROOT/restart-relay.sh"
  [ -x "$script" ] || refuse "restart-relay.sh is not installed on this host yet"
  [ "$dry" = 1 ] || refuse_if_held restart-relay
  step "restart-relay"
  local args=()
  [ "$dry" = 1 ] && args+=(--dry-run)
  # --reason lands in the script's own receipt. The tag is a single validated
  # token (the workflow sends its run id), so the text handed to --reason is
  # assembled here from constants plus that one token and can carry nothing else.
  args+=(--reason "CI${tag:+ run $tag}")
  $MV_CI_SUDO "$script" "${args[@]}"
}

# restart-archive is accepted ONLY as a rehearsal. The header says why; this is
# where it is enforced. Note the order: the refusal is decided before the script
# is even looked for, so no argument can reach it.
verb_restart_archive() {
  local dry="$1"
  [ "$dry" = 1 ] || refuse "a real archive restart is not offered through CI.
     It replays the whole ledger, costs the map a full relay outage for the
     length of the replay, and needs an operator's measured proof that the
     replay fits in memory (RESTART-POLICY.md, 'Archive restart'). Run it by
     hand. CI offers the rehearsal only."
  local script="$MV_CI_KIT_ROOT/restart-archive.sh"
  [ -x "$script" ] || refuse "restart-archive.sh is not installed on this host yet"
  step "restart-archive --dry-run"
  $MV_CI_SUDO "$script" --dry-run
}

# receipt — the facts a deployment record needs, and nothing that is a secret.
#
# The kit listing digest is computed the way ops/source-lock.yaml defines it, so
# the number in a CI receipt and the number in the lock are comparable rather
# than merely similar.
verb_receipt() {
  step "receipt"
  printf 'utc=%s\n' "$(now)"
  printf 'host_kit_dir=%s\n' "$MV_CI_KIT_ROOT"
  # The kit listing digest, asked of deploy.sh rather than computed here. That
  # script owns the definition; a second copy of it in this file is how the two
  # would silently stop agreeing.
  printf 'kit_listing_digest=%s\n' \
    "$("$MV_CI_KIT_ROOT/deploy.sh" --print-kit-digest "$MV_CI_KIT_ROOT" 2>/dev/null || echo unmeasured)"
  printf 'kit_file_count=%s\n' "$(find "$MV_CI_KIT_ROOT" -type f 2>/dev/null | wc -l)"
  # The recorded binary checksums, verbatim. phase_binaries writes this file, so
  # it is the host's own claim about what is installed rather than a re-derived
  # one — and it is what a receipt can be checked against later.
  if [ -r /etc/multiverse/BINARIES.sha256 ]; then
    while read -r sum name; do
      printf 'binary_%s=%s\n' "$(printf '%s' "$name" | tr '.-' '__')" "$sum"
    done < /etc/multiverse/BINARIES.sha256
  else
    printf 'binary_record=unreadable\n'
  fi
  local u
  for u in multiverse-relay multiverse-archive nginx \
           multiverse-monitor.timer multiverse-backup.timer multiverse-host-sample.timer; do
    printf 'unit_%s=%s\n' "$(printf '%s' "$u" | tr '.-' '__')" \
      "$(systemctl is-active "$u" 2>/dev/null || echo unknown)"
  done
  printf 'uptime=%s\n' "$(uptime -p 2>/dev/null || echo unknown)"
  printf 'reboot_required=%s\n' "$([ -e /var/run/reboot-required ] && echo yes || echo no)"
  printf 'archive_hold=%s\n' "$([ -e "$MV_CI_HOLD" ] && echo held || echo clear)"
}

# ---------------------------------------------------------------- dispatch

main() {
  local raw="${SSH_ORIGINAL_COMMAND:-}"
  [ -n "$raw" ] || refuse "no command. This key is a deployment key and has no shell."

  # A newline is refused rather than tolerated, and the reason is `read` itself:
  # `read -ra` stops at the first one, so "verify\nrm -rf /" would run `verify`
  # and DROP the rest without a word. Dropping is safe and silence is not — a
  # gate that quietly executes some other command than the one it was sent
  # cannot be audited from its own log. Anything else outside printable ASCII
  # goes the same way; no legitimate verb needs it.
  case "$raw" in
    *[![:print:][:space:]]*|*$'\n'*|*$'\r'*)
      refuse "the command contains a newline or a control character" ;;
  esac

  # Word-split only. No eval, no quote processing, no expansion: the far side
  # supplies tokens, not syntax.
  local words=()
  read -ra words <<<"$raw"
  [ "${#words[@]}" -gt 0 ] || refuse "empty command"

  local verb="${words[0]}"
  local args=("${words[@]:1}")

  # THE ALLOWLIST, in one line, on purpose. A reviewer asking "what can this key
  # do" reads exactly this.
  #
  # It is checked BEFORE the flags below, so that `rm -rf /` is refused as "not
  # an allowed verb: rm" rather than as "unknown flag: -rf". Both refuse, but
  # only one of them tells the truth about why, and the log is read by a person
  # trying to understand what reached this box.
  case "$verb" in
    verify|phase|kit-receive|kit-install|binaries-install|restart-relay|restart-archive|receipt) : ;;
    *) refuse "not an allowed verb: $verb" ;;
  esac

  # --dry-run is the one flag any verb may carry, and it is pulled out here so
  # that no verb has to parse flags itself.
  local dry=0 rest=() a
  for a in ${args[@]+"${args[@]}"}; do
    case "$a" in
      --dry-run) dry=1 ;;
      -*) refuse "unknown flag: $a" ;;
      *) rest+=("$a") ;;
    esac
  done

  # The ALLOW line is written here and not at the top of main: a rejected verb
  # must appear in the log as REFUSE only, never as an ALLOW followed by a
  # REFUSE, or the log stops being answerable.
  log ALLOW "$raw"

  case "$verb" in
    verify)
      [ "${#rest[@]}" -eq 0 ] || refuse "verify takes no argument"
      verb_verify "$dry" ;;
    phase)
      [ "${#rest[@]}" -eq 1 ] || refuse "phase takes exactly one phase name"
      verb_phase "${rest[0]}" "$dry" ;;
    kit-receive)
      [ "${#rest[@]}" -eq 1 ] || refuse "kit-receive takes exactly one revision"
      verb_kit_receive "${rest[0]}" ;;
    kit-install)
      [ "${#rest[@]}" -ge 1 ] && [ "${#rest[@]}" -le 2 ] \
        || refuse "kit-install takes a kit directory and an optional expected digest"
      [ "${#rest[@]}" = 2 ] && { ok_token "${rest[1]}" || refuse "the expected digest is not a plain token"; }
      verb_kit_install "${rest[0]}" "$dry" "${rest[1]:-}" ;;
    binaries-install)
      [ "${#rest[@]}" -ge 1 ] && [ "${#rest[@]}" -le 2 ] \
        || refuse "binaries-install takes a kit directory and an optional expected digest"
      [ "${#rest[@]}" = 2 ] && { ok_token "${rest[1]}" || refuse "the expected digest is not a plain token"; }
      verb_binaries_install "${rest[0]}" "$dry" "${rest[1]:-}" ;;
    restart-relay)
      [ "${#rest[@]}" -le 1 ] || refuse "restart-relay takes at most one run tag"
      if [ "${#rest[@]}" = 1 ]; then
        ok_token "${rest[0]}" || refuse "the run tag is not a plain word"
        verb_restart_relay "$dry" "${rest[0]}"
      else
        verb_restart_relay "$dry" ""
      fi ;;
    restart-archive)
      [ "${#rest[@]}" -eq 0 ] || refuse "restart-archive takes no argument"
      verb_restart_archive "$dry" ;;
    receipt)
      [ "${#rest[@]}" -eq 0 ] || refuse "receipt takes no argument"
      verb_receipt ;;
  esac
}

main
