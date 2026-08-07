using System.Collections.Generic;
using System.Globalization;
using OneUseScripts;
using SettingScripts;
using SimulationScripts.BibiteScripts;
using UnityEngine;

namespace BibitesMultiverse
{
    /// <summary>
    /// The instrumentation behind decision D10 and the two in-game verifications that gate it
    /// (m3_considerations.md Risks 1 and 2; contract-a.md §4.3.1, §14 A13).
    ///
    /// D10 keeps <c>worldWrapping</c> ON and makes the vanilla wrap the containment mechanism for
    /// everything a capture band does not take. **Under two-way lanes that is no longer "the edges with
    /// no strip"** — every edge has one (§18, A38) — but the four cases §4.3.1 lists: an organism of an
    /// excluded species (A39), one whose edge is closed, one still inside its entry-immunity window, and
    /// one already in flight. The wrap is more load-bearing than before, not less, and the
    /// square-crossing series should fall sharply without reaching zero. That puts two rules on the same
    /// organisms:
    ///
    /// * the **wrap** teleports at <c>r − bodyLength ≥ 1.5·S + 1000</c>, keeping velocity and
    ///   rotation (m2_findings.md §3, BibitePropulsion.cs:240-242);
    /// * the **capture band** takes an export from <c>S − W</c> outward with no outer bound.
    ///
    /// They meet in the outer band, and the only thing that separates them is the outward-velocity
    /// test: a wrapped organism lands on the far side travelling **inward**. Remove that test and
    /// every wrapped arrival exports through the edge it landed behind (Risk 1).
    ///
    /// Every line here carries the grepable prefix <c>[M3-D10]</c>. The wrap events and the band
    /// decisions are the evidence the two verifications are read from, and they keep working
    /// afterwards as the diagnostic for a containment complaint.
    /// </summary>
    internal class Containment
    {
        internal const string Prefix = "[M3-D10]";

        /// <summary>
        /// A per-tick displacement larger than this is a teleport, not swimming. The fastest
        /// organisms move a few world units per tick at 40 TPS; the wrap moves about 2·(1.5·S + 1000).
        /// </summary>
        private const float TeleportThreshold = 500f;

        /// <summary>A band decision is logged at most once per organism per this many simulated seconds.</summary>
        private const double BandLogPeriodSimSeconds = 1.0;

        private struct Track
        {
            internal Vector2 position;
            internal long lastSeenTick;
            internal double nextBandLogSimTime;
        }

        private readonly Dictionary<int, Track> tracks = new Dictionary<int, Track>();
        private readonly List<int> pruneScratch = new List<int>();

        private long wrapEvents;

        internal long WrapEvents => wrapEvents;

        internal void Reset()
        {
            tracks.Clear();
            wrapEvents = 0;
        }

        internal void Forget(int entityId)
        {
            tracks.Remove(entityId);
        }

        /// <summary>
        /// One line at world load: the settings the containment claim rests on, and the two radii it
        /// says do not compete. Read-only — under D10 the mod never writes <c>worldWrapping</c>.
        /// </summary>
        internal static void LogSetup(BorderGeometry geometry, IList<Edge> exportEdges, IList<Edge> borderEdges)
        {
            string wrapping = "<unreadable>";
            string shade = "<unreadable>";
            string shadeAvoidance = "<unreadable>";
            try
            {
                ScenarioSettings settings = ScenarioSettings.Instance;
                wrapping = settings.worldWrapping.val.ToString();
                shade = settings.shadeOutsideOfBounds.val.ToString();
                shadeAvoidance = settings.shadeAvoidance.val.ToString();
            }
            catch (System.Exception e)
            {
                MultiversePlugin.Log.LogWarning($"{Prefix} could not read the boundary settings: {e.Message}");
            }

            MultiversePlugin.Log.LogInfo(
                $"{Prefix} containment armed — worldWrapping={wrapping} (the mod never writes it, D10) " +
                $"shadeOutsideOfBounds={shade} shadeAvoidance={shadeAvoidance} " +
                $"S={geometry.S.ToString("F1", CultureInfo.InvariantCulture)} " +
                $"W={geometry.W.ToString("F1", CultureInfo.InvariantCulture)} " +
                $"exportEdges=[{ContractA.EdgeNames(exportEdges)}](one capture band each) " +
                $"borderEdges=[{ContractA.EdgeNames(borderEdges)}](every one of them accepts an arrival) " +
                $"bandInner=[{BandInnerBoundaries(geometry, exportEdges)}] " +
                $"bandOuter=none wrapRadius={geometry.WrapRadius.ToString("F1", CultureInfo.InvariantCulture)}");

            if (!string.Equals(shadeAvoidance, "False", System.StringComparison.OrdinalIgnoreCase))
            {
                MultiversePlugin.Log.LogWarning(
                    $"{Prefix} shadeAvoidance is on. It shares an if/else-if with the wrap " +
                    "(BibitePropulsion.cs:235-243), so an organism past 1.5*S steers back instead of wrapping " +
                    "and the wrap is unreachable (m2_findings.md §1.3).");
            }
        }

        /// <summary>
        /// One organism, one tick, from the BibiteBody.FixedUpdate postfix. Detects the wrap by the
        /// displacement it produces: the game writes <c>rb2d.position</c> directly, so the jump shows
        /// up in <c>transform.position</c> on the tick after it fired.
        /// </summary>
        private static string BandInnerBoundaries(BorderGeometry geometry, IList<Edge> exportEdges)
        {
            System.Text.StringBuilder text = new System.Text.StringBuilder();
            for (int i = 0; i < exportEdges.Count; i++)
            {
                if (i > 0)
                {
                    text.Append(' ');
                }

                text.Append(ContractA.EdgeName(exportEdges[i])).Append('=')
                    .Append(geometry.BandInnerBoundary(exportEdges[i]).ToString("F1", CultureInfo.InvariantCulture));
            }

            return text.ToString();
        }

        private static string LandedIn(BorderGeometry geometry, IList<Edge> exportEdges, Vector2 position)
        {
            System.Text.StringBuilder text = new System.Text.StringBuilder();
            for (int i = 0; i < exportEdges.Count; i++)
            {
                if (i > 0)
                {
                    text.Append(' ');
                }

                text.Append(ContractA.EdgeName(exportEdges[i])).Append('=')
                    .Append(geometry.InCaptureBand(exportEdges[i], position));
            }

            return text.ToString();
        }

        internal void Observe(
            BibiteBody body,
            int entityId,
            IList<Edge> exportEdges,
            BorderGeometry geometry,
            Vector2 position,
            long simTick)
        {
            if (tracks.TryGetValue(entityId, out Track previous))
            {
                float moved = (position - previous.position).magnitude;
                if (moved >= TeleportThreshold)
                {
                    wrapEvents++;
                    float bodyLength = 0f;
                    float health = 0f;
                    Vector2 velocity = Vector2.zero;
                    try
                    {
                        bodyLength = body.bodyLength;
                        health = body.health;

                        // Only here: a wrap is rare, and a GetComponent for every organism on every
                        // tick would be a real cost in a world of two thousand.
                        Rigidbody2D rigidbody = body.GetComponent<Rigidbody2D>();
                        if (rigidbody != null)
                        {
                            velocity = rigidbody.linearVelocity;
                        }
                    }
                    catch (System.Exception)
                    {
                        // Reported values only.
                    }

                    MultiversePlugin.Log.LogInfo(
                        $"{Prefix} event=WRAP entityId={entityId} simTick={simTick} moved={moved.ToString("F1", CultureInfo.InvariantCulture)} " +
                        $"from=({previous.position.x.ToString("F1", CultureInfo.InvariantCulture)},{previous.position.y.ToString("F1", CultureInfo.InvariantCulture)}) " +
                        $"rBefore={previous.position.magnitude.ToString("F1", CultureInfo.InvariantCulture)} " +
                        $"to=({position.x.ToString("F1", CultureInfo.InvariantCulture)},{position.y.ToString("F1", CultureInfo.InvariantCulture)}) " +
                        $"rAfter={position.magnitude.ToString("F1", CultureInfo.InvariantCulture)} " +
                        $"vel=({velocity.x.ToString("F2", CultureInfo.InvariantCulture)},{velocity.y.ToString("F2", CultureInfo.InvariantCulture)}) " +
                        $"heading={body.transform.localRotation.eulerAngles.z.ToString("F2", CultureInfo.InvariantCulture)} " +
                        $"health={health.ToString("F2", CultureInfo.InvariantCulture)} bodyLength={bodyLength.ToString("F2", CultureInfo.InvariantCulture)} " +
                        $"wrapRadius={geometry.WrapRadius.ToString("F1", CultureInfo.InvariantCulture)} " +
                        $"landedInCaptureBand=[{LandedIn(geometry, exportEdges, position)}] " +
                        $"totalWrapEvents={wrapEvents} population={GameBridge.LivingPopulation()}");

                    previous.nextBandLogSimTime = 0.0; // let the next band decision speak immediately
                }
            }

            tracks[entityId] = new Track
            {
                position = position,
                lastSeenTick = simTick,
                nextBandLogSimTime = previous.nextBandLogSimTime
            };
        }

        /// <summary>
        /// One band decision, and the line both D10 verifications are read from.
        ///
        /// A **refusal** is throttled per organism, because an organism can loiter in the band for
        /// thousands of ticks. A **capture** never is: it happens at most once per organism per
        /// migration, and losing it to a throttle would lose the only record that the band fired.
        /// </summary>
        internal void NoteBandDecision(
            string outcome,
            int entityId,
            Edge edge,
            BorderGeometry geometry,
            Vector2 position,
            Vector2 velocity,
            bool edgeOpen,
            long simTick,
            bool throttle)
        {
            double now = TimeKeeper.simulatedTime;
            if (throttle && tracks.TryGetValue(entityId, out Track track))
            {
                if (now < track.nextBandLogSimTime)
                {
                    return;
                }

                track.nextBandLogSimTime = now + BandLogPeriodSimSeconds;
                tracks[entityId] = track;
            }

            float outward = Vector2.Dot(velocity, BorderGeometry.OutwardNormal(edge));
            float raw = geometry.RawExitPosition(edge, position);
            float clamped = geometry.ExitPosition(edge, position);

            MultiversePlugin.Log.LogInfo(
                $"{Prefix} event={outcome} entityId={entityId} simTick={simTick} edge={ContractA.EdgeName(edge)} " +
                $"pos=({position.x.ToString("F1", CultureInfo.InvariantCulture)},{position.y.ToString("F1", CultureInfo.InvariantCulture)}) " +
                $"vel=({velocity.x.ToString("F2", CultureInfo.InvariantCulture)},{velocity.y.ToString("F2", CultureInfo.InvariantCulture)}) " +
                $"outwardComponent={outward.ToString("F2", CultureInfo.InvariantCulture)} " +
                $"bandInner={geometry.BandInnerBoundary(edge).ToString("F1", CultureInfo.InvariantCulture)} " +
                $"outsideSquare={geometry.Beyond(edge, position)} " +
                $"r={position.magnitude.ToString("F1", CultureInfo.InvariantCulture)} " +
                $"wrapRadius={geometry.WrapRadius.ToString("F1", CultureInfo.InvariantCulture)} " +
                $"exitPositionRaw={raw.ToString("F6", CultureInfo.InvariantCulture)} " +
                $"exitPositionClamped={clamped.ToString("F6", CultureInfo.InvariantCulture)} " +
                $"clampFired={(!Mathf.Approximately(raw, clamped))} edgeOpen={edgeOpen}");
        }

        /// <summary>Drop organisms that have not been seen for a simulated minute at 40 TPS.</summary>
        internal void Prune(long simTick)
        {
            const long staleAfterTicks = 2400;
            const long pruneEveryTicks = 400;
            if (tracks.Count < 64 || simTick % pruneEveryTicks != 0)
            {
                return;
            }

            pruneScratch.Clear();
            foreach (KeyValuePair<int, Track> entry in tracks)
            {
                if (simTick - entry.Value.lastSeenTick > staleAfterTicks)
                {
                    pruneScratch.Add(entry.Key);
                }
            }

            foreach (int id in pruneScratch)
            {
                tracks.Remove(id);
            }
        }
    }
}
