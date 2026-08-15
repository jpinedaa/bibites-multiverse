# Restart policy

The relay and archive are separate services.
A relay restart is usually short.
An archive restart replays the ledger, and its cost grows with that ledger.

This policy describes reusable behavior.
Private operations storage owns each restart window, approval, receipt, and incident record.

## Restart classes

| Class | Effect | Notice |
|---|---|---|
| Certificate reload | nginx loads a renewed pair. The relay and archive continue. | No notice is normally necessary. |
| Relay only | Peers disconnect, reconnect, and reclaim their slots. | Announce a planned restart. |
| Archive only | The website stops while the archive replays the ledger. | Announce it and state the expected replay time. |
| Host reboot | The relay and archive stop. The archive replays after boot. | Announce it and schedule it. |
| Unplanned restart | The effect matches the failed service and includes diagnosis time. | Explain it after service returns. |

## Certificate reload

`tls-deploy-hook.sh` installs the certificate and key through atomic replacements.
It reloads nginx after both files are present.

Existing WebSocket connections stay on the old nginx workers.
New connections use the renewed certificate.

The monitor compares the served certificate with the installed certificate.
This check detects a deploy-hook failure before expiry.

## Relay restart

The relay reads a small durable identity set at startup.
It does not replay the archive ledger.

During a relay restart, each sidecar reconnects on its backoff schedule.
The sidecar reclaims its existing slot and coordinate.

The sidecar journal preserves unsent custody records.
Some crossings can arrive later after the restart.

Credential creation is a startup operation.
Batch multiple new credentials into one planned relay restart.
Back up `ring.json` and `peers.json` after the batch.

## Archive restart

The archive replays every ledger record before it serves current views.
Use current record count and a measured replay rate for the estimate.

The reference benchmark produced these conservative memory factors:

- Resident state: approximately `0.30 KB` per ledger record.
- Streaming replay peak: approximately `0.18 KB` per ledger record.

The benchmark host replayed approximately 37,000 to 49,000 records each second.
A smaller host can be slower.

Use this estimate:

```text
replay_seconds = ledger_records / measured_records_per_second
resident_bytes = ledger_records * 0.30 KB
replay_peak_bytes = ledger_records * 0.18 KB
```

Measure the rate on the target host when possible.
Do not use an old elapsed time as the estimate for a growing ledger.

An archive that is not subscribed cannot record map activity.
If the map continues during replay, the permanent record has a gap.

If the record must remain complete, stop relay activity before the archive restart.
Start the relay only after the archive subscribes again.

Batch archive changes into one restart.
Do not restart the archive only to collect a diagnostic number.

## Host reboot

Treat a reboot as a combined relay and archive restart.

Before a planned reboot:

1. Read the full monitor result.
2. Make sure that projected archive memory is below the critical threshold.
3. Create and check an identity backup.
4. Create the approved off-host backup or snapshot.
5. Announce the window.

Stop relay activity before the reboot when ledger continuity is required.

After boot, check service enablement and archive subscription.
Do not infer archive health from the process state alone.

## Unplanned restart

systemd restarts failed services within configured rate limits.
It stops retrying after repeated failures.

This limit prevents an archive replay failure from consuming the host indefinitely.
The monitor must alert a person when a unit enters the failed state.

The host protects relay availability under memory pressure.
An archive failure loses record coverage, but a relay failure stops the map.

Do not improvise a destructive recovery on a live service.
Capture logs and state first.
Use the private incident runbook to resolve exact resources and commands.

## Pre-restart checks

Complete these checks before a planned restart:

1. Run `monitor.sh --verbose`.
2. Check free disk space and projected memory.
3. Count current ledger records.
4. Estimate replay time with the target-host rate.
5. Create and check the identity backup.
6. Record the reason, approval, and rollback condition.
7. Send the required notice.

If projected archive memory is critical, do not restart the archive.
Increase capacity or reduce the approved retained state first.

## Post-restart checks

Run these checks in order:

1. Make sure that the required units are active.
2. Check the relay health endpoint.
3. Check that the archive reports `relayConnected: true`.
4. Check that expected peers reclaimed their slots.
5. Check that the verifier-store count is unchanged.
6. Run `monitor.sh --verbose`.
7. Record the actual outage and replay times.

An active archive without a relay subscription does not record crossings.
Treat this state as a failed restart.

## Participant statement

Tell participants that planned restarts can occur during the service period.
Usually, the sidecar reconnects without participant action.

State that an archive restart can make the public page unavailable.
State any permanent record gap after an unplanned archive outage.

Do not include private hostnames, resource identifiers, or operational commands in the participant notice.
