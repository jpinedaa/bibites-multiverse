package logging

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestNoFileMeansStderrAsBefore(t *testing.T) {
	var buf bytes.Buffer
	log, closer, err := New(&buf, Options{Level: "info"})
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Close()
	log.Info("hello", "n", 1)
	log.Debug("not at info level")
	if !strings.Contains(buf.String(), "hello") {
		t.Fatalf("the fallback writer got %q", buf.String())
	}
	if strings.Contains(buf.String(), "not at info level") {
		t.Fatal("the level was not applied")
	}
}

// TestRotationBoundsTheFile is the property the ENOSPC of 2026-08-08 needed and
// did not have: a bound on what one process writes to disk.
func TestRotationBoundsTheFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sidecar.log")
	w, err := NewRotatingWriter(path, 1<<10, 3)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	line := bytes.Repeat([]byte("x"), 200)
	line = append(line, '\n')
	for i := 0; i < 200; i++ {
		if _, err := w.Write(line); err != nil {
			t.Fatal(err)
		}
	}

	// keep+1 files at most, none of them over the cap by more than one record.
	names, err := filepath.Glob(path + "*")
	if err != nil {
		t.Fatal(err)
	}
	if len(names) > 4 {
		t.Fatalf("rotation kept %d files, want at most 4: %v", len(names), names)
	}
	var total int64
	for _, n := range names {
		info, err := os.Stat(n)
		if err != nil {
			t.Fatal(err)
		}
		if info.Size() > 1<<10+int64(len(line)) {
			t.Fatalf("%s is %d bytes, over the 1 KiB cap", n, info.Size())
		}
		total += info.Size()
	}
	if written := int64(200 * len(line)); total >= written {
		t.Fatalf("nothing was ever dropped: %d bytes on disk for %d written", total, written)
	}
	// The oldest generation must be gone, not merely renamed forever.
	if _, err := os.Stat(path + ".4"); !os.IsNotExist(err) {
		t.Fatalf("generation 4 exists past keep=3: %v", err)
	}
}

// TestRotationNeverSplitsARecord matters because a log line is the unit an
// operator greps for. slog hands the writer one whole record per call, so a
// rotation must fall between two of them.
func TestRotationNeverSplitsARecord(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "relay.log")
	log, closer, err := New(nil, Options{Level: "info", File: path, RotateBytes: 2048, Keep: 5})
	if err != nil {
		t.Fatal(err)
	}
	const records = 300
	for i := 0; i < records; i++ {
		log.Info("crossing", "seq", i, "pad", strings.Repeat("y", 40))
	}
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}

	names, err := filepath.Glob(path + "*")
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, n := range names {
		b, err := os.ReadFile(n)
		if err != nil {
			t.Fatal(err)
		}
		if len(b) == 0 {
			continue
		}
		if b[len(b)-1] != '\n' {
			t.Fatalf("%s does not end on a record boundary", n)
		}
		for _, ln := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
			if !strings.Contains(ln, "msg=crossing") || !strings.Contains(ln, "seq=") {
				t.Fatalf("%s holds a torn record: %q", n, ln)
			}
			seen++
		}
	}
	if seen == 0 {
		t.Fatal("no records survived at all")
	}
}

// TestRestartAppendsRatherThanTruncates: a rolling restart must not throw away
// what the previous process wrote.
func TestRestartAppendsRatherThanTruncates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "archive.log")
	first, err := NewRotatingWriter(path, 1<<20, 5)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Write([]byte("before the restart\n")); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := NewRotatingWriter(path, 1<<20, 5)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if _, err := second.Write([]byte("after the restart\n")); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(b); got != "before the restart\nafter the restart\n" {
		t.Fatalf("the restart did not append: %q", got)
	}
}

// TestOversizedLogIsRotatedOnFirstWrite is the state the fix lands in: the rig's
// live logs are already hundreds of megabytes when the new binary starts.
func TestOversizedLogIsRotatedOnFirstWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sidecar-3.log")
	if err := os.WriteFile(path, bytes.Repeat([]byte("old\n"), 1000), 0o644); err != nil {
		t.Fatal(err)
	}
	w, err := NewRotatingWriter(path, 512, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if _, err := w.Write([]byte("new\n")); err != nil {
		t.Fatal(err)
	}
	live, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(live) != "new\n" {
		t.Fatalf("the oversized log was not rotated aside: %q", live)
	}
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Fatalf("the old content was not preserved as generation 1: %v", err)
	}
}

func TestConcurrentWritersDoNotInterleave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "concurrent.log")
	w, err := NewRotatingWriter(path, 4096, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				line := fmt.Sprintf("goroutine=%d seq=%d %s\n", g, i, strings.Repeat("z", 30))
				if _, err := w.Write([]byte(line)); err != nil {
					t.Errorf("write: %v", err)
					return
				}
			}
		}(g)
	}
	wg.Wait()

	names, err := filepath.Glob(path + "*")
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range names {
		b, err := os.ReadFile(n)
		if err != nil {
			t.Fatal(err)
		}
		for _, ln := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
			if ln == "" {
				continue
			}
			if !strings.HasPrefix(ln, "goroutine=") || !strings.HasSuffix(ln, strings.Repeat("z", 30)) {
				t.Fatalf("%s holds an interleaved line: %q", n, ln)
			}
		}
	}
}

func TestNegativeRotateBytesDisablesRotation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "unbounded.log")
	w, err := NewRotatingWriter(path, -1, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	for i := 0; i < 100; i++ {
		if _, err := w.Write(bytes.Repeat([]byte("q"), 100)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(path + ".1"); !os.IsNotExist(err) {
		t.Fatal("rotation happened although it was disabled")
	}
}
