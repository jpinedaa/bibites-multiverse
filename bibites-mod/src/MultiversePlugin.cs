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
        public const string Version = "0.1.0";

        /// <summary>Set this to 1/true/yes to turn the auto-test on without editing the config file.</summary>
        public const string AutoTestEnvironmentVariable = "MULTIVERSE_AUTOTEST";

        internal static ManualLogSource Log;

        private void Awake()
        {
            Log = Logger;
            Log.LogInfo($"{Name} {Version} loaded — M1 round-trip dev command.");
            Log.LogInfo($"Application.version = {Application.version}");
            Log.LogInfo($"Application.unityVersion = {Application.unityVersion}");

            gameObject.AddComponent<RoundTripCommand>();
            Log.LogInfo($"Round-trip dev command armed — press {RoundTripCommand.Hotkey} inside a running simulation.");

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
