using System;
using System.Collections.Generic;
using System.Reflection;
using HarmonyLib;
using ManagementScripts;
using OneUseScripts;
using SimulationScripts.BibiteScripts;
using UnityEngine;
using Utility;

namespace BibitesMultiverse
{
    /// <summary>
    /// The guard for a **game-side** defect in the species-history arrays that stops a world from
    /// saving at all (2026-08-06/07: slot 5 lost 27 consecutive saves on mod 0.4.0, slot 4 lost 18 plus
    /// the save on quit on 0.5.0). It is not our bug and this class does not pretend to fix it; it
    /// repairs one derived counter immediately before the game serializes the arrays, which is the
    /// smallest place the damage can be undone.
    ///
    /// **The defect.** <c>Utility.LogLikePresenceArray&lt;T&gt;</c> keeps a species' history in ten
    /// ring buffers plus an unbounded tail, and describes the live window twice over:
    ///
    /// * as four indices — <c>scaleOfApparition</c>/<c>indexOfApparition</c> and
    ///   <c>scaleOfDisappearance</c>/<c>indexOfDisappearance</c>, the ends of the window; and
    /// * as <c>nPresentAtScale[]</c>, a per-scale running count maintained one push at a time in the
    ///   private <c>NewPointPushedToScale</c> (<c>LogLikePresenceArray.cs:51-88</c>).
    ///
    /// <c>SaveStateBin</c> (<c>LogLikePresenceArray.cs:117-163</c>) uses **both**, and they have to
    /// agree. It sizes its five temporary arrays from the counter —
    /// <c>CreateTempArrays(nPointsToSave)</c> at line 138, where
    /// <c>nPointsToSave == nPresentAtScale.Sum() + pushCounters.Sum()</c> (line 28) — and then fills
    /// them from a loop driven by the **indices** (lines 140-160). When the counter reads lower than
    /// the window the indices describe, the loop walks off the end of the arrays and
    /// <c>LogLikeSpeciesDataArray.SavePointToArrays</c> (<c>LogLikeSpeciesDataArray.cs:138-147</c>)
    /// throws <c>IndexOutOfRangeException</c> on its first store. That is the whole of the reported
    /// stack, and it repeats on every save from then on because nothing in the game recomputes the
    /// counter outside a prune merge.
    ///
    /// **How the counter gets ahead of itself.** <c>nPresentAtScale</c> is a <c>ushort[]</c>, and line
    /// 80 decrements it whenever the disappearance front advances into a scale:
    ///
    /// <code>
    ///   if (i == scaleOfDisappearance &amp;&amp; indexOfDisappearance &lt; DataSizeAtScale(i) - 1)
    ///   {  indexOfDisappearance++;  nPresentAtScale[i]--;  }        // line 77-81
    ///   ...
    ///   if (i == scaleOfApparition &amp;&amp; indexOfApparition &lt; DataSizeAtScale(i) - 1)
    ///   {  indexOfApparition++;     nPresentAtScale[i]++;  }        // line 83-87
    /// </code>
    ///
    /// The two normally pair up. They do not when the front reaches a scale the species has never put
    /// a **living** point into — <c>scaleOfApparition</c> is still below <c>i</c>, so only the
    /// decrement runs, and <c>0 - 1</c> on a <c>ushort</c> is <b>65535</b>. Saves keep working while
    /// that value stands (the arrays are merely 65535 points too big) and the species also stops being
    /// prunable, because <c>SpeciesTreeCleanup</c> reads the same sum. Then the species is repopulated,
    /// a living point finally reaches that scale, line 86 increments 65535 back to **0** — and from
    /// that moment the counter is short by exactly the window it forgot, and every save throws.
    ///
    /// **Whose fault it is.** Not the mod's, and specifically not §16's species creation: the same
    /// stack, from the same two call sites, appears in the slot-5 log of 2026-08-06 under mod
    /// **0.4.0**, which has no species block, no <c>SpeciesRegistry</c> and no <c>CreateLocal</c> at
    /// all. What the multiverse supplies is the traffic: organisms arrive, empty a species when they
    /// migrate on, and a later arrival of the same species repopulates it — the extinction-then-return
    /// cycle the defect needs, on hundreds of species per world instead of a handful.
    ///
    /// **What the guard does.** Before the game sizes or writes the lineage block, every recorded
    /// species' <c>nPresentAtScale</c> is recomputed to exactly the number of points
    /// <c>SaveStateBin</c>'s own loop will write for that scale. That is the same quantity the game's
    /// own <c>RefreshPresentsFromIndices</c> (<c>LogLikePresenceArray.cs:90-109</c>) computes when a
    /// prune merges two species, so the value written is the game's, not ours — but unlike that method
    /// this one leaves <c>nDataAtScale</c> alone, because that field steers <c>Push</c>'s cascade and
    /// rewriting it outside a merge would change how histories are compressed. On a healthy array the
    /// repair is a no-op; on a poisoned one it makes the two descriptions agree, which is the only
    /// property <c>SaveStateBin</c> needs.
    ///
    /// Repairing at the save boundary also un-sticks the pruning: a species whose count read 65535
    /// could never be collected, and the corrected sum lets <c>SpeciesTreeCleanup</c> take it on the
    /// next logging tick.
    ///
    /// Every line carries the grepable prefix <c>[M5-HISTORY]</c>.
    /// </summary>
    internal static class SpeciesHistoryGuard
    {
        internal const string Prefix = "[M5-HISTORY]";

        /// <summary>
        /// Set to 0/false/off to leave the arrays alone. It exists for **the regression check** — it
        /// is the only way to reproduce the pre-guard failure on a build that also carries the
        /// <c>speciespoison</c> verb — and it defaults to on. A rig that sets it to 0 is asking for a
        /// world that cannot save.
        /// </summary>
        internal const string EnvEnabled = "MULTIVERSE_SPECIES_GUARD";

        private static Harmony harmony;

        internal static bool Enabled { get; private set; } = true;

        /// <summary>How many species arrays this process has had to correct, and how many scales.</summary>
        internal static long RepairedArrays { get; private set; }
        internal static long RepairedScales { get; private set; }

        /// <summary>The last state we corrected, kept for the log line and the audit verb.</summary>
        internal static string LastRepairDetail { get; private set; } = "<none>";

        private static long loggedArrays = -1;

        // ---- arming ------------------------------------------------------------------------------

        /// <summary>
        /// Patch the two entry points the lineage block is serialized through. Both are prefixes and
        /// the repair is idempotent, so being called twice per save costs one pass over a short array
        /// and changes nothing the second time.
        ///
        /// <c>BytesSpace</c> matters as much as <c>SaveStateBin</c>: <c>SaveableBinStack</c> sizes the
        /// whole buffer from it (<c>SaveableBinStack.cs:12-15</c>) and then hands each element a slice
        /// (line 26-30), so a repair applied only at write time would move the byte count under a
        /// buffer that was already allocated.
        /// </summary>
        internal static bool Apply(string id)
        {
            Enabled = ReadEnabled();

            if (!Enabled)
            {
                MultiversePlugin.Log.LogWarning(
                    $"{Prefix} DISABLED by {EnvEnabled} — the species-history arrays are left exactly as the game " +
                    "leaves them. This is the regression-check setting: a world whose counters have been poisoned " +
                    "will fail every world save with IndexOutOfRangeException in " +
                    "LogLikeSpeciesDataArray.SavePointToArrays. Do not run a deployment this way.");
                return true;
            }

            if (harmony != null)
            {
                return true;
            }

            try
            {
                MethodInfo bytesSpace = AccessTools.Method(typeof(GlobalLineageManager), "BytesSpace");
                MethodInfo saveStateBin = AccessTools.Method(
                    typeof(GlobalLineageManager), "SaveStateBin", new[] { typeof(byte[]), typeof(int) });

                if (bytesSpace == null || saveStateBin == null)
                {
                    MultiversePlugin.Log.LogError(
                        $"{Prefix} GlobalLineageManager.BytesSpace/SaveStateBin not found — the species-history guard " +
                        "cannot be armed and world saves stay exposed to the game's counter defect. The game API changed.");
                    return false;
                }

                MethodInfo prefix = typeof(SpeciesHistoryGuard).GetMethod(
                    nameof(BeforeLineageSave), BindingFlags.Static | BindingFlags.NonPublic);

                harmony = new Harmony(id + ".specieshistory");
                harmony.Patch(bytesSpace, prefix: new HarmonyMethod(prefix));
                harmony.Patch(saveStateBin, prefix: new HarmonyMethod(prefix));

                MultiversePlugin.Log.LogInfo(
                    $"{Prefix} armed — nPresentAtScale is reconciled with the apparition/disappearance window before " +
                    "GlobalLineageManager.BytesSpace and .SaveStateBin. This guards a game-side defect in " +
                    "LogLikePresenceArray (a ushort presence counter that underflows at a scale which has never held a " +
                    "living point, then wraps back through zero when the species returns), not a mod bug.");
                return true;
            }
            catch (Exception e)
            {
                harmony = null;
                MultiversePlugin.Log.LogError($"{Prefix} could not patch the lineage save path — {e}");
                return false;
            }
        }

        private static bool ReadEnabled()
        {
            string raw = (MultiverseConfig.Env(EnvEnabled) ?? string.Empty).Trim().ToLowerInvariant();
            if (raw.Length == 0)
            {
                return true;
            }

            return !(raw == "0" || raw == "false" || raw == "no" || raw == "off");
        }

        private static void BeforeLineageSave(GlobalLineageManager __instance)
        {
            // A throw here would land inside the game's own save. Never let one out: an unrepaired
            // array saves exactly as badly as it did before this class existed, and that is strictly
            // better than turning a guard into the thing that breaks the save.
            try
            {
                // The repair is a span of binMs on the [M4-SAVE] line, not a separate phase; the
                // bracket is here rather than a patch because patching a Harmony prefix is a knot.
                SavePhases.GuardEnter();
                try
                {
                    RepairAll(__instance);
                }
                finally
                {
                    SavePhases.GuardExit();
                }
            }
            catch (Exception e)
            {
                MultiversePlugin.Log.LogError($"{Prefix} the pre-save reconciliation threw — the save proceeds unguarded. {e}");
            }
        }

        // ---- the repair --------------------------------------------------------------------------

        /// <summary>
        /// Reconcile every recorded species. Returns the number of arrays that needed it.
        ///
        /// It walks <c>recordedSpecies</c>, which is the same list and the same order
        /// <c>GlobalLineageManager.BytesSpace</c> and <c>.SaveStateBin</c> walk
        /// (<c>GlobalLineageManager.cs:436, 450</c>), so nothing that is about to be serialized is
        /// missed and nothing that is not is touched.
        /// </summary>
        internal static int RepairAll(GlobalLineageManager manager)
        {
            if (manager == null || manager.recordedSpecies == null)
            {
                return 0;
            }

            int arrays = 0;
            int scales = 0;
            string worst = null;

            List<Species> recorded = manager.recordedSpecies;
            for (int i = 0; i < recorded.Count; i++)
            {
                Species species = recorded[i];
                LogLikeSpeciesDataArray data = (species != null) ? species.speciesData : null;
                if (data == null)
                {
                    continue;
                }

                int before = PointsTheSaveLoopWrites(data) - NPointsToSave(data);
                int corrected = Repair(data);
                if (corrected <= 0)
                {
                    continue;
                }

                arrays++;
                scales += corrected;
                if (before > 0 && worst == null)
                {
                    // Only an array that was about to overrun is worth naming: a merely over-sized one
                    // was saving fine and its correction is bookkeeping.
                    worst = $"'{species.name}' (localId={species.speciesID}) was short by {before} point(s)";
                }
            }

            if (arrays > 0)
            {
                RepairedArrays += arrays;
                RepairedScales += scales;
                LastRepairDetail = worst ?? "no array was short; the corrections were over-counts";

                // One line per save at most, and only when the number moved, so a world that is
                // quietly repairing the same handful of species does not fill the log.
                if (RepairedArrays != loggedArrays)
                {
                    loggedArrays = RepairedArrays;
                    MultiversePlugin.Log.LogInfo(
                        $"{Prefix} reconciled {arrays} species history array(s) ({scales} scale counters) before the " +
                        $"world save — {LastRepairDetail}. This is the game's LogLikePresenceArray counter defect, " +
                        $"not a mod state error; the save proceeds normally. sessionArrays={RepairedArrays}.");
                }
            }

            return arrays;
        }

        /// <summary>
        /// Set <c>nPresentAtScale[i]</c> to the number of points <c>SaveStateBin</c>'s presence loop
        /// will write at scale <c>i</c> — <c>0</c> outside <c>[scaleOfDisappearance,
        /// scaleOfApparition]</c>, and the width of the window inside it. Returns how many entries
        /// changed.
        ///
        /// The expression mirrors <c>LogLikePresenceArray.cs:142-150</c> term for term, including its
        /// <c>num4 &gt;= 0</c> guard (an <c>indexOfApparition</c> of -1 means the top scale holds
        /// nothing yet). <c>pushCounters</c> is deliberately left alone: the save loop consumes it only
        /// up to <c>scaleOfApparition</c> while <c>nPointsToSave</c> sums all of it, so it can only
        /// make the budget too large, never too small.
        /// </summary>
        internal static int Repair(LogLikeSpeciesDataArray data)
        {
            ushort[] present = data.nPresentAtScale;
            if (present == null)
            {
                return 0;
            }

            int scales = Math.Min(present.Length, data.nScales);
            int changed = 0;

            for (int i = 0; i < scales; i++)
            {
                int want = WindowWidth(data, i);
                if (want > ushort.MaxValue)
                {
                    // Unreachable with the shipped SerialSpeciesConfig (the widest bounded scale is 36
                    // points and the unbounded tail takes one point per 8640 logging ticks), but a
                    // silent truncation here would re-create the very under-count this class removes.
                    MultiversePlugin.Log.LogWarning(
                        $"{Prefix} scale {i} describes {want} present points, which does not fit the game's ushort " +
                        "counter. Clamping — this save may still overrun and the world needs a reload from disk.");
                    want = ushort.MaxValue;
                }

                if (present[i] != (ushort)want)
                {
                    present[i] = (ushort)want;
                    changed++;
                }
            }

            return changed;
        }

        /// <summary>
        /// The width of the presence window at one scale, exactly as <c>SaveStateBin</c> walks it.
        /// </summary>
        private static int WindowWidth(LogLikeSpeciesDataArray data, int scale)
        {
            if (scale < data.scaleOfDisappearance || scale > data.scaleOfApparition)
            {
                return 0;
            }

            int first = (scale == data.scaleOfDisappearance) ? (data.indexOfDisappearance + 1) : 0;
            int last = (scale == data.scaleOfApparition) ? data.indexOfApparition : (data.DataSizeAtScale(scale) - 1);

            if (first < 0 || last < 0 || last < first)
            {
                return 0;
            }

            return last - first + 1;
        }

        // ---- audit -------------------------------------------------------------------------------

        /// <summary>The number of points <c>SaveStateBin</c>'s two loops will hand to <c>SavePointToArrays</c>.</summary>
        internal static int PointsTheSaveLoopWrites(LogLikeSpeciesDataArray data)
        {
            int total = 0;
            for (int i = 0; i <= data.scaleOfApparition && i < data.nScales; i++)
            {
                total += WindowWidth(data, i);
                if (i == data.nScales - 1)
                {
                    break;
                }

                total += data.pushCounters[i];
            }

            return total;
        }

        /// <summary>The size <c>CreateTempArrays</c> will be given: <c>LogLikePresenceArray.cs:28</c>.</summary>
        internal static int NPointsToSave(LogLikeSpeciesDataArray data)
        {
            int total = 0;
            for (int i = 0; i < data.nPresentAtScale.Length; i++)
            {
                total += data.nPresentAtScale[i];
            }

            for (int i = 0; i < data.pushCounters.Length; i++)
            {
                total += data.pushCounters[i];
            }

            return total;
        }

        internal struct Audit
        {
            internal int recorded;
            internal int wouldOverrun;
            internal int inconsistent;
            internal int repaired;
            internal string worst;
        }

        /// <summary>
        /// Report — and optionally correct — the state of every recorded species' history array. This
        /// is the operational read of the defect: <c>wouldOverrun</c> above zero means the next world
        /// save throws unless the guard runs first.
        /// </summary>
        internal static Audit Scan(bool repair)
        {
            Audit audit = default(Audit);
            audit.worst = "<none>";

            GlobalLineageManager manager = GlobalLineageManager.Instance;
            if (manager == null || manager.recordedSpecies == null)
            {
                return audit;
            }

            int worstShort = 0;
            List<Species> recorded = manager.recordedSpecies;
            audit.recorded = recorded.Count;

            for (int i = 0; i < recorded.Count; i++)
            {
                Species species = recorded[i];
                LogLikeSpeciesDataArray data = (species != null) ? species.speciesData : null;
                if (data == null)
                {
                    continue;
                }

                int loop = PointsTheSaveLoopWrites(data);
                int budget = NPointsToSave(data);
                if (loop > budget)
                {
                    audit.wouldOverrun++;
                    if (loop - budget > worstShort)
                    {
                        worstShort = loop - budget;
                        audit.worst =
                            $"'{species.name}' localId={species.speciesID} saveLoopWrites={loop} budget={budget} " +
                            $"short={loop - budget} scaleApparition={data.scaleOfApparition} " +
                            $"indexApparition={data.indexOfApparition} scaleDisappearance={data.scaleOfDisappearance} " +
                            $"indexDisappearance={data.indexOfDisappearance} nPresentAtScale=[{Join(data.nPresentAtScale)}]";
                    }
                }

                if (loop != budget)
                {
                    audit.inconsistent++;
                }

                if (repair && Repair(data) > 0)
                {
                    audit.repaired++;
                }
            }

            return audit;
        }

        private static string Join(ushort[] values)
        {
            if (values == null)
            {
                return string.Empty;
            }

            string[] parts = new string[values.Length];
            for (int i = 0; i < values.Length; i++)
            {
                parts[i] = values[i].ToString(System.Globalization.CultureInfo.InvariantCulture);
            }

            return string.Join(",", parts);
        }

        // ---- the regression check ----------------------------------------------------------------

        /// <summary>
        /// Manufacture the failure, deterministically, on a throwaway species.
        ///
        /// It registers one extra <c>Species</c> in <c>recordedSpecies</c> — so the world save walks it
        /// like any other — and pushes the documented sequence into its history: a run of empty points
        /// long enough for the disappearance front to reach scale 1 and underflow that scale's counter
        /// to 65535, then living points until one of them is compressed up into scale 1 and wraps the
        /// counter back through zero. The array is then short by exactly one point and the next world
        /// save throws <c>IndexOutOfRangeException</c> inside
        /// <c>LogLikeSpeciesDataArray.SavePointToArrays</c> — the same stack as the incident — unless
        /// this guard is armed.
        ///
        /// With the shipped <c>DataLogger.SerialSpeciesConfig</c> that is 14 empty pushes followed by
        /// 14 living ones; the loop below does not assume those numbers, it pushes until the array is
        /// actually short.
        ///
        /// <paramref name="cleanup"/> takes the species back out again, so a world can be poisoned,
        /// observed and restored without a reload.
        /// </summary>
        internal static string Poison(bool cleanup, out bool ok)
        {
            ok = false;

            GlobalLineageManager manager = GlobalLineageManager.Instance;
            if (manager == null || manager.recordedSpecies == null)
            {
                return "GlobalLineageManager.Instance is not available (no world is loaded)";
            }

            const string testName = "Poisoned testcase";

            if (cleanup)
            {
                int removed = manager.recordedSpecies.RemoveAll(s => s != null && s.name == testName);
                manager.activeSpecies?.RemoveAll(s => s != null && s.name == testName);
                ok = true;
                return $"removed {removed} test species named '{testName}'";
            }

            Species victim = new Species
            {
                speciesID = Species.speciesMaxID++,
                timeCreation = TimeKeeper.simulatedTime,
                genericName = "Poisoned",
                specificName = "testcase"
            };
            victim.rootSpecies = victim;
            victim.template.nodes = new NEATBrain.Node[0];
            victim.template.synapses = new NEATBrain.Synaps[0];
            victim.template.genes = new float[BibiteGenes.NGene];
            victim.template.name = testName;
            victim.template.speciesName = testName;

            manager.recordedSpecies.Add(victim);

            LogLikeSpeciesDataArray data = victim.speciesData;
            SpeciesDataPoint empty = default(SpeciesDataPoint);
            SpeciesDataPoint living = default(SpeciesDataPoint);
            living.count = 4;
            living.energy = 40f;
            living.posStdDev = Vector2.one;

            int emptyPushes = 0;
            int livingPushes = 0;
            const int cap = 4000;

            // Phase 1: empty points until a scale counter has underflowed (nothing else can put a
            // value that large in there).
            while (emptyPushes < cap && !HasUnderflow(data))
            {
                data.Push(empty);
                emptyPushes++;
            }

            if (!HasUnderflow(data))
            {
                manager.recordedSpecies.Remove(victim);
                return $"no scale counter underflowed within {cap} empty pushes — the game's array layout changed";
            }

            // Phase 2: living points until the wrap makes the array short.
            while (livingPushes < cap && PointsTheSaveLoopWrites(data) <= NPointsToSave(data))
            {
                data.Push(living);
                livingPushes++;
            }

            int loop = PointsTheSaveLoopWrites(data);
            int budget = NPointsToSave(data);

            if (loop <= budget)
            {
                manager.recordedSpecies.Remove(victim);
                return $"the counter never wrapped back through zero within {cap} living pushes " +
                       $"(empty={emptyPushes} living={livingPushes}) — the game's array layout changed";
            }

            ok = true;
            return $"poisoned '{testName}' localId={victim.speciesID} with {emptyPushes} empty + {livingPushes} living " +
                   $"history points: saveLoopWrites={loop} budget={budget} short={loop - budget} " +
                   $"scaleApparition={data.scaleOfApparition} indexApparition={data.indexOfApparition} " +
                   $"scaleDisappearance={data.scaleOfDisappearance} indexDisappearance={data.indexOfDisappearance} " +
                   $"nPresentAtScale=[{Join(data.nPresentAtScale)}] — the next world save throws unless the guard is armed " +
                   $"(guard={(Enabled ? "ON" : "OFF")})";
        }

        private static bool HasUnderflow(LogLikeSpeciesDataArray data)
        {
            ushort[] present = data.nPresentAtScale;
            for (int i = 0; i < present.Length; i++)
            {
                // A real window is at most DataSizeAtScale wide; anything near the top of the ushort
                // range came from the 0-- at LogLikePresenceArray.cs:80.
                if (present[i] > 30000)
                {
                    return true;
                }
            }

            return false;
        }
    }
}
