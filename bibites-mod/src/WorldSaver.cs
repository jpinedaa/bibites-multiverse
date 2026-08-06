using System;
using System.Collections;
using System.Collections.Generic;
using System.Diagnostics;
using System.Globalization;
using System.IO;
using System.IO.Compression;
using System.Text.RegularExpressions;
using ManagementScripts;
using Newtonsoft.Json.Linq;
using OneUseScripts;
using UnityEngine;

namespace BibitesMultiverse
{
    /// <summary>
    /// One periodic world save, and its receipt (contract-a.md §5.2 <c>lastSave</c>, §15 A21;
    /// m4_considerations.md, Question 8 and Risk 3; decision D14).
    ///
    /// It exists because T1 lost twelve hours of four worlds to two closed windows. The game's own
    /// autosave is not the answer and stays off: it reloads the scene when it fires, which under a live
    /// rig is worse than a stale save.
    /// </summary>
    internal sealed class SaveReceipt
    {
        internal long atMs;
        internal double simulatedTime;
        internal int population;
        internal string name;
        internal long bytes;
        internal int durationMs;

        /// <summary>
        /// §5.2 — <c>lastSave</c> is a **receipt, not a request**: no component asks for a save,
        /// schedules one, or reacts to a missing one, and <c>atMs</c> is a label on this machine's clock
        /// that no correctness decision reads (D5). It is OPTIONAL, so a mod that has not saved yet
        /// omits it and the status page reads the world as unknown — an honest gap, never a zero.
        /// </summary>
        internal JObject ToJson()
        {
            JObject json = new JObject
            {
                ["atMs"] = atMs,
                ["simulatedTime"] = simulatedTime,
                ["population"] = population
            };

            if (!string.IsNullOrEmpty(name))
            {
                json["name"] = name;
            }

            if (bytes > 0)
            {
                json["bytes"] = bytes;
            }

            json["durationMs"] = durationMs;
            return json;
        }
    }

    /// <summary>
    /// The mod-owned save timer, the save on quit, and the rotation (D14).
    ///
    /// **The save is driven synchronously, and that is deliberate.** <c>SaveSystem.CreateSave</c> is a
    /// private iterator that the public <c>SaveGame</c> hands to <c>StartCoroutine</c>, where an
    /// exception is swallowed by Unity's coroutine runner (m1_findings.md). Its only suspension points
    /// are two bare <c>yield return null</c>s, and nothing inside it needs a frame boundary — the
    /// screenshot path renders synchronously inside <c>cam.Render()</c>. Driving the enumerator to
    /// completion in one call therefore (a) lets a failure reach a log line, (b) makes the stall one
    /// measurable number instead of three frames of guesswork, and (c) is the only shape that works
    /// from <c>OnApplicationQuit</c>, where coroutines no longer run. The owner's budget is **2000 ms**
    /// (Risk 3); this class measures it and says so rather than hiding a slow save.
    ///
    /// **Rotation keeps the live name stable.** The save is written to <c>&lt;world&gt;.partial.zip</c>,
    /// verified as a readable archive with a <c>scene.bb8scene</c> entry, and only then does the old
    /// <c>&lt;world&gt;.zip</c> become <c>&lt;world&gt;-&lt;utc&gt;.zip</c> and the new one take its
    /// place. A half-written save can never replace the last good one, and <c>MULTIVERSE_WORLD</c>
    /// always names the newest — which is what lets the recovery script restart a slot with no
    /// operator (Exit Test Part 7).
    ///
    /// Two rules of Risk 3 are enforced here: never start a save while a <c>MIGRATE_IN</c> waits in the
    /// mod's queue, and never save two worlds on one machine at the same moment. The second needs a
    /// cross-process gate, because the six-instance rig runs four or five games on this machine out of
    /// one save directory, so it is a lock file in that directory.
    ///
    /// Every line carries the grepable prefix <c>[M4-SAVE]</c>.
    /// </summary>
    internal class WorldSaver : MonoBehaviour
    {
        internal const string Prefix = "[M4-SAVE]";

        /// <summary>Risk 3 — the owner's budget for the simulation stall of one periodic save.</summary>
        internal const int StallBudgetMs = 2000;

        private const float DeferSeconds = 15f;
        private const int MaxDeferrals = 6;
        private const string PartialSuffix = ".partial";
        private const double StaleLockSeconds = 180.0;

        internal static WorldSaver Instance;

        internal float IntervalMinutes = MultiverseConfig.DefaultSaveMinutes;
        internal int Keep = MultiverseConfig.DefaultSaveKeep;
        internal bool SaveOnQuit = true;
        internal string PreferredWorldName = string.Empty;

        /// <summary>
        /// Risk 3, rule 2: "never start a save while a <c>MIGRATE_IN</c> waits in the mod's queue".
        /// Supplied by <see cref="MultiverseClient"/> so this class needs no reference to the wire.
        /// </summary>
        internal Func<bool> DeferWhile;

        internal SaveReceipt LastSave { get; private set; }
        internal int SaveCount { get; private set; }

        private static readonly Regex RotatedName = new Regex(@"-\d{8}T\d{6}Z\.zip$", RegexOptions.CultureInvariant);

        private bool armed;
        private bool quitSaved;
        private float nextSaveRealtime;
        private int deferrals;
        private FileStream lockStream;

        private void Awake()
        {
            Instance = this;
        }

        private void OnDestroy()
        {
            if (Instance == this)
            {
                Instance = null;
            }

            ReleaseLock();
        }

        private void Update()
        {
            try
            {
                Pump();
            }
            catch (Exception e)
            {
                MultiversePlugin.Log.LogError($"{Prefix} the save timer threw — {e}");
            }
        }

        private void Pump()
        {
            if (!GameBridge.SimulationReady())
            {
                armed = false;
                return;
            }

            if (!armed)
            {
                armed = true;
                deferrals = 0;
                nextSaveRealtime = Time.realtimeSinceStartup + IntervalMinutes * 60f;
                CleanUpPartial(ResolveWorldName());

                if (IntervalMinutes > 0f)
                {
                    MultiversePlugin.Log.LogInfo(
                        $"{Prefix} armed — world='{ResolveWorldName()}' " +
                        $"interval={IntervalMinutes.ToString("F1", CultureInfo.InvariantCulture)} min keep={Keep} " +
                        $"saveOnQuit={SaveOnQuit} budgetMs={StallBudgetMs} directory='{SaveController.SavePath}'.");
                }
                else
                {
                    MultiversePlugin.Log.LogWarning(
                        $"{Prefix} the periodic save is OFF (interval 0). D14 exists because a lost night costs more than a " +
                        "slower rig — turn it on with MULTIVERSE_SAVE_MINUTES or [M4] SaveMinutes.");
                }
            }

            if (IntervalMinutes <= 0f)
            {
                return;
            }

            float now = Time.realtimeSinceStartup;
            if (now < nextSaveRealtime)
            {
                return;
            }

            string blocked = null;
            try
            {
                if (DeferWhile != null && DeferWhile())
                {
                    blocked = "a MIGRATE_IN is waiting in the mod's queue (Risk 3, rule 2)";
                }
            }
            catch (Exception e)
            {
                MultiversePlugin.Log.LogWarning($"{Prefix} the defer check threw ({e.Message}) — treating the queue as empty.");
            }

            bool holdsLock = false;
            if (blocked == null)
            {
                if (TryAcquireLock(out string holder))
                {
                    holdsLock = true;
                }
                else
                {
                    blocked = "another instance on this machine is saving (" + holder + ")";
                }
            }

            if (blocked != null)
            {
                if (deferrals < MaxDeferrals)
                {
                    deferrals++;
                    nextSaveRealtime = now + DeferSeconds;
                    MultiversePlugin.Log.LogInfo(
                        $"{Prefix} deferred {deferrals}/{MaxDeferrals} by {DeferSeconds:F0} s — {blocked}.");
                    return;
                }

                MultiversePlugin.Log.LogWarning(
                    $"{Prefix} saving anyway after {deferrals} deferrals — {blocked}. A stale world is the worse failure (D14).");
            }

            deferrals = 0;
            nextSaveRealtime = Time.realtimeSinceStartup + IntervalMinutes * 60f;

            try
            {
                Save("periodic");
            }
            finally
            {
                if (holdsLock)
                {
                    ReleaseLock();
                }
            }
        }

        /// <summary>
        /// D14's save on quit. A coroutine cannot run here, which is the second reason the save path is
        /// synchronous. The lock is best effort: losing the world is worse than two saves overlapping.
        /// </summary>
        private void OnApplicationQuit()
        {
            if (quitSaved || !SaveOnQuit)
            {
                return;
            }

            quitSaved = true;

            if (!GameBridge.SimulationReady())
            {
                MultiversePlugin.Log.LogInfo($"{Prefix} quit with no world loaded — nothing to save.");
                return;
            }

            bool holdsLock = TryAcquireLock(out string holder);
            if (!holdsLock)
            {
                MultiversePlugin.Log.LogWarning(
                    $"{Prefix} quit: the save lock is held ({holder}) — saving anyway, because a discarded world is the worse outcome.");
            }

            try
            {
                Save("quit");
            }
            catch (Exception e)
            {
                MultiversePlugin.Log.LogError($"{Prefix} the save on quit threw — {e}");
            }
            finally
            {
                if (holdsLock)
                {
                    ReleaseLock();
                }
            }
        }

        /// <summary>
        /// One save, start to finish, on the main thread. Returns false and leaves the previous save
        /// untouched on any failure.
        /// </summary>
        internal bool Save(string why)
        {
            string world = ResolveWorldName();
            if (string.IsNullOrEmpty(world))
            {
                MultiversePlugin.Log.LogError($"{Prefix} event=FAILED why={why} — no world name to save under.");
                return false;
            }

            string directory = SaveController.SavePath;
            string finalPath = Path.Combine(directory, world + ".zip");
            string partialPath = Path.Combine(directory, world + PartialSuffix + ".zip");

            int population = GameBridge.LivingPopulation();
            int eggs = GameBridge.EggPopulation();
            double simulatedTime = TimeKeeper.simulatedTime;

            Stopwatch total = Stopwatch.StartNew();
            long writeMs;
            string failure = Write(partialPath, out writeMs);
            if (failure == null)
            {
                failure = Verify(partialPath);
            }

            long rotateStart = total.ElapsedMilliseconds;
            string rotated = null;
            if (failure == null)
            {
                failure = Rotate(finalPath, partialPath, out rotated);
            }

            total.Stop();
            long stallMs = total.ElapsedMilliseconds;
            long rotateMs = stallMs - rotateStart;

            if (failure != null)
            {
                TryDelete(partialPath);
                MultiversePlugin.Log.LogError(
                    $"{Prefix} event=FAILED why={why} world='{world}' stallMs={stallMs} population={population} " +
                    $"reason=\"{failure}\" — the previous save is untouched.");
                return false;
            }

            long bytes = 0;
            try
            {
                bytes = new FileInfo(finalPath).Length;
            }
            catch (Exception)
            {
                // Reported size only; the archive check already passed.
            }

            int pruned = Prune(directory, world);

            SaveCount++;
            LastSave = new SaveReceipt
            {
                atMs = ContractA.UnixMilliseconds(),
                simulatedTime = simulatedTime,
                population = population,
                name = Path.GetFileName(finalPath),
                bytes = bytes,
                durationMs = (int)stallMs
            };

            bool withinBudget = stallMs <= StallBudgetMs;
            MultiversePlugin.Log.LogInfo(
                $"{Prefix} event=SAVED why={why} n={SaveCount} world='{world}' path='{finalPath}' " +
                $"stallMs={stallMs} writeMs={writeMs} rotateMs={rotateMs} budgetMs={StallBudgetMs} withinBudget={withinBudget} " +
                $"bytes={bytes} population={population} eggs={eggs} " +
                $"simulatedTime={simulatedTime.ToString("F2", CultureInfo.InvariantCulture)} " +
                $"rotatedTo={(rotated ?? "<none>")} pruned={pruned} keep={Keep}");

            if (!withinBudget)
            {
                // Risk 3: "A save that breaks the 2-second budget needs a different cadence, or a save
                // path that does not block the tick. It does not need a different decision." Recorded,
                // not optimized away, and not silently retuned.
                MultiversePlugin.Log.LogWarning(
                    $"{Prefix} event=BUDGET_EXCEEDED stallMs={stallMs} budgetMs={StallBudgetMs} " +
                    $"population={population} eggs={eggs} bytes={bytes} — D14's budget is the test bar of Risk 3. " +
                    "Lengthen the cadence or move the save off the tick; do not lower the bar.");
            }

            return true;
        }

        /// <summary>
        /// Drive <c>SaveSystem.CreateSave</c> to completion in one call. Its two <c>yield return
        /// null</c>s are frame breaks and nothing more, so skipping them changes no behaviour and turns
        /// the whole save into one measurable stall.
        /// </summary>
        private static string Write(string path, out long writeMs)
        {
            writeMs = 0;

            IEnumerator save;
            try
            {
                save = GameBridge.CreateSaveEnumerator(path);
            }
            catch (Exception e)
            {
                return "could not reach SaveSystem.CreateSave by reflection: " + e.Message;
            }

            Stopwatch clock = Stopwatch.StartNew();
            try
            {
                while (save.MoveNext())
                {
                    // Every yield inside CreateSave is a bare `yield return null`. There is nothing to
                    // wait for, so the loop just keeps going.
                }
            }
            catch (Exception e)
            {
                clock.Stop();
                writeMs = clock.ElapsedMilliseconds;
                MultiversePlugin.Log.LogError($"{Prefix} the world save threw — {e}");
                return "the world save threw: " + e.GetType().Name + ": " + e.Message;
            }

            clock.Stop();
            writeMs = clock.ElapsedMilliseconds;
            return null;
        }

        private static string Verify(string path)
        {
            if (!File.Exists(path))
            {
                return "the world save produced no file at " + path;
            }

            try
            {
                using (ZipArchive zip = ZipFile.Open(path, ZipArchiveMode.Read))
                {
                    if (zip.GetEntry("scene.bb8scene") == null)
                    {
                        return "the world save has no scene.bb8scene entry";
                    }
                }
            }
            catch (Exception e)
            {
                return "the world save is not a readable archive: " + e.Message;
            }

            return null;
        }

        /// <summary>
        /// Two renames: the old live save becomes a rotated copy, then the verified partial becomes the
        /// live save. The window between them is two file-system metadata operations, and the rotated
        /// copy is what covers it.
        /// </summary>
        private static string Rotate(string finalPath, string partialPath, out string rotated)
        {
            rotated = null;

            try
            {
                if (File.Exists(finalPath))
                {
                    string stamp = DateTime.UtcNow.ToString("yyyyMMdd'T'HHmmss'Z'", CultureInfo.InvariantCulture);
                    string backup = Path.Combine(
                        Path.GetDirectoryName(finalPath) ?? string.Empty,
                        Path.GetFileNameWithoutExtension(finalPath) + "-" + stamp + ".zip");

                    if (File.Exists(backup))
                    {
                        File.Delete(backup);
                    }

                    File.Move(finalPath, backup);
                    rotated = Path.GetFileName(backup);
                }

                File.Move(partialPath, finalPath);
                return null;
            }
            catch (Exception e)
            {
                return "the rotation failed: " + e.Message;
            }
        }

        /// <summary>Keep the newest <see cref="Keep"/> rotated saves. Only this world's rotations are touched.</summary>
        private int Prune(string directory, string world)
        {
            try
            {
                string[] candidates = Directory.GetFiles(directory, world + "-*.zip");
                List<string> rotations = new List<string>();
                foreach (string candidate in candidates)
                {
                    string name = Path.GetFileName(candidate);
                    if (name != null && RotatedName.IsMatch(name))
                    {
                        rotations.Add(candidate);
                    }
                }

                if (rotations.Count <= Keep)
                {
                    return 0;
                }

                // The stamp is UTC and fixed width, so the name sorts by age.
                rotations.Sort(StringComparer.Ordinal);

                int removed = 0;
                for (int i = 0; i < rotations.Count - Keep; i++)
                {
                    if (TryDelete(rotations[i]))
                    {
                        removed++;
                    }
                }

                return removed;
            }
            catch (Exception e)
            {
                MultiversePlugin.Log.LogWarning($"{Prefix} could not prune the rotated saves: {e.Message}");
                return 0;
            }
        }

        internal string ResolveWorldName()
        {
            // MULTIVERSE_WORLD is the rig's identity for this instance and the only reliable name:
            // CreateSave never writes a gameName key, so SimulationManager.gameName is usually empty
            // for a world the game itself saved (m1_findings.md, hazards).
            if (!string.IsNullOrEmpty(PreferredWorldName))
            {
                return PreferredWorldName;
            }

            string gameName = SimulationManager.gameName;
            if (!string.IsNullOrEmpty(gameName))
            {
                return gameName;
            }

            return "MultiverseWorld";
        }

        private void CleanUpPartial(string world)
        {
            if (string.IsNullOrEmpty(world))
            {
                return;
            }

            string partial = Path.Combine(SaveController.SavePath, world + PartialSuffix + ".zip");
            if (File.Exists(partial) && TryDelete(partial))
            {
                MultiversePlugin.Log.LogWarning(
                    $"{Prefix} removed a leftover partial save '{partial}' — an earlier run died mid-save. The live save is intact.");
            }
        }

        private static bool TryDelete(string path)
        {
            try
            {
                if (File.Exists(path))
                {
                    File.Delete(path);
                    return true;
                }
            }
            catch (Exception e)
            {
                MultiversePlugin.Log.LogWarning($"{Prefix} could not delete '{path}': {e.Message}");
            }

            return false;
        }

        // ---- the cross-instance gate (Risk 3, rule 1) --------------------------------------------

        private static string LockPath => Path.Combine(SaveController.SavePath, ".multiverse-save.lock");

        private bool TryAcquireLock(out string holder)
        {
            holder = "unknown";
            if (lockStream != null)
            {
                return true;
            }

            for (int attempt = 0; attempt < 2; attempt++)
            {
                try
                {
                    lockStream = new FileStream(LockPath, FileMode.CreateNew, FileAccess.Write, FileShare.None);
                    byte[] note = System.Text.Encoding.UTF8.GetBytes(
                        ResolveWorldName() + " pid=" + Process.GetCurrentProcess().Id.ToString(CultureInfo.InvariantCulture)
                        + " at=" + DateTime.UtcNow.ToString("O", CultureInfo.InvariantCulture) + "\n");
                    lockStream.Write(note, 0, note.Length);
                    lockStream.Flush();
                    return true;
                }
                catch (Exception)
                {
                    lockStream = null;
                }

                try
                {
                    holder = File.ReadAllText(LockPath).Trim();
                    double age = (DateTime.UtcNow - File.GetLastWriteTimeUtc(LockPath)).TotalSeconds;
                    if (age <= StaleLockSeconds)
                    {
                        return false;
                    }

                    MultiversePlugin.Log.LogWarning(
                        $"{Prefix} the save lock is {age:F0} s old ('{holder}') — an instance died holding it. Breaking it.");
                    File.Delete(LockPath);
                }
                catch (Exception e)
                {
                    holder = "unreadable: " + e.Message;
                    return false;
                }
            }

            return false;
        }

        private void ReleaseLock()
        {
            if (lockStream == null)
            {
                return;
            }

            try
            {
                lockStream.Dispose();
            }
            catch (Exception)
            {
                // Nothing useful to do; the delete below is what matters.
            }

            lockStream = null;
            TryDelete(LockPath);
        }
    }
}
