using System;
using System.Collections.Generic;
using System.Globalization;
using ManagementScripts;
using Newtonsoft.Json;
using Newtonsoft.Json.Linq;
using OneUseScripts;
using SimulationScripts.BibiteScripts;
using UnityEngine;

namespace BibitesMultiverse
{
    /// <summary>
    /// The Contract A client (contracts/contract-a.md). One MonoBehaviour owns the whole main-thread
    /// side: the session lifecycle, the handshake, the heartbeat, EDGE_STATUS gating, and the drain of
    /// everything the socket thread produced.
    ///
    /// Thread rule (§11.2): nothing touches Unity off the main thread. The transport publishes into a
    /// concurrent queue; this component drains it. Frames that only change client state are handled in
    /// Update, so a paused simulation still handshakes and heartbeats. Frames that spawn or destroy an
    /// organism are deferred to FixedUpdate (§5.5, §5.7).
    ///
    /// Connection lifecycle (§2): the client dials only while a simulation world is loaded, and closes
    /// with 1000 when the world unloads. It never holds a connection open on the main menu.
    /// </summary>
    internal class MultiverseClient : MonoBehaviour
    {
        internal const string Prefix = "[M2]";

        /// <summary>Set while a world is loaded and the client is configured. The Harmony postfix reads it.</summary>
        internal static MultiverseClient Active;

        private MultiverseConfig config;
        private BorderGeometry geometry;
        private WorldSettings worldSettings;
        private CrossingStats crossing;
        private MigrationExporter exporter;
        private MigrationImporter importer;
        private WebSocketTransport transport;

        /// <summary>A frame that will spawn, destroy or revive an organism, waiting for FixedUpdate.</summary>
        private struct PendingMutation
        {
            internal string type;
            internal JObject data;
            internal float queuedAtRealtime;
        }

        private readonly List<PendingMutation> pendingMutations = new List<PendingMutation>();

        private bool armed;
        private string sessionId;
        private long simTick;
        private long lastEdgeStatusEpoch;
        private bool edgeOpen;
        private bool handshakeSent;

        /// <summary>
        /// The transport generation this client handshook on (§5.1). A socket that has re-opened but
        /// whose <c>Connected</c> event has not been drained yet is a different connection, and
        /// sending a heartbeat into it puts a non-CONFIG_UPDATE frame first, which is close 4003.
        /// </summary>
        private int handshakeGeneration = -1;
        private float nextHeartbeatRealtime;
        private bool haltedReported;

        internal MultiverseConfig Config => config;
        internal BorderGeometry Geometry => geometry;
        internal MigrationExporter Exporter => exporter;
        internal MigrationImporter Importer => importer;
        internal long SimTick => simTick;
        internal bool EdgeOpen => edgeOpen && transport != null && transport.IsConnected;

        internal void Initialize(MultiverseConfig configuration)
        {
            config = configuration;
            geometry = new BorderGeometry(config, OnSimulationSizeChanged);
            worldSettings = new WorldSettings();
            crossing = new CrossingStats();
            exporter = new MigrationExporter(this);
            importer = new MigrationImporter(this);
            transport = new WebSocketTransport(config.Url);

            MultiversePlugin.Log.LogInfo($"{Prefix} transport probe: {WebSocketTransport.ProbeAvailability()}");
            MultiversePlugin.Log.LogInfo(
                $"{Prefix} client ready — it dials {config.Url} only while a world is loaded (contract-a.md §2). " +
                "Every edge stays closed until the first EDGE_STATUS.");
        }

        // ---- world lifecycle -------------------------------------------------------------------

        private void Update()
        {
            try
            {
                bool ready = GameBridge.SimulationReady();
                if (ready && !armed)
                {
                    Arm();
                }
                else if (!ready && armed)
                {
                    Disarm("the world unloaded");
                }

                DrainTransport();

                if (armed)
                {
                    PumpHeartbeat();
                    RejectWhilePaused();
                }
            }
            catch (Exception e)
            {
                MultiversePlugin.Log.LogError($"{Prefix} Update threw — {e}");
            }
        }

        private void FixedUpdate()
        {
            if (!armed)
            {
                return;
            }

            try
            {
                simTick++;
                ApplyMutations();
                exporter.CheckTimeouts();
                crossing.Tick(config.BorderEdge, geometry, GameBridge.LivingPopulation(), simTick);
            }
            catch (Exception e)
            {
                MultiversePlugin.Log.LogError($"{Prefix} FixedUpdate threw — {e}");
            }
        }

        private void OnDestroy()
        {
            if (armed)
            {
                Disarm("the plugin is shutting down");
            }
        }

        private void OnApplicationQuit()
        {
            if (armed)
            {
                Disarm("the game is quitting");
            }
        }

        private void Arm()
        {
            armed = true;
            Active = this;

            // §5.1 — a fresh sessionId on every world load. It is what tells the sidecar the mod lost
            // all in-memory state and that the world may have rolled back to an earlier save (§7.4).
            // The game's own gameName is unusable for this: CreateSave never writes one.
            sessionId = ContractA.NewUuid();
            simTick = 0;
            lastEdgeStatusEpoch = 0;
            edgeOpen = false;
            handshakeSent = false;
            handshakeGeneration = -1;
            haltedReported = false;
            pendingMutations.Clear();
            exporter.Clear();
            importer.Clear();
            crossing.Reset();

            geometry.Subscribe();
            worldSettings.LogBoundarySettings(geometry.S);
            worldSettings.LogBibiteHolderTransform();
            worldSettings.Apply();

            MultiversePlugin.Log.LogInfo(
                $"{Prefix} world loaded — sessionId={sessionId} edge={ContractA.EdgeName(config.BorderEdge)} " +
                $"S={geometry.S.ToString("F2", CultureInfo.InvariantCulture)} W={geometry.W.ToString("F2", CultureInfo.InvariantCulture)} " +
                $"entryInset={(geometry.W + geometry.EntryMargin).ToString("F2", CultureInfo.InvariantCulture)} " +
                $"population={GameBridge.LivingPopulation()}. Dialling {config.Url}.");

            transport.Start();
        }

        private void Disarm(string why)
        {
            MultiversePlugin.Log.LogInfo($"{Prefix} stopping the Contract A client: {why}.");

            foreach (PendingMutation pending in pendingMutations)
            {
                if (string.Equals(pending.type, ContractA.TypeMigrateIn, StringComparison.Ordinal))
                {
                    importer.NackShuttingDown(pending.data);
                }
            }

            pendingMutations.Clear();

            exporter.NoteUnresolvedAtUnload();
            importer.Clear();
            geometry.Unsubscribe();
            worldSettings.Restore();

            transport.Stop(ContractA.CloseNormal, why);

            edgeOpen = false;
            handshakeSent = false;
            handshakeGeneration = -1;
            armed = false;
            Active = null;

            MultiversePlugin.Log.LogInfo(
                $"{CrossingStats.Prefix} session summary: totalStripEntries={crossing.TotalStripEntries} " +
                $"totalCrossings={crossing.TotalCrossings}");
        }

        private void OnSimulationSizeChanged(float value)
        {
            if (!armed)
            {
                return;
            }

            // §5.1 receiver obligations: a change in simulationSize makes the sidecar re-check peer
            // agreement. Belt to the heartbeat's braces.
            SendConfigUpdate("sim_size_changed");
        }

        // ---- transport drain ---------------------------------------------------------------------

        private void DrainTransport()
        {
            if (transport == null)
            {
                return;
            }

            while (transport.TryDequeue(out TransportEvent transportEvent))
            {
                switch (transportEvent.kind)
                {
                    case TransportEventKind.Log:
                        LogFromTransport(transportEvent);
                        break;

                    case TransportEventKind.Connected:
                        OnConnected(transportEvent.generation);
                        break;

                    case TransportEventKind.Disconnected:
                        OnDisconnected(transportEvent.closeCode, transportEvent.text);
                        break;

                    case TransportEventKind.Frame:
                        OnFrame(transportEvent.text);
                        break;
                }
            }
        }

        private static void LogFromTransport(TransportEvent transportEvent)
        {
            switch (transportEvent.level)
            {
                case TransportLogLevel.Error:
                    MultiversePlugin.Log.LogError($"{Prefix} transport: {transportEvent.text}");
                    break;
                case TransportLogLevel.Warning:
                    MultiversePlugin.Log.LogWarning($"{Prefix} transport: {transportEvent.text}");
                    break;
                default:
                    MultiversePlugin.Log.LogInfo($"{Prefix} transport: {transportEvent.text}");
                    break;
            }
        }

        private void OnConnected(int connectionGeneration)
        {
            if (!armed)
            {
                // §2: no connection is held open without a world. Close it and stop.
                transport.Stop(ContractA.CloseNormal, "no world is loaded");
                return;
            }

            // §6.2 — the EDGE_STATUS epoch counter resets on a new connection, so the mod resets its
            // last-applied epoch when it opens one.
            lastEdgeStatusEpoch = 0;
            edgeOpen = false;
            importer.ClearConnectionState();

            handshakeGeneration = connectionGeneration;
            SendConfigUpdate("connect");
            handshakeSent = true;
            nextHeartbeatRealtime = Time.realtimeSinceStartup + ContractA.HeartbeatIntervalMs / 1000f;

            // §6.3 — an unresolved MIGRATE_OUT is replayed unchanged on the new connection.
            exporter.ReplayOnReconnect();
        }

        private void OnDisconnected(int closeCode, string reason)
        {
            handshakeSent = false;
            handshakeGeneration = -1;
            bool wasOpen = edgeOpen;
            edgeOpen = false;

            bool fatal = closeCode == ContractA.CloseProtocolUnsupported
                || closeCode == ContractA.CloseSectorMismatch
                || closeCode == ContractA.CloseGameVersionUnsupported
                || closeCode == ContractA.CloseReplaced;

            string description = WebSocketTransport.DescribeCloseCode(closeCode);
            if (fatal)
            {
                transport.Halt();
                MultiversePlugin.Log.LogError(
                    $"{Prefix} the sidecar closed with {description}: {reason}. This is not retried — restart or reconfigure the mod. " +
                    "Every edge stays closed.");
            }
            else
            {
                MultiversePlugin.Log.LogWarning(
                    $"{Prefix} disconnected ({description}: {reason}). Every edge is closed until a new EDGE_STATUS arrives; " +
                    "reconnecting with full-jitter backoff.");
            }

            if (wasOpen)
            {
                MultiversePlugin.Log.LogInfo($"{Prefix} edge={ContractA.EdgeName(config.BorderEdge)} open=false reason=disconnected");
            }
        }

        private void OnFrame(string text)
        {
            if (!ContractA.TryReadEnvelope(text, out string type, out JObject data, out string problem, out int closeCode))
            {
                // §3.2 / §9.3 — a bad envelope terminates the session. A wrong major version is 4000
                // and stops reconnection; anything else is 4003 and reconnects with backoff.
                MultiversePlugin.Log.LogError(
                    $"{Prefix} bad frame from the sidecar ({problem}) — closing {WebSocketTransport.DescribeCloseCode(closeCode)}.");
                transport.Stop(closeCode, problem);

                if (closeCode == ContractA.CloseProtocolUnsupported)
                {
                    transport.Halt();
                }
                else if (armed)
                {
                    transport.Start(1);
                }

                return;
            }

            switch (type)
            {
                case ContractA.TypeEdgeStatus:
                    ApplyEdgeStatus(data);
                    break;

                case ContractA.TypeMigrateIn:
                case ContractA.TypeMigrateOutAck:
                case ContractA.TypeMigrateOutNack:
                    // Anything that spawns, destroys or revives an organism waits for FixedUpdate.
                    pendingMutations.Add(new PendingMutation
                    {
                        type = type,
                        data = data,
                        queuedAtRealtime = Time.realtimeSinceStartup
                    });
                    break;

                default:
                    // §3.1 — an unknown type is a forward-compatible addition, not a fault.
                    MultiversePlugin.Log.LogWarning($"{Prefix} ignoring an unknown message type '{type}'.");
                    break;
            }
        }

        private void ApplyMutations()
        {
            if (pendingMutations.Count == 0)
            {
                return;
            }

            List<PendingMutation> batch = new List<PendingMutation>(pendingMutations);
            pendingMutations.Clear();

            foreach (PendingMutation entry in batch)
            {
                try
                {
                    switch (entry.type)
                    {
                        case ContractA.TypeMigrateIn:
                            importer.Handle(entry.data);
                            break;
                        case ContractA.TypeMigrateOutAck:
                            HandleMigrateOutAck(entry.data);
                            break;
                        case ContractA.TypeMigrateOutNack:
                            HandleMigrateOutNack(entry.data);
                            break;
                    }
                }
                catch (Exception e)
                {
                    MultiversePlugin.Log.LogError($"{Prefix} handling {entry.type} threw — {e}");
                }
            }
        }

        /// <summary>
        /// FixedUpdate does not run at timeScale 0, so a delivery that arrives while the sim is paused
        /// would sit unanswered. §9.2 has the code for exactly that: SIM_NOT_READY, transient.
        /// </summary>
        private void RejectWhilePaused()
        {
            if (pendingMutations.Count == 0 || Time.timeScale > 0f)
            {
                return;
            }

            float now = Time.realtimeSinceStartup;
            for (int i = pendingMutations.Count - 1; i >= 0; i--)
            {
                PendingMutation entry = pendingMutations[i];
                if (now - entry.queuedAtRealtime < 2f || !string.Equals(entry.type, ContractA.TypeMigrateIn, StringComparison.Ordinal))
                {
                    continue;
                }

                importer.NackNotReady(entry.data, "the simulation is paused (timeScale 0), so no organism can be spawned");
                pendingMutations.RemoveAt(i);
            }
        }

        private void HandleMigrateOutAck(JObject data)
        {
            if (!ContractA.TryString(data, "migrationId", out string migrationId) || !ContractA.TryInt(data, "entityId", out int entityId))
            {
                MultiversePlugin.Log.LogError($"{Prefix} MIGRATE_OUT_ACK is missing 'migrationId' or 'entityId' — ignored.");
                return;
            }

            ContractA.TryLong(data, "journaledAt", out long journaledAt);
            ContractA.TryBool(data, "unsolicited", out bool unsolicited);
            exporter.HandleAck(migrationId, entityId, unsolicited, journaledAt);
        }

        private void HandleMigrateOutNack(JObject data)
        {
            if (!ContractA.TryString(data, "migrationId", out string migrationId) || !ContractA.TryInt(data, "entityId", out int entityId))
            {
                MultiversePlugin.Log.LogError($"{Prefix} MIGRATE_OUT_NACK is missing 'migrationId' or 'entityId' — ignored.");
                return;
            }

            if (!ContractA.TryString(data, "code", out string code))
            {
                code = "UNKNOWN";
            }

            // §9 — never switch on 'code' without a default branch. An unrecognised code is handled as
            // its stated class, which is why 'class' is redundant on purpose.
            if (!ContractA.TryString(data, "class", out string nackClass)
                || (!string.Equals(nackClass, ContractA.ClassTransient, StringComparison.Ordinal)
                    && !string.Equals(nackClass, ContractA.ClassPermanent, StringComparison.Ordinal)))
            {
                MultiversePlugin.Log.LogWarning($"{Prefix} MIGRATE_OUT_NACK code={code} has no usable 'class' — treating it as transient.");
                nackClass = ContractA.ClassTransient;
            }

            ContractA.TryString(data, "message", out string message);
            bool hasRetry = ContractA.TryInt(data, "retryAfterMs", out int retryAfterMs);
            exporter.HandleNack(migrationId, entityId, code, nackClass, message ?? string.Empty, retryAfterMs, hasRetry);
        }

        // ---- EDGE_STATUS (§5.4) --------------------------------------------------------------------

        private void ApplyEdgeStatus(JObject data)
        {
            if (!ContractA.TryLong(data, "epoch", out long epoch))
            {
                MultiversePlugin.Log.LogError($"{Prefix} EDGE_STATUS without a numeric 'epoch' — ignored.");
                return;
            }

            if (epoch <= lastEdgeStatusEpoch)
            {
                MultiversePlugin.Log.LogInfo($"{Prefix} EDGE_STATUS epoch={epoch} is not newer than {lastEdgeStatusEpoch} — ignored (replay-safe).");
                return;
            }

            if (!(data["edges"] is JArray edges))
            {
                MultiversePlugin.Log.LogError($"{Prefix} EDGE_STATUS without an 'edges' array — ignored.");
                return;
            }

            lastEdgeStatusEpoch = epoch;

            // Full state, not a delta: an edge that is absent from the array is closed.
            bool open = false;
            string reason = "no_peer";
            float peerSize = 0f;
            bool peerSizeKnown = false;

            foreach (JToken token in edges)
            {
                if (!(token is JObject entry))
                {
                    continue;
                }

                if (!ContractA.TryString(entry, "edge", out string edgeText) || !ContractA.TryParseEdge(edgeText, out Edge edge))
                {
                    continue;
                }

                if (edge != config.BorderEdge)
                {
                    MultiversePlugin.Log.LogWarning($"{Prefix} EDGE_STATUS mentions edge {edgeText}, which this mod did not declare — ignored.");
                    continue;
                }

                ContractA.TryBool(entry, "open", out open);
                if (!ContractA.TryString(entry, "reason", out reason))
                {
                    reason = open ? "peer_live" : "no_peer";
                }

                peerSizeKnown = ContractA.TryFloat(entry, "peerSimulationSize", out peerSize);
            }

            // §5.4 — the mod compares peerSimulationSize against its own S and treats the edge as
            // closed on a mismatch, even though the sidecar already checked. Two independent checks,
            // because a mid-run resize can race.
            if (open && peerSizeKnown && Mathf.Abs(peerSize - geometry.S) > Mathf.Max(0.001f, 1e-4f * geometry.S))
            {
                MultiversePlugin.Log.LogWarning(
                    $"{Prefix} EDGE_STATUS says edge {ContractA.EdgeName(config.BorderEdge)} is open, but the peer's S is " +
                    $"{peerSize:F2} and this sim's S is {geometry.S:F2} — holding the edge closed.");
                open = false;
                reason = "sim_size_mismatch";
            }

            bool changed = open != edgeOpen;
            edgeOpen = open;

            MultiversePlugin.Log.LogInfo(
                $"{Prefix} EDGE_STATUS epoch={epoch} edge={ContractA.EdgeName(config.BorderEdge)} open={open} reason={reason} " +
                $"peerS={(peerSizeKnown ? peerSize.ToString("F2", CultureInfo.InvariantCulture) : "<absent>")} changed={changed}");
        }

        /// <summary>A transient NACK that names the edge stops migration there until a new EDGE_STATUS.</summary>
        internal void CloseEdgeUntilNextStatus(string code)
        {
            if (!edgeOpen)
            {
                return;
            }

            edgeOpen = false;
            MultiversePlugin.Log.LogWarning(
                $"{Prefix} edge={ContractA.EdgeName(config.BorderEdge)} closed locally after {code}; it reopens only on a new EDGE_STATUS.");
        }

        // ---- outbound ------------------------------------------------------------------------------

        internal void SendFrame(string frame)
        {
            transport?.Send(frame);
        }

        private void SendConfigUpdate(string reason)
        {
            JObject data = new JObject
            {
                ["sessionId"] = sessionId,
                ["reason"] = reason,
                ["gameVersion"] = Application.version,
                ["modVersion"] = MultiversePlugin.Version,
                ["simulationSize"] = geometry.S,
                ["borderEdges"] = new JArray { ContractA.EdgeName(config.BorderEdge) },
                ["borderWidth"] = geometry.W
            };

            if (config.HasSector)
            {
                data["sector"] = new JObject { ["x"] = config.SectorX, ["y"] = config.SectorY };
            }

            string worldName = SimulationManager.gameName;
            if (!string.IsNullOrEmpty(worldName))
            {
                data["worldName"] = worldName;
            }

            SendFrame(ContractA.Envelope(ContractA.TypeConfigUpdate, data).ToString(Formatting.None));
            MultiversePlugin.Log.LogInfo(
                $"{Prefix} CONFIG_UPDATE reason={reason} sessionId={sessionId} " +
                $"sector={(config.HasSector ? $"({config.SectorX},{config.SectorY})" : "<omitted>")} " +
                $"S={geometry.S.ToString("F2", CultureInfo.InvariantCulture)} W={geometry.W.ToString("F2", CultureInfo.InvariantCulture)} " +
                $"borderEdges=[{ContractA.EdgeName(config.BorderEdge)}]");
        }

        /// <summary>§8 — wall-clock cadence. A paused sim, 0x and 20x all heartbeat the same.</summary>
        private void PumpHeartbeat()
        {
            if (!handshakeSent || transport == null || !transport.IsConnected || transport.Generation != handshakeGeneration)
            {
                if (transport != null && transport.IsHalted && !haltedReported)
                {
                    haltedReported = true;
                    MultiversePlugin.Log.LogError($"{Prefix} the transport is halted — no further connection attempts will be made.");
                }

                return;
            }

            float now = Time.realtimeSinceStartup;
            if (now < nextHeartbeatRealtime)
            {
                return;
            }

            nextHeartbeatRealtime = now + ContractA.HeartbeatIntervalMs / 1000f;

            JObject data = new JObject
            {
                ["sessionId"] = sessionId,
                ["simTick"] = simTick,
                ["simulatedTime"] = TimeKeeper.simulatedTime,
                ["population"] = GameBridge.LivingPopulation(),
                ["eggCount"] = GameBridge.EggPopulation(),
                ["paused"] = TimeController.paused,
                ["timeScale"] = Time.timeScale,
                ["simulationSize"] = geometry.S,
                ["inFlightOut"] = exporter.InFlightCount,
                ["pendingIn"] = pendingMutations.Count
            };

            SendFrame(ContractA.Envelope(ContractA.TypeHeartbeat, data).ToString(Formatting.None));
        }

        // ---- the border strip (called from the Harmony postfix) --------------------------------------

        /// <summary>
        /// One organism, one tick. Every guard the early returns of BibiteBody.FixedUpdate imply is
        /// re-applied here, because a postfix runs on every return path (m2_findings.md §(b)).
        /// </summary>
        internal void OnBodyTick(BibiteBody body)
        {
            if (!armed || body == null)
            {
                return;
            }

            if (!body.born || body.dead || body.dying || body.destroyed || !(Time.timeScale > 0f))
            {
                return;
            }

            BibiteID identity = body.id;
            if (identity == null || identity.id == 0)
            {
                return;
            }

            int entityId = identity.id;
            Edge edge = config.BorderEdge;
            Vector2 position = body.transform.position;

            crossing.Observe(entityId, edge, geometry, position, simTick);

            if (!geometry.InStrip(edge, position))
            {
                return;
            }

            if (exporter.IsInFlight(entityId) || importer.HasImmunity(entityId))
            {
                return;
            }

            if (exporter.IsBlocked(entityId, edge, geometry, position))
            {
                return;
            }

            if (!EdgeOpen)
            {
                return;
            }

            Rigidbody2D rigidbody = body.GetComponent<Rigidbody2D>();
            if (rigidbody == null)
            {
                return;
            }

            // m2_findings.md §4.4 guard 2 — only an organism actually leaving qualifies.
            if (Vector2.Dot(rigidbody.linearVelocity, BorderGeometry.OutwardNormal(edge)) <= 0f)
            {
                return;
            }

            exporter.TryBegin(body, rigidbody, edge, geometry);
        }

        internal void NoteDeparture(int entityId)
        {
            crossing.Forget(entityId);
            importer.Forget(entityId);
            exporter.Forget(entityId);
        }

        internal void NoteArrival(int entityId)
        {
            crossing.Forget(entityId);
            exporter.Forget(entityId);
        }

        /// <summary>
        /// Drop the export cooldown and the entry-immunity window for one organism. The rig's forced
        /// export (<see cref="DevCommands"/>) uses it so a second hop is not silently swallowed by the
        /// guards that exist to stop *natural* ping-pong. It relaxes no rule of the pipeline itself:
        /// the strip test, the outward-velocity test and the whole custody flow still run.
        /// </summary>
        internal void ClearMigrationBlocks(int entityId)
        {
            importer.Forget(entityId);
            exporter.Forget(entityId);
        }
    }
}
