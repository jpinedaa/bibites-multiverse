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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"multiverse/internal/contractb"
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

// Ledger is the append-only record file.
type Ledger struct {
	mu   sync.Mutex
	path string
	f    *os.File
	// repaired is the size of an unterminated final line dropped when this
	// process opened the file. It is 0 for every ledger closed cleanly.
	repaired int64
}

const ledgerName = "migrations.jsonl"

// OpenLedger opens or creates <dir>/migrations.jsonl for appending, dropping an
// unterminated final line first (see repairTornTail).
func OpenLedger(dir string) (*Ledger, error) {
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
	l := &Ledger{path: path, f: f}
	if err := l.repairTornTail(); err != nil {
		f.Close()
		return nil, err
	}
	return l, nil
}

// Path is the ledger file.
func (l *Ledger) Path() string { return l.path }

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
func (l *Ledger) Append(rec Record) error {
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	l.mu.Lock()
	defer l.mu.Unlock()
	before, statErr := l.size()
	n, err := l.f.Write(b)
	if err != nil {
		l.truncateBack(before, statErr)
		return err
	}
	if n != len(b) {
		// io.Writer permits a short write only with a non-nil error, but the
		// cost of trusting that here is every record written after this one.
		l.truncateBack(before, statErr)
		return fmt.Errorf("archive: short ledger write, %d of %d bytes", n, len(b))
	}
	return l.f.Sync()
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
	TornTail int64
}

// Any is true when the ledger held a complete line that could not be replayed.
func (d LedgerDamage) Any() bool { return d.Lines > 0 }

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
func ScanLedger(dir string, fn func(Record)) (LedgerDamage, error) {
	var dmg LedgerDamage
	f, err := os.Open(filepath.Join(dir, ledgerName))
	if errors.Is(err, os.ErrNotExist) {
		return dmg, nil
	}
	if err != nil {
		return dmg, err
	}
	defer f.Close()

	r := bufio.NewReaderSize(f, 1<<20)
	for {
		line, err := readLine(r)
		if len(line) > 0 {
			switch {
			case errors.Is(err, io.EOF):
				// No newline on the last line, so the write that put it there
				// landed short and the record was never durable — whether or not
				// what arrived happens to parse. Drop it, and say how much.
				dmg.TornTail = int64(len(line))
			default:
				var rec Record
				if json.Unmarshal(line, &rec) != nil {
					dmg.Lines++
					dmg.Bytes += int64(len(line))
					break
				}
				fn(rec)
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return dmg, err
		}
	}
	return dmg, nil
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
