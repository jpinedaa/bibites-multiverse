package archive

// The on-demand cold restore (coldrestore.go), and the three review defects it
// carries: ONE download brings a whole bundle home (H3), a cached miss keeps
// asking until it lands (M4), and a restore names its tier so it cannot fetch a
// same-named ledger segment back by mistake (H2). Plus the crash-path startup
// load (H4): a gz left behind by a kill mid-retirement is still cold.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"multiverse/internal/bb8"
)

// retireOneLowBundle puts hashes into one low-shard bundle, cold-copies it, and
// retires it, returning the bundle name and its .jsonl.gz bytes (captured before
// retirement so a fake restore can hand them back).
func retireOneLowBundle(t *testing.T, a *Archive, dir string, hashes []string) (string, []byte) {
	t.Helper()
	for _, h := range hashes {
		putBlob(t, a, h)
		ageBlob(t, a, h, 30*24*time.Hour)
	}
	a.ColdTierNow(time.Now())
	names := bundleBaseNames(t, dir)
	if len(names) != 1 {
		t.Fatalf("want 1 bundle, got %v", names)
	}
	name := names[0]
	gzBytes, err := os.ReadFile(filepath.Join(genomeBundlesDir(dir), name+gzSuffix))
	if err != nil {
		t.Fatal(err)
	}
	writeGoodBundleReceipt(t, dir, name)
	a.ColdTierNow(time.Now())
	for _, h := range hashes {
		if a.genomes.Has(h) {
			t.Fatalf("fixture: %s is still hot after retirement", h)
		}
	}
	return name, gzBytes
}

// drainRestoreQueue runs the restore worker's body by hand, without its goroutine.
func drainRestoreQueue(t *testing.T, a *Archive) {
	t.Helper()
	for i := 0; i < 10000; i++ {
		a.restore.mu.Lock()
		if len(a.restore.queue) == 0 {
			a.restore.mu.Unlock()
			return
		}
		h := a.restore.queue[0]
		a.restore.queue = a.restore.queue[1:]
		a.restore.mu.Unlock()
		a.doColdRestore(h)
		a.restore.mu.Lock()
		delete(a.restore.pending, h)
		a.restore.mu.Unlock()
	}
	t.Fatal("the restore queue never drained")
}

// TestColdRestoreDownloadsOnceForAWholeBundle is H3: fifty distinct cold hashes
// from one manifest must be ONE download, and every one of their cached misses
// invalidated, not just the requested hash's.
func TestColdRestoreDownloadsOnceForAWholeBundle(t *testing.T) {
	dir := t.TempDir()
	a := coldArchive(t, dir, 7*24*time.Hour)
	hashes := []string{lowHash(1), lowHash(2), lowHash(3)}
	name, gzBytes := retireOneLowBundle(t, a, dir, hashes)
	bd := genomeBundlesDir(dir)

	calls := 0
	a.restore.restoreBundle = func(n string) error {
		if n != name {
			t.Errorf("restore asked for %q, want %q", n, name)
		}
		calls++
		return os.WriteFile(filepath.Join(bd, n+gzSuffix), gzBytes, 0o644)
	}

	// Every hash read cold, caches a miss, and enqueues — as the genealogy would.
	for _, h := range hashes {
		if _, ok := a.brainFor(h); ok {
			t.Fatalf("a cold blob read as present: %s", h)
		}
	}
	drainRestoreQueue(t, a)

	if calls != 1 {
		t.Fatalf("restoreBundle was called %d times, want 1 for the whole bundle", calls)
	}
	for _, h := range hashes {
		if !a.genomes.Has(h) {
			t.Fatalf("%s was not restored to the hot store", h)
		}
		a.brains.mu.Lock()
		_, cached := a.brains.byHash[h]
		a.brains.mu.Unlock()
		if cached {
			t.Fatalf("the cached miss for %s was not invalidated by the restore", h)
		}
	}
	if exists(filepath.Join(bd, name+gzSuffix)) {
		t.Fatal("the restored .jsonl.gz was not removed after re-Put (it stays retired)")
	}
}

// TestColdCachedMissReEnqueuesRestore is M4: a miss cached on a cold hash must
// re-ask on the next poll, or a genome whose first restore failed is invisible
// forever.
func TestColdCachedMissReEnqueuesRestore(t *testing.T) {
	dir := t.TempDir()
	a := coldArchive(t, dir, 7*24*time.Hour)
	h := lowHash(5)
	retireOneLowBundle(t, a, dir, []string{h})
	// Restore always fails, so the blob stays cold and missing.
	a.restore.restoreBundle = func(string) error { return errColdRestoreUnconfigured }

	if _, ok := a.brainFor(h); ok {
		t.Fatal("a cold blob read as present")
	}
	// The worker consumed the enqueue and the restore failed: queue and pending
	// are empty, the miss is still cached.
	a.restore.mu.Lock()
	a.restore.queue = nil
	a.restore.pending = map[string]struct{}{}
	a.restore.mu.Unlock()

	// The next poll on the still-cold, still-missing blob MUST re-enqueue.
	if _, ok := a.brainFor(h); ok {
		t.Fatal("a cold blob read as present on the second poll")
	}
	a.restore.mu.Lock()
	_, pending := a.restore.pending[h]
	queued := len(a.restore.queue)
	a.restore.mu.Unlock()
	if !pending || queued == 0 {
		t.Fatal("a cached miss on a cold hash did not re-enqueue its restore (M4)")
	}
}

// TestDefaultRestoreBundleNamesTheGenomeTier is H2: the archive's restore must
// name the tier, or coldcopy.sh resolves the day-seq name to the ledger tier and
// fetches the wrong object.
func TestDefaultRestoreBundleNamesTheGenomeTier(t *testing.T) {
	dir := t.TempDir()
	a := coldArchive(t, dir, 7*24*time.Hour)
	a.ctx = context.Background() // Start sets this; New does not, and exec needs it.

	argsFile := filepath.Join(dir, "args.txt")
	script := filepath.Join(dir, "fake-coldcopy.sh")
	body := "#!/usr/bin/env bash\nprintf '%s\\n' \"$@\" > " + argsFile + "\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MULTIVERSE_COLDCOPY_SCRIPT", script)

	if err := a.defaultRestoreBundle("2026-08-25-0000"); err != nil {
		t.Fatalf("defaultRestoreBundle: %v", err)
	}
	got, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	if !strings.Contains(s, "--restore-tier") || !strings.Contains(s, "\ngenomes\n") {
		t.Fatalf("restore did not name the genome tier:\n%s", s)
	}
	if !strings.Contains(s, "2026-08-25-0000.jsonl.gz") {
		t.Fatalf("restore did not name the bundle:\n%s", s)
	}
}

// TestColdIndexLoadsAGzLeftByACrash is H4's startup half: a kill after the hot
// blobs were deleted but before the local .jsonl.gz was removed leaves gz +
// manifest + a valid receipt. A restart MUST count those hashes as cold, or the
// relay loop re-fetches blobs that are safely off-host.
func TestColdIndexLoadsAGzLeftByACrash(t *testing.T) {
	dir := t.TempDir()
	first := coldArchive(t, dir, 7*24*time.Hour)
	h := lowHash(6)
	putBlob(t, first, h)
	ageBlob(t, first, h, 30*24*time.Hour)
	first.ColdTierNow(time.Now()) // bundle: gz + manifest present
	name := bundleBaseNames(t, dir)[0]
	writeGoodBundleReceipt(t, dir, name)

	// Simulate the crash: delete the hot blob by hand (as RetireHashes would) but
	// LEAVE the .jsonl.gz on the host.
	hex := bb8.HashHex(h)
	if err := os.Remove(filepath.Join(first.genomes.Dir(), hex[:2], hex+".json")); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if !exists(filepath.Join(genomeBundlesDir(dir), name+gzSuffix)) {
		t.Fatal("fixture: the gz should still be present")
	}

	second := coldArchive(t, dir, 7*24*time.Hour)
	if second.cold.len() == 0 {
		t.Fatal("H4: a gz-present bundle with a valid receipt was not loaded into the cold index at startup")
	}
	second.mu.Lock()
	held := second.heldOrColdLocked(h)
	second.mu.Unlock()
	if !held {
		t.Fatal("H4: a blob whose hot copy a crash deleted (gz still present) does not read as held after restart")
	}
}
