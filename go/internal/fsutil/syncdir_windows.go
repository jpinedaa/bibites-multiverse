//go:build windows

package fsutil

// SyncDir is a no-op on Windows. A directory handle cannot be flushed there —
// os.Open on a directory followed by Sync returns "Access is denied" — and NTFS
// journals rename and create metadata itself, so there is nothing for the
// caller to force. See the package comment in syncdir.go.
func SyncDir(string) error { return nil }
