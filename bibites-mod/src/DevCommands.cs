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
                "Verbs: export <family|any|id>, place <family|any|id> <x> <y> [vx] [vy], edge <open|closed>, " +
                "census, count <id>, family, save [name], timescale <x>, autosave <on|off>, quit.");
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

                case "place":
                    Place(token, argument);
                    break;

                case "edge":
                    EdgeOverride(token, argument);
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

            Edge edge = client.Config.ExportEdge;
            BorderGeometry geometry = client.Geometry;
            Vector2 before = target.transform.position;
            Vector2 outward = BorderGeometry.OutwardNormal(edge);

            // Land in the middle of the strip, off the corners, so the placement satisfies the capture band
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

            bool inStrip = geometry.InCaptureBand(edge, destination);
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

        /// <summary>
        /// <c>place &lt;selector&gt; &lt;x&gt; &lt;y&gt; [vx] [vy]</c> — put one organism at an exact
        /// world position with an exact world velocity, and **do not export it**.
        ///
        /// This is the instrument the two D10 verifications need (m3_considerations.md Risks 1 and 2).
        /// <see cref="Export"/> always aims at the middle of the export strip, so it cannot ask the two
        /// questions D10 poses: what happens to an organism placed **past the wrap radius**, and what
        /// happens to one placed **outside the square but inside the capture band**, once travelling
        /// outward and once inward. Nothing about the pipeline is bypassed — the placement is only a
        /// teleport, and the ordinary FixedUpdate capture rule decides what follows.
        ///
        /// Both the transform and the Rigidbody2D are written, in the same frame: the body overwrites
        /// a transform-only correction on the next tick (m2_findings.md §4.3, caveat 2).
        /// </summary>
        private void Place(string token, string argument)
        {
            MultiverseClient client = MultiverseClient.Active;
            if (client == null)
            {
                Report(token, false, "no multiverse client is armed (no world loaded, or no export edge configured)");
                return;
            }

            if (!GameBridge.SimulationReady())
            {
                Report(token, false, "the simulation is not ready");
                return;
            }

            string[] parts = (argument ?? string.Empty).Split(new[] { ' ', '\t' }, StringSplitOptions.RemoveEmptyEntries);
            if (parts.Length < 3)
            {
                Report(token, false, "usage: place <family|any|id> <x> <y> [vx] [vy]");
                return;
            }

            if (!TryNumber(parts[1], out float x) || !TryNumber(parts[2], out float y))
            {
                Report(token, false, $"'{parts[1]} {parts[2]}' is not a world position");
                return;
            }

            float vx = 0f;
            float vy = 0f;
            if (parts.Length > 3 && !TryNumber(parts[3], out vx))
            {
                Report(token, false, $"'{parts[3]}' is not a velocity component");
                return;
            }

            if (parts.Length > 4 && !TryNumber(parts[4], out vy))
            {
                Report(token, false, $"'{parts[4]}' is not a velocity component");
                return;
            }

            BibiteBody target = SelectTarget(parts[0], out string how);
            if (target == null)
            {
                Report(token, false, $"no target for selector '{parts[0]}': {how}");
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

            BorderGeometry geometry = client.Geometry;
            Edge edge = client.Config.ExportEdge;
            Vector2 before = target.transform.position;
            Vector2 beforeVelocity = rigidbody.linearVelocity;
            Vector2 destination = new Vector2(x, y);
            Vector2 velocity = new Vector2(vx, vy);

            // A leftover cooldown or entry-immunity window would silently swallow the very decision the
            // verification is trying to observe. Clearing them also drops the containment tracker's last
            // position, so this teleport is not misread as a wrap.
            client.ClearMigrationBlocks(entityId);

            target.transform.position = new Vector3(destination.x, destination.y, target.transform.position.z);
            rigidbody.position = destination;
            rigidbody.linearVelocity = velocity;

            bool inBand = geometry.InCaptureBand(edge, destination);
            bool outward = Vector2.Dot(velocity, BorderGeometry.OutwardNormal(edge)) > 0f;
            float radius = destination.magnitude;
            float bodyLength = 0f;
            try
            {
                bodyLength = target.bodyLength;
            }
            catch (Exception)
            {
                // Reported value only.
            }

            bool pastWrapRadius = radius - bodyLength >= geometry.WrapRadius;

            string details =
                $"entityId={entityId} selector={how} edge={ContractA.EdgeName(edge)} " +
                $"from=({before.x.ToString("F2", CultureInfo.InvariantCulture)},{before.y.ToString("F2", CultureInfo.InvariantCulture)}) " +
                $"fromVel=({beforeVelocity.x.ToString("F2", CultureInfo.InvariantCulture)},{beforeVelocity.y.ToString("F2", CultureInfo.InvariantCulture)}) " +
                $"to=({destination.x.ToString("F2", CultureInfo.InvariantCulture)},{destination.y.ToString("F2", CultureInfo.InvariantCulture)}) " +
                $"vel=({velocity.x.ToString("F2", CultureInfo.InvariantCulture)},{velocity.y.ToString("F2", CultureInfo.InvariantCulture)}) " +
                $"r={radius.ToString("F1", CultureInfo.InvariantCulture)} bodyLength={bodyLength.ToString("F2", CultureInfo.InvariantCulture)} " +
                $"wrapRadius={geometry.WrapRadius.ToString("F1", CultureInfo.InvariantCulture)} pastWrapRadius={pastWrapRadius} " +
                $"bandInner={geometry.BandInnerBoundary(edge).ToString("F1", CultureInfo.InvariantCulture)} " +
                $"inCaptureBand={inBand} outward={outward} outsideSquare={geometry.Beyond(edge, destination)} " +
                $"edgeOpen={client.EdgeOpen} S={geometry.S.ToString("F1", CultureInfo.InvariantCulture)} " +
                $"W={geometry.W.ToString("F1", CultureInfo.InvariantCulture)}";

            Report(token, true, details);
            MultiversePlugin.Log.LogInfo($"{Containment.Prefix} event=PLACED {details}");
        }

        /// <summary>
        /// <c>edge &lt;open|closed&gt;</c> — force the export-edge gate locally, with no sidecar behind
        /// it. The D10 verifications run on one game with no rig, and without this the gate of §5.4
        /// keeps the export edge shut and no capture could ever be observed. It relaxes nothing else,
        /// and the next real EDGE_STATUS clears it.
        /// </summary>
        private void EdgeOverride(string token, string argument)
        {
            MultiverseClient client = MultiverseClient.Active;
            if (client == null)
            {
                Report(token, false, "no multiverse client is armed");
                return;
            }

            bool open;
            if (string.Equals(argument, "open", StringComparison.OrdinalIgnoreCase))
            {
                open = true;
            }
            else if (string.Equals(argument, "closed", StringComparison.OrdinalIgnoreCase) || string.Equals(argument, "close", StringComparison.OrdinalIgnoreCase))
            {
                open = false;
            }
            else
            {
                Report(token, false, $"'{argument}' is not open or closed");
                return;
            }

            client.SetDevEdgeOverride(open);
            Report(token, true, $"devEdgeOverride={open} edgeOpen={client.EdgeOpen} edge={ContractA.EdgeName(client.Config.ExportEdge)}");
        }

        private static bool TryNumber(string text, out float value)
        {
            return float.TryParse(text, NumberStyles.Float, CultureInfo.InvariantCulture, out value)
                && !float.IsNaN(value)
                && !float.IsInfinity(value);
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
