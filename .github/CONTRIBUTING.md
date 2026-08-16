# Contributing to Bibites Multiverse

Thank you for helping this artificial-life experiment grow. Contributions can include code,
documentation, tests, bug reports, and reproducible experiment ideas.

## Before you start

1. Read the [project README](../README.md) and the
   [system design](../system_decomposition.md). Read the
   [current public phase](../STATUS.md).
2. Search the [open issues](https://github.com/jpinedaa/bibites-multiverse/issues).
3. Open an issue before a change to a protocol, stored format, deployment, or release package.

For a security vulnerability, do not open an issue. Read the
[security policy](SECURITY.md) and send a private report.

## Development areas

| Area | Start here | What CI runs on your pull request |
|---|---|---|
| Go relay, sidecar, archive, or tools | [`go/`](../go/) and the [developer guide](../dev_environment.md) | `go vet` for this host and for Windows, `go test ./...`, and the cross-builds the release and the hosting kit ship |
| Game mod | [`bibites-mod/`](../bibites-mod/) and the [developer guide](../dev_environment.md) | Not the build itself — it needs the game assemblies. `release/check-drift.sh` instead: the plugin version has to agree everywhere it is stated, and the tested build recorded in `docs/support-matrix.md` has to describe the mod in the tree |
| Player packages | [`release/README.md`](../release/README.md) | The Windows installer script compiles against a stub payload, every shipped PowerShell file parses, and the release version surface agrees with itself |
| Participant documentation | [`docs/README.md`](../docs/README.md) | Nothing yet. Make sure that all local links resolve |
| Hosted service | [`deploy/README.md`](../deploy/README.md) | `deploy/test-units.sh` and `deploy/test-front-door.sh`, the second against a real nginx |

## What CI runs, and how to run it yourself

Everything above is `.github/workflows/checks.yml`, on every pull request and every push to `main`.
It needs no game file and holds no secret, so a pull request from a fork runs the whole set. Run
the same checks before you push:

```sh
cd go && go vet ./... && go test ./...          # the go job
bash -n $(git ls-files '*.sh')                  # the scripts job
deploy/test-units.sh && deploy/test-front-door.sh
release/check-nsis.sh                           # the installer job; needs makensis
release/bump-version.sh --check                 # the consistency job, first step
release/check-drift.sh                          # ... and its second
```

`test-front-door.sh` skips its nginx syntax check when nginx is not installed; CI installs one so
the check is real. The PowerShell parse needs PowerShell, so it runs on Windows:

```powershell
pwsh -NoProfile -File release/pscheck.ps1 release/kit/*.ps1 farend/setup-farend.ps1
```

**What CI cannot run, and you should.** The mod build needs the managed game assemblies in
`bibites-mod/libs/`, so build it locally with
`dotnet build bibites-mod/BibitesMultiverse.csproj -c Release`. The install-and-uninstall suites
need a real game: `release/test-install-uninstall.sh --real-game-dir …` on Linux, and
`release/test-install-uninstall.ps1 -RealGameDir …` on Windows.

Do not commit those assemblies or any other game file. CI fails a pull request that adds an
unexpected tracked binary, a `.dll` above all. When the binary genuinely belongs in the repository
— a new documentation image, a replaced icon, a test fixture archive — add its path to
[`release/tracked-binaries.txt`](../release/tracked-binaries.txt) in the same pull request, and say
in the description why the repository is that file's only distribution channel.

A change to `bibites-mod/` is not finished until somebody has tested the plugin it builds and
recorded that build in `docs/support-matrix.md` — the machine with the game does both.
[`release/README.md`](../release/README.md) has that procedure, `release/record-tested-build.sh`
prints the record, and `release/check-drift.sh` tells you whether it is outstanding.

**Two things that surprise people.** Some documentation lines are read by the two gates above — the
publish commands in `release/README.md`, three lines of `STATUS.md`, and the Plugin row of
`dev_environment.md` — so a documentation-only change can turn CI red. Run both gates before you
push. And if `consistency` is red on a change that goes nowhere near the release surface, read what
`release/check-drift.sh` printed: an outstanding tested-build record fails it for everybody, is
not yours to fix, and is described under *Day one* in
[`release/README.md`](../release/README.md).

## Make a focused change

- Keep each pull request focused on one behavior or documentation goal.
- Add or update tests when behavior changes.
- Update contracts and documentation when an interface changes.
- Keep credentials, game files, saves, logs, and private deployment data out of Git.
- Do not weaken a security control to make a test pass.

## Submit a pull request

1. Describe the problem and the chosen solution.
2. List the commands that you ran.
3. Link the issue that the pull request closes or advances.
4. State any untested behavior or operational risk.

A maintainer will review the behavior, tests, documentation, and effect on the live experiment.
