using System.Collections.Generic;
using System.Text;
using ManagementScripts;
using SimulationScripts.BibiteScripts;

namespace BibitesMultiverse
{
    /// <summary>What one sweep of the live world found in the way of family links.</summary>
    internal struct FamilyReport
    {
        internal int living;
        internal int eggs;
        internal int withLivingParent;
        internal int withLivingChild;
        internal int parentLinks;
        internal int childLinks;

        /// <summary>An organism that some **living** parent still lists in its children.</summary>
        internal int sampleChildId;
        internal int sampleChildParentId;

        /// <summary>An organism that still holds a **living** child in its own children list.</summary>
        internal int sampleParentId;
        internal int sampleParentChildId;

        /// <summary>m2_considerations.md Risk 5 needs at least one of the two.</summary>
        internal bool Any => withLivingParent > 0 || withLivingChild > 0;
    }

    /// <summary>
    /// The dev hook that answers "does this world hold a family link yet?".
    ///
    /// m2_considerations.md Risk 5 is the reason it exists: the parent/child repair and the two
    /// <c>MIGRATE_IN_ACK</c> counters have never been exercised, because M1's test organism had no
    /// relatives. The M2 rig has to grow a world until a link exists, and a script cannot see Unity
    /// objects — so the mod prints the number and the script polls the log.
    ///
    /// "Living" means a live <see cref="BibiteBody"/> or a live egg: an egg's <c>BibiteID</c> sits in
    /// its parent's <c>eggLayer.children</c> exactly like a hatched child's, and it carries the same
    /// stale-reference risk on the next world save.
    /// </summary>
    internal static class FamilyScan
    {
        internal const string Prefix = "[M2-FAMILY]";

        internal static FamilyReport Scan()
        {
            FamilyReport report = default(FamilyReport);

            BibiteTracker tracker = BibiteTracker.instance;
            if (tracker == null || tracker.bibites == null)
            {
                return report;
            }

            HashSet<int> liveIds = new HashSet<int>();
            List<BibiteBody> alive = new List<BibiteBody>();
            foreach (BibiteBody body in tracker.bibites)
            {
                if (!IsAlive(body) || body.id == null || body.id.id == 0)
                {
                    continue;
                }

                alive.Add(body);
                liveIds.Add(body.id.id);
            }

            if (tracker.eggs != null)
            {
                foreach (EggHatching egg in tracker.eggs)
                {
                    if (egg != null && egg.id != null && egg.id.id != 0)
                    {
                        liveIds.Add(egg.id.id);
                        report.eggs++;
                    }
                }
            }

            report.living = alive.Count;

            foreach (BibiteBody body in alive)
            {
                int selfId = body.id.id;

                int parents = 0;
                int firstParent = 0;
                BibiteGenes genes = body.gene;
                if (genes != null)
                {
                    // In a live session the link is the GameObject; the integer id is only filled in
                    // by a load (BibiteGenes.cs:598-605). Read both, exactly like OrganismLinks does,
                    // or every freshly hatched child looks parentless.
                    foreach (int parentId in new[]
                    {
                        ResolveParentId(genes.parent1, genes.parent1ID),
                        ResolveParentId(genes.parent2, genes.parent2ID)
                    })
                    {
                        if (parentId != 0 && parentId != selfId && liveIds.Contains(parentId))
                        {
                            parents++;
                            if (firstParent == 0)
                            {
                                firstParent = parentId;
                            }
                        }
                    }
                }

                if (parents > 0)
                {
                    report.withLivingParent++;
                    report.parentLinks += parents;
                    if (report.sampleChildId == 0)
                    {
                        report.sampleChildId = selfId;
                        report.sampleChildParentId = firstParent;
                    }
                }

                int children = 0;
                int firstChild = 0;
                BibiteEggLayingOrgan layer = body.eggLayer;
                if (layer != null && layer.children != null)
                {
                    foreach (BibiteID child in layer.children)
                    {
                        if (child != null && child.id != 0 && child.id != selfId && liveIds.Contains(child.id))
                        {
                            children++;
                            if (firstChild == 0)
                            {
                                firstChild = child.id;
                            }
                        }
                    }
                }

                if (children > 0)
                {
                    report.withLivingChild++;
                    report.childLinks += children;
                    if (report.sampleParentId == 0)
                    {
                        report.sampleParentId = selfId;
                        report.sampleParentChildId = firstChild;
                    }
                }
            }

            return report;
        }

        internal static void LogReport(FamilyReport report, string why)
        {
            MultiversePlugin.Log.LogInfo(
                $"{Prefix} why={why} living={report.living} eggs={report.eggs} " +
                $"withLivingParent={report.withLivingParent} withLivingChild={report.withLivingChild} " +
                $"parentLinks={report.parentLinks} childLinks={report.childLinks} " +
                $"sampleChild={report.sampleChildId}(parent={report.sampleChildParentId}) " +
                $"sampleParent={report.sampleParentId}(child={report.sampleParentChildId}) " +
                $"linked={(report.Any ? "YES" : "NO")}");
        }

        /// <summary>
        /// The organism the family re-link test should migrate.
        ///
        /// Preference order matters. An organism whose **living parent** lists it in
        /// <c>eggLayer.children</c> is the strongest case: when it leaves, that stale
        /// <c>BibiteID</c> is what makes the origin world's next save throw
        /// (<c>BibiteEggLayingOrgan.SaveState</c>), which is precisely Risk 5. Second choice is an
        /// organism that still holds a living child of its own.
        /// </summary>
        internal static BibiteBody PickLinked(out string description)
        {
            FamilyReport report = Scan();

            if (report.sampleChildId != 0)
            {
                BibiteBody body = GameBridge.FindBodyById(report.sampleChildId);
                if (body != null)
                {
                    description = $"has a living parent ({report.sampleChildParentId})";
                    return body;
                }
            }

            if (report.sampleParentId != 0)
            {
                BibiteBody body = GameBridge.FindBodyById(report.sampleParentId);
                if (body != null)
                {
                    description = $"has a living child ({report.sampleParentChildId})";
                    return body;
                }
            }

            description = "no organism in this world has a living parent or a living child";
            return null;
        }

        /// <summary>Every living entity id, for the kill test's "count the organism" step.</summary>
        internal static string Census(out int population)
        {
            population = 0;
            StringBuilder ids = new StringBuilder();

            BibiteTracker tracker = BibiteTracker.instance;
            if (tracker == null || tracker.bibites == null)
            {
                return string.Empty;
            }

            foreach (BibiteBody body in tracker.bibites)
            {
                if (!IsAlive(body) || body.id == null)
                {
                    continue;
                }

                population++;
                if (ids.Length > 0)
                {
                    ids.Append(',');
                }

                ids.Append(body.id.id);
            }

            return ids.ToString();
        }

        internal static bool IsAlive(BibiteBody body)
        {
            return GameBridge.IsUsable(body) && !body.dead && !body.dying;
        }

        /// <summary>The live GameObject wins over the persisted integer, as in <see cref="OrganismLinks"/>.</summary>
        private static int ResolveParentId(UnityEngine.GameObject parent, int parentId)
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
    }
}
