# M2 Considerations — Two Sims, One Machine

This report expands milestone M2 of `system_decomposition.md`.

**Status: COMPLETE.** The exit test passed on 2026-08-02/03. See *Exit Test*, *Result*.

## Purpose

M2 is the first vertical slice with a network. It runs two game instances, two sidecars
and one relay on one machine. The milestone proves the full custody chain end to end.
One organism leaves sim A, arrives in sim B, and comes back.

M2 also proves the failure behaviour. A process kill during a migration must never make
two copies of one organism.

M1 removed the in-game risks. `m1_findings.md` holds those results. `m2_findings.md`
holds the research on void avoidance, world geometry, edge behaviour and entry
positioning. `contracts/contract-a.md` holds the wire specification. Those three
documents are the source for every API name and every message field below.

## Scope

In scope:

- Two game instances on one machine, each with the mod
- Two `multiverse-sidecar` processes, one for each game instance
- One `multiverse-relay` process with frame forwarding and a two-sector registry
- Contract A on localhost, in full
- The complete custody chain of decision D2, including the migration journal
- A border strip on one designated edge for each sim
- Measurement of the natural crossing rate at that edge
- A low-fertility corridor zone, added only if the measured rate is near zero
- Suppression of void avoidance in a world that enables the setting
- An override for world wrapping while an edge is open
- Per-instance mod configuration through environment variables

Out of scope:

- NAT traversal and hole punching (M4)
- libp2p and direct peer-to-peer transport (M4)
- The `species-catalog` module (M5)
- Corpse, pellet and egg payloads (M5)
- A hosted relay and unknown peers (M3)
- The synthetic pheromone beacon (M3, and only after a measured baseline)
- More than two sectors, and more than one open edge per sim

**Five of those milestone numbers predate the renumberings**, and the items themselves are
unchanged. D9 (2026-08-03) moved public exposure out of M3 into a milestone of its own and
D16 (2026-08-05) shifted everything behind it, so read: a hosted relay and unknown peers
**M5**, NAT traversal and libp2p **M6**, `species-catalog` and the corpse, pellet and egg
payloads **M7**. The beacon is not one of them — it stayed parked where it was.
`system_decomposition.md` is the current numbering; this list is the M2 record.

## Risks

### Risk 1 — The crossing rate. The stated premise was wrong. CLOSED.

Decision D3 said that the vanilla void-avoidance AI keeps the crossing rate near zero.
`m2_findings.md` disproves that mechanism.

Void avoidance is one torque blend in `BibitePropulsion.UpdateOrgan`. A single global
static gates it. The setting `ScenarioSettings.voidAvoidance` has `DefaultValue = false`.
No shipped scenario turns it on. The `Apocalypse` scenario sets it to `false` explicitly.

WP2 confirmed this in the game on 2026-08-02. A loaded world reported
`voidAvoidance=False`. The mod wrote no patch and logged the reason.

The real wall is food density. Pellets spawn only inside a `Zone`. The `3 Islands`
scenario separates its islands with a fertility ratio of about 3000 to 1. Organisms stay
on the islands because the space between them holds no food.

Suppression therefore removes a force that is already absent. Suppression alone will not
raise the crossing rate.

**M2 measured first.** WP4 opened one edge in each sim and counted the strip entries. The
measurement ran for 30 real minutes at 20x speed, with no forcing.

| Sector | Open edge | Strip entries for each simulated hour | Population |
|---|---|---|---|
| A | E | 20.5 | ~26 |
| B | W | 24.4 | ~26 |

Crossing is frequent once an edge is open. **The corridor lure is canceled.** Build the
lure only after the ecology changes and a new measurement returns a rate near zero.

The researched lure stays on record. It is a low-fertility `ZoneSettings` from the nearest
island to the open edge, added with the public method
`ScenarioSettings.Instance.AddNewZone`. The precedent is the `Void` zone of the shipped
`3 Islands` scenario. Gate the corridor behind a mod configuration flag. Remove the
corridor when the world unloads, because a zone is persisted world data. Keep the
pheromone beacon in M3.

Keep the suppression code anyway. It is two small Harmony patches. A player world that
enables *Void-No-Mo'* still needs it. Read `voidAvoidance.val` at world load first. Write
no patch when the flag is already off.

Always change a game setting with `Setting.SetValue`. A direct write to `val` fires no
event. The cached static then stays stale.

### Risk 2 — The parent transform of the payload. CLOSED.

The payload key `transform` holds `transform.localPosition`. The key `rb2d` holds the
world position. The two agree only when the parent transform is the identity. The parent
is `WorldObjectsSpawner.Instance.bibiteHolder`. That object is Unity scene data, so the
decompiled source cannot settle the question.

WP2 settled it at runtime on 2026-08-02. The mod logs the holder transform one time for
each world load. The holder is at `position=(0, 0, −0.01)`. Its rotation is zero. Its
scale is one. The transform is therefore the identity in `x` and in `y`. The payload key
`transform` is world space in the two coordinates that a migration writes. See
`m2_findings.md` §4.3 and open uncertainty 1.

Keep the re-assert. Always re-assert `transform.position` and `rb2d.position` after a
restore. Do the re-assert in the same frame as the restore. Also re-assert the velocity
and both rotations. The `Rigidbody2D` overwrites a transform-only correction on the next
tick. That second caveat stays open, and it is the reason for the re-assert.

### Risk 3 — `SimulationSize` is live-mutable.

`ScenarioIndependentSettings.Instance.SimulationSize` is an ordinary `FloatSetting`. Ten
game subsystems subscribe to it. A mid-run change is therefore possible.

The border strip is defined against that value. A cached copy silently moves the strip.
Two paired sims then disagree about the length of their shared edge.

Subscribe to the setting. Never snapshot it. Send a `CONFIG_UPDATE` from the subscription
callback. Report the current value in every `HEARTBEAT` as well. Refuse to open an edge
against a peer with a different value.

### Risk 4 — World wrapping is on by default.

`ScenarioSettings.worldWrapping` has `DefaultValue = true`. An organism that passes
`1.5 · S + 1000` teleports to the opposite point of the map. Velocity and rotation
survive the teleport.

The mod owns the border strip, so a missed capture sends the organism outward. The
teleport that follows looks like a duplication bug caused by the mod.

Disable world wrapping while an edge is open. Use
`ScenarioSettings.Instance.worldWrapping.SetValue(false)`. Snapshot the previous value
first. Restore that value when the world unloads.

**M3 reverses this instruction.** Decision D10 keeps `worldWrapping` ON. The mod only
reads the value and reports it. See *Carried to M3*.

Note the sibling setting. `shadeAvoidance` and `worldWrapping` sit on one `if/else if`
chain. With shade avoidance on, the wrap branch never runs.

### Risk 5 — The family re-link code was unexercised. CLOSED.

M1 implemented the parent and child repair. The test organism had no parents and no
children. The stale-reference trap therefore never fired.

A stale `BibiteID` in a parent's `eggLayer.children` makes
`BibiteEggLayingOrgan.SaveState` throw on the next world save. That failure looks like a
corrupted save file.

WP2 wrote the repair code and the two counters. WP4 exercised both of them in the game on
2026-08-02/03.

The exit test migrated a child out of the world of its parent. **The trap fired**, and the
mod cleared it: `cleared 1 stale children entries`. A later hop landed the parent in the
world of its own child, and the `MIGRATE_IN_ACK` reported `relinkedParents=1`. Both worlds
then saved with no exception.

Migrate the child before the parent. `BibiteGenes.SaveState` writes the parent identifier
only while the parent GameObject lives. See *Carried to M3*.

Drive the world save by hand. `SaveSystem.SaveGame` wraps `CreateSave` in a coroutine and
Unity swallows the exception. Reflect on the private `CreateSave` iterator and call
`MoveNext()` inside a `try` block.

### Risk 6 — A process kill must never duplicate an organism. CLOSED.

Decision D2 accepts rare loss. It does not accept duplication.

Three kills threaten the invariant. A relay kill loses frames in flight. A sidecar kill
loses everything that was not flushed to the journal. A game kill rolls the world back to
its last autosave.

The game kill is the hardest case. The rolled-back world can hold an organism that the
sidecar already exported. Two copies then exist.

`contracts/contract-a.md` §7.4 defines the answer. The mod mints a new `sessionId` for
every world load. The sidecar recognises the new value. The sidecar then re-sends an
unsolicited `MIGRATE_OUT_ACK` for every organism it still owns. Each one carries the
`entityId`. The mod finds the local copy by that ID and destroys it.

Two more rules protect the invariant:

- The sidecar writes the journal entry and flushes it to disk **before** it sends
  `MIGRATE_OUT_ACK`.
- The mod keeps one organism bound to one `migrationId`. It re-sends the identical
  message after a reconnect. It never mints a second identifier for the same organism.

The kill gauntlet ran all three kills on 2026-08-02/03. The organism existed exactly once
after every recovery. See *Exit Test*, *Result*.

## Work Packages

Four packages. All four are complete. WP1 and WP2 depend only on
`contracts/contract-a.md`. They ran in parallel, and their owners did not talk.

The parallel build worked, and it also proved a cost. Each side resolved the same
ambiguities alone, and the two answers had to be reconciled afterwards. `contract-a.md`
§13 holds the ten resolutions. `contract-b-m2.md` §8 holds three more.

### WP1 — The Go side. DONE, 2026-08-02.

**Depends on:** `contracts/contract-a.md`.
**Needs the game:** no.
**Delivered in:** `go/`, commit `3546f15`.

- [x] A `bb8-schema` skeleton (`internal/bb8`). It validates the size cap and the
      top-level object shape. It hashes the payload. It reads the entity ID, the heading
      and the version key out of the blob. The `Hook` variable is the seam for deep
      validation.
- [ ] Gene, node and synapse validation. Dialect detection. The `Utility.Version` alpha
      quirk in the version order. **These four move to M3.** The M2 delivery path does not
      need them. A skeleton that claims to validate a gene object is worse than one that
      does not claim it.
- [x] `multiverse-relay` (`internal/relay`). It forwards Contract B frames. It holds a
      two-sector registry. It never parses a bb8 body.
- [x] `multiverse-sidecar` (`internal/sidecar`). It serves Contract A. It owns the
      journal, the route from exit edge to peer, the admission control and the bounce-back
      path.
- [x] A durable journal (`internal/journal`). Every mutating call flushes before it
      returns. The tests cover a torn tail, compaction and a `kill -9` between the flush
      and the ACK.
- [x] Integration tests with fake mod clients. They cover the happy path, a reconnection
      with replay, a session rollback with custody reassertion, a relay drop, a heartbeat
      timeout, every close code, and the main NACK codes.

Contract A and Contract B both run with no game installed. A fake mod client is a
WebSocket client and a canned payload string.

### WP2 — The mod side. DONE, 2026-08-02.

**Depends on:** `contracts/contract-a.md`.
**Needs the game:** yes.
**Delivered in:** `bibites-mod/src/`, commit `7b5e4d9`.

- [x] Border-strip detection, on a Harmony postfix on `BibiteBody.FixedUpdate`
      (`BorderPatches.cs`). It re-applies every guard of the method's early returns.
- [x] The outward-velocity test (`MultiverseClient.cs`, `BorderGeometry.OutwardNormal`).
- [x] Export with the full custody flow (`MigrationExporter.cs`). The organism goes inert
      on send. It is destroyed only on `MIGRATE_OUT_ACK`.
- [x] Import (`MigrationImporter.cs`). It rewrites the eight position numbers, restores,
      re-asserts in the same frame, repairs the family links, and replies.
- [x] The entry-immunity window, keyed on the entity ID.
- [x] A thread-safe inbound queue (`WebSocketTransport.cs`). Control frames drain in
      `Update`. Organism work runs in `FixedUpdate`. See `contract-a.md` §13, A4.
- [x] Configuration from environment variables (`MultiverseConfig.cs`).
- [x] The settings work of Risks 1, 3 and 4 (`WorldSettings.cs`), and the runtime readings
      of Risk 2.
- [x] The crossing-rate counter (`CrossingStats.cs`). It prints one `[M2-CROSSING]` line
      for each simulated minute.

A world load, a Contract A handshake and an `EDGE_STATUS` all ran in the game on
2026-08-02. Two paths have code but no game evidence. WP4 owns them.

Both game instances share one plugin DLL and one BepInEx configuration directory. Per
instance settings therefore need environment variables. The WSL to Windows hop needs
`WSLENV` to name each variable. See `dev_environment.md`.

### WP3 — The two-instance rig. DONE, 2026-08-02.

**Depends on:** WP2's environment-variable names.
**Needs the game:** yes.
**Delivered in:** `bibites-mod/game.sh` and `e2e/run-m2.sh`, commit `9742cb7`.

- [x] `game.sh <command> <instance>` for `start`, `stop`, `status`, `log`, `logfile`,
      `pid` and `wait`. The old single-instance forms still work.
- [x] Per-instance PID tracking. `start` records the identifier that
      `Start-Process -PassThru` returns, so `stop <instance>` hits one game only.
      `stop all` stops every recorded instance, then sweeps the orphans by process name.
- [x] Log resolution by content. Each instance greps both `LogOutput.log` and
      `LogOutput.log.1` for its own startup marker. BepInEx assigns those two files by
      lock order, which is a race, not a rule.
- [x] `e2e/run-m2.sh` drives the whole rig: `build`, `seed`, `up`, the phases, `down`.

Copy `LogOutput.log` before a restart. BepInEx overwrites it on every launch.

Stop the game before every deploy. Windows holds the plugin DLL open.

### WP4 — The end-to-end exit test. DONE, 2026-08-02/03.

**Depends on:** WP1, WP2 and WP3.
**Delivered in:** `e2e/run-m2.sh`, commit `9742cb7`.

The test runs with no operator, and each phase is separately invokable on a healthy rig.
The rig steers each game through the command file that `MULTIVERSE_CMD_FILE` names
(`bibites-mod/src/DevCommands.cs`). A forced export only teleports the organism into the
strip with an outward velocity. The ordinary `MIGRATE_OUT` path and every guard on it then
run, so the test bypasses nothing.

**Two paths had code but no game evidence. WP4 exercised both.** Contract tests with a
fake mod prove the Go half. They cannot prove the C# half, because the fake mod never
touches Unity.

1. **The export path.** [x] An organism left a real sim, went inert, and the mod destroyed
   it on `MIGRATE_OUT_ACK`.
2. **The family re-link on a linked organism.** [x] Both counters reported non-zero
   repairs. See Risk 5.

Neither item needed new production code. The test did find one defect in the mod's
transport. See *Exit Test*, *Result*.

## Exit Test

The test runs with no operator. It uses two seeded worlds, `M2-SectorA` and
`M2-SectorB`. Both worlds use the same `SimulationSize`. Sim A opens its east edge. Sim B
opens its west edge.

### Part 1 — The round trip

1. Start the relay, then both sidecars, then both game instances.
2. Wait for both mods to report an open edge.
3. Select one organism in sim A. Prefer an organism with living parents and living
   children.
4. Record its payload and its entity ID.
5. Push the organism across the east edge. Place the organism in the strip with an
   outward velocity.
6. Wait for the arrival in sim B.
7. Record the payload in sim B. Compare it against the payload from sim A.
8. Push the organism back across the west edge of sim B.
9. Wait for the arrival in sim A.
10. Record the payload in sim A a second time. Compare it against the first payload.
11. Save both worlds. Drive `CreateSave` by hand.

The test passes when all of these conditions hold:

- The organism arrives in sim B one time.
- The organism arrives back in sim A one time.
- The payload after each hop equals the payload before that hop. Only the eight position
  numbers and the tick counters differ.
- The entity ID is equal at every hop.
- The `MIGRATE_IN_ACK` messages report a non-zero `relinkedParents` or
  `relinkedChildren`.
- Both world saves complete with no exception.
- Neither BepInEx log holds an error.

### Part 2 — The kill test

Run each kill separately. Restore the full rig between kills.

For each kill:

1. Start a migration.
2. Send `kill -9` to the target process during the migration.
3. Restart that process.
4. Wait for the rig to recover.
5. Count the copies of the organism. Use the entity ID. Search both worlds.

Kill these three targets, one at a time:

- The relay
- One sidecar
- One game instance

The test passes when the organism exists **exactly once** across both worlds after every
recovery. Zero copies is also a pass, because decision D2 accepts loss. Two copies is a
failure.

Record the count for each kill. A pass with zero copies must name the process that lost
the organism.

### Result — PASS, 2026-08-02/03

`e2e/run-m2.sh` ran every phase with no operator, against game `0.6.3.1`.

- **Part 1, the round trip.** One organism went A→B→A. A `sha256` comparison found the
  payloads byte-equal in both directions. The entity identifier stayed equal at every hop.
- **Part 1, the family re-link.** The Risk 5 trap fired, and the mod cleared it. A parent
  landed in the world of its own child and reported `relinkedParents=1`. Both worlds saved
  with no exception.
- **Part 2, the kill gauntlet.** Three kills ran, one at a time: `kill -9` on the relay,
  `kill -9` on sidecar B after the journal flush, and a force-stop of game B. The game
  came back from its autosave holding a resurrected copy, and the custody assertion
  destroyed that copy.

| Count point | Copies of the organism |
|---|---|
| After the A→B→A round trip | 1 |
| After the relay `kill -9` | 1 |
| After the sidecar `kill -9` | 1 |
| After the game force-stop and the autosave rollback | 1 |

The organism existed **exactly once** at every count point — four for four. No count point
reported zero copies or two copies. The final journal held 512 migrations on each side,
with no stuck entry.

**One defect, found by this test and fixed in `9742cb7`.** The mod's WebSocket transport
leaked its send loop across a reconnect. `Task.WhenAny` left the loser of the race alive,
and the send loop parks on a semaphore that every connection shares. After any disconnect
the leaked loop stole the first frame of the next session. The sidecar then closed the
connection with code 4003, and the mod never completed a second handshake. That failure
silently voided all crash recovery, and Risk 6 depends on that recovery. The fix is a
per-session cancellation token plus a transport `Generation` counter on every inbound
event. **Neither per-side test suite catches this defect.** Only a real restart under a
live game exposes it.

## Deliverables

- [x] `contracts/contract-a.md` — the wire specification. Amended on 2026-08-02 with the
      ten resolutions of §13.
- [x] `contracts/contract-b-m2.md` — the sidecar-to-relay subset, with three amendments.
- [x] A `bb8-schema` skeleton with unit tests. Deep validation moves to M3. See WP1.
- [x] A `multiverse-relay` binary with a two-sector registry
- [x] A `multiverse-sidecar` binary with a durable journal and the Contract A server
- [x] Sidecar integration tests that run with no game
- [x] A mod build with border detection, export, import and the custody flow
- [x] An update to `m2_findings.md` with the `bibiteHolder` transform result
- [x] A two-instance `game.sh`, and the `e2e/run-m2.sh` orchestration
- [x] The automated M2 exit test, with its recorded result. See *Exit Test*, *Result*.
- [x] The measured crossing rate at an open edge. The corridor zone is canceled, so no
      second measurement exists. See Risk 1.

## Carried to M3

- ~~**Guard all four edges.**~~ **Superseded by decision D10 on 2026-08-03.** M2 disables
  `worldWrapping` while an edge is open (Risk 4). Organisms then escape through the three
  unguarded edges. The rig saw one at `y=7186` with `S=2000`. An M2 world therefore leaks
  population. M3 guards no extra edge. M3 keeps `worldWrapping` ON, and the vanilla wrap
  contains each edge that no strip guards. The mod only reads the setting and reports it.
  The export capture band also extends outside the square. Each export needs an outward
  velocity, because a wrapped organism travels inward. See `system_decomposition.md` D10,
  `m3_considerations.md` Risks 1 and 2, and `contracts/contract-a.md` §4.3.1 and §14 A13.
- **Migration order is child first, then parent.** `BibiteGenes.SaveState` drops the
  parentage of a child once the parent GameObject is gone. A child that migrates after its
  parent therefore arrives with no link back. The behaviour matches vanilla, so this is a
  fidelity limit, not a defect. M3 owns the answer.
- **Contract debt A5.** `MIGRATE_OUT_NACK` carries no `edge` field. One edge for each sim
  makes the field redundant in M2. A multi-edge sim needs it. See
  `contracts/contract-a.md` §13 A5 and §12 open item 6.
- **Deep bb8 validation.** Gene, node and synapse validation, dialect detection, and the
  `Utility.Version` alpha quirk. See WP1.
- **The pheromone beacon.** Held in M3 from the start. The measured crossing rate of
  Risk 1 removes the reason to build it. See `m2_findings.md` §1.5, option 5.

## Next Steps

M2 is closed. Every step below is complete.

1. ~~Log the `bibiteHolder` transform.~~ **Done.** The transform is the identity in `x`
   and in `y`. See `m2_findings.md` §4.3.
2. ~~Log the five boundary settings and `SimulationSize` at world load.~~ **Done.** See
   the runtime table at the top of `m2_findings.md`.
3. ~~Start WP1 and WP2 in parallel.~~ **Done.** Both are complete.
4. ~~Build WP3.~~ **Done.** `game.sh` drives two instances, and `e2e/run-m2.sh` drives the
   rig.
5. ~~Measure the natural crossing rate in a default world.~~ **Done.** 20.5 and 24.4 strip
   entries for each simulated hour. See Risk 1.
6. ~~Add the corridor zone if step 5 returns a rate near zero.~~ **Canceled.** The rate is
   not near zero.
7. ~~Build WP4. Exercise the export path and the family re-link. Then run the exit test.~~
   **Done.** The exit test passed on 2026-08-02/03.
8. ~~Record the results.~~ **Done.** See *Exit Test*, *Result*, `m2_findings.md` open
   uncertainty 2, and the M2 entry of `system_decomposition.md`.

Start M3 from *Carried to M3*.
