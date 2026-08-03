using System;
using ManagementScripts;
using SettingScripts;
using UnityEngine;

namespace BibitesMultiverse
{
    /// <summary>
    /// The boundary settings the capture band has to coexist with, plus the one-off runtime facts
    /// m2_findings.md left open.
    ///
    /// **`worldWrapping` is no longer one of them.** M2 disabled it while an edge was open, so a
    /// missed capture could not teleport an organism to the antipode — and that traded the teleport
    /// for a leak, because nothing else in the game contains an organism at the square edge and the
    /// three unguarded edges then guarded nothing at all (m2_findings.md §3). Decision D10 reverses
    /// the trade: the wrap stays **ON** at all times and becomes the containment mechanism, and the
    /// mod only reads and reports the setting (contract-a.md §5.4, §11.2, §14 A13). The two radii do
    /// not compete — the capture band takes an export from `S − W` outward, before the wrap fires at
    /// `1.5·S + 1000`.
    ///
    /// One setting is still overridden, and it is restored on world unload:
    ///
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
        private bool voidAvoidanceWas;
        private bool voidAvoidanceChanged;

        /// <summary>The wrap state as read at world load. Reported on CONFIG_UPDATE, never written.</summary>
        internal bool WorldWrappingOn { get; private set; }

        internal void LogBoundarySettings(float simulationSize)
        {
            try
            {
                ScenarioSettings settings = ScenarioSettings.Instance;
                WorldWrappingOn = settings.worldWrapping.val;
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

            // D10, contract-a.md §14 A13: the mod snapshots worldWrapping and reports it. It does not
            // write it, and there is nothing to restore on unload.
            try
            {
                bool wrapping = ScenarioSettings.Instance.worldWrapping.val;
                WorldWrappingOn = wrapping;
                if (wrapping)
                {
                    MultiversePlugin.Log.LogInfo(
                        "[M2] worldWrapping is ON and stays ON — it is the containment mechanism for every edge no " +
                        "strip guards (D10). The capture band takes an export from S-W outward, before the wrap fires.");
                }
                else
                {
                    MultiversePlugin.Log.LogWarning(
                        "[M2] worldWrapping is OFF in this world. The mod does not turn it on (D10 says the mod never " +
                        "writes it), so nothing contains an organism that leaves through an unguarded edge — this world " +
                        "will leak population. Turn the setting on in the scenario.");
                }
            }
            catch (Exception e)
            {
                MultiversePlugin.Log.LogWarning($"[M2] could not read worldWrapping: {e.Message}");
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
