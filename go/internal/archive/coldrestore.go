package archive

// FETCH-ON-DEMAND: the one path that reads a cold blob's bytes back.
//
// The only place the archive reads a stored genome for anything but serving it
// straight back is brainFor (brain.go): the genealogy's brain shape. When that
// misses on a hash the cold index says is off-host, this restores the blob's
// bundle, re-stores every blob in it (content-addressed, so a re-Put is always
// safe), and INVALIDATES the cached miss so the next poll parses the blob that
// has just come home.
//
// THE APPROACH, AND WHY THIS ONE. A restore is a background queue, never a
// blocking read: brainFor runs off a.mu and off the cache lock precisely so a
// viewer never makes the migration path wait, and a synchronous S3 fetch under
// it would undo that. So the miss returns "absent for now" — which brain.go rule
// 2 already draws as nothing, exactly as a not-yet-arrived genome does — and the
// worker fills the answer in when the bytes land. The queue is a dedup SET, so a
// genealogy that polls a cold species every two seconds enqueues one restore, not
// one per poll.
//
// THE MISS IS INVALIDATED, NOT LEFT TO EXPIRE. brainFor caches its miss so an
// ordinary poll is a map lookup; without the invalidation below, a restored blob
// would stay invisible until the bounded cache turned over. The worker deletes
// the cached miss on success, which is the one thing brain.go's rule-2 comment
// names as the lag this must not reintroduce.
//
// RESTORE IS OFF UNLESS A COMMAND IS CONFIGURED. The archive has no S3 client of
// its own — deploy/coldcopy.sh is the one uploader and the one downloader — so a
// restore shells `MULTIVERSE_COLDCOPY_SCRIPT --restore <bundle>`. With that unset
// the restore is a no-op and a cold genome simply draws nothing, which is a
// correct answer (rule 2) and not an error.

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"multiverse/internal/bb8"
)

// coldRestore is the background restore queue. It is bounded only by the number
// of DISTINCT cold hashes a viewer asks for, which is at most the drawn tree.
type coldRestore struct {
	a  *Archive
	mu sync.Mutex
	// pending dedups: a hash already queued is not queued again.
	pending map[string]struct{}
	queue   []string
	wake    chan struct{}
	// restoreBundle fetches a bundle's .jsonl.gz back onto the host. nil disables
	// on-demand restore. It is a field so a test can supply a local-file fetch in
	// place of the S3 shell-out.
	restoreBundle func(name string) error
}

func newColdRestore(a *Archive) *coldRestore {
	return &coldRestore{
		a:             a,
		pending:       map[string]struct{}{},
		wake:          make(chan struct{}, 1),
		restoreBundle: a.defaultRestoreBundle,
	}
}

// enqueueColdRestore asks for a cold blob to be brought home. It is idempotent
// and never blocks.
func (a *Archive) enqueueColdRestore(hash string) {
	r := a.restore
	if r == nil || hash == "" {
		return
	}
	r.mu.Lock()
	if _, ok := r.pending[hash]; ok {
		r.mu.Unlock()
		return
	}
	r.pending[hash] = struct{}{}
	r.queue = append(r.queue, hash)
	r.mu.Unlock()
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

// startColdRestore launches the worker, only when a hot window is configured (an
// archive with no cold tier never has a cold miss to restore).
func (a *Archive) startColdRestore() {
	if !a.hotWindowOn() || a.restore == nil {
		return
	}
	a.wg.Add(1)
	go func() { defer a.wg.Done(); a.restore.loop(a.ctx.Done()) }()
}

func (r *coldRestore) loop(done <-chan struct{}) {
	for {
		r.mu.Lock()
		if len(r.queue) == 0 {
			r.mu.Unlock()
			select {
			case <-done:
				return
			case <-r.wake:
				continue
			}
		}
		hash := r.queue[0]
		r.queue = r.queue[1:]
		r.mu.Unlock()

		r.a.doColdRestore(hash)

		r.mu.Lock()
		delete(r.pending, hash)
		r.mu.Unlock()

		select {
		case <-done:
			return
		default:
		}
	}
}

// doColdRestore brings one hash home: it finds the bundle whose manifest names
// the hash, fetches that bundle, re-stores every blob in it, and clears the
// cached brain miss for the requested hash.
func (a *Archive) doColdRestore(hash string) {
	hexDigest := bb8.HashHex(hash)
	if hexDigest == "" {
		return
	}
	// ALREADY HOME. Fifty distinct cold hashes queued from one manifest would
	// otherwise be fifty full-bundle downloads of the same ~90 MB object; the
	// first one restores the whole bundle, so every later sibling finds its blob
	// hot and does nothing but clear its own cached miss. This is the check that
	// turns fifty downloads into one.
	if a.genomes.Has(hash) {
		a.invalidateBrainMiss(hash)
		return
	}
	bd := genomeBundlesDir(a.cfg.DataDir)
	name, ok := findBundleForDigest(bd, hexDigest)
	if !ok {
		return
	}
	gz := filepath.Join(bd, name+gzSuffix)
	// Fetch the bundle if it is not already on the host (a restore of a sibling
	// blob may have just brought it back).
	if _, err := os.Stat(gz); err != nil {
		if a.restore == nil || a.restore.restoreBundle == nil {
			return
		}
		if err := a.restore.restoreBundle(name); err != nil {
			a.log.Warn("archive: restoring a genome bundle from cold storage failed",
				"bundle", name, "hash", hash, "err", err)
			return
		}
	}
	restored, err := a.reputBundle(gz)
	if err != nil {
		a.log.Warn("archive: re-storing a restored genome bundle failed", "bundle", name, "err", err)
		return
	}
	// The bundle's bytes are back in the hot store; the local .jsonl.gz can go
	// again. The manifest and receipt stay, so the bundle is still "retired" and
	// the blob is still cold-backed off-host.
	_ = os.Remove(gz)
	// INVALIDATE THE MISS FOR EVERY HASH IN THE BUNDLE, not only the one asked for:
	// they are all hot now, and a genealogy waiting on any sibling should see it on
	// its next poll rather than after its own restore that will short-circuit on
	// the Has check above.
	invalidated := 0
	if hashes, err := readManifest(filepath.Join(bd, name+manifestSuffix)); err == nil {
		for _, h := range hashes {
			a.invalidateBrainMiss(bb8.HashPrefix + h)
			invalidated++
		}
	} else {
		a.invalidateBrainMiss(hash)
	}
	a.log.Info("archive: restored a genome bundle from cold storage on demand",
		"bundle", name, "blobsRestored", restored, "missesInvalidated", invalidated, "forHash", hash)
}

// reputBundle reads a local bundle .jsonl.gz and Puts every blob back into the
// hot store. Content addressing makes each Put idempotent and safe.
func (a *Archive) reputBundle(gz string) (int, error) {
	f, err := os.Open(gz)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	zr, err := gzip.NewReader(bufio.NewReaderSize(f, 1<<20))
	if err != nil {
		return 0, err
	}
	defer zr.Close()
	sc := bufio.NewScanner(zr)
	sc.Buffer(make([]byte, 64<<10), 16<<20)
	n := 0
	for sc.Scan() {
		var line bundleLine
		if json.Unmarshal(sc.Bytes(), &line) != nil || line.Hash == "" {
			continue
		}
		if err := a.genomes.Put(line.Hash, line.Version, line.BB8); err == nil {
			n++
		}
	}
	return n, sc.Err()
}

// findBundleForDigest returns the name of the bundle whose manifest contains the
// digest. Manifests are sorted, so membership is a binary search per manifest;
// this runs on the restore worker, never on the hot path.
func findBundleForDigest(bd, hexDigest string) (string, bool) {
	ents, err := os.ReadDir(bd)
	if err != nil {
		return "", false
	}
	for _, e := range ents {
		name := e.Name()
		if !strings.HasSuffix(name, manifestSuffix) {
			continue
		}
		base := strings.TrimSuffix(name, manifestSuffix)
		hashes, err := readManifest(filepath.Join(bd, name))
		if err != nil {
			continue
		}
		if sortedContains(hashes, hexDigest) {
			return base, true
		}
	}
	return "", false
}

func sortedContains(sorted []string, want string) bool {
	lo, hi := 0, len(sorted)
	for lo < hi {
		mid := (lo + hi) / 2
		switch {
		case sorted[mid] == want:
			return true
		case sorted[mid] < want:
			lo = mid + 1
		default:
			hi = mid
		}
	}
	return false
}

// defaultRestoreBundle shells deploy/coldcopy.sh --restore, named by
// MULTIVERSE_COLDCOPY_SCRIPT. Empty disables on-demand restore.
//
// IT NAMES THE TIER EXPLICITLY. All three cold-copy tiers share the day-seq name
// grammar, so a bundle named 2026-08-25-0000.jsonl.gz can collide with a ledger
// segment of the same name; --restore without --restore-tier resolves to whichever
// tier the script searches first (the ledger), which would fetch the wrong object.
// A genome restore is always --restore-tier genomes.
func (a *Archive) defaultRestoreBundle(name string) error {
	script := strings.TrimSpace(os.Getenv("MULTIVERSE_COLDCOPY_SCRIPT"))
	if script == "" {
		return errColdRestoreUnconfigured
	}
	cmd := exec.CommandContext(a.ctx, script, "--restore", name+gzSuffix, "--restore-tier", "genomes")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return &restoreError{name: name, out: string(out), err: err}
	}
	return nil
}

var errColdRestoreUnconfigured = &restoreError{name: "", out: "MULTIVERSE_COLDCOPY_SCRIPT is unset", err: nil}

type restoreError struct {
	name string
	out  string
	err  error
}

func (e *restoreError) Error() string {
	msg := "archive: cold restore not configured (set MULTIVERSE_COLDCOPY_SCRIPT)"
	if e.name != "" {
		msg = "archive: cold restore of " + e.name + " failed: " + strings.TrimSpace(e.out)
	}
	return msg
}
