package archive

// THE SEGMENTED LEDGER: the record is still one append-only stream, but it is
// stored as an ordered run of files instead of one.
//
// WHAT DID NOT CHANGE, and this is the important half. The live file is still
// <data-dir>/migrations.jsonl, still opened O_APPEND, still one fsync per
// record, still all-or-nothing on a failed write, still never rewritten in
// place. Every rule store.go states about the live file is untouched. What
// changed is that the file no longer holds the whole record: at each UTC day
// boundary it is CLOSED BY RENAME into <data-dir>/segments/<day>-<seq>.jsonl
// and a fresh empty one is opened behind it. A closed segment is immutable from
// the instant of the rename.
//
// WHY, in one line: the ledger grows 1.0-1.3 GB a day and the announced run has
// 89 days left in it. archive_rollup_design.md, "What it costs today, measured",
// has the arithmetic; the short version is that the promise is not funded to the
// end of its own run on the host it was made on. Segmentation is what makes the
// three things that fix it possible at all — compression of what is closed, a
// window on what is kept, and an off-host copy of what leaves.
//
// THE ROTATION RULE IS THE UTC DAY, and it is ONE rule rather than a day-or-size
// pair. A size cap was considered and rejected: the window this feeds is a TIME
// window (30 days, the genome horizon — see "The window" below), and a
// size-capped segment has no single day, so every retirement decision would have
// to read the segment to find out how old it is. Under the day rule the NAME IS
// THE CLOCK: every record in 2026-08-16-0000 has a recordedAt inside
// 2026-08-16 UTC, by construction, because rotation happens on the first append
// whose own recordedAt lands on a later day. Nothing has to be read to know how
// old a segment is, which is what makes retirement a stat call rather than a
// decompression.
//
//	Rotation is on the RECORD's clock, not the wall clock, for the same reason
//	the gap horizon is (eviction.go): recordedAt is the archive's own clock and
//	it is what the segment's name claims about its contents. A record whose day
//	is BEFORE the live segment's day does not rotate — it is appended where it
//	is. A backwards clock step must not produce an out-of-order segment, and the
//	honest cost is that such a record sits in a segment named for a later day,
//	which only ever makes that segment look YOUNGER than it is and so retires
//	LATER. The safe direction for a rule that deletes.
//
// THE SEQUENCE NUMBER exists so that no rename ever overwrites a segment. In
// ordinary running it is always 0000: one day, one rotation. It moves when a
// segment for that day already exists — a clock that went backwards over a
// restart, an operator-forced rotation, a migration whose legacy segment ends on
// the same day the live file then closes.
//
// ORDER IS BY (first day, seq) AND IS PARSED, NEVER LEXICAL. "legacy-" sorts
// after a digit in ASCII, so a lexical sort would replay the migration's legacy
// segment — which holds the OLDEST records in the system — last. Every ordering
// in this file goes through segmentOrder.
//
// B20 RESTATED FOR SEGMENTS. store.go's rule is that a complete line that does
// not parse is SKIPPED AND COUNTED, never stopped at, because the ledger is
// append-only and cannot compact its damage away. Segmentation does not soften
// that; it sharpens it in three places:
//
//  1. A damaged line in a CLOSED segment, plain or gz, is skipped and counted
//     exactly as it is in the live file. A closed segment is never rewritten
//     either, so its damage is just as permanent, and compression cannot heal it
//     — the round-trip check below proves only that the gz decompresses to the
//     SAME BYTES, damage included. That is the correct guarantee: a compressor
//     that "fixed" a line would be inventing a record.
//  2. A TORN TAIL ONLY EVER EXISTS IN THE LIVE FILE. Rotation renames a file
//     that was newline-terminated — the rename happens under the ledger's own
//     lock, between two appends, on a file whose last append either completed or
//     was truncated back. So a closed segment that does not end in a newline was
//     damaged AFTER it was closed, by something outside this process, and its
//     final partial line is counted as DAMAGE rather than dropped as a tail. The
//     live file keeps the old rule, because there the missing newline really is
//     a write that landed short.
//  3. A segment that cannot be read AT ALL — a truncated gzip stream, an
//     unreadable file — is skipped WHOLE and counted in LedgerDamage.Segments,
//     and the replay continues into the next segment. Stopping there would be
//     the 2026-08-08 loss with a new cause: every record behind the bad segment
//     is still perfectly readable and must still be replayed.
//
// COMPRESSION IS VERIFIED BEFORE THE PLAIN FILE IS DELETED, and the check is the
// sha256 of the DECOMPRESSED BYTES against the sha256 of the plain file, plus
// the line count. Anything less is trusting a compressor with the only copy of
// the record.

import (
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"multiverse/internal/fsutil"
)

const (
	// segmentsDirName holds every closed segment and every cold-copy receipt.
	// Its EXISTENCE is the marker that this data directory has been segmented:
	// see migrateMonolithic.
	segmentsDirName = "segments"
	// plainSuffix and gzSuffix are the two forms a closed segment takes. The
	// plain one is what the rename produces; the gz one is what replaces it once
	// the round trip has been checked.
	plainSuffix = ".jsonl"
	gzSuffix    = ".jsonl.gz"
	// tmpSuffix marks a compression in progress. Every one found at startup is
	// deleted: it is a derived artifact whose source is still present.
	tmpSuffix = ".tmp"
	// receiptSuffix is the cold-copy receipt (coldcopy.go). It is the ONLY thing
	// that lets a segment be removed from the host.
	receiptSuffix = ".receipt"
	// legacyPrefix names the one segment the migration produces: the whole
	// pre-segmentation ledger, renamed and not rewritten.
	legacyPrefix = "legacy-"
	// dayLayout is the segment name's day, always UTC.
	dayLayout = "2006-01-02"
	// segmentSeqDigits keeps the sequence number fixed-width so a same-day
	// sort by name agrees with a sort by number.
	segmentSeqDigits = 4
)

// gzipEstimate is the measured compression of a closed ledger at gzip -1:
// 5.75x (archive_rollup_design.md, "What it costs today, measured"), rounded
// DOWN to 5 wherever it is used to estimate an uncompressed size from a
// compressed one, so an estimate is never smaller than the truth.
const gzipEstimate = 5

// ErrPositionRetired is returned by a replay whose start position names a
// segment that is no longer on the host. It is not a corruption: it is a caller
// resuming from a point older than the raw window, and the honest answer is to
// say so rather than to silently replay from somewhere else. The caller's
// recovery is a full replay of whatever segments remain, plus the knowledge that
// its state is older than the window.
var ErrPositionRetired = errors.New("archive: the replay position names a segment that has been retired")

// Segment is one closed segment of the ledger, or the record of one that has
// been retired.
//
// FirstDay and LastDay are UTC midnights and they are read from the NAME, never
// from the contents. For an ordinary segment they are the same day. For the
// migration's legacy segment they are the first and last day of the ledger it
// was made from. EndMs is the instant after which no record in this segment can
// have been written, and it is the only clock retirement uses.
type Segment struct {
	Name     string // "2026-08-16-0000", "legacy-2026-07-01-to-2026-08-16-0000"
	FirstDay time.Time
	LastDay  time.Time
	Seq      int
	Legacy   bool
	// Compressed is true when the file on disk is <Name>.jsonl.gz.
	Compressed bool
	// Path is the file that exists. Empty for a retired segment.
	Path string
	// Bytes is Path's size on disk. 0 for a retired segment.
	Bytes int64
	// Receipt is the parsed cold-copy receipt when one is present.
	Receipt *ColdCopyReceipt
	// Retired is true when the receipt is present and the segment file is not:
	// the bytes are off-host and the host copy has been removed. The receipt is
	// KEPT — it is a few hundred bytes and it is the only record of where the
	// segment went.
	Retired bool
}

// End is the instant after which no record in this segment can have been
// recorded: the midnight that ends its last day.
func (s Segment) End() time.Time { return s.LastDay.AddDate(0, 0, 1) }

// FileName is the segment's file as it is stored, or "" if it is retired.
func (s Segment) FileName() string {
	if s.Retired {
		return ""
	}
	if s.Compressed {
		return s.Name + gzSuffix
	}
	return s.Name + plainSuffix
}

// segmentOrder is the ONE ordering: oldest first day first, then sequence, then
// name so the result is total. Never sort segment names lexically — see the file
// header.
func segmentOrder(a, b Segment) bool {
	if !a.FirstDay.Equal(b.FirstDay) {
		return a.FirstDay.Before(b.FirstDay)
	}
	if a.Seq != b.Seq {
		return a.Seq < b.Seq
	}
	return a.Name < b.Name
}

// segmentsDir is <dir>/segments.
func segmentsDir(dir string) string { return filepath.Join(dir, segmentsDirName) }

// parseSegmentName reads a segment's identity out of its base name — the file
// name with .jsonl or .jsonl.gz already removed. It returns false for anything
// that is not a segment, and the caller LEAVES THOSE ALONE: an unrecognised file
// in the segments directory is somebody else's, and this package does not delete
// what it does not understand.
func parseSegmentName(base string) (Segment, bool) {
	if rest, ok := strings.CutPrefix(base, legacyPrefix); ok {
		// legacy-<first>-to-<last>-<seq>
		first, tail, ok := cutDay(rest)
		if !ok {
			return Segment{}, false
		}
		tail, ok = strings.CutPrefix(tail, "-to-")
		if !ok {
			return Segment{}, false
		}
		last, tail, ok := cutDay(tail)
		if !ok {
			return Segment{}, false
		}
		seq, ok := cutSeq(tail)
		if !ok || last.Before(first) {
			return Segment{}, false
		}
		return Segment{Name: base, FirstDay: first, LastDay: last, Seq: seq, Legacy: true}, true
	}
	day, tail, ok := cutDay(base)
	if !ok {
		return Segment{}, false
	}
	seq, ok := cutSeq(tail)
	if !ok {
		return Segment{}, false
	}
	return Segment{Name: base, FirstDay: day, LastDay: day, Seq: seq}, true
}

// cutDay takes a leading YYYY-MM-DD off s and returns it as a UTC midnight.
func cutDay(s string) (time.Time, string, bool) {
	if len(s) < len(dayLayout) {
		return time.Time{}, "", false
	}
	d, err := time.ParseInLocation(dayLayout, s[:len(dayLayout)], time.UTC)
	if err != nil {
		return time.Time{}, "", false
	}
	return d, s[len(dayLayout):], true
}

// cutSeq takes a trailing -NNNN off s.
func cutSeq(s string) (int, bool) {
	rest, ok := strings.CutPrefix(s, "-")
	if !ok || len(rest) != segmentSeqDigits {
		return 0, false
	}
	n, err := strconv.Atoi(rest)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

// segmentName builds an ordinary segment's name.
func segmentName(day time.Time, seq int) string {
	return fmt.Sprintf("%s-%0*d", day.UTC().Format(dayLayout), segmentSeqDigits, seq)
}

// legacySegmentName builds the migration's one segment name.
func legacySegmentName(first, last time.Time, seq int) string {
	return fmt.Sprintf("%s%s-to-%s-%0*d", legacyPrefix,
		first.UTC().Format(dayLayout), last.UTC().Format(dayLayout), segmentSeqDigits, seq)
}

// segmentDirState is one read of the segments directory: what is there, in
// order, plus everything that is there and should not be.
type segmentDirState struct {
	Segments []Segment
	// Temps are half-written compressions: <name>.jsonl.gz.tmp. Each one's
	// source is still present, so each one is garbage.
	Temps []string
	// BothForms names segments present as BOTH plain and gz — a crash between
	// the compressor's rename and its delete. The plain file is authoritative
	// until the gz has been checked against it.
	BothForms []string
	// Unknown is every other name in the directory. Counted, logged, never
	// touched.
	Unknown []string
}

// readSegmentDir enumerates <dir>/segments without changing anything.
//
// WHEN BOTH FORMS EXIST THE PLAIN ONE WINS, and that is what makes a replay
// correct on a directory nobody has reconciled — `archive list` run on a copy,
// a test, a host that crashed mid-compression and has not restarted. Counting
// both would replay every record in that segment twice.
func readSegmentDir(dir string) (segmentDirState, error) {
	var st segmentDirState
	sd := segmentsDir(dir)
	ents, err := os.ReadDir(sd)
	if errors.Is(err, os.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	type forms struct {
		plain, gz   *Segment
		plainB, gzB int64
		receipt     bool
	}
	byName := map[string]*forms{}
	var names []string
	get := func(n string) *forms {
		f, ok := byName[n]
		if !ok {
			f = &forms{}
			byName[n] = f
			names = append(names, n)
		}
		return f
	}
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() {
			st.Unknown = append(st.Unknown, name)
			continue
		}
		var size int64
		if info, err := e.Info(); err == nil {
			size = info.Size()
		}
		switch {
		case strings.HasSuffix(name, tmpSuffix):
			st.Temps = append(st.Temps, name)
		case strings.HasSuffix(name, gzSuffix+receiptSuffix):
			base := strings.TrimSuffix(name, gzSuffix+receiptSuffix)
			if _, ok := parseSegmentName(base); !ok {
				st.Unknown = append(st.Unknown, name)
				continue
			}
			get(base).receipt = true
		case strings.HasSuffix(name, gzSuffix):
			base := strings.TrimSuffix(name, gzSuffix)
			seg, ok := parseSegmentName(base)
			if !ok {
				st.Unknown = append(st.Unknown, name)
				continue
			}
			seg.Compressed = true
			seg.Path = filepath.Join(sd, name)
			seg.Bytes = size
			f := get(base)
			f.gz, f.gzB = &seg, size
		case strings.HasSuffix(name, plainSuffix):
			base := strings.TrimSuffix(name, plainSuffix)
			seg, ok := parseSegmentName(base)
			if !ok {
				st.Unknown = append(st.Unknown, name)
				continue
			}
			seg.Path = filepath.Join(sd, name)
			seg.Bytes = size
			f := get(base)
			f.plain, f.plainB = &seg, size
		default:
			// coldcopy.jsonl and anything else. Named, never touched.
			st.Unknown = append(st.Unknown, name)
		}
	}
	for _, n := range names {
		f := byName[n]
		var seg Segment
		switch {
		case f.plain != nil:
			seg = *f.plain
			if f.gz != nil {
				st.BothForms = append(st.BothForms, n)
			}
		case f.gz != nil:
			seg = *f.gz
		case f.receipt:
			// Receipt with no file: the segment has been retired. Keep it in the
			// ordered set so the window's start and the retired total stay
			// answerable after the bytes are gone.
			parsed, ok := parseSegmentName(n)
			if !ok {
				continue
			}
			seg = parsed
			seg.Retired = true
			seg.Compressed = true
		default:
			continue
		}
		if f.receipt {
			if r, err := readReceipt(filepath.Join(sd, n+gzSuffix+receiptSuffix)); err == nil {
				seg.Receipt = r
			}
		}
		st.Segments = append(st.Segments, seg)
	}
	sortSegments(st.Segments)
	sort.Strings(st.Temps)
	sort.Strings(st.BothForms)
	sort.Strings(st.Unknown)
	return st, nil
}

// LedgerSegments is the ordered set of closed segments in dir, oldest first,
// including the retired ones (which carry only a receipt). It reads the
// directory and nothing else: no segment is opened and nothing is decompressed.
func LedgerSegments(dir string) ([]Segment, error) {
	st, err := readSegmentDir(dir)
	return st.Segments, err
}

// ------------------------------------------------------------------ migration

// logf is the one logging shape this file needs, so its functions can be called
// from tests and from OpenLedger without carrying a *slog.Logger through every
// signature.
type logf func(msg string, kv ...any)

func mkdirSync(parent, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return fsutil.SyncDir(parent)
}

// ledgerDayBounds reads the first and last usable recordedAt out of a ledger
// file and returns their UTC days. It reads at most a chunk from each end.
func ledgerDayBounds(path string, fallback time.Time) (first, last time.Time) {
	fb := fallback.UTC().Truncate(24 * time.Hour)
	first, last = fb, fb
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return
	}
	// The head: the first record that parses within the first chunk.
	const chunk = 1 << 20
	head := make([]byte, min64(chunk, info.Size()))
	if n, err := f.ReadAt(head, 0); err == nil || (err == io.EOF && n > 0) {
		head = head[:n]
		for _, line := range completeLines(head, true) {
			if ms, ok := recordedAtOf(line); ok {
				first = time.UnixMilli(ms).UTC().Truncate(24 * time.Hour)
				break
			}
		}
	}
	// The tail: the last record that parses within the last chunk.
	at := info.Size() - chunk
	if at < 0 {
		at = 0
	}
	tail := make([]byte, info.Size()-at)
	if n, err := f.ReadAt(tail, at); err == nil || (err == io.EOF && n > 0) {
		tail = tail[:n]
		lines := completeLines(tail, at == 0)
		for i := len(lines) - 1; i >= 0; i-- {
			if ms, ok := recordedAtOf(lines[i]); ok {
				last = time.UnixMilli(ms).UTC().Truncate(24 * time.Hour)
				break
			}
		}
	}
	if last.Before(first) {
		last = first
	}
	return
}

// splitLines returns the COMPLETE newline-terminated lines in b. When keepFirst
// is false the first element is dropped, because a chunk read from the middle of
// a file starts in the middle of a line.
func completeLines(b []byte, keepFirst bool) [][]byte {
	var out [][]byte
	start := 0
	for i := 0; i < len(b); i++ {
		if b[i] == '\n' {
			out = append(out, b[start:i])
			start = i + 1
		}
	}
	if !keepFirst && len(out) > 0 {
		out = out[1:]
	}
	return out
}

// recordedAtOf pulls the archive's own clock off one line without building a
// Record. A line that does not parse has no clock and says so.
func recordedAtOf(line []byte) (int64, bool) {
	var rec Record
	if len(line) == 0 || json.Unmarshal(line, &rec) != nil || rec.RecordedAt <= 0 {
		return 0, false
	}
	return rec.RecordedAt, true
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

// --------------------------------------------------------------- reconciliation

// SegmentReconcile is what a startup reconciliation did. Every field is a crash
// the previous process did not finish, so a non-zero one is worth a log line.
type SegmentReconcile struct {
	// TempsRemoved is half-written compressions deleted. Their source segment is
	// still present, so nothing was lost.
	TempsRemoved int
	// PlainRemoved is segments whose gz was verified against the plain file, so
	// the plain file was finally deleted — the compressor's last step, finished
	// late.
	PlainRemoved int
	// GzRemoved is segments whose gz did NOT match the plain file. The gz is the
	// derived artifact and the plain file is the record, so the gz is deleted
	// and the compression is done again.
	GzRemoved int
	// Unknown is files in the segments directory this package does not
	// recognise. It touches none of them.
	Unknown []string
}

func (r SegmentReconcile) Any() bool {
	return r.TempsRemoved > 0 || r.PlainRemoved > 0 || r.GzRemoved > 0
}

// reconcileSegments makes the segments directory consistent after a crash.
//
// THE MATRIX, and every row loses nothing:
//
//	plain only            the ordinary state of a just-closed segment.
//	                      Left alone; the next maintenance pass compresses it.
//	gz only               the ordinary state of a compressed segment. Left alone.
//	plain + gz            a crash between the compressor's rename and its
//	                      delete. VERIFY the gz against the plain file: if the
//	                      decompressed bytes hash equal, delete the plain file;
//	                      if they do not, delete the GZ and compress again.
//	                      Until it is resolved the replay uses the plain file
//	                      (readSegmentDir), so the record is whole either way.
//	*.tmp                 a crash during a compression. Deleted: the source is
//	                      still there.
//	receipt, no file      a retired segment. Left exactly as it is — the receipt
//	                      is the only record of where the bytes went.
//	anything else         counted, logged, untouched.
func reconcileSegments(dir string, log logf) (SegmentReconcile, error) {
	var rec SegmentReconcile
	st, err := readSegmentDir(dir)
	if err != nil {
		return rec, err
	}
	sd := segmentsDir(dir)
	for _, t := range st.Temps {
		if err := os.Remove(filepath.Join(sd, t)); err == nil {
			rec.TempsRemoved++
		}
	}
	for _, name := range st.BothForms {
		plain := filepath.Join(sd, name+plainSuffix)
		gz := filepath.Join(sd, name+gzSuffix)
		ok, err := gzMatchesPlain(gz, plain)
		if err != nil || !ok {
			if rmErr := os.Remove(gz); rmErr == nil {
				rec.GzRemoved++
			}
			log("archive: a compressed segment did not match its plain file and was DELETED; "+
				"the plain segment is the record and is compressed again",
				"segment", name, "err", err)
			continue
		}
		if rmErr := os.Remove(plain); rmErr == nil {
			rec.PlainRemoved++
		}
	}
	if rec.Any() {
		_ = fsutil.SyncDir(sd)
	}
	rec.Unknown = st.Unknown
	if len(st.Unknown) > 0 {
		log("archive: the segments directory holds files this build does not recognise; "+
			"they were left exactly as they are",
			"files", strings.Join(st.Unknown, ","))
	}
	return rec, nil
}

// --------------------------------------------------------------- compression

// segmentDigest is the pair a compression is checked on: the sha256 of the bytes
// and the number of newline-terminated lines in them.
type segmentDigest struct {
	SHA256 string
	Lines  int64
	Bytes  int64
}

func (d segmentDigest) equal(o segmentDigest) bool {
	return d.SHA256 == o.SHA256 && d.Lines == o.Lines && d.Bytes == o.Bytes
}

// digestOf hashes and counts a stream.
func digestOf(r io.Reader) (segmentDigest, error) {
	var d segmentDigest
	h := sha256.New()
	buf := make([]byte, 1<<20)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			h.Write(buf[:n])
			d.Bytes += int64(n)
			for _, b := range buf[:n] {
				if b == '\n' {
					d.Lines++
				}
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return d, err
		}
	}
	d.SHA256 = hex.EncodeToString(h.Sum(nil))
	return d, nil
}

func digestOfFile(path string) (segmentDigest, error) {
	f, err := os.Open(path)
	if err != nil {
		return segmentDigest{}, err
	}
	defer f.Close()
	return digestOf(bufio.NewReaderSize(f, 1<<20))
}

func digestOfGz(path string) (segmentDigest, error) {
	f, err := os.Open(path)
	if err != nil {
		return segmentDigest{}, err
	}
	defer f.Close()
	zr, err := gzip.NewReader(bufio.NewReaderSize(f, 1<<20))
	if err != nil {
		return segmentDigest{}, err
	}
	defer zr.Close()
	return digestOf(zr)
}

// gzMatchesPlain is the round trip: does gz decompress to exactly the bytes in
// plain? Nothing else is a sufficient check before deleting the only copy.
func gzMatchesPlain(gzPath, plainPath string) (bool, error) {
	want, err := digestOfFile(plainPath)
	if err != nil {
		return false, err
	}
	got, err := digestOfGz(gzPath)
	if err != nil {
		return false, err
	}
	return want.equal(got), nil
}

// compressSegment turns <name>.jsonl into <name>.jsonl.gz, in five steps, and a
// crash at any of them loses nothing.
//
//	1  write <name>.jsonl.gz.tmp and fsync it     crash: the tmp is deleted at
//	                                              startup, the plain file is
//	                                              untouched
//	2  VERIFY the tmp decompresses to the plain    crash: as above
//	   file's exact bytes and line count
//	3  rename tmp -> <name>.jsonl.gz, fsync dir    crash: both forms present;
//	                                              startup verifies and finishes
//	4  delete <name>.jsonl                         crash: as step 3
//	5  fsync dir                                   crash: the delete is either
//	                                              durable or it is repeated
//
// gzip.BestSpeed is the level, and it is the level backup.sh already uses for
// the same file for the same reason: CPU is the scarce thing on a two-vCPU box
// that is also serving a live map, and the measured difference is 5.75x against
// 8.5x — 1.6 GB across a 30-day window, for roughly triple the CPU.
func compressSegment(dir, name string, level int) (plainBytes, gzBytes int64, err error) {
	sd := segmentsDir(dir)
	plain := filepath.Join(sd, name+plainSuffix)
	gz := filepath.Join(sd, name+gzSuffix)
	tmp := gz + tmpSuffix

	src, err := os.Open(plain)
	if err != nil {
		return 0, 0, err
	}
	defer src.Close()

	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return 0, 0, err
	}
	// Any failure from here leaves no tmp behind: the startup sweep would get it,
	// but a pass that fails every minute must not leave a file every minute.
	fail := func(e error) (int64, int64, error) {
		out.Close()
		_ = os.Remove(tmp)
		return 0, 0, e
	}
	zw, err := gzip.NewWriterLevel(out, level)
	if err != nil {
		return fail(err)
	}
	// The plain file's digest is taken on the SAME PASS as the compression, so
	// the file is read once rather than twice: 1.3 GB of a two-vCPU box's disk
	// bandwidth, not 2.6 GB.
	var h hash.Hash = sha256.New()
	var want segmentDigest
	buf := make([]byte, 1<<20)
	for {
		n, rerr := src.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			h.Write(chunk)
			want.Bytes += int64(n)
			for _, b := range chunk {
				if b == '\n' {
					want.Lines++
				}
			}
			if _, werr := zw.Write(chunk); werr != nil {
				return fail(werr)
			}
		}
		if rerr != nil {
			if errors.Is(rerr, io.EOF) {
				break
			}
			return fail(rerr)
		}
	}
	want.SHA256 = hex.EncodeToString(h.Sum(nil))
	if err := zw.Close(); err != nil {
		return fail(err)
	}
	if err := out.Sync(); err != nil {
		return fail(err)
	}
	if info, err := out.Stat(); err == nil {
		gzBytes = info.Size()
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return 0, 0, err
	}
	// Step 2. The tmp is re-read from disk rather than checked against the
	// buffer that produced it: what has to be proved is that the BYTES ON DISK
	// decompress to the record, and a check against memory proves the compressor
	// agreed with itself.
	got, err := digestOfGz(tmp)
	if err != nil {
		_ = os.Remove(tmp)
		return 0, 0, fmt.Errorf("archive: segment %s did not decompress after compression: %w", name, err)
	}
	if !want.equal(got) {
		_ = os.Remove(tmp)
		return 0, 0, fmt.Errorf(
			"archive: segment %s round trip FAILED (plain %d lines/%d bytes/%s, gz %d lines/%d bytes/%s); "+
				"the plain segment is kept and nothing was deleted",
			name, want.Lines, want.Bytes, want.SHA256[:12], got.Lines, got.Bytes, got.SHA256[:12])
	}
	// Step 3.
	if err := os.Rename(tmp, gz); err != nil {
		_ = os.Remove(tmp)
		return 0, 0, err
	}
	if err := fsutil.SyncDir(sd); err != nil {
		return want.Bytes, gzBytes, err
	}
	// Step 4 and 5.
	if err := os.Remove(plain); err != nil {
		return want.Bytes, gzBytes, err
	}
	return want.Bytes, gzBytes, fsutil.SyncDir(sd)
}

// ------------------------------------------------- the archive's own upkeep

// THE WINDOW, AND WHY IT IS ONE NUMBER WITH THE GENOME HORIZON.
//
// archive_rollup_design.md, "The window, and why it is the genome horizon":
// §23's B34 already made the genome store's eviction horizon and the fetch
// queue's retirement horizon ONE number, because two numbers make an archive
// that re-fetches what it just deleted. The raw ledger window is the THIRD
// mechanism reading the same clock, and the argument is the same shape:
// gapPastHorizonLocked never queues a gap whose crossing is older than the
// horizon, so a raw window equal to the horizon holds EXACTLY the crossings
// whose gaps can still be fetched. The fetch queue rebuilds from the window with
// nothing lost. A shorter window abandons live gaps; a longer one buys nothing.
//
// ONE HORIZON, THREE MECHANISMS. So LedgerWindow's default is not a number, it
// is "whatever GenomeHorizon is" — the operator sets one knob and gets a
// consistent archive. --ledger-window exists to break that tie DELIBERATELY,
// and when it does the archive says so at startup, every start.
//
// OFF IS STILL THE DEFAULT, because GenomeHorizon's default is 0 and 0 here
// means the same thing it means there: nothing is ever removed. An archive that
// has not been told a horizon rotates and compresses and never retires, which is
// exactly design option D — the whole announced run in 16-21 GB with no promise
// changed. Turning the horizon on turns the window on with it, and the RECEIPT
// GATE is what makes that safe: with no cold archive configured, no receipt is
// ever written, and no segment is ever removed.

// segState is the segmented ledger's accounting, under Archive.mu with the rest
// of the counters the status view reads.
type segState struct {
	// segments, rawBytes and windowFromMs are refreshed by each maintenance pass
	// and by the pass that runs at Start, so the status page never has to walk a
	// directory on a request.
	segments     int
	rawBytes     int64
	windowFromMs int64
	// awaitingColdCopy is closed, compressed segments that are past the window
	// and have NO usable receipt. It is the number that says the cold archive
	// has stopped working, and it is the number an operator watches instead of
	// watching for a record that is gone.
	awaitingColdCopy int
	// badReceipts is the loud subset of awaitingColdCopy: a receipt exists and
	// does not hold up against the bytes on this disk.
	badReceipts int
	// retired and retiredBytes are what this process removed after confirming
	// an off-host copy. compressed and compressedBytes are what it compressed.
	retired         int
	retiredBytes    int64
	compressed      int
	compressedBytes int64
	lastAt          time.Time
}

// defaultSegmentMaintenanceInterval is how often the pass runs. It is minutes
// rather than seconds because everything it does is either a no-op or expensive:
// on an ordinary day it compresses one segment and retires one, both once.
const defaultSegmentMaintenanceInterval = 5 * time.Minute

// ledgerWindow is the raw window this archive keeps on the host.
//
//	> 0  that duration
//	  0  the genome horizon, which is itself 0 — off — by default
//	< 0  OFF explicitly, whatever the horizon is: rotate and compress, retire
//	     nothing. This is design option D and it is the escape hatch for an
//	     operator who wants the horizon without the window.
func (a *Archive) ledgerWindow() time.Duration {
	if a.cfg.LedgerWindow < 0 {
		return 0
	}
	if a.cfg.LedgerWindow > 0 {
		return a.cfg.LedgerWindow
	}
	return a.cfg.GenomeHorizon
}

// startLedgerMaintenance runs one pass immediately — so the status page has real
// numbers from the first request, and so a segment left uncompressed by a crash
// is compressed at once — and then on a ticker.
func (a *Archive) startLedgerMaintenance() {
	if a.cfg.LedgerMaintenanceInterval < 0 {
		return // tests that drive the pass by hand
	}
	window := a.ledgerWindow()
	switch {
	case window > 0 && a.cfg.LedgerWindow > 0 && a.cfg.GenomeHorizon > 0 &&
		a.cfg.LedgerWindow != a.cfg.GenomeHorizon:
		a.log.Warn("archive: the raw ledger window and the genome horizon are DIFFERENT numbers",
			"ledgerWindow", window.String(), "genomeHorizon", a.cfg.GenomeHorizon.String(),
			"note", "one horizon, three mechanisms (contract-b-m4.md §23 B34, extended to the "+
				"raw ledger). A window SHORTER than the horizon abandons genome gaps that are "+
				"still live; a LONGER one buys nothing. This was set deliberately or it is a "+
				"defect")
	case window > 0:
		a.log.Warn("archive: the raw ledger window is ON; closed segments older than it are "+
			"REMOVED FROM THIS HOST once an off-host copy is confirmed",
			"window", window.String(), "segments", segmentsDir(a.cfg.DataDir),
			"gate", "a segment is removed ONLY if <name>.jsonl.gz.receipt confirms an off-host "+
				"copy that matches the bytes on this disk. No receipt means it stays, forever if "+
				"need be",
			"note", "the answers are kept forever; this is the RAW LINES only")
	default:
		a.log.Info("archive: the raw ledger window is OFF; every closed segment is kept",
			"note", "segments still rotate daily and are compressed once closed, which changes "+
				"no promise: nothing is removed")
	}
	a.ledgerMaintenancePass(time.Now())
	a.wg.Add(1)
	go func() { defer a.wg.Done(); a.ledgerMaintenanceLoop() }()
}

func (a *Archive) ledgerMaintenanceLoop() {
	interval := a.cfg.LedgerMaintenanceInterval
	if interval == 0 {
		interval = defaultSegmentMaintenanceInterval
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-a.ctx.Done():
			return
		case now := <-t.C:
			a.ledgerMaintenancePass(now)
		}
	}
}

// ledgerMaintenancePass is compression, then retirement, then the status
// refresh. It runs OFF THE ARCHIVE'S LOCK — the same rule the eviction pass and
// the history strip follow (§10.1, §21 B21): nothing that reads a directory or
// compresses a gigabyte may hold the lock the relay read loop needs. The lock is
// taken once at the end, to fold the result.
func (a *Archive) ledgerMaintenancePass(now time.Time) {
	dir := a.cfg.DataDir

	// A process that has been up across a midnight with no traffic still holds
	// yesterday's records in the live file, because rotation happens on an
	// append and there has not been one. Close it here so the window and the
	// cold copy are not waiting on the next crossing.
	if name, err := a.rotateIfStale(now); err != nil {
		a.log.Warn("archive: closing a stale live segment failed", "err", err)
	} else if name != "" {
		// THE ROLL-UP'S POSITIONS MOVE WITH THE BYTES. This rotation had no
		// append behind it, so nothing else tells the sidecar that its cursor
		// and its hour index now name a segment rather than the live file
		// (rollup.go, noteRotationLocked).
		a.mu.Lock()
		a.noteRotationLocked(name)
		a.mu.Unlock()
		a.log.Info("archive: closed the live segment on a quiet day boundary", "segment", name)
	}

	var compressed, retired int
	var compressedBytes, retiredBytes int64

	// COMPRESSION FIRST, so a segment is never retired in its plain form: the
	// receipt gate is written against the .jsonl.gz and there is exactly one
	// shape of thing that leaves this host.
	st, err := readSegmentDir(dir)
	if err != nil {
		a.log.Warn("archive: the segments directory could not be read", "err", err, "dir", segmentsDir(dir))
		return
	}
	if !a.cfg.DisableLedgerCompression {
		for _, s := range st.Segments {
			if s.Retired || s.Compressed {
				continue
			}
			plainB, gzB, err := compressSegment(dir, s.Name, gzip.BestSpeed)
			if err != nil {
				a.log.Error("archive: compressing a closed segment failed; the plain segment is KEPT",
					"segment", s.Name, "err", err)
				continue
			}
			compressed++
			compressedBytes += gzB
			ratio := 0.0
			if gzB > 0 {
				ratio = float64(plainB) / float64(gzB)
			}
			a.log.Info("archive: compressed a closed segment and verified the round trip",
				"segment", s.Name, "plainBytes", plainB, "gzBytes", gzB,
				"ratio", fmt.Sprintf("%.2fx", ratio),
				"verified", "sha256 and line count of the DECOMPRESSED bytes against the plain file")
		}
		if compressed > 0 {
			if st, err = readSegmentDir(dir); err != nil {
				return
			}
		}
	}

	// RETIREMENT SECOND, and only ever behind a confirmed off-host copy.
	window := a.ledgerWindow()
	awaiting, bad := 0, 0
	for _, s := range st.Segments {
		if s.Retired || !s.Compressed || s.Path == "" {
			continue
		}
		if window <= 0 || now.Sub(s.End()) <= window {
			continue
		}
		verdict, why := checkColdCopy(dir, s)
		switch verdict {
		case coldCopyMissing:
			awaiting++
			continue
		case coldCopyBad:
			awaiting++
			bad++
			a.log.Error("archive: a cold-copy receipt does not hold up; the segment is KEPT",
				"segment", s.Name, "reason", why,
				"note", "no receipt, no retirement. Fix the receipt or re-upload the segment")
			continue
		}
		bytes := s.Bytes
		if err := os.Remove(s.Path); err != nil {
			a.log.Error("archive: removing a retired segment failed", "segment", s.Name, "err", err)
			awaiting++
			continue
		}
		_ = fsutil.SyncDir(segmentsDir(dir))
		retired++
		retiredBytes += bytes
		r, _ := ReceiptFor(dir, s.Name)
		dest := ""
		if r != nil {
			dest = r.Destination
		}
		a.log.Warn("archive: a closed segment was REMOVED FROM THIS HOST; its confirmed off-host copy is the only one",
			"segment", s.Name+gzSuffix, "bytes", bytes,
			"lastDay", s.LastDay.Format(dayLayout), "window", window.String(),
			"destination", dest,
			"receiptVerifiedAgo", receiptAge(r, now).Truncate(time.Second).String(),
			"restore", "deploy/coldcopy.sh --restore "+s.Name+gzSuffix,
			"note", "the receipt is KEPT beside the gap it leaves; this is the point of no return "+
				"for this day's raw lines")
	}

	// The status refresh, over the directory as it now is.
	if compressed > 0 || retired > 0 {
		if st, err = readSegmentDir(dir); err != nil {
			return
		}
	}
	present, rawBytes, from := 0, int64(0), int64(0)
	for _, s := range st.Segments {
		if s.Retired {
			continue
		}
		present++
		rawBytes += s.Bytes
		if from == 0 {
			from = s.FirstDay.UnixMilli()
		}
	}
	if live := a.ledger; live != nil {
		rawBytes += live.Size()
		if from == 0 {
			from = live.FirstRecordedAtMs()
		}
	}

	a.mu.Lock()
	a.seg.segments = present
	a.seg.rawBytes = rawBytes
	a.seg.windowFromMs = from
	a.seg.awaitingColdCopy = awaiting
	a.seg.badReceipts = bad
	a.seg.retired += retired
	a.seg.retiredBytes += retiredBytes
	a.seg.compressed += compressed
	a.seg.compressedBytes += compressedBytes
	a.seg.lastAt = now
	a.mu.Unlock()
}

// rotateIfStale closes the live file when it holds records from a day that is
// already over. Rotation is normally driven by an append; this covers the map
// that went quiet before midnight.
func (a *Archive) rotateIfStale(now time.Time) (string, error) {
	l := a.ledger
	if l == nil {
		return "", nil
	}
	l.mu.Lock()
	day := l.day
	l.mu.Unlock()
	if day.IsZero() || !now.UTC().Truncate(24*time.Hour).After(day) {
		return "", nil
	}
	return l.RotateNow()
}

// SegmentCounters is the segmented ledger's accounting, for tests and for the
// status view.
func (a *Archive) SegmentCounters() (segments int, rawBytes int64, awaiting, retired int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.seg.segments, a.seg.rawBytes, a.seg.awaitingColdCopy, a.seg.retired
}

// LedgerMaintenanceNow runs one pass synchronously. It is the tests' handle on
// a loop that otherwise runs on a five-minute ticker, and it is what an operator
// tool would call.
func (a *Archive) LedgerMaintenanceNow(now time.Time) { a.ledgerMaintenancePass(now) }

// sortSegments orders a set of segments by the one ordering: oldest first day
// first, then sequence. Exported to the tests and to any later caller that
// builds a set by hand — the ordering rule must exist in exactly one place.
func sortSegments(segs []Segment) {
	sort.Slice(segs, func(i, j int) bool { return segmentOrder(segs[i], segs[j]) })
}
