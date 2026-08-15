using System;
using BepInEx;
using BepInEx.Configuration;
using BepInEx.Logging;
using UnityEngine;

namespace BibitesMultiverse
{
    [BepInPlugin(Guid, Name, Version)]
    public class MultiversePlugin : BaseUnityPlugin
    {
        public const string Guid = "dev.multiverse.bibites";
        public const string Name = "Bibites Multiverse";
        public const string Version = "0.6.4";

        /// <summary>Set this to 1/true/yes to turn the auto-test on without editing the config file.</summary>
        public const string AutoTestEnvironmentVariable = "MULTIVERSE_AUTOTEST";

        internal static ManualLogSource Log;

        private void Awake()
        {
            Log = Logger;
            Log.LogInfo(
                $"{Name} {Version} loaded — {ContractA.Protocol} client. The dial is authenticated: " +
                $"{ContractAToken.HeaderName}: Bearer on the HTTP upgrade, from {ContractAToken.EnvTokenFile} or " +
                $"{ContractAToken.EnvToken}, re-read on every dial and never logged (§21 A47). Two-way lanes " +
                "(D17, §18 A38): all four edges declared by default, a capture band on each (§4.3.1) and the corner " +
                "rule at all four corners (§4.3.2). Plus the migration exclusion list (D18, §18 A39), the lineage " +
                "annex, species identity in the envelope (§16), the active-species census on the heartbeat (§17), " +
                "this world's settings on the handshake (§19 A42 — read-only, never a control surface), wrap-on " +
                "containment (D10), periodic world saves (D14) and the two-lane portal (WP7).");
            Log.LogInfo($"Application.version = {Application.version}");
            Log.LogInfo($"Application.unityVersion = {Application.unityVersion}");

            gameObject.AddComponent<RoundTripCommand>();
            Log.LogInfo($"Round-trip dev command armed — press {RoundTripCommand.Hotkey} inside a running simulation.");

            MultiverseConfig config = ReadConfig();

            // Armed before anything can save. Like the saver below it is deliberately NOT gated on the
            // multiverse client: the defect it guards is the game's own, it stops a world saving at
            // all, and a world is worth saving whether or not this instance is wired into a map.
            SpeciesHistoryGuard.Apply(Guid);

            // The save-stall decomposition, armed next to the guard because it times the same call and
            // one of its spans is the guard. Measurement only: it patches nothing that changes what a
            // save does, and if a span will not resolve every phase reads 0 and the save is unaffected.
            SavePhases.Apply(Guid);

            // Also the game's own behaviour rather than the multiverse's, and also armed before a world
            // loads: the minimum-FPS servo starts governing the moment TimeKeeper does, so a later hook
            // would spend the first seconds fighting it.
            MinFpsGovernor.Apply(Guid);

            // The saver is created **before** the client, because the client hands it the "a MIGRATE_IN
            // is waiting" gate of Risk 3 at Initialize time and AddComponent runs Awake immediately.
            StartWorldSaver(config);
            StartMultiverseClient(config);
            StartDevCommands(config);
            StartSpectatorDirector();
            StartAutoTest();
        }

        private MultiverseConfig ReadConfig()
        {
            MultiverseConfig config;
            try
            {
                config = MultiverseConfig.Read(Config);
            }
            catch (Exception e)
            {
                Log.LogError($"[M2] configuration failed — the multiverse client stays off: {e}");
                return null;
            }

            config.LogSummary();
            return config;
        }

        /// <summary>
        /// D14's periodic save. It is deliberately **not** gated on the multiverse client: a world is
        /// worth saving whether or not this instance is wired into a map, and the save timer is the one
        /// thing T1 proved the rig could not do without.
        /// </summary>
        private void StartWorldSaver(MultiverseConfig config)
        {
            try
            {
                WorldSaver saver = gameObject.AddComponent<WorldSaver>();
                if (config != null)
                {
                    saver.IntervalMinutes = config.SaveMinutes;
                    saver.Keep = config.SaveKeep;
                    saver.SaveOnQuit = config.SaveOnQuit;
                    saver.PreferredWorldName = config.WorldToLoad;
                }
            }
            catch (Exception e)
            {
                Log.LogError($"{WorldSaver.Prefix} the world saver could not be created — {e}");
            }
        }

        /// <summary>
        /// The M2 rig's dev channel. Armed by MULTIVERSE_CMD_FILE (the orchestration script's command
        /// file) or MULTIVERSE_FAMILY_REPORT; the forced-export hotkey works either way, so a manual
        /// run needs neither variable.
        /// </summary>
        private void StartDevCommands(MultiverseConfig config)
        {
            string commandFile = (MultiverseConfig.Env(DevCommands.EnvCommandFile) ?? string.Empty).Trim();
            string familyReport = (MultiverseConfig.Env(DevCommands.EnvFamilyReport) ?? string.Empty).Trim();

            float seconds = 0f;
            if (!string.IsNullOrEmpty(familyReport)
                && !float.TryParse(familyReport, System.Globalization.NumberStyles.Float, System.Globalization.CultureInfo.InvariantCulture, out seconds))
            {
                Log.LogWarning($"[M2] {DevCommands.EnvFamilyReport}='{familyReport}' is not a number of seconds — the periodic family report stays off.");
                seconds = 0f;
            }

            DevCommands commands = gameObject.AddComponent<DevCommands>();
            commands.CommandFile = commandFile;
            commands.FamilyReportSeconds = seconds;
            commands.FallbackWorldName = (config != null) ? config.WorldToLoad : string.Empty;
        }

        /// <summary>
        /// The public broadcast camera is opt-in and affects selection and camera position only. It is
        /// deliberately independent of the multiverse client: an offline rehearsal must be able to
        /// prove rendering without joining the public map or changing any ecology.
        /// </summary>
        private void StartSpectatorDirector()
        {
            if (!SpectatorDirector.Requested())
            {
                return;
            }

            try
            {
                gameObject.AddComponent<SpectatorDirector>();
            }
            catch (Exception e)
            {
                Log.LogError($"{SpectatorDirector.Prefix} the spectator director could not be created — {e}");
            }
        }

        /// <summary>The Contract A client: the world loader, the Harmony border hook and the transport.</summary>
        private void StartMultiverseClient(MultiverseConfig config)
        {
            if (config == null)
            {
                return;
            }

            if (!string.IsNullOrEmpty(config.WorldToLoad))
            {
                WorldLoader loader = gameObject.AddComponent<WorldLoader>();
                loader.SaveName = config.WorldToLoad;
            }

            if (!config.Enabled)
            {
                Log.LogInfo(
                    $"[M2] the export edges are set to 'none' ({MultiverseConfig.EnvExportEdges}, " +
                    $"{MultiverseConfig.EnvExportEdge}, {MultiverseConfig.EnvOpenEdge} or [M4] ExportEdges) — the Contract A " +
                    "client, the capture bands, the crossing counters and the portal all stay off. Unset the variable to get " +
                    "D17's default of all four edges. The periodic world save is unaffected.");
                return;
            }

            if (!BorderPatches.Apply(Guid))
            {
                Log.LogError("[M2] the border hook could not be installed — the multiverse client stays off.");
                return;
            }

            MultiverseClient client = gameObject.AddComponent<MultiverseClient>();
            client.Initialize(config);
        }

        private void StartAutoTest()
        {
            ConfigEntry<bool> autoTest = Config.Bind(
                "M1",
                "AutoTest",
                false,
                "Run the unattended M1 exit test at startup: load the '" + AutoTest.SaveName + "' world, " +
                "round-trip one organism, watch it for 30 simulated seconds, save the world, then quit. " +
                "The environment variable " + AutoTestEnvironmentVariable + "=1 turns it on as well.");

            string fromEnvironment = ReadEnvironmentFlag();
            bool enabled = autoTest.Value || IsTruthy(fromEnvironment);
            if (!enabled)
            {
                Log.LogInfo(
                    $"Auto-test is off ([M1] AutoTest={autoTest.Value}, {AutoTestEnvironmentVariable}=" +
                    $"{(string.IsNullOrEmpty(fromEnvironment) ? "<unset>" : fromEnvironment)}).");
                return;
            }

            Log.LogInfo(
                $"Auto-test is ON ([M1] AutoTest={autoTest.Value}, {AutoTestEnvironmentVariable}=" +
                $"{(string.IsNullOrEmpty(fromEnvironment) ? "<unset>" : fromEnvironment)}).");
            gameObject.AddComponent<AutoTest>();
        }

        private static string ReadEnvironmentFlag()
        {
            try
            {
                return Environment.GetEnvironmentVariable(AutoTestEnvironmentVariable);
            }
            catch (Exception e)
            {
                Log.LogWarning($"could not read {AutoTestEnvironmentVariable}: {e.Message}");
                return null;
            }
        }

        private static bool IsTruthy(string value)
        {
            if (string.IsNullOrEmpty(value))
            {
                return false;
            }

            string normalized = value.Trim().ToLowerInvariant();
            return normalized == "1" || normalized == "true" || normalized == "yes" || normalized == "on";
        }
    }
}
