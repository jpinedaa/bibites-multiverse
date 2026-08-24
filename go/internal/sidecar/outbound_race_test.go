package sidecar

import (
	"testing"
	"time"

	"multiverse/internal/journal"
)

// TestCreateSnapshotCannotCommitASecondSend forces the handler/scheduler
// interleaving that a Create clone used to make unsafe. The MIGRATE_OUT handler
// stops after Create. The custody scheduler advances the real journal entry to
// sent and enqueues it. When the handler resumes, its stale pending clone must
// not enqueue the migration again.
func TestCreateSnapshotCannotCommitASecondSend(t *testing.T) {
	g := newGrid(t, 2, gridOptions{tune: func(i int, cfg *Config) {
		if i == 0 {
			// Only the explicit tick below may race the blocked handler.
			cfg.TickInterval = time.Hour
		}
	}})
	source := g.node(0)
	dest := g.node(1)

	created := make(chan struct{})
	resume := make(chan struct{})
	handlerDone := make(chan struct{})
	source.side.mu.Lock()
	source.side.afterOutboundCreate = func() {
		close(created)
		<-resume
	}
	source.side.afterOutboundImmediateTick = func() { close(handlerDone) }
	source.side.mu.Unlock()

	migrationID := source.mod.migrateOut(testEntityID, "E", 0.5)
	select {
	case <-created:
	case <-time.After(5 * time.Second):
		t.Fatal("MIGRATE_OUT handler did not stop after Journal.Create")
	}

	source.side.mu.Lock()
	createdState, ok := source.side.jr.Get(migrationID)
	if !ok || createdState.Handoff != journal.HandoffPending {
		source.side.mu.Unlock()
		t.Fatalf("created state = %+v, want pending outbound entry", createdState)
	}
	source.side.tickOutbound(createdState, source.side.now())
	source.side.mu.Unlock()

	waitFor(t, 5*time.Second, "the scheduler's one relay enqueue", func() bool {
		return g.relay.relay.ForwardedCount() == 1
	})
	close(resume)
	select {
	case <-handlerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("resumed MIGRATE_OUT handler did not complete its immediate custody tick")
	}

	waitFor(t, 5*time.Second, "one destination spawn", func() bool {
		return dest.world.spawnCount(migrationID) == 1
	})

	if got := g.relay.relay.ForwardedCount(); got != 1 {
		t.Fatalf("relay accepted %d payload enqueues, want exactly 1", got)
	}
	sent, _ := g.relay.relay.ReceiptCounts()
	if sent != 1 {
		t.Fatalf("relay emitted %d FORWARD_RECEIPTs, want exactly 1", sent)
	}
	if got := dest.world.spawnCount(migrationID); got != 1 {
		t.Fatalf("destination spawned migration %d times, want exactly 1", got)
	}
}
