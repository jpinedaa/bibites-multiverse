using System;
using System.Collections.Generic;
using System.Globalization;
using ManagementScripts;
using Newtonsoft.Json.Linq;
using OneUseScripts;
using SettingScripts;
using SimulationScripts.BibiteScripts;
using UnityEngine;
using Utility;

namespace BibitesMultiverse
{
    /// <summary>
    /// The mod's half of species identity (D4, contract-a.md §5.3, §5.7, §16 A30–A32).
    ///
    /// **A species id is world-local and a species name is not.** A `.bb8` payload carries exactly one
    /// thing about species — <c>genes.speciesID</c>, drawn from the world-local counter
    /// <c>Species.speciesMaxID</c> (<c>Species.cs:19, 89</c>) — and <c>BibiteGenes.LoadState</c> looks
    /// that integer up in the **local** registry anyway (<c>BibiteGenes.cs:581-583</c>). Carried across
    /// worlds it either collides with an unrelated local species, which is silently wrong, or misses and
    /// leaves the arrival to be classified by genetic distance, which is the honest answer. §16 puts the
    /// **name** in the envelope instead, and this class is both ends of that:
    ///
    /// * <see cref="BlockForExport"/> reads the two names off the migrant's live <c>Species</c> record in
    ///   the same FixedUpdate as its serialization — never out of the payload, which has no name in it;
    /// * <see cref="Resolve"/> answers "which local species is this name" against
    ///   <c>GlobalLineageManager.recordedSpecies</c>, creating one when the name is new, so the importer
    ///   can rewrite <c>$.genes.speciesID</c> to a **local** id before the game's deserializer runs.
    ///
    /// Nothing here can refuse an organism. Every failure — no block, a malformed block, no registry, a
    /// create that throws — falls through to the absent-block rule (A32), which deletes the key and lets
    /// the game classify the arrival itself. Custody outranks bookkeeping: a name is recoverable on the
    /// next hop, and an organism refused at the door is not.
    /// </summary>
    internal class SpeciesRegistry
    {
        internal const string Prefix = "[M5-SPECIES]";

        internal const string ActionMatched = "matched";
        internal const string ActionCreated = "created";
        internal const string ActionFallback = "fallback";

        /// <summary>
        /// What one resolve produced. <see cref="resolved"/> false means the caller MUST take the
        /// absent-block rule — remove <c>$.genes.speciesID</c> — whatever the reason was (§16, A32).
        /// </summary>
        internal struct Resolution
        {
            internal bool resolved;
            internal long localId;

            /// <summary>matched | created | fallback, for the one grepable log line per import.</summary>
            internal string action;

            internal bool parentLinked;

            /// <summary>The name that was asked for, or "&lt;none&gt;" when no block arrived.</summary>
            internal string name;

            internal string detail;

            /// <summary>
            /// The species this resolve **created**, and therefore owns until the organism is standing.
            /// A created record carries a placeholder template until <see cref="AdoptTemplate"/> fills it
            /// from the restored organism, so an import that never produces one has to take it back out
            /// again (<see cref="Rollback"/>).
            /// </summary>
            internal Species created;
        }

        /// <summary>
        /// §11.2 — the per-tick resolve cache. <c>recordedSpecies</c> is a <c>List&lt;Species&gt;</c> and
        /// the match is a linear scan, so a burst of arrivals from one species would otherwise rescan the
        /// whole registry once per organism inside one FixedUpdate. It is deliberately **per tick**: a
        /// create in this tick must be visible to the next arrival in it, and no <c>Species</c> reference
        /// may be held across ticks, where a prune could have merged it away.
        /// </summary>
        private readonly Dictionary<string, Species> cache = new Dictionary<string, Species>(StringComparer.Ordinal);
        private long cacheTick = long.MinValue;

        internal void Clear()
        {
            cache.Clear();
            cacheTick = long.MinValue;
        }

        // ---- export (§5.3, §16 A30) -------------------------------------------------------------

        /// <summary>
        /// The <c>species</c> block for one migrant, or null when there is nothing to send.
        ///
        /// The source is <c>BibiteGenes.species</c>, the **live record**, read on the main thread in the
        /// same FixedUpdate as the migrant's own serialization. A null record is a normal state — a
        /// restored organism has one only once <c>ResumeBody</c> has run <c>CheckNewSpecies</c>
        /// (<c>BibiteBody.cs:477-480</c>) — and an omitted block is valid, never an error.
        ///
        /// **The halves are whitespace-normalized here and nowhere else** (§16, A34): trimmed at the
        /// edges, internal runs collapsed to one U+0020, before validation. The game's generator issues
        /// an edge space in about 2% of halves, which the wire rule refuses; before A34 every one of
        /// those organisms travelled with no block at all. Normalizing at the source is what keeps the
        /// wire rule intact — no sidecar, relay or archive may repair a name (A30's opacity rule) — and
        /// the importer closes the other half of the loop by comparing on normalized forms (A34), so a
        /// normalized arrival still finds a raw-form local species instead of founding a twin.
        /// </summary>
        internal static JObject BlockForExport(BibiteGenes genes, int entityId, out string summary)
        {
            summary = "<none>";

            Species species = (genes != null) ? genes.species : null;
            if (species == null)
            {
                return null;
            }

            SpeciesIdentity identity = new SpeciesIdentity
            {
                GenericName = ContractA.NormalizeSpeciesWhitespace(species.genericName),
                SpecificName = ContractA.NormalizeSpeciesWhitespace(species.specificName)
            };

            // One grepable line whenever A34 actually repaired something, so the rollout can be shown to
            // carry names that used to be dropped. Silent when the name was already conformant.
            if (!string.Equals(identity.Name, species.name, StringComparison.Ordinal))
            {
                MultiversePlugin.Log.LogInfo(
                    $"{Prefix} entity={entityId}: species name normalized for the wire (§16, A34): " +
                    $"raw=\"{species.name}\" -> \"{identity.Name}\".");
            }

            // §5.3 — the parent pair travels exactly when Species.parentSpecies is non-null, and exactly
            // one generation travels: a grandparent never does. A parent whose own name would break the
            // shape rules is dropped **as a pair**, which leaves a valid block that says "root": the
            // alternative is an invalid block, and an invalid block loses the migrant's own name too.
            Species parent = species.parentSpecies;
            if (parent != null)
            {
                string parentGeneric = ContractA.NormalizeSpeciesWhitespace(parent.genericName);
                string parentSpecific = ContractA.NormalizeSpeciesWhitespace(parent.specificName);

                if (ContractA.IsValidSpeciesNameHalf(parentGeneric, out string parentProblem)
                    && ContractA.IsValidSpeciesNameHalf(parentSpecific, out parentProblem))
                {
                    identity.ParentGenericName = parentGeneric;
                    identity.ParentSpecificName = parentSpecific;
                }
                else
                {
                    MultiversePlugin.Log.LogWarning(
                        $"{Prefix} entity={entityId}: the parent species name of '{species.name}' {parentProblem} — " +
                        "sending the block without the parent pair (§5.3: the pair is all-or-nothing). The migrant's " +
                        "species arrives as a root.");
                }
            }

            JObject block = ContractA.SpeciesBlock(identity, out string problem);
            if (block == null)
            {
                // Strip, never NACK (§5.3, §9.1). The organism migrates; only the label is lost, and the
                // destination applies the absent-block rule to it (§16, A32).
                MultiversePlugin.Log.LogWarning(
                    $"{Prefix} entity={entityId}: species '{species.name}' cannot be put on the wire ({problem}) — " +
                    "migrating without the species block.");
                return null;
            }

            summary = identity.ToString();
            return block;
        }

        // ---- import (§5.7 step 3(a), §16 A31) ----------------------------------------------------

        /// <summary>
        /// Resolve one arriving name against this world's own registry, on the Unity main thread and
        /// **before** the restore. Ordinal, case-sensitive equality against <c>Species.name</c>, over
        /// the full <c>recordedSpecies</c> list and not the active subset, **compared on the
        /// whitespace-normalized form of both sides** (§16, A34) — a normalized arrival has to find the
        /// raw-form local species it names, or the import founds a whitespace twin of it. Only the
        /// comparison is normalized; no stored name is ever rewritten.
        ///
        /// An exact name match **is** the same species (A31). That accepts a rare false merge — the
        /// game's name generator only checks uniqueness within one world — and the owner accepts it:
        /// the alternative is a genetic-distance threshold that has to mean something across two
        /// independently drifting populations, which is a much larger guess than a name collision.
        ///
        /// Never throws. A resolve that cannot produce a species returns <c>resolved == false</c>, and
        /// the caller deletes <c>$.genes.speciesID</c> instead (§16, A32).
        /// </summary>
        internal Resolution Resolve(SpeciesIdentity identity, long simTick)
        {
            Resolution result = default(Resolution);
            result.action = ActionFallback;
            result.name = "<none>";

            if (identity == null)
            {
                result.detail = "no usable species block arrived";
                return result;
            }

            result.name = identity.Name;

            try
            {
                if (simTick != cacheTick)
                {
                    cache.Clear();
                    cacheTick = simTick;
                }

                // §16 A34 — the cache is keyed on the normalized form, so the raw and normalized
                // spellings of one name share a slot instead of racing each other to create it.
                string cacheKey = ContractA.NormalizeSpeciesWhitespace(result.name);

                if (cache.TryGetValue(cacheKey, out Species remembered) && remembered != null)
                {
                    result.resolved = true;
                    result.localId = remembered.speciesID;
                    result.action = ActionMatched;
                    result.parentLinked = remembered.parentSpecies != null;
                    result.detail = string.Equals(remembered.name, result.name, StringComparison.Ordinal)
                        ? "from the per-tick cache"
                        : "from the per-tick cache, under the raw local form '" + remembered.name + "'";
                    return result;
                }

                GlobalLineageManager manager = GlobalLineageManager.Instance;
                if (manager == null || manager.recordedSpecies == null || manager.activeSpecies == null)
                {
                    result.detail = "GlobalLineageManager.Instance is not available";
                    return result;
                }

                Species match = FindByName(manager, result.name, out int matches, out bool exactForm);
                if (match != null)
                {
                    if (matches > 1)
                    {
                        // Two shapes of surprise, and they are worth telling apart. A true byte-identical
                        // duplicate should not exist — the game's own generator name-checks against
                        // recordedSpecies before it issues a name. A normalized-equal pair is the
                        // whitespace twin A34 exists to stop being created; a world that already holds
                        // one keeps it, and every arrival now lands on one of the two, deterministically.
                        MultiversePlugin.Log.LogWarning(
                            $"{Prefix} '{result.name}' matches {matches} records in recordedSpecies once whitespace is " +
                            $"normalized — taking the first (localId={match.speciesID}, stored name '{match.name}'). " +
                            "A within-world duplicate name is a game-side surprise; a whitespace twin is pre-A34 history.");
                    }

                    Reactivate(manager, match);
                    cache[cacheKey] = match;

                    result.resolved = true;
                    result.localId = match.speciesID;
                    result.action = ActionMatched;
                    result.parentLinked = match.parentSpecies != null;
                    result.detail = exactForm
                        ? "already recorded in this world"
                        : "already recorded in this world under the raw form '" + match.name +
                          "' — matched on the normalized form (§16, A34)";
                    return result;
                }

                // §5.7 step 3(a), no match: create one, under the named parent when this world has it.
                // The parent lookup runs through the same normalized comparison, so a local parent held
                // in its raw form is still found.
                Species parent = null;
                if (identity.HasParent)
                {
                    parent = FindByName(manager, identity.ParentName, out int _, out bool _);
                }

                Species fresh = CreateLocal(manager, identity, parent);
                cache[cacheKey] = fresh;

                result.resolved = true;
                result.localId = fresh.speciesID;
                result.action = ActionCreated;
                result.parentLinked = parent != null;
                result.created = fresh;
                result.detail = (parent != null)
                    ? "created under local parent '" + parent.name + "'"
                    : (identity.HasParent
                        ? "created as a root: '" + identity.ParentName + "' is not a species of this world"
                        : "created as a root: the migrant's species has no parent");
                return result;
            }
            catch (Exception e)
            {
                // §5.7 — a failure to resolve or create is never a NACK. Fall back to the absent-block
                // rule and let the organism spawn. A create that had already registered before the throw
                // is taken back out: a half-made record is worse than none.
                result.resolved = false;
                result.action = ActionFallback;
                result.detail = "the resolve threw: " + e.Message;
                Rollback(ref result);
                MultiversePlugin.Log.LogWarning($"{Prefix} resolving '{result.name}' threw — falling back to the absent-block rule. {e}");
                return result;
            }
        }

        /// <summary>
        /// The one grepable line per import (§16). It is emitted before the restore, so it is in the log
        /// even when the restore then fails.
        /// </summary>
        internal static void LogResolution(string migrationId, int entityId, Resolution resolution)
        {
            string line =
                $"{Prefix} migrationId={migrationId} entityId={entityId} action={resolution.action} " +
                $"name=\"{resolution.name}\" localId={(resolution.resolved ? resolution.localId.ToString(CultureInfo.InvariantCulture) : "none")} " +
                $"parentLinked={(resolution.parentLinked ? "true" : "false")} detail=\"{resolution.detail}\"";

            if (resolution.resolved)
            {
                MultiversePlugin.Log.LogInfo(line);
            }
            else
            {
                // Not an error: the absent-block rule is the floor for every import, and the game's own
                // genetic classifier is what runs next (§16, A32).
                MultiversePlugin.Log.LogInfo(line + " — the absent-block rule applies: $.genes.speciesID is removed.");
            }
        }

        /// <summary>
        /// Finish a species this import created, now that the organism is standing.
        ///
        /// A species created before the restore cannot have a genome yet — the migrant is still a JSON
        /// string — so <see cref="CreateLocal"/> gives it a placeholder <c>BibiteTemplate</c>, and this
        /// replaces it with the real one, exactly as the game's own <c>Species(BibiteBody, parent)</c>
        /// constructor builds it (<c>Species.cs:101-126</c>) and **without** its
        /// <c>GenerateNewName()</c> call, which would throw the arriving identity away.
        ///
        /// This matters beyond tidiness: <c>Species.template</c> is what every
        /// <c>GeneticDistanceToIndividual</c> reads, so a species left holding a placeholder would push
        /// its own members away in <c>CheckNewSpecies</c> and re-split the lineage §16 just merged, and
        /// <c>Species.SaveState</c> walks the same arrays on the next world save.
        /// </summary>
        internal void AdoptTemplate(ref Resolution resolution, BibiteBody body)
        {
            Species created = resolution.created;
            if (created == null)
            {
                return;
            }

            if (body == null || body.gene == null || !ReferenceEquals(body.gene.species, created))
            {
                string bound = (body != null && body.gene != null && body.gene.species != null) ? body.gene.species.name : "<none>";
                MultiversePlugin.Log.LogWarning(
                    $"{Prefix} '{created.name}' (localId={created.speciesID}) was created for this arrival but the restored " +
                    $"organism bound species '{bound}' instead — unregistering the unused record.");
                Rollback(ref resolution);
                return;
            }

            try
            {
                BibiteTemplate template = new BibiteTemplate(body);
                template.speciesName = created.name;
                template.name = created.name;

                Species parent = created.parentSpecies;
                if (parent != null && parent.template != null && parent.template.nodeAnchors != null && parent.template.nodeAnchors.Count > 0)
                {
                    template.nodeAnchors = new Dictionary<long, Vector2>(parent.template.nodeAnchors);
                }

                created.template = template;
                created.generationOfFirstSpecimen = body.gene.generation;
            }
            catch (Exception e)
            {
                // The placeholder template is valid (non-null arrays, default gene values), so the world
                // still saves and no distance computation throws — the species is just genetically
                // uninformative until one of its members speciates. Never a reason to fail an arrival.
                MultiversePlugin.Log.LogWarning(
                    $"{Prefix} '{created.name}' (localId={created.speciesID}) kept its placeholder template — " +
                    $"building one from the restored organism threw: {e.Message}");
            }

            resolution.created = null;
        }

        /// <summary>
        /// Take back a species this import created when the organism never stood up. Without this, a
        /// permanently failed restore would leave a named, memberless record holding a placeholder
        /// template, and the **next** migrant of that species would match it and inherit the placeholder
        /// — which is the mis-classification §16 exists to remove.
        ///
        /// The burnt <c>speciesID</c> is not reclaimed: <c>Species.speciesMaxID</c> is monotonic and a
        /// skipped value is harmless, while rewinding a counter that something else may already have
        /// drawn from is not.
        /// </summary>
        internal void Rollback(ref Resolution resolution)
        {
            Species created = resolution.created;
            resolution.created = null;
            if (created == null)
            {
                return;
            }

            try
            {
                cache.Remove(created.name);

                GlobalLineageManager manager = GlobalLineageManager.Instance;
                if (manager != null)
                {
                    manager.activeSpecies?.Remove(created);
                    manager.recordedSpecies?.Remove(created);
                    SafeUpdateSpan(manager);
                }

                created.parentSpecies?.children.Remove(created);
                created.parentSpecies = null;
                created.rootSpecies = created;

                MultiversePlugin.Log.LogInfo(
                    $"{Prefix} unregistered '{created.name}' (localId={created.speciesID}): the arrival it was created for " +
                    "did not complete, so the record would have had no members and no genome.");
            }
            catch (Exception e)
            {
                MultiversePlugin.Log.LogWarning($"{Prefix} could not unregister '{created.name}' — {e.Message}");
            }
        }

        // ---- the game's own registration, reproduced -------------------------------------------

        /// <summary>
        /// Ordinal, case-sensitive match against <c>Species.name</c> over the whole
        /// <c>recordedSpecies</c> list — the same predicate <c>FindSpeciesFromTemplate</c> uses
        /// (<c>GlobalLineageManager.cs:273</c>) — **compared on the whitespace-normalized form of both
        /// sides** (§16, A34).
        ///
        /// The normalization is for the comparison and nothing else: the stored
        /// <c>Species.name</c> is never touched, and neither is the arriving one. It has to work this
        /// way because a destination registry is full of **raw** names its own evolution produced
        /// ("Izus  copedylanus", with the doubled space), while the wire now carries the normalized form
        /// of the same name. A raw comparison would miss and the importer would found a second species
        /// differing from the first only in whitespace — a twin that reads identically in every UI.
        ///
        /// A byte-for-byte match still wins when one exists, so a world holding both forms resolves to
        /// the one that was actually asked for. <paramref name="exactForm"/> reports which happened, and
        /// <paramref name="matches"/> counts every normalized-equal candidate so a duplicate can be
        /// reported.
        /// </summary>
        private static Species FindByName(GlobalLineageManager manager, string name, out int matches, out bool exactForm)
        {
            matches = 0;
            exactForm = false;

            Species firstExact = null;
            Species firstNormalized = null;
            string target = ContractA.NormalizeSpeciesWhitespace(name);

            List<Species> recorded = manager.recordedSpecies;
            for (int i = 0; i < recorded.Count; i++)
            {
                Species candidate = recorded[i];
                if (candidate == null || candidate.name == null)
                {
                    continue;
                }

                if (!string.Equals(ContractA.NormalizeSpeciesWhitespace(candidate.name), target, StringComparison.Ordinal))
                {
                    continue;
                }

                matches++;

                if (firstNormalized == null)
                {
                    firstNormalized = candidate;
                }

                if (firstExact == null && string.Equals(candidate.name, name, StringComparison.Ordinal))
                {
                    firstExact = candidate;
                }
            }

            if (firstExact != null)
            {
                exactForm = true;
                return firstExact;
            }

            return firstNormalized;
        }

        /// <summary>
        /// Re-activate a recorded species, exactly as <c>FindSpeciesFromTemplate</c> does
        /// (<c>GlobalLineageManager.cs:278-296</c>): insert it into <c>activeSpecies</c> at the position
        /// that keeps that list in <c>recordedSpecies</c> order, append when it belongs last, and
        /// recompute the effective species span.
        ///
        /// **A migrant reviving a locally extinct species of the same name is the correct outcome**
        /// (§5.7): the name is the same species, and the species genuinely came back.
        /// </summary>
        private static void Reactivate(GlobalLineageManager manager, Species species)
        {
            List<Species> active = manager.activeSpecies;
            if (active.Contains(species))
            {
                return;
            }

            int index = manager.recordedSpecies.IndexOf(species);
            for (int i = 0; i < active.Count; i++)
            {
                if (manager.recordedSpecies.IndexOf(active[i]) > index)
                {
                    active.Insert(i, species);
                    break;
                }
            }

            if (!active.Contains(species))
            {
                active.Add(species);
            }

            SafeUpdateSpan(manager);
        }

        /// <summary>
        /// Create one local species with **exactly** the arriving names, and register it the way
        /// <c>CreateNewSpecies(bibite, parentSpecies)</c> does (<c>GlobalLineageManager.cs:317-344</c>):
        /// inserted straight after its parent in <c>recordedSpecies</c> and into <c>activeSpecies</c> in
        /// that same order when there is a parent, appended to both when there is not.
        ///
        /// The construction is the one thing that cannot be delegated. Both public <c>Species</c>
        /// constructors name the species themselves — <c>Species(BibiteTemplate)</c> capitalizes and
        /// lower-cases the halves it splits out, and <c>Species(BibiteBody, parent)</c> calls
        /// <c>GenerateNewName()</c> and takes the genus from the parent — and a mod that lets either
        /// stand has thrown the identity away (§5.7 step 3(a)). So the parameterless constructor is used
        /// and every field those two set is set here instead: the id from the same monotonic
        /// <c>Species.speciesMaxID</c> counter, the creation time, the root, and the parent's child list.
        /// </summary>
        private static Species CreateLocal(GlobalLineageManager manager, SpeciesIdentity identity, Species parent)
        {
            Species species = new Species
            {
                // Species.cs:89, 103 — the id discipline both constructors share. It is world-local by
                // design, which is exactly why it is minted here and never carried on the wire.
                speciesID = Species.speciesMaxID++,
                timeCreation = TimeKeeper.simulatedTime,

                // Byte for byte as the origin world holds them. Never re-generated, never tidied.
                genericName = identity.GenericName,
                specificName = identity.SpecificName
            };

            if (parent != null)
            {
                species.parentSpecies = parent;
                species.rootSpecies = parent.rootSpecies ?? parent;
                parent.children.Add(species);
            }
            else
            {
                species.rootSpecies = species;
            }

            // The genome the species is "about" only exists once the payload is restored, so the record
            // starts with a valid placeholder and AdoptTemplate replaces it in the same FixedUpdate.
            // Valid matters: Species.SaveState walks nodes, synapses and genes on every world save, and
            // GeneticDistanceToIndividual indexes genes on every classification.
            species.template = PlaceholderTemplate(species.name);

            if (parent != null)
            {
                int index = manager.recordedSpecies.IndexOf(parent) + 1;
                manager.recordedSpecies.Insert(index, species);
                for (int i = 0; i < manager.activeSpecies.Count; i++)
                {
                    if (manager.recordedSpecies.IndexOf(manager.activeSpecies[i]) > index)
                    {
                        manager.activeSpecies.Insert(i, species);
                        break;
                    }
                }

                if (!manager.activeSpecies.Contains(species))
                {
                    manager.activeSpecies.Add(species);
                }
            }
            else
            {
                manager.recordedSpecies.Add(species);
                manager.activeSpecies.Add(species);
            }

            SafeUpdateSpan(manager);
            return species;
        }

        /// <summary>
        /// A <c>BibiteTemplate</c> that is safe to hold and safe to save: default gene values and empty
        /// brain arrays. <c>new BibiteTemplate()</c> alone leaves <c>genes</c>, <c>nodes</c> and
        /// <c>synapses</c> null, and every one of them is dereferenced without a guard by
        /// <c>Species.SaveState</c> and by the distance functions.
        /// </summary>
        private static BibiteTemplate PlaceholderTemplate(string name)
        {
            BibiteTemplate template = new BibiteTemplate
            {
                name = name,
                speciesName = name,
                nodes = new NEATBrain.Node[0],
                synapses = new NEATBrain.Synaps[0],
                genes = new float[BibiteGenes.NGene]
            };

            try
            {
                for (int i = 0; i < template.genes.Length; i++)
                {
                    GeneSetting setting = BibiteEditorSettings.SettingOfGene(i);
                    template.genes[i] = (setting != null) ? setting.DefaultValue : 0f;
                }
            }
            catch (Exception e)
            {
                MultiversePlugin.Log.LogWarning($"{Prefix} could not seed a placeholder gene array with the editor defaults — {e.Message}");
            }

            return template;
        }

        /// <summary>
        /// <c>UpdateEffectiveSpeciesSpan</c> is what every registration path calls last, and it writes to
        /// a UI handle. A UI hiccup must not be able to undo a registration that already happened, so it
        /// is called on its own.
        /// </summary>
        private static void SafeUpdateSpan(GlobalLineageManager manager)
        {
            try
            {
                manager.UpdateEffectiveSpeciesSpan();
            }
            catch (Exception e)
            {
                MultiversePlugin.Log.LogWarning($"{Prefix} GlobalLineageManager.UpdateEffectiveSpeciesSpan threw — {e.Message}");
            }
        }
    }
}
