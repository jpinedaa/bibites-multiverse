package archive

// The receipt format's own tests. The gate that reads these files is tested
// against the archive in segments_test.go; this file tests the WRITER, because
// the uploader in deploy/coldcopy.sh is the ordinary writer but not the only
// possible one and a second implementation of the format is a second chance to
// get the gate wrong.

import (
	"encoding/json"
	"os"
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
