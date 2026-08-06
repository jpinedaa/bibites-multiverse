using System.Collections.Generic;
using System.Globalization;
using System.Text;
using OneUseScripts;
using UnityEngine;

namespace BibitesMultiverse
{
    /// <summary>
    /// The border instrumentation, in two series over one pass.
    ///
    /// <b>[M2-CROSSING]</b> is m2_considerations.md Risk 1's measure-first counter. D3 assumed vanilla
    /// void avoidance keeps the crossing rate near zero; m2_findings.md §1.4 shows void avoidance ships
    /// disabled and that what actually holds organisms on their island is food density. So the lure
    /// decision has to be made from a measured number. Per simulated minute, **per export edge**:
    ///
    /// * <b>strip entries</b> — an organism crossing into that edge's capture band from inside;
    /// * <b>natural crossings</b> — an organism passing the playable square edge itself.
    ///
    /// <b>[M4-CROWDING]</b> is m4_considerations.md Question 9's answer-check. The owner saw a mass of
    /// organisms on the west border after the overnight run, and named the cause: a slot slept for two
    /// hours and both piles released together at wake. M4 pays for a delivery rate limit at the
    /// sidecar, and **this is the only proof that a burst arrived as a stream**. Per simulated minute,
    /// per entry edge: the live count inside that edge's quarter of the map, the arrivals in the
    /// window, and an eight-bucket histogram of where along the edge they landed.
    ///
    /// Both series come out of one dictionary and one pass over the organisms the Harmony postfix
    /// already visits. The quarter count is maintained on **transitions**, so the per-organism per-tick
    /// cost is one dictionary probe and one comparison per declared edge — which is what "cheap" has to
    /// mean at six instances running at 20×.
    /// </summary>
    internal class CrossingStats
    {
        internal const string Prefix = "[M2-CROSSING]";
        internal const string CrowdingPrefix = "[M4-CROWDING]";

        private const double ReportPeriodSimSeconds = 60.0;
        private const int Slots = 5; // Edge.None, N, S, E, W
        private const int HistogramBuckets = 8;

        private struct Observation
        {
            /// <summary>Bit per edge slot: the organism was inside that edge's capture band.</summary>
            internal int inBand;

            /// <summary>Bit per edge slot: the organism was outside the square on that edge.</summary>
            internal int beyond;

            /// <summary>Bit per edge slot: the organism was inside that edge's quarter of the map.</summary>
            internal int inQuarter;

            internal long lastSeenTick;
            internal bool seen;
        }

        private readonly Dictionary<int, Observation> observations = new Dictionary<int, Observation>();
        private readonly List<int> pruneScratch = new List<int>();

        private readonly int[] windowStripEntries = new int[Slots];
        private readonly int[] windowCrossings = new int[Slots];
        private readonly long[] totalStripEntries = new long[Slots];
        private readonly long[] totalCrossings = new long[Slots];

        private readonly int[] quarterCount = new int[Slots];
        private readonly int[] windowArrivals = new int[Slots];
        private readonly long[] totalArrivals = new long[Slots];
        private readonly int[,] windowHistogram = new int[Slots, HistogramBuckets];

        private double windowStartSimTime;
        private double startSimTime;
        private long windowIndex;
        private long grandStripEntries;
        private long grandCrossings;

        internal long TotalStripEntries => grandStripEntries;
        internal long TotalCrossings => grandCrossings;

        internal void Reset()
        {
            observations.Clear();
            windowStartSimTime = TimeKeeper.simulatedTime;
            startSimTime = windowStartSimTime;
            windowIndex = 0;
            grandStripEntries = 0;
            grandCrossings = 0;

            for (int slot = 0; slot < Slots; slot++)
            {
                windowStripEntries[slot] = 0;
                windowCrossings[slot] = 0;
                totalStripEntries[slot] = 0;
                totalCrossings[slot] = 0;
                quarterCount[slot] = 0;
                windowArrivals[slot] = 0;
                totalArrivals[slot] = 0;
                for (int bucket = 0; bucket < HistogramBuckets; bucket++)
                {
                    windowHistogram[slot, bucket] = 0;
                }
            }
        }

        /// <summary>
        /// Called once per organism per tick from the BibiteBody.FixedUpdate postfix. It records the
        /// transitions, not the states, so an organism loitering in a band is counted once — and the
        /// entry-quarter population is a live count kept by those same transitions.
        /// </summary>
        internal void Observe(int entityId, MultiverseConfig config, BorderGeometry geometry, Vector2 position, long simTick)
        {
            int inBand = 0;
            int beyond = 0;
            int inQuarter = 0;

            for (int i = 0; i < config.ExportEdges.Count; i++)
            {
                Edge edge = config.ExportEdges[i];
                int bit = 1 << (int)edge;
                if (geometry.InCaptureBand(edge, position))
                {
                    inBand |= bit;
                }

                if (geometry.Beyond(edge, position))
                {
                    beyond |= bit;
                }
            }

            for (int i = 0; i < config.EntryEdges.Count; i++)
            {
                Edge edge = config.EntryEdges[i];
                if (geometry.InEntryQuarter(edge, position))
                {
                    inQuarter |= 1 << (int)edge;
                }
            }

            observations.TryGetValue(entityId, out Observation previous);

            for (int i = 0; i < config.ExportEdges.Count; i++)
            {
                Edge edge = config.ExportEdges[i];
                int slot = (int)edge;
                int bit = 1 << slot;

                bool nowInBand = (inBand & bit) != 0;
                bool wasInBand = previous.seen && (previous.inBand & bit) != 0;

                // First sight already inside the band counts: otherwise an organism that spawns or
                // arrives there is invisible to the measurement.
                if (nowInBand && !wasInBand)
                {
                    windowStripEntries[slot]++;
                    totalStripEntries[slot]++;
                    grandStripEntries++;
                }

                bool nowBeyond = (beyond & bit) != 0;
                bool wasBeyond = previous.seen && (previous.beyond & bit) != 0;
                if (nowBeyond && !wasBeyond)
                {
                    windowCrossings[slot]++;
                    totalCrossings[slot]++;
                    grandCrossings++;
                }
            }

            for (int i = 0; i < config.EntryEdges.Count; i++)
            {
                int slot = (int)config.EntryEdges[i];
                int bit = 1 << slot;
                bool nowIn = (inQuarter & bit) != 0;
                bool wasIn = previous.seen && (previous.inQuarter & bit) != 0;
                if (nowIn && !wasIn)
                {
                    quarterCount[slot]++;
                }
                else if (!nowIn && wasIn)
                {
                    quarterCount[slot]--;
                }
            }

            observations[entityId] = new Observation
            {
                inBand = inBand,
                beyond = beyond,
                inQuarter = inQuarter,
                lastSeenTick = simTick,
                seen = true
            };
        }

        /// <summary>An organism left this world — drop it out of every live count.</summary>
        internal void Forget(int entityId)
        {
            if (observations.TryGetValue(entityId, out Observation previous))
            {
                Release(previous);
                observations.Remove(entityId);
            }
        }

        /// <summary>
        /// One arrival, counted at the moment the importer restored the organism. The histogram bucket
        /// is the position along the entry edge, which is what says whether arrivals cluster
        /// transversally (Question 9, secondary contributor 2) or spread.
        /// </summary>
        internal void NoteArrival(Edge entryEdge, float entryPosition)
        {
            if (entryEdge == Edge.None)
            {
                return;
            }

            int slot = (int)entryEdge;
            windowArrivals[slot]++;
            totalArrivals[slot]++;

            int bucket = Mathf.Clamp((int)(Mathf.Clamp01(entryPosition) * HistogramBuckets), 0, HistogramBuckets - 1);
            windowHistogram[slot, bucket]++;
        }

        /// <summary>Called from the client's FixedUpdate. Emits at most one line per edge per simulated minute.</summary>
        internal void Tick(MultiverseConfig config, BorderGeometry geometry, int population, long simTick)
        {
            double now = TimeKeeper.simulatedTime;
            if (now - windowStartSimTime < ReportPeriodSimSeconds)
            {
                Prune(simTick);
                return;
            }

            windowIndex++;
            double elapsedMinutes = (now - startSimTime) / 60.0;

            foreach (Edge edge in config.ExportEdges)
            {
                int slot = (int)edge;
                double cumulative = (elapsedMinutes > 0.0) ? totalStripEntries[slot] / elapsedMinutes : 0.0;
                MultiversePlugin.Log.LogInfo(
                    $"{Prefix} edge={ContractA.EdgeName(edge)} simMinute={windowIndex.ToString(CultureInfo.InvariantCulture)} " +
                    $"stripEntries={windowStripEntries[slot]} crossings={windowCrossings[slot]} " +
                    $"totalStripEntries={totalStripEntries[slot]} totalCrossings={totalCrossings[slot]} " +
                    $"cumulativeStripEntriesPerSimMin={cumulative.ToString("F2", CultureInfo.InvariantCulture)} " +
                    $"population={population} S={geometry.S.ToString("F1", CultureInfo.InvariantCulture)} " +
                    $"W={geometry.W.ToString("F1", CultureInfo.InvariantCulture)} simTick={simTick}");

                windowStripEntries[slot] = 0;
                windowCrossings[slot] = 0;
            }

            foreach (Edge edge in config.EntryEdges)
            {
                int slot = (int)edge;
                double cumulativeArrivals = (elapsedMinutes > 0.0) ? totalArrivals[slot] / elapsedMinutes : 0.0;
                double share = (population > 0) ? (double)quarterCount[slot] / population : 0.0;

                MultiversePlugin.Log.LogInfo(
                    $"{CrowdingPrefix} edge={ContractA.EdgeName(edge)} simMinute={windowIndex.ToString(CultureInfo.InvariantCulture)} " +
                    $"inQuarter={quarterCount[slot]} population={population} " +
                    $"quarterShare={share.ToString("F3", CultureInfo.InvariantCulture)} " +
                    $"arrivals={windowArrivals[slot]} totalArrivals={totalArrivals[slot]} " +
                    $"arrivalsPerSimMin={cumulativeArrivals.ToString("F2", CultureInfo.InvariantCulture)} " +
                    $"arrivalHistogram=[{Histogram(slot)}] " +
                    $"quarterInner={(0.5f * geometry.S).ToString("F1", CultureInfo.InvariantCulture)} simTick={simTick}");

                windowArrivals[slot] = 0;
                for (int bucket = 0; bucket < HistogramBuckets; bucket++)
                {
                    windowHistogram[slot, bucket] = 0;
                }
            }

            windowStartSimTime = now;
            Prune(simTick);
        }

        private string Histogram(int slot)
        {
            StringBuilder text = new StringBuilder();
            for (int bucket = 0; bucket < HistogramBuckets; bucket++)
            {
                if (bucket > 0)
                {
                    text.Append(',');
                }

                text.Append(windowHistogram[slot, bucket].ToString(CultureInfo.InvariantCulture));
            }

            return text.ToString();
        }

        private void Release(Observation observation)
        {
            if (!observation.seen || observation.inQuarter == 0)
            {
                return;
            }

            for (int slot = 0; slot < Slots; slot++)
            {
                if ((observation.inQuarter & (1 << slot)) != 0 && quarterCount[slot] > 0)
                {
                    quarterCount[slot]--;
                }
            }
        }

        private void Prune(long simTick)
        {
            // 2400 ticks is 60 s at the default 40 TPS (m1_findings.md §8). An organism that stopped
            // ticking is dead or destroyed, so it must also leave the live entry-quarter count.
            const long staleAfterTicks = 2400;
            if (observations.Count < 64)
            {
                return;
            }

            pruneScratch.Clear();
            foreach (KeyValuePair<int, Observation> entry in observations)
            {
                if (simTick - entry.Value.lastSeenTick > staleAfterTicks)
                {
                    pruneScratch.Add(entry.Key);
                }
            }

            foreach (int id in pruneScratch)
            {
                Release(observations[id]);
                observations.Remove(id);
            }
        }
    }
}
