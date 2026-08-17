# Bibites Multiverse — System Decomposition

## The Picture

```
  ┌──────────────────────────────────────────────────────────────────┐
  │                         One Peer Node                           │
  │                                                                 │
  │  ┌─────────────┐    Contract A     ┌──────────────────────┐     │
  │  │  BepInEx Mod │◄── localhost ───►│   Sidecar Daemon      │     │
  │  │    (C#)      │    WebSocket     │        (Go)           │     │
  │  └──────┬───────┘                  └──────────┬───────────┘     │
  │         │                                     │                 │
  │         │ Harmony patches                     │ Contract B      │
  │         ▼                                     │ (wire protocol) │
  │  ┌─────────────┐                              │                 │
  │  │  The Bibites │                              │                 │
  │  │   (Unity)    │                              │                 │
  │  └─────────────┘                              │                 │
  └───────────────────────────────────────────────┼─────────────────┘
                                                  │
   Sidecar↔sidecar transport is phased. Contract B message shapes are
   identical in both phases — only the pipe changes:

   Phase 1 (M2–M5): star through the relay     Phase 2 (M6): direct mesh
   ─────────────────────────────────────────   ──────────────────────────
              ┌──────────────────┐                 Peer B ◄────► Peer C
    Peer B ──►│ multiverse-relay │◄── Peer C          ▲            ▲
    Peer D ──►│ (spatial router) │                    └── Peer D ──┘
              └──────────────────┘             (relay degrades to bootstrap)

   The wiring is a star; the *map* is a grid (D8, D13). `multiverse-archive` (D11)
   sits beside the relay from M3 and records every envelope that crosses it.

   From M4 the map heals and grows (D12): a dark slot is routed around rather than
   waited on, and a peer splices in between two live slots. D13 generalizes the map
   again — the ring is the one-row case of the grid, and M4 runs the vertical lanes
   live on a 3×2 map.

   D17 makes every lane two-way (2026-08-07). Each of a peer's four edges is BOTH
   an export edge and an entry edge: a west export routes to the west neighbour,
   who receives it on its east edge, and the same holds per axis in both
   directions. The permanent one-way current of D8 is gone; what replaces it is a
   live border on all four sides, held safe by the arrival inset, the
   entry-immunity window and the outward-velocity capture test.
```

---

## Design Decisions

Recorded so divergences are deliberate, not accidental. Section references (§) are to
`initial_research.md`.

| # | Decision | Why | Consequence |
|---|---|---|---|
| **D1** | **Relay-first, P2P-ready.** MVP transport is a deliberately dumb relay server (the topology §6.1 recommends); direct libp2p arrives in M6 behind unchanged Contract B message shapes. | Full P2P puts NAT traversal, gossip, DHT, and CRDTs on the critical path, and the research itself judged a pure mesh unviable. The sidecar boundary means the mod never notices the transport change. | New `multiverse-relay` component. Ring insertion (D8) is relay-arbitrated until M6. "Dumb" is load-bearing: it is why the organism database is a separate service (D11) and not a relay feature. |
| **D2** | **Durable custody handoff, idempotent delivery.** The sidecar journals an organism to disk *before* ACKing `MIGRATE_OUT`; receivers dedup on `migrationId`; failed deliveries bounce back into the local sim. Preference: rare loss over duplication. | Destroy-on-ACK without persistence silently kills organisms on any crash; retries without dedup clone them. Loss reads as natural death; duplication reads as cloning. | Contract A gains `MIGRATE_IN_ACK/NACK`; the envelope gains `migrationId`; the sidecar owns a migration journal. See the custody chain under Contract A. ~~**Amended 2026-08-05 by the owner's M4 sign-off — at-most-once now carries exactly one bounded exception.** An organism forwarded to a slot that then went dark, with no proof of non-delivery, is **held for a configurable timeout (default 24 hours) and then bounces home automatically**. The owner accepts the residual duplication risk of the invisible-delivery case: a far sidecar that took custody, died before its ACK, and replays its own journal when it returns. An unbounded hold was the alternative and was rejected — an organism stranded forever in a journal nobody reads is the more likely loss, and it is invisible.~~ **Reversed 2026-08-17 by the same owner (`contract-b-m4.md` §25, B37): AT-MOST-ONCE CARRIES NO EXCEPTION.** *"we don't mind much on bibites being lost, we can see migrating as dangerous, and then there wouldn't be duplicates while making things simpler."* A forwarded frame is forwarded **once** — no hold, no re-forward, no clock, no automatic bounce. An entry with no answer within `forwardTimeoutMs` (24 hours) is recorded **lost**: the organism is gone, counted on `stats.lostForwardTotal` and readable on the map. **Migration is the dangerous act, and it is dangerous in one direction only.** The row's own preference sentence — *rare loss over duplication* — is now the whole rule rather than the rule with an exception attached. What stays: re-routing under a **proof** that no custody moved (a `pending` entry, or a NACK), because a statement is not silence and cannot duplicate; bounce-back on a NACK and on an entry that reached nobody; and `--release-inflight`, which is now the **only** way left to duplicate an organism and says so before it acts. |
| **D3** | **Map-edge borders; gateway towers deferred.** Migration triggers at designated map edges, and the mod owns whatever it takes to get organisms *to* an open edge (§5.1). | Edges preserve the illusion of one continuous world. Towers (§5.3) remain the fallback if crossing rates stay too low — parked, and revisited only if a real multi-peer world crosses far below M2's measured rate. **Mechanism corrected 2026-08-02 (`m2_findings.md`):** the original rationale — "vanilla void-avoidance AI keeps crossing rates near zero" — misattributed the cause. Void avoidance is a single torque blend in `BibitePropulsion.UpdateOrgan` gated on a global static that **ships off** (`ScenarioSettings.voidAvoidance`, `DefaultValue = false`; no shipped scenario enables it, and `Apocalypse` disables it explicitly). What actually keeps organisms on their islands is **food density**: pellets spawn only inside a `Zone`, and `3 Islands` separates its islands with a ~3000× fertility ratio, not with a steering rule. | Mod still owns the void-avoidance override (cheap, and worlds that enable *Void-No-Mo'* need it); the sidecar tells the mod which edges are open (`EDGE_STATUS`). **Consequence of the correction:** suppression alone cannot raise a rate that steering was never holding down, so M2 *measured* the natural crossing rate at an open edge first. **Measured 2026-08-02/03: the lure is unnecessary** — crossing is frequent once an edge is open (20.5 and 24.4 strip entries per sim-hour), so the corridor zone was canceled and the pheromone beacon stays parked. M2 also disabled `worldWrapping` while an edge was open, so a missed capture could not teleport an organism to the antipode — which in turn leaked organisms out of the three unguarded edges. **D10 reverses that trade for M3:** the vanilla wrap goes back on and becomes the containment mechanism. |
| **D4** | **The bb8 body is opaque to the mod.** The mod ships the game's own Newtonsoft output as a version-tagged blob; parsing, validation, and indexing live only in the sidecar's `bb8-schema`. | The authoritative serializer is the game itself. One schema implementation instead of two, no cross-language fidelity risk, and the mod survives game updates that add fields. | `bb8-schema` needs no C# implementation. All payload validation happens sidecar-side, before anything reaches a mod. |
| **D5** | **No global clock.** Every sector runs at its own sim speed; envelope timestamps are informational; a migrating organism may experience time discontinuities. | Peers' sim speeds differ by hardware and settings; synchronizing would couple every peer to the slowest. | Age/season continuity across sectors is explicitly not guaranteed. |
| **D6** | **Species catalog is a module inside the sidecar, post-MVP (M7).** | It shares the sidecar's storage and (later) DHT; a standalone service created a circular dependency in the earlier draft. | Off the MVP critical path. **D11 does not overturn this:** `multiverse-archive` is one central recorder on the relay's host, not a per-peer library, and it depends on no sidecar internals. Whether the M7 catalog seeds from the archive or supersedes it is open — see the research table. **The call moves into M5, ratified 2026-08-10** (`m5_considerations.md`, decision 3): a public archive fills on somebody else's timetable and nothing may evict, so M5 produces the retention rule — keep everything and size the disk, keep the ledger and prune genomes to a horizon, or graduate to the catalog now — and the graduation question is answered with it rather than waiting for M7. |
| **D7** | **Sidecar and relay are written in Go.** | go-libp2p is the reference libp2p implementation, which settles the M6 transport. A single static binary is also the easiest thing to ask a player to run alongside the mod. | `multiverse-sidecar`, `multiverse-relay`, `multiverse-archive` (D11) and `bb8-schema` are Go. `bb8-schema` needs no second implementation — D4 already removed the C# side. |
| **D8** | **Ring topology with one-way lanes.** Peers form one ring. Every instance has exactly two neighbours: it **exports through its east edge** and **receives through its west edge**. Out and in are different doors, so a returning organism must traverse the whole neighbouring world — no boomerang at the shared edge. A sector is a **ring slot**, and a slot belongs to a **peer identity, not a connection**: an offline peer keeps its slot (its edges close, the reservation does not expire), a new peer inserts into the ring, and the map never reshuffles. The `{x, y}` grid is retired — kept on record as a possible far-future extension, not deleted from history. | A 2-D grid needs per-edge pairing, up to four live neighbours per peer, and a map that re-flows whenever a peer joins or leaves. A ring needs one rule and one neighbour link. One-way lanes turn migration into a permanent eastward current: an organism that wants to come home circles the whole ring, so genes mix across every peer instead of oscillating between two. A stable slot-to-identity binding means a peer that goes away for a week comes back to the same neighbours. | Relay sector assignment becomes **ring insertion**. `EDGE_STATUS` governs the **export edge only**; the entry edge is passive and always accepts (Contract A). Contract C simplifies: `sourceSector`/`destSector` are ring slots and `exitEdge` is always `E`. Contract debt A5 (`MIGRATE_OUT_NACK` carries no `edge`) stays moot while each sim exports through exactly one edge. |
| **D9** | **M3 is a LAN milestone.** No VPS, no public relay, no strangers. The relay runs on the owner's main machine; the owner's second computer joins over the LAN as a real remote peer. | A LAN peer is a *real* remote peer — second machine, second clock, second install, real network hop — and it buys that proof without buying hosting, TLS for public exposure, abuse limits, or unknown game versions at the same time. Those belong to one risk set and deserve their own milestone. | The public items move intact to a new **M4 — Public release**; the old M4 (direct P2P) becomes **M5** and the old M5 (ecosystem completeness) becomes **M6**. **D16 renumbered again on 2026-08-05**: public release is now M5, direct P2P M6, ecosystem completeness M7, and M4 is operations. M3 acquires a new problem the single-machine rig never had: an install story for a Windows machine with no dev environment. |
| **D10** | **Containment via the vanilla world wrap**, replacing the "guard all four edges" item carried out of M2. M3 keeps `worldWrapping` **ON**. | The M2 leak was self-inflicted: M2 turned the wrap off while an edge was open, and nothing else in the game contains an organism at the square edge (`m2_findings.md` §3). The two radii do not compete — the migration strips capture at `±S`, the wrap fires at `1.5·S + 1000` (4000 with `S = 2000`), so an export always wins the race. The wrap then catches what the strips do not: the north and south edges, and anything that slips past the passive entry edge. | The four-edge guard is dropped. Two **in-game verifications**, not assumptions, gate the decision: **(a)** wrap-on coexists with the strips — no false migration, no interference; **(b)** export capture must reach organisms already **outside** the square past the strip line (the 2000–4000 band), not only those inside the strip. See `m3_considerations.md`, Risks 1 and 2. |
| **D11** | **Lineage annex on every envelope, recorded by a new `multiverse-archive` service.** The annex carries the parent entity IDs plus a content hash of each parent genome. The mod supplies the parents as opaque blobs and the **sidecar** hashes them, because D4 keeps the body opaque to the mod. The archive runs beside the relay, records every envelope and annex, and fetches an unknown genome by hash from the source sidecar. | Migration is already the moment a genome crosses a machine boundary, so it is the cheapest possible place to build the organism database — and that database is the seed of the M7 species catalog. Without the annex an archive holds a pile of unrelated arrivals; with it, the same stream is a lineage graph. The archive sits **outside** the relay because D1 makes the relay dumb, and a relay that indexes genomes is no longer a frame forwarder. | New `multiverse-archive` component. Contract C gains the annex; Contract B gains a genome fetch by hash. **Known limit:** a database built from migrations holds migrants and their ancestors only, never the resident population. Periodic census uploads are a later option, not M3. |
| **D12** | **Route around gaps; splice in anywhere.** When a slot stops being deliverable the ring **heals**: its west neighbour's lane re-pairs to the next deliverable slot east and the current keeps flowing. Insertion is no longer tail-only — a peer splices in **between two live slots**, and one mechanism serves both growth and return. | The overnight run measured the cost of the old rule (`e2e/baselines/20260805T230730Z/t1-report.md`): slot 2 went dark for two hours, slot 1's export edge closed, and its organisms then walked out of the playable square instead of migrating — square crossings spiked about 9× while slot 3's inbound stream, and with it most of its genetic mixing, collapsed. A ring that stops at its first gap couples every peer's liveness to every other peer's, and that is the one property a public ring cannot have. Healing also makes *return* free: a reclaimed slot keeps its number and its position, so coming back is a liveness event, not a topology change. | The relay computes an **effective** east neighbour and names it in `SECTOR_GRANT`; `PEER_STATUS` still publishes the structural ring order, so the map stays readable. `EDGE_STATUS` closes only when **no** slot in the ring is deliverable. The migration journal gains a handoff state, because re-routing a journaled hop is safe only when the relay can prove the frame was never forwarded — where custody may already have moved, D2 forbids every available action, and since 2026-08-17 that means the organism is recorded lost rather than held (`contract-b-m4.md` §25, B37). Insertion is safe for work in flight by construction: a released slot number is never reissued, so no journaled `destSlot` can ever name a newcomer. Releasing or handing over a slot never transfers a journal. |
| **D13** | **The grid.** The topology generalizes from a one-way ring to a **one-way 2-D structure**: a peer exports **east and north**, receives **west and south**, and has up to four neighbours. No-boomerang holds per axis, and the two currents compose into a diagonal drift. **M4 ships the full grid — plural exports, per-lane edge state, coordinate addressing, per-axis route-around, and the vertical lanes live — and proves it on a two-axis rig.** The ring is the one-row case of the grid. | One current mixes genes along one cycle; two currents mix them across a torus, and an organism comes home only after a whole number of both circuits. The abstractions have to land **before** M5 makes the wire public, because a breaking wire change after strangers run the build costs a migration story rather than a rebuild. **The owner overrode the deferral on 2026-08-05:** shipping an untested flag is shipping an unknown, and a lane nobody has run under real organisms is not a lane. The hardware objection did not survive its own numbers — the main machine ran three game instances comfortably at about 550 MB each on a 128 GiB host, so four or five locally plus one or two on the second computer puts an honest **3×2** map (six instances) inside the existing rig. | `exportEdge` becomes `exportEdges`; `EDGE_STATUS.edges` carries one entry per export edge; `borderEdges` becomes all four edges. A reservation gains coordinates while the **slot number stays the routing address** — positions move, addresses never do. The mod gains a second capture band, a second entry mapping, and a corner rule where the two bands overlap. Contract debt A5 stays moot: a `MIGRATE_OUT_NACK` is correlated on `migrationId`, not on an edge. The second capture band and the corner rule therefore run **in production**, not behind a flag, and the exit test attempts a live 3×2 map across the two machines. A degenerate proof — a 1×2 map on one machine — is the fallback **only** if the two machines genuinely cannot hold six instances. |
| **D14** | **Hard-stop recovery is a first-class feature, and a world save is part of it.** Any single instance may crash or hard-stop and must recover cleanly: the world reloads from its last periodic save, the sidecar replays its journal, and custody reassertion handles the resurrection. **No work goes into preventing sleep or crashes.** | T1 is the argument. The overnight run was *safe* and still lost the worlds: 22 595 hops held exactly-once, no organism was lost, and about **97% of the simulated world state was discarded**, because the rig turns the game's autosave off — a scene reload under a live rig is the worse failure — and then never sent a save of its own. Custody protects organisms in flight; nothing protected the organisms at home. Recovery is also the cheaper half: the rig runs on a desktop the owner uses, so hard stops are normal input, not incidents. | The mod owns a periodic save on a wall-clock timer, a save on quit, and a rotation so one bad save cannot destroy the last good one. Save cost is measured against the sim before the cadence is fixed. The journal replay and custody reassertion of `contract-a.md` §7.4–§7.5 become **tested** paths instead of believed ones. `e2e/data/slot-1/journal` still holds one real in-flight hop from 2026-08-04 — migration `9d6db335-b1ae-433e-a44a-bb2109912913`, entity `2004967003`, slot 1 → slot 2 — and delivering it is M4's resume test. Those journals must not be deleted — the rig was retired and compressed on 2026-08-08, so the directory now lives inside `e2e/data.tar.zst` and is extracted before the test runs, never thrown away. |
| **D15** | **Operations and observability are milestone work, not a side effect.** M4 buys the operator surface: population on the Contract B ring view, durable metrics outside the BepInEx logs, an archive-served live HTML status page and a `ringstat` terminal command, log preservation at shutdown, a slot handover command, and a join kit. It also **instruments the entry edge**, where the owner observed a mass of organisms on the west border after the overnight run. | T1 produced every one of its findings from 147 MB of BepInEx logs that BepInEx overwrites on the next game launch, and its best series — population and edge pressure per simulated minute — existed nowhere else. An operator who cannot see the ring cannot run it, and M5 puts strangers on it. **The entry-edge cause was named by the owner on 2026-08-05, and it retires the measure-first argument:** the sleeping machine **dammed the flow**. Queued custody deliveries plus the west neighbour's contained export pile released together the moment the slot woke, and the entry edge took the whole dam at once. The mechanism is a burst, not a steady rate, so the fix is a rate limit rather than a different arrival position. | `HEARTBEAT` already carries population on Contract A; Contract B carries it to the relay so the ring view has it without reading a log. The archive gains a read-only HTML surface beside its query surface, because it already subscribes to every envelope and already runs beside the relay. M4 ships a **delivery rate limit**: the sidecar paces `MIGRATE_IN` delivery out of its journal at a configurable maximum spawn rate per sim-time unit, so every burst — sleep recovery, an edge reopening, a future mass migration — trickles in instead of flooding. The crowding metric ships beside it, because it is cheap and because it is how the pacing is verified. **Arrival-position spreading stays parked**: it trades D3's one-continuous-world illusion for a flatter histogram, and the dam, not the position, was the problem. |
| **D16** | **Milestone renumbering.** M4 becomes **Operations: resilience, topology, observability**. Public release moves to **M5**, direct P2P to **M6**, and ecosystem completeness with the species catalog to **M7**. | D12 to D15 are one coherent risk set — a ring that survives its own operators — and they are prerequisites for a public ring rather than parts of it. Shipping a public relay on a topology that stops at the first dark peer, with a rig that loses a night of worlds and an operator surface made of perishable log files, exports all three defects to strangers. | Every "M4" that meant public release, every "M5" that meant libp2p and every "M6" that meant the catalog shifts by one. This document is corrected throughout. The contracts are **not**: `contracts/contract-b-m3.md` §13 and `contracts/contract-a.md` §12–§14 say "M4" where they now mean M5, and the implementation wave corrects them. |
| **D17** | **Two-way lanes.** Every lane becomes bidirectional. Each of a peer's four edges is **both an export edge and an entry edge**: a `W` export routes to the west neighbour, who receives it on its `E` edge; an `S` export routes south and is received on `N`; and so on per axis in both directions. **This supersedes D8's one-way rule** — "out and in are different doors" is retired, and with it the guarantee that an organism only comes home by traversing the whole cycle. **D8's stable-slot rules survive untouched:** a slot is still a routing address bound to a peer identity, still never reused, still never renumbered, and an offline peer still keeps its slot and its position. | One-way lanes bought no-boomerang with a permanent current. The price was that **half of every world's border is dead geometry**: an organism that walks west is not a migrant, and D10's wrap teleports it to the antipode — which a player reads as a wall inside a world that is supposed to be continuous. The three protections M4 already ships turn out to be sufficient on their own, and each is now load-bearing rather than belt-and-braces: an arrival is inset past **every** capture band on **both** axes (`contract-a.md` §4.3, §15 A28), the entry-immunity window keeps the spawn and the next export two separable events, and the outward-velocity test means an **ordinary arrival — which by construction arrives travelling inward — is never in the band of the edge it landed behind**. What one-way lanes prevented *instantaneously*, those three prevent instantaneously. What one-way lanes prevented *over a whole traverse* — a round trip — is exactly the property being given up on purpose: a world's neighbour is now a place an organism can visit and return from, which is what a map of worlds should mean. | `EDGE_STATUS` carries **four** entries, one per edge, each with its own neighbour-liveness state; the four open and close independently. Relay routing gains the reverse direction per axis, and route-around applies symmetrically — `effective(W)` and `effective(S)` are the same walk with the step negated. Capture bands run on all four edges, so the band overlap of the corner rule now exists at **all four corners** (never with two opposite edges: `E` and `W` are mutually exclusive while `S > W`). Bounce-back semantics are **unchanged** — an organism bounces home through the edge it left by, and that edge is now also a normal entry edge, which is fine: a bounce-back arrives moving *outward*, inset past the band, and the immunity window covers the ticks it takes to reach the band line, exactly as it already did for `E` and `N`. **D10's wrap loses most of its duty** and keeps the rest: it now contains only excluded species (D18), organisms whose relevant lane is closed, and organisms still inside the immunity window. **On an axis of length 2 the forward and reverse lanes name the same peer** (§2.1's degenerate case, now doubled), so the 3×2 rig's columns become two-lane pairs and hop rate on that axis rises disproportionately — which D20 has to absorb. Contract A takes **no version bump**; Contract B takes a **minor** to `contract-b/3.3`. |
| **D18** | **Migration exclusion policy.** The mod gains a **local** policy: species named on an exclusion list **never export**. The capture test skips them, and each excluded organism is logged **once per organism per session** so the line is evidence without being a cost. Config is `MULTIVERSE_MIGRATION_EXCLUDE`, comma-separated **full** species names matched on **A34-normalized** forms, defaulting to **`Basic bibite`** by the owner's call. | Two-way lanes make every border live, and the founder species is the one population that should not ride them. `Basic bibite` is the seed stock every world starts with: it is not an evolutionary result, so moving it between worlds mixes nothing — it spreads one starting genome across the map and competes for the arrival budget with the migrants that do carry information. Keeping it home also gives every world a stable resident baseline against which the immigrant species are legible on the census. The policy is **local and export-side only** because that is the only place it can be cheap: one set lookup inside a test that already runs every `FixedUpdate` on every organism. | **No wire change of any kind** — no field, no enum, no version, and no sidecar, relay or archive behaviour. The asymmetry it creates is a **documented feature, not a defect**: an excluded species accumulates where it is born and never appears in the migration ledger, while the census and the live page keep showing it **normally**, so "who lives here" and "who crossed" now legitimately disagree and a reader must not reconcile them. **D10's wrap becomes load-bearing again** — an excluded organism in an outward band is contained by the wrap and by nothing else, which is why the wrap stays on. The list is per-world, so a world that does not exclude a species may still **send** it to one that does; exclusion never refuses an arrival. Matching on the A34-normalized form is what makes the list work at all, because about 2% of the game's generated name halves carry an edge space. |
| **D19** | **Live hop animation.** When a migration lands, the live map animates the **species glyph** — the coloured bibite silhouette the census view already draws — travelling along that lane's arrow in near-real time. The archive gains a **recent-hops feed**: lane, species names and timestamp for roughly the last 60 seconds, bounded by both time and count, served beside `/api/status` and polled on the page's existing 2-second cycle. The client animates the glyph between polls, arrival-triggered and brighter than the ambient pulses. | The map already pulses per lane at a rate derived from `recentHops`, but **a pulse is anonymous** — it says traffic is flowing and nothing about what is flowing. The census gave every species a colour and a shape, and the ledger already carries a species block on every `MIGRATION_PAYLOAD`, so both halves of "which species is crossing which lane, right now" were already inside the archive and only the join was missing. It is also the cheapest possible verification of the two decisions above: two-way lanes show as glyphs travelling **both ways along one arrow**, and the exclusion shows as a species that is everywhere in the census and never on a lane. | Archive-internal and page-only. **No wire change**, and `contract-b-m4.md` §10.1's rules are the ones it already has to obey rather than new ones: the feed is built from envelope copies the archive already records, so "one source, no polling" is untouched; a hop whose envelope carried no species block animates the **neutral glyph**, never a guessed name; and the feed is **history — what crossed — never abundance** (D11, B12), so it may not be summed into a population. The bound is not optional: `Status` is serialized verbatim into `metrics.jsonl` once a minute, so an unbounded array on that struct would land in a durable file every minute forever. |
| **D20** | **The delivery rate limit rises**, from `inboundRatePerSimMinute` **2.0 to 12.0** and then, **the same day, to 100.0**, with `inboundRateBurst` **5 to 15** and then to **50**. It also **becomes a real knob** — a flag and an environment variable — which it has never been: it is a compiled Go constant today, reachable only by editing source. | **2.0 was reasoned, not guessed, and D13 spent the reasoning.** T1 measured 0.4 arrivals per simulated minute into slot 1, and *five times the measured natural rate* was the margin chosen so that ordinary traffic is never held back (`contract-a.md` §7.5). Then the grid gave every slot a **second** inbound lane and doubled the map. Thirty-five hours of the living deployment now measure a **median of 1.19 and a p95 of 2.21** arrivals per simulated minute per slot — **12% of all slot-samples above a limit whose entire purpose is to be above them**. Slots 3 and 4 sit above it 20% and 26% of the time, and slot 4's paced backlog has been pinned at `inboundQueueMax` for a quarter of the run. §7.5's own diagnostic names this exactly: *a depth that never falls names a limit set too low*. D17 then doubles the inbound surface again. | **12.0 is five times the projected median under two-way lanes** — the same rule that produced 2.0, applied to the topology that ships. It is also where the measured curve flattens: at 12.0 only 0.3% of projected samples are throttled, and that residue is genuine dam events (a far-end dropout at 18.5 arrivals per simulated minute), which is precisely what the limit exists for. The dam is still spread — a 900-organism backlog takes 75 simulated minutes to drain instead of arriving in one breath — and the A29 livelock defences are untouched, because the mod's ingest ceiling of 4 applications per `FixedUpdate` is three orders of magnitude above 12 per simulated minute. The knob is the other half: `holdTimeoutMs` set the precedent in M4 by gaining `--hold-timeout` for exactly this reason (that tunable is now `forwardTimeoutMs` and `--forward-timeout` — `contract-b-m4.md` §25, B37 — and the precedent it set is what the flag is cited for), and a tunable an operator cannot retune from the metric that measures it is not a tunable. **12.0 did not hold, and the second raise is recorded here rather than left to the contract.** It was a projection made before two-way lanes ran; once they ran, slot 3 sat at a `pacedDepth` of 16 under the 12.0 limit — §7.5's own verdict a second time — so the owner raised the default to **100.0/50** on 2026-08-07, which stops sizing the limit from projected offered load and sizes it from A29's ingest ceiling instead, two orders above it. The burst gained a knob of its own with it (`--inbound-burst`), and each sidecar now **publishes** the rate and burst it is running with on the peer stats block (`contract-b-m4.md` §18, B16), because a default that moves twice in a day is not what any particular slot is running. |
| **D21** | **Per-peer credentials take Contract B to a major.** Replacing §3.1's one shared token with a credential **bound to the `peerId`** is a changed rule with an installed base, not an added field, so the wire moves to **`contract-b/4`** and the path moves with it. It lands in M5's first work package, before any stranger runs the build. Contract A's bearer token stays a **minor** (`contract-a/2.4`) — an additive field on an existing handshake. | **D13's argument, one milestone later.** A breaking wire change after strangers run the build costs a migration story rather than a rebuild, and M5 is the last moment that sentence is true. Both half-measures fail on their own: TLS without credentials encrypts a wire on which any participant can still impersonate any other, and a presence-detected credential field carried on a minor leaves §3.2's `4006` eviction reachable by anyone who can read a `peerId` off the status page — a one-frame denial of service against any peer on the map. | `contract-b-m4.md` §3.1's token rule is **replaced**, not extended, and §3.2's eviction requires the credential; the acceptance test is adversarial — a peer presenting a valid credential with another peer's `peerId` is refused at the handshake and the legitimate peer observes nothing. The living deployment is the first fleet that has to cross the major: six worlds on two machines, upgraded in lockstep, which is the migration rehearsal M5 gets for free. Where credentials come from is M5 design work and the smallest form is the recommended one — the relay mints a peer secret at first claim and prints a join string; recovery is the slot handover §7.5 already has; a stranger who loses the join string loses that world's identity until an operator hands the slot over by name. |
| **D22** | **Version compatibility is layered, and the wire is the membership test.** The relay cares about one thing: that each sidecar speaks a compatible **contract** version. The **game** version is a per-machine concern — each operator needs a sidecar and mod build compatible with the game version *they* run — so what the project publishes is a **support matrix over game versions**, not one build the whole map pins to. **This supersedes the fleet-wide same-game-version rule.** | **The owner's own call**, and the rule it replaces was unenforceable and misplaced at the same time. Steam auto-updates stay on and land unevenly, so "every peer runs the same game build" stops being true within hours of an update and there is nobody to tell. What actually has to agree for two worlds to exchange organisms is the wire, and the relay is the only party that sees every peer's claimed contract version at once. `m5_considerations.md` DQ5's three alternatives — a tolerance window, a forced-update path, a stated refusal — all price the game version as a map-wide property, and it is not one. | The relay handshake gates on the **contract** version and never on the game version, and it is a **compatibility control, never a security control**: a claimed version is attacker-chosen text (`contract-b-m4.md` §13 item 7). `setup-farend.ps1`'s `$AssemblySha256` refusal and the far-end bundle's pin survive as **that machine's** entry in the matrix rather than as a rule about the map. **The wire and the code are both behind this row, and after 2026-08-11 they stay there on purpose:** `contract-b-m4.md` §6.1 still states normatively that the relay *MUST* refuse a peer whose `gameVersion` is incompatible with the map's, and the relay does exactly that (`relay.go:467` at the handshake, `:655` on a claim, `:771` in its own neighbour walk). WP1 **adds** the contract-version gate beside it rather than replacing it — see the closing paragraph of this cell. **The design's named remaining work is answered — by the owner, on 2026-08-11, and the answer is to assume rather than to research.** Organisms cross *between* games, so a bb8 payload serialized by game version X is loaded into game version Y by `SaveSystem.LoadBibiteOrEggFromData`. In his words: *"for now lets just keep doing the payload opaque and assume it will work, we will worry about doing the normalized own schema or the cheaper alternative in the future if we run into an issue, for now we can just assume this will work and carry the game version info for potential future debugging."* So: the bb8 body **stays opaque** (D4, unchanged), cross-version loading is **assumed to work** — no gate, no refusal path built for it, no stability research now — and the envelope carries the serializing game's version as **diagnostic metadata only**, under the prohibition `contract-b-m4.md` §13 item 7 already puts on `contractAVersion`: *a reader **MUST NOT** parse it into a capability decision*. Both fuller designs — a normalized canonical schema of this project's own, and the cheaper marker-plus-refusal — are **deferred until an incident**, and the incident that reopens them is a real cross-version load failure. **The `bb8-schema` cross-version row therefore leaves M5's critical path** and returns to ordinary research-table status, and Contract Change 10a is redefined from a compatibility mechanism to a small additive diagnostic field. One thing the owner's words do not settle, recorded rather than assumed away: the field is **already carried** end to end (`contract-a.md` §5.3 `MIGRATE_OUT.gameVersion` and `MIGRATE_IN.gameVersion`, `contract-b-m4.md`'s `body.version`, the sidecar journal, the archive ledger), and four shipped mechanisms still decide on it — §9.2's `VERSION_UNSUPPORTED`, `mapwalk`'s `peer_incompatible` skip, close `4002` and §6.1's relay refusal — so retiring them is behaviour rather than wording, and WP1 put it to the owner (`m5_considerations.md` DQ5). **Answered the same day, and this is the row's final state: the gates stay** — *"lets not change this then we will reconsider in the future if there's issues i think we can leave it like its working now."* Net effect of the two exchanges of 2026-08-11: cross-version loading is assumed to work and nothing is built to police it, **and nothing shipped is removed either**, so the assumption is never actually exercised — a game-version mismatch still refuses organisms and closes the edges between the mismatched pair, which is safe, because no cross-version payload reaches the loader at all. WP1 therefore retires no gate; it states the game version's diagnostic intent, binds **new** readers only, and names the four as kept exceptions. The whole cross-version design space — retiring the gates, the normalized schema, marker-plus-refusal — waits for version skew to cause a real operational problem, the everyday form of which is a map partitioned along a version boundary after a staggered game update (`dev_environment.md`, *The far end*). |
| **D23** | **The control surface is M6 work.** M5 is the first milestone in which it becomes *possible* — its three blockers are all M5 items — and it still does not land there. The operator surface stays read-only end to end through the public release. | A43's argument survives contact with the milestone: a new message type is additive and costs a minor, so deferring is cheap, while the surface itself needs ordering and idempotency for a write that races a world load, semantics for a disconnected target, authorization across machines D9 keeps undriven, and an audit trail — and *"reversing `CONFIG_UPDATE` or making a stats field writable is not the cheap version of that work; it is the same work with the questions skipped."* Adding all of that to the milestone that already replaces the authentication model, hosts the relay and meets strangers is how a public release slips. | **The risk is accepted, named and dated:** `m5_considerations.md` Risk 9 is the concrete case — an operator can watch a stranger's world run at seven times everyone else's speed, read `timeScale` per peer, and do nothing about it, because the field is a report and no message sets it (`contract-b-m4.md` §18, B16). The mitigation is arithmetic, not control: read rates per peer with the time scale beside them and never aggregate a per-simulated-minute series across peers running at different speeds. M6 gains the design alongside libp2p; `contract-a.md` §12 item 10 and §19 A43, and `contract-b-m4.md` §13 item 8, are the statements of what it must answer. |
| **D24** | **The public relay is a bounded, announced commitment.** The owner runs the hosted relay and archive for a **stated period** — the playtest — announced to participants before they join, rather than offering an open-ended service. | A playtest that succeeds creates a standing obligation on one person, with a support load attached, and every previous milestone ended while this one starts something that does not. The project's own record says what unattended means here: a collector no reboot restarts, a seven-step hand bring-up, a full-disk outage discovered by its symptoms. Deciding the commitment **after** the strangers arrive is worse than deciding it now, and an announced end is the difference between a service being wound down and a service being abandoned. | This is the answer the research table has carried as open since D9 answered it for the LAN only. The announcement is a **deliverable**, not a courtesy: participants are told the period, what a restart looks like, and what happens to the map and the archive at the end — which is also where D6's retention call (decision 3) lands, because a bounded run with an announced ending is the moment the archive's fate has to be stated. The publish-the-relay path stays prepared, so "what happens after" is answered rather than improvised in front of participants. |
| **D25** | **The package ships from GitHub Releases, and states the default it ships with.** Distribution is a GitHub release page carrying the mod, the sidecar and the installer, with **published checksums** and the `Unblock-File` story written on the page itself. A bare install keeps **D17's default — all four edges, client on** — stated explicitly in the packaging documentation and the installer's own output. | The channel is a support model, not a URL: it decides who updates a player's install, whether an update can be pushed, and where a confused person goes to complain. GitHub Releases pushes nothing, which is consistent with D22 — there is no fleet-wide forced update to build a policy on, so the matrix and the release page are the mechanism. On the default: DQ4's working-through holds, and the safety half dissolves under D21 — an unconfigured install has no relay URL and no credential, so it cannot connect whatever its edge set says. What is left is semantics, and the whole perimeter is the right answer for a player who configured a relay and not an edge set. | The release page owes what a signed installer would have removed: checksums a reader can verify and an honest account of the mark of the web, instead of a README that talks a stranger through `Set-ExecutionPolicy Bypass`. Anything stronger than checksums is WP6 design. The shipped default is stated **once**, in the package and the documentation, and the rig keeps its explicit-variable discipline (`e2e/run-m4.sh:59-62`) so a future change to the mod's default can move neither what the rig measures nor what a stranger's world does. |

The project owner ratified D1 (relay-first), D2 (at-most-once custody), and D3
(edges-over-towers) on 2026-08-02; they are settled, not provisional.

The owner ratified D8 (ring topology), D9 (LAN M3), D10 (wrap containment) and D11
(lineage annex + archive) on 2026-08-03. Together they redefine M3, retire the `{x, y}`
sector grid, supersede M2's "guard all four edges" carry item, and shift the public
milestone out of M3. Where an older passage in this document disagrees with them, D8–D11
win.

The owner ratified D12 (route around gaps, splice in anywhere), D13 (the grid), D14
(hard-stop recovery), D15 (operations and observability) and D16 (renumbering) on
**2026-08-05**, after the overnight run and its harvest (T1). Together they define a new
M4 and push every later milestone out by one. **D12 and D13 amend D8 rather than replace
it:** a slot still belongs to a peer identity, slot numbers are still never reused, and
out and in are still different doors. What changes is that a gap no longer stops the
current, a peer no longer has to join at the tail, and the ring is no longer the only
shape the map can take. The `{x, y}` grid D8 retired comes back in a different form —
one-way per axis, addressed by slot and positioned by coordinate — and D13 states why the
old objection (per-edge pairing, four live neighbours, a map that re-flows on every join)
no longer holds: route-around is what makes a map with holes viable. Where an older
passage disagrees with D12–D16, the new rows win.

The owner then signed off M4's design on **2026-08-05**, overriding three of its
recommendations. The four calls, recorded in the rows above and worked through in
`m4_considerations.md`:

1. **The full grid ships in M4** (D13), not the abstractions behind an off flag. The
   vertical lanes go live and the exit test attempts a 3×2 map across the two machines.
   *Overrides the recommendation.*
2. ~~**A hold is bounded** (D2, D12). An orphaned in-flight organism waits a configurable
   timeout — 24 hours by default — and then bounces home by itself. The owner accepts the
   residual duplication risk.~~ **Reversed 2026-08-17 (`contract-b-m4.md` §25, B37):
   there is no hold.** An organism forwarded into silence is recorded **lost** at
   `forwardTimeoutMs` and is neither re-sent nor brought home, so at-most-once carries no
   exception and the map has no duplication case at all. *Overrides the recommendation, and
   then overrides the override.*
3. **Arrival pacing ships now** (D15). The owner named the mechanism — a sleeping machine
   dams the flow and releases it all at once — so the sidecar rate-limits delivery instead
   of waiting for a measurement. The metric ships too, as the verification.
   *Replaces the measure-first-versus-mitigate question.*
4. **The save budget is 2 seconds** (D14). A periodic save stalls the simulation by no
   more than that, which makes the simple synchronous save the shipped design and gives
   the save-timer work item a bar to pass.

The owner ratified D17 (two-way lanes), D18 (migration exclusion), D19 (live hop animation)
and D20 (the pacing raise) on **2026-08-07**, against the living M4 deployment rather than
against a plan. They ship together in one implementation wave, and they are one idea seen
four ways: **make the whole map live, and then make it legible.** D17 opens the four edges,
D18 decides who is allowed through them, D20 sizes the pipe they feed, and D19 is how an
operator sees all three working. Where an older passage disagrees with D17–D20, the new rows
win.

**D17 supersedes D8's one-way rule explicitly, and only that rule.** D8 stated three things
and the new row keeps two of them:

| D8 said | Under D17 |
|---|---|
| **Every instance exports through its east edge and receives through its west edge.** Out and in are different doors, so a returning organism must traverse the whole neighbouring world — no boomerang at the shared edge. | **Superseded.** Every edge is both a door out and a door in. A round trip through one neighbour is now legal and intended. The *instantaneous* no-re-export guarantee survives, but it is now carried by the arrival inset, the entry-immunity window and the outward-velocity capture test rather than by the geometry. |
| **A sector is a ring slot, and a slot belongs to a peer identity, not a connection.** An offline peer keeps its slot, a new peer inserts, the map never reshuffles. | **Survives unchanged.** Addresses are still never reused and never renumbered; positions still move and addresses still do not (D12, D13). |
| **The `{x, y}` grid is retired.** | **Long since reversed by D13**, and D17 changes nothing about it: the map is a rectangle addressed by slot and positioned by coordinate. |

D13's own reasoning is where the supersession bites hardest, and it is worth stating rather
than quietly overwriting. D13 argued that one current mixes genes along one cycle and two
currents mix them across a torus, so an organism comes home only after a whole number of both
circuits. **That property is now gone on purpose.** The exchange is a border that is live on
all four sides instead of half of one, and — with D18 holding the founder stock in place — a
map where the thing that travels is the thing that evolved.

The owner ratified D21 (the Contract B major), D22 (layered version compatibility), D23 (the
control surface is M6 work), D24 (a bounded relay commitment) and D25 (GitHub Releases, with
the shipped default stated) on **2026-08-10**, closing the nine decisions
`m5_considerations.md` opened on 2026-08-09. Seven of the nine adopt that document's
recommendation. **Two are the owner's own:** D22, which supersedes a rule this document stated
rather than choosing between the options offered, and the channel inside D25, which answers a
question that document declined to answer for him. **D22 was then refined twice on
2026-08-11**, by the same hand: the payload question it named as its own remaining work is
settled by *assumption plus a diagnostic field* rather than by research, which takes the
`bb8-schema` cross-version row back off M5's critical path — and then the one thing that
refinement left open, whether the four shipped mechanisms that refuse on the game version
should be retired to match it, is settled the other way: **they stay, exactly as they run
today.** Nothing new is built on that axis and nothing shipped comes off it, so a version
mismatch keeps partitioning the map and the assumption keeps going untested. Its row carries
both calls. The remaining four
ratifications are calls inside the milestone rather than rows here, and
`m5_considerations.md`, *Decisions for the Owner*, carries each in full:

1. **The archive's retention rule is decided in M5** (decision 3) — an M5 work item that
   produces a rule even if the rule is *keep everything*, and ships the per-peer growth
   arithmetic to whoever hosts it either way. It forces D6's graduation question early; D6's
   row records that.
2. **A bare install keeps D17's four-edge default** (decision 7), stated in the packaging
   documentation and the installer's output rather than left to be discovered. D25 carries it.
3. **The exit-test bar is at least four non-owner peers, at least 72 continuous hours, and
   zero operator actions on any participant's machine** (decision 8), with relay-side actions
   allowed and counted. The M5 exit test below states it.
4. **The velocity magnitude floor is not built in M5** (decision 9). It is instrumented during
   the playtest and the measurement decides whether it is ever built, so `contract-a.md` §12
   item 7 stays open across a public release with its cost accepted (§4.3.1's `OPEN` note,
   reopened by §18 A38).

Where an older passage disagrees with D21–D25, the new rows win.

---

## Components

### 1. `bb8-schema` — The Data Language
**What it is:** A library that parses, validates, and serializes migration payloads.
**Language:** Go, sidecar-side only — the mod treats payloads as opaque blobs (D4, D7).
**Depends on:** Nothing. Pure data.
**Tested by:** Unit tests against real `.bb8` files from the community.

**Owns:**
- Gene array structure (flat floats)
- Node registry (48 standard: 33 input + 15 output, hidden nodes at index 48+)
- Synapse structure (`NodeIn`, `NodeOut`, `Weight` as float|string, `En` as bool)
- Validation rules: no input→input synapses, index bounds, weight type handling,
  gene bounds, NaN/Inf rejection, synapse-count and blob-size caps
- Live attribute schema (health, energy, stomach, maturity, size, position, velocity)
- Dialect detection: a `.bb8` written by the game's own `SaveSystem.SaveBibite` carries
  full live state under a `body` key; a `.bb8template` (and community template exports)
  carry genes + brain topology only
- Payload-kind registry: `bibite` now; `corpse`/`pellet`/`egg` shapes later (§6.4)
- Versioning — tagged with the game version it targets; cross-version conversion
  (prior art: EinsteinEditor's auto-conversion). Version ordering must match the game's
  `Utility.Version` semantics, alpha quirk included (see the research table below)

---

### 2. `bibites-mod` — The Game Hook
**What it is:** A BepInEx/HarmonyX plugin installed into The Bibites.
**Language:** C# (.NET Standard), referencing the game's Managed DLLs.
**Depends on:** Game's Managed DLLs and `Newtonsoft.Json.UnityConverters.dll` only —
no `bb8-schema` (D4).
**Tested by:** In-game manual testing; the M1 round-trip spike is its first exit test.

**Owns:**
- Harmony patches on `SimulationScripts.BibiteScripts.BibiteBody` (the live organism
  class; `BibiteBody.FixedUpdate()` is the per-organism tick)
- Border detection (position check each `FixedUpdate()`, hysteresis zone) — on the
  **export edge (east)** and the **passive entry edge (west)** from M3 (D8). From M4 the
  mod declares **`exportEdges`, plural**, and carries a second capture band and a second
  passive entry edge for the north/south axis (D13). The vertical pair is **live** in M4,
  with the corner rule deciding which edge claims an organism inside both bands.
  **From D17 the mod declares all four**: every edge runs a capture band and every edge
  also accepts arrivals, so there is no passive edge left and the corner rule arbitrates
  at all four corners
- **The migration exclusion list** (D18) — `MULTIVERSE_MIGRATION_EXCLUDE`, default
  `Basic bibite`. A species on the list is skipped by the capture test and logged once per
  organism per session. Local policy, matched on the A34-normalized name, never on the wire
- **Periodic world save and save-on-quit** (D14). A wall-clock timer drives the game's own
  `SaveSystem.CreateSave` by hand — the public `SaveGame` wrapper swallows exceptions
  inside a coroutine (`m1_findings.md`) — with a rotation of the last N saves, and one
  receipt line per save for the harvest tooling. The game's own autosave stays **off**: it
  reloads the scene under a live rig, which is the worse failure (D14). **Budget, signed
  off 2026-08-05: a save stalls the sim by at most 2 seconds**, which is what makes the
  simple synchronous save acceptable and gives the save timer a test bar
- **The species-history guard** (`[M5-HISTORY]`, mod 0.5.2) — a guard for a **game-side**
  defect, not a mod bug. `Utility.LogLikePresenceArray` describes a species' live history
  window both as apparition/disappearance indices and as a running `ushort` count, and
  `SaveStateBin` sizes its buffers from the count while filling them from the indices. The
  count underflows to 65535 when the disappearance front reaches a scale that never held a
  living point, then wraps back to 0 when the species is repopulated — and from then on
  **every world save throws** `IndexOutOfRangeException` in `SavePointToArrays`. Migration
  traffic makes that extinction-and-return cycle routine. A Harmony prefix on
  `GlobalLineageManager.BytesSpace` and `.SaveStateBin` recomputes the count to the window
  the save loop will actually walk, which is what the game's own
  `RefreshPresentsFromIndices` computes during a prune merge. No-op on a healthy array;
  `speciesaudit` reports it and `e2e/species-guard-check.sh` proves both directions
- **Edge instrumentation** (D15): the existing `[M2-CROSSING]` series gains entry-edge
  occupancy and an arrival histogram along the entry edge — now the verification that the
  sidecar's arrival pacing works, rather than a measurement taken before a decision
- **Void-avoidance suppression at open edges** (D3) — without it, migration ~never happens
- **World wrapping stays ON** (D10). The strips capture at `±S`; the vanilla wrap at
  `1.5·S + 1000` contains everything the strips do not. The mod no longer touches the
  setting — it only snapshots and reports it
- Export: game-native serialize (`SaveSystem.SerializeBibite`) → opaque blob + version
  tag + exit edge + border position + velocity + **the lineage inputs** (D11) →
  `MIGRATE_OUT` → destroy locally **only on `MIGRATE_OUT_ACK`** (custody transferred, D2).
  Local removal is a raw `Object.Destroy` after de-listing from `BibiteTracker` — no
  corpse, no meat, no eggs
- Lineage inputs (D11): the migrant's parent entity IDs, and one **opaque serialized blob**
  for each parent that is still alive locally. The mod never hashes a genome — D4 keeps the
  body opaque to it, so the sidecar hashes the blobs and assembles the annex.
  `BibiteGenes` drops the parentage once a parent GameObject is gone, so a missing parent
  is normal and is recorded as a gap
- Import: `MIGRATE_IN` → thread-safe queue → `FixedUpdate()` dequeue → native
  full-state restore (`SaveSystem.LoadBibiteOrEggFromData`, which preserves the entity
  ID and needs no state overwrite) → re-link parent/child references →
  **report `MIGRATE_IN_ACK/NACK`**
- All Unity main-thread constraints (instantiation, destroy, transform writes)
- Config: sector bounds, sidecar address/port, migration cooldown

The mod knows nothing about topology or destinations — not the ring, not its slot, not its
neighbours. It knows only whether its own export edge is open (`EDGE_STATUS`). Routing is
the sidecar's job.

---

### 3. `multiverse-sidecar` — The Network Brain
**What it is:** A standalone daemon that handles all networking and custody.
**Language:** Go — go-libp2p for M6, and one static binary for players to run (D7).
**Depends on:** `bb8-schema`. Contains the `species-catalog` module (D6).
**Tested by:** Integration tests with multiple sidecar instances, no game needed.

**Owns — in every phase:**
- Localhost API server (Contract A — serves the mod)
- Migration journal: durable custody of in-flight organisms, recovery on restart (D2).
  From M4 an entry also records its **handoff state** — never forwarded, forwarded, or
  acknowledged — because that is what decides whether an orphaned hop may be re-routed
  (D12), and **when it was forwarded**, because a forwarded-then-silent entry is recorded
  lost at `forwardTimeoutMs` (D2, as reversed 2026-08-17 — `contract-b-m4.md` §25, B37;
  it recorded a *hold deadline* and bounced home before that). **From 2026-08-08 the journal is also bounded
  in bytes**: it compacts on a timer rather than only at startup, and an append is
  all-or-nothing so a short write cannot leave a fragment that makes replay discard
  everything behind it (`contract-b-m4.md` §20, B20)
- Routing: the export edge → the ring's next slot east (D8). From M4 that target is the
  **effective** east neighbour: the next *deliverable* slot, with dark slots bypassed
  (D12), and one target per export edge, east and north, on the live grid (D13). From D17
  there are **four** targets, one per edge, and the west and south walks are the east and
  north walks with the step negated — route-around is symmetric. The mod
  never sees any of this — route-around and the grid are invisible on Contract A
- Payload validation via `bb8-schema` — nothing invalid ever reaches a mod, in
  either direction
- Admission control: inbound migration rate limits, population-aware via `HEARTBEAT`
- Bounce-back: re-inject an organism locally when remote delivery fails
- Compatibility handshake on peer connect. **From D22 the handshake's membership test is the
  contract version, not the game version**: the sidecar states which contract versions it
  speaks and which game version and mod build it is running, and the game version travels as
  a report. Whether a sidecar build exists for a given game version is the published support
  matrix's answer, settled on the operator's own machine before it ever dials the relay.
  **The game version is not *only* a report yet, and after 2026-08-11 that is deliberate:**
  the sidecar's own walk still skips a peer on a different game version
  (`mapwalk.Deliverable` → `peer_incompatible`), and the owner chose to leave that and the
  three other shipped version gates alone until skew causes an operational problem
- Lineage annex assembly (D11): hash the migrant and its parent blobs with `bb8-schema`,
  build the annex, cache the blobs by hash, and strip them from the wire envelope
- Serving a genome by content hash to `multiverse-archive` (D11) — from that cache, the
  migration journal and its tombstones

**Owns — from M4 (operations, D14–D15):**
- Durable metrics outside the BepInEx logs: per-slot population, crossings, edge state,
  custody depth and save receipts, written at a fixed cadence to a file the harvest tools
  and `ringstat` read
- Journal replay and custody reassertion as a **scripted, tested** recovery path, not an
  assumed one
- **Arrival pacing** (D15): `MIGRATE_IN` leaves the journal at a configurable maximum
  spawn rate per sim-time unit. The sidecar is the right place for it because the journal
  is where a burst accumulates — a woken slot drains its own backlog and its neighbour's
  released export pile through the same gate, and the mod sees an ordinary arrival stream.
  **D20 raises the rate to 100.0 per simulated minute and makes it a real knob**: the grid
  and then two-way lanes overran a limit that was sized from a one-lane ring, so the
  limit had begun throttling ordinary traffic instead of only spreading dams. The raise
  landed in two steps on the same day — 12.0 first, then 100.0 when two-way lanes ran and
  the residual backlog did not clear — and each sidecar now publishes the rate it is
  actually running with (`contract-b-m4.md` §18, B16), because two defaults in one day is
  what a queue depth read against the wrong cap looks like

**Owns — M2–M5 (relay transport):**
- Relay client connection (TLS from M5; a shared token over the LAN from M3, replaced in M5
  by a per-peer credential bound to the `peerId` at `contract-b/4` — D21)

**Owns — M6 (direct P2P):**
- libp2p transport, NAT traversal (hole punching, STUN, relay fallback)
- Peer discovery (bootstrap nodes, mDNS for LAN, peer exchange)
- Gossip-based topology dissemination; lease-based sector claims (design open)

---

### 4. `multiverse-relay` — The Spatial Router (MVP transport)
**What it is:** A small, deliberately dumb server that forwards Contract B frames
between sidecars and arbitrates the ring. It never parses bb8 bodies.
**Language:** Go, same as the sidecar (shared framing code) (D7).
**Depends on:** Contract B envelope framing only.
**Tested by:** Integration tests with N sidecar instances.

**Owns:**
- Peer registry and connections
- **Ring insertion** (D8): the single arbiter of the map — which peer identity holds
  which slot, and therefore who each peer's neighbours are (until M6's leases).
  A slot is bound to a `peerId`, so an offline peer keeps it and the reservation does not
  expire. Slot numbers are never reused, and no surviving slot is ever renumbered
- **Insertion anywhere** (D12): a peer joins between two live slots, not only at the tail.
  Insertion changes two structural lanes and no slot numbers, so it can never redirect an
  organism already in flight
- **The effective-neighbour map** (D12): the first *deliverable* slot along each axis,
  recomputed on every liveness change and published in `SECTOR_GRANT`, while
  `PEER_STATUS` keeps publishing the structural order. Computing it uses only state the
  relay already holds; it still parses no body and indexes nothing (D1)
- **A proof of non-delivery** (D12): a frame the relay could not forward is answered so,
  explicitly. That answer is what lets a sender re-route a journaled hop without risking
  the duplication D2 forbids
- Frame forwarding (`MIGRATION_PAYLOAD`, ACKs, etc.)
- Compatibility enforcement at connect. **D22 adds a contract-version gate**: the relay
  refuses a sidecar whose contract version it cannot speak and publishes the minimum it
  accepts. The gate is a compatibility control, never a security one, because a claimed version
  is attacker-chosen text (`contract-b-m4.md` §13 item 7). D22's design says the relay should
  not police **game** versions — those are per-machine, the support matrix's business — but the
  shipped relay does, refusing a mismatched handshake and a mismatched `SECTOR_CLAIM` (§6.1,
  §7.2 rule 8), and **on 2026-08-11 the owner chose to leave it that way**. So the two gates
  run side by side until version skew causes an operational problem, and the map still
  partitions along a game-version boundary
- Liveness: a dark peer keeps its slot, and the ring **heals around it** (D12). The west
  neighbour re-pairs to the next deliverable slot east and keeps exporting; the dark
  peer's own east neighbour is unaffected and simply receives nothing. An export edge
  closes only when the ring holds no deliverable slot at all
- **Slot handover** (D15): an operator command that rebinds a reservation to a new
  `peerId` in place, keeping the slot number and the position. It never moves a journal —
  custody stays on the machine that holds it
- **M6 role:** degrades to bootstrap/rendezvous node + relay-of-last-resort for
  unpunchable NATs

Who operates it: the owner's main machine from M3 (D9). **A public one is the owner's, for a
bounded and announced period — D24, ratified 2026-08-10** — with the publish-the-relay path
prepared so other people can run their own maps afterwards.

---

### 5. `multiverse-archive` — The Organism Database (M3)
**What it is:** A small service that runs beside the relay and records every migration
that crosses it: the envelope, its lineage annex, and the genome itself.
**Language:** Go (D7). Deliberately **outside** the relay — D1 makes the relay a frame
forwarder, and a forwarder that indexes genomes is not dumb any more.
**Depends on:** Contract B framing; `bb8-schema` for the genome projection and its hash.
**Tested by:** Integration tests over recorded frames. No game needed.

**Owns:**
- The migration record: one row per envelope — `migrationId`, both ring slots, both peer
  ids, timestamp, and the lineage annex (D11). **From 2026-08-09 the ledger carries the
  journal's durability rule, in the form an unrewritable file needs**: an append is
  all-or-nothing, and replay **skips** a line it cannot parse instead of stopping at it,
  because nothing ever compacts this file and a stop would discard more of it every hour
  (`contract-b-m4.md` §20, B20)
- Content-addressed genome storage (hash → genome), sharing `bb8-schema`'s canonical
  genome projection so the same genome hashes identically everywhere
- **Fetch by hash:** when an annex names a genome the archive has never seen, it asks the
  source sidecar for it over Contract B
- The lineage graph: child genome hash → parent genome hashes, assembled from annexes
- A query surface over the two (read-only in M3)
- **From M4 (D15): the live status page.** The archive already subscribes to every
  envelope and already runs beside the relay, so it is the one process that can serve a
  self-contained HTML view of the whole ring — slots, liveness, effective lanes,
  populations, per-lane flow and custody depth — with no new component and no rule added
  to the relay. `ringstat` reads the same data from the terminal
- **From D19: the recent-hops feed and the animated species glyph.** The archive already
  sees every `MIGRATION_PAYLOAD` with its species block, so it keeps a bounded, in-memory
  ring of the last ~60 seconds of hops — lane, species names, timestamp — and serves it
  beside `/api/status`. The page polls it on the 2-second cycle it already runs and
  animates the census glyph along the lane arrow between polls. Bounded by time **and**
  count, because the status view is written verbatim into the durable metrics file once a
  minute. A hop with no species block animates the neutral glyph, never a guessed one

**Known limit, by construction:** a database built from migrations holds migrants and
their ancestors — never the resident population of a peer. Periodic census uploads would
close the gap and are a later option (D11).

**Second known limit, and it is an operator's problem rather than a design one:** the
archive is the one component here that **grows without bound on purpose**. Its ledger and
its genome store are the record of what happened, so nothing may evict from them
(`contract-b-m4.md` §10, §20), and unlike a sidecar's cache neither is capped by a
tunable. Measured on the six-slot deployment on 2026-08-08: **58 MB/h** of genomes,
**19 MB/h** of ledger and 0.7 MB/h of metrics. Those are the archive's own three files;
against everything the rig writes the figure is **97 MB/h ≈ 70 GB/month**, which is about
**three months** on the 251 GB volume. Somebody has to decide what the archive is for before
that runs out; every other file in this system is now bounded (see `dev_environment.md`,
*The disk budget*, which holds the full table and is the number of record).

Open: how it sees every envelope (a relay copy, or the archive joining as a slot-less
peer); the fetch protocol; and whether M7's `species-catalog` seeds from it or supersedes
it (D6). See the research table.

---

### 6. `species-catalog` — The Distributed Library (module, M7)
**What it is:** Content-addressed storage and replication of `.bb8` genomes, as a
module inside the sidecar (D6).
**Depends on:** `bb8-schema` (indexing/metadata), sidecar storage and (M6) DHT.
**Tested by:** Unit tests for storage, integration tests for replication.

**Owns:**
- Content-addressed storage (hash → `.bb8` file)
- DHT-based lookup (find which peers have a given genome)
- Selective replication (peer chooses what to cache based on interest/capacity)
- Browse/search API (by species traits, lineage, neural complexity)
- Background lazy replication (not latency-sensitive)

M3's `multiverse-archive` is the centralized ancestor of this module, not a competitor:
one recorder on one host, no DHT, no per-peer choice. It hashes genomes with the same
`bb8-schema` projection, so its records stay meaningful to whatever M7 builds.

---

## Contracts

### Contract A: Mod ↔ Sidecar (localhost)

Transport: WebSocket on `localhost:{configured_port}`
Format: JSON messages with a `type` discriminator

```
Direction: Mod → Sidecar
─────────────────────────
MIGRATE_OUT        Organism hit an open edge: opaque bb8 blob + game version +
                   exit edge + border position + velocity + parent entity IDs +
                   one opaque blob per living parent (D11 — the sidecar hashes
                   those blobs into the lineage annex, because D4 keeps the body
                   opaque to the mod)
MIGRATE_IN_ACK     Incoming organism spawned and overwritten successfully (migrationId)
MIGRATE_IN_NACK    Spawn failed (deserialization error, sim overloaded) — sidecar
                   keeps custody
HEARTBEAT          Mod is alive: current sim tick, population count (feeds admission
                   control)
CONFIG_UPDATE      Sector bounds changed, mod settings changed

Direction: Sidecar → Mod
─────────────────────────
MIGRATE_IN         Incoming organism: bb8 blob + entry coords + velocity + migrationId
MIGRATE_OUT_ACK    Organism durably journaled, custody transferred — mod destroys it now
MIGRATE_OUT_NACK   Sidecar can't take custody (edge has no live neighbor, journal
                   full, incompatible) — mod keeps the organism
EDGE_STATUS        Whether the export edge is open for migration — drives the mod's
                   void-avoidance suppression. From M3 the ring gives each sim one
                   export edge (east) and one passive entry edge (west, always
                   accepts), so this message narrows to the export edge (D8). From M4
                   it carries one entry per export edge — one on a one-row map, two on
                   the live grid (D13) — and a lane that re-pairs around a dark slot
                   never closes it at all (D12). From D17 it carries all four, one
                   per edge, each with its own neighbour state. A closed edge stops
                   EXPORTS through it and never stops ARRIVALS on it
```

> [!IMPORTANT]
> This is the **only** interface the mod needs to know about. It never touches the
> network and never sees topology — only its own export edge's open/closed status. The
> sidecar is a black box to the mod.

**The custody chain (D2):**

1. Mod sends `MIGRATE_OUT`.
2. Sidecar validates, journals to disk, replies `MIGRATE_OUT_ACK`. Custody: sidecar.
   Mod destroys the organism.
3. Sidecar sends `MIGRATION_PAYLOAD` (via relay in M2–M5, direct in M6).
4. Receiving sidecar validates, admission-checks, journals, sends `MIGRATE_IN` to its
   mod. Mod restores the organism from the blob and re-links its parent/child
   references, then replies `MIGRATE_IN_ACK`.
5. Receiving sidecar sends `MIGRATION_ACK`; the sender deletes its journal entry.
6. Any remote NACK or timeout → the sender re-injects the organism into its own sim
   via `MIGRATE_IN` at the same edge (bounce-back).
7. Receivers dedup `MIGRATION_PAYLOAD` on `migrationId`, so a resend after a lost ACK
   is idempotent. Loss is possible only if a journal is destroyed mid-flight — and
   reads as death (D2).
8. **From M4 (D12), a hop whose destination went dark is re-routed, bounced or lost —
   and which one it is depends on custody, never on convenience.** A frame the relay
   proves it never forwarded may be re-routed east to the effective neighbour under its
   original `migrationId`; a peer-specific refusal (overloaded, size mismatch) may be
   re-routed the same way; a payload-fatal refusal bounces home as in step 6. A frame
   that was forwarded and never answered gets **nothing done to it at all**, because the
   far sidecar may already have taken custody and every available action would risk
   duplication: it is not re-sent, not re-routed and not brought home. At
   `forwardTimeoutMs` — 24 hours by default — the entry is recorded **lost** and the
   organism is gone (reversed 2026-08-17, `contract-b-m4.md` §25 B37; before that it was
   *held* and bounced home on its own, which was the one exception at-most-once carried).
   `--release-inflight` still resolves one by hand, and it is now the only thing on this
   map that can duplicate an organism.
9. **From M4 (D15), delivery out of the journal is paced.** The receiving sidecar
   releases `MIGRATE_IN` at a configurable maximum rate per sim-time unit, so a backlog
   drains as a stream instead of a flood. Pacing changes when an organism arrives; it
   never changes whether it arrives, and custody is unaffected.

---

### Contract B: Sidecar ↔ Sidecar (wire protocol)

Transport: relay-forwarded frames, plain over the LAN with a shared token (M3) and over
TLS once the relay is public (M5), where the shared token becomes a per-peer credential bound
to the `peerId` and the wire moves to `contract-b/4` (D21); libp2p streams (M6). Same shapes
in every phase (D1).
Format: TBD (Protobuf likely for the envelope — the bb8 body stays an opaque bytes
field per D4)

```
HANDSHAKE          Game version, mod list, node count, protocol version
SECTOR_CLAIM       Request/renew a ring slot — relay-arbitrated in M2–M5; lease-based
                   design open for M6. From M3 a grant also names the peer's one east
                   neighbour, which is the whole topology a sidecar needs (D8). From M4
                   the claim carries live population for the ring view (D15) and an
                   advisory insertion position (D12); the grant names the *effective*
                   neighbour per export edge (D12, D13)
TOPOLOGY_GOSSIP    Ring-map dissemination (M6 only — before that the relay is the
                   single source of truth)
MIGRATION_PAYLOAD  MigrationEnvelope (Contract C); receivers dedup on migrationId
MIGRATION_ACK      Receiving mod confirmed the spawn (sent only after MIGRATE_IN_ACK)
                   — sender clears its journal entry
MIGRATION_NACK     Rejected (incompatible, overloaded, invalid payload) — sender
                   bounces the organism back locally
GENOME_REQUEST     multiverse-archive asks a sidecar for a genome by content hash,
                   for an annex hash it has never seen (M3, D11)
GENOME_RESPONSE    The genome, or "unknown hash" (M3, D11)
PEER_EXCHANGE      Share known peer addresses (M6)
CATALOG_QUERY      Request a bb8 by content hash (M7)
CATALOG_RESPONSE   Return a bb8 file, or "not found, try these peers" (M7)
PING / PONG        Liveness
```

---

### Contract C: MigrationEnvelope + bb8-schema (shared data model)

Not a network contract — a **code contract** over what a migration carries:

```
MigrationEnvelope
├── migrationId: uuid                  # idempotency key — receivers dedup on it
├── kind: enum(bibite, corpse, pellet, egg)  # MVP ships bibite; others are
│                                      #   protocol-ready for M7 (§6.4)
├── body: (by kind, below)
├── lineage: LineageAnnex              # D11 — below
├── species: SpeciesTag?               # OPTIONAL — the migrant's species identity, by
│                                      #   name. Ratified 2026-08-07 ("species identity
│                                      #   travels in the envelope"). Carried opaquely:
│                                      #   sidecars and the relay schema-validate it and
│                                      #   never interpret it, so it is envelope metadata
│                                      #   beside the blob exactly like exitEdge, and D4
│                                      #   is unchanged. The importing mod resolves the
│                                      #   name against its own registry and rewrites
│                                      #   genes.speciesID inside the blob before the
│                                      #   restore; an envelope without the block makes
│                                      #   the importer *remove* that key, so the game
│                                      #   classifies the arrival by its own genetic
│                                      #   distance instead of by a foreign world's
│                                      #   counter (contract-a.md §16 A30–A34,
│                                      #   contract-b-m4.md §15 B9–B10). A34: the
│                                      #   EXPORTING mod normalizes the halves'
│                                      #   whitespace and the importer matches on
│                                      #   the normalized form; the wire rule and
│                                      #   sidecar opacity are unchanged.
├── sourcePeer: string                 # peer ID of sender
├── sourceSector: ringSlot             # the sender's slot in the ring (D8)
├── destSector: ringSlot               # the sender's east neighbour (D8). From M4 the
│                                      #   *effective* one — dark slots are bypassed
│                                      #   (D12) — and a journaled value is rewritten
│                                      #   only under a relay proof of non-delivery
├── exitEdge: enum(N, S, E, W)         # which border it crossed. Always E on a
│                                      #   one-row map; E or N on the live grid
│                                      #   (D13), with the entry edge the opposite
│                                      #   one and always passive. From D17 any of
│                                      #   the four, and the opposite edge is an
│                                      #   ordinary export edge of the receiver
├── exitPosition: float                # 0..1 along that edge — receiver mirrors it
│                                      #   into entry coordinates. Mirroring stays:
│                                      #   crowding is answered by pacing delivery,
│                                      #   and spreading arrivals is parked (D15)
├── velocity: {x, y}
└── timestamp: int64                   # unix millis, informational only (D5)

lineage: LineageAnnex (D11)
├── genomeHash: string                 # content hash of this organism's own genome —
│                                      #   the join key in multiverse-archive
└── parents: [{ entityId: int32,       # 0..2 entries, one per known parent
              genomeHash: string }]    #   an absent parent is a recorded gap, not an
                                       #   error: BibiteGenes drops the parentage once
                                       #   the parent GameObject is gone

species: SpeciesTag (OPTIONAL, 2026-08-07)
├── genericName: string                # the genus half of the game's Species.name
├── specificName: string               # the specific half; the importer matches on
│                                      #   genericName + " " + specificName, exactly
├── parentGenericName: string?         # the *immediate* parent species' name, present
└── parentSpecificName: string?        #   only when the source species had a parent —
                                       #   both fields or neither. One generation only:
                                       #   deeper chains do not travel, because the
                                       #   importer can link only under a parent that
                                       #   already exists locally

body when kind = bibite:
├── version: string                    # game version that serialized it
└── bb8: string                        # the game's own Newtonsoft output — opaque to
                                       #   the mod (D4), parsed sidecar-side

body when kind = corpse | pellet (M7):
├── mass, material, decayState

body when kind = egg (M7): shape TBD — open research
```

A `genomeHash` is a hash of a **canonical genome projection** — genes, nodes and synapses
only. It is never a hash of the whole `.bb8`, because live state changes every tick and
would make every hash unique. `bb8-schema` owns the projection and the hash, so the mod,
the sidecar and the archive all produce the same value (D4).

Delivery is single-hop (sender → receiver, the relay merely forwards), so there is no
TTL/hop counter and no store-and-forward routing.

What `bb8-schema` validates inside the `bibite` blob:

```
BibitePayload
├── genes: float[]                     # physical stats
├── nodes: Node[]                      # brain neurons
│   ├── typeName: string               #   activation function (ReLu, TanH, Linear...)
│   ├── index: int                     #   0-47 = standard, 48+ = hidden
│   └── desc: string                   #   human-readable name
├── synapses: Synapse[]                # brain connections
│   ├── nodeIn: int                    #   source node index
│   ├── nodeOut: int                   #   target node index
│   ├── weight: float | string         #   signal multiplier OR gene reference
│   └── en: bool                       #   active/dormant
└── liveState: LiveState               # the game's own .bb8 already persists all of
                                       #   this; only .bb8template strips it
    ├── health, energy, stomach, maturity, size, age: float
    ├── position: {x, y}
    ├── velocity: {x, y}
    └── dying: bool
```

---

## Dependency Graph

```mermaid
graph LR
    S["bb8-schema"] --> D["sidecar core"]
    subgraph SP["multiverse-sidecar process"]
        D --> C["species-catalog<br/>module (M7)"]
    end
    S --> C
    M["bibites-mod"] ---|"Contract A<br/>(localhost WS)"| D
    D ---|"Contract B via relay<br/>(M2–M5)"| R["multiverse-relay"]
    R --- D2["other sidecars"]
    D -.-|"Contract B direct<br/>(M6)"| D2
    R -->|"envelope + annex<br/>(M3)"| A["multiverse-archive"]
    A -.->|"genome fetch<br/>by hash"| D
    S --> A

    style S fill:#4a9eff,color:#fff
    style M fill:#ff6b6b,color:#fff
    style D fill:#51cf66,color:#fff
    style R fill:#b197fc,color:#fff
    style C fill:#ffd43b,color:#333
    style A fill:#ffa94d,color:#333
    style D2 fill:#51cf66,color:#fff,stroke-dasharray: 5 5
```

Note the mod no longer depends on `bb8-schema` (D4). `multiverse-archive` hangs off the
relay's host, not off any sidecar's process — it reads envelopes and pulls genomes, and
nothing in the migration path waits for it.

---

## Milestones

Vertical slices, riskiest first — not component-by-component. `bb8-schema` is not a
standalone first step: its real shape is discovered in M1 and hardens through M2–M3.

**Renumbered 2026-08-03 (D9).** M3 lost its public half to a new M4 — public release —
and everything after it shifted by one: direct P2P became M5, ecosystem completeness M6.

**Renumbered again 2026-08-05 (D16).** The overnight run and its harvest bought a second
shift: M4 is now **Operations**, public release is **M5**, direct P2P is **M6**, and
ecosystem completeness with the catalog is **M7**. Older documents that say "M4" for the
public release, "M5" for libp2p or "M6" for the catalog predate this change — including
`contracts/contract-a.md` §12–§14 and `contracts/contract-b-m3.md` §13, which the
implementation wave corrects.

### M1 — In-game round trip (the de-risking spike) — **COMPLETE**
One game instance, no sidecar, no network. A dev command in the mod: serialize a live
organism (genes + brain + live state) → destroy it → respawn via the game's own
full-state restore (`SaveSystem.LoadBibiteOrEggFromData`, ID preserved) → re-link its
parent/child references.
**Exit test:** the organism resumes life with no observable change — same brain
topology, stomach contents, maturity — and a subsequent world save still succeeds.
**Passed 2026-08-02**, on two unattended runs against game `0.6.3.1`: run 2 compared the
before/after payloads as **EQUAL token for token**, the entity ID survived, and the game's
own world save completed with no exception. The whole exit test now runs headless from
`bibites-mod/src/AutoTest.cs` (`MULTIVERSE_AUTOTEST=1`), including seeding and loading its
own world. Carried to M2: the re-link code ran against an organism that had no parents and
no children, so the stale-reference trap is still unexercised. Full results in
`m1_findings.md`, *Runtime results*.
**Why first:** every high-risk unknown in the table below lived here (spawn API,
state access, destroy safety), and its findings define the true payload shape.
The decompiled-source pass (`m1_findings.md`) closed the spawn-API, state-access and
payload-fidelity unknowns; the in-game run has now confirmed them.

### M2 — Two sims, one machine — **COMPLETE**
Trivial relay (frame forwarding + a two-sector registry), sidecar skeleton, Contract A,
and the full custody chain — all on localhost. One designated open edge per sim.
**Full plan: `m2_considerations.md`. Wire spec: `contracts/contract-a.md` (complete).
Research: `m2_findings.md`.**
**Migration rate — measured first, and the lure turned out to be unnecessary.** D3's
original premise was wrong: void avoidance ships off, and food density is what keeps
organisms off the edges (`m2_findings.md` §1.4). M2 therefore *measured* the natural
crossing rate at an open edge before building any lure, kept the (cheap) suppression for
worlds that enable *Void-No-Mo'*, and disabled `worldWrapping` while an edge was open.
That last choice leaked population through the three unguarded edges; **D10 reverses it in
M3**, and the M2 text below is the historical record, not current guidance.
**Exit test:** an organism crosses from sim A to sim B and back, payloads equal at each
hop and both worlds still save; then `kill -9` the relay, a sidecar and a game
mid-migration (one at a time) and the organism exists **exactly once** after each
recovery (rare loss acceptable, duplication is failure).
**Passed 2026-08-02/03** on the unattended two-instance rig (`e2e/run-m2.sh`): the A→B→A
round trip came back sha256-identical in both directions with the entity ID intact, the
family re-link fired the stale-`BibiteID` trap and cleared it, and the organism was counted
**exactly once** after every recovery — four for four across the round trip and the three
kills, 512 journalled migrations per side, nothing stuck. **The measured crossing rate
answered the lure question:** 20.5 (sector A) and 24.4 (sector B) strip entries per
simulated hour at a population of ~26, over 30 real minutes at 20× with no forcing —
frequent enough that the corridor lure was **canceled**, not built. Full results in
`m2_considerations.md`, *Exit Test → Result*.
**Prerequisite — confirmed 2026-08-02:** two game instances do run side by side on one
machine (see the research table), so M2 needed no second machine. M3 takes a second
machine deliberately (D9), and keeps the two-instance trick to fill its third ring slot.
**Carried in from M1 and now closed:** the round trip ran on an organism with living
parents and living children, so the stale-reference trap is proven, not just implemented.

### M3 — The LAN ring: two machines, one current — **COMPLETE**
**Redefined 2026-08-03 by D8–D11. Full plan: `m3_considerations.md`.**

The first milestone with a genuinely remote peer, and the first with a topology. The relay
runs on the owner's main machine; the owner's second computer joins over the LAN (D9). No
VPS, no strangers, no public exposure — those are M5.

Three things arrive together:

- **The ring** (D8). Ring insertion replaces sector assignment at the relay. Each sim
  exports through its east edge into its one east neighbour and receives through its west
  edge. Migration becomes a permanent eastward current, so an organism comes home only by
  going all the way round. A slot belongs to a peer identity, so an offline peer keeps its
  place.
- **Containment by the vanilla wrap** (D10), replacing M2's "guard all four edges" item.
  `worldWrapping` goes back on. Two in-game verifications gate it: wrap/strip coexistence,
  and export capture in the band *outside* the square (see D10).
- **The lineage annex and `multiverse-archive`** (D11). Every envelope carries parent
  entity IDs and parent genome hashes; a new service beside the relay records every
  envelope and fetches genomes it has not seen. This is the seed of M7's catalog.

Also in M3: handshake and compatibility enforcement across two real installs; a shared
token on the Contract B upgrade now that the wire leaves the loopback; admission control;
journal-recovery hardening; the `EDGE_STATUS` ripple when a peer dies (under the ring it
travels one way — the dead peer's *west* neighbour closes its export edge).

Migration-rate tuning is largely settled by M2's measurement — the pheromone beacon
(`m2_findings.md` §1.5, option 5) and gateway towers stay parked unless a real multi-peer
world crosses far below M2's ~20–25 strip entries per sim-hour.

**Carried in from M2** (`m2_considerations.md`, *Carried to M3*):
- ~~**Guard all four edges.**~~ **Superseded by D10.** M2 disabled `worldWrapping` while an
  edge was open, and organisms then escaped through the three unguarded edges (seen at
  `y=7186` with `S=2000`). M3 does not guard four edges; it stops disabling the wrap. The
  same passage in `m2_findings.md` §3 and `m2_considerations.md` predates the decision.
- **Migration order is child-then-parent.** `BibiteGenes.SaveState` drops a child's
  parentage once the parent GameObject is gone, so a child that migrates *after* its
  parent arrives unlinked. Vanilla-consistent, so it is a fidelity limit rather than a bug.
  The lineage annex (D11) softens it: the archive keeps the parent hashes even when the
  in-game link is lost.
- **Contract debt A5** — `MIGRATE_OUT_NACK` carries no `edge` field. **The ring makes this
  moot** rather than urgent: a sim exports through exactly one edge, so a NACK can only
  ever refer to that edge (`contracts/contract-a.md` §13 A5, §12 item 6). The debt returns
  only if the retired `{x, y}` grid ever comes back.
- **Deep bb8 validation** — genes, nodes, synapses, dialect detection, the
  `Utility.Version` alpha quirk (deferred from M2's WP1 skeleton). The annex raises the
  stakes: the genome projection that `genomeHash` covers is the same data.

**Exit test:** an organism circumnavigates the ring across two physical machines and
arrives home; the archive shows its lineage annex with the correct parent hashes; a subset
of the kill gauntlet repeats cross-machine; every world and every journal stays clean. The
ring needs **three** slots for that — two slots make a degenerate ring where one hop is
already a return trip — so the main machine runs two of them and the second computer runs
the third. Full conditions in `m3_considerations.md`.
**Passed 2026-08-03**, every phase on the first attempt (`e2e/run-m3-lan.sh`): the ring
closed across two physical machines — slots 1 and 3 here, slot 2 on the owner's second
computer — and a *natural* emigrant, not the forced one, completed the circuit and came home
byte-equal, counted **exactly once** ring-wide. Natural migration is already flowing across
the LAN: the far world exported seven organisms on its own in the ~12 minutes the ring was
live before the test began, so phase 3 never had to wait. **The rig was left running as a
living deployment.** Full result in `m3_considerations.md`, *Exit Test → Result*.
**What the living deployment then taught, and why M4 exists.** The rig ran unattended for
about 12.5 hours and stopped with its host on 2026-08-04T14:13Z. T0 and T1 captured it
(`e2e/baselines/20260804T013927Z/baseline-report.md`,
`e2e/baselines/20260805T230730Z/t1-report.md`). The migration layer held perfectly —
22 595 hops, exactly-once, one hop still in flight — and the three worlds evolved as **one
coupled population**, which is the result the ring was built to produce. Everything around
that layer failed: the worlds were never saved and 97% of their state is gone, a two-hour
outage on one slot stopped the current for every peer behind it, and every surviving series
came out of BepInEx logs that the next game launch overwrites. Those three failures are
D14, D12 and D15.

### M4 — Operations: resilience, topology, observability — **COMPLETE**
**Defined 2026-08-05 by D12–D16. Full plan: `m4_considerations.md`. Findings:
`m4_findings.md`.** Still a LAN
milestone: no VPS, no strangers, no public exposure. M4 makes the rig something that can be
run, rather than something that has to be watched.

Four things arrive together. The owner signed the design off on 2026-08-05 and overrode
three of its recommendations — the grid, the hold and the entry edge below carry the
result. **The hold was then removed entirely on 2026-08-17** (`contract-b-m4.md` §25, B37),
so the paragraph below records what M4 shipped and the sentence after it records what runs.

- **A map that heals and grows** (D12). A dark slot is routed around, not waited on; a
  returning peer or a brand-new instance splices in **between two live slots**. The
  journal learns a handoff state, because re-routing an orphaned hop is safe only under a
  proof of non-delivery, and doing nothing is the answer whenever custody may have moved
  (D2). ~~A hold now expires: after a configurable timeout, 24 hours by default, the
  organism bounces home.~~ **Since 2026-08-17 nothing is held and nothing comes home**:
  the entry is recorded lost at `forwardTimeoutMs` and the organism is gone, which is what
  buys the map a system with no duplication case in it.
- **The grid, live** (D13). Plural export edges, per-lane edge state, coordinate
  addressing and per-axis route-around land now, before M5 makes the wire public — and
  the **vertical lanes run**, with the second capture band and the corner rule in
  production. The rig grows to an honest **3×2** map, four or five instances on the main
  machine and one or two on the second.
- **Hard-stop recovery** (D14). Periodic world saves and a save on quit, a scripted
  single-instance recovery procedure, and a rig-wide resume test that delivers the
  organism which has been in flight in slot 1's journal since 2026-08-04. A save stalls
  the sim by at most 2 seconds. No sleep-prevention work of any kind.
- **The operator surface** (D15). Population on the ring view, durable metrics outside the
  BepInEx logs, an archive-served live status page and a `ringstat` command, log
  preservation at shutdown, a slot handover command, a join kit, and **arrival pacing** at
  the entry edge with the crowding metric that verifies it.

**Exit test:** one slot is hard-stopped mid-flow and the ring heals around it while the
current keeps running; the dead slot splices back in between two live slots; a brand-new
instance splices into a live ring; a live 3×2 grid drifts organisms diagonally across both
axes; a dark slot is woken against accumulated traffic and its arrivals are paced rather
than instantaneous; the 2026-08-04 in-flight organism arrives; a kill-and-reload proves the
periodic saves by coming back with fresh state instead of stale state; and the status page
shows all of it. Full conditions in `m4_considerations.md`.
**Passed 2026-08-06** (`e2e/run-m4-lan.sh`): **six real worlds on two computers** in a 3×2
map, twelve lanes open, organisms migrating on both axes, and a hard-killed slot healed
**across the LAN** before it reclaimed its own slot and position — with exactly-once held
everywhere and the portals confirmed on screen by the owner.
The rig then tested two-way lanes and restart recovery. Full results are in
`m4_considerations.md`, *Exit Test → Result*. Stable findings from the rig are in
`dev_environment.md`, *The living deployment*.

### M5 — Public release — **PUBLIC EVIDENCE PHASE**

**Opened 2026-08-09. Full plan: `m5_considerations.md`. Frozen delivery record:
`m5_tracking.md`. Current public state: `STATUS.md`.**

The owner ratified nine decisions on 2026-08-10. Five became D21 through D25 above.
Four remain milestone decisions in `m5_considerations.md`.

M5 delivered the public `0.1.0` release and its hosted service. The release includes TLS,
peer-bound credentials, capacity limits, participant packages, diagnostics, and support guides.
It publishes `contract-a/2.4` and `contract-b/4.0`.

The first announced service period runs from **August 14 through November 14, 2026**.
The release channel is GitHub Releases, with checksums and security guidance.

The remaining M5 evidence comes from the public playtest. The exit test requires at least four
non-owner peers, at least 72 continuous hours, and no operator action on participant computers.
Relay-side actions are allowed and counted. Full conditions are in `m5_considerations.md`,
*Exit Test*.

### M6 — Direct P2P
libp2p transport behind unchanged Contract B shapes; NAT traversal; peer discovery;
gossip topology; lease-based slot claims. The relay degrades to bootstrap +
relay-of-last-resort. **It also carries the control surface (D23)**, which M5 makes possible
and does not build: its own message type, ordering and idempotency for a write that races a
world load, semantics for a disconnected target, and an audit trail. M5's per-peer credential
should be read with this milestone in view — a secret issued by a relay is a different object
from a peer identity that survives the relay's demotion.

### M7 — Ecosystem completeness
Corpse and pellet payloads for biomass continuity (§6.4); egg-handling research;
species-catalog module, and its reconciliation with M3's archive (D6, D11).

**Nothing is unscheduled.** The live grid was the one open call, and the owner answered it
into M4 on 2026-08-05: the vertical lanes ship on, the second capture band runs in
production, and the rig grows to six instances across the two machines. See
`m4_considerations.md`, *Owner Sign-Offs*.

---

## What We Know vs. What We Need to Research

| Component | What we know (from research doc) | What we need to figure out |
|---|---|---|
| `bb8-schema` | JSON structure, node layout, synapse rules, gene array, weight polymorphism, validation constraints. From `m1_findings.md`: the game's `.bb8` top-level keys (`transform`, `rb2d`, `genes`, `body`, `clock`, `brain`, `version`, `desc`) and the fact that a `.bb8` carries full live state while a `.bb8template` does not; genes are keyed by enum **name**, so gene reordering is safe and only additions/removals need conversion; **the `Utility.Version` alpha quirk** — a 4-argument `new Version(0,6,3,3)` binds to the alpha overload and means `0.6.3a3`, *not* `0.6.3.3`, and `V3` sorts before `Alpha`, so `bb8-schema` must reproduce that ordering | Version differences between game updates; cross-version conversion (EinsteinEditor prior art) — **back to ordinary research status on 2026-08-11, after one day on M5's critical path**. D22 put it there because a support matrix over game versions means a payload serialized by game version X can be restored into game version Y; the owner's refinement **took it off again**: the payload stays opaque, cross-version loading is **assumed** to work, the envelope carries the serializing game's version as diagnostic metadata no new reader may parse into a capability decision, and both fuller answers — evidence of stability, or a marker with a defined refusal path — are deferred. **The same day he also chose to keep the four shipped version gates**, which changes what would reopen this row: a cross-version payload cannot reach the loader while they stand, so the trigger is not a failed restore but version skew becoming an operational problem — a map partitioned along a version boundary for longer than an update takes to roll out. **The trigger, not the schedule, is what reopens this row**; the exact byte shape of community-tool `.bb8` dialects; whether Newtonsoft honours `[NonSerialized]` on `NEATBrain.Node.NIn/NOut` in the shipped DLL; corpse/pellet/egg payload shapes (M7). **New with D11:** the canonical **genome projection** behind `genomeHash` — which keys it covers, how it is ordered and normalized so two peers hash one genome identically, and whether a mutated child must hash differently from its parent for the lineage graph to be useful |
| `bibites-mod` | BepInEx/Harmony setup, export/import patterns, threading constraints, the Constance-Mod overwrite technique (now only needed for the egg-hatch fallback). From `m1_findings.md`: exact signatures in `BibitesAssembly.dll`; spawn/restore API (`SaveSystem.LoadBibiteOrEggFromData`, public, no mutation, ID-preserving); serialize API (`SaveSystem.SerializeBibite`/`SaveBibite`); live-attribute access is public except `InternalClock` (covered by public `SerializationHelper.DeserializeObject`, so no mod-side reflection); no ID registry exists — IDs are random int32; clean removal is raw `Object.Destroy` after de-listing from `BibiteTracker`; patch targets (`BibiteBody.FixedUpdate()`, `EggHatching.Hatch()`). **Confirmed in-game 2026-08-02** (`m1_findings.md`, *Runtime results*): the round trip is byte-exact, the ID survives, and the world save that follows succeeds; `GameManager.StartGame(path)` loads a named world headlessly, and `SaveSystem.CreateSave` must be driven by hand because the public `SaveGame` wrapper swallows save exceptions inside a coroutine. **Two game instances do run at once on one machine** — verified 2026-08-02: both processes persisted, and BepInEx gave the second instance `BepInEx/LogOutput.log.1` because the first holds a lock on `LogOutput.log`, so neither log is truncated | ~~Re-linking parent/child references after respawn: unproven against a linked organism~~ — **closed in M2** (2026-08-02/03): a real family migration fired the stale-reference trap, the mod cleared the stale `BibiteID`, a landing parent reported `relinkedParents=1`, and both worlds saved. Still open: migration order is child-then-parent, because `BibiteGenes.SaveState` drops a child's parentage once the parent GameObject is gone (M3). **Void-avoidance suppression is answered** (`m2_findings.md` §1.5): a Harmony prefix/postfix pair on `BibitePropulsion.UpdateOrgan` toggling the private static `avoidVoid` gives per-edge scoping with no persisted trace — but the setting ships off, so the real M2 question is the **lure**, not the suppression. **Per-instance mod config is answered**: environment variables named in `WSLENV`, since both instances share one plugin DLL and one BepInEx config directory. ~~Whether `WorldObjectsSpawner.bibiteHolder`'s transform is the identity~~ — **answered 2026-08-02** (`m2_findings.md` §4.3): the holder sits at `(0, 0, −0.01)` with zero rotation and unit scale, so the payload's `transform` key is world space in `x` and `y`. **New with D10:** does the vanilla wrap coexist with the border strips — no false migration, no interference; and does export capture reach an organism already **outside** the square, in the band between the strip line and the wrap radius (2000–4000 at `S = 2000`)? **New with D11:** the cost of serializing up to two living parents at export time, on the main thread, inside `FixedUpdate`. The mod ships opaque blobs and never hashes them — D4 forbids it to parse a genome (`m3_considerations.md` Risk 8) **New with D13:** the **corner rule** — an organism inside both capture bands with both velocity components outward must pick one export edge, and nothing in the ring's geometry answers that. **New with D14:** what a periodic `SaveSystem.CreateSave` costs a running sim, at what world size, and whether it holds the **2-second stall budget** the owner set on 2026-08-05 — the budget is the test bar, and a simple synchronous save is the design that has to pass it. ~~**New with D15:** why organisms crowd the entry edge~~ — **answered by the owner 2026-08-05**: the sleeping machine dammed the flow, and the queued custody deliveries plus the west neighbour's contained export pile released together on wake. The fix is the sidecar's arrival pacing (D15), not a mod-side change; the mod's job is the crowding metric that verifies the pacing. The other candidates — arrival rate near carrying capacity, transverse clustering from `exitPosition`, no pellet zone under the arrival band, the shaded band outside the square — stay as secondary contributors the metric will rank. |
| `multiverse-sidecar` | Custody chain and journal semantics (D2); admission-control levers. Contract A reconnection semantics are now settled in `contracts/contract-a.md` §6–§8: the mod is stateless and reconnects with jittered backoff, the sidecar replays every un-ACKed `MIGRATE_IN` in journal order, both sides dedup on `migrationId`, and a changed `sessionId` triggers custody reassertion | Journal format and crash recovery (durability rule is fixed: flush before `MIGRATE_OUT_ACK`, and from 2026-08-08 also **truncate back on a failed append** — refusing to ACK was necessary and not sufficient, because a short write left a fragment that made replay discard eight hours of history behind it, `contract-b-m4.md` §20 — **verified in the field** by the host reboots of 2026-08-08 and 2026-08-09, the only event that replays a journal on a rig whose value is staying up: both replayed all five journals with zero discarded bytes and no error line, `m4_considerations.md` Risk 6); go-libp2p maturity (M6); lease design and churn healing for slot claims (M6); how a sidecar serves a genome by hash for a migration it has already tombstoned (D11) **New with D12:** **re-route versus hold for an orphaned journal hop** — the proposed rule is re-route only under a relay proof of non-delivery and hold otherwise, and what needs settling is the exact set of relay answers that count as proof, and whether a peer-specific `MIGRATION_NACK` (overloaded, size mismatch) is such a proof. ~~What an operator may do with an entry held forever~~ — **answered 2026-08-05**, and answered again on **2026-08-17**: nothing is held at all. The entry is recorded **lost** at `forwardTimeoutMs`, the organism is gone rather than returned, and `--release-inflight` resolves one earlier by hand (`contract-b-m4.md` §25, B37). The deadline still lives in the journal entry — as `sentAt`, a durable wall-clock instant — but it is now a bookkeeping deadline rather than a custody one, so a sidecar that slept through it records a loss that had already happened. **New with D15:** **arrival pacing** — what rate unit the limit uses when the sim runs at 20×, whether pacing is per source lane or per slot, what a paced backlog does to journal depth and to the custody timeout above, and how the metric proves a burst was actually spread. **New with D14:** journal replay and custody reassertion against a world that reloaded from a save *older* than the journal — the organism is delivered by custody, not by the save, and the two views of the same world then disagree. |
| `multiverse-relay` | Star routing and single-arbiter slot assignment are well-trodden. ~~Who operates it~~ — **answered for M3 by D9:** the owner's main machine. ~~Grid pairing of `{x, y}` sectors~~ — **retired by D8:** the ring gives each peer one east neighbour, and the pairing question disappears with the grid | **The ring insertion protocol** (D8): where a new peer is inserted, whether the owner chooses the slot or the relay does, how the east-neighbour map is published and re-published, and what a slot reservation costs when a peer never comes back. ~~**Who operates a public relay**~~ — **answered 2026-08-10 by D24**: the owner, for a **bounded and announced period** rather than as an open-ended service, with the publish-the-relay path prepared and the ending stated to participants before they join. Who operates the later bootstrap nodes is M6's question and stays open. Still open here: capacity and abuse limits. `m5_considerations.md` DQ3 carries them, every one of them countable at the frame level so D1 survives them. **New with D12:** the **2-D and healing insertion protocol** — how a peer names where it splices in, who arbitrates a contested position, how the effective-neighbour map is recomputed and republished without a storm, and what the relay must answer so a sender can prove a frame was never forwarded. **New with D13:** how a coordinate map grows — fill a hole, or extend an axis and create a row of holes — and whether a ragged grid is ever legal. **New with D15:** slot handover, and what it must refuse. |
| `multiverse-archive` | Migration is the moment a genome crosses a machine boundary (D11); content-addressing is the same technique `species-catalog` will need | **How it sees every envelope**: a copy forwarded by the relay, or the archive joining the ring as a slot-less peer — the first adds a rule to a deliberately dumb relay (D1), the second adds a peer that owns no sector. **The fetch protocol**: `GENOME_REQUEST`/`GENOME_RESPONSE` shape, who answers when the source peer is offline, and what the archive does with a hash nobody can serve — one hash has reached the top of the ladder in the whole history of the rig, because every peer here comes back, and `m5_considerations.md` **Risk 7** is where a departed stranger makes that the normal case. Storage growth, and the query surface. Whether M7's catalog seeds from it or supersedes it (D6) — **the call moves into M5, ratified 2026-08-10** (`m5_considerations.md`, decision 3), because a public archive fills on somebody else's timetable and nothing may evict (D11; `contract-b-m4.md` §10, §20). M5 produces the retention rule — keep everything and size the disk, keep the ledger and prune genomes to a horizon, or graduate to the catalog now — **even if the rule is *keep everything***, and ships the per-peer growth arithmetic to whoever hosts it either way. What the rule will say is the open part; that it exists by the end of M5 is not. D24's announced ending is where it has to be stated, because a bounded run has to say what becomes of the record. **New with D15:** the live status page and the durable metrics — what the archive can serve from envelopes alone, what it needs the sidecars to report, and how a self-contained HTML page stays honest about a peer it cannot see. |
| `species-catalog` | Community already shares `.bb8` on GitHub/Steam Workshop; content-addressing makes sense | Storage limits, search/index strategy, replication policy (all M7) |
| **The LAN rig (M3, D9)** | Two game instances run side by side on one machine (M2); `game.sh` starts, stops and tails each one; per-instance configuration travels in environment variables named in `WSLENV`; the dev machine is WSL driving a Windows game install (`dev_environment.md`) | **LAN reachability**: which host the second machine dials, whether a Windows Firewall rule is needed for the relay port, and hostname vs static IP. **The second machine's install path**: it has no WSL, no .NET SDK and no Go toolchain, so the mod DLL, BepInEx, the sidecar binary and the sidecar's configuration must arrive as artifacts, and the environment variables must be set without `WSLENV`. **Test drive**: whether `e2e/` can steer a game on the far machine at all, or whether that end of the M3 exit test is operator-driven. **Drift**: two wall clocks and two game/mod versions instead of one **New with D14–D15 (the rig as a deployment):** the rig now has to survive its own operator. Open: the save cadence that costs the least sim time; how a save rotation avoids a half-written zip; how the far end saves and preserves logs without a drive path (M3's answer was "observe only", and a save is not an observation); where the durable metrics live so a harvest needs no BepInEx log; and how a fourth or fifth instance is added to a machine that already runs three — now a **six-instance** question, because D13's live grid puts 4+2 or 5+1 across the two machines. Per-instance BepInEx log naming (`LogOutput.log.N`), per-instance environment variables and the far end's two PowerShell scripts all have to scale with it. |
| **The portal (M4)** | **Research complete: `m4_portal_findings.md` (2026-08-05). It is feasible with zero shipped assets.** The game bundles Freya Holmér's **Shapes** (`ShapesRuntime.dll`), all 154 `Shapes/*` shader variants are in the build's *Always Included Shaders* list, and the game itself already does `AddComponent<Rectangle>()` at runtime. Retained mode works; immediate mode (`Shapes.Draw.*`) does not, because it needs a `ShapesRenderFeature` on the URP renderer that a mod cannot add. Recommended build: one **additive gradient `Rectangle` per open edge** with a chevron `DashOffset` flow — outward on an export edge, inward on an entry edge — and **pixel-space line thickness**, which is the one-enum answer to the 800× zoom range. Per-event flourish: an expanding `Disc` ring, preferred over `ParticlesMaster` because the particle route goes silent when the player turns pheromone display off. Geometry comes from the same `BorderGeometry` the capture logic uses, so the visual cannot drift from the mechanic | **The camera layer and culling mask** — ranked top risk, and the one thing that makes the portal invisible with no error. Layers live in prefabs, not in the assembly, so it is not resolvable statically; mitigated by copying the layer from a live zone renderer, and it costs one runtime session to settle. Then: world-space sorting order against bibites and pellets (make it configurable), and a legibility sweep at `orthographicSize` 5, 250, 2000 and 4000. Informational only: whether the URP renderer carries `ShapesRenderFeature`, and whether a per-material `_fertilityColor` overrides the global (it gates the parked native-look option, not the recommended one) |
| **The map (D12, D13)** | The ring is proven across two machines and 22 595 hops. A slot number is an address, a coordinate is a position, and the two are now separate concepts. The ring is the one-row case of the grid. | **2-D insertion**: what "between two live slots" means when the map has two axes, whether a newcomer fills a hole or extends an axis, and who chooses. **Route-around on a grid**: skipping along one axis while the other lane is healthy, and what an export does when a whole row or column is dark. ~~**Degenerate shapes**: what is the smallest honest grid, and can two machines host it~~ — **answered 2026-08-05**: 2×2 is degenerate in both axes exactly as a 2-slot ring is, **3×2 is the smallest honest two-axis map**, and the two machines do host it — three game instances ran comfortably at about 550 MB each on the 128 GiB main host, so 4+2 or 5+1 is the M4 target. **The organism's view**: an organism that is routed around a slot never sees that world, so a bypass is not free — it is a shorter cycle. |
| **Contract A** | **Fully specified in `contracts/contract-a.md` (2026-08-02):** envelope with a `protocol` version field, all nine message schemas with field tables and examples, the edge-position formula, connection/reconnection and replay semantics, heartbeat cadence and stop behaviour, both NACK error taxonomies, WebSocket close codes, and the tunables table | Nothing blocking M2. Deferred: authentication — Contract A stays on the loopback even in M3, because the mod and its sidecar always share a machine (D9), so a bearer token can wait for M4. Confirm the three design calls flagged in §12 — unsolicited `MIGRATE_OUT_ACK` as custody reassertion, normalized rather than absolute entry coordinates, and `entityId` as the mod's durable dedup key. **M3 amendments:** narrow `EDGE_STATUS` to the single export edge and state that the entry edge is passive (D8); carry the lineage annex on `MIGRATE_OUT` (D11) **M4 amendments (D13–D15):** `exportEdge` becomes `exportEdges` and `EDGE_STATUS.edges` carries one entry per export edge, which is a **breaking** change and the reason it lands before the wire goes public; `borderEdges` becomes all four edges; the entry-position rule **keeps mirroring**, because D15 answers crowding with sidecar-side pacing and parks arrival spreading; a bearer token still waits, now for M5. Contract debt A5 stays moot even under two export edges, because a `MIGRATE_OUT_NACK` is correlated on `migrationId`, never on an edge. **Species identity now travels (ratified 2026-08-07):** an OPTIONAL species-name block rides `MIGRATE_OUT` / `MIGRATE_IN` and the Contract C envelope, opaque to every sidecar and to the relay, and the importing mod resolves it by name and rewrites `genes.speciesID` in the blob before the restore — `contract-a.md` §16 (A30–A34, `contract-a/2.1`) and `contract-b-m4.md` §15 (B9–B10, `contract-b/3.1`). **A34** adds the one repair the system allows: the *exporting* mod normalizes each name half's whitespace before validating it, because the game issues an edge space in about 2% of halves and those organisms were travelling with no block at all; the importer matches on the normalized form so a repaired name still finds the raw-form local species. No field, no version and no sidecar behaviour moves. |
| **Contract B** | Migration payloads = envelope + opaque body; ACK gated on the receiving mod's `MIGRATE_IN_ACK` | Protobuf vs JSON framing; encryption (a shared token over the LAN in M3, TLS once the relay is public in M5, libp2p secure channels in M6); protocol versioning. **M5 amendments (D21, D22):** the shared token becomes a per-peer credential bound to the `peerId`, which replaces §3.1's rule rather than extending it and takes the wire to **`contract-b/4`** — the one major this project has planned, taken before strangers run the build; and the handshake **gains** a compatibility gate on the **contract** version, with the game version a per-machine matter in the design and a claimed version never a security decision — though the shipped game-version refusal is **kept** beside it, by the owner's call of 2026-08-11, until skew causes an operational problem. **M3 amendments:** `SECTOR_CLAIM`/`SECTOR_GRANT` become ring claims that also name the east neighbour, the envelope gains the annex, and `GENOME_REQUEST`/`GENOME_RESPONSE` join the catalogue (D8, D11) **M4 amendments (D12, D13, D15):** a grant names the **effective** neighbour per export edge and the slots it skipped; `PEER_STATUS` keeps the structural order and gains population per slot; a claim carries an advisory insertion position and, once the grid is live, coordinates; the relay gains an explicit non-delivery answer that a sender may treat as proof. |
