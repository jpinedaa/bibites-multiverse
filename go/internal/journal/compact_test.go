package journal

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// The periodic compaction of contract-b-m4.md §12 has three obligations, and
// each has a test here: it must not lose an entry while the sidecar is writing,
// it must actually shrink the file, and a crash in its one dangerous instant
// must cost nothing.

// TestCompactUnderConcurrentWritesKeepsEveryLiveEntry runs compactions against a
// journal that is being written the whole time.
//
// The value under test is AccruedHoldMs. It is not bookkeeping: contract-b-m4.md
// §9.3 makes the accrued dark time the thing that decides when a held organism
// bounces home, and it lives in the entry precisely because a restart must not
// lose time already served or invent time that was never served. A compaction
// that dropped it would silently reset every hold clock in the rig.
func TestCompactUnderConcurrentWritesKeepsEveryLiveEntry(t *testing.T) {
	dir := t.TempDir()
	j, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()

	const writers = 8
	const perWriter = 40
	want := map[string]int64{}
	var mu sync.Mutex

	stop := make(chan struct{})
	compactions := 0
	var compactWG sync.WaitGroup
	compactWG.Add(1)
	go func() {
		defer compactWG.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			if _, _, err := j.Compact(); err != nil {
				t.Errorf("Compact during writes: %v", err)
				return
			}
			compactions++
			time.Sleep(time.Millisecond)
		}
	}()

	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWriter; i++ {
				id := fmt.Sprintf("w%d-m%d", w, i)
				if _, err := j.Create(Out, sampleEntry(id), false); err != nil {
					t.Errorf("Create %s: %v", id, err)
					return
				}
				// Several accrual writes per entry, so a compaction is very
				// likely to land between two of them.
				var last int64
				for k := 1; k <= 3; k++ {
					last = int64(w*1000 + i*10 + k)
					h := HandoffHeld
					if _, err := j.Apply(id, Update{Status: StatusInFlight, Handoff: h,
						AccruedHoldMs: &last}); err != nil {
						t.Errorf("Apply %s: %v", id, err)
						return
					}
				}
				mu.Lock()
				want[id] = last
				mu.Unlock()
			}
		}(w)
	}
	wg.Wait()
	close(stop)
	compactWG.Wait()
	if compactions == 0 {
		t.Fatal("the compactor never ran, so this test proved nothing")
	}

	check := func(what string, get func(string) (*State, bool)) {
		t.Helper()
		for id, hold := range want {
			st, ok := get(id)
			if !ok {
				t.Fatalf("%s: entry %s vanished", what, id)
			}
			if st.AccruedHoldMs != hold {
				t.Fatalf("%s: entry %s accruedHoldMs = %d, want %d",
					what, id, st.AccruedHoldMs, hold)
			}
			if st.Handoff != HandoffHeld {
				t.Fatalf("%s: entry %s handoff = %q, want held", what, id, st.Handoff)
			}
			if st.Entry.Payload != sampleEntry(id).Payload {
				t.Fatalf("%s: entry %s lost its payload", what, id)
			}
		}
	}
	check("in memory", j.Get)

	// And the same after a reopen, which is the only proof that the compacted
	// FILE — not just the map — still holds every value.
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if got := reopened.Live(); got != len(want) {
		t.Fatalf("after reopen the journal holds %d entries, want %d", got, len(want))
	}
	check("after reopen", reopened.Get)
}

// TestCompactShrinksALiveJournal is the disk-budget claim itself: a journal that
// has churned through a thousand completed migrations must come back down to the
// size of what it still holds, without a restart.
func TestCompactShrinksALiveJournal(t *testing.T) {
	dir := t.TempDir()
	j, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()

	const done = 200
	for i := 0; i < done; i++ {
		id := fmt.Sprintf("done-%d", i)
		if _, err := j.Create(Out, sampleEntry(id), false); err != nil {
			t.Fatal(err)
		}
		completed := time.Now().Add(-2 * time.Hour).UnixMilli()
		if _, err := j.Apply(id, Update{Status: StatusDone, Handoff: HandoffDone,
			CompletedAt: &completed}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := j.Create(Out, sampleEntry("still-here"), false); err != nil {
		t.Fatal(err)
	}
	if _, err := j.PurgeExpired(time.Hour, time.Now()); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, fileName)
	grown := fileSize(t, path)
	before, after, err := j.Compact()
	if err != nil {
		t.Fatal(err)
	}
	if before != grown {
		t.Fatalf("Compact reported %d bytes before, the file was %d", before, grown)
	}
	if after >= before {
		t.Fatalf("compaction did not shrink the journal: %d -> %d", before, after)
	}
	if got := fileSize(t, path); got != after {
		t.Fatalf("Compact reported %d bytes after, the file is %d", after, got)
	}
	if j.Live() != 1 {
		t.Fatalf("live entries = %d, want 1", j.Live())
	}
	// The scratch file must not survive a successful compaction.
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("journal.log.tmp survived: %v", err)
	}

	// The journal is still writable through the reopened handle, which is the
	// half of Compact that a rename would otherwise break.
	if _, err := j.Create(Out, sampleEntry("after-compact"), false); err != nil {
		t.Fatalf("Create after Compact: %v", err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	for _, id := range []string{"still-here", "after-compact"} {
		if _, ok := reopened.Get(id); !ok {
			t.Fatalf("%s did not survive compaction and reopen", id)
		}
	}
	if _, ok := reopened.Get("done-0"); ok {
		t.Fatal("a purged tombstone came back")
	}
}

const crashDirEnv = "MULTIVERSE_JOURNAL_COMPACT_CRASH_DIR"

// TestCompactCrashBetweenScratchAndRenameLosesNothing SIGKILLs a real process in
// the exact instant compaction is dangerous: the scratch file written and
// fsynced, the rename not yet done. Nothing may be lost, because journal.log is
// still whole at that moment and the rename is atomic.
func TestCompactCrashBetweenScratchAndRenameLosesNothing(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=TestCompactCrashChild")
	cmd.Env = append(os.Environ(), crashDirEnv+"="+dir)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("the child exited cleanly, so it never crashed mid-compaction:\n%s", out)
	}

	// The scratch file is the evidence that the kill landed in the window: it
	// exists only between the fsync and the rename, and compact's cleanup
	// cannot have run, because SIGKILL runs no defers.
	if _, err := os.Stat(filepath.Join(dir, fileName+".tmp")); err != nil {
		t.Fatalf("no journal.log.tmp, so the child was not killed mid-compaction: %v\n%s", err, out)
	}

	j, err := Open(dir)
	if err != nil {
		t.Fatalf("the journal did not reopen after the crash: %v", err)
	}
	defer j.Close()
	if got := j.Live(); got != crashEntries {
		t.Fatalf("after a crash mid-compaction the journal holds %d entries, want %d", got, crashEntries)
	}
	for i := 0; i < crashEntries; i++ {
		id := fmt.Sprintf("crash-%d", i)
		st, ok := j.Get(id)
		if !ok {
			t.Fatalf("%s was lost by the crash", id)
		}
		if want := int64(i + 1); st.AccruedHoldMs != want {
			t.Fatalf("%s accruedHoldMs = %d, want %d", id, st.AccruedHoldMs, want)
		}
		if st.Entry.Payload != sampleEntry(id).Payload {
			t.Fatalf("%s lost its payload", id)
		}
	}
	// Reopening compacted, so the scratch file is gone and the log is small.
	if _, err := os.Stat(filepath.Join(dir, fileName+".tmp")); !os.IsNotExist(err) {
		t.Fatalf("the stale scratch file survived the reopen: %v", err)
	}
}

const crashEntries = 12

// TestCompactCrashChild is the child half of the test above. It is a no-op
// unless the parent points it at a directory.
func TestCompactCrashChild(t *testing.T) {
	dir := os.Getenv(crashDirEnv)
	if dir == "" {
		t.Skip("child process of TestCompactCrashBetweenScratchAndRenameLosesNothing")
	}
	j, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < crashEntries; i++ {
		id := fmt.Sprintf("crash-%d", i)
		if _, err := j.Create(Out, sampleEntry(id), false); err != nil {
			t.Fatal(err)
		}
		hold := int64(i + 1)
		if _, err := j.Apply(id, Update{Status: StatusInFlight, Handoff: HandoffHeld,
			AccruedHoldMs: &hold}); err != nil {
			t.Fatal(err)
		}
	}
	testHookPreRename = func() {
		p, err := os.FindProcess(os.Getpid())
		if err != nil {
			t.Fatal(err)
		}
		_ = p.Kill()
		// Kill is asynchronous. Block so nothing else in this process can run.
		select {}
	}
	_, _, _ = j.Compact()
	t.Fatal("the child survived its own kill")
}

// TestShortWriteLeavesNoPartialRecord is the 2026-08-08 failure in miniature.
//
// A full disk made one append land short. The record was rejected and never
// ACKed, which was correct — but its bytes stayed in the log, the next append
// wrote a whole record behind them, and replay then stopped at that seam and
// discarded EVERY record after it. Five sidecars ran for eight hours with
// journals that replayed to 01:07:40 and no further, and nothing said so.
func TestShortWriteLeavesNoPartialRecord(t *testing.T) {
	dir := t.TempDir()
	j, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := j.Create(Out, sampleEntry("before"), false); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, fileName)

	// Simulate the short write the way the disk did: bytes in the file, an
	// error to the caller, nothing ACKed.
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"op":"create","migrationId":"torn","ent`); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	// And the append that lands behind it, which is what turns a torn TAIL into
	// a torn MIDDLE.
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Open truncated the partial tail, so the log is whole again and the entry
	// written before it survived.
	if _, ok := reopened.Get("before"); !ok {
		t.Fatal("the record written before the short write was lost")
	}
	// An unparsable line with nothing behind it is the ordinary torn tail and
	// cost nothing that was ever durable.
	if got := reopened.Discarded(); got != 0 {
		t.Fatalf("a torn tail reported %d discarded bytes; it should report none", got)
	}
	if reopened.Live() != 1 {
		t.Fatalf("the journal holds %d entries, want 1", reopened.Live())
	}
	if _, err := reopened.Create(Out, sampleEntry("after"), false); err != nil {
		t.Fatal(err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
	final, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer final.Close()
	for _, id := range []string{"before", "after"} {
		if _, ok := final.Get(id); !ok {
			t.Fatalf("%s did not survive the torn record", id)
		}
	}
	if final.Discarded() != 0 {
		t.Fatalf("a healthy journal reported %d discarded bytes", final.Discarded())
	}
}

// TestDiscardedReportsWhatReplayThrewAway makes the silent case loud: a journal
// with an unparsable record in the MIDDLE must say how much it lost.
func TestDiscardedReportsWhatReplayThrewAway(t *testing.T) {
	dir := t.TempDir()
	j, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := j.Create(Out, sampleEntry("kept"), false); err != nil {
		t.Fatal(err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, fileName)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	// A half record, a newline, then a whole one behind it — the exact shape a
	// short write on a full disk left in every journal of the living rig.
	torn := "{\"op\":\"create\",\"migrationId\":\"tor\n"
	behind := `{"op":"create","migrationId":"lost","at":1,"direction":"out","entry":{"migrationId":"lost"}}` + "\n"
	if _, err := f.WriteString(torn + behind); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if _, ok := reopened.Get("kept"); !ok {
		t.Fatal("the record before the damage was lost")
	}
	if _, ok := reopened.Get("lost"); ok {
		t.Fatal("a record behind an unparsable line was replayed")
	}
	// What is reported is what was LOST: the complete record behind the damage.
	// The half-written line itself was never durable and cost nothing.
	if got := reopened.Discarded(); got != int64(len(behind)) {
		t.Fatalf("Discarded = %d, want %d — the loss must be measured, not silent",
			got, len(behind))
	}
}
