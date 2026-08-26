package archive

// The receipt format's own tests. The gate that reads these files is tested
// against the archive in segments_test.go; this file tests the WRITER, because
// the uploader in deploy/coldcopy.sh is the ordinary writer but not the only
// possible one and a second implementation of the format is a second chance to
// get the gate wrong.

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestWriteReceiptRefusesWhatCannotConfirmAnything: a receipt that names no
// compressed segment, or that was never verified against the store, is not a
// receipt. Refusing it here means the archive never has to decide what a
// half-written one means.
func TestWriteReceiptRefusesWhatCannotConfirmAnything(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(segmentsDir(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	for label, r := range map[string]ColdCopyReceipt{
		"not a compressed segment": {Segment: "2026-08-14-0000.jsonl", VerifiedAtMs: now},
		"not a segment name":       {Segment: "migrations.jsonl.gz", VerifiedAtMs: now},
		"never verified":           {Segment: "2026-08-14-0000.jsonl.gz", VerifiedAtMs: 0},
	} {
		if err := WriteReceipt(dir, r); err == nil {
			t.Fatalf("%s: WriteReceipt accepted it", label)
		}
	}
}

// TestAReceiptRoundTripsThroughTheFileTheUploaderWrites pins the JSON keys. The
// uploader is a shell script writing this object by hand, so a renamed field is
// a gate that silently stops opening — or, worse, one that opens on a file the
// archive misreads.
func TestAReceiptRoundTripsThroughTheFileTheUploaderWrites(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(segmentsDir(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	want := ColdCopyReceipt{
		Segment: "2026-08-14-0000.jsonl.gz", Bytes: 1234, SHA256: strings.Repeat("a", 64),
		Destination:    "s3://bucket/ledger/2026-08-14-0000.jsonl.gz",
		Endpoint:       "https://s3.example",
		RemoteChecksum: "qqqq", RemoteChecksumKind: "sha256",
		UploadedAtMs: now, VerifiedAtMs: now, VerifiedBy: "test",
	}
	if err := WriteReceipt(dir, want); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(receiptPath(dir, "2026-08-14-0000"))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"segment", "bytes", "sha256", "destination", "endpoint",
		"remoteChecksum", "remoteChecksumKind", "uploadedAtMs", "verifiedAtMs", "verifiedBy"} {
		if !strings.Contains(string(b), `"`+key+`"`) {
			t.Fatalf("the receipt file has no %q; deploy/coldcopy.sh writes this shape by hand", key)
		}
	}
	var got ColdCopyReceipt
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("the receipt did not round trip:\n got %+v\nwant %+v", got, want)
	}
	back, err := ReceiptFor(dir, "2026-08-14-0000")
	if err != nil || *back != want {
		t.Fatalf("ReceiptFor returned %+v, %v", back, err)
	}
}

// TestACorruptReceiptIsAnErrorAndNeverAnAbsence: the two mean different things
// to the gate. Treating a corrupt receipt as "not uploaded yet" would hide it
// forever behind an uploader that has already done its work.
func TestACorruptReceiptIsAnErrorAndNeverAnAbsence(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(segmentsDir(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(receiptPath(dir, "2026-08-14-0000"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReceiptFor(dir, "2026-08-14-0000"); err == nil {
		t.Fatal("a receipt that does not parse was read as an absent one")
	}
	verdict, why := checkColdCopy(dir, Segment{Name: "2026-08-14-0000", Compressed: true})
	if verdict != coldCopyBad {
		t.Fatalf("a corrupt receipt gave verdict %v (%s), want coldCopyBad", verdict, why)
	}
}

// writeGz writes a small gzip file for a bundle/metrics receipt to verify against.
func writeGz(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	zw.Write([]byte(body))
	zw.Close()
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestTheSecondNameClassIsAcceptedForGenomeBundles: the receipt gate now confirms
// two kinds of thing, the ledger segment and the genome bundle, through ONE
// verifier. A bundle receipt round-trips, opens the gate for a matching file, and
// stays shut for a mismatched one — the same guarantees the ledger segment has.
func TestTheSecondNameClassIsAcceptedForGenomeBundles(t *testing.T) {
	dir := t.TempDir()
	bd := genomeBundlesDir(dir)
	name := "2026-08-20-0000"
	gz := filepath.Join(bd, name+gzSuffix)
	writeGz(t, gz, `{"hash":"bb8-genome/1:sha256:abc"}`+"\n")

	// A bundle receipt lands beside the bundle, not under segments/, and is
	// accepted by the bundle gate.
	writeGoodBundleReceipt2(t, dir, name)
	if v, why := checkColdCopyBundle(bd, name); v != coldCopyOK {
		t.Fatalf("a good bundle receipt did not open the gate: %v (%s)", v, why)
	}
	got, err := BundleReceiptFor(bd, name)
	if err != nil || got.Segment != name+gzSuffix {
		t.Fatalf("BundleReceiptFor returned %+v, %v", got, err)
	}

	// A receipt whose sha no longer matches the bundle on disk does NOT open it.
	writeGz(t, gz, `{"hash":"bb8-genome/1:sha256:def"}`+"\n") // rewrite: sha changes
	if v, _ := checkColdCopyBundle(bd, name); v != coldCopyBad {
		t.Fatalf("a receipt whose sha does not match the bundle opened the gate: %v", v)
	}

	// WriteBundleReceipt refuses what cannot confirm anything, exactly as the
	// ledger writer does.
	for label, r := range map[string]ColdCopyReceipt{
		"not compressed": {Segment: name + ".jsonl", VerifiedAtMs: 1},
		"not a name":     {Segment: "genomes.jsonl.gz", VerifiedAtMs: 1},
		"never verified": {Segment: name + gzSuffix, VerifiedAtMs: 0},
	} {
		if err := WriteBundleReceipt(bd, r); err == nil {
			t.Fatalf("%s: WriteBundleReceipt accepted it", label)
		}
	}
}

func writeGoodBundleReceipt2(t *testing.T, dir, name string) {
	t.Helper()
	bd := genomeBundlesDir(dir)
	info, err := os.Stat(filepath.Join(bd, name+gzSuffix))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	if err := WriteBundleReceipt(bd, ColdCopyReceipt{
		Segment: name + gzSuffix, Bytes: info.Size(), SHA256: fileSHA(t, filepath.Join(bd, name+gzSuffix)),
		Destination: "s3://b/genomes/" + name + gzSuffix, RemoteChecksum: "x", RemoteChecksumKind: "etag",
		UploadedAtMs: now, VerifiedAtMs: now,
	}); err != nil {
		t.Fatal(err)
	}
}

// TestTheMetricsNameClassIsAcceptedToo: the same verifier confirms a metrics
// segment's off-host copy.
func TestTheMetricsNameClassIsAcceptedToo(t *testing.T) {
	dir := t.TempDir()
	msd := metricsSegmentsDir(dir)
	name := "2026-08-20-0000"
	gz := filepath.Join(msd, name+gzSuffix)
	writeGz(t, gz, `{"generatedAtMs":1}`+"\n")
	info, _ := os.Stat(gz)
	now := time.Now().UnixMilli()
	if err := WriteMetricsReceipt(msd, ColdCopyReceipt{
		Segment: name + gzSuffix, Bytes: info.Size(), SHA256: fileSHA(t, gz),
		Destination: "s3://b/metrics/" + name + gzSuffix, RemoteChecksum: "x", RemoteChecksumKind: "etag",
		UploadedAtMs: now, VerifiedAtMs: now,
	}); err != nil {
		t.Fatal(err)
	}
	if v, why := checkColdCopyMetrics(msd, name); v != coldCopyOK {
		t.Fatalf("a good metrics receipt did not open the gate: %v (%s)", v, why)
	}
}
