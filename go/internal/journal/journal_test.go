package journal

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"multiverse/internal/wire"
)

func sampleEntry(id string) Entry {
	return Entry{
		MigrationID:    id,
		EntityID:       -843827577,
		Kind:           "bibite",
		GameVersion:    "0.6.3.1",
		Payload:        `{"body":{"id":{"id":-843827577}},"version":"0.6.3.1"}`,
		PayloadHash:    "cafebabe",
		Edge:           "E",
		Position:       0.6031925,
		VelocityX:      6.12,
		VelocityY:      0.44,
		Heading:        274.11,
		SimulationSize: 2000,
		SimTick:        4820344,
		DestSlot:       2,
		JournaledAt:    time.Now().UnixMilli(),
	}
}

func TestCreateAndGet(t *testing.T) {
	j, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()

	st, err := j.Create(Out, sampleEntry("m1"), false)
	if err != nil {
		t.Fatal(err)
	}
	if st.Status != StatusOpen || st.Direction != Out {
		t.Fatalf("new state = %+v", st)
	}
	if _, err := j.Create(Out, sampleEntry("m1"), false); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("second Create error = %v, want ErrDuplicate", err)
	}
	got, ok := j.Get("m1")
	if !ok || got.Entry.Payload != sampleEntry("m1").Payload {
		t.Fatalf("Get = %+v ok=%v", got, ok)
	}
	if _, ok := j.Get("nope"); ok {
		t.Fatal("Get invented an entry")
	}
}

func TestApplyIsDurableAndReplayed(t *testing.T) {
	dir := t.TempDir()
	j, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := j.Create(In, sampleEntry("m1"), true); err != nil {
		t.Fatal(err)
	}
	attempt := 3
	if _, err := j.Apply("m1", Update{Status: StatusInFlight, Attempt: &attempt}); err != nil {
		t.Fatal(err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	st, ok := reopened.Get("m1")
	if !ok {
		t.Fatal("the entry did not survive a reopen")
	}
	if st.Status != StatusInFlight || st.Attempt != 3 || !st.BounceBack || st.Direction != In {
		t.Fatalf("replayed state = %+v", st)
	}
	if st.Entry.Payload != sampleEntry("m1").Payload {
		t.Fatal("the payload did not survive a reopen")
	}
}

func TestApplyUnknownMigration(t *testing.T) {
	j, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	if _, err := j.Apply("nope", Update{Status: StatusDone}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Apply error = %v, want ErrNotFound", err)
	}
}

func TestDoneDropsThePayloadButKeepsTheIdentity(t *testing.T) {
	dir := t.TempDir()
	j, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := j.Create(Out, sampleEntry("m1"), false); err != nil {
		t.Fatal(err)
	}
	at := time.Now().UnixMilli()
	st, err := j.Apply("m1", Update{Status: StatusDone, CompletedAt: &at})
	if err != nil {
		t.Fatal(err)
	}
	if st.Entry.Payload != "" {
		t.Fatal("a tombstone must not keep the payload")
	}
	// contract-a.md §7.2: the tombstone is what makes a late retry safe, so the
	// hash and the entity id must survive.
	if st.Entry.PayloadHash != "cafebabe" || st.Entry.EntityID != -843827577 {
		t.Fatalf("tombstone lost its identity: %+v", st.Entry)
	}
	_ = j.Close()

	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if got, ok := reopened.Get("m1"); !ok || got.Status != StatusDone || got.Entry.PayloadHash != "cafebabe" {
		t.Fatalf("tombstone after reopen = %+v ok=%v", got, ok)
	}
}

func TestListIsInJournalOrder(t *testing.T) {
	dir := t.TempDir()
	j, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"m1", "m2", "m3"} {
		if _, err := j.Create(In, sampleEntry(id), false); err != nil {
			t.Fatal(err)
		}
	}
	assertOrder := func(j *Journal) {
		t.Helper()
		got := []string{}
		for _, st := range j.List() {
			got = append(got, st.Entry.MigrationID)
		}
		if strings.Join(got, ",") != "m1,m2,m3" {
			t.Fatalf("List order = %v", got)
		}
	}
	assertOrder(j)
	_ = j.Close()

	// contract-a.md §7.5 replays in journal order, so compaction must preserve it.
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	assertOrder(reopened)
}

func TestTornTailIsDiscarded(t *testing.T) {
	dir := t.TempDir()
	j, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := j.Create(Out, sampleEntry("m1"), false); err != nil {
		t.Fatal(err)
	}
	_ = j.Close()

	// Simulate a kill -9 in the middle of the next append.
	path := filepath.Join(dir, fileName)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"op":"create","migrationId":"m2","entry":{"migrat`); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	reopened, err := Open(dir)
	if err != nil {
		t.Fatalf("Open after a torn write: %v", err)
	}
	defer reopened.Close()
	if _, ok := reopened.Get("m1"); !ok {
		t.Fatal("the complete record was lost")
	}
	if _, ok := reopened.Get("m2"); ok {
		t.Fatal("a torn record was accepted as durable")
	}
	// The log must be usable again straight away.
	if _, err := reopened.Create(Out, sampleEntry("m3"), false); err != nil {
		t.Fatalf("Create after a torn tail: %v", err)
	}
	if _, ok := reopened.Get("m3"); !ok {
		t.Fatal("the post-recovery record was lost")
	}
}

func TestCountPending(t *testing.T) {
	j, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	for _, id := range []string{"m1", "m2"} {
		if _, err := j.Create(In, sampleEntry(id), false); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := j.Create(Out, sampleEntry("m3"), false); err != nil {
		t.Fatal(err)
	}
	if got := j.CountPending(In); got != 2 {
		t.Fatalf("CountPending(In) = %d, want 2", got)
	}
	at := time.Now().UnixMilli()
	if _, err := j.Apply("m1", Update{Status: StatusDone, CompletedAt: &at}); err != nil {
		t.Fatal(err)
	}
	if got := j.CountPending(In); got != 1 {
		t.Fatalf("CountPending(In) after a tombstone = %d, want 1", got)
	}
	if got := j.CountPending(Out); got != 1 {
		t.Fatalf("CountPending(Out) = %d, want 1", got)
	}
}

func TestPurgeExpiredOnlyTakesOldTombstones(t *testing.T) {
	dir := t.TempDir()
	j, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"old", "fresh", "live"} {
		if _, err := j.Create(Out, sampleEntry(id), false); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	oldAt := now.Add(-2 * time.Hour).UnixMilli()
	freshAt := now.UnixMilli()
	if _, err := j.Apply("old", Update{Status: StatusDone, CompletedAt: &oldAt}); err != nil {
		t.Fatal(err)
	}
	if _, err := j.Apply("fresh", Update{Status: StatusDone, CompletedAt: &freshAt}); err != nil {
		t.Fatal(err)
	}
	n, err := j.PurgeExpired(time.Hour, now)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("purged %d entries, want 1", n)
	}
	if _, ok := j.Get("old"); ok {
		t.Fatal("the expired tombstone survived")
	}
	if _, ok := j.Get("fresh"); !ok {
		t.Fatal("a fresh tombstone was purged")
	}
	if _, ok := j.Get("live"); !ok {
		t.Fatal("a live entry was purged")
	}
	_ = j.Close()

	// The purge must be durable too, or §7.4 breaks after a restart.
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if _, ok := reopened.Get("old"); ok {
		t.Fatal("the purge was not durable")
	}
}

func TestPurgeExpiredKeepsAnInboundTombstoneUntilItsUpstreamAckLeaves(t *testing.T) {
	dir := t.TempDir()
	j, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"ack-pending", "ack-sent"} {
		entry := sampleEntry(id)
		entry.SourcePeer = "peer-source"
		if _, err := j.Create(In, entry, false); err != nil {
			t.Fatal(err)
		}
	}
	completed := time.Now().Add(-2 * time.Hour).UnixMilli()
	if _, err := j.Apply("ack-pending", Update{
		Status: StatusDone, CompletedAt: &completed,
	}); err != nil {
		t.Fatal(err)
	}
	acked := true
	if _, err := j.Apply("ack-sent", Update{
		Status: StatusDone, CompletedAt: &completed, Acked: &acked,
	}); err != nil {
		t.Fatal(err)
	}

	n, err := j.PurgeExpired(time.Hour, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("purged %d entries, want only the acknowledged tombstone", n)
	}
	if _, ok := j.Get("ack-pending"); !ok {
		t.Fatal("purge deleted an inbound tombstone before its upstream ACK left")
	}
	if _, ok := j.Get("ack-sent"); ok {
		t.Fatal("purge retained an expired inbound tombstone whose upstream ACK left")
	}
}

func TestCompactionShrinksTheLog(t *testing.T) {
	dir := t.TempDir()
	j, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := j.Create(Out, sampleEntry("m1"), false); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 40; i++ {
		attempt := i
		if _, err := j.Apply("m1", Update{Status: StatusInFlight, Attempt: &attempt}); err != nil {
			t.Fatal(err)
		}
	}
	_ = j.Close()
	before := fileSize(t, filepath.Join(dir, fileName))

	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	after := fileSize(t, filepath.Join(dir, fileName))
	if after >= before {
		t.Fatalf("compaction did not shrink the log: %d -> %d", before, after)
	}
	st, ok := reopened.Get("m1")
	if !ok || st.Attempt != 39 {
		t.Fatalf("compaction lost state: %+v ok=%v", st, ok)
	}
}

func fileSize(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Size()
}

// TestM3InFlightEntryReplaysAsSent pins the M3 -> M4 journal migration path.
//
// An M3 journal carries no `handoff` field. Its sidecar moved an outbound entry
// to in_flight only after MIGRATION_PAYLOAD had been written to a live relay
// connection, so that state means what M4 calls `sent`: custody MAY have moved.
// Replaying it as `pending` would say the frame reached nobody, and
// contract-b-m4.md §9.2 lets a pending entry re-route to a DIFFERENT slot with
// no proof of non-delivery — which is the duplication D2 refuses.
//
// Found by the M4 rehearsal, against the real T1 journal of 2026-08-04.
func TestM3InFlightEntryReplaysAsSent(t *testing.T) {
	dir := t.TempDir()
	// Two records in exactly M3's shape: a create with no handoff, and a status
	// with no handoff.
	lines := `{"op":"create","migrationId":"9d6db335-b1ae-433e-a44a-bb2109912913","at":1785852794196,` +
		`"direction":"out","entry":{"migrationId":"9d6db335-b1ae-433e-a44a-bb2109912913","entityId":2004967003,` +
		`"kind":"bibite","gameVersion":"0.6.3.1","payload":"{}","payloadHash":"c35a","edge":"E","position":0.22,` +
		`"velocityX":11.6,"velocityY":-5.7,"heading":-116,"simulationSize":2000,"simTick":48160940,` +
		`"sourceSlot":1,"destSlot":2,"journaledAt":1785852794196},"bounceBack":false}
{"op":"status","migrationId":"9d6db335-b1ae-433e-a44a-bb2109912913","at":1785852794201,"status":"in_flight"}
`
	if err := os.WriteFile(filepath.Join(dir, "journal.log"), []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	j, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer j.Close()

	st, ok := j.Get("9d6db335-b1ae-433e-a44a-bb2109912913")
	if !ok {
		t.Fatal("the M3 entry did not survive replay")
	}
	if st.Status != StatusInFlight {
		t.Fatalf("status = %q, want %q", st.Status, StatusInFlight)
	}
	if st.Handoff != HandoffSent {
		t.Fatalf("handoff = %q, want %q — an M3 in_flight entry was written to a live relay "+
			"connection, so custody MAY have moved", st.Handoff, HandoffSent)
	}
	if !st.Handoff.CustodyMayHaveMoved() {
		t.Fatal("the replayed entry reports that custody cannot have moved; §9.2 would re-route it")
	}
	if st.Entry.DestSlot != 2 {
		t.Fatalf("destSlot = %d, want the recorded 2", st.Entry.DestSlot)
	}
}

// TestM4InFlightEntryKeepsItsRecordedHandoff is the other half: an M4 record
// carries handoff explicitly and the M3 migration rule must never overwrite it.
//
// The recorded value here is `held`, which is the state §25's B37 retired, so
// this covers the SECOND migration in the same function: a pre-B37 journal
// replays its held entries as `sent`, because that is what `held` always meant —
// written to a live relay connection, custody may have moved. Replaying it as
// the literal string would leave the entry in no case of the sender's state
// machine: never resolved, never counted, never even logged.
func TestM4InFlightEntryKeepsItsRecordedHandoff(t *testing.T) {
	dir := t.TempDir()
	lines := `{"op":"create","migrationId":"11111111-1111-4111-8111-111111111111","at":1,` +
		`"direction":"out","entry":{"migrationId":"11111111-1111-4111-8111-111111111111","entityId":7,` +
		`"kind":"bibite","gameVersion":"0.6.3.1","payload":"{}","payloadHash":"aa","edge":"E","position":0.5,` +
		`"velocityX":1,"velocityY":0,"heading":0,"simulationSize":2000,"simTick":1,` +
		`"sourceSlot":1,"destSlot":2,"journaledAt":1},"bounceBack":false}
{"op":"status","migrationId":"11111111-1111-4111-8111-111111111111","at":2,"status":"in_flight","handoff":"held"}
`
	if err := os.WriteFile(filepath.Join(dir, "journal.log"), []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	j, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer j.Close()
	st, _ := j.Get("11111111-1111-4111-8111-111111111111")
	if st.Handoff != HandoffSent {
		t.Fatalf("handoff = %q, want %q — a recorded pre-B37 `held` replays as `sent`",
			st.Handoff, HandoffSent)
	}
	if !st.Handoff.CustodyMayHaveMoved() {
		t.Fatal("a replayed `held` entry reports that custody cannot have moved; §9.2 would re-route it")
	}
}

// TestSpeciesSurvivesReplayAndNoUpdateCanTouchIt covers the durability half of
// contract-a.md §16 A30 and contract-b-m4.md §15 B9.
//
// Two properties, and they are different. The first is ordinary: the block is on
// the immutable half of the record, so it survives a restart exactly as the
// payload does. The second is the load-bearing one — a RE-ROUTE rewrites
// destSlot and records its evidence beside it (§9.2), and the species block must
// come through that untouched, because a re-routed frame carries the same
// migrationId, the same body AND THE SAME BLOCK, byte for byte. Update has no
// species field at all, which is what makes that true by construction; this test
// is the guard against someone adding one.
func TestSpeciesSurvivesReplayAndNoUpdateCanTouchIt(t *testing.T) {
	dir := t.TempDir()
	j, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := wire.Species{
		GenericName: "Cyanëa", SpecificName: `velox"íssima`,
		ParentGenericName: "Cyanëa", ParentSpecificName: "prīma",
	}
	entry := sampleEntry("m-species")
	seed := want
	entry.Species = &seed
	if _, err := j.Create(Out, entry, false); err != nil {
		t.Fatal(err)
	}

	// The re-route of §9.2, exactly as the sidecar writes it.
	dest, count, from, at := 7, 1, 2, time.Now().UnixMilli()
	proof := "relay_never_forwarded"
	st, err := j.Apply("m-species", Update{
		Handoff: HandoffPending, DestSlot: &dest, RerouteCount: &count,
		RerouteFrom: &from, RerouteProof: &proof, RerouteAtMs: &at,
		RefusedSlots: []int{2, 5},
	})
	if err != nil {
		t.Fatal(err)
	}
	if st.Entry.DestSlot != 7 {
		t.Fatalf("destSlot = %d, want the re-routed 7", st.Entry.DestSlot)
	}
	if st.Entry.Species == nil || *st.Entry.Species != want {
		t.Fatalf("the re-route changed the species block: %+v", st.Entry.Species)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopen, which replays and compacts. Both paths have to carry the block.
	j2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer j2.Close()
	got, ok := j2.Get("m-species")
	if !ok {
		t.Fatal("the entry did not survive the restart")
	}
	if got.Entry.Species == nil {
		t.Fatal("the species block did not survive the journal round trip")
	}
	if *got.Entry.Species != want {
		t.Fatalf("the species block changed across the round trip:\n got %+v\nwant %+v",
			*got.Entry.Species, want)
	}
	if got.Entry.DestSlot != 7 {
		t.Fatalf("destSlot = %d after replay, want 7", got.Entry.DestSlot)
	}
	if !reflect.DeepEqual(got.RefusedSlots, []int{2, 5}) {
		t.Fatalf("refusedSlots = %v after replay and compaction, want [2 5]", got.RefusedSlots)
	}
}

// TestAnEntryWithNoSpeciesStaysThatWay: absent is absent, across a replay too.
// It is never filled in, never defaulted, and never becomes an empty block.
func TestAnEntryWithNoSpeciesStaysThatWay(t *testing.T) {
	dir := t.TempDir()
	j, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := j.Create(In, sampleEntry("m-plain"), false); err != nil {
		t.Fatal(err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, fileName))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "species") {
		t.Fatalf("an entry with no species block wrote one to the log:\n%s", raw)
	}
	j2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer j2.Close()
	got, ok := j2.Get("m-plain")
	if !ok {
		t.Fatal("the entry did not survive")
	}
	if got.Entry.Species != nil {
		t.Fatalf("an absent block was invented on replay: %+v", got.Entry.Species)
	}
}
