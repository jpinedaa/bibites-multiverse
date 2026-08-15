using System;
using System.Globalization;
using System.IO;
using ManagementScripts;
using Newtonsoft.Json.Linq;
using SimulationScripts.BibiteScripts;
using UnityEngine;

namespace BibitesMultiverse
{
    /// <summary>
    /// Drives the single shared broadcast camera. It chooses the youngest living Bibite once, follows
    /// that organism until it dies or leaves this world, and only then chooses again. A later birth
    /// does not steal the camera from the current subject.
    ///
    /// This component changes selection and camera state only. It never moves, heals, kills, exports,
    /// or otherwise mutates an organism or the simulation.
    /// </summary>
    internal sealed class SpectatorDirector : MonoBehaviour
    {
        internal const string Prefix = "[BROADCAST]";
        internal const string EnvEnabled = "MULTIVERSE_BROADCAST";
        internal const string EnvZoom = "MULTIVERSE_BROADCAST_ZOOM";
        internal const string EnvReselectDelay = "MULTIVERSE_BROADCAST_RESELECT_DELAY";
        internal const string EnvStatusFile = "MULTIVERSE_BROADCAST_STATUS_FILE";
        internal const string EnvHideUI = "MULTIVERSE_BROADCAST_HIDE_UI";

        private const float DefaultZoom = 35f;
        private const float DefaultReselectDelay = 2f;
        private const float PollSeconds = 0.2f;
        private const float StatusSeconds = 2f;

        private BibiteBody target;
        private float zoom;
        private float reselectDelay;
        private float nextPoll;
        private float nextSelection;
        private float nextStatus;
        private string statusFile;
        private string lastEndReason = "startup";
        private bool hideUI;
        private bool uiApplied;
        private int lastEntityId;
        private string lastSpecies = string.Empty;
        private DateTime selectedAtUtc;

        internal static bool Requested()
        {
            return ReadBool(EnvEnabled, false);
        }

        private void Awake()
        {
            zoom = ReadFloat(EnvZoom, DefaultZoom, 0.1f);
            reselectDelay = ReadFloat(EnvReselectDelay, DefaultReselectDelay, 0f);
            statusFile = (MultiverseConfig.Env(EnvStatusFile) ?? string.Empty).Trim();
            hideUI = ReadBool(EnvHideUI, false);

            MultiversePlugin.Log.LogInfo(
                $"{Prefix} director armed — rule=youngest-until-death-or-departure " +
                $"zoom={zoom.ToString("F1", CultureInfo.InvariantCulture)} " +
                $"reselectDelay={reselectDelay.ToString("F1", CultureInfo.InvariantCulture)}s " +
                $"hideUI={hideUI} statusFile={(statusFile.Length == 0 ? "<off>" : statusFile)}");
        }

        private void Update()
        {
            if (Time.realtimeSinceStartup < nextPoll)
            {
                return;
            }
            nextPoll = Time.realtimeSinceStartup + PollSeconds;

            if (!GameBridge.SimulationReady())
            {
                WriteStatus("waiting_for_world", "simulation not ready");
                return;
            }

            ApplyUIChoice();

            if (target != null && (target.dead || target.dying || target.destroyed))
            {
                EndTarget(target.dead || target.dying ? "death" : "departure");
            }
            else if (target == null && lastEntityId != 0)
            {
                // Unity objects compare equal to null after Destroy. A clean migration follows this
                // path: it is a departure, not a death, and the ecology is left untouched.
                EndTarget("departure");
            }

            if (target == null && Time.realtimeSinceStartup >= nextSelection)
            {
                SelectYoungest(lastEndReason);
            }

            if (target != null)
            {
                EnforceZoom();
                WriteStatus("following", string.Empty);
            }
            else
            {
                WriteStatus("waiting_for_bibite", "no living Bibite is available");
            }
        }

        private void EndTarget(string reason)
        {
            int ended = lastEntityId;
            string species = lastSpecies;
            target = null;
            lastEntityId = 0;
            lastSpecies = string.Empty;
            lastEndReason = reason;
            nextSelection = Time.realtimeSinceStartup + reselectDelay;
            MultiversePlugin.Log.LogInfo(
                $"{Prefix} subject ended — entityId={ended} species='{species}' reason={reason}; " +
                $"reselecting in {reselectDelay.ToString("F1", CultureInfo.InvariantCulture)}s");
        }

        private void SelectYoungest(string reason)
        {
            BibiteTracker tracker = BibiteTracker.instance;
            if (tracker == null || tracker.bibites == null)
            {
                return;
            }

            BibiteBody youngest = null;
            float youngestAge = float.MaxValue;
            int youngestId = int.MaxValue;

            foreach (BibiteBody candidate in tracker.bibites)
            {
                if (!Eligible(candidate))
                {
                    continue;
                }

                float age = candidate.age;
                int id = (candidate.id != null) ? candidate.id.id : int.MaxValue;
                if (youngest == null || age < youngestAge || (Math.Abs(age - youngestAge) < 0.000001f && id < youngestId))
                {
                    youngest = candidate;
                    youngestAge = age;
                    youngestId = id;
                }
            }

            if (youngest == null)
            {
                UserControl.Instance?.SelectBibiteTarget(null);
                return;
            }

            UserControl control = UserControl.Instance;
            if (control == null)
            {
                return;
            }

            control.SelectBibiteTarget(youngest);
            if (control.target != youngest.gameObject)
            {
                MultiversePlugin.Log.LogWarning(
                    $"{Prefix} the game refused selection of entity {youngestId}; selection remains idle");
                return;
            }

            target = youngest;
            lastEntityId = youngestId;
            lastSpecies = SpeciesName(youngest);
            selectedAtUtc = DateTime.UtcNow;
            lastEndReason = string.Empty;
            EnforceZoom();
            nextStatus = 0f;

            MultiversePlugin.Log.LogInfo(
                $"{Prefix} following entityId={lastEntityId} species='{lastSpecies}' " +
                $"ageHours={youngestAge.ToString("F6", CultureInfo.InvariantCulture)} selectedBecause={reason}");
        }

        private static bool Eligible(BibiteBody body)
        {
            return body != null && body.born && !body.dead && !body.dying && !body.destroyed;
        }

        private void EnforceZoom()
        {
            Camera camera = Camera.main;
            if (camera == null)
            {
                return;
            }

            if (Math.Abs(camera.orthographicSize - zoom) < 0.01f)
            {
                return;
            }

            if (CameraManager.instance != null)
            {
                CameraManager.instance.SetCamSize(zoom);
            }
            else
            {
                camera.orthographicSize = zoom;
            }
            CameraManager.OnCameraZoomChange.Invoke();
            CameraManager.onCameraSizeChange.Invoke(zoom);
        }

        private void ApplyUIChoice()
        {
            if (uiApplied || UserControl.Instance == null || UserControl.Instance.mainUI == null)
            {
                return;
            }

            uiApplied = true;
            if (hideUI)
            {
                UserControl.Instance.mainUI.SetActive(false);
            }
        }

        private void WriteStatus(string state, string detail)
        {
            if (statusFile.Length == 0 || Time.realtimeSinceStartup < nextStatus)
            {
                return;
            }
            nextStatus = Time.realtimeSinceStartup + StatusSeconds;

            try
            {
                string directory = Path.GetDirectoryName(statusFile);
                if (!string.IsNullOrEmpty(directory))
                {
                    Directory.CreateDirectory(directory);
                }

                BibiteTracker tracker = BibiteTracker.instance;
                JObject status = new JObject
                {
                    ["state"] = state,
                    ["detail"] = detail,
                    ["world"] = MultiverseConfig.Env(MultiverseConfig.EnvWorld) ?? string.Empty,
                    ["entityId"] = target != null ? lastEntityId : 0,
                    ["species"] = target != null ? lastSpecies : string.Empty,
                    ["ageHours"] = target != null ? target.age : 0f,
                    ["population"] = tracker?.bibites?.Count ?? 0,
                    ["zoom"] = zoom,
                    ["selectedAt"] = target != null ? selectedAtUtc.ToString("O", CultureInfo.InvariantCulture) : string.Empty,
                    ["updatedAt"] = DateTime.UtcNow.ToString("O", CultureInfo.InvariantCulture)
                };

                string temporary = statusFile + ".new";
                File.WriteAllText(temporary, status.ToString());
                if (File.Exists(statusFile))
                {
                    File.Replace(temporary, statusFile, null);
                }
                else
                {
                    File.Move(temporary, statusFile);
                }
            }
            catch (Exception e)
            {
                MultiversePlugin.Log.LogWarning($"{Prefix} could not write {statusFile}: {e.Message}");
            }
        }

        private static string SpeciesName(BibiteBody body)
        {
            try
            {
                return body?.gene?.species?.name ?? string.Empty;
            }
            catch
            {
                return string.Empty;
            }
        }

        private static bool ReadBool(string name, bool fallback)
        {
            string text = (MultiverseConfig.Env(name) ?? string.Empty).Trim().ToLowerInvariant();
            if (text.Length == 0)
            {
                return fallback;
            }
            if (text == "1" || text == "true" || text == "yes" || text == "on")
            {
                return true;
            }
            if (text == "0" || text == "false" || text == "no" || text == "off")
            {
                return false;
            }
            MultiversePlugin.Log.LogWarning($"{Prefix} {name}='{text}' is not a boolean; using {fallback}");
            return fallback;
        }

        private static float ReadFloat(string name, float fallback, float minimum)
        {
            string text = (MultiverseConfig.Env(name) ?? string.Empty).Trim();
            if (text.Length == 0)
            {
                return fallback;
            }
            if (float.TryParse(text, NumberStyles.Float, CultureInfo.InvariantCulture, out float value) && value >= minimum)
            {
                return value;
            }
            MultiversePlugin.Log.LogWarning(
                $"{Prefix} {name}='{text}' is not a number >= {minimum.ToString(CultureInfo.InvariantCulture)}; " +
                $"using {fallback.ToString(CultureInfo.InvariantCulture)}");
            return fallback;
        }
    }
}
