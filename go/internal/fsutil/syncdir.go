//go:build !windows

// Package fsutil holds the one filesystem primitive whose correct form differs
// between the two operating systems this project runs on.
//
// The sidecar's durability rule (contract-a.md §5.3 step 5) is that a
// MIGRATE_OUT_ACK is only correct after the entry reached durable storage. On a
// POSIX filesystem a file fsync is not enough for a rename or a create: the
// directory entry itself has to be flushed, so every rename-into-place is
// followed by an fsync of the containing directory.
//
// Windows has no such call. Opening a directory for the handle that
// FlushFileBuffers needs fails with "Access is denied", which is what the M3
// Windows-sidecar proof hit on the first start: the journal opened, the
// directory sync failed, and the sidecar exited before it ever claimed a slot.
// NTFS journals the metadata of a rename itself, so the POSIX step has no
// Windows counterpart to perform.
package fsutil

import "os"

// SyncDir flushes a directory entry to durable storage. On Windows it is a
// no-op; see the package comment.
func SyncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
