# Hosting capacity model

This document contains stable engineering measurements and sizing formulas.
It does not contain a current cloud quote or a live deployment forecast.

Check vendor prices and service limits when you make a purchase decision.
Keep the selected resources, current forecast, and budget in private operations storage.

## Workload variable

Define `S` as the sum of achieved time scales across all live peers.

Peer count alone does not predict durable growth.
Most durable events follow simulated time instead of wall time.

`S` predicts durable growth. It does not predict network transfer.
Use the crossing rate and the slot count for transfer; see "Network transfer".

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
| Archive retained state | approximately 28 to 31 B per ledger record |
| Archive resident set | approximately 64 B per ledger record on two cores |
| Reference replay rate | 37,000 to 49,000 records each second |

These measurements are capacity inputs, not guarantees.
Measure the target deployment and update its private forecast.

The two archive rows describe the build of `2026-08-16` and change when that build changes.
The archive resident set is also the replay peak; they are one number.
"Archive memory" owns both rows and states the workload they were measured on.

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

Use genome demand instead of observed genome-store growth.
An archive can build a genome-fetch backlog.
Observed growth can then report supply instead of demand.

This table once carried a relay forward-traffic term of `357 MB` for each unit of `S`.
Network transfer no longer follows `S`. The measured model is in "Network transfer".

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

Archive memory is one term, not two.
The replay peak and the settled resident set are the same number.
A replay of a real ledger measured them `0.06 percent` apart in every run with no binding
`GOMEMLIMIT`.
The resident set rises through the replay and is then flat.
Size against that one number, and never add a peak term to a resident term.

Use this model:

```text
resident_B = ledger_records * bytes_per_record
replay_peak_B = resident_B
```

`bytes_per_record` is a property of the build and of the host's core count.
It is not a constant of this project.
Measure it, and record the build that produced the measurement.
`monitor.sh` carries it as `MV_REPLAY_PEAK_B` and `MV_REPLAY_RESIDENT_B`,
so a measurement on a real ledger corrects the check without a new release.

Compare the result with physical RAM, and do not add swap to that comparison.
The swap rule below is the reason.
`monitor.sh` divides by `MemTotal` alone for this reason, and reports swap in a separate check.

### What the archive retains

Retained state is what the archive still holds after the collector runs.
Resident set is what the kernel counts against host memory, and what the kernel kills for.
The two are different numbers and they answer different questions.

The largest retained term is the duplicate-suppression set.
It holds one entry for each ledger record that carries a `migrationId`.
It is never evicted, because a re-forwarded migration must stay refusable.
It held approximately 88 percent of archive resident memory before the change below.
That set now holds a 128-bit fingerprint of each key instead of the key,
which lowered its cost from `89 B` to `26 B` for each key.

These measurements replay one copy of the production ledger of `2026-08-16T19:35:41Z`.
The copy holds `5,408,123` records, `5,134,867` duplicate keys and `1,836,382,633` bytes.
Both builds replayed that same file on two pinned cores, which is the service host's core count.

| Shape, two pinned cores | Before the change | After it | Factor |
|---|---:|---:|---:|
| Retained state for each record | `89 B` | `28` to `31 B` | `3.0` |
| Resident set for each record, no genome gap | `213 B` | `64 B` [1] | `3.3` |
| Resident set for each record, every genome missing | `280 B` | `110 B` | `2.5` |
| Replay time for the whole file | `70.1 s` | `58.2 s` | `1.20` |

[1] Derived. The same pair measured `186 B` and `56 B` on sixteen cores,
and two cores cost `14.8 percent` more resident set.

The `89 B` retained figure is converged.
It is the live heap under a `GOMEMLIMIT` set below that heap, where the collector
leaves no floating garbage.
Runs at two cores and at sixteen cores agree on it to one megabyte.

The two resident rows are the same file at two genome horizons.
"No genome gap" is the live service, which reports one or two gaps.
"Every genome missing" is a restore from the ledger alone, with no genome store.
That restore costs `273,252` pending gaps on this ledger, and it is the shape to size against.
A backup that carries the ledger and not the genome store therefore needs more memory to
restore than the running service needs to serve.

The replay implementation once materialized the complete ledger.
That design used approximately `1.03` to `1.29 KB` for each record at peak.
The streaming implementation applies and releases one record at a time.
The file format and wire contracts did not change.

### Collector headroom, and what `GOMEMLIMIT` does

The resident set is larger than the retained state, and the difference is collector headroom.
With no binding limit the resident set measured `2.05` times the live heap on two cores.
`GOGC=100` asks for that headroom by design.
Two vCPUs make it worse: the same workload on sixteen cores held `15` to `22 percent` less.
With two Ps the collector cannot keep pace with the replay's allocation rate,
so the heap runs past its goal before the cycle ends.

`GOMEMLIMIT` reclaims that headroom. It does not reduce retained state.

- It lowers the resident set, which is the number the kernel kills for.
- It cannot fail a replay. It is a soft limit, and the runtime spends CPU instead of failing.
  A limit set below the live heap ran the collector continuously, replayed every record,
  and left the ledger byte-identical.
- Below the live heap it stops buying resident set and buys only wall time.
  The measured floor is `1.27` times the live heap on two cores and `1.13` times on sixteen.
- It does not move `monitor.sh`'s replay-headroom ratio, which is a model of record count.

This is the measured band, on two cores against the real ledger:

| `GOMEMLIMIT` | Resident set | Replay time | Collector share of CPU |
|---|---:|---:|---:|
| none | `1,100 MB` | `65.1 s` | `6 percent` |
| `1200MiB` | `1,074 MB` | `65.0 s` | not separated |
| `800MiB` | `775 MB` | `66.6 s` | `8 percent` |
| `600MiB` | `602 MB` | `73.8 s` | `14 percent` |
| `450MiB`, below the live heap | `579 MB` | `94.2 s` | `14 percent` |

An earlier version of this document said a tight limit increases CPU use during replay.
That is true in CPU-seconds. It is false in wall time on a host with spare cores.

Select the limit from the host's archive ceiling, not from its total RAM:

```text
archive_ceiling = MemTotal - the memory no other process on the host will return
MV_ARCHIVE_GOMEMLIMIT <= archive_ceiling / 1.14
```

`1.14` is the measured excursion of peak over settled under a binding limit.
The `2 GB` service host measured an archive ceiling of `1,534 MiB`, so the selected value is
`1100MiB`.
That leaves `434 MiB` for the excursion, for the collector floor, and for a restore-shaped replay.
Once the collector floor passes the limit, the exact value of the limit stops mattering:
`800MiB` and `1200MiB` reach the same ceiling on the same day.
The value decides how much resident headroom and CPU the host has until then.

### Swap

Do not use swap as the normal capacity for a live replay heap.
A small swap file is a recorded crash barrier and not replay capacity.
Set `vm.swappiness=10` with it, so steady-state processes stay resident and a peak can still spill.
`monitor.sh` reports swap in a separate check and never counts it as headroom.
Sustained swap activity means that the instance needs more memory or less retained state.

### A worked horizon, and what it does not bound

This example is dated and it is not a purchase recommendation.
Vendor prices and the live forecast stay in private operations storage.
The arithmetic is the reusable part, not the answer.

Inputs, all measured on `2026-08-16`:

- `5,545,189` ledger records at `20:25Z`.
- `3.00` million records each day, over a `41 h` ledger. The observed band is `2.90` to `3.14`.
- `338 B` for each record on disk, which is about `1 GB` each day.
- An archive ceiling of `1,534 MiB` on the `2 GB` host.
  That is `MemTotal` less the `373 MiB` that no other process on the box will return.
- `28` to `31 B` of retained state for each record, with the fingerprint change.
- A collector floor of `1.27` times the live heap under a binding limit.

The memory horizon:

```text
live_ceiling   = 1,534 MiB / 1.27                     = 1,208 MiB = 1,266,500,000 B
record_ceiling = 1,266,500,000 B / 31 B               = 40,900,000 records
               = 1,266,500,000 B / 28 B               = 45,200,000 records
days           = (40,900,000 - 5,545,189) / 3,000,000 = 11.8
               = (45,200,000 - 5,545,189) / 3,000,000 = 13.2
```

That is **`12` to `13` days from `2026-08-16`**, or `11` to `14` days across the growth band.
It is the day the collector starts to bind hard, not the day the service fails.

Compare the three states of the same host:

| State | Ceiling reached at | Days from `2026-08-16` |
|---|---:|---:|
| Before the change, limit inert at `5GiB` | `7.5` million records | `0.7` |
| Before the change, binding limit | `14.3` million records | `2.9` |
| After the change, limit inert | `25` million records | `6.5` |
| After the change, binding limit at `1100MiB` | `41` to `45` million records | `12` to `13` |

Disk is not the binding term on this host.
`46 GB` were free on `2026-08-16`, which is about `45` days at `1 GB` each day.

Larger sizes at the same growth rate and the same measured constants:

| Public bundle | RAM | Disk | Memory horizon | Disk horizon | List price each month |
|---|---:|---:|---:|---:|---:|
| `small_3_0`, the current size | 2 GB | 60 GB | `12` to `13` days | ~`45` days | `$12` |
| `medium_3_0` | 4 GB | 80 GB | `29` to `32` days | ~`65` days | `$24` |
| `large_3_0` | 8 GB | 160 GB | `61` to `68` days | ~`144` days | `$44` |

Read that table for what it is: **every cell is a date and none of them is a bound.**
Retained state grows with each ledger record, and nothing in the list stops it.
Two things bound it, and only two:

- a lower crossing rate, which means a lower `S`;
- an on-disk index, so that resident memory follows the working set instead of the record count.

A larger instance and a tighter limit buy weeks.
Revisit the size decision inside that window, and record the date on which it was revisited.

### The gate's constants, and which value lives where

`monitor.sh` ships `MV_REPLAY_PEAK_B=184` and `MV_REPLAY_RESIDENT_B=300`.
Those are the reference-workload figures and they are conservative for the current build.
Keep them as the shipped defaults.
A conservative gate calls a host full before it is, and an optimistic one calls it free
after it is not, so the shipped default takes the safe error.

`deploy.env.example` carries the measured pair for the current build, `110` and `110`,
which a host adopts through its own `deploy.env`.
`110 B` for each record is the measured resident cost of a restore-shaped replay on two cores.
The live service shape costs about `64 B`, so the selected value carries `1.7` times of margin
and still describes a replay this host can really be asked to run.
Both constants take the same value because the peak and the settled resident set are one number;
`monitor.sh` keeps its `max()` against a future model change.

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

Measure the rate on the target host. Do not carry a rate between hosts.

| Host | Measured rate |
|---|---:|
| Reference host | `37,000` to `49,000` records each second |
| Two-vCPU service host, seven starts | `61,000` to `73,000` records each second |
| Two-core development box, same file | `83,000` records each second |

The service host is `1.37` times slower than a development box with the same core count.
The difference is disk: the host streams a `1.8 GB` file with a cold page cache.
Multiply a two-core development measurement by `1.37` to project this host.

The rate falls slowly as the ledger grows.
It measured `72,000` records each second at `4.0` million records and `61,000` at `5.3` million.
Use the most recent measurement, never the best one.

The duplicate-suppression fingerprint change replayed the same file `6` to `26 percent` faster.
Wall time was the least repeatable measurement in that set, so do not plan against one figure.

Replay time grows with every retained ledger record.
Recalculate it before each planned archive restart.

An archive does not subscribe during replay.
If the relay stays live, every crossing during replay creates a gap in the archive record.
The record-preserving sequence holds the relay down for the whole replay,
so the replay time is also the participant outage.
At `61,000` records each second, `10` million records cost `164 s`
and `40` million cost about `11` minutes.

Replay time is not what ends this host's life. Memory is.

## Network transfer

**Count both directions, and count in `2^30` bytes.** A transfer allowance is usually
consumed by inbound and outbound together, while only outbound overage is charged. A
model that tracks egress alone understates the draw by more than half at this workload.
The provider's "GB" is `2^30` bytes: reconciling the host's own counter against the
billing record gives a ratio of `1.01` on `2^30` and `1.08` on `10^9`, so the decimal
base is the wrong one. Every figure in this section is `2^30` bytes over a 31-day month.

**Inbound is free, and inbound is not free.** Only outbound above the allowance is
charged, so inbound never appears on an invoice. Inbound still consumes the allowance, so
every inbound byte pushes an outbound byte into overage. Price an inbound driver at the
same rate as an outbound one.

**The host's own interface counter is the measurement.** `/proc/net/dev` on the billed
interface tracks the provider's own metric to `0.002 percent` outbound and `0.9 percent`
inbound, and needs no cloud credential. It is what the transfer check reads; see
[`../docs/observability.md`](../docs/observability.md), "Layer 4 — Cost".

### Peer traffic

Peer traffic is the largest term. It is set by the **simulation throughput of the host
that runs the worlds**, not by any world's configured time scale:

```text
peer_GB_per_month  = 88 * total_crossings_per_second + 5.9 * slot_count^2
crossings_per_second_of_one_world = 0.010 * population * achieved_time_scale
```

Both terms count both directions. The first term is organism migration: one envelope for
each edge crossing, measured at approximately `17.9 KB` on the wire. The second term is
the periodic map-status broadcast, which every peer and the archive receives, so its
map-wide cost grows with the square of the slot count.

A seven-slot reference map measured `2,508 GB` each month across all peers: `82 percent`
migration payloads, `12 percent` status broadcasts, `3 percent` genome responses, and the
balance in acknowledgements and pings. Duplicate migration identifiers were `0` to `1`
for each stream, so the volume is real work and not a retry storm.

**Peer cost looks constant for each peer, and that is a property of the host.** Six worlds
measured `3.3` to `4.0` crossings each second while their achieved time scales spanned
`x3.5` to `x17.9` and their populations spanned `12` to `131`. The product of population
and achieved time scale is what sets the crossing rate, and that product is pinned by the
shared host's CPU. A world that runs fast holds a small population, and a world that runs
slow holds a large one.

**Read the consequence before you plan a change.** Lowering a configured time scale
lowers transfer only if population does not grow into the freed CPU. On a CPU-bound host
it will not. **Removing a peer removes its crossings and its share of the status
broadcast. Slowing a peer can remove nothing.**

An earlier rule in this document gave peer traffic as `50 GB` each month for each unit of
`S`. At the reference map that result lands close to the measured total by accident. Its
shape is wrong, and the shape is what a capacity decision uses.

The status coefficient is a measurement of the current build and is an upper bound: the
reference map emitted status frames about six times more often than its own design
intends.

**The peer wire is uncompressed JSON.** WebSocket `permessage-deflate` is disabled at
every endpoint and the reverse proxy does not compress the upgrade. Offline `deflate` at
level `6`, with a `32 KiB` window and per-stream context takeover, measured `9.0x` on
migration payloads, `9.8x` on status frames, `10.4x` on genome responses, and `8.5x`
overall. **Negotiated transport compression is the largest available reduction in this
document and costs no simulation throughput and no fidelity.** Budget `7x` to `8.5x`, or
about `4.5x` without context takeover. Each compression state costs approximately
`260 KB` of memory for each connection, so choose a smaller window or no context takeover
on a map of more than a few dozen peers.

### Video

Publisher ingest is a fixed inbound cost. The service pays it when nobody watches:

```text
ingest_GB_per_month = 312 * publish_Mbit_per_second
```

A `2.5 Mbit/s` publisher costs approximately `780 GB` each month, inbound, in every month
that it runs. Against a `3,072 GB` allowance that is a quarter of the budget spent on an
empty room. Lower the publish bitrate, or publish only while an audience exists.

Viewer cost is not the media rate:

```text
viewer_GB_per_month = 1150       low-latency HLS, 2.5 Mbit/s media
viewer_GB_per_month = 790        non-low-latency HLS, same media
```

The media is `2.52 Mbit/s` as designed. Low-latency HLS delivers `3.69 Mbit/s`, because
it re-fetches its playlist approximately as often as it delivers a media part: a measured
`1.035` to `1` playlist-to-part ratio, with `31.8 percent` of delivered bytes in
playlists. A non-low-latency variant removes that overhead and adds several seconds of
latency.

**Low-latency HLS is also the variant a cache cannot help.** Each playlist request stays
open until the next part exists, so an edge cannot serve it from a stored object. Select
the non-low-latency variant before a content delivery network, or the network delivers
much less than its price implies.

Match the segment target to the encoder keyframe interval. A server extends a segment
until the segment contains a keyframe, so a `1 s` target against a `2 s` keyframe
interval produces `2 s` segments and a configuration that describes nothing.

Measure a stalled client separately from a viewer. One stuck low-latency player measured
`8.7` playlist requests for each media part and delivered no media at all, and it cost
more than a real viewer.

### Console

The live console is a much smaller term than this document once claimed, but the
per-tab figure is larger:

```text
console_GB_per_month_per_open_tab = 12.3
```

The earlier estimate of `4 GB` gzipped counted only the status endpoint. The hop
feed polls faster and carries more, and is roughly twice as expensive. Measure
every endpoint the page polls, not the one it is named after. The page can add
endpoints or change cadence without changing this document.

### The consequence

Use a CDN or a managed video service before direct-origin transfer exceeds its approved
budget. At the measured rate, ten continuous viewers is not a capacity question but a
four-figure monthly bill. Select the non-low-latency variant first.

A map that carries seven peers and one publisher spends approximately `3,300 GB` each
month before the first viewer arrives. Size the allowance from the crossing rate, the
slot count, and the publisher, and treat every viewer as an addition to it.

Compress the peer wire before you buy a larger allowance. The peer term is `76 percent`
of that figure and compresses about `8.5` times, which is a larger reduction than any
other change in this document and costs no simulation throughput.

## Sizing procedure

Run this procedure during provisioning and after a capacity alert:

1. Sum `achievedTimeScale` to get `S`.
2. Read the current slot count and the crossing rate.
3. Calculate daily durable growth.
4. Read actual free space and recent daily growth.
5. Count ledger records.
6. Calculate the archive resident set, which is also the replay peak.
7. Select `MV_ARCHIVE_GOMEMLIMIT` from the host's archive ceiling.
8. Measure or select a conservative replay rate.
9. Calculate the remaining disk and memory headroom, and the date each one ends.
10. Calculate peer, publisher, and viewer transfer, and compare the total with the
    provider allowance and budget in both directions.
11. Record the result in private operations storage.

Actual recent growth outranks the model when the measurement window is representative.
The model remains useful for new peers and higher achieved time scales.

## Selection rules

Select a host that meets these conditions:

- The volume holds the announced period plus recovery headroom.
- The archive resident estimate stays below the approved memory threshold.
- `MV_ARCHIVE_GOMEMLIMIT` is selected from the host's archive ceiling, and the date on
  which the collector floor reaches that ceiling is recorded with the selection.
- The archive can replay inside the announced maintenance window, and the replay time is
  budgeted as participant outage.
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
