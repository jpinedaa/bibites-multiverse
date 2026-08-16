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
    /// Two settings are written here, and the two are written **differently** on purpose:
    ///
    /// * <c>voidAvoidance</c> (default **false**) blends the brain's steering towards the nearest
    ///   zone centre (m2_findings.md §1.1). It ships disabled, so this is measure-first: read it, and
    ///   only write when the world actually turned it on (m2_findings.md, Recommended approach,
    ///   layer 0 then the layer-2 fallback). It **is** restored on world unload, because the mod
    ///   turned it off behind the operator's back and no save should carry that decision away.
    ///
    /// * <c>pelletGrowth</c> — the game's *Global Fertility*, in E/u²s — is written only when
    ///   <see cref="EnvFertility"/> names a value, and it is **not** restored on unload. An operator
    ///   asked for this one by name, and the periodic saves of D14 have already written it to disk
    ///   long before the quit save runs: restoring only the quit save would leave the world's
    ///   fertility depending on which save it was last reloaded from. The honest rule is the simple
    ///   one — *while the variable is set, the world's saved fertility is overwritten at every start;
    ///   unset it and the world keeps whatever it last saved.*
    ///
    /// Every write uses <c>Setting.SetValue</c>. A direct write to <c>val</c> fires no event and
    /// leaves the game's cached statics stale (m2_findings.md §1.2). For fertility the event is the
    /// whole mechanism: every <c>Zone</c> subscribes to <c>pelletGrowth</c> and recomputes its pellet
    /// production from <c>fertility × area × pelletGrowth</c> when it fires (<c>Zone.cs</c>,
    /// <c>ZoneSettings.cs</c>), so a write lands on the running world with no restart.
    /// </summary>
    internal class WorldSettings
    {
        /// <summary>
        /// The world's Global Fertility, in E/u²s, as a positive number — for example
        /// <c>3.5E-05</c>. Applied to <c>ScenarioIndependentSettings.pelletGrowth</c> at every world
        /// load, and never restored (see the class remarks). Unset or empty writes nothing at all,
        /// which is what every installation that does not set it gets.
        ///
        /// **Env-only, with no BepInEx config entry**, following <see cref="MinFpsGovernor"/>: a new
        /// <c>Config.Bind</c> makes every instance on the machine rewrite one shared config file at
        /// once, and instances lose that race (dev_environment.md, Gotchas).
        ///
        /// It is applied from this class, so it shares this class's one precondition: the Contract A
        /// client has to be on, because that is what loads the world settings. A world configured
        /// with <c>MULTIVERSE_EXPORT_EDGES=none</c> never reaches here, and
        /// <see cref="MultiversePlugin"/> says so where it reports that choice.
        /// </summary>
        internal const string EnvFertility = "MULTIVERSE_FERTILITY";

        internal const string FertilityPrefix = "[M5-FOOD]";

        /// <summary>
        /// <c>pelletGrowth.DefaultValue</c>, which is also the game's "Normal" landmark. Used for the
        /// log line only; the accepted range comes from the live setting.
        /// </summary>
        private const float FertilityNormal = 1E-05f;

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

            ApplyFertility();
        }

        /// <summary>
        /// Overwrites the world's Global Fertility when <see cref="EnvFertility"/> asks for it.
        ///
        /// The knob exists for the public broadcast world, where a denser food supply is the one
        /// lever that raises juvenile survival and so keeps a watchable population on camera. It is
        /// the game's own setting, not a mod invention: nothing is spawned, moved, healed or edited
        /// here, and a bibite still has to find the food and live off it.
        ///
        /// Refusals are loud and change nothing. The setting's own <c>canGoOutOfBounds</c> is
        /// <c>true</c>, so <c>SetValue</c> would happily store a value the game's own UI cannot
        /// produce — the range check below is the only one there is.
        /// </summary>
        private void ApplyFertility()
        {
            string requested = (MultiverseConfig.Env(EnvFertility) ?? string.Empty).Trim();

            FloatSetting growth;
            try
            {
                growth = ScenarioIndependentSettings.Instance.pelletGrowth;
            }
            catch (Exception e)
            {
                MultiversePlugin.Log.LogWarning(
                    $"{FertilityPrefix} could not reach the Global Fertility setting — {EnvFertility} is not applied: {e.Message}");
                return;
            }

            if (growth == null)
            {
                MultiversePlugin.Log.LogWarning(
                    $"{FertilityPrefix} the game has no Global Fertility setting at world load — {EnvFertility} is not applied.");
                return;
            }

            float current = growth.val;
            if (requested.Length == 0)
            {
                MultiversePlugin.Log.LogInfo(
                    $"{FertilityPrefix} {EnvFertility} is unset — this world keeps the Global Fertility it was saved with, " +
                    $"{Number(current)} E/u²s. Nothing is written.");
                return;
            }

            if (!float.TryParse(
                    requested,
                    System.Globalization.NumberStyles.Float,
                    System.Globalization.CultureInfo.InvariantCulture,
                    out float value))
            {
                MultiversePlugin.Log.LogWarning(
                    $"{FertilityPrefix} {EnvFertility}='{requested}' is not a number — this world keeps its saved Global " +
                    $"Fertility of {Number(current)} E/u²s. Write it as a plain or scientific decimal, for example 3.5E-05.");
                return;
            }

            float max = growth.maxValue;
            if (float.IsNaN(value) || float.IsInfinity(value) || value <= 0f || value > max)
            {
                MultiversePlugin.Log.LogWarning(
                    $"{FertilityPrefix} {EnvFertility}='{requested}' is outside the game's range for Global Fertility — " +
                    $"it takes a positive value up to {Number(max)} E/u²s. This world keeps its saved " +
                    $"{Number(current)} E/u²s. (Zero is the game's \"Sterile\" landmark and this knob will not write it: " +
                    "a mistyped variable must not starve a world.)");
                return;
            }

            try
            {
                growth.SetValue(value);
            }
            catch (Exception e)
            {
                MultiversePlugin.Log.LogWarning(
                    $"{FertilityPrefix} could not write Global Fertility — this world keeps its saved " +
                    $"{Number(current)} E/u²s: {e.Message}");
                return;
            }

            MultiversePlugin.Log.LogInfo(
                $"{FertilityPrefix} Global Fertility {Number(current)} -> {Number(value)} E/u²s, from {EnvFertility}. " +
                $"The game's default is {Number(FertilityNormal)} (\"Normal\"); its landmarks above it are 2E-05 (\"High\") " +
                "and 5E-05 (\"Lush\"), and the shipped \"Thick Soup\" scenario uses 3.5E-05. Every zone recomputes its pellet " +
                "production on this change, so it is live now. It is NOT restored on unload: while the variable is set, this " +
                "world's saved fertility is overwritten at every start. Unset it and the world keeps whatever it last saved.");
        }

        /// <summary>Round-trippable, culture-free, and short enough to read in a log line.</summary>
        private static string Number(float value)
        {
            return value.ToString("G6", System.Globalization.CultureInfo.InvariantCulture);
        }

        internal void Restore()
        {
            if (!applied)
            {
                return;
            }

            applied = false;

            // Global Fertility is deliberately absent here. The periodic saves of D14 have already
            // written the applied value to disk, so restoring it for the quit save alone would make a
            // world's fertility depend on which save it is next reloaded from. See the class remarks.
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
