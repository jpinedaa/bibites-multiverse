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

The largest retained term is the duplicate-suppression set.
It holds one entry for each ledger record that carries a `migrationId`.
It is never evicted, because a re-forwarded migration must stay refusable.
It measured approximately 88 percent of archive resident memory.

That set now holds a 128-bit fingerprint of each key instead of the key.
This table replays one copy of the production ledger of 2026-08-16.
The copy held 5,408,123 records and 5,134,867 duplicate keys.
Both replays used a 720 hour genome horizon and an empty genome store.

| Archive build | Settled resident | For each record |
|---|---:|---:|
| before the change | 1,177 MB | 0.22 KB |
| after the change | 546 MB | 0.10 KB |

An empty genome store makes both results conservative.
Those replays rebuilt 273,252 genome gaps, and a live archive reports one or two.
The same pair with no genome gap measured 958 MB and 290 MB.

The `0.30 KB` constant above describes the deployed build.
`deploy/monitor.sh` uses the same constant.
Change both when the fingerprinted build ships.

## World process memory

**This is the binding constraint on a host that runs worlds, and it is not a
function of `S`.**

Every other term in this document scales with simulated time. This one does not.
A world process grows at a measured rate with **wall-clock** age:

```text
world_GiB = 0.6 + 0.72 * hours_since_start
```

Use `0.72 GiB` for each world for each real hour. The growth is independent of
the configured time scale, of the achieved rate, and of population. A world held
at `x1` and simulating almost nothing grew at `0.75 GiB` each hour, inside the
band set by worlds running between `x5` and `x19` on the same host at the same
time. Population predicts nothing: the largest population on the reference host
held the smallest resident set.

Time to exhaustion for a host running `N` worlds:

```text
hours_to_exhaustion = (usable_GiB - 0.6 * N) / (0.72 * N)
```

The reference host, six worlds on `30.65 GiB` usable, predicted `6.3` hours and
observed `6.95` and `7.0` hours in two independent cycles.

Read the consequence directly from the formula. A continuous-operation target of
`H` hours needs `0.72 * N * H` GiB of RAM before any other term. Six worlds for
72 hours is approximately **315 GiB**, which no ordinary instance offers. **A
host that runs worlds therefore needs a restart policy, not more memory.**

This growth is a defect and not a property of the workload. Treat every number
here as a description of the current build, recheck it after any game or mod
change, and delete this section when the growth stops.

### Why the failure looks like a CPU problem

It does not present as memory exhaustion. It presents as peers disconnecting.

As available memory falls the kernel enters direct reclaim, and allocating
threads stall inside it. A sidecar that cannot run for fifteen seconds misses
three consecutive sends and the relay drops it on its liveness deadline. On the
reference host, 47 of 48 recorded drops occurred with memory between 85 and 99
percent, while CPU sat at 96 to 99 percent for the entire period **including a
7.1-hour window that contained no drops at all**.

Use pressure stall information to tell the two apart, because utilisation cannot:

```text
/proc/pressure/cpu     full total    time no task could run for want of CPU
/proc/pressure/memory  full total    time no task could run for want of memory
```

On the reference host the CPU figure was **zero** and the memory figure was
several hundred seconds. A host can be pinned at 99 percent CPU indefinitely and
serve perfectly well; the same host stalls hard at 96 percent memory.

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

**Count both directions.** A transfer allowance is usually consumed by inbound
and outbound together, while only outbound overage is charged. A model that
tracks egress alone understates the draw by roughly 40 percent.

Peer traffic is the largest term and it scales with `S`:

```text
peer_GB_per_month = 27.5 * S          outbound
allowance_GB_per_month = 50 * S       both directions
```

Measured, not modelled: `52 GB` each day outbound to six peers whose combined
achieved time scale was `57.5`. A participant at `x1` draws about `50 GB` each
month; an accelerated world at `x10` draws about `500 GB`.

The earlier model in this document gave `54 GB` each month at `S = 5`, which is
`10.9 GB` for each unit of `S`. **The measured figure is 2.5 times that.** Use
the measured rule.

The live console is a much smaller term than this document once claimed, but the
per-tab figure is larger:

```text
console_GB_per_month_per_open_tab = 12.3
```

The earlier estimate of `4 GB` gzipped counted only the status endpoint. The hop
feed polls faster and carries more, and is roughly twice as expensive. Measure
every endpoint the page polls, not the one it is named after. The page can add
endpoints or change cadence without changing this document.

Video transfer is a separate workload, and the wire cost is not the media rate:

```text
viewer_GB_per_month = 1190        measured, low-latency HLS at 2.5 Mbit/s
```

The media itself is `2.51 Mbit/s` as designed, which is the `810 GB` this
document used to quote. The delivered cost is `3.67 Mbit/s`, because low-latency
HLS re-fetches its playlist more often than it delivers a media part — a
measured `8,745` playlist requests against `8,441` parts. Raising the part
duration, or serving a non-low-latency variant, removes that 32 percent.

Use a CDN or managed video service before direct-origin transfer exceeds its
approved budget. At the measured rate, ten continuous viewers is not a capacity
question but a four-figure monthly bill.

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
- **A host that runs worlds has a restart policy, or enough memory for the
  announced continuous-operation target — and for six worlds and 72 hours no
  ordinary instance has that much.** Size the restart interval from the world
  memory formula, never from a guess about CPU.
- The transfer allowance covers map, console, and optional video traffic **in
  both directions**, at the measured rates rather than the modelled ones.
- The provider supports checked off-host backups.
- The operator can increase capacity without changing the participant service name.

Do not store a current vendor recommendation in this file.
Vendor prices, quotas, and instance availability are mutable operations data.
