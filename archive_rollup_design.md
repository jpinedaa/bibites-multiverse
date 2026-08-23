# Archive record roll-up — design

**State: ratified, implemented, and deployed on `2026-08-17`.** ~~Proposed, not ratified. Nothing in
this document is implemented and nothing in it changes a published promise on its own.~~ It is the
design study behind the change to what the archive keeps, written so the change could be argued
with before it was made. **It is left as it was written**, except where a later phase corrected it
in place. The first four corrections are struck through and dated. The fifth is the lineage
format correction here.

**Fifth correction, 2026-08-23.** The original species fold kept one mutable parent edge for each
normalized name. The game can reuse a name. A later occurrence can then replace an older edge
and create a cycle in the derived tree. Roll-up format 3 keeps bounded, immutable lineage
instances. Each instance is one normalized name on one recorded parent path. A format-2 sidecar
does not contain the ordered evidence needed to build them. The first format-3 start must rebuild
the fold from the ordered raw record. `deploy/restart-archive.sh --rebuild-rollup` performs that
operation and preserves the old sidecar. A format-3 archive refuses to start while a format-2
sidecar remains in place.

The owner ratified shape **B for the state, D for the raw, and a window on top of D** on
`2026-08-17`; all six phases shipped and the build carrying them is in production. The governing
texts it amends — `contracts/contract-b-m4.md` §10, §23 and the new **§26 (B39–B40)**,
`system_decomposition.md` D6 and D11, `m5_considerations.md` Decision 3,
`docs/participant/join.md`, `docs/participant/leave.md`, `deploy/SIZING.md`,
`deploy/WIND-DOWN.md` — **all carry the amendment**. Each of them, and not this study, is the
authority for what the map promises and what the archive does.

`m5_considerations.md`, Design Question 7, already states the procedure this document follows:
*"If the owner wants the stronger promise, that is a change to D11's never-evict rule and belongs
in a decision row, not in a support reply."* This is that decision row's engineering half.

## The question

The archive keeps one file, `migrations.jsonl`, with one JSON line per recorded event, appended
with an `fsync` each and **never rewritten** (`go/internal/archive/store.go`, file header). That
file is the record. Everything the archive can say — every lane rate, every species' first
crossing, every ancestry edge, every genome gap — is a fold over it, rebuilt from scratch by a
full replay at every start (`archive.go`, `New`).

The rule that goes with it is stated as strongly as the code: *"nothing here may evict, and a
retention rule that contradicts that is a change to D11 rather than a configuration of it"*
(`contract-b-m4.md` §10). §23 then changed exactly one half of it — the genome **blobs** get a
horizon, the ledger does not.

The question this document answers is whether the other half should change too, and if so into
what. The proposal is: **keep the answers forever and the raw lines for a window.**

## What it costs today, measured

All figures are from the `2026-08-16` production ledger, replayed offline on a copy. The
per-record constants belong to that build; `deploy/SIZING.md` owns them and states the workload.

| Quantity | Measured |
|---|---:|
| Records in the copy | `5,408,123` |
| Bytes on disk | `1,836,382,633` — `339.6 B` per record |
| Records for each crossing | `2.12` observed, `2.14` reference |
| Growth | `3.0` to `3.9` million records each day |
| Disk growth | `1.0` to `1.3 GB` each day |
| Replay rate, service host | `60,631` records each second, falling slowly as the file grows |
| Replay time today | about `90 s` |
| Retained state, current build | `28` to `31 B` for each record |
| Compression of a closed ledger, `gzip -1` | `5.75x` (`58.8 B` for each record) |
| Compression of a closed ledger, `gzip -6` | `8.5x` |

Three separate things grow with the record, and they do not have the same fix.

**Memory.** One structure grows with the ledger and nothing else does: the duplicate-suppression
set (`dedup.go`, file header). Every other retained thing is bounded by construction. The species
bound is ~~`4096`~~ **`65,536`, corrected 2026-08-21 after the public map exhausted the original
bound**. The lineage-instance bound is **`131,072`, corrected 2026-08-23 after name reuse produced
derived cycles**. Each species keeps at most `8192` genome fingerprints. Brain buckets, lanes, and
genome gaps also have fixed bounds. The measurement isolated the duplicate set. Before the
`2026-08-16` fingerprint change, it used about `70 percent` of the settled resident set. After the
change, it used about `128 MB` of live heap.

**Without this set, the retained state has a fixed bound and does not follow the record count.**
The archive uses about `30 MB` of live heap and `60` to `70 MB` of resident memory. The old shape
can reach `7.6` to `10.9 GB` by the announced end of the run.

That removal is not this document's to make. It is a consequence of the migration protocol
becoming pure at-most-once — no hold, no re-forward, no bounce — which makes a legitimate
duplicate impossible by construction. See "What the duplicate set becomes" below.

**Disk.** `1.0` to `1.3 GB` each day, and the run is announced as bounded — three months,
`2026-08-14` to `2026-11-14` (`docs/participant/leave.md`). The remaining `89` days of it are
`89` to `116 GB` of new ledger. The service host's volume does not hold that, and neither does
the next size up. **The promise "kept for the whole run and beyond" is not funded to the end of
its own run on the host it was made on.** That is the finding, and it is not softened by a larger
instance: a larger instance moves the date and changes nothing about the shape.

**Restart time.** The replay reads every record ever written before the archive serves anything,
and the record-preserving restart holds the relay down for its whole length
(`deploy/RESTART-POLICY.md`), so replay time is participant outage. It is about `90 s` today. At
the announced end of the run, at the measured rate and without a change, it is **`75` to
`96` minutes**. That is also why an archive restart is the one action deliberately kept out of
the deployment workflow.

## What each surface actually needs

Every live endpoint and every M5 evidence item, against the minimal durable state that serves it.
"Bounded" means bounded by something other than the record count.

| Surface | Data it needs | Bounded | Source today | Proposed source |
|---|---|---|---|---|
| `/api/status` map, slots, census | last `PEER_STATUS` | yes, live | relay, in memory | unchanged |
| `/api/status` lane totals, `perMinute` | per-lane cumulative counts | yes, grid | full replay | roll-up state |
| `/api/status` `ledgerRecords` | one counter | yes | full replay | roll-up state |
| `/api/status` `genomeGaps` | pending fetch set | yes, horizon | full replay | replay of the raw window |
| `/api/status` eviction counters | four counters | yes | process-local | roll-up state |
| `/api/hops` | last `60 s` of crossings | yes, time and count | live only, never persisted | unchanged |
| `/api/species` | census, joined to all-time species counts | yes, ~~`4096`~~ **`65,536`, corrected 2026-08-21** | full replay | roll-up state |
| `/api/species/tree` | immutable lineage instances, ancestry floor | yes, **`131,072` instances, corrected 2026-08-23** | full replay | roll-up state, format 3 |
| `/api/species/trends` | `metrics.jsonl` tail | yes, byte-bounded read | sample file | unchanged |
| `/api/species/history` | `metrics.jsonl` tail | yes, byte-bounded read | sample file | unchanged |
| `/api/species/brains` numerator | brain histograms for each bucket | yes, `365 d` | `brains.jsonl` | unchanged |
| `/api/species/brains` denominator | distinct genomes seen for each bucket | yes, `365 d` | **full replay, never persisted** | **`brains.jsonl`, added** |
| `/api/history`, `range=all` | `metrics.jsonl` | yes, `2 GB` read bound | sample file | unchanged |
| `/live`, `/watch`, `/`, static | none | — | — | unchanged |
| `archive list` | the raw lines | no | full file | the raw window, and says so |

The M5 evidence record, the same way:

| Evidence item | Data it needs | Proposed source |
|---|---|---|
| At-most-once, losses counted | loss and refusal counters | **new counters**, see below |
| Crossing rates, per-lane flow | lane cumulative totals | roll-up state |
| Per-peer archive growth | per-peer record counters | **new counter**, bounded by slot count |
| All-time species crossings, first, last | species aggregate | roll-up state |
| Genealogy depth, ancestry floor | lineage instances and `edgeFirstMs` | roll-up state, format 3 |
| Brain-complexity coverage | numerator and denominator | `brains.jsonl`, denominator added |
| Version skew | `gameVersion` for each crossing | `metrics.jsonl`, or a **new bounded counter** |
| Velocity floor | `60 s` hop window | **never durable, today or after**; derive from the raw window |
| Genome-gap accounting | pending set and `genomeGapsExpired` | raw window plus roll-up state |

Three of those are gaps that exist **today** and that a roll-up makes visible rather than
creates: the brain-coverage denominator is re-derived from the full replay on purpose
(`brainhist.go`, and the note in `brainsave.go` that says so), there is no per-peer record
counter, and the hop feed has never survived a restart. Each must be persisted **before** the
first raw segment leaves the host, or the evidence for that period is not recoverable.

## Four shapes, compared

Numbers use `3.9` million records each day — the fast end — and a `30`-day window where a window
applies.

### A — periodic snapshot of the aggregates, plus a rolling raw window

Write the whole in-memory aggregate state to a temporary file and rename it, every `T`. Replay
loads the newest snapshot and then reads only the records behind it.

The aggregates are bounded, so a snapshot is a bounded cost rather than a growing one — about
`13 MB`, dominated by a year of five-minute brain buckets. At one an hour that is `312 MB` of
writes each day, a `25` to `30 percent` increase in the archive's write volume, to save at most
an hour of tail replay. Restart falls to seconds. RAM unchanged. Crash story is simple: a
snapshot is atomic by rename, and a torn one is discarded for the previous one.

**Effort** about `14` to `18 h`. **Risk** low. **Against it:** `brains.jsonl` already faced this
choice and argued the other way in its own header — a full rewrite on a timer costs the size of
the whole state every time it runs — and having two persistence disciplines for two aggregates in
one package is a cost that never stops being paid.

### B — incremental append-only aggregate log, plus a rolling raw window

The `brains.jsonl` shape, applied to the rest of the fold: one sidecar, a version header, a
line for each aggregate record, **append only what changed since the last save**, last-writer-wins
for each key on replay, and a full rewrite only when the file exceeds a fixed multiple of its live
content. The loss rule is already designed and already tested there.

Save every `30 s`, appending only the species that crossed and the buckets that moved: `20` to
`90 MB` of writes each day, an order of magnitude under A. Compaction at `3x` live content keeps
the file at tens of megabytes. Restart reads the sidecar plus at most `30 s` of raw records —
about `2,000` — so it is **a few seconds, flat, for the life of the deployment**. RAM is the
bounded aggregates: about `30 MB` live heap. Disk for the raw window is `6.2` to `8.0 GB` at
`gzip -1`, or `4.5` to `5.8 GB` at `gzip -6`, flat.

Crash story: identical to the ledger's and the journal's, which is the point — one discipline in
the package, not two. A torn tail is dropped, an unreadable file is kept beside a new one and the
state is rebuilt by a full replay of whatever raw segments are still present.

**Effort** about `18` to `22 h` for the sidecar, plus `22 h` for segmentation. **Risk** medium,
and concentrated in one place: a fold that is persisted incrementally must have every write site
mark its key dirty, and a missed site is a silently stale number. That is exactly what the
real-ledger validation below exists to catch.

### C — an embedded store for the aggregates, plus a raw window

SQLite or bbolt behind the aggregates. `store.go`'s header rejected a database for three named
reasons and all three still hold: it would no longer be inspectable with the tools already on both
machines, it would add a second crash discipline beside the journal's, and it would have a schema
to migrate. `deploy/SIZING.md` names an on-disk index as one of only two things that can bound
retained state — but that argument is about **memory**, and memory is bounded by construction once
the duplicate set goes. An index buys nothing this roll-up needs.

**Effort** `60` to `120 h` plus migration and soak. **Risk** high.

It becomes the right answer the day a surface needs random access into per-crossing history —
"show me this species' crossings from the first month" — which is M7's catalog and not this.

### D — compress closed segments, delete nothing

Rotate the ledger daily, compress each closed segment, keep every one. The whole announced run is
`94` to `121 GB` raw and **`16` to `21 GB` compressed**, which the current volume holds. The
promise does not change at all, and `zcat`, `zgrep` and `jq` still read it.

**Effort** about `12 h`. **Risk** low. **What it does not fix:** restart time, which stays a
function of the whole record and gets worse because each restart now decompresses as well as
parses. And it is unbounded in exactly the way that produced this question — a map with more peers
raises the rate, and nothing in the rule notices.

### The recommendation

**B for the state, D for the raw, and a window on top of D.** They are complementary rather than
alternatives, and each fixes what the other cannot:

- **B** persists the fold, which is the only thing that bounds restart time. It converts an
  archive restart from a `75`-to-`96`-minute participant outage at the end of the run into ~~a
  few seconds, permanently~~ **a replay of the duplicate window — corrected 2026-08-17 by phase 1,
  and resolved.** The sidecar bounds the *aggregate* cost of a restart and not the *raw* one: the
  duplicate guard and the genome-gap fetch queue are both rebuilt from raw records, so a replay
  scans `max(dedupWindow, genomeHorizon)` and this study's "a few seconds, flat" (shape B above)
  was true of neither. **Phase 3 resolved it** by persisting the gap queue — see "The window, and
  why it is the genome horizon" — after which **only the duplicate window binds**: `48 h` of raw
  today, and `1 h` once the participant release has been out for a cycle and `duplicatesRefused`
  says it is safe. The first start after the deployment still replays the whole ledger once,
  exactly as "Migration, in one restart" says; every start after it is the window.
  `deploy/RESTART-POLICY.md` and `deploy/SIZING.md` carry the shipped model.
- **D** compresses closed segments, which is what makes a long window affordable at all.
- **The window** is the bound, and it is the part that needs the owner: it changes a published
  promise.

Choose B over A because the package already contains B, argued and tested, and a second discipline
is a permanent tax. Choose against C because it solves a problem that the at-most-once change
solves for free.

**Backups become a different and much better problem, and this is worth stating on its own.**
Today the irreplaceable thing is a `1.8 GB` file that grows `1.3 GB` each day, that `backup.sh`
copies to the same volume in one generation, and that it **skips entirely** when free space falls
under `30 percent` — which is the state a growing ledger produces. After the change the
irreplaceable thing is a tens-of-megabytes sidecar plus a set of `226 MB` immutable daily
segments. The sidecar joins the hourly identity tier with many generations; each closed segment
ships off-host exactly once and never changes again. An off-host copy stops being a project and
becomes a `226 MB` daily upload.

## The window, and why it is the genome horizon

The window should be **`30` days — the same `720 h` the genome blobs already have.** The
arithmetic and the argument both land there.

**The argument first, because it is the stronger half.** §23's B34 made the store's eviction
horizon and the fetch queue's retirement horizon **one number**, because two numbers make an
archive that re-fetches what it just deleted. A ledger window is the third mechanism reading the
same clock. A genome gap whose crossing is older than the horizon is never queued
(`eviction.go`, `gapPastHorizonLocked`), so **a raw window equal to the horizon contains exactly
the crossings whose gaps can still be fetched.** ~~The fetch queue is then rebuilt from the window
alone, with nothing new to persist and nothing lost.~~ **Corrected 2026-08-17 by phases 1 and 3,
and resolved: the fetch queue is persisted.** Rebuilding it from the window is correct but it is
what forced every restart to parse `720 h` of raw records, which is the contradiction phase 1
measured. Phase 3 gave the roll-up state a `gq` line kind — additive, with a tombstone, because a
set is the one thing last-writer-wins cannot express cheaply — carrying the hash, `crossedAt` (the
only lawful clock for this horizon), the source peer, and what `brainhist.go` needs when the blob
finally lands. A restored queue is **drained on load**, so it is in the state a full replay would
have produced, and the honest consequence is written down with it: **retirement is durable**, and
turning the horizon back up does not resume asking for a gap already retired. A shorter window
silently abandons live gaps; a longer one buys nothing. One horizon, three mechanisms.

**The arithmetic.** At the fast rate, `3.9` million records each day is `1.32 GB` each day:

```text
live segment                     1.32 GB
29 closed segments, gzip -1      29 * 1.32 / 5.75  = 6.66 GB
window total                                        7.98 GB
```

At the slow rate the same sum is `6.16 GB`. At `gzip -6` it is `4.5` to `5.8 GB`. Against a
`60 GB` volume that already holds a genome store of about `5 GB`, an operating system, logs and a
backup tier, a flat `6` to `8 GB` is affordable with room for the crossing rate to double. The
same window on the next size up is the same number, because the window does not grow.

Compare the alternatives on the same volume: no window at all is `94` to `121 GB` and fails
before the run ends; a `7`-day window is `1.5` to `2.0 GB` and abandons live genome gaps for no
gain; a `90`-day window is `19` to `24 GB` and is just option D with extra steps.

**`metrics.jsonl` is a smaller problem and does not need the same treatment.** It grows with wall
time rather than with the record — `1.6 MB` each day for each slot — which is under `1 GB` for the
whole announced run, already under the `2 GB` bound its own all-record reader applies. Rotate and
compress its closed months on the same machinery, keep every one, and revisit if the slot count
grows. It is not urgent and it should not hold this change up.

## Migration, in one restart

1. **Take the by-hand backup `backup.sh` already demands before a retention-rule change**, and
   ship the current whole ledger off-host as a one-off cold archive. This is the copy that makes
   every later "could we still compute X for the first month" answerable. A development-box copy
   is a proof artifact under its own handling rules and is **not** that archive.
2. **Deploy the new binary and restart once**, under the record-preserving sequence — the relay
   held down for the replay, exactly as today.
3. **On first start** the archive finds no sidecar, replays the whole existing file exactly as it
   does now, writes the first roll-up state from that replay, and then **renames the existing file
   whole into one legacy segment** and opens a fresh live file. Nothing is re-shuffled and no
   record is rewritten; the rename is atomic within the directory. That legacy segment is
   compressed on the next pass and retires under the ordinary window rule, `30` days later, in one
   step.
4. **Rollback** is a concatenation. Segments are ordered, append-only and never rewritten, so
   `zcat` of the segments in order followed by the live file reproduces a byte-exact
   `migrations.jsonl`, and the previous binary runs on it unchanged. The cost of rolling back is
   one full replay.
5. **The first segment retirement is the point of no return**, not the deployment. Everything up
   to it is reversible on the host. Set the off-host copy as a precondition for it, in the code:
   **a closed segment is not removed until its off-host copy is confirmed.**

## What becomes unrecoverable, stated plainly

Once a raw segment leaves the host, **any aggregate that was not being computed while it was there
cannot be computed for the period it covers**, except by fetching the cold copy back and replaying
it offline. That is the real cost of this change and it is not reduced by careful implementation.
Three consequences follow and each is a decision, not a detail:

- **Every aggregate the evidence phase needs must be persisted before the first retirement.** The
  table above is that list. A gap in it is discovered `30` days late.
- **Per-crossing questions become window questions.** "Which individual crossed when" is
  answerable for `30` days on the host and for the run in the cold archive, and not on any live
  surface. The exactly-once census by entity identifier is the example, and it is moot for a
  different reason — see below.
- **A new aggregate cannot be added retroactively** to a period whose segments have gone. Adding
  one is cheap while the cold archive exists and impossible if it does not, which is the strongest
  argument for keeping the cold archive under an approved policy rather than as a courtesy.

## What the duplicate set becomes

With the protocol at pure at-most-once — no hold, no re-forward, no bounce — a legitimate
duplicate cannot occur. The unbounded duplicate set is then unnecessary **for its stated purpose**,
and it is the structure whose removal bounds the archive's memory.

Removing it **entirely** is the wrong step, and the reason is the one the record cares about: a
second record with the same identifier can still arrive from a build that predates the change,
from a relay fault, or from a peer that chooses its own identifiers. Today such a record is
refused. With no guard it is appended, and the permanent record gains a duplicate that nothing can
remove, because the file is never rewritten.

**Keep a bounded guard.** The same structure, holding only the newest window of keys and rebuilt
from the raw window at startup. Under at-most-once with no hold, the longest interval over which a
duplicate can arrive is one relay session's retransmit — seconds. An hour of keys is a margin of
three orders of magnitude and costs about `4 MB`; a day costs about `97 MB` and buys nothing more.
Publish a `duplicatesRefused` counter beside it: during the transition a non-zero value is exactly
the evidence that a peer is still running the old build, and after it a non-zero value is a defect
report.

**The evidence claim changes with it.** It is no longer "exactly once, proved by an entity-identifier
census over the whole ledger" — a census that stops being computable on the host once the window
turns over. It becomes **"at-most-once by construction, with losses counted"**: no component
re-forwards, no component bounces, a migration to a dark destination is a counted loss, and the
evidence is the loss counters, the duplicate-guard counter, and the per-lane flow totals. Those
counters must exist before the claim is made.

## Implementation order

| Phase | Work | Hours |
|---|---|---:|
| 1 | Roll-up state sidecar: line shape, header, dirty tracking, append-and-compact, loss rule, load before the tail replay | `18`–`22` |
| 2 | Segment rotation, compression of closed segments, crash reconciliation at startup, window retirement gated on an off-host receipt | `22` |
| 3 | Read paths: `archive list`, the materialising reader, the gap rebuild from the window, the honesty fields that say what the window is | `6` |
| 4 | Deployment surface: `SIZING.md`, `backup.sh` tiers, `monitor.sh` (its replay gate is a model of **record count** and must become a model of the **window**), `RESTART-POLICY.md`, `docs/observability.md`, and the `MV_RETENTION` enum — `provision.sh` validates it against a closed list of four values and this rule is not one of them | `8` |
| 5 | Governing texts and every user-facing string that repeats the promise: the contract amendment set, D11 and D6, Decision 3, the two participant pages, the console's own tooltips and status line, the `--genome-horizon` and `--deny-list` help text, the `--diagnose` output, the participant notice | `8` |
| 6 | Validation on a real-ledger copy | `6` |
| | **Total** | **`68`–`74`** |

Phases 1 and 2 are independent and can run in parallel. Phases 5 and 6 gate the deployment.

**Validation is the phase that decides whether this is safe**, and it has a specific bar: replay
the real-ledger copy under the current build and capture every replay-derived endpoint; replay the
same file under the new build; then **roll it into a first sidecar, restart from the sidecar alone,
and require every one of those endpoints to be identical**. The same method already proved the
duplicate-set change lossless, and the clock-artifact fields it has to exclude are already known.
A second run must prove that a segmented replay equals a monolithic one over the same records, and
a third that a crash at each step of a rotation loses nothing.

## Where the promise is repeated, and what else moves with it

The promise is not in one place. A sweep of the tree found it stated in five different registers,
and a change that reaches only the participant pages leaves the product contradicting them.

- **The participant pages.** `docs/participant/join.md` — *"It keeps the crossing record and
  ancestry for the whole run and beyond"* — and `docs/participant/leave.md` — *"Every crossing your
  world made is in the map's archive, and nothing evicts from it"*. The second is the sharper one,
  because it is the sentence used to **refuse a takedown**. Narrowing the raw window does not make
  a takedown possible, and the replacement wording has to keep that clear rather than leave a
  reader to infer it.
- **The live console.** The status page's own tooltips say *"keeps a permanent record of every
  migration"*, *"THE RECORD OF WHAT HAPPENED IS KEPT FOREVER AND IS NEVER AFFECTED BY THIS"* under
  the retention horizon, and the status line reads *"(the ledger is kept forever)"*. These are
  published text and change with the promise. ~~The genealogy's record-floor caption becomes a
  **rolling** date rather than a fixed one~~ — **corrected 2026-08-17, when the texts were
  amended: the ratified `contract-b-m4.md` §26 B39 puts the earliest record that ever carried an
  ancestry edge on the normative list of folds kept forever, so the floor does NOT roll.** One
  word changed: the caption said the oldest crossing *kept here* and now says the oldest crossing
  this archive *ever recorded*. Its tooltip currently tells a reader that the top
  of a family is the edge of the record — which after this change is the edge of the aggregate,
  not of the window.
- **The command-line and diagnostic surfaces.** The `--genome-horizon` help says *"IT NEVER
  TOUCHES THE LEDGER: migrations.jsonl is kept forever at every setting"*, the `--deny-list` help
  and its runtime warning say the same, and the sidecar's `--diagnose` output tells a participant
  that nothing in the system shrinks a durable record.
- **The contract and the design record.** `contract-b-m4.md` §10, §20 B20, §23 B33, the tunables
  table, and `system_decomposition.md`'s statement that the archive *"grows without bound on
  purpose"*. Two of these need more than a wording change. **B20's rule that a replay must skip a
  damaged line and keep every record behind it** is argued from the file being never rewritten and
  the damage therefore being permanent; under segmentation that argument has to be restated rather
  than assumed. And **`SIZING.md`'s conclusion that only two things bound retained state** —
  a lower crossing rate, or an on-disk index — becomes false: a ledger window is a third, and it
  is the one that answers the memory horizon the same section computes.
- **The kit's own vocabulary.** `deploy/WIND-DOWN.md` enumerates four retention shapes and
  `deploy/provision.sh` validates `MV_RETENTION` against exactly those four. This rule is not one
  of them: `bounded-ledger` is a **post-run** disposition, `prune-genomes` keeps the full ledger,
  and this is a ledger window applied **during** the run with the blob rule beside it. A fifth
  value is needed, in the enum and in the table, and the operator-facing description has to say
  what the participant was told.

Two test surfaces assert the current shape rather than describe it, and will fail honestly:
`e2e/baseline.sh` reports the ledger as cumulative across every earlier run, and two of the
end-to-end scripts narrate the same assumption. `m4_findings.md` already records a real defect
caused by relying on it — a wait for "a record of shape X exists" that passed instantly against an
hour-old record — so re-examining them is worth doing on its own.

## Open questions

**Answered on `2026-08-17` by the ratification and by what shipped, and left below as they were
asked.** 1 — `30` days, `720 h`, on the argument as written. 2 — yes, and the rule is in the code:
a closed segment is removed only after its off-host copy is confirmed, so the earliest a segment
can leave the host is `2026-09-16`. 3 — an object store off the host, copied hourly by
`deploy/coldcopy.sh`, and a *confirmed* copy is a receipt file the archive requires beside the
segment before it removes anything; `deploy/README.md` documents the job and its checks. 4 — the
counters shipped with the at-most-once change and the roll-up: forwarded losses, `duplicatesRefused`
and the per-lane flow totals, and `contracts/contract-b-m4.md` §26 states the claim. **5 is still
open** — `metrics.jsonl` is not windowed by this set, and it is bounded for this run only.

1. **The window's value.** `30` days is proposed because it is already the genome horizon and
   because one horizon for three mechanisms is a rule rather than a coincidence. Any other value
   breaks that tie and needs its own argument.
2. **Whether a segment may be deleted on-host at all before the run ends**, given that compressing
   and keeping every segment fits the volume for this run. The window is the bound that survives a
   larger map; keeping everything is the promise that survives this run. They can both be true —
   window on the host, cold archive off it — only if the cold archive is real.
3. **The cold archive.** Where it lives, who checks it, and what a confirmed copy means in code,
   since segment retirement is proposed to depend on it.
4. **The evidence claim.** At-most-once with counted losses replaces exactly-once by census. The
   counters it rests on do not exist yet.
5. **`metrics.jsonl`**, which is bounded for this run and unbounded in principle.

## Related documents

- [`contracts/contract-b-m4.md`](contracts/contract-b-m4.md) §10, §20 B20, §23 B33–B34 — the
  never-evict rule, and the one amendment that has narrowed it.
- [`system_decomposition.md`](system_decomposition.md) D6, D11 — what the archive is for.
- [`m5_considerations.md`](m5_considerations.md) Design Question 3, Decision 3 — the retention
  decision and its deadline.
- [`deploy/SIZING.md`](deploy/SIZING.md) — the measurements, the growth model and the sizing
  procedure.
- [`deploy/RESTART-POLICY.md`](deploy/RESTART-POLICY.md) — why replay time is participant outage.
- [`deploy/WIND-DOWN.md`](deploy/WIND-DOWN.md) — the four retention shapes the kit supports, and
  the rule that a shorter horizon needs a new participant notice. This proposal is a fifth shape.
- [`deploy/ANNOUNCEMENT.md`](deploy/ANNOUNCEMENT.md) — the rule that a retention change is a
  participant notice.
