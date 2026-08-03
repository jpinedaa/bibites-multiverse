package bb8

import (
	"errors"
	"strings"
	"testing"

	"multiverse/internal/wire"
)

const sample = `{"transform":{"position":[2000.0,412.77],"rotation":274.11,"scale":0.9312},` +
	`"rb2d":{"px":2000.0,"py":412.77,"vx":6.12,"vy":0.44,"r":274.11},` +
	`"genes":{"names":["a"],"values":[0.1]},` +
	`"body":{"id":{"id":-843827577},"health":42.0},` +
	`"clock":{"simulatedTime":1.0},"brain":{"Nodes":[],"Synapses":[]},"version":"0.6.3.1"}`

func TestValidateAcceptsASaneBlob(t *testing.T) {
	if err := Validate("0.6.3.1", sample); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidateRejects(t *testing.T) {
	if err := Validate("", sample); !errors.Is(err, ErrNoVersion) {
		t.Fatalf("no version: %v", err)
	}
	if err := Validate("0.6.3.1", ""); !errors.Is(err, ErrEmpty) {
		t.Fatalf("empty: %v", err)
	}
	if err := Validate("0.6.3.1", "not json"); !errors.Is(err, ErrNotJSONObject) {
		t.Fatalf("not json: %v", err)
	}
	if err := Validate("0.6.3.1", `["an","array"]`); !errors.Is(err, ErrNotJSONObject) {
		t.Fatalf("array: %v", err)
	}
	big := "{\"pad\":\"" + strings.Repeat("x", wire.MaxPayloadBytes) + "\"}"
	if err := Validate("0.6.3.1", big); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("oversize: %v", err)
	}
}

func TestHookIsCalledLast(t *testing.T) {
	sentinel := errors.New("schema says no")
	Hook = func(gameVersion, payload string) error { return sentinel }
	defer func() { Hook = nil }()

	if err := Validate("0.6.3.1", sample); !errors.Is(err, sentinel) {
		t.Fatalf("Validate did not call the hook: %v", err)
	}
	// The structural gate still runs first.
	if err := Validate("0.6.3.1", "not json"); !errors.Is(err, ErrNotJSONObject) {
		t.Fatalf("the hook ran before the structural gate: %v", err)
	}
}

func TestHashIsStableAndDistinguishing(t *testing.T) {
	if Hash(sample) != Hash(sample) {
		t.Fatal("Hash is not stable")
	}
	if Hash(sample) == Hash(sample+" ") {
		t.Fatal("Hash does not distinguish payloads")
	}
}

func TestInspectReadsTheWrappedEntityID(t *testing.T) {
	info := Inspect(sample)
	if !info.HasEntityID || info.EntityID != -843827577 {
		t.Fatalf("entityId = %d (has=%v), want -843827577", info.EntityID, info.HasEntityID)
	}
	if !info.HasHeading || info.Heading != 274.11 {
		t.Fatalf("heading = %v (has=%v), want 274.11", info.Heading, info.HasHeading)
	}
	if info.Version != "0.6.3.1" {
		t.Fatalf("version = %q", info.Version)
	}
}

func TestInspectReadsABareEntityID(t *testing.T) {
	// contract-a.md §5.3's example elides the BibiteID wrapper, so both shapes
	// must work.
	info := Inspect(`{"body":{"id":-1},"rb2d":{"r":10.0}}`)
	if !info.HasEntityID || info.EntityID != -1 {
		t.Fatalf("entityId = %d (has=%v), want -1", info.EntityID, info.HasEntityID)
	}
}

func TestInspectToleratesAnythingMissing(t *testing.T) {
	info := Inspect(`{"version":"0.6.3.1"}`)
	if info.HasEntityID || info.HasHeading {
		t.Fatalf("Inspect invented fields: %+v", info)
	}
	if Inspect("not json").Version != "" {
		t.Fatal("Inspect must not panic or invent on garbage")
	}
}

func TestGameVersionSupported(t *testing.T) {
	if !GameVersionSupported("0.6.3.1") {
		t.Fatal("an empty allow-list must accept every non-empty version")
	}
	if GameVersionSupported("") {
		t.Fatal("an empty version is never supported")
	}
	SupportedGameVersions = []string{"0.6.3.1"}
	defer func() { SupportedGameVersions = nil }()
	if !GameVersionSupported("0.6.3.1") || GameVersionSupported("0.7.0.0") {
		t.Fatal("the allow-list is not enforced")
	}
}
