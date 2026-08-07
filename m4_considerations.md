# M4 Considerations — Operations: Resilience, Topology, Observability

This report expands milestone M4 of `system_decomposition.md`.

**Status: COMPLETE.** The LAN exit test passed on 2026-08-06, on a 3×2 map of six real
worlds across two computers. *Exit Test → Result* records the run, phase by phase. The rig
came back up after the test, and it runs now as the living multiverse.

The owner ratified decisions D12 to D16 on 2026-08-05. The owner signed off this design on
the same day, and overrode three of its recommendations. The section *Owner Sign-Offs*
records the four calls, and each affected section carries the result. M3 stays complete.
M4 kept the LAN rig and added no public exposure.

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
public. The owner's sign-off runs that shape live, on a 3×2 map of six instances.

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

The grid is the same map with a second axis. M4 builds it and runs it:

```
   row 1:  ┌──► (0,1) ──► (1,1) ──► (2,1) ──┐
           └───────── east wrap ────────────┘
              ▲          ▲          ▲          north lanes, live in M4
              │          │          │          (they wrap to row 1)
   row 0:  ┌──► (0,0) ──► (1,0) ──► (2,0) ──┐
           └───────── east wrap ────────────┘

   A map of one row is the ring. Each peer exports east and north.
   Each peer receives west and south. No lane returns to its source.
   The 3×2 map above is M4's target rig: six instances, two machines.
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
- The vertical lanes, live, with the second capture band and the corner rule (D13)
- A rig of six instances in a 3×2 map, across the two machines (D13)
- Periodic world saves, a save on quit, and a save rotation (D14)
- A scripted recovery procedure for one instance (D14)
- A rig-wide resume test that delivers the organism in flight since 2026-08-04 (D14)
- Population on the Contract B ring view (D15)
- Durable metrics outside the BepInEx logs (D15)
- A live status page, served by the archive, and a `ringstat` terminal command (D15)
- Log preservation at shutdown (D15)
- A slot handover command at the relay (D15)
- A join kit for a new instance (D15)
- A delivery rate limit at the sidecar, which paces every arrival burst (D15)
- An entry-edge crowding metric, which verifies the rate limit (D15)
- A bounded hold on an orphaned journal entry, and its automatic bounce (D2, D12)
- The portal work item, built to the recommendation in `m4_portal_findings.md`

Out of scope. Decision D16 keeps these items in M5:

- A hosted relay on a VPS
- TLS, and authentication for public exposure
- Capacity limits and abuse limits
- A player-facing package for the mod and the sidecar
- A community playtest with strangers

Also out of scope:

- **Sleep prevention, and crash prevention.** Decision D14 accepts the hard stop and
  builds the recovery
- **Arrival-position spreading.** The rate limit answers the crowd, so decision D15 keeps
  this mitigation parked (Question 9)
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

## Design Question 2 — An orphaned journal hop: re-route, hold or bounce

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
| Forwarded, then silence | None | **Hold, then bounce.** Retry the recorded `destSlot` on the retry cadence. The retry is idempotent. Bounce home at the hold timeout. |

**The hold is bounded, and the owner set the bound.** A held entry waits for its recorded
`destSlot` for a configurable timeout. The default is 24 hours. The sidecar then bounces
the organism home by itself, and records the reason.

**The owner accepts the residual risk.** One case duplicates: the far sidecar took
custody, died before its acknowledgement, and replays its own journal on its return. That
case needs an invisible delivery and a return after the timeout. Weigh it against the
alternative. An unbounded hold strands the organism in a file that nobody reads, and a
stranded organism is invisible.

**The deadline lives in the journal entry.** The timer starts at the forward. A sidecar
that restarts therefore reads one deadline back, and never starts a fresh one.

**Keep the escape hatch for earlier intervention.** `multiverse-sidecar --release-inflight
<migrationId> bounce|drop` releases one held entry by hand, before the timeout expires.
The command names the duplication risk in its own output. It is the custody twin of
`--release-slot`, and it is a deliberate act on the machine that holds the journal.

**Report every bounce that a timeout caused.** The status page and `ringstat` both name
it. An automatic bounce is a fact the operator reads, not a silent repair.

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
- No flag. The sidecar sets `exportEdges` from the grant, so a one-row map gives one edge

The per-tick cost is two comparisons for each organism. The M3 band test already runs each
`FixedUpdate`, so the second band adds no new pattern.

## Design Question 7 — How much grid M4 builds

**The owner's answer: all of it.** The live two-axis grid ships in M4, and the exit test
proves it. This overrides the earlier recommendation, which shipped the vertical lanes
behind a flag that was off.

**The wire deadline still holds.** `exportEdges` and the plural `EDGE_STATUS` break
Contract A. M5 puts the wire in front of strangers. A break before M5 costs a rebuild of
three binaries and one mod. A break after M5 costs a version-migration path for people the
owner cannot reach. This argument put the abstractions in M4, and it is unchanged.

**A flag that ships off proves nothing.** An untested lane is an unknown, and M5 exports
every unknown to strangers. The flag also hides the two items that need the game: the
second capture band under real organisms, and the corner rule. A flag moves that risk into
M5. It does not remove it.

**The rig arithmetic answers the hardware objection.** The earlier recommendation called
six instances hardware that does not exist. Measure it instead:

| Fact | Value |
|---|---|
| Game instances that run together on the main machine today | 3, comfortably |
| Memory for each instance | about 550 MB |
| Main machine memory | 128 GiB |
| Instances that the memory alone allows | far more than six |

Four or five instances on the main machine, plus one or two on the second computer, give
six. **Attempt 4+2 or 5+1 across the two machines.** Watch CPU, not memory, because each
instance runs its own simulation at 20×.

**The smallest honest map is 3×2.** A 2×2 map is degenerate on both axes, exactly as a
two-slot ring is. One hop east and one hop north return an organism to its own row and its
own column. A 3×2 map gives a real east cycle of three, and it is M4's target.

**Name the fallback, and keep it a fallback.** A degenerate proof — a 1×2 map on one
machine — is acceptable **only** when the two machines cannot hold six instances. Record
the measurement that forces that choice. Do not take it first.

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

The owner saw a mass of organisms on the west border after the overnight run. The owner
also named the mechanism. M4 therefore builds the answer, and measures it.

**The sleeping machine dammed the flow.** One slot went dark for two hours. Its inbound
deliveries queued in its west neighbour's journal. That neighbour also lost its export
target, and contained the organisms with no open lane. Both piles released together at
wake, and the entry edge took all of them at once.

**This is a burst, not a rate.** A steady rate spreads a crowd across hours. A dam delivers
hours of traffic in seconds. The two need different answers, and the dam is what the owner
saw.

**M4 ships a delivery rate limit.** The sidecar releases each `MIGRATE_IN` from its journal
at a configurable maximum spawn rate, per unit of simulated time. Every burst then trickles
in: sleep recovery, an edge that reopens, and a future mass migration.

**Pace at the sidecar, not at the mod.** The journal is where a burst accumulates, and the
sidecar owns the journal. The mod keeps one arrival path and needs no change, so decision
D8 holds. Pacing changes when an organism arrives. It never changes whether an organism
arrives, and custody is untouched.

**What the geometry does today.** An organism leaves the east edge at `y`. It arrives at
`x = −S + W + margin` with the same `y`, the same velocity and the same heading
(`m2_findings.md` §4.4). It therefore arrives inside the world and travels east.

**Four secondary contributors stay open.** They raise the resting crowd. They do not
explain the pile-up, and the metric ranks them:

1. **Arrival rate.** Slot 1 imported 6 003 organisms in 12.56 hours, into a population near
   22. Each world receives about 21 organisms each simulated hour. Most of the population
   is a recent arrival.
2. **Transverse concentration.** `exitPosition` clusters at the source. The receiver mirrors
   it, so arrivals cluster in the same band of `y`.
3. **No food under the arrival band.** Pellets spawn inside a zone only (`m2_findings.md`
   §1.4). An arrival band outside every zone holds hungry organisms.
4. **The shaded band.** An organism that walks west leaves the square and travels to 4000
   before the wrap fires. It stays alive and visible outside the west edge for a long time.

**The metric ships too, and it verifies the rate limit.** It is cheap, and it is the only
proof that a burst arrived as a stream. Add to the mod's `[M2-CROSSING]` series, once each
simulated minute:

- The count of organisms inside the entry band
- A histogram of arrival positions along the entry edge
- The residence time between an arrival and its first departure from the band

Add one series at the sidecar: the journal depth waiting on the rate limit. A depth that
never falls names a limit that is too low.

**Arrival-position spreading stays parked.** Spreading arrivals along the entry edge, in
place of mirroring `exitPosition`, flattens the histogram. It also breaks the transverse
continuity that decision D3 buys. An organism that leaves at one `y` then arrives at a
different `y`, and the one-continuous-world illusion weakens. The dam caused the crowd, so
pay that price only if the metric shows a crowd after the pacing works.

## Risks

### Risk 1 — The six-instance rig

The sign-off puts the live grid in M4, so the rig grows from three instances to six. The
old scope boundary — the wire only, and nothing the game needs — is retired with it. The
memory arithmetic is comfortable (Question 7). Nothing else is measured.

Four things scale with the instance count:

- **CPU.** Each instance runs its own simulation at 20×.
- **BepInEx logs.** The second instance already needs `LogOutput.log.1`, because the first
  holds a lock.
- **Per-instance configuration.** The environment variables travel through `WSLENV`.
- **The far end.** `start-slot2.ps1` and `stop-slot2.ps1` each drive one instance today.

Measure the rig before WP8 depends on it. Start six instances, run them for one hour, and
record the simulation rate of each one. A rig that loses speed at six proves a slower grid.
It does not prove a broken one.

Take the 1×2 fallback of Question 7 only against a recorded measurement.

**Measured 2026-08-06, and the answer was BepInEx, not the hardware.** Six games run
together at about 550 MB each. BepInEx hands out five log files on this install, and an
instance with no log file never loads the mod. Five real games is therefore the local
ceiling, so slot 6 moved to the second computer. The map kept its 3×2 shape and the fallback
stayed unused. `dev_environment.md`, *The five-instance ceiling*, holds the numbers.

### Risk 2 — Healing interacts with at-most-once custody

Decision D2 refuses duplication. Route-around introduces a second destination for one
organism. The two rules meet in each orphaned journal entry.

Question 2 holds the answer: re-route only under a relay proof of non-delivery, and hold
otherwise. The risk is not the rule. The risk is an implementation that treats silence as
proof.

The bounded hold adds one accepted exception, and the owner owns it. A far sidecar that
took custody, died before its acknowledgement, and returns after the timeout produces two
organisms. Do not widen that exception. Every other path stays at most once.

Four tests gate this risk:

- Kill a destination sidecar **after** the relay forwards a frame and **before** the
  acknowledgement. Restart it. The organism exists exactly once.
- Stop a destination peer **before** any frame reaches it. The sender re-routes. The
  organism exists exactly once, in the new destination.
- Hold an entry for one hour against a dark slot. Return the slot. The organism arrives,
  and no copy exists anywhere else.
- Set the hold timeout to minutes. Hold an entry against a slot that stays dark. The
  organism bounces home, exists exactly once, and the bounce is reported.

A count of two organisms in the first three tests fails the milestone.

### Risk 3 — The save timer competes with the simulation

A save serializes every organism, every pellet and every zone. The rig runs at 20× or more.
A save that stalls the simulation reduces the very throughput that M4 measures.

**The owner set the budget: a periodic save stalls the simulation for 2 seconds at most.**
That number is the test bar for the save-timer work item, and it selects the design. A
simple synchronous save is the shipped answer while it holds the budget.

Measure the save cost first. Record the duration against the population and the world size.
Then choose the cadence from the measurement, not from a guess.

Two rules limit the damage. Never save two worlds on one machine at the same moment. Never
start a save while a `MIGRATE_IN` waits in the mod's queue. Both rules matter more at six
instances than at three (Risk 1).

A save that breaks the 2-second budget needs a different cadence, or a save path that does
not block the tick. It does not need a different decision. Decision D14 accepts a slower
rig over a lost night.

**Measured 2026-08-06, at five concurrent worlds: 241 ms to 538 ms.** The budget held with
room to spare, the simple synchronous save stays, and the save lock never deferred.

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

### Risk 8 — The portal renders nothing, and reports no error

`m4_portal_findings.md` landed on 2026-08-05. The research is complete, and the portal is
an M4 deliverable (WP7).

One risk survives that research, and it is ranked first there: the camera's layer and its
culling mask. Layers live in prefabs, not in the assembly, so no static evidence settles
them. A portal on a layer the camera does not draw is invisible, and nothing writes an
error.

Mitigate it in the first runtime session. Copy the layer from a live zone renderer. Log
`Camera.main.cullingMask`, the donor layer and the rectangle's world bounds. Confirm that
`Shader.Find("Shapes/Rect Additive")` returns a shader.

The second risk is the sorting order against organisms and pellets. The assembly does not
answer it. Make the order configurable, and tune it in the same session.

**Closed 2026-08-06.** Every local slot logged `event=BUILT` with its layer, its culling
mask and its strip bounds, and every open edge showed a strip. The owner confirmed the look
on screen, at the shipped `[M4] PortalSortingOrder = -50`. The portal renders, and it needed
no tuning.

### Risk 9 — The rate limit interacts with the hold timeout

Pacing delays each `MIGRATE_IN`. `MIGRATION_ACK` waits for the mod's `MIGRATE_IN_ACK`, so
pacing also delays the sender's release. A deep backlog at a live peer then looks like
silence at the sender.

The sign-off scopes the timeout narrowly, and that scope is the answer. **The hold clock
runs only while the destination is dark.** A live and connected destination is slow, not
orphaned. Stop the clock when the destination returns.

Do not answer this by moving the custody gate. `MIGRATION_ACK` follows the receiving mod's
`MIGRATE_IN_ACK` today (`contract-b-m3.md` §6.7, §9), and that gate makes the spawn the
proof of delivery.

Test it. Set a low rate limit, a short hold timeout and a deep backlog against a live peer.
No entry bounces, and no organism duplicates.

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
| 14 | `contract-b-m3.md` §9, §12 | State the bounded hold: the timeout, its 24-hour default, the automatic bounce and the accepted duplication case. The clock runs only while the destination is dark. | D2, D12 |
| 15 | `contract-a.md` §7.5, §10 | State that the sidecar paces inbound delivery, and add the rate limit to the tunables. Replay keeps its journal order. | D15 |

## Work Packages

Eight packages. WP1 gates the wire work. WP6 gates the exit test.

**Delivery: all eight packages are done, 2026-08-06.** WP1 to WP5 and WP7 landed on
2026-08-05. WP6 and WP8 closed with the LAN exit test on 2026-08-06, and that run also
closed the two measurements WP4 and WP7 left open. Each package carries its own status line
below.

### WP1 — The contract amendments

**Depends on:** this document.
**Needs the game:** no.

- The fifteen items of *Contract Changes Needed*
- One worked example for each changed message
- The version bump, and a migration note for the M3 rig

Write WP1 before any code. M2 and M3 both paid for the alternative.

**Status: DONE, 2026-08-05** (`8d6bdce`). `contract-a.md` §15 carries A18 to A25.
`contract-b-m4.md` supersedes `contract-b-m3.md` at `contract-b/3.0`.

**Reconciled 2026-08-05**, after both implementations landed. Six resolutions from the code
are now law: `contract-a.md` §15 A26 and A27, and `contract-b-m4.md` §14 B4 to B7. All six
are clarifying. Neither version moves.

**Amended again 2026-08-07, for species identity in the envelope** (`18954a6`). One OPTIONAL
`species` block on two Contract A messages and one Contract B message, nothing removed and no
type changed, so **both minors moved and neither major did**: `contract-a.md` §16 carries
A30 to A33 at **`contract-a/2.1`**, `contract-b-m4.md` §15 carries B9 and B10 at
**`contract-b/3.1`**. The rollout to the living deployment on
2026-08-07 left the far end on the previous minor on purpose and it kept working, which is
the compatibility rule demonstrated rather than asserted.

**Amended once more the same day, for the names the game itself will not spell cleanly**:
about 2% of generated name halves carry an edge space, which the wire rule refuses, so those
organisms were migrating with no species block at all. `contract-a.md` §16 **A34** makes the
*exporting* mod normalize each half's whitespace before it validates — trim the edges, collapse
internal runs to one U+0020 — and makes the importer compare on the normalized form so a
repaired name still matches the raw-form species a destination world grew for itself.
**No version moved**: A34 adds no field and relaxes no rule, so both wires stayed at
`contract-a/2.1` and `contract-b/3.1`, and no sidecar, relay or archive changed.

**Both wires have moved twice more since, and this document is not the record of it.** The
species census took both minors to `contract-a/2.2` and `contract-b/3.2`
(`contract-a.md` §17 A35–A37, `contract-b-m4.md` §16 B11–B12), and **D17's two-way lanes**
took Contract B alone to **`contract-b/3.3`** (`contract-b-m4.md` §17 B13–B15) while Contract
A stayed at **`contract-a/2.2`** — its fields have accepted four export edges since A18, so
§18's A41 finds no bump to make. **The contracts are the authority on the current wire; this
section is the M4 work-order history.** Two of the same wave's decisions land outside the wire
entirely: D18's migration exclusion is mod-local policy (`contract-a.md` §18 A39), and D19's
hop animation is archive-and-page only (`contract-b-m4.md` §17 B14). The third, **D20**,
raises `inboundRatePerSimMinute` from 2.0 to 12.0 — see Risk 9 below, whose answer is
unchanged and whose numbers are not.

### WP2 — The relay: healing, insertion and handover

**Depends on:** WP1.
**Needs the game:** no.

- The deliverability filter, and the effective-neighbour walk
- The two published maps: structure in `PEER_STATUS`, effect in `SECTOR_GRANT`
- Insertion at a position, with the tail as the fallback
- The explicit non-delivery answer
- `--handover-slot`, and the held-entry report for both operator commands
- Per-axis walking, east and north, on the live grid
- The coordinate map: fill a hole, extend an axis, and refuse a ragged rectangle

**Status: DONE, 2026-08-05** (`823a70f`). `go/internal/relay/` and `go/internal/mapwalk/`
carry every item. `--reserve-slot <peerId>[@<col>,<row>]` places a peer before it connects.
`--handover-slot <n>=<peerId>` rebinds a reservation.

### WP3 — The sidecar: handoff state, re-route and metrics

**Depends on:** WP1, WP2.
**Needs the game:** no.

- The handoff state on each journal entry, and its hold deadline
- The re-route, bounce and hold rule of Question 2
- The hold timeout, its automatic bounce, and the dark-only clock of Risk 9
- `--release-inflight`
- The delivery rate limit of Question 9, and the journal-depth series behind it
- Outbound re-evaluation at reconnect (Question 4)
- Durable metrics: population, crossings, edge state, custody depth, save receipts
- Population on the ring claim
- One lane for each export edge, east and north, both live

**Status: DONE, 2026-08-05** (`823a70f`). `go/internal/sidecar/` carries every item.
`--position <col>,<row>`, `--insert-after-slot` and `--insert-axis` place the peer.
`--list-inflight` and `--release-inflight` operate the custody hatch.

### WP4 — The mod: saves, edges and instrumentation

**Depends on:** WP1.
**Needs the game:** yes.

- The periodic save, the save on quit and the rotation
- The save receipt line and its metric record
- The save-cost measurement of Risk 3, against the 2-second budget
- `exportEdges` on `CONFIG_UPDATE`, and one edge state for each
- The second capture band and the corner rule, in production
- The second passive entry edge, and its entry-position mapping
- The entry-edge metrics of Question 9

**Status: DONE, 2026-08-05** (`efd74a1`); its last measurement closed 2026-08-06.
`WorldSaver.cs` saves, verifies, rotates and prunes. `CrossingStats.cs` reports one `[M2-CROSSING]` line per
export edge and one `[M4-CROWDING]` line per entry edge. `MultiverseConfig.cs` parses
`exportEdges` and derives the entry and border sets. The corner rule runs in
`MultiverseClient.OnBodyTick`.

**The save-stall re-measure is done, 2026-08-06.** Five concurrent local worlds saved on
interval through the exit test. Every stall fell between **241 ms and 538 ms**, against the
2 000 ms budget. Phase 6 sampled the five most recent, at about 280 ms to 440 ms. No save
logged `event=BUDGET_EXCEEDED`, and the save lock never had to defer.

**A game-side defect stopped two worlds saving at all — guarded 2026-08-07** (`SpeciesHistoryGuard.cs`,
mod **0.5.2**). Slot 5 lost 27 consecutive periodic saves on 2026-08-06 and slot 4 lost 19 plus
the save on quit on 2026-08-07, every one of them
`IndexOutOfRangeException` in `Utility.LogLikeSpeciesDataArray.SavePointToArrays`, under
`GlobalLineageManager.SaveStateBin`. **It is not our bug**: the same stack, from the same call
sites, is in the slot-5 log under mod **0.4.0**, which has no species block, no
`SpeciesRegistry` and no `CreateLocal`. The cause is in
`Utility/LogLikePresenceArray.cs`, which describes a species' live history window twice —
as four apparition/disappearance indices, and as the running `nPresentAtScale[]` count —
and lets the two drift. `SaveStateBin` sizes its temporary arrays from the count (line 138)
and fills them from the indices (lines 140-160), so a count that reads low overruns the
arrays. The count goes low because `nPresentAtScale[i]--` at line 80 fires without its
paired `++` at line 86 whenever the disappearance front reaches a scale that has never held
a living point: `0 - 1` on a `ushort` is 65535, saves keep working (over-sized) and the
species also stops being prunable, and then the species is repopulated, the `++` wraps 65535
back to 0, and every save from that moment throws. **What the multiverse supplies is the
traffic** — arrivals that empty a species when they migrate on and later repopulate it, on
hundreds of species per world instead of a handful.

The guard is a Harmony prefix on `GlobalLineageManager.BytesSpace` **and** `.SaveStateBin`
(both, because `SaveableBinStack` sizes the whole buffer from the first and then hands the
second a slice) that recomputes `nPresentAtScale` to exactly the window the save loop will
walk. That is the same quantity the game's own `RefreshPresentsFromIndices` computes during a
prune merge, so the value written is the game's; unlike that method the guard leaves
`nDataAtScale` alone, because that field steers `Push`'s cascade. On a healthy array it is a
no-op. `e2e/species-guard-check.sh` is the regression check: with
`MULTIVERSE_SPECIES_GUARD=0` the dev-channel `speciespoison` verb reproduces the incident's
stack on demand, and with the guard at its default the same poison saves cleanly and the
world reloads. **Fourteen of the 124 species in a live slot-1 world were already carrying a
drifted counter** when the guard was first run against it, which is how routine the drift is.

### WP5 — The archive: the status page

**Depends on:** WP1, WP3.
**Needs the game:** no.

- A live HTML status page: the map shape, slots, liveness, effective lanes, bypasses,
  populations, per-lane flow, custody depth and the last save of each world
- Each bounce that a hold timeout caused
- The honest-gap rule of Risk 4
- `ringstat`, over the same data, in the terminal

**Status: DONE, 2026-08-05** (`823a70f`). `go/internal/archive/status.go` and `page.go` serve
the page on `--http`, default `127.0.0.1:8791`. `go/cmd/ringstat/` reads the same data.
An absent stat renders as `unknown`, and `statsAsOfMs` ages every block.

### WP6 — The rig: recovery, preservation and the join kit

**Depends on:** WP2, WP3, WP4.
**Needs the game:** yes.

- The scripted single-instance recovery of Question 8
- Log preservation in the stop script, and the refusal in the start script
- The far-end save and preservation steps, in the two PowerShell scripts
- The join kit: the bundle, the token, the relay address and one page of steps
- **The six-instance rig**: 4+2 or 5+1 across the two machines, in a 3×2 map
- The rig measurement of Risk 1, before WP8 depends on it
- Per-instance log names, environment variables and far-end scripts, for six slots

**Status: DONE, 2026-08-06.** `e2e/run-m4.sh` is the local rehearsal — a 3×2 map of six
slots, five real games and `bin/fakemod` for the sixth, because BepInEx caps this install at
five log files and an instance with no log file never runs the mod at all.
`e2e/run-m4-lan.sh` is the exit-test rig: the same map with **slot 6 on the second
computer**, which retires the synthetic peer and makes the shape an honest 5+1. The far-end
bundle is on `contract-b/v3`, port `8795`, `MULTIVERSE_EXPORT_EDGES=E,N` and slot 6 at (2,1).
`dev_environment.md`, *The M4 rigs*, carries both command lists and the reasoning for the slot
choice.

**The LAN run happened on 2026-08-06, and it passed.** The owner ran the elevated networking
steps here and one setup pass on the second computer. The run also produced the save-stall
re-measure of Risk 3. The rig measurement of Risk 1 is recorded in `dev_environment.md`,
*The five-instance ceiling*, and it moved slot 6 to the second computer. See
*Exit Test → Result*.

### WP7 — The portal

**Depends on:** WP4. Input: `m4_portal_findings.md`.
**Needs the game:** yes.

Build the recommended approach of the findings, and nothing else:

- One `Shapes.Rectangle` for each open edge, additive blend with a linear-gradient fill
- A chevron `DashOffset` flow: outward on an export edge, inward on an entry edge
- `ThicknessSpace.Pixels`, which holds the line legible across the 800× zoom range
- Geometry from `BorderGeometry.W` and `BandInnerBoundary(edge)`, and the live
  `SimulationSize` callback, so the visual cannot drift from the capture rule
- One expanding additive `Disc` for each migration event, rate-limited, hooked at
  `MigrationExporter` and `MigrationImporter`
- The runtime checks of Risk 8, in the order the findings give

Do not use immediate mode. `Shapes.Draw.*` needs a `ShapesRenderFeature` on the URP
renderer, and a mod cannot add one. Keep `ParticlesMaster` as an optional extra only: the
game silences it when the player turns pheromone display off.

**Status: DONE, 2026-08-05** (`efd74a1`), and **confirmed on screen 2026-08-06**.
`PortalVisual.cs` draws one strip per open export edge, and one per declared entry edge.
`MULTIVERSE_PORTAL` and
`MULTIVERSE_PORTAL_FLOURISHES` gate it, both on by default. `[M4-PORTAL] event=BUILT` reports
the layer, the culling mask, the shaders and the first strip's world bounds, which is Risk 8's
runtime check. Entry-portal visibility is now defined in `contract-a.md` §15 A27.

**The two looks are closed, and the owner closed them.** Exit-test phase 7 proved the
runtime half on every local slot: each slot logged `event=BUILT`, and each open edge showed
its strip. The owner then watched the live rig and ratified the visual result — "portals
look great". That answers the sorting order at `[M4] PortalSortingOrder = -50`, and it
answers the zoom legibility. **This was the last item in M4 that only a person could
settle.**

### WP8 — The exit test

**Depends on:** WP2, WP3, WP4, WP5, WP6.
**Needs the game:** yes.

Build the test on `e2e/run-m3-lan.sh`. The ring form-up, the command file and the archive
checks all carry over. The rig grows to six instances, so the harness starts and stops
each slot by number.

**Status: DONE, 2026-08-06. The run passed.** `e2e/run-m4-lan.sh` is that test — five
real games here, slot 6 real on the second computer, and the far end never driven. Its phases
map onto the parts below: phase 1 to Part 1's form-up, phases 2 and 3 to the two-axis current
(phase 3 waits for a **natural** crossing out of the far world, because nothing here may force
one), phase 4 to Parts 2 and 3, phase 5 and phase5far to Part 5, phase 6 to Part 7's save
half, phase 7 to Part 8, and phase 8 to Parts 9 and 10. Part 4 (a brand-new instance) and the
bounded-hold case are proven by `run-m4.sh` phases 5 and 7 instead, and `run-m4-lan.sh`'s
header states why neither can be re-proven against a machine the rig refuses to command.
Part 2 changed on 2026-08-05 — see the amendment under *Exit Test*, Part 2.

## Exit Test

The test runs a live **3×2 map**: six instances, four or five on the main machine and one
or two on the second computer. Row 0 holds slots 1 to 3. Row 1 holds slots 4 to 6. Each
peer exports east and north, and receives west and south.

Attempt that shape first. Take the 1×2 fallback of Question 7 only against a recorded rig
measurement (Risk 1). A fallback run states which parts it did not prove.

**The run took the full shape, and the fallback stayed unused.** *Result* below records it.

### Part 1 — The two-axis current runs

1. Start the rig. Confirm six slots, in a 3×2 map, with every lane open.
2. Record the per-lane flow for ten minutes, on both axes.
3. Track one organism by entity ID across four hops or more.

The test passes when all of these conditions hold:

- Every peer exports on two edges, and receives on two edges.
- East flow and north flow are both non-zero, on every lane.
- The tracked organism drifts diagonally. It returns to its own slot only after a whole
  number of both circuits.
- The corner rule gives each organism in both bands exactly one export edge. No organism
  exports twice.
- The map holds exactly one copy of each organism, counted by entity ID.

### Part 2 — The map heals around a hard stop

4. Hard-kill the game and the sidecar of slot 5, mid-flow.
5. Watch its west neighbour's export edge, and its south neighbour's export target.

The test passes when all of these conditions hold:

- The east lane re-pairs along row 1. Slot 4 exports east to slot 6, and skips slot 5.
- The east lane stays open. Row 1 still holds two deliverable slots.
- **Slot 2 closes its north lane with `no_peer`.** Column 1 holds only slot 2 and slot 5.
- The east current continues. The north current continues on both other columns.
- The status page shows slot 5 as bypassed, with the time it went dark.
- Each organism in flight to slot 5 at the kill is held, re-routed or bounced by the rule
  of Question 2. Each entry states which answer it took, and why.
- The map holds exactly one copy of each organism, counted by entity ID.

**Amendment, 2026-08-05: a height-2 map proves the degenerate answer, not the re-pair.**
The first three conditions above replace an earlier pair that required the north lane to
re-pair and required no export edge to close. Those two conditions are unsatisfiable on a
3×2 map, and Contract B §2.1 says why. A lane needs a third party to route around a gap.
Column 1 of a 3×2 map holds two slots. Kill one and the survivor has nothing to re-pair to.

The corrected conditions assert the **correct degenerate behaviour**: the width axis routes
around the dead slot, and the height axis closes with `no_peer`. That result proves
route-around works against a degenerate axis. A closed north lane here is a pass, not a
failure, and an implementation that re-pairs it is defective.

**The full north re-pair applies only on a map of height 3 or more.** Record it as the 3×3
stretch goal. Run it when the two machines hold nine instances, and measure the rig first
(Risk 1). A 3×3 run asserts the original conditions unchanged: both lanes re-pair, and no
export edge closes.

### Part 3 — The dead slot splices back in

6. Start slot 5's sidecar. Confirm the reclaim of its slot number and its coordinate.
7. Start slot 5's game against its last periodic save.
8. Watch both neighbours' export targets.

The test passes when all of these conditions hold:

- Slot 5 returns to the same slot number and the same coordinate.
- Both lanes re-pair back to slot 5, with no operator action.
- Slot 5 replays every un-acknowledged inbound entry to its mod.
- Slot 5's world holds the population of its last save, and not the population of its
  first start.
- Any held entry that names slot 5 is delivered, and the organism exists exactly once.

### Part 4 — A brand-new instance splices into a live map

9. Start a seventh sidecar with a new `peerId`. Ask the relay to extend the map to 4×2.
10. Start a seventh game, on a new world.

The test passes when all of these conditions hold:

- The relay grants the newcomer a position in the new column, and renumbers no other slot.
- The second position in the new column is a hole, and both axes route around it.
- Its west neighbour and its south neighbour re-pair, within one status broadcast.
- No organism in flight at the moment of the insertion changes its destination.
- Organisms cross into the new slot and out of it, without a forced export.
- The archive records every new lane.

A rig that cannot hold a seventh instance proves the same rule differently. Release one
slot, which leaves a hole, and splice a new instance into that hole. Record the
substitution.

### Part 5 — A burst arrives paced, not all at once

11. Set the delivery rate limit to a value the entry-edge metric resolves.
12. Hard-stop one slot. Let its inbound traffic and its west neighbour's export pile
    accumulate for one hour.
13. Wake the slot. Start its sidecar, then its game.
14. Record the arrival histogram, the entry-band count and the journal depth, each
    simulated minute.

The test passes when all of these conditions hold:

- Delivery never exceeds the configured rate, in any sampled minute.
- The journal depth falls steadily to zero.
- The entry-band count stays below the count the T1 run recorded after its outage.
- No held entry bounces while its destination is live (Risk 9).
- Every accumulated organism arrives, and each one exists exactly once.

### Part 6 — The resume test

15. Stop the rig. Keep every journal.
16. Start the rig against `e2e/data/slot-1/journal` from 2026-08-04.

The test passes when migration `9d6db335-b1ae-433e-a44a-bb2109912913`, entity
`2004967003`, arrives in slot 2 and exists exactly once in the map.

### Part 7 — Periodic saves, proven by a kill and a reload

17. Run one world for one hour with the periodic save on.
18. Record its population and its simulated minute.
19. Hard-kill the game.
20. Restart it against its last periodic save.

The test passes when all of these conditions hold:

- The reloaded world is within one save interval of the recorded state.
- The reloaded world is **not** the state of the first start.
- The save receipts name every save of the hour.
- No save stalls the simulation for more than 2 seconds, at six instances.
- `worldstat` reads the reloaded save with no error.
- The recovery script runs the whole sequence with no manual step on this machine.

### Part 8 — The portal is visible

21. Watch one export edge and one entry edge, at four zoom levels.

The test passes when all of these conditions hold:

- Each open edge carries a portal strip. A closed edge carries none.
- The chevrons flow outward on an export edge, and inward on an entry edge.
- The strip stays legible at `orthographicSize` 5, 250, 2000 and 4000.
- A migration event draws its ring, and the ring never blocks the simulation.
- The strip follows a live `SimulationSize` change.

### Part 9 — The status page shows it all

22. Read the status page after each part above.

The test passes when the page states all of these values, at each point above:

- The map, its shape and every hole
- The liveness and the population of each slot
- Each effective lane, and each bypass with its start time
- The custody depth of each sidecar, and each bounce a hold timeout caused
- The last save of each world
- The entry-edge crowding metric, and the paced journal depth

An unknown value reads as unknown. A zero reads as a measurement.

### Part 10 — The error sweep

The test passes when no log holds an error at the end of the run. Sweep every BepInEx log,
sidecar log, relay log and archive log.

### Result — PASS, 2026-08-06

`e2e/run-m4-lan.sh` ran the full 3×2 map: **six real worlds across two computers**, five
here and slot 6 at (2,1) on the second computer. Twelve lanes carried traffic on both axes.
No synthetic peer took part, and the rig sent no command to the second computer.

**Phase 1 — the grid forms, including the far slot. PASS.** Six slots reported a 3×2 map.
Every peer opened two export edges and two entry edges. All twelve lanes read `peer_live`.

**Phase 2 — forced hops on both axes across the LAN. PASS.** Each forced organism reached
the far world byte-equal, and the census counted each one exactly once.

**Phase 3 — the far world exports on both axes with nobody driving it. PASS.** Slot 6
crossed east and north on its own traffic. Both organisms landed exactly once.

**Phase 4 — local slot 5 hard-killed mid-column. PASS.** The width axis healed **across the
LAN**: slot 4 bypassed the dead slot and exported east to slot 6 on the second computer.
Slot 2 closed its north lane with `no_peer`, which is the degenerate answer Contract B §2.1
requires on a column of two. Slot 5 then spliced back in and reclaimed both its slot number
and its position at (1,1).

**Phase 5 and phase5far — arrival pacing. No LAN result, by the script's own design.**
`run-m4-lan.sh` phase 5 runs the rehearsal's pacing test verbatim, against a local dam. It
proves nothing the rehearsal has not proven, and `all` does not gate on it. `phase5far`
drives that dam on the far world instead. It needs two commands typed on the second
computer, and D9 forbids this rig to send them, so `all` excludes it. A test that blocks on
a person is confirmation, never the only evidence.

**Phase 6 — periodic saves. PASS.** The five local worlds saved on interval and stayed
inside the budget. The sampled stalls ran about 280 ms to 440 ms against 2 000 ms, and the
whole run stayed between 241 ms and 538 ms. The far world proved its own save through the
`HEARTBEAT.lastSave` receipt on the wire. That receipt is the only save evidence an undriven
peer produces.

**Phase 7 — the portals. PASS.** Every local slot logged `[M4-PORTAL] event=BUILT` and
showed a strip on each open edge, which completes Risk 8's runtime check. The owner then
looked at the live screen and confirmed the result — "portals look great". That closes the
last item in M4 that only a person could settle.

**Phase 8 — the close-out. PASS.** Exactly-once held across both computers. The archive
recorded every cross-machine lane. No log held an unexplained error, and the teardown of
this machine was clean.

**Three parts came from the local rehearsal, and `run-m4-lan.sh` states why.** Part 4 (a
brand-new instance), Part 5 (paced arrivals) and Part 6 (the resume test) are proven by
`run-m4.sh` phases 5, 6 and 3. Phase 7 of the same rehearsal proves the bounded-hold case.
Each of the four needs a command sent to the slot under test, and the far end takes no
commands (D9). The resume test also stays local, because `e2e/data/slot-1/journal` is the
only copy of its input.

**A harness caveat, and phase 6 found it.** Phase 6 failed on its first run, purely on
timing. Phase 4 restarts slot 5, and a restarted world restarts its save clock with it. The
check then ran before that world's first two-minute tick. The re-run passed with no code
change. **Wait one save interval after any slot restart before you assert a save.**

**The rig came back up, and it stays up.** The deployment now runs as the living multiverse:
six worlds, twelve lanes and about 198 organisms at bring-up. The archive binds
`0.0.0.0:8796` so the status page is readable from the LAN — see `dev_environment.md`,
*Owner steps*.

## Deliverables

Every item is delivered, except the last one. The date beside each item is the date it
landed.

- **Done, 2026-08-05.** The wire specification, from WP1: `contract-a.md` and `contract-b`
  at a new major version
- **Done, 2026-08-05.** A relay with lane re-pairing, insertion at a position, handover, the
  coordinate map and the non-delivery answer
- **Done, 2026-08-05.** A sidecar with the handoff state, the re-route rule, the bounded hold
  and its automatic bounce, `--release-inflight`, the delivery rate limit and durable metrics
- **Done, 2026-08-05.** A mod with periodic saves, save-on-quit, save rotation, plural export
  edges, the second capture band, the corner rule and the entry-edge metrics
- **Done, 2026-08-06.** A live 3×2 map, running on both axes across the two machines. It
  runs still, as the living deployment
- **Done, 2026-08-05.** An archive with a live status page, and a `ringstat` command
- **Done, 2026-08-06.** A visible portal at each open edge, built to
  `m4_portal_findings.md`, and confirmed on screen by the owner
- **Done, 2026-08-05.** A scripted single-instance recovery procedure
- **Done, 2026-08-05.** A join kit for a new instance
- **Done, 2026-08-05.** Log preservation in the start and stop scripts
- **Done, 2026-08-06.** The save-cost measurement of Risk 3, against the 2-second budget:
  241 ms to 538 ms at five concurrent worlds
- **Done, 2026-08-06.** The rig measurement of Risk 1, at six instances. It found a BepInEx
  ceiling of five log files on this install, and it put the sixth world on the second
  computer
- **Done, 2026-08-05.** The entry-edge crowding measurement of Question 9, which verifies
  the pacing. `run-m4.sh` phase 6 holds it
- **Done, 2026-08-06.** The automated M4 exit test, with its recorded result. See
  *Exit Test → Result*
- **Open.** An `m4_findings.md` with the research results of the milestone. This is the one
  M4 deliverable nobody has written. The results live in this document, in the rig logs
  under `e2e/logs-m4-lan/` and in `m4_portal_findings.md`

## Owner Sign-Offs

The owner signed this design off on 2026-08-05, and overrode three recommendations. Every
call below is settled. The sections named beside each one carry the result.

1. **The full grid ships in M4** (Question 7, D13). The live two-axis grid ships and is
   proven in M4. **This overrides the recommendation** to ship the vertical lanes behind a
   flag that is off. The exit test attempts an honest 3×2 map: six instances, 4+2 or 5+1
   across the two machines. The premise of hardware that does not exist is withdrawn. The
   main machine already runs three instances comfortably, at about 550 MB each on a
   128 GiB host. A degenerate proof is the fallback, and only when the two machines
   genuinely cannot hold six instances.
2. **A hold is bounded, and then it bounces** (Question 2, D2). An orphaned in-flight
   organism waits for a configurable timeout, 24 hours by default, and then bounces home
   by itself. An orphan is a forwarded organism with no proof of non-delivery and a dark
   destination. **This overrides the recommendation** of an unbounded hold. The owner accepts the residual
   duplication risk of the invisible-delivery case. At-most-once now carries this one
   bounded exception, and no other. `--release-inflight` stays, for earlier intervention.
3. **Arrival pacing ships now** (Question 9, D15). The owner named the mechanism: the
   sleeping machine dammed the flow, and the queued custody deliveries plus the
   neighbour's contained export pile released together at wake. M4 therefore ships a
   delivery rate limit at the sidecar. **This replaces the measure-first-versus-mitigate
   question.** The crowding metric ships too, because it is cheap and because it verifies
   the pacing. Arrival-position spreading stays parked.
4. **The save budget is 2 seconds** (Risk 3, D14). A periodic save stalls the simulation
   for 2 seconds at most. That selects the simple synchronous save, and it gives the
   save-timer work item a bar to pass.

## Next Steps

Updated 2026-08-06, after the exit test passed. Every work package of M4 is closed. The
steps below carry the milestone's remainder into the living deployment and into M5.

1. **Keep the living deployment running.** Six worlds, twelve lanes, periodic saves on the
   rig's 2-minute interval here and the far end's 10-minute one. It is the first rig that
   survives its own operator, and the next overnight harvest measures that claim against T1.
2. **Re-run `run-m4-lan.sh lanhost` after every WSL restart.** The WSL address changes, and
   the portproxy behind `8795` then points at nothing. The far world drops out of the map
   until the owner re-adds it.
3. **Write `m4_findings.md`.** It is the one M4 deliverable still open. The inputs are this
   document, `e2e/logs-m4-lan/` and `m4_portal_findings.md`.
4. **Run `phase5far` once, when a person is at the second computer.** It confirms the pacing
   rule across the LAN. Phase 5 already proves the rule locally, so this is confirmation.
5. **Preserve the T1 journals and the harvested BepInEx logs.** Part 6 has no other input.
6. **Open M5** (public release). The join kit is its starting point, and the wire shapes it
   publishes are whatever the contracts say at the time — **`contract-a/2.2` plus
   `contract-b/3.3` as of 2026-08-07**, after the census and D17's two-way lanes. Read the
   contracts for the current pair rather than this line.
