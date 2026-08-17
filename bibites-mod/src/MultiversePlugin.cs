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
        public const string Version = "0.6.7";

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

            // ONE CONFIG-FILE WRITE PER GAME START, NOT FOURTEEN. BepInEx 5.4 saves the whole file
            // from inside every Bind, and each of those writes is a window in which any other holder
            // of the path — a second instance, an anti-virus, a search indexer walking a freshly
            // installed plugin folder — makes the bind throw. Suspend the per-entry save here, bind
            // everything, and write once at the end of Awake (LOCAL-CONFIGRACE).
            bool configSaves = MultiverseConfig.SuspendSaves(Config);

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

            // Also the game's own behaviour rather than the multiverse's, and also not gated on the
            // client: SimulationManager.Start resets every world it loads to x1, in code, and this is
            // what puts the configured speed back. Created before the world loader below, so it is
            // already watching when a world arrives.
            StartStartupTimescale();

            // The saver is created **before** the client, because the client hands it the "a MIGRATE_IN
            // is waiting" gate of Risk 3 at Initialize time and AddComponent runs Awake immediately.
            StartWorldSaver(config);
            StartMultiverseClient(config);
            StartDevCommands(config);
            StartSpectatorDirector();
            StartAutoTest();

            // Every entry is bound by now, so this is the only write, and its failure is a warning
            // rather than the end of the client: see MultiverseConfig.PersistConfig.
            MultiverseConfig.PersistConfig(Config, configSaves);
        }

        /// <summary>
        /// The speed a world starts at (<see cref="StartupTimescale"/>). Armed for every instance,
        /// because the reset to x1 it answers is the game's own and happens whether or not this world
        /// is on a map.
        /// </summary>
        private void StartStartupTimescale()
        {
            try
            {
                gameObject.AddComponent<StartupTimescale>();
            }
            catch (Exception e)
            {
                Log.LogError(
                    $"{StartupTimescale.Prefix} the startup time scale could not be armed — every world this game " +
                    $"loads will start at the game's own x1: {e}");
            }
        }

        /// <summary>
        /// The configuration, and it never returns null for a reason a file can cause.
        ///
        /// **This used to be the silent killer.** Any throw here — in practice a
        /// <c>Sharing violation</c> from inside BepInEx's own <c>ConfigFile.Save</c>, which it calls
        /// from every <c>Bind</c> — turned the whole Contract A client off. The game then loaded, the
        /// mod framework reported a clean start, no world was ever auto-loaded, no heartbeat was ever
        /// sent, and the installer, the launcher and the sidecar all reported success while the game
        /// sat at the main menu (LOCAL-CONFIGRACE; observed on a fresh single install, 2026-08-17).
        ///
        /// The settings are not in that file in any case. Every one of them is read from the
        /// environment afterwards, and a packaged install sets all of them there. So a file that
        /// cannot be read costs its shipped defaults for that start and nothing else, and the client
        /// stays on.
        /// </summary>
        private MultiverseConfig ReadConfig()
        {
            MultiverseConfig config;
            try
            {
                config = MultiverseConfig.Read(Config);
            }
            catch (Exception e)
            {
                Log.LogError(
                    "[M2] the settings file could not be used at all — this world carries on with the shipped " +
                    "defaults and its own environment, which is where a packaged install keeps every setting. " +
                    $"The multiverse client stays ON. {e}");
                try
                {
                    config = MultiverseConfig.Read(null);
                }
                catch (Exception fatal)
                {
                    Log.LogError(
                        "[M2] configuration failed even with no file involved — the multiverse client stays off. " +
                        $"This is a defect in the mod rather than a machine problem; please report it: {fatal}");
                    return null;
                }
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
        /// The public broadcast camera is opt-in. It controls spectator presentation and can request
        /// a simulation speed. It is deliberately independent of the multiverse client: an offline
        /// rehearsal must prove rendering without joining the public map or editing any organism.
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
                    "client, the capture bands, the crossing counters and the portal all stay off. The world-settings hook " +
                    "goes with them, so " + WorldSettings.EnvFertility + " is not applied either. Unset the variable to get " +
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
            bool autoTest = false;
            try
            {
                autoTest = Config.Bind(
                    "M1",
                    "AutoTest",
                    false,
                    "Run the unattended M1 exit test at startup: load the '" + AutoTest.SaveName + "' world, " +
                    "round-trip one organism, watch it for 30 simulated seconds, save the world, then quit. " +
                    "The environment variable " + AutoTestEnvironmentVariable + "=1 turns it on as well.").Value;
            }
            catch (Exception e)
            {
                // Same rule as every other entry: a settings file that will not co-operate costs this
                // one default and nothing else. The environment variable below still turns it on.
                Log.LogWarning(
                    $"[M1] the AutoTest entry could not be read from the settings file ({e.GetType().Name}: " +
                    $"{e.Message}) — treating it as off. {AutoTestEnvironmentVariable} still works.");
            }

            string fromEnvironment = ReadEnvironmentFlag();
            bool enabled = autoTest || IsTruthy(fromEnvironment);
            if (!enabled)
            {
                Log.LogInfo(
                    $"Auto-test is off ([M1] AutoTest={autoTest}, {AutoTestEnvironmentVariable}=" +
                    $"{(string.IsNullOrEmpty(fromEnvironment) ? "<unset>" : fromEnvironment)}).");
                return;
            }

            Log.LogInfo(
                $"Auto-test is ON ([M1] AutoTest={autoTest}, {AutoTestEnvironmentVariable}=" +
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
