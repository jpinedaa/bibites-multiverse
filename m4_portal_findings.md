# M4 Findings — A Visible Portal at the Migration Edges

Source: `decompiled/BibitesAssembly/` (ilspycmd output of
`The Bibites_Data/Managed/BibitesAssembly.dll`, Steam app 2736860, buildid 22383127,
game version `0.6.3.1`), plus two artefacts **outside** the repo:

- `ShapesRuntime.dll` from `The Bibites_Data/Managed/`, decompiled to a scratch directory
  (not committed). Reproduce with
  `ilspycmd -p -o <scratch> "…/The Bibites_Data/Managed/ShapesRuntime.dll"`.
  Shapes citations below are by **type and member**, not line number, because that
  decompile is not in the checkout.
- `The Bibites_Data/globalgamemanagers`, read with `strings`. This is where Unity stores
  the tag/layer table and the *Always Included Shaders* list, and it settles the single
  question that decides this whole milestone (§2.2).

Line numbers for `decompiled/` and `bibites-mod/src/` track this checkout and change when
`sync-game-refs.sh` regenerates the decompile. Items that could not be settled statically
are marked **UNCERTAIN**; nothing here was verified by running the game (the task
explicitly forbade it), so every "it renders" claim is a static-evidence claim.

This document answers the five M4 rendering questions. It does not repeat
`m2_findings.md` §2 (world geometry) or `m2_findings.md` §3 (edge behaviour); where a
geometry fact is needed it is cited to M2 rather than re-derived.

## Headline result

**Yes, qualified — and the qualification is smaller than expected.** The game ships
**Shapes** (Freya Holmér's vector-graphics library, `ShapesRuntime.dll`), it already
creates Shapes components **at runtime from code**
(`UIScripts.CustomGraphics/ShapesMask.cs:21-44` does `gameObject.AddComponent<Rectangle>()`
and configures it entirely through the public API), and all **154** `Shapes/*` shader
variants — every shape type × every blend mode, including `Additive` — are in the build's
*Always Included Shaders* list, so `Shader.Find` resolves them at runtime (§2.2). A mod
can therefore draw a glowing, gradient-filled, animated strip along an edge with **zero
shipped assets, zero custom shaders, and no asset-bundle loading**.

The three things that would have killed it are all absent:

| Feared blocker | Actual state |
|---|---|
| Shapes shaders stripped from the build | **Not stripped** — 154 variants in `globalgamemanagers` (§2.2) |
| Shapes' runtime `ScriptableObject`s missing | `Shapes Assets` and `Shapes Config` are both in `resources.assets`, which is exactly the `Resources.Load` path they use (§2.1) |
| Needs a custom shader for a glow | `ShapesBlendMode.Additive` + `Rectangle.UseFill` linear gradient does it (§3.1) |

The one real constraint is that **immediate mode (`Shapes.Draw.*`) will not work**: under
a scriptable render pipeline it needs a `ShapesRenderFeature` on the URP renderer, and the
mod cannot add one (§2.3). Retained mode — a `GameObject` with a `Shapes.Rectangle` /
`Shapes.Line` / `Shapes.Disc` component — needs no render feature and is what the game
itself uses.

The **native-look** option (reuse a game visual) is feasible but **not recommended as the
first build**: the zone's fertility material takes its colour from a *global* shader
property shared with every other zone (§3.3), so a portal built from it looks like — and
recolours with — the fertility overlay. That is the opposite of "a player can see the
portal".

| Question | Short answer |
|---|---|
| 1. How does the game render world visuals? | One orthographic `Camera.main` at `z = −855`, URP with `Renderer2D` present. Every world visual is a `SpriteRenderer` driven by a Shader-Graph shader; there is **no** `LineRenderer` anywhere in the assembly. Layer and sorting must be set explicitly — §1 |
| 2. Is Shapes usable from a mod? | **Yes.** Public MonoBehaviour API, shaders present, runtime-`AddComponent` precedent in-game — §2 |
| 3. Cheapest portal visual? | A `Shapes.Rectangle` per open edge: additive blend, linear-gradient alpha, thickness in **pixel** space so it stays legible at every zoom — §3.1 |
| 4. Per-event flourish? | **Yes, two ways.** `ParticlesMaster.Instance.EmitPheromonesAtPosition(pos, r, g, b)` is public, uses in-game particle assets, and is visual-only; or an expanding `Shapes.Disc` — §4 |
| 5. Culling/zoom gotchas? | Max zoom-out is `max(250, 2·S)` half-height, so the whole world is on screen at once and a 20-unit strip is ~2 px. Pixel-space thickness is the answer. Frustum culling is a non-issue (Shapes uses 65536-unit bounds). Layer/sorting is the real risk — §5 |

---

## 1. HOW THE GAME RENDERS WORLD-SPACE VISUALS

### 1.1 Two cameras, orthographic, URP

`CameraManager` grabs `Camera.main` (`ManagementScripts/CameraManager.cs:63`) and never
changes its culling mask — grep for `cullingMask` across the assembly returns nothing.
The camera sits at a fixed depth offset `new Vector3(0f, 0f, -855f)`
(`CameraManager.cs:34`), applied when following a target (`:114`) and when the player
presses Home (`:104`). It is orthographic: `cam.orthographicSize` is the zoom
(`:69, 123, 225`).

A second camera exists for UI only: `OneUseScripts/UICamera.cs` exposes
`public static Camera cam`, and every UI raycast/tooltip path uses it
(`UIScripts.UIReferences/TooltipHandle.cs:35`,
`UIScripts.UIPanels/BiologyPanel.cs:451`). World-space code uses `Camera.main`
(`SimulationScripts/PelletPlacer.cs:67`, `SimulationScripts/EffectTower.cs:42`,
`ManagementScripts/UserControl.cs:228, 420`).

The pipeline is URP: `ManagementScripts/SpriteGenerator.cs:55-71` hooks
`RenderPipelineManager.endCameraRendering`, and `The Bibites_Data/Managed/` ships
`Unity.RenderPipelines.Universal.Runtime.dll` and
`Unity.RenderPipelines.Universal.2D.Runtime.dll`. `globalgamemanagers.assets` contains the
type names `Renderer2D`, `Renderer2DData` and `DrawRenderer2DPass`, so the **URP 2D
renderer** is in the build. **UNCERTAIN:** whether the active renderer asset is the 2D
renderer or a forward renderer — the renderer asset's *contents* are binary and were not
decoded. It does not change the recommendation: no `Light2D` is used anywhere in the
assembly (grep returns nothing), so all world visuals are unlit and an unlit mod shader
behaves the same under either renderer.

### 1.2 The world layers

`globalgamemanagers` holds the tag/layer table. Tags: `pellet`, `bibite`, `egg`,
`pheros`, `FoodSource`, `Controller`, `EggSpawner`, `ToDestroy`, `VirusSpawner`,
`bibitePart`, `ColorKiller`, `zone`, `Tower`, `PheroTower`.

Layers, in table order (Unity omits empty slots from the string table, so the index
mapping is inferred and then cross-checked against code):

| Index | Name | Cross-check |
|---|---|---|
| 0 | `Default` | Unity reserved |
| 1 | `TransparentFX` | Unity reserved |
| 2 | `Ignore Raycast` | Unity reserved |
| 3 | `UIRenders` | |
| 4 | `Water` | Unity reserved |
| 5 | `UIPhysics` | |
| 6 | `SceneTriggers` | |
| 7 | `bibite` | `RedDeathBloomManager.cs:140` sets `obj.layer = 7` on the damage triggers that watch for `bibitePart` colliders; `SimulationScripts.BibiteScripts/FieldOfView.cs:183` resolves `LayerMask.NameToLayer("bibite")` |
| 8 | `pellets` | `FieldOfView.cs:184` |
| 9 | `pheromones` | `SimulationScripts.BibiteScripts/Pherosense.cs:9` `pheroMask` |
| 10 | `UIRenders2` | |
| 11 | `BackGround` | |
| 12 | `Recessed5` | |

**This is the single biggest practical risk for a mod-created renderer.** A new
`GameObject` is created on layer 0 (`Default`), and nothing in the decompiled source
proves that `Camera.main`'s culling mask includes layer 0. Do not guess — see §5.3 for the
mitigation (copy the layer off a live game renderer, and log the mask once).

Sorting layers appear to be a very short list (`Default`, `UIRenders`, …). Note that
`ShapesMask.cs:31` calls `SortingLayer.NameToID("UI")`, and **no sorting layer named `UI`
appears in the table** — `NameToID` returns 0 for an unknown name, which is the `Default`
sorting layer, so that line is effectively a no-op. **Do not rely on a named sorting
layer.** Stay on the default sorting layer and control depth with `SortingOrder` and `z`.

### 1.3 Every world visual is a `SpriteRenderer` plus a Shader-Graph shader

There is one consistent pattern, repeated for every world visual in the game:

| Visual | Renderer | Material handling |
|---|---|---|
| Background + grid + microscope shade | `SpriteRenderer backgroundSR` and a `GameObject microscopeShade` (`OneUseScripts/BackgroundManager.cs:12, 15`) | reads `backgroundSR.material` once (`:71`) and drives ~14 shader properties (`:19-49`); also publishes four **global** floats `_SimSize`, `_ShadeFadeDistance`, `_ShadeStart`, `_ShadeEnd` (`:105-112`) |
| Zone / fertility circle | `public SpriteRenderer sr` (`SimulationScripts/Zone.cs:23`) | clones a template `public Material zoneMaterial` (`:25`) per zone: `mat = Instantiate(zoneMaterial); sr.material = mat;` (`:112-113`); properties `_Ring`, `_Rect`, `_insideRadius`, `_Opacity`, `_PelletSpawnCoefficient` (`:29-37, 226-249`); shown/hidden by `sr.enabled` off `UserSettings.ShowFertility` (`:374-377`) |
| Red Death bloom | `public SpriteRenderer bloomSR` (`SimulationScripts.Events/RedDeathBloomManager.cs:13`) | same clone-on-init idiom (`:130-131`), one `_Fill` float (`:72, 188`), and the object is scaled to the **whole world**: `transform.localScale = 2f * BackgroundManager.shadeEnd * Vector3.one` (`:134`) |
| Rad tower area | `SpriteRenderer sr` (`OneUseScripts/RadioArea.cs:11`) | `mat = new Material(sr.material); sr.material = mat;` then `mat.SetColor("_Color", color)` with `public static Color color = new Color32(176, 255, 0, 128)` (`:22, 42-46`); radius via `transform.localScale = Vector3.one * (radius * 2f)` **and** a `_size` float (`:49-56`) |
| Bibite field of view | `SpriteRenderer sr` (`SimulationScripts.Rendering/FieldOfViewDisplay.cs:18`) | one `_viewAngle` float; radius via `localScale = 2f * viewRadius * Vector3.one` (`:69, 128`) |
| Selection highlight | swaps the target's `Renderer.material` for a shared `highlightMaterial` and sets `_HighlightColor`, `_focus`, `_pixelOffset` (`ManagementScripts/UserControl.cs:68, 279-289`) |

Shader names visible in `globalgamemanagers` confirm these are Shader Graph assets:
`Shader Graphs/microscopeShade`, `Shader Graphs/algeaBloom`, `Shader Graphs/UIEggShader`,
`Shader Graphs/Rainbow`.

Three consequences for a mod:

1. **The idiom is "clone a material, set floats".** A mod that wants a game-native look
   has to obtain a donor material from a live object; there is no `Resources` path to the
   game's materials in code.
2. **Scale, not size.** The game encodes radius as `localScale`, so its sprites are unit
   sized. A cloned zone sprite scaled to a `4000 × 40` strip is legitimate — `Zone`
   already does exactly that for rectangular zones (`Zone.cs:387-398`).
3. **There is no `LineRenderer` anywhere in the assembly** (grep returns zero files). If
   you use one you are on your own for material and sorting; nothing in the game
   establishes that it works here.

### 1.4 Will a mod-instantiated standard renderer show up?

Yes, subject to four constraints, none of which needs a custom shader:

| Constraint | What to do |
|---|---|
| **Layer** must be inside `Camera.main.cullingMask` | Copy it from a live world renderer rather than guessing — §5.3 |
| **Depth** — the camera sits at `z = −855` looking toward `+z` | Put the portal at `z = 0`. That is provably inside the clip range: zones land there because `Zone.UpdatePosition` assigns a `Vector2` to `transform.position` (`Zone.cs:451`), which zeroes `z`, and pellets sit at `z = +0.1` (`ManagementScripts/WorldObjectsSpawner.cs:83`) |
| **Material** — URP has no "default" for a bare `MeshRenderer` | Either use a Shapes component (brings its own material, §2) or clone a donor material (§3.3). A `new Material(Shader.Find(…))` on a *game* shader name is fine; on `"Sprites/Default"` it is **UNCERTAIN** whether that shader survived URP build stripping |
| **Sorting** | Default sorting layer, explicit `sortingOrder`. Nothing in the game sets a world-space sorting order in code, so the prefab values are unknown — **UNCERTAIN**; pick a clearly negative order to sit behind organisms, or a clearly positive one to sit in front, and tune once at runtime |

### 1.5 Runtime-created visuals have precedent in-game

Two, both directly transferable:

- `RedDeathBloomManager.Initialize` (`:136-148`) builds 32 `GameObject`s from scratch at
  runtime, parents them, sets `obj.layer = 7`, and adds components. That is the exact
  shape of what a portal builder does.
- `UIScripts.CustomGraphics/ShapesMask.Awake` (`:21-44`) creates a `GameObject`, sets its
  layer, and does `gameObject.AddComponent<Rectangle>()` — a **Shapes** component created
  from code and configured purely through public properties (`SortingLayerID`,
  `BlendMode`, `StencilComp`, `SortingOrder`, `Color`, `Width`, `Height`). This is the
  existence proof for §2.

---

## 2. THE BUNDLED SHAPES LIBRARY

### 2.1 What ships

`The Bibites_Data/Managed/` contains `ShapesRuntime.dll` (380 KB) and `ShapesSamples.dll`.
The runtime assembly decompiles to 124 files under namespace `Shapes`, including the
public MonoBehaviour shape renderers `Line`, `Polyline`, `Rectangle`, `Disc`, `Triangle`,
`Quad`, `Polygon`, `RegularPolygon`, `Sphere`, `Cone`, `Cuboid`, `Torus`, `TextElement`,
all deriving from the public abstract `Shapes.ShapeRenderer`.

The game already uses four of them at runtime:

| Type | Used by |
|---|---|
| `Rectangle` | `UIScripts.CustomGraphics/ShapesMask.cs:11-13`, `UIScripts.UIReferences/GraphDataBar.cs:12` |
| `Line` | `UIScripts.UIReferences/GraphBackgroundLineLabel.cs:15`, `UIScripts.UIReferences.Graphs/BaseLineGraph.cs:11` |
| `Disc` | `UIScripts.UIReferences/GraphDataLine.cs:18`, `UIScripts.UIReferences.Graphs/GraphPointsHandle.cs:9`, `.../DraggableGraphPointsHandle.cs:20` |
| `Polyline` | `PolylineRaycastProxy.cs:12`, `UIScripts.UIReferences/GraphDataLine.cs:15` |

Shapes loads two `ScriptableObject`s lazily through `Resources.Load`:
`ShapesAssets.Instance` → `Resources.Load<ShapesAssets>("Shapes Assets")` and
`ShapesConfig.Instance` → `Resources.Load<ShapesConfig>("Shapes Config")`. Both string
keys are present in `The Bibites_Data/resources.assets`, so those loads resolve. (Only
`ShapesAssets` matters for meshes, and only for 3D shapes; 2D shapes use the shared quad.)

### 2.2 The shaders are in the build — this is the decisive fact

`ShapeRenderer` obtains its materials through `Shapes.ShapesMaterials`, whose constructor
does `Shader.Find("Shapes/" + shaderName + " " + blendMode)` and logs
`"Could not find shader …"` on failure. `Shader.Find` at runtime only resolves shaders the
build actually contains. Shapes' importer registers its shaders in Unity's *Always
Included Shaders*, and that list lives in `globalgamemanagers`:

```
$ strings -n 6 ".../The Bibites_Data/globalgamemanagers" | grep -c '^Shapes/'
154
```

154 = 14 shape types × 11 blend modes. Every variant a portal could want is present, e.g.:

```
Shapes/Rect Opaque   Shapes/Rect Transparent   Shapes/Rect Additive   Shapes/Rect Screen …
Shapes/Line 2D Additive  Shapes/Disc Additive  Shapes/Polyline 2D Additive …
```

Corroborating negative evidence: the live `BepInEx/LogOutput.log` (97 MB, 355 391 lines,
covering every run of the mod so far) contains **no** occurrence of `shader` or `shapes`
in any case. `ShapesMaterials` static initialisation builds all 11 blend-mode materials
for every shape type the game touches; had any been stripped, the log would carry
`Could not find shader …` errors. It does not.

### 2.3 Retained mode works. Immediate mode almost certainly does not

Shapes has two modes:

- **Retained** — `AddComponent<Rectangle>()` etc. `ShapeRenderer` internally adds a
  `MeshFilter` + `MeshRenderer` (`ShapeRenderer.MakeSureComponentExists<T>`) and is
  rendered by the ordinary pipeline like any other mesh. **No render feature needed.**
  This is what the game uses.
- **Immediate** — `Shapes.Draw.Line(...)` inside an `ImmediateModeShapeDrawer.DrawShapes`
  override. `ImmediateModeShapeDrawer` subscribes to
  `RenderPipelineManager.beginCameraRendering`, fills `DrawCommand.cBuffersRendering`, and
  **relies on `Shapes.ShapesRenderFeature`** (a `ScriptableRendererFeature`) to drain that
  dictionary in `AddRenderPasses`. Without the feature on the active URP renderer, nothing
  is drawn.

`ShapesRenderFeature` appears in `globalgamemanagers.assets` only inside the assembly/type
mapping table (adjacent to `Shapes`, `ShapesRuntime`, `BibitesAssembly`), not demonstrably
as an instance on a renderer asset — and the game never calls `Draw.*` (grep for `Draw.`
against the Shapes namespace in the assembly finds nothing), so it had no reason to add
one. **UNCERTAIN** but the safe reading is: immediate mode is unavailable. It costs
nothing to avoid it — retained mode is a better fit for a portal anyway, since a portal is
a small number of long-lived shapes, not a per-frame draw list.

### 2.4 The API surface that matters

All public, all settable from a mod, all on `Shapes.ShapeRenderer` unless noted:

| Member | Why it matters for a portal |
|---|---|
| `Color`, `BlendMode` (`ShapesBlendMode.Additive`) | a glow without a custom shader |
| `SortingOrder`, `SortingLayerID`, `RenderQueue`, `ZTest`, `ColorMask` | depth control (§5.3) |
| `Culling` (`ShapeCulling.CalculatedLocal` / `SimpleGlobal`), `BoundsPadding`, `GetWorldBounds()` | frustum behaviour (§5.2) |
| `Rectangle.Width` / `.Height` / `.Pivot` (`RectPivot.Center` default) / `.Type` (`HardSolid`, `HardBorder`, `RoundedSolid`, `RoundedBorder`) / `.Thickness` / `.ThicknessSpace` | the strip itself, solid or as an outline |
| `Rectangle.UseFill`, `.FillType` (`LinearGradient` \| `RadialGradient`), `.FillSpace` (`Local` \| `World`), `.FillLinearStart` / `.FillLinearEnd`, `.FillColorStart` / `.FillColorEnd` | **the gradient that makes it read as a portal**: opaque at the map edge, transparent inward |
| `Rectangle.Dashed`, `.DashSize`, `.DashSpacing`, `.DashOffset`, `.DashType` (`Basic`, `Angled`, `Rounded`, `Chevron`), `.DashSpace` (`Meters`, `Relative`, `FixedCount`), `.DashSnap` | animation by incrementing one float; `Chevron` gives a *directional* pattern — outward on the export edge, inward on the import edge |
| `Line.Start` / `.End` / `.Thickness` / `.ThicknessSpace` / `.EndCaps` / `.ColorStart` / `.ColorEnd` (with `ColorMode`) | the single-line variant |
| `Disc.Type` (`Disc`, `Pie`, `Ring`, `Arc`), `.Radius`, `.RadiusInner`, `.Thickness`, `.ColorInner` / `.ColorOuter`, `.AngRadiansStart` / `.AngRadiansEnd` | the per-event expanding ring (§4.2) |

### 2.5 `ThicknessSpace.Pixels` is the reason to prefer Shapes

`Line`, `Rectangle` and `Disc` all expose `ThicknessSpace` with values
`Meters | Pixels | Noots`, and `Disc` also has `RadiusSpace`. **Pixel-space thickness is
resolved in the shader against `_ScreenParams`, so the shape keeps a constant on-screen
width at every zoom level.**

This matters more than it sounds. §5.1 shows the camera zooms from `orthographicSize = 5`
to `max(250, 2·S)` — a range of 800× at the default `S = 2000`. A portal drawn as a
20-world-unit-wide strip is ~2 px at full zoom-out (invisible) and taller than the screen
at full zoom-in (a wall of colour). Doing this with a `SpriteRenderer` means subscribing
to `CameraManager.onCameraSizeChange` (a `public static UnityEvent<float>`,
`CameraManager.cs:22`, fired at `:227`) and rescaling by hand — which is exactly what
`BackgroundManager.UpdateLinesAlpha` does for the grid (`:91, 143-169`). With Shapes it is
one enum value.

The good design is a **hybrid**: the strip's *length and inset* in world units (it must
line up with the real capture band, `BorderGeometry.BandInnerBoundary`,
`bibites-mod/src/BorderGeometry.cs:169-173`), its *edge line* in pixel units so it never
disappears.

---

## 3. OPTIONS FOR THE PORTAL VISUAL, CHEAPEST FIRST

| # | Option | Assets | Effort | Look | Main risk |
|---|---|---|---|---|---|
| A | Shapes `Rectangle`, additive + linear-gradient alpha | none | ~120 lines | distinctive, deliberately not-vanilla | layer/sorting (§5.3) |
| B | A + animation (`DashOffset`, colour pulse) | none | +20 lines | alive | none beyond A |
| C1 | Clone `Zone.zoneMaterial` onto a rect sprite | none | ~80 lines | *identical* to a fertility zone | **colour is a global** — cannot be recoloured reliably |
| C2 | Clone a `RadioArea` material | none | ~60 lines | native glow, recolourable | needs a **live Rad Tower** to clone from; usually none exists |
| D | Bare `SpriteRenderer` with a runtime-generated `Texture2D` | none | ~150 lines | flat | material/shader availability under URP is unproven |

### 3.1 Option A — a Shapes strip along the open edge (recommended)

**Sketch.** One `GameObject` per open edge, built once when the simulation scene has a
`Camera.main` and torn down on world unload.

```
portal = new GameObject("MultiversePortal_E");
portal.layer = <copied from a live world renderer — §5.3>;
portal.transform.position = new Vector3(S - W/2, 0f, 0f);   // E edge: centre of the strip
portal.transform.rotation = Quaternion.identity;

Rectangle r = portal.AddComponent<Rectangle>();
r.Type       = Rectangle.RectangleType.HardSolid;
r.Pivot      = RectPivot.Center;
r.Width      = W;            // strip depth,  BorderGeometry.W
r.Height     = 2f * S;       // full edge length
r.BlendMode  = ShapesBlendMode.Additive;
r.Color      = portalColour;                     // e.g. cyan for export, amber for import
r.UseFill    = true;                             // alpha ramp: solid at the map edge, 0 inward
r.FillType   = FillType.LinearGradient;
r.FillSpace  = FillSpace.Local;
r.FillLinearStart = new Vector3(-0.5f, 0f, 0f);  // inner face
r.FillLinearEnd   = new Vector3( 0.5f, 0f, 0f);  // map edge
r.FillColorStart  = new Color(c.r, c.g, c.b, 0f);
r.FillColorEnd    = new Color(c.r, c.g, c.b, 0.55f);
r.SortingOrder = -50;                            // behind organisms
```

Plus a thin bright `Line` right on `x = S`, `ThicknessSpace = Pixels`, `Thickness ≈ 3`,
additive — that is the part that stays visible when the player zooms out to see the whole
world.

**Geometry source.** `BorderGeometry` already owns everything needed and — importantly —
**subscribes to `SimulationSize` rather than snapshotting it**
(`bibites-mod/src/BorderGeometry.cs:61-78, 105-117`), with an `onChanged` callback. The
portal builder should hang off that same callback so the strip follows a live `S` change.
Use `BorderGeometry.S`, `.W` and `.BandInnerBoundary(edge)`
(`BorderGeometry.cs:33, 36-47, 169-173`). Aligning the visual to the *real* capture band
(`InCaptureBand`, `:159-166`) is what makes the portal honest instead of decorative.

**Which edges.** In M3 the mod has one export edge and one (usually different) entry edge
(`bibites-mod/src/MultiverseConfig.cs`, `ExportEdge` / `OpenEdge`). Two portals, two
colours, and the chevron dash direction (§3.2) tells the player which is which.

**Files.** New `bibites-mod/src/PortalVisual.cs` (a `MonoBehaviour` added from
`MultiversePlugin.Awake`, alongside `RoundTripCommand`, `DevCommands`, `WorldLoader`,
`MultiverseClient` — `MultiversePlugin.cs:28, 54, 78, 96`), a reference to
`ShapesRuntime.dll` added to `bibites-mod/libs/` and `BibitesMultiverse.csproj`, and one
line in `sync-game-refs.sh` so the DLL is re-copied after a game update.

**Risks.** (i) layer/culling mask — §5.3, mitigated by copying the layer; (ii) sorting
order relative to organisms is unknown statically — §5.3, mitigated by making it a config
value; (iii) `ShapesRuntime.dll` becomes a new build reference that must be kept in sync
with the game like the others (`dev_environment.md`, "libs/ and decompiled/ are copies of
one game build").

### 3.2 Option B — animation, still with no assets

Three independent motions, all one float per frame in `Update()`:

1. **Scrolling chevrons.** `r.Dashed = true; r.DashType = DashType.Chevron;
   r.DashSpace = DashSpace.Meters; r.DashSize = …; r.DashOffset += speed * dt;` — a
   *directional* flow. Positive speed on the export edge (organisms leaving), negative on
   the entry edge.
2. **Breathing alpha.** Lerp `FillColorEnd.a` on a sine. Use `Time.unscaledTime`, not
   `Time.time`: `Zone.FixedUpdate` bails out entirely when `Time.timeScale <= 0`
   (`Zone.cs:150-153`) but `CameraManager` keeps working off `Time.unscaledDeltaTime`
   (`CameraManager.cs:195, 207`), so the player still pans while paused. A portal that
   freezes when the game is paused reads as a bug.
3. **A flash on traffic.** Kick the alpha up on each export/import and decay it — a cheap
   way to make the portal *mean* something (§4.3).

The game also ships **LeanTween** (`LeanTween.Framework/LeanTween.cs`, a public static
API with `LeanTween.value`, `.alpha`, `.scale`, easing and ping-pong loops) if a tween is
preferred to a hand-rolled `Update`. It is already loaded, so using it costs nothing; a
four-line `Update` costs less still.

Scrolling *textures* are not available without shipping an asset, but chevron dashes read
the same way and cost nothing.

### 3.3 Option C — reusing a game visual for a native look

**C1: the zone material. Verdict: do not build this first.**

`Zone` exposes `public SpriteRenderer sr` and `public Material zoneMaterial`
(`Zone.cs:23, 25`), both reachable as `ZoneManager.instance.zones[i]`
(`ZoneManager.cs:18, 22`). A mod could clone `zoneMaterial`, set `_Rect = 1`
(`Zone.cs:37, 229`) and `_Opacity` (`:29, 239`), and scale a `SpriteRenderer` to the strip
— `Zone` itself renders rectangles this way (`Zone.cs:387-398`). Perfectly native.

Two problems:

1. **Colour is global.** The zone shader's colour comes from
   `Shader.SetGlobalColor(Shader.PropertyToID("_fertilityColor"), val)`
   (`UIScripts.SettingHandles/UserSettingsManager.cs:136, 211-213`), driven by
   `UserSettings.FertilityColor`. Nothing in `Zone` sets a per-material colour. So the
   portal would be *whatever colour the player's fertility overlay is*, and would change
   with it. A player cannot tell the portal from a fertility zone — which defeats the
   whole point. **UNCERTAIN:** a per-material `mat.SetColor("_fertilityColor", …)` *may*
   override the global for that material (Unity gives material values priority over
   globals). That is worth a five-minute runtime test, and if it works C1 becomes viable —
   but it is not something to design around unverified.
2. **A donor is needed.** `zones` can legitimately be empty (`ZoneManager.cs:54-56`
   regenerates from `ScenarioSettings.Instance.allZones`), so the mod needs a fallback.

Note this is a *visual-only* clone. Do **not** create a real `Zone` — `m2_findings.md`
§1.5 item 4 already ruled that out: a real zone persists to the save file, appears in the
Zones editor, spawns pellets and counts biomass.

**C2: the Rad Tower area.** `OneUseScripts/RadioArea.cs` is the better donor: it sets a
**per-material** `_Color` (`:22, 42-46`) and a `_size` float, giving a recolourable
glowing disc, and `RadioArea.color` is `public static` so its default is readable. The
blocker is availability: `RadioArea` only exists inside a player-placed Rad Tower
(`WorldObjectsSpawner.GenerateRadTower`, `:350-353`), and generating one is invasive — a
Rad Tower mutates organisms (`RadioArea.FixedUpdate` → `IncreaseMutationFactor`, `:59-80`)
and is an `ISaveable` (`EffectTower.cs:109-121`) that would end up in the save file. Not
worth it.

**C3: the background shader globals.** `BackgroundManager` publishes `_SimSize`,
`_ShadeStart`, `_ShadeEnd`, `_ShadeFadeDistance` as **global** shader floats
(`:105-112`). A mod could read `BackgroundManager.shadeStart` / `.shadeEnd` /
`.SimulationSize` (all `public static`, `:55, 63, 65`) to place a visual exactly where the
shade begins. Useful as *input*, not as a donor visual.

**Overall verdict on the native look:** achievable, but every route either loses colour
control (C1), needs a simulation-affecting donor object (C2), or gives nothing to draw
with (C3). And there is a design argument against it anyway: a portal is *not* a vanilla
concept, and making it indistinguishable from a fertility zone is a worse player
experience than a deliberately alien-looking rift. Build A, and keep C1 in reserve as a
"blend in" config option if the owner asks for it after seeing A.

### 3.4 Option D — the no-Shapes fallback

If `ShapesRuntime.dll` turns out to be unusable at runtime for a reason this static
analysis missed, the fallback is a `SpriteRenderer` with a runtime-generated
`Texture2D`/`Sprite` (an N×1 gradient strip, `Sprite.Create`), stretched with
`SpriteDrawMode.Tiled` or plain `localScale`. The open question there is which material to
give it — see §1.4. The cheapest safe answer is to clone a donor material off a live game
`SpriteRenderer` (§3.3) and accept its colour behaviour. Keep this in the back pocket; do
not build it first.

---

## 4. PER-EVENT FLOURISHES

Feasible, and one of the two options is already written and public.

### 4.1 `ParticlesMaster` — a ready-made burst using in-game particle assets

`ManagementScripts/ParticlesMaster.cs` is a singleton (`public static ParticlesMaster
Instance`, `:9`) holding four `ParticleSystem`s: `pheromoneSpawnerR/G/B` and
`bloodSpawner` (`:11-21`). Two public methods are directly usable:

```
ParticlesMaster.Instance.EmitPheromonesAtPosition(Vector3 pos, float r, float g, float b)   // :45-63
ParticlesMaster.Instance.BloodSplatterAtPosition(Vector3 pos, Vector2 vel, int n, float radius) // :65-68
```

`EmitPheromonesAtPosition` moves the master transform to `pos` and emits `(int)r`, `(int)g`
and `(int)b` particles from the red/green/blue systems. This is exactly how the game itself
uses it — `PheromoneSpot.FixedUpdate` calls it every 0.25 s (`SimulationScripts/PheromoneSpot.cs:124`)
— so a mod calling it is on a well-trodden path.

Crucially, **it is visual only**. Pheromone *sensing* goes through physics colliders on
the `pheromones` layer (`SimulationScripts.BibiteScripts/Pherosense.cs:9`), not through
these particles. Emitting them creates no chemistry and no simulation side effect.

Three caveats:

- It is gated on `UserSettings.ShowPheromones` (`ParticlesMaster.cs:23-30, 47`). If the
  player has pheromone display off, nothing appears. Silent, not broken — but the portal
  should not depend on it as its only feedback.
- The particles are literally the pheromone visuals, so a burst may be misread as "a
  pheromone cloud appeared". Mitigate by pairing it with the portal's own flash (§3.2
  item 3) so the two read as one event.
- `BloodSplatterAtPosition` is ungated but reads as death. Wrong semantics for a
  migration — an organism that leaves is not killed.

**UNCERTAIN:** whether the pheromone particle systems use world or local simulation space.
`RedDeathBloomManager.Initialize` explicitly sets `scalingMode` on its own system (`:132-133`)
but nothing sets simulation space for `ParticlesMaster`'s, and the value lives in the
prefab. If it were local space, moving the master transform would drag live particles.
Since the game moves that transform on every pheromone tick already, the risk is
effectively nil — but it is not *proven* from source.

### 4.2 An expanding Shapes ring

The zero-dependency alternative: spawn a `Shapes.Disc` with `Type = DiscType.Ring`,
`BlendMode = Additive`, `ThicknessSpace = Pixels`, then over ~0.4 s (unscaled) ramp
`Radius` from 0 to a few body lengths while ramping alpha to 0, and destroy it. Twenty
lines, no assets, no settings gate, and it composes with the portal's palette so the
player learns "cyan ring = something left here". `LeanTween.value` will drive it if a
tween is preferred.

### 4.3 Where to hook

The mod already has the two exact positions:

| Event | Hook | Position |
|---|---|---|
| **Export / vanish** | `MigrationExporter.DestroyForCustody` (`bibites-mod/src/MigrationExporter.cs:323-350`), immediately before `GameBridge.DestroyCleanly(body)` at `:345` | `body.transform.position` — the organism is still alive and placed at that instant |
| **Import / appear** | `MigrationImporter.Handle` (`bibites-mod/src/MigrationImporter.cs:264`), where `finalPosition` is already computed for the `phase=SPAWNED` log line at `:265-269` | `body.transform.position`, i.e. `BorderGeometry.EntryPoint(edge, entryPosition)` after re-assertion |

Both sites already log the event, so the effect is a one-line addition next to an existing
log call. A rejected export (`HandleNack`, `:353`) should deliberately *not* flash — the
organism stayed.

Two disciplines: the effect must be **fire-and-forget and exception-guarded** (a visual
must never be able to break custody transfer — wrap in `try/catch` and log at warning),
and it must be **rate-limited**, because M2 measured ~20–25 border-strip entries per
simulated hour (`m2_findings.md`, headline) but the sim can run at high time scale.

---

## 5. CULLING, SCALING AND ZOOM GOTCHAS

### 5.1 The zoom range, and what it means at the edge

`CameraManager.CameraZoom` (`CameraManager.cs:210-228`) clamps `orthographicSize`:

```
min = 5f
max = Mathf.Max(250f, 2f * simulationSize)      // :214-215
```

At the default `S = 2000` (`m2_findings.md` §2.1, runtime-confirmed) the maximum
half-height is **4000** — twice the playable half-extent. **So yes: an object at
`x = ±2000` renders when zoomed out; at full zoom-out the entire playable square plus most
of the shade is on screen at once.** There is no distance-based culling of far-edge
objects.

The corollary is the legibility problem of §2.5, not a visibility problem: at
`orthographicSize = 4000` on a 1080-tall window, one world unit is ~0.135 px. A 20-unit
strip is 2.7 px; the map is 540 px tall. At `orthographicSize = 5` the same strip is
2160 px — the whole screen. Pixel-space thickness for the edge line, world-space geometry
for the band.

`CameraManager` publishes two public statics a mod can use for zoom-reactive behaviour:
`public static UnityEvent OnCameraZoomChange` and
`public static UnityEvent<float> onCameraSizeChange` (`:20, 22`), both fired on every zoom
(`:226-227`), plus `public static float camSize` (`:24`). `BackgroundManager` uses the
latter to fade the grid (`:91, 143-169`) — the same trick works to fade the portal's fill
in close and leave only the line far out.

### 5.2 Frustum culling — a non-issue

Shapes deliberately defeats it. `ShapesConfig` sets `boundsSizeQuad = 65536f` with the
comment *"practically 'turning off' frustum culling"*, and `ShapeRenderer.UpdateBounds`
either writes those bounds onto the shared mesh or sets `rnd.localBounds` from
`GetBounds()` when `Culling == ShapeCulling.CalculatedLocal` — which for a `Rectangle`
derives from its own `Width`/`Height` and is therefore correct for a 4000-unit strip. If
anything ever pops, `ShapeRenderer.BoundsPadding` and `ShapeCulling.SimpleGlobal` are the
escape hatches.

For a `SpriteRenderer` fallback, bounds come from the sprite × `localScale` and are also
correct. The only classic Unity trap — a giant *thin* mesh with default bounds — does not
arise here.

The game does have a real cull at world scale, but it is for **pellets**, not renderers:
`ZoneManager.RefreshPelletBiomassCounter` destroys pellets beyond `1.5·S + 500`
(`ZoneManager.cs:216, 232`). It touches only `MatterPellet` objects under the pellet
holders. A portal at `|x| = S` is nowhere near it and is not a pellet.

`ZoneManager.OnDrawGizmos` (`:198-206`) draws the world wire-cube and zone spheres — this
is **editor-only** and never appears in a player build. Do not model the portal on it.

### 5.3 Layer, culling mask and depth — the actual risk

This is where a portal implementation is most likely to fail silently, and the decompiled
source cannot settle it because layers live in prefabs.

**Mitigation, in order:**

1. **Copy the layer from a live world renderer** instead of guessing. Best donor:
   `ZoneManager.instance.zones[0].sr.gameObject.layer` (`Zone.cs:23`) — a zone's fertility
   sprite is definitively drawn by the world camera when *Show Fertility* is on. Fall back
   to `WorldObjectsSpawner.Instance.bibiteHolder.layer`
   (`ManagementScripts/WorldObjectsSpawner.cs:68`), then to layer 0.
2. **Log `Camera.main.cullingMask` once** at portal construction, next to the existing
   startup logging in `MultiversePlugin.Awake` (`:24-26`). One line turns an invisible
   failure into a diagnosable one.
3. **Place at `z = 0`** — proven in range, since zones land there (`Zone.cs:451`, `Vector2`
   → `Vector3` zeroes `z`) and the camera is at `z = −855` (`CameraManager.cs:34`).
4. **Make `sortingOrder` and `z` config values.** Nothing in the assembly sets a
   world-space sorting order in code, so the prefab values for bibites, pellets and zones
   are **UNCERTAIN**. A `[M4] PortalSortingOrder` config entry turns a guess into a tuning
   knob.

### 5.4 Pause, time scale and teardown

- Animate on `Time.unscaledTime`. `Time.timeScale` reaches 0 whenever the player pauses
  (`Zone.FixedUpdate` bails at `:150-153`), but the camera keeps working off unscaled time
  (`CameraManager.cs:195, 207`).
- The BepInEx plugin `GameObject` survives scene changes; a portal built against a
  specific world must be destroyed on world unload. The mod already has the notion — see
  `MigrationExporter.NoteUnresolvedAtUnload` (`:434`). Build the portal only when
  `Camera.main != null` **and** the simulation singletons are up (the mod's existing
  readiness check, `bibites-mod/src/GameBridge.cs:278-282`).
- Destroy any material or mesh the portal instantiates. Every game class that clones a
  material destroys it in `OnDestroy` (`Zone.cs:488`, `RedDeathBloomManager.cs:276`), and
  `ZoneManager` runs `Resources.UnloadUnusedAssets()` every 600 s (`:180-188`). Shapes
  handles its own instanced materials (`ShapeRenderer.TryDestroyInstancedMaterials`), so
  option A has nothing to clean up beyond the `GameObject`.

---

## Recommended approach

**Build option A + B first: a Shapes strip with a pixel-width edge line and a scrolling
chevron dash, one per open edge, driven off `BorderGeometry`.**

Why this one:

1. **It is the only option whose every dependency is proven from static evidence.** The
   library ships (§2.1), the shaders are in the build (§2.2 — 154 variants, all blend
   modes), the runtime `ScriptableObject`s resolve from `resources.assets`, and the game
   itself creates Shapes components from code at runtime (`ShapesMask.cs:29`). Nothing has
   to be shipped, imported, bundled, or shader-compiled.
2. **It solves the zoom problem that would otherwise dominate.** An 800× zoom range
   (§5.1) makes any fixed-world-size marker either invisible or overwhelming.
   `ThicknessSpace.Pixels` is a one-enum answer that no other option has; the
   `SpriteRenderer` routes all need a hand-written `onCameraSizeChange` rescale.
3. **It can be honest about the real geometry.** The strip is drawn from the same
   `BorderGeometry.W` and `BandInnerBoundary(edge)` the capture logic uses
   (`BorderGeometry.cs:36-47, 159-173`), and follows a live `SimulationSize` change through
   the existing `onChanged` callback (`:105-117`). The visual and the mechanic cannot
   drift apart.
4. **It carries the meaning the owner actually wants.** Two edges, two colours, and
   chevrons that flow *out* on the export edge and *in* on the entry edge — a player learns
   the topology by looking at it. A colour-locked clone of the fertility overlay (option
   C1) cannot say any of that.

Ship it with the per-event flourish from §4.2 (an expanding additive `Disc`, matched to
the portal's palette) hooked at `MigrationExporter.cs:345` and `MigrationImporter.cs:264`,
guarded and rate-limited. Prefer the Shapes ring over
`ParticlesMaster.EmitPheromonesAtPosition` as the *primary* flourish, because the particle
route is silently disabled when the player turns pheromone display off
(`ParticlesMaster.cs:47`) and reads as pheromones rather than as a migration; keep the
particle call as an optional extra garnish.

**What to verify in the first runtime session, in this order** — each is a one-line log or
a config flip, and the first two are the only ones that can invalidate the plan:

1. Does anything render at all? Log `Camera.main.cullingMask`, the donor layer, and
   `portal.GetComponent<Rectangle>().GetWorldBounds()`.
2. Is `Shader.Find("Shapes/Rect Additive")` non-null at runtime? (Static evidence says yes;
   this is the cheap confirmation.)
3. Sorting order relative to bibites and pellets — tune `PortalSortingOrder`.
4. Legibility sweep at `orthographicSize` 5, 250, 2000 and 4000.
5. Then, and only then, if the owner wants the native look: test whether
   `mat.SetColor("_fertilityColor", …)` on a cloned `Zone.zoneMaterial` overrides the
   global (§3.3 C1). If it does, offer C1 as a "blend in" config option.

## Open uncertainties, ranked

1. **Layer and culling mask (§1.2, §5.3).** The one thing that can make the portal
   invisible with no error. Not resolvable from the assembly — layers live in prefabs.
   Mitigated, not eliminated, by copying a donor layer. Costs one runtime session.
2. **World-space sorting order of bibites / pellets / zones (§5.3 item 4).** Determines
   whether the portal sits behind organisms (wanted) or over them. Unknowable statically;
   make it configurable.
3. **Whether the URP renderer carries `ShapesRenderFeature` (§2.3).** Only matters if
   someone tries immediate mode. The recommendation avoids it entirely, so this is
   informational.
4. **Whether a per-material `_fertilityColor` overrides the global (§3.3 C1).** Gates the
   native-look option, not the recommended one.
5. **`ParticlesMaster`'s particle simulation space (§4.1).** Low risk — the game moves that
   transform on every pheromone tick already — but unproven from source.
6. **Camera near/far clip planes.** Not in code; live in the scene. Placing the portal at
   `z = 0` sidesteps it, because the game already renders zones and organisms at that
   depth.
