package sidecar

// The tests of `--diagnose`, and they are written against the SIX RULES of
// docs/sidecar-diagnose-spec.md §1 rather than against the implementation: a
// diagnostic is a promise about what it will not do, and a promise nothing
// checks is a comment.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"multiverse/internal/contracta"
	"multiverse/internal/contractb"
	"multiverse/internal/modtoken"
)

// layout2x2 is the smallest map on which BOTH axes exist, so every declared
// edge has somewhere to go and a healthy world's lanes are all open. A one-row
// rig is the A50 partial case instead, which is a different test.
func layout2x2() []contractb.Position {
	return []contractb.Position{
		{Col: 0, Row: 0}, {Col: 1, Row: 0}, {Col: 0, Row: 1}, {Col: 1, Row: 1},
	}
}

// healthyGrid is a 2x2 map whose mods are beating, with the Contract A token
// FILE that a shipped install has and this suite's rig does not.
//
// fastConfig hands the token to both processes in memory, because the file path
// is exercised by contractatoken_test.go; a shipped sidecar mints the file at
// first start instead, and --diagnose's whole contract-a-token check is about
// whether the two processes read ONE PATH. So the rig is given the file it would
// have, at the mode it would have.
func healthyGrid(t *testing.T) *grid {
	t.Helper()
	g := newGrid(t, 4, gridOptions{layout: layout2x2(), heartbeat: 100 * time.Millisecond})
	for _, nd := range g.nodes {
		if err := os.WriteFile(tokenFileOf(nd),
			[]byte(nd.cfg.ContractAToken+"\n"), 0o600); err != nil {
			t.Fatalf("write the token file: %v", err)
		}
	}
	// A speed reading needs a heartbeat, and a lane needs a grant.
	for _, nd := range g.nodes {
		side := nd.side
		waitFor(t, 10*time.Second, "a heartbeat from "+nd.cfg.PeerID, func() bool {
			return side.Stats().TimeScale != nil
		})
		nd.mod.waitEdge(contracta.EdgeN, true, 10*time.Second)
	}
	return g
}

// tokenFileOf is where a node's Contract A token file lives. Config.applyDefaults
// fills the field on the SIDECAR's copy of the config and not on the test's, so
// the path is derived the same way it is there.
func tokenFileOf(nd *node) string {
	return filepath.Join(nd.cfg.DataDir, modtoken.DefaultFileName)
}

// diagnoseOn runs the command against one live node, the way a participant runs
// it against their own machine.
func diagnoseOn(t *testing.T, nd *node, tune func(*DiagnoseOptions)) Report {
	t.Helper()
	opts := DiagnoseOptions{
		DataDir:            nd.cfg.DataDir,
		RelayURL:           nd.cfg.RelayURL,
		ContractATokenFile: tokenFileOf(nd),
		SecretConfigured:   nd.cfg.Secret != "",
		Timeout:            3 * time.Second,
	}
	if tune != nil {
		tune(&opts)
	}
	return Diagnose(opts)
}

func check(t *testing.T, rep Report, id string) CheckResult {
	t.Helper()
	for _, c := range rep.Checks {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("the report has no check %q", id)
	return CheckResult{}
}

func mustVerdict(t *testing.T, rep Report, id string, want Verdict) CheckResult {
	t.Helper()
	c := check(t, rep, id)
	if c.Verdict != want {
		t.Fatalf("%s is %s (%s), want %s\ndetail: %s",
			id, c.Verdict, c.Says, want, strings.Join(c.Detail, " | "))
	}
	return c
}

// TestDiagnoseOnAHealthyWorldReportsEveryCheckAndFailsNone is the ordinary case:
// a sidecar that is up, a game behind it, a slot held and lanes open. The bar is
// not that everything passes — several checks are honestly UNKNOWN on a rig with
// no packaged install — but that NOTHING FAILS, that every check in the
// specification is present, and that they arrive in the specification's own
// dependency order.
func TestDiagnoseOnAHealthyWorldReportsEveryCheckAndFailsNone(t *testing.T) {
	g := healthyGrid(t)
	nd := g.node(0)

	rep := diagnoseOn(t, nd, nil)

	if len(rep.Checks) != len(CheckIDs) {
		t.Fatalf("the report has %d checks, want the specification's %d",
			len(rep.Checks), len(CheckIDs))
	}
	for i, c := range rep.Checks {
		if c.ID != CheckIDs[i] {
			t.Fatalf("check %d is %s, want %s: the order is a DEPENDENCY order and it is part "+
				"of the contract", i, c.ID, CheckIDs[i])
		}
	}
	if rep.Summary.Fail != 0 {
		for _, c := range rep.Checks {
			if c.Verdict == VerdictFail {
				t.Errorf("FAIL %s: %s (%s)", c.ID, c.Says, strings.Join(c.Detail, " | "))
			}
		}
		t.Fatalf("a healthy world failed %d checks", rep.Summary.Fail)
	}
	if rep.Exit != ExitOK {
		t.Fatalf("exit %d with no failure, want %d", rep.Exit, ExitOK)
	}
	// The checks a live, healthy world must actually answer.
	for _, id := range []string{"data-dir", "stale-process", "contract-a-token", "mod-connected",
		"export-edges", "journal-replay", "journal-depths", "versions", "relay-reachable",
		"credential", "contract-version", "limits", "slot", "edges", "neighbours"} {
		mustVerdict(t, rep, id, VerdictPass)
	}
	// And mod-log is SKIPped rather than answered, because there is no trap to
	// tell apart when a game is connected.
	mustVerdict(t, rep, "mod-log", VerdictSkip)
	if !rep.LiveRead {
		t.Fatalf("the report did not read the running sidecar: %s", rep.LiveSource)
	}
}

// TestDiagnoseChangesNothingAndTakesNoSession is rule 1 and rule 4 together, and
// it is the test that matters most: this command runs beside a world that is
// carrying organisms, on a stranger's machine, and its whole licence to exist is
// that it cannot break one.
func TestDiagnoseChangesNothingAndTakesNoSession(t *testing.T) {
	g := healthyGrid(t)
	nd := g.node(0)
	journalPath := filepath.Join(nd.cfg.DataDir, "journal", "journal.log")

	before, err := os.Stat(journalPath)
	if err != nil {
		t.Fatalf("stat the journal: %v", err)
	}
	beforeSession := nd.side.RelaySessionID()
	beforeSlot := nd.side.Slot()
	nd.side.mu.Lock()
	beforeFrames := nd.side.sent.totalFrames
	nd.side.mu.Unlock()

	for i := 0; i < 3; i++ {
		if rep := diagnoseOn(t, nd, nil); rep.Summary.Fail != 0 {
			t.Fatalf("run %d failed a check on a healthy world", i)
		}
	}

	// The journal is untouched. journal.Open COMPACTS, which would rewrite this
	// file; OpenReadOnly must not, and the size and the modification time are
	// what prove it.
	after, err := os.Stat(journalPath)
	if err != nil {
		t.Fatalf("stat the journal after: %v", err)
	}
	if after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
		t.Fatalf("the journal moved under a read-only diagnostic: %d bytes at %s became %d at %s",
			before.Size(), before.ModTime(), after.Size(), after.ModTime())
	}
	// The session is untouched. A second Contract B connection would either be
	// shed for maxConnectionsPerPeer or would take this one over under the
	// newer-connection-replaces-older rule, and either shows up as a new session
	// id or a lost slot.
	if got := nd.side.RelaySessionID(); got != beforeSession {
		t.Fatalf("the relay session changed under a diagnostic: %q became %q", beforeSession, got)
	}
	if got := nd.side.Slot(); got != beforeSlot {
		t.Fatalf("the slot changed under a diagnostic: %d became %d", beforeSlot, got)
	}
	// And the diagnostic sent no frame of its own THROUGH this sidecar either:
	// the own-slot view is derived from state the process already had.
	nd.side.mu.Lock()
	afterFrames := nd.side.sent.totalFrames
	nd.side.mu.Unlock()
	if afterFrames < beforeFrames {
		t.Fatalf("the frame counter went backwards: %d then %d", beforeFrames, afterFrames)
	}
	// The one write it makes is removed. Nothing named diagnose survives, and
	// nothing at all is left in the journal directory.
	entries, err := os.ReadDir(nd.cfg.DataDir)
	if err != nil {
		t.Fatalf("read the data directory: %v", err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), "diagnose") {
			t.Fatalf("the diagnostic left %s behind", e.Name())
		}
	}
	jentries, err := os.ReadDir(filepath.Join(nd.cfg.DataDir, "journal"))
	if err != nil {
		t.Fatalf("read the journal directory: %v", err)
	}
	for _, e := range jentries {
		if e.Name() != "journal.log" {
			t.Fatalf("the journal directory gained %s; the diagnostic must never write here",
				e.Name())
		}
	}
}

// TestOneRootCauseProducesOneFailAndATrailOfUnknowns is the ordering rule: a
// dangling or unmounted data directory fails EVERY path at once, and the whole
// value of the dependency order is that the report says so once instead of
// fifteen times.
func TestOneRootCauseProducesOneFailAndATrailOfUnknowns(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "not-mounted")
	rep := Diagnose(DiagnoseOptions{
		DataDir:  missing,
		RelayURL: "ws://127.0.0.1:1",
		Timeout:  500 * time.Millisecond,
	})
	root := mustVerdict(t, rep, "data-dir", VerdictFail)
	if root.Actor != ActorYou || len(root.Taxonomy) == 0 {
		t.Fatalf("the root failure carries actor %q and taxonomy %v", root.Actor, root.Taxonomy)
	}
	for _, id := range []string{"stale-process", "journal-replay", "journal-depths",
		"disk-headroom"} {
		c := mustVerdict(t, rep, id, VerdictUnknown)
		if c.WaitingOn != "data-dir" {
			t.Fatalf("%s is unknown and waits on %q, want data-dir: a check whose precondition "+
				"failed must NAME it", id, c.WaitingOn)
		}
	}
	// Nothing that reads a file produces a SECOND failure behind the first. The
	// two other checks that can fail here are the ones that read no file at all
	// — the relay is not there and no credential is configured — and those are
	// separate root causes rather than consequences of this one.
	for _, c := range rep.Checks {
		if c.Verdict != VerdictFail {
			continue
		}
		switch c.ID {
		case "data-dir", "relay-reachable", "credential":
		default:
			t.Errorf("%s also failed; a check whose precondition failed must report UNKNOWN "+
				"and name it rather than produce a second, derived failure: %s", c.ID, c.Says)
		}
	}
	if rep.Exit != ExitFail {
		t.Fatalf("exit %d, want %d", rep.Exit, ExitFail)
	}
}

// TestNoVerdictIsEverASilentGuess is rules 3, 5 and 6 asserted over every report
// this suite can produce: an UNKNOWN never carries a remedy, a FAIL or a WARN
// always carries BOTH a taxonomy id and one of the three actors, and every
// taxonomy id it prints is one docs/error-taxonomy.md actually defines.
func TestNoVerdictIsEverASilentGuess(t *testing.T) {
	g := healthyGrid(t)
	reports := []Report{
		diagnoseOn(t, g.node(0), nil),
		// A world whose token file is not the one its mod reads.
		diagnoseOn(t, g.node(0), func(o *DiagnoseOptions) {
			o.ContractATokenFile = filepath.Join(t.TempDir(), "elsewhere.token")
		}),
		// A world with no credential at all.
		diagnoseOn(t, g.node(0), func(o *DiagnoseOptions) { o.SecretConfigured = false }),
		// A machine with no data directory.
		Diagnose(DiagnoseOptions{DataDir: filepath.Join(t.TempDir(), "gone"),
			RelayURL: "ws://127.0.0.1:1", Timeout: 300 * time.Millisecond}),
	}
	taxonomy := readTaxonomy(t)
	for _, rep := range reports {
		for _, c := range rep.Checks {
			switch c.Verdict {
			case VerdictFail, VerdictWarn:
				if c.Actor == "" {
					t.Errorf("%s %s carries no actor: %s", c.Verdict, c.ID, c.Says)
				}
				if !validActor(c.Actor) {
					t.Errorf("%s %s names actor %q, and there are only three", c.Verdict, c.ID,
						c.Actor)
				}
				if len(c.Taxonomy) == 0 {
					t.Errorf("%s %s carries no taxonomy id: %s", c.Verdict, c.ID, c.Says)
				}
				if c.Remedy == "" {
					t.Errorf("%s %s carries no remedy: %s", c.Verdict, c.ID, c.Says)
				}
				for _, tax := range c.Taxonomy {
					if taxonomy != nil && !taxonomy[tax] {
						t.Errorf("%s %s points at %q, which docs/error-taxonomy.md does not "+
							"define", c.Verdict, c.ID, tax)
					}
				}
			case VerdictUnknown:
				if c.Remedy != "" {
					t.Errorf("UNKNOWN %s carries a remedy, which makes it read as a failure: %s",
						c.ID, c.Remedy)
				}
				if len(c.Taxonomy) > 0 {
					t.Errorf("UNKNOWN %s points at a taxonomy entry; an unanswered question has "+
						"no cause yet", c.ID)
				}
			}
		}
	}
}

func validActor(a string) bool {
	return strings.HasPrefix(a, ActorYou) || strings.HasPrefix(a, ActorOperator) ||
		strings.HasPrefix(a, ActorNobody)
}

// readTaxonomy lifts every id docs/error-taxonomy.md defines out of the document
// itself, so that a check pointing at an entry nobody wrote is a test failure
// rather than a dead end in a support conversation. It returns nil — and the
// caller then skips the assertion — when the document is not beside this tree,
// because the package must still build and test outside the repository.
func readTaxonomy(t *testing.T) map[string]bool {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "error-taxonomy.md"))
	if err != nil {
		t.Logf("docs/error-taxonomy.md is not readable from here (%v); "+
			"the id-exists assertion is skipped", err)
		return nil
	}
	ids := map[string]bool{}
	// The ids are the first column of the tables, written in backticks.
	re := regexp.MustCompile("`([A-Z]+[A-Za-z]*-[A-Za-z0-9_*-]+)`")
	for _, m := range re.FindAllStringSubmatch(string(body), -1) {
		ids[m[1]] = true
	}
	// A few entries are families with a * in them; register the family for the
	// members a check can name.
	for id := range ids {
		if strings.HasSuffix(id, "*") {
			prefix := strings.TrimSuffix(id, "*")
			for _, member := range []string{"rate_limited", "role_has_no_slot",
				"version_incompatible", "peer_live", "no_peer", "peer_mod_absent",
				"peer_incompatible", "sim_size_mismatch", "peer_unreachable",
				"peer_overloaded", "admin_closed"} {
				ids[prefix+member] = true
			}
		}
	}
	if len(ids) < 20 {
		t.Fatalf("only %d taxonomy ids were found in the document; the reader is broken", len(ids))
	}
	return ids
}

// TestNothingInAReportIsASecret is rule 2, and it is checked against BOTH forms:
// a participant pastes one of them into a support conversation, and a message
// that contains a credential turns a support question into a slot handover.
func TestNothingInAReportIsASecret(t *testing.T) {
	g := healthyGrid(t)
	nd := g.node(0)
	secret := nd.cfg.Secret
	if secret == "" {
		t.Fatal("the rig gave this peer no credential, so this test would prove nothing")
	}
	token := nd.cfg.ContractAToken
	if token == "" {
		t.Fatal("the rig gave this peer no Contract A token, so this test would prove nothing")
	}

	rep := diagnoseOn(t, nd, nil)
	var human, machine bytes.Buffer
	RenderDiagnosis(&human, rep)
	if err := WriteDiagnosisJSON(&machine, rep); err != nil {
		t.Fatalf("json: %v", err)
	}
	view := nd.side.OwnSlot()
	var slotHuman, slotJSON bytes.Buffer
	RenderOwnSlot(&slotHuman, view)
	if err := json.NewEncoder(&slotJSON).Encode(view); err != nil {
		t.Fatalf("json: %v", err)
	}

	for _, surface := range []struct {
		what string
		text string
	}{
		{"the human report", human.String()},
		{"the json report", machine.String()},
		{"the human own-slot view", slotHuman.String()},
		{"the json own-slot view", slotJSON.String()},
	} {
		for _, secretText := range []struct{ what, value string }{
			{"the relay credential", secret},
			{"the Contract A token", token},
		} {
			if strings.Contains(surface.text, secretText.value) {
				t.Errorf("%s contains %s", surface.what, secretText.what)
			}
			// A prefix is a secret too. Eight characters of a token is eight
			// characters an attacker does not have to guess.
			if len(secretText.value) > 8 &&
				strings.Contains(surface.text, secretText.value[:8]) {
				t.Errorf("%s contains a PREFIX of %s", surface.what, secretText.what)
			}
		}
	}
	// What it MAY print, and must, because it is the fact a participant needs.
	if !strings.Contains(machine.String(), tokenFileOf(nd)) &&
		!strings.Contains(human.String(), tokenFileOf(nd)) {
		t.Error("the report never names the token file's PATH, which is the whole of what a " +
			"participant can act on")
	}
}

// TestTwoProcessesReadingTwoTokenFilesIsTheLocalAuthenticationFailure is A47's
// whole failure mode, and it is invisible from either process alone: the mod
// sees a 401 and the sidecar sees a refused upgrade, and neither can see the
// other's path.
func TestTwoProcessesReadingTwoTokenFilesIsTheLocalAuthenticationFailure(t *testing.T) {
	g := healthyGrid(t)
	nd := g.node(0)
	elsewhere := filepath.Join(t.TempDir(), "start-script-was-edited.token")
	if err := os.WriteFile(elsewhere, []byte("not-the-same-file\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	rep := diagnoseOn(t, nd, func(o *DiagnoseOptions) { o.ContractATokenFile = elsewhere })
	c := mustVerdict(t, rep, "contract-a-token", VerdictFail)
	if c.Actor != ActorYou {
		t.Fatalf("actor %q, want you: nothing about this involves the map", c.Actor)
	}
	if len(c.Taxonomy) == 0 || c.Taxonomy[0] != "A-401" {
		t.Fatalf("taxonomy %v, want A-401", c.Taxonomy)
	}
	// Both paths, because the remedy is to make them one path.
	joined := c.Says + " " + c.Remedy + " " + strings.Join(c.Detail, " ")
	if !strings.Contains(joined, elsewhere) || !strings.Contains(joined, tokenFileOf(nd)) {
		t.Fatalf("the failure names %q but not both paths", joined)
	}
}

// TestAnUnverifiableCertificateFailsAndTestsTheClockToo is B23 and spec §3's
// relay-tls row: no skip, no prompt, and the clock reported beside the
// certificate's window, because a wrong clock fails a valid certificate and the
// two produce the same error.
func TestAnUnverifiableCertificateFailsAndTestsTheClockToo(t *testing.T) {
	certPath, keyPath, _ := writeLocalhostPair(t)
	rl := startRelayWithTLS(t, certPath, keyPath)
	rep := Diagnose(DiagnoseOptions{
		DataDir:          t.TempDir(),
		RelayURL:         rl.url(),
		SecretConfigured: true,
		Timeout:          3 * time.Second,
	})
	// The connection itself is fine: the two failures are told apart.
	mustVerdict(t, rep, "relay-reachable", VerdictPass)
	c := mustVerdict(t, rep, "relay-tls", VerdictFail)
	if len(c.Taxonomy) == 0 || c.Taxonomy[0] != "B-TLS" {
		t.Fatalf("taxonomy %v, want B-TLS", c.Taxonomy)
	}
	joined := strings.Join(c.Detail, " | ")
	if !strings.Contains(joined, "clock") {
		t.Fatalf("the failure does not report this machine's clock: %s", joined)
	}
	if !strings.Contains(joined, "valid from") {
		t.Fatalf("the failure does not name the certificate's validity window: %s", joined)
	}
	if !strings.Contains(strings.ToLower(c.Remedy), "clock") {
		t.Fatalf("the remedy does not send the reader at their own clock first: %s", c.Remedy)
	}
	// And nothing downstream of it pretends to know anything.
	if got := mustVerdict(t, rep, "credential", VerdictUnknown); got.WaitingOn != "relay-tls" {
		t.Fatalf("credential waits on %q, want relay-tls", got.WaitingOn)
	}
}

// TestARelayThatIsNotThereIsToldFromOneThatDoesNotResolve is the relay-reachable
// row: on a stranger's machine the network is one they do not administer, so the
// check says which of the three failures it is and stops guessing.
func TestARelayThatIsNotThereIsToldFromOneThatDoesNotResolve(t *testing.T) {
	// Nothing listening. A port bound and immediately closed is the most
	// reliable way to name an address that will refuse.
	dead := startRelay(t)
	addr := dead.addr
	dead.kill()

	rep := Diagnose(DiagnoseOptions{DataDir: t.TempDir(),
		RelayURL: "ws://" + addr + contractb.ContractBPath, Timeout: 2 * time.Second})
	c := mustVerdict(t, rep, "relay-reachable", VerdictFail)
	if !strings.Contains(c.Says, "connects nowhere") {
		t.Fatalf("a refused connection says %q; the three failures must be told apart", c.Says)
	}
	if !strings.Contains(strings.Join(c.Detail, " "), addr) {
		t.Fatalf("the failure does not name where it tried: %v", c.Detail)
	}
}

// TestADeclaredEdgeOnAnAxisTheMapDoesNotHaveIsAWarningWithNoRemedy is A50's
// partial case, which is LEGAL and unchanged and NOT a fault — and the warning
// deliberately states no action, because the remedy would be a map that grows an
// axis and that is nobody at this machine's to apply.
func TestADeclaredEdgeOnAnAxisTheMapDoesNotHaveIsAWarningWithNoRemedy(t *testing.T) {
	// A one-row map: the row axis exists, the column axis does not, and a mod
	// declaring all four edges is the shipped default.
	g := newGrid(t, 2, gridOptions{layout: layoutRow(2)})
	rep := diagnoseOn(t, g.node(0), nil)
	c := mustVerdict(t, rep, "export-edges", VerdictWarn)
	if c.Actor != ActorNobody {
		t.Fatalf("actor %q, want nobody: a map shape is not a misconfiguration", c.Actor)
	}
	if len(c.Taxonomy) == 0 || c.Taxonomy[0] != "LANE-A50-partial" {
		t.Fatalf("taxonomy %v, want LANE-A50-partial", c.Taxonomy)
	}
	if !strings.Contains(c.Remedy, "deliberate") {
		t.Fatalf("the warning must say it claims no remedy on purpose: %s", c.Remedy)
	}
	joined := strings.Join(c.Detail, " ")
	if !strings.Contains(joined, "2x1") {
		t.Fatalf("the warning does not name the map's shape: %s", joined)
	}
}

// TestTheApproachingALimitBandIsAFractionOfThePublishedCeiling closes the
// specification's §7 slot and pins the reasoning: the band is derived from this
// build's own pacer and never chosen, and a limit the map did not publish is
// UNKNOWN rather than a default this side invented.
func TestTheApproachingALimitBandIsAFractionOfThePublishedCeiling(t *testing.T) {
	if LimitWarnFraction != sendPaceRateFraction+sendPaceBurstFraction {
		t.Fatalf("the band is %v; it must be the most this sidecar's own paced sender can emit "+
			"in one second, or it warns about correct behaviour", LimitWarnFraction)
	}
	// A published ceiling of 100: 75 is what a correctly pacing peer may emit
	// and is not a warning; 76 is.
	if _, _, over := approachingLimit(75, 100); over {
		t.Fatal("a reading at exactly the pacer's own worst case must not warn")
	}
	if _, _, over := approachingLimit(76, 100); !over {
		t.Fatal("a reading above the pacer's own worst case must warn")
	}
	if _, known, _ := approachingLimit(1000, 0); known {
		t.Fatal("an unpublished ceiling must read as unknown and never as `no limit`")
	}

	// And the check itself, over a synthetic live view: the reading is measured,
	// the ceiling is the published one, and the warning names both.
	d := &diag{opts: DiagnoseOptions{}, now: time.Now(), results: map[string]CheckResult{}}
	d.live = ownSlotResult{OK: true, View: OwnSlot{
		Relay: RelayState{Connected: true},
		Wire: WireState{
			Limits: map[string]int64{
				contractb.LimitMaxFramesPerSecond: 50,
				contractb.LimitMaxBytesPerSecond:  4194304,
			},
			PeakFramesPerSecond: 40,
			WarnFraction:        LimitWarnFraction,
		},
	}}
	c := d.checkLimits()
	if c.Verdict != VerdictWarn {
		t.Fatalf("40 frames against a published 50 is %s, want WARN: it is 80%% of the ceiling",
			c.Verdict)
	}
	if c.Actor == "" || len(c.Taxonomy) == 0 {
		t.Fatalf("an approaching-a-limit warning carries actor %q and taxonomy %v",
			c.Actor, c.Taxonomy)
	}
	if !strings.Contains(strings.Join(append(c.Detail, c.Says), " "), "maxFramesPerSecond") {
		t.Fatalf("the warning does not name the limit it is about: %v", c.Detail)
	}

	// A relay that published no table at all leaves it UNKNOWN, because absence
	// reads as unknown and never as "no ceilings" (contract-b-m4.md §6.2).
	d.live.View.Wire.Limits = nil
	if got := d.checkLimits(); got.Verdict != VerdictUnknown {
		t.Fatalf("with no published table the check is %s, want UNKNOWN", got.Verdict)
	}
}

func TestJournalDepthsNamesAnUpstreamAckQueueWithoutInventingAVerdict(t *testing.T) {
	d := &diag{opts: DiagnoseOptions{}, now: time.Now(), results: map[string]CheckResult{}}
	d.live = ownSlotResult{OK: true, View: OwnSlot{Custody: CustodyState{
		PendingAckDepth:         12,
		OldestPendingAckAgeMs:   (31 * time.Second).Milliseconds(),
		InboundRatePerSimMinute: 20,
	}}}
	c := d.checkJournalDepths()
	if c.Verdict != VerdictPass {
		t.Fatalf("a pending upstream ACK reading is %s, want PASS without an invented threshold",
			c.Verdict)
	}
	joined := strings.Join(append(c.Detail, c.Says), " ")
	if !strings.Contains(joined, "ACK") || !strings.Contains(joined, "12") {
		t.Fatalf("the warning does not name the pending ACK queue and depth: %s", joined)
	}
}

func TestJournalDepthsDoesNotInventAnAckAgeForAnOldRecord(t *testing.T) {
	d := &diag{opts: DiagnoseOptions{}, now: time.Now(), results: map[string]CheckResult{}}
	d.live = ownSlotResult{OK: true, View: OwnSlot{Custody: CustodyState{
		PendingAckDepth: 1,
	}}}
	c := d.checkJournalDepths()
	joined := strings.Join(append(c.Detail, c.Says), " ")
	if strings.Contains(joined, "oldest completed arrival") {
		t.Fatalf("a pending ACK without completedAt acquired an invented age: %s", joined)
	}
}

// TestTheThreeUnmeasuredThresholdsAreLeftUnmeasured is spec §6: three criteria
// belong to packages that have not measured them, and an implementation arc that
// invented one would have put a number nobody can defend into a support tool.
func TestTheThreeUnmeasuredThresholdsAreLeftUnmeasured(t *testing.T) {
	g := healthyGrid(t)
	rep := diagnoseOn(t, g.node(0), nil)

	// disk-headroom: it fails below the ceilings this install already promised
	// to write, which is arithmetic, and above them it says WP3 owns the number.
	disk := check(t, rep, "disk-headroom")
	if disk.Verdict == VerdictFail {
		t.Skip("this machine is genuinely short of disk; the rest of this assertion needs room")
	}
	if disk.Verdict != VerdictUnknown {
		t.Fatalf("disk-headroom is %s; with room and no published multiple it must be UNKNOWN",
			disk.Verdict)
	}
	if !strings.Contains(strings.Join(disk.Detail, " "), "WP3") {
		t.Fatalf("disk-headroom does not name the package that owns the missing number: %v",
			disk.Detail)
	}

	// time-scale: both readings, and no verdict on the gap between them.
	ts := check(t, rep, "time-scale")
	if ts.Verdict != VerdictUnknown {
		t.Fatalf("time-scale is %s; the band that would judge it is WP8's", ts.Verdict)
	}
	joined := strings.Join(ts.Detail, " ")
	if !strings.Contains(joined, "applied") {
		t.Fatalf("time-scale does not report the applied speed: %v", ts.Detail)
	}
	if !strings.Contains(joined, "WP8") {
		t.Fatalf("time-scale does not name the package that owns the missing band: %v", ts.Detail)
	}
}

// TestASaveThatBreachedItsBudgetIsAReadingAndNotARate is the save-health row:
// the 2000 ms stall budget is D14's and published, so a breach is reportable;
// HOW OFTEN a breach is too often is WP8's, so the check says the reading and
// refuses to judge the rate.
func TestASaveThatBreachedItsBudgetIsAReadingAndNotARate(t *testing.T) {
	now := time.Now()
	d := &diag{opts: DiagnoseOptions{}, now: now, results: map[string]CheckResult{}}
	minutes := 10.0
	d.live = ownSlotResult{OK: true, View: OwnSlot{Mod: ModState{
		Connected:   true,
		SaveMinutes: &minutes,
		LastSave: &contracta.SaveReceipt{
			AtMs: now.Add(-time.Minute).UnixMilli(), DurationMs: 4200, Bytes: 1 << 20},
	}}}
	c := d.checkSaveHealth()
	if c.Verdict != VerdictWarn {
		t.Fatalf("a 4200 ms save is %s, want WARN against the 2000 ms budget", c.Verdict)
	}
	if !strings.Contains(strings.Join(c.Detail, " "), "WP8") {
		t.Fatalf("the warning does not say who owns the breach-RATE band: %v", c.Detail)
	}
	if !strings.Contains(c.Remedy, "never an organism") {
		t.Fatalf("the remedy must say what a stall does and does not cost: %s", c.Remedy)
	}

	// A world with its save timer off is a READING and not a gap, so the check
	// is not applicable rather than unanswered.
	zero := 0.0
	d.live.View.Mod.SaveMinutes = &zero
	if got := d.checkSaveHealth(); got.Verdict != VerdictSkip {
		t.Fatalf("saveMinutes 0 is %s, want SKIP", got.Verdict)
	}
}

// TestTheNeighboursCheckNamesTheWorldThatIsBehindAndSaysItIsNotYours is DQ8's
// whole argument, and the reason this work package exists: the peer that suffers
// is not the peer that is stale.
func TestTheNeighboursCheckNamesTheWorldThatIsBehindAndSaysItIsNotYours(t *testing.T) {
	d := &diag{opts: DiagnoseOptions{}, now: time.Now(), results: map[string]CheckResult{}}
	d.live = ownSlotResult{OK: true, View: OwnSlot{
		Slot: SlotState{Slot: 1, MapWidth: 2, MapHeight: 1, SlotCount: 2},
		Mod:  ModState{Connected: true, GameVersion: "0.6.3.1", SimulationSize: 2000},
		Edges: []EdgeState{
			{Edge: contracta.EdgeE, Open: false, Reason: contracta.ReasonPeerIncompatible,
				Usable: true},
		},
		Peers: []PeerState{
			{Slot: 1, Col: 0, Row: 0, Me: true, Live: true, ModConnected: true,
				GameVersion: "0.6.3.1"},
			{Slot: 2, Col: 1, Row: 0, Live: true, ModConnected: true, GameVersion: "0.6.4.0"},
		},
	}}
	c := d.checkNeighbours()
	if c.Verdict != VerdictWarn {
		t.Fatalf("a neighbour on another build is %s, want WARN", c.Verdict)
	}
	if c.Actor != ActorOperator {
		t.Fatalf("actor %q, want operator: a participant cannot reach another participant",
			c.Actor)
	}
	joined := strings.Join(c.Detail, " ")
	if !strings.Contains(joined, "slot 2") || !strings.Contains(joined, "0.6.4.0") {
		t.Fatalf("the warning does not name the world that is behind: %v", c.Detail)
	}
	if !strings.Contains(c.Remedy, "NOT AT THIS MACHINE") {
		t.Fatalf("the warning must say the remedy is somewhere else: %s", c.Remedy)
	}
	if !strings.Contains(c.Remedy, "operator") {
		t.Fatalf("the warning must hand the evidence to the one party who can act: %s", c.Remedy)
	}
}

// TestTheJsonFormCarriesEverythingTheHumanFormDoes, because the machine form
// exists for a person pasting it into a support conversation and a shape that
// dropped the actor would drop the whole point of the taxonomy.
func TestTheJsonFormCarriesEverythingTheHumanFormDoes(t *testing.T) {
	g := healthyGrid(t)
	rep := diagnoseOn(t, g.node(0), nil)
	var buf bytes.Buffer
	if err := WriteDiagnosisJSON(&buf, rep); err != nil {
		t.Fatalf("json: %v", err)
	}
	var back Report
	if err := json.Unmarshal(buf.Bytes(), &back); err != nil {
		t.Fatalf("the json form does not decode: %v", err)
	}
	if back.Schema != DiagnoseSchema {
		t.Fatalf("schema %q, want %q: the shape is stable across releases so an old build's "+
			"report is still readable", back.Schema, DiagnoseSchema)
	}
	if len(back.Checks) != len(rep.Checks) {
		t.Fatalf("the json form has %d checks and the report has %d", len(back.Checks),
			len(rep.Checks))
	}
	if back.Exit != rep.Exit || back.ExitMeaning == "" {
		t.Fatalf("the json form does not carry the exit contract: %d %q", back.Exit,
			back.ExitMeaning)
	}
	for i, c := range back.Checks {
		if c.ID != rep.Checks[i].ID || c.Verdict != rep.Checks[i].Verdict {
			t.Fatalf("check %d differs between the forms", i)
		}
	}
}

// TestCheckFiltersTheReportAndNotTheWork: a check's precondition still has to be
// evaluated, or its answer means nothing, and the exit code then reflects what
// was actually reported.
func TestCheckFiltersTheReportAndNotTheWork(t *testing.T) {
	g := healthyGrid(t)
	rep := diagnoseOn(t, g.node(0), func(o *DiagnoseOptions) {
		o.Only = []string{"edges"}
	})
	if len(rep.Checks) != 1 || rep.Checks[0].ID != "edges" {
		t.Fatalf("--check reported %d checks, want just edges", len(rep.Checks))
	}
	// edges depends on slot, which depends on the live view: an answer that is
	// not UNKNOWN proves the precondition was evaluated even though it was never
	// reported.
	if rep.Checks[0].Verdict != VerdictPass {
		t.Fatalf("edges is %s with its precondition unreported; the filter must not change the "+
			"work: %s", rep.Checks[0].Verdict, rep.Checks[0].Says)
	}
	if bad := UnknownCheckIDs([]string{"edges", "not-a-check"}); len(bad) != 1 ||
		bad[0] != "not-a-check" {
		t.Fatalf("UnknownCheckIDs returned %v", bad)
	}
}

// TestNeitherReadOnlyCommandOpensTheSidecarsLogFile is rule 1 at the flag
// surface, and it is the one place the rule is easy to break by accident:
// --log-file has an environment default, so a participant running --diagnose
// with the environment their start script sets would have the diagnostic open —
// and rotate — the log of the sidecar that is running beside it.
func TestNeitherReadOnlyCommandOpensTheSidecarsLogFile(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "sidecar.log")
	for _, cmd := range []string{"--diagnose", "--my-slot"} {
		var out, errBuf bytes.Buffer
		Main([]string{cmd, "--data-dir", dir, "--log-file", logFile,
			"--relay", "ws://127.0.0.1:1", "--timeout", "300ms"}, &out, &errBuf)
		if _, err := os.Stat(logFile); !os.IsNotExist(err) {
			t.Fatalf("%s created %s; a read-only command must not touch the log of a sidecar "+
				"that may be running beside it", cmd, logFile)
		}
	}
}

// TestTheHumanFormPrintsOneLinePerCheckWithItsActor is the output contract this
// arc fixed: verdict, id, one sentence, and on anything but a PASS the taxonomy
// id and the actor.
func TestTheHumanFormPrintsOneLinePerCheckWithItsActor(t *testing.T) {
	rep := Diagnose(DiagnoseOptions{
		DataDir:  filepath.Join(t.TempDir(), "gone"),
		RelayURL: "ws://127.0.0.1:1",
		Timeout:  300 * time.Millisecond,
	})
	var buf bytes.Buffer
	RenderDiagnosis(&buf, rep)
	out := buf.String()
	for _, id := range CheckIDs {
		if !strings.Contains(out, id) {
			t.Errorf("the human form never mentions %s", id)
		}
	}
	if !strings.Contains(out, "FAIL     data-dir") {
		t.Errorf("the human form does not put the verdict and the id on one line:\n%s", out)
	}
	if !strings.Contains(out, "who acts: you") {
		t.Errorf("the human form does not name the actor:\n%s", out)
	}
	if !strings.Contains(out, "exit 1") {
		t.Errorf("the human form does not say what its exit code means:\n%s", out)
	}
	if !strings.Contains(out, "safe to send") {
		t.Errorf("the human form does not tell a participant the report carries no secret")
	}
}
