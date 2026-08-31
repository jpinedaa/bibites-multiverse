package archive

// PARTICIPANT RECOGNITION: the little the archive REMEMBERS about who is on the
// map (contract-b-m4.md §33, B49).
//
// WHAT IS NEW HERE IS MEMORY, NOT A WIRE FIELD. `keeper` and `worldName` ride on
// the stats block, and the stats block is STATE: §10.1 rule 3 makes a block older
// than statsStaleMs history rather than state, and the moment a world's sidecar
// stops publishing, everything on that block reads as unknown. That is exactly
// right for a population and exactly wrong for a name. A dark slot whose keeper
// went unknown the instant it went dark is the ONE case B49 was written for —
// "which is what lets an operator name a dark slot instead of pointing at a
// number" — so the name has to outlive the block that carried it, and outliving
// the process is the same problem one restart later.
//
// KEYED ON peerId, NEVER ON slot. Slots are reserved and never reused (§7.3),
// but a slot is a ROUTING ADDRESS and the identity is the peerId; a store keyed
// on the seat would hand a new participant the previous occupant's history the
// first time the map is rebuilt. The entry carries the slot as a FACT ABOUT THE
// PEER, refreshed from every frame, so a peer that moves keeps its own record.
//
// THREE THINGS ARE REMEMBERED AND EACH HAS ITS OWN RULE:
//
//   - firstSeenMs is SET ONCE, at the first PEER_STATUS this archive ever saw
//     the peer in, and never touched again. It is "when this archive first knew
//     about you" and it is honest about being that rather than about when the
//     world was created, which nothing on either wire says.
//
//   - maxSimulatedTime is a HIGH-WATER MARK and never a reading. simulatedTime
//     runs continuously across a restart of the same world, so it only falls
//     when a save is restored from behind where the world had got to — a rewind
//     of the LIVE clock that is not a rewind of what the world actually
//     simulated. achieved.go handles the same fall by throwing its window away,
//     because a rate across a rewind is not a measurement; here the stored value
//     simply does not go down. See the continuity note at achieved.go:110.
//
//   - keeper and worldName are COPIED VERBATIM FROM A FRESH BLOCK, INCLUDING
//     WHEN THE FRESH BLOCK OMITS THEM. A participant who deletes the setting is
//     asking to be unnamed, and a store that kept the last name it liked would
//     turn a consent decision into a durable publication. A slot with NO stats
//     block at all is the other case entirely — nothing has been said, so the
//     last thing said stands.
//
// AND IT IS BOUNDED, like every other retained structure in this package. One
// entry per peerId with nothing ever removed is a slow leak rather than a fast
// one — the relay grants the slots, and there are dozens — but "it grows slowly"
// is a promise about somebody else's behaviour, so recognitionPeerMax caps it and
// pruneLocked drops the least recently seen past the cap.
//
// IT IS DISPLAY STATE AND NOTHING ELSE. Nothing routes, matches, deduplicates or
// authorizes on any of it (B49's "a label, never a key"), the strings are stored
// as the untrusted display text they are — no trimming, no repair, the author's
// side already did that and §33 forbids a second party doing it again — and a
// lost sidecar costs the map its memory of who was there, never a record of what
// crossed.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"multiverse/internal/contractb"
)

const (
	// recognitionStateName is the sidecar, beside metrics-rollup.json.
	recognitionStateName = "recognition.json"
	// recognitionStateVersion is the format's own version. A file whose header
	// names a version this build does not know is refused whole rather than
	// half-read, exactly as the brain sidecar's is.
	recognitionStateVersion = 1
	// recognitionSaveInterval is how often the store is flushed, on the same
	// timer and behind its own interval as the brain and roll-up sidecars. What
	// a hard kill costs is thirty seconds of first sightings and at most thirty
	// seconds of simulated time off a high-water mark.
	recognitionSaveInterval = 30 * time.Second
	// recognitionPeerMax bounds the store, on the same terms as every other
	// retained structure in this package (speciesAggMax, rollupPeerMax,
	// brainCacheMax): a map keyed on a string that arrives from outside gets a
	// CAP AND A RULE rather than a promise about how the outside behaves.
	//
	// It is not a limit anything is expected to reach. An entry is created only
	// for a peerId the RELAY put in a slot, slots are reserved and never reused
	// (§7.3), and the public map's slot count is in the dozens — so this is three
	// orders of magnitude of headroom over the announced run, and a store this
	// size is about 200 kB of JSON. What it removes is the unbounded case: a relay
	// bug, a rehearsal that cycles identities, or an operator's own long-lived
	// archive over years, none of which should end in a sidecar nothing trims.
	//
	// Past it the OLDEST entries go, oldest meaning least recently seen — see
	// pruneLocked. A dropped peer is not erased from the map, and nothing about
	// the world changes: its next PEER_STATUS makes a new entry with a new first
	// sighting, which is the same honest loss the sidecar already takes when the
	// file is lost, and the entries reaching the cap are the ones this archive
	// has heard nothing from for longest.
	recognitionPeerMax = 4096
)

// recognitionEntry is one peer's durable identity on this map.
type recognitionEntry struct {
	PeerID string `json:"peerId"`
	// Slot is the seat this peer was last seen in. It is a fact about the peer
	// and never the key: see the header.
	Slot        int   `json:"slot"`
	FirstSeenMs int64 `json:"firstSeenMs"`
	// MaxSimulatedTime is simulated SECONDS, monotonic, and never falls.
	MaxSimulatedTime float64 `json:"maxSimulatedTime,omitempty"`
	// HaveSimulatedTime is the ABSENT/ZERO distinction made explicit, for the
	// same reason SpeciesKnown exists on SlotView: a world that has simulated
	// exactly nothing reads 0, and a world no stats block ever carried a
	// simulatedTime for reads UNKNOWN. They are different facts and a float on
	// its own cannot hold both.
	HaveSimulatedTime bool   `json:"haveSimulatedTime,omitempty"`
	Keeper            string `json:"keeper,omitempty"`
	WorldName         string `json:"worldName,omitempty"`
	// LastSeenMs is the last PEER_STATUS this archive saw the peer in, and it is
	// the store's OWN HOUSEKEEPING FIELD: nothing serves it, nothing renders it,
	// and recognitionPeerMax's eviction is the only thing that reads it.
	//
	// IT DOES NOT MAKE THE STORE DIRTY. Every frame moves it, so a save keyed on
	// it would rewrite this file every thirty seconds forever — including on a
	// map where nothing has changed, which is the case the dirty flag exists for.
	// It is written out whenever something else moves, so what survives a restart
	// is a slightly stale ordering rather than none, and a stale ordering is
	// enough for a bound nothing is expected to reach.
	LastSeenMs int64 `json:"lastSeenMs,omitempty"`
}

// recognitionState is the on-disk shape: a version and a sorted list, so a
// diff between two saves reads and `jq` over one is worth typing.
type recognitionState struct {
	V     int                `json:"v"`
	Peers []recognitionEntry `json:"peers"`
}

// recognitionStore is the live map of entries.
//
// ITS LOCK IS ITS OWN, and it is never held across a file write. The intake path
// takes the archive's lock and then this one, so a save that wrote to disk under
// this lock would put a disk write on the read loop's critical path — Risk 4's
// exact shape. save copies what it needs out and writes with the lock released.
type recognitionStore struct {
	mu    sync.Mutex
	path  string
	peers map[string]recognitionEntry
	// dirty is what has moved since the last successful save, so an archive that
	// nothing has joined rewrites nothing on every tick.
	dirty bool
}

// openRecognitionStore loads the sidecar, or starts an empty one. An unreadable
// sidecar is a loss that is SAID and never a reason to refuse to run: the map
// keeps working and the names start again from the next PEER_STATUS.
func openRecognitionStore(dir string) (*recognitionStore, error) {
	r := &recognitionStore{
		path:  filepath.Join(dir, recognitionStateName),
		peers: map[string]recognitionEntry{},
	}
	b, err := os.ReadFile(r.path)
	if errors.Is(err, os.ErrNotExist) {
		return r, nil
	}
	if err != nil {
		return r, err
	}
	var st recognitionState
	if err := json.Unmarshal(b, &st); err != nil || st.V != recognitionStateVersion {
		// Move the unusable file aside rather than overwrite it: it is the only
		// copy of when each participant joined, and a build that cannot read it
		// is not proof the next one cannot either.
		aside := fmt.Sprintf("%s.unreadable.%d", r.path, time.Now().UnixNano())
		if rnErr := os.Rename(r.path, aside); rnErr != nil {
			return r, rnErr
		}
		if err == nil {
			err = fmt.Errorf("format version %d, want %d", st.V, recognitionStateVersion)
		}
		return r, fmt.Errorf("recognition sidecar %s could not be used and was moved to %s: %w",
			filepath.Base(r.path), filepath.Base(aside), err)
	}
	for _, e := range st.Peers {
		if e.PeerID == "" {
			continue
		}
		r.peers[e.PeerID] = e
	}
	// A file written by a build with no bound, or with a larger one, is trimmed
	// to this build's on the way in rather than on the first frame — which is
	// also what makes the bound true of an archive nothing has joined yet.
	r.pruneLocked()
	return r, nil
}

// Path is the file behind the store, for a startup log line and for tests.
func (r *recognitionStore) Path() string {
	if r == nil {
		return ""
	}
	return r.path
}

// Len is how many peers the store remembers.
func (r *recognitionStore) Len() int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.peers)
}

// observe folds one accepted PEER_STATUS in. It is called from the frame handler
// rather than from StatusView for the reason achieved.go gives: §10.1 rule 1 is
// ONE SOURCE, NO POLLING, and a first sighting that only happened when somebody
// loaded the page would be a measurement of the reader.
//
// It touches no file. nowMs is the archive's own clock, which is what
// firstSeenMs is honest about being: the relay clock on the frame ages the STATS
// (§6.5), and this is the moment this archive first knew about the peer.
func (r *recognitionStore) observe(status contractb.PeerStatus, nowMs int64) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, si := range status.Slots {
		if si.PeerID == "" {
			// A seat with no peer in it is a reservation, not a participant.
			continue
		}
		e, known := r.peers[si.PeerID]
		if !known {
			e.PeerID, e.FirstSeenMs = si.PeerID, nowMs
			r.dirty = true
		}
		if e.Slot != si.Slot {
			e.Slot = si.Slot
			r.dirty = true
		}
		// Housekeeping only, and deliberately not a change: see the field.
		e.LastSeenMs = nowMs
		if si.Stats == nil {
			// A DARK WORLD KEEPS ITS NAME. Nothing was said on this frame, so
			// the last thing said stands — which is the whole point of the
			// store and the case B49 names.
			r.peers[si.PeerID] = e
			continue
		}
		// A block that carries the fields carries the ABSENCE of them too. A
		// participant who removed the setting is asking to be unnamed, and the
		// only way to honour that is to copy an empty string as readily as a
		// name (§33's "absence is unknown, and unknown is not anonymous").
		if e.Keeper != si.Stats.Keeper || e.WorldName != si.Stats.WorldName {
			e.Keeper, e.WorldName = si.Stats.Keeper, si.Stats.WorldName
			r.dirty = true
		}
		// The high-water mark, and the only comparison in this file: a restore
		// can rewind the live clock, and what a world has simulated does not
		// un-happen because a save was loaded from behind it.
		if sim := si.Stats.SimulatedTime; sim != nil && (!e.HaveSimulatedTime || *sim > e.MaxSimulatedTime) {
			e.MaxSimulatedTime, e.HaveSimulatedTime = *sim, true
			r.dirty = true
		}
		r.peers[si.PeerID] = e
	}
	r.pruneLocked()
}

// pruneLocked keeps the store inside recognitionPeerMax by dropping the entries
// this archive has heard from least recently. The caller holds the lock, and the
// ordinary path is one length check.
//
// LEAST RECENTLY SEEN, AND THE SEAT AS THE TIEBREAK. Slots are handed out in
// order and never reused (§7.3), so a higher slot is a later arrival — the same
// fact the landing page's "new on the map" list is ordered by — which leaves the
// order total even for a store reloaded from a file written before lastSeenMs
// was recorded for every entry.
func (r *recognitionStore) pruneLocked() {
	if len(r.peers) <= recognitionPeerMax {
		return
	}
	ranked := make([]recognitionEntry, 0, len(r.peers))
	for _, e := range r.peers {
		ranked = append(ranked, e)
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].LastSeenMs != ranked[j].LastSeenMs {
			return ranked[i].LastSeenMs > ranked[j].LastSeenMs
		}
		if ranked[i].Slot != ranked[j].Slot {
			return ranked[i].Slot > ranked[j].Slot
		}
		return ranked[i].PeerID < ranked[j].PeerID
	})
	for _, e := range ranked[recognitionPeerMax:] {
		delete(r.peers, e.PeerID)
	}
	// This one IS a change: the file has to stop carrying what the store no
	// longer does, or a restart would load the evicted entries straight back.
	r.dirty = true
}

// lookup is one peer's entry, or (zero, false) for a peer this archive has never
// seen. The caller renders the false case as UNKNOWN and never as anonymous.
func (r *recognitionStore) lookup(peerID string) (recognitionEntry, bool) {
	if r == nil || peerID == "" {
		return recognitionEntry{}, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.peers[peerID]
	return e, ok
}

// entries is every remembered peer, sorted by slot, for tests and operator
// tools.
func (r *recognitionStore) entries() []recognitionEntry {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return sortedRecognitionEntries(r.peers)
}

// save writes the sidecar when anything has moved, and does nothing at all when
// nothing has. The snapshot is taken under the lock and the bytes are written
// with it RELEASED: see the type's comment for why.
func (r *recognitionStore) save() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	if !r.dirty {
		r.mu.Unlock()
		return nil
	}
	st := recognitionState{V: recognitionStateVersion, Peers: sortedRecognitionEntries(r.peers)}
	r.dirty = false
	r.mu.Unlock()

	b, err := json.Marshal(st)
	if err != nil {
		r.remarkDirty()
		return err
	}
	tmp := r.path + tmpSuffix
	// FSYNC BEFORE RENAME, as the metrics rollup does: this sidecar is the only
	// copy of when each participant joined, and a torn write here loses that for
	// good rather than losing a number that can be recomputed.
	if err := writeFileSync(tmp, append(b, '\n')); err != nil {
		_ = os.Remove(tmp)
		r.remarkDirty()
		return err
	}
	if err := os.Rename(tmp, r.path); err != nil {
		r.remarkDirty()
		return err
	}
	return nil
}

// remarkDirty puts the flag back after a failed write, so the next tick retries
// rather than treating the loss as saved.
func (r *recognitionStore) remarkDirty() {
	r.mu.Lock()
	r.dirty = true
	r.mu.Unlock()
}

func sortedRecognitionEntries(peers map[string]recognitionEntry) []recognitionEntry {
	out := make([]recognitionEntry, 0, len(peers))
	for _, e := range peers {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Slot != out[j].Slot {
			return out[i].Slot < out[j].Slot
		}
		return out[i].PeerID < out[j].PeerID
	})
	return out
}
