package archive

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
	"time"

	"multiverse/internal/contractb"
)

// The ledger's durability tests. They are the archive's half of the rule the
// sidecar journal learned on 2026-08-08 (internal/journal, TestShortWrite...):
// a failed append leaves nothing behind, and a line that cannot be read is read
// past rather than stopped at.

func appendLine(t *testing.T, path, s string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(s); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func mustAppend(t *testing.T, l *Ledger, id string) {
	t.Helper()
	if err := l.Append(Record{Type: RecordMigration, MigrationID: id, EntityID: 1}); err != nil {
		t.Fatalf("append %s: %v", id, err)
	}
}

func ids(recs []Record) []string {
	out := make([]string, 0, len(recs))
	for _, r := range recs {
		out = append(out, r.MigrationID)
	}
	return out
}

// TestUnparsableLineIsSkippedNotStoppedAt is the 2026-08-08 damage in
// miniature, and the defect found in the living archive on 2026-08-09.
//
// A full disk left ONE spliced 776-byte line at line 874163 of the LAN rig's
// migrations.jsonl: the head of a NACK written at 01:21:49 and the tail of a
// record written at 08:12:05. Replay used to break at the first line that would
// not parse, so the 390,005 records behind it were unreachable and the next
// restart would have reverted the archive's totals, lanes and species
// aggregates to 01:21 in silence. The line is still there — the ledger is
// append-only and nothing rewrites it — so replay must read past it forever.
func TestUnparsableLineIsSkippedNotStoppedAt(t *testing.T) {
	dir := t.TempDir()
	ledger, err := OpenLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	mustAppend(t, ledger, "before-1")
	mustAppend(t, ledger, "before-2")
	if err := ledger.Close(); err != nil {
		t.Fatal(err)
	}

	// The exact shape a short write leaves behind: a half record with no newline
	// of its own, and the next successful append spliced onto it.
	path := filepath.Join(dir, ledgerName)
	poison := `{"type":"NACK","migrationId":"torn","cod` +
		`{"type":"MIGRATION","migrationId":"spliced","entityId":9}` + "\n"
	appendLine(t, path, poison)

	reopened, err := OpenLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	// A record written after the damage. This is what the old replay lost.
	mustAppend(t, reopened, "after-1")
	mustAppend(t, reopened, "after-2")
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}

	recs, damage, err := ReadLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"before-1", "before-2", "after-1", "after-2"}
	got := ids(recs)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("replay read %v, want %v — every record behind the damage must survive", got, want)
	}
	if damage.Lines != 1 {
		t.Fatalf("damage.Lines = %d, want 1", damage.Lines)
	}
	// What is reported is the damaged line itself. This is where the ledger
	// DEPARTS from the journal, which reports the history it threw away behind
	// the damage: the ledger throws nothing away, so there is nothing else to
	// count.
	if want := int64(len(poison) - 1); damage.Bytes != want {
		t.Fatalf("damage.Bytes = %d, want %d", damage.Bytes, want)
	}
	if damage.TornTail != 0 {
		t.Fatalf("a newline-terminated poison line was reported as a torn tail (%d bytes)", damage.TornTail)
	}
	if !damage.Any() {
		t.Fatal("damage.Any() is false for a ledger that lost a record")
	}

	// And the replay the archive actually performs at startup: the count it
	// reports is the records it recovered, and the loss is on the operator's
	// screen rather than only in a log line nobody kept.
	a, err := New(Config{DataDir: dir, PeerID: "archive-test", RelayURL: "ws://test"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer a.Close()
	view := a.StatusView()
	if view.Records != len(want) {
		t.Fatalf("StatusView records = %d, want %d", view.Records, len(want))
	}
	if view.LedgerSkipped != 1 {
		t.Fatalf("StatusView ledgerSkippedLines = %d, want 1", view.LedgerSkipped)
	}
}

// TestTornTailIsDroppedAtOpen covers the other half: an unterminated final line
// is a write that never finished, so it was never durable and its caller was
// told the append failed. Open drops it, which is also what stops the NEXT
// append from splicing itself onto those bytes and making the poison line above.
func TestTornTailIsDroppedAtOpen(t *testing.T) {
	dir := t.TempDir()
	ledger, err := OpenLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	mustAppend(t, ledger, "whole")
	if err := ledger.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, ledgerName)
	torn := `{"type":"MIGRATION","migrationId":"tor`
	appendLine(t, path, torn)

	// The read-only path sees it and says so without touching the file.
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	recs, damage, err := ReadLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 || recs[0].MigrationID != "whole" {
		t.Fatalf("replay read %v, want [whole]", ids(recs))
	}
	if damage.TornTail != int64(len(torn)) {
		t.Fatalf("damage.TornTail = %d, want %d", damage.TornTail, len(torn))
	}
	if damage.Any() {
		t.Fatalf("a torn tail was counted as lost history: %+v damage", damage)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("ReadLedger modified the ledger; the read path must never write")
	}

	// Opening for append repairs it, and the record written next lands on a
	// clean boundary rather than splicing onto the torn bytes.
	reopened, err := OpenLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Repaired(); got != int64(len(torn)) {
		t.Fatalf("Repaired() = %d, want %d", got, len(torn))
	}
	mustAppend(t, reopened, "next")
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
	recs, damage, err = ReadLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(ids(recs), ","); got != "whole,next" {
		t.Fatalf("replay read %q, want \"whole,next\"", got)
	}
	if damage != (LedgerDamage{}) {
		t.Fatalf("a repaired ledger still reports damage: %+v", damage)
	}
	if fresh, err := OpenLedger(dir); err != nil {
		t.Fatal(err)
	} else {
		if fresh.Repaired() != 0 {
			t.Fatalf("a healthy ledger reported %d repaired bytes", fresh.Repaired())
		}
		_ = fresh.Close()
	}
}

// TestTornTailThatParsesIsStillDropped: a final line with no newline is a short
// write whether or not the bytes that arrived happen to be valid JSON. The
// record was never durable and its caller never ACKed, so keeping it would
// resurrect a write the rest of the system was told had failed — and the read
// path and the append path must agree about that, or `list` would show a record
// the next restart deletes.
func TestTornTailThatParsesIsStillDropped(t *testing.T) {
	dir := t.TempDir()
	ledger, err := OpenLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	mustAppend(t, ledger, "whole")
	if err := ledger.Close(); err != nil {
		t.Fatal(err)
	}
	line, err := json.Marshal(Record{Type: RecordMigration, MigrationID: "no-newline"})
	if err != nil {
		t.Fatal(err)
	}
	appendLine(t, filepath.Join(dir, ledgerName), string(line)) // no '\n'

	recs, damage, err := ReadLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(ids(recs), ","); got != "whole" {
		t.Fatalf("replay read %q, want \"whole\"", got)
	}
	if damage.TornTail != int64(len(line)) {
		t.Fatalf("damage.TornTail = %d, want %d", damage.TornTail, len(line))
	}
}

// TestLedgerWithNoLineEndingAtAllIsRefused: repair cuts back to the last
// newline, so a file with none of them is either an empty torn line or not this
// archive's ledger. A short one is dropped; one longer than any record could be
// is refused, because guessing where to cut could throw away everything the
// archive has.
func TestLedgerWithNoLineEndingAtAllIsRefused(t *testing.T) {
	dir := t.TempDir()
	appendLine(t, filepath.Join(dir, ledgerName), `{"type":"MIGRATION"`)
	l, err := OpenLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	if l.Repaired() != 19 {
		t.Fatalf("Repaired() = %d, want 19", l.Repaired())
	}
	_ = l.Close()

	big := t.TempDir()
	appendLine(t, filepath.Join(big, ledgerName), strings.Repeat("x", maxLedgerLine+1))
	if _, err := OpenLedger(big); err == nil {
		t.Fatal("a file with no line ending in a record's worth of bytes was opened for append")
	}
}

// TestScanAndReadLedgerAgree pins the two halves of the 2026-08-12 split to each
// other on a ledger that carries BOTH kinds of damage at once.
//
// The replay in New reads through ScanLedger and `Records` reads through
// ReadLedger, so the day they disagree about a record or about the damage is the
// day the archive's own startup and the archive's own `list` describe different
// histories out of one file — which is the silent divergence the 2026-08-08
// splice actually cost, in a new place. The two must stay one implementation:
// ReadLedger is the scan with an append for a callback, and this is what says so
// out loud. The three tests above pin the SEMANTICS; this one pins the identity.
func TestScanAndReadLedgerAgree(t *testing.T) {
	dir := t.TempDir()
	ledger, err := OpenLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	mustAppend(t, ledger, "before-1")
	// A record with the pointer-bearing fields populated, so agreement is tested
	// on a decoded shape and not only on flat strings.
	if err := ledger.Append(Record{
		Type: RecordMigration, RecordedAt: 1_700_000_000_000, MigrationID: "before-2",
		SourcePeer: "peer-a", SourceSlot: 1, DestSlot: 4, EntityID: 77, ExitEdge: "north",
		Lineage: &contractb.Lineage{
			GenomeHash: "sha256:aa",
			Parents:    []contractb.Parent{{EntityID: 5, GenomeHash: "sha256:bb"}},
		},
		Species: &contractb.Species{GenericName: "Nibbles", SpecificName: "primus"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Close(); err != nil {
		t.Fatal(err)
	}

	// The 2026-08-08 shape: a half record with no newline of its own and the next
	// successful append spliced onto it, in the MIDDLE of the file.
	path := filepath.Join(dir, ledgerName)
	poison := `{"type":"NACK","migrationId":"torn","cod` +
		`{"type":"MIGRATION","migrationId":"spliced","entityId":9}` + "\n"
	appendLine(t, path, poison)

	reopened, err := OpenLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	mustAppend(t, reopened, "after-1")
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
	// And the other kind of damage on the same file: an unterminated final line.
	torn := `{"type":"MIGRATION","migrationId":"never-durable"`
	appendLine(t, path, torn)

	var scanned []Record
	scanDamage, err := ScanLedger(dir, func(rec Record) { scanned = append(scanned, rec) })
	if err != nil {
		t.Fatalf("ScanLedger: %v", err)
	}
	read, readDamage, err := ReadLedger(dir)
	if err != nil {
		t.Fatalf("ReadLedger: %v", err)
	}

	if !reflect.DeepEqual(scanned, read) {
		t.Fatalf("the scan and the materialising read disagree about the records:\nscan %+v\nread %+v",
			ids(scanned), ids(read))
	}
	if scanDamage != readDamage {
		t.Fatalf("the scan and the materialising read disagree about the damage: %+v vs %+v",
			scanDamage, readDamage)
	}

	// Spelled out, so a change that breaks both of them identically still fails.
	if got := strings.Join(ids(scanned), ","); got != "before-1,before-2,after-1" {
		t.Fatalf("replay read %q, want \"before-1,before-2,after-1\"", got)
	}
	want := LedgerDamage{Lines: 1, Bytes: int64(len(poison) - 1), TornTail: int64(len(torn))}
	if scanDamage != want {
		t.Fatalf("damage = %+v, want %+v", scanDamage, want)
	}
	if scanned[1].Lineage == nil || scanned[1].Lineage.Parents[0].GenomeHash != "sha256:bb" {
		t.Fatalf("the lineage annex did not survive the scan: %+v", scanned[1].Lineage)
	}
}

// TestReplayPeakDoesNotGrowWithTheLedger is the regression guard on the SHAPE of
// the replay, which is the thing that was actually wrong.
//
// Materialising the ledger into one []Record made the replay's peak the size of
// the file by construction: the reference 8,156,868-record replay measured
// 1,030–1,286 B of peak RSS per record, which is what took an 8 GB box
// from restartable to not restartable around day 26 of a three-month run
// (wp3_hosting_options.md). Streaming measured 184 B/record. No collector
// setting reaches under a live slice, so a future edit that quietly collects the
// records again would undo the whole fix while every other test still passed.
//
// THE BOUNDS ARE DELIBERATELY LOOSE. This asserts a shape, not a number: that
// the scan's heap does not scale with the record count, and that the
// materialising read's does. GOGC is pinned for the duration so the answer does
// not depend on the environment the suite was started in.
func TestReplayPeakDoesNotGrowWithTheLedger(t *testing.T) {
	if testing.Short() {
		t.Skip("writes a multi-megabyte ledger and samples the heap")
	}
	defer debug.SetGCPercent(debug.SetGCPercent(100))

	const records = 100_000
	dir := t.TempDir()
	writeSyntheticLedger(t, dir, records)

	var scanned int
	scanPeak := peakHeapDuring(func() {
		if _, err := ScanLedger(dir, func(Record) { scanned++ }); err != nil {
			t.Errorf("ScanLedger: %v", err)
		}
	})
	if scanned != records {
		t.Fatalf("the scan saw %d records, want %d", scanned, records)
	}
	readPeak := peakHeapDuring(func() {
		recs, _, err := ReadLedger(dir)
		if err != nil {
			t.Errorf("ReadLedger: %v", err)
		}
		if len(recs) != records {
			t.Errorf("the read saw %d records, want %d", len(recs), records)
		}
		runtime.KeepAlive(recs)
	})

	// The measured streamed model, used as a ceiling rather than a target: a bare
	// scan retains nothing per record at all, so 184 B/record is several times
	// the room it can possibly need and still an order of magnitude under what
	// materialising costs.
	const modelPerRecord = 184
	if limit := uint64(records * modelPerRecord); scanPeak > limit {
		t.Fatalf("the streamed replay peaked at %s over %d records (%d B/record) — above the %s "+
			"the measured streaming model allows. The replay is holding records again.",
			mib(scanPeak), records, scanPeak/records, mib(limit))
	}
	// And it is not merely smaller: materialising the same file must cost several
	// times more, or this ledger was too small to have measured anything.
	if scanPeak*4 > readPeak {
		t.Fatalf("streamed peak %s against materialised peak %s over %d records: less than the 4x "+
			"the shape implies, so either the replay materialises again or this measurement is not "+
			"measuring the heap it thinks it is.", mib(scanPeak), mib(readPeak), records)
	}
	t.Logf("streamed %s (%d B/record), materialised %s (%d B/record), %.1fx",
		mib(scanPeak), scanPeak/records, mib(readPeak), readPeak/records,
		float64(readPeak)/float64(scanPeak))
}

func mib(b uint64) string { return fmt.Sprintf("%.1f MiB", float64(b)/(1<<20)) }

// writeSyntheticLedger writes n MIGRATION records with a lineage annex — the
// shape the deployment's ledger is mostly made of — straight through a buffered
// writer. It deliberately does not go through Ledger.Append: one fsync per
// record is the durability rule the live path owes and it is not what this
// fixture is testing.
func writeSyntheticLedger(t *testing.T, dir string, n int) {
	t.Helper()
	f, err := os.Create(filepath.Join(dir, ledgerName))
	if err != nil {
		t.Fatal(err)
	}
	w := bufio.NewWriterSize(f, 1<<20)
	enc := json.NewEncoder(w)
	for i := range n {
		if err := enc.Encode(Record{
			Type:        RecordMigration,
			RecordedAt:  1_700_000_000_000 + int64(i),
			MigrationID: fmt.Sprintf("mig-%08d-0123456789abcdef", i),
			SourcePeer:  fmt.Sprintf("peer-%02d", i%12),
			SourceSlot:  i % 6,
			DestSlot:    (i + 1) % 6,
			EntityID:    int32(i),
			ExitEdge:    "north",
			Lineage: &contractb.Lineage{
				GenomeHash: fmt.Sprintf("sha256:%064x", i),
				Parents:    []contractb.Parent{{EntityID: int32(i), GenomeHash: fmt.Sprintf("sha256:%064x", i+1)}},
			},
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

// peakHeapDuring is the highest HeapAlloc seen while fn ran, over the heap that
// was already live when it started. It samples rather than reading a peak
// counter because the runtime does not publish one; the sample rate only has to
// be fast against the seconds a replay takes, and the value it is compared
// against is a shape, not a budget.
func peakHeapDuring(fn func()) uint64 {
	runtime.GC()
	var base, ms runtime.MemStats
	runtime.ReadMemStats(&base)

	stop := make(chan struct{})
	peaked := make(chan uint64, 1)
	go func() {
		var peak uint64
		tick := time.NewTicker(time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				var final runtime.MemStats
				runtime.ReadMemStats(&final)
				peaked <- max(peak, final.HeapAlloc)
				return
			case <-tick.C:
				runtime.ReadMemStats(&ms)
				peak = max(peak, ms.HeapAlloc)
			}
		}
	}()
	fn()
	close(stop)
	peak := <-peaked
	if peak < base.HeapAlloc {
		return 0
	}
	return peak - base.HeapAlloc
}
