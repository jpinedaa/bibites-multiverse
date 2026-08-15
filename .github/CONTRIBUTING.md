# Contributing to Bibites Multiverse

Thank you for helping this artificial-life experiment grow. Contributions can include code,
documentation, tests, bug reports, and reproducible experiment ideas.

## Before you start

1. Read the [project README](../README.md) and the
   [system design](../system_decomposition.md).
2. Search the [open issues](https://github.com/jpinedaa/bibites-multiverse/issues).
3. Open an issue before a change to a protocol, stored format, deployment, or release package.

For a security vulnerability, do not open an issue. Read the
[security policy](SECURITY.md) and send a private report.

## Development areas

| Area | Start here | Minimum local test |
|---|---|---|
| Go relay, sidecar, archive, or tools | [`go/`](../go/) and [`dev_environment.md`](../dev_environment.md) | `cd go && go test ./...` |
| Game mod | [`bibites-mod/`](../bibites-mod/) and [`dev_environment.md`](../dev_environment.md) | `dotnet build bibites-mod/BibitesMultiverse.csproj -c Release` |
| Player packages | [`release/README.md`](../release/README.md) | Run the applicable install and uninstall test |
| Participant documentation | [`docs/README.md`](../docs/README.md) | Make sure that all local links resolve |
| Hosted service | [`deploy/README.md`](../deploy/README.md) | Use the documented dry run or local test |

The mod build needs the managed game assemblies in `bibites-mod/libs/`. Do not commit those
assemblies or other game files.

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
