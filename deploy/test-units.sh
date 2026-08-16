#!/usr/bin/env bash
# Check the systemd units in this kit without changing a host.
#
# THE ONE RULE THIS FILE EXISTS FOR: the archive must not name the relay in
# Wants=. Wants= is a start-time pull, so `systemctl restart multiverse-archive`
# would start a relay an operator had just stopped, and that silently defeats the
# record-preserving sequence in RESTART-POLICY.md.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
archive="$HERE/systemd/multiverse-archive.service"
relay="$HERE/systemd/multiverse-relay.service"

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

echo "systemd units OK"
