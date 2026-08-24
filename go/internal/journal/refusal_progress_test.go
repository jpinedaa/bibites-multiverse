package journal

import (
	"reflect"
	"testing"
	"time"
)

// TestRefusalProgressSurvivesCompactionAndReplay pins the internal durability
// contract for a relay transport refusal. The deadline and tried destinations
// are one migration's state, not process-local scheduler state.
func TestRefusalProgressSurvivesCompactionAndReplay(t *testing.T) {
	dir := t.TempDir()
	j, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	const id = "refusal-progress"
	entry := sampleEntry(id)
	entry.DestSlot = 2
	if _, err := j.Create(Out, entry, false); err != nil {
		t.Fatal(err)
	}

	deadline := time.Date(2026, 8, 23, 12, 0, 20, 0, time.UTC).UnixMilli()
	proof := "relay_never_forwarded"
	if _, err := j.Apply(id, Update{
		Handoff:           HandoffRefused,
		RerouteProof:      &proof,
		RefusedSlots:      []int{2},
		RefusalDeadlineMs: &deadline,
	}); err != nil {
		t.Fatal(err)
	}

	// A re-route changes the destination and handoff. It does not replace the
	// first refusal deadline or forget the transport queue already tried.
	dest, count, from := 3, 1, 2
	at := deadline - int64(15*time.Second/time.Millisecond)
	if _, err := j.Apply(id, Update{
		Handoff:      HandoffPending,
		DestSlot:     &dest,
		RerouteCount: &count,
		RerouteFrom:  &from,
		RerouteProof: &proof,
		RerouteAtMs:  &at,
		RefusedSlots: []int{2},
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := j.Compact(); err != nil {
		t.Fatal(err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}

	replayed, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer replayed.Close()
	st, ok := replayed.Get(id)
	if !ok {
		t.Fatal("the refusal entry disappeared during compaction and replay")
	}
	if st.RefusalDeadlineMs != deadline {
		t.Fatalf("refusal deadline = %d, want first deadline %d", st.RefusalDeadlineMs, deadline)
	}
	if !reflect.DeepEqual(st.RefusedSlots, []int{2}) {
		t.Fatalf("refused slots = %v, want [2]", st.RefusedSlots)
	}
	if st.Entry.DestSlot != 3 || st.Handoff != HandoffPending {
		t.Fatalf("replayed progress = dest %d handoff %q, want dest 3 pending",
			st.Entry.DestSlot, st.Handoff)
	}
}
