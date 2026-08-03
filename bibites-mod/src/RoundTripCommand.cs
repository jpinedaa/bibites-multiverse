using System;
using System.Collections;
using System.Collections.Generic;
using System.IO;
using ManagementScripts;
using Newtonsoft.Json;
using Newtonsoft.Json.Linq;
using SimulationScripts.BibiteScripts;
using UnityEngine;

namespace BibitesMultiverse
{
    /// <summary>
    /// The M1 dev command: serialize → destroy → respawn → verify, on one live organism,
    /// through the game's own full-fidelity save path. Everything is wrapped so a failure
    /// logs instead of taking the simulation down with it.
    /// </summary>
    public class RoundTripCommand : MonoBehaviour
    {
        public const KeyCode Hotkey = KeyCode.F9;

        private const string LogFolder = "multiverse-logs";
        private const string BeforeFile = "roundtrip-before.json";
        private const string AfterFile = "roundtrip-after.json";
        private const int MaxReportedDiffs = 50;

        private static string cachedLogDirectory;

        private bool running;

        private class Context
        {
            public BibiteBody body;
            public GameObject gameObject;
            public string source;
            public int id;
            public JObject before;
            public string beforeJson;
            public OrganismLinks links;
        }

        private void Update()
        {
            try
            {
                if (!Input.GetKeyDown(Hotkey))
                {
                    return;
                }

                if (running)
                {
                    MultiversePlugin.Log.LogWarning("Round trip already running — ignoring this key press.");
                    return;
                }

                running = true;
                StartCoroutine(RunRoundTrip());
            }
            catch (Exception e)
            {
                running = false;
                MultiversePlugin.Log.LogError($"Round trip failed to start: {e}");
            }
        }

        private IEnumerator RunRoundTrip()
        {
            Context context = new Context();

            if (SerializeAndDestroy(context))
            {
                // UnityEngine.Object.Destroy is deferred to the end of the frame, so the
                // old GameObject is still alive right now. Give Unity that frame.
                yield return null;

                RespawnAndVerify(context);
            }

            running = false;
        }

        // ---- step 1: pick, serialize, record links, destroy -------------------------------

        private bool SerializeAndDestroy(Context context)
        {
            try
            {
                MultiversePlugin.Log.LogInfo("=== M1 round trip: start ===");

                context.body = GameBridge.PickTarget(out context.source);
                if (context.body == null)
                {
                    MultiversePlugin.Log.LogWarning(
                        "No target organism: nothing is selected and BibiteTracker.instance.bibites has no living entry. " +
                        "Load a simulation with at least one bibite first.");
                    return false;
                }

                context.gameObject = context.body.gameObject;
                context.id = (context.body.id != null) ? context.body.id.id : 0;
                LogTarget(context);

                context.before = GameBridge.SerializeBibite(context.gameObject);
                if (context.before == null)
                {
                    MultiversePlugin.Log.LogError("SaveSystem.SerializeBibite returned null (no BibiteGenes on the target) — aborting.");
                    return false;
                }

                context.beforeJson = Stamp(context.before, "multiverse M1 round trip (before)");
                MultiversePlugin.Log.LogInfo($"serialized BEFORE payload: {context.beforeJson.Length} chars");
                WritePayload(BeforeFile, context.beforeJson);

                context.links = OrganismLinks.Capture(context.body);
                context.links.LogSummary();

                // Remove from the tracker first so OnDestroy -> BibiteDeath does not bump
                // nDeath or the death-age quantiles (BibiteTracker.cs:172-179).
                BibiteTracker tracker = BibiteTracker.instance;
                bool removed = tracker != null && tracker.bibites != null && tracker.bibites.Remove(context.body);
                MultiversePlugin.Log.LogInfo(
                    $"removed from BibiteTracker.instance.bibites: {removed} (remaining={(tracker != null && tracker.bibites != null ? tracker.bibites.Count : -1)})");

                // Raw Destroy: no corpse, no meat, no stomach pellets, no eggs laid.
                // Precedent: UIScripts/SelectionPanel.cs:310. Never Die().
                UnityEngine.Object.Destroy(context.gameObject);
                MultiversePlugin.Log.LogInfo("destroyed the organism GameObject — waiting one frame for Unity to finish.");
                return true;
            }
            catch (Exception e)
            {
                MultiversePlugin.Log.LogError($"Round trip aborted during serialize/destroy: {e}");
                return false;
            }
        }

        // ---- step 2: respawn, relink, verify ---------------------------------------------

        private void RespawnAndVerify(Context context)
        {
            try
            {
                SaveSystem save = SaveSystem.instance;
                if (save == null)
                {
                    MultiversePlugin.Log.LogError("SaveSystem.instance is null — cannot respawn. The organism is gone.");
                    return;
                }

                GameObject spawned = null;
                try
                {
                    // birthAtMaturity stays null on purpose: any value resets health and energy
                    // and rolls RNG (BibiteBody.cs:451-456).
                    spawned = save.LoadBibiteOrEggFromData(context.beforeJson, resume: true, problems: null, birthAtMaturity: null);
                }
                catch (Exception e)
                {
                    MultiversePlugin.Log.LogError($"LoadBibiteOrEggFromData threw (unexpected — it normally swallows): {e}");
                }

                if (spawned == null)
                {
                    MultiversePlugin.Log.LogError(
                        "LoadBibiteOrEggFromData returned null. That wrapper swallows every exception " +
                        "(SaveSystem.cs:76-79) — re-running SaveSystem.LoadBibite by reflection to surface the real error.");
                    spawned = GameBridge.LoadBibiteDirect(JObject.Parse(context.beforeJson));
                    if (spawned != null)
                    {
                        MultiversePlugin.Log.LogWarning("The reflected LoadBibite call succeeded where the public wrapper failed.");
                    }
                }

                if (spawned == null)
                {
                    MultiversePlugin.Log.LogError("Respawn failed — no GameObject was produced. The organism is gone.");
                    return;
                }

                BibiteBody newBody = spawned.GetComponent<BibiteBody>();
                if (newBody == null)
                {
                    MultiversePlugin.Log.LogError("Respawned GameObject has no BibiteBody — aborting the verify step.");
                    return;
                }

                int newId = (newBody.id != null) ? newBody.id.id : 0;
                MultiversePlugin.Log.LogInfo(
                    $"respawned: id={newId} (before={context.id}) idPreserved={(newId == context.id)} " +
                    $"tracked={(BibiteTracker.instance != null && BibiteTracker.instance.bibites != null && BibiteTracker.instance.bibites.Contains(newBody))}");

                if (context.links != null)
                {
                    context.links.Restore(newBody);
                }

                JObject after = GameBridge.SerializeBibite(spawned);
                if (after == null)
                {
                    MultiversePlugin.Log.LogError("SaveSystem.SerializeBibite returned null for the respawned organism — cannot verify.");
                    return;
                }

                string afterJson = Stamp(after, "multiverse M1 round trip (after)");
                MultiversePlugin.Log.LogInfo($"serialized AFTER payload: {afterJson.Length} chars");
                WritePayload(AfterFile, afterJson);

                LogVerdict(context.before, after);
                LogTargetState("after ", newBody);
                MultiversePlugin.Log.LogInfo("=== M1 round trip: done ===");
            }
            catch (Exception e)
            {
                MultiversePlugin.Log.LogError($"Round trip aborted during respawn/verify: {e}");
            }
        }

        private static void LogVerdict(JObject before, JObject after)
        {
            List<string> diffs = JsonDiff.Compare(before, after, MaxReportedDiffs);
            if (diffs.Count == 0)
            {
                MultiversePlugin.Log.LogInfo("ROUND TRIP VERDICT: EQUAL — the two payloads match token for token.");
                return;
            }

            string cap = (diffs.Count >= MaxReportedDiffs) ? $" (capped at {MaxReportedDiffs})" : string.Empty;
            MultiversePlugin.Log.LogWarning($"ROUND TRIP VERDICT: DIFFERENT — {diffs.Count} differing path(s){cap}:");
            foreach (string diff in diffs)
            {
                MultiversePlugin.Log.LogWarning($"  {diff}");
            }
        }

        // ---- helpers ---------------------------------------------------------------------

        private static void LogTarget(Context context)
        {
            MultiversePlugin.Log.LogInfo($"target from {context.source}");
            LogTargetState("before", context.body);
        }

        private static void LogTargetState(string label, BibiteBody body)
        {
            try
            {
                BibiteGenes genes = body.gene;
                Vector3 position = body.transform.position;
                string tag = (genes != null && !string.IsNullOrEmpty(genes.speciesTag)) ? genes.speciesTag : "<none>";
                int generation = (genes != null) ? genes.generation : -1;
                float growth = (body.growth != null) ? body.growth.growth : float.NaN;
                MultiversePlugin.Log.LogInfo(
                    $"{label}: id={((body.id != null) ? body.id.id : 0)} tag={tag} gen={generation} " +
                    $"health={body.Health.Amount:F4}/{body.Health.MaxAmount:F4} energy={body.Energy.Amount:F4}/{body.Energy.MaxAmount:F4} " +
                    $"d2Size={body.d2Size:F4} growth={growth:F4} age={body.age:F4}h " +
                    $"pos=({position.x:F3},{position.y:F3}) dying={body.dying} dead={body.dead}");
            }
            catch (Exception e)
            {
                MultiversePlugin.Log.LogWarning($"could not log organism state: {e.Message}");
            }
        }

        /// <summary>
        /// Copy the payload and stamp it the way SaveSystem.SaveBibite does (SaveSystem.cs:147-148),
        /// so the file on disk is a valid .bb8 and the diff still runs on the raw serializer output.
        /// </summary>
        private static string Stamp(JObject payload, string description)
        {
            JObject stamped = (JObject)payload.DeepClone();
            stamped["version"] = Application.version;
            stamped["desc"] = description;
            return stamped.ToString(Formatting.Indented);
        }

        private static void WritePayload(string fileName, string json)
        {
            try
            {
                string path = Path.Combine(LogDirectory(), fileName);
                File.WriteAllText(path, json);
                MultiversePlugin.Log.LogInfo($"wrote {path}");
            }
            catch (Exception e)
            {
                MultiversePlugin.Log.LogError($"could not write {fileName}: {e}");
            }
        }

        private static string LogDirectory()
        {
            if (!string.IsNullOrEmpty(cachedLogDirectory))
            {
                return cachedLogDirectory;
            }

            string baseDirectory = null;
            try
            {
                baseDirectory = Path.GetDirectoryName(typeof(RoundTripCommand).Assembly.Location);
            }
            catch (Exception)
            {
                baseDirectory = null;
            }

            if (string.IsNullOrEmpty(baseDirectory))
            {
                baseDirectory = Path.Combine(Application.dataPath, "../BepInEx/plugins");
            }

            cachedLogDirectory = Path.Combine(baseDirectory, LogFolder);
            Directory.CreateDirectory(cachedLogDirectory);
            return cachedLogDirectory;
        }
    }
}
