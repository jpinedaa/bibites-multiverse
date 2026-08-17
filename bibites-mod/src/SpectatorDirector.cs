using System;
using System.Collections.Generic;
using System.Globalization;
using System.IO;
using ManagementScripts;
using Newtonsoft.Json.Linq;
using SettingScripts;
using SimulationScripts.BibiteScripts;
using UIScripts.UIPanels;
using UnityEngine;

namespace BibitesMultiverse
{
    /// <summary>
    /// Drives the single shared broadcast camera. It chooses the youngest living Bibite once, follows
    /// that organism until it dies or leaves this world, and only then chooses again. A later birth
    /// does not steal the camera from the current subject.
    ///
    /// This component changes spectator presentation and can set the requested simulation speed. It
    /// never moves, heals, kills, exports, or otherwise edits an organism.
    /// </summary>
    internal sealed class SpectatorDirector : MonoBehaviour
    {
        internal const string Prefix = "[BROADCAST]";
        internal const string EnvEnabled = "MULTIVERSE_BROADCAST";
        internal const string EnvZoom = "MULTIVERSE_BROADCAST_ZOOM";
        internal const string EnvReselectDelay = "MULTIVERSE_BROADCAST_RESELECT_DELAY";
        internal const string EnvStatusFile = "MULTIVERSE_BROADCAST_STATUS_FILE";
        internal const string EnvHideUI = "MULTIVERSE_BROADCAST_HIDE_UI";
        internal const string EnvTimeScale = "MULTIVERSE_BROADCAST_TIME_SCALE";
        internal const string EnvPanels = "MULTIVERSE_BROADCAST_PANELS";
        internal const string EnvPanelSeconds = "MULTIVERSE_BROADCAST_PANEL_SECONDS";
        internal const string EnvShowFieldOfView = "MULTIVERSE_BROADCAST_SHOW_FOV";
        internal const string EnvDisableSpawnTemplates = "MULTIVERSE_BROADCAST_DISABLE_SPAWN_TEMPLATES";

        private const float DefaultZoom = 35f;
        private const float DefaultReselectDelay = 2f;
        private const float DefaultPanelSeconds = 15f;
        private const float PollSeconds = 0.2f;
        private const float StatusSeconds = 2f;

        private readonly List<BibitePanels> panelRotation = new List<BibitePanels>();
        private readonly HashSet<string> disabledSpawnTemplates =
            new HashSet<string>(StringComparer.OrdinalIgnoreCase);
        private BibiteBody target;
        private float zoom;
        private float reselectDelay;
        private float broadcastTimeScale;
        private float panelSeconds;
        private float nextPoll;
        private float nextSelection;
        private float nextStatus;
        private float nextPanel;
        private string statusFile;
        private string lastEndReason = "startup";
        private string activePanel = string.Empty;
        private bool hideUI;
        private bool showFieldOfView;
        private bool spawnRuleReported;
        private int panelIndex;
        private int disabledSpawnSettings;
        private int lastEntityId;
        private string lastSpecies = string.Empty;
        private DateTime selectedAtUtc;

        internal static bool Requested()
        {
            return ReadBool(EnvEnabled, false);
        }

        /// <summary>
        /// True when this process is configured to hold a broadcast time scale — the director then
        /// re-asserts it on every poll, so nothing else can own the speed. Read without
        /// <see cref="ReadFloat"/> on purpose: this is a question, and asking it must not log a
        /// second warning about a value the director itself already reported.
        /// </summary>
        internal static bool OwnsTimeScale()
        {
            if (!Requested())
            {
                return false;
            }

            string text = (MultiverseConfig.Env(EnvTimeScale) ?? string.Empty).Trim();
            return text.Length > 0
                && float.TryParse(text, NumberStyles.Float, CultureInfo.InvariantCulture, out float value)
                && value >= 0f;
        }

        private void Awake()
        {
            zoom = ReadFloat(EnvZoom, DefaultZoom, 0.1f);
            reselectDelay = ReadFloat(EnvReselectDelay, DefaultReselectDelay, 0f);
            statusFile = (MultiverseConfig.Env(EnvStatusFile) ?? string.Empty).Trim();
            hideUI = ReadBool(EnvHideUI, false);
            broadcastTimeScale = ReadFloat(EnvTimeScale, -1f, 0f);
            panelSeconds = ReadFloat(EnvPanelSeconds, DefaultPanelSeconds, 2f);
            showFieldOfView = ReadBool(EnvShowFieldOfView, true);
            ReadPanels(MultiverseConfig.Env(EnvPanels));
            ReadSpawnTemplates(MultiverseConfig.Env(EnvDisableSpawnTemplates));

            if (hideUI && panelRotation.Count > 0)
            {
                MultiversePlugin.Log.LogWarning(
                    $"{Prefix} {EnvHideUI}=true disables the configured panel rotation");
            }

            MultiversePlugin.Log.LogInfo(
                $"{Prefix} director armed — rule=youngest-until-death-or-departure " +
                $"zoom={zoom.ToString("F1", CultureInfo.InvariantCulture)} " +
                $"reselectDelay={reselectDelay.ToString("F1", CultureInfo.InvariantCulture)}s " +
                $"timeScale={(broadcastTimeScale < 0f ? "<unchanged>" : broadcastTimeScale.ToString("F2", CultureInfo.InvariantCulture))} " +
                $"panels={PanelList()} panelSeconds={panelSeconds.ToString("F1", CultureInfo.InvariantCulture)} " +
                $"showFOV={showFieldOfView} hideUI={hideUI} " +
                $"disabledSpawnTemplates={SpawnTemplateList()} " +
                $"statusFile={(statusFile.Length == 0 ? "<off>" : statusFile)}");
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
                spawnRuleReported = false;
                disabledSpawnSettings = 0;
                WriteStatus("waiting_for_world", "simulation not ready");
                return;
            }

            ApplyUIChoice();
            DisableTemplateSpawning();
            EnforceTimeScale();

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
                EnforcePanel();
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
            activePanel = string.Empty;
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
            panelIndex = 0;
            nextPanel = 0f;
            activePanel = string.Empty;
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
            UserControl control = UserControl.Instance;
            if (control != null && control.mainUI != null && control.mainUI.activeSelf == hideUI)
            {
                control.mainUI.SetActive(!hideUI);
            }

            if (UserSettings.ShowBibiteFOW.val != showFieldOfView)
            {
                UserSettings.ShowBibiteFOW.SetValue(showFieldOfView);
            }
        }

        private void EnforceTimeScale()
        {
            if (broadcastTimeScale < 0f || TimeController.Instance == null)
            {
                return;
            }

            if (!Mathf.Approximately(TimeController.targetTimeScale.val, broadcastTimeScale))
            {
                WorldSeeder.SetTimeScale(broadcastTimeScale, Prefix);
            }
        }

        private void DisableTemplateSpawning()
        {
            if (disabledSpawnTemplates.Count == 0)
            {
                disabledSpawnSettings = 0;
                return;
            }

            ScenarioSettings scenario = ScenarioSettings.Instance;
            if (scenario == null || scenario.bibites == null)
            {
                return;
            }

            int matched = 0;
            int changed = 0;
            foreach (BibiteSettings settings in scenario.bibites)
            {
                if (settings == null || !SpawnTemplateDisabled(settings))
                {
                    continue;
                }

                matched++;
                if (settings.minimumNumber.val != 0)
                {
                    settings.minimumNumber.SetValue(0);
                    changed++;
                }
            }

            disabledSpawnSettings = matched;
            if (!spawnRuleReported || changed > 0)
            {
                spawnRuleReported = true;
                if (matched == 0)
                {
                    MultiversePlugin.Log.LogWarning(
                        $"{Prefix} no configured spawn template matched {SpawnTemplateList()}");
                }
                else
                {
                    MultiversePlugin.Log.LogInfo(
                        $"{Prefix} automatic template spawning disabled for {SpawnTemplateList()} " +
                        $"(matched={matched} changed={changed}); existing Bibites and reproduction are unchanged");
                }
            }
        }

        private void EnforcePanel()
        {
            if (hideUI || panelRotation.Count == 0 || Time.realtimeSinceStartup < nextPanel)
            {
                return;
            }

            GUIManager manager = GUIManager.Instance;
            if (manager == null || !manager.hasTarget)
            {
                return;
            }

            BibitePanels panel = panelRotation[panelIndex];
            try
            {
                manager.OpenBibitePanel(panel, false);
                activePanel = PanelName(panel);
                panelIndex = (panelIndex + 1) % panelRotation.Count;
                nextPanel = Time.realtimeSinceStartup + panelSeconds;
                MultiversePlugin.Log.LogInfo(
                    $"{Prefix} showing {activePanel} panel for entityId={lastEntityId}");
            }
            catch (Exception e)
            {
                nextPanel = Time.realtimeSinceStartup + 1f;
                MultiversePlugin.Log.LogWarning($"{Prefix} could not show the {PanelName(panel)} panel: {e.Message}");
            }
        }

        /// <summary>
        /// The world's Global Fertility as it is running right now, in E/u²s. It is not the
        /// director's setting — <see cref="WorldSettings"/> applies <c>MULTIVERSE_FERTILITY</c> at
        /// world load — but it belongs on the same receipt as the zoom and the time scale, because
        /// it is part of the same broadcast profile and an operator checks the profile in one place.
        /// </summary>
        private static float ReadFertility()
        {
            try
            {
                return ScenarioIndependentSettings.Instance.pelletGrowth.val;
            }
            catch (Exception)
            {
                return 0f;
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
                    ["panel"] = activePanel,
                    ["fieldOfView"] = showFieldOfView,
                    ["targetTimeScale"] = broadcastTimeScale,
                    ["engineTimeScale"] = TimeController.engineTimeScale.val,
                    ["fertility"] = ReadFertility(),
                    ["disabledSpawnTemplates"] = SpawnTemplateList(),
                    ["disabledSpawnSettings"] = disabledSpawnSettings,
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

        private void ReadPanels(string value)
        {
            string text = (value ?? string.Empty).Trim();
            if (text.Length == 0)
            {
                return;
            }

            foreach (string entry in text.Split(','))
            {
                string name = entry.Trim().ToLowerInvariant().Replace('_', '-');
                BibitePanels panel;
                switch (name)
                {
                    case "brain":
                        panel = BibitePanels.BrainPanel;
                        break;
                    case "biology":
                    case "body":
                        panel = BibitePanels.BiologyPanel;
                        break;
                    case "expanded-brain":
                        panel = BibitePanels.ExpendedBrainPanel;
                        break;
                    default:
                        MultiversePlugin.Log.LogWarning(
                            $"{Prefix} {EnvPanels} contains unsupported panel '{entry.Trim()}'; ignoring it");
                        continue;
                }

                panelRotation.Add(panel);
            }
        }

        private void ReadSpawnTemplates(string value)
        {
            foreach (string entry in (value ?? string.Empty).Split(','))
            {
                string name = NormalizeTemplateName(entry);
                if (name.Length > 0)
                {
                    disabledSpawnTemplates.Add(name);
                }
            }
        }

        private bool SpawnTemplateDisabled(BibiteSettings settings)
        {
            string fileName = NormalizeTemplateName(settings.filePath);
            string templateName = NormalizeTemplateName(settings.templateName);
            return disabledSpawnTemplates.Contains(fileName) || disabledSpawnTemplates.Contains(templateName);
        }

        private static string NormalizeTemplateName(string value)
        {
            string text = (value ?? string.Empty).Trim();
            return text.Length == 0 ? string.Empty : Path.GetFileNameWithoutExtension(text).Trim();
        }

        private string PanelList()
        {
            if (panelRotation.Count == 0)
            {
                return "<off>";
            }

            string[] names = new string[panelRotation.Count];
            for (int index = 0; index < panelRotation.Count; index++)
            {
                names[index] = PanelName(panelRotation[index]);
            }
            return string.Join(",", names);
        }

        private string SpawnTemplateList()
        {
            return disabledSpawnTemplates.Count == 0
                ? "<off>"
                : string.Join(",", disabledSpawnTemplates);
        }

        private static string PanelName(BibitePanels panel)
        {
            switch (panel)
            {
                case BibitePanels.BrainPanel:
                    return "brain";
                case BibitePanels.BiologyPanel:
                    return "biology";
                case BibitePanels.ExpendedBrainPanel:
                    return "expanded-brain";
                default:
                    return panel.ToString();
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
