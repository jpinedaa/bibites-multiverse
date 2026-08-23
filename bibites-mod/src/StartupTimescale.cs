using System;
using System.Globalization;
using ManagementScripts;
using UnityEngine;

namespace BibitesMultiverse
{
    /// <summary>
    /// The speed a world starts at.
    ///
    /// <b>The game resets every world to x1, in code.</b> <c>SimulationManager.Start</c> runs
    /// <c>TimeController.targetTimeScale.SetValue(1f)</c> and <c>engineTimeScale.SetValue(1f)</c> on
    /// every simulation scene — a new game, a loaded save and an autosave reload alike. There is no
    /// saved speed anywhere for an installer to seed: <c>targetTimeScale</c> is a
    /// <c>[NonSerialized]</c> static <c>FloatSetting</c> with no <c>PlayerPrefs</c> key, so nothing
    /// carries it across a process, and the world's own archive does not hold one either. A default
    /// speed can therefore only be applied from inside the running game, which is what this is.
    ///
    /// <b>Once per world load, and never again.</b> The apply fires on the rising edge of
    /// <see cref="GameBridge.SimulationReady"/> — the same edge the Contract A client arms on — and
    /// that is exactly the instant the game has just written its own x1. It does not run on a timer
    /// and it does not watch the setting afterwards, so the in-game speed slider, the
    /// <c>timescale</c> dev command and everything else that moves the speed keep working and are
    /// never fought. The next world load re-applies it, because the game reset it again.
    ///
    /// <b>What x10 means.</b> It is the game's own *Time Speed* — <c>targetTimeScale</c>, the value
    /// under the speed slider — set to 10, which is precisely what a person dragging that slider to
    /// "X 10.00" does. It is a **target**, not a promise: <c>TimeController.CheckMinFPS</c> is a
    /// servo that trades simulation speed for frame rate at <c>UserSettings.minimumFPS</c> (15), so a
    /// machine that cannot keep up simply runs slower and stays smooth, and <c>Time.maximumDeltaTime</c>
    /// caps the achieved rate at <c>fps/simTPS</c> whatever the target says. Read
    /// <c>targetTimeScale</c> for what was asked and simulated time for what was achieved
    /// (dev_environment.md, *A world can be at the wrong time scale*).
    ///
    /// <b>Env-only, with no BepInEx config entry</b>, following <see cref="MinFpsGovernor"/> and
    /// <see cref="WorldSettings.EnvFertility"/>: a new <c>Config.Bind</c> makes every instance on the
    /// machine rewrite one shared config file at once, and instances lose that race.
    ///
    /// It is deliberately **not** gated on the multiverse client, like the world saver and the
    /// minimum-FPS governor beside it: the reset to x1 is the game's own behaviour, and a world is
    /// worth starting at the speed it was configured for whether or not this instance is wired into
    /// a map.
    /// </summary>
    internal sealed class StartupTimescale : MonoBehaviour
    {
        internal const string Prefix = "[M5-SPEED]";

        /// <summary>
        /// The speed a world starts at, as a number in the game's own slider range. <c>off</c> (or
        /// <c>none</c>) leaves the game's x1 alone. Unset means <see cref="Default"/>.
        /// </summary>
        internal const string EnvironmentVariable = "MULTIVERSE_STARTUP_TIME_SCALE";

        /// <summary>
        /// x10 — what a fresh install starts at. The map's own worlds run in this range, an inbound
        /// migration is paced in *simulated* minutes so a slow world drains its queue slowly, and the
        /// minimum-FPS servo already protects a machine that cannot hold it.
        /// </summary>
        internal const float Default = 10f;

        /// <summary>The game's own range for <c>targetTimeScale</c> (TimeController.cs).</summary>
        private const float Minimum = 0.1f;
        private const float Maximum = 100f;

        /// <summary>The scale to apply, or negative for "leave the game's own x1 alone".</summary>
        private float requested;

        /// <summary>Where <see cref="requested"/> came from, for the log lines.</summary>
        private string source = string.Empty;

        private bool ready;
        private bool broadcastReported;

        private void Awake()
        {
            requested = Read(out source);

            if (requested < 0f)
            {
                MultiversePlugin.Log.LogInfo(
                    $"{Prefix} startup time scale is OFF ({source}) — every world this game loads keeps the game's own " +
                    $"x1 until something asks for more. Set {EnvironmentVariable} to a number between " +
                    $"{Number(Minimum)} and {Number(Maximum)} to start faster.");
                return;
            }

            MultiversePlugin.Log.LogInfo(
                $"{Prefix} startup time scale armed at x{Number(requested)} ({source}). It is applied once per world " +
                "load, at the instant the game has finished resetting the world to x1, and never again: the in-game " +
                "speed slider and the 'timescale' dev command stay in charge for the rest of the session.");
        }

        private void Update()
        {
            try
            {
                bool now = GameBridge.SimulationReady();
                if (now == ready)
                {
                    return;
                }

                ready = now;
                if (ready)
                {
                    Apply("world load");
                }
            }
            catch (Exception e)
            {
                MultiversePlugin.Log.LogError($"{Prefix} the world-load watch threw — {e}");
            }
        }

        /// <summary>
        /// Set the configured startup speed now. Called on every world load, and once more by
        /// <see cref="WorldLoader"/> after it seeds a world — seeding runs at its own fixed speed and
        /// leaves the world live, so without this a first start would keep the seeding speed rather
        /// than the configured one.
        /// </summary>
        internal void Apply(string why)
        {
            if (requested < 0f)
            {
                return;
            }

            // The broadcast director holds a time scale of its own, every poll, for as long as it
            // runs. Writing a different one here would be overwritten within 0.2 s and would put a
            // contradiction in the log; stand down instead and say so once.
            if (SpectatorDirector.OwnsTimeScale())
            {
                if (!broadcastReported)
                {
                    broadcastReported = true;
                    MultiversePlugin.Log.LogInfo(
                        $"{Prefix} not applied: {SpectatorDirector.EnvTimeScale} is set, and the broadcast director " +
                        "holds that speed for as long as it runs. That one wins.");
                }

                return;
            }

            float before = CurrentTarget();
            WorldSeeder.SetTimeScale(requested, Prefix);

            MultiversePlugin.Log.LogInfo(
                $"{Prefix} targetTimeScale x{Number(before)} -> x{Number(requested)} at {why} ({source}). " +
                "The game itself resets every world it loads to x1 (SimulationManager.Start); this is what puts it " +
                "back. Nothing here writes it again until the next world load, so the in-game speed slider and " +
                "'timescale <x>' on the command file both stick. Time.timeScale climbs to it over the next few " +
                "seconds — the game's minimum-FPS servo ramps the applied value and will hold it below the target on " +
                "a machine that cannot draw fast enough, which costs simulation speed and not smoothness.");
        }

        internal static float CurrentTarget()
        {
            try
            {
                return TimeController.targetTimeScale.val;
            }
            catch (Exception)
            {
                return -1f;
            }
        }

        /// <summary>
        /// Reads the variable. Anything it refuses leaves the game's own x1 in place and says so
        /// loudly: a mistyped speed must not silently become some other speed.
        /// </summary>
        private static float Read(out string source)
        {
            string text = (MultiverseConfig.Env(EnvironmentVariable) ?? string.Empty).Trim();

            if (text.Length == 0)
            {
                source = EnvironmentVariable + " unset, so the shipped default";
                return Default;
            }

            source = EnvironmentVariable + "='" + text + "'";

            if (string.Equals(text, "off", StringComparison.OrdinalIgnoreCase)
                || string.Equals(text, "none", StringComparison.OrdinalIgnoreCase))
            {
                return -1f;
            }

            if (!float.TryParse(text, NumberStyles.Float, CultureInfo.InvariantCulture, out float value)
                || float.IsNaN(value)
                || float.IsInfinity(value))
            {
                MultiversePlugin.Log.LogWarning(
                    $"{Prefix} {EnvironmentVariable}='{text}' is not a number — this game's worlds start at the game's " +
                    $"own x1. Write it as a plain decimal between {Number(Minimum)} and {Number(Maximum)}, for example " +
                    $"{Number(Default)}, or 'off' to mean the game's own speed on purpose.");
                return -1f;
            }

            if (value < Minimum || value > Maximum)
            {
                MultiversePlugin.Log.LogWarning(
                    $"{Prefix} {EnvironmentVariable}='{text}' is outside the game's own speed range of " +
                    $"{Number(Minimum)} to {Number(Maximum)} — this game's worlds start at the game's own x1. " +
                    "(Zero would start every world paused, and this knob will not write it: a mistyped variable must " +
                    "not stop a world. Use 'off' to keep the game's own speed on purpose.)");
                return -1f;
            }

            return value;
        }

        /// <summary>Culture-free and short enough to read in a log line.</summary>
        private static string Number(float value)
        {
            return value.ToString("G6", CultureInfo.InvariantCulture);
        }
    }
}
