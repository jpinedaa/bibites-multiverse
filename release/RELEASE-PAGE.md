# Bibites Multiverse `@@RELEASE@@`

**A shared map for The Bibites.** Install this on your own copy of the game and your world
becomes one square of a grid of other people's worlds. Organisms that reach the edge of your
world cross into a neighbour's and go on living there; theirs arrive in yours. Your world stays
on your machine, saves to your disk, and is yours.

You need **The Bibites** — either the **Steam copy on Windows** or the **native Linux build from
itch.io** — and a **join string**, which the operator of a map hands you. You do not need a
compiler, an SDK or a runtime.

**Nothing in this release will ever ask you to turn a security control off.** No
execution-policy bypass, no `--insecure` flag, no skipped certificate check, no `curl | sh`, no
`sudo`. If any file, page or script here asks you for one of those, that is a defect: report it,
and do not work around it.

**Two archives, one mod.** The plugin inside them is the same file, byte for byte — it is
platform-independent IL. What differs is the sidecar, the mod framework, and the kit: PowerShell
against bash.

---

## Before you download — read this part first

A few things stand between a download and a program you should be willing to run. They are here,
above the download link, because that is where they are useful. **The first one is the same on
both platforms and it is the one that matters most.**

### 1. Check the file against the checksum below

The SHA-256 of every published file is on this page. Check yours before you unpack it.

**On Windows:**

```powershell
(Get-FileHash -Algorithm SHA256 .\@@ZIP_NAME@@).Hash -eq '@@ZIP_SHA256@@'
```

`True` means the file you have is the file that was published here.

**On Linux:**

```sh
sha256sum @@LINUX_ZIP_NAME@@
```

Compare what it prints with the value in the table below. `sha256sum` prints lower case and the
table is upper case in places; they are the same value.

**If it does not match, delete the download and try again. If it does not match twice, stop,
report it, and do not run it.** That is `INS-CHECKSUM` in the error taxonomy.

### 2. On Windows: clear the mark of the web — on the archive, before you unpack it

Windows attaches a mark to every file that came from the internet, and the mark travels into
each file you extract from an archive. It is a real control and this release does not switch it
off wholesale.

**Clear it once, on the archive, after you have checked its checksum:**

```powershell
Unblock-File .\@@ZIP_NAME@@
```

or right-click the archive → **Properties** → tick **Unblock** → OK. Then extract it. Files
extracted from an unmarked archive carry no mark.

Two things worth knowing. **The order is the point:** the checksum is what makes clearing the
mark a decision instead of a ritual. And if you extracted first, the installer's own first step
verifies every file against `MANIFEST.sha256` and then clears the mark from **exactly** those
files, by name — never from anything else on your machine.

That is `INS-MARKOFWEB` in the error taxonomy.

### 2b. On Linux: there is no mark of the web, and no ritual replaces it

**Nothing here arrives quarantined on Linux and no control has to be cleared**, so this page does
not invent a step to make the two platforms look alike. What Linux has instead is a permission
bit, and the installer's own first step handles it in the order that makes it a decision:

- it verifies every file in the folder against `MANIFEST.sha256`, and **then**
- makes executable exactly the files it just verified, by name, and nothing else.

**Checksum first, then the executable bit.** That is the same sentence as *checksum first, then
the mark* — it is the ordering, not the control, that this project cares about. If the archive
reached you with its mode bits intact you can simply run `./install-bibites-multiverse.sh`; if it
did not, `bash install-bibites-multiverse.sh` runs it once and its step 1 fixes the rest.

### 3. On Windows: running a PowerShell script at all

**This release is not code-signed** (see *Known limits* at the bottom for what that costs and why
it is not done yet), so Windows will not run its scripts under a fresh Windows PowerShell's
default policy. There are two honest ways in, and neither of them is `Bypass`:

**Either — use PowerShell 7**, whose default policy on Windows is already `RemoteSigned`. With
the archive unblocked before extraction, the installer runs with no policy change at all.
PowerShell 7 is Microsoft's own, and `winget install Microsoft.PowerShell` installs it.

**Or — allow your own account to run local scripts**, which is a narrowing rather than an
off switch:

```powershell
Set-ExecutionPolicy -Scope CurrentUser -ExecutionPolicy RemoteSigned
```

`RemoteSigned` still refuses any unsigned script that carries the mark of the web — which is why
step 2 above is a deliberate act on one archive rather than a blanket. It is the default on
Windows Server and in PowerShell 7. It changes nothing for other accounts and nothing
machine-wide, and you can put it back with:

```powershell
Set-ExecutionPolicy -Scope CurrentUser -ExecutionPolicy Undefined
```

**What you will not be asked to do**, by this page or by anything in the archive:
`-ExecutionPolicy Bypass`, `Get-ChildItem . | Unblock-File` on a folder nobody checked, or any
flag with `insecure` in its name.

---

## Downloads

**Take the one for your platform.** They are not interchangeable: each installer looks its own
platform's rows up in the support matrix, so the wrong archive stops with `INS-GAMEBUILD` rather
than doing something surprising.

| File | For | Size | SHA-256 |
|---|---|---|---|
| [`@@ZIP_NAME@@`](https://github.com/@@REPO@@/releases/download/@@TAG@@/@@ZIP_NAME@@) | Windows, the Steam copy | @@ZIP_SIZE@@ | `@@ZIP_SHA256@@` |
| [`@@LINUX_ZIP_NAME@@`](https://github.com/@@REPO@@/releases/download/@@TAG@@/@@LINUX_ZIP_NAME@@) | Linux, the itch.io build | @@LINUX_ZIP_SIZE@@ | `@@LINUX_ZIP_SHA256@@` |
| [`SHA256SUMS`](https://github.com/@@REPO@@/releases/download/@@TAG@@/SHA256SUMS) | both | — | the same two values, as a file |

Built @@BUILT_UTC@@ from `@@COMMIT@@`.

Inside the Windows archive, each file with its own SHA-256 — the installer checks all of them
against `MANIFEST.sha256` before it does anything:

@@INNER_TABLE@@

Inside the Linux archive, the same, and the mod is the same file:

@@LINUX_INNER_TABLE@@

---

## What a bare install does, stated before you install it

**Your world exports on all four edges.** Nothing configured means the whole perimeter, not
silence. Every edge is a door that works both ways: organisms leave your world on every side and
arrive from every side. That is the shipped default, it is deliberate, and the installer states
it again on your screen while it runs.

**An unconfigured install also connects to nothing.** Without a relay address and a credential
there is no map to join, so the export default is a question about what your world *means to do*
rather than a question about safety. If you want a wall on one side, say so with
`-ExportEdges`. If you want your world off the map entirely, do not join it.

The other four settings this release ships with, and what each one spends, are in
[install.md](https://github.com/@@REPO@@/blob/@@TAG@@/docs/participant/install.md) and in the
installer's own output.

---

## Your game build has to be in the support matrix

The mod is a patch against a named game assembly, so a build it was not compiled against can
fail to load or behave differently. **The installer checks your build and stops if there is no
entry for it** — before it installs anything.

| Game version | Platform | Store | Mod | Sidecar | BepInEx | Wire |
|---|---|---|---|---|---|---|
| **0.6.3.1** | Windows | Steam | `0.6.4` | `m5.0` | `5.4.23.3` `win_x64` | `contract-b/4.0`, `contract-a/2.4` |
| **0.6.3.1** | Linux | itch.io | `0.6.4` | `m5.0` | `5.4.23.3` `linux_x64` | `contract-b/4.0`, `contract-a/2.4` |

**A row is a game version *and* a platform**, because the same version is two different files:
`BibitesAssembly.dll` differs by 512 bytes between them, and one hash cannot stand for both. The
two rows also do not carry the same weight, and the matrix page says which is which rather than
letting the table imply they do.

The full table, how to look your own build up, and what a map with two game builds on it does,
are in [support-matrix.md](https://github.com/@@REPO@@/blob/@@TAG@@/docs/support-matrix.md).
Steam updates the game on its own schedule and nobody here can defer that for you; the itch.io
download updates nothing by itself, which is the same problem from the other side. If your build
is not listed, wait for a release that lists it. **This release page pushes nothing to anybody** —
the way this project moves its fleet is by publishing, and you update when you choose to.

---

## Install — Windows

Unpack the archive, open PowerShell in the folder, and:

```powershell
.\Install-BibitesMultiverse.ps1
```

It asks for your join string with the typing hidden. **No parameter takes the secret on a
command line** — a value typed there is in every process listing on your machine and in your
shell history — and the wire itself has the same rule.

Then:

```powershell
.\Start-Multiverse.ps1      # the sidecar, then the game
.\Stop-Multiverse.ps1       # the game, then the sidecar
```

A private or LAN map whose relay signs its own certificate needs one more argument,
`-CaFile .\ca.crt`. **On a public map nothing is imported into any trust store**, and the
installer says so while it runs.

## Install — Linux

Unpack the archive, open a terminal in the folder, and:

```sh
./install-bibites-multiverse.sh
```

The same hidden prompt, and the same rule: **no option takes the secret on a command line.**

```sh
./start-multiverse.sh      # the sidecar, then the game
./stop-multiverse.sh       # the game, then the sidecar
```

Three things are genuinely different here and none of them is cosmetic:

- **The game is started through BepInEx's own launcher**, `./run_bepinex.sh "./The
  Bibites.x86_64"`. The mod framework hooks the game through `LD_PRELOAD` and a shim library
  rather than through a DLL the loader picks up, so the launcher is the thing that runs.
- **A private map's authority is never written to a system trust store.** `--ca-file` puts the
  copy beside your data and sets `SSL_CERT_FILE` in the start script — the platform's own
  mechanism, for that one process. Nothing goes into `/etc/ssl` or
  `/usr/local/share/ca-certificates`, `update-ca-certificates` is never run, and no step needs
  root.
- **One game instance per game folder.** Two of them share BepInEx's log file, both keep writing
  to it, and it ends as mostly NUL bytes — while everything else works. See `LOCAL-LOGSHRED` in
  the error taxonomy, and unpack a second copy of the game if you want a second world.

`sha256sum`, `awk`, `unzip` and `file` are what it needs; the installer checks for all four before
it touches anything. `file` is used by BepInEx's own launcher, and without it the game will not
start.

## What it changes on your machine

| It adds | On Windows | On Linux |
|---|---|---|
| BepInEx, if you do not already have it | your game folder | your game folder, plus `run_bepinex.sh`, `libdoorstop.so` and `.doorstop_version` beside the game binary |
| `BibitesMultiverse.dll` | `<game folder>\BepInEx\plugins` | `<game folder>/BepInEx/plugins` |
| your map credential, your journal, your logs | `%LOCALAPPDATA%\BibitesMultiverse` | `${XDG_DATA_HOME:-$HOME/.local/share}/bibites-multiverse` |
| the start and stop scripts | `Start-Multiverse.ps1`, `Stop-Multiverse.ps1`, beside the installer | `start-multiverse.sh`, `stop-multiverse.sh`, beside the installer |

**Your worlds are not in any of that.** They stay where the game puts them:
`%USERPROFILE%\AppData\LocalLow\The Bibites` on Windows, and
`${XDG_CONFIG_HOME:-$HOME/.config}/unity3d/The Bibites/The Bibites` on Linux — where setting
`XDG_CONFIG_HOME` before the game starts moves the whole tree, which is a thing you do to the
game rather than to this mod.

Neither installer needs administrator rights or root. Neither adds a service, a scheduled task, a
systemd unit, a registry entry or a desktop file, and neither starts anything by itself. **Neither
touches your worlds or their backups.**

```powershell
.\Uninstall-BibitesMultiverse.ps1 -DryRun   # the ledger, changing nothing
.\Uninstall-BibitesMultiverse.ps1
```

```sh
./uninstall-bibites-multiverse.sh --dry-run
./uninstall-bibites-multiverse.sh
```

The uninstall reads the record the installer wrote and removes only what is named in it,
hash-checked, printing a line per path. If BepInEx was already on your machine it is left whole.
Your journal is kept unless you ask for it to go. **Both are proven by a test that installs into a
sandbox game tree and compares it hash-for-hash afterwards** — the Linux one compares permissions
as well.

## What joining publishes about your world

**Nothing on this wire is confidential, and that is a rule rather than a circumstance.** The map
broadcasts, to every other participant: your world's identity, slot and position; its population,
egg count and species census — the names your player sees; its mod, wire and game versions; its
save policy, exclusion list and whether it wraps; the speed it runs at and its queue depths.

That is a fairly complete profile of one world on your machine, and it is written here so you
know it before you join.
[join.md](https://github.com/@@REPO@@/blob/@@TAG@@/docs/participant/join.md) has the whole of it.

## The map you join is a bounded, announced commitment

The public map runs from **2026-08-14 to 2026-11-14** — **three months**. That is the
whole commitment. It is stated here so that you know it before you install anything.

**Your world is not part of that ending.** It lives and saves on your machine. It remains a
Bibites world with or without a map. What ends is the shared map: the relay, its status page,
and the operator's support of them.

**What happens to the record at the end is written down in advance.** This includes what
becomes of the map's files and when you receive notices. The reminders begin 30 days out.

**The bound can be extended, and it will never be silently shortened.** An extension is
announced no later than a week before the end. If the run must finish early, that date is also
announced at least a week ahead, with the same procedure.

Restarts during the run are routine and short. See
[join.md](https://github.com/@@REPO@@/blob/@@TAG@@/docs/participant/join.md) for what one looks
like from your side.

## Documentation

Written for somebody who installed a mod and joined a map, not for whoever built it.

| Page | What it answers |
|---|---|
| [install](https://github.com/@@REPO@@/blob/@@TAG@@/docs/participant/install.md) | What do I need, what does the installer do, and what does a bare install actually do? |
| [join](https://github.com/@@REPO@@/blob/@@TAG@@/docs/participant/join.md) | What is a join string, what happens on my first claim, and what does joining publish? |
| [diagnose](https://github.com/@@REPO@@/blob/@@TAG@@/docs/participant/diagnose.md) | Something is wrong. What do I read, in what order, and what do I send when I ask for help? |
| [leave](https://github.com/@@REPO@@/blob/@@TAG@@/docs/participant/leave.md) | What do stopping, leaving and handing my place over mean? |
| [error taxonomy](https://github.com/@@REPO@@/blob/@@TAG@@/docs/error-taxonomy.md) | Every refusal this system can hand you, with the remedy **and who must act** |
| [support matrix](https://github.com/@@REPO@@/blob/@@TAG@@/docs/support-matrix.md) | Which build goes with which game version, and what version skew does |
| [defaults audit](https://github.com/@@REPO@@/blob/@@TAG@@/docs/defaults-audit.md) | Every default this release ships with, what a bare install does with it, and the verdict |

**Never send anybody your join string** — not in an issue, a screenshot or a log. No log in this
system prints one, and no diagnostic asks for one. A message that contains one turns a support
question into a slot handover.

## Building it yourself

Everything here is built from the tagged tree by `release/make-release.sh`, which refuses to
produce an archive whose mod and sidecar are not byte-identical to the ones this project runs on
its own deployment. Clone the tag, run it, and compare hashes with the tables above. The Linux
sidecar is the one artifact with no byte-identity reference to compare against — the deployment
is Windows — so what the build proves for it is that `go/` is identical, commit for commit, to the
tree the deployment's sidecar was built from. Same source, second target, and the script says so
rather than implying more.

## Known limits of this release

- **It is not code-signed.** That is why step 3 above exists at all, on Windows. An Authenticode
  certificate — an OV certificate is roughly $200–400 a year, and an EV certificate, which also
  clears SmartScreen's reputation prompt immediately, roughly $300–700 a year on a hardware
  token — would remove the execution-policy step for everybody and reduce the mark-of-the-web
  step to a formality. It is a standing cost on a hobby project with one operator, and it is
  recorded as a decision to revisit rather than as an oversight. The floor this release commits
  to instead is the one above: **published checksums, and the mark-of-the-web story stated where
  a reader meets it before the download.** On Linux there is nothing to sign for and nothing to
  clear, so that whole class of step is absent rather than replaced.
- **Two platforms, and they are not equally proven.** The Windows/Steam row is weeks of continuous
  running on this project's own six-world deployment. The Linux/itch.io row is **one** 14-minute
  authenticated headless session on one machine on one day, plus a decompile of both builds that
  puts every difference between them outside the types the mod touches. The support matrix says
  exactly that in the row itself.
- **macOS is not supported.** BepInEx ships a `macos_x64` build and the game has a macOS build, so
  the shape of the work is known — but nobody has run it, and this project does not publish a row
  it has not earned.
- **One game build.** See the support matrix.
- **Whether an organism serialized by one game build loads in another is assumed and untested.**
  It is also unreachable: worlds on different game builds do not exchange organisms at all, by
  four deliberate gates. What that produces is a map that splits along a version boundary after a
  staggered game update, until everybody is on one build.
