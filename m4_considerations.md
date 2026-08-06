# M4 Considerations — Operations: Resilience, Topology, Observability

This report expands milestone M4 of `system_decomposition.md`.

**Status: OPEN.** The owner ratified decisions D12 to D16 on 2026-08-05. This document
holds the design work behind them. M3 stays complete. M4 keeps the LAN rig and adds no
public exposure.

## Purpose

M3 proved the ring. M4 makes the ring survivable.

The M3 rig ran for 12 hours and 34 minutes without an operator. The T1 harvest
(`e2e/baselines/20260805T230730Z/t1-report.md`) reports the result in two halves.

The migration layer held. The ring moved 22 595 organisms, 1 799 each hour. Exactly-once
held across every hop. One hop stayed in flight when the host went away. The three worlds
evolved as one coupled population, and one of them ran on a different computer.

Everything around the migration layer failed:

- The rig never saved a world. About 97% of the simulated state is gone.
- One slot went dark for two hours. Its west neighbour then closed its export edge, and
  organisms walked out of the playable square instead of migrating.
- Every surviving measurement came from BepInEx logs. The next game launch overwrites
  them.

M4 answers those three failures with decisions D12, D14 and D15. Decision D13 adds the
next shape of the map, because the wire must carry that shape before M5 makes the wire
public.

M1 removed the in-game risks. M2 proved the custody chain. M3 proved the ring across two
machines. `m1_findings.md`, `m2_findings.md`, `m3_considerations.md`,
`contracts/contract-a.md`, `contracts/contract-b-m3.md` and the two baseline reports hold
those results. This document names the new work only.

## The map after M4

A dark slot no longer stops the current:

```
   Today (M3). Slot 2 is dark, and the current stops at slot 1.

   ┌────────► slot 1 ──╳──► slot 2         slot 3 ────────┐
   │            (edge closed)  (dark)     (receives nothing)│
   └────────────────────────────────────────────────────────┘

   After M4 (D12). The lane re-pairs, and the current continues.

   ┌────────► slot 1 ─────────────────────► slot 3 ────────┐
   │                   slot 2 is bypassed                   │
   └────────────────────────────────────────────────────────┘
```

A peer joins between two live slots, not only at the tail:

```
   Before:   … ──► slot 1 ──────────────► slot 3 ──► …
   After:    … ──► slot 1 ──► slot 4 ──► slot 3 ──► …
             one new lane, two changed lanes, no renumbering
```

The grid is the same map with a second axis. M4 builds the abstractions and keeps the
vertical lanes switched off:

```
   row 1:  ┌──► (0,1) ──► (1,1) ──► (2,1) ──┐
           └───────── east wrap ────────────┘
              ▲          ▲          ▲          north lanes, flag off in M4
              │          │          │          (they wrap to row 1)
   row 0:  ┌──► (0,0) ──► (1,0) ──► (2,0) ──┐
           └───────── east wrap ────────────┘

   A map of one row is the ring. Each peer exports east and north.
   Each peer receives west and south. No lane returns to its source.
```

## Scope

In scope:

- Lane re-pairing around a dark slot, and the `EDGE_STATUS` ripple that follows (D12)
- Insertion between two live slots, for a returning peer and for a new instance (D12)
- A handoff state on each journal entry, and the re-route rule that depends on it (D12)
- A relay answer that proves non-delivery (D12)
- Plural export edges on Contract A, and one edge state for each of them (D13)
- Coordinate addressing beside the slot number, with the ring as the one-row case (D13)
- Per-axis route-around (D13)
- The vertical lanes, behind a flag that ships off (D13)
- Periodic world saves, a save on quit, and a save rotation (D14)
- A scripted recovery procedure for one instance (D14)
- A rig-wide resume test that delivers the organism in flight since 2026-08-04 (D14)
- Population on the Contract B ring view (D15)
- Durable metrics outside the BepInEx logs (D15)
- A live status page, served by the archive, and a `ringstat` terminal command (D15)
- Log preservation at shutdown (D15)
- A slot handover command at the relay (D15)
- A join kit for a new instance (D15)
- An entry-edge crowding metric, and one candidate mitigation as a design item (D15)
- The portal work item. `m4_portal_findings.md` is its input, and that research is in
  progress

Out of scope. Decision D16 keeps these items in M5:

- A hosted relay on a VPS
- TLS, and authentication for public exposure
- Capacity limits and abuse limits
- A player-facing package for the mod and the sidecar
- A community playtest with strangers

Also out of scope:

- **Sleep prevention, and crash prevention.** Decision D14 accepts the hard stop and
  builds the recovery
- The live grid: vertical lanes switched on, the second capture band in production, and a
  rig with six or more instances (D13, and *Design Calls for the Owner*)
- libp2p and direct peer-to-peer transport (M6)
- The `species-catalog` module (M7)
- Corpse, pellet and egg payloads (M7)
- Census uploads to the archive (D11)
- A write interface on the archive. The status page reads only

## Design Question 1 — The lane re-pairing protocol

**Deliverability, not liveness, selects the target.** A slot accepts an organism under four
conditions. Its peer connects. Its mod connects. Its game version agrees. Its
`SimulationSize` agrees. Section 8 of `contract-b-m3.md` already lists those tests. M4
changes what the tests produce. Today they close an edge. From M4 they filter a list.

**The relay computes the effective neighbour.** Walk the ring order east from the peer.
Take the first deliverable slot. Skip every other slot. The relay holds the connection
state already, so this computation adds no new knowledge to a deliberately dumb relay
(D1). The relay still parses no body and indexes nothing.

**Publish both maps.** `SECTOR_GRANT` names the effective neighbour and the slots it
skipped. `PEER_STATUS` keeps the structural ring order. An operator reads the structure,
and a sidecar routes on the effect. One map without the other hides a defect: a structure
alone hides the bypass, and an effect alone hides the shape.

**The ripple keeps its direction.** A slot that goes dark tells its west neighbour, and
tells nobody else. The west neighbour re-targets and keeps its export edge open. The dark
slot's east neighbour receives nothing and hears nothing.

**An export edge closes for one reason only.** No slot in the ring is deliverable. A peer
never exports to itself. Section 8's other close reasons become skip reasons, and each one
keeps its name in the skip list.

**The mod sees none of this.** Contract A carries an open export edge and nothing else.
Route-around and the grid are invisible to the game (D8). This property is what keeps the
mod free of topology, and M4 must not break it.

**Ordering.** The relay writes the ring file first. It then answers the claim. It then
broadcasts `PEER_STATUS` with a higher epoch. It then sends a fresh `SECTOR_GRANT` to each
peer whose effective neighbour changed. A receiver applies the highest epoch only.

## Design Question 2 — An orphaned journal hop: re-route or hold

A journaled hop names the slot it recorded. That slot goes dark. Three answers exist:
hold the entry, bounce the organism home, or re-route the entry east.

**Reason from decision D2.** The sender holds custody until `MIGRATION_ACK`. The receiver
deduplicates on `migrationId`. A second delivery to the **same** peer is therefore safe. A
delivery to a **different** peer is a new custody, and no deduplication covers it.

**The dangerous case is invisible from the sender.** The relay forwards the frame. The
far sidecar journals it and takes custody. The far machine then dies before its
acknowledgement leaves. The sender sees silence. A re-route now creates a second organism,
because the far peer replays its own journal when it returns. Duplication is the one
failure decision D2 refuses.

**The safe case is provable at the relay.** The relay knows whether it forwarded a frame.
A frame it never forwarded reached no sidecar and created no custody. The relay states
that explicitly, and the sender treats that statement as proof.

**The rule.**

| Journal entry state | Evidence | Action |
|---|---|---|
| Never forwarded | The relay answers `SLOT_VACANT`, `PEER_OFFLINE` or `NOT_FORWARDED` | **Re-route** east to the current effective neighbour. Keep the `migrationId`. Rewrite `destSlot`. |
| Refused for a peer-local reason | `MIGRATION_NACK` with `OVERLOADED` or `SIM_SIZE_MISMATCH` | **Re-route.** The receiver states that it took no custody. Another slot accepts the same organism. |
| Refused for a payload reason | `MIGRATION_NACK` with `INVALID_PAYLOAD` or `KIND_UNSUPPORTED` | **Bounce home.** Every slot refuses this organism, so the ring is not the answer. |
| Forwarded, then silence | None | **Hold.** Retry the recorded `destSlot` on the retry cadence. The retry is idempotent. |

**Hold has no deadline, and that is deliberate.** A held organism is alive in a journal. It
is not lost and it is not duplicated. The T1 run holds exactly one such entry, and a rig
that comes back delivers it. A timer that converts a hold into a bounce trades a certain
state for a possible duplicate.

**Give the operator one escape hatch.** `multiverse-sidecar --release-inflight <migrationId>
bounce|drop` releases one held entry by hand. The command names the duplication risk in
its own output. It is the custody twin of `--release-slot`, and it is a deliberate act on
the machine that holds the journal.

**Rewrite the destination in one place only.** Section 7.3 of `contract-b-m3.md` forbids a
journal rewrite. M4 amends that rule with one exception, and the exception carries its own
evidence: a proof of non-delivery. Every other entry keeps the destination it recorded.

**A re-route keeps the organism moving east.** A bounce returns the organism to the edge it
left. The T1 run measured what a closed edge costs: square crossings rose about nine times
in the two hours slot 1 had nowhere to export. A re-route removes that pressure. This is
the second reason to prefer it over a bounce.

## Design Question 3 — Custody for a released or replaced slot

**A slot release never moves a journal.** The peer that holds a journal holds the
organisms in it. No other peer claims them, and no protocol transfers them. A transfer
protocol is a duplication mechanism with a friendly name.

**A released peer keeps two obligations.** It delivers its inbound entries to its own mod,
even outside the ring. It re-routes or releases its outbound entries when it rejoins.

**A slot number is never reused.** `maxSlotEverIssued` never decreases (§7.5). This is what
makes a vacant slot unambiguous. A journaled `destSlot` that no longer exists names a world
that never returns. `SLOT_VACANT` is therefore a permanent answer, and a valid proof.

**A handover rebinds the reservation, not the data** (D15). `multiverse-relay
--handover-slot <n> <newPeerId>` keeps the slot number and the position, and changes the
identity behind it. The old machine keeps its journal, its genome cache and its logs. The
relay refuses a handover while the old peer is live.

**Print the consequences before the act.** `--release-slot` and `--handover-slot` both list
the held entries that name the slot. The operator then makes one decision with the facts
in view.

## Design Question 4 — A returning peer's old journal

A peer returns after an outage. Its west neighbour bypassed it. Its own journal holds work
from before the outage. Four rules reconcile it.

1. **Reclaim first.** The peer claims its slot with its persisted `peerId`. It gets the
   same slot number and the same position (§7.2 rule 1). Its west neighbour's effective
   neighbour returns to it on the next liveness change.
2. **Replay the inbound entries.** The sidecar delivers every un-acknowledged `MIGRATE_IN`
   in journal order. Contract A §7.5 owns this path already.
3. **Re-evaluate the outbound entries under Question 2.** An entry that was never
   forwarded re-routes to the current effective neighbour. An entry that was forwarded
   retries its recorded destination. The receiver deduplicates, so a retry after a long
   outage is safe.
4. **Reassert custody against the mod.** A new `sessionId` triggers the reassertion of
   Contract A §7.4. A resurrected game reports the organisms it holds, and the sidecar
   rebuilds its view.

**No backlog arrives at the returning peer.** Its west neighbour re-routed only the entries
it never forwarded. Nothing accumulated behind the gap. A returning peer therefore receives
new traffic, not a flood.

**One conflict stays open.** The world reloads from a save. The journal is newer than the
save. An organism that the save still holds is also in a tombstone. Contract A §7.3 makes
`entityId` the durable deduplication key, so the mod refuses a second copy. Test that path
in M4 (Risk 2).

## Design Question 5 — Insertion between two live slots

**One rule replaces two.** Today insertion appends at the tail (§7.2 rule 4). M4 accepts a
position. The claim carries an advisory `insertAfterSlot`. The relay honours a position
that exists, and falls back to the tail.

**Insertion changes two lanes and no numbers.** The predecessor's structural east neighbour
becomes the newcomer. The newcomer's east neighbour becomes the old successor. Every other
slot keeps its number, its position and its relative order.

**Insertion cannot touch work in flight.** A newcomer gets a slot number greater than every
number ever issued. No journal entry names a number that did not exist when it was
written. This makes §7.3 rule 1 provably sufficient for insertion.

**Return needs no insertion.** A peer that keeps its reservation returns into its own
position. Route-around makes the return a liveness event. The insertion path therefore
serves a new instance, a reinstalled peer with a new identity, and a released slot only.

**Two peers that claim one position race.** The relay serializes claims and answers them in
order. The second peer lands after the first. The answer names the position it received,
and never fails for a lost race.

## Design Question 6 — The grid model, worked through

### Addressing

A ring slot is an integer. A grid position is a pair. Keep both, and keep them separate:

| Concept | Value | Property |
|---|---|---|
| Slot number | integer, `≥ 1` | The **routing address**. Never reused. Never renumbered. `destSlot` names it. |
| Coordinate | `{col, row}` | The **position**. It decides who the neighbours are, and nothing else. |

This split is what keeps the map safe. A position moves when the map grows. An address
never moves, so no journal entry ever points at the wrong world.

**Do not derive a coordinate from an index.** A row-major index over the ring order looks
cheaper. It renumbers every peer on each insertion, and decision D8 forbids a reshuffle.

**The ring is the one-row grid.** A map of height 1 gives each peer one east neighbour and
no north neighbour. Every M3 rule then holds unchanged. This is why the generalization
costs no behaviour change.

### Insertion in two dimensions

A grid holds holes. A hole is a position with no reservation. Route-around already bypasses
a hole, because a hole and a dark slot look the same to a router. **Route-around is
therefore the prerequisite that makes a grid with holes viable**, and it is the reason
decision D12 lands before decision D13.

Insertion has two forms:

1. **Fill a hole.** The newcomer takes a free position. Two lanes change on each axis.
2. **Extend an axis.** A new column adds `height` positions, and the newcomer fills one.
   The rest are holes.

A ragged map is not legal. Every position in the rectangle exists, and an empty position
is a hole with a defined bypass.

### Route-around on a grid

Route-around runs **per axis**. Walk east along the row for the east lane. Walk north up
the column for the north lane. Each lane closes only when its whole row or column holds no
deliverable slot.

Two consequences follow. A peer with a dead row still exports north. A peer alone in the
map exports nowhere, and both edges close with `no_peer`.

### Contract A deltas

| Field | Today | With the grid |
|---|---|---|
| `exportEdge` | One edge enum, `"E"` | `exportEdges`, an array. `["E"]` under the ring, `["E","N"]` on the grid |
| `borderEdges` | `["E","W"]` | `["E","N","W","S"]` |
| `EDGE_STATUS.edges` | Exactly one entry | One entry for each export edge, each with its own state and reason |
| `MIGRATE_OUT.exitEdge` | Always `"E"` | `"E"` or `"N"`. The enum already holds all four values |
| `MIGRATE_IN.entryEdge` | `"W"`, or the export edge for a bounce | `"W"` or `"S"`, or an export edge for a bounce |
| Capture band (§4.3.1) | One band on the east edge | Two bands. `x ≥ S − W` with `vx > 0`, and `y ≥ S − W` with `vy > 0` |

**`exportEdges` is a breaking change.** A field changes type, and `EDGE_STATUS` changes
cardinality. This is the whole reason the change lands in M4. After M5 the wire is public,
and a breaking change then costs a migration story instead of a rebuild.

**Contract debt A5 stays moot.** A `MIGRATE_OUT_NACK` carries no edge. The mod correlates
it through `migrationId`, and that correlation does not care how many edges exist. Do not
close the debt in M4.

**The corner is a new question.** An organism at `x ≥ S − W` and `y ≥ S − W` with both
velocity components outward sits in two bands. Pick one export edge. The proposed rule
takes the larger outward component, and takes `E` on a tie. Record the rule in the
contract, because two mods that answer differently produce different maps.

### Contract B deltas

| Message | Change |
|---|---|
| `SECTOR_CLAIM` | `exportEdge` becomes `exportEdges`. Add the advisory insertion position. Add `population` (D15). Add the coordinate once the grid is live |
| `SECTOR_GRANT` | `eastNeighbour` becomes `neighbours`, keyed by export edge. Each entry names the **effective** target and the slots it skipped |
| `PEER_STATUS` | Each slot gains `population`, and gains `{col,row}` with the grid. The list keeps the structural order. Add the map shape |
| `MIGRATION_PAYLOAD` | No shape change. Routing stays on `destSlot`, and delivery stays single-hop |
| Relay answers | Add the explicit non-delivery answer of Question 2 |

### Mod changes

- A second capture band, with the same outward-velocity test and the same cadence
- The corner rule
- A second passive entry edge on the south side
- A second entry-position mapping, from `contract-a.md` §4.3
- A flag that disables the vertical pair, and that ships off

The per-tick cost is two comparisons for each organism. The M3 band test already runs each
`FixedUpdate`, so the second band adds no new pattern.

## Design Question 7 — How much grid M4 builds

**Recommendation: build the abstractions, ship the vertical lanes behind a flag, and defer
the live grid.**

Three facts drive the recommendation.

**Fact 1 — the wire deadline is real.** `exportEdges` and the plural `EDGE_STATUS` break
Contract A. M5 puts the wire in front of strangers. A break before M5 costs a rebuild of
three binaries and one mod. A break after M5 costs a version-migration path for people the
owner cannot reach.

**Fact 2 — the proof needs a rig the owner does not have.** A 2×2 map is degenerate on both
axes, exactly as a two-slot ring is. One hop east and one hop north already return an
organism to its own row and column. An honest two-axis proof wants 3×3. A 3×2 map is the
smallest map that proves anything about a north lane. The rig runs three instances today,
on two machines. Six is a hardware question, not a code question.

**Fact 3 — the cost split favours the abstractions.** The abstractions are contract text,
relay bookkeeping and sidecar plumbing. They need no game. The remainder needs the game, the
hardware and a new test harness. It holds four items: the second capture band in
production, the corner rule under real organisms, a six-instance rig, and a two-axis exit
test. The abstractions are the smaller part, and they carry the deadline.

**What "behind a flag" means here.** Build the north lane end to end. Test it with two local
slots and a 1×2 map. Ship the flag off. M4's shipped behaviour is then exactly the ring, so
the living deployment carries no new risk.

**This recommendation defers part of decision D13, so it needs the owner's sign-off.** See
*Design Calls for the Owner*, call 1.

## Design Question 8 — The recovery flow

### What already exists

- A durable journal, flushed before the custody answer (D2)
- Deduplication on `migrationId`, against entries and tombstones
- Custody reassertion on a changed `sessionId` (`contract-a.md` §7.4)
- Inbound replay in journal order (§7.5)
- A durable ring file at the relay, and a durable `peerId` at each sidecar (§7.4)
- A genome cache on disk

### What M4 adds

**A periodic world save.** A wall-clock timer drives `SaveSystem.CreateSave` by hand. The
public `SaveGame` wrapper swallows exceptions inside a coroutine (`m1_findings.md`), so the
mod calls the inner method and reports the result. The game's own autosave stays off. An
autosave reloads the scene under a live rig, and that failure is worse than a stale save.

**A save on quit.** The mod saves when the application quits. T1 proves the need: the owner
closed two windows and discarded 12 hours of state.

**A save rotation.** Keep the last N saves. Write to a temporary name, then rename. A
half-written save must never replace the last good save.

**A save receipt.** One log line and one metric record for each save: the name, the byte
count, the population and the simulated minute. The T1 harvest reconstructed the save
vintage from those receipts, and that reconstruction must not depend on a log file again.

**A scripted single-instance recovery.** One script, one slot:

1. Record the slot's population, its simulated minute and its journal state.
2. Stop the game and the sidecar with a hard kill.
3. Start the sidecar. Confirm that it reclaims its slot.
4. Start the game. Load the last periodic save.
5. Confirm the replay of every un-acknowledged inbound entry.
6. Confirm custody reassertion against the new session.
7. Count the organisms by entity ID across the whole ring.

**A rig-wide resume test.** The T1 journals are on disk. They hold one real in-flight hop:
migration `9d6db335-b1ae-433e-a44a-bb2109912913`, entity `2004967003`, slot 1 to slot 2,
recorded 2026-08-04T14:13:14.202Z. Bring the rig up against those journals. The organism
must arrive in slot 2. **Do not delete `e2e/data/slot-1/journal`.** That directory is the
only copy of the test input.

### The interaction that needs care

A world reloads from a save that is older than the journal. The save holds an organism that
the journal already exported. The mod's durable deduplication key is `entityId`
(`contract-a.md` §7.3), so a second arrival of that organism is refused. The reverse case
also exists: the save does not hold an organism that arrived after the save. That organism
is lost, and decision D2 accepts loss. Name both cases in the recovery script's output.

## Design Question 9 — The entry edge

The owner saw a mass of organisms on the west border after the overnight run. No
measurement exists. Decision D15 therefore measures first.

**What the geometry does today.** An organism leaves the east edge at `y`. It arrives at
`x = −S + W + margin` with the same `y`, the same velocity and the same heading
(`m2_findings.md` §4.4). It therefore arrives inside the world and travels east.

**Four candidate causes.**

1. **Arrival rate.** Slot 1 imported 6 003 organisms in 12.56 hours, into a population near
   22. Each world receives about 21 organisms each simulated hour. Most of the population
   is a recent arrival.
2. **Transverse concentration.** `exitPosition` clusters at the source. The receiver mirrors
   it, so arrivals cluster in the same band of `y`.
3. **No food under the arrival band.** Pellets spawn inside a zone only (`m2_findings.md`
   §1.4). An arrival band outside every zone holds hungry organisms.
4. **The shaded band.** An organism that walks west leaves the square and travels to 4000
   before the wrap fires. It stays alive and visible outside the west edge for a long time.

**The metric.** Add to the mod's `[M2-CROSSING]` series, once each simulated minute:

- The count of organisms inside the entry band
- A histogram of arrival positions along the entry edge
- The residence time between an arrival and its first departure from the band

**The candidate mitigation, and its cost.** Spread arrival positions along the entry edge
instead of mirroring `exitPosition`. This flattens the arrival histogram. It also breaks
the transverse continuity that decision D3 buys. An organism that leaves at one `y` then
arrives at a different `y`, and the one-continuous-world illusion weakens. Hold the
mitigation until the metric names the cause.

## Risks

### Risk 1 — Grid scope creep

Decision D13 describes a complete new topology. M4 builds part of it. The boundary is easy
to cross, because each grid item looks small beside the one before it.

Hold the boundary at one line: **M4 builds what the wire needs, and nothing the game
needs.** Contract text, relay bookkeeping, sidecar lanes and a flagged north lane are in.
The second capture band in production, the corner rule under real organisms, and a
six-instance rig are out.

Test the boundary with one question for each item: does M5 break the wire without it? A
"no" moves the item out of M4.

### Risk 2 — Healing interacts with at-most-once custody

Decision D2 refuses duplication. Route-around introduces a second destination for one
organism. The two rules meet in each orphaned journal entry.

Question 2 holds the answer: re-route only under a relay proof of non-delivery, and hold
otherwise. The risk is not the rule. The risk is an implementation that treats silence as
proof.

Three tests gate this risk:

- Kill a destination sidecar **after** the relay forwards a frame and **before** the
  acknowledgement. Restart it. The organism exists exactly once.
- Stop a destination peer **before** any frame reaches it. The sender re-routes. The
  organism exists exactly once, in the new destination.
- Hold an entry for one hour against a dark slot. Return the slot. The organism arrives,
  and no copy exists anywhere else.

A count of two organisms in any test fails the milestone.

### Risk 3 — The save timer competes with the simulation

A save serializes every organism, every pellet and every zone. The rig runs at 20× or more.
A save that stalls the simulation reduces the very throughput that M4 measures.

Measure the save cost first. Record the duration against the population and the world size.
Then choose the cadence from the measurement, not from a guess.

Two rules limit the damage. Never save two worlds on one machine at the same moment. Never
start a save while a `MIGRATE_IN` waits in the mod's queue.

A save that costs more than a few seconds needs a different cadence. It does not need a
different decision. Decision D14 accepts a slower rig over a lost night.

### Risk 4 — The observability data path competes with the migration path

The status page, the metrics and `ringstat` all read state that the migration path owns. A
reader that blocks a writer converts an operator convenience into an outage.

Three rules keep the path clean. The archive serves the page, because it already
subscribes to every envelope and already runs beside the relay. The sidecars write metrics
to their own files, and no component reads another component's memory. Nothing on the
migration path waits for a metric write, exactly as nothing waits for the archive today.

The page must also state what it cannot see. A slot that reports nothing is unknown, not
empty. An honest gap beats a confident zero.

### Risk 5 — Route-around hides a dead peer

A healed ring keeps working. That is the point, and it is also the failure mode. An
operator sees a healthy current, and misses a world that went dark a day ago.

The status page therefore shows each bypassed slot, the time it went dark and the traffic
that no longer reaches it. `ringstat` prints the same list. A bypass is a warning, not a
normal state.

### Risk 6 — Logs die at shutdown

BepInEx overwrites `LogOutput.log` at each launch. The T1 harvest needed 147 MB of those
logs, and it saved them by hand hours before the next launch.

The stop script copies each BepInEx log to a timestamped file before any restart. The start
script refuses to launch a game whose log has not been preserved. Durable metrics reduce
the need for the logs, and they do not remove it.

### Risk 7 — The far end has no drive path

Decision D9's answer holds: the rig never drives the second computer. A save is not an
observation. The far end therefore needs two local actions that the main machine cannot
perform: a periodic save, and a log preservation step.

Put both in `start-slot2.ps1` and `stop-slot2.ps1`. The far operator then runs the same two
scripts as before, and the join kit ships them.

### Risk 8 — The portal work item has no research yet

`m4_portal_findings.md` is in progress in a parallel pass. This document records the
dependency and nothing more.

Do not schedule construction against the portal until those findings land. Read them first.
Then decide between three answers. The portal is an M4 deliverable, a mitigation for
entry-edge crowding (Question 9), or a revival of the gateway towers that decision D3
parked.

## Contract Changes Needed

This pass does not rewrite the contracts. The list below is the design input for the
implementation wave.

| # | Document | Change | Source |
|---|---|---|---|
| 1 | `contract-b-m3.md` §7.2 | Insertion accepts a position. Add the advisory `insertAfterSlot`, and keep the tail as the fallback. | D12 |
| 2 | `contract-b-m3.md` §7.3 | Add the one exception to the journal-rewrite rule: a re-route under a proof of non-delivery. Name the proofs. | D12 |
| 3 | `contract-b-m3.md` §8 | Replace "close the export edge" with "re-pair the lane". Close only when no slot is deliverable. Keep the one-way ripple. | D12 |
| 4 | `contract-b-m3.md` §6.4 | `SECTOR_GRANT` names the effective neighbour for each export edge, and the slots it skipped. | D12, D13 |
| 5 | `contract-b-m3.md` §6.5 | `PEER_STATUS` keeps the structural order, and gains `population` for each slot. | D12, D15 |
| 6 | `contract-b-m3.md` §6.3 | `SECTOR_CLAIM` gains `exportEdges`, the insertion position and `population`. | D13, D15 |
| 7 | `contract-b-m3.md` §6.8 | Add the explicit non-delivery answer, and separate a peer-local refusal from a payload refusal. | D12 |
| 8 | `contract-b-m3.md` §7.5 | Add `--handover-slot`. State that a handover never moves a journal. | D15 |
| 9 | `contract-a.md` §5.1, §5.4 | `exportEdge` becomes `exportEdges`. `EDGE_STATUS.edges` carries one entry for each export edge. **Breaking.** | D13 |
| 10 | `contract-a.md` §4.3.1 | State the second capture band, and the corner rule. | D13 |
| 11 | `contract-a.md` §13 A5 | Record that the debt stays moot under two export edges. Correlation is on `migrationId`. | D13 |
| 12 | Both documents | Bump the major version, because item 9 changes a field type. | D13 |
| 13 | Both documents | Replace each "M4" that means public release with "M5". | D16 |

## Work Packages

Eight packages. WP1 gates the wire work. WP7 gates the exit test.

### WP1 — The contract amendments

**Depends on:** this document.
**Needs the game:** no.

- The thirteen items of *Contract Changes Needed*
- One worked example for each changed message
- The version bump, and a migration note for the M3 rig

Write WP1 before any code. M2 and M3 both paid for the alternative.

### WP2 — The relay: healing, insertion and handover

**Depends on:** WP1.
**Needs the game:** no.

- The deliverability filter, and the effective-neighbour walk
- The two published maps: structure in `PEER_STATUS`, effect in `SECTOR_GRANT`
- Insertion at a position, with the tail as the fallback
- The explicit non-delivery answer
- `--handover-slot`, and the held-entry report for both operator commands
- Per-axis walking, behind the vertical-lane flag

### WP3 — The sidecar: handoff state, re-route and metrics

**Depends on:** WP1, WP2.
**Needs the game:** no.

- The handoff state on each journal entry
- The re-route, bounce and hold rule of Question 2
- `--release-inflight`
- Outbound re-evaluation at reconnect (Question 4)
- Durable metrics: population, crossings, edge state, custody depth, save receipts
- Population on the ring claim
- One lane for each export edge, with the second lane behind the flag

### WP4 — The mod: saves, edges and instrumentation

**Depends on:** WP1.
**Needs the game:** yes.

- The periodic save, the save on quit and the rotation
- The save receipt line and its metric record
- The save-cost measurement of Risk 3
- `exportEdges` on `CONFIG_UPDATE`, and one edge state for each
- The second capture band and the corner rule, behind the flag
- The entry-edge metrics of Question 9

### WP5 — The archive: the status page

**Depends on:** WP1, WP3.
**Needs the game:** no.

- A live HTML status page: slots, liveness, effective lanes, bypasses, populations,
  per-lane flow, custody depth and the last save of each world
- The honest-gap rule of Risk 4
- `ringstat`, over the same data, in the terminal

### WP6 — The rig: recovery, preservation and the join kit

**Depends on:** WP2, WP3, WP4.
**Needs the game:** yes.

- The scripted single-instance recovery of Question 8
- Log preservation in the stop script, and the refusal in the start script
- The far-end save and preservation steps, in the two PowerShell scripts
- The join kit: the bundle, the token, the relay address and one page of steps
- A fourth instance, for the splice-in test

### WP7 — The portal

**Depends on:** `m4_portal_findings.md`.
**Needs the game:** unknown.

Blocked on research. Read the findings first. Then scope this package, or move it out of
M4.

### WP8 — The exit test

**Depends on:** WP2, WP3, WP4, WP5, WP6.
**Needs the game:** yes.

Build the test on `e2e/run-m3-lan.sh`. The ring form-up, the command file and the archive
checks all carry over.

## Exit Test

The test uses the M3 rig and one new instance. Slot 1 and slot 3 run on the main machine.
Slot 2 runs on the second computer. Slot 4 joins during the test.

### Part 1 — The ring heals around a hard stop

1. Start the rig. Confirm three slots, in ring order, with all lanes open.
2. Record the per-lane flow for ten minutes.
3. Hard-kill the game and the sidecar of slot 3, mid-flow.
4. Watch slot 1's export edge and slot 2's export target.

The test passes when all of these conditions hold:

- Slot 2's lane re-pairs to slot 1, and its export edge stays open.
- No export edge closes.
- The current continues. Slot 1 and slot 2 keep exchanging organisms.
- The status page shows slot 3 as bypassed, with the time it went dark.
- Each organism in flight to slot 3 at the kill is held, re-routed or bounced by the rule
  of Question 2. Each entry states which answer it took, and why.
- The ring holds exactly one copy of each organism, counted by entity ID.

### Part 2 — The dead slot splices back in

5. Start slot 3's sidecar. Confirm the reclaim of its slot number and its position.
6. Start slot 3's game against its last periodic save.
7. Watch slot 2's export target.

The test passes when all of these conditions hold:

- Slot 3 returns to the same slot number and the same position between slot 2 and slot 1.
- Slot 2's lane re-pairs back to slot 3, with no operator action.
- Slot 3 replays every un-acknowledged inbound entry to its mod.
- Slot 3's world holds the population of its last save, and not the population of its
  first start.
- Any held entry that names slot 3 is delivered, and the organism exists exactly once.

### Part 3 — A brand-new instance splices into a live ring

8. Start a fourth sidecar with a new `peerId`. Ask for a position between slot 1 and slot 2.
9. Start a fourth game, on a new world.

The test passes when all of these conditions hold:

- The relay grants slot 4 between slot 1 and slot 2, with no renumbering of any other slot.
- Slot 1 exports into slot 4, and slot 4 exports into slot 2, within one status broadcast.
- No organism in flight at the moment of the insertion changes its destination.
- Organisms cross into slot 4 and out of it, without a forced export.
- The archive records both new lanes.

### Part 4 — The resume test

10. Stop the rig. Keep every journal.
11. Start the rig against `e2e/data/slot-1/journal` from 2026-08-04.

The test passes when migration `9d6db335-b1ae-433e-a44a-bb2109912913`, entity
`2004967003`, arrives in slot 2 and exists exactly once in the ring.

### Part 5 — Periodic saves, proven by a kill and a reload

12. Run one world for one hour with the periodic save on.
13. Record its population and its simulated minute.
14. Hard-kill the game.
15. Restart it against its last periodic save.

The test passes when all of these conditions hold:

- The reloaded world is within one save interval of the recorded state.
- The reloaded world is **not** the state of the first start.
- The save receipts name every save of the hour.
- `worldstat` reads the reloaded save with no error.
- The recovery script runs the whole sequence with no manual step on this machine.

### Part 6 — The status page shows it all

16. Read the status page after each part above.

The test passes when the page states all of these values, at each point above:

- The map and its shape
- The liveness and the population of each slot
- Each effective lane, and each bypass with its start time
- The custody depth of each sidecar
- The last save of each world
- The entry-edge crowding metric

An unknown value reads as unknown. A zero reads as a measurement.

### Part 7 — The error sweep

The test passes when no log holds an error at the end of the run. Sweep every BepInEx log,
sidecar log, relay log and archive log.

## Deliverables

- The wire specification, from WP1: `contract-a.md` and `contract-b` at a new major
  version
- A relay with lane re-pairing, insertion at a position, handover and the non-delivery
  answer
- A sidecar with the handoff state, the re-route rule, `--release-inflight` and durable
  metrics
- A mod with periodic saves, save-on-quit, save rotation, plural export edges and the
  entry-edge metrics
- The vertical lane, complete and switched off, with a 1×2 test that exercises it
- An archive with a live status page, and a `ringstat` command
- A scripted single-instance recovery procedure
- A join kit for a new instance
- Log preservation in the start and stop scripts
- The save-cost measurement of Risk 3
- The entry-edge crowding measurement of Question 9
- The automated M4 exit test, with its recorded result
- An `m4_findings.md` with the research results of the milestone
- A scoped or discharged portal work item, after `m4_portal_findings.md` lands

## Design Calls for the Owner

1. **How much grid M4 builds** (Question 7). The recommendation defers the live grid. Build
   the abstractions. Ship the vertical lanes behind a flag that is off. Hold the second
   capture band, the corner rule under real organisms and a six-instance rig for a later
   milestone. The reason is the rig, not the code. A 2×2 map is degenerate on both axes,
   and an honest proof wants six to nine instances against three today. **This defers part
   of decision D13 and needs a sign-off.**
2. **Where the live grid lands.** It has no milestone number yet.
   `system_decomposition.md` records it as unscheduled. Name it after call 1.
3. **The hold has no deadline** (Question 2). A held organism waits for its recorded
   destination forever, and only `--release-inflight` ends the wait. Confirm that decision
   D2's preference for loss over duplication extends to an unbounded hold.
4. **The entry-edge mitigation** (Question 9). Spreading arrivals along the entry edge
   flattens the crowd and weakens decision D3's one-continuous-world illusion. The
   recommendation measures first and holds the mitigation. Confirm that order.
5. **The save cadence** (Risk 3). The measurement chooses it. Name the acceptable stall
   before the measurement runs, so the result has a test to pass.

## Next Steps

1. Read `m4_portal_findings.md` when it lands. Scope WP7, or move it out of M4.
2. Answer the five design calls above.
3. Write WP1. No code starts before the wire settles.
4. Measure the save cost (Risk 3) on the M3 rig, before WP4 fixes a cadence.
5. Preserve the T1 journals and the harvested BepInEx logs. Part 4 has no other input.
6. Bring the M3 rig back up with periodic saves as soon as WP4 lands. The next overnight
   run then produces a full comparison, not a partial one.
