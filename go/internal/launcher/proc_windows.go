//go:build windows

package launcher

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

// The platform-dependent names. On Windows the kit ships the two binaries with
// their .exe suffix and the game is "The Bibites.exe".
const (
	sidecarExeName = "multiverse-sidecar.exe"
	gameExeName    = "The Bibites.exe"
)

const (
	// detachedProcess is DETACHED_PROCESS. syscall does not export it, and it is
	// not cosmetic: without it a spawned child inherits this console and gets
	// CTRL_CLOSE_EVENT when the launcher window closes, which would take the
	// sidecar and the game down with it. Start-Process -WindowStyle Hidden gave
	// the PowerShell script the same isolation for free. DETACHED_PROCESS and
	// CREATE_NO_WINDOW are mutually exclusive; do not add the second one.
	detachedProcess = 0x00000008

	// Liveness is answered from the process object itself, which is signaled
	// exactly when the process has terminated. os.Process.Signal cannot answer
	// it on Windows: for anything but Kill it returns EWINDOWS whether the
	// process is alive or not.
	synchronize                    = 0x00100000
	processQueryLimitedInformation = 0x00001000
	waitObject0                    = 0

	// OpenProcess tells the two failures apart, and the difference matters: a
	// pid this account may not open is a pid that EXISTS. Reading every failure
	// as "gone" makes the launcher orphan an elevated world and then delete the
	// only record of it.
	errorInvalidParameter = 87
	errorAccessDenied     = 5

	// maxLongPath is the buffer QueryFullProcessImageNameW is given. Windows
	// paths can exceed MAX_PATH when long-path support is on.
	maxLongPath = 32768
)

var (
	kernel32                       = syscall.NewLazyDLL("kernel32.dll")
	procQueryFullProcessImageNameW = kernel32.NewProc("QueryFullProcessImageNameW")
)

// detachedAttrs makes a spawned child outlive this console.
func detachedAttrs() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | detachedProcess,
	}
}

// protectUserFile makes path readable by this account only. It mirrors
// Protect-UserFile in Install-BibitesMultiverse.ps1: drop inheritance, grant
// this user, grant nobody else.
//
// The grant is (F), not (R,W): the installer grants FullControl, and the
// launcher DELETES these files (the pending enrollment record, and the whole
// data root under --remove-world-data). (R,W) omits DELETE, so under a data
// root whose parent does not grant FILE_DELETE_CHILD those removals fail.
func protectUserFile(path string) error {
	if err := os.Chmod(path, 0o600); err != nil {
		return err
	}
	current, err := user.Current()
	if err != nil {
		return fmt.Errorf("could not read this Windows account's name: %w", err)
	}
	cmd := exec.Command(systemTool("icacls.exe"), path,
		"/inheritance:r", "/grant:r", current.Username+":(F)", "/Q")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("icacls could not protect %s: %w: %s", path, err, string(out))
	}
	return nil
}

// gracefulStop asks the process to close, so a world's save-on-quit runs.
// taskkill without /F posts WM_CLOSE — see taskkillGracefulArgs for why the /T
// that used to be here made that ask fail on EVERY game, windowed or not.
//
// A headless game (-batchmode -nographics) has no window, so taskkill answers
// "can only be terminated forcefully". That is askImpossible rather than a
// failure: the caller forces at once instead of waiting out the timeout.
func gracefulStop(pid int) (askResult, error) {
	cmd := exec.Command(systemTool("taskkill.exe"), taskkillGracefulArgs(pid)...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return askAccepted, nil
	}
	return classifyTaskkill(err, string(out)),
		fmt.Errorf("taskkill could not ask pid %d to close: %w: %s", pid, err,
			strings.TrimSpace(string(out)))
}

// forceStop is the last resort on Windows: taskkill /T /F, which is
// TerminateProcess on the pid AND on its children, so whatever the process was
// writing is lost — which is exactly why gracefulStop is tried first.
//
// THE TREE IS THE POINT HERE, for one child in particular: the game always
// spawns UnityCrashHandler64.exe --attach <game pid>. That handler waits on its
// parent and exits with it, so a game that CLOSES needs nothing done about it;
// a game that is killed is the case where /T is what keeps the handler from
// lingering with a dead process to attach to.
func forceStop(pid int) error {
	cmd := exec.Command(systemTool("taskkill.exe"), taskkillForceArgs(pid)...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	// It may simply have gone in the meantime, which is the outcome asked for.
	if probeProcess(pid) == processDead {
		return nil
	}
	// taskkill is a program, and a machine that cannot run it still has to be
	// able to stop a world: fall back to TerminateProcess on the pid itself.
	process, findErr := os.FindProcess(pid)
	if findErr != nil {
		return fmt.Errorf("taskkill could not stop pid %d (%w: %s), and the process could not "+
			"be opened: %v", pid, err, strings.TrimSpace(string(out)), findErr)
	}
	defer process.Release()
	if killErr := process.Kill(); killErr != nil {
		return fmt.Errorf("taskkill could not stop pid %d (%w: %s), and terminating it directly "+
			"failed: %v", pid, err, strings.TrimSpace(string(out)), killErr)
	}
	return nil
}

// probeProcess answers what the operating system knows about pid.
func probeProcess(pid int) processState {
	if pid <= 0 {
		return processDead
	}
	handle, err := syscall.OpenProcess(synchronize|processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		if errors.Is(err, syscall.Errno(errorInvalidParameter)) {
			return processDead
		}
		// ERROR_ACCESS_DENIED, and anything else, means the number is in use by
		// something this account cannot look at.
		return processOpaque
	}
	defer syscall.CloseHandle(handle)
	event, err := syscall.WaitForSingleObject(handle, 0)
	if err != nil {
		return processOpaque
	}
	if event == waitObject0 {
		return processDead
	}
	return processRunning
}

// processImagePath names the executable a running process was started from.
// PROCESS_QUERY_LIMITED_INFORMATION is enough for this across an integrity
// boundary in most configurations; where it is not, the caller gets an error
// and concludes nothing.
func processImagePath(pid int) (string, error) {
	if pid <= 0 {
		return "", fmt.Errorf("pid %d is not a process", pid)
	}
	handle, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		return "", err
	}
	defer syscall.CloseHandle(handle)
	buffer := make([]uint16, maxLongPath)
	size := uint32(len(buffer))
	result, _, callErr := procQueryFullProcessImageNameW.Call(
		uintptr(handle), 0, uintptr(unsafe.Pointer(&buffer[0])), uintptr(unsafe.Pointer(&size)))
	if result == 0 {
		return "", callErr
	}
	return syscall.UTF16ToString(buffer[:size]), nil
}

// openFolder shows the user a directory. A failure here is a warning: it is a
// convenience, not part of running a world.
func openFolder(path string) error {
	return exec.Command("explorer.exe", path).Start()
}

// revealFile shows the folder holding one file, with that file selected.
//
// IT DOES NOT OPEN THE FILE. The one file this is used for is the mod
// framework's log, and what happens when a .log is "opened" is whatever this
// machine has associated with the extension — which may be nothing, may be an
// editor that locks it while the game is writing, and is not this program's
// choice to make. Selecting it in its folder always works and hands the person
// the folder as well.
func revealFile(path string) error {
	// The comma is explorer's own syntax and there is no space after it: the
	// whole '/select,<path>' is ONE argument. Splitting it opens the user's
	// documents folder instead, which is the failure this comment exists for.
	return exec.Command("explorer.exe", "/select,"+path).Start()
}

// systemTool prefers the copy in %SystemRoot%\System32 over whatever the PATH
// resolves, because both tools this file uses are Windows' own.
func systemTool(name string) string {
	if root := os.Getenv("SystemRoot"); root != "" {
		candidate := filepath.Join(root, "System32", name)
		if fileExists(candidate) {
			return candidate
		}
	}
	return name
}
