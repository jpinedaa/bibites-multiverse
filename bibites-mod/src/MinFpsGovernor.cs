using System;
using System.Reflection;
using HarmonyLib;
using ManagementScripts;
using SettingScripts;
using UnityEngine;
using UnityEngine.Rendering;

namespace BibitesMultiverse
{
    /// <summary>
    /// Disarms the game's minimum-FPS governor in a world nobody is looking at.
    ///
    /// <c>TimeController.CheckMinFPS</c> is a servo, not a floor. Every <c>TimeKeeper</c> update it
    /// runs <c>engineTimeScale = clamp(engineTimeScale * fps / minimumFPS, min(0.1, target), target)</c>,
    /// so it drives the engine scale toward whatever value keeps the frame rate at
    /// <c>UserSettings.minimumFPS</c> — default <b>15</b>. It exists to keep a window smooth for a
    /// person, and it is the reason a headless world reports a fluctuating non-integer instead of the
    /// speed it was asked for.
    ///
    /// <b>It still runs headless.</b> The gate is
    /// <c>Application.isFocused || !UserSettings.ignoreMinOnUnfocus.val</c>, and
    /// <c>ignoreMinOnUnfocus</c> ships <c>false</c>, so the second term is <c>true</c> and an unfocused
    /// — or windowless — process is governed exactly like a visible one. Measured on the living
    /// deployment 2026-08-10: five headless worlds asked for x100 and reported 11-81.
    ///
    /// <b>Why a patch and not the setting.</b> Writing <c>UserSettings.minimumFPS</c> would work — the
    /// stock code takes a "no minimum" branch at <c>&lt; 1</c> — but <c>IntUserSetting.val</c> is backed
    /// by <c>PlayerPrefs</c>, which is one store shared by every instance on the machine and persisted
    /// across runs. A headless rig would silently change the owner's own setting for their next drawn
    /// session. This prefix reproduces the <c>minimumFPS = 0</c> behaviour exactly and stores nothing.
    ///
    /// <b>What it does not buy.</b> <c>UpdateTimeScale</c> also pins
    /// <c>Time.maximumDeltaTime = Time.fixedDeltaTime = 1 / simTPS</c>, so one frame advances at most
    /// one simulation tick and the achieved rate cannot exceed <c>fps / simTPS</c> whatever the target
    /// says. Removing the governor removes a clamp; it does not remove that ceiling. See
    /// dev_environment.md, *A world can be at the wrong time scale*.
    /// </summary>
    internal static class MinFpsGovernor
    {
        internal const string Prefix = "[M4-SPEED]";

        /// <summary>Opt out, or force the behaviour on a drawn instance. Values: <c>auto</c> (default),
        /// <c>off</c> to leave the game's governor alone, <c>0</c> to disarm it always.</summary>
        internal const string EnvironmentVariable = "MULTIVERSE_MIN_FPS";

        private static Harmony harmony;

        internal static bool Disarmed { get; private set; }

        /// <summary>True when this process has no graphics device to keep smooth.</summary>
        internal static bool IsHeadless()
        {
            try
            {
                return Application.isBatchMode || SystemInfo.graphicsDeviceType == GraphicsDeviceType.Null;
            }
            catch (Exception e)
            {
                MultiversePlugin.Log.LogWarning($"{Prefix} could not tell whether this process is headless: {e.Message}");
                return false;
            }
        }

        internal static void Apply(string id)
        {
            if (harmony != null)
            {
                return;
            }

            bool headless = IsHeadless();
            string requested = MultiverseConfig.Env(EnvironmentVariable);
            string mode = string.IsNullOrEmpty(requested) ? "auto" : requested.Trim().ToLowerInvariant();

            bool disarm;
            switch (mode)
            {
                case "auto":
                    disarm = headless;
                    break;
                case "off":
                    disarm = false;
                    break;
                case "0":
                    disarm = true;
                    break;
                default:
                    MultiversePlugin.Log.LogWarning(
                        $"{Prefix} {EnvironmentVariable}='{requested}' is not auto, off or 0 — using auto. " +
                        "This knob only ever turns the governor off; it cannot set a different minimum.");
                    disarm = headless;
                    break;
            }

            if (!disarm)
            {
                MultiversePlugin.Log.LogInfo(
                    $"{Prefix} the game's minimum-FPS governor stays armed " +
                    $"(headless={headless} {EnvironmentVariable}={mode} minimumFPS={ReadMinimumFps()}).");
                return;
            }

            try
            {
                MethodInfo target = AccessTools.Method(typeof(TimeController), nameof(TimeController.CheckMinFPS));
                if (target == null)
                {
                    MultiversePlugin.Log.LogError(
                        $"{Prefix} TimeController.CheckMinFPS not found — the governor stays armed and this " +
                        "world will not hold its target time scale. The game API changed.");
                    return;
                }

                MethodInfo prefix = typeof(MinFpsGovernor).GetMethod(
                    nameof(CheckMinFpsPrefix),
                    BindingFlags.Static | BindingFlags.NonPublic);

                harmony = new Harmony(id + ".minfps");
                harmony.Patch(target, prefix: new HarmonyMethod(prefix));
                Disarmed = true;

                MultiversePlugin.Log.LogInfo(
                    $"{Prefix} minimum-FPS governor DISARMED (headless={headless} {EnvironmentVariable}={mode}, " +
                    $"the setting itself is untouched at {ReadMinimumFps()}). This world will apply its target " +
                    "time scale verbatim. The achieved rate is still capped at fps/simTPS by " +
                    "Time.maximumDeltaTime — read simulatedTime, not timeScale.");
            }
            catch (Exception e)
            {
                harmony = null;
                MultiversePlugin.Log.LogError($"{Prefix} could not disarm the minimum-FPS governor: {e}");
            }
        }

        private static int ReadMinimumFps()
        {
            try
            {
                return UserSettings.minimumFPS.val;
            }
            catch (Exception)
            {
                return -1;
            }
        }

        /// <summary>
        /// Stands in for <c>CheckMinFPS</c>. Stock code with <c>minimumFPS &lt; 1</c> never throttles and
        /// lets <c>SetTarget</c> apply the target directly; the same two rules are reproduced here,
        /// because <c>SetTarget</c> routes through this method whenever the stored minimum is >= 1 and
        /// skipping outright would leave the engine scale stranded below the target forever.
        /// </summary>
        private static bool CheckMinFpsPrefix()
        {
            try
            {
                // Stock CheckMinFPS does nothing while paused, and forcing the target here would
                // resume a world somebody deliberately stopped.
                if (TimeController.paused)
                {
                    return false;
                }

                float target = TimeController.targetTimeScale.val;
                if (!Mathf.Approximately(TimeController.engineTimeScale.val, target))
                {
                    TimeController.engineTimeScale.SetValue(target);
                }
            }
            catch (Exception e)
            {
                MultiversePlugin.Log.LogError($"{Prefix} the disarmed governor threw — {e}");
            }

            return false;
        }
    }
}
