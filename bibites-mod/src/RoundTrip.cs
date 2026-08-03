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
    /// <summary>Outcome of one round trip, so a caller can decide PASS/FAIL without re-reading the log.</summary>
    internal class RoundTripResult
    {
        internal string source = "none";
        internal int beforeId;
        internal int afterId;
        internal BibiteBody newBody;
        internal List<string> diffs;
        internal bool destroyed;
        internal bool respawned;
        internal bool compared;
        internal string failure;

        internal bool IdPreserved => beforeId != 0 && beforeId == afterId;
        internal bool PayloadsEqual => compared && diffs != null && diffs.Count == 0;
        internal bool Success => respawned && compared && string.IsNullOrEmpty(failure);
    }

    /// <summary>
    /// The M1 round trip: serialize → destroy → respawn → re-link → verify, on one live organism,
    /// through the game's own full-fidelity save path. Everything is wrapped so a failure logs
    /// instead of taking the simulation down with it.
    ///
    /// This is the callable core. <see cref="RoundTripCommand"/> drives it from F9;
    /// <see cref="AutoTest"/> drives it unattended.
    /// </summary>
    internal static class RoundTrip
    {
        private const string LogFolder = "multiverse-logs";
        private const string BeforeFile = "roundtrip-before.json";
        private const string AfterFile = "roundtrip-after.json";
        internal const int MaxReportedDiffs = 50;

        private static string cachedLogDirectory;

        private class Context
        {
            public RoundTripResult result;
            public BibiteBody body;
            public GameObject gameObject;
            public JObject before;
            public string beforeJson;
            public OrganismLinks links;
        }

        /// <summary>
        /// Run one round trip. <paramref name="target"/> may be null, in which case the organism is
        /// picked with <see cref="GameBridge.PickTarget"/>. Must be driven by a MonoBehaviour that
        /// survives the frame boundary — Destroy is deferred to the end of the frame.
        /// </summary>
        internal static IEnumerator Run(RoundTripResult result, BibiteBody target = null)
        {
            Context context = new Context { result = result, body = target };

            if (SerializeAndDestroy(context))
            {
                // UnityEngine.Object.Destroy is deferred to the end of the frame, so the
                // old GameObject is still alive right now. Give Unity that frame.
                yield return null;

                RespawnAndVerify(context);
            }

            if (!result.Success && string.IsNullOrEmpty(result.failure))
            {
                result.failure = "round trip did not complete";
            }
        }

        // ---- step 1: pick, serialize, record links, destroy -------------------------------

        private static bool SerializeAndDestroy(Context context)
        {
            RoundTripResult result = context.result;
            try
            {
                MultiversePlugin.Log.LogInfo("=== M1 round trip: start ===");

                if (context.body == null)
                {
                    context.body = GameBridge.PickTarget(out string picked);
                    result.source = picked;
                }
                else
                {
                    result.source = "caller-supplied target";
                }

                if (context.body == null)
                {
                    MultiversePlugin.Log.LogWarning(
                        "No target organism: nothing is selected and BibiteTracker.instance.bibites has no living entry. " +
                        "Load a simulation with at least one bibite first.");
                    result.failure = "no target organism";
                    return false;
                }

                context.gameObject = context.body.gameObject;
                result.beforeId = (context.body.id != null) ? context.body.id.id : 0;
                MultiversePlugin.Log.LogInfo($"target from {result.source}");
                LogTargetState("before", context.body);

                context.before = GameBridge.SerializeBibite(context.gameObject);
                if (context.before == null)
                {
                    MultiversePlugin.Log.LogError("SaveSystem.SerializeBibite returned null (no BibiteGenes on the target) — aborting.");
                    result.failure = "SerializeBibite returned null";
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
                result.destroyed = true;
                MultiversePlugin.Log.LogInfo("destroyed the organism GameObject — waiting one frame for Unity to finish.");
                return true;
            }
            catch (Exception e)
            {
                MultiversePlugin.Log.LogError($"Round trip aborted during serialize/destroy: {e}");
                result.failure = "exception during serialize/destroy: " + e.Message;
                return false;
            }
        }

        // ---- step 2: respawn, relink, verify ---------------------------------------------

        private static void RespawnAndVerify(Context context)
        {
            RoundTripResult result = context.result;
            try
            {
                SaveSystem save = SaveSystem.instance;
                if (save == null)
                {
                    MultiversePlugin.Log.LogError("SaveSystem.instance is null — cannot respawn. The organism is gone.");
                    result.failure = "SaveSystem.instance is null";
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
                    result.failure = "respawn produced no GameObject";
                    return;
                }

                BibiteBody newBody = spawned.GetComponent<BibiteBody>();
                if (newBody == null)
                {
                    MultiversePlugin.Log.LogError("Respawned GameObject has no BibiteBody — aborting the verify step.");
                    result.failure = "respawned GameObject has no BibiteBody";
                    return;
                }

                result.respawned = true;
                result.newBody = newBody;
                result.afterId = (newBody.id != null) ? newBody.id.id : 0;
                MultiversePlugin.Log.LogInfo(
                    $"respawned: id={result.afterId} (before={result.beforeId}) idPreserved={result.IdPreserved} " +
                    $"tracked={(BibiteTracker.instance != null && BibiteTracker.instance.bibites != null && BibiteTracker.instance.bibites.Contains(newBody))}");

                if (context.links != null)
                {
                    context.links.Restore(newBody);
                }

                JObject after = GameBridge.SerializeBibite(spawned);
                if (after == null)
                {
                    MultiversePlugin.Log.LogError("SaveSystem.SerializeBibite returned null for the respawned organism — cannot verify.");
                    result.failure = "SerializeBibite returned null for the respawned organism";
                    return;
                }

                string afterJson = Stamp(after, "multiverse M1 round trip (after)");
                MultiversePlugin.Log.LogInfo($"serialized AFTER payload: {afterJson.Length} chars");
                WritePayload(AfterFile, afterJson);

                result.diffs = LogVerdict(context.before, after);
                result.compared = true;
                LogTargetState("after ", newBody);
                MultiversePlugin.Log.LogInfo("=== M1 round trip: done ===");
            }
            catch (Exception e)
            {
                MultiversePlugin.Log.LogError($"Round trip aborted during respawn/verify: {e}");
                result.failure = "exception during respawn/verify: " + e.Message;
            }
        }

        private static List<string> LogVerdict(JObject before, JObject after)
        {
            List<string> diffs = JsonDiff.Compare(before, after, MaxReportedDiffs);
            if (diffs.Count == 0)
            {
                MultiversePlugin.Log.LogInfo("ROUND TRIP VERDICT: EQUAL — the two payloads match token for token.");
                return diffs;
            }

            string cap = (diffs.Count >= MaxReportedDiffs) ? $" (capped at {MaxReportedDiffs})" : string.Empty;
            MultiversePlugin.Log.LogWarning($"ROUND TRIP VERDICT: DIFFERENT — {diffs.Count} differing path(s){cap}:");
            foreach (string diff in diffs)
            {
                MultiversePlugin.Log.LogWarning($"  {diff}");
            }

            return diffs;
        }

        // ---- helpers ---------------------------------------------------------------------

        internal static void LogTargetState(string label, BibiteBody body)
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
                baseDirectory = Path.GetDirectoryName(typeof(RoundTrip).Assembly.Location);
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
