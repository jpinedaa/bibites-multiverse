package archive

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"testing"
	"time"

	"multiverse/internal/contractb"
	"multiverse/internal/wire"
)

// The §5.1 duplicate set, which is the one retained structure that grows with
// the ledger and therefore the one whose representation decides whether the
// next restart fits in memory. It holds a 128-bit fingerprint of the key rather
// than the key, so every property below is a property the fingerprint has to
// keep: the three record types stay apart, a NACK's code is part of its
// identity, an empty key is never seen, and the set the replay builds is the
// same set the live path then reads.

// TestEachRecordTypeDedupsOnItsOwnKey holds the shape of the key. One migration
// produces a MIGRATION, an ACK and possibly several NACKs, and every one of
// them is a separate fact about the same migrationId — so the type has to be
// part of what is compared, or the ACK would be refused as a copy of the
// migration it acknowledges.
func TestEachRecordTypeDedupsOnItsOwnKey(t *testing.T) {
	a := quietArchive(t)
	const id = "5f3b2a10-0000-4000-8000-00000000abcd"

	for _, tc := range []struct{ typ, key string }{
		{RecordMigration, id},
		{RecordAck, id},
		{RecordNack, dedupKey(RecordNack, id, contractb.NackSlotVacant)},
		{RecordNack, dedupKey(RecordNack, id, contractb.NackPeerOffline)},
	} {
		if a.markSeen(tc.typ, tc.key) {
			t.Fatalf("%s/%s was refused as a duplicate the first time it was recorded",
				tc.typ, tc.key)
		}
		if !a.markSeen(tc.typ, tc.key) {
			t.Fatalf("%s/%s was recorded twice: the §5.1 duplicate rule did not hold",
				tc.typ, tc.key)
		}
	}

	a.mu.Lock()
	n := a.seen.len()
	a.mu.Unlock()
	if n != 4 {
		t.Fatalf("the dedup set holds %d keys, want the 4 distinct ones recorded: two "+
			"NACK codes on one migrationId are two refusals and not one (§14, B7)", n)
	}
}

func TestAttemptScopedQueueRefusalsRemainDistinctAcrossReplay(t *testing.T) {
	dir := t.TempDir()
	ledger, err := OpenLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	const id = "f7fe2a70-3333-4000-8000-00000000b451"
	first := &contractb.MigrationAttempt{DestSlot: 2, RerouteCount: 0}
	second := &contractb.MigrationAttempt{DestSlot: 3, RerouteCount: 1}
	for i, attempt := range []*contractb.MigrationAttempt{first, second} {
		if err := ledger.Append(Record{
			Type: RecordNack, RecordedAt: now + int64(i), MigrationID: id,
			Code: contractb.NackNotForwarded, RefusedAttempt: attempt,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := ledger.Close(); err != nil {
		t.Fatal(err)
	}

	a, err := New(Config{DataDir: dir, PeerID: "archive-test", RelayURL: "ws://test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	for _, attempt := range []*contractb.MigrationAttempt{first, second} {
		key := dedupKey(RecordNack, id, contractb.NackNotForwarded, attempt)
		if !a.markSeen(RecordNack, key) {
			t.Fatalf("replay did not rebuild the key for attempt %+v", attempt)
		}
	}
	third := &contractb.MigrationAttempt{DestSlot: 4, RerouteCount: 2}
	if a.markSeen(RecordNack,
		dedupKey(RecordNack, id, contractb.NackNotForwarded, third)) {
		t.Fatal("a distinct field-present NOT_FORWARDED attempt collapsed after replay")
	}

	legacy := dedupKey(RecordNack, id, contractb.NackNotForwarded)
	if a.markSeen(RecordNack, legacy) {
		t.Fatal("a legacy absent-attempt NACK collided with a 4.2 attempt record")
	}
	if !a.markSeen(RecordNack, legacy) {
		t.Fatal("a repeated legacy absent-attempt NACK was not deduplicated")
	}
}

func TestLiveNackIngestPersistsAttemptScopedIdentity(t *testing.T) {
	a := quietArchive(t)
	const id = "bf8cda70-4444-4000-8000-00000000b452"
	for _, attempt := range []*contractb.MigrationAttempt{
		{DestSlot: 2, RerouteCount: 0},
		{DestSlot: 3, RerouteCount: 1},
	} {
		data, err := json.Marshal(contractb.MigrationNack{
			MigrationID: id, Code: contractb.NackNotForwarded,
			Class: contractb.ClassTransient, RefusedAttempt: attempt,
		})
		if err != nil {
			t.Fatal(err)
		}
		if !a.onNack(wire.Envelope{Data: data}) {
			t.Fatal("archive did not consume the NACK")
		}
	}
	recs, damage, err := ReadLedger(a.cfg.DataDir)
	if err != nil || damage != (LedgerDamage{}) {
		t.Fatalf("read live NACK records: damage=%+v err=%v", damage, err)
	}
	if len(recs) != 2 || recs[0].RefusedAttempt == nil || recs[1].RefusedAttempt == nil ||
		recs[0].RefusedAttempt.DestSlot != 2 || recs[1].RefusedAttempt.DestSlot != 3 {
		t.Fatalf("live archive records = %+v, want both attempt identities", recs)
	}

	// A field on another NACK code is not allowed to manufacture archive facts.
	otherA := &contractb.MigrationAttempt{DestSlot: 8, RerouteCount: 8}
	otherB := &contractb.MigrationAttempt{DestSlot: 9, RerouteCount: 9}
	if dedupKey(RecordNack, id, contractb.NackPeerOffline, otherA) !=
		dedupKey(RecordNack, id, contractb.NackPeerOffline, otherB) {
		t.Fatal("attempt correlation widened the key for a non-NOT_FORWARDED NACK")
	}
}

// TestAnEmptyKeyIsNeverSeen is the rule a GENOME record depends on. It carries
// no migrationId, cannot be deduplicated at all, and answering "duplicate" for
// it would drop every one of them after the first.
func TestAnEmptyKeyIsNeverSeen(t *testing.T) {
	a := quietArchive(t)
	for i := 0; i < 3; i++ {
		if a.markSeen(RecordMigration, "") {
			t.Fatal("an empty duplicate key was reported as already seen")
		}
	}
	a.mu.Lock()
	n := a.seen.len()
	a.mu.Unlock()
	if n != 0 {
		t.Fatalf("an empty key put %d entries in the dedup set; it must put none", n)
	}
}

// TestTheTwoHalvesOfADedupKeyAreNotInterchangeable is what the fingerprint owes
// the string it replaced. The old set hashed the single string typ+"/"+key, so
// the delimiter kept the halves apart; this one hashes the halves, and a reader
// is entitled to know that the separation survived the change.
func TestTheTwoHalvesOfADedupKeyAreNotInterchangeable(t *testing.T) {
	a := quietArchive(t)
	if a.markSeen("MIGRATION", "abc/def") {
		t.Fatal("the first record of a key was called a duplicate")
	}
	if a.markSeen("MIGRATION/abc", "def") {
		t.Fatal("two different (type, key) pairs that concatenate to the same string " +
			"were treated as one record")
	}
}

// TestAKeySeenInTheReplayIsADuplicateOnTheLivePath is the property the whole
// set exists for: a restart must not let a re-forwarded envelope into the file
// a second time. The replay and the live path build the key through the same
// dedupKey, and this test is what stops the two from drifting apart.
func TestAKeySeenInTheReplayIsADuplicateOnTheLivePath(t *testing.T) {
	dir := t.TempDir()
	ledger, err := OpenLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Now().UnixMilli()
	const id = "9a1c77e2-1111-4000-8000-0000000000aa"
	recs := []Record{
		{Type: RecordMigration, RecordedAt: base, MigrationID: id, SourceSlot: 1, DestSlot: 2},
		{Type: RecordAck, RecordedAt: base + 1, MigrationID: id},
		{Type: RecordNack, RecordedAt: base + 2, MigrationID: id, Code: contractb.NackSlotVacant},
		// A GENOME record carries no migrationId and must add no key.
		{Type: RecordGenome, RecordedAt: base + 3, GenomeHash: "bb8-genome/1:sha256:deadbeef"},
	}
	for _, rec := range recs {
		if err := ledger.Append(rec); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	if err := ledger.Close(); err != nil {
		t.Fatal(err)
	}

	a, err := New(Config{DataDir: dir, PeerID: "archive-test", RelayURL: "ws://test"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })

	a.mu.Lock()
	n := a.seen.len()
	a.mu.Unlock()
	if n != 3 {
		t.Fatalf("the replay rebuilt %d duplicate keys from 4 records, want 3: the GENOME "+
			"record carries no migrationId and contributes none", n)
	}

	// Exactly the three live call sites, with exactly the keys they pass.
	if !a.markSeen(RecordMigration, id) {
		t.Fatal("a MIGRATION already in the file was not recognised after the replay")
	}
	if !a.markSeen(RecordAck, id) {
		t.Fatal("an ACK already in the file was not recognised after the replay")
	}
	if !a.markSeen(RecordNack, dedupKey(RecordNack, id, contractb.NackSlotVacant)) {
		t.Fatal("a NACK already in the file was not recognised after the replay")
	}
	// A different refusal of the same migration is a different fact.
	if a.markSeen(RecordNack, dedupKey(RecordNack, id, contractb.NackPeerOffline)) {
		t.Fatal("a second NACK code on a replayed migrationId was refused as a duplicate")
	}
	if a.markSeen(RecordMigration, wire.NewUUID()) {
		t.Fatal("a migrationId the file has never held was called a duplicate")
	}
}

// TestTheDedupSeedsAreDrawnPerProcess is the reason a participant who chooses
// migrationIds cannot search for a colliding pair and hold it against the
// archive: the set is rebuilt from the ledger on every start and never
// persisted, so the fingerprints only have to agree with themselves inside one
// run. Two archives in one test process stand in for two runs.
func TestTheDedupSeedsAreDrawnPerProcess(t *testing.T) {
	a, b := quietArchive(t), quietArchive(t)
	const id = "c0ffee00-2222-4000-8000-00000000beef"
	if a.seen.fingerprint(RecordMigration, id) == b.seen.fingerprint(RecordMigration, id) {
		t.Fatal("two archives fingerprinted one key identically: the seeds are not " +
			"per-process, and an attacker who found one collision would keep it")
	}
	// And inside one archive it is a function, not a coin toss.
	if a.seen.fingerprint(RecordMigration, id) != a.seen.fingerprint(RecordMigration, id) {
		t.Fatal("one archive fingerprinted one key two different ways")
	}
	if a.seen.fingerprint(RecordMigration, id) == a.seen.fingerprint(RecordAck, id) {
		t.Fatal("the record type did not reach the fingerprint")
	}
}

var fingerprintSink dedupFP

// TestFingerprintingAllocatesNothing holds the reason the halves are hashed
// instead of the string they concatenate to. The replay calls this five million
// times on a production ledger; one 48-byte string per call would be a quarter
// of a gigabyte of garbage per restart, which is the cost the old set paid
// permanently and this one must not pay at all.
func TestFingerprintingAllocatesNothing(t *testing.T) {
	a := quietArchive(t)
	got := testing.AllocsPerRun(200, func() {
		fingerprintSink = a.seen.fingerprint(RecordMigration, "1d8f0c44-3333-4000-8000-0000000000ff")
	})
	if got != 0 {
		t.Fatalf("fingerprinting one key allocated %v times, want 0", got)
	}
}

// TestDedupHintIsAnEstimateAndNeverACount documents what the size hint is: the
// ledger's byte length divided by the measured bytes per duplicate key. It is a
// stat, not a pass over the file, and a wrong answer costs growth work or a few
// unused slots rather than correctness.
func TestDedupHintIsAnEstimateAndNeverACount(t *testing.T) {
	if got := dedupHint(0); got != 0 {
		t.Fatalf("dedupHint(0) = %d, want 0 for a ledger that does not exist yet", got)
	}
	if got := dedupHint(-1); got != 0 {
		t.Fatalf("dedupHint(-1) = %d, want 0", got)
	}
	// The production copy of 2026-08-16: 1,836,382,633 bytes over 5,408,123
	// lines, of which the MIGRATION and ACK records carry a migrationId. The
	// estimate has to land within a few per cent of that or it is not worth
	// making.
	const prodBytes = 1836382633
	got, want := dedupHint(prodBytes), 5408123*2.0/2.14
	if ratio := float64(got) / want; ratio < 0.9 || ratio > 1.1 {
		t.Fatalf("dedupHint(%d) = %d against about %.0f keys (ratio %.3f): the estimate "+
			"has drifted from the workload it was derived from", prodBytes, got, want, ratio)
	}
}

// TestTheDedupTableSurvivesEveryDoubling is the open-addressed table's own
// property, held apart from the archive that uses it. A set that only ever
// grows needs no tombstone, which is what lets it run at a load a Go map does
// not; the price is that a rehash has to move every key the shard holds, and a
// key lost in one would be a migration silently recorded twice for the rest of
// the run. This inserts across several doublings of every shard and then asks
// for all of them back.
func TestTheDedupTableSurvivesEveryDoubling(t *testing.T) {
	s := newDedupSet(0)
	const keys = 40_000
	if got := s.slots(); got != dedupMinSlots*dedupShards {
		t.Fatalf("an unhinted set starts with %d slots, want %d", got, dedupMinSlots*dedupShards)
	}
	for i := 0; i < keys; i++ {
		fp := s.fingerprint(RecordMigration, fmt.Sprintf("grow-%d", i))
		if s.add(fp) {
			t.Fatalf("key %d was already in a set it had never been put in", i)
		}
		if !s.add(fp) {
			t.Fatalf("key %d was recorded twice", i)
		}
	}
	if s.len() != keys {
		t.Fatalf("the set holds %d keys after %d distinct inserts", s.len(), keys)
	}
	if s.slots() <= dedupMinSlots*dedupShards {
		t.Fatal("no table grew, so this test measured nothing")
	}
	for i := range s.shards {
		d := &s.shards[i]
		if int64(d.n)*dedupLoadDen > int64(len(d.slots))*dedupLoadNum {
			t.Fatalf("shard %d holds %d keys in %d slots, over the %d/%d load the probe "+
				"count assumes", i, d.n, len(d.slots), dedupLoadNum, dedupLoadDen)
		}
		if d.n == 0 {
			t.Fatalf("shard %d took none of %d keys: the shard index is not spreading them",
				i, keys)
		}
	}
	for i := 0; i < keys; i++ {
		if !s.has(s.fingerprint(RecordMigration, fmt.Sprintf("grow-%d", i))) {
			t.Fatalf("key %d did not survive the rehashes", i)
		}
	}
	for i := keys; i < keys+2000; i++ {
		if s.has(s.fingerprint(RecordMigration, fmt.Sprintf("grow-%d", i))) {
			t.Fatalf("key %d was in the set without ever being added", i)
		}
	}
}

// TestTheEmptyFingerprintIsAKeyLikeAnyOther covers the one value the tables
// cannot store in a slot, because that value is what an empty slot looks like.
// A key hashing to it is a 2^-128 event and the set is exact anyway: it is held
// in a flag beside the tables rather than assumed away.
func TestTheEmptyFingerprintIsAKeyLikeAnyOther(t *testing.T) {
	s := newDedupSet(0)
	var empty dedupFP
	if s.has(empty) {
		t.Fatal("a fresh set already held the empty fingerprint")
	}
	if s.add(empty) {
		t.Fatal("the first record of the empty fingerprint was called a duplicate")
	}
	if !s.add(empty) {
		t.Fatal("the empty fingerprint was recorded twice")
	}
	if !s.has(empty) {
		t.Fatal("the empty fingerprint was added and then not found")
	}
	if s.len() != 1 {
		t.Fatalf("the set holds %d keys, want the 1 that was added", s.len())
	}
	// And it does not shadow an ordinary key.
	other := s.fingerprint(RecordMigration, "an-ordinary-key")
	if s.add(other) {
		t.Fatal("an ordinary key was shadowed by the empty fingerprint")
	}
	if s.len() != 2 {
		t.Fatalf("the set holds %d keys, want 2", s.len())
	}
}

// TestTheLedgerReportsItsOwnSize covers Ledger.Size, which exists for the dedup
// hint and for nothing else: a stat where the alternative is a pass over the
// file the replay is about to read anyway.
func TestTheLedgerReportsItsOwnSize(t *testing.T) {
	dir := t.TempDir()
	ledger, err := OpenLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := ledger.Size(); got != 0 {
		t.Fatalf("a ledger with no records reports %d bytes", got)
	}
	base := time.Now().UnixMilli()
	for i := 0; i < 3; i++ {
		if err := ledger.Append(Record{Type: RecordMigration, RecordedAt: base + int64(i),
			MigrationID: fmt.Sprintf("size-%d", i)}); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	onDisk, err := os.Stat(ledger.Path())
	if err != nil {
		t.Fatal(err)
	}
	if got := ledger.Size(); got != onDisk.Size() {
		t.Fatalf("the ledger reports %d bytes and the file holds %d", got, onDisk.Size())
	}
	if err := ledger.Close(); err != nil {
		t.Fatal(err)
	}
	// And the size an archive sizes its dedup set from is the size after the
	// open, which is where a torn tail has already been dropped.
	reopened, err := OpenLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if got := reopened.Size(); got != onDisk.Size() {
		t.Fatalf("a reopened ledger reports %d bytes against %d on disk", got, onDisk.Size())
	}
}

// TestASizedSetDoesNotRehashDuringTheReplay is what the size hint is for. A
// rehash runs under the lock the migration path takes, and the replay would
// otherwise run one per shard on the way up. Sized from the ledger's byte
// length, it runs none.
func TestASizedSetDoesNotRehashDuringTheReplay(t *testing.T) {
	// The 2026-08-16 production copy: 1,836,382,633 bytes.
	hint := dedupHint(1836382633)
	s := newDedupSet(hint)
	slots := s.slots()
	if slots != 1<<23 {
		t.Fatalf("a set sized for %d keys holds %d slots, want 2^23", hint, slots)
	}
	// Its real key count was under that estimate — the hint counts records and
	// only the ones carrying a migrationId become keys — and so is any count the
	// estimate is allowed to be wrong about by a few per cent.
	for i := 0; i < hint; i++ {
		s.add(s.fingerprint(RecordMigration, fmt.Sprintf("%08x-5555-4000-8000-%012x", i, i)))
	}
	if got := s.slots(); got != slots {
		t.Fatalf("the set grew from %d to %d slots while taking the keys it was sized for",
			slots, got)
	}
	if s.len() != hint {
		t.Fatalf("the set holds %d of %d distinct keys", s.len(), hint)
	}
}

// TestTheDedupSetDoesNotRetainTheKey is the regression guard on the SHAPE of
// what is retained per record, in the same spirit as
// TestReplayPeakDoesNotGrowWithTheLedger next door. The string form cost about
// 90 bytes of live heap for every key — 46 for the key and its allocation
// header, the rest for a map slot to point at it — and that one field was 88%
// of the archive's resident set on the production host. An edit that quietly
// put the string back would undo the whole change while every other test in
// this file still passed.
//
// THE MEASUREMENT IS TAKEN ON A SIZED SET, which is how the archive builds it:
// a table that has just doubled is half empty and would report twice the cost
// of the same set a moment before. 700,000 keys in a set sized for 786,432 sits
// at a two-thirds load with no shard near its own threshold.
func TestTheDedupSetDoesNotRetainTheKey(t *testing.T) {
	if testing.Short() {
		t.Skip("fills a dedup set with several hundred thousand keys and reads the heap")
	}
	const keys = 700_000
	live := func() uint64 {
		runtime.GC()
		runtime.GC()
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		return ms.HeapAlloc
	}
	before := live()
	s := newDedupSet(786_432)
	for i := 0; i < keys; i++ {
		// Built inside the loop and never held: a caller that kept the strings
		// alive would be measuring its own fixture.
		fp := s.fingerprint(RecordMigration, fmt.Sprintf("%08x-4444-4000-8000-%012x", i, i))
		if s.add(fp) {
			t.Fatalf("key %d was called a duplicate the first time it was recorded", i)
		}
	}
	after := live()
	runtime.KeepAlive(s)

	if s.len() != keys {
		t.Fatalf("the set holds %d of %d distinct keys: something collided or was dropped",
			s.len(), keys)
	}
	if got := s.slots(); got != 1<<20 {
		t.Fatalf("the set holds %d slots for %d keys, want 2^20: a shard rehashed and this "+
			"measurement is no longer taken at the load it claims", got, keys)
	}
	perKey := (after - before) / keys
	t.Logf("%d dedup keys cost %.1f MiB of live heap, %d B/key",
		keys, float64(after-before)/(1<<20), perKey)
	if perKey > 28 {
		t.Fatalf("the dedup set costs %d B for each key at a two-thirds load, above the 28 B "+
			"a flat 16-byte fingerprint table can need. The key is being retained again.",
			perKey)
	}
}

// TestAFetchAsksNobodyUntilItAsksSomebody is the lazy `asked` map. A restart
// discovers every missing hash in the ledger in one pass and the pump asks a
// bounded few per tick, so most entries in a fresh pending set have never been
// asked anything and must not each carry an allocated set to say so.
func TestAFetchAsksNobodyUntilItAsksSomebody(t *testing.T) {
	a := quietArchive(t)
	now := time.Now()
	hashes := seedGaps(a, 4, "slot-1", now)

	// The read path, on a fetch that has never been asked: nil is "nobody", and
	// the source peer is still the first choice (§6.5).
	a.mu.Lock()
	f := a.pending[hashes[0]]
	if f.asked != nil {
		t.Fatal("a gap the pump has never reached allocated a set of peers it has asked")
	}
	a.status = contractb.PeerStatus{
		Epoch: 1, Map: contractb.MapShape{Width: 1, Height: 1}, SlotCount: 1,
		Slots: []contractb.SlotInfo{{Slot: 1, PeerID: "slot-1", Live: true}},
	}
	if got := a.nextPeerLocked(f); got != "slot-1" {
		t.Fatalf("nextPeerLocked on a never-asked fetch chose %q, want the source peer", got)
	}
	a.ready = true
	a.conn = drainingConn(t)
	a.mu.Unlock()

	// The write path allocates on the first ask and records who was asked.
	a.pumpFetches(now)
	a.mu.Lock()
	f = a.pending[hashes[0]]
	if f.asked == nil || !f.asked["slot-1"] {
		t.Fatalf("after a pump the fetch's asked set is %v, want slot-1 recorded in it", f.asked)
	}
	a.mu.Unlock()

	// A rate limit is not an answer from that peer, and taking it back off a set
	// that may not exist must not panic.
	data, err := json.Marshal(contractb.GenomeResponse{
		GenomeHash: hashes[1], Found: false, SourcePeer: "slot-1",
		Reason: contractb.GenomeRateLimited, RetryAfterMs: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	a.mu.Lock()
	a.pending[hashes[1]].asked = nil
	a.mu.Unlock()
	a.onGenomeResponse(wire.Envelope{Type: contractb.TypeGenomeResponse, Data: data})

	// And the ladder reset hands the memory back rather than holding an empty
	// map for however long the retry ladder waits.
	a.mu.Lock()
	f = a.pending[hashes[0]]
	f.inFlight, f.inFlightPeer = "", ""
	a.inFlight = map[string]int{}
	a.status.Slots = nil
	f.sourcePeer = ""
	f.nextAt = now
	a.mu.Unlock()
	a.pumpFetches(now)
	a.mu.Lock()
	still := a.pending[hashes[0]].asked
	a.mu.Unlock()
	if still != nil {
		t.Fatalf("the no-peer ladder reset left an empty asked map (%v) behind", still)
	}
}

// TestTheRealLedgerDedups is the proof on the production record itself. It is
// skipped unless MULTIVERSE_REAL_LEDGER_DIR names a WRITABLE directory holding
// a copy of migrations.jsonl — the archive truncates a torn tail of whatever
// file it opens and writes its sidecars beside it, so this must never be
// pointed at a pristine copy.
func TestTheRealLedgerDedups(t *testing.T) {
	dir := os.Getenv("MULTIVERSE_REAL_LEDGER_DIR")
	if dir == "" {
		t.Skip("set MULTIVERSE_REAL_LEDGER_DIR to a writable copy of a real ledger")
	}
	var first, last string
	var migrations, records int
	damage, err := ScanLedger(dir, func(rec Record) {
		records++
		if rec.Type != RecordMigration || rec.MigrationID == "" {
			return
		}
		migrations++
		if first == "" {
			first = rec.MigrationID
		}
		last = rec.MigrationID
	})
	if err != nil {
		t.Fatalf("ScanLedger: %v", err)
	}
	if first == "" || last == "" || first == last {
		t.Fatalf("the ledger at %s holds %d records and %d usable MIGRATION ids", dir,
			records, migrations)
	}
	t.Logf("ledger: records=%d migrations=%d skippedLines=%d tornTail=%d first=%s last=%s",
		records, migrations, damage.Lines, damage.TornTail, first, last)

	// The horizon the deployment sets. Without one, every lineage hash in the
	// file becomes a pending gap and this test measures the gap set instead of
	// the thing it is about.
	a, err := New(Config{DataDir: dir, PeerID: "archive-test", RelayURL: "ws://test",
		GenomeHorizon: 720 * time.Hour})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })

	a.mu.Lock()
	keys, replayed, gaps := a.seen.len(), a.recordCount, len(a.pending)
	a.mu.Unlock()
	t.Logf("replay: records=%d dedupKeys=%d gaps=%d hint=%d slots=%d", replayed, keys, gaps,
		dedupHint(a.ledger.Size()), a.seen.slots())
	if replayed != records {
		t.Fatalf("the replay counted %d records against the scan's %d", replayed, records)
	}

	if !a.markSeen(RecordMigration, first) {
		t.Fatalf("the FIRST migrationId in the file (%s) was not recognised as recorded",
			first)
	}
	if !a.markSeen(RecordMigration, last) {
		t.Fatalf("the LAST migrationId in the file (%s) was not recognised as recorded",
			last)
	}
	if id := wire.NewUUID(); a.markSeen(RecordMigration, id) {
		t.Fatalf("a migrationId no ledger has ever held (%s) was called a duplicate", id)
	}
}

// ---------------------------------------------------------------- the window

// TestTheDedupWindowRefusesInsideItAndForgetsOutsideIt is §25's B38: the set
// stopped being unbounded, and this is exactly what that costs and buys.
//
// It is bounded because the thing it was unbounded FOR is gone. The set could
// never forget because a sender could re-forward a frame a year later and the
// ledger is never rewritten, so a duplicate row would be permanent. B37 removed
// the re-forward: a conforming sender writes a frame once, and the duplicates
// that remain — an old sidecar's retry, a defective peer — arrive in seconds.
//
// The guarantee is a FLOOR and not a deadline: two generations rotating every
// window remember at least one window and at most two. This asserts the floor,
// which is the half a correctness argument can be built on.
func TestTheDedupWindowRefusesInsideItAndForgetsOutsideIt(t *testing.T) {
	const window = time.Hour
	base := time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC)
	w := newDedupWindow(0, window, base)

	fp := w.fingerprint(RecordMigration, "0e5b1a44-0000-4000-8000-000000000001")
	if w.add(fp, base) {
		t.Fatal("the first record of a key was called a duplicate")
	}

	// Anywhere inside the window, the copy is refused. This is the whole of
	// what the archive needs from it during the transition.
	for _, at := range []time.Duration{0, time.Second, window / 2, window - time.Second} {
		if !w.add(fp, base.Add(at)) {
			t.Fatalf("a duplicate %s after the original was admitted; the floor is %s",
				at, window)
		}
	}

	// Two rotations later the key is gone, and a copy is recorded again. That
	// is a duplicate ROW in a permanent ledger, and it is the price B38 states
	// out loud rather than the defect it would have been before B37.
	if w.add(fp, base.Add(3*window)) {
		t.Fatalf("the key survived %s of a %s window; the set is not bounded and the "+
			"archive's memory is still a function of the record", 3*window, window)
	}
	if !w.add(fp, base.Add(3*window+time.Second)) {
		t.Fatal("the re-recorded key was not remembered by the new generation")
	}
}

// TestTheDedupWindowsMemoryIsAFunctionOfTheWindow is the sizing claim
// deploy/SIZING.md now makes, held as a test rather than as a paragraph: keys
// keep arriving forever and the set does not keep growing forever.
//
// The arithmetic it pins is the one an operator can check. At most two
// generations exist, each holds at most one window of keys, and a slot is 16
// bytes — so the set costs at most 2 x (keys per window) x 16 / load bytes,
// whatever the age of the ledger.
func TestTheDedupWindowsMemoryIsAFunctionOfTheWindow(t *testing.T) {
	const window = time.Hour
	const perWindow = 20_000
	base := time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC)
	w := newDedupWindow(perWindow, window, base)

	var peak int
	for gen := 0; gen < 12; gen++ {
		at := base.Add(time.Duration(gen) * window)
		for i := 0; i < perWindow; i++ {
			key := fmt.Sprintf("gen-%d-key-%d", gen, i)
			w.add(w.fingerprint(RecordMigration, key), at)
		}
		if n := w.slots(); n > peak {
			peak = n
		}
	}

	// Twelve windows of keys went in. Never more than two are held.
	if got := w.len(); got > 2*perWindow {
		t.Fatalf("the window holds %d keys after 12 windows of %d; at most two generations "+
			"may exist", got, perWindow)
	}
	if got := w.len(); got < perWindow {
		t.Fatalf("the window holds only %d keys, below one window of %d: it is forgetting "+
			"inside its own floor", got, perWindow)
	}
	// 240,000 distinct keys arrived. A set sized for them would need well over
	// 320,000 slots; this one is sized for two windows.
	if peak > 4*perWindow {
		t.Fatalf("the set peaked at %d slots for %d keys per window; the memory model is "+
			"still the record's size and not the window's", peak, perWindow)
	}
	t.Logf("12 windows x %d keys: peak %d slots (%d KiB), holding %d keys",
		perWindow, peak, peak*16/1024, w.len())
}

// TestARotationKeepsTheSeeds is what makes the two generations one set. A
// generation that hashed a key differently would answer a different question,
// and a rotation would silently forget everything it was meant to keep.
func TestARotationKeepsTheSeeds(t *testing.T) {
	base := time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC)
	w := newDedupWindow(0, time.Hour, base)
	before := w.fingerprint(RecordMigration, "same-key")
	w.tick(base.Add(time.Hour))
	if w.prev == nil {
		t.Fatal("the window did not rotate")
	}
	if after := w.fingerprint(RecordMigration, "same-key"); after != before {
		t.Fatalf("the fingerprint changed across a rotation: %v then %v", before, after)
	}
}

// TestTheReplayOnlyRebuildsTheWindow is the restart half of B38. Before it, a
// restart re-read the whole ledger into the set and paid the whole ledger's
// memory for it; now it inserts only what the window covers, so the cost of a
// restart stops growing with the age of the record.
func TestTheReplayOnlyRebuildsTheWindow(t *testing.T) {
	dir := t.TempDir()
	ledger, err := OpenLedger(dir)
	if err != nil {
		t.Fatalf("OpenLedger: %v", err)
	}
	now := time.Now()
	const old, recent = 400, 40
	for i := 0; i < old; i++ {
		_ = ledger.Append(Record{
			Type: RecordMigration, RecordedAt: now.Add(-30 * 24 * time.Hour).UnixMilli(),
			MigrationID: fmt.Sprintf("old-%d", i)})
	}
	for i := 0; i < recent; i++ {
		_ = ledger.Append(Record{
			Type: RecordMigration, RecordedAt: now.Add(-time.Minute).UnixMilli(),
			MigrationID: fmt.Sprintf("new-%d", i)})
	}
	_ = ledger.Close()

	a, err := New(Config{
		DataDir: dir, PeerID: "archive-test", RelayURL: "ws://test",
		DedupWindow: time.Hour,
		Logger:      slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError})),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })

	if a.recordCount != old+recent {
		t.Fatalf("the replay read %d records, want %d: B38 bounds the SET and never the "+
			"ledger", a.recordCount, old+recent)
	}
	a.mu.Lock()
	n := a.seen.len()
	a.mu.Unlock()
	if n != recent {
		t.Fatalf("the replay rebuilt %d keys, want the %d inside the window: a key whose "+
			"record is older than the window would not have been refused by the live path "+
			"either", n, recent)
	}
	// And the window it did rebuild works: a copy of a recent record is refused.
	if !a.markSeen(RecordMigration, "new-0") {
		t.Fatal("a duplicate of an in-window record was admitted after a restart")
	}
	// A copy of an out-of-window record is recorded again, which is the stated
	// price and not a defect.
	if a.markSeen(RecordMigration, "old-0") {
		t.Fatal("an out-of-window key was still in the set; the replay is not bounded")
	}
}
