using System;
using System.Collections;
using System.IO;
using System.IO.Compression;
using ManagementScripts;
using Newtonsoft.Json.Linq;
using ScriptHelpers;
using UnityEngine;
using UnityEngine.SceneManagement;

namespace BibitesMultiverse
{
    /// <summary>
    /// The outcome of one world operation. A C# iterator cannot carry an <c>out</c> parameter, so
    /// every coroutine here reports through one of these instead.
    /// </summary>
    internal class WorldOpResult
    {
        internal bool ok;
        internal string failure;
        internal bool timedOut;

        internal void Fail(string why)
        {
            ok = false;
            failure = why;
        }
    }

    /// <summary>
    /// Seeding and saving a **named** world with no operator.
    ///
    /// The M1 auto-test grew this code to seed <c>M1-AutoTest</c>. The M2 rig needs the same thing for
    /// <c>M2-SectorA</c> and <c>M2-SectorB</c>, so it lives here and both callers use it —
    /// <see cref="AutoTest"/> for M1 and <see cref="WorldLoader"/> for the two-instance rig.
    ///
    /// Two rules the sprite-atlas gotcha of dev_environment.md forces on every caller: never call
    /// <c>GameManager.StartGame</c> before <see cref="MenuReady"/>, and never treat a world as loaded
    /// before <see cref="GameBridge.SimulationReady"/> — which includes
    /// <c>ProceduralSpriteManager.Instance.done</c>. A fixed sleep is not a substitute.
    /// </summary>
    internal static class WorldSeeder
    {
        /// <summary>The stock scenario a fresh seed starts from. Read-only; nothing is written back.</summary>
        internal const string SeedScenario = "Default.zip";

        private const float SimTimeout = 300f;
        private const float SaveTimeout = 180f;

        internal static string SavePathFor(string saveName)
        {
            return Path.Combine(SaveController.SavePath, saveName + ".zip");
        }

        internal static string ScenarioPath(string fileName)
        {
            return Path.Combine(Path.Combine(Application.persistentDataPath, "Scenarios"), fileName);
        }

        /// <summary>
        /// The main menu **and** the procedural sprite atlas. Loading a world before the atlas is built
        /// makes every bibite spawn throw out of <c>BibiteBody.FinalizeBirth</c>, and the symptom is an
        /// empty world rather than an exception (dev_environment.md).
        /// </summary>
        internal static bool MenuReady()
        {
            return SceneManager.GetActiveScene().name == "Menu"
                && GameManager.activeScene == BibiteScenes.Menu
                && ProceduralSpriteManager.Instance != null
                && ProceduralSpriteManager.Instance.done;
        }

        /// <summary>
        /// Grow a fresh simulation from the stock scenario and save it under <paramref name="saveName"/>.
        /// The caller supplies the two population conditions so the M1 test can insist on an adult with
        /// stomach content while the M2 rig only needs a living population.
        /// </summary>
        internal static IEnumerator Seed(
            string saveName,
            float timeScale,
            Func<bool> preferred,
            float preferredTimeout,
            Func<bool> fallback,
            float fallbackTimeout,
            string prefix,
            WorldOpResult result)
        {
            result.ok = false;

            string scenario = ScenarioPath(SeedScenario);
            if (File.Exists(scenario))
            {
                Info(prefix, $"seed: applying the stock scenario '{SeedScenario}' (read-only).");
                try
                {
                    ApplyScenario(scenario);
                }
                catch (Exception e)
                {
                    Warn(prefix, $"seed: could not apply the scenario ({e.Message}) — continuing with the current settings.");
                }
            }
            else
            {
                Warn(prefix, $"seed: {scenario} not found — continuing with the current scenario settings.");
            }

            SimulationManager.gameName = saveName;

            bool started = false;
            try
            {
                GameManager.StartGame();
                started = true;
            }
            catch (Exception e)
            {
                result.Fail("GameManager.StartGame threw while seeding: " + e.Message);
                MultiversePlugin.Log.LogError($"{prefix} {e}");
            }

            if (!started)
            {
                yield break;
            }

            WorldOpResult wait = new WorldOpResult();
            yield return WaitFor(GameBridge.SimulationReady, SimTimeout, "the simulation scene (seed)", prefix, wait);
            if (wait.timedOut)
            {
                result.Fail("the seeding simulation never became ready");
                yield break;
            }

            Prepare(timeScale, prefix);
            Info(prefix, $"seed: simulation running at {timeScale:F0}x — waiting for bibites to spawn and eat.");

            yield return WaitFor(preferred, preferredTimeout, "a preferred organism (seed)", prefix, wait);
            if (wait.timedOut)
            {
                Warn(prefix, "seed: the preferred population condition timed out — falling back.");
                yield return WaitFor(fallback, fallbackTimeout, "any living bibite (seed)", prefix, wait);
                if (wait.timedOut)
                {
                    result.Fail("the seeding simulation never produced a living bibite");
                    yield break;
                }
            }

            Info(prefix, $"seed: done — bibites={GameBridge.LivingPopulation()} eggs={GameBridge.EggPopulation()} " +
                $"simulatedTime={OneUseScripts.TimeKeeper.simulatedTime:F1}s. Saving as '{saveName}'.");

            string path = SavePathFor(saveName);
            yield return SaveWorld(path, prefix, result);
            if (!result.ok)
            {
                yield break;
            }

            Info(prefix, $"seed: '{saveName}' written ({new FileInfo(path).Length} bytes).");
        }

        /// <summary>Mirror of UIScripts/ScenarioSelectorPanel.cs:342-383 — read only, nothing is written back.</summary>
        internal static void ApplyScenario(string zipPath)
        {
            using (ZipArchive zip = ZipFile.Open(zipPath, ZipArchiveMode.Read))
            {
                JObject info = SaveSystem.ReadJObjectFromArchive(zip.GetEntry("scenario.info"));
                Utility.Version version = Utility.Version.Parse(info["version"].ToString());
                SerializationHelper.DeserializeScenario(SaveSystem.GetSettingsOfSave(zip), version, checkForModifiers: true);
                SaveSystem.CheckTemplatesOfArchive(zip);
            }
        }

        /// <summary>
        /// Drive <c>SaveSystem.CreateSave</c> ourselves instead of going through <c>SaveGame</c>'s
        /// <c>StartCoroutine</c>, so an exception inside the save — the stale-reference symptom of
        /// m2_considerations.md Risk 5 — lands in our own catch instead of being swallowed by Unity's
        /// coroutine runner.
        /// </summary>
        internal static IEnumerator SaveWorld(string path, string prefix, WorldOpResult result)
        {
            result.ok = false;
            result.failure = null;

            IEnumerator save;
            try
            {
                save = GameBridge.CreateSaveEnumerator(path);
            }
            catch (Exception e)
            {
                Warn(prefix, $"save: could not reach SaveSystem.CreateSave by reflection ({e.Message}) — using SaveController.SaveWorld.");
                save = null;
            }

            if (save != null)
            {
                while (true)
                {
                    bool moved;
                    try
                    {
                        moved = save.MoveNext();
                    }
                    catch (Exception e)
                    {
                        result.Fail("the world save threw: " + e.GetType().Name + ": " + e.Message);
                        MultiversePlugin.Log.LogError($"{prefix} world save exception: {e}");
                        yield break;
                    }

                    if (!moved)
                    {
                        break;
                    }

                    yield return save.Current;
                }
            }
            else
            {
                SaveSystem system = SaveSystem.instance;
                SaveController controller = SaveController.Instance;
                if (system == null || controller == null)
                {
                    result.Fail("no SaveSystem/SaveController to save the world with");
                    yield break;
                }

                bool done = false;
                UnityEngine.Events.UnityAction handler = () => { done = true; };
                system.onSavingDone.AddListener(handler);
                controller.SaveWorld(path);

                WorldOpResult wait = new WorldOpResult();
                yield return WaitFor(() => done, SaveTimeout, "the world save", prefix, wait);
                system.onSavingDone.RemoveListener(handler);
                if (wait.timedOut)
                {
                    result.Fail("the world save did not finish within " + SaveTimeout.ToString("F0") + " s");
                    yield break;
                }
            }

            if (!File.Exists(path))
            {
                result.Fail("the world save produced no file at " + path);
                yield break;
            }

            try
            {
                using (ZipArchive zip = ZipFile.Open(path, ZipArchiveMode.Read))
                {
                    if (zip.GetEntry("scene.bb8scene") == null)
                    {
                        result.Fail("the world save has no scene.bb8scene entry");
                        yield break;
                    }
                }
            }
            catch (Exception e)
            {
                result.Fail("the world save is not a readable archive: " + e.Message);
                yield break;
            }

            result.ok = true;
        }

        /// <summary>Runtime-only: it never touches the user's saved AutoSave preference.</summary>
        internal static void Prepare(float timeScale, string prefix)
        {
            SetAutoSave(false, prefix);
            SetTimeScale(timeScale, prefix);
        }

        internal static void SetAutoSave(bool enabled, string prefix)
        {
            try
            {
                SaveController.Instance.ToggleAutoSave(enabled);
            }
            catch (Exception e)
            {
                Warn(prefix, $"could not set autosave to {enabled}: {e.Message}");
            }
        }

        internal static void SetTimeScale(float timeScale, string prefix)
        {
            try
            {
                // >0 routes through TimeController.ForcePlay, which also clears PauseAfterLoad.
                TimeController.targetTimeScale.SetValue(timeScale);
            }
            catch (Exception e)
            {
                Warn(prefix, $"could not set the time scale: {e.Message}");
            }
        }

        internal static IEnumerator WaitFor(Func<bool> condition, float timeoutSeconds, string what, string prefix, WorldOpResult waiter)
        {
            waiter.timedOut = false;
            float deadline = Time.realtimeSinceStartup + timeoutSeconds;
            float nextHeartbeat = Time.realtimeSinceStartup + 30f;

            while (true)
            {
                bool satisfied;
                try
                {
                    satisfied = condition();
                }
                catch (Exception e)
                {
                    Warn(prefix, $"waiting for {what}: the check threw ({e.Message}) — retrying.");
                    satisfied = false;
                }

                if (satisfied)
                {
                    yield break;
                }

                if (Time.realtimeSinceStartup > deadline)
                {
                    waiter.timedOut = true;
                    Warn(prefix, $"timed out after {timeoutSeconds:F0} s waiting for {what}.");
                    yield break;
                }

                if (Time.realtimeSinceStartup > nextHeartbeat)
                {
                    nextHeartbeat = Time.realtimeSinceStartup + 30f;
                    Info(prefix, $"still waiting for {what} (scene='{SceneManager.GetActiveScene().name}' " +
                        $"bibites={GameBridge.LivingPopulation()} simTime={OneUseScripts.TimeKeeper.simulatedTime:F0}s)");
                }

                yield return null;
            }
        }

        private static void Info(string prefix, string message)
        {
            MultiversePlugin.Log.LogInfo($"{prefix} {message}");
        }

        private static void Warn(string prefix, string message)
        {
            MultiversePlugin.Log.LogWarning($"{prefix} {message}");
        }
    }
}
