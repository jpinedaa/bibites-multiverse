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
        private Containment containment;
        private MigrationExporter exporter;
        private MigrationImporter importer;
        private WebSocketTransport transport;
        private PortalVisual portal;

        /// <summary>A frame that will spawn, destroy or revive an organism, waiting for FixedUpdate.</summary>
        private struct PendingMutation
        {
            internal string type;
            internal JObject data;
            internal float queuedAtRealtime;
        }

        /// <summary>
        /// The inbound mutations parsed out of the transport, waiting for FixedUpdate to apply them in
        /// arrival order.
        ///
        /// C4 — this list is bounded in practice and needs no cap of its own: the sidecar caps its un-ACKed
        /// inbound deliveries at 64 and paces their release (§7.5, §15 A20 and the parallel Go-side fix), so
        /// it cannot enqueue MIGRATE_INs faster than the mod ACKs them, and <see cref="ApplyMutations"/>
        /// drains this under a per-FixedUpdate budget (C1) far faster than that pacing refills it. Only
        /// MIGRATE_* frames ever land here; EDGE_STATUS and other control frames are applied straight out of
        /// <see cref="DrainTransport"/> and never queue.
        /// </summary>
        private readonly List<PendingMutation> pendingMutations = new List<PendingMutation>();

        // C1 (Slot-6 livelock) — inbound work is metered so one FixedUpdate and one Update can never spend
        // seconds on the main thread and starve the heartbeat into a 4004. The budgets are a wall-clock
        // ceiling and a count cap, whichever trips first; leftover work stays queued, strictly in order,
        // for the next frame. See ApplyMutations (FixedUpdate) and DrainTransport (Update).
        private const int MutationBudgetMs = 8;
        private const int MaxMutationsPerFixedUpdate = 4;
        private const int DrainBudgetMs = 2;
        private const int MaxFramesPerUpdate = 16;
        private readonly System.Diagnostics.Stopwatch mutationClock = new System.Diagnostics.Stopwatch();
        private readonly System.Diagnostics.Stopwatch drainClock = new System.Diagnostics.Stopwatch();

        private bool armed;
        private string sessionId;
        private long simTick;
        private long lastEdgeStatusEpoch;

        /// <summary>
        /// §5.4, §11.2, §15 A18 — the applied state of **every** declared export edge, in one immutable
        /// object. It is replaced whole by <see cref="ApplyEdgeStatus"/> in Update and read once per
        /// tick by <see cref="OnBodyTick"/>, because the corner rule (§4.3.2) needs both edges' `open`
        /// flags inside one FixedUpdate and a frame that lands mid-tick must not change the answer half
        /// way through the organism loop.
        /// </summary>
        private EdgeStates edgeStates;

        private bool handshakeSent;

        /// <summary>
        /// The transport generation this client handshook on (§5.1). A socket that has re-opened but
        /// whose <c>Connected</c> event has not been drained yet is a different connection, and
        /// sending a heartbeat into it puts a non-CONFIG_UPDATE frame first, which is close 4003.
        /// </summary>
        private int handshakeGeneration = -1;
        private float nextHeartbeatRealtime;
        private bool haltedReported;

        /// <summary>
        /// The dev channel's local override of the EDGE_STATUS gate, per edge (<see cref="DevCommands"/>,
        /// verb <c>edge</c>). It exists for the D10 verifications and the M4 portal check, which
        /// exercise the capture bands with no sidecar in the rig: without it every export edge is
        /// closed and <see cref="OnBodyTick"/> can never reach <see cref="MigrationExporter.TryBegin"/>,
        /// so a capture could not be observed at all. It relaxes nothing else — the bands, the
        /// outward-velocity test, the corner rule, the in-flight rule and the whole custody flow still
        /// run — and a real EDGE_STATUS clears it.
        /// </summary>
        private readonly bool[] devEdgeOverride = new bool[5];

        internal MultiverseConfig Config => config;
        internal BorderGeometry Geometry => geometry;
        internal Containment Containment => containment;
        internal MigrationExporter Exporter => exporter;
        internal MigrationImporter Importer => importer;
        internal long SimTick => simTick;

        /// <summary>True while the socket is up and the handshake for **this** connection has gone out.</summary>
        internal bool Connected => transport != null && transport.IsConnected && handshakeSent && transport.Generation == handshakeGeneration;

        /// <summary>
        /// §5.4 — one export edge's gate. Until the mod has applied its first EDGE_STATUS, and whenever
        /// it is disconnected, this is false for every edge: a mod that cannot reach its sidecar quietly
        /// stops migrating instead of losing organisms.
        /// </summary>
        internal bool IsEdgeOpen(Edge edge)
        {
            if (edge == Edge.None || !config.Exports(edge))
            {
                return false;
            }

            if (devEdgeOverride[(int)edge])
            {
                return true;
            }

            return edgeStates != null && edgeStates.IsOpen(edge) && transport != null && transport.IsConnected;
        }

        /// <summary>Every declared export edge and its gate, for a log line or a command result.</summary>
        internal string EdgeSummary()
        {
            System.Text.StringBuilder text = new System.Text.StringBuilder();
            for (int i = 0; i < config.ExportEdges.Count; i++)
            {
                Edge edge = config.ExportEdges[i];
                if (i > 0)
                {
                    text.Append(' ');
                }

                text.Append(ContractA.EdgeName(edge)).Append(':').Append(IsEdgeOpen(edge) ? "open" : "closed");
                if (devEdgeOverride[(int)edge])
                {
                    text.Append("(dev)");
                }
            }

            return text.ToString();
        }

        internal void Initialize(MultiverseConfig configuration)
        {
            config = configuration;
            geometry = new BorderGeometry(config, OnSimulationSizeChanged);
            worldSettings = new WorldSettings();
            crossing = new CrossingStats();
            containment = new Containment();
            exporter = new MigrationExporter(this);
            importer = new MigrationImporter(this);
            transport = new WebSocketTransport(config.Url);
            edgeStates = EdgeStates.AllClosed(config.ExportEdges, 0, ContractA.ReasonNoPeer);

            if (config.Portal)
            {
                try
                {
                    portal = gameObject.AddComponent<PortalVisual>();
                    portal.Initialize(config, geometry, IsEdgeOpen, () => Connected);
                }
                catch (Exception e)
                {
                    // ShapesRuntime.dll ships with the game, so this only fires if a game update moved
                    // it. The migration path must not care.
                    portal = null;
                    MultiversePlugin.Log.LogError(
                        $"{PortalVisual.Prefix} the portal could not be created and is off for this session — {e}");
                }
            }

            // Risk 3, rule 2: never start a save while a MIGRATE_IN waits in the mod's queue.
            if (WorldSaver.Instance != null)
            {
                WorldSaver.Instance.DeferWhile = () => PendingInboundCount() > 0;
            }

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
                crossing.Tick(config, geometry, GameBridge.LivingPopulation(), simTick);
                containment.Prune(simTick);
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
            edgeStates = EdgeStates.AllClosed(config.ExportEdges, 0, ContractA.ReasonNoPeer);
            handshakeSent = false;
            handshakeGeneration = -1;
            haltedReported = false;
            Array.Clear(devEdgeOverride, 0, devEdgeOverride.Length);
            pendingMutations.Clear();
            exporter.Clear();
            importer.Clear();
            crossing.Reset();
            containment.Reset();

            geometry.Subscribe();
            worldSettings.LogBoundarySettings(geometry.S);
            worldSettings.LogBibiteHolderTransform();
            worldSettings.Apply();
            Containment.LogSetup(geometry, config.ExportEdges, config.EntryEdges);
            portal?.OnWorldLoaded();

            MultiversePlugin.Log.LogInfo(
                $"{Prefix} world loaded — sessionId={sessionId} exportEdges=[{ContractA.EdgeNames(config.ExportEdges)}] " +
                $"entryEdges=[{ContractA.EdgeNames(config.EntryEdges)}](passive) " +
                $"borderEdges=[{ContractA.EdgeNames(config.BorderEdges)}] " +
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
            portal?.OnWorldUnloaded();

            transport.Stop(ContractA.CloseNormal, why);

            edgeStates = EdgeStates.AllClosed(config.ExportEdges, 0, ContractA.ReasonNoPeer);
            handshakeSent = false;
            handshakeGeneration = -1;
            armed = false;
            Active = null;

            MultiversePlugin.Log.LogInfo(
                $"{CrossingStats.Prefix} session summary: totalStripEntries={crossing.TotalStripEntries} " +
                $"totalCrossings={crossing.TotalCrossings}");

            if (portal != null)
            {
                MultiversePlugin.Log.LogInfo($"{PortalVisual.Prefix} session summary: {portal.Summary()}");
            }
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

            // The portal is laid out from the same S the capture band is tested against, so it follows
            // a live change rather than drifting away from the rule it draws.
            portal?.Relayout();
        }

        // ---- transport drain ---------------------------------------------------------------------

        private void DrainTransport()
        {
            if (transport == null)
            {
                return;
            }

            // C1 (Slot-6 livelock) — parse at most a bounded number of frames per Update. Each MIGRATE_IN
            // is a ~10 kB JSON envelope parse, and draining a replayed backlog of 64 in one Update spent
            // long enough on the main thread to delay the heartbeat. Leftover frames stay in the transport's
            // inbound queue, strictly in arrival order, for the next Update, and PendingInboundCount() counts
            // them so §5.2 pendingIn stays honest. Cheap control events (Connected / Disconnected / Log) are
            // never metered: they parse no payload, and Connected must reach the handshake promptly. Because
            // events are drained in arrival order, a control event can only ever sit behind frames of the
            // SAME or an earlier connection, and transport.IsConnected / Generation are already correct
            // before their Disconnected / Connected event is drained, so PumpHeartbeat is unaffected.
            drainClock.Restart();
            int frames = 0;
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
                        frames++;
                        if (frames >= MaxFramesPerUpdate || drainClock.ElapsedMilliseconds >= DrainBudgetMs)
                        {
                            // Stop after a whole frame, never mid-frame. The rest wait, in order, for the
                            // next Update — a small, bounded control-frame latency (worst case one Update).
                            return;
                        }

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
            edgeStates = EdgeStates.AllClosed(config.ExportEdges, 0, ContractA.ReasonNoPeer);

            // C2 (§7.3, §7.4) — the dedup ledger is keyed to the world identity, not the socket. This
            // reconnect carries the same sessionId (§6.3), so the ledger is kept: the sidecar's replayed
            // un-ACKed deliveries then hit the O(1) dedup key instead of re-running the full spawn, which
            // is the mod side of the Slot-6 livelock fix. Only a new sessionId — a world load or rollback —
            // clears it, because §7.4 custody reassertion keys off exactly that change.
            importer.OnHandshake(sessionId);

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
            string wasOpen = edgeStates.Describe(config.ExportEdges);
            edgeStates = EdgeStates.AllClosed(config.ExportEdges, 0, "disconnected");

            bool fatal = closeCode == ContractA.CloseProtocolUnsupported
                || closeCode == ContractA.CloseSlotMismatch
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

            MultiversePlugin.Log.LogInfo($"{Prefix} exportEdges were {wasOpen} — every one is now closed, reason=disconnected");
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

            // C1 (Slot-6 livelock) — apply pending mutations in arrival order, but stop when the wall-clock
            // budget or the count cap is hit, whichever comes first. Each MIGRATE_IN can cost a JSON parse,
            // a prefab instantiate, a brain-graph build and an O(N) world scan; applying a replayed backlog
            // of 64 in one FixedUpdate spent 2.5-4 s on the main thread, so Update never ran, no HEARTBEAT
            // was enqueued, and the sidecar closed 4004 (§5.7 permits a slower drain — the backlog is
            // reported honestly by pendingIn). Leftover entries stay queued, in order, for the next
            // FixedUpdate.
            //
            // Forward progress is guaranteed: the budget is tested only AFTER an entry is applied, so at
            // least one is always processed. A single pathological organism therefore costs exactly one
            // FixedUpdate and can never wedge the queue behind it.
            mutationClock.Restart();
            int applied = 0;
            while (applied < pendingMutations.Count)
            {
                PendingMutation entry = pendingMutations[applied];
                applied++;

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

                if (applied >= MaxMutationsPerFixedUpdate || mutationClock.ElapsedMilliseconds >= MutationBudgetMs)
                {
                    break;
                }
            }

            // Nothing appends to pendingMutations during this loop (DrainTransport and RejectWhilePaused
            // both run in Update, not FixedUpdate), so the first `applied` entries are exactly the ones
            // just handled, in order. Drop them and leave the rest for the next FixedUpdate.
            pendingMutations.RemoveRange(0, applied);
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
                if ((now - entry.queuedAtRealtime) * 1000f < ContractA.SimNotReadyGraceMs
                    || !string.Equals(entry.type, ContractA.TypeMigrateIn, StringComparison.Ordinal))
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

        // ---- EDGE_STATUS (§5.4, §14 A11) -----------------------------------------------------------

        /// <summary>
        /// Full state, not a delta, and under the grid it carries **one entry for each declared export
        /// edge** (§15, A18). The entry edges never appear here: they are passive, they always accept an
        /// inbound organism, and they have no capture band at all (§4.3.1). An empty <c>edges</c> array
        /// closes **every** export edge and is the correct frame when the sidecar holds no slot.
        ///
        /// Three rules, all of which follow from "full state":
        ///
        /// * <b>A declared export edge with no entry is CLOSED.</b> Absence is not "no change". That is
        ///   the fail-safe direction, and it is why the snapshot starts closed.
        /// * <b>An entry for an edge this mod did not declare is ignored</b>, with one logged warning
        ///   and no close. It is a forward-compatible shape under a later map extension, not a fault.
        /// * <b>The whole frame is applied atomically.</b> One snapshot is built and published with one
        ///   reference assignment, because a corner capture reads both edges in one FixedUpdate
        ///   (§4.3.2) and a half-applied frame could export an organism through an edge that just
        ///   closed.
        /// </summary>
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

            // Full state: every declared export edge starts closed, and only an entry reopens it.
            EdgeStates next = EdgeStates.AllClosed(config.ExportEdges, epoch, ContractA.ReasonNoPeer);
            int applied = 0;
            int ignored = 0;
            int seenMask = 0;

            foreach (JToken token in edges)
            {
                if (!(token is JObject entry))
                {
                    ignored++;
                    continue;
                }

                if (!ContractA.TryString(entry, "edge", out string edgeText) || !ContractA.TryParseEdge(edgeText, out Edge edge))
                {
                    ignored++;
                    continue;
                }

                if (!config.Exports(edge))
                {
                    ignored++;
                    MultiversePlugin.Log.LogWarning(
                        $"{Prefix} EDGE_STATUS carries edge {edgeText}, which this sim did not declare in exportEdges " +
                        $"([{ContractA.EdgeNames(config.ExportEdges)}]) — ignored, and the connection stays open (§15, A18).");
                    continue;
                }

                if ((seenMask & (1 << (int)edge)) != 0)
                {
                    // §5.4 — a duplicate `edge` is a sidecar defect: apply the FIRST and log one warning.
                    ignored++;
                    MultiversePlugin.Log.LogWarning(
                        $"{Prefix} EDGE_STATUS carries edge {edgeText} twice — the first entry is applied and this one is ignored.");
                    continue;
                }

                seenMask |= 1 << (int)edge;
                applied++;

                ContractA.TryBool(entry, "open", out bool open);
                if (!ContractA.TryString(entry, "reason", out string reason))
                {
                    reason = open ? ContractA.ReasonPeerLive : ContractA.ReasonNoPeer;
                }

                bool peerSizeKnown = ContractA.TryFloat(entry, "peerSimulationSize", out float peerSize);

                // §5.4 — the mod compares peerSimulationSize against its own S and treats **that** edge
                // as closed on a mismatch, even though the sidecar already checked. Two independent
                // checks, because a mid-run resize can race, and the two edges are different peers so
                // the check is per edge. §13 A10 gives the one relative test both sides use.
                if (open && peerSizeKnown && !ContractA.SimulationSizeEqual(peerSize, geometry.S))
                {
                    MultiversePlugin.Log.LogWarning(
                        $"{Prefix} EDGE_STATUS says export edge {edgeText} is open, but that peer's S is " +
                        $"{peerSize:F2} and this sim's S is {geometry.S:F2} — holding that edge closed.");
                    open = false;
                    reason = ContractA.ReasonSimSizeMismatch;
                }

                next.Set(edge, open, reason, peerSizeKnown, peerSize);

                // §14 A11 added this reason, and it is the one an operator can act on: the neighbour's
                // sidecar answered, so the map is wired correctly — the neighbour's *game* is not running.
                if (!open && string.Equals(reason, ContractA.ReasonPeerModAbsent, StringComparison.Ordinal))
                {
                    MultiversePlugin.Log.LogWarning(
                        $"{Prefix} export edge {edgeText} is closed because that neighbour's sidecar is live but has no mod " +
                        "connected — that peer's game is not running, or its world is not loaded. Nothing is wrong on this " +
                        "side; migration resumes when the neighbour loads a world.");
                }
            }

            string before = edgeStates.Describe(config.ExportEdges);

            // One reference assignment: the frame lands whole or not at all.
            edgeStates = next;

            ClearDevOverride("a real EDGE_STATUS always wins over it");

            MultiversePlugin.Log.LogInfo(
                $"{Prefix} EDGE_STATUS epoch={epoch} entries={edges.Count} applied={applied} ignored={ignored} " +
                $"before=[{before}] after=[{next.Describe(config.ExportEdges)}] " +
                $"(entryEdges [{ContractA.EdgeNames(config.EntryEdges)}] are passive and never appear here)");

            if (applied < config.ExportEdges.Count)
            {
                MultiversePlugin.Log.LogInfo(
                    $"{Prefix} EDGE_STATUS epoch={epoch} named {applied} of this sim's {config.ExportEdges.Count} declared export " +
                    "edge(s). A declared edge with no entry is CLOSED — the frame is the whole state, not a delta (§15, A18).");
            }
        }

        /// <summary>
        /// The dev channel's local override of an export-edge gate. Loud on purpose: it is a test
        /// affordance for the D10 verifications and the portal check, not a mode anyone should run a
        /// rig in. <c>Edge.None</c> means every declared export edge.
        /// </summary>
        internal void SetDevEdgeOverride(Edge edge, bool value)
        {
            if (edge == Edge.None)
            {
                foreach (Edge declared in config.ExportEdges)
                {
                    devEdgeOverride[(int)declared] = value;
                }
            }
            else
            {
                devEdgeOverride[(int)edge] = value;
            }

            string which = (edge == Edge.None) ? "[" + ContractA.EdgeNames(config.ExportEdges) + "]" : ContractA.EdgeName(edge);
            if (value)
            {
                MultiversePlugin.Log.LogWarning(
                    $"{Prefix} DEV OVERRIDE: export edge {which} is forced OPEN with no EDGE_STATUS behind it. Every other " +
                    "guard still applies — the band, the outward-velocity test and the corner rule — and the next real " +
                    "EDGE_STATUS clears this.");
            }
            else
            {
                MultiversePlugin.Log.LogWarning(
                    $"{Prefix} DEV OVERRIDE cleared for {which} — that edge is back under EDGE_STATUS control.");
            }
        }

        private void ClearDevOverride(string why)
        {
            bool any = false;
            for (int i = 0; i < devEdgeOverride.Length; i++)
            {
                if (devEdgeOverride[i])
                {
                    devEdgeOverride[i] = false;
                    any = true;
                }
            }

            if (any)
            {
                MultiversePlugin.Log.LogWarning($"{Prefix} the dev edge override is cleared — {why}.");
            }
        }

        /// <summary>
        /// §9.1, §15 A22 — a transient NACK closes **only the edge the organism left by**, until a new
        /// EDGE_STATUS. The NACK carries no <c>edge</c> field and does not need one: the exporter's
        /// in-flight record holds it. Closing every edge because one NACK arrived throws away a lane
        /// that is open, and that is the bug the amendment exists to name.
        /// </summary>
        internal void CloseEdgeUntilNextStatus(Edge edge, string code)
        {
            if (edge == Edge.None || !config.Exports(edge))
            {
                return;
            }

            devEdgeOverride[(int)edge] = false;
            if (!edgeStates.IsOpen(edge))
            {
                return;
            }

            edgeStates = edgeStates.WithEdgeClosed(edge, "local_" + code.ToLowerInvariant());
            MultiversePlugin.Log.LogWarning(
                $"{Prefix} export edge {ContractA.EdgeName(edge)} closed locally after {code}; it reopens only on a new " +
                $"EDGE_STATUS. Every other declared edge is untouched — now [{edgeStates.Describe(config.ExportEdges)}].");
        }

        // ---- outbound ------------------------------------------------------------------------------

        internal void SendFrame(string frame)
        {
            transport?.Send(frame);
        }

        /// <summary>
        /// §5.1, §15 A18. <c>borderEdges</c> is the set of edges that **accept** an inbound organism —
        /// all four under the grid — and <c>exportEdges</c>, an array, is the set this sim may export
        /// through. The singular <c>exportEdge</c> is **removed** in contract-a/2.0 and is not sent:
        /// there is no fallback, because a peer that speaks contract-a/1.x is rejected at the version
        /// check (close 4000) long before a field-level fallback could matter. <c>sector</c> stays
        /// retired, and <c>ringSlot</c> is the advisory integer that replaced it.
        ///
        /// The declaration is about **geometry, not topology**: it says "I run a capture band on these
        /// edges". Whether an edge has a lane is the sidecar's answer in EDGE_STATUS, and the mod never
        /// learns a coordinate, a neighbour or a map shape.
        /// </summary>
        private void SendConfigUpdate(string reason)
        {
            JObject data = new JObject
            {
                ["sessionId"] = sessionId,
                ["reason"] = reason,
                ["gameVersion"] = Application.version,
                ["modVersion"] = MultiversePlugin.Version,
                ["simulationSize"] = geometry.S,
                ["borderEdges"] = ContractA.EdgeArray(config.BorderEdges),
                ["exportEdges"] = ContractA.EdgeArray(config.ExportEdges),
                ["borderWidth"] = geometry.W
            };

            if (config.HasRingSlot)
            {
                data["ringSlot"] = config.RingSlot;
            }

            string worldName = SimulationManager.gameName;
            if (!string.IsNullOrEmpty(worldName))
            {
                data["worldName"] = worldName;
            }

            SendFrame(ContractA.Envelope(ContractA.TypeConfigUpdate, data).ToString(Formatting.None));
            MultiversePlugin.Log.LogInfo(
                $"{Prefix} CONFIG_UPDATE reason={reason} protocol={ContractA.Protocol} sessionId={sessionId} " +
                $"ringSlot={(config.HasRingSlot ? config.RingSlot.ToString(CultureInfo.InvariantCulture) : "<omitted>")} " +
                $"S={geometry.S.ToString("F2", CultureInfo.InvariantCulture)} W={geometry.W.ToString("F2", CultureInfo.InvariantCulture)} " +
                $"borderEdges=[{ContractA.EdgeNames(config.BorderEdges)}] " +
                $"exportEdges=[{ContractA.EdgeNames(config.ExportEdges)}] " +
                $"worldWrapping={worldSettings.WorldWrappingOn} (reported, never written — D10)");
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
                ["pendingIn"] = PendingInboundCount()
            };

            // §5.2, §15 A21 — the OPTIONAL save receipt. It is the only wire path the operator surface
            // has to a world's save state, and it exists because a sidecar on the second machine has no
            // file the archive can read. Absent until this mod has written a save, and absent forever
            // from a mod that does not save: an honest gap, never a zero.
            SaveReceipt lastSave = (WorldSaver.Instance != null) ? WorldSaver.Instance.LastSave : null;
            if (lastSave != null)
            {
                data["lastSave"] = lastSave.ToJson();
            }

            // §5.2, §17 A35 — the OPTIONAL species census. It is built HERE, inline, and not on a timer
            // of its own: A35's "one sample" rule says the census and the counters above it must
            // describe one instant, because the page reconciles Σbibites against `population` and
            // Σeggs against `eggCount`. A cached copy refreshed on a slower clock would break that
            // reconciliation and report a surplus this mod never had.
            //
            // Absent is UNKNOWN, not empty (A35). A null census — no world, one still loading — omits
            // the field entirely, and every reader from the sidecar to the page renders that world's
            // species as unknown. An empty array is the stronger claim: this world is reporting, and
            // nothing is alive in it.
            JArray census = SpeciesCensus.Build(out bool censusTruncated);
            if (census != null)
            {
                data["species"] = census;
                if (censusTruncated)
                {
                    // Qualifies `species` and nothing else, and it is monotonic from here: every hop may
                    // set it, none may clear it. Omitted rather than sent as false, because absent and
                    // false say the same thing.
                    data["truncated"] = true;
                }
            }

            SendFrame(ContractA.Envelope(ContractA.TypeHeartbeat, data).ToString(Formatting.None));
        }

        /// <summary>§5.2 <c>pendingIn</c> — MIGRATE_INs queued in the mod but not yet spawned.</summary>
        private int PendingInboundCount()
        {
            int count = 0;
            for (int i = 0; i < pendingMutations.Count; i++)
            {
                if (string.Equals(pendingMutations[i].type, ContractA.TypeMigrateIn, StringComparison.Ordinal))
                {
                    count++;
                }
            }

            // C1 — DrainTransport now parses only a few frames per Update, so inbound MIGRATE_INs can still
            // be sitting in the transport's queue, not yet parsed into pendingMutations. §5.2 pendingIn MUST
            // report the whole not-yet-applied backlog, or the sidecar's pacing over-sends into a queue it
            // believes is short — the exact pressure behind the Slot-6 flood. QueuedFrameCount is a small
            // over-estimate (a rarely-queued EDGE_STATUS frame counts too), which is the safe way to round:
            // it makes the sidecar pace slightly more conservatively, never less.
            if (transport != null)
            {
                count += transport.QueuedFrameCount;
            }

            return count;
        }

        // ---- the capture band (called from the Harmony postfix) --------------------------------------

        /// <summary>
        /// One organism, one tick, across **every declared export band**. Every guard the early returns
        /// of BibiteBody.FixedUpdate imply is re-applied here, because a postfix runs on every return
        /// path (m2_findings.md §(b)).
        ///
        /// The capture rule is contract-a.md §4.3.1 and §4.3.2, and every clause is load-bearing:
        ///
        /// <code>
        /// inBand(o, E)  ⇔  x ≥ S − W  ∧  velocity.x > 0
        /// inBand(o, N)  ⇔  y ≥ S − W  ∧  velocity.y > 0
        ///
        /// candidates    =  { e ∈ exportEdges : inBand(o, e) ∧ open(e) }
        /// |candidates| = 0 → no export this tick
        /// |candidates| = 1 → that edge
        /// |candidates| = 2 → "E" when velocity.x ≥ velocity.y, otherwise "N"
        /// </code>
        ///
        /// The band is half-open and **outward-unbounded**: it starts at the strip line and runs past
        /// `S`, past the wrap radius, to any coordinate — because a fast organism clears the whole strip
        /// in one FixedUpdate, and M2's inside-only rule then let the vanilla wrap teleport it to the
        /// antipode (m3_considerations.md Risk 2).
        ///
        /// The cadence is what makes the band beat the wrap: this runs every FixedUpdate, inside the
        /// organism's own tick, because the wrap fires from <c>BibitePropulsion.UpdateOrgan</c> a few
        /// lines earlier in that same tick (BibiteBody.cs:613-616). A capture test on a coroutine or a
        /// once-per-second timer loses the race for a fast organism.
        ///
        /// The outward-velocity test is what separates an export from a wrap. A wrapped organism
        /// arrives in the outer band on the **far** side and travels inward, so it fails the test.
        /// Remove it and every wrapped arrival immediately exports through the edge it landed behind
        /// (m3_considerations.md Risk 1).
        ///
        /// **Every band is evaluated; the loop never breaks on the first match** (§11.2). A first-match
        /// loop silently implements "whichever edge I wrote first wins", which is the exact defect the
        /// corner rule exists to prevent, and it would not show up until an organism reached a corner at
        /// speed. The tie is folded into the declaration order — <c>ExportEdges</c> is canonicalized to
        /// E, N, W, S — so <c>outward > best</c> keeps `E` on an exact tie with no separate branch, which
        /// is §4.3.2's rule and its deliberate absence of an epsilon.
        ///
        /// The entry edges are not tested at all: they are passive, and an organism that walks out
        /// through one is not a migrant — the vanilla wrap returns it (D10, §14 A11, §15 A19).
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
            Vector2 position = body.transform.position;

            // §11.2 — read the edge state **once** per tick. EDGE_STATUS is applied in Update, and the
            // corner decision below has to see both edges as of the same instant.
            EdgeStates states = edgeStates;

            crossing.Observe(entityId, config, geometry, position, simTick);
            containment.Observe(body, entityId, config.ExportEdges, geometry, position, simTick);

            // Cheap geometric pre-filter: the overwhelming majority of organisms are in no band at all,
            // and this keeps the per-organism cost at one comparison per declared edge.
            bool anyBand = false;
            for (int i = 0; i < config.ExportEdges.Count; i++)
            {
                if (geometry.InCaptureBand(config.ExportEdges[i], position))
                {
                    anyBand = true;
                    break;
                }
            }

            if (!anyBand)
            {
                return;
            }

            // §6.3 — one organism has at most one live migrationId. §5.7 step 6 — the entry-immunity
            // window is keyed on entityId and covers **both** bands.
            if (exporter.IsInFlight(entityId) || importer.HasImmunity(entityId))
            {
                return;
            }

            if (exporter.IsBlocked(entityId, geometry, position))
            {
                return;
            }

            Rigidbody2D rigidbody = body.GetComponent<Rigidbody2D>();
            if (rigidbody == null)
            {
                return;
            }

            Vector2 velocity = rigidbody.linearVelocity;

            Edge chosen = Edge.None;
            float best = 0f;

            for (int i = 0; i < config.ExportEdges.Count; i++)
            {
                Edge edge = config.ExportEdges[i];
                if (!geometry.InCaptureBand(edge, position))
                {
                    continue;
                }

                // §4.3.1 — REQUIRED everywhere in the band, not only outside the square.
                float outward = BorderGeometry.OutwardComponent(edge, velocity);
                if (outward <= 0f)
                {
                    containment.NoteBandDecision(
                        "BAND_INWARD_REFUSED", entityId, edge, geometry, position, velocity, IsEdgeOpen(edge), simTick, throttle: true);
                    continue;
                }

                // §4.3.2 — openness filters **before** the tie-break. An organism in both bands with only
                // the north lane open exports north, and is never pinned against a closed corner while a
                // live lane exists.
                if (!(devEdgeOverride[(int)edge] || (states.IsOpen(edge) && transport != null && transport.IsConnected)))
                {
                    containment.NoteBandDecision(
                        "BAND_WITHHELD_EDGE_CLOSED", entityId, edge, geometry, position, velocity, false, simTick, throttle: true);
                    continue;
                }

                if (chosen == Edge.None || outward > best)
                {
                    chosen = edge;
                    best = outward;
                }
            }

            if (chosen == Edge.None)
            {
                return;
            }

            containment.NoteBandDecision("BAND_CAPTURE", entityId, chosen, geometry, position, velocity, true, simTick, throttle: false);

            // Exactly one MIGRATE_OUT per organism per tick. A mod that emits two frames for one
            // organism in a corner is defective (§4.3.2).
            exporter.TryBegin(body, rigidbody, chosen, geometry);
        }

        internal void NoteDeparture(int entityId)
        {
            crossing.Forget(entityId);
            containment.Forget(entityId);
            importer.Forget(entityId);
            exporter.Forget(entityId);
        }

        internal void NoteArrival(int entityId, Edge entryEdge, float entryPosition)
        {
            crossing.Forget(entityId);
            crossing.NoteArrival(entryEdge, entryPosition);
            containment.Forget(entityId);
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

            // A rig teleport is not a wrap. Drop the tracked position too, or the next tick reports the
            // jump as a [M3-D10] WRAP event and the containment evidence becomes unreadable.
            containment.Forget(entityId);
        }
    }
}
