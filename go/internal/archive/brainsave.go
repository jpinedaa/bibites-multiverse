package archive

// THE BRAIN SIDECAR: <data-dir>/brains.jsonl, the durable half of brainhist.go.
//
// WHY IT EXISTS AT ALL. The aggregate is folded at genome arrival, which is the
// only place the measurement is free (brainhist.go rule 1) — and an arrival
// happens once. Nothing replays it: the ledger records that a genome was stored,
// not what was inside it, and re-deriving the series at startup would mean
// reading and parsing every blob in the store. Measured, that is about eleven
// minutes added to a 28-second replay of 10.62 M records, and it grows with the
// store forever. So the fold's own output is written down and REPLAYED, and the
// replay is a small file rather than a large one.
//
// THE LOSS RULE, and it is the whole reason this file is careful. A missing,
// unreadable or truncated sidecar means THIS HISTORY STARTS NOW. It never means
// a run of zeroes: a five minutes this archive holds no measurement for is a
// bucket with no reading, drawn as a gap, and the view publishes HaveFromMs so
// the page can caption where its knowledge begins — the same shape the genealogy
// uses for its ancestry floor. A zero would say the creatures on this map had no
// brains, which is a claim about the world made out of a failure of the record.
//
// THE SHAPE IS APPEND-AND-COMPACT, not rewrite-in-full, and that is a bound
// rather than an optimisation. A full rewrite on a timer costs the SIZE OF THE
// WHOLE HISTORY every time it runs — 1.1 MB today, ~54 MB after a year — which
// is precisely the shape species.go rule 1 refuses for the ledger: the cost of
// keeping the record must not grow with the age of the record. A save appends
// only what moved since the last one (the frontier bucket, whatever late
// arrivals landed, and the species records that changed) and the file is
// rewritten in full only when it has grown past brainCompactRatio times its live
// content. The replay is last-writer-wins per key, so a rewritten record simply
// supersedes its older lines.

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"multiverse/internal/fsutil"
)

const (
	brainSidecarName = "brains.jsonl"
	// brainSidecarVersion is the format's own version. A file whose header names
	// a version this build does not know is NOT half-read: it is refused whole,
	// and the history starts now.
	brainSidecarVersion = 1
	// brainSaveInterval is how often the aggregate is flushed. It bounds what a
	// hard kill loses to half a minute of arrivals — about seventy genomes at the
	// measured rate — and the append is a few hundred bytes.
	brainSaveInterval = 30 * time.Second
	// brainCompactRatio is when a rewrite is worth its cost: the file holds this
	// many times the bytes its live content needs. Three is the same order the
	// sidecar journal uses, for the same reason.
	brainCompactRatio = 3
	// brainCompactFloor is the size below which growth is not worth a rewrite at
	// all. Without it a brand-new archive compacts on almost every save.
	brainCompactFloor int64 = 256 << 10
)

// brainLine is one record of the sidecar. The two kinds share a line shape and
// are told apart by R, so the file is one stream that `jq` can read and a torn
// tail can be dropped from — exactly as the ledger and the metrics file are.
type brainLine struct {
	R string `json:"r"`
	// The header ("h").
	V        int   `json:"v,omitempty"`
	BucketMs int64 `json:"bucketMs,omitempty"`
	SavedAt  int64 `json:"savedAtMs,omitempty"`
	// A bucket ("b"). THERE IS DELIBERATELY NO `seen` HERE: the coverage
	// denominator is derived from the ledger, which is replayed at every start
	// anyway, so persisting it would put one fact in two places and let a restart
	// disagree with the record about how many genomes crossed. What cannot be
	// re-derived — what was INSIDE each genome — is what this file holds.
	T    int64          `json:"t,omitempty"`
	Held int            `json:"n,omitempty"`
	Neu  map[string]int `json:"neu,omitempty"`
	Syn  map[string]int `json:"syn,omitempty"`
	Bin  bool           `json:"bin,omitempty"`
	// A species record ("s").
	K        string `json:"k,omitempty"`
	Neurons  int    `json:"nu,omitempty"`
	Synapses int    `json:"sy,omitempty"`
	AtMs     int64  `json:"at,omitempty"`
	Hash     string `json:"h,omitempty"`
}

// brainSidecar owns the file. It has its own mutex and never takes the
// aggregate's while holding it for a write: a save copies what it needs out
// under the aggregate's lock and then writes with that lock released, so no disk
// write is ever performed under a lock the fold path needs.
type brainSidecar struct {
	mu   sync.Mutex
	path string
	f    *os.File
	// bytes is the file's size as this process has written it, and live an
	// estimate of the bytes a full rewrite would need. The two are what the
	// compaction test compares, and both are maintained rather than stat()ed.
	bytes int64
	live  int64
}

// openBrainSidecar opens or creates the file for appending and replays what is
// in it into g. A file it cannot use is REPORTED AND LEFT ALONE — never
// truncated, never deleted — because a file this build cannot read is a file the
// next build may be able to, and the archive can run perfectly well without it.
func openBrainSidecar(dir string, g *brainAgg) (*brainSidecar, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &brainSidecar{path: filepath.Join(dir, brainSidecarName)}
	loaded, usable, err := s.replay(g)
	if err != nil {
		return nil, err
	}
	g.mu.Lock()
	g.loaded = usable && loaded > 0
	g.lost = !usable
	g.mu.Unlock()
	if !usable {
		// The file exists and cannot be used. Start a NEW one beside it under a
		// name that says what happened, so the unreadable bytes survive for
		// anybody who wants to look and the archive still records from now.
		if err := os.Rename(s.path, s.path+".unreadable"); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
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

// replay reads the whole file into g. It returns how many records it applied and
// whether the file was usable at all.
//
// A TORN FINAL LINE IS DROPPED, which is the same rule the ledger and the metrics
// file apply: a process killed mid-write leaves a partial record and everything
// before it is good. A line that does not parse ANYWHERE ELSE in the file is a
// different matter — the file is written by one writer in one shape, so a broken
// line in the middle means the file is not what this reader thinks it is, and the
// whole of it is refused.
func (s *brainSidecar) replay(g *brainAgg) (applied int, usable bool, err error) {
	b, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		// NO FILE IS NOT A LOSS. It is a new archive, or the first run after this
		// feature existed, and the history starts now by design.
		return 0, true, nil
	}
	if err != nil {
		return 0, false, err
	}
	if len(b) == 0 {
		return 0, true, nil
	}
	lines := splitLines(b)
	if len(lines) == 0 {
		return 0, false, nil
	}
	var head brainLine
	if json.Unmarshal(lines[0], &head) != nil || head.R != "h" {
		return 0, false, nil
	}
	if head.V != brainSidecarVersion || head.BucketMs != BrainBucketMs {
		// A DIFFERENT RESOLUTION IS A DIFFERENT FILE. Buckets written at another
		// width cannot be merged with these without inventing a within-bucket
		// distribution, so the file is refused rather than reinterpreted.
		return 0, false, nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	for i := 1; i < len(lines); i++ {
		var rec brainLine
		if json.Unmarshal(lines[i], &rec) != nil {
			if i == len(lines)-1 {
				// The torn tail of an interrupted write.
				break
			}
			return 0, false, nil
		}
		switch rec.R {
		case "b":
			if rec.T <= 0 {
				continue
			}
			bk := g.bucketFor(rec.T)
			if bk == nil {
				continue
			}
			// LAST WRITER WINS PER BUCKET: an appended line is the whole of that
			// bucket's MEASUREMENTS as they stood when it was written, so it
			// REPLACES rather than adds. Adding would double every bucket that was
			// ever saved twice. `seen` is untouched — the ledger replay owns it,
			// and it has already run when this does.
			bk.held, bk.binned = rec.Held, rec.Bin
			bk.neurons, bk.synapses = brainHist{}, brainHist{}
			loadHist(bk.neurons, rec.Neu)
			loadHist(bk.synapses, rec.Syn)
			if rec.T > g.frontier {
				g.frontier = rec.T
			}
			applied++
		case "s":
			if rec.K == "" || rec.Hash == "" || rec.AtMs <= 0 {
				continue
			}
			if cur, ok := g.species[rec.K]; ok && cur.AtMs >= rec.AtMs {
				continue
			}
			g.species[rec.K] = brainRecord{Neurons: rec.Neurons, Synapses: rec.Synapses,
				AtMs: rec.AtMs, Hash: rec.Hash}
			applied++
		}
	}
	return applied, true, nil
}

func splitLines(b []byte) [][]byte {
	var out [][]byte
	start := 0
	for i := 0; i < len(b); i++ {
		if b[i] != '\n' {
			continue
		}
		if i > start {
			out = append(out, b[start:i])
		}
		start = i + 1
	}
	if start < len(b) {
		// No trailing newline: the last write did not finish. It is kept here and
		// dropped by the parse above, which is where "torn" is decided.
		out = append(out, b[start:])
	}
	return out
}

func loadHist(h brainHist, m map[string]int) {
	for k, c := range m {
		v, err := parseUint16(k)
		if err != nil || c <= 0 {
			continue
		}
		h[v] += uint32(c)
	}
}

func parseUint16(s string) (uint16, error) {
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, errBadKey
		}
		n = n*10 + int(s[i]-'0')
		if n > brainValueMax {
			return 0, errBadKey
		}
	}
	if len(s) == 0 {
		return 0, errBadKey
	}
	return uint16(n), nil
}

type brainKeyErr struct{}

func (brainKeyErr) Error() string { return "brains: bad histogram key" }

var errBadKey = brainKeyErr{}

func (s *brainSidecar) writeHeaderLocked() error {
	line, err := json.Marshal(brainLine{R: "h", V: brainSidecarVersion,
		BucketMs: BrainBucketMs, SavedAt: time.Now().UnixMilli()})
	if err != nil {
		return err
	}
	n, err := s.f.Write(append(line, '\n'))
	s.bytes += int64(n)
	return err
}

// Save appends everything that changed since the last one, and compacts when the
// file has grown past its worth. It is safe to call on a closed sidecar and on a
// nil one, because an archive with no data directory writable is still an
// archive.
func (s *brainSidecar) Save(g *brainAgg) error {
	if s == nil {
		return nil
	}
	// The copy happens under the AGGREGATE's lock and the write does not: no disk
	// write is ever performed under a lock the fold path needs.
	g.mu.Lock()
	if len(g.dirtyBuckets) == 0 && len(g.dirtySpecies) == 0 {
		g.mu.Unlock()
		return nil
	}
	lines := make([]brainLine, 0, len(g.dirtyBuckets)+len(g.dirtySpecies))
	for k := range g.dirtyBuckets {
		if b := g.buckets[k]; b != nil {
			lines = append(lines, bucketLine(k, b))
		}
	}
	for k := range g.dirtySpecies {
		if r, ok := g.species[k]; ok {
			lines = append(lines, speciesLine(k, r))
		}
	}
	g.dirtyBuckets = map[int64]bool{}
	g.dirtySpecies = map[string]bool{}
	live := int64(len(g.buckets))*brainLiveBucketBytes + int64(len(g.species))*brainLiveSpeciesBytes
	g.mu.Unlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.f == nil {
		return nil
	}
	s.live = live
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
	if s.bytes > brainCompactFloor && s.bytes > brainCompactRatio*s.live {
		return s.compactLocked(g)
	}
	return nil
}

// brainLiveBucketBytes and brainLiveSpeciesBytes are what one live record costs
// on disk, near enough for the compaction test. Measured on the running rig's
// shape — around 20 neuron keys and 50 synapse keys a bucket — a bucket line is
// about 500 bytes and a species line about 200.
const (
	brainLiveBucketBytes  int64 = 512
	brainLiveSpeciesBytes int64 = 200
)

func bucketLine(k int64, b *brainBucket) brainLine {
	l := brainLine{R: "b", T: k, Held: b.held, Bin: b.binned}
	if len(b.neurons) > 0 {
		l.Neu = dumpHist(b.neurons)
	}
	if len(b.synapses) > 0 {
		l.Syn = dumpHist(b.synapses)
	}
	return l
}

func speciesLine(k string, r brainRecord) brainLine {
	return brainLine{R: "s", K: k, Neurons: r.Neurons, Synapses: r.Synapses,
		AtMs: r.AtMs, Hash: r.Hash}
}

func dumpHist(h brainHist) map[string]int {
	out := make(map[string]int, len(h))
	for v, c := range h {
		out[histKey(int(v))] = int(c)
	}
	return out
}

func histKey(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [8]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

// compactLocked rewrites the whole file into place. The caller holds s.mu; the
// aggregate's lock is taken here, briefly, to copy the live set.
func (s *brainSidecar) compactLocked(g *brainAgg) error {
	g.mu.Lock()
	lines := make([]brainLine, 0, len(g.buckets)+len(g.species))
	for k, b := range g.buckets {
		lines = append(lines, bucketLine(k, b))
	}
	for k, r := range g.species {
		lines = append(lines, speciesLine(k, r))
	}
	// The dirty sets are cleared: everything is about to be on disk.
	g.dirtyBuckets = map[int64]bool{}
	g.dirtySpecies = map[string]bool{}
	g.mu.Unlock()

	tmp := s.path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	total := int64(0)
	head, err := json.Marshal(brainLine{R: "h", V: brainSidecarVersion,
		BucketMs: BrainBucketMs, SavedAt: time.Now().UnixMilli()})
	if err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	n, err := w.Write(append(head, '\n'))
	total += int64(n)
	if err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	for _, l := range lines {
		b, err := json.Marshal(l)
		if err != nil {
			continue
		}
		n, err := w.Write(append(b, '\n'))
		total += int64(n)
		if err != nil {
			f.Close()
			os.Remove(tmp)
			return err
		}
	}
	if err := w.Flush(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	// RENAME INTO PLACE, then flush the directory entry: the old file is whole
	// until the instant the new one is, so a kill during a compaction loses the
	// appends since the last one and never the history.
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

// Close flushes what is pending and closes the file.
func (s *brainSidecar) Close(g *brainAgg) error {
	if s == nil {
		return nil
	}
	if err := s.Save(g); err != nil {
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

// Path is the sidecar file, for tests and operator tools.
func (s *brainSidecar) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}
