package archive

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"multiverse/internal/bb8"
	"multiverse/internal/contractb"
)

// TestListGapReportCountsOnlyUnresolvedHashes covers contract-b-m3.md §10.
//
// The gap report is the archive's honest statement of what it does not have and
// could still fetch. A lineage entry with NO hash — `gapReason: "parent_gone"`,
// which contract-a.md §5.3 makes the normal shape of every hop after the first —
// is not an unresolved hash: nothing was recorded to resolve and no retry can
// produce one. Counting it as a gap put every ordinary migration in the report.
func TestListGapReportCountsOnlyUnresolvedHashes(t *testing.T) {
	dir := t.TempDir()
	ledger, err := OpenLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	store, err := bb8.OpenStore(dir + "/genomes")
	if err != nil {
		t.Fatal(err)
	}

	const held = "bb8-genome/1:sha256:1111111111111111111111111111111111111111111111111111111111111111"
	const absent = "bb8-genome/1:sha256:2222222222222222222222222222222222222222222222222222222222222222"
	if err := store.Put(held, "0.6.3.1", `{"version":"0.6.3.1"}`); err != nil {
		t.Fatal(err)
	}

	// 1. A hop whose migrant genome is held and whose only parent is a
	//    parent_gone gap. This is the ordinary second hop of a ring circuit and
	//    it must NOT appear in the gap report.
	if err := ledger.Append(Record{
		Type: RecordMigration, MigrationID: "m-ordinary", EntityID: 7,
		Lineage: &contractb.Lineage{GenomeHash: held, Parents: []contractb.Parent{
			{EntityID: 9, GapReason: contractb.GapParentGone}}},
	}); err != nil {
		t.Fatal(err)
	}
	// 2. A hop naming a parent hash the archive does not hold. This IS the gap.
	if err := ledger.Append(Record{
		Type: RecordMigration, MigrationID: "m-unresolved", EntityID: 8,
		Lineage: &contractb.Lineage{GenomeHash: held, Parents: []contractb.Parent{
			{EntityID: 9, GenomeHash: absent}}},
	}); err != nil {
		t.Fatal(err)
	}
	// 3. An unhashable migrant (contract-b-m3.md §6.6): the hash is the empty
	//    string, it is counted separately, and it is never a fetchable gap.
	if err := ledger.Append(Record{
		Type: RecordMigration, MigrationID: "m-unhashable", EntityID: 10,
		Lineage: &contractb.Lineage{GenomeHash: "", Parents: []contractb.Parent{}},
	}); err != nil {
		t.Fatal(err)
	}
	_ = ledger.Close()

	var out, errOut bytes.Buffer
	if code := Main([]string{"list", "--data-dir", dir}, &out, &errOut); code != 0 {
		t.Fatalf("list exited %d: %s", code, errOut.String())
	}
	summary := out.String()
	if !strings.Contains(summary, "3 migration(s) shown, 1 with an unresolved genome hash, 1 whose migrant genome would not hash") {
		t.Fatalf("summary line is wrong:\n%s", summary)
	}

	out.Reset()
	if code := Main([]string{"list", "--data-dir", dir, "--gaps"}, &out, &errOut); code != 0 {
		t.Fatalf("list --gaps exited %d: %s", code, errOut.String())
	}
	gapsOnly := out.String()
	if !strings.Contains(gapsOnly, "m-unresolved") {
		t.Fatalf("--gaps dropped the one migration with an unresolved hash:\n%s", gapsOnly)
	}
	if strings.Contains(gapsOnly, "m-ordinary") {
		t.Fatalf("--gaps listed a parent_gone entry as a gap:\n%s", gapsOnly)
	}
}

// TestListReadsTheSegmentedLedgerAndSaysWhatItCovers is phase 3's read path,
// and it has two halves.
//
// THE FIRST IS EQUALITY. `archive list` reads the RAW LINES, and after phase 2
// the raw lines are a run of files rather than one. A listing over the segmented
// directory must be the listing over the monolith it was made from, record for
// record — anything else is the 2026-08-08 loss wearing a friendlier face.
//
// THE SECOND IS THE CAPTION. A listing that began "here is every crossing" was
// true while nothing evicted and is false the first day a segment retires, so
// the command states the window it covers before it lists anything.
func TestListReadsTheSegmentedLedgerAndSaysWhatItCovers(t *testing.T) {
	day := int64(24 * 60 * 60 * 1000)
	base := (time.Now().UnixMilli()/day)*day - 3*day
	rec := func(i int) Record {
		return Record{
			Type: RecordMigration, RecordedAt: base + int64(i)*(day/10),
			MigrationID: fmt.Sprintf("m-%02d", i), EntityID: int32(i),
			SourceSlot: 1, DestSlot: 2, SourcePeer: "peer-a",
			Lineage: &contractb.Lineage{GenomeHash: fmt.Sprintf("bb8-%02d", i)},
		}
	}

	// THE MONOLITH, written by hand rather than through OpenLedger, because
	// OpenLedger is what segments a directory: this is the shape every archive
	// before phase 2 had, and it is the thing the segmented listing has to equal.
	mono := t.TempDir()
	var raw bytes.Buffer
	for i := 0; i < 40; i++ {
		b, err := json.Marshal(rec(i))
		if err != nil {
			t.Fatal(err)
		}
		raw.Write(append(b, '\n'))
	}
	if err := os.WriteFile(filepath.Join(mono, ledgerName), raw.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	var before, errOut bytes.Buffer
	if code := Main([]string{"list", "--data-dir", mono}, &before, &errOut); code != 0 {
		t.Fatalf("list exited %d: %s", code, errOut.String())
	}
	if strings.Contains(before.String(), "raw window on this host") {
		t.Error("a ledger that has never been segmented captions a window it does not have")
	}

	// THE SAME BYTES, SEGMENTED. Opening renames the whole file into one legacy
	// segment; the appends that follow rotate on the day boundary, so the
	// listing then spans a legacy segment, a closed day segment and the live
	// file.
	seg := t.TempDir()
	copyFile(t, filepath.Join(mono, ledgerName), filepath.Join(seg, ledgerName))
	l, err := OpenLedger(seg)
	if err != nil {
		t.Fatal(err)
	}
	for i := 40; i < 65; i++ {
		if err := l.Append(rec(i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	segs, err := LedgerSegments(seg)
	if err != nil || len(segs) < 2 {
		t.Fatalf("the fixture produced %d segments (%v), want at least 2", len(segs), err)
	}
	// And compress them, because a listing has to read a gz exactly as it reads
	// a plain file.
	for _, s := range segs {
		if s.Compressed {
			continue
		}
		if _, _, err := compressSegment(seg, s.Name, gzip.BestSpeed); err != nil {
			t.Fatalf("compress %s: %v", s.Name, err)
		}
	}

	var after bytes.Buffer
	if code := Main([]string{"list", "--data-dir", seg}, &after, &errOut); code != 0 {
		t.Fatalf("list over segments exited %d: %s", code, errOut.String())
	}
	if !strings.Contains(after.String(), "raw window on this host") {
		t.Errorf("a segmented listing does not say what it covers:\n%s", after.String())
	}
	if !strings.Contains(after.String(), "RAW LINES ONLY") {
		t.Errorf("a segmented listing does not say the aggregates outlive it:\n%s", after.String())
	}
	if !strings.Contains(after.String(), "65 migration(s) shown") {
		t.Errorf("the segmented listing does not show all 65 records:\n%s", after.String())
	}
	// EVERY LINE THE MONOLITHIC LISTING HELD IS IN THE SEGMENTED ONE.
	for _, want := range strings.Split(strings.TrimSpace(before.String()), "\n") {
		if want == "" || strings.Contains(want, "migration(s) shown") {
			continue
		}
		if !strings.Contains(after.String(), want) {
			t.Fatalf("the segmented listing lost a line the monolithic one had:\n%q", want)
		}
	}
}
