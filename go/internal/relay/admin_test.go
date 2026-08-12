package relay

// WP4's tests for contract-b-m4.md §7.5 and §22, B28: the authenticated admin
// path.
//
// The test this file exists for is TestANonAdminCredentialCannotActAndTheActDoesNotHappen,
// and it is the adversarial one for the same reason WP2's was: an admin path
// that only works for the operator proves nothing. B22 made the three grants
// disjoint, and the whole value of that disjointness is that a compromised peer
// or a compromised subscriber cannot reach an act — so the test asserts BOTH
// HALVES, the refusal AND the map that did not move.

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"multiverse/internal/contractb"
	"multiverse/internal/peercred"
)

// adminRig is a relay with its admin listener up beside its Contract B one.
type adminRig struct {
	*credRelay
	admin string
}

func startAdminRig(t *testing.T) *adminRig {
	t.Helper()
	r := startCredRelay(t, credRelayOptions{})
	r.mint("ops", peercred.GrantAdmin)
	ts := httptest.NewServer(r.srv.AdminHandler())
	t.Cleanup(ts.Close)
	return &adminRig{credRelay: r, admin: ts.URL}
}

func (a *adminRig) post(t *testing.T, path, credentialPeer string, body any) (int, map[string]any) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("encode %s: %v", path, err)
	}
	req, err := http.NewRequest(http.MethodPost, a.admin+path, bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("request %s: %v", path, err)
	}
	if credentialPeer != "" {
		req.Header.Set("Authorization", peercred.Header(credentialPeer, a.secret(credentialPeer)))
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post %s: %v", path, err)
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(resp.Body)
	out := map[string]any{}
	_ = json.Unmarshal(payload, &out)
	return resp.StatusCode, out
}

// TestReleaseOverThePathTakesTwoCallsAndKeepsTheHeldEntryReport is B28's third
// property. THE ACT STAYS DELIBERATE: the first call returns the same
// consequence report §7.5 has always required an operator to read, and the
// second performs the act.
func TestReleaseOverThePathTakesTwoCallsAndKeepsTheHeldEntryReport(t *testing.T) {
	a := startAdminRig(t)
	if _, _, err := a.srv.ReserveSlot("peer-departed", nil); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if _, _, err := a.srv.ReserveSlot("peer-stays", nil); err != nil {
		t.Fatalf("reserve: %v", err)
	}

	status, report := a.post(t, "/admin/release-slot", "ops",
		map[string]any{"slot": 1, "reason": "peer departed, operator request"})
	if status != http.StatusOK {
		t.Fatalf("the consequence call answered HTTP %d: %v", status, report)
	}
	// THE REPORT IS THE CONSOLE'S REPORT. The map's half in full, and the
	// custody half named as the thing the relay cannot know.
	for _, key := range []string{"act", "slot", "peerId", "position", "becomesHole",
		"lanesChanged", "addressRetiredForever", "heldEntriesAddressedHere",
		"confirmToken", "ringStateHash"} {
		if _, ok := report[key]; !ok {
			t.Fatalf("the release report is missing %q: %v", key, report)
		}
	}
	held, _ := report["heldEntriesAddressedHere"].(string)
	if !strings.Contains(held, "heldDepth") || !strings.Contains(held, "--list-inflight") {
		t.Fatalf("the held-entry report no longer says where the other half of the answer lives: %q",
			held)
	}
	if !strings.Contains(held, "not knowable from the relay") {
		t.Fatalf("the held-entry field claims the relay could look: %q", held)
	}

	// NOTHING HAS HAPPENED YET.
	if len(a.srv.Snapshot()) != 2 {
		t.Fatal("the consequence call changed the map; the report must change nothing")
	}

	status, applied := a.post(t, "/admin/release-slot/confirm", "ops", map[string]any{
		"confirmToken":  report["confirmToken"],
		"ringStateHash": report["ringStateHash"],
	})
	if status != http.StatusOK {
		t.Fatalf("the confirm call answered HTTP %d: %v", status, applied)
	}
	if applied["applied"] != true {
		t.Fatalf("the act did not apply: %v", applied)
	}
	if len(a.srv.Snapshot()) != 1 {
		t.Fatal("the slot was not released")
	}
	// §7.5: the address is retired forever, so maxSlotEverIssued never decreases.
	if got, want := applied["maxSlotEverIssued"], float64(2); got != want {
		t.Fatalf("maxSlotEverIssued is %v, want %v — a released number is never reused", got, want)
	}
}

// TestANonAdminCredentialCannotActAndTheActDoesNotHappen is the adversarial
// test. B22's three grants are DISJOINT, and this is the assertion that makes
// that word mean something on the admin path.
func TestANonAdminCredentialCannotActAndTheActDoesNotHappen(t *testing.T) {
	a := startAdminRig(t)
	a.mint("peer-a", peercred.GrantPeer)
	a.mint("archive-a", peercred.GrantSubscribe)
	if _, _, err := a.srv.ReserveSlot("peer-victim", nil); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	before := len(a.srv.Snapshot())

	for _, who := range []string{"peer-a", "archive-a"} {
		status, body := a.post(t, "/admin/release-slot", who,
			map[string]any{"slot": 1, "reason": "not mine to ask"})
		if status != http.StatusForbidden {
			t.Fatalf("%s's credential got HTTP %d from the admin path, want 403", who, status)
		}
		if _, leaked := body["confirmToken"]; leaked {
			t.Fatalf("%s was handed a confirmation token by a refusal", who)
		}
		// And the refusal names the remedy rather than the symptom.
		if msg, _ := body["error"].(string); !strings.Contains(msg, peercred.GrantAdmin) {
			t.Fatalf("the refusal to %s does not name the grant it needed: %q", who, msg)
		}
	}
	// A made-up credential gets 401 and the same nothing.
	status, _ := a.post(t, "/admin/release-slot", "",
		map[string]any{"slot": 1, "reason": "no credential at all"})
	if status != http.StatusUnauthorized {
		t.Fatalf("a credential-free admin call got HTTP %d, want 401", status)
	}

	// THE OTHER HALF: the act did not happen. Not partly, not eventually.
	if now := len(a.srv.Snapshot()); now != before {
		t.Fatalf("the map moved from %d slots to %d on a refused admin call", before, now)
	}
	if _, ok := a.srv.grid.ResOfPeer("peer-victim"); !ok {
		t.Fatal("the reservation a refused act named is gone")
	}
}

// TestAnActIsRefusedWhenTheMapMovedUnderItsToken is the reason the token is
// bound to ring.json's state at all: an operator confirming a consequence they
// read five minutes ago is confirming a consequence that may no longer be the
// consequence. A CONFIRMATION AN OPERATOR CANNOT SEE IS NOT A CONFIRMATION.
func TestAnActIsRefusedWhenTheMapMovedUnderItsToken(t *testing.T) {
	a := startAdminRig(t)
	if _, _, err := a.srv.ReserveSlot("peer-one", nil); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if _, _, err := a.srv.ReserveSlot("peer-two", nil); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	_, report := a.post(t, "/admin/release-slot", "ops",
		map[string]any{"slot": 1, "reason": "read this consequence"})

	// The map moves underneath: another peer takes a slot.
	if _, _, err := a.srv.ReserveSlot("peer-three", nil); err != nil {
		t.Fatalf("reserve: %v", err)
	}

	status, body := a.post(t, "/admin/release-slot/confirm", "ops", map[string]any{
		"confirmToken":  report["confirmToken"],
		"ringStateHash": report["ringStateHash"],
	})
	if status != http.StatusConflict {
		t.Fatalf("a stale confirmation answered HTTP %d, want 409", status)
	}
	if msg, _ := body["error"].(string); !strings.Contains(msg, "the map changed") {
		t.Fatalf("the refusal does not say WHY it refused: %q", msg)
	}
	if _, ok := a.srv.grid.ResOfPeer("peer-one"); !ok {
		t.Fatal("the act went through on a token bound to a map that had moved")
	}
}

// TestAConfirmationTokenIsSingleUse. A token that survived its use would be an
// act an operator could replay against a map they never read.
func TestAConfirmationTokenIsSingleUse(t *testing.T) {
	a := startAdminRig(t)
	for _, id := range []string{"peer-one", "peer-two"} {
		if _, _, err := a.srv.ReserveSlot(id, nil); err != nil {
			t.Fatalf("reserve: %v", err)
		}
	}
	_, report := a.post(t, "/admin/release-slot", "ops",
		map[string]any{"slot": 1, "reason": "first and only use"})
	if status, _ := a.post(t, "/admin/release-slot/confirm", "ops", map[string]any{
		"confirmToken": report["confirmToken"], "ringStateHash": report["ringStateHash"],
	}); status != http.StatusOK {
		t.Fatalf("the first confirmation answered HTTP %d", status)
	}
	status, body := a.post(t, "/admin/release-slot/confirm", "ops", map[string]any{
		"confirmToken": report["confirmToken"], "ringStateHash": report["ringStateHash"],
	})
	if status != http.StatusConflict {
		t.Fatalf("the replayed confirmation answered HTTP %d, want 409: %v", status, body)
	}
}

// TestHandoverOverThePathRebindsTheIdentityAndMintsAFreshCredential is B22's
// credential recovery path, reached over B28's door. There is no other
// recovery, and §3.1 says so rather than implying it.
func TestHandoverOverThePathRebindsTheIdentityAndMintsAFreshCredential(t *testing.T) {
	a := startAdminRig(t)
	if _, _, err := a.srv.ReserveSlot("peer-lost-its-string", nil); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if _, err := a.srv.Credentials().Mint("peer-lost-its-string", peercred.GrantPeer); err != nil {
		t.Fatalf("mint: %v", err)
	}

	status, report := a.post(t, "/admin/handover-slot", "ops", map[string]any{
		"slot": 1, "newPeerId": "peer-fresh", "reason": "join string lost",
	})
	if status != http.StatusOK {
		t.Fatalf("the handover report answered HTTP %d: %v", status, report)
	}
	if report["newPeerId"] != "peer-fresh" {
		t.Fatalf("the report does not name the new identity: %v", report)
	}
	status, applied := a.post(t, "/admin/handover-slot/confirm", "ops", map[string]any{
		"confirmToken": report["confirmToken"], "ringStateHash": report["ringStateHash"],
	})
	if status != http.StatusOK {
		t.Fatalf("the handover confirm answered HTTP %d: %v", status, applied)
	}
	join, _ := applied["joinString"].(string)
	if !strings.Contains(join, "peer-fresh") {
		t.Fatalf("the handover returned no usable join string: %q", join)
	}
	if _, held := a.srv.Credentials().GrantOf("peer-lost-its-string"); held {
		t.Fatal("the old identity kept a credential against a reservation it no longer holds")
	}
	res, ok := a.srv.grid.ResOfPeer("peer-fresh")
	if !ok || res.Slot != 1 {
		t.Fatalf("the reservation did not move to the new identity: %+v", res)
	}
}

// TestAnEvictedPeerIsToldWhatADrainingRelayTellsItAndNothingElse is the
// taxonomy fact this package owes the documentation spine, asserted rather than
// asserted about.
//
// B28 gives eviction close 4005 SHUTTING_DOWN — the SAME code a draining relay
// sends — and gives the refusal no shape of its own: no close code, no
// lastRefusal axis, no wire field. AN EVICTED PEER THEREFORE CANNOT TELL ITSELF
// APART FROM A PEER WHOSE RELAY IS RESTARTING, EXCEPT BY PERSISTENCE. That is
// the contract's shape and this test pins it, so a later change to it is a
// deliberate wire change and not a drift.
func TestAnEvictedPeerIsToldWhatADrainingRelayTellsItAndNothingElse(t *testing.T) {
	a := startAdminRig(t)
	a.mint("peer-loud", peercred.GrantPeer)
	p := a.dial(dialSpec{credentialPeer: "peer-loud", claimPeer: "peer-loud", sendHandshake: true})
	p.wait(contractb.TypeHandshakeAck, 2*time.Second)
	p.claim()
	p.wait(contractb.TypeSectorGrant, 2*time.Second)

	_, report := a.post(t, "/admin/evict-peer", "ops", map[string]any{
		"peerId": "peer-loud", "for": "1h", "reason": "would not stop after the deny list",
	})
	if status, applied := a.post(t, "/admin/evict-peer/confirm", "ops", map[string]any{
		"confirmToken": report["confirmToken"], "ringStateHash": report["ringStateHash"],
	}); status != http.StatusOK {
		t.Fatalf("the eviction confirm answered HTTP %d: %v", status, applied)
	}

	code, _ := p.waitClosed(3 * time.Second)
	if code != contractb.CloseShuttingDown {
		t.Fatalf("the evicted peer was closed %d, want 4005 SHUTTING_DOWN — the code a DRAINING "+
			"relay sends. B28 gives the refusal no distinguishable shape", code)
	}

	// Its reconnect is refused, and it is refused the same way: 4005 again, with
	// no new code and nothing on its slot to read.
	back := a.dial(dialSpec{credentialPeer: "peer-loud", claimPeer: "peer-loud", sendHandshake: true})
	code, _ = back.waitClosed(3 * time.Second)
	if code != contractb.CloseShuttingDown {
		t.Fatalf("the evicted peer's reconnect closed %d, want 4005", code)
	}
	// NOT 401: the credential is fine, and saying otherwise would send its
	// operator to the one remedy that cannot help them.
	if _, status, _ := a.tryDial("peer-loud"); status == http.StatusUnauthorized {
		t.Fatal("an evicted peer was answered 401; its credential is not the problem")
	}
	// And nothing on the map names it. lastRefusal carries three axes (§6.5) and
	// eviction is deliberately not a fourth.
	for _, res := range a.srv.Snapshot() {
		if res.PeerID != "peer-loud" {
			continue
		}
		a.srv.mu.Lock()
		refusal := a.srv.metaLocked(res.PeerID).lastRefusal
		a.srv.mu.Unlock()
		if refusal != "" {
			t.Fatalf("an eviction wrote %q on the slot's lastRefusal; §6.5 names three refusal "+
				"axes and eviction is not one of them", refusal)
		}
	}
}

// TestEvictionReleasesNothingAndLiftingRestoresAdmission. An eviction is a
// LIVENESS act, not a placement one: the reservation, the slot number and the
// position all survive, so the peer's return is an ordinary reclaim and its
// journal is addressable throughout.
func TestEvictionReleasesNothingAndLiftingRestoresAdmission(t *testing.T) {
	a := startAdminRig(t)
	a.mint("peer-loud", peercred.GrantPeer)
	res, _, err := a.srv.ReserveSlot("peer-loud", nil)
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}

	_, report := a.post(t, "/admin/evict-peer", "ops", map[string]any{
		"peerId": "peer-loud", "reason": "until lifted",
	})
	if _, applied := a.post(t, "/admin/evict-peer/confirm", "ops", map[string]any{
		"confirmToken": report["confirmToken"], "ringStateHash": report["ringStateHash"],
	}); applied["applied"] != true {
		t.Fatalf("the eviction did not apply: %v", applied)
	}

	after, ok := a.srv.grid.ResOfPeer("peer-loud")
	if !ok || after.Slot != res.Slot || after.Position() != res.Position() {
		t.Fatalf("the eviction moved the reservation: %+v -> %+v (present: %v)", res, after, ok)
	}
	if _, evicted := a.srv.Evictions()["peer-loud"]; !evicted {
		t.Fatal("the peer is not recorded as evicted")
	}

	// Lifting is spelled `for: 0`, so §7.5's table of THREE acts stays three.
	_, lift := a.post(t, "/admin/evict-peer", "ops", map[string]any{
		"peerId": "peer-loud", "for": "0", "reason": "it stopped",
	})
	if lift["lifts"] != true {
		t.Fatalf("the lift report does not say it lifts: %v", lift)
	}
	if _, applied := a.post(t, "/admin/evict-peer/confirm", "ops", map[string]any{
		"confirmToken": lift["confirmToken"], "ringStateHash": lift["ringStateHash"],
	}); applied["lifted"] != true {
		t.Fatalf("the lift did not apply: %v", applied)
	}
	if _, evicted := a.srv.Evictions()["peer-loud"]; evicted {
		t.Fatal("the eviction survived being lifted")
	}
	// And the peer is admitted again, back to its own slot. The grant this test
	// is about is the one that ANSWERS ITS CLAIM: a peer with a reservation also
	// receives the publisher's republished grant, spelled reason:"updated", and
	// which of the two lands first is the coalescing window's business.
	back := a.dial(dialSpec{credentialPeer: "peer-loud", claimPeer: "peer-loud", sendHandshake: true})
	back.wait(contractb.TypeHandshakeAck, 2*time.Second)
	back.claim()
	g := back.waitGrantReason(contractb.GrantReclaimed, 2*time.Second)
	if !g.Granted || g.Slot != res.Slot {
		t.Fatalf("the returning peer got %+v, want its own slot (%d) back", g, res.Slot)
	}
}

// TestAnActWithNoReasonIsRefused. The audit line records the operator's reason,
// and it is the one field the relay cannot reconstruct afterwards.
func TestAnActWithNoReasonIsRefused(t *testing.T) {
	a := startAdminRig(t)
	if _, _, err := a.srv.ReserveSlot("peer-one", nil); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	status, body := a.post(t, "/admin/release-slot", "ops", map[string]any{"slot": 1})
	if status != http.StatusBadRequest {
		t.Fatalf("a reasonless act answered HTTP %d, want 400: %v", status, body)
	}
}

// TestTheAdminPathIsNotOnTheContractBWire is B28's first property, and it is
// the one that keeps D1 intact: no frame of §6's catalogue may invoke an act.
// The admin mux answers nothing on the peer path, and the peer mux answers
// nothing on an admin path.
func TestTheAdminPathIsNotOnTheContractBWire(t *testing.T) {
	a := startAdminRig(t)
	ts := httptest.NewServer(a.srv.Handler())
	t.Cleanup(ts.Close)

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/admin/release-slot",
		strings.NewReader(`{"slot":1,"reason":"x"}`))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Authorization", peercred.Header("ops", a.secret("ops")))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("the Contract B listener answered an admin path with HTTP %d; the admin surface "+
			"is a SEPARATE LISTENER and nothing on this wire may reach it", resp.StatusCode)
	}
	// And the message catalogue did not grow: nothing on the peer wire is named
	// for an act.
	for _, typ := range []string{ActReleaseSlot, ActHandoverSlot, ActEvictPeer} {
		for _, known := range []string{
			contractb.TypeHandshake, contractb.TypeHandshakeAck, contractb.TypeSectorClaim,
			contractb.TypeSectorGrant, contractb.TypePeerStatus, contractb.TypeMigrationPayload,
			contractb.TypeMigrationAck, contractb.TypeMigrationNack, contractb.TypeGenomeRequest,
			contractb.TypeGenomeResponse, contractb.TypePing, contractb.TypePong,
			// The thirteenth, and the only message this project has added since M3
			// (§22, B26). B28's own table says the catalogue grows by exactly one
			// in the whole §22 set, and it is this — so the list that proves no
			// admin act is a message type has to carry it.
			contractb.TypeForwardReceipt,
		} {
			if strings.EqualFold(typ, known) {
				t.Fatalf("%q is both an admin act and a Contract B message type", typ)
			}
		}
	}
}
