package launcher

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// launcherLogRotateBytes bounds the launcher's own log. A log nothing rotates
// grows without limit, and a full disk here stops every journal write on the
// machine — the failure this rig has actually had (internal/logging).
const launcherLogRotateBytes = 1 << 20

// eventLog is the launcher's append-only record of what it did to one world:
// one line per event, <RFC3339 UTC> <level> <event> key=value ...
//
// NO SECRET IS EVER WRITTEN HERE. The values that reach it are paths, ports,
// pids and world names.
type eventLog struct {
	path string
	now  func() time.Time
}

func newEventLog(path string, now func() time.Time) *eventLog {
	return &eventLog{path: path, now: now}
}

// event appends one line. A logging failure never fails a command: the world
// starting matters more than the record of it starting.
func (e *eventLog) event(level, name string, pairs ...string) {
	if e == nil || e.path == "" {
		return
	}
	e.rotateIfLarge()
	var line strings.Builder
	line.WriteString(e.now().UTC().Format(time.RFC3339))
	line.WriteByte(' ')
	line.WriteString(level)
	line.WriteByte(' ')
	line.WriteString(name)
	for i := 0; i+1 < len(pairs); i += 2 {
		fmt.Fprintf(&line, " %s=%s", pairs[i], quoteValue(pairs[i+1]))
	}
	line.WriteByte('\n')

	file, err := os.OpenFile(e.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer file.Close()
	file.WriteString(line.String())
}

// rotateIfLarge renames a full log aside, replacing the previous generation, so
// the ceiling is two files.
func (e *eventLog) rotateIfLarge() {
	info, err := os.Stat(e.path)
	if err != nil || info.Size() < launcherLogRotateBytes {
		return
	}
	os.Remove(e.path + ".1")
	os.Rename(e.path, e.path+".1")
}

// quoteValue keeps one event on one line and keeps key=value parseable.
func quoteValue(value string) string {
	if value == "" {
		return `""`
	}
	if strings.ContainsAny(value, " \t\r\n\"") {
		return fmt.Sprintf("%q", value)
	}
	return value
}
