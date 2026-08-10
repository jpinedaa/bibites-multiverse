# M2 Findings — Void Avoidance, World Geometry, Edge Behaviour, Entry Positioning

Source: `decompiled/BibitesAssembly/` (ilspycmd output of
`The Bibites_Data/Managed/BibitesAssembly.dll`, Steam app 2736860, buildid 22383127,
game version `0.6.3.1`). All paths are repo-relative. Line numbers refer to the
decompiled files as they exist in this checkout; they change when the game updates and
`sync-game-refs.sh` regenerates the decompile.

This document answers the five M2 research questions raised against decision **D3**
(`system_decomposition.md:44`) and the M2 milestone entry of `system_decomposition.md`.
It does **not** repeat `m1_findings.md`, which already documents `SaveSystem`,
`BibiteBody`, `BibiteTracker`, `GameManager`, `TimeController` and the organism round
trip. Where M2 depends on an M1 fact, this document cites the M1 section instead of
re-deriving it.

Every claim here was originally read off the decompiled source, plus four shipped scenario
files read off disk. Items that could not be settled statically were marked **UNCERTAIN**.

**Runtime confirmations, 2026-08-02** (WP2, game `0.6.3.1`). The mod now logs the
boundary settings and the holder transform once per world load, so these are measured, not
inferred:

| Reading | Value | Settles |
|---|---|---|
| `bibiteHolder.transform` | `position=(0,0,−0.01)`, `rotation=(0,0,0)`, `lossyScale=(1,1,1)` | Open uncertainty 1 — §4.3 caveat 1. Identity in `x`/`y`; the payload's `transform` key is world space where it matters |
| `SimulationSize` | `2000.00` | §2.1 |
| `voidAvoidance` | `False` | §1.4 — the shipped default holds in a real world; there is nothing to suppress |
| `voidAvoidanceDistance` | `100.0` | §1.1 |
| `shadeOutsideOfBounds` / `shadeAvoidance` | `True` / `False` | §1.3 |
| `worldWrapping` | `True` | §3 — on by default, as read; the mod disables it while an edge is open, which costs population through the three unguarded edges (§3, and M3) |
| `corpsesEnabled` | `True` | §5 |

**Measured 2026-08-02/03** (WP4, the two-instance exit-test rig): open uncertainty 2 — the
crossing rate — is **resolved**, and no uncertainty blocks the milestone any more. An open
edge draws **20.5** (sector A, east) and **24.4** (sector B, west) border-strip entries per
simulated hour at a population of ~26. See *Open uncertainties*, item 2.

## Headline result

Automatic void avoidance is **one `if` block inside one method** —
`BibitePropulsion.UpdateOrgan()` (`SimulationScripts.BibiteScripts/BibitePropulsion.cs:209-229`)
— gated by a single `private static bool`. It is a steering-torque blend, not a clamp,
not a neural input, and not per-organism state. **It ships disabled**: the setting's
`DefaultValue` is `false` (`SettingScripts/ScenarioSettings.cs:294-301`), and none of the
four shipped scenarios inspected on disk turn it on (`Apocalypse` sets it explicitly to
`false`).

That reframes D3. The premise "vanilla AI avoids borders, so the mod must suppress
void avoidance" is **only conditionally true** — it is true for a world where the player
enabled *Void-No-Mo'*, and false for a default world. In a default world the reason
organisms do not reach the map edge is **ecological, not behavioural**: pellets spawn
only inside `Zone`s (`SimulationScripts/Zone.cs:251-273`), so there is nothing outside the
islands worth swimming to. **Suppression is cheap and should still be implemented, but on
its own it will not raise the crossing rate** — see *Recommended M2 approach*, item (a).
The inference drawn from that in the first draft, *"M2 needs a lure as well"*, did **not**
survive measurement: an open edge draws ~20–25 strip entries per simulated hour with no
lure at all (*Open uncertainties*, item 2), so the corridor zone was never built.

| Question | Short answer |
|---|---|
| 1. Mechanism | Steering-torque blend in `BibitePropulsion.UpdateOrgan`, gated on a global static; no neural input, no clamp |
| 2. Geometry | `ScenarioIndependentSettings.Instance.SimulationSize.val` = half-extent of a square; islands are `ZoneManager.instance.zones` (circles/rects) |
| 3. Edge today | Nothing at the square edge. Beyond `1.5·simSize + 1000` the organism is **teleported to the antipode** (world wrapping, ON by default). No damage, no death, no clamp |
| 4. Entry position | Rewrite `transform.position[0..1]`, `rb2d.px/py/vx/vy/r` in the `JObject` **and** re-assert both `transform.position` and `rb2d.position` after restore |
| 5. Pellets/corpses | One class `MatterPellet` for plant *and* meat; corpses are ordinary `BibiteBody` GameObjects with `dead == true` |

---

## 1. VOID AVOIDANCE

### 1.1 The mechanism — a torque blend inside the propulsion organ

`decompiled/BibitesAssembly/SimulationScripts.BibiteScripts/BibitePropulsion.cs:209-229`,
inside `public override void UpdateOrgan()` (`:185`):

```csharp
if (avoidVoid)
{
    Zone zone = null;
    Vector2 vector3 = float.PositiveInfinity * Vector2.one;
    foreach (Zone zone2 in ZoneManager.instance.zones)          // :213
    {
        Vector2 vector4 = zone2.pos - (Vector2)base.transform.position;
        if (vector4.magnitude - zone2.radius < vector3.magnitude)
        {
            vector3 = vector4 - zone2.radius * vector4.normalized;
            zone = zone2;
        }
    }
    vector3 = zone.pos - (Vector2)base.transform.position;      // :222
    if (vector3.magnitude > zone.radius)                        // :223
    {
        float a = (vector3.magnitude - zone.radius) / ScenarioSettings.Instance.voidAvoidanceDistance.val;
        a = Mathf.Min(a, 1f);                                   // :226
        num2 = num2 * (1f - a) + (float)Math.Tanh(Vector2.SignedAngle(vector2, vector3)) * a;  // :227
    }
}
```

`num2` is the turn command, initialised from the brain at `:201`
(`num2 = desireToTurn`, i.e. `brain.Output(NEATBrain.Outputs.Rotate)`, `:93`). `vector2`
is `base.transform.up`, the organism's heading (`:192`). So the block **lerps the brain's
own rotation output towards "turn to face the nearest zone centre"**, with blend weight
`a` ramping linearly from `0` at the zone's edge to `1` at `voidAvoidanceDistance`
(default `100 u`) beyond it. At `a == 1` the brain's steering output is completely
overridden.

`num2` is then clamped to `[-1, 1]`, scaled by `movementPenalty`, and applied as
`rb2d.AddTorque(num2 * muscleTurnStrength)` (`:246, :253`). Forward thrust (`num`) is
**not** touched by the block — an organism under full void avoidance still accelerates
in whatever direction its brain wants, it just cannot steer away from the island.

Consequences that matter for M2:

- It is **not** a hard clamp and **not** a positional constraint. An organism with
  `Accelerate` pinned high and enough speed will still leave; it simply cannot *steer*
  outward.
- It is **not** a neural input. `NEATBrain.Inputs` (`NEATBrain.cs:13-48`) has 33 members
  and **none** of them relates to zones, the void, the shade or the world edge. The
  organism has no sensory awareness of the boundary; the boundary is applied to it.
- It is inactive whenever the organism is inside *any* zone: the selection loop picks the
  zone with the smallest edge distance (which is negative inside a zone), and `:223`
  then tests `distance-to-centre > radius`.
- **Rect zones are invisible to it.** `Zone.radius` is assigned only in the non-rect
  branch of `Zone.UpdateSize` (`SimulationScripts/Zone.cs:400-412`, `radius =
  settings.absoluteRadius;` at `:402`). For `SpawnDistribution.Rect` zones
  (`ZoneSettings.isRect`, `SettingScripts/ZoneSettings.cs:301`) `radius` stays at its
  default `0f`, so a rectangular island is treated as a zero-radius point. **UNCERTAIN**
  whether this is intentional; either way, a rect zone cannot be used to shelter a
  border strip from void avoidance.
- **NRE hazard.** If `ZoneManager.instance.zones` is empty, the loop never runs, `zone`
  stays `null`, and `:222` dereferences it. Vanilla reaches this only in a zoneless
  world with the setting on; a mod that clears zones must not leave `avoidVoid` true.

### 1.2 What gates it — one global static, not per-organism state

`BibitePropulsion.cs:63`

```csharp
private static bool avoidVoid = ScenarioSettings.Instance.voidAvoidance.SubscribeTo<BoolSetting, bool>(UpdateVoidAvoidance);
```

`:112-115`

```csharp
private static void UpdateVoidAvoidance(bool val) { avoidVoid = val; }
```

- **Scope: global.** `private static` on `BibitePropulsion`. There is no per-organism
  flag, no gene, and no per-instance override anywhere in the assembly. Every live
  organism reads the same bool on the same tick.
- **Backing setting:** `ScenarioSettings.Instance.voidAvoidance`
  (`SettingScripts/ScenarioSettings.cs:294-301`), a `BoolSetting` named *"Void-No-Mo'"*,
  `WikiLink = "Automatic_Void_Avoidance"`, **`DefaultValue = false, val = false`**.
  `ScenarioSettings.Instance` is a plain `public static` field on a non-MonoBehaviour
  class (`ScenarioSettings.cs:9-11`), so it is reachable from mod code with no lookup.
- **Distance setting:** `ScenarioSettings.Instance.voidAvoidanceDistance`
  (`:303-315`), `FloatSetting`, default `100 u`, range `0.0001 … 1000`. It is read
  **live, every organ tick, off the setting object** (`BibitePropulsion.cs:225`) rather
  than from a cached static — unlike every other setting in the class. That makes it the
  one knob a mod can move without touching a static field.
- **How to change the gate correctly:** `Setting<T>.SetValue(T)`
  (`SettingScripts/Setting.cs:67`) assigns `val` *and* fires
  `OnValueChangeWithPrecedent` / `OnValueChange` / `OnChange`, which is what drives
  `UpdateVoidAvoidance`. Writing `voidAvoidance.val = false` directly
  (`Setting.cs:17`, the property is `public abstract … { get; set; }`) compiles but
  **fires no event, so the `avoidVoid` static stays stale**. Always use `SetValue`.

### 1.3 Two sibling boundary systems live in the same method

The void-avoidance block is immediately followed by the shade/wrap block,
`BibitePropulsion.cs:230-244`:

```csharp
if (shadeEnabled)
{
    Vector2 vector5 = base.transform.position;
    float magnitude = vector5.magnitude;
    float sqrMagnitude = vector5.sqrMagnitude;
    if (shadeAvoidance && sqrMagnitude >= OutOfBoundDist2)                       // :235
    {
        float num4 = Mathf.Clamp01(Mathf.InverseLerp(OutOfBoundDist2, OutOfBoundDist2 * 1.5f, sqrMagnitude));
        num2 = num2 * (1f - num4) + (float)Math.Tanh(Vector2.SignedAngle(vector2, -vector5)) * num4;
    }
    else if (worldWrapping && magnitude - body.bodyLength >= simSize * 1.5f + 1000f)   // :240
    {
        rb2d.position = -vector5.normalized * (simSize * 1.5f + 1000f - body.bodyLength * 0.95f);  // :242
    }
}
```

| System | Static | Setting | Declared | Default | Effect |
|---|---|---|---|---|---|
| Void avoidance | `avoidVoid` (`:63`) | `voidAvoidance` | `ScenarioSettings.cs:294` | **false** | torque blend toward nearest zone centre |
| Shade rendering + gate | `shadeEnabled` (`:75`, **public static**) | `shadeOutsideOfBounds` | `ScenarioSettings.cs:390-395` | **true** | gates the two below; also darkens the background (`OneUseScripts/BackgroundManager.cs:95-98`) |
| Shade avoidance | `shadeAvoidance` (`:73`) | `shadeAvoidance` | `ScenarioSettings.cs:397-404` | **false** | torque blend toward the world origin, from `r = 1.5·simSize` to `r ≈ 1.837·simSize` |
| World wrapping | `worldWrapping` (`:71`) | `worldWrapping` | `ScenarioSettings.cs:406-413` | **true** | hard teleport to the antipode at `r − bodyLength ≥ 1.5·simSize + 1000` |

`OutOfBoundDist2 = simSize * simSize * 2.25f` (`:77`, recomputed at `:150`) is the
*squared* radius `(1.5·simSize)²`. Note `shadeAvoidance` and `worldWrapping` are on an
`if/else if`: with both enabled, an organism past `1.5·simSize` gets the steering blend
and **never wraps**, because the `else if` is unreachable while `sqrMagnitude` is above
the threshold.

`shadeEnabled` at `:75` is **`public static`**, unlike the other three — the only one of
the four a mod can read and write without reflection.

### 1.4 The setting ships disabled — evidence from the shipped scenarios

Read directly off
`/mnt/c/Users/j_jor/AppData/LocalLow/The Bibites/The Bibites/Scenarios/*.zip`
(`settings.bb8settings`, UTF-8 BOM, Newtonsoft shape `{"Value": …}`):

| Scenario | `voidAvoidance` | `shadeOutsideOfBounds` | `shadeAvoidance` | `worldWrapping` |
|---|---|---|---|---|
| `Default` | key absent → class default `false` | absent → `true` | absent → `false` | absent → `true` |
| `3 Islands` | key absent → `false` | absent | absent | absent |
| `Deadly Tropics` | key absent → `false` | absent | absent | absent |
| `Apocalypse` | **`false` (explicit)** | **`true`** | **`false`** | **`true`** |

`Apocalypse` is the only one of the four that serialises the whole settings block, and it
confirms the class defaults. `Apocalypse` also carries `killZone: true` and
`killZoneRadiusFactor: 1.35` — **there is no `killZone` anywhere in the 0.6.3.1
assembly** (grep returns nothing). That setting was removed in some earlier version and
the scenario file is stale; the loader ignores unknown keys.

`3 Islands` is the island scenario the community means. Its zone list is:

| Zone | posX, posY (relative) | radius (relative) | fertility |
|---|---|---|---|
| Eastern Island | 0.75, 0.0 | 0.2 | 10.0 |
| Southern Island | −0.375, −0.6495 | 0.2 | 10.0 |
| Northern Island | −0.375, 0.6495 | 0.2 | 5.62 |
| **Void** | 0.0, 0.0 | **1.00000012** | **0.003** |

The "void" between islands is itself a `Zone` of radius `1.0 · simSize` with fertility
driven to near the minimum (`ZoneSettings.fertility.minValue = 0.001f`,
`SettingScripts/ZoneSettings.cs:57, 63`). Two consequences:

1. **In `3 Islands`, void avoidance would do almost nothing even if enabled** — the
   whole playable disc is inside the "Void" zone, so `:223` is false everywhere inside
   `r < simSize`. The steering blend only engages outside the square.
2. What actually keeps organisms on the islands there is the **fertility ratio**
   (10.0 vs 0.003, ~3000×), i.e. food density. This is emergent behaviour, not a coded
   avoidance rule.

**This is the single most important finding for D3.** The community's "organisms only
rarely cross voids" is an ecology result, not a steering result. Turning a steering flag
off will not, on its own, make organisms walk to the map edge.

### 1.5 Options for suppressing / bypassing it at ONE designated edge

Rated by invasiveness. "Code invasiveness" = how much of the game's control flow the mod
takes over. "World invasiveness" = how much of the user's persisted world data changes.

| # | Option | Code | World | Per-edge? | Verdict |
|---|---|---|---|---|---|
| 1 | **Assert-only**: read `ScenarioSettings.Instance.voidAvoidance.val` at world load; if `false`, do nothing | **none** | none | n/a | Do this first. In a default world it is the whole answer |
| 2 | **Global off**: `ScenarioSettings.Instance.voidAvoidance.SetValue(false)` | **very low** (1 line, public API) | medium — the flag is part of `ScenarioSettings` and is written into the world save | no, global | Good fallback for the M2 rig. Snapshot the old value and restore on unload |
| 3 | **Scoped static toggle** (recommended): Harmony `Prefix`+`Postfix` on `BibitePropulsion.UpdateOrgan()` — prefix saves and clears the `avoidVoid` static when the organism is inside the designated border strip, postfix restores it | low-medium (2 tiny patches, 1 private static via `AccessTools.StaticFieldRefAccess`, hot path at 40 Hz × N) | **none** | **yes** | The only option that is both per-edge and leaves no persisted trace |
| 4 | **Zone manipulation**: add a `ZoneSettings` whose circle covers the border strip, so `:223` is false there | low (public `ScenarioSettings.Instance.AddNewZone`, `:1185`) | **high** — the zone is persisted, appears in the Zones editor, renders a visible circle (`Zone.sr`), spawns pellets and counts biomass | yes | Not for suppression. But see item 5 — the *same* mechanism is the right lure |
| 5 | **Lure, not suppression**: give organisms a reason to approach the edge | see below | see below | yes | **Probably required regardless of 1–4** |

Notes on option 3. The patched method is `public override void UpdateOrgan()` declared on
`BibitePropulsion` (`:185`) — patch the concrete declaring type, not the
`BibiteOrgan.UpdateOrgan` abstract (`BibiteOrgan.cs:27`). Inside the patch,
`__instance.transform.position` is reachable (MonoBehaviour, public); `__instance.body`
is **not** — `BibiteOrgan.body` is `protected` (`BibiteOrgan.cs:7`) and needs
`AccessTools` if the patch wants the `BibiteBody`. For a pure position test it does not.

Notes on option 4's limits, if it is ever attempted: `Zone.UpdatePosition`
(`Zone.cs:434-452`) clamps a zone's centre to `±(simulationSize − radius)`, so a large
zone cannot be pushed out to the edge — it gets pulled inward. `ZoneSettings.posX/posY`
are themselves clamped to `[−1, 1]` with `canGoOutOfBounds = false`
(`ZoneSettings.cs:117-140`). Fertility and biomass density bottom out at `0.001`
(`ZoneSettings.cs:50, 63`), never exactly zero, so an "invisible" zone still leaks
pellets slowly.

Notes on option 5 (the lure). Two shapes, both with precedent in the codebase:

- **Corridor zone.** A low-fertility zone reaching from an island to the open edge, i.e.
  exactly what `3 Islands` does with its "Void" zone but shaped as a path. Pure world
  data, no Harmony; `ScenarioSettings.Instance.AddNewZone(ZoneSettings)` (`:1185`) fires
  `onZoneAdded` → `ZoneManager.ZoneAdded` (`ZoneManager.cs:99-103`) →
  `WorldObjectsSpawner.GenerateNewZone` (`WorldObjectsSpawner.cs:333-338`), all public.
  It also solves option 4 for free, since the strip ends up inside a zone.
- **Synthetic pheromone gradient.** The game already does this for the Red Death event:
  `Pherosense.PherosenseAround()` (`SimulationScripts.BibiteScripts/Pherosense.cs:69`)
  injects a *fake* red-pheromone source at `:114-125` whose direction points away from
  the danger circle, so that evolved organisms which already respond to red pheromones
  flee it without any new neural input. **The same trick with the sign flipped is a
  ready-made "swim toward the open edge" beacon**, using a channel organisms are already
  wired for. Cost: a Harmony postfix on `PherosenseAround`, writing the `internal`
  sense fields — noticeably more invasive than a corridor zone, and it changes what
  evolution optimises for. Park this as the M3 escalation if the corridor zone is not
  enough (the M3 milestone entry of `system_decomposition.md` already flags migration-rate
  tuning as the M3 revisit point).

---

## 2. WORLD GEOMETRY

### 2.1 The three radii

Everything spatial derives from one float:
`ScenarioIndependentSettings.Instance.SimulationSize.val`
(`SettingScripts/ScenarioIndependentSettings.cs:14, 16-24`) — a `FloatSetting`,
default `2000`, range `100 … 20000`, `canGoOutOfBounds = true`, with named landmarks
from "Smallest" (1000) to "Enormous" (10000) and beyond (`:28-40`).

Helper text (`:19`): *"From the center, the simulation expends in every cardinal
direction by this length."* So the nominal playable area is the **square**
`[−S, +S] × [−S, +S]`, confirmed by the editor gizmo
`Gizmos.DrawWireCube(Vector3.zero, new Vector3(2f*val, 2f*val, 1f))`
(`SimulationScripts/ZoneManager.cs:198-206`) and by
`ScenarioIndependentSettings.simArea => 4f * S * S` (`:230`).

Everything *beyond* the square is measured as a **radius**, not a square:

| Boundary | Value | Where |
|---|---|---|
| Playable square half-extent | `S` | `ScenarioIndependentSettings.cs:16` |
| Shade start (`shadeStart`) | `1.5 · S` | `OneUseScripts/BackgroundManager.cs:63`; the `1.5f` is also `public const float ShadeStartFactor` at `:59` |
| Shade fade length | `1000` (constant) | `BackgroundManager.cs:67` |
| Shade end (`shadeEnd`) | `1.5 · S + 1000` | `BackgroundManager.cs:65` |
| Shade-avoidance ramp | `1.5·S` → `1.5·S·√1.5 ≈ 1.837·S` | `BibitePropulsion.cs:77, 235-239` (the lerp is on *squared* magnitude) |
| Wrap teleport trigger | `r − bodyLength ≥ 1.5·S + 1000` | `BibitePropulsion.cs:240` |
| Pellet cull radius | `r > 1.5·S + 500` | `ZoneManager.cs:216, 232` |
| Red Death safe radius | `shadeEnd · (1 − t)`, shrinking | `SimulationScripts.Events/RedDeathBloomManager.cs:37, 181` |

**Design note for the border strip.** The "map edge" a player perceives is the shaded
square at `|x| = S` or `|y| = S`, because that is where the background stops being lit.
Put the M2 border strip there — e.g. `x ≥ S − W` for an east edge with `W` on the order
of a few body lengths — not at `1.5·S`, which is deep in the shade and outside every
zone.

### 2.2 Islands and zones at runtime

There is no `Island` type in the assembly (grep for "island" returns nothing in code —
only in the scenario file's zone *names*). **A zone is an island.**

| Concept | Runtime object | Settings object |
|---|---|---|
| Manager | `SimulationScripts/ZoneManager.cs:16`, `public static ZoneManager instance` (`:22`, set in `Awake` `:47-50`); `public List<Zone> zones` (`:18`) | `ScenarioSettings.Instance.allZones` (`List<ZoneSettings>`, `ScenarioSettings.cs:1041`) |
| One island | `SimulationScripts/Zone.cs:14` `public class Zone : MonoBehaviour` | `SettingScripts/ZoneSettings.cs:12` `public class ZoneSettings : ISaveable` |

`Zone`'s public geometry surface:

- `public Vector2 pos => base.transform.position;` (`Zone.cs:83`) — world position.
- `public float radius;` (`Zone.cs:63`) — set in `UpdateSize` (`:385-415`) as
  `settings.absoluteRadius` for round zones; **left at `0f` for rect zones** (§1.1).
- `public ZoneSettings settings;` (`Zone.cs:19`).
- `public List<MatterPellet> zonePellets` (`:79`), `public List<GameObject> pellets`
  (`:87`, the children of `pelletHolder`), `public float maxBiomass` / `pelletBiomass`
  / `freeBiomass` (`:69-71, 85`).

`ZoneSettings`' geometry, all normalised against `SimulationSize`:

- `posX`, `posY` — `FloatSetting`, range `[−1, +1]`, `canGoOutOfBounds = false`
  (`ZoneSettings.cs:117-140`). World centre is `S · (posX, posY)`
  (`Zone.UpdatePosition`, `Zone.cs:451`).
- `absoluteRadius` (`ZoneSettings.cs:365-375`), `absoluteWidth` (`:413-422`),
  `absoluteHeight` (`:389-398`) — each returns either the absolute setting or
  `relative · SimulationSize`, switched by `sizeScalesWithSim` (`:141`).
- `isRect` (`:301`), `isRing` (`:289`) — from `distribution`
  (`SettingScripts/SpawnDistribution.cs`).

Zone lifecycle is fully event-driven and public:
`ScenarioSettings.onZoneAdded` / `onZoneRemoved` (`ScenarioSettings.cs:1024, 1027`) are
`public static UnityEvent<ZoneSettings>`, wired in `ZoneManager.InitializeSpawner`
(`ZoneManager.cs:58-61`). Mutators: `AddNewZone` (`:1185`), `RemoveZone` (`:1195`),
`RemoveAllZones` (`:1205`), `SetZones` (`:1166`), `ClearZones` (`:1156`).

### 2.3 How a mod reads the playable extents

```csharp
float S = SettingScripts.ScenarioIndependentSettings.Instance.SimulationSize.val;
// playable square: x,y in [-S, +S]
// east-edge border strip, width W:  pos.x >= S - W
```

Both `Instance` and `val` are public; no reflection and no scene lookup. To stay correct
if the value changes, subscribe rather than cache:
`ScenarioIndependentSettings.Instance.SimulationSize.Subscribe(UnityAction<float>)`
(`SettingScripts/Setting.cs:28`), exactly as `ZoneManager.InitializeSpawner` does
(`ZoneManager.cs:62-63`).

### 2.4 Fixed at creation, or dynamic?

**Technically dynamic; in practice fixed per world.**

- It is an ordinary `FloatSetting`, so `SetValue` works at any time and ten subsystems
  subscribe to react (`ZoneManager.cs:62`, `Zone.cs:131`, `BibitePropulsion.cs:79`,
  `BackgroundManager.cs:77`, `CameraManager.cs:50-52`, `RedDeathBloomManager.cs:23-25`,
  `ColorKillerPanel.cs:62-64`, `SpeciesDistributionItem.cs:33-35`, `ZoneGroupHandle.cs:115`,
  `BiomassPreview.cs:22`).
- It is chosen in the scenario selector before the world starts
  (`UIScripts/ScenarioSelectorPanel.cs:154`, a `FloatSettingDropdown`) and restored from
  the save on load (`UIScripts/LoadGamePanel.cs:216` reads
  `settingsOfSave["SimulationSize"] ?? settingsOfSave["independents"]["SimulationSize"]`).
- **UNCERTAIN** whether the in-game settings UI exposes it mid-simulation. Nothing in the
  assembly blocks it, and the subscribers exist precisely so it can change. **The mod
  must therefore subscribe, not snapshot**, or a mid-run resize silently moves the border
  strip out from under it.

For M2 the two paired sims must agree on `S`, or the two edges are different lengths and
the mapping from exit coordinate to entry coordinate is ill-defined. **Recommend the mod
read `S` at handshake time and refuse to open an edge against a peer with a different
`S`** (or scale the transverse coordinate — but that breaks the "one continuous world"
illusion D3 is buying).

---

## 3. EDGE BEHAVIOUR TODAY

**At the playable square edge (`|x| = S` or `|y| = S`): absolutely nothing happens.**
No clamp, no damage, no death, no notification. Grep for `.Die(` finds eight call sites
and **none** is boundary-related:

| Call site | Cause |
|---|---|
| `BibiteBody.cs:409` | `Start()` re-applying a persisted `dying` flag |
| `BibiteBody.cs:620` | `float.IsNaN(health)` guard inside `FixedUpdate` |
| `BibiteBody.cs:754` | health reached zero (`Hurting`) |
| `SimulationScripts/ColorKiller.cs:174` | the player-placed killer tower |
| `GUIManager.cs:252`, `UserActions.cs:33`, `BibiteStatsPanel.cs:246`, `SelectionPanel.cs:288` | UI "kill" buttons |

What *does* happen further out, in order of radius, all inside
`BibitePropulsion.UpdateOrgan` unless noted:

1. **`r > 1.5·S`** — visual shade only, unless `shadeAvoidance` is on
   (default **off**), in which case the torque blend toward the origin begins
   (`:235-239`).
2. **`r − bodyLength ≥ 1.5·S + 1000`** — **world wrapping, ON by default**
   (`ScenarioSettings.cs:406-413`). `rb2d.position = -vector5.normalized * (simSize*1.5f
   + 1000f - body.bodyLength * 0.95f)` (`:242`): a hard teleport to the diametrically
   opposite point, keeping velocity and rotation. This is the only positional
   modification the game ever applies to a bibite from the boundary systems.
3. **Pellets** beyond `1.5·S + 500` are destroyed, but only when
   `ZoneManager.RefreshPelletBiomassCounter()` runs (`ZoneManager.cs:208-244`, culls at
   `:216` and `:232`). Callers: `ZoneManager.UpdateSimSize` (`:44`),
   `ManagementScripts/SaveController.cs:228` (every autosave), and
   `UIScripts/UserActions.cs:57`. Gated on `ZoneManager.shadeEnabled` (`:30`, public
   static). Matches the `shadeOutsideOfBounds` helper text, "pellets that stay in the
   shade for too long (checked every autosave) will be destroyed".
4. **Zones** are clamped to stay inside the square: `Zone.UpdatePosition` (`:434-452`)
   and `Zone.Move` (`:176-212`, which also reflects velocity off the wall).
5. **Red Death** (optional event, `ScenarioSettings.enableRedDeath`) applies escalating
   damage to any bibite outside `RedDeathBloomManager.safeRadius`
   (`RedDeathBloomManager.cs:210-245`, `item.Hurting(...)` at `:226`), driven by
   trigger enter/exit (`:247-273`). This is a shrinking circle event, not the map edge,
   and it is off unless the scenario enables it.

**Implications for M2's border detection — and one of them M3 has since superseded:**

- Because nothing happens at the square edge, the mod is free to define the edge
  wherever it likes and owns the entire behaviour there. There is no vanilla code to
  fight.
- **Corpses never wrap and never steer.** `BibiteBody.FixedUpdate` returns at `:585-589`
  (`if (dead) { CorpseUpdate(); return; }`) before `organs.ForEach(...UpdateOrgan())`
  at `:613-616`. A dead body drifting outward will sail past `1.5·S + 1000` and keep
  going forever. Relevant to M5's corpse migration — **M7** after the renumberings of D9
  and D16 — and a reason not to reuse the live-organism border logic for corpses.
- **World wrapping was treated as a latent M2 hazard.** If the mod's border strip ever
  fails to capture an organism (paused sim, peer down, `EDGE_STATUS` closed), the organism
  keeps going and eventually teleports to the antipode. That reads to a player as a
  duplication/teleport bug caused by the mod. M2 therefore called
  `ScenarioSettings.Instance.worldWrapping.SetValue(false)` while an edge was open, and
  restored it afterwards — one public call, same pattern as §1.5 option 2.
- **Disabling it traded the teleport for a leak — observed 2026-08-02/03.** With wrapping
  off and only *one* edge guarded by the mod, the other three edges guard nothing at all:
  nothing at the square edge stops an organism (see the top of this section), and past
  `1.5·S + 1000` there is now no wrap either. The M2 rig saw organisms at `y = 7186` with
  `S = 2000` — outward-bound, alive, and never coming back. **An M2 world therefore leaks
  population through its unguarded edges.** M2 accepted this.
- ~~**M3 must guard all four edges** — wrap selectively (closed edges only), clamp at the
  closed edges, or open all four.~~ **Superseded 2026-08-03 by decision D10.** M3 does not
  guard four edges: it stops disabling the wrap. `worldWrapping` stays **ON**, and the
  vanilla wrap becomes the containment mechanism for every edge no strip guards. The two
  radii do not compete — the strips capture from `S − W` outward and the wrap fires at
  `1.5·S + 1000` — and the export capture band now extends **outside** the square, with an
  outward-velocity test that a wrapped organism (travelling inward) cannot pass. The M2
  leak was self-inflicted by the `SetValue(false)` call above, not by the game. See
  `system_decomposition.md` D10, `m3_considerations.md` Risks 1 and 2 for the two in-game
  tests that gate it, and `contracts/contract-a.md` §4.3.1 and §14 A13 for the wire and
  behaviour rules.

---

## 4. POSITION / VELOCITY CONTROL

### 4.1 What the payload carries

`SerializationHelper.SerializePosition(JObject store, GameObject obj)`
(`ScriptHelpers/SerializationHelper.cs:195-203`) writes two keys, and
`SaveSystem.SerializeBibite` calls it first (see `m1_findings.md` §2.2):

```json
"transform": { "position": [x, y], "rotation": <deg>, "scale": <float> },
"rb2d":      { "px": …, "py": …, "vx": …, "vy": …, "r": <deg> }
```

- `transform` ← `new SerializedTransform(obj.transform)`
  (`ScriptHelpers/SerializedTransform.cs:20-26`): **`transform.localPosition`**,
  `localRotation.eulerAngles.z`, `localScale.x`.
- `rb2d` ← `SerializeRigidBody2D` (`SerializationHelper.cs:176-186`):
  `rb.position`, `rb.linearVelocity`, `rb.rotation`.

### 4.2 What the restore does with them

`SerializationHelper.DeserializePosition` (`:205-231`) is called from
`SaveSystem.LoadBibite` at `SaveSystem.cs:446` — **first**, before genes, body, clock and
brain (`m1_findings.md` §1.2). It:

1. `serializedTransform.DeserializeTransform(obj.transform)`
   (`ScriptHelpers/SerializedTransformExtensions.cs:7-12`) → writes
   `transform.localPosition`, `localRotation`, `localScale`.
2. If `store["rb2d"] != null`, `DeserializeRigidBody2D` (`:188-193`) → writes
   `rb.position`, `rb.linearVelocity`, `rb.rotation`.

Nothing later in the restore path moves the organism. `ResumeBody()`
(`BibiteBody.cs:460-487`) calls `Set2DSize(d2Size, initialSet: true)`, which touches
**`localScale` only** (`BibiteBody.cs:686-711`, the write is at `:694`).
`FinalizeBirth()` (`:488-…`) merely *reads* `transform.position` for shader uniforms
(`:494-495`).

### 4.3 Answer: yes, rewrite the JObject — with two caveats

**Rewriting the payload before calling `LoadBibiteOrEggFromData` is sufficient and is the
right primary technique.** Rewrite all six numbers:

```
$.transform.position[0], $.transform.position[1], $.transform.rotation
$.rb2d.px, $.rb2d.py, $.rb2d.vx, $.rb2d.vy, $.rb2d.r
```

Caveat 1 — **`transform` is `localPosition`, `rb2d` is world position.** The restored
GameObject is parented to `WorldObjectsSpawner.Instance.bibiteHolder`
(`WorldObjectsSpawner.cs:68`, `GenerateNewBibite` at `:138-141`). The two keys agree
only when `bibiteHolder`'s transform is identity in `x` and `y`.

**RESOLVED at runtime, 2026-08-02** (see *Open uncertainties*, item 1): the holder sits at
`position=(0, 0, −0.01)`, `rotation=(0,0,0)`, `lossyScale=(1,1,1)`. It is a depth offset
and nothing else, so `localPosition.x/y == position.x/y` and the two payload keys agree on
every number the migration path writes. No `InverseTransformPoint` conversion is needed.
Re-check this line after a game update — it is one log line per world load and it is the
only evidence, because `bibiteHolder` is a `[SerializeField] GameObject` set in the Unity
scene, which never appears in the decompiled C#.

Caveat 2 — **`Rigidbody2D` wins.** Writing only `transform.position` after the restore
is not enough: the next `FixedUpdate` re-synchronises the transform from the body, so a
transform-only correction snaps back. Any post-restore correction **must write both**:

```csharp
var rb = go.GetComponent<Rigidbody2D>();
go.transform.position = worldPos;    // immediate, for anything reading this frame
rb.position           = worldPos;    // authoritative for physics
rb.linearVelocity     = worldVel;
rb.rotation           = headingDeg;  // degrees, matches transform.localRotation.eulerAngles.z
go.transform.rotation = Quaternion.Euler(0f, 0f, headingDeg);
```

`BibiteBody.rb2d` is `private` (`BibiteBody.cs:19`), so the mod calls `GetComponent`
itself (`m1_findings.md` §3.1).

Recommended shape: **rewrite the JObject as the primary mechanism, and re-assert
position/velocity/rotation immediately after the restore as a belt-and-braces check**
(cheap, idempotent, and it removes the `bibiteHolder` uncertainty entirely). Do the
re-assert in the same frame as the restore, before the first `FixedUpdate` ticks the
organism.

### 4.4 Ping-pong: the entry position must not be inside the exit strip

A naive implementation places the organism exactly at the receiving sim's edge — which
is *inside* that sim's border strip, so the very next `BibiteBody.FixedUpdate` re-detects
it and migrates it straight back. Three independent guards, use at least two:

1. **Inset.** Place at the strip's inner face plus a margin, not at `±S`. For an east
   exit at `x = +S`, enter at `x = −S + W + margin` with heading and speed preserved.
2. **Outward-velocity test.** Only trigger on `Vector2.Dot(rb.linearVelocity,
   outwardNormal) > 0`. An organism that entered heading inward can never satisfy it.
3. **Per-ID immunity window.** Keyed on `BibiteBody.id.id` (preserved across the round
   trip, `m1_findings.md` §4.1), for a few simulated seconds after arrival. This also
   covers an organism that arrives, turns around, and leaves again legitimately — which
   should be allowed, just not within the same tick.

Preserve, do not mirror, the heading. Exiting east heading east must enter west still
heading east; that is the "one continuous world" illusion D3 is buying
(`system_decomposition.md:44`).

---

## 5. PELLETS AND CORPSES (M5 groundwork — **M7** after the renumberings of D9 and D16)

There is **one** pellet class for both food types: `SimulationScripts/MatterPellet.cs:12`
`public class MatterPellet : MonoBehaviour, ISaveable`, distinguished at runtime by
`public MatterMaterial material` (`:14`) compared against `MatterMaterialManager.Plant` /
`.Meat`, with mass/energy carried by a private `Matter` (`:16`; `Matter` documented in
`m1_findings.md` §2.3) and surfaced through `amount` / `energy` / `radius` / `mass`
(`:41-75`). Both come off the same two prefabs via
`WorldObjectsSpawner.SpawnPelletOfMatter` (`ManagementScripts/WorldObjectsSpawner.cs:234`)
and its `SpawnPlantPellet` / `SpawnMeatPellet` wrappers (`:257, :262`); the meat burst on
death is `SpawnMeatPile` (`:265`). Tracking is fourfold and partly redundant:
`WorldObjectsSpawner.allPellets` (`:77`) and `pelletsOfMaterial`
(`Dictionary<MatterMaterial, List<MatterPellet>>`, `:79`); per-zone `Zone.zonePellets`
(`Zone.cs:79`) plus the `Zone.pelletHolder` transform children (`Zone.cs:87`); the
unparented `WorldObjectsSpawner.freePelletHolder` (`:65`) for pellets not owned by a
zone (corpse spill, stomach spill, `PelletPlacer`); and the static counters
`PlantCounter.Count/.Biomass` and `MeatCounter` (`SimulationScripts/PlantCounter.cs:7-9`,
via the `IEntityCounter` interface). **Corpses have no class of their own** — a corpse is
the same `BibiteBody` GameObject with `dead == true` (`BibiteBody.cs:94-95`), removed
from `BibiteTracker.instance.bibites` by `Die()` (`BibiteBody.cs:778`) but still tagged
`"bibite"` and still a child of `bibiteHolder`, so `SaveSystem` serialises it through the
ordinary `SerializeBibite` path (`SaveSystem.cs:190-199`) and predators find it via
`FieldOfView.seenCorpses` (`FieldOfView.cs:20`, filled at `:266-269` from a physics
overlap, not from a registry). Its only distinct behaviour is `CorpseUpdate()`
(`BibiteBody.cs:628-635`), reached by the early return at `BibiteBody.cs:585-589`.
**For M5 — now M7 — this is good news**: a corpse is already covered by the M1 organism
round trip verbatim — same `SerializeBibite`, same `LoadBibiteOrEggFromData`, with
`dead: true` in the body payload — while pellets need a new, much smaller payload built on
`MatterPellet.SaveState()` plus a zone-assignment decision on arrival.

---

## Recommended M2 approach

### (a) Least-invasive void-avoidance suppression for one edge

**Three layers, applied in order. Stop as soon as the crossing rate is acceptable.**

**Layer 0 — measure before suppressing (zero invasiveness).**
At world load, log `ScenarioSettings.Instance.voidAvoidance.val`,
`.voidAvoidanceDistance.val`, `.shadeOutsideOfBounds.val`, `.shadeAvoidance.val`,
`.worldWrapping.val`, and `ScenarioIndependentSettings.Instance.SimulationSize.val`.
In a default world the first three are `false / 100 / true` and **there is nothing to
suppress**. Do not write a Harmony patch to disable a flag that is already off.

**Layer 1 — scoped suppression (option 3 of §1.5), only if the world has it on.**
Harmony `Prefix` + `Postfix` on
`SimulationScripts.BibiteScripts.BibitePropulsion.UpdateOrgan()`
(`BibitePropulsion.cs:185`, `public override void`, no args).

```
Prefix(BibitePropulsion __instance, out bool __state):
    __state = false;
    if (!EdgeIsOpen) return;
    if (!InBorderStrip(__instance.transform.position)) return;
    __state = AvoidVoidRef();            // AccessTools.StaticFieldRefAccess<BibitePropulsion, bool>("avoidVoid")
    AvoidVoidRef() = false;

Postfix(out bool __state):
    if (__state) AvoidVoidRef() = true;
```

Per-organism, per-edge, per-tick; no persisted state changes; the method body is
untouched. Cache the `StaticFieldRefAccess` delegate once — it runs at
`simTPS` (40 Hz default, `m1_findings.md` §8) × N organisms. Restore in the postfix
unconditionally on the true branch so a mid-run user change to the setting is not lost.

Simpler alternative if per-edge scoping turns out not to matter for the M2 exit test:
one call to `ScenarioSettings.Instance.voidAvoidance.SetValue(false)` at world load,
snapshotting the previous value and restoring it on unload (§1.5 option 2). Zero Harmony.
Prefer this for the first passing run, then tighten to layer 1 if the global change turns
out to distort the world.

**Also, in the same place: `ScenarioSettings.Instance.worldWrapping.SetValue(false)`
while an edge is open** (§3), so a missed capture does not turn into a teleport that
looks like a mod bug. Snapshot and restore like any other setting.

**Layer 2 — the lure. Measured 2026-08-02/03 as *not needed*.** With layers 0–1 only, an
open edge already draws 20.5 (A) and 24.4 (B) strip entries per simulated hour, so the
corridor zone below was canceled rather than built (*Open uncertainties*, item 2). What
follows is the design, kept for the day an ecology change drives the rate back to zero.
Suppression removes a force that ships disabled; it does not create a reason to go to
the edge. If the M2 crossing rate is near zero with layers 0–1 in place, add a
**corridor zone**: a low-fertility `ZoneSettings` reaching from the nearest island to the
open edge, added through the public `ScenarioSettings.Instance.AddNewZone`
(`ScenarioSettings.cs:1185`). Precedent: the `Void` zone in the shipped `3 Islands`
scenario (§1.4). It is world data, so gate it behind an explicit mod config flag and
remove it on unload. Escalate to the pheromone-beacon technique
(`Pherosense.cs:114-125`, sign flipped) only at M3, and only with a measured baseline —
the M3 milestone entry of `system_decomposition.md` already owns that decision.

### (b) Border detection placement

**Anchor: Harmony `Postfix` on `BibiteBody.FixedUpdate()`**
(`SimulationScripts.BibiteScripts/BibiteBody.cs:579`, `private void`, patched via
`AccessTools.Method(typeof(BibiteBody), "FixedUpdate")` — `m1_findings.md` §8).

Guards the postfix **must** re-apply, because a postfix runs on every return path
including the early ones:

| Guard | Why | Line |
|---|---|---|
| `Time.timeScale > 0f` | the method returns at `:583` when paused | `BibiteBody.cs:581-584` |
| `__instance.born` | same early return | `:581` |
| `!__instance.dead` | corpses return at `:588`; corpses must not migrate in M2 | `:585-589` |
| `!__instance.destroyed` | Unity fake-null window after `Destroy` | `m1_findings.md` §5.2 |
| not already in flight | the custody chain is async; one organism, one `migrationId` | D2 |

The test itself is a scalar compare against the strip, plus the outward-velocity test of
§4.4:

```csharp
float S = ScenarioIndependentSettings.Instance.SimulationSize.val;  // subscribe, don't snapshot (§2.4)
Vector2 p = __instance.transform.position;
if (p.x >= S - W && Vector2.Dot(rb.linearVelocity, Vector2.right) > 0f) → MIGRATE_OUT
```

**Cheaper alternative worth benchmarking**, since the postfix costs one delegate
invocation per organism per tick: a single central sweep over
`BibiteTracker.instance.bibites` (`ManagementScripts/BibiteTracker.cs:16`, filtering
`b != null` as every other consumer does) from one hook — either the same postfix on a
single well-known object, or `OneUseScripts/TimeKeeper.FixedUpdate` (`:148`,
`m1_findings.md` §8). One patch instead of N calls, and it can be throttled to every
k-th tick, since an organism moving at bibite speeds cannot cross a strip several body
lengths wide in one tick. **Start with the `BibiteBody.FixedUpdate` postfix (it is the
M1-anchored, already-understood hook) and only move to the sweep if profiling says so.**

Note the ordering inside a tick: `BibitePropulsion.UpdateOrgan` (the §(a) suppression
hook) runs at `BibiteBody.cs:613-616`, *before* the `FixedUpdate` postfix. So within one
tick the organism is first allowed to steer outward, then tested for the border. That is
the correct order.

### (c) Entry-position technique for imports

**Primary: rewrite the payload's `JObject` before `LoadBibiteOrEggFromData`.**
This is the M1 path unchanged (`m1_findings.md` §1.5), with eight numbers overwritten
first:

```csharp
var j = JObject.Parse(payload);
j["transform"]["position"][0] = entryX;   j["transform"]["position"][1] = entryY;
j["transform"]["rotation"]    = headingDeg;
j["rb2d"]["px"] = entryX;  j["rb2d"]["py"] = entryY;
j["rb2d"]["vx"] = vel.x;   j["rb2d"]["vy"] = vel.y;
j["rb2d"]["r"]  = headingDeg;
var go = SaveSystem.instance.LoadBibiteOrEggFromData(j.ToString(), true, null, null);
if (go == null) { /* the method swallows every exception — m1 §1.2 */ }
```

`DeserializePosition` consumes both keys at `SaveSystem.cs:446`, before any other
restore step, and nothing downstream moves the organism (§4.2).

**Then re-assert, in the same frame** (§4.3): write `go.transform.position`,
`rb.position`, `rb.linearVelocity`, `rb.rotation` and `go.transform.rotation` directly.
Two lines of insurance that keep the result independent of the parent transform — now
measured and benign (§4.3, caveat 1) but re-measured on every world load — and that
survive a future change to `SerializedTransform`. Caveat 2, the `Rigidbody2D` overwriting
a transform-only correction, is the live reason to keep them.

**Coordinate mapping**, for an organism leaving sim A eastward at `(S, y)` with velocity
`v`, entering sim B at its west edge:

```
entryX  = -S + W + margin        // inset past B's own border strip (§4.4)
entryY  =  y                     // transverse coordinate preserved  (requires S_A == S_B, §2.4)
vel     =  v                     // NOT mirrored — heading continuity is the point of D3
heading =  unchanged
```

Then apply the immunity window keyed on `BibiteBody.id.id` and the outward-velocity test
from §4.4, or the organism will migrate straight back on its first tick.

**Carried over from M1, unchanged and still required:** after the restore, repair
`genes.parent1/parent2` and any parent's `eggLayer.children` from the recorded IDs
(`m1_findings.md` §4.3, mirroring `SaveSystem.cs:748-782`). M1 never exercised that code
because its test organism had no links; M2 must run at least one migration on an organism
with living parents and living children (`system_decomposition.md:343`).

---

## Open uncertainties, ranked

1. ~~**Is `WorldObjectsSpawner.bibiteHolder`'s transform the identity?**~~ **RESOLVED at
   runtime, 2026-08-02** (§4.3, caveat 1). The mod logs it once per world load
   (`WorldSettings.LogBibiteHolderTransform`), and on game `0.6.3.1` it reports:

   ```
   bibiteHolder transform: position=(0.000000,0.000000,-0.010000)
                           rotation=(0.00, 0.00, 0.00) lossyScale=(1.00, 1.00, 1.00)
   ```

   **Identity in `x` and `y`, offset by `−0.01` in `z`, no rotation, unit scale.** The
   holder is a plain depth-sorting offset. The payload's `transform` key is therefore
   **world-space in the two coordinates the migration path writes**: `localPosition.x/y`
   equals `position.x/y`, and `transform` and `rb2d` agree on every number this contract
   touches. No `InverseTransformPoint` conversion is needed.

   Two consequences worth keeping:

   - A strict identity test reports `false` because of the `z` term. Test `x` and `y`, not
     `Vector3` equality, if anything ever branches on this.
   - `z` is not in the payload and the migration path never writes it, so the restored
     organism inherits the holder's depth like any other. Nothing to correct.

   The re-assert of §(c) stays anyway. It is cheap and idempotent, and it is what defends
   against caveat 2 — the `Rigidbody2D` overwriting a transform-only correction — which is
   a separate and still-live concern.
2. ~~**Will suppression alone produce a non-zero crossing rate?**~~ **RESOLVED by
   measurement, 2026-08-02/03** (§1.4, §1.5 option 5). The static reasoning held — void
   avoidance ships off, and what keeps organisms on islands is food density, not steering
   — but the conclusion drawn from it (that a lure would be needed) did **not**. With one
   edge open and no forcing, `CrossingStats` counted, over 30 real minutes at 20× on a
   seeded `Default` world:

   | Sector | Open edge | Strip entries / sim-hour | Population |
   |---|---|---|---|
   | A | E | 20.5 | ~26 |
   | B | W | 24.4 | ~26 |

   Roughly one border approach every three simulated minutes per sim, at a population of
   ~26. **The corridor-zone lure was therefore canceled, not built**, and the pheromone
   beacon (§1.5 option 5) stays parked in M3. The number to re-measure if the ecology
   changes is *strip entries per simulated hour*; a rate near zero is what would revive
   the lure. Raw counter lines land in `e2e/logs/crossing-{A,B}.log`.
3. **Can `SimulationSize` change mid-simulation through the UI?** (§2.4.) If it can, a
   cached `S` silently relocates the border strip, and two paired sims can drift out of
   agreement mid-run. Subscribing instead of snapshotting costs nothing and removes the
   question from the critical path — but the peer-agreement check at handshake time
   still needs to know whether to re-validate.

Lower-priority: whether `Zone.radius == 0` for rect zones is a game bug that will be
fixed (§1.1) — it only matters if the mod ever relies on rect zones; whether the
`avoidVoid` static is reachable before `BibitePropulsion`'s static constructor has run
(it is initialised lazily at class load, `BibitePropulsion.cs:63`), which a very early
Harmony patch could in principle observe; and whether disabling `worldWrapping` (§3) has
any second-order effect on an existing world's population distribution.
