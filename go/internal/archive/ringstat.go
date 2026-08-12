package archive

// ringstat: the status page as a terminal table, over EXACTLY the same data
// (§10.1, WP5). It reads the archive's own JSON endpoint, or the durable
// metrics file when the archive is not running, and it renders the same
// unknowns the page renders — an unknown reads as unknown and a zero reads as a
// measurement.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"multiverse/internal/contractb"
	"multiverse/internal/termsafe"
)

// FetchStatus reads one Status from an archive's HTTP endpoint.
func FetchStatus(base string, timeout time.Duration) (Status, error) {
	url := strings.TrimSuffix(base, "/") + "/api/status"
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		return Status{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Status{}, fmt.Errorf("archive: %s answered HTTP %d", url, resp.StatusCode)
	}
	var s Status
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return Status{}, err
	}
	return s, nil
}

// FetchSpecies reads one SpeciesIndex from an archive's HTTP endpoint. It is
// the ONLY thing in ringstat that is not available from the durable sample
// file, and it is optional for exactly that reason: the ledger annotations —
// crossings ever, distinct genomes, the lanes a species uses — are derived from
// migrations.jsonl, which the sample file does not carry and a terminal tool
// must not re-read per invocation. Against a metrics file the species view
// still answers the census half in full, and says the other half is unavailable
// rather than printing a zero.
func FetchSpecies(base string, timeout time.Duration) (SpeciesIndex, error) {
	url := strings.TrimSuffix(base, "/") + "/api/species"
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		return SpeciesIndex{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return SpeciesIndex{}, fmt.Errorf("archive: %s answered HTTP %d", url, resp.StatusCode)
	}
	var s SpeciesIndex
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return SpeciesIndex{}, err
	}
	return s, nil
}

// LastSample reads the newest sample out of a metrics file.
func LastSample(path string) (Status, error) {
	samples, err := ReadMetrics(path)
	if err != nil {
		return Status{}, err
	}
	if len(samples) == 0 {
		return Status{}, errors.New("archive: the metrics file holds no sample yet")
	}
	return samples[len(samples)-1], nil
}

// RenderRingstat writes the terminal table.
func RenderRingstat(w io.Writer, s Status) {
	fmt.Fprintf(w, "map %dx%d   %d slot(s), %d hole(s)   epoch %d   observers %d\n",
		s.Map.Width, s.Map.Height, s.SlotCount, len(s.Holes), s.Epoch, s.Observers)
	link := "linked"
	if !s.RelayConnected {
		link = "RELAY LINK DOWN"
	}
	fmt.Fprintf(w, "archive %s (%s)   state %s old   %d ledger record(s), %d genome gap(s)\n",
		s.ArchivePeerID, link, dur(s.StatusAgeMs), s.Records, s.Gaps)
	// Only for a damaged ledger, and then on its own line, because it is the one
	// thing on this screen that says the record of what happened is incomplete.
	if s.LedgerSkipped > 0 {
		fmt.Fprintf(w, "LEDGER DAMAGE: %d line(s) unreadable and skipped on replay\n", s.LedgerSkipped)
	}
	fmt.Fprintln(w)

	// speed is the world's time scale — what the game reports applying, then
	// after the arrow what the archive measured it delivering over the last
	// achievedWindowMs — and pace is queued/cap per SIMULATED minute of that
	// world. They are the same cells the map draws, so the two operator
	// surfaces cannot disagree about them either.
	fmt.Fprintf(w, "%-5s %-7s %-22s %-9s %s %10s %8s %10s %6s %8s %s\n",
		"slot", "pos", "peer", "state", padCell("speed→got", 13),
		"population", "custody", "pace", "held", "bounced", "last save")
	fmt.Fprintln(w, strings.Repeat("-", 128))
	for _, v := range s.Slots {
		state := "live"
		if !v.Live {
			state = "DARK " + dur(v.DarkForMs)
		} else if !v.ModConnected {
			state = "no mod"
		}
		save := "unknown"
		if v.LastSave != nil && v.LastSaveAgeMs != nil {
			save = dur(*v.LastSaveAgeMs) + " ago"
			if v.LastSave.DurationMs > 0 {
				save += fmt.Sprintf(" (%dms)", v.LastSave.DurationMs)
			}
		}
		fmt.Fprintf(w, "%-5d %-7s %-22s %-9s %s %10s %8s %10s %6s %8s %s\n",
			v.Slot, fmt.Sprintf("(%d,%d)", v.Position.Col, v.Position.Row),
			trunc(v.PeerID, 22), state, padCell(speedPair(v), 13),
			opt(v.Population), opt(v.CustodyDepth),
			optShort(v.PacedDepth)+"/"+scale(v.InboundRatePerSimMinute), opt(v.HeldDepth),
			opt(v.BouncedTimeoutTotal), save)
	}
	if len(s.Holes) > 0 {
		holes := make([]string, 0, len(s.Holes))
		for _, h := range s.Holes {
			holes = append(holes, fmt.Sprintf("(%d,%d)", h.Col, h.Row))
		}
		sort.Strings(holes)
		fmt.Fprintf(w, "\nholes: %s   (both axes route around them)\n", strings.Join(holes, " "))
	}

	fmt.Fprintf(w, "\n%-6s %-5s %-10s %-28s %10s %8s\n",
		"from", "edge", "to", "state", "envelopes", "/min")
	fmt.Fprintln(w, strings.Repeat("-", 76))
	for _, l := range s.Lanes {
		to, state := "—", "closed: "+l.Reason
		if l.Open {
			to = fmt.Sprintf("slot %d", l.ToSlot)
			state = "open"
		}
		if len(l.Skipped) > 0 {
			parts := make([]string, 0, len(l.Skipped))
			for _, k := range l.Skipped {
				if k.Slot == nil {
					parts = append(parts, fmt.Sprintf("hole(%d,%d)", k.Position.Col, k.Position.Row))
					continue
				}
				parts = append(parts, fmt.Sprintf("slot %d:%s", *k.Slot, k.Reason))
			}
			state += " [bypassing " + strings.Join(parts, ", ") + "]"
		}
		fmt.Fprintf(w, "%-6d %-5s %-10s %-28s %10d %8.2f\n",
			l.FromSlot, l.Edge, to, state, l.Migrations, l.PerMinute)
	}

	renderCensus(w, s)

	t := s.Totals
	fmt.Fprintf(w, "\ntotals: %d live, %d dark, %d hole(s); population %s, custody %s, paced %s, held %s\n",
		t.LiveSlots, t.DarkSlots, t.Holes,
		opt(t.Population), opt(t.CustodyDepth), opt(t.PacedDepth), opt(t.HeldDepth))
	fmt.Fprintf(w, "        %s bounce(s) caused by a hold timeout; %d slot(s) reporting nothing\n",
		opt(t.TimeoutBounce), t.UnknownSlots)
	fmt.Fprintf(w, "        %d envelope(s) recorded, %.2f/min across every lane\n",
		t.Migrations, t.PerMinute)
	fmt.Fprintf(w, "\nan unknown is a slot that reported nothing, or reported it too long ago.\n"+
		"a zero is a measurement. the lanes are recomputed for display; the relay's\n"+
		"SECTOR_GRANT is the authority for what a peer actually routes.\n")
}

// renderCensus is the species half of the operator surface in the terminal —
// the same census the page draws, off the same Status, so the two tools cannot
// disagree about which species live where (contract-b-m4.md §10.1, §16 B12).
//
// It obeys §10.1's rules exactly as the page does: an absent census is UNKNOWN
// and says so, a present empty one is the different and stronger fact, a
// truncated one says the rest is unreported, and NO NAME IS REPAIRED — the raw
// spelling the owning world holds is what is printed, doubled spaces and all
// (contract-a.md §17, A36).
func renderCensus(w io.Writer, s Status) {
	if len(s.Slots) == 0 {
		return
	}
	fmt.Fprintf(w, "\n%-6s %s\n", "slot", "species (living + eggs, most numerous first, "+
		"named exactly as the owning world spells them)")
	fmt.Fprintln(w, strings.Repeat("-", 100))
	for _, v := range s.Slots {
		switch {
		case !v.StatsKnown || !v.SpeciesKnown:
			fmt.Fprintf(w, "%-6d unknown — this world reports no census\n", v.Slot)
			continue
		case len(v.Species) == 0:
			fmt.Fprintf(w, "%-6d none alive — this world is reporting, and holds no species\n", v.Slot)
			continue
		}
		parts := make([]string, 0, len(v.Species))
		for _, e := range v.Species {
			// NO NAME IS REPAIRED and every name is SANITIZED, and the two are
			// different obligations: the raw spelling the owning world holds is
			// what is printed, doubled spaces and all (contract-a.md §17, A36),
			// with anything a terminal would execute replaced (§22, B30).
			part := fmt.Sprintf("%s ×%d", safeTerm(e.GenericName+" "+e.SpecificName), e.Bibites)
			if e.Eggs > 0 {
				part += fmt.Sprintf("+%degg", e.Eggs)
			}
			parts = append(parts, part)
		}
		line := strings.Join(parts, ", ")
		if v.SpeciesTruncated {
			line += "  [32 most numerous only; the rest is UNREPORTED]"
		}
		fmt.Fprintf(w, "%-6d %s\n", v.Slot, line)
	}
}

// RenderSettings is the SETTINGS view in the terminal — one block per world,
// over exactly the data the settings tab renders, from the same Status
// (contract-b-m4.md §19, B19). It obeys the same two rules the page does:
//
//	UNKNOWN IS PRINTED, NEVER SUBSTITUTED. There is no default anywhere in this
//	chain to substitute FROM, and the one that would be reached for —
//	saveMinutes 10, which is what the mod ships with — would claim a world is
//	being saved when its timer may be off.
//
//	AND IT IS READ-ONLY. This tool renders settings and offers no way to change
//	one; a control surface is a separate design (contract-a.md §19, A43).
func RenderSettings(w io.Writer, s Status) {
	fmt.Fprintf(w, "map %dx%d   %d slot(s)   epoch %d   state %s old\n",
		s.Map.Width, s.Map.Height, s.SlotCount, s.Epoch, dur(s.StatusAgeMs))
	fmt.Fprintf(w, "READ-ONLY: what each world was told to do. This tool changes nothing.\n\n")
	renderPublished(w, s)
	if len(s.Slots) == 0 {
		fmt.Fprintln(w, "no slots reserved yet")
		return
	}
	fmt.Fprintf(w, "%-5s %-16s %-9s %-12s %-14s %-9s %-7s %-6s %-5s %s\n",
		"slot", "peer", "state", "mod", "contract-a", "game", "save", "keep",
		"quit", "wrap")
	fmt.Fprintln(w, strings.Repeat("-", 118))
	for _, v := range s.Slots {
		state := "live"
		if !v.Live {
			state = "DARK"
		} else if !v.ModConnected {
			state = "no mod"
		}
		fmt.Fprintf(w, "%-5d %-16s %-9s %-12s %-14s %-9s %-7s %-6s %-5s %s\n",
			v.Slot, trunc(v.PeerID, 16), state,
			str(v.ModVersion), str(v.ContractAVersion), str(v.GameVersion),
			saveEvery(v.SaveMinutes), optShortInt(v.SaveKeep),
			optBool(v.SaveOnQuit), optBool(v.WorldWrapping))
	}
	fmt.Fprintf(w, "\n%-5s %s\n", "slot", "species this world never exports")
	fmt.Fprintln(w, strings.Repeat("-", 100))
	for _, v := range s.Slots {
		switch {
		case !v.MigrationExcludeKnown:
			fmt.Fprintf(w, "%-5d unknown — this world has not told us\n", v.Slot)
		case len(v.MigrationExclude) == 0:
			// A PRESENT EMPTY LIST is a stronger, different statement than
			// absence: the exclusion policy is switched off.
			fmt.Fprintf(w, "%-5d none — the exclusion policy is off\n", v.Slot)
		default:
			// migrationExclude is another attacker-choosable string array on
			// §10.1's inventory, on the same rendering surface (§22, B30).
			names := make([]string, 0, len(v.MigrationExclude))
			for _, n := range v.MigrationExclude {
				names = append(names, safeTerm(n))
			}
			fmt.Fprintf(w, "%-5d %s\n", v.Slot, strings.Join(names, ", "))
		}
	}
	fmt.Fprintf(w, "\na '?' is a world that has not published that setting — an older mod, or an\n"+
		"older sidecar. It is NEVER the value the game ships with. 'save OFF' is a\n"+
		"reading: that world's save timer is off, which is why it may never report a\n"+
		"last save. these settings are read-only everywhere in this system.\n")
}

// renderPublished prints WHAT THE MAP ITSELF IS RUNNING WITH, above the worlds
// that are measured against it (contract-b-m4.md §22, B24 and B25). It is the
// same two values the settings tab draws, off the same Status, so the terminal
// and the page cannot disagree about the ceilings either of them quotes.
//
// They are the only lines in this view that are AUTHORITATIVE rather than
// reported — a world's setting is that world's claim about itself, and these are
// the relay's own configuration — and the two absences are DIFFERENT FACTS:
//
//	NO TABLE is a relay older than the table (B24). It reads UNKNOWN, never as
//	"this map has no ceilings", and never as the defaults this build ships with.
//
//	NO FLOOR is the relay's real answer (B25): every compatible version is
//	admitted, which is the default and a decision rather than a gap.
func renderPublished(w io.Writer, s Status) {
	floor := s.MinContractVersion
	if floor == "" {
		floor = "none — every compatible version is admitted"
	}
	fmt.Fprintf(w, "the map itself: oldest helper version admitted: %s\n", safeTerm(floor))
	if len(s.Limits) == 0 {
		fmt.Fprintf(w, "  ceilings: unknown — this map publishes none, which means a relay older\n"+
			"  than the published table. It is NOT a map without ceilings.\n\n")
		return
	}
	fmt.Fprintln(w, "  ceilings every world here is measured against, as the relay publishes them:")
	seen := map[string]bool{}
	for _, k := range contractb.PublishedLimitKeys {
		seen[k] = true
		v, ok := s.Limits[k]
		if !ok {
			// A published table with a hole in it is not a table (§3.3): a key
			// this map did not publish is an unknown ceiling, and this tool has a
			// default for it that it must not print.
			fmt.Fprintf(w, "  %-28s unknown\n", k)
			continue
		}
		fmt.Fprintf(w, "  %-28s %d\n", k, v)
	}
	// A relay may publish a ceiling this build has never heard of, and dropping
	// it would be this tool deciding what the table contains.
	extra := make([]string, 0, len(s.Limits))
	for k := range s.Limits {
		if !seen[k] {
			extra = append(extra, k)
		}
	}
	sort.Strings(extra)
	for _, k := range extra {
		fmt.Fprintf(w, "  %-28s %d\n", trunc(safeTerm(k), 28), s.Limits[k])
	}
	fmt.Fprintln(w)
}

// RenderSpecies is the SPECIES view in the terminal: every species alive
// anywhere right now, joined to what the crossing record knows about it. idx is
// nil when the source was a metrics file rather than a running archive, and the
// ledger columns then say so instead of printing zeros.
func RenderSpecies(w io.Writer, s Status, idx *SpeciesIndex) {
	rows, reporting, censusless, truncated := aliveSpecies(s)
	fmt.Fprintf(w, "map %dx%d   state %s old   %d world(s) reporting a census",
		s.Map.Width, s.Map.Height, dur(s.StatusAgeMs), reporting)
	if censusless > 0 {
		fmt.Fprintf(w, ", %d reporting none", censusless)
	}
	if truncated > 0 {
		fmt.Fprintf(w, ", %d capped at 32 (the rest UNREPORTED)", truncated)
	}
	fmt.Fprintf(w, "\n%d species alive right now", len(rows))
	if idx != nil {
		fmt.Fprintf(w, "; the crossing record holds %d that have ever travelled", idx.LedgerSpecies)
	}
	fmt.Fprintf(w, "\n\n")
	if len(rows) == 0 {
		fmt.Fprintln(w, "no world is reporting a species right now.")
		return
	}
	// The ledger half, by key, when a running archive supplied one.
	ledger := map[string]SpeciesRow{}
	if idx != nil {
		for _, r := range idx.Species {
			ledger[r.Key] = r
		}
	}
	fmt.Fprintf(w, "%-34s %6s %6s %-26s %9s %10s %s\n",
		"species (as its world spells it)", "alive", "eggs", "worlds", "crossings",
		"last", "badges")
	fmt.Fprintln(w, strings.Repeat("-", 124))
	for _, r := range rows {
		where := make([]string, 0, len(r.Worlds))
		for _, x := range r.Worlds {
			part := fmt.Sprintf("S%d:%d", x.Slot, x.Bibites)
			if x.Eggs > 0 {
				part += fmt.Sprintf("+%de", x.Eggs)
			}
			where = append(where, part)
		}
		crossings, last := "n/a", "n/a"
		badges := []string{}
		if lr, ok := ledger[r.Key]; ok {
			crossings = strconv.Itoa(lr.Crossings)
			last = "never"
			if lr.LastAgeMs != nil {
				last = dur(*lr.LastAgeMs) + " ago"
			}
			if lr.Excluded {
				badges = append(badges, "never-exported")
			}
			if lr.Everywhere {
				badges = append(badges, "everywhere")
			}
			if lr.Endemic {
				badges = append(badges, "endemic")
			}
		} else if idx != nil {
			crossings, last = "0", "never"
			if r.Excluded {
				badges = append(badges, "never-exported")
			}
			if r.Everywhere {
				badges = append(badges, "everywhere")
			}
			if r.Endemic {
				badges = append(badges, "endemic")
			}
		} else {
			if r.Excluded {
				badges = append(badges, "never-exported")
			}
			if r.Everywhere {
				badges = append(badges, "everywhere")
			}
			if r.Endemic {
				badges = append(badges, "endemic")
			}
		}
		fmt.Fprintf(w, "%-34s %6d %6d %-26s %9s %10s %s\n",
			trunc(r.Name, 34), r.Population, r.Eggs, trunc(strings.Join(where, " "), 26),
			crossings, last, strings.Join(badges, " "))
	}
	if idx == nil {
		fmt.Fprintf(w, "\ncrossings read 'n/a' because this came from a metrics file. That half of the\n"+
			"answer is derived from the migration ledger, which only a running archive holds.\n")
	}
	fmt.Fprintf(w, "\nthis is who is ALIVE, from each world's own census. it is not who has crossed:\n"+
		"a species that died out is not here however far it travelled. names are printed\n"+
		"exactly as the owning world spells them (contract-a.md §17, A36).\n")
}

// aliveSpecies is the alive union computed from a Status alone, for the case
// ringstat has no running archive to ask. IT IS THE SAME COMPUTATION
// SpeciesIndexView performs, minus the ledger join, and it is deliberately
// written against the same key helper so a terminal and a browser cannot
// disagree about which species are endemic.
func aliveSpecies(s Status) (rows []SpeciesRow, reporting, censusless, truncated int) {
	byKey := map[string]*SpeciesRow{}
	order := []string{}
	excluded := map[string][]int{}
	for _, sv := range s.Slots {
		if sv.MigrationExcludeKnown {
			for _, n := range sv.MigrationExclude {
				excluded[n] = append(excluded[n], sv.Slot)
			}
		}
		if !sv.StatsKnown {
			continue
		}
		if !sv.SpeciesKnown {
			censusless++
			continue
		}
		reporting++
		if sv.SpeciesTruncated {
			truncated++
		}
		for _, e := range sv.Species {
			key := censusEntryKey(e)
			if key == "" {
				continue
			}
			r := byKey[key]
			if r == nil {
				r = &SpeciesRow{Key: key, Name: e.GenericName + " " + e.SpecificName}
				byKey[key] = r
				order = append(order, key)
			}
			r.Population += e.Bibites
			r.Eggs += e.Eggs
			r.Worlds = append(r.Worlds, SpeciesWorld{Slot: sv.Slot, Bibites: e.Bibites, Eggs: e.Eggs})
		}
	}
	for _, key := range order {
		r := byKey[key]
		r.Endemic = len(r.Worlds) == 1
		r.Everywhere = reporting >= 2 && len(r.Worlds) == reporting
		if slots, ok := excluded[key]; ok {
			r.Excluded = true
			r.ExcludedBy = slots
		}
		rows = append(rows, *r)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Population != rows[j].Population {
			return rows[i].Population > rows[j].Population
		}
		return rows[i].Key < rows[j].Key
	})
	return rows, reporting, censusless, truncated
}

// str prints an optional string, sanitized: modVersion, contractAVersion and
// gameVersion are all attacker-chosen free text on §10.1's inventory, and a
// version string is exactly the field nobody thinks to escape.
func str(v string) string {
	if v == "" {
		return "?"
	}
	return safeTerm(v)
}

func optShortInt(v *int) string {
	if v == nil {
		return "?"
	}
	return strconv.Itoa(*v)
}

func optBool(v *bool) string {
	if v == nil {
		return "?"
	}
	if *v {
		return "yes"
	}
	return "no"
}

// saveEvery prints the save interval. 0 IS A READING AND IT IS PRINTED AS ONE:
// "OFF" is that world's save timer switched off, which is a different fact from
// the "?" of a world that has not told us — and the two have opposite
// consequences for a hard stop.
func saveEvery(v *float64) string {
	if v == nil {
		return "?"
	}
	if *v == 0 {
		return "OFF"
	}
	return scale(v) + "m"
}

func opt(v *int) string {
	if v == nil {
		return "unknown"
	}
	return fmt.Sprintf("%d", *v)
}

// optShort is opt for a column that pairs two numbers, where "unknown" would be
// wider than the column and less readable than the gap it marks.
func optShort(v *int) string {
	if v == nil {
		return "?"
	}
	return fmt.Sprintf("%d", *v)
}

// scale prints a setting the way the map draws it: a whole number without a
// decimal point, one place otherwise, and an UNPUBLISHED one as "?". Never the
// shipped default — inboundRatePerSimMinute has been changed three times, so a
// number filled in here would be a confident wrong answer about the one peer
// whose build is too old to say.
func scale(v *float64) string {
	if v == nil {
		return "?"
	}
	if *v == math.Trunc(*v) {
		return strconv.FormatFloat(*v, 'f', 0, 64)
	}
	return strconv.FormatFloat(*v, 'f', 1, 64)
}

func speed(v *float64) string { return "×" + scale(v) }

// speedPair is the map's speed cell as one string: the scale the game reports
// APPLYING, and after the arrow the rate the archive MEASURED it delivering.
// The measured half is omitted rather than shown as "?" — the archive not
// having watched long enough yet is a different thing from a peer refusing to
// say, and only the second deserves the warning glyph.
func speedPair(v SlotView) string {
	s := speed(v.TimeScale)
	if v.AchievedTimeScale != nil {
		s += "→~" + speed(v.AchievedTimeScale)
	}
	return s
}

// padCell pads to n DISPLAY columns rather than n bytes, which is what fmt's
// width counts. "×" and "→" are two and three bytes, so the speed column — the
// only one carrying either — cannot be aligned with a plain %-Ns.
func padCell(s string, n int) string {
	if w := len([]rune(s)); w < n {
		return s + strings.Repeat(" ", n-w)
	}
	return s
}

func dur(ms int64) string {
	d := time.Duration(ms) * time.Millisecond
	switch {
	case ms <= 0:
		return "0s"
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	default:
		return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	}
}

// trunc fits a string to a column, AFTER sanitizing it. Every caller of trunc
// is printing somebody else's chosen text, so the sanitizer belongs here rather
// than at each call site: a new column added next year gets it for free, and
// forgetting it is not one of the available mistakes.
func trunc(s string, n int) string { return termsafe.Clip(s, n) }

// safeTerm is B30's escaping obligation for THE TERMINAL, which §22 binds
// identically to the page: "escape for the surface rendered into — HTML, an
// HTML attribute, a URL, JSON in a script, TERMINAL ESCAPE SEQUENCES — and
// never render one as markup. ringstat's terminal is a rendering surface with
// its own injection story."
//
// The rule itself now lives in internal/termsafe, because WP7 added a SECOND
// terminal surface printing text this project did not author — the sidecar's
// own-slot view and `--diagnose`, which render peer ids, game versions, close
// reasons and `lastRefusal` on the participant's own machine. A rule
// re-implemented per surface is a rule eventually implemented once too few.
func safeTerm(s string) string { return termsafe.Text(s) }
