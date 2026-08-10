//go:build unix

package archive

import (
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// TestShortAppendLeavesNoPartialRecord is the write half of the 2026-08-08
// failure, reproduced rather than simulated.
//
// RLIMIT_FSIZE makes the kernel do to one append exactly what the full volume
// did to every append that night: it writes as many bytes as it can, refuses the
// rest, and returns an error. The record must not be recorded and — the part
// that cost the deployment eight hours of custody and the archive 390,005
// records — its bytes must not stay in the file, because the next append behind
// them would splice two records into one line no replay can parse.
//
// The limit is process-wide, so it is held for exactly one Write and restored
// immediately; SIGXFSZ is ignored for the same window because the kernel raises
// it on the offending thread.
func TestShortAppendLeavesNoPartialRecord(t *testing.T) {
	dir := t.TempDir()
	ledger, err := OpenLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	mustAppend(t, ledger, "before")

	path := filepath.Join(dir, ledgerName)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// A limit a few bytes past the end of the file: the next record starts,
	// crosses it, and is cut off in the middle.
	long := Record{Type: RecordMigration, MigrationID: strings.Repeat("s", 512), EntityID: 2}
	err = withFileSizeLimit(t, int64(len(before))+8, func() error { return ledger.Append(long) })
	if err == nil {
		t.Fatal("an append the kernel truncated returned no error")
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("a failed append left %d byte(s) behind:\n%q", len(after)-len(before),
			after[min(len(before), len(after)):])
	}

	// And the ledger is still usable: the next append lands on a clean boundary
	// and replay reads both records with nothing to report.
	mustAppend(t, ledger, "after")
	recs, damage, err := ReadLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(ids(recs), ","); got != "before,after" {
		t.Fatalf("replay read %q, want \"before,after\"", got)
	}
	if damage != (LedgerDamage{}) {
		t.Fatalf("a ledger that only ever refused a write reports damage: %+v", damage)
	}
}

func withFileSizeLimit(t *testing.T, limit int64, fn func() error) error {
	t.Helper()
	var old syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_FSIZE, &old); err != nil {
		t.Skipf("RLIMIT_FSIZE is unavailable: %v", err)
	}
	signal.Ignore(syscall.SIGXFSZ)
	defer signal.Reset(syscall.SIGXFSZ)
	if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE,
		&syscall.Rlimit{Cur: uint64(limit), Max: old.Max}); err != nil {
		t.Skipf("RLIMIT_FSIZE cannot be lowered here: %v", err)
	}
	defer func() {
		if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &old); err != nil {
			t.Fatalf("the file size limit was not restored: %v", err)
		}
	}()
	return fn()
}
