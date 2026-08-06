// Package lantoken loads and compares the shared bearer token of
// contract-b-m4.md §3.1.
//
// M2 ran on loopback and needed no authentication. M3's wire crosses a LAN, so
// any device on that network can reach the relay port. This is deliberately the
// smallest answer that works: one token for the whole ring, on the HTTP upgrade
// only, never in a frame. TLS and per-peer identity are M4 (D9).
//
// There is no flag that takes the token literally. That is a rule, not an
// omission: a literal flag would put the secret in every process listing.
package lantoken

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
)

// EnvVar is the environment variable both sides read.
const EnvVar = "MULTIVERSE_TOKEN"

const (
	minLen = 16
	maxLen = 256
)

// ErrNoToken means neither MULTIVERSE_TOKEN nor --token-file supplied a value.
var ErrNoToken = errors.New("lantoken: no token configured")

// Load reads the token from tokenFile when it is set, else from the
// environment. A token file's first line is used, with trailing whitespace
// stripped.
func Load(tokenFile string) (string, error) {
	if tokenFile != "" {
		b, err := os.ReadFile(tokenFile)
		if err != nil {
			return "", fmt.Errorf("lantoken: read %s: %w", tokenFile, err)
		}
		line := string(b)
		if i := strings.IndexAny(line, "\r\n"); i >= 0 {
			line = line[:i]
		}
		return validate(strings.TrimRight(line, " \t"))
	}
	if v := os.Getenv(EnvVar); v != "" {
		return validate(strings.TrimSpace(v))
	}
	return "", ErrNoToken
}

func validate(tok string) (string, error) {
	if tok == "" {
		return "", ErrNoToken
	}
	if len(tok) < minLen || len(tok) > maxLen {
		return "", fmt.Errorf("lantoken: token is %d bytes, want %d to %d", len(tok), minLen, maxLen)
	}
	for i := 0; i < len(tok); i++ {
		if tok[i] < 0x21 || tok[i] > 0x7e {
			return "", errors.New("lantoken: token must be printable ASCII with no spaces")
		}
	}
	return tok, nil
}

// Equal is a constant-time comparison. §3.1 forbids ==.
func Equal(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// FromRequest pulls the bearer token out of an HTTP upgrade request.
func FromRequest(r *http.Request) string {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(h[len(prefix):])
}

// Header is the value a client puts on its upgrade request.
func Header(token string) string { return "Bearer " + token }
