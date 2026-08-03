# M1 Findings — Decompiled-Source Research and Runtime Results

Source: `decompiled/BibitesAssembly/` (ilspycmd output of
`The Bibites_Data/Managed/BibitesAssembly.dll`, Steam app 2736860, buildid 22383127).
All paths in this document are repo-relative. Line numbers refer to the decompiled
files as they exist in this checkout; they change when the game updates and
`sync-game-refs.sh` regenerates the decompile.

§1–§8 answer the eight research questions raised against Risks 1–5 of
`m1_considerations.md`. **Runtime results** at the end of this document records what the
in-game exit test actually did, on 2026-08-02.

## Headline result

The game already contains a complete, public, full-fidelity round trip for a single
live organism. M1 does not need to invent one.

| Step | Native API |
|---|---|
| Serialize live organism | `SaveSystem.SerializeBibite(GameObject)` (private) — or the public `SaveSystem.SaveBibite(GameObject, path, desc)` wrapper |
| Respawn from that payload | `SaveSystem.LoadBibiteOrEggFromData(string json, ...)` — **public, and called from nowhere in the assembly** |

`LoadBibiteOrEggFromData` instantiates the prefab, restores genes, body, clock and
brain, and resumes the organism. It does not mutate, does not randomize genes, and
preserves the entity ID. That collapses Risk 1, Risk 2 and Risk 4 to a much smaller
problem than the milestone assumed.

**Confirmed in-game on 2026-08-02.** The automated exit test passed on both of its two
runs. On the run against a world loaded from a save, the payloads before and after the
round trip were equal token for token, the entity ID survived, and the world save that
follows completed with no exception. See *Runtime results* at the end.

---

## 1. SPAWN API (Risk 1)

### 1.1 The single prefab instantiator

Every live bibite in the game comes from one call:

`decompiled/BibitesAssembly/ManagementScripts/WorldObjectsSpawner.cs:138-141`

```csharp
public GameObject GenerateNewBibite(Vector3? pos = null)
{
    return UnityEngine.Object.Instantiate(bibiteEntry, pos ?? Vector3.zero, Quaternion.identity, bibiteHolder.transform);
}
```

`bibiteEntry` is a `[SerializeField] GameObject` prefab (`WorldObjectsSpawner.cs:21`).
`WorldObjectsSpawner.Instance` is a public static singleton (`:17`, set in `Awake` at
`:114-126`). The new object is parented to `bibiteHolder` (`:68`), which is what
`nBibite` (`:89-99`) and `allBibites` (`:101-103`) enumerate.

`GenerateNewBibite` has exactly two callers in the whole assembly:

| Caller | Path |
|---|---|
| `SimulationScripts.BibiteScripts/EggHatching.cs:179` | egg hatching (the natural birth path) |
| `ManagementScripts/SaveSystem.cs:440` | `LoadBibite` (the save-restore path) |

`GenerateNewBibite` alone returns an **inert** GameObject: `Awake` has run on the
components, but no genes, no brain, no body. It must be followed by gene + brain +
body initialization or it will not function.

### 1.2 Path A — full-state restore (RECOMMENDED for M1)

`decompiled/BibitesAssembly/ManagementScripts/SaveSystem.cs:65-80`

```csharp
public GameObject LoadBibiteOrEggFromData(string json, bool resume = true, DialogGroupHandle problems = null, float? birthAtMaturity = null)
```

Behaviour: `JObject.Parse(json)`; if `jObject["egg"] == null` it calls
`LoadBibite(jObject, resume, problems, Utility.Version.Present, birthAtMaturity)`,
otherwise `LoadEgg(...)`. It is wrapped in `try { } catch { return null; }`
(`:76-79`) — **it swallows every exception and returns null**, so the mod must
null-check and cannot see the underlying error.

Access: `public` instance method on `SaveSystem`; the singleton is
`public static SaveSystem instance` (`SaveSystem.cs:27`, assigned in `Awake` at
`:45-49`, on the same GameObject as `WorldObjectsSpawner`).

`decompiled/BibitesAssembly/ManagementScripts/SaveSystem.cs:438-468`

```csharp
private GameObject LoadBibite(JObject state, bool resume = true, DialogGroupHandle problems = null, Utility.Version fromVersion = default(Utility.Version), float? birthAtMaturity = null)
```

Ordered side effects, verbatim from the source:

| Line | Call |
|---|---|
| 440 | `spawner.GenerateNewBibite(null)` |
| 446 | `SerializationHelper.DeserializePosition(state, gameObject, problems)` — transform + Rigidbody2D position/velocity/rotation |
| 447 | `SerializationHelper.DeserializeObject(component, (JObject)state["genes"])` → `BibiteGenes.LoadState` |
| 449 | `BibiteUpdater.UpdateGenesToPresentVersion(component.genes, fromVersion, state["genes"]["genes"], false)` |
| 450 | `DeserializeObject(component2, (JObject)state["body"])` → `BibiteBody.LoadState` |
| 451 | `DeserializeObject(component3, (JObject)state["clock"])` → `InternalClock` `[SerializeField]` fields |
| 452 | `DeserializeObject(component4, (JObject)state["brain"])` → `NEATBrain` `[SerializeField]` fields |
| 453 | `BrainUpdater.UpdateBrainFromVersion(component4, fromVersion)` |
| 458 | `component4.ResumeBrain()` |
| 461 or 465 | `StartBodyAtGrowthAndNormalize(birthAtMaturity.Value)` **or** `ResumeBody()` |

**Side effects that matter for M1:**

- **No mutation.** `LoadBibite` never calls `BibiteGenes.Mutate` or
  `NEATBrain.CopyBrain(..., mutate: true)`.
- **No gene randomization.** No `RandomGenes` / `RandomColor`.
- **ID preserved.** `BibiteBody.LoadState` sets `id.id` from the payload
  (`BibiteBody.cs:986-993`) before `ResumeBody()` runs. `FinalizeBirth` calls
  `id.CheckGetNew()` (`BibiteBody.cs:520`), which is a no-op when `id != 0`
  (`BibiteID.cs:9-15`).
- **Version migration runs.** Because `LoadBibiteOrEggFromData` passes
  `Utility.Version.Present` as `fromVersion`, all the `from < X` migration branches in
  `BibiteUpdater` / `BrainUpdater` are skipped for a same-version payload. Good for
  M1; relevant later for cross-version transfer.
- **Three RNG draws remain.** `ResumeBody()` → `StartBodyAtGrowthAndNormalize` is
  *not* taken when `birthAtMaturity` is null, so the `UnityEngine.Random.Range` calls
  at `BibiteBody.cs:436-438` do **not** run in the restore path. They only run in
  `StartBody()` and `StartBodyAtGrowthAndNormalize()`. **Pass `birthAtMaturity: null`.**
- **Live registration happens.** `ResumeBody()` calls
  `GlobalLineageManager.Instance.CheckNewSpecies(this)` if `gene.species == null`
  (`BibiteBody.cs:477-480`), `FinalizeBirth()` (`:481`) and
  `BibiteTracker.instance.TrackBibite(this)` when not dead (`:482-485`).
- **`dying` is honoured.** `BibiteBody.Start()` (`:405-411`) runs
  `if (dying) { Die(); }` on the next Unity `Start` pass.
- **Parent/child links are NOT relinked.** See §4.3.

### 1.3 Path B — template spawn (what the game UI uses)

`decompiled/BibitesAssembly/ManagementScripts/WorldObjectsSpawner.cs:143-181`

```csharp
public GameObject SpawnBibiteFromTemplate(BibiteTemplate template, RandomizeGenes randomizeGenes = RandomizeGenes.No, Tagging taggingChoice = Tagging.NoTagging, GrowthAtSpawn targetGrowth = GrowthAtSpawn.Egg, Vector3? pos = null, float? angle = null, string customTag = "")
```

```csharp
public GameObject SpawnEggFromTemplate(BibiteTemplate template, RandomizeGenes randomizeGenes = RandomizeGenes.No, Tagging taggingChoice = Tagging.NoTagging, Vector3? position = null, string customTag = "")
```
(`WorldObjectsSpawner.cs:183-208`)

Enums: `SettingScripts/RandomizeGenes.cs` (`No, NormalMutations, Color, AllGenes`),
`SettingScripts/Tagging.cs` (`NoTagging, SpeciesTagging, RandomTagging, CustomTagging`),
`SettingScripts/GrowthAtSpawn.cs` (`Egg, Baby, Adult, Elder`).

**This path is unsuitable for M1.** Side effects:

- `targetGrowth == GrowthAtSpawn.Egg` (the default) silently redirects to
  `SpawnEggFromTemplate` (`:146-149`), which calls
  `component.Mutate((int)component.genes[9])` (`:198`) and
  `CopyBrain(..., mutate: true)` (`:205`) — **guaranteed mutation**.
- Random rotation when `angle` is null: `Quaternion.Euler(0f, 0f, angle ?? UnityEngine.Random.Range(0, 360))` (`:145`).
- `StartBodyAtGrowthAndNormalize(...)` (`:173-179`) **overwrites the live state**:
  `health = maxHealth` (`BibiteBody.cs:451`), `Energy.Amount = Energy.MaxAmount`
  (`:452`), `growth.growth = targetGrowth` (`:453`), plus the three
  `UnityEngine.Random.Range` draws at `:454-456`.
- A brand-new random ID is assigned via `FinalizeBirth` → `id.CheckGetNew()`.
- The template carries only genes + brain topology. All live state is lost (§2.4).

Callers of `SpawnBibiteFromTemplate`:
- `ManagementScripts/BibitePlacer.cs:77` — the click-to-place editor tool.
- `SimulationScripts/BibiteSpawner.cs:168` — the scenario auto-spawner
  (`SpawnEntity(float delay)` coroutine, `:121-177`). Templates and per-species spawn
  policy come from `SimulationScripts/BibiteSpawnInfo.cs` and `ScenarioSettings.Instance.bibites`.

There is no separate "scenario spawn" API — scenarios spawn through `BibiteSpawner`,
which spawns through `SpawnBibiteFromTemplate`.

### 1.4 Path C — egg / hatch (the Constance-Mod hook)

See §6.

### 1.5 Verdict for Risk 1

Use **Path A**: `SaveSystem.instance.LoadBibiteOrEggFromData(json, resume: true, problems: null, birthAtMaturity: null)`.

It is public, has no in-assembly callers (so no other system will fight it), applies
no mutation or randomization, preserves the entity ID, and restores position and
velocity. A Harmony postfix to "undo the game's randomization" is **not required**
for this path — which removes the single largest unknown named in
`m1_considerations.md`.

---

## 2. SERIALIZATION (Risk 4)

### 2.1 Export entry points

| Line | Signature | Writes |
|---|---|---|
| `ManagementScripts/SaveSystem.cs:99` | `public void SaveBibiteOrEgg(GameObject target, string path, string desc)` | dispatches on the GameObject tag (`"bibite"` / `"egg"`) |
| `ManagementScripts/SaveSystem.cs:135` | `public void SaveBibite(GameObject bibite, string path, string desc)` | **`.bb8` with full live state** |
| `ManagementScripts/SaveSystem.cs:153` | `public void SaveEgg(GameObject egg, string path, string desc)` | `.bb8` (egg variant) |
| `ManagementScripts/SaveSystem.cs:115` | `public void SaveBibiteAsTemplate(GameObject bibite, string path, string bibiteName, string description)` | `.bb8template` |
| `ManagementScripts/SaveSystem.cs:124` | `public static void SaveTemplate(BibiteTemplate template, string path = null)` | `.bb8template` |
| `ManagementScripts/SaveSystem.cs:406` | `private static JObject SerializeBibite(GameObject bibite)` | the payload builder |

`SaveBibite` (`:135-151`) calls `SerializeBibite`, then adds
`jObject.Add("version", Application.version)` (`:147`) and
`jObject.Add("desc", desc)` (`:148`), then writes with
`Formatting.Indented` or `Formatting.None` per `UserSettings.FormatBibiteJSON.val`
(`:150`). Default fallback path is
`Path.Combine(Application.persistentDataPath, "bibite_quick_save.bb8")` (`:144`).

The UI chain behind the "save bibite" button:
`UIScripts/BibiteStatsPanel.cs:279` → `SaveController.Instance.SaveBibiteOrEgg(GameObject)`
(`ManagementScripts/SaveController.cs:102`) → dialog →
`SaveController.SaveDialogConfirmed()` (`:134`) →
`saveSystem.SaveBibiteOrEgg(tempObj, savePath, desc)` (`:157`).
The dialog only computes the path:
`UIScripts.UIReferences/SaveBibiteDialogHandle.cs:73-76`, `GetSavePath()`.

Storage roots: `SaveSystem.savedBibitePath` = `<persistentDataPath>/Bibites/`
(`SaveSystem.cs:41`); `SaveSystem.bibiteTemplatePath` = `<persistentDataPath>/Bibites/Templates/`
(`:43`). On this machine that resolves to
`C:\Users\<user>\AppData\LocalLow\The Bibites\The Bibites\Bibites\`.

### 2.2 The payload shape

`decompiled/BibitesAssembly/ManagementScripts/SaveSystem.cs:406-423`

```csharp
private static JObject SerializeBibite(GameObject bibite)
{
    JObject jObject = new JObject();
    SerializationHelper.SerializePosition(jObject, bibite);
    BibiteGenes component = bibite.GetComponent<BibiteGenes>();
    BibiteBody component2 = bibite.GetComponent<BibiteBody>();
    InternalClock component3 = bibite.GetComponent<InternalClock>();
    NEATBrain component4 = bibite.GetComponent<NEATBrain>();
    if (component == null) { return null; }
    jObject["genes"] = component.SaveState();
    jObject["body"] = component2.SaveState();
    jObject["clock"] = SerializationHelper.SerializeObject(component3);
    jObject["brain"] = SerializationHelper.SerializeObject(component4);
    return jObject;
}
```

Resulting top-level keys: `transform`, `rb2d`, `genes`, `body`, `clock`, `brain`,
plus `version` and `desc` when written through `SaveBibite`.

- `transform` — `ScriptHelpers/SerializedTransform.cs`: `float[2] position`,
  `float rotation`, `float scale`. Written at `SerializationHelper.cs:197`.
- `rb2d` — `SerializationHelper.SerializeRigidBody2D` (`:176-186`): `px, py, vx, vy, r`.
- `genes` — `BibiteGenes.SaveState()` (`SimulationScripts.BibiteScripts/BibiteGenes.cs:519-568`):
  `tag`, `speciesID`, `isReady`, `gen`, optional `isMutant`/`isRad`/`isRain`/`isKiller`/`isSource`,
  `parent1`/`parent2` (integer IDs), and a nested `genes` object keyed by
  `BibiteGenes.Genes` enum **name** (`:560-566`).
- `body` — `BibiteBody.SaveState()` (`SimulationScripts.BibiteScripts/BibiteBody.cs:928-950`). See §3.
- `clock` — `SerializeGeneralObject(InternalClock)`: `tic`, `ticProgress`, `timeAlive`,
  `chronoTime` (`InternalClock.cs:16-26`).
- `brain` — `SerializeGeneralObject(NEATBrain)`: `isReady`, `Nodes`, `Synapses`
  (`NEATBrain.cs:224-231`, the only three `[SerializeField]` members).

### 2.3 Data-model classes

**Genome.** `SimulationScripts.BibiteScripts/BibiteGenes.cs:13`
`public class BibiteGenes : MonoBehaviour, ISaveable`. Genome is
`public float[] genes` (`:70`), length `BibiteGenes.NGene` (`:116`), indexed by the
34-member `public enum Genes` at `:15-52`. Serialized by enum **name**, so
gene-order changes between versions do not corrupt payloads — only added/removed
genes do (handled by `ScriptHelpers/BibiteUpdater.cs`).

**Brain.** `SimulationScripts.BibiteScripts/NEATBrain.cs:11`
`public class NEATBrain : MonoBehaviour` — **not** `ISaveable`.

- `public struct Node` (`:87-155`): `NodeType Type`, `float baseActivation`,
  `string TypeName`, `int Index`, `long Inov`, `[NonSerialized] int NIn`,
  `[NonSerialized] int NOut`, `string Desc`, **`float Value`, `float LastInput`,
  `float LastOutput`**. The last three are the live neuron activation state.
  `public JObject SaveForTemplate()` (`:144-154`) deliberately emits only
  `Type, Index, Inov, Desc, baseActivation` — templates strip activations.
- `public struct Synaps` (`:164-185`): `long Inov`, `int NodeIn`, `int NodeOut`,
  `float Weight`, `bool En`.
- Arrays: `public Node[] Nodes` (`:227-228`), `public Synaps[] Synapses` (`:230-231`).
- `public void CopyBrain(Node[] parentNodes, Synaps[] parentSynapses, bool mutate, bool finalizeRightAway = true, bool checkForFloatingHidden = false)` (`:374`).
- `public void ResumeBrain()` (`:361-372`) — recomputes `nHidden`, `nSynaps`,
  `ComputeActiveNeurons()`, `ComputeProcessingStructure()`, `StartUsedSystems()`.

**Important asymmetry:** `FinalizeEditing()` (`:429-466`) copies `Inov, Type,
TypeName, Desc, NIn, NOut, Index, baseActivation` but **not** `Value / LastInput /
LastOutput`. So `CopyBrain` (the template path) always produces a zeroed brain, while
the save/restore path (`DeserializeObject` writing the whole `Node[]` then
`ResumeBrain()`) preserves activations. This is another reason Path A beats Path B.

**Stomach.** `SimulationScripts.BibiteScripts/StomachContent.cs:7`
`public struct StomachContent : ISaveable` with `MatterMaterial material`,
`float amount`, `digestionRate`, `lastConversionEfficiency`, `averageChunkAmount`,
`efficiency`. Its `SaveState()` (`:49-57`) writes only `material` (by name), `amount`,
`averageChunkAmount`.

**Matter.** `SimulationScripts/Matter.cs:8` `public class Matter : ISaveable` —
`public MatterMaterial Material`, `public float Amount`, `public float MaxAmount`.

### 2.4 Newtonsoft usage and what drives field inclusion

There is **no** `JsonConvert`, `JsonSerializerSettings`, `ContractResolver` or custom
`JsonConverter` anywhere in the assembly. Everything is hand-built
`Newtonsoft.Json.Linq` (`JObject`/`JArray`/`JToken`). The only
`using Newtonsoft.Json;` is `SaveSystem.cs:8`, purely for the `Formatting` enum.

The reflection engine is `decompiled/BibitesAssembly/ScriptHelpers/SerializationHelper.cs`:

| Line | Signature |
|---|---|
| `:23` | `public static JToken SerializeObject(object obj)` |
| `:36` | `public static JToken SerializeCollection(ICollection collection)` |
| `:57` | `public static JObject SerializeGeneralObject(object obj, JObject jObj = null)` |
| `:87` | `public static void DeserializeObject(object obj, JToken state)` |
| `:106` | `public static void DeserializeGeneralObject(object obj, JToken state)` |
| `:176` / `:188` | `SerializeRigidBody2D` / `DeserializeRigidBody2D` |
| `:195` / `:205` | `SerializePosition` / `DeserializePosition` |

`SerializeObject` dispatches: `ISaveable` → `SaveState()`; `ICollection` →
`SerializeCollection`; otherwise `SerializeGeneralObject`.

Field inclusion is driven by **`UnityEngine.SerializeField`**, checked by reflection:

```csharp
// SerializationHelper.cs:63-65
foreach (FieldInfo item in from field in obj.GetType().GetFields(BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic)
    where field.IsDefined(typeof(SerializeField), inherit: false)
    select field)
```

`BindingFlags.NonPublic` is present, so **private and internal `[SerializeField]`
fields are included**. `null` values are omitted (`:68`). Enums are written as names
(`:74-77`). `[NonSerialized]` plays no role in this helper — it matters only where the
helper hands off to raw Json.NET (`JToken.FromObject` at `:51`, `JObject.FromObject`
at `:197`).

`ScriptHelpers/ISaveable.cs`: `JObject SaveState(); void LoadState(JObject state);`

`ScriptHelpers/BibiteUpdater.cs:19`
`public static bool UpdateTemplateToPresentVersion(BibiteTemplate template, JToken json, bool isSaved)`
and `:31`
`public static float[] UpdateGenesToPresentVersion(float[] genes, Version from, JToken genesJson, bool geneRemapingNecessary = true, bool isSaved = false)`.
The `isSaved` flag reconciles the two on-disk shapes: `.bb8` nests genes at
`genes.genes`, `.bb8template` puts them flat at `genes`.

### 2.5 Import entry points

| Entry | Fidelity |
|---|---|
| `SaveSystem.LoadBibiteOrEggFromData(string json, ...)` (`:65`) | **full live state** — the only such path, and it has zero in-assembly callers |
| `SaveSystem.LoadGame(string zipFileName)` (`:617`) → `LoadBibite` (`:735`) / `LoadEgg` (`:739`) | full live state, but whole-world only |
| `Utility/BibiteTemplate.cs:63` `BibiteTemplate(string path, bool external = false)` | **template only** |

`BibiteTemplate`'s `.bb8` reader (`BibiteTemplate.cs:75-129`) extracts only
`isOfficial`, `desc`, `version`, `name`, `genes.genes[*]`, `genes.gen`,
`brain.Nodes`, `brain.Synapses`. It never touches `body`, `clock`, `transform` or
`rb2d`. Every UI import route — `BibiteTemplateSelectorPanel` (`:312-366`),
`BibitePlacer` (`:93`), `DefaultBibitesPanel` (`:83`), Steam Workshop
(`SteamIntegrations/WorkshopItem.cs:253, 471`) — goes through `BibiteTemplate`, so
**the shipped game always degrades a `.bb8` to a template on import**.

### 2.6 Does the on-disk `.bb8` include live runtime state? YES

A `.bb8` written by `SaveSystem.SaveBibite` contains position, linear velocity,
rotation, health, energy, fat reserves, stomach contents, maturity (`d2Size`), age
(`clock.timeAlive`), egg progress, damage/kill counters, and per-neuron activations —
in addition to genes and brain topology. Proof is `SerializeBibite` (§2.2) and
`BibiteBody.SaveState` (§3).

A `.bb8template` written by `SaveSystem.SaveTemplate` contains **only** the template.
`BibiteTemplate`'s `[SerializeField]` members are exactly five metadata strings/ints
(`BibiteTemplate.cs:31-44`); everything else on the class is `[NonSerialized]`
(`:16-59`). `SaveState()` (`:225-259`) then appends `nodes` (via
`Node.SaveForTemplate()`, activations stripped), `synapses`, a flat `genes` object,
optional `nodeAnchors` and `isOfficial`.

This corrects the assumption in `m1_considerations.md` that the `.bb8` schema is
"static". The **format** already carries the live payload; it is only the **reader**
used by the UI that throws it away.

Two `.bb8` dialects therefore exist in the wild:
`SerializeBibite` shape (`genes.genes`, plus `body`/`clock`/`transform`/`rb2d`) and
whatever community tools emit. `bb8-schema` (decision D4) must key off the presence
of `body` to tell them apart, exactly as `LoadBibiteOrEggFromData` keys off `egg`
(`SaveSystem.cs:70`).

---

## 3. LIVE STATE (Risk 2)

The live organism is a single GameObject tagged `"bibite"` with these components
(gathered in `BibiteBody.Awake` at `BibiteBody.cs:394-403` and wired as public fields
at `:19-56`): `Rigidbody2D`, `BibiteID`, `NEATBrain`, `BibiteGenes`, `BibiteGrowth`,
`InternalClock`, `Pherosense`, `BibitePheromoneOrgan`, `BibiteProceduralSpriter`,
`BibiteMouth`, `BibiteStomach`, `BibiteEggLayingOrgan`, `BibiteFatOrgan`,
`BibiteArmor`, `BibitePropulsion`, `FieldOfView`, `CapsuleCollider2D`.

`SimulationScripts.BibiteScripts/BibiteBody.cs:17`
`public class BibiteBody : MonoBehaviour, ISaveable`.

### 3.1 Access table

Read access is **public for everything M1 needs except the clock**. `internal` fields
are *not* accessible from a separate mod assembly and require reflection.

| Attribute | Type + member | File:line | Access | Notes |
|---|---|---|---|---|
| health | `BibiteBody.Health` (`Matter`) → `.Amount` | `BibiteBody.cs:78-82`; `Matter.cs:12` | **public field on public field** | read+write. `BibiteBody.health` property (`:268-278`) has `private set` — use `Health.Amount` |
| max health | `BibiteBody.Health.MaxAmount` | `Matter.cs:14` | public | recomputed by `Set2DSize` (`:704`) |
| energy | `BibiteBody.Energy.Amount` | `BibiteBody.cs:73-77`; `Matter.cs:12` | **public** | read+write |
| max energy | `BibiteBody.Energy.MaxAmount` | `Matter.cs:14` | public | recomputed by `Set2DSize` (`:705`) |
| fat reserves | `BibiteBody.fat.fatReserves` (`Matter`) → `.Amount` | `BibiteFatOrgan.cs:9-10` | public (`[NonSerialized]`) | serialized separately as `fatReservesAmount` |
| stomach contents | `BibiteBody.stomach.stomachContents` (`StomachContent[]`) + `.nContent` | `BibiteStomach.cs:27, 29` | **public** | fixed-size array; only the first `nContent` entries are valid |
| stomach fullness | `BibiteStomach.totalAmount` / `.fullness` / `.availableSpace` | `:95-108` | public getters | derived |
| maturity | `BibiteGrowth.growth` | `BibiteGrowth.cs:20-21` | **public** `[SerializeField]` | plus `public float maturity => growth / growthAtMature` (`:52`) and `public float growthAtMature` (`:23`) |
| age (seconds) | `InternalClock.timeAlive` | `InternalClock.cs:22-23` | **`internal`** `[SerializeField]` | **read via `BibiteBody.age` (public, `:304`, = `clock.timeAlive / 3600f`); write needs reflection or `SerializationHelper.DeserializeObject`** |
| chrono | `InternalClock.chronoTime` | `InternalClock.cs:25-26` | **`internal`** | same |
| clock tick | `InternalClock.tic`, `.ticProgress` | `InternalClock.cs:16-20` | `internal` / `private` | same |
| size (2D) | `BibiteBody.d2Size` | `BibiteBody.cs:84-85` | **public** `[SerializeField]` | the authoritative size; `growth` is derived from it on load (`:996`) |
| size (1D) | `BibiteBody.d1Size` | `BibiteBody.cs:87` | public | derived, `= sqrt(d2Size)` (`:690`) |
| position | `transform.position` | Unity | public | serialized as `transform` (localPosition) + `rb2d.px/py` |
| velocity | `GetComponent<Rigidbody2D>().linearVelocity` | Unity | public | `BibiteBody.rb2d` itself is `private` (`:19`) — call `GetComponent` from the mod |
| rotation | `transform.localRotation` / `rb2d.rotation` | Unity | public | |
| dying flag | `BibiteBody.dying` | `BibiteBody.cs:91-92` | **public** `[SerializeField]` | |
| dead flag | `BibiteBody.dead` | `BibiteBody.cs:94-95` | **public** `[SerializeField]` | corpse state |
| born flag | `BibiteBody.born` | `BibiteBody.cs:97-98` | public `[NonSerialized]` | gates `Update`/`FixedUpdate` |
| destroyed flag | `BibiteBody.destroyed` | `BibiteBody.cs:141-142` | public `[NonSerialized]` | set in `OnDestroy` |
| entity ID | `BibiteBody.id.id` | `BibiteID.cs:7` | **public** | |
| species | `BibiteGenes.species` (`Species`) | `BibiteGenes.cs:56` | public | |
| species tag | `BibiteGenes.speciesTag` | `BibiteGenes.cs:54` | public | |
| generation | `BibiteGenes.generation` | `BibiteGenes.cs:100` | public | |
| genome | `BibiteGenes.genes` (`float[]`) | `BibiteGenes.cs:70` | **public** | |
| brain nodes | `NEATBrain.Nodes` (`Node[]`) | `NEATBrain.cs:227-228` | **public** `[SerializeField]` | includes live `Value`/`LastInput`/`LastOutput` |
| brain synapses | `NEATBrain.Synapses` | `NEATBrain.cs:230-231` | **public** `[SerializeField]` | |
| total energy | `BibiteBody.totalEnergy` | `BibiteBody.cs:256-266` | public getter | derived |
| metabolism | `BibiteBody.metabolism` | `BibiteBody.cs:124` | public | |
| egg progress | `BibiteEggLayingOrgan.eggProgress` | `BibiteEggLayingOrgan.cs:27-28` | public `[NonSerialized]` | |

### 3.2 The native serializer already solves the read problem

`BibiteBody.SaveState()` (`BibiteBody.cs:928-950`):

```csharp
jObject["mouth"]            = mouth.SaveState();
jObject["stomach"]          = stomach.SaveState();
jObject["phero"]            = pheromoneOrgan.SaveState();
jObject["health"]           = Health.Amount;
jObject["energy"]           = Energy.Amount;
jObject["eggLayer"]         = eggLayer.SaveState();
jObject["control"]          = move.SaveState();
jObject["fatReservesAmount"]= fat.fatReserves.Amount;
jObject["id"]               = id.id;
// + every [SerializeField] field via reflection
```

The reflected `[SerializeField]` fields of `BibiteBody` are: `d2Size` (`:84`),
`dying` (`:91`), `dead` (`:94`), `attackedDmg` (`:100`), `timesAttacked` (`:103`),
`totalDamageSuffered` (`:106`), `brainTicksCount` (`:109`), `visionLookupCount`
(`:112`), `visionSensingCount` (`:115`), `corpseEnergyOffset` (`:132`).

Sub-organ payloads:
- `BibiteStomach.SaveState` (`BibiteStomach.cs:346-349`) — `content` array.
- `BibiteMouth.SaveState` (`BibiteMouth.cs:830-841`) — `attackedLastFrame`,
  `biteProgress`, `bibitesBitten`, `totalDamageDealt`, `totalMurders`, `murderedArea`.
- `BibiteEggLayingOrgan.SaveState` (`:228-238`) — `eggProgress`, `nEggsLaid`, `children`.
- `BibitePropulsion.SaveState` (`:267-270`) — `totalTravel`.
- `BibitePheromoneOrgan.SaveState` (`:87-90`) — `progress`.

`BibiteBody.LoadState()` (`:952-997`) mirrors it, and finishes with
`growth.growth = d2Size / (sizeRatio * sizeRatio)` (`:996`) — maturity is *derived*
from `d2Size`, not stored independently.

**Risk 2 verdict.** Read access is public for every attribute except the clock, and
even the clock does not need mod-side reflection: `SerializationHelper.SerializeObject`
and `DeserializeObject` are public static and do the `internal`-field reflection for
you. Nothing in `BibiteBody.SaveState`/`LoadState` requires the mod to reflect at all.

### 3.3 What is NOT in the payload

- `InternalClock.agingProgress`, `.period`, `.counting`, `.triggered1Day`
  (`InternalClock.cs:28-34`) — private, no `[SerializeField]`. `period` and `counting`
  are rebuilt by the `waitReady` coroutine (`:99-107`); `agingProgress` resets to 0.
  Cost: up to 120 s of simulated aging cadence drift. Cosmetic for M1.
- `BibiteGrowth.progress` **is** `[SerializeField]` (`:41-42`) but `BibiteGrowth` is
  not serialized at all by `SerializeBibite` — only `growth` survives, via `d2Size`.
- `FieldOfView` sensor arrays, `BibiteMouth.links` (latched joints),
  `BibiteBody.organs` — all rebuilt.
- `BibiteGenes.parent1` / `parent2` GameObject references — only the **IDs** survive
  (`BibiteGenes.cs:552-559` / `:598-605`). See §4.3.

---

## 4. ENTITY REGISTRY (Risk 5)

### 4.1 There is no ID registry

`decompiled/BibitesAssembly/SimulationScripts.BibiteScripts/BibiteID.cs` in full:

```csharp
public class BibiteID : MonoBehaviour
{
    public int id;                                        // :7

    public void CheckGetNew()                             // :9
    {
        while (id == 0)
        {
            id = Random.Range(int.MinValue, int.MaxValue); // :13
        }
    }

    public void Set(BibiteID from) { id = from.id; }      // :17
}
```

**There is no counter and no ID allocator.** IDs are random `int32` over the full
signed range, with `0` as the "unassigned" sentinel. Nothing about IDs is persisted
beyond each entity's own value. There is **no dictionary keyed by ID anywhere** — the
only lookup-by-ID in the assembly is a linear `FirstOrDefault` scan during world load
(`SaveSystem.cs:754, 766, 769, 779, 782`).

`CheckGetNew()` has exactly one caller: `BibiteBody.cs:520`, inside `FinalizeBirth()`.
It is a no-op when the ID is already non-zero, which is why the restore path keeps
the original ID.

`Set(BibiteID)` has exactly one caller: `EggHatching.cs:196`,
`newBornBody.id.Set(id)` — the egg's ID transfers to the newborn.

**Risk 5 verdict: substantially smaller than assumed.** There is no ID re-index step
and no collision-prone registry table. Reusing the original ID on respawn is the
*correct* behaviour, not a hazard. Collision probability for a fresh random ID is
~n/2^32.

### 4.2 What actually tracks live entities

| Registry | Location | Rebuilt on destroy? |
|---|---|---|
| `BibiteTracker.instance.bibites` (`List<BibiteBody>`) | `ManagementScripts/BibiteTracker.cs:14, 16` | yes — `BibiteBody.OnDestroy` calls `BibiteDeath` (`BibiteBody.cs:903`) |
| `BibiteTracker.instance.eggs` (`List<EggHatching>`) | `BibiteTracker.cs:18` | yes — `EggHatching.OnDestroy` (`:263`) |
| `Species.bibites` (`List<BibiteBody>`) | `SimulationScripts.BibiteScripts/Species.cs:58` | yes — `OnDestroy` calls `gene.species?.RemoveFromSpecies(this)` (`BibiteBody.cs:894`) |
| `BibiteTag.bibites` | `ManagementScripts/BibiteTag.cs:13` | yes — `OnDestroy` (`BibiteBody.cs:898`) |
| Transform children of `WorldObjectsSpawner.Instance.bibiteHolder` | `WorldObjectsSpawner.cs:68, 101-107` | yes — Unity |
| `GlobalLineageManager.Instance.recordedSpecies` / `.activeSpecies` | `ManagementScripts/GlobalLineageManager.cs:20, 23` | species-level only; no per-individual record |

`BibiteTracker.instance` is `public static` (`:14`), assigned in `Awake` (`:42-49`)
with **no null-guard and no `DontDestroyOnLoad`**. Methods:

```csharp
public void BibiteBirth(BibiteBody bibite)   // :161  — bibites.Add(bibite); nBirth++;
public void TrackBibite(BibiteBody bibite)   // :167  — bibites.Add(bibite);   NO duplicate check
public void BibiteDeath(BibiteBody bibite)   // :172  — if (bibites.Contains) { Remove; nDeath++; deathAgesQuantiles.Add(bibite.age); }
```

`TrackBibite` has no duplicate guard. Every consumer of `bibites` filters
`.Where(b => b != null)` because destroyed entries can linger as Unity fake-null
(`BibiteTracker.cs:28-40, 58, 113-115, 142-144`).

Persisted global counters (all in `GlobalLineageManager.SaveState`/`LoadState`,
`:352-383`): `nodeMaxInnovation` (`NEATBrain.nodeMaxInov`), `connectionMaxInnovation`
(`NEATBrain.connectionMaxInov`), `nextSpeciesID` (`Species.speciesMaxID`, declared
`public static long speciesMaxID = 1L` at `Species.cs:19`). Plus `BibiteTracker.nBirth`
/ `nDeath` (`:194-195` / `:209-210`).

### 4.3 What a mod MUST re-index after destroy + respawn

`LoadBibite` does **not** relink object references. `SaveSystem.LoadGame` does it
afterwards, at `SaveSystem.cs:741-784`:

1. `genes.parent1` / `genes.parent2` — `BibiteGenes.LoadState` only fills the integer
   `parent1ID` / `parent2ID` (`BibiteGenes.cs:598-605`). The `GameObject` fields
   (`:60`, `:65`) stay null. `LoadGame` resolves them at `SaveSystem.cs:766, 769`
   with a linear scan.
2. `eggLayer.children` (`List<BibiteID>`) — `LoadState` fills only `childIDs`
   (`BibiteEggLayingOrgan.cs:67, 252`). `LoadGame` resolves at `SaveSystem.cs:748-760`.
3. `gene.species` — this one **is** handled inside `BibiteGenes.LoadState` (`:581-584`)
   via `GlobalLineageManager.Instance.recordedSpecies.FirstOrDefault(s => s.speciesID == ...)`,
   with `ResumeBody` falling back to `CheckNewSpecies` when it is still null
   (`BibiteBody.cs:477-480`).

For M1 (destroy + immediately respawn the same organism in the same world), items 1
and 2 must be re-applied by the mod, or the respawned organism loses its parent link
and its outstanding egg children. Two additional consequences:

- **Other bibites' `eggLayer.children` lists** hold `BibiteID` component references to
  the destroyed organism. After a raw `Destroy`, those become fake-null and
  `BibiteEggLayingOrgan.SaveState` (`:235`, `children.Select(c => c.id)`) will throw
  on the next world save.
- **Other bibites' `genes.parent1`** may point at the destroyed GameObject.
  `GlobalLineageManager.cs:230-232` does
  `BibiteBody bibiteBody = ((bibite.gene.parent1 == null) ? bibite : bibite.gene.parent1.GetComponent<BibiteBody>());`
  then dereferences `bibiteBody`. On a destroyed parent, `parent1 == null` is *true*
  under Unity's fake-null operator, so this specific line is safe — but a destroyed
  parent GameObject that is *not yet* finalized (same frame) returns a null
  `GetComponent` and the following lines NRE. Low probability, worth guarding.

**Mitigation for M1:** repair both link sets after respawn by scanning
`BibiteTracker.instance.bibites` for the matching `id.id`, exactly as
`SaveSystem.cs:748-782` does. Since M1 reuses the original ID, the repair is a
straight substitution of the new `BibiteID`/`GameObject` for the old one.

---

## 5. DESTROY PATH (Risk 3)

### 5.1 The corpse decision

`SimulationScripts.BibiteScripts/BibiteBody.cs:771-816`, `public void Die(bool swallowed = false)`:

```
:773  dying = true;
:778  BibiteTracker.instance.BibiteDeath(this);
:784  gene.species?.RemoveFromSpecies(this);
:788  TagsManager.instance.RemoveFromTag(this, localTag);
:791  dead = true;
:792  onDeathStatusChange.Invoke(this);
:797  if (!ScenarioSettings.Instance.corpsesEnabled.val) { ExplodeToMeat(); }   // -> meat
:801  else if (born) {
:803      if (swallowed) { eggLayer.eggProgress = 0f; Destroy(gameObject); return; }
:809      growth.enabled = false; clock.enabled = false;
:811      corpseEnergyOffset = ...; Energy.Amount = 0f; Health.Amount = Health.MaxAmount;
      }                                                                          // -> corpse
```

`corpsesEnabled` is declared at `SettingScripts/ScenarioSettings.cs:277-284`
(`DefaultValue = true`) and read **live** (`.val`) at exactly one gameplay site:
`BibiteBody.cs:797`.

`public void ExplodeToMeat()` (`:830-842`) is the meat source:
`ParticlesMaster.BloodSplatterAtPosition` (`:834`),
`WorldObjectsSpawner.Instance.SpawnMeatPile(totalEnergy, position, bodyLength/2f)`
(`:835`), then `organs.ForEach(o => o.OnBodyDestroyed())` (`:836-839`) — and
`BibiteStomach.OnBodyDestroyed()` (`BibiteStomach.cs:227-252`) spawns **one pellet per
stomach content** (`:245-249`), a second independent pellet source. Then
`Destroy(gameObject)` (`:840`).

### 5.2 Removing an organism WITHOUT a corpse and WITHOUT meat

**Answer: `UnityEngine.Object.Destroy(body.gameObject)` directly.**

This is exactly what the shipped game does in
`decompiled/BibitesAssembly/UIScripts/SelectionPanel.cs:306-312` (`DeleteSelection()`
at `:306`, `Object.Destroy(item.gameObject)` at `:310`):

```csharp
public void DeleteSelection()
{
    foreach (BibiteBody item in bibitesSelection.Where((BibiteBody b) => b != null && !b.destroyed))
    {
        Object.Destroy(item.gameObject);
    }
    ...
}
```

`BibiteBody.OnDestroy()` (`:884-926`) **spawns nothing**. It sets `destroyed = true`
(`:886`), destroys `brain`/`fow`/`pheromoneOrgan` (`:887-889`), deregisters from the
species (`:894`), the tag (`:898`) and the tracker (`:903`), invokes and clears
`onDestroyed` (`:905-907`), and nulls every field (`:908-923`). Critically it **never
calls `OnBodyDestroyed()`**, so the stomach never vomits pellets.

| Approach | Corpse | Meat pile | Stomach pellets | Blood | Eggs laid | Registries cleaned |
|---|---|---|---|---|---|---|
| `Object.Destroy(body.gameObject)` | no | no | no | no | no | **yes** (`OnDestroy` `:894/898/903`) |
| `body.Die()` | yes (if enabled) | eventually | eventually | eventually | **yes** | yes |
| `body.Die(swallowed: true)` | no *if `corpsesEnabled`* | **meat if `corpsesEnabled == false`** | " | " | no (`:805` zeroes progress) | yes |
| `body.Swallow()` | routes to `Die(swallowed: true)` when alive (`:826`) | same caveat | " | " | no | yes |
| `body.ExplodeToMeat()` | no | **yes** | **yes** | **yes** | no | yes |

`Die(swallowed: true)` is a trap: when `corpsesEnabled` is **false** it falls into
`ExplodeToMeat()` at `:799` before reaching the swallow branch. Only raw `Destroy` is
unconditionally clean.

Note `BibiteEggLayingOrgan.OnDeathStatusChange` (`:205-221`) lays out any remaining
whole eggs on death (`while (eggProgress >= 1f) { LayEgg(); eggProgress -= 1f; }`,
`:215-219`). Raw `Destroy` never fires `onDeathStatusChange`, so it emits no eggs —
another reason to prefer it for M1.

Two caveats for a mod using raw `Destroy`:

- `OnDestroy` still calls `BibiteTracker.BibiteDeath` (`:903`), which increments
  `nDeath` and pushes the age into `deathAgesQuantiles` (`BibiteTracker.cs:174-179`).
  To make the removal statistically invisible, remove the body from
  `BibiteTracker.instance.bibites` **first** — then `bibites.Contains(bibite)` is
  false and the counters stay untouched.
- `Destroy` is deferred to end-of-frame. `body.destroyed` stays `false` until
  `OnDestroy` runs. `SelectionPanel` compensates with
  `.Where(b => b != null && !b.destroyed)`. **M1 must serialize before destroying and
  must not assume the old object is gone in the same frame.**

### 5.3 Systems that hold a live reference

| System | Field | File:line | Self-heals? |
|---|---|---|---|
| Selection / camera / inspector UI | `TargetableObject.onDestroy` (UnityEvent) | `PropertiesScripts/TargetableObject.cs:10, 22-28` | **YES — this is the game's real destruction contract.** Fires from Unity's own `OnDestroy`, therefore for *any* removal path including a raw mod `Destroy`. |
| `UserControl` | `public GameObject target` / `public BibiteBody bibiteTarget` | `ManagementScripts/UserControl.cs:35, 37` | mostly. `SelectTarget` subscribes `component.onDestroy.AddListener(TargetDied)` (`:561`); `TargetDied` (`:513-532`) clears or reselects. **`bibiteTarget` (`:37`) is never nulled** — it retains a fake-null until the next `SelectBibiteTarget`. Cosmetic. |
| `CameraManager` | `private GameObject followTarget` | `ManagementScripts/CameraManager.cs:30, 236-240` | yes — `LateUpdate` (`:110-115`) null-guards; wired to `onTargetChange` (`:68`) |
| `GUIManager` + `BibitePanel`s | `private BibiteBody body` | `GUIManager.cs:83`; `UIScripts.UIPanels/BibitePanel.cs:8` | yes — via `UserControl.onTargetChange` → `ClearTarget()` (`GUIManager.cs:345-359`) → `panel.ResetState()` |
| Predator vision | `FieldOfView.seenBibites` / `seenCorpses` / `bibiteWeights` / `held` | `SimulationScripts.BibiteScripts/FieldOfView.cs:17, 20, 35, 38` | **partially.** Fixed 128-entry arrays, refreshed only every `visionLookupPeriod` brain ticks (`BibiteBody.cs:594-599`), never cleared — only counts reset (`FieldOfView.cs:209`). Every consumer null-checks (`:311, 324, 337, 350`), so stale entries are tolerated by Unity fake-null. |
| Predator mouth | `BibiteMouth.otherBody` (`private BibiteBody`), `objectsInMouth[]`, `links[]` | `BibiteMouth.cs:48, 40, 36` | **weakest link.** `otherBody` is persistent and never cleared. `objectsInMouth` is maintained by `OnTriggerExit2D` (`:430-447`), which **may not fire if the object is destroyed inside the trigger**. `OnLinkBroke` (`:449-467`) does an unguarded `links[i].connectedBody.GetComponent<GrabbableObject>().Release(this)` (`:459`) — NRE if the grabbed object is gone. Same unguarded pattern in `ReleaseAndMaybeThrowAllHeldObjects` (`:710`). |
| Parent's egg layer | `BibiteEggLayingOrgan.children` (`List<BibiteID>`) | `BibiteEggLayingOrgan.cs:65` | **NO.** Cleared only in the *owner's* `OnDestroy` (`:223-226`). A destroyed child leaves a fake-null that `SaveState` (`:235`) dereferences → throws on next world save. |
| Child's gene record | `BibiteGenes.parent1` / `parent2` (`GameObject`) | `BibiteGenes.cs:60, 65` | **NO.** Never cleared. Guarded at the save site (`:552-559`) but see `GlobalLineageManager.cs:230-232`. |
| `RadioZone` (only for `isRad` bibites) | attached-bibite ref | `OneUseScripts/RadioZone.cs:54` | yes — `Update` null-checks (`:61-65`) and self-fades |
| FOV debug renderer | `FieldOfViewDisplay.body` | `SimulationScripts.Rendering/FieldOfViewDisplay.cs:38` | yes, via `onDestroyed` |

Subscribers to `BibiteBody.onDestroyed` (`:150`): exactly **one** —
`SimulationScripts.Rendering/FieldOfViewDisplay.cs:123`.
Subscribers to `onDeathStatusChange` (`:147`): `BibiteOrgan.cs:21`,
`BibiteEggLayingOrgan.cs:188` (parent watching a child), `UserControl.cs:585`,
`BibiteStatsPanel.cs:149`, `BiologyPanel.cs:541`, `FieldOfViewDisplay.cs:122`,
`RadioZone.cs:54`. Both events are `RemoveAllListeners()`'d in `OnDestroy`
(`:906-907`).

**Risk 3 verdict.** Destroying one live bibite mid-simulation is safe and already
done by shipped UI code. Two pre-existing latent hazards are worth defensive handling
in the mod: a bibite being actively *bitten or grabbed* by another bibite when it is
destroyed, and stale `BibiteID` entries in a parent's `children` list. For M1 the
simplest mitigations are (a) pick a test subject that is not latched to another
organism, and (b) after respawn, substitute the new `BibiteID` into any parent's
`children` list that held the old one.

---

## 6. EGG / HATCH FALLBACK

`decompiled/BibitesAssembly/SimulationScripts.BibiteScripts/EggHatching.cs:10`
`public class EggHatching : MonoBehaviour, ISaveable`.

Public fields: `eggGene` (`:12`), `eggBrain` (`:14`), `id` (`:16`), **`newBornBody`
(`:22`)**, `onHatch` (`:26`, `UnityEvent<EggHatching, BibiteBody>`), `energy` (`:28`),
`totalRequiredEnergy` (`:30`), `isHatching` (`:32`), `hatchProgress` (`:34`),
`startTime` (`:36`), `hatchTime` (`:38`), `aborting` (`:40`), `[SerializeField] int
bibiteID` (`:42-43`), `birthOrientation` (`:45`), `sizeGene` (`:47`), `eggMass` (`:49`).

```csharp
public void StartHatch(float? hatchEnergy = null)   // :105
public void ResumeHatch()                           // :133
private void FixedUpdate()                          // :156  — drives hatchProgress, calls Hatch() at :167
private void Hatch()                                // :172  <-- THE HOOK
```

`Hatch()` body, in order:

| Line | Statement |
|---|---|
| 174-178 | `if (totalRequiredEnergy > energy) { Abort(); return; }` |
| **179** | **`GameObject gameObject = WorldObjectsSpawner.Instance.GenerateNewBibite(null);`** |
| 180 | `gameObject.transform.position = base.transform.position;` |
| 181-188 | orientation from `birthOrientation`, else random |
| 189-191 | `newBornGene` / `newBornBrain` / `newBornBody = gameObject.GetComponent<...>()` |
| 192 | `newBornGene.CopyGene(eggGene);` |
| 193 | `newBornGene.SetParents(eggGene.parent1, eggGene.parent2);` |
| 194 | `newBornBrain.CopyBrain(eggBrain.Nodes, eggBrain.Synapses, mutate: false);` |
| 195 | `newBornBody.Energy.Amount = energy;` |
| 196 | `newBornBody.id.Set(id);` |
| **197** | **`newBornBody.StartBody();`** |
| 198 | `energy = 0f;` |
| 199 | `onHatch.Invoke(this, newBornBody);` |
| 200 | `UnityEngine.Object.Destroy(base.gameObject);` |

**Harmony target: `[HarmonyPostfix]` on
`SimulationScripts.BibiteScripts.EggHatching.Hatch()` (`EggHatching.cs:172`,
`private void`, no args).** In the postfix, `__instance.newBornBody` is a public field
still valid (the `Destroy` at `:200` is deferred to end-of-frame). This is the
Constance-Mod hook point.

Note that mutation happens at **lay** time, not hatch time: `BibiteEggLayingOrgan.LayEgg()`
does `eggGene.CopyGene(genes, increaseGeneration: true, mutate: true)`
(`BibiteEggLayingOrgan.cs:168`) and `eggBrain.CopyBrain(..., mutate: true)` (`:170`).
`Hatch()` itself copies with `mutate: false`.

Reachable alternative without patching: subscribe to the public
`onHatch` event (`:26`). It is also invoked with `null` on `Abort()` (`:206`), so a
listener must null-check.

Direct egg-laying API: `public float LayEgg()` (`BibiteEggLayingOrgan.cs:159`) — no
arguments, self-contained, calls `GenerateNewEgg` (`:163`) and `StartHatch(eggEnergy)`
(`:171`). It does not debit the parent's energy or decrement `eggProgress`; callers do
`while (eggProgress >= 1f) { LayEgg(); eggProgress -= 1f; }`.

Egg-from-template: `WorldObjectsSpawner.SpawnEggFromTemplate` (`:183-208`) ends with
`component2.StartHatch(null)` (`:206`) after setting
`component2.energy = component.LayBodyEnergy` (`:204`).

**Assessment for M1.** The egg path is a valid fallback but strictly worse than Path A:
`StartBody()` (`BibiteBody.cs:413-441`) resets health to max (`:427`), sets
`d2Size = 0` then `Set2DSize(gene.D2SizeAtBirth)` (`:415, 426`), restarts the clock
(`:432`), and draws three `UnityEngine.Random.Range` values (`:436-438`). Every live
attribute would have to be overwritten afterwards, and the brain's neuron activations
would be zeroed by `CopyBrain`/`FinalizeEditing`. **Keep it as a fallback only.**

---

## 7. VERSION

### 7.1 Where the version lives

The assembly **never hardcodes** the game version. It only reads
`UnityEngine.Application.version`:

| File:line | Code |
|---|---|
| `Utility/Version.cs:21` | `public static Version Present => Parse(Application.version);` |
| `UIScripts/VersionDisplay.cs:13` | `((TMP_Text)text).text = "The Bibites " + Application.version;` |
| `ManagementScripts/SaveSystem.cs:147, 165, 184, 656, 667, 964` | stamps `version` into `.bb8`, `.bb8scene` and save metadata |

`Utility/Version.cs:7` `public struct Version : IComparable<Version>` with fields
`V0, V1, V2, V3, Alpha` and a parse regex at `:19`:
`^[0-9]+(\.[0-9]+)(\.[0-9]+)?(\.[0-9]+)?([aA][0-9]+)?$`.
`Properties/AssemblyInfo.cs:7` is `[assembly: AssemblyVersion("0.0.0.0")]` — an ILSpy
stub, useless.

### 7.2 The actual version string: **`0.6.3.1`**

`Application.version` is `bundleVersion` in the Unity build data, not in the
assembly. Extracted from
`C:\Program Files (x86)\Steam\steamapps\common\The Bibites\The Bibites_Data\globalgamemanagers`
at byte offset 2008, in the PlayerSettings length-prefixed string table:

```
"The Bibites" "The Bibites" "public.app-category.games" "1.0" "1.0" "0.6.3.1" "752d1031-9ef2-4d5e-b966-46b3cb42acf..."
```

`0.6.3.1` sits in the `bundleVersion` slot, immediately before the Unity
`cloudProjectId` GUID (which matches the `Unity/752d1031-.../` folder in the game's
persistent data). It is the only game-authored version-shaped string in the file
(the other is the editor version `6000.0.44f1`, matching `dev_environment.md`).

**Confirmed at runtime, 2026-08-02.** The plugin logs
`Application.version = 0.6.3.1` at startup, so the static reading of `globalgamemanagers`
was correct. This also matches the Steam release note quoted at `initial_research.md:287`
("The Bibites 0.6.3: Corpses and Seasons").

Steam metadata: app 2736860, `buildid 22383127`, `"name" "The Bibites: Digital Life"`.
Unity 6000.0.44f1, Mono backend.

### 7.3 A trap in the version constructors

`Utility/Version.cs` declares two constructors:

```csharp
public Version(int v0 = 0, int v1 = 0, int v2 = 0, int a = 0)      // :54  — 4th arg is ALPHA
public Version(int v0, int v1, int v2, int v3, int a = 0)          // :63  — 4th arg is V3
```

For a 4-argument call, C# overload resolution prefers the candidate with no omitted
optional parameters, i.e. the **first** overload. So every `new Version(0, 6, 0, 15)`
style call in the codebase means **`0.6.0a15`, an alpha, not `0.6.0.15`**.

Corrected reading of the migration thresholds:

| Literal | Actual meaning | Where |
|---|---|---|
| `new Utility.Version(0, 6, 3, 3)` | **0.6.3a3** — highest literal in the assembly | `ScriptHelpers/SerializationHelper.cs:467` (gates `settingsChangers`) |
| `new Version(0, 6, 2, 1)` | 0.6.2a1 — newest brain-layout change | `ScriptHelpers/BrainUpdater.cs:16` |
| `new Version(0, 6, 0, 18)` / `(0,6,0,16)` / `(0,6,0,15)` / `(0,6,0,13)` / `(0,6,0,10)` / `(0,6,0,9)` / `(0,6,0,5)` / `(0,6,0,4)` / `(0,6,0,1)` | 0.6.0a18 … 0.6.0a1 | `BrainUpdater.cs`, `VersionTracker.cs`, `Utility/BibiteTemplate.cs:314`, `Utility/DataLogger.cs:362-363`, `Utility.DataLogging/LineageVersionUpdater.cs:14` |
| `new Version(0, 5, 1)` | 0.5.1 (3-arg → V2) | `UIScripts.UIReferences/ScenarioItemReference.cs:89` |
| `new Version(0, 4)` / `(0, 3)` | 0.4.0 / 0.3.0, `canUpdateFromOlderVersion = false` | `ScriptHelpers/VersionTracker.cs:37, 42` |

Running `0.6.3.1` is above all of them (`operator >` at `Version.cs:127-166` compares
`V3` before `Alpha`, so `0.6.3.1 > 0.6.3a3`). This is self-consistent and corroborates
the `0.6.3.1` reading. **`bb8-schema` must reproduce this constructor quirk or it will
mis-order versions.**

---

## 8. PATCH TARGETS

`BibiteBody` lifecycle methods (`SimulationScripts.BibiteScripts/BibiteBody.cs`), all
`private void` — Harmony patches private methods fine via
`AccessTools.Method(typeof(BibiteBody), "FixedUpdate")`:

| Method | Line | Notes |
|---|---|---|
| `Awake()` | 394 | caches sibling components |
| `Start()` | 405 | `if (dying) Die();` |
| **`Update()`** | **564** | gated on `born`; shader/position updates only |
| **`FixedUpdate()`** | **579** | **the real per-organism tick.** Gated on `born && Time.timeScale > 0f`. Runs metabolism (`:590`), brain ticks (`:591-608`), `organs.ForEach(o => o.UpdateOrgan())` (`:613-616`), `Heal()` (`:622`), rigidbody mass/inertia/drag (`:623-625`). **This is the border-hook target for M2.** |
| `OnDestroy()` | 884 | |
| `OnJointBreak2D(Joint2D)` | 637 | public |

Other bibite components (agent-verified, all `public class`, all lifecycle methods
`private void`):

| Class | Awake | Start | FixedUpdate | OnDestroy | Other |
|---|---|---|---|---|---|
| `NEATBrain` | 350 | — | — | 1095 | `Thinker()` 554, `UpdateSenses()` 571, `ResumeBrain()` 361, `CopyBrain()` 374 |
| `BibiteGrowth` | 54 | — | 91 | — | |
| `InternalClock` | — | — | 65 | — | |
| `BibiteMouth` | — | — | — | 480 | `OnTriggerEnter2D` 420, `OnTriggerExit2D` 430 |
| `BibitePropulsion` | — | — | — | 258 | |
| `FieldOfView` | — | — | — | 477 | `FindSeenEntities()` 203, `ComputeSenses()` 275 |
| `BibiteArmor` | — | — | — | 65 | |
| `BibiteEggLayingOrgan` | — | — | — | 223 | |
| `BibitePheromoneOrgan` | — | — | — | 81 | |
| `BibiteProceduralSpriter` | 52 | 66 | — | 156 | |
| `BibiteGenes`, `BibiteStomach`, `BibiteFatOrgan`, `Pherosense` | — | — | — | — | no Unity lifecycle methods at all |

**No bibite component has `Update`, `LateUpdate`, `OnEnable` or `OnDisable` except
`BibiteBody.Update` and `BibiteProceduralSpriter.Start`.** The organs are not ticked
independently by Unity — `BibiteBody.FixedUpdate:615` is the only caller of
`UpdateOrgan()` in the whole assembly. To hook a specific organ, patch its concrete
`public override void UpdateOrgan()`: `BibiteStomach.cs:162`, `BibiteMouth.cs:325`,
`BibitePropulsion.cs:185`, `BibiteEggLayingOrgan.cs:119`, `BibiteFatOrgan.cs:89`,
`BibiteArmor.cs:61`, `BibitePheromoneOrgan.cs:62`.

Managers:

| Class | Singleton | Lifecycle | Useful surface |
|---|---|---|---|
| `ManagementScripts/SimulationManager.cs:11` | `public static SimulationManager Instance` (`:16`) | `Awake` 38, `Start` 50 | `public UnityEvent beforeSimStart` (`:34`), fired at `:75` after world load — **the "world is ready" signal for a mod** |
| `ManagementScripts/GameManager.cs:9` | none — all static | **none** | `public static UnityEvent onSceneChange` (`:21`), `public static UnityEvent<string> onChangeToScene` (`:23`), `public static bool isSim` (`:29`) |
| `ManagementScripts/TimeController.cs:13` | `public static TimeController Instance` (`:15`) | `Awake` 64, `OnDestroy` 178 | `public static bool paused` (`:55`); **`public void TogglePauseGame(string source = "base", bool isUnpause = false)` (`:141`) — a refcounted, source-keyed pause stack; use a unique source string** |
| `OneUseScripts/TimeKeeper.cs:8` | `public static double simulatedTime` (`:16`) | `Start` 76, **`Update` 95**, **`FixedUpdate` 148** | the best global per-tick hook; `Update` early-returns while paused |

Sim tick rate comes from `ScenarioIndependentSettings.Instance.simTPS` via
`TimeController.UpdateFixedDeltaTime(int tps)` (`:77-81`); `baseFixedDeltaTime`
defaults to `0.025f` (40 TPS, `:57`).

---

## Recommended M1 approach

### Spawn path

Use `SaveSystem.instance.LoadBibiteOrEggFromData(json, resume: true, problems: null, birthAtMaturity: null)`
(`ManagementScripts/SaveSystem.cs:65`).

Rationale: public; zero in-assembly callers; no mutation; no gene randomization; no
`Random.Range` draws on the restore branch; preserves entity ID, position, velocity,
neuron activations and age. **A postfix hook to undo randomization is not needed.**

### Where to get the payload

Two options, both acceptable:

1. **In-memory (preferred).** Reflect once on
   `private static JObject SerializeBibite(GameObject)` (`SaveSystem.cs:406`) via
   `AccessTools.Method(typeof(SaveSystem), "SerializeBibite")`, invoke it, add
   `["version"] = Application.version`, and pass `.ToString()` straight to
   `LoadBibiteOrEggFromData`. No disk I/O, no `.bb8` dialect ambiguity.
2. **Via disk (fully public, zero reflection).** Call
   `SaveSystem.instance.SaveBibite(go, tempPath, "m1")` (`:135`), then
   `LoadBibiteOrEggFromData(File.ReadAllText(tempPath))`. Slower, but every call is
   public API and the temp file doubles as the exit-test artifact.

Start with (2) for the first passing run — it gives a human-readable payload for the
"compare the two payloads" exit test — then switch to (1) once the round trip is green.

### Order of operations

1. Read the selected organism: `UserControl.Instance.bibiteTarget`
   (`ManagementScripts/UserControl.cs:37`) or scan
   `BibiteTracker.instance.bibites`.
2. Note `body.id.id`, and record which other bibites hold it in
   `eggLayer.children` or `genes.parent1/parent2` (scan
   `BibiteTracker.instance.bibites` once).
3. Serialize → string. Write to the BepInEx log / a file.
4. `BibiteTracker.instance.bibites.Remove(body);` (keeps `nDeath` and the
   death-age quantiles clean).
5. `UnityEngine.Object.Destroy(body.gameObject);` — no corpse, no meat, no eggs.
   Precedent: `UIScripts/SelectionPanel.cs:310`.
6. `var go = SaveSystem.instance.LoadBibiteOrEggFromData(json, true, null, null);`
   Null-check — the method swallows all exceptions (`:76-79`).
7. Re-apply the links recorded in step 2, mirroring `SaveSystem.cs:748-782`:
   set `genes.parent1/parent2` GameObjects from the IDs, and substitute the new
   `BibiteID` component into any parent's `eggLayer.children`.
8. Serialize again and diff. Expect an exact match including `id`.

### Where to overwrite state (if a fallback path is used)

Only needed for the egg fallback. Postfix
`EggHatching.Hatch()` (`SimulationScripts.BibiteScripts/EggHatching.cs:172`) and
overwrite `__instance.newBornBody`. The cleanest overwrite is not field-by-field —
call `SerializationHelper.DeserializeObject(newBornBody, bodyJObject)` (public static,
`ScriptHelpers/SerializationHelper.cs:87`), which routes to `BibiteBody.LoadState` and
handles all the `internal` and private `[SerializeField]` members for you. Same for the
clock: `DeserializeObject(clock, clockJObject)` reaches `InternalClock.timeAlive`
without any mod-side reflection.

### Expected obstacles

1. **`LoadBibiteOrEggFromData` swallows exceptions** (`SaveSystem.cs:76-79`). A
   malformed payload returns `null` with no diagnostic. Mitigation: call
   `LoadBibite` directly by reflection during development
   (`AccessTools.Method(typeof(SaveSystem), "LoadBibite")`) to see the real stack, then
   switch to the public wrapper.
2. **Deferred destruction.** Unity's `Destroy` runs at end-of-frame, so the old
   GameObject and the respawned one coexist briefly. `body.destroyed` stays `false`
   until then. Do not assume `nBibite` drops immediately, and do not run the
   respawn from inside `OnDestroy`.
3. **Cross-organism stale references.** The two lists that `BibiteBody.OnDestroy` does
   *not* clean are the **parent's** `BibiteEggLayingOrgan.children`
   (`BibiteEggLayingOrgan.cs:65`) and the **child's** `BibiteGenes.parent1/parent2`
   (`BibiteGenes.cs:60, 65`). A stale `BibiteID` in `children` makes
   `BibiteEggLayingOrgan.SaveState` (`:235`) throw on the next world save — which will
   look like a corrupted save, not like an M1 bug. Repair in step 7, and include a
   world save in the exit test to catch it.

Lower-priority obstacles: a test subject latched to another bibite's mouth
(`BibiteMouth.cs:449-467` and `:710` have unguarded dereferences); `InternalClock`
private fields `agingProgress`, `period`, `counting` are not persisted (up to 120 s of
aging-cadence drift, cosmetic); and `NEATBrain.Node.NIn/NOut` are `[NonSerialized]` but
recomputed by `ResumeBrain()` → `ComputeActiveNeurons()`, so they are immaterial.

### Items to verify at runtime in M1

- ~~`Application.version` — confirm the `0.6.3.1` reading (§7.2).~~ **Done.** The plugin
  logs `Application.version = 0.6.3.1` at startup. The static reading was correct.
- Whether Newtonsoft's default contract resolver honours `[NonSerialized]` on
  `NEATBrain.Node.NIn/NOut` in this shipped `Newtonsoft.Json.dll`. **Still open** — the
  exit test compares a payload against a payload, so it passes either way. Immaterial to
  correctness (both are recomputed), but it determines the exact `.bb8` byte shape
  that `bb8-schema` must accept.
- ~~That the two serializations in the exit test are byte-identical.~~ **Done, equal.**
  See *Runtime results*. The predicted drift fields (`brainTicksCount` /
  `visionLookupCount` / `visionSensingCount`, `BibiteBody.cs:109-116`, and
  `clock.ticProgress`) did not drift: the organism does not exist during the one frame
  the round trip waits for `Destroy`, and the second capture happens immediately after
  the restore, before the respawned organism ticks. The two paths that did differ on the
  first run were `$.transform.scale` and `$.body.health`, for reasons unrelated to ticks.

---

## Runtime results — automated exit test, 2026-08-02

The exit test defined in `m1_considerations.md` ran unattended twice on 2026-08-02,
against game `0.6.3.1`. **Both runs passed.**

### The harness

`bibites-mod/src/AutoTest.cs`, armed either by the BepInEx config entry `[M1] AutoTest` or
by the environment variable `MULTIVERSE_AUTOTEST=1`. PowerShell `Start-Process` hands its
own environment to the game, so `game.sh` needed no change — but the WSL → Windows hop
does need `WSLENV`; see `dev_environment.md`. Phases:

1. Wait for the main menu.
2. Seed a world named `M1-AutoTest`, if it does not exist yet.
3. Load it with `GameManager.StartGame(path)` — the same call the Load Game menu makes
   (`UIScripts/LoadGamePanel.cs:225`).
4. Pick an adult with synapses and stomach content, not latched to another mouth.
5. Run the round trip; diff the payload before against the payload after.
6. Watch the respawned organism for 30 simulated seconds.
7. Save the world back to `M1-AutoTest`, then quit.

Nothing but `M1-AutoTest.zip` is read, written or deleted.

**The world has to be grown, not loaded.** A fresh install has zero user saves, and the
shipped `Scenarios/*.zip` archives carry settings only — no bibites. Phase 2 therefore
applies the stock scenario, starts a fresh simulation, runs it at 10× until an adult with
food in its stomach exists, and saves that as `M1-AutoTest`.

**The world save is driven by hand, and this is not optional.** `SaveSystem.SaveGame`
wraps `CreateSave` in `StartCoroutine`, and Unity's coroutine runner swallows exceptions.
The Risk-5 symptom — `BibiteEggLayingOrgan.SaveState` dereferencing a fake-null
`BibiteID` (§4.3, §5.3) — would therefore be **invisible**: no log line, no
`onSavingDone`, just a save that never appears. The harness reflects the private
`CreateSave` iterator out of `SaveSystem` and calls `MoveNext()` itself inside a `try`, so
a throwing save becomes a test failure instead of silence. Any future automated check of
"did the world still save?" must do the same.

### Run 1 — freshly seeded world: PASS

The round trip reported two differing payload paths. Both are float artifacts, not lost
state:

| Path | Cause |
|---|---|
| `$.transform.scale` | The serializer reads the **live transform**: `SerializedTransform(Transform)` takes `scale = transform.localScale.x` (`ScriptHelpers/SerializedTransform.cs:25`). `BibiteGrowth.SetMaturity` pushes a new size into the transform only when relative growth passes `sizeUpdateThreshold` = `0.005f` (`BibiteGrowth.cs:50, 127-128`), so a growing organism's `localScale` lags its `d2Size`. The restore path has no such lag — `ResumeBody()` calls `Set2DSize(d2Size, initialSet: true)` (`BibiteBody.cs:465`) and recomputes the scale exactly from the authoritative `d2Size`. **The respawned organism is more precise than the original.** The artifact appears only when the live scale happens to be stale at the moment of capture. |
| `$.body.health` | 1.2e-7 relative difference — a float → text → float round trip through the JSON payload. |

Neither is a fidelity loss, and neither needs a mod-side correction.

### Run 2 — world loaded from the seeded save: PASS, payloads EQUAL

- The payload before and the payload after were **equal token for token**. No differing
  paths at all, `$.transform.scale` included.
- Entity ID `-843827577` survived the destroy and the respawn.
- The organism was alive and healthy after 30 simulated seconds.
- The world save completed with no exception.
- **Zero** Unity errors, exceptions or assertions across the whole run.

Run 2 is the run that meets the exit test's stated bar, and it is the configuration a
migration will actually hit: an organism that came out of a save file, not one the
scenario spawner had just created.

### World-level APIs that proved out

| API | Result |
|---|---|
| `GameManager.StartGame(string saveToLoadPath)` (`GameManager.cs:77`) | Loads a named world from mod code, unattended, with no UI interaction. This is the M2 rig's world-loading call. |
| `SaveSystem.CreateSave` (private iterator), driven with `MoveNext()` | Saves the world *and* surfaces its exceptions. The public `SaveGame` wrapper does not. |
| `SaveSystem.SerializeBibite` (private static) + `LoadBibiteOrEggFromData` (public) | The full organism round trip, exactly as §1 and §2 predicted from the source. |

### Residual gap — the re-link code is implemented but not yet exercised

The organisms in the seeded world were spawned as adults by the scenario spawner. They had
no parents and no children. `OrganismLinks` therefore ran against an organism with **no
actual links**, and the stale-reference trap of §4.3 was never triggered.

The clean world save proves that this round trip left no new stale reference behind. It
does **not** prove that the repair code works, because there was nothing to repair. This
carries to M2 as an open risk: run the round trip on an organism with living parents and
living children, in an evolved world.

### Hazards in the game's own API

Found while building the harness. None is a mod defect, and each one will meet the M2 rig
as well. Environment-level hazards — deploying while the game runs, the missing Unity log,
scenario-folder timestamps — are in `dev_environment.md` instead.

| Hazard | Detail |
|---|---|
| `SaveSystem.SaveGame` hides save failures | `StartCoroutine(CreateSave(...))`. An exception inside the iterator is swallowed by Unity's coroutine runner. Drive `CreateSave` directly whenever the save result is the thing under test. |
| `SaveController.GetLastSave()` throws on a fresh profile | `(… .Concat(…) orderby …).First()` (`SaveController.cs:292-297`). `.First()` on an empty sequence throws, so `GameManager.Continue()` (`GameManager.cs:85`) is unsafe until at least one save exists. Do not use it to bootstrap a test. |
| Auto-save restarts the scene under an automated test | With `UserSettings.ReloadAfterAutosaves` on, `SaveController` subscribes `ReloadAfterAutoSave` to `onSavingDone` (`SaveController.cs:234, 240-248`); it calls `GameManager.StartGame` and reloads the whole scene mid-test. Call `SaveController.Instance.ToggleAutoSave(false)` (`:84`) once the world is ready. That call is runtime-only — it does not write the user's `AutoSave` setting. |
| Setting the time scale clears every pause | `TimeController.targetTimeScale.SetValue(x)` with `x > 0` routes through `ForcePlay()` (`TimeController.cs:97-100, 166-176`), which does `Pauses.Clear()` — it drops **all** pause sources, `PauseAfterLoad` included. Convenient for a test; destructive for a mod that must respect another system's pause. Use the refcounted `TogglePauseGame(source)` for anything else. |
| A reloaded world has no name | `CreateSave` never writes a `gameName` key. `SaveSystem.cs:646-648` reads one on load, so `SimulationManager.gameName` comes back empty for any world the game itself saved. Cosmetic, but do not identify a world by it. |
