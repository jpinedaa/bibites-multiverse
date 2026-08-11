package modtoken

// contract-a.md §21, A47, at the level of the token itself. The behaviour on the
// wire — the 401, the refusal body, the minting at first start — is tested where
// the sidecar is; what is here is the handful of rules that would fail silently.

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureFileMintsOnceAndKeeps0600(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", DefaultFileName)
	first, err := EnsureFile(path)
	if err != nil {
		t.Fatalf("EnsureFile: %v", err)
	}
	if first == "" {
		t.Fatal("EnsureFile minted an empty token")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("mode %04o, want 0600 — the token's whole claim is that it raises the bar to "+
			"a process that can read a 0600 file owned by the player (A47)", perm)
	}
	// A SECOND call keeps it. Minting again on every start would 401 every mod on
	// the machine for no reason anybody asked for, and A47's rotation is a
	// deliberate delete rather than a restart.
	second, err := EnsureFile(path)
	if err != nil {
		t.Fatalf("EnsureFile (second): %v", err)
	}
	if second != first {
		t.Fatal("a second start minted a new token")
	}
	// Rotation: delete and restart. One reconnect, never a game restart.
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove: %v", err)
	}
	rotated, err := EnsureFile(path)
	if err != nil {
		t.Fatalf("EnsureFile (rotated): %v", err)
	}
	if rotated == first {
		t.Fatal("a rotation produced the same token")
	}
}

func TestLoadPrefersTheFileThenTheEnvironment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tok")
	if err := os.WriteFile(path, []byte("from-the-file\ntrailing junk\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got, err := Load(path); err != nil || got != "from-the-file" {
		t.Fatalf("Load(path) = (%q,%v), want the FIRST LINE only", got, err)
	}

	t.Setenv(FileEnvVar, path)
	if got, err := Load(""); err != nil || got != "from-the-file" {
		t.Fatalf("Load via %s = (%q,%v)", FileEnvVar, got, err)
	}
	t.Setenv(FileEnvVar, "")
	t.Setenv(EnvVar, "from-the-environment")
	if got, err := Load(""); err != nil || got != "from-the-environment" {
		t.Fatalf("Load via %s = (%q,%v)", EnvVar, got, err)
	}
	t.Setenv(EnvVar, "")
	if _, err := Load(""); err == nil {
		t.Fatal("Load invented a token from nothing; A47 forbids a client to mint or derive one")
	}
}

func TestFromRequestAndEqual(t *testing.T) {
	r, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:8791/contract-a/v2", nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if got := FromRequest(r); got != "" {
		t.Fatalf("FromRequest with no header = %q", got)
	}
	r.Header.Set("Authorization", Header("a-token"))
	if got := FromRequest(r); got != "a-token" {
		t.Fatalf("FromRequest = %q", got)
	}
	// The scheme is compared case-insensitively, because an HTTP client may send
	// "bearer" and a refusal over capitalisation is a support call.
	r.Header.Set("Authorization", "bearer a-token")
	if got := FromRequest(r); got != "a-token" {
		t.Fatalf("FromRequest with a lowercase scheme = %q", got)
	}
	r.Header.Set("Authorization", "Basic a-token")
	if got := FromRequest(r); got != "" {
		t.Fatalf("FromRequest accepted a non-bearer scheme: %q", got)
	}

	if !Equal("same", "same") || Equal("same", "different") || Equal("same", "sam") {
		t.Fatal("Equal does not compare what it is given")
	}
}

// TestTheOffSwitchIsNamedForItsOwnWire is A47's naming rule, which exists so a
// runbook cannot confuse two flags on two binaries for two wires. It is the kind
// of rule that is obviously satisfied on the day it is written and quietly
// broken by whoever renames a constant.
func TestTheOffSwitchIsNamedForItsOwnWire(t *testing.T) {
	for _, name := range []string{FileEnvVar, EnvVar, InsecureEnvVar} {
		if name == "MULTIVERSE_TOKEN" || name == "MULTIVERSE_TOKEN_FILE" ||
			name == "MULTIVERSE_INSECURE_NO_TOKEN" || name == "MULTIVERSE_PEER_SECRET" {
			t.Fatalf("%q is a Contract B name on a Contract A knob; they are different secrets, "+
				"in different files, on different wires (A47)", name)
		}
		if len(name) > 0 && name[:len("MULTIVERSE_")] != "MULTIVERSE_" {
			t.Fatalf("%q does not carry the project prefix", name)
		}
	}
}
