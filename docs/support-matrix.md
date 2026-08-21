# The support matrix

**Which mod and sidecar build goes with which build of The Bibites, on which platform.** This is
a **per-machine** question, settled on your own computer before anything dials a map. It is never
a statement about the map, and no entry here decides whether you may join one.

**This page is published beside every release** and the installer inside the release reads the
same table — the machine-readable copy at the end of this document is the one it reads, and it
is copied into **every** release archive as `support-matrix.json` unchanged. The Windows kit and
the Linux kit read the same bytes and refuse in the same words.

**A row is keyed on a game version *and* a platform**, because the two builds of one game version
are not the same file. `0.6.3.1` on Windows and `0.6.3.1` on Linux differ by 512 bytes in
`BibitesAssembly.dll`, so one hash cannot stand for both, and an installer that matched a hash
without knowing which platform it was on would be guessing.

## Two version tests that never meet (D22)

| The test | Who applies it | What it decides |
|---|---|---|
| **The wire version** — `contract-b/4.1` between your sidecar and the relay | The **relay**, at your first connection | Whether you are a member of that map. It is the only version question the map has an opinion about |
| **The game version** — the build of The Bibites this machine runs, from Steam on Windows or from itch.io on Linux | **Your own machine**, through this matrix and the installer's check | Whether a mod and sidecar build exists that works with your game at all |

The mod is a Harmony patch against a named game assembly, so the second test cannot be
delegated: a build the mod was not compiled against can fail to load, or load and behave
differently. That is why the installer refuses rather than warns, and why there is no flag that
skips it.

## The matrix

| Game version | Platform | Store | Store build | Mod | Sidecar | BepInEx | Wire | Tested against |
|---|---|---|---|---|---|---|---|---|
| **0.6.3.1** | Windows | Steam | app `2736860`, buildid `22383127` | `0.6.7` | `m5.0` | `5.4.23.3` `win_x64` | `contract-b/4.1`, `contract-a/2.4` | The six-world test deployment ran continuously from 2026-08-11 through publication on 2026-08-15 |
| **0.6.3.1** | Linux | itch.io | upload `16838443` | `0.6.7` | `m5.0` | `5.4.23.3` `linux_x64` | `contract-b/4.1`, `contract-a/2.4` | **One** 14-minute authenticated headless session, 2026-08-12, against a scratch relay: the mod loaded, every subsystem armed, eight saves rotated and pruned, clean quit with its save |

**`0.6.3.1` is the only tested game version, on two platforms**, and each row says what "tested"
means *for that row* rather than leaving the word to the reader. **The two rows do not carry the
same weight and the table does not pretend they do:** the Windows row covers a four-day continuous
six-world run. The Linux row is one rehearsal session on one machine on one day. A build that is
not in the table is not "probably fine": it is untested, and the installer treats untested as
unsupported.

**What makes the Linux row trustworthy for the thing the mod actually does** is not session
length. The mod is Harmony patches against named types, and a full decompile of both builds,
diffed, puts **every** difference between them in three places the mod never touches: the native
file-dialog shim (`StandaloneFileBrowserWindows` becomes `StandaloneFileBrowserLinux`), three
"reveal the save folder" calls that become no-ops, and Unity's own type bookkeeping. **All six
types the mod patches or reads are byte-identical across the two builds**, and all ten patch
targets are present on both sides.

The identity check is on the **SHA-256 of `BibitesAssembly.dll`**, not on a version string,
because a version string is a value the build can reuse and a hash is not. **The key is the pair
(game version, platform)** — the same version has two hashes:

| Game version | Platform | `BibitesAssembly.dll` SHA-256 |
|---|---|---|
| 0.6.3.1 | Windows | `12455E485199CDBCAEA5978B8B0095EEDCBDD09D1FB87EFD65CCACB15D96E7EE` |
| 0.6.3.1 | Linux | `5B145A0A941C3560888BAFBB320D984D7290A63467C9F65022FDB02878847ECA` |

The 512-byte difference between them is one PE alignment unit and it is the file-dialog shim.
**Each installer looks up only its own platform's rows**, so a hash that belongs to the other
platform is `INS-GAMEBUILD` rather than a confusing match.

### Where the Linux build in that row came from

Recorded so that "the itch.io build" is an artifact somebody can check rather than a phrase:

| | |
|---|---|
| Page | `thebibites.itch.io/the-bibites`, the Linux download |
| Upload id | `16838443` |
| Archive | `TheBibites-0.6.3.1-Linux.zip`, 79 773 740 bytes, 185 files |
| Archive SHA-256 | `E15695D7944B4DED9E6D29A21518D00E9689C1A2B8CEB7288D51651ADCE57F4E` |

**itch.io download links are per-purchase and time-limited**, so the link is not a fact this page
can publish; the upload id and the archive's checksum are. If your download's checksum differs
from the value above, the store has published a new build and the row below the change is the one
to read: your `BibitesAssembly.dll` hash is the test, not the file you downloaded it in.

## Looking your own build up

Four ways, in the order of least effort:

1. **Let the installer do it.** It is the first thing the installer checks after it finds your
   game — `Install-BibitesMultiverse.ps1` on Windows, `install-bibites-multiverse.sh` on Linux —
   and it prints the build it found either way.
2. **Read it in Windows PowerShell**, without installing anything:

   ```powershell
   (Get-FileHash -Algorithm SHA256 `
       "$env:ProgramFiles(x86)\Steam\steamapps\common\The Bibites\The Bibites_Data\Managed\BibitesAssembly.dll").Hash
   ```

   Compare the result with the Windows row above. Your Steam library may be on another drive; the
   installer finds it for you.
3. **Read it on Linux** with `sha256sum`, which is in coreutils and is already on your machine:

   ```sh
   sha256sum "<game folder>/The Bibites_Data/Managed/BibitesAssembly.dll"
   ```

   `sha256sum` prints lower case and the table is upper case; they are the same value. The game
   folder is wherever you unpacked the itch.io archive — there is no registry and no library index
   to find it in, so the two usual answers are the itch app's own install root,
   `~/.config/itch/apps/the-bibites`, and wherever you put it by hand. The installer looks in the
   first, and asks you for the second.
4. **Read it out of the game's own log** once the mod is installed. The plugin logs
   `Application.version = 0.6.3.1` at startup, in `<game folder>/BepInEx/LogOutput.log` — and on
   Linux, **read the warning about that file in [`error-taxonomy.md`](error-taxonomy.md)
   (`LOCAL-LOGSHRED`) before you trust what it says**, because more than one game instance run out
   of one game folder shreds it.

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
credential is written anywhere. The words above are the whole of the refusal and neither installer
adds to them; what each one prints *around* them is your own hash, your platform, and the rows this
release has — so a Windows hash on a Linux machine reads as the mistake it is rather than as a
mystery.

**Steam updates the game on its own schedule and this project cannot defer that for you.** The
honest reading of an off-matrix build is *wait for a release*, and the release is how this
project moves its fleet: it pushes nothing to anybody (D25).

**On Linux the update problem is the other way round.** The itch.io download updates nothing by
itself — you have the archive you unpacked until you deliberately fetch another — so an off-matrix
Linux build is almost always a build somebody *chose*, and the way back is the archive whose
checksum this page publishes. The two platforms fail in opposite directions and the remedy is the
same sentence.

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

**A platform is a row, not a footnote.** A game version that has been run on Windows and not on
Linux gets the Windows row and not the Linux one, however small the difference between the two
files is measured to be. The mod being the same IL on both platforms is an argument for *expecting*
the second row to be earnable, never for writing it before somebody earned it.

**There is no push.** A release page updates nobody's install: you download the new release when
your build needs it. The relay-side minimum wire version is raised only *after* the release that
satisfies it exists.

## The tested build

**The rows above say which *game* build this release supports. This section says which build of
*this project* earned that claim** — the mod and sidecar that were built, run, and observed, named
by hash rather than kept as a committed binary. It is `testedBuild` in the block below, and it is
one record rather than a per-row one because the plugin is a single file for both platforms.

| | |
|---|---|
| Mod version | `0.6.7` |
| `BibitesMultiverse.dll` SHA-256 | `2bd10461632f3f391ec85e3a8db6496b64ba35f2c3801354da5800a74a9ac861` |
| `bibites-mod/` tree | `bc3d8733e323a837d87dd0423bdd2f24862432bc` |
| `cmd/sidecar` source commit | `e0f9bb217fc3df6daefdc049e152a56b0419aa5b` |
| `cmd/sidecar` input digest | `cd7d606819d16cc464f778f6905156e7f9f30bb0743001bcd3c2c3f9904eb427` |
| Tested on | 2026-08-21 |

**Three refusals stand on those values.** `release/make-release.sh` will not package a release
whose freshly built plugin has a different SHA-256, whose `cmd/sidecar` input manifest digests to
something else, or whose plugin declares a `Version` other than `mod`; `release/check-drift.sh`
asks the same three questions from tracked text alone — no game, no .NET, no network — on every
pull request. The input digest is the digest of the manifest `release/lib/sidecar-manifest.sh`
writes: every file under `go/` that `cmd/sidecar` reaches, with its hash, plus the identity of
every external module that graph selects. Two checkouts whose manifests agree build the same
sidecar, so the digest catches *the sidecar source moved and the plugin did not* — the easy
mistake, because the sidecar changes far more often than the mod.

**What the record is worth, exactly.** It is self-asserted: somebody built these bytes, ran them,
and wrote down what they ran. It catches **forgetting** — a mod change that lands without a test,
a sidecar change that never reached a tested binary — and it does not prove that a test happened.
The `evidence` sentence is where the run is described, and it is written by the person who ran it.

**`Tested on` is the date of the most recent leg, not of every value above it.** The sidecar
changes far more often than the mod, so a re-record often re-attests only the sidecar half while
the plugin's hash and tree stay exactly where the earlier test left them. That is honest as long
as the `evidence` sentence says which leg proved what — and it must, including when the run
happened on a host whose game or mod is not the one this matrix names. Today's record is one of
those: the sidecar values were re-attested on 2026-08-21 after a formatting-only change. The
Windows and Linux `amd64` binaries built from the new source and from the previously recorded
source were byte-identical with VCS metadata disabled, and the full Go test and race suites
passed. This is equivalence evidence, not a new live run. Operational evidence still rests on the
2026-08-19 live broadcast-world run, and the unchanged plugin values still rest on the 2026-08-17
test.

**Updating it is the last act of a test, not the first act of a release.** Build the plugin, run
it, then record what ran: [`release/README.md`](../release/README.md) has the procedure and
`release/record-tested-build.sh` prints the block ready to paste. The `mod` field on both rows
moves in the same edit, because a release names one plugin version and this is where that number
is decided.

**The tree hash moves on its own in exactly one case.** `bibites-mod/` also holds developer
scripts — `deploy.sh`, `sync-game-refs.sh`, `game.sh`, `lib/` — that the plugin is not built from.
Changing one of them moves the directory's tree hash and cannot move the plugin's bytes, so
re-point `bibitesModTree` at the new tree in the same commit and leave the rest of the record
alone: `pluginSha256`, `testedOn` and `evidence` describe the test that happened. Prove it rather
than assert it — rebuild with `dotnet build bibites-mod/BibitesMultiverse.csproj -c Release` and
check the result still hashes to `pluginSha256`. Anything under `bibites-mod/src/` or
`BibitesMultiverse.csproj` is a build input, and that is a re-test, not a re-point.

---

## The machine-readable copy

**Both installers read this block, byte for byte.** `release/make-release.sh` extracts everything
between the two markers and writes it into **each** release archive as `support-matrix.json`, so
the words an installer prints on a refusal are the words on this page and cannot drift from them.
Edit the block and the tables above together.

**Every entry carries the same keys**, with no key that exists on one row and not another. That is
a rule and not an accident: both installers walk the whole list to print what the release supports,
and a row with a missing field is a reader that has to guess. Store-specific identity therefore
lives in one human-readable `storeBuild` string rather than in `steamAppId` / `itchUploadId` pairs
that only half the rows would have.

**`testedBuild` sits beside `release`, not on a row.** The plugin is one file for both platforms,
so a per-row copy would be the same data written twice and a change to the shape every installer
reads. The installers ignore it; the release gates do not.

| Key | What it is |
|---|---|
| `platform` | `Windows` or `Linux`. **With `gameVersion` this is the key of a row**; each installer filters the list to its own platform before it matches a hash |
| `store` | Where that build comes from — `Steam`, `itch.io` |
| `storeBuild` | That store's own name for the build, as a string meant for a person |
| `bepInExFlavour` | Which BepInEx archive the row's installer unpacks: `win_x64` or `linux_x64`. The version is `bepInEx` and is the same on both |
| `tested` | What "tested" means **for that row**. The two rows do not say the same thing |

<!-- SUPPORT-MATRIX-JSON-BEGIN -->
```json
{
  "matrix": "bibites-multiverse/support-matrix/3",
  "release": "0.3.2",
  "published": "2026-08-20",
  "testedBuild": {
    "mod": "0.6.7",
    "pluginSha256": "2bd10461632f3f391ec85e3a8db6496b64ba35f2c3801354da5800a74a9ac861",
    "bibitesModTree": "bc3d8733e323a837d87dd0423bdd2f24862432bc",
    "sidecarSourceCommit": "e0f9bb217fc3df6daefdc049e152a56b0419aa5b",
    "sidecarInputsSha256": "cd7d606819d16cc464f778f6905156e7f9f30bb0743001bcd3c2c3f9904eb427",
    "testedOn": "2026-08-21",
    "evidence": "on 2026-08-21 the sidecar source at e0f9bb2 was re-attested after gofmt-only changes: its Windows and Linux amd64 binaries were byte-identical to binaries built from the previously recorded e6cd359 source when both were built with CGO disabled, -trimpath, and VCS metadata disabled, and the full Go test and race suites passed; no new live deployment was run, so operational evidence remains the 2026-08-19 live public /watch broadcast-world run at slot 7, where the recorded sidecar recovered 64 journaled forwards, expired 66 lost forwards, cleared JOURNAL_FULL refusals during an 18-minute soak, restored outbound traffic, and passed diagnose; this equivalence re-attests the sidecar half only, while unchanged pluginSha256 and bibitesModTree still rest on the 2026-08-17 mod 0.6.7 test; not exercised on 2026-08-21: live relay connectivity, slot claim, mod traffic, crash replay, compaction, roll-up, archive, re-route or bounce-back, inflight administration, private-map behavior, and disk-failure paths"
  },
  "keyedOn": "gameVersion and platform",
  "refusal": "This release supports one game build, and the game on this machine is not it. The mod is a Harmony patch against a named game assembly: on a build it was not compiled against it can fail to load, or load and behave differently, and neither is a thing an installer may risk on your world. Nothing about the map can change this, and there is no flag that skips it. Two ways forward: wait for a release whose matrix lists your build, or put this machine on a build this matrix lists.",
  "entries": [
    {
      "gameVersion": "0.6.3.1",
      "platform": "Windows",
      "store": "Steam",
      "storeBuild": "app 2736860, buildid 22383127",
      "assemblySha256": "12455E485199CDBCAEA5978B8B0095EEDCBDD09D1FB87EFD65CCACB15D96E7EE",
      "mod": "0.6.7",
      "sidecar": "m5.0",
      "bepInEx": "5.4.23.3",
      "bepInExFlavour": "win_x64",
      "contractA": "contract-a/2.4",
      "contractB": "contract-b/4.1",
      "tested": "the six-world test deployment ran continuously from 2026-08-11 through publication on 2026-08-15"
    },
    {
      "gameVersion": "0.6.3.1",
      "platform": "Linux",
      "store": "itch.io",
      "storeBuild": "upload 16838443, archive sha256 E15695D7944B4DED9E6D29A21518D00E9689C1A2B8CEB7288D51651ADCE57F4E",
      "assemblySha256": "5B145A0A941C3560888BAFBB320D984D7290A63467C9F65022FDB02878847ECA",
      "mod": "0.6.7",
      "sidecar": "m5.0",
      "bepInEx": "5.4.23.3",
      "bepInExFlavour": "linux_x64",
      "contractA": "contract-a/2.4",
      "contractB": "contract-b/4.1",
      "tested": "one 14-minute authenticated headless session on 2026-08-12 against a scratch relay; all six patched types byte-identical to the Windows build"
    }
  ]
}
```
<!-- SUPPORT-MATRIX-JSON-END -->
