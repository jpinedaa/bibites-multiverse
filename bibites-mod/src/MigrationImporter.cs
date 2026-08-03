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
        private readonly HashSet<string> seenMigrations = new HashSet<string>(StringComparer.OrdinalIgnoreCase);
        private readonly Dictionary<int, double> immunityUntilSimTime = new Dictionary<int, double>();

        internal MigrationImporter(MultiverseClient client)
        {
            this.client = client;
        }

        internal void Clear()
        {
            seenMigrations.Clear();
            immunityUntilSimTime.Clear();
        }

        internal void ClearConnectionState()
        {
            // migrationId dedup is per connection (§7.3). entityId dedup lives in the world itself.
            seenMigrations.Clear();
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

            if (!ContractA.TryString(data, "migrationId", out migrationId))
            {
                MultiversePlugin.Log.LogError(
                    "[M2] MIGRATE_IN without a usable 'migrationId' — the NACK is keyed on that field, so this frame cannot be answered. Dropped.");
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

            MultiversePlugin.Log.LogInfo(
                $"[M2] migrationId={migrationId} entityId={entityId} phase=MIGRATE_IN_RECEIVED edge={ContractA.EdgeName(entryEdge)} " +
                $"entryPosition={entryPosition.ToString("F6", CultureInfo.InvariantCulture)} attempt={attempt} bounceBack={bounceBack} " +
                $"payloadBytes={System.Text.Encoding.UTF8.GetByteCount(payload)}");

            if (entryEdge != client.Config.BorderEdge)
            {
                Nack(migrationId, entityId, ContractA.NackEdgeClosed, ContractA.ClassTransient,
                    "this sim has no border strip on edge " + ContractA.EdgeName(entryEdge) + " (it declares " + ContractA.EdgeName(client.Config.BorderEdge) + ")",
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

            // ---- rewrite, restore, re-assert, re-link ------------------------------------------
            BorderGeometry geometry = client.Geometry;
            Vector2 entryPoint = geometry.EntryPoint(entryEdge, entryPosition);
            Vector2 velocity = new Vector2(velocityX, velocityY);

            string rewritten;
            try
            {
                rewritten = Rewrite(payload, entryPoint, velocity, heading);
            }
            catch (Exception e)
            {
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
                Nack(migrationId, entityId, ContractA.NackDeserializeFailed, ContractA.ClassPermanent, "LoadBibiteOrEggFromData returned null", 0);
                return;
            }

            BibiteBody body = spawned.GetComponent<BibiteBody>();
            if (body == null)
            {
                UnityEngine.Object.Destroy(spawned);
                Nack(migrationId, entityId, ContractA.NackDeserializeFailed, ContractA.ClassPermanent, "the restored GameObject has no BibiteBody", 0);
                return;
            }

            // §5.7 step 4 — re-assert in the same frame. The Rigidbody2D wins over the transform on
            // the next tick, and the bibiteHolder parent transform is not proven to be the identity
            // (m2_findings.md §4.3).
            ReassertPlacement(spawned, entryPoint, velocity, heading);

            int restoredId = (body.id != null) ? body.id.id : 0;

            LinkRepairCounts counts;
            try
            {
                OrganismLinks links = OrganismLinks.CaptureForArrival(restoredId != 0 ? restoredId : entityId);
                links.LogSummary();
                counts = links.Restore(body);
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
            immunityUntilSimTime[immunityKey] = TimeKeeper.simulatedTime + ContractA.EntryImmunitySeconds;
            client.NoteArrival(immunityKey);

            Vector2 finalPosition = body.transform.position;
            MultiversePlugin.Log.LogInfo(
                $"[M2] migrationId={migrationId} entityId={restoredId} phase=SPAWNED edge={ContractA.EdgeName(entryEdge)} " +
                $"pos=({finalPosition.x:F2},{finalPosition.y:F2}) vel=({velocity.x:F2},{velocity.y:F2}) heading={heading:F2} " +
                $"relinkedParents={counts.parents} relinkedChildren={counts.children} " +
                $"immunity={ContractA.EntryImmunitySeconds:F0}s population={GameBridge.LivingPopulation()}");

            if (restoredId != entityId)
            {
                MultiversePlugin.Log.LogError(
                    $"[M2] migrationId={migrationId}: the restored entity ID is {restoredId} but the MIGRATE_IN said {entityId} — that is a defect.");
            }

            Ack(migrationId, restoredId != 0 ? restoredId : entityId, duplicate: false, counts: counts);
        }

        /// <summary>
        /// §5.7 step 3 — the eight position numbers, and nothing else. The blob is opaque (D4): it is
        /// parsed with Newtonsoft.Json.Linq only, never deserialized into a typed model.
        /// </summary>
        internal static string Rewrite(string payload, Vector2 position, Vector2 velocity, float heading)
        {
            JObject json = JObject.Parse(payload);

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
            MultiversePlugin.Log.LogInfo(
                $"[M2] migrationId={migrationId} entityId={entityId} phase=MIGRATE_IN_ACK duplicate={duplicate} " +
                $"relinkedParents={counts.parents} relinkedChildren={counts.children}");
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
            if (!ContractA.TryString(data, "migrationId", out string migrationId))
            {
                return;
            }

            ContractA.TryInt(data, "entityId", out int entityId);
            Nack(migrationId, entityId, ContractA.NackSimNotReady, ContractA.ClassTransient, why, 5000);
        }

        /// <summary>§9.2 SHUTTING_DOWN — the world is unloading with deliveries still queued.</summary>
        internal void NackShuttingDown(JObject data)
        {
            if (!ContractA.TryString(data, "migrationId", out string migrationId))
            {
                return;
            }

            ContractA.TryInt(data, "entityId", out int entityId);
            Nack(migrationId, entityId, ContractA.NackShuttingDown, ContractA.ClassTransient, "the game is unloading the world", 0);
        }
    }
}
