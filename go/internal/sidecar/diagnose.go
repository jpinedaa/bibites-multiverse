package sidecar

// `multiverse-sidecar --diagnose` — the twenty-one checks of
// docs/sidecar-diagnose-spec.md, run on the participant's own machine.
//
// THE SIX RULES ARE THE DESIGN, so they are restated here as the things this
// file is not allowed to do:
//
//  1. IT CHANGES NOTHING. It mints no credential, restarts nothing, sets no
//     time scale, claims no slot, opens no Contract B session and writes to no
//     journal — the journal is opened with journal.OpenReadOnly, which neither
//     compacts it nor takes a write handle. There is exactly ONE write in the
//     whole command and it is named: data-dir creates one empty temporary file
//     in the data directory and removes it again, because whether a directory
//     can be written to cannot be answered by looking at it, and a full disk or
//     a read-only mount is precisely what the check exists to catch. It never
//     touches the journal directory and never a file the sidecar owns.
//  2. IT NEVER PRINTS A SECRET. Not the map credential, not the local bearer
//     token, in whole, in prefix, hashed or elided-with-length. It prints the
//     PATH each is read from and whether that file exists, because that is the
//     fact a participant needs and it is not the secret.
//  3. IT RUNS WITHOUT THE MAP. Every check that needs a relay reports UNKNOWN
//     and names the check that blocked it. A missing precondition is never a
//     PASS and never a FAIL.
//  4. IT RUNS BESIDE A RUNNING SIDECAR. It reads the running process's own
//     state over the loopback own-slot view rather than dialling the relay a
//     second time — a second Contract B connection would be shed by
//     maxConnectionsPerPeer or would take the session over under the
//     newer-connection-replaces-older rule, which is breaking the thing being
//     measured. The two probes it does make — a TCP connect and a TLS handshake
//     — send no HTTP request at all, so the relay's per-address accounting,
//     which is keyed on a request, never sees them.
//  5. EVERY FAILURE NAMES AN ACTOR. A FAIL or a WARN carries the taxonomy id
//     and one of you / operator / nobody. That is the whole rule of
//     docs/error-taxonomy.md and a diagnostic that reported causes without
//     actors would undo it.
//  6. UNKNOWN IS A VALUE. Never rendered as a pass, never as a failure.
//
// AND IT INVENTS NO THRESHOLD. Three criteria belong to packages that have not
// measured them yet (spec §6): the stall-BREACH RATE band is WP8's, the disk
// headroom MULTIPLE is WP3's, and the applied-against-achieved speed band is
// WP8's. Where one of those is the only open question the check states its
// measurement and reports UNKNOWN naming the owner, rather than judging against
// a number this arc made up. What it does judge is arithmetic already fixed
// elsewhere: two save intervals is a save that did not happen, the sum of this
// install's own configured ceilings is space it has already promised to write,
// and LimitWarnFraction is the most this build's own pacer can emit.

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"multiverse/internal/contracta"
	"multiverse/internal/contractb"
	"multiverse/internal/journal"
	"multiverse/internal/mapwalk"
	"multiverse/internal/termsafe"
	"multiverse/internal/wire"
)

// DiagnoseSchema names the JSON shape. It is versioned, and the promise is that
// a report from an old build is still readable: a support conversation that
// starts with a pasted report is the case this exists for.
const DiagnoseSchema = "multiverse-diagnose/1"

// The five verdicts of the specification.
type Verdict string

const (
	// VerdictPass — the criterion held.
	VerdictPass Verdict = "PASS"
	// VerdictFail — the criterion did not hold and it is a fault.
	VerdictFail Verdict = "FAIL"
	// VerdictWarn — it did not hold and it is legal, or the reading is outside a
	// healthy band without being a fault.
	VerdictWarn Verdict = "WARN"
	// VerdictUnknown — not answerable. A precondition failed, or the map has not
	// said yet.
	VerdictUnknown Verdict = "UNKNOWN"
	// VerdictSkip — not applicable to this configuration.
	VerdictSkip Verdict = "SKIP"
)

// The three actors, and nothing else is one (docs/error-taxonomy.md). A compound
// value is written the way the taxonomy writes it — "nobody, then you" — because
// several of its entries genuinely have an order.
const (
	ActorYou      = "you"
	ActorOperator = "operator"
	ActorNobody   = "nobody"
)

// The exit codes. THIS IS THE CONTRACT the specification left as a slot, and it
// is deliberately small: a script can branch on it and a person can remember it.
const (
	// ExitOK — no FAIL. WARN and UNKNOWN may be present, and often are: an
	// unknown is an honest gap, not a failure, and a map that has not answered
	// yet must not make a healthy machine look broken.
	ExitOK = 0
	// ExitFail — at least one reported check FAILed.
	ExitFail = 1
	// ExitCannotRun — the diagnostic itself could not run: no data directory
	// argument, an unreadable one, or arguments it rejected. It is NOT used for
	// anything the checks found; a diagnostic that exits 2 has told you nothing.
	ExitCannotRun = 2
)

// CheckResult is one check's answer.
type CheckResult struct {
	ID      string  `json:"id"`
	Verdict Verdict `json:"verdict"`
	// Says is the one sentence. It is what the human form prints beside the id.
	Says string `json:"says"`
	// Detail is the evidence: the numbers, the paths, the readings.
	Detail []string `json:"detail,omitempty"`
	// Taxonomy and Actor are REQUIRED on FAIL and WARN (rule 5).
	Taxonomy []string `json:"taxonomy,omitempty"`
	Actor    string   `json:"actor,omitempty"`
	Remedy   string   `json:"remedy,omitempty"`
	// WaitingOn is the check whose failure blocked this one. Only on UNKNOWN,
	// and it is what turns one root cause into one FAIL and a trail of unknowns.
	WaitingOn string `json:"waitingOn,omitempty"`
}

// Summary counts the verdicts.
type Summary struct {
	Pass    int `json:"pass"`
	Fail    int `json:"fail"`
	Warn    int `json:"warn"`
	Unknown int `json:"unknown"`
	Skip    int `json:"skip"`
}

// Report is the whole of one run, and the shape --json emits.
type Report struct {
	Schema         string `json:"schema"`
	SidecarVersion string `json:"sidecarVersion"`
	GeneratedAtMs  int64  `json:"generatedAtMs"`
	DataDir        string `json:"dataDir"`
	PeerID         string `json:"peerId,omitempty"`
	RelayURL       string `json:"relayUrl,omitempty"`
	// LiveSource says where the live half came from, or why there is none. It is
	// the first thing to read when a report is full of unknowns.
	LiveSource  string        `json:"liveSource"`
	LiveRead    bool          `json:"liveRead"`
	Checks      []CheckResult `json:"checks"`
	Summary     Summary       `json:"summary"`
	Exit        int           `json:"exit"`
	ExitMeaning string        `json:"exitMeaning"`
}

// DiagnoseOptions is what the command line resolved.
type DiagnoseOptions struct {
	// DataDir is the sidecar's data directory. Everything local hangs off it.
	DataDir string
	// RelayURL, ContractATokenFile and CredentialFile are this install's
	// configuration as the flags and the environment resolved them, so that the
	// check compares what the two processes are ACTUALLY pointed at.
	RelayURL           string
	ContractATokenFile string
	CredentialFile     string
	// SecretConfigured is whether a credential secret was resolvable at all —
	// from the file or from the environment. THE VALUE IS NEVER CARRIED.
	SecretConfigured bool
	// GameDir and MatrixFile override what the install record names. On a
	// packaged install neither is needed.
	GameDir    string
	MatrixFile string
	// Only filters the REPORT and not the work: a check's precondition still has
	// to be evaluated for its answer to mean anything.
	Only []string
	// Timeout bounds EACH probe — the own-slot fetch, the TCP connect and the
	// TLS handshake — rather than the run as a whole. A diagnostic that hangs is
	// a diagnostic nobody runs twice.
	Timeout time.Duration
	Now     func() time.Time
}

// CheckIDs is every check, IN THE SPECIFICATION'S DEPENDENCY ORDER: the
// participant's own machine first, then the path to the map. data-dir before
// anything that reads a file, relay-tls before credential, slot before edges.
var CheckIDs = []string{
	// §2 — the participant's own machine.
	"data-dir",
	"stale-process",
	"contract-a-token",
	"mod-connected",
	"mod-log",
	"export-edges",
	"time-scale",
	"journal-replay",
	"journal-depths",
	"save-health",
	"disk-headroom",
	"versions",
	// §3 — the path to the map.
	"relay-reachable",
	"relay-tls",
	"credential",
	"contract-version",
	"game-version",
	"limits",
	"slot",
	"edges",
	"neighbours",
}

// DefaultDiagnoseTimeout bounds each probe. Five seconds is long enough for a
// TLS handshake over a slow link and short enough that a participant does not
// think the command hung.
const DefaultDiagnoseTimeout = 5 * time.Second

// diag is one run.
type diag struct {
	opts DiagnoseOptions
	now  time.Time
	// live is the running sidecar's own state, or the reason there is none.
	live ownSlotResult
	// jr is a READ-ONLY replay of the journal, opened beside a running sidecar.
	jr    *journal.Journal
	jrErr error
	// record is the packaged install's own record, when there is one.
	record  installRecord
	haveRec bool
	// tcp and tlsInfo are the two probes, run once and remembered.
	probe relayProbe

	results map[string]CheckResult
	order   []string
}

// Diagnose runs the checks and returns the report. It never panics on a machine
// in any state: every path that cannot answer produces an UNKNOWN.
func Diagnose(opts DiagnoseOptions) Report {
	if opts.Timeout <= 0 {
		opts.Timeout = DefaultDiagnoseTimeout
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	d := &diag{opts: opts, now: opts.Now(), results: map[string]CheckResult{}}

	// The live half, read ONCE, before any check: every check that needs live
	// state reads the same instant, so two checks cannot disagree about it.
	d.live = fetchOwnSlot(opts.DataDir, opts.Timeout)
	d.record, d.haveRec = readInstallRecord(opts.DataDir)
	d.jr, d.jrErr = journal.OpenReadOnly(filepath.Join(opts.DataDir, "journal"))
	if d.jr != nil {
		defer d.jr.Close()
	}

	for _, id := range CheckIDs {
		d.add(d.run(id))
	}

	rep := Report{
		Schema:         DiagnoseSchema,
		SidecarVersion: Version,
		GeneratedAtMs:  d.now.UnixMilli(),
		DataDir:        opts.DataDir,
		RelayURL:       opts.RelayURL,
		LiveRead:       d.live.OK,
		LiveSource:     d.liveSource(),
	}
	if d.live.OK {
		rep.PeerID = d.live.View.PeerID
	} else if id, err := os.ReadFile(filepath.Join(opts.DataDir, "peer-id")); err == nil {
		rep.PeerID = strings.TrimSpace(string(id))
	}
	for _, id := range d.order {
		if !selected(opts.Only, id) {
			continue
		}
		r := d.results[id]
		rep.Checks = append(rep.Checks, r)
		switch r.Verdict {
		case VerdictPass:
			rep.Summary.Pass++
		case VerdictFail:
			rep.Summary.Fail++
		case VerdictWarn:
			rep.Summary.Warn++
		case VerdictUnknown:
			rep.Summary.Unknown++
		case VerdictSkip:
			rep.Summary.Skip++
		}
	}
	rep.Exit, rep.ExitMeaning = ExitOK, "no check failed"
	if rep.Summary.Fail > 0 {
		rep.Exit, rep.ExitMeaning = ExitFail, "at least one check failed"
	}
	return rep
}

func (d *diag) liveSource() string {
	if d.live.OK {
		return fmt.Sprintf("the running sidecar at %s (pid %d)", d.live.Addr, d.live.View.PID)
	}
	if d.live.Why != "" {
		return "no live state: " + d.live.Why
	}
	return "no live state"
}

func selected(only []string, id string) bool {
	if len(only) == 0 {
		return true
	}
	for _, want := range only {
		if want == id {
			return true
		}
	}
	return false
}

// UnknownCheckIDs returns the ids in want that are not checks, so the command
// line can refuse them before running anything.
func UnknownCheckIDs(want []string) []string {
	known := map[string]bool{}
	for _, id := range CheckIDs {
		known[id] = true
	}
	var bad []string
	for _, id := range want {
		if !known[id] {
			bad = append(bad, id)
		}
	}
	return bad
}

func (d *diag) add(r CheckResult) {
	d.results[r.ID] = r
	d.order = append(d.order, r.ID)
}

func (d *diag) verdict(id string) Verdict { return d.results[id].Verdict }

func (d *diag) run(id string) CheckResult {
	switch id {
	case "data-dir":
		return d.checkDataDir()
	case "stale-process":
		return d.checkStaleProcess()
	case "contract-a-token":
		return d.checkContractAToken()
	case "mod-connected":
		return d.checkModConnected()
	case "mod-log":
		return d.checkModLog()
	case "export-edges":
		return d.checkExportEdges()
	case "time-scale":
		return d.checkTimeScale()
	case "journal-replay":
		return d.checkJournalReplay()
	case "journal-depths":
		return d.checkJournalDepths()
	case "save-health":
		return d.checkSaveHealth()
	case "disk-headroom":
		return d.checkDiskHeadroom()
	case "versions":
		return d.checkVersions()
	case "relay-reachable":
		return d.checkRelayReachable()
	case "relay-tls":
		return d.checkRelayTLS()
	case "credential":
		return d.checkCredential()
	case "contract-version":
		return d.checkContractVersion()
	case "game-version":
		return d.checkGameVersion()
	case "limits":
		return d.checkLimits()
	case "slot":
		return d.checkSlot()
	case "edges":
		return d.checkEdges()
	case "neighbours":
		return d.checkNeighbours()
	}
	return unknown(id, "no such check", "")
}

// ---------------------------------------------------------------- verdicts

func pass(id, says string, detail ...string) CheckResult {
	return CheckResult{ID: id, Verdict: VerdictPass, Says: says, Detail: detail}
}

func fail(id, says, remedy, actor string, taxonomy []string, detail ...string) CheckResult {
	return CheckResult{ID: id, Verdict: VerdictFail, Says: says, Remedy: remedy,
		Actor: actor, Taxonomy: taxonomy, Detail: detail}
}

func warn(id, says, remedy, actor string, taxonomy []string, detail ...string) CheckResult {
	return CheckResult{ID: id, Verdict: VerdictWarn, Says: says, Remedy: remedy,
		Actor: actor, Taxonomy: taxonomy, Detail: detail}
}

func unknown(id, says, waitingOn string, detail ...string) CheckResult {
	return CheckResult{ID: id, Verdict: VerdictUnknown, Says: says, WaitingOn: waitingOn,
		Detail: detail}
}

func skip(id, says string, detail ...string) CheckResult {
	return CheckResult{ID: id, Verdict: VerdictSkip, Says: says, Detail: detail}
}

// needLive is the one gate every check that reads the running sidecar goes
// through, so they all name the same blocking check and say the same thing about
// why there is nothing to read.
func (d *diag) needLive(id string) (CheckResult, bool) {
	if d.live.OK {
		return CheckResult{}, true
	}
	return unknown(id,
		"this needs the running sidecar's own state and there is none to read",
		"stale-process", d.live.Why), false
}

// ---------------------------------------------------------- §2, own machine

// checkDataDir is the loudest possible failure and the least obvious: a dangling
// or unmounted path fails EVERY path at once and none of them says why.
func (d *diag) checkDataDir() CheckResult {
	const id = "data-dir"
	dir := d.opts.DataDir
	if dir == "" {
		return fail(id, "no data directory was named",
			"pass --data-dir, or set MULTIVERSE_DATA_DIR", ActorYou, []string{"LOCAL-DISK"})
	}
	abs, err := filepath.Abs(dir)
	if err == nil {
		dir = abs
	}
	info, err := os.Stat(dir)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return fail(id, "the data directory does not exist",
			"check the path, and check that the volume holding it is mounted — a dangling "+
				"path fails every other check at once and none of them says why",
			ActorYou, []string{"LOCAL-DISK"}, "looked in "+dir)
	case err != nil:
		return fail(id, "the data directory could not be read",
			"check the path and its permissions", ActorYou, []string{"LOCAL-DISK"},
			"looked in "+dir, err.Error())
	case !info.IsDir():
		return fail(id, "the data directory is not a directory",
			"point --data-dir at a directory", ActorYou, []string{"LOCAL-DISK"}, dir)
	}
	// The one write this command makes, and the only true test of the thing
	// being asked: a read-only mount and a full disk both look exactly like a
	// healthy directory from a stat.
	probe, err := os.CreateTemp(dir, ".diagnose-write-probe-*")
	if err != nil {
		return fail(id, "the data directory cannot be written to",
			"free space on this volume, or fix the directory's permissions; a sidecar that "+
				"cannot journal must stop taking custody",
			ActorYou, []string{"LOCAL-DISK", "AOUT-JOURNAL_ERROR"}, dir, err.Error())
	}
	name := probe.Name()
	_ = probe.Close()
	_ = os.Remove(name)

	detail := []string{dir}
	peerID, peerErr := os.ReadFile(filepath.Join(dir, "peer-id"))
	if peerErr != nil {
		return warn(id, "this directory is writable and no sidecar has ever started against it",
			"start the sidecar; it writes its identity at first start. If you expected a world "+
				"here, check that --data-dir names the directory that world has always used — "+
				"a peer that loses its identity strands its slot and every organism addressed to it",
			ActorYou, []string{"LOCAL-DISK"}, append(detail, "no peer-id file")...)
	}
	detail = append(detail, "peer-id "+termsafe.Text(strings.TrimSpace(string(peerID))))
	if slot, err := os.ReadFile(filepath.Join(dir, "slot")); err == nil {
		detail = append(detail, "remembered slot "+termsafe.Text(strings.TrimSpace(string(slot))))
	} else {
		// Not a fault: a world that has never been granted a slot has no file to
		// remember one in, and that is the opening state of every install.
		detail = append(detail, "no slot remembered yet — this world has not been granted one")
	}
	return pass(id, "the data directory is present, writable, and holds this world's identity",
		detail...)
}

// checkStaleProcess answers "is exactly one sidecar running against this data
// directory", from two independent pieces of evidence: the process record the
// sidecar writes at start, and whatever actually answers on its address.
func (d *diag) checkStaleProcess() CheckResult {
	const id = "stale-process"
	if d.verdict("data-dir") == VerdictFail {
		return unknown(id, "nothing can be read from a data directory that does not resolve",
			"data-dir")
	}
	rec, haveRec := readProcessRecord(d.opts.DataDir)
	recAlive := haveRec && processAlive(rec.PID)

	switch {
	case d.live.OK && haveRec && recAlive && rec.PID != d.live.View.PID:
		// Two processes: one wrote the record and a different one owns the
		// listener. The journal is a single-writer file and they are both
		// writing it.
		return fail(id, "TWO sidecars are running against this data directory",
			fmt.Sprintf("stop one of them. The journal is a single-writer file: pid %d wrote "+
				"the process record and pid %d owns the listener, and two processes appending "+
				"to one custody log is how custody history is lost",
				rec.PID, d.live.View.PID),
			ActorYou, []string{"LOCAL-TWOSIDECARS"},
			fmt.Sprintf("process record pid %d, listening pid %d", rec.PID, d.live.View.PID))
	case d.live.OK:
		det := []string{fmt.Sprintf("pid %d, listening on %s, up since %s",
			d.live.View.PID, d.live.Addr,
			time.UnixMilli(d.live.View.StartedAt).Format("2006-01-02 15:04:05"))}
		if !haveRec {
			det = append(det, "no process record on disk; this sidecar predates the record "+
				"or could not write it")
		}
		return pass(id, "one sidecar is running against this data directory", det...)
	case d.live.Predates:
		return unknown(id,
			"a sidecar is listening for this directory and is a build older than this one",
			"", d.live.Why,
			"restart it from this release to make the live half of this report answerable")
	case recAlive:
		return warn(id, "a process record names a live process that did not answer its own view",
			"check whether that process is really this sidecar — pid numbers are reused, so a "+
				"record alone is not evidence. If it is not, remove "+
				filepath.Join(d.opts.DataDir, processRecordName),
			ActorYou, []string{"LOCAL-STALEPID"},
			fmt.Sprintf("record pid %d, listen %s, started %s", rec.PID, rec.Listen,
				time.UnixMilli(rec.StartedAt).Format("2006-01-02 15:04:05")),
			d.live.Why)
	case haveRec:
		return warn(id, "a stale process record is left over from a sidecar that is gone",
			"none needed — the next start overwrites it. It is worth knowing because a record "+
				"like this makes a status query claim the thing is running when it is not, and "+
				"pid numbers are reused after a reboot",
			ActorNobody, []string{"LOCAL-STALEPID"},
			fmt.Sprintf("record names pid %d, which is not running", rec.PID))
	default:
		return unknown(id, "no sidecar is running against this data directory",
			"", d.live.Why,
			"the cold checks below still answer; every check that needs live state will not")
	}
}

// checkContractAToken is the single cause of the local authentication refusal,
// and it is invisible from either process alone (contract-a.md §21, A47).
func (d *diag) checkContractAToken() CheckResult {
	const id = "contract-a-token"
	if d.verdict("data-dir") == VerdictFail {
		return unknown(id, "the token file lives in the data directory", "data-dir")
	}
	// What THIS invocation resolves, which is what the start script gives the
	// mod: the flag, then MULTIVERSE_CONTRACT_A_TOKEN_FILE, then the default.
	mine := d.opts.ContractATokenFile
	if mine == "" {
		mine = filepath.Join(d.opts.DataDir, "contract-a.token")
	}
	detail := []string{"this configuration reads " + mine}

	if d.live.OK && d.live.View.ContractAInsecure {
		return warn(id, "this sidecar is running with the Contract A token check DISABLED",
			"stop passing --insecure-no-contract-a-token. Any local process can drive this "+
				"world's migrations and impersonate this sidecar to the mod; it exists for a "+
				"single-machine rehearsal and no document this project ships tells a player to "+
				"pass it",
			ActorYou, []string{"A-401"}, detail...)
	}
	if d.live.OK {
		theirs := d.live.View.ContractATokenFile
		detail = append(detail, "the running sidecar reads "+theirs)
		if !samePath(mine, theirs) {
			return fail(id, "the mod and this sidecar are not reading the same token file",
				"point both at one path. The running sidecar reads "+theirs+" and this "+
					"configuration names "+mine+" — a mismatch between the two is a hand-edited "+
					"start script, and it is the whole of the local authentication refusal",
				ActorYou, []string{"A-401"}, detail...)
		}
		if n := d.live.View.ContractAAuthFailures; n > 0 {
			return fail(id, fmt.Sprintf("this sidecar has refused %d mod connections for a bad token", n),
				"the mod is presenting a token this sidecar does not accept. Point both at the "+
					"same file; to rotate it, delete the file and restart the sidecar — the mod "+
					"re-reads it on its next dial. This is NOT the relay credential",
				ActorYou, []string{"A-401"}, detail...)
		}
	}
	info, err := os.Stat(mine)
	if errors.Is(err, os.ErrNotExist) {
		return fail(id, "the Contract A token file does not exist",
			"start the sidecar: it mints the file at first start, mode 0600. If the game is "+
				"pointed somewhere else, MULTIVERSE_CONTRACT_A_TOKEN_FILE is the setting the "+
				"start script puts the path in",
			ActorYou, []string{"A-401"}, detail...)
	}
	if err != nil {
		return fail(id, "the Contract A token file cannot be read",
			"fix the file's permissions; both processes on this machine read it",
			ActorYou, []string{"A-401"}, append(detail, err.Error())...)
	}
	if runtime.GOOS == "windows" {
		// Windows does not carry POSIX mode bits, and the installer sets a
		// user-only ACL instead. Reporting 0600 here would be a claim about
		// something that was not measured.
		return pass(id, "both processes read one token file, and it is present",
			append(detail, "file mode is not checked on Windows: the installer sets a "+
				"user-only ACL instead, which mode bits do not describe")...)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		return fail(id, fmt.Sprintf("the Contract A token file is mode %04o and not 0600", mode),
			fmt.Sprintf("chmod 600 %s. Every other account on this machine can currently read "+
				"the token that authorises driving this world's migrations", mine),
			ActorYou, []string{"A-401"}, detail...)
	}
	return pass(id, "both processes read one token file, present and owner-only", detail...)
}

// checkModConnected is readiness read off the sidecar's own state rather than a
// log. On a failure, mod-log is the check that says WHICH of the two traps it is.
func (d *diag) checkModConnected() CheckResult {
	const id = "mod-connected"
	res, ok := d.needLive(id)
	if !ok {
		return res
	}
	m := d.live.View.Mod
	if !m.Connected {
		return fail(id, "no game is connected to this sidecar",
			"read mod-log next: this symptom has two causes with different remedies, and "+
				"neither of them looks like a configuration problem from here",
			ActorYou, []string{"LOCAL-CONFIGRACE", "LOCAL-STARVATION"},
			"the map shows this slot live with no game behind it, which is the loudest-looking "+
				"symptom in this system")
	}
	age := time.Duration(m.LastHeartbeatAge) * time.Millisecond
	deadline := time.Duration(m.HeartbeatTimeout) * time.Millisecond
	det := []string{fmt.Sprintf("last heartbeat %s ago, against a %s deadline",
		age.Round(time.Millisecond), deadline)}
	if m.LastHeartbeatAge > 0 && age > deadline {
		return fail(id, "a game is connected and its heartbeats have stopped",
			"the session is about to close with 4004 and the mod will reconnect. The ordinary "+
				"cause is a world save blocking the thread the heartbeat is composed on; "+
				"arrivals are held in the journal, in order, for the whole silence and nothing "+
				"is lost",
			ActorNobody, []string{"A-4004", "LOCAL-SAVESTALL"}, det...)
	}
	return pass(id, "a game is connected and its heartbeats are arriving", det...)
}

// checkModLog tells the config race from log-file starvation. THE TELL IS AN
// ABSENCE, which is why a person who does not know both traps exist cannot tell
// them apart.
func (d *diag) checkModLog() CheckResult {
	const id = "mod-log"
	if d.verdict("mod-connected") == VerdictPass {
		return skip(id, "a game is connected, so there is no trap to tell apart")
	}
	gameDir := d.gameDir()
	if gameDir == "" {
		return unknown(id, "this check needs the game folder and nothing names one here",
			"", "pass --game-dir, or run this on a packaged install, whose install-record.json "+
				"names the folder the plugin was installed into")
	}
	logPath := filepath.Join(gameDir, "BepInEx", "LogOutput.log")
	plugin := filepath.Join(gameDir, "BepInEx", "plugins", "BibitesMultiverse.dll")
	body, err := os.ReadFile(logPath)
	if errors.Is(err, os.ErrNotExist) {
		return d.starvation(id, logPath, plugin, "there is no mod log at all")
	}
	if err != nil {
		return unknown(id, "the mod log could not be read", "", logPath, err.Error())
	}
	text := string(body)
	if strings.Contains(text, "[M2] configuration failed") {
		return fail(id, "the mod's configuration failed and its multiverse client stayed off",
			"restart THAT ONE game instance. Two mod instances rewrote the same configuration "+
				"file at once and one lost; the file is complete after the first winner writes "+
				"it, so the race cannot recur",
			ActorYou, []string{"LOCAL-CONFIGRACE"},
			"found `[M2] configuration failed — the multiverse client stays off:` in "+logPath)
	}
	if !strings.Contains(text, "Bibites Multiverse") || !strings.Contains(text, "loaded") {
		return d.starvation(id, logPath, plugin,
			"the mod log has no `Bibites Multiverse … loaded` line in it")
	}
	return unknown(id, "the mod log shows the plugin loading and does not name either trap",
		"", logPath,
		"send the lines either side of the last connection attempt: this is the case the two "+
			"named traps do not cover")
}

func (d *diag) starvation(id, logPath, plugin, saw string) CheckResult {
	det := []string{saw, "looked in " + logPath, "and for the plugin at " + plugin}
	if _, err := os.Stat(plugin); errors.Is(err, os.ErrNotExist) {
		det = append(det, "the plugin is NOT at that path, which on a one-world install is the "+
			"likelier reading of the same absence")
	}
	return fail(id, "the mod never loaded in that game instance, and the tell is an absence",
		"restart THAT ONE game instance, so its environment matches the others exactly. The mod "+
			"framework hands out a fixed number of log files and then gives up, and an instance "+
			"that gets no log file does not merely lose its log — the mod never loads in it. On "+
			"a single-world install the same absence more often means the plugin is not in "+
			"BepInEx\\plugins, or BepInEx never installed: check that path and re-run the "+
			"installer if it is empty",
		ActorYou, []string{"LOCAL-STARVATION"}, det...)
}

// checkExportEdges is A50 at the join of its two inputs: what this world
// declared, and what shape the map actually is.
func (d *diag) checkExportEdges() CheckResult {
	const id = "export-edges"
	res, ok := d.needLive(id)
	if !ok {
		return res
	}
	v := d.live.View
	if !v.Mod.Connected {
		return unknown(id, "the declared export set comes from the game's handshake", "mod-connected")
	}
	if len(v.Mod.ExportEdges) == 0 {
		return warn(id, "this world declared no export edges",
			"unset MULTIVERSE_EXPORT_EDGES for the shipped default of all four edges. A world "+
				"with none exports nothing and still receives on every edge it declared",
			ActorYou, []string{"A-4007"})
	}
	shape := contractb.MapShape{Width: v.Slot.MapWidth, Height: v.Slot.MapHeight}
	declared := edgeSetString(v.Mod.ExportEdges)
	if !mapHasAnAxis(shape) {
		return pass(id, "the map has no axis yet, so no declaration can be unusable and none is refused",
			"declared "+declared+" on a "+shapeString(shape)+" map",
			"a lone first peer on a map that has not grown is the normal opening state of every map")
	}
	usable, unusable := splitDeclaredEdges(v.Mod.ExportEdges, shape)
	if len(usable) == 0 {
		return fail(id, "every edge this world declared lies on an axis this map does not have",
			"set MULTIVERSE_EXPORT_EDGES to include an edge on an axis the map has, or unset it "+
				"entirely for the shipped default of all four edges. No declared edge can ever "+
				"carry an organism as it stands, and the sidecar refuses the session for it",
			ActorYou, []string{"A-4007"},
			"declared "+declared+" on a "+shapeString(shape)+" map")
	}
	if len(unusable) > 0 {
		return warn(id, "some declared edges lie on an axis this map does not have",
			// A50: the warning deliberately states no remedy, because the remedy
			// would be a map that grows an axis and that is nobody at this
			// machine's to apply.
			"none, and that is deliberate: these edges stay closed for the life of this map "+
				"shape, no organism is affected, and the only thing that would change it is a "+
				"map that grows an axis",
			ActorNobody, []string{"LANE-A50-partial"},
			"declared "+declared+" on a "+shapeString(shape)+" map",
			"unusable: "+edgeSetString(unusable))
	}
	return pass(id, "every declared export edge lies on an axis this map has",
		"declared "+declared+" on a "+shapeString(shape)+" map")
}

// checkTimeScale reports the applied speed and the achieved speed, and does not
// judge the gap between them.
//
// TWO HALVES ARE MISSING AND NEITHER IS THIS ARC'S TO INVENT. The first is a
// CONFIGURED TARGET: time scale is report-only on both wires, there is no
// message that sets one and no setting that records one, so "the speed it was
// told to" is not a value this machine holds — the control surface that would
// carry it is D23's, in M6. The second is the BAND between applied and achieved
// that is a WARN, which spec §6 assigns to WP8 and Risk 9 explains: at a speed a
// machine cannot meet the two come apart completely, and how far apart is
// tolerable is a measurement nobody has taken yet.
//
// So this reports both numbers and the two traps, and says what it is not
// judging. The gap is the reading, and a reader can see it.
func (d *diag) checkTimeScale() CheckResult {
	const id = "time-scale"
	res, ok := d.needLive(id)
	if !ok {
		return res
	}
	m := d.live.View.Mod
	if !m.Connected {
		return unknown(id, "a world's speed comes from its game's heartbeat", "mod-connected")
	}
	if m.TimeScale == nil {
		return unknown(id, "this world has not reported a speed yet", "mod-connected",
			"absent is a world that has not said; 0 would be a world standing still, which is a "+
				"reading")
	}
	det := []string{fmt.Sprintf("applied x%g", *m.TimeScale)}
	if m.AchievedTimeScale != nil {
		det = append(det, fmt.Sprintf("achieved x%.2f over the last %s",
			*m.AchievedTimeScale,
			(time.Duration(m.AchievedSpanMs)*time.Millisecond).Round(time.Second)))
		if *m.TimeScale > 0 {
			det = append(det, fmt.Sprintf("achieved is %.0f%% of applied",
				100**m.AchievedTimeScale/(*m.TimeScale)))
		}
	} else {
		det = append(det, "achieved is not measurable yet: it needs ten seconds of continuous "+
			"heartbeats from one session")
	}
	det = append(det,
		"there is no configured target to compare the applied value against: time scale is "+
			"report-only on both wires and no message sets one",
		"the band between applied and achieved that is a warning is WP8's to measure, so this "+
			"check states the two numbers and does not judge the gap",
		"two traps belong here: a world restores its own speed after it settles, which can land "+
			"AFTER a speed command; and the first speed command after a world load reports 1.00 "+
			"and sticks there — a second, later one takes")
	return unknown(id, "the speed readings are here and the band that would judge them is not",
		"", det...)
}

// checkJournalReplay: zero discarded bytes, on every start, for every healthy
// journal. Anything else is custody history that is gone.
func (d *diag) checkJournalReplay() CheckResult {
	const id = "journal-replay"
	if d.verdict("data-dir") == VerdictFail {
		return unknown(id, "the journal lives in the data directory", "data-dir")
	}
	// The running sidecar's own number is the authority: it is what THAT replay
	// discarded, and a later start compacts the damage away.
	if d.live.OK {
		lost := d.live.View.Custody.JournalDiscardedBytes
		if lost > 0 {
			return fail(id, fmt.Sprintf("this sidecar's journal replay discarded %d bytes at its last start", lost),
				"free disk, then report the byte count. Complete records behind the tear are "+
					"gone and the count is the only evidence that ever existed — it is not "+
					"recoverable afterwards",
				ActorYou, []string{"LOCAL-JOURNALTORN"},
				"a torn journal is ordinarily a full disk, sometimes a hard kill")
		}
		return pass(id, "the journal replayed cleanly at this sidecar's last start",
			"zero discarded bytes")
	}
	if d.jrErr != nil {
		return unknown(id, "the journal could not be replayed", "", d.jrErr.Error())
	}
	if _, err := os.Stat(d.jr.Path()); errors.Is(err, os.ErrNotExist) {
		return unknown(id, "this data directory holds no journal yet", "data-dir",
			"nothing has taken custody here, so there has been no replay to be clean or torn")
	}
	if lost := d.jr.Discarded(); lost > 0 {
		return fail(id, fmt.Sprintf("replaying this journal now discards %d bytes behind a torn record", lost),
			"free disk, then report the byte count. Complete records behind the tear are gone",
			ActorYou, []string{"LOCAL-JOURNALTORN"},
			"read from "+d.jr.Path()+" without opening it for writing")
	}
	return pass(id, "the journal replays cleanly", "zero discarded bytes, read without a write handle")
}

// checkJournalDepths reads the depths against THIS WORLD'S OWN CONFIGURED RATE
// rather than against a default, which has been changed three times.
func (d *diag) checkJournalDepths() CheckResult {
	const id = "journal-depths"
	if d.verdict("data-dir") == VerdictFail {
		return unknown(id, "the journal lives in the data directory", "data-dir")
	}
	var c CustodyState
	switch {
	case d.live.OK:
		c = d.live.View.Custody
	case d.jrErr != nil:
		return unknown(id, "the journal could not be read", "", d.jrErr.Error())
	default:
		states := d.jr.List()
		c.JournalLive = len(states)
		c.JournalBytes = d.jr.Size()
		c.CustodyDepth = d.jr.CountPending(journal.Out) + d.jr.CountPending(journal.In)
		for _, st := range states {
			if st.Direction == journal.In && st.Status == journal.StatusOpen {
				c.PacedDepth++
			}
			if st.Direction == journal.Out && st.Handoff == journal.HandoffHeld &&
				(st.Status == journal.StatusOpen || st.Status == journal.StatusInFlight) {
				c.HeldDepth++
			}
		}
		c.OldestPacedAgeMs, c.OldestHeldAgeMs = oldestWaiting(states, d.now)
		c.InboundRatePerSimMinute = contracta.InboundRatePerSimMinute
		if _, err := os.Stat(d.jr.Path()); errors.Is(err, os.ErrNotExist) {
			return pass(id, "this data directory holds no journal yet, so nothing is queued",
				"nothing has taken custody here")
		}
	}
	det := []string{fmt.Sprintf("in custody %d, paced %d, held %d, bounced home on a timeout %d",
		c.CustodyDepth, c.PacedDepth, c.HeldDepth, c.BouncedTimeoutTotal)}
	if !d.live.OK {
		det = append(det, "read from the journal on disk; the configured delivery rate is this "+
			"build's default because no running sidecar named one")
	}
	if c.HeldDepth > 0 {
		det = append(det, fmt.Sprintf("the oldest held entry has waited %s — a hold is designed "+
			"behaviour and its clock runs only while the destination is dark AND this sidecar "+
			"can see it", roundMs(c.OldestHeldAgeMs)))
	}
	if c.PacedDepth == 0 {
		return pass(id, "nothing is queued behind the delivery rate", det...)
	}

	// Is the paced queue draining at the rate this world configured? The
	// arithmetic is the configuration's own and no threshold is invented: at
	// `inboundRatePerSimMinute` releases per SIMULATED minute, and an applied
	// time scale of x, the current depth needs depth/rate simulated minutes,
	// which is depth/rate*60/x wall seconds. An entry that has waited longer
	// than the WHOLE queue should take is a queue that is not draining.
	rate := c.InboundRatePerSimMinute
	scale := 1.0
	if d.live.OK && d.live.View.Mod.TimeScale != nil && *d.live.View.Mod.TimeScale > 0 {
		scale = *d.live.View.Mod.TimeScale
	}
	det = append(det, fmt.Sprintf("this world's own delivery rate is %.1f per simulated minute, "+
		"at an applied speed of x%g", rate, scale))
	if rate <= 0 {
		return unknown(id, "the queue's depth cannot be read against a delivery rate of zero", "")
	}
	expected := time.Duration(float64(c.PacedDepth) / rate * 60 / scale * float64(time.Second))
	waited := time.Duration(c.OldestPacedAgeMs) * time.Millisecond
	det = append(det, fmt.Sprintf("the oldest paced entry has waited %s; the whole queue should "+
		"drain in about %s at that rate", waited.Round(time.Second), expected.Round(time.Second)))
	if waited > expected && waited > 30*time.Second {
		return warn(id, "the paced queue is not draining at this world's configured delivery rate",
			"raise --inbound-rate (MULTIVERSE_INBOUND_RATE): a paced depth that never falls names "+
				"a delivery rate set too low. Nothing is lost while it is dammed — the entries "+
				"are in the journal and released in order",
			ActorYou, []string{"AOUT-RATE_LIMITED", "BMIG-OVERLOADED"}, det...)
	}
	return pass(id, "the queues are shallow and moving at this world's configured rate", det...)
}

// checkSaveHealth. The consequence of a long stall is a session churn and a
// short delivery pause, NOT a lost organism: arrivals are held in order for the
// whole silence.
func (d *diag) checkSaveHealth() CheckResult {
	const id = "save-health"
	res, ok := d.needLive(id)
	if !ok {
		return res
	}
	m := d.live.View.Mod
	if !m.Connected {
		return unknown(id, "a save receipt arrives on the game's heartbeat", "mod-connected")
	}
	if m.SaveMinutes != nil && *m.SaveMinutes == 0 {
		return skip(id, "this world's save timer is off, so there is no schedule to be late for",
			"saveMinutes is 0, which is a reading and not an absence")
	}
	if m.LastSave == nil {
		return unknown(id, "no save receipt has arrived on this session", "",
			"the field is optional: a mod that omits it is conformant, and a world that has not "+
				"saved since this session began has nothing to report yet")
	}
	age := d.now.Sub(time.UnixMilli(m.LastSave.AtMs))
	det := []string{fmt.Sprintf("the last save completed %s ago, took %d ms and wrote %d bytes",
		age.Round(time.Second), m.LastSave.DurationMs, m.LastSave.Bytes)}
	if m.SaveMinutes != nil {
		det = append(det, fmt.Sprintf("this world's configured interval is %g minutes, keeping %s",
			*m.SaveMinutes, keepText(m.SaveKeep)))
	}
	// THE BREACH-RATE BAND IS WP8'S (spec §6) and this check does not invent
	// one. What it can say is what this one save did against D14's budget, and
	// that a rate needs a record rather than a reading.
	const stallBudgetMs = 2000
	if m.SaveMinutes != nil && *m.SaveMinutes > 0 {
		// Two intervals is arithmetic, not a threshold: one interval may simply
		// not have elapsed yet, and after two a save that was due has not
		// happened.
		missed := time.Duration(*m.SaveMinutes*2*60) * time.Second
		if age > missed {
			return warn(id, "at least one scheduled save has not happened",
				"check the game's own log for a save that failed or a world that is paused. "+
					"Nothing about custody is at risk from this — arrivals are journaled — but a "+
					"world that is not saving loses its own history on a crash",
				ActorYou, []string{"LOCAL-SAVESTALL"}, det...)
		}
	}
	if m.LastSave.DurationMs > stallBudgetMs {
		return warn(id, fmt.Sprintf("the last save took %d ms, over the 2000 ms stall budget",
			m.LastSave.DurationMs),
			"usually none. A save blocks the thread the heartbeat is composed on, so a long one "+
				"costs a Contract A session and a short delivery pause and never an organism. If "+
				"nearly every save does it, save less often: raise MULTIVERSE_SAVE_MINUTES",
			ActorNobody+", then "+ActorYou, []string{"LOCAL-SAVESTALL", "A-4004"},
			append(det, "how OFTEN a breach is too often is a band WP8 measures from the "+
				"playtest's record; this check reports the reading and does not judge the rate")...)
	}
	return pass(id, "saves are completing, on schedule, inside their stall budget", det...)
}

// checkDiskHeadroom. Nothing here shrinks a record: the durable files grow with
// traffic, and a full disk has previously torn an append-only log and left
// thousands of zero-byte scratch files at the moment inodes were what had run
// out.
func (d *diag) checkDiskHeadroom() CheckResult {
	const id = "disk-headroom"
	if d.verdict("data-dir") == VerdictFail {
		return unknown(id, "there is no volume to measure without a data directory", "data-dir")
	}
	free, ok := freeBytes(d.opts.DataDir)
	if !ok {
		return unknown(id, "the free space on this volume could not be read", "",
			"looked at the volume holding "+d.opts.DataDir)
	}
	// The three ceilings this install has already promised itself it may write.
	// Every one is read from this install's own configuration, never invented:
	// the genome cache cap is the sidecar's, the log ceiling is the rotation
	// size times the number kept, and the journal's own ceiling is what a
	// compaction leaves plus the frame ceiling it may add before the next one.
	genomeCap := contractb.GenomeCacheMaxBytes
	logCap := int64(defaultLogCeilingBytes)
	journalCap := int64(wire.MaxFrameBytes) * 8
	if d.live.OK && d.live.View.Custody.JournalBytes > journalCap {
		journalCap = d.live.View.Custody.JournalBytes
	}
	promised := genomeCap + logCap + journalCap
	det := []string{
		fmt.Sprintf("%s free on the volume holding %s", bytesText(free), d.opts.DataDir),
		fmt.Sprintf("this install's own ceilings add up to %s: genome cache %s, logs %s, "+
			"journal %s", bytesText(promised), bytesText(genomeCap), bytesText(logCap),
			bytesText(journalCap)),
		"the log figure is the shipped rotation ceiling, logRotateMb x (logKeep+1); the " +
			"unrotated sidecar.log.out beside it and this machine's world saves are not in it",
	}
	if free < promised {
		return fail(id, "there is less room left than this install has already promised to write",
			"free space on this volume and keep it free. Nothing in this system shrinks a "+
				"durable record: the genome cache, the journal and the logs grow with traffic, "+
				"and a full disk has previously torn an append-only custody log",
			ActorYou, []string{"LOCAL-DISK", "AOUT-JOURNAL_FULL", "AOUT-JOURNAL_ERROR"}, det...)
	}
	// THE MULTIPLE OF THOSE CEILINGS THAT COUNTS AS HEADROOM IS WP3'S (spec §6),
	// together with the per-participant growth arithmetic behind it. Above one
	// multiple this check has a measurement and no criterion, and says so.
	return unknown(id, "there is room for the ceilings and no published figure for how much more there should be",
		"", append(det, "the multiple of those ceilings that counts as headroom, and the "+
			"per-participant growth arithmetic behind it, is WP3's to publish — it owns the "+
			"retention rule")...)
}

// checkVersions always passes; it is a report, in the order a support
// conversation needs them (docs/error-taxonomy.md §8).
func (d *diag) checkVersions() CheckResult {
	const id = "versions"
	game, mod, contractA := "unknown", "unknown", "unknown"
	if d.live.OK && d.live.View.Mod.Connected {
		game = orUnknown(d.live.View.Mod.GameVersion)
		mod = orUnknown(d.live.View.Mod.ModVersion)
		contractA = orUnknown(d.live.View.Mod.ContractAVersion)
	}
	det := []string{
		"game " + termsafe.Text(game),
		"mod " + termsafe.Text(mod),
		"sidecar " + Version,
		"contract A " + wire.ProtocolA + " (this build); the mod's session speaks " +
			termsafe.Text(contractA),
		"contract B " + wire.ProtocolB,
	}
	if !d.live.OK {
		det = append(det, "the game and mod versions come from a running sidecar's own session "+
			"and there is none: "+d.live.Why)
	}
	return pass(id, "the five versions a support conversation asks for", det...)
}

// ------------------------------------------------------------ §3, the map

// relayProbe is the result of the two network probes, run once.
type relayProbe struct {
	done   bool
	host   string
	port   string
	scheme string
	// resolveErr, dialErr and tlsErr are the three distinct failures
	// relay-reachable has to tell apart, plus the certificate one.
	resolveErr error
	dialErr    error
	timedOut   bool
	addrs      []string
	tlsErr     error
	cert       *x509.Certificate
	parseErr   error
}

func (d *diag) relay() relayProbe {
	if d.probe.done {
		return d.probe
	}
	d.probe.done = true
	u, err := url.Parse(d.opts.RelayURL)
	if err != nil || u.Host == "" {
		d.probe.parseErr = fmt.Errorf("%q is not a relay URL", d.opts.RelayURL)
		return d.probe
	}
	d.probe.scheme = u.Scheme
	host, port, err := net.SplitHostPort(u.Host)
	if err != nil {
		host = u.Host
		port = "443"
		if u.Scheme == "ws" {
			port = "80"
		}
	}
	d.probe.host, d.probe.port = host, port

	// 1. Does the name resolve. A literal address resolves trivially and this
	// still answers, which is what makes "name does not resolve" a distinct
	// finding rather than an inference from a failed dial.
	resolver := &net.Resolver{}
	ctx, cancel := context.WithTimeout(context.Background(), d.opts.Timeout)
	addrs, err := resolver.LookupHost(ctx, host)
	cancel()
	if err != nil {
		d.probe.resolveErr = err
		return d.probe
	}
	d.probe.addrs = addrs

	// 2. Does it accept a connection. A TCP connect and an immediate close: no
	// HTTP request is sent, so the relay's per-address accounting — which is
	// keyed on a request — never counts it, and no WebSocket exists to be shed.
	dialer := &net.Dialer{Timeout: d.opts.Timeout}
	conn, err := dialer.Dial("tcp", net.JoinHostPort(host, port))
	if err != nil {
		d.probe.dialErr = err
		var nerr net.Error
		if errors.As(err, &nerr) && nerr.Timeout() {
			d.probe.timedOut = true
		}
		return d.probe
	}
	if d.probe.scheme != "wss" {
		conn.Close()
		return d.probe
	}

	// 3. Does the certificate verify against THIS MACHINE'S trust store. No
	// configuration is set and none may be (contract-b-m4.md §22, B23): the
	// handshake takes the platform default, which verifies the chain, the name
	// and the validity window, exactly as the sidecar's own dial does.
	tconn := tls.Client(conn, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
	// The socket's deadline is on the WALL clock and not on d.now: d.now is the
	// reading this check reports and compares against the certificate, and a
	// test that sets it would otherwise starve a real handshake.
	_ = tconn.SetDeadline(time.Now().Add(d.opts.Timeout))
	if err := tconn.Handshake(); err != nil {
		d.probe.tlsErr = err
		// The presented certificate is worth having even when it did not verify:
		// naming what was presented is half of what makes the failure readable.
		if state := tconn.ConnectionState(); len(state.PeerCertificates) > 0 {
			d.probe.cert = state.PeerCertificates[0]
		} else {
			d.probe.cert = peekCertificate(host, port, d.opts.Timeout)
		}
	} else if state := tconn.ConnectionState(); len(state.PeerCertificates) > 0 {
		d.probe.cert = state.PeerCertificates[0]
	}
	tconn.Close()
	return d.probe
}

// peekCertificate re-handshakes WITHOUT verification for the single purpose of
// reading what the server presented, so the failure can name it.
//
// IT IS NOT A FALLBACK AND IT CONNECTS NOTHING. No frame is sent, the connection
// is closed immediately, and nothing anywhere in this program will speak to a
// relay whose certificate did not verify — B23 forbids skipping verification,
// pinning, prompting and falling back, and none of those happens here. What this
// buys is a failure that says WHICH certificate failed instead of only that one
// did.
func peekCertificate(host, port string, timeout time.Duration) *x509.Certificate {
	dialer := &net.Dialer{Timeout: timeout}
	conn, err := dialer.Dial("tcp", net.JoinHostPort(host, port))
	if err != nil {
		return nil
	}
	defer conn.Close()
	//nolint:gosec // Verification is deliberately off for a read of the presented
	// chain, and this connection is used for nothing else.
	tconn := tls.Client(conn, &tls.Config{ServerName: host, InsecureSkipVerify: true,
		MinVersion: tls.VersionTLS12})
	_ = tconn.SetDeadline(time.Now().Add(timeout))
	if err := tconn.Handshake(); err != nil {
		return nil
	}
	defer tconn.Close()
	if state := tconn.ConnectionState(); len(state.PeerCertificates) > 0 {
		return state.PeerCertificates[0]
	}
	return nil
}

// checkRelayReachable distinguishes name does not resolve from connects nowhere
// from connects and hangs, and stops guessing.
func (d *diag) checkRelayReachable() CheckResult {
	const id = "relay-reachable"
	p := d.relay()
	if p.parseErr != nil {
		return fail(id, "the relay address is not a URL",
			"the address comes from the join string the operator handed over; re-apply it. It "+
				"is always wss:// and no part of this software falls back",
			ActorYou, []string{"INS-JOINSTRING"}, p.parseErr.Error())
	}
	where := net.JoinHostPort(p.host, p.port)
	if p.resolveErr != nil {
		return fail(id, "the relay's name does not resolve on this machine",
			"check the name in your join string and this machine's DNS. This is one of three "+
				"different failures and it is the one that never reached a network",
			ActorYou, []string{"B-401"},
			"looked up "+termsafe.Text(p.host), termsafe.Text(p.resolveErr.Error()))
	}
	if p.timedOut {
		return fail(id, "the relay's address accepts nothing and the connection hangs",
			"a hang rather than a refusal is ordinarily a firewall dropping the packets rather "+
				"than answering. On your own network it is a rule and a port forward; on one you "+
				"do not administer, the operator is the one who can say whether the port is open",
			ActorYou+", then "+ActorOperator, []string{"B-401"},
			"connecting to "+where+" timed out after "+d.opts.Timeout.String())
	}
	if p.dialErr != nil {
		return fail(id, "the relay's address resolves and connects nowhere",
			"the name is right and nothing is listening behind it, or something between here "+
				"and there refused. Check the port in your join string; then ask the operator "+
				"whether the relay is up",
			ActorYou+", then "+ActorOperator, []string{"B-401"},
			"connecting to "+where+" was refused", termsafe.Text(p.dialErr.Error()))
	}
	return pass(id, "the relay's address resolves and accepts a connection",
		"connected to "+where+" and closed again without sending anything",
		"resolved to "+strings.Join(p.addrs, ", "))
}

// checkRelayTLS. There is no skip and no prompt, and the check tests this
// machine's clock too, because a wrong clock fails a valid certificate and the
// two produce the same error.
func (d *diag) checkRelayTLS() CheckResult {
	const id = "relay-tls"
	p := d.relay()
	if p.scheme != "wss" {
		return skip(id, "this relay address is not wss://, so there is no certificate to verify",
			"a public map is always wss://, and a ws:// address in a join string is refused by "+
				"the installer rather than accepted quietly; a plaintext loopback relay is a "+
				"single-machine rehearsal")
	}
	if d.verdict("relay-reachable") == VerdictFail {
		return unknown(id, "a certificate cannot be verified over a connection that was not made",
			"relay-reachable")
	}
	det := []string{"this machine's clock reads " + d.now.UTC().Format(time.RFC3339)}
	if p.cert != nil {
		det = append(det,
			"the relay presented a certificate for "+termsafe.Clip(certNames(p.cert), 200),
			fmt.Sprintf("valid from %s to %s",
				p.cert.NotBefore.UTC().Format(time.RFC3339),
				p.cert.NotAfter.UTC().Format(time.RFC3339)))
	}
	if p.tlsErr == nil {
		return pass(id, "the relay's certificate verifies against this machine's trust store", det...)
	}
	det = append(det, termsafe.Text(p.tlsErr.Error()))

	// The clock half. It is asymmetric, and the asymmetry is the whole value: a
	// certificate cannot be issued in the future, so a clock BEFORE notBefore is
	// almost certainly this machine's fault. A clock after notAfter is genuinely
	// ambiguous — an expired certificate and a fast clock produce the same error
	// — and the check says so rather than picking one.
	if p.cert != nil && d.now.Before(p.cert.NotBefore) {
		return fail(id, "the certificate is not yet valid, and THIS MACHINE'S CLOCK IS BEHIND IT",
			"fix this machine's clock first. A certificate cannot be issued in the future, so a "+
				"clock reading earlier than the certificate's issue date is the likelier of the "+
				"two causes, and it is the one you can fix",
			ActorYou, []string{"B-TLS"}, det...)
	}
	if p.cert != nil && d.now.After(p.cert.NotAfter) {
		return fail(id, "the certificate's validity window has passed, on this machine's clock",
			"check this machine's clock BEFORE reporting an expired certificate: a fast clock "+
				"and an expired certificate produce the same error and cannot be told apart from "+
				"here. If the clock is right, the certificate is the operator's to renew, and no "+
				"client-side action makes it safe",
			ActorYou+", then "+ActorOperator, []string{"B-TLS"}, det...)
	}
	return fail(id, "the relay's certificate did not verify against this machine's trust store",
		"check this machine's clock and its trust store first. If both are right it is the "+
			"relay's certificate, and no client-side action makes it safe: this software will "+
			"not skip verification, pin a certificate, or fall back to ws://",
		ActorYou+", then "+ActorOperator, []string{"B-TLS"}, det...)
}

// checkCredential distinguishes refused at the door from refused at the
// handshake for an identity mismatch. NEITHER REACHES ANY SLOT'S lastRefusal, so
// the status page is silent on both and this is where a participant learns which
// one it is.
//
// It does not dial. A second Contract B session is the one thing rule 4 forbids,
// so the answer comes from what the running sidecar already learned.
func (d *diag) checkCredential() CheckResult {
	const id = "credential"
	// The cold half first: a credential that is not configured at all is
	// answerable with no sidecar and no map.
	if !d.opts.SecretConfigured {
		det := []string{"no credential secret is configured"}
		if d.opts.CredentialFile != "" {
			det = append(det, "--credential-file names "+d.opts.CredentialFile+
				", and that file is missing or empty")
		}
		return fail(id, "this world has no credential to present",
			"re-apply the join string: the secret goes in the file --credential-file names, or "+
				"in MULTIVERSE_PEER_SECRET, and never on a command line. If you have lost it "+
				"there is no software recovery — ask the operator for a slot handover",
			ActorYou+", then "+ActorOperator, []string{"B-401"}, det...)
	}
	if d.verdict("relay-tls") == VerdictFail {
		return unknown(id, "a credential cannot be presented over a connection that will not verify",
			"relay-tls")
	}
	res, ok := d.needLive(id)
	if !ok {
		return res
	}
	v := d.live.View
	det := []string{"the secret is read from a file and never printed; only whether one is configured"}
	if v.Relay.Connected {
		det = append(det, fmt.Sprintf("connected since %s, relay session %s",
			time.UnixMilli(v.Relay.ConnectedSince).Format("15:04:05"),
			termsafe.Clip(v.Relay.RelaySessionID, 40)))
		return pass(id, "this world's credential opened a session and the handshake completed", det...)
	}
	f := v.Relay
	switch {
	case f.LastFaultKind == FaultUnauthorized:
		return fail(id, "the relay refused this peer's credential at the door, with HTTP 401",
			"re-apply this world's join string. A refusal at the door is about the secret this "+
				"machine presented and about nothing on the map, and it reaches no slot's "+
				"lastRefusal — nobody else can see it. If the secret is lost, ask the operator "+
				"for a slot handover",
			ActorYou+", then "+ActorOperator, []string{"B-401"},
			append(det, fmt.Sprintf("%d in a row; after five the backoff pins and the log stops "+
				"saying anything new", f.LastFaultRepeats))...)
	case f.LastFaultKind == FaultClosed && strings.Contains(f.LastFaultReason, "does not match"):
		return fail(id, "the connection was accepted and then refused for an identity mismatch",
			"present the identity your join string names. The credential names one peer and the "+
				"handshake claimed another, which means your stored identity and your credential "+
				"have drifted apart — re-apply the join string. The peer whose id was claimed is "+
				"not touched and is not told",
			ActorYou, []string{"B-4003b"},
			append(det, termsafe.Text(f.LastFaultReason))...)
	case f.LastFaultKind == FaultTLS:
		return unknown(id, "the credential was never presented: the connection did not verify",
			"relay-tls")
	case f.LastFaultKind != "":
		return unknown(id, "the link is down for a reason that is not about this credential", "",
			append(det, termsafe.Text(f.LastFaultKind+": "+f.LastFaultReason))...)
	default:
		return unknown(id, "the link is not up yet and nothing has been refused", "",
			det...)
	}
}

// checkContractVersion. The map publishes its floor precisely so a peer can read
// what it will fail BEFORE it fails it.
func (d *diag) checkContractVersion() CheckResult {
	const id = "contract-version"
	mine := wire.ProtocolB
	res, ok := d.needLive(id)
	if !ok {
		return res
	}
	floor := d.live.View.Wire.MinContractVersion
	f := d.live.View.Relay
	if f.LastFaultKind == FaultClosed && strings.Contains(f.LastFaultReason, "below this relay's minimum") {
		return fail(id, "this build's wire version is below the floor this map admits",
			"upgrade from the published release. It is a compatibility control and never a "+
				"security one, and nobody on the relay's side can do it for you — the release "+
				"channel pushes nothing",
			ActorYou, []string{"B-4003d"},
			"this build speaks "+mine, termsafe.Text(f.LastFaultReason))
	}
	if f.LastFaultKind == FaultClosed && strings.Contains(f.LastFaultReason, "close 4000") {
		return fail(id, "this build and the map are on different major wire versions",
			"install the release that speaks the map's current major. A retired path answers "+
				"the same way, on purpose, so an old build gets a defined error instead of a "+
				"bare 404",
			ActorYou, []string{"B-4000"},
			"this build speaks "+mine, termsafe.Text(f.LastFaultReason))
	}
	if floor == "" {
		if !d.live.View.Relay.Connected {
			return unknown(id, "this map has not published a version floor to this peer yet",
				"credential", "this build speaks "+mine)
		}
		return pass(id, "this map publishes no minimum wire version",
			"this build speaks "+mine,
			"absent means no minimum, which is the default and the only honest one")
	}
	cmp, err := wire.CompareProtocol(mine, floor)
	if err != nil {
		return fail(id, "this build and the map are not on the same wire family",
			"install the release that speaks the map's wire",
			ActorYou, []string{"B-4000"},
			"this build speaks "+mine+"; the map's floor is "+termsafe.Text(floor))
	}
	if cmp < 0 {
		return fail(id, "this build is below the wire version floor this map publishes",
			"upgrade from the published release, before the map starts refusing you. The floor "+
				"is published precisely so a peer can read what it will fail before it fails it",
			ActorYou, []string{"B-4003d"},
			"this build speaks "+mine+"; this map's floor is "+termsafe.Text(floor))
	}
	return pass(id, "this build's wire version is at or above the floor this map publishes",
		"this build speaks "+mine+"; this map's floor is "+termsafe.Text(floor))
}

// checkGameVersion is two questions with two different answers: is this build
// supported at all, and is it the build the map is on.
func (d *diag) checkGameVersion() CheckResult {
	const id = "game-version"
	matrix, matrixPath, err := d.supportMatrix()
	if err != nil {
		return unknown(id, "this machine's support matrix could not be read", "", err.Error(),
			"a packaged install keeps its copy in the folder it was installed from; pass "+
				"--support-matrix to name another")
	}
	gameDir := d.gameDir()
	if gameDir == "" {
		return unknown(id, "this check needs the game folder and nothing names one here", "",
			"pass --game-dir, or run this on a packaged install, whose install-record.json "+
				"names the folder")
	}
	assembly := filepath.Join(gameDir, "The Bibites_Data", "Managed", "BibitesAssembly.dll")
	sum, err := fileSHA256(assembly)
	if err != nil {
		return unknown(id, "this machine's game assembly could not be hashed", "",
			assembly, err.Error())
	}
	det := []string{
		"matrix " + matrixPath + " for release " + termsafe.Text(matrix.Release),
		"this machine's BibitesAssembly.dll is SHA-256 " + sum,
	}
	var entry *matrixEntry
	for i := range matrix.Entries {
		if strings.EqualFold(matrix.Entries[i].AssemblySHA256, sum) {
			entry = &matrix.Entries[i]
			break
		}
	}
	if entry == nil {
		remedy := matrix.Refusal
		if remedy == "" {
			remedy = "wait for a release whose matrix lists your build, or put this machine on " +
				"a build this matrix lists"
		}
		for _, e := range matrix.Entries {
			det = append(det, "the matrix lists "+termsafe.Text(e.GameVersion)+" — SHA-256 "+
				termsafe.Text(e.AssemblySHA256))
		}
		return fail(id, "this machine's game build is not in the support matrix",
			termsafe.Text(remedy), ActorYou, []string{"INS-GAMEBUILD"}, det...)
	}
	det = append(det, "the matrix names this build "+termsafe.Text(entry.GameVersion)+
		", with mod "+termsafe.Text(entry.Mod)+" and sidecar "+termsafe.Text(entry.Sidecar))

	// The second question. It needs the map, so an absent map leaves it unknown
	// rather than answered by the first half.
	if !d.live.OK {
		return unknown(id, "this build is supported, and whether it is the build the map is on needs the map",
			"stale-process", det...)
	}
	mine := d.live.View.Mod.GameVersion
	var differing []string
	for _, p := range d.live.View.Peers {
		if p.Me || !p.Live || p.GameVersion == "" || mine == "" || p.GameVersion == mine {
			continue
		}
		differing = append(differing, fmt.Sprintf("slot %d reports %s", p.Slot,
			termsafe.Clip(p.GameVersion, 24)))
	}
	if len(differing) > 0 {
		return warn(id, "the map is partitioned along a game-version boundary",
			"none at this machine. This is the accepted behaviour after a staggered game update "+
				"and it ends when every machine is on the same build. Only the operator can see "+
				"which build the map is converging on",
			ActorOperator, []string{"B-4003c", "AIN-VERSION_UNSUPPORTED"},
			append(det, append([]string{"this world runs " + termsafe.Clip(orUnknown(mine), 24)},
				differing...)...)...)
	}
	return pass(id, "this game build is supported, and every live world on the map is on it", det...)
}

// checkLimits reads the limits object the map published and NEVER a constant of
// its own. Every one of the eight is a knob its operator may have turned.
func (d *diag) checkLimits() CheckResult {
	const id = "limits"
	res, ok := d.needLive(id)
	if !ok {
		return res
	}
	v := d.live.View
	if v.Relay.CapacitySheds > 0 && v.Relay.LastFaultKind == FaultCapacity {
		return fail(id, "the relay has shed this connection for a published capacity limit",
			"read the limits the map publishes and bring this world under them. If this peer is "+
				"legitimately inside them it is a defect worth reporting; if the map's limit is "+
				"genuinely too low for honest traffic the operator raises it, because every "+
				"limit is a knob",
			ActorYou+" (report), or "+ActorOperator, []string{"B-4007", "B-429", "B-1009"},
			"close 4007: "+termsafe.Text(v.Relay.LastFaultReason),
			fmt.Sprintf("%d shed(s) in this process's life", v.Relay.CapacitySheds))
	}
	if len(v.Wire.Limits) == 0 {
		return unknown(id, "this map has published no limits object",
			"credential",
			"absence reads as UNKNOWN and never as `no ceilings`: a relay that predates the "+
				"published table still completes a handshake, and no ceiling is invented for it")
	}
	// Every reading this peer can take of itself, against the ceiling it is
	// counted on. The three the peer cannot measure say so.
	readings := []struct {
		key      string
		measured int64
		what     string
	}{
		{contractb.LimitMaxFramesPerSecond, v.Wire.PeakFramesPerSecond, "peak frames in a second"},
		{contractb.LimitMaxBytesPerSecond, v.Wire.PeakBytesPerSecond, "peak bytes in a second"},
		{contractb.LimitMaxFrameBytes, v.Wire.LargestFrameBytes, "largest frame sent"},
		{contractb.LimitMaxClaimsPerMinute, int64(v.Wire.ClaimsLastMinute), "claims in the last minute"},
	}
	det := []string{fmt.Sprintf("a reading counts as approaching a ceiling above %.0f%% of it, "+
		"which is the most this sidecar's own paced sender can emit in one second",
		LimitWarnFraction*100)}
	var approaching []string
	for _, r := range readings {
		published, known := publishedLimit(v.Wire.Limits, r.key)
		if !known {
			det = append(det, r.key+": the map published no value")
			continue
		}
		frac, _, over := approachingLimit(r.measured, published)
		det = append(det, fmt.Sprintf("%s %d — %s %d (%.0f%%)",
			r.key, published, r.what, r.measured, frac*100))
		if over {
			approaching = append(approaching, fmt.Sprintf("%s: %d against a published %d",
				r.key, r.measured, published))
		}
	}
	if conns, known := publishedLimit(v.Wire.Limits, contractb.LimitMaxConnectionsPerPeer); known {
		det = append(det, fmt.Sprintf("%s %d — this sidecar holds one connection",
			contractb.LimitMaxConnectionsPerPeer, conns))
	}
	det = append(det,
		contractb.LimitMaxConnectionsPerAddress+": counted by the relay across every peer on "+
			"this address, so this peer cannot measure it; crossing it is HTTP 429 on the "+
			"upgrade and never a close",
		contractb.LimitMaxSubscribers+": a participant never meets it",
		fmt.Sprintf("this connection is pacing itself at %.0f frames a second and has deferred "+
			"%d bulk frames to stay there", v.Wire.PacedFramesPerSecond, v.Wire.PacedDeferrals))
	if len(approaching) > 0 {
		return warn(id, "this world's own traffic is approaching a limit the map publishes",
			"bring this world's traffic under the ceiling, or ask the operator to raise that "+
				"knob and restart. Nothing has been shed yet: this is the reading before the "+
				"refusal, which is the whole reason the map publishes what it is running with",
			ActorYou+", then "+ActorOperator, []string{"B-4007", "BCLAIM-rate_limited"},
			append(approaching, det...)...)
	}
	return pass(id, "this peer is inside every limit it can measure itself against", det...)
}

// checkSlot. A grant may land in the same second as the handshake or an hour
// later, so no grant yet is UNKNOWN and never a failure.
func (d *diag) checkSlot() CheckResult {
	const id = "slot"
	res, ok := d.needLive(id)
	if !ok {
		return res
	}
	v := d.live.View
	if v.Slot.GrantSeen && !v.Slot.Granted {
		return fail(id, "this world's placement claim was refused",
			"the relay named the reason: "+termsafe.Text(v.Slot.GrantReason)+". A rate-limited "+
				"claim clears itself once the storm stops and is usually a wandering time scale; "+
				"a version-incompatible one is the map's game-version gate and only the operator "+
				"can say when it converges",
			ActorYou, []string{"BCLAIM-" + termsafe.Clip(v.Slot.GrantReason, 40)},
			fmt.Sprintf("refused at %s", time.UnixMilli(v.Slot.GrantAt).Format("15:04:05")))
	}
	if v.Slot.Slot == 0 {
		return unknown(id, "no grant has arrived yet, so this world holds no slot", "credential",
			"a grant may land in the same second as the handshake or an hour later",
			fmt.Sprintf("this data directory remembers slot %d", v.Slot.RememberedSlot))
	}
	det := []string{fmt.Sprintf("slot %d at column %d, row %d of a %dx%d map, reason %s",
		v.Slot.Slot, v.Slot.Position.Col, v.Slot.Position.Row,
		v.Slot.MapWidth, v.Slot.MapHeight, termsafe.Clip(orUnknown(v.Slot.GrantReason), 40))}
	if v.Slot.LastRefusal != "" {
		det = append(det, "this slot carries a lastRefusal, visible to every other peer: "+
			termsafe.Clip(v.Slot.LastRefusal, 200))
	}
	if v.Slot.RememberedSlot != 0 && v.Slot.RememberedSlot != v.Slot.Slot {
		return warn(id, "this world holds a different slot from the one this directory remembers",
			"check that --data-dir names the directory this world has always used. A peer that "+
				"arrives with a different identity takes a second slot and strands its old one, "+
				"with every organism addressed to it",
			ActorYou+", then "+ActorOperator, []string{"BCLAIM-role_has_no_slot"},
			append(det, fmt.Sprintf("this directory remembers slot %d", v.Slot.RememberedSlot))...)
	}
	return pass(id, "this world holds its own slot, at the position it expects", det...)
}

// checkEdges names each closed edge's reason AND its actor, because the reasons
// have four different actors between them and only one of them is the
// participant.
func (d *diag) checkEdges() CheckResult {
	const id = "edges"
	res, ok := d.needLive(id)
	if !ok {
		return res
	}
	v := d.live.View
	if d.verdict("slot") == VerdictUnknown || v.Slot.Slot == 0 {
		return unknown(id, "a peer with no granted slot reports every edge closed on every axis",
			"slot")
	}
	if len(v.Edges) == 0 {
		return unknown(id, "there are no declared edges to report on", "mod-connected")
	}
	var closed []string
	worst := laneActor("")
	var taxonomy []string
	for _, e := range v.Edges {
		if e.Open {
			continue
		}
		closed = append(closed, fmt.Sprintf("%s closed: %s — %s", e.Edge,
			termsafe.Text(e.Reason), laneMeaning(e.Reason)))
		taxonomy = appendUnique(taxonomy, "LANE-"+e.Reason)
		worst = worseActor(worst, laneActor(e.Reason))
	}
	if len(closed) == 0 {
		det := make([]string, 0, len(v.Edges))
		for _, e := range v.Edges {
			det = append(det, fmt.Sprintf("%s open to slot %d", e.Edge, e.PeerSlot))
		}
		return pass(id, "every declared edge reports a live peer", det...)
	}
	return warn(id, fmt.Sprintf("%d of %d declared edges are closed", len(closed), len(v.Edges)),
		"read each reason above: they have four different actors between them and only one of "+
			"them is you. A closed edge refuses exports through it and never arrivals on it",
		worst, taxonomy, closed...)
}

// checkNeighbours is DQ8's whole argument: a world's lanes run badly, its queues
// pin, and the cause is in somebody else's install. The wire already gives the
// sufferer the evidence; what it cannot give is the remedy, so the output is
// shaped as EVIDENCE TO HAND TO THE OPERATOR and not as an action.
func (d *diag) checkNeighbours() CheckResult {
	const id = "neighbours"
	res, ok := d.needLive(id)
	if !ok {
		return res
	}
	v := d.live.View
	if v.Slot.Slot == 0 {
		return unknown(id, "the candidates on an axis are the peers this world would export to",
			"slot")
	}
	if len(v.Peers) < 2 {
		return pass(id, "there is no other world on this map yet",
			"a map of one refuses nobody and has nothing to be incompatible with")
	}
	status := d.statusFromPeers()
	me, found := mapwalk.Find(status, v.Slot.Slot)
	if !found {
		return unknown(id, "this world is not in the map the relay last broadcast", "slot")
	}
	var problems []string
	var taxonomy []string
	actor := ActorNobody
	seen := map[int]bool{}
	for _, e := range v.Edges {
		_, skipped, _ := mapwalk.Walk(status, me, e.Edge)
		for _, sk := range skipped {
			if sk.Slot == nil {
				// A hole: a position inside the rectangle that no slot names. It
				// is not a world and nobody owns it.
				continue
			}
			if seen[*sk.Slot] {
				continue
			}
			seen[*sk.Slot] = true
			p := peerBySlot(v.Peers, *sk.Slot)
			switch {
			case p == nil:
			case !p.Live:
				problems = append(problems, fmt.Sprintf("slot %d is dark%s", p.Slot,
					darkSince(p.DarkSinceMs)))
				taxonomy = appendUnique(taxonomy, "BMIG-PEER_OFFLINE")
			case !p.ModConnected:
				problems = append(problems, fmt.Sprintf("slot %d is connected with NO GAME "+
					"behind it; its owner has to restart that game and only the operator can "+
					"tell them", p.Slot))
				taxonomy = appendUnique(taxonomy, "LANE-peer_mod_absent", "BMIG-MOD_ABSENT")
				actor = ActorOperator
			case v.Mod.GameVersion != "" && p.GameVersion != "" && p.GameVersion != v.Mod.GameVersion:
				problems = append(problems, fmt.Sprintf("slot %d reports game %s; this world "+
					"runs %s. Lanes to it are closed by design until both are on one build",
					p.Slot, termsafe.Clip(p.GameVersion, 24), termsafe.Clip(v.Mod.GameVersion, 24)))
				taxonomy = appendUnique(taxonomy, "LANE-peer_incompatible", "AOUT-PEER_INCOMPATIBLE")
				actor = ActorOperator
			default:
				problems = append(problems, fmt.Sprintf("slot %d was skipped on the %s axis "+
					"and reports nothing this check can name", p.Slot, e.Edge))
			}
		}
	}
	if len(problems) == 0 {
		return pass(id, "every candidate this world would export to is live, has a game, and is on this build",
			fmt.Sprintf("%d worlds on the map", len(v.Peers)))
	}
	if actor == ActorNobody {
		taxonomy = appendUnique(taxonomy, "AOUT-EDGE_CLOSED")
	}
	return warn(id, "a world this one exports to is the reason a lane is closed",
		"THE REMEDY IS NOT AT THIS MACHINE. There is no directory, no contact field and nothing "+
			"on this wire that carries a message between two people, so this is evidence to hand "+
			"to the operator: they are the only party who can see both ends and tell the other "+
			"owner. Send the lines above with your slot number",
		actor, taxonomy, problems...)
}

// statusFromPeers rebuilds enough of the last PEER_STATUS for §8's walk. The
// walk needs liveness, position, game version, simulation size and whether a mod
// is connected, and the own-slot view carries every one of them except the
// simulation size, which the walk compares against this world's own.
func (d *diag) statusFromPeers() contractb.PeerStatus {
	v := d.live.View
	status := contractb.PeerStatus{
		Map:       contractb.MapShape{Width: v.Slot.MapWidth, Height: v.Slot.MapHeight},
		SlotCount: v.Slot.SlotCount,
	}
	for _, p := range v.Peers {
		status.Slots = append(status.Slots, contractb.SlotInfo{
			Slot:         p.Slot,
			Position:     contractb.Position{Col: p.Col, Row: p.Row},
			Live:         p.Live,
			ModConnected: p.ModConnected,
			GameVersion:  p.GameVersion,
			// Every world on one map runs one simulation size or its lanes would
			// be undefined; this walk is about liveness, games and versions, and
			// a size taken from this world keeps a size mismatch from masking one
			// of those as the reason.
			SimulationSize: v.Mod.SimulationSize,
			ExportEdges:    v.Mod.ExportEdges,
		})
	}
	return status
}

func peerBySlot(peers []PeerState, slot int) *PeerState {
	for i := range peers {
		if peers[i].Slot == slot {
			return &peers[i]
		}
	}
	return nil
}

// ---------------------------------------------------------------- helpers

// laneMeaning and laneActor are contract-b-m4.md §8's reasons with the taxonomy's
// actor beside each. The actor is the point: four different parties own these
// between them and only one of them is the participant.
func laneMeaning(reason string) string {
	switch reason {
	case contracta.ReasonNoPeer:
		return "no position on that axis was deliverable. On a one-row map the north and south " +
			"edges stay closed for the life of the map, and that is a map shape and not a fault"
	case contracta.ReasonPeerModAbsent:
		return "every candidate on that axis is connected with no game behind it"
	case contracta.ReasonPeerIncompatible:
		return "every candidate is on a different game build; the map is partitioned along a " +
			"version boundary"
	case contracta.ReasonSimSizeMismatch:
		return "every candidate disagrees with this world about simulation size"
	case contracta.ReasonPeerUnreachable:
		return "THIS sidecar's own relay connection is down, decided before the walk, so it is " +
			"about this machine and not the map"
	case contracta.ReasonPeerOverloaded:
		return "every candidate is shedding"
	case contracta.ReasonAdminClosed:
		return "this edge was closed locally"
	}
	return "the relay named a reason this build does not have a sentence for"
}

func laneActor(reason string) string {
	switch reason {
	case contracta.ReasonPeerUnreachable, contracta.ReasonAdminClosed:
		return ActorYou
	case contracta.ReasonPeerModAbsent, contracta.ReasonPeerIncompatible:
		return ActorOperator
	case contracta.ReasonSimSizeMismatch:
		return ActorOperator + ", then " + ActorYou
	}
	return ActorNobody
}

// worseActor keeps the actor a reader most needs to see when several edges are
// closed for different reasons: something the participant can fix outranks
// something only the operator can, which outranks waiting.
func worseActor(a, b string) string {
	rank := func(s string) int {
		switch {
		case strings.HasPrefix(s, ActorYou):
			return 3
		case strings.HasPrefix(s, ActorOperator):
			return 2
		case s == ActorNobody:
			return 1
		}
		return 0
	}
	if rank(b) > rank(a) {
		return b
	}
	if a == "" {
		return ActorNobody
	}
	return a
}

func appendUnique(list []string, add ...string) []string {
	for _, a := range add {
		found := false
		for _, x := range list {
			if x == a {
				found = true
				break
			}
		}
		if !found {
			list = append(list, a)
		}
	}
	return list
}

// installRecord is the packaged installer's own record, written into the data
// root. It is what lets this command find the game folder and the support matrix
// on a machine nobody here configured.
type installRecord struct {
	Record  string `json:"record"`
	Release string `json:"release"`
	KitDir  string `json:"kitDir"`
	GameDir string `json:"gameDir"`
	DataDir string `json:"dataDir"`
}

func readInstallRecord(dataDir string) (installRecord, bool) {
	// The installer writes it into the DATA ROOT, and the sidecar's data
	// directory is one level below that, so both are worth looking in.
	for _, dir := range []string{dataDir, filepath.Dir(dataDir)} {
		b, err := os.ReadFile(filepath.Join(dir, "install-record.json"))
		if err != nil {
			continue
		}
		var rec installRecord
		if json.Unmarshal(b, &rec) == nil && rec.GameDir != "" {
			return rec, true
		}
	}
	return installRecord{}, false
}

func (d *diag) gameDir() string {
	if d.opts.GameDir != "" {
		return d.opts.GameDir
	}
	if d.haveRec {
		return d.record.GameDir
	}
	return ""
}

// matrixFile and matrixEntry are docs/support-matrix.md's machine-readable
// block, which travels in the release archive as support-matrix.json — the same
// bytes the installer reads.
type matrixFile struct {
	Matrix  string        `json:"matrix"`
	Release string        `json:"release"`
	Refusal string        `json:"refusal"`
	Entries []matrixEntry `json:"entries"`
}

type matrixEntry struct {
	GameVersion    string `json:"gameVersion"`
	AssemblySHA256 string `json:"assemblySha256"`
	Mod            string `json:"mod"`
	Sidecar        string `json:"sidecar"`
}

func (d *diag) supportMatrix() (matrixFile, string, error) {
	var candidates []string
	if d.opts.MatrixFile != "" {
		candidates = append(candidates, d.opts.MatrixFile)
	}
	if d.haveRec && d.record.KitDir != "" {
		candidates = append(candidates, filepath.Join(d.record.KitDir, "support-matrix.json"))
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "support-matrix.json"))
	}
	var last error
	for _, path := range candidates {
		b, err := os.ReadFile(path)
		if err != nil {
			last = err
			continue
		}
		var m matrixFile
		if err := json.Unmarshal(b, &m); err != nil {
			return matrixFile{}, path, err
		}
		return m, path, nil
	}
	if last == nil {
		last = errors.New("no support-matrix.json was found beside this install")
	}
	return matrixFile{}, "", last
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return strings.ToUpper(hex.EncodeToString(h.Sum(nil))), nil
}

func certNames(cert *x509.Certificate) string {
	names := append([]string(nil), cert.DNSNames...)
	for _, ip := range cert.IPAddresses {
		names = append(names, ip.String())
	}
	if len(names) == 0 {
		names = append(names, cert.Subject.CommonName)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// samePath compares two configured paths as a person means them: cleaned,
// absolute where they can be made so, and case-insensitively on Windows.
func samePath(a, b string) bool {
	norm := func(p string) string {
		if abs, err := filepath.Abs(p); err == nil {
			p = abs
		}
		p = filepath.Clean(p)
		if runtime.GOOS == "windows" {
			p = strings.ToLower(p)
		}
		return p
	}
	return norm(a) == norm(b)
}

func orUnknown(v string) string {
	if v == "" {
		return "unknown"
	}
	return v
}

func keepText(keep *int) string {
	if keep == nil {
		return "an unreported number of saves"
	}
	return fmt.Sprintf("%d saves", *keep)
}

func darkSince(ms int64) string {
	if ms <= 0 {
		return ""
	}
	return " since " + time.UnixMilli(ms).Format("15:04:05")
}

func roundMs(ms int64) time.Duration {
	return (time.Duration(ms) * time.Millisecond).Round(time.Second)
}

func bytesText(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GiB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KiB", float64(n)/(1<<10))
	}
	return strconv.FormatInt(n, 10) + " B"
}

// defaultLogCeilingBytes is what the shipped log rotation may hold, and it is
// contract-b-m4.md §12's own arithmetic rather than a number this file chose:
// logRotateMb x (logKeep+1) per process, at the shipped defaults of 100 MiB and
// 5 generations. The `.out` file a packaged install writes beside it is a raw
// stdout redirect and is not rotated, so it is named in the check's output and
// deliberately not folded into a ceiling it does not have.
const defaultLogCeilingBytes = 100 * (1 << 20) * (5 + 1)

// ---------------------------------------------------------------- rendering

// RenderDiagnosis writes the human form, which is the default and the primary
// one. One line per check: verdict, id, one sentence — and on anything but a
// PASS, the evidence, the remedy, the taxonomy id and the actor.
func RenderDiagnosis(w io.Writer, rep Report) {
	fmt.Fprintf(w, "multiverse-sidecar --diagnose — sidecar %s, %s\n",
		rep.SidecarVersion, time.UnixMilli(rep.GeneratedAtMs).Format("2006-01-02 15:04:05"))
	fmt.Fprintf(w, "  data directory  %s\n", rep.DataDir)
	if rep.PeerID != "" {
		fmt.Fprintf(w, "  this world      %s\n", termsafe.Text(rep.PeerID))
	}
	if rep.RelayURL != "" {
		fmt.Fprintf(w, "  the map         %s\n", termsafe.Text(rep.RelayURL))
	}
	fmt.Fprintf(w, "  live state      %s\n\n", termsafe.Text(rep.LiveSource))

	for _, c := range rep.Checks {
		fmt.Fprintf(w, "%-8s %-17s %s\n", c.Verdict, c.ID, termsafe.Text(c.Says))
		for _, line := range c.Detail {
			fmt.Fprintf(w, "    %s\n", termsafe.Text(line))
		}
		if c.WaitingOn != "" {
			fmt.Fprintf(w, "    waiting on: %s\n", c.WaitingOn)
		}
		if c.Remedy != "" {
			fmt.Fprintf(w, "    remedy: %s\n", termsafe.Text(c.Remedy))
		}
		if len(c.Taxonomy) > 0 || c.Actor != "" {
			// The taxonomy id and the actor, on one line, because rule 5 is that
			// a failure without an actor is half a failure. "who acts" rather
			// than the specification's illustrative "<actor> acts": one of the
			// three values is "you", and "you acts" is not a sentence.
			fmt.Fprintf(w, "    taxonomy: %s — who acts: %s\n",
				strings.Join(c.Taxonomy, ", "), orUnknown(c.Actor))
		}
	}

	fmt.Fprintf(w, "\n%d checks: %d pass, %d fail, %d warn, %d unknown, %d skip\n",
		len(rep.Checks), rep.Summary.Pass, rep.Summary.Fail, rep.Summary.Warn,
		rep.Summary.Unknown, rep.Summary.Skip)
	fmt.Fprintf(w, "exit %d: %s.\n", rep.Exit, rep.ExitMeaning)
	if rep.Summary.Unknown > 0 {
		fmt.Fprintf(w, "An unknown is an honest gap and never a pass or a failure. Each one names "+
			"what it was waiting on.\n")
	}
	fmt.Fprintf(w, "Nothing above is a secret: no credential and no token is printed here at any "+
		"verbosity, only the paths they are read from. This whole report is safe to send.\n")
}

// WriteDiagnosisJSON emits the machine-readable form for a person who is pasting
// it into a support conversation.
func WriteDiagnosisJSON(w io.Writer, rep Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(rep)
}
