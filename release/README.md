# `release/` — the package, and how it is published

**Everything a GitHub release needs, built on the owner's own machine and published from a tag.**
Nothing in this directory publishes anything: `make-release.sh` produces artifacts and text into
`dist/`. Publication is `.github/workflows/release.yml`, which a `v*` tag starts on the owner's
self-hosted runner. The public homepage follows the newest release on its own, and is part of
neither.

The channel is **GitHub Releases** (D25). It pushes nothing to anybody, which is why this project
moves its fleet by publication — a release, a matrix row, and a relay-side minimum wire version
raised only after the release that satisfies it exists.

## What is here

| Path | What it is |
|---|---|
| `windows-installer.nsi` | Builds the single-file Windows setup with per-user shortcuts and uninstall registration |
| `kit/bibites-multiverse.ico` | The setup, desktop, and Start Menu icon |
| `kit/Install-BibitesMultiverse.cmd` | The advanced Windows ZIP entry point. It opens the same GUI |
| `kit/Install-BibitesMultiverse-Gui.ps1` | Selects the included or existing game and starts the installed world by default |
| `kit/Find-BibitesGame.ps1` | Searches Steam, itch.io, and common Windows game locations |
| `kit/Install-BibitesMultiverse.ps1` | The Windows installer and its advanced options |
| `kit/public-map.json` | Public join configuration with the HTTPS enrollment and WSS relay addresses. It contains no credential |
| `kit/Uninstall-BibitesMultiverse.ps1` | Removes what the Windows installer recorded, hash-checked, and nothing else |
| `kit/README.md` | The page inside the Windows archive |
| `kit/install-bibites-multiverse.sh` | The Linux installer. It uses bash, coreutils, `awk`, `unzip`, `file`, and `curl`; no toolchain, root, `jq`, or `python` |
| `kit/uninstall-bibites-multiverse.sh` | Removes what the Linux installer recorded, hash-checked, and nothing else |
| `kit/README-linux.md` | The page inside the Linux archive, staged into it as `README.md` |
| `RELEASE-PAGE.md` | The release page's text, with `@@…@@` fields the build fills in |
| `make-release.sh` | Builds both platform packages, the Windows setup, the two stable-named copies the homepage links, `SHA256SUMS`, and the release page |
| `bump-version.sh` | The release string's single surface. `--print` prints it, `--check` asserts every place that names it agrees, and `<version>` rewrites them all from an explicit allowlist |
| `check-drift.sh` | The half of `make-release.sh` that needs no game: is this tree still the build that was tested, and does the mod version agree everywhere it is stated |
| `check-nsis.sh` | Compiles `windows-installer.nsi` against a stub payload, in about two seconds and with no game file |
| `verify-build-log.sh` | Reads a finished build log and refuses a build whose strongest gates downgraded to a note |
| `pscheck.ps1` | Parses every shipped PowerShell file. It runs on Windows in CI, because this repository is written on Linux |
| `record-tested-build.sh` | Prints the `testedBuild` block for this tree, after a build and a test, ready to paste into `docs/support-matrix.md` |
| `lib/mod-version.sh` | One parse of the plugin's declared `Version`, shared by the gates and the recorder so no two readers can disagree about it |
| `lib/tested-build.sh` | One parse of the matrix's `testedBuild` record — the release reference — shared by `make-release.sh` and `check-drift.sh` |
| `lib/sidecar-manifest.sh` | The input manifest behind check 3 below, sourced by `make-release.sh`, `check-drift.sh` and `record-tested-build.sh`, so CI runs the gate itself rather than a lookalike |
| `tracked-binaries.txt` | Every binary file this repository is allowed to track. The `consistency` job diffs the real set against it, so a game assembly cannot land unnoticed |
| `test-install-uninstall.ps1` | The proof that the Windows uninstall leaves the game as it found it |
| `test-install-uninstall.sh` | The same proof for Linux, and it compares permissions as well as hashes. Runnable with no release build: it stages a kit out of this checkout |
| `../go/cmd/multiverse-launcher/` | Not in this directory, but built here: `make-release.sh` cross-compiles it into every Windows package as `BibitesMultiverseLauncher.exe`, the installed application the shortcuts open |
| `dist/` | Build output. **Not tracked** — see `.gitignore`, which says why |

**Two platforms, one mod.** The plugin in every package is the same file, byte for byte.
`make-release.sh` refuses to build if the Windows and Linux copies disagree. The sidecar,
BepInEx flavor, and installer differ by platform.

The build creates an add-on archive for each platform. Release `0.2.6` publishes one executable
Windows setup as the recommended Windows download, and every Windows package carries
`BibitesMultiverseLauncher.exe`. It publishes a complete Linux archive as the
recommended Linux download; the Linux kit keeps its shell scripts and ships no launcher in this
release. Both contain an authorized, unmodified game payload and a
redistribution notice. The advanced Windows complete ZIP remains available.
Every participant package contains `public-map.json`. This file lets each installer find the
deployed public map. Each installation still creates its own world identity and secret.
Every package contains the project `LICENSE` and `THIRD_PARTY_NOTICES.md` files.

The two documents that ship *beside* the release rather than inside it are
[`../docs/support-matrix.md`](../docs/support-matrix.md) — which the installer reads, as JSON
extracted from that same file — and [`../docs/defaults-audit.md`](../docs/defaults-audit.md).

## What this is not: `farend/`

`farend/` originated as the Windows bundle for the M4 two-computer test map.
It uses map slot 6, a LAN relay, and a private certificate authority.
The scripts preserve those test-map assumptions, but maintainers can rebuild the artifacts.
Nothing in `farend/dist/` is tracked any more: `make-farend-bundle.sh` builds a handover zip
on demand, and it is carried to that machine by hand.

This directory is the participant-facing descendant of the same tested mechanics.
These mechanics include Steam search, the game-build gate, BepInEx placement, and generated process scripts.
The public package uses automatic enrollment or a private-map join-string file.
The map assigns its slot and position.
Only a private map can require a certificate authority.
The public package never tells a participant to disable a security control.
The M4-specific `farend/README.md` does not define public package policy (`m5_considerations.md`, DQ4).

Where a fact belongs to both, the two are kept apart deliberately rather than shared: a change to
this installer must not silently re-cut the far end's bundle, and the far end's pin is that
machine's matrix entry rather than a rule about the map (D22).

## What a release is measured against: the tested build

The reference is a record, not a binary. `docs/support-matrix.md` carries a top-level
`testedBuild` block — the mod version, the sha256 of the plugin that was tested, the
`bibites-mod/` tree it was built from, the commit `cmd/sidecar`'s tested source came from, the
digest of that source's input manifest, the date, and one sentence of evidence. The build refuses
anything else, and `check-drift.sh` asks the same questions from tracked text on every pull
request.

It is self-asserted, and the honest reading is that it catches **forgetting** rather than proving
that a test happened: a mod change that lands without a test, or a sidecar source change that
never reached a tested binary. What it gains over a committed reference binary is that the claim
is one reviewable line in a diff. The strongest check in the build is still gate 3b, which
compares the plugin against the one this machine's game actually loaded.

## Building

```sh
release/make-release.sh
```

To build a complete edition, add one or both clean game directories and the redistribution notice:

```sh
release/make-release.sh \
  --windows-game-dir <clean-windows-game> \
  --linux-game-dir <clean-linux-game> \
  --game-redistribution-notice release/GAME-REDISTRIBUTION-NOTICE.txt
```

The build rejects a payload that does not match the support matrix or that already contains mod
files. The project received permission to redistribute the game with this installer. The notice
records that permission without changing the game's copyright or the source-code license.

It needs the game's reference assemblies (`bibites-mod/sync-game-refs.sh`), the .NET SDK, Go,
Git, Python 3, `tar`, `zip`, `unzip`, and `curl`. These tools are only build inputs.
Heavy commands run under `nice -n 19` to limit interference with other local work.
A Windows complete build also needs NSIS 3.09 or newer. Set `MAKENSIS` when the compiler is not
on `PATH`.

Go can omit VCS metadata when a linked worktree points to Git data on another filesystem. In that
case, set `RELEASE_SIDECAR_BUILD_REPO` to a clean checkout of the same commit. **That checkout now
builds two binaries** — the sidecar and the launcher, each for Windows and Linux — and the builder
checks the revision and refuses a missing stamp on either one. The variable keeps its name because
it is the clean Go build checkout rather than one command's.

**The builder checks package consistency, not remote execution.** Before it packages anything,
it checks these inputs:

1. `bibites-mod/libs/BibitesAssembly.dll` must match the Windows row in `docs/support-matrix.md`.
   The mod uses this file as its reference assembly.
2. The freshly built plugin's sha256 must equal `testedBuild.pluginSha256`.
   This is the mod that was tested, and no other.
3. The repository inputs and module versions selected by `go list` for Windows `cmd/sidecar`
   must digest to `testedBuild.sidecarInputsSha256`. Unrelated Go commands do not affect this
   comparison. VCS stamps can make two builds from the same inputs differ as files, which is why
   the inputs are compared rather than the binary.
4. If this machine's game directory is readable, the local plugin must also match.

Two cheaper refusals come before all four, in the first seconds and with no toolchain at all:
`release/bump-version.sh --check` must agree, and `bibites-mod/src/MultiversePlugin.cs`'s `Version`
must equal the `mod` field of `docs/support-matrix.md` on both rows **and** `testedBuild.mod`.
Both were things a release could previously ship past.

**The launcher is deliberately outside gate 3.** That gate digests the package graph of
`cmd/sidecar`, and the launcher is a separate command that
shares none of it — which is also why adding it could not disturb the sidecar's manifest. What
stands in its place is the same VCS-stamp rule the sidecar gets, plus a check that the Windows
build really is a `PE32+` executable, plus `go vet` over its two packages. It uses the standard
library only, so there is no module set to compare.

These comparisons do not prove that either artifact ran anywhere. What was run, and where, is the
`evidence` sentence of the tested-build record, written by the person who ran it.

It also checks two things the two-platform matrix made checkable: that **every matrix entry
carries the same keys** and that no two rows share a `(gameVersion, platform)` pair — because both
installers walk the whole list, and PowerShell under `Set-StrictMode` throws on a property one row
happens not to have. And when an unpacked Linux game is readable (`LINUX_GAME_DIR`, defaulting to
the rehearsal's copy), it checks the Linux row's hash against a real file.

**The Linux sidecar is the one artifact with no reference of its own.**
The tested build records one `cmd/sidecar` input digest, and both binaries are built from that
same Go package in this tree, so gate 3 covers the Linux build too.
It does not show that either binary ran successfully.

A mismatch stops the build and names which side moved.

**Whenever `bibites-mod/` or `cmd/sidecar` changes, the tested build is re-recorded — before the
release.** That record is what checks 2 and 3 compare against, so a mod change that lands without
it leaves the tree unreleasable. Four legs, on the machine that has the game:

1. Build and **test it**. `dotnet build bibites-mod/BibitesMultiverse.csproj -c Release`, then
   `bibites-mod/deploy.sh` to put that plugin in this machine's game, then run it and watch it.
   The record's `evidence` sentence is this leg written down, and nothing in the repository can
   perform it.
2. `release/record-tested-build.sh`, which prints the block:

   ```sh
   release/record-tested-build.sh --tested-on 2026-08-20 \
       --evidence 'the six-world deployment ran on this build for three days'
   ```

   It computes the four values by hand-typing none of them — the plugin's `sha256`,
   `git rev-parse HEAD:bibites-mod`, `git rev-parse HEAD`, and the `sha256` of the manifest
   `lib/sidecar-manifest.sh` writes — and warns when the tree is dirty or when this machine's game
   holds a different plugin. Run it from a committed tree: `sidecarSourceCommit` names `HEAD`.
3. Paste the block over `testedBuild` in
   [`../docs/support-matrix.md`](../docs/support-matrix.md), and update the human table in that
   document's *The tested build* section in the same edit.
4. The `mod` field on **both** rows of the matrix, the Mod column of both tables above the JSON,
   and the Plugin row of [`../dev_environment.md`](../dev_environment.md), whenever the plugin's
   `Version` moved.

Each of the four values is checkable by anyone reading the diff, which is the point of recording
them rather than committing a reference binary.

`release/check-drift.sh` tells you whether any of that is outstanding. It answers from tracked text
alone — no game files, no .NET, no network, no history — and CI runs it on every pull request. Run
it before you tag anything.

Every artifact is built deterministically with fixed timestamps and sorted entries. Rebuilding
from the same tag and inputs reproduces the checksums on the page, **for the same NSIS build**: the
Windows setup's bytes depend on the compiler that produced it, which is why the release machine
keeps the `MAKENSIS` it has always used and CI's stub compile uses a packaged one only to prove the
installer script still compiles. Linux archives keep their mode bits, and the build reads them back
out of the finished zip, including the game executable in a complete archive.

## Testing the install and the uninstall

```sh
release/test-install-uninstall.ps1 -RealGameDir <clean-or-installed-Windows-game>
release/test-install-uninstall.sh --real-game-dir <the Linux game directory>
```

The Windows suite copies a real supported game into each positive sandbox. The Linux suite copies
a real supported game into each positive sandbox. Each runs the real installer and uninstaller and
requires the tree to be **hash-for-hash identical** to its initial state. The Linux suite also
checks permissions. Neither changes the source game, a trust store, or a live process.

**The Linux proof needs no release build**: with no `--kit-dir` it stages a kit out of this
checkout — the two real scripts, the matrix extracted from `docs/support-matrix.md`, the real
BepInEx archive, and stand-ins for the plugin and the sidecar, neither of which it ever executes.
Both suites include a complete-package scenario: no game path is supplied, the game lands in a
versioned managed runtime, uninstall removes unchanged payload files, and a user-added file is
kept. Linux also covers **the other platform's build of the same game version** and **a kit file
that fails its manifest**. Its public-enrollment scenario also proves safe retry and identity
reuse without network access.

The Windows setup supports a no-install probe. Run the finished executable with `/PROBE` on
Windows. A successful probe proves that the executable can unpack its real payload and load the
embedded GUI.

**The build-side smoke checks**, which need no game and no release build:

```sh
cd go && go vet ./... && go test ./internal/launcher/... ./cmd/multiverse-launcher/...
cd go && GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -buildvcs=false -o /tmp/l.exe ./cmd/multiverse-launcher
cd go && GOOS=linux   GOARCH=amd64 CGO_ENABLED=0 go build -buildvcs=false -o /tmp/l    ./cmd/multiverse-launcher
bash -n release/make-release.sh release/test-install-uninstall.sh release/kit/*.sh
```

## What CI checks

`.github/workflows/checks.yml` runs on every pull request and every push to `main`, on
GitHub-hosted runners. It holds no secret and needs no game file, and the release runner's label
appears nowhere in it, so a pull request from a fork is safe to run.

| Job | What it proves | The same check locally |
|---|---|---|
| `go` | `go vet` for this host and for Windows, `go test ./...`, and the same cross-builds the release and the hosting kit perform | `cd go && go vet ./... && go test ./...` |
| `scripts` | `bash -n` over every tracked `*.sh`, then `deploy/test-units.sh` and `deploy/test-front-door.sh` against a real nginx | `bash -n` over the scripts, then the two `deploy/test-*.sh` |
| `installer` | `windows-installer.nsi` compiles against a stub payload | `release/check-nsis.sh` |
| `powershell` | every shipped `*.ps1` parses; PSScriptAnalyzer runs beside it, advisory only | `pwsh -NoProfile -File release/pscheck.ps1 release/kit/*.ps1 farend/setup-farend.ps1` — Windows only |
| `consistency` | `release/bump-version.sh --check`, `release/check-drift.sh`, and the tracked binary files against `release/tracked-binaries.txt` | `release/bump-version.sh --check && release/check-drift.sh` |

`consistency` checks out shallow. Nothing in it reads history: `check-drift.sh` compares this tree
against values recorded in `docs/support-matrix.md`, and both sides of every comparison come from
HEAD's own tree. A step that walks history has to restore `fetch-depth: 0` in the same change.

**What CI still cannot do.** The two install-and-uninstall suites need a real game, and
`test-install-uninstall.ps1` needs Windows as well: CI parses that file, it does not run it. The
launcher's Windows process primitives — the detached spawn, the owner-only credential file, and the
ask-before-forcing stop — compile here and are proved only on Windows. And the build's strongest
checks, 1, 2, and 4 above, need the proprietary game bytes, which is why the release build runs on
the owner's machine rather than on a hosted runner.

## Cutting a release

`make-release.sh` still publishes nothing. Publication is `.github/workflows/release.yml`, and a
`v*` tag on `main` is what starts it.

Settle the tested build first. `release/check-drift.sh` must be green before you tag anything: an
untested tree stops the build on the release machine minutes in, after the game payload has already
been staged. It is **not** green today — see *Day one* below.

The machine side of *The release runner* is built and smoke-tested; the `release` environment
exists, deliberately without a required reviewer. Read that section before the first tag, because
it says which controls are in place, which were left out while there is one maintainer, and what
has to change the day there are two.

```sh
# 1  bump the release string, on a branch off a clean main
git switch -c release/<version>
release/bump-version.sh <version>
#    work through the review list it prints. The dates, the matrix's "published" and "tested"
#    strings, and STATUS.md's release paragraph are yours to write, not the tool's
release/bump-version.sh --check
git add -- $(git diff --name-only)      # exactly what the bump touched, nothing else
git commit -m "Set the release to <version>"
git push -u origin release/<version>
#    open the pull request. checks.yml must be green before it is merged

# 2  tag the merge commit on main
git switch main && git pull --ff-only
git tag -a v<version> -m "Bibites Multiverse <version>"
git push origin v<version>

# 3  on the release machine: read the queue, then run the runner for exactly one job
gh run list --repo jpinedaa/bibites-multiverse --workflow release.yml --limit 5
cd /srv/bibites-release/runner && ./run.sh --once

# 4  watch it. There is no approval pause while there is one maintainer
gh run watch --repo jpinedaa/bibites-multiverse
```

Step 1 has a button as well: the `bump` workflow takes a version, runs the same script on a branch,
pushes it, and prints the compare link and the same review list. It stops there on purpose — a pull
request opened by a workflow does not start `checks.yml`, so the owner opens the pull request.

The order of steps 2 and 3 does not matter: a tag pushed while the runner is stopped queues the
job, and starting the runner picks it up. That is also why step 3 reads the queue first — the
runner takes whatever is waiting, not only the run you meant to start. *The release runner* below
says what to do with anything unexpected.

**What the workflow does.** The same build the last two releases ran by hand, with the checks that
used to be done by eye turned into refusals:

1. It refuses unless the tag is a `vMAJOR.MINOR.PATCH` tag, is on `main`, names the release
   `bump-version.sh --print` reports, and has no GitHub release already. A moved tag is a real case
   here: delete the existing release first, deliberately, by hand.
2. It refuses unless the machine's own inputs are there — the reference assemblies, the BepInEx
   cache, both clean game directories, and a **readable** plugin in this machine's game, because
   check 4 turns into a note when that file cannot be read. Those inputs are copied in from fixed
   paths outside the workspace into gitignored directories, so the tree stays clean and
   `--allow-dirty` is never used.
3. It runs `release/check-drift.sh`, then `make-release.sh` with both game directories, tee-ing the
   build to a log.
4. It runs `release/verify-build-log.sh`, which refuses a log missing any of the twenty-one success
   lines or carrying one of the three silent downgrades. Two of the build's strongest checks
   degrade to a note when an input is unreadable; under a dedicated runner user that is exactly how
   a check stops running for months without anybody noticing.
5. It verifies the artifacts independently: `SHA256SUMS`, each archive against its own
   `MANIFEST.sha256`, `LICENSE` and `THIRD_PARTY_NOTICES.md` and `public-map.json` and the launcher
   in the archives that must carry them, the VCS stamp on all three Go binaries pulled back out of
   the shipped zips, and every download row in `RELEASE-PAGE.md` re-hashed against the real file.
6. It uploads the build log, `RELEASE-PAGE.md`, and `SHA256SUMS` as workflow artifacts — never the
   packages, because GitHub Releases is their channel.
7. It publishes with the same `gh release create … --verify-tag … --latest` command, now carrying
   eight assets — the six the last releases used, plus the two stable-named copies the homepage
   links — then verifies what it published: eight assets uploaded, not a draft, `releases/latest`
   resolving to the tag, `/releases/latest/download/` resolving to this tag for both stable names,
   and a round trip that downloads two assets and compares them byte for byte against the local
   build.

**What is still yours, afterwards.** Read the published page as a stranger would, in a browser, and
check the three things that matter most about its shape: the checksum step is **above** the
download links; each download link's file matches the checksum beside it; and **the Linux row does
not read as an afterthought** — a reader on that platform should meet the
checksum-then-executable-bit ordering, the `run_bepinex.sh` difference and the
one-instance-per-game-folder warning without having to read the Windows sections first.

Also open each participant archive and probe the Windows setup. Make sure that `public-map.json`
contains the intended enrollment and relay addresses. It must not contain a world identity or
secret.

**The homepage needs nothing. No deployment is part of a release.** `bibitesmultiverse.com` names
no release: its two download buttons and its checksum link address
`https://github.com/jpinedaa/bibites-multiverse/releases/latest…`, which GitHub answers out of
whichever release is newest. `make-release.sh` publishes a byte-for-byte copy of the Windows setup
and the Linux complete package under the two names that never change —
`bibites-multiverse-windows-x64-setup.exe` and `bibites-multiverse-linux-x64-complete.zip` — and
the publish step uploads them, because `/releases/latest/download/<name>` only works for an asset
whose name is the same in every release. The moment the release is published, the homepage is
serving it. Open it afterwards and press both buttons, because a broken link is still a broken
link — but there is nothing to rebuild, no host value to set, and no relay outage to schedule.

A host whose `/etc/multiverse/deploy.env` still carries an `MV_HOMEPAGE_RELEASE=` line is fine:
nothing reads it any more. Delete it at the next deployment that happens for some other reason.

**And one more, if the repository is ever made private:** the page's documentation links resolve
only for somebody who can read the repository. Keep it public, or the four participant pages have
to travel with the release instead.

## The release runner — the owner's setup

`release.yml` is the only workflow that carries the release runner's label, because it is the only
one that needs a proprietary input. Registering that runner is a one-time act, and it is
deliberately not a daemon.

**Read this before building any of it, because it decides what the rest is worth.** On a tag push,
GitHub executes the workflow file **as it exists in the tag's own tree**, not as it exists on
`main`. Whoever can push a tag therefore supplies the workflow definition: `environment: release`
can simply be deleted from it, the `if: github.repository == …` guard still passes, `runs-on:`
still names this machine, and any `run:` step then executes as the runner user on the box that
holds this project's production access. Nothing written in this repository can prevent that,
because the attacker would be writing the file. **The controls that matter live outside the
workflow, and they are listed below.**

**Who can do that today: one person.** `jpinedaa/bibites-multiverse` is public, and its only
principal with write access is the owner, who is also its only admin. A tag ruleset and branch
protection would therefore restrain nobody but the owner — so they are deliberately **not** in
place, and the commands that add them are kept under *When a second maintainer joins* rather than
run now. What is in place is the part that is worth something with one maintainer: the runner is
not a daemon, and nothing reaches this machine unless somebody starts it by hand for one job.

### The machine, as built

| | |
|---|---|
| Runner directory | `/srv/bibites-release/runner` (the linux-x64 tarball from `actions/runner`, its published SHA-256 checked before unpacking) |
| Runs as | **`ubuntu`** — the owner's own account, not a dedicated user |
| Registered as | name `aorus-x570-wsl`, repository scope, labels `self-hosted`, `Linux`, `X64`, `bibites-release`, work directory `_work` |
| Service | **none.** `./run.sh --once` per release, by hand |
| Inputs | `/srv/bibites-release/inputs`, outside every checkout |
| NSIS | the distribution's `nsis` / `nsis-common` `3.09`, at `/usr/bin/makensis` with `/usr/share/nsis` |

**Running as `ubuntu` is a deliberate trade-off, and it is the weakest thing here.** The design
asked for a dedicated user with no read access to `~/.aws`, `~/.ssh`, `~/.multiverse` or the private
operations checkout. `ubuntu` reads all of them. The argument for it is that a separate user buys
nothing while the owner is the only principal who can push a tag or dispatch the workflow: the code
the runner would execute is the owner's own. The argument against it is that the separation is
exactly what would contain a hostile tag on the day somebody else has write access — so **a
dedicated runner user belongs in the same sitting as the second maintainer's account**, beside the
changes under *When a second maintainer joins*.

One access the runner needs whatever user it runs as: read on the plugin installed in this
machine's game (`…/Steam/steamapps/common/The Bibites/BepInEx/plugins/BibitesMultiverse.dll`), or
check 4 silently stops running. The workflow refuses in its first seconds when that file is
unreadable, and `verify-build-log.sh` refuses again afterwards.

### The inputs, outside every checkout

`actions/checkout` wipes the workspace between runs, so `release.yml` copies these in on every run
and never depends on what a previous build left behind.

| `/srv/bibites-release/inputs/…` | What it is |
|---|---|
| `game-refs/` | a read-only copy of the 13 reference assemblies (`BibitesAssembly.dll` among them) |
| `bepinex-cache/` | a read-only copy of both `BepInEx_*_x64_*.zip` archives. Without them the build downloads instead, and the release's SHA pins stop being a local guarantee |
| `windows-game` | a symlink to the clean Windows game directory |
| `linux-game` | a symlink to the clean Linux game directory. It must be a *fully unpacked* game: the matrix row's hash is checked against its `The Bibites_Data/Managed/BibitesAssembly.dll`, and the check downgrades to a note when that file is not there |

They are copies and symlinks, not the working tree, so a mod rebuild or a `git clean` in a checkout
cannot change what a release is built from. Refresh them on purpose when the game version moves.

### The runner's `.env`

`/srv/bibites-release/runner/.env` — never the repository, and never a GitHub secret. The runner
exports every line of it into the job.

| Variable | What it must be |
|---|---|
| `MV_RELEASE_GAME_REFS` | the directory holding the 13 reference assemblies, `BibitesAssembly.dll` among them. Copied into `bibites-mod/libs/` |
| `MV_RELEASE_BEPINEX_CACHE` | a directory holding `BepInEx_win_x64_*.zip` and `BepInEx_linux_x64_*.zip`. Copied into `farend/dist/cache/`; without it the build reaches the network instead |
| `MV_RELEASE_WINDOWS_GAME` | a clean Windows game directory containing `The Bibites.exe` |
| `MV_RELEASE_LINUX_GAME` | a clean Linux game directory containing an executable `The Bibites.x86_64` |
| `LINUX_GAME_DIR` | the unpacked Linux game the matrix row's hash is checked against. It defaults to `MV_RELEASE_LINUX_GAME`; it is set explicitly here so the value is stated once |
| `MAKENSIS` | the same NSIS build every release has used |
| `NSISDIR` | its data directory. An installed `makensis` finds its own, so this is strictly optional — it is set anyway, because a value that is stated is a value that can be checked, and because the preflight then reports `NSISDIR set` rather than leaving the reader to wonder |
| `GOROOT`, `DOTNET_ROOT` | the Go and .NET toolchains `make-release.sh` builds with. `release.yml` resolves both from here and puts them on the job's `PATH` itself |
| `GOPATH` | the machine's real `GOPATH`. A job reads no login profile, so without this Go takes its default (`~/go`) and re-downloads the whole module cache |
| `PATH` | pinned explicitly. `config.sh` writes the configuring shell's `PATH` into `.path`, which on this machine carried some sixty Windows interop entries; pinning it in `.env` makes the job independent of the shell `./run.sh` was started from. Nothing in the release build calls a Windows executable |

The NSIS entry is the one with history behind it. The design (E6) warned that releases so far used a
hand-unpacked NSIS under `/tmp`, that `/tmp` does not survive a reboot, and that the distribution
package was "a different build" — which would have quietly changed the setup's checksum. It is not
a different build: the `makensis` the distribution installs is **byte-identical** to the copy
`0.2.5` and `0.2.6` were built with, because the hand-unpacked copy was that same package unpacked
by hand. The reproducible-checksum claim survives the move to `/usr/bin/makensis`.

### The gates that are in place

**1. The runner is started by hand, for one job, only when a release is expected.** With one
maintainer this is not one control among several; it is *the* control. `./run.sh --once` runs a single
job and exits. Never install it as a service: a service is an always-listening agent on the machine
that holds production access, and it takes whatever is queued, whenever it was queued and by
whoever queued it. Look at the queue before you start it:

```sh
gh run list --repo jpinedaa/bibites-multiverse --workflow release.yml --limit 5
gh run view <id> --repo jpinedaa/bibites-multiverse     # the commit must be the tag you just pushed
gh run cancel <id> --repo jpinedaa/bibites-multiverse   # anything you did not start
```

That a queued job waits for the runner is the convenience which makes the order of tagging and
starting free. It is also the path by which a job you did not start would reach the machine. Look
first, then start.

**2. The `release` environment exists, so that GitHub did not create it.** A job that names an
environment which does not exist gets it **auto-created, with no protection rule on it**. A missing
environment is not a loud failure; it is a gate silently not being there — and worse, one that
*looks* configured in the workflow file. It was created deliberately instead, and **without a
required reviewer**: an approval prompt that only ever asks the one person who started the run is
a keystroke, not a control. What it does carry is a deployment ref policy, so the environment is a
real statement about which refs may release rather than an empty name:

```sh
gh api repos/jpinedaa/bibites-multiverse/environments/release/deployment-branch-policies \
    --jq '.branch_policies[] | [.type, .name] | @tsv'
# tag     v*     — every real release
# branch  main   — workflow_dispatch, which runs on main and checks out the tag input
```

Read this as it is, not as more than it is:

```sh
gh api repos/jpinedaa/bibites-multiverse/environments/release \
    --jq '.protection_rules[] | select(.type=="required_reviewers")'
```

Empty output means there is no approval gate. **That is the current, deliberate state.** The value
of having created the environment anyway is that adding the reviewer later is one command against
an object that already exists, instead of a discovery that the gate was never there.

**3. The actor guard in `release.yml`.** The first step refuses any run whose `github.actor` is not
the owner. It is belt and braces and it says so: a hostile tag's own tree can delete the step. It
stops the honest accident, which is the failure mode that actually exists here.

### When a second maintainer joins

The moment anybody else has write access, three things change on the same day, before their account
is created. Each is written out here so that nothing has to be re-derived under time pressure.

**A ruleset restricting who may create or update a `v*` tag.** This becomes the primary control: it
is the only one a tag-supplied workflow file cannot revoke, because it stops the tag from existing
at all.

*In the UI:* Settings → Rules → Rulesets → **New ruleset → New tag ruleset**. Name it, set
Enforcement to **Active**, target tags matching `v*`, tick **Restrict creations**, **Restrict
updates** and **Restrict deletions**, and put only the repository admin on the bypass list.

```sh
gh api -X POST repos/jpinedaa/bibites-multiverse/rulesets --input - <<'JSON'
{ "name": "release tags",
  "target": "tag",
  "enforcement": "active",
  "conditions": { "ref_name": { "include": ["refs/tags/v*"], "exclude": [] } },
  "bypass_actors": [ { "actor_id": 5, "actor_type": "RepositoryRole", "bypass_mode": "always" } ],
  "rules": [ { "type": "creation" }, { "type": "update" }, { "type": "deletion" } ] }
JSON
gh api repos/jpinedaa/bibites-multiverse/rulesets --jq '.[] | [.name, .target, .enforcement] | @tsv'
```

`actor_id: 5` is the repository-admin role.

**A required reviewer on the `release` environment.** The environment is already there, so this is
an edit, not a creation — and the deployment ref policy above stays as it is.

*In the UI:* Settings → Environments → `release` → tick **Required reviewers** and add yourself.

```sh
owner=$(gh api users/jpinedaa --jq .id)
gh api -X PUT repos/jpinedaa/bibites-multiverse/environments/release --input - <<JSON
{ "reviewers": [ { "type": "User", "id": $owner } ],
  "deployment_branch_policy": { "protected_branches": false, "custom_branch_policies": true } }
JSON
gh api repos/jpinedaa/bibites-multiverse/environments/release \
    --jq '.protection_rules[] | select(.type=="required_reviewers")'
```

The verification is the point: empty output still means there is no gate.

**Branch protection on `main`.** Until `main` is protected, "the tag is on `main`" is only as
strong as `main` is. Require a pull request, and set Actions → **Fork pull request workflows** to
require approval for all outside collaborators in the same sitting.

```sh
gh api -X PUT repos/jpinedaa/bibites-multiverse/branches/main/protection --input - <<'JSON'
{ "required_status_checks": { "strict": true, "contexts": [] },
  "required_pull_request_reviews": { "required_approving_review_count": 0 },
  "enforce_admins": false,
  "restrictions": null }
JSON
```

Start with an empty `contexts` list, and add `go`, `scripts`, `installer`, `powershell` and
`consistency` once they have been green at least once — see *Day one* below.

**And a dedicated runner user**, per *The machine, as built* above: one that cannot read `~/.aws`,
`~/.ssh`, `~/.multiverse` or the private operations checkout, but can still read the plugin in this
machine's game. Re-register the runner as that user and move `/srv/bibites-release` to it.

### Smoke-testing the runner without publishing anything

`workflow_dispatch` on a tag that **already has a release** exercises the whole front of the job —
the runner picks it up, the label match is proved, and every cheap guard runs — and then stops at
*The release does not exist yet*, before a single proprietary byte is copied.

```sh
cd /srv/bibites-release/runner && ./run.sh --once &      # exits after one job
gh workflow run release.yml --repo jpinedaa/bibites-multiverse -f tag=<an existing tag>
gh run watch --repo jpinedaa/bibites-multiverse
gh run view <id> --repo jpinedaa/bibites-multiverse --log
```

Read the step list, not just the red cross: what is being tested is *where* it stopped. A tag older
than this CI package stops earlier still, at *The version surface agrees with the tag*, because
`release/bump-version.sh` does not exist in that tag's tree — the workflow runs from `main`, the
scripts run from the checkout. Either stop proves the machine; neither builds or publishes
anything. Afterwards, make sure the runner process really exited:

```sh
pgrep -af Runner.Listener      # nothing
```

### Day one

The runner is registered, its inputs are in place, and the front of the release job has been proved
against a real dispatch. Two things are still not true, and both belong before the first tag.

**The tested build on `main` is out of date**, so `release/check-drift.sh` fails and the
`consistency` job is red on every pull request — including the one that introduces CI. The mod
moved to `0.6.5` after the recorded build and nobody has tested that plugin yet. Clear the debt
rather than paper over it, and clear it first: it is the four legs under *Building*, applied once
to catch up — build the `0.6.5` plugin, deploy it to this machine's game, run it, then
`release/record-tested-build.sh` and paste the block, with `0.6.5` on both matrix rows and in
`dev_environment.md`. Merging CI into a `main` that is genuinely releasable is the whole point of
doing it first.

**Nothing has run yet.** Do not make `checks.yml` a required status check until it has been green
at least once. `go test ./...` and the nginx leg of the front-door check have never run on a hosted
runner, and a first run read under a red banner teaches nobody anything. Watch one green run, then
add the five job names to the branch protection under *When a second maintainer joins* — which is
also the sitting in which that branch protection starts existing.

**No secret is needed, and none is stored.** `GITHUB_TOKEN` with `contents: write` publishes the
release. There is no personal access token, and no game byte is ever uploaded to GitHub as a build
input — not as a secret, not in a private repository. The proprietary inputs stay on the machine
that is licensed to hold them.

## The manual fallback

The workflow wraps a build the owner can still run, and this is the path back when the runner is
unavailable. It is how the last two releases were published, before the workflow existed.

1. **Build from a clean full clone of the tag**, not from a linked worktree, and tee the build to a
   log. Go omits the VCS revision when a worktree's Git data lives on another filesystem, and the
   build refuses a missing stamp on any of them; `RELEASE_SIDECAR_BUILD_REPO` is the
   escape hatch described above, but a full clone is simpler.
   ```sh
   MAKENSIS=… NSISDIR=… ./release/make-release.sh \
       --windows-game-dir <clean-windows-game> \
       --linux-game-dir <clean-linux-game> \
       --game-redistribution-notice release/GAME-REDISTRIBUTION-NOTICE.txt \
       2>&1 | tee release-build.log
   ```
2. **Run `release/verify-build-log.sh release-build.log` on that log.** By hand this is the only
   thing that catches a check which downgraded to a note because one path was not readable.
3. **Read `dist/RELEASE-PAGE.md`.** The build refuses unresolved template fields. Make sure that
   the generated page describes the intended artifacts and public map.
4. **Tag the commit the artifacts were built from**, and push the tag:
   `git tag v0.2.6 && git push origin v0.2.6`. The page's links point into the tag, so the
   documentation a reader follows is the documentation this release shipped with.
5. **Create the release** with `dist/RELEASE-PAGE.md` as its body. Attach both add-on archives,
   each complete archive that you built, the Windows setup, the two stable-named copies, and
   `SHA256SUMS` — the same eight assets the workflow publishes. **The two unversioned names are not
   optional:** the homepage links them through `/releases/latest/download/`, so a release published
   without them breaks both download buttons, and the version globs below do not match them.
   ```sh
   gh release create v0.2.6 \
       release/dist/bibites-multiverse-0.2.6-*.zip \
       release/dist/bibites-multiverse-0.2.6-*.exe \
       release/dist/bibites-multiverse-windows-x64-setup.exe \
       release/dist/bibites-multiverse-linux-x64-complete.zip \
       release/dist/SHA256SUMS \
       --repo jpinedaa/bibites-multiverse --verify-tag --latest \
       --title "Bibites Multiverse 0.2.6" \
       --notes-file release/dist/RELEASE-PAGE.md
   ```
6. **Verify what you published**, because nothing else will: `sha256sum -c SHA256SUMS`, each
   archive against its own `MANIFEST.sha256`, a `gh release download` round trip compared with the
   local build, and `curl -sI …/releases/latest/download/bibites-multiverse-windows-x64-setup.exe`
   redirecting to this tag. Then the by-hand reading above; the homepage follows on its own.

The version literals in steps 4 and 5 are the current release, and `release/bump-version.sh` moves
them with every bump — they are on its allowlist by exactly the shape written here. Reword those
two commands and the allowlist entry has to move with them.
