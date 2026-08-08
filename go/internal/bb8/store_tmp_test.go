package bb8

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// countTmp is what the outage counted by hand: 15,119 zero-byte scratch files
// left behind by writes that failed on a full disk.
func countTmp(t *testing.T, dir string) int {
	t.Helper()
	n := 0
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && filepath.Ext(p) == ".tmp" {
			n++
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// TestFailedPutLeavesNoScratchFile forces the write to fail the way ENOSPC does
// — after the file exists and before it holds anything — by making the shard
// directory read-only.
func TestFailedPutLeavesNoScratchFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("a read-only directory does not stop root")
	}
	dir := t.TempDir()
	s, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	hash := hashOf(t, `{"body":{"id":{"id":1}}}`)
	shard := filepath.Join(dir, HashHex(hash)[:2])
	if err := os.MkdirAll(shard, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(shard, 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(shard, 0o755)

	if err := s.Put(hash, "0.6.3.1", "blob"); err == nil {
		t.Fatal("Put succeeded into a read-only directory")
	}
	if err := os.Chmod(shard, 0o755); err != nil {
		t.Fatal(err)
	}
	if n := countTmp(t, dir); n != 0 {
		t.Fatalf("a failed Put left %d scratch file(s) behind", n)
	}
}

// TestSweepCollectsAbandonedScratchFiles covers what Put cannot: the process was
// killed between the write and the rename, so nothing on that code path ever
// ran. walk only ever sees .json, so without this the file is invisible forever.
func TestSweepCollectsAbandonedScratchFiles(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	shard := filepath.Join(dir, "ab")
	if err := os.MkdirAll(shard, 0o755); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(shard, "abcdef.json.tmp")
	fresh := filepath.Join(shard, "fedcba.json.tmp")
	for _, p := range []string{stale, fresh} {
		if err := os.WriteFile(p, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Sweep(0, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("the abandoned scratch file survived the sweep: %v", err)
	}
	// A scratch file young enough to belong to a live Put is left alone.
	if _, err := os.Stat(fresh); err != nil {
		t.Fatalf("the sweep took a scratch file a live write could still own: %v", err)
	}
}

// TestSweepKeepsStoredGenomes guards the obvious way to get the above wrong.
func TestSweepKeepsStoredGenomes(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	hash := hashOf(t, `{"body":{"id":{"id":7}}}`)
	if err := s.Put(hash, "0.6.3.1", "blob"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Sweep(0, 0); err != nil {
		t.Fatal(err)
	}
	if !s.Has(hash) {
		t.Fatal("the sweep removed a stored genome")
	}
	if n := countTmp(t, dir); n != 0 {
		t.Fatalf("a successful Put left %d scratch file(s)", n)
	}
}

// hashOf builds a well-formed bb8-genome/1 label from any string. The store
// never verifies a hash against its bytes — §6.10 puts that obligation on the
// fetching caller — so these tests only need a value HashHex accepts.
func hashOf(t *testing.T, seed string) string {
	t.Helper()
	sum := sha256.Sum256([]byte(seed))
	h := HashPrefix + hex.EncodeToString(sum[:])
	if HashHex(h) == "" {
		t.Fatalf("%q is not a bb8-genome/1 hash", h)
	}
	return h
}
