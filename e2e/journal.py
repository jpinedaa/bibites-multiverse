#!/usr/bin/env python3
"""Read a sidecar's durable custody journal.

The journal is the custody record of decision D2, so the kill test has to read it
directly: an organism that is in neither world but sits in an ``open`` or ``in_flight``
journal entry has not been lost, it is in transit and one sidecar still owns it.

Format (``go/internal/journal``): one JSON object per line, append-only, ``op`` is
``create`` or ``status``, replay applies records in file order and the last status wins.
A torn final line is the tail of a write a ``kill -9`` interrupted; it is skipped here for
the same reason the Go replay truncates it — it was never durably journaled.

Usage:
    journal.py summary  <journal.log> [...]        one line per migration
    journal.py custody  <entityId> <journal.log>   custody count for that organism
    journal.py hash     <migrationId> <journal.log>
"""

import json
import sys

LIVE = ("open", "in_flight")


def replay(path):
    """migrationId -> state, in file order, last status wins."""
    states = {}
    order = []
    try:
        with open(path, "r", encoding="utf-8") as handle:
            lines = handle.read().splitlines()
    except FileNotFoundError:
        return states, order

    for line in lines:
        line = line.strip()
        if not line:
            continue
        try:
            record = json.loads(line)
        except json.JSONDecodeError:
            # A torn tail: the record never reached durable storage.
            continue

        mid = record.get("migrationId")
        if not mid:
            continue

        if record.get("op") == "create":
            entry = record.get("entry", {}) or {}
            states[mid] = {
                "migrationId": mid,
                "entityId": entry.get("entityId"),
                "payloadHash": entry.get("payloadHash"),
                "edge": entry.get("edge"),
                "sourceSector": entry.get("sourceSector"),
                "destSector": entry.get("destSector"),
                "direction": record.get("direction"),
                "status": "open",
                "acked": False,
                "bounceBack": record.get("bounceBack", False),
            }
            order.append(mid)
        else:
            state = states.get(mid)
            if state is None:
                continue
            if record.get("purge"):
                del states[mid]
                order.remove(mid)
                continue
            if record.get("status"):
                state["status"] = record["status"]
            if record.get("direction"):
                state["direction"] = record["direction"]
            if record.get("ackedUpstream") is not None:
                state["acked"] = record["ackedUpstream"]
            if record.get("bounceBack") is not None:
                state["bounceBack"] = record["bounceBack"]

    return states, order


def cmd_summary(paths):
    for path in paths:
        states, order = replay(path)
        for mid in order:
            state = states[mid]
            print(
                "{path} migrationId={migrationId} entityId={entityId} dir={direction} "
                "status={status} acked={acked} bounce={bounceBack} edge={edge} "
                "route={sourceSector}->{destSector} payloadHash={payloadHash}".format(
                    path=path, **state
                )
            )
    return 0


def cmd_custody(entity_id, paths):
    """How many live custody records name this organism, and where."""
    total = 0
    for path in paths:
        states, _ = replay(path)
        for state in states.values():
            if str(state["entityId"]) != str(entity_id):
                continue
            live = state["status"] in LIVE
            print(
                "{path} migrationId={migrationId} dir={direction} status={status} "
                "live={live}".format(path=path, live=live, **state)
            )
            if live:
                total += 1
    print("custodyCount={0}".format(total))
    return 0


def cmd_hash(migration_id, paths):
    for path in paths:
        states, _ = replay(path)
        state = states.get(migration_id)
        if state:
            print("{0} {1} {2}".format(path, migration_id, state["payloadHash"]))
    return 0


def main(argv):
    if len(argv) < 3:
        print(__doc__, file=sys.stderr)
        return 2

    verb = argv[1]
    if verb == "summary":
        return cmd_summary(argv[2:])
    if verb == "custody":
        return cmd_custody(argv[2], argv[3:])
    if verb == "hash":
        return cmd_hash(argv[2], argv[3:])

    print("unknown verb {0!r}".format(verb), file=sys.stderr)
    return 2


if __name__ == "__main__":
    sys.exit(main(sys.argv))
