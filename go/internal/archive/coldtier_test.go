package archive

// The genome cold tier (genomecold.go): the hot window's bundle → cold-copy →
// retire sequence, the fail-safe when no cold archive is configured, the
// gap-requeue trap a retired-to-cold blob must not fall into, and the status
// fields. It is the genome half of the same shape segments_test.go holds for the
// ledger.

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// lowHash builds a valid bb8-genome/1 hash whose first byte is i (0-15), so a set
// of them lands in the first coldBundleShardsPerPass shards and one bundling pass
// collects them all into ONE bundle — the shape these tests assert on. hashN in
// eviction_test.go repeats one hex digit and would scatter blobs across shards a
// single bounded pass cannot reach.
func lowHash(i int) string {
	return "bb8-genome/1:sha256:" + fmt.Sprintf("%02x", i&0x0f) + strings.Repeat("0", 62)
}

func coldArchive(t *testing.T, dir string, hotWindow time.Duration) *Archive {
	t.Helper()
	a, err := New(Config{
		DataDir: dir, PeerID: "archive-test", RelayURL: "ws://test",
		GenomeHotWindow:            hotWindow,
		MetricsMaintenanceInterval: -1,
		Logger: slog.New(slog.NewTextHandler(io.Discard,
			&slog.HandlerOptions{Level: slog.LevelError})),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })
	return a
}

// bundleBaseNames lists the bundle base names present on disk (with a .jsonl.gz).
func bundleBaseNames(t *testing.T, dir string) []string {
	t.Helper()
	bd := genomeBundlesDir(dir)
	ents, err := os.ReadDir(bd)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		n := e.Name()
		if strings.HasSuffix(n, gzSuffix) && !strings.HasSuffix(n, gzSuffix+receiptSuffix) {
			out = append(out, strings.TrimSuffix(n, gzSuffix))
		}
	}
	return out
}

func writeGoodBundleReceipt(t *testing.T, dir, name string) {
	t.Helper()
	bd := genomeBundlesDir(dir)
	path := filepath.Join(bd, name+gzSuffix)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	if err := WriteBundleReceipt(bd, ColdCopyReceipt{
		Segment:            name + gzSuffix,
		Bytes:              info.Size(),
		SHA256:             fileSHA(t, path),
		Destination:        "s3://example-cold-archive/genomes/" + name + gzSuffix,
		RemoteChecksum:     "an-etag-or-checksum",
		RemoteChecksumKind: "etag",
		UploadedAtMs:       now,
		VerifiedAtMs:       now,
		VerifiedBy:         "test",
	}); err != nil {
		t.Fatal(err)
	}
}

func putBlob(t *testing.T, a *Archive, hash string) {
	t.Helper()
	if err := a.genomes.Put(hash, "0.6.3.1", `{"version":"0.6.3.1","nodes":[]}`); err != nil {
		t.Fatal(err)
	}
}

// TestTheColdTierBundlesButNeverDeletesWithoutAReceipt is the fail-safe: with a
// hot window set but NO cold archive configured, blobs past the window are
// bundled and then NOTHING retires. The disk holds the bundle and the hot blob;
// the record survives. It is the genome analogue of MV_COLDCOPY=off.
func TestTheColdTierBundlesButNeverDeletesWithoutAReceipt(t *testing.T) {
	dir := t.TempDir()
	a := coldArchive(t, dir, 7*24*time.Hour)
	aged := []string{lowHash(1), lowHash(2), lowHash(3)}
	for _, h := range aged {
		putBlob(t, a, h)
		ageBlob(t, a, h, 30*24*time.Hour)
	}

	// Two passes: the first bundles, the second's reconcile sees the awaiting
	// bundle. No receipt is ever written.
	a.ColdTierNow(time.Now())
	a.ColdTierNow(time.Now())

	names := bundleBaseNames(t, dir)
	if len(names) != 1 {
		t.Fatalf("bundled into %d bundles, want 1: %v", len(names), names)
	}
	for _, h := range aged {
		if !a.genomes.Has(h) {
			t.Fatalf("a bundled-but-not-cold-copied blob was deleted: %s", h)
		}
	}
	cold, retired, _, awaiting := a.ColdTierCounters()
	if retired != 0 || cold != 0 {
		t.Fatalf("nothing is cold-copied yet, but retired=%d cold=%d", retired, cold)
	}
	if awaiting != 1 {
		t.Fatalf("genomeBundlesAwaitingColdCopy=%d, want 1", awaiting)
	}
}

// TestAConfirmedBundleRetiresTheHotBlobsAndKeepsTheManifest is the whole
// sequence: hot blob → bundled → cold-copied+receipt → retired. The hot bytes go,
// the manifest and receipt stay, and every hash joins the cold index.
func TestAConfirmedBundleRetiresTheHotBlobsAndKeepsTheManifest(t *testing.T) {
	dir := t.TempDir()
	a := coldArchive(t, dir, 7*24*time.Hour)
	aged := []string{lowHash(1), lowHash(2), lowHash(3)}
	served := lowHash(3)
	for _, h := range aged {
		putBlob(t, a, h)
		ageBlob(t, a, h, 30*24*time.Hour)
	}

	a.ColdTierNow(time.Now()) // bundles
	names := bundleBaseNames(t, dir)
	if len(names) != 1 {
		t.Fatalf("want 1 bundle, got %v", names)
	}
	name := names[0]
	// One blob is SERVED between the bundle and the retirement: it must survive the
	// delete (fresh mtime) but is still cold-backed (in the off-host bundle).
	if _, ok := a.genomes.Get(served); !ok {
		t.Fatal("Get missed a stored genome")
	}
	writeGoodBundleReceipt(t, dir, name)

	a.ColdTierNow(time.Now()) // retires

	if a.genomes.Has(lowHash(1)) || a.genomes.Has(lowHash(2)) {
		t.Fatal("a confirmed blob was not removed from the hot store")
	}
	if !a.genomes.Has(served) {
		t.Fatal("a blob served since the bundle was written was retired anyway")
	}
	// The local bytes are gone; the manifest and receipt are kept.
	if exists(filepath.Join(genomeBundlesDir(dir), name+gzSuffix)) {
		t.Fatal("the local bundle bytes were not removed after a confirmed copy")
	}
	if !exists(filepath.Join(genomeBundlesDir(dir), name+manifestSuffix)) {
		t.Fatal("the manifest was deleted with the bundle; it is the record of what went cold")
	}
	if !exists(filepath.Join(genomeBundlesDir(dir), name+gzSuffix+receiptSuffix)) {
		t.Fatal("the receipt was deleted with the bundle")
	}
	cold, retired, coldBytes, awaiting := a.ColdTierCounters()
	if retired != 2 {
		t.Fatalf("genomesRetired=%d, want 2 (the served one was spared)", retired)
	}
	if cold != 3 {
		t.Fatalf("genomesCold=%d, want 3 (the whole manifest is cold-backed)", cold)
	}
	if coldBytes <= 0 || awaiting != 0 {
		t.Fatalf("coldBytes=%d awaiting=%d, want >0 and 0", coldBytes, awaiting)
	}
	// EVERY retired hash reads as held internally, hot copy or not.
	a.mu.Lock()
	for _, h := range aged {
		if !a.heldOrColdLocked(h) {
			a.mu.Unlock()
			t.Fatalf("a retired-to-cold blob does not read as held: %s", h)
		}
	}
	a.mu.Unlock()
}

// TestARetiredToColdBlobIsNotReQueuedForFetch is the CRITICAL correctness trap.
// Without the cold index, a blob retired to cold reads as absent from the hot
// store, so trackLocked queues a fetch, pumpFetches issues it, the blob comes
// back, and the next pass retires it again — forever. The cold index makes it
// count as held so the asking never starts.
func TestARetiredToColdBlobIsNotReQueuedForFetch(t *testing.T) {
	dir := t.TempDir()
	a := coldArchive(t, dir, 7*24*time.Hour)
	cold := lowHash(7)
	putBlob(t, a, cold)
	ageBlob(t, a, cold, 30*24*time.Hour)
	a.ColdTierNow(time.Now())
	name := bundleBaseNames(t, dir)[0]
	writeGoodBundleReceipt(t, dir, name)
	a.ColdTierNow(time.Now())
	if a.genomes.Has(cold) {
		t.Fatal("the fixture is wrong: the blob is still hot")
	}

	now := time.Now()
	a.mu.Lock()
	// trackLocked must treat the cold hash as held and queue NOTHING.
	a.trackLocked(lineageHash{hash: cold, own: true}, "", "", "m-cold", 1, now, now)
	a.ready = true
	a.mu.Unlock()
	if got := a.PendingGaps(); got != 0 {
		t.Fatalf("a retired-to-cold blob was queued as a gap: %d pending", got)
	}

	// And if one somehow got into the queue, pumpFetches retires it as held rather
	// than issuing a request for it.
	a.mu.Lock()
	a.pending[cold] = &fetch{hash: cold, crossedAt: now}
	a.pendingOrder = append(a.pendingOrder, cold)
	a.mu.Unlock()
	a.pumpFetches(now)
	if got := a.PendingGaps(); got != 0 {
		t.Fatalf("pumpFetches did not treat a cold blob as held: %d pending", got)
	}
}

// TestNoHotWindowTiersNothingAndSaysNothing is the off-by-default proof: an
// archive with no hot window bundles nothing, retires nothing, and carries none
// of the cold-tier fields on its status JSON.
func TestNoHotWindowTiersNothingAndSaysNothing(t *testing.T) {
	dir := t.TempDir()
	a := coldArchive(t, dir, 0)
	ancient := lowHash(9)
	putBlob(t, a, ancient)
	ageBlob(t, a, ancient, 3650*24*time.Hour)

	a.ColdTierNow(time.Now())
	if !a.genomes.Has(ancient) {
		t.Fatal("a blob was tiered by an archive with no hot window")
	}
	if len(bundleBaseNames(t, dir)) != 0 {
		t.Fatal("an archive with no hot window wrote a bundle")
	}
	b, err := json.Marshal(a.StatusView())
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"genomeHotWindowMs", "genomeBundlesAwaitingColdCopy",
		"genomesCold", "genomesColdBytes", "genomesRetired"} {
		if strings.Contains(string(b), key) {
			t.Fatalf("%s is on the status of an archive with no hot window:\n%s", key, b)
		}
	}
}

// TestTheHotWindowIsOnTheStatusWhenItIsOn is the other half.
func TestTheHotWindowIsOnTheStatusWhenItIsOn(t *testing.T) {
	a := coldArchive(t, t.TempDir(), 168*time.Hour)
	v := a.StatusView()
	if v.GenomeHotWindowMs != (168 * time.Hour).Milliseconds() {
		t.Fatalf("genomeHotWindowMs=%d, want %d", v.GenomeHotWindowMs, (168 * time.Hour).Milliseconds())
	}
	if !strings.Contains(mustJSON(t, v), `"genomeHotWindowMs"`) {
		t.Fatal("the hot window is not on the status JSON")
	}
}

// TestTheColdIndexSurvivesARestart is the persistence property: a blob retired to
// cold by one process reads as held by the next, because the cold index is loaded
// from the retired bundles' manifests at startup — no re-fetch storm on restart.
func TestTheColdIndexSurvivesARestart(t *testing.T) {
	dir := t.TempDir()
	first := coldArchive(t, dir, 7*24*time.Hour)
	cold := lowHash(5)
	putBlob(t, first, cold)
	ageBlob(t, first, cold, 30*24*time.Hour)
	first.ColdTierNow(time.Now())
	name := bundleBaseNames(t, dir)[0]
	writeGoodBundleReceipt(t, dir, name)
	first.ColdTierNow(time.Now())
	if first.genomes.Has(cold) {
		t.Fatal("the fixture is wrong")
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second := coldArchive(t, dir, 7*24*time.Hour)
	if second.cold.len() == 0 {
		t.Fatal("the cold index was empty after a restart; the manifests were not loaded")
	}
	second.mu.Lock()
	held := second.heldOrColdLocked(cold)
	second.mu.Unlock()
	if !held {
		t.Fatal("a retired-to-cold blob does not read as held after a restart")
	}
	// And `list` counts it as held rather than as a gap.
	if !loadColdSet(dir).has(cold) {
		t.Fatal("loadColdSet (the list CLI's path) does not see the cold blob")
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
