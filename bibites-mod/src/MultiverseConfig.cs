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
    /// **Two-way lanes (D17, contract-a.md §18 A38).** Every declared edge is **both** an export edge
    /// and an entry edge. A conformant mod declares all four — <c>["E", "N", "W", "S"]</c> — and that
    /// is now the default: <see cref="EnvExportEdges"/> is only needed to declare a **subset**, or the
    /// literal <c>none</c> to keep the client off. <c>borderEdges</c> is always all four (§5.1, A18):
    /// arrivals were never gated by <c>exportEdges</c>, so a mod that runs two bands still receives on
    /// all four edges (§18, A41, case 3). There is no passive edge any more.
    ///
    /// Declaring an edge is a statement about **geometry, not topology**: it means "I run a capture
    /// band here". Whether that edge has a lane is the sidecar's answer in <c>EDGE_STATUS</c>. A mod
    /// **MUST NOT** vary its declaration by map shape — it has no way to know the shape.
    ///
    /// **One setting deliberately does not live here** (contract-a.md §21, A47): the bearer token that
    /// authenticates the Contract A upgrade. Its two variables — <c>MULTIVERSE_CONTRACT_A_TOKEN_FILE</c>
    /// and <c>MULTIVERSE_CONTRACT_A_TOKEN</c> — are read by <see cref="ContractAToken"/> on the socket
    /// thread, on **every** dial, because a token read once at startup could not pick up a rotation
    /// without a game restart. They get no BepInEx config entry either: a secret in
    /// <c>config/</c> would be one file shared by every instance of the rig, and this one is per
    /// machine. Only their *presence* and the file's *path* are summarized below; the value is never
    /// logged.
    /// </summary>
    internal class MultiverseConfig
    {
        /// <summary>
        /// §5.1, §15 A18, §18 A38 — the edges this sim exports through. Unset means **all four**
        /// (D17). A subset such as "E,N" is still honoured, and "none" turns the client off.
        /// </summary>
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

        /// <summary>C3 (Slot-6 livelock) — opt-in verbose per-MIGRATE_IN logging and the payload SHA-256.</summary>
        internal const string EnvDebugIngest = "MULTIVERSE_DEBUG_INGEST";

        /// <summary>
        /// D18, §18 A39 — comma-separated full species names that never export. Default
        /// <c>"Basic bibite"</c>; an **explicitly empty** value disables the policy, so this variable is
        /// read on presence rather than on emptiness.
        /// </summary>
        internal const string EnvMigrationExclude = "MULTIVERSE_MIGRATION_EXCLUDE";

        /// <summary>Fraction of the half-extent S used for the border strip when nothing overrides it.</summary>
        internal const float DefaultWidthFactor = 0.02f;

        /// <summary>A bibite is roughly 10 u long, so a strip thinner than this cannot be crossed reliably.</summary>
        internal const float MinimumWidth = 20f;

        internal const float DefaultSaveMinutes = 10f;
        internal const int DefaultSaveKeep = 6;

        internal bool Enabled;

        /// <summary>§5.1 <c>exportEdges</c> — the edges MIGRATE_OUT may use, in ContractA.EdgeRank order.</summary>
        internal readonly List<Edge> ExportEdges = new List<Edge>();

        /// <summary>
        /// §5.1 <c>borderEdges</c> — every edge that accepts an inbound organism. **Always all four**
        /// (§15 A18, §18 A38): an arrival is never gated by <c>exportEdges</c>, and under D17 every
        /// declared export edge is one of these too.
        /// </summary>
        internal readonly List<Edge> BorderEdges = new List<Edge>();

        /// <summary>D18, §18 A39 — the species that never export. Empty when the policy is off.</summary>
        internal readonly MigrationExclusion Exclusion = new MigrationExclusion();

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

        /// <summary>
        /// C3 (Slot-6 livelock) — when false (the default) each MIGRATE_IN logs a single outcome line and
        /// never pays the hot-path SHA-256 of its ~10 kB payload. Turn it on to restore the full per-ingest
        /// trace — the received line with <c>payloadSha256</c>, the link-capture summary, and the per-link
        /// relink detail — when a migration needs debugging. Errors and NACKs are logged either way.
        /// </summary>
        internal bool DebugIngest;

        /// <summary>
        /// Depth of the portal's arrival lane, measured inward from the capture band's inner face.
        /// 0 derives it from <c>entryMargin</c>, which puts the lane's inner face on the arrival plane.
        /// </summary>
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
                "The map edges this instance exports through, comma separated. Empty means all four — " +
                "'E,N,W,S' — which is what a conformant mod declares under two-way lanes (D17, " +
                "contract-a.md §18 A38): every edge runs a capture band and every edge accepts arrivals. " +
                "Name a subset to run fewer bands; 'none' turns the multiverse client off entirely. " +
                "The environment variable " + EnvExportEdges + " overrides this, and " +
                EnvExportEdge + " / " + EnvOpenEdge + " are still read as a single-edge form.");

            ConfigEntry<string> excludeEntry = config.Bind(
                "M4",
                "MigrationExcludeSpecies",
                MigrationExclusion.DefaultNames,
                "Comma-separated full species names that never export (D18, contract-a.md §18 A39). " +
                "Matched on the §16 A34-normalized form — trimmed, internal whitespace runs collapsed — " +
                "so a name the game issued with an edge space still matches. Empty disables the policy. " +
                "An excluded species is never captured on any edge; it still lives here, the wrap " +
                "contains it (D10), and the census still reports it. The normalized list is published " +
                "on CONFIG_UPDATE so the site can say why a world's lanes are quiet (§19 A42); it is " +
                "read-only there and no other party acts on it. The environment variable " +
                EnvMigrationExclude + " overrides this, including when it is set to an empty value.");

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
                "Draw the portal: a capture lane on every open export edge, and an arrival lane on every " +
                "border edge while the sidecar is connected — two lanes on every edge under two-way " +
                "lanes (WP7, m4_portal_findings.md, D17). The environment variable " + EnvPortal +
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
                "Depth of the arrival lane of the portal, in world units, measured **inward from the " +
                "capture band's inner face** (D17: every edge now draws both lanes). 0 derives it from " +
                "entryMargin, which puts the lane's inner face exactly where an arriving organism is " +
                "placed — W + entryMargin from the map edge (contract-a.md §4.3).");

            ConfigEntry<bool> debugIngestEntry = config.Bind(
                "M4",
                "DebugIngest",
                false,
                "Verbose per-MIGRATE_IN logging: the received line with payloadSha256, the link-capture " +
                "summary and the per-link relink detail. Off by default because a deep inbound backlog would " +
                "otherwise spend the main thread on log I/O and a SHA-256 of every ~10 kB payload (the Slot-6 " +
                "livelock). The environment variable " + EnvDebugIngest + " overrides this. Errors and NACKs " +
                "are always logged.");

            MultiverseConfig result = new MultiverseConfig();

            string edgeText = Prefer(Prefer(Prefer(Env(EnvExportEdges), Env(EnvExportEdge)), Env(EnvOpenEdge)), edgeEntry.Value);
            result.ParseExportEdges(edgeText);

            // §18 A39 — read on **presence**, not on emptiness: an explicitly empty value is how an
            // operator disables the policy, and Prefer would read that as "unset" and restore the default.
            string excludeText = Env(EnvMigrationExclude) ?? excludeEntry.Value;
            result.Exclusion.Configure(excludeText);

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
            result.DebugIngest = ReadBool(EnvDebugIngest, debugIngestEntry.Value);

            return result;
        }

        /// <summary>
        /// §5.1 — the array **MUST** hold at least one entry and **MUST NOT** hold a duplicate, and
        /// every member of <c>exportEdges</c> **MUST** also be a member of <c>borderEdges</c>. The last
        /// rule is structural here: <c>borderEdges</c> is always all four (§15 A18, §18 A38), so no
        /// declared subset can disagree with it.
        ///
        /// **Two rules changed under D17** (§18, A38):
        ///
        /// * an absent or empty value now means **all four edges**, not "off". A conformant mod
        ///   declares all four, and the literal <c>none</c> (or a value naming no edge at all) is the
        ///   off switch;
        /// * <c>E</c> with <c>W</c>, and <c>N</c> with <c>S</c>, are **legal together**. Before D17 an
        ///   edge was a capture band or a passive entry and never both, so the pair was rejected. There
        ///   is no passive edge any more: every declared edge is both roles at once.
        ///
        /// The set is canonicalized into <see cref="ContractA.EdgeRank"/> order, which is what makes
        /// the corner rule's "E takes the tie" fall out of "the first declared edge keeps the win".
        /// </summary>
        private void ParseExportEdges(string text)
        {
            ExportEdges.Clear();
            BorderEdges.Clear();
            Enabled = false;

            // §5.1, §18 A38 — borderEdges is all four whatever the export set is. An arrival has never
            // been gated by exportEdges, so a two-band mod on a two-way map still receives on four
            // edges (§18, A41 case 3). It is filled even when the client stays off, so a log line about
            // a disabled client still shows what it would have declared.
            BorderEdges.AddRange(ContractA.AllEdges);

            string trimmed = (text ?? string.Empty).Trim();
            if (trimmed.Length == 0)
            {
                // D17's default. Nothing configured means the whole perimeter, not silence.
                ExportEdges.AddRange(ContractA.AllEdges);
                Enabled = true;
                return;
            }

            if (IsWord(trimmed, "none") || IsWord(trimmed, "off"))
            {
                MultiversePlugin.Log.LogInfo(
                    $"[M2] config: export edges are '{trimmed}' — the multiverse client stays off by request. " +
                    "Unset the variable to get D17's default of all four edges.");
                return;
            }

            if (IsWord(trimmed, "all") || IsWord(trimmed, "*"))
            {
                ExportEdges.AddRange(ContractA.AllEdges);
                Enabled = true;
                return;
            }

            string[] parts = trimmed.Split(new[] { ',', ';', ' ', '\t' }, StringSplitOptions.RemoveEmptyEntries);
            foreach (string part in parts)
            {
                if (!ContractA.TryParseEdge(part, out Edge edge))
                {
                    MultiversePlugin.Log.LogError(
                        $"[M2] config: '{part}' is not a valid edge (expected N, S, E, W, 'all' or 'none') — the multiverse " +
                        "client stays off.");
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

            if (ExportEdges.Count == 0)
            {
                // A value made only of separators is the older way of clearing the setting, and it still
                // clears it — with no error, exactly as it always has.
                MultiversePlugin.Log.LogInfo(
                    $"[M2] config: export edges '{trimmed}' name no edge — the multiverse client stays off.");
                return;
            }

            if (ExportEdges.Count < ContractA.AllEdges.Length)
            {
                MultiversePlugin.Log.LogWarning(
                    $"[M2] config: this instance declares {ExportEdges.Count} of the 4 export edges " +
                    $"([{ContractA.EdgeNames(ExportEdges)}]). Under two-way lanes a conformant mod declares all four " +
                    "(contract-a.md §18, A38); the undeclared edges still accept arrivals but can never export, which " +
                    "shows on the page as a slot with dead lanes. Unset " + EnvExportEdges + " to declare all four.");
            }

            Enabled = true;
        }

        private static bool IsWord(string text, string word)
        {
            return string.Equals(text, word, StringComparison.OrdinalIgnoreCase);
        }

        internal void LogSummary()
        {
            MultiversePlugin.Log.LogInfo(
                $"[M2] config: enabled={Enabled} exportEdges=[{ContractA.EdgeNames(ExportEdges)}] " +
                $"borderEdges=[{ContractA.EdgeNames(BorderEdges)}] (always all four: a declared edge both exports and " +
                "receives, and an undeclared one still receives — D17) " +
                $"ringSlot={(HasRingSlot ? RingSlot.ToString(CultureInfo.InvariantCulture) : "<omitted>")} " +
                $"port={Port} url={Url} " +
                $"borderWidth={(BorderWidthOverride > 0f ? BorderWidthOverride.ToString("F2", CultureInfo.InvariantCulture) : $"{DefaultWidthFactor}*S (min {MinimumWidth})")} " +
                $"world={(string.IsNullOrEmpty(WorldToLoad) ? "<unset>" : WorldToLoad)}");
            MultiversePlugin.Log.LogInfo(
                $"[M4] config: saveMinutes={SaveMinutes.ToString("F1", CultureInfo.InvariantCulture)} saveKeep={SaveKeep} " +
                $"saveOnQuit={SaveOnQuit} portal={Portal} portalFlourishes={PortalFlourishes} " +
                $"portalSortingOrder={PortalSortingOrder} " +
                $"portalEntryWidth={(PortalEntryWidth > 0f ? PortalEntryWidth.ToString("F1", CultureInfo.InvariantCulture) : "entryMargin")} " +
                $"debugIngest={DebugIngest}");
            MultiversePlugin.Log.LogInfo(
                $"{MigrationExclusion.Prefix} config: migrationExcludeSpecies={Exclusion.Describe()} " +
                $"(matched on the A34-normalized full name; export-side only, never a census filter; " +
                "published on CONFIG_UPDATE for reading only, and enforced here alone — §19 A42)");
            MultiversePlugin.Log.LogInfo(
                $"[M2] config sources: {EnvExportEdges}={Show(Env(EnvExportEdges))} {EnvExportEdge}={Show(Env(EnvExportEdge))} " +
                $"{EnvOpenEdge}={Show(Env(EnvOpenEdge))} " +
                $"{EnvRingSlot}={Show(Env(EnvRingSlot))} {EnvPort}={Show(Env(EnvPort))} " +
                $"{EnvBorderWidth}={Show(Env(EnvBorderWidth))} {EnvWorld}={Show(Env(EnvWorld))} " +
                $"{EnvSaveMinutes}={Show(Env(EnvSaveMinutes))} {EnvSaveKeep}={Show(Env(EnvSaveKeep))} " +
                $"{EnvSaveOnQuit}={Show(Env(EnvSaveOnQuit))} {EnvPortal}={Show(Env(EnvPortal))} " +
                $"{EnvPortalFlourishes}={Show(Env(EnvPortalFlourishes))} " +
                $"{EnvMigrationExclude}={Show(Env(EnvMigrationExclude))} " +
                "(environment beats the BepInEx config; WSLENV must name each variable)");
            MultiversePlugin.Log.LogInfo(
                $"[M2] contract A token: {ContractAToken.DescribeConfiguration()} — the bearer token of §21 A47, " +
                "which authenticates the WebSocket upgrade. It is not the relay's credential and no part of it is " +
                "ever written to this log. Both variables must be named in WSLENV like every other one.");
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
