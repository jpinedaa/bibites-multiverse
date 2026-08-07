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
	fmt.Fprintf(w, "archive %s (%s)   state %s old   %d ledger record(s), %d genome gap(s)\n\n",
		s.ArchivePeerID, link, dur(s.StatusAgeMs), s.Records, s.Gaps)

	// speed is the world's own time scale and pace is queued/cap per SIMULATED
	// minute of that world — the same two settings the map draws in every cell,
	// so the two operator surfaces cannot disagree about them either.
	fmt.Fprintf(w, "%-5s %-7s %-22s %-9s %6s %10s %8s %10s %6s %8s %s\n",
		"slot", "pos", "peer", "state", "speed", "population", "custody", "pace",
		"held", "bounced", "last save")
	fmt.Fprintln(w, strings.Repeat("-", 120))
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
		fmt.Fprintf(w, "%-5d %-7s %-22s %-9s %6s %10s %8s %10s %6s %8s %s\n",
			v.Slot, fmt.Sprintf("(%d,%d)", v.Position.Col, v.Position.Row),
			trunc(v.PeerID, 22), state, speed(v.TimeScale),
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
			part := fmt.Sprintf("%s ×%d", e.GenericName+" "+e.SpecificName, e.Bibites)
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
			fmt.Fprintf(w, "%-5d %s\n", v.Slot, strings.Join(v.MigrationExclude, ", "))
		}
	}
	fmt.Fprintf(w, "\na '?' is a world that has not published that setting — an older mod, or an\n"+
		"older sidecar. It is NEVER the value the game ships with. 'save OFF' is a\n"+
		"reading: that world's save timer is off, which is why it may never report a\n"+
		"last save. these settings are read-only everywhere in this system.\n")
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

func str(v string) string {
	if v == "" {
		return "?"
	}
	return v
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

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
