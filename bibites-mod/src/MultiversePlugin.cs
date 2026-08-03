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
        public const string Version = "0.2.0";

        /// <summary>Set this to 1/true/yes to turn the auto-test on without editing the config file.</summary>
        public const string AutoTestEnvironmentVariable = "MULTIVERSE_AUTOTEST";

        internal static ManualLogSource Log;

        private void Awake()
        {
            Log = Logger;
            Log.LogInfo($"{Name} {Version} loaded — M2 Contract A client, border strip, export and import pipelines.");
            Log.LogInfo($"Application.version = {Application.version}");
            Log.LogInfo($"Application.unityVersion = {Application.unityVersion}");

            gameObject.AddComponent<RoundTripCommand>();
            Log.LogInfo($"Round-trip dev command armed — press {RoundTripCommand.Hotkey} inside a running simulation.");

            MultiverseConfig config = StartMultiverseClient();
            StartDevCommands(config);
            StartAutoTest();
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

        /// <summary>The M2 client: configuration, the Harmony border hook and the Contract A transport.</summary>
        private MultiverseConfig StartMultiverseClient()
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

            if (!string.IsNullOrEmpty(config.WorldToLoad))
            {
                WorldLoader loader = gameObject.AddComponent<WorldLoader>();
                loader.SaveName = config.WorldToLoad;
            }

            if (!config.Enabled)
            {
                Log.LogInfo(
                    $"[M2] no border edge is configured ({MultiverseConfig.EnvOpenEdge} or [M2] OpenEdge) — " +
                    "the Contract A client, the border strip and the crossing counters all stay off.");
                return config;
            }

            if (!BorderPatches.Apply(Guid))
            {
                Log.LogError("[M2] the border hook could not be installed — the multiverse client stays off.");
                return config;
            }

            MultiverseClient client = gameObject.AddComponent<MultiverseClient>();
            client.Initialize(config);
            return config;
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
