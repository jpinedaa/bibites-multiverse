//go:build !windows

package sidecar

// The two host facts `--diagnose` needs and the standard library does not
// expose: how much room is left on a volume, and whether a recorded process id
// names a process that is still there.

import (
	"os"
	"syscall"
)

// freeBytes is the space available to this user on the filesystem holding path.
//
// It is Bavail and not Bfree: the reserved blocks a filesystem keeps for root
// are not room this sidecar will ever get, and a headroom check that counted
// them would pass a disk that is already full for the process doing the writing.
func freeBytes(path string) (int64, bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, false
	}
	return int64(st.Bavail) * int64(st.Bsize), true
}

// processAlive reports whether pid names a live process.
//
// Signal 0 DELIVERS NOTHING. It is the existence-and-permission probe POSIX
// defines for exactly this question, and it is the only one available without
// reading a platform-specific procfs. Nothing is signalled, nothing is
// interrupted, and a diagnostic that promises to disturb no running session
// keeps that promise here.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}
