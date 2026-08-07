package sidecar

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"multiverse/internal/archive"
	"multiverse/internal/contracta"
	"multiverse/internal/journal"
	"multiverse/internal/wire"
)

// awkwardSpecies is a legal block whose names are chosen to break any hop that
// re-encodes instead of copying: non-ASCII in both halves, an embedded quote,
// and the three characters Go's JSON encoder escapes to < > &.
// contract-a.md §5.7 matches on an exact ordinal comparison, so a hop that
// "tidies" any of this founds a second species in the destination world that
// reads identically to the first one in every UI that shows it.
func awkwardSpecies() *contracta.Species {
	return &contracta.Species{
		GenericName:        "Cyanëa<&>",
		SpecificName:       `velox"íssima`,
		ParentGenericName:  "Cyanëa",
		ParentSpecificName: "prīma",
	}
}

func speciesOf(t *testing.T, s *Sidecar, migrationID string) *contracta.Species {
	t.Helper()
	return journalEntry(t, s, migrationID).Entry.Species
}

// waitMigrateIn waits for the MIGRATE_IN that carries one specific migrationId,
// which a test that ships several organisms through one mod needs. It returns
// the raw envelope too, because "the key is not on the frame at all" is a
// different claim from "the decoded field is nil" and both are under test.
func waitMigrateIn(t *testing.T, m *fakeMod, from int, migrationID string) (contracta.MigrateIn, wire.Envelope) {
	t.Helper()
	env, _ := m.waitFrom(from, 15*time.Second, func(e wire.Envelope) bool {
		if e.Type != contracta.TypeMigrateIn {
			return false
		}
		var in contracta.MigrateIn
		return json.Unmarshal(e.Data, &in) == nil && in.MigrationID == migrationID
	})
	return decodeAs[contracta.MigrateIn](t, env), env
}

// hasSpeciesKey reports whether `data` carries a TOP-LEVEL species key. It looks
// at the frame's own bytes on purpose: the bb8 blob contains the string
// "speciesID", so a substring search over the frame would answer yes for every
// migration ever sent.
func hasSpeciesKey(t *testing.T, env wire.Envelope) bool {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(env.Data, &fields); err != nil {
		t.Fatalf("re-read %s data: %v", env.Type, err)
	}
	_, ok := fields["species"]
	return ok
}

func nackCodeFor(w *fakeWorld, migrationID string) string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.nackedOut[migrationID]
}

// TestSpeciesBlockCrossesTheRigByteIdentical is the end-to-end statement of
// contract-a.md §16 A30 and contract-b-m4.md §15 B9: the block leaves one mod
// and reaches the other one UNCHANGED, and every Go component between them
// carried it without reading a meaning out of it.
//
// The assertion is on all four names at once, and on the journal at BOTH ends,
// because that is where the block has to be for the organism to survive a
// restart with its identity attached.
func TestSpeciesBlockCrossesTheRigByteIdentical(t *testing.T) {
	g := newGrid(t, 2, gridOptions{layout: layoutRow(2)})
	a, b := g.node(0), g.node(1)
	want := awkwardSpecies()

	migrationID := a.mod.migrateOutSpecies(testEntityID, contracta.EdgeE, 0.5, want)

	in, _ := waitMigrateIn(t, b.mod, 0, migrationID)
	if in.Species == nil {
		t.Fatal("MIGRATE_IN carried no species block; the wire lost it")
	}
	if *in.Species != *want {
		t.Fatalf("the species block changed in flight:\n got %+v\nwant %+v", *in.Species, *want)
	}
	// The block rides the ENVELOPE, not the body: the payload is untouched and
	// carries only the origin's world-local speciesID, which is the defect §16
	// closes rather than the answer to it.
	if in.Payload != makePayload(testEntityID) {
		t.Fatal("the blob changed in flight")
	}

	// Both journals hold it, so both ends survive a restart with the identity.
	if got := speciesOf(t, a.side, migrationID); got == nil || *got != *want {
		t.Fatalf("the SOURCE journal entry holds %+v, want %+v", got, want)
	}
	if got := speciesOf(t, b.side, migrationID); got == nil || *got != *want {
		t.Fatalf("the DESTINATION journal entry holds %+v, want %+v", got, want)
	}

	waitFor(t, 10*time.Second, "the migration to complete", func() bool {
		return b.world.spawnCount(migrationID) == 1 &&
			custodyOf(a.side, migrationID) == "out/done"
	})
}

// TestAbsentSpeciesBlockStaysAbsent pins the other half of A30: an absent block
// is CONFORMANT, and nothing in the chain may invent one. A synthesized name
// would be worse than no name — it would look authoritative — and the importer's
// absent-block rule (A32) is a defined, better-than-before behaviour that only
// works if absent really is absent.
func TestAbsentSpeciesBlockStaysAbsent(t *testing.T) {
	g := newGrid(t, 2, gridOptions{layout: layoutRow(2)})
	a, b := g.node(0), g.node(1)

	migrationID := a.mod.migrateOut(testEntityID, contracta.EdgeE, 0.5)

	in, env := waitMigrateIn(t, b.mod, 0, migrationID)
	if in.Species != nil {
		t.Fatalf("a migration with no species block was delivered one: %+v", in.Species)
	}
	// The field must be OMITTED, not sent as an empty object: "absent" and "a
	// block with two empty names" are different statements and only one of them
	// is legal.
	if hasSpeciesKey(t, env) {
		t.Fatalf("MIGRATE_IN carried a species key with nothing in it: %s", env.Data)
	}
	if got := speciesOf(t, a.side, migrationID); got != nil {
		t.Fatalf("the source journal invented a species block: %+v", got)
	}
	if got := speciesOf(t, b.side, migrationID); got != nil {
		t.Fatalf("the destination journal invented a species block: %+v", got)
	}
}

// TestMalformedSpeciesBlockIsStrippedAtTheFirstHop is the rule that keeps a
// label from ever costing an organism (contract-a.md §16 A30, §9.3's one named
// exception; contract-b-m4.md §15 B9).
//
// Every shape here would be a MALFORMED_MESSAGE NACK for any other `data` field.
// For this one the sidecar strips the block, logs one line, and the migration
// COMPLETES — so the test asserts three things together: no NACK, no block on
// the far side, and one log line naming the reason.
func TestMalformedSpeciesBlockIsStrippedAtTheFirstHop(t *testing.T) {
	spy := newLogSpy(t)
	g := newGrid(t, 2, gridOptions{
		layout: layoutRow(2),
		tune: func(i int, c *Config) {
			if i == 0 {
				c.Logger = spy.logger()
			}
		},
	})
	a, b := g.node(0), g.node(1)

	cases := []struct {
		name       string
		raw        string
		entityID   int32
		wantReason string
	}{{
		name:       "a missing half",
		raw:        `{"genericName":"Cyanea"}`,
		entityID:   -843827001,
		wantReason: "species.specificName is absent",
	}, {
		name:       "an over-long half",
		raw:        `{"genericName":"` + strings.Repeat("a", 65) + `","specificName":"velox"}`,
		entityID:   -843827002,
		wantReason: "over the 64 limit",
	}, {
		name:       "a lone parent field",
		raw:        `{"genericName":"Cyanea","specificName":"velox","parentGenericName":"Cyanea"}`,
		entityID:   -843827003,
		wantReason: "the parent pair is all-or-nothing",
	}, {
		name:       "a non-string half",
		raw:        `{"genericName":41,"specificName":"velox"}`,
		entityID:   -843827004,
		wantReason: "not an object of four string fields",
	}, {
		name:       "a block that is not an object",
		raw:        `"Cyanea velox"`,
		entityID:   -843827005,
		wantReason: "not an object of four string fields",
	}}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			before := b.mod.frameCount()
			migrationID := a.mod.migrateOutRawSpecies(c.entityID, contracta.EdgeE, 0.5, c.raw)

			// THE MIGRATION STILL COMPLETES. That is the whole rule.
			in, env := waitMigrateIn(t, b.mod, before, migrationID)
			if code := nackCodeFor(a.world, migrationID); code != "" {
				t.Fatalf("a malformed species block produced MIGRATE_OUT_NACK %s; "+
					"§16 A30 forbids answering one with a NACK", code)
			}
			if in.Species != nil || hasSpeciesKey(t, env) {
				t.Fatalf("a malformed block reached the far mod: %s", env.Data)
			}
			if in.EntityID != c.entityID {
				t.Fatalf("delivered entity %d, want %d", in.EntityID, c.entityID)
			}
			if got := speciesOf(t, a.side, migrationID); got != nil {
				t.Fatalf("a malformed block was journaled: %+v", got)
			}
			if got := speciesOf(t, b.side, migrationID); got != nil {
				t.Fatalf("a malformed block was journaled at the destination: %+v", got)
			}
			// The strip is LOUD. A silent one leaves an operator with an organism
			// that quietly lost its name and no way to find out why.
			if spy.count("stripping a malformed species block") == 0 {
				t.Fatal("the strip was silent; §16 A30 requires one log line")
			}
			if spy.count(c.wantReason) == 0 {
				t.Fatalf("no log line named the reason %q", c.wantReason)
			}
			waitFor(t, 10*time.Second, "the stripped migration to complete", func() bool {
				return b.world.spawnCount(migrationID) == 1
			})
		})
	}
}

// TestSpeciesSurvivesAJournalRestartAndIsReplayed is the durability claim, run
// against a real restart rather than a unit round trip.
//
// The destination journals the organism and its block, the sidecar process
// dies, and the replay of contract-a.md §7.5 has to hand the mod the SAME
// block. A block held only in memory would leave the organism arriving under no
// name at all after any restart — and a restart is exactly when a rig's
// operator is least likely to notice.
func TestSpeciesSurvivesAJournalRestartAndIsReplayed(t *testing.T) {
	g := newGrid(t, 2, gridOptions{layout: layoutRow(2)})
	a, b := g.node(0), g.node(1)
	b.mod.setAckMode(ackSilent)
	want := awkwardSpecies()

	migrationID := a.mod.migrateOutSpecies(testEntityID, contracta.EdgeE, 0.5, want)
	waitFor(t, 10*time.Second, "the destination to journal the organism", func() bool {
		return custodyOf(b.side, migrationID) == "in/in_flight"
	})
	if got := speciesOf(t, b.side, migrationID); got == nil || *got != *want {
		t.Fatalf("the destination journaled %+v, want %+v", got, want)
	}

	cfg := b.cfg
	if err := b.side.Close(); err != nil {
		t.Fatalf("close the destination sidecar: %v", err)
	}
	reopened := startSidecar(t, cfg)
	slot := waitSlotAny(t, reopened)
	if got := speciesOf(t, reopened, migrationID); got == nil || *got != *want {
		t.Fatalf("the block did not survive the journal round trip: got %+v, want %+v", got, want)
	}

	// The replay is the real test: a fresh mod connects and the sidecar has to
	// deliver the organism again, block included.
	mod2 := dialFakeMod(t, fakeModOptions{
		url: reopened.URL(), world: newWorld(), ringSlot: &slot,
		heartbeat: 200 * time.Millisecond})
	in, _ := waitMigrateIn(t, mod2, 0, migrationID)
	if in.Species == nil || *in.Species != *want {
		t.Fatalf("the replayed MIGRATE_IN carried %+v, want %+v", in.Species, want)
	}
}

// TestBounceBackCarriesTheSpeciesHome covers §9.4 with the block attached: an
// organism that comes home comes home UNDER THE NAME IT LEFT WITH.
//
// A bounce is not a new migration — it is the same journal entry with its
// direction flipped — so the block has to ride the same record all the way back
// into the origin's own mod. Losing it here would be the quiet case: the
// organism reappears in the world it left, and joins a different species than
// the one it was in a minute earlier.
func TestBounceBackCarriesTheSpeciesHome(t *testing.T) {
	relaySrv := startRelay(t)
	dataDir := t.TempDir()
	want := awkwardSpecies()
	migrationID := seedOutboundCustody(t, dataDir, want)

	cfg := fastConfig(t, relaySrv.url(), "peer-a")
	cfg.DataDir = dataDir
	sideA := startSidecar(t, cfg)
	waitSlot(t, sideA, 1)

	world := newWorld()
	modA := dialFakeMod(t, fakeModOptions{
		url: sideA.URL(), world: world, heartbeat: 200 * time.Millisecond})

	in, _ := waitMigrateIn(t, modA, 0, migrationID)
	if !in.BounceBack {
		t.Fatal("a bounced delivery must carry bounceBack = true")
	}
	if in.Species == nil || *in.Species != *want {
		t.Fatalf("the bounce dropped the species block: got %+v, want %+v", in.Species, want)
	}
	waitFor(t, 10*time.Second, "the bounced organism to be alive again", func() bool {
		return world.spawnCount(migrationID) == 1
	})
}

// TestArchiveRecordsTheSpeciesBlockVerbatim is contract-b-m4.md §15, B10.
//
// The record is a LEDGER FACT, never a resolution: the archive writes the two
// names down exactly as they crossed and does not resolve, merge or rewrite
// them, because species resolution happens in exactly one place in this system
// and it is the importing mod. The second half of the test is the one that is
// easy to get wrong — a migration with NO block records no block, rather than
// "unknown" as a value.
func TestArchiveRecordsTheSpeciesBlockVerbatim(t *testing.T) {
	g := newGrid(t, 2, gridOptions{layout: layoutRow(2)})
	a, b := g.node(0), g.node(1)
	arc := startArchive(t, g.relay.url())
	want := awkwardSpecies()

	named := a.mod.migrateOutSpecies(testEntityID, contracta.EdgeE, 0.5, want)
	waitFor(t, 10*time.Second, "the named migration to arrive", func() bool {
		return b.world.spawnCount(named) == 1
	})
	nameless := a.mod.migrateOut(-843827999, contracta.EdgeE, 0.5)
	waitFor(t, 10*time.Second, "the nameless migration to arrive", func() bool {
		return b.world.spawnCount(nameless) == 1
	})

	var namedRec, namelessRec *archive.Record
	waitFor(t, 10*time.Second, "the archive to record both migrations", func() bool {
		records, err := arc.Records()
		if err != nil {
			return false
		}
		namedRec, namelessRec = nil, nil
		for i := range records {
			rec := &records[i]
			if rec.Type != archive.RecordMigration {
				continue
			}
			switch rec.MigrationID {
			case named:
				namedRec = rec
			case nameless:
				namelessRec = rec
			}
		}
		return namedRec != nil && namelessRec != nil
	})

	if namedRec.Species == nil {
		t.Fatal("the archive recorded a migration that carried a species block without it")
	}
	if *namedRec.Species != *want {
		t.Fatalf("the ledger changed the names:\n got %+v\nwant %+v", *namedRec.Species, *want)
	}
	if namelessRec.Species != nil {
		t.Fatalf("the archive invented a block for a nameless migration: %+v", namelessRec.Species)
	}

	// The ledger line itself: absent is absent, and the field is simply not
	// there. jq and grep are the archive's first read path (store.go), so the
	// shape on disk is part of the contract with an operator.
	line := ledgerLineFor(t, arc, nameless)
	if strings.Contains(line, "species") {
		t.Fatalf("the ledger line for a nameless migration carries a species key: %s", line)
	}
	line = ledgerLineFor(t, arc, named)
	if !strings.Contains(line, `"genericName"`) {
		t.Fatalf("the ledger line for a named migration carries no species block: %s", line)
	}
}

// ledgerLineFor returns the raw migrations.jsonl line for one migration, so a
// test can assert on the BYTES an operator's jq will see.
func ledgerLineFor(t *testing.T, arc *archive.Archive, migrationID string) string {
	t.Helper()
	records, err := arc.Records()
	if err != nil {
		t.Fatalf("archive records: %v", err)
	}
	for _, rec := range records {
		if rec.Type == archive.RecordMigration && rec.MigrationID == migrationID {
			b, err := json.Marshal(rec)
			if err != nil {
				t.Fatalf("re-marshal ledger record: %v", err)
			}
			return string(b)
		}
	}
	t.Fatalf("no ledger record for %s", migrationID)
	return ""
}

// TestSpeciesIsNotPartOfTheGenomeHash keeps the two identities apart, which is
// why B9 puts the block at the top level and NOT inside the lineage annex
// (genome-hash.md §4.3 excludes genes.speciesID from the canonical projection).
//
// If a name ever reached the hash, the rewrite the destination mod performs on
// $.genes.speciesID before the restore (A31) would invalidate a hash the source
// sidecar already computed, and every lineage join in the archive would break on
// exactly the organisms that migrated.
func TestSpeciesIsNotPartOfTheGenomeHash(t *testing.T) {
	g := newGrid(t, 2, gridOptions{layout: layoutRow(2)})
	a, b := g.node(0), g.node(1)
	payload := makePayload(testEntityID)
	wantHash := genomeHashOf(t, payload)

	withBlock := a.mod.migrateOutSpecies(testEntityID, contracta.EdgeE, 0.5, awkwardSpecies())
	waitFor(t, 10*time.Second, "the migration to arrive", func() bool {
		return b.world.spawnCount(withBlock) == 1
	})
	entry := journalEntry(t, b.side, withBlock)
	if entry.Entry.GenomeHash != wantHash {
		t.Fatalf("genomeHash = %s, want %s; the species block must not touch the annex",
			entry.Entry.GenomeHash, wantHash)
	}
	if entry.Direction != journal.In {
		t.Fatalf("direction = %s, want in", entry.Direction)
	}
}
