#!/usr/bin/env bash
# Check the systemd units in this kit without changing a host.
#
# THE RULE THIS FILE EXISTS FOR: the archive must not name the relay in Wants=.
# Wants= is a start-time pull, so `systemctl restart multiverse-archive` would
# start a relay an operator had just stopped, and that silently defeats the
# record-preserving sequence in RESTART-POLICY.md.
#
# The second rule has the same shape: a reboot must be able to hold the relay
# down, and the two things that make that possible — one boot pull-in, and a
# hold-down that is not `systemctl mask` — are checked below.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
archive="$HERE/systemd/multiverse-archive.service"
relay="$HERE/systemd/multiverse-relay.service"
provision="$HERE/provision.sh"
policy="$HERE/RESTART-POLICY.md"
hold_dropin="/etc/systemd/system/multiverse-relay.service.d/hold.conf"
hold_condition="ConditionPathExists=!/etc/multiverse/RELAY-HOLD"

fail() {
  echo "test-units: $1" >&2
  exit 1
}

wants="$(grep -E '^Wants=' "$archive" || true)"
[ -n "$wants" ] || fail "$archive has no Wants= line"
if printf '%s\n' "$wants" | grep -q 'multiverse-relay\.service'; then
  fail "the archive unit names the relay in Wants=. A manual archive restart would
    pull the relay back up and the map would run live through the whole replay
    with nothing recording it. Keep the relay in After= only (RESTART-POLICY.md)."
fi

grep -Eq '^After=.*multiverse-relay\.service' "$archive" ||
  fail "the archive unit must still order itself After= the relay"

# After= without Wants= is safe at boot ONLY because both units start from the
# target on their own. If either loses that, the fix above becomes a boot bug.
for unit in "$relay" "$archive"; do
  grep -Fqx 'WantedBy=multi-user.target' "$unit" ||
    fail "$unit must be WantedBy=multi-user.target, or After= alone leaves it unstarted at boot"
done

# The relay's boot pull-in must stay singular. `systemctl disable` is the
# hold-down that works on every host, and it removes exactly one thing: the
# WantedBy= above. A RequiredBy= or a second WantedBy= anywhere would start the
# relay in front of a replaying archive with the operator believing it held.
if grep -Eq '^(RequiredBy|Also)=' "$relay"; then
  fail "the relay unit adds a second boot pull-in. 'systemctl disable' removes the
    target's Wants only, so the reboot hold-down in RESTART-POLICY.md would no
    longer hold. Keep [Install] to WantedBy=multi-user.target alone."
fi

# The hold-down must not be `systemctl mask`. These units are installed as REAL
# FILES in /etc/systemd/system, and a mask is a symlink to /dev/null at that same
# path: "Failed to mask unit: File /etc/systemd/system/multiverse-relay.service
# already exists". It fails at the last step before the reboot. It was the
# documented instruction until 2026-08-16 and it must not come back.
if grep -Eq '^[^#]*systemctl (mask|unmask) multiverse-relay' "$policy"; then
  fail "$policy tells the operator to mask the relay. The units are real files in
    /etc/systemd/system, so a mask fails outright. The hold-down is 'systemctl
    disable', or the hold file below."
fi

grep -Fq "$hold_condition" "$provision" ||
  fail "$provision no longer installs the reboot hold-down condition
    '$hold_condition'. RESTART-POLICY.md documents it as the preferred hold-down."
grep -Fq "$hold_dropin" "$provision" ||
  fail "$provision no longer writes $hold_dropin"
grep -Fq "$hold_dropin" "$policy" ||
  fail "$policy no longer names $hold_dropin, so the document and the kit disagree
    about where the reboot hold-down lives."
grep -Fq "$hold_condition" "$policy" ||
  fail "$policy no longer quotes '$hold_condition'"

# Every timer in this kit, by glob rather than by a hand-kept list, so a new one
# is covered the day it is added. A timer that is not WantedBy=timers.target is
# installed, enabled and never fires; a timer whose service is missing fails at
# every tick. Both are silent until somebody reads a document that stopped
# updating.
timers=0
for timer in "$HERE"/systemd/*.timer; do
  [ -e "$timer" ] || continue
  timers=$((timers + 1))
  grep -Fqx 'WantedBy=timers.target' "$timer" ||
    fail "$timer must be WantedBy=timers.target, or enabling it starts nothing"
  service="${timer%.timer}.service"
  [ -f "$service" ] ||
    fail "$timer has no $service beside it; systemd would fail at every tick"
done
[ "$timers" -gt 0 ] || fail "no timer units found in $HERE/systemd"

# The viewer-presence unit runs as root for one reason — the nginx access log is
# root:adm — so every other privilege it would inherit has to be given back.
viewers="$HERE/systemd/multiverse-viewers.service"
[ -f "$viewers" ] || fail "missing $viewers"
for setting in 'NoNewPrivileges=true' 'ProtectSystem=strict' 'ProtectHome=true' \
               'LockPersonality=true'; do
  grep -Fqx "$setting" "$viewers" ||
    fail "$viewers must set $setting; it runs as root and reads a privileged log"
done

# The one path the unit exists to read is named in two files, and the front door
# moved it out of /var/log/nginx so that the kit could own its rotation policy.
# Read the directory out of the script's own default rather than repeating it
# here: a move that updates one file and not the other then fails this test
# instead of publishing "nobody is watching" forever on a live host.
presence="$HERE/viewers-presence.sh"
[ -f "$presence" ] || fail "missing $presence"
access_default="$(sed -n 's/^ACCESS_LOG="\${MV_ACCESS_LOG:-\(.*\)}"$/\1/p' "$presence")"
[ -n "$access_default" ] ||
  fail "$presence no longer sets a literal MV_ACCESS_LOG default, so the unit's
    ReadOnlyPaths cannot be checked against it"
grep -Fqx "ReadOnlyPaths=${access_default%/*}" "$viewers" ||
  fail "$viewers must set ReadOnlyPaths=${access_default%/*}, the directory of
    $presence's own MV_ACCESS_LOG default. The front door writes there and not
    in /var/log/nginx (deploy/nginx/multiverse-20-status.conf)."
# ProtectSystem=strict makes the whole tree read-only, so the one path it writes
# has to be named or the unit fails on its first tick.
grep -Eq '^ReadWritePaths=/var/www/multiverse-status$' "$viewers" ||
  fail "$viewers must declare ReadWritePaths for the directory it writes"
if grep -Eq '^RestrictAddressFamilies=.*AF_INET6' "$viewers"; then
  fail "$viewers dials 127.0.0.1 only; AF_INET6 is a privilege it does not need"
fi

# The contract in deploy/viewers-presence.sh promises a refresh at least every
# ten seconds, and systemd's default AccuracySec of one minute would break that
# promise while the timer file still said 10s.
viewers_timer="$HERE/systemd/multiverse-viewers.timer"
grep -Fqx 'OnUnitActiveSec=10s' "$viewers_timer" ||
  fail "$viewers_timer must fire every 10s: the publisher's start delay is added to it"
grep -Eq '^AccuracySec=[0-9]+(ms|s)$' "$viewers_timer" ||
  fail "$viewers_timer needs a tight AccuracySec; the default is one minute"
if grep -q 'RandomizedDelaySec' "$viewers_timer"; then
  fail "$viewers_timer must not jitter: a watcher reads the document's own age"
fi

echo "systemd units OK ($timers timers)"
