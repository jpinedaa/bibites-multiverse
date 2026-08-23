package archive

// The lineage-instance fold separates a portable species NAME from one
// occurrence of that name in the recorded family. The migration protocol does
// not carry a global species id. It does carry enough ordered evidence to build
// one: the species name, its parent name, the source world, the destination
// world, and the crossing time.
//
// A world binding is the newest lineage instance that a recorded crossing put
// under one name in that world. A new parent behind the same name creates a new
// immutable instance. This is the distinction the name-only aggregate cannot
// make, and it prevents a reused name from overwriting an older edge.

import (
	"crypto/sha256"
	"encoding/hex"

	"multiverse/internal/wire"
)

type speciesLineageLedger struct {
	byID     map[string]*speciesLineageInstance
	byName   map[string]map[string]*speciesLineageInstance
	byWorld  map[int]map[string]string
	max      int
	overflow int
	edges    int
}

// speciesLineageMax bounds immutable name-and-parent-path instances. The
// normalized-name aggregate admits at most 65,536 names. Two instances per
// name covers ordinary name reuse with the same memory headroom while keeping
// an adversarial sequence of new paths from making retained state unbounded.
const speciesLineageMax = speciesAggMax * 2

type speciesLineageInstance struct {
	id      string
	nameKey string
	name    string

	// parentKnown distinguishes a recorded root from a parent placeholder. A
	// placeholder is promoted when that parent's own crossing reaches the fold.
	parentKnown bool
	parentID    string
	parentKey   string
	parent      string
	placeholder bool
	conflict    bool

	crossings int
	firstMs   int64
	lastMs    int64
	recent    []SpeciesCrossing

	genomeHash string
	genomeAtMs int64

	// seenAt is the newest crossing that placed this instance under its name in
	// each world. On load, the greatest time reconstructs byWorld.
	seenAt map[int]int64
}

func newSpeciesLineageLedger() *speciesLineageLedger {
	return &speciesLineageLedger{
		byID:    map[string]*speciesLineageInstance{},
		byName:  map[string]map[string]*speciesLineageInstance{},
		byWorld: map[int]map[string]string{},
		max:     speciesLineageMax,
	}
}

func lineageDigest(kind, nameKey, parentID string) string {
	h := sha256.New()
	_, _ = h.Write([]byte(kind))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(nameKey))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(parentID))
	sum := h.Sum(nil)
	return "lineage:" + hex.EncodeToString(sum[:16])
}

func lineageInstanceID(nameKey, parentID string) string {
	kind := "child"
	if parentID == "" {
		kind = "root"
	}
	return lineageDigest(kind, nameKey, parentID)
}

func lineagePlaceholderID(nameKey string) string {
	return lineageDigest("unknown", nameKey, "")
}

func (l *speciesLineageLedger) add(inst *speciesLineageInstance) bool {
	if inst == nil || inst.id == "" || inst.nameKey == "" {
		return false
	}
	if l.byID[inst.id] == nil && len(l.byID) >= l.max {
		l.overflow++
		return false
	}
	if inst.seenAt == nil {
		inst.seenAt = map[int]int64{}
	}
	l.byID[inst.id] = inst
	if l.byName[inst.nameKey] == nil {
		l.byName[inst.nameKey] = map[string]*speciesLineageInstance{}
	}
	l.byName[inst.nameKey][inst.id] = inst
	return true
}

func (l *speciesLineageLedger) unique(nameKey string) *speciesLineageInstance {
	set := l.byName[nameKey]
	if len(set) != 1 {
		return nil
	}
	for _, inst := range set {
		return inst
	}
	return nil
}

func (l *speciesLineageLedger) bound(slot int, nameKey string) *speciesLineageInstance {
	if slot <= 0 || l.byWorld[slot] == nil {
		return nil
	}
	return l.byID[l.byWorld[slot][nameKey]]
}

func (l *speciesLineageLedger) bind(slot int, nameKey string, inst *speciesLineageInstance, at int64) {
	if slot <= 0 || nameKey == "" || inst == nil {
		return
	}
	if l.byWorld[slot] == nil {
		l.byWorld[slot] = map[string]string{}
	}
	cur := l.bound(slot, nameKey)
	if cur != nil {
		curAt := cur.seenAt[slot]
		// RecordedAt is a millisecond clock, so separate crossings can tie.
		// Use the immutable instance id as the second ordering key. This makes
		// a roll-up rebuild choose the same binding as the live fold even though
		// the persisted instances are loaded from a map.
		if curAt > at || (curAt == at && cur.id >= inst.id) {
			return
		}
	}
	l.byWorld[slot][nameKey] = inst.id
	if at >= inst.seenAt[slot] {
		inst.seenAt[slot] = at
	}
}

func (l *speciesLineageLedger) ensurePlaceholder(nameKey, raw string) *speciesLineageInstance {
	id := nameKey
	if len(l.byName[nameKey]) > 0 {
		id = lineagePlaceholderID(nameKey)
	}
	if inst := l.byID[id]; inst != nil {
		if inst.name == "" {
			inst.name = raw
		}
		return inst
	}
	inst := &speciesLineageInstance{
		id: id, nameKey: nameKey, name: raw, placeholder: true,
		seenAt: map[int]int64{},
	}
	if !l.add(inst) {
		return nil
	}
	return inst
}

func (l *speciesLineageLedger) wouldCycle(childID, parentID string) bool {
	seen := map[string]bool{childID: true}
	for cur := parentID; cur != ""; {
		if seen[cur] {
			return true
		}
		seen[cur] = true
		inst := l.byID[cur]
		if inst == nil || !inst.parentKnown {
			return false
		}
		cur = inst.parentID
	}
	return false
}

func (l *speciesLineageLedger) matching(nameKey, parentID string, parentKnown bool,
	parentKey string) *speciesLineageInstance {
	var match *speciesLineageInstance
	for _, inst := range l.byName[nameKey] {
		compatible := inst.parentKnown == parentKnown
		if parentKnown {
			compatible = compatible && inst.parentID == parentID
		} else {
			// Raw spelling is display data, not identity. A34 collapses whitespace
			// for comparison, so two unresolved claims with the same parentKey are
			// the same claim even when their source spellings differ.
			compatible = compatible && inst.parentKey == parentKey
		}
		if compatible && (match == nil || inst.id < match.id) {
			match = inst
		}
	}
	return match
}

// observe folds one migration into the lineage-instance graph. The caller
// holds Archive.mu. It returns every instance whose persisted scalar changed.
func (l *speciesLineageLedger) observe(rec Record, nameKey string) []*speciesLineageInstance {
	if rec.Species == nil || nameKey == "" {
		return nil
	}
	name := rec.Species.GenericName + " " + rec.Species.SpecificName
	changed := []*speciesLineageInstance{}
	parentKey := ""
	parent := ""
	parentKnown := true
	parentID := ""
	conflict := false
	if rec.Species.ParentGenericName != "" {
		parent = rec.Species.ParentGenericName + " " + rec.Species.ParentSpecificName
		parentKey = speciesNameKey(rec.Species.ParentGenericName, rec.Species.ParentSpecificName)
		if parentKey == "" || parentKey == nameKey {
			parentKnown = false
			conflict = true
		} else if p := l.bound(rec.SourceSlot, parentKey); p != nil {
			parentID = p.id
		} else if p := l.unique(parentKey); p != nil {
			parentID = p.id
		} else if len(l.byName[parentKey]) > 1 {
			// This world supplies no binding and the record contains more than
			// one occurrence of the parent name. Selecting any one would invent
			// an edge, so keep this child's identity unresolved.
			parentKnown = false
			conflict = true
		} else {
			p := l.ensurePlaceholder(parentKey, parent)
			if p == nil {
				parentKnown = false
				conflict = true
			} else {
				parentID = p.id
			}
		}
		if p := l.byID[parentID]; p != nil {
			// The child record proves that this parent instance exists in the
			// source world, even when the parent has never crossed itself.
			before := p.seenAt[rec.SourceSlot]
			l.bind(rec.SourceSlot, parentKey, p, rec.RecordedAt)
			if p.seenAt[rec.SourceSlot] != before {
				changed = append(changed, p)
			}
		}
	}

	current := l.bound(rec.SourceSlot, nameKey)
	inst := current
	if inst != nil && !inst.parentKnown && inst.placeholder && parentKnown {
		// Another world can establish this path before the placeholder's own
		// crossing arrives here. Reuse that instance instead of promoting the
		// placeholder into a duplicate of the same immutable path.
		if match := l.matching(nameKey, parentID, true, parentKey); match != nil && match.id != inst.id {
			inst = match
		} else if l.wouldCycle(inst.id, parentID) {
			inst = nil
		} else {
			inst.parentKnown = true
			inst.parentID = parentID
			inst.parentKey = parentKey
			inst.parent = parent
			inst.placeholder = false
			if parentID != "" {
				l.edges++
			}
			changed = append(changed, inst)
		}
	} else if inst != nil {
		if inst.parentKnown != parentKnown || inst.parentID != parentID ||
			inst.parentKey != parentKey {
			inst = nil
		}
	}

	if inst == nil {
		inst = l.matching(nameKey, parentID, parentKnown, parentKey)
	}
	if inst == nil {
		id := nameKey
		if len(l.byName[nameKey]) > 0 {
			id = lineageInstanceID(nameKey, parentID)
		}
		if !parentKnown && len(l.byName[nameKey]) > 0 {
			id = lineageDigest("conflict", nameKey, parentKey)
		}
		if existing := l.byID[id]; existing != nil {
			inst = existing
		} else {
			inst = &speciesLineageInstance{
				id: id, nameKey: nameKey, name: name,
				parentKnown: parentKnown, parentID: parentID,
				parentKey: parentKey, parent: parent, conflict: conflict,
				seenAt: map[int]int64{},
			}
			if !l.add(inst) {
				return changed
			}
			if parentKnown && parentID != "" {
				l.edges++
			}
			changed = append(changed, inst)
		}
	}
	if conflict {
		inst.conflict = true
	}
	if inst.name == "" {
		inst.name = name
	}
	inst.crossings++
	if inst.firstMs == 0 || rec.RecordedAt < inst.firstMs {
		inst.firstMs = rec.RecordedAt
	}
	if rec.RecordedAt > inst.lastMs {
		inst.lastMs = rec.RecordedAt
	}
	if rec.Lineage != nil && rec.Lineage.GenomeHash != "" && rec.RecordedAt >= inst.genomeAtMs {
		inst.genomeHash = rec.Lineage.GenomeHash
		inst.genomeAtMs = rec.RecordedAt
	}
	inst.recent = append(inst.recent, SpeciesCrossing{
		AtMs: rec.RecordedAt, FromSlot: rec.SourceSlot, ToSlot: rec.DestSlot,
		ExitEdge: rec.ExitEdge,
	})
	if n := len(inst.recent); n > speciesRecentMax {
		inst.recent = append(inst.recent[:0], inst.recent[n-speciesRecentMax:]...)
	}
	l.bind(rec.SourceSlot, nameKey, inst, rec.RecordedAt)
	l.bind(rec.DestSlot, nameKey, inst, rec.RecordedAt)
	changed = append(changed, inst)
	return changed
}

func (l *speciesLineageLedger) rebuildIndexes() {
	l.byName = map[string]map[string]*speciesLineageInstance{}
	l.byWorld = map[int]map[string]string{}
	l.edges = 0
	for _, inst := range l.byID {
		l.add(inst)
		if inst.parentKnown && inst.parentID != "" {
			l.edges++
		}
		for slot, at := range inst.seenAt {
			l.bind(slot, inst.nameKey, inst, at)
		}
	}
}

func (l *speciesLineageLedger) splitNames() int {
	n := 0
	for _, set := range l.byName {
		if len(set) > 1 {
			n++
		}
	}
	return n
}

func speciesNameKey(generic, specific string) string {
	return wire.SpeciesKey(generic, specific)
}
