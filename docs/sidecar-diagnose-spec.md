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

**Status: draft spine, WP7.** Checks are stated against the wire as WP1 published it. Where a
value, a wording or an exit code belongs to a package that has not landed, the row carries a
slot. §7 collects them.

---

## 1. The contract

| Rule | Statement |
|---|---|
| **It changes nothing** | `--diagnose` is read-only against the participant's machine and against the map. It **MUST NOT** rotate or mint a credential, restart a process, set a time scale, write to the journal, claim a slot, or send any frame that is not a connection this specification names. A diagnostic that repairs is a diagnostic nobody can run twice on the same evidence. |
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

**Proposed, for the implementation arc to fix or replace:**

```
multiverse-sidecar --diagnose [--json] [--check <id>[,<id>…]] [--timeout <duration>]
```

| Exit code | Meaning |
|---|---|
| `0` | No `FAIL`. `WARN` and `UNKNOWN` may be present. |
| `1` | At least one `FAIL`. |
| `2` | The diagnostic itself could not run — no configuration found, no data directory, arguments rejected. |

**The human form is the default and the primary one.** One line per check: verdict, id, one
sentence, and on anything but `PASS` the taxonomy id and the actor. `--json` emits the same
records for a person who is pasting them into a support conversation, and its shape is stable
across releases so that a report from an old build is still readable.

> **SLOT — WP7, later arc.** The exit codes, the flag names, the output format and the JSON
> schema. Everything in this subsection is a proposal from the spec, not a fixed contract.

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
| `stale-process` | Is exactly one sidecar running against this data directory? | One live process | `FAIL` on two. `WARN` on a recorded process id that is dead — a stale record makes a status query claim the thing is running when it is not, and pid numbers are reused after a reboot | — | Reboot ritual step 2; *Only one rig can run at a time* |
| `contract-a-token` | Do the game's mod and the sidecar read the same local token file, and does it exist with owner-only permissions? | Same path, file present, mode `0600` | `FAIL`. This is the single cause of the local authentication refusal, and it is invisible from either process alone | `A-401` | `contract-a.md` §21, A47; *Credentials, TLS, and the retired LAN token* in `dev_environment.md` |
| `mod-connected` | Is a game connected to this sidecar? | A live local session with heartbeats arriving | `FAIL`. **The check must then run `mod-log` before saying anything else**, because the two causes have different remedies | `LOCAL-CONFIGRACE`, `LOCAL-STARVATION` | Readiness read off the status endpoint rather than a log; the config race and starvation traps in *Gotchas* |
| `mod-log` | If no game is connected, which of the two traps is it? | `SKIP` when `mod-connected` passed | `FAIL` naming **one** of them: a log containing a configuration failure is the race; **no log file at all is starvation**. **The tell is an absence**, and this check exists because a person who does not know both traps exist cannot tell them apart | `LOCAL-CONFIGRACE`, `LOCAL-STARVATION` | *The FIRST bring-up after a deploy… races on the config file*; *The five-instance ceiling, and the log-file starvation trap* |
| `export-edges` | Can every edge this world declared for export actually carry an organism on this map? | Every declared edge lies on an axis the map has | `FAIL` when **none** can — the session is refused. `WARN`, once, per edge that cannot when at least one can: **legal, unchanged, and not a fault**, and the warning deliberately states no remedy because the remedy is a map that grows an axis | `A-4007`, `LANE-A50-partial` | `contract-a.md` §21, A50 — the refusal M5 invents on this wire |
| `time-scale` | Is this world running at the speed it was told to, and is the machine keeping up? | Applied speed matches the configured target, and the achieved rate is within a stated band of it | `WARN` on a mismatch. **Two traps belong to this check**: a world restores its own speed after it settles, which can land after a speed command; and **the first speed command after a world load reports `1.00` and sticks there** — a second, later one takes | `LOCAL-TIMESCALE`, `BCLAIM-rate_limited` | *A world can be at the wrong time scale, and the rig's own command may not stick to it*; the drift observed hours after a clean start, not only across a restart |
| `journal-replay` | Did this sidecar's journal replay cleanly at its last start? | **Zero** discarded bytes | `FAIL` on any non-zero count, quoting the number. Complete records behind a tear are gone, and the count is the only evidence that ever existed | `LOCAL-JOURNALTORN` | *A damaged journal is loud*; the rolling-restart record, whose every row reads zero discarded bytes |
| `journal-depths` | Are the custody, paced and held depths healthy, and are the bounce counters moving? | Depths small and falling; the timeout-bounce counter not increasing | `WARN` on a paced depth that never falls — **that names a delivery rate set too low**, and it is read against this world's own configured rate rather than against a default, which has been changed three times | `BMIG-OVERLOADED`, `AOUT-RATE_LIMITED` | The settled reading of the live map: all four depths at zero, no bypass; *Watch items* on reading a paced depth against its rate |
| `save-health` | Are this world's saves completing, on schedule, inside their stall budget? | Last save recent against the configured interval; stalls inside the budget; no failed save | `WARN` on a breach rate outside a stated band. The consequence of a long stall is a session churn and a short delivery pause, **not a lost organism** — arrivals are held in order for the whole silence | `LOCAL-SAVESTALL`, `A-4004` | *The 2-second save-stall budget is breached routinely*; the dose response of stall length against the heartbeat deadline; the save-file rotation layout |
| `disk-headroom` | Is there room for what this machine will write? | Free space above a stated multiple of the journal ceiling, the log ceiling and the genome cache cap | `FAIL` below it. **Nothing here shrinks a record**: the durable files grow with traffic, and a full disk has previously torn an append-only log and left thousands of zero-byte scratch files at the moment inodes were what had run out | `LOCAL-DISK`, `AOUT-JOURNAL_FULL`, `AOUT-JOURNAL_ERROR` | *The disk budget*; *What still grows forever*; `contract-b-m4.md` §20, B20 |
| `versions` | What is running here? | Always passes; it is a report | Prints the game, mod and sidecar versions and the two wire versions in one line, in the order a support conversation needs them | §8 of the taxonomy | *Versions* |

---

## 3. Checks — the path to the map

| Id | Question | `PASS` when | Otherwise | Taxonomy | Derived from |
|---|---|---|---|---|---|
| `relay-reachable` | Does the relay's address resolve and accept a connection? | Both | `FAIL`, distinguishing **name does not resolve** from **connects nowhere** from **connects and hangs**. On a rig this was a firewall rule and a port forward; on a stranger's machine it is a network they do not administer, so the check says which of the three it is and stops guessing | `B-401` preconditions | *Owner steps: making the relay reachable*; *A stale port proxy steals a port from Windows processes only*; *Never test reachability from inside the Linux environment against the host's own address* |
| `relay-tls` | Does the relay's certificate verify against this machine's trust store? | Chain, name and validity window all verify | `FAIL` naming the host, the presented name and the verification failure. **The check must test this machine's clock too**, because a wrong clock fails a valid certificate and the two produce the same error. There is no skip and no prompt | `B-TLS` | `contract-b-m4.md` §22, B23. **New in M5** — no by-hand precedent exists, because the rig ran plain on a LAN |
| `credential` | Does this world's credential open a session? | The upgrade succeeds and the handshake completes | `FAIL`, distinguishing **refused at the door** from **refused at the handshake for an identity mismatch**. The second means the stored identity and the credential have drifted apart | `B-401`, `B-4003b` | `contract-b-m4.md` §22, B22; *Credentials, TLS, and the retired LAN token* in `dev_environment.md` is the rig-era ancestor of this check |
| `contract-version` | Is this build's wire version admitted by this map? | At or above the map's published floor, same major | `FAIL` below the floor, quoting both versions — **the map publishes its floor precisely so a peer can read what it will fail before it fails it**. `FAIL` on a wrong major, which also covers a retired path | `B-4000`, `B-4003d` | `contract-b-m4.md` §22, B25, and the episode that earns it: a stale peer answered an upgraded neighbour's exports with a permanent refusal, and **two lanes ran at ten times everyone else's rate while two other worlds' queues pinned** |
| `game-version` | Is this machine's game build supported, and is it the build this map is on? | Supported by the matrix, and compatible with the map | `FAIL` on the first — this world cannot join. `WARN` on the second: **the map is partitioned along a version boundary**, which is the accepted behaviour after a staggered game update, and it ends when every machine is on the same build | `INS-GAMEBUILD`, `B-4003c`, `A-4002` | *Steam auto-updates stay on*; `contract-a.md` §21 A48 and `contract-b-m4.md` §22 B31 — the four kept version gates |
| `limits` | What ceilings is this map running with, and is this peer inside them? | Inside every published limit | `WARN` approaching one, `FAIL` after being shed for one. The map publishes the values it is **actually running with**, at connect and on every broadcast, so that a peer can respect a ceiling instead of discovering it as a close | `B-4007`, `B-429`, `BCLAIM-rate_limited`, `BGEN-rate_limited` | `contract-b-m4.md` §3.3, §22 B24. **New in M5** — the wire had no capacity limit before it |
| `slot` | Does this world hold a slot, at the position it expects? | A grant, with a reason of granted, reclaimed or updated, and a position matching the persisted one | `FAIL` on a claim refused, naming the refusal reason. `UNKNOWN` while no grant has arrived — **a grant may land in the same second as the handshake or an hour later** | `BCLAIM-*` | The `wait_grant` step of the rolling-restart sequence, whose criterion is *its own slot, its own coordinate, reason reclaimed* |
| `edges` | Is every declared edge open, and if not, why? | Every declared edge reports a live peer | `WARN` per closed edge, **naming its reason and its actor**, because the reasons have four different actors between them and only one of them is the participant | `LANE-*` | The settled reading of the live map: every lane open, none bypassed, every one reporting a live peer |
| `neighbours` | What are the worlds this world exports to, and is one of them the problem? | Every candidate on each axis is live, has a game connected, and reports the same game version and a compatible wire version | `WARN` naming each candidate that differs, **and stating that the remedy is not at this machine**. See §4 | `LANE-peer_incompatible`, `LANE-peer_mod_absent`, `BMIG-MOD_ABSENT`, `AOUT-PEER_INCOMPATIBLE` | The stale-peer episode, and DQ8's last row — *the peer that suffers is not the peer that is stale* |

---

## 4. The `neighbours` check, and what it does and does not close

**DQ8's argument for this whole work package is one row of a table**: a world's lanes run
badly, its queues pin, and **the cause is in somebody else's install**. A support surface that
assumes each participant can diagnose their own machine is wrong about this system's most
likely public failure.

**The wire already gives the sufferer the evidence.** The map broadcasts, per world, its
liveness, whether a game is connected, its mod version, its wire version and its game version —
to every peer, not only to the operator. So `neighbours` can name the world that is behind, from
the machine that is suffering for it, without an operator reading it out.

**What it cannot give is the remedy.** A participant cannot reach another participant: there is
no directory, no contact field, and nothing on this wire that carries a message between two
people. So the check's output is deliberately shaped as *evidence to hand to the operator*, not
as an action:

> `WARN  neighbours  slot 4 (peer-…) reports game 0.6.4.0; this world runs 0.6.3.1.
>       Lanes to it are closed by design until both are on one build.
>       Taxonomy: LANE-peer_incompatible — operator acts.`

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
called out here so the implementation arc does not invent a threshold:

| Check | The missing number | Owner |
|---|---|---|
| `save-health` | The breach-rate band that is a `WARN` rather than a fact of life. The project's own deployment breaches its stall budget routinely, and the honest band is a measurement rather than a target | **WP8**, from the playtest's record |
| `disk-headroom` | The multiple of the ceilings that counts as headroom, and the per-participant growth arithmetic behind it | **WP3**, which owns the retention rule and its arithmetic |
| `time-scale` | The band between applied and achieved speed that is a `WARN`. At a speed a machine cannot meet the two come apart completely, and **the gap is the reading** | **WP8**, and Risk 9 is why it matters — a stranger's world at the wrong speed corrupts every rate the playtest measures |

---

## 7. The slots

| Slot | What is missing | Owner |
|---|---|---|
| §1 | Exit codes, flag names, output format, JSON schema | **WP7**, later arc |
| §2 `mod-log` | The exact log wording a packaged install emits for each of the two traps | **WP6** |
| §3 `relay-tls`, `credential` | The refusal text and the paired log lines, as the implementation emits them | **WP2** |
| §3 `limits` | The map's published values, and the approaching-a-limit band | **WP4** |
| §3 `game-version` | The support matrix's location and lookup | **WP6** |
| §6 | The three thresholds above | **WP3**, **WP8** |

**This specification closes when every check runs and the playtest finds no failure that
`--diagnose` was silent about** (`m5_considerations.md`, *Exit Test*, Part 6).
