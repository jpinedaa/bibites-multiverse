package archive

// The metrics window (metricsseg.go) and its rollup (metricshist.go): the
// retirement gate cloned for metrics segments, the rollup-equivalence property
// that lets a day retire without the history losing it, that the history survives
// the retirement, and that LastSample reads a bounded tail.

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func metricsArchive(t *testing.T, dir string, window time.Duration) *Archive {
	t.Helper()
	a, err := New(Config{
		DataDir: dir, PeerID: "archive-test", RelayURL: "ws://test",
		MetricsWindow:              window,
		MetricsMaintenanceInterval: -1,
		Logger: slog.New(slog.NewTextHandler(io.Discard,
			&slog.HandlerOptions{Level: slog.LevelError})),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })
	return a
}

// sampleAt builds one compact history sample, the shape the rollup folds.
func sampleAt(genMs int64, pop, migrations int) historyStatus {
	p := pop
	tp := pop
	return historyStatus{
		GeneratedAtMs: genMs,
		Slots: []historySlotStatus{
			{Slot: 1, PeerID: "peer-1", Live: true, Population: &p},
		},
		Totals: historyTotalsStatus{Population: &tp, Migrations: migrations},
	}
}

// TestMetricsRollupFoldEquivalence is the property the whole window rests on:
// folding a run DAY BY DAY and folding it ALL AT ONCE produce the SAME buckets.
// It is why a day's raw segment can retire the moment it is folded — the aggregate
// does not depend on the split.
func TestMetricsRollupFoldEquivalence(t *testing.T) {
	base := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC).UnixMilli()
	var all []historyStatus
	mig := 0
	for i := 0; i < 200; i++ {
		mig += i % 3 // a cumulative counter that sometimes does not move
		all = append(all, sampleAt(base+int64(i)*15*60*1000, 10+i, mig))
	}
	// Split at a point that is NOT a base-bucket boundary, to prove the fold is
	// additive across an arbitrary cut.
	cut := 97
	older, newer := all[:cut], all[cut:]

	split := openRollupForTest(t)
	if err := split.foldSegment("2026-08-10-0000", older); err != nil {
		t.Fatal(err)
	}
	if err := split.foldSegment("2026-08-11-0000", newer); err != nil {
		t.Fatal(err)
	}

	whole := openRollupForTest(t)
	if err := whole.foldSegment("all-0000", all); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(split.baseBucketsForTest(), whole.baseBucketsForTest()) {
		t.Fatal("the per-segment fold does not equal the all-at-once fold: the rollup is not additive")
	}
	// A segment folded twice is folded ONCE: the name is the idempotence key, which
	// is what stops a restart double-counting a day.
	before := whole.baseBucketsForTest()
	if err := whole.foldSegment("all-0000", all); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, whole.baseBucketsForTest()) {
		t.Fatal("folding the same segment twice changed the buckets")
	}
}

func openRollupForTest(t *testing.T) *metricsRollup {
	t.Helper()
	mr, err := openMetricsRollup(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return mr
}

// TestARollupSurvivesAReopen: the fold is persisted, so a restart resumes the
// same buckets without re-reading a raw segment.
func TestARollupSurvivesAReopen(t *testing.T) {
	dir := t.TempDir()
	mr, err := openMetricsRollup(dir)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC).UnixMilli()
	var s []historyStatus
	for i := 0; i < 50; i++ {
		s = append(s, sampleAt(base+int64(i)*15*60*1000, 5+i, i))
	}
	if err := mr.foldSegment("2026-08-10-0000", s); err != nil {
		t.Fatal(err)
	}
	want := mr.baseBucketsForTest()

	reopened, err := openMetricsRollup(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(want, reopened.baseBucketsForTest()) {
		t.Fatal("the rollup did not survive a reopen")
	}
	if !reopened.alreadyFolded("2026-08-10-0000") {
		t.Fatal("a folded segment is not remembered across a reopen; it would be double-counted")
	}
}

// writeMetricsSegmentFile writes a plain metrics segment for a past day.
func writeMetricsSegmentFile(t *testing.T, dir, name string, samples []historyStatus) {
	t.Helper()
	msd := metricsSegmentsDir(dir)
	if err := os.MkdirAll(msd, 0o755); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, s := range samples {
		pop := 0
		if len(s.Slots) > 0 && s.Slots[0].Population != nil {
			pop = *s.Slots[0].Population
		}
		b.WriteString(fmt.Sprintf(
			`{"generatedAtMs":%d,"slots":[{"slot":1,"peerId":"peer-1","live":true,"population":%d}],"totals":{"population":%d,"migrations":%d}}`+"\n",
			s.GeneratedAtMs, pop, pop, s.Totals.Migrations))
	}
	if err := os.WriteFile(filepath.Join(msd, name+plainSuffix), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeGoodMetricsReceipt(t *testing.T, dir, name string) {
	t.Helper()
	msd := metricsSegmentsDir(dir)
	gz := filepath.Join(msd, name+gzSuffix)
	info, err := os.Stat(gz)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	if err := WriteMetricsReceipt(msd, ColdCopyReceipt{
		Segment: name + gzSuffix, Bytes: info.Size(), SHA256: fileSHA(t, gz),
		Destination: "s3://b/metrics/" + name + gzSuffix, RemoteChecksum: "x", RemoteChecksumKind: "etag",
		UploadedAtMs: now, VerifiedAtMs: now,
	}); err != nil {
		t.Fatal(err)
	}
}

// TestAMetricsSegmentRetiresOnlyBehindAReceiptAndTheHistorySurvives is the
// metrics half of the ledger's retirement-gate suite, plus the property that
// makes it safe: the day is folded into the rollup before its raw segment can go,
// so the history strip keeps it.
func TestAMetricsSegmentRetiresOnlyBehindAReceiptAndTheHistorySurvives(t *testing.T) {
	dir := t.TempDir()
	a := metricsArchive(t, dir, 48*time.Hour)

	day := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -5)
	name := segmentName(day, 0)
	var samples []historyStatus
	for i := 0; i < 20; i++ {
		samples = append(samples, sampleAt(day.Add(time.Duration(i)*time.Hour).UnixMilli(), 100+i, i))
	}
	writeMetricsSegmentFile(t, dir, name, samples)

	// Pass one: fold + compress, but NO receipt so nothing retires.
	a.MetricsMaintenanceNow(time.Now())
	gz := filepath.Join(metricsSegmentsDir(dir), name+gzSuffix)
	if !exists(gz) {
		t.Fatal("the metrics segment was not compressed")
	}
	if !a.metricsRollup.alreadyFolded(name) {
		t.Fatal("the metrics segment was not folded into the rollup")
	}
	segments, awaiting, retired := a.MetricsSegmentCounters()
	if retired != 0 || awaiting != 1 || segments != 1 {
		t.Fatalf("no-receipt: segments %d awaiting %d retired %d, want 1 1 0", segments, awaiting, retired)
	}

	// The history already survives even before retirement: it comes from the fold.
	h, err := a.HistoryAllView(120)
	if err != nil {
		t.Fatal(err)
	}
	if h.Samples == 0 {
		t.Fatal("the folded day is not in the all-record history")
	}

	// Pass two, with a receipt: the raw segment retires and the history is unchanged.
	writeGoodMetricsReceipt(t, dir, name)
	a.MetricsMaintenanceNow(time.Now())
	if exists(gz) {
		t.Fatal("a confirmed metrics segment was not retired")
	}
	if _, _, retired := a.MetricsSegmentCounters(); retired != 1 {
		t.Fatalf("metrics retired = %d, want 1", retired)
	}
	// Force the cache to rebuild and confirm the retired day still shows.
	a.historyAllAt = time.Time{}
	h2, err := a.HistoryAllView(120)
	if err != nil {
		t.Fatal(err)
	}
	if h2.Samples == 0 {
		t.Fatal("the history lost the retired day; the rollup did not preserve it")
	}
}

// TestNoMetricsWindowKeepsTheFileWhole is the off-by-default proof: with no
// window, the live file is never rotated and the all-record view scans it as it
// always did.
func TestNoMetricsWindowKeepsTheFileWhole(t *testing.T) {
	dir := t.TempDir()
	a := metricsArchive(t, dir, 0)

	a.MetricsMaintenanceNow(time.Now())
	if len(readMetricsSegmentDir(metricsSegmentsDir(dir))) != 0 {
		t.Fatal("an archive with no metrics window rotated a segment")
	}
	b, err := json.Marshal(a.StatusView())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "metricsWindowMs") {
		t.Fatalf("metricsWindowMs is on the status of an archive with no window:\n%s", b)
	}
	if strings.Contains(string(b), "metricsSegmentsAwaitingColdCopy") {
		t.Fatalf("metricsSegmentsAwaitingColdCopy is on a no-window archive:\n%s", b)
	}
}

// TestLastSampleReadsABoundedTail: LastSample returns the newest sample without
// reading the whole file, and ReadMetricsTail bounds the read.
func TestLastSampleReadsABoundedTail(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "metrics.jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	var lastMs int64
	for i := 0; i < 500; i++ {
		lastMs = int64(1_700_000_000_000 + i)
		fmt.Fprintf(f, `{"generatedAtMs":%d,"totals":{"population":%d,"migrations":%d}}`+"\n",
			lastMs, i, i)
	}
	f.Close()

	s, err := LastSample(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.GeneratedAtMs != lastMs {
		t.Fatalf("LastSample returned generatedAtMs %d, want the newest %d", s.GeneratedAtMs, lastMs)
	}
	// A tiny tail still yields the last complete line and reports it was truncated.
	tail, truncated, err := ReadMetricsTail(path, 200)
	if err != nil {
		t.Fatal(err)
	}
	if !truncated {
		t.Fatal("a 200-byte read of a many-KB file did not report truncation")
	}
	if len(tail) == 0 || tail[len(tail)-1].GeneratedAtMs != lastMs {
		t.Fatal("the bounded tail did not contain the newest sample")
	}
}
