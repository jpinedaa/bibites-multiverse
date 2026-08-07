using System;
using SettingScripts;
using UnityEngine;
using UnityEngine.Events;

namespace BibitesMultiverse
{
    /// <summary>
    /// The border strip, defined against the live simulation half-extent S.
    ///
    /// S is <c>ScenarioIndependentSettings.Instance.SimulationSize</c> (m2_findings.md §2.1). It is an
    /// ordinary FloatSetting and is live-mutable, so this class **subscribes and never snapshots**
    /// (m2_findings.md §2.4, m2_considerations.md Risk 3). A cached S would silently relocate the
    /// strip and put two paired sims out of agreement about their shared edge.
    /// </summary>
    internal class BorderGeometry
    {
        private readonly MultiverseConfig config;
        private readonly UnityAction<float> subscription;
        private readonly Action<float> onChanged;

        private float simulationSize = 2000f;
        private bool subscribed;

        internal BorderGeometry(MultiverseConfig config, Action<float> onChanged)
        {
            this.config = config;
            this.onChanged = onChanged;
            subscription = OnSimulationSizeChanged;
        }

        /// <summary>The playable half-extent. The playable square is [-S,+S] x [-S,+S].</summary>
        internal float S => simulationSize;

        /// <summary>The strip width W, in world units.</summary>
        internal float W
        {
            get
            {
                if (config.BorderWidthOverride > 0f)
                {
                    return config.BorderWidthOverride;
                }

                return Mathf.Max(MultiverseConfig.MinimumWidth, MultiverseConfig.DefaultWidthFactor * simulationSize);
            }
        }

        /// <summary>
        /// How far past the strip's inner face an arriving organism is placed (m2_findings.md §4.4,
        /// guard 1). Entry lands at W + EntryMargin from the edge, so it starts outside its own strip.
        /// </summary>
        internal float EntryMargin => Mathf.Max(5f, 0.5f * W);

        /// <summary>
        /// Hysteresis: an organism that was refused has to come back this far inside the strip's inner
        /// face before it may trigger again (m2_considerations.md, WP2).
        /// </summary>
        internal float Hysteresis => Mathf.Max(2f, 0.25f * W);

        internal void Subscribe()
        {
            if (subscribed)
            {
                return;
            }

            FloatSetting setting = Setting();
            if (setting == null)
            {
                MultiversePlugin.Log.LogError("[M2] ScenarioIndependentSettings.Instance.SimulationSize is unreachable — S stays at its last value.");
                return;
            }

            // call: true seeds the current value through the same path a later change takes.
            setting.Subscribe(subscription, call: true);
            subscribed = true;
        }

        internal void Unsubscribe()
        {
            if (!subscribed)
            {
                return;
            }

            FloatSetting setting = Setting();
            setting?.UnSubscribe(subscription);
            subscribed = false;
        }

        private static FloatSetting Setting()
        {
            try
            {
                return ScenarioIndependentSettings.Instance?.SimulationSize;
            }
            catch (Exception e)
            {
                MultiversePlugin.Log.LogWarning($"[M2] could not reach SimulationSize: {e.Message}");
                return null;
            }
        }

        private void OnSimulationSizeChanged(float value)
        {
            if (Mathf.Approximately(value, simulationSize))
            {
                simulationSize = value;
                return;
            }

            float previous = simulationSize;
            simulationSize = value;
            MultiversePlugin.Log.LogInfo($"[M2] SimulationSize changed {previous:F2} -> {value:F2} (W is now {W:F2}).");
            onChanged?.Invoke(value);
        }

        // ---- edge maths (contract-a.md §4.3) --------------------------------------------------

        internal static Vector2 OutwardNormal(Edge edge)
        {
            switch (edge)
            {
                case Edge.N: return new Vector2(0f, 1f);
                case Edge.S: return new Vector2(0f, -1f);
                case Edge.E: return new Vector2(1f, 0f);
                case Edge.W: return new Vector2(-1f, 0f);
                default: return Vector2.zero;
            }
        }

        /// <summary>The coordinate that the edge fixes: y for N/S, x for E/W.</summary>
        internal static float FixedCoordinate(Edge edge, Vector2 position)
        {
            return (edge == Edge.N || edge == Edge.S) ? position.y : position.x;
        }

        /// <summary>The coordinate that runs along the edge: x for N/S, y for E/W.</summary>
        internal static float FreeCoordinate(Edge edge, Vector2 position)
        {
            return (edge == Edge.N || edge == Edge.S) ? position.x : position.y;
        }

        /// <summary>
        /// §4.3.1 — the capture band, and the **whole** geometric half of the export trigger.
        ///
        /// It is half-open and outward-unbounded: it starts at the strip line <c>S − W</c> and runs
        /// outward past <c>S</c>, past the wrap radius, to any coordinate. There is no outer bound,
        /// because a fast organism clears the whole strip in one FixedUpdate and M2's inside-only
        /// rule then let the vanilla wrap teleport it to the antipode, which a player reads as a
        /// defect in the mod (m3_considerations.md Risk 2, contract-a.md §14 A13).
        ///
        /// **One projection, not one rule per edge** (§18, A38). The contract states the band as
        /// <c>p · n(e) ≥ S − W</c> with the outward normals <c>n(E) = (1,0)</c>, <c>n(N) = (0,1)</c>,
        /// <c>n(W) = (−1,0)</c>, <c>n(S) = (0,−1)</c>. Written out that is <c>x ≥ S − W</c> on `E` and
        /// <c>x ≤ −(S − W)</c> on `W` — the same band, mirrored — which is exactly what the
        /// <c>positiveSide</c> branch below computes, and why two-way lanes add no new geometry.
        ///
        /// The other half of the trigger is the outward-velocity test, and it is what separates an
        /// export from a wrap: a wrapped organism arrives in the outer band on the **far** side and
        /// travels inward, so it fails that test. The caller applies it — see
        /// <see cref="MultiverseClient.OnBodyTick"/>.
        /// </summary>
        internal bool InCaptureBand(Edge edge, Vector2 position)
        {
            float fixedCoordinate = FixedCoordinate(edge, position);
            bool positiveSide = edge == Edge.N || edge == Edge.E;
            return positiveSide
                ? fixedCoordinate >= simulationSize - W
                : fixedCoordinate <= -simulationSize + W;
        }

        /// <summary>
        /// §4.3.1's other half, made explicit so every caller applies the same rule: the organism must
        /// be **leaving** through that edge. This is what separates an export from a wrap — a wrapped
        /// organism arrives in the outer band on the far side and travels inward, so it fails the test
        /// (m3_considerations.md Risk 1). It is REQUIRED everywhere in the band, not only outside the
        /// square, and it is a strict comparison against zero with no magnitude floor
        /// (contract-a.md §12, open item 7).
        ///
        /// The dot product is what makes the sign convention hold on `W` and `S` without a second rule:
        /// <c>outward(v, W) = −vx</c>, so "leaving through the west edge" is <c>vx &lt; 0</c>, and
        /// <c>outward(v, S) = −vy</c> likewise. That is also the whole anti-boomerang argument under
        /// two-way lanes (§18, A38, rule 1): velocity is copied and never mirrored (§4.4), so an
        /// organism captured on `E` with <c>vx &gt; 0</c> arrives on `W` still moving east, and
        /// <c>outward(v, W) &lt; 0</c> keeps it out of the `W` band on the first tick and everywhere in
        /// it. Only a bounce-back arrives moving outward, by construction, and the entry-immunity
        /// window is what covers that case.
        /// </summary>
        internal static float OutwardComponent(Edge edge, Vector2 velocity)
        {
            return Vector2.Dot(velocity, OutwardNormal(edge));
        }

        /// <summary>
        /// The quarter of the playable square nearest one edge — the crowding metric's window
        /// (m4_considerations.md, Question 9). The entry quarter of `W` is `x ≤ −S/2`, which is a
        /// quarter of the map's area and wide enough to hold the mass the T1 run left on that border
        /// without being so wide that ordinary traffic saturates it.
        /// </summary>
        internal bool InEntryQuarter(Edge edge, Vector2 position)
        {
            float fixedCoordinate = FixedCoordinate(edge, position);
            bool positiveSide = edge == Edge.N || edge == Edge.E;
            float boundary = 0.5f * simulationSize;
            return positiveSide ? fixedCoordinate >= boundary : fixedCoordinate <= -boundary;
        }

        /// <summary>The inner boundary of the capture band on that edge, in world units.</summary>
        internal float BandInnerBoundary(Edge edge)
        {
            bool positiveSide = edge == Edge.N || edge == Edge.E;
            return positiveSide ? simulationSize - W : -simulationSize + W;
        }

        /// <summary>
        /// The radius at which <c>BibitePropulsion.UpdateOrgan</c> teleports an organism to the
        /// antipode: <c>r − bodyLength ≥ 1.5·S + 1000</c> (m2_findings.md §3, BibitePropulsion.cs:240).
        /// Reported, never enforced — under D10 the mod does not touch the wrap.
        /// </summary>
        internal float WrapRadius => 1.5f * simulationSize + 1000f;

        /// <summary>True when the point has cleared the strip's inner face by the hysteresis margin.</summary>
        internal bool ClearOfStrip(Edge edge, Vector2 position)
        {
            float fixedCoordinate = FixedCoordinate(edge, position);
            bool positiveSide = edge == Edge.N || edge == Edge.E;
            return positiveSide
                ? fixedCoordinate < simulationSize - W - Hysteresis
                : fixedCoordinate > -simulationSize + W + Hysteresis;
        }

        /// <summary>True when the point is outside the playable square on that edge — a natural crossing.</summary>
        internal bool Beyond(Edge edge, Vector2 position)
        {
            float fixedCoordinate = FixedCoordinate(edge, position);
            bool positiveSide = edge == Edge.N || edge == Edge.E;
            return positiveSide ? fixedCoordinate > simulationSize : fixedCoordinate < -simulationSize;
        }

        /// <summary>
        /// §4.3 before the clamp. Kept separate so the log can show what the clamp actually did: with
        /// the band of §4.3.1 reaching outside the square, a diagonal escape at x = 3500, y = 6000
        /// with S = 2000 produces 2.0, and the clamp is now load-bearing rather than cosmetic
        /// (§14, A13).
        /// </summary>
        internal float RawExitPosition(Edge edge, Vector2 position)
        {
            float free = FreeCoordinate(edge, position);
            return (free + simulationSize) / (2f * simulationSize);
        }

        /// <summary>§4.3 — the normalized position along the edge, always clamped by the sender.</summary>
        internal float ExitPosition(Edge edge, Vector2 position)
        {
            return Mathf.Clamp01(RawExitPosition(edge, position));
        }

        /// <summary>
        /// §4.3 inverse, applied with this sim's own S, W and margin. The absolute entry point is the
        /// mod's business alone — the sidecar never converts a normalized position (§4.3, note).
        /// </summary>
        internal Vector2 EntryPoint(Edge edge, float entryPosition)
        {
            float free = 2f * simulationSize * Mathf.Clamp01(entryPosition) - simulationSize;
            float inset = W + EntryMargin;

            // Keep the free coordinate off the exact corner, so a corner arrival is not inside the
            // strip of the two neighbouring edges as well.
            float freeLimit = simulationSize - inset;
            free = Mathf.Clamp(free, -freeLimit, freeLimit);

            switch (edge)
            {
                case Edge.N: return new Vector2(free, simulationSize - inset);
                case Edge.S: return new Vector2(free, -simulationSize + inset);
                case Edge.E: return new Vector2(simulationSize - inset, free);
                case Edge.W: return new Vector2(-simulationSize + inset, free);
                default: return Vector2.zero;
            }
        }
    }
}
