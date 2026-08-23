// Package journal is the sidecar's durable migration custody log (D2).
//
// The rule the whole design exists to serve is contract-a.md §5.3 step 5:
// MIGRATE_OUT_ACK is only correct after the entry has been flushed to durable
// storage. Every mutating call here fsyncs before it returns, so a caller that
// gets a nil error may ACK.
//
// Format: one JSON object per line in <dir>/journal.log, append-only, with a
// compaction rewrite at Open and, from Compact, at whatever interval the owner
// schedules. Two ops:
//
//	create  the immutable half of a migration, payload included, written once
//	status  a small state transition
//
// Replay applies records in file order, so the last status wins. A torn final
// line — the tail of a write that a kill -9 interrupted — is truncated away,
// which is exactly the "entry was never durably journaled" case.
//
// THE FILE ONLY EVER SHRANK AT Open. An append-only log whose only compaction
// runs at startup is a log that grows for as long as the process lives, and a
// create record carries the migrant's payload — up to 4 MiB of it. On
// 2026-08-08 five sidecars that had been up for two days held 445 MB, 500 MB,
// 905 MB, 683 MB and 720 MB of journal for a live set of a few hundred entries,
// and the root filesystem ran out. Compaction is now something the owner can
// schedule, and Open's compaction is one call to the same code.
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
//	refused   a statement arrived that proves no custody moved      NO
//	done      MIGRATION_ACK received; becomes a tombstone           it moved, and completed
//	lost      sent, and no answer came within forwardTimeoutMs      YES, unknowably — and
//	          unknowable forever: the organism is not re-sent and not returned
//
// THE `held` STATE IS GONE (contract-b-m4.md §25, B37). A forwarded frame is
// forwarded once. It is never re-forwarded, no clock accrues against it, and it
// never comes home on a timeout: migration is at-most-once with no exception,
// and a forward that is never answered is a loss.
//
// A bounce is a TERMINAL ACTION, not a state: the entry leaves the outbound
// journal, becomes an inbound delivery into this peer's own mod, and leaves a
// tombstone behind. It is reachable only from `pending` and `refused`, where no
// custody can have moved, and from the operator's --release-inflight.
type Handoff string

// retiredHandoffHeld is the `held` state of the bounded hold, retired by §25's
// B37. It is never written; it is only ever read out of a journal an older
// sidecar left behind, and it replays as HandoffSent.
const retiredHandoffHeld Handoff = "held"

const (
	HandoffPending Handoff = "pending"
	HandoffSent    Handoff = "sent"
	HandoffRefused Handoff = "refused"
	HandoffDone    Handoff = "done"
	HandoffLost    Handoff = "lost"
)

// CustodyMayHaveMoved reports whether the far side could hold this organism.
// It is the whole of the re-route safety rule: only pending and refused are
// safe to redirect, and silence never turns "yes" into "no".
func (h Handoff) CustodyMayHaveMoved() bool {
	return h == HandoffSent || h == HandoffLost
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
// SentAtMs is when the forward-resolution deadline started, and RerouteCount is
// what bounds the re-route. A sidecar that reconstructs any of them from memory
// at startup has lost the safety property, not just the bookkeeping.
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
	// ForwardReceipts is how many FORWARD_RECEIPTs this sender has recorded for
	// this migration (contract-b-m4.md §6.12, §22 B26). ONE FORWARD, ONE
	// RECEIPT, so two under one migrationId means this sender forwarded twice —
	// a fact about its own retries, never a duplicated organism, because the
	// migrationId is preserved and the destination deduplicates (§6.6).
	//
	// It is DURABLE for the reason B26 exists: the relay's forwarding record is
	// in memory and dies with the process, and the whole point of the receipt is
	// that the fact moves into the sender's own journal, where D2 keeps custody.
	// A sidecar that held it in memory would have bought nothing.
	ForwardReceipts int `json:"forwardReceipts,omitempty"`
	// ReceiptSessionID, ReceiptDestSlot and ReceiptForwardedAtMs are the LAST
	// receipt's three fields. The session is the load-bearing one: a receipt is a
	// statement about ONE relay session (§5.2), and comparing it against a relay
	// NACK's session is what makes ForwardedUnder a contradiction test rather
	// than a hint.
	ReceiptSessionID string `json:"receiptSessionId,omitempty"`
	// ReceiptDestSlot is the slot the relay wrote to, echoed on the receipt so a
	// sender that re-routed can tell two attempts apart (§6.12).
	ReceiptDestSlot int `json:"receiptDestSlot,omitempty"`
	// ReceiptForwardedAtMs is the RELAY's clock at the write. Informational (D5)
	// and kept for the operator's report only: no rule compares it with anything,
	// because a correctness decision on another machine's clock is what the
	// session id exists to avoid.
	ReceiptForwardedAtMs int64 `json:"receiptForwardedAt,omitempty"`
	// SentAtMs is the wall clock at the FIRST write of this entry to a live relay
	// connection. It is the only clock an outbound entry carries since §25's B37
	// removed the hold, and it decides one thing: when an unanswered forward
	// stops being in flight and is recorded LOST (forwardTimeoutMs, §9.3).
	//
	// A plain wall-clock instant is enough BECAUSE OF WHAT EXPIRY NOW DOES. The
	// accrual it replaces existed so that a sender which had never observed the
	// destination dark could not wake with an expired clock and bounce an
	// organism that was on its way — expiry moved an organism. Expiry now only
	// closes a record: nothing is re-sent, nothing comes home, and a sidecar that
	// slept through the deadline records a loss that had already happened.
	SentAtMs int64 `json:"sentAt,omitempty"`
	// RerouteCount, RerouteFrom, RerouteProof and RerouteAtMs are the §6.6
	// reroute block, kept so a re-forward reproduces it and the archive can say
	// WHY an organism took the lane it took.
	RerouteCount int    `json:"rerouteCount,omitempty"`
	RerouteFrom  int    `json:"rerouteFrom,omitempty"`
	RerouteProof string `json:"rerouteProof,omitempty"`
	RerouteAtMs  int64  `json:"rerouteAtMs,omitempty"`
	// RefusedSlots is the durable set of live destinations that explicitly
	// declined this migration. It prevents overload spillover from circling
	// back to a world already tried after a process restart.
	RefusedSlots []int `json:"refusedSlots,omitempty"`

	seq uint64
}

// AwaitsUpstreamAck reports whether this completed inbound migration still
// owes its source a durable MIGRATION_ACK. The scheduler, retention rule, and
// local diagnostics share this predicate so none can silently disagree about
// which tombstones remain retryable.
func (s *State) AwaitsUpstreamAck() bool {
	return s.Direction == In && s.Status == StatusDone && !s.AckedUpstream &&
		!s.BounceBack && s.Entry.SourcePeer != ""
}

// Clone returns a copy safe to hand outside the journal's lock.
func (s *State) Clone() *State {
	c := *s
	c.RefusedSlots = append([]int(nil), s.RefusedSlots...)
	return &c
}

// ForwardedUnder reports whether this entry holds a FORWARD_RECEIPT issued under
// session — that is, whether THIS SENDER'S OWN JOURNAL says the relay wrote this
// migration's bytes to a socket during that session (contract-b-m4.md §6.12,
// §22 B26).
//
// IT IS EVIDENCE IN EXACTLY ONE DIRECTION. True means the frame WAS forwarded.
// False means nothing at all: a receipt that was never sent, was dropped from a
// full outbound queue, or was lost with the session is indistinguishable from a
// forward that never happened, and §9.2 gained the row that says so — A MISSING
// RECEIPT IS SILENCE, AND SILENCE IS NEVER PROOF IN THIS CONTRACT.
//
// Its ONE caller is the sender's answer to a relay-generated
// `neverForwarded: true`, and the direction it can push that answer is toward
// HOLDING and never toward re-routing (B26's *It changes no safety rule* row).
// An empty session matches nothing, because "no session" is not a session two
// statements can be about.
func (s *State) ForwardedUnder(session string) bool {
	return s.ForwardReceipts > 0 && session != "" && s.ReceiptSessionID == session
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
	SentAtMs       *int64   `json:"sentAt,omitempty"`
	DestSlot       *int     `json:"destSlot,omitempty"`
	RerouteCount   *int     `json:"rerouteCount,omitempty"`
	RerouteFrom    *int     `json:"rerouteFrom,omitempty"`
	RerouteProof   *string  `json:"rerouteProof,omitempty"`
	RerouteAtMs    *int64   `json:"rerouteAtMs,omitempty"`
	RefusedSlots   []int    `json:"refusedSlots,omitempty"`

	// B26's four. The COUNT is written absolute rather than as an increment,
	// which is what makes a record idempotent under both of the ways this log is
	// read: replay applies every line once, and a compaction rewrites the live
	// state as one status record. An increment would be correct for the first
	// and a lie for the second.
	ForwardReceipts      *int    `json:"forwardReceipts,omitempty"`
	ReceiptSessionID     *string `json:"receiptSessionId,omitempty"`
	ReceiptDestSlot      *int    `json:"receiptDestSlot,omitempty"`
	ReceiptForwardedAtMs *int64  `json:"receiptForwardedAt,omitempty"`
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

// ErrReadOnly is returned by every mutating method of a journal opened with
// OpenReadOnly. A diagnostic holds one while the owning sidecar is running, and
// the single-writer rule is what keeps that safe.
var ErrReadOnly = errors.New("journal: opened read-only")

// testHookPreRename runs inside compact, after the scratch file is fsynced and
// before it is renamed into place — the one instant a crash has to be harmless.
// It is nil everywhere except the test that SIGKILLs a process there.
var testHookPreRename func()

// The three filesystem calls the compaction rewrite makes, indirected so a test
// can fail them deliberately. WINDOWS REFUSES TO RENAME OVER A FILE THAT IS
// OPEN, and the only way to prove on Linux that compaction closes its append
// handle BEFORE the rename is to make the rename fail while one is open.
var (
	openAppendFile = func(path string) (*os.File, error) {
		return os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	}
	closeFile  = func(f *os.File) error { return f.Close() }
	renameFile = os.Rename
)

// Journal is the durable custody log. Every method is safe for concurrent use.
type Journal struct {
	mu     sync.Mutex
	dir    string
	f      *os.File
	states map[string]*State
	seq    uint64
	closed bool
	// readOnly is set by OpenReadOnly. There is no write handle behind it, so
	// every mutating path refuses rather than dereferencing a nil file.
	readOnly bool
	// discarded is how many bytes replay threw away behind a torn line. It is
	// 0 for every healthy journal and a reason to shout for any other.
	discarded int64
}

// Discarded reports the bytes the last replay dropped behind an unparsable
// record. Anything but 0 means the log was damaged before this process opened
// it and that some custody history is gone; the caller MUST log it.
func (j *Journal) Discarded() int64 {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.discarded
}

// tailBytes is how much of the log sits at or after offset.
func (j *Journal) tailBytes(offset int64) int64 {
	info, err := os.Stat(j.path())
	if err != nil || info.Size() <= offset {
		return 0
	}
	return info.Size() - offset
}

// OpenReadOnly replays the journal in dir and returns it WITHOUT compacting it,
// WITHOUT creating anything, and WITHOUT a write handle. It is what a diagnostic
// opens.
//
// THE ORDINARY Open IS NOT A READ. It creates the directory, it rewrites the log
// to its live set, and it holds the file open for append — three things a tool
// that promises to change nothing may not do, and the last of which is why
// --list-inflight has always had to say "the sidecar must be stopped". Replay
// alone needs none of them: the log is append-only, so a reader that stops at
// the last complete record it can see has read a consistent prefix of it even
// while the owning process is writing the next one.
//
// The Journal it returns answers every read — List, Get, CountPending,
// Discarded, Size — and refuses every write with ErrReadOnly rather than
// corrupting a file another process owns.
func OpenReadOnly(dir string) (*Journal, error) {
	j := &Journal{dir: dir, states: map[string]*State{}, readOnly: true}
	if err := j.replay(); err != nil {
		return nil, err
	}
	return j, nil
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
	if err := j.openAppend(); err != nil {
		return nil, err
	}
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
				//
				// "And everything after it" is the dangerous half, and until
				// 2026-08-08 it happened in silence. A short write on a full
				// disk can put an unparsable line in the MIDDLE of the log, and
				// every record behind it — hours of custody — is then discarded
				// by a replay that reports nothing. append no longer leaves such
				// a line, but a journal written by an older binary still can,
				// so what is discarded is now measured and the caller is
				// expected to say so out loud.
				//
				// Only COMPLETE records behind the damage count. An unparsable
				// line with nothing after it is the ordinary torn tail of a
				// kill -9 and cost nothing that was ever durable; a newline
				// behind it means whole records are being thrown away.
				lineEnd := offset + int64(len(line))
				if err == nil {
					lineEnd++ // readLine stripped the terminating newline
				}
				j.discarded = j.tailBytes(lineEnd)
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
			if st.Handoff == retiredHandoffHeld {
				// A JOURNAL WRITTEN BEFORE §25's B37 HAS A `held` STATE, AND IT
				// MEANT EXACTLY WHAT `sent` MEANS: written to a live relay
				// connection, no terminal answer, custody may have moved. The
				// hold that distinguished the two is gone, so the state folds
				// into the one it was a sub-case of. Replaying it as an unknown
				// string instead would leave the entry in no case of the
				// sender's state machine — journaled, never resolved, never
				// counted.
				st.Handoff = HandoffSent
			}
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
		if rec.SentAtMs != nil {
			st.SentAtMs = *rec.SentAtMs
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
		if len(rec.RefusedSlots) > 0 {
			st.RefusedSlots = append([]int(nil), rec.RefusedSlots...)
		}
		if rec.ForwardReceipts != nil {
			st.ForwardReceipts = *rec.ForwardReceipts
		}
		if rec.ReceiptSessionID != nil {
			st.ReceiptSessionID = *rec.ReceiptSessionID
		}
		if rec.ReceiptDestSlot != nil {
			st.ReceiptDestSlot = *rec.ReceiptDestSlot
		}
		if rec.ReceiptForwardedAtMs != nil {
			st.ReceiptForwardedAtMs = *rec.ReceiptForwardedAtMs
		}
		if st.Status == StatusDone {
			// A tombstone keeps the identity and drops the bytes.
			st.Entry.Payload = ""
		}
	}
}

// Compact rewrites the live journal in place, dropping every superseded record.
//
// It is the whole of the disk-budget fix, and it is safe to call at any moment
// because it never reads the file it is replacing: the in-memory state map IS
// the compacted content, so a compaction costs one pass over the live entries —
// hundreds, not the millions of dead records on disk — and finishes in
// milliseconds. It reports the file size before and after so a caller can log
// what it reclaimed.
//
// CRASH SAFETY. The rewrite lands in journal.log.tmp, is fsynced, and is then
// renamed over journal.log with the directory synced after it, exactly as at
// Open. A kill at any point before the rename leaves the original journal
// untouched and complete; a kill after it leaves the compacted journal, which
// replays to the identical state. There is no window in which both files are
// partial, so no crash can lose an entry.
//
// THE APPEND HANDLE IS CLOSED FOR THE RENAME AND OPENED AGAIN AFTER IT, which on
// Windows is the difference between a journal that shrinks and one that never
// does; see compact. If that reopen fails the journal marks itself closed rather
// than claim a write handle it does not hold: a sidecar that cannot journal must
// stop ACKing (contract-a.md §5.3 step 5), and silently writing custody records
// into a file nothing can reach is the one outcome worse than an error. The log
// on disk is whole either way, and a restart replays it to the same state.
func (j *Journal) Compact() (before, after int64, err error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return 0, 0, os.ErrClosed
	}
	if j.readOnly {
		return 0, 0, ErrReadOnly
	}
	before = j.size()
	if err := j.compact(); err != nil {
		return before, j.size(), err
	}
	return before, j.size(), nil
}

// size is the journal file's length, or 0 if it cannot be read. Caller holds
// the lock.
func (j *Journal) size() int64 {
	info, err := os.Stat(j.path())
	if err != nil {
		return 0
	}
	return info.Size()
}

// Size is the log file's length on disk, and Path is where it is. Both are for
// a reader — a diagnostic reporting what this machine has written, and what it
// would have to say if the disk filled.
func (j *Journal) Size() int64 {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.size()
}

// Path is the journal log's path.
func (j *Journal) Path() string { return j.path() }

// Live is the number of entries a compaction would keep.
func (j *Journal) Live() int {
	j.mu.Lock()
	defer j.mu.Unlock()
	return len(j.states)
}

// compact rewrites the log as one create per surviving state plus its current
// status, then renames it into place and fsyncs the directory. An os.Rename
// without a directory sync is not durable (contract-a.md §11.1).
//
// THE APPEND HANDLE IS CLOSED BEFORE THE RENAME, AND THAT IS A WINDOWS RULE.
// Go's os.OpenFile asks for no FILE_SHARE_DELETE, so a file this process holds
// open cannot be replaced: MoveFileEx(REPLACE_EXISTING) — which is what
// os.Rename is on Windows — fails with
//
//	rename ...\journal\journal.log.tmp ...\journal\journal.log: Access is denied.
//
// and it fails that way on EVERY attempt for the life of the process, so the
// journal never shrinks again. Both sidecars of the living deployment logged
// exactly that line every fifteen minutes and reached 718 MB and 132 MB, past
// the ceiling the diagnostic advertises, while the code that was supposed to
// bound them ran on schedule. On Linux the same rename succeeds — the old inode
// outlives the handle — which is why no test on this machine ever saw it, and
// why TestCompactObeysTheWindowsRenameRule enforces the Windows rule by hand.
//
// THE OTHER CANDIDATE FIX WOULD HAVE BEEN WORSE. Opening the append handle with
// FILE_SHARE_DELETE does let the rename through, but the handle then keeps
// pointing at the file that was just replaced: every later append would land in
// a file with no name, be fsynced, be ACKed, and be gone at the next start —
// D2's whole promise, broken quietly. Closing the handle, renaming, and opening
// it again has no such window, and it is the same shape on both operating
// systems.
func (j *Journal) compact() error {
	tmp := j.path() + ".tmp"
	// A failed compaction must not leave its scratch file behind. The rewrite
	// can be megabytes, and the disk pressure this whole mechanism exists to
	// relieve is exactly when the write fails.
	renamed := false
	defer func() {
		if !renamed {
			_ = os.Remove(tmp)
		}
	}()
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
			SentAtMs: int64Ptr(st.SentAtMs), DestSlot: intPtr(st.Entry.DestSlot),
			RerouteCount: intPtr(st.RerouteCount), RerouteFrom: intPtr(st.RerouteFrom),
			RerouteProof: strPtr(st.RerouteProof), RerouteAtMs: int64Ptr(st.RerouteAtMs),
			RefusedSlots: append([]int(nil), st.RefusedSlots...)}
		if st.Handoff != "" {
			h := st.Handoff
			status.Handoff = &h
		}
		if st.RelaySessionID != "" {
			status.RelaySessionID = strPtr(st.RelaySessionID)
		}
		// B26's block survives a compaction, and it has to: a compaction is
		// routine (journalCompactMinutes, §20 B20) and an entry that lost its
		// receipt to one would have lost the evidence B26 exists to make durable.
		if st.ForwardReceipts > 0 {
			status.ForwardReceipts = intPtr(st.ForwardReceipts)
			status.ReceiptSessionID = strPtr(st.ReceiptSessionID)
			status.ReceiptDestSlot = intPtr(st.ReceiptDestSlot)
			status.ReceiptForwardedAtMs = int64Ptr(st.ReceiptForwardedAtMs)
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
	if testHookPreRename != nil {
		testHookPreRename()
	}
	// From here to the reopen this journal has no write handle. Nothing can try
	// to append in the window: every mutating path takes j.mu, and the caller
	// holds it.
	reopen := j.f != nil
	if err := j.closeAppend(); err != nil {
		// os.File.Close releases the descriptor even when it reports an error,
		// so there is no handle left to append through. Refusing every later
		// write is the safe direction: the caller stops ACKing.
		j.closed = true
		return err
	}
	if err := renameInto(tmp, j.path()); err != nil {
		// journal.log was not touched. It is whole, it holds every record, and
		// all that is lost is the reclaim — so take the append handle back and
		// let the sidecar keep taking custody.
		if reopen {
			if reopenErr := j.openAppend(); reopenErr != nil {
				j.closed = true
				return errors.Join(err, reopenErr)
			}
		}
		return err
	}
	renamed = true
	if reopen {
		if err := j.openAppend(); err != nil {
			j.closed = true
			return err
		}
	}
	return syncDir(j.dir)
}

// openAppend and closeAppend own j.f. Caller holds the lock.
func (j *Journal) openAppend() error {
	f, err := openAppendFile(j.path())
	if err != nil {
		return err
	}
	j.f = f
	return nil
}

func (j *Journal) closeAppend() error {
	f := j.f
	if f == nil {
		return nil
	}
	j.f = nil
	return closeFile(f)
}

// renameAttempts is how many times a compaction tries to put its scratch file in
// place before it gives up and waits for the next compaction.
const renameAttempts = 3

// renameInto renames the scratch file over the live log, retrying briefly.
//
// The retry is for Windows and for one caller above all: --diagnose replays this
// journal from ANOTHER process, and its read handle can make MoveFileEx fail
// here even though this process now holds none. A virus scanner or a backup
// agent does the same thing for the same reason. Three attempts inside 60 ms
// turn that into a pause instead of a whole compaction interval of unreclaimed
// growth. A rename does not fail transiently on Linux, so a genuinely failing
// one — a full disk, a read-only mount — costs 60 ms and returns the error it
// would have returned at once.
func renameInto(tmp, path string) error {
	delay := 20 * time.Millisecond
	for attempt := 1; ; attempt++ {
		err := renameFile(tmp, path)
		if err == nil || attempt == renameAttempts {
			return err
		}
		time.Sleep(delay)
		delay *= 2
	}
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
//
// A FAILED APPEND MUST LEAVE NO BYTES BEHIND, AND THIS IS WHAT THE 2026-08-08
// OUTAGE ACTUALLY COST. When the volume filled, a write landed SHORT: some of
// the record reached the file, the rest did not, and the call returned an error.
// The caller did the right thing with the error and never ACKed — but the
// half-record stayed in the log, and the next successful append wrote a whole
// record straight after it. That produced one unparsable line in the MIDDLE of
// the file, and replay stops at the first unparsable line, because a torn line
// is only ever supposed to be the last one.
//
// The five sidecars of the living deployment ran for eight hours after that with
// journals that replayed to 01:07:40 and no further. Nothing was visibly wrong:
// the processes held correct state in memory and kept ACKing correctly. Any
// restart in those eight hours would have silently reverted every one of them to
// its 01:07 state — the exact loss D2 exists to make impossible, arriving
// through the one path nobody had written a rule for.
//
// So the write is made all-or-nothing: on any error the file is truncated back
// to the length it had before the attempt. The caller still gets the error and
// still must not ACK; what changes is that the failure costs this record only,
// instead of every record written after it.
func (j *Journal) append(rec record) error {
	if j.readOnly {
		return ErrReadOnly
	}
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	before, statErr := j.openSize()
	n, err := j.f.Write(b)
	if err != nil {
		if statErr == nil {
			// Best effort: if this fails too the next Open truncates the torn
			// tail, which is correct as long as nothing was written after it —
			// and nothing will be, because the caller is about to see an error.
			_ = j.f.Truncate(before)
			_ = j.f.Sync()
		}
		return err
	}
	if n != len(b) {
		// io.Writer permits a short write only with a non-nil error, but the
		// cost of trusting that here is the whole log.
		if statErr == nil {
			_ = j.f.Truncate(before)
			_ = j.f.Sync()
		}
		return fmt.Errorf("journal: short write, %d of %d bytes", n, len(b))
	}
	return j.f.Sync()
}

// openSize is the log's length read from the open handle rather than the path.
func (j *Journal) openSize() (int64, error) {
	info, err := j.f.Stat()
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
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
	SentAtMs       *int64
	// DestSlot is the ONE exception to §7.3's no-rewrite rule, and it carries
	// its own evidence: a re-route under a proof of non-delivery (§9.2). Every
	// other entry keeps the destination it recorded.
	DestSlot     *int
	RerouteCount *int
	RerouteFrom  *int
	RerouteProof *string
	RerouteAtMs  *int64
	RefusedSlots []int
	// The FORWARD_RECEIPT block (§6.12, §22 B26). ForwardReceipts is the new
	// ABSOLUTE count, not a delta; the caller reads the current one and writes
	// count+1, so a replayed record can never double-count a forward.
	ForwardReceipts      *int
	ReceiptSessionID     *string
	ReceiptDestSlot      *int
	ReceiptForwardedAtMs *int64
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
		Note: u.Note, RelaySessionID: u.RelaySessionID, SentAtMs: u.SentAtMs,
		DestSlot: u.DestSlot, RerouteCount: u.RerouteCount, RerouteFrom: u.RerouteFrom,
		RerouteProof: u.RerouteProof, RerouteAtMs: u.RerouteAtMs,
		RefusedSlots:     append([]int(nil), u.RefusedSlots...),
		ForwardReceipts:  u.ForwardReceipts,
		ReceiptSessionID: u.ReceiptSessionID, ReceiptDestSlot: u.ReceiptDestSlot,
		ReceiptForwardedAtMs: u.ReceiptForwardedAtMs}
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

// PurgeExpired drops tombstones older than retention. An inbound tombstone
// that still owes its source a MIGRATION_ACK is not expired: AckedUpstream is
// the durable proof that the response left this sidecar. Tombstones must be
// durable, so the purge is durable too (contract-a.md §11.1).
//
// THE PURGE RECORD STAYS, EVEN THOUGH COMPACTION WOULD ERASE THE TOMBSTONE
// ANYWAY. Dropping the append would make a purge a memory-only act, and a crash
// between two compactions would then resurrect every tombstone the purge had
// removed. That is a safe direction to fail in — a resurrected tombstone only
// suppresses a duplicate for longer — but it is not the direction §11.1 states,
// and the saving does not justify the divergence: a purge record is one short
// line written once per tombstone that ever expires, while the bloat compaction
// exists to remove is the create record of every migration that ever ran, each
// carrying its payload. Purges were never the growth term.
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
		if st.AwaitsUpstreamAck() {
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
		_ = j.closeAppend()
		return err
	}
	return j.closeAppend()
}

func boolPtr(b bool) *bool    { return &b }
func intPtr(i int) *int       { return &i }
func int64Ptr(i int64) *int64 { return &i }
func strPtr(s string) *string { return &s }
