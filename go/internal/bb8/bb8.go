// Package bb8 is the M2 skeleton of the bb8-schema component
// (system_decomposition.md §1, D4).
//
// The organism payload is opaque to the mod and is validated only here, on the
// sidecar side, in both directions. M2 needs three things and nothing more:
//
//   - a size and shape gate, so a garbage blob is answered with
//     MIGRATE_OUT_NACK / INVALID_PAYLOAD instead of travelling the wire,
//   - a stable content hash, which is the payload-identity half of the
//     migrationId dedup rule (contract-a.md §5.3 step 2),
//   - the two fields the delivery path reads out of the blob: the entity id
//     (contract-a.md §5.7) and the heading (contract-a.md §4.4).
//
// Gene bounds, node and synapse validation, dialect detection and cross-version
// conversion are M3 work. Hook is the seam they plug into.
package bb8

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"multiverse/internal/wire"
)

var (
	ErrEmpty         = errors.New("bb8: empty payload")
	ErrTooLarge      = errors.New("bb8: payload over maxPayloadBytes")
	ErrNotJSONObject = errors.New("bb8: payload is not a JSON object")
	ErrNoVersion     = errors.New("bb8: no game version")
)

// Hook is the validation seam for the real bb8-schema (M3). It is called last,
// after the structural gate, and its error is returned unchanged. Nil disables
// deep validation, which is the M2 default.
//
// It is a package variable rather than an interface parameter so the sidecar
// never has to thread a validator through every call site before the real
// schema exists.
var Hook func(gameVersion, payload string) error

// SupportedGameVersions is the M2 allow-list used for the Contract A close code
// 4002 check. An empty list accepts every version, which is what a skeleton
// schema honestly reports.
var SupportedGameVersions []string

// GameVersionSupported reports whether bb8-schema claims a dialect for v.
func GameVersionSupported(v string) bool {
	if v == "" {
		return false
	}
	if len(SupportedGameVersions) == 0 {
		return true
	}
	for _, s := range SupportedGameVersions {
		if s == v {
			return true
		}
	}
	return false
}

// Validate is the M2 structural gate.
func Validate(gameVersion, payload string) error {
	if gameVersion == "" {
		return ErrNoVersion
	}
	if payload == "" {
		return ErrEmpty
	}
	if len(payload) > wire.MaxPayloadBytes {
		return fmt.Errorf("%w: %d bytes over %d", ErrTooLarge, len(payload), wire.MaxPayloadBytes)
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal([]byte(payload), &probe); err != nil || probe == nil {
		return fmt.Errorf("%w", ErrNotJSONObject)
	}
	if Hook != nil {
		return Hook(gameVersion, payload)
	}
	return nil
}

// Hash is the payload identity used by the duplicate-migrationId rule.
func Hash(payload string) string {
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

// Info holds the few blob fields the M2 delivery path needs.
type Info struct {
	EntityID    int32
	HasEntityID bool
	Heading     float64
	HasHeading  bool
	Version     string
}

// Inspect pulls the entity id, the heading and the blob's own version key out
// of a payload. Every field is optional: a caller falls back to the value
// carried beside the blob on the wire, which contract-a.md §4.6 makes
// authoritative anyway.
//
// The entity id is read from $.body.id.id, with $.body.id as a bare number
// accepted as well — contract-a.md §5.3's example elides the BibiteID wrapper
// and the two shapes are indistinguishable from the document alone.
func Inspect(payload string) Info {
	var info Info
	var root map[string]json.RawMessage
	if err := json.Unmarshal([]byte(payload), &root); err != nil {
		return info
	}
	if raw, ok := root["version"]; ok {
		_ = json.Unmarshal(raw, &info.Version)
	}
	if raw, ok := root["body"]; ok {
		var body map[string]json.RawMessage
		if json.Unmarshal(raw, &body) == nil {
			if idRaw, ok := body["id"]; ok {
				var n int64
				if json.Unmarshal(idRaw, &n) == nil {
					info.EntityID, info.HasEntityID = int32(n), true
				} else {
					var wrapper struct {
						ID *int64 `json:"id"`
					}
					if json.Unmarshal(idRaw, &wrapper) == nil && wrapper.ID != nil {
						info.EntityID, info.HasEntityID = int32(*wrapper.ID), true
					}
				}
			}
		}
	}
	if raw, ok := root["rb2d"]; ok {
		var rb struct {
			R *float64 `json:"r"`
		}
		if json.Unmarshal(raw, &rb) == nil && rb.R != nil && !math.IsNaN(*rb.R) && !math.IsInf(*rb.R, 0) {
			info.Heading, info.HasHeading = *rb.R, true
		}
	}
	return info
}
