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
| Wire specifications | `contracts/` — `contract-a.md` (mod ↔ sidecar, **`contract-a/2.3`**, amended in place; §15 is the M4 set, §16 the species-identity set, §17 the species-census set, §18 the two-way-lane set (A38–A41) and **§19 the world-settings set, A42–A44**), `contract-b-m4.md` (sidecar ↔ relay ↔ sidecar ↔ archive, **`contract-b/3.5`**; §14 is its reconciliation set, §15 the species-identity amendment, §16 the census amendment, §17 the two-way-lane and hop-feed amendment (B13–B15), §18 the pacing and speed readout, §19 the world-settings readout and **§20 the disk budget, B20** — the only amendment that changes no wire field), `genome-hash.md` (the canonical genome projection, unchanged by M4 and by every amendment since — the block rides **beside** the blob, so no hash moves). `contract-b-m3.md` and `contract-b-m2.md` are the superseded M3 and M2 wires, kept as the record of what `contract-b/2` and `contract-b/1` said — **neither is current guidance** |
| Rigs and exit tests | `e2e/` — **`run-m4.sh` = the 3×2 six-slot grid on one machine** (the M4 local rehearsal; read its header before running it), **`run-m4-lan.sh` = the same map with slot 6 on the second computer** (the M4 exit-test rig; it sources `run-m4.sh` with `M4_LIB=1`), `run-m3.sh` = the three-slot ring rig on one machine, `run-m3-lan.sh` = the same ring with slot 2 on the second computer, `run-m2.sh` = the M2 two-sector rig (**historical**, speaks `contract-b/1`), `baseline.sh` = the T0/T1 capture, `journal.py` = journal reader. **The M3 scripts still speak the retired wire** — see *The M4 rigs* |
| Far-end bundle (the second computer) | `farend/` — `setup-farend.ps1`, `README.md`, `make-farend-bundle.sh`. The build scratch and the BepInEx download cache under `farend/dist/` are **gitignored**; `farend/dist/farend-bundle.zip` itself is **tracked**, because the second computer takes it out of a clone rather than off a USB stick |
| Rig runtime state — **gitignored** | `bin/` (built Go binaries), `e2e/data*/` (per-sidecar data dirs: journal, `peer-id`, remembered slot, genome cache — the D2 custody record of one machine's run), `e2e/relay-data*/` (the relay's `ring.json` slot reservations), `e2e/archive-data*/` (`migrations.jsonl` and the content-addressed genome store), `e2e/logs*/`, `e2e/run*/` (pid files). Each rig has its own suffixed set; the M4 LAN rig's are the `-m4-lan` ones and are the **living deployment's** |
| Retired rigs — **gitignored** | `e2e/<name>.tar.zst`. A retired runtime dir is still the record of a run that happened, so it is compressed in place rather than deleted: `e2e/data.tar.zst` (the M2/M3 journals, and **the only copy of the M4 resume test's input** — `data/slot-1/journal/journal.log`, the real in-flight hop of 2026-08-04; extract before re-running that test), `e2e/logs-m3-lan.tar.zst`, `e2e/archive-data.tar.zst`, `e2e/data-m4.tar.zst`. Restore one with `tar --zstd -xf e2e/<name>.tar.zst -C e2e/` |
| **The repo itself** | **Real location `/mnt/wsl/data/bibites-multiverse/`; `~/bibites-multiverse` is a symlink to it** (moved 2026-08-08 — see *The disk budget*). Every absolute path in every script, pid file and command line resolves through the symlink unchanged, so nothing had to be rewritten. `/mnt/wsl/data` is a separate 251 GB ext4 volume; the WSL root it left is 98 GB and shared with everything else on this machine |
| Shared LAN token — **never in the repo** | `~/.multiverse-token`, mode `600`. See *The LAN token* below |
| Decompiled game source | `decompiled/BibitesAssembly/` (654 files, grep this to find APIs) |
| Game user data (`Application.persistentDataPath`) | `/mnt/c/Users/<user>/AppData/LocalLow/The Bibites/The Bibites/` — holds `Savefiles/`, `Autosaves/`, `Scenarios/`, `Bibites/` |

## Versions

| Component | Version |
|---|---|
| The Bibites | Steam app 2736860, buildid 22383127; game version `0.6.3.1` — first read out of `The Bibites_Data/globalgamemanagers` (`bundleVersion`), **confirmed at runtime 2026-08-02**: the plugin logs `Application.version = 0.6.3.1` at startup |
| The plugin | `0.6.1` (`MultiversePlugin.Version`) — the world-settings build: it publishes what it was *told to do* on the handshake (§19 A42 — the exclusion list, the save interval, the keep count, save-on-quit and world wrapping), on top of the two-way-lane build's four-edge capture (§18 A38), migration exclusion list (A39) and two-lane portals. It speaks `contract-a/2.3`. The far-end bundle carries the same DLL; `farend/make-farend-bundle.sh` builds it fresh, so a bundle is only as current as its last rebuild |
| The Go side | `contract-b/3.5` — the world-settings readout (§19) on top of §18's pacing and speed readout, §17's two-way lane walks, `--inbound-rate` and the `/api/hops` feed, plus **§20's disk budget (B20)**: timer journal compaction, size-based log rotation and all-or-nothing appends in both append-only logs — the sidecar's journal since 2026-08-08, the archive's ledger since 2026-08-09. §20 changes no wire field, so the identifier does not move — see *The disk budget*. It is what fills the status page's **Species** and **Settings** tabs and `ringstat --species` / `--settings`. Built from `go/` into `bin/` by `e2e/run-m4-lan.sh build` |
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
1.50 to 1.10 cores, and a headless instance sitting at the menu is ~390 MB. The far end
takes the same two flags through `start-slot6.ps1 -Headless`, and slot 6 has run that way
since (*The living deployment*). Exactly one thing is lost, and it is not the simulation —
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
here and every 10 on the far end. It has since survived two host reboots, on 2026-08-08 and
2026-08-09, and was brought back by hand both times. Do not start another rig against it, and
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
`contract-a/2` and `contract-b/3` — and moved both paths. A relay and a sidecar still
*serve* the old paths and then close the connection with an explanation, so a stale script
does not fail with a socket error — it connects, gets closed, and looks like a peer that
will not join. That is the failure to expect, and it is why this list exists.

**The minors are now `contract-a/2.3` and `contract-b/3.5`** (the world-settings set, 2026-08-07;
`2.2`/`3.4` and `2.2`/`3.3` were the two-way lanes and the pacing readout, `2.2`/`3.2` the species
census, `2.1`/`3.1` the species identity, all of the same day), and **nothing in this table moves
with them**. Contract A did **not** move for D17 — §18's A41 says so explicitly: every field the
two-way set touches already existed and already accepted all four edge values, so there was
nothing additive to bump. It **did** move for §19, because A42 adds five genuinely new
`CONFIG_UPDATE` fields. Compatibility is on the major alone: a minor is never a rejection reason,
the URL paths did not move, and every field these sets added — the migration `species` block, the
`HEARTBEAT` census with its `truncated` sibling, `SECTOR_GRANT.neighbours`' `"W"` and `"S"` keys,
and §19's five settings — is OPTIONAL on both wires. **The minor is a capability statement, not a
negotiation:** detect a feature by the presence of its field, never by arithmetic on the minor. A
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
| `/contract-b/v2` | **`/contract-b/v3`** | `e2e/run-m3.sh` (`RELAY_URL`), and it is **sourced as a library** by `run-m3-lan.sh` and `baseline.sh`, so this one line reaches all three. Also `e2e/run-m3-lan.sh` (the operator note). `e2e/run-m2.sh` still says `/contract-b/v1`, two majors back. **`farend/setup-farend.ps1` is done** — it builds `/contract-b/v3`. |
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
the exit test** — see *Watch items*; the exit test's 241–538 ms is a 2026-08-06 reading, not
the current one.

### The LAN token

`contract-b-m4.md` §3.1 puts one shared bearer token on the Contract B upgrade, because M3's
wire leaves the loopback. **There is no flag that takes the token literally** — that is a
rule, not an omission, because a literal flag puts the secret in every process listing. Mint
it once, into a file only you can read:

```sh
umask 077 && head -c 32 /dev/urandom | base64 | tr -d '/+=' > ~/.multiverse-token
chmod 600 ~/.multiverse-token
```

Every binary then takes `--token-file ~/.multiverse-token`, or reads `MULTIVERSE_TOKEN`, or
reads `MULTIVERSE_TOKEN_FILE`. The token must be 16–256 bytes of printable ASCII with no
spaces. The relay compares it in constant time and answers `401` on a mismatch; a client that
collects `authFailuresBeforeCeiling` (5) consecutive 401s pins its backoff at the ceiling, so
a wrong token looks like a peer that never joins rather than a reconnect storm.

`bin/relay --insecure-no-token` accepts unauthenticated connections. It is for a
single-machine test rig only and **never** on the LAN.

### Driving one component by hand

Start the rig in dependency order — the relay first, then the archive, then the three
sidecars, then the three game instances:

```sh
bin/relay   --listen 127.0.0.1:8795 --data-dir ./e2e/relay-data \
            --token-file ~/.multiverse-token
bin/archive --relay ws://127.0.0.1:8795/contract-b/v3 --peer-id archive-main \
            --data-dir ./e2e/archive-data --token-file ~/.multiverse-token \
            --http 127.0.0.1:8796
bin/sidecar --listen 127.0.0.1:8787 --relay ws://127.0.0.1:8795/contract-b/v3 \
            --peer-id slot-1 --position 0,0 \
            --data-dir ./e2e/data/slot-1 --token-file ~/.multiverse-token
bin/sidecar --listen 127.0.0.1:8788 --relay ws://127.0.0.1:8795/contract-b/v3 \
            --peer-id slot-2 --position 1,0 \
            --data-dir ./e2e/data/slot-2 --token-file ~/.multiverse-token
bin/sidecar --listen 127.0.0.1:8789 --relay ws://127.0.0.1:8795/contract-b/v3 \
            --peer-id slot-3 --position 2,0 \
            --data-dir ./e2e/data/slot-3 --token-file ~/.multiverse-token
```

Both of those port numbers are now the compiled defaults, so the two flags above are
redundant and are written out only to make the layout explicit. See *The M4 port plan*.

Four things about that invocation are load-bearing:

- **The path is `/contract-b/v3`.** M3's `/contract-b/v2` and M2's `/contract-b/v1` are
  different wires. The relay still *answers* on `v2` and closes the connection with `4000`,
  so a stale client looks like a peer that will not join rather than a 404.
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
`MULTIVERSE_PEER_ID`, `MULTIVERSE_DATA_DIR`, `MULTIVERSE_SLOT`, `MULTIVERSE_TOKEN_FILE`,
`MULTIVERSE_LOG_LEVEL`, and `MULTIVERSE_RELAY_LISTEN` / `MULTIVERSE_RELAY_DATA_DIR` /
`MULTIVERSE_ARCHIVE_PEER_ID` / `MULTIVERSE_ARCHIVE_DATA_DIR` for the other two binaries.
Relay and sidecar answer `GET /healthz`, and each sidecar writes its resolved listen address
to `<data-dir>/listen.addr`, which is how `--listen 127.0.0.1:0` stays usable from a script.
The journal lives in `<data-dir>/journal/` and is the durable custody of decision D2 — keep
it across a restart or the sidecar loses every organism it was holding.

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
and all — to the new identity, and it is deliberately **not** a wire operation: on this
transport a `peerId` is a claim, not a credential, so only a command on the relay's own
machine can do it.

```sh
bin/relay --data-dir ./e2e/relay-data --handover-slot 5=peer-lan-slot5-new   # rebinds, logs, exits 0
```

**Reserving a slot before its peer connects.** The mirror image of the release flag, and
the reason a LAN map can form in any start order:

```sh
bin/relay --data-dir ./e2e/relay-data \
          --reserve-slot slot-1@0,0 --reserve-slot slot-2@1,0 --reserve-slot slot-3@2,0 \
          --reserve-slot slot-4@0,1 --reserve-slot slot-5@1,1 --reserve-slot slot-6@2,1
```

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
`farend/dist/cache/`, SHA-256 verified). The script refuses to build when
`setup-farend.ps1`'s pinned `$AssemblySha256` no longer equals
`bibites-mod/libs/BibitesAssembly.dll` — that pin **is** the version gate on the far end, and a
stale one would let two different game builds into one map. It also warns, without stopping,
when `libs/` no longer matches the game installed here — the check `deploy.sh` used to make on
its behalf.

**The repo distributes the bundle.** `farend/dist/farend-bundle.zip` is tracked as of
`8463b72`, so the second computer clones the private GitHub repo and takes the zip out of the
checkout instead of receiving a hand-copied file. Only two things still travel by hand:
`~/.multiverse-token` (as `token.txt`) and the relay's LAN address — **neither belongs in the
repo**. **Re-run `make-farend-bundle.sh` and commit the new zip** whenever the plugin, the
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
.\setup-farend.ps1 -RelayHost 192.168.1.227 -TokenFile .\token.txt
.\start-slot6.ps1
```

`setup-farend.ps1` finds Steam's copy of the game (registry, the usual folders, and every
extra library in `libraryfolders.vdf`), verifies `BibitesAssembly.dll` against the pin,
validates `-Position` and `-ExportEdges` before anything can silently disable the client,
installs BepInEx if it is absent, copies the plugin, writes the token to
`%LOCALAPPDATA%\BibitesMultiverse\token.txt` with a user-only ACL, and generates
`start-slot6.ps1` and `stop-slot6.ps1`. Its M4 defaults are `-RelayPort 8795`, `-Slot 6`,
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

**The tracked bundle is current, and it does not need re-taking.** It was last rebuilt
2026-08-09, to carry the `-Headless` start template; the rebuild before it, at `6899273`,
already postdated the torn-append fix `f72dd14`
(`git merge-base --is-ancestor f72dd14 6899273` passes), and the `multiverse-sidecar.exe` inside
the zip carries `journal-compact-minutes`, `journalCompactMinutes`, `discardedBytes` and
`JournalCompactInterval` — the whole §20 disk budget. **Nothing under `go/` or
`bibites-mod/src/` changed between those two rebuilds**: the plugin DLL is byte-identical
across them and to the one deployed here, and the two sidecar binaries differ only in the VCS
revision and timestamp the Go toolchain stamps into them. **Whether the second computer has APPLIED
it cannot be determined from here, and no observation will settle it**: §20 changed no wire
field, so a far end with the fix and one without it are indistinguishable on Contract B
(`contract-b-m4.md` §20 says so as a design property). Slot 6's §19 settings block proves its
*mod* is at least the settings build and says nothing about its sidecar. Treat the far sidecar's
§20 status as **unknown rather than stale**, and ask its operator if it matters — do not rebuild
the bundle to answer the question, because a fresh bundle answers a different one. The
archive-ledger fix `e68550b` of the same evening does not re-open the question either: its code
is `go/internal/archive/*` plus one sidecar *test* file, the archive runs on this machine only,
and the 2026-08-09 rebuild — cut before that commit merged — is not made stale by it.

`start-slot6.ps1` sets the mod's whole configuration natively — there is no `WSLENV` on that
machine — starts the sidecar with `--position 2,1`, **waits for `contract B: slot granted` in
its log**, and only then starts the game. A failure to join prints the four usual causes,
names TCP 8795, and does not start the game.

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
of a run and not of the installation. Slot 6 has run headless since 2026-08-09 (*The living
deployment*). `start-slot6.ps1` is **generated** by `setup-farend.ps1`, so a far end installed
before 2026-08-09 has a start script without the switch, and takes it only from a re-run of
the installer out of the current bundle — or from the same edit by hand.

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
curved over the world they skip), pulses animating each lane at its measured hop rate, and a
glossary that explains every term to a reader who did not build the system. `#species` is **who
is alive right now** — the union of every world's census, keyed on the §16 A34-normalized name
and printed in each world's own spelling, annotated with what the ledger knows about it
(crossings ever, first and last sighting, distinct genomes, recent lanes, parent species) and
badged `everywhere` / `endemic` / `never-exported`. It is **alive-only**: a species that crossed
a thousand lanes and is extinct everywhere is a ledger fact, not a resident. `#settings` is one
card per world of what that world was **told to do** — mod and protocol versions, save interval,
keep count, save-on-quit, wrapping and the exclusion list — and it is read-only, and says so.

| Endpoint | What it answers |
|---|---|
| `/api/status` | the live frame every tab is drawn from. It gained `recentHops` per lane and `flowWindowMs` with §17, the speed and pacing readout with §18, and §19's seven per-slot fields: `modVersion`, `contractAVersion`, `migrationExclude` (with `migrationExcludeKnown`), `saveMinutes`, `saveKeep`, `saveOnQuit` and `worldWrapping` |
| `/api/hops` | the last minute of crossings, which the map animates |
| **`/api/species`** | the alive union with its ledger annotations, plus `reportingSlots`, `censuslessSlots`, `truncatedSlots`, `ledgerSpecies` and `ledgerRecords`. **The join happens here, not in the browser**, so the page and `ringstat` can never disagree about which species is endemic |
| **`/api/species/history?key=&hours=&buckets=`** | one species' per-world population over time, downsampled from `metrics.jsonl`. `key` is required and bounded: missing, empty or over-long answers **`400`** |
| `/api/history?hours=&buckets=` | the same downsample for every world's total population — the map's sparklines |

**The species aggregate costs one pass, not one per poll.** It is built during the ledger replay
the archive already performs at startup, and maintained per new record after that, so a
half-million-line ledger is never re-read to answer a request. Measured on 2026-08-07 against
599,184 records (176 MB, warm page cache): the replay took **~6 s**, and the archive's settled
RSS went from 146 MB to **170 MB**, with a transient peak near 690 MB while the pass ran. The
same rollout widened each `metrics.jsonl` sample by **~640 bytes (+7.6 %)** — the settings block
for five publishing worlds — on a file that samples `/api/status` verbatim.

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
**874,162** and reported nothing.

### What still grows forever

| File | Growth | Measured 2026-08-08, six live slots |
|---|---|---|
| `e2e/archive-data-m4-lan/genomes/` | with **new genomes**. Content-addressed, so it tracks evolution rather than hops. The archive never sweeps it — the sidecars' own caches are capped by `genomeCacheMaxBytes`, the archive's is the record | **58 MB/h = 1.4 GB/day** |
| `e2e/archive-data-m4-lan/migrations.jsonl` | with **traffic**. The ledger is the record of what happened and nothing may evict from it (`contract-b-m4.md` §10) | **19 MB/h = 0.45 GB/day**, at ~540 crossings/min |
| `e2e/archive-data-m4-lan/metrics.jsonl` | with **time**, one sample per slot per `metricsInterval` | **0.7 MB/h = 0.02 GB/day** |
| `e2e/baselines/m4-collector/` | with **time**. Not part of the deployment — it is a read-only observation loop, copying five world saves and one `/api/status` sample every 5 minutes. Written and launched by the evolution-analysis session on 2026-08-07, not by hand. Stop it with `kill "$(cat e2e/baselines/m4-collector/collector.pid)"`; **every reboot kills it and nothing restarts it** — see *The living deployment* | **19 MB/h = 0.5 GB/day** |
| **total** | | **97 MB/h = 2.3 GB/day = ~70 GB/month** |

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
full-disk outage, two host reboots and one far-end restart. This section is what it last read,
how to bring it back after a reboot, and what is being watched.

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

### Slot 6 runs headless, since 2026-08-09

The second computer's game now runs under `-batchmode -nographics`. The switch was a save, a
quit that took 13 s and exited clean, and `.\start-slot6.ps1 -GameOnly -Headless` against the
sidecar that never went down. **It is the same world**: `simulatedTime` runs continuously
across the restart, and the sidecar held its slot, its relay session and its custody
throughout, so the map never saw a hole. Nothing on the status page distinguishes a headless
slot from a drawn one, which is the point.

Two things came out of the first hours. The periodic save is **faster** with nothing to
draw — one 2 485 ms warm-up sample, the only breach of Risk 3's 2 000 ms budget, and then
672 ms steady against 1 093–1 535 ms in visual mode. And the sidecar's custody replay burst
briefly outran its send buffer: one transient `1011: outbound queue full` disconnect,
reconnected clean, no organism lost — the backlog draining as designed rather than a fault.

**The restart reset the runtime time scale**: slot 6 came back at ×1 and was set to ×6.5 on
that machine. Headless has nothing to do with that — the switch is a restart, so it inherits
the gotcha whole and adds nothing to it (*A world can be at the wrong time scale*, in
Gotchas).
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
went silent past Contract A's 3.5 s heartbeat timeout **ten times between 23:43:04Z and
23:46:20Z** — ten `4004` closes with `silentFor` pinned at 3.50–4.23 s, each followed by a
reconnect 0.3 to 14 s later, settling on generation 11 at 23:46:36Z, and no local
mod has churned since. **Its game never restarted** — `[M4-SAVE] armed` appears once in that
instance's whole log and the periodic counter runs unbroken through the burst — and **no save
was long enough to be the cause**: the three saves spanning the window stalled 1.67–1.82 s, well
inside the 3.5 s the dose response in *Watch items* needs. The map absorbed it the same way it
absorbed the far end: 20 lanes and two `peer_mod_absent` closures at 23:45:29Z, back to 24/24 at
×5 by 23:51:03Z, `timeoutBounces` still 0 for the whole run. Nothing was lost, but the trigger is
**unexplained**, and it is the first burst of this shape with no save behind it.

### Bringing it back after a reboot

Proven end to end twice, on 2026-08-08 and 2026-08-09. The steps are ordered and step 1
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

5. **Expect one game to come up starved of a log file**, and fix that one instance rather
   than the rig — see *The five-instance ceiling, and the log-file starvation trap* in
   Gotchas. It happened on both reboots.
6. **Read the time scale of every instance, not only a restarted one** — see *A world can be
   at the wrong time scale* in Gotchas. It needed a `send 1 timescale 5` after the
   2026-08-08 reboot and nothing after the 2026-08-09 one, so the check is the deliverable,
   not the correction.
7. **Relaunch the baseline collector by hand.** Every reboot kills it and nothing restarts
   it. It is gitignored, read-only against the deployment — it only copies the five save zips
   and curls `/api/status` — and costs ~0.5 GB/day (*The disk budget*).

   ```sh
   cd ~/bibites-multiverse/e2e/baselines/m4-collector
   setsid nohup ./collector.sh >> collector.log 2>&1 &
   ```

   A reboot that lands mid-cycle leaves an empty snapshot directory behind; there is one,
   `20260808T143150Z/`, and it is harmless.

**The next bring-up will make the archive's counters JUMP, and that is a recovery rather than
a fault.** The running archive replayed the ledger with the pre-fix code at its own boot, so
everything it shows — `ledgerRecords`, the lane counters, the species aggregates — is built on
the 874,162 records that replay could reach plus what it has recorded since: **1,096,418
against 1,293,613 lines in the file** at 23:45Z on 2026-08-09. Its in-memory state is correct
and complete for its own run; it is the *replay* that was short. The first restart after the
fix reads past the spliced line and recovers the ~197,000 older records this process never
saw, so the totals rise. Before the fix that same restart would have **reverted** them to
01:21 on 2026-08-08. See *The disk budget*. One caveat on the arithmetic: the jump will read
**larger** than the recovery alone, because the live `ledgerRecords` counter only increments
on migration, ACK and NACK records — a `GENOME` append (`archive.go`, `RecordGenome`) is
counted at boot replay but never during the run, and `GENOME` is ~11% of ledger lines. The
gap between the counter and `wc -l` therefore widens ~53 lines a minute on its own, replay
after replay, and a shortfall computed from those two figures conflates the skipped-line
loss with this counting artifact. Sizing from the file, not the counter, is the safe habit.

### Watch items

Three things are being watched on the running deployment. The first is a **measured breach of
a bar the owner set**, and it is waiting on a decision rather than on more data. The second is
a trend with no verdict yet. The third **has its verdict** — the reading is understood, it is
not a defect, and it stays on this list because M5 changes what it will mean.

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
  times the exit test. **It is not the worlds growing**, which was the first guess and is
  wrong: the collector's 243 five-slot snapshots put the save zips at 275–330 KB on
  2026-08-07, near 570 KB overnight on 08-08 at peak population, and 210–320 KB now. What
  moved is the **cost of a saved kilobyte** — 120–172 ms per 100 KB in the exit test,
  500–620 ms on 2026-08-08, 290–370 ms now. Slot 1 settles it: its population fell from 21
  to 7–11 and its file stayed near 220 KB while its median stall went 214 ms → 1 164 ms →
  704 ms. Nor is the payload mostly organisms — unzip
  any slot save and the bibites are about a fifth of it, behind `speciesData.json`
  (~900 KB raw), `data.bin` (~150 KB) and the 400×400 `img.png` the game renders inside
  `SaveSystem.CreateSave`. **A save costs roughly what it costs whether the world holds 7
  organisms or 40**, so the variable is the host — five Unity instances at ×5 with 24 lanes
  and ~500 crossings a minute — and a host reboot gives back about 40% of the cost.

  **A breach under about 3 s is a logged breach and nothing more; past 3.5 s it disconnects
  the mod.** Contract A's heartbeat timeout is 3.5 s and the stall blocks the thread that
  sends the heartbeat, so matching every save against the sidecars' 4004 closes gives a clean
  dose response: **0.2%** of saves under 2 s are followed by a `contract A: heartbeat timeout,
  closing with 4004` within twelve seconds, **1.2%** at 2.5–3 s, **7.0%** at 3–3.5 s and
  **50.8%** of the 63 saves over 3.5 s. The mod reconnects in about a second and the sidecar's
  quiet-mod gate holds `MIGRATE_IN` until heartbeats resume, so each one costs a session churn
  and a short delivery pause rather than an organism. **The dose response does not run the
  other way**: slot 4 took ten `4004` closes in three minutes at 23:43Z on 2026-08-09 with no
  save over 1.82 s behind it (*The reading of 2026-08-09, after the second reboot*), so a stall
  explains some churn and
  is not the only thing that causes it. Nothing else has been observed on the save path: no save
  has logged `event=FAILED` since the species-history guard shipped, and the deferral ladder
  has never run past 5 of 6. **The far end is the control and it is not in this discussion at
  all**: one game on its own host, saving a comparable 354 KB world in 924 ms — and 380 KB in
  **567 ms** at 00:47Z on 2026-08-10, after its own restart — on the mod's own shipped cadence
  of `saveMinutes=10 saveKeep=6`. The five local worlds run `2` and `4` because
  `run-m4.sh:270-271` overrides those defaults — an exit-test setting, chosen so a
  test window would contain several saves, that nobody revisited when the rig became a
  deployment. Risk 3 already names the escalation — a save that breaks the budget "needs a
  different cadence, or a save path that does not block the tick" — and D14 accepts a slower
  rig over a lost night. **Which of those the owner wants is the open question, and nothing
  should be retuned before that answer**, because the mod says so on every breach line: *do
  not lower the bar*.

- **Slot 1 is depressed, and it is the world to keep an eye on.** It fell to a population of
  **2** in the 2026-08-08 ENOSPC incident and did not recover with its neighbours: readings
  of 3, 10, 8 and 7 through 2026-08-09 put it inside its 6–15 recovery range but at the
  bottom of it, while the other four local worlds held 22–25. It then read **14** at 20:21Z
  the same day, so it *is* climbing — and it is still the smallest of the five by a factor of
  two and a half. A small world produces few young of its own and depends on arrivals; the
  open question is whether the lanes alone carry it back to its neighbours' range or whether
  it settles low.

- **`genomeGaps` is a fetch queue, its healthy value is 0, and what it counts is
  throughput.** The field is `len(a.pending)` (`go/internal/archive/status.go:247`, over the
  map declared at `go/internal/archive/archive.go:170`): the set of genome hashes the archive
  has asked a sidecar for and not yet received. It is a **backlog count, not an error count**,
  and it is not the same thing as `bin/archive list --gaps`, which reports what the *ledger*
  shows missing. An entry is created only for a hash the store does not already hold
  (`archive.go:766`) and leaves only when the genome arrives.

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

  **The mechanism is a rate cap, and it is deliberate.** The archive may send at most
  `GenomeRequestsPerMinute` = **30** genome requests a minute **per peer**
  (`go/internal/contractb/contractb.go:230`, enforced by `allowSendLocked` at
  `go/internal/archive/archive.go:899`), so the ladder is bounded near **180 fetches a
  minute** across six peers. Only a crossing carrying a hash the store does not already hold
  costs a fetch, which is why the *crossing* rate that meets that ceiling is about twice it:
  on this rig the backlog grows above roughly **400** migrations a minute and drains below
  roughly **340**. Under the ceiling the field reads 0; over it, `pending` **is** the queue,
  and it is worked off at the same cap when the current slows. The number to read is
  therefore the *direction* against the current rate, not the magnitude.

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
  below ~340/min is the reading that would mean something new.

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
  code against the changed APIs.
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
  it and confirm on `/api/status`; the command answers
  `targetTimeScale=5.00 Time.timeScale=5.00`. **`timeScale` on the page is the speed the game
  is *applying*, not one this rig measured** — the mod copies `Time.timeScale` into the
  heartbeat (`MultiverseClient.cs:1040`) and `contract-a.md` calls it *the effective time
  scale*. The game's own governor is what makes it move: `TimeController.CheckMinFPS` clamps
  the engine scale into `[min(0.1, target), target]` whenever the frame rate misses
  `minimumFPS`, so a world that keeps up reports its target exactly and a host that cannot
  reports the fluctuating non-integer it was throttled to. Neither figure is the achieved rate,
  because `UpdateTimeScale` also pins `Time.maximumDeltaTime` to one fixed step: at 00:47Z on
  2026-08-10 the five local worlds reported `5` and advanced 4.94–5.00× per wall second, while
  slot 6 reported `6.5` and advanced 5.84×. Three measurements say the scope is wider than
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

  So it is not a property of a restart. Sweep all five on `/api/status` after any bring-up,
  and again whenever a rate reading looks wrong: a world running seven times too fast inflates
  every per-simulated-minute series it produces.
- **A WSL environment variable does not reach the game on its own.** `MULTIVERSE_AUTOTEST=1
  game.sh start` is silently ignored: WSL only forwards variables that `WSLENV` names.
  Use `WSLENV=MULTIVERSE_AUTOTEST MULTIVERSE_AUTOTEST=1 game.sh start`. The second hop is
  free — PowerShell `Start-Process` passes its own environment to the game — so `game.sh`
  itself needs no change.
- **Unity's own errors never reach `LogOutput.log`.** BepInEx ships
  `[Logging.Disk] WriteUnityLog = false`, and on this install the chainloader also reports
  `Unable to start Unity log writer` at startup. An in-game `NullReferenceException`
  therefore leaves no trace in the BepInEx log. The mod subscribes to
  `Application.logMessageReceived` and forwards errors itself — keep that, or in-game
  failures go silent.
- **A headless world's save thumbnail is blank, and the error saying so cannot reach the
  BepInEx log.** Under `-batchmode -nographics` `SaveSystem.CreateSave` still calls
  `SceneScreenShotHandler.SaveScreenshotToZip`, and the `RenderTexture.GetTemporary(400, 400,
  24, ARGB32)` at `ScreenShotHandler.cs:117` fails with Unity's own `RenderTexture.Create
  failed` — which is invisible on disk for the reason above, and surfaces only through the
  mod's `Application.logMessageReceived` forwarder. **The save itself is whole.**
  `ReadPixels` against the failed target does not throw, so the 400×400 `img.png` is written
  flat rather than not at all, and every other entry — `scene.bb8scene` and `data.bin` ahead
  of it, the bibites, the eggs and the templates behind it — is written normally; confirmed
  2026-08-09 by extracting three headless saves. `WorldSaver`'s verify would not have caught
  a bad thumbnail in any case, because it asks only whether `scene.bb8scene` exists; what
  actually guards the file is its try/catch and the partial-then-rotate write. Read the line
  as cosmetic. The only casualty is the picture the game draws beside the save's name.
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
