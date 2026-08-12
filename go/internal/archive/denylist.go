package archive

// DQ7's operator-side render deny list (contract-b-m4.md §22, B30; §10.1).
//
// WHAT IT IS FOR. A species name is a string a player typed or the game
// generated, published to every subscriber and rendered on a page anyone with
// the URL can read. Until M5 there was no operator action between IGNORE IT and
// EVICT THE PEER. Suppression at render costs the suppressed world nothing,
// needs no peer's cooperation, adds no wire field and requires no contract
// change — which is the only combination that does not contradict something
// this system is built on. B28's eviction stays available for a peer that will
// not stop, and it is deliberately the stronger and worse tool.
//
// THE VIEW, NOT THE RECORD, AND THE DIFFERENCE IS PROMISED IN BOTH DIRECTIONS.
// D11 and §10 make the ledger a thing nothing ever evicts from. This file
// suppresses strings on the way OUT of the archive's HTTP surface and on the
// way into a terminal; it never touches migrations.jsonl, the genome store or
// metrics.jsonl, and the archive goes on recording verbatim. M5 PROMISES
// REMOVAL FROM THE VIEW AND EXPLICITLY DOES NOT PROMISE REMOVAL FROM THE
// RECORD. Anything stronger is a change to D11's never-evict rule and belongs
// in a decision row, not in a support reply.
//
// WHY IT IS APPLIED AT THE ARCHIVE'S HTTP BOUNDARY. The page and ringstat read
// the SAME JSON (§10.1), and the whole reason they do is that one join in one
// place is what stops the terminal tool and the page disagreeing. A deny list
// applied twice would be two lists to keep in step. It is applied AFTER
// StatusView builds the sample, so the durable metrics file keeps what the
// archive actually saw — the record is not where suppression happens.

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"multiverse/internal/contractb"
	"multiverse/internal/wire"
)

// Suppressed is what a denied string renders as. It is a MARKER AND NOT AN
// ERASURE: a page that silently dropped the row would be lying about how many
// species a world holds, and an operator looking at their own deny list has to
// be able to see it working.
const Suppressed = "[suppressed]"

// PeerPrefix marks a deny entry that names a PEER rather than a species. A peer
// entry suppresses every attacker-chosen string that peer publishes — its own
// id, its census names, its exclusion list, its version strings and its
// refusal text — because a peer that has to be suppressed is not one whose
// other free text can be trusted.
const PeerPrefix = "peer:"

// SpeciesPrefix is optional and exists only so an operator can be explicit. A
// bare line is a species name, which is the case DQ7 was written for.
const SpeciesPrefix = "species:"

// denyReloadInterval bounds how often the file is re-stat'ed. The deny list is
// a MODERATION control and an archive restart costs minutes of dark status page
// and a permanent hole in the ledger's record of what crossed while it replayed
// — so "edit the file" has to be the whole procedure, and this is what makes it
// one.
const denyReloadInterval = 2 * time.Second

// DenyList is the operator's list, reloaded from its file in place.
//
// The zero value is a working empty list that denies nothing, which is what
// every archive that never configured one uses.
type DenyList struct {
	mu       sync.RWMutex
	path     string
	species  map[string]bool
	peers    map[string]bool
	loadedAt time.Time
	modTime  time.Time
	size     int64
}

// NewDenyList reads path, or returns an empty list when path is "".
func NewDenyList(path string) (*DenyList, error) {
	d := &DenyList{path: path, species: map[string]bool{}, peers: map[string]bool{}}
	if path == "" {
		return d, nil
	}
	if err := d.reload(); err != nil {
		return nil, err
	}
	return d, nil
}

// Path is the file behind the list, or "".
func (d *DenyList) Path() string {
	if d == nil {
		return ""
	}
	return d.path
}

// Len is how many entries are loaded, for a startup log line.
func (d *DenyList) Len() int {
	if d == nil {
		return 0
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.species) + len(d.peers)
}

func (d *DenyList) reload() error {
	f, err := os.Open(d.path)
	if err != nil {
		if os.IsNotExist(err) {
			// A named file that does not exist yet is an EMPTY LIST, not a
			// failure: an operator who will need one tomorrow should be able to
			// name it today, and an archive that refused to start over a missing
			// moderation file would be the worst possible trade.
			d.mu.Lock()
			d.species, d.peers = map[string]bool{}, map[string]bool{}
			d.loadedAt = time.Now()
			d.modTime, d.size = time.Time{}, 0
			d.mu.Unlock()
			return nil
		}
		return err
	}
	defer f.Close()
	info, statErr := f.Stat()
	species := map[string]bool{}
	peers := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		switch {
		case strings.HasPrefix(line, PeerPrefix):
			if id := strings.TrimSpace(strings.TrimPrefix(line, PeerPrefix)); id != "" {
				peers[id] = true
			}
		case strings.HasPrefix(line, SpeciesPrefix):
			if name := wire.NormalizeSpeciesName(strings.TrimPrefix(line, SpeciesPrefix)); name != "" {
				species[name] = true
			}
		default:
			if name := wire.NormalizeSpeciesName(line); name != "" {
				species[name] = true
			}
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	d.mu.Lock()
	d.species, d.peers = species, peers
	d.loadedAt = time.Now()
	if statErr == nil && info != nil {
		d.modTime, d.size = info.ModTime(), info.Size()
	}
	d.mu.Unlock()
	return nil
}

// refresh re-reads the file when it has changed, at most once every
// denyReloadInterval.
func (d *DenyList) refresh() {
	if d == nil || d.path == "" {
		return
	}
	d.mu.RLock()
	fresh := time.Since(d.loadedAt) < denyReloadInterval
	modTime, size := d.modTime, d.size
	d.mu.RUnlock()
	if fresh {
		return
	}
	info, err := os.Stat(d.path)
	if err == nil && info.ModTime().Equal(modTime) && info.Size() == size {
		d.mu.Lock()
		d.loadedAt = time.Now()
		d.mu.Unlock()
		return
	}
	_ = d.reload()
}

// Empty reports whether the list denies nothing, so a caller can skip a whole
// copy on the ordinary path.
func (d *DenyList) Empty() bool {
	if d == nil {
		return true
	}
	d.refresh()
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.species) == 0 && len(d.peers) == 0
}

// DeniesPeer reports whether every string this peer publishes is suppressed.
func (d *DenyList) DeniesPeer(peerID string) bool {
	if d == nil || peerID == "" {
		return false
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.peers[peerID]
}

// DeniesSpecies matches on the NORMALIZED name — the same normalization the
// species index keys on, so a deny entry an operator typed matches the entry
// the page shows even when the world spells it with two spaces.
//
// A one-word entry also matches the GENERIC HALF alone, which is how an
// operator suppresses a whole genus without listing every specific name a
// mutating population will invent.
func (d *DenyList) DeniesSpecies(generic, specific string) bool {
	if d == nil {
		return false
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if len(d.species) == 0 {
		return false
	}
	full := wire.NormalizeSpeciesName(generic + " " + specific)
	if d.species[full] {
		return true
	}
	return d.species[wire.NormalizeSpeciesName(generic)]
}

// ---------------------------------------------------------------- application

// ApplyStatus returns s with every denied string replaced by Suppressed. The
// input is not modified: StatusView's own value goes on to metrics.jsonl
// verbatim, and only the served copy is suppressed.
func (d *DenyList) ApplyStatus(s Status) Status {
	if d.Empty() {
		return s
	}
	slots := make([]SlotView, len(s.Slots))
	copy(slots, s.Slots)
	for i := range slots {
		v := &slots[i]
		peerDenied := d.DeniesPeer(v.PeerID)
		if peerDenied {
			// Everything this peer AUTHORED, not everything about it. Its slot,
			// its position, its liveness, its depths and its populations are the
			// relay's and the archive's own numbers, and suppressing those would
			// hide a dark world rather than a name (Risk 5).
			v.PeerID = Suppressed
			v.ModVersion = suppressIfSet(v.ModVersion)
			v.ContractAVersion = suppressIfSet(v.ContractAVersion)
			v.GameVersion = suppressIfSet(v.GameVersion)
			v.LastRefusal = suppressIfSet(v.LastRefusal)
			if v.LastSave != nil {
				save := *v.LastSave
				save.Name = suppressIfSet(save.Name)
				v.LastSave = &save
			}
		}
		if len(v.Species) > 0 {
			census := make([]contractb.CensusEntry, len(v.Species))
			copy(census, v.Species)
			for j := range census {
				if peerDenied || d.DeniesSpecies(census[j].GenericName, census[j].SpecificName) {
					census[j].GenericName = Suppressed
					census[j].SpecificName = ""
				}
			}
			v.Species = census
		}
		if len(v.MigrationExclude) > 0 {
			excl := make([]string, len(v.MigrationExclude))
			copy(excl, v.MigrationExclude)
			for j := range excl {
				if peerDenied || d.DeniesSpecies(excl[j], "") {
					excl[j] = Suppressed
				}
			}
			v.MigrationExclude = excl
		}
	}
	s.Slots = slots
	return s
}

// ApplySpeciesIndex suppresses the species tab's own rows. A denied species
// keeps its counts and loses its name: the map still says how many organisms
// are alive, which is the fact an operator is running the archive for.
func (d *DenyList) ApplySpeciesIndex(idx SpeciesIndex) SpeciesIndex {
	if d.Empty() {
		return idx
	}
	rows := make([]SpeciesRow, len(idx.Species))
	copy(rows, idx.Species)
	for i := range rows {
		r := &rows[i]
		generic, specific, _ := strings.Cut(r.Key, " ")
		if !d.DeniesSpecies(generic, specific) {
			// A row a denied PEER holds is still that species' row: the same
			// species is alive in other worlds, and suppressing it because one
			// holder is denied would suppress a name nobody asked about.
			continue
		}
		// THE KEY GOES TOO, and it is replaced rather than blanked. It is the
		// page's row id, so two suppressed species must not collapse into one
		// row — but it is also the name, spelled normalized, and a suppression
		// that left the name in a field anyone with the URL can read would be a
		// suppression in name only. The derived id keeps the rows apart and
		// carries nothing back.
		r.Key = Suppressed + "#" + strconv.FormatUint(fingerprint(r.Key), 16)
		r.Name = Suppressed
		r.Spellings = nil
		r.Parent = ""
	}
	idx.Species = rows
	return idx
}

// ApplySpeciesTree suppresses the genealogy's node labels.
//
// THE TREE'S SHAPE SURVIVES AND ONLY THE NAMES GO, which is the same trade every
// other surface here makes: a suppressed species still has the ancestry it has,
// and deleting the node would silently reparent its children under a grandparent
// they never descended from — a lie about the record, told to hide a name.
//
// A KEY IS A NAME HERE TOO, spelled normalized, and it is ALSO AN EDGE: Parent
// and Roots reference it. So a suppressed key is REWRITTEN, exactly as
// ApplySpeciesIndex rewrites it, and the rewrite is then applied to every
// reference in one pass — otherwise a page would resolve a parent link back to
// the plaintext key the suppression was meant to remove.
func (d *DenyList) ApplySpeciesTree(t SpeciesTree) SpeciesTree {
	if d.Empty() {
		return t
	}
	// One pass to decide the renames, a second to apply them everywhere. Doing
	// it in one pass would leave a node's own Parent pointing at a key that had
	// not been renamed yet.
	rename := map[string]string{}
	for i := range t.Nodes {
		generic, specific, _ := strings.Cut(t.Nodes[i].Key, " ")
		if d.DeniesSpecies(generic, specific) {
			rename[t.Nodes[i].Key] = Suppressed + "#" +
				strconv.FormatUint(fingerprint(t.Nodes[i].Key), 16)
		}
	}
	if len(rename) == 0 {
		return t
	}
	nodes := make([]TreeNode, len(t.Nodes))
	copy(nodes, t.Nodes)
	for i := range nodes {
		n := &nodes[i]
		if to, ok := rename[n.Key]; ok {
			n.Key = to
			n.Name = Suppressed
			// The provenance goes with the name. Saying a suppressed label came
			// from a census would be a statement about a name nobody can read.
			n.NameFrom = ""
		}
		if to, ok := rename[n.Parent]; ok {
			n.Parent = to
		}
	}
	roots := make([]string, len(t.Roots))
	copy(roots, t.Roots)
	for i := range roots {
		if to, ok := rename[roots[i]]; ok {
			roots[i] = to
		}
	}
	t.Nodes, t.Roots = nodes, roots
	return t
}

// ApplySpeciesHistory suppresses the name a history answer echoes back.
func (d *DenyList) ApplySpeciesHistory(h SpeciesHistory) SpeciesHistory {
	if d.Empty() {
		return h
	}
	generic, specific, _ := strings.Cut(h.Key, " ")
	if d.DeniesSpecies(generic, specific) {
		h.Key = Suppressed
	}
	return h
}

// ApplyHopFeed suppresses the species name on a hop. The hop itself stays: a
// crossing happened, and the map's whole job is to show that it did.
func (d *DenyList) ApplyHopFeed(f HopFeed) HopFeed {
	if d.Empty() {
		return f
	}
	hops := make([]Hop, len(f.Hops))
	copy(hops, f.Hops)
	for i := range hops {
		sp := hops[i].Species
		if sp == nil {
			continue
		}
		if !d.DeniesSpecies(sp.GenericName, sp.SpecificName) &&
			!d.DeniesSpecies(sp.ParentGenericName, sp.ParentSpecificName) {
			continue
		}
		suppressed := *sp
		suppressed.GenericName = Suppressed
		suppressed.SpecificName = ""
		suppressed.ParentGenericName = ""
		suppressed.ParentSpecificName = ""
		hops[i].Species = &suppressed
	}
	f.Hops = hops
	return f
}

func suppressIfSet(v string) string {
	if v == "" {
		return ""
	}
	return Suppressed
}
