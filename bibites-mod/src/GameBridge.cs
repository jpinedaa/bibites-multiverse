using System;
using System.Reflection;
using System.Runtime.ExceptionServices;
using HarmonyLib;
using ManagementScripts;
using Newtonsoft.Json.Linq;
using SimulationScripts.BibiteScripts;
using UnityEngine;

namespace BibitesMultiverse
{
    /// <summary>
    /// Thin access layer over the game's own save/restore path.
    /// Everything here mirrors decompiled/BibitesAssembly/ManagementScripts/SaveSystem.cs.
    /// </summary>
    internal static class GameBridge
    {
        private static MethodInfo serializeBibite;
        private static MethodInfo loadBibite;

        /// <summary>SaveSystem.SerializeBibite(GameObject) — private static JObject (SaveSystem.cs:406).</summary>
        internal static JObject SerializeBibite(GameObject bibite)
        {
            if (serializeBibite == null)
            {
                serializeBibite = AccessTools.Method(typeof(SaveSystem), "SerializeBibite");
                if (serializeBibite == null)
                {
                    throw new MissingMethodException("SaveSystem.SerializeBibite(GameObject) not found — the game API changed.");
                }
            }

            return (JObject)Invoke(serializeBibite, null, new object[] { bibite });
        }

        /// <summary>
        /// SaveSystem.LoadBibite(JObject, bool, DialogGroupHandle, Utility.Version, float?) — private
        /// instance method (SaveSystem.cs:438). The public LoadBibiteOrEggFromData wrapper swallows
        /// every exception (SaveSystem.cs:76-79); this call lets the real failure reach the log.
        /// </summary>
        internal static GameObject LoadBibiteDirect(JObject state)
        {
            if (loadBibite == null)
            {
                loadBibite = AccessTools.Method(typeof(SaveSystem), "LoadBibite");
                if (loadBibite == null)
                {
                    throw new MissingMethodException("SaveSystem.LoadBibite(...) not found — the game API changed.");
                }
            }

            SaveSystem save = SaveSystem.instance;
            if (save == null)
            {
                throw new InvalidOperationException("SaveSystem.instance is null — no simulation scene is loaded.");
            }

            // birthAtMaturity MUST stay null: a value routes to StartBodyAtGrowthAndNormalize,
            // which resets health/energy and draws three Random.Range values (BibiteBody.cs:451-456).
            object[] args = { state, true, null, Utility.Version.Present, null };
            return (GameObject)Invoke(loadBibite, save, args);
        }

        private static object Invoke(MethodInfo method, object target, object[] args)
        {
            try
            {
                return method.Invoke(target, args);
            }
            catch (TargetInvocationException e) when (e.InnerException != null)
            {
                ExceptionDispatchInfo.Capture(e.InnerException).Throw();
                throw; // unreachable; keeps the compiler happy
            }
        }

        /// <summary>
        /// The organism the round trip should act on: the game's current selection when it has one
        /// (UserControl.cs:35-37), otherwise the first live entry of BibiteTracker.instance.bibites.
        /// </summary>
        internal static BibiteBody PickTarget(out string source)
        {
            UserControl control = UserControl.Instance;
            if (control != null)
            {
                BibiteBody selected = control.bibiteTarget;
                if (IsUsable(selected))
                {
                    source = "UserControl.Instance.bibiteTarget (current selection)";
                    return selected;
                }

                if (control.target != null)
                {
                    BibiteBody fromTarget = control.target.GetComponent<BibiteBody>();
                    if (IsUsable(fromTarget))
                    {
                        source = "UserControl.Instance.target (current selection)";
                        return fromTarget;
                    }
                }
            }

            BibiteTracker tracker = BibiteTracker.instance;
            if (tracker != null && tracker.bibites != null)
            {
                foreach (BibiteBody body in tracker.bibites)
                {
                    if (IsUsable(body) && !body.dead && !body.dying)
                    {
                        source = "BibiteTracker.instance.bibites (first living entry)";
                        return body;
                    }
                }
            }

            source = "none";
            return null;
        }

        internal static bool IsUsable(BibiteBody body)
        {
            return body != null && !body.destroyed;
        }

        /// <summary>Linear scan by entity ID, the same lookup LoadGame uses (SaveSystem.cs:754-782).</summary>
        internal static BibiteBody FindBodyById(int id)
        {
            if (id == 0)
            {
                return null;
            }

            BibiteTracker tracker = BibiteTracker.instance;
            if (tracker == null || tracker.bibites == null)
            {
                return null;
            }

            foreach (BibiteBody body in tracker.bibites)
            {
                if (body != null && body.id != null && body.id.id == id)
                {
                    return body;
                }
            }

            return null;
        }

        /// <summary>Bibite or egg BibiteID by entity ID — eggs can appear in a parent's children list.</summary>
        internal static BibiteID FindIdById(int id)
        {
            BibiteBody body = FindBodyById(id);
            if (body != null && body.id != null)
            {
                return body.id;
            }

            BibiteTracker tracker = BibiteTracker.instance;
            if (tracker == null || tracker.eggs == null)
            {
                return null;
            }

            foreach (EggHatching egg in tracker.eggs)
            {
                if (egg != null && egg.id != null && egg.id.id == id)
                {
                    return egg.id;
                }
            }

            return null;
        }
    }
}
