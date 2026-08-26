package archive

// THE COLD INDEX: the archive's local, authoritative answer to "does this genome
// live off-host?", consulted at every place the archive asks whether it already
// holds a blob. It exists to close ONE correctness trap and it is worth stating
// in full, because getting it wrong is a re-fetch storm that never ends:
//
//	A blob retired to cold has been DELETED from the hot store. Every
//	"do I hold this?" check (heldAlready, trackLocked, pumpFetches,
//	expireLoadedGapsLocked) reads the hot store with bb8.Store.Has. Without this
//	index, each of those reads false for a retired blob, re-queues a peer fetch
//	for it, the fetch lands, the blob is re-stored hot, the next hot-window pass
//	bundles and retires it again — forever, for every one of two million blobs.
//	The index makes a retired-to-cold blob count as HELD, so the asking stops,
//	exactly as the horizon's "keep the hash forever" makes a pruned hash answer
//	like an unknown one.
//
// IT IS A LOCAL LOOKUP AND NEVER A NETWORK CALL. Every consult happens under
// a.mu (the lock the relay read loop needs), so it must not read the disk or the
// wire; it is an in-memory set decode-and-lookup and nothing else. The disk —
// the per-bundle sorted manifests — is read only to LOAD the set, at startup and
// when a bundle retires, both off the hot path.
//
// RAM, AND WHY THIS SHAPE. The deployment holds ~2 million blobs. A naive
// map[string]struct{} of 64-hex-character digests is ~200 MB once Go's per-entry
// string headers and map overhead are counted. The digest is a sha256, so the
// hot 32 bytes are stored as a [32]byte VALUE key: map[[32]byte]struct{} carries
// no string header and no separate key allocation, and measures ~70 MB at two
// million — a third of the naive map, exact (no false "held" the way a bloom
// filter would give), and O(1) with no disk read per lookup. The sorted on-disk
// manifests remain the durable source; a bloom-filter front end over them is the
// drop-in if the cold set ever outgrows this, but exactness is the property the
// trap above cannot trade away, so the plain set is what ships.

import (
	"bufio"
	"encoding/hex"
	"os"
	"sync"

	"multiverse/internal/bb8"
)

// coldIndex is the set of genome digests that live in a receipted, retired
// off-host bundle. It carries its own lock — a leaf lock, always taken while a.mu
// is already held on the read path and always taken alone on the write path, so
// there is one lock order and it cannot deadlock a.mu.
type coldIndex struct {
	mu  sync.RWMutex
	set map[[32]byte]struct{}
}

func newColdIndex() *coldIndex {
	return &coldIndex{set: map[[32]byte]struct{}{}}
}

// digestOf turns a bb8-genome/1 hash into its raw 32-byte sha256, or false when
// the string is not one. It never allocates a string.
func coldDigestOf(hash string) ([32]byte, bool) {
	h := bb8.HashHex(hash)
	if h == "" {
		return [32]byte{}, false
	}
	var d [32]byte
	if _, err := hex.Decode(d[:], []byte(h)); err != nil {
		return [32]byte{}, false
	}
	return d, true
}

// has reports whether hash is in cold storage. It is the hot-path consult and it
// is a single map read under a read lock.
func (c *coldIndex) has(hash string) bool {
	if c == nil {
		return false
	}
	d, ok := coldDigestOf(hash)
	if !ok {
		return false
	}
	c.mu.RLock()
	_, in := c.set[d]
	c.mu.RUnlock()
	return in
}

// hasHex reports whether a 64-char hex digest is in cold storage. It is the
// bundling walk's skip check, which already holds the digest and not the hash.
func (c *coldIndex) hasHex(hexDigest string) bool {
	if c == nil || len(hexDigest) != 64 {
		return false
	}
	var d [32]byte
	if _, err := hex.Decode(d[:], []byte(hexDigest)); err != nil {
		return false
	}
	c.mu.RLock()
	_, in := c.set[d]
	c.mu.RUnlock()
	return in
}

// addHex records one hex digest (64 chars, as a manifest holds it). Unparseable
// input is ignored: a manifest line that is not a digest is not a cold blob.
func (c *coldIndex) addHex(hexDigest string) bool {
	var d [32]byte
	if len(hexDigest) != 64 {
		return false
	}
	if _, err := hex.Decode(d[:], []byte(hexDigest)); err != nil {
		return false
	}
	c.mu.Lock()
	if _, in := c.set[d]; in {
		c.mu.Unlock()
		return false
	}
	c.set[d] = struct{}{}
	c.mu.Unlock()
	return true
}

// addHash records one full bb8-genome/1 hash.
func (c *coldIndex) addHash(hash string) bool {
	d, ok := coldDigestOf(hash)
	if !ok {
		return false
	}
	c.mu.Lock()
	if _, in := c.set[d]; in {
		c.mu.Unlock()
		return false
	}
	c.set[d] = struct{}{}
	c.mu.Unlock()
	return true
}

func (c *coldIndex) len() int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	n := len(c.set)
	c.mu.RUnlock()
	return n
}

// readManifest returns the hex digests a bundle manifest names, one per line. A
// blank or malformed line is skipped, as a torn ledger line is.
func readManifest(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 4<<10), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if len(line) == 64 {
			out = append(out, line)
		}
	}
	return out, sc.Err()
}
