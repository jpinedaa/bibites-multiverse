// Package logging builds the one slog logger the sidecar, the relay and the
// archive all use, and bounds what it writes to disk.
//
// WHY THIS PACKAGE EXISTS. Every long-running process here logged to stderr and
// nothing else, so the rig captured it with a shell append redirect. A shell
// redirect has no size: on the night of 2026-08-08 the five sidecars and the
// archive wrote 3.5 GB of slog output into e2e/logs-m4-lan/ and filled the root
// filesystem, which stopped every genome write in the rig mid-rename. Nothing in
// the process could have prevented that, because nothing in the process knew the
// name of the file it was writing to.
//
// So the logger is given the path. --log-file makes the process the owner of its
// own log, and ownership is what makes rotation possible: at RotateBytes the
// writer renames the file aside, shifts the older generations up, drops the
// oldest, and opens a fresh one. Disk use becomes bounded — at most
// RotateBytes x (Keep+1) per process — instead of monotone.
//
// Without --log-file the logger writes to the fallback writer exactly as before,
// which keeps every test and every interactive run unchanged.
package logging

import (
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// Flags registers the disk-budget log flags on fs, identically for every
// process. They are deliberately opt-in: a process with no --log-file keeps
// writing to stderr, so a shell redirect, a test harness and an interactive run
// all behave as they always did.
func Flags(fs *flag.FlagSet) (file *string, rotateMB *int, keep *int) {
	file = fs.String("log-file", env("MULTIVERSE_LOG_FILE", ""),
		"write the log to this file and rotate it by size. Empty logs to stderr, "+
			"which no process can bound — see contract-b-m4.md §12")
	rotateMB = fs.Int("log-rotate-mb", envInt("MULTIVERSE_LOG_ROTATE_MB", 0),
		"size in MiB at which --log-file is rotated (contract-b-m4.md §12, "+
			"logRotateMb). 0 keeps the 100 MiB default; negative disables rotation")
	keep = fs.Int("log-keep", envInt("MULTIVERSE_LOG_KEEP", 0),
		"how many rotated generations to keep beside --log-file (contract-b-m4.md §12, "+
			"logKeep). 0 keeps the default of 5, so the ceiling is "+
			"logRotateMb x (logKeep+1) per process")
	return file, rotateMB, keep
}

func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

func envInt(name string, fallback int) int {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

// DefaultRotateBytes is the size one generation reaches before it is rotated.
const DefaultRotateBytes int64 = 100 << 20

// DefaultKeep is how many rotated generations survive beside the live file.
const DefaultKeep = 5

// Options configures New.
type Options struct {
	// Level is debug, info, warn or error. Anything else means info.
	Level string
	// File is the log path. Empty means "write to the fallback writer", which
	// is the pre-rotation behaviour and the default for tests.
	File string
	// RotateBytes is the size at which File is rotated. 0 means
	// DefaultRotateBytes. Negative disables rotation, which is a deliberate
	// choice an operator has to type.
	RotateBytes int64
	// Keep is how many rotated generations to retain. 0 means DefaultKeep.
	Keep int
}

// New returns the logger and, when it owns a file, the closer that flushes it.
// The closer is never nil; for the fallback-writer case it is a no-op.
func New(fallback io.Writer, o Options) (*slog.Logger, io.Closer, error) {
	if o.File == "" {
		return slog.New(slog.NewTextHandler(fallback, &slog.HandlerOptions{Level: parseLevel(o.Level)})),
			nopCloser{}, nil
	}
	w, err := NewRotatingWriter(o.File, o.RotateBytes, o.Keep)
	if err != nil {
		return nil, nil, err
	}
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: parseLevel(o.Level)})), w, nil
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

type nopCloser struct{}

func (nopCloser) Close() error { return nil }

// RotatingWriter is an io.WriteCloser over a file that never grows past max.
//
// It rotates on the write that WOULD cross the limit, not after, so a generation
// never exceeds max by more than the record that opened it. slog's TextHandler
// emits one record per Write call, so a rotation can never split a log line
// across two files.
//
// The scheme is the conventional numbered one: <path> is live, <path>.1 is the
// most recent rotation, <path>.<keep> is the oldest kept, and anything that
// would become <path>.<keep+1> is deleted. It is deliberately not time-based —
// a size cap is the property the disk actually has.
type RotatingWriter struct {
	mu   sync.Mutex
	path string
	max  int64
	keep int
	f    *os.File
	size int64
}

// NewRotatingWriter opens path for appending, creating its directory if needed.
// A max of 0 means DefaultRotateBytes and a negative max disables rotation; a
// keep of 0 means DefaultKeep.
func NewRotatingWriter(path string, max int64, keep int) (*RotatingWriter, error) {
	if max == 0 {
		max = DefaultRotateBytes
	}
	if keep <= 0 {
		keep = DefaultKeep
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("logging: %w", err)
	}
	w := &RotatingWriter{path: path, max: max, keep: keep}
	if err := w.open(); err != nil {
		return nil, err
	}
	return w, nil
}

// open attaches to the live file and learns its current size. Appending to an
// existing log is what makes a restart continue the same file rather than lose
// what the previous run wrote.
func (w *RotatingWriter) open() error {
	f, err := os.OpenFile(w.path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return fmt.Errorf("logging: %w", err)
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return fmt.Errorf("logging: %w", err)
	}
	w.f = f
	w.size = info.Size()
	return nil
}

func (w *RotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f == nil {
		return 0, os.ErrClosed
	}
	if w.max > 0 && w.size > 0 && w.size+int64(len(p)) > w.max {
		if err := w.rotate(); err != nil {
			// A failed rotation must not lose the line. Keep writing to the
			// file we still hold; the next write tries again.
			n, werr := w.f.Write(p)
			w.size += int64(n)
			if werr != nil {
				return n, werr
			}
			return n, err
		}
	}
	n, err := w.f.Write(p)
	w.size += int64(n)
	return n, err
}

// rotate shifts the generations up and opens a fresh live file. Caller holds
// the lock.
func (w *RotatingWriter) rotate() error {
	if err := w.f.Close(); err != nil {
		return err
	}
	w.f = nil
	_ = os.Remove(w.gen(w.keep))
	for i := w.keep - 1; i >= 1; i-- {
		// Rename is a no-op when the source is absent, which is the ordinary
		// case for a young log; only a real error is worth reporting.
		if err := os.Rename(w.gen(i), w.gen(i+1)); err != nil && !os.IsNotExist(err) {
			_ = w.open()
			return err
		}
	}
	if err := os.Rename(w.path, w.gen(1)); err != nil && !os.IsNotExist(err) {
		_ = w.open()
		return err
	}
	return w.open()
}

func (w *RotatingWriter) gen(n int) string { return fmt.Sprintf("%s.%d", w.path, n) }

// Close flushes and releases the file.
func (w *RotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f == nil {
		return nil
	}
	f := w.f
	w.f = nil
	return f.Close()
}
