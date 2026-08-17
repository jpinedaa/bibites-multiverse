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
                "Verbs: export <family|any|id> [edge], place <family|any|id> <x> <y> [vx] [vy], " +
                "edge <open|closed> [E|N|W|S|all], camera <size> [x] [y], flourish <export|entry> [x] [y], " +
                "census, count <id>, family, save [name], timescale [x], autosave <on|off>, " +
                "speciesaudit [repair], speciespoison [clean], quit.");
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

                case "camera":
                    CameraView(token, argument);
                    break;

                case "flourish":
                    Flourish(token, argument);
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

                case "speciesaudit":
                    SpeciesAudit(token, argument);
                    break;

                case "speciespoison":
                    SpeciesPoison(token, argument);
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

            // `export <selector> [edge]` — the edge is optional and defaults to the first declared
            // export edge, so every M2/M3 rig script keeps working unchanged.
            string[] words = (selector ?? string.Empty).Split(new[] { ' ', '\t' }, StringSplitOptions.RemoveEmptyEntries);
            string who = (words.Length > 0) ? words[0] : "family";
            Edge requested = client.Config.PrimaryExportEdge;
            if (words.Length > 1)
            {
                if (!ContractA.TryParseEdge(words[1], out requested) || !client.Config.Exports(requested))
                {
                    Report(token, false,
                        $"'{words[1]}' is not one of this sim's declared export edges [{ContractA.EdgeNames(client.Config.ExportEdges)}]");
                    return;
                }
            }

            string how;
            BibiteBody target = SelectTarget(who, out how);
            if (target == null)
            {
                Report(token, false, $"no target for selector '{who}': {how}");
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

            Edge edge = requested;
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
            bool edgeOpen = client.IsEdgeOpen(edge);
            Report(token, true,
                $"entityId={entityId} selector={how} edge={ContractA.EdgeName(edge)} " +
                $"from=({before.x.ToString("F2", CultureInfo.InvariantCulture)},{before.y.ToString("F2", CultureInfo.InvariantCulture)}) " +
                $"to=({destination.x.ToString("F2", CultureInfo.InvariantCulture)},{destination.y.ToString("F2", CultureInfo.InvariantCulture)}) " +
                $"vel=({velocity.x.ToString("F2", CultureInfo.InvariantCulture)},{velocity.y.ToString("F2", CultureInfo.InvariantCulture)}) " +
                $"inStrip={inStrip} edgeOpen={edgeOpen} edges=[{client.EdgeSummary()}] S={geometry.S:F1} W={geometry.W:F1}");

            if (!inStrip || !edgeOpen)
            {
                MultiversePlugin.Log.LogWarning(
                    $"{Prefix} token={token} the organism was placed but the pipeline will not fire " +
                    $"(inStrip={inStrip} edgeOpen={edgeOpen}).");
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
            Edge edge = client.Config.PrimaryExportEdge;
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
                $"bands=[{Bands(client, geometry, destination)}] " +
                $"edges=[{client.EdgeSummary()}] S={geometry.S.ToString("F1", CultureInfo.InvariantCulture)} " +
                $"W={geometry.W.ToString("F1", CultureInfo.InvariantCulture)}";

            Report(token, true, details);
            MultiversePlugin.Log.LogInfo($"{Containment.Prefix} event=PLACED {details}");
        }

        /// <summary>
        /// One line per declared export edge: whether the placed organism is inside that band.
        /// The corner is the interesting case, and this is what makes it visible in a command result.
        /// </summary>
        private static string Bands(MultiverseClient client, BorderGeometry geometry, Vector2 point)
        {
            System.Text.StringBuilder text = new System.Text.StringBuilder();
            for (int i = 0; i < client.Config.ExportEdges.Count; i++)
            {
                Edge edge = client.Config.ExportEdges[i];
                if (i > 0)
                {
                    text.Append(' ');
                }

                text.Append(ContractA.EdgeName(edge)).Append('=').Append(geometry.InCaptureBand(edge, point));
            }

            return text.ToString();
        }

        /// <summary>
        /// <c>edge &lt;open|closed&gt; [E|N|W|S|all]</c> — force an export-edge gate locally, with no
        /// sidecar behind it. The D10 verifications and the M4 portal check run on one game with no rig,
        /// and without this the gate of §5.4 keeps every export edge shut, so no capture and no portal
        /// could ever be observed. It relaxes nothing else — the bands, the outward-velocity test and
        /// the corner rule all still run — and the next real EDGE_STATUS clears it.
        ///
        /// The edge argument is optional and defaults to every declared export edge, so the M3 form
        /// <c>edge open</c> keeps working.
        /// </summary>
        private void EdgeOverride(string token, string argument)
        {
            MultiverseClient client = MultiverseClient.Active;
            if (client == null)
            {
                Report(token, false, "no multiverse client is armed");
                return;
            }

            string[] words = (argument ?? string.Empty).Split(new[] { ' ', '\t' }, StringSplitOptions.RemoveEmptyEntries);
            string state = (words.Length > 0) ? words[0] : string.Empty;

            bool open;
            if (string.Equals(state, "open", StringComparison.OrdinalIgnoreCase))
            {
                open = true;
            }
            else if (string.Equals(state, "closed", StringComparison.OrdinalIgnoreCase) || string.Equals(state, "close", StringComparison.OrdinalIgnoreCase))
            {
                open = false;
            }
            else
            {
                Report(token, false, $"'{argument}' is not open or closed");
                return;
            }

            Edge edge = Edge.None;
            if (words.Length > 1 && !string.Equals(words[1], "all", StringComparison.OrdinalIgnoreCase))
            {
                if (!ContractA.TryParseEdge(words[1], out edge) || !client.Config.Exports(edge))
                {
                    Report(token, false,
                        $"'{words[1]}' is not one of this sim's declared export edges [{ContractA.EdgeNames(client.Config.ExportEdges)}]");
                    return;
                }
            }

            client.SetDevEdgeOverride(edge, open);
            Report(token, true,
                $"devEdgeOverride={open} target={(edge == Edge.None ? "all" : ContractA.EdgeName(edge))} " +
                $"edges=[{client.EdgeSummary()}]");
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

        /// <summary>
        /// <c>flourish &lt;export|entry&gt; [x] [y]</c> — draw one migration ring on demand.
        ///
        /// The ring's real triggers are a completed export and a completed arrival, and **both need a
        /// sidecar**: the export ring fires on <c>MIGRATE_OUT_ACK</c> and the arrival ring fires on a
        /// spawn. A mod-only smoke test can therefore never reach either, so without this verb the
        /// question "does the Disc path throw?" stays open until the rehearsal rig runs. It draws
        /// through exactly the same <see cref="PortalVisual.Flash"/> entry point the two hooks use, so a
        /// green result here is a real statement about the shipped path.
        /// </summary>
        private void Flourish(string token, string argument)
        {
            string[] parts = (argument ?? string.Empty).Split(new[] { ' ', '\t' }, StringSplitOptions.RemoveEmptyEntries);
            bool export = parts.Length == 0 || !string.Equals(parts[0], "entry", StringComparison.OrdinalIgnoreCase);

            float x = 0f;
            float y = 0f;
            if (parts.Length >= 3 && (!TryNumber(parts[1], out x) || !TryNumber(parts[2], out y)))
            {
                Report(token, false, "usage: flourish <export|entry> [x] [y]");
                return;
            }

            PortalVisual.Flash(new Vector3(x, y, 0f), export);
            Report(token, true, $"flourish role={(export ? "export" : "entry")} at=({x.ToString("F1", CultureInfo.InvariantCulture)},{y.ToString("F1", CultureInfo.InvariantCulture)})");
        }

        /// <summary>
        /// <c>camera &lt;orthographicSize&gt; [x] [y]</c> — put the view where a check can see it.
        ///
        /// The portal has to be legible across an 800× zoom range (`m4_portal_findings.md` §5.1), and
        /// the exit test's Part 8 sweeps `orthographicSize` 5, 250, 2000 and 4000. A hand-driven mouse
        /// wheel cannot do that unattended, so the rig needs the same affordance the command file gives
        /// everything else. It drives the game's own public <c>CameraManager.SetCamSize</c> and then
        /// fires the two zoom events, so the background grid stays coherent with the new size exactly
        /// as it does after a real zoom.
        /// </summary>
        private void CameraView(string token, string argument)
        {
            string[] parts = (argument ?? string.Empty).Split(new[] { ' ', '\t' }, StringSplitOptions.RemoveEmptyEntries);
            if (parts.Length < 1 || !TryNumber(parts[0], out float size) || size <= 0f)
            {
                Report(token, false, "usage: camera <orthographicSize> [x] [y]");
                return;
            }

            Camera camera = Camera.main;
            if (camera == null)
            {
                Report(token, false, "Camera.main is null — no simulation camera to drive");
                return;
            }

            try
            {
                CameraManager manager = CameraManager.instance;
                if (manager != null)
                {
                    manager.SetCamSize(size);
                }
                else
                {
                    camera.orthographicSize = size;
                }

                if (parts.Length >= 3 && TryNumber(parts[1], out float x) && TryNumber(parts[2], out float y))
                {
                    Vector3 where = camera.transform.position;
                    camera.transform.position = new Vector3(x, y, where.z);
                }

                CameraManager.OnCameraZoomChange.Invoke();
                CameraManager.onCameraSizeChange.Invoke(size);
            }
            catch (Exception e)
            {
                Report(token, false, "could not drive the camera: " + e.Message);
                return;
            }

            Vector3 position = camera.transform.position;
            Report(token, true,
                $"orthographicSize={camera.orthographicSize.ToString("F1", CultureInfo.InvariantCulture)} " +
                $"position=({position.x.ToString("F1", CultureInfo.InvariantCulture)},{position.y.ToString("F1", CultureInfo.InvariantCulture)},{position.z.ToString("F1", CultureInfo.InvariantCulture)}) " +
                $"cullingMask=0x{camera.cullingMask:X8}");
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

        /// <summary>
        /// <c>speciesaudit [repair]</c> — the operational read of the game's species-history counter
        /// defect (<see cref="SpeciesHistoryGuard"/>). <c>wouldOverrun</c> above zero means the next
        /// world save would throw <c>IndexOutOfRangeException</c> in
        /// <c>LogLikeSpeciesDataArray.SavePointToArrays</c> if the guard were not armed; with the guard
        /// armed it is the count the guard is about to correct.
        /// </summary>
        private void SpeciesAudit(string token, string argument)
        {
            bool repair = string.Equals((argument ?? string.Empty).Trim(), "repair", StringComparison.OrdinalIgnoreCase);

            if (GlobalLineageManager.Instance == null)
            {
                Report(token, false, "no world is loaded (GlobalLineageManager.Instance is null)");
                return;
            }

            SpeciesHistoryGuard.Audit audit = SpeciesHistoryGuard.Scan(repair);
            Report(token, true,
                $"guard={(SpeciesHistoryGuard.Enabled ? "ON" : "OFF")} recorded={audit.recorded} " +
                $"wouldOverrun={audit.wouldOverrun} inconsistent={audit.inconsistent} repaired={audit.repaired} " +
                $"sessionArraysRepaired={SpeciesHistoryGuard.RepairedArrays} worst={audit.worst}");
        }

        /// <summary>
        /// <c>speciespoison [clean]</c> — the regression check. It drives one throwaway species'
        /// history into the exact state the 2026-08-06/07 save failures were in, so the failure can be
        /// produced on demand instead of waited for. Follow it with <c>save</c>:
        ///
        /// * with <c>MULTIVERSE_SPECIES_GUARD=0</c> the save throws, with the incident's stack;
        /// * with the guard at its default the save succeeds and the world on disk is complete.
        ///
        /// <c>speciespoison clean</c> takes the test species out again.
        /// </summary>
        private void SpeciesPoison(string token, string argument)
        {
            bool clean = string.Equals((argument ?? string.Empty).Trim(), "clean", StringComparison.OrdinalIgnoreCase);
            string detail = SpeciesHistoryGuard.Poison(clean, out bool ok);
            Report(token, ok, detail);
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

        /// <summary>
        /// <c>timescale x</c> sets the speed; <c>timescale</c> with no argument only reports it, which
        /// is how a verifier reads what a world is actually running at without moving it. Both answers
        /// start with <c>targetTimeScale=</c>, so a caller that greps for that prefix still matches.
        ///
        /// The two numbers differ on purpose. <c>targetTimeScale</c> is what was asked for — the value
        /// under the game's speed slider — and <c>Time.timeScale</c> is what the game's minimum-FPS
        /// servo is applying right now, which climbs toward the target over a few seconds and stays
        /// below it on a machine that cannot draw fast enough.
        /// </summary>
        private void TimeScale(string token, string argument)
        {
            string wanted = (argument ?? string.Empty).Trim();
            if (wanted.Length == 0)
            {
                Report(token, true,
                    $"targetTimeScale={CurrentTarget()} Time.timeScale={Time.timeScale:F2} " +
                    "(read only — pass a number to change it)");
                return;
            }

            if (!float.TryParse(wanted, NumberStyles.Float, CultureInfo.InvariantCulture, out float scale) || scale < 0f)
            {
                Report(token, false, $"'{argument}' is not a time scale");
                return;
            }

            WorldSeeder.SetTimeScale(scale, Prefix);
            Report(token, true, $"targetTimeScale={scale.ToString("F2", CultureInfo.InvariantCulture)} Time.timeScale={Time.timeScale:F2}");
        }

        /// <summary>The game's requested speed, or <c>?</c> when there is no TimeController to ask.</summary>
        private static string CurrentTarget()
        {
            try
            {
                return TimeController.targetTimeScale.val.ToString("F2", CultureInfo.InvariantCulture);
            }
            catch (Exception)
            {
                return "?";
            }
        }
    }
}
