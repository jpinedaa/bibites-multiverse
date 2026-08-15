# Hosting engineering evidence

This document records stable measurements and sizing rules for the public deployment tools.
It does not describe a live deployment.

Use current provider documentation when you select a service or estimate a bill.
Keep live inventory, quotes, account data, and operator decisions outside this repository.

## Evidence types

The tables use these evidence types:

- A **measurement** comes from the project rig or an isolated replay experiment.
- A **model** scales a measurement with the stated formula.
- A **design rule** follows from a public contract or implementation constraint.

Rates depend on workload and software versions.
Measure a new deployment before you buy fixed capacity.

## Reference workload

The reference rig ran six worlds.
Its summed achieved time scale was 67.9 simulated seconds per wall second.

Define **S** as the sum of achieved time scales across all worlds.
For example, five worlds that each achieve x1 give `S = 5`.

The reference workload produced these measurements:

| Quantity | Measurement |
|---|---:|
| Crossing rate | 1,065 per minute |
| Ledger size | 7,426,686 records in 2,504,257,342 bytes |
| Ledger bytes per record | 337 bytes |
| Ledger records per crossing | 2.14 |
| Ledger bytes per crossing | 703 bytes |
| Forwarded wire bytes per crossing | approximately 15.8 KB |
| Genome objects | approximately 504,000 |
| Mean genome object size | approximately 14.2 KB |
| Metrics growth | 1.6 MB per day for each slot |
| Relay resident memory | 93 MB |
| Relay CPU use | 0.045 cores |
| Archive settled resident memory | 2.26 GB |
| Archive CPU use | 0.18 cores |
| `/api/status` response | 19,705 bytes, or 2,254 bytes with gzip |
| `/api/hops` response | 49,307 bytes, or 6,562 bytes with gzip |
| `/api/species` response | 7,323 bytes, or 1,451 bytes with gzip |

The relay and archive binaries were static Linux executables without cgo.
Their measured sizes were approximately 10.5 MB and 10.6 MB.

## Workload scaling

Most durable data grows with simulated time.
Metrics grow with wall time and slot count.

Use these reference rates for one unit of S:

| Term | Daily rate |
|---|---:|
| Crossings | 22,600 |
| Ledger records | 48,350 |
| Ledger data | 15.9 MB |
| Relay forward egress | 357 MB |
| Genome demand | approximately 40 MB |

Add 1.6 MB of metrics for each slot each day.

Use this formula for durable growth:

```text
daily durable growth = (56 MB * S) + (1.6 MB * slot count)
```

The genome term uses demand, not observed supply.
A slow genome collector can hide work in its queue.

### Ninety-day examples

These examples use one world for each unit of S.
They keep all ledger records and genome objects.

| Workload | S | Durable data after 90 days | Ledger records after 90 days |
|---|---:|---:|---:|
| Five worlds at x1 | 5 | 26 GB | 21.8 million |
| Eight worlds at x1 | 8 | 41 GB | 34.8 million |
| Twelve worlds at x1 | 12 | 62 GB | 52.2 million |
| Twelve worlds at x5 | 60 | 303 GB | 261 million |

Add the operating system, logs, backups, and growth margin to these values.
Monitor free space as a service signal.

## Archive replay memory

The archive retains an index of ledger history in memory.
An old replay implementation also materialized the complete ledger before it applied records.

The materialized implementation produced these results:

| Replay setting | Peak memory for each record |
|---|---:|
| No Go memory limit | 1.03 to 1.29 KB |
| `GOMEMLIMIT=5GiB` | 721 to 772 bytes |
| Aggressive Go memory limits | as low as 600 bytes, with high CPU use |

The settled archive used approximately 0.30 KB for each ledger record.
The materialized replay used more memory than the retained state.

A streaming replay changed the peak to 184 bytes for each record.
The package benchmark measured 41 bytes for streaming and 1,007 bytes for materialization.

Use the streaming implementation for archive startup.
Treat `GOMEMLIMIT` as a guard, not as a replacement for streaming.
Size memory from the current ledger line count and a measured replay.

At `S = 5`, the ledger grows by approximately 242,000 records each day.
After 90 days, 21.8 million records use approximately 6.5 GB at 0.30 KB per record.

Retention changes this term.
The public Contract B rules define which records an implementation can remove.

## Status-page transfer

The status page polls `/api/status` every two seconds.
It polls `/api/hops` every 1.5 seconds.

At `S = 5` with six slots, one continuously open tab transfers these approximate monthly totals:

| Encoding | Monthly transfer |
|---|---:|
| Identity | 32 GB |
| Gzip | 4 GB |

Keep HTTP compression enabled.
Treat poll cadence as a user-interface decision.

## Core-service egress

This table models `S = 5`, six slots, and a 30-day month.

| Flow | Monthly transfer | Basis |
|---|---:|---|
| Migration forwards | 54 GB | 357 MB each day for each unit of S |
| Status page | 32 GB identity, or approximately 4 GB with gzip | one continuously open tab |
| Genome fetches | up to 92 GB | configured request ceiling and 14.2 KB objects |
| `FORWARD_RECEIPT` | 0.97 GB | 287 bytes for each crossing |
| Small control messages | negligible | measured against the other flows |
| Realistic planning range | 90 to 150 GB | allow a tail near 250 GB |

The `FORWARD_RECEIPT` harness sends 2,000 migrations through an in-process relay.
It uses a 15.8 KB migration body.

The harness measured these changes:

- Each receipt used 287 bytes.
- Relay-written frames increased from two to three for each migration.
- Relay-read frames stayed at two for each migration.
- Forward-path CPU increased by 1.3 percent.
- The sender appended approximately 221 bytes and performed one `fsync` for each receipt.

The receipt transfer is 1.8 percent of the migration-forward term.
The public test is `go/internal/relay/receipt_cost_test.go`.

## Core-service placement

Run the relay and archive on the same host when practical.
The archive subscribes to each migration payload.
Loopback placement removes that inter-service network flow.

Steady CPU use was small at the reference rate.
Archive history, replay behavior, and storage growth set the larger capacity limits.

Use a stable operator-controlled DNS name for the public endpoint.
The join string contains the relay URL, so a temporary provider name creates migration work.

Do not store the selected domain or registrar account in this document.

## Native Linux world evidence

The native Linux build was tested with game version 0.6.3.1.
The test used BepInEx 5.4.23.3 and the project mod.

The test confirmed these properties:

- The mod patch targets were present in the Linux managed assembly.
- BepInEx loaded the mod in headless mode.
- The Contract A token-file flow worked without path translation.
- The game created its save tree under the Linux persistent-data path.
- Save rotation and save-on-quit worked.
- One game instance used one log directory without shared-log corruption.

The short test produced these resource measurements:

| Quantity | Measurement |
|---|---:|
| Headless game resident memory | 368 to 401 MB |
| Headless game CPU use | 0.86 to 1.17 cores |
| Sidecar resident memory | 13 to 14 MB |
| Save pause | 277 to 682 ms |
| Clean quit with save-on-quit | 2.05 seconds |

The test was too short to measure a mature world.
Use 2 vCPU and 4 GB as the old conservative minimum.
Use 8 GB when a long-running world has not produced a better measurement.

Run one game instance for each log directory.
Multiple Linux processes that share one BepInEx log file can corrupt the log.

## Interruptible world state

Store world identity and custody data on persistent storage.
Keep these items across every instance replacement:

| State | Purpose |
|---|---|
| Sidecar journal | Preserves durable custody and in-flight organisms |
| Peer ID and remembered slot | Lets the peer reclaim its address |
| Contract A token and peer credential | Authenticates the returning peer |
| Game saves | Preserves the world |
| Genome cache | Avoids unnecessary fetches after restart |

Stop the game cleanly after an interruption notice when time permits.
Then stop the sidecar and attach the persistent storage to the replacement instance.

A reloaded save can omit organisms that arrived after the last save.
Use a save interval that limits this exposure without causing excessive pauses.

## Selection rules

Apply these rules when you evaluate a hosting option:

1. Measure the current ledger and replay before you select memory.
2. Calculate disk growth from S, slot count, retention, and the planned period.
3. Include status-page traffic and genome catch-up in the egress budget.
4. Keep coupled services in one network zone.
5. Put interruptible world state on persistent storage.
6. Test a clean shutdown and replacement before you rely on interruption notices.
7. Obtain current prices, quotas, and service terms from the provider.

Keep these mutable records in the operator system:

- current prices and quotes
- provider and region selections
- live account, stack, instance, volume, and snapshot identifiers
- quota requests and support cases
- domains, registrar accounts, and DNS records
- secret names and live secret layouts
- deployment receipts and incident timelines

This separation keeps the public measurements reusable without exposing a live environment.
