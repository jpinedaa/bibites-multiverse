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
//  1. BOUNDED WORK. A rolling request reads the tail of the sample file. An
//     all-record request reads only compact fields, stops at a fixed byte bound,
//     and uses a longer cache. The migration path never waits for either reader
//     (Risk 4), and neither does the disk.
//  2. UNKNOWN SURVIVES DOWNSAMPLING. A bucket in which no sample knew a value is
//     null, not zero — the same rule §10.1 applies to the live view. A gap in
//     the record has to look like a gap on the sparkline.
//  3. DARKNESS SURVIVES DOWNSAMPLING. A bucket in which a slot was seen not live
//     is marked dark, because Risk 5's failure mode is a healed map hiding a
//     world that died — and the history strip is where an operator sees the hour
//     it happened.

import (
	"bufio"
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

// historyAllMaxBytes keeps an all-record request finite. The public experiment
// produces about 1.2 GB of metrics at its current rate, so this covers the
// announced run with headroom. If the file grows past this bound, the response
// says that its oldest part was truncated instead of claiming a complete view.
const historyAllMaxBytes int64 = 2 << 30

// historyAllCacheFor prevents the default all-record view from rescanning a
// growing sample file every minute. Live values remain in the page header.
const historyAllCacheFor = 10 * time.Minute

// One status line contains census detail that the history view ignores. This
// bound rejects a damaged line before bufio.Reader can consume arbitrary memory.
const historyLineMaxBytes = 16 << 20

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
	// MaxPopulation is the largest per-slot population anywhere in the window.
	// It remains useful to API readers even though the page fits each population
	// chart to that chart's visible minimum and maximum.
	MaxPopulation int             `json:"maxPopulation"`
	Slots         []HistorySeries `json:"slots"`
	// Total is the map-wide population, summed over the slots that knew one.
	Total []HistoryPoint `json:"total"`
	// Flow is envelopes RECORDED IN each bucket — the difference between
	// consecutive cumulative counts, never the cumulative count itself.
	Flow []HistoryPoint `json:"flow"`
}

// historyStatus is the small part of a Status that the history strip reads.
// An all-record scan decodes directly into this shape, so census detail never
// accumulates in memory.
type historyStatus struct {
	GeneratedAtMs int64               `json:"generatedAtMs"`
	Slots         []historySlotStatus `json:"slots"`
	Totals        historyTotalsStatus `json:"totals"`
}

type historySlotStatus struct {
	Slot       int    `json:"slot"`
	PeerID     string `json:"peerId"`
	Live       bool   `json:"live"`
	Population *int   `json:"population"`
}

type historyTotalsStatus struct {
	Population *int `json:"population"`
	Migrations int  `json:"migrations"`
}

func compactHistoryStatus(s Status) historyStatus {
	out := historyStatus{
		GeneratedAtMs: s.GeneratedAtMs,
		Totals: historyTotalsStatus{
			Population: s.Totals.Population,
			Migrations: s.Totals.Migrations,
		},
		Slots: make([]historySlotStatus, 0, len(s.Slots)),
	}
	for _, slot := range s.Slots {
		out.Slots = append(out.Slots, historySlotStatus{
			Slot: slot.Slot, PeerID: slot.PeerID, Live: slot.Live,
			Population: slot.Population,
		})
	}
	return out
}

// ReadHistoryStatuses reads compact history fields from a bounded part of the
// metrics file. A partial first line and a torn final line are discarded.
func ReadHistoryStatuses(path string, maxBytes int64) ([]historyStatus, bool, error) {
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
	size := info.Size()
	truncated := maxBytes > 0 && size > maxBytes
	start := int64(0)
	if truncated {
		start = size - maxBytes
	}
	dropFirst := truncated
	if start > 0 {
		var previous [1]byte
		if _, err := f.ReadAt(previous[:], start-1); err != nil {
			return nil, truncated, err
		}
		dropFirst = previous[0] != '\n'
	}
	endsWithNewline := false
	if size > 0 {
		var last [1]byte
		if _, err := f.ReadAt(last[:], size-1); err != nil {
			return nil, truncated, err
		}
		endsWithNewline = last[0] == '\n'
	}

	scanner := bufio.NewScanner(io.NewSectionReader(f, start, size-start))
	scanner.Buffer(make([]byte, 64<<10), historyLineMaxBytes)
	var out []historyStatus
	appendLine := func(line []byte) {
		var s historyStatus
		if json.Unmarshal(line, &s) == nil {
			out = append(out, s)
		}
	}
	var pending []byte
	for scanner.Scan() {
		if dropFirst {
			dropFirst = false
			continue
		}
		if pending != nil {
			appendLine(pending)
		}
		pending = append(pending[:0], scanner.Bytes()...)
	}
	if err := scanner.Err(); err != nil {
		return nil, truncated, &os.PathError{Op: "read", Path: path, Err: err}
	}
	// Append writes the JSON and newline together. A final token without the
	// newline is a torn append, even when its JSON happens to parse.
	if pending != nil && endsWithNewline {
		appendLine(pending)
	}
	return out, truncated, nil
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
	compact := make([]historyStatus, 0, len(samples))
	for _, sample := range samples {
		compact = append(compact, compactHistoryStatus(sample))
	}
	return buildHistory(compact, nowMs, window, buckets, true)
}

func buildAllHistory(samples []historyStatus, nowMs int64, buckets int) History {
	window := HistoryMinWindow
	for _, sample := range samples {
		if sample.GeneratedAtMs <= 0 || sample.GeneratedAtMs > nowMs {
			continue
		}
		candidate := time.Duration(nowMs-sample.GeneratedAtMs+int64(buckets)) * time.Millisecond
		if candidate > window {
			window = candidate
		}
	}
	return buildHistory(samples, nowMs, window, buckets, false)
}

func buildHistory(samples []historyStatus, nowMs int64, window time.Duration, buckets int, clampMax bool) History {
	if window < HistoryMinWindow {
		window = HistoryMinWindow
	}
	if clampMax && window > HistoryMaxWindow {
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
	ordered := append([]historyStatus(nil), samples...)
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
	nowMs := time.Now().UnixMilli()
	var h History
	if a.metricsWindowOn() {
		// The live file holds only today once the metrics window is rotating it, so
		// a window reaching back into a closed day walks the rotated set too.
		compact, truncated, err := a.rollingCompactSamples(window, nowMs)
		if err != nil {
			return History{}, err
		}
		h = buildHistory(compact, nowMs, window, buckets, true)
		h.Truncated = truncated
	} else {
		samples, truncated, err := ReadMetricsTail(a.metrics.Path(), historyTailBytes)
		if err != nil {
			return History{}, err
		}
		h = BuildHistory(samples, nowMs, window, buckets)
		h.Truncated = truncated
	}
	a.historyKey, a.historyAt, a.historyVal = key, time.Now(), h
	return h, nil
}

// HistoryAllView answers the all-record range from a compact, bounded scan.
// Its cache is separate from the rolling-window cache because the page can
// switch between both ranges without forcing either file read again.
func (a *Archive) HistoryAllView(buckets int) (History, error) {
	a.historyMu.Lock()
	defer a.historyMu.Unlock()
	if !a.historyAllAt.IsZero() && time.Since(a.historyAllAt) < historyAllCacheFor &&
		a.historyAllVal.Buckets == buckets {
		return a.historyAllVal, nil
	}
	nowMs := time.Now().UnixMilli()
	var h History
	if a.metricsWindowOn() {
		// PERSISTED BUCKETS + LIVE TAIL, never a scan of one growing file. The
		// rollup holds every folded (closed) day — including days whose raw segment
		// has retired off the host — and the live file holds today.
		base, carry, haveCarry := a.metricsRollup.snapshot()
		live, truncated, err := ReadHistoryStatuses(a.metrics.Path(), historyTailBytes)
		if err != nil {
			return History{}, err
		}
		merged := mergeLiveIntoBase(base, a.metricsRollup.baseMs, carry, haveCarry, live)
		h = buildHistoryFromBase(merged, nowMs, buckets)
		h.Truncated = truncated
	} else {
		samples, truncated, err := ReadHistoryStatuses(a.metrics.Path(), historyAllMaxBytes)
		if err != nil {
			return History{}, err
		}
		h = buildAllHistory(samples, nowMs, buckets)
		h.Truncated = truncated
	}
	a.historyAllAt, a.historyAllVal = time.Now(), h
	return h, nil
}
