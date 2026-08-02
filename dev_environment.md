# Development Environment

The full loop — edit, build, deploy, run, read logs — runs from WSL with no manual steps.

## Layout

| What | Where |
|---|---|
| Game install (Windows) | `/mnt/c/Program Files (x86)/Steam/steamapps/common/The Bibites` |
| Game assembly | `…/The Bibites_Data/Managed/BibitesAssembly.dll` |
| BepInEx log | `…/The Bibites/BepInEx/LogOutput.log` |
| Plugin project | `bibites-mod/` (source in `src/`, reference DLLs in `libs/`) |
| Decompiled game source | `decompiled/BibitesAssembly/` (654 files, grep this to find APIs) |

## Versions

| Component | Version |
|---|---|
| The Bibites | Steam app 2736860, buildid 22383127; game version TBD — log `Application.version` from the plugin |
| Unity | 6000.0.44f1, **Mono** backend (not IL2CPP — Harmony and decompilation fully work) |
| BepInEx | 5.4.23.3 (win x64), installed in the game directory |
| .NET SDK | 8.0.423 in `~/.dotnet` (not on default PATH — scripts export it) |
| ilspycmd | 9.0.0.7889 (pinned; newer versions need a newer .NET) |

## Workflow

```sh
bibites-mod/deploy.sh          # dotnet build + copy DLL to BepInEx/plugins
bibites-mod/game.sh start      # launch the game, detached from the shell
bibites-mod/game.sh log 60     # read the last 60 BepInEx log lines
bibites-mod/game.sh status     # is the game running?
bibites-mod/game.sh stop       # kill the game
```

Smoke test passed 2026-08-02: plugin `Bibites Multiverse 0.1.0` loads and logs
through the BepInEx chainloader.

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
- `libs/` DLLs are copies pinned to the current game build. After a game update,
  re-copy them and re-run the decompile (`ilspycmd -p -o decompiled/BibitesAssembly
  --referencepath bibites-mod/libs bibites-mod/libs/BibitesAssembly.dll`).
- Steam auto-updates are a risk to signature stability: set the game to "update only
  on launch" in Steam properties, and prefer launching the exe directly (which skips
  the update check) — `game.sh start` does this.

## Useful decompiled entry points found so far

- `ScriptHelpers.VersionTracker`, `Utility.Version` — the game's own save-version
  compatibility machinery; relevant to `bb8-schema` versioning.
