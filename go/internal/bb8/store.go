package bb8

// Store is the on-disk, content-addressed genome cache of contract-b-m4.md §10.
//
// It is a directory of small JSON files under <dir>/<first two hex digits of
// the digest>/<full digest>.json, each holding the game version and the opaque
// blob. Three properties earned that shape: it survives a restart, which
// m3_considerations.md Risk 7 names as the fourth way a fetch fails; it is
// inspectable with cat and jq, which matters for a store whose whole purpose is
// answering "what do you have?"; and the file name is the key, so a corrupt
// index cannot make the store lie about what it holds.
//
// The sidecar and the archive share it. Both are content-addressed by the same
// genome-hash.md value, and neither ever trusts a label without recomputing the
// hash of the bytes behind it (§6.10).

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Store is safe for concurrent use.
type Store struct {
	mu  sync.Mutex
	dir string
}

// Entry is one stored genome, as it appears on disk.
type Entry struct {
	GenomeHash string `json:"genomeHash"`
	Version    string `json:"version"`
	BB8        string `json:"bb8"`
	StoredAt   int64  `json:"storedAt"`
}

// OpenStore creates dir if needed and returns a store rooted there.
func OpenStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

// Dir is the store's root.
func (s *Store) Dir() string { return s.dir }

func (s *Store) path(hash string) (string, error) {
	h := HashHex(hash)
	if h == "" {
		return "", fmt.Errorf("bb8: %q is not a bb8-genome/1 hash", hash)
	}
	return filepath.Join(s.dir, h[:2], h+".json"), nil
}

// Put stores blob under hash. It never verifies the hash itself — the caller
// computed it, and §6.10 makes verification the fetching caller's obligation
// precisely so a store write cannot be the place a wrong genome slips in.
func (s *Store) Put(hash, version, blob string) error {
	p, err := s.path(hash)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := os.Stat(p); err == nil {
		// Content addressing: the same hash is the same bytes. Refresh the
		// access time so the least-recently-served sweep keeps it.
		now := time.Now()
		_ = os.Chtimes(p, now, now)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	body, err := json.Marshal(Entry{GenomeHash: hash, Version: version, BB8: blob,
		StoredAt: time.Now().UnixMilli()})
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, append(body, '\n'), 0o644); err != nil {
		// THE SCRATCH FILE HAS TO GO, AND THE FAILURE THAT LEAVES IT IS THE ONE
		// THAT CANNOT AFFORD IT. os.WriteFile creates before it writes, so a
		// write that fails on a full disk leaves an empty <hash>.json.tmp
		// behind — and the store is content-addressed, so the same genome
		// retries under the same name and leaves the same corpse again. On
		// 2026-08-08 that turned one ENOSPC into 15,119 zero-byte files across
		// the rig's six genome stores, every one of them an inode spent on
		// nothing at the moment inodes and blocks were what the rig had run out
		// of. Removing it costs one syscall on a path already failing.
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, p); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// Get returns one genome. The bool reports whether it was held.
func (s *Store) Get(hash string) (Entry, bool) {
	p, err := s.path(hash)
	if err != nil {
		return Entry{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := os.ReadFile(p)
	if err != nil {
		return Entry{}, false
	}
	var e Entry
	if err := json.Unmarshal(b, &e); err != nil {
		return Entry{}, false
	}
	now := time.Now()
	_ = os.Chtimes(p, now, now)
	return e, true
}

// Has reports whether the store holds hash.
func (s *Store) Has(hash string) bool {
	p, err := s.path(hash)
	if err != nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = os.Stat(p)
	return err == nil
}

// Count returns the number of stored genomes.
func (s *Store) Count() int {
	n := 0
	_ = s.walk(func(string, os.FileInfo) error { n++; return nil })
	return n
}

// sweepStaleTmp removes scratch files no live Put can still own.
func (s *Store) sweepStaleTmp() {
	cutoff := time.Now().Add(-staleTmpAge)
	_ = filepath.Walk(s.dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		if info.IsDir() || filepath.Ext(path) != ".tmp" {
			return nil
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(path)
		}
		return nil
	})
}

func (s *Store) walk(fn func(path string, info os.FileInfo) error) error {
	return filepath.Walk(s.dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		if info.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		return fn(path, info)
	})
}

// staleTmpAge is how long a scratch file must have been untouched before the
// sweep treats it as abandoned. A live Put holds s.mu for the whole
// write-and-rename, so anything this old belongs to a process that is gone.
const staleTmpAge = time.Hour

// Sweep enforces contract-b-m4.md §12's genomeCacheRetentionDays and
// genomeCacheMaxBytes, least-recently-served first. It returns the number of
// entries removed.
//
// It also collects abandoned .tmp files. Put cleans up its own failures now,
// but a kill between the write and the rename cannot, and walk only ever sees
// .json — so without this a crashed process leaves a full-sized scratch file
// that nothing in the system will ever look at again.
func (s *Store) Sweep(retention time.Duration, maxBytes int64) (int, error) {
	type item struct {
		path string
		mod  time.Time
		size int64
	}
	var items []item
	var total int64
	if err := s.walk(func(p string, info os.FileInfo) error {
		items = append(items, item{path: p, mod: info.ModTime(), size: info.Size()})
		total += info.Size()
		return nil
	}); err != nil {
		return 0, err
	}
	s.sweepStaleTmp()
	sort.Slice(items, func(a, b int) bool { return items[a].mod.Before(items[b].mod) })

	s.mu.Lock()
	defer s.mu.Unlock()
	removed := 0
	cutoff := time.Now().Add(-retention)
	for _, it := range items {
		expired := retention > 0 && it.mod.Before(cutoff)
		oversize := maxBytes > 0 && total > maxBytes
		if !expired && !oversize {
			continue
		}
		if err := os.Remove(it.path); err != nil {
			continue
		}
		total -= it.size
		removed++
	}
	return removed, nil
}
