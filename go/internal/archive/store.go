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
//     system rather than two.
//  3. It has no schema to migrate. The lineage graph M6 will want is a
//     different shape from anything M4 could guess, and a ledger replays into
//     whatever shape that turns out to be.
//
// The cost is honest and bounded: reads are a full-file replay, so a query
// surface beyond "list what you recorded" would need an index. That is exactly
// the ambition M4 does not have.

import (
	"bufio"
	"encoding/json"
	"errors"
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
	Timestamp   int64              `json:"timestamp,omitempty"`
	Duplicate   bool               `json:"duplicate,omitempty"`
	Code        string             `json:"code,omitempty"`
	Message     string             `json:"message,omitempty"`
	GenomeHash  string             `json:"genomeHash,omitempty"`
	ServedBy    string             `json:"servedBy,omitempty"`
}

// Ledger is the append-only record file.
type Ledger struct {
	mu   sync.Mutex
	path string
	f    *os.File
}

const ledgerName = "migrations.jsonl"

// OpenLedger opens or creates <dir>/migrations.jsonl for appending.
func OpenLedger(dir string) (*Ledger, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, ledgerName)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	return &Ledger{path: path, f: f}, nil
}

// Path is the ledger file.
func (l *Ledger) Path() string { return l.path }

// Append writes one record and flushes it before returning.
func (l *Ledger) Append(rec Record) error {
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, err := l.f.Write(append(b, '\n')); err != nil {
		return err
	}
	return l.f.Sync()
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

// ReadLedger replays every record in dir's ledger, in write order. A torn final
// line — the tail of a write a kill -9 interrupted — is dropped, exactly as the
// sidecar journal drops one.
func ReadLedger(dir string) ([]Record, error) {
	f, err := os.Open(filepath.Join(dir, ledgerName))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []Record
	r := bufio.NewReaderSize(f, 1<<20)
	for {
		line, err := readLine(r)
		if len(line) > 0 {
			var rec Record
			if json.Unmarshal(line, &rec) != nil {
				break
			}
			out = append(out, rec)
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return out, err
		}
	}
	return out, nil
}

func readLine(r *bufio.Reader) ([]byte, error) {
	var buf []byte
	for {
		chunk, err := r.ReadSlice('\n')
		buf = append(buf, chunk...)
		if len(buf) > wire.MaxFrameBytes+(1<<20) {
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
