package archive

// Durable metrics: periodic PEER_STATUS samples, appended to
// <data-dir>/metrics.jsonl, so HISTORY SURVIVES EVERYTHING.
//
// The status page is live and forgets; a browser tab that was not open when a
// slot went dark shows nothing about it afterwards. Risk 6 is the same lesson
// from the other side — BepInEx overwrites its log at each launch, and T1 had
// to harvest 147 MB by hand hours before the next start. A sample file is the
// cheapest answer: one JSON object per line, fsynced, never rewritten, readable
// with tail and jq on either machine.
//
// It records exactly the view the page renders, so an operator comparing "what
// the page said at 04:12" with the file is comparing the same numbers, unknowns
// included.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const metricsName = "metrics.jsonl"

// MetricsLog is the append-only sample file.
//
// It carries the UTC day of its FIRST sample so the metrics window can name a
// rotated segment by the day it holds (metricsseg.go). The live file is never a
// segment itself: it stays for tail reads, and rotation closes it into a dated
// segment and reopens an empty one.
type MetricsLog struct {
	mu   sync.Mutex
	path string
	f    *os.File
	// firstMs is the GeneratedAtMs of the first sample in the live file, 0 when it
	// is empty. It is read from the file at open and set on the first Append.
	firstMs int64
}

// OpenMetrics opens or creates <dir>/metrics.jsonl for appending.
func OpenMetrics(dir string) (*MetricsLog, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, metricsName)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	m := &MetricsLog{path: path, f: f}
	m.firstMs = firstSampleMs(path)
	return m, nil
}

// Path is the sample file.
func (m *MetricsLog) Path() string { return m.path }

// FirstDayMs is the UTC midnight of the live file's first sample, or 0 when it is
// empty. It is what the rotation decides "is this file's day over" on.
func (m *MetricsLog) FirstDayMs() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.firstMs == 0 {
		return 0
	}
	return time.UnixMilli(m.firstMs).UTC().Truncate(24 * time.Hour).UnixMilli()
}

// Size is the live file's current byte size.
func (m *MetricsLog) Size() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.f == nil {
		return 0
	}
	info, err := m.f.Stat()
	if err != nil {
		return 0
	}
	return info.Size()
}

// RotateTo closes the live file, renames it to dest, and reopens an empty live
// file. It returns false with no error when the live file is empty — there is
// nothing to rotate. The caller has already ensured dest's directory exists.
func (m *MetricsLog) RotateTo(dest string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.f == nil {
		return false, os.ErrClosed
	}
	info, err := m.f.Stat()
	if err != nil {
		return false, err
	}
	if info.Size() == 0 {
		return false, nil
	}
	if err := m.f.Sync(); err != nil {
		return false, err
	}
	if err := m.f.Close(); err != nil {
		m.f = nil
		return false, err
	}
	if err := os.Rename(m.path, dest); err != nil {
		// Reopen the live file so the archive keeps sampling even if the rename
		// failed; the day will be retried on the next pass.
		m.f, _ = os.OpenFile(m.path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
		return false, err
	}
	f, err := os.OpenFile(m.path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return true, err
	}
	m.f = f
	m.firstMs = 0
	return true, nil
}

// Append writes one sample and flushes it before returning.
func (m *MetricsLog) Append(view Status) error {
	b, err := json.Marshal(view)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.f == nil {
		return os.ErrClosed
	}
	if _, err := m.f.Write(append(b, '\n')); err != nil {
		return err
	}
	if m.firstMs == 0 && view.GeneratedAtMs > 0 {
		m.firstMs = view.GeneratedAtMs
	}
	return m.f.Sync()
}

// firstSampleMs reads the GeneratedAtMs of the first parseable line of a metrics
// file, or 0.
func firstSampleMs(path string) int64 {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	var s Status
	if dec.Decode(&s) == nil {
		return s.GeneratedAtMs
	}
	return 0
}

// Close flushes and closes the sample file.
func (m *MetricsLog) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.f == nil {
		return nil
	}
	if err := m.f.Sync(); err != nil {
		m.f.Close()
		m.f = nil
		return err
	}
	err := m.f.Close()
	m.f = nil
	return err
}

// ReadMetrics replays every sample in dir's metrics file, in write order. A
// torn final line is dropped, exactly as the ledger and the journal drop one.
func ReadMetrics(path string) ([]Status, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Status
	start := 0
	for i := 0; i < len(b); i++ {
		if b[i] != '\n' {
			continue
		}
		var s Status
		if json.Unmarshal(b[start:i], &s) == nil {
			out = append(out, s)
		}
		start = i + 1
	}
	return out, nil
}
