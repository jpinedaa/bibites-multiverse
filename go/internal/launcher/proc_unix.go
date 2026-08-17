//go:build !windows

package launcher

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// The platform-dependent names. The Linux kit of this release ships no
// launcher; these exist so the whole program builds, vets and tests here.
const (
	sidecarExeName = "multiverse-sidecar"
	gameExeName    = "The Bibites.x86_64"
)

// noWindowAttrs is the Windows notion of a console program with no console.
// There is nothing to say here: a child whose streams are redirected shows
// nobody anything on this platform.
func noWindowAttrs() *syscall.SysProcAttr { return nil }

// detachedAttrs puts the child in its own session, so it survives the terminal
// the launcher was started from.
func detachedAttrs() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}

// protectUserFile makes path readable by this account only.
func protectUserFile(path string) error {
	return os.Chmod(path, 0o600)
}

// gracefulStop asks the process to close, so a world's save-on-quit runs. There
// is no headless gap here: SIGTERM reaches a process with no window exactly as
// it reaches one with a window, which is why LOCAL-HEADLESSSTOP is a Windows
// entry.
func gracefulStop(pid int) (askResult, error) {
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		return askFailed, err
	}
	return askAccepted, nil
}

// forceStop is the last resort: SIGKILL, so whatever the process was writing is
// lost — which is exactly why gracefulStop is tried first.
func forceStop(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	defer process.Release()
	return process.Kill()
}

// probeProcess answers what the operating system knows about pid. EPERM means
// the process exists and belongs to somebody else, which is not "gone".
func probeProcess(pid int) processState {
	if pid <= 0 {
		return processDead
	}
	err := syscall.Kill(pid, 0)
	switch {
	case err == nil:
		return processRunning
	case errors.Is(err, syscall.EPERM):
		return processOpaque
	default:
		return processDead
	}
}

// processImagePath names the executable a running process is currently in.
// /proc is the exact answer; ps is the fallback for a kernel without it, and
// its comm is truncated to 15 characters, which imageMatches allows for.
func processImagePath(pid int) (string, error) {
	if pid <= 0 {
		return "", fmt.Errorf("pid %d is not a process", pid)
	}
	if path, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid)); err == nil && path != "" {
		// A running program whose file was replaced or removed is reported with
		// this suffix; the path before it is still the identity.
		return strings.TrimSuffix(path, " (deleted)"), nil
	}
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output()
	if err != nil {
		return "", err
	}
	name := strings.TrimSpace(string(out))
	if name == "" {
		return "", fmt.Errorf("nothing named the program behind pid %d", pid)
	}
	return name, nil
}

// openFolder shows the user a directory.
func openFolder(path string) error {
	return exec.Command("xdg-open", path).Start()
}

// revealFile shows the folder holding one file. There is no portable "select
// this file" here, and the folder is the useful half.
func revealFile(path string) error {
	return openFolder(filepath.Dir(path))
}
