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
	"net/http"
	"sort"
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

	fmt.Fprintf(w, "%-5s %-7s %-22s %-9s %10s %8s %6s %6s %8s %s\n",
		"slot", "pos", "peer", "state", "population", "custody", "paced", "held", "bounced", "last save")
	fmt.Fprintln(w, strings.Repeat("-", 108))
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
		fmt.Fprintf(w, "%-5d %-7s %-22s %-9s %10s %8s %6s %6s %8s %s\n",
			v.Slot, fmt.Sprintf("(%d,%d)", v.Position.Col, v.Position.Row),
			trunc(v.PeerID, 22), state,
			opt(v.Population), opt(v.CustodyDepth), opt(v.PacedDepth), opt(v.HeldDepth),
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

func opt(v *int) string {
	if v == nil {
		return "unknown"
	}
	return fmt.Sprintf("%d", *v)
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

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
