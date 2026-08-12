# Sizing — the arithmetic the hoster is owed

**This document is Decision 3's other half.** The ratification said the retention
rule is decided in M5 *"even if the rule turns out to be keep everything"*, and
that **the arithmetic ships to the hoster either way** — *"published per-peer
growth rates and a sizing procedure, so that whoever runs the relay knows what
they signed up for before the volume fills rather than after"*. That is what is
below. It is a rule and a procedure, not a number, because the number depends on
something the operator cannot set.

The measurements are the living deployment's, taken 2026-08-11 ~22:10Z, except
the replay-memory figures in §4, which are a 2026-08-12 rig experiment on a copy
of that deployment's ledger. The full derivation with its provenance is in
`wp3_hosting_options.md`. This file is the operational form: what to buy, what to
watch, and what to do when the watch fires.

---

## 1. The one variable that matters

> **S = the sum of achieved time scales across every peer on the map.**

Not peer count. Every durable term in this system is produced by **simulated**
time, not wall clock: organisms cross when they walk into a portal, genomes
appear when evolution produces them. Six players at the game's default ×1 is
**S = 6**. The living deployment runs **S = 67.9** across six worlds — eleven
times a real map's rate — which is why its own figures have to be divided before
they mean anything about strangers.

**S is not an operator knob and will not become one during this run.** D23
deferred the control surface to M6, so the operator can watch a participant run
their world at seven times everyone else's speed and can do nothing about it.
Risk 9 recorded that as a measurement-quality cost. It is also a **billing**
cost: one enthusiastic participant at ×20 adds as much to the archive as twenty
participants at ×1.

Read S off the status page: `achievedTimeScale` per slot, summed.

## 2. The growth rule

> **Durable bytes per day  =  56 MB × S  +  1.6 MB × (number of slots)**

The second term is `metrics.jsonl`, which samples on a wall-clock timer and is
therefore speed-independent. The first term breaks down as:

| Term | Per day, per unit of S | Where it comes from |
|---|---|---|
| Crossings | 22,600 | 1,065/min measured ÷ S = 67.9 |
| Ledger records | 48,350 | × 2.14 records per crossing |
| **Ledger bytes** | **15.9 MB** | × 337 B/record |
| **Genome store, demand** | **~40 MB** | ~14.2 KB per object |
| Genome store, *observed* | ~18.6 MB | the store's actual growth |
| Relay egress from forwards | 357 MB | × 15.8 KB on the wire per crossing |

**Size against demand — 40 MB/day/S — and not against observed growth.** The
difference between the two is `genomeGaps`, the archive's arrears on genome
capture, which went 46,000 → 117,000 → 155,000 in two days on the rig. The
arrears are a debt the archive pays the moment it gets a quiet hour, so the
observed figure is a **queue's length, not its arrival rate**. Sizing off it
means sizing off how far behind you are.

The part of that debt which is never paid is Risk 7's: **a stranger who leaves
takes their unfetched genomes with them permanently.** Nothing recovers those.

**Add to every total:** 1.2 GB of rotated logs (100 MB × 6 generations × two
processes) and 3–5 GB for the OS image. `ring.json` and `peers.json` are
kilobytes.

## 3. Over D24's three months

| The map | S | GB/day | **90 days** | Ledger records at day 90 |
|---|---|---|---|---|
| 5 peers @ ×1 — the exit-test bar plus the owner | 5 | 0.29 | **26 GB** | 21.8 M |
| 8 peers @ ×1 | 8 | 0.46 | **41 GB** | 34.8 M |
| 12 peers @ ×1 | 12 | 0.69 | **62 GB** | 52.2 M |
| 12 peers @ ×5 — players who use the speed slider | 60 | 3.37 | **303 GB** | 261 M |

Disk to buy, including logs and OS:

| The run being provisioned for | Disk |
|---|---|
| The exit test and a quiet three months | **32 GB** |
| A modestly successful run | **70 GB** |
| A successful run where people use the speed slider | **310 GB** |

Lightsail can grow a volume online, which converts a sizing error from an outage
into a bill. That is worth more than getting the number right the first time.

## 4. The term that actually sizes the machine: memory

Disk is the obvious constraint and it is not the binding one. The archive has to
do two different things with memory and they have different limits: **hold** the
ledger's state while it serves, and **peak** while it replays the file at
startup. Until 2026-08-12 the peak was the binding one by a factor of four. It is
not any more, and that is a change to the machine you buy.

| | Measured | Per ledger record |
|---|---|---|
| Resident, 599 k records | 170 MB | 0.28 KB |
| Resident, 7.43 M records | 2.26 GB | **0.30 KB** |
| Replay high-water, materialising, ~6.4 M records | 8.24 GB | 1.30 KB |
| **Replay high-water, streaming, 8.16 M records** | **1.50 GB** | **0.18 KB** |

> **0.30 KB resident per ledger record. 0.18 KB peak while replaying.**

The replay figure is the one that moved. The archive used to read the whole
ledger into one list and walk it afterwards, so the peak was the size of the file
by construction and **about 77% of it was garbage the collector could not
reclaim** — nothing can collect a live list. It now streams: each record is
applied and dropped. On a copy of the living deployment's own 8,156,868-record
ledger that is **184 B of peak per record against 1,030–1,286 B — 5.6–7.0×
lower, at no measurable wall-clock cost**, and it is about 1.1× the state the
replay actually retains, so there is no remaining headroom to buy. The full
matrix, including what `GOMEMLIMIT` alone bought and what it cost, is in
`wp3_hosting_options.md`.

At the exit-test bar (S = 5, 242,000 records/day):

| Milestone | Records | Day of the run |
|---|---|---|
| Resident crosses 2 GB — a 2 GB instance stops holding the archive | 6.7 M | **day 28** |
| Resident crosses 4 GB | 13.3 M | day 55 |
| Resident crosses 8 GB — an 8 GB instance stops **holding** it | 26.7 M | **day 110** |
| Replay peak crosses 8 GB — an 8 GB instance stops being able to **restart** it | 43.5 M | **day 180** |
| Day 90: 6.5 GB resident, **4.0 GB peak**, ~9 minutes | 21.8 M | — |

**Read those two 8 GB lines in that order.** The wall on an 8 GB box used to be
day 26 and it was a *restart* wall; it is now day 110 and it is a *hold* wall —
past the announced ending, with the replay not binding until day 180. Resident is
sized from the living deployment's 0.30 KB/record and not from the streamed
build's own retained set, which measured 0.16 KB/record right after replay: no
streamed archive has served live traffic for a day yet, and the deployment's
figure is the one that has. **That gap is worth a day of somebody's attention.**
Part of what the running archive holds is its own replay, which it never fully
gives back, so if 0.16 KB survives a day of service every resident date above
moves out by about 80% — day 28 becomes day 50 on a 2 GB box — and until a
streamed archive has served that day, this kit and `monitor.sh` size the
conservative way on purpose. A tripwire that fires early costs a look; one that
fires late is the outage it existed to prevent.

`GOMEMLIMIT` is a real lever and it is **no longer the answer** — it moved the
old wall from day 26 to day 43 and cost 17–39% more CPU. `MV_ARCHIVE_GOMEMLIMIT`
is set to **5GiB** as a ceiling against regression rather than as a fix, and
`MV_SWAP_GB` is **0**: swap buys time against a peak that no longer exists, and
`deploy.env.example` says why in more detail than belongs here.

`monitor.sh`'s **replay** check is this table, evaluated continuously against the
box's real RAM plus swap. It is the tripwire that says *the archive can no longer
be sure of restarting* before a restart proves it. **Its per-record constant is
the streaming archive's**, so it is paired with the binary in this kit; an
instance still running an archive built before 2026-08-12 must be projected at
1.30 KB/record instead, and its wall is the old day 26.

**Every recorded replay figure expires the day after it is written.** Size a
replay from `wc -l` on the day, never from a quotation in a document — including
this one. Note also that `wc -l` is the denominator: `/api/status`'s
`ledgerRecords` is the same number from 2026-08-12 onward, and runs about 1.2%
*below* the file on any archive older than that.

## 5. Decision 3 is the instance-sizing decision

| The rule | Bundle | Three months | What is bought |
|---|---|---|---|
| Anything that **bounds the ledger** | **$12** — 2 GB, 60 GB, 3 TB | **$36**, or **$0** on the 90-day trial | The archive stays small and restarts in seconds. |
| **Keep everything** | **$44** — 8 GB, 160 GB, 5 TB | **$132** | 8 GB holds it *resident* to day 110 and can *restart* it to day 180. **Restartable for the whole announced period**, on the streamed replay. |

The gap is about **$96 over the period, and it used to be the difference between
a service that restarts and one that does not.** It is not any more: the
streaming replay made the $44 bundle carry an unbounded ledger for the entire
announced period, so the $96 now buys **headroom past the ending** rather than
the ability to come back in month three. **Decision 3 is a promise to
participants again**, which is what it should have been all along.

**One thing the rule does not buy, and it is the one an operator will assume it
does: pruning genome BLOBS does not bound the memory.** The resident set grows
with ledger *records*, and the ledger is the thing a blob horizon does not touch.
A rule of that shape saves disk and the egress behind it, and the box still has
to be sized from §4's table. Only a rule that bounds the ledger changes those
dates.

**A third plan exists and is cheaper than either row: start small and resize on
the tripwire.** Lightsail resizes through a snapshot, so a run that begins on the
$12 bundle and moves up when `monitor.sh`'s **replay** check goes WARN converts a
sizing error into a bill and a few minutes of downtime instead of an outage —
which is the same argument §3 makes about growing the volume. It costs an
announced restart, so it belongs in the plan rather than in the surprise:
`RESTART-POLICY.md` §1 case 4.

Set the answer in `deploy.env` as `MV_RETENTION`. It changes no command in this
kit — it changes the bundle, the backup tier and which arm of `WIND-DOWN.md` gets
announced. Writing it down is the point: an operator decision that lives only in
somebody's memory is not a rule.

## 6. Egress, for completeness

At the exit-test bar, monthly:

| Flow | Monthly |
|---|---|
| Migration forwards out of the relay | 54 GB |
| Status page, **per continuously open browser tab** | 32 GB uncompressed, **~4 GB gzipped** |
| Genome fetches served through the relay | up to 92 GB (ceiling; observed far below) |
| **Realistic budget** | **90–150 GB/month**, tail to ~250 GB |

The Lightsail bundles include 3–5 TB, so egress is not a bill here — it is a
number that stops needing attention, which for a one-person operator carrying
Risk 10 is worth more than the money it saves.

**The status page is the largest single egress term in the service, and it is
bigger than the game traffic.** Gzip negotiation is implemented in the archive
and is what turns ~32 GB/month/tab into ~4 GB; nginx passes `Accept-Encoding`
through untouched so the saving is preserved rather than re-implemented at the
proxy.

## 7. The sizing procedure

Run this on the day the instance is created, and again whenever `monitor.sh`'s
disk or replay check fires.

1. **Read S.** `curl -s https://<domain>:<status-port>/api/status | jq '[.slots[].achievedTimeScale // 0] | add'`
2. **Read the slot count.** `... | jq '.slotCount'`
3. **Daily growth** = `56 × S + 1.6 × slots` MB.
4. **Days of disk left** = free bytes ÷ that. `monitor.sh` also reports this from
   the box's own last 24 hours, which beats the model whenever they disagree.
5. **Ledger records** — `wc -l` on `migrations.jsonl`, or `jq '.ledgerRecords'`
   on an archive from 2026-08-12 or later — × **0.18 KB is the replay peak** and
   × **0.30 KB is the resident set the box must hold all day**. Compare the
   larger of the two against `MemTotal + SwapTotal`. Above 0.85 of it, the
   archive is one restart away from not coming back. On this build **resident is
   the larger number**, which is a reversal: the check that matters is now "can
   this box hold it", not "can this box restart it".
6. **If disk is the problem**: grow the Lightsail volume online, or apply the
   retention rule. Growing is a bill; running out is the 2026-08-08 ENOSPC
   outage, which stopped every genome write and left durability damage that took
   days to understand.
7. **If memory is the problem**: confirm the archive is a streaming build first —
   an older binary peaks seven times higher and the fix is the upgrade, not the
   knobs. Then `GOMEMLIMIT`, then swap, then the retention rule. In that order,
   because the first two are reversible.

## 8. What is NOT bounded, and must not be

**The ledger is unbounded by rule and always will be.** Nothing evicts from
`migrations.jsonl` at any setting of any knob: `contract-b-m4.md` §10 forbids it,
D11 makes it the seed of M7, and a rule that pruned it would delete what
participants were told was a record.

**The genome store is the operator's choice, and since §23 B33 it can be
bounded.** `MV_ARCHIVE_GENOME_HORIZON` sets a horizon on blobs — unset by
default, `720h` as deployed — and the archive evicts past it **during the run**,
not only at the wind-down. That is a background job that deletes, which is
exactly what the paragraph above forbids for the ledger, so it is bought with a
promise instead of a prohibition: **the horizon is announced before anybody
joins**, it takes only the blob and never the record, and a hash whose blob is
gone remains a lineage node in the ledger and in the gap report forever.
`WIND-DOWN.md` §4 is where the disposition is written down and
`ANNOUNCEMENT.md` is where participants are told.

**What it does and does not buy.** A horizon turns the genome store's growth from
a total into a rate — the steady state is a horizon's worth of blobs — so it
bounds **disk**, and disk was never the term that sized the machine. It does not
touch §4's memory table, because that grows with ledger *records*.
