# The support matrix

**Which mod and sidecar build goes with which build of The Bibites.** This is a **per-machine**
question, settled on your own computer before anything dials a map. It is never a statement
about the map, and no entry here decides whether you may join one.

**This page is published beside every release** and the installer inside the release reads the
same table — the machine-readable copy at the end of this document is the one it reads, and it
is copied into the release archive as `support-matrix.json` unchanged.

## Two version tests that never meet (D22)

| The test | Who applies it | What it decides |
|---|---|---|
| **The wire version** — `contract-b/4.0` between your sidecar and the relay | The **relay**, at your first connection | Whether you are a member of that map. It is the only version question the map has an opinion about |
| **The game version** — the build of The Bibites your Steam copy is on | **Your own machine**, through this matrix and the installer's check | Whether a mod and sidecar build exists that works with your game at all |

The mod is a Harmony patch against a named game assembly, so the second test cannot be
delegated: a build the mod was not compiled against can fail to load, or load and behave
differently. That is why the installer refuses rather than warns, and why there is no flag that
skips it.

## The matrix

| Game version | Steam build | Mod | Sidecar | BepInEx | Wire | Tested against |
|---|---|---|---|---|---|---|
| **0.6.3.1** | app `2736860`, buildid `22383127` | `0.6.4` | `m5.0` | `5.4.23.3` | `contract-b/4.0`, `contract-a/2.4` | The project's own six-world deployment, continuously, since 2026-08-11 |

**`0.6.3.1` is the only tested build**, and the row says what "tested" means for it rather than
leaving the word to the reader. A build that is not in the table is not "probably fine": it is
untested, and the installer treats untested as unsupported.

The identity check is on the **SHA-256 of `The Bibites_Data\Managed\BibitesAssembly.dll`**, not
on a version string, because a version string is a value the build can reuse and a hash is not:

| Game version | `BibitesAssembly.dll` SHA-256 |
|---|---|
| 0.6.3.1 | `12455E485199CDBCAEA5978B8B0095EEDCBDD09D1FB87EFD65CCACB15D96E7EE` |

## Looking your own build up

Three ways, in the order of least effort:

1. **Let the installer do it.** It is the first thing `Install-BibitesMultiverse.ps1` checks
   after it finds your game, and it prints the build it found either way.
2. **Read it in Windows PowerShell**, without installing anything:

   ```powershell
   (Get-FileHash -Algorithm SHA256 `
       "$env:ProgramFiles(x86)\Steam\steamapps\common\The Bibites\The Bibites_Data\Managed\BibitesAssembly.dll").Hash
   ```

   Compare the result with the table above. Your Steam library may be on another drive; the
   installer finds it for you.
3. **Read it out of the game's own log** once the mod is installed. The plugin logs
   `Application.version = 0.6.3.1` at startup, in `<game folder>\BepInEx\LogOutput.log`.

## When your build is not in the table

**The installer stops, and this is the sentence it prints** — the matrix's own words, quoted
verbatim by the installer so that the page and the tool cannot drift apart:

> This release supports one game build, and the game on this machine is not it. The mod is a
> Harmony patch against a named game assembly: on a build it was not compiled against it can
> fail to load, or load and behave differently, and neither is a thing an installer may risk on
> your world. Nothing about the map can change this, and there is no flag that skips it. Two
> ways forward: wait for a release whose matrix lists your build, or put this machine on a build
> this matrix lists.

That refusal is `INS-GAMEBUILD` in [`error-taxonomy.md`](error-taxonomy.md). **Nothing was
installed** when it fires: the check runs before BepInEx, before the plugin and before your
credential is written anywhere.

**Steam updates the game on its own schedule and this project cannot defer that for you.** The
honest reading of an off-matrix build is *wait for a release*, and the release is how this
project moves its fleet: it pushes nothing to anybody (D25).

## What a map with two game builds on it does

**It partitions, and that is the accepted behaviour rather than a defect** (DQ5,
`contract-b-m4.md` §22 B31). Two machines on different game builds do not exchange organisms:

- The relay refuses whichever peer disagrees with the game version the map already reports —
  close `4003` at connect, `version_incompatible` on a claim.
- The routing walk closes the lanes between mismatched worlds, on both the relay's side and the
  sidecar's, so a mismatched neighbour reads as `peer_incompatible` rather than as a fault.
- If a payload ever reaches a mod on another build, the import answers a **permanent**
  `VERSION_UNSUPPORTED` rather than restoring something the game may misread.

**That is safe** — no cross-version organism reaches the game's loader at all — and it is why
the cross-version payload question stays *assumed and untested* rather than validated.

**What it costs you:** after a game update that lands unevenly, the map is split until every
machine is on one build, and your lanes to a machine on the other side of the split are closed
for as long as that lasts. Nothing is lost while it lasts. Your own world runs normally, your
journal keeps custody of what it holds, and your slot and position are yours throughout.

## How this table moves

A new game build gets a row only after a mod and sidecar build exist for it and have been run
against it. The sequence is always the same, and the middle step is the one that takes time:

1. The game updates; this project re-syncs its reference assemblies and rebuilds.
2. The new build is run on the project's own deployment.
3. A release is published, and **this table gains a row in the same act**.

**There is no push.** A release page updates nobody's install: you download the new release when
your build needs it. The relay-side minimum wire version is raised only *after* the release that
satisfies it exists.

---

## The machine-readable copy

**The installer reads this block, byte for byte.** `release/make-release.sh` extracts everything
between the two markers and writes it into the release archive as `support-matrix.json`, so the
words the installer prints on a refusal are the words on this page and cannot drift from them.
Edit the block and the table above together.

<!-- SUPPORT-MATRIX-JSON-BEGIN -->
```json
{
  "matrix": "bibites-multiverse/support-matrix/1",
  "release": "m5.0",
  "published": "2026-08-12",
  "refusal": "This release supports one game build, and the game on this machine is not it. The mod is a Harmony patch against a named game assembly: on a build it was not compiled against it can fail to load, or load and behave differently, and neither is a thing an installer may risk on your world. Nothing about the map can change this, and there is no flag that skips it. Two ways forward: wait for a release whose matrix lists your build, or put this machine on a build this matrix lists.",
  "entries": [
    {
      "gameVersion": "0.6.3.1",
      "steamAppId": "2736860",
      "steamBuildId": "22383127",
      "assemblySha256": "12455E485199CDBCAEA5978B8B0095EEDCBDD09D1FB87EFD65CCACB15D96E7EE",
      "mod": "0.6.4",
      "sidecar": "m5.0",
      "bepInEx": "5.4.23.3",
      "contractA": "contract-a/2.4",
      "contractB": "contract-b/4.0",
      "tested": "the project's own six-world deployment, continuously, since 2026-08-11"
    }
  ]
}
```
<!-- SUPPORT-MATRIX-JSON-END -->
