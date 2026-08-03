using System;
using System.Collections;
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
        private static MethodInfo createSave;

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

        /// <summary>
        /// SaveSystem.CreateSave(string) — the private iterator behind the public SaveGame
        /// (SaveSystem.cs:176). SaveGame hands it to StartCoroutine, where an exception is swallowed
        /// by Unity's coroutine runner; returning the raw enumerator lets the caller drive it and see
        /// the failure — which is the whole point of the stale-reference check (m1_findings.md §4.3).
        /// </summary>
        internal static IEnumerator CreateSaveEnumerator(string path)
        {
            if (createSave == null)
            {
                createSave = AccessTools.Method(typeof(SaveSystem), "CreateSave");
                if (createSave == null)
                {
                    throw new MissingMethodException("SaveSystem.CreateSave(string) not found — the game API changed.");
                }
            }

            SaveSystem save = SaveSystem.instance;
            if (save == null)
            {
                throw new InvalidOperationException("SaveSystem.instance is null — no simulation scene is loaded.");
            }

            return (IEnumerator)Invoke(createSave, save, new object[] { path });
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

        /// <summary>
        /// The clean-removal recipe of m1_findings.md §5.2: de-list from BibiteTracker **first**, so
        /// OnDestroy -> BibiteDeath leaves nDeath and the death-age quantiles untouched, then a raw
        /// UnityEngine.Object.Destroy — no corpse, no meat pile, no stomach pellets, no eggs laid.
        /// Never Die(). Precedent: UIScripts/SelectionPanel.cs:310.
        ///
        /// Destruction is deferred to the end of the frame: the caller must not assume the object is
        /// gone in the same frame.
        /// </summary>
        internal static bool DestroyCleanly(BibiteBody body)
        {
            if (body == null)
            {
                return false;
            }

            bool delisted = Untrack(body);
            UnityEngine.Object.Destroy(body.gameObject);
            return delisted;
        }

        /// <summary>Remove a body from BibiteTracker.instance.bibites. True when it was listed.</summary>
        internal static bool Untrack(BibiteBody body)
        {
            BibiteTracker tracker = BibiteTracker.instance;
            if (tracker == null || tracker.bibites == null || body == null)
            {
                return false;
            }

            return tracker.bibites.Remove(body);
        }

        /// <summary>
        /// Put a body back in the tracker. BibiteTracker.TrackBibite has no duplicate guard
        /// (m1_findings.md §4.2), so this checks first.
        /// </summary>
        internal static bool Track(BibiteBody body)
        {
            BibiteTracker tracker = BibiteTracker.instance;
            if (tracker == null || tracker.bibites == null || body == null)
            {
                return false;
            }

            if (tracker.bibites.Contains(body))
            {
                return false;
            }

            tracker.bibites.Add(body);
            return true;
        }

        /// <summary>
        /// One organism's payload: SaveSystem.SerializeBibite plus the "version" key SaveBibite adds
        /// (SaveSystem.cs:147). This is the opaque bb8 blob of contract-a.md §4.6 — the mod produces
        /// it and never parses it beyond the eight position numbers.
        /// </summary>
        internal static JObject StampVersion(JObject payload, string description = null)
        {
            JObject stamped = (JObject)payload.DeepClone();
            stamped["version"] = Application.version;
            if (!string.IsNullOrEmpty(description))
            {
                stamped["desc"] = description;
            }

            return stamped;
        }

        internal static int LivingPopulation()
        {
            BibiteTracker tracker = BibiteTracker.instance;
            if (tracker == null || tracker.bibites == null)
            {
                return 0;
            }

            int count = 0;
            foreach (BibiteBody body in tracker.bibites)
            {
                if (body != null)
                {
                    count++;
                }
            }

            return count;
        }

        internal static int EggPopulation()
        {
            BibiteTracker tracker = BibiteTracker.instance;
            if (tracker == null || tracker.eggs == null)
            {
                return 0;
            }

            int count = 0;
            foreach (EggHatching egg in tracker.eggs)
            {
                if (egg != null)
                {
                    count++;
                }
            }

            return count;
        }

        /// <summary>
        /// One definition of "a world is loaded and an organism can be spawned into it".
        ///
        /// The ProceduralSpriteManager check is not cosmetic: BibiteBody.FinalizeBirth calls
        /// RequestBodySprite, which indexes the sprite lists directly, so restoring an organism before
        /// the atlas is built throws IndexOutOfRange inside LoadBibite. That exception is swallowed by
        /// LoadBibiteOrEggFromData, so without this gate a transient condition would be reported as a
        /// permanent DESERIALIZE_FAILED.
        /// </summary>
        internal static bool SimulationReady()
        {
            return GameManager.isSim
                && SimulationManager.Instance != null
                && SaveSystem.instance != null
                && SaveController.Instance != null
                && TimeController.Instance != null
                && BibiteTracker.instance != null
                && BibiteTracker.instance.bibites != null
                && WorldObjectsSpawner.Instance != null
                && ProceduralSpriteManager.Instance != null
                && ProceduralSpriteManager.Instance.done;
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
