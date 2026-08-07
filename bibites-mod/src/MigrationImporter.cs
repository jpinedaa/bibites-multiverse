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
    /// The inbound half of the custody chain (contract-a.md §5.7, §5.8, §5.9, §7.3).
    ///
    /// Every MIGRATE_IN is answered — a silent drop becomes a re-delivery, a longer journal and a
    /// slower kill test. Deduplication has two keys: migrationId in memory for this connection, and
    /// entityId scanned out of the live world, which is the check that survives a game restart.
    /// </summary>
    internal class MigrationImporter
    {
        private readonly MultiverseClient client;

        /// <summary>
        /// §7.3 — the migrationId dedup ledger. C2 (Slot-6 livelock): it is keyed to the world identity,
        /// not the socket. §7.3 lets it live only "for the current connection"; this mod keeps it for the
        /// whole world **session**, which is stricter (loss-not-duplication is preserved) and is what makes
        /// a socket reconnect's replayed backlog an O(1) dedup hit instead of a full re-spawn. It is a
        /// HashSet, so it grows for the life of the session — roughly 10k migrationId strings a day at the
        /// worst observed rate, about a megabyte a day, and a world load or rollback (a new sessionId, see
        /// <see cref="OnHandshake"/>) empties it. That is deliberately not capped: the durable backstop for
        /// an evicted entry would be the entityId world scan (§7.3), so a cap would trade bounded memory
        /// for a reintroduced duplication window, the wrong direction for D2.
        /// </summary>
        private readonly HashSet<string> seenMigrations = new HashSet<string>(StringComparer.OrdinalIgnoreCase);
        private readonly Dictionary<int, double> immunityUntilSimTime = new Dictionary<int, double>();

        /// <summary>
        /// §5.7 step 3(a), §16 A31 — this world's own species registry, seen by name. It holds the
        /// per-tick resolve cache, so it is one instance for the life of the importer and it is cleared
        /// with everything else on a world unload.
        /// </summary>
        private readonly SpeciesRegistry species = new SpeciesRegistry();

        /// <summary>
        /// C2 (§7.3, §7.4) — the world identity the ledger above belongs to. <see cref="OnHandshake"/>
        /// clears the ledger only when this changes, which is exactly when §7.4 custody reassertion wants
        /// it gone.
        /// </summary>
        private string ledgerSessionId;

        internal MigrationImporter(MultiverseClient client)
        {
            this.client = client;
        }

        internal void Clear()
        {
            seenMigrations.Clear();
            immunityUntilSimTime.Clear();
            species.Clear();
            ledgerSessionId = null;
        }

        /// <summary>
        /// C2 (§7.3, §7.4) — called on every handshake with the world's current sessionId. A plain socket
        /// reconnect keeps the same sessionId (§6.3), so the dedup ledger survives it: without this, the
        /// old <c>ClearConnectionState</c> wiped the ledger on every reconnect, and the sidecar's replayed
        /// 64 un-ACKed deliveries then missed the O(1) key and re-ran the full spawn, stalling the main
        /// thread into the next 4004 — the Slot-6 livelock. The ledger is cleared only when the sessionId
        /// actually changes: a world load or rollback, which is where §7.4 reasserts custody.
        /// </summary>
        internal void OnHandshake(string sessionId)
        {
            if (string.Equals(ledgerSessionId, sessionId, StringComparison.Ordinal))
            {
                return;
            }

            if (ledgerSessionId != null && seenMigrations.Count > 0)
            {
                MultiversePlugin.Log.LogInfo(
                    $"[M2] world identity changed (sessionId {ledgerSessionId} -> {sessionId}) — clearing the " +
                    $"{seenMigrations.Count}-entry migrationId dedup ledger (§7.4).");
            }

            seenMigrations.Clear();
            ledgerSessionId = sessionId;
        }

        /// <summary>m2_findings.md §4.4 guard 3 — a freshly arrived organism cannot re-trigger the strip.</summary>
        internal bool HasImmunity(int entityId)
        {
            if (!immunityUntilSimTime.TryGetValue(entityId, out double until))
            {
                return false;
            }

            if (TimeKeeper.simulatedTime < until)
            {
                return true;
            }

            immunityUntilSimTime.Remove(entityId);
            return false;
        }

        internal void Forget(int entityId)
        {
            immunityUntilSimTime.Remove(entityId);
        }

        /// <summary>
        /// Process one MIGRATE_IN. Main thread only, from the client's FixedUpdate. Always answers.
        /// </summary>
        internal void Handle(JObject data)
        {
            string migrationId = null;
            int entityId = 0;

            if (!ContractA.TryMigrationId(data, out migrationId))
            {
                MultiversePlugin.Log.LogError(
                    "[M2] MIGRATE_IN without a usable 'migrationId' — the NACK is keyed on that field, so this frame cannot be answered. Dropped (§13, amendment A2).");
                return;
            }

            if (!ContractA.TryInt(data, "entityId", out entityId))
            {
                Nack(migrationId, 0, ContractA.NackMalformedMessage, ContractA.ClassPermanent, "'entityId' is missing or not an int32", 0);
                return;
            }

            if (!ContractA.TryString(data, "kind", out string kind))
            {
                Nack(migrationId, entityId, ContractA.NackMalformedMessage, ContractA.ClassPermanent, "'kind' is missing", 0);
                return;
            }

            if (!string.Equals(kind, ContractA.KindBibite, StringComparison.Ordinal))
            {
                Nack(migrationId, entityId, ContractA.NackKindUnsupported, ContractA.ClassPermanent, "M2 only accepts kind 'bibite', got '" + kind + "'", 0);
                return;
            }

            if (!ContractA.TryString(data, "gameVersion", out string gameVersion)
                || !ContractA.TryString(data, "payload", out string payload)
                || !ContractA.TryString(data, "entryEdge", out string entryEdgeText)
                || !ContractA.TryFloat(data, "entryPosition", out float entryPosition)
                || !ContractA.TryVector(data, "velocity", out float velocityX, out float velocityY)
                || !ContractA.TryFloat(data, "heading", out float heading)
                || !ContractA.TryBool(data, "bounceBack", out bool bounceBack)
                || !ContractA.TryInt(data, "attempt", out int attempt))
            {
                Nack(migrationId, entityId, ContractA.NackMalformedMessage, ContractA.ClassPermanent, "a REQUIRED field of MIGRATE_IN.data is missing or has the wrong type", 0);
                return;
            }

            if (!ContractA.TryParseEdge(entryEdgeText, out Edge entryEdge))
            {
                Nack(migrationId, entityId, ContractA.NackMalformedMessage, ContractA.ClassPermanent, "'entryEdge' is not one of N/S/E/W: '" + entryEdgeText + "'", 0);
                return;
            }

            if (entryPosition < 0f || entryPosition > 1f)
            {
                // §4.3: a valid sender always clamps, so an out-of-range value is a defect.
                Nack(migrationId, entityId, ContractA.NackMalformedMessage, ContractA.ClassPermanent,
                    "'entryPosition' " + entryPosition.ToString("R", CultureInfo.InvariantCulture) + " is outside [0,1]", 0);
                return;
            }

            // C3 (Slot-6 livelock) — the received line and its payload SHA-256 are debug-only. The SHA is
            // log-only (nothing consumes it), and hashing a ~10 kB payload on every ingest is dead weight
            // on the main thread under a backlog. On success the SPAWNED line below carries the same key
            // fields; on rejection the NACK line does; both are always logged.
            if (client.Config.DebugIngest)
            {
                MultiversePlugin.Log.LogInfo(
                    $"[M2] migrationId={migrationId} entityId={entityId} phase=MIGRATE_IN_RECEIVED edge={ContractA.EdgeName(entryEdge)} " +
                    $"entryPosition={entryPosition.ToString("F6", CultureInfo.InvariantCulture)} attempt={attempt} bounceBack={bounceBack} " +
                    $"payloadBytes={System.Text.Encoding.UTF8.GetByteCount(payload)} payloadSha256={ContractA.Sha256Hex(payload)}");
            }

            // §13 A3 + §14 A11 + §15 A18 — inbound EDGE_CLOSED means "not a declared edge", never
            // "closed right now". Under the grid all four values appear here: "W" off an east lane, "S"
            // off a north lane, and an export edge for a bounce-back, which comes home through the door
            // it left by. A declared edge that is merely closed still accepts — a bounce-back arrives
            // exactly when the edge just closed.
            if (!client.Config.Declares(entryEdge))
            {
                Nack(migrationId, entityId, ContractA.NackEdgeClosed, ContractA.ClassTransient,
                    "this sim has no border strip on edge " + ContractA.EdgeName(entryEdge) + " (it declares ["
                        + ContractA.EdgeNames(client.Config.BorderEdges) + "])",
                    30000);
                return;
            }

            if (!string.Equals(gameVersion, Application.version, StringComparison.Ordinal))
            {
                Nack(migrationId, entityId, ContractA.NackVersionUnsupported, ContractA.ClassPermanent,
                    "payload is for game version '" + gameVersion + "', this game runs '" + Application.version + "'", 0);
                return;
            }

            if (!GameBridge.SimulationReady())
            {
                Nack(migrationId, entityId, ContractA.NackSimNotReady, ContractA.ClassTransient, "no simulation world is loaded", 5000);
                return;
            }

            // ---- dedup, two keys (§7.3) --------------------------------------------------------
            // C2 (Slot-6 livelock) — this is the O(1) key, checked before the O(N) entityId world scan and
            // before any spawn. Because the ledger now survives a socket reconnect, a replayed already-
            // handled migrationId lands here and is answered with a duplicate ACK for the cost of one
            // HashSet lookup — even if the organism it once spawned has since died, since that path is never
            // reached. The duplicate ACK is what clears the sidecar's journal entry (§5.8).
            if (seenMigrations.Contains(migrationId))
            {
                Ack(migrationId, entityId, duplicate: true, counts: default(LinkRepairCounts));
                MultiversePlugin.Log.LogInfo(
                    $"[M2] migrationId={migrationId} entityId={entityId} phase=DUPLICATE key=migrationId — nothing spawned.");
                return;
            }

            if (GameBridge.FindBodyById(entityId) != null || client.Exporter.IsInFlight(entityId))
            {
                seenMigrations.Add(migrationId);
                Ack(migrationId, entityId, duplicate: true, counts: default(LinkRepairCounts));
                MultiversePlugin.Log.LogInfo(
                    $"[M2] migrationId={migrationId} entityId={entityId} phase=DUPLICATE key=entityId — the organism is already in this world.");
                return;
            }

            // ---- resolve the species, rewrite, restore, re-assert, re-link ---------------------
            BorderGeometry geometry = client.Geometry;
            Vector2 entryPoint = geometry.EntryPoint(entryEdge, entryPosition);
            Vector2 velocity = new Vector2(velocityX, velocityY);

            // §5.7 step 3(a), §16 A30/A31/A32 — all of this is before the restore and on the main
            // thread, because BibiteGenes.LoadState binds gene.species from the blob it is handed
            // (BibiteGenes.cs:581-583) and nothing after that call can correct the binding without
            // moving a live organism between two Species records.
            //
            // A malformed block is treated as **absent**: it is logged once as a sidecar defect — the
            // sidecar was supposed to have stripped it (§5.3) — and never NACKed. Half a name is not a
            // weaker identity, it is a different one.
            SpeciesBlockState blockState = ContractA.ReadSpecies(data, out SpeciesIdentity identity, out string speciesProblem);
            if (blockState == SpeciesBlockState.Malformed)
            {
                MultiversePlugin.Log.LogWarning(
                    $"{SpeciesRegistry.Prefix} migrationId={migrationId} entityId={entityId} — the species block is malformed " +
                    $"({speciesProblem}). That is a sidecar defect: §5.3 says a bad block is stripped before it is forwarded. " +
                    "Treating it as absent (§5.7).");
                identity = null;
            }

            SpeciesRegistry.Resolution resolution = species.Resolve(identity, client.SimTick);
            SpeciesRegistry.LogResolution(migrationId, entityId, resolution);

            string rewritten;
            try
            {
                rewritten = Rewrite(payload, entryPoint, velocity, heading, resolution.resolved, resolution.localId, out string speciesEffect);
                if (client.Config.DebugIngest)
                {
                    MultiversePlugin.Log.LogInfo(
                        $"{SpeciesRegistry.Prefix} migrationId={migrationId} entityId={entityId} payloadRewrite={speciesEffect}");
                }
            }
            catch (Exception e)
            {
                species.Rollback(ref resolution);
                Nack(migrationId, entityId, ContractA.NackDeserializeFailed, ContractA.ClassPermanent,
                    "the payload has no rewritable transform/rb2d block: " + e.Message, 0);
                return;
            }

            GameObject spawned = null;
            try
            {
                // birthAtMaturity MUST stay null: any value routes to StartBodyAtGrowthAndNormalize,
                // which resets health and energy and rolls RNG (m1_findings.md §1.2).
                spawned = SaveSystem.instance.LoadBibiteOrEggFromData(rewritten, resume: true, problems: null, birthAtMaturity: null);
            }
            catch (Exception e)
            {
                MultiversePlugin.Log.LogError($"[M2] migrationId={migrationId}: LoadBibiteOrEggFromData threw (it normally swallows) — {e}");
            }

            if (spawned == null)
            {
                // The wrapper swallows every exception and returns null (m1_findings.md §1.2), so no
                // detail is available from the public path. Reflect once to put the real error in the log.
                try
                {
                    spawned = GameBridge.LoadBibiteDirect(JObject.Parse(rewritten));
                    if (spawned != null)
                    {
                        MultiversePlugin.Log.LogWarning("[M2] the reflected LoadBibite call succeeded where the public wrapper failed.");
                    }
                }
                catch (Exception e)
                {
                    MultiversePlugin.Log.LogError($"[M2] migrationId={migrationId}: the reflected LoadBibite call failed too — {e}");
                }
            }

            if (spawned == null)
            {
                species.Rollback(ref resolution);
                Nack(migrationId, entityId, ContractA.NackDeserializeFailed, ContractA.ClassPermanent, "LoadBibiteOrEggFromData returned null", 0);
                return;
            }

            BibiteBody body = spawned.GetComponent<BibiteBody>();
            if (body == null)
            {
                UnityEngine.Object.Destroy(spawned);
                species.Rollback(ref resolution);
                Nack(migrationId, entityId, ContractA.NackDeserializeFailed, ContractA.ClassPermanent, "the restored GameObject has no BibiteBody", 0);
                return;
            }

            // §5.7 step 3(a) — the organism is standing, so a species this import created can take its
            // genome. Until this runs the new record holds a placeholder template; after it, the record
            // is the one the game would have built for the same organism, minus the generated name.
            species.AdoptTemplate(ref resolution, body);

            // §5.7 step 4 — re-assert in the same frame. The Rigidbody2D wins over the transform on
            // the next tick, and the bibiteHolder parent transform is not proven to be the identity
            // (m2_findings.md §4.3).
            ReassertPlacement(spawned, entryPoint, velocity, heading);

            int restoredId = (body.id != null) ? body.id.id : 0;

            LinkRepairCounts counts;
            try
            {
                OrganismLinks links = OrganismLinks.CaptureForArrival(restoredId != 0 ? restoredId : entityId);
                if (client.Config.DebugIngest)
                {
                    // C3 — the capture summary and the per-link relink detail are debug-only chatter. The
                    // relink COUNTS still reach the SPAWNED line below (relinkedParents/relinkedChildren).
                    links.LogSummary();
                }

                counts = links.Restore(body, client.Config.DebugIngest);
            }
            catch (Exception e)
            {
                MultiversePlugin.Log.LogError($"[M2] migrationId={migrationId} entityId={entityId}: the parent/child repair threw — {e}");
                // §5.9 — no partially-restored organism may survive a NACK.
                GameBridge.DestroyCleanly(body);
                Nack(migrationId, entityId, ContractA.NackRelinkFailed, ContractA.ClassPermanent, "the parent/child repair threw: " + e.Message, 0);
                return;
            }

            seenMigrations.Add(migrationId);
            int immunityKey = (restoredId != 0) ? restoredId : entityId;

            // §5.7 step 6, §14 A11, §15 A19 — the entry-immunity window is REQUIRED and it covers
            // **both** capture bands. Two cases need it: a bounce-back lands inside the band it left
            // from, moving outward; and under the grid an ordinary arrival through W near a corner can
            // land inside the north band already travelling north. Neither may export in the arrival
            // tick, or one hop becomes indistinguishable from two. The key is the entity ID, and
            // HasImmunity is asked before any band is tested, so one window covers every band.
            immunityUntilSimTime[immunityKey] = TimeKeeper.simulatedTime + ContractA.EntryImmunitySeconds;
            client.NoteArrival(immunityKey, entryEdge, entryPosition);

            Vector2 finalPosition = body.transform.position;

            // §16 A31/A32 — what the game's own deserializer actually bound. With a block this is the
            // resolved name; without one it is whatever CheckNewSpecies chose by genetic distance, which
            // is the honest local answer and is only reachable because the foreign id was removed.
            string boundSpecies = (body.gene != null && body.gene.species != null) ? body.gene.species.name : "<none>";

            PortalVisual.Flash(finalPosition, export: false);
            MultiversePlugin.Log.LogInfo(
                $"[M2] migrationId={migrationId} entityId={restoredId} phase=SPAWNED edge={ContractA.EdgeName(entryEdge)} " +
                $"pos=({finalPosition.x:F2},{finalPosition.y:F2}) vel=({velocity.x:F2},{velocity.y:F2}) heading={heading:F2} " +
                $"relinkedParents={counts.parents} relinkedChildren={counts.children} species=\"{boundSpecies}\" " +
                $"immunity={ContractA.EntryImmunitySeconds:F0}s population={GameBridge.LivingPopulation()}");

            if (restoredId != entityId)
            {
                MultiversePlugin.Log.LogError(
                    $"[M2] migrationId={migrationId}: the restored entity ID is {restoredId} but the MIGRATE_IN said {entityId} — that is a defect.");
            }

            Ack(migrationId, restoredId != 0 ? restoredId : entityId, duplicate: false, counts: counts);
        }

        /// <summary>
        /// §5.7 step 3(b) and 3(c) — the eight position numbers and <c>$.genes.speciesID</c>, and nothing
        /// else. The blob is opaque (D4): it is parsed with Newtonsoft.Json.Linq only, never deserialized
        /// into a typed model, and both rewrites address a **named path** and write it. Neither reads a
        /// value back: the incoming <c>speciesID</c> is a foreign world's counter and there is nothing in
        /// it to learn (§4.6, §16 A31).
        ///
        /// <paramref name="hasLocalSpecies"/> false is the **absent-block rule**, and it is the floor for
        /// every import (§16, A32): the key is **removed**, not set to a sentinel.
        /// <c>BibiteGenes.LoadState</c> guards its registry lookup with
        /// <c>if (state["speciesID"] != null)</c>, so an absent key skips the lookup, leaves
        /// <c>gene.species</c> null, and lets <c>ResumeBody</c> → <c>CheckNewSpecies</c> classify the
        /// arrival by the game's own genetic distance. Any integer would be a claim that could come true:
        /// ids are minted from a monotonic counter, so an id chosen to be absent today is an id some
        /// later local species is issued.
        /// </summary>
        internal static string Rewrite(
            string payload,
            Vector2 position,
            Vector2 velocity,
            float heading,
            bool hasLocalSpecies,
            long localSpeciesId,
            out string speciesEffect)
        {
            JObject json = JObject.Parse(payload);

            if (json["genes"] is JObject genes)
            {
                if (hasLocalSpecies)
                {
                    // A JSON integer: Species.speciesID is an int64 (Species.cs:22).
                    genes["speciesID"] = localSpeciesId;
                    speciesEffect = "speciesID=" + localSpeciesId.ToString(CultureInfo.InvariantCulture);
                }
                else
                {
                    bool had = genes.Remove("speciesID");
                    speciesEffect = had ? "speciesID removed" : "speciesID absent already";
                }
            }
            else
            {
                // No $.genes object at all — there is no foreign id to carry and nothing to write. The
                // restore will fail on its own account (LoadState throws without genes data) and surface
                // as DESERIALIZE_FAILED, which is the right answer for a payload in that state.
                speciesEffect = "no $.genes object";
            }

            JToken transform = json["transform"];
            if (!(transform is JObject) || !(transform["position"] is JArray positionArray) || positionArray.Count < 2)
            {
                throw new InvalidOperationException("$.transform.position is missing or is not a two-element array");
            }

            positionArray[0] = position.x;
            positionArray[1] = position.y;
            transform["rotation"] = heading;

            if (!(json["rb2d"] is JObject rigidbody))
            {
                throw new InvalidOperationException("$.rb2d is missing");
            }

            rigidbody["px"] = position.x;
            rigidbody["py"] = position.y;
            rigidbody["vx"] = velocity.x;
            rigidbody["vy"] = velocity.y;
            rigidbody["r"] = heading;

            return json.ToString(Formatting.None);
        }

        private static void ReassertPlacement(GameObject spawned, Vector2 position, Vector2 velocity, float heading)
        {
            spawned.transform.position = new Vector3(position.x, position.y, spawned.transform.position.z);
            spawned.transform.rotation = Quaternion.Euler(0f, 0f, heading);

            Rigidbody2D rigidbody = spawned.GetComponent<Rigidbody2D>();
            if (rigidbody == null)
            {
                return;
            }

            rigidbody.position = position;
            rigidbody.linearVelocity = velocity;
            rigidbody.rotation = heading;
        }

        private void Ack(string migrationId, int entityId, bool duplicate, LinkRepairCounts counts)
        {
            JObject data = new JObject
            {
                ["migrationId"] = migrationId,
                ["entityId"] = entityId,
                ["duplicate"] = duplicate,
                ["simTick"] = client.SimTick,
                ["relinkedParents"] = counts.parents,
                ["relinkedChildren"] = counts.children
            };

            client.SendFrame(ContractA.Envelope(ContractA.TypeMigrateInAck, data).ToString(Formatting.None));

            // C3 — the ACK-sent confirmation is debug-only. On success the SPAWNED line already logged the
            // outcome one call earlier, and on a dedup hit the DUPLICATE line did; both name the same key
            // fields, so this line is pure duplication at info level.
            if (client.Config.DebugIngest)
            {
                MultiversePlugin.Log.LogInfo(
                    $"[M2] migrationId={migrationId} entityId={entityId} phase=MIGRATE_IN_ACK duplicate={duplicate} " +
                    $"relinkedParents={counts.parents} relinkedChildren={counts.children}");
            }
        }

        private void Nack(string migrationId, int entityId, string code, string nackClass, string message, int retryAfterMs)
        {
            JObject data = new JObject
            {
                ["migrationId"] = migrationId,
                ["entityId"] = entityId,
                ["code"] = code,
                ["class"] = nackClass,
                ["message"] = message
            };

            if (retryAfterMs > 0)
            {
                data["retryAfterMs"] = retryAfterMs;
            }

            client.SendFrame(ContractA.Envelope(ContractA.TypeMigrateInNack, data).ToString(Formatting.None));
            MultiversePlugin.Log.LogWarning(
                $"[M2] migrationId={migrationId} entityId={entityId} phase=MIGRATE_IN_NACK code={code} class={nackClass} message=\"{message}\"");
        }

        /// <summary>
        /// §9.2 SIM_NOT_READY — used when the sim is paused, so a delivery does not sit in the mod's
        /// queue with no answer. FixedUpdate does not run at timeScale 0, so the queue would otherwise
        /// never drain.
        /// </summary>
        internal void NackNotReady(JObject data, string why)
        {
            if (!ContractA.TryMigrationId(data, out string migrationId))
            {
                return;
            }

            ContractA.TryInt(data, "entityId", out int entityId);
            Nack(migrationId, entityId, ContractA.NackSimNotReady, ContractA.ClassTransient, why, 5000);
        }

        /// <summary>§9.2 SHUTTING_DOWN — the world is unloading with deliveries still queued.</summary>
        internal void NackShuttingDown(JObject data)
        {
            if (!ContractA.TryMigrationId(data, out string migrationId))
            {
                return;
            }

            ContractA.TryInt(data, "entityId", out int entityId);
            Nack(migrationId, entityId, ContractA.NackShuttingDown, ContractA.ClassTransient, "the game is unloading the world", 0);
        }
    }
}
