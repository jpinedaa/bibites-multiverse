package archive

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
