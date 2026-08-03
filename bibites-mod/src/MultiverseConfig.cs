using System;
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
    /// **The ring (D8, contract-a.md §14 A11).** A sim has exactly two doors and they are different
    /// doors: it **exports** through one edge and **receives** through the opposite one. Only the
    /// export edge is configured; the entry edge is derived, and it is passive — always accepting,
    /// never opened or closed, and with no capture band at all (§4.3.1).
    /// </summary>
    internal class MultiverseConfig
    {
        /// <summary>The one edge this sim exports through. "E" under the ring.</summary>
        internal const string EnvExportEdge = "MULTIVERSE_EXPORT_EDGE";

        /// <summary>M2's name for the same knob. Read when <see cref="EnvExportEdge"/> is unset.</summary>
        internal const string EnvOpenEdge = "MULTIVERSE_OPEN_EDGE";

        /// <summary>§5.1, §14 A14 — the advisory ring slot. Replaces M2's retired MULTIVERSE_SECTOR.</summary>
        internal const string EnvRingSlot = "MULTIVERSE_RING_SLOT";

        internal const string EnvPort = "MULTIVERSE_SIDECAR_PORT";
        internal const string EnvBorderWidth = "MULTIVERSE_BORDER_WIDTH";
        internal const string EnvWorld = "MULTIVERSE_WORLD";

        /// <summary>Fraction of the half-extent S used for the border strip when nothing overrides it.</summary>
        internal const float DefaultWidthFactor = 0.02f;

        /// <summary>A bibite is roughly 10 u long, so a strip thinner than this cannot be crossed reliably.</summary>
        internal const float MinimumWidth = 20f;

        internal bool Enabled;

        /// <summary>The one edge MIGRATE_OUT may use (§5.1 exportEdge, §4.3.1 capture band).</summary>
        internal Edge ExportEdge = Edge.None;

        /// <summary>The passive entry edge — the opposite of <see cref="ExportEdge"/>. Always accepts.</summary>
        internal Edge EntryEdge = Edge.None;

        internal int Port = ContractA.DefaultPort;
        internal bool HasRingSlot;
        internal int RingSlot;
        internal float BorderWidthOverride;
        internal string WorldToLoad = string.Empty;

        internal string Url => "ws://127.0.0.1:" + Port.ToString(CultureInfo.InvariantCulture) + ContractA.UrlPath;

        /// <summary>§5.1 borderEdges — the edges that accept an inbound organism. ["E", "W"] under the ring.</summary>
        internal bool Declares(Edge edge)
        {
            return edge != Edge.None && (edge == ExportEdge || edge == EntryEdge);
        }

        internal static MultiverseConfig Read(ConfigFile config)
        {
            ConfigEntry<string> edgeEntry = config.Bind(
                "M3",
                "ExportEdge",
                string.Empty,
                "The single map edge this instance exports through: N, S, E or W ('E' under the ring, D8). " +
                "The opposite edge becomes the passive entry edge. Empty disables the multiverse client " +
                "entirely. The environment variable " + EnvExportEdge + " overrides this.");

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
                "Border strip width W in world units, which is also the inner boundary of the capture " +
                "band (contract-a.md §4.3.1). 0 derives it from the simulation half-extent S as " +
                DefaultWidthFactor.ToString(CultureInfo.InvariantCulture) + "*S, with a floor of " +
                MinimumWidth.ToString(CultureInfo.InvariantCulture) + " u. The environment variable " +
                EnvBorderWidth + " overrides this.");

            MultiverseConfig result = new MultiverseConfig();

            string edgeText = Prefer(Prefer(Env(EnvExportEdge), Env(EnvOpenEdge)), edgeEntry.Value);
            if (!string.IsNullOrEmpty(edgeText) && !ContractA.TryParseEdge(edgeText, out result.ExportEdge))
            {
                MultiversePlugin.Log.LogError(
                    $"[M2] config: '{edgeText}' is not a valid edge (expected N, S, E or W) — the multiverse client stays off.");
                result.ExportEdge = Edge.None;
            }

            result.EntryEdge = ContractA.OppositeEdge(result.ExportEdge);
            result.Enabled = result.ExportEdge != Edge.None;

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

            return result;
        }

        internal void LogSummary()
        {
            MultiversePlugin.Log.LogInfo(
                $"[M2] config: enabled={Enabled} exportEdge={ContractA.EdgeName(ExportEdge)} " +
                $"entryEdge={ContractA.EdgeName(EntryEdge)} (passive) " +
                $"ringSlot={(HasRingSlot ? RingSlot.ToString(CultureInfo.InvariantCulture) : "<omitted>")} " +
                $"port={Port} url={Url} " +
                $"borderWidth={(BorderWidthOverride > 0f ? BorderWidthOverride.ToString("F2", CultureInfo.InvariantCulture) : $"{DefaultWidthFactor}*S (min {MinimumWidth})")} " +
                $"world={(string.IsNullOrEmpty(WorldToLoad) ? "<unset>" : WorldToLoad)}");
            MultiversePlugin.Log.LogInfo(
                $"[M2] config sources: {EnvExportEdge}={Show(Env(EnvExportEdge))} {EnvOpenEdge}={Show(Env(EnvOpenEdge))} " +
                $"{EnvRingSlot}={Show(Env(EnvRingSlot))} {EnvPort}={Show(Env(EnvPort))} " +
                $"{EnvBorderWidth}={Show(Env(EnvBorderWidth))} {EnvWorld}={Show(Env(EnvWorld))} " +
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
