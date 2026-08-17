package archive

// Decision 3's retention horizon, answered by the owner on 2026-08-12 and
// written into contract-b-m4.md §23, B33 and B34.
//
// Five properties, and the fifth is the one the rest of this suite proves for
// free: WITH NO HORIZON SET, NOTHING HERE HAPPENS. Every other test file in
// this package builds an archive without a horizon and is unchanged by this
// arc, which is what "off by default" has to mean to be worth writing down.

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"multiverse/internal/bb8"
	"multiverse/internal/contractb"
)

func horizonArchive(t *testing.T, dir string, horizon time.Duration) *Archive {
	t.Helper()
	a, err := New(Config{
		DataDir: dir, PeerID: "archive-test", RelayURL: "ws://test",
		GenomeHorizon: horizon,
		Logger: slog.New(slog.NewTextHandler(io.Discard,
			&slog.HandlerOptions{Level: slog.LevelError})),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })
	return a
}

// evictEverything cycles the bounded pass until the cursor wraps, which is what
// a running archive does over half an hour of ticks.
func evictEverything(t *testing.T, a *Archive) {
	t.Helper()
	for i := 0; i < bb8.EvictShards+2; i++ {
		if a.evictPass(time.Now()).Wrapped {
			return
		}
	}
	t.Fatal("the eviction cursor never wrapped")
}

func ageBlob(t *testing.T, a *Archive, hash string, d time.Duration) {
	t.Helper()
	h := bb8.HashHex(hash)
	if h == "" {
		t.Fatalf("%q is not a bb8-genome/1 hash", hash)
	}
	p := a.genomes.Dir() + "/" + h[:2] + "/" + h + ".json"
	when := time.Now().Add(-d)
	if err := os.Chtimes(p, when, when); err != nil {
		t.Fatal(err)
	}
}

func hashN(n byte) string {
	return "bb8-genome/1:sha256:" + strings.Repeat(string("0123456789abcdef"[n%16]), 64)
}

// TestEvictionPrunesTheBlobAndNeverTheLedger is the whole decision in one test:
// the genome store loses the bytes, the record keeps everything.
func TestEvictionPrunesTheBlobAndNeverTheLedger(t *testing.T) {
	dir := t.TempDir()
	const horizon = 30 * 24 * time.Hour
	a := horizonArchive(t, dir, horizon)

	old, fresh := hashN(1), hashN(2)
	for _, h := range []string{old, fresh} {
		if err := a.genomes.Put(h, "0.6.3.1", `{"version":"0.6.3.1"}`); err != nil {
			t.Fatal(err)
		}
	}
	// Two migrations and the GENOME line that says who served the old blob. All
	// three are the record, and all three must survive the pruning of the bytes.
	recs := []Record{
		{Type: RecordMigration, MigrationID: "m-old", EntityID: 1,
			RecordedAt: time.Now().Add(-40 * 24 * time.Hour).UnixMilli(),
			Lineage:    &contractb.Lineage{GenomeHash: old}},
		{Type: RecordMigration, MigrationID: "m-fresh", EntityID: 2,
			RecordedAt: time.Now().UnixMilli(),
			Lineage:    &contractb.Lineage{GenomeHash: fresh}},
		{Type: RecordGenome, GenomeHash: old, ServedBy: "peer-lan-slot4",
			RecordedAt: time.Now().Add(-40 * 24 * time.Hour).UnixMilli()},
	}
	for _, r := range recs {
		if err := a.ledger.Append(r); err != nil {
			t.Fatal(err)
		}
	}
	ageBlob(t, a, old, 40*24*time.Hour)

	before, err := os.Stat(a.ledger.Path())
	if err != nil {
		t.Fatal(err)
	}
	evictEverything(t, a)

	if a.genomes.Has(old) {
		t.Fatal("a blob past the horizon survived the pass")
	}
	if !a.genomes.Has(fresh) {
		t.Fatal("a blob inside the horizon was pruned")
	}
	evicted, bytes, _ := a.EvictionCounters()
	if evicted != 1 || bytes <= 0 {
		t.Fatalf("accounting is wrong: evicted %d, bytes %d, want 1 and >0", evicted, bytes)
	}

	// THE LEDGER IS UNTOUCHED. Same length, same records, same order — including
	// the GENOME line naming a blob that no longer exists, which is the archive
	// still knowing what it once held and who served it.
	after, err := os.Stat(a.ledger.Path())
	if err != nil {
		t.Fatal(err)
	}
	if after.Size() != before.Size() {
		t.Fatalf("the ledger changed size across an eviction: %d -> %d", before.Size(), after.Size())
	}
	got, dmg, err := ReadLedger(dir)
	if err != nil || dmg.Any() {
		t.Fatalf("replay after eviction: err %v damage %+v", err, dmg)
	}
	if len(got) != len(recs) {
		t.Fatalf("the ledger holds %d records after an eviction, want %d", len(got), len(recs))
	}
	for i, r := range recs {
		if got[i].MigrationID != r.MigrationID || got[i].Type != r.Type ||
			got[i].GenomeHash != r.GenomeHash {
			t.Fatalf("record %d changed: %+v want %+v", i, got[i], r)
		}
	}
}

// TestAPrunedHashReadsExactlyLikeAHashNobodyServed is §6.10's answer at the
// archive's own surface. The read path joins the ledger with the store, and a
// pruned hash and a hash no peer ever served produce the SAME line — [MISSING],
// and one entry in the gap report. That identity is why B33 changes no wire
// field: a holder that does not hold answers "unknown_hash" either way.
func TestAPrunedHashReadsExactlyLikeAHashNobodyServed(t *testing.T) {
	dir := t.TempDir()
	a := horizonArchive(t, dir, 30*24*time.Hour)

	pruned, neverHeld := hashN(3), hashN(4)
	if err := a.genomes.Put(pruned, "0.6.3.1", `{"version":"0.6.3.1"}`); err != nil {
		t.Fatal(err)
	}
	for i, h := range []string{pruned, neverHeld} {
		if err := a.ledger.Append(Record{
			Type: RecordMigration, MigrationID: []string{"m-pruned", "m-never"}[i],
			EntityID: int32(i), RecordedAt: time.Now().UnixMilli(),
			Lineage: &contractb.Lineage{GenomeHash: h},
		}); err != nil {
			t.Fatal(err)
		}
	}
	ageBlob(t, a, pruned, 90*24*time.Hour)
	evictEverything(t, a)

	migrations, _, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) != 2 {
		t.Fatalf("the record lost a migration to an eviction: %d of 2", len(migrations))
	}
	for _, m := range migrations {
		if m.GenomeHeld {
			t.Fatalf("%s still reports its genome as held", m.MigrationID)
		}
	}
	// And the same answer through the operator's own command.
	var out, errOut bytes.Buffer
	if code := Main([]string{"list", "--data-dir", dir}, &out, &errOut); code != 0 {
		t.Fatalf("list exited %d: %s", code, errOut.String())
	}
	if n := strings.Count(out.String(), "[MISSING]"); n != 2 {
		t.Fatalf("want both hashes reported MISSING, got %d:\n%s", n, out.String())
	}
	if !strings.Contains(out.String(), "2 with an unresolved genome hash") {
		t.Fatalf("the gap report does not count the pruned hash:\n%s", out.String())
	}
}

// TestTheFetchQueueRetiresAGapPastTheHorizon is B34: the drain the queue never
// had. A gap whose CROSSING is older than the horizon stops being retried,
// because the only blob it could win is one the next pass would delete.
//
// This is the AGES-OUT-WHILE-RUNNING path — both gaps are queued legitimately
// and one of them crosses the horizon later, which is what a long-lived archive
// actually does. The queue's other entrance, the startup replay, is covered by
// TestAReplayDoesNotRequeueACrossingPastTheHorizon.
func TestTheFetchQueueRetiresAGapPastTheHorizon(t *testing.T) {
	a := horizonArchive(t, t.TempDir(), 24*time.Hour)
	now := time.Now()
	aged, current := hashN(5), hashN(6)

	a.mu.Lock()
	// 20 hours old at seed time: inside the horizon, so both are queued.
	a.trackLocked(lineageHash{hash: aged, own: true}, "", "", "m-aged", 1, now.Add(-20*time.Hour), now)
	a.trackLocked(lineageHash{hash: current, own: true}, "", "", "m-current", 2, now, now)
	a.ready = true
	a.mu.Unlock()
	if got := a.PendingGaps(); got != 2 {
		t.Fatalf("seeded %d gaps, want 2", got)
	}

	// Five hours later the first crossing is 25 hours old and the second is 5.
	a.pumpFetches(now.Add(5 * time.Hour))

	if got := a.PendingGaps(); got != 1 {
		t.Fatalf("%d gaps after the pass, want 1 (the aged one retired)", got)
	}
	a.mu.Lock()
	_, agedStillQueued := a.pending[aged]
	_, currentStillQueued := a.pending[current]
	a.mu.Unlock()
	if agedStillQueued {
		t.Fatal("a gap past the horizon is still being retried")
	}
	if !currentStillQueued {
		t.Fatal("a gap inside the horizon was retired")
	}
	if _, _, expired := a.EvictionCounters(); expired != 1 {
		t.Fatalf("genomeGapsExpired is %d, want 1", expired)
	}
	if v := a.StatusView(); v.GapsExpired != 1 || v.Gaps != 1 {
		t.Fatalf("status says gaps %d expired %d, want 1 and 1", v.Gaps, v.GapsExpired)
	}
}

// TestAReplayDoesNotRequeueACrossingPastTheHorizon is the same rule at the one
// place it could quietly undo itself. The startup replay hands every hash in
// the ledger to trackLocked, so without this an archive would rebuild its whole
// retired backlog on every restart — and pay for it in resident memory as well
// as in work.
func TestAReplayDoesNotRequeueACrossingPastTheHorizon(t *testing.T) {
	// TWO LEDGERS WITH THE SAME RECORDS, and they have to be two since the roll-up
	// state made the QUEUE ITSELF DURABLE: a second archive on the first one's
	// data directory reads the first one's retirement decision out of the sidecar
	// rather than re-deciding it from raw. That is the point of persisting the
	// queue and it is asserted on its own below; here the horizon rule is what is
	// under test, so each setting gets a directory nobody else has folded.
	seedGaps := func(dir string) {
		seed := horizonArchive(t, dir, 24*time.Hour)
		for i, h := range []string{hashN(7), hashN(8)} {
			at := time.Now().Add(-72 * time.Hour)
			if i == 1 {
				at = time.Now()
			}
			if err := seed.ledger.Append(Record{
				Type: RecordMigration, MigrationID: []string{"m-stale", "m-recent"}[i],
				EntityID: int32(i), RecordedAt: at.UnixMilli(),
				Lineage: &contractb.Lineage{GenomeHash: h},
			}); err != nil {
				t.Fatal(err)
			}
		}
		if err := seed.Close(); err != nil {
			t.Fatal(err)
		}
		// The seed archive wrote a roll-up state of its own from an empty ledger;
		// remove it so the archive under test folds the records itself.
		if err := os.Remove(filepath.Join(dir, rollupSidecarName)); err != nil {
			t.Fatal(err)
		}
	}

	dir := t.TempDir()
	seedGaps(dir)
	withHorizon := horizonArchive(t, dir, 24*time.Hour)
	if got := withHorizon.PendingGaps(); got != 1 {
		t.Fatalf("the replay queued %d gaps, want 1 (the recent crossing only)", got)
	}
	if _, _, expired := withHorizon.EvictionCounters(); expired != 1 {
		t.Fatalf("the replay counted %d retired gaps, want 1", expired)
	}

	// The same records, no horizon: M4's behaviour, both hashes queued forever.
	plain := t.TempDir()
	seedGaps(plain)
	noHorizon := horizonArchive(t, plain, 0)
	if got := noHorizon.PendingGaps(); got != 2 {
		t.Fatalf("with no horizon the replay queued %d gaps, want 2", got)
	}
	if _, _, expired := noHorizon.EvictionCounters(); expired != 0 {
		t.Fatalf("an archive with no horizon retired %d gaps", expired)
	}
}

// TestTheGapQueueSurvivesARestartAndIsDrainedOnLoad is phase 3's change to this
// file, and it has three halves.
//
//  1. THE QUEUE IS PERSISTED. A restart resumes the same gaps without re-reading
//     the raw record for them, which is what took the retention horizon out of
//     the restart's raw scan altogether.
//  2. genomeGapsExpired IS ALL-TIME. It used to count what THE REPLAY declined,
//     so it read differently on a restart than on a first start — the only field
//     that differed in the whole phase-1 validation.
//  3. A RESTORED QUEUE IS DRAINED ON LOAD. An archive that was down for longer
//     than the horizon must come back in the state a full replay would have
//     produced, not in the state it was saved in.
//
// The honest consequence, stated because it is a change: RETIREMENT IS NOW
// DURABLE. Turning the horizon off after a gap has been retired does not bring
// it back, because nothing re-reads the crossing that named it. The hash is
// still in the ledger and still in the gap report (§10, "keep the hash
// forever"); what stays stopped is the asking.
func TestTheGapQueueSurvivesARestartAndIsDrainedOnLoad(t *testing.T) {
	dir := t.TempDir()
	fresh, aged := hashN(11), hashN(12)
	seed := horizonArchive(t, dir, 48*time.Hour)
	for i, h := range []string{fresh, aged} {
		at := time.Now().Add(-time.Hour)
		if i == 1 {
			// Inside a 48 h horizon and outside a 6 h one.
			at = time.Now().Add(-12 * time.Hour)
		}
		if err := seed.ledger.Append(Record{
			Type: RecordMigration, MigrationID: []string{"m-fresh", "m-aged"}[i],
			EntityID: int32(i), RecordedAt: at.UnixMilli(),
			Lineage: &contractb.Lineage{GenomeHash: h},
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := seed.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, rollupSidecarName)); err != nil {
		t.Fatal(err)
	}

	first := horizonArchive(t, dir, 48*time.Hour)
	if got := first.PendingGaps(); got != 2 {
		t.Fatalf("the first start queued %d gaps, want 2", got)
	}
	if first.replayRawRecords != 2 {
		t.Fatalf("the first start parsed %d raw records, want the whole ledger's 2",
			first.replayRawRecords)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	// (1) and (2): the same horizon, and the queue comes back out of the file.
	same := horizonArchive(t, dir, 48*time.Hour)
	if got := same.PendingGaps(); got != 2 {
		t.Fatalf("the restart resumed %d gaps, want the persisted 2", got)
	}
	if _, _, expired := same.EvictionCounters(); expired != 0 {
		t.Fatalf("the restart retired %d gaps, want 0", expired)
	}
	if err := same.Close(); err != nil {
		t.Fatal(err)
	}

	// (3): a shorter horizon on the same state drains what has aged out of it,
	// exactly as a full replay would have declined to queue it.
	shorter := horizonArchive(t, dir, 6*time.Hour)
	if got := shorter.PendingGaps(); got != 1 {
		t.Fatalf("a 6 h horizon resumed %d gaps, want 1: the 12 h-old crossing has aged out",
			got)
	}
	if _, _, expired := shorter.EvictionCounters(); expired != 1 {
		t.Fatalf("the load-time drain counted %d retirements, want 1", expired)
	}
	if _, live := shorter.pending[aged]; live {
		t.Error("the aged gap is still queued after a load-time drain")
	}
	if _, live := shorter.pending[fresh]; !live {
		t.Error("the fresh gap was drained")
	}
	if err := shorter.Close(); err != nil {
		t.Fatal(err)
	}

	// The retirement is DURABLE: turning the horizon back up does not un-retire
	// it, and the counter does not count it a second time.
	back := horizonArchive(t, dir, 48*time.Hour)
	if got := back.PendingGaps(); got != 1 {
		t.Fatalf("raising the horizon back re-queued a retired gap: %d queued, want 1", got)
	}
	if _, _, expired := back.EvictionCounters(); expired != 1 {
		t.Fatalf("genomeGapsExpired is %d after a restart, want the persisted all-time 1",
			expired)
	}
}

// TestNoHorizonEvictsNothingAndSaysNothing is the off-by-default proof made
// explicit. The contract's default is unset (§23, B33) and the deployment is
// what turns it on, so an archive nobody configured must behave exactly as it
// did before this arc — including on the status JSON, where every field of it
// is absent rather than zero (§10.1, unknown is a value).
func TestNoHorizonEvictsNothingAndSaysNothing(t *testing.T) {
	a := horizonArchive(t, t.TempDir(), 0)
	ancient := hashN(9)
	if err := a.genomes.Put(ancient, "0.6.3.1", `{"version":"0.6.3.1"}`); err != nil {
		t.Fatal(err)
	}
	ageBlob(t, a, ancient, 3650*24*time.Hour)

	if res := a.evictPass(time.Now()); res.Removed != 0 || res.Examined != 0 {
		t.Fatalf("an archive with no horizon evicted: %+v", res)
	}
	if !a.genomes.Has(ancient) {
		t.Fatal("a ten-year-old blob was pruned by an archive with no horizon")
	}

	now := time.Now()
	a.mu.Lock()
	a.trackLocked(lineageHash{hash: hashN(10), own: true}, "", "", "m-ancient", 1, now.Add(-3650*24*time.Hour), now)
	a.ready = true
	a.mu.Unlock()
	a.pumpFetches(now)
	if got := a.PendingGaps(); got != 1 {
		t.Fatalf("a gap was retired with no horizon set: %d gaps left", got)
	}

	b, err := json.Marshal(a.StatusView())
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"genomeHorizonMs", "genomesEvicted", "genomesEvictedBytes",
		"genomeGapsExpired"} {
		if strings.Contains(string(b), key) {
			t.Fatalf("%s is on the status of an archive with no horizon:\n%s", key, b)
		}
	}
	if !strings.Contains(string(b), `"genomeGaps"`) {
		t.Fatalf("the gap counter itself went missing:\n%s", b)
	}
}

// TestTheHorizonIsOnTheStatusWhenItIsOn is the other half: an operator reading
// a counter of zero has to be able to tell "nothing to prune yet" from "this
// archive prunes nothing".
func TestTheHorizonIsOnTheStatusWhenItIsOn(t *testing.T) {
	a := horizonArchive(t, t.TempDir(), 720*time.Hour)
	v := a.StatusView()
	if v.GenomeHorizonMs != (720 * time.Hour).Milliseconds() {
		t.Fatalf("genomeHorizonMs is %d, want %d", v.GenomeHorizonMs,
			(720 * time.Hour).Milliseconds())
	}
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"genomeHorizonMs"`) {
		t.Fatalf("the horizon is not on the status JSON:\n%s", b)
	}
}
