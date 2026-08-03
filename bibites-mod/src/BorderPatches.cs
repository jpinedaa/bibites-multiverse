using System;
using System.Reflection;
using HarmonyLib;
using SimulationScripts.BibiteScripts;

namespace BibitesMultiverse
{
    /// <summary>
    /// The border-detection anchor: a Harmony postfix on <c>BibiteBody.FixedUpdate()</c>
    /// (m2_findings.md §(b), m1_findings.md §8). That method is the real per-organism tick, and it is
    /// private, so it is reached through <c>AccessTools.Method</c>.
    ///
    /// A postfix runs on every return path, including the early ones at BibiteBody.cs:581-589, so
    /// every guard those returns imply is re-applied inside
    /// <see cref="MultiverseClient.OnBodyTick"/> rather than assumed here.
    ///
    /// Ordering inside a tick: BibitePropulsion.UpdateOrgan runs at BibiteBody.cs:613-616, before this
    /// postfix. The organism is therefore allowed to steer first and tested for the border second,
    /// which is the correct order.
    /// </summary>
    internal static class BorderPatches
    {
        private static Harmony harmony;

        internal static bool Apply(string id)
        {
            if (harmony != null)
            {
                return true;
            }

            try
            {
                MethodInfo target = AccessTools.Method(typeof(BibiteBody), "FixedUpdate");
                if (target == null)
                {
                    MultiversePlugin.Log.LogError("[M2] BibiteBody.FixedUpdate not found — the border strip cannot be armed. The game API changed.");
                    return false;
                }

                MethodInfo postfix = typeof(BorderPatches).GetMethod(
                    nameof(BibiteBodyFixedUpdatePostfix),
                    BindingFlags.Static | BindingFlags.NonPublic);

                harmony = new Harmony(id);
                harmony.Patch(target, postfix: new HarmonyMethod(postfix));
                MultiversePlugin.Log.LogInfo("[M2] border strip armed — postfix on BibiteBody.FixedUpdate.");
                return true;
            }
            catch (Exception e)
            {
                harmony = null;
                MultiversePlugin.Log.LogError($"[M2] could not patch BibiteBody.FixedUpdate — {e}");
                return false;
            }
        }

        private static void BibiteBodyFixedUpdatePostfix(BibiteBody __instance)
        {
            MultiverseClient client = MultiverseClient.Active;
            if (client == null)
            {
                return;
            }

            try
            {
                client.OnBodyTick(__instance);
            }
            catch (Exception e)
            {
                // A throw here would land inside the game's own per-organism tick. Never let it out.
                MultiversePlugin.Log.LogError($"[M2] border check threw — {e}");
            }
        }
    }
}
