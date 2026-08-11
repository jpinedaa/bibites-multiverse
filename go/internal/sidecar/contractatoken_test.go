package sidecar

// A47's sidecar half: contracts/contract-a.md §21, the bearer token on the
// Contract A upgrade, and §12 item 1 closing after three milestones of "not
// yet".
//
// The reason the answer changed is not that the wire changed — it did not, and
// §21 says so in its first paragraph. Contract A still runs on 127.0.0.1 between
// two processes D9 keeps on one machine. WHAT CHANGES IS WHOSE MACHINE IT IS:
// the package M5 ships installs both processes on a machine the owner has never
// seen, running whatever else its player runs, and "the only other processes
// here are mine" stops being something anybody can assert.
//
// Four rules are testable here and each one is below: the token is verified
// BEFORE the upgrade; the refusal is HTTP 401 AND NOT A CLOSE CODE; the sidecar
// MINTS the file at first start, 0600; and NOTHING ABOUT CUSTODY MOVES while the
// check fails.

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"multiverse/internal/contracta"
	"multiverse/internal/modtoken"
)

// probeContractA does the upgrade by hand so a test can read the HTTP status.
// §2.1: A REFUSED UPGRADE IS NOT A CLOSE — the codes there are statements made
// inside a session, and a session the token did not open never started.
func probeContractA(t *testing.T, addr, path, token string, sendHeader bool) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, "http://"+addr+path, nil)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	if sendHeader {
		req.Header.Set("Authorization", modtoken.Header(token))
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

// TestContractAUpgradeNeedsTheBearerToken is A47's *What refuses* row: HTTP 401,
// no WebSocket, and a body that is free text for a human — which says the one
// thing a confused player needs, that THIS IS NOT THE RELAY TOKEN.
func TestContractAUpgradeNeedsTheBearerToken(t *testing.T) {
	rl := startRelay(t)
	cfg := fastConfig(t, rl, "peer-contract-a-token")
	cfg.Secret = rl.secret("peer-contract-a-token")
	s := startSidecar(t, cfg)

	for name, c := range map[string]struct {
		token string
		send  bool
	}{
		"no Authorization header at all": {"", false},
		"an empty bearer value":          {"", true},
		"the wrong token":                {"ffffffffffffffffffffffffffffff", true},
		"the RELAY credential by mistake": {
			"peer-contract-a-token." + rl.secret("peer-contract-a-token"), true},
	} {
		t.Run(name, func(t *testing.T) {
			status, body := probeContractA(t, s.Addr(), contracta.ContractAPath, c.token, c.send)
			if status != http.StatusUnauthorized {
				t.Fatalf("HTTP %d, want 401 (contract-a.md §21, A47)", status)
			}
			if !strings.Contains(body, "This is not the relay token") {
				t.Fatalf("the 401 body is %q; A47's worked example names the confusion it exists "+
					"to clear", body)
			}
		})
	}

	// The right token upgrades. Without this the test above would pass against a
	// sidecar that refused everything.
	status, _ := probeContractA(t, s.Addr(), contracta.ContractAPath, testContractAToken, true)
	if status == http.StatusUnauthorized {
		t.Fatal("the configured token was refused; the check refuses everything and proves nothing")
	}
}

// TestARefusedModCostsNoCustody is the sentence A47 puts under its worked
// example and the reason the failure is survivable: "NOTHING ABOUT CUSTODY MOVES
// WHILE THIS FAILS. A mod that cannot authenticate is a mod with a closed export
// set (§5.4's fail-safe) and a sidecar holding its journal (§13, A1: no mod
// connection is not an error). The failure costs migrations that have not
// happened yet, and costs no organism that has."
func TestARefusedModCostsNoCustody(t *testing.T) {
	g := newGrid(t, 2, gridOptions{layout: layoutRow(2)})
	a, b := g.node(0), g.node(1)

	// One organism crosses first, so there IS custody to lose.
	id := a.mod.migrateOut(testEntityID, contracta.EdgeE, 0.5)
	waitFor(t, 10*time.Second, "the warm-up delivery", func() bool {
		return b.world.spawnCount(id) == 1
	})

	before := len(b.side.CustodySnapshot())

	// A mod with the wrong token dials b's sidecar, repeatedly, the way a
	// misconfigured install does.
	for i := 0; i < 6; i++ {
		status, _ := probeContractA(t, b.side.Addr(), contracta.ContractAPath, "deadbeefdeadbeefdeadbeef", true)
		if status != http.StatusUnauthorized {
			t.Fatalf("attempt %d got HTTP %d, want 401", i, status)
		}
	}
	time.Sleep(300 * time.Millisecond)

	// The LEGITIMATE mod's session is untouched, and so is the journal.
	if b.mod.isClosed() {
		t.Fatal("a refused upgrade closed the mod session that WAS authenticated; §2's 4006 " +
			"self-healing rule fires for a session that opened, not for one that never did")
	}
	if got := len(b.side.CustodySnapshot()); got != before {
		t.Fatalf("the journal changed from %d to %d entries because of a refused UPGRADE", before, got)
	}

	// And the map keeps running: another organism crosses after the refusals.
	second := a.mod.migrateOut(testEntityID+1, contracta.EdgeE, 0.5)
	waitFor(t, 10*time.Second, "a crossing after the refused upgrades", func() bool {
		return b.world.spawnCount(second) == 1
	})
}

// TestTheSidecarMintsItsTokenAtFirstStart is A47's *Where the token comes from*
// row and A52's migration-note ordering in one: the sidecar mints
// `contractATokenFile` at first start, 0600, so the file exists BEFORE any mod
// can dial. A rollout that brings the games back first meets a sidecar that is
// enforcing and answers 401 — recoverable, noisy, and entirely avoidable by
// ordering.
func TestTheSidecarMintsItsTokenAtFirstStart(t *testing.T) {
	rl := startRelay(t)
	cfg := fastConfig(t, rl, "peer-mints-its-token")
	cfg.Secret = rl.secret("peer-mints-its-token")
	// Let the sidecar own the file, which is what the shipped binary does.
	cfg.ContractAToken = ""
	s := startSidecar(t, cfg)

	path := filepath.Join(cfg.DataDir, modtoken.DefaultFileName)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("the sidecar did not mint %s: %v", modtoken.DefaultFileName, err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("the token file is mode %04o, want 0600 (A47)", perm)
	}
	minted, err := modtoken.Load(path)
	if err != nil {
		t.Fatalf("read the minted token: %v", err)
	}
	if len(minted) < 16 {
		t.Fatalf("the minted token is %d characters; it must not be guessable", len(minted))
	}

	// A mod that reads THAT FILE connects, which is the whole contract between
	// the two processes: both read the same path on the same machine (D9).
	status, _ := probeContractA(t, s.Addr(), contracta.ContractAPath, minted, true)
	if status == http.StatusUnauthorized {
		t.Fatal("the token the sidecar minted was refused by the sidecar that minted it")
	}

	// A SECOND start over the same data directory reuses it rather than minting
	// again: a rotation is deleting the file deliberately, not a side effect of a
	// restart, and a mod that kept working across a sidecar restart must go on
	// working (A47's rotation row).
	second, err := modtoken.EnsureFile(path)
	if err != nil {
		t.Fatalf("EnsureFile on an existing file: %v", err)
	}
	if second != minted {
		t.Fatal("a restart minted a NEW token; every mod on this machine would 401 until " +
			"somebody re-pointed it, for no reason a person asked for")
	}
}

// TestRetiredContractAPathTakesNoToken is A52's own sentence, and the reason is
// a support one rather than a security one: the sidecar owns the retired path
// and its 4000, which A47's token does not change, BECAUSE A PEER THAT CANNOT
// COMPLETE A HANDSHAKE SHOULD STILL LEARN WHY. A 401 here would teach a stale
// mod that its token is wrong, when what is wrong is its path.
func TestRetiredContractAPathTakesNoToken(t *testing.T) {
	g := newGrid(t, 1, gridOptions{noMods: true})
	url := "ws://" + g.node(0).side.Addr() + contracta.RetiredContractAPath

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ws, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		t.Fatalf("the retired path refused an unauthenticated dial (%v); it must ANSWER, with "+
			"4000, so a stale mod learns what is actually wrong", err)
	}
	defer ws.CloseNow()
	if _, _, err := ws.Read(ctx); int(websocket.CloseStatus(err)) != contracta.CloseProtocolUnsupported {
		t.Fatalf("the retired path closed with %d, want 4000", websocket.CloseStatus(err))
	}
}

// TestInsecureNoContractATokenIsAnExplicitChoice covers A47's off switch. It
// exists for a single-machine rehearsal and for nothing else, and it is a
// DIFFERENT FLAG ON A DIFFERENT BINARY FOR A DIFFERENT WIRE from the relay's
// --insecure-no-token — named differently on purpose so a runbook cannot confuse
// them.
func TestInsecureNoContractATokenIsAnExplicitChoice(t *testing.T) {
	rl := startRelay(t)
	cfg := fastConfig(t, rl, "peer-insecure-contract-a")
	cfg.Secret = rl.secret("peer-insecure-contract-a")
	cfg.ContractAToken = ""
	cfg.InsecureNoContractAToken = true
	s := startSidecar(t, cfg)

	// No header at all is accepted, which is the whole of what the flag does.
	status, _ := probeContractA(t, s.Addr(), contracta.ContractAPath, "", false)
	if status == http.StatusUnauthorized {
		t.Fatal("--insecure-no-contract-a-token still refused an unauthenticated upgrade")
	}
	// And it mints no file, because there is nothing to hand the mod.
	if _, err := os.Stat(filepath.Join(cfg.DataDir, modtoken.DefaultFileName)); err == nil {
		t.Fatal("the insecure sidecar minted a token file it will never check")
	}
	// The two flags are not the same string. This is a naming rule, and a naming
	// rule with no test is a naming suggestion.
	if modtoken.InsecureEnvVar == "MULTIVERSE_INSECURE_NO_TOKEN" {
		t.Fatal("the Contract A off switch has the relay's name; A47 names them differently on " +
			"purpose so a runbook cannot confuse them")
	}
}
