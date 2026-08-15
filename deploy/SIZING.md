# Hosting capacity model

This document contains stable engineering measurements and sizing formulas.
It does not contain a current cloud quote or a live deployment forecast.

Check vendor prices and service limits when you make a purchase decision.
Keep the selected resources, current forecast, and budget in private operations storage.

## Workload variable

Define `S` as the sum of achieved time scales across all live peers.

Peer count alone does not predict storage or traffic.
Most durable events follow simulated time instead of wall time.

For example, five peers at achieved time scale `1` give `S = 5`.
One peer at achieved time scale `20` contributes the same simulated-time load as twenty peers at `1`.

Read each peer value from `achievedTimeScale` in `/api/status`.

## Reference measurements

The reference workload used a six-slot map with `S = 67.9`.
The table shows the measured quantities that support the model.

| Quantity | Reference value |
|---|---:|
| Crossing rate | 1,065 each minute |
| Ledger records per crossing | 2.14 |
| Ledger bytes per crossing | 703 B |
| Forwarded wire bytes per crossing | approximately 15.8 KB |
| Genome object size | approximately 14.2 KB |
| Metrics growth | 1.6 MB each day per slot |
| Relay resident memory | 93 MB |
| Archive resident memory | approximately 0.30 KB per ledger record |
| Streaming replay peak | approximately 0.18 KB per ledger record |
| Reference replay rate | 37,000 to 49,000 records each second |

These measurements are capacity inputs, not guarantees.
Measure the target deployment and update its private forecast.

## Durable growth

Use this conservative rule:

```text
durable_MB_per_day = 56 * S + 1.6 * slot_count
```

The simulated-time term contains the ledger and expected genome demand.
The slot term contains wall-clock metrics samples.

The simulated-time term has this approximate shape:

| Term | Daily amount for each unit of `S` |
|---|---:|
| Crossings | 22,600 |
| Ledger records | 48,350 |
| Ledger | 15.9 MB |
| Genome demand | 40 MB |
| Relay forward traffic | 357 MB |

Use genome demand instead of observed genome-store growth.
An archive can build a genome-fetch backlog.
Observed growth can then report supply instead of demand.

Add operating-system, package, and log space to the result.
The default log ceiling for relay and archive is approximately 1.2 GB.

## Example disk projections

These examples use the conservative rule without a genome horizon.

| Workload | `S` | Daily durable growth | 90-day growth |
|---|---:|---:|---:|
| Five peers at `1` | 5 | 0.29 GB | 26 GB |
| Eight peers at `1` | 8 | 0.46 GB | 41 GB |
| Twelve peers at `1` | 12 | 0.69 GB | 62 GB |
| Twelve peers at `5` | 60 | 3.37 GB | 303 GB |

Add at least the log ceiling, operating-system space, and recovery headroom.
Do not provision a volume to the exact model result.

A genome horizon bounds stored blobs after the horizon reaches steady state.
It does not bound the ledger or archive memory.

## Archive memory

Use both memory terms:

```text
resident_KB = ledger_records * 0.30
replay_peak_KB = ledger_records * 0.18
```

The resident term is larger for the streaming archive.
Compare the larger result with usable RAM and swap.

The replay implementation once materialized the complete ledger.
That design used approximately 1.03 to 1.29 KB for each record at peak.

The streaming implementation applies and releases one record at a time.
It measured approximately 0.18 KB for each record at peak.
The file format and wire contracts did not change.

`GOMEMLIMIT` can limit a regression, but it does not reduce retained state.
A tight limit also increases CPU use during replay.

Do not use swap as the normal capacity for a live replay heap.
Sustained swap activity means that the instance needs more memory or less retained state.

## Replay time

Use this formula:

```text
replay_seconds = ledger_records / measured_records_per_second
```

The reference host measured 37,000 to 49,000 records each second.
Use the lower rate until the target host has its own result.

Replay time grows with every retained ledger record.
Recalculate it before each planned archive restart.

An archive does not subscribe during replay.
If the relay stays live, every crossing during replay creates a gap in the archive record.

## Network transfer

At `S = 5`, the reference model gives approximately 54 GB each month for migration forwards.
Genome fetch traffic depends on cache gaps and participant behavior.

The live console can exceed game traffic when a browser polls continuously.
HTTP gzip support reduces the reference console estimate from approximately 32 GB to 4 GB each month per open tab.

Measure all current endpoints before transfer becomes a billing constraint.
The page can add endpoints or change cadence without changing this document.

Video transfer is a separate workload.
A 2.5-Mbit/s stream uses approximately 810 GB for one continuously open viewer-month.
Use a CDN or managed video service before direct-origin transfer exceeds its approved budget.

## Sizing procedure

Run this procedure during provisioning and after a capacity alert:

1. Sum `achievedTimeScale` to get `S`.
2. Read the current slot count.
3. Calculate daily durable growth.
4. Read actual free space and recent daily growth.
5. Count ledger records.
6. Calculate resident memory and replay peak.
7. Measure or select a conservative replay rate.
8. Calculate the remaining disk and memory headroom.
9. Compare current transfer with the provider allowance and budget.
10. Record the result in private operations storage.

Actual recent growth outranks the model when the measurement window is representative.
The model remains useful for new peers and higher achieved time scales.

## Selection rules

Select a host that meets these conditions:

- The volume holds the announced period plus recovery headroom.
- The archive resident estimate stays below the approved memory threshold.
- The archive can replay inside the announced maintenance window.
- The transfer allowance covers map, console, and optional video traffic.
- The provider supports checked off-host backups.
- The operator can increase capacity without changing the participant service name.

Do not store a current vendor recommendation in this file.
Vendor prices, quotas, and instance availability are mutable operations data.
