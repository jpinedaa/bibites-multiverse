package archive

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"multiverse/internal/bb8"
	"multiverse/internal/contractb"
	"multiverse/internal/wire"
)

// brainBlob is a genome with a given brain, in the dialect bb8 parses. The
// neuron count is what BrainStats counts, so a test can ask for 55 neurons and
// mean "seven above the fixed floor".
func brainBlob(neurons, synapses int) string {
	nodes, syn := "", ""
	for i := 0; i < neurons; i++ {
		if i > 0 {
			nodes += ","
		}
		nodes += `{"Index":` + strconv.Itoa(i) + `,"Type":0}`
	}
	for i := 0; i < synapses; i++ {
		if i > 0 {
			syn += ","
		}
		syn += `{"NodeIn":0,"NodeOut":1,"Inov":` + strconv.Itoa(i) + `,"Weight":1.0}`
	}
	return `{"genes":{"SizeRatio":1.0},"nodes":[` + nodes + `],"synapses":[` + syn +
		`],"version":"0.6.3.1"}`
}

// brainGenome returns a blob and its REAL content hash, because the arrival path
// recomputes the hash and discards an answer whose bytes do not match its label
// (§6.10). A fixture that faked the hash would exercise the discard and not the
// fold.
func brainGenome(t *testing.T, neurons, synapses int) (blob, hash string) {
	t.Helper()
	blob = brainBlob(neurons, synapses)
	h, err := bb8.GenomeHash(blob, "0.6.3.1")
	if err != nil {
		t.Fatalf("GenomeHash: %v", err)
	}
	return blob, h
}

// arrive drives one GENOME_RESPONSE through the real handler, which is the only
// way to exercise the fold at the point it actually happens.
func arrive(t *testing.T, a *Archive, hash, blob string) {
	t.Helper()
	data, err := json.Marshal(contractb.GenomeResponse{
		GenomeHash: hash, Found: true, SourcePeer: "slot-1",
		Body: &contractb.Body{Version: "0.6.3.1", BB8: blob},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	a.onGenomeResponse(wire.Envelope{Type: contractb.TypeGenomeResponse, Data: data})
}

// brainAt is one bucket of a built history, by its start.
func brainAt(h BrainHistory, atMs int64) (BrainPoint, bool) {
	for _, p := range h.Points {
		if atMs >= p.AtMs && atMs < p.AtMs+h.BucketMs {
			return p, true
		}
	}
	return BrainPoint{}, false
}

// TestABrainIsFoldedAtArrivalIntoTheBucketOfItsCrossing is brainhist.go rule 1,
// and it is the whole reason the aggregate is maintained rather than computed.
//
// A bucket is about 42% covered while it is new and reaches about 97% five days
// later, because the fetch backlog drains BACKWARDS into it. So the measurement
// cannot be taken at the crossing — that would freeze every bucket at its
// worst-covered moment — and it is taken when the blob lands, keyed on the
// CROSSING'S OWN TIME. That key is fetch.crossedAt, which has been durable since
// the retention horizon needed it.
//
// The test drives the real arrival path with a blob whose bytes really do hash
// to their label, so what it exercises is the fold and not the discard.
func TestABrainIsFoldedAtArrivalIntoTheBucketOfItsCrossing(t *testing.T) {
	a, err := New(Config{DataDir: t.TempDir(), PeerID: "archive-test", RelayURL: "ws://test"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })

	// A crossing THREE HOURS AGO whose genome only arrives now — the ordinary
	// shape of a draining backlog.
	crossed := time.Now().Add(-3 * time.Hour)
	blob, hash := brainGenome(t, BrainNeuronFloor+7, 31)
	rec := migration(crossed.UnixMilli(), 1, 2, "E", "Beta", "one", hash)
	a.observeBrainSeen(rec.RecordedAt, hash)
	a.mu.Lock()
	a.trackLocked(lineageHash{hash: hash, own: true}, "Beta one", "slot-1", "m1", 1,
		crossed, time.Now())
	a.mu.Unlock()

	arrive(t, a, hash, blob)

	// THE BUCKET IS THE CROSSING'S, NOT THE ARRIVAL'S. Three hours of difference
	// is the entire point: measured at the arrival this reading would be filed
	// against a stretch of time the creature never crossed in.
	h := a.BrainHistoryView(crossed.Add(-time.Hour).UnixMilli(), time.Now().UnixMilli(), 240)
	p, ok := brainAt(h, crossed.UnixMilli())
	if !ok {
		t.Fatalf("no bucket covers the crossing at all: from %d to %d", h.FromMs, h.ToMs)
	}
	if p.N != 1 || p.Seen != 1 {
		t.Fatalf("the crossing's bucket holds n=%d seen=%d, want 1 and 1", p.N, p.Seen)
	}
	if p.MedSyn == nil || *p.MedSyn != 31 {
		t.Fatalf("median synapses is %v, want 31", p.MedSyn)
	}
	// THE FLOOR IS SUBTRACTED AND NOTHING ELSE IS. 48 of every neuron count is
	// the fixed sensor and actuator set, so a genome of 55 neurons is a brain
	// with SEVEN of its own — and the raw count would have said this creature is
	// 15% more complicated than an empty one instead of 7 neurons more.
	if p.MedHid == nil || *p.MedHid != 7 {
		t.Fatalf("median hidden neurons is %v, want 7 (55 less the fixed %d)",
			p.MedHid, BrainNeuronFloor)
	}
	if now, ok := brainAt(h, time.Now().UnixMilli()); ok && now.N != 0 {
		t.Fatalf("the reading was filed against the ARRIVAL's bucket too: %+v", now)
	}

	// AND THE ARRIVAL FILLS THE PER-HASH PARSE CACHE rather than leaving the miss
	// that was cached while the fetch was outstanding. The bytes were just read;
	// re-reading them from the store later would be the lag brain.go names.
	a.brains.mu.Lock()
	e, cached := a.brains.byHash[hash]
	a.brains.mu.Unlock()
	if !cached || !e.known || e.brain.Synapses != 31 {
		t.Fatalf("the arrival did not fill the parse cache: cached=%v %+v", cached, e)
	}
}

// TestAHashTheStoreAlreadyHoldsIsFoldedAtTheCrossing is the aggregate's OTHER
// write point, and it exists because a crossing whose genome the archive already
// holds has no arrival left to fold at. Two call sites, one fold function, no
// third view of the same fact.
func TestAHashTheStoreAlreadyHoldsIsFoldedAtTheCrossing(t *testing.T) {
	a, err := New(Config{DataDir: t.TempDir(), PeerID: "archive-test", RelayURL: "ws://test"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })

	blob, hash := brainGenome(t, BrainNeuronFloor+3, 12)
	if err := a.genomes.Put(hash, "0.6.3.1", blob); err != nil {
		t.Fatalf("store: %v", err)
	}
	at := time.Now().Add(-30 * time.Minute).UnixMilli()

	// The live path, exactly as onMigration performs it.
	a.observeBrainSeen(at, hash)
	br, ok := a.brainFor(hash)
	if !ok {
		t.Fatal("the stored blob did not parse")
	}
	a.observeBrainHeld(at, "Beta one", hash, br)

	// The three reads below take three DISTINCT windows on purpose: the view is
	// cached for ten seconds on (from, to, buckets), and a test fast enough to ask
	// twice inside one millisecond would be reading the first answer twice.
	nowMs := time.Now().UnixMilli()
	h := a.BrainHistoryView(at-time.Hour.Milliseconds(), nowMs, 120)
	p, found := brainAt(h, at)
	if !found || p.N != 1 || p.MedSyn == nil || *p.MedSyn != 12 || p.MedHid == nil || *p.MedHid != 3 {
		t.Fatalf("an already-held genome was not folded at its crossing: %+v", p)
	}

	// THE SAME GENOME CROSSING AGAIN IN THE SAME SLICE IS ONE GENOME. The rule is
	// every held DISTINCT genome, deduplicated by hash and never weighted by
	// crossings — a species that crosses two hundred times on one genome must not
	// drown out two hundred others.
	for i := 0; i < 5; i++ {
		a.observeBrainSeen(at+int64(i), hash)
		a.observeBrainHeld(at+int64(i), "Beta one", hash, br)
	}
	h = a.BrainHistoryView(at-time.Hour.Milliseconds(), nowMs+1, 120)
	p, _ = brainAt(h, at)
	if p.N != 1 || p.Seen != 1 {
		t.Fatalf("repeat crossings of one genome were counted %d/%d times, want 1 and 1",
			p.N, p.Seen)
	}

	// AND THE SAME GENOME IN A DIFFERENT SLICE IS A DIFFERENT SAMPLE. It really
	// did cross then, and the slice it crossed in is entitled to say what crossed.
	later := at + BrainBucketMs*3
	a.observeBrainSeen(later, hash)
	a.observeBrainHeld(later, "Beta one", hash, br)
	h = a.BrainHistoryView(at-time.Hour.Milliseconds(), nowMs+2, 120)
	if q, _ := brainAt(h, later); q.N != 1 {
		t.Fatalf("a genome crossing in a second slice was deduplicated across slices: %+v", q)
	}
}

// TestAParentGenomeIsMeasuredForNothing is the attribution rule, and it is the
// one place this aggregate could quietly lie about a trend.
//
// A parent hash in the lineage annex is the genome of the migrant's MOTHER OR
// FATHER — an individual, describing an earlier organism — and the species
// block's parentGenericName names a TAXONOMIC ancestor, which is a different
// thing again. Folding a parent's genome at the child's crossing time would drag
// older brains forward and flatten exactly the trend the panel exists to show,
// and attributing it to the named parent species would put a measurement on a
// species no record supports.
func TestAParentGenomeIsMeasuredForNothing(t *testing.T) {
	a, err := New(Config{DataDir: t.TempDir(), PeerID: "archive-test", RelayURL: "ws://test"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })

	crossed := time.Now().Add(-time.Hour)
	ownBlob, own := brainGenome(t, BrainNeuronFloor+9, 40)
	parBlob, par := brainGenome(t, BrainNeuronFloor+1, 3)

	a.observeBrainSeen(crossed.UnixMilli(), own)
	a.mu.Lock()
	a.trackLocked(lineageHash{hash: own, own: true}, "Beta one", "slot-1", "m1", 1,
		crossed, time.Now())
	a.trackLocked(lineageHash{hash: par}, "Beta one", "slot-1", "m1", 1,
		crossed, time.Now())
	a.mu.Unlock()
	arrive(t, a, par, parBlob)
	arrive(t, a, own, ownBlob)

	h := a.BrainHistoryView(crossed.Add(-time.Hour).UnixMilli(), time.Now().UnixMilli(), 120)
	p, _ := brainAt(h, crossed.UnixMilli())
	if p.N != 1 {
		t.Fatalf("the bucket holds %d genomes, want only the migrant's own", p.N)
	}
	if p.MedSyn == nil || *p.MedSyn != 40 {
		t.Fatalf("the parent's smaller brain reached the series: median %v, want 40", p.MedSyn)
	}
	// AND NEITHER GENOME'S SHAPE IS FILED UNDER A SPECIES THE PARENT MIGHT BE.
	// Only the migrant's own is, under the migrant's own key.
	if r, ok := a.brainAgg.record("Beta one"); !ok || r.Synapses != 40 {
		t.Fatalf("the migrant's own genome is not the species' record: %+v (%v)", r, ok)
	}
}

// TestTheBrainHistoryIsReAggregatedByMergingDistributions is rule 4, and it is
// the property that lets the panel share an axis that re-fits.
//
// The aggregate is kept at five minutes and the panel asks for whatever window
// the genealogy is drawing. A drawn bucket's median must therefore be the MEDIAN
// OF EVERY HELD GENOME INSIDE IT — which is what merging histograms gives — and
// not a mean of the finer medians it swallowed, which is a different number and
// is what a fixed-resolution series would have to produce.
func TestTheBrainHistoryIsReAggregatedByMergingDistributions(t *testing.T) {
	// Two source buckets: one holding nine genomes of 10 synapses, the next a
	// single genome of 100. The mean of the two medians is 55; the true median of
	// all ten is 10, and 10 is the honest answer about what crossed.
	src := []BrainBucket{
		{AtMs: 0, Held: 9, Seen: 9,
			Neurons:  map[int]int{BrainNeuronFloor + 1: 9},
			Synapses: map[int]int{10: 9}},
		{AtMs: BrainBucketMs, Held: 1, Seen: 1,
			Neurons:  map[int]int{BrainNeuronFloor + 30: 1},
			Synapses: map[int]int{100: 1}},
	}
	// A window of sixteen source buckets asked for in eight, so each drawn bucket
	// swallows two source ones and the two above land in the same drawn bucket.
	h := BuildBrainHistory(src, 0, 16*BrainBucketMs, 8)
	if h.BucketMs != 2*BrainBucketMs {
		t.Fatalf("the drawn resolution is %d, want two source buckets (%d)",
			h.BucketMs, 2*BrainBucketMs)
	}
	var merged *BrainPoint
	for i := range h.Points {
		if h.Points[i].N > 0 {
			if merged != nil {
				t.Fatal("the two source buckets landed in two drawn buckets; the merge is untested")
			}
			merged = &h.Points[i]
		}
	}
	if merged == nil {
		t.Fatal("nothing was drawn at all")
	}
	if merged.N != 10 {
		t.Fatalf("the merged bucket rests on %d genomes, want 10", merged.N)
	}
	if merged.MedSyn == nil || *merged.MedSyn != 10 {
		t.Fatalf("the merged median is %v, want the true median 10 of all ten genomes — a mean "+
			"of the two source medians would be 55", merged.MedSyn)
	}
	// The band is the middle half of the MERGED population, so the outlier is
	// outside it and is still reported as the extreme it is.
	if merged.HiSyn == nil || *merged.HiSyn != 10 || merged.MaxSyn == nil || *merged.MaxSyn != 100 {
		t.Fatalf("the band swallowed the outlier: p75=%v max=%v", merged.HiSyn, merged.MaxSyn)
	}
	// AND THE FLOOR TRAVELS THROUGH THE MERGE. hidden() is monotone, so the
	// quantile of the transform is the transform of the quantile.
	if merged.MedHid == nil || *merged.MedHid != 1 {
		t.Fatalf("the merged hidden-neuron median is %v, want 1", merged.MedHid)
	}
	if h.NeuronFloor != BrainNeuronFloor {
		t.Fatalf("the floor is not published: %d", h.NeuronFloor)
	}
}

// TestAGapStaysAGapAndIsNeverAZero is history.go rule 2 carried onto this panel,
// and on this record it is not a hypothetical: 45 of the first 183 hours hold no
// crossing at all, including one stretch of a whole day.
//
// Two DIFFERENT absences have to survive, because they mean different things and
// a reader acts differently on them:
//
//	NOTHING CROSSED — the map was down. No reading, and nothing was measurable.
//
//	THINGS CROSSED AND NONE OF THEM WAS EVER READ. No reading either, but the
//	coverage denominator is not zero, and the panel can say how much it missed.
func TestAGapStaysAGapAndIsNeverAZero(t *testing.T) {
	src := []BrainBucket{
		{AtMs: 0, Held: 4, Seen: 4,
			Neurons: map[int]int{BrainNeuronFloor + 2: 4}, Synapses: map[int]int{9: 4}},
		// A slice with crossings and no measurement at all.
		{AtMs: BrainBucketMs, Held: 0, Seen: 17},
		// And a slice with nothing in it whatever: not even a crossing.
		{AtMs: 2 * BrainBucketMs, Held: 6, Seen: 6,
			Neurons: map[int]int{BrainNeuronFloor + 5: 6}, Synapses: map[int]int{20: 6}},
	}
	h := BuildBrainHistory(src, 0, 4*BrainBucketMs, 4)
	if h.BucketMs != BrainBucketMs {
		t.Fatalf("the drawn resolution is %d, want the source's %d", h.BucketMs, BrainBucketMs)
	}
	if len(h.Points) != 4 {
		t.Fatalf("%d points, want 4", len(h.Points))
	}
	unread := h.Points[1]
	if unread.MedSyn != nil || unread.MedHid != nil {
		t.Fatalf("a slice with no measurement published a reading: %+v", unread)
	}
	if unread.N != 0 || unread.Seen != 17 {
		t.Fatalf("the unmeasured slice lost its coverage denominator: n=%d seen=%d",
			unread.N, unread.Seen)
	}
	empty := h.Points[3]
	if empty.MedSyn != nil || empty.N != 0 || empty.Seen != 0 {
		t.Fatalf("a slice with no crossing at all is not empty: %+v", empty)
	}
	// AND THE TWO READINGS EITHER SIDE ARE NOT JOINED THROUGH THE HOLE. Nothing
	// here carries a value forward or interpolates one, so a renderer that draws
	// runs of consecutive readings cannot draw a line across the outage.
	if h.Points[0].MedSyn == nil || h.Points[2].MedSyn == nil {
		t.Fatal("the readings either side of the gap were lost")
	}
	if *h.Points[0].MedSyn != 9 || *h.Points[2].MedSyn != 20 {
		t.Fatalf("the gap moved its neighbours: %d and %d", *h.Points[0].MedSyn, *h.Points[2].MedSyn)
	}
}

// TestCoverageIsCountedFromTheRecordAndTheHolding is the accounting behind the
// strip under the panel, and the reason the panel publishes two numbers rather
// than a ratio: n is what was measured, seen is what the record says crossed,
// and a reader can tell the difference between "few genomes crossed" and "few of
// them were readable".
func TestCoverageIsCountedFromTheRecordAndTheHolding(t *testing.T) {
	a, err := New(Config{DataDir: t.TempDir(), PeerID: "archive-test", RelayURL: "ws://test"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })

	at := time.Now().Add(-20 * time.Minute).UnixMilli()
	// Four distinct genomes crossed; the archive can read two of them.
	var hashes []string
	for i := 0; i < 4; i++ {
		blob, hash := brainGenome(t, BrainNeuronFloor+i, 10+i)
		hashes = append(hashes, hash)
		a.observeBrainSeen(at, hash)
		if i < 2 {
			if err := a.genomes.Put(hash, "0.6.3.1", blob); err != nil {
				t.Fatalf("store: %v", err)
			}
			br, ok := a.brainFor(hash)
			if !ok {
				t.Fatal("blob did not parse")
			}
			a.observeBrainHeld(at, "Beta one", hash, br)
		}
	}
	h := a.BrainHistoryView(at-time.Hour.Milliseconds(), time.Now().UnixMilli(), 120)
	p, _ := brainAt(h, at)
	if p.Seen != 4 || p.N != 2 {
		t.Fatalf("coverage is %d/%d, want 2 of 4", p.N, p.Seen)
	}
	// HELD IS A SUBSET OF SEEN, always: every held fold's slice came from a
	// crossing that folded seen first. A coverage over 100% would mean the panel
	// was measuring genomes the record does not hold.
	if p.N > p.Seen {
		t.Fatalf("more genomes measured than crossed: %d of %d", p.N, p.Seen)
	}
	if h.Genomes != 2 || h.Seen != 4 {
		t.Fatalf("the window's own totals are %d/%d, want 2 and 4", h.Genomes, h.Seen)
	}
	_ = hashes
}

// TestTheBrainSidecarRoundTripsAndItsLossStartsHistoryNow is rule 3, both halves.
//
// THE ROUND TRIP: a restart replays the sidecar rather than re-deriving anything.
// Re-deriving would mean reading and parsing every blob in the store — about
// eleven minutes on top of a 28-second replay, growing with the store forever —
// and the horizon makes an old bucket permanently unrecomputable anyway.
//
// THE LOSS: a sidecar that cannot be used means THIS HISTORY STARTS NOW. Never a
// run of zeroes, which would say the creatures on this map had no brains — a
// claim about the world made out of a failure of the record.
func TestTheBrainSidecarRoundTripsAndItsLossStartsHistoryNow(t *testing.T) {
	dir := t.TempDir()
	at := time.Now().Add(-2 * time.Hour).UnixMilli()
	blob, hash := brainGenome(t, BrainNeuronFloor+11, 44)

	a, err := New(Config{DataDir: dir, PeerID: "archive-test", RelayURL: "ws://test"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// A crossing IN THE LEDGER, so the restart can rebuild the coverage
	// denominator from the record — and a measurement, which it cannot.
	if err := a.ledger.Append(migration(at, 1, 2, "E", "Beta", "one", hash)); err != nil {
		t.Fatalf("append: %v", err)
	}
	a.observeBrainSeen(at, hash)
	a.observeBrainHeld(at, "Beta one", hash, bb8.Brain{Neurons: BrainNeuronFloor + 11, Synapses: 44})
	if err := a.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	_ = blob

	// ---- the round trip.
	b, err := New(Config{DataDir: dir, PeerID: "archive-test", RelayURL: "ws://test"})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	h := b.BrainHistoryView(at-time.Hour.Milliseconds(), time.Now().UnixMilli(), 240)
	p, ok := brainAt(h, at)
	if !ok || p.N != 1 || p.MedSyn == nil || *p.MedSyn != 44 || p.MedHid == nil || *p.MedHid != 11 {
		t.Fatalf("the measurement did not survive the restart: %+v (%v)", p, ok)
	}
	if p.Seen != 1 {
		t.Fatalf("the coverage denominator was not rebuilt from the ledger: seen=%d", p.Seen)
	}
	if !h.Persisted {
		t.Fatal("the view does not report that a sidecar was replayed")
	}
	// AND THE PER-SPECIES MEASUREMENT COMES BACK WITH IT, which is what stops a
	// restart emptying the tree's rings.
	if r, ok := b.brainAgg.record("Beta one"); !ok || r.Synapses != 44 || r.AtMs != at {
		t.Fatalf("the species' brain record did not survive the restart: %+v (%v)", r, ok)
	}
	if err := b.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// ---- the loss. The file is truncated to a header and half a record, which is
	// what a kill in the middle of a write leaves.
	path := dir + "/" + brainSidecarName
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read sidecar: %v", err)
	}
	// A TORN TAIL IS NOT A LOSS: it is dropped and everything before it is kept.
	// Corrupting a line in the MIDDLE is the loss, because the file is written by
	// one writer in one shape and a broken middle means it is not what this reader
	// thinks it is.
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(lines) < 3 {
		t.Fatalf("the sidecar holds %d lines; the fixture needs a header and two records "+
			"to corrupt a middle one", len(lines))
	}
	lines[1] = "{not json"
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	c, err := New(Config{DataDir: dir, PeerID: "archive-test", RelayURL: "ws://test"})
	if err != nil {
		t.Fatalf("reopen after loss: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	h = c.BrainHistoryView(at-time.Hour.Milliseconds(), time.Now().UnixMilli(), 240)
	if !h.Lost {
		t.Fatal("an unusable sidecar is not reported as a loss")
	}
	p, _ = brainAt(h, at)
	// NOT ZERO. The slice publishes NO READING — the same absence an outage
	// produces — and keeps the coverage denominator the ledger still supports, so
	// the panel can say "the record holds a crossing here and I measured none".
	if p.MedSyn != nil || p.MedHid != nil {
		t.Fatalf("a lost sidecar produced readings out of nothing: %+v", p)
	}
	if p.N != 0 {
		t.Fatalf("a lost measurement is reported as %d measured genomes", p.N)
	}
	if p.Seen != 1 {
		t.Fatalf("the loss took the record's own crossing with it: seen=%d", p.Seen)
	}
	// And the unreadable bytes are kept rather than deleted.
	if _, err := os.Stat(path + ".unreadable"); err != nil {
		t.Fatalf("the unusable sidecar was not kept beside the new one: %v", err)
	}
	// A species measured before the loss is absent afterwards, and absent is
	// ABSENT: no ring, never a ring of zero.
	if r, ok := c.brainAgg.record("Beta one"); ok {
		t.Fatalf("a lost measurement came back as a reading: %+v", r)
	}
}

// TestAKeptMeasurementOutlivesItsBlobAndGivesAnAncestorARing is the standing
// defect the persisted record fixes, stated as the three things it buys.
//
// Before it, the ring came from a per-process cache over blobs the store still
// held. So a restart emptied the picture, and an EXTINCT ancestor could never
// gain a ring at all — nothing is fetching a dead species' genomes any more, and
// once its blobs pass the retention horizon the measurement is permanently
// unobtainable. The tree's interior nodes, which are the reason the tree has
// interior nodes, were structurally ringless.
func TestAKeptMeasurementOutlivesItsBlobAndGivesAnAncestorARing(t *testing.T) {
	// Two living species with a common ancestor that is alive nowhere: the shape
	// the genealogy exists to draw, and the shape that had no ring.
	status := contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 1, Height: 1}, SlotCount: 1,
		Slots: []contractb.SlotInfo{slot(1, 0, 0, true, census(20, 0,
			entry("Beta", "one", 10, 0), entry("Gamma", "two", 10, 0)))},
	}
	a := newViewFixture(t, status, time.Second)
	base := time.Now().Add(-4 * time.Hour).UnixMilli()

	blob, hash := brainGenome(t, BrainNeuronFloor+6, 22)
	if err := a.genomes.Put(hash, "0.6.3.1", blob); err != nil {
		t.Fatalf("store: %v", err)
	}
	a.mu.Lock()
	// The ancestor crossed once, long ago, carrying a genome the store then held.
	a.observeSpeciesLocked(migration(base, 1, 2, "E", "Alpha", "nullus", hash))
	a.observeSpeciesLocked(child(base+1000, "Beta", "one", "Alpha", "nullus"))
	a.observeSpeciesLocked(child(base+2000, "Gamma", "two", "Alpha", "nullus"))
	a.mu.Unlock()

	byKey := treeNodes(t, a.SpeciesTreeView())
	anc, ok := byKey["Alpha nullus"]
	if !ok {
		t.Fatal("the branch point was not kept; the fixture is wrong")
	}
	if anc.Alive {
		t.Fatal("the ancestor is alive; this test is about the ones that are not")
	}
	if anc.Neurons != BrainNeuronFloor+6 || anc.Synapses != 22 {
		t.Fatalf("the ancestor has no ring: %d/%d", anc.Neurons, anc.Synapses)
	}
	// AND THE MEASUREMENT SAYS HOW OLD IT IS, because it can be days old and is
	// drawn beside living species' current readings.
	if anc.BrainAtMs != base {
		t.Fatalf("the measurement is undated: %d, want the crossing's %d", anc.BrainAtMs, base)
	}

	// ---- THE BLOB IS PRUNED, exactly as the retention horizon prunes one, and the
	// per-process parse cache is emptied, exactly as a restart empties one. The
	// measurement is the thing that has to survive both.
	hex := bb8.HashHex(hash)
	if err := os.Remove(a.genomes.Dir() + "/" + hex[:2] + "/" + hex + ".json"); err != nil {
		t.Fatalf("prune: %v", err)
	}
	a.brains.mu.Lock()
	a.brains.byHash = map[string]brainEntry{}
	a.brains.order = nil
	a.brains.mu.Unlock()

	byKey = treeNodes(t, a.SpeciesTreeView())
	if anc := byKey["Alpha nullus"]; anc.Neurons != BrainNeuronFloor+6 || anc.Synapses != 22 {
		t.Fatalf("the ancestor's ring died with its blob: %d/%d — the whole point of keeping "+
			"the measurement is that the horizon prunes the genome and not the number",
			anc.Neurons, anc.Synapses)
	}

	// ---- ABSENCE IS STILL ABSENCE. A species this archive never read a genome
	// for draws nothing at all, persisted record or not.
	if n := byKey["Gamma two"]; n.Neurons != 0 || n.Synapses != 0 || n.BrainAtMs != 0 {
		t.Fatalf("a species with no reading was given one: %+v", n)
	}
}

// TestAMeasurementNeverGoesBackwards is the ordering rule that makes an
// out-of-order arrival safe. Blobs land days after their crossings and in
// whatever order the fetch queue reaches them, so the record is latest-writer-
// wins on the CROSSING's clock and never on the reader's.
func TestAMeasurementNeverGoesBackwards(t *testing.T) {
	a, err := New(Config{DataDir: t.TempDir(), PeerID: "archive-test", RelayURL: "ws://test"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })

	newAt := time.Now().Add(-time.Hour).UnixMilli()
	oldAt := time.Now().Add(-5 * time.Hour).UnixMilli()
	// The NEWER genome is read first, the older one afterwards — the ordinary
	// shape of a queue draining out of order.
	a.observeBrainHeld(newAt, "Beta one", "hash-new", bb8.Brain{Neurons: 60, Synapses: 50})
	a.observeBrainHeld(oldAt, "Beta one", "hash-old", bb8.Brain{Neurons: 49, Synapses: 2})

	r, ok := a.brainAgg.record("Beta one")
	if !ok || r.Synapses != 50 || r.AtMs != newAt {
		t.Fatalf("an older genome read later overwrote a newer measurement: %+v", r)
	}
	// AND BOTH ARE STILL IN THE SERIES, because both really crossed: the series is
	// about what crossed when, and only the RECORD is a single latest answer.
	h := a.BrainHistoryView(oldAt-time.Hour.Milliseconds(), time.Now().UnixMilli(), 360)
	if p, _ := brainAt(h, oldAt); p.MedSyn == nil || *p.MedSyn != 2 {
		t.Fatalf("the older crossing lost its own slice's reading: %+v", p)
	}
	if p, _ := brainAt(h, newAt); p.MedSyn == nil || *p.MedSyn != 50 {
		t.Fatalf("the newer crossing lost its own slice's reading: %+v", p)
	}
}

// TestTheBrainEndpointTakesTheWindowTheDrawingIsUsing is what lets the panel
// share the genealogy's axis. That axis re-fits to the drawn set, so the page
// sends the two edges it is actually drawing rather than an hour count ending at
// now, and every bound is clamped rather than trusted.
func TestTheBrainEndpointTakesTheWindowTheDrawingIsUsing(t *testing.T) {
	a, err := New(Config{DataDir: t.TempDir(), PeerID: "archive-test", RelayURL: "ws://test"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })

	at := time.Now().Add(-90 * time.Minute).UnixMilli()
	a.observeBrainSeen(at, "hash-a")
	a.observeBrainHeld(at, "Beta one", "hash-a", bb8.Brain{Neurons: BrainNeuronFloor + 4, Synapses: 17})

	srv := httptest.NewServer(a.httpHandler())
	t.Cleanup(srv.Close)

	from := at - time.Hour.Milliseconds()
	to := time.Now().UnixMilli()
	url := srv.URL + "/api/species/brains?from=" + strconv.FormatInt(from, 10) +
		"&to=" + strconv.FormatInt(to, 10) + "&buckets=64"
	res, err := http.Get(url)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer res.Body.Close()
	var got BrainHistory
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// The answer publishes the resolution it actually used: never finer than the
	// aggregate holds, never more buckets than were asked for, and a left edge on
	// a source-bucket boundary at or before the one requested.
	if got.Buckets > 64 || len(got.Points) != got.Buckets {
		t.Fatalf("asked for at most 64 buckets, got %d/%d", got.Buckets, len(got.Points))
	}
	if got.BucketMs < BrainBucketMs || got.BucketMs%BrainBucketMs != 0 {
		t.Fatalf("the drawn resolution %d is not a whole number of source buckets", got.BucketMs)
	}
	if got.FromMs > from || from-got.FromMs >= BrainBucketMs {
		t.Fatalf("the answer starts at %d, not on the source boundary at or before the window "+
			"the drawing asked for (%d)", got.FromMs, from)
	}
	if got.SourceBucketMs != BrainBucketMs {
		t.Fatalf("the answer does not publish the resolution it actually holds: %d",
			got.SourceBucketMs)
	}
	p, ok := brainAt(got, at)
	if !ok || p.MedSyn == nil || *p.MedSyn != 17 {
		t.Fatalf("the reading is not on the requested window: %+v (%v)", p, ok)
	}

	// EVERY BOUND IS CLAMPED. A caller may ask for a different window; it may not
	// ask this archive to build an unbounded answer.
	res2, err := http.Get(srv.URL + "/api/species/brains?from=1&to=" +
		strconv.FormatInt(to, 10) + "&buckets=99999")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer res2.Body.Close()
	var big BrainHistory
	if err := json.NewDecoder(res2.Body).Decode(&big); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if big.Buckets > BrainMaxBuckets {
		t.Fatalf("the bucket count was not clamped: %d", big.Buckets)
	}
	if big.BucketMs%BrainBucketMs != 0 {
		t.Fatalf("the clamped resolution %d is not a whole number of source buckets", big.BucketMs)
	}
	// The left edge, not the right: the last drawn slice runs to the end of the
	// slice it is, so ToMs may sit up to one bucket past the moment asked for.
	// What must be bounded is how far back the answer reaches.
	if to-big.FromMs > BrainMaxWindow.Milliseconds()+BrainBucketMs {
		t.Fatalf("the window was not clamped: it reaches back %d ms", to-big.FromMs)
	}
	// AND IT CARRIES NO SPECIES NAME AT ALL, which is why it is the one endpoint
	// on this mux with no deny list applied: there is nothing on it to suppress.
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, forbidden := range []string{"Beta", "name", "key", "species"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("the brain series carries %q; it is meant to be counts and times only, "+
				"which is what makes the missing deny list a property rather than an omission",
				forbidden)
		}
	}
}
