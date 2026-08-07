package archive

// The durable half of the operator surface: the history strip under the live
// map.
//
// The live view forgets. A browser tab that was not open when a world went dark
// shows nothing about it afterwards, and metrics.jsonl exists for exactly that
// reason. This file turns that append-only sample file into something a page can
// draw without ever being handed the file itself: a fixed number of BUCKETS over
// a bounded window, each carrying the mean of the samples inside it.
//
// Three properties matter and each is a rule below:
//
//  1. BOUNDED WORK. A history request reads the TAIL of the sample file, never
//     the whole of it, and answers from a short-lived cache. The migration path
//     must never wait for a reader (Risk 4), and neither must the disk.
//  2. UNKNOWN SURVIVES DOWNSAMPLING. A bucket in which no sample knew a value is
//     null, not zero — the same rule §10.1 applies to the live view. A gap in
//     the record has to look like a gap on the sparkline.
//  3. DARKNESS SURVIVES DOWNSAMPLING. A bucket in which a slot was seen not live
//     is marked dark, because Risk 5's failure mode is a healed map hiding a
//     world that died — and the history strip is where an operator sees the hour
//     it happened.

import (
	"encoding/json"
	"io"
	"os"
	"sort"
	"strconv"
	"time"
)

// historyTailBytes bounds how much of metrics.jsonl one request reads. At the
// one-minute sample cadence a full day of a six-slot map is a few megabytes, so
// this covers roughly a week and still refuses to read an unbounded file.
const historyTailBytes int64 = 48 << 20

// historyCacheFor is how long one built history is reused. The page polls the
// live status every 2s and the history far more slowly; the cache is what keeps
// an over-eager reader (or a second browser) off the disk.
const historyCacheFor = 10 * time.Second

// The window and bucket count a request may ask for.
const (
	HistoryDefaultWindow  = 24 * time.Hour
	HistoryMinWindow      = 5 * time.Minute
	HistoryMaxWindow      = 7 * 24 * time.Hour
	HistoryDefaultBuckets = 120
	HistoryMinBuckets     = 8
	HistoryMaxBuckets     = 720
)

// HistoryPoint is one downsampled bucket of one series.
type HistoryPoint struct {
	// AtMs is the START of the bucket.
	AtMs int64 `json:"tMs"`
	// Value is the mean of the known values in the bucket, rounded — and NULL
	// when no sample in the bucket knew one. Never omitted, so the page can tell
	// "no reading" from "a reading of zero" without counting array indices.
	Value *int `json:"v"`
	// Dark marks a bucket in which the slot was seen not live at least once.
	Dark bool `json:"dark,omitempty"`
	// N is how many samples fell in the bucket; 0 is a gap in the record itself
	// — the archive was not running, or the map had not formed yet.
	N int `json:"n"`
}

// HistorySeries is one world's sparkline.
type HistorySeries struct {
	Slot   int            `json:"slot"`
	PeerID string         `json:"peerId"`
	Points []HistoryPoint `json:"points"`
	// Last is the newest known value in the window, or null when the whole
	// window is unknown.
	Last *int `json:"last"`
	// Max is the largest known value in this series.
	Max int `json:"max"`
}

// History is the whole strip: one series per slot, plus the two map-wide series
// the header reads.
type History struct {
	GeneratedAtMs int64 `json:"generatedAtMs"`
	FromMs        int64 `json:"fromMs"`
	ToMs          int64 `json:"toMs"`
	BucketMs      int64 `json:"bucketMs"`
	Buckets       int   `json:"buckets"`
	// Samples is how many raw samples fell inside the window, and Truncated says
	// the tail read did not reach FromMs — so the oldest buckets are empty
	// because of the read bound, not because nothing happened.
	Samples   int  `json:"samples"`
	Truncated bool `json:"truncated"`
	// MaxPopulation is the largest per-slot population anywhere in the window,
	// which is the SHARED y-scale of every sparkline. Small multiples that do not
	// share a scale invite the reader to compare shapes that are not comparable.
	MaxPopulation int             `json:"maxPopulation"`
	Slots         []HistorySeries `json:"slots"`
	// Total is the map-wide population, summed over the slots that knew one.
	Total []HistoryPoint `json:"total"`
	// Flow is envelopes RECORDED IN each bucket — the difference between
	// consecutive cumulative counts, never the cumulative count itself.
	Flow []HistoryPoint `json:"flow"`
}

// ReadMetricsTail replays at most maxBytes from the END of a metrics file, in
// write order. A partial first line (the read landed mid-record) is dropped, as
// is a torn final line, exactly as ReadMetrics drops one. The second return
// value is true when the file was longer than the read bound.
func ReadMetricsTail(path string, maxBytes int64) ([]Status, bool, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, false, err
	}
	truncated := false
	if maxBytes > 0 && info.Size() > maxBytes {
		if _, err := f.Seek(info.Size()-maxBytes, io.SeekStart); err != nil {
			return nil, false, err
		}
		truncated = true
	}
	b, err := io.ReadAll(f)
	if err != nil {
		return nil, truncated, err
	}
	start := 0
	if truncated {
		// The read began mid-record; skip to the first complete line.
		for start < len(b) && b[start] != '\n' {
			start++
		}
		if start < len(b) {
			start++
		}
	}
	var out []Status
	for i := start; i < len(b); i++ {
		if b[i] != '\n' {
			continue
		}
		var s Status
		if json.Unmarshal(b[start:i], &s) == nil {
			out = append(out, s)
		}
		start = i + 1
	}
	return out, truncated, nil
}

// bucketAcc accumulates one series inside one bucket.
type bucketAcc struct {
	sum  int
	have int
	n    int
	dark bool
}

func (b *bucketAcc) point(atMs int64) HistoryPoint {
	p := HistoryPoint{AtMs: atMs, N: b.n, Dark: b.dark}
	if b.have > 0 {
		v := int(float64(b.sum)/float64(b.have) + 0.5)
		p.Value = &v
	}
	return p
}

// BuildHistory downsamples samples into a fixed number of buckets ending at
// nowMs. It is pure: every input is an argument, which is what makes the
// downsampling testable without a rig or a file.
//
// Samples OLDER than the window are not discarded outright — the newest one
// before the window is the baseline the first flow delta is measured from, so
// the leftmost bar of the flow series is a real difference rather than a
// cumulative total drawn as a spike.
func BuildHistory(samples []Status, nowMs int64, window time.Duration, buckets int) History {
	if window < HistoryMinWindow {
		window = HistoryMinWindow
	}
	if window > HistoryMaxWindow {
		window = HistoryMaxWindow
	}
	if buckets < HistoryMinBuckets {
		buckets = HistoryMinBuckets
	}
	if buckets > HistoryMaxBuckets {
		buckets = HistoryMaxBuckets
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

	// Write order is the sample order, but a file concatenated across restarts
	// can hold a clock that stepped. Sort by the sample's own timestamp so the
	// flow deltas are measured along real time.
	ordered := append([]Status(nil), samples...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].GeneratedAtMs < ordered[j].GeneratedAtMs
	})

	perSlot := map[int][]bucketAcc{}
	peerOf := map[int]string{}
	total := make([]bucketAcc, buckets)
	flow := make([]bucketAcc, buckets)

	prevMigrations, havePrev := 0, false
	for _, s := range ordered {
		if s.GeneratedAtMs < fromMs {
			// Outside the window: it contributes nothing but the flow baseline.
			prevMigrations, havePrev = s.Totals.Migrations, true
			continue
		}
		idx := int((s.GeneratedAtMs - fromMs) / bucketMs)
		if idx < 0 || idx >= buckets {
			continue
		}
		h.Samples++

		for _, sv := range s.Slots {
			acc, ok := perSlot[sv.Slot]
			if !ok {
				acc = make([]bucketAcc, buckets)
				perSlot[sv.Slot] = acc
			}
			if sv.PeerID != "" {
				peerOf[sv.Slot] = sv.PeerID
			}
			acc[idx].n++
			if !sv.Live {
				acc[idx].dark = true
			}
			if sv.Population != nil {
				acc[idx].sum += *sv.Population
				acc[idx].have++
				if *sv.Population > h.MaxPopulation {
					h.MaxPopulation = *sv.Population
				}
			}
		}

		total[idx].n++
		if s.Totals.Population != nil {
			total[idx].sum += *s.Totals.Population
			total[idx].have++
		}

		// Flow is a DIFFERENCE. A cumulative counter that went backwards means a
		// new ledger, not negative migration: clamp rather than invent.
		flow[idx].n++
		if havePrev {
			d := s.Totals.Migrations - prevMigrations
			if d < 0 {
				d = 0
			}
			flow[idx].sum += d
			flow[idx].have = 1
		}
		prevMigrations, havePrev = s.Totals.Migrations, true
	}

	for i := 0; i < buckets; i++ {
		at := fromMs + int64(i)*bucketMs
		h.Total[i] = total[i].point(at)
		// The flow bucket is a SUM over the bucket, not a mean of deltas.
		p := HistoryPoint{AtMs: at, N: flow[i].n}
		if flow[i].have > 0 {
			v := flow[i].sum
			p.Value = &v
		}
		h.Flow[i] = p
	}

	slots := make([]int, 0, len(perSlot))
	for slot := range perSlot {
		slots = append(slots, slot)
	}
	sort.Ints(slots)
	for _, slot := range slots {
		acc := perSlot[slot]
		series := HistorySeries{Slot: slot, PeerID: peerOf[slot],
			Points: make([]HistoryPoint, buckets)}
		for i := 0; i < buckets; i++ {
			p := acc[i].point(fromMs + int64(i)*bucketMs)
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

// HistoryView answers one /api/history request, from a short-lived cache over a
// bounded tail read of the sample file.
func (a *Archive) HistoryView(window time.Duration, buckets int) (History, error) {
	key := window.String() + "/" + strconv.Itoa(buckets)
	a.historyMu.Lock()
	defer a.historyMu.Unlock()
	if a.historyKey == key && time.Since(a.historyAt) < historyCacheFor {
		return a.historyVal, nil
	}
	samples, truncated, err := ReadMetricsTail(a.metrics.Path(), historyTailBytes)
	if err != nil {
		return History{}, err
	}
	h := BuildHistory(samples, time.Now().UnixMilli(), window, buckets)
	h.Truncated = truncated
	a.historyKey, a.historyAt, a.historyVal = key, time.Now(), h
	return h, nil
}
