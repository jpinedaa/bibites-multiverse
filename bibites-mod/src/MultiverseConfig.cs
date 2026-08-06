using System;
using System.Collections.Generic;
using System.Globalization;
using BepInEx.Configuration;

namespace BibitesMultiverse
{
    /// <summary>
    /// Per-instance configuration. Every game instance of the rig shares one plugins/ DLL and one
    /// config/ directory (dev_environment.md), so the environment variable always wins over the
    /// BepInEx config entry — the config entry only exists so a single-instance run can be set up
    /// without touching the environment.
    ///
    /// The WSL to Windows hop needs WSLENV to name each variable; see dev_environment.md.
    ///
    /// **The grid (D13, contract-a.md §15 A18).** A sim declares a **set** of export edges —
    /// <c>["E", "N"]</c> under the grid — and receives on the opposite edge of each. The entry edges
    /// are derived, and they are passive: always accepting, never opened or closed, and with no
    /// capture band at all (§4.3.1). <c>borderEdges</c> is the union, and under the grid it is all
    /// four values.
    ///
    /// Declaring an edge is a statement about **geometry, not topology**: it means "I run a capture
    /// band here". Whether that edge has a lane is the sidecar's answer in <c>EDGE_STATUS</c>. A mod
    /// **MUST NOT** vary its declaration by map shape — it has no way to know the shape.
    /// </summary>
    internal class MultiverseConfig
    {
        /// <summary>§5.1, §15 A18 — the edges this sim exports through. "E,N" under the grid.</summary>
        internal const string EnvExportEdges = "MULTIVERSE_EXPORT_EDGES";

        /// <summary>M3's singular name. Read when <see cref="EnvExportEdges"/> is unset.</summary>
        internal const string EnvExportEdge = "MULTIVERSE_EXPORT_EDGE";

        /// <summary>M2's name for the same knob. Read when neither of the two above is set.</summary>
        internal const string EnvOpenEdge = "MULTIVERSE_OPEN_EDGE";

        /// <summary>§5.1, §14 A14 — the advisory ring slot. Replaces M2's retired MULTIVERSE_SECTOR.</summary>
        internal const string EnvRingSlot = "MULTIVERSE_RING_SLOT";

        internal const string EnvPort = "MULTIVERSE_SIDECAR_PORT";
        internal const string EnvBorderWidth = "MULTIVERSE_BORDER_WIDTH";
        internal const string EnvWorld = "MULTIVERSE_WORLD";

        // ---- M4 ------------------------------------------------------------------------------
        /// <summary>D14 — minutes of wall clock between periodic world saves. 0 turns the timer off.</summary>
        internal const string EnvSaveMinutes = "MULTIVERSE_SAVE_MINUTES";

        /// <summary>D14 — how many rotated saves to keep beside the live one.</summary>
        internal const string EnvSaveKeep = "MULTIVERSE_SAVE_KEEP";

        /// <summary>D14 — save when the application quits. T1 lost 12 hours to the absence of this.</summary>
        internal const string EnvSaveOnQuit = "MULTIVERSE_SAVE_ON_QUIT";

        /// <summary>WP7 — the portal strips.</summary>
        internal const string EnvPortal = "MULTIVERSE_PORTAL";

        /// <summary>WP7 — the per-event expanding ring.</summary>
        internal const string EnvPortalFlourishes = "MULTIVERSE_PORTAL_FLOURISHES";

        /// <summary>Fraction of the half-extent S used for the border strip when nothing overrides it.</summary>
        internal const float DefaultWidthFactor = 0.02f;

        /// <summary>A bibite is roughly 10 u long, so a strip thinner than this cannot be crossed reliably.</summary>
        internal const float MinimumWidth = 20f;

        internal const float DefaultSaveMinutes = 10f;
        internal const int DefaultSaveKeep = 6;

        internal bool Enabled;

        /// <summary>§5.1 <c>exportEdges</c> — the edges MIGRATE_OUT may use, in ContractA.EdgeRank order.</summary>
        internal readonly List<Edge> ExportEdges = new List<Edge>();

        /// <summary>The passive entry edges — the opposite of each export edge. They always accept.</summary>
        internal readonly List<Edge> EntryEdges = new List<Edge>();

        /// <summary>§5.1 <c>borderEdges</c> — every edge that accepts an inbound organism. All four under the grid.</summary>
        internal readonly List<Edge> BorderEdges = new List<Edge>();

        internal int Port = ContractA.DefaultPort;
        internal bool HasRingSlot;
        internal int RingSlot;
        internal float BorderWidthOverride;
        internal string WorldToLoad = string.Empty;

        internal float SaveMinutes = DefaultSaveMinutes;
        internal int SaveKeep = DefaultSaveKeep;
        internal bool SaveOnQuit = true;

        internal bool Portal = true;
        internal bool PortalFlourishes = true;
        internal int PortalSortingOrder = -50;

        /// <summary>Entry-strip depth for the portal, in world units. 0 derives it from W + entryMargin.</summary>
        internal float PortalEntryWidth;

        internal string Url => "ws://127.0.0.1:" + Port.ToString(CultureInfo.InvariantCulture) + ContractA.UrlPath;

        /// <summary>
        /// §5.1 <c>borderEdges</c>, §13 A3 — the edges that **accept** an inbound organism. Inbound
        /// <c>EDGE_CLOSED</c> means "not a declared edge", never "closed right now": a declared edge
        /// that is currently closed still accepts, because a bounce-back arrives exactly when the edge
        /// just closed.
        /// </summary>
        internal bool Declares(Edge edge)
        {
            return edge != Edge.None && BorderEdges.Contains(edge);
        }

        internal bool Exports(Edge edge)
        {
            return edge != Edge.None && ExportEdges.Contains(edge);
        }

        /// <summary>The first declared export edge, which is the rig's default target for a forced export.</summary>
        internal Edge PrimaryExportEdge => (ExportEdges.Count > 0) ? ExportEdges[0] : Edge.None;

        internal static MultiverseConfig Read(ConfigFile config)
        {
            ConfigEntry<string> edgeEntry = config.Bind(
                "M4",
                "ExportEdges",
                string.Empty,
                "The map edges this instance exports through, comma separated: 'E,N' under the grid (D13). " +
                "The opposite of each becomes a passive entry edge. Empty disables the multiverse client " +
                "entirely. The environment variable " + EnvExportEdges + " overrides this, and " +
                EnvExportEdge + " / " + EnvOpenEdge + " are still read as a single-edge form.");

            ConfigEntry<int> slotEntry = config.Bind(
                "M3",
                "RingSlot",
                0,
                "This instance's ring slot, an integer >= 1. Advisory: the sidecar closes the connection " +
                "with 4001 when it disagrees. 0 omits the field. The environment variable " + EnvRingSlot +
                " overrides this.");

            ConfigEntry<int> portEntry = config.Bind(
                "M3",
                "SidecarPort",
                ContractA.DefaultPort,
                "The loopback port the Contract A sidecar listens on. The environment variable " + EnvPort + " overrides this.");

            ConfigEntry<float> widthEntry = config.Bind(
                "M3",
                "BorderWidth",
                0f,
                "Border strip width W in world units, which is also the inner boundary of every capture " +
                "band (contract-a.md §4.3.1). 0 derives it from the simulation half-extent S as " +
                DefaultWidthFactor.ToString(CultureInfo.InvariantCulture) + "*S, with a floor of " +
                MinimumWidth.ToString(CultureInfo.InvariantCulture) + " u. The environment variable " +
                EnvBorderWidth + " overrides this.");

            ConfigEntry<float> saveMinutesEntry = config.Bind(
                "M4",
                "SaveMinutes",
                DefaultSaveMinutes,
                "Wall-clock minutes between periodic world saves (D14). 0 turns the timer off. The game's " +
                "own autosave stays off either way — an autosave reloads the scene under a live rig. The " +
                "environment variable " + EnvSaveMinutes + " overrides this.");

            ConfigEntry<int> saveKeepEntry = config.Bind(
                "M4",
                "SaveKeep",
                DefaultSaveKeep,
                "How many rotated saves to keep beside the live one. The live save keeps the world's own " +
                "name, so MULTIVERSE_WORLD always names the newest. The environment variable " +
                EnvSaveKeep + " overrides this.");

            ConfigEntry<bool> saveOnQuitEntry = config.Bind(
                "M4",
                "SaveOnQuit",
                true,
                "Save the loaded world when the application quits (D14). The environment variable " +
                EnvSaveOnQuit + " overrides this.");

            ConfigEntry<bool> portalEntry = config.Bind(
                "M4",
                "Portal",
                true,
                "Draw a portal strip on every open export edge and on every entry edge while the sidecar " +
                "is connected (WP7, m4_portal_findings.md). The environment variable " + EnvPortal +
                " overrides this.");

            ConfigEntry<bool> flourishEntry = config.Bind(
                "M4",
                "PortalFlourishes",
                true,
                "Draw an expanding ring at each export and each arrival (m4_portal_findings.md §4.2). " +
                "The environment variable " + EnvPortalFlourishes + " overrides this.");

            ConfigEntry<int> sortingEntry = config.Bind(
                "M4",
                "PortalSortingOrder",
                -50,
                "Sorting order of the portal renderers. Nothing in the game assembly sets a world-space " +
                "sorting order in code, so the values for bibites, pellets and zones are unknown " +
                "statically (m4_portal_findings.md §5.3 item 4). Negative sits behind organisms.");

            ConfigEntry<float> entryWidthEntry = config.Bind(
                "M4",
                "PortalEntryWidth",
                0f,
                "Depth of the entry-edge portal strip in world units. 0 derives it from W + entryMargin, " +
                "which is exactly where an arriving organism is placed (contract-a.md §4.3).");

            MultiverseConfig result = new MultiverseConfig();

            string edgeText = Prefer(Prefer(Prefer(Env(EnvExportEdges), Env(EnvExportEdge)), Env(EnvOpenEdge)), edgeEntry.Value);
            result.ParseExportEdges(edgeText);

            string portText = Prefer(Env(EnvPort), string.Empty);
            if (!string.IsNullOrEmpty(portText))
            {
                if (int.TryParse(portText.Trim(), NumberStyles.Integer, CultureInfo.InvariantCulture, out int port) && port > 0 && port <= 65535)
                {
                    result.Port = port;
                }
                else
                {
                    MultiversePlugin.Log.LogError($"[M2] config: {EnvPort}='{portText}' is not a valid port — using {portEntry.Value}.");
                    result.Port = portEntry.Value;
                }
            }
            else
            {
                result.Port = portEntry.Value;
            }

            string slotText = Prefer(Env(EnvRingSlot), string.Empty);
            int slot = slotEntry.Value;
            if (!string.IsNullOrEmpty(slotText)
                && !int.TryParse(slotText.Trim(), NumberStyles.Integer, CultureInfo.InvariantCulture, out slot))
            {
                MultiversePlugin.Log.LogWarning(
                    $"[M2] config: {EnvRingSlot}='{slotText}' is not an integer — CONFIG_UPDATE.ringSlot is omitted.");
                slot = 0;
            }

            if (slot >= 1)
            {
                result.HasRingSlot = true;
                result.RingSlot = slot;
            }
            else if (slot != 0)
            {
                MultiversePlugin.Log.LogWarning(
                    $"[M2] config: ringSlot {slot} is below 1 — the field is advisory and >= 1 (§5.1), so it is omitted.");
            }

            string widthText = Prefer(Env(EnvBorderWidth), string.Empty);
            if (!string.IsNullOrEmpty(widthText))
            {
                if (float.TryParse(widthText.Trim(), NumberStyles.Float, CultureInfo.InvariantCulture, out float width) && width > 0f)
                {
                    result.BorderWidthOverride = width;
                }
                else
                {
                    MultiversePlugin.Log.LogError($"[M2] config: {EnvBorderWidth}='{widthText}' is not a positive number — deriving W from S.");
                }
            }
            else if (widthEntry.Value > 0f)
            {
                result.BorderWidthOverride = widthEntry.Value;
            }

            result.WorldToLoad = (Env(EnvWorld) ?? string.Empty).Trim();

            result.SaveMinutes = ReadFloat(EnvSaveMinutes, saveMinutesEntry.Value, 0f);
            result.SaveKeep = Math.Max(0, ReadInt(EnvSaveKeep, saveKeepEntry.Value));
            result.SaveOnQuit = ReadBool(EnvSaveOnQuit, saveOnQuitEntry.Value);
            result.Portal = ReadBool(EnvPortal, portalEntry.Value);
            result.PortalFlourishes = ReadBool(EnvPortalFlourishes, flourishEntry.Value);
            result.PortalSortingOrder = sortingEntry.Value;
            result.PortalEntryWidth = Math.Max(0f, entryWidthEntry.Value);

            return result;
        }

        /// <summary>
        /// §5.1 — the array **MUST** hold at least one entry and **MUST NOT** hold a duplicate, and
        /// every member of <c>exportEdges</c> **MUST** also be a member of <c>borderEdges</c>. The last
        /// rule is structural here: <c>borderEdges</c> is built as the union of the export set and its
        /// opposites, so it cannot disagree.
        ///
        /// The set is canonicalized into <see cref="ContractA.EdgeRank"/> order, which is what makes
        /// the corner rule's "E takes the tie" fall out of "the first declared edge keeps the win".
        /// </summary>
        private void ParseExportEdges(string text)
        {
            ExportEdges.Clear();
            EntryEdges.Clear();
            BorderEdges.Clear();
            Enabled = false;

            if (string.IsNullOrEmpty(text))
            {
                return;
            }

            string[] parts = text.Split(new[] { ',', ';', ' ', '\t' }, StringSplitOptions.RemoveEmptyEntries);
            foreach (string part in parts)
            {
                if (!ContractA.TryParseEdge(part, out Edge edge))
                {
                    MultiversePlugin.Log.LogError(
                        $"[M2] config: '{part}' is not a valid edge (expected N, S, E or W) — the multiverse client stays off.");
                    ExportEdges.Clear();
                    return;
                }

                if (ExportEdges.Contains(edge))
                {
                    MultiversePlugin.Log.LogError(
                        $"[M2] config: export edge {ContractA.EdgeName(edge)} is declared twice. §5.1 forbids a duplicate in " +
                        "exportEdges — the multiverse client stays off.");
                    ExportEdges.Clear();
                    return;
                }

                ExportEdges.Add(edge);
            }

            ExportEdges.Sort((a, b) => ContractA.EdgeRank(a).CompareTo(ContractA.EdgeRank(b)));

            foreach (Edge edge in ExportEdges)
            {
                Edge opposite = ContractA.OppositeEdge(edge);
                if (ExportEdges.Contains(opposite))
                {
                    MultiversePlugin.Log.LogError(
                        $"[M2] config: {ContractA.EdgeName(edge)} and {ContractA.EdgeName(opposite)} are both declared as export " +
                        "edges, so one edge would have to be a capture band and a passive entry at the same time. That is not a " +
                        "map shape this contract has — the multiverse client stays off.");
                    ExportEdges.Clear();
                    EntryEdges.Clear();
                    return;
                }

                EntryEdges.Add(opposite);
            }

            EntryEdges.Sort((a, b) => ContractA.EdgeRank(a).CompareTo(ContractA.EdgeRank(b)));

            BorderEdges.AddRange(ExportEdges);
            BorderEdges.AddRange(EntryEdges);
            BorderEdges.Sort((a, b) => ContractA.EdgeRank(a).CompareTo(ContractA.EdgeRank(b)));

            Enabled = ExportEdges.Count > 0;
        }

        internal void LogSummary()
        {
            MultiversePlugin.Log.LogInfo(
                $"[M2] config: enabled={Enabled} exportEdges=[{ContractA.EdgeNames(ExportEdges)}] " +
                $"entryEdges=[{ContractA.EdgeNames(EntryEdges)}] (passive) " +
                $"borderEdges=[{ContractA.EdgeNames(BorderEdges)}] " +
                $"ringSlot={(HasRingSlot ? RingSlot.ToString(CultureInfo.InvariantCulture) : "<omitted>")} " +
                $"port={Port} url={Url} " +
                $"borderWidth={(BorderWidthOverride > 0f ? BorderWidthOverride.ToString("F2", CultureInfo.InvariantCulture) : $"{DefaultWidthFactor}*S (min {MinimumWidth})")} " +
                $"world={(string.IsNullOrEmpty(WorldToLoad) ? "<unset>" : WorldToLoad)}");
            MultiversePlugin.Log.LogInfo(
                $"[M4] config: saveMinutes={SaveMinutes.ToString("F1", CultureInfo.InvariantCulture)} saveKeep={SaveKeep} " +
                $"saveOnQuit={SaveOnQuit} portal={Portal} portalFlourishes={PortalFlourishes} " +
                $"portalSortingOrder={PortalSortingOrder} " +
                $"portalEntryWidth={(PortalEntryWidth > 0f ? PortalEntryWidth.ToString("F1", CultureInfo.InvariantCulture) : "W+entryMargin")}");
            MultiversePlugin.Log.LogInfo(
                $"[M2] config sources: {EnvExportEdges}={Show(Env(EnvExportEdges))} {EnvExportEdge}={Show(Env(EnvExportEdge))} " +
                $"{EnvOpenEdge}={Show(Env(EnvOpenEdge))} " +
                $"{EnvRingSlot}={Show(Env(EnvRingSlot))} {EnvPort}={Show(Env(EnvPort))} " +
                $"{EnvBorderWidth}={Show(Env(EnvBorderWidth))} {EnvWorld}={Show(Env(EnvWorld))} " +
                $"{EnvSaveMinutes}={Show(Env(EnvSaveMinutes))} {EnvSaveKeep}={Show(Env(EnvSaveKeep))} " +
                $"{EnvSaveOnQuit}={Show(Env(EnvSaveOnQuit))} {EnvPortal}={Show(Env(EnvPortal))} " +
                $"{EnvPortalFlourishes}={Show(Env(EnvPortalFlourishes))} " +
                "(environment beats the BepInEx config; WSLENV must name each variable)");
        }

        private static string Show(string value)
        {
            return string.IsNullOrEmpty(value) ? "<unset>" : value;
        }

        private static string Prefer(string environmentValue, string configValue)
        {
            return string.IsNullOrEmpty(environmentValue) ? configValue : environmentValue;
        }

        private static float ReadFloat(string name, float fallback, float floor)
        {
            string text = Env(name);
            if (string.IsNullOrEmpty(text))
            {
                return Math.Max(floor, fallback);
            }

            if (float.TryParse(text.Trim(), NumberStyles.Float, CultureInfo.InvariantCulture, out float value) && value >= floor)
            {
                return value;
            }

            MultiversePlugin.Log.LogWarning($"[M4] config: {name}='{text}' is not a number >= {floor} — using {fallback}.");
            return Math.Max(floor, fallback);
        }

        private static int ReadInt(string name, int fallback)
        {
            string text = Env(name);
            if (string.IsNullOrEmpty(text))
            {
                return fallback;
            }

            if (int.TryParse(text.Trim(), NumberStyles.Integer, CultureInfo.InvariantCulture, out int value))
            {
                return value;
            }

            MultiversePlugin.Log.LogWarning($"[M4] config: {name}='{text}' is not an integer — using {fallback}.");
            return fallback;
        }

        private static bool ReadBool(string name, bool fallback)
        {
            string text = Env(name);
            if (string.IsNullOrEmpty(text))
            {
                return fallback;
            }

            switch (text.Trim().ToLowerInvariant())
            {
                case "1":
                case "true":
                case "yes":
                case "on":
                    return true;
                case "0":
                case "false":
                case "no":
                case "off":
                    return false;
                default:
                    MultiversePlugin.Log.LogWarning($"[M4] config: {name}='{text}' is not a boolean — using {fallback}.");
                    return fallback;
            }
        }

        internal static string Env(string name)
        {
            try
            {
                return Environment.GetEnvironmentVariable(name);
            }
            catch (Exception e)
            {
                MultiversePlugin.Log.LogWarning($"[M2] could not read {name}: {e.Message}");
                return null;
            }
        }
    }
}
