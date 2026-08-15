# `release/` — the package, and how it is published

**Everything a GitHub release needs, built locally and published by hand.** Nothing in this
directory publishes anything: `make-release.sh` produces artifacts and text into `dist/`, and the
four steps at the bottom of this page are the owner's own act.

The channel is **GitHub Releases** (D25). It pushes nothing to anybody, which is why this project
moves its fleet by publication — a release, a matrix row, and a relay-side minimum wire version
raised only after the release that satisfies it exists.

## What is here

| Path | What it is |
|---|---|
| `kit/Install-BibitesMultiverse.cmd` | The Windows double-click launcher. It opens the GUI and uses `RemoteSigned` for its process only |
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
| `make-release.sh` | Builds two add-on archives, optional complete archives, `SHA256SUMS`, and the release page |
| `test-install-uninstall.ps1` | The proof that the Windows uninstall leaves the game as it found it |
| `test-install-uninstall.sh` | The same proof for Linux, and it compares permissions as well as hashes. Runnable with no release build: it stages a kit out of this checkout |
| `dist/` | Build output. **Not tracked** — see `.gitignore`, which says why |

**Two platforms, one mod.** The plugin in every archive is the same file, byte for byte.
`make-release.sh` refuses to build if the Windows and Linux copies disagree. The sidecar,
BepInEx flavor, and installer differ by platform.

The build creates an add-on archive for each platform. Release `0.2.1` also publishes a complete
archive for each platform as the recommended download. Each contains an authorized, unmodified
game payload and a redistribution notice.
Every participant archive contains `public-map.json`. This file lets each installer find the
deployed public map. Each installation still creates its own world identity and secret.
Every archive contains the project `LICENSE` and `THIRD_PARTY_NOTICES.md` files.

The two documents that ship *beside* the release rather than inside it are
[`../docs/support-matrix.md`](../docs/support-matrix.md) — which the installer reads, as JSON
extracted from that same file — and [`../docs/defaults-audit.md`](../docs/defaults-audit.md).

## What this is not: `farend/`

`farend/` is the preserved Windows bundle for the M4 two-computer test map.
It uses map slot 6, a LAN relay, and a private certificate authority.
The scripts keep the slot and test-map assumptions from that evidence run.

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

It needs the game's reference assemblies (`bibites-mod/sync-game-refs.sh`), the .NET SDK and Go.
These tools are only build inputs. Heavy commands run under `nice -n 19` to limit interference with other local work.

Go can omit VCS metadata when a linked worktree points to Git data on another filesystem. In that
case, set `RELEASE_SIDECAR_BUILD_REPO` to a clean checkout of the same commit. The builder checks
the revision and refuses a missing sidecar stamp.

**It refuses to build a release nobody has run.** Before it packages anything it requires:

1. `bibites-mod/libs/BibitesAssembly.dll` must match the Windows row in `docs/support-matrix.md`.
   The mod uses this file as its reference assembly.
2. The plugin must be **byte-identical** to the one in `farend/dist/farend-bundle.zip`.
   This file is the tracked M4 Windows evidence artifact. Every staged copy must use it.
3. The Go source must match the source for the sidecar in that bundle.
   VCS stamps can make two builds from the same source differ as files.
4. If this machine's game directory is readable, the local plugin must also match.

It also checks two things the two-platform matrix made checkable: that **every matrix entry
carries the same keys** and that no two rows share a `(gameVersion, platform)` pair — because both
installers walk the whole list, and PowerShell under `Set-StrictMode` throws on a property one row
happens not to have. And when an unpacked Linux game is readable (`LINUX_GAME_DIR`, defaulting to
the rehearsal's copy), it checks the Linux row's hash against a real file.

**The Linux sidecar is the one artifact with no byte-identity reference.**
The M4 bundle contains a Windows sidecar, so it has no Linux binary to compare.
The Windows gate proves that `go/` matches the source of the tested sidecar.
The Linux build uses that same source for a second target.

A mismatch stops the build and names which side moved.
Rebuild the far-end bundle and repeat its tests before you release changed plugin or sidecar code.

Every archive is built deterministically — fixed timestamps, sorted entries, no extra attributes
— so rebuilding from the same tag and inputs reproduces the checksums on the page. Linux archives
keep their mode bits, and the build reads them back out of the finished zip, including the game
executable in a complete archive.

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

## Publishing — the four steps, by hand

`make-release.sh` deliberately does none of these.

1. **Read `dist/RELEASE-PAGE.md`.** The build refuses unresolved template fields. Make sure that
   the generated page describes the intended artifacts and public map.
2. **Tag the commit the artifacts were built from**, and push the tag:
   `git tag v0.2.1 && git push origin v0.2.1`. The page's links point into the tag, so the
   documentation a reader follows is the documentation this release shipped with.
3. **Create the release** with `dist/RELEASE-PAGE.md` as its body. Attach both add-on archives,
   each complete archive that you built, and `SHA256SUMS`:
   ```sh
   gh release create v0.2.1 \
       release/dist/bibites-multiverse-0.2.1-*.zip \
       release/dist/SHA256SUMS \
       --title "Bibites Multiverse 0.2.1" \
       --notes-file release/dist/RELEASE-PAGE.md
   ```
4. **Read the published page as a stranger would**, in a browser, and check the three things that
   matter most about its shape: the checksum step is **above** the download links; each download
   link's file matches the checksum beside it; and **the Linux row does not read as an
   afterthought** — a reader on that platform should meet the checksum-then-executable-bit
   ordering, the launcher difference and the one-instance-per-game-folder warning without having
   to read the Windows sections first.

   Also open each participant archive and make sure that `public-map.json` contains the intended
   enrollment and relay addresses. It must not contain a world identity or secret.

**A fifth step, if the repository is private:** the page's documentation links resolve only for
somebody who can read the repository. Make it public, or the four participant pages have to
travel with the release instead.
