package relay

import (
	"encoding/json"
	"testing"
	"time"

	"multiverse/internal/contractb"
	"multiverse/internal/wire"
)

// readStatusFor reads PEER_STATUS frames until one carries a stats block for
// the wanted slot that satisfies ok, and returns that slot's RAW JSON — the
// bytes, not a re-decode, because "the relay did not rewrite it" is a claim
// about bytes.
func readStatusFor(t *testing.T, p *testPeer, slot int, what string,
	ok func(map[string]json.RawMessage) bool) map[string]json.RawMessage {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		env, err := wire.Decode(p.readFrame())
		if err != nil || env.Type != contractb.TypePeerStatus {
			continue
		}
		var status struct {
			Slots []map[string]json.RawMessage `json:"slots"`
		}
		if json.Unmarshal(env.Data, &status) != nil {
			continue
		}
		for _, s := range status.Slots {
			var n int
			if json.Unmarshal(s["slot"], &n) != nil || n != slot {
				continue
			}
			raw, present := s["stats"]
			if !present {
				continue
			}
			var stats map[string]json.RawMessage
			if json.Unmarshal(raw, &stats) != nil {
				continue
			}
			if ok(stats) {
				return stats
			}
		}
	}
	t.Fatalf("no PEER_STATUS ever carried %s for slot %d", what, slot)
	return nil
}

// TestRelayRepublishesTheCensusBlind is contract-b-m4.md §16, B11, and it is the
// same claim §15's B9 test makes about the migration lane, restated for the
// stats lane: THE RELAY'S ANSWER TO THIS AMENDMENT IS NOTHING.
//
// §6.3.1's "the relay does not interpret any of it" and §6.11's "it never
// routes, refuses, schedules or filters on a stat" already covered the census
// before it existed, so B11 adds no relay behaviour. What this test pins is
// that the code AGREES: the census a peer sends on a stats-bearing PING comes
// back out of the broadcast with every name intact, doubled space and all, and
// with the flag beside it.
//
// The names are chosen so that any repair shows: a trailing space, a doubled
// internal space, a half that is only whitespace, and one carrying < & > and a
// non-ASCII rune. A relay with an opinion about any of them is a relay that
// could one day route on one.
func TestRelayRepublishesTheCensusBlind(t *testing.T) {
	url := startTestRelay(t)
	peer := dialPeer(t, url, "peer-census")
	peer.claim(1)
	peer.waitGrant(1)

	sent := []contractb.CensusEntry{
		{GenericName: "Izus ", SpecificName: "copedylanus", Bibites: 96, Eggs: 14},
		{GenericName: "Izus", SpecificName: "copedylanus", Bibites: 61, Eggs: 9},
		{GenericName: " ", SpecificName: "anonymus", Bibites: 38, Eggs: 11},
		{GenericName: "Cyanëa<&>", SpecificName: `velox"issima  prima`, Bibites: 17, Eggs: 3},
	}
	peer.sendTyped(contractb.TypePing, contractb.Ping{
		Nonce: "n1",
		Stats: &contractb.PeerStats{
			Population: contractb.IntPtr(212),
			EggCount:   contractb.IntPtr(37),
			Species:    &contractb.Census{Entries: sent},
			Truncated:  true,
		},
	})

	stats := readStatusFor(t, peer, 1, "a species census", func(s map[string]json.RawMessage) bool {
		_, ok := s["species"]
		return ok
	})

	var got []contractb.CensusEntry
	if err := json.Unmarshal(stats["species"], &got); err != nil {
		t.Fatalf("the republished census is not an array of entries: %v", err)
	}
	if len(got) != len(sent) {
		t.Fatalf("the relay republished %d entries, want %d: %+v", len(got), len(sent), got)
	}
	for i := range sent {
		if got[i] != sent[i] {
			t.Fatalf("entry %d came back as %+v, want %+v — the relay rewrote a name it must "+
				"never read", i, got[i], sent[i])
		}
	}
	var truncated bool
	if err := json.Unmarshal(stats["truncated"], &truncated); err != nil || !truncated {
		t.Fatalf("truncated came back as %s; it is monotonic and this relay had nothing to strip",
			stats["truncated"])
	}
	// The census must not have cost the ordinary stats their place, either.
	if string(stats["population"]) != "212" || string(stats["eggCount"]) != "37" {
		t.Fatalf("the stats block came back as %v", stats)
	}
}

// TestRelayCarriesAStatItDoesNotKnow is contract-b-m4.md §16, B11's one SHOULD,
// "for the next field after this one".
//
// A relay that re-encodes a stats block from a typed model DROPS every field it
// does not have a struct member for — which is exactly how a contract-b/3.1
// relay loses the census, degrading a live species list to unknown for no
// reason but the order the code was written in. The fix is that the block keeps
// what it did not understand and re-emits it.
//
// The unknown field is still ignored for every PURPOSE: nothing reads it,
// nothing routes on it, and it reaches the broadcast in the only way a stat
// ever should — as a copy.
func TestRelayCarriesAStatItDoesNotKnow(t *testing.T) {
	url := startTestRelay(t)
	peer := dialPeer(t, url, "peer-future")
	peer.claim(1)
	peer.waitGrant(1)

	// A stats block from a sidecar one minor ahead of this build: two fields
	// this code has never heard of, beside two it has.
	future := `{"population":41,"custodyDepth":2,` +
		`"biomeCensus":[{"name":"kelp","area":12.5}],"moodRing":"chartreuse"}`
	frame, err := json.Marshal(map[string]any{
		"protocol":  wire.ProtocolB,
		"type":      contractb.TypePing,
		"messageId": wire.NewUUID(),
		"sentAt":    time.Now().UnixMilli(),
		"data":      json.RawMessage(`{"nonce":"n1","stats":` + future + `}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	peer.send(frame)

	stats := readStatusFor(t, peer, 1, "the future stats block",
		func(s map[string]json.RawMessage) bool { return string(s["population"]) == "41" })

	if got := string(stats["moodRing"]); got != `"chartreuse"` {
		t.Fatalf("an unknown scalar stat did not survive the relay: %q", got)
	}
	if got := string(stats["biomeCensus"]); got != `[{"name":"kelp","area":12.5}]` {
		t.Fatalf("an unknown structured stat did not survive the relay: %q", got)
	}
	if string(stats["custodyDepth"]) != "2" {
		t.Fatalf("a known stat was lost carrying the unknown ones: %v", stats)
	}
}

// TestRelayNeverInventsACensus is the other half of blindness. A peer that
// sends a stats block with no census gets one republished with no census: the
// relay has a map, six peers and a forwarding record, and it still has no
// opinion about which species live anywhere.
func TestRelayNeverInventsACensus(t *testing.T) {
	url := startTestRelay(t)
	peer := dialPeer(t, url, "peer-quiet")
	peer.claim(1)
	peer.waitGrant(1)

	peer.sendTyped(contractb.TypePing, contractb.Ping{
		Nonce: "n1",
		Stats: &contractb.PeerStats{Population: contractb.IntPtr(7), Truncated: true},
	})

	stats := readStatusFor(t, peer, 1, "the census-less stats block",
		func(s map[string]json.RawMessage) bool { return string(s["population"]) == "7" })
	if raw, ok := stats["species"]; ok {
		t.Fatalf("the relay published a census nobody sent: %s", raw)
	}
}
