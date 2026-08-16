package launcher

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestLockFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), lockFileName)
	clock := time.Date(2026, 8, 16, 9, 41, 7, 0, time.UTC)
	now := func() time.Time { return clock }
	var warnings []string
	warn := func(format string, args ...any) { warnings = append(warnings, format) }

	held, err := acquireLock(path, now, warn)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	content := readFile(t, path)
	fields := strings.Fields(content)
	if len(fields) != 2 {
		t.Fatalf("the lock holds %q, want '<pid> <timestamp>'", content)
	}
	if pid, err := strconv.Atoi(fields[0]); err != nil || pid != os.Getpid() {
		t.Fatalf("the lock names pid %q, want %d", fields[0], os.Getpid())
	}
	if _, err := time.Parse(time.RFC3339, fields[1]); err != nil {
		t.Fatalf("the lock's timestamp is %q: %v", fields[1], err)
	}

	// A live lock is obeyed.
	if _, err := acquireLock(path, now, warn); err == nil {
		t.Fatal("a lock taken a moment ago was broken")
	} else {
		mustContain(t, "the refusal", err.Error(), "another launcher is starting or stopping")
	}
	if len(warnings) != 0 {
		t.Fatalf("a live lock warned: %v", warnings)
	}

	// One second before the deadline it is still obeyed.
	clock = clock.Add(lockStaleAfter - time.Second)
	if _, err := acquireLock(path, now, warn); err == nil {
		t.Fatal("a lock younger than the stale window was broken")
	}

	// Past it, it is broken - loudly, because it is evidence of a crash.
	clock = clock.Add(2 * time.Second)
	second, err := acquireLock(path, now, warn)
	if err != nil {
		t.Fatalf("a stale lock was not broken: %v", err)
	}
	if len(warnings) != 1 {
		t.Fatalf("breaking a stale lock warned %d times, want 1", len(warnings))
	}
	second.release()
	if fileExists(path) {
		t.Fatal("release left the lock behind")
	}
	// Releasing twice, and releasing a lock that was never taken, are both safe.
	second.release()
	held.release()

	// A lock with no readable timestamp falls back to the file's own age, so it
	// is still bounded rather than eternal.
	writeFile(t, path, "garbage\n", 0o644)
	old := clock.Add(-2 * lockStaleAfter)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	third, err := acquireLock(path, now, warn)
	if err != nil {
		t.Fatalf("an old unreadable lock was not broken: %v", err)
	}
	third.release()
}

// TestLockBrokenWhenItsHolderIsGone: the lock names its holder, so a launcher
// killed mid-start must not wedge the world for the full stale window.
func TestLockBrokenWhenItsHolderIsGone(t *testing.T) {
	path := filepath.Join(t.TempDir(), lockFileName)
	clock := time.Date(2026, 8, 16, 9, 41, 7, 0, time.UTC)
	now := func() time.Time { return clock }
	var warnings []string
	warn := func(format string, args ...any) { warnings = append(warnings, fmt.Sprintf(format, args...)) }

	// A pid nothing can be running under, with a timestamp from a second ago.
	dead := findDeadPid(t)
	writeFile(t, path, fmt.Sprintf("%d %s\n", dead, clock.Add(-time.Second).Format(time.RFC3339)), 0o644)

	held, err := acquireLock(path, now, warn)
	if err != nil {
		t.Fatalf("a lock whose holder is gone was obeyed: %v", err)
	}
	held.release()
	if len(warnings) != 1 || !strings.Contains(warnings[0], "is gone") {
		t.Fatalf("breaking it did not say why: %v", warnings)
	}

	// A lock held by a process that IS alive - this one - is still obeyed.
	writeFile(t, path, fmt.Sprintf("%d %s\n", os.Getpid(), clock.Format(time.RFC3339)), 0o644)
	if _, err := acquireLock(path, now, warn); err == nil {
		t.Fatal("a lock held by a live launcher was broken")
	}
}

// findDeadPid returns a pid number nothing is using.
func findDeadPid(t *testing.T) int {
	t.Helper()
	for candidate := 4194300; candidate > 4194000; candidate-- {
		if !processAlive(candidate) {
			return candidate
		}
	}
	t.Fatal("could not find a pid nothing is using")
	return 0
}
