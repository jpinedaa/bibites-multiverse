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

**Both archive rows are per-record models and the build they describe was
replaced on `2026-08-17`.** After the record roll-up neither retained state nor
resident set follows the record count. Use them only to size a replay of a raw
ledger that has no state sidecar beside it; "The archive after the roll-up" has
the model for what is deployed.

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
Since the record roll-up it also sets the raw ledger's own window, unless a
deployment overrides that window; see "The raw ledger window" below.
It does not bound archive memory. The duplicate window does.

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

**That model describes a build with no state sidecar.** It is still the right
one for a restore from a raw ledger alone, and it is still what `monitor.sh`'s
`replay` gate computes, deliberately, because it errs toward calling a host
full. For the deployed archive read "The archive after the roll-up" below: the
record count stopped being the input.

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

The largest retained term was the duplicate-suppression set.
It holds one entry for each ledger record that carries a `migrationId`.
It held approximately 88 percent of archive resident memory before the first change below.
That set now holds a 128-bit fingerprint of each key instead of the key,
which lowered its cost from `89 B` to `26 B` for each key.

**It is now also bounded in time, and the memory model became a constant.**
Before `contract-b/4.1` the set could never forget a key.
A sender could re-forward a frame a year later, `migrations.jsonl` is never rewritten,
and a second copy of one record would put a permanent duplicate row in the record.
`contract-b-m4.md` §25 B37 removed the re-forward: a sender hands each migration over once.
What can still produce a second copy is a sidecar older than that amendment running its retry,
or a defective peer, and both arrive within seconds.
`contract-b-m4.md` §25 B38 therefore gave the set a window, `archiveDedupWindowMs`,
which is `48 hours` by default and `--dedup-window` or `MULTIVERSE_ARCHIVE_DEDUP_WINDOW`
on the command line.

The set is two generations of the same table, and it rotates on the window.
A key is refused for **at least** one window and **at most** two.
Nothing is deleted from a generation, which is what keeps the table free of tombstones;
the oldest generation is dropped whole and its memory returns in one piece.

**The arithmetic an operator can check.**
The set holds the smaller of two numbers, and never more than either:

```text
keys_held  = min(keys_in_the_whole_ledger, 2 * keys_in_one_window)
set_bytes  = keys_held * 26 B
```

`26 B` for each key is the measured cost of the fingerprint set at the load factor it keeps.
The `2 *` is the two generations. The `min` is why this change can never cost memory:
a ledger younger than two windows holds every key it ever held, exactly as before.

Read `keys_in_one_window` off your own ledger. One crossing writes two keys,
one `MIGRATION` record and one `MIGRATION_ACK` record, and a `GENOME` record writes none.

| Crossings each second, map-wide | Keys in a 48-hour window | Set at steady state |
|---:|---:|---:|
| `1` | `173,000` | `9 MB` |
| `5` | `864,000` | `45 MB` |
| `15` | `2,592,000` | `135 MB` |
| `25` | `4,320,000` | `225 MB` |

**The number stops growing, and that is the whole of the change.**
The production copy of `2026-08-16T19:35:41Z` holds `5,134,867` keys and costs `128 MiB`.
Unbounded, that figure rises for as long as the service runs: the same traffic for a year
reaches tens of millions of keys and gigabytes of resident set, and every restart pays it again.
Bounded, it settles at the table above and stays there.
The restart cost settles with it, because the replay inserts only the keys
whose records fall inside the window.

Raise the window while the map still holds sidecars older than `contract-b/4.1`,
and only while the archive has the memory for it. The cost is linear in the window.
Do not set it to zero. An archive with no window records a duplicate row
for every retry one old peer sends.

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

### The archive after the roll-up

**The record roll-up of `2026-08-17` removed the growth term.**
Before it, every restart re-derived the archive's whole fold from the raw ledger,
so retained state followed the record count and so did replay time.
The fold is now durable, in a state sidecar beside the ledger, and the only raw a
restart still parses is one duplicate window.

Use this model:

```text
resident_B ~= bounded_aggregates_B
            + min(keys_in_the_whole_ledger, 2 * keys_in_one_duplicate_window) * 26 B
            + queued_genome_gaps * 1.2 KB
```

Every term is bounded and none of them follows the ledger's length.

| Term | What bounds it |
|---|---|
| Bounded aggregates | The approved retained state: 65,536 species names, 131,072 lineage instances, 8,192 genome fingerprints for each species, brain-coverage buckets, and lanes. The next two sections explain the species and lineage bounds. |
| Duplicate keys | The duplicate window, times the crossing rate. Two generations of the table. |
| Genome-gap queue | The genome horizon. A gap is only ever queued for a crossing inside it, and a restart drains what aged out while the process was down. |

#### Species aggregate capacity

The species aggregate holds at most `65,536` distinct normalized species names. The original
`4,096` limit came from a six-world rig that produced approximately `780` species in one week. It
was not enough for the 17-world public map: the live archive reached the limit in the first week
of the announced run and refused `217,153` later migration observations. The census still listed
living species, but the full aggregate could not add them. The Species tab therefore labelled 26
of 32 living species as having no recorded ancestry even though the raw record held later
crossings for them.

`65,536` covers the announced 90-day run at the observed public growth rate with headroom. It
remains a fixed bound: the archive publishes a non-zero `ledgerOverflow` on
`/api/species/tree` if the assumption fails again. Treat any non-zero value as a capacity defect,
not as evidence that the named species never migrated.

Raising the compiled limit stops new loss but does not change an old roll-up.
During an upgrade from a saturated archive, run `restart-archive.sh --rebuild-rollup`.
The command preserves the old sidecar and rebuilds the fold from the raw record.
The command scans the durable segment receipts for an absent raw segment.
If a raw segment is absent, restore its confirmed cold copy before the rebuild.
An ordinary restart cannot recover observations that the old process did not write to the sidecar.

The production rebuild on 2026-08-22 folded `25,400,734` raw records in `406.39 s`.
The operator allowed `424 s` for the participant outage. The rebuilt aggregate held `4,379`
species names and `4,377` name-level parent claims, with `ledgerOverflow: 0`. It reduced the
living rows with no parent evidence from 14 to one excluded seed species. The old sidecar stayed
on the host as the rollback source.

#### Lineage-instance capacity

A normalized species name is a portable label. It is not a permanent family identity. The game
can reuse a name for a later species. The genealogy therefore keeps one immutable instance for
each recorded name and parent path. It binds each world to the newest instance that crossed into
or out of that world.

The archive holds at most `131,072` lineage instances. This is twice the species-name bound. It
covers ordinary name reuse and keeps hostile path churn bounded. The tree endpoint publishes
`lineageInstances`, `splitNames`, `unresolved`, and `lineageOverflow`. This overflow counts
refused instance insertions. A non-zero `lineageOverflow` is a capacity defect. `splitNames` can be non-zero on a
healthy record. `unresolved` means the ordered record cannot select one parent instance.

Roll-up format 3 persists the lineage instances and their per-world binding times. Format 2 kept
only one mutable parent for each normalized name. Do not use a format-2 sidecar for this view.
The format-3 archive refuses to start while that sidecar remains in place.
Run `restart-archive.sh --rebuild-rollup` during the upgrade. The command verifies that the raw
source is complete and preserves the old sidecar before it starts the full replay.

Measured, on two pinned cores, replaying one copy of the production ledger of
`2026-08-16` — `5,408,123` records — with the roll-up build:

| Duplicate window | Raw records inside it | Settled resident set |
|---|---:|---:|
| `48h` | `5,166,645` | `620.9 MiB` |
| `1h` | `91,858` | `485.6 MiB` |

The `135 MiB` between those two rows is the duplicate table, and it is the only
term that moved: `26 B` for each key, exactly as the table in "What the archive
retains" predicts.

**That copy carries no genome store, so every hash it names is a gap**:
`273,252` of them, which is about `313 MiB` of the figures above and is the
reason both rows are large. The live service reports one or two gaps. The same
binary on the same file with a horizon that empties the queue settled at
`176.7 MiB`, against `161.5 MiB` for the build before the roll-up — **`9 percent`
more resident for a fold that no longer has to be rebuilt.**

**Size the gap queue against a restore, not against the service.**
A restore from the ledger alone, with no genome store, is the shape that fills
that queue, and it is the shape "What the archive retains" already tells you to
size against.

### The state sidecar

`<archive-data>/rollup.jsonl` is the durable half of the fold.

| Quantity | Measured value |
|---|---:|
| File after folding a `5,408,123`-record ledger | `6.5 MB` |
| For each ledger record | `1.2 B`, against `338 B` for each record of raw |
| Appends at the shipped `30 s` save interval | `123 MiB` each day |
| Appends at `60 s` | `69 MiB` each day |
| Appends at `300 s` | `20 MiB` each day |
| For each genome gap still open when a save runs | `239 B` |
| Compaction estimate for each lineage instance | `512 B` |
| On-disk size at steady state | at most `3` times the live state, then it compacts |

The `6.5 MB` measurement predates roll-up format 3. For format-3 sizing, add the lineage-instance
term. After the first rebuild, measure the actual term on the target.

It is append-and-compact, in the shape `brains.jsonl` already used: a save
appends the keys that moved, and the file is rewritten whole when it grows past
`3` times the state it holds. So the `123 MiB` a day is write volume and not
growth. The file on disk stays around `20 MB` for the workload above.

**The save interval does not bound loss**, which is what makes it a free knob:
everything behind the sidecar's cursor is still in the raw record and the next
start folds it. It trades write volume against a few seconds of replay tail, and
never against a fact. `80 percent` of that volume is two "recent sample" terms
that exist so a restart's numbers are exact — the lanes' five-minute flow window
and each species' six newest crossings.

**The gap queue costs what the gap queue is.** `239 B` for each gap that is open
at a save. One or two on the live service; `273,252` on a copy with no genome
store beside it, which is `65 MB` of the sidecar and is an artefact of the copy.

**Losing this file costs replay time while every line it covers remains on the host.** Every value
in it is re-derivable from those raw lines. After a segment retires, recovery also needs its
confirmed cold copy; replaying only the on-host window would move the aggregate's floor forward and
lose the older answer. `backup.sh` copies the sidecar daily, and recovery preserves a damaged copy,
restores any required cold segments, and then pays one full replay.

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

**This section is history, and it is kept because the arithmetic is reusable and
because the date it computed is why the roll-up exists.**

It was written on `2026-08-16`, against the build of that date, in which the
archive's retained state grew with every ledger record there had ever been. On
that build the memory horizon was `9` to `13` days away. The record roll-up
shipped on `2026-08-17` and removed the growth term the whole calculation is
built on: retained state is now the bounded aggregates plus one duplicate
window of keys, and neither follows the record count.

**So do not read the dates below as a forecast for the running service.** Read
the method, and then read "The archive after the roll-up" for the model that
describes what is deployed. The one line to carry forward unchanged is the
growth band, because it is a property of the workload and not of the build.

Inputs, all measured on `2026-08-16`, on the build of that date:

- `5,545,189` ledger records at `20:25Z`.
- `338 B` for each record on disk.
- An archive ceiling of `1,534 MiB` on the `2 GB` host.
  That is `MemTotal` less the `373 MiB` that no other process on the box will return.
- `28` to `31 B` of retained state for each record, with the fingerprint change.
- A collector floor of `1.27` times the live heap under a binding limit.

**Growth is the input with the widest band, so carry it as a parameter and never as a
constant.** The ledger's own `41 h` lifetime average was `3.00` million records each day, in a
`2.90` to `3.14` band, and a later reading the same evening was `3.9`. Nothing structural
changed; a diurnal peak and a `41 h` mean are different measurements of one workload. Compute
every horizon at both ends and quote the shorter one.

The memory horizon:

```text
live_ceiling   = 1,534 MiB / 1.27       = 1,208 MiB = 1,266,500,000 B
record_ceiling = 1,266,500,000 B / 31 B = 40,900,000 records
               = 1,266,500,000 B / 28 B = 45,200,000 records
remaining      = 40,900,000 - 5,545,189 = 35,400,000 records
               = 45,200,000 - 5,545,189 = 39,700,000 records
days           = 35,400,000 / 3,900,000 =  9.1        (fast rate, conservative bound)
               = 39,700,000 / 3,000,000 = 13.2        (slow rate)
```

That is **`9` to `13` days from `2026-08-16`**, so between `2026-08-25` and `2026-08-30`.
It is the day the collector starts to bind hard, not the day the service fails.

**That date was never reached and cannot be, on this build.** It assumes
`31 B` of retained state for every record in the ledger for ever. Since the
roll-up the retained terms are all bounded — see the next section — so the
record ceiling is not a moving date but a fixed one that this workload does not
approach. The row for it is left in the table below as the state it describes.

Compare the states of the same host, at the same two rates:

| State, all on the `2026-08-16` build | Ceiling reached at | Days from `2026-08-16` |
|---|---:|---:|
| Before the fingerprint change, limit inert at `5GiB` | `7.5` million records | `0.5` to `0.7` |
| Before the fingerprint change, binding limit | `14.3` million records | `2.2` to `2.9` |
| After the fingerprint change, limit inert | `25` million records | `5.0` to `6.5` |
| After the fingerprint change, binding limit at `1100MiB` | `41` to `45` million records | `9` to `13` |
| **After the roll-up (`2026-08-17`)** | **no record ceiling** | **not a date** |

Disk was not the binding term on this host on that date either.
`46 GB` were free on `2026-08-16`, which is `35` to `45` days at `1.0` to `1.3 GB` each day.
The raw ledger window makes that term flat too; "The raw ledger window" has the figures.

Larger sizes at the same rates and the same measured constants:

| Public bundle | RAM | Disk | Memory horizon | Disk horizon | List price each month |
|---|---:|---:|---:|---:|---:|
| `small_3_0`, the current size | 2 GB | 60 GB | `9` to `13` days | `35` to `45` days | `$12` |
| `medium_3_0` | 4 GB | 80 GB | `22` to `32` days | `50` to `65` days | `$24` |
| `large_3_0` | 8 GB | 160 GB | `47` to `68` days | `111` to `144` days | `$44` |

Read that table for what it is: **every cell is a date and none of them is a bound.**
It describes the build of `2026-08-16`, in which retained state grew with each
ledger record and nothing in the list stopped it.

**Three things bound it, and the record roll-up shipped the third.**

- a lower crossing rate, which means a lower `S`;
- the **duplicate window**, which bounds the largest retained term and therefore
  bounds resident memory. It is the change that turned the table above into
  history: see "What the archive retains" and "The raw ledger window";
- the **raw ledger window**, which bounds the crossing lines held on the disk.

An on-disk index — resident memory following the working set instead of the
record count — was the fourth option and it was not needed. The window did the
same work for the memory term and for the disk term at once.

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

**The gate is now a conservative model of a bounded process, and it says so.**
When the archive publishes `rollupCoveredRecords`, the `replay` verdict adds a
sentence naming the record-count model as an over-estimate and pointing here.
Do not correct these constants from that sentence. Correct them from a
measurement on this host, and note that under the roll-up the honest model is
the three-term one in "The archive after the roll-up", not one number for each
record. The question of what a restart COSTS moved to its own check,
`replay-cost`, which reads the archive's own measurement of its last start.

## The raw ledger window

The archive rotates its ledger into one segment for each UTC day and compresses
each segment once it is closed.
Rotation and compression are always on. They remove nothing, so they change no
published commitment.

A **window** is the separate step that removes.
`MV_LEDGER_WINDOW` is a Go duration, and empty takes `MV_ARCHIVE_GENOME_HORIZON`.
This is one horizon and three mechanisms: the genome store evicts blobs past it,
the fetch queue retires gaps whose crossing is past it, and the raw ledger keeps
exactly the crossings whose gaps can still be fetched.

**A window removes raw crossing lines and nothing else.**
Every answer the archive publishes — lane totals, species counts, ancestry,
brain history, the record counter, the first record it ever folded — is an
aggregate and is kept for ever at every setting.

### The retirement gate

**No segment leaves this host without a confirmed copy of it somewhere else.**
The archive removes a closed segment only when all five of these hold:

1. the segment is closed, so never the live file;
2. the segment is compressed;
3. its day is older than the window;
4. a receipt beside it parses and names that segment;
5. the receipt's size and `sha256` match the file as it is on this disk **now**,
   recomputed rather than trusted, and the receipt names a destination and a
   checksum the store itself returned after the upload.

`deploy/coldcopy.sh` writes that receipt, and it writes it only after reading
the object back out of the store.
There is no timeout, no override, and no "old enough to risk it".

**So a cold archive that has stopped working costs disk and never record.**
It shows as `ledgerSegmentsAwaitingColdCopy` climbing on `/api/status`, which
`monitor.sh` watches, and as the volume filling.
With no destination configured at all, no receipt is ever written and nothing is
ever removed, whatever the window says. That is the safe default.

### Compression and the disk figures

Measured on one copy of the production ledger of `2026-08-16`:
`1,836,382,633 B` became `304,026,573 B`, which is `6.04` times, at `gzip -1`.
All arithmetic here uses a conservative `5.75`.

`gzip -1` is deliberate. The ledger is JSON lines and compresses well at any
level, and CPU is the scarce thing on a two-vCPU box that is also serving a live
map.

At `338 B` for each record and the `3.0` to `3.9` million records each day this
workload measured:

```text
slow   3.0 M x 338 B = 1.014 GB each day
fast   3.9 M x 338 B = 1.318 GB each day

a 30-day window on the host = the live day plain + 29 closed days compressed
slow   1.014 + 29 x 1.014 / 5.75 = 6.13 GB
fast   1.318 + 29 x 1.318 / 5.75 = 7.97 GB
```

| Raw ledger on the host | Today | Compression only, no window | A 30-day window |
|---|---:|---:|---:|
| Growth | `1.0` to `1.3 GB` each day | `0.18` to `0.23 GB` each day | flat |
| After 90 days | `94` to `121 GB` | `16` to `21 GB` | **`6.1` to `8.0 GB`** |

**Compression alone already fits a 90-day run in `16` to `21 GB`**, and it
changes no published commitment. The window is a separate decision, it needs its
own participant notice, and it needs a destination for the off-host copy before
it can take effect at all.

**Size the off-host store for the whole run and not for the window**: the cold
archive keeps everything, so `16` to `21 GB` compressed is the figure, and the
upload is `0.18` to `0.23 GB` each day.

Count that upload against the instance's transfer allowance.
Data transfer counts in both directions at the instance, and this workload is
already at the allowance; see "Network transfer".

`backup.sh`'s daily copy shrank by the same order.
It copies the live segment, which is at most one UTC day, and never a closed
segment: a closed segment is already compressed and already immutable, and a
second local copy of it would double the largest thing on the disk for no
protection.

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

**A restart reads one duplicate window of raw records, not the whole ledger.**
Use this model:

```text
raw_records_read = min(the raw window on this host, MV_ARCHIVE_DEDUP_WINDOW)
                   expressed in records, at this map's crossing rate
replay_seconds   = raw_records_read / measured_records_per_second
                 + the cost of reaching the window's start
```

The archive publishes both halves of the first term, measured at its own last
start: `replayRawRecords` and `replayRawSeconds` on `/api/status`.
`monitor.sh`'s `replay-cost` check projects the NEXT restart from them and from
this box's own record rate, and `restart-archive.sh` prints that projection
before it does anything.
Use the projection, not the last elapsed time: the first start after the roll-up
lands reads the whole ledger once, and every start after it reads a window.

**The second term is not free and it has no formula.**
Reaching the window's start is an `lseek` on a plain segment or on the live
file, and a decompress-and-discard on a compressed one, because a gzip stream
cannot seek. Measured: `5.9 s` to skip `1.8 GB`. In ordinary running the window
starts inside a recent day and the skip is small. The one-off legacy segment
that the first start after this deployment creates is the worst case, and it
ages out with the window.

Measured, on two pinned cores against one copy of the production ledger of
`2026-08-16` — `5,408,123` records, `1.8 GB` — with a legacy segment and a
state sidecar in place:

| Start | Raw records parsed | To answering `/healthz` |
|---|---:|---:|
| The build before the roll-up, full replay | `5,408,123` | `83.5 s` |
| First start after the roll-up, which folds everything once | `5,408,123` | `70.6 s` |
| A later restart, `48h` duplicate window | `5,166,645` (`95.5 percent`) | **`51.5 s`** |
| A later restart, `1h` duplicate window | `91,858` (`1.70 percent`) | **`14.3 s`** |

Two things that table says and that the formula alone does not.

**A window wider than the record is not a window.** That copy spans `41.3 h`, so
a `48 h` duplicate window is `95.5 percent` of it and the `51.5 s` is the durable
fold alone. The `14.3 s` row is the same binary on the same state with a window
that is genuinely shorter than the record, and it is the shape a long-running
deployment is in.

**`6.3 s` of that `14.3 s` is reaching the window's start**, through a
compressed legacy segment. Parsing `91,858` records is under two seconds.

Those are development-box figures. The service host measured `1.37` times
slower on the same file; multiply a two-core development measurement by `1.37`
to project it.

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

Replay time no longer grows with every retained ledger record.
It grows with the duplicate window and with the crossing rate, and it is flat in
the length of the run.
Recalculate it before each planned archive restart anyway: the crossing rate
moves, and `monitor.sh` is what recalculates it.

An archive does not subscribe during replay.
If the relay stays live, every crossing during replay creates a gap in the archive record.
The record-preserving sequence holds the relay down for the whole replay,
so the replay time is also the participant outage.

**Lowering the duplicate window is the one change that lowers this number.**
`MV_ARCHIVE_DEDUP_WINDOW` ships at `48h`, which is sized against how far behind
the oldest sidecar on the map can be and not against a relay retransmit.
`duplicatesRefused` on `/api/status` is the evidence for lowering it: it is
all-time, it is published even when it is zero, and zero is what says the fleet
has crossed.

The figures the old model produced are kept for the case it still describes —
an archive with no state sidecar, which is any archive replaying from a raw
record it has no fold for. At `61,000` records each second, `10` million records
cost `164 s` and `40` million cost about `11` minutes.

Neither replay time nor memory is what ends this host's life now.
Both are bounded by the duplicate window, and the disk is bounded by the ledger
window. What is left is the crossing rate itself.

## Relay pacing for frames and responses

Contract B §3.3 defines the transport bounds and §5.2 defines the attempted-write boundary.

`F` is the relay's `maxFramesPerSecond` value. This value is an inbound ceiling for
each peer connection. The relay publishes it in `HANDSHAKE_ACK` and `PEER_STATUS`.
Current relays refuse `F < 8` at start.

The relay derives one migration rate from `F`:

```text
R = max(1, floor(F / 8))
migration_interval = ceil(1 second / R)
```

`R` is not a second operator knob. A change to `F` changes both the connection ceiling
and the migration rate. The production value is `F = 50`, so `R = 6`. The destination
writer therefore spaces physical migration writes by at least `166,666,667 ns`.

This rate applies separately to each destination peer identity. Different destination
identities have independent rates. Writers in a reconnect overlap share the identity's
schedule, so a second connection does not double `R`. Thus `R` is not a map-wide crossing
limit.

### Destination transport queue

`B` is `maxBytesPerSecond`. Each destination connection has a migration transport queue
with these two bounds:

```text
queued_migration_frames <= F
retained_migration_bytes <= B
```

Both bounds apply at admission. The production values are `F = 50` and
`B = 4,194,304 B`.

The relay applies the source frame and byte meters before queue admission. Thus a single
frame larger than `B` normally closes the source with `4007` before the queue evaluates it.
The relay then validates the required routing fields and the `migrationId` UUID. An invalid
or empty `migrationId` closes the source as malformed before queue admission.

Source-meter admission does not guarantee destination queue space. If that space is
insufficient, the relay returns `NOT_FORWARDED` and does not close the destination.

The queue retains the byte-identical Contract B frame. The relay does not decode the body,
lineage, or species data. It does not create an organism job. For a frame with valid relay-visible
Contract B fields, a successful enqueue and its forwarding record form one atomic
attempted-write boundary.

If admission refuses the queue, the attempt adds no forwarding record. The source handler
does not wait for capacity or for a writer turn.

These payload bounds apply to capacity planning:

```text
retained_migration_frames <= destination_connection_count * F
retained_migration_bytes  <= destination_connection_count * B
```

The connection count includes a temporary reconnect overlap. These formulas bound retained
frame bytes. They do not include queue-item, WebSocket, or Go runtime overhead.

### Production control headroom

One written migration can cause one immediate `MIGRATION_ACK` or `MIGRATION_NACK` from
the destination. The production relay therefore limits this response cadence to six frames
each second for each destination.

The sidecar derives its bucket for deferred sends from the same published value. The refill rate
is one half of `F`, and the burst capacity is one quarter of `F`. Deferred traffic cannot
use half of that burst capacity. This protected half is the immediate control reserve.

The production frame arithmetic is:

| Term | Value at `F = 50` |
|---|---:|
| Relay migration write rate for one destination | `6/s` |
| Minimum migration interval | `166,666,667 ns` |
| Sidecar deferred refill rate | `25/s` |
| Sidecar total burst capacity | `12` frames |
| Protected immediate reserve | `6` frames |
| Deferred traffic plus bounded migration responses in one second | at most `37` frames |
| Remaining frame headroom for normal control traffic | at least `13` frames |

The six-frame immediate reserve matches the six-frame migration response cadence. Normal
`PONG`, stats, and claim traffic uses the remaining headroom. An immediate frame can create
bucket debt. Deferred traffic then waits until refill clears that debt.

A lower `F` is not a migration-only tuning method. This change also lowers liveness and
control headroom. Current code rejects values less than eight because those values cannot
preserve the one-eighth response reserve.

### Sidecar response classes

| Frame class | Send rule | Reason |
|---|---|---|
| `MIGRATION_PAYLOAD` | Deferred | The outbound journal offers the frame again. |
| Ordinary journal `MIGRATION_ACK` | Deferred, with strict priority over payload | `AckedUpstream` stays false until the sidecar transport accepts the frame. |
| Immediate `MIGRATION_NACK` | Bypasses the deferred gate | Relay pacing bounds the arrivals that cause these replies. |
| Tombstone re-ACK | Bypasses the deferred gate | The mod proved delivery. This reply does not clear the pending durable ACK. |
| `PONG` and other liveness control | Bypasses the deferred gate | Liveness cannot wait behind a journal drain. |

Each frame that passes the sidecar gate consumes its frame and byte budgets. Every frame that
reaches the relay also consumes its meter. A bypass is not a free frame. Immediate control
can spend unavailable tokens and creates debt.

Deferred frames wait for `HANDSHAKE_ACK`. Before that frame arrives, the sidecar does not
know the relay's session ceiling. A reconnect therefore cannot release an unpaced journal
backlog before the relay publishes its limit.

### Writer priority and close behavior

`C` is `MigrationControlBurst`:

```text
C = MigrationFanInDivisor - 1 = 7
```

While a migration turn is not due, non-migration frames can move ahead freely. After a turn
becomes due, at most `C` non-migration frames can move ahead before the migration writes.
This priority keeps `PONG` and other control traffic responsive without starving migration.
The formula defines `C`. It is not an operator knob.

Admission never waits for `R`. The source handler returns after a successful enqueue or a
`NOT_FORWARDED` response. Relay pacing therefore creates no source head-of-line wait and no
read-recovery credit.

An accepted migration stays in the selected destination connection queue. A connection-local
close or replacement drops queued migrations. At most one migration that the writer already
selected can finish. The relay does not move the backlog to the replacement, re-route it, or
send a later NACK.

The forwarding record already marks these frames as attempted writes. A later drop is
therefore delivery-ambiguous and follows the at-most-once loss rule. The queue is not custody,
and this design is not a lossless transport.

A relay-wide drain uses a different close path. It rejects new migration admission with
`NOT_FORWARDED` and no forwarding record. It drains all ordinary queues and accepted migration
queues. The migration queues keep their identity rates. All connection drains run concurrently
with one shared 18-second deadline.

For all supported `F` values, pacing delay alone is at most 15 seconds for a full queue. The
remaining margin covers scheduling and close work. This margin does not guarantee that all
queues drain. The shared deadline overrides a longer write timeout and force-closes the
remainder conservatively.

At the deadline, the relay drops each remaining queue and keeps its forwarding records. It
sends no late NACK.

At the production rate, a full 50-frame queue needs approximately `50 / 6 = 8.34 seconds`
of migration write turns. Non-migration frames can add bounded delay. The service-stop window
must include the full 18-second deadline. The 8.34-second calculation is not a guarantee.

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
peer_GB_per_month = 21 * total_crossings_per_second + 5.9 * slot_count^2   compressed wire
peer_GB_per_month = 88 * total_crossings_per_second + 5.9 * slot_count^2   uncompressed wire
crossings_per_second_of_one_world = 0.010 * population * achieved_time_scale
```

Both terms count both directions. The first term is organism migration: one envelope for
each edge crossing, measured at approximately `17.9 KB` on the uncompressed wire. The
second term is the periodic map-status broadcast, which every peer and the archive
receives, so its map-wide cost grows with the square of the slot count.

`88` is the reference measurement on an uncompressed wire, and it stays because it is what
was measured. `21` is `88` divided by the whole-wire reduction of `4.09` measured on the
live map on `2026-08-17`, after the peer wire began negotiating compression. Use `21` to
plan a compressed map at about the current world size, and treat it as derived rather than
measured: the `4.09` covers migration and status frames together, and this line applies it
to the migration term alone. Re-measure after a change to the world size, the status
cadence, or the wire format. "The peer wire is compressed now" below states the
measurement and its limits.

A seven-slot reference map on the uncompressed wire measured `2,508 GB` each month across
all peers: `82 percent` migration payloads, `12 percent` status broadcasts, `3 percent`
genome responses, and the balance in acknowledgements and pings. Duplicate migration identifiers were `0` to `1`
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

The status coefficient is an upper bound and it has not been re-measured on its own. The
reference map emitted status frames about six times more often than its own design
intends, and the relay release of `2026-08-16` fixed that cadence. `5.9` therefore
describes the map before that fix and overstates the map after it. It is the smaller of
the two terms, so the over-estimate is conservative in the safe direction.

#### The peer wire is compressed now

The peer wire was uncompressed JSON. WebSocket `permessage-deflate` is now negotiated at
every peer endpoint, and the relay reports `peer wire compression offered=true` at start.
**It was the largest available reduction in this document, it cost no simulation
throughput and no fidelity, and it is done.**

Offline `deflate` at level `6`, with a `32 KiB` window and per-stream context takeover,
measured `9.0x` on migration payloads, `9.8x` on status frames, `10.4x` on genome
responses, and `8.5x` overall. **The live map measured `4.09x`, and the gap is not a fault
in the compressor.** The same relay release also fixed the `PEER_STATUS` cadence, and that
fix removed the most compressible bytes from the wire before compression could remove
them. **Two reductions that act on the same bytes are partly substitutes: their separate
factors do not multiply.** Measure the pair, never the pieces added together.

Measured on the service host at `2026-08-17T01:05Z`, six cloud worlds, at a work rate
about `1.7x` higher than the earlier uncompressed reading:

| Quantity | Reading |
|---|---:|
| Six peers, outbound | `8.807 GB` each day |
| Six peers, inbound | `8.351 GB` each day |
| Six peers, both directions | `531.9 GB` each month |
| The same six before the change | `2,173 GB` each month |
| Whole-wire factor | `4.09` |
| For each slot | `3.9` to `4.8` outbound, `3.5` to `4.4` inbound |
| One peer, each direction | `1.4` to `1.6 GB` each day |
| Archive loopback subscription | `36.6` to `9.14 GB` each day |

The archive subscribes over loopback, so its `4.0x` saving never reaches the billed
interface. It is real work removed from the host all the same.

The seventh slot is a world on another machine, and it reaches the relay over the public
interface. Its own socket measured `0.90 GB` outbound and `0.92 GB` inbound each day at
`2026-08-17T01:46Z`, from a 60-second sample, which is about `56 GB` each month. It runs
the same source as the other six and negotiates compression too.

Budget `7x` to `8.5x` only for a wire whose status cadence has not already been fixed, and
about `4.5x` without context takeover. Each compression state costs approximately `260 KB`
of memory for each connection, so choose a smaller window or no context takeover on a map
of more than a few dozen peers.

### Video

Publisher ingest is an inbound cost, and it is paid for every minute the publisher runs
rather than for every minute anyone watches:

```text
ingest_GB_per_month = 312 * publish_Mbit_per_second      while publishing
```

**The publisher runs at `1,500 kbit/s` CBR since `2026-08-16`, so a continuous
publishing-month costs approximately `470 GB`.** It ran at `2.5 Mbit/s` before that, which
cost approximately `780 GB`: a quarter of a `3,072 GB` allowance, spent on an empty room.
The publish bitrate is the lever that multiplies this term and every viewer term together,
which is why it is the first one to reach for.

**Publish on demand, and the empty-room term goes to approximately zero.** This is live.
The steady state with no audience is no publisher and therefore no ingest; the service pays
the rate above only for the minutes somebody watches. Measured on `2026-08-17`: the
publisher starts `16 s` after the first external request for the stream, stops after
`181 s` with no viewer, and the origin reports `rtmp_conns 0` in between. It needs a
presence signal the publisher can read — see
[`../docs/live-broadcast.md`](../docs/live-broadcast.md), "Publish on demand".

Viewer cost is not the media rate:

```text
viewer_GB_per_month = 475        fmp4 HLS, 1.5 Mbit/s media    the current origin
viewer_GB_per_month = 790        fmp4 HLS, 2.5 Mbit/s media
viewer_GB_per_month = 1150       low-latency HLS, 2.5 Mbit/s media
```

`790` is the measured line, taken when the origin carried `2.52 Mbit/s` of media. The
origin now carries `1.5 Mbit/s`, and `475` is that measurement scaled by the bitrate:

```text
1.5 / 2.5 * 790 = 474
```

The scaling holds because this variant's delivery overhead is a small fixed cost for each
segment rather than a share of each byte, so halving the media halves the total. The
origin's own playlist agrees: it advertises `AVERAGE-BANDWIDTH=1507906`, which is
`1.51 Mbit/s` including container overhead against `1.50 Mbit/s` of video. Replace `475`
with a direct measurement once one continuous viewer has been observed long enough to make
one.

Low-latency HLS delivered `3.69 Mbit/s` for `2.5 Mbit/s` of media, because it re-fetches
its playlist approximately as often as it delivers a media part: a measured `1.035` to `1`
playlist-to-part ratio, with `31.8 percent` of delivered bytes in playlists. Dropping it
removed that overhead and moved viewer latency from about `1 s` to about `6` to `8 s`.

**Low-latency HLS is also the variant a cache cannot help.** Each playlist request stays
open until the next part exists, so an edge cannot serve it from a stored object. The
non-low-latency variant is therefore the precondition for a content delivery network, not
an alternative to one.

Match the segment target to the encoder keyframe interval. A server extends a segment
until the segment contains a keyframe, so a `1 s` target against a `2 s` keyframe interval
produces `2 s` segments and a configuration that describes nothing. The current origin
matches them, measured on `2026-08-17`: the publisher encodes a keyframe every `60` frames
at `30` frames each second, which is `2 s`, and the delivered segments measure `2.00 s`.
The playlist advertises `#EXT-X-TARGETDURATION:4`, which is the server's own ceiling and
not the segment length. Read the segment durations when you check this, never the target.

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
budget. At `475 GB` for each continuous viewer, ten continuous viewers is not a capacity
question but a four-figure monthly bill.

Before the peer wire was compressed and the publisher moved to `1.5 Mbit/s` on demand, a
seven-peer map with one continuous publisher spent approximately `3,300 GB` each month
before the first viewer arrived, and approximately `2,500 GB` with an on-demand publisher
and nobody watching. Both figures are history. They are kept because they are what a
`3,072 GB` allowance was first measured against.

**Steady state with zero audience, after the change.** The same seven-slot map, with the
compressed wire and the on-demand publisher, projects this on `2026-08-17`:

| Term | Reading each month |
|---|---:|
| Six cloud peers, both directions | `532 GB` |
| The seventh peer, on another machine | `56 GB` |
| Publisher ingest with an empty room | approximately `0` |
| Console, pages, and the presence document | small; see "Console" |
| **Total** | **approximately `590 GB`** |

That is about `19 percent` of a `3,072 GB` allowance. The same map projected over
`114 percent` of it before the change.

**Read that as a first reading, not as a month.** It is a projection from short samples
taken hours after the deployment: the six-peer figure is one reading at
`2026-08-17T01:05Z`, and the seventh is a 60-second sample at `01:46Z`. A host's own
trailing-24-hour projection still carries the uncompressed hours inside its window and
falls over the following day as they leave it, so a projection read too early reports the
old service. Confirm the figure against the transfer check over several days, and against
the provider's own metered quantity over several settled days, before you plan on it; see
[`../docs/observability.md`](../docs/observability.md), "Layer 4 — Cost".

Size the allowance from the crossing rate and the slot count, and treat each watched hour
as an addition to it. The peer wire is already compressed, so the next reduction, if the
map grows or an audience arrives, is a cache in front of the video — which the
non-low-latency variant already allows.

## Sizing procedure

Run this procedure during provisioning and after a capacity alert:

1. Sum `achievedTimeScale` to get `S`.
2. Read the current slot count and the crossing rate.
3. Read `F`, `B`, and the destination connection count. Calculate `R` and the retained
   migration queue bounds.
4. Reserve the full 18-second relay drain deadline in the service-stop window.
5. Calculate daily durable growth.
6. Read actual free space and recent daily growth.
7. Count ledger records.
8. Read `ledgerSpecies`, `lineageInstances`, `ledgerOverflow`, and `lineageOverflow` from
   `/api/species/tree`. Either overflow is a capacity defect. After you raise a compiled limit,
   rebuild the fold while the raw source still exists.
9. Calculate the archive resident set, which is also the replay peak.
10. Select `MV_ARCHIVE_GOMEMLIMIT` from the host's archive ceiling.
11. Measure or select a conservative replay rate.
12. Calculate the remaining disk and memory headroom, and the date each one ends.
13. Calculate peer, publisher, and viewer transfer, and compare the total with the
    provider allowance and budget in both directions.
14. Record the result in private operations storage.

Actual recent growth outranks the model when the measurement window is representative.
The model remains useful for new peers and higher achieved time scales.

## Selection rules

Select a host that meets these conditions:

- The relay memory budget includes `B` retained frame bytes for each destination connection,
  plus the queue and runtime overhead.
- The service-stop window includes the full 18-second relay drain deadline.
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
