package launcher

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// lockStaleAfter is how long a launcher.lock is believed. A start that dies
// between creating the lock and releasing it must not wedge the world forever,
// so an old lock is broken with a warning rather than obeyed. The shape is the
// mod's own save lock: exclusive create, bounded attempts, stale-break.
const lockStaleAfter = 60 * time.Second

// lock is held only while the launcher is starting or stopping one world. It
// is not a lock on the world: the sidecar's journal has its own single-writer
// rule, and the PID ledger is what says whether the world is running.
type lock struct {
	path string
}

// acquireLock takes the per-world lock, breaking one that is older than
// lockStaleAfter. warn is called when a stale lock is broken, because that is
// evidence of an earlier crash and should not vanish.
func acquireLock(path string, now func() time.Time, warn func(format string, args ...any)) (*lock, error) {
	for attempt := 0; attempt < 2; attempt++ {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			_, writeErr := fmt.Fprintf(file, "%d %s\n", os.Getpid(), now().UTC().Format(time.RFC3339))
			closeErr := file.Close()
			if writeErr != nil || closeErr != nil {
				os.Remove(path)
				return nil, fmt.Errorf("could not write %s: %w", path, firstError(writeErr, closeErr))
			}
			return &lock{path: path}, nil
		}
		if !os.IsExist(err) {
			return nil, fmt.Errorf("could not take %s: %w", path, err)
		}
		holder := lockHolder(path)
		if holder > 0 && !processAlive(holder) {
			// The lock names its holder, and that holder is gone. Waiting out
			// the full stale window for a launcher that was killed mid-start
			// wedges the world for no reason.
			warn("breaking the lock at %s; the launcher that took it (pid %d) is gone", path, holder)
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return nil, fmt.Errorf("could not break the lock at %s: %w", path, err)
			}
			continue
		}
		age, known := lockAge(path, now)
		if known && age < lockStaleAfter {
			return nil, fmt.Errorf("another launcher is starting or stopping this world (%s was taken "+
				"%s ago by pid %d). Wait for it to finish", path, age.Truncate(time.Second), holder)
		}
		warn("breaking a stale lock at %s; the launcher that took it did not finish", path)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("could not break the stale lock at %s: %w", path, err)
		}
	}
	return nil, fmt.Errorf("could not take %s; something is creating it faster than it can be broken", path)
}

// release drops the lock. It is safe to call twice.
func (l *lock) release() {
	if l == nil {
		return
	}
	os.Remove(l.path)
}

// lockAge reads the timestamp the lock holder wrote. A lock whose content
// cannot be read falls back to the file's own modification time, so a truncated
// lock is still bounded rather than eternal.
func lockAge(path string, now func() time.Time) (time.Duration, bool) {
	if raw, err := os.ReadFile(path); err == nil {
		fields := strings.Fields(string(raw))
		if len(fields) >= 2 {
			if taken, err := time.Parse(time.RFC3339, fields[1]); err == nil {
				return now().Sub(taken), true
			}
		}
	}
	if info, err := os.Stat(path); err == nil {
		return now().Sub(info.ModTime()), true
	}
	return 0, false
}

// lockHolder reads the pid the lock names, or 0 when it names none.
func lockHolder(path string) int {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(raw))
	if len(fields) == 0 {
		return 0
	}
	pid, err := strconv.Atoi(fields[0])
	if err != nil || pid <= 0 {
		return 0
	}
	return pid
}

func firstError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
