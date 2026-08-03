using System.Collections.Generic;
using System.Globalization;
using OneUseScripts;
using UnityEngine;

namespace BibitesMultiverse
{
    /// <summary>
    /// The measure-first instrumentation of m2_considerations.md Risk 1.
    ///
    /// D3 assumed vanilla void avoidance keeps the crossing rate near zero. m2_findings.md §1.4 shows
    /// void avoidance ships disabled and that what actually holds organisms on their island is food
    /// density. So the lure decision (a corridor zone) must be made from a measured number, not from
    /// the premise. This counts two things per simulated minute, on the configured edge:
    ///
    /// * <b>strip entries</b> — an organism crossing into the border strip from inside;
    /// * <b>natural crossings</b> — an organism passing the playable square edge itself.
    ///
    /// Every line carries the grepable prefix [M2-CROSSING].
    /// </summary>
    internal class CrossingStats
    {
        internal const string Prefix = "[M2-CROSSING]";
        private const double ReportPeriodSimSeconds = 60.0;

        private struct Observation
        {
            internal bool inStrip;
            internal bool beyond;
            internal long lastSeenTick;
        }

        private readonly Dictionary<int, Observation> observations = new Dictionary<int, Observation>();
        private readonly List<int> pruneScratch = new List<int>();

        private double windowStartSimTime;
        private long windowIndex;
        private int windowStripEntries;
        private int windowCrossings;
        private long totalStripEntries;
        private long totalCrossings;
        private double startSimTime;

        internal long TotalStripEntries => totalStripEntries;
        internal long TotalCrossings => totalCrossings;

        internal void Reset()
        {
            observations.Clear();
            windowStartSimTime = TimeKeeper.simulatedTime;
            startSimTime = windowStartSimTime;
            windowIndex = 0;
            windowStripEntries = 0;
            windowCrossings = 0;
            totalStripEntries = 0;
            totalCrossings = 0;
        }

        /// <summary>
        /// Called once per organism per tick from the BibiteBody.FixedUpdate postfix. It records the
        /// transitions, not the states, so an organism loitering in the strip is counted once.
        /// </summary>
        internal void Observe(int entityId, Edge edge, BorderGeometry geometry, Vector2 position, long simTick)
        {
            bool inStrip = geometry.InStrip(edge, position);
            bool beyond = geometry.Beyond(edge, position);

            if (observations.TryGetValue(entityId, out Observation previous))
            {
                if (inStrip && !previous.inStrip)
                {
                    windowStripEntries++;
                    totalStripEntries++;
                }

                if (beyond && !previous.beyond)
                {
                    windowCrossings++;
                    totalCrossings++;
                }
            }
            else if (inStrip)
            {
                // First sight already inside the strip: count it, otherwise an organism that spawns or
                // arrives there is invisible to the measurement.
                windowStripEntries++;
                totalStripEntries++;
            }

            observations[entityId] = new Observation { inStrip = inStrip, beyond = beyond, lastSeenTick = simTick };
        }

        internal void Forget(int entityId)
        {
            observations.Remove(entityId);
        }

        /// <summary>Called from the client's FixedUpdate. Emits at most one line per simulated minute.</summary>
        internal void Tick(Edge edge, BorderGeometry geometry, int population, long simTick)
        {
            double now = TimeKeeper.simulatedTime;
            if (now - windowStartSimTime < ReportPeriodSimSeconds)
            {
                Prune(simTick);
                return;
            }

            windowIndex++;
            double elapsedMinutes = (now - startSimTime) / 60.0;
            double cumulative = (elapsedMinutes > 0.0) ? totalStripEntries / elapsedMinutes : 0.0;

            MultiversePlugin.Log.LogInfo(
                $"{Prefix} edge={ContractA.EdgeName(edge)} simMinute={windowIndex.ToString(CultureInfo.InvariantCulture)} " +
                $"stripEntries={windowStripEntries} crossings={windowCrossings} " +
                $"totalStripEntries={totalStripEntries} totalCrossings={totalCrossings} " +
                $"cumulativeStripEntriesPerSimMin={cumulative.ToString("F2", CultureInfo.InvariantCulture)} " +
                $"population={population} S={geometry.S.ToString("F1", CultureInfo.InvariantCulture)} " +
                $"W={geometry.W.ToString("F1", CultureInfo.InvariantCulture)} simTick={simTick}");

            windowStartSimTime = now;
            windowStripEntries = 0;
            windowCrossings = 0;
            Prune(simTick);
        }

        private void Prune(long simTick)
        {
            // 2400 ticks is 60 s at the default 40 TPS (m1_findings.md §8).
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
                observations.Remove(id);
            }
        }
    }
}
