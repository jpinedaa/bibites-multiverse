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
    internal class OrganismLinks
    {
        private struct ChildSlot
        {
            public BibiteEggLayingOrgan organ;
            public int index;
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

            BibiteTracker tracker = BibiteTracker.instance;
            if (tracker != null)
            {
                if (tracker.bibites != null)
                {
                    foreach (BibiteBody other in tracker.bibites)
                    {
                        if (other == null || other == body)
                        {
                            continue;
                        }

                        int ownerId = (other.id != null) ? other.id.id : 0;
                        links.RecordChildSlots(other.eggLayer, ownerId);
                        links.RecordDependant(other.gene, body, ownerId);
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

                        links.RecordDependant(egg.eggGene, body, (egg.id != null) ? egg.id.id : 0);
                    }
                }
            }

            return links;
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
            if (layer == null || layer.children == null || selfId == 0)
            {
                return;
            }

            for (int i = 0; i < layer.children.Count; i++)
            {
                BibiteID child = layer.children[i];
                if (child != null && child.id == selfId)
                {
                    holders.Add(new ChildSlot { organ = layer, index = i, ownerId = ownerId });
                }
            }
        }

        private void RecordDependant(BibiteGenes genes, BibiteBody body, int ownerId)
        {
            if (genes == null)
            {
                return;
            }

            if (genes.parent1 == body.gameObject || (genes.parent1 == null && selfId != 0 && genes.parent1ID == selfId))
            {
                dependants.Add(new ParentSlot { genes = genes, slot = 1, ownerId = ownerId });
            }

            if (genes.parent2 == body.gameObject || (genes.parent2 == null && selfId != 0 && genes.parent2ID == selfId))
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
        internal void Restore(BibiteBody newBody)
        {
            RestoreOwnParents(newBody);
            RestoreOwnChildren(newBody);
            RestoreHolders(newBody);
            RestoreDependants(newBody);
        }

        private void RestoreOwnParents(BibiteBody newBody)
        {
            BibiteGenes genes = newBody.gene;
            if (genes == null)
            {
                MultiversePlugin.Log.LogWarning("relink: respawned organism has no BibiteGenes — parents not restored.");
                return;
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
        }

        private void RestoreOwnChildren(BibiteBody newBody)
        {
            BibiteEggLayingOrgan layer = newBody.eggLayer;
            if (layer == null)
            {
                return;
            }

            if (layer.children == null)
            {
                layer.children = new List<BibiteID>();
            }

            if (layer.childIDs == null)
            {
                MultiversePlugin.Log.LogInfo("relink: no children recorded in the payload.");
                return;
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
        }

        private void RestoreHolders(BibiteBody newBody)
        {
            BibiteID newId = newBody.id;
            if (newId == null)
            {
                MultiversePlugin.Log.LogWarning("relink: respawned organism has no BibiteID — parent lists not repaired.");
                return;
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
                if (slot.index < list.Count && (list[slot.index] == null || list[slot.index].id == selfId))
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
        }

        private void RestoreDependants(BibiteBody newBody)
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
        }
    }
}
