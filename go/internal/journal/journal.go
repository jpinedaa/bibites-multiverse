// Package journal is the sidecar's durable migration custody log (D2).
//
// The rule the whole design exists to serve is contract-a.md §5.3 step 5:
// MIGRATE_OUT_ACK is only correct after the entry has been flushed to durable
// storage. Every mutating call here fsyncs before it returns, so a caller that
// gets a nil error may ACK.
//
// Format: one JSON object per line in <dir>/journal.log, append-only, with a
// compaction rewrite at Open. Two ops:
//
//	create  the immutable half of a migration, payload included, written once
//	status  a small state transition
//
// Replay applies records in file order, so the last status wins. A torn final
// line — the tail of a write that a kill -9 interrupted — is truncated away,
// which is exactly the "entry was never durably journaled" case.
package journal

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"multiverse/internal/fsutil"
	"multiverse/internal/wire"
)

// Direction says which half of the custody chain an entry belongs to.
type Direction string

const (
	// Out is an organism this sim exported. Custody was taken from the local
	// mod and must reach a peer or come home.
	Out Direction = "out"
	// In is an organism a peer sent here. Custody must reach the local mod.
	In Direction = "in"
)

// Status is the state of one migration.
type Status string

const (
	// StatusOpen: durably ours, not yet handed on. Outbound means "not yet
	// accepted by a live peer"; inbound means "not yet delivered to the mod".
	StatusOpen Status = "open"
	// StatusInFlight: handed on and awaiting the answer. Outbound means
	// MIGRATION_PAYLOAD reached a live peer; inbound means MIGRATE_IN is
	// with the mod.
	StatusInFlight Status = "in_flight"
	// StatusDone: the chain completed. The record is a tombstone and is kept
	// for exportRetentionSeconds (contract-a.md §7.2).
	StatusDone Status = "done"
	// StatusHeld: permanently undeliverable and not safely returnable. Kept
	// for an operator (contract-a.md §5.9, contract-b-m4.md §9.4).
	StatusHeld Status = "held"
)

// Handoff is the durable handoff state of contract-b-m4.md §9.2. It answers the
// one question a re-route turns on: COULD CUSTODY HAVE MOVED?
//
//	pending   journaled, never written to a live relay connection   NO
//	sent      written to a live relay connection, no answer yet     YES, unknowably
//	held      sent, and the destination is observed dark            YES, unknowably
//	refused   a statement arrived that proves no custody moved      NO
//	done      MIGRATION_ACK received; becomes a tombstone           it moved, and completed
//
// A bounce is a TERMINAL ACTION, not a sixth state: the entry leaves the
// outbound journal, becomes an inbound delivery into this peer's own mod, and
// leaves a tombstone behind.
type Handoff string

const (
	HandoffPending Handoff = "pending"
	HandoffSent    Handoff = "sent"
	HandoffHeld    Handoff = "held"
	HandoffRefused Handoff = "refused"
	HandoffDone    Handoff = "done"
)

// CustodyMayHaveMoved reports whether the far side could hold this organism.
// It is the whole of the re-route safety rule: only pending and refused are
// safe to redirect, and silence never turns "yes" into "no".
func (h Handoff) CustodyMayHaveMoved() bool {
	return h == HandoffSent || h == HandoffHeld
}

// ParentRef is one entry of the lineage annex as the journal keeps it
// (contract-b-m4.md §6.6). An empty GenomeHash is a gap and GapReason says why.
type ParentRef struct {
	EntityID   int32  `json:"entityId"`
	GenomeHash string `json:"genomeHash,omitempty"`
	GapReason  string `json:"gapReason,omitempty"`
}

// Entry is the immutable half of a migration record.
type Entry struct {
	MigrationID    string  `json:"migrationId"`
	EntityID       int32   `json:"entityId"`
	Kind           string  `json:"kind"`
	GameVersion    string  `json:"gameVersion"`
	Payload        string  `json:"payload,omitempty"`
	PayloadHash    string  `json:"payloadHash"`
	Edge           string  `json:"edge"`
	Position       float64 `json:"position"`
	VelocityX      float64 `json:"velocityX"`
	VelocityY      float64 `json:"velocityY"`
	Heading        float64 `json:"heading"`
	SimulationSize float64 `json:"simulationSize"`
	SimTick        int64   `json:"simTick"`
	SourcePeer     string  `json:"sourcePeer,omitempty"`
	// SourceSlot and DestSlot are map slots (contract-b-m4.md §7.1). A journaled
	// outbound entry keeps the DestSlot it recorded: §7.3 forbids rewriting it
	// when the effective neighbour changes, and routing on the slot is what
	// makes a splice safe for work in flight. The ONE exception is a re-route
	// under a proof of non-delivery (§9.2), which rewrites it through
	// Update.DestSlot and records the evidence beside it.
	SourceSlot int `json:"sourceSlot,omitempty"`
	DestSlot   int `json:"destSlot,omitempty"`
	// GenomeHash and Parents are the lineage annex. The tombstone keeps them
	// after the payload is dropped, because the archive may ask for that genome
	// long after the migration completed (contract-b-m4.md §6.7).
	GenomeHash string      `json:"genomeHash,omitempty"`
	Parents    []ParentRef `json:"parents,omitempty"`
	// Species is the species-identity block, journaled with the entry so it
	// survives everything the organism survives (contract-a.md §16 A30,
	// contract-b-m4.md §15 B9). It is journaled ONLY after the shape check that
	// takes it off a wire, so a replayed entry needs no second one.
	//
	// It has to be durable for the same reason the payload is: a restart replays
	// this entry into a MIGRATION_PAYLOAD or a MIGRATE_IN, a bounce-back replays
	// it into the local mod, and a re-route replays it to another slot. A block
	// held only in memory would leave the organism arriving under a different
	// name than the one it left with, which is the failure §16 exists to close.
	Species     *wire.Species `json:"species,omitempty"`
	JournaledAt int64         `json:"journaledAt"`
}

// State is one migration's full durable state.
//
// The M4 fields are LOAD-BEARING and must survive a restart (§7.4): the handoff
// state says whether custody may have moved, RelaySessionID scopes the proof,
// AccruedHoldMs is what the bounded hold counts, and RerouteCount is what bounds
// it. A sidecar that reconstructs any of them from memory at startup has lost
// the safety property, not just the bookkeeping.
type State struct {
	Entry         Entry     `json:"entry"`
	Direction     Direction `json:"direction"`
	Status        Status    `json:"status"`
	Attempt       int       `json:"attempt"`
	BounceBack    bool      `json:"bounceBack"`
	AckedUpstream bool      `json:"ackedUpstream"`
	Duplicate     bool      `json:"duplicate"`
	Note          string    `json:"note,omitempty"`
	CompletedAt   int64     `json:"completedAt,omitempty"`

	// Handoff is §9.2's state. Outbound entries only.
	Handoff Handoff `json:"handoff,omitempty"`
	// RelaySessionID is the relay session in force at the FIRST write to a live
	// relay connection. A relay-generated neverForwarded: true counts as proof
	// only when its relaySessionId equals this one (§5.2): a link flap keeps the
	// id and keeps the proof; a relay restart changes it and the sender holds.
	RelaySessionID string `json:"relaySessionId,omitempty"`
	// AccruedHoldMs is ACCRUED dark time, not a deadline. A wall-clock deadline
	// cannot express a clock that stops, so the entry carries the accrual
	// instead — a restart cannot lose time already served, and cannot invent
	// time that was never served (§9.3).
	AccruedHoldMs int64 `json:"accruedHoldMs,omitempty"`
	// RerouteCount, RerouteFrom, RerouteProof and RerouteAtMs are the §6.6
	// reroute block, kept so a re-forward reproduces it and the archive can say
	// WHY an organism took the lane it took.
	RerouteCount int    `json:"rerouteCount,omitempty"`
	RerouteFrom  int    `json:"rerouteFrom,omitempty"`
	RerouteProof string `json:"rerouteProof,omitempty"`
	RerouteAtMs  int64  `json:"rerouteAtMs,omitempty"`
	// BouncedTimeout marks the tombstone of an entry the hold timeout bounced.
	// A MIGRATION_ACK that arrives afterwards is the accepted duplication case
	// of §9.3 announcing itself, and it MUST be logged at error level.
	BouncedTimeout bool `json:"bouncedTimeout,omitempty"`

	seq uint64
}

// Clone returns a copy safe to hand outside the journal's lock.
func (s *State) Clone() *State {
	c := *s
	return &c
}

type record struct {
	Op          string    `json:"op"`
	MigrationID string    `json:"migrationId"`
	At          int64     `json:"at"`
	Direction   Direction `json:"direction,omitempty"`
	Entry       *Entry    `json:"entry,omitempty"`
	Status      Status    `json:"status,omitempty"`
	Attempt     *int      `json:"attempt,omitempty"`
	BounceBack  *bool     `json:"bounceBack,omitempty"`
	Acked       *bool     `json:"ackedUpstream,omitempty"`
	Duplicate   *bool     `json:"duplicate,omitempty"`
	Edge        *string   `json:"edge,omitempty"`
	Note        string    `json:"note,omitempty"`
	CompletedAt *int64    `json:"completedAt,omitempty"`
	Purge       bool      `json:"purge,omitempty"`

	Handoff        *Handoff `json:"handoff,omitempty"`
	RelaySessionID *string  `json:"relaySessionId,omitempty"`
	AccruedHoldMs  *int64   `json:"accruedHoldMs,omitempty"`
	DestSlot       *int     `json:"destSlot,omitempty"`
	RerouteCount   *int     `json:"rerouteCount,omitempty"`
	RerouteFrom    *int     `json:"rerouteFrom,omitempty"`
	RerouteProof   *string  `json:"rerouteProof,omitempty"`
	RerouteAtMs    *int64   `json:"rerouteAtMs,omitempty"`
	BouncedTimeout *bool    `json:"bouncedTimeout,omitempty"`
}

const (
	opCreate = "create"
	opStatus = "status"
	fileName = "journal.log"
	// A payload can be 4 MiB, so the scanner needs headroom over the default
	// 64 KiB token limit.
	maxLine = wire.MaxFrameBytes + (1 << 20)
)

// ErrNotFound is returned for an unknown migrationId.
var ErrNotFound = errors.New("journal: migration not found")

// ErrDuplicate is returned when Create is called for an id that already exists.
var ErrDuplicate = errors.New("journal: migration already journaled")

// Journal is the durable custody log. Every method is safe for concurrent use.
type Journal struct {
	mu     sync.Mutex
	dir    string
	f      *os.File
	states map[string]*State
	seq    uint64
	closed bool
}

// Open loads (and compacts) the journal in dir, creating dir when needed.
func Open(dir string) (*Journal, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	j := &Journal{dir: dir, states: map[string]*State{}}
	if err := j.replay(); err != nil {
		return nil, err
	}
	if err := j.compact(); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(j.path(), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	j.f = f
	return j, nil
}

func (j *Journal) path() string { return filepath.Join(j.dir, fileName) }

func (j *Journal) replay() error {
	f, err := os.Open(j.path())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()

	r := bufio.NewReaderSize(f, 1<<20)
	var offset int64
	for {
		line, err := readLine(r)
		if len(line) > 0 {
			var rec record
			if jsonErr := json.Unmarshal(line, &rec); jsonErr != nil {
				// A torn tail: the write never completed, so the entry was
				// never durable. Drop it and everything after it.
				break
			}
			j.apply(rec)
			offset += int64(len(line)) + 1
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}
	}
	// Truncate a partial tail so the next append starts on a clean boundary.
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.Size() != offset {
		if err := os.Truncate(j.path(), offset); err != nil {
			return err
		}
	}
	return nil
}

// readLine returns one newline-terminated line without the newline. A final
// line with no newline is returned with io.EOF and treated as torn by the
// caller only if it does not parse; a complete last record without a trailing
// newline cannot happen because every append writes one.
func readLine(r *bufio.Reader) ([]byte, error) {
	var buf []byte
	for {
		chunk, err := r.ReadSlice('\n')
		buf = append(buf, chunk...)
		if len(buf) > maxLine {
			return nil, fmt.Errorf("journal: line over %d bytes", maxLine)
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

func (j *Journal) apply(rec record) {
	switch rec.Op {
	case opCreate:
		if rec.Entry == nil {
			return
		}
		j.seq++
		st := &State{
			Entry:     *rec.Entry,
			Direction: rec.Direction,
			Status:    StatusOpen,
			Attempt:   0,
			seq:       j.seq,
		}
		if rec.Direction == Out {
			// §9.2: a journal write lands in pending. The frame was never handed
			// to anybody, and the sender's own record is sufficient proof of that.
			st.Handoff = HandoffPending
		}
		if rec.BounceBack != nil {
			st.BounceBack = *rec.BounceBack
		}
		j.states[rec.MigrationID] = st
	case opStatus:
		st, ok := j.states[rec.MigrationID]
		if !ok {
			return
		}
		if rec.Purge {
			delete(j.states, rec.MigrationID)
			return
		}
		if rec.Status != "" {
			st.Status = rec.Status
		}
		if rec.Direction != "" {
			st.Direction = rec.Direction
		}
		if rec.Attempt != nil {
			st.Attempt = *rec.Attempt
		}
		if rec.BounceBack != nil {
			st.BounceBack = *rec.BounceBack
		}
		if rec.Acked != nil {
			st.AckedUpstream = *rec.Acked
		}
		if rec.Duplicate != nil {
			st.Duplicate = *rec.Duplicate
		}
		if rec.Edge != nil {
			st.Entry.Edge = *rec.Edge
		}
		if rec.CompletedAt != nil {
			st.CompletedAt = *rec.CompletedAt
		}
		if rec.Note != "" {
			st.Note = rec.Note
		}
		if rec.Handoff != nil {
			st.Handoff = *rec.Handoff
		} else if st.Direction == Out && rec.Status == StatusInFlight && st.Handoff == HandoffPending {
			// AN M3 JOURNAL HAS NO HANDOFF FIELD, AND REPLAYING IT AS `pending`
			// WOULD BE A LIE THAT COSTS AN ORGANISM.
			//
			// M3's sidecar moved an outbound entry to in_flight only after
			// MIGRATION_PAYLOAD had been written to a live relay connection, so
			// an M3 in_flight entry means exactly what M4 calls `sent`: custody
			// MAY have moved, unknowably. `pending` means the opposite — the
			// frame reached nobody — and §9.2 lets a pending entry RE-ROUTE to a
			// different slot with no proof of non-delivery. That re-route is the
			// duplication D2 refuses.
			//
			// The condition is exact rather than broad: every M4 write that sets
			// an OUTBOUND status to in_flight is forwardLocked, and it sets
			// handoff=sent in the same record. An outbound in_flight with no
			// handoff field therefore identifies an M3 record and nothing else.
			// Inbound entries are excluded because their in_flight means "with
			// the mod" and they have no handoff state at all (§9.2).
			st.Handoff = HandoffSent
		}
		if rec.RelaySessionID != nil {
			st.RelaySessionID = *rec.RelaySessionID
		}
		if rec.AccruedHoldMs != nil {
			st.AccruedHoldMs = *rec.AccruedHoldMs
		}
		if rec.DestSlot != nil {
			st.Entry.DestSlot = *rec.DestSlot
		}
		if rec.RerouteCount != nil {
			st.RerouteCount = *rec.RerouteCount
		}
		if rec.RerouteFrom != nil {
			st.RerouteFrom = *rec.RerouteFrom
		}
		if rec.RerouteProof != nil {
			st.RerouteProof = *rec.RerouteProof
		}
		if rec.RerouteAtMs != nil {
			st.RerouteAtMs = *rec.RerouteAtMs
		}
		if rec.BouncedTimeout != nil {
			st.BouncedTimeout = *rec.BouncedTimeout
		}
		if st.Status == StatusDone {
			// A tombstone keeps the identity and drops the bytes.
			st.Entry.Payload = ""
		}
	}
}

// compact rewrites the log as one create per surviving state plus its current
// status, then renames it into place and fsyncs the directory. An os.Rename
// without a directory sync is not durable (contract-a.md §11.1).
func (j *Journal) compact() error {
	tmp := j.path() + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	w := bufio.NewWriterSize(f, 1<<20)
	for _, st := range j.ordered() {
		entry := st.Entry
		create := record{Op: opCreate, MigrationID: entry.MigrationID, At: entry.JournaledAt,
			Direction: st.Direction, Entry: &entry, BounceBack: boolPtr(st.BounceBack)}
		if err := writeRecord(w, create); err != nil {
			f.Close()
			return err
		}
		status := record{Op: opStatus, MigrationID: entry.MigrationID, At: time.Now().UnixMilli(),
			Direction: st.Direction, Status: st.Status, Attempt: intPtr(st.Attempt),
			BounceBack: boolPtr(st.BounceBack), Acked: boolPtr(st.AckedUpstream),
			Duplicate: boolPtr(st.Duplicate), Note: st.Note,
			AccruedHoldMs: int64Ptr(st.AccruedHoldMs), DestSlot: intPtr(st.Entry.DestSlot),
			RerouteCount: intPtr(st.RerouteCount), RerouteFrom: intPtr(st.RerouteFrom),
			RerouteProof: strPtr(st.RerouteProof), RerouteAtMs: int64Ptr(st.RerouteAtMs),
			BouncedTimeout: boolPtr(st.BouncedTimeout)}
		if st.Handoff != "" {
			h := st.Handoff
			status.Handoff = &h
		}
		if st.RelaySessionID != "" {
			status.RelaySessionID = strPtr(st.RelaySessionID)
		}
		if st.CompletedAt != 0 {
			status.CompletedAt = int64Ptr(st.CompletedAt)
		}
		if err := writeRecord(w, status); err != nil {
			f.Close()
			return err
		}
	}
	if err := w.Flush(); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, j.path()); err != nil {
		return err
	}
	return syncDir(j.dir)
}

func writeRecord(w *bufio.Writer, rec record) error {
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	if _, err := w.Write(b); err != nil {
		return err
	}
	return w.WriteByte('\n')
}

// syncDir flushes the directory entry after a rename-into-place. It is
// platform-dependent — Windows has no directory fsync at all — so the primitive
// lives in internal/fsutil.
func syncDir(dir string) error { return fsutil.SyncDir(dir) }

// append writes one record and flushes it to durable storage before returning.
func (j *Journal) append(rec record) error {
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if _, err := j.f.Write(b); err != nil {
		return err
	}
	return j.f.Sync()
}

// Create durably journals a new migration. On a nil return the caller may ACK.
func (j *Journal) Create(dir Direction, entry Entry, bounceBack bool) (*State, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return nil, os.ErrClosed
	}
	if _, ok := j.states[entry.MigrationID]; ok {
		return nil, ErrDuplicate
	}
	rec := record{Op: opCreate, MigrationID: entry.MigrationID, At: entry.JournaledAt,
		Direction: dir, Entry: &entry, BounceBack: boolPtr(bounceBack)}
	if err := j.append(rec); err != nil {
		return nil, err
	}
	j.apply(rec)
	return j.states[entry.MigrationID].Clone(), nil
}

// Update is a durable state transition. mut receives a scratch record that the
// caller fills in; only the fields it sets are written.
type Update struct {
	Status      Status
	Direction   Direction
	Attempt     *int
	BounceBack  *bool
	Acked       *bool
	Duplicate   *bool
	Edge        *string
	CompletedAt *int64
	Note        string

	Handoff        Handoff
	RelaySessionID *string
	AccruedHoldMs  *int64
	// DestSlot is the ONE exception to §7.3's no-rewrite rule, and it carries
	// its own evidence: a re-route under a proof of non-delivery (§9.2). Every
	// other entry keeps the destination it recorded.
	DestSlot       *int
	RerouteCount   *int
	RerouteFrom    *int
	RerouteProof   *string
	RerouteAtMs    *int64
	BouncedTimeout *bool
}

// Apply durably records u against migrationID.
func (j *Journal) Apply(migrationID string, u Update) (*State, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return nil, os.ErrClosed
	}
	if _, ok := j.states[migrationID]; !ok {
		return nil, ErrNotFound
	}
	rec := record{Op: opStatus, MigrationID: migrationID, At: time.Now().UnixMilli(),
		Status: u.Status, Direction: u.Direction, Attempt: u.Attempt, BounceBack: u.BounceBack,
		Acked: u.Acked, Duplicate: u.Duplicate, Edge: u.Edge, CompletedAt: u.CompletedAt,
		Note: u.Note, RelaySessionID: u.RelaySessionID, AccruedHoldMs: u.AccruedHoldMs,
		DestSlot: u.DestSlot, RerouteCount: u.RerouteCount, RerouteFrom: u.RerouteFrom,
		RerouteProof: u.RerouteProof, RerouteAtMs: u.RerouteAtMs,
		BouncedTimeout: u.BouncedTimeout}
	if u.Handoff != "" {
		h := u.Handoff
		rec.Handoff = &h
	}
	if err := j.append(rec); err != nil {
		return nil, err
	}
	j.apply(rec)
	return j.states[migrationID].Clone(), nil
}

// Get returns a copy of one state.
func (j *Journal) Get(migrationID string) (*State, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	st, ok := j.states[migrationID]
	if !ok {
		return nil, false
	}
	return st.Clone(), true
}

// List returns every state in journal order (contract-a.md §7.5 replays in
// journal order).
func (j *Journal) List() []*State {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := make([]*State, 0, len(j.states))
	for _, st := range j.ordered() {
		out = append(out, st.Clone())
	}
	return out
}

// CountPending counts entries in a direction that have not reached a terminal
// state. It feeds inboundQueueMax admission control (contract-a.md §8 step 4).
func (j *Journal) CountPending(dir Direction) int {
	j.mu.Lock()
	defer j.mu.Unlock()
	n := 0
	for _, st := range j.states {
		if st.Direction == dir && (st.Status == StatusOpen || st.Status == StatusInFlight) {
			n++
		}
	}
	return n
}

// ordered returns states sorted by creation order. Caller holds the lock.
func (j *Journal) ordered() []*State {
	out := make([]*State, 0, len(j.states))
	for _, st := range j.states {
		out = append(out, st)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].seq < out[b].seq })
	return out
}

// PurgeExpired drops tombstones older than retention. Tombstones must be
// durable, so the purge is durable too (contract-a.md §11.1).
func (j *Journal) PurgeExpired(retention time.Duration, now time.Time) (int, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return 0, os.ErrClosed
	}
	cutoff := now.Add(-retention).UnixMilli()
	n := 0
	for _, st := range j.ordered() {
		if st.Status != StatusDone || st.CompletedAt == 0 || st.CompletedAt > cutoff {
			continue
		}
		rec := record{Op: opStatus, MigrationID: st.Entry.MigrationID,
			At: now.UnixMilli(), Purge: true}
		if err := j.append(rec); err != nil {
			return n, err
		}
		j.apply(rec)
		n++
	}
	return n, nil
}

// Close flushes and closes the log.
func (j *Journal) Close() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return nil
	}
	j.closed = true
	if j.f == nil {
		return nil
	}
	if err := j.f.Sync(); err != nil {
		j.f.Close()
		return err
	}
	return j.f.Close()
}

func boolPtr(b bool) *bool    { return &b }
func intPtr(i int) *int       { return &i }
func int64Ptr(i int64) *int64 { return &i }
func strPtr(s string) *string { return &s }
