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
//	by replaying the raw records inside cfg.DedupWindow, exactly as it is today.
//
//	THE GENOME-GAP FETCH QUEUE is rebuilt from the raw records inside the
//	retention horizon (§23, B34; eviction.go). The design leaves persisting it
//	to phase 3 and says the raw window rebuilds it; phase 1 keeps that rebuild
//	working unchanged.
//
// So the restart's floor is NOT the sidecar: it is
// max(cfg.DedupWindow, cfg.GenomeHorizon) of RAW records, which is what
// replayPlan below computes. On an archive with no horizon — the contract
// default — that is the whole file and the saving is the fold alone. The moment
// phase 2 gives the raw stream a window, the same plan reads the window and the
// restart becomes flat for the life of the deployment. That interaction is
// stated here rather than discovered later, because the design's "a few
// seconds, flat" and its "genomeGaps: replay of the raw window" cannot both be
// true unless the window is short.
//
// THE CURSOR IS A LINE COUNT, NOT A BYTE OFFSET. A byte offset would have to be
// maintained by the live append path to stay exact, and the append happens
// outside the lock that folds the record — so a save landing between the two
// would write an offset that covers a record the aggregate has not folded, and
// the tail replay would skip it forever. A line count is derived from the fold
// itself and cannot get ahead of it. recordSource keeps an Offset field for
// phase 2, whose segment boundaries ARE record boundaries and can carry one.

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"

	"multiverse/internal/fsutil"
)

const (
	rollupSidecarName = "rollup.jsonl"
	// rollupVersion is the format's own version. A file whose header names a
	// version this build does not know is NOT half-read: it is refused whole and
	// the state is rebuilt by a full replay.
	rollupVersion = 1
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
	rollupLiveGenomeBytes  int64 = 24
	rollupLiveLaneBytes    int64 = 128
	rollupLiveHopBytes     int64 = 15
	rollupLiveBucketBytes  int64 = 40
	rollupLiveDedupBytes   int64 = 24
	rollupLiveIndexBytes   int64 = 56
	rollupLiveFixedBytes   int64 = 2048
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
//	"sl" the species ledger's own globals, last-writer-wins
//	"ln" one directed lane's counters, last-writer-wins on (from, to, edge)
//	"rc" the record counters: total and per type, last-writer-wins
//	"rp" the per-peer record counters, last-writer-wins
//	"bs" one brain bucket's COVERAGE DENOMINATOR, last-writer-wins
//	"bd" NEW entries of the brain seen-dedup window for one bucket, ADDITIVE
//	"ix" one entry of the time->cursor index, last-writer-wins on the hour
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
	CoveredLines   int   `json:"covLines,omitempty"`
	CoveredRecords int   `json:"covRecords,omitempty"`
	SkippedLines   int   `json:"skippedLines,omitempty"`
	FirstMs        int64 `json:"firstMs,omitempty"`
	LastMs         int64 `json:"lastMs,omitempty"`
	RawFromMs      int64 `json:"rawFromMs,omitempty"`
	Frontier       int64 `json:"frontierMs,omitempty"`
	BucketsDropped int   `json:"bucketsDropped,omitempty"`

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

	// Genome fingerprints ("sg") and brain dedup fingerprints ("bd"). BOTH ARE
	// DECIMAL STRINGS, not JSON numbers: a 64-bit fingerprint above 2^53 does not
	// survive a reader that parses numbers as doubles, and this file is meant to
	// be read with `jq` (store.go's store-choice rule 1).
	FP []string `json:"fp,omitempty"`

	// Species ledger globals ("sl").
	Overflow    int   `json:"ov,omitempty"`
	Edges       int   `json:"ed,omitempty"`
	EdgeFirstMs int64 `json:"ef,omitempty"`

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

	// Index ("ix").
	Hour     int64 `json:"hr,omitempty"`
	AtLine   int   `json:"ln,omitempty"`
	AtRecord int   `json:"rn,omitempty"`
}

// ------------------------------------------------------------- the cursor

// rollupCursor is HOW MUCH OF THE RAW STREAM THE AGGREGATE HAS EATEN.
//
// Lines counts COMPLETE LINES, Records counts the records those lines parsed
// into, and the two differ by exactly the damaged lines a replay read past
// (store.go, ScanLedger). Lines is the one the tail replay skips on, because a
// skip that does not parse cannot tell a damaged line from a good one — which is
// also why SkippedLines is persisted beside it: the ledger is append-only, so
// its damage is permanent, and an archive that stopped counting it because it
// stopped re-reading it would quietly lose the number that explains why
// `ledgerRecords` and `wc -l` disagree.
//
// Offset is for phase 2. It is 0 here and deliberately unused: see the file
// header for why the live append path cannot maintain one exactly.
type rollupCursor struct {
	Lines   int
	Records int
	Offset  int64
}

// recordSource is where a replay reads raw records from. Phase 1 implements it
// against the single migrations.jsonl; phase 2 replaces it with the ordered set
// of segments plus the live file and changes nothing above this line.
type recordSource interface {
	// ScanFrom hands fn every record from line index fromLine onward, in write
	// order, with the line's own index. Lines before fromLine are counted and
	// NOT parsed, which is the whole point: parsing is the cost of a replay.
	//
	// damageFrom is where the DAMAGE COUNT starts, and it is a second index
	// because the skipped-line count is a persisted aggregate like every other
	// counter here: a line the sidecar has already accounted for must not be
	// counted a second time when the raw scan reaches further back than the
	// aggregate cursor does. It is always at or after fromLine.
	//
	// It returns how many complete lines it read in total — including the ones it
	// skipped — and what it could not read at or after damageFrom.
	ScanFrom(fromLine, damageFrom int, fn func(rec Record, line int)) (lines int,
		dmg LedgerDamage, err error)
}

// fileRecordSource is phase 1's source: the one append-only ledger file.
type fileRecordSource struct{ dir string }

func (s fileRecordSource) ScanFrom(fromLine, damageFrom int,
	fn func(Record, int)) (int, LedgerDamage, error) {

	var dmg LedgerDamage
	line := 0
	f, err := os.Open(filepath.Join(s.dir, ledgerName))
	if errors.Is(err, os.ErrNotExist) {
		return 0, dmg, nil
	}
	if err != nil {
		return 0, dmg, err
	}
	defer f.Close()

	r := bufio.NewReaderSize(f, 1<<20)
	for {
		raw, err := readLine(r)
		if len(raw) > 0 {
			switch {
			case errors.Is(err, io.EOF):
				// No newline on the last line: the write that put it there landed
				// short and the record was never durable. Same rule as ScanLedger.
				dmg.TornTail = int64(len(raw))
			case line < fromLine:
				// COVERED BY THE SIDECAR. Not parsed, not folded, not counted as
				// damage — a skip that does not parse cannot judge a line, and the
				// sidecar carries the damage count for the region it covers.
				line++
			default:
				var rec Record
				if json.Unmarshal(raw, &rec) != nil {
					if line >= damageFrom {
						dmg.Lines++
						dmg.Bytes += int64(len(raw))
					}
					line++
					break
				}
				fn(rec, line)
				line++
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return line, dmg, err
		}
	}
	return line, dmg, nil
}

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

type rollupLane struct {
	total   int
	firstAt int64
	lastAt  int64
	recent  []int64
}

// rollupState is a whole sidecar, parsed. Parsing and APPLYING are separate on
// purpose: last-writer-wins and set-union are decided here, over the whole file,
// and only then is the result handed to the aggregates in an order they can
// accept (oldest bucket first, so a full brain map evicts the way it would have).
type rollupState struct {
	savedAtMs      int64
	cursor         rollupCursor
	skippedLines   int
	firstMs        int64
	lastMs         int64
	rawFromMs      int64
	frontier       int64
	bucketsDropped int

	species     map[string]*rollupSpecies
	genomes     map[string][]uint64
	overflow    int
	edges       int
	edgeFirstMs int64

	lanes map[lanePair]*rollupLane

	records      int
	byType       map[string]int
	typeOverflow int
	byPeer       map[string]int
	peerOverflow int

	seen  map[int64]int
	dedup map[int64][]uint64

	index map[int64]rollupCursor
}

func newRollupState() *rollupState {
	return &rollupState{
		species: map[string]*rollupSpecies{},
		genomes: map[string][]uint64{},
		lanes:   map[lanePair]*rollupLane{},
		byType:  map[string]int{},
		byPeer:  map[string]int{},
		seen:    map[int64]int{},
		dedup:   map[int64][]uint64{},
		index:   map[int64]rollupCursor{},
	}
}

// lineAtOrBefore is the cursor of the newest index entry at or before atMs, and
// false when the index cannot answer — which is every archive whose sidecar
// predates the requested time. A caller that gets false MUST scan from 0: an
// index that cannot say where a window starts is not a licence to guess.
func (st *rollupState) lineAtOrBefore(atMs int64) (rollupCursor, bool) {
	if st == nil || len(st.index) == 0 || atMs <= 0 {
		return rollupCursor{}, false
	}
	best, found := rollupCursor{}, false
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
		return rollupCursor{}, false
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
		return rollupCursor{}, false
	}
	return best, true
}

// ------------------------------------------------------------- loading

// loadRollupState reads the sidecar. It returns (nil, true) when there is no
// file at all — a new archive, or the first run after this feature existed —
// and (nil, false) when a file exists and cannot be used, which is a LOSS: the
// caller keeps the bytes beside a fresh file and rebuilds by a full replay.
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
		st.cursor = rollupCursor{Lines: rec.CoveredLines, Records: rec.CoveredRecords}
		st.skippedLines = rec.SkippedLines
		st.firstMs, st.lastMs = rec.FirstMs, rec.LastMs
		st.rawFromMs = rec.RawFromMs
		st.frontier = rec.Frontier
		st.bucketsDropped = rec.BucketsDropped
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
	case "sl":
		st.overflow, st.edges, st.edgeFirstMs = rec.Overflow, rec.Edges, rec.EdgeFirstMs
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
	case "ix":
		if rec.Hour > 0 && len(st.index) < rollupIndexMax {
			st.index[rec.Hour] = rollupCursor{Lines: rec.AtLine, Records: rec.AtRecord}
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
	a.species.overflow = st.overflow
	a.species.edges = st.edges
	a.species.edgeFirstMs = st.edgeFirstMs
	for key, l := range st.lanes {
		a.lanes[key] = &lane{total: l.total, firstAt: l.firstAt, lastAt: l.lastAt,
			recent: append([]int64(nil), l.recent...)}
	}
	a.recordCount = st.records
	a.ledgerLines = st.cursor.Lines
	a.ledgerSkipped = st.skippedLines
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
	a.rollupIndex = map[int64]rollupCursor{}
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
// TWO CUT POINTS, ONE PASS. AggFrom is where the AGGREGATE fold resumes — the
// sidecar's cursor, because everything before it is already in the sidecar.
// RawFrom is where the RAW-DERIVED state must be rebuilt from, because it is not
// in the sidecar and cannot be (the duplicate guard's per-process seeds) or is
// not yet (the genome-gap queue, phase 3). ScanFrom is the earlier of the two.
type replayPlan struct {
	ScanFrom int
	AggFrom  int
	// RawSpan is the span of raw records the plan needs rebuilt, 0 for "all of
	// them". Published in the log so an operator can see what a restart cost and
	// why.
	RawSpan time.Duration
	// FromSidecar says the aggregates came from a sidecar rather than a replay.
	FromSidecar bool
}

// planReplay decides both cut points.
func (a *Archive) planReplay(st *rollupState, now time.Time) replayPlan {
	if st == nil || st.cursor.Lines <= 0 {
		return replayPlan{}
	}
	plan := replayPlan{AggFrom: st.cursor.Lines, FromSidecar: true}
	// The raw span. A horizon of 0 is an archive that retires no gap ever
	// (eviction.go, and it is the contract default), so the gap queue's rebuild
	// needs every record there has ever been and there is no span at all.
	if a.cfg.GenomeHorizon <= 0 {
		return plan
	}
	span := a.cfg.GenomeHorizon
	if a.cfg.DedupWindow > span {
		span = a.cfg.DedupWindow
	}
	plan.RawSpan = span
	// One stride of margin, because the index answers on an hour boundary and a
	// window that started inside an hour must not begin after its own start.
	from := now.Add(-span).UnixMilli() - rollupIndexStrideMs
	cur, ok := st.lineAtOrBefore(from)
	if !ok {
		return plan
	}
	plan.ScanFrom = cur.Lines
	if plan.ScanFrom > plan.AggFrom {
		plan.ScanFrom = plan.AggFrom
	}
	return plan
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
	species    map[string]bool
	genomes    map[string][]uint64
	lanes      map[lanePair]bool
	index      map[int64]bool
	ledger     bool
	counts     bool
	peers      bool
	everything bool
}

func newRollupDirty() rollupDirty {
	return rollupDirty{
		species: map[string]bool{},
		genomes: map[string][]uint64{},
		lanes:   map[lanePair]bool{},
		index:   map[int64]bool{},
	}
}

func (d *rollupDirty) any() bool {
	return d.everything || d.ledger || d.counts || d.peers ||
		len(d.species) > 0 || len(d.genomes) > 0 || len(d.lanes) > 0 || len(d.index) > 0
}

func (d *rollupDirty) clear() {
	d.species = map[string]bool{}
	d.genomes = map[string][]uint64{}
	d.lanes = map[lanePair]bool{}
	d.index = map[int64]bool{}
	d.ledger, d.counts, d.peers, d.everything = false, false, false, false
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
// line is the line's own index in the raw stream. The live path passes the
// archive's own count, which is exact because a live append is always one
// complete line.
func (a *Archive) countRecordLocked(rec Record, line int) {
	a.recordCount++
	a.ledgerLines = line + 1
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
			a.rollupIndex[hour] = rollupCursor{Lines: line, Records: a.recordCount - 1}
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
	if a.rollupDirty.ledger {
		lines = append(lines, rollupLine{R: "sl", Overflow: a.species.overflow,
			Edges: a.species.edges, EdgeFirstMs: a.species.edgeFirstMs})
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
			AtLine: cur.Lines, AtRecord: cur.Records})
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
	lines = append(lines, rollupLine{R: "sl", Overflow: a.species.overflow,
		Edges: a.species.edges, EdgeFirstMs: a.species.edgeFirstMs})
	for key, l := range a.lanes {
		lines = append(lines, laneRollupLine(key, l))
	}
	lines = append(lines, a.countsRollupLineLocked(), a.peersRollupLineLocked())
	for hour, cur := range a.rollupIndex {
		lines = append(lines, rollupLine{R: "ix", Hour: hour,
			AtLine: cur.Lines, AtRecord: cur.Records})
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
		a.rollupCovered = rollupCursor{Lines: lines[i].CoveredLines,
			Records: lines[i].CoveredRecords}
		a.rollupSavedAtMs = lines[i].SavedAt
		a.mu.Unlock()
		return
	}
}

func (a *Archive) floorRollupLineLocked() rollupLine {
	// rawFromMs is the EARLIEST RAW RECORD TIME STILL ON THE HOST. Under phase 1
	// the whole record is still on the host, so it is the first record ever
	// folded; phase 2 replaces it with the oldest retained segment's first record
	// and nothing else here changes.
	return rollupLine{R: "f",
		SavedAt:        time.Now().UnixMilli(),
		CoveredLines:   a.ledgerLines,
		CoveredRecords: a.recordCount,
		SkippedLines:   a.ledgerSkipped,
		FirstMs:        a.tally.firstMs,
		LastMs:         a.tally.lastMs,
		RawFromMs:      a.rawFromMs(),
	}
}

// rawFromMs is the honesty field of the same name: where the RAW record on this
// host begins. Phase 1 keeps every raw line, so it is the first record time.
func (a *Archive) rawFromMs() int64 { return a.tally.firstMs }

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
	for _, e := range a.species.byKey {
		live += int64(len(e.genomes)) * rollupLiveGenomeBytes
	}
	for _, l := range a.lanes {
		live += rollupLiveLaneBytes + int64(len(l.recent))*rollupLiveHopBytes
	}
	live += int64(len(a.rollupIndex)) * rollupLiveIndexBytes
	live += int64(len(a.tally.byPeer)) * 64
	return live
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
