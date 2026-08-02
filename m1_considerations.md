# M1 Considerations — In-Game Round-Trip Spike

This report expands milestone M1 of `system_decomposition.md`.

## Purpose

M1 is a spike: one game instance, no sidecar, no network. The spike moves one live
organism through a full round trip: serialize, destroy, respawn, overwrite. The goal
is to remove the largest project risks first. All later milestones depend on this
result. If M1 fails, the network design does not matter.

## Scope

In scope:

- Serialization of one live organism with the game's own Newtonsoft code
- Capture of the live state: health, energy, stomach, maturity, age, position, velocity
- Destruction of the organism with `UnityEngine.Object.Destroy()`
- Respawn through the native spawn path in `SimulationScripts`
- State overwrite in a postfix hook (the Constance-Mod technique)
- Re-index of the entity ID
- A dev command in the mod that starts the round trip

Out of scope:

- Border detection and the hysteresis zone (M2)
- Void-avoidance suppression (M2)
- The sidecar, Contract A, and all network code (M2)
- Thread-safe queues (M1 runs fully on the main thread; M2 adds the queue)
- Corpses, pellets, and eggs (M5)

## Risks That M1 Must Remove

**Risk 1 — The spawn API.** The research does not name the native spawn function. We
must find it in `BibitesAssembly.dll` with a decompiler. The spawn path can have side
effects. For example, the game can randomize the newborn organism. A postfix hook
must overwrite the organism after these effects. This function is the largest unknown
in the project.

**Risk 2 — Read access to the live state.** The Constance-Mod proves write access to
the live attributes: health, energy, stomach, maturity. It does not prove read access
on a live organism. M1 must find the read path for each attribute. If the fields are
not public, reflection is the method.

**Risk 3 — Destroy safety.** `UnityEngine.Object.Destroy()` removes the organism
object from the scene. Other systems can hold references to this object, for example
predator targets or the selection UI. A stale reference causes an engine error. M1
must prove that the destruction of one organism is safe while the simulation runs.

**Risk 4 — Fidelity of the payload.** The static `.bb8` schema is known from
community tools. The full live payload is unknown. Field names can differ between
game versions. M1 must record the exact field names and the source game version.
This record becomes the first input for `bb8-schema` (decision D4).

**Risk 5 — The entity ID re-index.** The game gives each active entity an internal
integer ID. A respawned organism gets a new ID from the registry. M1 must find where
the registry lives and how the game assigns the IDs. ID collisions cause memory
access violations (research §6.5).

## Prerequisites

- The game at a pinned version, v0.6.3 (Corpses and Seasons), with automatic updates off
- BepInEx, installed in the game directory
- dnSpy (or an equal decompiler) and read access to `BibitesAssembly.dll`
- A C# plugin project with `<HintPath>` references to the Managed DLLs

## Exit Test

The test uses one adult organism with a complex brain and food in the stomach.

1. Serialize the organism. Write the payload to a log file.
2. Run the round-trip command on the organism.
3. Serialize the organism again. Compare the two payloads.
4. Watch the organism for one minute of simulation time.

The test passes when:

- The two payloads are equal. Only the entity ID can differ.
- The organism continues its normal behavior: movement, food search, no health loss.
- The BepInEx console shows no errors.

## Fallbacks

If a clean hook on the spawn function is not possible, two fallbacks exist:

- **The import path.** The game has an internal import routine for `.bb8` files. The
  mod can call this routine with a temporary file.
- **The egg path.** The Constance-Mod intercepts the newborn at the hatch point. The
  mod can spawn an egg and overwrite the organism at hatch time. This path is slower,
  but the hook point is proven.

## Deliverables

- A mod build with the round-trip dev command
- A findings document with the class names, the method signatures, the field paths,
  and the true payload shape
- An updated risk table for M2

## Next Steps

1. Disable automatic updates for the game in Steam.
2. Install BepInEx in the game directory.
3. Decompile `BibitesAssembly.dll` with dnSpy.
4. Find the spawn functions and the serialization entry points. Record each signature
   in the findings document.
5. Create the C# plugin project with the `<HintPath>` references.
6. Write a hotkey command that serializes the selected organism to a log file.
7. Add the destroy step, the respawn step, and the overwrite step to the command.
8. Run the exit test.
