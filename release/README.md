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
| `kit/Install-BibitesMultiverse.ps1` | The installer a stranger runs. Windows PowerShell 5.1 and PowerShell 7; no toolchain, no administrator rights |
| `kit/Uninstall-BibitesMultiverse.ps1` | Removes what the installer recorded, hash-checked, and nothing else |
| `kit/README.md` | The page inside the archive |
| `RELEASE-PAGE.md` | The release page's text, with `@@…@@` fields the build fills in |
| `make-release.sh` | Builds `dist/` — the archive, `SHA256SUMS`, and the page with its checksums |
| `test-install-uninstall.ps1` | The proof that the uninstall leaves the game as it found it |
| `dist/` | Build output. **Not tracked** — see `.gitignore`, which says why |

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

It needs the game's reference assemblies (`bibites-mod/sync-game-refs.sh`), the .NET SDK and Go —
the build side, never the player's side. Everything heavy runs under `nice -n 19`, because this
host runs the living deployment.

**It refuses to build a release nobody has run.** Before it packages anything it requires:

1. `bibites-mod/libs/BibitesAssembly.dll` to be the game build `docs/support-matrix.md` names;
2. the plugin it builds to be **byte-identical** to the one in `farend/dist/farend-bundle.zip` —
   the tracked artifact set the living fleet runs;
3. the cross-compiled sidecar to be byte-identical to the same bundle's copy;
4. and, when this machine's game directory is readable, the deployed plugin to agree as well.

A mismatch stops the build and names which side moved. The fix is never to loosen the check: it
is to deploy, rebuild the far-end bundle, and release from there.

The archive is built deterministically — fixed timestamps, sorted entries, no extra attributes —
so rebuilding from the tag reproduces the checksum on the page.

## Testing the install and the uninstall

```sh
release/test-install-uninstall.ps1     # run it through powershell.exe
```

It builds a sandbox game directory, runs the real installer against it with a sandbox data root,
runs the real uninstaller, and requires the tree to be **hash-for-hash identical** to what it was
before the install. It touches no Steam copy of the game, no trust store, and no live process.
Run it after any change to either script.

## Publishing — the four steps, by hand

`make-release.sh` deliberately does none of these.

1. **Fill the owner-only fields in `dist/RELEASE-PAGE.md`.** The build lists every `@@OWNER:…@@`
   marker it left. Today that is the map's announced period and wind-down (D24), which is WP3's
   to state.
2. **Tag the commit the artifacts were built from**, and push the tag:
   `git tag m5.0 && git push origin m5.0`. The page's links point into the tag, so the
   documentation a reader follows is the documentation this release shipped with.
3. **Create the release** with `dist/RELEASE-PAGE.md` as its body and both files from `dist/` as
   its assets:
   ```sh
   gh release create m5.0 \
       release/dist/bibites-multiverse-m5.0-windows-x64.zip \
       release/dist/SHA256SUMS \
       --title "Bibites Multiverse m5.0" \
       --notes-file release/dist/RELEASE-PAGE.md
   ```
4. **Read the published page as a stranger would**, in a browser, and check the two things that
   matter most about its shape: the checksum and the mark-of-the-web steps are **above** the
   download link, and the download link's file matches the checksum beside it.

**A fifth step, if the repository is private:** the page's documentation links resolve only for
somebody who can read the repository. Make it public, or the four participant pages have to
travel with the release instead.
