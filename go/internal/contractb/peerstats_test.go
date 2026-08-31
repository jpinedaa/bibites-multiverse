package contractb

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestPeerStatsCarryTheParticipantsOwnNames is §33 B49 on the type itself: the
// two participant-chosen strings decode into typed fields, encode back under
// the keys the contract names, and do neither at the expense of a key from a
// newer peer that rides the same block (§16, B11).
func TestPeerStatsCarryTheParticipantsOwnNames(t *testing.T) {
	raw := []byte(`{"population":7,"keeper":"Jorge","worldName":"Cyanea Reach",` +
		`"someFutureSetting":{"x":1}}`)
	var stats PeerStats
	if err := json.Unmarshal(raw, &stats); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if stats.Keeper != "Jorge" {
		t.Fatalf("keeper decoded as %q, want %q", stats.Keeper, "Jorge")
	}
	if stats.WorldName != "Cyanea Reach" {
		t.Fatalf("worldName decoded as %q, want %q", stats.WorldName, "Cyanea Reach")
	}
	if stats.Population == nil || *stats.Population != 7 {
		t.Fatalf("population = %v; the two new fields must cost nothing beside them",
			stats.Population)
	}

	out, err := json.Marshal(stats)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var back map[string]json.RawMessage
	if err := json.Unmarshal(out, &back); err != nil {
		t.Fatalf("decode the re-encoded block: %v", err)
	}
	for key, want := range map[string]string{
		"keeper":            `"Jorge"`,
		"worldName":         `"Cyanea Reach"`,
		"someFutureSetting": `{"x":1}`,
	} {
		got, ok := back[key]
		if !ok {
			t.Fatalf("%s did not survive the round trip: %s", key, out)
		}
		if string(got) != want {
			t.Fatalf("%s came back as %s, want %s", key, got, want)
		}
	}
}

// TestParticipantNamesAreAbsentWhenEmpty is the consent rule where it shows on
// the wire: a sidecar with nothing configured sends no key at all, so a reader
// sees UNKNOWN rather than an empty name it might render as anonymous.
func TestParticipantNamesAreAbsentWhenEmpty(t *testing.T) {
	out, err := json.Marshal(PeerStats{Population: IntPtr(3)})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if strings.Contains(string(out), "keeper") || strings.Contains(string(out), "worldName") {
		t.Fatalf("an unset name reached the wire: %s", out)
	}
}

// TestParticipantNamesStopRidingAsUnknownExtras is why both keys belong in
// knownStatKeys. Before they were typed they arrived as strangers' keys and
// were re-emitted from `unknown` whatever this build held — so a block whose
// keeper this build had deliberately cleared would publish the old one back.
func TestParticipantNamesStopRidingAsUnknownExtras(t *testing.T) {
	var stats PeerStats
	if err := json.Unmarshal([]byte(`{"keeper":"Jorge","worldName":"Cyanea Reach"}`), &stats); err != nil {
		t.Fatalf("decode: %v", err)
	}
	stats.Keeper = ""
	stats.WorldName = ""
	out, err := json.Marshal(stats)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if string(out) != "{}" {
		t.Fatalf("a cleared name was republished from the unknown-key carrier: %s", out)
	}
}
