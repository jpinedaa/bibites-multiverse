package archive

import (
	"compress/gzip"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"multiverse/internal/contractb"
	"multiverse/internal/wire"
)

// ---------------------------------------------------------------- the fixture

// rollupSpeciesPool is a small genealogy: eight species, six of which name a
// parent, so the fixture exercises the edge count, the ancestry floor and the
// latest-writer rule rather than only the crossing counter.
var rollupSpeciesPool = []struct{ generic, specific, pg, ps string }{
	{"Izus", "copedylanus", "", ""},
	{"Izus", "velox", "Izus", "copedylanus"},
	{"Izus", "tardus", "Izus", "copedylanus"},
	{"Cyanea", "prima", "", ""},
	{"Cyanea", "secunda", "Cyanea", "prima"},
	{"Cyanea", "tertia", "Cyanea", "secunda"},
	{"Hellbeardus", "emilyae", "Cyanea", "prima"},
	{"Hellbeardus", "minor", "Hellbeardus", "emilyae"},
}

var rollupPeerPool = []string{"peer-a", "peer-b", "peer-c", "peer-d", "peer-e", "peer-f"}

var rollupEdgePool = []string{"E", "N", "W", "S", ""}

// syntheticRecords builds a ledger's worth of records that touches every fold
// this sidecar persists: four record types, several peers, several lanes,
// several species with parents, a genome-hash pool small enough that the same
// hash crosses many times inside one brain bucket (which is what makes the
// coverage denominator's dedup window matter), records with no species block at
// all, and records with parent hashes in the lineage annex.
//
// It is DETERMINISTIC for a seed and the caller shares one slice between the
// fixtures it compares, so the two folds see byte-identical input.
func syntheticRecords(seed int64, n int, baseMs, stepMs int64) []Record {
	rnd := rand.New(rand.NewSource(seed))
	hashes := make([]string, 24)
	for i := range hashes {
		hashes[i] = fmt.Sprintf("bb8-%02d-%s", i, strings.Repeat("f", 8))
	}
	out := make([]Record, 0, n)
	for i := 0; i < n; i++ {
		at := baseMs + int64(i)*stepMs
		id := fmt.Sprintf("mig-%06d", i)
		peer := rollupPeerPool[rnd.Intn(len(rollupPeerPool))]
		switch r := rnd.Intn(100); {
		case r < 62:
			sp := rollupSpeciesPool[rnd.Intn(len(rollupSpeciesPool))]
			rec := Record{
				Type: RecordMigration, RecordedAt: at, MigrationID: id,
				SourcePeer: peer, SourceSlot: 1 + rnd.Intn(6), DestSlot: 1 + rnd.Intn(6),
				ExitEdge: rollupEdgePool[rnd.Intn(len(rollupEdgePool))],
				EntityID: int32(i),
			}
			if rnd.Intn(10) > 0 {
				// One crossing in ten carries no species block at all: ABSENT IS
				// ABSENT, and the aggregate must not invent a key for it.
				rec.Species = &wire.Species{
					GenericName: sp.generic, SpecificName: sp.specific,
					ParentGenericName: sp.pg, ParentSpecificName: sp.ps,
				}
			}
			if rnd.Intn(12) > 0 {
				l := &contractb.Lineage{GenomeHash: hashes[rnd.Intn(len(hashes))]}
				if rnd.Intn(3) == 0 {
					l.Parents = []contractb.Parent{
						{GenomeHash: hashes[rnd.Intn(len(hashes))]},
					}
				}
				rec.Lineage = l
			}
			out = append(out, rec)
		case r < 78:
			out = append(out, Record{Type: RecordAck, RecordedAt: at, MigrationID: id,
				SourcePeer: peer, DestPeer: "peer-z", EntityID: int32(i)})
		case r < 92:
			out = append(out, Record{Type: RecordNack, RecordedAt: at, MigrationID: id,
				SourcePeer: peer, DestPeer: "peer-z", Code: "dark_destination"})
		default:
			out = append(out, Record{Type: RecordGenome, RecordedAt: at,
				GenomeHash: hashes[rnd.Intn(len(hashes))], ServedBy: peer})
		}
	}
	return out
}

func appendRecords(t *testing.T, dir string, recs []Record) {
	t.Helper()
	l, err := OpenLedger(dir)
	if err != nil {
		t.Fatalf("OpenLedger: %v", err)
	}
	for _, rec := range recs {
		if err := l.Append(rec); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	if err := l.Close(); err != nil {
		t.Fatalf("ledger close: %v", err)
	}
}

func openRollupArchive(t *testing.T, dir string) *Archive {
	t.Helper()
	a, err := New(Config{DataDir: dir, PeerID: "archive-test", RelayURL: "ws://test"})
	if err != nil {
		t.Fatalf("New(%s): %v", dir, err)
	}
	return a
}

// ---------------------------------------------------------------- the dump

// rollupDump renders EVERY aggregate the sidecar persists, deterministically, so
// two folds can be compared field for field and a difference names itself.
//
// It is the test's definition of "the state", and it is deliberately wider than
// any one endpoint: a roll-up that served /api/status correctly and lost a
// species' parent edge would pass a narrower comparison and be wrong.
func rollupDump(a *Archive) string {
	var b strings.Builder
	a.mu.Lock()
	fmt.Fprintf(&b, "records=%d skipped=%d gapsExpired=%d dupRefused=%d\n",
		a.recordCount, a.ledgerSkipped, a.evict.gapsExpired, a.duplicatesRefused)
	// THE GENOME-GAP QUEUE is a persisted aggregate since phase 3, so it belongs
	// in the definition of "the state": a tail replay that rebuilt a different
	// queue from the same records would pass every other line of this dump.
	gaps := make([]string, 0, len(a.pending))
	for hash := range a.pending {
		gaps = append(gaps, hash)
	}
	sort.Strings(gaps)
	for _, hash := range gaps {
		f := a.pending[hash]
		fmt.Fprintf(&b, "gap %s crossedAt=%d peer=%s mig=%s entity=%d migrant=%v species=%q\n",
			hash, f.crossedAt.UnixMilli(), f.sourcePeer, f.migrationID, f.entityID,
			f.migrant, f.speciesKey)
	}
	fmt.Fprintf(&b, "first=%d last=%d typeOverflow=%d peerOverflow=%d\n",
		a.tally.firstMs, a.tally.lastMs, a.tally.typeOverflow, a.tally.peerOverflow)
	for _, k := range sortedKeys(a.tally.byType) {
		fmt.Fprintf(&b, "type %s=%d\n", k, a.tally.byType[k])
	}
	for _, k := range sortedKeys(a.tally.byPeer) {
		fmt.Fprintf(&b, "peer %s=%d\n", k, a.tally.byPeer[k])
	}
	fmt.Fprintf(&b, "speciesLedger overflow=%d edges=%d edgeFirst=%d\n",
		a.species.overflow, a.species.edges, a.species.edgeFirstMs)
	keys := make([]string, 0, len(a.species.byKey))
	for k := range a.species.byKey {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		e := a.species.byKey[k]
		fps := make([]uint64, 0, len(e.genomes))
		for fp := range e.genomes {
			fps = append(fps, fp)
		}
		sort.Slice(fps, func(i, j int) bool { return fps[i] < fps[j] })
		fmt.Fprintf(&b, "species %q c=%d first=%d last=%d trunc=%v parent=%q pkey=%q pat=%d "+
			"hash=%q hat=%d genomes=%v recent=%v\n",
			k, e.crossings, e.firstMs, e.lastMs, e.genomesTruncated,
			e.parent, e.parentKey, e.parentAtMs, e.genomeHash, e.genomeAtMs, fps, e.recent)
	}
	lanes := make([]lanePair, 0, len(a.lanes))
	for k := range a.lanes {
		lanes = append(lanes, k)
	}
	sort.Slice(lanes, func(i, j int) bool {
		if lanes[i].from != lanes[j].from {
			return lanes[i].from < lanes[j].from
		}
		if lanes[i].to != lanes[j].to {
			return lanes[i].to < lanes[j].to
		}
		return lanes[i].edge < lanes[j].edge
	})
	for _, k := range lanes {
		l := a.lanes[k]
		fmt.Fprintf(&b, "lane %d->%d/%q total=%d first=%d last=%d recent=%v\n",
			k.from, k.to, k.edge, l.total, l.firstAt, l.lastAt, l.recent)
	}
	a.mu.Unlock()

	g := a.brainAgg
	g.mu.Lock()
	fmt.Fprintf(&b, "brain frontier=%d dropped=%d\n", g.frontier, g.bucketsDropped)
	bkeys := make([]int64, 0, len(g.buckets))
	for k := range g.buckets {
		bkeys = append(bkeys, k)
	}
	sort.Slice(bkeys, func(i, j int) bool { return bkeys[i] < bkeys[j] })
	for _, k := range bkeys {
		if g.buckets[k].seen > 0 {
			fmt.Fprintf(&b, "seen %d=%d\n", k, g.buckets[k].seen)
		}
	}
	dkeys := make([]int64, 0, len(g.dedup))
	for k := range g.dedup {
		dkeys = append(dkeys, k)
	}
	sort.Slice(dkeys, func(i, j int) bool { return dkeys[i] < dkeys[j] })
	for _, k := range dkeys {
		fps := make([]uint64, 0, len(g.dedup[k]))
		for fp, bits := range g.dedup[k] {
			if bits&1 != 0 {
				fps = append(fps, fp)
			}
		}
		sort.Slice(fps, func(i, j int) bool { return fps[i] < fps[j] })
		if len(fps) > 0 {
			fmt.Fprintf(&b, "dedup %d=%v\n", k, fps)
		}
	}
	g.mu.Unlock()
	return b.String()
}

func sortedKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// firstDiff names the first line two dumps disagree on, so a failure reads as a
// fact rather than as two pages of text.
func firstDiff(want, got string) string {
	w, g := strings.Split(want, "\n"), strings.Split(got, "\n")
	for i := 0; i < len(w) || i < len(g); i++ {
		lw, lg := "", ""
		if i < len(w) {
			lw = w[i]
		}
		if i < len(g) {
			lg = g[i]
		}
		if lw != lg {
			return fmt.Sprintf("line %d:\n  full replay: %s\n  sidecar+tail: %s", i+1, lw, lg)
		}
	}
	return ""
}

// ---------------------------------------------------------------- the invariant

// TestTheSidecarPlusATailEqualsAFullReplay is THE correctness invariant of the
// whole roll-up, and everything else in this file is a special case of it: the
// aggregate state after (load the sidecar + replay what is behind it) must EQUAL
// the state after a straight fold of the same records, field for field.
//
// The fixture saves at RANDOM points, several times over, so a save lands in the
// middle of a species' history, in the middle of a brain bucket and in the
// middle of a lane's flow window rather than only on tidy boundaries. Each round
// is a real restart: the archive is closed (which saves), more records are
// appended, and a new archive is opened over the same directory.
func TestTheSidecarPlusATailEqualsAFullReplay(t *testing.T) {
	base := time.Now().Add(-40 * time.Minute).UnixMilli()
	for _, seed := range []int64{1, 7, 31, 1009} {
		seed := seed
		t.Run(fmt.Sprintf("seed%d", seed), func(t *testing.T) {
			recs := syntheticRecords(seed, 900, base, 2_000)

			// The reference: one archive, one full replay, no sidecar.
			whole := t.TempDir()
			appendRecords(t, whole, recs)
			ref := openRollupArchive(t, whole)
			want := rollupDump(ref)
			if err := ref.Close(); err != nil {
				t.Fatalf("close: %v", err)
			}

			// The subject: the same records, folded across several restarts, with
			// the LAST chunk left behind the sidecar so the final archive is
			// genuinely a sidecar plus a tail.
			rnd := rand.New(rand.NewSource(seed))
			cuts := []int{}
			for prev, i := 0, 0; i < 4; i++ {
				next := prev + 60 + rnd.Intn(200)
				if next >= len(recs)-40 {
					break
				}
				cuts = append(cuts, next)
				prev = next
			}
			if len(cuts) == 0 {
				t.Fatal("the fixture produced no save points")
			}

			dir := t.TempDir()
			prev := 0
			for _, cut := range cuts {
				appendRecords(t, dir, recs[prev:cut])
				prev = cut
				a := openRollupArchive(t, dir)
				if err := a.Close(); err != nil {
					t.Fatalf("close: %v", err)
				}
			}
			appendRecords(t, dir, recs[prev:])

			a := openRollupArchive(t, dir)
			defer a.Close()
			a.mu.Lock()
			covered := a.rollupCovered
			a.mu.Unlock()
			if !a.rollupLoaded {
				t.Fatal("the last archive did not load a sidecar at all")
			}
			if covered.Record == 0 || covered.Record >= int64(len(recs)) {
				t.Fatalf("the sidecar covered %d of %d records: this is not a tail replay",
					covered.Record, len(recs))
			}
			if got := rollupDump(a); got != want {
				t.Fatalf("the sidecar plus its tail is not the full replay\n%s",
					firstDiff(want, got))
			}
		})
	}
}

// TestTheSidecarAloneReproducesTheWholeFold is the DIRTY-TRACKING AUDIT, and it
// is mechanical rather than a list of assertions somebody has to remember to
// extend.
//
// An incrementally persisted fold has one failure mode and it is silent: a write
// site that moves a field and forgets to mark its key dirty leaves a number in
// the file that is STALE rather than absent, and no page says so. This test
// makes the file the only source: the sidecar is copied into a directory with NO
// LEDGER AT ALL, so every value the aggregates hold came out of it. If any write
// site in the DELTA rounds below failed to mark its key, the value it moved is
// missing here and the dump differs.
//
// The first save after a full replay writes the whole state, so it would mask
// exactly the bug this looks for; the fixture therefore folds several rounds of
// new records through restarts, and every round after the first is a delta.
func TestTheSidecarAloneReproducesTheWholeFold(t *testing.T) {
	base := time.Now().Add(-40 * time.Minute).UnixMilli()
	recs := syntheticRecords(4242, 800, base, 2_500)

	// The reference is a STRAIGHT FOLD in a directory of its own. Taking it from
	// the incremental directory would read the sidecar to build it, so a stale
	// value would appear on both sides of the comparison and the test would prove
	// nothing — which is exactly what it did before a deliberately removed dirty
	// mark failed to fail it.
	whole := t.TempDir()
	appendRecords(t, whole, recs)
	ref := openRollupArchive(t, whole)
	want := rollupDump(ref)
	if err := ref.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	dir := t.TempDir()
	prev := 0
	for _, cut := range []int{120, 300, 480, 640, len(recs)} {
		appendRecords(t, dir, recs[prev:cut])
		prev = cut
		a := openRollupArchive(t, dir)
		if err := a.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
	}

	// The sidecar, alone, in a directory with no record behind it.
	alone := t.TempDir()
	copyFile(t, filepath.Join(dir, rollupSidecarName), filepath.Join(alone, rollupSidecarName))
	a := openRollupArchive(t, alone)
	defer a.Close()
	if got := rollupDump(a); got != want {
		t.Fatalf("a write site moved a value and did not mark its key dirty\n%s",
			firstDiff(want, got))
	}
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	b, err := os.ReadFile(from)
	if err != nil {
		t.Fatalf("read %s: %v", from, err)
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(to, b, 0o644); err != nil {
		t.Fatalf("write %s: %v", to, err)
	}
}

// TestEveryPersistedWriteSiteMarksItsKeyDirty names the sites one at a time.
//
// The audit above proves the SET of marks is complete for the records it folds;
// this proves each site individually, so a failure says which one. The list is
// the same list rollupDirty's comment carries, and a new persisted aggregate
// belongs in three places: the line shape, that comment, and here.
func TestEveryPersistedWriteSiteMarksItsKeyDirty(t *testing.T) {
	a := openRollupArchive(t, t.TempDir())
	defer a.Close()
	at := time.Now().Add(-10 * time.Minute).UnixMilli()

	clear := func() {
		a.mu.Lock()
		a.rollupDirty.clear()
		a.mu.Unlock()
		g := a.brainAgg
		g.mu.Lock()
		g.dirtySeen = map[int64]bool{}
		g.newDedup = map[int64][]uint64{}
		g.mu.Unlock()
	}

	// 1. observeSpeciesLocked: the species key, its new genome fingerprint, and
	//    the ledger globals when an edge or the ancestry floor moves.
	clear()
	rec := migration(at, 1, 2, "E", "Izus", "copedylanus", "hash-one")
	rec.Species.ParentGenericName, rec.Species.ParentSpecificName = "Cyanea", "prima"
	a.mu.Lock()
	a.observeSpeciesLocked(rec)
	sp := a.rollupDirty.species["Izus copedylanus"]
	gen := len(a.rollupDirty.genomes["Izus copedylanus"])
	led := a.rollupDirty.ledger
	a.mu.Unlock()
	if !sp {
		t.Error("observeSpeciesLocked did not mark the species key dirty")
	}
	if gen != 1 {
		t.Errorf("observeSpeciesLocked recorded %d new genome fingerprints, want 1", gen)
	}
	if !led {
		t.Error("observeSpeciesLocked did not mark the species ledger globals dirty")
	}

	// 1b. A genome fingerprint already in the set is NOT a new one: the additive
	//     line must carry what changed and nothing else.
	clear()
	a.mu.Lock()
	a.observeSpeciesLocked(rec)
	again := len(a.rollupDirty.genomes["Izus copedylanus"])
	a.mu.Unlock()
	if again != 0 {
		t.Errorf("a repeat genome hash produced %d fingerprint appends, want 0", again)
	}

	// 1c. THE ANCESTRY FLOOR ON ITS OWN. It is a minimum over every record that
	//     ever named a parent, so it is the one field a LOSING latest-writer test
	//     still moves — and the only field whose mark the whole-fold audit cannot
	//     reach, because a ledger's recordedAt is the archive's own clock and
	//     therefore monotonic. Driven here directly.
	clear()
	older := rec
	older.RecordedAt = at - 60_000
	a.mu.Lock()
	before := a.species.edgeFirstMs
	a.observeSpeciesLocked(older)
	floorMoved := a.species.edgeFirstMs < before
	floorDirty := a.rollupDirty.ledger
	a.mu.Unlock()
	if !floorMoved {
		t.Fatal("the fixture did not lower the ancestry floor")
	}
	if !floorDirty {
		t.Error("the ancestry floor moved without marking the species ledger globals dirty")
	}

	// 2. observeLaneLocked.
	clear()
	a.mu.Lock()
	a.observeLaneLocked(3, 4, "N", at)
	lane := a.rollupDirty.lanes[lanePair{from: 3, to: 4, edge: "N"}]
	a.mu.Unlock()
	if !lane {
		t.Error("observeLaneLocked did not mark the lane dirty")
	}

	// 3. countRecordLocked: the counters, the per-peer tally and the index.
	clear()
	a.mu.Lock()
	a.countRecordLocked(Record{Type: RecordAck, RecordedAt: at, SourcePeer: "peer-a"},
		a.ledgerPos)
	counts, peers, index := a.rollupDirty.counts, a.rollupDirty.peers, len(a.rollupDirty.index)
	a.mu.Unlock()
	if !counts {
		t.Error("countRecordLocked did not mark the record counters dirty")
	}
	if !peers {
		t.Error("countRecordLocked did not mark the per-peer counters dirty")
	}
	if index == 0 {
		t.Error("countRecordLocked did not mark a new index hour dirty")
	}

	// 4. observeBrainSeen: the coverage denominator and its dedup window.
	clear()
	a.observeBrainSeen(at, "hash-one")
	g := a.brainAgg
	g.mu.Lock()
	seen := g.dirtySeen[brainBucketStart(at)]
	ded := len(g.newDedup[brainBucketStart(at)])
	g.mu.Unlock()
	if !seen {
		t.Error("observeBrainSeen did not mark the coverage bucket dirty")
	}
	if ded != 1 {
		t.Errorf("observeBrainSeen recorded %d new dedup fingerprints, want 1", ded)
	}

	// 5. The species overflow counter, which is a published number and therefore
	//    a persisted one. It is driven through the same single writer.
	clear()
	a.mu.Lock()
	for i := 0; i < speciesAggMax+2 && len(a.species.byKey) < speciesAggMax; i++ {
		a.species.byKey[fmt.Sprintf("filler %d", i)] = &speciesAgg{
			genomes: map[uint64]struct{}{}}
	}
	a.rollupDirty.ledger = false
	a.observeSpeciesLocked(migration(at, 1, 2, "E", "Overflow", "species", ""))
	over, ledger := a.species.overflow, a.rollupDirty.ledger
	a.mu.Unlock()
	if over == 0 {
		t.Fatal("the fixture did not overflow the species aggregate")
	}
	if !ledger {
		t.Error("the species overflow counter moved without marking the ledger globals dirty")
	}
}

// ---------------------------------------------------------------- the loss rules

// TestATornRollUpSaveIsDroppedWhole covers the one rule this file adds to
// brainsave.go's replay: a save appends the keys that moved and THEN the floor
// line that says how much of the record they account for, so a save interrupted
// between the two must be dropped whole. Applying half of it would leave the
// aggregates ahead of the cursor, and the tail replay would fold the same
// records a second time — which is a DOUBLED counter, the worst failure this
// design can have, because it looks like data.
func TestATornRollUpSaveIsDroppedWhole(t *testing.T) {
	base := time.Now().Add(-30 * time.Minute).UnixMilli()
	recs := syntheticRecords(99, 400, base, 3_000)

	whole := t.TempDir()
	appendRecords(t, whole, recs)
	ref := openRollupArchive(t, whole)
	want := rollupDump(ref)
	if err := ref.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	dir := t.TempDir()
	appendRecords(t, dir, recs[:150])
	a := openRollupArchive(t, dir)
	if err := a.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	appendRecords(t, dir, recs[150:300])
	b := openRollupArchive(t, dir)
	if err := b.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Tear the file: drop the last save's floor line and half of the batch in
	// front of it, exactly as a kill mid-write would.
	path := filepath.Join(dir, rollupSidecarName)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	floors := 0
	for _, l := range lines {
		if strings.HasPrefix(l, `{"r":"f"`) {
			floors++
		}
	}
	if floors < 2 {
		t.Fatalf("the fixture wrote %d floor lines; the tear needs at least 2", floors)
	}
	// Keep everything up to and including the LAST line, then chop back past the
	// final floor line and a couple of the records it committed.
	cut := len(lines)
	for cut > 0 && !strings.HasPrefix(lines[cut-1], `{"r":"f"`) {
		cut--
	}
	cut-- // drop the floor line itself
	if cut > 2 {
		cut -= 2 // and two of the aggregate lines it was going to commit
	}
	torn := strings.Join(lines[:cut], "\n") + "\n"
	if err := os.WriteFile(path, []byte(torn), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	appendRecords(t, dir, recs[300:])
	c := openRollupArchive(t, dir)
	defer c.Close()
	if !c.rollupLoaded {
		t.Fatal("the torn sidecar was refused whole; the earlier saves were good")
	}
	if got := rollupDump(c); got != want {
		t.Fatalf("a torn save was not dropped whole\n%s", firstDiff(want, got))
	}
}

// TestAnUnreadableRollUpFileIsKeptBesideAFreshOne is the loss rule the whole
// package shares: a file this build cannot use is REPORTED AND LEFT ALONE, the
// state is rebuilt by a full replay of the raw record, and the archive runs.
func TestAnUnreadableRollUpFileIsKeptBesideAFreshOne(t *testing.T) {
	base := time.Now().Add(-20 * time.Minute).UnixMilli()
	recs := syntheticRecords(5, 200, base, 3_000)

	whole := t.TempDir()
	appendRecords(t, whole, recs)
	ref := openRollupArchive(t, whole)
	want := rollupDump(ref)
	_ = ref.Close()

	dir := t.TempDir()
	appendRecords(t, dir, recs)
	path := filepath.Join(dir, rollupSidecarName)
	if err := os.WriteFile(path, []byte("this is not a roll-up state\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	a := openRollupArchive(t, dir)
	defer a.Close()
	if !a.rollupLost {
		t.Error("an unreadable sidecar was not reported as a loss")
	}
	if _, err := os.Stat(path + ".unreadable"); err != nil {
		t.Errorf("the unreadable bytes were not kept: %v", err)
	}
	if got := rollupDump(a); got != want {
		t.Fatalf("the full replay after an unreadable sidecar is not the full replay\n%s",
			firstDiff(want, got))
	}
	// And a fresh file is being written from now.
	if err := a.SaveRollup(); err != nil {
		t.Fatalf("save: %v", err)
	}
	if info, err := os.Stat(path); err != nil || info.Size() == 0 {
		t.Fatalf("no fresh sidecar was written: %v", err)
	}
}

// TestAVersionItDoesNotKnowIsRefusedWhole: a header naming another version is
// not half-read, exactly as brains.jsonl refuses one.
func TestAVersionItDoesNotKnowIsRefusedWhole(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, rollupSidecarName)
	head := fmt.Sprintf(`{"r":"h","v":%d,"bucketMs":%d}`+"\n"+
		`{"r":"rc","records":9999}`+"\n"+
		`{"r":"f","covLines":9999,"covRecords":9999}`+"\n", rollupVersion+1, BrainBucketMs)
	if err := os.WriteFile(path, []byte(head), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	st, usable, err := loadRollupState(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if usable || st != nil {
		t.Fatal("a sidecar from another format version was accepted")
	}
}

// TestADifferentBrainBucketWidthIsADifferentFile: the coverage denominator is
// per bucket, so a file written at another resolution cannot be merged with
// these without inventing a within-bucket distribution.
func TestADifferentBrainBucketWidthIsADifferentFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, rollupSidecarName)
	head := fmt.Sprintf(`{"r":"h","v":%d,"bucketMs":%d}`+"\n", rollupVersion, BrainBucketMs*2)
	if err := os.WriteFile(path, []byte(head), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, usable, err := loadRollupState(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if usable {
		t.Fatal("a sidecar written at another bucket width was accepted")
	}
}

// ---------------------------------------------------------------- compaction

// TestTheSidecarCompactsAndStaysCorrect: the file is rewritten whole when it
// outgrows its live content, and the rewrite is the SAME state — a compaction
// that lost a key would be indistinguishable from a missing dirty mark on the
// next restart.
func TestTheSidecarCompactsAndStaysCorrect(t *testing.T) {
	base := time.Now().Add(-30 * time.Minute).UnixMilli()
	recs := syntheticRecords(17, 600, base, 2_500)

	whole := t.TempDir()
	appendRecords(t, whole, recs)
	ref := openRollupArchive(t, whole)
	want := rollupDump(ref)
	_ = ref.Close()

	dir := t.TempDir()
	prev, compactions := 0, 0
	for _, cut := range []int{100, 220, 350, 470} {
		appendRecords(t, dir, recs[prev:cut])
		prev = cut
		a := openRollupArchive(t, dir)
		// Force a rewrite on every save: the constants are per instance so a test
		// does not have to write a megabyte to reach them.
		a.rollup.mu.Lock()
		a.rollup.compactFloor, a.rollup.compactRatio = 0, 0
		a.rollup.mu.Unlock()
		before := fileSize(t, filepath.Join(dir, rollupSidecarName))
		if err := a.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
		if fileSize(t, filepath.Join(dir, rollupSidecarName)) <= before {
			compactions++
		}
	}
	if compactions == 0 {
		t.Log("note: no save shrank the file; the fixture may be too small to prove it")
	}
	appendRecords(t, dir, recs[prev:])
	a := openRollupArchive(t, dir)
	defer a.Close()
	if got := rollupDump(a); got != want {
		t.Fatalf("a compacted sidecar plus its tail is not the full replay\n%s",
			firstDiff(want, got))
	}
	// A compacted file holds ONE floor line, because a compaction is one batch.
	raw, err := os.ReadFile(filepath.Join(dir, rollupSidecarName))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if n := strings.Count(string(raw), `{"r":"f"`); n < 1 {
		t.Fatalf("the compacted file holds %d floor lines, want at least 1", n)
	}
}

func fileSize(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

// ---------------------------------------------------------------- the damage

// TestALoadedSidecarKeepsTheLedgerDamageCount. The ledger is append-only, so a
// line that does not parse is PERMANENT damage and every restart has to say so
// (store.go, ScanLedger). A tail replay does not re-read the region the sidecar
// covers and therefore cannot re-discover the damage in it, so the count is
// persisted — and an archive whose skipped-line count fell to zero because it
// stopped looking would be worse than one with no count at all.
func TestALoadedSidecarKeepsTheLedgerDamageCount(t *testing.T) {
	base := time.Now().Add(-20 * time.Minute).UnixMilli()
	recs := syntheticRecords(23, 200, base, 3_000)

	dir := t.TempDir()
	appendRecords(t, dir, recs[:80])
	// One line of damage, in the region the first save will cover.
	f, err := os.OpenFile(filepath.Join(dir, ledgerName), os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	if _, err := f.WriteString("{this is not json}\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	f.Close()

	a := openRollupArchive(t, dir)
	if a.ledgerSkipped != 1 {
		t.Fatalf("the full replay counted %d damaged lines, want 1", a.ledgerSkipped)
	}
	records := a.recordCount
	if err := a.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	appendRecords(t, dir, recs[80:])
	b := openRollupArchive(t, dir)
	defer b.Close()
	if b.ledgerSkipped != 1 {
		t.Errorf("the tail replay reports %d damaged lines, want the persisted 1",
			b.ledgerSkipped)
	}
	if b.recordCount != records+len(recs)-80 {
		t.Errorf("records=%d, want %d", b.recordCount, records+len(recs)-80)
	}
	// The cursor ends at the end of the raw record, because that is what the next
	// tail replay resumes from: get it wrong and the tail starts mid-record.
	if want := rawEndOffset(t, dir); b.ledgerPos.Offset != want {
		t.Errorf("the cursor ends at offset %d of %q, want the end of the raw record %d",
			b.ledgerPos.Offset, b.ledgerPos.Segment, want)
	}
	if b.ledgerPos.Record != int64(b.recordCount) {
		t.Errorf("the cursor covers %d records against a fold of %d",
			b.ledgerPos.Record, b.recordCount)
	}
}

// rawEndOffset is the length of the LAST source in the replay's ordered run,
// which is where a cursor that has read everything must sit.
func rawEndOffset(t *testing.T, dir string) int64 {
	t.Helper()
	srcs, err := ledgerSources(dir)
	if err != nil {
		t.Fatalf("ledgerSources: %v", err)
	}
	if len(srcs) == 0 {
		return 0
	}
	last := srcs[len(srcs)-1]
	info, err := os.Stat(last.Path)
	if err != nil {
		return 0
	}
	return info.Size()
}

// ---------------------------------------------------------------- the plan

// TestTheReplayPlanScansExactlyTheDuplicateWindow is the phase-3 claim, asserted
// on the plan itself: the ONE thing that cannot be persisted is the duplicate
// guard (its fingerprint seeds are per process by design), so the raw scan
// reaches back exactly cfg.DedupWindow behind the sidecar's cursor and no
// further. The genome horizon does not enter into it any more — the gap queue is
// persisted — and that is what turns a 720 h raw scan into a 48 h one, and a 48 h
// one into an hour when the window comes down.
func TestTheReplayPlanScansExactlyTheDuplicateWindow(t *testing.T) {
	now := time.Now()
	st := newRollupState()
	st.cursor = LedgerPos{Segment: "2026-08-17-0000", Offset: 500_000, Record: 5000}
	for h := 0; h < 72; h++ {
		hour := now.Add(-time.Duration(h) * time.Hour).UnixMilli()
		hour -= hour % rollupIndexStrideMs
		st.index[hour] = LedgerPos{Segment: "2026-08-17-0000",
			Offset: int64(500_000 - h*1000), Record: int64(5000 - h*10)}
	}

	// A 48 h window with a 72 h index: the scan starts inside the index and
	// behind the cursor, and the span it names is the window.
	a := &Archive{cfg: Config{DedupWindow: 48 * time.Hour, GenomeHorizon: 720 * time.Hour}}
	plan := a.planReplay(st, now)
	if !plan.FromSidecar {
		t.Fatal("a plan with a cursor is not FromSidecar")
	}
	if plan.RawSpan != 48*time.Hour {
		t.Errorf("RawSpan=%s, want the duplicate window 48h whatever the 720h horizon says",
			plan.RawSpan)
	}
	if plan.Floor.Record != 5000 || plan.Floor.Offset != 500_000 {
		t.Errorf("Floor=%+v, want the sidecar's cursor at record 5000, offset 500000",
			plan.Floor)
	}
	if plan.From.Record >= plan.Floor.Record || plan.From.Record == 0 {
		t.Errorf("the scan starts at record %d against a floor of %d: it should start "+
			"inside the window and behind the sidecar",
			plan.From.Record, plan.Floor.Record)
	}
	// One hour is one hour of index entries closer to the cursor. This is the
	// measurement the deployment will make when the fleet has crossed.
	short := &Archive{cfg: Config{DedupWindow: time.Hour, GenomeHorizon: 720 * time.Hour}}
	shortPlan := short.planReplay(st, now)
	if shortPlan.From.Record <= plan.From.Record {
		t.Errorf("a 1 h window starts at record %d and a 48 h one at %d: the shorter "+
			"window must start LATER", shortPlan.From.Record, plan.From.Record)
	}
	if shortPlan.RawSpan != time.Hour {
		t.Errorf("RawSpan=%s, want 1h", shortPlan.RawSpan)
	}

	// A window wider than the index can answer for scans from the start of what
	// is on the host. An index that cannot say where the window begins is not a
	// licence to guess.
	wide := &Archive{cfg: Config{DedupWindow: 400 * time.Hour}}
	if got := wide.planReplay(st, now); !got.From.IsStart() {
		t.Errorf("with a window older than the index the scan starts at %+v, want the "+
			"beginning of the raw window", got.From)
	}

	// No sidecar at all is the replay this archive has always had.
	if got := a.planReplay(nil, now); !got.From.IsStart() || !got.Floor.IsStart() ||
		got.FromSidecar {
		t.Errorf("with no sidecar the plan is %+v, want a full replay", got)
	}
}

// TestARestartReadsOnlyTheDuplicateWindowOfRaw is the same claim measured
// end to end rather than on the plan: two archives over the same 900-record
// ledger, one with a duplicate window that covers the whole of it and one with a
// window that covers a sliver, and the second parses a fraction of the raw the
// first does — with IDENTICAL aggregates.
func TestARestartReadsOnlyTheDuplicateWindowOfRaw(t *testing.T) {
	// The records span 900 * 30 s = 7.5 h, ending now.
	step := int64(30_000)
	n := 900
	base := time.Now().UnixMilli() - int64(n)*step
	recs := syntheticRecords(4242, n, base, step)

	run := func(window time.Duration) (*Archive, string) {
		dir := t.TempDir()
		appendRecords(t, dir, recs)
		cfg := Config{DataDir: dir, PeerID: "archive-test", RelayURL: "ws://test",
			DedupWindow: window}
		first, err := New(cfg)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		want := rollupDump(first)
		if err := first.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
		second, err := New(cfg)
		if err != nil {
			t.Fatalf("New restart: %v", err)
		}
		if got := rollupDump(second); got != want {
			t.Fatalf("a %s-window restart is not the full replay\n%s",
				window, firstDiff(want, got))
		}
		return second, dir
	}

	wide, _ := run(24 * time.Hour)
	defer wide.Close()
	narrow, _ := run(30 * time.Minute)
	defer narrow.Close()

	if wide.replayRawRecords != n {
		t.Errorf("a 24 h window over a 7.5 h ledger parsed %d records, want all %d",
			wide.replayRawRecords, n)
	}
	if narrow.replayRawRecords >= wide.replayRawRecords {
		t.Errorf("a 30 min window parsed %d raw records and a 24 h one parsed %d: the "+
			"short window is the whole saving", narrow.replayRawRecords, wide.replayRawRecords)
	}
	if narrow.replayRawRecords > n/2 {
		t.Errorf("a 30 min window over a 7.5 h ledger parsed %d of %d records; it should "+
			"be a small fraction", narrow.replayRawRecords, n)
	}
}

// TestATailReplayWithAHorizonStillEqualsAFullReplay: the two cut points are
// independent, so a plan that starts the RAW scan inside the window must still
// produce the same AGGREGATES as a full fold.
func TestATailReplayWithAHorizonStillEqualsAFullReplay(t *testing.T) {
	base := time.Now().Add(-30 * time.Minute).UnixMilli()
	recs := syntheticRecords(77, 500, base, 3_000)
	cfg := func(dir string) Config {
		return Config{DataDir: dir, PeerID: "archive-test", RelayURL: "ws://test",
			GenomeHorizon: 2 * time.Hour, DedupWindow: time.Hour}
	}

	whole := t.TempDir()
	appendRecords(t, whole, recs)
	ref, err := New(cfg(whole))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	want := rollupDump(ref)
	_ = ref.Close()

	dir := t.TempDir()
	appendRecords(t, dir, recs[:300])
	a, err := New(cfg(dir))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := a.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	appendRecords(t, dir, recs[300:])
	b, err := New(cfg(dir))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer b.Close()
	if got := rollupDump(b); got != want {
		t.Fatalf("a horizon-bounded tail replay is not the full replay\n%s",
			firstDiff(want, got))
	}
}

// ---------------------------------------------------------------- the surface

// TestTheHonestyFieldsSayWhatIsAggregateAndWhatIsRaw. The names are published
// and stable: a page, `monitor.sh` and the M5 evidence record read them.
func TestTheHonestyFieldsSayWhatIsAggregateAndWhatIsRaw(t *testing.T) {
	base := time.Now().Add(-20 * time.Minute).UnixMilli()
	recs := syntheticRecords(5150, 300, base, 3_000)

	dir := t.TempDir()
	appendRecords(t, dir, recs)
	a := openRollupArchive(t, dir)

	view := a.StatusView()
	if view.RollupCoveredRecords != len(recs) {
		t.Errorf("rollupCoveredRecords=%d after the first save, want %d",
			view.RollupCoveredRecords, len(recs))
	}
	if view.RollupSavedAtMs == 0 {
		t.Error("rollupSavedAtMs is 0 after a save")
	}
	if view.Records != len(recs) {
		t.Errorf("ledgerRecords=%d, want %d", view.Records, len(recs))
	}
	// WHAT THE START COST, measured. A first start reads every record there is,
	// so the two say so; a restart from the sidecar says something much smaller,
	// which is the whole point and is asserted in the restart tests.
	if view.ReplayRawRecords != len(recs) {
		t.Errorf("replayRawRecords=%d after a full replay, want %d",
			view.ReplayRawRecords, len(recs))
	}
	if view.ReplayRawSeconds < 0 {
		t.Errorf("replayRawSeconds=%v", view.ReplayRawSeconds)
	}
	if view.ReplayFromRetired {
		t.Error("replayFromRetired is set on an archive whose raw record is all here")
	}
	if view.DuplicatesRefused != 0 {
		t.Errorf("duplicatesRefused=%d on an archive nothing has been offered to twice",
			view.DuplicatesRefused)
	}
	_ = a.Close()

	// A brand-new archive has written nothing and says so rather than claiming
	// coverage it does not have.
	fresh := openRollupArchive(t, t.TempDir())
	defer fresh.Close()
	if v := fresh.StatusView(); v.RollupCoveredRecords != 0 || v.ReplayRawRecords != 0 {
		t.Errorf("a fresh archive publishes covered=%d replayRawRecords=%d, want 0 and 0",
			v.RollupCoveredRecords, v.ReplayRawRecords)
	}
}

// TestTheRecordCountersAreFoldedOnceByOneWriter. recordCount drifted BELOW
// `wc -l` for a whole deployment because one of its four live call sites did not
// count a GENOME line (archive.go). The per-type and per-peer counters have five
// call sites and the same hazard, so they have one writer and this test asserts
// the sum over the types is the total.
func TestTheRecordCountersAreFoldedOnceByOneWriter(t *testing.T) {
	base := time.Now().Add(-20 * time.Minute).UnixMilli()
	recs := syntheticRecords(808, 400, base, 3_000)
	dir := t.TempDir()
	appendRecords(t, dir, recs)
	a := openRollupArchive(t, dir)
	defer a.Close()

	a.mu.Lock()
	total, byType, byPeer := a.recordCount, a.tally.byType, a.tally.byPeer
	sum := 0
	for _, v := range byType {
		sum += v
	}
	peerSum := 0
	for _, v := range byPeer {
		peerSum += v
	}
	a.mu.Unlock()

	if total != len(recs) {
		t.Fatalf("recordCount=%d, want %d", total, len(recs))
	}
	if sum != total {
		t.Errorf("the per-type counters sum to %d against %d records", sum, total)
	}
	if peerSum != total {
		t.Errorf("the per-peer counters sum to %d against %d records: every synthetic "+
			"record names a peer", peerSum, total)
	}
	for _, typ := range []string{RecordMigration, RecordAck, RecordNack, RecordGenome} {
		if byType[typ] == 0 {
			t.Errorf("no %s records were counted; the fixture should hold some", typ)
		}
	}
}

// TestTheCursorNeverGoesBackwards. A raw stream shorter than the aggregate's own
// coverage is what phase 2 produces on purpose — a retired segment is records
// that were folded and are no longer on the host. Re-folding them would double
// every counter, so the cursor holds.
func TestTheCursorNeverGoesBackwards(t *testing.T) {
	base := time.Now().Add(-20 * time.Minute).UnixMilli()
	recs := syntheticRecords(31337, 200, base, 3_000)

	dir := t.TempDir()
	appendRecords(t, dir, recs)
	a := openRollupArchive(t, dir)
	want := rollupDump(a)
	_ = a.Close()

	// The sidecar alone, with the raw record removed under it.
	shorter := t.TempDir()
	copyFile(t, filepath.Join(dir, rollupSidecarName),
		filepath.Join(shorter, rollupSidecarName))
	b := openRollupArchive(t, shorter)
	defer b.Close()
	if got := rollupDump(b); got != want {
		t.Fatalf("the aggregates changed when the raw record went away\n%s",
			firstDiff(want, got))
	}
}

// ---------------------------------------------------------------- phase 3

// TestDuplicatesRefusedIsCountedAndPersisted. A refused duplicate is a real
// event that leaves NO OTHER TRACE anywhere: nothing is appended, nothing is
// logged per occurrence, and the ledger is by construction the ledger without
// it. §25's B38 and decision 0006 both rest on the counter — during the
// transition a non-zero value names a peer still running a build that
// re-forwards, and it is the evidence that says whether the 48 h duplicate
// window may come down to an hour — so it is all-time and it survives a restart.
func TestDuplicatesRefusedIsCountedAndPersisted(t *testing.T) {
	dir := t.TempDir()
	a := openRollupArchive(t, dir)

	if a.markSeen(RecordMigration, "mig-1") {
		t.Fatal("a key nobody has offered before was refused")
	}
	if !a.markSeen(RecordMigration, "mig-1") {
		t.Fatal("the second copy of a key was not refused")
	}
	if !a.markSeen(RecordMigration, "mig-1") {
		t.Fatal("the third copy of a key was not refused")
	}
	// An empty key is never seen at all: a record with no migrationId cannot be
	// deduplicated, and counting it would invent refusals nobody made.
	if a.markSeen(RecordMigration, "") {
		t.Fatal("an empty key was refused")
	}
	if got := a.StatusView().DuplicatesRefused; got != 2 {
		t.Fatalf("duplicatesRefused=%d, want 2", got)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}

	b := openRollupArchive(t, dir)
	defer b.Close()
	if got := b.StatusView().DuplicatesRefused; got != 2 {
		t.Fatalf("duplicatesRefused=%d after a restart, want the persisted 2: a counter "+
			"that resets cannot answer 'has anything been refused since the release'", got)
	}
}

// TestTheCursorSurvivesARotationAndAGzippedSegment. The cursor is a POSITION
// now, and a position naming the live file is about a file that ROTATES: the
// bytes move into a segment and a fresh live file starts at offset 0 with
// different records. Seeking to the old offset in the new file would skip
// records and fold the rest against the wrong numbers.
//
// Two guards, and this exercises both: a rotation re-points every position the
// roll-up holds at the segment that took the bytes, and the floor line records
// the live file's own first record so a load can tell a stale position from a
// good one. Whichever fires, the invariant is the same one the whole design
// rests on — THE SIDECAR PLUS ITS TAIL IS THE FULL REPLAY.
func TestTheCursorSurvivesARotationAndAGzippedSegment(t *testing.T) {
	day := int64(24 * 60 * 60 * 1000)
	base := (time.Now().UnixMilli()/day)*day - 3*day
	// 300 records over three days, so the ledger rotates twice under the fold.
	recs := syntheticRecords(90210, 300, base, day/100)

	cfg := func(dir string) Config {
		return Config{DataDir: dir, PeerID: "archive-test", RelayURL: "ws://test",
			DedupWindow: 96 * time.Hour}
	}

	whole := t.TempDir()
	appendRecords(t, whole, recs)
	ref, err := New(cfg(whole))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	want := rollupDump(ref)
	_ = ref.Close()

	// The same records, folded in three passes with a restart between each, and
	// the segments compressed under the archive between two of them.
	dir := t.TempDir()
	for _, cut := range [][2]int{{0, 120}, {120, 240}, {240, 300}} {
		appendRecords(t, dir, recs[cut[0]:cut[1]])
		a, err := New(cfg(dir))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if err := a.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
		segs, err := LedgerSegments(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range segs {
			if s.Compressed || s.Retired {
				continue
			}
			if _, _, err := compressSegment(dir, s.Name, gzip.BestSpeed); err != nil {
				t.Fatalf("compress %s: %v", s.Name, err)
			}
		}
	}
	b, err := New(cfg(dir))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer b.Close()
	if segs, err := LedgerSegments(dir); err != nil || len(segs) < 2 {
		t.Fatalf("the fixture produced %d segments (%v), want at least 2 rotations",
			len(segs), err)
	}
	if got := rollupDump(b); got != want {
		t.Fatalf("a tail replay across rotations and compressions is not the full replay\n%s",
			firstDiff(want, got))
	}
}

// TestACursorInARetiredSegmentIsSaidRatherThanGuessed. A position naming a
// segment that has left this host is a caller whose state is older than the raw
// window — an archive restored from a backup, or one that was down for longer
// than the window. The records between the cursor and the oldest surviving
// segment are gone with it.
//
// There are two honest answers and one dishonest one. The dishonest one is to
// pick a different start and carry on, which silently drops or doubles records.
// This one keeps the aggregates, folds everything still present on top, and SAYS
// SO on the status surface, because the hole is real and only
// `deploy/coldcopy.sh --restore` can close it.
func TestACursorInARetiredSegmentIsSaidRatherThanGuessed(t *testing.T) {
	day := int64(24 * 60 * 60 * 1000)
	base := (time.Now().UnixMilli()/day)*day - 3*day
	// 200 records at a hundredth of a day is two days of ledger ending yesterday,
	// so the live file's day is over and the maintenance pass will close it.
	recs := syntheticRecords(5150, 200, base, day/100)

	// A directory whose cursor sits in a CLOSED SEGMENT: the records are folded
	// and the live file is then rotated shut, which is the ordinary state of an
	// archive across a midnight.
	seed := func() (dir string, cursor LedgerPos, records int) {
		dir = t.TempDir()
		appendRecords(t, dir, recs)
		a, err := New(Config{DataDir: dir, PeerID: "archive-test", RelayURL: "ws://test"})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		// The maintenance pass closes a live file whose day is over, with no
		// append behind it — which is how a cursor comes to name a segment.
		a.LedgerMaintenanceNow(time.Now())
		a.mu.Lock()
		cursor, records = a.ledgerPos, a.recordCount
		a.mu.Unlock()
		if cursor.Segment == "" {
			t.Fatalf("the fixture left the cursor in the live file (%+v)", cursor)
		}
		if err := a.Close(); err != nil {
			t.Fatal(err)
		}
		return dir, cursor, records
	}
	retire := func(dir, name string) {
		for _, suffix := range []string{plainSuffix, gzSuffix} {
			_ = os.Remove(filepath.Join(segmentsDir(dir), name+suffix))
		}
		// The receipt outlives the segment: it is what a restore is planned from.
		if err := os.WriteFile(
			filepath.Join(segmentsDir(dir), name+gzSuffix+receiptSuffix),
			[]byte(`{"segment":"`+name+gzSuffix+`"}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfg := func(dir string) Config {
		return Config{DataDir: dir, PeerID: "archive-test", RelayURL: "ws://test"}
	}

	// CASE 1: the cursor is older than everything still here. Every segment is
	// retired and new crossings have landed since, so what is present has never
	// been folded and all of it is.
	t.Run("older than everything present", func(t *testing.T) {
		dir, _, records := seed()
		segs, err := LedgerSegments(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, sg := range segs {
			retire(dir, sg.Name)
		}
		fresh := syntheticRecords(777, 40, time.Now().UnixMilli(), 1000)
		appendRecords(t, dir, fresh)

		b, err := New(cfg(dir))
		if err != nil {
			t.Fatalf("New after a retirement: %v", err)
		}
		defer b.Close()
		if !b.replayFromRetired || !b.StatusView().ReplayFromRetired {
			t.Error("an archive whose cursor names a retired segment does not say so")
		}
		if b.recordCount != records+len(fresh) {
			t.Errorf("recordCount=%d, want the persisted %d plus the %d new records",
				b.recordCount, records, len(fresh))
		}
	})

	// CASE 2: the cursor is NOT older than what is here — which retirement
	// cannot produce and a raw record replaced under a running deployment can.
	// Re-folding would double every counter, so nothing is folded.
	t.Run("not older than what is present", func(t *testing.T) {
		dir, cursor, records := seed()
		segs, err := LedgerSegments(dir)
		if err != nil || len(segs) < 2 {
			t.Fatalf("the fixture produced %d segments (%v), want at least 2", len(segs), err)
		}
		retire(dir, cursor.Segment)

		b, err := New(cfg(dir))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		defer b.Close()
		if !b.replayFromRetired {
			t.Error("the archive does not say its cursor named a segment that has gone")
		}
		if b.recordCount != records {
			t.Errorf("recordCount=%d, want the persisted %d: the records still here were "+
				"already folded and re-folding them doubles every counter",
				b.recordCount, records)
		}
	})
}
