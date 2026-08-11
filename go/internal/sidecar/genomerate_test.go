package sidecar

// WP4's answering-side half of §3.3's maxGenomeRequestsPerMinute
// (contract-b-m4.md §22, B24; §10).
//
// It is the row of §3.3 that already existed and was the WORKED EXAMPLE OF THE
// FAILURE: it shipped as the compiled constant contractb.GenomeRequestsPerMinute
// = 30, reachable only by editing source, and D20 had already ruled on that
// shape of thing — "a tunable an operator cannot retune from the metric that
// measures it is not a tunable." B24 renames it into the published table and
// makes it a knob on every party that enforces it. This is the test that the
// knob is the thing being enforced.

import (
	"testing"

	"multiverse/internal/contractb"
)

func TestTheAnsweringSideEnforcesTheGenomeRateItIsConfiguredWith(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Listen = "127.0.0.1:0"
	cfg.PeerID = "peer-answering"
	cfg.DataDir = t.TempDir()
	cfg.Logger = testLogger(t)
	// NOT the shipped default: a test against 30 cannot tell a knob from a
	// constant.
	cfg.GenomeRequestsPerMinute = 2

	s, err := New(cfg)
	if err != nil {
		t.Fatalf("sidecar: new: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	for i := 1; i <= 2; i++ {
		if !s.allowGenomeRequest("archive-main") {
			t.Fatalf("request %d of 2 was refused; the ceiling is %d",
				i, cfg.GenomeRequestsPerMinute)
		}
	}
	if s.allowGenomeRequest("archive-main") {
		t.Fatal("the third request inside the minute was allowed; the knob is not what is enforced")
	}
	// PER REQUESTER, PER ANSWERING PEER (§3.3). One noisy requester must not
	// spend another's allowance, or a busy archive would starve every sidecar
	// that also fetches.
	if !s.allowGenomeRequest("peer-slot4") {
		t.Fatal("a second requester was refused on the first requester's count")
	}
	// And the default is still the contract's for a sidecar that names nothing.
	if DefaultConfig().GenomeRequestsPerMinute != contractb.GenomeRequestsPerMinute {
		t.Fatalf("the shipped default moved to %d; §3.3 says %d",
			DefaultConfig().GenomeRequestsPerMinute, contractb.GenomeRequestsPerMinute)
	}
}

// TestTwoCapacityShedsInARowPinTheBackoffAtTheCeiling is §3.2's client-side
// half of B24, and the reason it is a MUST: "a client that is over a limit will
// be over it again in a second." A sidecar that reconnected on the ordinary
// ladder after a 4007 would spend the relay's capacity on the peer that already
// exceeded it, which is the failure mode the ceiling exists to stop.
func TestTwoCapacityShedsInARowPinTheBackoffAtTheCeiling(t *testing.T) {
	if pinBackoff(0, 1) {
		t.Fatal("ONE capacity shed pinned the backoff; §3.2 says two in a row")
	}
	if !pinBackoff(0, 2) {
		t.Fatal("two capacity sheds in a row did not pin the backoff at the ceiling")
	}
	// The 401 ladder is unchanged and still its own count.
	if pinBackoff(contractb.AuthFailuresBeforeCeiling-1, 0) {
		t.Fatal("the credential ladder pinned early")
	}
	if !pinBackoff(contractb.AuthFailuresBeforeCeiling, 0) {
		t.Fatal("the credential ladder no longer pins at authFailuresBeforeCeiling")
	}
}
