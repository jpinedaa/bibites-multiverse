using System.Collections.Generic;
using ManagementScripts;
using SimulationScripts.BibiteScripts;
using UnityEngine;

namespace BibitesMultiverse
{
    /// <summary>
    /// The cross-organism references that survive neither Destroy nor LoadBibite.
    /// LoadBibite restores only the integer IDs (BibiteGenes.cs:598-605,
    /// BibiteEggLayingOrgan.cs:250-253); SaveSystem.LoadGame re-links the objects
    /// afterwards at SaveSystem.cs:748-782. This class does the same repair for a
    /// single organism, in both directions.
    /// </summary>
    /// <summary>How much repair work <see cref="OrganismLinks.Restore"/> actually did.</summary>
    internal struct LinkRepairCounts
    {
        internal int parents;
        internal int children;

        public override string ToString()
        {
            return $"parents={parents} children={children}";
        }
    }

    internal class OrganismLinks
    {
        private struct ChildSlot
        {
            public BibiteEggLayingOrgan organ;
            public int index; // -1 means "the parent claims this child by ID but has no live slot"
            public int ownerId;
        }

        private struct ParentSlot
        {
            public BibiteGenes genes;
            public int slot; // 1 or 2
            public int ownerId;
        }

        private readonly List<ChildSlot> holders = new List<ChildSlot>();
        private readonly List<ParentSlot> dependants = new List<ParentSlot>();

        internal int selfId;
        internal int parent1Id;
        internal int parent2Id;
        internal int ownChildCount;

        internal static OrganismLinks Capture(BibiteBody body)
        {
            OrganismLinks links = new OrganismLinks();
            links.selfId = (body.id != null) ? body.id.id : 0;

            BibiteGenes genes = body.gene;
            if (genes != null)
            {
                links.parent1Id = ResolveParentId(genes.parent1, genes.parent1ID);
                links.parent2Id = ResolveParentId(genes.parent2, genes.parent2ID);
            }

            BibiteEggLayingOrgan ownLayer = body.eggLayer;
            if (ownLayer != null && ownLayer.children != null)
            {
                links.ownChildCount = ownLayer.children.Count;
            }

            links.ScanWorld(body, body.gameObject);
            return links;
        }

        /// <summary>
        /// The import side of the same repair. The organism does not exist locally yet, so every match
        /// is by entity ID: parents whose childIDs claim it, and organisms whose parent1ID/parent2ID
        /// point at it. Cross-world these lists are normally empty, which is exactly why M2 has to run
        /// the code path and report the counts (m1_findings.md, residual gap).
        /// </summary>
        internal static OrganismLinks CaptureForArrival(int entityId)
        {
            OrganismLinks links = new OrganismLinks();
            links.selfId = entityId;
            links.ScanWorld(null, null);
            return links;
        }

        private void ScanWorld(BibiteBody self, GameObject selfObject)
        {
            BibiteTracker tracker = BibiteTracker.instance;
            if (tracker == null)
            {
                return;
            }

            if (tracker.bibites != null)
            {
                foreach (BibiteBody other in tracker.bibites)
                {
                    if (other == null || (self != null && other == self))
                    {
                        continue;
                    }

                    int ownerId = (other.id != null) ? other.id.id : 0;
                    RecordChildSlots(other.eggLayer, ownerId);
                    RecordDependant(other.gene, selfObject, ownerId);
                }
            }

            if (tracker.eggs != null)
            {
                foreach (EggHatching egg in tracker.eggs)
                {
                    if (egg == null)
                    {
                        continue;
                    }

                    RecordDependant(egg.eggGene, selfObject, (egg.id != null) ? egg.id.id : 0);
                }
            }
        }

        private static int ResolveParentId(GameObject parent, int parentId)
        {
            if (parent != null)
            {
                BibiteBody parentBody = parent.GetComponent<BibiteBody>();
                if (parentBody != null && parentBody.id != null && parentBody.id.id != 0)
                {
                    return parentBody.id.id;
                }
            }

            return parentId;
        }

        private void RecordChildSlots(BibiteEggLayingOrgan layer, int ownerId)
        {
            if (layer == null || selfId == 0)
            {
                return;
            }

            bool found = false;
            if (layer.children != null)
            {
                for (int i = 0; i < layer.children.Count; i++)
                {
                    BibiteID child = layer.children[i];
                    if (child != null && child.id == selfId)
                    {
                        holders.Add(new ChildSlot { organ = layer, index = i, ownerId = ownerId });
                        found = true;
                    }
                }
            }

            // A parent restored from a payload carries childIDs but an unresolved children list
            // (BibiteEggLayingOrgan.cs:250-253). That is the state an imported parent is in when its
            // child arrives afterwards, so claim the slot by ID as well.
            if (!found && layer.childIDs != null && layer.childIDs.Contains(selfId))
            {
                holders.Add(new ChildSlot { organ = layer, index = -1, ownerId = ownerId });
            }
        }

        private void RecordDependant(BibiteGenes genes, GameObject selfObject, int ownerId)
        {
            if (genes == null || selfId == 0)
            {
                return;
            }

            bool firstMatches = (selfObject != null && genes.parent1 == selfObject)
                || (genes.parent1 == null && genes.parent1ID == selfId);
            if (firstMatches)
            {
                dependants.Add(new ParentSlot { genes = genes, slot = 1, ownerId = ownerId });
            }

            bool secondMatches = (selfObject != null && genes.parent2 == selfObject)
                || (genes.parent2 == null && genes.parent2ID == selfId);
            if (secondMatches)
            {
                dependants.Add(new ParentSlot { genes = genes, slot = 2, ownerId = ownerId });
            }
        }

        internal void LogSummary()
        {
            MultiversePlugin.Log.LogInfo(
                $"links captured: id={selfId} parent1={parent1Id} parent2={parent2Id} " +
                $"ownChildren={ownChildCount} holdersListingMe={holders.Count} organismsCallingMeParent={dependants.Count}");

            foreach (ChildSlot slot in holders)
            {
                MultiversePlugin.Log.LogInfo($"  parent {slot.ownerId} lists me at children[{slot.index}]");
            }

            foreach (ParentSlot slot in dependants)
            {
                MultiversePlugin.Log.LogInfo($"  organism {slot.ownerId} has me as parent{slot.slot}");
            }
        }

        /// <summary>Re-apply every recorded link to the respawned instance, both directions.</summary>
        internal LinkRepairCounts Restore(BibiteBody newBody)
        {
            LinkRepairCounts counts = default(LinkRepairCounts);
            counts.parents += RestoreOwnParents(newBody);
            counts.children += RestoreOwnChildren(newBody);
            counts.children += RestoreHolders(newBody);
            counts.parents += RestoreDependants(newBody);
            return counts;
        }

        /// <summary>
        /// The export counterpart of <see cref="Restore"/>: the organism is leaving this world for
        /// good, so every reference to it has to go before it is destroyed. A stale BibiteID left in a
        /// parent's eggLayer.children makes BibiteEggLayingOrgan.SaveState throw on the next world
        /// save, which looks like a corrupted save rather than a mod defect (m1_findings.md §4.3,
        /// §5.3). The integer parentNID values stay — they are what the game itself persists.
        /// </summary>
        internal void Detach()
        {
            int clearedChildren = 0;
            foreach (ChildSlot slot in holders)
            {
                BibiteEggLayingOrgan organ = slot.organ;
                if (organ == null || organ.children == null)
                {
                    continue;
                }

                for (int i = organ.children.Count - 1; i >= 0; i--)
                {
                    BibiteID child = organ.children[i];
                    if (child == null || child.id == selfId)
                    {
                        organ.children.RemoveAt(i);
                        clearedChildren++;
                    }
                }
            }

            int clearedParents = 0;
            foreach (ParentSlot slot in dependants)
            {
                BibiteGenes genes = slot.genes;
                if (genes == null)
                {
                    continue;
                }

                if (slot.slot == 1)
                {
                    genes.parent1 = null;
                    genes.parent1ID = selfId;
                }
                else
                {
                    genes.parent2 = null;
                    genes.parent2ID = selfId;
                }

                clearedParents++;
            }

            MultiversePlugin.Log.LogInfo(
                $"[M2] detach id={selfId}: cleared {clearedChildren} stale children entr(ies) and {clearedParents} parent reference(s).");
        }

        private int RestoreOwnParents(BibiteBody newBody)
        {
            BibiteGenes genes = newBody.gene;
            if (genes == null)
            {
                MultiversePlugin.Log.LogWarning("relink: respawned organism has no BibiteGenes — parents not restored.");
                return 0;
            }

            int p1 = (parent1Id != 0) ? parent1Id : genes.parent1ID;
            int p2 = (parent2Id != 0) ? parent2Id : genes.parent2ID;
            genes.parent1ID = p1;
            genes.parent2ID = p2;

            BibiteBody first = GameBridge.FindBodyById(p1);
            BibiteBody second = GameBridge.FindBodyById(p2);
            genes.parent1 = (first != null) ? first.gameObject : null;
            genes.parent2 = (second != null) ? second.gameObject : null;

            MultiversePlugin.Log.LogInfo(
                $"relink: parents parent1={p1}{(p1 != 0 && first == null ? " (not live)" : "")} " +
                $"parent2={p2}{(p2 != 0 && second == null ? " (not live)" : "")}");
            return ((first != null) ? 1 : 0) + ((second != null) ? 1 : 0);
        }

        private int RestoreOwnChildren(BibiteBody newBody)
        {
            BibiteEggLayingOrgan layer = newBody.eggLayer;
            if (layer == null)
            {
                return 0;
            }

            if (layer.children == null)
            {
                layer.children = new List<BibiteID>();
            }

            if (layer.childIDs == null)
            {
                MultiversePlugin.Log.LogInfo("relink: no children recorded in the payload.");
                return 0;
            }

            layer.children.Clear();
            int resolved = 0;
            int missing = 0;
            foreach (int childId in layer.childIDs)
            {
                if (childId == 0)
                {
                    continue;
                }

                BibiteID child = GameBridge.FindIdById(childId);
                if (child != null)
                {
                    layer.children.Add(child);
                    resolved++;
                }
                else
                {
                    missing++;
                }
            }

            MultiversePlugin.Log.LogInfo($"relink: own children resolved={resolved} unresolved={missing}");
            return resolved;
        }

        private int RestoreHolders(BibiteBody newBody)
        {
            BibiteID newId = newBody.id;
            if (newId == null)
            {
                MultiversePlugin.Log.LogWarning("relink: respawned organism has no BibiteID — parent lists not repaired.");
                return 0;
            }

            int replaced = 0;
            int appended = 0;
            foreach (ChildSlot slot in holders)
            {
                BibiteEggLayingOrgan organ = slot.organ;
                if (organ == null || organ.children == null)
                {
                    continue;
                }

                List<BibiteID> list = organ.children;
                if (slot.index >= 0 && slot.index < list.Count && (list[slot.index] == null || list[slot.index].id == selfId))
                {
                    list[slot.index] = newId;
                    replaced++;
                }
                else if (!list.Contains(newId))
                {
                    list.Add(newId);
                    appended++;
                }
            }

            MultiversePlugin.Log.LogInfo($"relink: parent children lists repaired in place={replaced} appended={appended}");
            return replaced + appended;
        }

        private int RestoreDependants(BibiteBody newBody)
        {
            int restored = 0;
            foreach (ParentSlot slot in dependants)
            {
                BibiteGenes genes = slot.genes;
                if (genes == null)
                {
                    continue;
                }

                if (slot.slot == 1)
                {
                    genes.parent1 = newBody.gameObject;
                    genes.parent1ID = selfId;
                }
                else
                {
                    genes.parent2 = newBody.gameObject;
                    genes.parent2ID = selfId;
                }

                restored++;
            }

            MultiversePlugin.Log.LogInfo($"relink: organisms pointing at me as parent repaired={restored}");
            return restored;
        }
    }
}
