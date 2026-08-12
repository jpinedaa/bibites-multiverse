//go:build windows

package sidecar

// The Windows half of host_unix.go, and the platform every participant runs
// (docs/support-matrix.md). It uses syscall alone: this module has one
// dependency and a diagnostic is not the place to add a second.

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32               = syscall.NewLazyDLL("kernel32.dll")
	procGetDiskFreeSpaceEx = kernel32.NewProc("GetDiskFreeSpaceExW")
)

// freeBytes is the space available to THIS USER on the volume holding path,
// which is what GetDiskFreeSpaceEx's first output gives and what a per-user
// quota makes different from the volume's own free space.
func freeBytes(path string) (int64, bool) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, false
	}
	var freeToCaller, total, free uint64
	r, _, _ := procGetDiskFreeSpaceEx.Call(
		uintptr(unsafe.Pointer(p)),
		uintptr(unsafe.Pointer(&freeToCaller)),
		uintptr(unsafe.Pointer(&total)),
		uintptr(unsafe.Pointer(&free)),
	)
	if r == 0 {
		return 0, false
	}
	return int64(freeToCaller), true
}

// processAlive reports whether pid names a live process. On Windows
// os.FindProcess opens a handle and fails when there is nothing to open, so the
// lookup is the answer — and no signal is sent, which is the same promise the
// POSIX side makes.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	_ = p.Release()
	return true
}
