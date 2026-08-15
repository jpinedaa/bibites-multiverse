# M4 Findings — The Grid, the Living Deployment, and the Cost of Running One

Sources: the M4 implementation wave and everything that followed it, `74ebdc0` (2026-08-05) to
`65e66b6` (2026-08-10); the LAN exit test of 2026-08-06 and its logs under `e2e/logs-m4-lan/`;
the living deployment that has run the same six worlds continuously since that test; the
gitignored observation series in `e2e/baselines/m4-collector/` (467 samples at ~5.5-minute
spacing since 2026-08-07); and the contract amendments A18–A46 and B4–B20 that the running rig
forced.

**This document is the milestone record. Two other documents are titled like findings and are
not it.** `m4_portal_findings.md` is a pre-implementation research document with the same
top-line name; `m4_livelock_findings.md` is one incident. Both are complete on their own
subjects and this document points at them rather than absorbing them (§4, §10).

**What this document deliberately does not do.** `m4_considerations.md` is the design record and
carries its own inline updates against its own questions and risks; `dev_environment.md` is the
operations record and holds the current reading, the bring-up ritual and the open watch items;
the contracts are the authority on the wire. Where a finding is already fully told in one of
those, this document states the finding once, with numbers, and points at the owner. A findings
document that copied them would age the moment one of them moved — and on this milestone they
move daily.

M1 read the game's source and confirmed it in one automated run. M2 read the world geometry and
confirmed it on a two-instance rig. **M4's evidence is different in kind: it is a deployment.**
Almost nothing below could have been learned from a test, because almost everything below is a
consequence of time — worlds that accumulate history, files that accumulate bytes, and processes
that stay up long enough for their assumptions to expire.

---

## Headline result

The design was right about the wire and wrong about the clock.

Every M4 work package shipped, the exit test passed at the full 3×2 shape on its first attempt,
and no design decision of the milestone has been reversed. Then the rig kept running, and four
days of deployment produced more findings than the three previous milestones together. The
reason is one sentence: **every number M4 measured on 2026-08-06 was measured on worlds that
were seconds old, and few of them survived worlds with history.**

| What M4 established at the exit test | What running it taught |
|---|---|
| A periodic save stalls the simulation 241–538 ms, comfortably inside D14's 2 000 ms budget | 17% of 8 087 preserved saves breach it. The cost tracks the **uncompressed** payload, which grows with a world's evolutionary history and not with its population. `lineageMs` alone is 53% of a save, 994 ms median (§6) |
| Arrival pacing at `2.0` per simulated minute is five times the measured natural rate | Two-way lanes and a second inbound axis made it `100.0`/`50` inside a day, and the limit is now sized from the mod's ingest ceiling rather than from offered load (§3) |
| The rig writes what it writes | 1.1 GB/h, none of it bounded. A full volume then silently truncated five journals **and** the archive ledger (§5) |
| The error sweep at the end of the run is the proof | A durability rule proven in one implementation is not proven in the system, and **only a restart replays a journal** — so the rig whose value is staying up is the rig that cannot see its own durability (§5.2) |
| Plural export edges with `E,N` live is the grid | Half of every world's border was dead geometry. D17 opened all four edges, and the anti-boomerang property turned out to be arithmetic already in the contract (§2) |
| Six instances on the main machine is a hardware question | It is a BepInEx question: five log files, and an instance with no log file never loads the mod at all (§11) |
| The applied time scale is the world's speed | Asked-for, applied and achieved are three different numbers, and on this rig they differ by an order of magnitude (§7) |

The exit test was not wrong. It measured what it measured, honestly, and its measurements were
the state of a rig at one instant. **The milestone's real lesson is that an exit test measures a
rig and a deployment measures a rig that grows** — and every bar M4 set against a young world is
a bar that expires on its own.

---

## 1. THE GRID AS BUILT, AGAINST THE GRID AS DESIGNED

### 1.1 Splice-anywhere landed as designed. The rectangle retired its own fallback

Design Question 5 gave insertion a position: the claim carries an advisory `insertAfterSlot`, the
relay honours a position that exists, **and falls back to the tail**. The first two shipped
verbatim. The third does not exist, because a rectangle has no tail — the M3 ring's tail was the
end of one ordered list, and a grid has one list per row and one per column.

`contract-b-m4.md` §7.2 rule 6 is the generalization that replaced it: **fill the first hole in
structural order (`row` ascending, then `col` ascending), else grow the shorter axis.** It keeps
the intent — a peer with no opinion gets some placement, deterministically, with no operator —
and on a one-row map it reduces to exactly the old tail rule. The evidence that it is the right
generalization is that **it reproduces the exit test's 3×2 shape from six opinion-free claims**;
`go/internal/relay/grid_test.go` walks that table.

This is the shape of most of the grid work: the design's *intent* survived, and one clause of its
*mechanism* turned out to have been written for a line.

### 1.2 Route-around per axis, and the equivalence that made a grid viable

D12's claim — that a hole and a dark slot are indistinguishable to a router, so route-around is
the prerequisite for a grid with holes — held exactly. The relay walks east along the row for
the east lane and north up the column for the north lane, and each lane closes only when its
whole row or column holds no deliverable slot. Nothing in the walk knows whether a missing peer
is missing because it died or because it was never there.

**The exit test proved it across the LAN, which the design had not asked for.** Phase 4 hard-killed
local slot 5 mid-column; slot 4 bypassed it and exported east to slot 6 **on the second
computer**. The healed lane was the cross-machine one, not a local one, so the walk was proven
against a real network hop rather than against loopback.

### 1.3 The degenerate closure is a pass, and an implementation that "fixes" it is broken

The one exit-test condition M4 had to rewrite before the run is the most transferable result in
this section. Part 2's original conditions required the north lane to re-pair and required no
export edge to close. **Both are unsatisfiable on a map of height 2**: column 1 holds slot 2 and
slot 5, and a lane needs a third party to route around a gap. The amendment of 2026-08-05
replaced them with the correct degenerate assertions — the width axis routes around the dead
slot, and **slot 2 closes its north lane with `no_peer`** — and the run produced exactly that.

The rule, stated generally: **route-around's correct behaviour on an axis of length 2 is
closure, and closure there is evidence the walk is right.** An implementation that re-pairs a
two-slot column is re-pairing a lane to its own source. The full north re-pair is a 3×3 test and
remains unrun; it is not a gap in the grid's proof, it is a different assertion.

### 1.4 What the exit test asserted once, and the deployment has since made routine

Reclaim-in-place was asserted once in the exit test (phase 4's splice-back of slot 5, same slot
number, same coordinate, no operator action). The deployment has since exercised it across three
unplanned or rolling occasions and fifteen individual sidecar restarts, and the record is uniform:

| Occasion | Result |
|---|---|
| Two host reboots (2026-08-08, 2026-08-09) | All five local sidecars reclaimed slot and coordinate; **all five journals replayed with zero discarded bytes and no `level=ERROR` line**. On 2026-08-09 custody recovered inbound 0/0/37/15/12 with outbound 0 |
| The heartbeat/cadence rolling restart (2026-08-10, 15:35Z–15:59Z) | Five sidecars restarted one at a time, every one `reason=reclaimed` at its own coordinate, **journal bytes discarded: 0 on all five**, all eight lanes reopened each time, and the map never left 24/24 `peer_live` in any sample |
| The far end's two headless flips | The far *sidecar* never left the relay at all — `relay.log` records no `client connected` and no `client gone` for it in either window — so only the mod bounced, and the map read it as an ordinary `peer_mod_absent` flap |

Two properties fall out that no single test could have shown. **A mod-absent flap and a
peer-absent flap are different events on the map**, and the deployment produces the first
constantly and the second almost never: a mod bounce drops that slot's export lanes off the list
and closes the lanes into it with `peer_mod_absent`, while the slot reservation, the Contract B
session and the journal are never in question. And **exactly-once holds across a sidecar restart
because the mod reconnects on the next generation of its own session** — `gen=2` on four slots,
`gen=3` on one, in the rolling restart above.

### 1.5 The corner rule and the second band cost what the design said they would

Two comparisons per organism per tick, on a test that already ran every `FixedUpdate`. Neither
the exit test nor any subsequent reading has attributed a cost to them, and A26 generalized the
rule from "E or N" to any declared export set before D17 needed it — which is why opening all
four edges required no new rule at the corner, only more corners (§2.1).

---

## 2. TWO-WAY LANES: WHAT D8'S ONE-WAY RULE WAS ACTUALLY BUYING (D17–D19)

D17 was ratified on 2026-08-07 against the running deployment, not against a design document,
and it is the clearest case in the project of a **live rig retiring a rule that a static
argument had justified**. The rule was D8's: out and in are different doors, so an organism comes
home only by traversing the whole cycle. The price was that half of every world's border was
dead geometry — an organism walking west was not a migrant, and D10's wrap teleported it to the
antipode, which reads to a player as a wall inside a world advertised as continuous.

### 2.1 Anti-boomerang was already arithmetic, and nobody had noticed

The finding that made D17 cheap: **the three protections M4 already shipped are individually
sufficient against an instantaneous boomerang, and D8's one-way rule was buying nothing they did
not already buy.**

1. An arrival is inset past **every** capture band on **both** axes (`contract-a.md` §4.3, A28 —
   which clamps the free coordinate too, so no arrival can land in a band).
2. The entry-immunity window keeps the spawn and the next export separable events.
3. The outward-velocity test means **an ordinary arrival, which by construction arrives
   travelling inward, is never in the band of the edge it landed behind.**

Point 3 is the load-bearing one and it is pure arithmetic: the sign of one dot product, holding on
the first tick, with no window and no margin. Point 2 is A28's clamp, and point 3's one exception —
the bounce-back, which by construction arrives moving outward — is covered by
`entryImmunitySeconds` (5 simulated seconds), a case `E` and `N` have had since A19. What one-way
lanes additionally prevented was a *round trip* over a whole traverse — and that is exactly the
property D17 gives up deliberately, because a neighbour an organism can visit and return from is
what a map of worlds should mean.

**The version asymmetry across the two contracts is itself a finding about honest versioning.**
Contract A took **no** bump (A38, A41): every field the set touches already accepted all four edge
values, so there was nothing additive to detect. The reasoning against bumping anyway is the sharp
part — it "would be a lie in the one direction that matters", because §3.1 says a receiver detects a
feature by the **presence of a field**, and spending a minor here would imply that
`contract-a/2.2` *cannot* run a two-way map when it demonstrably can. Contract B took a minor to
`contract-b/3.3` (B13–B15), because there the minor is doing real work: a two-way map genuinely
cannot function against a relay that does not compute the reverse walks. **Same decision, opposite
answers, one rule** — and the rule is that a version states a capability, never an occasion.

**A design that generalizes without a wire change is the strongest available evidence that the
earlier generalization was right.** A18's plural `exportEdges` is what made D17 free — and A26 is
the accidental half of it: it generalized the corner rule to any declared export set because the
mod had implemented it generally, not because anyone expected a west lane.

The bill D17 does present is on the degenerate axis: **on an axis of length 2 the forward and
reverse lanes name the same peer**, so the 3×2 rig's columns became two-lane pairs and the hop
rate on that axis rose disproportionately. §3 is where that bill was paid.

### 2.2 The exclusion list is an ecology decision, and it made the containment series rise

D18 keeps the seed stock home: `Basic bibite` is never a capture candidate on any edge, matched
on the A34-normalized form, export-side, local, off the wire entirely. The reasoning is
ecological rather than mechanical — the founder species is not an evolutionary result, so moving
it spreads one starting genome across the map and competes for the arrival budget with migrants
that carry information, while keeping it home gives every world a stable resident baseline
against which immigrants are legible on the census.

**The measured consequence contradicted the contract's own prediction, and the contradiction is
the finding.** `contract-a.md` §4.3.1 predicts that a capture band on every edge makes the
square-crossing series *fall*. Measured on the live map on 2026-08-07 over matched
60-simulated-minute windows either side of the rollout, the `E`+`N` rate on the three
undistorted slots went **0.52 / 0.48 / 0.47 → 1.07 / 0.90 / 0.50** per simulated minute, and the
four-edge total is roughly double the old two-edge one. The larger of the two causes is D18
itself: **`Basic bibite` is 30–56% of every world's population**, and an excluded organism walks
past the square edge and is counted as a crossing where it used to be counted as an export.
§4.3.1 lists the exclusion list first among its residue cases; it did not anticipate the residue
being half the population.

Two rules for anyone reading these series afterwards. **Read `crossings` against the exclusion
list before reading it as a containment failure.** And the asymmetry the policy creates is a
documented feature: "who lives here" and "who crossed" now legitimately disagree, because the
census reports an excluded species normally and the migration ledger never will. A reader must
not reconcile them. On the species view the founder stock therefore reads as a **large resident
population that is zero percent of every lane's traffic** — permanently badged `endemic`, which is
the ecology decision made visible, and simultaneously the cheapest possible check that the policy is
running: zero `Basic bibite` in `/api/hops`, a steady trickle of `[M4-D18] event=EXCLUDED` in every
game log, and neither one an error.

D18 also made D10's wrap load-bearing again. An excluded organism in an outward band is
contained by the vanilla wrap and by nothing else, so the wrap that M2 turned off and M3 turned
back on is now the containment mechanism for a third distinct population.

### 2.3 Hop animation: an anonymous pulse was a lie the page had been telling

D19 animates the species glyph along the lane a real migration landed on, fed by a bounded
recent-hops feed the archive builds from envelope copies it already holds (B14). The finding is not
the animation, it is what shipping it removed. The map had drawn an **ambient pulse** per lane at a
rate derived from `recentHops` — and a pulse is anonymous: it says traffic is flowing and nothing
about what is flowing. Once real hops travelled the same path, the ambient pulse was **removed
outright** rather than kept beside them (B17, one minor later), so that **the only thing that ever
moves along a lane is a real crossing this archive was copied on**.

That is a rule about operator surfaces worth carrying forward: two things that move identically
on one page must not mean different things, and the decoration is the one that goes. It also
turned the page into the cheapest possible verification of the two decisions above — two-way
lanes show as glyphs travelling both ways along one arrow, and the exclusion shows as a species
that is everywhere in the census and never on a lane.

---

## 3. ARRIVAL PACING: THE DAM, AND HOW THE GRID SPENT THE RING'S MARGIN

The owner named the mechanism before M4 built anything: a sleeping machine **dammed the flow**,
queued custody deliveries plus the west neighbour's contained export pile released together at
wake, and the entry edge took the whole dam at once. That diagnosis was correct and it has never
needed revision. **What did not survive is the number chosen from it.**

| Date | `inboundRatePerSimMinute` / `inboundRateBurst` | Why |
|---|---|---|
| M4 design | **2.0 / 5** | T1 measured 0.4 arrivals per simulated minute into slot 1. Five times the measured natural rate was the margin, so ordinary traffic is never held back (`contract-a.md` §7.5) |
| 2026-08-07 | **12.0 / 15** | The grid gave every slot a second inbound lane and doubled the map. 35 hours of the deployment measured a **median of 1.19 and a p95 of 2.21** arrivals per simulated minute per slot — **12% of all slot-samples above a limit whose entire purpose is to be above them**, 20% and 26% on slots 3 and 4, and slot 4's paced backlog pinned at `inboundQueueMax` for a quarter of the run |
| 2026-08-07, same day | **100.0 / 50** | 12.0 was a projection made *before* two-way lanes ran. Once they ran, slot 3 sat at a `pacedDepth` of 16 under it — §7.5's own verdict a second time |

Three findings, in ascending order of transferability.

**A limit sized from projected offered load moves whenever the topology does.** 2.0 was reasoned,
not guessed; the reasoning was five-times-measured-rate, and it was applied to a one-way ring.
Every step of that reasoning survived and the answer still went wrong by a factor of fifty,
because the *measurement* was of a topology M4 then changed twice.

**So the final raise changed the sizing rule, not the number.** 100.0 is not five times anything
measured. It is sized from **A29's ingest ceiling** — the mod applies at most 4 spawns per
`FixedUpdate`, which works out at roughly **12 000 applications per simulated minute** — so
100.0 sits two orders *below* the mod's own budget and two orders *above* the measured offered
load, and the thing that actually protects a world is the budget rather than the limit. The
pacer's job reverts to what it was built for: spreading a genuine dam. A 900-organism backlog
still takes 9 simulated minutes to drain rather than arriving in one breath.

The retained sweep from the first raise is the useful curve: as the limit goes 2 / 4 / 6 / 8 / 12,
the share of throttled samples falls **57% → 12.5% → 0.6% → 0.4% → 0.3%** — flat by about 6 — and
what is left at the flat end is genuine dam events, the largest a far-end dropout at **18.5
arrivals per simulated minute**. A limit chosen anywhere on the flat part of that curve would have
been defensible; **2.0 was chosen from a point measurement rather than from the curve.**

**§7.5's own diagnostic is the reusable instrument.** *A depth that never falls names a limit set
too low.* It fired twice, at 2.0 and at 12.0, and both times it was right before any analysis
was. It is worth more than the numbers it produced. Its converse is also now documented, because
the deployment produced it: **a `pacedDepth` that will not fall is more often a story about the
traffic than about the rate** — the ~40 hops/min bounce loop of §9.2 pinned two slots at
`inboundQueueMax` while every other slot drained to 0, and raising the rate there would only have
pumped the loop faster. The diagnostic order is `pacedDepth` per slot, then that slot's
`bounceBack=true` share, then the rate.

Two smaller results came with it. The knob was compiled: `inboundRatePerSimMinute` was reachable
only by editing Go source, and **a tunable an operator cannot retune from the metric that
measures it is not a tunable** — it gained `--inbound-rate` and `--inbound-burst`, following
`holdTimeoutMs`'s precedent. And because a default that moves twice in a day is not what any
particular slot is running, each sidecar now **publishes** its own rate and burst on the peer
stats block (B16), which is the general form: *a default is not an observation.*

The pacer's clock stayed in simulated time throughout, deliberately. It is what stops a flood
entering a world that cannot run it, and it is why a healthy fast world drains a dam
proportionally faster.

---

## 4. THE SLOT-6 INCIDENTS — SUMMARIZED, NOT RESTATED

`m4_livelock_findings.md` is the record and it is complete. The one-paragraph version, and then
what the milestone should take from it.

Slot 6 held at ~1 TPS for over an hour on 2026-08-06 while every process, socket and contract
rule involved was individually behaving as specified. An arrival flood (~10 786 organisms
accepted in three hours) filled the paced backlog to `inboundQueueMax` (64); the mod drained the
whole queue in one `FixedUpdate`, blew Contract A's 3 500 ms heartbeat deadline, took a `4004`,
and the sidecar then replayed the same 64 un-ACKed deliveries on every reconnect — eleven
generations, `count=64 pacedDepth=64` identical each time — with 203 735 `duplicate
MIGRATION_PAYLOAD` frames each paying a full multi-MiB decode before the dedup lookup. Eight
defences landed as A29 and B8: an ingest budget on the mod (≤4 spawns / ~8 ms per `FixedUpdate`),
a `migrationId` ledger that survives socket reconnects and is cleared only on `sessionId` change,
a quiet-mod delivery gate, a pacing bucket that resets only on a new session, an escalating
replay delay, dedup before decode, and a re-forward backoff toward a live destination. **The
second act rewrote the compute story**: save forensics found 1 081 meat pellets stacked in a
30-unit blob at the world origin — the decayed corpses of the flood's starved migrants,
apparently position-clamped to (0,0) when organisms die out of bounds — costing PhysX ≈580 000
contact pairs every physics step. The remedy was save surgery, not code.

Three things the milestone takes from it.

**Five correct rules composed into one wrong system, and each fix is a break in one link.** The
diagnostic signature is worth more than any single defence: *compute-bound game process + idle
sidecar + healthy LAN + `count=<N> pacedDepth=<N>` repeating verbatim across reconnect
generations.* Every tempting hypothesis — a dead peer, a stuck send queue, a blocking network
call on the tick path — was false, and the architecture's correctness about the last one did not
help.

**The defences passed their first unplanned field test the same night** — the machine slept for
1 h 47 m, the sidecar closed the stale session, the replayed backlog hit the durable dedup
ledger, and paced delivery resumed in two log lines where the pre-A29 system produced an
afternoon-long livelock.

**And that document's own reasoning was superseded by this milestone's other arc.** Its "`4004`
still closes at 3 500 ms — with defence 1 the heartbeat survives any burst, so the timeout is
finally the dead-mod detector it was meant to be" is now false, and by its own argument: the
list considered ingest as the only long-frame source on the main thread, and D14's periodic world
save blocks the same thread for 0.6–9.4 s. 3 500 ms was never yet a dead-mod detector; it became
one at 13 000 ms (§6.3). The sentence stands as the intent, and the number in it is history.

---

## 5. THE LIVING DEPLOYMENT AS A FINDING IN ITSELF

The rig came back up after the exit test and has run the same six worlds ever since. **Running it
as a deployment rather than as a test is itself the most productive single decision of the
milestone**, and what it surfaced was not migration behaviour — the migration layer has held
throughout — but everything that had never been asked to last.

### 5.1 Nothing in the system had ever bounded what it consumed

The root filesystem filled on 2026-08-08 and every genome write in the rig stopped. Measured
after two days up, across five sidecars, the relay and the archive:

| Producer | Before | Bounded now by |
|---|---|---|
| Sidecar journals (5) | 445 / 500 / 905 / 683 / 720 MB — **3.25 GB** for a live set of a few thousand tombstones | `journalCompactMinutes`, default **15 min** (B20) |
| Sidecar and archive logs | **3.5 GB**, unrotated | `logRotateMb × (logKeep + 1)`, default **100 MB × 6** per process |
| Genome scratch files | 15 119 empty `*.json.tmp` from writes that failed on the full disk | the failing write removes its own scratch; the cache sweep collects abandoned ones |

Together **~1.1 GB/h** on a 98 GB volume shared with every other project on the machine. It is
**0.30 GB/h** now, of which only 78 MB/h is growth nothing reclaims.

Two mechanisms are worth carrying beyond this project. `internal/journal` compacted at `Open` and
nowhere else, so a process that stayed up accumulated every record it had ever written, payload
included — and `PurgeExpired` made it *worse*, because dropping a tombstone appends a purge
record, so cleanup grew the file. A compaction never reads the file it replaces (the in-memory
state map **is** the compacted content), so it costs one pass over the live entries and runs in
milliseconds; the fix was a timer, not an algorithm. And the log was a **shell append redirect**,
which has no size — every process now receives its own log path so it can rotate it, with the
shell redirect kept pointed at a `.stderr.log` because a Go panic goes to fd 2 and never passes
through slog.

Risk 6 had anticipated the opposite failure — logs that vanish too soon, because BepInEx
overwrites `LogOutput.log` on every launch and the T1 harvest needed 147 MB of them. **Preservation
and boundedness are the same question asked from two ends, and only one end had been asked.**

### 5.2 The torn append: the same rule, missed twice, in two implementations, a day apart

This is the milestone's most important durability finding and it cost eight hours of history on
five sidecars while every process looked healthy.

When the volume filled, an append to each journal landed **short**: some bytes reached the file,
the rest did not, the call returned an error, and the sidecar did the right thing with the error
and never ACKed. But the half-record stayed in the log, and the next successful append wrote a
whole record straight behind it — producing an unparsable line in the **middle** of the file.
Replay stops at the first unparsable line, because a torn line is only ever supposed to be the
last one. **All five sidecars then ran for eight hours with journals that replayed to 01:07:40 and
no further**, each holding correct state in memory and ACKing correctly. Any restart in those
eight hours would have silently reverted every one of them. That is precisely the loss D2 exists
to make impossible, arriving through the one path that had no rule written for it. The running
rig was recovered by reading the five journals out of `/proc/<pid>/fd/`, the only place the
post-01:07 records still existed.

Two fixes closed it in `internal/journal`: **an append is all-or-nothing** (truncate back to the
pre-write length on any write error — the caller still gets the error and still must not ACK, and
the failure now costs one record instead of every record behind it), and **a damaged journal is
loud** (`Journal.Discarded()` reports the bytes replay threw away behind a torn line, counting
only complete records, logged at **error** on startup).

**Then the archive's ledger turned out to be a second implementation of the same discipline, and
it got neither fix.** `migrations.jsonl` still carries that night's splice — **776 bytes at line
874 163**, the head of a NACK written at 01:21:49 joined to the tail of a record written at
08:12:05 once the disk had room. Replay broke there in silence, exactly as the journals had, while
the status page kept counting forward in memory. It was found and closed on 2026-08-09, and its
second half is deliberately **not** symmetrical with the journal's:

- The journal may **stop** at a damaged line and truncate, because `Open` compacts it from the
  in-memory state map immediately afterwards and loses nothing.
- The ledger has nothing to rebuild from. It **is** the state, §10 makes eviction from it illegal,
  and it will carry that line for the life of the deployment. So replay **skips** the line and
  keeps every record behind it, reports the damaged line itself rather than the history behind it,
  and carries `ledgerSkippedLines` on the status page, in `/api/status` and in `ringstat`.

**A durability rule proven in one implementation is not proven in the system.** The observation
that would have caught it — a replay compared against `wc -l` — costs one command.

The repaired replay met the living ledger for the first time on 2026-08-10 at 19:08Z and did
exactly what it was written to do: one line, 776 bytes, **3 700 672 records kept, of which
188 006 were behind the splice and unreachable before**. The arithmetic closed to the byte against
`wc -l`, and it exposed a counting subtlety worth keeping: `ledgerRecords` runs **below** the file
because a `GENOME` append is counted at boot replay and never during the run, so a shortfall
computed from the counter and the file is **not a loss**. Size the ledger from the file.

### 5.3 The reboot is the only observation that replays a journal

This is the epistemic finding of the milestone. The all-or-nothing append rule is easy to state
and impossible to verify in normal running: a torn append is invisible until something replays
the journal, and on a deployment whose whole value is staying up, that means a real restart and
nothing less.

The two host reboots of 2026-08-08 and 2026-08-09 supplied exactly that, and **both bring-ups
replayed all five journals with zero discarded bytes and zero `level=ERROR` lines** —
`Journal.Discarded()` is logged at error precisely so a non-zero result cannot be missed, and it
stayed silent. On 2026-08-09 custody recovered inbound 0/0/37/15/12 with outbound 0 on every
slot, and `pacedDepth` peaked at 23 on slot 3 while the replayed entries drained, then fell to 0.
The same two bring-ups replayed the **damaged** archive ledger and said nothing, which is how the
second defect was found.

The bring-up itself became a documented seven-step hand procedure (`dev_environment.md`,
*Bringing it back after a reboot*), and its steps are findings rather than instructions:

1. **The volume must be mounted first.** `/mnt/wsl` is a tmpfs, so an unmounted data disk leaves
   the repo symlink dangling and nothing resolves. There is deliberately no automation, because a
   script that created the directory would put the custody journals on a tmpfs a restart erases.
2. **Stale pid files survive a reboot** and pid reuse makes `down` dangerous — check liveness
   before deleting.
3. **Verify the Windows plumbing read-only.** The `8795` portproxy and firewall rule survive a
   reboot if the WSL address has not moved. The address did not move across either reboot.
   Re-running the elevated commands without a check drops the forward while they run. `lanhost`
   is the read-only check.
4. **Expect exactly one game to come up starved of a log file**, and fix that one instance
   (§11.1). It happened on both reboots, on different slots.
5. **Read the time scale of every instance, not only a restarted one** (§7).
6. **Relaunch the collector by hand** (§5.4).

### 5.4 The collector, and what an observation loop costs

The baseline collector is a read-only loop launched on 2026-08-07 that copies the five world
saves and curls `/api/status` every ~5 minutes. It is the source of most of this milestone's
longitudinal numbers — the `genomeGaps` series, the save-size series, the applied-versus-achieved
speed table — and none of them existed before it.

Three operational facts about it are findings in their own right. **Every reboot kills it and
nothing restarts it**, which is why it has gaps and why the M5 risk about unattended operation
cites it. It costs **19 MB/h = 0.5 GB/day**, which is a fifth of the whole deployment's monotone
growth. And it is **gitignored inside a tracked tree** — `e2e/baselines/` itself is tracked — so
a single `git add -A` would commit it and keep committing it; it held 208 snapshots, 1 262 files
and 375 MB when measured on 2026-08-09. That is the concrete reason this repo stages specific
paths.

What still grows forever, at six live slots, is worth preserving as the sizing arithmetic:
genomes **58 MB/h**, the ledger **19 MB/h** at ~540 crossings/min, metrics 0.7 MB/h, the
collector 19 MB/h — **97 MB/h = 2.3 GB/day ≈ 70 GB/month**, roughly three months of headroom on
the 251 GB volume, after which somebody has to decide what the archive is for. No rule in the
system will ever shrink the three durable files.

### 5.5 Three rules a deployment adds that a test harness never needed

**Only one rig may run at a time, and one is running.** Every rig script in the checkout uses the
same fixed ports and the same pid directory, so a second rig — *or a second agent working in this
repo* — silently attaches to or fights over the first one's processes, and the historical rigs
collide head-on because M2 and M3 used overlapping ports. The pre-flight is a listener check. This
is not a design flaw so much as the moment a repo of test scripts acquired a production tenant.

**A cumulative record needs a lower time bound before it can be asserted on.** The archive ledger
never evicts, so a harness wait of the form *"until a slot 2 → slot 3 record exists"* **succeeded
instantly against an hour-old record and reported a crossing that had not happened yet.** Every
wait in the LAN rigs is now filtered by a timestamp taken when the wait starts. It matters most on
the LAN rigs precisely because the archive is the **only** witness to a hop that lands on the other
computer.

**A stale forwarding rule that mostly works is worse than one that fails.** The M3-era portproxy on
`8790` happened to point at the live WSL address and therefore *mostly worked* — while `8790` is
slot 4's Contract A port under the M4 port plan, so the moment the WSL address moved it would
swallow slot 4's mod connection and the map would form one slot short with the reason
`peer_mod_absent`, **which reads like a mod bug**. The rigs now read the Windows listener table and
refuse to start rather than diagnose it later. The general rule: **a shared resource whose
misconfiguration is masked by luck should be turned into a start-up refusal, not a note.**

---

## 6. THE SAVE STALL

The full analytic record is the first watch item in `dev_environment.md`, *The living deployment →
Watch items*, and the escalation history is in `m4_considerations.md` Risk 3. Neither is repeated
here. What follows is the arc and its results.

### 6.1 The budget's history

D14's bar was 2 000 ms, set by the owner on 2026-08-05 to select the simple synchronous save. The
exit test measured **241–538 ms at five concurrent worlds** and the deliverable was recorded as
passed. **No generation since has held it.** Across the eleven preserved `[M4-SAVE]` generations —
8 087 saves, 8 072 of them periodic — `event=BUDGET_EXCEEDED` runs at **17%**, pooled median stall
**1 327 ms**, maximum **5 163 ms**; the bad nights ran 55–95% per slot, and the thirteen-hour
×100 generation produced maxima of 4 311 / 5 386 / 5 870 / 9 115 / 9 427 ms.

For four days the reading was "the worlds are not growing, so the variable is the host". The first
half of that was true in the only sense being measured — the save **zips** stayed at 210–570 KB
while population moved — and **that is exactly why it was wrong.**

### 6.2 The answer: cost tracks the uncompressed payload, and that payload is evolutionary history

Re-reading the whole preserved record on 2026-08-10 — **10 946 `event=SAVED` lines joined to
2 160 collector snapshots by exact zip size** — produced three findings.

**The metric was the trap.** `writeMs` is proportional to the payload's **uncompressed** byte
total with an intercept of about zero (620–1 580 ms per raw MB, per hour). `SaveSystem.CreateSave`
deflates every entry at `CompressionLevel.Optimal`, so CPU tracks the bytes going *in*, while
`bytes=` on the log line is the bytes coming *out*. Dividing one by the other measures the
payload's **compressibility**, and that is what moved.

**What grew is `speciesData.json`, and it is not organisms.** It is
`GlobalLineageManager.recordedSpecies` — every species the world has *ever* recorded, not the
living ones — serialized into one Newtonsoft `JObject` **per brain node and per synapse**, through
an uncached `GetFields` per object and an `Enum.Parse` per gene per species. Its cost is
`recordedSpecies × (nodes + synapses + NGene)` and is **independent of population**, which is
precisely why a save costs the same with 7 organisms as with 40:

| World | `speciesData.json` raw | of the raw payload |
|---|---|---|
| `M1-AutoTest.zip`, a fresh world | **5 KB** | 2% of 0.27 MB |
| `M3-Slot1.zip`, two days old | 44 KB | 14% of 0.58 MB |
| `M4-Slot1.zip`, 2026-08-10 | **677 KB** | **84%** of 0.97 MB |
| `M4-Slot2.zip`, 2026-08-10 | **960 KB** | 43% of 2.51 MB |

Those entries deflate about **13:1**, so 830 KB of new work adds ~65 KB to the zip. **The
raw-to-zip ratio of a save is therefore a clock on the world's age** — 2.4 fresh, 3.2 after two
days, 5.4–7.0 now — and that ratio *is* the "cost per saved kilobyte" curve, upside down. Slot 1
is the proof rather than the puzzle: same population (15 → 18), same file (147 → 192 KB),
**5.8× the write time** (171 → 988 ms), because its uncompressed payload went 0.36 → 0.99 MB while
its zip did not.

**And it is not species count that compounds, it is brains.** `nextSpeciesID` ran 2 162 → 4 832
over three days while `recordedSpecies` — what actually gets serialized — stayed flat at 84–178
per world: `SpeciesTreeCleanup` already discards about **97%** of what the lanes create. What grew
is each kept species' brain, **synapses per species 12.6 → 23.0** in three days. Pruning harder
attacks a term that is already pruned; **the term that is compounding is evolution itself.**

The instrument that confirmed it from the inside is mod `0.6.3`'s `SavePhases` — four Harmony-timed
spans, measurement only. Twenty-one warm saves, 2026-08-10:

| Phase | Median ms | Share of `writeMs` |
|---|---|---|
| **`lineageMs`** (building `speciesData.json`) | **994** | **53%** |
| remainder (bibites, eggs, pellets, templates) | 527 | 28% |
| `zipMs` | 211 | 11% |
| `verifyMs` | 24 | 1% |
| `shotMs` (headless thumbnail) | 10 | 1% |
| `binMs` / `guardMs` | 1 / 0 | 0% |

Slot 1 in one line: **10 organisms, a 149 KB file, an 873 ms stall, of which 676 ms is building
the species history.** Three of the four suspects the watch item had carried are closed by that
table — the headless screenshot is 1%, the species-history guard is 0–1 ms across its three passes
per save, and `data.bin` is 0–2 ms despite being 150 KB because it is `Buffer.BlockCopy` into one
pre-sized array rather than reflection.

### 6.3 The decision: save less often, and tolerate a longer silence

Taken by the owner on 2026-08-10. Both halves landed and **neither lowers the bar**:

| Half | Was | Is |
|---|---|---|
| Save cadence, five local worlds | `saveMinutes=2 saveKeep=4` (a rig override) | **`saveMinutes=10 saveKeep=6`** — the mod's own shipped defaults |
| Contract A heartbeat timeout | `3 500 ms` | **`13 000 ms`** (§20 A45; a §10 default, so **no version bump** — A46) |

The cadence cuts the *number* of exposures by five and costs only a longer worst-case rollback,
which `saveKeep=6` at ten-minute spacing turns into a **deeper** history than `4` at two — an hour
against eight minutes. The timeout removes the *consequence* of the exposures that remain, and the
reasoning behind it is the transferable part: **`3500` was three missed heartbeats plus slack,
which measures whether a process is alive — and the heartbeat is composed on the same Unity main
thread the save blocks.** The deadline was never sized for a save at all. 13 000 ms clears the
worst save ever logged here (9 427 ms) by 3 573 ms and stays under `wsPingIntervalMs` (15 000) so
the informative `4004` still fires first. What it is bought with is later detection of a genuinely
dead mod — ~16 s against ~4.4 s — during which the sidecar still publishes `modConnected: true`,
so no edge closes, no lane re-pairs, and the quiet-mod gate holds that world's arrivals in the
journal, in order, bounded by `inboundQueueMax`.

The dose response that justified it is worth preserving because it is the general shape of "a
budget breach that costs a session": against the old 3 500 ms deadline, **0.2%** of saves under 2 s
were followed by a `4004` within twelve seconds, **1.2%** at 2.5–3 s, **7.0%** at 3–3.5 s, and
**50.8%** of the 63 saves over 3.5 s. The first evidence after the change is one observation of
the thing it was for — slot 2's 4 207 ms save at 15:52:33Z took no `4004` — and one is not a rate.

**Why 13 000 and not 12 000 is the transferable half of the sizing.** 12 000 would have cleared the
worst logged save by 27%; 13 000 clears it by 38%. The argument for the wider margin is that
**the tail is not stationary** — the per-kilobyte cost of a save had already roughly doubled once, on
a change (headless at ×100) that was expected to make it cheaper — so "a margin thinner than a third
of the worst case buys the next regime change the same bug back". 15 000 was rejected for a different
reason: it collides with `wsPingIntervalMs`, and **both liveness paths close with the same code
`4004`**, so an operator's `4004` would become ambiguous when only the heartbeat path logs the
`silentFor` the dose response was matched on. One deadline pair inverts as a result — the ordering is
now hold delivery at 1 500, stop the pacing clock at 10 000, close the session at 13 000, ping the
transport at 15 000 — and `heartbeatDeliveryGraceMs` was deliberately **not** raised with the
timeout, because raising it would re-create exactly the force-feeding A29 removed.

One smaller correction the arc produced, recorded because the documentation had it backwards:
**`MULTIVERSE_SAVE_MINUTES` is wall-clock, not simulated** (the timer reads
`Time.realtimeSinceStartup`), so a world at ×20 saves every 10 real minutes and not every 10
simulated ones. The difference is a factor of the time scale, on the one setting the cadence
decision turns on.

### 6.4 What remains open

The **~40% host residual**. Of the change from the exit test to now, in log terms, ~60% is the
payload growing and ~40% is a genuine per-byte slowdown of the host. That residual is real,
non-monotone, **common-mode across all five worlds hour by hour** (which is what made "the host"
the natural first reading), **not reset by a game restart**, and *partly* reset by a host reboot —
926 → 620 ms per raw MB in the four hours after the 2026-08-09 reboot, climbing back to ~850 over
the next twenty. That is where the folk observation "a reboot gives back 40%" actually lives. Two
things it is **not**: not save concurrency between the five instances (de-trended, a save with
three neighbours saving within ±10 s costs 0.99× one with none, across 10 946 saves), and not host
simulation demand (log-log correlation against Σ time-scale × population is **−0.26**, the wrong
sign). It is bounded to where it can hide — `zipMs` is a tenth of the cost, so the residual is CPU
and allocation, not disk — and managed-heap/GC state with process uptime is the surviving suspect.

And the structural result: **the stall will keep growing on its own**, at unchanged population and
unchanged file size, because `recordedSpecies` and its brains only accumulate. D14's 2 000 ms was
set against worlds that were seconds old. Risk 3's remaining escalation — a save path that does
not block the tick — stays unspent, and it is now the only lever that touches the *stall* rather
than the cost.

---

## 7. SPEED: ASKED FOR, APPLIED, AND ACHIEVED ARE THREE DIFFERENT NUMBERS

### 7.1 The wire carried one of the three, and was silent about the gap

`timeScale` on Contract A is the mod copying `Time.timeScale` into the heartbeat — the speed the
game is **applying**. Until 2026-08-10 that was the only figure the operator surface had. The
finding is that it is not the world's speed and can be far from it in both directions.

Measured at 00:47Z on 2026-08-10 at a ×5 target, the five local worlds reported `5` and advanced
4.94–5.00× per wall second — the two figures agree when the host can meet the ask. Measured over
16 samples after the ×100 switch:

| Slot | Applied (`timeScale`) | Achieved per wall second |
|---|---|---|
| 1 (5–10 organisms) | 45–100, median **100** | 27–68, median **42** |
| 2 | 11–36, median 18 | 4.6–12, median **6.6** |
| 3 | 13–61, median 25 | 6.1–18, median **9.4** |
| 4 | 22–44, median 30 | 7.8–15, median **11** |
| 5 | 16–81, median 28 | 8.6–25, median **11** |

**Only the smallest world reports its target, and even it achieves under half of it.** So: a
target is an ask, an applied `timeScale` is what the governor allowed, and only
Δ`simulatedTime` / Δ wall is what happened.

There is a second, quieter finding here: **the applied scale drifts on its own, hours after a
clean bring-up.** At 20:21Z on 2026-08-09 slot 2 reported ×34.7 while the other four reported ×5,
with no restart involved; a plain `send 2 timescale 5` corrected it and it held. Three separate
episodes widened the scope from "the instance you just restarted" to "any instance at any time",
and a world running seven times too fast inflates **every per-simulated-minute series it
produces** — which is not decorative, because D20 sized the pacing limit from exactly those
series, twice.

### 7.2 The min-FPS servo, and why it is a patch rather than a setting

`TimeController.CheckMinFPS` is a **servo, not a floor**: on every `TimeKeeper` update it drives
`engineTimeScale` toward whatever value holds the frame rate at `UserSettings.minimumFPS`
(default **15**). **It still runs headless** — the gate is
`Application.isFocused || !UserSettings.ignoreMinOnUnfocus.val`, and `ignoreMinOnUnfocus` ships
`false`, so the second term is always true and a windowless process, which is never focused, is
governed exactly like a visible one. That is why five headless worlds asked for ×100 and reported
11–81.

Mod `0.6.2` disarms it with a Harmony prefix when the process has no graphics device. Measured
against the two hours immediately before the change, at comparable populations:

| Slot | Servo armed: applied / achieved | Disarmed: applied / achieved |
|---|---|---|
| 1 | 79 / **25.7** | 100 / **45.5** |
| 2 | 14 / **4.9** | 100 / **7.0** |
| 3 | 13 / **4.6** | 100 / **7.3** |
| 4 | 16 / **6.0** | 100 / **6.6** |
| 5 | 19 / **6.4** | 100 / **7.6** |

Local aggregate **48 → 74** simulated seconds per wall second. **The mechanism is not simply "the
clamp was the ceiling"** — achieved was *below* the clamped `engineTimeScale` on slots 2–5 both
before and after, so the frame rate was already binding. The leading explanation is the servo's
own cost: it called `SetValue` on nearly every update, and each call runs `UpdateTimeScale` and
its subscribers. The replacement writes only when the value actually changes.

**Why a patch and not the setting** is the transferable half, and it is a game-engine finding
(§11.3): writing `UserSettings.minimumFPS` below 1 would work, but `IntUserSetting.val` is backed
by **`PlayerPrefs`, one store shared by every instance on the machine and persisted across runs**,
so a headless rig would silently change the owner's own setting for their next drawn session.

### 7.3 The ceiling underneath, which nothing here lifts

`UpdateTimeScale` also pins `Time.maximumDeltaTime` to `Time.fixedDeltaTime` = `1 / simTPS`
(`simTPS` default **30**). One frame therefore advances at most one simulation tick, so **the
achieved rate cannot exceed `fps / simTPS`** however high the target goes. That is why slot 1
reported a clean ×100 and advanced 45×, and why asking for more than the host can render buys
nothing. The honest levers on throughput are per-tick cost (population, brain TPS) and `simTPS`
itself — not the time-scale target.

### 7.4 Headless: validated, and what it did and did not buy

The far end proved the flip **both ways** (2026-08-09 23:15Z → 2026-08-10 00:58Z), which is the
part that matters: each flip is a clean quit plus a start with or without `-Headless`, costs about
**90 s of world downtime** and nothing else, and **it is the same world across both** —
`simulatedTime` and population run continuously, the sidecar holds its slot, session and custody,
and the map never sees a hole. **Nothing on the status page distinguishes a headless slot from a
drawn one, which is the point.** The five local worlds went headless one slot at a time on
2026-08-10; every instance took back the log file its own stop had just freed, so the starvation
trap never fired, and each sidecar logged exactly one `4004` — its own game's.

What headless bought: the GPU, and about a gigabyte of RAM per instance (403–466 MB against
1.27–2.07 GB). What it did **not** buy: a cheaper save. The apparent doubling of "ms per 100 KB"
across the switch is **a 22% metric artifact** — the dropped `img.png` is an already-compressed
PNG that passed the zip at ~1:1, so it was ~75 KB of the *compressed* file and only ~75 KB of a
~1.6 MB *uncompressed* payload; measured against the uncompressed payload the switch moved the
cost per byte **not at all** (795 → 800 ms per raw MB). Normalise by the uncompressed total.

The aggregate result of headless plus ×100 is honest and smaller than the target suggests: the
five local worlds advance **63–124 simulated seconds per wall second together, median 80**,
against **24** at ×5 drawn — a real 3–5× on the whole map, not the 20× the number names. And the
worlds *grew* under the faster clock rather than thinning out (population 360 across six worlds
thirteen hours in), while achieved rates fell as they grew, because a bigger world costs more per
tick.

### 7.5 The readout, and the rule it obeys

Since 2026-08-10 the page shows both numbers per world as `×100 → ~×12`, with `achievedTimeScale`
and `achievedSpanMs` per slot on `/api/status` and a `speed→got` column in `ringstat`. **No wire
field was added**: it is Δ`simulatedTime` / Δ`statsAsOfMs` over the last minute, both of which
`PEER_STATUS` has always carried, so it is §10.1 rule 2's *derived, and marked as derived*. Absent
means the archive has not watched that world long enough — which is deliberately not the same fact
as a peer refusing to say.

**And publishing the applied scale at all (B16) closed a gap that had been open since M4 shipped:
`pacedDepth` had been on the peer stats block from the start and had never been readable.** A queue
is only deep against the cap it is queued behind, that cap is counted in *simulated* minutes, and
neither the cap nor the speed that spends it was published — so an operator reading `pacedDepth: 12`
could not tell a world nine seconds behind from one six minutes behind, and every analysis that
needed the number had to recover a time scale from somewhere else. **A queue depth without its rate
and its clock is not a measurement**, and it took a deployment to notice that the field had been
decorative for a milestone.

That is the general pattern of every observability addition in this milestone: **derive from what
the wire already carries, mark it as derived, and render an unknown as unknown.** It costs ~393
bytes (+2.7%) per `metrics.jsonl` sample and no protocol change at all.

---

## 8. SPECIES IDENTITY, THE CENSUS, AND WORLD SETTINGS

Three additive sets shipped between the exit test and M5, all tagged `[M5-*]` in the mod — which
names the milestone *window* they landed in, not a to-do. Together they changed what the wire
carries about an organism and what the archive can say about a map, without a single major bump.

### 8.1 Identity: a `speciesID` is local, and a name is the only portable handle

**The defect first.** A `.bb8` carries `genes.speciesID` and no name, and that id is drawn from the
**world-local** counter `Species.speciesMaxID` — yet `BibiteGenes.LoadState` looks a foreign id up
in the local registry anyway. Three outcomes follow: a **collision**, where the migrant silently
joins an unrelated local species with no genetic check; a **miss** resolved by nearest-species
classification; or a **miss** that produces an orphan **root** with a fresh random name, unprunable
because a parentless species is always kept. **About 85% of every rig world's species tree was
orphan roots of the third kind** — and of the two failures, the collision is the worse one, because
an orphan root is merely uninformative while a collision is silently wrong, and **no log line marked
either.**

A30–A34 close it. An OPTIONAL `species` block on `MIGRATE_OUT`, `MIGRATE_IN` and
`MIGRATION_PAYLOAD`, opaque end to end on Contract B (B9–B10), carrying **names, not ids** —
because "an id is world-local by construction, and that is the whole defect". The importing mod
resolves **by name** and rewrites `genes.speciesID` **before** the restore rather than correcting a
live organism's membership afterwards, which makes the collision row not rarer but **unreachable**.
The accepted cost is stated as policy: *a name match is the same species*, which admits a rare
false merge because both name halves come from one shared list with uniqueness checked only within
a world — **observed at 1 to 4 names per world pair**, against an alternative (genetic distance
measured across worlds) that is a much larger and much less honest guess.

The rule that generalized furthest is the **absent-block rule** (A32): **no import path may keep a
foreign `speciesID`.** The mod must *remove* the key rather than substitute a sentinel, because
`LoadState` guards on its presence and because **any integer is a claim that could come true** —
ids come from a monotonic counter, so an id chosen today to mean "absent" is issued for real later
and the defect returns silently in a world that has run long enough to reach it. A resolve or
create failure also falls back here and the organism still spawns: **custody outranks bookkeeping.**
A policy that cannot identify its subject must not guess, and the same sentence later became D18's
third structural property.

**A34 is the finding a real deployment produces and a test never would.** About **2% of the game's
generated name halves carry an edge space**, which the wire's own name rule refuses — so those
organisms were migrating with **no species block at all**, silently. The fix normalizes at the
source, before validation (trim the edges, collapse internal runs to one U+0020), and compares on
the normalized form so a repaired name still matches a raw-form species the destination grew for
itself. Only the comparison is normalized; no local record is ever rewritten. **It added no field
and relaxed no rule, so no version moved and no Go binary changed** — and the exclusion list of
D18 works at all only because it matches on that same normalized form.

### 8.2 The census, and the arc of the minors

A35–A37 put an OPTIONAL species census on `HEARTBEAT`, carried blind through the sidecar and relay
(B11) and rendered on the page (B12). The gap it closed is worth stating exactly, because it is a
category error the ledger invited: the status page described six worlds and every number on it was a
stat, but **what lives there was not on it** — and the migration ledger cannot answer it, because a
ledger holds migrants and their ancestors and never a peer's resident population. Asking it anyway
returns "which species have crossed into slot 4", **a different question with a plausible-looking
answer, which is the worst kind of wrong number to put on a page.**

Three rules in it are worth keeping. **The census carries names raw and A34 does not apply to it**
(A36) — a census reports on a world's own records rather than offering a handle another world will
resolve against — and that call was made on a measurement, not a preference: a sweep of the rig's
registries found **~13% of the species names standing in a world's registry** carry stray
whitespace, and in one sampled world **those species accounted for 27% of the living population**,
because abundance and spelling are unrelated and the most numerous species happened to carry a
doubled space. Dropping them would have deleted a quarter of a world from the page; normalizing them
would have invented a merge no world performed. **A failed census build costs the census, never the
heartbeat**: the series is silent in normal operation and emits at most one rate-limited warning a
minute naming names it cannot carry, marking the rest `truncated=true`. And **the reconciliation is
defined slack, never a rescale** — `Σbibites ≤ population` and `Σeggs ≤ eggCount`, with a shortfall
being a fact rather than an error.

The shared philosophy of both sets, in one sentence the contracts state twice: **a label is never
worth an organism, a session, or a heartbeat.**

The compatibility arc across these sets is a finding in itself, and it has two halves.

**Three minors were tolerated, deliberately.** The 2026-08-07 species rollout left the far end on
the previous minor **on purpose**, and it kept exchanging organisms in both directions with no
close — the compatibility rule demonstrated rather than asserted. Compatibility is on the major
alone; a minor is a capability statement, never a negotiation; detect a feature by the presence of
its field, never by arithmetic on the minor.

**The fourth was not, and the difference between the two kinds of staleness is the lesson** (§9.2).

### 8.3 Settings: read-only, and a control surface is not an extension of them

A42–A44 put five observable settings on `CONFIG_UPDATE` — save interval, keep count,
save-on-quit, world wrapping and the exclusion list, beside the mod and protocol versions — and
B18–B19 carried them to a Settings view. **The gap: every fact the operator surface held was a
measurement or a receipt, and nothing said what a world was configured to do — which is what makes
a measurement readable.** Three concrete blind spots named it. A quiet lane could mean no migrants
or a whole population excluded, and under the shipped default that is not a corner case. A missing
`lastSave` could mean a world has not saved yet or that **the timer is off**, and those have
opposite consequences for a hard stop — the failure D14 exists for, and the one T1 paid twelve hours
to. And organisms never leaving could mean the wrap is off, which D10 forbids and nothing downstream
could check.

Four rules landed with them. **A43: these are read-only, and a control surface is not an extension
of them** — publishing what a world was told to do is not the same act as telling it, and the
amendment exists so nobody has to reconstruct why: a write path needs its own answers to
authentication, cross-machine authorization under D9, idempotency against a world load, a write
whose target mod is disconnected, and an audit trail, *"because a setting that changed itself is the
worst thing to find in an incident"* — **none of which an OPTIONAL field on a handshake answers,
and reusing these fields would mean answering them by accident.** **A settings row is validated but
never fatal**: a malformed row is stripped before the handshake proceeds, because closing a session
over an observability field would cost the whole session. **A world whose mod predates the set
renders `?` in every cell** — honest, never a wrong number, and specifically never the value the
game ships with: *`saveMinutes` must never render as `10` because that is what the mod ships with*,
since a page that does it claims a world is being saved when its timer may be off. And **two
readings are facts rather than gaps**: `saveMinutes: 0` means the timer is off and
`migrationExclude: []` means the policy is off, which is `timeScale: 0`'s rule applied twice more.

Publishing the negotiated `contract-a` version alongside is the quiet structural win. Four field
groups arrived in three days — the species block, the census, the pacing settings, these — and each
one added a slot state reading "unknown" for a reason no reader could see. **A published protocol
version makes "unknown" self-explaining for every field group, including the ones added after it.**
The Settings view then names its own gap: *"mod 0.6.0, contract-a/2.2 — settings not reported by
this mod"*, the first unknown on the page that explains itself.

One relay-side rule generalizes past this project. B11 asks a relay to store the peer stats block as
**the bytes it arrived as** rather than re-encoding it from a typed model, because **a relay that
re-encodes silently drops every field it does not know about** — which would have deleted the
census, then the pacing settings, then these, each time from the component least likely to be
suspected. A dumb relay has to be dumb about serialization too.

The archive side has one measured cost worth recording: the species aggregate is built during the
ledger replay the archive already performs at startup and maintained per new record after that, so
a half-million-line ledger is never re-read to answer a request; the join happens server-side so
the page and `ringstat` can never disagree about which species is endemic. The settings block
widened each `metrics.jsonl` sample by **~640 bytes (+7.6%)**, which is the kind of cost an
additive field has when a struct is serialized verbatim into a durable file once a minute. B14's
recent-hops feed had to be bounded by both time and count for exactly that reason.

---

## 9. THE FAR END AS AN OPERATIONAL DISCOVERY

D9's rule — the rig never drives the second computer — was M3's, and M4 is where it started
producing findings rather than constraints.

### 9.1 Bundle mechanics, and version pinning as the real gate

The far end gets artifacts, not a build: one zip with `setup-farend.ps1`, a fresh
`BibitesMultiverse.dll`, a cross-compiled `multiverse-sidecar.exe`, and a pinned BepInEx. Two
mechanisms in it earned their place.

**`make-farend-bundle.sh` runs against a live deployment.** It calls `dotnet build` and reads the
result out of `bin/Release/`, deliberately *not* `deploy.sh`, because the copy into
`BepInEx/plugins/` is the only step a running game can block. Building a distributable and
deploying to this machine are separate acts with separate downtime.

**The version gate is a hash pin, not a version string.** The bundle refuses to build when
`setup-farend.ps1`'s `$AssemblySha256` no longer equals `bibites-mod/libs/BibitesAssembly.dll` —
that pin is what stops two different game builds joining one map, and a stale one would let them.
The bundle is tracked in the repo so the second computer takes it out of a checkout; only the
token and the relay's LAN address travel by hand, and neither belongs in the repo.

**Re-taking the bundle is not optional after a `contract-b` minor**, because the zip carries the
sidecar as well as the plugin — see §9.2 for the measured cost of not doing it. And because
`setup-farend.ps1` **generates** the start and stop scripts, a capability added to the template is
a capability the far end does not have until it re-runs the installer: a far end installed before
2026-08-09 has a `start-slot6.ps1` with no `-Headless` switch at all. **The bundle is a version of
the operator's procedure, not only of the binaries.**

One more failure mode belongs to the bundle rather than to the wire: **stop the old far-end sidecar
before unpacking a new one.** Windows locks a running `.exe`, so the copy fails with *Permission
denied* on `multiverse-sidecar.exe` and nothing in the error names the process holding it. It cost
the M4 far-end setup a confusing failure, because an M3 sidecar had been running there since
2026-08-03. It is the far-end twin of "stop the game before you deploy".

### 9.2 Two classes of staleness, and only one of them breaks traffic

This is the most reusable result of the far-end work.

- **A stale *value range* in a field both sides already exchange breaks traffic.**
  `contract-b/3.3` was the first minor a stale peer did not simply tolerate: a pre-`3.3` sidecar
  validates `MIGRATION_PAYLOAD.exitEdge` against `E`/`N` and answers anything else with a
  **permanent** `MALFORMED_MESSAGE` NACK. So an upgraded neighbour's west and south exports are
  refused, bounced home, and re-exported — which pins the exporter's paced queue at
  `inboundQueueMax` and makes it answer its own senders with `OVERLOADED`. Measured on the live map
  with slot 6 stale: **the two lanes into it ran at ~40 hops/min against ~4–6 on every other
  lane, and slots 3 and 4 sat at `pacedDepth` 63–65** while every other slot drained to 0. Nothing
  was lost — a bounce-back is a delivery — but a `3.3` map is not operationally complete until
  every peer is on `3.3`.
- **A stale *absent optional field* only ever costs a number on a page.** Slot 6's lag after that
  cleared was readout-only: `?` in its settings cells while its census kept reporting.

The rule: **re-take the bundle promptly for the first kind, at leisure for the second, and do not
confuse them.** The minor is still not a rejection reason in either case; the incompatibility in
the first is one field's value range.

### 9.3 §20's unknowable-by-design, which is a property and not a gap

Neither the disk-budget work (B20) nor the heartbeat raise (A45) changed a wire field. Therefore
**whether the second computer has applied either of them cannot be determined from here, and no
observation will settle it** — a far end with them and one without are indistinguishable on
Contract B, and both contracts say so as a design property (B20; A46 repeats it for the timeout).
Slot 6's settings block proves its *mod* is at least the settings build and says nothing about its
sidecar.

The operational consequence is a vocabulary rule: treat the far sidecar's state as **unknown
rather than stale**, ask its operator if it matters, and **do not rebuild the bundle to answer the
question** — a fresh bundle answers a different one. It also has a benign corollary: the heartbeat
timeout is per-sidecar and per-mod, each sidecar judging the one mod it serves, so a far end still
on 3 500 ms is a local policy and not a mismatch.

### 9.4 The far end as a control

Because it is one game on its own host, slot 6 is the only clean control the deployment has, and
it has settled two questions the local five could not. It saves a comparable 354–380 KB world in
**567–924 ms** on the mod's shipped ten-minute cadence, which is what took "headless" and "the
save path" off the suspect list in §6. And its own headless window produced the two facts that
generalized: the periodic save is *faster* with nothing to draw on that host, and a custody
replay burst after any world absence throws a single transient `1011: outbound queue full` and
heals itself — the same in both directions, so it is a property of a draining backlog and not of
either mode.

---

## 10. THE PORTAL — SUMMARIZED, NOT RESTATED

`m4_portal_findings.md` is the research record: two orthographic URP cameras, the bundled Shapes
library with all 154 shader variants in the build's *Always Included Shaders* list, retained mode
working and immediate mode not (it needs a `ShapesRenderFeature` on the URP renderer that a mod
cannot add), and `ThicknessSpace.Pixels` as the one-enum answer to an 800× zoom range. Its
recommendation shipped verbatim and nothing in it needed revision.

What M4 adds to it is the closure of its top-ranked risk, which was the one thing static analysis
could not settle: **layers live in prefabs, not in the assembly, so a portal on a layer the camera
does not draw is invisible and nothing writes an error.** Exit-test phase 7 answered it on every
local slot — each logged `[M4-PORTAL] event=BUILT` with its layer, culling mask, shaders and strip
bounds, and each open edge showed a strip — and the owner then ratified the visual result on the
live rig at the shipped `PortalSortingOrder = -50`, which also answered the sorting order and the
zoom legibility. **It was the last item in M4 that only a person could settle**, and it needed no
tuning.

D17 then doubled it without a design change: every edge now draws two lanes, an outer cyan capture
lane and an inner amber arrival lane, so a healthy world shows **eight** `event=SHOWN` lines. That
the visual geometry comes from the same `BorderGeometry` the capture logic uses is why the picture
could not drift from the mechanic when the mechanic changed.

---

## 11. GAME-ENGINE FINDINGS THAT TRANSFER

These are properties of BepInEx, Unity and the shipped game. None is a mod defect, each cost real
diagnosis time, and each will meet the next multi-instance rig.

### 11.1 BepInEx hands out five log files, and an instance with no log file never runs the mod

`DiskLogListener` walks `LogOutput.log`, then `.1` … `.4`, then gives up with *"Skipping log file
creation"*. **The consequence is not a missing log — the mod never loads in that instance.**
Measured in both directions on 2026-08-06: the sixth instance sat at the main menu at 355 MB and
208 s of CPU while the other five were at ~590 MB and ~1 200 s with worlds seeded, and freeing one
log file made that same instance seed in forty seconds while the one that gave up the file went
idle in its place. **Six games run fine on the hardware** — ~550 MB each, 3.3 GB of 62 GB, load
average ~2.4 on 16 cores. Five is the *BepInEx* ceiling, and that is the entire reason slot 6 is
the slot that moves to the second computer.

The same root cause bites a five-game bring-up as a **starvation trap**, because the allocation is
a lock race: one instance can lose it and get no file, coming up `modConnected=false` with
`exportEdges=[]` while its neighbours bypass it with `peer_mod_absent`. It is not slot-specific —
slot 1 on one reboot, slot 2 on the other. **The tell is an absence**: it writes nothing, because
there is no log to write into, and the starved instance sits far below its siblings in memory
because it never seeded a world. Restart that one instance with the rig's own launcher so the
environment matches, and a still-waiting bring-up completes normally.

### 11.2 A new config entry makes every instance rewrite one file, and two of them lose

BepInEx writes the whole config file back on every **new** `Bind`, so a mod release that adds an
entry makes all five instances rewrite `dev.multiverse.bibites.cfg` within seconds of each other.
Two lose, with `IOException: Sharing violation`, and **the symptom does not look like a config
problem**: the game loads, `Chainloader startup complete` appears, dev commands arm, and then
nothing — no world load, no sidecar connection, and the bring-up blocks in `world_ready` for its
full timeout. Measured rolling out `0.6.1`: slots 1 and 2 both lost. **The fix is a restart, not a
repair** — the file is complete after the first winner writes it, so no later `Bind` saves and the
race cannot recur.

The pair with §11.1 is the diagnosis: `modConnected=false` **with** `configuration failed` in a log
is the config race; `modConnected=false` with that string in **none** of the five logs is
starvation. Same symptom, different remedy, and the discriminator is which of the two wrote
anything at all.

### 11.3 `PlayerPrefs` is one store shared by every instance on the machine

And persisted across runs. Any mod setting written through a Unity `UserSetting` therefore leaks
between concurrent instances *and* into the human owner's next session. It is why §7.2's servo
disarm is a Harmony prefix reproducing the `minimumFPS < 1` branch rather than a one-line write to
`UserSettings.minimumFPS`. On a rig with N instances, **a shared persistent store is a shared
mutable global**, and the mod's own rule became: reproduce the behaviour, store nothing.

### 11.4 `LogLikePresenceArray` drifts, a `ushort` underflows, and world saves stop

The most expensive game-side defect the deployment found. Slot 5 lost 27 consecutive periodic
saves and slot 4 lost 19 plus its save-on-quit, every one an `IndexOutOfRangeException` in
`Utility.LogLikeSpeciesDataArray.SavePointToArrays` under `GlobalLineageManager.SaveStateBin`.
**It is not our bug** — the same stack from the same call sites appears under mod `0.4.0`, which
has no species block at all.

The mechanism: `LogLikePresenceArray` describes a species' live history window **twice** — as four
apparition/disappearance indices, and as a running `nPresentAtScale[]` count — and lets the two
drift. `SaveStateBin` sizes its temporary arrays from the count and fills them from the indices,
so a count that reads low overruns. The count goes low because `nPresentAtScale[i]--` fires
without its paired `++` whenever the disappearance front reaches a scale that has never held a
living point: `0 - 1` on a `ushort` is **65535**, saves keep working over-sized, the species stops
being prunable, and then the species is repopulated, the `++` wraps 65535 back to 0, and **every
save from that moment throws**.

**What the multiverse supplies is the traffic**: arrivals that empty a species when they migrate on
and later repopulate it, across hundreds of species per world instead of a handful.
**Fourteen of the 124 species in a live slot-1 world were already carrying a drifted counter** when
`SpeciesHistoryGuard` was first run against it, which is how routine the drift is.

The guard is a Harmony prefix on `GlobalLineageManager.BytesSpace` **and** `.SaveStateBin` — both,
because `SaveableBinStack` sizes the whole buffer from the first and then hands the second a slice
— recomputing `nPresentAtScale` to exactly the window the save loop will walk. The value written
is the game's own (the same quantity `RefreshPresentsFromIndices` computes during a prune merge),
it leaves `nDataAtScale` alone because that field steers `Push`'s cascade, and on a healthy array
it is a no-op. `e2e/species-guard-check.sh` reproduces the incident's stack on demand with the
guard off. **It has cost 0–1 ms per save across its three passes** (§6.2), and no save has logged
`event=FAILED` since it shipped.

### 11.5 A fast quit loses the save receipt, not the save

BepInEx's disk listener loses whatever is still buffered when the process goes, and a local quit
is 3–5 s — short enough to lose the last line. Two of five slots logged no `event=SAVED why=quit`
in the ×100 rollout, and **slot 1's save demonstrably happened anyway**: the zip and its rotation
copy are on disk, written inside the quit window. **On a fast quit the zip is the evidence, not the
log line** — check it there before the rotation prune reaches it, which is what happened to the
other slot's. Do not read a missing receipt as a missing save. The far end's 13-second quit does
not have this problem, which is how the difference was isolated.

### 11.6 The first save of a process carries the JIT of the whole path

`shotMs` reads **483–585 ms on `n=1` and 8–14 ms from `n=2` onward** — a 50× warm-up artifact, and
a trap set for anyone who reads a save taken minutes after a bring-up. Every save-cost reading in
§6 skips `n=1`. The general rule for this rig: **a measurement taken in the first minutes after a
restart is a measurement of a restart.**

### 11.7 A transient condition reported as a permanent fault

`MenuInitializer` builds the procedural sprite atlas asynchronously, and until
`ProceduralSpriteManager.Instance.done` **every bibite spawn throws** out of
`BibiteBody.FinalizeBirth`. `LoadBibiteOrEggFromData` swallows it and returns `null` (M1 §1.2), so
the symptom is a **world that loads empty**, or a migration answered `DESERIALIZE_FAILED` — a
transient startup condition presented as a permanent payload fault, with no stack trace anywhere.
The mod gates on the flag, and **sleeping a fixed number of seconds is not a substitute**, because
the build time scales with the sprite set. The general shape — *an async engine subsystem whose
not-ready state is indistinguishable from a bad input* — is the kind of thing only a rig that
restarts worlds unattended will meet.

### 11.8 Two smaller ones, already known and re-confirmed

Unity's own errors never reach `LogOutput.log` on this install (BepInEx logs *"Unable to start
Unity log writer"* at boot, and the mod's `Application.logMessageReceived` forwarder is armed only
under the auto-test), so **always check the game's own `Player.log` before concluding "no
exceptions"** — it hid the evidence for most of an evening during the slot-6 diagnosis, and it is
why five headless worlds wrote a failed-thumbnail signature into every save without one Unity error
line reaching any of the five logs. And a headless save's thumbnail is blank because
`RenderTexture.GetTemporary` returns a non-null texture whose GPU allocation failed; the save is
whole, `ReadPixels` does not throw, and `img.png` is **3 666 bytes** instead of ~79 KB. The tell is
the file, not the log, and the waste is ~10 ms — cosmetic, not a performance defect, but it bends
every zip-normalised statistic by 22% (§7.4).

---

## 12. OPERATIONAL NUMBERS WORTH PRESERVING

### 12.1 Where the exit test and the deployment disagree

| Quantity | Exit test, 2026-08-06 | Living deployment | Why they differ |
|---|---|---|---|
| Periodic save stall | **241–538 ms**, 0 breaches | median 1 327 ms, 17% of 8 087 saves breach, max **9 427 ms** | The worlds' `speciesData.json` went 5 KB → 677–960 KB. The exit test measured worlds seconds old (§6) |
| Lanes | 12 | **24** since D17 | Every edge became two-way (§2) |
| Arrival pacing | `2.0` / sim-min | `100.0`, burst 50 | Sizing rule changed from offered load to the ingest ceiling (§3) |
| Heartbeat deadline | 3 500 ms | **13 000 ms** | A save blocks the thread that heartbeats (§6.3) |
| Save cadence | 2 min / keep 4 | 10 min / keep 6 | Fewer exposures, deeper history (§6.3) |
| Applied vs achieved speed | not measured | ×100 applied → ×4.6–68 achieved | Servo, then `fps / simTPS` (§7) |
| Bytes the rig writes | not measured | 1.1 GB/h → **0.30 GB/h** | Journal compaction timer, log rotation (§5.1) |

### 12.2 Migration volume, across the project

| Run | Hops | Rate |
|---|---|---|
| M3's T1 overnight (12.56 h, three worlds) | 22 595 | 1 799/h ≈ 30/min |
| Living deployment, 2026-08-09 reading | 393 210 cumulative | **426/min** |
| 2026-08-10 00:47Z, after the far end slowed | 516 633 | 251/min |
| 2026-08-10 14:42Z, thirteen hours at ×100 | **1 366 165** (≈812 000 in 13 h) | **≈1 040/min**, median 949 |

Population over the same period ran 176–360 across six worlds. `pacedDepth`, `heldDepth` and
`timeoutBounces` have read 0 in nearly every sample, `custodyDepth` flickers 0–6, and no reading
has ever shown a `bounceBack=true` or an `OVERLOADED` outside the two incidents §4 and §9.2
describe.

### 12.3 The archive's restart cost, which grows

| Ledger | Replay | Peak RSS |
|---|---|---|
| 599 184 records, 176 MB (2026-08-07) | **~6 s** | transient ~690 MB, settled RSS 146 → 170 MB |
| 3 700 672 records, 1.22 GB (2026-08-10) | **~93 s** | **~5.2 GB** |

**The archive serves nothing until the replay finishes** — it binds its port after, not before —
so an archive restart now costs about **100 seconds with no status page**, and the same 100 seconds
of crossings are **never copied to it**: 1 600–2 300 hops at the prevailing rate, absent from the
ledger rather than delayed in it, which is §5.1's "a subscriber that is absent changes nothing
about a migration" working as designed. The bring-up's wait for `archive subscribed` was 60 s and
**would have failed a healthy bring-up**; it is 300 s now and wants raising again as the ledger
grows. **Plan an archive restart; do not treat it as free.**

### 12.4 `genomeGaps` is throughput, not a leak

The archive may send at most **30** genome requests a minute **per peer**, so the ladder is bounded
near **180 fetches/min** across six peers, and only a crossing carrying an unheld hash costs a
fetch. On this rig the backlog therefore **grows above roughly 400 migrations a minute and drains
below roughly 340.** The collector's series is the proof: flat **0–2** across 56 samples over five
hours on 2026-08-07 under 130–190 crossings/min; then 22 → 1 071 when the rate doubled; then a
**monotone drain** from 2 714 to 1 110 while `ledgerRecords` grew by ~500 lines a minute — *a leak
does not drain against traffic*. Since the ×100 switch the rate rarely falls below the drain
threshold, so the standing backlog reached **≈46 000** against 2.8 M ledger records. **The
magnitude is not the finding; the absence of a drain window is.** And in the whole log history
`archive: no peer has served this genome` appears **exactly once**, because every peer on this rig
comes back.

---

## What transfers forward

To M5, and to any milestone that ships a deployment rather than a test:

1. **A bar set against a young world expires.** D14's 2 000 ms was correct when it was set and is
   unreachable now, at unchanged population and unchanged file size, because the term that grows
   is evolutionary history. Any threshold a milestone signs off should name the quantity it
   assumes is stable, so a later reader can check whether it still is.
2. **A durability rule must be checked in every implementation, not the one it was written for.**
   The journal and the ledger were the same discipline written twice, and the second copy carried
   the defect a day longer. M5 is where the archive stops being a private recorder and becomes a
   service other people depend on.
3. **Only a restart replays a journal.** A deployment whose value is staying up cannot observe its
   own durability, so restarts have to be *planned* as evidence rather than suffered as outages.
4. **Size a rate limit from a ceiling you control, not from load you measured.** Offered load moves
   when topology moves; the mod's ingest budget does not.
5. **Publish what each peer is running, and never infer it from a default.** A default that moves
   twice in a day is not what any particular slot is running — which is why the pacing settings,
   the mod version, the save cadence and the exclusion list are all on the wire as reports.
6. **Distinguish unknown from stale, and unknown from zero.** §20's non-wire changes are
   unknowable by design; an unwatched world is unknown, not empty; and an absent optional field is
   a `?`, never the value the game ships with.
7. **A stale value range breaks traffic; a stale absent field costs a number on a page.** After
   M5 every peer is a peer the operator cannot reach, so this distinction becomes the whole
   upgrade policy. **It is now literally the policy** (2026-08-10, `system_decomposition.md`
   D22): the relay's membership test is the **contract** version — the axis on which a stale
   peer breaks its neighbours — and the game version is a per-machine support-matrix question
   the map holds no opinion on.
8. **Route-around's success is its own failure mode**, and it now hides a dead peer from an
   operator who is not reading the page. M4 built the surface; M5 needs somebody or something
   watching it.
9. **The observability pattern that worked**: derive from what the wire already carries, mark it
   derived, bound anything that lands in a durable file, do the join server-side so two views
   cannot disagree, and delete the decoration that looks like the signal.
10. **Keep the wrong readings.** The save-stall watch item is the most useful document the milestone
    produced, and it is useful partly because it preserves four days of a diagnosis that was mostly
    wrong, corrects one of its own corrections in place with the sign error named, and says which
    paragraphs are history. A retrospective that only records the answer teaches nobody how the
    answer was nearly missed — and here it was nearly missed because the natural metric normalised
    by the compressed size, which is exactly the number that hid the cause.

---

## Open uncertainties, ranked

1. **The ~40% per-byte host residual on the save path.** Real, common-mode across all five worlds
   hour by hour, not reset by a game restart, partly reset by a host reboot (926 → 620 ms per raw
   MB, climbing back to ~850 over twenty hours). Bounded to CPU and allocation rather than disk,
   because `zipMs` is a tenth of the cost. Managed-heap/GC state accumulating with process uptime
   is the surviving suspect and it is not settled. Ruled out: save concurrency (0.99×) and host
   simulation demand (correlation −0.26, wrong sign).
2. **Does `lineageMs` compound, and how fast?** It is 53% of a save and flat against population, so
   its slope is the prediction that matters. The first 53 minutes of instrumented data cannot
   answer it — pooled warm medians go 994 → 936 → 1 106 → 1 096 ms, which is noise. Read it against
   `simulatedTime`, skip every `n=1`, and compare per raw MB rather than per zip kilobyte.
3. **The residual `4004` churn with no save behind it.** Slot 4 took ten heartbeat closes in three
   minutes at 23:43Z on 2026-08-09 with no save over 1.82 s in the window, and its game never
   restarted. It is the one episode the 13-second raise cannot have fixed, and with the save-shaped
   closes gone it is finally separable.
4. **Whether slot 1 recovers.** It fell to a population of **2** in the ENOSPC incident and has run
   at 7–15 while its neighbours held 22–43; at ×100 it oscillates 2–22, which is its old range
   sampled far more finely. A small world produces few young and depends on arrivals. The open
   question is whether the lanes alone carry it back or whether it settles low — the one place the
   map's self-healing property is being tested on an ecology rather than on a lane.
5. **`genomeGaps` after M5.** The rate is understood and the healthy value is zero; what changes is
   the ending. An entry leaves `pending` only when the genome arrives, so a gap becomes permanent
   when the peer holding the blob is gone for good — the normal case once strangers can leave.
   Read `m5_considerations.md` Risk 7 against ≈46 000, not against the 2 714 the item was opened on.
6. **The capture band still has no velocity-magnitude floor, and D17 reopened the item at its
   original priority.** `contract-a.md` §12 item 7's argument — that a jittering organism's repeated
   exports were too cheap to matter — rested on the one-way lane, and A38 removed the lane: a
   loiterer can now oscillate across a shared border, bounded in rate by the inset traverse plus the
   immunity window but no longer impossible. The signature to watch for is D19's own hop feed showing
   one species crossing one lane both ways in quick succession, and the fix, if it is ever needed, is
   a floor on the **multi-tick** outward component and never on one sample. It is unmeasured, and M5
   puts it in front of strangers. **The owner ruled on 2026-08-10** (`m5_considerations.md`
   decision 9): the floor is **not built in M5** and the signature above is **instrumented during the
   playtest**, so this uncertainty becomes a measurement M5 is obliged to take rather than one it
   might. Whether the floor is ever built is what that measurement decides.
7. **Whether the corpse pile at the origin regrows.** If out-of-bounds deaths really clamp corpse
   pellets to (0,0), any future breakout re-seeds it, and the cheap watchdog
   (`m4_livelock_findings.md` §4b follow-up (a)) has not been built.
8. **Whether the far sidecar has applied the non-wire work.** Unknowable by design (§9.3), and
   listed here because it is a permanent open item rather than a pending one.
9. **`phase5far` has never been run.** Cross-LAN pacing is confirmed locally and never across the
   link, because it needs two commands typed at the second computer and D9 forbids the rig to send
   them. After M5 every peer is one the rig cannot drive, so a test that needs a person at the far
   end needs a person who volunteered.
10. **The 3×3 north re-pair.** A map of height 2 can only prove the degenerate closure (§1.3). The
    full two-axis re-pair is a real assertion and it is unrun; it needs nine instances across the
    two machines, and the rig measurement first.
11. **Whether the collector's output stays gitignored or is pruned with its keepers committed.** An
    open question for the owner, and it is here rather than in the operations record because it is
    structural: the two halves of `e2e/baselines/` still disagree about what kind of thing that
    directory is.
