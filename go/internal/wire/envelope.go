// Package wire implements the JSON envelope shared by Contract A
// (contracts/contract-a.md §3) and Contract B
// (contracts/contract-b-m4.md §4).
//
// Both contracts use exactly the same five-field envelope, the same
// major-version compatibility rule, and the same "ignore unknown fields,
// ignore unknown types, close on a malformed frame" behaviour, so one codec
// serves both wires.
package wire

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Protocol identifiers. The version segment is <major>.<minor>
// (contract-a.md §14, A16); compatibility is on the major only.
//
// Both minors moved for the species-identity set (contract-a.md §16 A33,
// contract-b-m4.md §15 B10): one OPTIONAL field on two Contract A messages and
// one on a Contract B message, nothing removed and no type changed, which each
// contract's own version test answers with a MINOR bump. The URL paths do not
// move, and a peer on either side that still speaks the previous minor stays
// compatible by construction — THE MINOR IS A CAPABILITY STATEMENT, NOT A
// NEGOTIATION, and a receiver detects the feature by the presence of the
// `species` field and never by arithmetic on the minor.
//
// Both moved again for the species CENSUS (contract-a.md §17 A37,
// contract-b-m4.md §16 B12): two OPTIONAL fields on one Contract A message and
// the same two inside one Contract B block, nothing removed, no type changed,
// no message type, enum, close code or NACK code touched — MINOR again, and the
// paths stay at /contract-a/v2 and /contract-b/v3. The detection rule is
// unchanged and is the reason a mixed rig degrades to "unknown" and never to a
// wrong number: a receiver detects the census by the presence of the array,
// never by arithmetic on the minor and never by the minor a peer claims.
// TWO-WAY LANES MOVE ONE OF THEM AND NOT THE OTHER, and the asymmetry is the
// point (contract-a.md §18 A41, contract-b-m4.md §17 B15).
//
// Contract B takes a minor: `SECTOR_GRANT.neighbours` gains the "W" and "S"
// keys, which is additive data in an existing enum-keyed map, and a two-way map
// genuinely CANNOT work against a relay that does not compute the reverse
// walks. The minor is the honest statement of which relays do, and a peer
// detects the capability the way §3.1 requires — by the presence of the W and S
// keys in its grant, never by arithmetic on the minor.
//
// Contract A takes NOTHING. `exportEdges` has been an array validated against
// all four borderEdges since A18, `EDGE_STATUS.edges` has been "one entry per
// declared export edge" since A18, the exitEdge/entryEdge enum has held four
// values since M2, and §4.3.2's corner rule has been stated for an arbitrary
// declared set since A26. A four-edge mod is legal against a contract-a/2.2
// sidecar TODAY, with no code change, so a 2.3 would be a lie in the one
// direction that matters: it would imply 2.2 cannot do this.
// THE WORLD SETTINGS MOVE BOTH, AND IT IS THE FIRST TIME SINCE M4's OPENING
// THAT BOTH MOVE TOGETHER (contract-a.md §19 A44, contract-b-m4.md §19 B19).
// Contract A gains five OPTIONAL fields on `CONFIG_UPDATE` — `migrationExclude`,
// `saveMinutes`, `saveKeep`, `saveOnQuit`, `worldWrapping` — and Contract B's
// stats block gains those five plus the two version strings the sidecar has
// always held and never published, `modVersion` and `contractAVersion`. Seven
// additive OPTIONAL fields, no removal, no type change, no message type, enum,
// close code or NACK code touched: MINOR on both, and the paths stay at
// /contract-a/v2 and /contract-b/v3.
//
// The detection rule is unchanged and is still the reason a mixed rig degrades
// to "unknown" and never to a wrong number: a receiver detects a setting BY THE
// PRESENCE OF THE FIELD, never by arithmetic on the minor and never by the
// minor a peer claims. A sidecar that speaks 2.3 against a mod that speaks 2.2
// publishes no settings and says so — `contractAVersion` is what makes that
// unknown self-explaining.
const (
	ProtocolA = "contract-a/2.3"
	ProtocolB = "contract-b/3.5"
)

// Shared size limits (contract-a.md §10, contract-b-m4.md §12).
const (
	MaxFrameBytes   = 8 << 20 // 8 MiB
	MaxPayloadBytes = 4 << 20 // 4 MiB
)

var (
	// ErrMalformed marks a frame that must be answered with close 4003:
	// not a JSON object, a missing REQUIRED envelope field, or an envelope
	// field of the wrong JSON type (contract-a.md §3.2).
	ErrMalformed = errors.New("wire: malformed frame")

	// ErrProtocolMajor marks a frame whose protocol major version is not
	// supported. It must be answered with close 4000 (contract-a.md §3.1).
	ErrProtocolMajor = errors.New("wire: unsupported protocol major version")
)

// Envelope is the on-the-wire frame. Data is kept raw so a receiver can switch
// on Type before it decodes the body (contract-a.md §11.1, two-pass decode).
type Envelope struct {
	Protocol  string          `json:"protocol"`
	Type      string          `json:"type"`
	MessageID string          `json:"messageId"`
	SentAt    int64           `json:"sentAt"`
	Data      json.RawMessage `json:"data"`
}

// Decode parses and strictly validates one frame. It does not look at Type and
// does not decode Data.
func Decode(raw []byte) (Envelope, error) {
	if len(raw) > MaxFrameBytes {
		return Envelope{}, fmt.Errorf("%w: %d bytes over the %d limit", ErrMalformed, len(raw), MaxFrameBytes)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return Envelope{}, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	if fields == nil {
		return Envelope{}, fmt.Errorf("%w: frame is null", ErrMalformed)
	}

	var env Envelope
	var err error
	if env.Protocol, err = requireString(fields, "protocol"); err != nil {
		return Envelope{}, err
	}
	if env.Type, err = requireString(fields, "type"); err != nil {
		return Envelope{}, err
	}
	if env.MessageID, err = requireString(fields, "messageId"); err != nil {
		return Envelope{}, err
	}
	if env.SentAt, err = requireInt64(fields, "sentAt"); err != nil {
		return Envelope{}, err
	}
	data, ok := fields["data"]
	if !ok {
		return Envelope{}, fmt.Errorf("%w: missing envelope field %q", ErrMalformed, "data")
	}
	if kind(data) != '{' {
		return Envelope{}, fmt.Errorf("%w: envelope field %q is not an object", ErrMalformed, "data")
	}
	env.Data = data

	if !ValidType(env.Type) {
		return Envelope{}, fmt.Errorf("%w: type %q is not A-Z/underscore", ErrMalformed, env.Type)
	}
	return env, nil
}

// CheckProtocol reports whether a decoded frame speaks a compatible major
// version of want.
//
// The identifier is parsed by splitting at the last "/" and then at the first
// "."; a missing "." means minor 0 (contract-a.md §14, A16). Compatibility is
// on the major only and the minor is never a rejection reason, which is what
// lets a contract-a/1.1 sidecar keep serving a contract-a/1 mod.
func CheckProtocol(got, want string) error {
	gf, gm, _, ok := splitProtocol(got)
	if !ok {
		return fmt.Errorf("%w: %q is not <family>/<major>[.<minor>]", ErrProtocolMajor, got)
	}
	wf, wm, _, ok := splitProtocol(want)
	if !ok {
		return fmt.Errorf("%w: %q is not <family>/<major>[.<minor>]", ErrProtocolMajor, want)
	}
	if gf != wf || gm != wm {
		return fmt.Errorf("%w: got %q, want %q", ErrProtocolMajor, got, want)
	}
	return nil
}

// ProtocolMinor returns the minor version of an identifier, or 0 when it
// carries none. It is informational: no rule may reject on it.
func ProtocolMinor(p string) int {
	_, _, minor, ok := splitProtocol(p)
	if !ok {
		return 0
	}
	return minor
}

func splitProtocol(p string) (family, major string, minor int, ok bool) {
	i := strings.LastIndex(p, "/")
	if i <= 0 || i == len(p)-1 {
		return "", "", 0, false
	}
	family, version := p[:i], p[i+1:]
	dot := strings.Index(version, ".")
	if dot < 0 {
		return family, version, 0, true
	}
	if dot == 0 {
		return "", "", 0, false
	}
	n, err := strconv.Atoi(version[dot+1:])
	if err != nil {
		// An unparsable minor is still a usable major: the minor is never a
		// rejection reason.
		n = 0
	}
	return family, version[:dot], n, true
}

// Encode builds a frame. It mints a fresh messageId and stamps sentAt.
func Encode(protocol, typ string, sentAt int64, data any) ([]byte, error) {
	if data == nil {
		data = struct{}{}
	}
	body, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	if kind(body) != '{' {
		return nil, fmt.Errorf("wire: data for %s encoded to %q, not an object", typ, string(body))
	}
	return json.Marshal(Envelope{
		Protocol:  protocol,
		Type:      typ,
		MessageID: NewUUID(),
		SentAt:    sentAt,
		Data:      body,
	})
}

// ValidType reports whether s is a legal message discriminator: uppercase
// A-Z and underscore only (contract-a.md §3).
func ValidType(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if (c < 'A' || c > 'Z') && c != '_' {
			return false
		}
	}
	return true
}

// NewUUID returns a lowercase hyphenated random UUID v4.
func NewUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("wire: crypto/rand failed: " + err.Error())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

// ValidUUID checks the 36-character hyphenated shape. The version nibble is
// deliberately not enforced: contract-a.md §4.1 asks for case-insensitive
// comparison and forward tolerance, and rejecting a v7 id would be a needless
// interop break.
func ValidUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i := 0; i < 36; i++ {
		c := s[i]
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !isHex(c) {
				return false
			}
		}
	}
	return true
}

// SameUUID compares two ids case-insensitively (contract-a.md §4.1).
func SameUUID(a, b string) bool { return strings.EqualFold(a, b) }

func isHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// Finite rejects NaN and ±Inf, which are forbidden anywhere in these contracts
// (contract-a.md §4.1).
//
// It is a second net, not the first one. encoding/json refuses an overflowing
// literal outright — 1e999 into a float64 fails with "cannot unmarshal number
// 1e999", and a bare NaN token is not JSON at all — so both surface one layer
// earlier, as a malformed envelope field or a failed data decode
// (contract-a.md §13, amendment A9). This check covers what a future codec, a
// json.Number path or a computed value could still produce.
func Finite(f float64) bool { return !math.IsNaN(f) && !math.IsInf(f, 0) }

func requireString(fields map[string]json.RawMessage, name string) (string, error) {
	raw, ok := fields[name]
	if !ok {
		return "", fmt.Errorf("%w: missing envelope field %q", ErrMalformed, name)
	}
	if kind(raw) != '"' {
		return "", fmt.Errorf("%w: envelope field %q is not a string", ErrMalformed, name)
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", fmt.Errorf("%w: envelope field %q: %v", ErrMalformed, name, err)
	}
	return s, nil
}

func requireInt64(fields map[string]json.RawMessage, name string) (int64, error) {
	raw, ok := fields[name]
	if !ok {
		return 0, fmt.Errorf("%w: missing envelope field %q", ErrMalformed, name)
	}
	if kind(raw) != '0' {
		return 0, fmt.Errorf("%w: envelope field %q is not a number", ErrMalformed, name)
	}
	var n int64
	if err := json.Unmarshal(raw, &n); err != nil {
		return 0, fmt.Errorf("%w: envelope field %q: %v", ErrMalformed, name, err)
	}
	return n, nil
}

// kind classifies a raw JSON value by its first significant byte: '{' object,
// '[' array, '"' string, '0' number, 't' true/false, 'n' null, 0 empty.
func kind(raw json.RawMessage) byte {
	for _, c := range raw {
		switch c {
		case ' ', '\t', '\n', '\r':
			continue
		case '{', '[', '"', 'n':
			return c
		case 't', 'f':
			return 't'
		case '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			return '0'
		default:
			return c
		}
	}
	return 0
}
