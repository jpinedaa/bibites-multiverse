# M1 Considerations — In-Game Round-Trip Spike

This report expands milestone M1 of `system_decomposition.md`.

## Purpose

M1 is a spike: one game instance, no sidecar, no network. The spike moves one live
organism through a full round trip: serialize, destroy, respawn, re-link. The goal
is to remove the largest project risks first. All later milestones depend on this
result. If M1 fails, the network design does not matter.

A research pass on the decompiled game source removed three of the five initial risks.
`m1_findings.md` holds that research. It is the source for the API names in this
document.

## Scope

In scope:

- Serialization of one live organism with the game's own Newtonsoft code
- Capture of the live state: health, energy, stomach, maturity, age, position, velocity
- Destruction of the organism with `UnityEngine.Object.Destroy()`
- Respawn with `SaveSystem.LoadBibiteOrEggFromData`
- Repair of the parent links and the child links after the respawn
- A dev command in the mod that starts the round trip

Out of scope:

- Border detection and the hysteresis zone (M2)
- Void-avoidance suppression (M2)
- The sidecar, Contract A, and all network code (M2)
- Thread-safe queues (M1 runs fully on the main thread; M2 adds the queue)
- Corpses, pellets, and eggs (M5 as written; **M7** after the renumberings of D9 and D16)

## Risks

### Resolved by the research pass

**Risk 1 — The spawn API. RESOLVED.** The spawn function is
`SaveSystem.LoadBibiteOrEggFromData`. The function is public. No other code in the
assembly calls it. The restore branch applies no mutation and no gene randomization.
It keeps the entity ID, the position, the velocity, the age and the neuron
activations. A postfix hook to undo randomization is not necessary. See
`m1_findings.md` §1.

**Risk 2 — Read access to the live state. RESOLVED.** Every attribute that M1 needs
is public, with one exception: the `InternalClock` fields are `internal`. The public
static method `SerializationHelper.DeserializeObject` reads and writes those fields.
The mod needs no reflection of its own. See `m1_findings.md` §3.

**Risk 4 — Fidelity of the payload. RESOLVED, and the assumption was wrong.** An
on-disk `.bb8` file holds the full live runtime state. `SaveSystem.SaveBibite` writes
the position, the velocity, the health, the energy, the stomach, the size, the age and
the neuron activations. Only the template reader of the user interface
(`Utility/BibiteTemplate.cs`) discards this data on import. A `.bb8template` file holds
the template alone. `bb8-schema` must separate the two shapes with the `body` key
(decision D4). See `m1_findings.md` §2.

### Open at the start of M1

Each entry carries its status after the exit test of 2026-08-02.

**Risk 3 — Destroy safety.** A direct `UnityEngine.Object.Destroy(body.gameObject)`
call makes no corpse, no meat pile and no eggs. `BibiteBody.OnDestroy` spawns nothing
and cleans all registries. The shipped user interface uses the same call in
`SelectionPanel.DeleteSelection`. Do not call `Die()`. That path makes a corpse, or
meat when corpses are off. Remove the body from `BibiteTracker.instance.bibites`
before the destroy call. This step keeps the death counters and the death-age
statistics clean. Unity defers the destruction to the end of the frame. The old
instance and the new instance therefore exist together for a short time. Do not
assume that the old object is gone in the same frame. See `m1_findings.md` §5.

Status after the exit test: the round trip ran this path two times with no error and no
Unity exception.

**Risk 5 — Stale cross-organism references.** `BibiteID.id` is a random 32-bit
integer. The game has no ID counter and no ID-to-entity registry. Reuse of the
original ID on respawn is the correct behavior. The real risk is a different one. Two
reference sets survive the destruction: the parent's `BibiteEggLayingOrgan.children`
and the child's `BibiteGenes.parent1` and `parent2`. A stale reference makes
`BibiteEggLayingOrgan.SaveState` throw an exception on the next world save. That
failure looks like a corrupted save file, not like a defect in the mod. Re-link both
sides after the respawn. Mirror the repair code at `SaveSystem.cs:748-782`. See
`m1_findings.md` §4.3.

Status after the exit test: the world save completed with no exception, so the round trip
left no new stale reference. The test organism had no parents and no children, so the
repair code itself is still untested. See *Carried to M2*.

**Risk 6 — Hidden load errors.** `LoadBibiteOrEggFromData` catches every exception
and returns null. A bad payload gives no diagnostic message. Call the private
`LoadBibite` through reflection during development to see the real error. Ship the mod
on the public wrapper. See `m1_findings.md` §1.2.

The save side hides errors in the same way. `SaveSystem.SaveGame` starts `CreateSave` as a
coroutine, and Unity swallows the exception. A Risk-5 failure is therefore silent. When
the save result is the thing under test, drive `CreateSave` directly.

Status after the exit test: the respawn returned a valid organism in both runs. The two
hazards stay in the game API.

## Prerequisites

- The installed game. Steam can update the game at any time. `sync-game-refs.sh` updates
  the reference DLLs and the decompiled source to the installed version
- BepInEx, installed in the game directory
- The decompiled game source in `decompiled/BibitesAssembly/`. `sync-game-refs.sh`
  makes it with `ilspycmd`
- A C# plugin project with `<HintPath>` references to the Managed DLLs

## Exit Test

The test uses one adult organism with a complex brain and food in the stomach. The
organism must not be latched to the mouth of another organism.

1. Start the game. Read the logged `Application.version` value in the BepInEx console.
2. Serialize the organism. Write the payload to a log file.
3. Run the round-trip command on the organism.
4. Serialize the organism again. Compare the two payloads.
5. Watch the organism for 30 simulated seconds.
6. Save the world from the game menu.

The test passes when:

- The logged game version is `0.6.3.1`.
- The two payloads are equal. The entity ID is equal too. Only the tick counters and
  `clock.ticProgress` can differ, and only by the ticks between the two captures.
- The organism continues its normal behavior: movement, food search, no health loss.
- The world save completes with no error. This result proves that no stale
  cross-organism reference remains.
- The BepInEx console shows no errors.

### Result — PASS, 2026-08-02

The exit test passed on 2026-08-02. The mod ran the test two times with no operator.
`bibites-mod/src/AutoTest.cs` holds the harness. The BepInEx config entry `[M1] AutoTest`
and the environment variable `MULTIVERSE_AUTOTEST=1` both start it.

- **Run 1 — PASS.** The harness seeded a new world in the same run. Two payload paths
  differed: `$.transform.scale` and `$.body.health`. Both differences are float artifacts.
  The round trip lost no live state.
- **Run 2 — PASS.** The harness loaded that world from its save file. The two payloads
  were **equal, token for token**.
- The entity ID survived the destroy and the respawn in both runs. In run 2 the ID was
  `-843827577`.
- The plugin logged `Application.version = 0.6.3.1`. This value satisfies the version
  condition.
- The organism stayed alive and healthy for 30 simulated seconds.
- The world save completed with no exception. The run showed no Unity error.

`m1_findings.md` gives the full runtime results, the cause of each float artifact, and the
API hazards that the harness work exposed.

## Fallbacks

If the restore path fails, one fallback exists:

- **The egg path.** Spawn an egg and let it hatch. Patch `EggHatching.Hatch()` with a
  Harmony postfix. Overwrite `__instance.newBornBody` with
  `SerializationHelper.DeserializeObject`. This path is worse than the restore path:
  `StartBody()` resets the health, the size and the clock, and `CopyBrain` zeroes the
  neuron activations. Keep this path as a fallback only.

## Deliverables

All three deliverables are complete.

- **Done.** A mod build with the round-trip dev command. The build also carries the
  unattended exit-test harness `AutoTest.cs`.
- **Done.** `m1_findings.md`, with the runtime results of the exit test added.
- **Done.** An updated risk table for M2. See *Carried to M2*.

## Carried to M2

M1 gives three items to the two-sim milestone. Item 3 already has an answer.

1. **The family re-link is untested.** The seeded world spawned adults with no parents and
   no children. `OrganismLinks` therefore ran against an organism with no links, and the
   Risk-5 trap never fired. Run the round trip in an evolved world. Select an organism
   with parents and children that are alive. Then save the world again.
2. **Void-avoidance suppression.** M1 kept this item out of scope. Organisms avoid the map
   edge, so migration almost never starts without the suppression (decision D3).
3. **The two-instance rig — RESOLVED on 2026-08-02.** Two instances of the game run at the
   same time on this machine. Both processes persist. The first instance locks
   `BepInEx/LogOutput.log`. BepInEx gives the second instance `LogOutput.log.1` instead, so
   no log is lost. `game.sh log` reads the first file only. Both instances load the same
   plugin DLL and the same BepInEx config, so per-instance settings need environment
   variables or a second game copy.

## Next Steps

All seven steps are complete. This list stays as the record of the M1 procedure.

1. Write the dev command. Follow the order of operations in `m1_findings.md`.
2. Serialize the organism with `SaveSystem.SaveBibite` to a temporary file for the
   first run. Switch to the in-memory `SerializeBibite` path after the test passes.
3. Remove the body from `BibiteTracker.instance.bibites`. Then destroy the GameObject.
4. Respawn with
   `SaveSystem.instance.LoadBibiteOrEggFromData(json, true, null, null)`. Null-check
   the result.
5. Re-link the parent references and the child references after the respawn.
6. Deploy the build with `deploy.sh`. Start the game with `game.sh start`.
7. Run the exit test. Record the results in `m1_findings.md`.
