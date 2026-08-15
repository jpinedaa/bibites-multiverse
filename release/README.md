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
| `kit/Install-BibitesMultiverse.ps1` | The Windows installer a stranger runs. Windows PowerShell 5.1 and PowerShell 7; no toolchain, no administrator rights |
| `kit/Uninstall-BibitesMultiverse.ps1` | Removes what the Windows installer recorded, hash-checked, and nothing else |
| `kit/README.md` | The page inside the Windows archive |
| `kit/install-bibites-multiverse.sh` | The Linux installer. bash, and `sha256sum`/`awk`/`unzip`/`file`; no toolchain, no root, and no `jq` or `python` — its JSON reader is 40 lines of awk, shared with the uninstaller |
| `kit/uninstall-bibites-multiverse.sh` | Removes what the Linux installer recorded, hash-checked, and nothing else |
| `kit/README-linux.md` | The page inside the Linux archive, staged into it as `README.md` |
| `RELEASE-PAGE.md` | The release page's text, with `@@…@@` fields the build fills in |
| `make-release.sh` | Builds `dist/` — two add-on archives, optional authorized complete archives, one `SHA256SUMS`, and the release page |
| `test-install-uninstall.ps1` | The proof that the Windows uninstall leaves the game as it found it |
| `test-install-uninstall.sh` | The same proof for Linux, and it compares permissions as well as hashes. Runnable with no release build: it stages a kit out of this checkout |
| `dist/` | Build output. **Not tracked** — see `.gitignore`, which says why |

**Two platforms, one mod.** The plugin in every archive is the same file, byte for byte.
`make-release.sh` refuses to build if the Windows and Linux copies disagree. The sidecar,
BepInEx flavor, and installer differ by platform.

The default build creates an add-on archive for each platform. An authorized build can also
create a complete archive with an unmodified game payload and its separate license file.
Every archive contains the project `LICENSE` and `THIRD_PARTY_NOTICES.md` files.

The two documents that ship *beside* the release rather than inside it are
[`../docs/support-matrix.md`](../docs/support-matrix.md) — which the installer reads, as JSON
extracted from that same file — and [`../docs/defaults-audit.md`](../docs/defaults-audit.md).

## What this is not: `farend/`

`farend/` is the **project's own second computer** — map slot 6, one known person, a LAN relay
with its own certificate authority, a bundle carried by hand and a slot number baked into the
script. It stays exactly as it is, and the living deployment still runs from it.

This directory is the stranger-facing descendant of the same mechanics: the Steam search, the
game-build gate, the BepInEx placement and the generated start and stop scripts are `farend`'s,
proven. What changed is everything about the audience — a join string instead of a hand-carried
token file, no slot and no position (the map places you), no certificate authority unless the map
is private, and **no instruction to switch a security control off**, which is the one thing
`farend/README.md` does and a public package may not (`m5_considerations.md`, DQ4).

Where a fact belongs to both, the two are kept apart deliberately rather than shared: a change to
this installer must not silently re-cut the far end's bundle, and the far end's pin is that
machine's matrix entry rather than a rule about the map (D22).

## Building

```sh
release/make-release.sh
```

To build an authorized complete edition, add one or both game directories and the publisher's
license file:

```sh
release/make-release.sh \
  --windows-game-dir <clean-windows-game> \
  --linux-game-dir <clean-linux-game> \
  --game-license <publisher-provided-license>
```

The complete editions are optional. The build rejects a payload that does not match the support
matrix or that already contains mod files. Supplying a license file does not create distribution
rights; the person publishing the archive is responsible for having the publisher's permission.

It needs the game's reference assemblies (`bibites-mod/sync-game-refs.sh`), the .NET SDK and Go —
the build side, never the player's side. Everything heavy runs under `nice -n 19`, because this
host runs the living deployment.

**It refuses to build a release nobody has run.** Before it packages anything it requires:

1. `bibites-mod/libs/BibitesAssembly.dll` to be the game build `docs/support-matrix.md`'s
   **Windows** row names — that is the reference assembly the mod is compiled against;
2. the plugin it builds to be **byte-identical** to the one in `farend/dist/farend-bundle.zip` —
   the tracked artifact set the living fleet runs — and every staged copy to be that same file;
3. the cross-compiled Windows sidecar to be byte-identical to the same bundle's copy;
4. and, when this machine's game directory is readable, the deployed plugin to agree as well.

It also checks two things the two-platform matrix made checkable: that **every matrix entry
carries the same keys** and that no two rows share a `(gameVersion, platform)` pair — because both
installers walk the whole list, and PowerShell under `Set-StrictMode` throws on a property one row
happens not to have. And when an unpacked Linux game is readable (`LINUX_GAME_DIR`, defaulting to
the rehearsal's copy), it checks the Linux row's hash against a real file.

**The Linux sidecar is the one artifact with no byte-identity reference**, and the script says so
rather than inventing one: the fleet is Windows, so the bundle holds nothing to compare against.
What stands in its place is the check that already gates the Windows build — `go/` identical,
commit for commit, to the tree the deployment's sidecar was built from. Same source, second target.

A mismatch stops the build and names which side moved. The fix is never to loosen the check: it
is to deploy, rebuild the far-end bundle, and release from there.

Every archive is built deterministically — fixed timestamps, sorted entries, no extra attributes
— so rebuilding from the same tag and inputs reproduces the checksums on the page. Linux archives
keep their mode bits, and the build reads them back out of the finished zip, including the game
executable in a complete archive.

## Testing the install and the uninstall

```sh
release/test-install-uninstall.ps1     # run it through powershell.exe
release/test-install-uninstall.sh --game-assembly <the LINUX BibitesAssembly.dll>
```

Each builds a sandbox game directory, runs the real installer against it with a sandbox data root,
runs the real uninstaller, and requires the tree to be **hash-for-hash identical** to what it was
before the install — the Linux one requires the permissions to match as well. Neither touches a
real copy of the game, a trust store, or a live process. Run them after any change to either kit.

**The Linux proof needs no release build**: with no `--kit-dir` it stages a kit out of this
checkout — the two real scripts, the matrix extracted from `docs/support-matrix.md`, the real
BepInEx archive, and stand-ins for the plugin and the sidecar, neither of which it ever executes.
Both suites include a complete-package scenario: no game path is supplied, the game lands in a
versioned managed runtime, uninstall removes unchanged payload files, and a user-added file is
kept. Linux also covers **the other platform's build of the same game version** and **a kit file
that fails its manifest**.

## Publishing — the four steps, by hand

`make-release.sh` deliberately does none of these.

1. **Read `dist/RELEASE-PAGE.md`.** The build refuses unresolved template fields. Make sure that
   the generated page describes the intended artifacts and public map.
2. **Tag the commit the artifacts were built from**, and push the tag:
   `git tag m5.0 && git push origin m5.0`. The page's links point into the tag, so the
   documentation a reader follows is the documentation this release shipped with.
3. **Create the release** with `dist/RELEASE-PAGE.md` as its body. Attach both add-on archives,
   each complete archive that you built, and `SHA256SUMS`:
   ```sh
   gh release create m5.0 \
       release/dist/bibites-multiverse-m5.0-*.zip \
       release/dist/SHA256SUMS \
       --title "Bibites Multiverse m5.0" \
       --notes-file release/dist/RELEASE-PAGE.md
   ```
4. **Read the published page as a stranger would**, in a browser, and check the three things that
   matter most about its shape: the checksum step is **above** the download links; each download
   link's file matches the checksum beside it; and **the Linux row does not read as an
   afterthought** — a reader on that platform should meet the checksum-then-executable-bit
   ordering, the launcher difference and the one-instance-per-game-folder warning without having
   to read the Windows sections first.

**A fifth step, if the repository is private:** the page's documentation links resolve only for
somebody who can read the repository. Make it public, or the four participant pages have to
travel with the release instead.
