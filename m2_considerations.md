# M2 Considerations — Two Sims, One Machine

This report expands milestone M2 of `system_decomposition.md`.

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

## Risks

### Risk 1 — The crossing rate. The stated premise was wrong.

Decision D3 said that the vanilla void-avoidance AI keeps the crossing rate near zero.
`m2_findings.md` disproves that mechanism.

Void avoidance is one torque blend in `BibitePropulsion.UpdateOrgan`. A single global
static gates it. The setting `ScenarioSettings.voidAvoidance` has `DefaultValue = false`.
No shipped scenario turns it on. The `Apocalypse` scenario sets it to `false` explicitly.

The real wall is food density. Pellets spawn only inside a `Zone`. The `3 Islands`
scenario separates its islands with a fertility ratio of about 3000 to 1. Organisms stay
on the islands because the space between them holds no food.

Suppression therefore removes a force that is already absent. Suppression alone will not
raise the crossing rate.

**M2 must measure first.** Open one edge. Count how many organisms enter the border strip
for each simulated hour. Record that number. If the number is near zero, add the lure.

The researched lure is a corridor zone. Add a low-fertility `ZoneSettings` from the
nearest island to the open edge. Use the public method
`ScenarioSettings.Instance.AddNewZone`. The precedent is the `Void` zone of the shipped
`3 Islands` scenario. Gate the corridor behind a mod configuration flag. Remove the
corridor when the world unloads, because a zone is persisted world data.

This work moves the lure from M3 into M2. Keep the pheromone beacon in M3.

Keep the suppression code anyway. It is two small Harmony patches. A player world that
enables *Void-No-Mo'* still needs it. Read `voidAvoidance.val` at world load first. Write
no patch when the flag is already off.

Always change a game setting with `Setting.SetValue`. A direct write to `val` fires no
event. The cached static then stays stale.

### Risk 2 — The parent transform of the payload is unproven.

The payload key `transform` holds `transform.localPosition`. The key `rb2d` holds the
world position. The two agree only when the parent transform is the identity. The parent
is `WorldObjectsSpawner.Instance.bibiteHolder`. That object is Unity scene data, so the
decompiled source cannot settle the question.

The M1 round trip does not settle it either. Both sides of that test used
`localPosition`.

Do two things. Log the position, the rotation and the scale of
`bibiteHolder.transform` one time at startup. Then update `m2_findings.md` with the
result. Always re-assert both `transform.position` and `rb2d.position` after a restore.
Do the re-assert in the same frame as the restore. Also re-assert the velocity and both
rotations. The `Rigidbody2D` overwrites a transform-only correction on the next tick.

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

Note the sibling setting. `shadeAvoidance` and `worldWrapping` sit on one `if/else if`
chain. With shade avoidance on, the wrap branch never runs.

### Risk 5 — The family re-link code is still unexercised.

M1 implemented the parent and child repair. The test organism had no parents and no
children. The stale-reference trap therefore never fired.

A stale `BibiteID` in a parent's `eggLayer.children` makes
`BibiteEggLayingOrgan.SaveState` throw on the next world save. That failure looks like a
corrupted save file.

Run one migration in an evolved world. Select an organism with living parents and living
children. Migrate it. Then save both worlds. Report the repaired counts in the
`MIGRATE_IN_ACK` fields `relinkedParents` and `relinkedChildren`. A zero in both fields
means the test did not exercise the code.

Drive the world save by hand. `SaveSystem.SaveGame` wraps `CreateSave` in a coroutine and
Unity swallows the exception. Reflect on the private `CreateSave` iterator and call
`MoveNext()` inside a `try` block.

### Risk 6 — A process kill must never duplicate an organism.

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

## Work Packages

Four packages. WP1 and WP2 depend only on `contracts/contract-a.md`. They run in
parallel, and their owners do not need to talk.

### WP1 — The Go side

**Depends on:** `contracts/contract-a.md`.
**Needs the game:** no.

- A `bb8-schema` skeleton. It validates the top-level keys, the gene object, the node
  and synapse arrays, and the size caps. It detects the two dialects with the `body` key.
  It reproduces the `Utility.Version` alpha quirk in its version ordering.
- `multiverse-relay`. It forwards Contract B frames. It holds a two-sector registry. It
  never parses a bb8 body.
- `multiverse-sidecar`. It serves Contract A. It owns the migration journal, the routing
  from exit edge to peer, the admission control and the bounce-back path.
- A durable journal with a crash-recovery test. Flush the entry before the ACK.
- Integration tests with fake mod clients. The tests cover the happy path, every NACK
  code, a reconnection with replay, and a session rollback with custody reassertion.

Finish WP1 with no game installed. A fake mod client is a WebSocket client and a
canned payload string.

### WP2 — The mod side

**Depends on:** `contracts/contract-a.md`.
**Needs the game:** yes.

- Border-strip detection. Anchor it on a Harmony postfix on `BibiteBody.FixedUpdate`.
  Re-apply every guard that the early returns of the method imply: `Time.timeScale > 0`,
  `born`, `!dead`, `!destroyed`, and "not already in flight".
- The outward-velocity test. Trigger only on a positive dot product with the outward
  normal.
- Export. Reuse the M1 serialization machinery. Add the custody flow of Contract A. Make
  the organism inert on send. Destroy it only on `MIGRATE_OUT_ACK`.
- Import. Rewrite the eight position numbers in the payload. Restore with
  `LoadBibiteOrEggFromData`. Re-assert the position, the velocity and the rotation in the
  same frame. Repair the parent and child links. Then reply.
- The entry-immunity window, keyed on the entity ID.
- A thread-safe inbound queue. Drain it on the Unity main thread only.
- Configuration from environment variables: the sector, the sidecar port, the open edge,
  the strip width and the corridor-zone flag.
- The settings work of Risks 1, 3 and 4.

Both game instances share one plugin DLL and one BepInEx configuration directory. Per
instance settings therefore need environment variables. The WSL to Windows hop needs
`WSLENV` to name each variable. See `dev_environment.md`.

### WP3 — The two-instance rig

**Depends on:** WP2's environment-variable names.
**Needs the game:** yes.

Extend `bibites-mod/game.sh` for two instances:

- `game.sh start <instance>` starts one instance with its own environment variables.
- `game.sh log <instance> [n]` reads the log of that instance. BepInEx gives the first
  instance `LogOutput.log` and the second instance `LogOutput.log.1`, because the first
  holds a lock on the first file.
- `game.sh stop <instance>` stops one instance. The current command stops both, because
  it matches on the process name. Match on the process identifier instead.
- `game.sh status` lists both instances with their identifiers.

Copy `LogOutput.log` before a restart. BepInEx overwrites it on every launch.

Stop the game before every deploy. Windows holds the plugin DLL open.

### WP4 — The end-to-end exit test

**Depends on:** WP1, WP2 and WP3.

Build the automated exit test of the next section. Follow the pattern of
`bibites-mod/src/AutoTest.cs`. Arm it with an environment variable. Print one result
line. Then quit.

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
5. Push the organism across the east edge. Use the corridor zone, or place the organism
   in the strip with an outward velocity.
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

## Deliverables

- `contracts/contract-a.md` — the wire specification. **Done.**
- A `bb8-schema` skeleton with unit tests
- A `multiverse-relay` binary with a two-sector registry
- A `multiverse-sidecar` binary with a durable journal and the Contract A server
- Sidecar integration tests that run with no game
- A mod build with border detection, export, import and the custody flow
- A two-instance `game.sh`
- The automated M2 exit test, with its recorded result
- The measured crossing rate, with and without the corridor zone
- An update to `m2_findings.md` with the `bibiteHolder` transform result

## Next Steps

1. Log the `bibiteHolder` transform. Update `m2_findings.md`. This is one line of code
   and it closes the largest open uncertainty of `m2_findings.md`.
2. Log the five boundary settings and `SimulationSize` at world load.
3. Measure the natural crossing rate in a default world. Report organisms for each
   simulated hour.
4. Start WP1 and WP2 in parallel.
5. Add the corridor zone if step 3 returns a rate near zero.
6. Build WP3.
7. Build WP4 and run the exit test.
8. Record the results in `m2_findings.md`. Update the status of this document.
