using System;
using System.Collections;
using System.IO;
using ManagementScripts;
using UnityEngine;
using UnityEngine.SceneManagement;

namespace BibitesMultiverse
{
    /// <summary>
    /// MULTIVERSE_WORLD=&lt;save name&gt; loads that world from the main menu with no operator, through
    /// <c>GameManager.StartGame(path)</c> — the same call the Load Game panel makes
    /// (UIScripts/LoadGamePanel.cs:225), proven by the M1 auto-test.
    ///
    /// The M2 rig needs it: two game instances have to reach their own world unattended, and the mod
    /// only dials its sidecar while a world is loaded (contract-a.md §2). It touches no save other
    /// than the one it is told to load, and does nothing when the variable is unset.
    /// </summary>
    internal class WorldLoader : MonoBehaviour
    {
        private const float MenuTimeout = 240f;
        private const float LoadTimeout = 300f;

        internal string SaveName;

        private void Start()
        {
            StartCoroutine(LoadWorld());
        }

        private IEnumerator LoadWorld()
        {
            string path = Path.Combine(SaveController.SavePath, SaveName + ".zip");
            MultiversePlugin.Log.LogInfo($"[M2] {MultiverseConfig.EnvWorld}={SaveName} — waiting for the main menu to load '{path}'.");

            if (!File.Exists(path))
            {
                MultiversePlugin.Log.LogError($"[M2] {MultiverseConfig.EnvWorld}={SaveName}: no save at {path}. Nothing is loaded.");
                yield break;
            }

            float deadline = Time.realtimeSinceStartup + MenuTimeout;
            while (!MenuReady())
            {
                if (Time.realtimeSinceStartup > deadline)
                {
                    MultiversePlugin.Log.LogError($"[M2] the main menu did not become ready within {MenuTimeout:F0} s — '{SaveName}' was not loaded.");
                    yield break;
                }

                yield return null;
            }

            try
            {
                GameManager.StartGame(path);
            }
            catch (Exception e)
            {
                MultiversePlugin.Log.LogError($"[M2] GameManager.StartGame('{path}') threw — {e}");
                yield break;
            }

            deadline = Time.realtimeSinceStartup + LoadTimeout;
            while (!GameBridge.SimulationReady())
            {
                if (Time.realtimeSinceStartup > deadline)
                {
                    MultiversePlugin.Log.LogError($"[M2] '{SaveName}' did not finish loading within {LoadTimeout:F0} s.");
                    yield break;
                }

                yield return null;
            }

            MultiversePlugin.Log.LogInfo($"[M2] world '{SaveName}' loaded — population={GameBridge.LivingPopulation()}.");
        }

        /// <summary>
        /// The sprite atlas gate matters as much as the scene: MenuInitializer.cs:102 is the only
        /// caller of ProceduralSpriteManager.StartLoadingSprites, and until it has finished every
        /// bibite spawn throws out of BibiteBody.FinalizeBirth -> RequestBodySprite. Loading a world
        /// too early therefore produces an empty world and a restore that returns null.
        /// </summary>
        private static bool MenuReady()
        {
            return SceneManager.GetActiveScene().name == "Menu"
                && GameManager.activeScene == BibiteScenes.Menu
                && ProceduralSpriteManager.Instance != null
                && ProceduralSpriteManager.Instance.done;
        }
    }
}
