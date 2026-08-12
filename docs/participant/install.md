# Install

**What you need:** Windows, Steam, and The Bibites. **What you do not need:** a compiler, an
SDK, a runtime, or anything from a developer's toolchain. If an installation step asks you for
one of those, stop — that is a defect, and reporting it is the right response.

**Where it comes from.** One archive, from this project's GitHub release page:
`bibites-multiverse-m5.0-windows-x64.zip`, with `SHA256SUMS` beside it. Nothing else is a
download, and no installer here fetches anything from the internet while it runs.

## Before you download

**The release page carries published checksums, and it carries them above the download link.**
Check the file you got against the checksum on the page before you run anything:

```powershell
(Get-FileHash -Algorithm SHA256 .\bibites-multiverse-m5.0-windows-x64.zip).Hash -eq '<the value on the page>'
```

`True` means you have the published file. If it says `False`, delete the download and try again;
if it says `False` twice, report it and do not run it. That is `INS-CHECKSUM`.

Windows marks every file that came out of a downloaded archive, and it will refuse to run parts
of the package until that mark is cleared. **Clear it once, on the archive, after the checksum
has passed and before you unpack it:**

```powershell
Unblock-File .\bibites-multiverse-m5.0-windows-x64.zip
```

or right-click the archive → **Properties** → tick **Unblock**. Files extracted from an unmarked
archive carry no mark. **The order is the point**: the checksum is what makes clearing the mark a
decision rather than a ritual. If you unpacked first, the installer's own first step verifies
every file against the `MANIFEST.sha256` inside the archive and then clears the mark from exactly
those files, by name, and from nothing else on your machine. That is `INS-MARKOFWEB`.

**This release is not code-signed**, so a fresh Windows PowerShell will not run its scripts at
all until you either use PowerShell 7 — whose default policy on Windows is already
`RemoteSigned` — or allow your own account to run local scripts with
`Set-ExecutionPolicy -Scope CurrentUser -ExecutionPolicy RemoteSigned`. That second one is a
narrowing rather than an off switch: it still refuses any unsigned script carrying the mark of
the web, it touches no other account, and
`Set-ExecutionPolicy -Scope CurrentUser -ExecutionPolicy Undefined` puts it back. **You will
never be asked for `-ExecutionPolicy Bypass`**, by this page or by anything in the archive. The
release page states the same two options and what signing would cost instead.

## The support matrix, and why the installer may stop

Steam updates The Bibites on its own schedule. This project publishes a **support matrix**: for
each game version, the mod and sidecar build that go with it. **The installer checks your
game's build against that matrix and stops if there is no entry.**

That refusal is doing its job. There is no flag to bypass it, and joining the map with an
unsupported build is not something the software offers. Either wait for a release that lists
your build, or install the build the matrix names.

**The matrix is a per-machine question and never a statement about the map.** Whether your
build can join *this map* is a different test, answered by the relay against the wire version —
see [join.md](join.md).

**Where it is published, and how to look yourself up.** The table is
[`../support-matrix.md`](../support-matrix.md), beside every release, and a machine-readable copy
of it travels inside the archive as `support-matrix.json` — the same bytes, so the words the
installer refuses with are the words on the page. It matches on the **SHA-256 of
`The Bibites_Data\Managed\BibitesAssembly.dll`** rather than on a version string, because a
version string is a value a build can reuse. Today the matrix has one row: game **0.6.3.1**, mod
`0.6.4`, sidecar `m5.0`. You can check yourself before downloading anything — the matrix page
gives the one-line command.

`INS-GAMEBUILD` is the refusal, and it prints the matrix's own sentence: *"This release supports
one game build, and the game on this machine is not it… Nothing about the map can change this,
and there is no flag that skips it. Two ways forward: wait for a release whose matrix lists your
build, or put this machine on a build this matrix lists."* **Nothing is installed when it fires**
— the check runs before the mod framework, before the plugin, and before your credential is
written anywhere.

## What the installer does

```powershell
.\Install-BibitesMultiverse.ps1     # asks for your join string, with the typing hidden
.\Start-Multiverse.ps1              # the sidecar, then the game
.\Stop-Multiverse.ps1               # the game, then the sidecar
```

**No parameter takes your join string on the command line**, on purpose: a value typed there is
in every process listing on your machine and in your shell history, and the wire itself has the
same rule. `-JoinStringFile .\join.txt` reads it from a file if you would rather not type it —
and then delete that file, which the installer tells you to do and will not do for you.

`Install-BibitesMultiverse.ps1` works in nine steps it names on your screen as it goes: it checks
itself against `MANIFEST.sha256` and clears the mark of the web from the files it verified;
finds Steam's copy of the game; checks the build against the matrix; installs BepInEx if it is
not already there; copies the plugin; splits your join string and stores the secret half in a
file only you can read; imports a certificate authority **only** if you gave it one for a
private map; states the settings this install ships with; and writes `Start-Multiverse.ps1`,
`Stop-Multiverse.ps1` and the record the uninstall reads.

**It needs no administrator rights**, adds no service, no scheduled task and no registry entry,
starts nothing by itself, and never touches your worlds or their backups.

**The uninstall.** `Uninstall-BibitesMultiverse.ps1`, and `-DryRun` first if you want the ledger
without the act. It reads the record the installer wrote and removes only what is named in it,
checking each file's hash before it goes, and prints a line per path for what it removed and what
it kept. Three things it deliberately keeps: **a file somebody changed after the install** — a
changed plugin is reported and left; **BepInEx**, whole, if it was on your machine before;
and **your journal**, which is the record of organisms other worlds handed you, unless you pass
`-RemoveWorldData`. Its two refusals are the game still running from that folder and this
install's own sidecar still running — both stop before removing anything, and both name the
command that fixes them.

## What a bare install does, stated once

**An unconfigured install exports on all four edges.** Nothing configured means the whole
perimeter, not silence — that is the shipped default and it is deliberate. The installer says
so in its own output, and this document says so here, so that neither is the only place a
reader could have learned it.

**An unconfigured install also connects to nothing.** Without a relay address and a credential
there is no map to join, so the export default is a question about *what your world means to
do* rather than a question about safety. When you join, it means: organisms leave your world on
every side, and arrive from every side.

If you want a wall on one side, say so: `-ExportEdges E,N` at install time, or the same value in
`Start-Multiverse.ps1` afterwards. If you want your world off the map entirely, do not join it.

## The settings a fresh install ships with

Four of them are worth reading before you start, because each one spends something of yours.

**All of them live in one file you can read and edit: `Start-Multiverse.ps1`**, which the
installer writes beside itself. Each is written out explicitly, *including* the ones that match
the mod's own default, so that a future change to a default cannot silently move what your world
does.

| Setting | In `Start-Multiverse.ps1` | Ships as | What it costs you |
|---|---|---|---|
| Export edges | `MULTIVERSE_EXPORT_EDGES` (installer: `-ExportEdges`) | `E,N,W,S` | Your world is a full member of the map in every direction. This is the default and it is stated in three places on purpose |
| Migration exclusion | `MULTIVERSE_MIGRATION_EXCLUDE` (installer: `-ExcludeSpecies`) | `Basic bibite` | It keeps founder stock off the lanes. **An empty value turns the policy off**, which floods a shared map with seed genomes and looks entirely normal in the census while it happens — so the installer refuses an empty value and makes you say `-NoMigrationExclusion` if you mean it, and prints what it costs when you do |
| Save interval and retained saves | `MULTIVERSE_SAVE_MINUTES`, `MULTIVERSE_SAVE_KEEP` (installer: `-SaveMinutes`, `-SaveKeep`) | `10` and `6` | Six copies of your world on your disk that you did not budget for — measured at about 2.4–2.9 MB in total on this project's own worlds. The interval is also how often your world pauses to write itself out — see [diagnose.md](diagnose.md) |
| Save on quit | `MULTIVERSE_SAVE_ON_QUIT` (installer: `-SaveOnQuit on\|off`) | `true` | Your world is written out when the game closes, so stopping is not losing |

The same names appear in the game's own settings file,
`<game folder>\BepInEx\config\dev.multiverse.bibites.cfg`, as `ExportEdges`,
`MigrationExcludeSpecies`, `SaveMinutes`, `SaveKeep` and `SaveOnQuit`. **The values in
`Start-Multiverse.ps1` win**, because the mod reads its environment before that file. Edit the
start script; the config file is there for somebody starting the game another way.

Every default this release ships with — these four, and the two the operator side has — is
audited in [`../defaults-audit.md`](../defaults-audit.md), with what a bare install does and a
verdict.

## Two flags no document here will ever ask you to pass

The relay and the sidecar each have a flag that disables their authentication for a
single-machine rehearsal. **No installer, script, page or document this project ships may
instruct you to use either one.** If something does, that is the defect.

They are named differently from each other on purpose, so that no instruction can confuse the
two, and neither is a thing a participant needs. One belongs to the relay and is not even in the
package; the other belongs to the sidecar, is off, and the generated `Start-Multiverse.ps1` does
not pass it. Both are named and audited in [`../defaults-audit.md`](../defaults-audit.md), so
that you can recognise one if somebody ever tells you to type it.

## Next

[join.md](join.md) — what a join string is, what happens on your first claim, and what joining
publishes about your world.
