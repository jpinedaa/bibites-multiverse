# `multiverse-sidecar --diagnose` — specification

**What this is.** The specification WP7's implementation arc builds from. It is not code and it
describes no implementation: it names each check, states the criterion that makes it pass, and
binds each failure to an entry in [`error-taxonomy.md`](error-taxonomy.md) so that every
failure the tool reports arrives with a remedy and an actor attached.

**Why it exists.** For the whole of this project's development the diagnostic surface was the
status page, a terminal tool, five kinds of log, and the owner — and the owner was the
diagnostics command (`m5_considerations.md`, DQ8). Every check below is one this project's
operator has done by hand, on the rig, and `dev_environment.md` is where each one is recorded.
The last column names that source, because a check that cannot say which by-hand procedure it
replaces is a check somebody invented.

**Status: implemented, with WP2's, WP4's and WP6's texture landed and WP7's own slots closed.**
Checks are stated against the wire as WP1 published it; the refusal wordings and published values
the credential, TLS and capacity work supplies are quoted from what ships; and the paths, log
lines and matrix lookup the package invents are quoted from the release. **The twenty-one checks,
the exit codes, the output shape and the approaching-a-limit band are built** — `go/internal/
sidecar/diagnose.go`, with the participant's own-slot view in `ownslot.go` and their tests beside
them. What remains is owned elsewhere: the three measured thresholds of §6. §7 collects them.

---

## 1. The contract

| Rule | Statement |
|---|---|
| **It changes nothing** | `--diagnose` is read-only against the participant's machine and against the map. It **MUST NOT** rotate or mint a credential, restart a process, set a time scale, write to the journal, claim a slot, or send any frame that is not a connection this specification names. A diagnostic that repairs is a diagnostic nobody can run twice on the same evidence. **One exception, named and bounded**: the `data-dir` check creates and immediately removes one empty temporary file in the data directory, because writability is not answerable by inspection. It is never in the journal directory and never a file the sidecar owns. See *The one write it makes*, below. |
| **It never prints a secret** | Neither the map credential nor the local bearer token, in whole, in prefix, hashed or elided-with-length, at any verbosity. It **MAY** print the *path* a secret was read from and whether the file exists and is readable, because that is the fact a participant needs and it is not the secret. |
| **It runs without the map** | Every check that needs a relay connection **MUST** degrade to `UNKNOWN` when there is none, naming the check whose failure blocked it. **A missing precondition is never a `PASS` and never a `FAIL`.** |
| **It runs beside a running sidecar** | The participant's ordinary state is a sidecar that is up. `--diagnose` **MUST NOT** require the sidecar to be stopped, and **MUST NOT** disturb its session — in particular it must not open a second connection that the map's per-peer connection limit would shed, or that would trigger the newer-connection-replaces-older rule. Where a check needs live map state, it reads what the running sidecar already holds rather than dialling again. |
| **Every failure names an actor** | A `FAIL` or `WARN` line **MUST** carry the taxonomy id and the actor — **you**, **operator** or **nobody**. That is the whole rule of the taxonomy, and a diagnostic that reported causes without actors would undo it. |
| **Unknown is a value** | A check that could not be answered reports `UNKNOWN` and says why. It is never rendered as a pass, and never as a failure. This is the same rule the status page runs on: an honest gap beats a confident zero. |

### Verdicts

| Verdict | Meaning |
|---|---|
| `PASS` | The criterion held. |
| `FAIL` | The criterion did not hold, and it is a fault. Carries a taxonomy id and an actor. |
| `WARN` | The criterion did not hold and it is legal, or the reading is outside a healthy band without being a fault. Carries a taxonomy id and an actor. |
| `UNKNOWN` | Not answerable — a precondition failed, or the map has not said yet. Names what it was waiting on. |
| `SKIP` | Not applicable to this configuration. |

### Invocation and exit

**Fixed by the implementation arc, and this is the contract:**

```
multiverse-sidecar --diagnose [--json] [--check <id>[,<id>…]] [--timeout <duration>]
                   [--data-dir <path>] [--relay <url>] [--credential-file <path>]
                   [--contract-a-token-file <path>] [--game-dir <path>]
                   [--support-matrix <path>]
```

`--json`, `--check`, `--timeout`, `--game-dir` and `--support-matrix` are the flags this command
adds. The rest are the sidecar's ordinary configuration flags, and they are named here because
**the check reports on the configuration it is given**: `--diagnose` run with the flags and
environment the start script uses is a diagnosis of that install, and run with different ones it
is a diagnosis of a machine nobody has. `--game-dir` and `--support-matrix` are needed only where
there is no packaged install — `install-record.json` in the data root names both, and the check
reads it.

| Exit code | Meaning |
|---|---|
| `0` | No `FAIL` among the checks it reported. `WARN` and `UNKNOWN` may be present, and on a healthy machine several always are. |
| `1` | At least one reported check `FAIL`ed. |
| `2` | **The diagnostic itself could not run** — no `--data-dir`, or a `--check` naming something that is not a check. It is never used for anything a check found, because a run that exits `2` has told the caller nothing. |

**The human form is the default and the primary one.** A header naming the data directory, this
world, the map and **where the live half came from**; then one line per check — verdict in eight
columns, id in seventeen, one sentence — with the evidence, the remedy, the taxonomy id and the
actor indented under anything that is not a `PASS`; then the counts and one line saying what the
exit code means. The actor line reads `taxonomy: A-401 — who acts: you`, and not §4's
illustrative `— you acts`, for the obvious reason.

**`--check` filters the report and not the work.** A check's precondition is still evaluated —
its answer would mean nothing otherwise — and the exit code then reflects what was reported.

**`--timeout` bounds each probe, not the run**: the local read, the relay connect and the TLS
handshake each get it, and the default is 5 s.

**`--json` emits the same records**, as `multiverse-diagnose/1`:

```json
{
  "schema": "multiverse-diagnose/1",
  "sidecarVersion": "m5.0",
  "generatedAtMs": 1786506912345,
  "dataDir": "...", "peerId": "...", "relayUrl": "...",
  "liveSource": "the running sidecar at 127.0.0.1:8788 (pid 1483238)",
  "liveRead": true,
  "checks": [
    { "id": "contract-a-token", "verdict": "FAIL",
      "says": "the mod and this sidecar are not reading the same token file",
      "detail": ["this configuration reads …", "the running sidecar reads …"],
      "taxonomy": ["A-401"], "actor": "you", "remedy": "point both at one path…" }
  ],
  "summary": { "pass": 12, "fail": 1, "warn": 2, "unknown": 5, "skip": 1 },
  "exit": 1, "exitMeaning": "at least one check failed"
}
```

`taxonomy`, `actor` and `remedy` are present exactly on `FAIL` and `WARN`; `waitingOn` is present
exactly on `UNKNOWN` and names the check that blocked it. **The shape is stable across releases**,
so a report from an old build is still readable — which is the whole reason it exists, because the
thing being pasted into a support conversation is usually not the current build.

### The one write it makes

`--diagnose` creates **one empty temporary file in the data directory and removes it**, and that
is the whole of it. Writability cannot be answered by inspection — a read-only mount and a full
disk both look exactly like a healthy directory from a stat — and `data-dir` exists to catch
precisely those. It never touches the journal directory and never a file the sidecar owns; the
journal is opened through a **read-only replay that neither compacts it nor takes a write
handle**, which is what lets it be read beside a sidecar that is writing it.

### Where the live half comes from

Rule 4 forbids a second Contract B session, so every check that needs live map state reads the
running sidecar's own state over **`GET /my-slot`**, a read-only endpoint on the Contract A
loopback listener — the same surface `multiverse-sidecar --my-slot` renders (§8). The two probes
that do touch the network, `relay-reachable` and `relay-tls`, are a TCP connect and a TLS
handshake with **no HTTP request at all**, so the relay's per-address accounting — which is keyed
on a request — never sees them, and no WebSocket exists to be shed.

**With no sidecar running the cold checks still answer** — the data directory, the token file, the
journal, the disk, the relay's reachability and its certificate — and everything else reports
`UNKNOWN` naming `stale-process`.

### Ordering

Checks run in the order below, and the order is a dependency order: `data-dir` before anything
that reads a file, `relay-tls` before `credential`, `slot` before `edges`. **A check whose
precondition failed reports `UNKNOWN` and names the check that failed** rather than producing a
second, derived failure — one root cause should produce one `FAIL` and a trail of `UNKNOWN`,
not fifteen failures.

---

## 2. Checks — the participant's own machine

| Id | Question | `PASS` when | Otherwise | Taxonomy | Derived from |
|---|---|---|---|---|---|
| `data-dir` | Is the sidecar's data directory present, writable, and holding this world's identity and slot? | The directory resolves, is writable, and both files are present and readable | `FAIL`. A dangling or unmounted path is the loudest possible failure and the least obvious: **every path fails at once and none of them says why** | `LOCAL-DISK` | *Where the rig lives now*; the reboot ritual's step 1, which gates every other step |
| `stale-process` | Is exactly one sidecar running against this data directory? | One live process | `FAIL` on two. `WARN` on a recorded process id that is dead — a stale record makes a status query claim the thing is running when it is not, and pid numbers are reused after a reboot. **Zero is `UNKNOWN`**, not a failure: running this with the sidecar stopped is a supported case, and it is what every later `UNKNOWN` names. **The record is `<data-dir>/sidecar-process.json`**, written after the listener binds and removed on a clean shutdown, and it carries the pid, the start time, the peer id and the address — because a pid alone is not evidence, so the check corroborates it against a process that actually answers | `LOCAL-TWOSIDECARS`, `LOCAL-STALEPID` | Reboot ritual step 2; *Only one rig can run at a time* |
| `contract-a-token` | Do the game's mod and the sidecar read the same local token file, and does it exist with owner-only permissions? | Same path, file present, mode `0600` | `FAIL`. This is the single cause of the local authentication refusal, and it is invisible from either process alone | `A-401` | `contract-a.md` §21, A47; *Credentials, TLS, and the retired LAN token* in `dev_environment.md` |
| `mod-connected` | Is a game connected to this sidecar? | A live local session with heartbeats arriving | `FAIL`. **The check must then run `mod-log` before saying anything else**, because the two causes have different remedies | `LOCAL-CONFIGRACE`, `LOCAL-STARVATION` | Readiness read off the status endpoint rather than a log; the config race and starvation traps in *Gotchas* |
| `mod-log` | If no game is connected, which of the two traps is it? | `SKIP` when `mod-connected` passed | `FAIL` naming **one** of them, from the mod log a packaged install writes at `<game folder>\BepInEx\LogOutput.log`: an error line reading `[M2] configuration failed — the multiverse client stays off:` is the race; **a log with no `Bibites Multiverse <version> loaded` line at all — or no log file — is starvation**. **The tell is an absence**, and this check exists because a person who does not know both traps exist cannot tell them apart. On a one-world install the same absence more often means the plugin is not in `BepInEx\plugins`, so the check reports the path it looked in | `LOCAL-CONFIGRACE`, `LOCAL-STARVATION` | *The FIRST bring-up after a deploy… races on the config file*; *The five-instance ceiling, and the log-file starvation trap*; the packaged install's own layout (`release/kit/Install-BibitesMultiverse.ps1`) |
| `export-edges` | Can every edge this world declared for export actually carry an organism on this map? | Every declared edge lies on an axis the map has | `FAIL` when **none** can — the session is refused. `WARN`, once, per edge that cannot when at least one can: **legal, unchanged, and not a fault**, and the warning deliberately states no remedy because the remedy is a map that grows an axis | `A-4007`, `LANE-A50-partial` | `contract-a.md` §21, A50 — the refusal M5 invents on this wire |
| `time-scale` | Is this world running at the speed it was told to, and is the machine keeping up? | Applied speed matches the configured target, and the achieved rate is within a stated band of it | **Neither half is answerable yet, so the check reports both readings and judges neither.** There is no configured target anywhere on this machine: time scale is report-only on both wires, nothing sets one, and the control surface that would carry one is D23's, in M6. And the band between applied and achieved is WP8's (§6). So the verdict is `UNKNOWN` with the applied scale, the achieved rate and the percentage between them printed — **the gap is the reading** — plus the two traps: a world restores its own speed after it settles, which can land after a speed command; and **the first speed command after a world load reports `1.00` and sticks there** — a second, later one takes. **The achieved rate is measured by the sidecar itself**, Δ simulated time over Δ wall from its own heartbeats, on the archive's guards and adding no wire field | `LOCAL-TIMESCALE`, `BCLAIM-rate_limited` | *A world can be at the wrong time scale, and the rig's own command may not stick to it*; the drift observed hours after a clean start, not only across a restart |
| `journal-replay` | Did this sidecar's journal replay cleanly at its last start? | **Zero** discarded bytes | `FAIL` on any non-zero count, quoting the number. Complete records behind a tear are gone, and the count is the only evidence that ever existed | `LOCAL-JOURNALTORN` | *A damaged journal is loud*; the rolling-restart record, whose every row reads zero discarded bytes |
| `journal-depths` | Are the custody, paced and held depths healthy, and are the bounce counters moving? | Depths small and falling; the timeout-bounce counter not increasing | `WARN` on a paced depth that never falls — **that names a delivery rate set too low**, and it is read against this world's own configured rate rather than against a default, which has been changed three times. **A single-shot tool cannot see a trend, so it reads an AGE instead of a slope**: at this world's own `inboundRatePerSimMinute` and its applied speed, the current depth needs a computable number of wall seconds to drain, and an entry that has already waited longer than the whole queue should take is a queue that is not draining. That is the configuration's own arithmetic and not a threshold | `BMIG-OVERLOADED`, `AOUT-RATE_LIMITED` | The settled reading of the live map: all four depths at zero, no bypass; *Watch items* on reading a paced depth against its rate |
| `save-health` | Are this world's saves completing, on schedule, inside their stall budget? | Last save recent against the configured interval; stalls inside the budget; no failed save | `WARN` on a breach rate outside a stated band. The consequence of a long stall is a session churn and a short delivery pause, **not a lost organism** — arrivals are held in order for the whole silence. **The band is WP8's (§6) and the reading is not**, so: `WARN` when the last save breached D14's published 2 000 ms budget, saying that how OFTEN a breach is too often is a rate WP8 measures; `WARN` when the newest receipt is older than **two** configured intervals, because one interval may simply not have elapsed and after two a save that was due did not happen; and `SKIP` on `saveMinutes` 0, which is a world with its save timer off and a reading rather than a gap | `LOCAL-SAVESTALL`, `A-4004` | *The 2-second save-stall budget is breached routinely*; the dose response of stall length against the heartbeat deadline; the save-file rotation layout |
| `disk-headroom` | Is there room for what this machine will write? | Free space above a stated multiple of the journal ceiling, the log ceiling and the genome cache cap | `FAIL` below **one** of them — the sum of the ceilings this install has already promised itself it may write is arithmetic rather than a threshold, and below it the ceilings cannot all be honoured. Above it the check has a measurement and no criterion, so it reports the free space and the three ceilings and returns `UNKNOWN` naming WP3 (§6). **Nothing here shrinks a record**: the durable files grow with traffic, and a full disk has previously torn an append-only log and left thousands of zero-byte scratch files at the moment inodes were what had run out | `LOCAL-DISK`, `AOUT-JOURNAL_FULL`, `AOUT-JOURNAL_ERROR` | *The disk budget*; *What still grows forever*; `contract-b-m4.md` §20, B20 |
| `versions` | What is running here? | Always passes; it is a report | Prints the game, mod and sidecar versions and the two wire versions in one line, in the order a support conversation needs them | §8 of the taxonomy | *Versions* |

---

## 3. Checks — the path to the map

| Id | Question | `PASS` when | Otherwise | Taxonomy | Derived from |
|---|---|---|---|---|---|
| `relay-reachable` | Does the relay's address resolve and accept a connection? | Both | `FAIL`, distinguishing **name does not resolve** from **connects nowhere** from **connects and hangs**. On a rig this was a firewall rule and a port forward; on a stranger's machine it is a network they do not administer, so the check says which of the three it is and stops guessing | `B-401` preconditions | *Owner steps: making the relay reachable*; *A stale port proxy steals a port from Windows processes only*; *Never test reachability from inside the Linux environment against the host's own address* |
| `relay-tls` | Does the relay's certificate verify against this machine's trust store? | Chain, name and validity window all verify | `FAIL` naming the host, the presented name and the verification failure. **The check must test this machine's clock too**, because a wrong clock fails a valid certificate and the two produce the same error. It prints this machine's clock beside the certificate's window, and **the one asymmetry it can act on**: a certificate cannot be issued in the future, so a clock earlier than `notBefore` is almost certainly this machine's fault and the remedy says so; a clock after `notAfter` is genuinely ambiguous and the check names both actors rather than picking one. `SKIP` on a `ws://` address, which is a single-machine rehearsal. There is no skip and no prompt. The running sidecar's own line is the one to correlate with: `contract B: the relay's TLS certificate did not verify; NOT CONNECTING`, carrying the relay URL, the error, a remedy naming both operators, and what it will not do — skip verification, pin a certificate, or fall back to `ws://` | `B-TLS` | `contract-b-m4.md` §22, B23. **New in M5** — no by-hand precedent exists, because the rig ran plain on a LAN |
| `credential` | Does this world's credential open a session? | The upgrade succeeds and the handshake completes | `FAIL`, distinguishing **refused at the door** from **refused at the handshake for an identity mismatch**, which the wire already tells apart: the first is HTTP **401** before any WebSocket, logged as `contract B: the relay refused THIS PEER'S CREDENTIAL with HTTP 401` and pinning the backoff after five; the second is close `4003 "peerId does not match the authenticated credential"` after the connection was accepted, and means the stored identity and the credential have drifted apart. **Neither reaches any slot's `lastRefusal`**, so the status page is silent on both and this check is where a participant learns which one it is. **It does not dial**: rule 4 forbids a second session, so the answer is the running sidecar's own classified record of how its last session ended, and with no sidecar running the check is `UNKNOWN`. The one half it answers cold is the loudest — a credential that is not configured at all is a `FAIL` before any relay is involved | `B-401`, `B-4003b` | `contract-b-m4.md` §22, B22; *Credentials, TLS, and the retired LAN token* in `dev_environment.md` is the rig-era ancestor of this check |
| `contract-version` | Is this build's wire version admitted by this map? | At or above the map's published floor, same major | `FAIL` below the floor, quoting both versions — **the map publishes its floor precisely so a peer can read what it will fail before it fails it**. `FAIL` on a wrong major, which also covers a retired path | `B-4000`, `B-4003d` | `contract-b-m4.md` §22, B25, and the episode that earns it: a stale peer answered an upgraded neighbour's exports with a permanent refusal, and **two lanes ran at ten times everyone else's rate while two other worlds' queues pinned** |
| `game-version` | Is this machine's game build supported, and is it the build this map is on? | Supported by the matrix, and compatible with the map | `FAIL` on the first — this world cannot join. `WARN` on the second: **the map is partitioned along a version boundary**, which is the accepted behaviour after a staggered game update, and it ends when every machine is on the same build. **The matrix is [`support-matrix.md`](support-matrix.md)**, published beside each release, whose machine-readable block ships inside the archive as `support-matrix.json` — the same bytes the installer reads. The lookup is the SHA-256 of `The Bibites_Data\Managed\BibitesAssembly.dll` against `entries[].assemblySha256`, never a version string; on no match the check quotes the matrix's own `refusal` field, exactly as the installer does. A packaged install keeps its copy in the folder it was installed from, so the check reads that one and reports the release it belongs to | `INS-GAMEBUILD`, `B-4003c`, `A-4002` | *Steam auto-updates stay on*; `contract-a.md` §21 A48 and `contract-b-m4.md` §22 B31 — the four kept version gates; `docs/support-matrix.md` (D22) |
| `limits` | What ceilings is this map running with, and is this peer inside them? | Inside every published limit | `WARN` approaching one, `FAIL` after being shed for one, quoting the close reason, which names the limit, its value and the measurement — `maxFramesPerSecond 50 exceeded (peak 412/s over 3s)`. **The check reads the `limits` object the map published and never a constant of its own**: a `contract-b/4.0` relay sends the values it is actually running with on `HANDSHAKE_ACK` and on every `PEER_STATUS`, and every one of the eight is a knob its operator may have turned. The defaults it ships with are `maxConnectionsPerPeer` 2, `maxConnectionsPerAddress` 8, `maxFramesPerSecond` 50, `maxFrameBytes` 8388608, `maxBytesPerSecond` 4194304, `maxClaimsPerMinute` 12, `maxGenomeRequestsPerMinute` 30, `maxSubscribers` 4. **Two of them are not `4007`**: the per-address cap is HTTP 429 on the upgrade, and an oversize frame is close 1009. **Approaching one is a reading above 0.75 of the published value** — see §7, where that number is derived rather than chosen — and the peer measures **itself**: its own peak frames and bytes in a second, its largest frame, and its own claims in a minute, counted the way the relay counts them. Two of the eight it cannot measure and says so: `maxConnectionsPerAddress` is counted across every peer on this address, and `maxSubscribers` is not a thing a participant meets | `B-4007`, `B-429`, `B-1009`, `BCLAIM-rate_limited`, `BGEN-rate_limited` | `contract-b-m4.md` §3.3, §22 B24. **New in M5** — the wire had no capacity limit before it |
| `slot` | Does this world hold a slot, at the position it expects? | A grant, with a reason of granted, reclaimed or updated, and a position matching the persisted one | `FAIL` on a claim refused, naming the refusal reason. `UNKNOWN` while no grant has arrived — **a grant may land in the same second as the handshake or an hour later** | `BCLAIM-*` | The `wait_grant` step of the rolling-restart sequence, whose criterion is *its own slot, its own coordinate, reason reclaimed* |
| `edges` | Is every declared edge open, and if not, why? | Every declared edge reports a live peer | `WARN` per closed edge, **naming its reason and its actor**, because the reasons have four different actors between them and only one of them is the participant | `LANE-*` | The settled reading of the live map: every lane open, none bypassed, every one reporting a live peer |
| `neighbours` | What are the worlds this world exports to, and is one of them the problem? | Every candidate on each axis is live, has a game connected, and reports the same game version and a compatible wire version | `WARN` naming each candidate that differs, **and stating that the remedy is not at this machine**. It reproduces §8's own walk over the last `PEER_STATUS` and reports the peers that walk SKIPPED, which is exactly the set that made the lane close. **The wire-version half of the criterion is not answerable about another peer** and §4 says why | `LANE-peer_incompatible`, `LANE-peer_mod_absent`, `BMIG-MOD_ABSENT`, `AOUT-PEER_INCOMPATIBLE` | The stale-peer episode, and DQ8's last row — *the peer that suffers is not the peer that is stale* |

---

## 4. The `neighbours` check, and what it does and does not close

**DQ8's argument for this whole work package is one row of a table**: a world's lanes run
badly, its queues pin, and **the cause is in somebody else's install**. A support surface that
assumes each participant can diagnose their own machine is wrong about this system's most
likely public failure.

**The wire already gives the sufferer the evidence.** The map broadcasts, per world, its
liveness, whether a game is connected, its mod version, the contract-A version its mod's session
speaks and its game version — to every peer, not only to the operator. So `neighbours` can name
the world that is behind, from the machine that is suffering for it, without an operator reading
it out.

**One version in that list is not actually on the broadcast, and the implementation found it:**
a peer's **contract-B** version. `PEER_STATUS.slots[].stats` carries `modVersion` and
`contractAVersion` and nothing that names the map wire the peer speaks — the relay learns it on
that peer's `HANDSHAKE` and does not republish it. The consequence is bounded and worth stating
rather than papering over: `neighbours` names a game-build partition and a world with no game
behind it, which are the two causes DQ8's row is about, and it cannot name a neighbour that is a
release behind on the map wire. **That case does not go unreported** — `contract-version` answers
it for *this* peer against the map's published floor, which is where a wire-version problem is
actionable — but the sufferer cannot see it about somebody else. Closing it would need a field on
`PEER_STATUS`, which is a wire change and out of WP7's scope by construction (D1: this package
adds no wire message).

**What it cannot give is the remedy.** A participant cannot reach another participant: there is
no directory, no contact field, and nothing on this wire that carries a message between two
people. So the check's output is deliberately shaped as *evidence to hand to the operator*, not
as an action:

```
WARN     neighbours        a world this one exports to is the reason a lane is closed
    slot 4 reports game 0.6.4.0; this world runs 0.6.3.1. Lanes to it are closed by design
    until both are on one build
    remedy: THE REMEDY IS NOT AT THIS MACHINE. …this is evidence to hand to the operator:
    they are the only party who can see both ends and tell the other owner…
    taxonomy: LANE-peer_incompatible, AOUT-PEER_INCOMPATIBLE — who acts: operator
```

That is the strongest honest form. It turns DQ8's worst class of failure — *the peer cannot see
the cause, and the cause is not on their machine* — into *the peer can see the cause, name it,
and hand it to the one party who can act on it*. **The half that stays open is the contact
path**, and it is an operator obligation rather than a wire feature.

---

## 5. What `--diagnose` deliberately does not check

Named so that nobody adds them later without arguing for them.

- **Another world's disk, journal, saves or logs.** Custody is local and there is no protocol
  that reads them. The map's honest answer for *which organisms is that world holding* is
  **"ask the peer"**.
- **Whether a stat is true.** A credential authenticates who is speaking, not what they say. A
  world can report any population or census it likes, and no rule on this wire will ever make a
  stat true; what a credential changes is that the stat is attributable.
- **Whether the map's operator is doing their job.** Restart policy, backups, monitoring and
  certificate renewal are the hosted service's own commitments, published rather than probed.
- **The payload assumption.** Whether an organism serialized by one game build loads in another
  is **assumed and untested**, by decision. The gates that would let it be tested are the same
  gates that are deliberately kept, so no diagnostic can exercise it, and reporting it as
  verified would be a lie.

---

## 6. Where each check's criterion has to come from

Three checks have criteria the wire does not fix, and each is a package's to supply. They are
called out here so the implementation arc does not invent a threshold — **and it did not**. Each
row's last column is what the built check does while the number is missing:

| Check | The missing number | Owner | What it does meanwhile |
|---|---|---|---|
| `save-health` | The breach-rate band that is a `WARN` rather than a fact of life. The project's own deployment breaches its stall budget routinely, and the honest band is a measurement rather than a target | **WP8**, from the playtest's record | Warns on the reading it can defend — a last save over D14's published 2 000 ms — and says in the same breath that the RATE is WP8's to measure |
| `disk-headroom` | The multiple of the ceilings that counts as headroom, and the per-participant growth arithmetic behind it | **WP3**, which owns the retention rule and its arithmetic | Fails below one multiple, which is arithmetic; above it reports the free space and the three ceilings and returns `UNKNOWN` naming WP3 |
| `time-scale` | The band between applied and achieved speed that is a `WARN`. At a speed a machine cannot meet the two come apart completely, and **the gap is the reading** | **WP8**, and Risk 9 is why it matters — a stranger's world at the wrong speed corrupts every rate the playtest measures | Measures the achieved rate itself and prints it beside the applied one with the percentage between them, and judges nothing. **A second half of that row is not WP8's and is not missing either — it is absent**: there is no configured target on this machine to compare the applied value against, because time scale is report-only on both wires and the control surface that would set one is D23's, in M6 |

---

## 7. The slots

| Slot | What is missing | Owner |
|---|---|---|
| §6 | The three thresholds above | **WP3**, **WP8** |

**WP7's own two are closed by the implementation arc.** §1 now fixes the exit codes, the flags,
the output shape and the `multiverse-diagnose/1` JSON schema, and names the one write the command
makes and where its live half comes from.

**And §3 `limits`' band is 0.75 of whatever the map published, DERIVED rather than chosen.** This
sidecar's own paced sender admits at most `sendPaceRateFraction + sendPaceBurstFraction` of a
published ceiling in any one second — half sustained plus a quarter of burst — so three quarters
is the most a peer doing exactly what B24 asks of it can produce. A band below that would warn
about correct behaviour; a band above it would be a number this project invented. It is a
fraction and never an absolute, for the same reason the pacer is one: every limit is a knob, the
relay publishes what it runs with, and an operator who raises one raises this with it at the next
connect with nothing to redeploy at this end.

**WP2's and WP4's slots are closed** (0393698, dc9d01f, dfbd1dc): `relay-tls`'s and
`credential`'s refusal texts and paired log lines, and `limits`' published table, are quoted
above from what those packages ship.

**WP6's two are closed as well**: `mod-log` names the packaged install's log path and quotes the
line each trap writes, and `game-version` names the support matrix, the file that carries it into
the archive, and the hash the lookup is on. Two checks also gained a packaged fact worth stating
here rather than leaving to the implementation: `data-dir` resolves
`%LOCALAPPDATA%\BibitesMultiverse\data` on a packaged install, and `contract-a-token` finds its
file at `data\contract-a.token`, which is the path `Start-Multiverse.ps1` puts in
`MULTIVERSE_CONTRACT_A_TOKEN_FILE` — so a mismatch between the two processes is a hand-edited
start script and the check can say so.

---

## 8. The participant's own-slot view — `multiverse-sidecar --my-slot`

**A sibling command, not a check**, and it is the other half of what WP7 owes: *a participant can
read their own slot's liveness, lanes, paced depth and last save without an operator*
(`m5_considerations.md`, DQ8). `--diagnose` judges; this one only shows, and the split is
deliberate — a view that graded itself would be a second, quieter diagnostic that nobody
specified.

**Why the sidecar and not the map's page.** DQ8 called it "mostly a presentation change, since
every field already exists", and the fields do exist — on the operator's status page. That page
lives at the map's address. It is one more thing that can be down, unreachable, or behind a
hosting decision WP3 has not made; and the participant who most needs to read their own slot is
the one whose link to the map is refused, which is exactly the participant least likely to reach
it. The sidecar is on their machine, is the process the question is about, and already holds
every field.

**So it adds no wire message** (D1). Its slot and position come from the `SECTOR_GRANT`, every
other world's liveness and game version from `PEER_STATUS`, its lanes from its own edge
computation, its depths from its own journal, its save receipt and speed from its mod's
`HEARTBEAT`. Nothing here asks the relay for anything, and a hundred reads of it cost the map
nothing.

| | |
|---|---|
| **The command** | `multiverse-sidecar --my-slot [--json] [--data-dir <path>]` |
| **The surface it reads** | `GET /my-slot` on the Contract A loopback listener, found through `<data-dir>/listen.addr`. JSON, schema `multiverse-own-slot/1` |
| **What it shows** | place and liveness; the map link and the last thing that refused it; this slot's `lastRefusal` and its last placement answer; the game, its versions, its population, and **applied speed beside achieved speed**; the last save against this world's own interval; one line per declared lane with its reason; custody, paced and held depths **with the delivery rate they are queued behind and the age of the oldest entry**; and the whole map's worlds with their liveness, their game and their build |
| **Exit** | `0` when it printed a view, `1` when there was none — and the failure names `--diagnose`, which answers without a running sidecar |
| **Authentication** | **None, deliberately.** A47's bearer token is an authority control on a wire that MOVES ORGANISMS; this endpoint moves nothing, answers only `GET`, and carries no field that is not already broadcast to every peer in `PEER_STATUS`. Putting it behind the token would also make the one diagnosis a participant most needs — *my mod cannot authenticate* — the one diagnosis they cannot run |
| **Secrets** | The same rule as §1: never one, in whole or in prefix. It carries the token file's PATH and whether a credential is configured, and nothing else about either |
| **Untrusted text** | Every peer id, game version, close reason and `lastRefusal` is escaped for the terminal before it is printed. A terminal's markup is the control character, and this surface renders other people's chosen text on the participant's own machine (`contract-b-m4.md` §22, B30) |

**This specification closes when every check runs and the playtest finds no failure that
`--diagnose` was silent about** (`m5_considerations.md`, *Exit Test*, Part 6).
