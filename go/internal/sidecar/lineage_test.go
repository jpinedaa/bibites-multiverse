package sidecar

// WP4's sidecar and archive half of Contract Change 11 (contract-a.md §21,
// A49): `parents[].blobDroppedForSize`, and the third `gapReason` value
// becoming reachable after two milestones in which it was defined and never
// emitted.
//
// THE FACT UNDER TEST IS A DISTINCTION, not a field. A dead parent and a parent
// whose blob the mod dropped to fit the frame arrive on this wire IDENTICALLY —
// an entityId with no payload — and until contract-a/2.4 the sidecar recorded
// both as "parent_gone". After M5 the archive's gap and the mod's log line are
// on two different people's computers, so "correlate them" stops being work and
// becomes a request to a stranger, on a question whose answer decides whether
// Risk 7's fetch ladder is chasing something that exists.

import (
	"testing"
	"time"

	"multiverse/internal/contracta"
	"multiverse/internal/contractb"
)

// TestASizeDroppedParentIsRecordedApartFromADeadOne is A49's whole point, end
// to end: one frame, two blobless parents, two different gap reasons, and the
// archive's ledger holding both.
func TestASizeDroppedParentIsRecordedApartFromADeadOne(t *testing.T) {
	g := newGrid(t, 2, gridOptions{layout: layoutRow(2)})
	one, two := g.node(0), g.node(1)
	arc := startArchive(t, g.relay)

	const droppedParent int32 = -1180911975
	const deadParent int32 = 204418833
	dropped, dead := droppedParent, deadParent

	migrationID := one.mod.migrateOutParents(testEntityID, contracta.EdgeE, 0.5,
		[]contracta.ParentBlob{
			// ALIVE AND SERIALIZED, and the largest thing in the frame: §5.3's
			// drop rule took it, largest first, and A49 is why the frame now says
			// so.
			{EntityID: &dropped, BlobDroppedForSize: true},
			// An ordinary gap: the GameObject is gone, the entity ID came from the
			// component's parent2ID, and no blob ever existed to drop.
			{EntityID: &dead},
		})
	waitFor(t, 10*time.Second, "the migration to complete", func() bool {
		return two.world.spawnCount(migrationID) == 1
	})
	waitFor(t, 15*time.Second, "the archive to record the migration", func() bool {
		list, err := arc.Records()
		if err != nil {
			return false
		}
		for _, rec := range list {
			if rec.MigrationID == migrationID && rec.Lineage != nil {
				return true
			}
		}
		return false
	})

	list, err := arc.Records()
	if err != nil {
		t.Fatalf("archive records: %v", err)
	}
	var found bool
	for _, rec := range list {
		if rec.MigrationID != migrationID || rec.Lineage == nil {
			continue
		}
		found = true
		if len(rec.Lineage.Parents) != 2 {
			t.Fatalf("the ledger recorded %d parents, want 2", len(rec.Lineage.Parents))
		}
		first, second := rec.Lineage.Parents[0], rec.Lineage.Parents[1]
		// THE ARCHIVE'S LEDGER reason FOR A SIZE-DROPPED PARENT IS NO LONGER
		// parent_gone. The archive changed nothing to make that true: it records
		// the annex verbatim, and the SIDECAR is what finally has the two facts
		// apart (§10, §15 B10).
		if first.GapReason != contractb.GapBlobDroppedForSize {
			t.Fatalf("the dropped parent recorded gapReason %q, want %q",
				first.GapReason, contractb.GapBlobDroppedForSize)
		}
		if first.GenomeHash != "" {
			t.Fatalf("a dropped blob produced a hash (%q); there was no blob to hash", first.GenomeHash)
		}
		if second.GapReason != contractb.GapParentGone {
			t.Fatalf("the dead parent recorded gapReason %q, want %q",
				second.GapReason, contractb.GapParentGone)
		}
	}
	if !found {
		t.Fatal("the archive has no record for the migration")
	}
}

// TestAnAbsentFlagIsNotFalseWithConfidence. Every contract-a/2.3 mod says
// nothing about every entry, and A49 is explicit that a reader MUST NOT read
// absence as false-with-confidence: it reads as the "parent_gone" the sidecar
// has always recorded, which is exactly the behaviour that must not change for
// the fleet that has not upgraded.
func TestAnAbsentFlagIsNotFalseWithConfidence(t *testing.T) {
	g := newGrid(t, 2, gridOptions{layout: layoutRow(2)})
	one, two := g.node(0), g.node(1)

	const gone int32 = 204418833
	dead := gone
	migrationID := one.mod.migrateOutParents(testEntityID, contracta.EdgeE, 0.5,
		[]contracta.ParentBlob{{EntityID: &dead}})
	waitFor(t, 10*time.Second, "the migration to complete", func() bool {
		return two.world.spawnCount(migrationID) == 1
	})

	for _, st := range one.side.CustodySnapshot() {
		if st.Entry.MigrationID != migrationID {
			continue
		}
		if len(st.Entry.Parents) != 1 {
			t.Fatalf("the journal recorded %d parents, want 1", len(st.Entry.Parents))
		}
		if st.Entry.Parents[0].GapReason != contractb.GapParentGone {
			t.Fatalf("a blobless parent with no flag recorded %q, want %q",
				st.Entry.Parents[0].GapReason, contractb.GapParentGone)
		}
		return
	}
	t.Fatal("the journal has no entry for the migration")
}

// TestTheFlagBesideAPayloadIsAModDefectAndTheBlobWins is A49's "where it may
// not appear" row: a true beside a present payload is a mod defect, the sidecar
// logs one warning and treats the entry as the ordinary blob-bearing parent it
// is — BECAUSE THE BLOB IS THE FACT AND THE FLAG IS THE LABEL.
func TestTheFlagBesideAPayloadIsAModDefectAndTheBlobWins(t *testing.T) {
	g := newGrid(t, 2, gridOptions{layout: layoutRow(2)})
	one, two := g.node(0), g.node(1)

	const livingParent int32 = -1180911975
	living := livingParent
	blob := makePayload(livingParent)
	hash := genomeHashOf(t, blob)

	migrationID := one.mod.migrateOutParents(testEntityID, contracta.EdgeE, 0.5,
		[]contracta.ParentBlob{
			{EntityID: &living, Payload: blob, GameVersion: "0.6.3.1", BlobDroppedForSize: true},
		})
	waitFor(t, 10*time.Second, "the migration to complete", func() bool {
		return two.world.spawnCount(migrationID) == 1
	})

	for _, st := range one.side.CustodySnapshot() {
		if st.Entry.MigrationID != migrationID {
			continue
		}
		if len(st.Entry.Parents) != 1 {
			t.Fatalf("the journal recorded %d parents, want 1", len(st.Entry.Parents))
		}
		if st.Entry.Parents[0].GapReason != "" {
			t.Fatalf("a blob-bearing parent was recorded as a gap (%q); the flag was believed "+
				"over the blob", st.Entry.Parents[0].GapReason)
		}
		if st.Entry.Parents[0].GenomeHash != hash {
			t.Fatalf("parent hash = %q, want %q", st.Entry.Parents[0].GenomeHash, hash)
		}
		if !one.side.Genomes().Has(hash) {
			t.Fatal("the blob was not cached; a mislabelled parent still ships a genome")
		}
		return
	}
	t.Fatal("the journal has no entry for the migration")
}
