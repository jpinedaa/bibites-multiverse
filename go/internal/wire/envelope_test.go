package wire

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
)

func TestDecodeAcceptsTheContractExample(t *testing.T) {
	// contract-a.md §3's example, verbatim at contract-a/2.2 (§17, A37).
	raw := []byte(`{
	  "protocol": "contract-a/2.2",
	  "type": "MIGRATE_OUT",
	  "messageId": "b7d1e0c4-9f2a-4c31-8b6d-2e0a41f5c7a9",
	  "sentAt": 1785693600123,
	  "data": { }
	}`)
	env, err := Decode(raw)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if env.Protocol != ProtocolA || env.Type != "MIGRATE_OUT" {
		t.Fatalf("decoded %+v", env)
	}
	if env.SentAt != 1785693600123 {
		t.Fatalf("sentAt = %d", env.SentAt)
	}
	if string(env.Data) == "" {
		t.Fatal("data was not captured")
	}
}

func TestDecodeRejectsMalformedFrames(t *testing.T) {
	cases := map[string]string{
		"not json":             `{`,
		"not an object":        `["contract-a/1"]`,
		"null":                 `null`,
		"missing protocol":     `{"type":"HEARTBEAT","messageId":"a","sentAt":1,"data":{}}`,
		"missing type":         `{"protocol":"contract-a/1","messageId":"a","sentAt":1,"data":{}}`,
		"missing messageId":    `{"protocol":"contract-a/1","type":"HEARTBEAT","sentAt":1,"data":{}}`,
		"missing sentAt":       `{"protocol":"contract-a/1","type":"HEARTBEAT","messageId":"a","data":{}}`,
		"missing data":         `{"protocol":"contract-a/1","type":"HEARTBEAT","messageId":"a","sentAt":1}`,
		"data is null":         `{"protocol":"contract-a/1","type":"HEARTBEAT","messageId":"a","sentAt":1,"data":null}`,
		"data is an array":     `{"protocol":"contract-a/1","type":"HEARTBEAT","messageId":"a","sentAt":1,"data":[]}`,
		"sentAt is a string":   `{"protocol":"contract-a/1","type":"HEARTBEAT","messageId":"a","sentAt":"1","data":{}}`,
		"type is a number":     `{"protocol":"contract-a/1","type":7,"messageId":"a","sentAt":1,"data":{}}`,
		"lowercase type":       `{"protocol":"contract-a/1","type":"heartbeat","messageId":"a","sentAt":1,"data":{}}`,
		"type with a hyphen":   `{"protocol":"contract-a/1","type":"MIGRATE-OUT","messageId":"a","sentAt":1,"data":{}}`,
		"protocol is a number": `{"protocol":1,"type":"HEARTBEAT","messageId":"a","sentAt":1,"data":{}}`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode([]byte(raw)); !errors.Is(err, ErrMalformed) {
				t.Fatalf("Decode(%s) error = %v, want ErrMalformed", raw, err)
			}
		})
	}
}

func TestDecodeIgnoresUnknownEnvelopeFields(t *testing.T) {
	// contract-a.md §3.1: unknown fields inside the envelope are ignored.
	raw := `{"protocol":"contract-a/1","type":"HEARTBEAT","messageId":"a","sentAt":1,"data":{},"future":42}`
	if _, err := Decode([]byte(raw)); err != nil {
		t.Fatalf("Decode: %v", err)
	}
}

// TestCheckProtocolComparesMajorOnly covers contract-a.md §14 A16 and §15 A23:
// the version segment is <major>.<minor>, a missing minor means 0, the minor is
// never a rejection reason, and M4's MAJOR bump on both wires is what stops an
// M3 peer from ever meeting an M4 one.
func TestCheckProtocolComparesMajorOnly(t *testing.T) {
	// Every minor of major 2 interoperates, in both directions: a
	// contract-a/2.1 sidecar and a contract-a/2.2 mod are compatible BY
	// CONSTRUCTION, and the census simply reads as unknown on the older one
	// (§17, A37). THE MINOR IS A CAPABILITY STATEMENT, NOT A NEGOTIATION.
	for _, got := range []string{"contract-a/2", "contract-a/2.0", "contract-a/2.1",
		"contract-a/2.2", "contract-a/2.7"} {
		if err := CheckProtocol(got, ProtocolA); err != nil {
			t.Fatalf("CheckProtocol(%q): %v", got, err)
		}
	}
	// The M3 wire. A contract-a/1.1 mod and a contract-a/2.0 sidecar are
	// incompatible BY DESIGN, and both sides say so loudly rather than misreading
	// a field (§15, A23).
	for _, old := range []string{"contract-a/1", "contract-a/1.1"} {
		if err := CheckProtocol(old, ProtocolA); !errors.Is(err, ErrProtocolMajor) {
			t.Fatalf("CheckProtocol(%q) accepted an M3 mod: %v", old, err)
		}
	}
	if err := CheckProtocol("contract-b/3.0", ProtocolA); !errors.Is(err, ErrProtocolMajor) {
		t.Fatalf("different family: %v", err)
	}
	if err := CheckProtocol("garbage", ProtocolA); !errors.Is(err, ErrProtocolMajor) {
		t.Fatalf("unparsable: %v", err)
	}
	if err := CheckProtocol(ProtocolB, ProtocolB); err != nil {
		t.Fatalf("contract B against itself: %v", err)
	}
	// M3's contract-b/2.0 against M4's contract-b/3.0: close 4000, never a
	// misrouted organism (contract-b-m4.md §4).
	if err := CheckProtocol("contract-b/2.0", ProtocolB); !errors.Is(err, ErrProtocolMajor) {
		t.Fatalf("an M3 sidecar was accepted by an M4 relay: %v", err)
	}
	if got := ProtocolMinor("contract-a/2.1"); got != 1 {
		t.Fatalf("ProtocolMinor = %d, want 1", got)
	}
	if got := ProtocolMinor("contract-b/3"); got != 0 {
		t.Fatalf("a missing minor must read as 0, got %d", got)
	}
}

func TestEncodeRoundTrip(t *testing.T) {
	type body struct {
		N int `json:"n"`
	}
	frame, err := Encode(ProtocolB, "PING", 99, body{N: 7})
	if err != nil {
		t.Fatal(err)
	}
	env, err := Decode(frame)
	if err != nil {
		t.Fatal(err)
	}
	if env.Protocol != ProtocolB || env.Type != "PING" || env.SentAt != 99 {
		t.Fatalf("round trip lost fields: %+v", env)
	}
	if !ValidUUID(env.MessageID) {
		t.Fatalf("messageId %q is not a uuid", env.MessageID)
	}
	var got body
	if err := json.Unmarshal(env.Data, &got); err != nil || got.N != 7 {
		t.Fatalf("data round trip: %v %+v", err, got)
	}
}

func TestEncodeNilDataIsAnEmptyObject(t *testing.T) {
	frame, err := Encode(ProtocolA, "HEARTBEAT", 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	env, err := Decode(frame)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(env.Data)) != "{}" {
		t.Fatalf("data = %s, want {}", env.Data)
	}
}

func TestUUIDShapeAndComparison(t *testing.T) {
	id := NewUUID()
	if len(id) != 36 || !ValidUUID(id) {
		t.Fatalf("NewUUID produced %q", id)
	}
	if id != strings.ToLower(id) {
		t.Fatalf("NewUUID must emit lowercase, got %q", id)
	}
	if !SameUUID(id, strings.ToUpper(id)) {
		t.Fatal("uuid comparison must be case-insensitive")
	}
	for _, bad := range []string{"", "abc", strings.Repeat("g", 36),
		"b7d1e0c4-9f2a-4c31-8b6d-2e0a41f5c7a", "b7d1e0c49f2a4c318b6d2e0a41f5c7a9"} {
		if ValidUUID(bad) {
			t.Fatalf("ValidUUID(%q) = true", bad)
		}
	}
}

func TestFiniteRejectsNaNAndInf(t *testing.T) {
	// contract-a.md §4.1: NaN and ±Inf are forbidden anywhere.
	if Finite(math.NaN()) || Finite(math.Inf(1)) || Finite(math.Inf(-1)) {
		t.Fatal("Finite accepted a non-finite value")
	}
	if !Finite(0) || !Finite(-2000.5) {
		t.Fatal("Finite rejected a finite value")
	}
	// contract-a.md §11.1 warns that 1e999 decodes to +Inf. Go's encoding/json
	// is stricter than that and refuses the token outright, so an out-of-range
	// literal is caught one layer earlier — as a malformed frame. Finite stays
	// as the second net, for values that turn non-finite by any other route.
	var v float64
	if err := json.Unmarshal([]byte(`1e999`), &v); err == nil {
		t.Fatal("encoding/json accepted 1e999")
	}
	raw := `{"protocol":"contract-a/1","type":"HEARTBEAT","messageId":"a","sentAt":1,"data":{"x":1e999}}`
	env, err := Decode([]byte(raw))
	if err != nil {
		t.Fatalf("Decode with 1e999 in data: %v", err)
	}
	var body struct {
		X float64 `json:"x"`
	}
	if err := json.Unmarshal(env.Data, &body); err == nil {
		t.Fatal("decoding 1e999 into a float64 must fail")
	}
}

func TestDecodeRejectsOversizeFrames(t *testing.T) {
	big := make([]byte, MaxFrameBytes+1)
	for i := range big {
		big[i] = ' '
	}
	if _, err := Decode(big); !errors.Is(err, ErrMalformed) {
		t.Fatalf("oversize frame error = %v", err)
	}
}
