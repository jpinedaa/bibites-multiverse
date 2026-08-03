using System;
using System.Collections.Generic;
using System.Globalization;
using Newtonsoft.Json;
using Newtonsoft.Json.Linq;
using OneUseScripts;
using SimulationScripts.BibiteScripts;
using UnityEngine;

namespace BibitesMultiverse
{
    /// <summary>
    /// The outbound half of the custody chain (contract-a.md §5.3, §5.5, §5.6, §6.3).
    ///
    /// One organism has at most one live migrationId at any time, and the mod enforces that — it is
    /// the mod-side half of at-most-once delivery (D2). An organism goes inert the moment MIGRATE_OUT
    /// is sent and stays inert until the message resolves: on ACK it is destroyed, on NACK it is
    /// revived, and on a timeout or a disconnect it stays inert with its migrationId bound to it, so
    /// the identical message can be replayed on the next connection.
    /// </summary>
    internal class MigrationExporter
    {
        internal class InFlight
        {
            internal string migrationId;
            internal int entityId;
            internal BibiteBody body;
            internal string frame;          // the exact MIGRATE_OUT frame, for an identical replay
            internal float sentAtRealtime;
            internal bool awaitingReconnect;
            internal bool timedOutLogged;
        }

        private class Cooldown
        {
            internal double untilSimTime;
            internal bool permanent;
            internal bool requireExit;
        }

        private readonly MultiverseClient client;
        private readonly Dictionary<string, InFlight> inFlight = new Dictionary<string, InFlight>();
        private readonly Dictionary<int, string> byEntity = new Dictionary<int, string>();
        private readonly Dictionary<int, Cooldown> cooldowns = new Dictionary<int, Cooldown>();
        private readonly List<string> scratch = new List<string>();

        internal MigrationExporter(MultiverseClient client)
        {
            this.client = client;
        }

        internal int InFlightCount => inFlight.Count;

        internal bool IsInFlight(int entityId)
        {
            return byEntity.ContainsKey(entityId);
        }

        internal void Clear()
        {
            inFlight.Clear();
            byEntity.Clear();
            cooldowns.Clear();
        }

        /// <summary>
        /// The strip test refuses an organism that is cooling down. The cooldown is released by time
        /// **and** by hysteresis — the organism has to clear the strip's inner face before it may
        /// trigger again, so a refused organism cannot spin against the border.
        /// </summary>
        internal bool IsBlocked(int entityId, Edge edge, BorderGeometry geometry, Vector2 position)
        {
            if (!cooldowns.TryGetValue(entityId, out Cooldown cooldown))
            {
                return false;
            }

            if (cooldown.permanent)
            {
                return true;
            }

            if (cooldown.requireExit)
            {
                if (!geometry.ClearOfStrip(edge, position))
                {
                    return true;
                }

                cooldown.requireExit = false;
            }

            if (TimeKeeper.simulatedTime < cooldown.untilSimTime)
            {
                return true;
            }

            cooldowns.Remove(entityId);
            return false;
        }

        internal void Forget(int entityId)
        {
            cooldowns.Remove(entityId);
        }

        // ---- send ---------------------------------------------------------------------------

        /// <summary>
        /// Freeze, serialize and send. Runs on the main thread, inside the BibiteBody.FixedUpdate
        /// postfix. Returns false and leaves the organism untouched when anything is not right.
        /// </summary>
        internal bool TryBegin(BibiteBody body, Rigidbody2D rigidbody, Edge edge, BorderGeometry geometry)
        {
            int entityId = (body.id != null) ? body.id.id : 0;
            if (entityId == 0)
            {
                return false;
            }

            if (byEntity.ContainsKey(entityId))
            {
                return false;
            }

            Vector2 position = body.transform.position;
            Vector2 velocity = rigidbody.linearVelocity;
            float heading = rigidbody.rotation;

            if (!ContractA.IsFinite(position.x) || !ContractA.IsFinite(position.y)
                || !ContractA.IsFinite(velocity.x) || !ContractA.IsFinite(velocity.y)
                || !ContractA.IsFinite(heading))
            {
                MultiversePlugin.Log.LogWarning(
                    $"[M2] entity={entityId} has a non-finite position/velocity/heading — NaN and Inf are forbidden on the wire (§4.1). Not migrating.");
                Block(entityId, permanent: true, seconds: 0f);
                return false;
            }

            string payload;
            try
            {
                JObject serialized = GameBridge.SerializeBibite(body.gameObject);
                if (serialized == null)
                {
                    MultiversePlugin.Log.LogWarning($"[M2] entity={entityId}: SerializeBibite returned null — not migrating.");
                    Block(entityId, permanent: true, seconds: 0f);
                    return false;
                }

                payload = GameBridge.StampVersion(serialized).ToString(Formatting.None);
            }
            catch (Exception e)
            {
                MultiversePlugin.Log.LogError($"[M2] entity={entityId}: serialization threw — {e}");
                Block(entityId, permanent: false, seconds: ContractA.MigrationCooldownSeconds);
                return false;
            }

            int payloadBytes = System.Text.Encoding.UTF8.GetByteCount(payload);
            if (payloadBytes > ContractA.MaxPayloadBytes)
            {
                MultiversePlugin.Log.LogError(
                    $"[M2] entity={entityId}: payload is {payloadBytes} bytes, over maxPayloadBytes ({ContractA.MaxPayloadBytes}) — not migrating.");
                Block(entityId, permanent: true, seconds: 0f);
                return false;
            }

            string migrationId = ContractA.NewUuid();
            float exitPosition = geometry.ExitPosition(edge, position);

            JObject data = new JObject
            {
                ["migrationId"] = migrationId,
                ["entityId"] = entityId,
                ["kind"] = ContractA.KindBibite,
                ["gameVersion"] = Application.version,
                ["payload"] = payload,
                ["exitEdge"] = ContractA.EdgeName(edge),
                ["exitPosition"] = exitPosition,
                ["velocity"] = new JObject { ["x"] = velocity.x, ["y"] = velocity.y },
                ["heading"] = heading,
                ["simulationSize"] = geometry.S,
                ["simTick"] = client.SimTick
            };

            // Inert before the frame goes out (§6.3): it cannot eat, breed, be eaten, move or be
            // seen as food while the sidecar decides.
            Freeze(body, rigidbody);

            InFlight record = new InFlight
            {
                migrationId = migrationId,
                entityId = entityId,
                body = body,
                frame = ContractA.Envelope(ContractA.TypeMigrateOut, data).ToString(Formatting.None),
                sentAtRealtime = Time.realtimeSinceStartup
            };

            inFlight[migrationId] = record;
            byEntity[entityId] = migrationId;
            client.SendFrame(record.frame);

            MultiversePlugin.Log.LogInfo(
                $"[M2] migrationId={migrationId} entityId={entityId} phase=MIGRATE_OUT_SENT edge={ContractA.EdgeName(edge)} " +
                $"exitPosition={exitPosition.ToString("F6", CultureInfo.InvariantCulture)} " +
                $"pos=({position.x:F2},{position.y:F2}) vel=({velocity.x:F2},{velocity.y:F2}) heading={heading:F2} " +
                $"payloadBytes={payloadBytes} payloadSha256={ContractA.Sha256Hex(payload)} S={geometry.S:F1}");
            return true;
        }

        /// <summary>§6.3 — re-send every unresolved MIGRATE_OUT, unchanged, on a fresh connection.</summary>
        internal void ReplayOnReconnect()
        {
            if (inFlight.Count == 0)
            {
                return;
            }

            foreach (KeyValuePair<string, InFlight> entry in inFlight)
            {
                InFlight record = entry.Value;
                record.sentAtRealtime = Time.realtimeSinceStartup;
                record.awaitingReconnect = false;
                record.timedOutLogged = false;
                client.SendFrame(record.frame);
                MultiversePlugin.Log.LogInfo(
                    $"[M2] migrationId={record.migrationId} entityId={record.entityId} phase=MIGRATE_OUT_REPLAY " +
                    "(identical frame, the organism stayed inert)");
            }
        }

        /// <summary>
        /// §6.3 — after migrateOutTimeoutMs the mod stops waiting, but it does **not** revive and it
        /// does **not** mint a new migrationId. Reviving here is how a duplicate is made.
        /// </summary>
        internal void CheckTimeouts()
        {
            float now = Time.realtimeSinceStartup;
            foreach (KeyValuePair<string, InFlight> entry in inFlight)
            {
                InFlight record = entry.Value;
                if (record.timedOutLogged || record.awaitingReconnect)
                {
                    continue;
                }

                if ((now - record.sentAtRealtime) * 1000f < ContractA.MigrateOutTimeoutMs)
                {
                    continue;
                }

                record.timedOutLogged = true;
                record.awaitingReconnect = true;
                MultiversePlugin.Log.LogWarning(
                    $"[M2] migrationId={record.migrationId} entityId={record.entityId} phase=MIGRATE_OUT_TIMEOUT " +
                    $"after {ContractA.MigrateOutTimeoutMs} ms — the organism stays inert and the id stays bound to it.");
            }
        }

        // ---- answers -------------------------------------------------------------------------

        /// <summary>
        /// §5.5 — custody moved. Destroy the organism: by migrationId when the record is known, and by
        /// entityId otherwise, which is the unsolicited custody-reassertion case of §7.4.
        /// </summary>
        internal void HandleAck(string migrationId, int entityId, bool unsolicited, long journaledAt)
        {
            if (migrationId != null && inFlight.TryGetValue(migrationId, out InFlight record))
            {
                inFlight.Remove(migrationId);
                byEntity.Remove(record.entityId);
                cooldowns.Remove(record.entityId);
                DestroyForCustody(record.body, record.entityId, migrationId, unsolicited ? "unsolicited-by-id" : "acked", journaledAt);
                return;
            }

            BibiteBody found = GameBridge.FindBodyById(entityId);
            if (found != null)
            {
                MultiversePlugin.Log.LogWarning(
                    $"[M2] migrationId={migrationId} entityId={entityId} phase=CUSTODY_ASSERTION unsolicited={unsolicited} — " +
                    "the sidecar holds custody but a local copy exists (a rollback resurrected it). Destroying it.");
                cooldowns.Remove(entityId);
                DestroyForCustody(found, entityId, migrationId, "custody-assertion", journaledAt);
                return;
            }

            MultiversePlugin.Log.LogInfo(
                $"[M2] migrationId={migrationId} entityId={entityId} phase=MIGRATE_OUT_ACK unsolicited={unsolicited} — " +
                "no in-flight record and no local copy. Custody already moved; nothing to do.");
        }

        private void DestroyForCustody(BibiteBody body, int entityId, string migrationId, string why, long journaledAt)
        {
            if (!GameBridge.IsUsable(body))
            {
                MultiversePlugin.Log.LogInfo(
                    $"[M2] migrationId={migrationId} entityId={entityId} phase=MIGRATE_OUT_ACK — the organism is already gone ({why}).");
                return;
            }

            try
            {
                // Capture first: a stale BibiteID left in a parent's children list makes the next
                // world save throw (m1_findings.md §4.3).
                OrganismLinks links = OrganismLinks.Capture(body);
                links.LogSummary();
                links.Detach();
            }
            catch (Exception e)
            {
                MultiversePlugin.Log.LogError($"[M2] migrationId={migrationId} entityId={entityId}: link detach threw — {e}");
            }

            bool delisted = GameBridge.DestroyCleanly(body);
            client.NoteDeparture(entityId);
            MultiversePlugin.Log.LogInfo(
                $"[M2] migrationId={migrationId} entityId={entityId} phase=DESTROYED reason={why} delisted={delisted} " +
                $"journaledAt={journaledAt} population={GameBridge.LivingPopulation()}");
        }

        /// <summary>§5.6 — the sidecar did not take custody. Revive, and apply the cooldown.</summary>
        internal void HandleNack(string migrationId, int entityId, string code, string nackClass, string message, int retryAfterMs, bool hasRetryAfter)
        {
            bool permanent = string.Equals(nackClass, ContractA.ClassPermanent, StringComparison.Ordinal);

            if (migrationId == null || !inFlight.TryGetValue(migrationId, out InFlight record))
            {
                MultiversePlugin.Log.LogWarning(
                    $"[M2] migrationId={migrationId} entityId={entityId} phase=MIGRATE_OUT_NACK code={code} class={nackClass} — " +
                    $"no in-flight record. {message}");
                return;
            }

            inFlight.Remove(migrationId);
            byEntity.Remove(record.entityId);

            bool revived = Revive(record.body);
            float seconds = ContractA.MigrationCooldownSeconds;
            if (hasRetryAfter && retryAfterMs > 0)
            {
                seconds = Mathf.Max(seconds, retryAfterMs / 1000f);
            }

            Block(record.entityId, permanent, seconds);

            // EDGE_CLOSED and SIM_SIZE_MISMATCH additionally stop this edge until a new EDGE_STATUS.
            if (string.Equals(code, "EDGE_CLOSED", StringComparison.Ordinal)
                || string.Equals(code, "SIM_SIZE_MISMATCH", StringComparison.Ordinal))
            {
                client.CloseEdgeUntilNextStatus(code);
            }

            MultiversePlugin.Log.LogWarning(
                $"[M2] migrationId={migrationId} entityId={record.entityId} phase=MIGRATE_OUT_NACK code={code} class={nackClass} " +
                $"revived={revived} cooldown={seconds:F1}s permanent={permanent} message=\"{message}\"");
        }

        private void Block(int entityId, bool permanent, float seconds)
        {
            cooldowns[entityId] = new Cooldown
            {
                permanent = permanent,
                untilSimTime = TimeKeeper.simulatedTime + seconds,
                requireExit = true
            };
        }

        // ---- the in-flight freeze (§6.3) -------------------------------------------------------

        private static void Freeze(BibiteBody body, Rigidbody2D rigidbody)
        {
            GameBridge.Untrack(body);
            body.enabled = false;           // stops BibiteBody.FixedUpdate: metabolism, brain, organs
            if (rigidbody != null)
            {
                rigidbody.simulated = false; // stops physics and the colliders, so it is not food either
            }
        }

        private static bool Revive(BibiteBody body)
        {
            if (!GameBridge.IsUsable(body))
            {
                return false;
            }

            Rigidbody2D rigidbody = body.GetComponent<Rigidbody2D>();
            if (rigidbody != null)
            {
                rigidbody.simulated = true;
            }

            body.enabled = true;
            GameBridge.Track(body);
            return true;
        }

        /// <summary>
        /// §6.4 — on world unload the mod closes with 1000 and **must not** try to resolve an
        /// in-flight migration first. The organism stays inert and the sidecar's journal decides what
        /// happens to it after the next handshake (§7.4). This only records what was left open.
        /// </summary>
        internal void NoteUnresolvedAtUnload()
        {
            if (inFlight.Count == 0)
            {
                return;
            }

            scratch.Clear();
            foreach (KeyValuePair<string, InFlight> entry in inFlight)
            {
                scratch.Add(entry.Key);
            }

            MultiversePlugin.Log.LogWarning(
                $"[M2] world unload with {scratch.Count} unresolved MIGRATE_OUT(s). The sidecar journal is authoritative and " +
                "reasserts custody after the next handshake (§6.4, §7.4).");

            foreach (string key in scratch)
            {
                InFlight record = inFlight[key];
                MultiversePlugin.Log.LogWarning(
                    $"[M2] migrationId={record.migrationId} entityId={record.entityId} phase=UNRESOLVED_AT_UNLOAD");
            }

            Clear();
        }
    }
}
