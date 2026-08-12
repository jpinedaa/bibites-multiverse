# Sizing — the arithmetic the hoster is owed

**This document is Decision 3's other half.** The ratification said the retention
rule is decided in M5 *"even if the rule turns out to be keep everything"*, and
that **the arithmetic ships to the hoster either way** — *"published per-peer
growth rates and a sizing procedure, so that whoever runs the relay knows what
they signed up for before the volume fills rather than after"*. That is what is
below. It is a rule and a procedure, not a number, because the number depends on
something the operator cannot set.

The measurements are the living deployment's, taken 2026-08-11 ~22:10Z, and the
full derivation with its provenance is in `wp3_hosting_options.md`. This file is
the operational form: what to buy, what to watch, and what to do when the watch
fires.

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

Disk is the obvious constraint and it is not the binding one. **The archive's
startup replay is.**

| | Measured | Per ledger record |
|---|---|---|
| Resident, 599 k records | 170 MB | 0.28 KB |
| Resident, 7.43 M records | 2.26 GB | **0.30 KB** |
| Replay high-water, 3.70 M records | 5.2 GB | 1.41 KB |
| Replay high-water, ~6.4 M records | 8.24 GB | **1.30 KB** |

Two independent pairs, twelvefold apart in scale, both linear:

> **0.30 KB resident per ledger record. 1.30 KB peak while replaying.**

At the exit-test bar (S = 5, 242,000 records/day):

| Milestone | Records | Day of the run |
|---|---|---|
| Resident crosses 2 GB — a 2 GB instance stops holding the archive | 6.7 M | **day 28** |
| Replay peak crosses 8 GB — an 8 GB instance stops being able to **restart** it | 6.2 M | **day 26** |
| Resident crosses 4 GB | 13.3 M | day 55 |
| Resident crosses 8 GB | 26.7 M | day 110 |
| Day 90: 6.5 GB resident, **28 GB peak**, ~9 minutes | 21.8 M | — |

**About 77% of the replay peak is garbage rather than state** — 1.30 KB peak
against 0.30 KB retained — because Go's collector is not being asked to run
during a tight replay loop. `GOMEMLIMIT` is the one-variable lever, and it is
**not** a certainty: trading resident memory for GC time can turn a nine-minute
replay into a much longer one, and the archive serves nothing until it finishes.
That is the experiment `MV_ARCHIVE_GOMEMLIMIT` and `MV_SWAP_GB` are waiting on.

`monitor.sh`'s **replay** check is this table, evaluated continuously against the
box's real RAM plus swap. It is the tripwire that says *the archive can no longer
be sure of restarting* before a restart proves it.

**Every recorded replay figure expires the day after it is written.** Size a
replay from `wc -l` on the day, never from a quotation in a document — including
this one.

## 5. Decision 3 is the instance-sizing decision

| The rule | Bundle | Three months | What is bought |
|---|---|---|---|
| Anything that **bounds the ledger** | **$12** — 2 GB, 60 GB, 3 TB | **$36**, or **$0** on the 90-day trial | The archive stays small and restarts in seconds. |
| **Keep everything** | **$44** — 8 GB, 160 GB, 5 TB | **$132** | 8 GB holds it *resident* to day 110 and stops being able to *replay* around day 26. **Not restartable in month three unless `GOMEMLIMIT` or swap is proven first.** |

The gap is about **$96 over the period, and it is the difference between a
service that restarts and one that does not.**

Set the answer in `deploy.env` as `MV_RETENTION`. It changes no command in this
kit — it changes the bundle, the two memory values, the backup tier and which arm
of `WIND-DOWN.md` gets announced. Writing it down is the point: an operator
decision that lives only in somebody's memory is not a rule.

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
5. **Ledger records** — `jq '.ledgerRecords'` — × 1.30 KB is the replay peak.
   Compare against `MemTotal + SwapTotal`. Above 0.85 of it, the archive is one
   restart away from not coming back.
6. **If disk is the problem**: grow the Lightsail volume online, or apply the
   retention rule. Growing is a bill; running out is the 2026-08-08 ENOSPC
   outage, which stopped every genome write and left durability damage that took
   days to understand.
7. **If memory is the problem**: `GOMEMLIMIT`, then swap, then the retention
   rule. In that order, because the first two are reversible.

## 8. What is NOT bounded, and must not be

Nothing here evicts from the ledger or the genome store. `contract-b-m4.md` §10
and §20 forbid it and D11 makes those files the seed of M7. A retention rule that
prunes is a rule about **what is written from now on** or about **what is moved
out of the live store at the wind-down**, announced before anybody joins — never
a background job that quietly deletes what participants were told was a record.
`WIND-DOWN.md` is where that is written down.
