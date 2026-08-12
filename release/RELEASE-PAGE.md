# Bibites Multiverse `@@RELEASE@@`

**A shared map for The Bibites.** Install this on your own copy of the game and your world
becomes one square of a grid of other people's worlds. Organisms that reach the edge of your
world cross into a neighbour's and go on living there; theirs arrive in yours. Your world stays
on your machine, saves to your disk, and is yours.

You need **Windows, Steam and The Bibites** — and a **join string**, which the operator of a map
hands you. You do not need a compiler, an SDK or a runtime.

**Nothing in this release will ever ask you to turn a security control off.** No
execution-policy bypass, no `--insecure` flag, no skipped certificate check. If any file, page or
script here asks you for one of those, that is a defect: report it, and do not work around it.

---

## Before you download — read this part first

Three things stand between a download and a program you should be willing to run. They are here,
above the download link, because that is where they are useful.

### 1. Check the file against the checksum below

The SHA-256 of every published file is on this page. Check yours before you unpack it:

```powershell
Get-FileHash -Algorithm SHA256 .\@@ZIP_NAME@@
```

Compare the `Hash` it prints with the value in the table below. They are the same string in a
different case; PowerShell can compare them for you:

```powershell
(Get-FileHash -Algorithm SHA256 .\@@ZIP_NAME@@).Hash -eq '@@ZIP_SHA256@@'
```

`True` means the file you have is the file that was published here. **If it says `False`, delete
the download and try again. If it says `False` twice, stop, report it, and do not run it.** That
is `INS-CHECKSUM` in the error taxonomy.

### 2. Clear the mark of the web — on the archive, before you unpack it

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

### 3. Running a PowerShell script at all

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

| File | Size | SHA-256 |
|---|---|---|
| [`@@ZIP_NAME@@`](https://github.com/@@REPO@@/releases/download/@@TAG@@/@@ZIP_NAME@@) | @@ZIP_SIZE@@ | `@@ZIP_SHA256@@` |
| [`SHA256SUMS`](https://github.com/@@REPO@@/releases/download/@@TAG@@/SHA256SUMS) | — | the same value, as a file |

Built @@BUILT_UTC@@ from `@@COMMIT@@`.

Inside the archive, each file with its own SHA-256 — the installer checks all of them against
`MANIFEST.sha256` before it does anything:

@@INNER_TABLE@@

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

| Game version | Mod | Sidecar | BepInEx | Wire |
|---|---|---|---|---|
| **0.6.3.1** | `0.6.4` | `m5.0` | `5.4.23.3` | `contract-b/4.0`, `contract-a/2.4` |

The full table, how to look your own build up, and what a map with two game builds on it does,
are in [support-matrix.md](https://github.com/@@REPO@@/blob/@@TAG@@/docs/support-matrix.md).
Steam updates the game on its own schedule and nobody here can defer that for you: if your build
is not listed, wait for a release that lists it. **This release page pushes nothing to anybody** —
the way this project moves its fleet is by publishing, and you update when you choose to.

---

## Install

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

## What it changes on your machine

| It adds | Where |
|---|---|
| BepInEx, if you do not already have it | your game folder |
| `BibitesMultiverse.dll` | `<game folder>\BepInEx\plugins` |
| your map credential, your journal, your logs | `%LOCALAPPDATA%\BibitesMultiverse` |
| `Start-Multiverse.ps1`, `Stop-Multiverse.ps1` | beside the installer |

It needs no administrator rights, adds no service, no scheduled task and no registry entry, and
starts nothing by itself. **It never touches your worlds or their backups.**

```powershell
.\Uninstall-BibitesMultiverse.ps1 -DryRun   # the ledger, changing nothing
.\Uninstall-BibitesMultiverse.ps1
```

The uninstall reads the record the installer wrote and removes only what is named in it,
hash-checked, printing a line per path. If BepInEx was already on your machine it is left whole.
Your journal is kept unless you ask for it to go.

## What joining publishes about your world

**Nothing on this wire is confidential, and that is a rule rather than a circumstance.** The map
broadcasts, to every other participant: your world's identity, slot and position; its population,
egg count and species census — the names your player sees; its mod, wire and game versions; its
save policy, exclusion list and whether it wraps; the speed it runs at and its queue depths.

That is a fairly complete profile of one world on your machine, and it is written here so you
know it before you join.
[join.md](https://github.com/@@REPO@@/blob/@@TAG@@/docs/participant/join.md) has the whole of it.

## The map you join is a bounded, announced commitment

@@OWNER:ANNOUNCED_PERIOD@@

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
its own deployment. Clone the tag, run it, and compare hashes with the table above.

## Known limits of this release

- **It is not code-signed.** That is why step 3 above exists at all. An Authenticode
  certificate — an OV certificate is roughly $200–400 a year, and an EV certificate, which also
  clears SmartScreen's reputation prompt immediately, roughly $300–700 a year on a hardware
  token — would remove the execution-policy step for everybody and reduce the mark-of-the-web
  step to a formality. It is a standing cost on a hobby project with one operator, and it is
  recorded as a decision to revisit rather than as an oversight. The floor this release commits
  to instead is the one above: **published checksums, and the mark-of-the-web story stated where
  a reader meets it before the download.**
- **Windows and Steam only.** The game has other builds; this release supports the Steam Windows
  one.
- **One game build.** See the support matrix.
- **Whether an organism serialized by one game build loads in another is assumed and untested.**
  It is also unreachable: worlds on different game builds do not exchange organisms at all, by
  four deliberate gates. What that produces is a map that splits along a version boundary after a
  staggered game update, until everybody is on one build.
