# Development Environment

The full loop — edit, build, deploy, run, read logs — runs from WSL with no manual steps.

## Layout

| What | Where |
|---|---|
| Game install (Windows) | `/mnt/c/Program Files (x86)/Steam/steamapps/common/The Bibites` |
| Game assembly | `…/The Bibites_Data/Managed/BibitesAssembly.dll` |
| BepInEx log | `…/The Bibites/BepInEx/LogOutput.log` |
| Plugin project | `bibites-mod/` (source in `src/`, reference DLLs in `libs/` — see *The reference DLL set*) |
| Go module (`multiverse-relay`, `multiverse-sidecar`, `multiverse-archive`) | `go/` (module `multiverse`; binaries in `cmd/`, libraries in `internal/`) |
| Wire specifications | `contracts/` — `contract-a.md` (mod ↔ sidecar, **`contract-a/2.0`**, amended in place; §15 is the M4 set), `contract-b-m4.md` (sidecar ↔ relay ↔ sidecar ↔ archive, **`contract-b/3.0`**; §14 is its reconciliation set), `genome-hash.md` (the canonical genome projection, unchanged by M4). `contract-b-m3.md` and `contract-b-m2.md` are the superseded M3 and M2 wires, kept as the record of what `contract-b/2` and `contract-b/1` said — **neither is current guidance** |
| Rigs and exit tests | `e2e/` — `run-m3.sh` = the three-slot ring rig on one machine, `run-m3-lan.sh` = the same ring with slot 2 on the second computer, `run-m2.sh` = the M2 two-sector rig (**historical**, speaks `contract-b/1`), `baseline.sh` = the T0/T1 capture, `journal.py` = journal reader. **There is no `run-m4.sh` yet, and the M3 scripts still speak the retired wire** — see *The M4 rig modernization* |
| Far-end bundle (the second computer) | `farend/` — `setup-farend.ps1`, `README.md`, `make-farend-bundle.sh`. The bundle itself lands in `farend/dist/`, which is **gitignored**: it holds binaries and a downloaded BepInEx release |
| Rig runtime state — **gitignored** | `bin/` (built Go binaries), `e2e/data/` (per-sidecar data dirs: journal, `peer-id`, remembered slot, genome cache — the D2 custody record of one machine's run), `e2e/relay-data/` (the relay's `ring.json` slot reservations), `e2e/archive-data/` (`migrations.jsonl` and the content-addressed genome store), `e2e/logs/`, `e2e/run/` (pid files) |
| Shared LAN token — **never in the repo** | `~/.multiverse-token`, mode `600`. See *The LAN token* below |
| Decompiled game source | `decompiled/BibitesAssembly/` (654 files, grep this to find APIs) |
| Game user data (`Application.persistentDataPath`) | `/mnt/c/Users/<user>/AppData/LocalLow/The Bibites/The Bibites/` — holds `Savefiles/`, `Autosaves/`, `Scenarios/`, `Bibites/` |

## Versions

| Component | Version |
|---|---|
| The Bibites | Steam app 2736860, buildid 22383127; game version `0.6.3.1` — first read out of `The Bibites_Data/globalgamemanagers` (`bundleVersion`), **confirmed at runtime 2026-08-02**: the plugin logs `Application.version = 0.6.3.1` at startup |
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

### The M4 rig modernization

**There is no `e2e/run-m4.sh`, and the M3 scripts speak a wire the binaries retired.** M4
bumped both majors (`contract-a/2.0`, `contract-b/3.0`) and moved both paths. A relay and a
sidecar still *serve* the old paths and then close the connection with an explanation, so a
stale script does not fail with a socket error — it connects, gets closed, and looks like a
peer that will not join. That is the failure to expect, and it is why this list exists.

| What the scripts speak | What M4 speaks | Where |
|---|---|---|
| `/contract-b/v2` | **`/contract-b/v3`** | `e2e/run-m3.sh` (`RELAY_URL`), and it is **sourced as a library** by `run-m3-lan.sh` and `baseline.sh`, so this one line reaches all three. Also `e2e/run-m3-lan.sh` (the operator note) and `farend/setup-farend.ps1`. `e2e/run-m2.sh` still says `/contract-b/v1`, two majors back. |
| `/contract-a/v1` | **`/contract-a/v2`** | **Nothing to change.** No script hardcodes a Contract A path — the sidecar's `--listen` and the mod's `MULTIVERSE_SIDECAR_PORT` carry it, and both sides already agree. |
| `MULTIVERSE_EXPORT_EDGE=E` | **`MULTIVERSE_EXPORT_EDGES=E,N`** | `e2e/run-m3.sh` and `farend/setup-farend.ps1`. The old name still parses, so this is not a break — it just cannot produce anything but a line topology. |
| the relay's `ring.json` key `"ring"` | **`"slots"`**, with `width`, `height`, `col`, `row` | `e2e/run-m3-lan.sh` (`ring_order()`) and `e2e/baseline.sh`. **This is the silent one.** `"ring"` is a read-only migration path: an M4 relay reads an M3 file once and never writes that key again. Both readers work today against the committed M3 `ring.json` and start returning empty the first time an M4 relay saves. |
| nothing | the archive's **`--http`** flag, and **`ringstat`** | No script mentions either. The archive is started without `--http`, so it binds the default `127.0.0.1:8791` — which is slot 5's Contract A port on a six-slot rig. `baseline.sh` reads the archive only through the filesystem, so the whole M4 observability surface — map shape, effective lanes, bypasses, custody/paced/held depths, hold-timeout bounces — is absent from the baseline capture. |
| `grep '[M2-CROSSING]' \| tail -n 1` | a filter on **`edge=`** | `e2e/baseline.sh`, in both `last_crossing()` and the population trend. See *Reading the logs* below: one window now emits one line **per export edge**. |

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
| `MULTIVERSE_EXPORT_EDGES` | **M4.** The edges this sim exports through, as a list — `E,N` under the grid (`contract-a.md` §15 A18). Separators are comma, semicolon, space or tab; each token is trimmed and upper-cased. The **opposite** of each becomes a passive entry edge, and the union of both sets becomes `borderEdges`. Unset **disables the multiverse client entirely**. |
| `MULTIVERSE_EXPORT_EDGE` | **The M3 singular name, still read, and it takes a list too** — it is the same parser behind a different name. Kept so an M3 script keeps working. |
| `MULTIVERSE_OPEN_EDGE` | M2's name for the same thing. Also still read. |
| `MULTIVERSE_RING_SLOT` | The advisory ring slot, an integer `≥ 1`. It is a **label**, not a routing input: the sidecar closes `4001` when it disagrees with the slot the relay granted, which turns a mis-wired rig into one second of diagnosis. **Replaces `MULTIVERSE_SECTOR`, which no longer reaches the mod at all** — D8 retired the `{x, y}` grid (`contract-a.md` §14 A14). |
| `MULTIVERSE_SIDECAR_PORT` | The loopback Contract A port this instance dials. Default `8787`. One per instance; this is what separates six games on one machine. |
| `MULTIVERSE_WORLD` | See below. Under M4 it also names the **save file**, and therefore the whole rotation set. |
| `MULTIVERSE_CMD_FILE` | See below. |
| `MULTIVERSE_BORDER_WIDTH` | `W` in world units, which is also the inner boundary of the capture band. `0`/unset derives `0.02·S`, floor 20 u. |
| `MULTIVERSE_SAVE_MINUTES` | **M4.** Periodic-save interval in simulated minutes. Default `10`. `0` turns the timer off; save-on-quit is unaffected. |
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
unparseable token, a duplicate edge, or an edge declared together with its opposite
(`E,W`) all clear the set and leave the client off. Each of those three logs an error. A
value that is only separators — `MULTIVERSE_EXPORT_EDGES=","` — clears it with **no** error,
and the only symptom is the generic "no export edge is configured" line. Check that line
before chasing a sidecar that never sees a mod.

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
  | `save [name]`, `timescale <x>`, `autosave <on\|off>`, `quit` | World control. `save` runs the same write-verify-rotate-prune path as a periodic save. |

### Reading the logs — the grepable series

Every mod log line carries a bracketed series tag, and the tag is the whole filter. **The Go
side carries none** — the relay, the sidecar, the archive and `ringstat` all log structured
`slog` key/value records, so grep them by key instead.

| Tag | What it reports |
|---|---|
| `[M2]` | The general mod series: config, connection, edge state, exports. |
| `[M2-CROSSING]` | Per-simulated-minute crossing counters, **one line per export edge**. |
| `[M2-CMD]` | Every command-file result, the same text as `<cmdfile>.log`. |
| `[M2-FAMILY]` | Family scans, on `MULTIVERSE_FAMILY_REPORT`. |
| `[M2-WORLD]` | World load and seed. |
| `[M3-D10]` | Containment: `event=WRAP`, `event=PLACED`, and each capture verdict. |
| `[M3-LINEAGE]` | The parent-blob collector. |
| `[M4-SAVE]` | **M4.** `event=SAVED`, `event=FAILED`, `event=BUDGET_EXCEEDED`, plus the armed line and each deferral. |
| `[M4-CROWDING]` | **M4.** Per-simulated-minute arrival crowding, **one line per entry edge**. |
| `[M4-PORTAL]` | **M4.** `event=BUILT`, `event=SHOWN`, `event=HIDDEN`, `event=RELAYOUT`. |
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
cumulativeStripEntriesPerSimMin population S W simTick`.

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
budget of Risk 3.

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
bin/relay   --listen 127.0.0.1:8790 --data-dir ./e2e/relay-data \
            --token-file ~/.multiverse-token
bin/archive --relay ws://127.0.0.1:8790/contract-b/v3 --peer-id archive-main \
            --data-dir ./e2e/archive-data --token-file ~/.multiverse-token \
            --http 127.0.0.1:8791
bin/sidecar --listen 127.0.0.1:8787 --relay ws://127.0.0.1:8790/contract-b/v3 \
            --peer-id slot-1 --position 0,0 \
            --data-dir ./e2e/data/slot-1 --token-file ~/.multiverse-token
bin/sidecar --listen 127.0.0.1:8788 --relay ws://127.0.0.1:8790/contract-b/v3 \
            --peer-id slot-2 --position 1,0 \
            --data-dir ./e2e/data/slot-2 --token-file ~/.multiverse-token
bin/sidecar --listen 127.0.0.1:18789 --relay ws://127.0.0.1:8790/contract-b/v3 \
            --peer-id slot-3 --position 2,0 \
            --data-dir ./e2e/data/slot-3 --token-file ~/.multiverse-token
```

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

`--listen` is loopback-only by contract and refuses a wildcard address. The relay's
`--listen` does not: M3 binds it LAN-reachable (`0.0.0.0:8790` by default). Every flag also
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
retry queue's input.

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
bibites-mod/game.sh stop all      # deploy.sh cannot overwrite a DLL a running game holds
farend/make-farend-bundle.sh      # -> farend/dist/farend-bundle.zip
```

The bundle holds `setup-farend.ps1`, `README.md`, a **fresh** `BibitesMultiverse.dll` (it runs
`deploy.sh`), `multiverse-sidecar.exe` (cross-compiled), and the pinned BepInEx 5.4.23.3 zip
(downloaded once into the gitignored `farend/dist/cache/`, SHA-256 verified). The script
refuses to build when `setup-farend.ps1`'s pinned `$AssemblySha256` no longer equals
`bibites-mod/libs/BibitesAssembly.dll` — that pin **is** the version gate on the far end, and a
stale one would let two different game builds into one ring.

**The repo distributes the bundle.** `farend/dist/farend-bundle.zip` is tracked as of
`8463b72`, so the second computer clones the private GitHub repo and takes the zip out of the
checkout instead of receiving a hand-copied file. Only two things still travel by hand:
`~/.multiverse-token` (as `token.txt`) and the relay's LAN address — **neither belongs in the
repo**. Re-run `make-farend-bundle.sh` and commit the new zip whenever the plugin, the sidecar
binary or the `$AssemblySha256` pin changes; a stale committed bundle is a stale far end.

With the zip in hand, its operator runs two commands, and `farend/README.md` is the whole of
their instructions:

```powershell
.\setup-farend.ps1 -RelayHost <relay LAN address> -TokenFile .\token.txt
.\start-slot2.ps1
```

`setup-farend.ps1` finds Steam's copy of the game (registry, the usual folders, and every
extra library in `libraryfolders.vdf`), verifies `BibitesAssembly.dll` against the pin,
installs BepInEx if it is absent, copies the plugin, writes the token to
`%LOCALAPPDATA%\BibitesMultiverse\token.txt` with a user-only ACL, and generates
`start-slot2.ps1` and `stop-slot2.ps1`. `start-slot2.ps1` sets `MULTIVERSE_EXPORT_EDGE=E`,
`MULTIVERSE_RING_SLOT=2`, `MULTIVERSE_SIDECAR_PORT=8787`, `MULTIVERSE_WORLD=M3-Slot2` and
`MULTIVERSE_CMD_FILE` natively — there is no `WSLENV` on that machine — starts the sidecar,
**waits for `ring slot granted` in its log**, and only then starts the game. A failure to join
prints the four usual causes and does not start the game.

### Owner steps: making the relay reachable (elevated, on this machine)

WSL2 here runs in **NAT mode**, and `.wslconfig` sets that deliberately: `networkingMode=mirrored`
breaks Docker Desktop port publishing on this host. A relay on `0.0.0.0:8790` inside WSL is
therefore reachable from Windows but **not from the LAN** until Windows forwards the port into
the VM. Both commands below need an elevated PowerShell and both are **owner steps**;
`e2e/run-m3-lan.sh lanhost` prints them with the current addresses filled in.

```powershell
# once. -Profile Any, NOT Private: an Ethernet NIC classified "Public" silently ignores a
# Private-only rule, and the far end just reports the relay unreachable. See Gotchas.
New-NetFirewallRule -DisplayName "Bibites Multiverse relay" -Direction Inbound `
  -Action Allow -Protocol TCP -LocalPort 8790 -Profile Any

# after every WSL restart: the WSL address changes
netsh interface portproxy delete v4tov4 listenport=8790 listenaddress=0.0.0.0
netsh interface portproxy add v4tov4 listenport=8790 listenaddress=0.0.0.0 `
  connectport=8790 connectaddress=<the WSL address from `lanhost`>
```

**Relay LAN host: `192.168.1.227`.** Confirmed 2026-08-03 by the far end itself: the second
computer ran `setup-farend.ps1 -RelayHost 192.168.1.227`, was granted slot 2, and the relay
reported `ringSize=3`. Give the same value to `setup-farend.ps1 -RelayHost`. It is one of this
machine's Windows IPv4 addresses; `lanhost` lists the candidates, and the home-LAN one is
normally `192.168.x.x` or `10.x.x.x`, never a `172.x` hypervisor address.

**The address alone is not enough — the portproxy behind it must exist.** `192.168.1.227:8790`
only reaches the relay while the `netsh` portproxy above points at the *current* WSL address,
and that address changes on every WSL restart. Re-run `e2e/run-m3-lan.sh lanhost` after a
restart and re-add the portproxy with the value it prints; the LAN host itself does not change,
so this line stays correct even when the far end suddenly cannot connect.

## Gotchas

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
  localhost forwarding resolved `ws://localhost:8790` into the VM, while `ws://127.0.0.1:8790`
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
- **The M3 rig binds four ports:** `8790` (relay, Contract B — bound on `127.0.0.1` for a
  local rehearsal, on `0.0.0.0` for the LAN), and `8787`, `8788`, `18789` (the Contract A
  loopback ports of slots 1, 2 and 3). They are fixed defaults shared by every rig in this
  checkout, so **only one rig can run at a time** — a second one, or a second agent
  working in this repo, silently attaches to or fights over the first one's processes.
  Check with `ss -ltn | grep -E '878[78]|8790|18789'` before starting, and give a throwaway
  smoke test high ports (`--listen 127.0.0.1:0` writes the resolved address to
  `<data-dir>/listen.addr`). The M2 rig used `8787`/`8788`/`8790` and collides head-on. On
  the LAN rig only `8787`, `18789` and `8790` are local; the far end's `8787` is on its own
  machine.
- **The M4 six-slot rig collides with its own defaults, on two ports. Plan the ports before
  building it.** `contract-a.md` §10 gives the six slots the Contract A range `8787`–`8792`.
  The relay's default listen port is **`8790`**, which is **slot 4's** port. The archive's
  default status-page bind is **`127.0.0.1:8791`**, which is **slot 5's**. Both defaults sit
  inside the range they have to avoid, and `ringstat` defaults to the archive's URL, so the
  collision moves with it. Nothing in the repo uses `8795` — a report that mentions it is
  describing a workaround somebody ran by hand, not a checked-in default. The M3 rig dodged
  the same problem differently: it gave slot 3 the five-digit port `18789`. **Pick one of the
  two dodges and write it down** — move the relay and the archive out of the way with
  `--listen` / `MULTIVERSE_RELAY_LISTEN` and `--http`, or give the slots five-digit ports.
  Do not leave it to the defaults; the symptom is a sidecar that cannot bind, or worse, a
  game that connects to the relay.
- **`kill $!` does not kill a program launched in the background *inside* a compound
  command.** In `( … & )`, or in `cmd1 && cmd2 &`, `$!` is the pid of the **subshell**; the
  real program is its child and outlives the kill. A relay started that way survived its own
  cleanup and kept holding `8790`, so the next rig start failed on an address already in use
  while nothing in the script's process table looked wrong. Kill by port
  (`ss -ltnp | grep 8790`) or by pattern (`pkill -f 'bin/relay'`), and check the port is free
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
  `curl http://<this machine's LAN IP>:8790` fails from WSL even when the firewall rule, the
  port proxy and the relay are all healthy. It is a false negative that reads exactly like a
  blocked port, and it costs an hour of chasing the firewall. Test from a Windows PowerShell
  on this machine, or from the second computer — never from WSL.
- **The archive ledger is cumulative, so an archive assertion needs a lower time bound.**
  `migrations.jsonl` keeps every hop of every earlier run, so "wait until a `slot 2 -> slot 3`
  record exists" succeeded instantly against an hour-old record and reported a crossing that
  had not happened yet. Every wait in `run-m3-lan.sh` is filtered by an RFC 3339 mark taken
  when the wait starts; the header line begins with that timestamp, so a string compare is
  the whole filter.
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
