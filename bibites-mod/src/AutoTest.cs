using System;
using System.Collections;
using System.Collections.Generic;
using System.IO;
using System.IO.Compression;
using ManagementScripts;
using Newtonsoft.Json.Linq;
using OneUseScripts;
using ScriptHelpers;
using SimulationScripts.BibiteScripts;
using UnityEngine;
using UnityEngine.SceneManagement;

namespace BibitesMultiverse
{
    /// <summary>
    /// The unattended M1 exit test. Enabled by the [M1] AutoTest config entry or MULTIVERSE_AUTOTEST=1.
    ///
    /// Phases, all logged with the grepable [M1-AUTOTEST] prefix:
    ///   1. wait for the main menu
    ///   2. seed the "M1-AutoTest" world if it does not exist yet (fresh sim from a stock scenario,
    ///      grown until a usable organism exists, then saved under that name)
    ///   3. load the "M1-AutoTest" world through GameManager.StartGame(path) — the same call the
    ///      Load Game menu makes (UIScripts/LoadGamePanel.cs:225)
    ///   4. pick a target: adult, with synapses and stomach content; any living bibite after a timeout
    ///   5. run the round trip (RoundTrip.Run)
    ///   6. watch the respawned organism for ~30 simulated seconds
    ///   7. save the world back to "M1-AutoTest" — the stale-reference check
    ///   8. print exactly one RESULT line and quit
    ///
    /// It never reads, writes or deletes any save other than "M1-AutoTest.zip".
    /// </summary>
    internal class AutoTest : MonoBehaviour
    {
        internal const string Prefix = "[M1-AUTOTEST]";
        internal const string SaveName = "M1-AutoTest";

        private const float MenuTimeout = 240f;
        private const float SimTimeout = 300f;
        private const float SeedPreferredTimeout = 240f;
        private const float SeedAnyTimeout = 120f;
        private const float SaveTimeout = 180f;
        private const float TargetPreferredTimeout = 90f;
        private const float ObserveSimSeconds = 30f;
        private const float ObserveRealTimeout = 300f;
        private const float SeedTimeScale = 10f;
        private const float TestTimeScale = 3f;

        private readonly List<string> unityErrors = new List<string>();
        private bool captureUnityErrors;
        private int unityErrorsAtPhaseStart;

        private bool timedOut;
        private string failure;
        private bool finished;

        private static string SaveFilePath => WorldSeeder.SavePathFor(SaveName);

        private static void Log(string message)
        {
            MultiversePlugin.Log.LogInfo($"{Prefix} {message}");
        }

        private static void Warn(string message)
        {
            MultiversePlugin.Log.LogWarning($"{Prefix} {message}");
        }

        private void Awake()
        {
            Application.logMessageReceived += OnUnityLog;
            StartCoroutine(RunAutoTest());
        }

        private void OnDestroy()
        {
            Application.logMessageReceived -= OnUnityLog;
        }

        private void OnUnityLog(string condition, string stackTrace, LogType type)
        {
            if (type != LogType.Error && type != LogType.Exception && type != LogType.Assert)
            {
                return;
            }

            string line = $"{type}: {condition}";
            unityErrors.Add(line);
            if (captureUnityErrors)
            {
                Warn($"unity {line}");
                string firstFrame = FirstStackFrame(stackTrace);
                if (!string.IsNullOrEmpty(firstFrame))
                {
                    Warn($"unity   at {firstFrame}");
                }
            }
        }

        private static string FirstStackFrame(string stackTrace)
        {
            if (string.IsNullOrEmpty(stackTrace))
            {
                return null;
            }

            string[] lines = stackTrace.Split('\n');
            return (lines.Length > 0) ? lines[0].Trim() : null;
        }

        // ---- the test ---------------------------------------------------------------------

        private IEnumerator RunAutoTest()
        {
            Log($"auto-test mode armed — game {Application.version}, Unity {Application.unityVersion}");
            Log($"save under test: {SaveFilePath}");

            yield return RunPhases();

            if (finished)
            {
                yield break;
            }

            finished = true;
            if (string.IsNullOrEmpty(failure))
            {
                Log("RESULT: PASS");
            }
            else
            {
                MultiversePlugin.Log.LogError($"{Prefix} RESULT: FAIL — {failure}");
            }

            Log($"unity errors/exceptions seen during the whole run: {unityErrors.Count}");
            Log("quitting the game (Application.Quit).");
            yield return new WaitForSecondsRealtime(3f);
            Application.Quit();

            // If the process is still here after this, something held the quit; say so rather than hang.
            yield return new WaitForSecondsRealtime(20f);
            Warn("Application.Quit did not terminate the process after 20 s.");
        }

        private IEnumerator RunPhases()
        {
            // ---- phase 1: main menu -------------------------------------------------------
            Log("phase 1: waiting for the main menu.");
            yield return WaitFor(MenuReady, MenuTimeout, "the main menu");
            if (timedOut)
            {
                failure = "the main menu never became ready";
                yield break;
            }

            Log($"phase 1: main menu ready (scene='{SceneManager.GetActiveScene().name}').");

            // ---- phase 2: seed the world if needed ----------------------------------------
            if (File.Exists(SaveFilePath))
            {
                Log($"phase 2: '{SaveName}' already exists ({new FileInfo(SaveFilePath).Length} bytes) — skipping the seed.");
            }
            else
            {
                Log($"phase 2: '{SaveName}' does not exist — seeding it from a fresh simulation. No other save is touched.");
                yield return SeedWorld();
                if (!string.IsNullOrEmpty(failure))
                {
                    yield break;
                }
            }

            // ---- phase 3: load the world --------------------------------------------------
            Log($"phase 3: loading '{SaveName}' through GameManager.StartGame(path).");
            bool started = false;
            try
            {
                GameManager.StartGame(SaveFilePath);
                started = true;
            }
            catch (Exception e)
            {
                failure = "GameManager.StartGame threw: " + e.Message;
                MultiversePlugin.Log.LogError($"{Prefix} {e}");
            }

            if (!started)
            {
                yield break;
            }

            yield return WaitFor(SimulationReady, SimTimeout, "the simulation scene");
            if (timedOut)
            {
                failure = "the simulation scene never became ready after the load";
                yield break;
            }

            yield return WaitFor(() => LivingCount() > 0, SimTimeout, "living bibites in the loaded world");
            if (timedOut)
            {
                failure = "the loaded world contains no living bibite";
                yield break;
            }

            PrepareSimulation(TestTimeScale);
            Log($"phase 3: world loaded — gameName='{SimulationManager.gameName}' " +
                $"bibites={LivingCount()} eggs={EggCount()} simulatedTime={TimeKeeper.simulatedTime:F1}s");

            // ---- phase 4: pick a target ---------------------------------------------------
            Log("phase 4: looking for an adult with synapses and stomach content.");
            yield return WaitFor(() => FindTarget(preferred: true) != null, TargetPreferredTimeout, "a preferred target");

            BibiteBody target = FindTarget(preferred: true);
            if (target == null)
            {
                Warn("phase 4: no preferred target within the timeout — falling back to any living bibite.");
                target = FindTarget(preferred: false);
            }

            if (target == null)
            {
                failure = "no living bibite to run the round trip on";
                yield break;
            }

            Log($"phase 4: target id={((target.id != null) ? target.id.id : 0)} " +
                $"maturity={((target.growth != null) ? target.growth.maturity : float.NaN):F3} " +
                $"synapses={SynapseCount(target)} stomachContents={StomachCount(target)} " +
                $"stomachAmount={StomachAmount(target):F4} latched={IsLatched(target)}");

            // ---- phase 5: the round trip --------------------------------------------------
            Log("phase 5: running the round trip.");
            BeginErrorWindow();
            RoundTripResult result = new RoundTripResult();
            yield return RoundTrip.Run(result, target);
            int roundTripErrors = EndErrorWindow();

            if (!result.Success)
            {
                failure = "round trip failed: " + (result.failure ?? "unknown reason");
                yield break;
            }

            Log($"phase 5: round trip done — idPreserved={result.IdPreserved} " +
                $"beforeId={result.beforeId} afterId={result.afterId} " +
                $"verdict={(result.PayloadsEqual ? "EQUAL" : $"DIFFERENT ({result.diffs.Count} path(s))")}");

            if (result.diffs != null)
            {
                foreach (string diff in result.diffs)
                {
                    Log($"phase 5: payload diff {diff}");
                }
            }

            if (!result.IdPreserved)
            {
                failure = $"the entity ID changed across the round trip ({result.beforeId} -> {result.afterId})";
                yield break;
            }

            if (roundTripErrors > 0)
            {
                failure = $"{roundTripErrors} unity error(s)/exception(s) during the round trip";
                yield break;
            }

            // ---- phase 6: observe ---------------------------------------------------------
            BibiteBody respawned = result.newBody;
            double startTime = TimeKeeper.simulatedTime;
            Log($"phase 6: watching the respawned organism for {ObserveSimSeconds:F0} simulated seconds " +
                $"(from t={startTime:F1}s).");

            BeginErrorWindow();
            yield return WaitFor(
                () => TimeKeeper.simulatedTime - startTime >= ObserveSimSeconds || !IsAlive(respawned),
                ObserveRealTimeout,
                "30 simulated seconds");
            int observeErrors = EndErrorWindow();

            if (timedOut)
            {
                failure = $"the simulation did not advance {ObserveSimSeconds:F0} simulated seconds within " +
                          $"{ObserveRealTimeout:F0} s of real time (advanced {TimeKeeper.simulatedTime - startTime:F1}s)";
                yield break;
            }

            if (!IsAlive(respawned))
            {
                failure = DeathReason(respawned);
                yield break;
            }

            RoundTrip.LogTargetState($"{Prefix} phase 6 after {TimeKeeper.simulatedTime - startTime:F1}s sim", respawned);
            Log($"phase 6: organism still alive — dying={respawned.dying} dead={respawned.dead} " +
                $"health={respawned.Health.Amount:F4}/{respawned.Health.MaxAmount:F4} " +
                $"energy={respawned.Energy.Amount:F4} unityErrors={observeErrors}");

            // ---- phase 7: world save ------------------------------------------------------
            Log($"phase 7: saving the world back to '{SaveName}' — the stale-reference check.");
            yield return SaveWorld(SaveFilePath);
            if (!string.IsNullOrEmpty(failure))
            {
                yield break;
            }

            Log($"phase 7: world save completed with no exception ({new FileInfo(SaveFilePath).Length} bytes).");
        }

        // ---- phase 2: seeding -------------------------------------------------------------

        private IEnumerator SeedWorld()
        {
            WorldOpResult seed = new WorldOpResult();
            yield return WorldSeeder.Seed(
                SaveName,
                SeedTimeScale,
                () => FindTarget(preferred: true) != null,
                SeedPreferredTimeout,
                () => LivingCount() > 0,
                SeedAnyTimeout,
                Prefix,
                seed);

            if (!seed.ok)
            {
                failure = seed.failure;
                yield break;
            }

            Log($"phase 2: '{SaveName}' written ({new FileInfo(SaveFilePath).Length} bytes).");
        }

        // ---- world save --------------------------------------------------------------------

        /// <summary>
        /// <see cref="WorldSeeder.SaveWorld"/> drives SaveSystem.CreateSave by hand, so an exception
        /// inside the save (the stale-reference symptom of Risk 5) reaches a catch instead of being
        /// swallowed by Unity's coroutine runner. This wrapper adds the auto-test's own error window
        /// on top, because a Unity error logged during the save is a failure even when CreateSave
        /// itself returns.
        /// </summary>
        private IEnumerator SaveWorld(string path)
        {
            BeginErrorWindow();

            WorldOpResult result = new WorldOpResult();
            yield return WorldSeeder.SaveWorld(path, Prefix, result);

            int errors = EndErrorWindow();
            if (!result.ok)
            {
                failure = result.failure;
                yield break;
            }

            if (errors > 0)
            {
                failure = errors + " unity error(s)/exception(s) during the world save";
            }
        }

        // ---- conditions ---------------------------------------------------------------------

        private static bool MenuReady()
        {
            return SceneManager.GetActiveScene().name == "Menu"
                && GameManager.activeScene == BibiteScenes.Menu
                && ProceduralSpriteManager.Instance != null
                && ProceduralSpriteManager.Instance.done;
        }

        private static bool SimulationReady()
        {
            return GameManager.isSim
                && SceneManager.GetActiveScene().name == "Main"
                && SimulationManager.Instance != null
                && SaveSystem.instance != null
                && SaveController.Instance != null
                && TimeController.Instance != null
                && BibiteTracker.instance != null
                && BibiteTracker.instance.bibites != null
                && WorldObjectsSpawner.Instance != null;
        }

        private static void PrepareSimulation(float timeScale)
        {
            try
            {
                // Runtime-only toggle: it does not touch the user's saved AutoSave preference.
                SaveController.Instance.ToggleAutoSave(false);
            }
            catch (Exception e)
            {
                Warn($"could not disable autosave: {e.Message}");
            }

            try
            {
                // >0 routes through TimeController.ForcePlay, which also clears PauseAfterLoad.
                TimeController.targetTimeScale.SetValue(timeScale);
            }
            catch (Exception e)
            {
                Warn($"could not set the time scale: {e.Message}");
            }
        }

        private static int LivingCount()
        {
            int count = 0;
            BibiteTracker tracker = BibiteTracker.instance;
            if (tracker == null || tracker.bibites == null)
            {
                return 0;
            }

            foreach (BibiteBody body in tracker.bibites)
            {
                if (IsAlive(body))
                {
                    count++;
                }
            }

            return count;
        }

        private static int EggCount()
        {
            BibiteTracker tracker = BibiteTracker.instance;
            return (tracker != null && tracker.eggs != null) ? tracker.eggs.Count : 0;
        }

        private static bool IsAlive(BibiteBody body)
        {
            return GameBridge.IsUsable(body) && !body.dead && !body.dying && body.Health.Amount > 0f;
        }

        private static string DeathReason(BibiteBody body)
        {
            if (body == null || body.Equals(null))
            {
                return "the respawned organism was destroyed during the 30 s observation";
            }

            if (body.destroyed)
            {
                return "the respawned organism was destroyed during the 30 s observation";
            }

            if (body.dead)
            {
                return "the respawned organism died during the 30 s observation";
            }

            if (body.dying)
            {
                return "the respawned organism started dying during the 30 s observation";
            }

            return $"the respawned organism lost all health during the 30 s observation (health={body.Health.Amount:F4})";
        }

        private static int SynapseCount(BibiteBody body)
        {
            NEATBrain brain = (body != null) ? body.brain : null;
            if (brain == null)
            {
                brain = (body != null) ? body.GetComponent<NEATBrain>() : null;
            }

            return (brain != null && brain.Synapses != null) ? brain.Synapses.Length : 0;
        }

        private static int StomachCount(BibiteBody body)
        {
            return (body != null && body.stomach != null) ? body.stomach.nContent : 0;
        }

        private static float StomachAmount(BibiteBody body)
        {
            return (body != null && body.stomach != null) ? body.stomach.totalAmount : 0f;
        }

        /// <summary>
        /// True when another organism holds this one in its mouth. BibiteMouth.links is public
        /// (BibiteMouth.cs:37) and holds the FixedJoint2D of every grabbed object.
        /// </summary>
        private static bool IsLatched(BibiteBody body)
        {
            Rigidbody2D rigidbody = (body != null) ? body.GetComponent<Rigidbody2D>() : null;
            if (rigidbody == null)
            {
                return false;
            }

            BibiteTracker tracker = BibiteTracker.instance;
            if (tracker == null || tracker.bibites == null)
            {
                return false;
            }

            foreach (BibiteBody other in tracker.bibites)
            {
                if (other == null || other == body || other.mouth == null || other.mouth.links == null)
                {
                    continue;
                }

                foreach (FixedJoint2D link in other.mouth.links)
                {
                    if (link != null && link.connectedBody == rigidbody)
                    {
                        return true;
                    }
                }
            }

            return false;
        }

        private static BibiteBody FindTarget(bool preferred)
        {
            BibiteTracker tracker = BibiteTracker.instance;
            if (tracker == null || tracker.bibites == null)
            {
                return null;
            }

            BibiteBody fallback = null;
            foreach (BibiteBody body in tracker.bibites)
            {
                if (!IsAlive(body))
                {
                    continue;
                }

                if (IsLatched(body))
                {
                    continue;
                }

                if (fallback == null)
                {
                    fallback = body;
                }

                bool adult = body.growth != null && body.growth.maturity >= 1f;
                if (adult && SynapseCount(body) > 0 && StomachCount(body) > 0 && StomachAmount(body) > 0f)
                {
                    return body;
                }
            }

            return preferred ? null : fallback;
        }

        // ---- plumbing ------------------------------------------------------------------------

        private IEnumerator WaitFor(Func<bool> condition, float timeoutSeconds, string what)
        {
            timedOut = false;
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
                    Warn($"waiting for {what}: the check threw ({e.Message}) — retrying.");
                    satisfied = false;
                }

                if (satisfied)
                {
                    yield break;
                }

                if (Time.realtimeSinceStartup > deadline)
                {
                    timedOut = true;
                    Warn($"timed out after {timeoutSeconds:F0} s waiting for {what}.");
                    yield break;
                }

                if (Time.realtimeSinceStartup > nextHeartbeat)
                {
                    nextHeartbeat = Time.realtimeSinceStartup + 30f;
                    Log($"still waiting for {what} " +
                        $"(scene='{SceneManager.GetActiveScene().name}' bibites={LivingCount()} simTime={TimeKeeper.simulatedTime:F0}s)");
                }

                yield return null;
            }
        }

        private void BeginErrorWindow()
        {
            unityErrorsAtPhaseStart = unityErrors.Count;
            captureUnityErrors = true;
        }

        private int EndErrorWindow()
        {
            captureUnityErrors = false;
            return unityErrors.Count - unityErrorsAtPhaseStart;
        }
    }
}
