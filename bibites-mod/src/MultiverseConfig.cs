using System;
using System.Globalization;
using BepInEx.Configuration;

namespace BibitesMultiverse
{
    /// <summary>
    /// Per-instance configuration. Both game instances of the M2 rig share one plugins/ DLL and one
    /// config/ directory (dev_environment.md), so the environment variable always wins over the
    /// BepInEx config entry — the config entry only exists so a single-instance run can be set up
    /// without touching the environment.
    ///
    /// The WSL to Windows hop needs WSLENV to name each variable; see dev_environment.md.
    /// </summary>
    internal class MultiverseConfig
    {
        internal const string EnvSector = "MULTIVERSE_SECTOR";
        internal const string EnvPort = "MULTIVERSE_SIDECAR_PORT";
        internal const string EnvOpenEdge = "MULTIVERSE_OPEN_EDGE";
        internal const string EnvBorderWidth = "MULTIVERSE_BORDER_WIDTH";
        internal const string EnvWorld = "MULTIVERSE_WORLD";

        /// <summary>Fraction of the half-extent S used for the border strip when nothing overrides it.</summary>
        internal const float DefaultWidthFactor = 0.02f;

        /// <summary>A bibite is roughly 10 u long, so a strip thinner than this cannot be crossed reliably.</summary>
        internal const float MinimumWidth = 20f;

        internal bool Enabled;
        internal Edge BorderEdge = Edge.None;
        internal int Port = ContractA.DefaultPort;
        internal string SectorLabel = string.Empty;
        internal bool HasSector;
        internal int SectorX;
        internal int SectorY;
        internal float BorderWidthOverride;
        internal string WorldToLoad = string.Empty;

        internal string Url => "ws://127.0.0.1:" + Port.ToString(CultureInfo.InvariantCulture) + ContractA.UrlPath;

        internal static MultiverseConfig Read(ConfigFile config)
        {
            ConfigEntry<string> edgeEntry = config.Bind(
                "M2",
                "OpenEdge",
                string.Empty,
                "The single map edge this instance migrates across: N, S, E or W. Empty disables the " +
                "multiverse client entirely. The environment variable " + EnvOpenEdge + " overrides this.");

            ConfigEntry<string> sectorEntry = config.Bind(
                "M2",
                "Sector",
                string.Empty,
                "This instance's sector, as a letter (A, B, C...) or as 'x,y'. Advisory: the sidecar closes " +
                "the connection with 4001 when it disagrees. The environment variable " + EnvSector + " overrides this.");

            ConfigEntry<int> portEntry = config.Bind(
                "M2",
                "SidecarPort",
                ContractA.DefaultPort,
                "The loopback port the Contract A sidecar listens on. The environment variable " + EnvPort + " overrides this.");

            ConfigEntry<float> widthEntry = config.Bind(
                "M2",
                "BorderWidth",
                0f,
                "Border strip width W in world units. 0 derives it from the simulation half-extent S as " +
                DefaultWidthFactor.ToString(CultureInfo.InvariantCulture) + "*S, with a floor of " +
                MinimumWidth.ToString(CultureInfo.InvariantCulture) + " u. The environment variable " +
                EnvBorderWidth + " overrides this.");

            MultiverseConfig result = new MultiverseConfig();

            string edgeText = Prefer(Env(EnvOpenEdge), edgeEntry.Value);
            if (!string.IsNullOrEmpty(edgeText) && !ContractA.TryParseEdge(edgeText, out result.BorderEdge))
            {
                MultiversePlugin.Log.LogError(
                    $"[M2] config: '{edgeText}' is not a valid edge (expected N, S, E or W) — the multiverse client stays off.");
                result.BorderEdge = Edge.None;
            }

            result.Enabled = result.BorderEdge != Edge.None;

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

            string sectorText = Prefer(Env(EnvSector), sectorEntry.Value);
            result.SectorLabel = string.IsNullOrEmpty(sectorText) ? string.Empty : sectorText.Trim();
            if (!string.IsNullOrEmpty(result.SectorLabel))
            {
                if (TryParseSector(result.SectorLabel, out result.SectorX, out result.SectorY))
                {
                    result.HasSector = true;
                }
                else
                {
                    MultiversePlugin.Log.LogWarning(
                        $"[M2] config: sector '{result.SectorLabel}' is neither a letter nor 'x,y' — CONFIG_UPDATE.sector is omitted.");
                }
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

        /// <summary>
        /// "A" .. "Z" map to (0,0), (1,0), ... — enough for the M2 two-sector rig and for a linear
        /// row of sectors. "x,y" is the explicit form for anything else.
        /// </summary>
        internal static bool TryParseSector(string text, out int x, out int y)
        {
            x = 0;
            y = 0;
            if (string.IsNullOrEmpty(text))
            {
                return false;
            }

            string trimmed = text.Trim();
            int comma = trimmed.IndexOf(',');
            if (comma > 0)
            {
                string left = trimmed.Substring(0, comma).Trim();
                string right = trimmed.Substring(comma + 1).Trim();
                return int.TryParse(left, NumberStyles.Integer, CultureInfo.InvariantCulture, out x)
                    && int.TryParse(right, NumberStyles.Integer, CultureInfo.InvariantCulture, out y);
            }

            if (trimmed.Length == 1)
            {
                char c = char.ToUpperInvariant(trimmed[0]);
                if (c >= 'A' && c <= 'Z')
                {
                    x = c - 'A';
                    y = 0;
                    return true;
                }
            }

            return false;
        }

        internal void LogSummary()
        {
            MultiversePlugin.Log.LogInfo(
                $"[M2] config: enabled={Enabled} edge={ContractA.EdgeName(BorderEdge)} " +
                $"sector={(string.IsNullOrEmpty(SectorLabel) ? "<unset>" : SectorLabel)}" +
                $"{(HasSector ? $" ({SectorX},{SectorY})" : " (not sent)")} " +
                $"port={Port} url={Url} " +
                $"borderWidth={(BorderWidthOverride > 0f ? BorderWidthOverride.ToString("F2", CultureInfo.InvariantCulture) : $"{DefaultWidthFactor}*S (min {MinimumWidth})")} " +
                $"world={(string.IsNullOrEmpty(WorldToLoad) ? "<unset>" : WorldToLoad)}");
            MultiversePlugin.Log.LogInfo(
                $"[M2] config sources: {EnvOpenEdge}={Show(Env(EnvOpenEdge))} {EnvSector}={Show(Env(EnvSector))} " +
                $"{EnvPort}={Show(Env(EnvPort))} {EnvBorderWidth}={Show(Env(EnvBorderWidth))} {EnvWorld}={Show(Env(EnvWorld))} " +
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
