package archive

// THE ROLL-UP STATE SIDECAR: <data-dir>/rollup.jsonl, the durable half of the
// LEDGER FOLD — phase 1 of archive_rollup_design.md ("B — incremental
// append-only aggregate log").
//
// WHY IT EXISTS. Every answer the archive can give about what has crossed —
// lane totals, species crossings, the ancestry floor, the brain-coverage
// denominator, ledgerRecords — is a fold over migrations.jsonl, and until now
// the only place that fold was ever written down was memory. So a restart had
// to read EVERY RECORD EVER WRITTEN before it could serve anything, and the
// record-preserving restart holds the relay down for the whole of it
// (deploy/RESTART-POLICY.md), which makes replay time participant outage. About
// 90 s today, 75-96 minutes at the announced end of the run.
//
// THE SHAPE IS brains.jsonl'S, deliberately and not by accident: a version
// header, one line per aggregate record, APPEND ONLY WHAT CHANGED since the
// last save, last-writer-wins for each key on replay, a torn tail dropped, an
// unreadable file kept beside a fresh one, and a full rewrite only when the file
// exceeds a fixed multiple of its live content. The package already contains
// that discipline, argued and tested (brainsave.go's header), and a second
// persistence discipline for a second aggregate in one package is a cost that
// never stops being paid. Option A of the design — a whole-state snapshot on a
// timer — was refused for exactly the reason brainsave.go refused it: a full
// rewrite costs the size of the whole state every time it runs.
//
// WHAT IT DOES NOT PERSIST, and this is the honest half of the design.
//
//	THE DUPLICATE GUARD (dedup.go) IS NOT PERSISTABLE. Its fingerprint seeds are
//	drawn per process precisely so a participant cannot search offline for a
//	colliding migrationId, and dedup.go says so in as many words. It is rebuilt
//	by replaying the raw records inside cfg.DedupWindow, exactly as it is today,
//	AND IT IS NOW THE ONLY THING THAT MAKES A RESTART READ A RAW RECORD AT ALL.
//
// THE RESTART-TIME MODEL, stated as an equation because the whole design turns
// on it (phase 3):
//
//	raw records parsed at a restart = min(age of the raw window, cfg.DedupWindow)
//	                                  worth of records
//	wall time                       = that count / ~62 000-72 000 records a second
//	                                  measured on the service host
//
// At the shipped 48 h window and the map's measured rate that is about 7 M
// records and about 100 s. At the 1 h the guard actually needs once the fleet
// has crossed (see the knob in main.go) it is about 160 000 records and 2-3 s.
// The two numbers are published — replayRawRecords and replayRawSeconds on
// /api/status — so an operator reads what a restart cost rather than a model of
// it, and so monitor.sh can gate on the measurement.
//
// THERE IS A SECOND TERM AND THE VALIDATION FOUND IT: getting TO the window's
// start. On a plain segment or the live file that is an lseek and costs nothing.
// On a COMPRESSED segment a gzip stream has no seek, so it is a
// decompress-and-discard of every byte before the start — measured at 5.9 s to
// skip 1.80 GB of the one-off legacy segment on the validation box, against
// 6.3 s for the whole restart. In ordinary running the window's start is inside
// a recent segment, so the skip is at most one day; the legacy segment the
// migration produces is the worst case and it happens once. If it ever matters
// more than that, the fix is an offset index per segment and not a shorter
// window.
//
// PHASE 1 COULD NOT SAY THAT, and the difference is what phase 3 built. Two
// things used to force the raw scan back to max(DedupWindow, GenomeHorizon) —
// 720 h on the deployment, and the WHOLE RECORD on an archive with no horizon:
//
//	THE GENOME-GAP FETCH QUEUE was rebuilt by re-reading every crossing inside
//	the retention horizon. IT IS NOW PERSISTED, as the "gq" line kind, so the
//	horizon no longer binds the raw scan. It is bounded by the horizon exactly as
//	before, drained by the same pass, and drained again ON LOAD so a queue
//	written a week ago retires what has aged out since.
//
//	genomeGapsExpired was a function of HOW MUCH RAW WAS REPLAYED rather than of
//	the record — the one number that differed in phase 1's whole real-ledger
//	validation. It is now an ALL-TIME counter in the floor line, folded once per
//	crossing, and it means the same thing on a restart as on a first start.
//
// THE CURSOR IS A POSITION (segments.go, LedgerPos): the segment, the offset
// into its UNCOMPRESSED bytes, and the record number. Phase 1 kept a line count,
// because a byte offset could not be maintained exactly by a live append path
// that appended OUTSIDE the lock that folded the record. Phase 3 closed that
// gap at the source instead: every live append is now APPEND FIRST, FOLD SECOND
// (store.go, AppendAt), so the fold and the offset move together under one lock
// and the cursor can be a position that a restart SEEKS to. On a plain segment
// or the live file that is an lseek; on a gz it is a decompress-and-discard;
// either is far cheaper than parsing.
//
// A POSITION NAMING THE LIVE FILE GOES STALE WHEN THE LIVE FILE ROTATES, because
// the bytes move into a segment and a fresh live file starts at offset 0 with
// different records. Two things guard it: a rotation re-points every position
// this file holds at the segment that took the bytes (noteRotationLocked), and
// the floor line records the live file's own first record time so a load can tell
// a stale position from a good one and fall back to LedgerPositionOfRecord.

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"

	"multiverse/internal/contractb"
	"multiverse/internal/fsutil"
)

var errLineageRollupRebuild = errors.New(
	"archive: roll-up format 2 requires restart-archive.sh --rebuild-rollup before format 3 can start")

const (
	rollupSidecarName = "rollup.jsonl"
	// rollupVersion is the format's own version. A file whose header names a
	// version this build does not know is NOT half-read: it is refused whole and
	// the state is rebuilt by a full replay.
	//
	// 2 is phase 3: the cursor became a POSITION, the hour index with it, and the
	// genome-gap queue and its expiry counter joined the file. A version-1 file
	// carries no gap queue at all, so reading one as if it did would silently
	// empty the fetch queue — which is exactly the shape of failure the version
	// byte exists to refuse.
	// 3 adds the lineage-instance fold. Version 2 stored one mutable parent per
	// normalized name. A reused name could overwrite an older edge and create a
	// derived ancestry cycle, so version 2 cannot be upgraded without replaying
	// the ordered migration record.
	rollupVersion = 3
	// rollupSaveInterval is how often the fold is flushed, on the same tick loop
	// the brain sidecar rides.
	//
	// IT MEANS SOMETHING DIFFERENT HERE THAN IT DOES IN brains.jsonl, and the
	// difference is worth stating because the two constants look identical. There,
	// a save interval bounds LOSS: nothing replays a genome arrival, so what is
	// not written is gone. Here it bounds TAIL LENGTH only — every record behind
	// the cursor is still in the raw record and the next start folds it — so the
	// knob trades write volume against a few seconds of restart and never against
	// a fact.
	//
	// MEASURED on the 2026-08-16 ledger copy, at its 14.5 crossings a second and
	// the per-line sizes below: one save appends a median 47 KiB, which is
	// 123 MiB a day. That is an order of magnitude under the whole-state snapshot
	// this shape was chosen over (312 MB a day) and somewhat over the design's
	// 20-90 MB estimate, which did not count the two RECENT SAMPLES this file
	// persists to keep a restart's numbers exact: the lanes' five-minute flow
	// window and each species' six newest crossings are 80 % of the volume
	// (8.7 KiB a save without them). A five-minute interval is 20 MiB a day and a
	// tail of about 4 400 records. The 30 s here buys the shortest tail; an
	// operator who would rather buy the writes back has the trade in one number.
	rollupSaveInterval = 30 * time.Second
	// rollupCompactRatio and rollupCompactFloor are brainsave.go's numbers, for
	// brainsave.go's reason: a rewrite is worth its cost when the file holds this
	// many times the bytes its live content needs, and below the floor a
	// brand-new archive would compact on almost every save.
	rollupCompactRatio       = 3
	rollupCompactFloor int64 = 256 << 10
	// rollupPeerMax and rollupTypeMax bound the two NEW counters. Both are
	// bounded by construction on a healthy map — the peer set is the slot count
	// and the type set is the four record types — and both are keyed on strings a
	// peer influences, so both get a cap and an overflow counter rather than a
	// promise.
	rollupPeerMax = 256
	rollupTypeMax = 32
	// rollupIndexStrideMs is the resolution of the time->cursor index. One entry
	// an hour is 24 a day and about 2 200 for the whole announced run, which is
	// nothing beside the aggregates; it exists so a replay that only needs the
	// last N hours of RAW records can find where they start without parsing
	// everything before them.
	rollupIndexStrideMs int64 = 60 * 60 * 1000
	// rollupIndexMax bounds it anyway, at about 400 days.
	rollupIndexMax = 24 * 400
)

// What one live record costs on disk, near enough for the compaction test.
//
// MEASURED, on the first roll-up of the 2026-08-16 production ledger copy —
// 5,408,123 records, 855 species, 254 000 distinct genome fingerprints, 48
// lanes, 495 brain buckets — which wrote 6,511,571 bytes in 2,325 lines:
// a species line 700 B, a genome fingerprint 21 B, a lane line with its
// five-minute window 2.2 kB, a brain bucket 37 B, a dedup fingerprint 21 B and
// an index entry 55 B. The genome fingerprint sets are 82 % of the file, so they
// are the term the compaction threshold actually tracks.
const (
	rollupLiveSpeciesBytes int64 = 704
	rollupLiveLineageBytes int64 = 512
	rollupLiveGenomeBytes  int64 = 24
	rollupLiveLaneBytes    int64 = 128
	rollupLiveHopBytes     int64 = 15
	rollupLiveBucketBytes  int64 = 40
	rollupLiveDedupBytes   int64 = 24
	rollupLiveIndexBytes   int64 = 56
	// A gap line carries a 64-hex hash, a migration id, a peer id and a species
	// key. Measured at 208 B on the 2026-08-16 copy's 273 252-entry queue; the
	// production queue is one or two entries, because production has a genome
	// store and the copy does not.
	rollupLiveGapBytes   int64 = 208
	rollupLiveFixedBytes int64 = 2048
)

// rollupLine is one record of the sidecar. Every kind shares a line shape and is
// told apart by R, so the file is one stream `jq` can read and a torn tail can be
// dropped from — exactly as the ledger, the metrics file and brains.jsonl are.
//
// The kinds:
//
//	"h"  header: version, brain bucket width, when it was written
//	"f"  the FLOOR AND CURSOR line: what the file covers and where the record
//	     starts. Written on every save, last-writer-wins.
//	"sp" one species' scalar aggregate, last-writer-wins on the key
//	"sg" one species' NEW genome fingerprints since the last save, ADDITIVE
//	"si" one immutable lineage instance, last-writer-wins on the instance id
//	"sl" the species ledger's own globals, last-writer-wins
//	"ln" one directed lane's counters, last-writer-wins on (from, to, edge)
//	"rc" the record counters: total and per type, last-writer-wins
//	"rp" the per-peer record counters, last-writer-wins
//	"bs" one brain bucket's COVERAGE DENOMINATOR, last-writer-wins
//	"bd" NEW entries of the brain seen-dedup window for one bucket, ADDITIVE
//	"gq" one genome-gap queue entry, ADDITIVE, with a TOMBSTONE ("x") for one
//	     that has been served or retired
//	"ix" one entry of the time->position index, last-writer-wins on the hour
//
// The two ADDITIVE kinds are sets, and a set is the one thing last-writer-wins
// cannot express cheaply: writing a species' whole 8 192-fingerprint set every
// time it crosses is the full-rewrite cost this shape exists to refuse. They are
// unioned on load and written whole by a compaction.
type rollupLine struct {
	R string `json:"r"`

	// Header ("h").
	V        int   `json:"v,omitempty"`
	BucketMs int64 `json:"bucketMs,omitempty"`
	SavedAt  int64 `json:"savedAtMs,omitempty"`

	// Floor and cursor ("f").
	//
	// Segment and Offset are the cursor's POSITION in the segmented ledger and
	// CoveredRecords is the fold's own record count at that position; the three
	// are written together and are exact together, because the live path folds
	// under the same lock that takes the offset (store.go, AppendAt).
	//
	// LiveFirstMs is the live file's first record time at the moment of the save,
	// and it is what makes a position naming the live file ("" segment) safe to
	// trust: a rotation empties that file and starts a new one, so a position
	// naming it after a rotation would seek into the wrong bytes. A mismatch here
	// means every "" position in the file is stale and is dropped.
	CoveredRecords int    `json:"covRecords,omitempty"`
	Segment        string `json:"seg,omitempty"`
	Offset         int64  `json:"off,omitempty"`
	LiveFirstMs    int64  `json:"liveFirstMs,omitempty"`
	SkippedLines   int    `json:"skippedLines,omitempty"`
	FirstMs        int64  `json:"firstMs,omitempty"`
	LastMs         int64  `json:"lastMs,omitempty"`
	RawFromMs      int64  `json:"rawFromMs,omitempty"`
	Frontier       int64  `json:"frontierMs,omitempty"`
	BucketsDropped int    `json:"bucketsDropped,omitempty"`
	// GapsExpired and DupRefused are ALL-TIME counters, and both are here rather
	// than derived because both used to be functions of how much raw a restart
	// happened to replay. See the file header.
	GapsExpired int `json:"gapsExpired,omitempty"`
	DupRefused  int `json:"dupRefused,omitempty"`

	// Species ("sp"), and the key of "sg" too.
	K            string            `json:"k,omitempty"`
	Crossings    int               `json:"c,omitempty"`
	SpFirstMs    int64             `json:"f1,omitempty"`
	SpLastMs     int64             `json:"l1,omitempty"`
	GenomesTrunc bool              `json:"gt,omitempty"`
	Recent       []SpeciesCrossing `json:"rec,omitempty"`
	Parent       string            `json:"p,omitempty"`
	ParentKey    string            `json:"pk,omitempty"`
	ParentAtMs   int64             `json:"pa,omitempty"`
	GenomeHash   string            `json:"gh,omitempty"`
	GenomeAtMs   int64             `json:"ga,omitempty"`

	// Lineage instance ("si"). K is the instance id. NameKey is the portable
	// normalized name. ParentID is the immutable parent instance. ParentKnown
	// distinguishes a recorded root from an unresolved parent placeholder.
	NameKey         string        `json:"nk,omitempty"`
	Name            string        `json:"nm,omitempty"`
	ParentID        string        `json:"pi,omitempty"`
	ParentKnown     bool          `json:"pkn,omitempty"`
	Placeholder     bool          `json:"ph,omitempty"`
	LineageConflict bool          `json:"cf,omitempty"`
	SeenAt          map[int]int64 `json:"wa,omitempty"`

	// Genome fingerprints ("sg") and brain dedup fingerprints ("bd"). BOTH ARE
	// DECIMAL STRINGS, not JSON numbers: a 64-bit fingerprint above 2^53 does not
	// survive a reader that parses numbers as doubles, and this file is meant to
	// be read with `jq` (store.go's store-choice rule 1).
	FP []string `json:"fp,omitempty"`

	// Species ledger globals ("sl").
	Overflow        int   `json:"ov,omitempty"`
	LineageOverflow int   `json:"lo,omitempty"`
	Edges           int   `json:"ed,omitempty"`
	EdgeFirstMs     int64 `json:"ef,omitempty"`

	// Lane ("ln").
	From        int     `json:"fr,omitempty"`
	To          int     `json:"to,omitempty"`
	Edge        string  `json:"e,omitempty"`
	Total       int     `json:"n,omitempty"`
	LaneFirstMs int64   `json:"lf,omitempty"`
	LaneLastMs  int64   `json:"la,omitempty"`
	RecentMs    []int64 `json:"rm,omitempty"`

	// Record counters ("rc") and per-peer counters ("rp").
	Records      int            `json:"records,omitempty"`
	ByType       map[string]int `json:"ty,omitempty"`
	TypeOverflow int            `json:"tyOv,omitempty"`
	ByPeer       map[string]int `json:"pe,omitempty"`
	PeerOverflow int            `json:"peOv,omitempty"`

	// Brain coverage ("bs") and its dedup window ("bd").
	T    int64 `json:"t,omitempty"`
	Seen int   `json:"s,omitempty"`

	// Genome gap ("gq"). The key is K, the hash. Gone is the TOMBSTONE: the gap
	// was served or retired and must not come back on the next load.
	//
	// What is here is what the pump and the brain aggregate cannot do without.
	// CrossedAt is the horizon's only lawful clock (§23, B34; eviction.go), and
	// Migrant/SpeciesKey are what brainhist.go needs at the moment the blob
	// finally lands, because by then the record that asked for it is long gone.
	//
	// WHAT IS DELIBERATELY NOT HERE: attempts, the backoff ladder and the set of
	// peers already asked. A restart has always reset the ladder — fetch.firstSeen
	// is the process's own clock — and persisting the attempt count would restore
	// a queue that is already deep in its backoff and starve it for as long as the
	// ladder says. The ladder is about THIS process's conversation with its peers;
	// the gap is about the record.
	Gone        bool   `json:"x,omitempty"`
	CrossedAt   int64  `json:"ca,omitempty"`
	GapPeer     string `json:"gp,omitempty"`
	GapMigID    string `json:"gm,omitempty"`
	GapEntityID int32  `json:"ge,omitempty"`
	GapMigrant  bool   `json:"mg,omitempty"`
	GapSpecies  string `json:"sk,omitempty"`

	// Index ("ix"): an hour, and the POSITION the raw stream had reached when its
	// first record arrived. AtRecord is that position's record number.
	Hour     int64  `json:"hr,omitempty"`
	AtSeg    string `json:"is,omitempty"`
	AtOffset int64  `json:"io,omitempty"`
	AtRecord int64  `json:"rn,omitempty"`
}

// pos and atPos read the two positions a line can carry.
func (l rollupLine) pos() LedgerPos {
	return LedgerPos{Segment: l.Segment, Offset: l.Offset, Record: int64(l.CoveredRecords)}
}

func (l rollupLine) atPos() LedgerPos {
	return LedgerPos{Segment: l.AtSeg, Offset: l.AtOffset, Record: l.AtRecord}
}

// ------------------------------------------------------------- the cursor

// THE CURSOR IS A LedgerPos (segments.go): the segment, the byte offset into its
// UNCOMPRESSED content, and the RECORD NUMBER the fold had reached there.
//
// The record number is what decides, on a tail replay, whether a delivered
// record is already in the aggregates: ScanLedgerFrom continues numbering from
// the position it is handed, so a record whose number is at or below the saved
// cursor's is one the sidecar already holds. It is exact because the fold and
// the offset move under one lock (store.go, AppendAt).
//
// The offset is what makes the restart cheap, and the record number is what
// makes it correct. Phase 1 had only the second half.
//
// SkippedLines is persisted beside it for a reason that is easy to miss: the
// ledger is append-only, so its damage is permanent, and an archive that stopped
// counting a damaged line because it stopped re-reading it would quietly lose
// the number that explains why ledgerRecords and `wc -l` disagree. A raw scan
// that reaches back past the cursor must not count that damage twice either,
// which is what scanLedgerFromFloor's damage floor is for.

// ------------------------------------------------------------- the state

// rollupSpecies is one species' scalar aggregate as the file holds it.
type rollupSpecies struct {
	crossings    int
	firstMs      int64
	lastMs       int64
	genomesTrunc bool
	recent       []SpeciesCrossing
	parent       string
	parentKey    string
	parentAtMs   int64
	genomeHash   string
	genomeAtMs   int64
}

type rollupLineage struct {
	nameKey     string
	name        string
	parentKnown bool
	parentID    string
	parentKey   string
	parent      string
	placeholder bool
	conflict    bool
	crossings   int
	firstMs     int64
	lastMs      int64
	recent      []SpeciesCrossing
	genomeHash  string
	genomeAtMs  int64
	seenAt      map[int]int64
}

type rollupLane struct {
	total   int
	firstAt int64
	lastAt  int64
	recent  []int64
}

// rollupGap is one genome-gap queue entry as the file holds it. See the "gq"
// fields on rollupLine for what is here and what is deliberately not.
type rollupGap struct {
	crossedAt   int64
	sourcePeer  string
	migrationID string
	entityID    int32
	migrant     bool
	speciesKey  string
}

// rollupState is a whole sidecar, parsed. Parsing and APPLYING are separate on
// purpose: last-writer-wins and set-union are decided here, over the whole file,
// and only then is the result handed to the aggregates in an order they can
// accept (oldest bucket first, so a full brain map evicts the way it would have).
type rollupState struct {
	savedAtMs int64
	// cursor is the position AND the record count the aggregates cover.
	// liveFirstMs is what says whether a cursor naming the live file is still
	// about the same live file; see the floor line's own comment.
	cursor         LedgerPos
	liveFirstMs    int64
	skippedLines   int
	firstMs        int64
	lastMs         int64
	rawFromMs      int64
	frontier       int64
	bucketsDropped int
	gapsExpired    int
	dupRefused     int

	species         map[string]*rollupSpecies
	genomes         map[string][]uint64
	lineages        map[string]*rollupLineage
	overflow        int
	lineageOverflow int
	edges           int
	edgeFirstMs     int64

	lanes map[lanePair]*rollupLane

	records      int
	byType       map[string]int
	typeOverflow int
	byPeer       map[string]int
	peerOverflow int

	seen  map[int64]int
	dedup map[int64][]uint64

	// gaps is the live queue: an entry added by a "gq" line and removed by its
	// tombstone, resolved over the whole file before anything is installed.
	gaps map[string]*rollupGap

	index map[int64]LedgerPos
}

func newRollupState() *rollupState {
	return &rollupState{
		species:  map[string]*rollupSpecies{},
		genomes:  map[string][]uint64{},
		lineages: map[string]*rollupLineage{},
		lanes:    map[lanePair]*rollupLane{},
		byType:   map[string]int{},
		byPeer:   map[string]int{},
		seen:     map[int64]int{},
		dedup:    map[int64][]uint64{},
		gaps:     map[string]*rollupGap{},
		index:    map[int64]LedgerPos{},
	}
}

// lineAtOrBefore is the cursor of the newest index entry at or before atMs, and
// false when the index cannot answer — which is every archive whose sidecar
// predates the requested time. A caller that gets false MUST scan from 0: an
// index that cannot say where a window starts is not a licence to guess.
func (st *rollupState) lineAtOrBefore(atMs int64) (LedgerPos, bool) {
	if st == nil || len(st.index) == 0 || atMs <= 0 {
		return LedgerPos{}, false
	}
	best, found := LedgerPos{}, false
	bestHour := int64(0)
	for hour, cur := range st.index {
		if hour > atMs {
			continue
		}
		if !found || hour > bestHour {
			best, bestHour, found = cur, hour, true
		}
	}
	if !found {
		return LedgerPos{}, false
	}
	// The oldest entry the index holds is only a floor for the whole file when
	// the file's own first record is at or after it. If the index starts later
	// than the record does, an answer for a time before it would skip records the
	// caller asked to see.
	oldest := int64(0)
	for hour := range st.index {
		if oldest == 0 || hour < oldest {
			oldest = hour
		}
	}
	if atMs < oldest {
		return LedgerPos{}, false
	}
	return best, true
}

// ------------------------------------------------------------- loading

// loadRollupState reads the sidecar. It returns (nil, true) when there is no
// file at all — a new archive, or the first run after this feature existed —
// and (nil, false) when a file exists and cannot be used, which is a LOSS: the
// caller keeps the bytes beside a fresh file and rebuilds by a full replay. A
// recognized format-2 predecessor returns errLineageRollupRebuild instead. Its
// missing lineage identity requires the recorded raw-completeness operation.
func loadRollupState(path string) (*rollupState, bool, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, true, nil
	}
	if err != nil {
		return nil, false, err
	}
	if len(b) == 0 {
		return nil, true, nil
	}
	lines := splitLines(b)
	if len(lines) == 0 {
		return nil, false, nil
	}
	var head rollupLine
	if json.Unmarshal(lines[0], &head) != nil || head.R != "h" {
		return nil, false, nil
	}
	if head.V == 2 && rollupVersion == 3 {
		// Version 2 has a valid durable aggregate, but it has only one mutable
		// parent per name. An ordinary fallback replay can start after raw
		// segments retire and silently lose the ordered evidence needed for the
		// instance graph. The recorded rebuild operation proves raw completeness
		// and moves the old sidecar before it starts this binary.
		return nil, false, errLineageRollupRebuild
	}
	if head.V != rollupVersion || head.BucketMs != BrainBucketMs {
		// A DIFFERENT BRAIN BUCKET WIDTH IS A DIFFERENT FILE, for brainsave.go's
		// reason: coverage counts written at another resolution cannot be merged
		// with these without inventing a within-bucket distribution.
		return nil, false, nil
	}
	st := newRollupState()
	st.savedAtMs = head.SavedAt
	// THE FLOOR LINE IS THE COMMIT MARKER, and this is the one place this file
	// departs from brains.jsonl's replay. A save appends the keys that moved and
	// THEN the floor line that says how much of the record they account for, so a
	// save interrupted between the two would otherwise leave the aggregates
	// ahead of the cursor — and a tail replay starting at that cursor would fold
	// the same records a second time. Reading the batch and committing it only
	// when its floor line arrives makes a save all-or-nothing on replay, which is
	// the discipline the sidecar journal already applies to its own writes.
	var batch []rollupLine
	for i := 1; i < len(lines); i++ {
		var rec rollupLine
		if json.Unmarshal(lines[i], &rec) != nil {
			if i == len(lines)-1 {
				// The torn tail of an interrupted write. Everything before it is
				// good, which is the rule the ledger, the metrics file and the
				// brain sidecar all apply.
				break
			}
			return nil, false, nil
		}
		if rec.R != "f" {
			batch = append(batch, rec)
			continue
		}
		for _, b := range batch {
			st.apply(b)
		}
		st.apply(rec)
		batch = batch[:0]
	}
	// Whatever is left never got its floor line: the save that wrote it did not
	// finish, and it is dropped WHOLE rather than half-applied.
	return st, true, nil
}

func (st *rollupState) apply(rec rollupLine) {
	switch rec.R {
	case "f":
		// The floor line's own clock, not the header's: the header is written
		// when the file is created or compacted, and what a reader wants to know
		// is when the STATE was last written.
		if rec.SavedAt > 0 {
			st.savedAtMs = rec.SavedAt
		}
		st.cursor = rec.pos()
		st.liveFirstMs = rec.LiveFirstMs
		st.skippedLines = rec.SkippedLines
		st.firstMs, st.lastMs = rec.FirstMs, rec.LastMs
		st.rawFromMs = rec.RawFromMs
		st.frontier = rec.Frontier
		st.bucketsDropped = rec.BucketsDropped
		st.gapsExpired = rec.GapsExpired
		st.dupRefused = rec.DupRefused
	case "sp":
		if rec.K == "" {
			return
		}
		st.species[rec.K] = &rollupSpecies{
			crossings: rec.Crossings, firstMs: rec.SpFirstMs, lastMs: rec.SpLastMs,
			genomesTrunc: rec.GenomesTrunc, recent: rec.Recent,
			parent: rec.Parent, parentKey: rec.ParentKey, parentAtMs: rec.ParentAtMs,
			genomeHash: rec.GenomeHash, genomeAtMs: rec.GenomeAtMs,
		}
	case "sg":
		if rec.K == "" {
			return
		}
		st.genomes[rec.K] = append(st.genomes[rec.K], parseFingerprints(rec.FP)...)
	case "si":
		if rec.K == "" || rec.NameKey == "" {
			return
		}
		seen := map[int]int64{}
		for slot, at := range rec.SeenAt {
			seen[slot] = at
		}
		st.lineages[rec.K] = &rollupLineage{
			nameKey: rec.NameKey, name: rec.Name,
			parentKnown: rec.ParentKnown, parentID: rec.ParentID,
			parentKey: rec.ParentKey, parent: rec.Parent,
			placeholder: rec.Placeholder, conflict: rec.LineageConflict,
			crossings: rec.Crossings, firstMs: rec.SpFirstMs, lastMs: rec.SpLastMs,
			recent:     append([]SpeciesCrossing(nil), rec.Recent...),
			genomeHash: rec.GenomeHash, genomeAtMs: rec.GenomeAtMs,
			seenAt: seen,
		}
	case "sl":
		st.overflow, st.lineageOverflow = rec.Overflow, rec.LineageOverflow
		st.edges, st.edgeFirstMs = rec.Edges, rec.EdgeFirstMs
	case "ln":
		st.lanes[lanePair{from: rec.From, to: rec.To, edge: rec.Edge}] = &rollupLane{
			total: rec.Total, firstAt: rec.LaneFirstMs, lastAt: rec.LaneLastMs,
			recent: rec.RecentMs,
		}
	case "rc":
		st.records = rec.Records
		st.byType = map[string]int{}
		for k, v := range rec.ByType {
			st.byType[k] = v
		}
		st.typeOverflow = rec.TypeOverflow
	case "rp":
		st.byPeer = map[string]int{}
		for k, v := range rec.ByPeer {
			st.byPeer[k] = v
		}
		st.peerOverflow = rec.PeerOverflow
	case "bs":
		if rec.T > 0 {
			st.seen[rec.T] = rec.Seen
		}
	case "bd":
		if rec.T > 0 {
			st.dedup[rec.T] = append(st.dedup[rec.T], parseFingerprints(rec.FP)...)
		}
	case "gq":
		if rec.K == "" {
			return
		}
		if rec.Gone {
			// The tombstone. A gap that was served or retired must not come back
			// on the next load, and a set is the one thing last-writer-wins
			// cannot express: the whole queue would have to be rewritten every
			// time one entry moved.
			delete(st.gaps, rec.K)
			return
		}
		st.gaps[rec.K] = &rollupGap{
			crossedAt: rec.CrossedAt, sourcePeer: rec.GapPeer,
			migrationID: rec.GapMigID, entityID: rec.GapEntityID,
			migrant: rec.GapMigrant, speciesKey: rec.GapSpecies,
		}
	case "ix":
		if rec.Hour > 0 && len(st.index) < rollupIndexMax {
			st.index[rec.Hour] = rec.atPos()
		}
	}
}

func parseFingerprints(in []string) []uint64 {
	if len(in) == 0 {
		return nil
	}
	out := make([]uint64, 0, len(in))
	for _, s := range in {
		v, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			continue
		}
		out = append(out, v)
	}
	return out
}

func dumpFingerprints(in []uint64) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		out = append(out, strconv.FormatUint(v, 10))
	}
	return out
}

// applyRollupState installs a parsed sidecar into the archive's aggregates. It
// runs in New before the tail replay and before anything else is started, so it
// takes no locks it has to fight for; it takes them anyway, because a fold site
// that is only correct when nothing else is running is a fold site that will be
// called from somewhere else one day.
func (a *Archive) applyRollupState(st *rollupState) {
	if st == nil {
		return
	}
	a.mu.Lock()
	for key, sp := range st.species {
		e := &speciesAgg{
			crossings: sp.crossings, firstMs: sp.firstMs, lastMs: sp.lastMs,
			genomes: map[uint64]struct{}{}, genomesTruncated: sp.genomesTrunc,
			recent: append([]SpeciesCrossing(nil), sp.recent...),
			parent: sp.parent, parentKey: sp.parentKey, parentAtMs: sp.parentAtMs,
			genomeHash: sp.genomeHash, genomeAtMs: sp.genomeAtMs,
		}
		for _, fp := range st.genomes[key] {
			if len(e.genomes) >= speciesGenomeMax {
				e.genomesTruncated = true
				break
			}
			e.genomes[fp] = struct{}{}
		}
		a.species.byKey[key] = e
	}
	a.species.lineage = newSpeciesLineageLedger()
	for id, li := range st.lineages {
		seen := map[int]int64{}
		for slot, at := range li.seenAt {
			seen[slot] = at
		}
		a.species.lineage.byID[id] = &speciesLineageInstance{
			id: id, nameKey: li.nameKey, name: li.name,
			parentKnown: li.parentKnown, parentID: li.parentID,
			parentKey: li.parentKey, parent: li.parent,
			placeholder: li.placeholder, conflict: li.conflict,
			crossings: li.crossings, firstMs: li.firstMs, lastMs: li.lastMs,
			recent:     append([]SpeciesCrossing(nil), li.recent...),
			genomeHash: li.genomeHash, genomeAtMs: li.genomeAtMs,
			seenAt: seen,
		}
	}
	a.species.lineage.overflow = st.lineageOverflow
	a.species.lineage.rebuildIndexes()
	a.species.overflow = st.overflow
	a.species.edges = st.edges
	a.species.edgeFirstMs = st.edgeFirstMs
	for key, l := range st.lanes {
		a.lanes[key] = &lane{total: l.total, firstAt: l.firstAt, lastAt: l.lastAt,
			recent: append([]int64(nil), l.recent...)}
	}
	// THE FLOOR LINE IS THE AUTHORITY ON THE RECORD COUNT, not the "rc" line:
	// the two carry the same number, written in the same batch, but the floor is
	// the commit marker and the cursor's own record field has to agree with it.
	a.recordCount = int(st.cursor.Record)
	a.ledgerPos = st.cursor
	a.ledgerSkipped = st.skippedLines
	a.evict.gapsExpired = st.gapsExpired
	a.duplicatesRefused = st.dupRefused
	for hash, g := range st.gaps {
		f := &fetch{
			hash:        hash,
			sourcePeer:  g.sourcePeer,
			migrationID: g.migrationID,
			entityID:    g.entityID,
			crossedAt:   time.UnixMilli(g.crossedAt),
			migrant:     g.migrant,
			speciesKey:  g.speciesKey,
		}
		if g.crossedAt <= 0 {
			f.crossedAt = time.Time{}
		}
		a.pending[hash] = f
	}
	// THE ROUND-ROBIN ORDER IS REBUILT BY THE CROSSING'S OWN CLOCK, oldest first,
	// which is the order the ledger would have produced. Go's map iteration is
	// random and pumpFetches walks this slice, so loading in map order would give
	// a restart a different fetch order every time for no reason anyone could
	// explain from the record.
	a.pendingOrder = a.pendingOrder[:0]
	for hash := range st.gaps {
		a.pendingOrder = append(a.pendingOrder, hash)
	}
	sort.Slice(a.pendingOrder, func(i, j int) bool {
		x, y := st.gaps[a.pendingOrder[i]], st.gaps[a.pendingOrder[j]]
		if x.crossedAt != y.crossedAt {
			return x.crossedAt < y.crossedAt
		}
		return a.pendingOrder[i] < a.pendingOrder[j]
	})
	a.pendingHead = 0
	a.tally.byType = map[string]int{}
	for k, v := range st.byType {
		a.tally.byType[k] = v
	}
	a.tally.typeOverflow = st.typeOverflow
	a.tally.byPeer = map[string]int{}
	for k, v := range st.byPeer {
		a.tally.byPeer[k] = v
	}
	a.tally.peerOverflow = st.peerOverflow
	a.tally.firstMs, a.tally.lastMs = st.firstMs, st.lastMs
	a.rollupIndex = map[int64]LedgerPos{}
	for k, v := range st.index {
		a.rollupIndex[k] = v
	}
	a.rollupCovered = st.cursor
	a.rollupSavedAtMs = st.savedAtMs
	a.mu.Unlock()

	g := a.brainAgg
	g.mu.Lock()
	// THE FRONTIER FIRST, and the whole seen map in bucket order. mark() drops
	// dedup sets that fall out of the window as the frontier advances, and
	// bucketFor evicts the oldest bucket when the retained year is full — so a
	// load in map order would evict a different set from the one the fold did.
	g.frontier = st.frontier
	g.bucketsDropped = st.bucketsDropped
	keys := make([]int64, 0, len(st.seen))
	for k := range st.seen {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	for _, k := range keys {
		if b := g.bucketFor(k); b != nil {
			b.seen = st.seen[k]
		}
	}
	cut := g.frontier - int64(brainDedupBuckets)*BrainBucketMs
	for k, fps := range st.dedup {
		if k < cut {
			// Outside the dedup window the fold does not deduplicate at all
			// (mark's own rule), so carrying the set forward would only cost
			// memory for a decision nothing makes.
			continue
		}
		set := g.dedup[k]
		if set == nil {
			set = map[uint64]uint8{}
			g.dedup[k] = set
		}
		for _, fp := range fps {
			// BIT 1 ONLY. Bit 2 is the HELD fold's, whose durable half is
			// brains.jsonl; the two files own the two halves of the brain
			// aggregate and neither writes the other's.
			set[fp] |= 1
		}
	}
	g.mu.Unlock()
}

// ------------------------------------------------------------- the replay plan

// replayPlan is where a restart starts reading and what it folds on the way.
//
// TWO CUT POINTS, ONE PASS.
//
//	From        where the RAW SCAN starts, because the duplicate guard cannot
//	            be persisted (dedup.go: its fingerprint seeds are per process on
//	            purpose) and can only be rebuilt from the raw record.
//	Floor       the sidecar's own cursor. A record at or before it is already in
//	            the aggregates and is read for the guard alone; a damaged line at
//	            or before it is already in ledgerSkippedLines.
//
// From is at or before Floor, and the distance between them is the WHOLE COST OF
// A RESTART: exactly cfg.DedupWindow of records, or the whole raw window when
// that is shorter. Nothing else reaches back — the genome-gap queue used to, and
// is persisted now (see the file header).
type replayPlan struct {
	From  LedgerPos
	Floor LedgerPos
	// RawSpan is the span of raw records the plan needs rebuilt. Published in the
	// log so an operator can see what a restart cost and why.
	RawSpan time.Duration
	// FromSidecar says the aggregates came from a sidecar rather than a replay.
	FromSidecar bool
	// Converted says the saved position could not be used as it stood and was
	// rebuilt from the record count by one walk (LedgerPositionOfRecord). It is
	// correct and it is slow, so it says so.
	Converted bool
}

// planReplay decides both cut points.
//
// dir is where the raw record is, because a plan may have to convert a record
// count into a position, and ledger is the open live file, because a position
// naming it is only usable if it is still the same file.
func (a *Archive) planReplay(st *rollupState, now time.Time) replayPlan {
	if st == nil || st.cursor.Record <= 0 {
		// No sidecar, or one whose first floor line never landed. This is the
		// replay the archive has always done: everything, folded from zero.
		return replayPlan{}
	}
	plan := replayPlan{Floor: st.cursor, FromSidecar: true}

	// IS THE SAVED POSITION STILL ABOUT THE FILE IT NAMES? A position inside a
	// closed segment always is: segments are immutable and a retired one is
	// caught by ScanLedgerFrom itself. A position naming the LIVE file is only
	// good while that live file is the one it was taken from — a rotation moves
	// those bytes into a segment and starts a fresh file at offset 0, so the same
	// offset would then seek into the middle of different records.
	// A cursor at offset 0 of the live file is safe whatever has happened since:
	// it can only have been written while that file was empty, an empty live file
	// never rotates (store.go, rotateLocked returns early), and offset 0 is the
	// start of whatever live file exists now — so everything before it is folded
	// and everything after it is not.
	liveOK := st.cursor.Segment != "" || st.cursor.Offset == 0 ||
		a.liveFileMatches(st.liveFirstMs)
	if !liveOK {
		// One walk of what is on the host, once, and the position is persisted
		// from now on. This is P's LedgerPositionOfRecord: the once-ever bridge,
		// used here for the rare case rather than for every start.
		pos, err := LedgerPositionOfRecord(a.cfg.DataDir, st.cursor.Record)
		if err != nil {
			return replayPlan{}
		}
		// The walk numbers records over WHAT IS PRESENT. When segments have been
		// retired that is fewer than the fold covers, and the walk stops at the
		// end rather than at the count — in which case everything still on the
		// host is behind the cursor, which is exactly what this says.
		pos.Record = st.cursor.Record
		plan.Floor, plan.Converted = pos, true
	}
	plan.From = plan.Floor

	// THE RAW SCAN'S START: one duplicate window behind the floor. cfg.DedupWindow
	// is never 0 — Config's own validation gives it contractb.ArchiveDedupWindow
	// — so this is always a bounded span, unlike the horizon it replaced.
	span := a.cfg.DedupWindow
	if span <= 0 {
		span = contractb.ArchiveDedupWindow
	}
	plan.RawSpan = span
	// One stride of margin, because the index answers on an hour boundary and a
	// window that started inside an hour must not begin after its own start.
	at := now.Add(-span).UnixMilli() - rollupIndexStrideMs
	pos, ok := st.lineAtOrBefore(at)
	if !ok {
		// The index cannot say where the window starts, so the scan starts at the
		// beginning of what is on the host. An index that cannot answer is not a
		// licence to guess, and after phase 2 "the beginning" is the raw window's
		// own start rather than the beginning of time.
		plan.From = LedgerPos{}
		return plan
	}
	if !liveOK && pos.Segment == "" {
		// Same staleness, same answer: an hour-index entry naming the live file
		// is about a live file that has rotated away.
		plan.From = LedgerPos{}
		return plan
	}
	if pos.Record < plan.Floor.Record {
		plan.From = pos
	}
	return plan
}

// resolveFloor checks the saved cursor against the raw lines that are actually
// on this host, and returns the floor the scan should use.
//
// There are exactly two cases when the cursor's segment has gone, and the
// difference between them is the difference between a hole and a doubled
// counter.
//
//	THE CURSOR IS OLDER THAN EVERYTHING STILL HERE. This is the real case:
//	retirement removes the OLDEST segments and the cursor moves forward every
//	thirty seconds, so a cursor that has been retired belongs to a state older
//	than the raw window — an archive restored from a backup, or one that was down
//	for longer than the window. Everything present is therefore after it and none
//	of it has been folded, so the floor becomes the zero position and all of it
//	is. The hole between the cursor and the oldest surviving segment is real and
//	unrecoverable on this host; deploy/coldcopy.sh --restore is what closes it.
//
//	THE CURSOR IS INSIDE OR AFTER WHAT IS STILL HERE. Retirement cannot produce
//	this; a raw record replaced under a running deployment can. The aggregates
//	have already eaten these records and re-eating them would DOUBLE EVERY
//	COUNTER, which is the one failure this design cannot tolerate because it
//	looks like data. The floor is left naming the missing segment, which
//	scanLedgerFromFloor reads as "nothing here is past it": the duplicate guard is
//	rebuilt and no aggregate moves.
//
// Either way the archive says so — replayFromRetired is on the status surface.
func (a *Archive) resolveFloor(dir string, cursor LedgerPos) (floor LedgerPos,
	missing, foldAll bool) {

	if cursor.Segment == "" {
		// The live file is always present, so a cursor naming it is never missing.
		return cursor, false, false
	}
	srcs, err := ledgerSources(dir)
	if err != nil {
		return cursor, false, false
	}
	oldest := ""
	for _, s := range srcs {
		if s.Name == cursor.Segment {
			return cursor, false, false
		}
		if oldest == "" && s.Name != "" {
			oldest = s.Name
		}
	}
	want, okCursor := parseSegmentName(cursor.Segment)
	have, okOldest := parseSegmentName(oldest)
	if okCursor && (!okOldest || segmentOrder(want, have)) {
		return LedgerPos{}, true, true
	}
	return cursor, true, false
}

// liveFileMatches reports whether the live file is still the one a saved
// position was taken from. An empty live file at both ends is a match: the
// position can then only be offset 0, which is the start of the file either way.
func (a *Archive) liveFileMatches(savedFirstMs int64) bool {
	if a.ledger == nil {
		return false
	}
	return a.ledger.FirstRecordedAtMs() == savedFirstMs
}

// expireLoadedGapsLocked drains the queue a sidecar just restored of everything
// that has aged past the retention horizon while the archive was down.
//
// IT IS THE HALF OF THE REBUILD THE FILE CANNOT DO. A full replay never queued
// an aged crossing in the first place (trackLocked), so a loaded queue has to be
// put in the same state a full replay would have produced, or an archive
// restarted after a fortnight would resume asking for genomes it retired a
// fortnight ago. The counter moves with it, which is what keeps
// genomeGapsExpired an all-time number rather than a fact about this process.
//
// A gap whose blob has since arrived by some other route is dropped the same
// way, for the reason pumpFetches drops it: the store already has it.
func (a *Archive) expireLoadedGapsLocked(now time.Time) (expired, held int) {
	if len(a.pending) == 0 {
		return 0, 0
	}
	for hash, f := range a.pending {
		switch {
		case a.heldOrColdLocked(hash):
			delete(a.pending, hash)
			a.markGapGoneLocked(hash)
			held++
		case a.gapPastHorizonLocked(f.crossedAt, now):
			delete(a.pending, hash)
			a.markGapGoneLocked(hash)
			a.evict.gapsExpired++
			a.rollupDirty.counts = true
			expired++
		}
	}
	if expired > 0 || held > 0 {
		order := a.pendingOrder[:0]
		for _, hash := range a.pendingOrder {
			if _, live := a.pending[hash]; live {
				order = append(order, hash)
			}
		}
		a.pendingOrder = order
		a.pendingHead = 0
	}
	return expired, held
}

// ------------------------------------------------------------- dirty tracking

// rollupDirty is WHAT HAS CHANGED SINCE THE LAST SAVE, per key, and it is the
// one place this design can go quietly wrong: an incrementally persisted fold
// whose write site forgets to mark its key produces a number that is stale
// rather than absent, and nothing on any page says so.
//
// THE WRITE SITES, ENUMERATED. Every one of them is asserted by
// TestEveryPersistedWriteSiteMarksItsKeyDirty in rollup_test.go, and a new one
// belongs in both lists:
//
//	species (sp)   observeSpeciesLocked            species.go
//	genomes (sg)   observeSpeciesLocked            species.go   (new fingerprint only)
//	lineage (si)   observeSpeciesLocked            species_lineage.go
//	ledger  (sl)   observeSpeciesLocked            species.go   (overflow/edges/floor)
//	lanes   (ln)   observeLaneLocked               archive.go
//	counts  (rc)   countRecordLocked               rollup.go    (all five call sites)
//	peers   (rp)   countRecordLocked               rollup.go
//	index   (ix)   countRecordLocked               rollup.go
//	seen    (bs)   observeBrainSeen                brainhist.go
//	dedup   (bd)   observeBrainSeen                brainhist.go
//	floor   (f)    every save                      rollup.go    (unconditional)
//
// The dirty sets for the first seven live under Archive.mu with the aggregates
// they describe; the last two live under brainAgg.mu with theirs, because that
// aggregate deliberately has its own lock (brainhist.go).
type rollupDirty struct {
	species  map[string]bool
	genomes  map[string][]uint64
	lineages map[string]bool
	lanes    map[lanePair]bool
	index    map[int64]bool
	// gapsNew and gapsGone are the queue's two halves, and they CANCEL: a gap
	// that arrived and was served between two saves appears in both and is
	// written in neither, which is the ordinary case on a healthy map and is why
	// persisting the queue costs almost nothing there. Only a gap that is still
	// open at a save costs a line, and only a gap that was open at the previous
	// save costs a tombstone.
	gapsNew    map[string]bool
	gapsGone   map[string]bool
	ledger     bool
	counts     bool
	peers      bool
	everything bool
}

func newRollupDirty() rollupDirty {
	return rollupDirty{
		species:  map[string]bool{},
		genomes:  map[string][]uint64{},
		lineages: map[string]bool{},
		lanes:    map[lanePair]bool{},
		index:    map[int64]bool{},
		gapsNew:  map[string]bool{},
		gapsGone: map[string]bool{},
	}
}

func (d *rollupDirty) any() bool {
	return d.everything || d.ledger || d.counts || d.peers ||
		len(d.species) > 0 || len(d.genomes) > 0 || len(d.lineages) > 0 ||
		len(d.lanes) > 0 ||
		len(d.index) > 0 || len(d.gapsNew) > 0 || len(d.gapsGone) > 0
}

func (d *rollupDirty) clear() {
	d.species = map[string]bool{}
	d.genomes = map[string][]uint64{}
	d.lineages = map[string]bool{}
	d.lanes = map[lanePair]bool{}
	d.index = map[int64]bool{}
	d.gapsNew = map[string]bool{}
	d.gapsGone = map[string]bool{}
	d.ledger, d.counts, d.peers, d.everything = false, false, false, false
}

// notePositionLocked adopts the position of a record the ledger has just
// written, and returns the position of the byte JUST BEFORE it — which is what
// countRecordLocked wants for the hour index.
//
// rotated names the segment this append's rotation closed, "" when it did not
// rotate. WHEN IT DID, EVERY POSITION THIS FILE HOLDS THAT NAMES THE LIVE FILE
// IS NOW ABOUT THAT SEGMENT: the bytes moved there wholesale, at the same
// offsets, and a fresh live file starts at 0 with different records. Re-pointing
// them here is what keeps a saved cursor and a saved hour index usable across a
// midnight. The floor line's LiveFirstMs is the belt to this brace, for a crash
// between the rotation and the next save.
func (a *Archive) notePositionLocked(offset int64, rotated string) LedgerPos {
	a.noteRotationLocked(rotated)
	before := a.ledgerPos
	// The record's own position: the live file, up to and including it. The
	// record number is the one countRecordLocked is about to reach, under this
	// same lock, on the very next statement of both its callers — which is the
	// whole reason this function exists rather than two assignments at each site.
	a.ledgerPos = LedgerPos{Segment: "", Offset: offset, Record: int64(a.recordCount) + 1}
	return before
}

// noteRotationLocked re-points every position this file holds that names the LIVE
// FILE at the segment a rotation has just closed.
//
// It has TWO callers and the second one is easy to miss: the append path above,
// and the maintenance pass, which closes a stale live file on a quiet day
// boundary with no append involved at all (segments.go, rotateIfStale). A
// rotation nobody told the roll-up about leaves a cursor naming the live file
// with an offset into a file that no longer holds those bytes, and the next
// start would seek past the beginning of records it has never folded.
func (a *Archive) noteRotationLocked(rotated string) {
	if rotated == "" {
		return
	}
	if a.ledgerPos.Segment == "" {
		a.ledgerPos.Segment = rotated
	}
	for hour, pos := range a.rollupIndex {
		if pos.Segment == "" {
			pos.Segment = rotated
			a.rollupIndex[hour] = pos
			a.rollupDirty.index[hour] = true
		}
	}
	a.rollupDirty.counts = true
}

// markGapNewLocked and markGapGoneLocked are the queue's two dirty marks, and
// they are the ONLY way the "gq" lines are produced. They cancel each other so
// a gap that opened and closed between two saves never reaches the file.
func (a *Archive) markGapNewLocked(hash string) {
	delete(a.rollupDirty.gapsGone, hash)
	a.rollupDirty.gapsNew[hash] = true
}

func (a *Archive) markGapGoneLocked(hash string) {
	if a.rollupDirty.gapsNew[hash] {
		// Never written, so nothing to tombstone.
		delete(a.rollupDirty.gapsNew, hash)
		return
	}
	a.rollupDirty.gapsGone[hash] = true
}

// ------------------------------------------------------------- record counters

// recordTally is the per-record accounting the M5 evidence record needs and the
// fold did not keep: how many records the archive holds OF EACH TYPE, how many
// it holds FOR EACH PEER, and the first and last record times.
//
// The per-peer counter is new (archive_rollup_design.md, "Per-peer archive
// growth"), and it has to exist BEFORE the first raw segment leaves the host:
// once a segment is gone, an aggregate nobody was computing while it was there
// cannot be computed for the period it covers.
//
// firstMs is the ALL-TIME first record time, and it is the reason this struct
// carries a clock at all. The genealogy's floor caption and AncestrySinceMs are
// minima over the record; a roll-up that kept the aggregate and forgot when the
// record began would let a page claim the record starts where the WINDOW starts.
type recordTally struct {
	byType       map[string]int
	typeOverflow int
	byPeer       map[string]int
	peerOverflow int
	firstMs      int64
	lastMs       int64
}

func newRecordTally() recordTally {
	return recordTally{byType: map[string]int{}, byPeer: map[string]int{}}
}

// countRecordLocked folds ONE ledger line into every record counter, and it is
// the single writer of all of them: the replay in New and the four live append
// paths (onMigration, onAck, onNack, onGenomeResponse) call this and nothing
// else touches recordCount. One function, five call sites, no second view of the
// same fact — species.go rule 1 applied once more.
//
// at is the position of the byte JUST BEFORE this record — where a replay that
// wanted to see this record would have to start. The live path passes its own
// cursor, which is exactly that because the append that produced it has already
// happened (store.go, AppendAt); the replay passes the position it was handed
// for the previous record.
func (a *Archive) countRecordLocked(rec Record, at LedgerPos) {
	a.recordCount++
	a.rollupDirty.counts = true

	typ := rec.Type
	if typ == "" {
		typ = "UNKNOWN"
	}
	if _, ok := a.tally.byType[typ]; !ok && len(a.tally.byType) >= rollupTypeMax {
		a.tally.typeOverflow++
	} else {
		a.tally.byType[typ]++
	}

	// THE PEER A RECORD IS ATTRIBUTABLE TO. A migration, an ACK and a NACK all
	// name the world the organism left; a GENOME line names the peer that served
	// the blob. A record naming neither is counted in the total and against
	// nobody, which is the honest answer rather than an invented bucket.
	peer := rec.SourcePeer
	if peer == "" {
		peer = rec.ServedBy
	}
	if peer != "" {
		if _, ok := a.tally.byPeer[peer]; !ok && len(a.tally.byPeer) >= rollupPeerMax {
			a.tally.peerOverflow++
		} else {
			a.tally.byPeer[peer]++
		}
		a.rollupDirty.peers = true
	}

	if rec.RecordedAt > 0 {
		if a.tally.firstMs == 0 || rec.RecordedAt < a.tally.firstMs {
			a.tally.firstMs = rec.RecordedAt
		}
		if rec.RecordedAt > a.tally.lastMs {
			a.tally.lastMs = rec.RecordedAt
		}
		hour := rec.RecordedAt - rec.RecordedAt%rollupIndexStrideMs
		if _, ok := a.rollupIndex[hour]; !ok && len(a.rollupIndex) < rollupIndexMax {
			a.rollupIndex[hour] = at
			a.rollupDirty.index[hour] = true
		}
	}
}

// ------------------------------------------------------------- the sidecar

// rollupSidecar owns the file. Like the brain sidecar it has its own mutex and
// never writes to disk under a lock the migration path needs: a save copies what
// it needs out under the aggregates' locks and writes with those released.
type rollupSidecar struct {
	mu   sync.Mutex
	path string
	f    *os.File
	// bytes is the file's size as this process has written it and live is an
	// estimate of what a full rewrite would need. Both are maintained rather than
	// stat()ed, and the compaction test compares them.
	bytes int64
	live  int64
	// compactRatio and compactFloor are the constants, per instance, so a test
	// can force a compaction without writing a megabyte.
	compactRatio int64
	compactFloor int64
}

// openRollupSidecar opens or creates the file for appending. It does NOT read
// it: loadRollupState has already done that, before the tail replay, because the
// state has to be in the aggregates before a single record is folded on top.
func openRollupSidecar(dir string) (*rollupSidecar, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &rollupSidecar{
		path:         filepath.Join(dir, rollupSidecarName),
		compactRatio: rollupCompactRatio,
		compactFloor: rollupCompactFloor,
	}
	f, err := os.OpenFile(s.path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	s.f = f
	if info, err := f.Stat(); err == nil {
		s.bytes = info.Size()
	}
	if s.bytes == 0 {
		if err := s.writeHeaderLocked(); err != nil {
			f.Close()
			return nil, err
		}
	}
	return s, nil
}

type rollupNotOpenErr struct{}

func (rollupNotOpenErr) Error() string { return "archive: the roll-up sidecar is not open" }

var errRollupNotOpen = rollupNotOpenErr{}

// keepUnreadable renames a file this build cannot use out of the way, so the
// bytes survive for anybody who wants to look and the archive still records from
// now. It is never truncated and never deleted: a file this build cannot read is
// a file the next build may be able to.
func keepUnreadable(path string) error {
	err := os.Rename(path, path+".unreadable")
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *rollupSidecar) writeHeaderLocked() error {
	line, err := json.Marshal(rollupLine{R: "h", V: rollupVersion,
		BucketMs: BrainBucketMs, SavedAt: time.Now().UnixMilli()})
	if err != nil {
		return err
	}
	n, err := s.f.Write(append(line, '\n'))
	s.bytes += int64(n)
	return err
}

// Path is the sidecar file, for tests and operator tools.
func (s *rollupSidecar) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

// Save appends everything that changed since the last one and compacts when the
// file has grown past its worth. It is safe on a nil sidecar, because an archive
// whose data directory is not writable is still an archive.
func (s *rollupSidecar) Save(a *Archive) (err error) {
	if s == nil || a == nil {
		return nil
	}
	lines, live, compact := a.rollupLines()
	if lines == nil {
		return nil
	}
	// A SAVE THAT FAILS COSTS THE WHOLE DELTA, so the next one writes the whole
	// state. The dirty sets were cleared to build this batch; without this the
	// keys in it would never be written again, and the cursor of a later save
	// would claim coverage of records whose aggregates never reached the file.
	// Rewriting everything is the only repair that cannot be partial.
	defer func() {
		if err == nil {
			a.publishRollupCursor(lines)
			return
		}
		a.mu.Lock()
		a.rollupDirty.everything = true
		a.mu.Unlock()
	}()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.f == nil {
		// The file was open and is not any more, which on a live archive means a
		// compaction failed to reopen it. It is an ERROR rather than a quiet
		// nothing: the delta this call consumed has to go back on the dirty set,
		// and the honesty fields must not advance past a file nobody wrote.
		return errRollupNotOpen
	}
	s.live = live
	if compact {
		return s.compactLocked(lines)
	}
	w := bufio.NewWriter(s.f)
	for _, l := range lines {
		b, err := json.Marshal(l)
		if err != nil {
			continue
		}
		n, err := w.Write(append(b, '\n'))
		s.bytes += int64(n)
		if err != nil {
			return err
		}
	}
	if err := w.Flush(); err != nil {
		return err
	}
	if err := s.f.Sync(); err != nil {
		return err
	}
	if s.bytes > s.compactFloor && s.bytes > s.compactRatio*s.live {
		// The append is durable; the rewrite that follows is an optimisation and
		// its failure costs nothing but the space.
		full, live := a.rollupFullState()
		s.live = live
		return s.compactLocked(full)
	}
	return nil
}

// rollupLines is the delta: every dirty key's current value, plus the floor line
// that is written every time because the cursor moves every time. It returns nil
// when nothing has changed at all.
//
// The copy happens under the aggregates' locks and the write does not.
func (a *Archive) rollupLines() (lines []rollupLine, live int64, compact bool) {
	a.mu.Lock()
	whole := a.rollupDirty.everything
	dirty := a.rollupDirty.any()
	a.mu.Unlock()
	if whole {
		// THE FIRST SAVE AFTER A FULL REPLAY IS A COMPACTION, not an append: the
		// delta and the whole state are the same thing at that moment, and
		// writing it as a rewrite leaves the file at exactly its live size.
		l, live := a.rollupFullState()
		return l, live, true
	}

	g := a.brainAgg
	g.mu.Lock()
	brainDirty := len(g.dirtySeen) > 0 || len(g.newDedup) > 0
	g.mu.Unlock()
	if !dirty && !brainDirty {
		return nil, 0, false
	}

	a.mu.Lock()
	for key := range a.rollupDirty.species {
		if e := a.species.byKey[key]; e != nil {
			lines = append(lines, speciesRollupLine(key, e))
		}
	}
	for key, fps := range a.rollupDirty.genomes {
		if len(fps) > 0 {
			lines = append(lines, rollupLine{R: "sg", K: key, FP: dumpFingerprints(fps)})
		}
	}
	for id := range a.rollupDirty.lineages {
		if inst := a.species.lineage.byID[id]; inst != nil {
			lines = append(lines, lineageRollupLine(inst))
		}
	}
	if a.rollupDirty.ledger {
		lines = append(lines, rollupLine{R: "sl", Overflow: a.species.overflow,
			LineageOverflow: a.species.lineage.overflow,
			Edges:           a.species.edges, EdgeFirstMs: a.species.edgeFirstMs})
	}
	for key := range a.rollupDirty.lanes {
		if l := a.lanes[key]; l != nil {
			lines = append(lines, laneRollupLine(key, l))
		}
	}
	if a.rollupDirty.counts {
		lines = append(lines, a.countsRollupLineLocked())
	}
	if a.rollupDirty.peers {
		lines = append(lines, a.peersRollupLineLocked())
	}
	for hour := range a.rollupDirty.index {
		cur := a.rollupIndex[hour]
		lines = append(lines, rollupLine{R: "ix", Hour: hour,
			AtSeg: cur.Segment, AtOffset: cur.Offset, AtRecord: cur.Record})
	}
	for hash := range a.rollupDirty.gapsNew {
		if f := a.pending[hash]; f != nil {
			lines = append(lines, gapRollupLine(f))
		}
	}
	for hash := range a.rollupDirty.gapsGone {
		lines = append(lines, rollupLine{R: "gq", K: hash, Gone: true})
	}
	a.rollupDirty.clear()
	floor := a.floorRollupLineLocked()
	liveA := a.liveBytesLocked()
	a.mu.Unlock()

	// THE BRAIN HALF IS READ AFTER THE FLOOR, AND THE ORDER IS DELIBERATE. The two
	// aggregates have two locks and this file may not nest them (brainhist.go:
	// the fold sites take that lock briefly and never while holding Archive.mu),
	// so one of them is read a moment after the cursor is taken. Read SECOND, the
	// coverage counts can be a record AHEAD of the cursor, and the tail replay
	// then folds that record's crossing again — which the "bd" line below refuses,
	// because the fingerprint that suppressed it is saved in the same pass. Read
	// FIRST they would be a record BEHIND, and nothing would refuse the loss.
	// Conservative in the direction the dedup window can repair.
	g.mu.Lock()
	for key := range g.dirtySeen {
		if b := g.buckets[key]; b != nil {
			lines = append(lines, rollupLine{R: "bs", T: key, Seen: b.seen})
		}
	}
	for key, fps := range g.newDedup {
		if _, live := g.dedup[key]; live && len(fps) > 0 {
			lines = append(lines, rollupLine{R: "bd", T: key, FP: dumpFingerprints(fps)})
		}
	}
	g.dirtySeen = map[int64]bool{}
	g.newDedup = map[int64][]uint64{}
	floor.Frontier = g.frontier
	floor.BucketsDropped = g.bucketsDropped
	liveG := int64(len(g.buckets)) * rollupLiveBucketBytes
	for _, set := range g.dedup {
		liveG += int64(len(set)) * rollupLiveDedupBytes
	}
	g.mu.Unlock()

	lines = append(lines, floor)
	return lines, liveA + liveG + rollupLiveFixedBytes, false
}

// rollupFullState is the whole live state, for a compaction and for the first
// save after a full replay. It takes each aggregate's own lock in turn and
// releases it before taking the next; nothing here writes to disk.
func (a *Archive) rollupFullState() ([]rollupLine, int64) {
	var lines []rollupLine

	a.mu.Lock()
	for key, e := range a.species.byKey {
		lines = append(lines, speciesRollupLine(key, e))
		if len(e.genomes) > 0 {
			fps := make([]uint64, 0, len(e.genomes))
			for fp := range e.genomes {
				fps = append(fps, fp)
			}
			sort.Slice(fps, func(i, j int) bool { return fps[i] < fps[j] })
			lines = append(lines, rollupLine{R: "sg", K: key, FP: dumpFingerprints(fps)})
		}
	}
	for _, inst := range a.species.lineage.byID {
		lines = append(lines, lineageRollupLine(inst))
	}
	lines = append(lines, rollupLine{R: "sl", Overflow: a.species.overflow,
		LineageOverflow: a.species.lineage.overflow,
		Edges:           a.species.edges, EdgeFirstMs: a.species.edgeFirstMs})
	for key, l := range a.lanes {
		lines = append(lines, laneRollupLine(key, l))
	}
	lines = append(lines, a.countsRollupLineLocked(), a.peersRollupLineLocked())
	for hour, cur := range a.rollupIndex {
		lines = append(lines, rollupLine{R: "ix", Hour: hour,
			AtSeg: cur.Segment, AtOffset: cur.Offset, AtRecord: cur.Record})
	}
	// THE WHOLE QUEUE AND NO TOMBSTONES. A compaction is a rewrite, so what it
	// writes IS the live set and every tombstone the old file carried has done
	// its work already.
	for _, f := range a.pending {
		lines = append(lines, gapRollupLine(f))
	}
	a.rollupDirty.clear()
	floor := a.floorRollupLineLocked()
	live := a.liveBytesLocked()
	a.mu.Unlock()

	g := a.brainAgg
	g.mu.Lock()
	for key, b := range g.buckets {
		if b.seen > 0 {
			lines = append(lines, rollupLine{R: "bs", T: key, Seen: b.seen})
		}
	}
	for key, set := range g.dedup {
		fps := make([]uint64, 0, len(set))
		for fp, bits := range set {
			if bits&1 != 0 {
				fps = append(fps, fp)
			}
		}
		if len(fps) == 0 {
			continue
		}
		sort.Slice(fps, func(i, j int) bool { return fps[i] < fps[j] })
		lines = append(lines, rollupLine{R: "bd", T: key, FP: dumpFingerprints(fps)})
		live += int64(len(fps)) * rollupLiveDedupBytes
	}
	g.dirtySeen = map[int64]bool{}
	g.newDedup = map[int64][]uint64{}
	floor.Frontier = g.frontier
	floor.BucketsDropped = g.bucketsDropped
	live += int64(len(g.buckets)) * rollupLiveBucketBytes
	g.mu.Unlock()

	lines = append(lines, floor)
	return lines, live + rollupLiveFixedBytes
}

// publishRollupCursor moves the honesty fields to what is now ON DISK. It runs
// only after a successful write, because a field that says how much of the
// record the sidecar covers must never be ahead of the file.
func (a *Archive) publishRollupCursor(lines []rollupLine) {
	for i := len(lines) - 1; i >= 0; i-- {
		if lines[i].R != "f" {
			continue
		}
		a.mu.Lock()
		a.rollupCovered = lines[i].pos()
		a.rollupSavedAtMs = lines[i].SavedAt
		a.mu.Unlock()
		return
	}
}

func (a *Archive) floorRollupLineLocked() rollupLine {
	line := rollupLine{R: "f",
		SavedAt:        time.Now().UnixMilli(),
		CoveredRecords: a.recordCount,
		Segment:        a.ledgerPos.Segment,
		Offset:         a.ledgerPos.Offset,
		SkippedLines:   a.ledgerSkipped,
		FirstMs:        a.tally.firstMs,
		LastMs:         a.tally.lastMs,
		RawFromMs:      a.rawFromMs(),
		GapsExpired:    a.evict.gapsExpired,
		DupRefused:     a.duplicatesRefused,
	}
	if a.ledger != nil {
		line.LiveFirstMs = a.ledger.FirstRecordedAtMs()
	}
	return line
}

// rawFromMs is where the RAW record on this host begins, for the sidecar's own
// floor line. It is DURABLE STATE rather than the published view: the published
// one is ledgerRawWindowFromMs (status.go), which the segment maintenance pass
// refreshes from the directory. This one is what the file says the state was
// written against, and a load carries it forward so an archive whose segments
// have all been retired can still say where its raw stream began.
func (a *Archive) rawFromMs() int64 {
	if a.seg.windowFromMs > 0 {
		return a.seg.windowFromMs
	}
	return a.tally.firstMs
}

func (a *Archive) countsRollupLineLocked() rollupLine {
	byType := make(map[string]int, len(a.tally.byType))
	for k, v := range a.tally.byType {
		byType[k] = v
	}
	return rollupLine{R: "rc", Records: a.recordCount, ByType: byType,
		TypeOverflow: a.tally.typeOverflow}
}

func (a *Archive) peersRollupLineLocked() rollupLine {
	byPeer := make(map[string]int, len(a.tally.byPeer))
	for k, v := range a.tally.byPeer {
		byPeer[k] = v
	}
	return rollupLine{R: "rp", ByPeer: byPeer, PeerOverflow: a.tally.peerOverflow}
}

func (a *Archive) liveBytesLocked() int64 {
	live := int64(len(a.species.byKey)) * rollupLiveSpeciesBytes
	live += int64(len(a.species.lineage.byID)) * rollupLiveLineageBytes
	for _, e := range a.species.byKey {
		live += int64(len(e.genomes)) * rollupLiveGenomeBytes
	}
	for _, l := range a.lanes {
		live += rollupLiveLaneBytes + int64(len(l.recent))*rollupLiveHopBytes
	}
	live += int64(len(a.rollupIndex)) * rollupLiveIndexBytes
	live += int64(len(a.pending)) * rollupLiveGapBytes
	live += int64(len(a.tally.byPeer)) * 64
	return live
}

func gapRollupLine(f *fetch) rollupLine {
	line := rollupLine{R: "gq", K: f.hash,
		GapPeer: f.sourcePeer, GapMigID: f.migrationID, GapEntityID: f.entityID,
		GapMigrant: f.migrant, GapSpecies: f.speciesKey,
	}
	if !f.crossedAt.IsZero() {
		line.CrossedAt = f.crossedAt.UnixMilli()
	}
	return line
}

func speciesRollupLine(key string, e *speciesAgg) rollupLine {
	return rollupLine{R: "sp", K: key,
		Crossings: e.crossings, SpFirstMs: e.firstMs, SpLastMs: e.lastMs,
		GenomesTrunc: e.genomesTruncated,
		Recent:       append([]SpeciesCrossing(nil), e.recent...),
		Parent:       e.parent, ParentKey: e.parentKey, ParentAtMs: e.parentAtMs,
		GenomeHash: e.genomeHash, GenomeAtMs: e.genomeAtMs,
	}
}

func lineageRollupLine(inst *speciesLineageInstance) rollupLine {
	seen := make(map[int]int64, len(inst.seenAt))
	for slot, at := range inst.seenAt {
		seen[slot] = at
	}
	return rollupLine{R: "si", K: inst.id,
		NameKey: inst.nameKey, Name: inst.name,
		ParentKnown: inst.parentKnown, ParentID: inst.parentID,
		ParentKey: inst.parentKey, Parent: inst.parent,
		Placeholder: inst.placeholder, LineageConflict: inst.conflict,
		Crossings: inst.crossings, SpFirstMs: inst.firstMs, SpLastMs: inst.lastMs,
		Recent:     append([]SpeciesCrossing(nil), inst.recent...),
		GenomeHash: inst.genomeHash, GenomeAtMs: inst.genomeAtMs,
		SeenAt: seen,
	}
}

func laneRollupLine(key lanePair, l *lane) rollupLine {
	return rollupLine{R: "ln", From: key.from, To: key.to, Edge: key.edge,
		Total: l.total, LaneFirstMs: l.firstAt, LaneLastMs: l.lastAt,
		RecentMs: append([]int64(nil), l.recent...)}
}

// compactLocked rewrites the whole file into place: a temporary file, an fsync,
// a rename, and an fsync of the directory entry. The old file is whole until the
// instant the new one is, so a kill during a compaction loses the appends since
// the last save and never the state.
func (s *rollupSidecar) compactLocked(lines []rollupLine) error {
	tmp := s.path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	fail := func(err error) error {
		f.Close()
		os.Remove(tmp)
		return err
	}
	w := bufio.NewWriter(f)
	total := int64(0)
	head, err := json.Marshal(rollupLine{R: "h", V: rollupVersion,
		BucketMs: BrainBucketMs, SavedAt: time.Now().UnixMilli()})
	if err != nil {
		return fail(err)
	}
	n, err := w.Write(append(head, '\n'))
	total += int64(n)
	if err != nil {
		return fail(err)
	}
	for _, l := range lines {
		b, err := json.Marshal(l)
		if err != nil {
			continue
		}
		n, err := w.Write(append(b, '\n'))
		total += int64(n)
		if err != nil {
			return fail(err)
		}
	}
	if err := w.Flush(); err != nil {
		return fail(err)
	}
	if err := f.Sync(); err != nil {
		return fail(err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := fsutil.SyncDir(filepath.Dir(s.path)); err != nil {
		return err
	}
	if s.f != nil {
		_ = s.f.Close()
	}
	nf, err := os.OpenFile(s.path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		s.f = nil
		return err
	}
	s.f, s.bytes = nf, total
	return nil
}

// Close flushes what is pending and closes the file, so an orderly shutdown
// leaves a sidecar that covers every record the ledger holds.
func (s *rollupSidecar) Close(a *Archive) error {
	if s == nil {
		return nil
	}
	if err := s.Save(a); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.f == nil {
		return nil
	}
	err := s.f.Close()
	s.f = nil
	return err
}

// SaveRollup flushes the roll-up state now. It exists for tests and for the
// tick loop; nothing else should need it.
func (a *Archive) SaveRollup() error { return a.rollup.Save(a) }

// RollupPath is the sidecar file, for tests and operator tools.
func (a *Archive) RollupPath() string { return a.rollup.Path() }
