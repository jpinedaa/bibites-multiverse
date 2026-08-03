// Package wire implements the JSON envelope shared by Contract A
// (contracts/contract-a.md §3) and the M2 subset of Contract B
// (contracts/contract-b-m2.md §3).
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
	"strings"
)

// Protocol identifiers.
const (
	ProtocolA = "contract-a/1"
	ProtocolB = "contract-b/1"
)

// Shared size limits (contract-a.md §10, contract-b-m2.md §9).
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
// version of want. Compatibility is on the major part only (contract-a.md §3.1).
func CheckProtocol(got, want string) error {
	gf, gm, ok := splitProtocol(got)
	if !ok {
		return fmt.Errorf("%w: %q is not <family>/<major>", ErrProtocolMajor, got)
	}
	wf, wm, ok := splitProtocol(want)
	if !ok {
		return fmt.Errorf("%w: %q is not <family>/<major>", ErrProtocolMajor, want)
	}
	if gf != wf || gm != wm {
		return fmt.Errorf("%w: got %q, want %q", ErrProtocolMajor, got, want)
	}
	return nil
}

func splitProtocol(p string) (family, major string, ok bool) {
	i := strings.LastIndex(p, "/")
	if i <= 0 || i == len(p)-1 {
		return "", "", false
	}
	return p[:i], p[i+1:], true
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
// (contract-a.md §4.1). 1e999 decodes to +Inf, so this check is not optional.
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
