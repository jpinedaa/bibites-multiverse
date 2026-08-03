# M3 Considerations — The LAN Ring

This report expands milestone M3 of `system_decomposition.md`.

**Status: IN PROGRESS.** The owner redefined this milestone on 2026-08-03. Decisions D8 to
D11 hold the new shape. The three-slot rehearsal on one machine passed. The LAN packaging
is built. The LAN exit test is the remaining step.

## Purpose

M3 is the first slice with a real remote peer. The relay runs on the main machine of the
owner. The second computer of the owner joins over the LAN. That second machine gives a
second clock, a second install and a real network hop.

M3 also gives the multiverse a shape. Decision D8 puts every peer in one ring. Each sim
exports through its east edge and receives through its west edge. The two doors are
different, so an organism comes home only after a full circuit.

M3 starts the organism database. Decision D11 adds a lineage annex to every migration
envelope. A new service records each envelope, each annex and each genome.

M1 removed the in-game risks. M2 proved the custody chain on one machine. `m1_findings.md`,
`m2_findings.md`, `m2_considerations.md`, `contracts/contract-a.md` and
`contracts/contract-b-m2.md` hold those results. This document names the new work only.

Three slots make the ring, on two physical machines:

```
   ┌────────► slot 1 ────────► slot 2 ────────► slot 3 ────────┐
   │           (main)          (second)         (main)         │
   └───────────────────────────────────────────────────────────┘

   Every arrow is one lane: out of an east edge, into a west edge.
   Main machine:    slot 1, slot 3, the relay, the archive.
   Second computer: slot 2.
```

Two slots make a degenerate ring. Slot A exports to slot B, and slot B exports back to
slot A. One hop then returns an organism home, and the milestone proves no circuit. Three
slots are therefore the minimum, and the LAN carries two of the three lanes.

## Scope

In scope:

- One ring of three slots, across two physical machines
- Ring insertion at the relay, keyed on the peer identity
- One export edge (east) and one passive entry edge (west) for each sim
- The relay, the archive, one sidecar and one game instance on the main machine
- A second sidecar and a second game instance on the main machine, for slot 3
- One sidecar and one game instance on the second computer, over the LAN
- A shared token on the Contract B connection
- World wrapping left ON as the containment mechanism (D10)
- The two in-game tests of D10 — Risk 1 and Risk 2 below
- The lineage annex on every migration envelope
- `multiverse-archive`, beside the relay
- Genome fetch by content hash, from the archive to a sidecar
- Handshake and compatibility enforcement across two separate installs
- Admission control, and hardening of the journal recovery
- Deep bb8 validation, carried from M2
- An install path for a Windows machine with no development environment

Out of scope. Decision D9 moves these five items, intact, to M4:

- A hosted relay on a VPS
- TLS, and authentication for public exposure
- Capacity limits and abuse limits
- A player-facing package for the mod and the sidecar
- A community playtest with strangers

Also out of scope:

- libp2p and direct peer-to-peer transport (M5)
- The `species-catalog` module (M6)
- Corpse, pellet and egg payloads (M6)
- The `{x, y}` sector grid. Decision D8 retires it
- Census uploads to the archive. The annex covers migrants and their ancestors only (D11)
- The pheromone beacon and the gateway towers. Both stay parked (D3)
- A write interface on the archive. M3 records and reads only

## Risks

### Risk 1 — The wrap and the strips act on the same organisms. (D10, test a)

Decision D10 keeps `worldWrapping` ON. The wrap teleports an organism at `1.5 · S + 1000`
to the opposite point of the map. It keeps the velocity and the rotation.

The two radii are far apart. The strip captures at `±S`, and the wrap fires at 4000 with
`S = 2000`. An export therefore wins the race on paper. Paper is not evidence, and D10
calls for a test in the game.

The velocity test is what separates the two rules. An organism that the wrap moves arrives
in the outer band on the far side, and it travels **inward**. An export needs an
**outward** velocity. Risk 2 extends the capture into that same outer band, so the two
rules meet there.

Run test (a) on a world with an open export edge. The test passes when all of these
results hold:

- The wrap starts no migration.
- A wrapped organism keeps its velocity, its rotation and its health.
- No organism crosses the export edge without an outward velocity.
- The strip counter and the wrap events agree with the population count.
- Neither BepInEx log holds an error.

Do not remove the outward-velocity test from the outer band. Without it, every wrapped
arrival exports on the edge it lands behind.

### Risk 2 — Export capture must reach outside the square. (D10, test b)

The strip is a band inside the square. A fast organism passes the whole strip in one tick.
That organism is then outside the square, and outside the capture rule of M2.

With the wrap ON, that organism travels to `1.5 · S + 1000` and teleports home. It does
not leave the world, and it does not migrate either. A player reads the teleport as a
defect in the mod.

Extend the export capture to the whole band from the strip line to the wrap radius. With
`S = 2000` that band is 2000 to 4000. Keep the outward-velocity test on the band, for the
reason in Risk 1.

Two details need care in that band. `exitPosition` needs the clamp of
`contracts/contract-a.md` §4.3, because a diagonal escape gives a value outside `[0, 1]`.
An organism near the wrap radius also needs capture **before** the wrap fires, so test the
position each `FixedUpdate` and never on a slower timer.

Run test (b) with a forced outward push, at a speed that clears the strip in one tick. The
test passes when the organism migrates from the band, and the wrap never fires.

### Risk 3 — The entry edge is passive, and passive is not open

Under decision D8 the west edge receives only. It never exports. An organism that walks
west out of the sim is not a migrant, and the wrap returns it (D10).

The one-way lanes remove the ping-pong problem of M2. An arrival lands in the west entry
strip, and the export strip is on the east side. An immediate re-export is therefore
impossible by geometry, not by a timer.

Keep the entry-immunity window anyway. It is cheap, and it still covers a bounce-back,
which re-delivers an organism on the edge it left from.

Confirm one behaviour in the game: an organism that arrives on the west edge and then
travels west leaves the square, and the wrap returns it. That path is the entry-edge half
of D10, and no strip guards it.

### Risk 4 — Ring insertion must not move a peer that already has a slot

Decision D8 binds a slot to a peer identity. The relay must hold that slot while the peer
is offline. The reservation never expires.

Three failures are possible. A reinstalled sidecar with a new `peerId` takes a second
slot, and its old slot stays reserved forever. An insertion between two live peers changes
the east neighbour of one peer. An in-flight migration then names a destination that is no
longer the neighbour.

Three rules answer them. Persist `peerId` outside the journal, in the data directory, with
the same care as the sector file of `contract-b-m2.md` §8 item 4. Apply an insertion to new
migrations only, and let the journal keep the destination that it recorded. Give the owner
a command that releases a reserved slot by hand.

The relay grants the slot **and** the east neighbour together. A sidecar needs no other
topology.

### Risk 5 — Two machines have two clocks and two installs

Decision D5 already removes the global clock, and envelope timestamps are informational.
The second machine adds wall-clock drift between the two hosts.

Drift is harmless for custody and painful for reading logs. The archive records one
timestamp for each envelope, and that record comes from two clocks. Record the receive
time of the relay beside the sender timestamp, so one clock orders the archive.

Version drift is the harder half. Steam updates the second machine on its own schedule, so
the game version, the mod version and `SimulationSize` can all differ. The relay already
enforces compatibility at connect, and `contract-a.md` §13 A10 gives the relative test
for `S`.

Make the rejection loud. A silent version mismatch looks exactly like a dead peer, because
both end with a closed export edge.

### Risk 6 — The second computer has no development environment

**Status: RESOLVED on 2026-08-03.**

The main machine runs WSL, the .NET SDK, the Go toolchain and `game.sh`. The second
computer has none of them. `WSLENV` does not exist there.

The first half of the answer is a bundle of artifacts, not a build.
`farend/make-farend-bundle.sh` makes `farend/dist/farend-bundle.zip` on the main machine.
The zip holds the BepInEx release, a fresh plugin DLL, the Windows sidecar and two
scripts. `setup-farend.ps1` finds the Steam install and installs the three parts. It also
compares `BibitesAssembly.dll` against a hash in the script. A different hash stops the
installation and prints the two ways forward. `start-slot2.ps1` starts the sidecar, waits
for the ring slot, and then starts the game. `farend/README.md` holds the human steps.

The second half of the answer removes the drive path. **The rig never drives the far
end.** `e2e/run-m3-lan.sh` reads every fact about slot 2 on the main machine:

- The relay's `ring.json` gives the slot of the far end.
- The `EDGE_STATUS` ripple into slot 1 gives its liveness. An export edge opens only for a
  live neighbour that has a game.
- The archive records each hop into slot 2 and each hop out of it. The relay copies each
  envelope and each answer to the archive.
- Slot 3 records the arrival of each organism that leaves slot 2.

The rig cannot force an export on the far end, and it does not need to. Organisms cross
east by themselves. Phase 3 waits for that natural crossing, with a wide timeout. The
operator of the second computer runs two scripts and nothing more. The unattended property
of M2 stays intact for every part of the test that this machine owns.

Two network steps stay with the owner. Windows Firewall blocks the relay port. WSL2 also
runs behind NAT on the main machine, so Windows must forward the port into the WSL virtual
machine. `e2e/run-m3-lan.sh lanhost` prints both commands with the current addresses.
`dev_environment.md` records the two commands and the port.

### Risk 7 — The archive fetches genomes across the LAN

The archive holds a hash for every parent in every annex. It holds the genome only after a
fetch succeeds.

Four failures are possible. The source peer is offline. The parent never migrated, so no
sidecar journal holds it. A busy ring produces a fetch storm. The source sidecar restarts
and loses its cache.

The proposed answer keeps the migration path free of the archive. Never block a migration
on a fetch. Record an unresolved hash as a gap, and retry it later. Keep the hash forever,
because a later fetch can succeed. Rate-limit the fetches from one archive to one sidecar.

The parent-genome cache is the part that needs the owner's decision. See *Contract Changes
Needed*, item 4, and *Design Calls for the Owner*.

### Risk 8 — The mod must not parse a genome, and the annex needs a genome hash

Decision D4 makes the bb8 body opaque to the mod. Only `bb8-schema` parses it, sidecar
side. A hash of a genome projection is a parse.

The mod therefore cannot compute a parent hash. The proposed split keeps D4 intact. The
mod reads the parent entity IDs, serializes each living parent with
`SaveSystem.SerializeBibite`, and sends the two opaque blobs beside the migrant on
`MIGRATE_OUT`. The sidecar hashes them with `bb8-schema`, builds the annex from the
hashes, caches the blobs by hash, and strips the blobs from the wire envelope.

This split answers Risk 7 as well. The source sidecar holds the parent genome, so it can
answer the fetch of the archive.

The cost lands on the export path, on the main thread. An export already serializes one
organism. The annex adds up to two more serializations in the same `FixedUpdate`.
Measure that cost before the exit test, and cache by entity ID inside one tick.

A missing parent is normal, and it is not an error. `BibiteGenes` drops the parentage once
the parent GameObject is gone. Record the gap in the annex.

### Risk 9 — The genome projection must be stable across peers

A hash is useful only when two peers produce the same value for one genome. A hash over
the whole `.bb8` fails, because live state changes every tick.

`bb8-schema` owns the projection: genes, nodes and synapses, in a canonical order, with a
fixed number format. Two open questions come with it. Float formatting must be exact, and
a re-serialized genome must give the same bytes on both machines. A mutated child must
also hash differently from its parent, or the lineage graph collapses into one node.

Answer both questions with the deep bb8 validation work that M2 deferred. The two tasks
read the same fields.

### Risk 10 — Contract debt A5 is moot under the ring

`MIGRATE_OUT_NACK` carries no `edge` field (`contract-a.md` §13 A5, §12 item 6). M2
accepted the debt because each sim opened exactly one edge.

The ring keeps that condition permanently. Each sim exports through its east edge only, so
a `MIGRATE_OUT_NACK` can name no other edge. The mod correlates the NACK through
`migrationId`, exactly as in M2.

**Assessment: M3 needs no change here.** Do not close the debt in the contract. The retired
`{x, y}` grid revives it, and D8 keeps that grid on record as a far-future extension.
Record this assessment in the next contract pass.

## Contract Changes Needed

This pass does not rewrite the contracts. The list below is the design input for the
implementation wave. Each item names the document and the decision behind it.

| # | Document | Change | Source |
|---|---|---|---|
| 1 | `contract-b-m2.md` §1, §5.3, §5.4 | Replace the two-sector map with the ring. `SECTOR_CLAIM` becomes a ring claim. `SECTOR_GRANT` returns the slot **and** the east neighbour. Retire the `["A", "B"]` set and the fixed east-west pairing of §10 item 3. | D8 |
| 2 | `contract-b-m2.md` §5.5 | `PEER_STATUS` reports the ring order, not a sector list. A vacant slot stays in the ring with no live peer. | D8 |
| 3 | `contract-b-m2.md` §5.6 | `MIGRATION_PAYLOAD` gains the `lineage` annex. `sourceSector` and `destSector` become ring slots. `exitEdge` is always `E`. | D8, D11 |
| 4 | `contract-b-m2.md` §5 | Add `GENOME_REQUEST` and `GENOME_RESPONSE`, for the archive. Name the answering party, the timeout, and the answer for an unknown hash. | D11 |
| 5 | `contract-b-m2.md` §2, §10 item 1 | Add the shared token on the WebSocket upgrade. The wire leaves the loopback in M3. TLS waits for M4. | D9 |
| 6 | `contract-a.md` §5.4 | Narrow `EDGE_STATUS` to the export edge. State that the entry edge is passive and always accepts an inbound organism. | D8 |
| 7 | `contract-a.md` §5.3 | `MIGRATE_OUT` gains the parent entity IDs and the opaque parent blobs. The sidecar hashes them and strips them from the wire envelope. | D11, D4 |
| 8 | `contract-a.md` §4.3 | State the capture band outside the square, and the clamp that a diagonal escape needs. | D10 |
| 9 | `contract-a.md` §13 A5 | Record the assessment of Risk 10. The debt stays open and stays moot. | D8 |
| 10 | Both documents | Bump the major version, because item 1 and item 6 remove fields. | D8 |

## Work Packages

Seven packages. WP1 gates every other package, because it settles the wire.

### WP1 — The contract amendments

**Depends on:** this document.
**Needs the game:** no.

- The ten items of *Contract Changes Needed*
- One worked example for each new message
- The version bump, and the migration note for the M2 rig

Write WP1 before any code. M2 proved the cost of two implementations that resolve the same
ambiguity alone. `contract-a.md` §13 holds ten such resolutions.

### WP2 — `bb8-schema`: the genome projection and the deep validation

**Depends on:** WP1.
**Needs the game:** no.

- The canonical genome projection, and the hash over it (Risk 9)
- Gene, node and synapse validation, carried from M2
- Dialect detection between `.bb8` and `.bb8template`
- The `Utility.Version` alpha quirk in the version order
- A cross-machine test: one genome, two hosts, one hash

WP2 blocks WP3 and WP5. Both of them hash genomes.

### WP3 — The Go side: the ring, the token and the annex

**Depends on:** WP1, WP2.
**Needs the game:** no.

- Ring insertion at the relay, keyed on `peerId`, with a held slot and a manual release
- ✓ A manual reservation as well, `--reserve-slot <peerId>`. It writes the ring order
  before any peer connects, so the LAN ring forms in any start order (WP6, 2026-08-03)
- The east-neighbour map, published in `SECTOR_GRANT` and `PEER_STATUS`
- The one-way `EDGE_STATUS` ripple. A dead peer closes the export edge of its west
  neighbour only
- The shared token on the Contract B upgrade
- Annex assembly in the sidecar: hash the parent blobs, build the annex, strip the blobs
- The genome cache, keyed by hash, and the fetch answer for the archive
- Journal-recovery hardening, and admission control across three slots

### WP4 — The mod: containment, capture and the parent blobs

**Depends on:** WP1.
**Needs the game:** yes.

- Stop disabling `worldWrapping`. Snapshot the value and report it only (D10)
- Extend the export capture to the band outside the square, with the outward-velocity
  test (Risk 2)
- The passive entry edge on the west side (Risk 3)
- Parent collection on export: the entity IDs, and one serialized blob for each living
  parent (Risk 8)
- The cost measurement for the extra serializations

### WP5 — `multiverse-archive`

**Depends on:** WP1, WP2, WP3.
**Needs the game:** no.

- The envelope record, with both ring slots, both peer ids and the relay receive time
- The content-addressed genome store
- Fetch by hash, with the retry and the rate limit of Risk 7
- The lineage graph, built from the annexes
- A read-only query surface: one organism, its parents, and its ancestors
- The gap report, for a hash that no peer can serve

Decide first how the archive sees every envelope. A relay copy adds a rule to a
deliberately dumb relay (D1). A slot-less peer adds a member that owns no world. See
*Design Calls for the Owner*.

### WP6 — The LAN rig

**Depends on:** WP3, WP4.
**Needs the game:** yes.
**Status: built on 2026-08-03. The LAN exit test is the remaining step.**

- ✓ The artifact set for the second computer, and the script that starts it (Risk 6).
  `farend/`: `make-farend-bundle.sh`, `setup-farend.ps1`, the generated `start-slot2.ps1`
  and `stop-slot2.ps1`, and `README.md`
- ✓ The Windows sidecar binary, `bin/multiverse-sidecar.exe`, proven on this machine in a
  two-slot ring across the WSL boundary: ring form-up, a migration, and a slot reclaim
  after a restart
- ✓ The firewall rule and the port forward for the relay port, printed by
  `e2e/run-m3-lan.sh lanhost`. **The relay host address stays `TODO-owner`**
- ✓ The third slot: a second sidecar and a second game instance on the main machine
- ✓ The far-end drive path. Risk 6 holds the answer: there is no drive path, and
  `e2e/run-m3-lan.sh` verifies slot 2 from the archive
- ✓ Ring pre-seeding, so the LAN ring forms in any start order. `bin/relay
  --reserve-slot <peerId>` writes the reservations before any peer connects
- ✓ An update to `dev_environment.md`

### WP7 — The exit test

**Depends on:** WP2, WP3, WP4, WP5, WP6.
**Needs the game:** yes.

Build the test on `e2e/run-m2.sh`. The phases, the command file and the forced export all
carry over. A forced export still teleports the organism into the strip with an outward
velocity, so every guard on the ordinary path runs.

## Exit Test

The test uses three seeded worlds, `M3-Slot1`, `M3-Slot2` and `M3-Slot3`. All three worlds
use the same `SimulationSize`. Slot 1 and slot 3 run on the main machine. Slot 2 runs on
the second computer.

### The second-computer procedure

Do these ten steps one time, before Part 1. `e2e/run-m3-lan.sh` runs the test itself.

On the main machine:

1. Stop all games. Then run `farend/make-farend-bundle.sh`.
2. Run `e2e/run-m3-lan.sh reserve`. This holds slot 2 for the second computer.
3. Run `e2e/run-m3-lan.sh lanhost`. Write down the LAN address of this machine.
4. In an elevated PowerShell, add the firewall rule and the port forward from step 3.
5. Copy `farend/dist/farend-bundle.zip` and `~/.multiverse-token` to the second computer.
6. Run `e2e/run-m3-lan.sh up`.

On the second computer:

7. Unpack the zip into one folder. Put the token beside it as `token.txt`.
8. Run `.\setup-farend.ps1 -RelayHost <the address from step 3> -TokenFile .\token.txt`.
9. Run `.\start-slot2.ps1`. Wait for the line `RING SLOT GRANTED`.

Back on the main machine:

10. Run `e2e/run-m3-lan.sh phase1`. The ring is complete when phase 1 passes.

After the test, run `e2e/run-m3-lan.sh down` here. The other operator runs
`.\stop-slot2.ps1` there. Neither command touches the other machine.

### Part 1 — The circuit

1. Start the relay and the archive on the main machine.
2. Start the two local sidecars, then the remote sidecar over the LAN.
3. Confirm that the relay grants three slots, in ring order.
4. Start all three game instances.
5. Wait for all three mods to report an open export edge.
6. Select one organism in slot 1. Prefer an organism with two living parents.
7. Record its payload, its entity ID and both parent entity IDs.
8. Push the organism across the east edge of slot 1.
9. Wait for the arrival in slot 2, on the second computer.
10. Record the payload in slot 2, and compare it against the payload from slot 1.
11. Push the organism across the east edge of slot 2.
12. Wait for the arrival in slot 3, on the main machine.
13. Push the organism across the east edge of slot 3.
14. Wait for the arrival back in slot 1.
15. Record the payload in slot 1 a second time.
16. Query the archive for the entity ID of the organism.
17. Save all three worlds. Drive `CreateSave` by hand.

The test passes when all of these conditions hold:

- The organism arrives in each of slot 2, slot 3 and slot 1 exactly one time.
- The payload after each hop equals the payload before that hop. Only the eight position
  numbers and the tick counters differ.
- The entity ID is equal at every hop.
- The organism never returns to the slot it came from. Every hop moves east.
- The archive holds three envelope records for the organism, one for each hop.
- The archive holds a lineage annex on each record, with two parent entries.
- Each parent hash in the archive equals the hash of the genome of that parent.
- The archive holds the genome behind each parent hash, after a fetch or from the export.
- All three world saves complete with no exception.
- No BepInEx log and no sidecar log holds an error.

### Part 2 — Containment

Run the two tests of decision D10. Risk 1 and Risk 2 hold the pass conditions.

Then run a population count. Record the population of each world at the start and after
two simulated hours at 20x speed, with no forced export.

The test passes when the total population across the three worlds accounts for every
organism. Births and deaths explain the change. No world leaks organisms outward.

### Part 3 — The kill gauntlet, across the LAN

Run each kill separately. Restore the full rig between kills. Count the copies of the
organism by entity ID, in all three worlds.

Kill these three targets, one at a time:

- The relay, on the main machine
- The remote sidecar, on the second computer
- The remote game instance, on the second computer

The test passes when the organism exists **exactly once** after every recovery. Zero copies
is also a pass, because decision D2 accepts loss. Two copies is a failure.

Record the count for each kill. A pass with zero copies must name the process that lost the
organism.

Kill 2 and kill 3 happen on the second computer, and Risk 6 keeps that machine
operator-driven. Its operator stops the process by hand and starts it again with
`.\start-slot2.ps1`. Record both kills as manual steps. The count after each recovery
comes from the main machine, in the same way as phase 6 of `e2e/run-m3-lan.sh`.

## Deliverables

- An amended `contracts/contract-a.md` and `contracts/contract-b-m2.md`, from WP1
- A `bb8-schema` with the genome projection, the hash and the deep validation
- A `multiverse-relay` with ring insertion and a held slot
- A `multiverse-sidecar` with annex assembly, the genome cache and the fetch answer
- A `multiverse-archive` binary, with its store and its query surface
- A mod build with wrap containment, the outside band and parent collection
- ✓ The artifact set and the start script for the second computer (`farend/`)
- ✓ An update to `dev_environment.md` with the LAN rig, the bundle workflow, the firewall
  rule and the port forward
- The recorded results of the two D10 tests
- The automated M3 exit test, with its recorded result
- An `m3_findings.md` with the research results of the milestone

## Design Calls for the Owner

Four calls need an answer before WP1 closes.

1. **The parent-blob route** (Risk 8). The mod sends opaque parent blobs, and the sidecar
   hashes them. This route keeps decision D4 intact and gives the archive a source for
   every parent genome. It costs up to two extra serializations for each export.
2. **How the archive sees every envelope** (WP5). A relay copy is simple and adds a rule to
   a relay that decision D1 keeps dumb. A slot-less peer keeps the relay dumb and adds a
   ring member with no world.
3. **The self hash in the annex.** The envelope carries the genome of the migrant already,
   so the archive can hash it. An explicit `genomeHash` costs one field and makes the join
   key deterministic at both ends.
4. **The far end of the rig** (Risk 6). **Answered on 2026-08-03.** Neither option was
   necessary. The far end is set up one time and then observed only. The archive, the
   relay's ring state and the `EDGE_STATUS` ripple carry every fact to the main machine.
   The one manual step left is Part 3, kill 3.

## Next Steps

1. Answer the four calls in *Design Calls for the Owner*.
2. Write WP1. Amend both contracts, and bump the major version.
3. Run the two D10 tests as early as possible. They gate the containment design.
4. Start WP2. The genome projection blocks the annex and the archive.
5. Start WP3 and WP4 in parallel, from the amended contracts.
6. Build WP5 after the archive route of call 2 is settled.
7. ✓ Build WP6. The bundle, the LAN rig and the two network commands are ready. The owner
   installs the second computer and opens the port, with the ten-step procedure above.
8. Run WP7. Record the results in `m3_findings.md` and in this document.
