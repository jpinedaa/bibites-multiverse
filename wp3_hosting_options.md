# WP3 Hosting Options — a costed set, for iteration

**Status: an options document. Nothing here is decided.** It exists so that the hosting calls
WP3 waits on can be made against arithmetic instead of against a feeling, and every option in it
is written to be argued with. Where a number is a measurement it says so and says when; where it
is a vendor's published price it says which page and what date; where it is a guess it is called
a guess.

**What it is for.** `m5_tracking.md` records WP3 as *Not started — opens on the owner's hosting
calls*, and `system_decomposition.md`'s **D24** now fixes one of those calls: a **bounded,
announced** run of **3 months**, chosen 2026-08-11. The substrate is still open, and the owner has
narrowed it to **AWS or GCP** and raised a second idea worth taking seriously — **running game
simulations in the cloud on spot instances, with handoff as they come and go, over cheaper
permanent storage**. This document costs both.

**What it is not.** No account exists, nothing is signed up for, nothing is deployed. It also does
not decide the retention rule, which is Decision 3's, nor the wind-down, which is WP3's own
deliverable. What it does is put a **dollar figure on Decision 3**, which turns out to be the
most consequential thing in it.

**Three things it found that were not in the plan**, and which the owner should read before the
options:

1. **The archive's memory, not its disk, is what sizes the machine.** The relay is 93 MB and
   4.5% of one core. The archive's *steady state* is cheap too. But its startup replay wants
   **1.3 KB of RAM per ledger record**, and DQ2's whole argument is that on a hosted service
   restarts stop being rare. At the exit-test bar over the full three months that is a
   **28 GB** replay on today's implementation. Measured since **[rig experiment, 2026-08-12]**:
   `GOMEMLIMIT` takes about a third off that peak for no measurable wall clock and is worth
   setting on any instance bought, but it does not reach day 90 —
   **the ledger is materialised in memory before it is applied, and streaming it instead takes
   5.6–7.0× off the peak for about 80 lines of one package.**
2. **The status page is the largest single egress term**, and it is bigger than the game
   traffic. It serves ~20 KB every 2 s and ~49 KB every 1.5 s, **uncompressed as measured**, per
   open browser tab. That is ~3.7 GB/day/tab at this rig's rate. Gzip negotiation has since been
   implemented **[implemented 2026-08-12]** and takes effect at the archive's next restart, which
   is what turns ~32 GB/month/tab into ~4 GB.
3. **The Bibites has a free, native Linux build on itch.io at exactly the version this rig
   runs.** That removes Steam, Wine and the Windows licence premium from the spot-simulation
   idea's critical path, and takes a cloud world from roughly $90/month to roughly $30.

---

## How to read the numbers

Five provenance classes, used consistently below.

| Tag | Means |
|---|---|
| **[rig]** | Measured on the living deployment, by this pass, on **2026-08-11 ~22:10Z**. Reproducible; the command is named where it is not obvious. |
| **[rig experiment, date]** | Measured by a run made **for** this document against a **copy** of the deployment's data in a scratch directory — never against the deployment itself, and never writing to it. One so far: the replay-memory matrix of 2026-08-12 in *The term nobody has priced*. |
| **[record]** | Quoted from the project's own record — `dev_environment.md`, `m5_considerations.md`, the contracts. Cited by section. |
| **[web, date]** | Fetched from a vendor page or a search result on the stated date. Vendor prices move; every one of these needs re-checking at the moment of purchase. |
| **[training]** | Carried from model training knowledge, not verified by search this pass. Treat as a lead, not a price. |

**All prices are US-region list prices in USD, before tax, and assume no committed-use discount** —
a 3-month bound is shorter than any 1-year commitment either vendor sells, so commitments are out
of scope by construction.

---

## The measurement base

Everything in Part 1 and Part 2 is derived from the table below. It is worth reading once,
because the model it feeds is what makes a stranger's map costable at all.

### What was measured, 2026-08-11 ~22:10Z **[rig]**

| Quantity | Value | Where it came from |
|---|---|---|
| Map | 6 slots live, 3×2, population 318 | `/api/status` |
| **Achieved speed, summed over the map** | **67.9** simulated seconds per wall second (23.2, 5.2, 8.2, 8.9, 10.8, 11.6) | `achievedTimeScale` per slot |
| Crossing rate | **1,065/min** steady (`totals.perMinute`); a 60 s counter delta caught **1,320** | `/api/status`, and a `migrations` delta |
| Ledger | 7,426,686 records in 2,504,257,342 bytes → **337 B/record** | `ledgerRecords`, `ls -l migrations.jsonl` |
| Records per crossing | **2.14** (MIGRATION + ACK + a GENOME record when one is first served) | 7,426,686 / 3,475,388 |
| **Ledger bytes per crossing** | **703 B** | 60 s file delta of 928,596 B over 1,320 crossings |
| **Wire bytes per crossing** | **~15.8 KB** — the mean `create` record in a sidecar journal, which is the `MIGRATION_PAYLOAD` body plus its envelope | 24 records sampled from slot 1's journal tail |
| Genome store | ~7.1 GB, **~504,000 objects, ~14.2 KB each** | 3 of 256 shards sampled at 27–28 MB / 1,933–1,999 files |
| Genome store growth | **1.26 GB/day** over the 5.65 days since the store was created (2026-08-06 06:34) | `du` sample × 256, against directory age |
| `genomeGaps` | **154,999** — up from ~117,000 on 08-11 and ~46,000 on 08-10 | `/api/status` |
| `metrics.jsonl` | 53,986,300 B / 5.65 days = **9.6 MB/day** at six slots = **1.6 MB/day/slot** | `ls -l`, directory age |
| **Relay** | RSS **93 MB** (high-water 93 MB), 765 s CPU in 17,128 s wall = **0.045 cores** | `/proc/<pid>/status`, `ps -o etimes,times` |
| **Archive** | RSS **2.26 GB settled**, high-water **8.24 GB**, 5,214 s CPU in 28,960 s wall = **0.18 cores** | same |
| `/api/status` | **19,705 B**, gzip 2,254 (**8.7×**) | `curl`, `gzip -9` |
| `/api/hops` | **49,307 B**, gzip 6,562 (**7.5×**) | same |
| `/api/species` | **7,323 B**, gzip 1,451 (5.0×) | same |
| Poll cadence | status **2,000 ms**, hops **1,500 ms**, history 60,000 ms | `go/internal/archive/page.go:2749–2755` |
| Compression on the wire | **none as measured** — a request with `Accept-Encoding: gzip` returned the identical byte count. Answered since: gzip negotiation landed 2026-08-12 and takes effect at the archive's next restart (see *the status page*, below) | `curl -H 'Accept-Encoding: gzip'` |
| Binaries | `relay` 10.5 MB, `archive` 10.6 MB, **statically linked ELF, no cgo** | `file bin/relay bin/archive` |

Two of these are worth pausing on. The relay is a **single static binary with one non-stdlib
dependency and no cgo** **[record: *Versions*]**, which means it cross-compiles to `arm64`
without ceremony — ARM instances are cheaper on both clouds and cost nothing to adopt here. And
the archive's **high-water mark is 3.6× its resident set** — 8.24 GB against 2.26 GB, on a process
that has been up eight hours and is using 18% of one core. That entire gap is the startup replay,
and it is the finding this document is built around.

### The scale-down to real player speeds

Define **S** = the sum of achieved time scales across every peer on the map. This rig runs
**S = 67.9** across six worlds — an average of **×11.3 per world**. A public map of strangers runs
at whatever each player chooses; the game's own default is ×1, so six real players at default speed
is **S = 6**, and this rig is running the map **eleven times faster than that**.

**S is the right variable** because every durable term in the system is produced by *simulated*
time, not by wall clock: organisms cross when they walk into a portal, genomes appear when
evolution produces them. Two terms are exceptions and are handled separately — `metrics.jsonl`
samples on a wall-clock timer, and the status page is served on a wall-clock poll.

Rates per unit of S, derived from the table above:

| Term | Rate | Derivation |
|---|---|---|
| Crossings | **22,600 / day per unit S** | 1,065.2/min ÷ 67.9 × 1,440 |
| Ledger records | **48,350 / day per unit S** | × 2.14 records per crossing |
| Ledger bytes | **15.9 MB/day per unit S** | × 703 B |
| **Relay egress from forwards** | **357 MB/day per unit S** | × 15.8 KB |
| Genome store, *demand* | **~40 MB/day per unit S** | see below |
| Genome store, *observed supply* | ~18.6 MB/day per unit S | 1.26 GB/day ÷ 67.9 |
| `metrics.jsonl` | 1.6 MB/day **per slot** — speed-independent | measured |

**The model reproduces the project's own recorded figure, which is the check that matters.**
`dev_environment.md`, *The disk budget*, measured the deployment on 2026-08-08 at 58 MB/h of
genomes, 19 MB/h of ledger and 0.7 MB/h of metrics — 1.86 GB/day excluding the collector, at
~540 crossings/min **[record]**. 540 crossings/min corresponds to **S ≈ 34.4** in this model, and
the model at S = 34.4 gives 1.94 GB/day. Within 5%.

**But the genome term needs its two halves stated separately, and this is a Risk 5 and Risk 7
finding rather than an accounting nicety.** At the 08-08 measurement the pump was keeping up, and
genomes grew at 1.39 GB/day at S ≈ 34.4 — **40 MB/day per unit S**. Today S is twice as high and
the store grows *more slowly per unit S*, at 18.6. The difference has not gone anywhere: it is in
`genomeGaps`, which has gone **46,000 → 117,000 → 155,000 in two days with no drain window**
**[record: *Standing watch items*]**. The archive is in arrears on genome capture, and the store's
observed growth is therefore the **supply** rate, not the demand rate.

So a hoster sizing *keep everything* must size against **demand — 40 MB/day per unit S** — because
the arrears are a debt the archive pays the moment it gets a quiet hour. The part of the debt that
is never paid is Risk 7's: a stranger who leaves takes their unfetched genomes with them
permanently. **Sizing off observed growth would be sizing off a queue's length rather than its
arrival rate.**

**Total durable growth, the sizing basis:**

> **56 MB/day × S  +  1.6 MB/day × (number of slots)**

### What that gives over D24's three months

Ninety days, six-peer-and-up maps, `keep everything`:

| The map | S | Durable GB/day | **90 days** | Ledger records at day 90 |
|---|---|---|---|---|
| 5 peers @ ×1 — the exit-test bar plus the owner | 5 | 0.29 | **26 GB** | 21.8 M |
| 8 peers @ ×1 | 8 | 0.46 | **41 GB** | 34.8 M |
| 12 peers @ ×1 | 12 | 0.69 | **62 GB** | 52.2 M |
| 12 peers @ ×5 — players who use the speed slider | 60 | 3.37 | **303 GB** | 261 M |
| This rig's own regime, for scale | 67.9 | 3.81 | 343 GB | 295 M |

Add, on top of every row: **1.2 GB** of rotated logs (`--log-rotate-mb 100 × --log-keep 6`, two
processes) **[record: *The disk budget*]**, plus whatever the OS image costs — 3–5 GB is typical.
`ring.json` and `peers.json` are kilobytes.

**The row that should worry the owner is the fourth.** The archive's disk is linear in a variable
the operator explicitly cannot set: **D23 deferred the control surface to M6, so the operator can
watch a stranger's world run at seven times everyone else's speed and can do nothing about it**
**[record: Decision 2, Risk 9]**. Risk 9's accepted cost was described as a measurement-quality
problem. It is also a **billing** problem, and this is the first document to say so. A single
enthusiastic participant running at ×20 adds as much to the archive as twenty participants running
at ×1.

### The term nobody has priced: the archive's memory

This is the finding that changes the recommendation.

| | Measured | Per ledger record |
|---|---|---|
| Archive resident, 599 k records, 2026-08-07 **[record]** | 170 MB | 0.28 KB |
| Archive resident, 7.43 M records, 2026-08-11 **[rig]** | 2.26 GB | **0.30 KB** |
| Replay high-water, 3.70 M records, 2026-08-10 **[record]** | 5.2 GB | 1.41 KB |
| Replay high-water, ~6.4 M records, 2026-08-11 **[rig]** | 8.24 GB | **1.30 KB** |
| Replay high-water, 8.16 M records, 2026-08-12 **[rig experiment, 2026-08-12]** | 8.40–10.49 GB | **1.03–1.29 KB** |

Two independent pairs, twelve-fold apart in scale, both linear, **and the fourth row has since been
reproduced directly**: three replays of a copy of today's ledger peaked between 1.03 and 1.29 KB per
record, with the model's 1.30 at the top of the range. The model stands:
**0.30 KB resident per record, 1.30 KB peak during replay** — and *sizing must use the top of that
range*, because the spread is not noise in the measurement, it is the collector choosing a different
heap goal on each run (see the matrix below).

Applied to the sizing table, at the exit-test bar (S = 5, 242,000 records/day):

| Milestone | Records | Day of the run | What it means |
|---|---|---|---|
| Resident crosses 2 GB | 6.7 M | **day 28** | a 2 GB instance stops holding the archive |
| Replay peak crosses 8 GB | 6.2 M | **day 26** | an 8 GB instance stops being able to *restart* the archive |
| Resident crosses 4 GB | 13.3 M | day 55 | |
| Resident crosses 8 GB | 26.7 M | day 110 | just past the announced ending |
| Replay at day 90 | 21.8 M | — | **6.5 GB resident, 28 GB peak, ~9 minutes** |

Replay time comes from the documented rate of **~40,000 records/s on this host** **[record]**, and
`dev_environment.md` is emphatic that every recorded replay figure expires the day after it is
written — size it from `wc -l` on the day, never from a quotation. On a smaller cloud vCPU it will
be slower than 40,000/s, not faster. The rate has since been measured again on 8.16 M records:
**36,900–48,600 records/s**, the low end being a run whose genome-store metadata was still cold.
A fresh cloud instance is always the cold case.

**One denominator warning before any of this is re-derived.** `/api/status`'s `ledgerRecords` is
**not** `wc -l` on the ledger: `RecordGenome` lines are appended without incrementing the counter
(`go/internal/archive/archive.go:1219` against the three `recordCount++` sites), so the counter
drifts below the file from every restart onward — 8,060,891 against 8,156,869 lines on 2026-08-12,
1.2% low **[rig experiment, 2026-08-12]**. Every per-record figure here is denominated in **lines
actually replayed**, which is what the memory is spent on.

#### The measured answer: `GOMEMLIMIT` works, and it is not the fix

**[rig experiment, 2026-08-12.]** A copy of the deployment's `migrations.jsonl` — 2,726,784,283
bytes, **8,156,868 records replayed** — taken at 08:13Z into a scratch directory and replayed there
by an archive built from `HEAD`, with the genome store read through a symlink, no relay to dial and
the status page on a scratch port. `nice -n 19`, one run at a time, page cache warm unless noted.
The deployment was read and never written; it kept running throughout.

| Setting | Runs | Replay wall | Peak RSS (`VmHWM`) | Peak per record | CPU spent replaying | RSS 3 min later |
|---|---|---|---|---|---|---|
| **no `GOMEMLIMIT`** — what the deployment does today | 3 | 168–221 s | **8.40–10.49 GB** | 1,030–1,286 B | 196–256 s (**1.16 cores**) | 5.3–6.4 GB |
| `GOMEMLIMIT=5GiB` — about 2× the retained set | 3 | 173–200 s | **5.88–6.30 GB** | 721–772 B | 255–300 s (1.4–1.5 cores) | 4.4–4.6 GB |
| `GOMEMLIMIT=4GiB` — about 1.5× | 1 | 283 s | 5.80 GB | 711 B | **1,408 s (4.97 cores)** | 2.3 GB |
| `GOMEMLIMIT=3GiB` — aggressive | 1 | **403 s** | 4.90 GB | 600 B | **2,534 s (6.28 cores)** | 2.1 GB |
| **a streaming replay**, no `GOMEMLIMIT` | 2 | 189–201 s | **1.50 GB** | **184 B** | 258–271 s (1.35 cores) | 1.3–1.4 GB |

**Six things follow, in the order they matter.**

1. **`GOMEMLIMIT` is real and it should be set.** At 5 GiB it turns a peak that wandered between
   **8.40 and 10.49 GB** across three runs of the identical file into one that sat between **5.88
   and 6.30 GB** — a 25–44% cut depending on which pair is compared, and, more usefully, a
   *predictable* number, since an unpredictable peak has to be sized against its worst case anyway.
   It cost **no measurable wall clock** (−9% to +14% across matched pairs, inside this host's
   run-to-run noise) and **17–39% more CPU**. That is a good trade on any machine and it is one line
   in a service unit.
2. **It does not reach day 90.** 772 B/record × 21.8 M records is **16.8 GB**. The $44 bundle has 8.
3. **Below ~2× the retained set the trade turns ruinous, in exactly the resource a cheap instance
   has least of.** 4 GiB bought a 1.4% better peak for **7× the CPU**; 3 GiB bought 17% for **13×**.
   Those runs demanded 5–6.3 cores, which a 16-core host absorbs in parallel and a **2-vCPU bundle
   cannot**: 1,408 CPU-seconds is **≥ 12 minutes** of wall clock on two vCPUs and 2,534 is **≥ 21**,
   against about three today. The archive serves nothing for every second of it. At 5 GiB the CPU
   still fits inside two vCPUs (1.5 cores), so the winner is *the loosest limit that helps*, not the
   tightest one that fits.
4. **There is a floor, and no collector setting goes under it.** The most aggressive run that
   completed still peaked at 4.90 GB — **600 B/record, 3.6× the state the replay retains** — because
   `ReadLedger` (`go/internal/archive/store.go:339`) materialises the **entire ledger as one
   `[]Record`** and `New` walks the slice afterwards. The whole file is live at once by
   construction, so there is nothing for a collector to reclaim.
5. **The floor is a shape, not a size, and the shape is cheap to change.** A prototype that streams
   the replay — `ScanLedger(dir, func(Record))`, with `New`'s existing loop body as the callback —
   peaked at **1.50 GB, 184 B/record — 5.6× lower than the baseline run of the identical build, and
   7.0× lower than the worst baseline seen — and about 1.1× the state it retains**, with
   no `GOMEMLIMIT` at all. Both streaming runs peaked within 0.3% of each other, against the
   baseline's 25% spread: a small heap is a predictable one. It replayed the same 8,156,868 records
   through the same code that applies them, left the same aggregates, and
   passed the archive package's tests unchanged; it is about 80 lines across two functions in one
   package and changes no contract, no wire type and no file format. **This, not `GOMEMLIMIT`, is
   the answer to the $96 question.** *(Measured on a patched build in a scratch tree. It is not in
   `bin/`, not committed, and nobody has yet written a test that pins the streaming path itself.)*
6. **Swap is not an alternative, and the peak is not a spike.** RSS stayed within 0.1% of its
   high-water mark for **150 seconds after the replay ended** before Go's scavenger returned
   anything. A box that meets its peak in swap is thrashing a live heap that the collector is
   walking, for minutes.

**What each answer buys**, at the exit-test bar (S = 5, 242,000 records/day, from an empty ledger on
day one). The day is when the *replay* stops fitting; resident is the separate line above:

| Replay implementation | Peak/record | Day-90 peak | 2 GB | 4 GB | 8 GB |
|---|---|---|---|---|---|
| today's, no `GOMEMLIMIT` | 1,286 B | **28.0 GB** | day 6 | day 13 | **day 26** |
| today's, `GOMEMLIMIT=5GiB` | 772 B | 16.8 GB | day 11 | day 21 | **day 43** |
| today's, `GOMEMLIMIT` at the floor, at 5–6 cores | 600 B | 13.1 GB | day 14 | day 28 | day 55 |
| **streaming replay** | 184 B | **4.0 GB** | day 45 | day 90 | **day 180** |

**Read the last row against the resident line and the whole shape of the problem changes.** With a
streaming replay the replay stops being the binding constraint at all: resident crosses 8 GB at day
110 and the replay would not cross it until day 180, so **the $44 bundle becomes restartable for the
entire announced period** — and it is the *retention rule*, not the collector, that decides
everything again. Note also that the streaming build's retained set right after replay was
**0.16 KB/record**, half the 0.30 KB the deployment shows: part of what the running archive holds is
its own replay, which it never fully gives back. That halving is **not** claimed as a resident
model — no streamed archive has yet served live traffic for a day — so keep sizing resident from the
deployment's own 0.30 KB/record until one has.

**Three things still follow, and the third is still the biggest.**

- **Steady state is not the constraint.** 0.045 cores for the relay and 0.18 for the archive, at
  eleven times a real map's rate, means the *running* map fits on almost anything.
- **~77% of the replay peak is garbage, not state** — the measurement that says so is above. Go's
  collector is not being asked to run during a tight replay loop, and when it is asked, it collects
  what it can and then hits the ledger-sized slice it cannot.
- **Decision 3 is the instance-sizing decision.** Retained memory grows forever at 0.30 KB/record
  because nothing may evict from the ledger (`contract-b-m4.md` §10). A retention rule that bounds
  the ledger — any of the three options Decision 3 names — puts the whole service on a 2 GB box.
  *Keep everything* puts it on an 8 GB box that **cannot restart in month three on today's
  implementation, and `GOMEMLIMIT` alone does not change that** — it moves the wall from day 26 to
  day 43. **The gap between those two is about $96 over the period, and it is the difference between
  a service that restarts and one that does not** — but the measurement has now added a third
  option that costs neither: fix the replay's shape and the $44 bundle carries *keep everything* for
  the whole run.

### The other term nobody has priced: the status page

`/api/status` is 19.7 KB and refreshes every 2 s; `/api/hops` is 49.3 KB and refreshes every 1.5 s;
**neither was compressed when this was measured** **[rig]** — see *Compression: done*, below. Per
continuously open browser tab, at this rig's rate:

> 19.7 KB / 2 s + 49.3 KB / 1.5 s ≈ **43 KB/s = 3.7 GB/day = ~111 GB/month, per tab.**

`/api/hops` scales down with the crossing rate, so at the exit-test bar it shrinks to ~3.6 KB, and
`/api/status` scales with slot count (~3.3 KB/slot), so it does not shrink at all. Per tab at
S = 5, six slots:

> **~1.06 GB/day = ~32 GB/month, per continuously open tab.**

That is **more than half the entire migration traffic of the map** (54 GB/month at S = 5), produced
by one person leaving a browser tab open. Five curious strangers doing the same triples the
service's egress, and none of it is game traffic.

**Two cheap answers. The first has been taken; the second is still the page owner's.**

**Compression: done** **[implemented 2026-08-12]**. `go/internal/archive/compress.go` wraps the
whole operator mux in standard `Accept-Encoding` negotiation — the five JSON endpoints and the page
itself. A client that asks for gzip gets it; a client that says nothing gets the identical bytes it
always got; `Vary: Accept-Encoding` is on both, nothing is encoded twice, and an answer under one
packet is left alone. On a six-slot fixture the status frame compresses **10.2×**, which is the
same shape as the **8.7×** measured here with `gzip -9`, and the page's own **138 KB** first load
falls to **45 KB** — a term this section had not counted at all. It is transport and nothing else:
no payload changed shape, no interval moved, and no existing reader had to change. `curl` sends no
`Accept-Encoding`, so every shell probe in `e2e/` keeps parsing the identity bytes it always did;
`ringstat` is Go, and `net/http` asks and decompresses on its own behalf, so the terminal tool takes
the saving for free. **It is not on the wire yet** — it rides the archive's next restart and joins
the debt `m5_tracking.md` already holds against that restart (WP4's deny-list flag and the `limits`
key on `/api/status`). An archive restart is expensive and its cost grows with the ledger, so the
rule there stands: batch the reasons, never restart to collect one.

**Cadence: still open, and still not this document's.** Lengthening the two poll intervals would
cut what is left proportionally and would change what the page feels like. **The measurement is
offered; that change belongs to whoever owns `page.go`.**

Note also that this is the same page `m5_tracking.md` records as **never having been reachable by
anyone but the owner** — the `8796` firewall rule and portproxy have no record of being run. WP3
is where it becomes public for the first time, and the first time it is public is also the first
time its egress is real.

---

## Part 1 — the core service: relay + archive

### What has to run

Two static binaries and their durable files. `m5_considerations.md`'s DQ2 names the six operational
things that must exist around them — a supervisor, monitoring that speaks, backup of `ring.json`
and the archive's three durable files, a written restart policy, a name with a renewable
certificate, and D24's announced period with its wind-down. **None of those change between the
options below**, so they are not re-costed per option; they are labour, and Risk 10 says the labour
is one person's.

Resource profile, from **[rig]** and scaled by the model:

| | Relay | Archive |
|---|---|---|
| Binary | 10.5 MB static ELF, no cgo | 10.6 MB static ELF, no cgo |
| CPU at this rig's rate (S = 67.9) | 0.045 cores | 0.18 cores |
| CPU at the exit-test bar (S = 5) | ~0.003 cores | ~0.013 cores |
| Resident | 93 MB, flat | **0.30 KB × ledger records** — 6.5 GB at day 90, S = 5 |
| Peak | 93 MB | **1.30 KB × ledger records** during replay |
| Disk | `ring.json` + `peers.json`, kilobytes | the sizing table above |
| I/O | negligible | append-only, sequential; the replay reads the whole ledger |

**Co-host them.** The archive is an authorised Contract B subscriber (B27) and receives a copy of
every `MIGRATION_PAYLOAD` — 15.8 KB per crossing. Over loopback that is free; across a network it
is the single largest flow in the system. Every option below assumes one instance running both.

### Disk, sized from the arithmetic

From the sizing table, plus 1.2 GB of logs and ~5 GB of OS:

| The run you are provisioning for | Disk to buy |
|---|---|
| The exit test and a quiet 3 months (5 peers @ ×1) | **32 GB** |
| A modestly successful run (12 peers @ ×1) | **70 GB** |
| A successful run where people use the speed slider (12 peers @ ×5) | **310 GB** |

**The honest statement to the hoster** — which is what Decision 3 and Risk 5 actually ask for — is
not a number but a rule: *the archive grows at 56 MB per day per unit of summed map speed, plus
1.6 MB per day per slot; free space is a first-class monitored signal; and the 2026-08-08 ENOSPC
outage is what running out looks like.* Both clouds can grow a volume online, which converts the
sizing error from an outage into a bill — that is worth more than getting the number right.

### Egress, against the actual traffic shape

Monthly, at the exit-test bar (S = 5, six slots):

| Flow | Monthly | Notes |
|---|---|---|
| Migration forwards out of the relay | **54 GB** | 357 MB/day × S; ingress is free on both clouds |
| Status page | **32 GB per continuously open tab** | uncompressed; **~4 GB gzipped**, which is what it becomes at the archive's next restart **[implemented 2026-08-12]** |
| Genome fetches served through the relay | **up to 92 GB** | ceiling is `genomeRequestsPerMinute` 30/peer × 14.2 KB; observed far below |
| `FORWARD_RECEIPT`, one per forward | **0.97 GB** | **287 B measured**, × 22,600 crossings/day × S. B26's whole egress cost, and **1.8% of the forward term it rides beside** **[measured 2026-08-12]** |
| `PEER_STATUS` broadcasts | negligible | 43.9/min after WP5's coalescing window **[record]**, small frames |
| ACKs, `PONG`s, save receipts | negligible | a few hundred bytes each |
| **Realistic budget** | **90–150 GB/month** | tail risk to ~250 GB if the genome pump ever drains; the receipt row sits inside this band and does not move it |

**This is small by cloud standards and awkward by cloud pricing.** It sits just above AWS's 100 GB
free allowance and far above GCP Premium Tier's 1 GiB, which is exactly the band where the pricing
model matters more than the volume.

**The receipt row is a measurement, not an estimate, and that was WP3's job.** B26 declined to
assume its own cost away and handed the arithmetic here (`contract-b-m4.md` §5.2; DQ2). The
harness — `go/internal/relay/receipt_cost_test.go`, 2 000 migrations through an in-process relay
at 15.8 KB a crossing — measures **287 B per receipt**, **relay-written frames per migration 2 → 3**
(forward + ack + receipt) with relay-*read* frames unchanged at 2, and **1.3% marginal CPU** on the
forward path, which is dominated by the JSON decode of the 15.8 KB frame it acknowledges. Three
properties keep the row this small and all three are contract, not luck: the frame is four fields
with no body, it is **not copied to subscribers** (§5.1's fan-out set is unchanged), and it enters
**no** §3.3 ceiling, because every published limit counts the relay's *inbound* path. The term the
egress table cannot show is the **sender's**: one journal `Apply` — an appended ~221 B record and an
fsync — per forwarded migration, taking the outbound path from 3 durable writes to 4. That is disk
and latency on a participant's own machine, not egress on the hoster's bill.

### TLS, DNS, and what the name costs to change later

**What the certificate binds to.** B23 puts TLS at the relay's own front door, with a
rotation-surviving reload already implemented and tested in WP2 **[record: WP2 row]** — so ACME
renewal does not require a restart, which is the one thing DQ2 asked for. A certificate binds
**names, not ports**, so one certificate covers both the relay's `wss://` listener and the status
page.

**What changing the name costs.** The join string is minted once per peer and is literally
`multiverse-join/1 wss://<host>/contract-b/v4 <peerId>.<secret>` — the relay's advertised URL is
baked into it (`go/internal/peercred/peercred.go:485`, `relay.go:63`). Every participant's sidecar
holds that URL in its configuration. **D25 chose a channel that pushes nothing**, so a host name
change after strangers have joined is a message the owner has to deliver to people he may not be
able to reach, followed by a re-mint or a B28 handover per peer. **Treat the name as permanent for
the run.**

That single fact settles the three options:

| Option | 3-month cost | What it gives, and what it costs |
|---|---|---|
| **The cloud's own hostname** | $0 | **Not viable.** Let's Encrypt refuses to issue for `*.compute.amazonaws.com` by policy **[web, 2026-08-11]**, and GCE assigns no public DNS name at all. It also ties the name to one instance, which is the exact thing you must not do. |
| **Free dynamic DNS** (DuckDNS, no-ip) | **$0** | Owner-controlled A record, so the name survives a host move — which is the property that matters. DNS-01 issuance works. The costs are a third party's continued existence and terms over the announced period, and a name that reads as provisional in a join string handed to a stranger. |
| **A registered domain** | **~$5** | ~$10–15/year at a registrar **[training]** amortised over 3 months, plus a zone: Route 53 **$0.50/zone/month**, Cloud DNS **$0.20/zone/month**, both plus $0.40/million queries **[web, 2026-08-11]**; Cloudflare's free tier is $0. Same host-independence, no third-party name in the join string, and it outlives the run — which matters if D24 is ever extended. |

**A domain is the cheapest insurance in this document.** Five dollars decouples the string every
participant holds from the machine it points at, and every migration path in Part 3 depends on that
decoupling.

One design note WP3 will have to take, flagged rather than answered: the relay currently listens on
its own port with its own TLS. Participants behind restrictive networks will have an easier time on
**443**. Putting both the relay and the status page on 443 needs a reverse proxy, which either
duplicates or terminates the TLS B23 gave the relay. The cheap version is to move the relay to 443
and give the status page its own port or its own name; the expensive version is a proxy. **This is
a WP3 call, and it should be made before the first join string is minted, because the port is in
the join string too.**

### The options, costed over D24's three months

Common assumptions: US region, Linux, relay and archive co-hosted, ~120 GB/month egress, disk sized
for a modest run.

| # | Option | Compute (3 mo) | Disk (3 mo) | IPv4 (3 mo) | Egress (3 mo) | **3-month total** | RAM verdict |
|---|---|---|---|---|---|---|---|
| 1 | **Lightsail $12** — 2 vCPU, 2 GB, 60 GB SSD, 3 TB transfer | $36.00 | incl. | incl. | incl. | **$36.00** — or **$0** on the 90-day trial | 2 GB: resident crosses it at **day 28**, but the **replay** crosses it at **day 6** — day 11 with `GOMEMLIMIT`, day 45 streamed |
| 2 | **Lightsail $24** — 2 vCPU, 4 GB, 80 GB SSD, 4 TB | $72.00 | incl. | incl. | incl. | **$72.00** | 4 GB: resident at **day 55**, replay at **day 13** — day 21 with `GOMEMLIMIT`, day 90 streamed |
| 3 | **Lightsail $44** — 2 vCPU, 8 GB, 160 GB SSD, 5 TB | $132.00 | incl. | incl. | incl. | **$132.00** | 8 GB: survives 90 days resident; **cannot replay after ~day 26**. `GOMEMLIMIT` moves that to **day 43**, not to 90; a streaming replay moves it to **day 180** **[rig experiment, 2026-08-12]** |
| 4 | **EC2 t4g.small** (ARM) + 60 GB gp3 | $36.79 | $14.40 | $10.95 | ~$5 | **~$67** | as row 1 |
| 5 | **EC2 t4g.medium** (ARM, 4 GB) + 80 GB gp3 | $73.59 | $19.20 | $10.95 | ~$5 | **~$109** | as row 2 |
| 6 | **GCE e2-micro, Always Free** + 30 GB standard PD | **$0** | **$0** | ~$11 | $0 on Standard Tier | **~$11** | 1 GB. **Relay only — it cannot host the archive at any interesting ledger size** |
| 7 | **GCE e2-small** (2 GB) + 60 GB pd-balanced | $36.69 | $18.00 | $10.95 | $0 on Standard Tier | **~$66** | as row 1 |
| 8 | **GCE e2-medium** (4 GB) + 80 GB pd-balanced | $121.14 | $24.00 | $10.95 | $0 on Standard Tier | **~$156** | as row 2 |

Price provenance, all **[web, 2026-08-11]** unless marked:

- Lightsail bundles and their included transfer, from `aws.amazon.com/lightsail/pricing`: $5/0.5 GB/20 GB/1 TB, $7/1 GB/40 GB/2 TB, $12/2 GB/60 GB/3 TB, $24/4 GB/80 GB/4 TB, $44/8 GB/160 GB/5 TB. Static IP included; block storage $0.10/GB/month; overage from $0.09/GB. IPv6-only bundles are $1.50–$4/month cheaper.
- **Lightsail's 90-day free trial**: new accounts get 3 months free on the $5/$7/$12 Linux plans (and $9.50/$14/$22 Windows), **one bundle per account**, for accounts that started using Lightsail on or after 2021-07-08.
- EC2 `t4g.small` $0.0168/hr in us-east-1 → $12.26/month; `t4g.medium` is twice that **[training, consistent with the t4g.small quote]**. EBS gp3 $0.08/GB-month. Public IPv4 $0.005/hr = $3.65/month since 2024-02-01. Data transfer out: first **100 GB/month free** account-wide, then $0.09/GB to 10 TB.
- GCE `e2-small` $12.23/month, `e2-medium` ~$40.38/month, `e2-standard-2` $48.92/month in us-central1. pd-balanced ~$0.10/GiB-month. External IP on a standard VM $0.005/hr; **on a Spot VM $0.0025/hr**.
- **GCP Always Free**, from `cloud.google.com/free/docs/free-cloud-features`: one non-preemptible **e2-micro** per month in `us-west1`, `us-central1` or `us-east1`; **30 GB-months standard persistent disk**; **1 GB of egress from North America**. The page's Compute Engine list contains **no external IP address**, so budget ~$3.65/month for one — verify on the billing console at signup, because this is the single most commonly surprising line on a "free" GCP VM.
- GCP egress: **Premium Tier** $0.12/GiB after **1 GiB** free; **Standard Tier** $0.085/GiB after **200 GiB** free. **Choosing Standard Tier is worth ~$14/month at this traffic level** and is a per-resource setting, not a discount you apply later.

**What the table says.**

- **Lightsail wins at every RAM tier that matters**, and it wins on a dimension that is not money:
  the bundled transfer removes egress from the operator's attention entirely. For a one-person
  operator carrying Risk 10, "3 TB included" is worth more than the $30 it saves.
- **GCP's free e2-micro is real, and it is the wrong shape.** 1 GB of RAM cannot replay a ledger
  past ~750,000 records — about **three days** at the exit-test bar. It *can* host the relay
  indefinitely: 93 MB resident and 0.045 cores at eleven times a real map's rate. Splitting relay
  and archive across two VMs in the same zone keeps their traffic free (intra-zone internal-IP
  traffic is not charged), but the archive's VM then dominates the bill anyway, so the split buys
  nothing except a free relay. **It is worth keeping in mind for one thing: a permanently free
  relay is a good place to put a map that outlives D24's period**, which is exactly the
  publish-the-relay path Decision 4 asked to have prepared.
- **GCE is 2–3× Lightsail for the same RAM** and offers nothing here in exchange.
- **The AWS free-tier interaction is the biggest single number on the page**: the Lightsail trial is
  90 days, D24's announced period is 3 months, and the $12 bundle is on the trial list. If the
  retention rule bounds the ledger, **the announced run can cost $0 in compute.** That is a
  coincidence worth exploiting and worth *not* depending on — verify eligibility before announcing
  anything, and note that only **one** bundle per account is covered, so a second instance for a
  cloud world (Part 2) is not free.

### The single recommendation, and the condition that has now been discharged

> **Amazon Lightsail, US East, one instance running relay and archive together, on a domain the
> owner registers. Which bundle depends on Decision 3, and that is the point.**

- **If the retention rule bounds the ledger** — any of Decision 3's three options other than *keep
  everything* — take the **$12 bundle**: 2 GB, 60 GB, 3 TB. **$36 for the period, or $0 under the
  90-day trial**, plus ~$5 for the domain. The archive stays small, restarts in seconds, and the
  whole service fits in the smallest thing either vendor sells.
- **If the rule is *keep everything*** — which Decision 3 explicitly allows — take the **$44
  bundle**: 8 GB, 160 GB, 5 TB, **$132 for the period**. And understand what is being bought: 8 GB
  holds the archive *resident* through day 110, but on today's implementation it stops being able
  to *replay* at around day 26 — **day 43 with `GOMEMLIMIT`, which is still inside the announced
  period.** Buying this bundle without changing the replay buys a machine that holds the archive and
  cannot restart it in month three.

**The condition was a gate, and the gate has been run** — the replay-memory matrix above,
**[rig experiment, 2026-08-12]**, against a copy of the ledger and never the deployment. Its answer,
in the order the money depends on it:

- **Set `GOMEMLIMIT` on whatever is bought.** `GOMEMLIMIT=5GiB` on an 8 GB instance, or ~75% of RAM
  on any other, in the service unit beside the binary. It took **a third off the replay peak for no
  measurable wall clock**, and it converts an out-of-memory kill into a slow start, which is the
  trade an unattended box should always take. **Do not tighten it past ~2× the retained set**: at
  1.5× the same run cost 7× the CPU, and on two vCPUs that is the difference between a three-minute
  restart and a twelve-minute one.
- **`GOMEMLIMIT` does not make *keep everything* survive its own restart policy**, so it does not
  by itself justify the extra $96. It moves the wall from day 26 to day 43.
- **The thing that does is a change to the archive, not to the invoice.** The replay materialises
  the whole ledger in memory before applying any of it; streaming it instead measured a
  **5.6–7.0× lower peak**, and puts the $44 bundle past day 180 and the $12 bundle at day 45. It
  is ~80 lines in one package with no contract implication, and it is now the cheapest item on this
  page. **Land it before day one and the bundle question goes back to being about disk and
  traffic.**

WP3's done-when already requires a written restart policy. It can now be written from measurements
instead of from a model: **restart cost is ~3 minutes and ~10 GB today, ~3 minutes and ~6 GB with
`GOMEMLIMIT`, and it grows linearly with the ledger** — size it from `wc -l` on the day, never from
this page.

**Why not GCP.** Nothing disqualifies it, and if the owner has existing GCP familiarity that is a
real cost saving this document cannot price. But at every RAM tier it is more expensive, its egress
needs a tier choice made correctly at creation time, its free instance is the wrong shape for the
archive, and its Spot preemption notice is 30 s against AWS's 120 s — which matters in Part 2. **The
one thing GCP has that AWS does not is a genuinely free, genuinely permanent small instance**, and
the place for that is a relay that outlives the announced period, not the service D24 is about.

---

## Part 2 — the spot-simulation idea, taken seriously

The owner's sketch: cloud worlds on spot/preemptible instances that come and go, with automatic
handoff, over cheaper permanent storage. **The half of this that sounds hard is already built. The
half that sounds easy is the one that needs a rehearsal.**

### Why the churn half is already built, and proven

A world in this system is three things: **a journal, a slot reservation, and a credential.** None of
them lives on the instance's ephemeral state, and all three survive the machine going away.

| What spot does to you | What M5 already built for it | Evidence |
|---|---|---|
| The machine vanishes with two minutes' notice | Leave is a normal event. Lanes re-pair as bypasses, neighbours route around a dark peer, and the map keeps running | WP5 rolled out 2026-08-11; the rig runs 18/20 lanes with slot 6 dark, by design **[record]** |
| The peer comes back on a different machine | The slot reservation is durable relay-side and the address is never reused (D8, D12). A returning peer reclaims with `reason=reclaimed` at **its own coordinate** | Five rolling sidecar restarts on 2026-08-11, every one `reason=reclaimed`, **zero discarded journal bytes** on all five **[record: WP4+WP5 rollout]** |
| Organisms in flight when it dies | Custody is durable before ACK (D2). Orphaned outbound entries are held and bounce home at the bounded 24-hour timeout | `contract-a.md` D2; `contract-b-m4.md` §9 |
| It might never come back | "Never return" is a tested case, not an edge case | WP5's churn harness: **50 M events in 5 m 42 s, seven invariants clean, no address re-issued** **[record]** |
| It comes back to a backlog | The sidecar replays its journal; WP4's limits and the outbound pacing fix bound the rejoin burst | the rejoin-burst finding and its dispatched fix **[record: *Standing watch items*]** |

**So spot interruption is not a new failure mode. It is the churn mode the milestone was designed
around, arriving on a schedule instead of by accident.** That is a genuinely strong position and it
should be said plainly: *if a cloud world dies and nothing on the map breaks, that is not luck, it
is WP5 working.*

### The hard part is the game — and one search changed the shape of it

`dev_environment.md`'s *Versions* table describes a **Windows/Steam** title: The Bibites, Steam app
2736860, game version **0.6.3.1**, Unity **6000.0.44f1 Mono**, BepInEx **5.4.23.3 win x64**
**[record]**. Every operational fact in this project is about running that on Windows.

**But The Bibites has a native Linux build, on itch.io, free, at version 0.6.3.1 — the exact version
this rig runs** **[web, 2026-08-11, `thebibites.itch.io/the-bibites`]**. The page offers Windows
64/32-bit, macOS Universal and **Linux**; the model is *name your own price*; and the developer
states that the itch.io build is *"the standard and complete version of the game"*, with Steam
adding achievements and Workshop integration rather than content.

That single fact removes Steam, Wine, Proton and the Windows licence premium from the critical path
at once. It does **not** remove the rehearsal — nothing below has been tested — but it changes what
the rehearsal is *of*.

#### (a) Windows spot instances — the cost of not needing them

Costed anyway, because it is the fallback if the Linux path fails.

- **The licence premium is real and it does not scale down.** AWS prices Windows at **$0.046 per
  vCPU-hour** on top of the Linux rate **[web, 2026-08-11]**. GCP charges roughly **$0.04 per
  vCPU-hour**, and Google's own documentation is explicit that **premium image licensing charges
  apply to Spot VMs at the standard rate** — *the Spot discount does not apply to the Windows
  licence* **[web, 2026-08-11]**.
- **On AWS this could not be settled by search.** AWS quotes one Spot price for a Windows instance
  with the licence included, and no authoritative statement was found on whether the discount
  compresses the licence portion. The practical consequence is a range, and it should be resolved
  by reading the actual Spot price history for the chosen instance type before committing.

For a 2 vCPU / 4 GB box (c7i.large, us-east-1, **[web, 2026-08-11]**: Linux on-demand $0.08925/hr,
Linux spot **$0.032/hr**):

| | Hourly | Monthly (730 h) |
|---|---|---|
| Linux, spot | $0.032 | **$23.36** |
| Linux, on-demand | $0.08925 | $65.15 |
| Windows, on-demand | ~$0.181 (Linux + 2 × $0.046) | ~$132 |
| Windows, spot — if the discount covers only compute | ~$0.124 | **~$90** |
| Windows, spot — if the discount covers everything | ~$0.065 | **~$47** |
| GCP `e2-standard-2` Windows spot | $0.0335 + 2 × $0.04 | **~$83** (of which **$58 is licence**) |

**On GCP the Windows licence alone costs more than twice the entire Linux instance.** That is the
number that makes the itch.io build worth an evening of rehearsal.

#### (b) Linux — the native build works, and it has now been run

**The native Linux build removes Wine/Proton from the plan entirely** — rehearsed end to end on
this machine on 2026-08-12. Three reasons said the native path was plausible; all three held:

- **Unity Mono is nearly the same IL on every platform — and "nearly" is now measured, not
  assumed.** The mod is Harmony patches against `BibitesAssembly.dll`, and a Mono build's managed
  assemblies are *almost* platform-independent: the two builds differ only where the game calls a
  native file dialog or asks the shell to reveal a folder (question 1 below). That is the whole
  reason `dev_environment.md` records "Mono backend (not IL2CPP — Harmony and decompilation fully
  work)" as a load-bearing fact **[record]**.
- **BepInEx 5 ships Linux builds.** 5.4.23.x splits its unix zip into `linux_x86`, `linux_x64` and
  `macos_x64`, with Doorstop 4.3 and a `run_bepinex.sh` launcher, and BepInEx's own guidance is to
  stay on 5 for Unity Mono games **[web, 2026-08-11]**.
- **Headless already works here.** `-batchmode -nographics` are Unity's own flags, the rig has run
  all five local worlds that way since 2026-08-10, and `0.6.2`'s `MinFpsGovernor` exists precisely
  to disarm the game's min-FPS servo in a process with no graphics device **[record]**.

**These five were the rehearsal items. All five are now ANSWERED on this machine**
**[rig experiment, 2026-08-12]** — the itch.io Linux build downloaded non-interactively, unpacked
to scratch, run headless against a scratch relay and sidecar on ports 18795/18787, while the live
five-world rig kept running untouched:

1. **Does the Linux `BibitesAssembly.dll` bind the mod's patches? YES — and the hash differs, which
   is D22's support matrix doing exactly its job.** Windows/Steam is
   `12455e48…` (1 376 256 B); Linux/itch.io is `5b145a0a…` (1 375 744 B) — 512 bytes, one PE
   alignment unit. A full `ilspycmd` decompile of both, diffed, puts **every** difference in three
   places the mod never touches: `StandaloneFileBrowserWindows` + `WindowWrapper` (`user32.dll`,
   `System.Windows.Forms`, `Ookii.Dialogs`) become `StandaloneFileBrowserLinux`
   (`DllImport("StandaloneFileBrowser")`); three "reveal the save folder"
   `Process.Start("explorer.exe", …)` calls become no-ops; and Unity's own `MonoScriptData`
   bookkeeping shifts (`TotalTypes` 631 → 630, exactly the one type removed). **All six types the
   mod patches or reads are byte-identical across the two builds** — `TimeController`, `BibiteBody`,
   `GlobalLineageManager`, `ScreenShotHandler`, `SaveSystem`, `SaveController` — and all ten patch
   targets are present on both sides.
2. **Does BepInEx 5.4.23.3 `linux_x64` load the plugin, and does `0.6.4` behave? YES, completely.**
   `BepInEx_linux_x64_5.4.23.3.zip` over the game directory, `BibitesMultiverse.dll` (`fae0d50c…`)
   into `BepInEx/plugins/`, `./run_bepinex.sh "./The Bibites.x86_64" -batchmode -nographics`.
   The log reads `System platform: Bits64, Linux`, `Detected Unity version: v6000.0.44f1`,
   `Loading [Bibites Multiverse 0.6.4]`, and the mod prints `Application.version = 0.6.3.1` — the
   build **is** the version this rig runs. Config was read from the BepInEx `.cfg` and the
   environment; every subsystem armed (border strip on `BibiteBody.FixedUpdate`, `[M5-HISTORY]`,
   `[M4-PHASE]`, dev commands); `MinFpsGovernor` detected `headless=True` and disarmed itself
   unprompted. **The Contract A dial used the shipped token flow, not the rig escape hatch** — the
   sidecar minted `<data-dir>/contract-a.token` 0600 at first start and the mod read it from
   `MULTIVERSE_CONTRACT_A_TOKEN_FILE` — and the handshake carried `gameVersion=0.6.3.1
   modVersion=0.6.4 contractAVersion=contract-a/2.4`. The relay saw `modConnected=true
   exportEdges="[E N W S]"`. **The session ran 14 m 08 s and ended only when told to quit**, which
   is the heartbeat answer: the sidecar's 13-second deadline never fired once.
   **On Linux there is no `WSLENV` and no path translation** — the token path passes through as
   itself, deleting the single fiddliest line in `run-m4.sh`'s `start_game`.
3. **What breaks on `persistentDataPath`? Nothing in the mod; four lines in the rig scripts.** The
   path resolved to `/home/ubuntu/.config/unity3d/The Bibites/The Bibites`, with `Savefiles/`,
   `Scenarios/`, `Bibites/Templates/` and `prefs` all created correctly. **The mod is already
   clean** — every path goes through `SaveController.SavePath` (which is
   `Path.Combine(Application.persistentDataPath, "Savefiles")`) or `Application.persistentDataPath`
   directly, and it logged its own directory correctly at startup. Eight saves landed, rotation and
   pruning worked (`keep=4`, `pruned=1`), and save-on-quit fired (`why=quit`). **What hardcodes the
   Windows path is the rig, not the product**: `e2e/run-m4.sh:355`, `e2e/baseline.sh:55` and `:711`,
   and `e2e/species-guard-check.sh:27-28` all glob
   `/mnt/c/Users/*/AppData/LocalLow/...`. `$XDG_CONFIG_HOME` relocates the whole tree, which is how
   a cloud world puts its saves on the persistent volume of (d).
4. **Does the log-starvation trap exist on Linux? NO — and what replaces it is worse to debug,
   though harmless to a one-instance-per-VM cloud world.** Five concurrent instances out of one
   game directory produced **no `LogOutput.log.1`–`.4` at all**: the Windows fallback is driven by
   an exclusive file lock Linux never takes. **All five loaded the mod** — `BibitesMultiverse.dll`
   is mapped in all five processes — so the *functional* failure mode of the Windows trap (an
   instance that gets no log file never loads the mod, and comes up `modConnected=false` with
   `exportEdges=[]`) **does not exist here**. Instead all five held an open descriptor on the *same*
   `LogOutput.log`, each with its own file offset and each truncating on launch: the file ended as
   **227 402 bytes of which 200 557 were NUL**, `file` reporting `data` rather than text, with only
   one of five session headers and two of five `event=SAVED` lines surviving. **Windows loses one
   instance loudly; Linux keeps every instance and shreds the shared evidence window for all of
   them at once.** A cloud world runs one instance per VM and never meets this. **A Linux
   *participant* running more than one world on one box does**, and it is a WP6 packaging item, not
   a cloud one.
5. **Does the achieved-vs-applied time-scale gap look the same? The gap is real and WIDER here, but
   this host cannot settle the number.** `Time.timeScale` accepted **100.00 on the first send** —
   the Windows rig's rule that the first `timescale` after a load sticks at 1.00 and needs a second
   send twenty seconds later **did not reproduce** (observed once; not yet a rule). Achieved rate
   over six 67-second samples was **6.3–7.2×** against an applied 100, at a population of 14–28.
   **That number is not a cloud number and must not be quoted as one:** it was taken on a WSL2 VM
   whose 16 cores were carrying five live Windows worlds, the relay, the archive and the collector
   at a load average of 14–20, with everything niced to 19. It bounds the gap's *existence*, not its
   size. **A clean single-tenant box is the only place this can be measured, and Stage 2 is that
   box.**

**Sizing, from the same run** — the cloud-world numbers of *Costing one cloud world* are
Windows-measured, and the Linux instance is **cheaper on memory**: the headless game held
**368–401 MB** RSS (rising steadily over 6.5 minutes as population grew 14 → 28) against Windows's
403–466 MB fresh and 1 954–2 450 MB at a ×100 target, and burned **0.86–1.17 cores** against
Windows's 1.04–1.40. Its sidecar held **13–14 MB** — far under the Windows 70–145 MB — at
effectively no CPU on a single-peer map with no lanes open. **2 vCPU / 4 GB remains the honest
minimum and 8 GB the honest recommendation**, because nothing here ran long enough to reach the
steady state that produces the 2 GB figure. Save stalls were **277–682 ms across all eight saves,
every one inside D14's 2 000 ms budget** — against the live rig's 43-of-76 breach rate — but the
worlds are not comparable: `lineageMs` was 4–11 ms here against 53% of `writeMs` there, because this
world had minutes of species history and the rig's have weeks. **Clean quit with save-on-quit took
2.05 s**, comfortably inside even GCP's 30-second preemption notice.

**Proton/Wine is now dead as a path, not merely a fallback.** Questions 1–4 all passed on the native
build, so the translation layer buys nothing and costs a Windows game binary under a process that
must run unattended for weeks. **It should not be revisited unless a future game version breaks the
native build.**

#### (c) The Steam constraint, and the honest licensing position

**The constraint is real on Steam.** A Steam account is licensed for one active session: install on
as many machines as you like, but a second concurrent login invalidates the first, and the
first-logged-in user gets *Invalid Steam UserID Ticket* **[web, 2026-08-11]**. **N cloud copies on
one Steam account is not a configuration problem — it is the licence saying no.**

**Two facts make this moot rather than blocking.**

- **The itch.io build is DRM-free, free (name-your-own-price), and complete**, and it is the same
  version the rig runs **[web, 2026-08-11]**. There is no session ticket to invalidate.
- **This rig is already not using Steam to run the game.** `bibites-mod/game.sh:28` launches
  `The Bibites.exe` directly, and `dev_environment.md` records that "the game's parser reads only
  `-steam`" **[record]** — the Steam build starts and runs headless without the client. That is
  evidence the binary is not DRM-wrapped; it is **not** evidence that running N copies of it is
  licensed, and those are different questions.

**The honest position, stated as a position rather than as a legal opinion:** use the itch.io build
for cloud worlds, **pay the developer for each one** — name-your-own-price makes that a choice
rather than a transaction, which is precisely why choosing to pay matters — and, because this
project is about to ask strangers to install a mod for his game, **ask him.** The itch.io page
points at a Patreon Discord; the developer is reachable, and "we would like to run some headless
instances in the cloud to feed a multiplayer map, is that alright with you" is a better opening than
a licence analysis. **That is the owner's call and nobody else's.**

#### (d) Persistent volumes and the handoff mechanics

**What must survive a spot generation** — the whole of a world's identity:

| File | Why it cannot be lost |
|---|---|
| The sidecar's `<data-dir>/journal/journal.log` | D2 custody. Losing it loses organisms in flight and breaks at-most-once |
| `<data-dir>/peer-id` and the remembered slot | it is what makes the return a **reclaim** rather than a new peer at a new address |
| `<data-dir>/contract-a.token`, and the peer credential secret | B22; without it the returning peer is refused at the handshake |
| The game's save directory | the world itself |
| The genome cache | rebuildable, but rebuilding it costs the map genome fetches |

All of it fits comfortably on the smallest volume either cloud sells. **EBS gp3 at $0.08/GB-month
and pd-balanced at ~$0.10/GiB-month** **[web, 2026-08-11]** make 40 GB cost **$3.20 or $4.00 a
month** — the "cheap more permanent option" the owner asked for is exactly this, and it is a
rounding error against the compute.

**The handoff, step by step, with the measured costs this project already has:**

1. **Notice.** AWS gives **2 minutes**; GCP gives **30 seconds**, best-effort, with a 120-second
   setting in preview **[web, 2026-08-11]**.
2. **Quit the game cleanly.** The rig's own `send <n> quit` triggers save-on-quit. Measured: **3–5 s
   headless locally, 13 s at the far end** **[record]**. The save itself is 0.47–2.69 s, and 43 of
   76 breach D14's 2 s budget **[record: *Watch items*]**.
3. **Stop the sidecar** with `SIGTERM`. Its journal is already fsynced per record — durability
   happens before the ACK, not at shutdown — so this costs nothing.
4. **Detach the volume** (or simply let the instance terminate with `DeleteOnTermination=false`).
5. **The map notices and routes around.** Lanes re-pair as bypasses, held entries wait on the
   bounded 24-hour hold. **Nothing on the map breaks.**
6. **A new spot instance attaches the volume**, starts the sidecar, and the sidecar reclaims:
   `reason=reclaimed`, same coordinate, same `peerId`, journal replayed with **zero discarded
   bytes** — the sequence proven five times on 2026-08-11 **[record]**.
7. **The game loads its last save.**

**AWS's 2 minutes is comfortable for all of that. GCP's 30 seconds fits the measured 3–13 s quit
with no margin at all**, on a "best effort" promise, which is a real argument for AWS in Part 2
specifically.

**The one thing that is genuinely new, and it should not be waved past.** Step 7 loads the *last
save*, and the default is `saveMinutes 10`. Organisms that arrived, were spawned and were **ACKed**
in the minutes since that save are recorded in the archive's ledger as delivered and are absent
from the reloaded world. This is not a new hazard — `contract-a.md:1721` names exactly this
sequence ("delivered → spawned → ACK lost → game killed → world reloads from an autosave"), and
§7.3's dedup key is in-memory for the current world session and clears when `sessionId` changes, so
the contract knows about it. **What is new is the frequency.** On a desktop this is a rare crash;
on spot it is the normal end of every generation. The lever is `saveMinutes` — set a cloud world
to 2 rather than 10 and the exposure drops five-fold — and the cost is the save stall, which is
Risk 3's open item. **Nothing in M5 has measured this, and it is the first thing a rehearsal should
count.**

### Costing one cloud world

Sizing, from **[record]**: a headless game instance on this rig holds WorkingSet **1,954–2,450 MB**
and burns **1.04–1.40 cores** at a ×100 target with 40–60 organisms; it was 403–466 MB when fresh.
Its sidecar adds 70–145 MB and 0.25–0.58 cores. **2 vCPU and 4 GB is the honest minimum for one
fast world**, and 8 GB is the honest recommendation, because populations and `recordedSpecies` only
accumulate.

Per month, one world, same region and availability zone as the relay:

| Variant | Compute | Volume (40 GB) | IP | Egress | **Monthly** |
|---|---|---|---|---|---|
| **AWS Linux spot** `c7i.large` (2 vCPU, 4 GB) | $23.36 | $3.20 gp3 | $3.65 | ~$0 — see below | **~$30** |
| AWS Linux **on-demand**, same box | $65.15 | $3.20 | $3.65 | ~$0 | ~$72 |
| **GCP Linux spot** `e2-standard-2` (2 vCPU, 8 GB) | $24.46 | $4.00 pd-balanced | $1.83 | $0 intra-zone | **~$30** |
| **AWS Windows spot**, same box | $47–$90 | $4.80 (60 GB) | $3.65 | ~$0 | **~$56–99** |
| **GCP Windows spot**, same box | $82.86 | $4.00 | $1.83 | $0 | **~$89** |

**Spot prices are the most perishable numbers in this document.** `c7i.large` Linux spot at
$0.032/hr and `e2-standard-2` spot at $0.0335/hr are single quotes taken **[web, 2026-08-11]**;
AWS adjusts continuously and GCP's Spot prices can move once every 30 days. Re-quote before
buying, and check the interruption *frequency* for the chosen type as well as its price — a type
that is 20% cheaper and interrupts four times as often is not cheaper.

**Put the cloud world in the same availability zone as the relay, and its migration traffic is
free.** Same-AZ traffic over private addresses is unbilled on AWS, and intra-zone internal-IP
traffic is unbilled on GCP. Across zones or over the public internet, a world running at ×15 emits
**357 MB/day per unit S** — about **5.4 GB/day, 160 GB/month, ~$14** at AWS's $0.09/GB. **Placement
is worth as much as the spot discount here**, and it is a free decision made once.

**Two honest counter-arguments to the whole idea:**

- **The reason to run a cloud world is speed, and speed is what costs money.** A cloud world at ×1
  is a slower version of something the owner's desktop already does for free. A cloud world at an
  achieved ×15 costs $30/month and contributes **15 units of S**, which at 56 MB/day per unit is
  **~840 MB/day of archive growth of its own** — about **75 GB over a full 90 days**, or three
  times the entire five-peer human map. A cloud world is not only a compute bill; it is a storage
  bill on the service in Part 1, and it lands on the *same* volume Decision 3 is about.
- **A world the owner runs does not count toward the exit test.** Decision 8's bar is **≥4
  non-owner peers**. A cloud world is the owner's peer wherever it runs.

**And one strong argument for it that the plan does not currently have.** WP5's churn harness is
synthetic — 50 M events with no game behind them. Risk 2 says the exit test cannot be re-run
cheaply because it spends other people's goodwill. **A spot-hosted cloud world is a churn generator
with a real game behind it, interrupting on somebody else's schedule, that costs nobody's
goodwill.** That is the one rehearsal the milestone cannot currently buy, and it is worth $30 a
month on its own merits regardless of whether cloud worlds are ever scaled.

---

## Part 3 — the recommendation ladder

Three stages. Each one is separately abandonable, and each one names what it proves.

### Stage 1 — the core service. This is WP3 proper.

**The measurement this stage used to wait on is done** **[rig experiment, 2026-08-12]**:
`GOMEMLIMIT=5GiB` takes about a third off the replay peak for no measurable wall clock and belongs
in the service unit, and it moves *keep everything* on an 8 GB instance from day 26 to day 43 — not
to 90.
**What the bundle waits on now is Decision 3 and one ~80-line change to the archive's replay**,
which takes 5.6–7.0× off the peak and puts the same instance past day 180.

| | |
|---|---|
| **What** | One Lightsail instance, US East, relay + archive co-hosted, on an owner-registered domain, with the six DQ2 obligations built around it |
| **Cost** | **$36 + ~$5 domain** if Decision 3 bounds the ledger — **$0 + $5** if the 90-day trial applies. **$132 + $5** if the rule is *keep everything* |
| **Proves** | WP3's done-when clauses: restart on exit and on boot, a survived certificate rotation, monitoring that reaches a person, backup of `ring.json`, `peers.json` and the archive's three durable files, a written restart policy, the forward receipt measured at rate, the announced period, the wind-down, and the retention rule with its arithmetic |
| **D24 says** | **the clock starts here.** The announced period begins when participants can join, which means the wind-down procedure has to be written **before** this stage completes, not after. The ending is a stated event, and this is the stage that states it |
| **Owner's calls in it** | the retention rule; the bundle; the name; whether the relay moves to 443 |

### Stage 2 — ONE experimental cloud world, as a rehearsal.

| | |
|---|---|
| **What** | One spot instance, same AZ as Stage 1, running the **itch.io Linux build** headless under BepInEx `linux_x64`, with a persistent volume carrying the journal, peer identity, credential and saves, and a handoff script on the interruption notice |
| **Cost** | **~$30/month**, so **~$60–90** for the remainder of the period. Plus ~0.8 GB/day of archive growth if it runs fast — ~$0 on a Lightsail bundle's included disk, real on a metered one |
| **Proves, in order of value** | (1) **what a world costs per achieved ×** on a clean single-tenant box — Part 2(b)'s rehearsal answered its five questions but could only bound the time-scale gap's *existence*, because this host carries five live worlds; that number decides whether Stage 3 is worth anything; (2) whether the spot handoff is as free as WP5 says, with a **real game** behind it; (3) how often the reload-to-last-save gap actually bites, which nothing has measured; (4) that the Linux + BepInEx path survives *unattended for weeks*, which one 15-minute rig run cannot show |
| **D24 says** | it fits inside the period with room to fail. It is also the **only** stage that can be abandoned mid-run with no participant-facing consequence, because it is just another peer |
| **The system working** | if it dies and the map does not notice, that is the result, not a near miss. Say so in the record |
| **Owner's calls in it** | whether to ask the developer first; whether to pay him and how much; whether a peer he controls belongs on a map he is asking strangers to join |

### Stage 3 — scale, only if Stage 2 proves out.

| | |
|---|---|
| **What** | N cloud worlds, same pattern |
| **Cost** | **~$30/month each**, plus **~0.8 GB/day each** of archive growth at ×15 |
| **Proves** | nothing new about the design. This stage is about wanting more worlds, not about learning anything |
| **D24 says** | **the binding constraint is the archive, not the compute.** Four cloud worlds at an achieved ×15 add 60 units of S — **~3.4 GB/day, ~300 GB over three months**, more than ten times the entire five-peer human map. Stage 3 does not scale the bill linearly; it scales Decision 3's deadline *forward* |
| **Owner's calls in it** | all of it. Also: whether cloud worlds crowd out the human peers the exit test actually needs |

### What the three months mean, taken together

D24's period was chosen on 2026-08-11 and runs to roughly **2026-11-11**. Three consequences the
ladder has to respect:

- **The archive's ledger at day 90 is bigger than this rig's is today**, even at the exit-test bar
  and even at ×1. Whatever is true about restarting the archive gets less true every day of the
  run, and month three is when it matters. That is not a reason to shorten the period; it is a
  reason to decide Decision 3 before day one.
- **Lightsail's free trial is 90 days.** The alignment is a coincidence and should be treated as
  one — verify eligibility, and do not let a free tier decide an announced commitment.
- **The wind-down is a Stage 1 deliverable, not a Stage 3 one.** What becomes of `ring.json` and
  the archive's three durable files is Decision 3's answer, delivered at D24's ending, and it has
  to be written while there is still nobody to disappoint.

---

## What remains the owner's to decide

Ordered by how much else depends on them.

1. **The retention rule (Decision 3).** It sets the disk, the instance, whether the archive can
   restart in month three, and about $96 of the Part 1 bill. It is the one decision everything else
   in this document waits on.
2. **AWS or GCP.** This document recommends AWS on price, on bundled egress, and on a 2-minute spot
   notice against 30 seconds. It does not know what the owner already knows how to operate, and
   that is worth more than the difference.
3. **The bundle**, once (1) is answered: $12 with a bounded ledger, $44 with *keep everything*.
   With *keep everything*, the $44 bundle is only restartable through month three if the replay is
   streamed first (measured 2026-08-12); `GOMEMLIMIT` alone gets it to day 43.
4. **The name.** A registered domain (~$5 for the period) or free dynamic DNS ($0). Either works;
   the cloud's own hostname does not. **It is effectively permanent once the first join string is
   minted.**
5. **Port 443 or the relay's own port**, decided before the first join string is minted, because
   the port is in the string.
6. **Whether to run any cloud world at all**, and whether to ask the developer first.
7. **Whether to pay for the itch.io build per cloud instance**, and how much. Name-your-own-price
   makes this a choice.
8. **Whether the status page's poll cadence is worth changing.** The measurement is here; the
   design is not this document's. Compression is no longer part of this question — it was
   implemented on 2026-08-12 and ships with the archive's next restart.
9. **Whether a peer the owner controls belongs on a map of strangers**, given that Decision 8's bar
   counts only non-owner peers.

## Open questions this document could not close

- **Does AWS's Spot discount apply to the Windows licence portion?** Not answerable from search.
  Resolve by reading the Spot price history for the chosen instance type. It is only load-bearing
  if the Linux path in Part 2(b) fails.
- **Does GCP's Always Free e2-micro include an external IPv4 address?** The free-features page lists
  the instance, 30 GB of standard PD and 1 GB of egress, and does not mention an IP. Budget
  ~$3.65/month and verify on the billing console.
- **Does GCP's Standard Tier 200 GiB free allowance stack with the Always Free tier's 1 GB?** They
  are separate programmes and this pass could not confirm how they combine.
- **Does the itch.io Linux build's `BibitesAssembly.dll` match the Windows one?** Downloadable and
  checkable in minutes; not done here because this document was scoped to buy nothing and install
  nothing.
- ~~**Will `GOMEMLIMIT` bring the archive's replay peak down, and at what cost in replay time?**~~
  **Closed 2026-08-12** by the matrix in *The term nobody has priced*: yes — about a third off the
  peak at `GOMEMLIMIT=5GiB` for no measurable wall clock and 17–39% more CPU, ruinous below ~2× the
  retained set, and **not enough to reach day 90**. It left a bigger question in its place, one
  that is not open so much as unowned: **the replay materialises the whole ledger before applying
  it, and streaming it instead measured a 5.6–7.0× lower peak in ~80 lines.** That change belongs
  to whoever owns `go/internal/archive/store.go`; this document only measured it.

## Sources

Vendor and search sources, all consulted **2026-08-11**:

- [Amazon Lightsail pricing](https://aws.amazon.com/lightsail/pricing/) — bundle tiers, included transfer, block storage, IPv6-only pricing
- [Lightsail free tier specifications (3 months / 90 days)](https://repost.aws/questions/QUgWKsPw-aSZqMUHFFvPS3kg/lightsail-free-tier-specifications-3-months-90-days) — trial terms and eligible plans
- [EC2 On-Demand Instance Pricing](https://aws.amazon.com/ec2/pricing/on-demand/) and [t4g.small pricing and specs — Vantage](https://instances.vantage.sh/aws/ec2/t4g.small) — `t4g.small` hourly rate
- [c7i.large pricing and specs — Vantage](https://instances.vantage.sh/aws/ec2/c7i.large) — Linux on-demand and spot rates
- [AWS Data Transfer Out to Internet Pricing (first 100 GB free)](https://egresscost.com/aws/data-transfer-pricing/) — egress tiers
- [New AWS public IPv4 address charge](https://aws.amazon.com/blogs/aws/new-aws-public-ipv4-address-charge-public-ip-insights/) — $0.005/hr since 2024-02-01
- [Amazon EBS pricing](https://aws.amazon.com/ebs/pricing/) — gp3 $0.08/GB-month
- [Amazon Route 53 pricing](https://aws.amazon.com/route53/pricing/) — hosted zone and query pricing
- [Google Cloud free features](https://docs.cloud.google.com/free/docs/free-cloud-features) — Always Free Compute Engine terms
- [Google Cloud VM instance pricing](https://cloud.google.com/compute/vm-instance-pricing) and [Compute Engine pricing guide 2026 — CloudZero](https://www.cloudzero.com/blog/google-cloud-compute-engine-pricing-guide/) — `e2-small`, `e2-medium`, `e2-standard-2`, spot rates and the 60–91% discount range
- [Google Cloud Network Service Tiers pricing](https://cloud.google.com/network-tiers/pricing) and [VPC network pricing](https://cloud.google.com/vpc/network-pricing) — Premium vs Standard egress, external IP rates
- [Google Cloud disk and image pricing](https://cloud.google.com/compute/disks-image-pricing) — pd-balanced
- [Google Cloud DNS pricing](https://cloud.google.com/dns/pricing) — managed zone pricing
- [Spot VMs — Compute Engine documentation](https://docs.cloud.google.com/compute/docs/instances/spot) — 30-second preemption notice, premium licensing on Spot
- [Spot Instance interruption notices — Amazon EC2](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/spot-instance-termination-notices.html) — 2-minute notice
- [Understanding Windows license costs on AWS — Strategic Blue](https://strategic-blue.com/resources/blog/understanding-windows-license-costs-on-aws-analysis-and-pricing-insights) — $0.046/vCPU-hour
- [The Bibites on itch.io](https://thebibites.itch.io/the-bibites) — platforms, version 0.6.3.1, name-your-own-price, relationship to the Steam build
- [Steam Subscriber Agreement](https://store.steampowered.com/subscriber_agreement/) and [Steam concurrent-session discussion](https://steamcommunity.com/discussions/forum/1/558752449678816417/) — one active session per account
- [BepInEx 5.4.23.2 release notes](https://github.com/BepInEx/BepInEx/releases/tag/v5.4.23.2) and [Installing BepInEx on Mono Unity](https://docs.bepinex.dev/master/articles/user_guide/installation/unity_mono.html) — Linux `linux_x64` builds, Doorstop 4, `run_bepinex.sh`
- [Let's Encrypt community — policy forbids issuing for AWS EC2 hostnames](https://community.letsencrypt.org/t/policy-forbids-issuing-for-name-on-amazon-ec2-domain/12692) — `*.compute.amazonaws.com` is blocked by issuance policy

Project sources, cited inline by document and section: `m5_tracking.md`, `m5_considerations.md`
(DQ2, DQ3, Decisions 2/3/4/8, Risks 3/5/7/9/10, WP3–WP5), `dev_environment.md` (*Versions*,
*The disk budget*, *The living deployment*, *Watch items*, *Gotchas*),
`contracts/contract-a.md` (D2, §7.3), `contracts/contract-b-m4.md` (§9, §10, B21–B32),
`go/internal/peercred/peercred.go`, `go/internal/archive/page.go`,
`go/internal/archive/store.go` (`ReadLedger`) and `go/internal/archive/archive.go` (`New`'s replay),
`go/internal/relay/relay.go`, `bibites-mod/game.sh`.
