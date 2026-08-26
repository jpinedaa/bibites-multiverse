package archive

// THE METRICS WINDOW. metrics.jsonl was append-forever with no window at all —
// the one durable file in this system that grew without bound. This gives it the
// SAME shape the ledger has: rotate into dated segments, fold each closed day
// into the persisted history rollup (metricshist.go) so the operator's history
// survives, compress the closed segment, and remove it from the host once an
// off-host copy is confirmed. The live file stays for tail reads.
//
// OFF BY DEFAULT. With no metrics window set nothing here rotates, folds, or
// retires: metrics.jsonl is one growing file and HistoryAllView scans it exactly
// as it always did. The deployment sets 720h, and turning the window on turns the
// rollup on with it — the buckets HistoryAllView answers from once the raw days
// start leaving the host.
//
// THE FOLD PRECEDES THE RETIREMENT, always. A day's raw segment cannot be removed
// until its samples are in the rollup, so the history strip never loses an hour
// to a segment that left. That ordering is this file's whole safety argument, the
// same one archive_rollup_design.md makes for the ledger: the aggregate is
// written before the raw it summarises can go.

import (
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"multiverse/internal/fsutil"
)

const metricsSegmentsDirName = "metrics-segments"

func metricsSegmentsDir(dir string) string { return filepath.Join(dir, metricsSegmentsDirName) }

// metricsSegState is the metrics window's accounting, under Archive.mu.
type metricsSegState struct {
	segments     int
	awaiting     int
	badReceipts  int
	retired      int
	retiredBytes int64
	compressed   int
	folded       int
	lastAt       time.Time
}

// metricsWindowOn reports whether closed metrics segments are ever removed.
func (a *Archive) metricsWindowOn() bool { return a.cfg.MetricsWindow > 0 }

// startMetricsMaintenance runs one pass immediately — so a segment a crash left
// unfolded or uncompressed is handled at once — and then on a ticker. It does
// nothing when no window is set: no rotation, no fold, no retirement.
func (a *Archive) startMetricsMaintenance() {
	if a.cfg.MetricsMaintenanceInterval < 0 {
		return // tests that drive the pass by hand
	}
	if !a.metricsWindowOn() {
		a.log.Info("archive: the metrics window is OFF; metrics.jsonl is kept whole",
			"note", "the history strip scans the file as it always has; nothing rotates or retires")
		return
	}
	a.log.Warn("archive: the metrics window is ON; closed metrics segments are folded into the "+
		"history rollup and removed from this host once an off-host copy is confirmed",
		"window", a.cfg.MetricsWindow.String(), "segments", metricsSegmentsDir(a.cfg.DataDir),
		"gate", "a segment is removed ONLY if its .jsonl.gz.receipt confirms an off-host copy. "+
			"No receipt means it stays, forever if need be",
		"note", "the history the strip draws is kept forever in the rollup; this is the RAW samples only")
	a.metricsMaintenancePass(time.Now())
	a.wg.Add(1)
	go func() { defer a.wg.Done(); a.metricsMaintenanceLoop() }()
}

func (a *Archive) metricsMaintenanceLoop() {
	interval := a.cfg.MetricsMaintenanceInterval
	if interval == 0 {
		interval = defaultSegmentMaintenanceInterval
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-a.ctx.Done():
			return
		case now := <-t.C:
			a.metricsMaintenancePass(now)
		}
	}
}

// metricsMaintenancePass rotates a stale live file, folds and compresses closed
// segments, retires what is past the window and confirmed off-host, and refreshes
// the counters. It runs OFF the archive's lock.
func (a *Archive) metricsMaintenancePass(now time.Time) {
	if !a.metricsWindowOn() {
		return // off by default: metrics.jsonl is kept whole and unrotated
	}
	dir := a.cfg.DataDir
	// Rotate the live file when its day is over, even with no sample since — the
	// same quiet-boundary close the ledger does.
	if name, err := a.rotateMetricsIfStale(now); err != nil {
		a.log.Warn("archive: rotating the metrics live file failed", "err", err)
	} else if name != "" {
		a.log.Info("archive: closed the metrics live file into a segment", "segment", name)
	}

	msd := metricsSegmentsDir(dir)
	segs := readMetricsSegmentDir(msd)

	// FOLD FIRST, so nothing retires before its day is in the rollup.
	folded := 0
	for _, s := range segs {
		if s.retired || a.metricsRollup.alreadyFolded(s.name) {
			continue
		}
		if s.path == "" {
			continue
		}
		samples, err := readCompactSamples(s.path)
		if err != nil {
			a.log.Warn("archive: reading a metrics segment to fold it failed; it is KEPT and NOT retired",
				"segment", s.name, "err", err)
			continue
		}
		if err := a.metricsRollup.foldSegment(s.name, samples); err != nil {
			a.log.Error("archive: folding a metrics segment into the history rollup failed; it is KEPT",
				"segment", s.name, "err", err)
			continue
		}
		folded++
	}

	// COMPRESS closed plain segments.
	compressed := 0
	for _, s := range segs {
		if s.retired || s.compressed || s.path == "" {
			continue
		}
		if err := compressPlainFile(s.path); err != nil {
			a.log.Error("archive: compressing a metrics segment failed; the plain file is KEPT",
				"segment", s.name, "err", err)
			continue
		}
		compressed++
	}
	if compressed > 0 || folded > 0 {
		segs = readMetricsSegmentDir(msd)
	}

	// RETIRE past the window and behind a confirmed off-host copy.
	window := a.cfg.MetricsWindow
	awaiting, bad, retired := 0, 0, 0
	var retiredBytes int64
	for _, s := range segs {
		if s.retired || !s.compressed || s.path == "" {
			continue
		}
		// A segment must be folded before it can retire; the fold pass above did it,
		// but a segment that failed to fold is held here too.
		if !a.metricsRollup.alreadyFolded(s.name) {
			awaiting++
			continue
		}
		if window <= 0 || now.Sub(a.metricsSegEnd(s)) <= window {
			continue
		}
		verdict, why := checkColdCopyMetrics(msd, s.name)
		switch verdict {
		case coldCopyMissing:
			awaiting++
			continue
		case coldCopyBad:
			awaiting++
			bad++
			a.log.Error("archive: a metrics segment's cold-copy receipt does not hold up; the segment is KEPT",
				"segment", s.name, "reason", why)
			continue
		}
		bytes := s.bytes
		if err := os.Remove(s.path); err != nil {
			a.log.Error("archive: removing a retired metrics segment failed", "segment", s.name, "err", err)
			awaiting++
			continue
		}
		_ = fsutil.SyncDir(msd)
		retired++
		retiredBytes += bytes
		a.log.Warn("archive: a metrics segment was REMOVED FROM THIS HOST; the history rollup keeps its day",
			"segment", s.name+gzSuffix, "bytes", bytes, "window", window.String())
	}

	present := 0
	for _, s := range segs {
		if !s.retired {
			present++
		}
	}

	a.mu.Lock()
	a.metricsSeg.segments = present
	a.metricsSeg.awaiting = awaiting
	a.metricsSeg.badReceipts = bad
	a.metricsSeg.retired += retired
	a.metricsSeg.retiredBytes += retiredBytes
	a.metricsSeg.compressed += compressed
	a.metricsSeg.folded += folded
	a.metricsSeg.lastAt = now
	a.mu.Unlock()
}

// rotateMetricsIfStale closes the live file into a dated segment when its first
// sample's day is already over. The FOLD is left to the maintenance pass (and, on
// a crash between here and it, to foldPendingMetricsSegments at startup), which
// folds oldest-first — the one ordering the flow carry depends on.
func (a *Archive) rotateMetricsIfStale(now time.Time) (string, error) {
	if a.metrics == nil {
		return "", nil
	}
	firstDayMs := a.metrics.FirstDayMs()
	if firstDayMs == 0 {
		return "", nil
	}
	today := now.UTC().Truncate(24 * time.Hour).UnixMilli()
	if firstDayMs >= today {
		return "", nil
	}
	msd := metricsSegmentsDir(a.cfg.DataDir)
	if err := os.MkdirAll(msd, 0o755); err != nil {
		return "", err
	}
	day := time.UnixMilli(firstDayMs).UTC()
	name := segmentName(day, nextMetricsSeq(msd, day))
	dest := filepath.Join(msd, name+plainSuffix)
	rotated, err := a.metrics.RotateTo(dest)
	if err != nil || !rotated {
		return "", err
	}
	// NO IMMEDIATE FOLD HERE. The maintenance pass re-reads the directory and folds
	// oldest-first straight after this returns; folding here as well would fold the
	// just-rotated (newest) day BEFORE an older unfolded segment, and foldSegment's
	// out-of-order guard would then refuse that older day forever. One fold path,
	// in order, is the whole point.
	return name, nil
}

// foldPendingMetricsSegments folds every closed metrics segment the rollup does
// not already hold. It runs once from New, so a segment a crash left between the
// rotation and the fold is folded before anything can retire it.
func (a *Archive) foldPendingMetricsSegments() {
	msd := metricsSegmentsDir(a.cfg.DataDir)
	for _, s := range readMetricsSegmentDir(msd) {
		if s.retired || s.path == "" || a.metricsRollup.alreadyFolded(s.name) {
			continue
		}
		samples, err := readCompactSamples(s.path)
		if err != nil {
			continue
		}
		_ = a.metricsRollup.foldSegment(s.name, samples)
	}
}

// MetricsMaintenanceNow runs one pass synchronously, for tests.
func (a *Archive) MetricsMaintenanceNow(now time.Time) { a.metricsMaintenancePass(now) }

// rollingCompactSamples reads the compact history samples a rolling window needs:
// the live file's bounded tail, plus every closed metrics segment whose day
// intersects the window. It is what makes HistoryView correct after the live file
// has rotated a day out from under a 24 h window. The bool is true when the read
// could not reach the window's start — a needed segment has retired off the host,
// or the live tail was bounded short.
func (a *Archive) rollingCompactSamples(window time.Duration, nowMs int64) ([]historyStatus, bool, error) {
	fromMs := nowMs - window.Milliseconds()
	msd := metricsSegmentsDir(a.cfg.DataDir)
	var out []historyStatus
	truncated := false
	for _, s := range readMetricsSegmentDir(msd) {
		if a.metricsSegEnd(s).UnixMilli() <= fromMs {
			continue // wholly older than the window
		}
		if s.retired || s.path == "" {
			// A day the window needs whose raw samples have left the host. The
			// history strip still has it in the rollup, but this rolling read cannot
			// reconstruct per-sample detail for it, so it says the read is short.
			truncated = true
			continue
		}
		samples, err := readCompactSamples(s.path)
		if err != nil {
			return nil, truncated, err
		}
		out = append(out, samples...)
	}
	live, liveTrunc, err := ReadHistoryStatuses(a.metrics.Path(), historyTailBytes)
	if err != nil {
		return nil, truncated, err
	}
	out = append(out, live...)
	return out, truncated || liveTrunc, nil
}

// MetricsSegmentCounters is the metrics window's accounting, for tests and the
// status view.
func (a *Archive) MetricsSegmentCounters() (segments, awaiting, retired int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.metricsSeg.segments, a.metricsSeg.awaiting, a.metricsSeg.retired
}

// ---------------------------------------------------------------- the dir

// metricsSeg is one metrics segment on disk.
type metricsSeg struct {
	name       string
	day        time.Time
	path       string
	bytes      int64
	compressed bool
	retired    bool
}

// end is the segment's end-of-day from its NAME's first day. It is a fallback;
// a.metricsSegEnd prefers the folded last-sample day where the rollup knows it.
func (s metricsSeg) end() time.Time { return s.day.AddDate(0, 0, 1) }

// metricsSegEnd is the instant after which no sample in this segment was written,
// judged by the LAST sample's day when the rollup has folded it, and by the
// name's first day otherwise. RotateTo moves the whole live file into one segment
// named by its FIRST sample's day, so at first enablement the historical
// metrics.jsonl becomes one segment labelled weeks ago; retiring it or excluding
// it from the rolling view on that label would drop recent raw samples it still
// holds. The last-sample day is the honest end.
func (a *Archive) metricsSegEnd(s metricsSeg) time.Time {
	if ms, ok := a.metricsRollup.segmentLastDayMs(s.name); ok {
		return time.UnixMilli(ms).UTC().AddDate(0, 0, 1)
	}
	return s.end()
}

// readMetricsSegmentDir enumerates metrics-segments, oldest first. Both forms
// present means a crash mid-compression: the plain file wins, as the ledger's does.
func readMetricsSegmentDir(msd string) []metricsSeg {
	ents, err := os.ReadDir(msd)
	if err != nil {
		return nil
	}
	type forms struct {
		plain, gz *metricsSeg
		receipt   bool
	}
	byName := map[string]*forms{}
	var names []string
	get := func(n string) *forms {
		f, ok := byName[n]
		if !ok {
			f = &forms{}
			byName[n] = f
			names = append(names, n)
		}
		return f
	}
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() || strings.HasSuffix(name, tmpSuffix) {
			continue
		}
		var size int64
		if info, err := e.Info(); err == nil {
			size = info.Size()
		}
		switch {
		case strings.HasSuffix(name, gzSuffix+receiptSuffix):
			base := strings.TrimSuffix(name, gzSuffix+receiptSuffix)
			if _, ok := parseSegmentName(base); ok {
				get(base).receipt = true
			}
		case strings.HasSuffix(name, gzSuffix):
			base := strings.TrimSuffix(name, gzSuffix)
			if s, ok := parseSegmentName(base); ok {
				seg := metricsSeg{name: base, day: s.FirstDay, path: filepath.Join(msd, name),
					bytes: size, compressed: true}
				get(base).gz = &seg
			}
		case strings.HasSuffix(name, plainSuffix):
			base := strings.TrimSuffix(name, plainSuffix)
			if s, ok := parseSegmentName(base); ok {
				seg := metricsSeg{name: base, day: s.FirstDay, path: filepath.Join(msd, name), bytes: size}
				get(base).plain = &seg
			}
		}
	}
	out := make([]metricsSeg, 0, len(names))
	for _, n := range names {
		f := byName[n]
		switch {
		case f.plain != nil:
			out = append(out, *f.plain)
		case f.gz != nil:
			out = append(out, *f.gz)
		case f.receipt:
			if s, ok := parseSegmentName(n); ok {
				out = append(out, metricsSeg{name: n, day: s.FirstDay, compressed: true, retired: true})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].day.Equal(out[j].day) {
			return out[i].day.Before(out[j].day)
		}
		return out[i].name < out[j].name
	})
	return out
}

func nextMetricsSeq(msd string, day time.Time) int {
	prefix := day.Format(dayLayout)
	seq := 0
	for _, s := range readMetricsSegmentDir(msd) {
		if s.day.Format(dayLayout) != prefix {
			continue
		}
		if ps, ok := parseSegmentName(s.name); ok && ps.Seq >= seq {
			seq = ps.Seq + 1
		}
	}
	return seq
}

// checkColdCopyMetrics is the segment gate pointed at the metrics-segments dir.
func checkColdCopyMetrics(msd, name string) (coldCopyVerdict, string) {
	gz := filepath.Join(msd, name+gzSuffix)
	return verifyColdCopyAt(gz, gz+receiptSuffix, name+gzSuffix)
}

// WriteMetricsReceipt writes a cold-copy receipt for a metrics segment, beside it.
func WriteMetricsReceipt(msd string, r ColdCopyReceipt) error {
	if !strings.HasSuffix(r.Segment, gzSuffix) {
		return fmt.Errorf("archive: a cold-copy receipt names a compressed file, got %q", r.Segment)
	}
	name := strings.TrimSuffix(r.Segment, gzSuffix)
	if _, ok := parseSegmentName(name); !ok {
		return fmt.Errorf("archive: %q is not a metrics segment name", name)
	}
	if r.VerifiedAtMs <= 0 {
		return errors.New("archive: a cold-copy receipt with no verification time does not confirm anything")
	}
	return writeReceiptFile(filepath.Join(msd, name+gzSuffix+receiptSuffix), r)
}

// compressPlainFile turns <path> (a .jsonl) into <path>.gz-form and removes the
// plain file, verifying the round trip first. It is metricsseg's own compressor:
// compressSegment is bound to the ledger's segments dir, and a metrics segment is
// the same job in a different directory.
func compressPlainFile(plainPath string) error {
	if !strings.HasSuffix(plainPath, plainSuffix) {
		return fmt.Errorf("archive: %q is not a plain segment", plainPath)
	}
	gz := strings.TrimSuffix(plainPath, plainSuffix) + gzSuffix
	tmp := gz + tmpSuffix
	want, err := digestOfFile(plainPath)
	if err != nil {
		return err
	}
	src, err := os.Open(plainPath)
	if err != nil {
		return err
	}
	defer src.Close()
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	zw, err := gzip.NewWriterLevel(out, gzip.BestSpeed)
	if err != nil {
		out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if _, err := io.Copy(zw, src); err != nil {
		zw.Close()
		out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := zw.Close(); err != nil {
		out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	got, err := digestOfGz(tmp)
	if err != nil || !want.equal(got) {
		_ = os.Remove(tmp)
		return fmt.Errorf("archive: metrics segment %s round trip FAILED; the plain file is kept",
			filepath.Base(plainPath))
	}
	if err := os.Rename(tmp, gz); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := fsutil.SyncDir(filepath.Dir(gz)); err != nil {
		return err
	}
	if err := os.Remove(plainPath); err != nil {
		return err
	}
	return fsutil.SyncDir(filepath.Dir(plainPath))
}
