using System;
using System.Collections.Generic;
using System.Globalization;
using ManagementScripts;
using Newtonsoft.Json.Linq;
using SimulationScripts.BibiteScripts;
using UnityEngine;

namespace BibitesMultiverse
{
    /// <summary>
    /// The world's active species census — the OPTIONAL <c>HEARTBEAT.species</c> array of
    /// <c>contract-a.md</c> §5.2 and §17 (A35, A36).
    ///
    /// **This is not <see cref="SpeciesRegistry"/>, and the difference is the point.**
    /// <see cref="SpeciesRegistry.BlockForExport"/> builds a **matching key**: one migrant, resolved
    /// against a foreign registry, whitespace-normalized at the source because a stray space there is a
    /// silent mis-merge (§16, A34). This builds a **display census**: a whole world, read by a human on
    /// a status page and by nothing else, carrying the bytes this world's own <c>Species</c> record
    /// holds so the label reads exactly as this world's player sees it (§17, A36).
    ///
    /// So the two reads are kept in two files with two names, deliberately. §11.2 names a shared helper
    /// that normalizes for both as "the single most likely way to get this wrong": about **13%** of the
    /// names standing in the rig's registries carry stray whitespace, and in one sampled world those
    /// species were **27% of the living population** — normalizing here would print a name no player can
    /// find and merge two records the world genuinely holds apart, and dropping them would delete a
    /// quarter of a world from the page.
    ///
    /// Nothing here can fail a heartbeat. <c>HEARTBEAT</c> has no NACK channel at all, so a census that
    /// cannot be built is simply **absent** — which the whole chain already reads as *unknown* — and a
    /// species record that cannot be shaped costs that entry and sets <c>truncated</c>. A label is never
    /// worth an organism, a session, or a heartbeat.
    ///
    /// Threading and sampling (§11.2, §17 A35): <see cref="Build"/> touches Unity state, so it is
    /// **main-thread only**, and it is called from <c>MultiverseClient.PumpHeartbeat</c> in the same
    /// <c>Update</c> that reads <c>population</c>, <c>eggCount</c> and <c>simulatedTime</c>. One
    /// heartbeat therefore describes one instant, which is what makes
    /// <c>Σ bibites ≤ population</c> a checkable statement. It is **never** built in
    /// <c>FixedUpdate</c> — a per-tick registry scan is the A29 livelock — and it is never cached
    /// across heartbeats, because a repeated stale copy breaks that reconciliation.
    /// </summary>
    internal static class SpeciesCensus
    {
        internal const string Prefix = "[M5-CENSUS]";

        /// <summary>One species of this world at the instant the heartbeat was built.</summary>
        private struct Row
        {
            internal string generic;
            internal string specific;
            internal int bibites;
            internal int eggs;

            internal int Total { get { return bibites + eggs; } }
        }

        /// <summary>
        /// Re-used across heartbeats so a per-second census allocates one list, not sixty a minute.
        /// Main-thread only, like everything else here, and cleared at the top of every build.
        /// </summary>
        private static readonly List<Row> rows = new List<Row>(64);

        /// <summary>Descending by <c>bibites + eggs</c> (§17, A35). Ties fall in any order, and §5.2 says a reader must not read meaning into one.</summary>
        private static readonly Comparison<Row> ByTotalDescending = (a, b) => b.Total.CompareTo(a.Total);

        /// <summary>Rate limit for the one warning shape this can emit; a per-second log line is a defect of its own.</summary>
        private const float SkipLogIntervalSeconds = 60f;
        private static float nextSkipLogRealtime;

        /// <summary>
        /// The census for this heartbeat, or <c>null</c> when this world cannot answer the question.
        ///
        /// **`null` is absent, and absent is "unknown" all the way to the page** — it is what a menu, a
        /// loading world or a mod that predates §17 produces, and no reader may confuse it with an empty
        /// census. An **empty** array is the stronger statement: this mod is reporting, and this world
        /// genuinely has no species with anything alive in it.
        /// </summary>
        /// <param name="truncated">
        /// <c>true</c> when the returned array is **not the whole census** — the cap fired, or an entry
        /// this world holds could not be shaped. It is never set on an absent census, and §5.2 makes it
        /// monotonic from here on: every hop may set it, none may clear it.
        /// </param>
        internal static JArray Build(out bool truncated)
        {
            truncated = false;

            // The candidate set is activeSpecies (GlobalLineageManager.cs:23), never the organism list:
            // each entry already carries its own counts, so a census costs one pass over a few hundred
            // species records and no pass over the population at all.
            GlobalLineageManager manager = GlobalLineageManager.Instance;
            if (manager == null || manager.activeSpecies == null)
            {
                // No world, or one still loading. Absent, never an empty census: an empty census is a
                // claim about a world, and there is no world here to make it about.
                return null;
            }

            rows.Clear();
            int skipped = 0;
            string skipExample = null;

            List<Species> active = manager.activeSpecies;
            for (int i = 0; i < active.Count; i++)
            {
                Species species = active[i];
                if (species == null)
                {
                    continue;
                }

                // §17, A35 — apply the "≥ 1" test here rather than trusting membership. The game prunes
                // activeSpecies with RemoveAll(s => s.count < 1) on its own periodic pass
                // (GlobalLineageManager.cs:64, 90), so between passes the list can hold a species with
                // nothing alive in it, and §5.2 says *active*.
                int bibites = (species.bibites != null) ? species.bibites.Count : 0;
                int eggs = (species.eggs != null) ? species.eggs.Count : 0;
                if (bibites + eggs < 1)
                {
                    continue;
                }

                // §17, A36 — the halves are copied and NOT touched. No Trim, no whitespace collapsing,
                // no case folding, no Normalize. Edge and doubled internal whitespace are legal here and
                // are exactly what a faithful census carries.
                string generic = species.genericName;
                string specific = species.specificName;

                if (!IsShippableNameHalf(generic) || !IsShippableNameHalf(specific))
                {
                    // The only two rules a name has to satisfy — non-empty, and at most 64 UTF-8 bytes —
                    // and a record that breaks one cannot go on the wire at all. Strip the entry, keep
                    // the world, and say so with `truncated`. A whitespace-only half is NOT this case: it
                    // passes, because that is a name the world is genuinely displaying.
                    skipped++;
                    if (skipExample == null)
                    {
                        skipExample = Describe(species);
                    }

                    continue;
                }

                rows.Add(new Row { generic = generic, specific = specific, bibites = bibites, eggs = eggs });
            }

            if (skipped > 0)
            {
                truncated = true;
                LogSkips(skipped, skipExample);
            }

            // Sorting is a SENDER obligation and not a display preference: the cap below keeps the head
            // of this list, so the order is what decides which species survive it. Nothing downstream
            // re-sorts (§17, A35).
            rows.Sort(ByTotalDescending);

            int count = rows.Count;
            if (count > ContractA.SpeciesCensusMax)
            {
                count = ContractA.SpeciesCensusMax;
                truncated = true;
            }

            JArray census = new JArray();
            for (int i = 0; i < count; i++)
            {
                Row row = rows[i];
                census.Add(new JObject
                {
                    ["genericName"] = row.generic,
                    ["specificName"] = row.specific,
                    ["bibites"] = row.bibites,
                    ["eggs"] = row.eggs
                });
            }

            rows.Clear();
            return census;
        }

        /// <summary>
        /// The whole of the census name rule, and deliberately **not**
        /// <see cref="ContractA.IsValidSpeciesNameHalf"/>: non-empty and at most
        /// <see cref="ContractA.MaxSpeciesNameBytes"/> UTF-8 bytes, with **no whitespace test at all**
        /// (§17, A36). Both are checkable without an opinion about what a name should look like, which
        /// is what lets the Go sidecar and this mod agree on a census without agreeing about language.
        /// </summary>
        private static bool IsShippableNameHalf(string value)
        {
            if (string.IsNullOrEmpty(value))
            {
                return false;
            }

            // The byte count is taken on the encoded form, which is what actually travels.
            return System.Text.Encoding.UTF8.GetByteCount(value) <= ContractA.MaxSpeciesNameBytes;
        }

        private static string Describe(Species species)
        {
            try
            {
                return "speciesID=" + species.speciesID.ToString(CultureInfo.InvariantCulture)
                    + " genericName=\"" + (species.genericName ?? "<null>") + "\""
                    + " specificName=\"" + (species.specificName ?? "<null>") + "\"";
            }
            catch
            {
                return "<unreadable species record>";
            }
        }

        /// <summary>
        /// One line, at most once a minute. A species whose name cannot be shaped stays unshaped for as
        /// long as it lives, so an unthrottled line here would be a per-second log at the exact moment
        /// an operator is trying to read the log for something else.
        /// </summary>
        private static void LogSkips(int skipped, string example)
        {
            float now = Time.realtimeSinceStartup;
            if (now < nextSkipLogRealtime)
            {
                return;
            }

            nextSkipLogRealtime = now + SkipLogIntervalSeconds;
            MultiversePlugin.Log.LogWarning(
                $"{Prefix} {skipped} active species could not go into the census this heartbeat — a name half is empty or "
                + $"over {ContractA.MaxSpeciesNameBytes} UTF-8 bytes (§5.2). The census carries the rest with truncated=true. "
                + $"First: {example}");
        }
    }
}
