package archive

// THE METRICS ROLLUP: the persisted buckets HistoryAllView survives retirement
// on. metrics.jsonl was append-forever with no window at all; the metrics window
// (metricsseg.go) rotates it into dated segments and removes the old ones behind
// a confirmed off-host copy, exactly as the ledger window does. This file is what
// makes that safe for the OPERATOR's history strip: before a day's raw segment
// can retire, the day is FOLDED into these buckets, so the all-record view is
// answered from a compact aggregate rather than from a growing file — and the
// answer survives the raw segment leaving the host.
//
// IT IS THE SAME "one full scan, then map lookups" the ledger roll-up is. The
// buckets are additive sufficient statistics at a fixed, absolute-aligned base
// resolution, so folding a run day by day and folding it all at once produce the
// SAME buckets — the rollup-equivalence property the tests pin. The flow series
// is a difference of a cumulative counter, so the fold carries the last count
// across the day boundary; that carry is the one piece of state that makes the
// per-day fold equal the all-at-once fold.

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// metricsRollupName is the sidecar the rollup persists to, beside metrics.jsonl.
const metricsRollupName = "metrics-rollup.json"

// metricsRollupBaseMs is the base bucket width — one hour. The all-record view
// draws at most HistoryMaxBuckets (720) points, which over the announced 30-day
// run is one point an hour, so an hourly base is the resolution the view already
// shows and never coarser than it. A run shorter than the base sees the base as
// its floor, which is the correct behaviour for an "all record" overview.
const metricsRollupBaseMs int64 = 60 * 60 * 1000

// statAcc is one series' sufficient statistics inside one base bucket. Every
// field is additive over disjoint time ranges, which is what makes the fold
// associative and therefore equivalent however the samples are split.
type statAcc struct {
	Sum  int  `json:"s"`
	Have int  `json:"h"`
	N    int  `json:"n"`
	Dark bool `json:"d,omitempty"`
}

func (a *statAcc) add(o statAcc) {
	a.Sum += o.Sum
	a.Have += o.Have
	a.N += o.N
	if o.Dark {
		a.Dark = true
	}
}

// slotAcc is one slot's stats in one base bucket.
type slotAcc struct {
	Slot   int     `json:"slot"`
	PeerID string  `json:"peer,omitempty"`
	Acc    statAcc `json:"acc"`
}

// baseBucket is one hour of the record: per-slot, total and flow sufficient
// statistics. It is the persisted unit.
type baseBucket struct {
	StartMs int64     `json:"t"`
	Slots   []slotAcc `json:"slots"`
	Total   statAcc   `json:"total"`
	Flow    statAcc   `json:"flow"`
}

// metricsRollupState is the on-disk shape.
type metricsRollupState struct {
	BaseMs         int64        `json:"baseMs"`
	Folded         []string     `json:"folded"`
	Buckets        []baseBucket `json:"buckets"`
	LastMigrations int          `json:"lastMigrations"`
	LastGenAtMs    int64        `json:"lastGenAtMs"`
	HaveLast       bool         `json:"haveLast"`
	// LastDays is each folded segment's NEWEST-sample UTC-midnight ms. It exists
	// because RotateTo moves the whole live file into one segment named by its
	// FIRST day — so a segment's retention and rolling-view membership must be
	// judged by its LAST sample, not by the weeks-ago label at first enablement.
	LastDays map[string]int64 `json:"lastDays,omitempty"`
}

// metricsRollup is the live rollup. Its lock is its own — a fold reads a segment
// and rewrites a small sidecar, and nothing that touches a file may hold the
// migration-path lock.
type metricsRollup struct {
	path    string
	mu      sync.Mutex
	baseMs  int64
	folded  map[string]bool
	buckets map[int64]*baseBucket
	// slotIdx indexes each bucket's Slots by slot number, for O(1) accumulation.
	slotIdx map[int64]map[int]int
	// lastDayByName is each folded segment's newest-sample UTC-midnight ms.
	lastDayByName  map[string]int64
	lastMigrations int
	lastGenAtMs    int64
	haveLast       bool
}

// openMetricsRollup loads the rollup sidecar, or starts an empty one. An
// unreadable sidecar is a loss that is said so and never a reason to refuse to
// run: the all-record view falls back to the raw segments still on the host.
func openMetricsRollup(dir string) (*metricsRollup, error) {
	mr := &metricsRollup{
		path:          filepath.Join(dir, metricsRollupName),
		baseMs:        metricsRollupBaseMs,
		folded:        map[string]bool{},
		buckets:       map[int64]*baseBucket{},
		slotIdx:       map[int64]map[int]int{},
		lastDayByName: map[string]int64{},
	}
	b, err := os.ReadFile(mr.path)
	if errors.Is(err, os.ErrNotExist) {
		return mr, nil
	}
	if err != nil {
		return mr, err
	}
	var st metricsRollupState
	if err := json.Unmarshal(b, &st); err != nil {
		// A TORN SIDECAR MUST NOT SILENTLY BECOME AN EMPTY ROLLUP: that would
		// discard the folded history of every already-retired day (whose raw
		// segment is gone from the host) without a trace. Move the corrupt file
		// aside so it can be recovered by hand, start fresh, and make the caller
		// say so at Error.
		aside := fmt.Sprintf("%s.unreadable.%d", mr.path, time.Now().UnixNano())
		_ = os.Rename(mr.path, aside)
		return mr, fmt.Errorf("metrics rollup sidecar %s did not parse and was moved to %s: %w",
			filepath.Base(mr.path), filepath.Base(aside), err)
	}
	if st.BaseMs > 0 {
		mr.baseMs = st.BaseMs
	}
	for _, n := range st.Folded {
		mr.folded[n] = true
	}
	for i := range st.Buckets {
		bb := st.Buckets[i]
		cp := bb
		mr.buckets[bb.StartMs] = &cp
		idx := map[int]int{}
		for j, s := range cp.Slots {
			idx[s.Slot] = j
		}
		mr.slotIdx[bb.StartMs] = idx
	}
	for n, d := range st.LastDays {
		mr.lastDayByName[n] = d
	}
	mr.lastMigrations = st.LastMigrations
	mr.lastGenAtMs = st.LastGenAtMs
	mr.haveLast = st.HaveLast
	return mr, nil
}

// alreadyFolded reports whether a segment has been folded into the rollup.
func (mr *metricsRollup) alreadyFolded(name string) bool {
	if mr == nil {
		return true // no rollup: never fold, the raw segments answer
	}
	mr.mu.Lock()
	defer mr.mu.Unlock()
	return mr.folded[name]
}

// foldSegment folds one closed metrics segment's samples into the rollup and
// records it as folded. It is idempotent on the segment NAME: a segment already
// folded is skipped, which is what stops a restart double-counting a day.
func (mr *metricsRollup) foldSegment(name string, samples []historyStatus) error {
	if mr == nil {
		return nil
	}
	mr.mu.Lock()
	defer mr.mu.Unlock()
	if mr.folded[name] {
		return nil
	}
	// REFUSE AN OUT-OF-ORDER FOLD. The flow series is a running difference of a
	// cumulative counter carried across the day boundary (lastMigrations), so
	// folding a day whose samples predate what the rollup has already folded would
	// regress that carry and double-count flow in the forever-kept rollup. It
	// happens when a day's fold fails transiently (an ENOSPC) and a LATER day folds
	// before it retries. Refuse it loudly; the segment stays unfolded and therefore
	// un-retired, which is the safe direction.
	newest := int64(0)
	for _, s := range samples {
		if s.GeneratedAtMs > newest {
			newest = s.GeneratedAtMs
		}
	}
	if mr.haveLast && newest > 0 && newest/mr.baseMs < mr.lastGenAtMs/mr.baseMs {
		return fmt.Errorf("metrics rollup: refusing to fold %s OUT OF ORDER: its newest sample %d "+
			"predates the folded frontier %d; the flow carry would regress", name, newest, mr.lastGenAtMs)
	}
	mr.foldSamplesLocked(samples)
	mr.folded[name] = true
	if newest > 0 {
		mr.lastDayByName[name] = time.UnixMilli(newest).UTC().Truncate(24 * time.Hour).UnixMilli()
	}
	return mr.saveLocked()
}

// segmentLastDayMs is the newest-sample UTC-midnight of a folded segment, or
// (0,false) when it has not been folded — the caller then falls back to the day
// in the segment's name.
func (mr *metricsRollup) segmentLastDayMs(name string) (int64, bool) {
	if mr == nil {
		return 0, false
	}
	mr.mu.Lock()
	defer mr.mu.Unlock()
	d, ok := mr.lastDayByName[name]
	return d, ok
}

// foldSamplesLocked accumulates samples into the base buckets, in the sample's
// own time order, carrying the flow baseline across whatever boundary the caller
// split the record on.
func (mr *metricsRollup) foldSamplesLocked(samples []historyStatus) {
	ordered := append([]historyStatus(nil), samples...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].GeneratedAtMs < ordered[j].GeneratedAtMs
	})
	for _, s := range ordered {
		if s.GeneratedAtMs <= 0 {
			continue
		}
		start := (s.GeneratedAtMs / mr.baseMs) * mr.baseMs
		bb := mr.buckets[start]
		if bb == nil {
			bb = &baseBucket{StartMs: start}
			mr.buckets[start] = bb
			mr.slotIdx[start] = map[int]int{}
		}
		idx := mr.slotIdx[start]
		for _, sv := range s.Slots {
			j, ok := idx[sv.Slot]
			if !ok {
				bb.Slots = append(bb.Slots, slotAcc{Slot: sv.Slot, PeerID: sv.PeerID})
				j = len(bb.Slots) - 1
				idx[sv.Slot] = j
			}
			if sv.PeerID != "" {
				bb.Slots[j].PeerID = sv.PeerID
			}
			acc := &bb.Slots[j].Acc
			acc.N++
			if !sv.Live {
				acc.Dark = true
			}
			if sv.Population != nil {
				acc.Sum += *sv.Population
				acc.Have++
			}
		}
		bb.Total.N++
		if s.Totals.Population != nil {
			bb.Total.Sum += *s.Totals.Population
			bb.Total.Have++
		}
		bb.Flow.N++
		if mr.haveLast {
			d := s.Totals.Migrations - mr.lastMigrations
			if d < 0 {
				d = 0
			}
			bb.Flow.Sum += d
			bb.Flow.Have = 1
		}
		mr.lastMigrations = s.Totals.Migrations
		mr.lastGenAtMs = s.GeneratedAtMs
		mr.haveLast = true
	}
}

// snapshot returns a copy of the base buckets plus the flow carry, for a reader
// that folds the live tail on top without touching the persisted state.
func (mr *metricsRollup) snapshot() ([]baseBucket, int, bool) {
	if mr == nil {
		return nil, 0, false
	}
	mr.mu.Lock()
	defer mr.mu.Unlock()
	out := make([]baseBucket, 0, len(mr.buckets))
	for _, bb := range mr.buckets {
		cp := *bb
		cp.Slots = append([]slotAcc(nil), bb.Slots...)
		out = append(out, cp)
	}
	return out, mr.lastMigrations, mr.haveLast
}

// baseBucketsForTest exposes the buckets sorted by time, for the equivalence
// tests. It is exact sufficient statistics, so two rollups that fold the same
// samples — however split — compare equal here.
func (mr *metricsRollup) baseBucketsForTest() []baseBucket {
	buckets, _, _ := mr.snapshot()
	sort.Slice(buckets, func(i, j int) bool { return buckets[i].StartMs < buckets[j].StartMs })
	for i := range buckets {
		sort.Slice(buckets[i].Slots, func(a, b int) bool {
			return buckets[i].Slots[a].Slot < buckets[i].Slots[b].Slot
		})
	}
	return buckets
}

func (mr *metricsRollup) saveLocked() error {
	st := metricsRollupState{
		BaseMs:         mr.baseMs,
		LastMigrations: mr.lastMigrations,
		LastGenAtMs:    mr.lastGenAtMs,
		HaveLast:       mr.haveLast,
		LastDays:       map[string]int64{},
	}
	for n, d := range mr.lastDayByName {
		st.LastDays[n] = d
	}
	for n := range mr.folded {
		st.Folded = append(st.Folded, n)
	}
	sort.Strings(st.Folded)
	for _, bb := range mr.buckets {
		st.Buckets = append(st.Buckets, *bb)
	}
	sort.Slice(st.Buckets, func(i, j int) bool { return st.Buckets[i].StartMs < st.Buckets[j].StartMs })
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	tmp := mr.path + tmpSuffix
	// FSYNC BEFORE RENAME. This sidecar is the ONLY copy of the folded history for
	// days whose raw segment has already retired off the host; a torn write here
	// loses that history for good, so the bytes must reach the disk before the
	// rename makes them the live file.
	if err := writeFileSync(tmp, append(b, '\n')); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, mr.path)
}

// mergeLiveIntoBase folds the live-file samples on top of a copy of the persisted
// base buckets, continuing the flow carry from the last folded sample. It is what
// lets HistoryAllView answer from the rollup PLUS today without persisting today.
func mergeLiveIntoBase(base []baseBucket, baseMs int64, lastMigrations int, haveLast bool,
	live []historyStatus) []baseBucket {

	if baseMs <= 0 {
		baseMs = metricsRollupBaseMs
	}
	m := map[int64]*baseBucket{}
	idxOf := map[int64]map[int]int{}
	for i := range base {
		cp := base[i]
		cp.Slots = append([]slotAcc(nil), base[i].Slots...)
		m[cp.StartMs] = &cp
		idx := map[int]int{}
		for j, s := range cp.Slots {
			idx[s.Slot] = j
		}
		idxOf[cp.StartMs] = idx
	}
	ordered := append([]historyStatus(nil), live...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].GeneratedAtMs < ordered[j].GeneratedAtMs
	})
	for _, s := range ordered {
		if s.GeneratedAtMs <= 0 {
			continue
		}
		start := (s.GeneratedAtMs / baseMs) * baseMs
		bb := m[start]
		if bb == nil {
			bb = &baseBucket{StartMs: start}
			m[start] = bb
			idxOf[start] = map[int]int{}
		}
		idx := idxOf[start]
		for _, sv := range s.Slots {
			j, ok := idx[sv.Slot]
			if !ok {
				bb.Slots = append(bb.Slots, slotAcc{Slot: sv.Slot, PeerID: sv.PeerID})
				j = len(bb.Slots) - 1
				idx[sv.Slot] = j
			}
			if sv.PeerID != "" {
				bb.Slots[j].PeerID = sv.PeerID
			}
			acc := &bb.Slots[j].Acc
			acc.N++
			if !sv.Live {
				acc.Dark = true
			}
			if sv.Population != nil {
				acc.Sum += *sv.Population
				acc.Have++
			}
		}
		bb.Total.N++
		if s.Totals.Population != nil {
			bb.Total.Sum += *s.Totals.Population
			bb.Total.Have++
		}
		bb.Flow.N++
		if haveLast {
			d := s.Totals.Migrations - lastMigrations
			if d < 0 {
				d = 0
			}
			bb.Flow.Sum += d
			bb.Flow.Have = 1
		}
		lastMigrations = s.Totals.Migrations
		haveLast = true
	}
	out := make([]baseBucket, 0, len(m))
	for _, bb := range m {
		out = append(out, *bb)
	}
	return out
}

// buildHistoryFromBase turns base buckets — the persisted rollup plus the live
// tail folded on top — into a History at the requested bucket count. It is the
// all-record view's builder now that the view no longer scans the raw file.
func buildHistoryFromBase(base []baseBucket, nowMs int64, buckets int) History {
	if buckets < HistoryMinBuckets {
		buckets = HistoryMinBuckets
	}
	if buckets > HistoryMaxBuckets {
		buckets = HistoryMaxBuckets
	}
	fromBase := int64(0)
	haveAny := false
	for _, bb := range base {
		if !haveAny || bb.StartMs < fromBase {
			fromBase, haveAny = bb.StartMs, true
		}
	}
	window := HistoryMinWindow
	if haveAny {
		if cand := time.Duration(nowMs-fromBase+int64(buckets)) * time.Millisecond; cand > window {
			window = cand
		}
	}
	windowMs := window.Milliseconds()
	bucketMs := windowMs / int64(buckets)
	if bucketMs < 1 {
		bucketMs = 1
	}
	fromMs := nowMs - bucketMs*int64(buckets)

	h := History{
		GeneratedAtMs: nowMs,
		FromMs:        fromMs,
		ToMs:          fromMs + bucketMs*int64(buckets),
		BucketMs:      bucketMs,
		Buckets:       buckets,
		Slots:         []HistorySeries{},
		Total:         make([]HistoryPoint, buckets),
		Flow:          make([]HistoryPoint, buckets),
	}
	perSlot := map[int][]statAcc{}
	peerOf := map[int]string{}
	total := make([]statAcc, buckets)
	flow := make([]statAcc, buckets)

	for _, bb := range base {
		if bb.StartMs < fromMs {
			continue
		}
		idx := int((bb.StartMs - fromMs) / bucketMs)
		if idx < 0 || idx >= buckets {
			continue
		}
		h.Samples += bb.Total.N
		total[idx].add(bb.Total)
		// MaxPopulation tracks the largest bucket MEAN: the raw peak is not
		// reconstructable from sums, and the all-record view uses the mean anyway.
		flow[idx].Sum += bb.Flow.Sum
		flow[idx].N += bb.Flow.N
		if bb.Flow.Have > 0 {
			flow[idx].Have = 1
		}
		for _, sv := range bb.Slots {
			acc := perSlot[sv.Slot]
			if acc == nil {
				acc = make([]statAcc, buckets)
				perSlot[sv.Slot] = acc
			}
			acc[idx].add(sv.Acc)
			if sv.PeerID != "" {
				peerOf[sv.Slot] = sv.PeerID
			}
			if sv.Acc.Have > 0 {
				mean := int(float64(sv.Acc.Sum)/float64(sv.Acc.Have) + 0.5)
				if mean > h.MaxPopulation {
					h.MaxPopulation = mean
				}
			}
		}
	}

	for i := 0; i < buckets; i++ {
		at := fromMs + int64(i)*bucketMs
		h.Total[i] = pointOf(total[i], at)
		p := HistoryPoint{AtMs: at, N: flow[i].N}
		if flow[i].Have > 0 {
			v := flow[i].Sum
			p.Value = &v
		}
		h.Flow[i] = p
	}
	slots := make([]int, 0, len(perSlot))
	for s := range perSlot {
		slots = append(slots, s)
	}
	sort.Ints(slots)
	for _, slot := range slots {
		acc := perSlot[slot]
		series := HistorySeries{Slot: slot, PeerID: peerOf[slot], Points: make([]HistoryPoint, buckets)}
		for i := 0; i < buckets; i++ {
			p := pointOf(acc[i], fromMs+int64(i)*bucketMs)
			series.Points[i] = p
			if p.Value != nil {
				if *p.Value > series.Max {
					series.Max = *p.Value
				}
				v := *p.Value
				series.Last = &v
			}
		}
		h.Slots = append(h.Slots, series)
	}
	return h
}

func pointOf(a statAcc, atMs int64) HistoryPoint {
	p := HistoryPoint{AtMs: atMs, N: a.N, Dark: a.Dark}
	if a.Have > 0 {
		v := int(float64(a.Sum)/float64(a.Have) + 0.5)
		p.Value = &v
	}
	return p
}

// readCompactSamples reads the compact history fields from a metrics segment,
// plain or gzip, in write order. A torn line is dropped.
func readCompactSamples(path string) ([]historyStatus, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var r *bufio.Reader
	if strings.HasSuffix(path, gzSuffix) {
		zr, err := gzip.NewReader(bufio.NewReaderSize(f, 1<<20))
		if err != nil {
			return nil, err
		}
		defer zr.Close()
		r = bufio.NewReaderSize(zr, 1<<20)
	} else {
		r = bufio.NewReaderSize(f, 1<<20)
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), historyLineMaxBytes)
	var out []historyStatus
	for sc.Scan() {
		var s historyStatus
		if json.Unmarshal(sc.Bytes(), &s) == nil {
			out = append(out, s)
		}
	}
	return out, sc.Err()
}
