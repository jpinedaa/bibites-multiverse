package archive

// The durable half of contract-b-m4.md §33, B49: what the archive REMEMBERS
// about who is on the map. Each test below pins one of the four rules the store
// exists for — a first sighting that is set once, a simulated clock that never
// goes down, a name that is cleared when the participant clears it and kept when
// nothing was said, and a record that survives the process.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"multiverse/internal/contractb"
)

// recogSlot is one seat with a chosen peer id, live, at (n-1, 0).
func recogSlot(n int, peerID string, stats *contractb.PeerStats) contractb.SlotInfo {
	s := slot(n, n-1, 0, true, stats)
	s.PeerID = peerID
	return s
}

// recogStatus is one PEER_STATUS carrying the seats given.
func recogStatus(slots ...contractb.SlotInfo) contractb.PeerStatus {
	return contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: len(slots), Height: 1},
		SlotCount: len(slots), Slots: slots,
	}
}

// keptBy is the stats block B49 added the two fields to. sim may be nil, which
// is what a block from a sidecar with no mod connected looks like.
func keptBy(keeper, world string, sim *float64) *contractb.PeerStats {
	return &contractb.PeerStats{Keeper: keeper, WorldName: world, SimulatedTime: sim}
}

func openRecog(t *testing.T, dir string) *recognitionStore {
	t.Helper()
	r, err := openRecognitionStore(dir)
	if err != nil {
		t.Fatalf("openRecognitionStore: %v", err)
	}
	return r
}

// TestAFirstSightingIsRecordedOnceAndNeverMoves. firstSeenMs answers "since
// when has this archive known about you", and an answer that crept forward on
// every frame would be a clock rather than an arrival.
func TestAFirstSightingIsRecordedOnceAndNeverMoves(t *testing.T) {
	r := openRecog(t, t.TempDir())

	r.observe(recogStatus(recogSlot(1, "peer-one", keptBy("ada", "Tidepool", f64(10)))), 1_000)
	e, ok := r.lookup("peer-one")
	if !ok {
		t.Fatal("the first PEER_STATUS a peer appeared in recorded nothing about it")
	}
	if e.FirstSeenMs != 1_000 || e.Slot != 1 {
		t.Fatalf("first sighting = %+v, want firstSeenMs 1000 in slot 1", e)
	}

	// Later frames: one that renames the world, one with no stats block at all.
	// Neither is an arrival.
	r.observe(recogStatus(recogSlot(1, "peer-one", keptBy("ada", "Rockpool", f64(90)))), 9_000)
	r.observe(recogStatus(recogSlot(1, "peer-one", nil)), 20_000)
	if e, _ = r.lookup("peer-one"); e.FirstSeenMs != 1_000 {
		t.Fatalf("firstSeenMs moved to %d; a first sighting happens once", e.FirstSeenMs)
	}
	if e.WorldName != "Rockpool" {
		t.Fatalf("the world's later name did not land: %+v", e)
	}

	// A peer that joins later gets its OWN arrival, and the one already there is
	// not re-stamped with it. The store is keyed on peerId, so two seats are two
	// records.
	r.observe(recogStatus(
		recogSlot(1, "peer-one", keptBy("ada", "Rockpool", f64(120))),
		recogSlot(2, "peer-two", keptBy("bo", "Saltmarsh", f64(5)))), 30_000)
	if e, _ = r.lookup("peer-one"); e.FirstSeenMs != 1_000 {
		t.Fatalf("an established peer's firstSeenMs moved to %d when a new one joined", e.FirstSeenMs)
	}
	two, ok := r.lookup("peer-two")
	if !ok || two.FirstSeenMs != 30_000 || two.Slot != 2 {
		t.Fatalf("the newcomer's record = %+v, want firstSeenMs 30000 in slot 2", two)
	}

	// A seat with no peer in it is a reservation, not a participant.
	r.observe(recogStatus(recogSlot(3, "", keptBy("nobody", "Nowhere", nil))), 40_000)
	if r.Len() != 2 {
		t.Fatalf("the store remembers %d peers, want 2; an empty peerId is not a participant",
			r.Len())
	}
}

// TestTheRememberedSimulatedTimeIsAHighWaterMark is the continuity rule from the
// other side (achieved.go:110). simulatedTime runs continuously across a restart
// of the same world, so a FALL means a save was restored from behind where the
// world had got to — a rewind of the live clock and never a rewind of what the
// world actually simulated. achieved.go throws its window away, because a rate
// across a rewind is not a measurement; the stored total simply does not fall.
func TestTheRememberedSimulatedTimeIsAHighWaterMark(t *testing.T) {
	r := openRecog(t, t.TempDir())

	r.observe(recogStatus(recogSlot(1, "peer-one", keptBy("ada", "Tidepool", f64(864_000)))), 1_000)
	// The restore: the live clock reads a tenth of what it did a frame ago.
	r.observe(recogStatus(recogSlot(1, "peer-one", keptBy("ada", "Tidepool", f64(86_400)))), 2_000)
	e, _ := r.lookup("peer-one")
	if e.MaxSimulatedTime != 864_000 {
		t.Fatalf("maxSimulatedTime fell to %v after a restore; ten simulated days did not "+
			"un-happen because a save was loaded from behind them", e.MaxSimulatedTime)
	}
	// It resumes from the restored clock and only rises again past the mark.
	r.observe(recogStatus(recogSlot(1, "peer-one", keptBy("ada", "Tidepool", f64(863_999)))), 3_000)
	if e, _ = r.lookup("peer-one"); e.MaxSimulatedTime != 864_000 {
		t.Fatalf("maxSimulatedTime = %v, want the mark held at 864000", e.MaxSimulatedTime)
	}
	r.observe(recogStatus(recogSlot(1, "peer-one", keptBy("ada", "Tidepool", f64(900_000)))), 4_000)
	if e, _ = r.lookup("peer-one"); e.MaxSimulatedTime != 900_000 {
		t.Fatalf("maxSimulatedTime = %v, want 900000 once the world passed its own mark",
			e.MaxSimulatedTime)
	}

	// ABSENT IS NOT ZERO. A block with no simulatedTime on it — a sidecar whose
	// mod is not connected — leaves the mark alone and never lowers it, and a
	// peer no block ever carried one for reads UNKNOWN rather than 0.
	r.observe(recogStatus(recogSlot(1, "peer-one", keptBy("ada", "Tidepool", nil))), 5_000)
	if e, _ = r.lookup("peer-one"); e.MaxSimulatedTime != 900_000 || !e.HaveSimulatedTime {
		t.Fatalf("a mod-quiet block moved the mark: %+v", e)
	}
	r.observe(recogStatus(recogSlot(2, "peer-two", keptBy("bo", "Saltmarsh", nil))), 6_000)
	two, _ := r.lookup("peer-two")
	if two.HaveSimulatedTime {
		t.Fatalf("a peer no simulatedTime ever arrived for claims to know one: %+v", two)
	}
	// And a world that has genuinely simulated nothing reads 0 rather than
	// unknown, which is the distinction the flag exists for.
	r.observe(recogStatus(recogSlot(2, "peer-two", keptBy("bo", "Saltmarsh", f64(0)))), 7_000)
	if two, _ = r.lookup("peer-two"); !two.HaveSimulatedTime || two.MaxSimulatedTime != 0 {
		t.Fatalf("a world at exactly zero simulated seconds reads %+v, want a known 0", two)
	}
}

// TestAClearedNameIsClearedAndADarkWorldKeepsTheOneItHad is the consent half of
// §33 and the operator half of it, in one test, because the two rules are only
// correct together.
//
// A FRESH BLOCK IS THE PARTICIPANT SPEAKING. One that omits the fields says "I
// have no name published", and a store that kept the last name it liked would
// turn a deletion into a durable publication. A slot with NO BLOCK AT ALL is the
// opposite case: nothing was said, so the last thing said stands — which is what
// lets an operator name the world that went dark instead of pointing at a peer
// id (B49's own justification).
func TestAClearedNameIsClearedAndADarkWorldKeepsTheOneItHad(t *testing.T) {
	r := openRecog(t, t.TempDir())

	r.observe(recogStatus(
		recogSlot(1, "peer-one", keptBy("ada", "Tidepool", f64(100))),
		recogSlot(2, "peer-two", keptBy("bo", "Saltmarsh", f64(100)))), 1_000)

	// peer-one's participant removed the configuration and restarted. peer-two
	// went dark: its seat is still reserved and nothing is publishing on it.
	dark := recogSlot(2, "peer-two", nil)
	dark.Live, dark.ModConnected = false, false
	dark.DarkSinceMs = 2_000
	r.observe(recogStatus(recogSlot(1, "peer-one", keptBy("", "", f64(200))), dark), 2_000)

	one, _ := r.lookup("peer-one")
	if one.Keeper != "" || one.WorldName != "" {
		t.Fatalf("a participant who removed their name is still published as %+v; a fresh block "+
			"that omits the fields is the participant asking to be unnamed", one)
	}
	if one.MaxSimulatedTime != 200 || one.FirstSeenMs != 1_000 {
		t.Fatalf("clearing the name took the rest of the record with it: %+v", one)
	}
	two, _ := r.lookup("peer-two")
	if two.Keeper != "bo" || two.WorldName != "Saltmarsh" {
		t.Fatalf("the dark world lost its name (%+v); naming a dark slot is the case B49 was "+
			"written for", two)
	}

	// The strings are stored EXACTLY as they arrived. §33 puts the trimming,
	// stripping and clipping at the authoring sidecar and forbids a second party
	// doing it again, exactly as no party repairs a census name.
	r.observe(recogStatus(recogSlot(1, "peer-one",
		keptBy("  ada  ", "<script>alert(1)</script>", nil))), 3_000)
	if one, _ = r.lookup("peer-one"); one.Keeper != "  ada  " {
		t.Fatalf("the keeper handle was REPAIRED to %q; §33 enforces the bound at the author "+
			"and forbids a reader re-doing it", one.Keeper)
	}
	if one.WorldName != "<script>alert(1)</script>" {
		t.Fatalf("the world name was altered on the way in (%q); escaping is the renderer's "+
			"job and not the record's", one.WorldName)
	}
}

// TestTheRecognitionStoreSurvivesARestart. The whole point of a sidecar: an
// archive that forgot who was on the map every time it restarted would tell six
// participants they all joined a minute ago.
func TestTheRecognitionStoreSurvivesARestart(t *testing.T) {
	dir := t.TempDir()
	r := openRecog(t, dir)

	// Nothing observed, nothing written: a settled map does not rewrite a file
	// every thirty seconds for the pleasure of it.
	if err := r.save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, recognitionStateName)); !os.IsNotExist(err) {
		t.Fatalf("an untouched store wrote %s anyway (%v)", recognitionStateName, err)
	}

	r.observe(recogStatus(
		recogSlot(1, "peer-one", keptBy("ada", "Tidepool", f64(864_000))),
		recogSlot(2, "peer-two", keptBy("", "", nil))), 1_000)
	if err := r.save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	back := openRecog(t, dir)
	one, ok := back.lookup("peer-one")
	if !ok {
		t.Fatal("the restarted archive remembers nothing about a peer it had seen")
	}
	if one.Slot != 1 || one.FirstSeenMs != 1_000 || one.Keeper != "ada" ||
		one.WorldName != "Tidepool" || one.MaxSimulatedTime != 864_000 || !one.HaveSimulatedTime {
		t.Fatalf("the reloaded record = %+v, want every field back", one)
	}
	// An unnamed peer round-trips as unnamed, and as SEEN: the absence of a name
	// is not the absence of a record.
	two, ok := back.lookup("peer-two")
	if !ok || two.Keeper != "" || two.FirstSeenMs != 1_000 {
		t.Fatalf("the unnamed peer's record = %+v (found %v)", two, ok)
	}
	if len(back.entries()) != 2 {
		t.Fatalf("the reloaded store holds %d entries, want 2", len(back.entries()))
	}

	// The next process goes on from there rather than starting over.
	back.observe(recogStatus(recogSlot(1, "peer-one", keptBy("ada", "Tidepool", f64(900_000)))),
		2_000)
	if err := back.save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	again := openRecog(t, dir)
	if one, _ = again.lookup("peer-one"); one.MaxSimulatedTime != 900_000 || one.FirstSeenMs != 1_000 {
		t.Fatalf("the twice-reloaded record = %+v", one)
	}
}

// TestTheRecognitionStoreIsBounded. Every retained structure in this package has
// a cap and a rule for what happens past it (speciesAggMax, rollupPeerMax,
// brainCacheMax), and this one is keyed on a string that arrives from outside:
// one entry per peerId, created by whatever the relay puts in a slot, and
// nothing ever removed. The growth is slow — slots are granted, not claimed —
// but "it grows slowly" is a promise about somebody else's behaviour, and the
// bound is what makes it the archive's own.
//
// PAST THE CAP THE LEAST RECENTLY SEEN GO, and the peers still on the map keep
// everything: their arrival, their names and their high-water marks.
func TestTheRecognitionStoreIsBounded(t *testing.T) {
	r := openRecog(t, t.TempDir())

	// One frame per peer, each a millisecond later than the last, so the store
	// fills exactly to the cap in a known order of last sighting.
	for i := 0; i < recognitionPeerMax; i++ {
		id := fmt.Sprintf("peer-%05d", i)
		r.observe(recogStatus(recogSlot(i+1, id, keptBy("ada", "Tidepool", f64(float64(i))))),
			int64(1_000+i))
	}
	if r.Len() != recognitionPeerMax {
		t.Fatalf("the store holds %d entries, want the cap of %d", r.Len(), recognitionPeerMax)
	}
	if _, ok := r.lookup("peer-00000"); !ok {
		t.Fatal("the store dropped an entry before it reached its bound")
	}

	// The oldest peer is still on the map: it is seen again, which is what makes
	// eviction about SILENCE rather than about age. Two newcomers then push the
	// store past the cap.
	r.observe(recogStatus(recogSlot(1, "peer-00000", keptBy("ada", "Tidepool", f64(9_000)))),
		9_000_000)
	r.observe(recogStatus(
		recogSlot(recognitionPeerMax+1, "peer-new-one", keptBy("bo", "Saltmarsh", f64(1))),
		recogSlot(recognitionPeerMax+2, "peer-new-two", keptBy("cy", "Rockpool", f64(1)))),
		9_000_001)

	if r.Len() != recognitionPeerMax {
		t.Fatalf("the store grew to %d entries past its bound of %d", r.Len(), recognitionPeerMax)
	}
	for _, id := range []string{"peer-new-one", "peer-new-two"} {
		if _, ok := r.lookup(id); !ok {
			t.Fatalf("%s was evicted the moment it arrived", id)
		}
	}
	// The two the archive has heard from least recently, and only those two.
	for _, id := range []string{"peer-00001", "peer-00002"} {
		if _, ok := r.lookup(id); ok {
			t.Fatalf("%s survived; the entries past the cap are the least recently seen", id)
		}
	}
	kept, ok := r.lookup("peer-00000")
	if !ok {
		t.Fatal("the oldest ARRIVAL was evicted; the bound is about the last sighting, and " +
			"this peer was seen a moment ago")
	}
	if kept.FirstSeenMs != 1_000 || kept.Keeper != "ada" || kept.MaxSimulatedTime != 9_000 {
		t.Fatalf("a surviving entry was damaged by the eviction: %+v", kept)
	}
	if _, ok := r.lookup("peer-00003"); !ok {
		t.Fatal("the eviction took more than it had to")
	}

	// AND THE FILE IS TRIMMED TOO. A store that evicted in memory and reloaded
	// the evicted entries on the next start would not be bounded at all.
	if err := r.save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	back := openRecog(t, filepath.Dir(r.Path()))
	if back.Len() != recognitionPeerMax {
		t.Fatalf("the reloaded store holds %d entries, want %d", back.Len(), recognitionPeerMax)
	}
	if _, ok := back.lookup("peer-00001"); ok {
		t.Fatal("an evicted peer came back from the file")
	}
}

// TestAnUnusableRecognitionSidecarIsMovedAsideAndTheMapRunsOn. The loss rule
// every sidecar in this package shares: a file this build cannot use is REPORTED
// and LEFT ALONE — never truncated, never deleted — and the archive still runs,
// because a lost memory of who joined is not a reason to take the map down.
func TestAnUnusableRecognitionSidecarIsMovedAsideAndTheMapRunsOn(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, recognitionStateName)
	write(t, path, "{ this is not json")

	r, err := openRecognitionStore(dir)
	if err == nil {
		t.Fatal("a torn sidecar was read as an empty one; the loss has to be said out loud")
	}
	if r == nil {
		t.Fatal("a torn sidecar stopped the archive from having a store at all")
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatal("the unreadable file is still in place; it must be moved aside so the next " +
			"start writes a fresh one and the bytes survive for anybody who wants to look")
	}
	found := false
	names, _ := os.ReadDir(dir)
	for _, n := range names {
		if strings.HasPrefix(n.Name(), recognitionStateName+".unreadable.") {
			found = true
		}
	}
	if !found {
		t.Fatal("the unreadable sidecar was DELETED rather than moved aside")
	}

	// And the store works from now.
	r.observe(recogStatus(recogSlot(1, "peer-one", keptBy("ada", "Tidepool", nil))), 1_000)
	if e, ok := r.lookup("peer-one"); !ok || e.Keeper != "ada" {
		t.Fatalf("the store did not record after the loss: %+v", e)
	}
}

// TestTheArchiveWritesItsRecognitionStoreOnTheWayOut. The intake fold and the
// shutdown flush are the two ends of the same promise, and Close is what makes
// an orderly restart lose nobody's arrival time.
func TestTheArchiveWritesItsRecognitionStoreOnTheWayOut(t *testing.T) {
	dir := t.TempDir()
	a, err := New(Config{DataDir: dir, PeerID: "archive-test", RelayURL: "ws://test"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	a.recognition.observe(recogStatus(
		recogSlot(1, "peer-one", keptBy("ada", "Tidepool", f64(3_600)))), time.Now().UnixMilli())
	if err := a.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, recognitionStateName)); err != nil {
		t.Fatalf("Close left no %s behind: %v", recognitionStateName, err)
	}

	b, err := New(Config{DataDir: dir, PeerID: "archive-test", RelayURL: "ws://test"})
	if err != nil {
		t.Fatalf("New after restart: %v", err)
	}
	t.Cleanup(func() { _ = b.Close() })
	if e, ok := b.recognition.lookup("peer-one"); !ok || e.Keeper != "ada" ||
		e.MaxSimulatedTime != 3_600 {
		t.Fatalf("the restarted archive's record = %+v (found %v)", e, ok)
	}
}
