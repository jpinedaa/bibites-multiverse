using System;
using System.Collections;
using System.IO;
using ManagementScripts;
using UnityEngine;

namespace BibitesMultiverse
{
    /// <summary>
    /// MULTIVERSE_WORLD=&lt;save name&gt; loads that world from the main menu with no operator, through
    /// <c>GameManager.StartGame(path)</c> — the same call the Load Game panel makes
    /// (UIScripts/LoadGamePanel.cs:225), proven by the M1 auto-test.
    ///
    /// When the save does not exist it is **seeded first**, through the same path the M1 auto-test
    /// uses for <c>M1-AutoTest</c> (<see cref="WorldSeeder"/>): apply the stock scenario, start a fresh
    /// simulation under that name, run it until it holds living bibites, save. That is what lets the
    /// M2 rig bring up <c>M2-SectorA</c> and <c>M2-SectorB</c> unattended on a clean profile.
    ///
    /// The M2 rig needs the loader itself as well: two game instances have to reach their own world
    /// unattended, and the mod only dials its sidecar while a world is loaded (contract-a.md §2). It
    /// touches no save other than the one it is told to load, and does nothing when the variable is
    /// unset.
    /// </summary>
    internal class WorldLoader : MonoBehaviour
    {
        private const float MenuTimeout = 240f;
        private const float LoadTimeout = 300f;
        private const float SeedTimeScale = 10f;
        private const float SeedPreferredTimeout = 240f;
        private const float SeedAnyTimeout = 120f;

        /// <summary>How many living bibites a seeded world should hold before it is written out.</summary>
        private const int SeedPreferredPopulation = 8;

        internal const string Prefix = "[M2-WORLD]";

        internal string SaveName;

        private void Start()
        {
            StartCoroutine(LoadWorld());
        }

        private IEnumerator LoadWorld()
        {
            string path = WorldSeeder.SavePathFor(SaveName);
            MultiversePlugin.Log.LogInfo($"{Prefix} {MultiverseConfig.EnvWorld}={SaveName} — waiting for the main menu to load '{path}'.");

            WorldOpResult waiter = new WorldOpResult();
            yield return WorldSeeder.WaitFor(WorldSeeder.MenuReady, MenuTimeout, "the main menu", Prefix, waiter);
            if (waiter.timedOut)
            {
                MultiversePlugin.Log.LogError($"{Prefix} the main menu did not become ready within {MenuTimeout:F0} s — '{SaveName}' was not loaded.");
                yield break;
            }

            if (!File.Exists(path))
            {
                MultiversePlugin.Log.LogInfo(
                    $"{Prefix} '{SaveName}' does not exist — seeding it from a fresh simulation. No other save is touched.");

                WorldOpResult seed = new WorldOpResult();
                yield return WorldSeeder.Seed(
                    SaveName,
                    SeedTimeScale,
                    () => GameBridge.LivingPopulation() >= SeedPreferredPopulation,
                    SeedPreferredTimeout,
                    () => GameBridge.LivingPopulation() > 0,
                    SeedAnyTimeout,
                    Prefix,
                    seed);

                if (!seed.ok)
                {
                    MultiversePlugin.Log.LogError($"{Prefix} seeding '{SaveName}' failed — {seed.failure}. Nothing is loaded.");
                    yield break;
                }

                // Seeding runs at its own fixed speed and leaves the world live, so the configured
                // startup speed — which was already applied when the seeding scene became ready — has
                // been overwritten by it. Put it back, once, on the world the participant now has.
                GetComponent<StartupTimescale>()?.Apply("the end of seeding");

                // The seeding run already left that world loaded and running, so there is nothing to
                // load a second time. Reloading it would only throw away the simulated time.
                MultiversePlugin.Log.LogInfo(
                    $"{Prefix} world '{SaveName}' seeded and live — population={GameBridge.LivingPopulation()} " +
                    $"eggs={GameBridge.EggPopulation()}.");
                yield break;
            }

            bool started = false;
            try
            {
                GameManager.StartGame(path);
                started = true;
            }
            catch (Exception e)
            {
                MultiversePlugin.Log.LogError($"{Prefix} GameManager.StartGame('{path}') threw — {e}");
            }

            if (!started)
            {
                yield break;
            }

            yield return WorldSeeder.WaitFor(GameBridge.SimulationReady, LoadTimeout, "the simulation scene", Prefix, waiter);
            if (waiter.timedOut)
            {
                MultiversePlugin.Log.LogError($"{Prefix} '{SaveName}' did not finish loading within {LoadTimeout:F0} s.");
                yield break;
            }

            MultiversePlugin.Log.LogInfo(
                $"{Prefix} world '{SaveName}' loaded — population={GameBridge.LivingPopulation()} eggs={GameBridge.EggPopulation()}.");
        }
    }
}
