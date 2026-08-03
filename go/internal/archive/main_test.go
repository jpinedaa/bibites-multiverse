package archive

import (
	"bytes"
	"strings"
	"testing"

	"multiverse/internal/bb8"
	"multiverse/internal/contractb"
)

// TestListGapReportCountsOnlyUnresolvedHashes covers contract-b-m3.md §10.
//
// The gap report is the archive's honest statement of what it does not have and
// could still fetch. A lineage entry with NO hash — `gapReason: "parent_gone"`,
// which contract-a.md §5.3 makes the normal shape of every hop after the first —
// is not an unresolved hash: nothing was recorded to resolve and no retry can
// produce one. Counting it as a gap put every ordinary migration in the report.
func TestListGapReportCountsOnlyUnresolvedHashes(t *testing.T) {
	dir := t.TempDir()
	ledger, err := OpenLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	store, err := bb8.OpenStore(dir + "/genomes")
	if err != nil {
		t.Fatal(err)
	}

	const held = "bb8-genome/1:sha256:1111111111111111111111111111111111111111111111111111111111111111"
	const absent = "bb8-genome/1:sha256:2222222222222222222222222222222222222222222222222222222222222222"
	if err := store.Put(held, "0.6.3.1", `{"version":"0.6.3.1"}`); err != nil {
		t.Fatal(err)
	}

	// 1. A hop whose migrant genome is held and whose only parent is a
	//    parent_gone gap. This is the ordinary second hop of a ring circuit and
	//    it must NOT appear in the gap report.
	if err := ledger.Append(Record{
		Type: RecordMigration, MigrationID: "m-ordinary", EntityID: 7,
		Lineage: &contractb.Lineage{GenomeHash: held, Parents: []contractb.Parent{
			{EntityID: 9, GapReason: contractb.GapParentGone}}},
	}); err != nil {
		t.Fatal(err)
	}
	// 2. A hop naming a parent hash the archive does not hold. This IS the gap.
	if err := ledger.Append(Record{
		Type: RecordMigration, MigrationID: "m-unresolved", EntityID: 8,
		Lineage: &contractb.Lineage{GenomeHash: held, Parents: []contractb.Parent{
			{EntityID: 9, GenomeHash: absent}}},
	}); err != nil {
		t.Fatal(err)
	}
	// 3. An unhashable migrant (contract-b-m3.md §6.6): the hash is the empty
	//    string, it is counted separately, and it is never a fetchable gap.
	if err := ledger.Append(Record{
		Type: RecordMigration, MigrationID: "m-unhashable", EntityID: 10,
		Lineage: &contractb.Lineage{GenomeHash: "", Parents: []contractb.Parent{}},
	}); err != nil {
		t.Fatal(err)
	}
	_ = ledger.Close()

	var out, errOut bytes.Buffer
	if code := Main([]string{"list", "--data-dir", dir}, &out, &errOut); code != 0 {
		t.Fatalf("list exited %d: %s", code, errOut.String())
	}
	summary := out.String()
	if !strings.Contains(summary, "3 migration(s) shown, 1 with an unresolved genome hash, 1 whose migrant genome would not hash") {
		t.Fatalf("summary line is wrong:\n%s", summary)
	}

	out.Reset()
	if code := Main([]string{"list", "--data-dir", dir, "--gaps"}, &out, &errOut); code != 0 {
		t.Fatalf("list --gaps exited %d: %s", code, errOut.String())
	}
	gapsOnly := out.String()
	if !strings.Contains(gapsOnly, "m-unresolved") {
		t.Fatalf("--gaps dropped the one migration with an unresolved hash:\n%s", gapsOnly)
	}
	if strings.Contains(gapsOnly, "m-ordinary") {
		t.Fatalf("--gaps listed a parent_gone entry as a gap:\n%s", gapsOnly)
	}
}
