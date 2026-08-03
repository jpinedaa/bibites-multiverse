using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.Globalization;
using Newtonsoft.Json;
using Newtonsoft.Json.Linq;
using SimulationScripts.BibiteScripts;
using UnityEngine;

namespace BibitesMultiverse
{
    /// <summary>
    /// The mod's half of the lineage annex (D11, contract-a.md §5.3, §14 A12).
    ///
    /// **The mod ships, the sidecar hashes.** D11 wants a content hash of each parent genome in every
    /// migration envelope; D4 forbids the mod to parse a bb8 body, and a hash of a genome projection
    /// is a parse (m3_considerations.md Risk 8). The split resolves it at the process boundary: this
    /// class collects the parent entity IDs and one **opaque** serialized blob per living parent, and
    /// the sidecar does every hash, cache and strip. Nothing here ever looks inside a blob.
    ///
    /// Three rules from §5.3 shape the result:
    ///
    /// * a parent is serialized **only while its GameObject is alive**, in the same FixedUpdate as the
    ///   migrant. <c>SaveSystem.SerializeBibite</c> on a destroyed GameObject is a Unity fake-null trap;
    /// * a parent with no blob — dead, unserializable, or dropped for size — is a **gap**, which is the
    ///   normal case and never an error. <c>BibiteGenes</c> drops the parentage once the parent
    ///   GameObject is gone, so an entity ID with no blob is what a departed parent looks like;
    /// * an entity ID of <c>0</c> is the game's "unassigned" sentinel, and such an entry is omitted
    ///   entirely.
    ///
    /// The blob is cached by entity ID **within one tick**: a brood of siblings crossing together
    /// would otherwise serialize the same mother once per sibling, on the main thread (Risk 8).
    /// </summary>
    internal class LineageCollector
    {
        internal const string Prefix = "[M3-LINEAGE]";

        internal struct Result
        {
            /// <summary>The <c>parents</c> array, or null when no parent entity ID is known at all.</summary>
            internal JArray parents;

            /// <summary>A one-token-per-parent description for the export log.</summary>
            internal string summary;

            /// <summary>What the extra serializations cost on the main thread (m3_considerations.md WP4).</summary>
            internal double serializeMs;

            internal int blobs;
            internal int gaps;
            internal int blobBytes;
            internal int cacheHits;
        }

        private readonly Dictionary<int, string> blobCache = new Dictionary<int, string>();
        private long cacheTick = long.MinValue;
        private int cacheHits;

        private readonly List<int> ids = new List<int>(ContractA.MaxParentBlobs);
        private readonly List<GameObject> objects = new List<GameObject>(ContractA.MaxParentBlobs);
        private readonly List<string> blobs = new List<string>(ContractA.MaxParentBlobs);
        private readonly List<int> sizes = new List<int>(ContractA.MaxParentBlobs);

        internal void Clear()
        {
            blobCache.Clear();
            cacheTick = long.MinValue;
        }

        /// <summary>
        /// Collect the annex inputs for one migrant. <paramref name="migrantPayloadBytes"/> is what the
        /// migrant's own blob already costs, because the frame budget of §5.3 is shared.
        /// </summary>
        internal Result Collect(BibiteBody migrant, int selfId, int migrantPayloadBytes, long simTick, string gameVersion)
        {
            Result result = default(Result);
            result.summary = "<none>";

            if (simTick != cacheTick)
            {
                blobCache.Clear();
                cacheTick = simTick;
            }

            ids.Clear();
            objects.Clear();
            blobs.Clear();
            sizes.Clear();
            cacheHits = 0;

            BibiteGenes genes = (migrant != null) ? migrant.gene : null;
            if (genes == null)
            {
                return result;
            }

            // §5.3: parent1 then parent2, in that order. The integers are the same values the migrant's
            // own $.genes.parent1 / $.genes.parent2 carry (BibiteGenes.cs:552-559) — read from the
            // component rather than out of the blob, so no genome is parsed (D4) and so a parent whose
            // GameObject is already gone still contributes its entity ID as a gap.
            Consider(genes.parent1, genes.parent1ID, selfId);
            Consider(genes.parent2, genes.parent2ID, selfId);

            if (ids.Count == 0)
            {
                return result;
            }

            Stopwatch clock = Stopwatch.StartNew();
            for (int i = 0; i < ids.Count; i++)
            {
                string blob = Serialize(ids[i], objects[i], gameVersion);
                blobs.Add(blob);
                sizes.Add(blob != null ? System.Text.Encoding.UTF8.GetByteCount(blob) : 0);
            }

            clock.Stop();
            result.serializeMs = clock.Elapsed.TotalMilliseconds;

            DropForSize(migrantPayloadBytes);

            JArray parents = new JArray();
            System.Text.StringBuilder summary = new System.Text.StringBuilder();
            for (int i = 0; i < ids.Count; i++)
            {
                JObject entry = new JObject { ["entityId"] = ids[i] };
                if (blobs[i] != null)
                {
                    entry["payload"] = blobs[i];
                    entry["gameVersion"] = gameVersion;
                    result.blobs++;
                    result.blobBytes += sizes[i];
                }
                else
                {
                    result.gaps++;
                }

                parents.Add(entry);

                if (summary.Length > 0)
                {
                    summary.Append(',');
                }

                summary.Append(ids[i].ToString(CultureInfo.InvariantCulture));
                summary.Append(blobs[i] != null ? ":blob(" + sizes[i].ToString(CultureInfo.InvariantCulture) + "B)" : ":gap");
            }

            result.parents = parents;
            result.summary = summary.ToString();
            result.cacheHits = cacheHits;
            return result;
        }

        private void Consider(GameObject parent, int recordedId, int selfId)
        {
            if (ids.Count >= ContractA.MaxParentBlobs)
            {
                return;
            }

            // Unity's fake null: a destroyed GameObject compares equal to null here, which is exactly
            // the "the parent is gone" case, so the recorded integer is the only handle left.
            GameObject live = (parent != null) ? parent : null;

            int id = 0;
            if (live != null)
            {
                BibiteBody body = live.GetComponent<BibiteBody>();
                if (GameBridge.IsUsable(body) && body.id != null)
                {
                    id = body.id.id;
                }
                else
                {
                    live = null; // present but unusable — treat it as gone and record the gap
                }
            }

            if (id == 0)
            {
                id = recordedId;
            }

            // §5.3 — 0 is the game's "unassigned" sentinel and such an entry MUST be omitted entirely.
            if (id == 0 || id == selfId || ids.Contains(id))
            {
                return;
            }

            ids.Add(id);
            objects.Add(live);
        }

        private string Serialize(int entityId, GameObject parent, string gameVersion)
        {
            if (parent == null)
            {
                return null;
            }

            if (blobCache.TryGetValue(entityId, out string cached))
            {
                cacheHits++;
                return cached;
            }

            string blob;
            try
            {
                JObject serialized = GameBridge.SerializeBibite(parent);
                if (serialized == null)
                {
                    MultiversePlugin.Log.LogInfo(
                        $"{Prefix} parent {entityId}: SerializeBibite returned null — recorded as a gap, which is never an error (§14 A12).");
                    return null;
                }

                JObject stamped = (JObject)serialized.DeepClone();
                stamped["version"] = gameVersion;
                blob = stamped.ToString(Formatting.None);
            }
            catch (Exception e)
            {
                // §5.3 — a serialization failure for a parent is a gap, not an error.
                MultiversePlugin.Log.LogWarning($"{Prefix} parent {entityId}: serialization threw ({e.Message}) — recorded as a gap.");
                return null;
            }

            int bytes = System.Text.Encoding.UTF8.GetByteCount(blob);
            if (bytes > ContractA.MaxPayloadBytes)
            {
                MultiversePlugin.Log.LogWarning(
                    $"{Prefix} parent {entityId}: blob is {bytes} bytes, over maxPayloadBytes ({ContractA.MaxPayloadBytes}) — recorded as a gap.");
                return null;
            }

            blobCache[entityId] = blob;
            return blob;
        }

        /// <summary>
        /// §5.3 — keep the whole frame under <c>maxFrameBytes − frameHeadroomBytes</c> by dropping
        /// parent **blobs**, largest first, and keeping their entity IDs. A dropped blob is a gap, and
        /// a gap is never a reason to abandon a migration. The migrant's own payload is never dropped.
        /// </summary>
        private void DropForSize(int migrantPayloadBytes)
        {
            int budget = ContractA.MaxFrameBytes - ContractA.FrameHeadroomBytes - migrantPayloadBytes;
            int used = 0;
            for (int i = 0; i < sizes.Count; i++)
            {
                used += (blobs[i] != null) ? sizes[i] : 0;
            }

            while (used > budget)
            {
                int largest = -1;
                for (int i = 0; i < blobs.Count; i++)
                {
                    if (blobs[i] == null)
                    {
                        continue;
                    }

                    if (largest < 0 || sizes[i] > sizes[largest])
                    {
                        largest = i;
                    }
                }

                if (largest < 0)
                {
                    return;
                }

                MultiversePlugin.Log.LogWarning(
                    $"{Prefix} dropping the blob of parent {ids[largest]} ({sizes[largest]} bytes): the migrant plus its " +
                    $"parents would pass maxFrameBytes - frameHeadroomBytes. The entity ID stays and the blob becomes a gap.");
                used -= sizes[largest];
                blobs[largest] = null;
            }
        }
    }
}
