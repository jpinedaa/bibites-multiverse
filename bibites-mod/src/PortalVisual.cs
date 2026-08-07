using System;
using System.Collections.Generic;
using System.Globalization;
using ManagementScripts;
using Shapes;
using SimulationScripts;
using UnityEngine;

namespace BibitesMultiverse
{
    /// <summary>
    /// The portal: a visible rift on every edge organisms actually cross
    /// (m4_considerations.md WP7, built from `m4_portal_findings.md`'s Recommended approach).
    ///
    /// **Why Shapes and not a sprite.** The game ships `ShapesRuntime.dll`, all 154 `Shapes/*` shader
    /// variants are in the build's *Always Included Shaders* list, and the game itself creates Shapes
    /// components from code at runtime (`UIScripts.CustomGraphics/ShapesMask.Awake`). So the portal
    /// needs no shipped asset, no custom shader and no asset bundle. The decisive property is
    /// `ThicknessSpace.Pixels`: the camera zooms across an 800× range
    /// (`orthographicSize` 5 to `max(250, 2·S)`), so any fixed-world-width marker is either invisible
    /// or a wall of colour. A pixel-space edge line is one enum value where a `SpriteRenderer` would
    /// need a hand-written `onCameraSizeChange` rescale.
    ///
    /// **Retained mode only.** `Shapes.Draw.*` needs a `ShapesRenderFeature` on the URP renderer and a
    /// mod cannot add one. A `GameObject` with a `Rectangle` / `Line` / `Disc` component needs no
    /// render feature, and is what the game uses.
    ///
    /// **The visual cannot drift from the mechanic.** The strip is laid out from the same
    /// <see cref="BorderGeometry"/> the capture band is tested against — `S`, `W` and
    /// `BandInnerBoundary` — and it re-lays out from the live `SimulationSize` callback. A portal that
    /// showed a band the exporter does not use would be worse than no portal.
    ///
    /// **Two lanes per edge, because every edge now carries both flows** (D17, `contract-a.md` §18
    /// A38). An edge is no longer one role: it exports *and* receives, so one strip per edge cannot say
    /// what the edge does, and two strips stacked on the same world coordinates would only be two
    /// additive rectangles fighting and two chevron lines exactly coincident. The border is therefore
    /// **tiled**, not overlapped:
    ///
    /// * the **outer lane**, `[S − W, S]`, is the real capture band — cyan, chevrons running one way
    ///   along the rim, its line on the map edge itself. Organisms leave from here;
    /// * the **inner lane**, `[S − W − margin, S − W]`, is the arrival inset — amber, chevrons running
    ///   the other way, its line on the arrival plane, which is exactly where
    ///   <see cref="BorderGeometry.EntryPoint"/> puts an arriving organism.
    ///
    /// Nothing overlaps, both lanes are visible at every zoom, and the two chevron flows read as one
    /// two-way lane rather than as a contradiction. It is also still true to the mechanic: each lane is
    /// drawn from the same numbers the pipeline uses, and the boundary between them is the band line an
    /// arrival must cross under its own power before it can be captured.
    ///
    /// **What decides whether a lane is drawn.** The capture lane is visible exactly while that edge is
    /// open for export (`EDGE_STATUS`, or the dev override). The arrival lane is visible exactly while
    /// the sidecar connection is up, because an arrival is never gated by edge state (§18, A38: `open`
    /// governs exports only) but nothing can arrive at all while the mod is disconnected.
    ///
    /// Risk 8 is the reason for the one loud line at build time: layers live in prefabs, not in the
    /// assembly, so nothing static settles whether the camera draws layer 0. The mitigation is to copy
    /// the layer off a live zone renderer and to **log the culling mask**, which turns an invisible
    /// failure into a diagnosable one.
    ///
    /// Every line carries the grepable prefix <c>[M4-PORTAL]</c>.
    /// </summary>
    internal class PortalVisual : MonoBehaviour
    {
        internal const string Prefix = "[M4-PORTAL]";

        /// <summary>Capture-lane cyan. Deliberately not a vanilla colour: a portal is not a vanilla concept.</summary>
        private static readonly Color ExportColour = new Color(0.25f, 0.90f, 1.00f, 1f);

        /// <summary>Arrival-lane amber.</summary>
        private static readonly Color EntryColour = new Color(1.00f, 0.65f, 0.20f, 1f);

        private const float FillAlpha = 0.55f;
        private const float LineAlpha = 0.90f;
        private const float LinePixelThickness = 3f;
        private const float DashRelativeSize = 3f;

        /// <summary>Relative dash units per unscaled second. Positive flows counter-clockwise around the rim.</summary>
        private const float DashSpeed = 6f;

        private const float FlourishSeconds = 1.0f;
        private const float FlourishPixelThickness = 4f;
        private const int FlourishPoolSize = 8;
        private const float FlourishMinIntervalSeconds = 0.05f;

        internal static PortalVisual Active;

        private sealed class Strip
        {
            internal Edge edge;
            internal bool export;
            internal GameObject fillObject;
            internal GameObject lineObject;
            internal Rectangle fill;
            internal Line line;
            internal bool visible;
        }

        private sealed class Flourish
        {
            internal GameObject go;
            internal Disc disc;
            internal float startedAt;
            internal bool active;
            internal Color colour;
            internal float maxRadius;
        }

        private MultiverseConfig config;
        private BorderGeometry geometry;
        private Func<Edge, bool> exportEdgeOpen;
        private Func<bool> connected;

        private readonly List<Strip> strips = new List<Strip>();
        private readonly Flourish[] flourishes = new Flourish[FlourishPoolSize];

        private GameObject root;
        private int layer;
        private bool built;
        private bool buildFailed;
        private bool wanted;
        private float lastFlourishAt;
        private long flourishesDrawn;
        private long flourishesDropped;

        internal void Initialize(
            MultiverseConfig configuration,
            BorderGeometry borderGeometry,
            Func<Edge, bool> isExportEdgeOpen,
            Func<bool> isConnected)
        {
            config = configuration;
            geometry = borderGeometry;
            exportEdgeOpen = isExportEdgeOpen;
            connected = isConnected;
            Active = this;
        }

        private void OnDestroy()
        {
            if (Active == this)
            {
                Active = null;
            }

            Teardown();
        }

        /// <summary>The world is loaded: the portal may be built as soon as a camera and the zones exist.</summary>
        internal void OnWorldLoaded()
        {
            wanted = config != null && config.Portal;
            buildFailed = false;
        }

        /// <summary>
        /// The BepInEx plugin GameObject survives a scene change, so a portal built against one world
        /// must be destroyed with it (m4_portal_findings.md §5.4).
        /// </summary>
        internal void OnWorldUnloaded()
        {
            wanted = false;
            Teardown();
        }

        /// <summary>A live `SimulationSize` change moves the real capture band, so it moves the strip too.</summary>
        internal void Relayout()
        {
            if (!built)
            {
                return;
            }

            foreach (Strip strip in strips)
            {
                try
                {
                    Layout(strip);
                }
                catch (Exception e)
                {
                    MultiversePlugin.Log.LogWarning($"{Prefix} could not re-lay out the {ContractA.EdgeName(strip.edge)} strip: {e.Message}");
                }
            }

            MultiversePlugin.Log.LogInfo(
                $"{Prefix} event=RELAYOUT S={geometry.S.ToString("F1", CultureInfo.InvariantCulture)} " +
                $"W={geometry.W.ToString("F1", CultureInfo.InvariantCulture)} strips={strips.Count}");
        }

        private void Update()
        {
            try
            {
                Pump();
            }
            catch (Exception e)
            {
                // A visual must never be able to break the simulation. One report, then it stays quiet.
                MultiversePlugin.Log.LogError($"{Prefix} the portal update threw, and the portal is torn down — {e}");
                buildFailed = true;
                Teardown();
            }
        }

        private void Pump()
        {
            if (!wanted || buildFailed)
            {
                return;
            }

            if (!built)
            {
                if (!GameBridge.SimulationReady() || Camera.main == null)
                {
                    return;
                }

                Build();
                if (!built)
                {
                    return;
                }
            }

            // Time.unscaledTime, never Time.time: the player can pan and zoom while paused
            // (CameraManager runs on unscaled time), and a portal that freezes on pause reads as a bug.
            float now = Time.unscaledTime;

            foreach (Strip strip in strips)
            {
                bool show = strip.export ? IsExportOpen(strip.edge) : IsConnected();
                if (show != strip.visible)
                {
                    strip.visible = show;
                    strip.fillObject.SetActive(show);
                    strip.lineObject.SetActive(show);
                    MultiversePlugin.Log.LogInfo(
                        $"{Prefix} event={(show ? "SHOWN" : "HIDDEN")} edge={ContractA.EdgeName(strip.edge)} " +
                        $"lane={(strip.export ? "capture" : "arrival")}");
                }

                if (!show)
                {
                    continue;
                }

                // The chevrons run **along** the rim, counter-clockwise as the line is oriented, and the
                // sign of the flow is what separates the two lanes of one edge: the capture lane runs one
                // way, the arrival lane the other (m4_portal_findings.md §3.2 item 1). Under D17 both are
                // drawn on every edge, which is the point — a two-way lane looks like two-way traffic.
                strip.line.DashOffset = (strip.export ? 1f : -1f) * DashSpeed * now;
            }

            PumpFlourishes(now);
        }

        private bool IsExportOpen(Edge edge)
        {
            try
            {
                return exportEdgeOpen != null && exportEdgeOpen(edge);
            }
            catch (Exception)
            {
                return false;
            }
        }

        private bool IsConnected()
        {
            try
            {
                return connected != null && connected();
            }
            catch (Exception)
            {
                return false;
            }
        }

        // ---- construction -------------------------------------------------------------------------

        private void Build()
        {
            layer = DonorLayer(out string donor);

            root = new GameObject("MultiversePortal");
            root.transform.position = Vector3.zero;
            root.layer = layer;

            // Two lanes on every edge that has both roles, and only the lane an edge actually has when
            // it does not: an undeclared export edge still receives, so it gets an arrival lane and no
            // capture lane (§18, A38, A41 case 3).
            foreach (Edge edge in config.ExportEdges)
            {
                strips.Add(BuildStrip(edge, export: true));
            }

            foreach (Edge edge in config.BorderEdges)
            {
                strips.Add(BuildStrip(edge, export: false));
            }

            built = true;

            // Risk 8, in the order m4_portal_findings.md gives: the layer, the mask, the shaders, the
            // bounds. One line turns "nothing renders and nothing errors" into a diagnosis.
            Camera camera = Camera.main;
            int mask = (camera != null) ? camera.cullingMask : 0;
            bool drawn = camera != null && (mask & (1 << layer)) != 0;
            string bounds = "<none>";
            if (strips.Count > 0 && strips[0].fill != null)
            {
                bounds = strips[0].fill.GetWorldBounds().ToString();
            }

            MultiversePlugin.Log.LogInfo(
                $"{Prefix} event=BUILT layer={layer} layerName='{LayerMask.LayerToName(layer)}' donor={donor} " +
                $"cameraCullingMask=0x{mask:X8} maskIncludesLayer={drawn} " +
                $"cameraOrthographic={(camera != null ? camera.orthographic.ToString() : "<no camera>")} " +
                $"cameraSize={(camera != null ? camera.orthographicSize.ToString("F1", CultureInfo.InvariantCulture) : "-")} " +
                $"shaderRectAdditive={Found("Shapes/Rect Additive")} shaderLineAdditive={Found("Shapes/Line 2D Additive")} " +
                $"shaderDiscAdditive={Found("Shapes/Disc Additive")} " +
                $"sortingOrder={config.PortalSortingOrder} flourishes={config.PortalFlourishes} " +
                $"S={geometry.S.ToString("F1", CultureInfo.InvariantCulture)} W={geometry.W.ToString("F1", CultureInfo.InvariantCulture)} " +
                $"captureLanes=[{ContractA.EdgeNames(config.ExportEdges)}] arrivalLanes=[{ContractA.EdgeNames(config.BorderEdges)}] " +
                $"entryDepth={EntryDepth().ToString("F1", CultureInfo.InvariantCulture)} " +
                $"firstStripWorldBounds={bounds}");

            if (!drawn)
            {
                MultiversePlugin.Log.LogWarning(
                    $"{Prefix} Camera.main's culling mask 0x{mask:X8} does not include layer {layer} — the portal is built and " +
                    "will render nothing. This is Risk 8: layers live in prefabs, so no static evidence settles it. " +
                    "Pick another donor layer.");
            }
        }

        private static string Found(string shaderName)
        {
            try
            {
                return (Shader.Find(shaderName) != null) ? "found" : "MISSING";
            }
            catch (Exception e)
            {
                return "threw:" + e.GetType().Name;
            }
        }

        /// <summary>
        /// m4_portal_findings.md §5.3 item 1 — copy the layer from a live world renderer rather than
        /// guessing. A zone's fertility sprite is definitively drawn by the world camera; the bibite
        /// holder is the fallback; layer 0 is the last resort.
        /// </summary>
        private static int DonorLayer(out string donor)
        {
            try
            {
                ZoneManager zones = ZoneManager.instance;
                if (zones != null && zones.zones != null)
                {
                    foreach (Zone zone in zones.zones)
                    {
                        if (zone != null && zone.sr != null)
                        {
                            donor = "zone.sr";
                            return zone.sr.gameObject.layer;
                        }
                    }
                }
            }
            catch (Exception e)
            {
                MultiversePlugin.Log.LogWarning($"{Prefix} could not read a zone renderer's layer: {e.Message}");
            }

            try
            {
                WorldObjectsSpawner spawner = WorldObjectsSpawner.Instance;
                if (spawner != null && spawner.bibiteHolder != null)
                {
                    donor = "bibiteHolder";
                    return spawner.bibiteHolder.layer;
                }
            }
            catch (Exception e)
            {
                MultiversePlugin.Log.LogWarning($"{Prefix} could not read the bibite holder's layer: {e.Message}");
            }

            donor = "default(0)";
            return 0;
        }

        private Strip BuildStrip(Edge edge, bool export)
        {
            Color colour = export ? ExportColour : EntryColour;

            Strip strip = new Strip { edge = edge, export = export };

            // The role is in the name because an edge now carries two lanes, and two GameObjects called
            // MultiversePortal_E_fill would be unreadable in the hierarchy.
            string label = ContractA.EdgeName(edge) + (export ? "_capture" : "_arrival");

            strip.fillObject = new GameObject("MultiversePortal_" + label + "_fill");
            strip.fillObject.transform.SetParent(root.transform, worldPositionStays: false);
            strip.fillObject.layer = layer;
            strip.fill = strip.fillObject.AddComponent<Rectangle>();
            strip.fill.Type = Rectangle.RectangleType.HardSolid;
            strip.fill.Pivot = RectPivot.Center;
            strip.fill.BlendMode = ShapesBlendMode.Additive;
            strip.fill.Color = colour;
            strip.fill.UseFill = true;
            strip.fill.FillType = FillType.LinearGradient;
            strip.fill.FillSpace = FillSpace.Local;
            strip.fill.FillColorStart = new Color(colour.r, colour.g, colour.b, 0f);
            strip.fill.FillColorEnd = new Color(colour.r, colour.g, colour.b, FillAlpha);
            strip.fill.SortingOrder = config.PortalSortingOrder;
            strip.fill.Culling = ShapeCulling.SimpleGlobal;

            strip.lineObject = new GameObject("MultiversePortal_" + label + "_line");
            strip.lineObject.transform.SetParent(root.transform, worldPositionStays: false);
            strip.lineObject.layer = layer;
            strip.line = strip.lineObject.AddComponent<Line>();
            strip.line.Geometry = LineGeometry.Flat2D;
            strip.line.BlendMode = ShapesBlendMode.Additive;
            strip.line.Color = new Color(colour.r, colour.g, colour.b, LineAlpha);
            strip.line.EndCaps = LineEndCap.None;

            // The one property that answers the 800× zoom range: a pixel-space thickness is resolved in
            // the shader against _ScreenParams, so the line keeps its on-screen width at every zoom.
            strip.line.ThicknessSpace = ThicknessSpace.Pixels;
            strip.line.Thickness = LinePixelThickness;

            // Relative dashes are sized against that same pixel thickness, so the chevrons stay a
            // constant on-screen size too — which is the whole reason the thickness is in pixels.
            strip.line.Dashed = true;
            strip.line.DashType = DashType.Chevron;
            strip.line.DashSpace = DashSpace.Relative;
            strip.line.DashSnap = DashSnapping.Off;
            strip.line.MatchDashSpacingToSize = true;
            strip.line.DashSize = DashRelativeSize;
            strip.line.SortingOrder = config.PortalSortingOrder + 1;
            strip.line.Culling = ShapeCulling.SimpleGlobal;

            Layout(strip);

            strip.visible = false;
            strip.fillObject.SetActive(false);
            strip.lineObject.SetActive(false);
            return strip;
        }

        /// <summary>
        /// One lane of one edge, laid out from the same numbers the pipeline uses. The capture lane runs
        /// from `BandInnerBoundary(edge)` to the map edge; the arrival lane runs inward from that same
        /// band line, so its inner face lands on the arrival plane `S − (W + entryMargin)` — exactly
        /// where <see cref="BorderGeometry.EntryPoint"/> puts an organism. The two tile the border and
        /// never overlap.
        ///
        /// The gradient runs from transparent at the lane's inner face to opaque at its outer face, so
        /// the rift reads as a hole in the world rather than as a painted line, and the chevron line
        /// sits on the face that means something: the map edge for a capture lane, the arrival plane for
        /// an arrival lane.
        /// </summary>
        private void Layout(Strip strip)
        {
            float s = geometry.S;
            float w = geometry.W;

            // Magnitudes first — the sign is the edge's side, and applying it once keeps W and S honest
            // on the negative edges instead of mirroring the whole layout by hand.
            float laneOuter = strip.export ? s : Mathf.Max(0f, s - w);
            float depth = strip.export ? w : EntryDepth();
            depth = Mathf.Clamp(depth, 1f, Mathf.Max(1f, laneOuter));
            float laneInner = laneOuter - depth;

            bool positiveSide = strip.edge == Edge.N || strip.edge == Edge.E;
            bool horizontalEdge = strip.edge == Edge.N || strip.edge == Edge.S;
            float sign = positiveSide ? 1f : -1f;
            float centreFixed = sign * (laneOuter - 0.5f * depth);

            // The chevron line marks the face that carries the lane's meaning.
            float lineFixed = sign * (strip.export ? laneOuter : laneInner);

            if (horizontalEdge)
            {
                strip.fillObject.transform.position = new Vector3(0f, centreFixed, 0f);
                strip.fill.Width = 2f * s;
                strip.fill.Height = depth;
                strip.fill.FillLinearStart = new Vector3(0f, -sign * 0.5f * depth, 0f);
                strip.fill.FillLinearEnd = new Vector3(0f, sign * 0.5f * depth, 0f);
            }
            else
            {
                strip.fillObject.transform.position = new Vector3(centreFixed, 0f, 0f);
                strip.fill.Width = depth;
                strip.fill.Height = 2f * s;
                strip.fill.FillLinearStart = new Vector3(-sign * 0.5f * depth, 0f, 0f);
                strip.fill.FillLinearEnd = new Vector3(sign * 0.5f * depth, 0f, 0f);
            }

            // Every edge is oriented counter-clockwise, so the chevron flow of one lane is visibly the
            // opposite of the other's on the same edge.
            strip.lineObject.transform.position = Vector3.zero;
            switch (strip.edge)
            {
                case Edge.E:
                    strip.line.Start = new Vector3(lineFixed, -s, 0f);
                    strip.line.End = new Vector3(lineFixed, s, 0f);
                    break;
                case Edge.N:
                    strip.line.Start = new Vector3(s, lineFixed, 0f);
                    strip.line.End = new Vector3(-s, lineFixed, 0f);
                    break;
                case Edge.W:
                    strip.line.Start = new Vector3(lineFixed, s, 0f);
                    strip.line.End = new Vector3(lineFixed, -s, 0f);
                    break;
                default:
                    strip.line.Start = new Vector3(-s, lineFixed, 0f);
                    strip.line.End = new Vector3(s, lineFixed, 0f);
                    break;
            }
        }

        /// <summary>
        /// The arrival lane's depth, measured inward from the capture band's inner face. Derived from
        /// <c>entryMargin</c>, which puts the lane's inner face on the arrival plane.
        /// </summary>
        private float EntryDepth()
        {
            if (config.PortalEntryWidth > 0f)
            {
                return config.PortalEntryWidth;
            }

            return geometry.EntryMargin;
        }

        private void Teardown()
        {
            strips.Clear();

            for (int i = 0; i < flourishes.Length; i++)
            {
                flourishes[i] = null;
            }

            if (root != null)
            {
                UnityEngine.Object.Destroy(root);
                root = null;
            }

            built = false;
        }

        // ---- the per-event flourish (m4_portal_findings.md §4.2) ------------------------------------

        /// <summary>
        /// An expanding additive ring at the point an organism left or arrived. Fire and forget,
        /// exception-guarded and rate-limited: a visual must never be able to break custody transfer,
        /// and the sim can run at 20× where a per-event allocation would not be free.
        ///
        /// A rejected export deliberately draws nothing — the organism stayed.
        /// </summary>
        internal static void Flash(Vector3 position, bool export)
        {
            PortalVisual portal = Active;
            if (portal == null)
            {
                return;
            }

            try
            {
                portal.Emit(position, export);
            }
            catch (Exception e)
            {
                MultiversePlugin.Log.LogWarning($"{Prefix} a flourish threw and was dropped: {e.Message}");
            }
        }

        private void Emit(Vector3 position, bool export)
        {
            if (!built || config == null || !config.PortalFlourishes)
            {
                return;
            }

            float now = Time.unscaledTime;
            if (now - lastFlourishAt < FlourishMinIntervalSeconds)
            {
                flourishesDropped++;
                return;
            }

            Flourish slot = TakeSlot();
            if (slot == null)
            {
                flourishesDropped++;
                return;
            }

            lastFlourishAt = now;
            flourishesDrawn++;

            slot.colour = export ? ExportColour : EntryColour;
            slot.maxRadius = Mathf.Max(30f, 0.02f * geometry.S);
            slot.startedAt = now;
            slot.active = true;
            slot.go.transform.position = new Vector3(position.x, position.y, 0f);
            slot.go.SetActive(true);
            Apply(slot, 0f);
        }

        private Flourish TakeSlot()
        {
            for (int i = 0; i < flourishes.Length; i++)
            {
                Flourish existing = flourishes[i];
                if (existing != null && !existing.active)
                {
                    return existing;
                }

                if (existing == null)
                {
                    Flourish created = new Flourish();
                    created.go = new GameObject("MultiversePortal_flourish_" + i.ToString(CultureInfo.InvariantCulture));
                    created.go.transform.SetParent(root.transform, worldPositionStays: false);
                    created.go.layer = layer;
                    created.disc = created.go.AddComponent<Disc>();
                    created.disc.Geometry = DiscGeometry.Flat2D;
                    created.disc.Type = DiscType.Ring;
                    created.disc.BlendMode = ShapesBlendMode.Additive;
                    created.disc.ThicknessSpace = ThicknessSpace.Pixels;
                    created.disc.Thickness = FlourishPixelThickness;
                    created.disc.RadiusSpace = ThicknessSpace.Meters;
                    created.disc.SortingOrder = config.PortalSortingOrder + 2;
                    created.disc.Culling = ShapeCulling.SimpleGlobal;
                    created.go.SetActive(false);
                    flourishes[i] = created;
                    return created;
                }
            }

            return null;
        }

        private void PumpFlourishes(float now)
        {
            for (int i = 0; i < flourishes.Length; i++)
            {
                Flourish flourish = flourishes[i];
                if (flourish == null || !flourish.active)
                {
                    continue;
                }

                float t = (now - flourish.startedAt) / FlourishSeconds;
                if (t >= 1f)
                {
                    flourish.active = false;
                    flourish.go.SetActive(false);
                    continue;
                }

                Apply(flourish, t);
            }
        }

        private static void Apply(Flourish flourish, float t)
        {
            flourish.disc.Radius = Mathf.Lerp(0f, flourish.maxRadius, t);
            Color colour = flourish.colour;
            flourish.disc.Color = new Color(colour.r, colour.g, colour.b, 1f - t);
        }

        /// <summary>One status line for the session summary, so a silent portal is still countable.</summary>
        internal string Summary()
        {
            return $"built={built} wanted={wanted} strips={strips.Count} layer={layer} " +
                $"flourishesDrawn={flourishesDrawn} flourishesDropped={flourishesDropped}";
        }
    }
}
