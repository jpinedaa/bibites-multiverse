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

        /// <summary>True when the point is inside the strip on that edge.</summary>
        internal bool InStrip(Edge edge, Vector2 position)
        {
            float fixedCoordinate = FixedCoordinate(edge, position);
            bool positiveSide = edge == Edge.N || edge == Edge.E;
            return positiveSide
                ? fixedCoordinate >= simulationSize - W
                : fixedCoordinate <= -simulationSize + W;
        }

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

        /// <summary>§4.3 — the normalized position along the edge, always clamped by the sender.</summary>
        internal float ExitPosition(Edge edge, Vector2 position)
        {
            float free = FreeCoordinate(edge, position);
            float normalized = (free + simulationSize) / (2f * simulationSize);
            return Mathf.Clamp01(normalized);
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
