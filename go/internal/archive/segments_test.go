package archive

// The segmented ledger's tests, and they exist to hold three claims:
//
//	A SEGMENTED REPLAY EQUALS A MONOLITHIC ONE over the same records — every
//	record, in order, field for field, with the same damage accounting.
//	archive_rollup_design.md's validation bar names this in as many words.
//
//	A CRASH AT ANY STEP OF A ROTATION OR A COMPRESSION LOSES NOTHING. Each step
//	is enumerated in the comment on the function that performs it, and each one
//	is fault-injected below by leaving the directory in exactly the state that
//	crash would leave it and then reopening.
//
//	NO RECEIPT, NO RETIREMENT. There is no path, no flag and no age at which a
//	segment is removed without a cold-copy receipt that matches the bytes on the
//	disk.

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"multiverse/internal/contractb"
)

// ---------------------------------------------------------------- helpers

func day(s string) time.Time {
	d, err := time.ParseInLocation(dayLayout, s, time.UTC)
	if err != nil {
		panic(err)
	}
	return d
}

// rec builds one migration record on a given day at a given hour.
func rec(id string, at time.Time) Record {
	return Record{
		Type:        RecordMigration,
		RecordedAt:  at.UnixMilli(),
		MigrationID: id,
		SourceSlot:  1,
		DestSlot:    2,
		EntityID:    7,
		ExitEdge:    "east",
		Lineage:     &contractb.Lineage{GenomeHash: "bb8-genome/1:sha256:" + id},
	}
}

func mustAppendAt(t *testing.T, l *Ledger, id string, at time.Time) {
	t.Helper()
	if err := l.Append(rec(id, at)); err != nil {
		t.Fatalf("append %s: %v", id, err)
	}
}

// scanAll replays a data directory and returns the records and the damage.
func scanAll(t *testing.T, dir string) ([]Record, LedgerDamage) {
	t.Helper()
	var out []Record
	dmg, err := ScanLedger(dir, func(r Record) { out = append(out, r) })
	if err != nil {
		t.Fatalf("scan %s: %v", dir, err)
	}
	return out, dmg
}

// segNames is the ordered set of segment names on disk, retired ones marked.
func segNames(t *testing.T, dir string) []string {
	t.Helper()
	segs, err := LedgerSegments(dir)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(segs))
	for _, s := range segs {
		n := s.Name
		if s.Retired {
			n += " [retired]"
		} else if s.Compressed {
			n += " [gz]"
		}
		out = append(out, n)
	}
	return out
}

// sameRecords compares two record runs field for field. It is not
// reflect.DeepEqual on the slices themselves, because a nil slice and an empty
// one are the same answer here and DeepEqual says they are not.
func sameRecords(a, b []Record) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !reflect.DeepEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

func fileSHA(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// ---------------------------------------------------------------- naming

// TestSegmentNamesRoundTripAndOrderIsParsedNotLexical pins the ordering rule.
// "legacy-" sorts AFTER a digit in ASCII, so a lexical sort would replay the
// migration's legacy segment — which holds the oldest records in the system —
// last, and every aggregate built on the replay would be built out of order.
func TestSegmentNamesRoundTripAndOrderIsParsedNotLexical(t *testing.T) {
	legacy := legacySegmentName(day("2026-07-01"), day("2026-08-16"), 0)
	if legacy != "legacy-2026-07-01-to-2026-08-16-0000" {
		t.Fatalf("legacy name is %q", legacy)
	}
	names := []string{
		segmentName(day("2026-08-17"), 0),
		legacy,
		segmentName(day("2026-08-16"), 1),
		segmentName(day("2026-08-16"), 0),
	}
	var segs []Segment
	for _, n := range names {
		s, ok := parseSegmentName(n)
		if !ok {
			t.Fatalf("%q did not parse as a segment", n)
		}
		segs = append(segs, s)
	}
	sortSegments(segs)
	got := []string{segs[0].Name, segs[1].Name, segs[2].Name, segs[3].Name}
	want := []string{legacy, "2026-08-16-0000", "2026-08-16-0001", "2026-08-17-0000"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("segment order is %v, want %v", got, want)
	}
	// A lexical sort would put the legacy segment LAST. Prove the difference is
	// real rather than incidental.
	lex := append([]string{}, names...)
	for i := range lex {
		for j := i + 1; j < len(lex); j++ {
			if lex[j] < lex[i] {
				lex[i], lex[j] = lex[j], lex[i]
			}
		}
	}
	if lex[0] == legacy {
		t.Fatal("a lexical sort happens to agree here, so this test proves nothing")
	}
	for _, bad := range []string{"migrations", "2026-08-16", "2026-13-01-0000", "2026-08-16-00", "coldcopy.jsonl"} {
		if _, ok := parseSegmentName(bad); ok {
			t.Fatalf("%q parsed as a segment and must not", bad)
		}
	}
}

// ---------------------------------------------------------------- rotation

// TestRotationClosesTheDayAndOpensAFreshLiveFile is the rotation rule: the live
// file is closed into a segment named for the day it HELD, not the day that
// opened, and the record that triggered the rotation lands in the fresh file.
func TestRotationClosesTheDayAndOpensAFreshLiveFile(t *testing.T) {
	dir := t.TempDir()
	l, err := OpenLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	d1, d2, d3 := day("2026-08-14"), day("2026-08-15"), day("2026-08-16")
	mustAppendAt(t, l, "a", d1.Add(1*time.Hour))
	mustAppendAt(t, l, "b", d1.Add(23*time.Hour))
	mustAppendAt(t, l, "c", d2.Add(2*time.Hour))
	mustAppendAt(t, l, "d", d3.Add(2*time.Hour))
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}

	if got, want := segNames(t, dir), []string{"2026-08-14-0000", "2026-08-15-0000"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("segments are %v, want %v", got, want)
	}
	// The live file holds only the newest day.
	live, err := os.ReadFile(filepath.Join(dir, ledgerName))
	if err != nil {
		t.Fatal(err)
	}
	if n := bytes.Count(live, []byte("\n")); n != 1 {
		t.Fatalf("the live file holds %d lines, want 1", n)
	}
	// The first day's segment holds exactly the first day's records.
	first, err := os.ReadFile(filepath.Join(segmentsDir(dir), "2026-08-14-0000"+plainSuffix))
	if err != nil {
		t.Fatal(err)
	}
	if n := bytes.Count(first, []byte("\n")); n != 2 {
		t.Fatalf("the first segment holds %d lines, want 2", n)
	}
	recs, dmg := scanAll(t, dir)
	if got := strings.Join(ids(recs), ","); got != "a,b,c,d" {
		t.Fatalf("segmented replay read %q, want a,b,c,d", got)
	}
	if dmg != (LedgerDamage{}) {
		t.Fatalf("a healthy segmented ledger reported damage: %+v", dmg)
	}
}

// TestARecordFromAnEarlierDayDoesNotRotate: a clock that steps backwards must
// not produce an out-of-order segment. The record lands where it is and the
// segment it eventually sits in is named for the LATER day, which only ever
// makes it retire later.
func TestARecordFromAnEarlierDayDoesNotRotate(t *testing.T) {
	dir := t.TempDir()
	l, err := OpenLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	mustAppendAt(t, l, "a", day("2026-08-15").Add(3*time.Hour))
	mustAppendAt(t, l, "back", day("2026-08-14").Add(3*time.Hour))
	mustAppendAt(t, l, "b", day("2026-08-15").Add(5*time.Hour))
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	if got := segNames(t, dir); len(got) != 0 {
		t.Fatalf("a backwards clock step rotated: %v", got)
	}
	recs, _ := scanAll(t, dir)
	if got := strings.Join(ids(recs), ","); got != "a,back,b" {
		t.Fatalf("replay read %q, want a,back,b — order is write order, never clock order", got)
	}
}

// TestARestartContinuesTheSameDaysSegment: rotation is driven by the RECORDS'
// day, and a restart reads that day back off the live file's last record. A
// restart must not close a segment that is still being written.
func TestARestartContinuesTheSameDaysSegment(t *testing.T) {
	dir := t.TempDir()
	l, err := OpenLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	mustAppendAt(t, l, "a", day("2026-08-15").Add(3*time.Hour))
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	l2, err := OpenLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	mustAppendAt(t, l2, "b", day("2026-08-15").Add(9*time.Hour))
	if l2.Rotations() != 0 {
		t.Fatalf("a restart inside the same day rotated %d times", l2.Rotations())
	}
	mustAppendAt(t, l2, "c", day("2026-08-16").Add(1*time.Hour))
	if l2.Rotations() != 1 {
		t.Fatalf("crossing midnight after a restart rotated %d times, want 1", l2.Rotations())
	}
	_ = l2.Close()
	if got, want := segNames(t, dir), []string{"2026-08-15-0000"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("segments are %v, want %v", got, want)
	}
	recs, _ := scanAll(t, dir)
	if got := strings.Join(ids(recs), ","); got != "a,b,c" {
		t.Fatalf("replay read %q, want a,b,c", got)
	}
}

// TestRotationNeverOverwritesAnExistingSegment: the sequence number exists so a
// second rotation for the same day cannot rename over the first one's records.
func TestRotationNeverOverwritesAnExistingSegment(t *testing.T) {
	dir := t.TempDir()
	l, err := OpenLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	mustAppendAt(t, l, "a", day("2026-08-15").Add(3*time.Hour))
	if _, err := l.RotateNow(); err != nil {
		t.Fatal(err)
	}
	mustAppendAt(t, l, "b", day("2026-08-15").Add(4*time.Hour))
	if _, err := l.RotateNow(); err != nil {
		t.Fatal(err)
	}
	_ = l.Close()
	if got, want := segNames(t, dir), []string{"2026-08-15-0000", "2026-08-15-0001"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("segments are %v, want %v", got, want)
	}
	recs, _ := scanAll(t, dir)
	if got := strings.Join(ids(recs), ","); got != "a,b" {
		t.Fatalf("replay read %q, want a,b — a rotation renamed over a segment", got)
	}
}

// ---------------------------------------------------------------- migration

// TestFirstStartRenamesTheWholeLedgerIntoOneLegacySegment is "Migration, in one
// restart", step 3. The claim it holds is NOTHING WAS REWRITTEN: the legacy
// segment must be byte-identical to the file that was there before.
func TestFirstStartRenamesTheWholeLedgerIntoOneLegacySegment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ledgerName)
	for i, at := range []time.Time{
		day("2026-07-01").Add(time.Hour),
		day("2026-07-20").Add(time.Hour),
		day("2026-08-16").Add(time.Hour),
	} {
		b, _ := json.Marshal(rec(string(rune('a'+i)), at))
		appendLine(t, path, string(b)+"\n")
	}
	before := fileSHA(t, path)
	beforeSize, _ := os.Stat(path)

	l, err := OpenLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	want := "legacy-2026-07-01-to-2026-08-16-0000"
	if l.Migrated() != want {
		t.Fatalf("Migrated() = %q, want %q", l.Migrated(), want)
	}
	seg := filepath.Join(segmentsDir(dir), want+plainSuffix)
	if got := fileSHA(t, seg); got != before {
		t.Fatal("the legacy segment is not byte-identical to the ledger it was made from")
	}
	if info, _ := os.Stat(seg); info.Size() != beforeSize.Size() {
		t.Fatal("the legacy segment changed size")
	}
	if info, err := os.Stat(path); err != nil || info.Size() != 0 {
		t.Fatalf("the fresh live file is %v (%v), want an empty file", info, err)
	}
	recs, dmg := scanAll(t, dir)
	if got := strings.Join(ids(recs), ","); got != "a,b,c" {
		t.Fatalf("replay after migration read %q, want a,b,c", got)
	}
	if dmg != (LedgerDamage{}) {
		t.Fatalf("migration introduced damage: %+v", dmg)
	}

	// AND IT RUNS EXACTLY ONCE. A second open must not rename anything.
	_ = l.Close()
	l2, err := OpenLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer l2.Close()
	if l2.Migrated() != "" {
		t.Fatalf("the migration ran a second time and produced %q", l2.Migrated())
	}
}

// TestMigrationRepairsTheTornTailBeforeItRenames: a partial record that was
// never durable must not be carried into an immutable segment, where nothing
// could ever repair it again.
func TestMigrationRepairsTheTornTailBeforeItRenames(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ledgerName)
	b, _ := json.Marshal(rec("whole", day("2026-08-01").Add(time.Hour)))
	appendLine(t, path, string(b)+"\n")
	appendLine(t, path, `{"type":"MIGRATION","migrationId":"tor`)

	l, err := OpenLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if l.Repaired() == 0 {
		t.Fatal("the torn tail was renamed into the legacy segment instead of being repaired")
	}
	recs, dmg := scanAll(t, dir)
	if got := strings.Join(ids(recs), ","); got != "whole" {
		t.Fatalf("replay read %q, want whole", got)
	}
	if dmg.Any() {
		t.Fatalf("the torn tail became permanent segment damage: %+v", dmg)
	}
}

// TestMigrationOfAnEmptyOrAbsentLedgerIsANoOp: a fresh data directory is not a
// migration, and it must still end up segmented so the question is asked once.
func TestMigrationOfAnEmptyOrAbsentLedgerIsANoOp(t *testing.T) {
	for _, name := range []string{"absent", "empty"} {
		dir := t.TempDir()
		if name == "empty" {
			appendLine(t, filepath.Join(dir, ledgerName), "")
		}
		l, err := OpenLedger(dir)
		if err != nil {
			t.Fatal(err)
		}
		if l.Migrated() != "" {
			t.Fatalf("%s: migrated %q", name, l.Migrated())
		}
		if !exists(segmentsDir(dir)) {
			t.Fatalf("%s: the segments directory was not created, so the migration would be asked again", name)
		}
		_ = l.Close()
	}
}

// TestRollbackIsAConcatenation is the design's rollback claim: segments are
// ordered, append-only and never rewritten, so the segments in order followed by
// the live file reproduce a byte-exact migrations.jsonl that the previous binary
// runs on unchanged.
func TestRollbackIsAConcatenation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ledgerName)
	var whole bytes.Buffer
	at := day("2026-08-14").Add(time.Hour)
	for i := 0; i < 12; i++ {
		b, _ := json.Marshal(rec(string(rune('a'+i)), at.Add(time.Duration(i)*10*time.Hour)))
		whole.Write(b)
		whole.WriteByte('\n')
	}
	if err := os.WriteFile(path, whole.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	l, err := OpenLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Keep writing after the migration so the rollback spans a legacy segment,
	// day segments and the live file.
	for i := 12; i < 20; i++ {
		mustAppendAt(t, l, string(rune('a'+i)), at.Add(time.Duration(i)*10*time.Hour))
	}
	a := &Archive{cfg: Config{DataDir: dir}, log: testLogger(), ledger: l}
	a.ledgerMaintenancePass(time.Now())
	_ = l.Close()

	// The rollback: cat/zcat the segments in order, then the live file.
	var back bytes.Buffer
	segs, err := LedgerSegments(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(segs) < 3 {
		t.Fatalf("only %d segments; this test needs a run of them", len(segs))
	}
	for _, s := range segs {
		f, err := os.Open(s.Path)
		if err != nil {
			t.Fatal(err)
		}
		var r = f
		if s.Compressed {
			zr, err := gzip.NewReader(f)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := back.ReadFrom(zr); err != nil {
				t.Fatal(err)
			}
			zr.Close()
			f.Close()
			continue
		}
		if _, err := back.ReadFrom(r); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	live, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	back.Write(live)

	// Everything written, in write order, byte for byte.
	var want bytes.Buffer
	want.Write(whole.Bytes())
	for i := 12; i < 20; i++ {
		b, _ := json.Marshal(rec(string(rune('a'+i)), at.Add(time.Duration(i)*10*time.Hour)))
		want.Write(b)
		want.WriteByte('\n')
	}
	if !bytes.Equal(back.Bytes(), want.Bytes()) {
		t.Fatalf("the rollback concatenation is %d bytes and the original is %d; they are not identical",
			back.Len(), want.Len())
	}
}

// --------------------------------------------------------- replay equivalence

// TestSegmentedReplayEqualsMonolithic is the validation bar
// archive_rollup_design.md sets: "a segmented replay equals a monolithic one over
// the same records". Every record, in order, FIELD FOR FIELD, and the same
// damage accounting — including damage, which is the case that would be easiest
// to get subtly wrong.
func TestSegmentedReplayEqualsMonolithic(t *testing.T) {
	mono := t.TempDir()
	monoPath := filepath.Join(mono, ledgerName)
	var lines []string
	at := day("2026-08-01").Add(time.Hour)
	for i := 0; i < 40; i++ {
		b, _ := json.Marshal(rec(idOf(i), at.Add(time.Duration(i)*6*time.Hour)))
		lines = append(lines, string(b))
	}
	// Two damaged lines, one in what will become a closed segment and one in
	// what will become the live file.
	lines[7] = `{"type":"MIGRATION","migrationId":"spliced"` + string(rune(0)) + `garbage`
	lines[37] = `not json at all`
	for _, ln := range lines {
		appendLine(t, monoPath, ln+"\n")
	}
	// The monolithic replay is the reference, taken BEFORE the file is
	// segmented: a data directory with no segments directory replays the live
	// file alone, exactly as it always did.
	monoRecs, monoDmg := scanAllNoMigrate(t, mono)

	// The same bytes, split by day into segments, half of them compressed.
	seg := t.TempDir()
	segPath := filepath.Join(seg, ledgerName)
	for _, ln := range lines {
		appendLine(t, segPath, ln+"\n")
	}
	l, err := OpenLedger(seg) // migrates the whole file into one legacy segment
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	// Now split that legacy segment into day segments by hand, so the comparison
	// is over a REAL run of segments rather than one.
	splitIntoDaySegments(t, seg)
	a := &Archive{cfg: Config{DataDir: seg}, log: testLogger()}
	a.ledgerMaintenancePass(time.Now()) // compresses every closed segment

	segRecs, segDmg := scanAll(t, seg)

	if len(segRecs) != len(monoRecs) {
		t.Fatalf("segmented replay read %d records, monolithic read %d", len(segRecs), len(monoRecs))
	}
	for i := range monoRecs {
		if !reflect.DeepEqual(segRecs[i], monoRecs[i]) {
			t.Fatalf("record %d differs:\n segmented %+v\n monolithic %+v", i, segRecs[i], monoRecs[i])
		}
	}
	if segDmg.Lines != monoDmg.Lines || segDmg.Bytes != monoDmg.Bytes {
		t.Fatalf("damage differs: segmented %+v, monolithic %+v", segDmg, monoDmg)
	}
	if segDmg.Lines != 2 {
		t.Fatalf("expected 2 damaged lines to survive segmentation, got %d", segDmg.Lines)
	}
	if segDmg.Segments != 0 {
		t.Fatalf("a healthy run of segments reported %d unreadable segments", segDmg.Segments)
	}
}

func idOf(i int) string { return fmt.Sprintf("m%03d", i) }

// scanAllNoMigrate replays a directory that has no segments directory, which is
// exactly the pre-segmentation behaviour.
func scanAllNoMigrate(t *testing.T, dir string) ([]Record, LedgerDamage) {
	t.Helper()
	if exists(segmentsDir(dir)) {
		t.Fatal("this directory is already segmented")
	}
	return scanAll(t, dir)
}

// splitIntoDaySegments rewrites one legacy segment into per-day segments. It is
// a TEST FIXTURE and not the production path — production splits by rotating —
// but it produces exactly the run of files a month of rotations would.
//
// It STREAMS, because the one ledger this is pointed at in anger is 1.8 GB and a
// fixture that materialises it would be measuring the fixture.
func splitIntoDaySegments(t *testing.T, dir string) {
	t.Helper()
	segs, err := LedgerSegments(dir)
	if err != nil {
		t.Fatal(err)
	}
	sd := segmentsDir(dir)
	for _, s := range segs {
		f, err := os.Open(s.Path)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]*os.File{}
		r := bufio.NewReaderSize(f, 1<<20)
		cur := s.FirstDay.Format(dayLayout)
		for {
			line, rerr := readLine(r)
			if len(line) > 0 && !errors.Is(rerr, io.EOF) {
				if ms, ok := recordedAtOf(line); ok {
					cur = dayOfMs(ms).Format(dayLayout)
				}
				w, ok := out[cur]
				if !ok {
					w, err = os.Create(filepath.Join(sd, segmentName(day(cur), 0)+plainSuffix))
					if err != nil {
						t.Fatal(err)
					}
					out[cur] = w
				}
				if _, err := w.Write(append(line, '\n')); err != nil {
					t.Fatal(err)
				}
			}
			if rerr != nil {
				break
			}
		}
		for _, w := range out {
			if err := w.Close(); err != nil {
				t.Fatal(err)
			}
		}
		f.Close()
		if err := os.Remove(s.Path); err != nil {
			t.Fatal(err)
		}
	}
}

// ------------------------------------------------------------- damage rules

// TestDamagedLineInAClosedGzSegmentIsSkippedAndCounted is B20 restated for
// segments: compression does not heal a damaged line and must not, because a
// compressor that "fixed" a line would be inventing a record.
func TestDamagedLineInAClosedGzSegmentIsSkippedAndCounted(t *testing.T) {
	dir := t.TempDir()
	sd := segmentsDir(dir)
	if err := os.MkdirAll(sd, 0o755); err != nil {
		t.Fatal(err)
	}
	good1, _ := json.Marshal(rec("a", day("2026-08-14").Add(time.Hour)))
	good2, _ := json.Marshal(rec("b", day("2026-08-14").Add(2*time.Hour)))
	body := string(good1) + "\n" + `{"broken":` + "\n" + string(good2) + "\n"
	if err := os.WriteFile(filepath.Join(sd, "2026-08-14-0000"+plainSuffix), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	plainRecs, plainDmg := scanAll(t, dir)

	if _, _, err := compressSegment(dir, "2026-08-14-0000", gzip.BestSpeed); err != nil {
		t.Fatalf("compressing a segment with a damaged line failed: %v", err)
	}
	if exists(filepath.Join(sd, "2026-08-14-0000"+plainSuffix)) {
		t.Fatal("the plain segment survived a verified compression")
	}
	gzRecs, gzDmg := scanAll(t, dir)

	if !reflect.DeepEqual(gzRecs, plainRecs) {
		t.Fatal("compression changed which records the replay delivered")
	}
	if gzDmg.Lines != 1 || gzDmg.Lines != plainDmg.Lines || gzDmg.Bytes != plainDmg.Bytes {
		t.Fatalf("damage differs across compression: plain %+v, gz %+v", plainDmg, gzDmg)
	}
	if got := strings.Join(ids(gzRecs), ","); got != "a,b" {
		t.Fatalf("replay read %q, want a,b — the record behind the damage was lost", got)
	}
}

// TestAClosedSegmentHasNoTornTail: a segment is renamed out of a
// newline-terminated file, so a missing newline in one is damage that arrived
// AFTER the segment closed. Counting it as a torn tail would silently drop a
// record that may well have been durable.
func TestAClosedSegmentHasNoTornTail(t *testing.T) {
	dir := t.TempDir()
	sd := segmentsDir(dir)
	if err := os.MkdirAll(sd, 0o755); err != nil {
		t.Fatal(err)
	}
	good, _ := json.Marshal(rec("a", day("2026-08-14").Add(time.Hour)))
	partial, _ := json.Marshal(rec("b", day("2026-08-14").Add(2*time.Hour)))
	body := string(good) + "\n" + string(partial) // no trailing newline
	if err := os.WriteFile(filepath.Join(sd, "2026-08-14-0000"+plainSuffix), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	recs, dmg := scanAll(t, dir)
	if dmg.TornTail != 0 {
		t.Fatalf("a closed segment reported a torn tail of %d bytes", dmg.TornTail)
	}
	if dmg.Lines != 1 {
		t.Fatalf("the unterminated final line of a closed segment counted %d damaged lines, want 1", dmg.Lines)
	}
	if got := strings.Join(ids(recs), ","); got != "a" {
		t.Fatalf("replay read %q, want a", got)
	}
}

// TestAnUnreadableSegmentIsSkippedWholeAndTheReplayCarriesOn is the 2026-08-08
// rule at the segment level: stopping at a bad segment would throw away every
// record behind it, and every one of them is still perfectly readable.
func TestAnUnreadableSegmentIsSkippedWholeAndTheReplayCarriesOn(t *testing.T) {
	dir := t.TempDir()
	sd := segmentsDir(dir)
	if err := os.MkdirAll(sd, 0o755); err != nil {
		t.Fatal(err)
	}
	for i, d := range []string{"2026-08-14", "2026-08-15", "2026-08-16"} {
		b, _ := json.Marshal(rec(string(rune('a'+i)), day(d).Add(time.Hour)))
		if err := os.WriteFile(filepath.Join(sd, segmentName(day(d), 0)+plainSuffix),
			append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Compress the middle one and then truncate its gzip stream.
	if _, _, err := compressSegment(dir, "2026-08-15-0000", gzip.BestSpeed); err != nil {
		t.Fatal(err)
	}
	gzPath := filepath.Join(sd, "2026-08-15-0000"+gzSuffix)
	b, err := os.ReadFile(gzPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(gzPath, b[:len(b)/2], 0o644); err != nil {
		t.Fatal(err)
	}

	recs, dmg := scanAll(t, dir)
	if got := strings.Join(ids(recs), ","); got != "a,c" {
		t.Fatalf("replay read %q, want a,c — the records behind the bad segment were lost", got)
	}
	if dmg.Segments != 1 {
		t.Fatalf("damage.Segments = %d, want 1", dmg.Segments)
	}
	if !strings.Contains(dmg.SegmentReasons, "2026-08-15-0000") {
		t.Fatalf("the damage does not say WHICH segment: %q", dmg.SegmentReasons)
	}
	if !dmg.Any() {
		t.Fatal("an unreadable segment did not register as damage")
	}
}

// ------------------------------------------------------------ compression

// TestCompressionIsVerifiedBeforeThePlainFileIsDeleted, and its converse: a
// round trip that does not hold up leaves the plain segment exactly where it
// was.
func TestCompressionIsVerifiedBeforeThePlainFileIsDeleted(t *testing.T) {
	dir := t.TempDir()
	sd := segmentsDir(dir)
	if err := os.MkdirAll(sd, 0o755); err != nil {
		t.Fatal(err)
	}
	name := "2026-08-14-0000"
	plain := filepath.Join(sd, name+plainSuffix)
	var body bytes.Buffer
	for i := 0; i < 200; i++ {
		b, _ := json.Marshal(rec(idOf(i), day("2026-08-14").Add(time.Duration(i)*time.Minute)))
		body.Write(b)
		body.WriteByte('\n')
	}
	if err := os.WriteFile(plain, body.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	want := fileSHA(t, plain)

	plainB, gzB, err := compressSegment(dir, name, gzip.BestSpeed)
	if err != nil {
		t.Fatal(err)
	}
	if plainB != int64(body.Len()) {
		t.Fatalf("compressSegment reported %d plain bytes, want %d", plainB, body.Len())
	}
	if gzB <= 0 || gzB >= plainB {
		t.Fatalf("gz is %d bytes against %d plain, which is not a compression", gzB, plainB)
	}
	if exists(plain) {
		t.Fatal("the plain segment survived a verified compression")
	}
	if exists(filepath.Join(sd, name+gzSuffix+tmpSuffix)) {
		t.Fatal("a temporary file was left behind")
	}
	// The gz decompresses to exactly what was there.
	d, err := digestOfGz(filepath.Join(sd, name+gzSuffix))
	if err != nil {
		t.Fatal(err)
	}
	if d.SHA256 != want {
		t.Fatal("the compressed segment does not decompress to the original bytes")
	}
	if d.Lines != 200 {
		t.Fatalf("the compressed segment holds %d lines, want 200", d.Lines)
	}
}

// TestAFailedRoundTripKeepsThePlainSegment: gzMatchesPlain is the gate on the
// only copy of the record, and this proves the gate is real by handing it a gz
// that decompresses to something else.
func TestAFailedRoundTripKeepsThePlainSegment(t *testing.T) {
	dir := t.TempDir()
	sd := segmentsDir(dir)
	if err := os.MkdirAll(sd, 0o755); err != nil {
		t.Fatal(err)
	}
	name := "2026-08-14-0000"
	plain := filepath.Join(sd, name+plainSuffix)
	b, _ := json.Marshal(rec("a", day("2026-08-14").Add(time.Hour)))
	if err := os.WriteFile(plain, append(b, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	// A gz of DIFFERENT content, beside the plain file: exactly what a crash
	// between a compressor's rename and its delete would leave if the compressor
	// were wrong.
	var gzbuf bytes.Buffer
	zw := gzip.NewWriter(&gzbuf)
	zw.Write([]byte("{\"type\":\"MIGRATION\",\"migrationId\":\"WRONG\"}\n"))
	zw.Close()
	if err := os.WriteFile(filepath.Join(sd, name+gzSuffix), gzbuf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	rec, err := reconcileSegments(dir, func(string, ...any) {})
	if err != nil {
		t.Fatal(err)
	}
	if rec.GzRemoved != 1 {
		t.Fatalf("reconcile removed %d bad gz files, want 1", rec.GzRemoved)
	}
	if !exists(plain) {
		t.Fatal("the plain segment — the record — was deleted in favour of a gz that did not match it")
	}
	recs, _ := scanAll(t, dir)
	if got := strings.Join(ids(recs), ","); got != "a" {
		t.Fatalf("replay read %q, want a", got)
	}
}

// ------------------------------------------------- crash reconciliation matrix

// TestCrashAtEachStepOfRotationAndCompressionLosesNothing enumerates every step
// of the two multi-step operations in this package and leaves the directory in
// the exact state a crash at that step would leave it. THE ASSERTION IS THE SAME
// EVERY TIME: reopen, replay, and read back every record that was ever durable.
//
// The steps are the ones the comments on rotateLocked and compressSegment name:
//
//	rotate 1  fsync'd, not yet renamed          the live file still holds the day
//	rotate 2  renamed, no fresh live file       segment present, live file absent
//	rotate 3  renamed and reopened, no dir sync indistinguishable from success
//	compress 1  tmp written, not verified       tmp + plain
//	compress 2  tmp verified, not renamed       tmp + plain
//	compress 3  renamed, plain not deleted      gz + plain
//	compress 4  plain deleted, dir not synced   gz only
func TestCrashAtEachStepOfRotationAndCompressionLosesNothing(t *testing.T) {
	// The records every case must be able to read back, whatever happened.
	build := func(t *testing.T) (string, []string) {
		t.Helper()
		dir := t.TempDir()
		l, err := OpenLedger(dir)
		if err != nil {
			t.Fatal(err)
		}
		var want []string
		for i, at := range []time.Time{
			day("2026-08-14").Add(time.Hour),
			day("2026-08-14").Add(2 * time.Hour),
			day("2026-08-15").Add(time.Hour),
		} {
			id := string(rune('a' + i))
			mustAppendAt(t, l, id, at)
			want = append(want, id)
		}
		if err := l.Close(); err != nil {
			t.Fatal(err)
		}
		return dir, want
	}
	assertWhole := func(t *testing.T, dir string, want []string) {
		t.Helper()
		l, err := OpenLedger(dir)
		if err != nil {
			t.Fatalf("reopen: %v", err)
		}
		defer l.Close()
		recs, dmg := scanAll(t, dir)
		if got := strings.Join(ids(recs), ","); got != strings.Join(want, ",") {
			t.Fatalf("replay read %q, want %q", got, strings.Join(want, ","))
		}
		if dmg.Any() {
			t.Fatalf("replay reported damage: %+v", dmg)
		}
	}

	sd := func(dir string) string { return segmentsDir(dir) }

	t.Run("rotate1_fsynced_not_renamed", func(t *testing.T) {
		// The live file still holds a day that is over and there is no segment
		// for it: exactly what a crash before the rename leaves. Nothing was
		// ever written twice, so the only cost is that the segment is named at
		// the next rotation instead of this one.
		dir := t.TempDir()
		l, err := OpenLedger(dir)
		if err != nil {
			t.Fatal(err)
		}
		mustAppendAt(t, l, "a", day("2026-08-14").Add(time.Hour))
		mustAppendAt(t, l, "b", day("2026-08-14").Add(2*time.Hour))
		_ = l.Close()
		if n := len(segNames(t, dir)); n != 0 {
			t.Fatalf("%d segments, want 0 for this case", n)
		}
		assertWhole(t, dir, []string{"a", "b"})
		// And the next append still closes it correctly.
		l2, err := OpenLedger(dir)
		if err != nil {
			t.Fatal(err)
		}
		mustAppendAt(t, l2, "c", day("2026-08-15").Add(time.Hour))
		_ = l2.Close()
		if got, want := segNames(t, dir), []string{"2026-08-14-0000"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("segments are %v, want %v", got, want)
		}
		assertWhole(t, dir, []string{"a", "b", "c"})
	})

	t.Run("rotate2_renamed_no_fresh_live_file", func(t *testing.T) {
		dir, want := build(t)
		// The live file holds day 3's record. Simulate a crash after the rename
		// and before the reopen: move it into a segment and delete it.
		if err := os.Rename(filepath.Join(dir, ledgerName),
			filepath.Join(sd(dir), "2026-08-15-0000"+plainSuffix)); err != nil {
			t.Fatal(err)
		}
		if exists(filepath.Join(dir, ledgerName)) {
			t.Fatal("the live file should be absent for this case")
		}
		assertWhole(t, dir, want)
	})

	t.Run("rotate3_renamed_and_reopened", func(t *testing.T) {
		dir, want := build(t)
		assertWhole(t, dir, want)
	})

	t.Run("compress1_tmp_written_not_verified", func(t *testing.T) {
		dir, want := build(t)
		name := "2026-08-14-0000"
		// A half-written tmp beside its intact source.
		if err := os.WriteFile(filepath.Join(sd(dir), name+gzSuffix+tmpSuffix),
			[]byte("\x1f\x8b truncated garbage"), 0o644); err != nil {
			t.Fatal(err)
		}
		l, err := OpenLedger(dir)
		if err != nil {
			t.Fatal(err)
		}
		if l.Reconciled().TempsRemoved != 1 {
			t.Fatalf("reconcile removed %d temps, want 1", l.Reconciled().TempsRemoved)
		}
		_ = l.Close()
		if exists(filepath.Join(sd(dir), name+gzSuffix+tmpSuffix)) {
			t.Fatal("the half-written temp survived reconciliation")
		}
		assertWhole(t, dir, want)
	})

	t.Run("compress2_tmp_verified_not_renamed", func(t *testing.T) {
		dir, want := build(t)
		name := "2026-08-14-0000"
		plain := filepath.Join(sd(dir), name+plainSuffix)
		b, err := os.ReadFile(plain)
		if err != nil {
			t.Fatal(err)
		}
		// A COMPLETE, CORRECT tmp — the compressor got all the way to its rename
		// and died. It is still deleted: a tmp is a derived artifact and the
		// source is right there.
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		zw.Write(b)
		zw.Close()
		if err := os.WriteFile(filepath.Join(sd(dir), name+gzSuffix+tmpSuffix), buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		l, err := OpenLedger(dir)
		if err != nil {
			t.Fatal(err)
		}
		if l.Reconciled().TempsRemoved != 1 {
			t.Fatalf("reconcile removed %d temps, want 1", l.Reconciled().TempsRemoved)
		}
		_ = l.Close()
		assertWhole(t, dir, want)
	})

	t.Run("compress3_renamed_plain_not_deleted", func(t *testing.T) {
		dir, want := build(t)
		name := "2026-08-14-0000"
		plain := filepath.Join(sd(dir), name+plainSuffix)
		b, err := os.ReadFile(plain)
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		zw.Write(b)
		zw.Close()
		if err := os.WriteFile(filepath.Join(sd(dir), name+gzSuffix), buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		// BEFORE reconciliation, the replay must already be correct: it prefers
		// the plain file, so the records are not doubled.
		recs, _ := scanAll(t, dir)
		if got := strings.Join(ids(recs), ","); got != strings.Join(want, ",") {
			t.Fatalf("an unreconciled both-forms directory replayed %q, want %q — records were doubled",
				got, strings.Join(want, ","))
		}
		l, err := OpenLedger(dir)
		if err != nil {
			t.Fatal(err)
		}
		if l.Reconciled().PlainRemoved != 1 {
			t.Fatalf("reconcile removed %d plain files, want 1", l.Reconciled().PlainRemoved)
		}
		_ = l.Close()
		if exists(plain) {
			t.Fatal("the plain file survived a verified gz")
		}
		assertWhole(t, dir, want)
	})

	t.Run("compress4_plain_deleted_dir_not_synced", func(t *testing.T) {
		dir, want := build(t)
		if _, _, err := compressSegment(dir, "2026-08-14-0000", gzip.BestSpeed); err != nil {
			t.Fatal(err)
		}
		assertWhole(t, dir, want)
	})
}

// ------------------------------------------------------------ retirement gate

// newTestArchive is the smallest Archive the maintenance pass needs. It does not
// dial, listen or replay: the pass reads a directory and writes counters.
func newTestArchive(t *testing.T, dir string, window time.Duration) *Archive {
	t.Helper()
	l, err := OpenLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	return &Archive{
		cfg:    Config{DataDir: dir, LedgerWindow: window},
		log:    testLogger(),
		ledger: l,
	}
}

// makeClosedSegment writes one closed, compressed segment for a day.
func makeClosedSegment(t *testing.T, dir, d string, n int) string {
	t.Helper()
	sd := segmentsDir(dir)
	if err := os.MkdirAll(sd, 0o755); err != nil {
		t.Fatal(err)
	}
	name := segmentName(day(d), 0)
	var body bytes.Buffer
	for i := 0; i < n; i++ {
		b, _ := json.Marshal(rec(d+"-"+strconv.Itoa(i), day(d).Add(time.Duration(i)*time.Minute)))
		body.Write(b)
		body.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(sd, name+plainSuffix), body.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := compressSegment(dir, name, gzip.BestSpeed); err != nil {
		t.Fatal(err)
	}
	return name
}

// writeGoodReceipt writes the receipt the gate accepts: it names the segment,
// carries the segment's real size and sha256, names a destination, carries a
// checksum read back from the store, and is verified.
func writeGoodReceipt(t *testing.T, dir, name string) {
	t.Helper()
	path := filepath.Join(segmentsDir(dir), name+gzSuffix)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	if err := WriteReceipt(dir, ColdCopyReceipt{
		Segment:            name + gzSuffix,
		Bytes:              info.Size(),
		SHA256:             fileSHA(t, path),
		Destination:        "s3://example-cold-archive/ledger/" + name + gzSuffix,
		RemoteChecksum:     "an-etag-or-checksum",
		RemoteChecksumKind: "etag",
		UploadedAtMs:       now,
		VerifiedAtMs:       now,
		VerifiedBy:         "test",
	}); err != nil {
		t.Fatal(err)
	}
}

// TestNoReceiptNoRetirement is the whole point of the gate. A segment far past
// the window, with no cold-copy receipt, STAYS — and says so in a counter.
func TestNoReceiptNoRetirement(t *testing.T) {
	dir := t.TempDir()
	name := makeClosedSegment(t, dir, "2026-01-01", 5)
	a := newTestArchive(t, dir, 720*time.Hour)

	a.ledgerMaintenancePass(time.Now())

	if !exists(filepath.Join(segmentsDir(dir), name+gzSuffix)) {
		t.Fatal("a segment with NO cold-copy receipt was removed from the host")
	}
	segments, _, awaiting, retired := a.SegmentCounters()
	if awaiting != 1 {
		t.Fatalf("ledgerSegmentsAwaitingColdCopy = %d, want 1", awaiting)
	}
	if retired != 0 {
		t.Fatalf("ledgerRetiredTotal = %d, want 0", retired)
	}
	if segments != 1 {
		t.Fatalf("ledgerSegments = %d, want 1", segments)
	}
	// And it stays waiting, pass after pass, forever if need be.
	for i := 0; i < 5; i++ {
		a.ledgerMaintenancePass(time.Now().Add(time.Duration(i) * 24 * time.Hour))
	}
	if _, _, awaiting, retired = a.SegmentCounters(); awaiting != 1 || retired != 0 {
		t.Fatalf("after five passes: awaiting %d, retired %d — the gate opened on its own", awaiting, retired)
	}
}

// TestAConfirmedReceiptRetiresTheSegmentAndKeepsTheReceipt.
func TestAConfirmedReceiptRetiresTheSegmentAndKeepsTheReceipt(t *testing.T) {
	dir := t.TempDir()
	name := makeClosedSegment(t, dir, "2026-01-01", 5)
	writeGoodReceipt(t, dir, name)
	a := newTestArchive(t, dir, 720*time.Hour)

	a.ledgerMaintenancePass(time.Now())

	if exists(filepath.Join(segmentsDir(dir), name+gzSuffix)) {
		t.Fatal("a segment with a confirmed off-host copy was not retired")
	}
	if !exists(receiptPath(dir, name)) {
		t.Fatal("the receipt was deleted with the segment; it is the only record of where the bytes went")
	}
	segments, _, awaiting, retired := a.SegmentCounters()
	if retired != 1 || awaiting != 0 || segments != 0 {
		t.Fatalf("segments %d, awaiting %d, retired %d; want 0, 0, 1", segments, awaiting, retired)
	}
	// The retired segment is still NAMED on the host, so the window's start and
	// the restore are both answerable.
	if got, want := segNames(t, dir), []string{name + " [retired]"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("segment set is %v, want %v", got, want)
	}
}

// TestARetirementNeedsTheWindowToBeOn: with no window, nothing is ever removed
// however old it is and however confirmed the copy. That is design option D and
// it is the default.
func TestARetirementNeedsTheWindowToBeOn(t *testing.T) {
	dir := t.TempDir()
	name := makeClosedSegment(t, dir, "2020-01-01", 3)
	writeGoodReceipt(t, dir, name)
	for _, window := range []time.Duration{0, -1} {
		a := newTestArchive(t, dir, window)
		a.ledgerMaintenancePass(time.Now())
		if !exists(filepath.Join(segmentsDir(dir), name+gzSuffix)) {
			t.Fatalf("window %v removed a segment", window)
		}
		if _, _, _, retired := a.SegmentCounters(); retired != 0 {
			t.Fatalf("window %v retired %d segments", window, retired)
		}
	}
}

// TestASegmentInsideTheWindowIsNeverRetired, even with a perfect receipt.
func TestASegmentInsideTheWindowIsNeverRetired(t *testing.T) {
	dir := t.TempDir()
	today := time.Now().UTC().Truncate(24 * time.Hour)
	name := makeClosedSegment(t, dir, today.AddDate(0, 0, -2).Format(dayLayout), 3)
	writeGoodReceipt(t, dir, name)
	a := newTestArchive(t, dir, 720*time.Hour)
	a.ledgerMaintenancePass(time.Now())
	if !exists(filepath.Join(segmentsDir(dir), name+gzSuffix)) {
		t.Fatal("a segment two days old was retired under a 720 h window")
	}
	if _, _, awaiting, retired := a.SegmentCounters(); awaiting != 0 || retired != 0 {
		t.Fatalf("a segment inside the window counted as awaiting %d / retired %d", awaiting, retired)
	}
}

// TestABadReceiptIsLoudAndDoesNotOpenTheGate walks every way a receipt can fail
// to hold up. Each one keeps the segment and each one counts.
func TestABadReceiptIsLoudAndDoesNotOpenTheGate(t *testing.T) {
	cases := map[string]func(r *ColdCopyReceipt){
		"wrong segment":      func(r *ColdCopyReceipt) { r.Segment = "2026-02-02-0000" + gzSuffix },
		"wrong size":         func(r *ColdCopyReceipt) { r.Bytes += 1 },
		"wrong sha256":       func(r *ColdCopyReceipt) { r.SHA256 = strings.Repeat("0", 64) },
		"never verified":     func(r *ColdCopyReceipt) { r.VerifiedAtMs = 0 },
		"no destination":     func(r *ColdCopyReceipt) { r.Destination = "  " },
		"no remote checksum": func(r *ColdCopyReceipt) { r.RemoteChecksum = "" },
	}
	for label, mangle := range cases {
		t.Run(label, func(t *testing.T) {
			dir := t.TempDir()
			name := makeClosedSegment(t, dir, "2026-01-01", 5)
			path := filepath.Join(segmentsDir(dir), name+gzSuffix)
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			now := time.Now().UnixMilli()
			r := ColdCopyReceipt{
				Segment: name + gzSuffix, Bytes: info.Size(), SHA256: fileSHA(t, path),
				Destination: "s3://b/k", RemoteChecksum: "x", RemoteChecksumKind: "etag",
				UploadedAtMs: now, VerifiedAtMs: now,
			}
			mangle(&r)
			// WriteReceipt refuses some of these outright, which is itself the
			// right answer; the rest are written by hand as a bad uploader
			// would.
			b, _ := json.Marshal(r)
			if err := os.WriteFile(receiptPath(dir, name), b, 0o644); err != nil {
				t.Fatal(err)
			}
			a := newTestArchive(t, dir, 720*time.Hour)
			a.ledgerMaintenancePass(time.Now())
			if !exists(path) {
				t.Fatalf("a receipt with %s retired the segment", label)
			}
			if _, _, awaiting, retired := a.SegmentCounters(); awaiting != 1 || retired != 0 {
				t.Fatalf("%s: awaiting %d, retired %d; want 1, 0", label, awaiting, retired)
			}
			if a.seg.badReceipts != 1 {
				t.Fatalf("%s: badReceipts = %d, want 1", label, a.seg.badReceipts)
			}
		})
	}
}

// TestAPlainSegmentIsNeverRetired: the only shape of thing that leaves this host
// is a verified .jsonl.gz with a receipt beside it.
func TestAPlainSegmentIsNeverRetired(t *testing.T) {
	dir := t.TempDir()
	sd := segmentsDir(dir)
	if err := os.MkdirAll(sd, 0o755); err != nil {
		t.Fatal(err)
	}
	name := segmentName(day("2020-01-01"), 0)
	b, _ := json.Marshal(rec("old", day("2020-01-01").Add(time.Hour)))
	if err := os.WriteFile(filepath.Join(sd, name+plainSuffix), append(b, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	a := newTestArchive(t, dir, 720*time.Hour)
	a.cfg.DisableLedgerCompression = true
	a.ledgerMaintenancePass(time.Now())
	if !exists(filepath.Join(sd, name+plainSuffix)) {
		t.Fatal("a plain segment was retired")
	}
	if _, _, _, retired := a.SegmentCounters(); retired != 0 {
		t.Fatalf("retired %d plain segments", retired)
	}
}

// TestTheWindowFollowsTheGenomeHorizon is "one horizon, three mechanisms" in
// code: setting only the genome horizon sets the ledger window with it, and
// --ledger-window exists to break the tie deliberately.
func TestTheWindowFollowsTheGenomeHorizon(t *testing.T) {
	for _, c := range []struct {
		horizon, window, want time.Duration
	}{
		{0, 0, 0},                             // the default: everything off
		{720 * time.Hour, 0, 720 * time.Hour}, // one knob, one number
		{720 * time.Hour, 24 * time.Hour, 24 * time.Hour}, // broken deliberately
		{720 * time.Hour, -1, 0},                          // horizon on, window explicitly off
		{0, 48 * time.Hour, 48 * time.Hour},               // window without a horizon
	} {
		a := &Archive{cfg: Config{GenomeHorizon: c.horizon, LedgerWindow: c.window}}
		if got := a.ledgerWindow(); got != c.want {
			t.Fatalf("horizon %v, window %v -> %v, want %v", c.horizon, c.window, got, c.want)
		}
	}
}

// --------------------------------------------------- the phase-1 replay source

// TestReplayFromAPositionEqualsTheTailOfAFullReplay is the API the roll-up
// sidecar tails on: replay from a saved position and get exactly the records a
// full replay would have delivered after that point.
func TestReplayFromAPositionEqualsTheTailOfAFullReplay(t *testing.T) {
	dir := t.TempDir()
	l, err := OpenLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	at := day("2026-08-01").Add(time.Hour)
	for i := 0; i < 30; i++ {
		mustAppendAt(t, l, idOf(i), at.Add(time.Duration(i)*8*time.Hour))
	}
	_ = l.Close()

	// Full replay, remembering the position after every record.
	var all []Record
	var positions []LedgerPos
	if _, _, err := ScanLedgerFrom(dir, LedgerPos{}, func(r Record, p LedgerPos) {
		all = append(all, r)
		positions = append(positions, p)
	}); err != nil {
		t.Fatal(err)
	}
	if len(all) != 30 {
		t.Fatalf("full replay read %d records, want 30", len(all))
	}
	for _, k := range []int{0, 1, 7, 12, 25, 29} {
		var tail []Record
		_, end, err := ScanLedgerFrom(dir, positions[k], func(r Record, _ LedgerPos) { tail = append(tail, r) })
		if err != nil {
			t.Fatalf("resume at %d: %v", k, err)
		}
		if !sameRecords(tail, all[k+1:]) {
			t.Fatalf("resume after record %d read %v, want %v", k, ids(tail), ids(all[k+1:]))
		}
		if int(end.Record) != len(all) {
			t.Fatalf("resume after record %d ended at record %d, want %d", k, end.Record, len(all))
		}
	}
}

// TestAPositionSurvivesACompression is why the offset is into the segment's
// UNCOMPRESSED bytes rather than into the file: the sidecar's saved position must
// not be invalidated by a maintenance pass it knows nothing about.
func TestAPositionSurvivesACompression(t *testing.T) {
	dir := t.TempDir()
	l, err := OpenLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	at := day("2026-08-01").Add(time.Hour)
	for i := 0; i < 20; i++ {
		mustAppendAt(t, l, idOf(i), at.Add(time.Duration(i)*8*time.Hour))
	}
	var saved LedgerPos
	var all []Record
	if _, _, err := ScanLedgerFrom(dir, LedgerPos{}, func(r Record, p LedgerPos) {
		all = append(all, r)
		if len(all) == 5 {
			saved = p
		}
	}); err != nil {
		t.Fatal(err)
	}
	if saved.Segment == "" {
		t.Fatal("the saved position is in the live file; this test needs it in a closed segment")
	}
	a := &Archive{cfg: Config{DataDir: dir}, log: testLogger(), ledger: l}
	a.ledgerMaintenancePass(time.Now())
	segs, _ := LedgerSegments(dir)
	compressed := false
	for _, s := range segs {
		if s.Name == saved.Segment && s.Compressed {
			compressed = true
		}
	}
	if !compressed {
		t.Fatal("the segment holding the saved position was not compressed; this test proves nothing")
	}

	var tail []Record
	if _, _, err := ScanLedgerFrom(dir, saved, func(r Record, _ LedgerPos) { tail = append(tail, r) }); err != nil {
		t.Fatal(err)
	}
	if !sameRecords(tail, all[5:]) {
		t.Fatalf("after compression the resume read %v, want %v", ids(tail), ids(all[5:]))
	}
	_ = l.Close()
}

// TestReplayFromARecordNumberIsTheBridge: phase 1's "replay from record N",
// plumbed onto "replay from (segment, offset)". A damaged line is skipped and
// does NOT advance the record number, which is exactly what makes a line count
// the wrong thing to persist.
func TestReplayFromARecordNumberIsTheBridge(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ledgerName)
	at := day("2026-08-01").Add(time.Hour)
	var want []Record
	for i := 0; i < 12; i++ {
		if i == 4 {
			appendLine(t, path, "{not json\n")
			continue
		}
		r := rec(idOf(i), at.Add(time.Duration(i)*8*time.Hour))
		b, _ := json.Marshal(r)
		appendLine(t, path, string(b)+"\n")
		want = append(want, r)
	}
	l, err := OpenLedger(dir) // migrate, then split by day
	if err != nil {
		t.Fatal(err)
	}
	_ = l.Close()
	splitIntoDaySegments(t, dir)

	var tail []Record
	dmg, from, end, err := ScanLedgerFromRecord(dir, 6, func(r Record, _ LedgerPos) { tail = append(tail, r) })
	if err != nil {
		t.Fatal(err)
	}
	if !sameRecords(tail, want[6:]) {
		t.Fatalf("resume from record 6 read %v, want %v", ids(tail), ids(want[6:]))
	}
	if from.Record != 6 {
		t.Fatalf("the start position is at record %d, want 6", from.Record)
	}
	if int(end.Record) != len(want) {
		t.Fatalf("the end position is at record %d, want %d", end.Record, len(want))
	}
	// The damaged line is BEFORE the resume point, so a resumed replay does not
	// count it again.
	if dmg.Lines != 0 {
		t.Fatalf("a resumed replay re-counted %d damaged lines from before its start", dmg.Lines)
	}
}

// TestAPositionInARetiredSegmentSaysSo: silently replaying from somewhere else
// would drop records without a word, so a position older than the raw window is
// an error the caller can act on.
func TestAPositionInARetiredSegmentSaysSo(t *testing.T) {
	dir := t.TempDir()
	name := makeClosedSegment(t, dir, "2026-01-01", 5)
	writeGoodReceipt(t, dir, name)
	a := newTestArchive(t, dir, 720*time.Hour)
	a.ledgerMaintenancePass(time.Now())

	_, _, err := ScanLedgerFrom(dir, LedgerPos{Segment: name, Offset: 10, Record: 2},
		func(Record, LedgerPos) { t.Fatal("a retired position delivered a record") })
	if err == nil || !strings.Contains(err.Error(), "retired") {
		t.Fatalf("resuming from a retired segment returned %v, want ErrPositionRetired", err)
	}
}

// TestLedgerSourcesAreOrderedOldestFirstAndEndAtTheLiveFile.
func TestLedgerSourcesAreOrderedOldestFirstAndEndAtTheLiveFile(t *testing.T) {
	dir := t.TempDir()
	l, err := OpenLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	for i, d := range []string{"2026-08-14", "2026-08-15", "2026-08-16"} {
		mustAppendAt(t, l, string(rune('a'+i)), day(d).Add(time.Hour))
	}
	_ = l.Close()
	got, err := LedgerSources(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"2026-08-14-0000", "2026-08-15-0000", ""}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sources are %v, want %v (the live file is last and its name is empty)", got, want)
	}
}

// ------------------------------------------------------------ status fields

// TestTheSegmentStatusFieldsAreThereAndMeanWhatTheySay pins the five fields
// phase 4's monitor gate and docs/observability.md will read.
func TestTheSegmentStatusFieldsAreThereAndMeanWhatTheySay(t *testing.T) {
	dir := t.TempDir()
	old := makeClosedSegment(t, dir, "2026-01-01", 4)  // past the window, no receipt
	done := makeClosedSegment(t, dir, "2026-01-02", 4) // past the window, receipt
	writeGoodReceipt(t, dir, done)
	recent := makeClosedSegment(t, dir, time.Now().UTC().AddDate(0, 0, -1).Format(dayLayout), 4)
	a := newTestArchive(t, dir, 720*time.Hour)
	a.ledgerMaintenancePass(time.Now())

	v := a.StatusView()
	if v.LedgerSegments != 2 {
		t.Fatalf("ledgerSegments = %d, want 2 (%s and %s)", v.LedgerSegments, old, recent)
	}
	if v.LedgerSegmentsAwaitingColdCopy != 1 {
		t.Fatalf("ledgerSegmentsAwaitingColdCopy = %d, want 1", v.LedgerSegmentsAwaitingColdCopy)
	}
	if v.LedgerRetired != 1 {
		t.Fatalf("ledgerRetiredTotal = %d, want 1", v.LedgerRetired)
	}
	if v.LedgerRawBytes <= 0 {
		t.Fatalf("ledgerRawBytes = %d, want the bytes the segments occupy", v.LedgerRawBytes)
	}
	if v.LedgerRawWindowFromMs != day("2026-01-01").UnixMilli() {
		t.Fatalf("ledgerRawWindowFromMs = %d, want the oldest PRESENT segment's day %d",
			v.LedgerRawWindowFromMs, day("2026-01-01").UnixMilli())
	}
	if v.LedgerWindowMs != (720 * time.Hour).Milliseconds() {
		t.Fatalf("ledgerWindowMs = %d", v.LedgerWindowMs)
	}
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"ledgerSegments", "ledgerRawBytes", "ledgerRawWindowFromMs",
		"ledgerSegmentsAwaitingColdCopy", "ledgerRetiredTotal"} {
		if !strings.Contains(string(b), `"`+key+`"`) {
			t.Fatalf("the status JSON has no %q", key)
		}
	}
}

// TestUnknownFilesInTheSegmentsDirectoryAreLeftAlone: coldcopy.jsonl lives there
// and so might an operator's notes. This package does not delete what it does
// not understand.
func TestUnknownFilesInTheSegmentsDirectoryAreLeftAlone(t *testing.T) {
	dir := t.TempDir()
	name := makeClosedSegment(t, dir, "2026-08-14", 3)
	stray := filepath.Join(segmentsDir(dir), "coldcopy.jsonl")
	if err := os.WriteFile(stray, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := reconcileSegments(dir, func(string, ...any) {})
	if err != nil {
		t.Fatal(err)
	}
	if !exists(stray) {
		t.Fatal("reconciliation deleted a file it does not understand")
	}
	if len(r.Unknown) != 1 || r.Unknown[0] != "coldcopy.jsonl" {
		t.Fatalf("unknown files are %v, want [coldcopy.jsonl]", r.Unknown)
	}
	if got := segNames(t, dir); len(got) != 1 || got[0] != name+" [gz]" {
		t.Fatalf("segments are %v", got)
	}
}

// testLogger is a logger the tests can leave on: warnings and errors go to the
// test output where a failure can be read, and info does not.
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// ---------------------------------------------------- the real-ledger check

// TestRealLedgerSegmentedReplayEqualsMonolithic is the validation bar run
// against the production record itself rather than a fixture.
//
// It is SKIPPED unless MV_REAL_LEDGER names a COPY of a real ledger, because
// there is no such file in this repository and there must never be one. Point it
// at a copy — never at a live ledger and never at the pristine copy itself: this
// test writes a working copy of what it is given, splits it into day segments
// and compresses them, which is several gigabytes of scratch.
//
// The claim it holds is the strongest one in this package: over the same bytes,
// a replay of an ordered run of compressed day segments delivers the same
// records, in the same order, byte for byte, with the same damage accounting, as
// a replay of the one file they were cut from.
func TestRealLedgerSegmentedReplayEqualsMonolithic(t *testing.T) {
	src := os.Getenv("MV_REAL_LEDGER")
	if src == "" {
		t.Skip("MV_REAL_LEDGER is not set; this check needs a copy of a real ledger")
	}
	scratch := os.Getenv("MV_REAL_LEDGER_SCRATCH")
	if scratch == "" {
		t.Skip("MV_REAL_LEDGER_SCRATCH is not set; this check needs several GB of scratch")
	}
	if err := os.MkdirAll(scratch, 0o755); err != nil {
		t.Fatal(err)
	}

	// The monolithic baseline: a data directory with no segments directory
	// replays the live file alone, which is exactly the pre-segmentation
	// behaviour.
	mono := filepath.Join(scratch, "mono")
	if err := os.MkdirAll(mono, 0o755); err != nil {
		t.Fatal(err)
	}
	monoLedger := filepath.Join(mono, ledgerName)
	if !exists(monoLedger) {
		copyFile(t, src, monoLedger)
	}
	monoN, monoDmg, monoHash := replayFingerprint(t, mono)
	t.Logf("monolithic: %d records, damage %+v, fingerprint %s", monoN, monoDmg, monoHash[:16])

	// The same bytes as an ordered run of compressed day segments.
	seg := filepath.Join(scratch, "seg")
	if err := os.MkdirAll(seg, 0o755); err != nil {
		t.Fatal(err)
	}
	if !exists(filepath.Join(seg, segmentsDirName)) {
		copyFile(t, src, filepath.Join(seg, ledgerName))
		l, err := OpenLedgerLog(seg, func(m string, kv ...any) { t.Log(m, kv) })
		if err != nil {
			t.Fatal(err)
		}
		if l.Migrated() == "" {
			t.Fatal("the real ledger was not migrated into a legacy segment")
		}
		if err := l.Close(); err != nil {
			t.Fatal(err)
		}
		splitIntoDaySegments(t, seg)
		a := &Archive{cfg: Config{DataDir: seg}, log: testLogger()}
		a.ledgerMaintenancePass(time.Now())
	}
	segs, err := LedgerSegments(seg)
	if err != nil {
		t.Fatal(err)
	}
	var plain, gz int
	var onDisk int64
	for _, s := range segs {
		onDisk += s.Bytes
		if s.Compressed {
			gz++
		} else {
			plain++
		}
	}
	t.Logf("segmented: %d segments (%d gz, %d plain), %d bytes on disk", len(segs), gz, plain, onDisk)
	if len(segs) < 2 {
		t.Fatalf("only %d segment(s); the split produced no run to compare", len(segs))
	}
	if plain != 0 {
		t.Fatalf("%d segments were left uncompressed", plain)
	}

	segN, segDmg, segHash := replayFingerprint(t, seg)
	t.Logf("segmented:  %d records, damage %+v, fingerprint %s", segN, segDmg, segHash[:16])

	if segN != monoN {
		t.Fatalf("record count differs: segmented %d, monolithic %d", segN, monoN)
	}
	if segDmg.Lines != monoDmg.Lines || segDmg.Bytes != monoDmg.Bytes {
		t.Fatalf("damage differs: segmented %+v, monolithic %+v", segDmg, monoDmg)
	}
	if segDmg.Segments != 0 {
		t.Fatalf("%d segment(s) were unreadable: %s", segDmg.Segments, segDmg.SegmentReasons)
	}
	if segHash != monoHash {
		t.Fatal("the replays delivered different records: the fingerprints differ")
	}
	// Every record, in order, field for field: the fingerprint above is the
	// sha256 of each record re-marshalled in delivery order, so equality is
	// equality of the whole run and not of a count.
	t.Logf("EQUAL: %d records, %d damaged lines, identical record fingerprint", segN, segDmg.Lines)
}

// replayFingerprint replays a data directory and folds every record it is handed
// into one sha256, so two replays can be compared without either of them being
// held in memory. It re-marshals each record rather than hashing the line, which
// is what makes it a comparison of the RECORDS the replay produced rather than
// of the bytes it read.
func replayFingerprint(t *testing.T, dir string) (int64, LedgerDamage, string) {
	t.Helper()
	h := sha256.New()
	var n int64
	dmg, err := ScanLedger(dir, func(r Record) {
		n++
		b, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		h.Write(b)
		h.Write([]byte{'\n'})
	})
	if err != nil {
		t.Fatal(err)
	}
	return n, dmg, hex.EncodeToString(h.Sum(nil))
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
}
