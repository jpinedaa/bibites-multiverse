using System;
using ManagementScripts;
using SettingScripts;
using UnityEngine;

namespace BibitesMultiverse
{
    /// <summary>
    /// The boundary settings the border strip has to fight, plus the one-off runtime facts
    /// m2_findings.md left open.
    ///
    /// Two settings are overridden while an edge is open, and both are restored on world unload:
    ///
    /// * <c>worldWrapping</c> (default **true**) teleports an organism past 1.5*S + 1000 to the
    ///   antipode (m2_findings.md §3, m2_considerations.md Risk 4). A missed capture would read to a
    ///   player as a duplication bug caused by this mod.
    /// * <c>voidAvoidance</c> (default **false**) blends the brain's steering towards the nearest
    ///   zone centre (m2_findings.md §1.1). It ships disabled, so this is measure-first: read it, and
    ///   only write when the world actually turned it on (m2_findings.md, Recommended approach,
    ///   layer 0 then the layer-2 fallback).
    ///
    /// Every write uses <c>Setting.SetValue</c>. A direct write to <c>val</c> fires no event and
    /// leaves the game's cached statics stale (m2_findings.md §1.2).
    /// </summary>
    internal class WorldSettings
    {
        private bool applied;
        private bool worldWrappingWas;
        private bool worldWrappingChanged;
        private bool voidAvoidanceWas;
        private bool voidAvoidanceChanged;

        internal void LogBoundarySettings(float simulationSize)
        {
            try
            {
                ScenarioSettings settings = ScenarioSettings.Instance;
                MultiversePlugin.Log.LogInfo(
                    $"[M2] boundary settings at world load: SimulationSize={simulationSize:F2} " +
                    $"voidAvoidance={settings.voidAvoidance.val} voidAvoidanceDistance={settings.voidAvoidanceDistance.val:F1} " +
                    $"shadeOutsideOfBounds={settings.shadeOutsideOfBounds.val} shadeAvoidance={settings.shadeAvoidance.val} " +
                    $"worldWrapping={settings.worldWrapping.val} corpsesEnabled={settings.corpsesEnabled.val}");
            }
            catch (Exception e)
            {
                MultiversePlugin.Log.LogWarning($"[M2] could not read the boundary settings: {e.Message}");
            }
        }

        /// <summary>
        /// m2_considerations.md Risk 2 / m2_findings.md open uncertainty 1: the payload's "transform"
        /// key holds localPosition while "rb2d" holds the world position, and the two agree only when
        /// the bibiteHolder transform is the identity. That object is Unity scene data, so the
        /// decompiled source cannot answer it — log it once and settle the question.
        /// </summary>
        internal void LogBibiteHolderTransform()
        {
            try
            {
                WorldObjectsSpawner spawner = WorldObjectsSpawner.Instance;
                if (spawner == null || spawner.bibiteHolder == null)
                {
                    MultiversePlugin.Log.LogWarning("[M2] WorldObjectsSpawner.Instance.bibiteHolder is not available yet — cannot report its transform.");
                    return;
                }

                Transform holder = spawner.bibiteHolder.transform;
                bool identity = holder.position == Vector3.zero
                    && holder.rotation == Quaternion.identity
                    && holder.lossyScale == Vector3.one;
                MultiversePlugin.Log.LogInfo(
                    $"[M2] bibiteHolder transform: position=({holder.position.x:F6},{holder.position.y:F6},{holder.position.z:F6}) " +
                    $"rotation={holder.rotation.eulerAngles} lossyScale={holder.lossyScale} identity={identity} " +
                    "(identity means the payload's 'transform' key is world space — m2_findings.md §4.3 caveat 1)");
            }
            catch (Exception e)
            {
                MultiversePlugin.Log.LogWarning($"[M2] could not read the bibiteHolder transform: {e.Message}");
            }
        }

        internal void Apply()
        {
            if (applied)
            {
                return;
            }

            applied = true;

            try
            {
                BoolSetting wrapping = ScenarioSettings.Instance.worldWrapping;
                worldWrappingWas = wrapping.val;
                if (worldWrappingWas)
                {
                    wrapping.SetValue(false);
                    worldWrappingChanged = true;
                    MultiversePlugin.Log.LogInfo(
                        "[M2] worldWrapping was on — disabled while the edge is open so a missed capture cannot teleport an " +
                        "organism to the antipode (m2_findings.md §3). It is restored on world unload.");
                }
                else
                {
                    MultiversePlugin.Log.LogInfo("[M2] worldWrapping is already off — nothing to override.");
                }
            }
            catch (Exception e)
            {
                MultiversePlugin.Log.LogWarning($"[M2] could not override worldWrapping: {e.Message}");
            }

            try
            {
                BoolSetting avoidance = ScenarioSettings.Instance.voidAvoidance;
                voidAvoidanceWas = avoidance.val;
                if (voidAvoidanceWas)
                {
                    avoidance.SetValue(false);
                    voidAvoidanceChanged = true;
                    MultiversePlugin.Log.LogInfo(
                        "[M2] voidAvoidance was on — disabled globally for this session (m2_findings.md §1.5 option 2). " +
                        "It is restored on world unload.");
                }
                else
                {
                    MultiversePlugin.Log.LogInfo(
                        "[M2] voidAvoidance is off, which is the shipped default — there is nothing to suppress " +
                        "(m2_findings.md §1.4). The crossing rate is an ecology question, not a steering one.");
                }
            }
            catch (Exception e)
            {
                MultiversePlugin.Log.LogWarning($"[M2] could not read voidAvoidance: {e.Message}");
            }
        }

        internal void Restore()
        {
            if (!applied)
            {
                return;
            }

            applied = false;

            if (worldWrappingChanged)
            {
                worldWrappingChanged = false;
                try
                {
                    ScenarioSettings.Instance.worldWrapping.SetValue(worldWrappingWas);
                    MultiversePlugin.Log.LogInfo($"[M2] worldWrapping restored to {worldWrappingWas}.");
                }
                catch (Exception e)
                {
                    MultiversePlugin.Log.LogWarning($"[M2] could not restore worldWrapping: {e.Message}");
                }
            }

            if (voidAvoidanceChanged)
            {
                voidAvoidanceChanged = false;
                try
                {
                    ScenarioSettings.Instance.voidAvoidance.SetValue(voidAvoidanceWas);
                    MultiversePlugin.Log.LogInfo($"[M2] voidAvoidance restored to {voidAvoidanceWas}.");
                }
                catch (Exception e)
                {
                    MultiversePlugin.Log.LogWarning($"[M2] could not restore voidAvoidance: {e.Message}");
                }
            }
        }
    }
}
