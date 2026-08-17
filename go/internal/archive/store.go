package archive

// The archive's durable store.
//
// STORE CHOICE, and why: an append-only JSON Lines ledger
// (<data-dir>/migrations.jsonl) beside the content-addressed genome directory
// (<data-dir>/genomes/, bb8.Store). One JSON object per line, fsynced on every
// append, never rewritten in place.
//
// The archive is a recorder, not a query engine (D11, contract-b-m4.md §1:
// "M4 records and reads only"). Three properties decided it over SQLite or a
// key-value store:
//
//  1. It is inspectable with tools that are already on both machines — tail,
//     grep, jq. The first question anyone asks a fresh archive is "did you get
//     it?", and answering that must not require the archive's own binary.
//  2. Append-only with one fsync per record is the same durability discipline
//     the sidecar journal already uses, so there is one crash story in the
//     system rather than two. That was a claim before it was a fact: the journal
//     was made all-or-nothing after the 2026-08-08 full disk and the ledger was
//     not, and the two rules were only brought back together on 2026-08-09. Both
//     halves of the discipline now live here — Append leaves no torn record, and
//     ScanLedger reads past one rather than stopping at it.
//  3. It has no schema to migrate. The lineage graph M6 will want is a
//     different shape from anything M4 could guess, and a ledger replays into
//     whatever shape that turns out to be.
//
// The cost is honest and bounded: reads are a full-file replay, so a query
// surface beyond "list what you recorded" would need an index. That is exactly
// the ambition M4 does not have.

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"multiverse/internal/contractb"
	"multiverse/internal/fsutil"
	"multiverse/internal/wire"
)

// Record types.
const (
	RecordMigration = "MIGRATION"
	RecordAck       = "ACK"
	RecordNack      = "NACK"
	RecordGenome    = "GENOME"
)

// Record is one ledger line.
type Record struct {
	Type string `json:"type"`
	// RecordedAt is the archive's own clock. Two machines have two clocks
	// (m3_considerations.md Risk 5, m4_considerations.md Risk 4), so the archive orders by the one clock it
	// controls and keeps the origin's timestamp beside it as data.
	RecordedAt  int64              `json:"recordedAt"`
	MigrationID string             `json:"migrationId,omitempty"`
	SourcePeer  string             `json:"sourcePeer,omitempty"`
	SourceSlot  int                `json:"sourceSlot,omitempty"`
	DestSlot    int                `json:"destSlot,omitempty"`
	DestPeer    string             `json:"destPeer,omitempty"`
	EntityID    int32              `json:"entityId,omitempty"`
	Kind        string             `json:"kind,omitempty"`
	GameVersion string             `json:"gameVersion,omitempty"`
	Lineage     *contractb.Lineage `json:"lineage,omitempty"`
	// Species is the species-identity block the envelope carried, recorded
	// verbatim (contract-b-m4.md §15, B10). It is a LEDGER FACT, NEVER A
	// RESOLUTION: the archive does not resolve, merge or rewrite a name, because
	// species resolution happens in exactly one place in this system and it is
	// the importing mod. ABSENT IS ABSENT — never "unknown" as a value — and
	// dedup is unchanged, on migrationId alone.
	//
	// Nothing renders it in M4 and that is not a reason to skip it: the ledger
	// could name only hashes until now, the name is the one label a human reads,
	// and adding the field later would leave every migration before that date
	// nameless.
	Species *contractb.Species `json:"species,omitempty"`
	// ExitEdge is the edge the organism left its own world by (added with the
	// two-way lanes of contract-b-m4.md §17, B13). It is recorded for the same
	// reason Species is: the ledger describes what happened, and under D17 the
	// pair (sourceSlot, destSlot) NO LONGER NAMES ONE LANE — on an axis of
	// length 2 the forward and reverse lanes join the same two worlds, and the
	// 3x2 rig's every column is such an axis. Without this field the two lanes
	// of a shuttle are one number.
	//
	// A record written before this field existed has "" here. That is honest and
	// it is handled at display time rather than guessed at: see StatusView.
	ExitEdge   string `json:"exitEdge,omitempty"`
	Timestamp  int64  `json:"timestamp,omitempty"`
	Duplicate  bool   `json:"duplicate,omitempty"`
	Code       string `json:"code,omitempty"`
	Message    string `json:"message,omitempty"`
	GenomeHash string `json:"genomeHash,omitempty"`
	ServedBy   string `json:"servedBy,omitempty"`
}

// Ledger is the append-only record file: the LIVE segment of the record.
//
// Everything in this type is about <data-dir>/migrations.jsonl and only about
// it. The closed segments behind it are immutable and belong to segments.go;
// this type's one dealing with them is rotation, which closes the live file into
// one of them by rename and opens a fresh one.
type Ledger struct {
	mu   sync.Mutex
	dir  string
	path string
	f    *os.File
	// repaired is the size of an unterminated final line dropped when this
	// process opened the file. It is 0 for every ledger closed cleanly.
	repaired int64
	// day is the UTC day of the records currently in the live file, "" when it
	// is empty. It is taken from the RECORDS' own recordedAt and never from the
	// wall clock (segments.go, "THE ROTATION RULE"), and it is what the next
	// append compares itself against to decide whether to rotate.
	day time.Time
	// firstMs is the recordedAt of the live file's first record, 0 when it is
	// empty. It answers "where does the raw window start" on an archive that has
	// no closed segments yet.
	firstMs int64
	// rotations counts what this process closed, for the status view and the
	// tests. rotatedName is the newest one and rotateErr is the last rotation
	// that failed — kept rather than returned, because a rotation failure must
	// not fail the append that triggered it (see Append).
	rotations   int
	rotatedName string
	rotateErr   error
	// off is the live file's length in bytes, MAINTAINED rather than stat()ed:
	// set when the file is opened or rotated and advanced by every successful
	// write. AppendAt returns it, and the roll-up sidecar persists it as its
	// cursor, so it may never be a guess — a cursor one record out would make the
	// next tail replay fold a record twice or skip it entirely.
	off int64
	// migrated names the legacy segment the first start produced, "" otherwise.
	migrated string
	// reconciled is what the startup crash reconciliation did.
	reconciled SegmentReconcile
}

const ledgerName = "migrations.jsonl"

// OpenLedger opens or creates <dir>/migrations.jsonl for appending.
//
// Four things happen here and the ORDER IS THE CRASH STORY:
//
//  1. OPEN and REPAIR THE TORN TAIL, exactly as before, and BEFORE anything
//     else — a partial record that was never durable must not be carried into
//     an immutable segment by the migration below, where nothing could ever
//     repair it again.
//  2. MIGRATE. If this data directory has never been segmented and the repaired
//     ledger is not empty, this is the first start on a whole-file ledger: it is
//     renamed WHOLE into one legacy segment and a fresh live file is opened
//     behind it. Nothing is read, nothing is rewritten, no record moves.
//  3. RECONCILE. Resolve what a crash left in the segments directory — a
//     half-written compression, a segment present in both forms. This MUST
//     happen before anything replays, and the replay is defensive about it
//     anyway (readSegmentDir prefers the plain file), so a directory nobody
//     reconciled still replays each record exactly once.
//  4. READ THE LIVE FILE'S DAY off its last record, so a restart continues the
//     segment it was writing rather than rotating on the first append.
func OpenLedger(dir string) (*Ledger, error) {
	return OpenLedgerLog(dir, func(string, ...any) {})
}

// OpenLedgerLog is OpenLedger with somewhere to say what it did. The migration
// and the reconciliation are once-in-a-lifetime events on a production data
// directory and both deserve a line in the log rather than a silent rename.
func OpenLedgerLog(dir string, log func(msg string, kv ...any)) (*Ledger, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, ledgerName)
	// O_RDWR, not O_WRONLY: opening reads the tail to decide whether the last
	// write finished. O_APPEND still puts every write at the end.
	f, err := os.OpenFile(path, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	l := &Ledger{dir: dir, path: path, f: f}
	if err := l.repairTornTail(); err != nil {
		f.Close()
		return nil, err
	}
	if err := l.migrateIfMonolithic(log); err != nil {
		l.Close()
		return nil, err
	}
	rec, err := reconcileSegments(dir, log)
	if err != nil {
		l.Close()
		return nil, err
	}
	l.reconciled = rec
	if err := fsutil.SyncDir(dir); err != nil {
		l.Close()
		return nil, err
	}
	l.readDay()
	if info, err := l.f.Stat(); err == nil {
		l.off = info.Size()
	}
	return l, nil
}

// migrateIfMonolithic is "Migration, in one restart", step 3, performed on an
// already-open and already-repaired live file: the whole existing ledger becomes
// one legacy segment by rename, and a fresh empty live file is opened behind it.
//
// IT RUNS EXACTLY ONCE in the life of a data directory, and what says so is
// segmentsAreEstablished: the marker FILE, or a segment that is already there.
//
// IT USED TO BE THE EXISTENCE OF <dir>/segments, AND THAT WAS WRONG. The
// reasoning was that a marker which can be lost is a migration that can run
// twice; the flaw is that the directory is not this package's to own. On
// 2026-08-17 the production deployment created <archive-data>/segments before
// the archive ever started, because multiverse-coldcopy.service names it in
// ReadWritePaths and systemd refuses to start a unit whose ReadWritePaths is
// absent. An empty directory then read as "already segmented" and the one-time
// migration was silently skipped: no rename, no legacy segment, and a
// monolithic ledger that the next day boundary closed under a name that does
// not say which days are in it. Nothing was lost and nothing was at risk — the
// failure was entirely one of a marker that meant something other than what it
// was read as.
//
// The marker file is written on BOTH paths that establish the layout, so a
// directory this function has finished with always carries it.
func (l *Ledger) migrateIfMonolithic(log func(string, ...any)) error {
	sd := segmentsDir(l.dir)
	done, err := segmentsAreEstablished(l.dir)
	if err != nil {
		return err
	}
	if done {
		// Already segmented. Write the marker if it is only the presence of a
		// segment that says so — an archive that migrated under the older build,
		// or one whose first rotation happened before this one ever ran.
		return writeMigratedMarker(l.dir)
	}
	info, err := l.f.Stat()
	if err != nil {
		return err
	}
	if info.Size() == 0 {
		// A fresh data directory, or one whose ledger was a torn tail and
		// nothing else. There is nothing to migrate, and the marker below is the
		// last time the question is asked.
		if err := mkdirSync(l.dir, sd); err != nil {
			return err
		}
		return writeMigratedMarker(l.dir)
	}
	first, last := ledgerDayBounds(l.path, info.ModTime())
	if err := l.f.Sync(); err != nil {
		return err
	}
	if err := l.f.Close(); err != nil {
		l.f = nil
		return err
	}
	l.f = nil
	if err := mkdirSync(l.dir, sd); err != nil {
		return err
	}
	name := legacySegmentName(first, last, 0)
	if err := os.Rename(l.path, filepath.Join(sd, name+plainSuffix)); err != nil {
		l.f, _ = os.OpenFile(l.path, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0o644)
		return err
	}
	_ = fsutil.SyncDir(sd)
	f, err := os.OpenFile(l.path, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	l.f = f
	// The marker before the parent sync, so the one fsync that makes the rename
	// durable makes the marker durable with it. A crash between the two leaves a
	// segments directory with a segment in it, which segmentsAreEstablished reads
	// correctly anyway.
	if err := writeMigratedMarker(l.dir); err != nil {
		return err
	}
	if err := fsutil.SyncDir(l.dir); err != nil {
		return err
	}
	l.off = 0
	l.migrated = name
	log("archive: the existing ledger was renamed WHOLE into one legacy segment; nothing was rewritten",
		"segment", name+plainSuffix, "bytes", info.Size(),
		"firstDay", first.Format(dayLayout), "lastDay", last.Format(dayLayout),
		"rollback", "cat/zcat the segments in order followed by migrations.jsonl reproduces the "+
			"file byte for byte",
		"note", "this segment is compressed on the next maintenance pass and retires under the "+
			"ordinary window rule, which needs a cold-copy receipt first")
	return nil
}

// readDay takes the live file's day from its LAST record and its start from its
// FIRST, in two small reads at the two ends.
func (l *Ledger) readDay() {
	info, err := l.f.Stat()
	if err != nil || info.Size() == 0 {
		return
	}
	first, last := ledgerDayBounds(l.path, info.ModTime())
	l.day = last
	if ms, ok := firstRecordedAt(l.path); ok {
		l.firstMs = ms
	} else {
		l.firstMs = first.UnixMilli()
	}
}

// firstRecordedAt is the live file's own start, for ledgerRawWindowFromMs on an
// archive that has not rotated yet.
func firstRecordedAt(path string) (int64, bool) {
	f, err := os.Open(path)
	if err != nil {
		return 0, false
	}
	defer f.Close()
	buf := make([]byte, 1<<16)
	n, err := f.ReadAt(buf, 0)
	if n == 0 && err != nil {
		return 0, false
	}
	for _, line := range completeLines(buf[:n], true) {
		if ms, ok := recordedAtOf(line); ok {
			return ms, true
		}
	}
	return 0, false
}

// Path is the LIVE ledger file. The closed segments are LedgerSegments(dir).
func (l *Ledger) Path() string { return l.path }

// Dir is the data directory the live file and the segments directory share.
func (l *Ledger) Dir() string { return l.dir }

// Migrated names the legacy segment the first start under segmentation
// produced, or "" on every later start.
func (l *Ledger) Migrated() string { return l.migrated }

// Reconciled is what the startup crash reconciliation had to fix.
func (l *Ledger) Reconciled() SegmentReconcile { return l.reconciled }

// Rotations is how many segments this process has closed.
func (l *Ledger) Rotations() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.rotations
}

// FirstRecordedAtMs is the live file's first record, 0 when it is empty.
func (l *Ledger) FirstRecordedAtMs() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.firstMs
}

// HintBytes is what the duplicate set sizes itself from: the live file plus an
// UNCOMPRESSED estimate of the newest closed segments, bounded to the two that
// can hold a key inside the 48 h duplicate window.
//
// Size() was that estimate when the ledger was one file. Under segmentation the
// live file is at most one day, so Size() alone would under-size the map at
// every start and pay for it in rehashes through the whole replay. The gz
// estimate is the measured 5.75x rounded DOWN to 5, so the hint is never
// smaller than the truth.
func (l *Ledger) HintBytes() int64 {
	total := l.Size()
	segs, err := LedgerSegments(l.dir)
	if err != nil {
		return total
	}
	for i, n := len(segs)-1, 0; i >= 0 && n < 2; i-- {
		s := segs[i]
		if s.Retired {
			continue
		}
		n++
		if s.Compressed {
			total += s.Bytes * gzipEstimate
			continue
		}
		total += s.Bytes
	}
	return total
}

// Size is the ledger's length in bytes, after any torn tail this open dropped.
// It is one stat call and never a read, and it exists so a caller can SIZE what
// the replay is about to build without walking the file first: the ledger is
// one record per line at a measured average width, so its length is the only
// cheap estimate of its record count there is. A caller that needs the exact
// count must scan, and ScanLedger is that scan.
func (l *Ledger) Size() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	info, err := l.f.Stat()
	if err != nil {
		return 0
	}
	return info.Size()
}

// Repaired reports the bytes of an unterminated final line this open dropped.
// It is 0 for a ledger whose last write completed. Anything else means the
// previous process died mid-append — the record was never durable and its
// caller was told the write failed, so nothing that was ever promised is lost;
// the caller SHOULD still say so out loud.
func (l *Ledger) Repaired() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.repaired
}

// repairTornTail truncates away a final line with no newline on it.
//
// Append writes the record and its newline as ONE buffer, so a ledger that does
// not end in a newline ended in a write that landed short. Two things follow.
// The record is not durable and never was: its caller got an error and, under
// §5, did not ACK. And the next append would land straight behind those bytes
// and SPLICE two records into one unparsable line — which is precisely the
// 2026-08-08 damage this file still carries at line 874163.
//
// Append truncates its own failed write back, so this is the belt to that
// braces: it catches a process killed between the short write and the
// truncation. The live archive holds the ledger open for the life of the
// process, so this only ever runs at startup.
func (l *Ledger) repairTornTail() error {
	info, err := l.f.Stat()
	if err != nil {
		return err
	}
	size := info.Size()
	if size == 0 {
		return nil
	}
	const chunk = 1 << 16
	buf := make([]byte, chunk)
	for at := size; at > 0; {
		n := int64(chunk)
		if at < n {
			n = at
		}
		at -= n
		if _, err := l.f.ReadAt(buf[:n], at); err != nil {
			return err
		}
		if i := bytes.LastIndexByte(buf[:n], '\n'); i >= 0 {
			end := at + int64(i) + 1
			if end == size {
				return nil // the last write finished; nothing to repair
			}
			if err := l.f.Truncate(end); err != nil {
				return err
			}
			if err := l.f.Sync(); err != nil {
				return err
			}
			l.repaired = size - end
			return nil
		}
		if size-at > maxLedgerLine {
			// No newline within a record's worth of bytes. This is not a torn
			// tail, it is not this ledger's shape, and guessing where to cut it
			// could throw away everything the archive has. Refuse to open.
			return fmt.Errorf("archive: no line ending in the last %d bytes of %s", size-at, l.path)
		}
	}
	// Not one newline in the whole file: a single torn line, worth nothing.
	if err := l.f.Truncate(0); err != nil {
		return err
	}
	if err := l.f.Sync(); err != nil {
		return err
	}
	l.repaired = size
	return nil
}

// Append writes one record and flushes it before returning.
//
// A FAILED APPEND MUST LEAVE NO BYTES BEHIND. This is the sidecar journal's
// rule (internal/journal, append) and it is here for the same reason and at the
// same cost. When the volume filled on 2026-08-08 a write landed SHORT: some of
// the record reached the file, the rest did not, the call returned an error, and
// the archive correctly recorded nothing. But the half-record stayed, the next
// successful append — seven hours later, once the disk had room — wrote a whole
// record straight behind it, and the two became one unparsable line in the
// MIDDLE of the ledger.
//
// Replay used to stop at that line, so the archive's whole history behind it
// (390,005 records and growing) was one restart away from vanishing: totals,
// lane counters and species aggregates would have silently reverted to 01:21.
// The journal learned this in f72dd14 and the ledger did not, which is why it
// is spelled out twice.
//
// So the write is all-or-nothing: on any error the file is truncated back to the
// length it had before the attempt. The caller still gets the error and still
// must not ACK; what changes is that the failure costs this record only.
// ROTATION HAPPENS HERE, BEFORE THE WRITE, and it is the only thing this method
// gained under segmentation. If this record's own day is after the day the live
// file holds, the live file is closed into a segment by rename and a fresh one
// is opened, and only then is the record appended — so the record lands in the
// file named for ITS day and the segment name's promise holds by construction.
//
// A ROTATION THAT FAILS DOES NOT FAIL THE APPEND. The record is what matters and
// the file layout is not: on a rotation error the record is written into the
// current live file, which then simply spans two days and is named for the later
// one at its next successful rotation. Refusing the append instead would turn a
// full-disk rename failure into a refused crossing, which is the wrong trade in
// exactly the incident this whole design comes from.
func (l *Ledger) Append(rec Record) error {
	_, _, err := l.AppendAt(rec)
	return err
}

// AppendAt is Append that says WHERE the record landed: the offset of the byte
// just past it in the LIVE file, and the name of the segment this append's
// rotation closed, "" when it did not rotate.
//
// THE ROLL-UP SIDECAR IS WHY IT EXISTS. Its cursor has to be a position rather
// than a count, so that a restart seeks instead of parsing (segments.go,
// LedgerPos), and a position is only exact if the fold that produced it and the
// write that placed it move together. The caller therefore folds under its own
// lock with this offset in hand, which is also why every live append is now
// APPEND FIRST, FOLD SECOND: a fold ahead of the file would be re-folded by the
// next tail replay and every counter it touched would double.
//
// The rotated name matters for the same reason. The bytes that were in the live
// file are in that segment now, at the same offsets, so any position naming the
// live file has to be re-pointed at it — the caller does that for its cursor and
// for its hour index.
func (l *Ledger) AppendAt(rec Record) (offset int64, rotatedInto string, err error) {
	b, err := json.Marshal(rec)
	if err != nil {
		return 0, "", err
	}
	b = append(b, '\n')
	l.mu.Lock()
	defer l.mu.Unlock()
	rotations := l.rotations
	l.maybeRotateLocked(rec.RecordedAt)
	if l.rotations != rotations {
		rotatedInto = l.rotatedName
	}
	before, statErr := l.size()
	n, err := l.f.Write(b)
	if err != nil {
		l.truncateBack(before, statErr)
		return 0, rotatedInto, err
	}
	if n != len(b) {
		// io.Writer permits a short write only with a non-nil error, but the
		// cost of trusting that here is every record written after this one.
		l.truncateBack(before, statErr)
		return 0, rotatedInto, fmt.Errorf("archive: short ledger write, %d of %d bytes", n, len(b))
	}
	if err := l.f.Sync(); err != nil {
		return 0, rotatedInto, err
	}
	// The live file's day is only moved by a record that reached the disk. A
	// failed append must not advance it, or a rotation would name a segment for
	// a day whose only record was never durable.
	if rec.RecordedAt > 0 {
		if day := dayOfMs(rec.RecordedAt); l.day.IsZero() || day.After(l.day) {
			l.day = day
		}
		if l.firstMs == 0 || rec.RecordedAt < l.firstMs {
			l.firstMs = rec.RecordedAt
		}
	}
	l.off += int64(n)
	return l.off, rotatedInto, nil
}

// dayOfMs is one record's UTC day.
func dayOfMs(ms int64) time.Time { return time.UnixMilli(ms).UTC().Truncate(24 * time.Hour) }

// maybeRotateLocked closes the live file into a segment when the incoming
// record belongs to a later UTC day than the records already in it.
//
// A record whose day is BEFORE the live day does not rotate — see segments.go's
// header. A record with no recordedAt at all does not rotate either: a record
// with no clock cannot claim a day.
func (l *Ledger) maybeRotateLocked(recordedAt int64) {
	if recordedAt <= 0 || l.day.IsZero() {
		return
	}
	if !dayOfMs(recordedAt).After(l.day) {
		return
	}
	if err := l.rotateLocked(l.day); err != nil {
		// Deliberately not fatal — see Append's comment. The next append tries
		// again, so a transient failure costs one oversized segment and nothing
		// else.
		l.rotateErr = err
	}
}

// RotateNow closes the live file into a segment immediately, whatever day it
// holds. It exists for the tests, for the maintenance pass's "the process has
// been up across a midnight with no traffic" case, and for an operator who wants
// a segment closed before taking a copy of it.
//
// A rotation of an EMPTY live file is a no-op: an empty segment is a file whose
// name makes a claim about a day it holds no record for.
func (l *Ledger) RotateNow() (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	day := l.day
	if day.IsZero() {
		return "", nil
	}
	return l.rotatedName, l.rotateLocked(day)
}

// rotateLocked is the rotation, in four steps. A crash at any of them loses
// nothing:
//
//	1 fsync and close the live file        crash: the live file is intact and
//	                                       still the live file; the next append
//	                                       rotates again
//	2 rename it into segments/<day>-<seq>  crash: rename is atomic — the bytes
//	                                       are at one name or the other, never
//	                                       at neither
//	3 fsync both directories               crash: as 2; the rename is redone or
//	                                       already durable
//	4 open a fresh empty live file         crash: the live file is absent, and
//	                                       OpenLedger creates it empty
//
// The sequence number is chosen by looking at what is already in the directory
// and the target is re-checked immediately before the rename, so no rotation
// ever renames over a segment.
func (l *Ledger) rotateLocked(day time.Time) error {
	if info, err := l.f.Stat(); err != nil || info.Size() == 0 {
		// Nothing to close. Adopt the new day silently.
		l.day = time.Time{}
		l.firstMs = 0
		l.off = 0
		return err
	}
	sd := segmentsDir(l.dir)
	if err := os.MkdirAll(sd, 0o755); err != nil {
		return err
	}
	name, target, err := freeSegmentName(sd, day)
	if err != nil {
		return err
	}
	if err := l.f.Sync(); err != nil {
		return err
	}
	if err := l.f.Close(); err != nil {
		l.f = nil
		return err
	}
	l.f = nil
	if err := os.Rename(l.path, target); err != nil {
		// Reopen so the archive keeps recording: a rename that failed must not
		// cost the next crossing.
		l.f, _ = os.OpenFile(l.path, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0o644)
		return err
	}
	_ = fsutil.SyncDir(sd)
	f, err := os.OpenFile(l.path, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	l.f = f
	if err := fsutil.SyncDir(l.dir); err != nil {
		return err
	}
	l.day = time.Time{}
	l.firstMs = 0
	l.off = 0
	l.rotations++
	l.rotatedName = name
	l.rotateErr = nil
	return nil
}

// freeSegmentName picks the lowest sequence number for day that is not already
// taken, in either form, and returns the name and the full path to rename to.
func freeSegmentName(sd string, day time.Time) (string, string, error) {
	for seq := 0; seq < 10000; seq++ {
		name := segmentName(day, seq)
		plain := filepath.Join(sd, name+plainSuffix)
		gz := filepath.Join(sd, name+gzSuffix)
		if _, err := os.Stat(plain); err == nil {
			continue
		}
		if _, err := os.Stat(gz); err == nil {
			continue
		}
		return name, plain, nil
	}
	return "", "", fmt.Errorf("archive: %s already holds 10000 segments for %s",
		sd, day.Format(dayLayout))
}

// truncateBack undoes a failed append. Best effort: if it fails too, the next
// OpenLedger repairs the torn tail, which is correct as long as nothing is
// written behind it — and nothing will be, because the caller is about to see
// an error.
func (l *Ledger) truncateBack(before int64, statErr error) {
	if statErr != nil {
		return
	}
	_ = l.f.Truncate(before)
	_ = l.f.Sync()
}

// size is the ledger's length read from the open handle rather than the path.
func (l *Ledger) size() (int64, error) {
	info, err := l.f.Stat()
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// Close flushes and closes the ledger.
func (l *Ledger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return nil
	}
	if err := l.f.Sync(); err != nil {
		l.f.Close()
		l.f = nil
		return err
	}
	err := l.f.Close()
	l.f = nil
	return err
}

// LedgerDamage is what a replay refused to trust. It is the zero value for
// every healthy ledger and a reason to shout for any other.
type LedgerDamage struct {
	// Lines and Bytes count COMPLETE lines — newline-terminated — that did not
	// parse. Each one is skipped, so the count is what the replay lost and not,
	// as it once was, the point where it gave up.
	Lines int
	Bytes int64
	// TornTail is the size of an unterminated final line that was dropped. It
	// costs nothing that was ever durable (see repairTornTail) and is reported
	// apart from Lines for exactly that reason. Reading a ledger the archive
	// holds open sees 0 here, because OpenLedger has already repaired it.
	//
	// IT IS ONLY EVER THE LIVE FILE'S. A closed segment is renamed out of a file
	// that was newline-terminated, so an unterminated final line in one is
	// damage that arrived after the segment closed, and it is counted in Lines.
	// See segments.go, "B20 RESTATED FOR SEGMENTS".
	TornTail int64
	// Segments counts CLOSED SEGMENTS that could not be read at all — a
	// truncated gzip stream, an unreadable file — and SegmentBytes is what they
	// occupied. Each one is skipped WHOLE and the replay carries on into the
	// next: every record behind a bad segment is still readable and stopping
	// there would be the 2026-08-08 loss with a new cause. SegmentReasons names
	// which and why, because "one segment is unreadable" and "which day is gone"
	// are different questions and only the second one can be acted on.
	//
	// It is a joined string rather than a slice so that LedgerDamage STAYS
	// COMPARABLE: every test in this package asserts a clean replay with
	// `damage != (LedgerDamage{})`, and that is a better property to keep than a
	// tidier type for a field that is empty on every healthy archive.
	Segments       int
	SegmentBytes   int64
	SegmentReasons string
}

// Any is true when the replay could not read something a record was in — a
// complete line, or a whole segment.
func (d LedgerDamage) Any() bool { return d.Lines > 0 || d.Segments > 0 }

// add folds one source's damage into a running total.
func (d *LedgerDamage) add(o LedgerDamage) {
	d.Lines += o.Lines
	d.Bytes += o.Bytes
	d.TornTail += o.TornTail
	d.Segments += o.Segments
	d.SegmentBytes += o.SegmentBytes
	if o.SegmentReasons != "" {
		if d.SegmentReasons != "" {
			d.SegmentReasons += "; "
		}
		d.SegmentReasons += o.SegmentReasons
	}
}

// LedgerPos is a resumable position in the segmented ledger, and it is the
// interface the roll-up sidecar resumes on.
//
// Segment is a segment's NAME — "2026-08-16-0000" — or "" for the live file.
// Offset is a byte offset into that segment's UNCOMPRESSED bytes, so the same
// position means the same record whether the segment is still plain or has since
// been compressed; a compression does not invalidate a saved position, which is
// the whole reason the offset is not a file offset. Record is how many records
// had been delivered before this point, and it is exactly Archive.recordCount —
// a damaged line is skipped and does NOT advance it, because it never became a
// record.
//
// THE OFFSET ONLY EVER ADVANCES OVER A COMPLETE, NEWLINE-TERMINATED LINE. An
// unterminated tail in the live file is not counted into it, because the next
// process truncates those bytes away (repairTornTail) and a position that
// included them would start the next replay in the middle of a record.
type LedgerPos struct {
	Segment string `json:"segment,omitempty"`
	Offset  int64  `json:"offset"`
	Record  int64  `json:"record"`
}

// IsStart is the zero position: replay everything.
func (p LedgerPos) IsStart() bool { return p.Segment == "" && p.Offset == 0 && p.Record == 0 }

// ledgerSource is one file in the replay's ordered run: a closed segment, or
// the live file (Name "").
type ledgerSource struct {
	Name       string
	Path       string
	Compressed bool
	Bytes      int64
	Live       bool
}

// ledgerSources is the ordered run a replay walks: every present closed segment,
// oldest first, then the live file. Retired segments are not in it — their bytes
// are off-host — and that is the honest shape: the run has a start, and
// LedgerRawWindowFrom is where it is.
func ledgerSources(dir string) ([]ledgerSource, error) {
	segs, err := LedgerSegments(dir)
	if err != nil {
		return nil, err
	}
	var out []ledgerSource
	for _, s := range segs {
		if s.Retired || s.Path == "" {
			continue
		}
		out = append(out, ledgerSource{
			Name: s.Name, Path: s.Path, Compressed: s.Compressed, Bytes: s.Bytes,
		})
	}
	live := filepath.Join(dir, ledgerName)
	if info, err := os.Stat(live); err == nil {
		out = append(out, ledgerSource{Path: live, Bytes: info.Size(), Live: true})
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	} else {
		// A data directory with segments and no live file is what a crash
		// between a rotation's rename and its reopen leaves. The replay is still
		// complete; it just ends at the last segment.
		out = append(out, ledgerSource{Path: live, Live: true})
	}
	return out, nil
}

// LedgerSources names the files a replay would read, in order, for tools and
// tests. The live file is last and its name is "".
func LedgerSources(dir string) ([]string, error) {
	srcs, err := ledgerSources(dir)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(srcs))
	for _, s := range srcs {
		out = append(out, s.Name)
	}
	return out, nil
}

// ScanLedger replays every record in dir's ledger, in write order, hands each
// one to fn, and reports what it could not read.
//
// IT HANDS THE RECORD OVER AND FORGETS IT, and that is the whole point: a replay
// costs one record of memory rather than a ledger of it. Materialising the file
// first — which is what this function did until 2026-08-12, and what ReadLedger
// still does for the callers that genuinely want a slice — put the ENTIRE ledger
// in one live []Record while New walked it, so the peak was the file by
// construction and no collector setting could get under it. The reference
// 8,156,868-record replay in wp3_hosting_options.md, "Archive replay memory",
// measured 1,030–1,286 B of peak RSS per record materialising against
// **184 B streaming** — 5.6–7.0× lower, about 1.1× the state the replay
// actually retains, with no GOMEMLIMIT and no measurable wall-clock cost. That
// is the difference between an archive that can restart on an 8 GB box for
// three weeks and one that can restart for six months.
//
// fn is called once per record, in file order, on the caller's goroutine. It
// MUST NOT retain the Record beyond what it needs; New keeps the aggregates and
// drops the record, which is why the peak is flat.
//
// A LINE THAT DOES NOT PARSE IS SKIPPED, NEVER STOPPED AT. Until 2026-08-09 this
// loop broke at the first such line, silently: the 776-byte record the full disk
// spliced together at line 874163 made the 390,005 records behind it — every
// crossing recorded in the following eight hours — unreachable on replay, so the
// next restart would have reverted the archive's totals, lane counters and
// species aggregates to 01:21 without a word. The ledger is append-only and
// never rewritten (see the file header), so unlike the sidecar journal it cannot
// compact its damage away: it will carry that line for the life of the deployment
// and must read straight past it, every time, out loud.
//
// This is where the archive DEPARTS from internal/journal, deliberately. The
// journal stops at damage and truncates the file to the last good record,
// because Open compacts it from the in-memory state map immediately afterwards
// and loses nothing. The ledger has no such state to rebuild from — it IS the
// state — so it skips the line and keeps every record behind it, and Discarded
// therefore counts the damaged lines themselves rather than the history behind
// them, which is no longer thrown away.
//
// Skipping is safe for the replay in New: records are applied in file order and
// order is preserved, dedup is keyed on the ids the records carry rather than on
// position, and a skipped line simply never contributes. The one honest cost is
// that a skipped record is not in the dedup set, so if a peer re-sends exactly
// that migration or NACK it is recorded a second time.
// UNDER SEGMENTATION IT IS A RUN OF FILES, and the run is the record. Every
// closed segment in order, oldest first, then the live file. A segmented replay
// delivers exactly the records a monolithic replay of the same lines would, in
// the same order, with the same damage accounting — which is a testable claim
// and is tested (segments_test.go, TestSegmentedReplayEqualsMonolithic, and the
// real-ledger check beside it).
func ScanLedger(dir string, fn func(Record)) (LedgerDamage, error) {
	dmg, _, err := ScanLedgerFrom(dir, LedgerPos{}, func(rec Record, _ LedgerPos) { fn(rec) })
	return dmg, err
}

// ScanLedgerFrom is ScanLedger resumed: it replays from `from` — the zero value
// being the very beginning — and hands fn each record together with the position
// IMMEDIATELY AFTER it, which is the position to persist in order to resume
// there next time.
//
// THIS IS THE SOURCE THE ROLL-UP SIDECAR TAILS. The sidecar saves the position
// of the last record it folded; at the next start it loads its own state and
// replays from that position, so a restart costs the sidecar plus the tail
// rather than the whole record. The position survives a compression, because the
// offset is into the segment's UNCOMPRESSED bytes.
//
// A position naming a segment that is no longer on the host returns
// ErrPositionRetired and delivers nothing. That is a caller whose state is older
// than the raw window, and the only honest answers are "replay what is left and
// know your state has a hole" or "fetch the segment back from the cold archive".
// Guessing a different start would silently drop records.
//
// DAMAGE COUNTS ONLY WHAT THIS CALL REPLAYED. A resumed replay does not re-count
// the damaged lines an earlier one already accounted for.
func ScanLedgerFrom(dir string, from LedgerPos, fn func(Record, LedgerPos)) (LedgerDamage, LedgerPos, error) {
	return scanLedgerFromFloor(dir, from, from,
		func(rec Record, pos LedgerPos, _ bool) { fn(rec, pos) })
}

// scanLedgerFromFloor is ScanLedgerFrom with a FLOOR: every record and every
// damaged line is reported with whether it lies PAST the floor, and damage at or
// before it is not counted at all.
//
// The roll-up sidecar is the caller that needs this, and the floor is its
// cursor. A restart's raw scan reaches further back than that cursor — back to
// the start of the duplicate window, which the guard cannot inherit from any
// file (dedup.go) — so it re-reads records the aggregates already hold. The
// bool is what tells the two apart: false means "the sidecar has folded this,
// rebuild the guard from it and fold nothing", true means "this is new".
//
// The same bool governs the damage count, because ledgerSkippedLines is a
// persisted aggregate like every other counter here: without a floor every
// restart would count the ledger's permanent damage a second time and the number
// would climb with uptime rather than with damage.
//
// THE FLOOR IS A POSITION AND NOT A COUNT, and that is the whole reason it
// works. A damaged line produces no record and so has no record number to be
// compared on; and record numbers are relative to WHAT IS PRESENT, so they shift
// under a segment retirement while a position does not. The comparison is exact:
// a line's bytes either end at or before the floor or begin after it, and a floor
// is only ever written at a record boundary.
//
// A FLOOR NAMING A SOURCE THAT IS NOT PRESENT PUTS NOTHING PAST IT. That is the
// conservative direction and it is chosen deliberately: re-folding records the
// aggregates already hold doubles every counter, which is the one failure that
// looks like data. The caller that knows its floor is older than the whole raw
// window says so by passing the zero position instead (rollup.go, resolveFloor).
func scanLedgerFromFloor(dir string, from, floor LedgerPos,
	fn func(Record, LedgerPos, bool)) (LedgerDamage, LedgerPos, error) {

	var dmg LedgerDamage
	pos := from
	srcs, err := ledgerSources(dir)
	if err != nil {
		return dmg, pos, err
	}
	start := 0
	if !from.IsStart() {
		start = -1
		for i, s := range srcs {
			if s.Name == from.Segment {
				start = i
				break
			}
		}
		if start < 0 {
			return dmg, pos, fmt.Errorf("%w: %q", ErrPositionRetired, from.Segment)
		}
	}
	// Where the floor falls in the same ordered run.
	floorAt := 0
	if !floor.IsStart() {
		floorAt = len(srcs)
		for i, s := range srcs {
			if s.Name == floor.Segment {
				floorAt = i
				break
			}
		}
	}
	for i := start; i < len(srcs); i++ {
		s := srcs[i]
		skip := int64(0)
		if i == start {
			// pos already carries from.Offset and from.Record. The NAME still
			// has to be adopted: a zero start position names the live file ("")
			// while the first source is usually a segment, and a position handed
			// back to the caller from inside the first source must name the file
			// it is actually in.
			skip = from.Offset
		} else {
			pos.Offset = 0
		}
		pos.Segment = s.Name
		// pastFrom is the offset inside THIS source past which a line is new to
		// the caller: everything for a source after the floor's, nothing for one
		// before it, and the floor's own offset for the source it falls in.
		pastFrom := int64(0)
		switch {
		case i < floorAt:
			pastFrom = maxLedgerOffset
		case i == floorAt:
			pastFrom = floor.Offset
		}
		d, err := scanOneSource(s, skip, 0, pastFrom, &pos, fn)
		dmg.add(d)
		if err != nil {
			return dmg, pos, err
		}
	}
	return dmg, pos, nil
}

// maxLedgerOffset is "no offset in this file is past the floor". It is a
// sentinel rather than a flag so the comparison in scanOneSource stays one
// expression.
const maxLedgerOffset int64 = 1<<62 - 1

// scanOneSource replays one segment or the live file, from `skip` bytes into its
// uncompressed content.
//
// A CLOSED SEGMENT THAT WILL NOT OPEN OR WILL NOT DECOMPRESS IS SKIPPED WHOLE
// AND COUNTED, and the caller carries on into the next one. The live file is
// different: a live file that will not open is the archive's own state and the
// error is returned.
// limit, when non-zero, stops the scan as soon as pos.Record reaches it. It is
// how LedgerPositionOfRecord finds the position of the Nth record without
// reading past it.
//
// pastFrom is the offset inside THIS source past which a line is NEW to the
// caller; see scanLedgerFromFloor. 0 makes everything new, which is what every
// caller but the roll-up's tail replay wants.
func scanOneSource(s ledgerSource, skip, limit, pastFrom int64, pos *LedgerPos, fn func(Record, LedgerPos, bool)) (LedgerDamage, error) {
	var dmg LedgerDamage
	f, err := os.Open(s.Path)
	if errors.Is(err, os.ErrNotExist) && s.Live {
		return dmg, nil
	}
	if err != nil {
		if s.Live {
			return dmg, err
		}
		return segmentUnreadable(s, err), nil
	}
	defer f.Close()

	var src io.Reader = f
	if s.Compressed {
		zr, err := gzip.NewReader(bufio.NewReaderSize(f, 1<<20))
		if err != nil {
			return segmentUnreadable(s, err), nil
		}
		defer zr.Close()
		src = zr
	} else if skip > 0 {
		// A plain segment or the live file seeks. This is the O(1) half of the
		// resume and the reason a hot sidecar restart is seconds rather than
		// minutes.
		if _, err := f.Seek(skip, io.SeekStart); err != nil {
			return dmg, err
		}
		skip = 0
	}
	r := bufio.NewReaderSize(src, 1<<20)
	if skip > 0 {
		// A gzip stream has no seek: the only way to a byte offset is through
		// the bytes. It is still far cheaper than parsing them.
		if _, err := io.CopyN(io.Discard, r, skip); err != nil {
			if s.Live {
				return dmg, err
			}
			return segmentUnreadable(s, err), nil
		}
	}
	for {
		line, err := readLine(r)
		if len(line) > 0 {
			switch {
			case errors.Is(err, io.EOF):
				if s.Live {
					// No newline on the last line, so the write that put it
					// there landed short and the record was never durable —
					// whether or not what arrived happens to parse. Drop it, and
					// say how much. The offset does NOT advance over it: the
					// next open truncates those bytes away.
					dmg.TornTail = int64(len(line))
					break
				}
				// A CLOSED segment does not have torn tails (segments.go, B20
				// restated): it was renamed out of a newline-terminated file, so
				// a missing newline here is damage that arrived afterwards.
				if pos.Offset >= pastFrom {
					dmg.Lines++
					dmg.Bytes += int64(len(line))
				}
			default:
				pos.Offset += int64(len(line)) + 1
				past := pos.Offset > pastFrom
				var rec Record
				if json.Unmarshal(line, &rec) != nil {
					if past {
						dmg.Lines++
						dmg.Bytes += int64(len(line))
					}
					break
				}
				pos.Record++
				fn(rec, *pos, past)
				if limit > 0 && pos.Record >= limit {
					return dmg, nil
				}
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			if s.Live {
				return dmg, err
			}
			d := segmentUnreadable(s, err)
			dmg.add(d)
			return dmg, nil
		}
	}
	return dmg, nil
}

func segmentUnreadable(s ledgerSource, err error) LedgerDamage {
	return LedgerDamage{
		Segments:       1,
		SegmentBytes:   s.Bytes,
		SegmentReasons: s.Name + ": " + err.Error(),
	}
}

// LedgerPositionOfRecord converts "I have folded N records" into a position the
// fast resume can use. It is the bridge for a caller that persisted a record
// COUNT rather than a position — a roll-up state written before positions
// existed, or one rebuilt from a full replay.
//
// It costs one walk of the record, parsing each line only far enough to know
// whether it counts, because record numbering skips damaged lines and a line
// count is therefore not a record count. That is a real cost and it is why the
// position, not the count, is the thing to persist: this function is the
// once-ever conversion, and ScanLedgerFrom is the everyday path.
func LedgerPositionOfRecord(dir string, n int64) (LedgerPos, error) {
	var pos LedgerPos
	if n <= 0 {
		return pos, nil
	}
	srcs, err := ledgerSources(dir)
	if err != nil {
		return pos, err
	}
	for _, s := range srcs {
		pos.Segment = s.Name
		pos.Offset = 0
		before := pos.Record
		if _, err := scanOneSource(s, 0, n, 0, &pos, func(Record, LedgerPos, bool) {}); err != nil {
			return pos, err
		}
		if pos.Record >= n && pos.Record > before {
			return pos, nil
		}
	}
	// Fewer records exist than the caller has folded. That is a caller whose
	// state is ahead of the record — a restored ledger, or a sidecar copied from
	// another host — and the position at the end of what exists is the only
	// answer that does not replay something twice.
	return pos, nil
}

// ScanLedgerFromRecord is LedgerPositionOfRecord followed by ScanLedgerFrom:
// replay everything after the caller's Nth record. It returns the position it
// started from as well, so the caller can persist a position and never pay for
// this walk again.
func ScanLedgerFromRecord(dir string, n int64, fn func(Record, LedgerPos)) (LedgerDamage, LedgerPos, LedgerPos, error) {
	from, err := LedgerPositionOfRecord(dir, n)
	if err != nil {
		return LedgerDamage{}, from, from, err
	}
	dmg, end, err := ScanLedgerFrom(dir, from, fn)
	return dmg, from, end, err
}

// ReadLedger is ScanLedger materialised: the same replay, the same skip-with-
// accounting, collected into one slice for the callers that genuinely want the
// records rather than a pass over them — Archive.Records, and the tests.
//
// IT IS NOT THE REPLAY PATH AND MUST NOT BECOME ONE AGAIN. The slice is the
// whole ledger, live at once; on the deployment's ledger that is gigabytes and
// it is why an 8 GB box lost the ability to restart the archive around day 26.
// Anything that walks the records once and keeps an aggregate — New, List —
// belongs on ScanLedger. A caller that returns the slice to a human, on a
// ledger it did not size first, is choosing the old peak deliberately.
//
// A read that fails partway returns the records it reached, the damage it
// counted so far, and the error, exactly as the scan reports them.
func ReadLedger(dir string) ([]Record, LedgerDamage, error) {
	var out []Record
	dmg, err := ScanLedger(dir, func(rec Record) { out = append(out, rec) })
	return out, dmg, err
}

// maxLedgerLine bounds one line: a whole frame, and room for the record the
// archive wraps it in.
const maxLedgerLine = wire.MaxFrameBytes + (1 << 20)

func readLine(r *bufio.Reader) ([]byte, error) {
	var buf []byte
	for {
		chunk, err := r.ReadSlice('\n')
		buf = append(buf, chunk...)
		if len(buf) > maxLedgerLine {
			return nil, errors.New("archive: ledger line is too long")
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if err != nil {
			return buf, err
		}
		return buf[:len(buf)-1], nil
	}
}
