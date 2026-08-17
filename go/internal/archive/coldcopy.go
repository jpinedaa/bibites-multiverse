package archive

// THE COLD-COPY RECEIPT, and the retirement gate it opens.
//
// archive_rollup_design.md, "Migration, in one restart", step 5:
//
//	"THE FIRST SEGMENT RETIREMENT IS THE POINT OF NO RETURN, not the deployment.
//	 Everything up to it is reversible on the host. Set the off-host copy as a
//	 precondition for it, IN THE CODE: a closed segment is not removed until its
//	 off-host copy is confirmed."
//
// This file is that precondition, and it is a precondition rather than a
// procedure on purpose. A rule that lives in a runbook is a rule that is
// followed until the night it is not; a rule that lives here cannot be skipped
// by an operator in a hurry, because there is no code path that deletes a
// segment without first reading a receipt for it and checking the receipt
// against the bytes on the disk.
//
// WHAT A RECEIPT IS. One JSON object at <data-dir>/segments/<name>.jsonl.gz.receipt,
// written by the uploader (deploy/coldcopy.sh) AFTER it re-read the object's
// checksum back OUT of the object store. It is not a note that an upload was
// attempted; it is a statement that the destination was asked what it holds and
// answered with the same digest the host holds. The distinction is the whole
// value of the file: an upload that returned 200 and wrote nothing durable is
// exactly the failure a retention rule must not act on.
//
// WHAT IT IS CHECKED AGAINST, every time, before a delete:
//
//	the receipt parses, and names THIS segment
//	Bytes equals the size of the local .jsonl.gz on disk
//	SHA256 equals the sha256 of the local .jsonl.gz on disk, RECOMPUTED
//	VerifiedAtMs is set, and RemoteChecksum is not empty
//
// The sha256 is recomputed rather than trusted because the receipt and the
// segment are two files that can drift: a receipt restored from a backup beside
// a segment that was recompressed, a receipt copied by hand from another host, a
// segment truncated by a full disk after its receipt was written. Recomputing
// costs one read of a ~230 MB file ONCE in the segment's life — the pass that
// passes the check is the pass that deletes it.
//
// NO RECEIPT MEANS THE SEGMENT STAYS, FOREVER IF NEED BE. There is no timeout,
// no "old enough to risk it" and no override flag. A cold archive that stopped
// working shows up as ledgerSegmentsAwaitingColdCopy climbing and as disk
// filling — both visible, both recoverable — instead of as a record that is
// gone. That is the correct failure direction for the one file in this system
// that no peer and no relay holds a copy of.
//
// THE RECEIPT OUTLIVES THE SEGMENT. Retirement deletes the .jsonl.gz and KEEPS
// the .receipt: a few hundred bytes that name the destination URI, the digest
// and the day, which is what a restore is planned from and what makes
// "where did the first month go" answerable on the host itself.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ColdCopyReceipt is the uploader's statement that a closed segment exists
// off-host and that the destination was asked to confirm it.
type ColdCopyReceipt struct {
	// Segment is the file name the receipt is for, with its .jsonl.gz suffix —
	// "2026-08-16-0000.jsonl.gz". It is written down rather than inferred from
	// the receipt's own name so a misfiled receipt is a detectable error.
	Segment string `json:"segment"`
	// Bytes and SHA256 are the LOCAL compressed segment as the uploader read it.
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
	// Destination is the full object URI — s3://bucket/prefix/name.jsonl.gz.
	// Endpoint is the S3-compatible endpoint it was written through, empty for
	// AWS S3 itself. Together they are enough to fetch the object back without
	// any other document.
	Destination string `json:"destination"`
	Endpoint    string `json:"endpoint,omitempty"`
	// RemoteChecksum is what the STORE said it holds, read back after the
	// upload: an S3 ChecksumSHA256 (base64 of the raw digest) when the store
	// supports one, otherwise the ETag of a single-part PUT, which is the MD5.
	// RemoteChecksumKind says which, because the two are checked differently and
	// a receipt that does not say is a receipt nobody can re-verify.
	RemoteChecksum     string `json:"remoteChecksum"`
	RemoteChecksumKind string `json:"remoteChecksumKind"`
	// UploadedAtMs is when the PUT returned. VerifiedAtMs is when the store was
	// re-read and agreed — THE FIELD THAT MATTERS. A receipt with an upload time
	// and no verification time is an upload nobody checked, and it does not open
	// the gate.
	UploadedAtMs int64 `json:"uploadedAtMs"`
	VerifiedAtMs int64 `json:"verifiedAtMs"`
	// VerifiedBy names the tool and its version, so a receipt written by a
	// version with a known defect can be found.
	VerifiedBy string `json:"verifiedBy,omitempty"`
}

// receiptPath is where a segment's receipt lives.
func receiptPath(dir, name string) string {
	return filepath.Join(segmentsDir(dir), name+gzSuffix+receiptSuffix)
}

// readReceipt parses one receipt. A receipt that does not parse is an error and
// never an absence: the two mean different things to the gate, and treating a
// corrupt receipt as "no receipt yet" would hide it forever behind an uploader
// that has already done its work.
func readReceipt(path string) (*ColdCopyReceipt, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var r ColdCopyReceipt
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("archive: cold-copy receipt %s does not parse: %w", filepath.Base(path), err)
	}
	return &r, nil
}

// coldCopyVerdict is why a segment may or may not be retired. It is a value
// rather than a bool so the status counters and the log line can say WHICH
// reason, which is the difference between "the uploader has not got to it yet"
// and "the uploader wrote a receipt that does not match the disk".
type coldCopyVerdict int

const (
	coldCopyOK coldCopyVerdict = iota
	// coldCopyMissing: no receipt file. The ordinary state of a segment the
	// uploader has not reached yet.
	coldCopyMissing
	// coldCopyBad: a receipt exists and does not hold up. Loud, and it never
	// opens the gate.
	coldCopyBad
)

// checkColdCopy decides whether a segment's off-host copy is confirmed.
//
// It is called only for a segment that is otherwise eligible — closed,
// compressed, and past the window — so the sha256 it recomputes is paid once per
// segment and never on a pass that was not going to delete anything.
func checkColdCopy(dir string, seg Segment) (coldCopyVerdict, string) {
	path := receiptPath(dir, seg.Name)
	r, err := readReceipt(path)
	if errors.Is(err, os.ErrNotExist) {
		return coldCopyMissing, "no receipt"
	}
	if err != nil {
		return coldCopyBad, err.Error()
	}
	want := seg.Name + gzSuffix
	if r.Segment != want {
		return coldCopyBad, fmt.Sprintf("the receipt names %q, not %q", r.Segment, want)
	}
	if r.VerifiedAtMs <= 0 {
		return coldCopyBad, "the receipt has no verification time: the upload was never read back"
	}
	if strings.TrimSpace(r.Destination) == "" {
		return coldCopyBad, "the receipt names no destination"
	}
	if strings.TrimSpace(r.RemoteChecksum) == "" {
		return coldCopyBad, "the receipt carries no checksum read back from the store"
	}
	info, err := os.Stat(seg.Path)
	if err != nil {
		return coldCopyBad, "the segment could not be stat'd: " + err.Error()
	}
	if r.Bytes != info.Size() {
		return coldCopyBad, fmt.Sprintf(
			"the receipt is for %d bytes and the segment on this disk is %d", r.Bytes, info.Size())
	}
	d, err := digestOfFile(seg.Path)
	if err != nil {
		return coldCopyBad, "the segment could not be hashed: " + err.Error()
	}
	if !strings.EqualFold(d.SHA256, r.SHA256) {
		return coldCopyBad, fmt.Sprintf(
			"the receipt's sha256 is %s and the segment on this disk hashes to %s",
			short12(r.SHA256), short12(d.SHA256))
	}
	return coldCopyOK, ""
}

func short12(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

// WriteReceipt writes a cold-copy receipt atomically. It is exported because the
// uploader in deploy/coldcopy.sh is the ordinary writer but not the only
// possible one — a restore, a one-off copy taken by hand before a retention
// change, or a future in-process uploader all need the same file in the same
// shape, and a second implementation of this format is a second chance to get
// the gate wrong.
func WriteReceipt(dir string, r ColdCopyReceipt) error {
	if !strings.HasSuffix(r.Segment, gzSuffix) {
		return fmt.Errorf("archive: a cold-copy receipt names a compressed segment, got %q", r.Segment)
	}
	name := strings.TrimSuffix(r.Segment, gzSuffix)
	if _, ok := parseSegmentName(name); !ok {
		return fmt.Errorf("archive: %q is not a segment name", name)
	}
	if r.VerifiedAtMs <= 0 {
		return errors.New("archive: a cold-copy receipt with no verification time does not confirm anything")
	}
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	path := receiptPath(dir, name)
	tmp := path + tmpSuffix
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// ReceiptFor reads the receipt for a segment name, for tools and tests.
func ReceiptFor(dir, name string) (*ColdCopyReceipt, error) {
	return readReceipt(receiptPath(dir, name))
}

// receiptAge is how long ago a receipt was verified, for the log line that says
// a segment finally went.
func receiptAge(r *ColdCopyReceipt, now time.Time) time.Duration {
	if r == nil || r.VerifiedAtMs <= 0 {
		return 0
	}
	return now.Sub(time.UnixMilli(r.VerifiedAtMs))
}
