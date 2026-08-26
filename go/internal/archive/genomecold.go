package archive

// THE GENOME COLD TIER. The genome BLOB store is 43 GB across two million files
// and it grew without a bound the ledger already had: the horizon (eviction.go)
// prunes it, but eviction is a permanent DELETE and off by default, so on the
// hosted run nothing had aged out and the disk filled. This file gives genomes
// the SAME shape the ledger has — a bounded HOT WINDOW on the host, an off-host
// COLD copy whose receipt gates retirement, and a forever record of what crossed
// — so the bytes leave the host without the record ever leaving with them.
//
// THE SEQUENCE, and every arrow is a separate, crash-safe step:
//
//	hot blob  ──past the hot window──▶  BUNDLED     a dated .jsonl.gz of the blobs
//	                                                plus a sorted .manifest of
//	                                                their hashes. NOTHING deleted.
//	bundle    ──deploy/coldcopy.sh───▶  COLD-COPIED uploaded and read back, a
//	                                                receipt written beside it.
//	bundle    ──receipt confirms─────▶  RETIRED     the hot blobs are deleted, the
//	                                                local .jsonl.gz is deleted, the
//	                                                manifest+receipt are KEPT, and
//	                                                every hash joins the cold index.
//
// OFF BY DEFAULT AND FAIL-SAFE, exactly like the ledger's MV_COLDCOPY=off. With
// no hot window set nothing here runs at all (§ mirrors eviction.go). With a hot
// window set but no cold archive configured, no receipt is ever written, so no
// bundle retires and no blob is deleted — the disk fills and the record survives,
// which is the only safe direction. And bundling itself is capped: it will not
// build past coldBundleMaxAwaiting un-retired bundles, so a stopped cold archive
// costs a small, bounded amount of DUPLICATED bundle space and never a runaway.
//
// A BLOB SERVED SINCE IT WAS BUNDLED KEEPS ITS WHOLE WINDOW. Retirement re-reads
// the mtime under the store's mutex (bb8.RetireHashes), so a genome fetched
// between the bundle and the receipt is spared the delete — the same guarantee
// the horizon sweep gives, and the reason the hot window is measured on "last
// stored or last served".

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"multiverse/internal/bb8"
	"multiverse/internal/fsutil"
)

const (
	// genomeBundlesDirName holds every genome bundle, its manifest, and its
	// cold-copy receipt. provision.sh names it in multiverse-coldcopy.service's
	// ReadWritePaths so the uploader can write receipts into it.
	genomeBundlesDirName = "genome-bundles"
	// manifestSuffix is the sorted list of hashes a bundle contains. It outlives
	// the bundle's bytes: it is what the cold index loads from and what a restore
	// is planned against.
	manifestSuffix = ".manifest"
	// coldBundleMaxAwaiting is how many un-retired bundles may exist before
	// bundling stops. It is the fail-safe's bound: with the cold archive off,
	// awaiting climbs to here and no more duplicated bundle space is spent.
	coldBundleMaxAwaiting = 4
	// defaultColdTierInterval is how often one bundling+retirement pass runs.
	defaultColdTierInterval = time.Minute
	// The bounds of one bundling pass, in the spirit of eviction.go's: a pass
	// costs what the archive chose, never what the store grew to.
	coldBundleShardsPerPass  = 16
	coldBundleExaminePerPass = 8192
	// coldBundleSize is how many blobs one pass pulls into one bundle.
	coldBundleSize = 4096
)

// coldTierState is the cold tier's whole accounting, under Archive.mu with the
// rest of the counters the status view reads. Zero everywhere on an archive with
// no hot window — which is every archive by default.
type coldTierState struct {
	cursor bb8.EvictCursor
	passes int
	// bundlesWritten and genomesBundled are process totals: bundles created and
	// blobs put into them this run.
	bundlesWritten int
	genomesBundled int
	// retiredBundles is bundles whose off-host copy was confirmed and whose local
	// bytes were removed. genomesRetired and genomesColdBytes are the hot blobs
	// those retirements DELETED and the hot bytes they RECLAIMED — process totals,
	// parallel to GenomesEvicted/GenomesEvictedBytes.
	retiredBundles   int
	genomesRetired   int
	genomesColdBytes int64
	// awaiting is bundles present on the host with no usable receipt, refreshed
	// each pass. badReceipts is the loud subset: a receipt exists and does not
	// hold up. Both are the numbers an operator watches instead of watching for a
	// blob that is gone.
	awaiting    int
	badReceipts int
	// awaitingSet is the hex digests in those un-retired bundles, so the bundling
	// walk does not collect a blob into a second bundle.
	awaitingSet map[string]struct{}
	// loadedCold names the retired bundles whose manifests are already in the cold
	// index, so a pass does not re-read every retired manifest from disk. It is
	// touched only by the cold-tier goroutine (reconcileBundles), never by the
	// status view, so it needs no lock.
	loadedCold map[string]bool
	// badReceipt maps a bundle name to the mtime of the receipt we last verified
	// bad, so a persistently bad receipt is not re-hashed (a ~90 MB read) every
	// pass — only when it changes on disk. Same single-goroutine discipline.
	badReceipt map[string]int64
	lastAt     time.Time
}

// hotWindowOn reports whether this archive tiers anything to cold at all.
func (a *Archive) hotWindowOn() bool { return a.cfg.GenomeHotWindow > 0 }

// heldOrColdLocked is the archive's answer to "do I already hold this genome",
// and it is the ONE place the cold index is consulted from the hot path. A blob
// held HOT (bb8.Store.Has) or held COLD (retired to a receipted off-host bundle)
// both count as held, so nothing re-queues a peer fetch for a blob that has
// simply moved off the host. It is a LOCAL lookup only — an os.Stat and a map
// read — because every caller holds a.mu, the lock the relay read loop needs.
func (a *Archive) heldOrColdLocked(hash string) bool {
	if hash == "" {
		return false
	}
	if a.genomes.Has(hash) {
		return true
	}
	return a.cold.has(hash)
}

func genomeBundlesDir(dir string) string { return filepath.Join(dir, genomeBundlesDirName) }

// startColdTier launches the bundling+retirement loop when a hot window is
// configured, and does nothing at all when one is not. Called from Start.
func (a *Archive) startColdTier() {
	if !a.hotWindowOn() {
		return
	}
	a.log.Warn("archive: the genome HOT WINDOW is ON; blobs older than it are bundled and, once an "+
		"off-host copy is confirmed, removed from the hot store",
		"hotWindow", a.cfg.GenomeHotWindow.String(), "bundles", genomeBundlesDir(a.cfg.DataDir),
		"gate", "a blob is deleted from the hot store ONLY when its bundle's .jsonl.gz.receipt "+
			"confirms an off-host copy. No receipt means it stays, forever if need be",
		"note", "the record of what crossed is kept forever; a retired blob restores on demand and "+
			"answers, until then, exactly like a hash no peer has served yet")
	a.wg.Add(1)
	go func() { defer a.wg.Done(); a.coldTierLoop() }()
}

func (a *Archive) coldTierLoop() {
	interval := a.cfg.ColdTierInterval
	if interval <= 0 {
		interval = defaultColdTierInterval
	}
	a.coldTierPass(time.Now())
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-a.ctx.Done():
			return
		case now := <-t.C:
			a.coldTierPass(now)
		}
	}
}

// coldTierPass is one retirement-then-bundling sweep. Like the eviction pass and
// the ledger maintenance pass it runs OFF THE ARCHIVE'S LOCK — nothing that reads
// a directory or gzips a bundle may hold the lock the relay read loop needs — and
// takes the lock only to read the cursor and to fold the result.
func (a *Archive) coldTierPass(now time.Time) {
	if !a.hotWindowOn() {
		return
	}
	// RETIREMENT FIRST, so a receipted bundle's space is reclaimed before more is
	// bundled, and so the awaiting count that gates bundling is fresh.
	rec := a.reconcileBundles(now)

	a.mu.Lock()
	cur := a.coldTier.cursor
	a.mu.Unlock()

	var wroteBundle bool
	var bundled int
	// BUNDLING, capped: only while the awaiting backlog is below the fail-safe
	// bound. A stopped cold archive lets awaiting climb to the cap and bundling
	// stops there.
	if rec.awaiting < coldBundleMaxAwaiting {
		var err error
		cur, wroteBundle, bundled, err = a.bundlePass(now, cur, rec.awaitingSet)
		if err != nil {
			a.log.Error("archive: a genome bundling pass failed", "err", err,
				"hotWindow", a.cfg.GenomeHotWindow.String())
		}
	}

	a.mu.Lock()
	a.coldTier.cursor = cur
	a.coldTier.passes++
	a.coldTier.awaiting = rec.awaiting
	a.coldTier.badReceipts = rec.bad
	a.coldTier.awaitingSet = rec.awaitingSet
	a.coldTier.retiredBundles += rec.retiredBundles
	a.coldTier.genomesRetired += rec.retiredBlobs
	a.coldTier.genomesColdBytes += rec.freedBytes
	if wroteBundle {
		a.coldTier.bundlesWritten++
		a.coldTier.genomesBundled += bundled
	}
	a.coldTier.lastAt = now
	a.mu.Unlock()

	if rec.retiredBundles > 0 || rec.retiredBlobs > 0 {
		a.log.Warn("archive: retired genome blobs to cold storage after confirming an off-host copy",
			"bundlesRetired", rec.retiredBundles, "blobsRemoved", rec.retiredBlobs,
			"bytesReclaimed", rec.freedBytes, "coldTotal", a.cold.len(),
			"hotWindow", a.cfg.GenomeHotWindow.String())
	}
	if wroteBundle {
		a.log.Info("archive: bundled genome blobs past the hot window for cold copy",
			"blobs", bundled, "awaiting", rec.awaiting+1,
			"note", "deploy/coldcopy.sh uploads genome-bundles/*.jsonl.gz; the blobs are NOT yet "+
				"removed from the hot store")
	}
}

// bundleReconcile is what one reconcile-and-retire returned.
type bundleReconcile struct {
	awaiting       int
	bad            int
	retiredBundles int
	retiredBlobs   int
	freedBytes     int64
	// awaitingSet is the hashes still in un-retired bundles, for the bundling
	// walk's skip.
	awaitingSet map[string]struct{}
}

// reconcileBundles walks the bundle directory once: it retires every bundle whose
// off-host copy is confirmed, and it counts the rest. It NEVER deletes a bundle
// that has no confirmed copy — no receipt, no retirement, forever if need be.
func (a *Archive) reconcileBundles(now time.Time) bundleReconcile {
	out := bundleReconcile{awaitingSet: map[string]struct{}{}}
	dir := a.cfg.DataDir
	bd := genomeBundlesDir(dir)
	ents, err := os.ReadDir(bd)
	if errors.Is(err, os.ErrNotExist) {
		return out
	}
	if err != nil {
		a.log.Warn("archive: the genome-bundles directory could not be read", "err", err, "dir", bd)
		return out
	}
	type forms struct {
		gz, manifest, receipt bool
		gzBytes               int64
	}
	byName := map[string]*forms{}
	var names []string
	get := func(n string) *forms {
		f, ok := byName[n]
		if !ok {
			f = &forms{}
			byName[n] = f
			names = append(names, n)
		}
		return f
	}
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() || strings.HasSuffix(name, tmpSuffix) {
			continue
		}
		switch {
		case strings.HasSuffix(name, gzSuffix+receiptSuffix):
			base := strings.TrimSuffix(name, gzSuffix+receiptSuffix)
			if _, ok := parseSegmentName(base); ok {
				get(base).receipt = true
			}
		case strings.HasSuffix(name, manifestSuffix):
			base := strings.TrimSuffix(name, manifestSuffix)
			if _, ok := parseSegmentName(base); ok {
				get(base).manifest = true
			}
		case strings.HasSuffix(name, gzSuffix):
			base := strings.TrimSuffix(name, gzSuffix)
			if _, ok := parseSegmentName(base); ok {
				f := get(base)
				f.gz = true
				if info, err := e.Info(); err == nil {
					f.gzBytes = info.Size()
				}
			}
		}
	}
	sort.Strings(names)
	cutoff := now.Add(-a.cfg.GenomeHotWindow)
	if a.coldTier.loadedCold == nil {
		a.coldTier.loadedCold = map[string]bool{}
	}
	if a.coldTier.badReceipt == nil {
		a.coldTier.badReceipt = map[string]int64{}
	}
	for _, name := range names {
		f := byName[name]
		switch {
		case !f.gz && f.receipt && f.manifest:
			// Already retired: the bytes are off-host, the local .jsonl.gz is gone,
			// and the manifest+receipt are the record of where it went. Its hashes
			// are in the cold index (from startup, or from the retirement earlier
			// this run); read the manifest again only if we have not already loaded
			// this one — otherwise every pass re-reads every retired manifest.
			if !a.coldTier.loadedCold[name] {
				a.loadManifestIntoCold(bd, name)
				a.coldTier.loadedCold[name] = true
			}
		case f.gz:
			// A receipt we already verified BAD is not re-sha256'd every pass — that
			// is a ~90 MB read on a two-vCPU box, per pass, forever. We remember the
			// receipt's mtime and re-check only when it changes (a re-upload).
			if f.receipt {
				if mt, ok := receiptMtime(bd, name); ok && a.coldTier.badReceipt[name] == mt {
					out.awaiting++
					out.bad++
					a.addAwaiting(bd, name, out.awaitingSet)
					continue
				}
			}
			verdict, why := checkColdCopyBundle(bd, name)
			switch verdict {
			case coldCopyMissing:
				out.awaiting++
				a.addAwaiting(bd, name, out.awaitingSet)
			case coldCopyBad:
				out.awaiting++
				out.bad++
				a.addAwaiting(bd, name, out.awaitingSet)
				if mt, ok := receiptMtime(bd, name); ok {
					a.coldTier.badReceipt[name] = mt
				}
				a.log.Error("archive: a genome bundle's cold-copy receipt does not hold up; the bundle is KEPT",
					"bundle", name, "reason", why,
					"note", "no receipt, no retirement. Fix the receipt or re-upload the bundle")
			case coldCopyOK:
				delete(a.coldTier.badReceipt, name)
				if retired, removed, freed := a.retireBundle(bd, name, cutoff); retired {
					a.coldTier.loadedCold[name] = true
					out.retiredBundles++
					out.retiredBlobs += removed
					out.freedBytes += freed
				}
			}
		}
	}
	return out
}

// retireBundle deletes the hot blobs a confirmed bundle names, deletes the local
// .jsonl.gz, and moves every hash in the manifest into the cold index. The
// manifest and the receipt are KEPT: they are the only record of where the bytes
// went and what a restore fetches. The bool is true when the bundle was actually
// retired (its bytes removed), so the caller counts only a real retirement.
func (a *Archive) retireBundle(bd, name string, cutoff time.Time) (retired bool, removed int, freed int64) {
	hashes, err := readManifest(filepath.Join(bd, name+manifestSuffix))
	if err != nil {
		a.log.Error("archive: a confirmed genome bundle's manifest could not be read; NOT retiring it",
			"bundle", name, "err", err)
		return false, 0, 0
	}
	// THE COLD INDEX IS UPDATED FIRST, before a single blob leaves the hot store.
	// The receipt is already verified by the time we are here, so "cold" is
	// truthful, and inserting first closes the window in which heldOrColdLocked
	// would see a blob neither hot (about to be deleted) nor cold (not yet
	// inserted) and re-queue a peer fetch for it. Every hash is cold-backed: the
	// off-host bundle holds it whether or not the mtime check below spares its hot
	// copy.
	for _, h := range hashes {
		a.cold.addHex(h)
	}
	// Map the hex digests back to full bb8 hashes for the store. A manifest holds
	// the 64-hex digest; bb8.RetireHashes wants the labelled hash.
	full := make([]string, 0, len(hashes))
	for _, h := range hashes {
		full = append(full, bb8.HashPrefix+h)
	}
	removed, freed = a.genomes.RetireHashes(full, cutoff)
	if err := os.Remove(filepath.Join(bd, name+gzSuffix)); err != nil {
		a.log.Error("archive: removing a retired bundle's local bytes failed", "bundle", name, "err", err)
		return false, removed, freed
	}
	_ = fsutil.SyncDir(bd)
	r, _ := BundleReceiptFor(bd, name)
	dest := ""
	if r != nil {
		dest = r.Destination
	}
	a.log.Warn("archive: a genome bundle was REMOVED FROM THE HOT STORE; its confirmed off-host copy is the record",
		"bundle", name+gzSuffix, "blobsRemoved", removed, "bytesReclaimed", freed,
		"destination", dest, "restore", "deploy/coldcopy.sh --restore "+name+gzSuffix+" --restore-tier genomes")
	return true, removed, freed
}

// addAwaiting records the hashes of an un-retired bundle in the skip set so the
// bundling walk does not collect the same blob into a second bundle.
func (a *Archive) addAwaiting(bd, name string, set map[string]struct{}) {
	hashes, err := readManifest(filepath.Join(bd, name+manifestSuffix))
	if err != nil {
		return
	}
	for _, h := range hashes {
		set[h] = struct{}{}
	}
}

// loadManifestIntoCold adds a bundle's manifest hashes to the cold index.
func (a *Archive) loadManifestIntoCold(bd, name string) {
	hashes, err := readManifest(filepath.Join(bd, name+manifestSuffix))
	if err != nil {
		return
	}
	for _, h := range hashes {
		a.cold.addHex(h)
	}
}

// bundlePass collects up to coldBundleSize blobs past the hot window and writes
// them as one dated bundle plus a sorted manifest. It writes NOTHING and deletes
// NOTHING in the store; the cold copy and the retirement are separate steps.
func (a *Archive) bundlePass(now time.Time, cur bb8.EvictCursor,
	awaitingSet map[string]struct{}) (bb8.EvictCursor, bool, int, error) {

	cutoff := now.Add(-a.cfg.GenomeHotWindow)
	skip := func(hexDigest string) bool {
		if a.cold.hasHex(hexDigest) {
			return true
		}
		_, awaiting := awaitingSet[hexDigest]
		return awaiting
	}
	res, err := a.genomes.BundleOlderThan(cutoff, cur, bb8.EvictBudget{
		Shards:  coldBundleShardsPerPass,
		Examine: coldBundleExaminePerPass,
		Remove:  coldBundleSize,
	}, skip)
	if err != nil {
		return res.Cursor, false, 0, err
	}
	if len(res.Entries) == 0 {
		return res.Cursor, false, 0, nil
	}
	if err := a.writeBundle(now, res.Entries); err != nil {
		return res.Cursor, false, 0, err
	}
	return res.Cursor, true, len(res.Entries), nil
}

// bundleLine is one blob as it appears in a bundle: everything a restore needs to
// re-Put it, which is content-addressed and therefore always safe.
type bundleLine struct {
	Hash    string `json:"hash"`
	Version string `json:"version"`
	BB8     string `json:"bb8"`
}

// writeBundle writes <name>.jsonl.gz and <name>.manifest atomically. The bundle
// is a gzip of one bundleLine per entry; the manifest is the sorted hex digests.
// A crash between the two leaves a bundle with no manifest, which the next
// reconcile ignores (no manifest, not retirable) and the startup sweep collects.
func (a *Archive) writeBundle(now time.Time, entries []bb8.Entry) error {
	bd := genomeBundlesDir(a.cfg.DataDir)
	if err := os.MkdirAll(bd, 0o755); err != nil {
		return err
	}
	name, err := a.nextBundleName(bd, now)
	if err != nil {
		return err
	}
	// The bundle bytes.
	gz := filepath.Join(bd, name+gzSuffix)
	gtmp := gz + tmpSuffix
	out, err := os.OpenFile(gtmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	zw, err := gzip.NewWriterLevel(out, gzip.BestSpeed)
	if err != nil {
		out.Close()
		_ = os.Remove(gtmp)
		return err
	}
	digests := make([]string, 0, len(entries))
	encFail := func(e error) error {
		zw.Close()
		out.Close()
		_ = os.Remove(gtmp)
		return e
	}
	for _, ent := range entries {
		b, err := json.Marshal(bundleLine{Hash: ent.GenomeHash, Version: ent.Version, BB8: ent.BB8})
		if err != nil {
			return encFail(err)
		}
		if _, err := zw.Write(append(b, '\n')); err != nil {
			return encFail(err)
		}
		if h := bb8.HashHex(ent.GenomeHash); h != "" {
			digests = append(digests, h)
		}
	}
	if err := zw.Close(); err != nil {
		out.Close()
		_ = os.Remove(gtmp)
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		_ = os.Remove(gtmp)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(gtmp)
		return err
	}
	if err := os.Rename(gtmp, gz); err != nil {
		_ = os.Remove(gtmp)
		return err
	}
	// The manifest: sorted so a restore can binary-search it and so two manifests
	// of the same blobs are byte-identical.
	sort.Strings(digests)
	var mbuf strings.Builder
	for _, d := range digests {
		mbuf.WriteString(d)
		mbuf.WriteByte('\n')
	}
	mf := filepath.Join(bd, name+manifestSuffix)
	mtmp := mf + tmpSuffix
	// FSYNC BEFORE RENAME. The gz is fsynced above; the manifest must be too, or a
	// power loss can leave the durable gz beside a manifest that reverts to empty
	// — and the manifest is the cold index's whole source of truth about which
	// hashes that bundle holds. A bundle with no manifest is not retirable (the
	// reconcile ignores it), so an empty one silently strands the bundle.
	if err := writeFileSync(mtmp, []byte(mbuf.String())); err != nil {
		_ = os.Remove(mtmp)
		return err
	}
	if err := os.Rename(mtmp, mf); err != nil {
		_ = os.Remove(mtmp)
		return err
	}
	return fsutil.SyncDir(bd)
}

// writeFileSync writes b to path and fsyncs it before returning, so a following
// rename cannot outrun the data to disk.
func writeFileSync(path string, b []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// nextBundleName is today's date and the next free sequence in the bundle dir.
func (a *Archive) nextBundleName(bd string, now time.Time) (string, error) {
	day := now.UTC().Truncate(24 * time.Hour)
	prefix := day.Format(dayLayout)
	ents, err := os.ReadDir(bd)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	seq := 0
	for _, e := range ents {
		name := e.Name()
		base := name
		for _, suf := range []string{gzSuffix + receiptSuffix, manifestSuffix, gzSuffix, plainSuffix} {
			if strings.HasSuffix(base, suf) {
				base = strings.TrimSuffix(base, suf)
				break
			}
		}
		s, ok := parseSegmentName(base)
		if !ok || s.FirstDay.Format(dayLayout) != prefix {
			continue
		}
		if s.Seq >= seq {
			seq = s.Seq + 1
		}
	}
	return segmentName(day, seq), nil
}

// loadColdIndexAtStart fills the cold index from every bundle that has a manifest
// and a verified receipt — WHETHER OR NOT its .jsonl.gz is still on the host. That
// last part is the crash-safety: a process killed after RetireHashes deleted the
// hot blobs but BEFORE it removed the local .jsonl.gz leaves the gz present, and
// skipping such a bundle would leave thousands of hashes deleted-from-hot and
// absent-from-index while the relay loop and the fetch pump are already running —
// a startup fetch-storm for blobs that are safely off-host. So:
//
//	gz absent  + manifest + receipt   RETIRED: trust the receipt, load.
//	gz present + manifest + receipt   verify the receipt; load iff it holds up
//	                                  (the crash case, and also a bundle whose copy
//	                                  is confirmed but whose retirement has not run
//	                                  yet — both are genuinely cold-backed).
//
// It runs once, from New. It returns how many distinct hashes it loaded.
func (a *Archive) loadColdIndexAtStart() int {
	bd := genomeBundlesDir(a.cfg.DataDir)
	if a.coldTier.loadedCold == nil {
		a.coldTier.loadedCold = map[string]bool{}
	}
	for _, base := range coldBackedBundles(bd) {
		a.loadManifestIntoCold(bd, base)
		a.coldTier.loadedCold[base] = true
	}
	return a.cold.len()
}

// coldBackedBundles is the shared enumeration behind loadColdIndexAtStart and
// loadColdSet: every bundle whose manifest+receipt confirm an off-host copy, gz
// present or not. A gz-present bundle is included only if its receipt actually
// verifies against the bytes on disk.
func coldBackedBundles(bd string) []string {
	ents, err := os.ReadDir(bd)
	if err != nil {
		return nil
	}
	present := map[string]bool{}
	manifests := map[string]bool{}
	receipts := map[string]bool{}
	for _, e := range ents {
		name := e.Name()
		switch {
		case strings.HasSuffix(name, gzSuffix+receiptSuffix):
			receipts[strings.TrimSuffix(name, gzSuffix+receiptSuffix)] = true
		case strings.HasSuffix(name, manifestSuffix):
			manifests[strings.TrimSuffix(name, manifestSuffix)] = true
		case strings.HasSuffix(name, gzSuffix):
			present[strings.TrimSuffix(name, gzSuffix)] = true
		}
	}
	var out []string
	for base := range manifests {
		if !receipts[base] {
			continue
		}
		if present[base] {
			// gz still on the host: only cold if the receipt holds up against it.
			if v, _ := checkColdCopyBundle(bd, base); v != coldCopyOK {
				continue
			}
		}
		out = append(out, base)
	}
	sort.Strings(out)
	return out
}

// receiptMtime is the on-disk mtime of a bundle's receipt, for the bad-receipt
// re-check cache. The bool is false when there is no receipt.
func receiptMtime(bd, name string) (int64, bool) {
	info, err := os.Stat(filepath.Join(bd, name+gzSuffix+receiptSuffix))
	if err != nil {
		return 0, false
	}
	return info.ModTime().UnixNano(), true
}

// loadColdSet builds a cold index from a data directory's retired bundles, for
// callers with no running archive — the `list` CLI most of all, which must count
// a genome retired to cold as held rather than as a gap.
func loadColdSet(dir string) *coldIndex {
	c := newColdIndex()
	bd := genomeBundlesDir(dir)
	for _, base := range coldBackedBundles(bd) {
		if hashes, err := readManifest(filepath.Join(bd, base+manifestSuffix)); err == nil {
			for _, h := range hashes {
				c.addHex(h)
			}
		}
	}
	return c
}

// ColdTierCounters is the cold tier's accounting, for tests and the status view.
func (a *Archive) ColdTierCounters() (cold, retired int, coldBytes int64, awaiting int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cold.len(), a.coldTier.genomesRetired, a.coldTier.genomesColdBytes, a.coldTier.awaiting
}

// ColdTierNow runs one pass synchronously, for tests that drive the loop by hand.
func (a *Archive) ColdTierNow(now time.Time) { a.coldTierPass(now) }

// --------------------------------------------------------------- receipt gate

// checkColdCopyBundle decides whether a bundle's off-host copy is confirmed. It
// is the segment gate (coldcopy.go) pointed at the bundle directory: the same
// read-back-and-verify of size and sha256, against the same receipt shape.
func checkColdCopyBundle(bd, name string) (coldCopyVerdict, string) {
	gz := filepath.Join(bd, name+gzSuffix)
	rcpt := gz + receiptSuffix
	return verifyColdCopyAt(gz, rcpt, name+gzSuffix)
}

// WriteBundleReceipt writes a cold-copy receipt for a genome bundle, beside the
// bundle. It is the bundle analogue of WriteReceipt and it accepts the same name
// class — a dated day-seq name — because a bundle and a ledger segment share the
// grammar and differ only in which directory the receipt lands in.
func WriteBundleReceipt(bd string, r ColdCopyReceipt) error {
	if !strings.HasSuffix(r.Segment, gzSuffix) {
		return fmt.Errorf("archive: a cold-copy receipt names a compressed file, got %q", r.Segment)
	}
	name := strings.TrimSuffix(r.Segment, gzSuffix)
	if _, ok := parseSegmentName(name); !ok {
		return fmt.Errorf("archive: %q is not a bundle name", name)
	}
	if r.VerifiedAtMs <= 0 {
		return errors.New("archive: a cold-copy receipt with no verification time does not confirm anything")
	}
	return writeReceiptFile(filepath.Join(bd, name+gzSuffix+receiptSuffix), r)
}

// BundleReceiptFor reads a bundle's receipt, for tools and tests.
func BundleReceiptFor(bd, name string) (*ColdCopyReceipt, error) {
	return readReceipt(filepath.Join(bd, name+gzSuffix+receiptSuffix))
}
