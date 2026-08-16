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

echo "systemd units OK"
