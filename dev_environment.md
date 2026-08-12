# Development Environment

The full loop — edit, build, deploy, run, read logs — runs from WSL with no manual steps.

## Layout

| What | Where |
|---|---|
| Game install (Windows) | `/mnt/c/Program Files (x86)/Steam/steamapps/common/The Bibites` |
| Game assembly | `…/The Bibites_Data/Managed/BibitesAssembly.dll` |
| BepInEx log | `…/The Bibites/BepInEx/LogOutput.log` |
| Plugin project | `bibites-mod/` (source in `src/`, reference DLLs in `libs/` — see *The reference DLL set*) |
| Go module (`multiverse-relay`, `multiverse-sidecar`, `multiverse-archive`) | `go/` (module `multiverse`; binaries in `cmd/`, libraries in `internal/`). `cmd/worldstat`, `cmd/ringstat` and **`cmd/fakemod`** are rig tools rather than rig components — `fakemod` is a Contract A peer with no game, and *The five-instance ceiling* below is why it exists |
| Wire specifications | `contracts/` — `contract-a.md` (mod ↔ sidecar, **`contract-a/2.4`**, amended in place; §15 is the M4 set, §16 the species-identity set, §17 the species-census set, §18 the two-way-lane set (A38–A41), **§19 the world-settings set, A42–A44**, §20 the heartbeat set (A45–A46) and **§21 the M5 public-release set, A47–A52**), `contract-b-m4.md` (sidecar ↔ relay ↔ sidecar ↔ archive, **`contract-b/4.0`**; §14 is its reconciliation set, §15 the species-identity amendment, §16 the census amendment, §17 the two-way-lane and hop-feed amendment (B13–B15), §18 the pacing and speed readout, §19 the world-settings readout, **§20 the disk budget, B20** — the only amendment that changes no wire field — §21 the genome-pump bounds (B21) and **§22 the M5 public-release set, B22–B32**. **The running fleet speaks both of these as of 2026-08-11**: the living deployment crossed to `contract-b/4.0` / `contract-a/2.4` / mod `0.6.4` in a 7m18s window that day, per `e2e/crossing/RUNBOOK.md` — see *The crossing to `contract-b/4.0`* under *The living deployment*. Slot 6 crossed the same evening (far end, 2026-08-12 ~01:19Z), so the whole map now speaks both), `genome-hash.md` (the canonical genome projection, unchanged by M4 and by every amendment since — the block rides **beside** the blob, so no hash moves). `contract-b-m3.md` and `contract-b-m2.md` are the superseded M3 and M2 wires, kept as the record of what `contract-b/2` and `contract-b/1` said — **neither is current guidance** |
| Participant docs and the support surface | `docs/` — the error taxonomy, the participant set (install / join / diagnose / leave) and the `--diagnose` spec, written for a stranger rather than for this rig; `docs/README.md` states the audience rule. Drafted by M5's WP7 with its open slots marked by owning package. This document stays the rig's own book — nothing in `docs/` is a copy of it |
| Rigs and exit tests | `e2e/` — **`run-m4.sh` = the 3×2 six-slot grid on one machine** (the M4 local rehearsal; read its header before running it), **`run-m4-lan.sh` = the same map with slot 6 on the second computer** (the M4 exit-test rig; it sources `run-m4.sh` with `M4_LIB=1`), `run-m3.sh` = the three-slot ring rig on one machine, `run-m3-lan.sh` = the same ring with slot 2 on the second computer, `run-m2.sh` = the M2 two-sector rig (**historical**, speaks `contract-b/1`), `baseline.sh` = the T0/T1 capture, `journal.py` = journal reader. **The M3 scripts still speak the retired wire** — see *The M4 rigs* |
| Far-end bundle (the second computer) | `farend/` — `setup-farend.ps1`, `README.md`, `make-farend-bundle.sh`. The build scratch and the BepInEx download cache under `farend/dist/` are **gitignored**; `farend/dist/farend-bundle.zip` itself is **tracked**, because the second computer takes it out of a clone rather than off a USB stick |
| Rig runtime state — **gitignored** | `bin/` (built Go binaries), `e2e/data*/` (per-sidecar data dirs: journal, `peer-id`, remembered slot, genome cache — the D2 custody record of one machine's run), `e2e/relay-data*/` (the relay's `ring.json` slot reservations and, since 2026-08-11, its `peers.json` credential verifiers), `e2e/archive-data*/` (`migrations.jsonl` and the content-addressed genome store), `e2e/logs*/`, `e2e/run*/` (pid files). Each rig has its own suffixed set; the M4 LAN rig's are the `-m4-lan` ones and are the **living deployment's** |
| Retired rigs — **gitignored** | `e2e/<name>.tar.zst`. A retired runtime dir is still the record of a run that happened, so it is compressed in place rather than deleted: `e2e/data.tar.zst` (the M2/M3 journals, and **the only copy of the M4 resume test's input** — `data/slot-1/journal/journal.log`, the real in-flight hop of 2026-08-04; extract before re-running that test), `e2e/logs-m3-lan.tar.zst`, `e2e/archive-data.tar.zst`, `e2e/data-m4.tar.zst`. Restore one with `tar --zstd -xf e2e/<name>.tar.zst -C e2e/` |
| **The repo itself** | **Real location `/mnt/wsl/data/bibites-multiverse/`; `~/bibites-multiverse` is a symlink to it** (moved 2026-08-08 — see *The disk budget*). Every absolute path in every script, pid file and command line resolves through the symlink unchanged, so nothing had to be rewritten. `/mnt/wsl/data` is a separate 251 GB ext4 volume; the WSL root it left is 98 GB and shared with everything else on this machine |
| Secrets and TLS material — **never in the repo** | Per-peer credential secrets in `~/.multiverse/*.secret`, mode `600`; the relay's own key pair and CA in the **gitignored** `e2e/tls-m4-lan/`; each sidecar's minted `<data-dir>/contract-a.token`, mode `600`. The shared `~/.multiverse-token` is **retired since the crossing of 2026-08-11** and nothing reads it. See *Credentials, TLS, and the retired LAN token* below |
| The relay's third durable file | `e2e/relay-data-m4-lan/peers.json` — the credential **verifiers** (salted SHA-256, never a secret) and their grants. **Back it up with `ring.json`**: a lost verifier store costs a slot handover per peer |
| Decompiled game source | `decompiled/BibitesAssembly/` (654 files, grep this to find APIs) |
| Game user data (`Application.persistentDataPath`) | `/mnt/c/Users/<user>/AppData/LocalLow/The Bibites/The Bibites/` — holds `Savefiles/`, `Autosaves/`, `Scenarios/`, `Bibites/` |

## Versions

| Component | Version |
|---|---|
| The Bibites | Steam app 2736860, buildid 22383127; game version `0.6.3.1` — first read out of `The Bibites_Data/globalgamemanagers` (`bundleVersion`), **confirmed at runtime 2026-08-02**: the plugin logs `Application.version = 0.6.3.1` at startup |
| The plugin | `0.6.4` (`MultiversePlugin.Version`), **deployed to the five local games 2026-08-11 in the crossing window** — the **public-release build**, and the first that speaks **`contract-a/2.4`**: it presents a bearer token on the Contract A upgrade (§21 A47), reading the path in `MULTIVERSE_CONTRACT_A_TOKEN_FILE`; it reports `blobDroppedForSize` on `parents[]` (A49); and it handles the sidecar's `4007` close for an export set the map cannot use (A50). Underneath it is unchanged: `0.6.3`'s `SavePhases`, which times `SaveSystem.CreateSave` from the inside and puts the decomposition on every `[M4-SAVE]` line (*Watch items*, first item, and it is measurement only — a span it cannot resolve reads `0` rather than failing the save); `0.6.2`'s headless-speed work (`MinFpsGovernor`, which disarms the game's minimum-FPS servo in a process with no graphics device — *A world can be at the wrong time scale*, in Gotchas); `0.6.1`'s world-settings publication (§19 A42 — the exclusion list, the save interval, the keep count, save-on-quit and world wrapping); and the two-way-lane build's four-edge capture (§18 A38), migration exclusion list (A39) and two-lane portals. **The minor moved this time, and it did not for `0.6.2` or `0.6.3`** — those added no wire field, which is why the five local slots published `0.6.3` against `contract-a/2.3` through 2026-08-10. **Slot 6 crossed the evening of 2026-08-11** (far end, ~01:19Z 08-12): it took the `0.6.4` bundle (`32f41f8`) onto `contract-a/2.4` / `wss://…/contract-b/v4`, and its sidecar was brought current to the WP4/WP5 rebuild (`4dd2ac1`) ~9 min later — so slot 6 now publishes `0.6.4` / `contract-a/2.4` like the five local slots, and the whole map is on one version. The far-end bundle carries whatever DLL it was last built with; `farend/make-farend-bundle.sh` builds it fresh, so a bundle is only as current as its last rebuild |
| The Go side | **`contract-b/4.0`**, running on the living deployment since the crossing of **2026-08-11** — the M5 public-release set (§22, B22–B32), plus **§23's B33/B34 genome-retention horizon** and the archive batch, both live since the batched debt window of **2026-08-12** (*The living deployment*). The three that changed how this rig is *operated*: **B22** replaces the one shared LAN token with a per-peer credential bound to the `peerId`, verifiers in `<relay-data-dir>/peers.json`; **B23** puts TLS at the relay's front door, so the whole listener is `wss://` including loopback; and **B32** moves the path to **`/contract-b/v4`** and crosses the fleet in lockstep. Beside them: B24's published capacity table (published on the wire, logged at startup, and **on the archive's `/api/status` as `limits` since the archive-views window of 2026-08-12**), B25's optional `--min-contract-version` floor (left **unset** here, so `minContractVersion` is absent from that same page), **B26's forward receipt** — the relay emits one per forward and the sending sidecar records it durably against its journal entry, running on the deployment since 2026-08-12 — B27's `subscribe` grant for the archive, B28's authenticated admin path, B29's placement-under-churn rules and B30/B31's escaping and game-version rules, B30 also being the archive's operator-side render deny list (`--deny-list`, **unset** here). The archive itself now **streams** its ledger replay rather than holding it — 8.44 M records replay in ~148 s at a **1.53 GiB** peak, against ~7.86 GiB for the same work before — **gzips** its HTTP surface under standard `Accept-Encoding` negotiation (9.15× on `/api/status`, identity below a 1,400-byte floor), counts a `GENOME` line as the ledger record it is so `ledgerRecords` matches `wc -l` minus only the lines replay could not parse, and takes a **genome horizon** (`--genome-horizon` / `MULTIVERSE_GENOME_HORIZON`) that is **unset on this rig** — an absent `genomeHorizonMs` on the status page is how a reader knows nothing is being pruned. Under all of it, unchanged: the world-settings readout (§19), §18's pacing and speed readout, §17's two-way lane walks, `--inbound-rate` and the `/api/hops` feed, **§20's disk budget (B20)** — timer journal compaction, size-based log rotation and all-or-nothing appends in both append-only logs — and §21's genome-pump bounds (B21). It is what fills the status page's **Species** and **Settings** tabs and `ringstat --species` / `--settings`, and since the archive-views window of 2026-08-12 it also derives the **genealogy of everything alive** on its own `/api/species/tree` — kept off `/api/status` on purpose, because a derived tree must not be written into the durable sample file once a minute (§20 B20), and since 2026-08-10 it **measures** each world's achieved time scale beside the applied one (`achievedTimeScale`; *A world can be at the wrong time scale*, in Gotchas). Built from `go/` into `bin/` by `e2e/run-m4-lan.sh build` — **except against a live map**, where a running sidecar holds `bin/sidecar` open and an in-place build fails with `ETXTBSY`, so the build goes to a scratch directory on the same filesystem and is renamed over the binary instead. That is a crossing's P0.3 and P3 (`e2e/crossing/RUNBOOK.md`) and it is equally the shape of every rolling sidecar roll since (*The living deployment*) |
| Unity | 6000.0.44f1, **Mono** backend (not IL2CPP — Harmony and decompilation fully work) |
| BepInEx | 5.4.23.3 (win x64), installed in the game directory |
| .NET SDK | 8.0.423 in `~/.dotnet` (not on default PATH — scripts export it) |
| ilspycmd | 9.0.0.7889 (pinned; newer versions need a newer .NET) |
| Go | 1.26.5 (linux-amd64), installed without sudo from the official tarball into `~/go-dist/go`. `~/go` is a symlink to it, which is what makes the pre-existing `export GOROOT=$HOME/go` in `~/.bashrc` correct and puts `go` on the inherited `PATH` through `$HOME/go/bin`. `GOPATH` stays `~/gopath`. |
| Go dependencies | `github.com/coder/websocket` v1.8.15, the only non-stdlib import. It has no transitive dependencies and needs no cgo. |

## Workflow

```sh
bibites-mod/sync-game-refs.sh   # re-sync libs/ + decompiled/ after a game update (no-op if current)
bibites-mod/deploy.sh           # dotnet build + copy DLL to BepInEx/plugins

# game.sh drives one OR two instances; every command takes an instance name
# (default `main`, so the old single-instance forms still work).
bibites-mod/game.sh start A     # launch instance A, detached, and record its Windows PID
bibites-mod/game.sh log A 60    # last 60 BepInEx log lines of THAT instance
bibites-mod/game.sh logfile A   # which log file that instance owns
bibites-mod/game.sh pid A       # its recorded Windows PID
bibites-mod/game.sh status      # every recorded instance: pid, running/gone, log file
bibites-mod/game.sh wait A 60   # block until it exits; fail after 60 s
bibites-mod/game.sh stop A      # stop that instance only
bibites-mod/game.sh stop        # stop every recorded instance, then sweep orphans by name

# Headless: the sim runs, nothing renders. The flags are Unity's own; the game's parser
# reads only -steam. game.sh reads this itself, in WSL, so it is not a WSLENV variable.
BIBITES_EXTRA_ARGS='-batchmode -nographics' bibites-mod/game.sh start A

# Run the M1 exit test with no operator, then quit. WSLENV is required — see Gotchas.
WSLENV=MULTIVERSE_AUTOTEST MULTIVERSE_AUTOTEST=1 bibites-mod/game.sh start
```

Two rules make multi-instance control work, and both are load-bearing. `start` launches
through PowerShell `Start-Process -PassThru` and records the returned Windows PID under
`$BIBITES_RUN_DIR` (default `~/.cache/bibites-multiverse/instances`), so `stop <instance>`
kills exactly one game instead of every one by process name. And an instance is mapped to
its log **by content, never by start order**: `game.sh` greps both `LogOutput.log` and
`LogOutput.log.1` for that instance's own startup marker, because which process gets which
file is a lock race (see Gotchas).

**Headless is validated, 2026-08-09.** The M1 exit test ran to `RESULT: PASS` under
`-batchmode -nographics`: the procedural sprite atlas builds CPU-side, so the world still
seeds, saves and quits with nothing drawn and nobody watching. Windows reports
`MainWindowHandle=0` for that process and gives it **no `GPU Engine` counter instances at
all** — not idle ones, none — where a visual instance held ~24% of the GPU; CPU fell from
1.50 to 1.10 cores, and a headless instance sitting at the menu is ~390 MB. **A headless
instance holding a live world is barely larger** — the five local worlds measured 403–466 MB
each right after the 2026-08-10 switch, against the 1.27–2.07 GB the same five held drawn.
The far end takes the same two flags through `start-slot6.ps1 -Headless`, and the five local
worlds take them from `e2e/run-m4-lan.sh`, which exports `BIBITES_EXTRA_ARGS` for every game
it starts: **the five local worlds have run headless since 2026-08-10**, and slot 6 ran that
way from 2026-08-09 23:15Z to 2026-08-10 00:58Z, drawn from then until 2026-08-12 ~11:08Z, and
**headless again since** — the flip is proven in both directions, three flips deep, and belongs
to a start, not to the installation (*The living deployment*). The 08-12 flip used the clean
sequence: `quit` via the cmd file (the on-quit save rotates), `start-slot6.ps1 -GameOnly
-Headless` against the sidecar left running, then the time-scale re-send.
Exactly one thing is lost, and it is not the simulation —
see *A headless world's save thumbnail is blank* in Gotchas.

### The reference DLL set

`bibites-mod/sync-game-refs.sh` copies thirteen DLLs into `bibites-mod/libs/` — eleven from
the game's `The Bibites_Data/Managed`, and `0Harmony.dll` and `BepInEx.dll` from
`BepInEx/core`. It then regenerates `decompiled/BibitesAssembly/` with `ilspycmd`, and writes
`libs/.sync-stamp` with the Steam buildid and the `BibitesAssembly.dll` hash.

**`ShapesRuntime.dll` joined the set for M4** (`efd74a1`), and it is what the portal is drawn
with. `bibites-mod/BibitesMultiverse.csproj` references it like every other one, with
`Private="false"` so it is never copied into `plugins/`. The full Managed set is
`BibitesAssembly`, `Newtonsoft.Json`, `Newtonsoft.Json.UnityConverters`, **`ShapesRuntime`**,
`UnityEngine`, `UnityEngine.CoreModule`, `UnityEngine.InputLegacyModule`,
`UnityEngine.JSONSerializeModule`, `UnityEngine.Physics2DModule`, `UnityEngine.PhysicsModule`
and `UnityEngine.UI`.

**The freshness check only looks at `BibitesAssembly.dll`.** It compares that one file's
SHA-256 against `libs/` and reports the Steam buildid; when they match it prints
`up to date (buildid N)` and **copies nothing**. So a `libs/` that is missing or stale on
`ShapesRuntime.dll` alone is not detected — which is exactly the state every checkout was in
before M4. Run `sync-game-refs.sh --force` after any change to the DLL list, and whenever the
build fails to resolve a reference that the script claims to have copied.

The auto-test (`bibites-mod/src/AutoTest.cs`) drives the whole M1 exit test unattended: it
waits for the main menu, seeds or loads its own `M1-AutoTest` world via
`GameManager.StartGame(path)`, picks an adult organism, runs the round trip, watches it for
30 simulated seconds, saves the world, prints one `[M1-AUTOTEST] RESULT:` line and quits.
It touches no save other than `M1-AutoTest.zip`. The BepInEx config entry
`[M1] AutoTest = true` arms it the same way, without the environment variable.

Smoke test passed 2026-08-02: plugin `Bibites Multiverse 0.1.0` loads and logs
through the BepInEx chainloader. M1 exit test passed 2026-08-02, on two unattended runs —
see `m1_findings.md`, *Runtime results*. M2 exit test passed 2026-08-02/03 on the
two-instance rig, which `e2e/run-m2.sh` records — see `m2_considerations.md`,
*Exit Test → Result*.

The Go side — the relay, the sidecar and the archive — builds, tests and runs with no game
installed. Its contract suite drives both contracts with fake mod clients, so it is the fast
loop:

```sh
cd go
go build ./...                       # compile everything
go test ./...                        # the whole contract suite, ~11 s
go test -race ./...                  # same, with the race detector
gofmt -l . && go vet ./...           # both must print nothing

# Static binaries for the rig: relay, sidecar and archive.
# A running process holds its binary open, so stop a service before rebuilding
# over it — an in-place build against a live binary fails with ETXTBSY.
CGO_ENABLED=0 go build -o ../bin/ ./cmd/...

# The sidecar for the second computer. No cgo, so it cross-compiles from here.
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o ../bin/multiverse-sidecar.exe ./cmd/sidecar
```

`e2e/run-m3.sh` runs the whole M3 ring rig and its rehearsal with no operator, and every
phase is separately invokable on a healthy rig, so a failed phase re-runs without redoing
the seed or the bring-up:

```sh
e2e/run-m3.sh build      # Go binaries into bin/
e2e/run-m3.sh seed       # create (or evolve) M3-Slot1, M3-Slot2, M3-Slot3, then quit the games
e2e/run-m3.sh up         # relay → archive → sidecars 1,2,3 → games 1,2,3; waits for three open export edges
e2e/run-m3.sh phase1     # the ring forms: three slots in order, EDGE_STATUS open on every export edge
e2e/run-m3.sh phase2     # circumnavigation: one organism 1→2→3→1, byte-identical blob per hop
e2e/run-m3.sh phase3     # archive truth: three hops recorded, lineage joined, no parent blob on the wire
e2e/run-m3.sh phase4     # containment spot-check (D10): outside-the-square inward, and a far-north wrap
e2e/run-m3.sh phase5a    # kill -9 the relay mid-migration
e2e/run-m3.sh phase5b    # kill -9 a sidecar mid-migration (MULTIVERSE_FAULT=post-journal)
e2e/run-m3.sh journal    # summarise all three sidecar journals
e2e/run-m3.sh archive    # bin/archive list over the rig's archive data dir
e2e/run-m3.sh errors     # every unexplained error line in all three BepInEx logs
e2e/run-m3.sh down       # stop everything and leave no process behind
e2e/run-m3.sh all        # build, seed, up, phase1..5, errors, down
```

`e2e/run-m3-lan.sh` is the same milestone with **slot 2 on the second computer**. It sources
`run-m3.sh` with `M3_LIB=1` and reuses every helper — ports, token, game control, log waits,
`field`, seeding, teardown — and replaces only what the LAN changes: two local slots instead
of three, and every assertion about the third one read from somewhere the far end sends by
itself. That resolves `m3_considerations.md` Risk 6: **the far end is never driven.** Ring
state comes from the relay's `ring.json`, slot 2's liveness from the `EDGE_STATUS` ripple
into slot 1 (an export edge opens only when its east neighbour is live *and* mod-connected),
and every hop through slot 2 from the **archive**, which the relay feeds a copy of every
`MIGRATION_PAYLOAD` and every `MIGRATION_ACK`. No agent, no remote PowerShell, no file read
across the network.

```sh
e2e/run-m3-lan.sh lanhost   # the relay's LAN address + the two elevated owner commands
e2e/run-m3-lan.sh reserve   # pre-seed the ring: slot-1, slot-2, slot-3, in ring order
e2e/run-m3-lan.sh seed      # M3-Slot1 and M3-Slot3 only; the far end seeds M3-Slot2 itself
e2e/run-m3-lan.sh up        # relay (0.0.0.0) → archive → sidecars 1,3 → games 1,3
e2e/run-m3-lan.sh phase1    # ring form-up, including the remote slot 2
e2e/run-m3-lan.sh phase2    # forced export 1→2; the arrival is read from the ARCHIVE
e2e/run-m3-lan.sh phase3    # a NATURAL eastward crossing out of slot 2, into slot 3
e2e/run-m3-lan.sh phase4    # the return hop 3→1: the circuit closes
e2e/run-m3-lan.sh phase5    # archive truth: all three lanes, lineage, genomes held
e2e/run-m3-lan.sh phase6    # exactly-once; slot 2 is inferred, and the inference is printed
e2e/run-m3-lan.sh down      # stops THIS machine only; the far end is never touched
```

Three things about it are load-bearing:

- **Pre-seed the ring before the first `up`.** Slot order is start order on a fresh ring
  (`contract-b-m3.md` §7.2 rule 4), and the second computer is started by a person, so start
  order is not something to depend on. `reserve` runs `bin/relay --reserve-slot slot-1
  --reserve-slot slot-2 --reserve-slot slot-3`, which writes the three reservations and
  exits; rule 1 then hands each peer the slot its `peerId` already owns, in any join order.
  `up` does it for you when `ring.json` is not already in that shape.
- **Phase 3 waits for the ring's own traffic.** Nothing can force an export on an undriven
  far end. The measured crossing rate is ~20 eastward crossings per *simulated* hour, which
  is one every ~180 s of wall clock at 1x and every ~9 s at 20x, so the default
  `NATURAL_TIMEOUT` is 1800 s with progress every 30 s. The far end's operator can press
  `F10` in the game to force one by hand.
- **Slot 2 is inferred in the exactly-once count, and phase 6 says so.** Slots 1 and 3 are
  counted in their own worlds through `count <id>`; slot 2 cannot be, so its holding is
  derived from the last archive record that names the organism and printed with the
  reasoning beside it.

`e2e/run-m2.sh` is **historical**. It is the recorded shape of the M2 exit test — two
sectors, `contract-b/1`, `MULTIVERSE_SECTOR`, `--sector A`, `/contract-b/v1` — and none of
those reach the current binaries. Read it for its phase structure and its evidence, not as a
runnable rig; `run-m3.sh` is the one that runs.

### The M4 rigs — `run-m4.sh` and `run-m4-lan.sh`

`e2e/run-m4.sh` is the **local rehearsal**: a 3×2 map of six slots on this machine, five of
them real games and the sixth driven by `bin/fakemod` because BepInEx hands out five log
files (*The five-instance ceiling*, in Gotchas). Every phase is separately invokable on a
healthy rig.

```sh
e2e/run-m4.sh reserve    # pre-place the map: slot-1..slot-6 at (0,0)..(2,1)
e2e/run-m4.sh arm        # copy e2e/data/slot-1 — the T1 journal — into the M4 rig
e2e/run-m4.sh seed       # create (or evolve) M4-Slot1..M4-Slot5
e2e/run-m4.sh up         # relay → archive → sidecars 1..6 → games 1..5 + fakemod
e2e/run-m4.sh phase1     # the grid forms: 3×2, per-edge EDGE_STATUS, status page
e2e/run-m4.sh phase2     # two-axis migration out of slot 1: east to 2, north to 4
e2e/run-m4.sh phase3     # the resume test: the 2026-08-04 stranded organism delivers
e2e/run-m4.sh phase4     # kill slot 5 mid-column, then splice it back in
e2e/run-m4.sh phase5     # a NEW seventh peer splices into a live map
e2e/run-m4.sh phase6     # burst pacing: dark slot 2, accumulate, wake, assert the pacing
e2e/run-m4.sh phase7     # bounce-after-hold, against a short configured holdTimeoutMs
e2e/run-m4.sh phase8     # periodic saves: [M4-SAVE] on interval, rotation on disk
e2e/run-m4.sh phase9     # portals on screen, plus a flourish
e2e/run-m4.sh phase10    # error sweep, teardown, exactly-once census
e2e/run-m4.sh all        # build, reserve, arm, seed, up, phase1..10
```

`e2e/run-m4-lan.sh` is **the same map with slot 6 on the second computer**, and it is the
exit-test rig. It sources `run-m4.sh` with `M4_LIB=1` — which sources `run-m3.sh` with
`M3_LIB=1` — so it inherits the M4 topology, the map readers, the status-page accessors,
`hop`, the seeding and the teardown, and replaces only what the LAN changes.

```sh
e2e/run-m4-lan.sh lanhost    # the relay's LAN address + the elevated owner commands
e2e/run-m4-lan.sh reserve    # pre-place ALL SIX slots, including slot-6@2,1 for the far end
e2e/run-m4-lan.sh seed       # M4-Slot1..M4-Slot5 here; the far end seeds M4-Slot6 itself
e2e/run-m4-lan.sh up         # relay (0.0.0.0) → archive → sidecars 1..5 → games 1..5
e2e/run-m4-lan.sh phase1     # the grid forms across two machines; the far slot reports for duty
e2e/run-m4-lan.sh phase2     # forced hops INTO the far slot on BOTH axes, read from the archive
e2e/run-m4-lan.sh phase3     # the return current: NATURAL hops OUT of the far slot, both axes
e2e/run-m4-lan.sh phase4     # kill-and-heal on LOCAL slot 5; the healed east lane crosses the LAN
e2e/run-m4-lan.sh phase5     # burst pacing, dam on a LOCAL slot (run-m4.sh phase 6, unchanged)
e2e/run-m4-lan.sh phase5far  # the same test against the FAR slot — two owner commands, then observed
e2e/run-m4-lan.sh phase6     # periodic saves: five worlds on disk here, the sixth via HEARTBEAT.lastSave
e2e/run-m4-lan.sh phase7     # portals: the local evidence, and what only a person can say
e2e/run-m4-lan.sh phase8     # exactly-once, error sweep, teardown of THIS machine only
e2e/run-m4-lan.sh all        # up, phase1..8 (phase5far is excluded: it blocks on a person)
```

**The LAN rig passed the M4 exit test on 2026-08-06, and it is running now.** It came back
up after the test and holds the living deployment: six worlds, periodic saves every 2 minutes
here and every 10 on the far end, **all six headless** (the five local since 2026-08-10; slot 6
drawn 2026-08-10 → 2026-08-12 ~11:08Z and headless again since) **and the five local ones
targeting ×100 since 2026-08-10**. It has since survived two host reboots, on 2026-08-08 and
2026-08-09, and was brought back by hand both times; on **2026-08-11 it crossed to
`contract-b/4.0`** in a 7m18s window, which is the third planned outage it has been through and
the only one that changed the wire (*The living deployment → The crossing to `contract-b/4.0`*).
Later the same day it took **WP4 and WP5** by rolling restart with **no map outage** — a relay
restart plus five one-at-a-time sidecar restarts, no game touched (*The living deployment → The
WP4 + WP5 rollout*).
Do not start another rig against it, and
do not stop a process it owns — see *Only one rig can run at a time* in Gotchas.
`m4_considerations.md`, *Exit Test → Result*, records the exit-test run; **the current
reading, the reboot ritual and the open watch items are in *The living deployment* below.**

**Since the two-way-lane rollout of 2026-08-07 the map draws one lane per declared edge, not
twelve.** A slot that declared only two edges contributed two, so the status page read **22**
lanes while the far end was on the one-way build and **24** once it took the new one — which it
now has: measured **24 open, no bypass** on 2026-08-07 after the settings rollout. Count lanes
from `sum(len(slot.exportEdges))`, never against a constant — `run-m4-lan.sh phase1` was changed
to do exactly that. Both axes wrap, so every declared edge opens.

Six things about it are load-bearing:

- **The far slot is slot 6, at (2,1), and the choice is forced by the map.** It was already
  the rehearsal's synthetic slot, so every "read this from somewhere else" branch carries
  over unchanged. It keeps the **resume test** local (slot 1 → slot 2, and the T1 journal is
  the only copy of that input) and the **kill-and-heal test** local (a hard kill is a command
  to a process, and the far end takes no commands). It has every lane, because a 3×2 map is
  a torus — under D17 (2,1) exports east to slot 4, north to slot 3, west to slot 4 and south
  to slot 3, and receives on all four. (On a 3×2 map every column has length 2, so the north
  and south lanes of any slot name the **same** peer; that is A38's stated consequence, not a
  routing bug.) And killing slot 5 leaves row 1 as {4, 6}, so **phase 4's healed east lane runs
  across the network**, which is a stronger test than the rehearsal's, not a weaker one.
- **Five real games here plus one real game there is six real worlds and no synthetic peer.**
  This is the honest 3×2 map `m4_considerations.md`'s exit test asks for, in its "5+1 across
  the two machines" form. `bin/fakemod` is not started at all.
- **The far end is never driven** (D9, `m3_considerations.md` Risk 6, `m4_considerations.md`
  Risk 7). Slot 6's existence comes from the relay's map; its liveness from the EDGE_STATUS
  ripple; every hop through it from the archive; and its population, custody depth, paced
  depth, simulated time and last save from the status page. **Under M4 the ripple is sharper
  than M3's**: with the far end down, slot 5's east lane *bypasses* the hole to slot 4 and
  slot 3's north lane *closes* `no_peer` (column 2 is {3, 6}, and §2.1 has nothing to
  re-pair to); the instant it joins, slot 5's east lane moves to slot 6 and slot 3's north
  lane opens. Two independent local witnesses to a remote world's health.
- **It uses its own relay data dir**, `e2e/relay-data-m4-lan/`, and its own journals, logs
  and command-file directory. `run-m4.sh phase5` splices a seventh peer into
  `e2e/relay-data-m4/ring.json` and leaves that map **4×2**; a reservation is idempotent and
  never shrinks a map, so sharing it would start the LAN rig on a map that fails its own
  first assertion.
- **`phase5far` is the only phase that needs a person**, and it needs exactly two commands on
  the second computer: `.\stop-slot6.ps1 -GameOnly` and `.\start-slot6.ps1 -GameOnly`. The
  `-GameOnly` switches exist for this: the world goes away while **the sidecar keeps running**,
  so the burst accumulates in the far journal and drains, paced, when the world returns. The
  rig sends nothing — it prints the command and polls `modConnected` on the status page. The
  automatable `phase5` proves the same rule with a local dam, so `phase5far` is confirmation
  and never the only evidence.
- **Three of the rehearsal's phases are not re-run, and the script says why.** The resume
  test is local by construction. The seventh-peer splice needs a mod-less sidecar on this
  machine and the relay is the only arbiter. The bounded hold (`run-m4.sh phase7`) needs the
  destination killed in the window between the forward and the acknowledgement, which is two
  commands to the destination — and the destination is now a computer this rig will not
  command.

**The M3 scripts still speak a wire the binaries retired.** M4 bumped both majors — to
`contract-a/2` and `contract-b/3` — and moved both paths; **M5 bumped Contract B's again on
2026-08-11, to `contract-b/4`, and moved that path to `/contract-b/v4`** (§22, B32), so the M3
scripts are now two Contract B majors behind and `run-m2.sh` is three. A relay and a sidecar
still *serve* the old paths and then close the connection with an explanation, so a stale script
does not fail with a socket error — it connects, gets closed, and looks like a peer that
will not join. That is the failure to expect, and it is why this list exists. **Since the
crossing there is a second failure to expect on top of it**, and it comes first: the relay's
whole listener is TLS, so a client dialling `ws://` on any of these paths does not reach the
close code at all — a plain HTTP `GET` to `127.0.0.1:8795` answers `400 Client sent an HTTP
request to an HTTPS server`.

**The wire is now `contract-a/2.4` and `contract-b/4.0`, since 2026-08-11** — the public-release
sets, `contract-a.md` §21 (A47–A52) and `contract-b-m4.md` §22 (B22–B32). **The minors before it
were `contract-a/2.3` and `contract-b/3.5`** (the world-settings set, 2026-08-07;
`2.2`/`3.4` and `2.2`/`3.3` were the two-way lanes and the pacing readout, `2.2`/`3.2` the species
census, `2.1`/`3.1` the species identity, all of the same day), and **no minor ever moved anything in
this table**. The `contract-b/4.0` **major** did: the first row's path is now `/contract-b/v4`,
and the scheme is `wss://`. Contract A did **not** move for D17 — §18's A41 says so explicitly: every field the
two-way set touches already existed and already accepted all four edge values, so there was
nothing additive to bump. It **did** move for §19, because A42 adds five genuinely new
`CONFIG_UPDATE` fields. Compatibility is on the major alone: a minor is never a rejection reason,
none of those minors moved a URL path, and every field they added — the migration `species` block, the
`HEARTBEAT` census with its `truncated` sibling, `SECTOR_GRANT.neighbours`' `"W"` and `"S"` keys,
and §19's five settings — is OPTIONAL on both wires. **The minor is a capability statement, not a
negotiation:** detect a feature by the presence of its field, never by arithmetic on the minor.
**The corollary is what the 2026-08-11 window was spent on**: a *major* is exactly the rejection
reason a minor is not, so `contract-b/4.0` moved the path, and every peer on this map had to cross
in one window rather than at leisure (§22, B32). Nothing straddles a major. A
world whose mod predates the census therefore reads **species unknown** on the status page, and
one whose mod predates §19 reads **`?`** in every settings cell — honest, and never a wrong
number, and never the value the game ships with. Measured on the live map on 2026-08-07 with the
five local worlds on `0.6.1` and slot 6 still behind: the Settings tab printed `0.6.1` /
`contract-a/2.3` / `every 2 min` / `4` / `yes` / `on` / `Basic bibite` for slots 1–5 and
`?` / `?` / `?` / `?` / `?` / `?` / `this world has not told us` for slot 6. **Slot 6 no longer
reads `?`, and that example is the old state.** Re-read on the live map on 2026-08-09 it
publishes a full §19 block — `0.6.1` / `contract-a/2.3` / `every 10 min` / `6` / `yes` / `on` /
`Basic bibite` — so the far end has applied a bundle at least as new as the settings build and
no cell on the Settings tab is unknown any more. **A settings row is
validated but never fatal** — §19 makes a malformed row something the sidecar strips before the
handshake proceeds, because closing the handshake over an observability field would cost the
whole session.

**But `contract-b/3.3` is the first minor a stale peer does not simply tolerate, and that is the
correction to make here.** The two previous rollouts left slot 6 on the previous minor and it
kept exchanging organisms in both directions with no `4000` close, which is where the paragraph
above came from. `3.3` breaks that streak on one point: a pre-`3.3` **sidecar** validates
`MIGRATION_PAYLOAD.exitEdge` against `E`/`N` and answers anything else with a **permanent**
`MALFORMED_MESSAGE` NACK — `exitEdge "W" is not E/N; the grid exports east and north`. So an
upgraded neighbour's west and south exports to that peer are refused, bounced home by §9.4, and
re-exported, which pins the exporter's paced queue at `inboundQueueMax` and makes it answer its
own senders with `OVERLOADED`. Measured on the live map on 2026-08-07 with slot 6 stale: the two
lanes into it ran at ~40 hops/min against ~4–6 on every other lane, and slots 3 and 4 sat at
`pacedDepth` 63–65 while every other slot drained to 0. Nothing is lost — a bounce-back is a
delivery — but a `3.3` map is **not** operationally complete until every peer is on `3.3`. The
minor is still not a rejection reason; the incompatibility is in one field's value range, and it
is why the far-end bundle had to be re-taken with that build rather than at leisure.

**That one cleared, and the difference between the two kinds of staleness is the lesson.** The
far end took the `3.3` bundle, and on 2026-08-07 after the `3.5` rollout the live map read 24
open lanes, no bypass, and `pacedDepth` 0 on all six slots — the ~40 hops/min bounce loop is
gone. What was left of slot 6's lag was **readout only** — its settings cells printed `?` while
its census kept reporting — and on 2026-08-09 that cleared too: it now publishes a full §19
settings block. A stale *value range* in a field both sides already exchange breaks traffic; a
stale *absent optional field* only ever costs a number on a page. Re-take the bundle promptly
for the first kind, at leisure for the second, and do not confuse them.

| What the scripts speak | What M4 speaks | Where |
|---|---|---|
| `/contract-b/v2` | **`wss://…/contract-b/v4`**, since 2026-08-11 (it was `ws://…/contract-b/v3` from M4 until then) | `e2e/run-m3.sh` (`RELAY_URL`), and it is **sourced as a library** by `run-m3-lan.sh` and `baseline.sh`, so this one line reaches all three. Also `e2e/run-m3-lan.sh` (the operator note). `e2e/run-m2.sh` still says `/contract-b/v1`, three majors back. **`farend/setup-farend.ps1` is done** — it builds `wss://<host>:8795/contract-b/v4` and takes the CA to verify it. The M3 scripts are **deliberately not** patched: they speak retired wires, they are the record of runs that happened, and they keep their own `ensure_token` plumbing because it is what they need. |
| `/contract-a/v1` | **`/contract-a/v2`** | **Nothing to change.** No script hardcodes a Contract A path — the sidecar's `--listen` and the mod's `MULTIVERSE_SIDECAR_PORT` carry it, and both sides already agree. |
| `MULTIVERSE_EXPORT_EDGE=E` | **`MULTIVERSE_EXPORT_EDGES=E,N,W,S`** | `e2e/run-m3.sh`. The old name still parses, so this is not a break — it just cannot produce anything but a line topology. **`farend/setup-farend.ps1` is done**: its generated `start-slot6.ps1` sets the plural name, its `-ExportEdges` default is all four, and it rejects a token that is not an edge or an edge repeated. It no longer rejects an edge declared with its opposite — that refusal rested on the one-way lane and D17 retired it (§18 A38). |
| the relay's `ring.json` key `"ring"` | **`"slots"`**, with `width`, `height`, `col`, `row` | `e2e/run-m3-lan.sh` (`ring_order()`) and `e2e/baseline.sh`. **This is the silent one.** `"ring"` is a read-only migration path: an M4 relay reads an M3 file once and never writes that key again. Both readers work today against the committed M3 `ring.json` and start returning empty the first time an M4 relay saves. |
| nothing | the archive's **`--http`** flag, and **`ringstat`** | No **M3** script mentions either, so the archive binds its compiled default — `127.0.0.1:8796` since the M4 port plan, `127.0.0.1:8791` before it. The M4 rigs do pass `--http`, from `ARCHIVE_HTTP` (*Owner steps*). `baseline.sh` reads the archive only through the filesystem, so the whole M4 observability surface — map shape, effective lanes, bypasses, custody/paced/held depths, hold-timeout bounces — is absent from the baseline capture. |
| relay `8790`, archive `8791` | relay **`8795`**, archive **`8796`** | The Go defaults already moved (`contractb.DefaultRelayPort`, the archive's `--http`, `ringstat`'s `--url`), and so has `$RelayPort` in `farend/setup-farend.ps1`. Still to change: `RELAY_PORT` in `e2e/run-m3.sh`, and **the firewall rule and the portproxy, which are owner steps** (below). See *The M4 port plan* — the old defaults are slot 4's and slot 5's Contract A ports on a six-slot rig. |
| `grep '[M2-CROSSING]' \| tail -n 1` | a filter on **`edge=`** | `e2e/baseline.sh`, in both `last_crossing()` and the population trend. **Fixed 2026-08-05** in the M4 pre-flight: both now key on `edge=`. See *Reading the logs* below — one window emits one line **per export edge**. |

**`--reserve-slot` did not change, and a report saying otherwise is wrong.** The relay's flag
is `--reserve-slot <peerId>[@<col>,<row>]`, and the coordinate suffix is **optional** — a bare
`--reserve-slot slot-1` is valid M4 syntax and lets the §7.2 placement rules choose. There is
no `--position` on the relay. `--position <col>,<row>` is a **sidecar** flag, beside
`--insert-after-slot` and `--insert-axis`. So `run-m3-lan.sh reserve` still parses; it simply
no longer pins a coordinate, which for a three-slot line is the same outcome.

Environment variables are what make an instance scriptable, and `run-m3.sh` sets them all.
Name every one of them in `WSLENV` or WSL forwards none (see Gotchas).

| Variable | Meaning |
|---|---|
| `MULTIVERSE_EXPORT_EDGES` | **M4.** The edges this sim exports through, as a list — **`E,N,W,S` under the grid** (`contract-a.md` §15 A18, §18 A38). Separators are comma, semicolon, space or tab; each token is trimmed and upper-cased. **Every declared edge is both an export edge and an entry edge**, so `borderEdges` equals this set rather than being a superset of it, and there is no passive edge any more. **Unset now means all four**, not "off" (§18 A41 case 3); the literal `none`, or a value naming no edge at all, is what turns the client off. Set it only to declare a **subset** — or, as the M4 rigs do, to state the geometry explicitly so a change to the default cannot move what a rig measures. |
| `MULTIVERSE_MIGRATION_EXCLUDE` | **The two-way-lane set (D18, §18 A39).** Comma-separated full species names that **never export**, matched on the §16 A34-normalized form. Default `"Basic bibite"` — the seed stock stays home, so the organisms that travel are the ones evolution produced. Set to an **empty value** to disable the policy. It is **local and export-side only**: never on the wire, never a census filter, and never a refusal of an arrival. An excluded organism is simply not a capture candidate on any edge, so D10's wrap returns it — which is why the square-crossing series rises with this policy on rather than falling. One `[M4-D18]` line per distinct organism, capped at 4096 names per session. |
| `MULTIVERSE_EXPORT_EDGE` | **The M3 singular name, still read, and it takes a list too** — it is the same parser behind a different name. Kept so an M3 script keeps working. |
| `MULTIVERSE_OPEN_EDGE` | M2's name for the same thing. Also still read. |
| `MULTIVERSE_RING_SLOT` | The advisory ring slot, an integer `≥ 1`. It is a **label**, not a routing input: the sidecar closes `4001` when it disagrees with the slot the relay granted, which turns a mis-wired rig into one second of diagnosis. **Replaces `MULTIVERSE_SECTOR`, which no longer reaches the mod at all** — D8 retired the `{x, y}` grid (`contract-a.md` §14 A14). |
| `MULTIVERSE_SIDECAR_PORT` | The loopback Contract A port this instance dials. Default `8787`. One per instance; this is what separates six games on one machine. |
| `MULTIVERSE_WORLD` | See below. Under M4 it also names the **save file**, and therefore the whole rotation set. |
| `MULTIVERSE_CMD_FILE` | See below. |
| `MULTIVERSE_BORDER_WIDTH` | `W` in world units, which is also the inner boundary of the capture band. `0`/unset derives `0.02·S`, floor 20 u. |
| `MULTIVERSE_SAVE_MINUTES` | **M4.** Periodic-save interval in **wall-clock** minutes. Default `10`. `0` turns the timer off; save-on-quit is unaffected. **The name says minutes and the timer is `Time.realtimeSinceStartup`** (`WorldSaver.cs`), which is what D14 asked for — "a wall-clock timer drives `SaveSystem.CreateSave`" — so a world at 20× saves every 10 real minutes, not every 10 simulated ones. An earlier version of this row said *simulated*; it was wrong, and the difference is a factor of the time scale. |
| `MULTIVERSE_SAVE_KEEP` | **M4.** How many timestamped backups of *this world* survive a prune. Default `6`. `0` keeps none. |
| `MULTIVERSE_SAVE_ON_QUIT` | **M4.** Save once on `OnApplicationQuit`. Default `true`. |
| `MULTIVERSE_PORTAL` | **M4.** Draw the portal strips. Default `true`. `false` never creates the component, so no strip of either kind appears. |
| `MULTIVERSE_PORTAL_FLOURISHES` | **M4.** Draw the arrival and departure rings. Default `true`. It does **not** gate the strips. |
| `MULTIVERSE_FAMILY_REPORT` | Seconds between `[M2-FAMILY]` reports. Default `0`, off. |
| `MULTIVERSE_MIN_FPS` | **0.6.2.** What to do about the game's minimum-FPS servo. `auto` (the default, and what unset means) disarms it when the process has no graphics device and leaves it alone otherwise; `off` never touches it; `0` disarms it even in a drawn instance. It is the one knob here with **no BepInEx config entry**, deliberately — a new `Bind` makes all five instances rewrite `dev.multiverse.bibites.cfg` at once and two of them lose that race (*The FIRST bring-up after a deploy…*, in Gotchas), and this knob was not worth that. It never writes `UserSettings.minimumFPS`, so the owner's own setting is untouched. See *A world can be at the wrong time scale*. |
| `MULTIVERSE_AUTOTEST` | Arms the M1 auto-test. It **ORs** with the `[M1] AutoTest` config entry rather than overriding it. |

**Three names reach the same field, and the first non-empty one wins outright:**
`MULTIVERSE_EXPORT_EDGES`, then `MULTIVERSE_EXPORT_EDGE`, then `MULTIVERSE_OPEN_EDGE`, then
the BepInEx entry `[M4] ExportEdges`. **They do not merge.** Set the plural one and the other
two are ignored — and note that the mod's `[M2] config sources:` line still echoes all three,
which reads as if the others applied. They did not.

**A rejected edge set disables the client silently, and that is the one trap here.** An
unparseable token or a duplicate edge clears the set and leaves the client off, and each logs
an error. **`E,W` is no longer one of them** — an edge with its opposite was a contradiction
only while an edge was a capture band or a passive entry and never both, and §18's A38 retired
that; all four together is now the *conformant* declaration. A value that is only separators —
`MULTIVERSE_EXPORT_EDGES=","` — clears it with **no** error, and the only symptom is the
generic "no export edge is configured" line. Check that line before chasing a sidecar that
never sees a mod. Note that an **unset** variable is no longer this failure: it is the
all-four default.

Two knobs have **no** environment variable and are BepInEx config entries only:
`[M4] PortalSortingOrder` (default `-50`) and `[M4] PortalEntryWidth` (default `0`, which
derives the entry strip's depth from `W` plus the entry margin).

- **`MULTIVERSE_WORLD=<save name>`** loads that world from the main menu through
  `GameManager.StartGame(path)`, and **seeds it first if it does not exist** — apply the
  stock scenario, run until the world holds living bibites, save. `WorldSeeder.cs` owns
  that path and both callers share it: `AutoTest` for M1, `WorldLoader` for every rig
  since. It is what brings `M3-Slot1`, `M3-Slot2` and `M3-Slot3` up on a clean profile.
- **`MULTIVERSE_CMD_FILE=<Windows path>`** is the dev-command channel (`DevCommands.cs`).
  There is **no default** — unset it and only the `F10` hotkey works. The mod polls that file
  every 0.2 s and runs **one command per file**, never one per line: it reads the whole file,
  runs the first three whitespace-separated fields as `<token> <verb> [argument]`, and
  **deletes the file before running the command**. Write it **atomically** (temp file, then
  rename) — content that does not end in a newline is read as a partial write and left for
  the next poll. Every finished command appends `<token> OK|ERROR <details>` to
  `<cmdfile>.log`, which is what a script waits on. Keep the file on the Windows side of the
  boundary (`$env:TEMP`); a WSL path is not reliably reachable from a Windows process.

  | Verb | What it does |
  |---|---|
  | `export <family\|any\|id> [E\|N\|W\|S]` | Force one migration. It only teleports the organism into the border strip with an outward velocity — the ordinary `MIGRATE_OUT` path and every guard on it still runs, so the test bypasses no part of the pipeline. `family` prefers an organism with living parents, which is what makes the lineage annex non-empty. **M4 added the trailing edge**; omit it and the first declared export edge in canonical order is used. A letter that is not a declared export edge is refused, and the error names the declared set. |
  | `place <family\|any\|id> <x> <y> [vx] [vy]` | **M3.** Put one organism at an exact world position with an exact velocity, then report the capture-band verdict. This is how the two D10 containment questions are asked: an organism outside the square travelling **inward** (must not export), and one past the wrap radius (the wrap must return it). It clears the migration cooldown and the entry-immunity window first, so the decision under test is not swallowed by a leftover guard, and it drops the containment tracker's last position so the teleport is not misread as a wrap. Nothing in the pipeline is bypassed. |
  | `edge <open\|closed\|close> [E\|N\|W\|S\|all]` | **M3, extended in M4.** Force an export edge open or closed locally, without touching the sidecar or the relay. **The target is new**: omit it, or pass `all`, and every declared export edge moves together — so the bare M3 form `edge open` still works and now means "all". A letter that is not a declared export edge is refused. |
  | `camera <orthographicSize> [x] [y]` | **M4.** Set the camera zoom, and move it when both coordinates are given. It drives `CameraManager.SetCamSize` and then fires the zoom-change events, so anything that reacts to a manual zoom reacts to this. It reports the resulting `orthographicSize`, `position` and `cullingMask`. **This is what makes the WP7 zoom-legibility sweep scriptable** — 5, 250, 2 000 and 4 000 in four commands. |
  | `flourish <export\|entry> [x] [y]` | **M4.** Fire one portal flourish ring by hand, without waiting for a migration. Anything other than `entry` in the first argument selects `export`, including a missing argument. Both coordinates are read only when both are present; otherwise the ring fires at the origin. |
  | `census`, `count <id>`, `family` | Population and lineage readouts. `count <id>` is the exactly-once check. |
  | `save [name]`, `timescale <x>`, `autosave <on\|off>`, `quit` | World control. `save` goes through `WorldSeeder.SaveWorld`, which writes and then verifies the zip holds `scene.bb8scene` — but it **overwrites `<world>.zip` in place**, and `SaveSystem.CreateSave` deletes the old file before it writes the new one. No partial, no rotation, no prune, and nothing kept of what it replaced. Only the periodic save and the save on quit rotate (*The save-file rotation layout*). |

### Reading the logs — the grepable series

Every mod log line carries a bracketed series tag, and the tag is the whole filter. **The Go
side carries none** — the relay, the sidecar, the archive and `ringstat` all log structured
`slog` key/value records, so grep them by key instead.

| Tag | What it reports |
|---|---|
| `[M2]` | The general mod series: config, connection, edge state, exports. **The world-settings set (§19 A42) has no tag of its own** — it rides the `CONFIG_UPDATE reason=connect` line, which now ends `worldWrapping=… migrationExclude=… saveMinutes=… saveKeep=… saveOnQuit=…`. That line is the one to grep to see what a world published about itself. |
| `[M2-CROSSING]` | Per-simulated-minute crossing counters, **one line per export edge** — four of them under D17. |
| `[M2-CMD]` | Every command-file result, the same text as `<cmdfile>.log`. |
| `[M2-FAMILY]` | Family scans, on `MULTIVERSE_FAMILY_REPORT`. |
| `[M2-WORLD]` | World load and seed. |
| `[M3-D10]` | Containment: `event=WRAP`, `event=PLACED`, and each capture verdict. |
| `[M3-LINEAGE]` | The parent-blob collector. |
| `[M4-SAVE]` | **M4.** `event=SAVED`, `event=FAILED`, `event=BUDGET_EXCEEDED`, plus the armed line and each deferral. |
| `[M4-CROWDING]` | **M4.** Per-simulated-minute arrival crowding, **one line per entry edge**. |
| `[M4-PORTAL]` | **M4.** `event=BUILT`, `event=SHOWN`, `event=HIDDEN`, `event=RELAYOUT`. Under D17 every edge draws **two** lanes — an outer cyan capture lane and an inner amber arrival lane — so a healthy world shows eight `event=SHOWN` lines, `edge=E\|N\|W\|S` × `lane=capture\|arrival`. |
| `[M4-D18]` | **The two-way-lane set (D18, `contract-a.md` §18 A39).** The migration exclusion list. One `config:` line at startup naming the list, one `event=EXCLUDED` line per **distinct** organism whose capture the policy stopped (capped at 4096 per session, then one summary), and a `session summary:` line. These are **not errors and not a defect**: an excluded organism stays in its world, D10's wrap contains it, and §17's census still reports it. The reading to expect on the default list is a steady trickle of `species="Basic bibite"` on every slot, and **zero** `Basic bibite` in `/api/hops`. |
| `[M5-SPECIES]` | **The species-identity set** (`contract-a/2.1`). One `action=created\|matched\|fallback` line per import, with `migrationId`, `entityId`, `name="…"`, `localId`, `parentLinked` and `detail`; plus the export-side warning when a species name cannot go on the wire. `action=fallback` is the absent-block rule, and it is the normal reading for an arrival from a peer that predates the set. |
| `[M5-CENSUS]` | **The species-census set** (`contract-a/2.2`). **Silent in normal operation** — the census rides every `HEARTBEAT` and logs nothing. The one line it can emit is a rate-limited warning (at most one a minute) naming active species whose name half is empty or over 64 UTF-8 bytes and therefore cannot go on the wire; the census carries the rest with `truncated=true`. A quiet `[M5-CENSUS]` is the expected state, and a sidecar line reading `stripped part of a HEARTBEAT species census` with a quiet mod log means the strip happened for a reason the mod did not see coming. |
| `[M1-AUTOTEST]` | The M1 auto-test, including its one `RESULT:` line. |

**`[M2-CROSSING]` is now several lines per window, and every parser must key on `edge=`.**
The field itself is not new — the M3 line already carried `edge=`, with the same field names
in the same order — but M3 declared one export edge, so one window produced one line. Under
the grid a window produces **one line per declared export edge**, and each line's counters are
**that edge's**, not the world's. There is no cross-edge total on the line at all. Two
consequences, and both have already bitten `e2e/baseline.sh`:

- `grep '[M2-CROSSING]' | tail -n 1` no longer returns "the" sample. It returns whichever
  edge happened to be emitted last, and reports it as the world's figure.
- A population trend built by `sed`-ing `population=` out of every line now yields *N*
  identical samples per simulated minute, so a fixed-length tail covers `1/N` of the time it
  claims and the sample count is inflated *N*-fold.

Filter to one edge for a world-level series, or sum across edges deliberately. The full
field order is `edge simMinute stripEntries crossings totalStripEntries totalCrossings
cumulativeStripEntriesPerSimMin population S W simTick`. Note that `total*` is
**session-cumulative**, so a rate compared across a restart has to be built from the
per-window `stripEntries`/`crossings` fields and never from a difference of the totals.

**The `crossings` series went UP with D17, not down, and that is worth writing here because
`contract-a.md` §4.3.1 predicts the opposite.** The prediction — "the square-crossing series
should fall sharply and not reach zero" — is sound for A38 read alone: a band on every edge
captures organisms that used to walk out through `W` and `S`. Measured on the live map on
2026-08-07, over matched 60-simulated-minute windows either side of the rollout, the `E`+`N`
rate on the three slots not distorted by a stale peer went from 0.52/0.48/0.47 per simulated
minute to 1.07/0.90/0.50, and the four-edge total is roughly double the old two-edge one. Two
causes, and neither is a defect:

- **A39 is the larger one.** D18 puts `Basic bibite` — 30–56% of every world's population — on
  the exclusion list, and an excluded organism is never a capture candidate on any edge. It
  therefore walks past the square edge and is counted as a crossing where it used to be counted
  as an export. §4.3.1 lists the exclusion list first among the four residue cases; it just did
  not anticipate the residue being half the population.
- **A stale `contract-b` peer inflates the two lanes pointing at it** (see *The minors*), because
  every refused export bounces home moving outward and crosses on the way. That part clears when
  the far end takes the current bundle.

So read `crossings` against the exclusion list before reading it as a containment failure.

`[M4-CROWDING]` mirrors it on the **entry** side, one line per entry edge per simulated
minute, and carries `inQuarter`, `quarterShare`, `arrivals`, `arrivalsPerSimMin` and an
eight-bucket `arrivalHistogram=[…]` along the edge. It is what verifies the arrival pacing.

### The save-file rotation layout

M4 gave the mod its own periodic save (`WorldSaver.cs`), and it writes **three kinds of file**
into the game's `Savefiles/` directory — the same directory the game itself uses, which is
`Application.persistentDataPath/Savefiles`. `<world>` is `MULTIVERSE_WORLD`, falling back to
the loaded game's name.

| File | What it is |
|---|---|
| `<world>.zip` | **The live save.** The name is stable, so it always points at the newest good save. |
| `<world>-<yyyyMMddTHHmmssZ>.zip` | A rotated backup. UTC, second resolution, fixed width — so an **ordinal name sort is an age sort**. `MULTIVERSE_SAVE_KEEP` (default 6) of them survive a prune, and only this world's are ever pruned. |
| `<world>.partial.zip` | **Transient.** The save is written here first, verified as a zip containing `scene.bb8scene`, and only then rotated into place. A failure at any step deletes it and **leaves the previous live save untouched**. |
| `.multiverse-save.lock` | The cross-instance save lock. Broken automatically when older than 180 s. |

The order is write → verify → rotate → prune, and rotate is **two renames**: the old
`<world>.zip` becomes the timestamped backup, then the partial becomes `<world>.zip`.

**What this means for anything that scans the directory** — `worldstat`, a recovery
procedure, a baseline capture:

- **`worldstat` itself is safe by construction.** It takes exactly one `.zip` argument and
  never scans a directory. The hazard is entirely in the caller. `e2e/baseline.sh` does it
  correctly: it names `<world>.zip` explicitly and copies the zip out before reading it.
- **A `*.zip` glob now picks up files that are not worlds you want.** Exclude
  `*.partial.zip`, and match a backup with `-\d{8}T\d{6}Z\.zip$`. The game's own `quick.zip`
  and its `Autosaves/` subdirectory are also in there.
- **Between the two renames, `<world>.zip` does not exist at all.** A tool that assumes the
  live name is always present fails with "file not found" for the width of two filesystem
  metadata operations. **Retry — do not treat it as a missing world.**
- **A `<world>.partial.zip` found at rest means a run died mid-save.** It is safe to delete,
  and the mod deletes it itself the next time it arms.
- Recovery reads the live name for the newest good save, and the highest-sorting backup name
  for the one before it.

A periodic save is deferred by 15 s, up to six times, while a `MIGRATE_IN` is queued or while
another instance holds the lock; after six it saves anyway and warns. The save on quit ignores
the lock. Every outcome is one `[M4-SAVE]` line with a `stallMs` measured against the 2 000 ms
budget of Risk 3. **The living deployment breaches that budget routinely and has done since
the exit test** — see *Watch items*; the exit test's 241–538 ms is a 2026-08-06 reading of a
world that was **seconds old**, and the answer to why it no longer reads that way is in the same
place. Since `0.6.3` the same line also carries `verifyMs` and the four `SavePhases` fields, so
the stall is no longer one number.

### Credentials, TLS, and the retired LAN token

**Since the crossing of 2026-08-11 there are three secrets on this rig and none of them is
shared.** `contract-b-m4.md` §22 (B22, B23) replaced M4's one LAN token with a **per-peer
credential bound to the `peerId`**, and put **TLS at the relay's front door**. `contract-a.md`
§21 (A47) added a third, local, secret on the mod ↔ sidecar hop, which had none at all.

**A credential is a join string, and it is born at the relay.** `--reserve-slot <peerId>[@c,r]`
mints one for a peer that has none and `--mint-credential <peerId> --grant <g>` mints one for an
identity that holds no slot; either prints the join string **once**, on stdout. The relay keeps
only a **verifier** — a salted SHA-256 — in `<data-dir>/peers.json`, so it cannot print it again.
The three grants are **disjoint** (B27): a `subscribe` credential cannot claim a slot and a
`peer` credential cannot subscribe; `admin` is the release/handover/eviction path (B28). On this
map that is seven records: six `peer` and the archive's one `subscribe`, which is what
`credentials=7` on the relay's startup line counts.

**Losing a secret costs an identity, not a password.** The only recovery is
`--handover-slot <n>=<newPeerId>`, which mints for a **new** `peerId` and drops the old — and for
slot 6 that means an errand on somebody else's machine, because the second computer has persisted
the old id. So the secrets are captured into files, never into a scrollback:
`e2e/crossing/mint-credentials.sh` is the only place this project mints, and `run-m4.sh reserve`
delegates to it.

| Secret | Where | Who reads it |
|---|---|---|
| Five local peer credentials | `~/.multiverse/peer-slot-1.secret` … `peer-slot-5.secret`, mode `600` | each sidecar, as `--credential-file` (or `MULTIVERSE_PEER_SECRET`) |
| The archive's subscribe credential | `~/.multiverse/archive-main.secret`, mode `600` | the archive, as `--credential-file` |
| Slot 6's credential | **the second computer.** The handoff copy is `~/.multiverse/slot-6-handoff.secret` and is **deleted once the far end has applied it** — this machine has no use for another machine's identity | `setup-farend.ps1 -PeerSecretFile`, which stores it under `%LOCALAPPDATA%` with a user-only ACL |
| Contract A's token, per slot | `<data-dir>/contract-a.token`, mode `600`, **minted by the sidecar itself** at first start | that slot's game, through `MULTIVERSE_CONTRACT_A_TOKEN_FILE` |

**There is still no flag that takes a secret literally**, on either contract — the same rule the
shared token lived by, and for the same reason: a literal flag puts the secret in every process
listing. `--credential-file` carries the **secret half only**; the `peerId` half is `--peer-id`
and `<data-dir>/peer-id`, and the relay refuses a connection whose handshake claims a different
one. That binding is the whole of B22: a `peerId` used to be a claim and is now a credential.
The relay answers `401` on a mismatch, and a client that collects `authFailuresBeforeCeiling`
(**5**) consecutive 401s pins its backoff at the ceiling — so a wrong secret still looks like a
peer that never joins rather than a reconnect storm.

**TLS is the other half of the same amendment, and splitting them is what B23 forbids.** The
relay wraps its *whole* listener in TLS, so no plaintext path survives even on loopback: the five
local sidecars and the archive dial `wss://127.0.0.1:8795/contract-b/v4`, and a plain HTTP `GET`
to that port answers `400 Client sent an HTTP request to an HTTPS server` (measured 2026-08-11 —
B23's `426` is a different rule for a deployment shape this rig does not have).

- **The material lives in `e2e/tls-m4-lan/`, which is gitignored**, and is minted by
  `e2e/crossing/mint-tls.sh`. The certificate carries four SANs and each is load-bearing:
  `127.0.0.1` and `localhost` for the local clients, **`192.168.1.227`** because the far end's
  TLS client verifies the name it *dialled* — the Windows LAN address, not the WSL address behind
  the portproxy — and `172.24.110.174` so a debugging dial straight at the VM does not fail on the
  name.
- **`ca.key` must never be lost.** A new CA means a fresh trust import on the second computer,
  which is an errand D9 forbids this machine to run. **The leaf expires 2027-09-12**;
  `mint-tls.sh --renew-leaf` mints a new server certificate and **keeps** the CA, so a renewal
  costs the far end nothing.
- **Trusting the CA here needs no `sudo`.** A Contract B client verifies against its platform
  trust store and there is deliberately no flag between it and one, so `run-m4.sh` exports
  `SSL_CERT_FILE=$TLS_CA` — Go's `crypto/x509` and `curl` both read it, and nothing in `/etc`
  changes. Without it a sidecar logs *the relay's TLS certificate did not verify; NOT CONNECTING*
  and never dials.

**Contract A's token is local, per-slot, and nobody mints it by hand** (A47). Each sidecar writes
`<data-dir>/contract-a.token` at `0600` on first start, and `start_game` hands that path to the
game in `MULTIVERSE_CONTRACT_A_TOKEN_FILE` — the one variable in `WSLENV` carrying the **`/p`**
flag, because the mod is a Windows process and the value is a path: it arrives as
`\\wsl.localhost\Ubuntu\mnt\wsl\data\…\slot-N\contract-a.token`. A `/p` on the wrong variable
would corrupt it, which is why the flag is on exactly one name. **Nothing anywhere writes the
token's value — only its path.** A slot whose variable did not cross the boundary logs
`<unset>`, dials bare, is refused `401` and reconnects on the ordinary ladder; the fix is to
restart that one instance through the rig's `start_game`, never to hand-edit its environment.

`--insecure-no-token` and `--insecure-no-contract-a-token` accept unauthenticated connections.
They are for a single-machine test rig only, `--insecure-no-token` now **refuses to bind anything
but loopback**, and **no document this project ships may instruct anyone to pass them.**

**The shared LAN token is history, and this is its record.** From M3 until 2026-08-11 one secret
in `~/.multiverse-token` (mode `600`) authenticated every peer on the Contract B upgrade, passed
as `--token-file`, `MULTIVERSE_TOKEN` or `MULTIVERSE_TOKEN_FILE`, minted by hand with
`head -c 32 /dev/urandom | base64`. It travelled to the second computer by hand as `token.txt`.
`--token-file` now exists on **no binary**, no running process references the file, and
`e2e/run-m3.sh` keeps its own `ensure_token` only because it still speaks the retired wire. The
file itself is **left in place pending the owner's disposal call** — it is mode `600` and outside
the repo, so leaving it costs nothing and deleting it removes a retired secret.

### Driving one component by hand

Start the rig in dependency order — the relay first, then the archive, then the three
sidecars, then the three game instances:

```sh
export SSL_CERT_FILE=./e2e/tls-m4-lan/ca.crt        # trust this rig's CA, no sudo, no /etc

bin/relay   --listen 127.0.0.1:8795 --data-dir ./e2e/relay-data \
            --tls-cert ./e2e/tls-m4-lan/relay.crt --tls-key ./e2e/tls-m4-lan/relay.key
bin/archive --relay wss://127.0.0.1:8795/contract-b/v4 --peer-id archive-main \
            --data-dir ./e2e/archive-data --credential-file ~/.multiverse/archive-main.secret \
            --http 127.0.0.1:8796
bin/sidecar --listen 127.0.0.1:8787 --relay wss://127.0.0.1:8795/contract-b/v4 \
            --peer-id slot-1 --position 0,0 \
            --data-dir ./e2e/data/slot-1 --credential-file ~/.multiverse/peer-slot-1.secret
bin/sidecar --listen 127.0.0.1:8788 --relay wss://127.0.0.1:8795/contract-b/v4 \
            --peer-id slot-2 --position 1,0 \
            --data-dir ./e2e/data/slot-2 --credential-file ~/.multiverse/peer-slot-2.secret
bin/sidecar --listen 127.0.0.1:8789 --relay wss://127.0.0.1:8795/contract-b/v4 \
            --peer-id slot-3 --position 2,0 \
            --data-dir ./e2e/data/slot-3 --credential-file ~/.multiverse/peer-slot-3.secret
```

Both of those port numbers are now the compiled defaults, so the two flags above are
redundant and are written out only to make the layout explicit. See *The M4 port plan*.
**`--contract-a-token-file` is deliberately absent**: it defaults to
`<data-dir>/contract-a.token`, which the sidecar mints itself, and that is the exact path the
game is handed (*Credentials, TLS, and the retired LAN token*).

Five things about that invocation are load-bearing:

- **The path is `/contract-b/v4` and the scheme is `wss://`, since 2026-08-11.** M4's
  `/contract-b/v3`, M3's `/contract-b/v2` and M2's `/contract-b/v1` are different wires. The
  relay still *answers* on the retired paths and closes the connection with `4000`, so a stale
  client looks like a peer that will not join rather than a 404 — but a client still dialling
  `ws://` never reaches that close code, because the listener is TLS to its front door.
- **The relay takes no secret on a flag; every client takes exactly one.** `--tls-cert` and
  `--tls-key` are the relay's whole authentication configuration — the verifiers are already in
  `<data-dir>/peers.json` — and each client passes its own `--credential-file`. Passing the wrong
  peer's file is a `401`, not a mis-placed slot.
- **`--position <col>,<row>` is a sidecar flag, and it is how a map takes a shape.** Without
  it a peer is placed by the relay: the first hole in row-major order, or a new column on the
  shorter axis. That is deterministic but it is not *your* layout. `--insert-after-slot` and
  `--insert-axis` splice a newcomer between two live slots instead.
- **Slot order is still start order when nobody expresses a preference**, so the first
  sidecar to claim gets slot 1. `--slot n` (or `MULTIVERSE_SLOT`) is only a preference; the
  relay arbitrates, and it recovers a remembered slot from the `peerId` regardless. **Do not
  rely on `--slot` to order the map — pass `--position`, or pre-place with the relay's
  `--reserve-slot`.**
- **`--peer-id` is the slot's identity, and it is persisted.** It lands in
  `<data-dir>/peer-id` and is generated once if omitted, so a sidecar restarted against the
  same data dir reclaims the same slot. A reinstall with a fresh data dir takes a *new* slot
  and leaves the old one reserved forever.

**`--inbound-rate` / `MULTIVERSE_INBOUND_RATE` is the delivery-pacing lever** (the two-way-lane
set, `contract-a.md` §18 A40). It sets `inboundRatePerSimMinute`: the maximum `MIGRATE_IN`
frames released per **simulated** minute of the receiving world, with a token bucket of
`inboundRateBurst` so ordinary traffic is never delayed. The default rose from `2.0`/`5` to
`12.0`/`15` with D17, because D13 and then D17 multiplied the inbound surface past the old
number, and the raise is what made the knob exist at all — before it, the rate was a compiled
constant. It rose again the same day, to **`100.0`/`50`**, once two-way lanes actually ran and
the residual `pacedDepth` did not clear under `12.0`; `100.0` is sized two orders below A29's
mod-side ingest ceiling rather than from a projected median. `0` or an unparseable value keeps
the default. **Reach for it last.** A `pacedDepth`
that will not fall is usually a story about the traffic rather than the rate: the map-wide
symptom of a stale `contract-b` peer (above) looks exactly like an under-set rate, and raising
the rate there only pumps a bounce loop faster. Read `pacedDepth` per slot on `/api/status`
first, then the `bounceBack=true` share of that slot's `delivered MIGRATE_IN` lines, and only
then the rate.

`--listen` is loopback-only by contract and refuses a wildcard address. The relay's
`--listen` does not: it binds LAN-reachable (`0.0.0.0:8795` by default). Every flag also
reads an environment variable — `MULTIVERSE_LISTEN`, `MULTIVERSE_RELAY`,
`MULTIVERSE_PEER_ID`, `MULTIVERSE_DATA_DIR`, `MULTIVERSE_SLOT`, **`MULTIVERSE_CREDENTIAL_FILE`**
(and `MULTIVERSE_PEER_SECRET` for the secret itself — the retired names were
`MULTIVERSE_TOKEN_FILE` and `MULTIVERSE_TOKEN`), **`MULTIVERSE_CONTRACT_A_TOKEN_FILE`**,
`MULTIVERSE_LOG_LEVEL`, and `MULTIVERSE_RELAY_LISTEN` / `MULTIVERSE_RELAY_DATA_DIR` /
`MULTIVERSE_RELAY_TLS_CERT` / `MULTIVERSE_RELAY_TLS_KEY` /
`MULTIVERSE_ARCHIVE_PEER_ID` / `MULTIVERSE_ARCHIVE_DATA_DIR` for the other two binaries.
Sidecar and relay answer `GET /healthz` — **the relay's is `https://` now**, which is why the
rigs carry a `RELAY_HEALTH` beside `RELAY_URL` — and each sidecar writes its resolved listen address
to `<data-dir>/listen.addr`, which is how `--listen 127.0.0.1:0` stays usable from a script.
Beside it, since WP7, it writes **`<data-dir>/sidecar-process.json`** once its listener is bound and
removes it on a clean shutdown: pid, start time, peer id and listen address, which is how the two
commands below find the running process and how `--diagnose` tells a stale pid from a live one. It
is deliberately **not** called `sidecar.pid` — the packaged install's `Start-Multiverse.ps1` already
writes a bare `sidecar.pid` one directory up, in the data *root*.
The journal lives in `<data-dir>/journal/` and is the durable custody of decision D2 — keep
it across a restart or the sidecar loses every organism it was holding.

**Asking a sidecar about itself — `--my-slot` and `--diagnose`.** WP7's two support commands, live
on this rig since the roll of 2026-08-12. Both are **read-only**, both print paths and never a
secret, both exit without starting anything, and both work with the map unreachable. They are the
participant's half of what `/api/status` and `ringstat` give an operator, and they are the first
thing to reach for on one slot — reach for the status page when the question is about the map.

```sh
bin/sidecar --data-dir e2e/data-m4-lan/slot-2 --my-slot     # what does the map say about MY world?
bin/sidecar --data-dir e2e/data-m4-lan/slot-2 --diagnose    # 21 checks; exit 0 clean, 1 a FAIL, 2 could not run
```

`--my-slot` prints this world's place, its map link, its game, its speed, its last save, its four
lanes, its custody/paced/held depths and the whole map's liveness. It reads the running sidecar over
`GET /my-slot`, a loopback, unauthenticated, read-only endpoint on the Contract A listener the
sidecar already binds — **no wire message and no new port** — finding it through `listen.addr`.
`--diagnose` runs the checks this document and the runbook otherwise do by hand and judges them
`PASS`/`FAIL`/`WARN`/`UNKNOWN`/`SKIP`, with the remedy and **who must act** under anything that is
not a pass; `--json` emits the same records as `multiverse-diagnose/1`. An `UNKNOWN` is an honest gap
and never a pass — several are expected on a healthy machine, and each names what it was waiting on.
`--my-slot` writes nothing at all, and the only write `--diagnose` makes is one empty temporary file
in the data directory, which it removes — writability is the one thing a stat cannot answer, because
a read-only mount and a full disk both look like a healthy directory.

**Pass `--diagnose` the flags the process actually runs with, or it diagnoses a machine nobody
has.** The checks report on the configuration they are given, so the bare form above uses the
*default* relay (`ws://`) and no credential file, and duly reports `credential` as a `FAIL` and skips
`relay-tls` — neither of which says anything about this deployment. On this rig the honest
invocation carries the CA, the relay URL and the slot's own credential, and it exits 0:

```sh
SSL_CERT_FILE=e2e/tls-m4-lan/ca.crt bin/sidecar --data-dir e2e/data-m4-lan/slot-2 \
  --relay wss://127.0.0.1:8795/contract-b/v4 \
  --credential-file ~/.multiverse/peer-slot-2.secret --diagnose
```

Neither command disturbs the running sidecar — measured on the live deployment, with the peer's
relay session, connection age and shed count identical across the run (*The living deployment → The
two sidecar-only rolls of 2026-08-12*). The specification is `docs/sidecar-diagnose-spec.md` and the
participant-facing pages are `docs/participant/`; both are written for a stranger, not for this rig.

**Reading the archive.** M3 gives the archive one read path, and it is a subcommand of the
same binary — the recorder does not have to be running:

```sh
bin/archive list --data-dir ./e2e/archive-data          # every migration, with its lineage
bin/archive list --data-dir ./e2e/archive-data --gaps   # only those with a genome it lacks
```

Each migration prints one header line — receive time, `migrationId`, entity, `slot n -> slot
m`, source peer, outcome — then the migrant's genome hash and each parent, every hash tagged
`[held]` or `[MISSING]` by whether the archive actually has the bytes. A parent with no hash
prints as `gap: <gapReason>`. The `[MISSING]` set **is** the gap report of
`contract-b-m4.md` §10: the archive's honest statement of what it does not have, and the
retry queue's input. **`genomeGaps` on `/api/status` is a different number** — it counts the
fetches currently outstanding, not what the ledger shows missing — and it is a standing watch
item; see *The living deployment → Watch items*.

**Releasing a ring slot.** A slot reservation never expires — that is what keeps a peer's
place while it is offline (`contract-b-m4.md` §7.4). The escape hatch for a slot reserved by
a peer that is never coming back is a **startup flag**: the relay releases the slot, says so,
and exits without serving.

```sh
bin/relay --data-dir ./e2e/relay-data --release-slot 2   # releases, logs, exits 0
```

The released number is **retired, not reused**: the next peer appends at the tail. Stop the
relay before running it, and restart it afterwards.

**Handing a slot to a different peer.** M4's addition, and the answer to a reinstall that took
a fresh data dir and therefore a fresh `peerId`. It rebinds the reservation — slot, position
and all — to the new identity, and it is a command on the relay's own machine rather than a wire
operation. **Since `contract-b/4.0` it also mints a credential for the new `peerId` and drops the
old one** (§7.5, B22), which is what makes it the *only* recovery from a lost join string — and
what makes it expensive for slot 6, whose machine has persisted the id being retired.

```sh
bin/relay --data-dir ./e2e/relay-data --handover-slot 5=peer-lan-slot5-new   # rebinds, mints, logs, exits 0
```

**Reserving a slot before its peer connects.** The mirror image of the release flag, and
the reason a LAN map can form in any start order:

```sh
bin/relay --data-dir ./e2e/relay-data \
          --reserve-slot slot-1@0,0 --reserve-slot slot-2@1,0 --reserve-slot slot-3@2,0 \
          --reserve-slot slot-4@0,1 --reserve-slot slot-5@1,1 --reserve-slot slot-6@2,1
```

**Do not run that form by hand any more.** B22 folded issuance into reservation: a peer that
holds no credential gets one minted and its join string printed **once**, on stdout, and the
relay can never print it again. Six unrecoverable secrets in a terminal scrollback is how a map
loses its own members, so the capture belongs in a tool —
`e2e/crossing/mint-credentials.sh`, which files each secret into a `0600` file and prints only
the far end's. `run-m4.sh reserve` and `run-m4-lan.sh reserve` both delegate to it. **A peer that
already holds a slot is untouched**, credential included, which is what made minting against the
live map safe on 2026-08-11: `ring.json` came back byte-identical.

**The `@<col>,<row>` suffix is M4's, and it is optional.** With it the flags *are* the map, and
a 3×2 grid forms whatever order the six peers join in. Without it — the M3 form,
`--reserve-slot slot-1` — each reservation appends in the order given and the relay places it,
which is still valid and is what `e2e/run-m3-lan.sh reserve` uses. There is **no `--position`
on the relay**; that flag belongs to the sidecar, which expresses the same preference from the
other end.

Both are idempotent — a `peerId` that already holds a slot keeps it — and, like
`--release-slot`, both are startup commands: the relay writes `ring.json` and exits without
serving.

## The far end — the second computer

The second computer has no development environment (`m3_considerations.md` Risk 6). It gets
**artifacts, not a build**, in one zip, and two PowerShell scripts. Everything it needs is
made here:

```sh
farend/make-farend-bundle.sh      # -> farend/dist/farend-bundle.zip
```

**It runs against a live deployment; no game has to be stopped.** The bundle *builds* the
plugin (`dotnet build`) and reads the result out of `bibites-mod/bin/Release/` — it never
writes into `BepInEx/plugins/`, and the copy into that folder is the only step a running game
can block. That is why it calls `dotnet build` rather than `deploy.sh`, which is build **plus**
that copy. Deploying to *this* machine's games is a separate act with its own downtime: quit
the games, `bibites-mod/deploy.sh`, bring the rig back up.

The bundle holds `setup-farend.ps1`, `README.md`, a **fresh** `BibitesMultiverse.dll`,
`multiverse-sidecar.exe` (cross-compiled `GOOS=windows CGO_ENABLED=0` from
`go/`), and the pinned BepInEx 5.4.23.3 zip (downloaded once into the gitignored
`farend/dist/cache/`, SHA-256 verified). **The zip is no longer the whole handover**: since
`contract-b/4.0` the far end needs two more files that are deliberately *not* in it — the
relay's CA (`e2e/tls-m4-lan/ca.crt`, per deployment and not a secret) and the secret half of slot
6's join string. Both are carried by hand; the script's closing message lists all three. The
script refuses to build when
`setup-farend.ps1`'s pinned `$AssemblySha256` no longer equals
`bibites-mod/libs/BibitesAssembly.dll` — that pin **is** the version gate on the far end, and a
stale one would let two different game builds into one map. It also warns, without stopping,
when `libs/` no longer matches the game installed here — the check `deploy.sh` used to make on
its behalf.

**What that pin means changes at M5, and the rig's practice does not.** D22 (ratified
2026-08-10) makes compatibility layered: the map's membership test is the **contract** version,
and the game version is a **per-machine** matter answered by a published support matrix. So
this pin stops being a rule about the map and becomes the far end's own matrix entry — one
sidecar-and-mod build, stated against one game version. Keep running the rig the way it runs
now, on one build across both computers, because that is what makes a two-machine deployment
diagnosable; but a stale pin is a stale *bundle*, not a broken map rule. What would have to be
answered before two game builds legitimately shared one map was the bb8 payload question D22
named as its remaining work — whether a payload serialized by one game version restores safely
into another.

**The owner refined that on 2026-08-11: it is assumed to work, and the envelope carries the
serializing game's version for diagnosis only.** Nothing has to be built for the carrying, and
no bundle has to be rebuilt for it. `gameVersion` has been a **REQUIRED** field on
`MIGRATE_OUT` and `MIGRATE_IN` — and `body.version` on the wire — since M2, and the archive
records it on every migration; the live ledger lines read `"gameVersion":"0.6.3.1"`. So every
mod build the matrix could ever list already sends it — the far end's `0.6.2` plugin included,
because the field predates both — and there is no absent case to interpret. The rig's real obstacle is the opposite one: the
importing mod still refuses a mismatch outright (`MigrationImporter.cs:199`, `contract-a.md`
§9.2 `VERSION_UNSUPPORTED`), the sidecar's walk still skips a peer on a different game
version (`mapwalk.Deliverable` → `peer_incompatible`), and the **relay** still refuses a
mismatched peer at the handshake and a mismatched `SECTOR_CLAIM`
(`go/internal/relay/relay.go:467`, `:655`; `contract-b-m4.md` §6.1, §7.2 rule 8). Two game
builds cannot in fact share this map, whatever the map *rule* now says.

**The owner settled that on 2026-08-11 too, and the answer is to leave it alone:** *"lets not
change this then we will reconsider in the future if there's issues i think we can leave it
like its working now."* So the gates stay as shipped, and **one build across both computers is
confirmed standing practice** — not an interim measure waiting on an answer. It is also
**safe** rather than merely tidy: because nothing cross-version is ever delivered, no payload
from another game build reaches `SaveSystem.LoadBibiteOrEggFromData`, which is why the
assume-it-works reading is untested and not disproved.

**The consequence to plan a game update around.** Steam auto-updates stay on and land on the
two computers at different moments, so between the first update and the last the two machines
are on different builds and **organism flow partitions**: the relay refuses whichever peer
disagrees with the version the map already reports (`lastRefusal` on that slot, a
`version_incompatible` grant), the edges between mismatched pairs close, and any payload that
did get through would be NACKed permanently. That is working as shipped, not a fault to
diagnose. What ends it is the ordinary ritual, done promptly rather than at leisure:
`sync-game-refs.sh`, rebuild, `deploy.sh` here, and a fresh `make-farend-bundle.sh` zip applied
on the far end — the same *re-take the bundle promptly* rule that *The minors* gives for a
breaking wire minor, now applying to the game version as well. Until every machine is on the
new build, expect closed lanes and a bypassed slot, and read them as skew rather than as a dead
peer.

**The repo distributes the bundle.** `farend/dist/farend-bundle.zip` is tracked as of
`8463b72`, so the second computer clones the private GitHub repo and takes the zip out of the
checkout instead of receiving a hand-copied file. **Three things still travel by hand, and since
2026-08-11 that is two more than it was**: the relay's CA as `ca.crt`, the secret half of slot 6's
join string as `peer-secret.txt`, and the relay's LAN address — **none of them belongs in the
repo**, and the middle one is a secret the relay can never reprint. The retired
`~/.multiverse-token`, carried as `token.txt`, was the single hand-carried file until the
crossing. **Re-run `make-farend-bundle.sh` and commit the new zip** whenever the plugin, the
sidecar binary, `setup-farend.ps1`, `farend/README.md` or the `$AssemblySha256` pin changes;
a stale committed bundle is a stale member of the map, and under M4 that is a missing sixth
world rather than a spare.

### M4: the far end is slot 6, at position (2,1)

Under M3 the second computer was ring slot 2 of three. Under M4 it is **slot 6 of a 3×2
map**, and it is a full member: a real world, a real simulation, real periodic saves and real
portals, exporting on **all four edges** and receiving on all four (§18 A38). It is not a
spare and it is not synthetic — `bin/fakemod` exists only for the local rehearsal, which
cannot run a sixth game (*The five-instance ceiling*, in Gotchas). `e2e/run-m4-lan.sh`'s
header carries the full argument for slot 6 rather than any other; the short form is that it
keeps the resume test and the kill-and-heal test on this machine and still puts the healed
lane of phase 4 across the network.

With the zip in hand, its operator runs two commands, and `farend/README.md` is the whole of
their instructions:

```powershell
.\setup-farend.ps1 -RelayHost 192.168.1.227 -CaFile .\ca.crt `
    -PeerSecretFile .\peer-secret.txt
.\start-slot6.ps1
```

`setup-farend.ps1` finds Steam's copy of the game (registry, the usual folders, and every
extra library in `libraryfolders.vdf`), verifies `BibitesAssembly.dll` against the pin,
validates `-Position` and `-ExportEdges` before anything can silently disable the client,
installs BepInEx if it is absent, copies the plugin, **imports the relay's CA and stores this
world's own credential**, and generates
`start-slot6.ps1` and `stop-slot6.ps1`. **Both credential steps are new at `contract-b/4.0`**;
before it the installer took `-TokenFile .\token.txt` and wrote the shared token to
`%LOCALAPPDATA%\BibitesMultiverse\token.txt` with a user-only ACL. The CA import goes into *that
operator's own* trust store, `Cert:\CurrentUser\Root` — no administrator rights and no other
account affected — and the installer prints the CA's subject, thumbprint and expiry first, reads
the store back by thumbprint to prove the import landed, and prints the exact manual command if
it did not. `-SkipCaImport` is there for an operator who would rather do it themselves.
**Asking somebody else's machine to trust this rig's CA is a real request, and it is the owner's
to make.** Its M4 defaults are `-RelayPort 8795`, `-Slot 6`,
`-Position '2,1'`, `-PeerId slot-6`, `-World M4-Slot6`, **`-ExportEdges 'E,N,W,S'`** and
`-SidecarPort 8787` (loopback on that machine, where nothing competes for it). The
`-ExportEdges` validation still refuses a token that is not an edge and an edge repeated; the
old refusal of an edge declared **with its opposite** is gone, because all four together is
now the conformant declaration (§18 A38).

**Re-taking the bundle is not optional after a `contract-b` minor.** The bundle carries the
plugin *and* `multiverse-sidecar.exe`, and a stale sidecar is what refuses an upgraded
neighbour's west and south exports — see *The minors* for the measured symptom. Rebuild with
`farend/make-farend-bundle.sh` (it builds both fresh) **after** `bibites-mod/deploy.sh`, so the
DLL in the zip is the one this machine is running.

**After a `contract-b` MAJOR it is not a matter of degree at all**, and 2026-08-11 is the first
time this rig has said that. A stale sidecar does not refuse two lanes — it never joins: it dials
`ws://…/contract-b/v3` at a listener that is TLS to its front door, with a shared token no binary
takes any more. **Slot 6 is dark from the moment the new relay starts until its own operator
applies the new bundle** (which it did the evening of 2026-08-11, ~01:19Z 08-12 — see the
plugin row and `m5_tracking.md` WP2), and this machine cannot shorten that and must not try (D9). The useful
bound is 24 hours (`holdTimeoutMs`), which is not a deadline but the point at which entries held
for slot 6 bounce home by themselves.

**The tracked bundle is current, and it was rebuilt for the crossing on 2026-08-11** — `32f41f8`,
carrying mod `0.6.4` and the `contract-b/4.0` sidecar, built after `bibites-mod/deploy.sh` so the
DLL in the zip is byte-identical to the one this machine runs. **That is the bundle slot 6 has not
taken yet.** Before it, the bundle was rebuilt three times on 2026-08-10 — at `c4a7b51` for
the `0.6.2` plugin (the minimum-FPS disarm, which matters on a far end that also runs headless);
again after `contract-a.md` §20 A45 raised `heartbeatTimeoutMs` to 13 000, because that
number lives in the **sidecar** and the zip's rule is that it carries what this machine runs; and
a third time for the `0.6.3` plugin, the save-phase timers (*Watch items*, first item). The
second rebuild changed only `multiverse-sidecar.exe` and the third only the DLL; after each one
the plugin in the zip is byte-identical to the one deployed here. **Two of the three therefore carry
the same mod version and different sidecars, which is a trap when reading a far end's `modVersion` —
the pairing is tabulated below.** **The far end is free to ignore
`0.6.3`** — it adds no wire field, and phase timers on a world that saves in 567–924 ms measure a
question that world does not have. It **did** take one of these bundles, at 17:16Z on 2026-08-10. Before those three, the 2026-08-09 rebuild carried
the `-Headless` start template, and the one before it at `6899273` already postdated the
torn-append fix `f72dd14` (`git merge-base --is-ancestor f72dd14 6899273` passes), so the exe has
carried `journal-compact-minutes`, `journalCompactMinutes`, `discardedBytes` and
`JournalCompactInterval` — the whole §20 disk budget — since then. It now also carries
`--heartbeat-timeout`.

**Whether the second computer has APPLIED any of it cannot be determined from here, and no
observation will settle *which build* it applied.** Neither the disk-budget work nor A45 changed a
wire field, so a far end with them and one without are indistinguishable on Contract B
(`contract-b-m4.md` §20 says so as a design property, and `contract-a.md` §20 A46 says it again for
the timeout). Slot 6's §19 settings block proves its *mod* is at least the settings build and says
nothing about its sidecar. The archive-ledger fix `e68550b` does not re-open it either: its code is
`go/internal/archive/*` plus one sidecar *test* file, and the archive runs on this machine only.

**But WHETHER the installer ran is observable, and on 2026-08-10 it was observed.** `relay.log`
records slot 6's sidecar leaving the relay at **17:16:21Z** (`reservationKept=true`) and a new one
connecting at **17:17:26Z** and reclaiming slot 6 with `reason=reclaimed`. A mod-only bounce cannot
produce that pair — the 2026-08-09 headless flip moved the mod for 41 seconds and logged *only*
placement claims — so a `client gone`/`client connected`/`reason=reclaimed` sequence on slot 6 is
the signature of `setup-farend.ps1` having run and replaced `multiverse-sidecar.exe`. **That is the
check to make**, and it is cheap: `grep 'peer=slot-6' relay.log | grep 'client '`. Details in
*Slot 6 took the refreshed bundle*.

**What it does not tell you is the value, and `modVersion` cannot close the gap — this is the trap.**
Mod `0.6.2` shipped in **two** consecutive bundles with **different** sidecars:

| Bundle | Committed | Mod in it | `HeartbeatTimeoutMs` in it | Wire it speaks |
|---|---|---|---|---|
| `c4a7b51` | 2026-08-10 15:05Z | `0.6.2` | **3 500** | `contract-b/3.5` |
| `0290f6b` | 2026-08-10 16:30Z | `0.6.2` | **13 000** | `contract-b/3.5` |
| `04f22ee` | 2026-08-10 18:13Z | `0.6.3` | 13 000 | `contract-b/3.5` |
| `32f41f8` | 2026-08-11 | **`0.6.4`** | 13 000 | **`contract-b/4.0`** — and this is the one that is not optional |

Slot 6 publishes `0.6.2`, so "the mod version moved, therefore it took the bundle carrying the
raise" **does not follow**. What narrows it is the clock, not the wire: it ran the installer at
17:16Z, by which time `c4a7b51` had been superseded for 46 minutes, so the far sidecar is on 13 000
ms **if the operator fetched the bundle when they applied it**. Treat that as *probable by
provenance and unverified by observation* — and note that this ambiguity is a direct consequence of
A46 declining a version bump for a §10 default. `contractAVersion` read `contract-a/2.3` on all six
slots while that was measured, and is designed not to discriminate here. **The crossing ends the
ambiguity rather than answering it**: whatever slot 6 is on now, it cannot join at all until it
takes `32f41f8`, and after that it is on `0.6.4`, `contract-a/2.4` and 13 000 ms by construction.
Until then the five local slots read `contract-a/2.4` and slot 6 reads nothing, because it is dark.
(Slot 6 crossed the evening of 2026-08-11 and now reads `contract-a/2.4` too; the settling
observation below is from before that.)
**The natural experiment that would settle it has not occurred**: slot 6's mod has not gone silent
since 17:17:51Z, so no `silentFor` has ever been produced on that host to read. A `peer_mod_absent`
closure after ~3.5 s of slot-6 mod silence would prove 3 500; the lanes staying open through 4–12 s
of it would prove 13 000. Watch for it; do not manufacture it, and do not rebuild the bundle to
answer the question — a fresh bundle answers a different one.

**And on either value the far end is benign, which is why none of the above is urgent.**
The timeout is **per-sidecar and per-mod**: each sidecar judges the liveness of the one mod on
its own loopback socket, and that judgement is never compared with another peer's or published as
a number. So the old value would cost slot 6 exactly what it has always cost it — its own mod's
occasional `4004` and reconnect — and costs this side nothing. It also costs slot 6 close to
nothing in practice: **its saves run 567–924 ms, and 875 ms for a 582 KB world at 20:18Z on
2026-08-10**, an order of magnitude inside either deadline, because it is one world on its own
host. **The far host is also the control for this machine's own troubles**: through the nineteen
minutes of 18:39Z in which every local peer churned, slot 6 logged nothing at all (*The evening's
two session storms*).

`start-slot6.ps1` sets the mod's whole configuration natively — there is no `WSLENV` on that
machine, so `MULTIVERSE_CONTRACT_A_TOKEN_FILE` is set directly and needs no path translation —
starts the sidecar with `--position 2,1`, **waits for `contract B: slot granted` in
its log**, and only then starts the game. A failure to join prints the four usual causes,
names TCP 8795, and does not start the game. **That order was already right and now has a second
reason**: the sidecar mints the Contract A token file the game reads, so a game started first
would meet an enforcing sidecar and take a `401` on the ordinary ladder.

| Variable it sets | Value | Why it belongs there and not here |
|---|---|---|
| `MULTIVERSE_EXPORT_EDGES` | `E,N,W,S` | The M4 plural name, and `setup-farend.ps1`'s `-ExportEdges` default. All four lanes wrap: a 3×2 map is a torus, so every slot declares all four axes and the sidecar decides from the relay's map which of them `EDGE_STATUS` opens. Under D17 each declared edge is both an export edge and an entry edge (`contract-a.md` §18 A38). |
| `MULTIVERSE_RING_SLOT` | `6` | The advisory label. The sidecar closes `4001` if it disagrees with the granted slot, which turns a mis-wired far end into one second of diagnosis. |
| `MULTIVERSE_SIDECAR_PORT` | `8787` | Contract A is loopback-only, on that machine's loopback. |
| `MULTIVERSE_WORLD` | `M4-Slot6` | The mod seeds it on the first start. This machine never creates it. |
| `MULTIVERSE_CMD_FILE` | `%TEMP%\bibites-m4\cmd-6.txt` | Local to that machine, and used only by its own operator (`F10` also works). |
| `MULTIVERSE_SAVE_MINUTES` / `_KEEP` / `_ON_QUIT` | `10` / `6` / `true` | **The periodic save is mod configuration, so it is set there and nowhere else.** This machine never asks for a save and never schedules one (`contract-a.md` §5.2). What crosses the network is a **receipt** — `HEARTBEAT.lastSave` — which reaches the status page and is the only wire path an operator surface has to a remote world's save state. |
| `MULTIVERSE_PORTAL` / `_FLOURISHES` | `true` / `true` | The far screen is the only place slot 6's strips and rings can be seen. |

**The `-GameOnly` switches are for the arrival-pacing test, and nothing else.**
`.\stop-slot6.ps1 -GameOnly` stops the world and **leaves the sidecar running**, so it keeps
its slot, its relay session and its journal, and goes on taking custody of arrivals while the
world is away. `.\start-slot6.ps1 -GameOnly` brings the world back against that sidecar. That
is the only way an undriven far end can play the dark half of `run-m4-lan.sh phase5far`: the
rig prints the command, waits for `modConnected` to flip on the status page, and observes.
A full `stop-slot6.ps1` would take the sidecar too, the lane would close, and there would be
nothing to accumulate.

**`-Headless` goes with either start form**, and it adds Unity's `-batchmode -nographics` to
the game launch and nothing else: the same world, the same saves, the same portals, drawn
nowhere. Leaving the switch off the next start brings the picture back, so it is a property
of a run and not of the installation — and slot 6 has now been taken through it both ways,
headless at 2026-08-09 23:15Z and drawn again at 2026-08-10 00:58Z, ~90 s of world downtime
each way with the sidecar up throughout and a time-scale re-send needed after every one of
them (*The living deployment*). `start-slot6.ps1` is **generated** by `setup-farend.ps1`, so a
far end installed before 2026-08-09 has a start script without the switch, and takes it only
from a re-run of the installer out of the current bundle — or from the same edit by hand.

### Owner steps: making the relay and the status page reachable (elevated, on this machine)

WSL2 here runs in **NAT mode**, and `.wslconfig` sets that deliberately: `networkingMode=mirrored`
breaks Docker Desktop port publishing on this host. A relay on `0.0.0.0:8795` inside WSL is
therefore reachable from Windows but **not from the LAN** until Windows forwards the port into
the VM. All three commands below need an elevated PowerShell and all three are **owner
steps**; `e2e/run-m4-lan.sh lanhost` prints them with the current addresses, the current
portproxy table and the recorded LAN host already filled in.

```powershell
# 1. FIRST, AND IT BLOCKS THE RIG: delete the M3-era portproxy on 8790. Under the M4
#    port plan 8790 is SLOT 4'S Contract A port, and this proxy listens on 0.0.0.0.
netsh interface portproxy delete v4tov4 listenport=8790 listenaddress=0.0.0.0

# 2. once. -Profile Any, NOT Private: an Ethernet NIC classified "Public" silently ignores a
#    Private-only rule, and the far end just reports the relay unreachable. See Gotchas.
New-NetFirewallRule -DisplayName "Bibites Multiverse relay 8795" -Direction Inbound `
  -Action Allow -Protocol TCP -LocalPort 8795 -Profile Any

# 3. whenever the WSL address has MOVED. Check first with `lanhost`; it did not move across
#    either the 2026-08-08 or the 2026-08-09 reboot, and re-running this blind drops the
#    forward for as long as the two commands take.
netsh interface portproxy delete v4tov4 listenport=8795 listenaddress=0.0.0.0
netsh interface portproxy add v4tov4 listenport=8795 listenaddress=0.0.0.0 `
  connectport=8795 connectaddress=<the WSL address from `lanhost`>
```

**Both survive a reboot.** The firewall rule and the portproxy are Windows state, not WSL
state, so a host reboot leaves them in place; and the WSL address they point at is **not**
guaranteed to change either — it stayed `172.24.110.174` across both of this deployment's
reboots. So step 3 is conditional, and the condition is a comparison: run
`e2e/run-m4-lan.sh lanhost`, which prints the live WSL address beside the current portproxy
table and changes nothing, and re-add only if the two disagree. Assume it *can* move on any
restart; do not assume it *has*.

**The port is `8795` from M4 on, not `8790`** (*The M4 port plan*, in Gotchas). The M3-era
firewall rule for 8790 can stay; it opens a port nothing serves. **The M3-era portproxy on
8790 could not stay, and step 1 is the one that removed it**: `run-m4.sh up` and
`run-m4-lan.sh up` both read the Windows listener table first and **refuse to start** while
any Windows process — a portproxy included — holds a Contract A port. While it existed the
only way past was the five-digit dodge, `SLOT_PORT_BASE=18787`, which moves all six slots to
`18787`–`18792`.

**It is gone, and the standing requirement is that it stays gone.** The proxy used to be
`0.0.0.0:8790 -> 172.24.110.174:8790` (measured 2026-08-06), which by luck pointed at the
live WSL address and so *mostly worked* — the worst possible state, because the moment the
WSL address moves the same proxy silently swallows slot 4's Contract A connection and the map
forms one slot short with the reason `peer_mod_absent`, which reads like a mod bug. Confirmed
absent on 2026-08-09: `lanhost` lists only the `8795` forward and an unrelated `8765` one.
A row for `0.0.0.0:8790` reappearing in that table is a regression, not a leftover.

**Relay LAN host: `192.168.1.227`.** Confirmed 2026-08-03 by the far end itself: the second
computer ran `setup-farend.ps1 -RelayHost 192.168.1.227`, was granted slot 2, and the relay
reported `ringSize=3`. Give the same value to `setup-farend.ps1 -RelayHost`. It is one of this
machine's Windows IPv4 addresses; `lanhost` lists the candidates, and the home-LAN one is
normally `192.168.x.x` or `10.x.x.x`, never a `172.x` hypervisor address.

**Since 2026-08-11 this address is also on the relay's certificate.** The far end's TLS client
verifies the name it *dialled*, so `192.168.1.227` is a SAN on the leaf that
`e2e/crossing/mint-tls.sh` mints (beside `127.0.0.1`, `localhost` and the WSL address). **If the
LAN host ever changes, the certificate has to be re-minted** — `mint-tls.sh --renew-leaf`, which
keeps the CA and therefore costs the second computer nothing. A far end dialling a name that is
not on the leaf fails verification and never connects, which from here is a dark slot 6 and no
other symptom.

**The address alone is not enough — the portproxy behind it must exist.** `192.168.1.227:8795`
only reaches the relay while the `netsh` portproxy above points at the *current* WSL address.
Re-run `e2e/run-m4-lan.sh lanhost` after a restart and compare; re-add the portproxy only if
the address it prints differs from the one in the table. The LAN host itself does not change,
so this line stays correct even when the far end suddenly cannot connect.

#### The status page on the LAN — the same pattern, on `8796`

The archive binds `127.0.0.1:8796` by default, so the status page is invisible from every
other machine on the LAN — including the second computer, whose operator has no other view
of the map. **`ARCHIVE_HTTP` overrides the bind address**, and `run-m4.sh` reads it, so
`run-m4-lan.sh` inherits it:

```sh
ARCHIVE_HTTP=0.0.0.0:8796 e2e/run-m4-lan.sh up     # the LAN binding
e2e/run-m4-lan.sh up                               # the default, 127.0.0.1:8796
```

The Windows plumbing is the relay's plumbing with one number changed. Both commands need an
elevated PowerShell, and both are **owner steps**:

```powershell
# once
New-NetFirewallRule -DisplayName "Bibites Multiverse status page 8796" -Direction Inbound `
  -Action Allow -Protocol TCP -LocalPort 8796 -Profile Any

# only when the WSL address has moved — check with `lanhost` first, as on 8795
netsh interface portproxy delete v4tov4 listenport=8796 listenaddress=0.0.0.0
netsh interface portproxy add v4tov4 listenport=8796 listenaddress=0.0.0.0 `
  connectport=8796 connectaddress=<the WSL address from `lanhost`>
```

`-Profile Any` matters here for the same reason it matters on `8795`: a Private-only rule
does nothing on a network Windows has classified `Public` (see Gotchas). The page then reads
as `http://192.168.1.227:8796/` from any machine on the LAN. **Test it from Windows or from
the second computer, never with `curl` from WSL** — WSL has no hairpin route back to its own
host, and the false negative reads exactly like a blocked port (see Gotchas).

The page is **read-only by design** (D15, `m4_considerations.md` *Scope*), so LAN exposure
adds no write surface. It is still a LAN-only step, and M5 owns public exposure.

The page is **three views over one poll — Map, Species and Settings — and the tab is in the URL
hash**, so `#species` is a link somebody can send and a reload lands where the reader was. `#map`
(the default, and anything unrecognised) is the **visual map**: an SVG grid of the worlds with
population drawn as dots, lanes drawn as arrows (wrap-arounds split at the map edge, bypasses
curved over the world they skip), each lane labelled with its measured hop rate, and a glossary
that explains every term to a reader who did not build the system. **The only thing that moves
along a lane is a real crossing** — the ambient pulse that used to walk every arrow was removed
once real hops travelled the same path (§10.1), so a glyph in motion is always an organism this
archive was copied on. **Each cell's speed chip carries two numbers, `×100 → ~×12`** — what the
game reports *applying*, then what the archive *measured* it delivering — and on this deployment
they are nowhere near each other. The second half is drawn only once it has been measured, so a
cell showing `×100` alone is an archive that has just started or a world that has just come
back, never a claim that the two agree; the tooltip, the world card and `ringstat`'s `speed→got`
column say the same thing at more length. Read the gap, not the target: *A world can be at the
wrong time scale* in Gotchas is what it means. `#species` is **who is alive right now** — the
union of every world's census, keyed on the §16 A34-normalized name
and printed in each world's own spelling, annotated with what the ledger knows about it
(crossings ever, first and last sighting, distinct genomes, recent lanes, parent species) and
badged `everywhere` / `endemic` / `never-exported`. It is **alive-only**: a species that crossed
a thousand lanes and is extinct everywhere is a ledger fact, not a resident. `#settings` is one
card per world of what that world was **told to do** — mod and protocol versions, save interval,
keep count, save-on-quit, wrapping and the exclusion list — and it is read-only, and says so.

| Endpoint | What it answers |
|---|---|
| `/api/status` | the live frame every tab is drawn from. It gained `recentHops` per lane and `flowWindowMs` with §17, the speed and pacing readout with §18, §19's seven per-slot fields — `modVersion`, `contractAVersion`, `migrationExclude` (with `migrationExcludeKnown`), `saveMinutes`, `saveKeep`, `saveOnQuit` and `worldWrapping` — and, since **2026-08-10**, the **measured** speed: `achievedTimeScale` and `achievedSpanMs` per slot beside the applied `timeScale`, with `achievedWindowMs` at the top. **No wire field was added for it** — it is Δ `simulatedTime` / Δ `statsAsOfMs` over the last minute, both of which `PEER_STATUS` has always carried, so it is §10.1 rule 2's *derived, and marked as derived* and nothing on the wire moved. Absent means the archive has not watched that world for long enough yet, which is not the same fact as a peer refusing to say |
| `/api/hops` | the last minute of crossings, which the map animates |
| **`/api/species`** | the alive union with its ledger annotations, plus `reportingSlots`, `censuslessSlots`, `truncatedSlots`, `ledgerSpecies` and `ledgerRecords`. **The join happens here, not in the browser**, so the page and `ringstat` can never disagree about which species is endemic |
| **`/api/species/history?key=&hours=&buckets=`** | one species' per-world population over time, downsampled from `metrics.jsonl`. `key` is required and bounded: missing, empty or over-long answers **`400`** |
| `/api/history?hours=&buckets=` | the same downsample for every world's total population — the map's sparklines |

**The species aggregate costs one pass, not one per poll.** It is built during the ledger replay
the archive already performs at startup, and maintained per new record after that, so a
half-million-line ledger is never re-read to answer a request. Measured on 2026-08-07 against
599,184 records (176 MB, warm page cache): the replay took **~6 s**, and the archive's settled
RSS went from 146 MB to **170 MB**, with a transient peak near 690 MB while the pass ran. That
rollout widened each `metrics.jsonl` sample by **~640 bytes (+7.6 %)** — the settings block for
five publishing worlds — and the achieved-speed readout added **~393 bytes (+2.7 %)** on
2026-08-10, on a file that samples `/api/status` verbatim. The published-limits view added
**~255 bytes (+3.2 %)** on 2026-08-12 (live from the next archive restart), same mechanism.

**But the pass is the cost of a restart, and it grows with the ledger.** Re-measured on
2026-08-10 against **3,700,672 records (1.22 GB)**: the replay took **~93 s** with a transient
peak RSS near **5.2 GB**, and **the archive serves nothing until it finishes** — it binds `8796`
after the replay, not before. That restart cost **100 seconds** of archive downtime end to end,
and the crossings inside the window are **never copied to it**: a subscriber that is absent
changes nothing about a migration (§5.1), which is exactly why they are absent from the record
rather than delayed in it. Plan an archive restart; do not treat it as free.

**Those seconds are a rate against the ledger, not a property of the archive, so every figure
above expires.** The ledger is append-only and is never rewritten (§10 makes eviction from it
illegal), so the replay grows exactly as the file does — about **40,000 records a second**, or
**13 MB/s**, on this host. On **2026-08-11** the file is **6.24 M records (2.0 GB)**, 69 % more
than the day before, which at the same rate is **~150 s of replay** and ~160 s of archive
downtime. **Any number of seconds recorded in this document is stale the day after it is
written.** The durable form is the rule, not the reading: size the replay from `wc -l` (or `du`)
on today's ledger against the rate above, and size every wait, budget and outage estimate around
it from that — never from a figure quoted here or in a script comment. See *Bringing it back
after a reboot* for the one wait this bites first.

`ringstat` is the same three views in a terminal, over the same data and from the same place:

```sh
bin/ringstat                        # the map: grid, lanes, flow, custody/paced/held, last save
bin/ringstat --species              # who is alive, joined to the crossing record
bin/ringstat --settings             # what each world was told to do; '?' where it has not said
bin/ringstat --watch 5s             # repeat, clearing the screen
bin/ringstat --metrics <file>       # read the newest sample from metrics.jsonl instead
```

`--url` defaults to `http://127.0.0.1:8796` (`MULTIVERSE_ARCHIVE_URL`), `--timeout` to 5 s.
`--species` against `--metrics` prints `n/a` in the crossing columns and says so: that half is
derived from the ledger, and only a running archive holds it.

## The disk budget

**The living deployment filled the root filesystem on 2026-08-08 and stopped writing
genomes.** Nothing in the system had ever bounded what it consumed. This section is what
that cost, what bounds it now, and what still grows forever.

### What it was writing

Measured on the five local sidecars, the relay and the archive, after two days up:

| Producer | Before the fix | Bounded now by |
|---|---|---|
| Sidecar journals (5) | 445, 500, 905, 683, 720 MB — **3.25 GB**, for a live set of a few thousand tombstones | `journalCompactMinutes`, default **15 min** |
| Sidecar and archive logs | **3.5 GB**, unrotated | `logRotateMb` x (`logKeep` + 1), default **100 MB x 6** per process |
| Genome scratch files | 15,119 empty `*.json.tmp` left by writes that failed on the full disk | the failing write removes its own scratch file; the cache sweep collects any a crash abandoned |

Together about **1.1 GB/hour**, on a 98 GB volume shared with every other project on this
machine. The rig cannot run overnight against that. It is **0.30 GB/h** now, and only
78 MB/h of that is growth nothing ever reclaims — see *What the fix measured*.

### The journal only ever shrank at startup

`internal/journal` compacted at `Open` and nowhere else, so a sidecar that stayed up
accumulated every `create` record it had ever written — payload included, up to 4 MiB
each. `PurgeExpired` made this *worse*, not better: dropping a tombstone appends a purge
record, so cleanup grew the file.

A compaction never reads the file it replaces — the in-memory state map **is** the
compacted content — so it costs one pass over the live entries and runs in milliseconds.
It now runs on a timer as well, outside the sidecar's custody lock, with the same
scratch-fsync-rename-syncdir discipline `Open` already used. `contract-b-m4.md` §20 (B20)
is the rule; `--journal-compact-minutes` and `MULTIVERSE_JOURNAL_COMPACT_MINUTES` are the
knobs.

### The log was a shell redirect

Every process wrote slog to stderr and `run-m4.sh` caught it with `>>`, which has no size.
Each process is now **given** its log path (`--log-file`) so it can rotate it, at
`--log-rotate-mb` keeping `--log-keep` generations. The path is the same one every helper
in `run-m4.sh` already greps, so nothing that reads a log changed. The shell redirect
stays, pointed at a `<name>.stderr.log`, because a Go panic goes to fd 2 and never passes
through slog; those files stay tiny.

Without `--log-file` a process still logs to stderr and bounds nothing. That is the
default, and it is what the tests and interactive runs use.

### THE EXPENSIVE PART WAS NOT THE DISK — IT WAS ONE SHORT WRITE

When the volume filled, an append to each journal landed **short**: some bytes reached the
file, the rest did not, and the call returned an error. The sidecar did the right thing
with the error and never ACKed. But the half-record stayed in the log, and the next
successful append wrote a whole record straight behind it — producing an unparsable line
in the **middle** of the file. Replay stops at the first unparsable line, because a torn
line is only ever supposed to be the last one.

**All five sidecars then ran for eight hours with journals that replayed to 01:07:40 and
no further.** Nothing looked wrong: each process held correct state in memory and kept
ACKing correctly. Any restart in those eight hours — a crash, a deploy, a reboot — would
have silently reverted every one of them to its 01:07 state. That is precisely the loss D2
exists to make impossible, arriving through the one path that had no rule written for it.

Two changes close it in `internal/journal`:

- **An append is all-or-nothing.** On any write error the file is truncated back to the
  length it had before the attempt. The caller still gets the error and still must not
  ACK; the failure now costs that one record instead of every record behind it.
- **A damaged journal is loud.** `Journal.Discarded()` reports the bytes replay threw away
  behind a torn line, counting only *complete* records — an unparsable line with nothing
  after it is the ordinary `kill -9` torn tail and cost nothing durable. The sidecar logs
  it at **error** on startup.

Recovering the running rig is in the git history of 2026-08-08: the five journals were read
out of `/proc/<pid>/fd/` while the processes still held them, which is the only place the
post-01:07 records existed.

**The archive's ledger had the same defect, and kept it a day longer.** `internal/archive`
is a second implementation of the same append-only discipline and it got neither fix on
2026-08-08, so `migrations.jsonl` still carries that night's splice: **776 bytes at line
874,163**, the head of a `NACK` written at 01:21:49 joined to the tail of a record written at
08:12:05, once the disk had room again. Replay broke there in silence, exactly as the
journals had. Found and closed on **2026-08-09**, and the two halves are not symmetrical
with the journal's:

- **The same all-or-nothing append.** `Ledger.Append` truncates back to the pre-write length
  on a short or failed write, and `OpenLedger` drops an unterminated final line before the
  first append of the new process can splice itself onto it.
- **Replay SKIPS a damaged line instead of stopping at it**, which is where the ledger
  departs from the journal deliberately. The journal may stop and truncate because `Open`
  compacts it from the in-memory state map immediately afterwards and loses nothing. The
  ledger has nothing to rebuild from — it **is** the state, it is never rewritten (§10 makes
  eviction from it illegal), and it will carry that line for the life of the deployment. So
  it reads past it and keeps every record behind it, and what it reports is the damaged line
  itself rather than the history behind it, because none is thrown away. It is logged at
  **error** on startup and carried as `ledgerSkippedLines` on the status page, in the
  `/api/status` JSON and in `ringstat`.

Verified against a copy of the living ledger, 2026-08-09: **1,287,592 lines replay to
1,287,591 records with exactly one line skipped, 776 bytes.** The old code stopped at
**874,162** and reported nothing. **Proven on the deployment itself on 2026-08-10**, on the
first restart after the fix: the same one line and the same 776 bytes, 3,700,672 records kept,
188,006 of them recovered from behind the splice — see *The archive's ledger recovery* in *The
living deployment*, which also has the reconciliation against `wc -l`.

### What still grows forever

| File | Growth | Measured 2026-08-08, six live slots |
|---|---|---|
| `e2e/archive-data-m4-lan/genomes/` | with **new genomes**. Content-addressed, so it tracks evolution rather than hops. The archive never sweeps it — the sidecars' own caches are capped by `genomeCacheMaxBytes`, the archive's is the record | **58 MB/h = 1.4 GB/day** |
| `e2e/archive-data-m4-lan/migrations.jsonl` | with **traffic**. The ledger is the record of what happened and nothing may evict from it (`contract-b-m4.md` §10) | **19 MB/h = 0.45 GB/day**, at ~540 crossings/min |
| `e2e/archive-data-m4-lan/metrics.jsonl` | with **time**, one sample per slot per `metricsInterval` | **0.7 MB/h = 0.02 GB/day** |
| `e2e/baselines/m4-collector/` | with **time**. Not part of the deployment — it is a read-only observation loop, copying five world saves and one `/api/status` sample every 5 minutes. Written and launched by the evolution-analysis session on 2026-08-07, not by hand. Stop it with `kill "$(cat e2e/baselines/m4-collector/collector.pid)"`; **every reboot kills it and nothing restarts it** — see *The living deployment* | **19 MB/h = 0.5 GB/day** |
| **total** | | **97 MB/h = 2.3 GB/day = ~70 GB/month** |
| `e2e/logs-m4-lan/bepinex/` | with **restarts, not with time** — so it is not in the total above. Every archiving pass copies the whole live BepInEx set, and nothing ever sweeps a generation | **3.4 GB → 21 GB in the single crossing window of 2026-08-11** |

**The BepInEx log archive is the grower nobody sized, and one window found it.** The five live
`LogOutput.log` files were **6.08 GB** on 2026-08-11, and a crossing copies them **three times**:
once by hand before anything stops (`RUNBOOK.md` P1, so a mod deploy cannot truncate the
`[M4-SAVE]` history it is deployed to measure), then again inside `lan_down`, then again inside
`lan_up`. That is ~18 GB added in seven minutes, and it took `e2e/logs-m4-lan/bepinex/` from
**3.4 GB to 21 GB**. The volume absorbed it — `/mnt/wsl/data` went from **39 GB to 56 GB used,
195 GB free** — so this cost the window nothing but time. **What it costs is time**: see the same
window's entry in *The living deployment*, where those copies are most of the difference between
the runbook's estimate and the measured outage. **A retention rule for this directory is a watch
item** (*Watch items*, fourth): the archive is the only record of `[M4-SAVE]` history and cannot
simply be deleted, but keeping every generation of a 6 GB set at three copies per window is not a
rule either.

**`e2e/baselines/m4-collector/` is gitignored, and that is load-bearing.** `e2e/baselines/`
itself **is** tracked — 57 files including earlier baseline save zips — so this directory
sits inside a tracked tree while growing half a gigabyte a day; measured 2026-08-09T20:21Z
it held **208 snapshots, 1,262 files, 375 MB**. It was excluded in `8cbeca6` precisely
because a single `git add -A` would otherwise commit all of it and keep committing it;
`git add -f` is the escape hatch for a keeper worth committing by explicit path. **Stage
specific paths in this repo** either way. Whether the collector's output should stay ignored
outright or be periodically pruned with its keepers committed is **an open question for the
owner** — the two halves of `e2e/baselines/` still disagree about what kind of thing it is.

No rule in this system will ever shrink the three durable files above. At 97 MB/h the
251 GB data volume holds roughly **three months** of this deployment, and then somebody has
to decide what the archive is for. **Sizing and pruning it is an operator job**, and it is
why the rig now lives on the data volume rather than the WSL root.

### What the fix measured

Over a 19.5-minute window with all six slots live, immediately after the rolling restart:

| | Before | After |
|---|---|---|
| Everything the rig writes | **~1.1 GB/h**, all of it monotone | **0.30 GB/h**, of which only 78 MB/h is monotone |
| Sidecar journals | grew forever; 3.25 GB after two days | a **sawtooth**: ~2.5 MB after a compaction, ~40 MB before the next one. The first timer compaction reclaimed 147 MB across five sidecars in one pass |
| Logs | grew forever; 3.5 GB after two days | 42 MB/h into files capped at `100 MB x 6` per process |

The five journals went from 445/500/905/683/720 MB to **4.6/5.3/10.4/7.5/7.9 MB** on the
restart's replay-and-compact, and the 15-minute timer has held them there since. The
pre-fix logs were rotated aside to `*.log.1` on the first write — 1.5 GB of them, kept
deliberately because they contain the outage.

### Where the rig lives now

The whole repo was moved to `/mnt/wsl/data/bibites-multiverse/` on 2026-08-08 and
`~/bibites-multiverse` is a symlink to it. Every absolute path — in `run-m4.sh`, in the pid
files, in the running command lines — resolves through the symlink unchanged, so this was a
move with no rewrite. `git status` from the symlinked path is clean and git is untroubled by
it.

**`/mnt/wsl/data` must be mounted before the rig starts.** It is a separate WSL disk, and
`/mnt/wsl` itself is a tmpfs, so when the volume is not attached the symlink **dangles**:
every path under `~/bibites-multiverse` fails with *No such file or directory*, `git` cannot
find the repo, and the rig cannot start at all. The symptom is unmistakable and it is not a
rig bug. **The owner's mount setup is the fix** — there is deliberately no automation here,
because a script that silently created the directory would put the custody journals on a
tmpfs that a restart erases. It is **step 1** of the reboot ritual below, and no other step
can run before it:

```sh
df -h /mnt/wsl/data          # /dev/sdX, 251G — if this is missing, stop and mount it
ls ~/bibites-multiverse/     # must list the repo, not fail
```

## The living deployment

The M4 LAN rig has carried the same six worlds since the exit test of 2026-08-06 — across a
full-disk outage, two host reboots, one far-end restart and, on 2026-08-11, a **major wire
crossing** followed the same day by the **WP4 + WP5 rollout, which cost no downtime at all**
because it moved binaries and not the wire. This section is what it last read, how to bring it
back after a reboot, and what is being watched.

### The reading of 2026-08-09, after the second reboot

Taken from `/api/status` once the map had settled. Every number is a reading, not a target;
the cumulative ones move continuously and the point of recording them is the shape, not the
digits.

| | |
|---|---|
| Slots | **6/6 live and `modConnected`**, no holes, no unknown slots |
| Populations | slot 1 **7**, slot 2 **24**, slot 3 **25**, slot 4 **22**, slot 5 **24**, slot 6 (far) **104** — **206** total |
| Lanes | **24**, counted as `sum(len(slot.exportEdges))` over the live slots. **24 open, 0 closed, 0 bypass**, every one `reason=peer_live` |
| Queues | `pacedDepth`, `heldDepth`, `custodyDepth` and `timeoutBounces` all **0** |
| Traffic | **393,210** cumulative migrations at **426.4/min**; epoch **394** |
| Errors | zero `OVERLOADED`, zero `MALFORMED_MESSAGE`, zero `bounceBack=true`, and no unexplained error line in any log |
| Portals | **8/8** `[M4-PORTAL] event=SHOWN` per game — four edges × two lanes, which is the full D17 set |
| Speed | ×5 on all five local worlds; the far end runs ~21–24, which is its own operator's setting and not this rig's business |
| Saves | Every world saving on its 2-minute interval, every rotation clean, no `event=FAILED`. **Not clean against D14, though**: read over the whole run rather than at one instant, stalls span **0.47–2.69 s** and **20 of 718** saves exceeded the 2 000 ms budget. See *Watch items* |

**Re-read the same day at 20:21Z, and three of those lines had moved in ways worth knowing.**
Population **243** (slot 1 **14**, slot 2 **43**, slot 3 **40**, slot 4 **34**, slot 5 **43**,
slot 6 **69**), **431,645** migrations at **516.4/min**, epoch **6,960** — the cumulative
counters and the epoch simply run, and a number quoted from either table is a timestamp, not a
property. Still 24/24 `peer_live`, still no bypass, `pacedDepth` and `heldDepth` still 0
(`custodyDepth` 1, which is one organism in flight). But **slot 2 reported ×34.7 while the
other four still reported ×5** — the time scale drifted *hours after* a clean bring-up, not
only across a restart, which widens the gotcha below. It was **corrected later the same hour**
with the rig's own `e2e/run-m4-lan.sh send 2 timescale 5`, which answered
`targetTimeScale=5.00 Time.timeScale=5.00` and held on the following sample with all five
local worlds back at ×5. And both of the population and backlog watch items moved; see
*Watch items*, which since 2026-08-09 carries a third.

**The far end needed nothing.** Neither reboot took the second computer down, and slot 6
reconnected by itself both times — at 12:07:10 on 2026-08-08 — so the map formed 24/24 with
no bypass and no `no_peer` closure. There is no far-end step in the ritual below, and adding
one would be a D9 violation as well as unnecessary.

### Slot 6's headless window, 2026-08-09 23:15Z to 2026-08-10 00:58Z

**The mode belongs to a start, and it has now been flipped both ways.** The second computer's
game ran under `-batchmode -nographics` for that 1 h 43 m and has been drawn again since. Each
flip is the same two moves — a clean quit, whose save-on-quit writes the world out, then
`.\start-slot6.ps1 -GameOnly` with or without `-Headless` — and each costs about **90 s of
world downtime** and nothing else. Going headless quit in 13 s; coming back quit in 3 s and
left `M4-Slot6-20260810T005739Z.zip` behind. **It is the same world across both**:
`simulatedTime` and the population run continuously through each restart, and the sidecar held
its slot, its relay session and its custody throughout, so the map never saw a hole. Nothing
on the status page distinguishes a headless slot from a drawn one, which is the point.

Two things came out of the headless hours. The periodic save is **faster** with nothing to
draw — one 2 485 ms warm-up sample, the only breach of Risk 3's 2 000 ms budget, and then
672 ms steady against 1 093–1 535 ms in visual mode. And the sidecar's custody replay burst
briefly outran its send buffer: one transient `1011: outbound queue full` disconnect,
reconnected clean, no organism lost — the backlog draining as designed rather than a fault.

**The return trip added two facts and no surprises.** The window handle and the `GPU Engine`
counters come back with the picture — ~60–80% of that machine's 3d engine for the process,
where headless had no counter instances at all — so the process-level evidence is symmetric in
both directions. And the replay burst threw the **same** single `1011: outbound queue full`
and healed itself again, which makes that a property of a backlog draining after any world
absence, not of either mode.

**Every one of these restarts resets the runtime time scale.** Slot 6 came back at ×1 the
first time and was set to ×6.5; the second time a `timescale 6.5` sent mid-settle answered
`Time.timeScale=3.32`, and the documented re-send took it to 6.50. Headless has nothing to do
with that — a flip is a restart, so it inherits the gotcha whole and adds nothing to it (*A
world can be at the wrong time scale*, in Gotchas).
**What the wire saw of that switch, from this side.** Slot 6's mod went quiet at **23:14:43Z**
and was back at **23:15:23Z** — 41 seconds over the whole window, of which the quit itself took
13 — claiming `simulationSize=2000` and `exportEdges [E N W S]` again. **Only the mod
bounced.** The far *sidecar* never left the relay — `relay.log` has no `client connected` and no
`client gone` in that whole hour, only placement claims — so the slot reservation, the
Contract B session and that machine's journal were never in question, and the map read the
outage the ordinary way: a mod-absent flap drops slot 6's four export lanes off the list and
closes the ones into it with `peer_mod_absent`, which is exactly what an earlier flap looks
like in the 21:59:53Z collector sample (20 lanes, `custodyDepth` 3, `pacedDepth` 3). **Nothing
was re-sent and nothing bounced.** Across the 23:00Z hour the five local sidecars logged 10,230
`MIGRATION_PAYLOAD` forwards, every one at `reroutes=0`, and **zero** `bounceBack=true`, **zero**
`duplicate=true`, **zero** `OVERLOADED` and **zero** `MALFORMED_MESSAGE`. Three organisms
crossed into slot 6's sidecar after its mod went quiet; the quiet-mod gate held them and the
world took them when heartbeats resumed. The dedup path was therefore **never exercised** —
there was no duplicate to catch, which is the M4 defences reading correctly rather than a hole
in the evidence — and slot 6's first post-restart save receipt reached the page by 23:25Z.

**Read again at 00:47Z on 2026-08-10.** 6/6 live and `modConnected`, **24/24** `peer_live` with
no bypass, `pacedDepth`, `heldDepth` and `timeoutBounces` 0 and `custodyDepth` flickering 0–1 as
organisms cross. Population **176** (slot 1 **8**, slot 2 **27**, slot 3 **31**, slot 4 **33**,
slot 5 **29**, slot 6 **48**), **516,633** migrations at **251.4/min**, epoch **26,656**,
`genomeGaps` **600** against `ledgerRecords` **1,129,222**. Slot 6's §19 block is complete and
unchanged — `0.6.1`, `contract-a/2.3`, `saveMinutes=10`, `saveKeep=6`, save-on-quit, world
wrapping, `migrationExclude=[Basic bibite]` — and its save receipt is flowing: `M4-Slot6.zip`,
380,069 bytes in **567 ms**.

**The speed change is the rig's business only through the crossing rate.** The five local
worlds still report ×5, and against the simulated clock they advance 4.94–5.00× per wall
second. Slot 6 reported a fluctuating **17–54** across the 34 collector samples between the
reboot and 22:00Z — the `~21–24` in the table is one point on that curve, not a property — and
the post-switch **×6.5** is a target its host nearly holds, at 5.84 simulated seconds per wall
second. What speed the second computer runs at is still its own operator's business; what is this rig's
business is that the crossing rate fell with it, from **~500/min** at 20:21Z to **230–305/min**
over the ten collector samples since. That is why the third watch item drained, and it is the
cleanest demonstration yet of the mechanism that item names.

**A local world churned in the same hour and the save budget does not explain it.** Slot 4's mod
went silent past Contract A's heartbeat timeout — **3.5 s at the time, 13 s since 2026-08-10
(§20 A45)** — **ten times between 23:43:04Z and
23:46:20Z**: ten `4004` closes with `silentFor` pinned at 3.50–4.23 s, each followed by a
reconnect 0.3 to 14 s later, settling on generation 11 at 23:46:36Z, and no local
mod has churned since. **Its game never restarted** — `[M4-SAVE] armed` appears once in that
instance's whole log and the periodic counter runs unbroken through the burst — and **no save
was long enough to be the cause**: the three saves spanning the window stalled 1.67–1.82 s, well
inside the 3.5 s the dose response in *Watch items* needs. **This burst is therefore the one
episode the 13 s raise cannot have fixed**, and it is what makes the residual churn a separate
question there. The map absorbed it the same way it
absorbed the far end: 20 lanes and two `peer_mod_absent` closures at 23:45:29Z, back to 24/24 at
×5 by 23:51:03Z, `timeoutBounces` still 0 for the whole run. Nothing was lost, but the trigger is
**unexplained**, and it is the first burst of this shape with no save behind it.

### The five local worlds run headless and target ×100, since 2026-08-10

(The far end had already proven the flip both ways by then and was drawn again from 00:58Z —
its mode is its operator's business, and no part of this change.)

The owner's decision, taken after the far end proved the pattern: switch the five local games
to `-batchmode -nographics` and raise their time-scale target from ×5 to ×100. It is a
**game-process-only** change — no sidecar, relay, archive or collector was stopped — and both
halves now live in `e2e/run-m4-lan.sh`, so a bring-up reproduces the regime without a
command-line flag (*Bringing it back after a reboot*, step 4). The rehearsal `run-m4.sh` is
untouched at ×5 and drawn.

**The rollout, one slot at a time between 01:23Z and 01:30Z on 2026-08-10.** For each slot:
`send <n> quit`, wait for the process, then the rig's own `start_game <n>` with
`BIBITES_EXTRA_ARGS` exported, then `send <n> timescale 100`.

| Slot | Quit | Mod back after | Log file | First save headless |
|---|---|---|---|---|
| 1 | 3 s | 16 s | `LogOutput.log.1`, the one it freed | 609 ms / 110 KB |
| 2 | 5 s | 12 s | `LogOutput.log` | 1 846 ms / 231 KB |
| 3 | 4 s | 15 s | `LogOutput.log.2` | 2 093 ms / 266 KB |
| 4 | 4 s | 12 s | `LogOutput.log.3` | 1 910 ms / 226 KB |
| 5 | 4 s | 12 s | `LogOutput.log.4` | 1 373 ms / 210 KB |

Every instance took back **the log file its own stop had just freed**, so the starvation trap
never fired — which is what one-at-a-time buys. **Only the mod bounced, five times over**:
`relay.log` records no `client connected` and no `client gone` for any local peer in the whole
window, and each sidecar logged exactly **one** `4004` heartbeat close, its own game's. The map
never dropped below 24 lanes long enough for a collector sample to see it, and `timeoutBounces`
is still 0.

**A local quit is 3–5 s, not the far end's 13, and its receipt may not survive the exit.**
Slots 2, 3 and 4 logged `[M4-SAVE] event=SAVED why=quit`; **slots 1 and 5 did not, and slot 1's
save demonstrably happened anyway** — `M4-Slot1-20260810T012349Z.zip` and a `M4-Slot1.zip`
written at 01:23:49Z, both inside a quit window whose last periodic save was 01:22:41Z.
BepInEx's disk listener loses whatever is still buffered when the process goes, and a 3-second
quit is short enough to lose the last line. So on a fast quit **the zip on disk is the evidence,
not the log line** — check it there before the `keep=4` prune reaches it, which is what happened
to slot 5's. Do not read a missing `why=quit` line as a missing save.

**×100 does not hold, and this is the interesting result.** Over 16 collector-grade samples
between 01:36Z and 01:49Z, with `timeScale` read off `/api/status` and the achieved rate
measured against each world's own `simulatedTime`:

| Slot | Applied (`timeScale`) | Achieved per wall second |
|---|---|---|
| 1 | 45–100, median **100** | 27–68, median **42** |
| 2 | 11–36, median 18 | 4.6–12, median **6.6** |
| 3 | 13–61, median 25 | 6.1–18, median **9.4** |
| 4 | 22–44, median 30 | 7.8–15, median **11** |
| 5 | 16–81, median 28 | 8.6–25, median **11** |
| 6 (far) | 9–72, median 43 | 4.3–24, median 16 |

Only slot 1 — 5 to 10 organisms — reports its target, and even it achieves under half of it.
`TimeController.CheckMinFPS` clamps the other four hard, and `Time.maximumDeltaTime` takes
another cut on top (*A world can be at the wrong time scale*, in Gotchas). **What actually
changed is the aggregate**: the five local worlds advanced **63–124 simulated seconds per wall
second together, median 80**, against **24** at ×5 drawn — a real 3–5× on the whole map, not
the 20× the target names. Headless bought the GPU and about a gigabyte of RAM per instance
(403–466 MB against 1.27–2.07 GB); ×100 spent the CPU that bought.

**The far end is off ×6.5 on its own account.** Slot 6 reported 78–90 as the local rollout
began and 9–72 through the hour after it, achieving 4–26. That is still its operator's business and
no part of this change; it is recorded because the ×6.5 in the section above is now history.

**Crossings roughly doubled, and the queues did not move at all.** The status page's own
`perMinute` read **345–367** in the samples before the rollout and **603–781** after it; an
independent migration-delta measurement over the same windows gives 573/min before and
603–993, median 734, after. Across that second window: 6/6 live and `modConnected`, **24/24 lanes `peer_live` with no
bypass**, `pacedDepth` 0, `heldDepth` 0, `timeoutBounces` 0, `custodyDepth` 0–6, and **zero**
`OVERLOADED`, `MALFORMED_MESSAGE` or `level=ERROR` on the Go side. Population held its range:
**186–242** total against 212–236 before, with slot 1 still the small one at 2–22.

**Two watch items moved.** `genomeGaps` climbed exactly as its mechanism predicts for a
doubled crossing rate, and the save stall moved the other way from the hope — worse, not
better. Both are written up in *Watch items*; neither was retuned.

**Thirteen hours later the regime had settled, and the readings are bigger than the first
hour's.** At 14:42Z, after running unattended since 01:30Z: 6/6 live, **24/24 `peer_live`, no
bypass**, `pacedDepth` and `heldDepth` 0, `timeoutBounces` 0. Population **360** — slot 1 **15**,
slot 2 **60**, slot 3 **80**, slot 4 **49**, slot 5 **42**, slot 6 **114** — so the worlds grew
under the faster clock rather than thinning out, and slot 6 grew most. **1,366,165** cumulative
migrations (≈812,000 crossings in thirteen hours, ≈1,040/min), `ledgerRecords` **2,824,211**, and
`genomeGaps` **≈46,000**. Achieved rates fell as the populations rose: the local aggregate read
**48** simulated seconds per wall second over 12:04Z–14:07Z against the **80** measured in the
first hour, because a bigger world costs more per tick.

**Then the minimum-FPS servo came off, in the same regime and by the same method.** `0.6.2`
disarms the game's own frame-rate governor in a process with no graphics device, which is what
was holding four of the five worlds at an applied 13–19 while they were asked for 100. The
rollout was the same shape as the time-scale one with one difference that matters: **a mod deploy
locks the plugin DLL, so all five games have to be down at once** rather than one at a time.
Both passes (14:39Z and 14:57Z — the second only to bump the version so the page can name the
build) went: quit all five, `deploy.sh`, then `start_game` one at a time with a log-file check, a
world-ready wait, a mod-connect wait and `timescale 100`. All five came back inside **100
seconds**, none starved of a log file, `relay.log` shows no local sidecar leaving, and every slot
now publishes `modVersion 0.6.2` and a clean `timeScale 100`. The result — a local aggregate of
**74** against the servo's 48 — is in *A world can be at the wrong time scale*, in Gotchas, with
the caveats.

**Three costs came with it, and none is settled.** The five-at-once outage produced a **custody
replay burst** on the way back — `custodyDepth` 55 and `pacedDepth` 14 at its peak — which
**drained to 0 within about four minutes** with `timeoutBounces` still 0 and nothing lost, the
same shape the far end's switch showed. The **archive's own relay session dropped and reconnected**
at 14:39:29Z, five seconds apart, while its process stayed up: harmless, but it resets the
`epoch` the page shows, so an epoch that suddenly reads in the hundreds after a bring-up is a
resubscription and not a new map. And the **save stall got worse again** — see *Watch items*.

### The five local worlds save every ten minutes, and a silent mod gets thirteen seconds, since 2026-08-10

The owner's second decision of the day, and the one the first watch item had been waiting on:
**save less often, and allow a longer disconnect.** The two halves are independent changes that
answer one measurement, so they were rolled out together — see *Watch items*, *The decision*, for
why each number is what it is, and `contracts/contract-a.md` §20 (A45, A46) for the contract half.

|  | Was | Is | Who carries it |
|---|---|---|---|
| Save cadence | `saveMinutes=2 saveKeep=4` | `saveMinutes=10 saveKeep=6` | `e2e/run-m4-lan.sh`, assigned unconditionally after the source. **The games** read it from the environment at load, so each has to restart once |
| Heartbeat timeout | `3500 ms` | `13000 ms` | **The sidecars**, from a compiled default in `go/internal/contracta/contracta.go`, and newly overridable with `--heartbeat-timeout` / `MULTIVERSE_HEARTBEAT_TIMEOUT` |

**Nothing else moved. The mod did not change**, so no DLL was rebuilt and no version bumped —
`heartbeatTimeoutMs` is sidecar-owned and the mod has no matching constant. **That is what made
this a rolling change**: the DLL was never locked, so no step took more than one world down at a
time, and there was no five-at-once custody burst of the kind the `0.6.2` deploy paid.

**The rollout, 15:35Z to 15:59Z, in two passes of five.** The rig has no single-sidecar command,
so the sequence was composed from the functions phase 4 already uses to splice one slot back in:
`kill_pid sidecar-<n> -TERM`, wait for the process, `start_sidecar <n>`, `wait_healthy`,
`wait_grant`. Then the games, in the shape the ×100 rollout used: `send <n> quit`,
`game.sh wait`, `start_game <n>` through the rig so `BIBITES_EXTRA_ARGS` and the save environment
are carried, wait for the world and the mod, `send <n> timescale 100`.

| Slot | Sidecar restarted | Journal bytes discarded | Custody recovered on replay | Slot reclaim | Game quit | New cadence on the page after |
|---|---|---|---|---|---|---|
| 1 | 15:35:12Z | **0** | 3 out, 1 in | `reason=reclaimed` (0,0) | 6 s | +42 s |
| 2 | 15:36:39Z | **0** | none pending | `reason=reclaimed` (1,0) | 5 s | +45 s |
| 3 | 15:37:19Z | **0** | none pending | `reason=reclaimed` (2,0) | 7 s | +23 s |
| 4 | 15:37:59Z | **0** | 0 out, 1 in | `reason=reclaimed` (0,1) | 5 s | +17 s |
| 5 | 15:38:45Z | **0** | 0 out, 2 in | `reason=reclaimed` (1,1) | 4 s | +20 s |

**Every sidecar came back to its own slot and its own coordinate, replayed its journal with zero
discarded bytes, and reopened all eight of its lanes.** The map never left **24/24 `peer_live`**
in any sample across either pass, `pacedDepth` and `heldDepth` stayed 0, and `timeoutBounces` is
still 0. Each mod reconnected on the next generation of its own session — `gen=2` on four slots,
`gen=3` on slot 4 — which is the ordinary shape of a sidecar restart and is why exactly-once
holds across one.

**Two operational notes worth keeping.**

- **Rebuild the Go binaries by rename, not in place.** Five running sidecars hold `bin/sidecar`
  open, so an in-place `go build -o ../bin/` fails with `ETXTBSY` (*Workflow*). Building to a
  scratch directory and `mv`-ing over `bin/sidecar` is atomic and leaves every running process on
  its old inode until its own restart — which is exactly what a rolling change needs. **Verify per
  slot by digest, not by inode**: `sha256sum /proc/<pid>/exe` against `sha256sum bin/<name>`, plus
  `readlink /proc/<pid>/exe`, whose **`(deleted)`** suffix is what a replaced binary looks like from
  the outside. **An inode comparison cannot work on this host** — measured 2026-08-11,
  `stat -c %i /proc/<pid>/exe` returns the *procfs entry's own* inode, a distinct `7xxxxxx` per pid,
  never the target file's, so the two numbers can never match and the check says nothing. The digest
  is read *through* the magic link, so it names the file the process is actually executing. On the
  live rig this reads exactly as the rule predicts: the running relay is `DIFFERENT` and
  `(deleted)`, because `bin/relay` was rebuilt after it started, and the five sidecars are `SAME`.
  `e2e/crossing/RUNBOOK.md` **P6.2** has the loop over the pid files.
- **A restarted game can land on a DIFFERENT BepInEx log file than the one it left**, so a line
  mark taken in `game.sh logfile` before the quit is meaningless against the file that comes
  back, and a `wait_log` against it hangs. **Read readiness off `/api/status` instead**: it is
  per-slot, rotation-proof, and `saveMinutes` on it is the thing being rolled out. Slot 1 was
  restarted before this was understood and came back reporting `timeScale 1`, because the
  timescale command landed on a world that had not finished loading; a second `send 1 timescale
  100` twenty seconds later took. **The first `timescale` after a load answers
  `Time.timeScale=1.00` and the reported scale sticks there** — send it again.

**All five instances kept their own log file** (`LogOutput.log` and `.1` to `.4`, one each), so
the starvation trap never fired, which is what one-at-a-time buys.

**The first evidence, 15:40Z to 16:26Z, and it is a first reading and not a verdict.** Thirteen
periodic saves across the five worlds, counted from each slot's own game restart, against every
`4004` its own sidecar logged since its own restart:

| Slot | Save stalls, in order | Over the 2 000 ms budget | Over the retired 3 500 ms | `4004` behind any of them |
|---|---|---|---|---|
| 1 | 1 564 / 957 / 959 / 741 ms | 0 of 4 | 0 | none |
| 2 | **4 207** / 2 223 / 2 711 ms | 3 of 3 | **1** | **none** |
| 3 | 2 406 / 1 953 ms | 1 of 2 | 0 | none |
| 4 | 2 727 / 1 595 ms | 1 of 2 | 0 | none |
| 5 | 2 716 / 1 684 ms | 1 of 2 | 0 | none |

**Slot 2's 4 207 ms save at 15:52:33Z is the one that matters.** Under the old deadline it had
about a one-in-two chance of a `4004`; it took none, and slot 2's sidecar has logged no heartbeat
close at all since 15:42:33Z. That is one observation of the thing the decision was for, and one
is not a rate — the 13-hour generation that produced this problem had 63 saves over 3.5 s, and
this window produced one. **The proof accrues over the next generation**, which at a ten-minute
cadence is where it has to accrue.

**Four `4004` closes happened during the rollout itself, and none of them is a save.** All four
fall inside the restart passes — 15:38:19Z and 15:57:20Z on slot 4, 15:42:33Z on slot 2,
15:58:13Z on slot 5 — and every one reports `silentFor` of **13.0 to 14.2 s**, which is the new
deadline plus up to one monitor tick doing exactly what it is supposed to. Slot 4's first is the
clearest and worth keeping: its mod's `gen=2` connection dialed at 15:38:06.799Z and then **never
handshook**, the monitor reaped it 13.000 s later, and `gen=3` was up 0.29 s after that. So the
visible cost of the raise, in the only place it has shown up, is that **a zombie connection made
during a restart now lives thirteen seconds instead of three and a half** — and the working
connection beside it never noticed.

**Everything else held for the whole window.** 24/24 lanes `peer_live` with no bypass in every
sample, `pacedDepth` and `heldDepth` 0, `bouncedTimeoutTotal` 0 on all six slots, no
`level=ERROR` anywhere on the Go side, `genomeGaps` flat at ≈46 200–46 700 against
`ledgerRecords` climbing 2 915 260 → 3 003 379, and the far end untouched — as of that window
`relay.log` recorded its last `client connected` on 2026-08-09. **That check no longer reads that
way**: the far end took the refreshed bundle at 17:16Z the same day, so re-running it now returns
2026-08-10 (*Slot 6 took the refreshed bundle*). The local aggregate reads **48 simulated seconds per
wall second** over a 60-second sample with all five reporting an applied `timeScale` of 100,
which is the thirteen-hour figure rather than the 74 measured at smaller populations right after
the servo came off; the worlds are bigger now (*A world can be at the wrong time scale*).

### Slot 6 took the refreshed bundle, 2026-08-10 17:16Z

**The far end re-ran `setup-farend.ps1`, and for the first time the relay log shows it.** Slot 6's
mod published `modConnected=false simulationSize=0 exportEdges=[]` at **17:16:20Z**, its
**sidecar** left the relay one second later at **17:16:21Z** with `reservationKept=true`, a new
sidecar connected at **17:17:26Z** and reclaimed slot 6 at (2,1) with `reason=reclaimed`, and the
new mod was up at **17:17:51Z** claiming `simulationSize=2000` and `exportEdges [E N W S]`. Total
outage **91 seconds**, and the map absorbed it the ordinary way.

**That `client gone`/`client connected` pair is the whole finding, because a mod-only bounce never
produces one.** The 2026-08-09 headless flip is the control: it moved slot 6's mod for 41 seconds
and `relay.log` recorded *only placement claims* through the entire window (*Slot 6's headless
window*). A Contract B session ending and a slot being **reclaimed** is the signature of the
sidecar *process* being replaced — the same signature the five local sidecars made during their own
rolling restart. So this is not an inference from a version number: the far sidecar binary was
swapped, which is what `setup-farend.ps1` does (it installs the plugin DLL *and*
`multiverse-sidecar.exe`, and regenerates `start-slot6.ps1`).

**It is the same world.** `simulatedTime` runs continuously across the gap — 5,305,464 s at
16:03Z to 5,315,625 s at 17:21Z — and slot 6's §19 block is unchanged apart from the version:
`contract-a/2.3`, `saveMinutes=10`, `saveKeep=6`, save-on-quit, world wrapping,
`migrationExclude=[Basic bibite]`. Its save receipt still flows, at **875 ms for 582 KB**.

**`modVersion` moved `0.6.1` → `0.6.2`, and that does NOT identify which bundle it took** — see
*The far end — the second computer* for why, and for what is and is not settled about its
heartbeat timeout as a result. Its time scale is its own operator's business as ever; it now asks
for **100** and achieves 4–26, and it has been observed at `timeScale` **0** for one collector
sample (18:06Z), which is what put `custodyDepth` 106 and `pacedDepth` 52 on the map in that
sample and drained by the next one.

### The evening's two session storms, 2026-08-10 18:39Z and 20:22Z

Two Contract B churn episodes ran on the same evening, and a third was then produced on purpose.
**None lost an organism** (`timeoutBounces` 0 throughout, no `bounceBack=true`, no
`MALFORMED_MESSAGE`), and **none was a save.** They are recorded together and read separately
because they are different animals.

- **The 18:39Z storm** is the residual-churn question in *Watch items* finally showing itself at
  scale: a host that stopped scheduling anything. It was correlation only — until the evening's
  own verification load reproduced it at 21:25Z, which is recorded under it and **promotes it to
  a reproduction**.
- **The 20:22Z flap** was **a defect in the archive**: a fetch queue that walked its whole
  64,000-entry backlog under its own lock every second and starved its own read loop, so the relay
  closed it for not reading. **Root-caused, fixed and deployed the same evening** as
  `contract-b-m4.md` §21, B21. It cost the ledger **9,641 crossings** and cost the map nothing.
  Read it as the worked example of a subscriber that is present and not reading, because that state
  is invisible in every log until the relay gives up on it — and it is losing records the whole
  time.

**18:39:52Z to 18:59:10Z — nineteen minutes in which the host could not keep any peer alive.**
This is the largest churn episode the deployment has logged, and it reached *past* the mods to
the Go processes:

| Peer | `peer silent, dropping` | `client gone` | reconnects | `reservationKept=true` |
|---|---|---|---|---|
| slot 1 | 6 | 6 | 6 | 6 |
| slot 2 | 5 | 5 | 5 | 5 |
| slot 3 | 4 | 5 | 5 | 5 |
| slot 4 | 5 | 8 | 8 | 8 |
| slot 5 | 6 | 6 | 6 | 6 |
| **five sidecars** | **26** | **30** | **30** | **30** |
| `archive-main` | 2 | 14 | 14 | 0 (a subscriber holds no slot) |
| slot 6 (far) | **0** | **0** | **0** | — |

**A sidecar being dropped for silence is the new information.** `relay: peer silent, dropping` is
the relay's own Contract B liveness judgement, and a sidecar has almost nothing to do but answer
it — so thirty of them in nineteen minutes says the *host* stopped scheduling Go processes on a
loopback socket, not that anything in the rig misbehaved. Alongside them the five mods took **15
`4004` closes with `silentFor` of 13.40 to 24.41 s**, every one of them **past the new 13 s
deadline**, and five `wsutil: outbound queue full` forward failures (four on slot 4, one on
slot 3).

**No save is behind any of it.** The eight periodic saves inside the window stalled **1 146 to
3 006 ms** — an ordinary reading for this regime — and the nearest own-slot save to each of the
fifteen `4004`s is 38 to 294 seconds away. A mod that goes silent for 22 seconds without a save
running is not the save path; it is the thread not being scheduled at all.

**Every reservation was kept, and the map healed itself.** The one collector sample that caught
the storm, at **18:45:05Z**, reads `liveSlots` **5**, **20** lanes, `custodyDepth` **345**,
`pacedDepth` 7, `heldDepth` 17 and `perMinute` down to **511.8** from ~1,100; the next sample at
18:50:39Z is back to 6/6, 24 lanes and `custodyDepth` 28, and by 18:56Z `perMinute` is 1,144
again. `timeoutBounces` never left 0.

**The far end is the control and it never noticed.** Slot 6 has exactly **two** relay-lifecycle
events in the whole of 2026-08-10 — the 17:16Z bundle swap above — so the storm was bounded to
this machine, which is the strongest evidence available that its cause is this host's load and not
the map, the relay or the wire.

**What it correlates with, stated as correlation.** The window opens 93 seconds after commit
`c1ad688` (18:38:19Z), whose measurement re-read **10,946 `[M4-SAVE]` lines across ~3.5 GB of
archived BepInEx logs joined to 725 collector snapshots** — on the same host that was running five
Unity processes at an applied ×100, the relay, the archive and five sidecars. Thirty minutes later
the archive's 5.2 GB replay ran (*The archive's ledger recovery*). **No process accounting was
kept, so this cannot be promoted to cause** — but the shape (common-mode across every local peer,
absent on the far host, ending as abruptly as it began, and not recurring in the far hotter hours
since) fits host starvation and fits nothing else on offer. **The operational reading is that
measuring this deployment hard enough perturbs it**, and a churn burst with no long save behind it
should be dated against what else the box was doing before it is called a defect.

**A controlled reproduction of the 18:39Z storm, 21:25:21Z to 21:31:11Z — and this time the load
was known, because it was mine.** Verifying the §21 archive fix below meant running `go test ./...`
(the sidecar package alone is 56 s of concurrent tests) and two passes that each read 500 MB of the
ledger, on the same box as five Unity worlds at ×100. That produced the 18:39Z signature, at one
tenth the scale and with **no save behind any of it**:

| | 18:39Z storm, 19 min | 21:25Z reproduction, 6 min |
|---|---|---|
| sidecar `client gone` (all `reservationKept=true`) | 30 | **7** — slot 3 ×4, slot 5 ×2, slot 4 ×1 |
| relay-side `peer silent, dropping` | 26 | **1** |
| mod `4004` closes, and their `silentFor` | 15, at 13.40–24.41 s | **3**, at **14.80 / 15.89 / 18.20 s** — all past the 13 s deadline |
| `wsutil: outbound queue full` on a forward | 5 | **6**, on slot 3 and slot 5 |
| `bounceBack=true` / `timeoutBounces` / `heldDepth` | 0 / 0 / 0 | **0 / 0 / 0** |
| the far end | never noticed | **never noticed** |

**This promotes the reading above from correlation to reproduction.** The 18:39Z paragraph could
only say the shape "fits host starvation and fits nothing else on offer"; a known, dated,
self-inflicted load now produces the same shape on demand — common-mode across local peers, absent
on the far host, `silentFor` well past a deadline that no save came near, and gone as soon as the
load stopped. **The 24 `OVERLOADED` NACKs in the window are not part of the finding**: they are
`class=transient` with `handoff=sent` and the sender keeps custody, and slots 4 and 5 have logged
602 and 1,011 of them across the whole run. **Nothing was lost** — every reservation kept, every
lane back to `peer_live`, `custodyDepth` down from 311 to 5 within five minutes.

**Two operational rules come out of it.** First, `go test ./...` and a full-ledger read are
**production load on this host**, and a churn episode dated to one of them is not a deployment
fault; batch the heavy analysis, or accept the churn and say so in the record. **`nice -n 19 go
test -p 1 ./...` is the cheap mitigation and it was measured**: the same suite re-run that way
twelve minutes later, on a rig at 1,018 crossings a minute, produced **zero** peer drops of any
kind. Serialising the packages and yielding to the Unity threads costs about 20 s of wall clock and
buys the whole episode back. Second, and this is
what makes the reproduction lucky rather than merely embarrassing: **`archive-main` took zero drops
through all of it**, which is a harder test of the §21 fix than the quiet window it was supposed to
get. The pre-fix archive could not hold a session at a *healthy* 1,378 crossings a minute; the
fixed one held one through the host starvation that dropped three sidecars seven times.

**20:22:52Z to 20:52:23Z — the archive alone, twenty-six times, and the genome backlog is the
engine. ROOT-CAUSED AND FIXED THE SAME EVENING; the paragraphs below are the closed account and
they correct the first reading in place.** `archive-main` dropped and reconnected **26 times in
thirty minutes**, `reservationKept=false` every time. **No sidecar dropped, no mod took a `4004`,
and the map stayed 24/24 `peer_live`.**

**Why the session died, and it is not the burst.** The first reading blamed the burst of
`GENOME_REQUEST`s each resubscribe fired, and the burst is real — **180 requests inside 33 to
176 ms**, which is `genomeRequestsPerMinute` (30) times the six peers, spent in one pass, because
`allowSendLocked` opens a fresh one-minute window per peer and then admits the whole of it at
once. **It is not what killed the session.** Two things say so. The bursts are ~68 s apart and
the session deaths are not aligned with them; and 180 frames never came near the archive's own
256-frame outbound queue. What killed it is the **walk**, not what the walk sends:

- `pumpFetches` walked the **whole** `pending` map on every one-second tick, holding `a.mu` —
  the same lock `handle()` needs for `PING`, `PEER_STATUS` and every recorded migration — and
  calling the genome store's `Has` on every entry. **`Has` is an `os.Stat`.** At the 64,736-entry
  backlog that is 64,736 stat calls a second: **284 ms to 1.02 s measured** against this store on
  an otherwise quiet box, and more under the load of five Unity worlds at ×100.
- With the walk restarting every second, the read loop was admitted about **once per pass**
  instead of the ~30 frames a second the relay was copying to it. `archive.log` shows the
  starvation directly as gaps in the `recorded a migration` stream: **48 gaps over 1 s in nine
  minutes, quantised at roughly one-second multiples, the longest 6.5 s.**
- The relay's per-peer outbound queue is **128 frames** (`wsutil.New(ws, 128)` in `relay.go`).
  At ~46 frames a second — one `MIGRATION_PAYLOAD` and one `MIGRATION_ACK` per crossing at the
  1,336–1,378/min the rate hit at 20:14Z and 20:19Z, the evening's two highest samples — a
  **~2.8-second** stall fills it, and `wsutil.Send` then closes rather than growing without limit.
  **So the relay killed the session, and it said so:** `archive.log` carries
  `relay session ended … received close frame: status = StatusInternalError and reason =
  "outbound queue full"`. The `read: connection reset by peer` and `unexpected EOF` endings are
  the same close arriving as a reset.
- The resubscribe changed nothing about the backlog, so the walk resumed and the queue filled
  again. **That is the whole loop**, and it is self-sustaining without any help from the burst.

**Two claims in the first reading were wrong and are corrected here.** "The rest the archive
closed on itself" — **no**: only 2 of the drops were relay-side `peer silent`, and the other 24
were the relay closing the archive with **1011**, which the archive received. The archive closed
nothing. And the `send failed … GENOME_REQUEST err="wsutil: connection closed"` runs — 231 of them
in one instance — are not a cause either: they are the pump walking on through a dead connection,
logging one line per remaining entry. **A quiet `epoch` was still not resolution**: that part
stands, and the flap did stop on its own at **20:52:23Z** after sixteen minutes of stability at a
comparable crossing rate, which is the marginality of the mechanism showing rather than a
recovery. It would have returned at the next resubscribe with the backlog still there.

**Why 20:22Z.** Two curves reached their extremes together for the first time. The walk's cost
grows with `genomeGaps`, which had climbed to ~63,000 — the 19:08Z replay stepped it 56,658 →
60,675 in one jump (*The archive's ledger recovery*) and the ×100 regime added the rest. The
queue-fill deadline **shrinks** with the crossing rate, and 20:14Z and 20:19Z are the two highest
`perMinute` samples of the evening. At ~63,000 entries and ~1,378/min the pass cost and the
2.8-second deadline finally met. Nothing about that is a threshold to remember — it is a
**marginal** mechanism, sensitive to host load, and it is the same sensitivity the 18:39Z storm
above records. The fix removes the marginality rather than moving the margin.

**The fix: `contract-b-m4.md` §21, B21 — a rate is not a budget.** A `contract-b` amendment that
**does not move the version** (`contract-b/3.5` stood at the time; nothing on the wire changes and a bounded
requester still never exceeds `genomeRequestsPerMinute`), on the §20 precedent. Four bounds, all
of them on the pump and **none on the retry ladder** — `genomeScanPerTick` 2048 examined per pass,
walked **round-robin over a stable order** and resumed where the last pass stopped so no gap is
starved; `genomeScanChunkSize` 256 per acquisition of `a.mu`, which is the yield that keeps the
read loop fed; `genomeRequestsPerTick` 8; `genomeInFlightPerPeer` 8. A resubscribe now forgets its
per-peer in-flight **accounting** and nothing else, and an expired request is reaped from the
small outstanding set rather than when the cursor next passes it, so `genomeRequestTimeoutMs` and
the 1m/5m/30m/6h/daily ladder fall due at exactly the moments they always did.

**What it measured, deployed to the living deployment at 21:08:50Z** (89 s of replay,
`skippedLines=1`, counters continued from the file, `epoch` restarted from 1 — the ordinary shape
of every archive restart, see *Bringing it back after a reboot*):

| | Before | After |
|---|---|---|
| Requests in the widest one-second window | **180**, inside 33–176 ms | **8** |
| Fetch rate achieved | ~175/min | ~175/min — unchanged, the rate cap still binds |
| Longest starvation gap in `recorded a migration` | **6.5 s** | none that ends a session |
| Archive process CPU, steady state | **~100%** of a core | **8.1%** |
| Relay sessions lost | **26** in 30 min | **0** in the first 24 min, `epoch` monotone past 1,750, at a rate that reached **1,221/min** — including six minutes of host starvation that dropped three sidecars seven times (*A controlled reproduction of the 18:39Z storm*) |
| Crossings the ledger is missing | **7,789** of 33,730 | **0** of **14,437** over the twelve minutes after it |

**The CPU line is the one to read twice.** A whole core was going on 64,736 `stat` calls a second
asking a question the pump had already answered — a hash is in `pending` precisely because the
store does not hold it — and the read loop was queued behind every one of them. The bound did not
slow the fetcher down; it stopped it doing 97% of a pass for nothing.

**What it cost, and this is the exact number.** §5.1 means a subscriber that is absent changes
nothing about a migration, so the traffic ran untouched and only the **record** has the gap.
Matched crossing by crossing — every `migrationId` the five sidecars forwarded or took inbound
custody of, against the `MIGRATION` records the ledger actually holds — the flap of 20:22:35Z to
20:52:26Z cost **7,789 crossings, 23.1% of the 33,730 that crossed inside it**, and the 89-second
restart that fixed it cost a further **1,852 of 1,879**. **The incident's total ledger loss is
9,641 crossings**, and they are permanent: the ledger is append-only and nothing replays a
migration nobody recorded.

**The ledger cost is about twice the disconnected time, and that is the lesson in the number.**
The archive was absent for **12.1%** of the flap's span by the relay's own lifecycle lines, and
lost **23.1%** of the crossings. The difference is the starvation: **3,441** of the lost crossings
were copied by the relay in the ten seconds *before* a drop, into a queue the archive had already
stopped reading, and discarded when the relay closed it. **A subscriber that has stopped reading
is already losing records before anything in any log says it is gone.** Counting absent windows
therefore *understates* a starvation incident, and the crossing-by-crossing match is the method
to use — it is exact, and it validates itself: **0 of 17,249** crossings are missing in the twelve
minutes before the flap, **0 of 17,947** in the sixteen-minute lull, and **0 of 14,437** in the
twelve minutes after the fix. **This is now the way to cost any archive absence on this rig**:
join the sidecars' `sidecar: forwarded MIGRATION_PAYLOAD` and `contract B: took custody of an
inbound organism` lines by `migrationId` — the first set is every crossing the five local worlds
sourced, and what the second holds and the first does not is slot 6's — then ask the ledger which
of those `migrationId`s it has. Its own zeros either side of the window are the proof that the
count is a count.

**It cost the map nothing**, through the flap and through the fix: zero sidecar `client gone`,
zero `4004` since 19:00Z on any slot but one (slot 4, once, before the fix), 24/24 `peer_live`,
`pacedDepth` and `heldDepth` 0, `timeoutBounces` 0, and zero `OVERLOADED`, `MALFORMED_MESSAGE` or
`bounceBack=true` in every sample throughout. **This was a new cost of a large backlog and it is
now closed.** The backlog itself is not — it goes on growing, and it belongs to the `genomeGaps`
watch item.

**What both storms change about reading `custodyDepth`, because it is the number an operator will
notice first.** At the ×100 crossing rate the healthy band is no longer the 0–6 of the ×5 era.
Over 31 collector samples and six live polls on the evening of 2026-08-10 it read **4–52 with a
median near 15**, moving that far inside **two minutes** — 18 → 52 → 21 → 29 → 24 → 18 across six
polls twenty seconds apart — because at this rate the map always has organisms in flight.
**Read the direction and the company it keeps, not the magnitude**: `custodyDepth` 26 beside
`pacedDepth` 0, `heldDepth` 0, `timeoutBounces` 0 and 24/24 `peer_live` is one ordinary sample.
The two readings that meant something both had company — **345 with `heldDepth` 17 and only 20 lanes**
in the storm, and **106 with `pacedDepth` 52** at 18:06Z when slot 6 sat at `timeScale` 0 and its
sidecar held what it could not deliver. Both drained by the next sample.

**A third excursion, at 21:25–21:32Z, is the one with a known cause, and its company was churn
rather than a queue.** `custodyDepth` ran **121 → 183 → 311** over four minutes, spread evenly
across all six slots, with `pacedDepth` 0–3, `heldDepth` 0 and `timeoutBounces` 0 — so by the rule
above it was not the *custody* number that meant anything. The company was elsewhere in the same
window, and it is the 18:39Z storm's signature again: see *A controlled reproduction of the 18:39Z
storm* under that item. `custodyDepth` drained **311 → 244 → 218 → 176 → 99 → 21 → 5** over the
five minutes after the load stopped. **So `custodyDepth` at ×100 is a host-load gauge as much as a
traffic one**, and the two lessons compose: read its company, and if the company is peer churn
rather than a queue depth, ask what the box was doing.

### The archive's ledger recovery, 2026-08-10 19:08Z

**The skip-and-recover replay of `e68550b` ran against the living ledger for the first time, and
it did exactly what it was written to do.** The archive was restarted on its own — no game, no
sidecar, no relay, no collector was stopped — to pick up the achieved-speed readout, and that
restart was also the first boot of the repaired code against the splice of 2026-08-08. It
reported the damage and kept everything behind it:

```
level=ERROR msg="archive: the ledger holds line(s) that do not parse; replay SKIPPED them
and kept every record behind them" skippedLines=1 skippedBytes=776 records=3700672
```

**One line, 776 bytes** — the same splice measured offline on 2026-08-09 (*The disk budget*),
now confirmed against the file the deployment is actually writing. `ledgerSkippedLines` is
non-zero on the page and in `ringstat` for the first time; it will stay 1 for the life of this
ledger, which is the point of carrying it.

**The counters jumped, and the arithmetic closes exactly.** `ledgerRecords` went
**3,342,560 → 3,703,832** across the restart, `+361,272`. Against the file:

| | |
|---|---|
| `wc -l` at 19:07Z, before the stop | 3,696,781 |
| the old process's `ledgerRecords` | 3,342,560 |
| its shortfall against its own file | **354,221** |
| lines appended before it stopped | 3,892 |
| records the new replay reached | 3,700,672 — **plus the 1 skipped line, that is the whole file** |
| recorded live since the restart | 3,160 |

`354,221 + 3,892 − 1 + 3,160 = 361,272`. Nothing is unaccounted for.

**And the shortfall splits the way the caveat said it would.** The ledger holds **268,895**
`GENOME` lines, of which **166,214** were appended during the old process's 22-hour run — and a
`GENOME` append is counted at boot replay and never by the live counter, so those 166,214 are
pure counting artifact. The genuine recovery — records behind the spliced line that the pre-fix
boot never reached — is therefore `354,221 − 166,214 − 1 =` **188,006**, and the one damaged
line is permanently lost. The **~197,000** predicted on 2026-08-09 was the conflated figure the
caveat warned about; it read ~9,200 high because that many `GENOME` lines had already
accumulated when it was computed. **The counter and `wc -l` answer different questions, and a
shortfall computed from the two of them is not a loss.**

**Everything downstream came back full, not reverted.** All 24 lane counters rose (map total
1,622,953 → 1,708,450 envelopes, the recovered older migrations); the species aggregate holds
**1,727** ledger species with sightings reaching back to 2026-08-07T05:14Z; `genomeGaps` rose
56,662 → 60,639, because the recovered records brought their own unfetched hashes with them.
Before `e68550b` this same restart would have **reverted** all of it to 01:21 on 2026-08-08.

**What it cost, measured exactly.** The archive was down **100 seconds** — 19:08:42Z to 19:10:22Z
— of which ~93 s was the replay itself (peak RSS ~5.2 GB; see *The status page on the LAN*). The
map was unwatched for that minute and a half, and **1,940 crossings** were never copied to the
archive and are therefore absent from the ledger. That is §5.1 working: a subscriber that is
absent changes nothing about a migration, so the traffic ran untouched and only the *record* has
the gap. **The number is a count, not an estimate, and it replaces the "roughly 1,600–2,300 at the
prevailing 960–1,400/min" first written here** — every `migrationId` the five sidecars forwarded
or took inbound custody of in the window, matched against the `MIGRATION` records the ledger
holds. The method is worth knowing because it is exact and self-validating (0 missing in the nine
minutes before and the ten after), and it is the one *The evening's two session storms* uses; the
arithmetic estimate was right, and only a count can prove a loss. The `epoch` the page shows restarted from 1 and was
back in the hundreds within minutes, which is resubscription and not a new map (*The five local
worlds run headless*).

**Nothing else moved.** `relay.log` shows exactly one `client gone` for `archive-main`
(`reservationKept=false` — a subscriber holds no slot) and one `client connected` 100 s later,
and no other peer line in the window. All five sidecars and the relay kept their original
process ids; 24/24 lanes stayed `peer_live` with `pacedDepth`, `heldDepth` and `timeoutBounces`
0 throughout; and the five sidecar logs recorded zero `OVERLOADED`, zero `MALFORMED_MESSAGE` and
zero `bounceBack=true` across the restart. The far end never noticed.

### The crossing to `contract-b/4.0`, 2026-08-11 — a 7m18s window

**The whole fleet moved wire in one window, because a major is the one thing that cannot be
rolled out at leisure.** `contract-b/3.5` → **`contract-b/4.0`**, `contract-a/2.3` →
**`contract-a/2.4`**, mod `0.6.3` → **`0.6.4`**; the shared LAN token replaced by per-peer
credentials, `ws://` replaced by `wss://` on every client including the loopback ones, and the
path moved from `/contract-b/v3` to `/contract-b/v4`. It ran literally from
`e2e/crossing/RUNBOOK.md`, which now carries the annotations this execution produced. The two
commits are `8e714ba` (the launch material) and `32f41f8` (the rebuilt far-end bundle).

| | |
|---|---|
| Map down | **7m18s** — `down` at **17:12:57Z**, `up` returned at **17:20:15Z** |
| The new relay | started **17:17:10Z**, terminating TLS itself: `scheme=wss path=/contract-b/v4 retiredPaths=[/contract-b/v2 /contract-b/v3] credentials=7 minContractVersion=<none>`. `credentials=7` is the count that proves the mint landed: six `peer`, one `subscribe` |
| The archive | replay **128 s** at **6.24 M records** — against the ~150 s the runbook derived — re-subscribed **17:19:19.040Z** |
| The five sidecars | every one `reason=reclaimed`, on its own coordinate, with **zero discarded journal bytes** |
| The five games | `0.6.4`, `contract-a/2.4`, `timeScale 100` confirmed on all five |
| Slot 6 | **dark**, holding its reservation. Nothing named it in the new relay's log, so nothing on this side has yet observed the far end even attempt the new wire. Its `holdTimeoutMs` — 24 h, after which entries held for it bounce home by themselves — runs from **17:17:10Z**, the moment the new relay started |

**The ledger gap was zero crossings, and that is the finding worth carrying to the next major.**
§5.1 makes any archive absence *while the map is live* a permanent hole in the record — the last
measured restart cost **1,940** crossings and the runbook budgeted ~1,600 for a repeat. Restarting
it inside a full outage costs nothing instead, because there are no crossings to miss: the archive
re-subscribed at 17:19:19.040Z and the **first sidecar was granted 880 ms later**. **Restart the
archive inside the outage** is now a proven cheap pattern and not a theory, and it is the reason
the archive is restarted exactly once in that runbook and why that once is where it is.

**The expected custody burst did not happen, and the reason changes what to expect next time.**
The runbook budgeted the recorded peak of `custodyDepth` **55** / `pacedDepth` **14** draining over
about four minutes. Measured this window: peak custody **4**, paced **0**. The recorded burst came
from a **mod-deploy** cycle in which the *sidecars stayed up* and took custody of everything their
absent worlds could not accept; a full `down` takes the sidecars too, so nothing accumulates
anywhere — each journal replays and flushes the moment its own mod attaches. **A mod deploy inside
a full rig outage costs no custody burst**; a mod deploy with the sidecars left running costs the
55/14 one. Both are now on record, and which one applies is decided by whether the sidecars go
down.

**The time-scale trap did not fire either, and the check is still the deliverable.** *A world can
be at the wrong time scale* says the first `timescale` after a world load answers
`Time.timeScale=1.00` and the reported scale sticks there. This time all five slots already read
`100` before the documented re-send, so the re-send changed nothing. Read the scale; do not
predict it.

**The `down` estimate was the one number that was badly wrong, and the cause is the log archive.**
The runbook's §9 sized `lan_down` at ~15 s. Both `lan_down` **and** `lan_up` re-archive the
BepInEx logs *inside* the outage, and at 2026-08-11's **6.08 GB** of live logs and drvfs read
rates of **24–30 MB/s** that is ~4 minutes cold. It cost **90 s and 65 s** here only because P1's
own archiving pass had already warmed the page cache. The disk side of the same fact is in *The
disk budget* — three copies of 6.08 GB in one window, and the archive directory went 3.4 GB → 21 GB.

**What is durable now that was not before.** `e2e/relay-data-m4-lan/peers.json` — the credential
verifier store, salted SHA-256 verifiers and grants, never secrets — is the relay's **third**
durable file beside `ring.json` and the archive's set, and it was backed up this window to
`~/.multiverse/backup-20260811T170259Z/`. The TLS material in the gitignored `e2e/tls-m4-lan/`
is the fourth thing to keep: **`ca.key` must never be lost**, because a new CA means a fresh trust
import on somebody else's computer.

**The rollback set is still on disk and there is a rule for retiring it.**
`~/.multiverse-rollback-bin/` (the `contract-b/3.5` binaries) and the `0.6.3` DLL copy beside it
**stay until slot 6 has rejoined and the map has run a day on the new wire.** Until then the far
end is the half of this deployment that has not crossed, and the old binaries are the only thing
that could carry it back.

**Two defects in this rig's own tooling were found by running it**, and both are fixed where they
live: `e2e/crossing/mint-credentials.sh` defaulted `RELAY_BIN` to `bin/relay`, which at P0.5 is
still the old binary with none of the new flags, and the runbook's P4 gate asserted a `grep` count
that the patch it applies makes impossible. `e2e/crossing/RUNBOOK.md` carries both corrections, so
the next major inherits a corrected template rather than this one.

### The WP4 + WP5 rollout, 2026-08-11 — a rolling window with no map outage

**The second planned change of the day moved no wire, and that is why it cost no downtime.** WP4
(capacity limits, the admin path, the render deny list, A49/A50's sidecar halves) and WP5
(holes-before-growth, the widening coalescing window, the quiet re-claim) went onto the live map
together, batched exactly as `m5_tracking.md` said to batch them. Nothing in either package
changes a frame's shape, so the relay and the five sidecars were replaced under a running map
rather than around a stopped one.

| | |
|---|---|
| Map down | **none.** The relay was absent for under one second; each sidecar for one to two seconds, one at a time. `/api/status` never left 5/5 live |
| The new relay | started **20:56:15.826Z**, same flags, same `--advertise-url wss://192.168.1.227:8795/contract-b/v4`, `credentials=7`, `relaySessionId=0eab5f24`. Three startup lines are new: the **capacity-limit envelope**, the **`PEER_STATUS` coalescing window**, and the **slot-space** line |
| The five sidecars | rolled one at a time, **20:59:10Z → 21:03:53Z**. Every one `reason=reclaimed`, on its own coordinate, with **zero discarded journal bytes**; slots 2 and 5 replayed one journal entry each and flushed it |
| The games | **not restarted.** No mod change, so no `deploy.sh`, no `archive_bepinex_logs`, no time-scale re-send. Each slot's mod redialled Contract A on its own ladder and `modConnected` came back in **4 s to 20 s** |
| The archive | **not restarted**, deliberately — the deny list is an optional flag for a later batch and A49's ledger reason is sidecar-authored. It kept its session across the relay restart and its pid across the whole window, so the ledger has **no gap** |
| Slot 6 | **dark at both ends of the window.** The owner's far-end trip has not happened |

**A relay restart is not an outage when the sidecars stay up, and the measured gap is under a
second.** All five sidecars reclaimed between **20:56:15.945Z and 20:56:16.474Z** — inside 650 ms
of the new relay's listen line, so not one of them reached the first rung of the 1 s–30 s
reconnect ladder. Because the sidecars are *not* restarted in this phase each holds exactly one
connection, so `maxConnectionsPerPeer` **2** is never approached and the hard-kill trap that WP4's
rollout note warns about does not arise. The transient is a second of `4005 relay draining`,
`NO_ROUTE` and *holding a forwarded organism whose destination went dark* on local destinations,
all of it drained by the next sample. Contrast the crossing's 7m18s: **what costs the map time is
a wire change, not a binary change.**

**The three new startup lines are the deliverable, because a limit an operator cannot read in
their own log is one they read off a peer's `4007`:**

```
msg="relay: capacity limits, as this relay is running them (contract-b-m4.md §3.3)"
  limits="map[maxBytesPerSecond:4194304 maxClaimsPerMinute:12 maxConnectionsPerAddress:8
  maxConnectionsPerPeer:2 maxFrameBytes:8388608 maxFramesPerSecond:50
  maxGenomeRequestsPerMinute:30 maxSubscribers:4]"

msg="relay: the PEER_STATUS coalescing window, and the broadcast rate it bounds (§7.2, §22 B29)"
  statusCoalesceMs=250 statusCoalesceMaxMs=2000 statusChurnBurstThreshold=8
  broadcastsPerMinuteAtRest=240 broadcastsPerMinuteUnderAStorm=30
  costPerBroadcast="slotCount stats blocks to every peer AND every subscriber"

msg="relay: slot space (contract-b-m4.md §7.2, §7.5)"
  maxSlotEverIssued=6 map="{Width:3 Height:2}" slotCount=6 holes=[]
```

**`maxSlotEverIssued=6` on a six-slot map is WP5's own invariant read off the living deployment**
— the address counter has never exceeded the placements, which is what the churn harness proved to
exhaustion and what this line lets an operator confirm without one.

**The published limits are readable in the relay's log and nowhere else on this rig, and that is
expected rather than a fault.** The running archive predates WP4, so `/api/status` carries no
`limits` key and neither the page nor `ringstat` shows the table. The sidecars do not log it
either. That visibility arrives with the next archive restart — batch it with the deny list; do
not restart the archive to see a number the relay already printed.

**`--admin-listen` was deliberately not passed, and the admin listener therefore bound nothing.**
B28's path is compiled into this relay and is off: an empty `--admin-listen` returns before it
looks at anything, and even with the flag it refuses to bind on a relay holding no `admin`
credential. Turning it on is a separate, owner-level act with its own credential mint.

**The `PEER_STATUS` cadence fell 20.6%, and the honest reading is that this is the FLOOR of what
WP5 buys rather than its measure.** Measured off `/api/status`'s `epoch`, which is a per-connection
count of the broadcasts the archive received and therefore the broadcast rate exactly: **55.4 a
minute before** (20:49:26Z–20:56:08Z, 371 epochs in 402 s) and **43.9 a minute after**
(21:04:02Z–21:16:18Z, 539 epochs in 736 s). The reason it is not larger is that **the peer B29's
quiet re-claim was written for is not on the map**: DQ3's 64 re-claims in a day came from slot 6 as
its measured time scale wandered, and slot 6 is dark. The five local slots claim a handful of times
an *hour* — 25 claims in this whole window, every one structural and every one correctly
broadcast — so what remains is the stats-block term, one `pending` per §14 B4 stats update off a
PING, which WP5 does not bound and does not intend to. **Re-measure when slot 6 rejoins**; that is
the reading the amendment is about.

**Nothing else moved.** `modVersion` **0.6.4** and `contractAVersion` **contract-a/2.4** on all
five throughout — nothing mod-side was touched. Lanes were identical before and after: **18/20
open**, slot 3's north and south closed `no_peer`, four bypasses — the shape a hole at (2,1) makes.
`heldDepth` **0** and `timeoutBounces` **0** on every slot in every sample; `custodyDepth` ranged
1–27 and `pacedDepth` 0–7 and both fell every time they rose. Zero `level=ERROR` lines on the Go
side, zero `4007`s, and **A50's partial-case WARN fired nothing** — on a full 3×2 every declared
edge lies on an axis the map has, which is exactly what it should say. `genomeGaps` read **136,628**
at a ledger of 6.87 M and grew ~**72 a minute** across the window with no drain, which is another
sample for its watch item and moves nothing about the finding.

**The sidecar changed, so the bundle was re-taken — and this time the trip carries everything in
one go.** `farend/dist/farend-bundle.zip` was rebuilt at 21:04Z: **6,637,894 bytes**, sha256
`11e58d7e2862f886918739d4667ce26df72e0e4fe7084f508c39657e7dcc2c40`. It carries the **new**
`multiverse-sidecar.exe` (`63b1de8b…`) and the **same** `BibitesMultiverse.dll` (`fae0d50c…`, mod
`0.6.4`) that is deployed here — byte-identical to the plugin in the game's own `BepInEx/plugins/`,
which is the check the bundle rule asks for. `deploy.sh` was not run and no game was stopped: the
mod did not change. **Because the owner's trip has not happened yet, there is no version debt** —
whenever it does, it delivers the latest sidecar and the latest bundle together. Had the far end
already crossed, it would now be one sidecar version behind and owed a later application at its
operator's leisure; nothing in WP4 or WP5 moves the wire, so either way it is wire-compatible.

**The rollback set for this window is NOT `~/.multiverse-rollback-bin/`.** That set is the
`contract-b/3.5` binaries and it predates the crossing, so restoring it would take the map back
across a major. The pre-rollout `contract-b/4.0` binaries this window replaced were copied aside
first, to **`/mnt/wsl/data/bibites-multiverse-prewp45-bin/`** (`relay`, `sidecar`, `archive`,
`multiverse-sidecar.exe`) — that is the set a bad relay rolls back to, and it retires on the same
rule the crossing's set does: when the map has run a day on the new binaries.

**One thing on this rig has been dialling the relay in plaintext since the moment TLS went up, and
it stopped by itself before this window.** `relay.stderr.log` holds 809 `TLS handshake error …
client sent an HTTP request to an HTTPS server` records from **172.24.96.1** — the Windows host
side of the WSL boundary, which is also what the `8795` portproxy rewrites a LAN source to — at
roughly one every 15 s from **17:17:22Z**, twelve seconds after the crossing relay started, until
**20:45:52Z**. The cadence is a client on a fixed retry ladder and the wire is `ws://`, which is
what a **pre-crossing slot 6** would do; a TLS handshake never names a peer, so this is consistent
with the crossing record's "nothing has yet observed the far end attempt the new wire" rather than
a contradiction of it. It ceased ten minutes before this window opened and has not resumed under
the new relay. **Treat it as evidence about the far end and not about this machine**, and expect it
to stop for good once the far end takes the bundle. The same file holds **two** unexplained
plaintext dials from **127.0.0.1** at 21:14:09Z, inside this window and from nothing this window
ran; nothing on the rig dials `8795` over `http` — the collector reads only `8796` and every rig
helper uses `https://…/healthz` — so they are noted and not accounted for.

### The two sidecar-only rolls of 2026-08-12

**Two more rolling windows in the small hours, both sidecar-only, and neither cost the map a
moment.** The relay, the archive, the five games and the mod were untouched by both, so neither is
in the class of the crossing: what a sidecar-only roll costs is one to three seconds per slot, one
slot at a time, and the pattern is now the WP4 + WP5 window's with the relay step removed. The
first carried the outbound pacing fix (`9b1592d`); the second carried WP7's `--diagnose` and
`--my-slot` (`385991f`). **Slot 6 was live at both ends of both windows and was not touched** — the
far end crossed on the evening of the 11th, and rolling it is its own operator's act.

**The pacing roll, 02:20–02:23Z.** All five local sidecars onto the paced outbound sender, zero
discarded journal bytes, 24/24 lanes throughout. Its measure is the thing that stopped: the last
capacity shed anywhere on this rig is slot 1's at **02:15:37Z**, five minutes *before* the roll
opened, with slot 5's run of seven ending at **01:43:54Z** — and there has been **none since**,
through the WP7 window and the observation after it. The defect, its
fleet-wide evidence and the far-end debt it left are in `m5_tracking.md`'s standing watch items;
what belongs here is only that the deployment took it and stayed up.

**The WP7 roll, 04:28:22Z → 04:34:14Z.** Five slots, one at a time, and nothing else.

| | |
|---|---|
| Map down | **none.** `/api/status` read 6/6 live and 24/24 lanes at every check; only slot 5's own stats block was briefly absent, for the one sample between its grant and its next `PEER_STATUS` |
| The five sidecars | every one `reason=reclaimed`, on its own coordinate, with **zero discarded journal bytes**. TERM to grant: slot 2 **1.8 s**, slot 3 **2.9 s**, slot 4 **2.9 s**, slot 5 **3.4 s** — and slot 1 **2m 05s**, which was operator error and not the build (below) |
| The new gates | `<data-dir>/listen.addr` **and `<data-dir>/sidecar-process.json`** present on all five after restart, the second carrying pid, start time, peer id and listen address; `--my-slot` answered each slot's real state, exit 0 on all five; every `/proc/<pid>/exe` digest **SAME** as `bin/sidecar` with no `(deleted)` |
| The pacing line | present at every reconnect, `pacedFramesPerSecond=25 pacedBurstFrames=12` against a published ceiling of 50 — the previous roll's change, read back on this one |
| The games | **not restarted.** No mod change, so no `deploy.sh` and no time-scale re-send. `modVersion` **0.6.4** and `contractAVersion` **contract-a/2.4** throughout |
| The archive and the relay | **not restarted**, deliberately. `ringstat.go` changed under `385991f`'s termsafe refactor and is behaviour-identical, so the archive and `ringstat` wait for their own batched restart — taken later the same day (*The batched debt window*). WP4's `limits` table was expected to ride with it and **did not**: it turned out never to have been implemented, and that window records why |
| Observation | 11 samples at 30 s, 04:34:48Z–04:40:22Z: `heldDepth` **0** and `timeoutBounces` **0** in every one; `custodyDepth` 3–20 and `pacedDepth` 0–5, both falling every time they rose; ~990–1 070 migrations a minute; population 368–432. **Zero** new `level=ERROR`, **zero** sheds and **zero** discarded bytes on all five sidecars and on the relay |

**The one thing that went wrong was the restart environment, not the binary, and it is the lesson
of the window.** Slot 1's replacement was launched with the running process's argv but not its
environment, and the rig's CA trust is an *environment* variable — `export
SSL_CERT_FILE="${SSL_CERT_FILE:-$TLS_CA}"`, `e2e/run-m4.sh` line 249, pointing at
`e2e/tls-m4-lan/ca.crt`. Without it the sidecar dialled the relay, refused the certificate and said
so exactly as B23 requires: `contract B: the relay's TLS certificate did not verify; NOT CONNECTING`,
naming the remedy and stating that it would not skip verification, pin a certificate or fall back to
`ws://`. It then sat on the reconnect ladder for **2m 05s** and eleven `ERROR` lines, all of that one
message, until it was relaunched with the environment replayed from a live sibling's
`/proc/<pid>/environ` — after which it reclaimed slot 1 at (0,0) on the first attempt. **Restarting
one sidecar by hand means replaying `/proc/<pid>/cmdline` *and* `/proc/<pid>/environ`**; the argv
alone looks complete and is not. Nothing was lost: zero discarded bytes across the whole slot-1
disturbance, and the journal replayed its two inbound entries and flushed them.

**`--diagnose` now answers its live half against the running deployment, and that is the whole
point of the roll.** Run against slot 2 before the roll — the old build listening, with no
`/my-slot` — it returned **6 pass, 1 fail, 0 warn, 13 unknown, 1 skip**, every unknown reading *a
sidecar is listening at 127.0.0.1:8788 and does not serve `/my-slot`, so it is a build older than
this one*. Run after, the thirteen collapsed to three: **15 pass, 1 fail, 0 warn, 3 unknown, 2
skip**. The three that remain are honest gaps by design and not this rig's fault — `time-scale` and
`disk-headroom` are WP8's and WP3's bands to publish, and `game-version` wants a
`support-matrix.json` that only a packaged install has.

**Read the flags before reading the verdict: a bare `--diagnose` diagnoses the configuration it is
given, and on this rig that is not the configuration the rig runs.** The bare form's single `FAIL`
is `credential`, and it is correct — no `--credential-file` was passed, so there was no credential
to present — while `relay-tls` skipped because the *default* relay address is `ws://`. Given the
five sidecars' own flags and environment the same build reports **17 pass, 0 fail, 0 warn, 3
unknown, 1 skip, exit 0**, with `relay-tls` verifying the relay's certificate against
`SSL_CERT_FILE` and `credential` reporting the session it actually holds. This is
`docs/sidecar-diagnose-spec.md` §1 working as written — *the check reports on the configuration it
is given* — rather than a defect, but an operator who runs the short form on this rig will read a
`FAIL` that says nothing about the deployment:

```sh
SSL_CERT_FILE=e2e/tls-m4-lan/ca.crt bin/sidecar --data-dir e2e/data-m4-lan/slot-2 \
  --relay wss://127.0.0.1:8795/contract-b/v4 \
  --credential-file ~/.multiverse/peer-slot-2.secret --diagnose
```

**It disturbed nothing, and the evidence is the session it was asked about.** Across both
`--diagnose` runs, slot 2's `pid`, `startedAtMs`, `connectedSinceMs`, `capacityShedTotal` and
`relaySessionId` — `0eab5f24-40f9-4dcd-be06-1b44e4ee28f9` — were **identical** before and after, and
`sidecar-2.log` gained **no** session-ended, claim, error or shed line across the window. The one
write the specification allows is the temporary file in the data directory, and the relay saw one
TCP connect that closed without sending anything.

**A narrow shutdown race showed up on slot 4, once, and it fails in the safe direction.** A
`MIGRATE_OUT` arrived from the mod **45 ms after** `sidecar: shutting down`, when the journal was
already closed: `contract A: journal write failed … err="file already closed"`, answered with
`MIGRATE_OUT_NACK code=JOURNAL_ERROR`. A NACK means custody was never taken, so the organism stayed
in slot 4's world — nothing was lost and nothing was duplicated. The NACK itself could not reach the
mod, because Contract A was already closing, which is also safe: the mod removes an organism only on
an ACK. It is the first occurrence in every sidecar log this rig still holds, and it is the designed
`AOUT-JOURNAL_ERROR` path rather than a new fault.

**`ps` understates process age on this host, and the roll had to prove the relay's identity another
way.** `ps -o etime,lstart` put the relay's start at ~21:42Z and the archive's at ~22:24Z; both are
wrong, because the jiffies clock stops while the Windows host sleeps and every process started
before a suspend therefore looks younger than it is. The relay's real start is **20:56:15.826Z** —
its `relay: listening` line, the mtime of `e2e/run-m4-lan/m3-relay.pid`, and the
`relaySessionId=0eab5f24` that every live peer still reports, all agreeing. **Age a process by its
own startup line and its pid file, never by `ps`**; this is the same family of trap as the
`/proc/<pid>/exe` inode comparison that `e2e/crossing/RUNBOOK.md` P6.2 already documents.

**The bundle was re-taken, and the far end still owes exactly ONE swap.**
`farend/dist/farend-bundle.zip` is now **6,807,249 bytes**, sha256
`4ab0f47683b3a1ea0fb34757247b7199e6a8bdebee45f2e13a85e36de8e22bde`. It carries the new
`multiverse-sidecar.exe` (`ac14bb69…`, was `28d2c7e6…`) and the **same** `BibitesMultiverse.dll`
(`fae0d50c…`, mod 0.6.4), verified byte-identical to the plugin in the game's own
`BepInEx/plugins/`. Slot 6 is one sidecar version behind and **wire-compatible** — neither roll
moved a wire field — so its operator applies it at their leisure. **The two rolls do not compound
into two trips**: one swap now delivers pacing, `--diagnose` and `--my-slot` together, and until it
happens slot 6 remains the one peer that will burst on a rejoin.

**The rollback set for this window is `/mnt/wsl/data/rollout-wp7-20260812/bin-rollback/`** —
`sidecar` (`4857ded4…`) and `multiverse-sidecar.exe` (`28d2c7e6…`), the pacing-roll binaries these
replaced. It is not `bibites-multiverse-prewp45-bin/`, which is a roll older, and not
`~/.multiverse-rollback-bin/`, which is pre-crossing. It retires on the usual rule: when the map has
run a day on the new binaries.

**The far end took its owed swap on 2026-08-12 ~09:17Z, and Windows took the stop half without
asking.** An overnight reboot (03:04:48 local; both processes gone at 03:00, the sidecar logging
`shutting down` then `shutdown timed out` as Windows killed it after 5 s) had slot 6 dark for
2 h 17 m before the operator's window opened — so the swap itself was a file copy over a stopped
install, not a stop/start, and the far machine auto-starts nothing after a reboot: a dark slot 6
on a quiet morning is Windows Update until proven otherwise. The bundle verified (`4ab0f476…`),
the exe it carries verified in place
(`ac14bb691603eb7f61757e0f0dd1d1954cd35f53a381c4a3485d37ae35aab374`), and what it replaced was
**`63b1de8b…`** — the 01:28Z WP4/WP5-without-pacing build — kept beside the install as
`multiverse-sidecar.exe.pre-pacing.bak`, which is the far end's rollback now. On start the new
sidecar was granted `slot=6 reason=reclaimed position={Col:2 Row:1}` within a second, recovered
custody `outbound=0 inbound=5` with **zero discarded bytes** (the damage line is an ERROR that
prints only when `discardedBytes>0` — `go/internal/sidecar/sidecar.go` — so its absence is the
pass), and printed the line the swap was for: `pacing this sidecar's own outbound frames under the
relay's published ceiling` with `publishedFramesPerSecond=50 pacedFramesPerSecond=25
pacedBurstFrames=12`. `--diagnose` read the live connection through the data dir — peak 12
frames/s against the ceiling of 50, 0 bulk frames deferred, every declared edge open to a live
peer — and `--my-slot` renders the slot, the custody depths and the whole map from the same door.
The mod reattached by itself every time the sidecar returned (~25 s to `contract A: mod
connected`), and the relay re-pushed every un-ACKed payload into `duplicate MIGRATION_PAYLOAD,
delivering nothing`: exactly-once held across three sidecar generations in one morning. One
gotcha re-proved: the world restarts at its persisted default timescale, so `timescale 6.5` was
re-sent and confirmed via the cmd file after the game came back.

**A rejoin after hours dark spends its first minute purging, and the relay will drop it once for
that.** The first reconnect purged **23,974 expired tombstones** in one synchronous sweep, the
connection went quiet for 52 s, the relay closed it, and the second connect re-granted
`reason=reclaimed` and stayed. Read the pattern — `slot granted`, silence, `send failed:
connection closed`, `slot granted` again — as the purge, not as a 4007 shed: the pacing build
never bursts, and the journal is untouched by it.

**The installed far-end scripts cannot swap a sidecar under a running game; that gap is plugged
locally and still owed upstream.** `stop-slot6.ps1` stops the game unconditionally before its
`-GameOnly` branch is consulted, and the full `start-slot6.ps1` starts a second game against one
already running. The swap ran hand-mirrored sidecar-only halves instead, installed beside the
originals as `start-sidecar-only.ps1` / `stop-sidecar-only.ps1` in
`C:\Users\j_jor\multiverse-farend\`. `setup-farend.ps1` should grow the same pair so the next far
end gets them from the template.

**A fixed archive name plus `-Force` destroyed 107 MB of history in one line.** The first
sidecar-only start archived the pre-swap log to a literal `…-20260812-preswap.log`; run again the
same morning, its `Move-Item -Force` replaced that archive with the two-minute log of the
generation in between. The 03:00 reboot evidence survives (quoted above, and
`sidecar-slot6-20260812-swapproof.log` holds the swap-validation generation); the Aug 11
21:27 → Aug 12 03:00 bulk does not. **Generation-stamp every archive name and never pass
`-Force` to an archive move** — the installed script now does the former and omits the latter.

**`phase5far` cannot run as written against today's map, and arming it is how we know.** Its dam
and drain arithmetic hardcodes the OLD §15 A20 pair — burst 5, 2.0 per simulated minute — which
`run-m4.sh` phase 6 keeps honest locally by pinning those numbers on its dam slot with
`--inbound-rate`/`--inbound-burst` **and** slowing every sim first. Neither survives contact with
the live map: slot 6 measured **132.5 inbound/min at 6.5×** (168 arrivals in a 76 s steady
window, 09:32–09:34Z), so even with every sender at 1× the ~20/min that remains is ten times the
drain bound's release rate, and the sidecar's 64-entry inbound queue trips OVERLOADED mid-phase. A
two-minute pin rehearsal (09:29–09:31Z, rate 2.0 burst 5, world at 1×) dammed 33 organisms in
45 s — the §9.4 mechanism, observed cross-machine — and was reverted before the cap; slot 6 runs
the shipped defaults again. The honest paths from here: **(a)** parametrise the phase's rate,
burst and bound (`FAR_PACE_RATE`/`FAR_PACE_BURST` in `allowed(t)`), calibrate them against the
measured slowed-sender arrival rate, pin slot 6 the way phase 6 pins its dam, and finally run the
thread out; or **(b)** leave the carried thread open and record that today's reboot supplied the
dark-and-rejoin half unobserved — route-around held (the map ran 18/20 `peer_live` shapes while
slot 6 was dark before, and did again), custody survived with zero discarded bytes, and the drain
was paced by the very build the swap installed. The choice is the operator's; nothing about it
blocks M5.

### The batched debt window, 2026-08-12 — one pause, every banked rollout

**The window the two sidecar-only rolls kept deferring to: the archive and the relay restarted
together, inside a deliberate pause of the five local sidecars, so that the archive's replay cost
the ledger nothing.** It carried B26's forward receipt (the relay emits, the sidecar records) and
the whole archive batch — gzip on the HTTP surface, the streamed ledger replay, and the
`ledgerRecords` counter fix, plus two *capabilities* that arrive switched off on this rig: the
render deny list (`--deny-list`) and the genome-retention horizon (`--genome-horizon`). The five
games and the mod were untouched; slot 6 is the owner's and was not touched either. **One item the
window was expected to carry was not in the tree at all** — WP4's `limits` table; see below.

| | |
|---|---|
| Crossings paused | **5m 21s** — no local sidecar was up between **11:28:39.1Z** and **11:33:59.7Z**. The worlds simulated throughout; the mods closed their edges and held |
| The archive | down **11:29:38.2Z → 11:32:07.5Z (2m 29s)**, entirely inside that pause. Replay of **8,441,773 records / 2.7 GB** took **148 s** |
| Replay memory | **VmHWM 1,599,064 kB (1.53 GiB)** against the outgoing process's **8,244,952 kB (7.86 GiB)** — the streamed replay is a **5.2× cut in peak**, and it cost no wall time at all: **57 k records/s**, against the crossing's 128 s for 6.24 M records (**49 k/s**) on the old model |
| The ledger gap | **ZERO, and not by a narrow margin** — see the accounting below |
| The relay | TERM to healthy in **0.9 s**; slot 6 reclaimed **0.4 s** after the listener came up |
| The five sidecars | every one `reason=reclaimed`, own coordinate, **zero discarded journal bytes**, pacing line present (`pacedFramesPerSecond=25 pacedBurstFrames=12`). Start to grant: **0.8 s, 0.8 s, 1.9 s, 1.8 s, 1.6 s** |
| Observation | 44 samples, 11:37:32Z–11:49:32Z: **24/24** lanes and **6/6** live in every one, `holes` **0**, `heldDepth` **0** and `timeoutBounces` **0** throughout; `custodyDepth` 3–37 and `pacedDepth` 0–7, both falling every time they rose; population 340–442; ~2,290 ledger records a minute. **Zero** new `level=ERROR` and **zero** sheds on the relay and all five sidecars — the archive's single `ERROR` is the old damaged ledger line, re-reported at replay |

**The zero-gap accounting is the point of the pause, and it is an argument about what *could* have
happened, not about how fast the archive was.** With all five local sidecars down and slot 6's
sidecar left up but with no live neighbour, **no crossing can complete**, so the archive's outage
had nothing to miss. The ledger says so directly: the last record of any kind before the archive
stopped was a `GENOME` at **11:28:47.0Z** — **51 s before** the TERM — and the first after it
returned was a `GENOME` at **11:33:16.2Z**, **69 s after** it re-subscribed. The last real crossing
before the window was `MIGRATION` slot 6 → 5 at **11:28:40.5Z**; the first after was slot 2 → 1 at
**11:35:06.4Z**, once both ends of that lane were back. The archive's outage sits inside a hole in
which nothing was written *because nothing could be*, with **59 s of margin before and 112 s
after**. This is the crossing's result re-proved on a ledger a third larger, and it is now the
pattern for any archive restart: pause the senders first and the gap is empty by construction.
**The counter-example is in this file**: the unpaused restart of 2026-08-10 (*The archive's ledger
recovery*) cost the ledger **1,940 crossings** in 100 seconds, because the map went on crossing
while its only subscriber was away. Those two windows are the same operation with and without the
pause, and the pause is cheap — the worlds never stop simulating, they only stop *exchanging*.

**`ledgerRecords` and `wc -l` finally answer the same question, and the counter JUMPED because of
it.** Before the window the page read **8,294,368** against **8,415,926** lines — **121,558 low,
1.44%** — because the startup replay counted `GENOME` lines and the live path did not, so the
counter drifted below the file from the first genome fetch after every boot. **This is the artifact
named on 2026-08-10** (*The archive's ledger recovery*, which measured 166,214 `GENOME` lines of
pure counting artifact inside that restart's apparent shortfall); what was diagnosed there is
fixed here, and a shortfall computed from the counter and `wc -l` is now a real one. With the map paused
the ledger is frozen, which makes the reconciliation exact rather than a race against an appending
file: **`wc -l` 8,441,774, `ledgerRecords` 8,441,773, `ledgerSkippedLines` 1, difference 0.** The
jump is the fix landing, not new records. The one skipped line is old, permanent damage in an
append-only file and is *the whole* of the difference — it is also the window's only `level=ERROR`
line, and it is the archive saying so on purpose.

**Gzip is negotiated, and only negotiated.** `/api/status` answers `Content-Encoding: gzip` with
`Vary: Accept-Encoding` to a client that asks, **1,687 bytes against 15,430 — 9.15×** — and plain
identity JSON with the same `Vary` to one that does not. `/healthz` stays identity in both cases:
it is three bytes, well under the 1,400-byte floor the compressor commits above.

**The genome horizon is OFF here, and the page proves it by omission.** `genomeHorizonMs` is
**absent** from `/api/status`, as are `genomesEvicted`, `genomesEvictedBytes` and
`genomeGapsExpired`. That is the contract default and §10.1's rule that unknown is a value: an
absent horizon is how a reader knows nothing is being pruned. The rig was restarted through the
launch path with `ARCHIVE_HTTP=0.0.0.0:8796` preserved and **no** new environment, so
`MULTIVERSE_GENOME_HORIZON` is unset. The hosted deployment is where `720h` goes on.

**B26's receipt closed its loop on the first traffic, and the journals are the evidence.** Every
one of the five sidecar journals held **zero** lines carrying `receiptSessionId` before the window
and hundreds within ten minutes after it — **slot 1: 460, slot 2: 360, slot 3: 488, slot 4: 264,
slot 5: 112** — each carrying `forwardReceipts`, `receiptDestSlot`, `receiptForwardedAt` and the
new relay's own `relaySessionId` `27005d53-02f0-4447-985f-67b0d03e2c86`. The session is the
load-bearing field: a receipt is a fact about *this* relay generation. Nothing is logged on the
happy path by design — a single receipt is journalled silently and only a *second* one under the
same `migrationId` says anything — so **the journal, not the log, is where a receipt is read.**

**The rollout order was honoured by the shape of the window rather than by sequencing.** B26 is
safe in a mixed fleet both ways, but a pre-B26 sidecar logs one unknown-type warning per outbound
migration once the relay ships receipts, so the order is sidecars first. Here the five local
sidecars were *down* while the receipt-emitting relay started and came back on the new build, so no
old local sidecar ever met a receipt.

**Slot 6 did meet one, and its warning rate is larger than "harmless" — because the far end has no
log rotation.** Its sidecar is pre-B26 against a receipt-emitting relay, so it now logs
`contract B: ignoring unknown type type=FORWARD_RECEIPT` once per **outbound** migration. Measured
on the live map, slot 6 sends **158.8 hops/min** out of its four lanes, so that is **~159
warnings/min ≈ 107 B each ≈ 0.97 MB/hour, 23 MB/day, 0.68 GB per 30 days**. The assumption that
this is bounded by log rotation **does not hold at the far end**: `setup-farend.ps1` starts the
sidecar with `-RedirectStandardError` to a plain file and passes **no** `--log-file`,
`--log-rotate-mb` or `--log-keep`, so slot 6's `sidecar-slot6.log` grows without limit and will
bury its own diagnostics before it fills anything. It is not urgent and it is not silent: the
single pending trip fixes it, because the rebuilt bundle carries the B26-aware sidecar. Until then,
read slot 6's log from the tail. **`setup-farend.ps1` should pass the rotation flags the local rig
has always passed** — the same template gap as the missing sidecar-only start/stop pair.

**WP4's `limits` table did NOT arrive with this window, because it is not built.** The WP7 roll
recorded that the `limits` table was waiting on the archive's batched restart; this *was* that
restart, and the table is still absent from `/api/status`. The reason is not the rollout: the
relay publishes its capacity table on the wire and logs it at startup (`maxFramesPerSecond:50`,
`maxBytesPerSecond:4194304`, and six more), and `contractb.PeerStatus` carries a required `Limits`
field that the archive *stores* — but the archive's `StatusView` has no `limits` member and the
`archive` package contains **no reference to `Limits` at all**. Nothing was deferred and nothing
regressed; the exposure was simply never implemented. It needs a field on `StatusView`, populated
from the stored `PeerStatus`, and it will cost another archive restart — which now means another
paused window, so it should ride with the next archive change rather than alone.

**The bundle was rebuilt so the one pending trip carries HEAD, and the far end still owes exactly
ONE swap.** `farend/dist/farend-bundle.zip` is now **6,814,671 bytes**, sha256
`01284f64f7ddc7ecb4e79dee9c065a66fdf4f6d21baa1f1278bcf6bc0e18e207`, carrying
`multiverse-sidecar.exe` `cae0f90b…` (was `ac14bb69…`) and the same `BibitesMultiverse.dll` at mod
`0.6.4`. **The rolls still do not compound into separate trips**: one swap now delivers pacing,
`--diagnose`, `--my-slot` *and* B26 together, and it is the swap that ends slot 6's warning stream.

**The rollback set for this window is `/mnt/wsl/data/bdw-2026-08-12/rollback-bin/`.** It holds the
whole outgoing set — `relay` (`89a12c97…`), `sidecar` (`1870ef6f…`), `multiverse-sidecar.exe`
(`ac14bb69…`), `fakemod`, `ringstat`, `worldstat` — plus **`archive.RUNNING` (`e8b13a3d…`), copied
out of `/proc/<pid>/exe`**, which is the archive that was actually running: `bin/archive` had
already been replaced by a later build than the live process, so the file in `bin/` was *not* the
rollback. **Copy a live binary from `/proc/<pid>/exe`, not from `bin/`** — on a rig where builds
land between restarts they are not the same thing. It retires on the usual rule, when the map has
run a day on the new binaries.

**One operational trap cost a minute of the pause: the rig cannot be sourced as a library through
its `status` verb while the map is down.** `source e2e/run-m4-lan.sh status` shells out to
`game.sh status`, which greps the multi-hundred-megabyte BepInEx logs across the Windows 9p mount;
with the sidecars paused it had not returned after a minute. **`statuspage` is the read-only verb
to source through** — one curl to the local archive, 0.5 s — and it leaves every function
(`start_relay`, `start_archive`, `start_sidecar`, `kill_pid`) defined just the same.

**The far end took the B26 trip on 2026-08-12 ~11:56Z, and the warning stream is over.** The
rebuilt bundle verified against its published sha256, the exe it carries
(`cae0f90bb75df6869951bdcfafc512131d675775adf800e3c3ba40543fba2518`) verified before and after
the copy, and the swap ran the sidecar-only pair the morning's first trip installed — the game
(headless) never restarted, the mod reattached by itself, and the world's time scale was
untouched because only a game restart resets it. Reclaimed at (2,1) in ~0.1 s, custody recovered
`outbound=5 inbound=0` with zero discarded bytes, pacing engaged at 25/50. The two things the
trip was for, both confirmed against live traffic: **zero** `ignoring unknown type` lines across
the first 21 outbound migrations (the pre-B26 sidecar logged one per migration, ~159/min), and
**26 `receiptSessionId` entries in `data-slot-6/journal/journal.log`** — the receipt loop closes
at the far end too, read from the journal as designed. The outgoing exe is kept beside the
install as `multiverse-sidecar.exe.pre-receipts.bak` (`ac14bb69…`), the far end's rollback for
this window. The warning-heavy log generation (43.9 MB grown in ~2.5 h without rotation) is
archived generation-stamped as `sidecar-slot6-20260812-075624.log`; the far end still has **no
log rotation** until `setup-farend.ps1` grows the flags, so that debt stands.

### The archive-views window, 2026-08-12 — the published limits and the genealogy tree go live

**The same pattern as the debt window above, at a quarter of its scope: only the archive package
had moved, so only the archive was restarted, and the five local sidecars were paused for the one
reason that they are ever paused — to make its ledger gap zero by construction.** It carried the
two views that landed after that window: **B24's published limits and B25's floor on
`/api/status`** (`e6d7911`, the exposure the debt window found had never been built) and the
**derived genealogy of everything alive on `/api/species/tree`** (`41eef26`). The relay was **not**
restarted, so its `relaySessionId` is unchanged and B26's receipts never broke stride; the five
games, the mod and slot 6 were untouched.

| | |
|---|---|
| Crossings paused | **4m 57.0s** — no local sidecar was up between **16:23:20.107Z** and **16:28:17.151Z**. The worlds simulated throughout |
| The archive | down **16:23:20.208Z → 16:26:26.497Z (3m 06.3s)**, entirely inside that pause. Replay of **8,873,505 records / 2.96 GB** took **186 s** |
| Replay memory | **VmHWM 1,716,880 kB (1.64 GiB)**, against the outgoing process's 2,214,924 kB — the streamed profile, on a ledger 5% larger than the debt window's |
| Replay speed | **47.8 k records/s against that window's 57 k/s** — 16% slower per record. See below; this is the tree's price |
| The ledger gap | **ZERO** — see the accounting below |
| The five sidecars | every one `reason=reclaimed`, own coordinate, **zero discarded journal bytes**, mod back, pacing line present (`pacedFramesPerSecond=25 pacedBurstFrames=12`). TERM to exit: **0.15 / 0.13 / 0.09 / 0.13 / 0.08 s**. Start to grant: **1.23 / 1.74 / 0.76 / 1.46 / 1.42 s** |
| Observation | 20 samples, 16:28:51Z–16:33:29Z: **6/6** live and **24/24** lanes in every sample but the first, taken 28 s after the resume with the edges still reopening (12/16); `holes`, `heldDepth` and `timeoutBounces` **0** throughout; `custodyDepth` 2–38 and `pacedDepth` 0–14, both falling every time they rose. **Zero** new `level=ERROR` and **zero** sheds across the relay, the five sidecars and the archive — the archive's single `ERROR` is the old damaged ledger line (`skippedLines=1`, `skippedBytes=776`), re-reported at replay |

**The zero-gap accounting, and why its margins are thinner than the debt window's.** The last
ledger record of any kind before the archive stopped was a `MIGRATION`, slot 6 → slot 3 on its
north edge, at **16:23:20.157Z** — **51 ms** before the TERM and **58 ms after the last local
sidecar had already exited**, which is exactly what an in-flight frame looks like at a pause
boundary. The first record after the archive returned was a `NACK` at **16:26:35.900Z**, **9.4 s**
after it re-subscribed; the first `MIGRATION` was slot 5 → slot 6 at **16:28:37.500Z**, once both
ends of a lane were back. The ledger file was **byte-frozen at 2,964,162,941 bytes** for the whole
outage. So the margins are **0.49 s before and 9.4 s after**, against the debt window's 59 s and
112 s, because this window TERMed the archive a tenth of a second after the pause completed
instead of a minute later. **The margin is not what makes the gap zero — the structure is**, and
here the relay says so directly: it logged **nothing at all** between its last `client gone` at
**16:23:20.347Z** and the archive's return. No forward, no hold, no shed. With every local peer
disconnected and all four of slot 6's neighbours dark, no forward was possible. The organisms that
*were* in flight are accounted for on both sides rather than lost: the five sidecars recovered
outbound custody **1 / 1 / 1 / 0 / 2** and inbound **0 / 1 / 1 / 2 / 0** from their journals, slot
1 re-forwarded `526c11e0` (recorded at 16:23:19.225Z, before the pause) at 16:28:17.374Z, and slot
3 resumed a bounded hold on `e64547f2` with **13 ms accrued**. **A short margin with an empty relay
log is a stronger proof than a long margin alone**, but take the long one anyway when the pause is
cheap: it costs nothing to wait a minute before the TERM, and it makes the argument readable
without the relay log.

**The counter still answers the same question as the file.** Measured on the frozen ledger, with
the map paused so the reconciliation is exact rather than a race: **`wc -l` 8,873,507,
`ledgerRecords` 8,873,506, `ledgerSkippedLines` 1, difference 0.** The counter read the same value
before and after the 25 s `wc`, which is what makes the pair exact.

**`limits` is on the page, and the relay never changed.** Eight keys, key for key against the
envelope this same relay generation logged at startup: `maxConnectionsPerPeer` 2,
`maxConnectionsPerAddress` 8, `maxFramesPerSecond` 50, `maxFrameBytes` 8388608,
`maxBytesPerSecond` 4194304, `maxClaimsPerMinute` 12, `maxGenomeRequestsPerMinute` 30,
`maxSubscribers` 4. `minContractVersion` is **absent**, which is B25's default and the relay's real
answer — its startup line reads `minContractVersion=<none>`. The before-and-after is the whole
point of the amendment: the *new* `ringstat --settings` run against the *old* page printed
"ceilings: unknown — this map publishes none, which means a relay older than the published table",
and against the new page it prints all eight in §3.3's order. **Nothing about the relay moved
between those two runs.** The UNKNOWN was the archive's blindness, and §10.1's rule is what kept it
from being read as "no ceilings".

**The tree serves the real thing, and it is a view of the LIVE census — so it must not be read
while the map is paused.** With all six slots reporting: **18 nodes, 16 parent edges, 2 roots**
(`Zhiluus tardisitguyus` and `Basic bibite`), **14 leaves, all 14 alive**, `alive` 15, `connected`
14, `isolated` 1, `unrecorded` 1, `ancestors` 3, `collapsed` 14, `maxDepth` **40**, reduced from
`ledgerSpecies` **2,430** and `ledgerEdges` **2,022**; `reportingSlots` 6, `censuslessSlots` 0,
`truncatedSlots` 0, and no `walkCapped`, `cycleGuard` or `nodesCapped`. **5,667 bytes** identity,
**680 gzipped**. The commit's honest shapes are all live at once: a forest rather than one root, and
`Basic bibite` alive as its own isolated root, counted as `unrecorded` — the seeded species the
record connects to nothing because nothing ever migrated carrying it as a parent. **Measured
*inside* the pause the same endpoint served a truthful but useless tree** — 7 nodes, 1 root, 5
alive, `reportingSlots` **1** — because only slot 6 was still reporting a census. That is not a
fault and it is the reader's trap this endpoint carries: the tree's leaves are whoever is alive
*now*, so a paused or half-dark map draws a small tree, and the gate has to be taken after the
resume.

**Gzip, and the horizon, unchanged.** `/api/status` 15,241 → **1,687 bytes (9.03×)**, the new
`/api/species/tree` 2,340 → **680 bytes (3.44×)**, both with `Vary: Accept-Encoding`, and
`/healthz` identity in both directions at 3 bytes. `genomeHorizonMs` is still **absent**, as are
`genomesEvicted`, `genomesEvictedBytes` and `genomeGapsExpired`: the relaunch went through the
rig's own `start_archive` with `ARCHIVE_HTTP=0.0.0.0:8796` preserved and **no** new environment.

**THE TREE COSTS REPLAY TIME, AND THAT IS THE ONE NUMBER THAT MOVED THE WRONG WAY.** 8.87 M records
in 186 s is **47.8 k/s**, against 8.44 M in 148 s — **57 k/s** — three hours earlier on the same
host. The ledger grew 5%; the wall grew 25%. The suspect is the tree itself: its parent edge is
maintained per record inside `observeSpeciesLocked`, which the streamed replay calls on every
record, so O(1) per record is still a new constant on 8.9 M of them. It is not a fault and no gate
failed, but **the replay estimate for the next paused window should be sized from 48 k/s, not
57 k/s**, and if the rate keeps sliding it is worth measuring rather than assuming.

**A DIGEST COMPARISON AGAINST `bin/` CANNOT TELL YOU WHAT CHANGED ON THIS RIG.** `build()` runs
`CGO_ENABLED=0 go build`, which leaves `-buildvcs=auto` on, so every binary embeds
`vcs.revision` — and every digest therefore moves the moment HEAD moves, whether or not one line of
its code did. `bin/` was built from `bde0f1c` and HEAD was `eeff267`, so a plain digest comparison
reported **all six binaries changed** when only the archive package had been touched. The comparison
that answers the question strips both the stamp and the source path from both sides: extract the
old revision (`git archive bde0f1c go/ | tar -x -C <scratch>`), build it and HEAD with
`CGO_ENABLED=0 go build -trimpath -buildvcs=false`, and compare those. That gave **`archive` and
`ringstat`, and nothing else** — `relay`, `sidecar`, `fakemod` and `worldstat` byte-identical — and
only those two were moved over `bin/`. Two smaller traps sit under it: a `go build` without
`CGO_ENABLED=0` is a different binary again, so a comparison built with the wrong flags is
meaningless; and the binaries actually **installed** must be the rig's own flavour, stamp included,
so that `go version -m bin/archive` keeps answering which commit is running.

**The rollback set is `/mnt/wsl/data/avw-2026-08-12/rollback-bin/`** — `archive.RUNNING`
(`1e30b66e…`, copied from `/proc/2149599/exe`), `archive.bin` and `ringstat` (`7609d23b…`). **This
time `bin/archive` and the running process were the same bytes**, so the debt window's trap did not
recur — but the two copies are taken and compared rather than assumed, which is the only way to
know. It retires on the usual rule.

**One reading is misleading for five minutes after any archive restart, and it looks alarming.**
`flowWindowMs` is **300,000**, and `perMinute` is measured over that whole window whether or not the
window has any history in it, so a fresh archive divides a few seconds of hops by five minutes. The
first sample after the resume read **29/min** on a map that had been doing 647/min. It climbed
monotonically — 29, 75, 123, … 917 — for exactly as long as the window took to fill, and none of
that curve is traffic. **Read `perMinute` no sooner than five minutes after the archive comes back**;
before then, `ledgerRecords` between two samples is the honest rate. Once it had filled it settled at
**~920/min against the 647/min read before the window**, which is a real change and **not** something
an archive restart can cause — the archive moves no organisms. It is the `achievedTimeScale` readout
explaining itself: population fell 466 → ~385, slot 3's world emptied from 114 to 47 and its achieved
scale doubled from 4.1 to 8.8, and **a world that simulates more minutes per wall minute exports more
organisms per wall minute**. The rate is spread evenly over all six slots (131–167/min each, slot 6 at
167 against the 158.8 measured for it in the window above), which is what rules out one sick lane.

**Two smaller things worth keeping.** `bin/sidecar --diagnose --data-dir <dir>` is the mechanical
form of the zero-discarded-bytes gate — it prints `PASS journal-replay … zero discarded bytes`, and
it passed on all five — but **pass it `--credential-file` too**, or its `credential` check fails on
a rig that keeps secrets in `~/.multiverse/` rather than in the data directory, and the report reads
worse than the rig is. And the journals **shrank at the restart** (slot 1, 36.9 MB → 28.4 MB) with
no `compacted the journal` line anywhere: that is `internal/journal` still compacting at `Open`, as
it always has, and it is a rewrite of live entries, not a discard.

### The stratigraphic-lifespan window, 2026-08-12 — the merged view goes live, and two defects with it

**The third archive-only pause of the day, run under the owner's standing grant, and the first one
whose point was a PAGE rather than an endpoint.** It carried the merged stratigraphic lifespan view
(`5f2dc83` — the species tab and the tree tab become one drawing against time) and the root badges
(`bdb5efb`). The relay was not restarted and its `relaySessionId` is still
`27005d53-02f0-4447-985f-67b0d03e2c86`; the five games, the mod and slot 6 were untouched. **The
view is live and it is right about the map — and the browser sweep it was restarted for found two
rendering defects, neither of them patched.** They are below, with their evidence. **Both — and the
softer third under them, and the blinking ring — closed in `979d72b` and were verified on the live
page the same evening**; see *The genealogy-fix window* below, which also records what the floor
fix did NOT change.

| | |
|---|---|
| Crossings paused | **5m 02.2s** — no local sidecar was up between **21:58:13.18Z** and **22:03:15.4Z**; all five had reclaimed by **22:03:29.6Z**. The worlds simulated throughout |
| The archive | down **21:59:19.927Z → 22:03:11.409Z (3m 51.5s)**, entirely inside that pause. Replay of **9,511,821 records / 3.18 GB** took **231 s** |
| Replay memory | **VmHWM 1,928,848 kB (1.84 GiB)** against the outgoing process's 2,282,392 kB — the streamed profile again, on a ledger 7% larger than the archive-views window's |
| Replay speed | **41.2 k records/s**, against that window's 47.8 k and the debt window's 57 k. **The slide is real and it is now three points on one host** — size the next paused window from **41 k/s**, and see below |
| The ledger gap | **ZERO**, with the widest margins of the three — see the accounting below |
| The five sidecars | every one `reason=reclaimed`, own coordinate, pacing line present (`pacedFramesPerSecond=25 pacedBurstFrames=12`), and `--diagnose` **PASS journal-replay … zero discarded bytes** on all five. TERM to exit: **5.1 / 5.4 / 1.3 / 0.14 / 0.16 s**. Start to healthy: **2.1 / 2.2 / 1.7 / 2.5 / 5.7 s** |
| Observation | 8 samples, 22:04:26Z–22:06:58Z, then a `rig-check --wire` at 22:13Z: `holes`, `heldDepth` and `timeoutBounces` **0** throughout; `custodyDepth` 61–112 and `pacedDepth` 22–42, both falling every time they rose; population 258–301. **Zero** new `level=ERROR` and **zero** sheds across the relay, the five sidecars and the archive — the archive's single `ERROR` is the old damaged ledger line (`skippedLines=1 skippedBytes=776`), re-reported at replay |

**The zero-gap accounting, and the widest margins yet.** The last ledger record of any kind before
the archive stopped was an `ACK` at **21:58:12.743Z** — **67.2 s** before the TERM and **0.43 s
BEFORE the last local sidecar had exited**, so the ledger was already still while the pause was
still closing. The first record after the archive returned was a `MIGRATION`, slot 5 → slot 2 on
its south edge, at **22:03:46.775Z**, **35.9 s** after it re-subscribed and **17.1 s** after the
last sidecar was back. The ledger file was **byte-frozen at 3,176,865,744 bytes** for the whole
outage. And the relay says the same thing the archive-views window's did: it logged **nothing at
all** between its last `client gone` at **21:58:13.182Z** and the archive's return at
**22:03:10.887Z** — no forward, no hold, no shed. Take the long margin when the pause is cheap; it
cost 60 s of `sleep` and it makes the argument readable without opening the relay log.

**The counter and the file still answer the same question.** Measured on the frozen ledger:
**`wc -l` 9,511,822, `ledgerRecords` 9,511,821, `ledgerSkippedLines` 1, difference 0** — and the
counter read the same value either side of the 3 s `wc`, which is what makes the pair exact. The
`wc` itself was 3 s rather than the archive-views window's 25 s, because the replay had just walked
the whole file into page cache.

**Gzip, the ceilings and the horizon, all unchanged.** `/api/status` **13,045 → 1,227 bytes
(10.63×)** with `Vary: Accept-Encoding`, `/healthz` identity at 3 bytes when gzip is offered. All
eight `limits` keys are still published and `minContractVersion` is still absent. `genomeHorizonMs`,
`genomesEvicted`, `genomesEvictedBytes` and `genomeGapsExpired` are **all four absent**: the
relaunch went through the rig's own `start_archive` with `ARCHIVE_HTTP=0.0.0.0:8796` preserved and
**no** new environment.

**A SHARED INTERNAL PACKAGE MAKES MOVERS OUT OF COMMANDS THE CHANGE WAS NOT ABOUT.** The
archive-views window taught the stamp-stripped comparison; this one adds the half it did not need.
`bin/` was not one revision but two — `archive` and `ringstat` at `eeff267`, the other four at
`bde0f1c` — so the comparison has to run **per binary against ITS OWN installed revision**, and
`-trimpath -buildvcs=false` builds of all three revisions found **four** movers where the window was
about one: `archive` and `ringstat` as expected, and **`sidecar` and `worldstat` as well**, because
`5f2dc83` lifted `Canonical`'s dialect selection into a shared `bb8.dialect()` and added
`bb8.BrainStats` for the brain rings, and both of those commands link `internal/bb8`. Reading the
diff settles what the digest cannot: `Canonical` keeps the same three error paths and the same
outcome on each, so neither command's behaviour moved. **Only `archive` and `ringstat` were moved
over `bin/`**; the sidecar and worldstat were deliberately left at `bde0f1c` so the window stayed
archive-only and the five live peers restarted on the very bytes they had been running. **A mover
is not automatically a thing to install** — and a plain `go build -o ../bin/ ./cmd/...` after this
would quietly install two of them, so run it inside a window that means to.

**The rollback set is `/mnt/wsl/data/slv-2026-08-12/rollback-bin/`** — `archive.RUNNING`
(`3af843a8…`, copied from `/proc/2266682/exe`), `archive.bin` and `ringstat` (`6f458a5d…`). The two
archive copies were the same bytes again, as in the archive-views window, but they are still taken
and compared rather than assumed. It retires on the usual rule.

**THE REPLAY IS STILL SLOWING, AND THE TREE IS NO LONGER THE ONLY SUSPECT.** 57 k/s at 8.44 M
records, 47.8 k/s at 8.87 M, **41.2 k/s at 9.51 M** — the ledger grew 13% since the middle point
and the per-record cost grew another 16%, on the same host in the same afternoon. The archive-views
window named the tree's per-record `observeSpeciesLocked` maintenance as the prime suspect; this
window adds per-record work of its own to the same path (the merged view's spans and the per-node
lifespan bounds are maintained there too), so the honest reading is that **the species derivation as
a whole now costs about 40% of the replay it did two windows ago**. That is not yet a fault and no
gate failed, but it is a third point on a straight line, and the next archive change is where it is
worth measuring rather than assuming — a 12 M-record ledger at this rate is a five-minute pause.

**CORRECTION, taken the same evening: the fourth point broke this line.** 9.58 M records replayed
at **47.3 k/s** — a 0.7% bigger ledger, 12% faster in wall clock — against a commit that adds no
per-record work at all. The line was over-read, and this series measures host load at least as much
as it measures the derivation. See *The genealogy-fix window* below: size from **~45 k/s** and treat
the figure as noisy rather than as a trend.

**Slot 6's game went away 33 minutes before the pause, and that is the far end's own event.** The
relay logged `peer=slot-6 … modConnected=false exportEdges=[]` at **21:25:03Z**, with its sidecar
still connected — so this window ran against a five-game map, and the shape after the resume is the
one this file already documents for a dark slot 6: **18/20 lanes open, slot 3's north and south
closed with `peer_mod_absent`, and slot 4's west and slot 5's east re-paired as bypasses**. Slot 6's
sidecar sits on 22–28 inbound deliveries it cannot hand to a game, which is custody working. It is
not the orchestrator's to fix (D9), and it makes the zero-gap argument stronger rather than weaker:
a slot whose mod is gone cannot export at all. **It also moved under the pre-flight** — `rig-check
--wire` was at the bar before the build, and the far end changed between that check and the pause,
so a pre-flight is a fact about a moment and the post-resume bar has to be read against what the map
IS.

**THE BROWSER VERDICT: the view is right about the map, and two things it draws cannot be read.**
Everything the merged view claims about this rig checks out against `/api/status` — 20 rows with the
species glyph as the only coloured thing, bars on a time axis with `now` at the right edge, 18 parent
drops, 3 dotted collapsed lead-ins labelled `+7 +1 +8` against the stat line's "generations collapsed
16", 16 sparklines against 16 series from `/api/species/trends`, and mini-maps cross-checked four
ways (`joeyclanahani` S1 4 / S2 14 / S3 9 / S4 15 / S5 20, `garnariensis` slot 3 alone,
`Todae hernandius` slot 5 alone, `Basic bibite` slot 2 alone) with **slot 6 drawn as an unknown dot
and never an empty one**. A row opens to `worlds`, `record` (`41164 crossing(s) · 4625 distinct
genome(s) · first 4h 11m ago`), `brain` and `parent species … shown, never resolved`. The population
toggle re-ranks the 16 living species and draws **no** edges, as designed; search keeps a match's
connecting ancestor and answers "no species matches that search"; `#tree` lands on the species tab;
the map and settings tabs are unchanged, ceilings card and all. **Zero console messages of any
kind.** The two defects:

- **THE ROOT BADGES ARE CLIPPED OUT OF VIEW — the whole of `bdb5efb` is unreadable on the page.**
  *(Fixed in `979d72b`: the badges take their own line under their own wider clip, and the row's
  `<title>` is now real. Verified live — see below.)*
  Every label rides in one `<text class="nm" clip-path="url(#lfclip)">`, the clip is
  `LF_NAMEX-4 = 26` wide by `LF_NAMEW = 366` so its right edge is **x 392**, and the badges are
  `tspan`s appended to that same run with **no width budget at all**. Measured live:
  `Basic bibite`'s `NO RECORDED ANCESTRY` spans **x 367 → 493** and is painted as **"NO R"**;
  `Zhiluus tardisitguyus`'s `THE RECORD BEGINS HERE · 31 GENERATIONS ABOVE` spans **x 425 → 708**
  and is **invisible entirely**, because it begins 33 px past the clip. `ALSO AN ANCESTOR` loses its
  tail on both rows that carry it, and every ancestor's `· extinct here · N living lines below` is
  cut mid-word. The mitigation the code comments promise — "the full name is always in the row's
  tooltip" — **does not exist**: the row group has no `<title>` and no `aria-label`, so nothing on
  the page can recover the clipped text. Both badges are correct in the DOM and in
  `/api/species/tree`; only the drawing loses them.
- **THE AMBER ANCESTRY FLOOR BOUNDARY IS NEVER DRAWN.** *(Fixed in `979d72b` with the `>=` and a
  zero-width shade allowed to be zero-width; drawn and verified live below. Note that the clamp this
  entry describes is also why hiding the seed stock tightens no axis — same section.)* `tree.go`
  clamps `SpanStartMs` down to
  `AncestrySinceMs` under the comment "THE FLOOR IS ALWAYS INSIDE THE PICTURE"; `lfAxis` then guards
  the line with `x.ancestrySinceMs > sc.t0` where `sc.t0` **is** `spanStartMs`. Equal is not
  strictly greater, so on the normal case — the floor older than every bar, which is what the server
  guarantees — `.floor`, `.floorlbl` and `.prefloor` are **all three absent from the DOM**. Measured
  live, both are `1786079662896` (2026-08-07T05:14:22.896Z) against an earliest bar 14 s later. The
  textual floor still prints in the stat line ("ancestry recorded since 2026-08-07 UTC"), so what is
  missing is exactly the boundary. `>=`, and a zero-width shade allowed to be zero-width, is the
  whole fix.

**A third, softer one: the drawing is 1344 px wide no matter how wide the window is.** *(Fixed in
`979d72b` — the plot is elastic, floors at `LF_PLOTMIN` 280, and a resize repaints; verified at 500,
1280 and 1600 px below.)*
`LF_PLOTW` 700 plus the fixed columns is a constant, and `#lfbox` is the window minus 83, so the
picture **fits only at a window ≥ 1427 px**. At 1280 the box is 1197, `scrollLeft` is 0 and there is
no scroll-to-now, so **147 px — the `now` end of every living bar, which is the point of the
view — sit past the right edge** until the reader finds the inner scrollbar. The outer rule is
respected and worth recording: at 500 px `documentElement.scrollWidth == clientWidth == 485`, **no
horizontal page scroll**, and every overflowing element is inside `.lifewrap`.

**The brain ring blinks, and that is the genome gaps showing through.** Sampled ten times over a
minute, between **2 and 6** of ~20 rows carried a ring and the membership changed on nearly every
poll; only `Sheeplasius godopedrus` (extinct here, so its latest genome is frozen) and
`Basic bibite` (which never exports) were stable. With **154 k+ `genomeGaps`** outstanding, a
species that crosses often rotates its newest genome hash faster than the archive can fetch the
blob, and the row's own detail says so in words — `brain  no copy of its latest genome is held
here`. That is §10.1 working exactly as written, but **a reader must not expect the ring to be a
property of a row**; it is a property of this minute. *(Stabilised in `979d72b`: the ring is drawn
from the newest genome of that species the archive has been able to READ, not the newest hash it has
seen. Ten polls, one membership set — see below. The backlog itself is unchanged, so a row the
archive holds no blob for still says so in words.)*

**One smaller thing.** `save-health` came back `WARN` on slots 2, 3 and 4 immediately after the
window — 2336, 2670 and 4072 ms against the 2000 ms stall budget — and was clean again later. The
archive's 3.18 GB replay is production load on this host in exactly the way `go test ./...` is
(*A controlled reproduction of the 18:39Z storm*), so a save that overruns inside a replay window is
dated to the replay, not to the game.

### The genealogy-fix window, 2026-08-12 — three defects close, and the axis turns out to be pinned by something else

**The fourth archive-only pause of the day, run under the owner's standing grant, and the first one
whose point was to VERIFY rather than to land.** It carried `979d72b`: the seed stock out of the
default drawing, the stratigraphic window's three rendering defects, and the brain ring's
stabilisation. The relay was not restarted and its `relaySessionId` is still
`27005d53-02f0-4447-985f-67b0d03e2c86`; the five games, the mod and slot 6 were untouched. **All
four fixes verify on the live page, and the sweep found no new rendering defect — but it did find
that the change the seed-hiding was FOR does not happen on this map**, for a reason that is nobody's
bug and is below.

| | |
|---|---|
| Crossings paused | **4m 33.9s** — no local sidecar was up between **22:49:29.93Z** and **22:54:03.87Z**; all five had reclaimed by **22:54:12.01Z**. The worlds simulated throughout |
| The archive | down **22:50:36.796Z → 22:53:59.324Z (3m 22.5s)**, entirely inside that pause. Replay of **9,579,220 records / 3.20 GB** took **202.5 s** |
| Replay memory | **VmHWM 1,879,668 kB (1.79 GiB)** against the outgoing process's 2,513,544 kB — the streamed profile, and slightly *lower* than the stratigraphic window's on a slightly larger ledger |
| Replay speed | **47.3 k records/s**, against 41.2 k three hours earlier. **The slide reversed on a ledger 0.7% bigger** — see below; this is the fourth point and it breaks the line the third one drew |
| The ledger gap | **ZERO**, margins 67.1 s before and 4.8 s after |
| The five sidecars | every one `reason=reclaimed`, own coordinate, pacing line present (`pacedFramesPerSecond=25 pacedBurstFrames=12`), and `--diagnose` **PASS journal-replay … zero discarded bytes** on all five. TERM to exit: **80 / 78 / 53 / 75 / 76 ms**. Start to grant: **2.06 / 1.76 / 1.10 / 2.10 / 1.77 s**. Custody recovered outbound/inbound **9/0, 0/16, 16/0, 6/1, 15/0** |
| Observation | 12 samples, 22:55:40Z–23:01:41Z, then a `rig-check --wire` at 23:05Z: `holes`, `heldDepth` and `timeoutBounces` **0** throughout, lanes **18/20** in every sample; `custodyDepth` 58–98 and `pacedDepth` 22–41, both falling every time they rose; population 228–264. **Zero** new `level=ERROR` and **zero** sheds across the relay, the five sidecars and the archive — the archive's single `ERROR` is the old damaged ledger line (`skippedLines=1 skippedBytes=776`), re-reported at replay |

**The zero-gap accounting, and a detail the earlier windows did not have to name.** The last
CROSSING record before the archive stopped was an `ACK` at **22:49:29.726Z** — **67.1 s** before the
TERM and **0.21 s BEFORE the last local sidecar had exited**, the same shape the stratigraphic
window measured. But the last record of **any** kind was a `GENOME` at **22:50:28.752Z**,
`servedBy=slot-6`, only 8.0 s before the TERM: **slot 6's sidecar stays connected through a local
pause and keeps serving genome blobs, so the ledger is not still during the pause — it is still
during the OUTAGE**, which is the interval the gate is actually about. It was: **zero records carry
a `recordedAt` inside 22:50:36.796Z → 22:53:59.324Z**, and the file was **byte-frozen at
3,199,213,777 bytes** across it. The first record after the return was a `NACK` at 22:54:04.129Z
(4.8 s after it answered) and the first `MIGRATION` slot 5 → slot 2 at 22:54:43.876Z. The relay says
the same as ever: inside the outage it logged **exactly two lines, both about the archive itself** —
its own `client gone` at 22:50:38.097Z and `client connected` at 22:53:59.221Z. No forward, no hold,
no shed.

**The counter, gzip and the ceilings, all unchanged.** On the frozen ledger: **`wc -l` 9,579,221,
`ledgerRecords` 9,579,220, `ledgerSkippedLines` 1, difference 0**, with the counter reading the same
value either side of a 2.1 s `wc`. Gzip after the resume: `/api/status` **18,172 → 2,298 (7.91×)**,
`/api/species/tree` **11,275 → 2,193 (5.14×)**, both with `Vary: Accept-Encoding`, `/healthz`
identity at 3 bytes. All eight `limits` keys published, `minContractVersion` absent, and
`genomeHorizonMs`, `genomesEvicted`, `genomesEvictedBytes` and `genomeGapsExpired` all four absent:
the relaunch went through `start_archive` with `ARCHIVE_HTTP=0.0.0.0:8796` preserved and **no** new
environment. **Take the tree's gzip figure after the resume, never inside the pause** — measured
while paused it was 824 bytes, below the 1,400-byte identity floor, because a paused map draws the
small useless tree the archive-views window warned about.

**THE STAMP-STRIPPED COMPARISON NEEDS A SECOND BASELINE, NOT JUST A BETTER ONE.** `bin/` was two
revisions again (`archive` and `ringstat` at `5f2dc83`, the other four at `bde0f1c`), and comparing
each binary against its own installed revision reported **four** movers — `archive`, `ringstat`,
`sidecar` and `worldstat` — exactly as the stratigraphic window predicted, because the sidecar and
worldstat still carry that window's uninstalled `internal/bb8` delta. That answer is true and
useless: it cannot tell a mover *this* commit made from one banked earlier. **Build the previous
HEAD too and compare `5f2dc83 → 979d72b`**, which gives the question its real answer — `archive` and
`ringstat` **only**, with `sidecar`, `worldstat`, `relay` and `fakemod` byte-identical — matching a
diff that touches six files, all of them in `go/internal/archive`. Two baselines, two questions:
*what is stale on this rig* and *what did this change move*. Only the two were installed; the
sidecar and worldstat stay at `bde0f1c` and the five live peers restarted on the very bytes they had
been running.

**The rollback set is `/mnt/wsl/data/gfw-2026-08-12/rollback-bin/`** — `archive.RUNNING`
(`9e57a303…`, copied from `/proc/2466790/exe`), `archive.bin` and `ringstat` (`b0296614…`). The two
archive copies were the same bytes for the third window running, and are still taken and compared
rather than assumed. It retires on the usual rule.

**THE REPLAY RATE SERIES BROKE ITS OWN LINE, AND THE THIRD POINT WAS OVER-READ.** 57 k/s at 8.44 M
records, 47.8 k at 8.87 M, 41.2 k at 9.51 M — and now **47.3 k at 9.58 M**, a ledger 0.7% larger
replayed **12% faster in wall clock** (202.5 s against 231 s) on the same host the same evening.
This commit adds **no** per-record work to check it against: it touches no replay-path function at
all — `observeSpeciesLocked` is untouched, the seed-stock rule is evaluated in `speciesIndexFrom` on
the view path, and `brain.go` parses on demand. So the fourth point is not a code change and the
third one was not purely a ledger-size cost. **The honest reading is that this series measures host
load at least as much as it measures the derivation**, and the stratigraphic window's "three points
on one straight line" over-read it — that window's own `save-health` WARNs on three slots say the
host was busier then. **Size the next paused window from ~45 k/s and treat the number as noisy**;
if the cost of the species derivation is ever worth a decision, measure it on a quiet host against a
fixed ledger, not by differencing production windows.

**THE BROWSER VERDICT: all four fixes are live and correct on the page.** Swept at a 1280 px window,
`#tree`, with **zero console messages of any kind** and **no page-level horizontal scroll on any
tab**.

- **The seed stock is out of the drawing, and the way back in is free.** 14 rows drawn and
  `Basic bibite` absent; the stat line reads *"1 seed species hidden — excluded from migration on
  every world where it lives · show"*. `show` gives 15 rows and flips the notice to *"…shown … ·
  hide"*. Searching `basic` reveals the hidden row on its own; `todae` gives 4 rows — the three
  matches **plus** their connecting ancestor — and does **not** drag the seed in; a miss still says
  *"no species matches that search"*; clearing restores 14 with the seed hidden. The claim that the
  reveal costs no second request holds under a decisive test: **10 toggles inside a second cost 0
  HTTP requests, against 3 in an equal idle second** (the page polls ~1.5 s, so a single-click
  measurement will catch an ambient poll and mean nothing). The revealed row carries **SEED STOCK ·
  NEVER EXPORTED S1 S2 S3 S4 S5**, then **ENDEMIC**, then **NO RECORDED ANCESTRY** — the badge that
  used to paint as `NO R` — all three complete.
- **The badges are readable, and the tooltip the old comment promised now exists.** They ride in
  their own `text.bdg` on their own line (y 77 against the name's 60) under their own clip
  `#lfbclip` (x 26, width 574, right edge **600**) instead of the name's `#lfclip` (right edge 392).
  The widest badge line on the map — the root's `· extinct here · 9 living lines below` +
  `THE RECORD BEGINS HERE · 31 GENERATIONS ABOVE` — ends at **x 562.9, 37.1 px inside its clip**,
  and every other badge line has more room than that. Every row group opens with a real
  `<title>`.
- **The amber floor is drawn.** `line.floor` at x 614 in `rgb(226,185,59)` with `.floorlbl`
  *"ancestry recorded from here"* beside it, and `.prefloor` present as a **zero-width** rect —
  which is the fix: `>=`, and a shade allowed to be zero wide.
- **The plot is elastic and `now` is on screen.** At 1280 the svg is 1196 and the wrapper's
  `scrollWidth == clientWidth == 1197`, so `now` at x 1166 needs no scrolling — against the old
  fixed 1344 that put 147 px past the edge. A resize repaints: 1600 → svg 1516. At 500 px it floors
  at `LF_PLOTMIN` 280 and takes an inner scrollbar, while
  `documentElement.scrollWidth == clientWidth == 485` and **`.lifewrap` is the only overflowing
  element on the page** — the outer rule is still respected.
- **The brain ring has stopped blinking.** Ten polls over 30 s: **exactly 3 of 14 rows carried a
  ring every single time, the same three, one distinct membership set** — against 2–6 of ~20 rows
  changing on nearly every poll before. The ringed set is exactly the set of nodes carrying
  `neurons`/`synapses`, the open row now says *"(from the latest genome of it this archive holds)"*,
  and a species the archive has read no blob for still says *"no copy of its latest genome is held
  here"*. `genomeGaps` is still **~153 k**, so the backlog did not go away — the ring stopped being
  a property of this minute.
- **Everything the previous sweep passed still passes.** Mini-maps match the census slot-for-slot on
  all 8 living rows, on the map's own grid with **row 0 at the bottom** — read them in that
  orientation or the cross-check will look wrong — with slot 6 the **unknown** dot on every row and
  never an empty one. Dotted lead-ins `+7 +1 +8` sum to 16 against `collapsed` 16; 9 sparklines for
  9 living drawn rows; the population toggle re-ranks to 8 rows and draws **zero** edges and zero
  chains, and `family` restores the order exactly; a row opens to `worlds`, `record`, `brain` and
  `parent species … shown, never resolved`; `#tree` and `#species` both land on the species tab and
  `#map`/`#settings` are unchanged, ceilings card and all eight limits included.

**AND THE ONE THING THAT DID NOT FOLLOW: hiding the seed changed no axis at all, because the axis was
never the seed's to set.** `tree.go` computes two left edges — `SpanStartMs` over the drawn bars and
`SpanStartSeedMs` with the seed's in — and then clamps **both** down to `AncestrySinceMs` under
"THE FLOOR IS ALWAYS INSIDE THE PICTURE", which is what fix (d) needs in order to have a boundary to
draw. On this map the floor is **2026-08-07T05:14:22.896Z** and the oldest drawn bar starts
**2026-08-11T01:11:11.818Z**, so the clamp wins and all three values are the same int64:
`spanStartMs == spanStartSeedMs == ancestrySinceMs`. Measured on the page, revealing the seed moved
**no tick by a pixel**. The consequence is visible: the plot runs x 614 → 1166 and the earliest bar
begins at x 982.6, so **368.6 px of 552 — 66.8% of the drawing — is empty**, and every bar the view
is about is packed into the right third. The seed's full-width bar used to occupy that space; now
nothing does. **The seed was never what stretched the axis** — the floor was, and the floor is fixed
at the record's beginning while living species turn over every few hours, so **this gap widens for
as long as the map runs**. Nothing here is a bug: `tree.go` does exactly what its comment says, the
second axis is correct and merely inert on a map where the seed's first crossing (05:14:36.853Z) is
*younger* than the floor (05:14:22.896Z), and no gate failed. It is a design collision between two
rules that were written separately, and **it is the owner's call**, not a patch to make quietly: the
choices are to let the drawn axis start at the oldest drawn bar and render the floor as an
off-picture caption rather than a boundary, or to keep the boundary and accept a drawing that is
mostly empty and getting emptier.

**Two smaller things.** `ringstat --species` does **not** hide the seed and should not — it prints
`Basic bibite` with `never-exported endemic`, which is the operator's view of the same fact the page
calls SEED STOCK; the vocabularies differ between the two surfaces. And `perMinute` did its
documented five-minute climb again after the restart — 130.8 → 808.2 across the 12 samples — then
settled at **821/min against the 695.6/min read before the window**, which is the achieved-time-scale
readout explaining itself once more (slot 1 at 31.0, slot 2 4.5 → 6.9) and not something an archive
restart can cause.

**The 21:25:03Z far-end event has a name now: the headless game crashed in the engine, and the
first headless crash on record cost six simulated minutes and nothing else.** Windows Application
Error 1000 at 21:25:01Z: `The Bibites.exe` faulted in **`UnityPlayer.dll`** (6000.0.44.7313),
exception `0xc0000005`, offset `0x7350d2`, ~10.3 h into the headless run — no mod frame, no
operator, no Steam action (the last content-log commit for app 2736860 is 2026-08-01, and
`BibitesAssembly.dll` still matches the bundle's pin `12455E48…` byte for byte, checked before
the restart was allowed). The BepInEx log simulates normally to the last line; there was **no
quit save** — the last periodic save landed at 21:19:23Z (and logged `BUDGET_EXCEEDED`,
stallMs 2022 against the 2000 budget, its own standing watch item) — so the world resumed from
six minutes back, which is what D14's cadence prices a crash at. The sidecar held slot 6
throughout (custody 28, paced 22 at the deepest, oldest waiting 2h11m), the world came back
headless at 23:38Z through `start-slot6.ps1 -GameOnly -Headless`, the mod reattached (gen=5),
the time scale was re-sent, and the backlog drained to zero inside two minutes. Exactly-once
held: the un-ACKed inbound replayed as duplicates and delivered nothing twice. One session
mystery kept honest: this session's `Player.log` never flushed (mtime frozen at the 11:07Z
start), so the engine's own account of the fault is only in WER — the far end's crash forensics
are the event log and the sidecar log, not Unity's. Also observed while diagnosing: an
unexplained clean `world unloaded`/reattach cycle at 19:07:49Z (gen 3 → 4) the world survived
without a mark; noted, not chased.

### Bringing it back after a reboot

Proven end to end twice, on 2026-08-08 and 2026-08-09, and once more as the second half of the
2026-08-11 crossing window — where step 4 was the same `up` and the archive replay was the same
single largest term. The steps are ordered and step 1
gates the rest.

1. **Mount `/mnt/wsl/data`.** See *Where the rig lives now* above. Nothing — not `git`, not
   the rig, not a path in a pid file — resolves while the symlink dangles.
2. **Remove the previous run's pid files, after confirming each pid is dead.** A reboot
   leaves seven of them in `e2e/run-m4-lan/`: `m3-relay.pid`, `m3-archive.pid` and
   `m3-sidecar-1.pid` .. `m3-sidecar-5.pid` (the `m3-` prefix is inherited from
   `run-m3.sh`, which is the base library — it does not mean an M3 rig ran). They do not
   block `up`, but `status` reports the rig running when it is not and `down` would `kill -9`
   whatever now owns that number. **Check liveness first — pid reuse across a reboot is
   exactly the case this guards:**

   ```sh
   cd ~/bibites-multiverse/e2e/run-m4-lan
   for f in *.pid; do p=$(cat "$f"); kill -0 "$p" 2>/dev/null && echo "ALIVE $f $p" || rm -v "$f"; done
   ```

3. **Verify the Windows plumbing read-only; do not re-run it blind.** The `8795` portproxy
   and the firewall rule **survive a reboot** as long as the WSL address has not changed, and
   it did not across either reboot — it stayed `172.24.110.174`. `e2e/run-m4-lan.sh lanhost`
   prints the live WSL address beside the current portproxy table and touches nothing, so it
   is the check. Only if the two disagree do the elevated commands in *Owner steps* apply.
   The M3-era `8790` portproxy must **stay deleted**; `lanhost` shows the table, and a row for
   `0.0.0.0:8790` means slot 4 is about to look like a mod bug.
4. **Bring the rig up**, with the status page bound for the LAN:

   ```sh
   ARCHIVE_HTTP=0.0.0.0:8796 ./e2e/run-m4-lan.sh up
   ```

   **It refuses to start without the credentials, and that refusal is the feature.** Since
   2026-08-11 `up` runs `ensure_credentials` first — it mints nothing, because only the relay can
   mint; it checks that `e2e/tls-m4-lan/`'s three files and the six secrets under
   `~/.multiverse/` are on disk, and fails loudly if any is missing. The alternative it exists to
   prevent is five sidecars on a backoff ladder against a `401`. If it ever fires after a reboot,
   the answer is a restored secret, never a re-mint: re-minting is `--handover-slot`, which
   changes a `peerId` (*Credentials, TLS, and the retired LAN token*).

   **That one command now reproduces the whole regime, and the command line does not carry
   it.** `run-m4-lan.sh` exports `BIBITES_EXTRA_ARGS='-batchmode -nographics'` and assigns
   `TIME_SCALE=100`, `SAVE_MINUTES=10` and `SAVE_KEEP=6` unconditionally after it sources the
   rehearsal (the file's own capture-before-source idiom), so every game `start_game` launches is
   headless, saves on the mod's own shipped cadence, and the last
   act of `up` is `send <n> timescale 100` on all five. All four are still environment overrides:
   `TIME_SCALE=5 …` brings the old speed back, `SAVE_MINUTES=2 SAVE_KEEP=4 …` brings the old
   cadence back for a measurement, and `BIBITES_EXTRA_ARGS= …` — empty, which is
   why that default uses `-` and not `:-` — brings the windows back. `run-m4.sh` is untouched
   in kind: the rehearsal still runs drawn at ×5 and still saves every two minutes, on purpose
   (*Watch items*, *The decision*).

   **Check the four on the page, not in the script.** `/api/status` publishes `saveMinutes` and
   `saveKeep` per slot (§19 A42) beside `timeScale` — and, since 2026-08-10, `achievedTimeScale`
   beside that — so one read confirms the whole regime: five local slots at `10`/`6` and
   `timeScale 100`, slot 6 at whatever its own operator set. A slot reporting `2`/`4` came up
   from a stale environment and needs its game restarted, not the rig.

5. **Expect one game to come up starved of a log file**, and fix that one instance rather
   than the rig — see *The five-instance ceiling, and the log-file starvation trap* in
   Gotchas. It happened on both reboots.
6. **Read the time scale of every instance, not only a restarted one** — see *A world can be
   at the wrong time scale* in Gotchas. It needed a `send 1 timescale 5` after the
   2026-08-08 reboot and nothing after the 2026-08-09 one, so the check is the deliverable,
   not the correction. **The target for a local world is now ×100** and the correction is
   `e2e/run-m4-lan.sh send <n> timescale 100`; the far end's speed is still its own operator's.
   Read the *achieved* rate too, not only the reported one — at ×100 the two are far apart,
   and the gotcha says what that means. **Since 2026-08-10 the page reads it for you**, as the
   second half of each cell's speed chip; it appears about ten seconds after the archive has a
   world to watch, so give it a poll or two before reading a fresh bring-up.
7. **Relaunch the baseline collector by hand.** Every reboot kills it and nothing restarts
   it. It is gitignored, read-only against the deployment — it only copies the five save zips
   and curls `/api/status` — and costs ~0.5 GB/day (*The disk budget*).

   ```sh
   cd ~/bibites-multiverse/e2e/baselines/m4-collector
   setsid nohup ./collector.sh >> collector.log 2>&1 &
   ```

   A reboot that lands mid-cycle leaves an empty snapshot directory behind; there is one,
   `20260808T143150Z/`, and it is harmless.

**The archive's counters jumped once, on 2026-08-10, and they will not jump again** — the
recovery has happened and every later replay reads the same records. See *The archive's ledger
recovery* above for what it cost and what it proved. Two things from it stay true of every
bring-up from here: the replay costs **minutes of no status page and more of them every day**
— ~93 s at 2026-08-10's 3.7 M records / 1.22 GB, and **128 s measured on 2026-08-11 at 6.24 M /
2.0 GB**, against the ~150 s the same rate predicted for it — so `up` reaches `archive subscribed`
two to three minutes after it starts the archive rather
than instantly, and **the seconds in that sentence are stale as soon as they are written**
(*The status page on the LAN* has the rate to compute them from). **The wait in `run-m4-lan.sh`
is the thing this breaks first**: it was 60 s and would have failed a healthy bring-up, it was
300 s from 2026-08-10, and one day of ledger growth took that from 3× headroom to
2× — **it has been 600 s since the crossing of 2026-08-11**. Size it from the ledger and raise it
rather than believing a timeout here. The second: `ledgerRecords` ran
**below** `wc -l` and drifted further every hour,
because the live counter only incremented on migration, ACK and NACK records while a `GENOME`
append (`archive.go`, `RecordGenome`) was counted at boot replay and never during the run —
8,060,891 against 8,156,869 lines on 2026-08-12, 1.2% low. **Fixed 2026-08-12**: the live path
counts a `GENOME` line like every other record, so from that build onward the counter and the
file answer the same question and `ledgerSkippedLines` is the whole of the difference. **The
running deployment keeps the artifact until the archive is next restarted** — the fix is in the
binary, not in the file — and every archive built before that date has it forever, so
**sizing the ledger from the file, not the counter, remains the safe habit** and is the only
safe one against a build you did not check.

### Watch items

Four things are being watched on the running deployment. The first is a **measured breach of
a bar the owner set**; the owner's decision on it **landed on 2026-08-10**, so what it now
waits on is whether the change works — which is evidence, and evidence takes a generation to
accrue. **The first evidence is in, as of 2026-08-10 20:20Z, and it points the right way without
being a rate yet**: no save has cost a session since the raise, and every `4004` logged since is
attributable to something that is not a save. It also grew a fourth sub-question that is no longer
about saves at all — see *What to watch now* under that item. The second is a trend with no verdict
yet, and it **grew a defect and lost it again on 2026-08-10**: the magnitude of the `genomeGaps`
backlog cost the archive twenty-six relay sessions and the ledger 9,641 records, that half is
root-caused, fixed and closed (`contract-b-m4.md` §21, B21), and what stays on this list is the
growth of the queue itself. The third **has its verdict** — the reading is understood, it is not a
defect, and it stays on this list because M5 changes what it will mean. **The fourth was opened
by the crossing of 2026-08-11**: the BepInEx log archive has no retention rule, and one window
took it from 3.4 GB to 21 GB.

- **The 2-second save-stall budget is breached routinely, and it has been since the day the
  exit test passed.** D14 set the bar at 2 000 ms and the exit test of 2026-08-06 measured
  241–538 ms against it (`m4_considerations.md`, Risk 3). **No generation since has held
  it.** Across the eleven preserved `[M4-SAVE]` generations in `e2e/logs-m4-lan/bepinex/` —
  8 087 saves, 8 072 of them periodic — `event=BUDGET_EXCEEDED` runs at **17%**, at a pooled
  median stall of **1 327 ms** and a maximum of **5 163 ms**. The bad nights are far worse:
  29–70% per slot through the evening of 2026-08-07, and 55–95% across the four populated
  slots on 2026-08-08, where slot 3 breached **41 of its last 43** saves at 1 909–3 413 ms.
  The current run reads better and not well — **20 breaches in 718 saves** since the
  2026-08-09 bring-up, medians of 0.70 s on slot 1 to 1.38 s on slot 5, still two to four
  times the exit test. **It is not the worlds growing** — not in the sense of population, which
  was the first guess: the collector's 243 five-slot snapshots put the save zips at 275–330 KB on
  2026-08-07, near 570 KB overnight on 08-08 at peak population, and 210–320 KB now. What
  moved is the **cost of a saved kilobyte** — 120–172 ms per 100 KB in the exit test,
  500–620 ms on 2026-08-08, 290–370 ms now. Slot 1 sharpens it: its population fell from 21
  to 7–11 and its file stayed near 220 KB while its median stall went 214 ms → 1 164 ms →
  704 ms. Nor is the payload mostly organisms — unzip
  any slot save and the bibites are about a fifth of it, behind `speciesData.json`
  (~900 KB raw), `data.bin` (~150 KB) and the 400×400 `img.png` the game renders inside
  `SaveSystem.CreateSave`. **A save costs roughly what it costs whether the world holds 7
  organisms or 40.** For four days that was read as "so the variable is the host", and it is
  **mostly wrong** — the worlds *were* growing, in the one dimension nobody was measuring, and
  "per saved kilobyte" is normalised by the *compressed* size, which is exactly the number that
  hides it. See *The answer*, below; the paragraphs between here and there are the record of how
  the reading was arrived at, and they are left as they were measured.

  **The 2026-08-10 headless-at-×100 switch is the cleanest evidence yet that the host is the
  variable, and it moved the wrong way.** Both changes landed together on the same five worlds
  within seven minutes, so this is one before/after on one host. Against 993 drawn saves at ×5
  in the generations preserved as `…headless.*` in `e2e/logs-m4-lan/bepinex/`, and 51 headless
  saves at the ×100 target in the hour after:

  | Slot | Drawn ×5 — median stall / ms per 100 KB / breaches | Headless ×100 — median stall / ms per 100 KB / breaches |
  |---|---|---|
  | 1 | 722 ms / 300 / 0 of 198 | 862 ms / 665 / 0 of 13 |
  | 2 | 1 099 ms / 314 / 3 of 196 | 1 799 ms / 538 / 4 of 10 |
  | 3 | 1 327 ms / 362 / 11 of 199 | 1 939 ms / 644 / 4 of 9 |
  | 4 | 1 189 ms / 346 / 1 of 200 | 1 582 ms / 635 / 1 of 9 |
  | 5 | 1 390 ms / 389 / 16 of 200 | 1 607 ms / 609 / 1 of 10 |

  **The cost of a saved kilobyte roughly doubled** — 300–389 ms per 100 KB to 538–665 — and the
  pooled breach rate went **3.1% → 20%**. Two corrections were made to the naive reading at the
  time, and **the first of them is itself wrong; it is corrected in place here, 2026-08-10,
  because leaving it would leave a sign error in the file.** It said: the headless zips are
  ~75 KB *smaller* than the drawn ones of the same world, because the thumbnail is now a flat
  3 666-byte `img.png` (*A headless world's save thumbnail is blank*), so the per-kilobyte figure
  understates nothing. **It overstates, by about 22%.** The dropped `img.png` is an
  already-compressed PNG: it passed through the zip at ~1:1, so it was ~75 KB of the *compressed*
  file and only ~75 KB of a ~1.6 MB *uncompressed* payload. Removing it shrinks the denominator
  of "ms per 100 KB" by a quarter and the work by 5%, and the ratio rises for free. Measured
  against the uncompressed payload the switch moved the cost per byte **not at all** — 795 ms per
  raw MB in the drawn hour before, 800 in the headless hour after. The second correction stands
  as written: **headless is not what did this** — the far end's 672 ms is one headless world on
  its own host, and slot 1's stall barely moved while its file halved. What is left of the
  apparent doubling after the 22% metric artifact is taken out is small, and the ×100 target is a
  weaker lever on the stall than this table made it look.

  **A full thirteen hours of ×100 confirmed it and made it worse than the first hour suggested.**
  Across the five preserved generations of that run — **1,801 saves**, in
  `e2e/logs-m4-lan/bepinex/*.minfps.*` — `event=BUDGET_EXCEEDED` runs at **13.9%**, and the
  maxima are the new information: **4 311 ms** on slot 1, **5 386** on slot 2, **5 870** on slot 3,
  **9 115** on slot 5 and **9 427** on slot 4. Per slot the medians are 686 / 1 530 / 1 699 /
  1 474 / 1 345 ms at breach rates of 0 / 17 / 28 / 16 / 9%, and the cost of a saved kilobyte sits
  at **414–461 ms per 100 KB** — better than the first hour's 538–665 because the worlds grew and
  the fixed part of a save amortises, and still worse than the 300–389 of ×5 drawn. **Nine seconds
  was two and a half times the heartbeat timeout**, so at the 3.5 s of the day those saves were mod
  disconnections by construction, and the dose response below says half of everything over 3.5 s
  took one. The
  worlds absorbed it — `timeoutBounces` 0 for the whole thirteen hours, no `event=FAILED`, and the
  populations *grew* — so this was still a logged breach and a session churn rather than a loss.
  **This table is also the evidence the 13 s deadline was sized from**: 9 427 ms is the worst save
  the deployment has ever logged, and it now sits 3 573 ms *inside* the timeout rather than two and
  a half times outside it (*The decision*, below).
  It was also the strongest argument in the file for answering Risk 3's open question, because
  the ×100 regime is where D14's bar stops being missed narrowly.

  **Taking the minimum-FPS servo off did not help the stall and may have cost a little.** In the
  first fifteen minutes of `0.6.2` the five slots read medians of 820 / 1 771 / 1 664 / 2 952 /
  1 956 ms, with fifteen `4004` closes across the five sidecars — of which five are the deploy's
  own quits. That is 4 to 7 saves per slot, taken minutes after a world reload, against a host
  now running about 1.5× more simulation per wall second: too few and too early to be a verdict,
  and pointed the wrong way. **Re-read it over a full generation before drawing one.**

  **A breach under about 3 s was a logged breach and nothing more; past 3.5 s it disconnected
  the mod.** This is the dose response measured **against the 3.5 s heartbeat timeout, which
  stood until 2026-08-10** — read the paragraph as history, and see *The decision* below for what
  replaced the deadline. Contract A's timeout was 3.5 s and the stall blocks the thread that
  sends the heartbeat, so matching every save against the sidecars' 4004 closes gave a clean
  dose response: **0.2%** of saves under 2 s were followed by a `contract A: heartbeat timeout,
  closing with 4004` within twelve seconds, **1.2%** at 2.5–3 s, **7.0%** at 3–3.5 s and
  **50.8%** of the 63 saves over 3.5 s. The mod reconnects in about a second and the sidecar's
  quiet-mod gate holds `MIGRATE_IN` until heartbeats resume, so each one cost a session churn
  and a short delivery pause rather than an organism. **The dose response does not run the
  other way**: slot 4 took ten `4004` closes in three minutes at 23:43Z on 2026-08-09 with no
  save over 1.82 s behind it (*The reading of 2026-08-09, after the second reboot*), so a stall
  explains some churn and
  is not the only thing that causes it — which is also why raising the deadline cannot be
  expected to take the `4004` count to zero. Nothing else has been observed on the save path: no
  save has logged `event=FAILED` since the species-history guard shipped, and the deferral ladder
  has never run past 5 of 6. **The far end is the control and it is not in this discussion at
  all**: one game on its own host, saving a comparable 354 KB world in 924 ms — and 380 KB in
  **567 ms** at 00:47Z on 2026-08-10, after its own restart — on the mod's own shipped cadence
  of `saveMinutes=10 saveKeep=6`, which the five local worlds now run too.

  **The decision, taken by the owner on 2026-08-10: save less often, and tolerate a longer
  silence. Both halves landed, and neither lowers the bar.** Risk 3 already named the
  escalation — a save that breaks the budget "needs a different cadence, or a save path that does
  not block the tick" — and D14 accepts a slower rig over a lost night. The answer is the
  cadence, plus the observation that the *deadline* the stall kept breaking was never sized for a
  save at all: `3500` was three missed heartbeats plus slack, which measures whether a process is
  alive, and the heartbeat is composed on the same Unity main thread the save blocks.

  | Half | Was | Is | Where it lives |
  |---|---|---|---|
  | Save cadence, five local worlds | `saveMinutes=2 saveKeep=4` | **`saveMinutes=10 saveKeep=6`** — the mod's own shipped defaults | `e2e/run-m4-lan.sh`, assigned after the source in the file's capture-before-source idiom, so a bring-up reproduces it. **`run-m4.sh` keeps 2 and 4** and now says why: phase 8 asserts the rotation layout and needs a run window that contains several saves |
  | Contract A heartbeat timeout | `3500 ms` | **`13000 ms`** | `contracts/contract-a.md` §20 A45 — a §10 default, so **no version bump** (A46) — `go/internal/contracta/contracta.go`, and newly overridable with `--heartbeat-timeout` / `MULTIVERSE_HEARTBEAT_TIMEOUT` |

  **Why those two together.** The cadence cuts the *number* of exposures by five — ten minutes
  instead of two, so about six saves an hour across the five worlds instead of thirty — and costs
  only a longer worst-case rollback, which `saveKeep=6` at ten-minute spacing turns into a
  *deeper* history than `4` at two (an hour of it, against eight minutes). The timeout removes
  the *consequence* of the exposures that remain: 13 000 ms clears the worst save ever logged
  here, 9 427 ms, by 3 573 ms, and stays under `wsPingIntervalMs` (15 000) so the informative
  `4004` — the one that logs a `silentFor` — is still the first to fire. **What it is bought
  with** is later detection of a genuinely dead mod: up to ~16 s including the monitor tick,
  against ~4.4 s before. Through that window the sidecar still publishes `modConnected: true`, so
  no edge closes and no lane re-pairs, and the **quiet-mod gate holds that world's arrivals in
  the journal for the whole thirteen seconds** instead — lossless, in order, bounded by
  `inboundQueueMax` (64). The `4004` it replaces was not free: a session churn, an edge-close
  broadcast over Contract B and a paced replay. **Neither half moves D14's 2 000 ms budget**, and
  §20 A45 says so explicitly, because the mod's own line on every breach still stands: *do not
  lower the bar*.

  **The answer, 2026-08-10: a save costs what its UNCOMPRESSED payload costs, and that payload
  grows with a world's evolutionary history while the zip it produces does not.** The whole
  preserved record was re-read for this — **10,946 `[M4-SAVE] event=SAVED` lines** across every
  generation in `e2e/logs-m4-lan/bepinex/` plus the live logs, joined to the collector's
  **2,160 five-slot snapshots** by exact zip size, which puts each save's stall next to the
  *contents* of the file it wrote. Three findings, in the order they fall out.

  **1. The metric was the trap.** `writeMs` is proportional to the payload's **uncompressed**
  byte total with an intercept of about zero — fitted per hour across all five slots it lands at
  **620–1 580 ms per raw MB** (R² 0.2–0.87 per hour), and no fixed cost worth naming. `SaveSystem.CreateSave`
  deflates every entry at `CompressionLevel.Optimal` through the no-level `CreateEntry` overload
  (`SaveSystem.cs:306,312`), so CPU tracks the bytes going *in*. `bytes=` on the `[M4-SAVE]` line
  is the bytes coming *out*. Dividing one by the other measures the payload's **compressibility**,
  and that is what moved.

  **2. What grew is `speciesData.json`, and it is not organisms.** It is
  `GlobalLineageManager.recordedSpecies` — every species the world has ever recorded, not the
  living ones — serialised by `SerializationHelper.SerializeGeneralObject` into one Newtonsoft
  `JObject` **per brain node and per synapse**, through an uncached `GetFields` per object and an
  `Enum.Parse` per gene per species (`GlobalLineageManager.cs:352`, `BibiteTemplate.cs:268-298`).
  Its cost is `recordedSpecies × (nodes + synapses + NGene)` and is **independent of population**,
  which is exactly why a save costs the same with 7 organisms as with 40. Measured across the
  save files (`speciesData.json` parsed out of 725 collector snapshots):

  | World | `speciesData.json` raw | `data.bin` | of the raw payload |
  |---|---|---|---|
  | `M1-AutoTest.zip`, a fresh world | **5 KB** | 1 KB | 2% of 0.27 MB |
  | `M3-Slot1.zip`, two days old | 44 KB | 36 KB | 14% of 0.58 MB |
  | `M4-Slot1.zip`, 2026-08-10 | **677 KB** | 159 KB | **84%** of 0.97 MB |
  | `M4-Slot2.zip`, 2026-08-10 | **960 KB** | 150 KB | 43% of 2.51 MB |

  Those two entries deflate about **13:1** (969 KB → 74 KB, measured), so 830 KB of new work adds
  ~65 KB to the zip. The raw-to-zip ratio of a save is therefore a **clock on the world's age** —
  **2.4** for a fresh one, **3.2** for `M3-Slot1` after two days, **5.4–7.0** across the five
  local worlds now — and that ratio *is* the "cost per saved kilobyte" curve, upside down. Slot 1
  is the proof rather than the puzzle: same
  population (15 → 18), same file (147 KB → 192 KB), **5.8× the write time** (171 ms → 988 ms) —
  because its uncompressed payload went 0.36 MB → 0.99 MB while its zip did not.

  **And it is not species *count* that is still growing — it is brains.** Parsing the JSON out of
  725 snapshots: `nextSpeciesID` (species ever created) ran 2 162 → 4 832 over 2026-08-07 to
  08-10, while `recordedSpecies` — what actually gets serialised — stayed flat at **84–178** per
  world. `SpeciesTreeCleanup` is already discarding about **97%** of what the lanes create. What
  grew instead is the size of each kept species' brain: synapses per species went **12.6 → 23.0**
  in three days, nodes per species 50.3 → 53.2. That matters for the levers below: pruning harder
  attacks a term that is already pruned, and **the term that is compounding is evolution itself**.

  **3. The split, honestly.** Fitting `writeMs` against brain elements and bytes together —
  **0.07–0.12 ms per (node + synapse)** and **740–1 180 ms per raw MB** of everything else, per
  regime, from 2,088 saves joined to their own snapshot — accounts
  for most of slot 1's 5.8×, and leaves a residual. Of the change from the exit test to now, in
  log terms: **~60% is the payload growing, ~40% is a genuine per-byte slowdown of the host.**
  The residual is real and it is *not* monotone — it is common-mode across all five worlds hour by
  hour (the five slots move together at the same wall-clock times, which is what made "the host"
  the natural first reading), it is **not reset by a game restart**, and it *is* partly reset by a
  host reboot: 926 → 620 ms per raw MB in the four hours after the 2026-08-09 reboot, then a slow
  climb back to ~850 over the next twenty hours. That is where the "a reboot gives back 40%"
  observation actually lives. **Two things it is not**: it is not save concurrency between the
  five instances (de-trended, a save with three neighbours saving within ±10 s costs 0.99× one
  with none, across 10,946 saves), and it is not host simulation demand (the log-log correlation
  against Σ time-scale × population is **−0.26**, the wrong sign).

  **What this makes actionable — the owner's call, not this file's.** Nothing here is a defect and
  nothing was changed to chase it. But the shape of the cost is now known, and it names its own
  levers: the cadence, already taken, and the only one that scales with the whole cost;
  `SpeciesPrunePointLimit` — the game setting that decides which extinct species
  `SpeciesTreeCleanup` may drop, default **2** — which the measurement above says is a **weak**
  lever, because 97% is already being dropped; and Risk 3's unspent escalation, a save path that
  does not block the tick, which is the only one that touches the *stall* rather than the cost.
  What it also means is that **the stall will keep growing on its own**, because brains only get
  bigger — so a budget set against a young world is a budget that expires, and D14's 2 000 ms was
  set against worlds that were seconds old.

  **The instrument, mod `0.6.3`, deployed 2026-08-10 17:33Z.** `writeMs` was one opaque number
  covering the whole of `CreateSave`, which is why every hypothesis above argued over the same
  undivided total. `SavePhases` (`bibites-mod/src/SavePhases.cs`) Harmony-times four spans from
  the inside and every `[M4-SAVE] event=SAVED` line now carries them, plus `verifyMs`, which was
  previously the unlabelled remainder of `stallMs`:

  | Field | What it times | Read it for |
  |---|---|---|
  | `lineageMs` | `GlobalLineageManager.SaveState()` — building the whole `speciesData.json` tree | finding 2, from the inside. Pure CPU and allocation; no file is touched |
  | `binMs` | `BytesSpace()` ×2 + `SaveStateBin()` — the `data.bin` lineage block | the other history-shaped term |
  | `guardMs` | `SpeciesHistoryGuard`'s repair, **inside `binMs`** — subtract it to price the game's own bin work | what the guard costs. It runs **three** times per save, not twice: `SaveableBinStack` asks `BytesSpace` twice and then `SaveStateBin` |
  | `shotMs` | `SaveScreenshotToZip`, exclusive of the `img.png` archive write | what a blank headless thumbnail costs — the `Texture2D`, the `Apply` and the PNG encode all still run |
  | `zipMs` / `zipN` | every `WriteJObjectToArchive` / `WriteFileToArchive` call, and how many | string materialisation + deflate + write — the disk-and-deflate share, as against the CPU-and-reflection share above it |
  | `verifyMs` | the mod's own `Verify` — re-opens the zip and reads its central directory | the one part of the stall that scales with population, one record per bibite and per egg |

  `writeMs − (lineageMs + binMs + shotMs + zipMs)` is the remainder of `CreateSave`: settings, one
  JObject per bibite, egg, pellet and pheromone, and `AddTemplatesToArchive`'s `File.ReadAllText`
  + `JObject.Parse` **per scenario template per save**. All of it is measurement — `SavePhases`
  changes nothing about what a save does, and a span it cannot resolve logs one `[M4-PHASE]` error
  and reads `0` thereafter, which a real save can never do because `zipMs` cannot be zero.
  **The rollout was the `0.6.2` shape** — a mod deploy locks the DLL, so all five games go down at
  once — and took **110 seconds** end to end at 17:33Z: quit five, `deploy.sh`, then `start_game`
  one at a time with readiness read off `/api/status` rather than a log file, and `timescale 100`
  sent twice per slot. All five came back on `0.6.3` at a clean ×100. **The custody burst was not
  sampled at its peak** — the first reading is two minutes after the last slot came up, at 1–12,
  and eight minutes after at 1–7 — with `pacedDepth`, `heldDepth` and `timeoutBounces` 0 at both,
  and no `4004` in any local sidecar log. Slot 6 was not touched, and still publishes `0.6.2`.

  **The first reading confirms finding 2 from the inside and kills three of the four named
  suspects.** Twenty-six instrumented saves, 17:43Z–18:36Z. **Read only `n≥2`**: the first save of a
  process carries the JIT of the whole path, and it is not small — `shotMs` reads **483–585 ms on
  `n=1` and 8–14 ms from `n=2` onwards**, which is a warm-up artifact and a trap set for anyone
  who reads a save taken minutes after a bring-up. Across the twenty-one warm saves, with the
  per-world medians in the last column:

  | Phase | Median ms | Share of `writeMs` | Per world (slots 1–5) |
  |---|---|---|---|
  | **`lineageMs`** | **994** | **53%** | 653 / 976 / 1 136 / 1 028 / 1 228 |
  | remainder (bibites, eggs, pellets, templates) | 527 | 28% | 169 / 934 / 1 331 / 533 / 598, tracking population 10 → 85 |
  | `zipMs` | 211 | 11% | 78 / 270 / 365 / 224 / 213 |
  | `verifyMs` | 24 | 1% | 17–33 |
  | `shotMs` | 10 | 1% | 8–14, one 30 |
  | `binMs` | 1 | 0% | 0–2 |
  | `guardMs` | 0 | 0% | 0–1 |

  **The reading is stable.** Recomputed at ten, fifteen and twenty-one warm saves the `lineageMs`
  median held at 977 / 994 / 994 ms and its share at 58 / 55 / 53% — the shares drift only because
  the populations grew through the window, which is the remainder's term and not this one.

  **`speciesData.json` is the save.** `lineageMs` is the largest phase in every world, and it is
  the *flattest* — 653 to 1 228 ms while populations run 10 to 85 and zips run 149 to 809 KB.
  Slot 1 is the whole argument in one line: **10 organisms, a 149 KB file, an 873 ms stall, of
  which 676 ms is building the species history**. Three of the four candidates this item carried
  are now measured and closed: **the screenshot is 1%** (the headless thumbnail is wasteful and
  it is not expensive — ~10 ms, see *A headless world's save thumbnail is blank*), **the
  species-history guard is 0–1 ms** (its three passes per save cost nothing worth naming), and
  **`data.bin` is 0–2 ms** despite being 150 KB, because it is `Buffer.BlockCopy` into one
  pre-sized array rather than reflection. The fourth — managed-heap/GC state with process uptime —
  is not settled, but it is now **bounded to where it can hide**: `zipMs` is a tenth of the cost,
  so the residual host multiplier is CPU and allocation, not disk. `lineageMs` and the remainder
  are both reflection-and-JObject churn, and both are where to look next. **That last inference is
  wrong and is corrected under item 2 below** — it argues from a phase's *share*, and the 2 h 47 m
  reading of 20:20Z catches an excursion that took `zipMs` with it at ×1.37. The bound is the whole
  process, not a phase.

  **What to watch now, in this order.**

  1. **Do the `4004`s stop? — read 2026-08-10 20:20Z: no save has cost a session since the raise,
     and the sample is still too thin to call it a rate.** The proof is a `[M4-SAVE]` stall over
     3.5 s with **no** `contract A: heartbeat timeout` behind it in
     `e2e/logs-m4-lan/sidecar-*.log`. Across the **76 instrumented periodic saves** of 17:43Z to
     20:20Z — every save of the current five game processes, which all started at 17:33Z — plus the
     13 already recorded at 15:40Z–16:26Z:

     | | |
     |---|---|
     | Saves observed since the raise | **89** (76 instrumented + 13 earlier) |
     | Of them over the retired 3 500 ms | **3** — slot 2 at 4 207 ms (15:52Z), slot 3 at 3 686 ms (17:45Z), slot 5 at 3 613 ms (19:22Z) |
     | `4004` closes attributable to any of those three | **0** — nothing within ±60 s on the save's own slot |
     | Saves over the new 13 000 ms | **0**, and nothing near it |
     | Worst stall in the 76 | **3 686 ms**, against the 9 427 ms this deadline was sized from |

     **So `M`=3 and `K`=0.** That is the right sign and it is not yet a rate: the 13-hour generation
     that produced the problem had 63 saves over 3.5 s and this one has three, because **the tail
     collapsed**. Per slot all 76 saves read medians **1 048 / 2 162 / 2 804 / 1 984 / 2 513 ms**
     with maxima 1 876 / 3 021 / 3 686 / 2 869 / 3 613 — so against the `*.minfps.*` generation the
     **medians rose** (686/1 530/1 699/1 474/1 345) while the **maxima fell by a factor of three**
     (4 311/5 386/5 870/9 427/9 115). The ten-minute cadence did not make the typical save cheaper —
     the worlds are bigger, and *The answer* says why — but it removed the multi-second outliers that
     the old deadline turned into disconnections. **43 of 76 (57%) still breach D14's 2 000 ms**, which
     is item 3.
     **What the raise has not bought is a quiet log**: 21 `4004` closes have been logged since
     15:39Z, and **every one of them is attributed elsewhere** — three to the sidecar rollout itself,
     three to the `0.6.3` deploy's own quits at 17:33Z, and **fifteen to the host-starvation storm of
     18:39Z–18:59Z**, whose `silentFor` ran 13.40–24.41 s with no save over 3 006 ms anywhere near it
     (*The evening's two session storms*). That storm is item 4, not this item.
     **A gap in the evidence, and it is permanent.** 16:26Z to 17:43Z — roughly 33 saves — cannot be
     read: the `0.6.3` deploy restarted the games without archiving the BepInEx logs first, and
     BepInEx truncates `LogOutput.log` on launch. The newest generation in
     `e2e/logs-m4-lan/bepinex/` is still `…minfps.20260810-1058*`. **Run `archive_bepinex_logs`
     before any mod deploy**, or that deploy destroys the `[M4-SAVE]` history it is being deployed to
     measure.
  2. **Does `lineageMs` climb over a full generation?** The first reading already has it at ~53%
     of `writeMs` and flat against population; the question the next generation answers is its
     *slope*, because that is the term predicted to compound. **The first 53 minutes cannot answer
     it and should not be read as if they did**: pooled by save ordinal the warm medians go
     994 → 936 → 1 106 → 1 096 ms, which is noise, and per world three of five trend up (slot 5
     1 099 → 1 531, slot 3 1 026 → 1 232, slot 4 960 → 1 096), slot 2 is flat and slot 1 wobbles.
     Five saves per world across 0.2 simulated days is not a slope. Read it against
     `simulatedTime`, not against wall clock or population. Two rules for anyone reading these
     numbers: **skip every `n=1`** (JIT — `shotMs` is 50× too high on it), and **compare per raw
     MB, never per zip kilobyte** — the old `ms per 100 KB` figures (`*.minfps.*` at 414–461) are
     comparable only within a drawn-or-headless-matched set, and that trap is what the previous
     reading fell into.

     **Re-read on 2 h 47 m at 2026-08-10 20:20Z — 69 warm saves joined to their own collector
     snapshot. The window still refuses the slope, and now it says why.** Normalised per raw MB and
     binned by wall clock, with all five slots in every bin:

     | 20-min bin | `lineageMs` | remainder | `zipMs` | `verifyMs` | `writeMs` | `lineage`+rem share |
     |---|---|---|---|---|---|---|
     | 17:40 | 416 | 299 | 93 | 10 | **811** | 88% |
     | 18:00 | 496 | 232 | 85 | 11 | 831 | 88% |
     | 18:20 | 486 | 251 | 106 | 11 | 838 | 88% |
     | 18:40 | 457 | 301 | 98 | 11 | 918 | 82% |
     | 19:00 | 518 | 240 | 97 | 13 | 878 | 86% |
     | 19:20 | **791** | **349** | **123** | 16 | **1 267** | 90% |
     | 19:40 | 632 | 336 | 113 | 14 | 1 099 | 88% |
     | 20:00 | 630 | 262 | 111 | 14 | 1 015 | 88% |

     **Three things fall out, and the third is the useful one.**

     **The residual does not live in `lineageMs`.** The 19:20Z excursion multiplies *every* phase by
     about the same factor — `lineageMs` ×1.64, the remainder ×1.41, `zipMs` ×1.37, `verifyMs` ×1.53,
     `writeMs` ×1.54 against the 17:40+18:00 baseline — and `lineage`+remainder holds **82–90% of
     `writeMs` in every bin** across the whole 2 h 47 m. The shares do not move; the whole save
     scales. **This corrects the bound in the paragraph above**: "`zipMs` is a tenth of the cost, so
     the residual is CPU and allocation, not disk" was an argument from *share*, and a share cannot
     make that case — when an excursion actually arrived it took the deflate-and-write phase with it
     at ×1.37. What the residual is bounded to is **the whole process**, not a phase of it.

     **The slope cannot be attributed, and the reason is design, not sample size.** All five games
     started at 17:33Z, so **process uptime and host wall-clock are the same variable in this
     sample** — cost against save ordinal gives r = +0.51 and against wall seconds r = +0.52, which
     is one measurement reported twice. Worse for the aging hypothesis: the series is **not a ramp**.
     It is flat at 811–838 for the first hour, steps to a 1 267 peak at 19:20Z, then **decays** to
     1 015. Heap fragmentation does not undo itself, so a decaying excursion is evidence *against*
     process aging. **Separating the two needs staggered game restarts**, which no rollout has ever
     used — every one takes all five down together or walks them in seven minutes.

     **What it is not: concurrent load.** Against the concurrent crossing rate, over a window where
     the rig ran **741 to 1 417 migrations a minute**, r = **+0.11** — nothing. Against population,
     r = **−0.005**, which re-confirms *The answer*'s central claim from a third direction. So the
     rig running hot is **not** what makes a save expensive.

     **What the excursion does line up with is the host, and the host is now crowded.** The 19:20Z
     peak is the 10–30 minutes after the archive's 5.2 GB replay (*The archive's ledger recovery*),
     and it follows the 18:39Z starvation storm. Read-only process facts taken at 20:20Z: the five
     games hold WorkingSet **1 954–2 450 MB** each — against the **403–466 MB** measured when they
     went headless at 01:30Z — and have burned **10 315–13 971 s of CPU in 9 960 s of wall clock**,
     which is 1.04–1.40 cores each and **~5.9 cores across the five**. Beside them on the same box:
     the archive at 1.25 GB, the relay at 344 MB, and ~6 GB of editor, node and agent processes doing
     the measuring. **The "40% host residual" has a candidate that is not the game at all**, and it
     is the reason the next reading of this term should record what else the box was running.
  3. **The 2-second budget is still breached, and that is still why this item is open — and it
     will get worse on its own.** The bar was not moved. What changed is that breaching it now
     costs only the throughput D14 priced, not a session as well. But `recordedSpecies` and its
     brains only accumulate, so *The answer* predicts the stall keeps climbing at unchanged
     population and unchanged file size. Risk 3's remaining escalation — *a save path that does not
     block the tick* — stays available and unspent, and it is now the escalation that scales.
  4. **The residual churn — and as of 2026-08-10 18:39Z it is no longer a footnote.** The 23:43Z
     slot-4 episode had no long save behind it, so some `4004` closes come from somewhere else. With
     the save-shaped ones gone, whatever is left became separable — and the first thing it did was
     produce **every one of the 21 `4004` closes logged since the raise**, none of them a save.
     **The 18:39Z–18:59Z storm is the same mechanism at five times the scale**, and it settles one
     thing the 23:43Z episode could not: it dropped **thirty Contract B sidecar sessions** for
     silence as well as fifteen mod sessions, and a Go sidecar on a loopback socket has no save path,
     no Unity main thread and no species history. **So the residual churn is not a mod problem and
     never was** — it is this host failing to schedule its processes, and it is bounded to this host
     because slot 6 logged nothing (*The evening's two session storms*). That makes the next
     diagnosis a **host** one: record contemporaneous load, not more mod instrumentation. The
     13 s deadline is doing its job here in the only way it can — it did not prevent these closes and
     was never going to, and `timeoutBounces` stayed 0 through all of them.

- **Slot 1 is depressed, and it is the world to keep an eye on.** It fell to a population of
  **2** in the 2026-08-08 ENOSPC incident and did not recover with its neighbours: readings
  of 3, 10, 8 and 7 through 2026-08-09 put it inside its 6–15 recovery range but at the
  bottom of it, while the other four local worlds held 22–25. It then read **14** at 20:21Z
  the same day, so it *is* climbing — and it is still the smallest of the five by a factor of
  two and a half. A small world produces few young of its own and depends on arrivals; the
  open question is whether the lanes alone carry it back to its neighbours' range or whether
  it settles low. **The ×100 switch made it the fast one and did not make it the big one.**
  It was 18 an hour before, dropped to 1–3 in the minutes when it alone ran at ×100 among four
  worlds still at ×5 — it exported into them 20× faster than they exported back, and the log
  shows 98 `MIGRATE_OUT_SENT` against 0 arrivals in that first minute — and then oscillated
  **2–22** once all five were switched. That range is its old one sampled far more finely:
  ×100 compresses a population cycle into a minute of wall clock, so a single low reading now
  means much less than it did. Being the smallest is also why it is the only world that
  *reports* its ×100 target (*The five local worlds run headless and target ×100,
  since 2026-08-10*); the four bigger ones are clamped.

  **Read again over 2026-08-10 17:30Z–20:20Z, 33 collector samples: it oscillates 4–23, median 13,
  and that is the same band as the first hour's.** Seven of the 33 samples sit at 4–9 and four sit at
  17–23, so a spot reading of 4 and a spot reading of 23 are both ordinary and neither is a trend —
  which is the point already made above, now with a longer series behind it. It has not climbed to
  its neighbours' range: slots 2–5 read 41–63 at 20:18Z against slot 1's 17, so it is still
  the smallest by a factor of three to four. **What has changed is that the question is getting
  harder to answer, not that the answer arrived** — at an achieved 25–32× against its neighbours'
  6–10× (its populations stay small, so `CheckMinFPS` never clamps it) slot 1 lives several times
  more simulated time per wall second than the worlds feeding it, so its low readings are a fast
  world waiting on slow neighbours as much as a small world waiting on arrivals. Reading it against
  `simulatedTime` rather than wall clock is the way to separate those, and nothing has done that yet.

- **`genomeGaps` is a fetch queue, its healthy value is 0, and what it counts is
  throughput.** The field is `len(a.pending)` (`go/internal/archive/status.go`, over the
  map declared in `go/internal/archive/archive.go`): the set of genome hashes the archive
  has asked a sidecar for and not yet received. It is a **backlog count, not an error count**,
  and it is not the same thing as `bin/archive list --gaps`, which reports what the *ledger*
  shows missing. An entry is created only for a hash the store does not already hold
  (`trackLocked`) and leaves only when the genome arrives. Since §21, B21 the map has a
  **companion round-robin order** (`pendingOrder`) that the pump walks a bounded slice of per
  tick; `genomeGaps` still counts the map, so the number on the page is unchanged.

  **The baseline exists, and it is zero.** The gitignored collector has curled `/api/status`
  every ~5.5 minutes since 2026-08-07 (`e2e/baselines/m4-collector/`), and that series is the
  one this item used to ask for:

  - **2026-08-07, 14:07Z to 19:13Z:** `genomeGaps` reads **0 to 2** for five unbroken hours,
    across 56 samples, while crossings stay under 200 a minute throughout — mostly 130–190.
    That is the healthy shape, and it is flat zero.
  - **19:19Z the same day:** the crossing rate more than doubled, to 454/min, and the backlog
    left zero in that same sample — 22, then 44, 88, 152, and 1,071 by 21:00Z.
  - **2026-08-08:** the same rise and the same recovery, peaking at **3,334** at 12:26Z and
    already down to 2,939 two hours later.
  - **2026-08-09:** 855 when the collector came back after the reboot at 18:51Z, climbing to
    **2,714** at 22:05Z on 400–520 crossings a minute, then a **monotone drain** to **1,110**
    by 23:56Z as the rate fell to 140–340 — while `ledgerRecords` went on growing by about 500
    lines a minute through the entire drain. A leak does not drain against traffic. The drain
    then carried straight through the far-end restart to **552** by 00:52Z on 2026-08-10, ten
    more unbroken samples at 230–305 crossings a minute, which is the confirming case.
  - **2026-08-10, from 01:19Z: the ×100 switch is the loaded case this item predicts, and it
    behaved like one.** `genomeGaps` read **291** at 345 crossings a minute just before the
    rollout and climbed to **1,071** by 01:55Z, with the rate settled at 603–781/min — above
    the ~400/min growth threshold for the whole window. `ledgerRecords` grew from 1,145,694 to
    1,200,972 across the same half hour and nothing else moved: 24/24 lanes, all queues 0, no
    errors. **A climb under a doubled current is the mechanism, not a fault** — and the regime
    now sits above the threshold most of the time, so expect a standing backlog rather than the
    flat zeros of 2026-08-07.
  - **2026-08-10, thirteen hours later: `≈46,000` against `ledgerRecords` 2,824,211**, on a
    crossing rate holding a median **949/min**. That is an order of magnitude past any earlier
    reading and it is still the same curve — the rate never dropped under ~340/min for long
    enough to drain, because at ×100 it no longer does. **The magnitude is not the finding; the
    absence of a drain window is.** Nothing else moved with it: 24/24 lanes, `pacedDepth` and
    `heldDepth` 0, `timeoutBounces` 0, one `no peer has served this genome` line in all of
    history, and `ledgerRecords` more than doubling across the same span. **What this reading does
    change is the M5 arithmetic**: a queue of this size is a very different thing to inherit when a
    departed stranger can make an entry permanent, so read Risk 7 against the **current** reading
    below rather than against the 2,714 the item was opened on.
  - **2026-08-10 evening: still the same curve, still no drain window, and the backlog has now
    started to cost something other than its own size.** Over 17:32Z to 20:20Z, 31 collector samples
    at a crossing rate of **806–1,417/min**, `genomeGaps` climbed **49,461 → 62,850** and read
    **63,996** on a live poll at 20:31Z; `ledgerRecords` climbed past **3,889,000** across the same
    span. The one step in the series is not growth: **56,658 → 60,675 across the 19:08Z archive
    restart**, which is the replay bringing its own unfetched hashes with it, exactly as *The
    archive's ledger recovery* records. The rate never fell near the ~340/min drain threshold at any
    point, so the test this item wants still has not come round.
    **The magnitude cost a session, twenty-six of them, and that half is CLOSED.** From 20:22:52Z
    to 20:52:23Z the archive could not hold a relay session, and the engine was this queue — but not
    in the way it was first written here, and the correction matters because it is the difference
    between a pacing problem and a starvation one. `pumpFetches` walked **all** ~64,000 `pending`
    entries on every one-second tick, holding the archive's own `mu` — the lock its frame handler
    needs — and calling the genome store's `Has`, which is an **`os.Stat`**, on each. That is
    284 ms–1.02 s of held lock per second measured, so the read loop was admitted about once per
    pass, the **relay's** 128-frame outbound queue to the archive filled in ~2.8 s at the
    1,378/min the rate had reached, and the relay closed the session with **1011 "outbound queue
    full"**. The burst is real and was **not** the killer: `GenomeRequestsPerMinute` = 30 bounds the
    **rate** and not the **burst** — `allowSendLocked` opens a fresh one-minute window per peer and
    then admits 30 immediately — so one pass fired **180 requests in 33–176 ms**, but 180 frames
    never troubled the archive's own 256-frame queue and the bursts do not line up with the session
    deaths. It cost the **ledger 9,641 crossings** and the map nothing. The fix is
    `contract-b-m4.md` **§21, B21**, deployed 21:08:50Z: the pass is bounded at
    `genomeScanPerTick` = 2048 entries walked round-robin over a stable order, the lock is released
    every `genomeScanChunkSize` = 256, and the **burst bound** is `genomeRequestsPerTick` = 8 with
    `genomeInFlightPerPeer` = 8 — none of which moves the retry ladder. Archive CPU fell from a full
    core to **8.1%** and the flap ended. Full account in *The evening's two session storms*.
    **What stays open is the backlog, not the flap.** The queue goes on growing, no drain window has
    come round, and the cost of carrying it is now bounded rather than absent. **Read this against
    Risk 7**: M5's permanent entries make the queue monotone, and §21's bounds are what stop a
    monotone queue making each archive reconnect more expensive than the last.
  - **2026-08-11, and this is the current reading: `116,803` to `117,418`**, against a ledger of
    **6.24 M records**, still with §21's bounds carrying it and still with no drain window. **It
    roughly doubled again in a day** — the ≈46,000–65,000 recorded through 2026-08-10 is one day
    old and reads about half the live figure. **Nothing about the finding moved,
    only the magnitude**: the queue is throughput and not a leak, the crossing rate has not sat
    under ~340/min for long enough to drain at ×100, and the drain test this item wants still has
    not come round. So this is the number **Risk 7** is read against — and the doubling is the
    reason to carry the *mechanism* rather than the reading, because any figure written on this
    line is low within a day. `ledgerSkippedLines` was re-checked in the same samples: **still
    exactly 1**, the single 776-byte splice of 2026-08-09, with no second damaged line since.

  **The mechanism is a rate cap, and it is deliberate.** The archive may send at most
  `GenomeRequestsPerMinute` = **30** genome requests a minute **per peer**
  (`go/internal/contractb/contractb.go`, enforced by `allowSendLocked` in
  `go/internal/archive/archive.go`), so the ladder is bounded near **180 fetches a
  minute** across six peers. Only a crossing carrying a hash the store does not already hold
  costs a fetch, which is why the *crossing* rate that meets that ceiling is about twice it:
  on this rig the backlog grows above roughly **400** migrations a minute and drains below
  roughly **340**. Under the ceiling the field reads 0; over it, `pending` **is** the queue,
  and it is worked off at the same cap when the current slows. The number to read is
  therefore the *direction* against the current rate, not the magnitude.

  **A rate is not a budget, and since 2026-08-10 there are three more bounds beside it**
  (`contract-b-m4.md` §21, B21). The rate cap says nothing about how much work one pass over
  the queue may do or how many of a minute's requests may leave at once, and at 64,000 entries
  both of those cost the archive its relay session twenty-six times. So: `genomeScanPerTick`
  = **2048** entries examined per pass, round-robin over a stable order and resumed where the
  last pass stopped; `genomeScanChunkSize` = **256** per acquisition of the archive's lock, so
  the read loop is never starved; **`genomeRequestsPerTick` = 8, which is the burst bound**;
  and `genomeInFlightPerPeer` = **8**. All four sit above what the 30-per-minute rate can
  sustain, so **the achieved fetch rate is unchanged at ~175/min** — what changed is that it
  arrives 8 a second instead of 180 in one 50 ms window. **None of them moves the retry
  ladder.** The rate is still the number that decides whether the backlog grows.

  **The two spot readings that opened this item sit on that curve** — **881 against 878,904
  ledger records** after the reboot is the collector's 855 at 18:51Z, and **2,467 against
  959,243** at 20:21Z is its 2,473 at 20:19Z. (Both `ledgerRecords` figures are this process's
  own count, which the pre-fix replay started **197,195 records short of the file**; the next
  restart raises the denominator by that much in one step, so a ratio taken across it is not a
  series. See *Bringing it back after a reboot*.) The two further samples minutes apart later
  the same hour, **2,477** and **2,503**, are the same rising limb; the peak came at 22:05Z
  and the drain followed it.

  **Exactly one hash has ever run out of peers.** `archive: no peer has served this genome` —
  the §10 gap report, logged when every live peer has been asked in one round and none served
  it — appears **once** in the whole log history (`e2e/logs-m4-lan/archive.log`,
  2026-08-08T14:14Z, `attempts=6`, `retryIn=24h`, one hash). The reason is that every peer on
  this rig comes back, so the ladder always eventually finds a holder.

  **It stays a watch item because M5 changes the ending, not the rate.** An entry leaves
  `pending` only when the genome arrives, so **a gap becomes permanent if the peer that holds
  the blob is gone for good** — and after M5 a departed stranger makes that the normal case,
  against a fetch ladder that runs from one minute to daily and a sidecar cache capped at 30
  days and 2 GiB. `m5_considerations.md` **Risk 7** carries that case and its mitigations. On
  this rig, watch the direction: a backlog that does not drain when the crossing rate falls
  below ~340/min is the reading that would mean something new. Since the ×100 switch that test
  comes round less often, so take it when it does — a lull, a restart, or a deliberate
  `send <n> timescale 5` on one world.

- **The BepInEx log archive has no retention rule, and it is now the fastest-growing thing on
  the volume. Opened 2026-08-11.** `e2e/logs-m4-lan/bepinex/` went from **3.4 GB to 21 GB in the
  single crossing window**, because the five live `LogOutput.log` files were 6.08 GB and a
  crossing copies them three times — once by hand at P1, then inside `lan_down`, then inside
  `lan_up` (*The disk budget*). Nothing sweeps it, and the copies are not redundant in any way a
  script can see: each is a different generation of a file BepInEx truncates on launch.
  **It is not a defect and it cannot simply be deleted** — this directory is the *only* record of
  `[M4-SAVE]` history, which is the evidence the first watch item is built on, and the one time
  archiving was skipped the loss was permanent (~33 saves, 2026-08-10). **What is missing is a
  rule**, and it is an owner's call of the same shape as the collector's: which generations are
  keepers, what prunes, and whether an archived generation should be compressed in place the way a
  retired rig directory is (`e2e/<name>.tar.zst`, *Layout*). **Watch the volume, not the
  directory**: `/mnt/wsl/data` went 39 GB → 56 GB used across the window and has 195 GB free, so
  the pressure is real but slow. At roughly 18 GB per crossing window and ~2.3 GB/day of ordinary
  growth, this is the first term that would make the *three months* estimate in *The disk budget*
  wrong.

## Gotchas

- **The rig lives on `/mnt/wsl/data`, reached through a symlink.** `~/bibites-multiverse`
  is a symlink to `/mnt/wsl/data/bibites-multiverse`. If that volume is not mounted the
  symlink dangles and every path fails with *No such file or directory* — see *The disk
  budget*, *Where the rig lives now*.
- **A full disk does not just stop the rig, it can silently truncate an append-only log's
  history.** One short write leaves an unparsable line mid-file and replay discards
  everything behind it. Fixed in the sidecar journal on 2026-08-08 (all-or-nothing appends,
  and `Discarded()` logged at error) and in the archive's ledger on 2026-08-09 (the same
  append rule, and a replay that *skips* the damaged line instead of stopping at it). **The
  day between those dates is the lesson**: the rule was written for one implementation while a
  second one, holding the record of everything that ever crossed, had the identical bug and
  nobody looked. Know the shape before trusting any append-only log here. See *The disk
  budget*.
- **Target `netstandard2.1`**, not 2.0 — Unity 6 assemblies reference netstandard 2.1
  and the build fails with CS1705 otherwise.
- **MSB3277 version-conflict warnings are benign** — the game's Mono runtime resolves
  assemblies at runtime; don't chase them.
- **Never launch the game with `cmd.exe /c start` from a foreground WSL command** —
  the game lands in that shell's Windows process tree and dies when the WSL command
  is killed (this is why the first smoke test's game exited). `game.sh start` uses
  PowerShell `Start-Process`, which detaches properly.
- `Program Files (x86)` is writable from WSL without elevation (Steam's ACLs).
- Windows interop tools need a Windows-side cwd: `cd /mnt/c` first (the scripts do).
- `libs/` DLLs and `decompiled/` are copies of one game build. `sync-game-refs.sh`
  re-copies the DLL set and regenerates the decompile in one step; run it instead of
  copying by hand.
- **Steam auto-updates stay on** — the toolchain detects drift rather than pinning a
  version. `sync-game-refs.sh` compares the game's `BibitesAssembly.dll` hash against
  `libs/` (and reports the Steam buildid); `deploy.sh` runs the same cheap check and
  warns before building against stale references. After any re-sync, re-verify mod
  code against the changed APIs. **An update also costs the map, not just the build:** the two
  computers update at different moments, and a game-version mismatch refuses organisms and
  closes the edges between the mismatched pair until both are on the new build — kept
  behaviour, by the owner's call of 2026-08-11. Re-sync, rebuild and re-take the far-end bundle
  promptly; *The far end* states the ritual and what the partition looks like.
- **Stop the game before you deploy.** With the game running, `deploy.sh` fails at the
  copy step with `Invalid argument` — Windows holds `BibitesMultiverse.dll` open. The
  `dotnet build` before it still succeeds, so the output reads like a success until the
  last line. Run `game.sh stop` first, then `deploy.sh`.
- **The FIRST bring-up after a deploy that adds a config entry races on `dev.multiverse.bibites.cfg`,
  and the instance that loses sits at the main menu with the client OFF.** BepInEx writes the whole
  config file back on every *new* `Bind`, so a mod release with a new entry makes all five instances
  rewrite the same file within seconds of each other. Two of them lose:
  `[M2] configuration failed — the multiverse client stays off: System.IO.IOException: Sharing
  violation on path …\BepInEx\config\dev.multiverse.bibites.cfg`. **The symptom does not look like
  a config problem** — the game loads, BepInEx says `Chainloader startup complete`, dev commands arm,
  and then nothing: no `[M2-WORLD]`, no `MULTIVERSE_WORLD` load, no sidecar connection, and `up`
  blocks in `world_ready` for its full 1200 s. Measured 2026-08-07 rolling out `0.6.1`: slots 1 and 2
  both lost, slots 3–5 were fine. **The fix is a restart, not a repair** — the file is complete after
  the first winner writes it, so no later `Bind` saves and the race cannot recur. Stop the affected
  instances and start them **one at a time**, with the same environment `run-m4.sh start_game` sets;
  a running `up` picks them up where it is waiting. Check for it with
  `grep -a 'configuration failed' "…/BepInEx/LogOutput.log"*` whenever a slot is `live` on the status
  page with `modConnected` false. **A `modConnected` false with no `configuration failed` anywhere
  is the other failure with the same symptom** — the log-file starvation trap, in *The
  five-instance ceiling* below, which has a different remedy.
- **A world can be at the wrong time scale, and the rig's own `timescale` may not stick to
  it. Read every instance, not only the one you touched.** The game restores its own speed
  after the world settles, which can land after the rig's `send <slot> timescale <x>`. Re-send
  it and confirm on `/api/status`; the command echoes both halves, as
  `targetTimeScale=100.00 Time.timeScale=100.00`. **The target for the five local worlds is
  ×100 since 2026-08-10** (*The living deployment*), so that is the number a correction sends;
  the readings below that say `×5` are the record of when the target was ×5, not stale advice.
  **`timeScale` on the page is the speed the game
  is *applying*, not one this rig measured** — the mod copies `Time.timeScale` into the
  heartbeat (`MultiverseClient.cs:1040`) and `contract-a.md` calls it *the effective time
  scale*. **Until 2026-08-10 that was the only figure the page had, and it was silent about the
  gap. It now shows both**, per world, as `×100 → ~×12`: the applied scale, then the achieved
  rate the archive measures over the last minute (`achievedTimeScale` on `/api/status`, the
  `speed→got` column in `ringstat`). Everything below still holds — it is *why* the second
  number exists — but "read the achieved rate too" is no longer a separate step an operator has
  to remember to take. The archive computes it from `Δ simulatedTime / Δ statsAsOfMs` on the
  `PEER_STATUS` blocks it already receives; **the paragraphs below that read a rate off a
  stopwatch are the record of how it was measured before the page did it.**
  The game's own governor is what makes it move: `TimeController.CheckMinFPS` clamps
  the engine scale into `[min(0.1, target), target]` whenever the frame rate misses
  `minimumFPS`, so a world that keeps up reports its target exactly and a host that cannot
  reports the fluctuating non-integer it was throttled to. Neither figure is the achieved rate,
  because `UpdateTimeScale` also pins `Time.maximumDeltaTime` to one fixed step: at 00:47Z on
  2026-08-10 the five local worlds reported `5` and advanced 4.94–5.00× per wall second, while
  slot 6 reported `6.5` and advanced 5.84×. **At a target the host cannot meet the two figures
  come apart completely, and the gap is where the news is.** Under the ×100 switch the same
  evening, over 16 samples: slot 1 (5–10 organisms) reported its `100` and advanced 27–68×,
  while slots 2–5 (20–60 organisms each) reported a fluctuating 11–81 and advanced 4.6–25×.
  **So a target is an ask, an applied `timeScale` is what the governor allowed, and only
  `Δ simulatedTime / Δ wall` is what happened** — take the third before believing a rate.
  Three measurements say the scope of the *drift* is wider than
  "the instance you just restarted", and each widened it:
  - after the 2026-08-07 config-race restart, slots 1 and 2 reported `×100` and `×58` while
    the three the rig started reported `×5`;
  - after the 2026-08-08 reboot, the single restarted instance (slot 1) rejoined at `×100`
    and needed `send 1 timescale 5` — but the correction is **intermittent**, because after
    the 2026-08-09 reboot the restarted instance came back at `×5` and none was needed;
  - and at 20:21Z on 2026-08-09, hours into a settled map, **slot 2 reported `×34.7` while
    the other four still reported `×5`** — no restart involved at all. A plain
    `send 2 timescale 5` corrected it and it held, so the one-command remedy does not depend
    on what caused the drift.

  **The 2026-08-11 crossing is the fourth reading and it needed nothing**: all five worlds came
  back from a full `down`/`up` already reporting `timeScale 100`, so the documented re-send
  changed nothing and the *first-send-answers-`1.00`* trap never fired. That is two bring-ups in
  a row with no correction needed — which is exactly why the sweep, and not the correction, is the
  deliverable.

  So it is not a property of a restart. Sweep all five on `/api/status` after any bring-up,
  and again whenever a rate reading looks wrong: a world running seven times too fast inflates
  every per-simulated-minute series it produces. **The sweep is now one glance at the map** —
  six cells, each with both numbers on it — and a drifted world shows up as an applied scale
  that disagrees with its neighbours', which is the same signal it always was.

  **The governor is a servo, not a floor, and it still runs with nothing on screen. `0.6.2`
  disarms it headless.** `CheckMinFPS` is called from `TimeKeeper` on every update, and its gate
  is `Application.isFocused || !UserSettings.ignoreMinOnUnfocus.val`. `ignoreMinOnUnfocus` ships
  **`false`**, so the second term is always `true` and a windowless process — which is never
  focused — is governed exactly like a visible one. `minimumFPS` ships **15**, and the servo
  drives `engineTimeScale` toward whatever value holds the frame rate there, which is why five
  headless worlds asked for ×100 and reported 11–81. **It was costing real simulation, not just
  a tidy number.** Measured on the living deployment 2026-08-10 against the two hours
  immediately before the change, at comparable populations:

  | Slot | Servo armed: applied / achieved | Disarmed: applied / achieved |
  |---|---|---|
  | 1 | 79 / **25.7** | 100 / **45.5** |
  | 2 | 14 / **4.9** | 100 / **7.0** |
  | 3 | 13 / **4.6** | 100 / **7.3** |
  | 4 | 16 / **6.0** | 100 / **6.6** |
  | 5 | 19 / **6.4** | 100 / **7.6** |

  Local aggregate went **48 → 74** simulated seconds per wall second. Populations drifted lower
  across the two windows, so some of that is the worlds and not the patch; slot 4 at an
  identical population still gained ~10%, and the direction is the same on every slot. The
  mechanism is not simply "the clamp was the ceiling" — achieved was *below* the clamped
  `engineTimeScale` on slots 2–5 both before and after, so the frame rate was already binding.
  The leading explanation is the servo's own cost: it called `SetValue` on nearly every update,
  and each call runs `UpdateTimeScale` and its subscribers. `MinFpsGovernor` writes only when
  the value actually changes.

  **`MULTIVERSE_MIN_FPS=off` puts the servo back**, and `0` disarms it in a drawn instance too.
  The patch never writes `UserSettings.minimumFPS`, on purpose: `IntUserSetting.val` is backed by
  `PlayerPrefs`, which is **one store shared by every instance on the machine and persisted
  across runs**, so setting it would silently change the owner's own setting for their next drawn
  session. (`MinimumFPS` has never been written on this machine — the key is absent from
  `HKCU\Software\The Bibites\The Bibites`, so the live value is the built-in 15.)

  **`Time.maximumDeltaTime` is the ceiling underneath the servo, and nothing here lifts it.**
  `UpdateTimeScale` pins it to `Time.fixedDeltaTime`, which is `1 / simTPS` — `simTPS` defaults
  to **30** (`ScenarioIndependentSettings`, range 5–100, "not recommended below 20"). One frame
  therefore advances at most one simulation tick, so **the achieved rate cannot exceed
  `fps / simTPS`** however high the target goes. That is why slot 1 reported a clean ×100 and
  advanced 45×, and why asking for more than the host can render buys nothing. The honest levers
  on throughput are the per-tick cost (population, brain TPS) and `simTPS` itself — not the
  time-scale target.
- **A WSL environment variable does not reach the game on its own.** `MULTIVERSE_AUTOTEST=1
  game.sh start` is silently ignored: WSL only forwards variables that `WSLENV` names.
  Use `WSLENV=MULTIVERSE_AUTOTEST MULTIVERSE_AUTOTEST=1 game.sh start`. The second hop is
  free — PowerShell `Start-Process` passes its own environment to the game — so `game.sh`
  itself needs no change.
- **Unity's own errors never reach `LogOutput.log`.** BepInEx ships
  `[Logging.Disk] WriteUnityLog = false`, and on this install the chainloader also reports
  `Unable to start Unity log writer` at startup. An in-game `NullReferenceException`
  therefore leaves no trace in the BepInEx log. The mod does subscribe to
  `Application.logMessageReceived` and forward errors itself — **but only under the
  auto-test**: the handler is `AutoTest.Awake` (`AutoTest.cs:71`), and that component is
  added only when `[M1] AutoTest` or `MULTIVERSE_AUTOTEST` armed it
  (`MultiversePlugin.cs:174`). **On the living deployment nothing forwards them, so in-game
  Unity failures are silent** — confirmed 2026-08-10, when five headless worlds wrote a flat
  thumbnail into every save, which is the `RenderTexture.Create failed` signature, and not one
  Unity error line reached any of the five logs. A rig that needs to see Unity's own errors
  has to subscribe outside `AutoTest`.
- **A headless world's save thumbnail is blank, and the error saying so cannot reach the
  BepInEx log.** Under `-batchmode -nographics` `SaveSystem.CreateSave` still calls
  `SceneScreenShotHandler.SaveScreenshotToZip`, and the `RenderTexture.GetTemporary(400, 400,
  24, ARGB32)` at `ScreenShotHandler.cs:117` fails with Unity's own `RenderTexture.Create
  failed` — which is invisible on disk for the reason above, and reaches a log **only under
  the auto-test**, whose forwarder is the only subscriber (see the gotcha above). On the
  deployment there is no line at all, so **the tell is the file, not the log**: `img.png` is
  **3,666 bytes** in every headless save and ~**79 KB** in the drawn one it replaced — measured
  across `M4-Slot3`'s own rotation set on either side of the 2026-08-10 switch. That also makes
  a headless save ~75 KB smaller than the drawn save of the same world, which is worth knowing
  before comparing save sizes across the switch. **The save itself is whole.**
  `ReadPixels` against the failed target does not throw, so the 400×400 `img.png` is written
  flat rather than not at all, and every other entry — `scene.bb8scene` and `data.bin` ahead
  of it, the bibites, the eggs and the templates behind it — is written normally; confirmed
  2026-08-09 by extracting three headless saves. `WorldSaver`'s verify would not have caught
  a bad thumbnail in any case, because it asks only whether `scene.bb8scene` exists; what
  actually guards the file is its try/catch and the partial-then-rotate write. Read the line
  as cosmetic. The only casualty is the picture the game draws beside the save's name.
  **Headless does not make the thumbnail free, and it does bend every per-kilobyte
  measurement** (added 2026-08-10). `GetTemporary` returns a non-null `RenderTexture` whose GPU
  allocation failed, so `cam.Render()` is a no-op — but the `new Texture2D(400, 400, ARGB32)`,
  the `Apply()`, the `EncodeToPNG` and the deflate of the result all still run on the CPU
  (`ScreenShotHandler.cs:120-174`). **Measured with `shotMs` on 2026-08-10, that waste is ~10 ms
  a save** — real, and 1% of the stall, so the blank thumbnail is a cosmetic defect and not a
  performance one. (On the **first** save of a process it reads 483–585 ms; that is JIT of the
  encode path, not the encode.) What headless removes is not work but **75 KB of *incompressible* bytes**: an already-deflated
  PNG passes through the zip at ~1:1, so dropping it takes ~75 KB off the compressed file while
  taking only ~75 KB off a ~1.6 MB uncompressed payload. Any statistic normalised by the **zip**
  size — "ms per 100 KB" — therefore jumps about **22%** across the switch for no change in work
  at all. Normalise by the uncompressed total instead; see *Watch items*, first item.
- **`LogOutput.log` is overwritten on every launch** (`[Logging.Disk] AppendLog = false`).
  Copy it before restarting the game if you still need the previous run.
- **Two game instances run fine side by side, and get separate logs — but which log is a
  race.** Verified 2026-08-02: both processes persist, the first to start holds a lock on
  `BepInEx/LogOutput.log`, and BepInEx falls back to `LogOutput.log.1` for the second, so
  nothing is truncated. **Never map an instance to a log file by start order.** `game.sh`
  greps both candidates for that instance's own startup marker instead. Both instances also
  load the same `plugins/` DLL and the same `config/`, so per-instance settings can only
  arrive through the environment.
- **A Go program that runs on Windows must not fsync a directory.** POSIX needs the
  containing directory flushed after a rename-into-place, and the journal and the relay's
  `ring.json` both did it unconditionally. On Windows `os.Open` of a directory followed by
  `Sync` fails with `Access is denied`, so `multiverse-sidecar.exe` exited at startup with
  `open journal: sync …\journal: Access is denied` — before it ever claimed a slot, which
  reads exactly like a dead peer. The primitive now lives in `go/internal/fsutil` with a
  build-tagged Windows no-op (NTFS journals that metadata itself). Found by running the
  cross-compiled sidecar on this machine, not on the second computer.
- **From a Windows process, WSL services answer on `localhost`, not on `127.0.0.1`.** WSL2's
  localhost forwarding resolved `ws://localhost:<relayPort>` into the VM, while `ws://127.0.0.1:<relayPort>`
  got `connectex: No connection could be made`. It matters only for a cross-boundary test on
  this machine; the second computer dials a LAN address either way.
- **`Start-Process` from a WSL `powershell.exe` call keeps the interop pipe open.** The
  launched child inherits it, so `ps_run … | tail` never returns even though PowerShell
  itself exited. Redirect the PowerShell output to a file and read the file, with a
  `timeout` around the call. `game.sh` is unaffected because the game detaches with its own
  window.
- **`~/.bashrc` sets `GOROOT=$HOME/go` before any Go exists.** Installing the tarball
  anywhere else leaves every `go` command failing with `cannot find GOROOT directory`. The
  install therefore keeps the real toolchain at `~/go-dist/go` and symlinks `~/go` to it,
  which satisfies the stale variable and the stale `PATH` entry at the same time. Do not
  delete the symlink, and do not use `~/go` as a `GOPATH` — that is `~/gopath`.
- **Never load a world before `ProceduralSpriteManager.Instance.done`.** `MenuInitializer`
  starts the sprite atlas build asynchronously, and until it reports `done` every bibite
  spawn throws `IndexOutOfRangeException` out of `BibiteBody.FinalizeBirth` →
  `RequestBodySprite`. `LoadBibiteOrEggFromData` swallows that exception and returns
  `null`, so the symptom is **a world that loads empty**, or a migration answered
  `DESERIALIZE_FAILED` — a transient condition reported as a permanent fault, with no
  stack trace anywhere. The mod already gates on it, in two places: `WorldLoader.MenuReady`
  before it calls `GameManager.StartGame`, and `GameBridge.SimulationReady` before any
  spawn. **Any E2E rig, script or auto-test that loads a world must wait on the same
  flag** — sleeping for a fixed number of seconds is not a substitute, because the build
  time scales with the sprite set.
- **Only one rig can run at a time, and one is running now.** `run-m4-lan.sh` holds the
  living deployment since 2026-08-06 (*The M4 rigs*). Every rig in this checkout uses the same
  fixed defaults, so a second rig — or a second agent working in this repo — silently attaches
  to or fights over the first one's processes. Check the whole plan with
  `ss -ltn | grep -E ':(878[7-9]|879[0-6]|18789) '` before starting, and give a throwaway
  smoke test high ports (`--listen 127.0.0.1:0` writes the resolved address to
  `<data-dir>/listen.addr`). The historical rigs collide head-on with the current one: M2
  used `8787`/`8788`/`8790`, and M3 used `8787`/`8788`/`18789` plus the relay on `8790`.
- **The five-instance ceiling, and the log-file starvation trap. Measured 2026-08-06, and
  it is BepInEx, not the hardware.**
  Six game instances run together at about 550 MB each — 3.3 GB of 62 GB — at a load
  average near 2.4 on 16 cores. But BepInEx's `DiskLogListener` walks `LogOutput.log`,
  then `LogOutput.log.1` .. `.4`, and then gives up with *"Skipping log file creation"*.
  **An instance that gets no log file does not merely lose its log: the mod never runs in
  it.** Measured in both directions — the sixth instance sat at the main menu at 355 MB
  and 208 s of CPU while the other five were at ~590 MB and ~1200 s and had seeded their
  worlds; freeing one log file made that same instance seed in forty seconds and left the
  instance that gave up the file idle in its place. So five real games is the ceiling on
  this install, and `e2e/run-m4.sh` drives its sixth slot with `bin/fakemod` instead. **The
  LAN rig does not hit this at all** as a *ceiling*: the second computer has its own BepInEx,
  so `e2e/run-m4-lan.sh` runs five real games here and one real game there — six real worlds,
  no synthetic peer. That is the reason slot 6 is the slot that moves.

  **But the same root cause bites a five-game bring-up as a starvation trap, and that is the
  failure to expect on every reboot.** Five instances need exactly the five files BepInEx
  hands out, and the allocation is a lock race (see *Two game instances run fine side by
  side* above). One of them can lose it and get **no log file at all**, so the mod never
  loads in that instance: it comes up with `modConnected=false` and `exportEdges=[]`, and its
  neighbours' lanes bypass it with `peer_mod_absent`. **It is not slot-specific** — it
  starved slot 1 on 2026-08-08 and slot 2 on 2026-08-09, on the two reboots this deployment
  has had.

  **Distinguishing it from the config race above is the whole diagnosis, and the tell is an
  absence.** The config race *writes* `configuration failed` into a log; starvation writes
  **nothing**, because there is no log to write into. On 2026-08-09 `configuration failed`
  appeared in none of the five logs and the base `LogOutput.log` held a single 55-byte
  `Unable to start Unity log writer` line and nothing else. The second tell is memory: the
  starved instance sits far below its siblings — 321 MB against ~590 MB on one run, 348 MB
  against ~700 MB on the other — because it never seeded a world.

  **The remedy, proven on both reboots: restart that one instance, and only that one.**
  A still-waiting `up` picks it up and completes normally, so nothing else has to be
  disturbed.

  ```sh
  bibites-mod/game.sh stop slot<n>              # ONLY the starved instance
  source e2e/run-m4-lan.sh status               # its default dispatch is read-only
  start_game <n>                                # the rig's own launcher, so the env matches exactly
  ```

  Re-driving the rig's own `start_game` rather than `game.sh start` is the load-bearing part:
  it is what sets `MULTIVERSE_*` and `WSLENV` to exactly what the other four got. The
  instance takes the log name its own stop just freed and connects in under a minute.
- **Stop the old far-end sidecar before you unpack a new bundle.** Windows locks a running
  `.exe`, so the copy step of `setup-farend.ps1` fails with **`Permission denied`** on
  `multiverse-sidecar.exe` while the previous rig's sidecar is still up. It cost the M4 far-end
  setup a confusing failure: the M3 sidecar had been running there since 2026-08-03, and
  nothing in the error names it. Run `Stop-Process -Name multiverse-sidecar -Force` (or the old
  rig's `.\stop-slot2.ps1`) on that machine first, then unpack. This is the far-end twin of
  *Stop the game before you deploy* above.
- **A PowerShell command pasted into WSL bash fails with `command not found`.** Every far-end
  instruction is PowerShell — `.\setup-farend.ps1`, `.\start-slot6.ps1`, `Stop-Process` — and
  so is every elevated owner step. **Type them in a PowerShell window on the machine they
  belong to.** The far-end ones belong to the second computer, which this rig never drives
  (D9). For a command that targets *this* machine's Windows side, the interop binary works
  from WSL: `powershell.exe -File <script>`, with a Windows-side cwd (see the `cd /mnt/c` note
  above). The failure is loud and harmless, and it wastes a minute every time.
- **A stale `netsh portproxy` steals a Contract A port from Windows processes only.** The
  M3 LAN rig left `0.0.0.0:8790 -> <a WSL address>:8790` behind, and under the M4 port plan
  8790 is **slot 4's** Contract A port. It listens on `0.0.0.0`, so it shadows
  `127.0.0.1:8790` for **Windows** processes while WSL keeps its own. When the recorded
  address is stale the sidecar binds 8790 inside WSL and reports success, the game dials it
  from Windows and gets *"An existing connection was forcibly closed by the remote host"* six
  times, and the map forms one slot short with the reason `peer_mod_absent` — which reads
  like a mod bug and is a network artifact. **When it happens to be current it works, which
  is worse**: measured 2026-08-06 the proxy pointed at `172.24.110.174`, the live WSL address,
  so slot 4 worked that day and would have broken at the next WSL restart. `e2e/run-m4.sh up`
  and `e2e/run-m4-lan.sh up` both check the Windows listener table and refuse to start; the
  workaround while it existed was `SLOT_PORT_BASE=18787 e2e/run-m4.sh up`.
  **The owner deleted it, and it is confirmed absent as of 2026-08-09** —
  `e2e/run-m4-lan.sh lanhost` prints the live portproxy table and lists only the `8795`
  forward and an unrelated `8765` one. Keep it that way: check the table after a reboot
  rather than re-running the delete blind, and treat a returning `0.0.0.0:8790` row as a
  regression.
- **The M4 port plan — settled 2026-08-05, and these are now the compiled defaults.** The
  six-slot rig has to fit inside `contract-a.md` §10's Contract A range `8787`–`8792`
  without the relay or the archive standing on one of those six ports, which is exactly what
  M3's defaults did: relay `8790` is slot 4's port and archive `8791` is slot 5's, and
  `ringstat` inherited the second collision because it defaults to the archive's URL. **The
  decision: move the two components, keep the six slots.**

  | Component | Port | Bind |
  |---|---|---|
  | Contract A, slots 1–6 | `8787` `8788` `8789` `8790` `8791` `8792` | loopback only, by contract |
  | Relay, Contract B | **`8795`** | `127.0.0.1` local, `0.0.0.0` for the LAN |
  | Archive status page + `/api/status` | **`8796`** | `127.0.0.1` by default, `0.0.0.0` through `ARCHIVE_HTTP` for LAN viewing (*Owner steps*) |

  `go/internal/contractb.DefaultRelayPort`, the archive's `--http` default and `ringstat`'s
  `--url` default all carry these numbers, so a rig that passes no port flags gets this
  layout. `contract-b-m4.md` §3, *The M4 port plan*, is the normative statement.

  **Three things move with the relay port and are easy to forget:** the Windows Firewall
  rule and the WSL portproxy (both in *Owner steps* above), and the far end's
  `setup-farend.ps1 -RelayPort`. The bundle is now on `8795`; a far end still set up against
  the old default connects to nothing and looks like a peer that will not join, so a second
  computer prepared before 2026-08-06 has to re-run `setup-farend.ps1` from the current
  bundle.
- **`kill $!` does not kill a program launched in the background *inside* a compound
  command.** In `( … & )`, or in `cmd1 && cmd2 &`, `$!` is the pid of the **subshell**; the
  real program is its child and outlives the kill. A relay started that way survived its own
  cleanup and kept holding the relay port, so the next rig start failed on an address already in use
  while nothing in the script's process table looked wrong. Kill by port
  (`ss -ltnp | grep 8795`) or by pattern (`pkill -f 'bin/relay'`), and check the port is free
  before starting a rig — do not trust a recorded `$!`.
- **A Windows Firewall rule scoped to the Private profile does nothing on a network Windows
  has classified `Public`.** This is what blocked the first LAN attempt: the relay, the port
  proxy and the rule all looked correct, and the far end simply reported the relay
  unreachable. The Ethernet adapter here was on a `Public` profile, so the `-Profile Private`
  rule never matched. Check with `Get-NetConnectionProfile` in an elevated PowerShell, and fix
  with `Set-NetFirewallRule -DisplayName "Bibites Multiverse relay" -Profile Any`. Windows
  re-classifies a network on its own, so `-Profile Any` is the durable form — the command in
  *Owner steps* above now uses it.
- **Never test LAN reachability with `curl` from WSL to this host's own LAN address.** WSL2
  here is NAT'd and has no hairpin route back to the Windows host it runs on, so
  `curl http://<this machine's LAN IP>:8795` fails from WSL even when the firewall rule, the
  port proxy and the relay are all healthy. It is a false negative that reads exactly like a
  blocked port, and it costs an hour of chasing the firewall. Test from a Windows PowerShell
  on this machine, or from the second computer — never from WSL.
- **The archive ledger is cumulative, so an archive assertion needs a lower time bound.**
  `migrations.jsonl` keeps every hop of every earlier run, so "wait until a `slot 2 -> slot 3`
  record exists" succeeded instantly against an hour-old record and reported a crossing that
  had not happened yet. Every wait in `run-m3-lan.sh` and `run-m4-lan.sh` is filtered by an
  RFC 3339 mark taken when the wait starts; the header line begins with that timestamp, so a
  string compare is the whole filter. It matters more on the LAN rigs than anywhere else,
  because the archive is the *only* witness to a hop that lands on the other computer.
- **Reconnect bugs only appear in a real restart under a live game.** The M2 exit test
  found a leaked send loop in the mod's WebSocket transport that no per-side test suite
  could reach: `Task.WhenAny` left the losing loop alive, it parked on a semaphore shared
  across connections, and after *any* disconnect it stole the next session's first frame —
  the sidecar closed 4003 and the mod never re-handshook, silently voiding all crash
  recovery. Go contract tests pass (the fake mod is a fresh client each time) and the mod
  loads fine. **Always exercise a kill-and-restart against a running game** before trusting
  reconnection or recovery. Fixed in `9742cb7` with per-session cancellation plus a
  transport `Generation` counter stamped on every inbound event.
- **Fresh timestamps under `Scenarios/` and `Bibites/Templates/` are not your mod.**
  `AppInitializer.ReImportOfficialScenarios()` (`AppInitializer.cs:92, 123`) re-extracts
  the official scenarios and templates on **every** launch. Those archives also contain
  settings only — no bibites — so they cannot seed a populated test world.

## Useful decompiled entry points found so far

Full analysis in `m1_findings.md`; line numbers there track the current decompile.

- `ManagementScripts.SaveSystem` (singleton `SaveSystem.instance`) — the whole
  organism round trip: `SerializeBibite(GameObject)` (private, the payload builder),
  `SaveBibite(GameObject, path, desc)` → `.bb8` with full live state,
  `SaveBibiteAsTemplate` / `SaveTemplate` → `.bb8template`, and
  `LoadBibiteOrEggFromData(json, resume, problems, birthAtMaturity)` — public,
  uncalled in-assembly, restores full live state and preserves the entity ID.
  It swallows all exceptions and returns null; reflect on private `LoadBibite` to
  debug. World-level: `LoadGame(zip)`, the private `CreateSave` iterator — drive it
  yourself, because the public `SaveGame` wrapper hides its exceptions inside a coroutine —
  plus the parent/child re-link code at `SaveSystem.cs:748-782`.
- `ManagementScripts.GameManager` (all static) — `StartGame(string saveToLoadPath)` loads a
  named world with no UI interaction, the same call the Load Game panel makes. Proven by
  the auto-test. `Continue()` is **not** safe on a fresh profile: `SaveController.GetLastSave()`
  calls `.First()` on a possibly empty sequence.
- `ScriptHelpers.SerializationHelper` — the reflection engine behind every payload
  (`SerializeObject` / `DeserializeObject`, public static). It reads and writes
  `[SerializeField]` members including private and `internal` ones, so the mod needs
  no reflection of its own (e.g. for `InternalClock`).
- `ManagementScripts.BibiteTracker` (`BibiteTracker.instance.bibites` / `.eggs`) —
  the live-entity lists; the only way to enumerate or look up organisms (there is no
  ID→entity registry).
- `ManagementScripts.WorldObjectsSpawner.Instance` — `GenerateNewBibite`,
  `SpawnBibiteFromTemplate` / `SpawnEggFromTemplate` (template paths mutate; not for
  migration), `SpawnMeatPile`.
- `SimulationScripts.BibiteScripts.EggHatching.Hatch()` — the Constance-Mod hook
  point (postfix; `__instance.newBornBody`); public `onHatch` event as a no-patch
  alternative.
- `SimulationScripts.BibiteScripts.BibiteBody` — the live organism.
  `FixedUpdate()` (private) is the per-organism tick and **the M2 border-hook
  target**; `SaveState`/`LoadState` define the `body` payload; `OnDestroy` cleans the
  registries, `Die()` does not (it makes corpses/meat).
- `ScriptHelpers.VersionTracker`, `Utility.Version` — the game's own save-version
  compatibility machinery; relevant to `bb8-schema` versioning. Watch the constructor
  quirk: `new Version(0,6,3,3)` means `0.6.3a3`, not `0.6.3.3`.
