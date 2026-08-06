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
)

const metricsName = "metrics.jsonl"

// MetricsLog is the append-only sample file.
type MetricsLog struct {
	mu   sync.Mutex
	path string
	f    *os.File
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
	return &MetricsLog{path: path, f: f}, nil
}

// Path is the sample file.
func (m *MetricsLog) Path() string { return m.path }

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
	return m.f.Sync()
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
