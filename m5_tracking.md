# M5 Implementation Tracking

**Status: WP1 is done — the wire M5 ships is published (`contract-b/4.0`, `contract-a/2.4`;
commits 35f208f and 350988e, 2026-08-11). Seven packages remain; WP2 and WP7's documentation
spine are the startable pair.**

This is the orchestrator's execution state for M5. `m5_considerations.md` is the milestone's
design and it is ratified; this document is what is *done* against it, what is next, and the
rules the doing has to follow. The two are meant to be read together and never to say the same
thing twice.

## What this document is, and how to use it

**Read this, then read the sections it points at.** A session that opens M5 needs four things
before it dispatches anything: the decided state (below, compressed), the work-package order,
the operating rules this project runs on, and the constraints the living deployment puts on
every rollout. All four are here. Everything else — why a decision went the way it did, what a
package must contain, what the exit test asks — stays in `m5_considerations.md`, and this
document points rather than restates.

**The division of labour, stated so it survives:**

| Question | Where the answer lives |
|---|---|
| Why is the wire going to `contract-b/4`? What must WP3 contain? What does the exit test require? | `m5_considerations.md` — decisions, work packages, exit test |
| What is decided, project-wide, and what does it bind? | `system_decomposition.md` — D1–D25 and the milestone entries |
| What does the rig do when you deploy to it? | `dev_environment.md` — *The living deployment*, *Watch items*, *Gotchas* |
| Is WP4 started? What did it land? What is safe to start next? | **here** |

**This is a living document, and it is updated by integration rather than by appending.** When a
package lands, its row changes state and its evidence goes with it; when a package changes
shape, the row is rewritten rather than annotated with a correction underneath the old text. A
finding that contradicts something already written here is surfaced and resolved, not stacked
beside it. If an update would make a section too long or land in the wrong place, restructure
the section. The same rule the project applies to every other document applies to this one, and
the reason is the same: the next reader is a fresh session with no memory of the edit.

**One thing this document must never become** is a second copy of the design. If a status line
starts explaining *why*, the explanation belongs in `m5_considerations.md` and the line belongs
here as a pointer to it.

## The decided state, compressed

Nine decisions were ratified on 2026-08-10, and two of them were refined by the owner on
2026-08-11. Five became D21–D25 in `system_decomposition.md`; four are milestone-internal and
live in `m5_considerations.md`, *Decisions for the Owner*. One line each — the argument is at
the pointer.

| What is settled | The call | Pointer |
|---|---|---|
| **D21** — the wire's version | Per-peer credentials and TLS ship **together** at **`contract-b/4`**, the credential bound to the `peerId`; Contract A's bearer token is the minor **`contract-a/2.4`**. Taken in WP1, pre-strangers, for D13's reason | DQ1, decision 1 |
| **D22** — compatibility | **Layered.** The relay's membership test is the **contract** version; the game version is a per-machine matter answered by a published support matrix. Refined 2026-08-11: the bb8 payload stays opaque, cross-version loading is **assumed**, the envelope's game version is **diagnostic metadata** no *new* reader may parse into a capability decision | DQ5, decision 5 |
| **D22, second call** — the shipped gates | **They stay.** `VERSION_UNSUPPORTED`, `mapwalk`'s `peer_incompatible`, close `4002` and §6.1's relay refusal are all kept and written down as named exceptions. **WP1 retires nothing.** The accepted behaviour is a **partition** along a version boundary after a staggered game update; the whole cross-version design space waits for that to become an operational problem | DQ5, *ANSWERED 2026-08-11* |
| **D23** — the control surface | **Deferred to M6.** The operator surface stays read-only through the public release, and Risk 9 is the accepted cost, named and dated | Decision 2, Risk 9 |
| **D24** — the hosting commitment | A **bounded, announced** run. The period is stated to participants before they join, and the ending is a stated event with a wind-down procedure that says what becomes of `ring.json` and the archive's durable files | Decision 4, DQ2 |
| **D25** — the channel | **GitHub Releases**, with published checksums and the `Unblock-File` story on the release page itself. It pushes nothing, so the fleet moves by publication plus a relay-side minimum contract version. It also carries decision 7's stated default: **D17's four edges, client on**, stated in the packaging documentation and the installer's own output | Decision 6, decision 7, DQ4 |
| Archive retention (milestone-internal) | **The deciding happens in M5**, in WP3, even if the rule is *keep everything* — and the per-peer growth arithmetic ships to the hoster either way. It answers D6's graduation question with it; D24's announced ending is its deadline | Decision 3, DQ3, Risk 5 |
| The exit bar (milestone-internal) | **≥4 non-owner peers, ≥72 continuous hours, zero operator actions on any participant's machine.** Relay-side actions are allowed and counted | Decision 8, *Exit Test* |
| The velocity floor (milestone-internal) | **Not built.** Instrumented during the playtest, and the measurement decides whether it is ever built. Contract Change 14 leaves the milestone; Risk 8 is the accepted cost for the duration | Decision 9, Risk 8 |

**Nothing in M5 is gated on a signature any more.** Every package below is startable in
dependency order.

## Work packages

Eight packages, as `m5_considerations.md`, *Work Packages* defines them. **Delivers** is one
line and is not the package's contents; **Done when** is the bar the package reports against.
Neither replaces reading the package.

| WP | Delivers | Depends on | Status | Done when |
|---|---|---|---|---|
| **WP1** — The contract amendments | The wire M5 ships: `contract-b/4`, `contract-a/2.4`, and the sixteen M5 rows of *Contract Changes Needed* | Nothing — its gate opened 2026-08-10 | **Done 2026-08-11.** Commit 35f208f landed item 16; commit 350988e landed the wave: `contract-b/4.0` as `contract-b-m4.md` §22 (B22–B32, in place, path `/contract-b/v4`), `contract-a/2.4` as `contract-a.md` §21 (A47–A52, path unchanged), `genome-hash.md` re-verified at `bb8-genome/1`. Every done-when clause met: row 13 assessed (A51 — stays open and moot), row 14 not written, D22's statement in B31 + A48 with the four gates as named exceptions, migration note in B32/A52. The two sets cross-cite by §- and amendment number | Both contracts published at their new versions; every M5 row applied, or assessed and recorded as closed (row 13); one worked example per changed or added message; D22's layered statement written with row 10a's diagnostic-only rule binding **new readers only** and the four shipped gates named as kept exceptions; the migration note for the living deployment written; item 16's stale-milestone corrections landed. Row 14 is **not** written |
| **WP2** — Transport security | TLS on Contract B, per-peer credentials bound to `peerId`, the archive as an authorised subscriber, a bearer token on Contract A | WP1 | **Go side done 2026-08-11** (commit 0393698): B22 credentials + join strings (`peercred`, verifier store in `<data-dir>/peers.json`), B23 TLS with rotation-surviving reload and the off-loopback 426 rule, B25 version floor (default unset), B27 subscribe grant with the boundary tested as not-a-privileged-view, B32 path move with v2/v3 retired, A47 sidecar side (`modtoken`); Risk 1's adversarial test passes both halves (`TestRisk1_ValidCredentialWithAnotherPeersID`). **Mod side done** (dc9d01f: 0.6.4 sends the bearer token at `contract-a/2.4`; 53c2f1f batches A49's flag and A50's 4007 handling into the same undeployed build). Remaining: the rig crossing — one five-games-down window, relay restart, far-end bundle + a far-end CA trust step; crossing prep material in `e2e/crossing/` | The functional path works **and Risk 1's adversarial test passes**: a peer presenting a valid credential with another peer's `peerId` is refused at the handshake and the legitimate peer observes nothing. The archive's visibility boundary is stated, not implied. The living deployment has crossed to `contract-b/4` in lockstep with zero discarded journal bytes |
| **WP3** — The hosted deployment | The relay and archive as a *service*: VPS, DNS, ACME, supervision, monitoring, backup, restart policy — plus the forward receipt, D24's commitment and the retention rule | WP2 | **Not started** | The service restarts on exit and on boot; a certificate rotation is survived rather than scheduled around; monitoring reaches a person for peers lost, persistent bypasses, free disk and error lines; `ring.json` and the archive's three durable files are backed up; the restart policy and its participant-facing statement are written; the forward receipt ships **with its measured per-migration cost at rate**; D24's period is announced and its wind-down written; the retention rule exists with the per-peer growth arithmetic beside it |
| **WP4** — Capacity, abuse limits, admin path | The published limit table, the authenticated admin path, the render deny list, and the three cheap contract closures | WP1, WP2 | **Not started**, with two heads start: the MOD halves of closures 11 (A49) and 12 (A50) were pulled forward into the 0.6.4 window batch (53c2f1f); their sidecar halves stay here and land by rolling restart. Closure 13 was WP1's (assessed, A51) | Every limit is a knob, every peer-visible one is published on the stats block, and **every one is countable at the frame level** — D1 survives the package. Release, handover and eviction run over an authenticated path and keep the printed held-entry report. Contract Changes 11, 12 and 13 are closed |
| **WP5** — Placement and route-around under churn | Auto-placement, the coalescing window, the slot-space policy — proven against a synthetic churn harness | WP1, WP2 | **Not started** | Holes fill before an axis extends, and the axis rule is stated; the coalescing window and the `PEER_STATUS` broadcast bound are measured; `maxSlotEverIssued` growth is **tested rather than assumed**; the operator's answer for a position that will never be filled again is written; and the churn harness — join, leave, return, never return — has been **run to exhaustion before WP8 involves anybody** |
| **WP6** — The package | A GitHub release a stranger can install from, with the defaults audit and the support matrix | WP2 (input: `farend/`) — **needs the game** | **Not started** | An installer with no build toolchain and no instruction to disable a security control; the release page carries artifacts, published checksums and the `Unblock-File` story before the download; D17's default is stated in the documentation and in the installer's output; the four-default audit is complete (`--insecure-no-token`, the archive bind habit, the exclusion default, the save defaults); D22's support matrix is published beside the release and the installer's per-machine check enforces the entry; the uninstall leaves the player's game as it found it |
| **WP7** — The support surface | The error taxonomy, `--diagnose`, the participant's view of their own slot, renderer escaping | WP1 — **needs the game** for the mod-side failure modes | **Spine drafted 2026-08-11** — `docs/`: the error taxonomy (91 ids, every one naming remedy and actor), the participant set (install/join/diagnose/leave), and the `--diagnose` spec (21 named checks). 20 marked slots owned by WP2/WP3/WP4/WP6/WP8 and this package's later arcs; closes after WP2 and WP4 land the refusal texture, the CI escaping test and the slot view | Every refusal in the taxonomy names the remedy **and who must act**; `multiverse-sidecar --diagnose` runs the checks the runbook does by hand; a participant can read their own slot's liveness, lanes, paced depth and last save without an operator; renderer escaping has the markup-species-name test **in CI, not by eye**; the documentation covers install, join, diagnose and leave |
| **WP8** — The playtest | The exit test, the two instrumented watches, and `m5_findings.md` | WP2–WP7 — **needs the game on other people's machines** | **Not started** | The eight parts of *Exit Test* run against the ratified bar; the velocity-floor signature and the version-skew **partition** are counted rather than eyeballed; the payload assumption is reported as **untested**, never as stable; the record — per-peer archive growth, crossing rates, epoch rate, broadcast volume, disk arithmetic — is captured; and `m5_findings.md` is written. M3 and M4 both left this deliverable open |

### Recommended sequence

**1. WP1 alone.** *"Write WP1 before any code. M2 and M3 both paid for the alternative."* It is
the root dependency: `contract-b/4` is the wire every other package builds on, and taking the
major before strangers run the build is the whole of D21's reasoning. Nothing else should start
while the wire is being written, because everything else would be built against a moving target.

**2. WP2 next, with WP7's documentation spine beside it.** WP2 gates four of the remaining six
packages. **WP7 is the only package that does not depend on WP2** — it depends on WP1 alone — so
its taxonomy skeleton, its documentation set and `--diagnose`'s check list can be drafted in
parallel. It cannot *finish* there: half the refusals the taxonomy has to cover are the ones
WP2 invents, so WP7 closes after WP2 lands.

**3. After WP2 — WP3 in parallel with WP4, then WP5.** WP3, WP4 and WP5 declare no dependency on
each other, but they are not equally parallel in practice. WP3 is off-rig work (a VPS, DNS,
supervision), so it genuinely runs concurrently with anything. WP4 and WP5 both change relay
behaviour and both want the living deployment to verify against, so **serialise those two
against each other** rather than dispatching both at once — the rig is one map and two agents
restarting the same relay is how a rollout becomes an incident. WP4 first is the cheaper order:
its three contract closures (11, 12, 13) are additive and are the last moment they are cheap.

**4. WP6 after WP4 and WP5 land.** The plan only requires WP2, and that is a judgment call this
document is making rather than a constraint `m5_considerations.md` states: the package has to
carry the final defaults, the published limit table and the support matrix, so packaging before
WP4 and WP5 have settled the sidecar and the relay means packaging twice. It also needs the
game, which makes it a rollout rather than an edit.

**5. WP7 closes before WP8.** The taxonomy has to cover every refusal WP2 and WP4 added, or the
playtest discovers the gaps on strangers.

**6. WP8 last, and only after the harness and the abuse cases have been rehearsed.** Risk 2's
accepted cost is that the exit test **cannot be re-run cheaply** — it spends other people's
goodwill each time — which is exactly why WP5's churn harness runs to exhaustion first and WP4's
abuse cases are rehearsed against the rig before Part 5 is run against a live public relay.

**One rollout fact that shapes the middle of this sequence.** WP2 puts a bearer token on
Contract A, which is a **mod** change, so the rig's crossing to `contract-b/4` is a five-games-down
deploy and not a rolling sidecar change. Batch every other pending mod change into that same
window — the constraints below price a mod deploy at about 100 seconds of downtime plus a custody
burst, and each one also costs the far end a bundle rebuild.

### Carried actions that belong to no package

- **Keep the living deployment running, and treat it as M5's first participant.** Every upgrade
  the milestone ships has to move it, and it is the only peer whose before and after are fully
  observable. (`m5_considerations.md`, *Next Steps* 3.)
- **Run `phase5far` while there is still one far end that answers the phone.** It has never been
  run; it needs two commands typed at the second computer, and D9 forbids the rig to send them.
  After M5 every peer is a peer the rig cannot drive. (*Next Steps* 4, *Inherited threads*.)

## Orchestration protocol

These are the operating rules the session that produced this document proved. A fresh
orchestrator inherits them whole.

**Opus 5 subagents do all substantive work; the main loop orchestrates, integrates and commits.**
The orchestrator's job is to scope an arc, dispatch it, read the report, integrate the result and
commit it. It does not do the work itself.

**One agent per coherent arc.** An arc is design **plus** implementation **plus** deployment
**plus** verification **plus** the documentation the change owes — not a slice of each handed to
a different agent. A package this size will be several arcs; each arc is one agent, and the
agent owns its arc end to end so that the person who deployed it is the person who verifies it.

**Agents do not commit.** An agent reports the paths it changed and a proposed commit message;
the orchestrator stages and commits. This is not ceremony — parallel sessions share this
checkout, and concurrent staging is how one session's index picks up another's half-finished
work.

**Committing, therefore:**

- `git fetch` before staging, every time. Another session may have moved `origin/main`.
- **Stage explicit paths.** Never `git add -A`, never `git add .`, never `git commit -a`.
- Commit **sequentially**, one arc at a time, so the index is never shared between two writes.
- Message style: **imperative, descriptive, one line of subject**, matching the log — *Ratify the
  nine M5 decisions and mint D21-D25*, *Bound the archive's genome pump in work, burst and
  concurrency*. **No AI attribution, no generated-by footer, no co-author line**, in the message
  or in anything the change touches.
- Group commits where the material separates cleanly: **contracts** in one, **implementation plus
  rig plus documentation** in another, and the **far-end bundle** in its own, since that is a
  binary artifact with its own rebuild rule.

**Documentation is integrated, not appended.** Read the whole target document before changing it;
make targeted edits anchored on current content rather than whole-file writes; merge an addition
into the entry that already covers it instead of adding a second version; surface and resolve a
contradiction rather than leaving both readings standing; and revalidate the inbound and outbound
cross-references after the edit. `dev_environment.md`, `system_decomposition.md` and the contracts
are all documents another session may be editing at the same moment.

**The stop rule.** Anything beyond a documented pattern — an unfamiliar failure, a deployment
step no runbook describes, a rig state that does not match what `dev_environment.md` records —
means **stop, leave the system as found, and report with evidence**. Do not improvise a recovery
on a live deployment. An agent that stops with a clear account of what it saw has done its job;
an agent that guesses has spent the deployment's evidence.

## Live-rig constraints during M5

**The rig keeps running through the whole milestone.** Six worlds, two machines, a relay, an
archive and a collector that has been up since 2026-08-07. It is M5's first participant and the
only fleet that rehearses `contract-b/4`. Every constraint below is on record in
`dev_environment.md` and every one of them binds a rollout.

- **Heavy analysis and test runs go through `nice -n 19`.** `nice -n 19 go test -p 1 ./...` is the
  measured form. Unniced load on this host reproduces the sidecar session storm — **it is on
  record twice**, once as the 18:39Z–18:59Z episode and once as a deliberate reproduction at
  21:25Z the same evening that dropped seven sidecar sessions and three mods. Serialising the
  packages costs about 20 s of wall clock and buys the whole episode back. A churn episode dated
  to one of these is not a deployment fault — but it must be said in the record.
- **Build Go binaries to a scratch directory and `mv` over `bin/`.** Five running sidecars hold
  `bin/sidecar` open, so an in-place `go build -o ../bin/` fails with `ETXTBSY`. The rename is
  atomic and leaves every running process on its old inode until its own restart, which is what a
  rolling change needs. Verify per slot by comparing `/proc/<pid>/exe`'s inode against
  `bin/sidecar`'s.
- **Sidecar restarts are rolling: one at a time, and the gate is zero discarded journal bytes.**
  The proven sequence per slot is `kill_pid sidecar-<n> -TERM`, wait for the process,
  `start_sidecar <n>`, `wait_healthy`, `wait_grant`. Each sidecar must come back to **its own slot
  and its own coordinate** with `reason=reclaimed`, replay its journal with **zero** discarded
  bytes, and reopen all eight of its lanes. The map should never leave 24/24 `peer_live` in any
  sample.
- **A mod deploy takes all five games down at once**, because `deploy.sh` copies into
  `BepInEx/plugins/` and a running game locks the DLL. Budget ~100–110 s end to end: quit five,
  `deploy.sh`, then `start_game` **one at a time**. Expect a **custody replay burst** on the way
  back — `custodyDepth` 55 and `pacedDepth` 14 at its measured peak — draining to 0 in about four
  minutes with nothing lost.
- **Re-send the time scale after every world load, and read readiness off `/api/status`.** The
  first `timescale` after a load answers `Time.timeScale=1.00` and **the reported scale sticks
  there**; a second send twenty seconds later takes. A restarted game can land on a *different*
  BepInEx log file than the one it left, so a line mark taken before the quit is meaningless and a
  `wait_log` against it hangs — `/api/status` is per-slot and rotation-proof.
- **Archive restarts are expensive and permanent.** The replay is **~93 s of no status page** at
  the current ledger size and the process is down about **100 s**, during which the crossings that
  happen are **never copied to the ledger** — the last measured restart cost **1,940 crossings**,
  absent from the record forever. That is §5.1 working as designed: the traffic is untouched and
  only the record has the gap. **Batch the reasons to restart the archive**; never restart it to
  check something.
- **`ARCHIVE_HTTP=0.0.0.0:8796` must be preserved** on every bring-up
  (`ARCHIVE_HTTP=0.0.0.0:8796 ./e2e/run-m4-lan.sh up`). Without it the archive binds its compiled
  `127.0.0.1:8796` and the second computer's operator loses their only view of the map.
- **The far-end bundle is rebuilt after any mod or sidecar change.** `farend/dist/farend-bundle.zip`
  is **tracked**, and its rule is that it carries what this machine runs: run
  `farend/make-farend-bundle.sh` **after** `bibites-mod/deploy.sh` so the DLL in the zip is the one
  deployed here. Re-taking the bundle is not optional after a `contract-b` change — a stale far-end
  sidecar is what refuses an upgraded neighbour's exports. **The far end applies it at its
  operator's leisure**, and whether it has applied it cannot be determined from here; the one
  observable is a `client gone`/`client connected`/`reason=reclaimed` triple for `peer=slot-6` in
  `relay.log`.
- **Slot 6, the relay and the collector are not the orchestrator's to restart.** Slot 6 is the
  second computer and D9 forbids the rig to drive it. The collector is gitignored, hand-launched
  and relaunched by the owner after every reboot. A relay restart drops every peer's session at
  once and is an owner-level act, not a debugging step.
- **The BepInEx log-file starvation trap, on every five-game bring-up.** BepInEx hands out
  `LogOutput.log` and `.1`–`.4` and then gives up; an instance that gets no log file **does not
  merely lose its log — the mod never loads in it**, and it comes up `modConnected=false` with
  `exportEdges=[]`. The tell is an *absence*: starvation writes nothing at all, where the config
  race writes `configuration failed`. The remedy is to restart **that one instance** through the
  rig's own `start_game <n>`, so the environment matches the other four exactly.
- **Archive the BepInEx logs BEFORE any deploy.** `LogOutput.log` is truncated on launch, so a
  deploy that skips `archive_bepinex_logs` destroys the evidence window it is being deployed to
  measure. **It has happened once** — the `0.6.3` deploy cost roughly 33 saves' worth of history
  between 16:26Z and 17:43Z on 2026-08-10, and the gap is permanent.

## Standing watch items that interact with M5 work

Full readings and their evidence are in `dev_environment.md`, *The living deployment → Watch
items*. What follows is only why each one touches this milestone.

- **The 13-second heartbeat verdict is still accruing.** The raise from 3 500 ms landed
  2026-08-10 and the first reading points the right way without being a rate — 3 saves over the
  retired deadline, 0 sessions lost to them. WP2 and WP6 both ship into this: the timeout is a
  sidecar-side number a stranger will inherit.
- **The species-history serializer is the highest-value save-stall lever, and it is undesigned.**
  `lineageMs` is **53% of `writeMs`** and flat against population, and the term compounds because
  brains only get bigger. **43 of 76 saves still breach D14's 2 000 ms.** Risk 3's remaining
  escalation — *a save path that does not block the tick* — is unspent, it is the only lever that
  touches the stall rather than the cost, and it is **the owner's call, not a package's**. M5's
  exposure is Risk 3: the cadence ships as a package default onto machines nobody here can retune.
- **Slot 1 is the fragile world at ×100.** It oscillates 4–23 against its neighbours' 41–63, and
  at an achieved 25–32× it lives several times more simulated time per wall second than the worlds
  feeding it. Read it against `simulatedTime`, not wall clock. It is the world most likely to
  distort a WP5 churn measurement taken on the live map.
- **`genomeGaps` sits at ~46 000–65 000 with no drain window.** The rate cap is the mechanism and
  the magnitude is not the finding — **the absence of a drain window is**. This is the number
  **Risk 7** must be read against, not the 2 714 the item was opened on: after M5 a departed
  stranger makes an entry permanent, and a monotone queue at this size is what §21's bounds exist
  to stop making each archive reconnect more expensive than the last.
- **The `8796` firewall rule and portproxy have no record of being run.** The owner steps exist in
  `dev_environment.md` and the reboot ritual verifies only `8795`, so the status page is
  effectively loopback-only from off this machine. **M5 makes the question moot rather than
  answering it** — WP3 hosts the archive publicly behind a DNS name and TLS — but the connection is
  worth carrying: the participant-facing view WP7 owes is the same page, and it has never actually
  been reachable by anyone but the owner.

## Session-resume checklist

1. **`git fetch` first.** Parallel sessions share this checkout. Reconcile against `origin/main`
   before staging anything, and stage explicit paths only.
2. **Read the workspace and attention state if AI-viz is up.** That is where a standing picture of
   in-flight work and anything awaiting the owner's answer lives.
3. **Check rig health before any operation**, off `/api/status` rather than off a log: 6/6 slots
   live and `modConnected`, 24/24 lanes `peer_live` with no bypass, `pacedDepth`, `heldDepth` and
   `timeoutBounces` at 0, and the five local slots reporting `saveMinutes 10`, `saveKeep 6` and
   `timeScale 100`. A slot reporting `2`/`4` came up from a stale environment and needs its game
   restarted, not the rig. If the page does not answer, check whether the archive is inside its
   ~93 s replay before concluding anything.
4. **Check the memory directory is current** —
   `/home/ubuntu/.claude/projects/-mnt-wsl-data-bibites-multiverse/memory/MEMORY.md` and what it
   indexes — and update it if this document has moved past it.
5. **Then read the work-package table above**, pick the next startable package by the recommended
   sequence, and read that package in `m5_considerations.md` before dispatching an agent at it.
