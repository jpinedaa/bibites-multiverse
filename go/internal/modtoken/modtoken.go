// Package modtoken implements Contract A's bearer token, contracts/contract-a.md
// §2 and §21 A47.
//
// IT IS NOT CONTRACT B'S CREDENTIAL, and A47 says so in its own row because the
// only thing the two share is the word *bearer*. Contract B's is a PER-PEER
// credential bound to a peerId and carrying one of three disjoint grants
// (internal/peercred). This one binds TWO PROCESSES ON ONE MACHINE, names no
// identity, carries no grant and answers no question about the map. A sidecar
// MUST NOT present its Contract A token to a relay and MUST NOT accept a
// Contract B credential on this wire. Different secrets, different files,
// different wires — and named differently on purpose so a runbook cannot confuse
// them.
//
// What it buys, stated so nobody assumes the stronger version (A47): it raises
// the bar from *any local process* to *any local process that can read a 0600
// file owned by the player*. It is not confidentiality — this wire is plain HTTP
// on loopback and §21 deliberately does not give it TLS, because a loopback
// wire's confidentiality is the operating system's job. It is not an identity:
// there is one mod and one sidecar, and §2's 4006 self-healing rule is unchanged.
// And it does not make a compromised machine safe; nothing on a machine can.
package modtoken

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"multiverse/internal/fsutil"
)

const (
	// FileEnvVar names the file BOTH processes read, on the same machine (D9).
	// The sidecar's --contract-a-token-file is the flag form.
	FileEnvVar = "MULTIVERSE_CONTRACT_A_TOKEN_FILE"
	// EnvVar is the mod's alternative: the value directly (§10,
	// `contractATokenFile`). The sidecar deliberately does NOT read it — the
	// sidecar owns the file, because it is the process that mints it.
	EnvVar = "MULTIVERSE_CONTRACT_A_TOKEN"
	// InsecureEnvVar is the off switch of A47, and the flag form is
	// --insecure-no-contract-a-token. It exists for a single-machine rehearsal
	// and for nothing else: NO DOCUMENT THIS PROJECT SHIPS MAY INSTRUCT A PLAYER
	// TO PASS IT (A47; m5_considerations.md, DQ4).
	InsecureEnvVar = "MULTIVERSE_INSECURE_NO_CONTRACT_A_TOKEN"
	// DefaultFileName is `contractATokenFile`'s leaf (§10). The sidecar roots it
	// at its own --data-dir, which is the state directory it already owns.
	DefaultFileName = "contract-a.token"
)

// Refusal is the 401 body of A47's worked example. It is free text for a human
// and no side parses it — and it says the one thing a confused player needs,
// which is that this is not the relay token.
const Refusal = "contract A: bearer token rejected. The mod and the sidecar must read the same\n" +
	"MULTIVERSE_CONTRACT_A_TOKEN_FILE on this machine. This is not the relay token.\n"

// ErrNoToken means no token file and no environment value.
var ErrNoToken = errors.New("modtoken: no contract A token configured")

const mintBytes = 16

// Mint returns a fresh token: 16 random bytes, hex-encoded, which is the shape
// A47's example carries.
func Mint() string {
	var b [mintBytes]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("modtoken: crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}

// EnsureFile is the sidecar's side of "where the token comes from" (A47): read
// the file, or MINT IT AT FIRST START when it does not exist, mode 0600.
//
// The ordering this creates is the migration note's second consequence
// (contract-a.md §21, A52): the sidecar must be up on the new build, with the
// file present, BEFORE the mod that needs it comes back. A mod that returns
// first meets a sidecar that is enforcing and answers 401 on the ordinary
// ladder — recoverable, noisy, and entirely avoidable by ordering.
func EnsureFile(path string) (string, error) {
	if path == "" {
		return "", ErrNoToken
	}
	if tok, err := readFile(path); err == nil {
		return tok, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	tok := Mint()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(tok+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("modtoken: write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("modtoken: rename into %s: %w", path, err)
	}
	if err := fsutil.SyncDir(filepath.Dir(path)); err != nil {
		return "", err
	}
	return tok, nil
}

// Load reads a token for a CLIENT of this wire — the mod, or the rig's fakemod.
// The file wins over the environment value, and neither is ever minted here: a
// client that mints its own token is a client inventing an answer to a refusal,
// which A47 forbids in as many words.
func Load(path string) (string, error) {
	if path != "" {
		return readFile(path)
	}
	if v := strings.TrimSpace(os.Getenv(FileEnvVar)); v != "" {
		return readFile(v)
	}
	if v := strings.TrimSpace(os.Getenv(EnvVar)); v != "" {
		return v, nil
	}
	return "", ErrNoToken
}

func readFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	line := string(b)
	if i := strings.IndexAny(line, "\r\n"); i >= 0 {
		line = line[:i]
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return "", fmt.Errorf("modtoken: %s is empty", path)
	}
	return line, nil
}

// Equal is a constant-time comparison on the raw bytes (A47). Never ==.
func Equal(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// Header is the value a mod puts on its upgrade request.
func Header(token string) string { return "Bearer " + token }

// FromRequest pulls the token off an upgrade request. NEITHER SIDE MAY LOG THE
// VALUE, at any level, in whole or in prefix (A47, §11.1, §11.2) — which is why
// nothing in this package formats a token into an error.
func FromRequest(r *http.Request) string {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(h[len(prefix):])
}
