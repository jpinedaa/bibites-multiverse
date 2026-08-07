using System;
using System.Collections.Generic;
using System.Text;
using SimulationScripts.BibiteScripts;

namespace BibitesMultiverse
{
    /// <summary>
    /// The migration exclusion list (D18, contract-a.md §18 A39, §4.3.1, §10).
    ///
    /// A named species is **never captured**, so it never exports. §4.3.1's capture predicate gains one
    /// conjunct — <c>not excluded by local policy</c> — and nothing else in the pipeline changes.
    ///
    /// Four properties of A39 are structural here rather than remembered:
    ///
    /// * **It is a skip, not a refusal.** No <c>MIGRATE_OUT</c> is sent, so there is no NACK, no journal
    ///   entry, no custody question and nothing to correlate. The test therefore runs **before** the
    ///   freeze and the serialization, not after: an organism the policy stops must never have paid for
    ///   <c>SerializeBibite</c>, the parent-blob collection or the frame.
    /// * **It matches on the §16 A34-normalized form, on both sides.** The game issues an edge space in
    ///   about 2% of generated name halves, so a raw comparison would silently fail to exclude
    ///   <c>Basic  bibite</c>. Only the comparison is normalized: no local <c>Species</c> record is ever
    ///   rewritten.
    /// * **An organism the policy cannot name is never excluded.** A null <c>Species</c>, or a half that
    ///   normalizes to nothing, means the mod cannot identify its subject — and a policy that cannot
    ///   identify its subject MUST NOT guess (the same rule §16 A32 applies to import).
    /// * **One log line per organism per session.** An excluded organism loiters in a band for its whole
    ///   life, and a per-tick line would be the most expensive log in the system. The line is keyed on
    ///   <c>entityId</c> and is evidence that the policy fired without being a cost.
    ///
    /// The verdict is memoized on the <c>Species</c> **reference**, so the steady-state cost of an
    /// excluded population sitting in four capture bands is one dictionary probe per organism per tick —
    /// no string is built and nothing is allocated after a species is first seen.
    ///
    /// This is export-side only and purely local: it never refuses an arrival, it is invisible to the
    /// sidecar, the relay and the archive, and it is **not** a census filter — an excluded species lives
    /// in this world like any other and §17's census reports it (A39, "Not a census filter").
    /// </summary>
    internal sealed class MigrationExclusion
    {
        /// <summary>
        /// Milestone and decision, the way <see cref="Containment"/> carries <c>[M3-D10]</c>: this is
        /// D18, ratified against the running M4 deployment.
        /// </summary>
        internal const string Prefix = "[M4-D18]";

        /// <summary>§10 <c>migrationExcludeSpecies</c> — the shipped default: the seed stock stays home.</summary>
        internal const string DefaultNames = "Basic bibite";

        /// <summary>
        /// How many distinct organisms may be named in the log before it falls back to one summary line.
        /// A39 asks for one line per organism; this keeps that promise bounded on a session that runs for
        /// days, where the set itself would otherwise be the cost the amendment was avoiding.
        /// </summary>
        private const int MaxLoggedOrganisms = 4096;

        private readonly HashSet<string> names = new HashSet<string>(StringComparer.Ordinal);
        private readonly List<string> ordered = new List<string>();

        /// <summary>
        /// Species reference to verdict. Cleared with the world, because the records die with it. A
        /// species' halves are public mutable fields, so a **rename** inside a running world keeps the
        /// old verdict until the next world load — accepted deliberately: the alternative is rebuilding
        /// the name on every organism on every tick, which is the cost A39 put the lookup here to avoid.
        /// </summary>
        private readonly Dictionary<Species, bool> verdicts = new Dictionary<Species, bool>();

        private readonly HashSet<int> logged = new HashSet<int>();
        private readonly HashSet<int> allowed = new HashSet<int>();

        private long organismsLogged;
        private long refusals;
        private bool logCapReported;

        /// <summary>True when at least one name is configured. An empty list disables the policy (A39).</summary>
        internal bool Enabled => names.Count > 0;

        /// <summary>Distinct organisms this session whose capture the policy stopped, as far as the log names them.</summary>
        internal long OrganismsExcluded => organismsLogged;

        /// <summary>Band ticks refused. Larger than <see cref="OrganismsExcluded"/>: an excluded organism loiters.</summary>
        internal long Refusals => refusals;

        /// <summary>
        /// §10 — comma-separated **full** species names. Each is normalized once, here, so the hot path
        /// compares two already-normalized strings. An entry that normalizes to nothing is dropped.
        /// </summary>
        internal void Configure(string text)
        {
            names.Clear();
            ordered.Clear();
            Reset();

            if (string.IsNullOrEmpty(text))
            {
                return;
            }

            foreach (string part in text.Split(','))
            {
                string name = ContractA.NormalizeSpeciesWhitespace(part);
                if (string.IsNullOrEmpty(name))
                {
                    continue;
                }

                if (names.Add(name))
                {
                    ordered.Add(name);
                }
            }
        }

        /// <summary>The configured list, for a log line.</summary>
        internal string Describe()
        {
            if (ordered.Count == 0)
            {
                return "<disabled>";
            }

            StringBuilder text = new StringBuilder();
            for (int i = 0; i < ordered.Count; i++)
            {
                if (i > 0)
                {
                    text.Append(", ");
                }

                text.Append('"').Append(ordered[i]).Append('"');
            }

            return text.ToString();
        }

        /// <summary>A new world is a new session: new <c>Species</c> records and new entity ids.</summary>
        internal void Reset()
        {
            verdicts.Clear();
            logged.Clear();
            allowed.Clear();
            organismsLogged = 0;
            refusals = 0;
            logCapReported = false;
        }

        /// <summary>An organism left this world, or arrived in it. Its id carries nothing forward.</summary>
        internal void Forget(int entityId)
        {
            logged.Remove(entityId);
            allowed.Remove(entityId);
        }

        /// <summary>
        /// The rig's forced export (<see cref="DevCommands"/>) exempts one organism, exactly as it drops
        /// that organism's export cooldown and entry-immunity window. The dev channel exists to make a
        /// capture happen on demand, and the default list names the species every seeded world is full
        /// of — so without this the M4 rig could not force a single hop. It relaxes nothing else, and it
        /// is cleared the moment the organism departs.
        /// </summary>
        internal void AllowOnce(int entityId)
        {
            if (!Enabled)
            {
                return;
            }

            if (allowed.Add(entityId))
            {
                MultiversePlugin.Log.LogInfo(
                    $"{Prefix} entityId={entityId} is exempted from the migration exclusion list for this world — " +
                    "a forced export from the dev channel, never a natural capture (§18, A39).");
            }
        }

        /// <summary>
        /// §4.3.1's <c>not excluded by local policy</c> conjunct, evaluated every FixedUpdate for an
        /// organism that is inside some capture band. Never throws: a policy failure must not be able to
        /// stop the pipeline, and the safe answer is always "not excluded".
        /// </summary>
        internal bool Excludes(BibiteBody body, int entityId)
        {
            if (names.Count == 0 || body == null)
            {
                return false;
            }

            if (allowed.Count > 0 && allowed.Contains(entityId))
            {
                return false;
            }

            Species species;
            try
            {
                BibiteGenes genes = body.gene;
                species = (genes != null) ? genes.species : null;
            }
            catch (Exception)
            {
                return false;
            }

            // A39 — an organism with no species is never excluded. A restored organism has no record
            // until ResumeBody has run CheckNewSpecies, and the mod cannot name what it cannot read.
            if (species == null)
            {
                return false;
            }

            if (!verdicts.TryGetValue(species, out bool excluded))
            {
                excluded = Matches(species);
                verdicts[species] = excluded;
            }

            if (!excluded)
            {
                return false;
            }

            refusals++;
            Note(entityId, species);
            return true;
        }

        /// <summary>
        /// The full name — both halves joined by one U+0020 — compared on the A34-normalized form. The
        /// game's <c>Species.name</c> is a computed property that concatenates on every read, so the
        /// halves are read directly and the string is built at most once per species per world.
        /// </summary>
        private bool Matches(Species species)
        {
            string generic;
            string specific;
            try
            {
                generic = ContractA.NormalizeSpeciesWhitespace(species.genericName);
                specific = ContractA.NormalizeSpeciesWhitespace(species.specificName);
            }
            catch (Exception)
            {
                return false;
            }

            if (string.IsNullOrEmpty(generic) || string.IsNullOrEmpty(specific))
            {
                // Half a name is not a weaker identity, it is a different one. The policy does not guess.
                return false;
            }

            return names.Contains(generic + " " + specific);
        }

        /// <summary>A39's logging rule: once per organism per session, then one line saying so.</summary>
        private void Note(int entityId, Species species)
        {
            if (logged.Count >= MaxLoggedOrganisms)
            {
                if (!logCapReported)
                {
                    logCapReported = true;
                    MultiversePlugin.Log.LogInfo(
                        $"{Prefix} {MaxLoggedOrganisms} organisms have now been named on this world's exclusion log — " +
                        "further exclusions are counted but not named individually. The policy is unchanged.");
                }

                return;
            }

            if (!logged.Add(entityId))
            {
                return;
            }

            organismsLogged++;

            string name;
            try
            {
                name = species.name;
            }
            catch (Exception)
            {
                name = "<unreadable>";
            }

            MultiversePlugin.Log.LogInfo(
                $"{Prefix} event=EXCLUDED entityId={entityId} species=\"{name}\" — this species is on the migration " +
                $"exclusion list, so it is never captured on any edge and never exports (§18, A39). It stays in this " +
                $"world, the wrap contains it (D10), and the census still reports it. organismsExcluded={organismsLogged}");
        }

        /// <summary>One line for the session summary, so a policy that never fired is still countable.</summary>
        internal string Summary()
        {
            return $"list={Describe()} organismsExcluded={organismsLogged} bandTicksRefused={refusals}";
        }
    }
}
