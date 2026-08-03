using System;
using System.Collections;
using System.Globalization;
using System.IO;
using System.Text;
using ManagementScripts;
using SimulationScripts.BibiteScripts;
using UnityEngine;

namespace BibitesMultiverse
{
    /// <summary>
    /// The M2 rig's remote control: a one-line-at-a-time command file the orchestration script writes
    /// from WSL and the game polls from Windows.
    ///
    /// It exists because the exit test must not depend on ecology. A natural border crossing may be
    /// rare (m2_considerations.md Risk 1), and the kill test needs a migration to start at an exactly
    /// known instant, twice, in two different processes. A hotkey cannot do that unattended and an
    /// environment variable cannot do it twice.
    ///
    /// **The migration pipeline itself is untouched.** <c>export</c> only teleports an organism into
    /// the border strip with an outward velocity, exactly as if it had swum there; the Harmony postfix
    /// on <c>BibiteBody.FixedUpdate</c> then runs the ordinary <c>MIGRATE_OUT</c> path with all of its
    /// guards. There is no bypass.
    ///
    /// Protocol. The script writes one command per file, atomically (write a temp file, rename over
    /// the target), as <c>&lt;token&gt; &lt;verb&gt; [args]</c> plus a trailing newline. A file whose
    /// content does not end in a newline is a partial write and is left for the next poll. Every
    /// finished command appends one line to <c>&lt;cmdfile&gt;.log</c> — <c>&lt;token&gt; OK|ERROR
    /// &lt;details&gt;</c> — and logs the same thing with the grepable <c>[M2-CMD]</c> prefix.
    /// </summary>
    internal class DevCommands : MonoBehaviour
    {
        internal const string Prefix = "[M2-CMD]";
        internal const KeyCode ForceExportHotkey = KeyCode.F10;

        internal const string EnvCommandFile = "MULTIVERSE_CMD_FILE";
        internal const string EnvFamilyReport = "MULTIVERSE_FAMILY_REPORT";

        /// <summary>A forced export needs enough outward speed that the pipeline's dot-product guard passes.</summary>
        private const float MinimumExportSpeed = 8f;

        private const float PollSeconds = 0.2f;

        internal string CommandFile;
        internal string FallbackWorldName = string.Empty;
        internal float FamilyReportSeconds;

        private float nextPoll;
        private float nextFamilyReport;
        private bool busy;
        private bool autosaveDisabled;
        private bool announced;

        private void Update()
        {
            try
            {
                AnnounceOnce();
                DisableAutosaveOnce();
                PumpFamilyReport();

                if (Input.GetKeyDown(ForceExportHotkey) && !busy)
                {
                    StartCoroutine(Execute("hotkey", "export", "family"));
                }

                if (busy || string.IsNullOrEmpty(CommandFile) || Time.realtimeSinceStartup < nextPoll)
                {
                    return;
                }

                nextPoll = Time.realtimeSinceStartup + PollSeconds;
                string line = TakeCommand();
                if (string.IsNullOrEmpty(line))
                {
                    return;
                }

                string token;
                string verb;
                string argument;
                Split(line, out token, out verb, out argument);
                StartCoroutine(Execute(token, verb, argument));
            }
            catch (Exception e)
            {
                MultiversePlugin.Log.LogError($"{Prefix} the command poll threw — {e}");
            }
        }

        private void AnnounceOnce()
        {
            if (announced)
            {
                return;
            }

            announced = true;
            MultiversePlugin.Log.LogInfo(
                $"{Prefix} dev commands armed — file={(string.IsNullOrEmpty(CommandFile) ? "<none>" : CommandFile)} " +
                $"hotkey={ForceExportHotkey} familyReport={FamilyReportSeconds:F0}s. " +
                "Verbs: export <family|any|id>, census, count <id>, family, save [name], timescale <x>, autosave <on|off>, quit.");
        }

        /// <summary>
        /// A surprise autosave would rewrite the world in the middle of a kill test, so the rig turns
        /// it off for the session. Runtime only — the user's saved preference is untouched.
        /// </summary>
        private void DisableAutosaveOnce()
        {
            if (autosaveDisabled || !GameBridge.SimulationReady())
            {
                return;
            }

            autosaveDisabled = true;
            WorldSeeder.SetAutoSave(false, Prefix);
            MultiversePlugin.Log.LogInfo($"{Prefix} autosave disabled for this session (runtime only) — the rig drives every world save.");
        }

        private void PumpFamilyReport()
        {
            if (FamilyReportSeconds <= 0f || !GameBridge.SimulationReady() || Time.realtimeSinceStartup < nextFamilyReport)
            {
                return;
            }

            nextFamilyReport = Time.realtimeSinceStartup + FamilyReportSeconds;
            FamilyScan.LogReport(FamilyScan.Scan(), "periodic");
        }

        // ---- the command file -------------------------------------------------------------------

        /// <summary>Read and consume one command. Returns null when there is nothing complete to run.</summary>
        private string TakeCommand()
        {
            string content;
            try
            {
                if (!File.Exists(CommandFile))
                {
                    return null;
                }

                content = File.ReadAllText(CommandFile);
            }
            catch (Exception)
            {
                // The writer still holds it, or the rename has not landed. Try again next poll.
                return null;
            }

            if (string.IsNullOrEmpty(content) || !content.EndsWith("\n", StringComparison.Ordinal))
            {
                return null;
            }

            try
            {
                File.Delete(CommandFile);
            }
            catch (Exception e)
            {
                MultiversePlugin.Log.LogWarning($"{Prefix} could not delete {CommandFile} ({e.Message}) — the command is skipped to avoid running it twice.");
                return null;
            }

            return content.Trim();
        }

        private static void Split(string line, out string token, out string verb, out string argument)
        {
            token = "-";
            verb = string.Empty;
            argument = string.Empty;

            string[] parts = line.Split(new[] { ' ', '\t' }, 3, StringSplitOptions.RemoveEmptyEntries);
            if (parts.Length > 0)
            {
                token = parts[0];
            }

            if (parts.Length > 1)
            {
                verb = parts[1].ToLowerInvariant();
            }

            if (parts.Length > 2)
            {
                argument = parts[2].Trim();
            }
        }

        private void Report(string token, bool ok, string details)
        {
            string status = ok ? "OK" : "ERROR";
            string line = $"{token} {status} {details}";

            if (ok)
            {
                MultiversePlugin.Log.LogInfo($"{Prefix} {line}");
            }
            else
            {
                MultiversePlugin.Log.LogError($"{Prefix} {line}");
            }

            if (string.IsNullOrEmpty(CommandFile))
            {
                return;
            }

            try
            {
                File.AppendAllText(CommandFile + ".log", line + "\n", new UTF8Encoding(false));
            }
            catch (Exception e)
            {
                MultiversePlugin.Log.LogWarning($"{Prefix} could not append to the result log: {e.Message}");
            }
        }

        // ---- the verbs ---------------------------------------------------------------------------

        private IEnumerator Execute(string token, string verb, string argument)
        {
            busy = true;
            MultiversePlugin.Log.LogInfo($"{Prefix} token={token} verb={verb} arg='{argument}' accepted.");

            switch (verb)
            {
                case "export":
                    Export(token, argument);
                    break;

                case "census":
                    Census(token);
                    break;

                case "count":
                    Count(token, argument);
                    break;

                case "family":
                    FamilyReport report = FamilyScan.Scan();
                    FamilyScan.LogReport(report, "command");
                    Report(token, true,
                        $"living={report.living} eggs={report.eggs} withLivingParent={report.withLivingParent} " +
                        $"withLivingChild={report.withLivingChild} sampleChild={report.sampleChildId} " +
                        $"sampleParent={report.sampleParentId} linked={(report.Any ? "YES" : "NO")}");
                    break;

                case "save":
                    yield return Save(token, argument);
                    break;

                case "timescale":
                    TimeScale(token, argument);
                    break;

                case "autosave":
                    WorldSeeder.SetAutoSave(string.Equals(argument, "on", StringComparison.OrdinalIgnoreCase), Prefix);
                    Report(token, true, "autosave=" + argument);
                    break;

                case "quit":
                    Report(token, true, "quitting");
                    yield return new WaitForSecondsRealtime(0.5f);
                    Application.Quit();
                    break;

                default:
                    Report(token, false, $"unknown verb '{verb}'");
                    break;
            }

            busy = false;
        }

        /// <summary>
        /// Teleport one organism into the border strip with an outward velocity and let the ordinary
        /// pipeline take it from there. Writes <c>transform.position</c> **and** the
        /// <c>Rigidbody2D</c>'s position, because the body overwrites a transform-only correction on
        /// the next tick (m2_findings.md §4.3, caveat 2).
        /// </summary>
        private void Export(string token, string selector)
        {
            MultiverseClient client = MultiverseClient.Active;
            if (client == null)
            {
                Report(token, false, "no multiverse client is armed (no world loaded, or no border edge configured)");
                return;
            }

            if (!GameBridge.SimulationReady())
            {
                Report(token, false, "the simulation is not ready");
                return;
            }

            string how;
            BibiteBody target = SelectTarget(string.IsNullOrEmpty(selector) ? "family" : selector, out how);
            if (target == null)
            {
                Report(token, false, $"no target for selector '{selector}': {how}");
                return;
            }

            Rigidbody2D rigidbody = target.GetComponent<Rigidbody2D>();
            if (rigidbody == null)
            {
                Report(token, false, "the target has no Rigidbody2D");
                return;
            }

            int entityId = (target.id != null) ? target.id.id : 0;
            if (entityId == 0)
            {
                Report(token, false, "the target has no entity id");
                return;
            }

            Edge edge = client.Config.BorderEdge;
            BorderGeometry geometry = client.Geometry;
            Vector2 before = target.transform.position;
            Vector2 outward = BorderGeometry.OutwardNormal(edge);

            // Land in the middle of the strip, off the corners, so the placement satisfies InStrip
            // without sitting on the neighbouring edges' strips as well.
            float free = BorderGeometry.FreeCoordinate(edge, before);
            float freeLimit = Mathf.Max(0f, geometry.S - (geometry.W + geometry.EntryMargin));
            free = Mathf.Clamp(free, -freeLimit, freeLimit);

            bool positiveSide = edge == Edge.N || edge == Edge.E;
            float fixedCoordinate = positiveSide ? geometry.S - 0.5f * geometry.W : -geometry.S + 0.5f * geometry.W;
            Vector2 destination = (edge == Edge.N || edge == Edge.S)
                ? new Vector2(free, fixedCoordinate)
                : new Vector2(fixedCoordinate, free);

            float speed = Mathf.Max(rigidbody.linearVelocity.magnitude, MinimumExportSpeed);
            Vector2 velocity = outward * speed;

            // Any cooldown or entry-immunity left over from an earlier hop would silently swallow this
            // export. Clearing them changes no pipeline rule; it only stops the rig fighting itself.
            client.ClearMigrationBlocks(entityId);

            target.transform.position = new Vector3(destination.x, destination.y, target.transform.position.z);
            rigidbody.position = destination;
            rigidbody.linearVelocity = velocity;

            bool inStrip = geometry.InStrip(edge, destination);
            Report(token, true,
                $"entityId={entityId} selector={how} edge={ContractA.EdgeName(edge)} " +
                $"from=({before.x.ToString("F2", CultureInfo.InvariantCulture)},{before.y.ToString("F2", CultureInfo.InvariantCulture)}) " +
                $"to=({destination.x.ToString("F2", CultureInfo.InvariantCulture)},{destination.y.ToString("F2", CultureInfo.InvariantCulture)}) " +
                $"vel=({velocity.x.ToString("F2", CultureInfo.InvariantCulture)},{velocity.y.ToString("F2", CultureInfo.InvariantCulture)}) " +
                $"inStrip={inStrip} edgeOpen={client.EdgeOpen} S={geometry.S:F1} W={geometry.W:F1}");

            if (!inStrip || !client.EdgeOpen)
            {
                MultiversePlugin.Log.LogWarning(
                    $"{Prefix} token={token} the organism was placed but the pipeline will not fire " +
                    $"(inStrip={inStrip} edgeOpen={client.EdgeOpen}).");
            }
        }

        private BibiteBody SelectTarget(string selector, out string how)
        {
            if (int.TryParse(selector, NumberStyles.Integer, CultureInfo.InvariantCulture, out int explicitId))
            {
                BibiteBody body = GameBridge.FindBodyById(explicitId);
                how = (body != null) ? $"id={explicitId}" : $"no living organism with id {explicitId}";
                return body;
            }

            if (string.Equals(selector, "family", StringComparison.OrdinalIgnoreCase))
            {
                BibiteBody linked = FamilyScan.PickLinked(out string description);
                how = description;
                return linked;
            }

            BibiteTracker tracker = BibiteTracker.instance;
            if (tracker != null && tracker.bibites != null)
            {
                foreach (BibiteBody body in tracker.bibites)
                {
                    if (FamilyScan.IsAlive(body) && body.id != null && body.id.id != 0)
                    {
                        how = "first living organism";
                        return body;
                    }
                }
            }

            how = "the world holds no living organism";
            return null;
        }

        private void Census(string token)
        {
            string ids = FamilyScan.Census(out int population);
            Report(token, true, $"population={population} eggs={GameBridge.EggPopulation()} ids=[{ids}]");
        }

        private void Count(string token, string argument)
        {
            if (!int.TryParse(argument, NumberStyles.Integer, CultureInfo.InvariantCulture, out int entityId))
            {
                Report(token, false, $"'{argument}' is not an entity id");
                return;
            }

            BibiteBody body = GameBridge.FindBodyById(entityId);
            bool alive = body != null && FamilyScan.IsAlive(body);
            Report(token, true, $"entityId={entityId} present={(body != null ? 1 : 0)} alive={(alive ? 1 : 0)} " +
                $"population={GameBridge.LivingPopulation()}");
        }

        private IEnumerator Save(string token, string argument)
        {
            string name = argument;
            if (string.IsNullOrEmpty(name))
            {
                name = SimulationManager.gameName;
            }

            if (string.IsNullOrEmpty(name))
            {
                name = FallbackWorldName;
            }

            if (string.IsNullOrEmpty(name))
            {
                Report(token, false, "no world name to save under");
                yield break;
            }

            string path = WorldSeeder.SavePathFor(name);
            WorldOpResult result = new WorldOpResult();
            yield return WorldSeeder.SaveWorld(path, Prefix, result);

            if (!result.ok)
            {
                Report(token, false, $"world='{name}' {result.failure}");
                yield break;
            }

            long bytes = 0;
            try
            {
                bytes = new FileInfo(path).Length;
            }
            catch (Exception)
            {
                // Reported size only; the archive check inside SaveWorld already passed.
            }

            Report(token, true, $"world='{name}' path='{path}' bytes={bytes} population={GameBridge.LivingPopulation()}");
        }

        private void TimeScale(string token, string argument)
        {
            if (!float.TryParse(argument, NumberStyles.Float, CultureInfo.InvariantCulture, out float scale) || scale < 0f)
            {
                Report(token, false, $"'{argument}' is not a time scale");
                return;
            }

            WorldSeeder.SetTimeScale(scale, Prefix);
            Report(token, true, $"targetTimeScale={scale.ToString("F2", CultureInfo.InvariantCulture)} Time.timeScale={Time.timeScale:F2}");
        }
    }
}
