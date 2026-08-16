# Restart policy

The relay and archive are separate services.
A relay restart is usually short.
An archive restart replays the ledger, and its cost grows with that ledger.
The host that runs worlds has one class of its own, and that class is automatic.

This policy describes reusable behavior.
Private operations storage owns each restart window, approval, receipt, and incident record.

## Restart classes

| Class | Effect | Notice |
|---|---|---|
| Certificate reload | nginx loads a renewed pair. The relay and archive continue. | No notice is normally necessary. |
| World recycle | One world on the world host stops and starts again. The relay, the archive, and every other world continue. | No notice is normally necessary. It is automatic and recurring. |
| Relay only | Peers disconnect, reconnect, and reclaim their slots. | Announce a planned restart. |
| Archive only | The website stops while the archive replays the ledger. A restart that keeps the record complete also stops the relay. | Announce it and state the expected replay time. |
| Host reboot | The relay and archive stop. The archive replays after boot. | Announce it and schedule it. |
| Unplanned restart | The effect matches the failed service and includes diagnosis time. | Explain it after service returns. |

## Certificate reload

`tls-deploy-hook.sh` installs the certificate and key through atomic replacements.
It reloads nginx after both files are present.

Existing WebSocket connections stay on the old nginx workers.
New connections use the renewed certificate.

The monitor compares the served certificate with the installed certificate.
This check detects a deploy-hook failure before expiry.

## World recycle

This class belongs to the host that runs worlds, and not to the relay and archive host.

A world process grows with wall-clock time, at a rate that its time scale, its achieved rate, and
its population do not predict.
[`SIZING.md`](SIZING.md) carries that rate and the exhaustion model it implies.
A host that runs worlds therefore needs a restart policy, and `bibites-recycle.timer` is that
policy.

The trigger is memory and not a clock.
The timer asks every ten minutes.
`bibites-recycle-world` does nothing while available host memory stays above its threshold.
Below that threshold it restarts one world, the largest that has been running for at least an hour.

Three guards limit it.
It does not act while any world is already dark, so it cannot deepen an outage.
It waits out a cool-off after each recycle.
It refuses outright when it cannot confirm that the chosen world saves on quit.
Those guards are the whole pre-restart check for this class.
No person approves each recycle, because the alternative to acting is a kernel that acts instead.

The recycle costs no simulated time.
The game receives `SIGTERM`, saves on quit, and resumes from that save.
The world restarts in either case: without the recycle, the kernel out-of-memory killer performs
the same restart with `SIGKILL`, later, and drags every other world through a swap storm first.
This class is the planned form of that unplanned restart.

Only the game unit restarts.
The sidecar keeps its relay connection, its slot, and its coordinate.
The map shows that slot as connected with no game attached, and the relay routes organisms around
it until the game returns.

The recycle touches neither the archive nor the relay.
There is no ledger replay and no gap in the permanent record.
It is the opposite case to [Restart with a complete record](#restart-with-a-complete-record), which
stops the relay for the length of a replay to keep that record whole.

The time scale returns with the world.
`bibites-game@<world>.service` wants `bibites-timescale@<world>.service`, so every start of a world
re-runs the unit that applies its target scale.
A world that comes back at the game's built-in `x1` and stays there is a fault in that dependency.

Each recycle leaves its own record, and [`cloud/aws/README.md`](../cloud/aws/README.md) says where
to read it.

Stop the whole behavior with one command:

```sh
sudo systemctl disable --now bibites-recycle.timer
```

Nothing else changes when it is disabled.
The growth continues, and the out-of-memory killer performs the restarts instead.

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

### Restart with a complete record

Run this sequence when the record must stay complete:

```sh
sudo systemctl stop multiverse-relay
sudo systemctl restart multiverse-archive

# Wait for the replay. The archive binds its HTTP port only after the replay
# finishes, so this refuses the connection until the archive is ready.
until curl -fs --max-time 5 http://127.0.0.1:8796/healthz >/dev/null 2>&1; do sleep 5; done

sudo systemctl start multiverse-relay

# Confirm the subscription. Do not call the restart complete before this passes.
until curl -fs --max-time 5 http://127.0.0.1:8796/api/status 2>/dev/null \
  | grep -q '"relayConnected": true'; do sleep 2; done
```

Use the `MV_ARCHIVE_HTTP` address from the parameter file if it is not the default.
The archive dials the relay on a backoff between 1 and 30 seconds.
The last loop normally passes within a few seconds.

The archive and every sidecar reconnect on that same backoff.
The archive dials only after the replay, so its backoff is still short.
Each sidecar failed for the whole outage, so its backoff is near 30 seconds.
The archive normally resubscribes before the relay places the first peer.
That is a race and not a guarantee, so start the relay promptly after `/healthz` answers.

Prove the result from the relay log:

```sh
sudo grep -E 'client connected.*role=archive|placement claim' /var/log/multiverse/relay.log | tail -20
```

Run this immediately after the sequence, so the last lines belong to this restart.
If the archive line comes before the first placement claim, the record is complete.
If a placement claim comes first, the record has a gap.
The gap runs from that claim to the archive line.
Write that gap into the deployment record.

**CAUTION.** This sequence needs the current archive unit file.
An older unit named the relay in `Wants=` as well as in `After=`.
`Wants=` is a start-time pull, so `restart multiverse-archive` started the stopped relay again.
The map then ran live for the whole replay and nothing recorded it.
Install the current units first:

```sh
sudo /opt/multiverse/deploy/provision.sh --only systemd
```

That phase reloads the systemd configuration and costs no outage.
Run it before you stop the relay, because it starts stopped units.
Then check that the archive unit does not name the relay in `Wants=`:

```sh
systemctl show -p Wants multiverse-archive.service
```

**CAUTION.** A migration count that only rises does not prove a complete record.
The counter reports what the archive recorded.
It cannot report a crossing that reached no subscriber.

The sequence costs a full relay outage for the length of the replay.
Estimate that length with the formula above.
The hosted run measured approximately 60 seconds at approximately 3.8 million records.
Each sidecar reconnects on its backoff schedule after the relay returns.
The sidecar journal holds unsent custody records for the whole outage.
That participant outage is the price of a complete record.
The monitor reports the stopped relay as critical for the whole window.

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

A reboot starts both units from `multi-user.target`.
The relay is live while the archive replays, so the record has a gap.
Mask the relay before the reboot when ledger continuity is required:

```sh
sudo systemctl mask multiverse-relay
sudo reboot
```

After boot, wait for the archive and then return the relay:

```sh
until curl -fs --max-time 5 http://127.0.0.1:8796/healthz >/dev/null 2>&1; do sleep 5; done
sudo systemctl unmask multiverse-relay
sudo systemctl start multiverse-relay
until curl -fs --max-time 5 http://127.0.0.1:8796/api/status 2>/dev/null \
  | grep -q '"relayConnected": true'; do sleep 2; done
```

A mask holds across every later boot.
Unmask the relay in the same session, or the map stays down.
The monitor reports the masked relay as critical until you start it.
The participant cost is the same as the archive-restart sequence above.

After boot, check service enablement and archive subscription.
Do not infer archive health from the process state alone.

## Unplanned restart

systemd restarts failed services within configured rate limits.
It stops retrying after repeated failures.

This limit prevents an archive replay failure from consuming the host indefinitely.
The monitor must alert a person when a unit enters the failed state.

The host protects relay availability under memory pressure.
An archive failure loses record coverage, but a relay failure stops the map.

A world that the kernel kills for memory on the world host is an unplanned restart of this class.
[World recycle](#world-recycle) exists to convert it into a planned one before the kernel acts.
An unplanned world restart also loses its applied time scale unless the unit dependency named in
that section is installed.

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
7. Send the required notice on the announcements page. [`ANNOUNCEMENT.md`](ANNOUNCEMENT.md) names
   that channel and states what the notice must contain.

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

State that a world the operator runs can restart itself for memory.
Its slot then shows no game attached for the length of that restart, and its neighbours continue.
No participant world restarts for this reason, and no record is lost.

Do not include private hostnames, resource identifiers, or operational commands in the participant notice.
